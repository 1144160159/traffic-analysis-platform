package consumer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

type AssetUpsertedV2 struct {
	EventID          string             `json:"event_id"`
	EventType        string             `json:"event_type"`
	SchemaVersion    int                `json:"schema_version"`
	AggregateVersion int64              `json:"aggregate_version"`
	PartitionKey     string             `json:"partition_key"`
	TenantID         string             `json:"tenant_id"`
	AssetID          string             `json:"asset_id"`
	Revision         int64              `json:"revision"`
	TraceID          string             `json:"trace_id"`
	Asset            config.AssetRecord `json:"asset"`
}

type AssetProjectionEventConsumer struct {
	db *sql.DB
}

func NewAssetProjectionEventConsumer(db *sql.DB) (*AssetProjectionEventConsumer, error) {
	if db == nil {
		return nil, fmt.Errorf("asset projection database is required")
	}
	return &AssetProjectionEventConsumer{db: db}, nil
}

func (c *AssetProjectionEventConsumer) Handle(ctx context.Context, message *kafkaCommon.ReceivedMessage) error {
	var event AssetUpsertedV2
	if err := json.Unmarshal(message.Value, &event); err != nil {
		return fmt.Errorf("decode asset projection event: %w", err)
	}
	if err := validateAssetProjectionHeaders(message, event); err != nil {
		return err
	}
	return c.Accept(ctx, event, message.Partition, message.Offset, message.Value)
}

func validateAssetProjectionHeaders(message *kafkaCommon.ReceivedMessage, event AssetUpsertedV2) error {
	expected := map[string]string{
		"event_id":          event.EventID,
		"event_type":        event.EventType,
		"schema_version":    "2",
		"aggregate_version": fmt.Sprintf("%d", event.AggregateVersion),
		"tenant_id":         event.TenantID,
		"asset_id":          event.AssetID,
		"trace_id":          event.TraceID,
	}
	for key, value := range expected {
		if message.GetHeader(key) != value {
			return fmt.Errorf("asset projection header %s does not match payload", key)
		}
	}
	if string(message.Key) != event.PartitionKey {
		return fmt.Errorf("asset projection partition key does not match payload")
	}
	return nil
}

func (c *AssetProjectionEventConsumer) Accept(
	ctx context.Context,
	event AssetUpsertedV2,
	partition int,
	offset int64,
	rawPayload []byte,
) error {
	if err := validateAssetProjectionEvent(event); err != nil {
		return err
	}
	canonicalPayload, err := canonicalJSON(rawPayload)
	if err != nil {
		return fmt.Errorf("canonicalize asset projection event: %w", err)
	}
	payloadHash := sha256.Sum256(canonicalPayload)
	payloadSHA := hex.EncodeToString(payloadHash[:])
	canonicalAsset, err := json.Marshal(event.Asset)
	if err != nil {
		return fmt.Errorf("marshal asset projection payload: %w", err)
	}

	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin asset projection accept: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		fmt.Sprintf("%d:%s:%s:%d", len(event.TenantID), event.TenantID, event.AssetID, event.AggregateVersion),
	); err != nil {
		return fmt.Errorf("lock asset projection aggregate: %w", err)
	}

	var authoritativeTenant, authoritativeAsset, authoritativeTrace string
	var authoritativeRevision int64
	var authoritativePayload []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT tenant_id,asset_id::text,revision,trace_id,new_value::text
		FROM asset_events
		WHERE event_uuid=$1`,
		event.EventID,
	).Scan(
		&authoritativeTenant, &authoritativeAsset, &authoritativeRevision,
		&authoritativeTrace, &authoritativePayload,
	); err != nil {
		return fmt.Errorf("read authoritative asset history: %w", err)
	}
	canonicalAuthoritative, err := canonicalJSON(authoritativePayload)
	if err != nil {
		return fmt.Errorf("decode authoritative asset history: %w", err)
	}
	canonicalEventAsset, err := canonicalJSON(canonicalAsset)
	if err != nil {
		return fmt.Errorf("decode asset event payload: %w", err)
	}
	if authoritativeTenant != event.TenantID ||
		authoritativeAsset != event.AssetID ||
		authoritativeRevision != event.AggregateVersion ||
		authoritativeTrace != event.TraceID ||
		string(canonicalAuthoritative) != string(canonicalEventAsset) {
		return fmt.Errorf("asset projection event does not match authoritative history")
	}

	var existingEventID, existingHash string
	err = tx.QueryRowContext(ctx, `
		SELECT event_id::text,payload_sha256
		FROM asset_projection_inbox
		WHERE event_id=$1
		   OR (tenant_id=$2 AND asset_id=$3 AND aggregate_version=$4)
		FOR UPDATE`,
		event.EventID, event.TenantID, event.AssetID, event.AggregateVersion,
	).Scan(&existingEventID, &existingHash)
	if err == nil {
		if existingEventID != event.EventID || existingHash != payloadSHA {
			return fmt.Errorf("asset projection event identity collision")
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit asset projection replay: %w", err)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("read asset projection replay: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_projection_inbox (
		  event_id,tenant_id,asset_id,aggregate_version,schema_version,
		  partition_key,trace_id,payload,payload_sha256,kafka_partition,kafka_offset
		) VALUES ($1,$2,$3,$4,2,$5,$6,$7,$8,$9,$10)`,
		event.EventID, event.TenantID, event.AssetID, event.AggregateVersion,
		event.PartitionKey, event.TraceID, rawPayload, payloadSHA, partition, offset,
	); err != nil {
		return fmt.Errorf("insert asset projection inbox: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit asset projection inbox: %w", err)
	}
	return nil
}

func validateAssetProjectionEvent(event AssetUpsertedV2) error {
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("asset event_id must be UUID")
	}
	if _, err := uuid.Parse(event.AssetID); err != nil {
		return fmt.Errorf("asset asset_id must be UUID")
	}
	if event.EventType != "traffic.asset.v2.AssetUpserted" ||
		event.SchemaVersion != 2 ||
		event.AggregateVersion <= 0 ||
		event.Revision != event.AggregateVersion ||
		event.Asset.Revision != event.AggregateVersion ||
		event.TenantID == "" ||
		event.Asset.TenantID != event.TenantID ||
		event.Asset.AssetID != event.AssetID ||
		event.PartitionKey != event.TenantID+":"+event.AssetID {
		return fmt.Errorf("invalid asset upserted v2 envelope")
	}
	return nil
}

func canonicalJSON(value []byte) ([]byte, error) {
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return json.Marshal(decoded)
}

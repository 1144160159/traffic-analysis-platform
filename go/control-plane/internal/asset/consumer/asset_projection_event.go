package consumer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/sourcequality"
)

var (
	ErrAssetProjectionInvalid  = errors.New("invalid asset projection event")
	ErrAssetProjectionEnvelope = errors.New("asset projection envelope mismatch")
	ErrAssetProjectionConflict = errors.New("asset projection identity conflict")
	ErrAssetProjectionLate     = errors.New("late asset projection event")
)

type AssetProjectionDisposition string

const (
	AssetProjectionAccepted  AssetProjectionDisposition = "accepted"
	AssetProjectionDuplicate AssetProjectionDisposition = "duplicate"
)

type SourceQualityRecorder interface {
	Record(context.Context, sourcequality.Receipt) error
}

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
	SourceTopic      string             `json:"-"`
	SourcePartition  int                `json:"-"`
	SourceOffset     int64              `json:"-"`
	SourceTimestamp  int64              `json:"-"`
	SourceSHA256     string             `json:"-"`
	RawPayload       []byte             `json:"-"`
}

type AssetProjectionEventConsumer struct {
	db              *sql.DB
	consumerGroup   string
	qualityRecorder SourceQualityRecorder
	clickHouseFacts bool
}

// SetClickHouseProjectionEnabled controls only the durable inbox target state.
// It must be set before the consumer goroutine starts.
func (c *AssetProjectionEventConsumer) SetClickHouseProjectionEnabled(enabled bool) {
	c.clickHouseFacts = enabled
}

func NewAssetProjectionEventConsumer(db *sql.DB) (*AssetProjectionEventConsumer, error) {
	if db == nil {
		return nil, fmt.Errorf("asset projection database is required")
	}
	return &AssetProjectionEventConsumer{db: db}, nil
}

func NewAssetProjectionEventConsumerWithQuality(
	db *sql.DB,
	consumerGroup string,
	recorder SourceQualityRecorder,
) (*AssetProjectionEventConsumer, error) {
	consumer, err := NewAssetProjectionEventConsumer(db)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(consumerGroup) == "" || recorder == nil {
		return nil, fmt.Errorf("asset source-quality group and recorder are required")
	}
	consumer.consumerGroup = strings.TrimSpace(consumerGroup)
	consumer.qualityRecorder = recorder
	return consumer, nil
}

func (c *AssetProjectionEventConsumer) Handle(ctx context.Context, message *kafkaCommon.ReceivedMessage) error {
	if message == nil {
		return kafkaCommon.Permanent(fmt.Errorf("%w: Kafka message is nil", ErrAssetProjectionEnvelope))
	}
	if message.Topic != "asset.events.v2" || len(message.DuplicateHeaderNames()) > 0 {
		return kafkaCommon.Permanent(fmt.Errorf(
			"%w: source topic or duplicate headers are invalid", ErrAssetProjectionEnvelope))
	}
	if message.Time.UnixMilli() <= 0 {
		return kafkaCommon.Permanent(fmt.Errorf(
			"%w: Kafka timestamp is required", ErrAssetProjectionEnvelope))
	}
	var event AssetUpsertedV2
	if err := json.Unmarshal(message.Value, &event); err != nil {
		return kafkaCommon.Permanent(fmt.Errorf(
			"%w: decode asset projection event: %v", ErrAssetProjectionInvalid, err))
	}
	if err := validateAssetProjectionHeaders(message, event); err != nil {
		return kafkaCommon.Permanent(fmt.Errorf("%w: %v", ErrAssetProjectionEnvelope, err))
	}
	disposition, err := c.AcceptClassifiedSource(
		ctx, event, message.Topic, message.Partition, message.Offset,
		message.Time.UnixMilli(), message.Value)
	if err != nil {
		return err
	}
	if c.qualityRecorder == nil {
		return nil
	}
	category := sourcequality.Accepted
	reasonCode := ""
	if disposition == AssetProjectionDuplicate {
		category = sourcequality.Duplicate
		reasonCode = "DUPLICATE_EVENT"
	}
	return c.recordQuality(ctx, message, event.TenantID, event.EventID, category, reasonCode)
}

// RecordDLQAcknowledgement is called only after canonical DLQ broker ACK and
// must succeed before the common consumer is allowed to commit the source offset.
func (c *AssetProjectionEventConsumer) RecordDLQAcknowledgement(
	ctx context.Context,
	message *kafkaCommon.ReceivedMessage,
	processingErr error,
) error {
	if c.qualityRecorder == nil {
		return fmt.Errorf("asset source-quality recorder is unavailable")
	}
	if message == nil || !kafkaCommon.IsPermanent(processingErr) {
		return fmt.Errorf("asset DLQ receipt requires a permanent source failure")
	}
	category := sourcequality.Invalid
	reasonCode := "INVALID_EVENT"
	if errors.Is(processingErr, ErrAssetProjectionEnvelope) {
		category = sourcequality.Rejected
		reasonCode = "ENVELOPE_MISMATCH"
	} else if errors.Is(processingErr, ErrAssetProjectionConflict) {
		category = sourcequality.Conflict
		reasonCode = "EVENT_ID_CONFLICT"
	} else if errors.Is(processingErr, ErrAssetProjectionLate) {
		category = sourcequality.Late
		reasonCode = "LATE_EVENT"
	}
	tenantID := strings.TrimSpace(message.TenantID())
	if tenantID == "" {
		tenantID = "unknown"
	}
	return c.recordQuality(
		ctx, message, tenantID, message.EventID(), category, reasonCode)
}

func (c *AssetProjectionEventConsumer) recordQuality(
	ctx context.Context,
	message *kafkaCommon.ReceivedMessage,
	tenantID string,
	eventID string,
	category sourcequality.Category,
	reasonCode string,
) error {
	observedAtMS := message.Time.UnixMilli()
	if observedAtMS <= 0 {
		observedAtMS = 1
	}
	receipt, err := sourcequality.Build(sourcequality.Input{
		TenantID:      tenantID,
		Rail:          sourcequality.RailAsset,
		ConsumerGroup: c.consumerGroup,
		Source: sourcequality.SourceTuple{
			Topic: message.Topic, Partition: message.Partition, Offset: message.Offset,
		},
		Category:     category,
		EventID:      eventID,
		SourceSHA256: sourcequality.HashSource(message.Value),
		WatermarkMS:  -1,
		ObservedAtMS: observedAtMS,
		ReasonCode:   reasonCode,
	})
	if err != nil {
		return fmt.Errorf("build asset source-quality receipt: %w", err)
	}
	if err := c.qualityRecorder.Record(ctx, receipt); err != nil {
		return fmt.Errorf("persist asset source-quality receipt: %w", err)
	}
	return nil
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
	_, err := c.AcceptClassified(ctx, event, partition, offset, rawPayload)
	return err
}

func (c *AssetProjectionEventConsumer) AcceptClassified(
	ctx context.Context,
	event AssetUpsertedV2,
	partition int,
	offset int64,
	rawPayload []byte,
) (AssetProjectionDisposition, error) {
	return c.AcceptClassifiedSource(
		ctx, event, "asset.events.v2", partition, offset,
		event.Asset.LastSeen.UnixMilli(), rawPayload)
}

func (c *AssetProjectionEventConsumer) AcceptClassifiedSource(
	ctx context.Context,
	event AssetUpsertedV2,
	topic string,
	partition int,
	offset int64,
	kafkaTimestampMS int64,
	rawPayload []byte,
) (AssetProjectionDisposition, error) {
	if err := validateAssetProjectionEvent(event); err != nil {
		return "", kafkaCommon.Permanent(fmt.Errorf("%w: %v", ErrAssetProjectionInvalid, err))
	}
	if topic != "asset.events.v2" || partition < 0 || offset < 0 || kafkaTimestampMS <= 0 {
		return "", kafkaCommon.Permanent(fmt.Errorf(
			"%w: invalid Kafka source coordinates", ErrAssetProjectionEnvelope))
	}
	canonicalPayload, err := canonicalJSON(rawPayload)
	if err != nil {
		return "", kafkaCommon.Permanent(fmt.Errorf(
			"%w: canonicalize asset projection event: %v", ErrAssetProjectionInvalid, err))
	}
	payloadHash := sha256.Sum256(canonicalPayload)
	payloadSHA := hex.EncodeToString(payloadHash[:])
	canonicalAsset, err := json.Marshal(event.Asset)
	if err != nil {
		return "", kafkaCommon.Permanent(fmt.Errorf(
			"%w: marshal asset projection payload: %v", ErrAssetProjectionInvalid, err))
	}

	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return "", fmt.Errorf("begin asset projection accept: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		fmt.Sprintf("%d:%s:%s", len(event.TenantID), event.TenantID, event.AssetID),
	); err != nil {
		return "", fmt.Errorf("lock asset projection aggregate: %w", err)
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
		return "", fmt.Errorf("read authoritative asset history: %w", err)
	}
	canonicalAuthoritative, err := canonicalJSON(authoritativePayload)
	if err != nil {
		return "", fmt.Errorf("decode authoritative asset history: %w", err)
	}
	canonicalEventAsset, err := canonicalJSON(canonicalAsset)
	if err != nil {
		return "", kafkaCommon.Permanent(fmt.Errorf(
			"%w: decode asset event payload: %v", ErrAssetProjectionInvalid, err))
	}
	if authoritativeTenant != event.TenantID ||
		authoritativeAsset != event.AssetID ||
		authoritativeRevision != event.AggregateVersion ||
		authoritativeTrace != event.TraceID ||
		string(canonicalAuthoritative) != string(canonicalEventAsset) {
		return "", kafkaCommon.Permanent(fmt.Errorf(
			"%w: event does not match authoritative history", ErrAssetProjectionConflict))
	}

	var existingEventID, existingHash string
	var existingPartition int
	var existingOffset int64
	err = tx.QueryRowContext(ctx, `
		SELECT event_id::text,payload_sha256,kafka_partition,kafka_offset
		FROM asset_projection_inbox
		WHERE event_id=$1
		   OR (tenant_id=$2 AND asset_id=$3 AND aggregate_version=$4)
		FOR UPDATE`,
		event.EventID, event.TenantID, event.AssetID, event.AggregateVersion,
	).Scan(&existingEventID, &existingHash, &existingPartition, &existingOffset)
	if err == nil {
		if existingEventID != event.EventID || existingHash != payloadSHA {
			return "", kafkaCommon.Permanent(fmt.Errorf(
				"%w: event_id or payload collision", ErrAssetProjectionConflict))
		}
		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("commit asset projection replay: %w", err)
		}
		if existingPartition == partition && existingOffset == offset {
			return AssetProjectionAccepted, nil
		}
		return AssetProjectionDuplicate, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("read asset projection replay: %w", err)
	}

	var maximumVersion int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(aggregate_version),0)
		FROM asset_projection_inbox
		WHERE tenant_id=$1 AND asset_id=$2`,
		event.TenantID, event.AssetID,
	).Scan(&maximumVersion); err != nil {
		return "", fmt.Errorf("read asset projection maximum version: %w", err)
	}
	if event.AggregateVersion < maximumVersion {
		return "", kafkaCommon.Permanent(fmt.Errorf(
			"%w: aggregate_version=%d maximum=%d",
			ErrAssetProjectionLate, event.AggregateVersion, maximumVersion))
	}

	chStatus := "disabled"
	if c.clickHouseFacts {
		chStatus = "pending"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_projection_inbox (
		  event_id,tenant_id,asset_id,aggregate_version,schema_version,
		  partition_key,trace_id,payload,payload_sha256,kafka_partition,kafka_offset,
		  kafka_topic,kafka_timestamp_ms,raw_payload,source_sha256,ch_status
		) VALUES ($1,$2,$3,$4,2,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		event.EventID, event.TenantID, event.AssetID, event.AggregateVersion,
		event.PartitionKey, event.TraceID, rawPayload, payloadSHA, partition, offset,
		topic, kafkaTimestampMS, rawPayload, sourcequality.HashSource(rawPayload), chStatus,
	); err != nil {
		return "", fmt.Errorf("insert asset projection inbox: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit asset projection inbox: %w", err)
	}
	return AssetProjectionAccepted, nil
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

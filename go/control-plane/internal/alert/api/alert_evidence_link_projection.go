package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

type AlertEvidenceLinkProjectionInput struct {
	EventID          string
	EventType        string
	TenantID         string
	AggregateID      string
	AggregateVersion int64
	PartitionKey     string
	AlertID          string
	EvidenceID       string
	EvidenceType     string
	Status           string
	SourceStore      string
	ObjectBucket     string
	ObjectKey        string
	ObjectVersion    string
	ObjectSHA256     string
	SizeBytes        int64
	ContentType      string
	ManifestRevision int64
	Reason           string
	TraceID          string
	OccurredAt       time.Time
	Payload          map[string]interface{}
	KafkaTopic       string
	KafkaPartition   int
	KafkaOffset      int64
	ReceivedAt       time.Time
}

type AlertEvidenceLinkProjectionApplier struct {
	postgres   *sql.DB
	clickhouse *storage.ClickHouseClient
}

func NewAlertEvidenceLinkProjectionApplier(
	postgres *sql.DB, clickhouse *storage.ClickHouseClient,
) *AlertEvidenceLinkProjectionApplier {
	if postgres == nil || clickhouse == nil {
		return nil
	}
	return &AlertEvidenceLinkProjectionApplier{postgres: postgres, clickhouse: clickhouse}
}

func (a *AlertEvidenceLinkProjectionApplier) VerifySchema(ctx context.Context) error {
	if a == nil || a.postgres == nil || a.clickhouse == nil {
		return fmt.Errorf("alert evidence link projection stores are unavailable")
	}
	tx, err := a.postgres.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := verifyAlertEvidenceLinkSchemaTx(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	row, err := a.clickhouse.QueryRow(ctx, `SELECT count() FROM system.tables
		WHERE database='traffic' AND name IN ('alert_evidence_links_v1_local','alert_evidence_links_v1')`)
	if err != nil {
		return err
	}
	var tableCount uint64
	if err := row.Scan(&tableCount); err != nil {
		return err
	}
	if tableCount != 2 {
		return fmt.Errorf("alert evidence link ClickHouse projection schema is incomplete")
	}
	return nil
}

func (a *AlertEvidenceLinkProjectionApplier) Apply(
	ctx context.Context, input AlertEvidenceLinkProjectionInput,
) error {
	if a == nil || a.postgres == nil || a.clickhouse == nil {
		return fmt.Errorf("alert evidence link projection stores are unavailable")
	}
	if err := input.Validate(); err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(input.Payload)
	if err != nil {
		return fmt.Errorf("marshal alert evidence link projection: %w", err)
	}
	digest := sha256.Sum256(payloadJSON)
	payloadSHA := hex.EncodeToString(digest[:])
	if input.ReceivedAt.IsZero() {
		input.ReceivedAt = time.Now().UTC()
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = input.ReceivedAt
	}
	tx, err := a.postgres.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin alert evidence projection inbox: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO alert_evidence_link_projection_inbox
		(event_id,tenant_id,aggregate_id,aggregate_version,event_type,partition_key,payload_sha256,payload,
		 first_kafka_topic,first_kafka_partition,first_kafka_offset,received_at)
		VALUES ($1::uuid,$2,$3::uuid,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12)
		ON CONFLICT DO NOTHING`, input.EventID, input.TenantID, input.AggregateID, input.AggregateVersion,
		input.EventType, input.PartitionKey, payloadSHA, string(payloadJSON), input.KafkaTopic,
		input.KafkaPartition, input.KafkaOffset, input.ReceivedAt)
	if err != nil {
		return fmt.Errorf("insert alert evidence projection inbox: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		var exact bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM alert_evidence_link_projection_inbox
			WHERE event_id=$1::uuid AND tenant_id=$2 AND aggregate_id=$3::uuid
			  AND aggregate_version=$4 AND event_type=$5 AND partition_key=$6
			  AND payload_sha256=$7 AND payload=$8::jsonb
		)`, input.EventID, input.TenantID, input.AggregateID, input.AggregateVersion,
			input.EventType, input.PartitionKey, payloadSHA, string(payloadJSON)).Scan(&exact)
		if err != nil {
			return fmt.Errorf("verify duplicate alert evidence projection: %w", err)
		}
		if !exact {
			return fmt.Errorf("alert evidence event identity or aggregate revision collision")
		}
	}
	delivery, err := tx.ExecContext(ctx, `INSERT INTO alert_evidence_link_projection_deliveries
		(kafka_topic,kafka_partition,kafka_offset,event_id,received_at)
		VALUES ($1,$2,$3,$4::uuid,$5) ON CONFLICT DO NOTHING`, input.KafkaTopic,
		input.KafkaPartition, input.KafkaOffset, input.EventID, input.ReceivedAt)
	if err != nil {
		return fmt.Errorf("insert alert evidence projection delivery: %w", err)
	}
	deliveryInserted, err := delivery.RowsAffected()
	if err != nil {
		return err
	}
	if deliveryInserted == 0 {
		var exact bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM alert_evidence_link_projection_deliveries
			WHERE kafka_topic=$1 AND kafka_partition=$2 AND kafka_offset=$3 AND event_id=$4::uuid
		)`, input.KafkaTopic, input.KafkaPartition, input.KafkaOffset, input.EventID).Scan(&exact)
		if err != nil {
			return fmt.Errorf("verify duplicate alert evidence delivery: %w", err)
		}
		if !exact {
			return fmt.Errorf("alert evidence Kafka position collision at %s/%d/%d", input.KafkaTopic, input.KafkaPartition, input.KafkaOffset)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE alert_evidence_link_projection_inbox
		SET projection_attempts=projection_attempts+1,last_error=''
		WHERE event_id=$1::uuid`, input.EventID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert evidence projection inbox: %w", err)
	}

	err = a.clickhouse.Exec(ctx, `INSERT INTO traffic.alert_evidence_links_v1_local
		(tenant_id,relation_id,alert_id,evidence_id,evidence_type,status,relation_revision,event_id,
		 event_type,schema_version,source_store,object_bucket,object_key,object_version,object_sha256,
		 size_bytes,content_type,manifest_revision,reason,trace_id,source_topic,source_partition,
		 source_offset,source_timestamp,payload_sha256,payload_json,projected_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		input.TenantID, input.AggregateID, input.AlertID, input.EvidenceID, input.EvidenceType,
		input.Status, uint64(input.AggregateVersion), input.EventID, input.EventType, uint16(1),
		input.SourceStore, input.ObjectBucket, input.ObjectKey, input.ObjectVersion, input.ObjectSHA256,
		uint64(input.SizeBytes), input.ContentType, uint64(input.ManifestRevision), input.Reason,
		input.TraceID, input.KafkaTopic, int32(input.KafkaPartition), input.KafkaOffset,
		input.ReceivedAt.UTC(), payloadSHA, string(payloadJSON), time.Now().UTC())
	if err != nil {
		_, _ = a.postgres.ExecContext(ctx, `UPDATE alert_evidence_link_projection_inbox
			SET last_error=$2 WHERE event_id=$1::uuid`, input.EventID, truncateEvidenceOutboxError(err.Error()))
		return fmt.Errorf("write alert evidence ClickHouse projection: %w", err)
	}

	tx, err = a.postgres.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE alert_evidence_link_projection_inbox
		SET projection_status='projected',projected_at=now(),last_error=''
		WHERE event_id=$1::uuid`, input.EventID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_evidence_link_projection_watermarks
		(kafka_topic,kafka_partition,last_offset,last_event_id)
		VALUES ($1,$2,$3,$4::uuid)
		ON CONFLICT (kafka_topic,kafka_partition) DO UPDATE SET
		 last_offset=EXCLUDED.last_offset,last_event_id=EXCLUDED.last_event_id,updated_at=now()
		WHERE EXCLUDED.last_offset>alert_evidence_link_projection_watermarks.last_offset`,
		input.KafkaTopic, input.KafkaPartition, input.KafkaOffset, input.EventID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (input AlertEvidenceLinkProjectionInput) Validate() error {
	if strings.TrimSpace(input.EventID) == "" || strings.TrimSpace(input.TenantID) == "" ||
		strings.TrimSpace(input.AggregateID) == "" || input.AggregateVersion < 1 ||
		strings.TrimSpace(input.AlertID) == "" || strings.TrimSpace(input.EvidenceID) == "" ||
		input.SizeBytes < 0 || input.ManifestRevision < 1 {
		return fmt.Errorf("incomplete alert evidence link projection input")
	}
	return nil
}

package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// PlaybookExecutionEventProjectionInput is the strict Kafka-to-PostgreSQL
// projection boundary. Kafka coordinates are part of the durable identity so
// duplicate delivery is harmless while payload or offset collisions fail.
type PlaybookExecutionEventProjectionInput struct {
	EventID          string
	TenantID         string
	ExecutionID      string
	PlaybookName     string
	PlaybookVersion  int
	AlertID          string
	EventType        string
	Status           string
	ApprovalStatus   string
	ExecutorStatus   string
	SchemaVersion    int
	AggregateVersion int64
	PartitionKey     string
	TraceID          string
	Payload          map[string]interface{}
	KafkaTopic       string
	KafkaPartition   int
	KafkaOffset      int64
}

func (h *AdvancedHandler) ApplyPlaybookExecutionEventProjection(ctx context.Context, input PlaybookExecutionEventProjectionInput) error {
	if h.advancedRepo == nil || h.advancedRepo.db == nil {
		return fmt.Errorf("playbook execution projection database is unavailable")
	}
	canonical, err := json.Marshal(input.Payload)
	if err != nil {
		return fmt.Errorf("marshal playbook execution projection: %w", err)
	}
	digest := sha256.Sum256(canonical)
	payloadSHA := hex.EncodeToString(digest[:])
	tx, err := h.advancedRepo.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var authorityRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT workflow_revision FROM alert_playbook_executions
		WHERE tenant_id=$1 AND execution_id=$2`, input.TenantID, input.ExecutionID).Scan(&authorityRevision); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("playbook execution authority is missing")
		}
		return err
	}
	if authorityRevision < input.AggregateVersion {
		return fmt.Errorf("playbook execution event is ahead of PostgreSQL authority")
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO alert_playbook_execution_event_projection
		(event_id,tenant_id,execution_id,playbook_name,event_type,schema_version,aggregate_version,
		 partition_key,trace_id,payload,payload_sha256,kafka_topic,kafka_partition,kafka_offset)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,$13,$14)
		ON CONFLICT (event_id) DO NOTHING`, input.EventID, input.TenantID, input.ExecutionID,
		input.PlaybookName, input.EventType, input.SchemaVersion, input.AggregateVersion,
		input.PartitionKey, input.TraceID, string(canonical), payloadSHA, input.KafkaTopic,
		input.KafkaPartition, input.KafkaOffset)
	if err != nil {
		return fmt.Errorf("insert playbook execution event projection: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		var existingSHA, existingTopic, existingTenant, existingExecution string
		var existingPartition int
		var existingOffset, existingVersion int64
		if err := tx.QueryRowContext(ctx, `SELECT payload_sha256,kafka_topic,kafka_partition,kafka_offset,
			tenant_id,execution_id,aggregate_version FROM alert_playbook_execution_event_projection
			WHERE event_id=$1::uuid`, input.EventID).Scan(&existingSHA, &existingTopic, &existingPartition,
			&existingOffset, &existingTenant, &existingExecution, &existingVersion); err != nil {
			return err
		}
		if existingSHA != payloadSHA || existingTopic != input.KafkaTopic ||
			existingPartition != input.KafkaPartition || existingOffset != input.KafkaOffset ||
			existingTenant != input.TenantID || existingExecution != input.ExecutionID ||
			existingVersion != input.AggregateVersion {
			return fmt.Errorf("playbook execution event replay identity collision")
		}
		return tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO alert_playbook_execution_state_projection
		(tenant_id,execution_id,playbook_name,playbook_version,alert_id,status,approval_status,
		 executor_status,aggregate_version,event_type,trace_id,last_event_id,payload,payload_sha256,
		 kafka_topic,kafka_partition,kafka_offset)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::uuid,$13::jsonb,$14,$15,$16,$17)
		ON CONFLICT (tenant_id,execution_id) DO UPDATE SET
		 playbook_name=EXCLUDED.playbook_name,
		 playbook_version=CASE WHEN EXCLUDED.playbook_version>0 THEN EXCLUDED.playbook_version ELSE alert_playbook_execution_state_projection.playbook_version END,
		 alert_id=CASE WHEN EXCLUDED.alert_id<>'' THEN EXCLUDED.alert_id ELSE alert_playbook_execution_state_projection.alert_id END,
		 status=EXCLUDED.status,
		 approval_status=CASE WHEN EXCLUDED.approval_status<>'' THEN EXCLUDED.approval_status ELSE alert_playbook_execution_state_projection.approval_status END,
		 executor_status=CASE WHEN EXCLUDED.executor_status<>'' THEN EXCLUDED.executor_status ELSE alert_playbook_execution_state_projection.executor_status END,
		 aggregate_version=EXCLUDED.aggregate_version,event_type=EXCLUDED.event_type,trace_id=EXCLUDED.trace_id,
		 last_event_id=EXCLUDED.last_event_id,payload=EXCLUDED.payload,payload_sha256=EXCLUDED.payload_sha256,
		 kafka_topic=EXCLUDED.kafka_topic,kafka_partition=EXCLUDED.kafka_partition,kafka_offset=EXCLUDED.kafka_offset,updated_at=now()
		WHERE alert_playbook_execution_state_projection.aggregate_version < EXCLUDED.aggregate_version`,
		input.TenantID, input.ExecutionID, input.PlaybookName, input.PlaybookVersion, input.AlertID,
		input.Status, input.ApprovalStatus, input.ExecutorStatus, input.AggregateVersion, input.EventType,
		input.TraceID, input.EventID, string(canonical), payloadSHA, input.KafkaTopic,
		input.KafkaPartition, input.KafkaOffset)
	if err != nil {
		return fmt.Errorf("upsert playbook execution state projection: %w", err)
	}
	return tx.Commit()
}

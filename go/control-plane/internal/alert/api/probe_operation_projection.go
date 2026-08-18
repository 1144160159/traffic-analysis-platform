package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var ErrProbeOperationProjectionConflict = fmt.Errorf("probe operation event semantic identity conflict")

type ProbeOperationProjectionInput struct {
	EventID        string
	EventType      string
	TenantID       string
	ProbeID        string
	OperationID    string
	Revision       int64
	Status         string
	TraceID        string
	Payload        map[string]interface{}
	KafkaPartition int
	KafkaOffset    int64
}

// ApplyProbeOperationProjection records the immutable lifecycle event and
// advances the latest projection only for a newer revision. Kafka coordinates
// are receipt facts, not semantic event identity: an exact event replay at a
// different offset is idempotent and preserves the first stored coordinate.
func (h *SystemHandler) ApplyProbeOperationProjection(
	ctx context.Context,
	input ProbeOperationProjectionInput,
) error {
	if h.pgDB == nil {
		return fmt.Errorf("probe operation projection database is unavailable")
	}
	if err := validateProbeOperationProjectionInput(input); err != nil {
		return err
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return fmt.Errorf("marshal probe operation projection payload: %w", err)
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin probe operation projection: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO probe_operation_event_projection
			(event_id,operation_id,tenant_id,probe_id,event_type,revision,status,
			 trace_id,payload,kafka_partition,kafka_offset)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11)
		ON CONFLICT DO NOTHING`,
		input.EventID, input.OperationID, input.TenantID, input.ProbeID,
		input.EventType, input.Revision, input.Status, input.TraceID,
		string(payload), input.KafkaPartition, input.KafkaOffset,
	)
	if err != nil {
		return fmt.Errorf("insert probe operation event projection: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect probe operation event insert: %w", err)
	}
	if inserted == 0 {
		var (
			existingOperationID string
			existingTenantID    string
			existingProbeID     string
			existingEventType   string
			existingRevision    int64
			existingStatus      string
			existingTraceID     string
			existingPayload     []byte
		)
		err = tx.QueryRowContext(ctx, `
			SELECT operation_id::text,tenant_id,probe_id,event_type,revision,status,
			       trace_id,payload
			FROM probe_operation_event_projection
			WHERE event_id=$1::uuid`, input.EventID,
		).Scan(
			&existingOperationID,
			&existingTenantID,
			&existingProbeID,
			&existingEventType,
			&existingRevision,
			&existingStatus,
			&existingTraceID,
			&existingPayload,
		)
		if err != nil {
			return fmt.Errorf("verify duplicate probe operation event: %w", err)
		}
		if existingOperationID != input.OperationID || existingTenantID != input.TenantID ||
			existingProbeID != input.ProbeID || existingEventType != input.EventType ||
			existingRevision != input.Revision || existingStatus != input.Status ||
			existingTraceID != input.TraceID || !jsonSemanticEqual(existingPayload, payload) {
			return ErrProbeOperationProjectionConflict
		}
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO probe_operation_state_projection
			(tenant_id,operation_id,probe_id,revision,event_type,status,trace_id,
			 last_event_id,payload,kafka_partition,kafka_offset)
		VALUES ($1,$2::uuid,$3,$4,$5,$6,$7,$8::uuid,$9::jsonb,$10,$11)
		ON CONFLICT (tenant_id,operation_id) DO UPDATE SET
			probe_id=EXCLUDED.probe_id,
			revision=EXCLUDED.revision,
			event_type=EXCLUDED.event_type,
			status=EXCLUDED.status,
			trace_id=EXCLUDED.trace_id,
			last_event_id=EXCLUDED.last_event_id,
			payload=EXCLUDED.payload,
			kafka_partition=EXCLUDED.kafka_partition,
			kafka_offset=EXCLUDED.kafka_offset,
			updated_at=now()
		WHERE EXCLUDED.revision > probe_operation_state_projection.revision`,
		input.TenantID, input.OperationID, input.ProbeID, input.Revision,
		input.EventType, input.Status, input.TraceID, input.EventID,
		string(payload), input.KafkaPartition, input.KafkaOffset,
	); err != nil {
		return fmt.Errorf("upsert probe operation state projection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit probe operation projection: %w", err)
	}
	return nil
}

func validateProbeOperationProjectionInput(input ProbeOperationProjectionInput) error {
	if _, err := uuid.Parse(input.EventID); err != nil {
		return fmt.Errorf("invalid probe operation projection event_id")
	}
	if _, err := uuid.Parse(input.OperationID); err != nil {
		return fmt.Errorf("invalid probe operation projection operation_id")
	}
	switch input.EventType {
	case probeOperationAcknowledgedEvent, probeOperationExpiredEvent:
	default:
		return fmt.Errorf("unsupported probe operation projection event_type")
	}
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.ProbeID) == "" ||
		strings.TrimSpace(input.EventType) == "" || input.Revision <= 0 ||
		strings.TrimSpace(input.Status) == "" || strings.TrimSpace(input.TraceID) == "" ||
		input.Payload == nil || input.KafkaPartition < 0 || input.KafkaOffset < 0 {
		return fmt.Errorf("incomplete probe operation projection input")
	}
	return nil
}

func jsonSemanticEqual(left, right []byte) bool {
	var leftValue interface{}
	var rightValue interface{}
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	leftCanonical, err := json.Marshal(leftValue)
	if err != nil {
		return false
	}
	rightCanonical, err := json.Marshal(rightValue)
	if err != nil {
		return false
	}
	return sha256.Sum256(leftCanonical) == sha256.Sum256(rightCanonical)
}

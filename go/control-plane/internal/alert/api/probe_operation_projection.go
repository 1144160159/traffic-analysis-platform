package api

import (
	"context"
	"encoding/json"
	"fmt"
)

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
// advances the latest projection only for a newer revision. Duplicate events
// are accepted only when their identity, payload and Kafka location match.
func (h *SystemHandler) ApplyProbeOperationProjection(
	ctx context.Context,
	input ProbeOperationProjectionInput,
) error {
	if h.pgDB == nil {
		return fmt.Errorf("probe operation projection database is unavailable")
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
		var exactDuplicate bool
		err = tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM probe_operation_event_projection
				WHERE event_id=$1::uuid AND operation_id=$2::uuid AND tenant_id=$3
				  AND probe_id=$4 AND event_type=$5 AND revision=$6 AND status=$7
				  AND trace_id=$8 AND payload=$9::jsonb
				  AND kafka_partition=$10 AND kafka_offset=$11
			)`,
			input.EventID, input.OperationID, input.TenantID, input.ProbeID,
			input.EventType, input.Revision, input.Status, input.TraceID,
			string(payload), input.KafkaPartition, input.KafkaOffset,
		).Scan(&exactDuplicate)
		if err != nil {
			return fmt.Errorf("verify duplicate probe operation event: %w", err)
		}
		if !exactDuplicate {
			return fmt.Errorf("probe operation event identity or Kafka offset collision")
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

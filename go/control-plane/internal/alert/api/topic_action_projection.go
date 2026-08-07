package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// TopicActionProjectionInput is the validated Kafka lifecycle envelope handed
// to the PostgreSQL projection transaction. Payload is retained so the read
// model can be rebuilt and reconciled without consulting frontend state.
type TopicActionProjectionInput struct {
	EventID        string
	EventType      string
	TenantID       string
	Topic          string
	JobID          string
	ActionID       string
	Revision       int64
	Status         string
	TraceID        string
	Payload        map[string]interface{}
	KafkaPartition int
	KafkaOffset    int64
}

// ApplyTopicActionProjection commits the immutable event and the latest
// per-job projection in one transaction. Duplicate event IDs are accepted only
// when their complete identity and payload match; revision ordering prevents a
// late ActionRequested message from overwriting a newer ActionResult.
func (h *SystemHandler) ApplyTopicActionProjection(
	ctx context.Context,
	input TopicActionProjectionInput,
) error {
	if h.pgDB == nil {
		return fmt.Errorf("topic action projection database is unavailable")
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return fmt.Errorf("marshal topic action projection payload: %w", err)
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin topic action projection: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO topic_action_event_projection
			(event_id,job_id,tenant_id,topic,event_type,revision,action_id,status,
			 trace_id,payload,kafka_partition,kafka_offset)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12)
		ON CONFLICT DO NOTHING`,
		input.EventID, input.JobID, input.TenantID, input.Topic, input.EventType,
		input.Revision, input.ActionID, input.Status, input.TraceID, string(payload),
		input.KafkaPartition, input.KafkaOffset,
	)
	if err != nil {
		return fmt.Errorf("insert topic action event projection: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect topic action event projection insert: %w", err)
	}
	if inserted == 0 {
		var exactDuplicate bool
		err = tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM topic_action_event_projection
				WHERE event_id=$1::uuid AND job_id=$2::uuid AND tenant_id=$3
				  AND topic=$4 AND event_type=$5 AND revision=$6
				  AND action_id=$7 AND status=$8 AND trace_id=$9
				  AND payload=$10::jsonb AND kafka_partition=$11 AND kafka_offset=$12
			)`,
			input.EventID, input.JobID, input.TenantID, input.Topic, input.EventType,
			input.Revision, input.ActionID, input.Status, input.TraceID, string(payload),
			input.KafkaPartition, input.KafkaOffset,
		).Scan(&exactDuplicate)
		if err != nil {
			return fmt.Errorf("verify duplicate topic action event: %w", err)
		}
		if !exactDuplicate {
			return fmt.Errorf("topic action event identity or Kafka offset collision")
		}
		return tx.Commit()
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO topic_action_job_projection
			(tenant_id,job_id,topic,revision,event_type,action_id,status,trace_id,
			 last_event_id,payload,kafka_partition,kafka_offset)
		VALUES ($1,$2::uuid,$3,$4,$5,$6,$7,$8,$9::uuid,$10::jsonb,$11,$12)
		ON CONFLICT (tenant_id,job_id) DO UPDATE SET
			topic=EXCLUDED.topic,
			revision=EXCLUDED.revision,
			event_type=EXCLUDED.event_type,
			action_id=EXCLUDED.action_id,
			status=EXCLUDED.status,
			trace_id=EXCLUDED.trace_id,
			last_event_id=EXCLUDED.last_event_id,
			payload=EXCLUDED.payload,
			kafka_partition=EXCLUDED.kafka_partition,
			kafka_offset=EXCLUDED.kafka_offset,
			updated_at=now()
		WHERE EXCLUDED.revision > topic_action_job_projection.revision`,
		input.TenantID, input.JobID, input.Topic, input.Revision, input.EventType,
		input.ActionID, input.Status, input.TraceID, input.EventID, string(payload),
		input.KafkaPartition, input.KafkaOffset,
	); err != nil {
		return fmt.Errorf("upsert topic action job projection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit topic action projection: %w", err)
	}
	return nil
}

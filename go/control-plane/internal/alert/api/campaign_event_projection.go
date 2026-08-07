package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// CampaignEventProjectionInput is the validated Kafka envelope at the durable
// PostgreSQL boundary. Downstream CH/OS/Nebula work remains explicitly pending
// in target_status until those projectors acknowledge their own effects.
type CampaignEventProjectionInput struct {
	Stream            string
	EventID           string
	TenantID          string
	AggregateID       string
	CampaignID        string
	RelationID        string
	AlertID           string
	EventType         string
	SchemaVersion     int
	AggregateRevision int64
	RelationRevision  int64
	PartitionKey      string
	TraceID           string
	Payload           map[string]interface{}
	KafkaTopic        string
	KafkaPartition    int
	KafkaOffset       int64
	ReceivedAt        time.Time
}

// ApplyCampaignEventProjection records event identity, every Kafka delivery
// and the partition watermark atomically. Duplicate delivery is accepted only
// when both the stable event and Kafka position retain their original meaning.
func (h *SystemHandler) ApplyCampaignEventProjection(ctx context.Context, input CampaignEventProjectionInput) error {
	if h.pgDB == nil {
		return fmt.Errorf("campaign event projection database is unavailable")
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return fmt.Errorf("marshal campaign event projection payload: %w", err)
	}
	if input.ReceivedAt.IsZero() {
		input.ReceivedAt = time.Now().UTC()
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin campaign event projection: %w", err)
	}
	defer tx.Rollback()
	var relationID interface{}
	if input.RelationID != "" {
		relationID = input.RelationID
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO campaign_event_projection_inbox
		(stream,event_id,tenant_id,aggregate_id,campaign_id,relation_id,alert_id,event_type,
		 schema_version,aggregate_revision,relation_revision,partition_key,trace_id,payload,
		 first_kafka_topic,first_kafka_partition,first_kafka_offset,received_at)
		VALUES ($1,$2::uuid,$3,$4,$5,$6::uuid,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15,$16,$17,$18)
		ON CONFLICT DO NOTHING`, input.Stream, input.EventID, input.TenantID, input.AggregateID,
		input.CampaignID, relationID, input.AlertID, input.EventType, input.SchemaVersion,
		input.AggregateRevision, input.RelationRevision, input.PartitionKey, input.TraceID,
		string(payload), input.KafkaTopic, input.KafkaPartition, input.KafkaOffset, input.ReceivedAt)
	if err != nil {
		return fmt.Errorf("insert campaign event inbox: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect campaign event inbox insert: %w", err)
	}
	if inserted == 0 {
		var exact bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM campaign_event_projection_inbox
			WHERE stream=$1 AND event_id=$2::uuid AND tenant_id=$3 AND aggregate_id=$4
			  AND campaign_id=$5 AND relation_id IS NOT DISTINCT FROM $6::uuid AND alert_id=$7
			  AND event_type=$8 AND schema_version=$9 AND aggregate_revision=$10
			  AND relation_revision=$11 AND partition_key=$12 AND trace_id=$13 AND payload=$14::jsonb
		)`, input.Stream, input.EventID, input.TenantID, input.AggregateID, input.CampaignID,
			relationID, input.AlertID, input.EventType, input.SchemaVersion, input.AggregateRevision,
			input.RelationRevision, input.PartitionKey, input.TraceID, string(payload)).Scan(&exact)
		if err != nil {
			return fmt.Errorf("verify duplicate campaign event: %w", err)
		}
		if !exact {
			return fmt.Errorf("campaign event identity collision for %s/%s", input.Stream, input.EventID)
		}
	}
	delivery, err := tx.ExecContext(ctx, `
		INSERT INTO campaign_event_projection_deliveries
		(kafka_topic,kafka_partition,kafka_offset,stream,event_id,received_at)
		VALUES ($1,$2,$3,$4,$5::uuid,$6) ON CONFLICT DO NOTHING`, input.KafkaTopic,
		input.KafkaPartition, input.KafkaOffset, input.Stream, input.EventID, input.ReceivedAt)
	if err != nil {
		return fmt.Errorf("insert campaign event delivery: %w", err)
	}
	deliveryInserted, err := delivery.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect campaign event delivery: %w", err)
	}
	if deliveryInserted == 0 {
		var exact bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM campaign_event_projection_deliveries
			WHERE kafka_topic=$1 AND kafka_partition=$2 AND kafka_offset=$3
			  AND stream=$4 AND event_id=$5::uuid
		)`, input.KafkaTopic, input.KafkaPartition, input.KafkaOffset,
			input.Stream, input.EventID).Scan(&exact)
		if err != nil {
			return fmt.Errorf("verify campaign event delivery collision: %w", err)
		}
		if !exact {
			return fmt.Errorf("campaign Kafka position collision at %s/%d/%d", input.KafkaTopic, input.KafkaPartition, input.KafkaOffset)
		}
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO campaign_event_projection_watermarks
		(kafka_topic,kafka_partition,last_offset,last_event_id,last_stream,last_received_at)
		VALUES ($1,$2,$3,$4::uuid,$5,$6)
		ON CONFLICT (kafka_topic,kafka_partition) DO UPDATE SET
		  last_offset=EXCLUDED.last_offset,last_event_id=EXCLUDED.last_event_id,
		  last_stream=EXCLUDED.last_stream,last_received_at=EXCLUDED.last_received_at,updated_at=now()
		WHERE EXCLUDED.last_offset > campaign_event_projection_watermarks.last_offset`,
		input.KafkaTopic, input.KafkaPartition, input.KafkaOffset, input.EventID, input.Stream, input.ReceivedAt); err != nil {
		return fmt.Errorf("advance campaign event watermark: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit campaign event projection: %w", err)
	}
	return nil
}

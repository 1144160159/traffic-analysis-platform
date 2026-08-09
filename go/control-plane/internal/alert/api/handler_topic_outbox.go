package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const topicActionKafkaTopic = "traffic.topic.action.v2"

type topicActionOutboxItem struct {
	EventID          string
	JobID            string
	TenantID         string
	EventType        string
	PartitionKey     string
	AggregateVersion int64
	SchemaVersion    int
	Payload          []byte
}

// StartTopicActionOutboxWorker publishes committed topic action events with a
// lease and bounded retry. A row is marked published only after required-acks
// Kafka delivery returns successfully.
func (h *SystemHandler) StartTopicActionOutboxWorker(ctx context.Context, interval time.Duration) error {
	if h.pgDB == nil {
		return fmt.Errorf("topic action outbox database is unavailable")
	}
	if h.topicActionPublish == nil {
		return fmt.Errorf("topic action Kafka publisher is unavailable")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := hostnameOrDefault() + ":topic-outbox:" + uuid.NewString()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := h.drainTopicActionOutbox(ctx, workerID, 50); err != nil && ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Failed to drain topic action outbox", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (h *SystemHandler) drainTopicActionOutbox(ctx context.Context, workerID string, limit int) (int, error) {
	if h.pgDB == nil || h.topicActionPublish == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := h.pgDB.QueryContext(ctx, `
		WITH candidates AS (
			SELECT event_id FROM topic_action_outbox
			WHERE published=false AND next_attempt_at <= now()
			  AND event_type IN ('traffic.topic.v2.ActionRequested','traffic.topic.v2.ActionResult')
			  AND (locked_until IS NULL OR locked_until < now())
			ORDER BY next_attempt_at,created_at,event_id
			LIMIT $1 FOR UPDATE SKIP LOCKED
		), claimed AS (
			UPDATE topic_action_outbox o
			SET locked_until=now()+interval '60 seconds',locked_by=$2
			FROM candidates c WHERE o.event_id=c.event_id
			RETURNING o.event_id::text,o.job_id::text,o.tenant_id,o.event_type,
			          o.partition_key,o.aggregate_version,o.schema_version,o.payload::text
		)
		SELECT event_id,job_id,tenant_id,event_type,partition_key,
		       aggregate_version,schema_version,payload
		FROM claimed ORDER BY event_id`, limit, workerID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]topicActionOutboxItem, 0, limit)
	for rows.Next() {
		var item topicActionOutboxItem
		var rawPayload string
		if err := rows.Scan(
			&item.EventID, &item.JobID, &item.TenantID, &item.EventType,
			&item.PartitionKey, &item.AggregateVersion, &item.SchemaVersion, &rawPayload,
		); err != nil {
			return len(items), err
		}
		item.Payload = []byte(rawPayload)
		if !json.Valid(item.Payload) {
			h.releaseTopicActionOutboxLease(ctx, workerID, item.EventID, "invalid outbox JSON payload")
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return len(items), err
	}
	processed := 0
	for _, item := range items {
		if err := h.publishTopicActionOutboxItem(ctx, workerID, item); err != nil {
			if h.logger != nil {
				h.logger.Warn(
					"Topic action outbox delivery failed",
					zap.String("event_id", item.EventID),
					zap.String("job_id", item.JobID),
					zap.String("event_type", item.EventType),
					zap.Error(err),
				)
			}
			continue
		}
		processed++
	}
	return processed, nil
}

func (h *SystemHandler) publishTopicActionOutboxItem(
	ctx context.Context,
	workerID string,
	item topicActionOutboxItem,
) error {
	if h.topicActionPublish == nil {
		return fmt.Errorf("topic action Kafka publisher is unavailable")
	}
	err := h.topicActionPublish(ctx, item.PartitionKey, item.Payload,
		commonkafka.MessageHeader{Key: "event_id", Value: item.EventID},
		commonkafka.MessageHeader{Key: "event_type", Value: item.EventType},
		commonkafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
		commonkafka.MessageHeader{Key: "job_id", Value: item.JobID},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: fmt.Sprint(item.AggregateVersion)},
		commonkafka.MessageHeader{Key: "schema_version", Value: fmt.Sprint(item.SchemaVersion)},
		commonkafka.MessageHeader{Key: "target_topic", Value: topicActionKafkaTopic},
	)
	if err != nil {
		h.releaseTopicActionOutboxLease(ctx, workerID, item.EventID, err.Error())
		return err
	}
	result, err := h.pgDB.ExecContext(ctx, `
		UPDATE topic_action_outbox
		SET published=true,attempts=attempts+1,last_error='',published_at=now(),
		    locked_until=NULL,locked_by=''
		WHERE event_id=$1::uuid AND published=false AND locked_by=$2`,
		item.EventID, workerID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("topic action outbox lease lost before publish acknowledgement")
	}
	return nil
}

func (h *SystemHandler) releaseTopicActionOutboxLease(
	ctx context.Context,
	workerID, eventID, message string,
) {
	if h.pgDB == nil {
		return
	}
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = h.pgDB.ExecContext(ctx, `
		UPDATE topic_action_outbox
		SET attempts=attempts+1,last_error=$2,
		    next_attempt_at=now()+(LEAST(300,POWER(2,LEAST(attempts+1,8)))::text || ' seconds')::interval,
		    locked_until=NULL,locked_by=''
		WHERE event_id=$1::uuid AND published=false AND locked_by=$3`,
		eventID, message, workerID)
}

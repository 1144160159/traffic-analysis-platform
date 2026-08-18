package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type feedbackOutboxItem struct {
	OutboxID         int64
	EventID          string
	FeedbackID       string
	TenantID         string
	AlertID          string
	PartitionKey     string
	SchemaVersion    int
	AggregateVersion int64
	Payload          []byte
}

type feedbackOutboxEnvelope struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	SchemaVersion    int    `json:"schema_version"`
	AggregateVersion int64  `json:"aggregate_version"`
	FeedbackID       string `json:"feedback_id"`
	TenantID         string `json:"tenant_id"`
	AlertID          string `json:"alert_id"`
	PredictionID     string `json:"prediction_id,omitempty"`
	LabelRevision    int64  `json:"label_revision,omitempty"`
}

func (h *Handler) StartFeedbackOutboxWorker(ctx context.Context, interval time.Duration) error {
	if h == nil || h.feedbackHandler == nil {
		return fmt.Errorf("feedback handler is unavailable")
	}
	return h.feedbackHandler.startOutboxWorker(ctx, interval)
}

func (h *Handler) StartModelFeedbackRevisionOutboxWorker(ctx context.Context, interval time.Duration) error {
	if h == nil || h.feedbackHandler == nil {
		return fmt.Errorf("feedback handler is unavailable")
	}
	return h.feedbackHandler.startTypedOutboxWorker(
		ctx, interval, modelFeedbackRevisionEventType, h.feedbackHandler.revisionProducer,
	)
}

func (h *FeedbackHandler) startOutboxWorker(ctx context.Context, interval time.Duration) error {
	return h.startTypedOutboxWorker(ctx, interval, "alert.feedback.v1", h.kafkaProducer)
}

func (h *FeedbackHandler) startTypedOutboxWorker(
	ctx context.Context,
	interval time.Duration,
	eventType string,
	producer *kafka.Producer,
) error {
	if h.actionAudit == nil || h.actionAudit.db == nil {
		return fmt.Errorf("feedback outbox database is unavailable")
	}
	if producer == nil {
		return fmt.Errorf("feedback Kafka publisher is unavailable")
	}
	if !h.transactionalOutboxEnabled {
		return fmt.Errorf("feedback transactional outbox is disabled")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := hostnameOrDefault() + ":feedback-outbox:" + eventType + ":" + uuid.NewString()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := h.drainTypedOutbox(ctx, workerID, eventType, producer, 50); err != nil && ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Failed to drain alert feedback outbox", zap.Error(err))
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

func (h *FeedbackHandler) drainOutbox(
	ctx context.Context,
	workerID string,
	limit int,
) (int, error) {
	return h.drainTypedOutbox(ctx, workerID, "alert.feedback.v1", h.kafkaProducer, limit)
}

func (h *FeedbackHandler) drainTypedOutbox(
	ctx context.Context,
	workerID, eventType string,
	producer *kafka.Producer,
	limit int,
) (int, error) {
	if h.actionAudit == nil || h.actionAudit.db == nil || producer == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := h.actionAudit.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT outbox_id FROM alert_feedback_outbox
			WHERE published=false AND next_attempt_at <= now()
			  AND payload->>'event_type'=$3
			  AND (locked_until IS NULL OR locked_until < now())
			ORDER BY next_attempt_at,outbox_id
			LIMIT $1 FOR UPDATE SKIP LOCKED
		), claimed AS (
			UPDATE alert_feedback_outbox o
			SET locked_until=now()+interval '60 seconds',locked_by=$2
			FROM candidates c WHERE o.outbox_id=c.outbox_id
			RETURNING o.outbox_id,o.event_id::text,o.feedback_id::text,o.tenant_id,
			          o.alert_id,o.partition_key,o.schema_version,o.aggregate_version,
			          o.payload::text
		)
		SELECT outbox_id,event_id,feedback_id,tenant_id,alert_id,partition_key,
		       schema_version,aggregate_version,payload
		FROM claimed ORDER BY outbox_id`,
		limit, workerID, eventType,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]feedbackOutboxItem, 0, limit)
	for rows.Next() {
		var item feedbackOutboxItem
		var rawPayload string
		if err := rows.Scan(
			&item.OutboxID, &item.EventID, &item.FeedbackID, &item.TenantID,
			&item.AlertID, &item.PartitionKey, &item.SchemaVersion,
			&item.AggregateVersion, &rawPayload,
		); err != nil {
			return len(items), err
		}
		item.Payload = []byte(rawPayload)
		if !json.Valid(item.Payload) {
			h.releaseOutboxLease(ctx, workerID, item.OutboxID, "invalid outbox JSON payload")
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return len(items), err
	}
	processed := 0
	for _, item := range items {
		if err := h.publishTypedOutboxItem(ctx, workerID, eventType, producer, item); err != nil {
			if h.logger != nil {
				h.logger.Warn(
					"Alert feedback outbox delivery failed",
					zap.String("event_id", item.EventID),
					zap.String("feedback_id", item.FeedbackID),
					zap.Error(err),
				)
			}
			continue
		}
		processed++
	}
	return processed, nil
}

func (h *FeedbackHandler) publishOutboxItem(
	ctx context.Context,
	workerID string,
	item feedbackOutboxItem,
) error {
	return h.publishTypedOutboxItem(ctx, workerID, "alert.feedback.v1", h.kafkaProducer, item)
}

func (h *FeedbackHandler) publishTypedOutboxItem(
	ctx context.Context,
	workerID, eventType string,
	producer *kafka.Producer,
	item feedbackOutboxItem,
) error {
	if producer == nil || h.actionAudit == nil || h.actionAudit.db == nil {
		return fmt.Errorf("feedback outbox dependencies are unavailable")
	}
	var envelope feedbackOutboxEnvelope
	if err := json.Unmarshal(item.Payload, &envelope); err != nil ||
		envelope.EventType != eventType || envelope.EventID != item.EventID ||
		envelope.TenantID != item.TenantID || envelope.AlertID != item.AlertID ||
		envelope.SchemaVersion != 1 || envelope.AggregateVersion < 1 {
		h.releaseOutboxLease(ctx, workerID, item.OutboxID, "outbox envelope does not match claimed row")
		return fmt.Errorf("feedback outbox envelope does not match claimed row")
	}
	headers := []kafka.MessageHeader{
		{Key: "event_id", Value: envelope.EventID},
		{Key: "event_type", Value: envelope.EventType},
		{Key: "schema_version", Value: fmt.Sprint(envelope.SchemaVersion)},
		{Key: "aggregate_version", Value: fmt.Sprint(envelope.AggregateVersion)},
		{Key: "tenant_id", Value: envelope.TenantID},
		{Key: "alert_id", Value: envelope.AlertID},
		{Key: "feedback_id", Value: envelope.FeedbackID},
	}
	if eventType == modelFeedbackRevisionEventType {
		if envelope.PredictionID == "" || envelope.LabelRevision != envelope.AggregateVersion {
			h.releaseOutboxLease(ctx, workerID, item.OutboxID, "invalid model feedback revision envelope")
			return fmt.Errorf("invalid model feedback revision envelope")
		}
		headers = append(headers,
			kafka.MessageHeader{Key: "prediction_id", Value: envelope.PredictionID},
			kafka.MessageHeader{Key: "label_revision", Value: fmt.Sprint(envelope.LabelRevision)},
		)
	}
	err := producer.Send(ctx, item.PartitionKey, item.Payload, headers...)
	if err != nil {
		h.releaseOutboxLease(ctx, workerID, item.OutboxID, err.Error())
		return err
	}
	result, err := h.actionAudit.db.ExecContext(ctx, `
		UPDATE alert_feedback_outbox
		SET published=true,attempts=attempts+1,last_error='',published_at=now(),
		    locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND published=false AND locked_by=$2`,
		item.OutboxID, workerID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("feedback outbox lease lost before publish acknowledgement")
	}
	return nil
}

func (h *FeedbackHandler) releaseOutboxLease(
	ctx context.Context,
	workerID string,
	outboxID int64,
	message string,
) {
	if h.actionAudit == nil || h.actionAudit.db == nil {
		return
	}
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = h.actionAudit.db.ExecContext(ctx, `
		UPDATE alert_feedback_outbox
		SET attempts=attempts+1,last_error=$2,
		    next_attempt_at=now()+(LEAST(300,POWER(2,LEAST(attempts+1,8)))::text || ' seconds')::interval,
		    locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND published=false AND locked_by=$3`,
		outboxID, message, workerID,
	)
}

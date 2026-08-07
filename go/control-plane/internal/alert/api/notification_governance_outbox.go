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

const notificationGovernanceOutboxMaxAttempts = 10

type notificationGovernanceEventProducer interface {
	SendJSON(context.Context, string, interface{}, ...commonkafka.MessageHeader) error
}

type notificationGovernanceOutboxItem struct {
	OutboxID         int64
	EventID          string
	AggregateType    string
	AggregateID      string
	AggregateVersion int64
	TenantID         string
	EventType        string
	SchemaVersion    int
	PartitionKey     string
	TraceID          string
	Payload          json.RawMessage
}

func (h *AdvancedHandler) SetNotificationGovernanceEventProducer(producer notificationGovernanceEventProducer) {
	if h != nil {
		h.notificationGovernanceProducer = producer
	}
}

func (h *AdvancedHandler) StartNotificationGovernanceOutboxWorker(ctx context.Context, interval time.Duration) error {
	if h == nil || h.advancedRepo == nil || h.advancedRepo.db == nil {
		return fmt.Errorf("notification governance outbox database is unavailable")
	}
	if h.notificationGovernanceProducer == nil {
		return fmt.Errorf("notification governance Kafka publisher is unavailable")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := hostnameOrDefault() + ":notification-governance-outbox:" + uuid.NewString()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := h.drainNotificationGovernanceOutbox(ctx, workerID, 50); err != nil && ctx.Err() == nil && h.advancedRepo.logger != nil {
				h.advancedRepo.logger.Warn("Failed to drain notification governance outbox", zap.Error(err))
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

func (h *AdvancedHandler) drainNotificationGovernanceOutbox(ctx context.Context, workerID string, limit int) (int, error) {
	if h == nil || h.advancedRepo == nil || h.advancedRepo.db == nil || h.notificationGovernanceProducer == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := h.advancedRepo.db.QueryContext(ctx, `WITH candidates AS (
		SELECT outbox_id FROM notification_governance_outbox
		WHERE status IN ('pending','processing') AND publish_attempts<$3 AND next_retry_at<=now()
		  AND (locked_until IS NULL OR locked_until<now())
		ORDER BY next_retry_at,occurred_at,outbox_id LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE notification_governance_outbox o
		SET status='processing',locked_until=now()+interval '60 seconds',locked_by=$2
		FROM candidates c WHERE o.outbox_id=c.outbox_id
		RETURNING o.outbox_id,o.event_id::text,o.aggregate_type,o.aggregate_id,o.aggregate_version,
		          o.tenant_id,o.event_type,o.schema_version,o.partition_key,o.trace_id,o.payload::text
	) SELECT outbox_id,event_id,aggregate_type,aggregate_id,aggregate_version,tenant_id,
	         event_type,schema_version,partition_key,trace_id,payload FROM claimed ORDER BY outbox_id`,
		limit, workerID, notificationGovernanceOutboxMaxAttempts)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]notificationGovernanceOutboxItem, 0, limit)
	for rows.Next() {
		var item notificationGovernanceOutboxItem
		var payload string
		if err := rows.Scan(&item.OutboxID, &item.EventID, &item.AggregateType, &item.AggregateID,
			&item.AggregateVersion, &item.TenantID, &item.EventType, &item.SchemaVersion,
			&item.PartitionKey, &item.TraceID, &payload); err != nil {
			return len(items), err
		}
		item.Payload = json.RawMessage(payload)
		if !json.Valid(item.Payload) {
			h.failNotificationGovernanceOutboxItem(ctx, workerID, item.OutboxID, "invalid outbox JSON payload")
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return len(items), err
	}
	processed := 0
	for _, item := range items {
		if err := h.publishNotificationGovernanceOutboxItem(ctx, workerID, item); err != nil {
			if h.advancedRepo.logger != nil {
				h.advancedRepo.logger.Warn("Notification governance event delivery failed", zap.String("event_id", item.EventID), zap.Error(err))
			}
			continue
		}
		processed++
	}
	return processed, nil
}

func (h *AdvancedHandler) publishNotificationGovernanceOutboxItem(ctx context.Context, workerID string, item notificationGovernanceOutboxItem) error {
	validLifecycle := (item.AggregateType == "notification_rule" &&
		(item.EventType == "traffic.notification.rule.v1.RuleCreated" || item.EventType == "traffic.notification.rule.v1.RuleUpdated")) ||
		(item.AggregateType == "notification_template" &&
			(item.EventType == "traffic.notification.template.v1.TemplateCreated" || item.EventType == "traffic.notification.template.v1.TemplateUpdated")) ||
		(item.AggregateType == "notification_escalation_policy" &&
			(item.EventType == "traffic.notification.escalation.v1.PolicyCreated" || item.EventType == "traffic.notification.escalation.v1.PolicyUpdated")) ||
		(item.AggregateType == "notification_silence_rule" &&
			(item.EventType == "traffic.notification.silence.v1.SilenceCreated" || item.EventType == "traffic.notification.silence.v1.SilenceUpdated")) ||
		(item.AggregateType == "notification_settings" && item.EventType == "traffic.notification.settings.v1.SettingsUpdated")
	if !validLifecycle || item.AggregateVersion < 1 || item.SchemaVersion != 1 {
		err := fmt.Errorf("invalid notification governance event envelope")
		h.failNotificationGovernanceOutboxItem(ctx, workerID, item.OutboxID, err.Error())
		return err
	}
	if h.notificationGovernanceProducer == nil {
		return fmt.Errorf("notification governance Kafka publisher is unavailable")
	}
	err := h.notificationGovernanceProducer.SendJSON(ctx, item.PartitionKey, item.Payload,
		commonkafka.MessageHeader{Key: "event_id", Value: item.EventID},
		commonkafka.MessageHeader{Key: "event_type", Value: item.EventType},
		commonkafka.MessageHeader{Key: "schema_version", Value: fmt.Sprint(item.SchemaVersion)},
		commonkafka.MessageHeader{Key: "aggregate_type", Value: item.AggregateType},
		commonkafka.MessageHeader{Key: "aggregate_id", Value: item.AggregateID},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: fmt.Sprint(item.AggregateVersion)},
		commonkafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
		commonkafka.MessageHeader{Key: "trace_id", Value: item.TraceID})
	if err != nil {
		h.failNotificationGovernanceOutboxItem(ctx, workerID, item.OutboxID, err.Error())
		return err
	}
	result, err := h.advancedRepo.db.ExecContext(ctx, `UPDATE notification_governance_outbox
		SET status='published',publish_attempts=publish_attempts+1,last_error='',published_at=now(),
		    locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`, item.OutboxID, workerID)
	if err != nil {
		return err
	}
	if affectedRows(result) != 1 {
		return fmt.Errorf("notification governance outbox lease lost after Kafka acknowledgement")
	}
	return nil
}

func (h *AdvancedHandler) failNotificationGovernanceOutboxItem(ctx context.Context, workerID string, outboxID int64, message string) {
	if h == nil || h.advancedRepo == nil || h.advancedRepo.db == nil {
		return
	}
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = h.advancedRepo.db.ExecContext(ctx, `UPDATE notification_governance_outbox
		SET publish_attempts=publish_attempts+1,last_error=$2,
		    status=CASE WHEN publish_attempts+1 >= $4 THEN 'dead' ELSE 'pending' END,
		    next_retry_at=now()+(LEAST(300,POWER(2,LEAST(publish_attempts+1,8)))::text || ' seconds')::interval,
		    locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$3`,
		outboxID, message, workerID, notificationGovernanceOutboxMaxAttempts)
}

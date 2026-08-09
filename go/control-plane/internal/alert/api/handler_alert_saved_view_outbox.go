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

const savedViewOutboxMaxAttempts = 10

type savedViewEventProducer interface {
	SendJSON(context.Context, string, interface{}, ...kafka.MessageHeader) error
}

type savedViewOutboxItem struct {
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

// StartSavedViewOutboxWorker publishes only committed saved-view events. It
// reclaims expired processing leases, bounds retries and moves poison events to
// an operational dead state instead of spinning forever.
func (h *Handler) StartSavedViewOutboxWorker(ctx context.Context, interval time.Duration) error {
	if h == nil || h.actionAudit == nil || h.actionAudit.db == nil {
		return fmt.Errorf("saved-view outbox database is unavailable")
	}
	if h.savedViewProducer == nil {
		return fmt.Errorf("saved-view Kafka publisher is unavailable")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := hostnameOrDefault() + ":saved-view-outbox:" + uuid.NewString()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := h.drainSavedViewOutbox(ctx, workerID, 50); err != nil && ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Failed to drain saved-view outbox", zap.Error(err))
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

func (h *Handler) drainSavedViewOutbox(ctx context.Context, workerID string, limit int) (int, error) {
	if h.savedViewProducer == nil || h.actionAudit == nil || h.actionAudit.db == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := h.actionAudit.db.QueryContext(ctx, `WITH candidates AS (
		SELECT outbox_id FROM alert_saved_view_outbox
		WHERE status IN ('pending','processing')
		  AND publish_attempts < $3 AND next_retry_at <= now()
		  AND (locked_until IS NULL OR locked_until < now())
		ORDER BY next_retry_at,occurred_at,outbox_id
		LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE alert_saved_view_outbox o
		SET status='processing',locked_until=now()+interval '60 seconds',locked_by=$2
		FROM candidates c WHERE o.outbox_id=c.outbox_id
		RETURNING o.outbox_id,o.event_id::text,o.aggregate_type,o.aggregate_id::text,
		          o.aggregate_version,o.tenant_id,o.event_type,o.schema_version,
		          o.partition_key,o.trace_id,o.payload::text
	) SELECT outbox_id,event_id,aggregate_type,aggregate_id,aggregate_version,
	         tenant_id,event_type,schema_version,partition_key,trace_id,payload
	  FROM claimed ORDER BY outbox_id`, limit, workerID, savedViewOutboxMaxAttempts)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]savedViewOutboxItem, 0, limit)
	for rows.Next() {
		var item savedViewOutboxItem
		var rawPayload string
		if err := rows.Scan(
			&item.OutboxID, &item.EventID, &item.AggregateType, &item.AggregateID,
			&item.AggregateVersion, &item.TenantID, &item.EventType,
			&item.SchemaVersion, &item.PartitionKey, &item.TraceID, &rawPayload,
		); err != nil {
			return len(items), err
		}
		item.Payload = json.RawMessage(rawPayload)
		if !json.Valid(item.Payload) {
			h.failSavedViewOutboxItem(ctx, workerID, item.OutboxID, "invalid outbox JSON payload")
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return len(items), err
	}
	processed := 0
	for _, item := range items {
		if err := h.publishSavedViewOutboxItem(ctx, workerID, item); err != nil {
			if h.logger != nil {
				h.logger.Warn("Saved-view outbox delivery failed", zap.String("event_id", item.EventID), zap.Error(err))
			}
			continue
		}
		processed++
	}
	return processed, nil
}

func (h *Handler) publishSavedViewOutboxItem(ctx context.Context, workerID string, item savedViewOutboxItem) error {
	if h.savedViewProducer == nil || h.actionAudit == nil || h.actionAudit.db == nil {
		return fmt.Errorf("saved-view outbox dependencies are unavailable")
	}
	if item.EventType != "alert.saved-view.saved.v1" || item.SchemaVersion != 1 || item.AggregateVersion < 1 {
		err := fmt.Errorf("invalid saved-view event envelope")
		h.failSavedViewOutboxItem(ctx, workerID, item.OutboxID, err.Error())
		return err
	}
	err := h.savedViewProducer.SendJSON(ctx, item.PartitionKey, item.Payload,
		kafka.MessageHeader{Key: "event_id", Value: item.EventID},
		kafka.MessageHeader{Key: "event_type", Value: item.EventType},
		kafka.MessageHeader{Key: "schema_version", Value: fmt.Sprint(item.SchemaVersion)},
		kafka.MessageHeader{Key: "aggregate_type", Value: item.AggregateType},
		kafka.MessageHeader{Key: "aggregate_id", Value: item.AggregateID},
		kafka.MessageHeader{Key: "aggregate_version", Value: fmt.Sprint(item.AggregateVersion)},
		kafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
		kafka.MessageHeader{Key: "trace_id", Value: item.TraceID})
	if err != nil {
		h.failSavedViewOutboxItem(ctx, workerID, item.OutboxID, err.Error())
		return err
	}
	result, err := h.actionAudit.db.ExecContext(ctx, `UPDATE alert_saved_view_outbox
		SET status='published',publish_attempts=publish_attempts+1,last_error='',
		    published_at=now(),locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`, item.OutboxID, workerID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("saved-view outbox lease lost after Kafka acknowledgement")
	}
	return nil
}

func (h *Handler) failSavedViewOutboxItem(ctx context.Context, workerID string, outboxID int64, message string) {
	if h.actionAudit == nil || h.actionAudit.db == nil {
		return
	}
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = h.actionAudit.db.ExecContext(ctx, `UPDATE alert_saved_view_outbox
		SET publish_attempts=publish_attempts+1,last_error=$2,
		    status=CASE WHEN publish_attempts+1 >= $4 THEN 'dead' ELSE 'pending' END,
		    next_retry_at=now()+(LEAST(300,POWER(2,LEAST(publish_attempts+1,8)))::text || ' seconds')::interval,
		    locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$3`,
		outboxID, message, workerID, savedViewOutboxMaxAttempts)
}

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

type alertEvidenceLinkOutboxItem struct {
	EventID          string
	TenantID         string
	AggregateID      string
	AggregateVersion int64
	EventType        string
	PartitionKey     string
	SchemaVersion    int
	Attempts         int
	Payload          []byte
	TraceID          string
}

type alertEvidenceLinkEnvelope struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	TenantID         string `json:"tenant_id"`
	AggregateType    string `json:"aggregate_type"`
	AggregateID      string `json:"aggregate_id"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
	SchemaVersion    int    `json:"schema_version"`
	AlertID          string `json:"alert_id"`
	EvidenceID       string `json:"evidence_id"`
	ObjectVersion    string `json:"object_version"`
	ObjectSHA256     string `json:"object_sha256"`
	TraceID          string `json:"trace_id"`
}

func (h *Handler) StartAlertEvidenceLinkOutboxWorker(ctx context.Context, interval time.Duration) error {
	if !h.alertEvidenceLinkEnabled {
		return fmt.Errorf("alert evidence link writer is disabled")
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		return fmt.Errorf("alert evidence link outbox database is unavailable")
	}
	if h.alertEvidenceLinkPublisher == nil || h.alertEvidenceLinkConsumerReady == nil {
		return fmt.Errorf("alert evidence link publisher and consumer readiness are required")
	}
	if err := h.alertEvidenceLinkConsumerReady(ctx); err != nil {
		return fmt.Errorf("alert evidence link consumer is not ready: %w", err)
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := hostnameOrDefault() + ":alert-evidence-link-outbox:" + uuid.NewString()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := h.drainAlertEvidenceLinkOutbox(ctx, workerID, 50); err != nil && ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Failed to drain alert evidence link outbox", zap.Error(err))
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

func (h *Handler) drainAlertEvidenceLinkOutbox(ctx context.Context, workerID string, limit int) (int, error) {
	if h.actionAudit == nil || h.actionAudit.db == nil || h.alertEvidenceLinkPublisher == nil {
		return 0, nil
	}
	if h.alertEvidenceLinkConsumerReady == nil {
		return 0, fmt.Errorf("alert evidence link consumer readiness gate is unavailable")
	}
	if err := h.alertEvidenceLinkConsumerReady(ctx); err != nil {
		return 0, fmt.Errorf("alert evidence link consumer readiness withdrawn: %w", err)
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := h.actionAudit.db.QueryContext(ctx, `WITH candidates AS (
		SELECT event_id FROM alert_evidence_link_outbox
		WHERE status IN ('pending','processing') AND next_attempt_at<=now()
		  AND (status='pending' OR locked_until IS NULL OR locked_until<now())
		ORDER BY next_attempt_at,created_at,event_id
		LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE alert_evidence_link_outbox o
		SET status='processing',attempts=attempts+1,last_attempt_at=now(),
		    locked_until=now()+interval '60 seconds',locked_by=$2
		FROM candidates c WHERE o.event_id=c.event_id
		RETURNING o.event_id::text,o.tenant_id,o.aggregate_id::text,o.aggregate_version,
		          o.event_type,o.partition_key,o.schema_version,o.attempts,o.payload::text
	)
	SELECT event_id,tenant_id,aggregate_id,aggregate_version,event_type,partition_key,
	       schema_version,attempts,payload FROM claimed ORDER BY event_id`, limit, workerID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]alertEvidenceLinkOutboxItem, 0, limit)
	for rows.Next() {
		var item alertEvidenceLinkOutboxItem
		var payload string
		if err := rows.Scan(&item.EventID, &item.TenantID, &item.AggregateID, &item.AggregateVersion,
			&item.EventType, &item.PartitionKey, &item.SchemaVersion, &item.Attempts, &payload); err != nil {
			return 0, err
		}
		item.Payload = []byte(payload)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	processed := 0
	for i := range items {
		if err := h.publishAlertEvidenceLinkOutboxItem(ctx, workerID, &items[i]); err != nil {
			if h.logger != nil {
				h.logger.Warn("Alert evidence link event delivery failed", zap.String("event_id", items[i].EventID), zap.Error(err))
			}
			continue
		}
		processed++
	}
	return processed, nil
}

func (h *Handler) publishAlertEvidenceLinkOutboxItem(
	ctx context.Context, workerID string, item *alertEvidenceLinkOutboxItem,
) error {
	if err := validateAlertEvidenceLinkOutboxItem(item); err != nil {
		h.failAlertEvidenceLinkOutboxItem(ctx, workerID, *item, err.Error())
		return err
	}
	headers := []commonkafka.MessageHeader{
		{Key: "event_id", Value: item.EventID},
		{Key: "event_type", Value: item.EventType},
		{Key: "tenant_id", Value: item.TenantID},
		{Key: "stream", Value: "alert_evidence_link"},
		{Key: "aggregate_id", Value: item.AggregateID},
		{Key: "aggregate_version", Value: fmt.Sprint(item.AggregateVersion)},
		{Key: "schema_version", Value: fmt.Sprint(item.SchemaVersion)},
		{Key: "trace_id", Value: item.TraceID},
		{Key: "target_topic", Value: AlertEvidenceLinkEventTopic},
		{Key: "content_type", Value: "application/json"},
		{Key: commonkafka.PublishAttemptHeader, Value: item.EventID},
	}
	receipt, err := h.alertEvidenceLinkPublisher.Send(ctx, item.PartitionKey, item.Payload, headers...)
	if err != nil {
		h.failAlertEvidenceLinkOutboxItem(ctx, workerID, *item, err.Error())
		return err
	}
	if receipt.Topic != AlertEvidenceLinkEventTopic || receipt.Key != item.PartitionKey ||
		receipt.Partition < 0 || receipt.Offset < 0 || receipt.AcknowledgedAt.IsZero() {
		err := fmt.Errorf("incomplete or mismatched alert evidence broker receipt")
		h.failAlertEvidenceLinkOutboxItem(ctx, workerID, *item, err.Error())
		return err
	}
	result, err := h.actionAudit.db.ExecContext(ctx, `UPDATE alert_evidence_link_outbox
		SET status='published',last_error='',published_at=now(),locked_until=NULL,locked_by='',
		    broker_partition=$3,broker_offset=$4,broker_acknowledged_at=$5
		WHERE event_id=$1::uuid AND status='processing' AND locked_by=$2`, item.EventID, workerID,
		receipt.Partition, receipt.Offset, receipt.AcknowledgedAt.UTC())
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("alert evidence link outbox lease lost after Kafka acknowledgement")
	}
	return nil
}

func validateAlertEvidenceLinkOutboxItem(item *alertEvidenceLinkOutboxItem) error {
	var envelope alertEvidenceLinkEnvelope
	if err := json.Unmarshal(item.Payload, &envelope); err != nil {
		return fmt.Errorf("decode alert evidence link envelope: %w", err)
	}
	if _, err := uuid.Parse(item.EventID); err != nil || envelope.EventID != item.EventID ||
		envelope.EventType != item.EventType || envelope.TenantID != item.TenantID ||
		envelope.AggregateType != "alert_evidence_link" || envelope.AggregateID != item.AggregateID ||
		envelope.AggregateVersion != item.AggregateVersion || envelope.PartitionKey != item.PartitionKey ||
		envelope.SchemaVersion != 1 || item.SchemaVersion != 1 || strings.TrimSpace(envelope.TraceID) == "" ||
		strings.TrimSpace(envelope.AlertID) == "" || strings.TrimSpace(envelope.EvidenceID) == "" {
		return fmt.Errorf("alert evidence link event envelope identity mismatch")
	}
	if envelope.EventType != "traffic.alert-evidence.v1.Linked" && envelope.EventType != "traffic.alert-evidence.v1.Unlinked" {
		return fmt.Errorf("unsupported alert evidence link event type")
	}
	if envelope.ObjectSHA256 != "" && !validLowerSHA256(envelope.ObjectSHA256) {
		return fmt.Errorf("alert evidence link object digest is invalid")
	}
	item.TraceID = envelope.TraceID
	return nil
}

func (h *Handler) failAlertEvidenceLinkOutboxItem(
	ctx context.Context, workerID string, item alertEvidenceLinkOutboxItem, message string,
) {
	status := "pending"
	deadAt := interface{}(nil)
	if item.Attempts >= alertEvidenceLinkMaxAttempts {
		status = "dead"
		deadAt = time.Now().UTC()
	}
	_, _ = h.actionAudit.db.ExecContext(ctx, `UPDATE alert_evidence_link_outbox
		SET status=$3,last_error=$4,next_attempt_at=now()+
		    make_interval(secs=>LEAST(300,GREATEST(1,power(2,LEAST(attempts,8))::int))),
		    locked_until=NULL,locked_by='',dead_at=$5
		WHERE event_id=$1::uuid AND status='processing' AND locked_by=$2`,
		item.EventID, workerID, status, truncateEvidenceOutboxError(message), deadAt)
}

func truncateEvidenceOutboxError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 2000 {
		return value[:2000]
	}
	return value
}

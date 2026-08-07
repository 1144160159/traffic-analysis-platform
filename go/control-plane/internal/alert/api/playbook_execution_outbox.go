package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	PlaybookExecutionEventTopic = "playbook.execution.events.v2"
	playbookOutboxMaxAttempts   = 8
)

type playbookExecutionOutboxItem struct {
	OutboxID         int64
	EventID          string
	ExecutionID      string
	TenantID         string
	PlaybookName     string
	EventType        string
	SchemaVersion    int
	AggregateVersion int64
	PartitionKey     string
	Payload          []byte
	Attempts         int
	TraceID          string
}

type playbookExecutionEventEnvelope struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	TenantID         string `json:"tenant_id"`
	AggregateType    string `json:"aggregate_type"`
	AggregateID      string `json:"aggregate_id"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
	SchemaVersion    int    `json:"schema_version"`
	ExecutionID      string `json:"execution_id"`
	PlaybookName     string `json:"playbook_name"`
	Status           string `json:"status"`
	TraceID          string `json:"trace_id"`
}

// SetPlaybookExecutionEventProducer enables broker-acknowledged delivery of
// execution lifecycle rows. A nil producer deliberately leaves outbox rows
// pending and therefore cannot be mistaken for Kafka success.
func (h *AdvancedHandler) SetPlaybookExecutionEventProducer(producer *commonkafka.Producer, topic string) {
	h.playbookExecutionPublish = nil
	h.playbookExecutionTopic = ""
	if producer != nil {
		h.playbookExecutionPublish = producer.Send
		h.playbookExecutionTopic = strings.TrimSpace(topic)
	}
}

func (h *AdvancedHandler) StartPlaybookExecutionEventOutboxWorker(ctx context.Context, interval time.Duration) error {
	if h.advancedRepo == nil || h.advancedRepo.db == nil {
		return fmt.Errorf("playbook execution outbox database is unavailable")
	}
	if h.playbookExecutionPublish == nil || h.playbookExecutionTopic == "" {
		return fmt.Errorf("playbook execution Kafka publisher is unavailable")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := hostnameOrDefault() + ":playbook-execution-outbox:" + uuid.NewString()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := h.drainPlaybookExecutionOutbox(ctx, workerID, 50); err != nil && ctx.Err() == nil && h.advancedRepo.logger != nil {
				h.advancedRepo.logger.Warn("Failed to drain playbook execution outbox", zap.Error(err))
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

func (h *AdvancedHandler) drainPlaybookExecutionOutbox(ctx context.Context, workerID string, limit int) (int, error) {
	if h.advancedRepo == nil || h.advancedRepo.db == nil || h.playbookExecutionPublish == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	items, err := h.claimPlaybookExecutionOutbox(ctx, workerID, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, item := range items {
		if err := h.publishPlaybookExecutionOutboxItem(ctx, workerID, &item); err != nil {
			if h.advancedRepo.logger != nil {
				h.advancedRepo.logger.Warn("Playbook execution event delivery failed", zap.String("event_id", item.EventID), zap.Error(err))
			}
			continue
		}
		processed++
	}
	return processed, nil
}

func (h *AdvancedHandler) claimPlaybookExecutionOutbox(ctx context.Context, workerID string, limit int) ([]playbookExecutionOutboxItem, error) {
	rows, err := h.advancedRepo.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT outbox_id FROM alert_playbook_execution_outbox
			WHERE published=false AND status IN ('pending','processing')
			  AND next_attempt_at<=now()
			  AND (status='pending' OR locked_until IS NULL OR locked_until<now())
			ORDER BY next_attempt_at,created_at,outbox_id
			LIMIT $1 FOR UPDATE SKIP LOCKED
		), claimed AS (
			UPDATE alert_playbook_execution_outbox o
			SET status='processing',attempts=attempts+1,last_attempt_at=now(),
			    locked_until=now()+interval '60 seconds',locked_by=$2
			FROM candidates c WHERE o.outbox_id=c.outbox_id
			RETURNING o.outbox_id,o.event_id::text,o.execution_id,o.tenant_id,o.playbook_name,
			          o.event_type,o.schema_version,o.aggregate_version,o.partition_key,
			          o.payload::text,o.attempts
		)
		SELECT outbox_id,event_id,execution_id,tenant_id,playbook_name,event_type,
		       schema_version,aggregate_version,partition_key,payload,attempts
		FROM claimed ORDER BY outbox_id`, limit, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]playbookExecutionOutboxItem, 0, limit)
	for rows.Next() {
		var item playbookExecutionOutboxItem
		var payload string
		if err := rows.Scan(&item.OutboxID, &item.EventID, &item.ExecutionID, &item.TenantID,
			&item.PlaybookName, &item.EventType, &item.SchemaVersion, &item.AggregateVersion,
			&item.PartitionKey, &payload, &item.Attempts); err != nil {
			return items, err
		}
		item.Payload = []byte(payload)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (h *AdvancedHandler) publishPlaybookExecutionOutboxItem(ctx context.Context, workerID string, item *playbookExecutionOutboxItem) error {
	if err := validatePlaybookExecutionOutboxItem(item); err != nil {
		h.failPlaybookExecutionOutboxItem(ctx, workerID, *item, err.Error())
		return err
	}
	headers := []commonkafka.MessageHeader{
		{Key: "event_id", Value: item.EventID},
		{Key: "event_type", Value: item.EventType},
		{Key: "tenant_id", Value: item.TenantID},
		{Key: "aggregate_type", Value: "playbook_execution"},
		{Key: "aggregate_id", Value: item.ExecutionID},
		{Key: "aggregate_version", Value: strconv.FormatInt(item.AggregateVersion, 10)},
		{Key: "schema_version", Value: strconv.Itoa(item.SchemaVersion)},
		{Key: "trace_id", Value: item.TraceID},
		{Key: "target_topic", Value: h.playbookExecutionTopic},
	}
	if err := h.playbookExecutionPublish(ctx, item.PartitionKey, item.Payload, headers...); err != nil {
		h.failPlaybookExecutionOutboxItem(ctx, workerID, *item, err.Error())
		return err
	}
	result, err := h.advancedRepo.db.ExecContext(ctx, `UPDATE alert_playbook_execution_outbox
		SET status='published',published=true,last_error='',published_at=now(),locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND status='processing' AND published=false AND locked_by=$2`, item.OutboxID, workerID)
	if err != nil {
		return err
	}
	if affectedRows(result) != 1 {
		return fmt.Errorf("playbook execution outbox lease lost after Kafka acknowledgement")
	}
	return nil
}

func validatePlaybookExecutionOutboxItem(item *playbookExecutionOutboxItem) error {
	var envelope playbookExecutionEventEnvelope
	if err := json.Unmarshal(item.Payload, &envelope); err != nil {
		return fmt.Errorf("decode playbook execution event: %w", err)
	}
	if _, err := uuid.Parse(item.EventID); err != nil || envelope.EventID != item.EventID {
		return fmt.Errorf("playbook execution event_id is invalid or mismatched")
	}
	if item.SchemaVersion != 2 || envelope.SchemaVersion != 2 ||
		envelope.EventType != item.EventType || !validPlaybookExecutionEvent(envelope.EventType) ||
		envelope.TenantID != item.TenantID || envelope.AggregateType != "playbook_execution" ||
		envelope.AggregateID != item.ExecutionID || envelope.ExecutionID != item.ExecutionID ||
		envelope.AggregateVersion != item.AggregateVersion || item.AggregateVersion <= 0 ||
		envelope.PartitionKey != item.PartitionKey || envelope.PlaybookName != item.PlaybookName ||
		strings.TrimSpace(envelope.Status) == "" || strings.TrimSpace(envelope.TraceID) == "" {
		return fmt.Errorf("playbook execution event envelope identity mismatch")
	}
	item.TraceID = envelope.TraceID
	return nil
}

func validPlaybookExecutionEvent(value string) bool {
	switch value {
	case "traffic.playbook.v2.ExecutionRequested", "traffic.playbook.v2.ExecutionApproved",
		"traffic.playbook.v2.ExecutionRejected", "traffic.playbook.v2.ExecutionCancelled",
		"traffic.playbook.v2.ExecutionCompleted", "traffic.playbook.v2.ExecutionPartial",
		"traffic.playbook.v2.ExecutionFailed", "traffic.playbook.v2.CompensationRequested",
		"traffic.playbook.v2.Compensated", "traffic.playbook.v2.CompensationFailed":
		return true
	default:
		return false
	}
}

func (h *AdvancedHandler) failPlaybookExecutionOutboxItem(ctx context.Context, workerID string, item playbookExecutionOutboxItem, message string) {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	status := "pending"
	if item.Attempts >= playbookOutboxMaxAttempts {
		status = "dead"
	}
	_, _ = h.advancedRepo.db.ExecContext(ctx, `UPDATE alert_playbook_execution_outbox
		SET status=$2,last_error=$3,
		    next_attempt_at=now()+(LEAST(300,POWER(2,LEAST(attempts,8)))::text || ' seconds')::interval,
		    dead_at=CASE WHEN $2='dead' THEN now() ELSE NULL END,
		    locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND status='processing' AND published=false AND locked_by=$4`,
		item.OutboxID, status, message, workerID)
}

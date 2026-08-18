package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	probeOperationRequestedEvent    = "traffic.probe.v2.OperationRequested"
	probeOperationAcknowledgedEvent = "traffic.probe.v2.OperationAcknowledged"
	probeOperationExpiredEvent      = "traffic.probe.v2.OperationExpired"
)

type probeOperationOutboxItem struct {
	EventID          string
	OperationID      string
	TenantID         string
	EventType        string
	PartitionKey     string
	AggregateVersion int64
	SchemaVersion    int
	PublishAttempt   string
	Payload          []byte
}

func (h *SystemHandler) StartProbeOperationOutboxWorker(ctx context.Context, interval time.Duration) error {
	if h.pgDB == nil {
		return fmt.Errorf("probe operation outbox database is unavailable")
	}
	if h.probeCommandPublish == nil || h.probeEventPublish == nil {
		return fmt.Errorf("probe operation Kafka publishers are unavailable")
	}
	if h.probeDispatcherGate == nil {
		return fmt.Errorf("probe operation dispatcher readiness gate is unavailable")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := hostnameOrDefault() + ":probe-outbox:" + uuid.NewString()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := h.drainProbeOperationOutbox(ctx, workerID, 50); err != nil && ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Failed to drain probe operation outbox", zap.Error(err))
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

func (h *SystemHandler) drainProbeOperationOutbox(ctx context.Context, workerID string, limit int) (int, error) {
	if h.pgDB == nil || h.probeCommandPublish == nil || h.probeEventPublish == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if _, err := h.expireProbeOperations(ctx, limit); err != nil {
		return 0, err
	}
	items, err := h.claimProbeOperationOutbox(ctx, workerID, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, item := range items {
		if err := h.publishProbeOperationOutboxItem(ctx, workerID, item); err != nil {
			if h.logger != nil {
				h.logger.Warn(
					"Probe operation outbox delivery failed",
					zap.String("event_id", item.EventID),
					zap.String("operation_id", item.OperationID),
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

func (h *SystemHandler) claimProbeOperationOutbox(
	ctx context.Context,
	workerID string,
	limit int,
) ([]probeOperationOutboxItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if h.probeDispatcherGate == nil {
		// Direct drain remains available to existing recovery tools and focused
		// tests. The production worker cannot enter this compatibility seam:
		// StartProbeOperationOutboxWorker rejects a nil readiness gate above.
		rows, err := queryProbeOperationOutboxClaims(ctx, h.pgDB, workerID, limit)
		if err != nil {
			return nil, err
		}
		items, invalidEventIDs, err := scanProbeOperationOutboxClaims(rows, limit)
		if err != nil {
			return items, err
		}
		for _, eventID := range invalidEventIDs {
			releaseInvalidProbeOutboxClaim(ctx, h.pgDB, workerID, eventID)
		}
		return items, nil
	}
	return h.probeDispatcherGate.AllowClaim(ctx, workerID, limit)
}

type probeOutboxClaimQueryer interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

type probeOutboxClaimExecutor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func queryProbeOperationOutboxClaims(
	ctx context.Context,
	queryer probeOutboxClaimQueryer,
	workerID string,
	limit int,
) (*sql.Rows, error) {
	return queryer.QueryContext(ctx, `WITH candidates AS (
		SELECT o.event_id FROM probe_operation_outbox o
		JOIN probe_operations p ON p.operation_id=o.operation_id
		WHERE o.publish_state IN ('PENDING','OUTCOME_UNKNOWN') AND o.next_attempt_at <= now()
		  AND o.event_type IN ('traffic.probe.v2.OperationRequested','traffic.probe.v2.OperationAcknowledged','traffic.probe.v2.OperationExpired')
		  AND (o.event_type IN ('traffic.probe.v2.OperationAcknowledged','traffic.probe.v2.OperationExpired')
		       OR (p.status IN ('accepted','delivered') AND p.expires_at > now()))
		  AND (o.locked_until IS NULL OR o.locked_until < now())
		ORDER BY o.next_attempt_at,o.created_at,o.event_id
		LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE probe_operation_outbox o
		SET locked_until=now()+interval '60 seconds',locked_by=$2,
		    publish_state='OUTCOME_UNKNOWN',publish_attempt=uuid_generate_v4(),
		    broker_topic='',broker_partition=NULL,broker_offset=NULL,acked_at=NULL
		FROM candidates c WHERE o.event_id=c.event_id
		RETURNING o.event_id::text,o.operation_id::text,o.tenant_id,o.event_type,
		          o.partition_key,o.aggregate_version,o.schema_version,
		          o.publish_attempt::text,o.payload::text
	) SELECT event_id,operation_id,tenant_id,event_type,partition_key,
	         aggregate_version,schema_version,publish_attempt,payload
	  FROM claimed ORDER BY event_id`, limit, workerID)
}

func scanProbeOperationOutboxClaims(
	rows *sql.Rows,
	limit int,
) ([]probeOperationOutboxItem, []string, error) {
	defer rows.Close()
	items := make([]probeOperationOutboxItem, 0, limit)
	invalidEventIDs := make([]string, 0)
	for rows.Next() {
		var item probeOperationOutboxItem
		var rawPayload string
		if err := rows.Scan(
			&item.EventID, &item.OperationID, &item.TenantID, &item.EventType,
			&item.PartitionKey, &item.AggregateVersion, &item.SchemaVersion,
			&item.PublishAttempt, &rawPayload,
		); err != nil {
			return items, invalidEventIDs, err
		}
		item.Payload = []byte(rawPayload)
		if !json.Valid(item.Payload) {
			invalidEventIDs = append(invalidEventIDs, item.EventID)
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return items, invalidEventIDs, err
	}
	return items, invalidEventIDs, nil
}

func releaseInvalidProbeOutboxClaim(
	ctx context.Context,
	executor probeOutboxClaimExecutor,
	workerID string,
	eventID string,
) {
	_, _ = executor.ExecContext(ctx, `
		UPDATE probe_operation_outbox
		SET attempts=attempts+1,last_error='invalid outbox JSON payload',
		    publish_state='PENDING',locked_until=NULL,locked_by=''
		WHERE event_id=$1::uuid AND published=false
		  AND publish_state='OUTCOME_UNKNOWN' AND locked_by=$2`, eventID, workerID)
}

func (h *SystemHandler) publishProbeOperationOutboxItem(ctx context.Context, workerID string, item probeOperationOutboxItem) error {
	topic, err := probeOperationEventTopic(item.EventType)
	if err != nil {
		return err
	}
	publish := h.probeEventPublish
	if topic == "probe.control.v2" {
		publish = h.probeCommandPublish
	}
	if publish == nil {
		return fmt.Errorf("publisher for %s is unavailable", item.EventType)
	}
	var envelope struct {
		EventID         string `json:"event_id"`
		EventType       string `json:"event_type"`
		TenantID        string `json:"tenant_id"`
		ProbeID         string `json:"probe_id"`
		OperationID     string `json:"operation_id"`
		CommandRevision int64  `json:"command_revision"`
		Revision        int64  `json:"revision"`
		SchemaVersion   int    `json:"schema_version"`
	}
	if err := json.Unmarshal(item.Payload, &envelope); err != nil {
		return fmt.Errorf("decode probe outbox envelope: %w", err)
	}
	if envelope.EventID != item.EventID || envelope.EventType != item.EventType ||
		envelope.TenantID != item.TenantID || envelope.OperationID != item.OperationID ||
		envelope.SchemaVersion != item.SchemaVersion || strings.TrimSpace(envelope.ProbeID) == "" ||
		envelope.CommandRevision <= 0 {
		return fmt.Errorf("probe outbox envelope conflicts with durable row identity")
	}
	if item.EventType == probeOperationRequestedEvent && envelope.CommandRevision != item.AggregateVersion {
		return fmt.Errorf("probe command revision conflicts with durable aggregate version")
	}
	if (item.EventType == probeOperationAcknowledgedEvent || item.EventType == probeOperationExpiredEvent) &&
		envelope.Revision != item.AggregateVersion {
		return fmt.Errorf("probe lifecycle revision conflicts with durable aggregate version")
	}
	receipt, err := publish(ctx, item.PartitionKey, item.Payload,
		commonkafka.MessageHeader{Key: "event_id", Value: item.EventID},
		commonkafka.MessageHeader{Key: "event_type", Value: item.EventType},
		commonkafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
		commonkafka.MessageHeader{Key: "probe_id", Value: envelope.ProbeID},
		commonkafka.MessageHeader{Key: "operation_id", Value: item.OperationID},
		commonkafka.MessageHeader{Key: "command_revision", Value: fmt.Sprint(envelope.CommandRevision)},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: fmt.Sprint(item.AggregateVersion)},
		commonkafka.MessageHeader{Key: "schema_version", Value: fmt.Sprint(item.SchemaVersion)},
		commonkafka.MessageHeader{Key: "target_topic", Value: topic},
		commonkafka.MessageHeader{Key: commonkafka.PublishAttemptHeader, Value: item.PublishAttempt},
	)
	if err != nil {
		var unknown *commonkafka.PublishOutcomeUnknownError
		if errors.As(err, &unknown) {
			h.markProbeOutboxOutcomeUnknown(ctx, workerID, item, unknown.Receipt, err.Error())
		} else {
			h.releaseProbeOutboxLease(ctx, workerID, item.EventID, err.Error())
		}
		return err
	}
	if receipt.AttemptID != item.PublishAttempt || receipt.Topic != topic ||
		receipt.Key != item.PartitionKey || receipt.Partition < 0 || receipt.Offset < 0 ||
		receipt.AcknowledgedAt.IsZero() {
		err := fmt.Errorf("Kafka broker receipt conflicts with probe outbox attempt identity")
		h.markProbeOutboxOutcomeUnknown(ctx, workerID, item, receipt, err.Error())
		return err
	}

	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		h.markProbeOutboxOutcomeUnknown(ctx, workerID, item, receipt, err.Error())
		return err
	}
	defer tx.Rollback()
	unknownAfterRollback := func(cause error) error {
		_ = tx.Rollback()
		h.markProbeOutboxOutcomeUnknown(ctx, workerID, item, receipt, cause.Error())
		return cause
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE probe_operation_outbox
		SET published=true,attempts=attempts+1,last_error='',published_at=now(),
		    publish_state='KAFKA_ACKED',broker_topic=$3,broker_partition=$4,
		    broker_offset=$5,acked_at=$6,locked_until=NULL,locked_by=''
		WHERE event_id=$1::uuid AND published=false AND locked_by=$2
		  AND publish_attempt=$7::uuid`,
		item.EventID, workerID, receipt.Topic, receipt.Partition,
		receipt.Offset, receipt.AcknowledgedAt, receipt.AttemptID)
	if err != nil {
		return unknownAfterRollback(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return unknownAfterRollback(err)
	}
	if affected != 1 {
		return unknownAfterRollback(fmt.Errorf("probe outbox lease lost before publish acknowledgement"))
	}
	if item.EventType == probeOperationRequestedEvent {
		var stateRevision int64
		err = tx.QueryRowContext(ctx, `
			UPDATE probe_operations
			SET status='delivered',state_revision=state_revision+1,delivered_at=now(),updated_at=now()
			WHERE operation_id=$1::uuid AND tenant_id=$2 AND status='accepted'
			RETURNING state_revision`, item.OperationID, item.TenantID).Scan(&stateRevision)
		if err != nil && err != sql.ErrNoRows {
			return unknownAfterRollback(err)
		}
		if err == nil {
			detail, _ := json.Marshal(map[string]interface{}{
				"event_id": item.EventID, "event_type": item.EventType,
				"kafka_topic": topic, "aggregate_version": item.AggregateVersion,
			})
			if _, err = tx.ExecContext(ctx, `
				INSERT INTO probe_operation_history
					(operation_id,tenant_id,state_revision,from_status,to_status,detail)
				VALUES ($1::uuid,$2,$3,'accepted','delivered',$4::jsonb)`,
				item.OperationID, item.TenantID, stateRevision, string(detail)); err != nil {
				return unknownAfterRollback(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		h.markProbeOutboxOutcomeUnknown(ctx, workerID, item, receipt, err.Error())
		return err
	}
	return nil
}

func probeOperationEventTopic(eventType string) (string, error) {
	switch eventType {
	case probeOperationRequestedEvent:
		return "probe.control.v2", nil
	case probeOperationAcknowledgedEvent, probeOperationExpiredEvent:
		return "probe.events.v2", nil
	default:
		return "", fmt.Errorf("unsupported probe operation outbox event_type %q", eventType)
	}
}

func (h *SystemHandler) releaseProbeOutboxLease(ctx context.Context, workerID, eventID, message string) {
	if h.pgDB == nil {
		return
	}
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = h.pgDB.ExecContext(ctx, `
		UPDATE probe_operation_outbox
		SET attempts=attempts+1,last_error=$2,publish_state='PENDING',
		    next_attempt_at=now()+(LEAST(300,POWER(2,LEAST(attempts+1,8)))::text || ' seconds')::interval,
		    locked_until=NULL,locked_by=''
		WHERE event_id=$1::uuid AND published=false
		  AND publish_state='OUTCOME_UNKNOWN' AND locked_by=$3`,
		eventID, message, workerID)
}

func (h *SystemHandler) markProbeOutboxOutcomeUnknown(
	ctx context.Context,
	workerID string,
	item probeOperationOutboxItem,
	receipt commonkafka.BrokerReceipt,
	message string,
) {
	if h.pgDB == nil {
		return
	}
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	var partition interface{}
	var offset interface{}
	var ackedAt interface{}
	if receipt.Partition >= 0 {
		partition = receipt.Partition
	}
	if receipt.Offset >= 0 {
		offset = receipt.Offset
	}
	if !receipt.AcknowledgedAt.IsZero() {
		ackedAt = receipt.AcknowledgedAt
	}
	_, _ = h.pgDB.ExecContext(ctx, `
		UPDATE probe_operation_outbox
		SET attempts=attempts+1,last_error=$4,publish_state='OUTCOME_UNKNOWN',
		    broker_topic=$5,broker_partition=$6,broker_offset=$7,acked_at=$8,
		    next_attempt_at=now()+(LEAST(300,POWER(2,LEAST(attempts+1,8)))::text || ' seconds')::interval,
		    locked_until=NULL,locked_by=''
		WHERE event_id=$1::uuid AND published=false AND locked_by=$2
		  AND publish_attempt=$3::uuid`,
		item.EventID, workerID, item.PublishAttempt, message,
		receipt.Topic, partition, offset, ackedAt)
}

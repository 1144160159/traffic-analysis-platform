package api

import (
	"context"
	"database/sql"
	"encoding/json"
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
)

type probeOperationOutboxItem struct {
	EventID          string
	OperationID      string
	TenantID         string
	EventType        string
	PartitionKey     string
	AggregateVersion int64
	SchemaVersion    int
	Payload          []byte
}

func (h *SystemHandler) StartProbeOperationOutboxWorker(ctx context.Context, interval time.Duration) error {
	if h.pgDB == nil {
		return fmt.Errorf("probe operation outbox database is unavailable")
	}
	if h.probeCommandPublish == nil || h.probeEventPublish == nil {
		return fmt.Errorf("probe operation Kafka publishers are unavailable")
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
	if _, err := h.pgDB.ExecContext(ctx, `WITH expired AS (
		UPDATE probe_operations
		SET status='expired',state_revision=state_revision+1,
		    ack_error='operation expired before ACK',updated_at=now()
		WHERE status IN ('accepted','delivered') AND expires_at <= now()
		RETURNING operation_id,tenant_id,state_revision,
		          CASE WHEN delivered_at IS NULL THEN 'accepted' ELSE 'delivered' END AS from_status
	)
	INSERT INTO probe_operation_history
		(operation_id,tenant_id,state_revision,from_status,to_status,detail)
	SELECT operation_id,tenant_id,state_revision,from_status,
	       'expired','{"reason":"operation expired before ACK"}'::jsonb
	FROM expired`); err != nil {
		return 0, err
	}
	rows, err := h.pgDB.QueryContext(ctx, `WITH candidates AS (
		SELECT o.event_id FROM probe_operation_outbox o
		JOIN probe_operations p ON p.operation_id=o.operation_id
		WHERE o.published=false AND o.next_attempt_at <= now()
		  AND o.event_type IN ('traffic.probe.v2.OperationRequested','traffic.probe.v2.OperationAcknowledged')
		  AND (o.event_type='traffic.probe.v2.OperationAcknowledged'
		       OR (p.status IN ('accepted','delivered') AND p.expires_at > now()))
		  AND (o.locked_until IS NULL OR o.locked_until < now())
		ORDER BY o.next_attempt_at,o.created_at,o.event_id
		LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE probe_operation_outbox o
		SET locked_until=now()+interval '60 seconds',locked_by=$2
		FROM candidates c WHERE o.event_id=c.event_id
		RETURNING o.event_id::text,o.operation_id::text,o.tenant_id,o.event_type,
		          o.partition_key,o.aggregate_version,o.schema_version,o.payload::text
	) SELECT event_id,operation_id,tenant_id,event_type,partition_key,
	         aggregate_version,schema_version,payload
	  FROM claimed ORDER BY event_id`, limit, workerID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]probeOperationOutboxItem, 0, limit)
	for rows.Next() {
		var item probeOperationOutboxItem
		var rawPayload string
		if err := rows.Scan(
			&item.EventID, &item.OperationID, &item.TenantID, &item.EventType,
			&item.PartitionKey, &item.AggregateVersion, &item.SchemaVersion, &rawPayload,
		); err != nil {
			return len(items), err
		}
		item.Payload = []byte(rawPayload)
		if !json.Valid(item.Payload) {
			h.releaseProbeOutboxLease(ctx, workerID, item.EventID, "invalid outbox JSON payload")
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return len(items), err
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

func (h *SystemHandler) publishProbeOperationOutboxItem(ctx context.Context, workerID string, item probeOperationOutboxItem) error {
	publish := h.probeEventPublish
	topic := "probe.events.v2"
	if item.EventType == probeOperationRequestedEvent {
		publish = h.probeCommandPublish
		topic = "probe.control.v2"
	}
	if publish == nil {
		return fmt.Errorf("publisher for %s is unavailable", item.EventType)
	}
	err := publish(ctx, item.PartitionKey, item.Payload,
		commonkafka.MessageHeader{Key: "event_id", Value: item.EventID},
		commonkafka.MessageHeader{Key: "event_type", Value: item.EventType},
		commonkafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
		commonkafka.MessageHeader{Key: "operation_id", Value: item.OperationID},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: fmt.Sprint(item.AggregateVersion)},
		commonkafka.MessageHeader{Key: "schema_version", Value: fmt.Sprint(item.SchemaVersion)},
		commonkafka.MessageHeader{Key: "target_topic", Value: topic},
	)
	if err != nil {
		h.releaseProbeOutboxLease(ctx, workerID, item.EventID, err.Error())
		return err
	}

	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE probe_operation_outbox
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
		return fmt.Errorf("probe outbox lease lost before publish acknowledgement")
	}
	if item.EventType == probeOperationRequestedEvent {
		var stateRevision int64
		err = tx.QueryRowContext(ctx, `
			UPDATE probe_operations
			SET status='delivered',state_revision=state_revision+1,delivered_at=now(),updated_at=now()
			WHERE operation_id=$1::uuid AND tenant_id=$2 AND status='accepted'
			RETURNING state_revision`, item.OperationID, item.TenantID).Scan(&stateRevision)
		if err != nil && err != sql.ErrNoRows {
			return err
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
				return err
			}
		}
	}
	return tx.Commit()
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
		SET attempts=attempts+1,last_error=$2,
		    next_attempt_at=now()+(LEAST(300,POWER(2,LEAST(attempts+1,8)))::text || ' seconds')::interval,
		    locked_until=NULL,locked_by=''
		WHERE event_id=$1::uuid AND published=false AND locked_by=$3`,
		eventID, message, workerID)
}

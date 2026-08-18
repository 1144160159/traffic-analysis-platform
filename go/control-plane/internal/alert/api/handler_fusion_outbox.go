package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/fusion"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"go.uber.org/zap"
)

type fusionCommandOutboxItem struct {
	EventID          string
	TenantID         string
	AggregateID      string
	AggregateVersion int64
	EventType        string
	PartitionKey     string
	Payload          []byte
	PayloadSHA256    string
	TraceID          string
	ClaimToken       string
}

// StartFusionCommandOutboxWorker publishes only source-sync commands. Snapshot
// and resolution events remain pending for their separately versioned
// downstream topics, so enabling this worker cannot route unlike payloads into
// fusion.commands.v1.
func (h *SystemHandler) StartFusionCommandOutboxWorker(ctx context.Context, interval time.Duration) error {
	if h == nil || h.pgDB == nil || h.fusionCommandPublish == nil || h.fusionReadinessGate == nil ||
		len(h.fusionCandidateSHA256) != 64 {
		return fmt.Errorf("fusion command outbox dependencies are unavailable")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := h.drainFusionCommandOutbox(ctx, 50); err != nil && ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Failed to drain fusion command outbox", zap.Error(err))
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

func (h *SystemHandler) drainFusionCommandOutbox(ctx context.Context, limit int) (int, error) {
	if h == nil || h.pgDB == nil || h.fusionCommandPublish == nil || h.fusionReadinessGate == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin fusion command outbox claim: %w", err)
	}
	defer tx.Rollback()
	if err := h.fusionReadinessGate.AssertReadyTx(ctx, tx, h.fusionCandidateSHA256); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `WITH candidates AS (
		SELECT event_id FROM fusion_projection_outbox
		WHERE event_type=$1 AND next_attempt_at<=now()
		  AND (publish_state='PENDING' OR (publish_state='OUTCOME_UNKNOWN' AND claimed_at<now()-interval '30 seconds'))
		ORDER BY next_attempt_at,created_at,event_id LIMIT $2 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE fusion_projection_outbox o SET publish_state='OUTCOME_UNKNOWN',claim_token=uuid_generate_v4(),
			claimed_at=now(),attempts=attempts+1,next_attempt_at=now()+interval '30 seconds',last_error=''
		FROM candidates c WHERE o.event_id=c.event_id
		RETURNING o.event_id::text,o.tenant_id,o.aggregate_id::text,o.aggregate_version,o.event_type,
			o.partition_key,o.payload::text,o.payload_sha256,o.trace_id,o.claim_token::text
	) SELECT event_id,tenant_id,aggregate_id,aggregate_version,event_type,partition_key,payload,
		payload_sha256,trace_id,claim_token FROM claimed ORDER BY event_id`, fusion.SourceSyncEventType, limit)
	if err != nil {
		return 0, fmt.Errorf("claim fusion command outbox: %w", err)
	}
	items := make([]fusionCommandOutboxItem, 0, limit)
	for rows.Next() {
		var item fusionCommandOutboxItem
		var payload string
		if err := rows.Scan(&item.EventID, &item.TenantID, &item.AggregateID, &item.AggregateVersion,
			&item.EventType, &item.PartitionKey, &payload, &item.PayloadSHA256, &item.TraceID, &item.ClaimToken); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan fusion command outbox: %w", err)
		}
		item.Payload = []byte(payload)
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close fusion command outbox rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit fusion command outbox claims: %w", err)
	}
	processed := 0
	for _, item := range items {
		if err := h.publishFusionCommandOutboxItem(ctx, item); err != nil {
			if h.logger != nil {
				h.logger.Warn("Fusion command outbox delivery failed", zap.String("event_id", item.EventID),
					zap.String("job_id", item.AggregateID), zap.Error(err))
			}
			continue
		}
		processed++
	}
	return processed, nil
}

func (h *SystemHandler) publishFusionCommandOutboxItem(ctx context.Context, item fusionCommandOutboxItem) error {
	if h == nil || h.fusionCommandPublish == nil {
		return fmt.Errorf("fusion command Kafka publisher is unavailable")
	}
	payloadHash := fmt.Sprintf("%x", sha256.Sum256(item.Payload))
	var command fusion.SourceSyncCommand
	if payloadHash != item.PayloadSHA256 || len(item.ClaimToken) == 0 ||
		json.Unmarshal(item.Payload, &command) != nil || command.Validate() != nil ||
		command.EventID != item.EventID || command.JobID != item.AggregateID ||
		command.TenantID != item.TenantID || command.AggregateVersion != item.AggregateVersion ||
		command.EventType != item.EventType || command.PartitionKey != item.PartitionKey ||
		command.TraceID != item.TraceID {
		h.releaseFusionCommandClaim(ctx, item, "INVALID_OUTBOX_PAYLOAD")
		return fmt.Errorf("fusion command outbox payload identity is invalid")
	}
	receipt, err := h.fusionCommandPublish.Send(ctx, item.PartitionKey, item.Payload,
		commonkafka.MessageHeader{Key: "event_id", Value: item.EventID},
		commonkafka.MessageHeader{Key: "event_type", Value: item.EventType},
		commonkafka.MessageHeader{Key: "schema_version", Value: "1"},
		commonkafka.MessageHeader{Key: "aggregate_type", Value: "source_sync_job"},
		commonkafka.MessageHeader{Key: "aggregate_id", Value: item.AggregateID},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: fmt.Sprint(item.AggregateVersion)},
		commonkafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
		commonkafka.MessageHeader{Key: "job_id", Value: item.AggregateID},
		commonkafka.MessageHeader{Key: "source_id", Value: command.SourceID},
		commonkafka.MessageHeader{Key: "trace_id", Value: item.TraceID},
		commonkafka.MessageHeader{Key: "target_topic", Value: fusion.SourceSyncTopic},
		commonkafka.MessageHeader{Key: commonkafka.PublishAttemptHeader, Value: item.ClaimToken},
	)
	if err != nil {
		var outcomeUnknown *commonkafka.PublishOutcomeUnknownError
		if !errors.As(err, &outcomeUnknown) {
			h.releaseFusionCommandClaim(ctx, item, err.Error())
		}
		return err
	}
	if receipt.AttemptID != item.ClaimToken || receipt.Topic != fusion.SourceSyncTopic ||
		receipt.Partition < 0 || receipt.Offset < 0 || receipt.Key != item.PartitionKey || receipt.AcknowledgedAt.IsZero() {
		return fmt.Errorf("fusion command broker receipt identity is invalid")
	}
	result, err := h.pgDB.ExecContext(ctx, `UPDATE fusion_projection_outbox SET publish_state='KAFKA_ACKED',
		broker_topic=$1,broker_partition=$2,broker_offset=$3,acked_at=$4,claim_token=NULL,claimed_at=NULL,
		next_attempt_at=now(),last_error=''
		WHERE event_id=$5 AND publish_state='OUTCOME_UNKNOWN' AND claim_token=$6`, receipt.Topic,
		receipt.Partition, receipt.Offset, receipt.AcknowledgedAt, item.EventID, item.ClaimToken)
	if err != nil {
		return fmt.Errorf("record fusion command broker acknowledgement: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("fusion command claim was lost before broker acknowledgement")
	}
	return nil
}

func (h *SystemHandler) releaseFusionCommandClaim(ctx context.Context, item fusionCommandOutboxItem, message string) {
	if h == nil || h.pgDB == nil {
		return
	}
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = h.pgDB.ExecContext(ctx, `UPDATE fusion_projection_outbox SET publish_state='PENDING',
		claim_token=NULL,claimed_at=NULL,next_attempt_at=now()+interval '10 seconds',last_error=$1
		WHERE event_id=$2 AND publish_state='OUTCOME_UNKNOWN' AND claim_token=$3`, message, item.EventID, item.ClaimToken)
}

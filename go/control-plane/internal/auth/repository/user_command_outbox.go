package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

type userCommandOutboxItem struct {
	OutboxID         int64
	EventID          string
	TenantID         string
	UserID           string
	AggregateVersion int64
	EventType        string
	SchemaVersion    int
	PartitionKey     string
	Payload          []byte
}

func (w *UserSettingsOutboxWorker) DrainUserCommands(ctx context.Context, limit int) (int, error) {
	if w == nil || w.db == nil || w.producer == nil {
		return 0, fmt.Errorf("user command outbox worker is unavailable")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := w.db.QueryContext(ctx, `WITH candidates AS (
		SELECT outbox_id FROM user_command_outbox
		WHERE status IN ('pending','processing') AND publish_attempts<$3 AND next_retry_at<=now()
		  AND (locked_until IS NULL OR locked_until<now())
		ORDER BY next_retry_at,occurred_at,outbox_id LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE user_command_outbox o SET status='processing',locked_until=now()+interval '60 seconds',locked_by=$2
		FROM candidates c WHERE o.outbox_id=c.outbox_id
		RETURNING o.outbox_id,o.event_id::text,o.tenant_id,o.user_id::text,o.aggregate_version,
		          o.event_type,o.schema_version,o.partition_key,o.payload::text
	) SELECT outbox_id,event_id,tenant_id,user_id,aggregate_version,event_type,schema_version,partition_key,payload
	  FROM claimed ORDER BY outbox_id`, limit, w.workerID, userSettingsOutboxMaxAttempts)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]userCommandOutboxItem, 0, limit)
	for rows.Next() {
		var item userCommandOutboxItem
		var payload string
		if err := rows.Scan(&item.OutboxID, &item.EventID, &item.TenantID, &item.UserID,
			&item.AggregateVersion, &item.EventType, &item.SchemaVersion, &item.PartitionKey, &payload); err != nil {
			return len(items), err
		}
		item.Payload = []byte(payload)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return len(items), err
	}
	processed := 0
	for _, item := range items {
		if err := w.publishUserCommand(ctx, item); err != nil {
			w.logger.Warn("User command event delivery failed")
			continue
		}
		processed++
	}
	return processed, nil
}

func (w *UserSettingsOutboxWorker) publishUserCommand(ctx context.Context, item userCommandOutboxItem) error {
	if !strings.HasPrefix(item.EventType, "traffic.user.command.v1.") || item.AggregateVersion < 1 || item.SchemaVersion != 1 {
		err := fmt.Errorf("invalid user command outbox envelope")
		w.failUserCommand(ctx, item.OutboxID, err.Error())
		return err
	}
	var event pb.UserEvent
	if err := json.Unmarshal(item.Payload, &event); err != nil {
		w.failUserCommand(ctx, item.OutboxID, "invalid UserEvent payload: "+err.Error())
		return err
	}
	if event.EventId != item.EventID || event.TenantId != item.TenantID || event.UserId != item.UserID ||
		event.EventType == "" || event.Timestamp <= 0 || item.PartitionKey != item.TenantID+":"+item.UserID {
		err := fmt.Errorf("user command UserEvent identity mismatch")
		w.failUserCommand(ctx, item.OutboxID, err.Error())
		return err
	}
	if err := w.producer.SendProto(ctx, item.PartitionKey, &event,
		commonkafka.MessageHeader{Key: "event_id", Value: item.EventID},
		commonkafka.MessageHeader{Key: "event_type", Value: item.EventType},
		commonkafka.MessageHeader{Key: "schema_version", Value: fmt.Sprint(item.SchemaVersion)},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: fmt.Sprint(item.AggregateVersion)},
		commonkafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
	); err != nil {
		w.failUserCommand(ctx, item.OutboxID, err.Error())
		return err
	}
	result, err := w.db.ExecContext(ctx, `UPDATE user_command_outbox
		SET status='published',publish_attempts=publish_attempts+1,last_error='',published_at=now(),locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`, item.OutboxID, w.workerID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("user command outbox lease lost after Kafka acknowledgement")
	}
	return nil
}

func (w *UserSettingsOutboxWorker) failUserCommand(ctx context.Context, outboxID int64, message string) {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = w.db.ExecContext(ctx, `UPDATE user_command_outbox
		SET publish_attempts=publish_attempts+1,last_error=$2,
		    status=CASE WHEN publish_attempts+1 >= $4 THEN 'dead' ELSE 'pending' END,
		    next_retry_at=now()+(LEAST(300,POWER(2,LEAST(publish_attempts+1,8)))::text || ' seconds')::interval,
		    locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$3`, outboxID, message, w.workerID, userSettingsOutboxMaxAttempts)
}

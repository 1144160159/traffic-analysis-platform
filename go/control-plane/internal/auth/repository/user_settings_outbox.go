package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

const userSettingsOutboxMaxAttempts = 10

type UserSettingsEventProducer interface {
	SendProto(context.Context, string, proto.Message, ...commonkafka.MessageHeader) error
}

type UserSettingsOutboxWorker struct {
	db       *sql.DB
	producer UserSettingsEventProducer
	logger   *zap.Logger
	workerID string
}

type userSettingsOutboxItem struct {
	OutboxID         int64
	EventID          string
	TenantID         string
	UserID           string
	Category         string
	AggregateVersion int64
	EventType        string
	SchemaVersion    int
	PartitionKey     string
	Payload          []byte
}

func NewUserSettingsOutboxWorker(db *sql.DB, producer UserSettingsEventProducer, logger *zap.Logger) *UserSettingsOutboxWorker {
	if logger == nil {
		logger = zap.NewNop()
	}
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "auth-service"
	}
	return &UserSettingsOutboxWorker{
		db: db, producer: producer, logger: logger,
		workerID: hostname + ":user-settings-outbox:" + uuid.NewString(),
	}
}

// Run blocks until ctx is cancelled and continuously reclaims or publishes pending rows.
func (w *UserSettingsOutboxWorker) Run(ctx context.Context, interval time.Duration) error {
	if w == nil || w.db == nil {
		return fmt.Errorf("user settings outbox database is unavailable")
	}
	if w.producer == nil {
		return fmt.Errorf("user settings event producer is unavailable")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := w.Drain(ctx, 50); err != nil && ctx.Err() == nil {
			w.logger.Warn("Failed to drain user settings outbox", zap.Error(err))
		}
		if _, err := w.DrainUserCommands(ctx, 50); err != nil && ctx.Err() == nil {
			w.logger.Warn("Failed to drain user command outbox", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *UserSettingsOutboxWorker) Drain(ctx context.Context, limit int) (int, error) {
	if w == nil || w.db == nil || w.producer == nil {
		return 0, fmt.Errorf("user settings outbox worker is unavailable")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := w.db.QueryContext(ctx, `WITH candidates AS (
		SELECT outbox_id FROM user_settings_outbox
		WHERE status IN ('pending','processing') AND publish_attempts<$3 AND next_retry_at<=now()
		  AND (locked_until IS NULL OR locked_until<now())
		ORDER BY next_retry_at,occurred_at,outbox_id LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE user_settings_outbox o SET status='processing',locked_until=now()+interval '60 seconds',locked_by=$2
		FROM candidates c WHERE o.outbox_id=c.outbox_id
		RETURNING o.outbox_id,o.event_id::text,o.tenant_id,o.user_id::text,o.category,o.aggregate_version,
		          o.event_type,o.schema_version,o.partition_key,o.payload::text
	) SELECT outbox_id,event_id,tenant_id,user_id,category,aggregate_version,event_type,schema_version,partition_key,payload
	  FROM claimed ORDER BY outbox_id`, limit, w.workerID, userSettingsOutboxMaxAttempts)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]userSettingsOutboxItem, 0, limit)
	for rows.Next() {
		var item userSettingsOutboxItem
		var payload string
		if err := rows.Scan(&item.OutboxID, &item.EventID, &item.TenantID, &item.UserID, &item.Category,
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
		if err := w.publish(ctx, item); err != nil {
			w.logger.Warn("User settings event delivery failed", zap.String("event_id", item.EventID), zap.Error(err))
			continue
		}
		processed++
	}
	return processed, nil
}

func (w *UserSettingsOutboxWorker) publish(ctx context.Context, item userSettingsOutboxItem) error {
	if item.EventType != "traffic.user.settings.v1.SettingsUpdated" || item.AggregateVersion < 1 || item.SchemaVersion != 1 {
		err := fmt.Errorf("invalid user settings outbox envelope")
		w.fail(ctx, item.OutboxID, err.Error())
		return err
	}
	var event pb.UserEvent
	if err := json.Unmarshal(item.Payload, &event); err != nil {
		w.fail(ctx, item.OutboxID, "invalid UserEvent payload: "+err.Error())
		return err
	}
	if event.EventId != item.EventID || event.TenantId != item.TenantID || event.UserId != item.UserID ||
		event.EventType != "settings_update" || event.Timestamp <= 0 {
		err := fmt.Errorf("user settings UserEvent identity mismatch")
		w.fail(ctx, item.OutboxID, err.Error())
		return err
	}
	if err := w.producer.SendProto(ctx, item.PartitionKey, &event,
		commonkafka.MessageHeader{Key: "event_id", Value: item.EventID},
		commonkafka.MessageHeader{Key: "event_type", Value: item.EventType},
		commonkafka.MessageHeader{Key: "schema_version", Value: fmt.Sprint(item.SchemaVersion)},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: fmt.Sprint(item.AggregateVersion)},
		commonkafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
	); err != nil {
		w.fail(ctx, item.OutboxID, err.Error())
		return err
	}
	result, err := w.db.ExecContext(ctx, `UPDATE user_settings_outbox
		SET status='published',publish_attempts=publish_attempts+1,last_error='',published_at=now(),locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`, item.OutboxID, w.workerID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("user settings outbox lease lost after Kafka acknowledgement")
	}
	return nil
}

func (w *UserSettingsOutboxWorker) fail(ctx context.Context, outboxID int64, message string) {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = w.db.ExecContext(ctx, `UPDATE user_settings_outbox
		SET publish_attempts=publish_attempts+1,last_error=$2,
		    status=CASE WHEN publish_attempts+1 >= $4 THEN 'dead' ELSE 'pending' END,
		    next_retry_at=now()+(LEAST(300,POWER(2,LEAST(publish_attempts+1,8)))::text || ' seconds')::interval,
		    locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$3`, outboxID, message, w.workerID, userSettingsOutboxMaxAttempts)
}

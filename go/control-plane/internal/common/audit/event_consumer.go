package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// KafkaEventType 事件消费者支持的消息类型
type KafkaEventType string

const (
	KafkaEventUser       KafkaEventType = "user_event"
	KafkaEventDevice     KafkaEventType = "device_log"
	KafkaEventDeadLetter KafkaEventType = "dead_letter"
)

// EventConsumer 多类型事件消费者 — UserEvent/DeviceLog/DeadLetter → ClickHouse
type EventConsumer struct {
	kafkaConsumer *kafka.Consumer
	chClient      *storage.ClickHouseClient
	logger        *zap.Logger
	eventType     KafkaEventType
	topic         string
	groupID       string
	batchSize     int
	flushInterval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewEventConsumer(
	kc *kafka.Consumer, ch *storage.ClickHouseClient, logger *zap.Logger,
	et KafkaEventType, topic, groupID string,
) *EventConsumer {
	ctx, cancel := context.WithCancel(context.Background())
	return &EventConsumer{
		kafkaConsumer: kc, chClient: ch, logger: logger,
		eventType: et, topic: topic, groupID: groupID,
		batchSize: 200, flushInterval: 3 * time.Second,
		ctx: ctx, cancel: cancel,
	}
}

func (c *EventConsumer) Start(ctx context.Context) error {
	c.logger.Info("Event consumer starting", zap.String("type", string(c.eventType)), zap.String("topic", c.topic))
	return c.kafkaConsumer.BatchConsume(ctx, c.batchSize, c.flushInterval, c.handleBatch)
}

func (c *EventConsumer) StartAsync(ctx context.Context) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.Start(ctx); err != nil && err != context.Canceled {
			c.logger.Error("Event consumer error", zap.String("type", string(c.eventType)), zap.Error(err))
		}
	}()
}

func (c *EventConsumer) Stop() { c.cancel(); c.wg.Wait() }

func (c *EventConsumer) handleBatch(ctx context.Context, msgs []*kafka.ReceivedMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	switch c.eventType {
	case KafkaEventUser:
		return c.handleUserEvents(ctx, msgs)
	case KafkaEventDevice:
		return c.handleDeviceLogs(ctx, msgs)
	case KafkaEventDeadLetter:
		return c.handleDeadLetters(ctx, msgs)
	default:
		return fmt.Errorf("unknown event type: %s", c.eventType)
	}
}

// ---- UserEvent ----

func (c *EventConsumer) handleUserEvents(ctx context.Context, msgs []*kafka.ReceivedMessage) error {
	users := make([]*pb.UserEvent, 0, len(msgs))
	for _, msg := range msgs {
		var batch pb.UserEventBatch
		if err := proto.Unmarshal(msg.Value, &batch); err == nil && len(batch.Events) > 0 {
			users = append(users, batch.Events...)
			continue
		}
		var ue pb.UserEvent
		if err := proto.Unmarshal(msg.Value, &ue); err == nil && ue.EventId != "" {
			users = append(users, &ue)
			continue
		}
	}
	if len(users) == 0 {
		return nil
	}
	return c.chClient.BatchInsert(ctx,
		`INSERT INTO traffic.user_events (event_id, tenant_id, user_id, username, event_type, source_ip, user_agent, resource, action, result, timestamp)`,
		func(batch driver.Batch) error {
			for _, u := range users {
				if err := batch.Append(u.EventId, u.TenantId, u.UserId, u.Username,
					u.EventType, u.SourceIp, u.UserAgent, u.Resource, u.Action, u.Result,
					time.UnixMilli(u.Timestamp)); err != nil {
					return err
				}
			}
			return nil
		})
}

// ---- DeviceLog ----

func (c *EventConsumer) handleDeviceLogs(ctx context.Context, msgs []*kafka.ReceivedMessage) error {
	logs := make([]*pb.DeviceLog, 0, len(msgs))
	for _, msg := range msgs {
		var batch pb.DeviceLogBatch
		if err := proto.Unmarshal(msg.Value, &batch); err == nil && len(batch.Events) > 0 {
			logs = append(logs, batch.Events...)
			continue
		}
		var dl pb.DeviceLog
		if err := proto.Unmarshal(msg.Value, &dl); err == nil && dl.LogId != "" {
			logs = append(logs, &dl)
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal(msg.Value, &raw); err == nil {
			if dl := jsonToDeviceLog(raw); dl != nil {
				logs = append(logs, dl)
			}
		}
	}
	if len(logs) == 0 {
		return nil
	}
	return c.chClient.BatchInsert(ctx,
		`INSERT INTO traffic.device_logs (log_id, tenant_id, device_ip, device_type, facility, severity, timestamp, message, parsed, source)`,
		func(batch driver.Batch) error {
			for _, l := range logs {
				if err := batch.Append(l.LogId, l.TenantId, l.DeviceIp, l.DeviceType,
					l.Facility, l.Severity, time.UnixMilli(l.Timestamp), l.Message, l.Parsed, l.Source); err != nil {
					return err
				}
			}
			return nil
		})
}

// ---- DeadLetter ----

func (c *EventConsumer) handleDeadLetters(ctx context.Context, msgs []*kafka.ReceivedMessage) error {
	items := make([]*pb.DeadLetter, 0, len(msgs))
	for _, msg := range msgs {
		var batch pb.DeadLetterBatch
		if err := proto.Unmarshal(msg.Value, &batch); err == nil && len(batch.Events) > 0 {
			items = append(items, batch.Events...)
			continue
		}
		var dl pb.DeadLetter
		if err := proto.Unmarshal(msg.Value, &dl); err == nil && dl.EventId != "" {
			items = append(items, &dl)
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal(msg.Value, &raw); err == nil {
			if dl := jsonToDeadLetter(raw); dl != nil {
				items = append(items, dl)
			}
		}
	}
	if len(items) == 0 {
		return nil
	}
	return c.chClient.BatchInsert(ctx,
		`INSERT INTO traffic.dlq_events (event_id, tenant_id, source_topic, source_key, error_msg, raw_payload, retry_count, created_at)`,
		func(batch driver.Batch) error {
			for _, d := range items {
				if err := batch.Append(d.EventId, d.TenantId, d.SourceTopic, d.SourceKey,
					d.ErrorMsg, d.RawPayload, d.RetryCount, time.UnixMilli(d.CreatedAt)); err != nil {
					return err
				}
			}
			return nil
		})
}

// ---- JSON 兼容 ----

func jsonToDeviceLog(raw map[string]interface{}) *pb.DeviceLog {
	s := func(k string) string { if v, ok := raw[k].(string); ok { return v }; return "" }
	u := func(k string) uint32 { if v, ok := raw[k].(float64); ok { return uint32(v) }; return 0 }
	i := func(k string) int64 { if v, ok := raw[k].(float64); ok { return int64(v) }; return 0 }
	lid := s("log_id")
	if lid == "" {
		return nil
	}
	return &pb.DeviceLog{LogId: lid, TenantId: s("tenant_id"), DeviceIp: s("device_ip"),
		DeviceType: s("device_type"), Facility: u("facility"), Severity: u("severity"),
		Timestamp: i("timestamp"), Message: s("message"), Parsed: s("parsed"), Source: s("source")}
}

func jsonToDeadLetter(raw map[string]interface{}) *pb.DeadLetter {
	s := func(k string) string { if v, ok := raw[k].(string); ok { return v }; return "" }
	u := func(k string) uint32 { if v, ok := raw[k].(float64); ok { return uint32(v) }; return 0 }
	i := func(k string) int64 { if v, ok := raw[k].(float64); ok { return int64(v) }; return 0 }
	eid := s("event_id")
	if eid == "" {
		return nil
	}
	return &pb.DeadLetter{EventId: eid, TenantId: s("tenant_id"), SourceTopic: s("source_topic"),
		SourceKey: s("source_key"), ErrorMsg: s("error_msg"), RawPayload: s("raw_payload"),
		RetryCount: u("retry_count"), CreatedAt: i("created_at")}
}

// ---- DDL ----

func InitEventSchemas(ctx context.Context, db *sql.DB) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS traffic.user_events (
			event_id String, tenant_id String, user_id String, username String,
			event_type String, source_ip String, user_agent String,
			resource String, action String, result String, timestamp DateTime64(3)
		) ENGINE = MergeTree() ORDER BY (tenant_id, timestamp) TTL timestamp + INTERVAL 180 DAY`,
		`CREATE TABLE IF NOT EXISTS traffic.device_logs (
			log_id String, tenant_id String, device_ip String, device_type String,
			facility UInt32, severity UInt32, timestamp DateTime64(3),
			message String, parsed String, source String
		) ENGINE = MergeTree() ORDER BY (tenant_id, device_ip, timestamp) TTL timestamp + INTERVAL 30 DAY`,
		`CREATE TABLE IF NOT EXISTS traffic.dlq_events (
			event_id String, tenant_id String, source_topic String, source_key String,
			error_msg String, raw_payload String, retry_count UInt32, created_at DateTime64(3)
		) ENGINE = MergeTree() ORDER BY (tenant_id, created_at) TTL created_at + INTERVAL 168 DAY`,
	} {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("init event schema: %w", err)
		}
	}
	return nil
}

package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// ConsumerConfig 审计日志消费者配置
type ConsumerConfig struct {
	Brokers       []string
	Topic         string
	GroupID       string
	BatchSize     int
	FlushInterval time.Duration
	EnableDLQ     bool
}

// DefaultConsumerConfig 返回默认配置
func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{
		Topic:         "audit.logs",
		GroupID:       "audit-consumer",
		BatchSize:     200,
		FlushInterval: 3 * time.Second,
		EnableDLQ:     true,
	}
}

// Consumer 审计日志消费者 — 从 Kafka 消费并写入 PostgreSQL
type Consumer struct {
	kafkaConsumer *kafka.Consumer
	db            *sql.DB
	logger        *zap.Logger
	topic         string
	groupID       string
	batchSize     int
	flushInterval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewConsumer 创建审计日志消费者
func NewConsumer(kc *kafka.Consumer, db *sql.DB, logger *zap.Logger, topic, groupID string) *Consumer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Consumer{
		kafkaConsumer: kc,
		db:            db,
		logger:        logger,
		topic:         topic,
		groupID:       groupID,
		batchSize:     200,
		flushInterval: 3 * time.Second,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start 启动消费循环（阻塞）
func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("Audit log consumer starting",
		zap.String("topic", c.topic),
		zap.String("group_id", c.groupID))

	if err := c.initSchema(ctx); err != nil {
		return fmt.Errorf("init audit schema: %w", err)
	}

	return c.kafkaConsumer.BatchConsume(ctx, c.batchSize, c.flushInterval, c.handleBatch)
}

// StartAsync 异步启动
func (c *Consumer) StartAsync(ctx context.Context) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.Start(ctx); err != nil && err != context.Canceled {
			c.logger.Error("Audit consumer stopped with error", zap.Error(err))
		}
	}()
}

// Stop 优雅停止
func (c *Consumer) Stop() {
	c.logger.Info("Stopping audit log consumer")
	c.cancel()
	c.wg.Wait()
	c.logger.Info("Audit log consumer stopped")
}

// initSchema 初始化审计日志表
func (c *Consumer) initSchema(ctx context.Context) error {
	ddl := `
	CREATE TABLE IF NOT EXISTS audit_logs (
		id          BIGSERIAL PRIMARY KEY,
		event_id    TEXT NOT NULL UNIQUE,
		tenant_id   TEXT NOT NULL,
		user_id     TEXT,
		action      TEXT NOT NULL,
		object_type TEXT,
		object_id   TEXT,
		detail      JSONB,
		ip_addr     TEXT,
		user_agent  TEXT,
		created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_audit_tenant ON audit_logs(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_logs(tenant_id, user_id);
	CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at);
	`
	_, err := c.db.ExecContext(ctx, ddl)
	return err
}

// handleBatch 批量处理审计日志消息
func (c *Consumer) handleBatch(ctx context.Context, messages []*kafka.ReceivedMessage) error {
	if len(messages) == 0 {
		return nil
	}

	// 按租户分组，批量 INSERT
	type auditEntry struct {
		eventID    string
		tenantID   string
		userID     string
		action     string
		objectType string
		objectID   string
		detail     string
		ipAddr     string
		userAgent  string
		createdAt  int64
	}

	entries := make([]auditEntry, 0, len(messages))
	for _, msg := range messages {
		entry, err := c.parseMessage(msg)
		if err != nil {
			c.logger.Warn("Failed to parse audit message, skipping",
				zap.Int64("offset", msg.Offset),
				zap.Error(err))
			continue
		}
		if entry != nil {
			entries = append(entries, *entry)
		}
	}

	if len(entries) == 0 {
		return nil
	}

	// 批量插入（使用事务）
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (event_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, e := range entries {
		ts := time.UnixMilli(e.createdAt)
		if e.createdAt == 0 {
			ts = time.Now()
		}
		if _, err := stmt.ExecContext(ctx,
			e.eventID, e.tenantID, e.userID, e.action,
			e.objectType, e.objectID, e.detail, e.ipAddr, e.userAgent, ts,
		); err != nil {
			c.logger.Warn("Failed to insert audit log",
				zap.String("event_id", e.eventID),
				zap.Error(err))
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	c.logger.Debug("Audit batch committed",
		zap.Int("messages", len(messages)),
		zap.Int("inserted", len(entries)))
	return nil
}

// parseMessage 解析审计日志消息（支持 AuditLog 和 AuditLogBatch 格式）
func (c *Consumer) parseMessage(msg *kafka.ReceivedMessage) (*struct {
	eventID, tenantID, userID, action, objectType, objectID, detail, ipAddr, userAgent string
	createdAt                                                                           int64
}, error) {
	// 尝试 AuditLogBatch
	var batch pb.AuditLogBatch
	if err := proto.Unmarshal(msg.Value, &batch); err == nil && len(batch.Events) > 0 {
		e := batch.Events[0]
		return &struct {
			eventID, tenantID, userID, action, objectType, objectID, detail, ipAddr, userAgent string
			createdAt                                                                           int64
		}{
			eventID:    e.EventId,
			tenantID:   e.TenantId,
			userID:     e.UserId,
			action:     e.Action,
			objectType: e.ObjectType,
			objectID:   e.ObjectId,
			detail:     e.Detail,
			ipAddr:     e.IpAddr,
			userAgent:  e.UserAgent,
			createdAt:  e.CreatedAt,
		}, nil
	}

	// 尝试单个 AuditLog
	var single pb.AuditLog
	if err := proto.Unmarshal(msg.Value, &single); err == nil && single.EventId != "" {
		return &struct {
			eventID, tenantID, userID, action, objectType, objectID, detail, ipAddr, userAgent string
			createdAt                                                                           int64
		}{
			eventID:    single.EventId,
			tenantID:   single.TenantId,
			userID:     single.UserId,
			action:     single.Action,
			objectType: single.ObjectType,
			objectID:   single.ObjectId,
			detail:     single.Detail,
			ipAddr:     single.IpAddr,
			userAgent:  single.UserAgent,
			createdAt:  single.CreatedAt,
		}, nil
	}

	// 尝试 JSON 格式（兼容性）
	var raw map[string]interface{}
	if err := json.Unmarshal(msg.Value, &raw); err == nil {
		getStr := func(k string) string {
			if v, ok := raw[k]; ok {
				if s, ok := v.(string); ok {
					return s
				}
			}
			return ""
		}
		eid := getStr("event_id")
		if eid == "" {
			eid = getStr("log_id")
		}
		if eid == "" {
			return nil, fmt.Errorf("unknown audit format")
		}
		return &struct {
			eventID, tenantID, userID, action, objectType, objectID, detail, ipAddr, userAgent string
			createdAt                                                                           int64
		}{
			eventID:    eid,
			tenantID:   getStr("tenant_id"),
			userID:     getStr("user_id"),
			action:     getStr("action"),
			objectType: getStr("object_type"),
			objectID:   getStr("object_id"),
			detail:     getStr("detail"),
			ipAddr:     getStr("ip_addr"),
			userAgent:  getStr("user_agent"),
			createdAt:  0, // JSON uses created_at as string
		}, nil
	}

	return nil, fmt.Errorf("unmarshal audit message: unknown format")
}

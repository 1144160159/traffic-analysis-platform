package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

var ErrAuditEventIdentityCollision = errors.New("audit event_id payload collision")

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
	ready  atomic.Bool
}

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
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	if c.ctx != nil {
		go func() {
			select {
			case <-c.ctx.Done():
				runCancel()
			case <-runCtx.Done():
			}
		}()
	}
	c.logger.Info("Audit log consumer starting",
		zap.String("topic", c.topic),
		zap.String("group_id", c.groupID))

	if err := c.verifySchema(runCtx); err != nil {
		return fmt.Errorf("verify audit schema: %w", err)
	}
	c.ready.Store(true)
	defer c.ready.Store(false)

	return c.kafkaConsumer.Consume(runCtx, c.handleMessageWithReadiness)
}

// Ready reports whether the versioned PostgreSQL schema has been verified and
// the consumer loop is active. It deliberately becomes false again when the
// loop exits so Kubernetes readiness cannot hide a stopped materializer.
func (c *Consumer) Ready() bool {
	return c.ready.Load()
}

func (c *Consumer) handleBatchWithReadiness(ctx context.Context, messages []*kafka.ReceivedMessage) error {
	err := c.handleBatch(ctx, messages)
	c.ready.Store(err == nil || kafka.IsPermanent(err))
	return err
}

func (c *Consumer) handleMessageWithReadiness(ctx context.Context, message *kafka.ReceivedMessage) error {
	err := c.handleMessage(ctx, message)
	// A permanent payload error is handled by the shared consumer's durable DLQ
	// barrier. Retryable PG failures withdraw readiness until a later success.
	c.ready.Store(err == nil || kafka.IsPermanent(err))
	return err
}

func (c *Consumer) handleMessage(ctx context.Context, message *kafka.ReceivedMessage) error {
	entries, err := c.parseMessages(message)
	if err != nil {
		return kafka.Permanent(fmt.Errorf(
			"parse audit message partition=%d offset=%d: %w",
			message.Partition,
			message.Offset,
			err,
		))
	}
	err = c.persistEntries(ctx, entries)
	if errors.Is(err, ErrAuditEventIdentityCollision) {
		return kafka.Permanent(err)
	}
	return err
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

type auditSchemaColumn struct {
	dataType string
	nullable bool
}

var requiredAuditSchema = map[string]auditSchemaColumn{
	"event_id":    {dataType: "text", nullable: false},
	"tenant_id":   {dataType: "text", nullable: false},
	"user_id":     {dataType: "text", nullable: true},
	"action":      {dataType: "text", nullable: false},
	"object_type": {dataType: "text", nullable: false},
	"object_id":   {dataType: "text", nullable: true},
	"detail":      {dataType: "jsonb", nullable: false},
	"ip_addr":     {dataType: "text", nullable: true},
	"user_agent":  {dataType: "text", nullable: true},
	"created_at":  {dataType: "timestamp with time zone", nullable: false},
}

// verifySchema treats versioned migrations as the only schema authority.
// Startup may prove required capabilities, but must never create or alter a
// production table as a side effect of starting a consumer.
func (c *Consumer) verifySchema(ctx context.Context) error {
	rows, err := c.db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable
		  FROM information_schema.columns
		 WHERE table_schema = current_schema()
		   AND table_name = 'audit_logs'`)
	if err != nil {
		return fmt.Errorf("read audit_logs columns: %w", err)
	}
	defer rows.Close()

	observed := make(map[string]auditSchemaColumn)
	for rows.Next() {
		var name, dataType, nullable string
		if err := rows.Scan(&name, &dataType, &nullable); err != nil {
			return fmt.Errorf("scan audit_logs column: %w", err)
		}
		observed[name] = auditSchemaColumn{
			dataType: dataType,
			nullable: strings.EqualFold(nullable, "YES"),
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate audit_logs columns: %w", err)
	}

	for name, expected := range requiredAuditSchema {
		actual, ok := observed[name]
		if !ok {
			return fmt.Errorf("audit_logs schema is below the required migration: missing column %s", name)
		}
		if actual != expected {
			return fmt.Errorf(
				"audit_logs column %s mismatch: type=%s nullable=%t, require type=%s nullable=%t",
				name, actual.dataType, actual.nullable, expected.dataType, expected.nullable,
			)
		}
	}

	var eventIDUnique bool
	if err := c.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM pg_index i
			  JOIN pg_class t ON t.oid = i.indrelid
			  JOIN pg_namespace n ON n.oid = t.relnamespace
			 WHERE n.nspname = current_schema()
			   AND t.relname = 'audit_logs'
			   AND i.indisunique
			   AND pg_get_indexdef(i.indexrelid) ~* '\(event_id\)'
		)`).Scan(&eventIDUnique); err != nil {
		return fmt.Errorf("verify audit_logs event_id uniqueness: %w", err)
	}
	if !eventIDUnique {
		return fmt.Errorf("audit_logs schema is below the required migration: event_id unique index missing")
	}
	return nil
}

// handleBatch 批量处理审计日志消息
func (c *Consumer) handleBatch(ctx context.Context, messages []*kafka.ReceivedMessage) error {
	if len(messages) == 0 {
		return nil
	}

	entries := make([]auditEntry, 0, len(messages))
	for _, msg := range messages {
		parsed, err := c.parseMessages(msg)
		if err != nil {
			return kafka.Permanent(fmt.Errorf("parse audit message partition=%d offset=%d: %w", msg.Partition, msg.Offset, err))
		}
		entries = append(entries, parsed...)
	}
	err := c.persistEntries(ctx, entries)
	if errors.Is(err, ErrAuditEventIdentityCollision) {
		return kafka.Permanent(err)
	}
	return err
}

func (c *Consumer) persistEntries(ctx context.Context, entries []auditEntry) error {
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
		 ON CONFLICT (event_id) DO UPDATE SET event_id=EXCLUDED.event_id
		 WHERE audit_logs.tenant_id=EXCLUDED.tenant_id
		   AND audit_logs.user_id IS NOT DISTINCT FROM EXCLUDED.user_id
		   AND audit_logs.action=EXCLUDED.action
		   AND audit_logs.object_type=EXCLUDED.object_type
		   AND audit_logs.object_id IS NOT DISTINCT FROM EXCLUDED.object_id
		   AND audit_logs.detail=EXCLUDED.detail
		   AND audit_logs.ip_addr IS NOT DISTINCT FROM EXCLUDED.ip_addr
		   AND audit_logs.user_agent IS NOT DISTINCT FROM EXCLUDED.user_agent
		   AND ($11::boolean=false OR audit_logs.created_at=EXCLUDED.created_at)`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, e := range entries {
		ts := time.UnixMilli(e.createdAt)
		if e.createdAt == 0 {
			ts = time.Now()
		}
		result, err := stmt.ExecContext(ctx,
			e.eventID, e.tenantID, e.userID, e.action,
			e.objectType, e.objectID, e.detail, e.ipAddr, e.userAgent, ts, e.createdAt != 0,
		)
		if err != nil {
			return fmt.Errorf("insert audit event %s: %w", e.eventID, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect audit event %s persistence: %w", e.eventID, err)
		}
		if affected != 1 {
			return fmt.Errorf("%w: event_id=%s", ErrAuditEventIdentityCollision, e.eventID)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	c.logger.Debug("Audit batch committed",
		zap.Int("events", len(entries)))
	return nil
}

// parseMessages parses every event in AuditLogBatch. Returning an error is
// intentionally fail-closed: the consumer must not commit an offset whose
// audit payload was only partially persisted.
func (c *Consumer) parseMessages(msg *kafka.ReceivedMessage) ([]auditEntry, error) {
	// 尝试 AuditLogBatch
	var batch pb.AuditLogBatch
	if err := proto.Unmarshal(msg.Value, &batch); err == nil && len(batch.Events) > 0 {
		entries := make([]auditEntry, 0, len(batch.Events))
		seen := make(map[string]struct{}, len(batch.Events))
		for index, event := range batch.Events {
			entry, err := auditEntryFromProto(event)
			if err != nil {
				return nil, fmt.Errorf("batch event %d: %w", index, err)
			}
			if _, duplicate := seen[entry.eventID]; duplicate {
				return nil, fmt.Errorf("batch event %d: duplicate event_id %s", index, entry.eventID)
			}
			seen[entry.eventID] = struct{}{}
			entries = append(entries, entry)
		}
		return entries, nil
	}

	// 尝试单个 AuditLog
	var single pb.AuditLog
	if err := proto.Unmarshal(msg.Value, &single); err == nil && single.EventId != "" {
		entry, err := auditEntryFromProto(&single)
		if err != nil {
			return nil, err
		}
		return []auditEntry{entry}, nil
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
		entry := auditEntry{
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
		}
		if entry.tenantID == "" || entry.action == "" {
			return nil, fmt.Errorf("JSON audit event requires tenant_id and action")
		}
		if value, ok := raw["detail"]; ok {
			switch typed := value.(type) {
			case string:
				entry.detail = typed
			default:
				encoded, err := json.Marshal(typed)
				if err != nil {
					return nil, fmt.Errorf("marshal JSON audit detail: %w", err)
				}
				entry.detail = string(encoded)
			}
		}
		normalized, err := normalizeAuditEntry(entry)
		if err != nil {
			return nil, err
		}
		return []auditEntry{normalized}, nil
	}

	return nil, fmt.Errorf("unmarshal audit message: unknown format")
}

func auditEntryFromProto(event *pb.AuditLog) (auditEntry, error) {
	if event == nil {
		return auditEntry{}, fmt.Errorf("event is nil")
	}
	if strings.TrimSpace(event.EventId) == "" || strings.TrimSpace(event.TenantId) == "" || strings.TrimSpace(event.Action) == "" {
		return auditEntry{}, fmt.Errorf("event_id, tenant_id and action are required")
	}
	return normalizeAuditEntry(auditEntry{
		eventID:    event.EventId,
		tenantID:   event.TenantId,
		userID:     event.UserId,
		action:     event.Action,
		objectType: event.ObjectType,
		objectID:   event.ObjectId,
		detail:     event.Detail,
		ipAddr:     event.IpAddr,
		userAgent:  event.UserAgent,
		createdAt:  event.CreatedAt,
	})
}

func normalizeAuditEntry(entry auditEntry) (auditEntry, error) {
	entry.eventID = strings.TrimSpace(entry.eventID)
	entry.tenantID = strings.TrimSpace(entry.tenantID)
	entry.userID = strings.TrimSpace(entry.userID)
	entry.action = strings.TrimSpace(entry.action)
	entry.objectType = strings.TrimSpace(entry.objectType)
	if entry.objectType == "" {
		entry.objectType = "unknown"
	}
	entry.detail = strings.TrimSpace(entry.detail)
	if entry.detail == "" {
		entry.detail = "{}"
	}
	if !json.Valid([]byte(entry.detail)) {
		return auditEntry{}, fmt.Errorf("audit detail must be valid JSON")
	}
	return entry, nil
}

////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/publisher/kafka_publisher.go
// 规则 Kafka 发布器 - 完整修复版
// 修复内容：
// 1. 添加发送超时控制
// 2. 添加重试指标记录
// 3. 增强错误处理和日志
// 4. 添加批量发布优化
// 5. 添加健康检查接口
////////////////////////////////////////////////////////////////////////////////

package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/converter"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

// PublisherConfig 发布器配置
type PublisherConfig struct {
	// Kafka 配置
	Brokers    []string
	RuleTopic  string
	AuditTopic string

	// 超时配置
	SendTimeout    time.Duration
	PublishTimeout time.Duration

	// 重试配置
	MaxRetries   int
	RetryBackoff time.Duration

	// 批量配置
	BatchSize    int
	BatchTimeout time.Duration

	// 压缩配置
	Compression string // lz4, zstd, snappy, gzip

	// 可靠性配置
	RequiredAcks string // none, one, all
	Async        bool
}

// DefaultPublisherConfig 默认配置
func DefaultPublisherConfig() PublisherConfig {
	return PublisherConfig{
		SendTimeout:    5 * time.Second,
		PublishTimeout: 30 * time.Second,
		MaxRetries:     3,
		RetryBackoff:   100 * time.Millisecond,
		BatchSize:      100,
		BatchTimeout:   100 * time.Millisecond,
		Compression:    "lz4",
		RequiredAcks:   "all",
		Async:          false,
	}
}

// PublisherMetrics 发布器指标
type PublisherMetrics struct {
	// 规则发布指标
	RuleMessagesSent  int64
	RuleMessagesError int64
	RuleBytesSent     int64
	RuleRetries       int64
	RuleCompensations int64
	RuleLastSendTime  time.Time
	RuleLastErrorTime time.Time
	RuleLastError     string

	// 审计发布指标
	AuditMessagesSent  int64
	AuditMessagesError int64
	AuditBytesSent     int64
	AuditLastSendTime  time.Time
	AuditLastErrorTime time.Time

	// 发布延迟
	RuleAvgLatencyMs  float64
	AuditAvgLatencyMs float64
}

// KafkaPublisher Kafka 发布器
type KafkaPublisher struct {
	ruleProducer  *kafka.Producer
	auditProducer *kafka.Producer
	config        PublisherConfig
	logger        *zap.Logger

	// 指标
	metrics PublisherMetrics
	mu      sync.RWMutex

	// 状态
	closed int32
}

// NewKafkaPublisher 创建 Kafka 发布器
func NewKafkaPublisher(brokers []string, ruleTopic, auditTopic string, logger *zap.Logger) (*KafkaPublisher, error) {
	cfg := DefaultPublisherConfig()
	cfg.Brokers = brokers
	cfg.RuleTopic = ruleTopic
	cfg.AuditTopic = auditTopic

	return NewKafkaPublisherWithConfig(cfg, logger)
}

// NewKafkaPublisherWithConfig 使用配置创建 Kafka 发布器
func NewKafkaPublisherWithConfig(cfg PublisherConfig, logger *zap.Logger) (*KafkaPublisher, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers not configured")
	}
	if cfg.RuleTopic == "" {
		return nil, fmt.Errorf("rule topic not configured")
	}

	// 规则更新 Producer - 需要强一致性
	ruleCfg := kafka.ProducerConfig{
		Brokers:      cfg.Brokers,
		Topic:        cfg.RuleTopic,
		BatchSize:    1, // 规则更新不批量，确保实时性
		BatchTimeout: 10 * time.Millisecond,
		MaxAttempts:  cfg.MaxRetries,
		RequiredAcks: cfg.RequiredAcks,
		Compression:  cfg.Compression,
		Async:        false, // 同步发送，确保可靠性
	}

	ruleProducer, err := kafka.NewProducer(ruleCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create rule producer: %w", err)
	}

	// 审计日志 Producer - 可以异步批量
	var auditProducer *kafka.Producer
	if cfg.AuditTopic != "" {
		auditCfg := kafka.ProducerConfig{
			Brokers:      cfg.Brokers,
			Topic:        cfg.AuditTopic,
			BatchSize:    cfg.BatchSize,
			BatchTimeout: cfg.BatchTimeout,
			MaxAttempts:  cfg.MaxRetries,
			RequiredAcks: "one", // 单副本确认即可
			Compression:  cfg.Compression,
			Async:        true, // 异步发送
		}

		auditProducer, err = kafka.NewProducer(auditCfg, logger)
		if err != nil {
			ruleProducer.Close()
			return nil, fmt.Errorf("failed to create audit producer: %w", err)
		}
	}

	logger.Info("Kafka publishers initialized",
		zap.String("rule_topic", cfg.RuleTopic),
		zap.String("audit_topic", cfg.AuditTopic),
		zap.Strings("brokers", cfg.Brokers))

	return &KafkaPublisher{
		ruleProducer:  ruleProducer,
		auditProducer: auditProducer,
		config:        cfg,
		logger:        logger,
	}, nil
}

// PublishRuleCommand 发布规则命令
func (p *KafkaPublisher) PublishRuleCommand(ctx context.Context, cmd *model.RuleCommand) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New(errors.ErrCodeServiceUnavailable, "publisher is closed")
	}

	ctx, span := otel.StartSpan(ctx, "KafkaPublisher.PublishRuleCommand")
	defer span.End()

	startTime := time.Now()

	// 设置超时
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.SendTimeout)
		defer cancel()
	}

	// 设置时间戳
	if cmd.Timestamp.IsZero() {
		cmd.Timestamp = time.Now()
	}

	// 转换为 Proto 兼容格式
	protoCmd := converter.CommandToProto(cmd)

	// 序列化
	value, err := json.Marshal(protoCmd)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal rule command")
	}

	// 构建 Headers
	headers := []kafka.MessageHeader{
		{Key: "event_id", Value: protoCmd.EventID},
		{Key: "action", Value: cmd.Action},
		{Key: "tenant_id", Value: cmd.Rule.TenantID},
		{Key: "rule_id", Value: cmd.Rule.RuleID},
		{Key: "rule_version", Value: converter.FormatVersion(cmd.Rule.Version)},
		{Key: "operator_id", Value: cmd.OperatorID},
		{Key: "event_type", Value: "rule_command"},
		{Key: "event_ts", Value: fmt.Sprintf("%d", cmd.Timestamp.UnixMilli())},
		{Key: "content_type", Value: "application/json"},
		{Key: "schema_version", Value: "1.0"},
	}

	// 如果有 checksum，添加到 header
	if protoCmd.Checksum != "" {
		headers = append(headers, kafka.MessageHeader{Key: "checksum", Value: protoCmd.Checksum})
	}

	// 发送消息 - 使用 rule_id 作为 key 确保同一规则的消息有序
	err = p.ruleProducer.Send(ctx, cmd.Rule.RuleID, value, headers...)

	// 记录指标
	latency := time.Since(startTime)
	p.recordRuleMetrics(err, int64(len(value)), latency)

	if err != nil {
		p.logger.Error("Failed to publish rule command",
			zap.String("rule_id", cmd.Rule.RuleID),
			zap.String("event_id", protoCmd.EventID),
			zap.String("action", cmd.Action),
			zap.Int64("version", cmd.Rule.Version),
			zap.Duration("latency", latency),
			zap.Error(err))
		otel.RecordError(ctx, err)
		return errors.Wrap(err, errors.ErrCodeKafkaError, "failed to publish rule command")
	}

	p.logger.Info("Rule command published",
		zap.String("rule_id", cmd.Rule.RuleID),
		zap.String("event_id", protoCmd.EventID),
		zap.String("action", cmd.Action),
		zap.Int64("version", cmd.Rule.Version),
		zap.String("operator_id", cmd.OperatorID),
		zap.Duration("latency", latency))

	return nil
}

// PublishRuleCommandWithRetry 带重试的发布规则命令
func (p *KafkaPublisher) PublishRuleCommandWithRetry(ctx context.Context, cmd *model.RuleCommand, maxRetries int) error {
	if maxRetries <= 0 {
		maxRetries = p.config.MaxRetries
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 记录重试
			atomic.AddInt64(&p.metrics.RuleRetries, 1)

			// 指数退避
			backoff := p.config.RetryBackoff * time.Duration(1<<uint(attempt-1))
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}

			p.logger.Warn("Retrying rule command publish",
				zap.String("rule_id", cmd.Rule.RuleID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries),
				zap.Duration("backoff", backoff))

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := p.PublishRuleCommand(ctx, cmd)
		if err == nil {
			return nil
		}

		lastErr = err

		// 检查是否可重试
		if !errors.IsRetryableError(err) {
			p.logger.Error("Non-retryable error, giving up",
				zap.String("rule_id", cmd.Rule.RuleID),
				zap.Error(err))
			return err
		}
	}

	return fmt.Errorf("all %d retries failed: %w", maxRetries, lastErr)
}

// PublishCompensation 发布补偿命令（用于回滚）
func (p *KafkaPublisher) PublishCompensation(ctx context.Context, ruleID, tenantID, action, operatorID string, version int64) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New(errors.ErrCodeServiceUnavailable, "publisher is closed")
	}

	ctx, span := otel.StartSpan(ctx, "KafkaPublisher.PublishCompensation")
	defer span.End()

	// 记录补偿计数
	atomic.AddInt64(&p.metrics.RuleCompensations, 1)

	compensation := &model.RuleCommand{
		Action: "compensate_" + action,
		Rule: &model.Rule{
			RuleID:   ruleID,
			TenantID: tenantID,
			Version:  version,
		},
		Timestamp:  time.Now(),
		OperatorID: operatorID,
	}

	// 转换为 Proto 兼容格式
	protoCmd := converter.CommandToProto(compensation)

	value, err := json.Marshal(protoCmd)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal compensation")
	}

	headers := []kafka.MessageHeader{
		{Key: "event_id", Value: protoCmd.EventID},
		{Key: "action", Value: compensation.Action},
		{Key: "tenant_id", Value: tenantID},
		{Key: "rule_id", Value: ruleID},
		{Key: "rule_version", Value: converter.FormatVersion(version)},
		{Key: "is_compensation", Value: "true"},
		{Key: "original_action", Value: action},
		{Key: "event_ts", Value: fmt.Sprintf("%d", compensation.Timestamp.UnixMilli())},
	}

	// 设置超时
	ctx, cancel := context.WithTimeout(ctx, p.config.SendTimeout)
	defer cancel()

	err = p.ruleProducer.Send(ctx, ruleID, value, headers...)
	if err != nil {
		p.logger.Error("Failed to publish compensation",
			zap.String("rule_id", ruleID),
			zap.String("event_id", protoCmd.EventID),
			zap.String("action", compensation.Action),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeKafkaError, "failed to publish compensation")
	}

	p.logger.Warn("Compensation command published",
		zap.String("rule_id", ruleID),
		zap.String("event_id", protoCmd.EventID),
		zap.String("action", compensation.Action),
		zap.String("original_action", action),
		zap.Int64("version", version))

	return nil
}

// PublishAuditLog 发布审计日志
func (p *KafkaPublisher) PublishAuditLog(ctx context.Context, log *model.AuditLog) error {
	if p.auditProducer == nil {
		// 审计日志未配置，静默跳过
		return nil
	}

	if atomic.LoadInt32(&p.closed) == 1 {
		return nil // 关闭时不报错，静默丢弃审计日志
	}

	ctx, span := otel.StartSpan(ctx, "KafkaPublisher.PublishAuditLog")
	defer span.End()

	startTime := time.Now()

	// 设置时间戳
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}

	// 序列化
	value, err := json.Marshal(log)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal audit log")
	}

	eventID := uuid.New().String()

	// 构建 Headers
	headers := []kafka.MessageHeader{
		{Key: "tenant_id", Value: log.TenantID},
		{Key: "user_id", Value: log.UserID},
		{Key: "event_id", Value: eventID},
		{Key: "action", Value: log.Action},
		{Key: "object_type", Value: log.ObjectType},
		{Key: "object_id", Value: log.ObjectID},
		{Key: "event_type", Value: "audit_log"},
		{Key: "event_ts", Value: fmt.Sprintf("%d", log.Timestamp.UnixMilli())},
	}

	// 设置超时
	ctx, cancel := context.WithTimeout(ctx, p.config.SendTimeout)
	defer cancel()

	// 发送消息（异步）- 使用 tenant_id 作为 key
	err = p.auditProducer.Send(ctx, log.TenantID, value, headers...)

	// 记录指标
	latency := time.Since(startTime)
	p.recordAuditMetrics(err, int64(len(value)), latency)

	if err != nil {
		// 审计日志发送失败不阻塞业务，只记录错误
		p.logger.Warn("Failed to publish audit log",
			zap.String("event_id", eventID),
			zap.String("action", log.Action),
			zap.String("object_type", log.ObjectType),
			zap.String("object_id", log.ObjectID),
			zap.Error(err))
		return err
	}

	p.logger.Debug("Audit log published",
		zap.String("tenant_id", log.TenantID),
		zap.String("event_id", eventID),
		zap.String("action", log.Action),
		zap.String("object_type", log.ObjectType),
		zap.String("object_id", log.ObjectID),
		zap.Duration("latency", latency))

	return nil
}

// PublishBatchRuleCommands 批量发布规则命令
func (p *KafkaPublisher) PublishBatchRuleCommands(ctx context.Context, commands []*model.RuleCommand) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New(errors.ErrCodeServiceUnavailable, "publisher is closed")
	}

	ctx, span := otel.StartSpan(ctx, "KafkaPublisher.PublishBatchRuleCommands")
	defer span.End()

	if len(commands) == 0 {
		return nil
	}

	startTime := time.Now()
	messages := make([]kafka.Message, 0, len(commands))
	var totalBytes int64

	for _, cmd := range commands {
		if cmd.Timestamp.IsZero() {
			cmd.Timestamp = time.Now()
		}

		// 转换为 Proto 兼容格式
		protoCmd := converter.CommandToProto(cmd)

		value, err := json.Marshal(protoCmd)
		if err != nil {
			p.logger.Warn("Failed to marshal command in batch, skipping",
				zap.String("rule_id", cmd.Rule.RuleID),
				zap.Error(err))
			continue
		}

		totalBytes += int64(len(value))

		messages = append(messages, kafka.Message{
			Key:   cmd.Rule.RuleID,
			Value: value,
			Headers: []kafka.MessageHeader{
				{Key: "event_id", Value: protoCmd.EventID},
				{Key: "action", Value: cmd.Action},
				{Key: "tenant_id", Value: cmd.Rule.TenantID},
				{Key: "rule_id", Value: cmd.Rule.RuleID},
				{Key: "rule_version", Value: converter.FormatVersion(cmd.Rule.Version)},
				{Key: "event_ts", Value: fmt.Sprintf("%d", cmd.Timestamp.UnixMilli())},
				{Key: "batch", Value: "true"},
			},
			Time: cmd.Timestamp,
		})
	}

	if len(messages) == 0 {
		return nil
	}

	// 批量发送
	err := p.ruleProducer.SendBatch(ctx, messages)

	// 记录指标
	latency := time.Since(startTime)
	if err != nil {
		p.mu.Lock()
		p.metrics.RuleMessagesError += int64(len(messages))
		p.metrics.RuleLastErrorTime = time.Now()
		p.metrics.RuleLastError = err.Error()
		p.mu.Unlock()

		p.logger.Error("Failed to publish batch rule commands",
			zap.Int("count", len(messages)),
			zap.Duration("latency", latency),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeKafkaError, "failed to publish batch rule commands")
	}

	p.mu.Lock()
	p.metrics.RuleMessagesSent += int64(len(messages))
	p.metrics.RuleBytesSent += totalBytes
	p.metrics.RuleLastSendTime = time.Now()
	p.mu.Unlock()

	p.logger.Info("Batch rule commands published",
		zap.Int("count", len(messages)),
		zap.Int64("bytes", totalBytes),
		zap.Duration("latency", latency))

	return nil
}

// PublishDeploymentEvent 发布部署事件
func (p *KafkaPublisher) PublishDeploymentEvent(ctx context.Context, deployment *model.Deployment, action, operatorID string) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New(errors.ErrCodeServiceUnavailable, "publisher is closed")
	}

	ctx, span := otel.StartSpan(ctx, "KafkaPublisher.PublishDeploymentEvent")
	defer span.End()

	eventID := uuid.New().String()
	now := time.Now()

	event := map[string]interface{}{
		"event_id":       eventID,
		"event_type":     "deployment_event",
		"action":         action,
		"deployment_id":  deployment.DeploymentID,
		"tenant_id":      deployment.TenantID,
		"rule_version":   deployment.RuleVersion,
		"model_version":  deployment.ModelVersion,
		"feature_set_id": deployment.FeatureSetID,
		"scope":          deployment.Scope,
		"status":         deployment.Status,
		"operator_id":    operatorID,
		"timestamp":      now.UnixMilli(),
	}

	value, err := json.Marshal(event)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal deployment event")
	}

	headers := []kafka.MessageHeader{
		{Key: "event_id", Value: eventID},
		{Key: "event_type", Value: "deployment_event"},
		{Key: "action", Value: action},
		{Key: "tenant_id", Value: deployment.TenantID},
		{Key: "deployment_id", Value: deployment.DeploymentID},
		{Key: "event_ts", Value: fmt.Sprintf("%d", now.UnixMilli())},
	}

	// 设置超时
	ctx, cancel := context.WithTimeout(ctx, p.config.SendTimeout)
	defer cancel()

	err = p.ruleProducer.Send(ctx, deployment.DeploymentID, value, headers...)
	if err != nil {
		p.logger.Error("Failed to publish deployment event",
			zap.String("deployment_id", deployment.DeploymentID),
			zap.String("action", action),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeKafkaError, "failed to publish deployment event")
	}

	p.logger.Info("Deployment event published",
		zap.String("deployment_id", deployment.DeploymentID),
		zap.String("event_id", eventID),
		zap.String("action", action),
		zap.String("status", deployment.Status))

	return nil
}

// recordRuleMetrics 记录规则发布指标
func (p *KafkaPublisher) recordRuleMetrics(err error, bytes int64, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err != nil {
		p.metrics.RuleMessagesError++
		p.metrics.RuleLastErrorTime = time.Now()
		p.metrics.RuleLastError = err.Error()
	} else {
		p.metrics.RuleMessagesSent++
		p.metrics.RuleBytesSent += bytes
		p.metrics.RuleLastSendTime = time.Now()

		// 更新平均延迟（简单移动平均）
		count := p.metrics.RuleMessagesSent
		p.metrics.RuleAvgLatencyMs = (p.metrics.RuleAvgLatencyMs*float64(count-1) + float64(latency.Milliseconds())) / float64(count)
	}
}

// recordAuditMetrics 记录审计发布指标
func (p *KafkaPublisher) recordAuditMetrics(err error, bytes int64, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err != nil {
		p.metrics.AuditMessagesError++
		p.metrics.AuditLastErrorTime = time.Now()
	} else {
		p.metrics.AuditMessagesSent++
		p.metrics.AuditBytesSent += bytes
		p.metrics.AuditLastSendTime = time.Now()

		// 更新平均延迟
		count := p.metrics.AuditMessagesSent
		p.metrics.AuditAvgLatencyMs = (p.metrics.AuditAvgLatencyMs*float64(count-1) + float64(latency.Milliseconds())) / float64(count)
	}
}

// GetMetrics 获取发布器指标
func (p *KafkaPublisher) GetMetrics() PublisherMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.metrics
}

// GetMetricsMap 获取发布器指标（map 格式，用于 API）
func (p *KafkaPublisher) GetMetricsMap() map[string]interface{} {
	metrics := p.GetMetrics()

	return map[string]interface{}{
		"rule_producer": map[string]interface{}{
			"messages_sent":   metrics.RuleMessagesSent,
			"messages_error":  metrics.RuleMessagesError,
			"bytes_sent":      metrics.RuleBytesSent,
			"retries":         metrics.RuleRetries,
			"compensations":   metrics.RuleCompensations,
			"avg_latency_ms":  metrics.RuleAvgLatencyMs,
			"last_send_time":  metrics.RuleLastSendTime,
			"last_error_time": metrics.RuleLastErrorTime,
			"last_error":      metrics.RuleLastError,
		},
		"audit_producer": map[string]interface{}{
			"messages_sent":   metrics.AuditMessagesSent,
			"messages_error":  metrics.AuditMessagesError,
			"bytes_sent":      metrics.AuditBytesSent,
			"avg_latency_ms":  metrics.AuditAvgLatencyMs,
			"last_send_time":  metrics.AuditLastSendTime,
			"last_error_time": metrics.AuditLastErrorTime,
		},
	}
}

// HealthCheck 健康检查
func (p *KafkaPublisher) HealthCheck(ctx context.Context) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New(errors.ErrCodeServiceUnavailable, "publisher is closed")
	}

	// 检查最近是否有错误
	p.mu.RLock()
	lastErrorTime := p.metrics.RuleLastErrorTime
	lastSendTime := p.metrics.RuleLastSendTime
	lastError := p.metrics.RuleLastError
	p.mu.RUnlock()

	// 如果最近 1 分钟内有错误且没有成功发送，认为不健康
	if !lastErrorTime.IsZero() && time.Since(lastErrorTime) < time.Minute {
		if lastSendTime.IsZero() || lastErrorTime.After(lastSendTime) {
			return fmt.Errorf("recent error: %s", lastError)
		}
	}

	return nil
}

// IsHealthy 检查是否健康
func (p *KafkaPublisher) IsHealthy() bool {
	return p.HealthCheck(context.Background()) == nil
}

// Close 关闭发布器
func (p *KafkaPublisher) Close() error {
	if !atomic.CompareAndSwapInt32(&p.closed, 0, 1) {
		return nil // 已经关闭
	}

	var errs []error

	if err := p.ruleProducer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close rule producer: %w", err))
	}

	if p.auditProducer != nil {
		if err := p.auditProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close audit producer: %w", err))
		}
	}

	// 记录最终指标
	metrics := p.GetMetrics()
	p.logger.Info("Kafka publishers closed",
		zap.Int64("rule_messages_sent", metrics.RuleMessagesSent),
		zap.Int64("rule_messages_error", metrics.RuleMessagesError),
		zap.Int64("rule_retries", metrics.RuleRetries),
		zap.Int64("audit_messages_sent", metrics.AuditMessagesSent),
		zap.Int64("audit_messages_error", metrics.AuditMessagesError))

	if len(errs) > 0 {
		return fmt.Errorf("errors closing publishers: %v", errs)
	}

	return nil
}

// IsClosed 检查是否已关闭
func (p *KafkaPublisher) IsClosed() bool {
	return atomic.LoadInt32(&p.closed) == 1
}

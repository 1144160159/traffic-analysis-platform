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
	"strings"
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
	Brokers          []string
	RuleTopic        string
	ModelTopic       string
	ModelActionTopic string
	DeploymentTopic  string
	AuditTopic       string

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
	Security     kafka.SecurityConfig
}

// DefaultPublisherConfig 默认配置
func DefaultPublisherConfig() PublisherConfig {
	return PublisherConfig{
		SendTimeout:      5 * time.Second,
		PublishTimeout:   30 * time.Second,
		MaxRetries:       3,
		RetryBackoff:     100 * time.Millisecond,
		BatchSize:        100,
		BatchTimeout:     100 * time.Millisecond,
		Compression:      "lz4",
		RequiredAcks:     "all",
		Async:            false,
		ModelTopic:       "model-updates",
		ModelActionTopic: "model-actions.v1",
		DeploymentTopic:  "deployment.events.v1",
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

	// 模型热更新发布指标
	ModelMessagesSent  int64
	ModelMessagesError int64
	ModelBytesSent     int64
	ModelLastSendTime  time.Time
	ModelLastErrorTime time.Time
	ModelLastError     string

	// 发布延迟
	RuleAvgLatencyMs  float64
	AuditAvgLatencyMs float64
	ModelAvgLatencyMs float64
}

// KafkaPublisher Kafka 发布器
type KafkaPublisher struct {
	ruleProducer        *kafka.Producer
	modelProducer       *kafka.Producer
	modelActionProducer *kafka.Producer
	deploymentProducer  *kafka.Producer
	auditProducer       *kafka.Producer
	config              PublisherConfig
	logger              *zap.Logger

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
		Security:     cfg.Security,
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
			Security:     cfg.Security,
		}

		auditProducer, err = kafka.NewProducer(auditCfg, logger)
		if err != nil {
			ruleProducer.Close()
			return nil, fmt.Errorf("failed to create audit producer: %w", err)
		}
	}

	// 模型热更新 Producer - 单独写入 model-updates topic, 供 Behavior Job 广播消费
	var modelProducer *kafka.Producer
	if cfg.ModelTopic != "" {
		modelCfg := kafka.ProducerConfig{
			Brokers:      cfg.Brokers,
			Topic:        cfg.ModelTopic,
			BatchSize:    1,
			BatchTimeout: 10 * time.Millisecond,
			MaxAttempts:  cfg.MaxRetries,
			RequiredAcks: cfg.RequiredAcks,
			Compression:  cfg.Compression,
			Async:        false,
			Security:     cfg.Security,
		}

		modelProducer, err = kafka.NewProducer(modelCfg, logger)
		if err != nil {
			ruleProducer.Close()
			if auditProducer != nil {
				auditProducer.Close()
			}
			return nil, fmt.Errorf("failed to create model producer: %w", err)
		}
	}

	// 模型操作请求使用独立契约，避免污染只承载热更新事件的 model-updates。
	var modelActionProducer *kafka.Producer
	if cfg.ModelActionTopic != "" {
		actionCfg := kafka.ProducerConfig{
			Brokers: cfg.Brokers, Topic: cfg.ModelActionTopic, BatchSize: 1,
			BatchTimeout: 10 * time.Millisecond, MaxAttempts: cfg.MaxRetries,
			RequiredAcks: cfg.RequiredAcks, Compression: cfg.Compression, Async: false,
			Security: cfg.Security,
		}
		modelActionProducer, err = kafka.NewProducer(actionCfg, logger)
		if err != nil {
			ruleProducer.Close()
			if auditProducer != nil {
				auditProducer.Close()
			}
			if modelProducer != nil {
				modelProducer.Close()
			}
			return nil, fmt.Errorf("failed to create model action producer: %w", err)
		}
	}

	// 部署生命周期事件使用独立 topic，避免污染只承载 RuleCommand 的
	// rule.updates 契约。该 producer 同步确认，durable outbox 负责重试。
	var deploymentProducer *kafka.Producer
	if cfg.DeploymentTopic != "" {
		deploymentCfg := kafka.ProducerConfig{
			Brokers: cfg.Brokers, Topic: cfg.DeploymentTopic, BatchSize: 1,
			BatchTimeout: 10 * time.Millisecond, MaxAttempts: cfg.MaxRetries,
			RequiredAcks: cfg.RequiredAcks, Compression: cfg.Compression, Async: false,
			Security: cfg.Security,
		}
		deploymentProducer, err = kafka.NewProducer(deploymentCfg, logger)
		if err != nil {
			ruleProducer.Close()
			if auditProducer != nil {
				auditProducer.Close()
			}
			if modelProducer != nil {
				modelProducer.Close()
			}
			if modelActionProducer != nil {
				modelActionProducer.Close()
			}
			return nil, fmt.Errorf("failed to create deployment producer: %w", err)
		}
	}

	logger.Info("Kafka publishers initialized",
		zap.String("rule_topic", cfg.RuleTopic),
		zap.String("model_topic", cfg.ModelTopic),
		zap.String("model_action_topic", cfg.ModelActionTopic),
		zap.String("deployment_topic", cfg.DeploymentTopic),
		zap.String("audit_topic", cfg.AuditTopic),
		zap.Strings("brokers", cfg.Brokers))

	return &KafkaPublisher{
		ruleProducer:        ruleProducer,
		modelProducer:       modelProducer,
		modelActionProducer: modelActionProducer,
		deploymentProducer:  deploymentProducer,
		auditProducer:       auditProducer,
		config:              cfg,
		logger:              logger,
	}, nil
}

// PublishModelAction publishes a durable MLOps work request to the dedicated
// model-actions topic. This topic has a different schema from model-updates.
func (p *KafkaPublisher) PublishModelAction(ctx context.Context, key string, value []byte) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New(errors.ErrCodeServiceUnavailable, "publisher is closed")
	}
	if p.modelActionProducer == nil {
		return errors.New(errors.ErrCodeServiceUnavailable, "model action producer is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, p.config.SendTimeout)
	defer cancel()
	headers := []kafka.MessageHeader{
		{Key: "event_id", Value: uuid.NewString()},
		{Key: "event_type", Value: "model_action_requested"},
		{Key: "event_ts", Value: fmt.Sprintf("%d", time.Now().UnixMilli())},
		{Key: "content_type", Value: "application/json"},
	}
	if err := p.modelActionProducer.Send(ctx, key, value, headers...); err != nil {
		return errors.Wrap(err, errors.ErrCodeKafkaError, "failed to publish model action")
	}
	return nil
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
	return p.PublishDeploymentEventWithID(ctx, deployment, action, operatorID, uuid.New().String(), time.Now().UTC())
}

func buildDeploymentEventPayload(deployment *model.Deployment, action, operatorID, eventID string, occurredAt time.Time) map[string]interface{} {
	return map[string]interface{}{
		"event_id":       eventID,
		"schema_version": 1,
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
		"timestamp":      occurredAt.UnixMilli(),
	}
}

// PublishDeploymentEventWithID publishes a durable outbox envelope without
// regenerating identity or occurrence time on retry. This keeps downstream
// consumer deduplication stable under at-least-once delivery.
func (p *KafkaPublisher) PublishDeploymentEventWithID(ctx context.Context, deployment *model.Deployment, action, operatorID, eventID string, occurredAt time.Time) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New(errors.ErrCodeServiceUnavailable, "publisher is closed")
	}

	ctx, span := otel.StartSpan(ctx, "KafkaPublisher.PublishDeploymentEvent")
	defer span.End()

	if strings.TrimSpace(eventID) == "" {
		eventID = uuid.New().String()
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	event := buildDeploymentEventPayload(deployment, action, operatorID, eventID, occurredAt)

	value, err := json.Marshal(event)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal deployment event")
	}

	headers := []kafka.MessageHeader{
		{Key: "event_id", Value: eventID},
		{Key: "schema_version", Value: "1"},
		{Key: "event_type", Value: "deployment_event"},
		{Key: "action", Value: action},
		{Key: "tenant_id", Value: deployment.TenantID},
		{Key: "deployment_id", Value: deployment.DeploymentID},
		{Key: "event_ts", Value: fmt.Sprintf("%d", occurredAt.UnixMilli())},
	}

	// 设置超时
	ctx, cancel := context.WithTimeout(ctx, p.config.SendTimeout)
	defer cancel()

	if p.deploymentProducer == nil {
		return errors.New(errors.ErrCodeServiceUnavailable, "deployment event producer is not configured")
	}
	err = p.deploymentProducer.Send(ctx, deployment.DeploymentID, value, headers...)
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

// PublishModelUpdate 发布模型更新事件到 Kafka（MLOps 热更新通知）
func (p *KafkaPublisher) PublishModelUpdate(ctx context.Context, key string, value []byte) error {
	return p.PublishModelUpdateWithID(ctx, key, value, uuid.NewString(), time.Now().UTC())
}

// PublishModelUpdateWithID publishes an outbox-backed model update with a
// deterministic event identity. A database acknowledgement failure may cause
// an at-least-once retry, so consumers can deduplicate by event_id.
func (p *KafkaPublisher) PublishModelUpdateWithID(ctx context.Context, key string, value []byte, eventID string, occurredAt time.Time) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New(errors.ErrCodeServiceUnavailable, "publisher is closed")
	}
	if p.modelProducer == nil {
		return errors.New(errors.ErrCodeServiceUnavailable, "model update producer is not configured")
	}

	ctx, span := otel.StartSpan(ctx, "KafkaPublisher.PublishModelUpdate")
	defer span.End()

	if strings.TrimSpace(eventID) == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "model update event_id is required")
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	startTime := time.Now()

	headers := []kafka.MessageHeader{
		{Key: "event_id", Value: eventID},
		{Key: "event_type", Value: "model_update"},
		{Key: "action", Value: "update"},
		{Key: "event_ts", Value: fmt.Sprintf("%d", occurredAt.UnixMilli())},
		{Key: "content_type", Value: "application/json"},
	}

	ctx, cancel := context.WithTimeout(ctx, p.config.SendTimeout)
	defer cancel()

	err := p.modelProducer.Send(ctx, key, value, headers...)
	latency := time.Since(startTime)
	p.recordModelMetrics(err, int64(len(value)), latency)
	if err != nil {
		p.logger.Error("Failed to publish model update event",
			zap.String("topic", p.config.ModelTopic),
			zap.String("key", key),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeKafkaError, "failed to publish model update event")
	}

	p.logger.Info("Model update event published",
		zap.String("topic", p.config.ModelTopic),
		zap.String("event_id", eventID),
		zap.String("key", key))

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

// recordModelMetrics 记录模型热更新发布指标
func (p *KafkaPublisher) recordModelMetrics(err error, bytes int64, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err != nil {
		p.metrics.ModelMessagesError++
		p.metrics.ModelLastErrorTime = time.Now()
		p.metrics.ModelLastError = err.Error()
	} else {
		p.metrics.ModelMessagesSent++
		p.metrics.ModelBytesSent += bytes
		p.metrics.ModelLastSendTime = time.Now()

		count := p.metrics.ModelMessagesSent
		p.metrics.ModelAvgLatencyMs = (p.metrics.ModelAvgLatencyMs*float64(count-1) + float64(latency.Milliseconds())) / float64(count)
	}
}

// GetMetrics 获取发布器指标
func (p *KafkaPublisher) GetMetrics() PublisherMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.metrics
}

// PublishRaw 发布原始消息到已配置的 Kafka topic
func (p *KafkaPublisher) PublishRaw(ctx context.Context, topic, key string, value []byte) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New(errors.ErrCodeServiceUnavailable, "publisher is closed")
	}

	ctx, span := otel.StartSpan(ctx, "KafkaPublisher.PublishRaw")
	defer span.End()

	// 使用 ruleProducer 的基础能力发送到任意 topic
	headers := []kafka.MessageHeader{
		{Key: "event_type", Value: "model_update"},
		{Key: "event_ts", Value: fmt.Sprintf("%d", time.Now().UnixMilli())},
		{Key: "content_type", Value: "application/json"},
	}

	ctx, cancel := context.WithTimeout(ctx, p.config.SendTimeout)
	defer cancel()

	var producer *kafka.Producer
	switch topic {
	case p.config.RuleTopic:
		producer = p.ruleProducer
	case p.config.ModelTopic:
		producer = p.modelProducer
	case p.config.AuditTopic:
		producer = p.auditProducer
	default:
		return errors.Newf(errors.ErrCodeInvalidParameter, "topic is not configured: %s", topic)
	}
	if producer == nil {
		return errors.Newf(errors.ErrCodeServiceUnavailable, "producer is not configured for topic: %s", topic)
	}

	err := producer.Send(ctx, key, value, headers...)
	if err != nil {
		p.logger.Error("Failed to publish raw message",
			zap.String("topic", topic),
			zap.String("key", key),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeKafkaError, "failed to publish raw message")
	}

	p.logger.Debug("Raw message published",
		zap.String("topic", topic),
		zap.String("key", key))

	return nil
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
		"model_producer": map[string]interface{}{
			"topic":           p.config.ModelTopic,
			"messages_sent":   metrics.ModelMessagesSent,
			"messages_error":  metrics.ModelMessagesError,
			"bytes_sent":      metrics.ModelBytesSent,
			"avg_latency_ms":  metrics.ModelAvgLatencyMs,
			"last_send_time":  metrics.ModelLastSendTime,
			"last_error_time": metrics.ModelLastErrorTime,
			"last_error":      metrics.ModelLastError,
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

	if p.modelProducer != nil {
		if err := p.modelProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close model producer: %w", err))
		}
	}
	if p.modelActionProducer != nil {
		if err := p.modelActionProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close model action producer: %w", err))
		}
	}
	if p.deploymentProducer != nil {
		if err := p.deploymentProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close deployment producer: %w", err))
		}
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
		zap.Int64("model_messages_sent", metrics.ModelMessagesSent),
		zap.Int64("model_messages_error", metrics.ModelMessagesError),
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

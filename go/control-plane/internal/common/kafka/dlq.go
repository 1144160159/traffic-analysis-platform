////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/kafka/dlq.go
// 修复版 v3：
// 1. 优化 extractSchemaVersion 实现（简化逻辑）
// 2. 完整保留所有功能
////////////////////////////////////////////////////////////////////////////////

package kafka

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings" // 修复：添加 strings 导入
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// DLQConfig DLQ配置
type DLQConfig struct {
	Brokers     []string
	TopicPrefix string
	BatchSize   int
	MaxRetries  int
	RetryDelay  time.Duration
}

// ==================== 修复 #3：增强的 DLQMessage ====================

// DLQMessage DLQ消息格式（修复：增加 Protobuf 元数据）
type DLQMessage struct {
	// 原始消息信息
	OriginalTopic     string            `json:"original_topic"`
	OriginalPartition int               `json:"original_partition"`
	OriginalOffset    int64             `json:"original_offset"`
	OriginalKey       string            `json:"original_key"`
	OriginalValueB64  string            `json:"original_value_b64"` // base64 编码的原始值
	OriginalHeaders   map[string]string `json:"original_headers"`
	OriginalTimestamp time.Time         `json:"original_timestamp"`

	// 修复 #3：新增 Protobuf 元数据
	ContentType        string `json:"content_type"`         // application/x-protobuf, application/json
	ProtoMessageType   string `json:"proto_message_type"`   // 如：traffic.v1.FlowEvent
	ProtoSchemaVersion string `json:"proto_schema_version"` // 如：v1

	// 错误信息
	ErrorCode    string    `json:"error_code"`
	ErrorMessage string    `json:"error_message"`
	ErrorType    string    `json:"error_type"` // parse_error, validation_error, processing_error, timeout, etc.
	FailedAt     time.Time `json:"failed_at"`
	RetryCount   int       `json:"retry_count"`

	// 处理信息
	ServiceName    string    `json:"service_name"`
	ProcessingHost string    `json:"processing_host"`
	ProcessedAt    time.Time `json:"processed_at"`

	// 租户和追踪信息
	TenantID string `json:"tenant_id,omitempty"`
	EventID  string `json:"event_id,omitempty"`
	TraceID  string `json:"trace_id,omitempty"`
	RunID    string `json:"run_id,omitempty"`   // 新增：运行批次ID
	ProbeID  string `json:"probe_id,omitempty"` // 新增：探针ID

	// 额外元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// 重放控制（新增）
	ReplayPolicy ReplayPolicy `json:"replay_policy,omitempty"`
}

// ReplayPolicy 重放策略（新增）
type ReplayPolicy struct {
	MaxRetries       int           `json:"max_retries"`        // 最大重试次数（0 表示不重试）
	RetryBackoff     time.Duration `json:"retry_backoff"`      // 重试退避时间
	RetryableErrors  []string      `json:"retryable_errors"`   // 可重试的错误类型
	DeadlineAfter    time.Duration `json:"deadline_after"`     // 重试截止时间（从 FailedAt 开始计算）
	RequireManualAck bool          `json:"require_manual_ack"` // 是否需要人工确认才能重放
}

// DLQProducer DLQ生产者
type DLQProducer struct {
	writer      *kafka.Writer
	config      DLQConfig
	serviceName string
	hostname    string
	logger      *zap.Logger
	mu          sync.Mutex
	closed      bool
}

// NewDLQProducer 创建DLQ生产者
func NewDLQProducer(config DLQConfig, serviceName string, logger *zap.Logger) *DLQProducer {
	// 设置默认值
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 100 * time.Millisecond
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(config.Brokers...),
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    config.BatchSize,
		BatchTimeout: 100 * time.Millisecond,
		Compression:  kafka.Lz4,
		MaxAttempts:  config.MaxRetries,
		Async:        false, // 同步写入，确保不丢失
	}

	return &DLQProducer{
		writer:      writer,
		config:      config,
		serviceName: serviceName,
		hostname:    hostname,
		logger:      logger,
	}
}

// Send 发送单条消息到DLQ（修复版：增加 Protobuf 元数据）
func (p *DLQProducer) Send(ctx context.Context, msg *ReceivedMessage, err error) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("DLQ producer is closed")
	}
	p.mu.Unlock()

	// 修复 #3：构建 DLQ 消息（包含 Protobuf 元数据）
	dlqMsg := &DLQMessage{
		OriginalTopic:     msg.Topic,
		OriginalPartition: msg.Partition,
		OriginalOffset:    msg.Offset,
		OriginalKey:       string(msg.Key),
		OriginalValueB64:  base64.StdEncoding.EncodeToString(msg.Value),
		OriginalHeaders:   msg.GetAllHeaders(),
		OriginalTimestamp: msg.Time,

		// 修复 #3：提取 Protobuf 元数据
		ContentType:        msg.ContentType(),
		ProtoMessageType:   msg.ProtoMessageType(),
		ProtoSchemaVersion: extractSchemaVersion(msg.ProtoMessageType()),

		ErrorCode:      "PROCESSING_FAILED",
		ErrorMessage:   err.Error(),
		ErrorType:      categorizeError(err),
		FailedAt:       time.Now(),
		RetryCount:     0,
		ServiceName:    p.serviceName,
		ProcessingHost: p.hostname,
		ProcessedAt:    time.Now(),
		TenantID:       msg.TenantID(),
		EventID:        msg.EventID(),
		TraceID:        msg.TraceID(),
		RunID:          msg.RunID(),
		ProbeID:        msg.ProbeID(),

		// 修复 #3：设置重放策略
		ReplayPolicy: determineReplayPolicy(categorizeError(err)),
	}

	// 序列化为JSON
	payload, marshalErr := json.Marshal(dlqMsg)
	if marshalErr != nil {
		p.logger.Error("Failed to marshal DLQ message",
			zap.Error(marshalErr),
			zap.String("original_topic", msg.Topic),
			zap.Int64("original_offset", msg.Offset))
		return fmt.Errorf("failed to marshal DLQ message: %w", marshalErr)
	}

	// DLQ topic名称
	dlqTopic := p.config.TopicPrefix + msg.Topic

	// 构建Kafka消息
	kafkaMsg := kafka.Message{
		Topic: dlqTopic,
		Key:   msg.Key,
		Value: payload,
		Headers: []kafka.Header{
			{Key: "original_topic", Value: []byte(msg.Topic)},
			{Key: "original_partition", Value: []byte(fmt.Sprintf("%d", msg.Partition))},
			{Key: "original_offset", Value: []byte(fmt.Sprintf("%d", msg.Offset))},
			{Key: "error_type", Value: []byte(dlqMsg.ErrorType)},
			{Key: "error_code", Value: []byte(dlqMsg.ErrorCode)},
			{Key: "service_name", Value: []byte(p.serviceName)},
			{Key: "failed_at", Value: []byte(dlqMsg.FailedAt.Format(time.RFC3339))},
			// 修复 #3：添加 Protobuf 元数据到 Header
			{Key: "content_type", Value: []byte(dlqMsg.ContentType)},
			{Key: "proto_message_type", Value: []byte(dlqMsg.ProtoMessageType)},
		},
	}

	// 添加租户和追踪信息
	if dlqMsg.TenantID != "" {
		kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
			Key:   "tenant_id",
			Value: []byte(dlqMsg.TenantID),
		})
	}
	if dlqMsg.TraceID != "" {
		kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
			Key:   "trace_id",
			Value: []byte(dlqMsg.TraceID),
		})
	}
	if dlqMsg.RunID != "" {
		kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
			Key:   "run_id",
			Value: []byte(dlqMsg.RunID),
		})
	}

	// 带重试的写入
	var lastErr error
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(p.config.RetryDelay * time.Duration(attempt))
		}

		writeErr := p.writer.WriteMessages(ctx, kafkaMsg)
		if writeErr == nil {
			p.logger.Info("Message sent to DLQ",
				zap.String("dlq_topic", dlqTopic),
				zap.String("original_topic", msg.Topic),
				zap.Int("original_partition", msg.Partition),
				zap.Int64("original_offset", msg.Offset),
				zap.String("error_type", dlqMsg.ErrorType),
				zap.String("tenant_id", dlqMsg.TenantID),
				zap.String("proto_type", dlqMsg.ProtoMessageType))
			return nil
		}

		lastErr = writeErr
		p.logger.Warn("Failed to send to DLQ, retrying",
			zap.Error(writeErr),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", p.config.MaxRetries))
	}

	p.logger.Error("Failed to send message to DLQ after retries",
		zap.Error(lastErr),
		zap.String("dlq_topic", dlqTopic),
		zap.String("original_topic", msg.Topic),
		zap.Int64("original_offset", msg.Offset))

	return fmt.Errorf("failed to send to DLQ after %d retries: %w", p.config.MaxRetries, lastErr)
}

// SendBatch 批量发送到DLQ
func (p *DLQProducer) SendBatch(ctx context.Context, messages []struct {
	Msg *ReceivedMessage
	Err error
}) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("DLQ producer is closed")
	}
	p.mu.Unlock()

	if len(messages) == 0 {
		return nil
	}

	kafkaMessages := make([]kafka.Message, 0, len(messages))

	for _, item := range messages {
		msg := item.Msg
		err := item.Err

		// 构建DLQ消息
		dlqMsg := &DLQMessage{
			OriginalTopic:     msg.Topic,
			OriginalPartition: msg.Partition,
			OriginalOffset:    msg.Offset,
			OriginalKey:       string(msg.Key),
			OriginalValueB64:  base64.StdEncoding.EncodeToString(msg.Value),
			OriginalHeaders:   msg.GetAllHeaders(),
			OriginalTimestamp: msg.Time,

			// 修复 #3：Protobuf 元数据
			ContentType:        msg.ContentType(),
			ProtoMessageType:   msg.ProtoMessageType(),
			ProtoSchemaVersion: extractSchemaVersion(msg.ProtoMessageType()),

			ErrorCode:      "PROCESSING_FAILED",
			ErrorMessage:   err.Error(),
			ErrorType:      categorizeError(err),
			FailedAt:       time.Now(),
			RetryCount:     0,
			ServiceName:    p.serviceName,
			ProcessingHost: p.hostname,
			ProcessedAt:    time.Now(),
			TenantID:       msg.TenantID(),
			EventID:        msg.EventID(),
			TraceID:        msg.TraceID(),
			RunID:          msg.RunID(),
			ProbeID:        msg.ProbeID(),

			ReplayPolicy: determineReplayPolicy(categorizeError(err)),
		}

		payload, marshalErr := json.Marshal(dlqMsg)
		if marshalErr != nil {
			p.logger.Error("Failed to marshal DLQ message",
				zap.Error(marshalErr),
				zap.String("original_topic", msg.Topic),
				zap.Int64("original_offset", msg.Offset))
			continue
		}

		dlqTopic := p.config.TopicPrefix + msg.Topic

		kafkaMsg := kafka.Message{
			Topic: dlqTopic,
			Key:   msg.Key,
			Value: payload,
			Headers: []kafka.Header{
				{Key: "original_topic", Value: []byte(msg.Topic)},
				{Key: "original_partition", Value: []byte(fmt.Sprintf("%d", msg.Partition))},
				{Key: "original_offset", Value: []byte(fmt.Sprintf("%d", msg.Offset))},
				{Key: "error_type", Value: []byte(dlqMsg.ErrorType)},
				{Key: "error_code", Value: []byte(dlqMsg.ErrorCode)},
				{Key: "service_name", Value: []byte(p.serviceName)},
				{Key: "content_type", Value: []byte(dlqMsg.ContentType)},
				{Key: "proto_message_type", Value: []byte(dlqMsg.ProtoMessageType)},
			},
		}

		if dlqMsg.TenantID != "" {
			kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
				Key:   "tenant_id",
				Value: []byte(dlqMsg.TenantID),
			})
		}

		kafkaMessages = append(kafkaMessages, kafkaMsg)
	}

	if len(kafkaMessages) == 0 {
		return nil
	}

	// 带重试的批量写入
	var lastErr error
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(p.config.RetryDelay * time.Duration(attempt))
		}

		err := p.writer.WriteMessages(ctx, kafkaMessages...)
		if err == nil {
			p.logger.Info("Batch sent to DLQ",
				zap.Int("count", len(kafkaMessages)))
			return nil
		}

		lastErr = err
		p.logger.Warn("Failed to send batch to DLQ, retrying",
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.Int("count", len(kafkaMessages)))
	}

	p.logger.Error("Failed to send batch to DLQ after retries",
		zap.Error(lastErr),
		zap.Int("count", len(kafkaMessages)))

	return fmt.Errorf("failed to send batch to DLQ after %d retries: %w", p.config.MaxRetries, lastErr)
}

// Close 关闭DLQ生产者
func (p *DLQProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}

// ==================== 辅助函数 ====================

// 修复：优化 extractSchemaVersion 实现（简化逻辑）
func extractSchemaVersion(protoType string) string {
	if protoType == "" {
		return ""
	}

	// 分割成部分：traffic.v1.FlowEvent -> ["traffic", "v1", "FlowEvent"]
	parts := strings.Split(protoType, ".")
	for _, part := range parts {
		// 查找以 "v" 开头且后面跟数字的部分
		if len(part) > 1 && part[0] == 'v' {
			// 验证后面是否为数字或点
			isVersion := true
			for i := 1; i < len(part); i++ {
				if !((part[i] >= '0' && part[i] <= '9') || part[i] == '.') {
					isVersion = false
					break
				}
			}
			if isVersion {
				return part
			}
		}
	}
	return ""
}

// determineReplayPolicy 根据错误类型确定重放策略（新增）
func determineReplayPolicy(errorType string) ReplayPolicy {
	switch errorType {
	case "timeout", "connection_error":
		// 临时性错误，可以自动重试
		return ReplayPolicy{
			MaxRetries:       3,
			RetryBackoff:     5 * time.Minute,
			RetryableErrors:  []string{"timeout", "connection_error"},
			DeadlineAfter:    1 * time.Hour,
			RequireManualAck: false,
		}

	case "database_error", "storage_error":
		// 存储层错误，可以重试但需要更长的退避
		return ReplayPolicy{
			MaxRetries:       5,
			RetryBackoff:     10 * time.Minute,
			RetryableErrors:  []string{"database_error", "storage_error"},
			DeadlineAfter:    24 * time.Hour,
			RequireManualAck: false,
		}

	case "parse_error", "validation_error":
		// 数据格式错误，不可重试，需要人工干预
		return ReplayPolicy{
			MaxRetries:       0,
			RetryBackoff:     0,
			RetryableErrors:  []string{},
			DeadlineAfter:    0,
			RequireManualAck: true,
		}

	case "duplicate_error":
		// 幂等性冲突，不需要重试
		return ReplayPolicy{
			MaxRetries:       0,
			RetryBackoff:     0,
			RetryableErrors:  []string{},
			DeadlineAfter:    0,
			RequireManualAck: false,
		}

	case "permission_error":
		// 权限错误，需要人工修复
		return ReplayPolicy{
			MaxRetries:       0,
			RetryBackoff:     0,
			RetryableErrors:  []string{},
			DeadlineAfter:    0,
			RequireManualAck: true,
		}

	default:
		// 默认策略：保守重试
		return ReplayPolicy{
			MaxRetries:       1,
			RetryBackoff:     15 * time.Minute,
			RetryableErrors:  []string{"processing_error"},
			DeadlineAfter:    6 * time.Hour,
			RequireManualAck: false,
		}
	}
}

// categorizeError 分类错误类型（增强版）
func categorizeError(err error) string {
	if err == nil {
		return "unknown"
	}

	errStr := err.Error()

	// 使用更精确的错误分类
	switch {
	// 序列化错误
	case containsAny(errStr, []string{"unmarshal", "decode", "parse", "invalid json", "invalid protobuf"}):
		return "parse_error"

	// 验证错误
	case containsAny(errStr, []string{"validation", "invalid", "required", "missing"}):
		return "validation_error"

	// 超时错误
	case containsAny(errStr, []string{"timeout", "deadline", "context deadline exceeded"}):
		return "timeout"

	// 连接错误
	case containsAny(errStr, []string{"connection", "refused", "reset", "broken pipe", "no route to host"}):
		return "connection_error"

	// 数据库错误
	case containsAny(errStr, []string{"database", "sql", "clickhouse", "query failed"}):
		return "database_error"

	// 存储错误
	case containsAny(errStr, []string{"storage", "s3", "minio", "bucket"}):
		return "storage_error"

	// 重复错误
	case containsAny(errStr, []string{"duplicate", "already exists", "conflict"}):
		return "duplicate_error"

	// 权限错误
	case containsAny(errStr, []string{"permission", "unauthorized", "forbidden", "access denied"}):
		return "permission_error"

	// 资源不足
	case containsAny(errStr, []string{"quota", "limit", "capacity", "out of memory", "disk full"}):
		return "resource_error"

	// 默认处理错误
	default:
		return "processing_error"
	}
}

// containsAny 检查字符串是否包含任意一个子串（不区分大小写）
func containsAny(s string, substrs []string) bool {
	sLower := strings.ToLower(s)
	for _, substr := range substrs {
		if strings.Contains(sLower, strings.ToLower(substr)) {
			return true
		}
	}
	return false
}

// ==================== DLQ 消息操作（新增） ====================

// DecodeDLQMessage 解码DLQ消息（用于DLQ消费者）
func DecodeDLQMessage(data []byte) (*DLQMessage, error) {
	var msg DLQMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal DLQ message: %w", err)
	}
	return &msg, nil
}

// GetOriginalValue 获取原始值（解码 base64）
func (m *DLQMessage) GetOriginalValue() ([]byte, error) {
	if m.OriginalValueB64 == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(m.OriginalValueB64)
}

// IsRetryable 判断错误是否可重试
func (m *DLQMessage) IsRetryable() bool {
	switch m.ErrorType {
	case "timeout", "connection_error", "database_error", "storage_error":
		return true
	case "parse_error", "validation_error", "duplicate_error", "permission_error":
		return false
	default:
		return false
	}
}

// ShouldRetry 判断是否应该重试（考虑重试次数和策略）
func (m *DLQMessage) ShouldRetry(maxRetries int) bool {
	// 检查重放策略
	if m.ReplayPolicy.RequireManualAck {
		return false // 需要人工确认，不自动重试
	}

	// 检查是否超过重试截止时间
	if m.ReplayPolicy.DeadlineAfter > 0 {
		deadline := m.FailedAt.Add(m.ReplayPolicy.DeadlineAfter)
		if time.Now().After(deadline) {
			return false
		}
	}

	// 检查重试次数
	if m.ReplayPolicy.MaxRetries > 0 {
		maxRetries = m.ReplayPolicy.MaxRetries
	}

	return m.IsRetryable() && m.RetryCount < maxRetries
}

// CanReplayNow 判断是否可以立即重放（考虑退避时间）
func (m *DLQMessage) CanReplayNow() bool {
	if !m.ShouldRetry(999) { // 使用一个大数作为 maxRetries
		return false
	}

	// 计算下次重试时间
	backoff := m.ReplayPolicy.RetryBackoff
	if backoff <= 0 {
		backoff = 5 * time.Minute // 默认退避时间
	}

	// 指数退避
	nextRetryTime := m.FailedAt.Add(backoff * time.Duration(1<<uint(m.RetryCount)))

	return time.Now().After(nextRetryTime)
}

// IncrementRetryCount 增加重试计数（用于重放时）
func (m *DLQMessage) IncrementRetryCount() {
	m.RetryCount++
	m.ProcessedAt = time.Now()
}

// IsProtobuf 检查原始消息是否为 Protobuf（新增）
func (m *DLQMessage) IsProtobuf() bool {
	return m.ContentType == "application/x-protobuf" || m.ContentType == "application/protobuf"
}

// IsJSON 检查原始消息是否为 JSON（新增）
func (m *DLQMessage) IsJSON() bool {
	return m.ContentType == "application/json"
}

// GetProtoPackage 获取 Protobuf 包名（新增）
// 示例：traffic.v1.FlowEvent -> traffic.v1
func (m *DLQMessage) GetProtoPackage() string {
	if m.ProtoMessageType == "" {
		return ""
	}

	// 查找最后一个 '.'
	for i := len(m.ProtoMessageType) - 1; i >= 0; i-- {
		if m.ProtoMessageType[i] == '.' {
			return m.ProtoMessageType[:i]
		}
	}

	return ""
}

// GetProtoMessageName 获取 Protobuf 消息名（新增）
// 示例：traffic.v1.FlowEvent -> FlowEvent
func (m *DLQMessage) GetProtoMessageName() string {
	if m.ProtoMessageType == "" {
		return ""
	}

	// 查找最后一个 '.'
	for i := len(m.ProtoMessageType) - 1; i >= 0; i-- {
		if m.ProtoMessageType[i] == '.' {
			return m.ProtoMessageType[i+1:]
		}
	}

	return m.ProtoMessageType
}

// ToKafkaMessage 转换回 Kafka 消息格式（用于重放）
func (m *DLQMessage) ToKafkaMessage() (kafka.Message, error) {
	value, err := m.GetOriginalValue()
	if err != nil {
		return kafka.Message{}, fmt.Errorf("failed to decode original value: %w", err)
	}

	// 重建 Headers
	headers := make([]kafka.Header, 0, len(m.OriginalHeaders)+5)
	for k, v := range m.OriginalHeaders {
		headers = append(headers, kafka.Header{
			Key:   k,
			Value: []byte(v),
		})
	}

	// 添加重放标记
	headers = append(headers, kafka.Header{
		Key:   "x-replayed-from-dlq",
		Value: []byte("true"),
	})
	headers = append(headers, kafka.Header{
		Key:   "x-replay-count",
		Value: []byte(fmt.Sprintf("%d", m.RetryCount)),
	})
	headers = append(headers, kafka.Header{
		Key:   "x-original-failed-at",
		Value: []byte(m.FailedAt.Format(time.RFC3339)),
	})

	return kafka.Message{
		Topic:   m.OriginalTopic,
		Key:     []byte(m.OriginalKey),
		Value:   value,
		Headers: headers,
		Time:    time.Now(), // 使用当前时间作为重放时间
	}, nil
}

package kafka

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type DLQConfig struct {
	Brokers     []string
	TopicPrefix string
	BatchSize   int
	MaxRetries  int
	RetryDelay  time.Duration
}

type DLQMessage struct {
	OriginalTopic     string            `json:"original_topic"`
	OriginalPartition int               `json:"original_partition"`
	OriginalOffset    int64             `json:"original_offset"`
	OriginalKey       string            `json:"original_key"`
	OriginalValueB64  string            `json:"original_value_b64"`
	OriginalHeaders   map[string]string `json:"original_headers"`
	OriginalTimestamp time.Time         `json:"original_timestamp"`

	ContentType        string `json:"content_type"`
	ProtoMessageType   string `json:"proto_message_type"`
	ProtoSchemaVersion string `json:"proto_schema_version"`

	ErrorCode    string    `json:"error_code"`
	ErrorMessage string    `json:"error_message"`
	ErrorType    string    `json:"error_type"`
	FailedAt     time.Time `json:"failed_at"`
	RetryCount   int       `json:"retry_count"`

	ServiceName    string    `json:"service_name"`
	ProcessingHost string    `json:"processing_host"`
	ProcessedAt    time.Time `json:"processed_at"`

	TenantID string `json:"tenant_id,omitempty"`
	EventID  string `json:"event_id,omitempty"`
	TraceID  string `json:"trace_id,omitempty"`
	RunID    string `json:"run_id,omitempty"`
	ProbeID  string `json:"probe_id,omitempty"`

	Metadata map[string]interface{} `json:"metadata,omitempty"`

	ReplayPolicy ReplayPolicy `json:"replay_policy,omitempty"`
}

type ReplayPolicy struct {
	MaxRetries       int           `json:"max_retries"`
	RetryBackoff     time.Duration `json:"retry_backoff"`
	RetryableErrors  []string      `json:"retryable_errors"`
	DeadlineAfter    time.Duration `json:"deadline_after"`
	RequireManualAck bool          `json:"require_manual_ack"`
}

type DLQProducer struct {
	writer      *kafka.Writer
	config      DLQConfig
	serviceName string
	hostname    string
	logger      *zap.Logger
	mu          sync.Mutex
	closed      bool
}

func NewDLQProducer(config DLQConfig, serviceName string, logger *zap.Logger) *DLQProducer {

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
		Async:        false,
	}

	return &DLQProducer{
		writer:      writer,
		config:      config,
		serviceName: serviceName,
		hostname:    hostname,
		logger:      logger,
	}
}

func (p *DLQProducer) Send(ctx context.Context, msg *ReceivedMessage, err error) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("DLQ producer is closed")
	}
	p.mu.Unlock()

	dlqMsg := &DLQMessage{
		OriginalTopic:     msg.Topic,
		OriginalPartition: msg.Partition,
		OriginalOffset:    msg.Offset,
		OriginalKey:       string(msg.Key),
		OriginalValueB64:  base64.StdEncoding.EncodeToString(msg.Value),
		OriginalHeaders:   msg.GetAllHeaders(),
		OriginalTimestamp: msg.Time,

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
		return fmt.Errorf("failed to marshal DLQ message: %w", marshalErr)
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
			{Key: "failed_at", Value: []byte(dlqMsg.FailedAt.Format(time.RFC3339))},

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

		dlqMsg := &DLQMessage{
			OriginalTopic:     msg.Topic,
			OriginalPartition: msg.Partition,
			OriginalOffset:    msg.Offset,
			OriginalKey:       string(msg.Key),
			OriginalValueB64:  base64.StdEncoding.EncodeToString(msg.Value),
			OriginalHeaders:   msg.GetAllHeaders(),
			OriginalTimestamp: msg.Time,

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

func extractSchemaVersion(protoType string) string {
	if protoType == "" {
		return ""
	}

	parts := strings.Split(protoType, ".")
	for _, part := range parts {

		if len(part) > 1 && part[0] == 'v' {

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

func determineReplayPolicy(errorType string) ReplayPolicy {
	switch errorType {
	case "timeout", "connection_error":

		return ReplayPolicy{
			MaxRetries:       3,
			RetryBackoff:     5 * time.Minute,
			RetryableErrors:  []string{"timeout", "connection_error"},
			DeadlineAfter:    1 * time.Hour,
			RequireManualAck: false,
		}

	case "database_error", "storage_error":

		return ReplayPolicy{
			MaxRetries:       5,
			RetryBackoff:     10 * time.Minute,
			RetryableErrors:  []string{"database_error", "storage_error"},
			DeadlineAfter:    24 * time.Hour,
			RequireManualAck: false,
		}

	case "parse_error", "validation_error":

		return ReplayPolicy{
			MaxRetries:       0,
			RetryBackoff:     0,
			RetryableErrors:  []string{},
			DeadlineAfter:    0,
			RequireManualAck: true,
		}

	case "duplicate_error":

		return ReplayPolicy{
			MaxRetries:       0,
			RetryBackoff:     0,
			RetryableErrors:  []string{},
			DeadlineAfter:    0,
			RequireManualAck: false,
		}

	case "permission_error":

		return ReplayPolicy{
			MaxRetries:       0,
			RetryBackoff:     0,
			RetryableErrors:  []string{},
			DeadlineAfter:    0,
			RequireManualAck: true,
		}

	default:

		return ReplayPolicy{
			MaxRetries:       1,
			RetryBackoff:     15 * time.Minute,
			RetryableErrors:  []string{"processing_error"},
			DeadlineAfter:    6 * time.Hour,
			RequireManualAck: false,
		}
	}
}

func categorizeError(err error) string {
	if err == nil {
		return "unknown"
	}

	errStr := err.Error()

	switch {

	case containsAny(errStr, []string{"unmarshal", "decode", "parse", "invalid json", "invalid protobuf"}):
		return "parse_error"

	case containsAny(errStr, []string{"validation", "invalid", "required", "missing"}):
		return "validation_error"

	case containsAny(errStr, []string{"timeout", "deadline", "context deadline exceeded"}):
		return "timeout"

	case containsAny(errStr, []string{"connection", "refused", "reset", "broken pipe", "no route to host"}):
		return "connection_error"

	case containsAny(errStr, []string{"database", "sql", "clickhouse", "query failed"}):
		return "database_error"

	case containsAny(errStr, []string{"storage", "s3", "minio", "bucket"}):
		return "storage_error"

	case containsAny(errStr, []string{"duplicate", "already exists", "conflict"}):
		return "duplicate_error"

	case containsAny(errStr, []string{"permission", "unauthorized", "forbidden", "access denied"}):
		return "permission_error"

	case containsAny(errStr, []string{"quota", "limit", "capacity", "out of memory", "disk full"}):
		return "resource_error"

	default:
		return "processing_error"
	}
}

func containsAny(s string, substrs []string) bool {
	sLower := strings.ToLower(s)
	for _, substr := range substrs {
		if strings.Contains(sLower, strings.ToLower(substr)) {
			return true
		}
	}
	return false
}

func DecodeDLQMessage(data []byte) (*DLQMessage, error) {
	var msg DLQMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal DLQ message: %w", err)
	}
	return &msg, nil
}

func (m *DLQMessage) GetOriginalValue() ([]byte, error) {
	if m.OriginalValueB64 == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(m.OriginalValueB64)
}

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

func (m *DLQMessage) ShouldRetry(maxRetries int) bool {

	if m.ReplayPolicy.RequireManualAck {
		return false
	}

	if m.ReplayPolicy.DeadlineAfter > 0 {
		deadline := m.FailedAt.Add(m.ReplayPolicy.DeadlineAfter)
		if time.Now().After(deadline) {
			return false
		}
	}

	if m.ReplayPolicy.MaxRetries > 0 {
		maxRetries = m.ReplayPolicy.MaxRetries
	}

	return m.IsRetryable() && m.RetryCount < maxRetries
}

func (m *DLQMessage) CanReplayNow() bool {
	if !m.ShouldRetry(999) {
		return false
	}

	backoff := m.ReplayPolicy.RetryBackoff
	if backoff <= 0 {
		backoff = 5 * time.Minute
	}

	nextRetryTime := m.FailedAt.Add(backoff * time.Duration(1<<uint(m.RetryCount)))

	return time.Now().After(nextRetryTime)
}

func (m *DLQMessage) IncrementRetryCount() {
	m.RetryCount++
	m.ProcessedAt = time.Now()
}

func (m *DLQMessage) IsProtobuf() bool {
	return m.ContentType == "application/x-protobuf" || m.ContentType == "application/protobuf"
}

func (m *DLQMessage) IsJSON() bool {
	return m.ContentType == "application/json"
}

func (m *DLQMessage) GetProtoPackage() string {
	if m.ProtoMessageType == "" {
		return ""
	}

	for i := len(m.ProtoMessageType) - 1; i >= 0; i-- {
		if m.ProtoMessageType[i] == '.' {
			return m.ProtoMessageType[:i]
		}
	}

	return ""
}

func (m *DLQMessage) GetProtoMessageName() string {
	if m.ProtoMessageType == "" {
		return ""
	}

	for i := len(m.ProtoMessageType) - 1; i >= 0; i-- {
		if m.ProtoMessageType[i] == '.' {
			return m.ProtoMessageType[i+1:]
		}
	}

	return m.ProtoMessageType
}

func (m *DLQMessage) ToKafkaMessage() (kafka.Message, error) {
	value, err := m.GetOriginalValue()
	if err != nil {
		return kafka.Message{}, fmt.Errorf("failed to decode original value: %w", err)
	}

	headers := make([]kafka.Header, 0, len(m.OriginalHeaders)+5)
	for k, v := range m.OriginalHeaders {
		headers = append(headers, kafka.Header{
			Key:   k,
			Value: []byte(v),
		})
	}

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
		Time:    time.Now(),
	}, nil
}

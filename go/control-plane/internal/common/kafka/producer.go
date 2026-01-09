////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/kafka/producer.go
// 增强版 v5：
// 1. 添加 GetTopicProducer() 方法（支持 Ingest Gateway 获取指标）
// 2. 添加 SendBatch() 到 MultiTopicProducer
// 3. 修复所有之前的问题（atomic、资源泄漏、Header 去重）
////////////////////////////////////////////////////////////////////////////////

package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

// ProducerConfig 生产者配置
type ProducerConfig struct {
	Brokers       []string      `env:"KAFKA_BROKERS" envSeparator:","`
	Topic         string        `env:"KAFKA_TOPIC"`
	BatchSize     int           `env:"KAFKA_BATCH_SIZE" envDefault:"1000"`
	BatchTimeout  time.Duration `env:"KAFKA_BATCH_TIMEOUT" envDefault:"100ms"`
	MaxAttempts   int           `env:"KAFKA_MAX_ATTEMPTS" envDefault:"3"`
	RequiredAcks  string        `env:"KAFKA_REQUIRED_ACKS" envDefault:"all"`
	Compression   string        `env:"KAFKA_COMPRESSION" envDefault:"lz4"`
	Async         bool          `env:"KAFKA_ASYNC" envDefault:"false"`
	IdempotentKey string        `env:"KAFKA_IDEMPOTENT_KEY"`
}

// Producer Kafka生产者
type Producer struct {
	writer  *kafka.Writer
	logger  *zap.Logger
	config  ProducerConfig
	metrics *ProducerMetrics
	mu      sync.RWMutex

	closedFlag int32 // 0=未关闭, 1=已关闭
}

// ProducerMetrics 生产者指标（原子安全版本）
type ProducerMetrics struct {
	MessagesSent  int64
	MessagesError int64
	BytesSent     int64
	BatchesSent   int64

	lastSendTimeNano  int64
	lastErrorTimeNano int64

	lastErrorMsg sync.Map
}

// GetLastSendTime 获取最后发送时间
func (m *ProducerMetrics) GetLastSendTime() time.Time {
	nano := atomic.LoadInt64(&m.lastSendTimeNano)
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}

// GetLastErrorTime 获取最后错误时间
func (m *ProducerMetrics) GetLastErrorTime() time.Time {
	nano := atomic.LoadInt64(&m.lastErrorTimeNano)
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}

// GetLastError 获取最后错误信息
func (m *ProducerMetrics) GetLastError() string {
	if v, ok := m.lastErrorMsg.Load("error"); ok {
		return v.(string)
	}
	return ""
}

// NewProducer 创建生产者
func NewProducer(cfg ProducerConfig, logger *zap.Logger) (*Producer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers not configured")
	}
	if cfg.Topic == "" {
		return nil, fmt.Errorf("kafka topic not configured")
	}

	compression := getCompression(cfg.Compression)

	requiredAcks := kafka.RequireAll
	switch cfg.RequiredAcks {
	case "none":
		requiredAcks = kafka.RequireNone
	case "one":
		requiredAcks = kafka.RequireOne
	case "all":
		requiredAcks = kafka.RequireAll
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		Balancer:     &kafka.Hash{},
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
		MaxAttempts:  cfg.MaxAttempts,
		RequiredAcks: requiredAcks,
		Compression:  compression,
		Async:        cfg.Async,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logger.Error(fmt.Sprintf(msg, args...))
		}),
	}

	return &Producer{
		writer:     writer,
		logger:     logger,
		config:     cfg,
		metrics:    &ProducerMetrics{},
		closedFlag: 0,
	}, nil
}

// getCompression 获取压缩算法
func getCompression(name string) compress.Compression {
	switch name {
	case "gzip":
		return compress.Gzip
	case "snappy":
		return compress.Snappy
	case "lz4":
		return compress.Lz4
	case "zstd":
		return compress.Zstd
	default:
		return nil
	}
}

// Send 发送单条消息
func (p *Producer) Send(ctx context.Context, key string, value []byte, headers ...MessageHeader) error {
	return p.SendBatch(ctx, []Message{{Key: key, Value: value, Headers: headers}})
}

// SendJSON 发送JSON消息
func (p *Producer) SendJSON(ctx context.Context, key string, value interface{}, headers ...MessageHeader) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return p.Send(ctx, key, data, headers...)
}

// SendProto 发送Protobuf消息
func (p *Producer) SendProto(ctx context.Context, key string, msg proto.Message, headers ...MessageHeader) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal protobuf: %w", err)
	}
	return p.Send(ctx, key, data, headers...)
}

// Message 消息结构
type Message struct {
	Key     string
	Value   []byte
	Headers []MessageHeader
	Time    time.Time
}

// MessageHeader 消息头
type MessageHeader struct {
	Key   string
	Value string
}

// SendBatch 批量发送消息（原子安全版本）
func (p *Producer) SendBatch(ctx context.Context, messages []Message) error {
	if len(messages) == 0 {
		return nil
	}

	if atomic.LoadInt32(&p.closedFlag) == 1 {
		return fmt.Errorf("producer is closed")
	}

	ctx, span := otel.StartSpan(ctx, "kafka.produce")
	defer span.End()

	kafkaMessages := make([]kafka.Message, 0, len(messages))
	var totalBytes int64

	for _, msg := range messages {
		headers := make([]kafka.Header, 0, len(msg.Headers))

		existingKeys := make(map[string]bool)
		for _, h := range msg.Headers {
			headers = append(headers, kafka.Header{
				Key:   h.Key,
				Value: []byte(h.Value),
			})
			existingKeys[h.Key] = true
		}

		// 只添加不存在的 headers
		traceID := otel.GetTraceID(ctx)
		if traceID != "" && !existingKeys["trace_id"] {
			headers = append(headers, kafka.Header{
				Key:   "trace_id",
				Value: []byte(traceID),
			})
		}

		lc := logging.LogContextFromContext(ctx)
		if lc.TenantID != "" && !existingKeys["tenant_id"] {
			headers = append(headers, kafka.Header{
				Key:   "tenant_id",
				Value: []byte(lc.TenantID),
			})
		}

		msgTime := msg.Time
		if msgTime.IsZero() {
			msgTime = time.Now()
		}

		kafkaMessages = append(kafkaMessages, kafka.Message{
			Key:     []byte(msg.Key),
			Value:   msg.Value,
			Headers: headers,
			Time:    msgTime,
		})

		totalBytes += int64(len(msg.Key) + len(msg.Value))
	}

	start := time.Now()
	err := p.writer.WriteMessages(ctx, kafkaMessages...)
	duration := time.Since(start)

	if err != nil {
		atomic.AddInt64(&p.metrics.MessagesError, int64(len(messages)))
		atomic.StoreInt64(&p.metrics.lastErrorTimeNano, time.Now().UnixNano())
		p.metrics.lastErrorMsg.Store("error", err.Error())

		p.logger.Error("Failed to send messages to Kafka",
			zap.Error(err),
			zap.String("topic", p.config.Topic),
			zap.Int("count", len(messages)),
			zap.Duration("duration", duration))
		otel.RecordError(ctx, err)
		return fmt.Errorf("failed to send messages: %w", err)
	}

	atomic.AddInt64(&p.metrics.MessagesSent, int64(len(messages)))
	atomic.AddInt64(&p.metrics.BytesSent, totalBytes)
	atomic.AddInt64(&p.metrics.BatchesSent, 1)
	atomic.StoreInt64(&p.metrics.lastSendTimeNano, time.Now().UnixNano())

	p.logger.Debug("Messages sent to Kafka",
		zap.String("topic", p.config.Topic),
		zap.Int("count", len(messages)),
		zap.Int64("bytes", totalBytes),
		zap.Duration("duration", duration))

	return nil
}

// GetMetrics 获取指标快照
func (p *Producer) GetMetrics() ProducerMetricsSnapshot {
	return ProducerMetricsSnapshot{
		MessagesSent:  atomic.LoadInt64(&p.metrics.MessagesSent),
		MessagesError: atomic.LoadInt64(&p.metrics.MessagesError),
		BytesSent:     atomic.LoadInt64(&p.metrics.BytesSent),
		BatchesSent:   atomic.LoadInt64(&p.metrics.BatchesSent),
		LastSendTime:  p.metrics.GetLastSendTime(),
		LastErrorTime: p.metrics.GetLastErrorTime(),
		LastError:     p.metrics.GetLastError(),
	}
}

// ProducerMetricsSnapshot 指标快照（用于返回）
type ProducerMetricsSnapshot struct {
	MessagesSent  int64
	MessagesError int64
	BytesSent     int64
	BatchesSent   int64
	LastSendTime  time.Time
	LastErrorTime time.Time
	LastError     string
}

// Topic 获取topic名称
func (p *Producer) Topic() string {
	return p.config.Topic
}

// Close 关闭生产者（原子安全）
func (p *Producer) Close() error {
	if !atomic.CompareAndSwapInt32(&p.closedFlag, 0, 1) {
		return nil
	}

	return p.writer.Close()
}

// MultiTopicProducer 多topic生产者
type MultiTopicProducer struct {
	producers map[string]*Producer
	logger    *zap.Logger
	mu        sync.RWMutex
}

// NewMultiTopicProducer 创建多topic生产者
func NewMultiTopicProducer(logger *zap.Logger) *MultiTopicProducer {
	return &MultiTopicProducer{
		producers: make(map[string]*Producer),
		logger:    logger,
	}
}

// AddTopic 添加topic（防止资源泄漏）
func (mp *MultiTopicProducer) AddTopic(topic string, cfg ProducerConfig) error {
	cfg.Topic = topic
	producer, err := NewProducer(cfg, mp.logger)
	if err != nil {
		return err
	}

	mp.mu.Lock()
	defer mp.mu.Unlock()

	if oldProducer, exists := mp.producers[topic]; exists {
		mp.logger.Warn("Replacing existing producer for topic, closing old producer",
			zap.String("topic", topic))
		if closeErr := oldProducer.Close(); closeErr != nil {
			mp.logger.Error("Failed to close old producer",
				zap.String("topic", topic),
				zap.Error(closeErr))
		}
	}

	mp.producers[topic] = producer
	mp.logger.Info("Producer added for topic", zap.String("topic", topic))
	return nil
}

// Send 发送到指定topic
func (mp *MultiTopicProducer) Send(ctx context.Context, topic, key string, value []byte, headers ...MessageHeader) error {
	mp.mu.RLock()
	producer, ok := mp.producers[topic]
	mp.mu.RUnlock()

	if !ok {
		return fmt.Errorf("topic not found: %s", topic)
	}

	return producer.Send(ctx, key, value, headers...)
}

// SendBatch 批量发送到指定topic（新增方法）
func (mp *MultiTopicProducer) SendBatch(ctx context.Context, topic string, messages []Message) error {
	mp.mu.RLock()
	producer, ok := mp.producers[topic]
	mp.mu.RUnlock()

	if !ok {
		return fmt.Errorf("topic not found: %s", topic)
	}

	return producer.SendBatch(ctx, messages)
}

// GetTopicProducer 获取指定 Topic 的 Producer（新增方法，供 Ingest Gateway 使用）
func (mp *MultiTopicProducer) GetTopicProducer(topic string) (*Producer, error) {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	producer, ok := mp.producers[topic]
	if !ok {
		return nil, fmt.Errorf("topic not found: %s", topic)
	}

	return producer, nil
}

// GetTopicMetrics 获取指定 Topic 的指标（新增方法）
func (mp *MultiTopicProducer) GetTopicMetrics(topic string) (ProducerMetricsSnapshot, error) {
	producer, err := mp.GetTopicProducer(topic)
	if err != nil {
		return ProducerMetricsSnapshot{}, err
	}

	return producer.GetMetrics(), nil
}

// GetAllMetrics 获取所有 Topic 的指标（新增方法）
func (mp *MultiTopicProducer) GetAllMetrics() map[string]ProducerMetricsSnapshot {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	metrics := make(map[string]ProducerMetricsSnapshot)
	for topic, producer := range mp.producers {
		metrics[topic] = producer.GetMetrics()
	}

	return metrics
}

// Close 关闭所有生产者
func (mp *MultiTopicProducer) Close() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	var errs []error
	for topic, producer := range mp.producers {
		if err := producer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close producer for topic %s: %w", topic, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing producers: %v", errs)
	}
	return nil
}

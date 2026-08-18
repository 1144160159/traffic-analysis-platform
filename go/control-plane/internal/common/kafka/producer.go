package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/gzip"
	"github.com/segmentio/kafka-go/lz4"
	"github.com/segmentio/kafka-go/snappy"
	"github.com/segmentio/kafka-go/zstd"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

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
	Security      SecurityConfig
}

type Producer struct {
	writer  *kafka.Writer
	logger  *zap.Logger
	config  ProducerConfig
	metrics *ProducerMetrics
	mu      sync.RWMutex

	closedFlag int32
}

// KeyedProducer is the fail-closed producer used by streams whose ordering
// contract is scoped by the Kafka message key. It deliberately does not use
// ProducerConfig.IdempotentKey: that value describes an application key and
// must never be allowed to select the broker partitioning algorithm.
type KeyedProducer struct {
	producer *Producer
	send     func(context.Context, string, []byte, ...MessageHeader) error
	mu       sync.Mutex
	pending  map[string]chan brokerCompletion
}

const PublishAttemptHeader = "publish_attempt"

type BrokerReceipt struct {
	AttemptID      string
	Topic          string
	Partition      int
	Offset         int64
	Key            string
	AcknowledgedAt time.Time
}

type PublishOutcomeUnknownError struct {
	Receipt BrokerReceipt
	Cause   error
}

func (err *PublishOutcomeUnknownError) Error() string {
	if err == nil {
		return "Kafka publish outcome is unknown"
	}
	return fmt.Sprintf("Kafka publish outcome is unknown for attempt %s: %v", err.Receipt.AttemptID, err.Cause)
}

func (err *PublishOutcomeUnknownError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

type brokerCompletion struct {
	receipt BrokerReceipt
	err     error
}

var (
	ErrEmptyKafkaMessageKey = errors.New("keyed kafka producer requires a nonempty message key")
	ErrWeakKeyedProducer    = errors.New("keyed kafka producer requires synchronous acks=all")
)

type ProducerMetrics struct {
	MessagesSent  int64
	MessagesError int64
	BytesSent     int64
	BatchesSent   int64

	lastSendTimeNano  int64
	lastErrorTimeNano int64

	lastErrorMsg sync.Map
}

func (m *ProducerMetrics) GetLastSendTime() time.Time {
	nano := atomic.LoadInt64(&m.lastSendTimeNano)
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}

func (m *ProducerMetrics) GetLastErrorTime() time.Time {
	nano := atomic.LoadInt64(&m.lastErrorTimeNano)
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}

func (m *ProducerMetrics) GetLastError() string {
	if v, ok := m.lastErrorMsg.Load("error"); ok {
		return v.(string)
	}
	return ""
}

func NewProducer(cfg ProducerConfig, logger *zap.Logger) (*Producer, error) {
	// Preserve the existing generic producer policy for compatibility. Streams
	// that require per-key ordering must use NewKeyedProducer instead.
	var balancer kafka.Balancer = &kafka.Hash{}
	if cfg.IdempotentKey == "" {
		balancer = &kafka.RoundRobin{}
	}
	return newProducer(cfg, logger, balancer)
}

// NewKeyedProducer constructs a producer with a stable FNV-1a Hash balancer.
// Weak durability settings fail before a writer is created, and the selected
// balancer is independent of IdempotentKey or any other descriptive config.
func NewKeyedProducer(cfg ProducerConfig, logger *zap.Logger) (*KeyedProducer, error) {
	if cfg.Async || strings.ToLower(strings.TrimSpace(cfg.RequiredAcks)) != "all" {
		return nil, fmt.Errorf(
			"%w: required_acks=%q async=%t",
			ErrWeakKeyedProducer,
			cfg.RequiredAcks,
			cfg.Async,
		)
	}

	producer, err := newProducer(cfg, logger, &kafka.Hash{})
	if err != nil {
		return nil, err
	}
	keyed := &KeyedProducer{
		producer: producer,
		pending:  make(map[string]chan brokerCompletion),
	}
	keyed.send = producer.Send
	producer.writer.Completion = keyed.complete
	return keyed, nil
}

func newProducer(cfg ProducerConfig, logger *zap.Logger, balancer kafka.Balancer) (*Producer, error) {
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

	if balancer == nil {
		return nil, fmt.Errorf("kafka balancer not configured")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	dialer, err := cfg.Security.Dialer("traffic-control-plane-producer")
	if err != nil {
		return nil, err
	}

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:          cfg.Brokers,
		Topic:            cfg.Topic,
		Dialer:           dialer,
		Balancer:         balancer,
		BatchSize:        cfg.BatchSize,
		BatchTimeout:     cfg.BatchTimeout,
		MaxAttempts:      cfg.MaxAttempts,
		RequiredAcks:     int(requiredAcks),
		CompressionCodec: compression,
		Async:            cfg.Async,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logger.Error(fmt.Sprintf(msg, args...))
		}),
	})

	return &Producer{
		writer:     writer,
		logger:     logger,
		config:     cfg,
		metrics:    &ProducerMetrics{},
		closedFlag: 0,
	}, nil
}

func (p *KeyedProducer) Send(
	ctx context.Context,
	key string,
	value []byte,
	headers ...MessageHeader,
) (BrokerReceipt, error) {
	if p == nil || p.producer == nil {
		return BrokerReceipt{}, fmt.Errorf("keyed kafka producer is unavailable")
	}
	if strings.TrimSpace(key) == "" {
		return BrokerReceipt{}, ErrEmptyKafkaMessageKey
	}
	attemptID, err := publishAttemptID(headers)
	if err != nil {
		return BrokerReceipt{}, err
	}
	if attemptID == "" {
		attemptID = uuid.NewString()
		headers = append(headers, MessageHeader{Key: PublishAttemptHeader, Value: attemptID})
	}
	receipt := BrokerReceipt{
		AttemptID: attemptID,
		Topic:     p.producer.Topic(),
		Partition: -1,
		Offset:    -1,
		Key:       key,
	}
	completionChannel := make(chan brokerCompletion, 1)
	p.mu.Lock()
	if p.pending == nil {
		p.pending = make(map[string]chan brokerCompletion)
	}
	if _, exists := p.pending[attemptID]; exists {
		p.mu.Unlock()
		return receipt, fmt.Errorf("duplicate in-flight Kafka publish_attempt %s", attemptID)
	}
	p.pending[attemptID] = completionChannel
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.pending, attemptID)
		p.mu.Unlock()
	}()

	send := p.send
	if send == nil {
		send = p.producer.Send
	}
	sendErr := send(ctx, key, value, headers...)
	select {
	case completion := <-completionChannel:
		if completion.err != nil {
			return receipt, fmt.Errorf("Kafka publish attempt %s failed: %w", attemptID, completion.err)
		}
		if completion.receipt.AttemptID != attemptID || completion.receipt.Key != key ||
			completion.receipt.Topic != p.producer.Topic() || completion.receipt.Partition < 0 ||
			completion.receipt.Offset < 0 {
			return receipt, &PublishOutcomeUnknownError{
				Receipt: receipt,
				Cause:   fmt.Errorf("broker completion identity is incomplete or mismatched"),
			}
		}
		return completion.receipt, nil
	default:
		if sendErr == nil {
			sendErr = fmt.Errorf("synchronous writer returned without an exact broker completion")
		}
		return receipt, &PublishOutcomeUnknownError{Receipt: receipt, Cause: sendErr}
	}
}

func (p *KeyedProducer) complete(messages []kafka.Message, completionErr error) {
	for _, message := range messages {
		attemptID := kafkaMessageHeader(message.Headers, PublishAttemptHeader)
		if attemptID == "" {
			continue
		}
		topic := message.Topic
		if topic == "" && p != nil && p.producer != nil {
			topic = p.producer.Topic()
		}
		completion := brokerCompletion{
			receipt: BrokerReceipt{
				AttemptID:      attemptID,
				Topic:          topic,
				Partition:      message.Partition,
				Offset:         message.Offset,
				Key:            string(message.Key),
				AcknowledgedAt: time.Now().UTC(),
			},
			err: completionErr,
		}
		p.mu.Lock()
		channel := p.pending[attemptID]
		p.mu.Unlock()
		if channel != nil {
			select {
			case channel <- completion:
			default:
			}
		}
	}
}

func publishAttemptID(headers []MessageHeader) (string, error) {
	var attemptID string
	for _, header := range headers {
		if !strings.EqualFold(strings.TrimSpace(header.Key), PublishAttemptHeader) {
			continue
		}
		value := strings.TrimSpace(header.Value)
		if attemptID != "" {
			return "", fmt.Errorf("duplicate %s header", PublishAttemptHeader)
		}
		if _, err := uuid.Parse(value); err != nil {
			return "", fmt.Errorf("invalid %s header", PublishAttemptHeader)
		}
		attemptID = value
	}
	return attemptID, nil
}

func kafkaMessageHeader(headers []kafka.Header, key string) string {
	for _, header := range headers {
		if strings.EqualFold(strings.TrimSpace(header.Key), key) {
			return string(header.Value)
		}
	}
	return ""
}

func (p *KeyedProducer) Topic() string {
	if p == nil || p.producer == nil {
		return ""
	}
	return p.producer.Topic()
}

func (p *KeyedProducer) Close() error {
	if p == nil || p.producer == nil {
		return nil
	}
	return p.producer.Close()
}

func getCompression(name string) kafka.CompressionCodec {
	switch name {
	case "gzip":
		return gzip.NewCompressionCodec()
	case "snappy":
		return snappy.NewCompressionCodec()
	case "lz4":
		return lz4.NewCompressionCodec()
	case "zstd":
		return zstd.NewCompressionCodec()
	default:
		return nil
	}
}

func (p *Producer) Send(ctx context.Context, key string, value []byte, headers ...MessageHeader) error {
	return p.SendBatch(ctx, []Message{{Key: key, Value: value, Headers: headers}})
}

func (p *Producer) SendJSON(ctx context.Context, key string, value interface{}, headers ...MessageHeader) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return p.Send(ctx, key, data, headers...)
}

func (p *Producer) SendProto(ctx context.Context, key string, msg proto.Message, headers ...MessageHeader) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal protobuf: %w", err)
	}
	return p.Send(ctx, key, data, headers...)
}

type Message struct {
	Key     string
	Value   []byte
	Headers []MessageHeader
	Time    time.Time
}

type MessageHeader struct {
	Key   string
	Value string
}

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
		headers, err := buildKafkaHeaders(ctx, msg.Headers)
		if err != nil {
			return err
		}

		lc := logging.LogContextFromContext(ctx)
		if lc.TenantID != "" && !hasKafkaHeader(headers, "tenant_id") {
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

func buildKafkaHeaders(ctx context.Context, declared []MessageHeader) ([]kafka.Header, error) {
	headers := make([]kafka.Header, 0, len(declared)+3)
	existing := make(map[string]string, len(declared))
	for _, item := range declared {
		key := strings.ToLower(strings.TrimSpace(item.Key))
		if key == "" {
			continue
		}
		headers = append(headers, kafka.Header{Key: key, Value: []byte(item.Value)})
		existing[key] = item.Value
	}

	traceID := otel.GetTraceID(ctx)
	if traceID != "" {
		if declaredTrace := strings.TrimSpace(existing["trace_id"]); declaredTrace != "" && declaredTrace != traceID {
			return nil, fmt.Errorf("declared trace_id conflicts with W3C context")
		}
		if existing["trace_id"] == "" {
			headers = append(headers, kafka.Header{Key: "trace_id", Value: []byte(traceID)})
		}
		carrier := otel.InjectToMap(ctx)
		for _, key := range []string{"traceparent", "tracestate"} {
			if value := strings.TrimSpace(carrier[key]); value != "" && existing[key] == "" {
				headers = append(headers, kafka.Header{Key: key, Value: []byte(value)})
			}
		}
	}
	return headers, nil
}

func hasKafkaHeader(headers []kafka.Header, key string) bool {
	for _, header := range headers {
		if strings.EqualFold(header.Key, key) {
			return true
		}
	}
	return false
}

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

type ProducerMetricsSnapshot struct {
	MessagesSent  int64
	MessagesError int64
	BytesSent     int64
	BatchesSent   int64
	LastSendTime  time.Time
	LastErrorTime time.Time
	LastError     string
}

func (p *Producer) Topic() string {
	if p == nil {
		return ""
	}
	return p.config.Topic
}

func (p *Producer) Close() error {
	if !atomic.CompareAndSwapInt32(&p.closedFlag, 0, 1) {
		return nil
	}

	return p.writer.Close()
}

type MultiTopicProducer struct {
	producers map[string]*Producer
	logger    *zap.Logger
	mu        sync.RWMutex
}

func NewMultiTopicProducer(logger *zap.Logger) *MultiTopicProducer {
	return &MultiTopicProducer{
		producers: make(map[string]*Producer),
		logger:    logger,
	}
}

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

func (mp *MultiTopicProducer) Send(ctx context.Context, topic, key string, value []byte, headers ...MessageHeader) error {
	mp.mu.RLock()
	producer, ok := mp.producers[topic]
	mp.mu.RUnlock()

	if !ok {
		return fmt.Errorf("topic not found: %s", topic)
	}

	return producer.Send(ctx, key, value, headers...)
}

func (mp *MultiTopicProducer) SendBatch(ctx context.Context, topic string, messages []Message) error {
	mp.mu.RLock()
	producer, ok := mp.producers[topic]
	mp.mu.RUnlock()

	if !ok {
		return fmt.Errorf("topic not found: %s", topic)
	}

	return producer.SendBatch(ctx, messages)
}

func (mp *MultiTopicProducer) GetTopicProducer(topic string) (*Producer, error) {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	producer, ok := mp.producers[topic]
	if !ok {
		return nil, fmt.Errorf("topic not found: %s", topic)
	}

	return producer, nil
}

func (mp *MultiTopicProducer) GetTopicMetrics(topic string) (ProducerMetricsSnapshot, error) {
	producer, err := mp.GetTopicProducer(topic)
	if err != nil {
		return ProducerMetricsSnapshot{}, err
	}

	return producer.GetMetrics(), nil
}

func (mp *MultiTopicProducer) GetAllMetrics() map[string]ProducerMetricsSnapshot {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	metrics := make(map[string]ProducerMetricsSnapshot)
	for topic, producer := range mp.producers {
		metrics[topic] = producer.GetMetrics()
	}

	return metrics
}

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

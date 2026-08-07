package kafka

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	commonotel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

type ConsumerConfig struct {
	Brokers        []string
	Topic          string
	GroupID        string
	MinBytes       int
	MaxBytes       int
	MaxWait        time.Duration
	CommitInterval time.Duration
	StartOffset    int64
	MaxRetries     int
	RetryBackoff   time.Duration
	EnableDLQ      bool
	DLQTopicPrefix string
	DLQTopic       string

	CommitOnDLQSuccess   bool
	CommitOnHandlerError bool
	DLQPermanentOnly     bool
	Security             SecurityConfig
}

type Consumer struct {
	reader             *kafka.Reader
	config             ConsumerConfig
	logger             *zap.Logger
	metrics            ConsumerMetrics
	closed             int32
	mu                 sync.RWMutex
	commitChan         chan commitRequest
	stopCommitter      chan struct{}
	dlqProducer        *DLQProducer
	fetchFailures      int64
	lastFetchErr       string
	lastFetchAt        time.Time
	processingFailures int64
	lastProcessingErr  string
	lastProcessingAt   time.Time
	commitObserver     func([]kafka.Message)
}

type ConsumerMetrics struct {
	MessagesReceived              int64
	MessagesProcessed             int64
	MessagesFailed                int64
	MessagesDLQ                   int64
	CommitsSucceeded              int64
	CommitsFailed                 int64
	LastOffset                    int64
	Lag                           int64
	ConsecutiveFetchFailures      int64
	LastFetchErrorUnix            int64
	ConsecutiveProcessingFailures int64
	LastProcessingErrorUnix       int64
}

type commitRequest struct {
	messages []kafka.Message
	doneChan chan error
}

func NewConsumer(config ConsumerConfig, logger *zap.Logger) (*Consumer, error) {
	if len(config.Brokers) == 0 {
		return nil, fmt.Errorf("brokers cannot be empty")
	}
	if config.Topic == "" {
		return nil, fmt.Errorf("topic cannot be empty")
	}
	if config.GroupID == "" {
		return nil, fmt.Errorf("groupID cannot be empty")
	}

	if config.MinBytes == 0 {
		config.MinBytes = 1024
	}
	if config.MaxBytes == 0 {
		config.MaxBytes = 10 * 1024 * 1024
	}
	if config.MaxWait == 0 {
		config.MaxWait = 500 * time.Millisecond
	}
	if config.StartOffset == 0 {
		config.StartOffset = kafka.LastOffset
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryBackoff == 0 {
		config.RetryBackoff = time.Second
	}
	if config.DLQTopicPrefix == "" {
		config.DLQTopicPrefix = "dlq."
	}

	dialer, err := config.Security.Dialer("traffic-control-plane-consumer")
	if err != nil {
		return nil, err
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        config.Brokers,
		Topic:          config.Topic,
		GroupID:        config.GroupID,
		Dialer:         dialer,
		MinBytes:       config.MinBytes,
		MaxBytes:       config.MaxBytes,
		MaxWait:        config.MaxWait,
		StartOffset:    config.StartOffset,
		CommitInterval: 0,
		// 消费者组负载均衡: 使用 LeastBytes 策略避免热点分区
		GroupBalancers: []kafka.GroupBalancer{
			kafka.RoundRobinGroupBalancer{},
		},
	})
	logger.Info("Kafka consumer created",
		zap.String("group", config.GroupID),
		zap.String("topic", config.Topic),
		zap.Strings("brokers", config.Brokers))

	c := &Consumer{
		reader:        reader,
		config:        config,
		logger:        logger,
		commitChan:    make(chan commitRequest, 100),
		stopCommitter: make(chan struct{}),
	}

	if config.EnableDLQ {
		dlqConfig := DLQConfig{
			Brokers:     config.Brokers,
			TopicPrefix: config.DLQTopicPrefix,
			TargetTopic: config.DLQTopic,
			BatchSize:   100,
			MaxRetries:  3,
			RetryDelay:  100 * time.Millisecond,
			Security:    config.Security,
		}
		c.dlqProducer = NewDLQProducer(dlqConfig, "consumer-"+config.Topic, logger)
	}

	if config.CommitInterval > 0 {
		go c.backgroundCommitter()
	}

	return c, nil
}

func (c *Consumer) dlqEligible(err error) bool {
	return !c.config.DLQPermanentOnly || IsPermanent(err)
}

func (c *Consumer) backgroundCommitter() {
	ticker := time.NewTicker(c.config.CommitInterval)
	defer ticker.Stop()

	var pendingMessages []kafka.Message
	var mu sync.Mutex

	for {
		select {
		case <-c.stopCommitter:

			mu.Lock()
			if len(pendingMessages) > 0 {
				c.commitMessages(pendingMessages)
			}
			mu.Unlock()
			return

		case req := <-c.commitChan:
			mu.Lock()
			pendingMessages = append(pendingMessages, req.messages...)
			mu.Unlock()
			if req.doneChan != nil {
				req.doneChan <- nil
				close(req.doneChan)
			}

		case <-ticker.C:
			mu.Lock()
			if len(pendingMessages) > 0 {
				c.commitMessages(pendingMessages)
				pendingMessages = pendingMessages[:0]
			}
			mu.Unlock()
		}
	}
}

func (c *Consumer) commitMessages(messages []kafka.Message) error {
	if len(messages) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.reader.CommitMessages(ctx, messages...); err != nil {
		atomic.AddInt64(&c.metrics.CommitsFailed, 1)
		c.logger.Error("Failed to commit messages",
			zap.Int("count", len(messages)),
			zap.Error(err))
		return err
	}

	atomic.AddInt64(&c.metrics.CommitsSucceeded, 1)
	if len(messages) > 0 {
		atomic.StoreInt64(&c.metrics.LastOffset, messages[len(messages)-1].Offset)
	}

	c.logger.Debug("Messages committed",
		zap.Int("count", len(messages)))
	c.notifyCommitObserver(messages)

	return nil
}

// SetCommitObserver installs a post-commit observer for progress metrics. The
// callback runs only after Kafka acknowledges the offset commit and cannot
// change commit success semantics.
func (c *Consumer) SetCommitObserver(observer func([]kafka.Message)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commitObserver = observer
}

func (c *Consumer) notifyCommitObserver(messages []kafka.Message) {
	c.mu.RLock()
	observer := c.commitObserver
	c.mu.RUnlock()
	if observer == nil {
		return
	}
	committed := append([]kafka.Message(nil), messages...)
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				c.logger.Error("Kafka commit observer panicked after successful commit", zap.Any("panic", recovered))
			}
		}()
		observer(committed)
	}()
}

type ReceivedMessage struct {
	kafka.Message
	headersMap map[string]string
	once       sync.Once
}

func (m *ReceivedMessage) initHeaders() {
	m.once.Do(func() {
		m.headersMap = make(map[string]string, len(m.Headers))
		for _, h := range m.Headers {
			m.headersMap[h.Key] = string(h.Value)
		}
	})
}

func (m *ReceivedMessage) GetHeader(key string) string {
	m.initHeaders()
	return m.headersMap[key]
}

func (m *ReceivedMessage) GetHeaderWithDefault(key, defaultValue string) string {
	m.initHeaders()
	if val, ok := m.headersMap[key]; ok {
		return val
	}
	return defaultValue
}

func (m *ReceivedMessage) HasHeader(key string) bool {
	m.initHeaders()
	_, exists := m.headersMap[key]
	return exists
}

func (m *ReceivedMessage) GetAllHeaders() map[string]string {
	m.initHeaders()

	copied := make(map[string]string, len(m.headersMap))
	for k, v := range m.headersMap {
		copied[k] = v
	}
	return copied
}

func (m *ReceivedMessage) TenantID() string {
	return m.GetHeader("tenant_id")
}

func (m *ReceivedMessage) EventID() string {
	return m.GetHeader("event_id")
}

func (m *ReceivedMessage) TraceID() string {
	return m.GetHeader("trace_id")
}

func (m *ReceivedMessage) RunID() string {
	return m.GetHeader("run_id")
}

func (m *ReceivedMessage) ProbeID() string {
	return m.GetHeader("probe_id")
}

func (m *ReceivedMessage) FeatureSetID() string {
	return m.GetHeader("feature_set_id")
}

func (m *ReceivedMessage) ContentType() string {
	return m.GetHeaderWithDefault("content_type", "application/octet-stream")
}

func (m *ReceivedMessage) IsProtobuf() bool {
	ct := m.ContentType()
	return ct == "application/x-protobuf" || ct == "application/protobuf"
}

func (m *ReceivedMessage) IsJSON() bool {
	ct := m.ContentType()
	return ct == "application/json"
}

func (m *ReceivedMessage) ProtoMessageType() string {
	return m.GetHeader("proto_message_type")
}

func (m *ReceivedMessage) UnmarshalProto(v interface{}) error {
	if message, ok := v.(proto.Message); ok {
		return proto.Unmarshal(m.Value, message)
	}
	if unmarshaler, ok := v.(interface{ Unmarshal([]byte) error }); ok {
		return unmarshaler.Unmarshal(m.Value)
	}
	return fmt.Errorf("type does not implement protobuf message or legacy Unmarshal method")
}

func (m *ReceivedMessage) GetMetadata() map[string]interface{} {
	return map[string]interface{}{
		"topic":        m.Topic,
		"partition":    m.Partition,
		"offset":       m.Offset,
		"key":          string(m.Key),
		"tenant_id":    m.TenantID(),
		"event_id":     m.EventID(),
		"trace_id":     m.TraceID(),
		"run_id":       m.RunID(),
		"content_type": m.ContentType(),
		"timestamp":    m.Time.Format(time.RFC3339),
	}
}

// Context reconstructs the W3C parent carried by Kafka. Legacy 32-hex
// trace_id-only messages remain correlated, while malformed legacy values are
// not trusted as trace identities.
func (m *ReceivedMessage) Context(parent context.Context) context.Context {
	carrier := map[string]string{}
	for key, value := range m.GetAllHeaders() {
		lower := strings.ToLower(strings.TrimSpace(key))
		if lower == "traceparent" || lower == "tracestate" {
			carrier[lower] = value
		}
	}
	ctx := commonotel.ExtractFromMap(parent, carrier)
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		traceID := strings.TrimSpace(m.TraceID())
		if len(traceID) == 32 {
			digest := sha256.Sum256([]byte(m.Topic + ":" + strconv.Itoa(m.Partition) + ":" + strconv.FormatInt(m.Offset, 10)))
			spanID := fmt.Sprintf("%x", digest[:8])
			if fallback, err := commonotel.NewSpanContext(traceID, spanID, true); err == nil && fallback.IsValid() {
				ctx = commonotel.ContextWithRemoteSpanContext(parent, fallback)
				spanContext = fallback
			}
		}
	}
	if spanContext.IsValid() {
		ctx = logging.WithTraceID(ctx, spanContext.TraceID().String())
	}
	return ctx
}

type MessageHandler func(context.Context, *ReceivedMessage) error

type BatchMessageHandler func(context.Context, []*ReceivedMessage) error

func (c *Consumer) Consume(ctx context.Context, handler MessageHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if atomic.LoadInt32(&c.closed) == 1 {
			return fmt.Errorf("consumer is closed")
		}

		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				return err
			}
			c.recordFetchFailure(err)
			c.logger.Error("Failed to fetch message", zap.Error(err))
			time.Sleep(c.config.RetryBackoff)
			continue
		}
		c.recordFetchSuccess()

		atomic.AddInt64(&c.metrics.MessagesReceived, 1)

		receivedMsg := &ReceivedMessage{Message: msg}

		shouldCommit := true
		messageContext := receivedMsg.Context(ctx)
		if err := handler(messageContext, receivedMsg); err != nil {
			atomic.AddInt64(&c.metrics.MessagesFailed, 1)
			c.logger.Error("Message handler error",
				zap.String("topic", msg.Topic),
				zap.Int("partition", msg.Partition),
				zap.Int64("offset", msg.Offset),
				zap.String("event_id", receivedMsg.EventID()),
				zap.Error(err))

			shouldCommit = c.config.CommitOnHandlerError
			if c.dlqProducer != nil && c.dlqEligible(err) {
				if dlqErr := c.dlqProducer.Send(ctx, receivedMsg, err); dlqErr != nil {
					c.logger.Error("Failed to send to DLQ",
						zap.Error(dlqErr),
						zap.String("event_id", receivedMsg.EventID()))

					shouldCommit = false
					c.recordProcessingFailure(dlqErr)
				} else {
					atomic.AddInt64(&c.metrics.MessagesDLQ, 1)

					shouldCommit = c.config.CommitOnDLQSuccess
					if shouldCommit {
						c.recordProcessingSuccess()
					} else {
						c.recordProcessingFailure(err)
					}
					c.logger.Info("Message sent to DLQ",
						zap.String("event_id", receivedMsg.EventID()),
						zap.Bool("will_commit", shouldCommit))
				}
			} else {
				// At-least-once consumers must not lose a message on a transient
				// handler or database failure. Only explicitly permanent errors are
				// eligible for DLQ quarantine; callers can still opt into the old
				// commit-on-error behavior when required.
				c.recordProcessingFailure(err)
			}
			if shouldCommit && (c.dlqProducer == nil || (c.config.DLQPermanentOnly && !IsPermanent(err))) {
				c.recordProcessingSuccess()
			}

			if !shouldCommit {
				c.logger.Warn("Message not committed, will be redelivered",
					zap.Int64("offset", msg.Offset))
				continue
			}
		} else {
			atomic.AddInt64(&c.metrics.MessagesProcessed, 1)
			c.recordProcessingSuccess()
		}

		if c.config.CommitInterval > 0 {
			c.commitChan <- commitRequest{messages: []kafka.Message{msg}}
		} else {
			if err := c.commitMessages([]kafka.Message{msg}); err != nil {
				c.recordProcessingFailure(fmt.Errorf("commit offset: %w", err))
				continue
			}
		}
	}
}

func (c *Consumer) BatchConsume(ctx context.Context, batchSize int, flushInterval time.Duration, handler BatchMessageHandler) error {
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushInterval <= 0 {
		flushInterval = time.Second
	}

	batch := make([]*ReceivedMessage, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	processBatch := func() error {
		if len(batch) == 0 {
			return nil
		}

		shouldCommit := true
		if err := handler(ctx, batch); err != nil {

			c.logger.Error("Batch handler error",
				zap.Int("batch_size", len(batch)),
				zap.Error(err))

			if c.dlqProducer != nil {
				failedMessages := make([]struct {
					Msg *ReceivedMessage
					Err error
				}, len(batch))
				for i, msg := range batch {
					failedMessages[i] = struct {
						Msg *ReceivedMessage
						Err error
					}{Msg: msg, Err: err}
				}

				if dlqErr := c.dlqProducer.SendBatch(ctx, failedMessages); dlqErr != nil {
					c.logger.Error("Failed to send batch to DLQ",
						zap.Error(dlqErr))
					shouldCommit = false
				} else {
					atomic.AddInt64(&c.metrics.MessagesDLQ, int64(len(batch)))
					shouldCommit = c.config.CommitOnDLQSuccess
					c.logger.Info("Batch sent to DLQ",
						zap.Int("count", len(batch)),
						zap.Bool("will_commit", shouldCommit))
				}
			} else {
				shouldCommit = false
			}

			if !shouldCommit {

				return err
			}
		}

		if shouldCommit {
			messages := make([]kafka.Message, len(batch))
			for i, msg := range batch {
				messages[i] = msg.Message
				atomic.AddInt64(&c.metrics.MessagesProcessed, 1)
			}

			if c.config.CommitInterval > 0 {
				doneChan := make(chan error, 1)
				c.commitChan <- commitRequest{
					messages: messages,
					doneChan: doneChan,
				}

				<-doneChan
			} else {
				c.commitMessages(messages)
			}
		}

		batch = batch[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():

			processBatch()
			return ctx.Err()

		case <-ticker.C:

			if err := processBatch(); err != nil {

			}

		default:
		}

		if atomic.LoadInt32(&c.closed) == 1 {
			processBatch()
			return fmt.Errorf("consumer is closed")
		}

		// 动态超时: 空 batch 时使用更长超时避免 busy-loop
		fetchTimeout := flushInterval
		if len(batch) == 0 {
			fetchTimeout = 5 * time.Second
		}
		fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
		msg, err := c.reader.FetchMessage(fetchCtx)
		cancel()

		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {

				if len(batch) > 0 {
					processBatch()
				}
				continue
			}
			c.recordFetchFailure(err)
			c.logger.Error("Failed to fetch message", zap.Error(err))
			time.Sleep(c.config.RetryBackoff)
			continue
		}
		c.recordFetchSuccess()

		atomic.AddInt64(&c.metrics.MessagesReceived, 1)

		receivedMsg := &ReceivedMessage{Message: msg}
		batch = append(batch, receivedMsg)

		if len(batch) >= batchSize {
			if err := processBatch(); err != nil {

			}
		}
	}
}

func (c *Consumer) Commit(ctx context.Context, messages ...kafka.Message) error {
	return c.commitMessages(messages)
}

func (c *Consumer) Lag(ctx context.Context) (int64, error) {
	stats := c.reader.Stats()
	atomic.StoreInt64(&c.metrics.Lag, stats.Lag)
	return stats.Lag, nil
}

func (c *Consumer) GetMetrics() ConsumerMetrics {
	c.mu.RLock()
	lastFetchAt := c.lastFetchAt
	lastProcessingAt := c.lastProcessingAt
	c.mu.RUnlock()
	return ConsumerMetrics{
		MessagesReceived:              atomic.LoadInt64(&c.metrics.MessagesReceived),
		MessagesProcessed:             atomic.LoadInt64(&c.metrics.MessagesProcessed),
		MessagesFailed:                atomic.LoadInt64(&c.metrics.MessagesFailed),
		MessagesDLQ:                   atomic.LoadInt64(&c.metrics.MessagesDLQ),
		CommitsSucceeded:              atomic.LoadInt64(&c.metrics.CommitsSucceeded),
		CommitsFailed:                 atomic.LoadInt64(&c.metrics.CommitsFailed),
		LastOffset:                    atomic.LoadInt64(&c.metrics.LastOffset),
		Lag:                           atomic.LoadInt64(&c.metrics.Lag),
		ConsecutiveFetchFailures:      atomic.LoadInt64(&c.fetchFailures),
		LastFetchErrorUnix:            lastFetchAt.Unix(),
		ConsecutiveProcessingFailures: atomic.LoadInt64(&c.processingFailures),
		LastProcessingErrorUnix:       lastProcessingAt.Unix(),
	}
}

// HealthCheck reports persistent transport/authentication failures without
// treating normal idle-topic fetch timeouts as unhealthy.
func (c *Consumer) HealthCheck() error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return fmt.Errorf("consumer is closed")
	}
	failures := atomic.LoadInt64(&c.fetchFailures)
	if failures < 3 {
		processingFailures := atomic.LoadInt64(&c.processingFailures)
		if processingFailures == 0 {
			return nil
		}
		c.mu.RLock()
		lastErr := c.lastProcessingErr
		c.mu.RUnlock()
		return fmt.Errorf("message processing failed %d consecutive times: %s", processingFailures, lastErr)
	}
	c.mu.RLock()
	lastErr := c.lastFetchErr
	c.mu.RUnlock()
	return fmt.Errorf("kafka fetch failed %d consecutive times: %s", failures, lastErr)
}

func (c *Consumer) recordFetchFailure(err error) {
	atomic.AddInt64(&c.fetchFailures, 1)
	c.mu.Lock()
	c.lastFetchErr = err.Error()
	c.lastFetchAt = time.Now()
	c.mu.Unlock()
}

func (c *Consumer) recordFetchSuccess() {
	atomic.StoreInt64(&c.fetchFailures, 0)
	c.mu.Lock()
	c.lastFetchErr = ""
	c.lastFetchAt = time.Time{}
	c.mu.Unlock()
}

func (c *Consumer) recordProcessingFailure(err error) {
	atomic.AddInt64(&c.processingFailures, 1)
	c.mu.Lock()
	c.lastProcessingErr = err.Error()
	c.lastProcessingAt = time.Now()
	c.mu.Unlock()
}

func (c *Consumer) recordProcessingSuccess() {
	atomic.StoreInt64(&c.processingFailures, 0)
	c.mu.Lock()
	c.lastProcessingErr = ""
	c.lastProcessingAt = time.Time{}
	c.mu.Unlock()
}

func (c *Consumer) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}

	c.logger.Info("Closing Kafka consumer")

	close(c.stopCommitter)

	if c.dlqProducer != nil {
		if err := c.dlqProducer.Close(); err != nil {
			c.logger.Error("Failed to close DLQ producer", zap.Error(err))
		}
	}

	if err := c.reader.Close(); err != nil {
		c.logger.Error("Failed to close reader", zap.Error(err))
		return err
	}

	c.logger.Info("Kafka consumer closed")
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/kafka/consumer.go
// 修复版本 v2：
// 1. 修复 #2：ReceivedMessage 增加通用 Header 辅助方法
// 2. 增强消息元数据提取能力
// 3. 增加 Protobuf 类型自动识别
////////////////////////////////////////////////////////////////////////////////

package kafka

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// ConsumerConfig Kafka消费者配置
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

	// 修复：新增配置选项
	CommitOnDLQSuccess bool // DLQ 发送成功后是否提交 offset（默认 true）
}

// Consumer Kafka消费者
type Consumer struct {
	reader        *kafka.Reader
	config        ConsumerConfig
	logger        *zap.Logger
	metrics       ConsumerMetrics
	closed        int32
	mu            sync.RWMutex
	commitChan    chan commitRequest
	stopCommitter chan struct{}
	dlqProducer   *DLQProducer
}

// ConsumerMetrics 消费者指标
type ConsumerMetrics struct {
	MessagesReceived  int64
	MessagesProcessed int64
	MessagesFailed    int64
	MessagesDLQ       int64
	CommitsSucceeded  int64
	CommitsFailed     int64
	LastOffset        int64
	Lag               int64
}

// commitRequest 提交请求
type commitRequest struct {
	messages []kafka.Message
	doneChan chan error
}

// NewConsumer 创建Kafka消费者
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

	// 设置默认值
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

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        config.Brokers,
		Topic:          config.Topic,
		GroupID:        config.GroupID,
		MinBytes:       config.MinBytes,
		MaxBytes:       config.MaxBytes,
		MaxWait:        config.MaxWait,
		StartOffset:    config.StartOffset,
		CommitInterval: 0, // 手动提交
	})

	c := &Consumer{
		reader:        reader,
		config:        config,
		logger:        logger,
		commitChan:    make(chan commitRequest, 100),
		stopCommitter: make(chan struct{}),
	}

	// 初始化 DLQ Producer
	if config.EnableDLQ {
		dlqConfig := DLQConfig{
			Brokers:     config.Brokers,
			TopicPrefix: config.DLQTopicPrefix,
			BatchSize:   100,
			MaxRetries:  3,
			RetryDelay:  100 * time.Millisecond,
		}
		c.dlqProducer = NewDLQProducer(dlqConfig, "consumer-"+config.Topic, logger)
	}

	// 启动后台提交器
	if config.CommitInterval > 0 {
		go c.backgroundCommitter()
	}

	return c, nil
}

// backgroundCommitter 后台提交器
func (c *Consumer) backgroundCommitter() {
	ticker := time.NewTicker(c.config.CommitInterval)
	defer ticker.Stop()

	var pendingMessages []kafka.Message
	var mu sync.Mutex

	for {
		select {
		case <-c.stopCommitter:
			// 提交剩余消息
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

// commitMessages 提交消息
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

	return nil
}

// ==================== 修复 #2：增强的 ReceivedMessage ====================

// ReceivedMessage 接收到的消息（增强版）
type ReceivedMessage struct {
	kafka.Message
	headersMap map[string]string // 缓存的 Header 映射
	once       sync.Once
}

// initHeaders 初始化 Header 映射（懒加载）
func (m *ReceivedMessage) initHeaders() {
	m.once.Do(func() {
		m.headersMap = make(map[string]string, len(m.Headers))
		for _, h := range m.Headers {
			m.headersMap[h.Key] = string(h.Value)
		}
	})
}

// GetHeader 获取指定 Header 值（新增：通用方法）
func (m *ReceivedMessage) GetHeader(key string) string {
	m.initHeaders()
	return m.headersMap[key]
}

// GetHeaderWithDefault 获取 Header 值，不存在时返回默认值（新增）
func (m *ReceivedMessage) GetHeaderWithDefault(key, defaultValue string) string {
	m.initHeaders()
	if val, ok := m.headersMap[key]; ok {
		return val
	}
	return defaultValue
}

// HasHeader 检查 Header 是否存在（新增）
func (m *ReceivedMessage) HasHeader(key string) bool {
	m.initHeaders()
	_, exists := m.headersMap[key]
	return exists
}

// GetAllHeaders 获取所有 Header（新增）
func (m *ReceivedMessage) GetAllHeaders() map[string]string {
	m.initHeaders()
	// 返回副本防止外部修改
	copied := make(map[string]string, len(m.headersMap))
	for k, v := range m.headersMap {
		copied[k] = v
	}
	return copied
}

// TenantID 从消息头提取租户ID
func (m *ReceivedMessage) TenantID() string {
	return m.GetHeader("tenant_id")
}

// EventID 从消息头提取事件ID
func (m *ReceivedMessage) EventID() string {
	return m.GetHeader("event_id")
}

// TraceID 从消息头提取追踪ID
func (m *ReceivedMessage) TraceID() string {
	return m.GetHeader("trace_id")
}

// RunID 从消息头提取运行批次ID（新增）
func (m *ReceivedMessage) RunID() string {
	return m.GetHeader("run_id")
}

// ProbeID 从消息头提取探针ID（新增）
func (m *ReceivedMessage) ProbeID() string {
	return m.GetHeader("probe_id")
}

// FeatureSetID 从消息头提取特征集ID（新增）
func (m *ReceivedMessage) FeatureSetID() string {
	return m.GetHeader("feature_set_id")
}

// ContentType 获取内容类型（新增：用于识别 Protobuf/JSON）
func (m *ReceivedMessage) ContentType() string {
	return m.GetHeaderWithDefault("content_type", "application/octet-stream")
}

// IsProtobuf 检查是否为 Protobuf 消息（新增）
func (m *ReceivedMessage) IsProtobuf() bool {
	ct := m.ContentType()
	return ct == "application/x-protobuf" || ct == "application/protobuf"
}

// IsJSON 检查是否为 JSON 消息（新增）
func (m *ReceivedMessage) IsJSON() bool {
	ct := m.ContentType()
	return ct == "application/json"
}

// ProtoMessageType 获取 Protobuf 消息类型全名（新增）
// 示例：traffic.v1.FlowEvent
func (m *ReceivedMessage) ProtoMessageType() string {
	return m.GetHeader("proto_message_type")
}

// UnmarshalProto 反序列化Protobuf消息
func (m *ReceivedMessage) UnmarshalProto(v interface{}) error {
	if unmarshaler, ok := v.(interface{ Unmarshal([]byte) error }); ok {
		return unmarshaler.Unmarshal(m.Value)
	}
	return fmt.Errorf("type does not implement Unmarshal method")
}

// GetMetadata 获取消息元数据摘要（新增：用于日志）
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

// MessageHandler 消息处理函数
type MessageHandler func(context.Context, *ReceivedMessage) error

// BatchMessageHandler 批量消息处理函数
type BatchMessageHandler func(context.Context, []*ReceivedMessage) error

// Consume 消费消息（单条处理）- 修复版
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
			c.logger.Error("Failed to fetch message", zap.Error(err))
			time.Sleep(c.config.RetryBackoff)
			continue
		}

		atomic.AddInt64(&c.metrics.MessagesReceived, 1)

		receivedMsg := &ReceivedMessage{Message: msg}

		// 处理消息
		shouldCommit := true
		if err := handler(ctx, receivedMsg); err != nil {
			atomic.AddInt64(&c.metrics.MessagesFailed, 1)
			c.logger.Error("Message handler error",
				zap.String("topic", msg.Topic),
				zap.Int("partition", msg.Partition),
				zap.Int64("offset", msg.Offset),
				zap.String("event_id", receivedMsg.EventID()),
				zap.Error(err))

			// 修复：发送到 DLQ
			if c.dlqProducer != nil {
				if dlqErr := c.dlqProducer.Send(ctx, receivedMsg, err); dlqErr != nil {
					c.logger.Error("Failed to send to DLQ",
						zap.Error(dlqErr),
						zap.String("event_id", receivedMsg.EventID()))
					// DLQ 发送失败，不提交 offset
					shouldCommit = false
				} else {
					atomic.AddInt64(&c.metrics.MessagesDLQ, 1)
					// 修复：DLQ 发送成功，根据配置决定是否提交
					shouldCommit = c.config.CommitOnDLQSuccess
					c.logger.Info("Message sent to DLQ",
						zap.String("event_id", receivedMsg.EventID()),
						zap.Bool("will_commit", shouldCommit))
				}
			} else {
				// 没有启用 DLQ，不提交 offset（让消息重新投递）
				shouldCommit = false
			}

			if !shouldCommit {
				c.logger.Warn("Message not committed, will be redelivered",
					zap.Int64("offset", msg.Offset))
				continue
			}
		} else {
			atomic.AddInt64(&c.metrics.MessagesProcessed, 1)
		}

		// 提交offset
		if c.config.CommitInterval > 0 {
			c.commitChan <- commitRequest{messages: []kafka.Message{msg}}
		} else {
			c.commitMessages([]kafka.Message{msg})
		}
	}
}

// BatchConsume 批量消费消息 - 修复版
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
			// 处理失败时的逻辑
			c.logger.Error("Batch handler error",
				zap.Int("batch_size", len(batch)),
				zap.Error(err))

			// 修复：批量发送到 DLQ
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
				// 不清空batch，等待下次重试
				return err
			}
		}

		// 提交offset
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
				// 等待提交完成
				<-doneChan
			} else {
				c.commitMessages(messages)
			}
		}

		// 清空batch
		batch = batch[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			// 处理剩余batch
			processBatch()
			return ctx.Err()

		case <-ticker.C:
			// 定时刷新
			if err := processBatch(); err != nil {
				// 继续消费
			}

		default:
		}

		if atomic.LoadInt32(&c.closed) == 1 {
			processBatch()
			return fmt.Errorf("consumer is closed")
		}

		// 设置读取超时
		fetchCtx, cancel := context.WithTimeout(ctx, flushInterval)
		msg, err := c.reader.FetchMessage(fetchCtx)
		cancel()

		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				// 超时，处理当前batch
				if len(batch) > 0 {
					processBatch()
				}
				continue
			}
			c.logger.Error("Failed to fetch message", zap.Error(err))
			time.Sleep(c.config.RetryBackoff)
			continue
		}

		atomic.AddInt64(&c.metrics.MessagesReceived, 1)

		receivedMsg := &ReceivedMessage{Message: msg}
		batch = append(batch, receivedMsg)

		// 达到batch大小，立即处理
		if len(batch) >= batchSize {
			if err := processBatch(); err != nil {
				// 处理失败，继续累积
			}
		}
	}
}

// Commit 手动提交offset
func (c *Consumer) Commit(ctx context.Context, messages ...kafka.Message) error {
	return c.commitMessages(messages)
}

// Lag 获取消费延迟
func (c *Consumer) Lag(ctx context.Context) (int64, error) {
	stats := c.reader.Stats()
	return stats.Lag, nil
}

// GetMetrics 获取消费者指标
func (c *Consumer) GetMetrics() ConsumerMetrics {
	return ConsumerMetrics{
		MessagesReceived:  atomic.LoadInt64(&c.metrics.MessagesReceived),
		MessagesProcessed: atomic.LoadInt64(&c.metrics.MessagesProcessed),
		MessagesFailed:    atomic.LoadInt64(&c.metrics.MessagesFailed),
		MessagesDLQ:       atomic.LoadInt64(&c.metrics.MessagesDLQ),
		CommitsSucceeded:  atomic.LoadInt64(&c.metrics.CommitsSucceeded),
		CommitsFailed:     atomic.LoadInt64(&c.metrics.CommitsFailed),
		LastOffset:        atomic.LoadInt64(&c.metrics.LastOffset),
		Lag:               atomic.LoadInt64(&c.metrics.Lag),
	}
}

// Close 关闭消费者
func (c *Consumer) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}

	c.logger.Info("Closing Kafka consumer")

	// 停止后台提交器
	close(c.stopCommitter)

	// 关闭 DLQ Producer
	if c.dlqProducer != nil {
		if err := c.dlqProducer.Close(); err != nil {
			c.logger.Error("Failed to close DLQ producer", zap.Error(err))
		}
	}

	// 关闭reader
	if err := c.reader.Close(); err != nil {
		c.logger.Error("Failed to close reader", zap.Error(err))
		return err
	}

	c.logger.Info("Kafka consumer closed")
	return nil
}

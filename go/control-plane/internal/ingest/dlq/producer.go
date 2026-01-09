////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/dlq/producer.go
// 修复版 v5：
// 1. 修复 decodeBase64 实现（使用标准库）
// 2. 优化 splitLines/splitPipe 实现
// 3. 完整保留所有功能
////////////////////////////////////////////////////////////////////////////////

package dlq

import (
	"context"
	"encoding/base64" // 修复：添加标准库导入
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// Config DLQ 配置
type Config struct {
	// Kafka DLQ Topic
	Brokers      []string
	DLQTopic     string
	BatchSize    int
	MaxRetries   int
	RetryBackoff time.Duration

	// 文件降级配置
	EnableFallback  bool
	FallbackDir     string
	MaxFallbackSize int64 // 单个降级文件最大大小（字节）

	// 回放配置
	ReplayInterval  time.Duration // 回放检查间隔
	ReplayBatchSize int           // 回放批次大小
}

// DefaultConfig 默认配置
func DefaultConfig(brokers []string) Config {
	return Config{
		Brokers:         brokers,
		DLQTopic:        "dlq.ingest-gateway",
		BatchSize:       100,
		MaxRetries:      3,
		RetryBackoff:    100 * time.Millisecond,
		EnableFallback:  true,
		FallbackDir:     "/var/log/ingest-gateway/dlq-fallback",
		MaxFallbackSize: 100 * 1024 * 1024, // 100MB
		ReplayInterval:  5 * time.Minute,
		ReplayBatchSize: 1000,
	}
}

// Producer DLQ 生产者
type Producer struct {
	config Config
	logger *zap.Logger

	// Kafka Writer
	writer *kafka.Writer

	// 文件降级
	fallbackMu      sync.Mutex
	fallbackFile    *os.File
	fallbackSize    int64
	fallbackSeqNum  int64
	fallbackEnabled bool

	// 原始 Topic Writers（用于回放）
	flowWriter    *kafka.Writer
	pcapWriter    *kafka.Writer
	sessionWriter *kafka.Writer

	// 统计
	kafkaSuccessCount int64
	kafkaFailCount    int64
	fallbackCount     int64
	replayCount       int64

	// 关闭控制
	closed    int32
	closeChan chan struct{}
	wg        sync.WaitGroup
}

// NewProducer 创建 DLQ 生产者
func NewProducer(cfg Config, logger *zap.Logger) *Producer {
	// 验证配置
	if cfg.DLQTopic == "" {
		cfg.DLQTopic = "dlq.ingest-gateway"
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 100 * time.Millisecond
	}
	if cfg.ReplayInterval <= 0 {
		cfg.ReplayInterval = 5 * time.Minute
	}
	if cfg.ReplayBatchSize <= 0 {
		cfg.ReplayBatchSize = 1000
	}

	// 创建 Kafka Writer（DLQ）
	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.DLQTopic,
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    cfg.BatchSize,
		BatchTimeout: 100 * time.Millisecond,
		Compression:  kafka.Lz4,
		MaxAttempts:  cfg.MaxRetries,
		Async:        false, // 同步写入，确保不丢失
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logger.Error(fmt.Sprintf(msg, args...))
		}),
	}

	// 创建原始 Topic Writers（用于回放）
	flowWriter := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        "flow.events.v1",
		Balancer:     &kafka.Hash{},
		BatchSize:    1000,
		BatchTimeout: 100 * time.Millisecond,
		Compression:  kafka.Lz4,
		MaxAttempts:  3,
		Async:        false,
	}

	pcapWriter := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        "pcap.index.v1",
		Balancer:     &kafka.Hash{},
		BatchSize:    100,
		BatchTimeout: 100 * time.Millisecond,
		Compression:  kafka.Lz4,
		MaxAttempts:  3,
		Async:        false,
	}

	sessionWriter := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        "session.events.v1",
		Balancer:     &kafka.Hash{},
		BatchSize:    1000,
		BatchTimeout: 100 * time.Millisecond,
		Compression:  kafka.Lz4,
		MaxAttempts:  3,
		Async:        false,
	}

	p := &Producer{
		config:          cfg,
		logger:          logger,
		writer:          writer,
		flowWriter:      flowWriter,
		pcapWriter:      pcapWriter,
		sessionWriter:   sessionWriter,
		closeChan:       make(chan struct{}),
		fallbackEnabled: cfg.EnableFallback,
	}

	// 创建降级目录
	if cfg.EnableFallback && cfg.FallbackDir != "" {
		if err := os.MkdirAll(cfg.FallbackDir, 0755); err != nil {
			logger.Warn("Failed to create fallback directory, disabling fallback",
				zap.String("dir", cfg.FallbackDir),
				zap.Error(err))
			p.fallbackEnabled = false
		} else {
			logger.Info("DLQ fallback directory ready",
				zap.String("dir", cfg.FallbackDir))
		}
	}

	logger.Info("DLQ producer initialized",
		zap.String("dlq_topic", cfg.DLQTopic),
		zap.Bool("fallback_enabled", p.fallbackEnabled),
		zap.String("fallback_dir", cfg.FallbackDir))

	return p
}

// DLQMessage DLQ 消息包装
type DLQMessage struct {
	OriginalTopic string            `json:"original_topic"`
	EventType     string            `json:"event_type"` // flow, pcap, session
	TenantID      string            `json:"tenant_id"`
	ProbeID       string            `json:"probe_id"`
	EventID       string            `json:"event_id"`
	FailedAt      time.Time         `json:"failed_at"`
	ErrorMessage  string            `json:"error_message"`
	RetryCount    int               `json:"retry_count"`
	Headers       map[string]string `json:"headers"`
	PayloadBase64 string            `json:"payload_base64"` // Protobuf 消息的 base64
}

// SendFlowEvents 发送 Flow 事件到 DLQ
func (p *Producer) SendFlowEvents(ctx context.Context, events []*pb.FlowEvent, err error) error {
	if len(events) == 0 {
		return nil
	}

	messages := make([]kafka.Message, 0, len(events))

	for _, event := range events {
		if event == nil || event.Header == nil {
			continue
		}

		// 序列化 Protobuf
		payload, marshalErr := proto.Marshal(event)
		if marshalErr != nil {
			p.logger.Error("Failed to marshal flow event for DLQ",
				zap.String("event_id", event.Header.EventId),
				zap.Error(marshalErr))
			continue
		}

		// 构建 DLQ 消息
		dlqMsg := &DLQMessage{
			OriginalTopic: "flow.events.v1",
			EventType:     "flow",
			TenantID:      event.Header.TenantId,
			ProbeID:       event.Header.ProbeId,
			EventID:       event.Header.EventId,
			FailedAt:      time.Now(),
			ErrorMessage:  err.Error(),
			RetryCount:    0,
			Headers: map[string]string{
				"tenant_id":          event.Header.TenantId,
				"probe_id":           event.Header.ProbeId,
				"event_id":           event.Header.EventId,
				"run_id":             event.Header.RunId,
				"feature_set_id":     event.Header.FeatureSetId,
				"community_id":       event.CommunityId,
				"content_type":       "application/x-protobuf",
				"proto_message_type": "traffic.v1.FlowEvent",
			},
			PayloadBase64: encodeBase64(payload),
		}

		// 序列化为 JSON
		msgData, jsonErr := json.Marshal(dlqMsg)
		if jsonErr != nil {
			p.logger.Error("Failed to marshal DLQ message",
				zap.String("event_id", event.Header.EventId),
				zap.Error(jsonErr))
			continue
		}

		// 构建 Kafka 消息
		key := fmt.Sprintf("%s:%s", event.Header.TenantId, event.Header.EventId)
		messages = append(messages, kafka.Message{
			Key:   []byte(key),
			Value: msgData,
			Headers: []kafka.Header{
				{Key: "original_topic", Value: []byte("flow.events.v1")},
				{Key: "event_type", Value: []byte("flow")},
				{Key: "tenant_id", Value: []byte(event.Header.TenantId)},
				{Key: "event_id", Value: []byte(event.Header.EventId)},
				{Key: "failed_at", Value: []byte(time.Now().Format(time.RFC3339))},
			},
		})
	}

	if len(messages) == 0 {
		return nil
	}

	// 尝试写入 Kafka DLQ
	writeErr := p.writeToKafka(ctx, messages)
	if writeErr != nil {
		// Kafka 失败，降级到文件
		if p.fallbackEnabled {
			return p.writeToFallback(messages)
		}
		return writeErr
	}

	atomic.AddInt64(&p.kafkaSuccessCount, int64(len(messages)))
	return nil
}

// SendPcapIndex 发送 PCAP 索引到 DLQ
func (p *Producer) SendPcapIndex(ctx context.Context, meta *pb.PcapIndexMeta, err error) error {
	if meta == nil {
		return nil
	}

	// 序列化 Protobuf
	payload, marshalErr := proto.Marshal(meta)
	if marshalErr != nil {
		p.logger.Error("Failed to marshal pcap index for DLQ",
			zap.String("file_key", meta.FileKey),
			zap.Error(marshalErr))
		return marshalErr
	}

	// 构建 DLQ 消息
	dlqMsg := &DLQMessage{
		OriginalTopic: "pcap.index.v1",
		EventType:     "pcap",
		TenantID:      meta.TenantId,
		ProbeID:       meta.ProbeId,
		EventID:       fmt.Sprintf("pcap:%s", meta.FileKey),
		FailedAt:      time.Now(),
		ErrorMessage:  err.Error(),
		RetryCount:    0,
		Headers: map[string]string{
			"tenant_id":          meta.TenantId,
			"probe_id":           meta.ProbeId,
			"file_key":           meta.FileKey,
			"content_type":       "application/x-protobuf",
			"proto_message_type": "traffic.v1.PcapIndexMeta",
		},
		PayloadBase64: encodeBase64(payload),
	}

	// 序列化为 JSON
	msgData, jsonErr := json.Marshal(dlqMsg)
	if jsonErr != nil {
		p.logger.Error("Failed to marshal DLQ message",
			zap.String("file_key", meta.FileKey),
			zap.Error(jsonErr))
		return jsonErr
	}

	// 构建 Kafka 消息
	key := fmt.Sprintf("%s:%s", meta.TenantId, meta.FileKey)
	msg := kafka.Message{
		Key:   []byte(key),
		Value: msgData,
		Headers: []kafka.Header{
			{Key: "original_topic", Value: []byte("pcap.index.v1")},
			{Key: "event_type", Value: []byte("pcap")},
			{Key: "tenant_id", Value: []byte(meta.TenantId)},
			{Key: "file_key", Value: []byte(meta.FileKey)},
			{Key: "failed_at", Value: []byte(time.Now().Format(time.RFC3339))},
		},
	}

	// 写入 Kafka
	writeErr := p.writeToKafka(ctx, []kafka.Message{msg})
	if writeErr != nil {
		// 降级到文件
		if p.fallbackEnabled {
			return p.writeToFallback([]kafka.Message{msg})
		}
		return writeErr
	}

	atomic.AddInt64(&p.kafkaSuccessCount, 1)
	return nil
}

// SendSessionEvents 发送 Session 事件到 DLQ
func (p *Producer) SendSessionEvents(ctx context.Context, sessions []*pb.SessionEvent, err error) error {
	if len(sessions) == 0 {
		return nil
	}

	messages := make([]kafka.Message, 0, len(sessions))

	for _, session := range sessions {
		if session == nil || session.Header == nil {
			continue
		}

		// 序列化 Protobuf
		payload, marshalErr := proto.Marshal(session)
		if marshalErr != nil {
			p.logger.Error("Failed to marshal session event for DLQ",
				zap.String("session_id", session.SessionId),
				zap.Error(marshalErr))
			continue
		}

		// 构建 DLQ 消息
		dlqMsg := &DLQMessage{
			OriginalTopic: "session.events.v1",
			EventType:     "session",
			TenantID:      session.Header.TenantId,
			ProbeID:       session.Header.ProbeId,
			EventID:       session.Header.EventId,
			FailedAt:      time.Now(),
			ErrorMessage:  err.Error(),
			RetryCount:    0,
			Headers: map[string]string{
				"tenant_id":          session.Header.TenantId,
				"probe_id":           session.Header.ProbeId,
				"event_id":           session.Header.EventId,
				"session_id":         session.SessionId,
				"community_id":       session.CommunityId,
				"content_type":       "application/x-protobuf",
				"proto_message_type": "traffic.v1.SessionEvent",
			},
			PayloadBase64: encodeBase64(payload),
		}

		// 序列化为 JSON
		msgData, jsonErr := json.Marshal(dlqMsg)
		if jsonErr != nil {
			p.logger.Error("Failed to marshal DLQ message",
				zap.String("session_id", session.SessionId),
				zap.Error(jsonErr))
			continue
		}

		// 构建 Kafka 消息
		key := fmt.Sprintf("%s:%s", session.Header.TenantId, session.SessionId)
		messages = append(messages, kafka.Message{
			Key:   []byte(key),
			Value: msgData,
			Headers: []kafka.Header{
				{Key: "original_topic", Value: []byte("session.events.v1")},
				{Key: "event_type", Value: []byte("session")},
				{Key: "tenant_id", Value: []byte(session.Header.TenantId)},
				{Key: "session_id", Value: []byte(session.SessionId)},
				{Key: "failed_at", Value: []byte(time.Now().Format(time.RFC3339))},
			},
		})
	}

	if len(messages) == 0 {
		return nil
	}

	// 写入 Kafka
	writeErr := p.writeToKafka(ctx, messages)
	if writeErr != nil {
		// 降级到文件
		if p.fallbackEnabled {
			return p.writeToFallback(messages)
		}
		return writeErr
	}

	atomic.AddInt64(&p.kafkaSuccessCount, int64(len(messages)))
	return nil
}

// writeToKafka 写入 Kafka（带重试）
func (p *Producer) writeToKafka(ctx context.Context, messages []kafka.Message) error {
	var lastErr error

	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(p.config.RetryBackoff * time.Duration(attempt))
		}

		err := p.writer.WriteMessages(ctx, messages...)
		if err == nil {
			return nil
		}

		lastErr = err
		p.logger.Warn("Failed to write to DLQ Kafka, retrying",
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", p.config.MaxRetries))
	}

	atomic.AddInt64(&p.kafkaFailCount, int64(len(messages)))
	return lastErr
}

// writeToFallback 降级到文件
func (p *Producer) writeToFallback(messages []kafka.Message) error {
	p.fallbackMu.Lock()
	defer p.fallbackMu.Unlock()

	// 检查或创建新文件
	if p.fallbackFile == nil || p.fallbackSize >= p.config.MaxFallbackSize {
		if err := p.rotateFallbackFile(); err != nil {
			return fmt.Errorf("failed to rotate fallback file: %w", err)
		}
	}

	// 写入消息
	for _, msg := range messages {
		// 构建行格式：topic|key|value\n
		line := fmt.Sprintf("%s|%s|%s\n", msg.Topic, string(msg.Key), string(msg.Value))
		n, err := p.fallbackFile.WriteString(line)
		if err != nil {
			return fmt.Errorf("failed to write to fallback file: %w", err)
		}
		p.fallbackSize += int64(n)
	}

	// 刷盘
	if err := p.fallbackFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync fallback file: %w", err)
	}

	atomic.AddInt64(&p.fallbackCount, int64(len(messages)))

	p.logger.Info("Messages written to DLQ fallback file",
		zap.Int("count", len(messages)),
		zap.Int64("file_size", p.fallbackSize))

	return nil
}

// rotateFallbackFile 轮转降级文件
func (p *Producer) rotateFallbackFile() error {
	// 关闭旧文件
	if p.fallbackFile != nil {
		p.fallbackFile.Close()
	}

	// 创建新文件
	seqNum := atomic.AddInt64(&p.fallbackSeqNum, 1)
	filename := fmt.Sprintf("dlq-fallback-%d-%d.log", time.Now().Unix(), seqNum)
	filepath := filepath.Join(p.config.FallbackDir, filename)

	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	p.fallbackFile = file
	p.fallbackSize = 0

	p.logger.Info("Rotated DLQ fallback file", zap.String("file", filepath))
	return nil
}

// StartFallbackReplay 启动降级文件回放任务
func (p *Producer) StartFallbackReplay(ctx context.Context, interval time.Duration) {
	if !p.fallbackEnabled {
		return
	}

	if interval <= 0 {
		interval = p.config.ReplayInterval
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		p.logger.Info("DLQ fallback replay started", zap.Duration("interval", interval))

		for {
			select {
			case <-ctx.Done():
				p.logger.Info("DLQ fallback replay stopped")
				return
			case <-p.closeChan:
				p.logger.Info("DLQ fallback replay stopped (producer closed)")
				return
			case <-ticker.C:
				p.replayFallbackFiles(ctx)
			}
		}
	}()
}

// replayFallbackFiles 回放降级文件
func (p *Producer) replayFallbackFiles(ctx context.Context) {
	if !p.fallbackEnabled || p.config.FallbackDir == "" {
		return
	}

	// 列出所有降级文件
	files, err := ioutil.ReadDir(p.config.FallbackDir)
	if err != nil {
		p.logger.Error("Failed to read fallback directory", zap.Error(err))
		return
	}

	if len(files) == 0 {
		return
	}

	p.logger.Info("Starting DLQ fallback replay", zap.Int("file_count", len(files)))

	successCount := 0
	failCount := 0

	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			continue
		}

		// 跳过当前正在写入的文件
		p.fallbackMu.Lock()
		isCurrent := p.fallbackFile != nil && fileInfo.Name() == filepath.Base(p.fallbackFile.Name())
		p.fallbackMu.Unlock()

		if isCurrent {
			continue
		}

		// 回放文件
		filePath := filepath.Join(p.config.FallbackDir, fileInfo.Name())
		if err := p.replayFile(ctx, filePath); err != nil {
			p.logger.Error("Failed to replay fallback file",
				zap.String("file", filePath),
				zap.Error(err))
			failCount++
		} else {
			successCount++
		}
	}

	if successCount > 0 || failCount > 0 {
		p.logger.Info("DLQ fallback replay completed",
			zap.Int("success", successCount),
			zap.Int("failed", failCount))
	}
}

// replayFile 回放单个文件
func (p *Producer) replayFile(ctx context.Context, filePath string) error {
	totalSuccess := 0
	totalFailed := 0
	// 读取文件
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// 解析行
	lines := splitLines(string(data))
	if len(lines) == 0 {
		// 空文件，直接删除
		return os.Remove(filePath)
	}

	// 按批次回放
	batch := make([]kafka.Message, 0, p.config.ReplayBatchSize)

	for _, line := range lines {
		if line == "" {
			continue
		}

		// 解析行格式：topic|key|value
		parts := splitPipe(line)
		if len(parts) != 3 {
			p.logger.Warn("Invalid fallback line format", zap.String("line", line))
			continue
		}

		topic := parts[0]
		key := parts[1]
		value := parts[2]

		// 解析 DLQ 消息
		var dlqMsg DLQMessage
		if err := json.Unmarshal([]byte(value), &dlqMsg); err != nil {
			p.logger.Warn("Failed to unmarshal DLQ message", zap.Error(err))
			continue
		}

		// 修复：使用本地 decodeBase64 实现
		payload, err := decodeBase64(dlqMsg.PayloadBase64)
		if err != nil {
			p.logger.Warn("Failed to decode payload", zap.Error(err))
			continue
		}

		// 构建原始消息
		originalMsg := kafka.Message{
			Topic:   dlqMsg.OriginalTopic,
			Key:     []byte(key),
			Value:   payload,
			Headers: buildHeaders(dlqMsg.Headers),
		}

		batch = append(batch, originalMsg)

		// 批次满时回放
		if len(batch) >= p.config.ReplayBatchSize {
			if err := p.replayBatch(ctx, batch); err != nil {
				p.logger.Error("Batch replay failed",
					zap.String("file", filePath),
					zap.Int("batch_size", len(batch)),
					zap.Error(err))
				totalFailed += len(batch)
			} else {
				totalSuccess += len(batch)
			}

			batch = batch[:0]
		}
	}

	// 回放剩余消息
	if len(batch) > 0 {
		if err := p.replayBatch(ctx, batch); err != nil {
			p.logger.Error("Final batch replay failed", zap.Error(err))
			totalFailed += len(batch)
		} else {
			totalSuccess += len(batch)
		}
	}

	// 回放成功，删除文件
	successRate := float64(totalSuccess) / float64(totalSuccess+totalFailed)
	if successRate > 0.5 { // 超过 50% 成功
		if err := os.Remove(filePath); err != nil {
			p.logger.Warn("Failed to delete replayed file", zap.Error(err))
		} else {
			p.logger.Info("Replayed file deleted",
				zap.String("file", filePath),
				zap.Int("success", totalSuccess),
				zap.Int("failed", totalFailed))
		}
	} else {
		p.logger.Warn("Replay success rate too low, keeping file",
			zap.String("file", filePath),
			zap.Float64("success_rate", successRate))
		return fmt.Errorf("replay success rate %.2f%% < 50%%", successRate*100)
	}

	return nil

}

// replayBatch 回放批次（写入原始 Topic）
func (p *Producer) replayBatch(ctx context.Context, messages []kafka.Message) error {
	if len(messages) == 0 {
		return nil
	}

	// 按 Topic 分组
	flowMsgs := make([]kafka.Message, 0)
	pcapMsgs := make([]kafka.Message, 0)
	sessionMsgs := make([]kafka.Message, 0)

	for _, msg := range messages {
		switch msg.Topic {
		case "flow.events.v1":
			flowMsgs = append(flowMsgs, msg)
		case "pcap.index.v1":
			pcapMsgs = append(pcapMsgs, msg)
		case "session.events.v1":
			sessionMsgs = append(sessionMsgs, msg)
		}
	}

	// 回放 Flow 事件
	if len(flowMsgs) > 0 {
		if err := p.flowWriter.WriteMessages(ctx, flowMsgs...); err != nil {
			return fmt.Errorf("failed to replay flow messages: %w", err)
		}
		atomic.AddInt64(&p.replayCount, int64(len(flowMsgs)))
	}

	// 回放 PCAP 索引
	if len(pcapMsgs) > 0 {
		if err := p.pcapWriter.WriteMessages(ctx, pcapMsgs...); err != nil {
			return fmt.Errorf("failed to replay pcap messages: %w", err)
		}
		atomic.AddInt64(&p.replayCount, int64(len(pcapMsgs)))
	}

	// 回放 Session 事件
	if len(sessionMsgs) > 0 {
		if err := p.sessionWriter.WriteMessages(ctx, sessionMsgs...); err != nil {
			return fmt.Errorf("failed to replay session messages: %w", err)
		}
		atomic.AddInt64(&p.replayCount, int64(len(sessionMsgs)))
	}

	return nil
}

// GetFallbackStats 获取降级文件统计
func (p *Producer) GetFallbackStats() (fileCount int, totalSize int64, err error) {
	if !p.fallbackEnabled || p.config.FallbackDir == "" {
		return 0, 0, nil
	}

	files, err := ioutil.ReadDir(p.config.FallbackDir)
	if err != nil {
		return 0, 0, err
	}

	for _, f := range files {
		if !f.IsDir() {
			fileCount++
			totalSize += f.Size()
		}
	}

	return fileCount, totalSize, nil
}

// GetStats 获取统计信息
func (p *Producer) GetStats() DLQStats {
	return DLQStats{
		KafkaSuccessCount: atomic.LoadInt64(&p.kafkaSuccessCount),
		KafkaFailCount:    atomic.LoadInt64(&p.kafkaFailCount),
		FallbackCount:     atomic.LoadInt64(&p.fallbackCount),
		ReplayCount:       atomic.LoadInt64(&p.replayCount),
	}
}

// DLQStats DLQ 统计
type DLQStats struct {
	KafkaSuccessCount int64
	KafkaFailCount    int64
	FallbackCount     int64
	ReplayCount       int64
}

// Close 关闭 DLQ 生产者
func (p *Producer) Close() error {
	if !atomic.CompareAndSwapInt32(&p.closed, 0, 1) {
		return nil
	}

	p.logger.Info("Closing DLQ producer")

	// 关闭回放任务
	close(p.closeChan)

	// 等待回放任务退出
	p.wg.Wait()

	// 关闭降级文件
	p.fallbackMu.Lock()
	if p.fallbackFile != nil {
		p.fallbackFile.Close()
	}
	p.fallbackMu.Unlock()

	// 关闭 Kafka Writers
	var errs []error
	if err := p.writer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close dlq writer: %w", err))
	}
	if err := p.flowWriter.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close flow writer: %w", err))
	}
	if err := p.pcapWriter.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close pcap writer: %w", err))
	}
	if err := p.sessionWriter.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close session writer: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing DLQ producer: %v", errs)
	}

	p.logger.Info("DLQ producer closed")
	return nil
}

// ==================== 辅助函数 ====================

func encodeBase64(data []byte) string {
	// 修复：简化实现，使用标准库
	return base64.StdEncoding.EncodeToString(data)
}

// 修复：添加本地实现，移除对 kafkaCommon.DecodeBase64 的依赖
func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// 修复：优化 splitLines 实现
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// 修复：优化 splitPipe 实现
func splitPipe(s string) []string {
	return strings.SplitN(s, "|", 3) // 只分割前 3 部分
}

func buildHeaders(m map[string]string) []kafka.Header {
	headers := make([]kafka.Header, 0, len(m))
	for k, v := range m {
		headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
	}
	return headers
}

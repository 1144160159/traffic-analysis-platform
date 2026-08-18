package dlq

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/lz4"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

type Config struct {
	Brokers      []string
	DLQTopic     string
	BatchSize    int
	MaxRetries   int
	RetryBackoff time.Duration

	FlowTopic    string
	SessionTopic string
	PcapTopic    string

	EnableFallback  bool
	FallbackDir     string
	MaxFallbackSize int64

	ReplayInterval       time.Duration
	ReplayBatchSize      int
	ReplaySuccessRateMin float64
	Security             kafkaCommon.SecurityConfig
}

func DefaultConfig(brokers []string) Config {
	return Config{
		Brokers:              brokers,
		DLQTopic:             config.TopicDLQ,
		FlowTopic:            config.TopicFlowEvents,
		SessionTopic:         config.TopicSessionEvents,
		PcapTopic:            config.TopicPcapIndex,
		BatchSize:            config.DefaultDLQBatchSize,
		MaxRetries:           config.DefaultKafkaMaxRetries,
		RetryBackoff:         config.DefaultDLQRetryBackoff,
		EnableFallback:       true,
		FallbackDir:          config.DefaultDLQFallbackDir,
		MaxFallbackSize:      config.MaxFallbackFileSize,
		ReplayInterval:       config.DefaultDLQReplayInterval,
		ReplayBatchSize:      config.DefaultDLQReplayBatchSize,
		ReplaySuccessRateMin: config.DefaultDLQReplaySuccessRate,
	}
}

type Producer struct {
	config Config
	logger *zap.Logger

	writer *kafka.Writer

	fallbackMu      sync.Mutex
	fallbackFile    *os.File
	fallbackSize    int64
	fallbackSeqNum  int64
	fallbackEnabled bool

	topicWriters map[string]*kafka.Writer
	writersMu    sync.RWMutex

	kafkaSuccessCount int64
	kafkaFailCount    int64
	fallbackCount     int64
	replayCount       int64

	closed    int32
	closeChan chan struct{}
	wg        sync.WaitGroup
}

func NewProducer(cfg Config, logger *zap.Logger) (*Producer, error) {

	if cfg.DLQTopic == "" {
		cfg.DLQTopic = config.TopicDLQ
	}
	if cfg.FlowTopic == "" {
		cfg.FlowTopic = config.TopicFlowEvents
	}
	if cfg.SessionTopic == "" {
		cfg.SessionTopic = config.TopicSessionEvents
	}
	if cfg.PcapTopic == "" {
		cfg.PcapTopic = config.TopicPcapIndex
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = config.DefaultDLQBatchSize
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = config.DefaultKafkaMaxRetries
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = config.DefaultDLQRetryBackoff
	}
	if cfg.ReplayInterval <= 0 {
		cfg.ReplayInterval = config.DefaultDLQReplayInterval
	}
	if cfg.ReplayBatchSize <= 0 {
		cfg.ReplayBatchSize = config.DefaultDLQReplayBatchSize
	}
	if cfg.ReplaySuccessRateMin == 0 {
		cfg.ReplaySuccessRateMin = config.DefaultDLQReplaySuccessRate
	}
	if cfg.MaxFallbackSize <= 0 {
		cfg.MaxFallbackSize = config.MaxFallbackFileSize
	}

	dialer, err := cfg.Security.Dialer("ingest-gateway-dlq")
	if err != nil {
		return nil, fmt.Errorf("invalid Kafka DLQ security configuration: %w", err)
	}

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:          cfg.Brokers,
		Topic:            cfg.DLQTopic,
		Dialer:           dialer,
		Balancer:         &kafka.LeastBytes{},
		BatchSize:        cfg.BatchSize,
		BatchTimeout:     config.KafkaBatchTimeout,
		CompressionCodec: lz4.NewCompressionCodec(),
		MaxAttempts:      cfg.MaxRetries,
		Async:            false,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logger.Error(fmt.Sprintf(msg, args...))
		}),
	})

	p := &Producer{
		config:          cfg,
		logger:          logger,
		writer:          writer,
		topicWriters:    make(map[string]*kafka.Writer),
		closeChan:       make(chan struct{}),
		fallbackEnabled: cfg.EnableFallback,
	}

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

	return p, nil
}

func (p *Producer) getTopicWriter(topic string) (*kafka.Writer, error) {
	p.writersMu.RLock()
	writer, exists := p.topicWriters[topic]
	p.writersMu.RUnlock()

	if exists {
		return writer, nil
	}

	p.writersMu.Lock()
	defer p.writersMu.Unlock()

	if writer, exists := p.topicWriters[topic]; exists {
		return writer, nil
	}

	dialer, err := p.config.Security.Dialer("ingest-gateway-dlq-replay")
	if err != nil {
		return nil, fmt.Errorf("invalid Kafka replay security configuration: %w", err)
	}

	writer = kafka.NewWriter(kafka.WriterConfig{
		Brokers:          p.config.Brokers,
		Balancer:         &kafka.Hash{},
		BatchSize:        config.DefaultKafkaBatchSize,
		BatchTimeout:     config.KafkaBatchTimeout,
		CompressionCodec: lz4.NewCompressionCodec(),
		MaxAttempts:      p.config.MaxRetries,
		Async:            false,
		Dialer:           dialer,
	})

	p.topicWriters[topic] = writer
	p.logger.Info("Created topic writer for replay", zap.String("topic", topic))

	return writer, nil
}

type DLQMessage struct {
	OriginalTopic string            `json:"original_topic"`
	EventType     string            `json:"event_type"`
	TenantID      string            `json:"tenant_id"`
	ProbeID       string            `json:"probe_id"`
	EventID       string            `json:"event_id"`
	FailedAt      time.Time         `json:"failed_at"`
	ErrorMessage  string            `json:"error_message"`
	RetryCount    int               `json:"retry_count"`
	Headers       map[string]string `json:"headers"`
	PayloadBase64 string            `json:"payload_base64"`
}

type eventMetadata interface {
	getOriginalTopic() string
	getEventType() string
	getTenantID() string
	getProbeID() string
	getEventID() string
	getHeaders() map[string]string
}

type flowEventMetadata struct {
	event *pb.FlowEvent
	topic string
}

func (m *flowEventMetadata) getOriginalTopic() string { return m.topic }
func (m *flowEventMetadata) getEventType() string     { return "flow" }
func (m *flowEventMetadata) getTenantID() string      { return m.event.Header.TenantId }
func (m *flowEventMetadata) getProbeID() string       { return m.event.Header.ProbeId }
func (m *flowEventMetadata) getEventID() string       { return m.event.Header.EventId }
func (m *flowEventMetadata) getHeaders() map[string]string {
	return map[string]string{
		"tenant_id":          m.event.Header.TenantId,
		"probe_id":           m.event.Header.ProbeId,
		"event_id":           m.event.Header.EventId,
		"run_id":             m.event.Header.RunId,
		"feature_set_id":     m.event.Header.FeatureSetId,
		"community_id":       m.event.CommunityId,
		"content_type":       config.ContentTypeProtobuf,
		"proto_message_type": config.ProtoMessageFlowEvent,
	}
}

type sessionEventMetadata struct {
	event *pb.SessionEvent
	topic string
}

func (m *sessionEventMetadata) getOriginalTopic() string { return m.topic }
func (m *sessionEventMetadata) getEventType() string     { return "session" }
func (m *sessionEventMetadata) getTenantID() string      { return m.event.Header.TenantId }
func (m *sessionEventMetadata) getProbeID() string       { return m.event.Header.ProbeId }
func (m *sessionEventMetadata) getEventID() string       { return m.event.Header.EventId }
func (m *sessionEventMetadata) getHeaders() map[string]string {
	return map[string]string{
		"tenant_id":          m.event.Header.TenantId,
		"probe_id":           m.event.Header.ProbeId,
		"event_id":           m.event.Header.EventId,
		"session_id":         m.event.SessionId,
		"community_id":       m.event.CommunityId,
		"content_type":       config.ContentTypeProtobuf,
		"proto_message_type": config.ProtoMessageSessionEvent,
	}
}

type pcapIndexMetadata struct {
	meta  *pb.PcapIndexMeta
	topic string
}

func (m *pcapIndexMetadata) getOriginalTopic() string { return m.topic }
func (m *pcapIndexMetadata) getEventType() string     { return "pcap" }
func (m *pcapIndexMetadata) getTenantID() string      { return m.meta.TenantId }
func (m *pcapIndexMetadata) getProbeID() string       { return m.meta.ProbeId }
func (m *pcapIndexMetadata) getEventID() string       { return fmt.Sprintf("pcap:%s", m.meta.FileKey) }
func (m *pcapIndexMetadata) getHeaders() map[string]string {
	return map[string]string{
		"tenant_id":          m.meta.TenantId,
		"probe_id":           m.meta.ProbeId,
		"file_key":           m.meta.FileKey,
		"content_type":       config.ContentTypeProtobuf,
		"proto_message_type": config.ProtoMessagePcapIndex,
	}
}

func (p *Producer) sendToDLQ(ctx context.Context, payload []byte, metadata eventMetadata, err error) error {

	dlqMsg := &DLQMessage{
		OriginalTopic: metadata.getOriginalTopic(),
		EventType:     metadata.getEventType(),
		TenantID:      metadata.getTenantID(),
		ProbeID:       metadata.getProbeID(),
		EventID:       metadata.getEventID(),
		FailedAt:      time.Now(),
		ErrorMessage:  err.Error(),
		RetryCount:    0,
		Headers:       metadata.getHeaders(),
		PayloadBase64: base64.StdEncoding.EncodeToString(payload),
	}

	msgData, jsonErr := json.Marshal(dlqMsg)
	if jsonErr != nil {
		p.logger.Error("Failed to marshal DLQ message",
			zap.String("event_id", metadata.getEventID()),
			zap.Error(jsonErr))
		return jsonErr
	}

	key := fmt.Sprintf("%s:%s", metadata.getTenantID(), metadata.getEventID())
	msg := kafka.Message{
		Key:   []byte(key),
		Value: msgData,
		Headers: []kafka.Header{
			{Key: "original_topic", Value: []byte(metadata.getOriginalTopic())},
			{Key: "event_type", Value: []byte(metadata.getEventType())},
			{Key: "tenant_id", Value: []byte(metadata.getTenantID())},
			{Key: "event_id", Value: []byte(metadata.getEventID())},
			{Key: "failed_at", Value: []byte(time.Now().Format(time.RFC3339))},
		},
	}

	writeErr := p.writeToKafka(ctx, []kafka.Message{msg})
	if writeErr != nil {

		if p.fallbackEnabled {
			return p.writeToFallback([]kafka.Message{msg})
		}
		return writeErr
	}

	atomic.AddInt64(&p.kafkaSuccessCount, 1)
	return nil
}

func (p *Producer) SendFlowEvents(ctx context.Context, events []*pb.FlowEvent, err error) error {
	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		if event == nil || event.Header == nil {
			continue
		}

		payload, marshalErr := proto.Marshal(event)
		if marshalErr != nil {
			p.logger.Error("Failed to marshal flow event for DLQ",
				zap.String("event_id", event.Header.EventId),
				zap.Error(marshalErr))
			continue
		}

		metadata := &flowEventMetadata{event: event, topic: p.config.FlowTopic}
		if sendErr := p.sendToDLQ(ctx, payload, metadata, err); sendErr != nil {
			p.logger.Error("Failed to send flow event to DLQ",
				zap.String("event_id", event.Header.EventId),
				zap.Error(sendErr))
		}
	}

	return nil
}

func (p *Producer) SendPcapIndex(ctx context.Context, meta *pb.PcapIndexMeta, err error) error {
	if meta == nil {
		return nil
	}

	payload, marshalErr := proto.Marshal(meta)
	if marshalErr != nil {
		p.logger.Error("Failed to marshal pcap index for DLQ",
			zap.String("file_key", meta.FileKey),
			zap.Error(marshalErr))
		return marshalErr
	}

	metadata := &pcapIndexMetadata{meta: meta, topic: p.config.PcapTopic}
	return p.sendToDLQ(ctx, payload, metadata, err)
}

func (p *Producer) SendSessionEvents(ctx context.Context, sessions []*pb.SessionEvent, err error) error {
	if len(sessions) == 0 {
		return nil
	}

	for _, session := range sessions {
		if session == nil || session.Header == nil {
			continue
		}

		payload, marshalErr := proto.Marshal(session)
		if marshalErr != nil {
			p.logger.Error("Failed to marshal session event for DLQ",
				zap.String("session_id", session.SessionId),
				zap.Error(marshalErr))
			continue
		}

		metadata := &sessionEventMetadata{event: session, topic: p.config.SessionTopic}
		if sendErr := p.sendToDLQ(ctx, payload, metadata, err); sendErr != nil {
			p.logger.Error("Failed to send session event to DLQ",
				zap.String("session_id", session.SessionId),
				zap.Error(sendErr))
		}
	}

	return nil
}

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

func (p *Producer) writeToFallback(messages []kafka.Message) error {
	p.fallbackMu.Lock()
	defer p.fallbackMu.Unlock()

	if p.fallbackFile == nil || p.fallbackSize >= p.config.MaxFallbackSize {
		if err := p.rotateFallbackFile(); err != nil {
			return fmt.Errorf("failed to rotate fallback file: %w", err)
		}
	}

	for _, msg := range messages {
		line := fmt.Sprintf("%s|%s|%s\n", p.config.DLQTopic, string(msg.Key), string(msg.Value))
		n, err := p.fallbackFile.WriteString(line)
		if err != nil {
			return fmt.Errorf("failed to write to fallback file: %w", err)
		}
		p.fallbackSize += int64(n)
	}

	if err := p.fallbackFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync fallback file: %w", err)
	}

	atomic.AddInt64(&p.fallbackCount, int64(len(messages)))

	p.logger.Info("Messages written to DLQ fallback file",
		zap.Int("count", len(messages)),
		zap.Int64("file_size", p.fallbackSize))

	return nil
}

func (p *Producer) rotateFallbackFile() error {

	if p.fallbackFile != nil {
		p.fallbackFile.Close()
	}

	seqNum := atomic.AddInt64(&p.fallbackSeqNum, 1)
	filename := fmt.Sprintf(config.DLQFallbackFileFormat, time.Now().Unix(), seqNum)
	filePath := filepath.Join(p.config.FallbackDir, filename)

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	p.fallbackFile = file
	p.fallbackSize = 0

	p.logger.Info("Rotated DLQ fallback file", zap.String("file", filePath))
	return nil
}

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

func (p *Producer) replayFallbackFiles(ctx context.Context) {
	report := p.ReplayFallbackFiles(ctx)
	if report.ReplayedFiles > 0 || report.FailedFiles > 0 {
		p.logger.Info("DLQ fallback replay completed",
			zap.Int("success", report.ReplayedFiles),
			zap.Int("failed", report.FailedFiles),
			zap.Int("remaining_files", report.RemainingFallbackFiles))
	}
}

func (p *Producer) ReplayFallbackFiles(ctx context.Context) (report FallbackReplayReport) {
	report.StartedAt = time.Now()
	defer func() {
		report.FinishedAt = time.Now()
		report.RemainingFallbackFiles, report.RemainingFallbackBytes, _ = p.GetFallbackStats()
	}()

	if !p.fallbackEnabled || p.config.FallbackDir == "" {
		return report
	}
	report.FallbackReplayAvailable = true

	entries, err := os.ReadDir(p.config.FallbackDir)
	if err != nil {
		p.logger.Error("Failed to read fallback directory", zap.Error(err))
		report.Errors = append(report.Errors, err.Error())
		return report
	}

	if len(entries) == 0 {
		return report
	}
	report.FileCount = len(entries)

	p.logger.Info("Starting DLQ fallback replay", zap.Int("file_count", len(entries)))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		p.fallbackMu.Lock()
		isCurrent := p.fallbackFile != nil && entry.Name() == filepath.Base(p.fallbackFile.Name())
		p.fallbackMu.Unlock()

		if isCurrent {
			report.SkippedCurrentFiles++
			continue
		}

		filePath := filepath.Join(p.config.FallbackDir, entry.Name())
		if err := p.replayFile(ctx, filePath); err != nil {
			p.logger.Error("Failed to replay fallback file",
				zap.String("file", filePath),
				zap.Error(err))
			report.FailedFiles++
			report.Errors = append(report.Errors, err.Error())
		} else {
			report.ReplayedFiles++
		}
	}

	return report
}

// ReplayFallbackFilesForTenant 仅回放指定租户的 fallback 行,并保留其他租户行。
// 与全量回放不同:租户级回放不删除文件,只把"其他租户行 + 无效行 + 重放失败行"
// 原子重写回文件,避免误删其他租户的待重放消息。
func (p *Producer) ReplayFallbackFilesForTenant(ctx context.Context, tenantID string) (report FallbackReplayReport) {
	report.StartedAt = time.Now()
	defer func() {
		report.FinishedAt = time.Now()
		report.RemainingFallbackFiles, report.RemainingFallbackBytes, _ = p.GetFallbackStats()
	}()

	if !p.fallbackEnabled || p.config.FallbackDir == "" || strings.TrimSpace(tenantID) == "" {
		return report
	}
	report.FallbackReplayAvailable = true

	entries, err := os.ReadDir(p.config.FallbackDir)
	if err != nil {
		p.logger.Error("Failed to read fallback directory", zap.Error(err))
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	if len(entries) == 0 {
		return report
	}
	report.FileCount = len(entries)

	p.logger.Info("Starting tenant-scoped DLQ fallback replay",
		zap.Int("file_count", len(entries)),
		zap.String("tenant_id", tenantID))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		p.fallbackMu.Lock()
		isCurrent := p.fallbackFile != nil && entry.Name() == filepath.Base(p.fallbackFile.Name())
		p.fallbackMu.Unlock()

		if isCurrent {
			report.SkippedCurrentFiles++
			continue
		}

		filePath := filepath.Join(p.config.FallbackDir, entry.Name())
		if err := p.replayFileFiltered(ctx, filePath, tenantID); err != nil {
			p.logger.Error("Failed to replay fallback file for tenant",
				zap.String("file", filePath),
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			report.FailedFiles++
			report.Errors = append(report.Errors, err.Error())
		} else {
			report.ReplayedFiles++
		}
	}

	return report
}

// replayFileFiltered 租户过滤回放单个 fallback 文件:
//   - 解析失败/其他租户/解码失败的行原样保留;
//   - 本租户行进入重放,重放失败的行一并保留;
//   - 无本租户行时文件原样保留(不重写不删除);
//   - 有本租户行时,剩余行原子重写回原文件;剩余为空则删除文件。
func (p *Producer) replayFileFiltered(ctx context.Context, filePath, tenantID string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return nil
	}

	var keepLines []string
	var failedLines []string
	matched := 0

	batch := make([]replayItem, 0, p.config.ReplayBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		success, failed := p.replayBatch(ctx, batch)
		matched += success
		failedLines = append(failedLines, failed...)
		batch = batch[:0]
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			keepLines = append(keepLines, line)
			continue
		}
		var dlqMsg DLQMessage
		if err := json.Unmarshal([]byte(parts[2]), &dlqMsg); err != nil {
			keepLines = append(keepLines, line)
			continue
		}
		if strings.TrimSpace(dlqMsg.TenantID) != tenantID {
			keepLines = append(keepLines, line)
			continue
		}
		payload, err := base64.StdEncoding.DecodeString(dlqMsg.PayloadBase64)
		if err != nil {
			keepLines = append(keepLines, line)
			continue
		}
		originalMsg := kafka.Message{
			Topic:   dlqMsg.OriginalTopic,
			Key:     []byte(parts[1]),
			Value:   payload,
			Headers: buildHeaders(dlqMsg.Headers),
		}
		batch = append(batch, replayItem{line: line, msg: originalMsg})
		if len(batch) >= p.config.ReplayBatchSize {
			flush()
		}
	}
	flush()

	if matched == 0 && len(failedLines) == 0 {
		// 本文件没有该租户的消息:原样保留。
		return nil
	}

	remaining := append(keepLines, failedLines...)
	if len(remaining) == 0 {
		if err := os.Remove(filePath); err != nil {
			p.logger.Warn("Failed to delete replayed file", zap.Error(err))
		}
		return nil
	}
	return p.persistFailedLines(filePath, remaining)
}

// replayItem 关联 fallback 原始行与其对应的待重放 Kafka 消息,失败时可精确保留原始行。
type replayItem struct {
	line string
	msg  kafka.Message
}

func (p *Producer) replayFile(ctx context.Context, filePath string) error {
	totalSuccess := 0
	totalFailed := 0
	var failedLines []string

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {

		return os.Remove(filePath)
	}

	batch := make([]replayItem, 0, p.config.ReplayBatchSize)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			p.logger.Warn("Invalid fallback line format", zap.String("line", line))
			totalFailed++
			failedLines = append(failedLines, line)
			continue
		}

		key := parts[1]
		value := parts[2]

		var dlqMsg DLQMessage
		if err := json.Unmarshal([]byte(value), &dlqMsg); err != nil {
			p.logger.Warn("Failed to unmarshal DLQ message", zap.Error(err))
			totalFailed++
			failedLines = append(failedLines, line)
			continue
		}

		payload, err := base64.StdEncoding.DecodeString(dlqMsg.PayloadBase64)
		if err != nil {
			p.logger.Warn("Failed to decode payload", zap.Error(err))
			totalFailed++
			failedLines = append(failedLines, line)
			continue
		}

		originalMsg := kafka.Message{
			Topic:   dlqMsg.OriginalTopic,
			Key:     []byte(key),
			Value:   payload,
			Headers: buildHeaders(dlqMsg.Headers),
		}

		batch = append(batch, replayItem{line: line, msg: originalMsg})

		if len(batch) >= p.config.ReplayBatchSize {
			success, failedMsgs := p.replayBatch(ctx, batch)
			totalSuccess += success
			totalFailed += len(failedMsgs)
			failedLines = append(failedLines, failedMsgs...)
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		success, failedMsgs := p.replayBatch(ctx, batch)
		totalSuccess += success
		totalFailed += len(failedMsgs)
		failedLines = append(failedLines, failedMsgs...)
	}

	totalProcessed := totalSuccess + totalFailed
	if totalProcessed == 0 {

		p.logger.Info("No valid messages in file, removing",
			zap.String("file", filePath))
		return os.Remove(filePath)
	}

	successRate := float64(totalSuccess) / float64(totalProcessed)

	if successRate >= p.config.ReplaySuccessRateMin {
		if len(failedLines) > 0 {
			// 失败消息必须保留:将原文件原子替换为仅含失败行的新文件,等待下一轮重放。
			if err := p.persistFailedLines(filePath, failedLines); err != nil {
				return fmt.Errorf("replay finished with %d failed messages but failed to persist them: %w",
					len(failedLines), err)
			}
			p.logger.Warn("Replay completed with failures; failed messages preserved for retry",
				zap.String("file", filePath),
				zap.Int("success", totalSuccess),
				zap.Int("failed", len(failedLines)),
				zap.Float64("success_rate", successRate))
			return nil
		}

		if err := os.Remove(filePath); err != nil {
			p.logger.Warn("Failed to delete replayed file", zap.Error(err))
		} else {
			p.logger.Info("Replayed file deleted",
				zap.String("file", filePath),
				zap.Int("success", totalSuccess),
				zap.Int("failed", totalFailed),
				zap.Float64("success_rate", successRate))
		}
		return nil
	}

	p.logger.Warn("Replay success rate too low, keeping file",
		zap.String("file", filePath),
		zap.Float64("success_rate", successRate),
		zap.Float64("min_required", p.config.ReplaySuccessRateMin))

	return fmt.Errorf("replay success rate %.2f%% < %.2f%%",
		successRate*100, p.config.ReplaySuccessRateMin*100)
}

// persistFailedLines 将失败行原子写入目标 fallback 文件(先写临时文件再 rename),
// 确保删除/替换原文件前失败消息已有落盘副本。
func (p *Producer) persistFailedLines(filePath string, failedLines []string) error {
	if len(failedLines) == 0 {
		return nil
	}

	tmp, err := os.CreateTemp(p.config.FallbackDir, ".replay-failed-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file for failed replay lines: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	writer := bufio.NewWriter(tmp)
	for _, line := range failedLines {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("failed to write failed replay line: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to flush failed replay lines: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to sync failed replay lines: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close failed replay lines file: %w", err)
	}
	if err := os.Rename(tmpName, filePath); err != nil {
		return fmt.Errorf("failed to atomically replace fallback file with failed lines: %w", err)
	}
	tmpName = ""
	return nil
}

func (p *Producer) replayBatch(ctx context.Context, items []replayItem) (success int, failed []string) {
	if len(items) == 0 {
		return 0, nil
	}

	topicGroups := make(map[string][]replayItem)
	for _, item := range items {
		topicGroups[item.msg.Topic] = append(topicGroups[item.msg.Topic], item)
	}

	for topic, group := range topicGroups {
		writer, err := p.getTopicWriter(topic)
		if err != nil {
			p.logger.Error("Failed to create replay writer",
				zap.String("topic", topic),
				zap.Int("count", len(group)),
				zap.Error(err))
			for _, item := range group {
				failed = append(failed, item.line)
			}
			continue
		}

		msgs := make([]kafka.Message, 0, len(group))
		for _, item := range group {
			msgs = append(msgs, item.msg)
		}
		if err := writer.WriteMessages(ctx, msgs...); err != nil {
			p.logger.Error("Failed to replay messages",
				zap.String("topic", topic),
				zap.Int("count", len(msgs)),
				zap.Error(err))
			for _, item := range group {
				failed = append(failed, item.line)
			}
		} else {
			success += len(group)
			atomic.AddInt64(&p.replayCount, int64(len(group)))
		}
	}

	return success, failed
}

func buildHeaders(m map[string]string) []kafka.Header {
	headers := make([]kafka.Header, 0, len(m))
	for k, v := range m {
		headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
	}
	return headers
}

func (p *Producer) GetFallbackStats() (fileCount int, totalSize int64, err error) {
	if !p.fallbackEnabled || p.config.FallbackDir == "" {
		return 0, 0, nil
	}

	entries, err := os.ReadDir(p.config.FallbackDir)
	if err != nil {
		return 0, 0, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			fileCount++
			info, err := entry.Info()
			if err == nil {
				totalSize += info.Size()
			}
		}
	}

	return fileCount, totalSize, nil
}

func (p *Producer) GetStats() DLQStats {
	return DLQStats{
		KafkaSuccessCount: atomic.LoadInt64(&p.kafkaSuccessCount),
		KafkaFailCount:    atomic.LoadInt64(&p.kafkaFailCount),
		FallbackCount:     atomic.LoadInt64(&p.fallbackCount),
		ReplayCount:       atomic.LoadInt64(&p.replayCount),
	}
}

type DLQStats struct {
	KafkaSuccessCount int64
	KafkaFailCount    int64
	FallbackCount     int64
	ReplayCount       int64
}

type FallbackReplayReport struct {
	StartedAt               time.Time `json:"started_at"`
	FinishedAt              time.Time `json:"finished_at"`
	FileCount               int       `json:"file_count"`
	ReplayedFiles           int       `json:"replayed_files"`
	FailedFiles             int       `json:"failed_files"`
	SkippedCurrentFiles     int       `json:"skipped_current_files"`
	RemainingFallbackFiles  int       `json:"remaining_fallback_files"`
	RemainingFallbackBytes  int64     `json:"remaining_fallback_bytes"`
	Errors                  []string  `json:"errors,omitempty"`
	FallbackReplayAvailable bool      `json:"fallback_replay_available"`
}

func (p *Producer) Close() error {
	if !atomic.CompareAndSwapInt32(&p.closed, 0, 1) {
		return nil
	}

	p.logger.Info("Closing DLQ producer")

	close(p.closeChan)

	p.wg.Wait()

	p.fallbackMu.Lock()
	if p.fallbackFile != nil {
		p.fallbackFile.Close()
	}
	p.fallbackMu.Unlock()

	var errs []error
	if err := p.writer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close dlq writer: %w", err))
	}

	p.writersMu.Lock()
	for topic, writer := range p.topicWriters {
		if err := writer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s writer: %w", topic, err))
		}
	}
	p.writersMu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("errors closing DLQ producer: %v", errs)
	}

	p.logger.Info("DLQ producer closed")
	return nil
}

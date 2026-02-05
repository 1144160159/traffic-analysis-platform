////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/queue/producer.go
// 优化版 v3：
// 1. 移除所有硬编码（Topic、Header、Protobuf类型）
// 2. 使用 config 常量
// 3. 统一错误处理和日志
////////////////////////////////////////////////////////////////////////////////

package queue

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// ProducerConfig 生产者配置
type ProducerConfig struct {
	Brokers           []string      `env:"KAFKA_BROKERS" envSeparator:","`
	FlowTopic         string        `env:"KAFKA_FLOW_TOPIC"`
	PcapIndexTopic    string        `env:"KAFKA_PCAP_INDEX_TOPIC"`
	SessionTopic      string        `env:"KAFKA_SESSION_TOPIC"`
	BatchSize         int           `env:"KAFKA_BATCH_SIZE"`
	BatchTimeout      time.Duration `env:"KAFKA_BATCH_TIMEOUT"`
	Compression       string        `env:"KAFKA_COMPRESSION"`
	RequiredAcks      string        `env:"KAFKA_REQUIRED_ACKS"`
	MaxRetries        int           `env:"KAFKA_MAX_RETRIES"`
	EnableIdempotence bool          `env:"KAFKA_ENABLE_IDEMPOTENCE"`
	EnableValidation  bool          `env:"KAFKA_ENABLE_VALIDATION"`
}

// Producer Kafka 生产者
type Producer struct {
	multiProducer *kafkaCommon.MultiTopicProducer
	partitioner   *TenantCommunityPartitioner
	logger        *zap.Logger
	config        ProducerConfig
}

// NewProducer 创建生产者
func NewProducer(cfg ProducerConfig, logger *zap.Logger) (*Producer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers not configured")
	}

	// 应用默认值（使用 config 常量）
	if cfg.FlowTopic == "" {
		cfg.FlowTopic = config.TopicFlowEvents
	}
	if cfg.SessionTopic == "" {
		cfg.SessionTopic = config.TopicSessionEvents
	}
	if cfg.PcapIndexTopic == "" {
		cfg.PcapIndexTopic = config.TopicPcapIndex
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = config.DefaultKafkaBatchSize
	}
	if cfg.Compression == "" {
		cfg.Compression = config.DefaultKafkaCompression
	}

	// 创建 MultiTopicProducer
	multiProducer := kafkaCommon.NewMultiTopicProducer(logger)

	// 构建通用配置
	baseConfig := kafkaCommon.ProducerConfig{
		Brokers:      cfg.Brokers,
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
		Compression:  cfg.Compression,
		RequiredAcks: cfg.RequiredAcks,
		MaxAttempts:  cfg.MaxRetries,
		Async:        false,
	}

	// 添加 Flow Topic
	flowConfig := baseConfig
	flowConfig.Topic = cfg.FlowTopic
	if err := multiProducer.AddTopic(cfg.FlowTopic, flowConfig); err != nil {
		return nil, fmt.Errorf("failed to add flow topic: %w", err)
	}

	// 添加 PCAP Topic
	pcapConfig := baseConfig
	pcapConfig.Topic = cfg.PcapIndexTopic
	if err := multiProducer.AddTopic(cfg.PcapIndexTopic, pcapConfig); err != nil {
		multiProducer.Close()
		return nil, fmt.Errorf("failed to add pcap topic: %w", err)
	}

	// 添加 Session Topic
	sessionConfig := baseConfig
	sessionConfig.Topic = cfg.SessionTopic
	if err := multiProducer.AddTopic(cfg.SessionTopic, sessionConfig); err != nil {
		multiProducer.Close()
		return nil, fmt.Errorf("failed to add session topic: %w", err)
	}

	logger.Info("Kafka producer initialized",
		zap.Strings("brokers", cfg.Brokers),
		zap.String("flow_topic", cfg.FlowTopic),
		zap.String("pcap_topic", cfg.PcapIndexTopic),
		zap.String("session_topic", cfg.SessionTopic),
		zap.Bool("idempotence", cfg.EnableIdempotence),
		zap.String("acks", cfg.RequiredAcks))

	return &Producer{
		multiProducer: multiProducer,
		partitioner:   NewTenantCommunityPartitioner(12),
		logger:        logger,
		config:        cfg,
	}, nil
}

// WriteFlowEvents 批量写入 Flow 事件
func (p *Producer) WriteFlowEvents(ctx context.Context, events []*pb.FlowEvent) error {
	ctx, span := otel.StartSpan(ctx, "producer.write_flow_events")
	defer span.End()

	if len(events) == 0 {
		return nil
	}

	logger := logging.L(ctx)

	// 转换为 common/kafka.Message
	messages := make([]kafkaCommon.Message, 0, len(events))

	for _, event := range events {
		if event == nil || event.Header == nil {
			continue
		}

		// 数据验证
		if p.config.EnableValidation {
			p.validateFlowEvent(event, logger)
		}

		// 序列化
		value, err := proto.Marshal(event)
		if err != nil {
			logger.Error("Failed to marshal flow event",
				zap.String("event_id", event.Header.EventId),
				zap.Error(err))
			continue
		}

		// 构建消息 Key
		key := fmt.Sprintf("%s:%s", event.Header.TenantId, event.CommunityId)

		// 构建 Headers（使用 config 常量）
		headers := []kafkaCommon.MessageHeader{
			{Key: "tenant_id", Value: event.Header.TenantId},
			{Key: "probe_id", Value: event.Header.ProbeId},
			{Key: "event_id", Value: event.Header.EventId},
			{Key: "run_id", Value: event.Header.RunId},
			{Key: "feature_set_id", Value: event.Header.FeatureSetId},
			{Key: "community_id", Value: event.CommunityId},
			{Key: "content_type", Value: config.ContentTypeProtobuf},
			{Key: "proto_message_type", Value: config.ProtoMessageFlowEvent},
			{Key: "proto_schema_version", Value: config.ProtoSchemaVersion},
			{Key: "proto_package", Value: config.ProtoPackage},
			{Key: "event_ts", Value: fmt.Sprintf("%d", event.Header.EventTs)},
			{Key: "ingest_ts", Value: fmt.Sprintf("%d", event.Header.IngestTs)},
		}

		messages = append(messages, kafkaCommon.Message{
			Key:     key,
			Value:   value,
			Headers: headers,
			Time:    time.UnixMilli(event.Header.EventTs),
		})
	}

	if len(messages) == 0 {
		return nil
	}

	// 批量发送
	start := time.Now()
	if err := p.multiProducer.SendBatch(ctx, p.config.FlowTopic, messages); err != nil {
		logger.Error("Failed to write flow events",
			zap.Int("count", len(messages)),
			zap.Error(err))
		return fmt.Errorf("failed to write flow events: %w", err)
	}

	logger.Debug("Flow events written",
		zap.Int("count", len(messages)),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// WritePcapIndex 写入 PCAP 索引
func (p *Producer) WritePcapIndex(ctx context.Context, meta *pb.PcapIndexMeta) error {
	ctx, span := otel.StartSpan(ctx, "producer.write_pcap_index")
	defer span.End()

	if meta == nil {
		return fmt.Errorf("invalid pcap index meta: nil")
	}

	logger := logging.L(ctx)

	// 数据验证
	if p.config.EnableValidation {
		p.validatePcapIndex(meta, logger)
	}

	// 序列化
	value, err := proto.Marshal(meta)
	if err != nil {
		logger.Error("Failed to marshal pcap index meta",
			zap.String("file_key", meta.FileKey),
			zap.Error(err))
		return fmt.Errorf("failed to marshal pcap index: %w", err)
	}

	// 构建消息 Key
	key := fmt.Sprintf("%s:%s", meta.TenantId, meta.ProbeId)

	// 构建 Headers（使用 config 常量）
	headers := []kafkaCommon.MessageHeader{
		{Key: "tenant_id", Value: meta.TenantId},
		{Key: "probe_id", Value: meta.ProbeId},
		{Key: "file_key", Value: meta.FileKey},
		{Key: "community_id", Value: meta.CommunityId},
		{Key: "sha256", Value: meta.Sha256},
		{Key: "content_type", Value: config.ContentTypeProtobuf},
		{Key: "proto_message_type", Value: config.ProtoMessagePcapIndex},
		{Key: "proto_schema_version", Value: config.ProtoSchemaVersion},
		{Key: "ts_start", Value: fmt.Sprintf("%d", meta.TsStart)},
		{Key: "ts_end", Value: fmt.Sprintf("%d", meta.TsEnd)},
	}

	start := time.Now()
	err = p.multiProducer.Send(ctx, p.config.PcapIndexTopic, key, value, headers...)
	duration := time.Since(start)

	if err != nil {
		logger.Error("Failed to write pcap index",
			zap.String("file_key", meta.FileKey),
			zap.Duration("duration", duration),
			zap.Error(err))
		return fmt.Errorf("failed to write pcap index: %w", err)
	}

	logger.Debug("PCAP index written",
		zap.String("file_key", meta.FileKey),
		zap.String("tenant_id", meta.TenantId),
		zap.Duration("duration", duration))

	return nil
}

// WriteSessionEvents 批量写入 Session 事件
func (p *Producer) WriteSessionEvents(ctx context.Context, sessions []*pb.SessionEvent) error {
	ctx, span := otel.StartSpan(ctx, "producer.write_session_events")
	defer span.End()

	if len(sessions) == 0 {
		return nil
	}

	logger := logging.L(ctx)

	// 转换为 common/kafka.Message
	messages := make([]kafkaCommon.Message, 0, len(sessions))

	for _, session := range sessions {
		if session == nil || session.Header == nil {
			continue
		}

		// 数据验证
		if p.config.EnableValidation {
			p.validateSessionEvent(session, logger)
		}

		// 序列化
		value, err := proto.Marshal(session)
		if err != nil {
			logger.Error("Failed to marshal session event",
				zap.String("session_id", session.SessionId),
				zap.Error(err))
			continue
		}

		// 构建消息 Key
		key := fmt.Sprintf("%s:%s", session.Header.TenantId, session.CommunityId)

		// 构建 Headers（使用 config 常量）
		headers := []kafkaCommon.MessageHeader{
			{Key: "tenant_id", Value: session.Header.TenantId},
			{Key: "probe_id", Value: session.Header.ProbeId},
			{Key: "event_id", Value: session.Header.EventId},
			{Key: "session_id", Value: session.SessionId},
			{Key: "community_id", Value: session.CommunityId},
			{Key: "content_type", Value: config.ContentTypeProtobuf},
			{Key: "proto_message_type", Value: config.ProtoMessageSessionEvent},
			{Key: "proto_schema_version", Value: config.ProtoSchemaVersion},
			{Key: "proto_package", Value: config.ProtoPackage},
			{Key: "event_ts", Value: fmt.Sprintf("%d", session.Header.EventTs)},
		}

		messages = append(messages, kafkaCommon.Message{
			Key:     key,
			Value:   value,
			Headers: headers,
			Time:    time.UnixMilli(session.Header.EventTs),
		})
	}

	if len(messages) == 0 {
		return nil
	}

	// 批量发送
	start := time.Now()
	if err := p.multiProducer.SendBatch(ctx, p.config.SessionTopic, messages); err != nil {
		logger.Error("Failed to write session events",
			zap.Int("count", len(messages)),
			zap.Error(err))
		return fmt.Errorf("failed to write session events: %w", err)
	}

	logger.Debug("Session events written",
		zap.Int("count", len(messages)),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// validateFlowEvent 验证 Flow 事件
func (p *Producer) validateFlowEvent(event *pb.FlowEvent, logger *zap.Logger) {
	if event == nil || event.Tuple == nil {
		return
	}

	if event.Tuple.SrcPort > config.MaxUInt16 {
		logger.Warn("Source port exceeds UInt16 range",
			zap.String("event_id", event.Header.EventId),
			zap.Uint32("src_port", event.Tuple.SrcPort))
	}
	if event.Tuple.DstPort > config.MaxUInt16 {
		logger.Warn("Destination port exceeds UInt16 range",
			zap.String("event_id", event.Header.EventId),
			zap.Uint32("dst_port", event.Tuple.DstPort))
	}
	if event.Tuple.Protocol > config.MaxUInt8 {
		logger.Warn("Protocol number exceeds UInt8 range",
			zap.String("event_id", event.Header.EventId),
			zap.Uint32("protocol", event.Tuple.Protocol))
	}
}

// validatePcapIndex 验证 PCAP 索引
func (p *Producer) validatePcapIndex(meta *pb.PcapIndexMeta, logger *zap.Logger) {
	if meta == nil {
		return
	}
	if meta.ZstdLevel > config.MaxUInt8 {
		logger.Warn("ZSTD level exceeds UInt8 range",
			zap.String("file_key", meta.FileKey),
			zap.Uint32("zstd_level", meta.ZstdLevel))
	}
}

// validateSessionEvent 验证 Session 事件
func (p *Producer) validateSessionEvent(session *pb.SessionEvent, logger *zap.Logger) {
	if session == nil {
		return
	}
	if session.ClientPort > config.MaxUInt16 {
		logger.Warn("Client port exceeds UInt16 range",
			zap.String("session_id", session.SessionId),
			zap.Uint32("client_port", session.ClientPort))
	}
	if session.ServerPort > config.MaxUInt16 {
		logger.Warn("Server port exceeds UInt16 range",
			zap.String("session_id", session.SessionId),
			zap.Uint32("server_port", session.ServerPort))
	}
	if session.Protocol > config.MaxUInt8 {
		logger.Warn("Protocol exceeds UInt8 range",
			zap.String("session_id", session.SessionId),
			zap.Uint32("protocol", session.Protocol))
	}
}

// GetMetrics 获取生产者指标
func (p *Producer) GetMetrics() ProducerMetrics {
	flowMetrics, _ := p.multiProducer.GetTopicMetrics(p.config.FlowTopic)
	pcapMetrics, _ := p.multiProducer.GetTopicMetrics(p.config.PcapIndexTopic)
	sessionMetrics, _ := p.multiProducer.GetTopicMetrics(p.config.SessionTopic)

	return ProducerMetrics{
		FlowMessagesSent:       flowMetrics.MessagesSent,
		FlowMessagesError:      flowMetrics.MessagesError,
		PcapIndexMessagesSent:  pcapMetrics.MessagesSent,
		PcapIndexMessagesError: pcapMetrics.MessagesError,
		SessionMessagesSent:    sessionMetrics.MessagesSent,
		SessionMessagesError:   sessionMetrics.MessagesError,
		LastSendTime:           flowMetrics.LastSendTime,
	}
}

// ProducerMetrics 生产者指标
type ProducerMetrics struct {
	FlowMessagesSent       int64
	FlowMessagesError      int64
	PcapIndexMessagesSent  int64
	PcapIndexMessagesError int64
	SessionMessagesSent    int64
	SessionMessagesError   int64
	LastSendTime           time.Time
}

// Close 关闭生产者
func (p *Producer) Close() error {
	return p.multiProducer.Close()
}

// Healthy 检查生产者健康状态
func (p *Producer) Healthy() bool {
	return p.multiProducer != nil
}

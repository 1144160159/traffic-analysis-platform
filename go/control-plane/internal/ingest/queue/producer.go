package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

type ProducerConfig struct {
	Brokers           []string      `env:"KAFKA_BROKERS" envSeparator:","`
	FlowTopic         string        `env:"KAFKA_FLOW_TOPIC"`
	PcapIndexTopic    string        `env:"KAFKA_PCAP_INDEX_TOPIC"`
	SessionTopic      string        `env:"KAFKA_SESSION_TOPIC"`
	BindingTopic      string        `env:"KAFKA_ASSET_BINDING_TOPIC"`
	BatchSize         int           `env:"KAFKA_BATCH_SIZE"`
	BatchTimeout      time.Duration `env:"KAFKA_BATCH_TIMEOUT"`
	Compression       string        `env:"KAFKA_COMPRESSION"`
	RequiredAcks      string        `env:"KAFKA_REQUIRED_ACKS"`
	MaxRetries        int           `env:"KAFKA_MAX_RETRIES"`
	EnableIdempotence bool          `env:"KAFKA_ENABLE_IDEMPOTENCE"`
	EnableValidation  bool          `env:"KAFKA_ENABLE_VALIDATION"`
	Security          kafkaCommon.SecurityConfig
}

type Producer struct {
	multiProducer     *kafkaCommon.MultiTopicProducer
	writeFlowBatch    func(context.Context, string, []kafkaCommon.Message) error
	writeBindingBatch func(context.Context, string, []kafkaCommon.Message) error
	partitioner       *TenantCommunityPartitioner
	logger            *zap.Logger
	config            ProducerConfig
}

func NewProducer(cfg ProducerConfig, logger *zap.Logger) (*Producer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers not configured")
	}

	if cfg.FlowTopic == "" {
		cfg.FlowTopic = config.TopicFlowEvents
	}
	if cfg.SessionTopic == "" {
		cfg.SessionTopic = config.TopicSessionEvents
	}
	if cfg.PcapIndexTopic == "" {
		cfg.PcapIndexTopic = config.TopicPcapIndex
	}
	if cfg.BindingTopic == "" {
		cfg.BindingTopic = config.TopicAssetBindings
	}
	if cfg.BindingTopic != config.TopicAssetBindings {
		return nil, fmt.Errorf("asset binding topic must be %q, got %q", config.TopicAssetBindings, cfg.BindingTopic)
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = config.DefaultKafkaBatchSize
	}
	if cfg.Compression == "" {
		cfg.Compression = config.DefaultKafkaCompression
	}
	if cfg.RequiredAcks == "" {
		cfg.RequiredAcks = config.KafkaRequiredAcksAll
	}
	if cfg.RequiredAcks != config.KafkaRequiredAcksAll {
		return nil, fmt.Errorf(
			"ingest durability barrier requires KAFKA_REQUIRED_ACKS=all, got %q",
			cfg.RequiredAcks,
		)
	}

	multiProducer := kafkaCommon.NewMultiTopicProducer(logger)

	baseConfig := kafkaCommon.ProducerConfig{
		Brokers:      cfg.Brokers,
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
		Compression:  cfg.Compression,
		RequiredAcks: cfg.RequiredAcks,
		MaxAttempts:  cfg.MaxRetries,
		Async:        false,
		Security:     cfg.Security,
	}
	// 幂等语义落地:启用 KAFKA_ENABLE_IDEMPOTENCE 后按消息 key(tenant:community)
	// 使用稳定 Hash 分区,保证同一 key 的消息进入同一分区、顺序有序,与下游
	// 按 key 消费的去重/排序契约一致;关闭时保持原有 RoundRobin 行为。
	if cfg.EnableIdempotence {
		baseConfig.IdempotentKey = "tenant:community"
	}

	flowConfig := baseConfig
	flowConfig.Topic = cfg.FlowTopic
	if err := multiProducer.AddTopic(cfg.FlowTopic, flowConfig); err != nil {
		return nil, fmt.Errorf("failed to add flow topic: %w", err)
	}

	pcapConfig := baseConfig
	pcapConfig.Topic = cfg.PcapIndexTopic
	if err := multiProducer.AddTopic(cfg.PcapIndexTopic, pcapConfig); err != nil {
		multiProducer.Close()
		return nil, fmt.Errorf("failed to add pcap topic: %w", err)
	}

	sessionConfig := baseConfig
	sessionConfig.Topic = cfg.SessionTopic
	if err := multiProducer.AddTopic(cfg.SessionTopic, sessionConfig); err != nil {
		multiProducer.Close()
		return nil, fmt.Errorf("failed to add session topic: %w", err)
	}

	bindingConfig := baseConfig
	bindingConfig.Topic = cfg.BindingTopic
	if err := multiProducer.AddTopic(cfg.BindingTopic, bindingConfig); err != nil {
		multiProducer.Close()
		return nil, fmt.Errorf("failed to add asset binding topic: %w", err)
	}

	logger.Info("Kafka producer initialized",
		zap.Strings("brokers", cfg.Brokers),
		zap.String("flow_topic", cfg.FlowTopic),
		zap.String("pcap_topic", cfg.PcapIndexTopic),
		zap.String("session_topic", cfg.SessionTopic),
		zap.String("binding_topic", cfg.BindingTopic),
		zap.Bool("idempotence", cfg.EnableIdempotence),
		zap.String("acks", cfg.RequiredAcks))

	return &Producer{
		multiProducer:     multiProducer,
		writeFlowBatch:    multiProducer.SendBatch,
		writeBindingBatch: multiProducer.SendBatch,
		partitioner:       NewTenantCommunityPartitioner(12),
		logger:            logger,
		config:            cfg,
	}, nil
}

type AssetBindingWriteItemResult struct {
	InputIndex    int
	ObservationID string
	Disposition   pb.AssetBindingItemDisposition
	ReasonCode    string
	AckScope      string
}

type AssetBindingWriteResult struct {
	Items []AssetBindingWriteItemResult
}

func (r AssetBindingWriteResult) ValidateExactSet(inputCount int) error {
	if len(r.Items) != inputCount {
		return fmt.Errorf("asset binding write result cardinality mismatch: got %d want %d", len(r.Items), inputCount)
	}
	seen := make([]bool, inputCount)
	for _, item := range r.Items {
		if item.InputIndex < 0 || item.InputIndex >= inputCount {
			return fmt.Errorf("asset binding write result index %d out of range", item.InputIndex)
		}
		if seen[item.InputIndex] {
			return fmt.Errorf("duplicate asset binding write result index %d", item.InputIndex)
		}
		if item.Disposition == pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_UNSPECIFIED {
			return fmt.Errorf("asset binding write result index %d has unspecified disposition", item.InputIndex)
		}
		seen[item.InputIndex] = true
	}
	return nil
}

func (p *Producer) WriteAssetBindings(ctx context.Context, bindings []*pb.MacIpBinding) (AssetBindingWriteResult, error) {
	ctx, span := otel.StartSpan(ctx, "producer.write_asset_bindings")
	defer span.End()

	result := AssetBindingWriteResult{Items: make([]AssetBindingWriteItemResult, len(bindings))}
	messages := make([]kafkaCommon.Message, 0, len(bindings))
	messageInputIndexes := make([]int, 0, len(bindings))
	for inputIndex, binding := range bindings {
		item := AssetBindingWriteItemResult{
			InputIndex:  inputIndex,
			Disposition: pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_REJECTED_INVALID,
			AckScope:    "INPUT_ITEM",
		}
		if binding == nil {
			item.ReasonCode = "BINDING_REQUIRED"
			result.Items[inputIndex] = item
			continue
		}
		item.ObservationID = binding.ObservationId
		if binding.TenantId == "" || binding.ProbeId == "" || binding.ObservationId == "" ||
			binding.MacAddress == "" || binding.IpAddress == "" || binding.ObservedAt <= 0 ||
			(binding.Source != "arp" && binding.Source != "dhcp") || binding.SchemaVersion != 1 {
			item.ReasonCode = "BINDING_CONTRACT_INVALID"
			result.Items[inputIndex] = item
			continue
		}
		value, err := proto.Marshal(binding)
		if err != nil {
			item.ReasonCode = "PROTO_ENCODE_INVALID"
			result.Items[inputIndex] = item
			continue
		}
		now := time.Now()
		messages = append(messages, kafkaCommon.Message{
			Key:   fmt.Sprintf("%s:%s", binding.TenantId, binding.MacAddress),
			Value: value,
			Headers: []kafkaCommon.MessageHeader{
				{Key: "tenant_id", Value: binding.TenantId},
				{Key: "probe_id", Value: binding.ProbeId},
				{Key: "observation_id", Value: binding.ObservationId},
				{Key: "event_id", Value: binding.ObservationId},
				{Key: "source", Value: binding.Source},
				{Key: "schema_version", Value: "1"},
				{Key: "content_type", Value: config.ContentTypeProtobuf},
				{Key: "message_type", Value: config.ProtoMessageAssetBinding},
			},
			Time: now,
		})
		messageInputIndexes = append(messageInputIndexes, inputIndex)
		item.Disposition = pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_RETRYABLE
		item.ReasonCode = "KAFKA_NOT_ATTEMPTED"
		item.AckScope = "KAFKA_RECORD"
		result.Items[inputIndex] = item
	}
	if len(messages) == 0 {
		return result, nil
	}

	writeBindingBatch := p.writeBindingBatch
	if writeBindingBatch == nil && p.multiProducer != nil {
		writeBindingBatch = p.multiProducer.SendBatch
	}
	if writeBindingBatch == nil {
		err := fmt.Errorf("asset binding Kafka writer is not configured")
		classifyAssetBindingWholeBatchFailure(ctx, result.Items, messageInputIndexes, err)
		return result, err
	}
	if err := writeBindingBatch(ctx, p.config.BindingTopic, messages); err != nil {
		classifyAssetBindingKafkaWriteFailure(ctx, result.Items, messageInputIndexes, err)
		return result, fmt.Errorf("failed to write asset bindings: %w", err)
	}
	for _, inputIndex := range messageInputIndexes {
		result.Items[inputIndex].Disposition = pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_KAFKA_ACKED
		result.Items[inputIndex].ReasonCode = "KAFKA_REQUIRED_ACKS_ALL"
	}
	return result, nil
}

func classifyAssetBindingKafkaWriteFailure(ctx context.Context, items []AssetBindingWriteItemResult, indexes []int, err error) {
	var writeErrors kafka.WriteErrors
	if errors.As(err, &writeErrors) && len(writeErrors) == len(indexes) {
		for messageIndex, writeErr := range writeErrors {
			inputIndex := indexes[messageIndex]
			if writeErr == nil {
				items[inputIndex].Disposition = pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_KAFKA_ACKED
				items[inputIndex].ReasonCode = "KAFKA_REQUIRED_ACKS_ALL"
				continue
			}
			classifyOneAssetBindingWriteFailure(ctx, &items[inputIndex], writeErr)
		}
		return
	}
	classifyAssetBindingWholeBatchFailure(ctx, items, indexes, err)
}

func classifyAssetBindingWholeBatchFailure(ctx context.Context, items []AssetBindingWriteItemResult, indexes []int, err error) {
	for _, inputIndex := range indexes {
		classifyOneAssetBindingWriteFailure(ctx, &items[inputIndex], err)
	}
}

func classifyOneAssetBindingWriteFailure(ctx context.Context, item *AssetBindingWriteItemResult, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		item.Disposition = pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_OUTCOME_UNKNOWN
		item.ReasonCode = "KAFKA_OUTCOME_UNKNOWN"
		return
	}
	item.Disposition = pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_RETRYABLE
	item.ReasonCode = "KAFKA_RETRYABLE"
}

type FlowWriteItemResult struct {
	InputIndex  int
	EventID     string
	Disposition pb.FlowItemDisposition
	ReasonCode  string
	AckScope    string
}

type BatchWriteResult struct {
	Items []FlowWriteItemResult
}

func (r BatchWriteResult) ValidateExactSet(inputCount int) error {
	if len(r.Items) != inputCount {
		return fmt.Errorf("flow write result cardinality mismatch: got %d want %d", len(r.Items), inputCount)
	}
	seen := make([]bool, inputCount)
	for _, item := range r.Items {
		if item.InputIndex < 0 || item.InputIndex >= inputCount {
			return fmt.Errorf("flow write result index %d out of range", item.InputIndex)
		}
		if seen[item.InputIndex] {
			return fmt.Errorf("duplicate flow write result index %d", item.InputIndex)
		}
		if item.Disposition == pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_UNSPECIFIED {
			return fmt.Errorf("flow write result index %d has unspecified disposition", item.InputIndex)
		}
		seen[item.InputIndex] = true
	}
	return nil
}

func (p *Producer) WriteFlowEvents(ctx context.Context, events []*pb.FlowEvent) (BatchWriteResult, error) {
	ctx, span := otel.StartSpan(ctx, "producer.write_flow_events")
	defer span.End()

	result := BatchWriteResult{Items: make([]FlowWriteItemResult, len(events))}
	if len(events) == 0 {
		return result, nil
	}

	logger := logging.L(ctx)

	messages := make([]kafkaCommon.Message, 0, len(events))
	messageInputIndexes := make([]int, 0, len(events))

	for inputIndex, event := range events {
		item := FlowWriteItemResult{
			InputIndex:  inputIndex,
			Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_REJECTED_INVALID,
			AckScope:    "INPUT_ITEM",
		}
		if event == nil || event.Header == nil {
			item.ReasonCode = "INVALID_EVENT_HEADER"
			result.Items[inputIndex] = item
			continue
		}
		item.EventID = event.Header.EventId
		if event.Header.EventId == "" {
			item.ReasonCode = "EVENT_ID_REQUIRED"
			result.Items[inputIndex] = item
			continue
		}

		kafkaTs := time.Now().UnixMilli()
		event.Header.KafkaTs = kafkaTs
		event.Header.FlinkOutTs = 0

		if p.config.EnableValidation {
			p.validateFlowEvent(event, logger)
		}

		value, err := proto.Marshal(event)
		if err != nil {
			logger.Error("Failed to marshal flow event",
				zap.String("event_id", event.Header.EventId),
				zap.Error(err))
			item.ReasonCode = "PROTO_ENCODE_INVALID"
			result.Items[inputIndex] = item
			continue
		}

		key := fmt.Sprintf("%s:%s", event.Header.TenantId, event.CommunityId)

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
			{Key: "kafka_ts", Value: fmt.Sprintf("%d", event.Header.KafkaTs)},
		}

		messages = append(messages, kafkaCommon.Message{
			Key:     key,
			Value:   value,
			Headers: headers,
			Time:    time.UnixMilli(kafkaTs),
		})
		messageInputIndexes = append(messageInputIndexes, inputIndex)
		item.Disposition = pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE
		item.ReasonCode = "KAFKA_NOT_ATTEMPTED"
		item.AckScope = "KAFKA_RECORD"
		result.Items[inputIndex] = item
	}

	if len(messages) == 0 {
		return result, nil
	}

	start := time.Now()
	writeFlowBatch := p.writeFlowBatch
	if writeFlowBatch == nil && p.multiProducer != nil {
		writeFlowBatch = p.multiProducer.SendBatch
	}
	if writeFlowBatch == nil {
		err := fmt.Errorf("flow Kafka writer is not configured")
		classifyWholeBatchFailure(ctx, result.Items, messageInputIndexes, err)
		return result, err
	}

	if err := writeFlowBatch(ctx, p.config.FlowTopic, messages); err != nil {
		classifyKafkaWriteFailure(ctx, result.Items, messageInputIndexes, err)
		logger.Error("Failed to write flow events",
			zap.Int("count", len(messages)),
			zap.Error(err))
		return result, fmt.Errorf("failed to write flow events: %w", err)
	}

	for _, inputIndex := range messageInputIndexes {
		result.Items[inputIndex].Disposition = pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED
		result.Items[inputIndex].ReasonCode = "KAFKA_REQUIRED_ACKS_ALL"
	}

	logger.Debug("Flow events written",
		zap.Int("count", len(messages)),
		zap.Duration("duration", time.Since(start)))

	return result, nil
}

func classifyKafkaWriteFailure(
	ctx context.Context,
	items []FlowWriteItemResult,
	messageInputIndexes []int,
	err error,
) {
	var writeErrors kafka.WriteErrors
	if errors.As(err, &writeErrors) && len(writeErrors) == len(messageInputIndexes) {
		for messageIndex, writeErr := range writeErrors {
			inputIndex := messageInputIndexes[messageIndex]
			if writeErr == nil {
				items[inputIndex].Disposition = pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED
				items[inputIndex].ReasonCode = "KAFKA_REQUIRED_ACKS_ALL"
				continue
			}
			classifyOneWriteFailure(ctx, &items[inputIndex], writeErr)
		}
		return
	}
	classifyWholeBatchFailure(ctx, items, messageInputIndexes, err)
}

func classifyWholeBatchFailure(
	ctx context.Context,
	items []FlowWriteItemResult,
	messageInputIndexes []int,
	err error,
) {
	for _, inputIndex := range messageInputIndexes {
		classifyOneWriteFailure(ctx, &items[inputIndex], err)
	}
}

func classifyOneWriteFailure(ctx context.Context, item *FlowWriteItemResult, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		item.Disposition = pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_OUTCOME_UNKNOWN
		item.ReasonCode = "KAFKA_OUTCOME_UNKNOWN"
		return
	}
	item.Disposition = pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE
	item.ReasonCode = "KAFKA_RETRYABLE"
}

func (p *Producer) WritePcapIndex(ctx context.Context, meta *pb.PcapIndexMeta) error {
	ctx, span := otel.StartSpan(ctx, "producer.write_pcap_index")
	defer span.End()

	if meta == nil {
		return fmt.Errorf("invalid pcap index meta: nil")
	}

	logger := logging.L(ctx)

	if p.config.EnableValidation {
		p.validatePcapIndex(meta, logger)
	}

	value, err := proto.Marshal(meta)
	if err != nil {
		logger.Error("Failed to marshal pcap index meta",
			zap.String("file_key", meta.FileKey),
			zap.Error(err))
		return fmt.Errorf("failed to marshal pcap index: %w", err)
	}

	key := fmt.Sprintf("%s:%s", meta.TenantId, meta.ProbeId)

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

func (p *Producer) WriteSessionEvents(ctx context.Context, sessions []*pb.SessionEvent) error {
	ctx, span := otel.StartSpan(ctx, "producer.write_session_events")
	defer span.End()

	if len(sessions) == 0 {
		return nil
	}

	logger := logging.L(ctx)

	messages := make([]kafkaCommon.Message, 0, len(sessions))

	for _, session := range sessions {
		if session == nil || session.Header == nil {
			continue
		}

		kafkaTs := time.Now().UnixMilli()
		session.Header.KafkaTs = kafkaTs
		session.Header.FlinkOutTs = 0

		if p.config.EnableValidation {
			p.validateSessionEvent(session, logger)
		}

		value, err := proto.Marshal(session)
		if err != nil {
			logger.Error("Failed to marshal session event",
				zap.String("session_id", session.SessionId),
				zap.Error(err))
			continue
		}

		key := fmt.Sprintf("%s:%s", session.Header.TenantId, session.CommunityId)

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
			{Key: "ingest_ts", Value: fmt.Sprintf("%d", session.Header.IngestTs)},
			{Key: "kafka_ts", Value: fmt.Sprintf("%d", session.Header.KafkaTs)},
		}

		messages = append(messages, kafkaCommon.Message{
			Key:     key,
			Value:   value,
			Headers: headers,
			Time:    time.UnixMilli(kafkaTs),
		})
	}

	if len(messages) == 0 {
		return nil
	}

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

type ProducerMetrics struct {
	FlowMessagesSent       int64
	FlowMessagesError      int64
	PcapIndexMessagesSent  int64
	PcapIndexMessagesError int64
	SessionMessagesSent    int64
	SessionMessagesError   int64
	LastSendTime           time.Time
}

func (p *Producer) Close() error {
	return p.multiProducer.Close()
}

func (p *Producer) Healthy() bool {
	return p.multiProducer != nil
}

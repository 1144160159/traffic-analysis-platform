package consumer

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

type bindingRecorder interface {
	RecordMacIpBinding(context.Context, []*config.MacIpBinding, service.BindingProvenance) (int32, int32, error)
}

type BindingConsumer struct {
	consumer      *kafkaCommon.Consumer
	recorder      bindingRecorder
	topic         string
	consumerGroup string
	maxBytes      int
	logger        *zap.Logger
}

func NewBindingConsumer(
	cfg config.KafkaConfig,
	recorder bindingRecorder,
	barrier kafkaCommon.DLQAcknowledgementBarrier,
	logger *zap.Logger,
) (*BindingConsumer, error) {
	brokers := cfg.BrokerList()
	if len(brokers) == 0 || strings.TrimSpace(cfg.Topic) == "" || strings.TrimSpace(cfg.GroupID) == "" {
		return nil, fmt.Errorf("asset binding Kafka brokers, topic and group are required")
	}
	if cfg.Topic != "asset.bindings.v1" || cfg.BindingDLQTopic != "dlq.v1" {
		return nil, fmt.Errorf("asset binding source and DLQ topics must be asset.bindings.v1 and dlq.v1")
	}
	if recorder == nil || barrier == nil {
		return nil, fmt.Errorf("asset binding recorder and durable DLQ acknowledgement barrier are required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	owned, err := kafkaCommon.NewConsumer(kafkaCommon.ConsumerConfig{
		Brokers: brokers, Topic: cfg.Topic, GroupID: cfg.GroupID,
		MinBytes: cfg.MinBytes, MaxBytes: maxBytes, StartOffset: -2,
		MaxRetries: cfg.BindingMaxAttempts, RetryBackoff: time.Second,
		EnableDLQ: true, DLQTopic: cfg.BindingDLQTopic,
		CommitOnDLQSuccess: true, CommitOnHandlerError: false, DLQPermanentOnly: true,
		Security: cfg.Security,
	}, logger)
	if err != nil {
		return nil, err
	}
	owned.SetDLQAcknowledgementBarrier(barrier)
	return &BindingConsumer{
		consumer: owned, recorder: recorder, topic: cfg.Topic,
		consumerGroup: cfg.GroupID, maxBytes: maxBytes, logger: logger,
	}, nil
}

func (c *BindingConsumer) Run(ctx context.Context) {
	c.logger.Info("asset binding consumer started",
		zap.String("topic", c.topic), zap.String("group_id", c.consumerGroup))
	if err := c.consumer.Consume(ctx, c.handleMessage); err != nil && ctx.Err() == nil {
		c.logger.Error("asset binding consumer stopped", zap.Error(err))
	}
}

func (c *BindingConsumer) handleMessage(ctx context.Context, message *kafkaCommon.ReceivedMessage) error {
	binding, err := decodeAndValidateBinding(message, c.topic, c.maxBytes)
	if err != nil {
		return kafkaCommon.Permanent(err)
	}
	accepted, rejected, err := c.recorder.RecordMacIpBinding(
		ctx,
		[]*config.MacIpBinding{binding},
		service.BindingProvenance{
			Channel: service.BindingChannelKafka, Topic: message.Topic,
			Partition: message.Partition, Offset: message.Offset, MessageTime: message.Time,
			Actor:     "probe:" + binding.ProbeID,
			TraceID:   "asset-binding:" + binding.ObservationID,
			RequestID: fmt.Sprintf("%s/%d/%d", message.Topic, message.Partition, message.Offset),
		},
	)
	if err != nil {
		return fmt.Errorf("record asset binding authority transaction: %w", err)
	}
	if accepted != 1 || rejected != 0 {
		return kafkaCommon.Permanent(fmt.Errorf(
			"asset binding authority rejected record: accepted=%d rejected=%d", accepted, rejected))
	}
	return nil
}

func (c *BindingConsumer) Close() error {
	if c == nil || c.consumer == nil {
		return nil
	}
	return c.consumer.Close()
}

func decodeAndValidateBinding(message *kafkaCommon.ReceivedMessage, expectedTopic string, maxBytes int) (*config.MacIpBinding, error) {
	if message == nil {
		return nil, fmt.Errorf("asset binding message is required")
	}
	if message.Topic != expectedTopic {
		return nil, fmt.Errorf("asset binding source topic mismatch")
	}
	if len(message.DuplicateHeaderNames()) != 0 {
		return nil, fmt.Errorf("asset binding envelope contains duplicate headers")
	}
	if len(message.Value) == 0 || len(message.Value) > maxBytes {
		return nil, fmt.Errorf("asset binding payload size is invalid")
	}
	var wire pb.MacIpBinding
	if err := proto.Unmarshal(message.Value, &wire); err != nil {
		return nil, fmt.Errorf("invalid MacIpBinding protobuf: %w", err)
	}
	if len(wire.ProtoReflect().GetUnknown()) != 0 {
		return nil, fmt.Errorf("MacIpBinding contains unknown protobuf fields")
	}
	canonicalMAC, err := canonicalMACAddress(wire.MacAddress)
	if err != nil || canonicalMAC != wire.MacAddress {
		return nil, fmt.Errorf("asset binding MAC address is not canonical")
	}
	if parsed := net.ParseIP(wire.IpAddress); parsed == nil || parsed.String() != wire.IpAddress {
		return nil, fmt.Errorf("asset binding IP address is not canonical")
	}
	if strings.TrimSpace(wire.TenantId) == "" || strings.TrimSpace(wire.ProbeId) == "" ||
		strings.TrimSpace(wire.ObservationId) == "" || wire.ObservedAt <= 0 || wire.SchemaVersion != 1 {
		return nil, fmt.Errorf("asset binding stable identity is incomplete")
	}
	if wire.Source != "arp" && wire.Source != "dhcp" {
		return nil, fmt.Errorf("asset binding source must be arp or dhcp")
	}
	if message.Time.IsZero() || wire.ObservedAt > message.Time.Add(5*time.Minute).UnixMilli() {
		return nil, fmt.Errorf("asset binding timestamp exceeds Kafka time plus allowed skew")
	}
	requiredHeaders := map[string]string{
		"tenant_id": wire.TenantId, "probe_id": wire.ProbeId,
		"observation_id": wire.ObservationId, "event_id": wire.ObservationId,
		"source": wire.Source, "schema_version": "1",
		"content_type": "application/x-protobuf", "message_type": "traffic.v1.MacIpBinding",
	}
	for name, expected := range requiredHeaders {
		if message.GetHeader(name) != expected {
			return nil, fmt.Errorf("asset binding header %s is missing or inconsistent", name)
		}
	}
	if string(message.Key) != wire.TenantId+":"+wire.MacAddress {
		return nil, fmt.Errorf("asset binding Kafka key must equal tenant_id:mac_address")
	}
	return &config.MacIpBinding{
		MACAddress: wire.MacAddress, IPAddress: wire.IpAddress,
		TenantID: wire.TenantId, ObservedAt: wire.ObservedAt, Source: wire.Source,
		ObservationID: wire.ObservationId, ProbeID: wire.ProbeId,
		VlanID: wire.VlanId, SourceEventID: wire.SourceEventId,
	}, nil
}

func canonicalMACAddress(value string) (string, error) {
	parsed, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil || len(parsed) != 6 {
		return "", fmt.Errorf("invalid MAC address")
	}
	return strings.ToLower(parsed.String()), nil
}

package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

// ProbeCommandPublisher 探针命令发布端口(调度中心唯一出站动作:协议转换后的命令投递)。
type ProbeCommandPublisher interface {
	Publish(ctx context.Context, env contract.ProbeCommandEnvelope) error
}

// KafkaProbeCommandPublisher 经 probe.control.v2 投递命令(含桥接器强校验的
// canonical headers;与 alert-service probe outbox 同契约)。
type KafkaProbeCommandPublisher struct {
	multi *kafkaCommon.MultiTopicProducer
}

// NewKafkaProbeCommandPublisher 构造(复用统一 Kafka 身份;acks=all)。
func NewKafkaProbeCommandPublisher(brokers []string, security kafkaCommon.SecurityConfig, logger *zap.Logger) (*KafkaProbeCommandPublisher, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	multi := kafkaCommon.NewMultiTopicProducer(logger)
	cfg := kafkaCommon.ProducerConfig{
		Brokers: brokers, RequiredAcks: "all", Compression: "lz4", Security: security,
	}
	if err := multi.AddTopic(contract.ProbeControlTopic, cfg); err != nil {
		return nil, err
	}
	return &KafkaProbeCommandPublisher{multi: multi}, nil
}

// Publish 投递命令信封 + canonical headers(bridge 逐键校验)。
func (p *KafkaProbeCommandPublisher) Publish(ctx context.Context, env contract.ProbeCommandEnvelope) error {
	value, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal probe command: %w", err)
	}
	revision := strconv.FormatInt(env.CommandRevision, 10)
	headers := []kafkaCommon.MessageHeader{
		{Key: "event_id", Value: env.EventID},
		{Key: "event_type", Value: env.EventType},
		{Key: "tenant_id", Value: env.TenantID},
		{Key: "probe_id", Value: env.ProbeID},
		{Key: "operation_id", Value: env.OperationID},
		{Key: "command_revision", Value: revision},
		{Key: "aggregate_version", Value: revision},
		{Key: "schema_version", Value: strconv.Itoa(env.SchemaVersion)},
		{Key: "target_topic", Value: contract.ProbeControlTopic},
	}
	if err := p.multi.Send(ctx, contract.ProbeControlTopic, env.TenantID+":"+env.ProbeID, value, headers...); err != nil {
		return fmt.Errorf("publish probe command: %w", err)
	}
	return nil
}

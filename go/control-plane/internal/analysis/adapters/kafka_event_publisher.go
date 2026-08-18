// Package adapters Kafka 权威事件发布器(outbox 中继的传输实现)。
package adapters

import (
	"context"

	"go.uber.org/zap"

	commoncontracts "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/contracts"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

// KafkaEventPublisher 以 MultiTopicProducer 发布权威 topic(每条消息按 key 哈希分区)。
type KafkaEventPublisher struct {
	multi *kafkaCommon.MultiTopicProducer
}

// NewKafkaEventPublisher 为 run/plan/report 三个权威 topic 各建一个生产者
// (SASL_SSL 凭据经 SecurityConfig 注入;acks=all)。
func NewKafkaEventPublisher(brokers []string, security kafkaCommon.SecurityConfig, logger *zap.Logger) (*KafkaEventPublisher, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	multi := kafkaCommon.NewMultiTopicProducer(logger)
	for _, topic := range []string{commoncontracts.TopicAnalysisRunEvents, commoncontracts.TopicAnalysisPlanEvents, commoncontracts.TopicAnalysisReportRequests} {
		cfg := kafkaCommon.ProducerConfig{
			Brokers:      brokers,
			RequiredAcks: "all",
			Compression:  "lz4",
			Security:     security,
		}
		if err := multi.AddTopic(topic, cfg); err != nil {
			return nil, err
		}
	}
	return &KafkaEventPublisher{multi: multi}, nil
}

// Publish 发送一条键控消息。
func (p *KafkaEventPublisher) Publish(ctx context.Context, topic, key string, payload []byte) error {
	return p.multi.Send(ctx, topic, key, payload)
}

// Close 释放所有生产者。
func (p *KafkaEventPublisher) Close() error {
	if p.multi != nil {
		p.multi.Close()
	}
	return nil
}

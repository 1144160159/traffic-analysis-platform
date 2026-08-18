package main

import (
	"fmt"
	"strings"
	"time"

	alertconfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

func newProbeAckConsumerConfig(cfg alertconfig.KafkaConfig) (commonkafka.ConsumerConfig, error) {
	if len(cfg.Brokers) == 0 || strings.TrimSpace(cfg.ProbeAckTopic) == "" ||
		strings.TrimSpace(cfg.ProbeAckGroup) == "" {
		return commonkafka.ConsumerConfig{}, fmt.Errorf("probe ACK brokers topic and group are required")
	}
	return commonkafka.ConsumerConfig{
		Brokers:              cfg.Brokers,
		Topic:                cfg.ProbeAckTopic,
		GroupID:              cfg.ProbeAckGroup,
		MaxRetries:           3,
		RetryBackoff:         time.Second,
		EnableDLQ:            true,
		DLQTopicPrefix:       "dlq.",
		CommitOnDLQSuccess:   true,
		CommitOnHandlerError: false,
		DLQPermanentOnly:     true,
		Security:             cfg.Security,
	}, nil
}

package config

import (
	"fmt"
	"strings"
)

// ProbeControlPipelineConfig preserves independent rollout/rollback controls.
// The generation consumer and readiness publisher form one topology but are
// still enabled separately so producer-first expansion remains possible.
type ProbeControlPipelineConfig struct {
	CommandConsumerEnabled    bool `env:"PROBE_COMMAND_CONSUMER_V2_ENABLED" envDefault:"false"`
	HeartbeatDeliveryEnabled  bool `env:"PROBE_HEARTBEAT_DELIVERY_V2_ENABLED" envDefault:"false"`
	AckPublisherEnabled       bool `env:"PROBE_ACK_PUBLISHER_V2_ENABLED" envDefault:"false"`
	ReadinessPublisherEnabled bool `env:"PROBE_READINESS_PUBLISHER_V1_ENABLED" envDefault:"false"`
}

func (cfg ProbeControlPipelineConfig) Validate(kafka KafkaConfig) error {
	if cfg.HeartbeatDeliveryEnabled && (!cfg.CommandConsumerEnabled || !cfg.AckPublisherEnabled) {
		return fmt.Errorf("probe heartbeat delivery requires command consumer and ACK publisher")
	}
	if cfg.ReadinessPublisherEnabled && !cfg.CommandConsumerEnabled {
		return fmt.Errorf("probe readiness publisher requires the generation command consumer")
	}
	if cfg.CommandConsumerEnabled &&
		(len(kafka.Brokers) == 0 || strings.TrimSpace(kafka.ProbeControlTopic) == "" ||
			strings.TrimSpace(kafka.ProbeControlGroup) == "") {
		return fmt.Errorf("probe command consumer brokers topic and group are required")
	}
	if cfg.ReadinessPublisherEnabled && kafka.ProbeGroupReadinessTopic != "probe.group-readiness.v1" {
		return fmt.Errorf("probe readiness publisher requires its fixed topic")
	}
	return nil
}

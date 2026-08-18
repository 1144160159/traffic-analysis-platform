package config

import (
	"fmt"
	"strings"
)

// ProbeOperationPipelineConfig separates every authority and publisher. All
// flags are default-false; no legacy aggregate flag may enable this pipeline.
type ProbeOperationPipelineConfig struct {
	AckConsumerEnabled        bool `env:"PROBE_ACK_CONSUMER_V2_ENABLED" envDefault:"false"`
	LifecycleConsumerEnabled  bool `env:"PROBE_LIFECYCLE_CONSUMER_V2_ENABLED" envDefault:"false"`
	ReadinessConsumerEnabled  bool `env:"PROBE_READINESS_CONSUMER_V1_ENABLED" envDefault:"false"`
	DesiredWriterEnabled      bool `env:"PROBE_DESIRED_WRITER_V2_ENABLED" envDefault:"false"`
	ControlPublisherEnabled   bool `env:"PROBE_CONTROL_PUBLISHER_V2_ENABLED" envDefault:"false"`
	LifecyclePublisherEnabled bool `env:"PROBE_LIFECYCLE_PUBLISHER_V2_ENABLED" envDefault:"false"`
	DispatcherEnabled         bool `env:"PROBE_DISPATCHER_V2_ENABLED" envDefault:"false"`
}

func (cfg ProbeOperationPipelineConfig) Validate(kafka KafkaConfig) error {
	if cfg.DispatcherEnabled && (!cfg.AckConsumerEnabled || !cfg.LifecycleConsumerEnabled ||
		!cfg.ReadinessConsumerEnabled || !cfg.ControlPublisherEnabled ||
		!cfg.LifecyclePublisherEnabled) {
		return fmt.Errorf("probe dispatcher requires ACK lifecycle readiness consumers and both publishers")
	}
	if (cfg.AckConsumerEnabled || cfg.LifecycleConsumerEnabled || cfg.ReadinessConsumerEnabled ||
		cfg.ControlPublisherEnabled || cfg.LifecyclePublisherEnabled) && len(kafka.Brokers) == 0 {
		return fmt.Errorf("probe operation pipeline Kafka brokers are required")
	}
	if cfg.AckConsumerEnabled && (strings.TrimSpace(kafka.ProbeAckTopic) == "" || strings.TrimSpace(kafka.ProbeAckGroup) == "") {
		return fmt.Errorf("probe ACK consumer topic and group are required")
	}
	if cfg.LifecycleConsumerEnabled && (strings.TrimSpace(kafka.ProbeEventTopic) == "" || strings.TrimSpace(kafka.ProbeEventGroup) == "") {
		return fmt.Errorf("probe lifecycle consumer topic and group are required")
	}
	if cfg.ReadinessConsumerEnabled &&
		(kafka.ProbeGroupReadinessTopic != "probe.group-readiness.v1" ||
			kafka.ProbeGroupReadinessGroup != "alert-service-probe-group-readiness-v1") {
		return fmt.Errorf("probe readiness consumer requires its fixed topic and group")
	}
	return nil
}

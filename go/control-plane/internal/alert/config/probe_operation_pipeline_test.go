package config

import "testing"

func TestProbeOperationPipelineDefaultsFailClosed(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProbeOperation != (ProbeOperationPipelineConfig{}) {
		t.Fatalf("probe operation flags must default false: %#v", cfg.ProbeOperation)
	}
}

func TestProbeOperationDispatcherDependencyClosure(t *testing.T) {
	kafka := KafkaConfig{
		Brokers: []string{"broker:9092"}, ProbeAckTopic: "probe.acks.v2",
		ProbeAckGroup: "alert-service-probe-acks-v2", ProbeEventTopic: "probe.events.v2",
		ProbeEventGroup:          "alert-service-probe-event-projection-v2",
		ProbeGroupReadinessTopic: "probe.group-readiness.v1",
		ProbeGroupReadinessGroup: "alert-service-probe-group-readiness-v1",
	}
	if err := (ProbeOperationPipelineConfig{DispatcherEnabled: true}).Validate(kafka); err == nil {
		t.Fatal("dispatcher without consumer/publisher closure was accepted")
	}
	complete := ProbeOperationPipelineConfig{
		AckConsumerEnabled: true, LifecycleConsumerEnabled: true,
		ReadinessConsumerEnabled: true, ControlPublisherEnabled: true,
		LifecyclePublisherEnabled: true, DispatcherEnabled: true,
	}
	if err := complete.Validate(kafka); err != nil {
		t.Fatal(err)
	}
}

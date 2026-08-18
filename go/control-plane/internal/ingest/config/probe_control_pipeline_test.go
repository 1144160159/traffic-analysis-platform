package config

import "testing"

func TestProbeControlPipelineDefaultsFailClosed(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	if cfg.ProbeControl != (ProbeControlPipelineConfig{}) {
		t.Fatalf("probe control flags must default false: %#v", cfg.ProbeControl)
	}
}

func TestProbeControlPipelineDependencyClosure(t *testing.T) {
	kafka := KafkaConfig{
		Brokers: []string{"broker:9092"}, ProbeControlTopic: "probe.control.v2",
		ProbeControlGroup:        "ingest-gateway-probe-control-v2",
		ProbeGroupReadinessTopic: "probe.group-readiness.v1",
	}
	if err := (ProbeControlPipelineConfig{HeartbeatDeliveryEnabled: true}).Validate(kafka); err == nil {
		t.Fatal("heartbeat delivery without consumer and ACK publisher was accepted")
	}
	if err := (ProbeControlPipelineConfig{ReadinessPublisherEnabled: true}).Validate(kafka); err == nil {
		t.Fatal("readiness publisher without generation consumer was accepted")
	}
	complete := ProbeControlPipelineConfig{
		CommandConsumerEnabled: true, HeartbeatDeliveryEnabled: true,
		AckPublisherEnabled: true, ReadinessPublisherEnabled: true,
	}
	if err := complete.Validate(kafka); err != nil {
		t.Fatal(err)
	}
}

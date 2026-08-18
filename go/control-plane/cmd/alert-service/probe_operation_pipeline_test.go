package main

import (
	"testing"

	alertconfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
)

func TestNewProbeAckConsumerConfigFailsClosed(t *testing.T) {
	cfg, err := newProbeAckConsumerConfig(alertconfig.KafkaConfig{
		Brokers: []string{"kafka:9092"}, ProbeAckTopic: "probe.acks.v2",
		ProbeAckGroup: "alert-service-probe-acks-v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.EnableDLQ || !cfg.CommitOnDLQSuccess || cfg.CommitOnHandlerError || !cfg.DLQPermanentOnly {
		t.Fatalf("unsafe probe ACK consumer config: %#v", cfg)
	}
	if _, err := newProbeAckConsumerConfig(alertconfig.KafkaConfig{}); err == nil {
		t.Fatal("expected incomplete config error")
	}
}

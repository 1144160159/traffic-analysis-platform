package queue

import (
	"testing"

	"go.uber.org/zap"
)

func TestProducerConfigDefaults(t *testing.T) {
	cfg := ProducerConfig{
		Brokers:    []string{"localhost:9092"},
		FlowTopic:  "",
		BatchSize:  0,
		Compression: "",
	}
	p, err := NewProducer(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	if p.config.FlowTopic != "flow.events.v1" {
		t.Errorf("FlowTopic=%q, want flow.events.v1", p.config.FlowTopic)
	}
	if p.config.SessionTopic != "session.events.v1" {
		t.Errorf("SessionTopic=%q", p.config.SessionTopic)
	}
	if p.config.PcapIndexTopic != "pcap.index.v1" {
		t.Errorf("PcapIndexTopic=%q", p.config.PcapIndexTopic)
	}
}

func TestProducerNoBrokers(t *testing.T) {
	_, err := NewProducer(ProducerConfig{}, zap.NewNop())
	if err == nil {
		t.Error("expected error for empty brokers")
	}
}

func TestProducerCustomTopics(t *testing.T) {
	cfg := ProducerConfig{
		Brokers:        []string{"localhost:9092"},
		FlowTopic:      "custom.flow",
		SessionTopic:   "custom.session",
		PcapIndexTopic: "custom.pcap",
	}
	p, err := NewProducer(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if p.config.FlowTopic != "custom.flow" {
		t.Errorf("FlowTopic=%q", p.config.FlowTopic)
	}
}

func TestTenantCommunityPartitioner(t *testing.T) {
	partitioner := NewTenantCommunityPartitioner(12)
	p1 := partitioner.Partition("tenant-01", "community-id-12345")
	if p1 >= 12 || p1 < 0 {
		t.Errorf("partition=%d out of range [0,12)", p1)
	}
	// Same keys should map to same partition
	p2 := partitioner.Partition("tenant-01", "community-id-12345")
	if p2 != p1 {
		t.Errorf("same keys mapped to different partitions: %d vs %d", p2, p1)
	}
	// Different keys may map to different partitions (no strict assertion)
	p3 := partitioner.Partition("tenant-02", "other-community")
	if p3 >= 12 || p3 < 0 {
		t.Errorf("partition=%d out of range", p3)
	}
}

func TestProducerConfig(t *testing.T) {
	cfg := ProducerConfig{
		Brokers:           []string{"kafka:9092"},
		FlowTopic:         "flow.events.v1",
		BatchSize:         1000,
		BatchTimeout:      100,
		Compression:       "lz4",
		RequiredAcks:      "all",
		MaxRetries:        3,
		EnableIdempotence: true,
	}
	if cfg.FlowTopic != "flow.events.v1" {
		t.Error("FlowTopic mismatch")
	}
	if cfg.MaxRetries != 3 {
		t.Error("MaxRetries mismatch")
	}
}

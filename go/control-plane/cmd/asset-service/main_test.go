package main

import (
	"testing"

	segmentKafka "github.com/segmentio/kafka-go"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func TestAssetProjectionConsumerConfigReplaysRetainedLogFailClosed(t *testing.T) {
	cfg := config.KafkaConfig{
		Brokers:               "kafka-a:9092,kafka-b:9092",
		EventTopic:            "asset.events.v2",
		ProjectionGroupID:     "asset-projection-v2",
		MinBytes:              1,
		MaxBytes:              1 << 20,
		ProjectionMaxAttempts: 8,
	}

	got := assetProjectionConsumerConfig(cfg)
	if got.StartOffset != segmentKafka.FirstOffset {
		t.Fatalf("start offset=%d want earliest=%d", got.StartOffset, segmentKafka.FirstOffset)
	}
	if got.Topic != cfg.EventTopic || got.GroupID != cfg.ProjectionGroupID {
		t.Fatalf("unexpected topic/group: topic=%q group=%q", got.Topic, got.GroupID)
	}
	if got.CommitOnHandlerError {
		t.Fatal("projection consumer must not commit a failed projection event")
	}
	if got.EnableDLQ {
		t.Fatal("projection consumer must not enable DLQ until commit-after-DLQ is wired and verified")
	}
}

package kafka

import "testing"

func TestResolveDLQTopicUsesRegisteredTarget(t *testing.T) {
	config := DLQConfig{TopicPrefix: "dlq.", TargetTopic: "dlq.v1"}
	if got := resolveDLQTopic(config, "traffic.topic.action.v2"); got != "dlq.v1" {
		t.Fatalf("resolved DLQ topic=%q want dlq.v1", got)
	}
}

func TestResolveDLQTopicPreservesLegacyPrefixMode(t *testing.T) {
	config := DLQConfig{TopicPrefix: "dlq."}
	if got := resolveDLQTopic(config, "probe.acks.v2"); got != "dlq.probe.acks.v2" {
		t.Fatalf("resolved DLQ topic=%q want dlq.probe.acks.v2", got)
	}
}

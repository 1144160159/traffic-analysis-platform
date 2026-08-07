package kafka

import (
	"testing"

	segmentKafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func TestCommitObserverReceivesIsolatedCommittedMessages(t *testing.T) {
	consumer := &Consumer{logger: zap.NewNop()}
	var observed []segmentKafka.Message
	consumer.SetCommitObserver(func(messages []segmentKafka.Message) {
		observed = messages
		messages[0].Offset = 999
	})
	original := []segmentKafka.Message{{Topic: "detections.v1", Partition: 2, Offset: 41}}
	consumer.notifyCommitObserver(original)
	if len(observed) != 1 || original[0].Offset != 41 {
		t.Fatalf("commit observer did not receive an isolated copy: observed=%v original=%v", observed, original)
	}
}

func TestCommitObserverPanicCannotChangeCommittedOutcome(t *testing.T) {
	consumer := &Consumer{logger: zap.NewNop()}
	consumer.SetCommitObserver(func([]segmentKafka.Message) { panic("metric bug") })
	consumer.notifyCommitObserver([]segmentKafka.Message{{Offset: 1}})
}

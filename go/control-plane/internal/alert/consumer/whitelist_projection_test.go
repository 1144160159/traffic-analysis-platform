package consumer

import (
	"context"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type fixedWhitelistMatcher struct {
	matched bool
	err     error
	calls   int
}

func (m *fixedWhitelistMatcher) MatchDetection(_ context.Context, _, _, _, _ string) (bool, error) {
	m.calls++
	return m.matched, m.err
}

func TestAppliedWhitelistProjectionStopsDetectionBeforeDedupAndStorage(t *testing.T) {
	behavior := validDetectionBehavior()
	payload, err := proto.Marshal(&pb.DetectionBatch{TenantId: "tenant-a", Behaviors: []*pb.DetectionBehavior{behavior}})
	if err != nil {
		t.Fatal(err)
	}
	matcher := &fixedWhitelistMatcher{matched: true}
	consumer := &Consumer{logger: zap.NewNop(), whitelistMatcher: matcher, timeBucket: 5}
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{Value: payload}}
	alert, evidence, err := consumer.processMessage(context.Background(), message)
	if err != nil {
		t.Fatal(err)
	}
	if matcher.calls != 1 || alert != nil || evidence != nil {
		t.Fatalf("calls=%d alert=%v evidence=%v", matcher.calls, alert, evidence)
	}
	// redisDedup is deliberately nil. Reaching dedup would panic, proving the
	// applied whitelist projection is the first side-effect barrier.
}

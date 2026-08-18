package publisher

import (
	"context"
	"sync/atomic"
	"testing"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"go.uber.org/zap"
)

func TestPublishCompensationFailsClosedWithoutWritingKafka(t *testing.T) {
	publisher := &KafkaPublisher{logger: zap.NewNop()}
	err := publisher.PublishCompensation(context.Background(), "rule-1", "tenant-1", "update", "operator-1", 7)
	if commonerrors.GetCode(err) != commonerrors.ErrCodeInvalidStateTransition {
		t.Fatalf("PublishCompensation() error = %v, code = %s", err, commonerrors.GetCode(err))
	}
	if got := atomic.LoadInt64(&publisher.metrics.RuleCompensations); got != 1 {
		t.Fatalf("rejected compensation metric = %d, want 1", got)
	}
}

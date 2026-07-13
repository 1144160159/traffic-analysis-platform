package consumer

import (
	"testing"

	"go.uber.org/zap"
)

func TestCloseSignalsStopAndIsIdempotent(t *testing.T) {
	consumer := &Consumer{
		logger:   zap.NewNop(),
		stopChan: make(chan struct{}),
	}

	if err := consumer.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	select {
	case <-consumer.stopChan:
	default:
		t.Fatal("Close() did not signal the consumer to stop")
	}

	if err := consumer.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

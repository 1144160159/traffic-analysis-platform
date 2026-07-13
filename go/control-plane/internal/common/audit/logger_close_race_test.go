package audit

import (
	"context"
	"sync"
	"testing"

	"go.uber.org/zap"
)

func TestLogConcurrentWithCloseDoesNotPanic(t *testing.T) {
	logger, err := NewLogger(Config{
		KafkaBrokers:  []string{"127.0.0.1:1"},
		Topic:         "audit.test",
		ServiceName:   "audit-test",
		BufferSize:    256,
		BackupEnabled: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	start := make(chan struct{})
	var writers sync.WaitGroup
	for index := 0; index < 64; index++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			for attempt := 0; attempt < 64; attempt++ {
				logger.Log(context.Background(), &AuditEvent{EventType: EventTypeDataIngested})
			}
		}()
	}
	close(start)
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	writers.Wait()
}

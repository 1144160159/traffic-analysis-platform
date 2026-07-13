package audit

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestLogAlertActionWithDetailRecordsReason(t *testing.T) {
	logger := &Logger{
		logger:      zap.NewNop(),
		serviceName: "alert-service-test",
		buffer:      make(chan *AuditEvent, 1),
	}

	logger.LogAlertActionWithDetail(
		context.Background(),
		EventTypeAlertTriage,
		"tenant-a",
		"user-a",
		"AL-1",
		"triage",
		"assigned",
		map[string]interface{}{"reason": "确认责任人接手处置"},
	)

	event := <-logger.buffer
	if event.TenantID != "tenant-a" || event.UserID != "user-a" || event.ResourceID != "AL-1" {
		t.Fatalf("unexpected audit identity: %#v", event)
	}
	if got := event.Detail["reason"]; got != "确认责任人接手处置" {
		t.Fatalf("reason detail=%v want=%s", got, "确认责任人接手处置")
	}
	if event.OldValue == nil || event.NewValue == nil {
		t.Fatalf("expected old/new status values: %#v", event)
	}
}

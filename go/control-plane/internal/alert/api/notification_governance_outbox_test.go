package api

import (
	"context"
	"errors"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

type fakeNotificationGovernanceProducer struct {
	err  error
	keys []string
}

func (p *fakeNotificationGovernanceProducer) SendJSON(_ context.Context, key string, _ interface{}, _ ...commonkafka.MessageHeader) error {
	p.keys = append(p.keys, key)
	return p.err
}

func notificationGovernanceOutboxRows(payload string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"outbox_id", "event_id", "aggregate_type", "aggregate_id", "aggregate_version",
		"tenant_id", "event_type", "schema_version", "partition_key", "trace_id", "payload",
	}).AddRow(
		int64(9), "00000000-0000-0000-0000-000000000209", "notification_rule",
		"00000000-0000-0000-0000-000000000109", int64(3), "tenant-a",
		"traffic.notification.rule.v1.RuleUpdated", 1,
		"tenant-a:00000000-0000-0000-0000-000000000109", "trace-a", payload,
	)
}

func newNotificationGovernanceOutboxTestHandler(t *testing.T, producer *fakeNotificationGovernanceProducer) (*AdvancedHandler, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	handler := NewAdvancedHandler(nil, nil, nil, nil, NewAdvancedRepository(db, zap.NewNop()))
	handler.SetNotificationGovernanceEventProducer(producer)
	return handler, mock
}

func TestNotificationGovernanceOutboxMarksPublishedOnlyAfterKafkaAcknowledgement(t *testing.T) {
	producer := &fakeNotificationGovernanceProducer{}
	handler, mock := newNotificationGovernanceOutboxTestHandler(t, producer)
	mock.ExpectQuery("WITH candidates AS").
		WithArgs(50, "worker-a", notificationGovernanceOutboxMaxAttempts).
		WillReturnRows(notificationGovernanceOutboxRows(`{"event_id":"00000000-0000-0000-0000-000000000209"}`))
	mock.ExpectExec("UPDATE notification_governance_outbox").
		WithArgs(int64(9), "worker-a").
		WillReturnResult(sqlmock.NewResult(0, 1))
	processed, err := handler.drainNotificationGovernanceOutbox(context.Background(), "worker-a", 50)
	if err != nil || processed != 1 || len(producer.keys) != 1 {
		t.Fatalf("processed=%d publishes=%d err=%v", processed, len(producer.keys), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationGovernanceOutboxFailureReleasesLeaseForBoundedRetry(t *testing.T) {
	producer := &fakeNotificationGovernanceProducer{err: errors.New("broker unavailable")}
	handler, mock := newNotificationGovernanceOutboxTestHandler(t, producer)
	mock.ExpectQuery("WITH candidates AS").
		WithArgs(50, "worker-b", notificationGovernanceOutboxMaxAttempts).
		WillReturnRows(notificationGovernanceOutboxRows(`{"event_id":"00000000-0000-0000-0000-000000000209"}`))
	mock.ExpectExec("UPDATE notification_governance_outbox").
		WithArgs(int64(9), "broker unavailable", "worker-b", notificationGovernanceOutboxMaxAttempts).
		WillReturnResult(sqlmock.NewResult(0, 1))
	processed, err := handler.drainNotificationGovernanceOutbox(context.Background(), "worker-b", 50)
	if err != nil || processed != 0 || len(producer.keys) != 1 {
		t.Fatalf("processed=%d publishes=%d err=%v", processed, len(producer.keys), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationGovernanceOutboxKafkaAckBeforeMarkAllowsIdempotentDuplicate(t *testing.T) {
	producer := &fakeNotificationGovernanceProducer{}
	handler, mock := newNotificationGovernanceOutboxTestHandler(t, producer)
	mock.ExpectExec("UPDATE notification_governance_outbox").
		WithArgs(int64(9), "worker-c").
		WillReturnResult(sqlmock.NewResult(0, 0))
	item := notificationGovernanceOutboxItem{
		OutboxID: 9, EventID: "00000000-0000-0000-0000-000000000209", AggregateType: "notification_rule",
		AggregateID: "00000000-0000-0000-0000-000000000109", AggregateVersion: 3, TenantID: "tenant-a",
		EventType: "traffic.notification.rule.v1.RuleUpdated", SchemaVersion: 1,
		PartitionKey: "tenant-a:00000000-0000-0000-0000-000000000109", TraceID: "trace-a",
		Payload: []byte(`{"event_id":"00000000-0000-0000-0000-000000000209"}`),
	}
	err := handler.publishNotificationGovernanceOutboxItem(context.Background(), "worker-c", item)
	if err == nil || err.Error() != "notification governance outbox lease lost after Kafka acknowledgement" || len(producer.keys) != 1 {
		t.Fatalf("publishes=%d err=%v", len(producer.keys), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationGovernanceOutboxAcceptsTemplateLifecycle(t *testing.T) {
	producer := &fakeNotificationGovernanceProducer{}
	handler, mock := newNotificationGovernanceOutboxTestHandler(t, producer)
	mock.ExpectExec("UPDATE notification_governance_outbox").
		WithArgs(int64(10), "worker-template").
		WillReturnResult(sqlmock.NewResult(0, 1))
	item := notificationGovernanceOutboxItem{
		OutboxID: 10, EventID: "00000000-0000-0000-0000-000000000210", AggregateType: "notification_template",
		AggregateID: "00000000-0000-0000-0000-000000000110", AggregateVersion: 2, TenantID: "tenant-a",
		EventType: "traffic.notification.template.v1.TemplateUpdated", SchemaVersion: 1,
		PartitionKey: "tenant-a:00000000-0000-0000-0000-000000000110", TraceID: "trace-template",
		Payload: []byte(`{"event_id":"00000000-0000-0000-0000-000000000210","template":{}}`),
	}
	if err := handler.publishNotificationGovernanceOutboxItem(context.Background(), "worker-template", item); err != nil {
		t.Fatal(err)
	}
	if len(producer.keys) != 1 {
		t.Fatalf("template publishes=%d, want 1", len(producer.keys))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationGovernanceOutboxAcceptsSilenceAndSettingsLifecycle(t *testing.T) {
	tests := []struct {
		name          string
		aggregateType string
		eventType     string
		payload       string
	}{
		{"silence", "notification_silence_rule", "traffic.notification.silence.v1.SilenceUpdated", `{"silence_rule":{}}`},
		{"settings", "notification_settings", "traffic.notification.settings.v1.SettingsUpdated", `{"settings":{}}`},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			producer := &fakeNotificationGovernanceProducer{}
			handler, mock := newNotificationGovernanceOutboxTestHandler(t, producer)
			outboxID := int64(20 + index)
			workerID := "worker-" + test.name
			mock.ExpectExec("UPDATE notification_governance_outbox").
				WithArgs(outboxID, workerID).
				WillReturnResult(sqlmock.NewResult(0, 1))
			item := notificationGovernanceOutboxItem{
				OutboxID: outboxID, EventID: "00000000-0000-0000-0000-000000000220", AggregateType: test.aggregateType,
				AggregateID: "00000000-0000-0000-0000-000000000120", AggregateVersion: 2, TenantID: "tenant-a",
				EventType: test.eventType, SchemaVersion: 1,
				PartitionKey: "tenant-a:00000000-0000-0000-0000-000000000120", TraceID: "trace-" + test.name,
				Payload: []byte(test.payload),
			}
			if err := handler.publishNotificationGovernanceOutboxItem(context.Background(), workerID, item); err != nil {
				t.Fatal(err)
			}
			if len(producer.keys) != 1 {
				t.Fatalf("publishes=%d, want 1", len(producer.keys))
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

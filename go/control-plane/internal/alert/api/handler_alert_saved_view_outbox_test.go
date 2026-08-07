package api

import (
	"context"
	"errors"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

type fakeSavedViewProducer struct {
	err     error
	keys    []string
	headers [][]kafka.MessageHeader
}

func (p *fakeSavedViewProducer) SendJSON(_ context.Context, key string, _ interface{}, headers ...kafka.MessageHeader) error {
	p.keys = append(p.keys, key)
	p.headers = append(p.headers, headers)
	return p.err
}

func savedViewOutboxRows(payload string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"outbox_id", "event_id", "aggregate_type", "aggregate_id", "aggregate_version",
		"tenant_id", "event_type", "schema_version", "partition_key", "trace_id", "payload",
	}).AddRow(
		int64(7), "00000000-0000-0000-0000-000000000207", "alert_saved_view",
		"00000000-0000-0000-0000-000000000107", int64(3), "tenant-a",
		"alert.saved-view.saved.v1", 1,
		"tenant-a:00000000-0000-0000-0000-000000000107", "trace-a", payload,
	)
}

func newSavedViewOutboxTestHandler(t *testing.T, producer *fakeSavedViewProducer) (*Handler, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	handler.SetSavedViewEventProducer(producer)
	return handler, mock
}

func TestSavedViewOutboxClaimsPublishesAndMarksAfterAcknowledgement(t *testing.T) {
	producer := &fakeSavedViewProducer{}
	handler, mock := newSavedViewOutboxTestHandler(t, producer)
	mock.ExpectQuery("WITH candidates AS").
		WithArgs(50, "worker-a", savedViewOutboxMaxAttempts).
		WillReturnRows(savedViewOutboxRows(`{"event_id":"00000000-0000-0000-0000-000000000207"}`))
	mock.ExpectExec("UPDATE alert_saved_view_outbox").
		WithArgs(int64(7), "worker-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := handler.drainSavedViewOutbox(context.Background(), "worker-a", 50)
	if err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	if len(producer.keys) != 1 || producer.keys[0] != "tenant-a:00000000-0000-0000-0000-000000000107" {
		t.Fatalf("unexpected published keys: %#v", producer.keys)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSavedViewOutboxPublishFailureReleasesLeaseWithBoundedRetry(t *testing.T) {
	producer := &fakeSavedViewProducer{err: errors.New("broker unavailable")}
	handler, mock := newSavedViewOutboxTestHandler(t, producer)
	mock.ExpectQuery("WITH candidates AS").
		WithArgs(50, "worker-b", savedViewOutboxMaxAttempts).
		WillReturnRows(savedViewOutboxRows(`{"event_id":"00000000-0000-0000-0000-000000000207"}`))
	mock.ExpectExec("UPDATE alert_saved_view_outbox").
		WithArgs(int64(7), "broker unavailable", "worker-b", savedViewOutboxMaxAttempts).
		WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := handler.drainSavedViewOutbox(context.Background(), "worker-b", 50)
	if err != nil || processed != 0 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	if len(producer.keys) != 1 {
		t.Fatalf("publish attempts=%d, want 1", len(producer.keys))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSavedViewOutboxInvalidPayloadMovesTowardDeadWithoutPublishing(t *testing.T) {
	producer := &fakeSavedViewProducer{}
	handler, mock := newSavedViewOutboxTestHandler(t, producer)
	mock.ExpectQuery("WITH candidates AS").
		WithArgs(50, "worker-c", savedViewOutboxMaxAttempts).
		WillReturnRows(savedViewOutboxRows(`{"event_id":`))
	mock.ExpectExec("UPDATE alert_saved_view_outbox").
		WithArgs(int64(7), "invalid outbox JSON payload", "worker-c", savedViewOutboxMaxAttempts).
		WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := handler.drainSavedViewOutbox(context.Background(), "worker-c", 50)
	if err != nil || processed != 0 || len(producer.keys) != 0 {
		t.Fatalf("processed=%d publishes=%d err=%v", processed, len(producer.keys), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSavedViewOutboxKafkaAckBeforeMarkIsRetriableDuplicate(t *testing.T) {
	producer := &fakeSavedViewProducer{}
	handler, mock := newSavedViewOutboxTestHandler(t, producer)
	mock.ExpectExec("UPDATE alert_saved_view_outbox").
		WithArgs(int64(7), "worker-d").
		WillReturnResult(sqlmock.NewResult(0, 0))
	item := savedViewOutboxItem{
		OutboxID: 7, EventID: "00000000-0000-0000-0000-000000000207",
		AggregateType: "alert_saved_view", AggregateID: "00000000-0000-0000-0000-000000000107",
		AggregateVersion: 3, TenantID: "tenant-a", EventType: "alert.saved-view.saved.v1",
		SchemaVersion: 1, PartitionKey: "tenant-a:view-a", TraceID: "trace-a",
		Payload: []byte(`{"event_id":"00000000-0000-0000-0000-000000000207"}`),
	}
	err := handler.publishSavedViewOutboxItem(context.Background(), "worker-d", item)
	if err == nil || err.Error() != "saved-view outbox lease lost after Kafka acknowledgement" {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(producer.keys) != 1 {
		t.Fatalf("Kafka must have acknowledged before mark failure; publishes=%d", len(producer.keys))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

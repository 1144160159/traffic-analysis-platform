package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestCanonicalProbeCommandJSONIsObjectOrderIndependent(t *testing.T) {
	left, err := canonicalProbeCommandJSON(map[string]interface{}{
		"b": []interface{}{2, 1},
		"a": map[string]interface{}{"z": true, "x": "v"},
	})
	if err != nil {
		t.Fatal(err)
	}
	right, err := canonicalProbeCommandJSON(json.RawMessage(
		`{"a":{"x":"v","z":true},"b":[2,1]}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(left) != sha256.Sum256(right) {
		t.Fatalf("canonical hashes differ: %s != %s", left, right)
	}
}

func probeOutboxRows(eventID, operationID, eventType string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"event_id", "operation_id", "tenant_id", "event_type",
		"partition_key", "aggregate_version", "schema_version", "payload",
	}).AddRow(
		eventID, operationID, "tenant-a", eventType,
		"tenant-a:probe-a", int64(3), 2,
		`{"event_id":"`+eventID+`","probe_id":"probe-a","operation_id":"`+operationID+`"}`,
	)
}

func TestDrainProbeOperationOutboxPublishesThenMarksDelivered(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	eventID := "11111111-1111-4111-8111-111111111111"
	operationID := "22222222-2222-4222-8222-222222222222"
	var published bool
	handler.probeCommandPublish = func(_ context.Context, key string, payload []byte, headers ...commonkafka.MessageHeader) error {
		published = true
		if key != "tenant-a:probe-a" {
			t.Fatalf("unexpected key %q", key)
		}
		if len(payload) == 0 {
			t.Fatal("payload is empty")
		}
		assertProbeHeader(t, headers, "event_id", eventID)
		assertProbeHeader(t, headers, "schema_version", "2")
		assertProbeHeader(t, headers, "target_topic", "probe.control.v2")
		return nil
	}
	handler.probeEventPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) error {
		t.Fatal("lifecycle publisher must not receive requested command")
		return nil
	}

	mock.ExpectExec(regexp.QuoteMeta("WITH expired AS (")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(50, "worker-1").
		WillReturnRows(probeOutboxRows(eventID, operationID, probeOperationRequestedEvent))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE probe_operation_outbox").
		WithArgs(eventID, "worker-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("UPDATE probe_operations").
		WithArgs(operationID, "tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{"state_revision"}).AddRow(int64(2)))
	mock.ExpectExec("INSERT INTO probe_operation_history").
		WithArgs(operationID, "tenant-a", int64(2), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	processed, err := handler.drainProbeOperationOutbox(context.Background(), "worker-1", 50)
	if err != nil {
		t.Fatalf("drainProbeOperationOutbox() error = %v", err)
	}
	if processed != 1 || !published {
		t.Fatalf("processed=%d published=%v, want 1/true", processed, published)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDrainProbeOperationOutboxFailureRemainsPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	eventID := "33333333-3333-4333-8333-333333333333"
	operationID := "44444444-4444-4444-8444-444444444444"
	handler.probeCommandPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) error {
		return errors.New("kafka unavailable")
	}
	handler.probeEventPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) error {
		return nil
	}

	mock.ExpectExec(regexp.QuoteMeta("WITH expired AS (")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(50, "worker-2").
		WillReturnRows(probeOutboxRows(eventID, operationID, probeOperationRequestedEvent))
	mock.ExpectExec("UPDATE probe_operation_outbox").
		WithArgs(eventID, "kafka unavailable", "worker-2").
		WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := handler.drainProbeOperationOutbox(context.Background(), "worker-2", 50)
	if err != nil {
		t.Fatalf("drainProbeOperationOutbox() error = %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed=%d, want 0", processed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDrainProbeOperationOutboxRoutesAcknowledgementToEventTopic(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	eventID := "55555555-5555-4555-8555-555555555555"
	operationID := "66666666-6666-4666-8666-666666666666"
	handler.probeCommandPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) error {
		t.Fatal("command publisher must not receive acknowledgement event")
		return nil
	}
	handler.probeEventPublish = func(_ context.Context, _ string, _ []byte, headers ...commonkafka.MessageHeader) error {
		assertProbeHeader(t, headers, "schema_version", "2")
		assertProbeHeader(t, headers, "target_topic", "probe.events.v2")
		return nil
	}

	mock.ExpectExec(regexp.QuoteMeta("WITH expired AS (")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(50, "worker-3").
		WillReturnRows(probeOutboxRows(eventID, operationID, probeOperationAcknowledgedEvent))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE probe_operation_outbox").
		WithArgs(eventID, "worker-3").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	processed, err := handler.drainProbeOperationOutbox(context.Background(), "worker-3", 50)
	if err != nil {
		t.Fatalf("drainProbeOperationOutbox() error = %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed=%d, want 1", processed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func assertProbeHeader(t *testing.T, headers []commonkafka.MessageHeader, key, want string) {
	t.Helper()
	for _, header := range headers {
		if header.Key == key {
			if header.Value != want {
				t.Fatalf("header %s=%q, want %q", key, header.Value, want)
			}
			return
		}
	}
	t.Fatalf("missing header %s", key)
}

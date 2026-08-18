package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

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
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": eventType, "schema_version": 2,
		"tenant_id": "tenant-a", "probe_id": "probe-a", "operation_id": operationID,
		"command_revision": int64(3),
	}
	if eventType == probeOperationAcknowledgedEvent || eventType == probeOperationExpiredEvent {
		payload["revision"] = int64(3)
	}
	rawPayload, _ := json.Marshal(payload)
	return sqlmock.NewRows([]string{
		"event_id", "operation_id", "tenant_id", "event_type",
		"partition_key", "aggregate_version", "schema_version", "publish_attempt", "payload",
	}).AddRow(
		eventID, operationID, "tenant-a", eventType,
		"tenant-a:probe-a", int64(3), 2, probePublishAttemptID,
		string(rawPayload),
	)
}

const probePublishAttemptID = "99999999-9999-4999-8999-999999999999"

func probeBrokerReceipt(topic, key string) commonkafka.BrokerReceipt {
	return commonkafka.BrokerReceipt{
		AttemptID: probePublishAttemptID, Topic: topic, Partition: 2, Offset: 17,
		Key: key, AcknowledgedAt: time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC),
	}
}

func expectProbeExpiryPass(mock sqlmock.Sqlmock, limit int) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(limit).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
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
	handler.probeCommandPublish = func(_ context.Context, key string, payload []byte, headers ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
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
		return probeBrokerReceipt("probe.control.v2", key), nil
	}
	handler.probeEventPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
		t.Fatal("lifecycle publisher must not receive requested command")
		return commonkafka.BrokerReceipt{}, nil
	}

	expectProbeExpiryPass(mock, 50)
	mock.ExpectQuery(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(50, "worker-1").
		WillReturnRows(probeOutboxRows(eventID, operationID, probeOperationRequestedEvent))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE probe_operation_outbox").
		WithArgs(eventID, "worker-1", "probe.control.v2", 2, int64(17), sqlmock.AnyArg(), probePublishAttemptID).
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
	handler.probeCommandPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
		return commonkafka.BrokerReceipt{}, errors.New("kafka unavailable")
	}
	handler.probeEventPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
		return commonkafka.BrokerReceipt{}, nil
	}

	expectProbeExpiryPass(mock, 50)
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
	handler.probeCommandPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
		t.Fatal("command publisher must not receive acknowledgement event")
		return commonkafka.BrokerReceipt{}, nil
	}
	handler.probeEventPublish = func(_ context.Context, key string, _ []byte, headers ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
		assertProbeHeader(t, headers, "schema_version", "2")
		assertProbeHeader(t, headers, "target_topic", "probe.events.v2")
		return probeBrokerReceipt("probe.events.v2", key), nil
	}

	expectProbeExpiryPass(mock, 50)
	mock.ExpectQuery(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(50, "worker-3").
		WillReturnRows(probeOutboxRows(eventID, operationID, probeOperationAcknowledgedEvent))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE probe_operation_outbox").
		WithArgs(eventID, "worker-3", "probe.events.v2", 2, int64(17), sqlmock.AnyArg(), probePublishAttemptID).
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

func TestProbeOperationExpiredDispatcherWhitelist(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	eventID := "77777777-7777-4777-8777-777777777777"
	operationID := "88888888-8888-4888-8888-888888888888"
	handler.probeCommandPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
		t.Fatal("expiry must not enter the command publisher")
		return commonkafka.BrokerReceipt{}, nil
	}
	handler.probeEventPublish = func(_ context.Context, key string, _ []byte, headers ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
		if key != "tenant-a:probe-a" {
			t.Fatalf("key=%q", key)
		}
		assertProbeHeader(t, headers, "event_type", probeOperationExpiredEvent)
		assertProbeHeader(t, headers, "target_topic", "probe.events.v2")
		return probeBrokerReceipt("probe.events.v2", key), nil
	}

	expectProbeExpiryPass(mock, 50)
	mock.ExpectQuery(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(50, "worker-expiry").
		WillReturnRows(probeOutboxRows(eventID, operationID, probeOperationExpiredEvent))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE probe_operation_outbox").
		WithArgs(eventID, "worker-expiry", "probe.events.v2", 2, int64(17), sqlmock.AnyArg(), probePublishAttemptID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	processed, err := handler.drainProbeOperationOutbox(context.Background(), "worker-expiry", 50)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed=%d, want 1", processed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProbeOperationEventTopicFailsClosed(t *testing.T) {
	if _, err := probeOperationEventTopic("traffic.probe.v2.OperationDeleted"); err == nil {
		t.Fatal("unknown event type was routed")
	}
}

func TestPublishProbeOperationOutboxReceiptMatrix(t *testing.T) {
	eventID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	operationID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	payload, err := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": probeOperationAcknowledgedEvent,
		"schema_version": 2, "tenant_id": "tenant-a", "probe_id": "probe-a",
		"operation_id": operationID, "command_revision": int64(7), "revision": int64(4),
	})
	if err != nil {
		t.Fatal(err)
	}
	item := probeOperationOutboxItem{
		EventID: eventID, OperationID: operationID, TenantID: "tenant-a",
		EventType: probeOperationAcknowledgedEvent, PartitionKey: "tenant-a:probe-a",
		AggregateVersion: 4, SchemaVersion: 2, PublishAttempt: probePublishAttemptID,
		Payload: payload,
	}
	receipt := probeBrokerReceipt("probe.events.v2", item.PartitionKey)

	t.Run("exact broker receipt becomes durable", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		handler := NewSystemHandler(nil, db, zap.NewNop())
		handler.probeEventPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
			return receipt, nil
		}
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE probe_operation_outbox").
			WithArgs(eventID, "worker", receipt.Topic, receipt.Partition, receipt.Offset,
				receipt.AcknowledgedAt, receipt.AttemptID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		if err := handler.publishProbeOperationOutboxItem(context.Background(), "worker", item); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("broker acknowledged but database begin fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		handler := NewSystemHandler(nil, db, zap.NewNop())
		handler.probeEventPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
			return receipt, nil
		}
		mock.ExpectBegin().WillReturnError(errors.New("postgres unavailable"))
		expectProbeOutcomeUnknown(mock, item, "worker", receipt)
		if err := handler.publishProbeOperationOutboxItem(context.Background(), "worker", item); err == nil {
			t.Fatal("database receipt failure was reported successful")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("broker completion identity mismatch", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		handler := NewSystemHandler(nil, db, zap.NewNop())
		mismatch := receipt
		mismatch.AttemptID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		handler.probeEventPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
			return mismatch, nil
		}
		expectProbeOutcomeUnknown(mock, item, "worker", mismatch)
		if err := handler.publishProbeOperationOutboxItem(context.Background(), "worker", item); err == nil {
			t.Fatal("mismatched receipt was reported successful")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unknown broker outcome remains explicit", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		handler := NewSystemHandler(nil, db, zap.NewNop())
		unknown := &commonkafka.PublishOutcomeUnknownError{
			Receipt: receipt, Cause: context.DeadlineExceeded,
		}
		handler.probeEventPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
			return receipt, unknown
		}
		expectProbeOutcomeUnknown(mock, item, "worker", receipt)
		if err := handler.publishProbeOperationOutboxItem(context.Background(), "worker", item); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err=%v, want deadline cause", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func expectProbeOutcomeUnknown(
	mock sqlmock.Sqlmock,
	item probeOperationOutboxItem,
	workerID string,
	receipt commonkafka.BrokerReceipt,
) {
	mock.ExpectExec("UPDATE probe_operation_outbox").
		WithArgs(
			item.EventID, workerID, item.PublishAttempt,
			sqlmock.AnyArg(), receipt.Topic, receipt.Partition, receipt.Offset,
			receipt.AcknowledgedAt,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
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

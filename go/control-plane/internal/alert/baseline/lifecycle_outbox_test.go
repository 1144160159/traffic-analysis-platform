package baseline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
)

type lifecyclePublisherStub struct {
	receipt commonkafka.BrokerReceipt
	err     error
	called  bool
	headers map[string]string
}

func (stub *lifecyclePublisherStub) Send(
	_ context.Context,
	key string,
	_ []byte,
	headers ...commonkafka.MessageHeader,
) (commonkafka.BrokerReceipt, error) {
	stub.called = true
	stub.headers = make(map[string]string, len(headers))
	for _, header := range headers {
		stub.headers[header.Key] = header.Value
	}
	stub.receipt.Key = key
	return stub.receipt, stub.err
}

func TestLifecycleOutboxPublishRecordsExactBrokerAcknowledgement(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item, candidate := lifecycleActivationTestItem(t)
	acknowledgedAt := time.Unix(8000, 0).UTC()
	publisher := &lifecyclePublisherStub{receipt: commonkafka.BrokerReceipt{
		AttemptID: item.ClaimToken, Topic: LifecycleTopic, Partition: 1, Offset: 27,
		AcknowledgedAt: acknowledgedAt,
	}}
	dispatcher := &LifecycleOutboxDispatcher{db: db, publisher: publisher, candidateSHA256: candidate}
	mock.ExpectExec(`UPDATE behavior_baseline_lifecycle_outbox_v1`).
		WithArgs(LifecycleTopic, 1, int64(27), acknowledgedAt, item.EventID, item.ClaimToken).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := dispatcher.publish(context.Background(), item); err != nil {
		t.Fatalf("publish lifecycle event: %v", err)
	}
	if !publisher.called {
		t.Fatal("expected Kafka publisher call")
	}
	if publisher.headers["candidate_sha256"] != candidate || publisher.headers["snapshot_sha256"] == "" {
		t.Fatalf("candidate or snapshot identity missing from lifecycle headers: %#v", publisher.headers)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleOutboxAmbiguousPublishStaysOutcomeUnknown(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item, candidate := lifecycleActivationTestItem(t)
	publisher := &lifecyclePublisherStub{err: &commonkafka.PublishOutcomeUnknownError{
		Receipt: commonkafka.BrokerReceipt{AttemptID: item.ClaimToken, Topic: LifecycleTopic},
		Cause:   errors.New("broker acknowledgement timed out"),
	}}
	dispatcher := &LifecycleOutboxDispatcher{db: db, publisher: publisher, candidateSHA256: candidate}
	if err := dispatcher.publish(context.Background(), item); err == nil {
		t.Fatal("expected outcome-unknown error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ambiguous outcome must not be reset or falsely acknowledged: %v", err)
	}
}

func TestLifecycleOutboxRejectsCandidateMismatchBeforeKafka(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item, _ := lifecycleActivationTestItem(t)
	publisher := &lifecyclePublisherStub{}
	dispatcher := &LifecycleOutboxDispatcher{db: db, publisher: publisher,
		candidateSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	mock.ExpectExec(`UPDATE behavior_baseline_lifecycle_outbox_v1`).
		WithArgs("INVALID_OUTBOX_PAYLOAD", item.EventID, item.ClaimToken).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := dispatcher.publish(context.Background(), item); err == nil {
		t.Fatal("expected candidate mismatch")
	}
	if publisher.called {
		t.Fatal("candidate mismatch must fail before Kafka")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleOutboxDrainClaimsOnlyTheEarliestUnackedPartitionSequence(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, candidate := lifecycleActivationTestItem(t)
	now := time.Unix(9000, 0).UTC()
	readiness := &AckReadinessStore{db: db, now: func() time.Time { return now }}
	dispatcher := &LifecycleOutboxDispatcher{db: db, publisher: &lifecyclePublisherStub{},
		readiness: readiness, candidateSHA256: candidate, now: func() time.Time { return now }}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT observed_topic,candidate_sha256,state,lease_expires_at`).
		WithArgs(AckPipelineID, ActivationAckGroup).
		WillReturnRows(sqlmock.NewRows([]string{"topic", "candidate", "state", "expires"}).
			AddRow(ActivationAckTopic, candidate, "READY", now.Add(time.Minute)))
	mock.ExpectQuery(`prior\.outbox_sequence<o\.outbox_sequence`).WithArgs(50).
		WillReturnRows(sqlmock.NewRows([]string{"outbox_sequence", "event_id", "tenant_id", "baseline_id",
			"aggregate_type", "aggregate_id", "aggregate_version", "event_type", "partition_key", "payload",
			"payload_sha256", "trace_id", "claim_token"}))
	mock.ExpectCommit()
	count, err := dispatcher.Drain(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("empty ordered claim returned %d publications", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func lifecycleActivationTestItem(t *testing.T) (lifecycleOutboxItem, string) {
	t.Helper()
	candidate := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	eventID := "00000000-0000-4000-8000-000000000501"
	payload, err := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": "baseline.activation.requested.v1", "schema_version": 1,
		"partition_key": "tenant-a:user:user-a", "tenant_id": "tenant-a", "baseline_id": "user:user-a",
		"baseline_version": 3, "definition_revision": 4,
		"baseline_kind": "dynamic", "algorithm_version": "zscore-v1",
		"threshold_spec":     map[string]interface{}{"warning": 2},
		"statistics":         map[string]interface{}{"metric": map[string]interface{}{"mean": 1}},
		"candidate_sha256":   candidate,
		"snapshot_sha256":    "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		"approval_id":        "00000000-0000-4000-8000-000000000502",
		"expected_consumers": []string{"flink-user-behavior-job"}, "trace_id": "trace-baseline-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(payload)
	return lifecycleOutboxItem{
		EventID: eventID, TenantID: "tenant-a", BaselineID: "user:user-a", AggregateType: "baseline_version",
		AggregateID: "00000000-0000-4000-8000-000000000503", AggregateVersion: 3,
		EventType: "baseline.activation.requested.v1", PartitionKey: "tenant-a:user:user-a", Payload: payload,
		PayloadSHA256: hex.EncodeToString(hash[:]), TraceID: "trace-baseline-1",
		ClaimToken: "00000000-0000-4000-8000-000000000504",
	}, candidate
}

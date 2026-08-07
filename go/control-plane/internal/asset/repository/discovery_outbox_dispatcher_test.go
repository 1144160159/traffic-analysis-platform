package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
)

type recordingDiscoveryPublisher struct {
	key     string
	payload []byte
	headers map[string]string
	err     error
}

func (p *recordingDiscoveryPublisher) Send(
	_ context.Context,
	key string,
	payload []byte,
	headers ...kafkaCommon.MessageHeader,
) error {
	p.key = key
	p.payload = append([]byte(nil), payload...)
	p.headers = make(map[string]string, len(headers))
	for _, header := range headers {
		p.headers[header.Key] = header.Value
	}
	return p.err
}

func discoveryOutboxRows(payload []byte, attempt int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"outbox_id", "event_id", "run_id", "resource_type", "resource_id", "action_id",
		"tenant_id", "aggregate_version",
		"schema_version", "partition_key", "event_type", "payload", "attempt_count",
	}).AddRow(
		17, "11111111-1111-4111-8111-111111111111", "run-a", "run", "run-a",
		"asset-active-discovery-run", "tenant-a", 2, 1,
		"tenant-a:run-a", "traffic.asset.discovery.v1.JobStateChanged", payload, attempt,
	)
}

func TestDiscoveryOutboxDispatcherPublishesStableEnvelope(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	publisher := &recordingDiscoveryPublisher{}
	dispatcher, err := NewDiscoveryOutboxDispatcher(db, publisher, OutboxDispatcherConfig{
		WorkerID: "discovery-worker-a", Lease: 30 * time.Second, MaxAttempts: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"traffic.asset.discovery.v1.JobStateChanged","schema_version":1,"aggregate_version":2,"partition_key":"tenant-a:run-a","tenant_id":"tenant-a","resource_type":"run","resource_id":"run-a","action_id":"asset-active-discovery-run","revision":2,"run_id":"run-a","trace_id":"trace-a","status":"succeeded"}`)
	mock.ExpectQuery("WITH candidate AS").
		WithArgs("discovery-worker-a", int64(30)).
		WillReturnRows(discoveryOutboxRows(payload, 1))
	mock.ExpectExec("UPDATE asset_discovery_outbox").
		WithArgs(int64(17), "discovery-worker-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	found, err := dispatcher.DispatchNext(context.Background())
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if publisher.key != "tenant-a:run-a" {
		t.Fatalf("partition key=%q", publisher.key)
	}
	for key, want := range map[string]string{
		"event_id":          "11111111-1111-4111-8111-111111111111",
		"event_type":        "traffic.asset.discovery.v1.JobStateChanged",
		"schema_version":    "1",
		"aggregate_version": "2",
		"tenant_id":         "tenant-a",
		"run_id":            "run-a",
		"trace_id":          "trace-a",
	} {
		if publisher.headers[key] != want {
			t.Fatalf("header %s=%q want=%q", key, publisher.headers[key], want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryOutboxDispatcherPublishFailureReturnsToPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	publisher := &recordingDiscoveryPublisher{err: errors.New("broker unavailable")}
	dispatcher, _ := NewDiscoveryOutboxDispatcher(db, publisher, OutboxDispatcherConfig{
		WorkerID: "discovery-worker-b", Lease: time.Second, MaxAttempts: 8,
	})
	payload := []byte(`{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"traffic.asset.discovery.v1.JobStateChanged","schema_version":1,"aggregate_version":2,"partition_key":"tenant-a:run-a","tenant_id":"tenant-a","resource_type":"run","resource_id":"run-a","action_id":"asset-active-discovery-run","revision":2,"run_id":"run-a","trace_id":"trace-a","status":"failed"}`)
	mock.ExpectQuery("WITH candidate AS").WillReturnRows(discoveryOutboxRows(payload, 2))
	mock.ExpectExec("UPDATE asset_discovery_outbox").
		WithArgs(int64(17), "discovery-worker-b", "pending", 4, "broker unavailable").
		WillReturnResult(sqlmock.NewResult(0, 1))

	found, err := dispatcher.DispatchNext(context.Background())
	if !found || err == nil {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryOutboxDispatcherRejectsEnvelopeCollision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	publisher := &recordingDiscoveryPublisher{}
	dispatcher, _ := NewDiscoveryOutboxDispatcher(db, publisher, OutboxDispatcherConfig{
		WorkerID: "discovery-worker-c", Lease: time.Second, MaxAttempts: 8,
	})
	payload := []byte(`{"event_id":"different","event_type":"traffic.asset.discovery.v1.JobStateChanged","schema_version":1,"aggregate_version":2,"partition_key":"tenant-a:run-a","tenant_id":"tenant-a","resource_type":"run","resource_id":"run-a","action_id":"asset-active-discovery-run","revision":2,"run_id":"run-a","trace_id":"trace-a","status":"failed"}`)
	mock.ExpectQuery("WITH candidate AS").WillReturnRows(discoveryOutboxRows(payload, 8))
	mock.ExpectExec("UPDATE asset_discovery_outbox").
		WithArgs(int64(17), "discovery-worker-c", "dead", 256, "discovery outbox envelope does not match authoritative columns").
		WillReturnResult(sqlmock.NewResult(0, 1))

	found, err := dispatcher.DispatchNext(context.Background())
	if !found || err == nil {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if len(publisher.payload) != 0 {
		t.Fatal("mismatched envelope must never publish")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

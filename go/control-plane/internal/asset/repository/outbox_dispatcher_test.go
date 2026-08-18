package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
)

type recordingAssetPublisher struct {
	key     string
	payload []byte
	headers map[string]string
	err     error
}

func (p *recordingAssetPublisher) Send(
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

func TestAssetOutboxDispatcherPublishesStableEnvelope(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	publisher := &recordingAssetPublisher{}
	dispatcher, err := NewAssetOutboxDispatcher(db, publisher, OutboxDispatcherConfig{
		WorkerID: "worker-a", Lease: 30 * time.Second, MaxAttempts: 8, TenantID: "tenant-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"traffic.asset.v2.AssetUpserted","schema_version":2,"aggregate_version":3,"partition_key":"tenant-a:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","tenant_id":"tenant-a","asset_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","revision":3,"trace_id":"trace-a","asset":{}}`)
	mock.ExpectQuery("WITH candidate AS").
		WithArgs("worker-a", int64(30), "tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{
			"outbox_id", "event_id", "tenant_id", "asset_id", "aggregate_version",
			"schema_version", "partition_key", "event_type", "payload", "attempt_count",
		}).AddRow(
			9, "11111111-1111-4111-8111-111111111111", "tenant-a",
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", 3, 2,
			"tenant-a:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			"traffic.asset.v2.AssetUpserted", payload, 1,
		))
	mock.ExpectExec("UPDATE asset_event_outbox").
		WithArgs(int64(9), "worker-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	found, err := dispatcher.DispatchNext(context.Background())
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if publisher.key != "tenant-a:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
		t.Fatalf("partition key=%q", publisher.key)
	}
	for key, want := range map[string]string{
		"event_id":          "11111111-1111-4111-8111-111111111111",
		"schema_version":    "2",
		"aggregate_version": "3",
		"tenant_id":         "tenant-a",
		"asset_id":          "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
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

func TestAssetOutboxDispatcherPublishFailureReturnsToPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	publisher := &recordingAssetPublisher{err: errors.New("broker unavailable")}
	dispatcher, _ := NewAssetOutboxDispatcher(db, publisher, OutboxDispatcherConfig{
		WorkerID: "worker-b", Lease: time.Second, MaxAttempts: 8,
	})
	payload := []byte(`{"event_id":"22222222-2222-4222-8222-222222222222","event_type":"traffic.asset.v2.AssetUpserted","schema_version":2,"aggregate_version":1,"partition_key":"tenant-b:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","tenant_id":"tenant-b","asset_id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","revision":1,"trace_id":"trace-b"}`)
	mock.ExpectQuery("WITH candidate AS").
		WillReturnRows(sqlmock.NewRows([]string{
			"outbox_id", "event_id", "tenant_id", "asset_id", "aggregate_version",
			"schema_version", "partition_key", "event_type", "payload", "attempt_count",
		}).AddRow(
			10, "22222222-2222-4222-8222-222222222222", "tenant-b",
			"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", 1, 2,
			"tenant-b:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			"traffic.asset.v2.AssetUpserted", payload, 2,
		))
	mock.ExpectExec("UPDATE asset_event_outbox").
		WithArgs(int64(10), "worker-b", "pending", 4, "broker unavailable").
		WillReturnResult(sqlmock.NewResult(0, 1))
	found, err := dispatcher.DispatchNext(context.Background())
	if !found || err == nil {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAssetOutboxDispatcherEnvelopeCollisionGoesDead(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	publisher := &recordingAssetPublisher{}
	dispatcher, _ := NewAssetOutboxDispatcher(db, publisher, OutboxDispatcherConfig{
		WorkerID: "worker-c", Lease: time.Second, MaxAttempts: 8,
	})
	payload := []byte(`{"event_id":"different","event_type":"traffic.asset.v2.AssetUpserted","schema_version":2,"aggregate_version":1,"partition_key":"tenant-c:cccccccc-cccc-4ccc-8ccc-cccccccccccc","tenant_id":"tenant-c","asset_id":"cccccccc-cccc-4ccc-8ccc-cccccccccccc","revision":1}`)
	mock.ExpectQuery("WITH candidate AS").
		WillReturnRows(sqlmock.NewRows([]string{
			"outbox_id", "event_id", "tenant_id", "asset_id", "aggregate_version",
			"schema_version", "partition_key", "event_type", "payload", "attempt_count",
		}).AddRow(
			11, "33333333-3333-4333-8333-333333333333", "tenant-c",
			"cccccccc-cccc-4ccc-8ccc-cccccccccccc", 1, 2,
			"tenant-c:cccccccc-cccc-4ccc-8ccc-cccccccccccc",
			"traffic.asset.v2.AssetUpserted", payload, 8,
		))
	mock.ExpectExec("UPDATE asset_event_outbox").
		WithArgs(int64(11), "worker-c", "dead", 256, "asset outbox envelope does not match authoritative columns").
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

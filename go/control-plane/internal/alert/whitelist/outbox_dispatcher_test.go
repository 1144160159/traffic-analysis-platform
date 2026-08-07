package whitelist

import (
	"context"
	"errors"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
)

type fakeWhitelistPublisher struct {
	err     error
	keys    []string
	headers [][]commonkafka.MessageHeader
}

func (p *fakeWhitelistPublisher) Send(_ context.Context, key string, _ []byte, headers ...commonkafka.MessageHeader) error {
	p.keys = append(p.keys, key)
	p.headers = append(p.headers, headers)
	return p.err
}

func whitelistOutboxPayload() string {
	return `{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"traffic.whitelist.v2.EntryApproved","schema_version":2,"tenant_id":"tenant-a","entry_id":"22222222-2222-4222-8222-222222222222","aggregate_version":3,"action_id":"whitelist-approve","reason":"reviewed false positive","trace_id":"trace-a","desired_rule_state":"effective","occurred_at":"2026-08-07T10:00:00Z","entry":{"id":"22222222-2222-4222-8222-222222222222"}}`
}

func whitelistOutboxRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"outbox_id", "event_id", "tenant_id", "entry_id", "aggregate_version", "event_type",
		"schema_version", "partition_key", "trace_id", "payload", "attempt_count", "desired_state",
	}).AddRow(int64(7), "11111111-1111-4111-8111-111111111111", "tenant-a",
		"22222222-2222-4222-8222-222222222222", int64(3), "traffic.whitelist.v2.EntryApproved",
		2, "tenant-a:22222222-2222-4222-8222-222222222222", "trace-a", whitelistOutboxPayload(), 1, "effective")
}

func newWhitelistOutboxTestDispatcher(t *testing.T, publisher *fakeWhitelistPublisher) (*OutboxDispatcher, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	dispatcher, err := NewOutboxDispatcher(db, publisher, OutboxDispatcherConfig{WorkerID: "worker-a"})
	if err != nil {
		t.Fatal(err)
	}
	return dispatcher, mock
}

func TestWhitelistOutboxMarksPublishedOnlyAfterKafkaAck(t *testing.T) {
	publisher := &fakeWhitelistPublisher{}
	dispatcher, mock := newWhitelistOutboxTestDispatcher(t, publisher)
	mock.ExpectQuery("WITH candidate AS").WithArgs("worker-a", int64(30)).WillReturnRows(whitelistOutboxRows())
	mock.ExpectExec("UPDATE whitelist_event_outbox").WithArgs(int64(7), "worker-a").
		WillReturnResult(sqlmock.NewResult(0, 1))
	found, err := dispatcher.DispatchNext(context.Background())
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if len(publisher.keys) != 1 || publisher.keys[0] != "tenant-a:22222222-2222-4222-8222-222222222222" {
		t.Fatalf("unexpected publishes: %#v", publisher.keys)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWhitelistOutboxBrokerFailureNeverMarksPublished(t *testing.T) {
	publisher := &fakeWhitelistPublisher{err: errors.New("broker unavailable")}
	dispatcher, mock := newWhitelistOutboxTestDispatcher(t, publisher)
	mock.ExpectQuery("WITH candidate AS").WithArgs("worker-a", int64(30)).WillReturnRows(whitelistOutboxRows())
	mock.ExpectExec("UPDATE whitelist_event_outbox").
		WithArgs(int64(7), "worker-a", "pending", 2, "broker unavailable").
		WillReturnResult(sqlmock.NewResult(0, 1))
	found, err := dispatcher.DispatchNext(context.Background())
	if !found || err == nil || err.Error() != "broker unavailable" {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWhitelistOutboxRejectsAuthoritativeIdentityMismatch(t *testing.T) {
	publisher := &fakeWhitelistPublisher{}
	dispatcher, mock := newWhitelistOutboxTestDispatcher(t, publisher)
	rows := whitelistOutboxRows()
	// The fixture payload says tenant-a, while the authoritative row says tenant-b.
	rows = sqlmock.NewRows([]string{
		"outbox_id", "event_id", "tenant_id", "entry_id", "aggregate_version", "event_type",
		"schema_version", "partition_key", "trace_id", "payload", "attempt_count", "desired_state",
	}).AddRow(int64(7), "11111111-1111-4111-8111-111111111111", "tenant-b",
		"22222222-2222-4222-8222-222222222222", int64(3), "traffic.whitelist.v2.EntryApproved",
		2, "tenant-b:22222222-2222-4222-8222-222222222222", "trace-a", whitelistOutboxPayload(), 1, "effective")
	mock.ExpectQuery("WITH candidate AS").WithArgs("worker-a", int64(30)).WillReturnRows(rows)
	mock.ExpectExec("UPDATE whitelist_event_outbox").
		WithArgs(int64(7), "worker-a", "pending", 2, "whitelist outbox envelope does not match authoritative columns").
		WillReturnResult(sqlmock.NewResult(0, 1))
	found, err := dispatcher.DispatchNext(context.Background())
	if !found || err == nil || len(publisher.keys) != 0 {
		t.Fatalf("found=%v publishes=%d err=%v", found, len(publisher.keys), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

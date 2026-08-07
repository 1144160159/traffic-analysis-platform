package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/threatintel"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestCommitThreatIntelCommandCommitsRevisionHistoryAuditOutboxAndRequestTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := newTransactionalThreatIntelTestServer(db)
	command := transactionalThreatIntelCommandFixture()
	eventID := deterministicThreatIntelEventID(command.Event.TenantID, command.Meta.IdempotencyKey)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT request_sha256,response_payload::text").
		WithArgs(command.Event.TenantID, command.Meta.IdempotencyKey).
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "response_payload"}))
	mock.ExpectQuery("SELECT revision FROM threat_intel").
		WithArgs("tenant-a", "ip", "203.0.113.9").
		WillReturnRows(sqlmock.NewRows([]string{"revision"}))
	mock.ExpectExec("INSERT INTO threat_intel").
		WithArgs(
			"tenant-a", "ip", "203.0.113.9", threatintel.RepMalicious,
			"c2", "manual", "test indicator", command.Entries[0].LastSeen, int64(1),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO threat_intel_event_outbox").
		WithArgs(eventID, "tenant-a", "tenant-a", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO audit_logs").
		WithArgs(
			"audit-"+eventID, "tenant-a", "", "THREAT_INTEL_ENTRY_UPSERTED",
			"threat_intel", "ip:203.0.113.9", sqlmock.AnyArg(), "", "",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO threat_intel_command_history").
		WithArgs(
			eventID, "tenant-a", "entry", "ip:203.0.113.9", int64(1),
			command.Meta.ActionID, command.Meta.CommandType, command.Meta.Reason,
			command.Meta.TraceID, false, sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO threat_intel_command_requests").
		WithArgs(
			"tenant-a", command.Meta.IdempotencyKey, command.Meta.RequestSHA256,
			command.Meta.ActionID, command.Meta.CommandType, int64(0), int64(1), eventID,
			command.Meta.Reason, command.Meta.TraceID, false, sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	receipt, err := srv.commitThreatIntelCommand(context.Background(), nil, command)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.EventID != eventID || receipt.AggregateVersion != 1 || receipt.Replayed {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	if len(receipt.Entries) != 1 || receipt.Entries[0].Revision != 1 {
		t.Fatalf("entry revision not returned: %+v", receipt.Entries)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCommitThreatIntelCommandRollsBackWhenOutboxInsertFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := newTransactionalThreatIntelTestServer(db)
	command := transactionalThreatIntelCommandFixture()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT request_sha256,response_payload::text").
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "response_payload"}))
	mock.ExpectQuery("SELECT revision FROM threat_intel").
		WillReturnRows(sqlmock.NewRows([]string{"revision"}))
	mock.ExpectExec("INSERT INTO threat_intel").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO threat_intel_event_outbox").
		WillReturnError(errors.New("outbox unavailable"))
	mock.ExpectRollback()

	if _, err := srv.commitThreatIntelCommand(context.Background(), nil, command); err == nil {
		t.Fatal("expected transaction failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCommitThreatIntelCommandExactReplayDoesNotMutate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := newTransactionalThreatIntelTestServer(db)
	command := transactionalThreatIntelCommandFixture()
	eventID := deterministicThreatIntelEventID(command.Event.TenantID, command.Meta.IdempotencyKey)
	response := `{"event_id":"` + eventID + `","action_id":"action-threat-intel-entry-upsert","command_type":"entry_upsert","aggregate_version":7,"replayed":false,"compatibility_mode":false}`

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT request_sha256,response_payload::text").
		WithArgs(command.Event.TenantID, command.Meta.IdempotencyKey).
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "response_payload"}).
			AddRow(command.Meta.RequestSHA256, response))
	mock.ExpectRollback()

	receipt, err := srv.commitThreatIntelCommand(context.Background(), nil, command)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Replayed || receipt.AggregateVersion != 7 || receipt.EventID != eventID {
		t.Fatalf("unexpected replay receipt: %+v", receipt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCommitThreatIntelCommandRejectsChangedPayloadForSameKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := newTransactionalThreatIntelTestServer(db)
	command := transactionalThreatIntelCommandFixture()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT request_sha256,response_payload::text").
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "response_payload"}).
			AddRow("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", `{}`))
	mock.ExpectRollback()

	_, err = srv.commitThreatIntelCommand(context.Background(), nil, command)
	if !errors.Is(err, errThreatIntelIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func newTransactionalThreatIntelTestServer(sqlDB *sql.DB) *server {
	return &server{
		intel: threatintel.NewService(sqlDB, zap.NewNop()), auditDB: sqlDB,
		threatIntelTopic: "threat.intel.v1", logger: zap.NewNop(),
	}
}

func transactionalThreatIntelCommandFixture() threatIntelCommand {
	occurredAt := time.Date(2026, 7, 31, 1, 2, 3, 0, time.UTC)
	entry := threatintel.IntelEntry{
		TenantID: "tenant-a", Type: "ip", Value: "203.0.113.9",
		Reputation: threatintel.RepMalicious, Category: "c2",
		Source: "manual", Description: "test indicator", LastSeen: occurredAt,
	}
	meta := threatIntelCommandMeta{
		ActionID:       "action-threat-intel-entry-upsert",
		IdempotencyKey: "idem-threat-intel-entry-upsert-0001",
		RequestSHA256:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CommandType:    "entry_upsert", Reason: "analyst-confirmed",
		TraceID: "trace-a",
	}
	event := threatIntelEvent{
		EventType: "threat_intel.entry_upserted", Version: 1, SchemaVersion: 1,
		TenantID: "tenant-a", Source: "manual", Entry: &entry, Count: 1,
		TraceID: "trace-a", OccurredAt: occurredAt,
	}
	return threatIntelCommand{
		Entries: []threatintel.IntelEntry{entry}, Event: event, Meta: meta,
		Action: "THREAT_INTEL_ENTRY_UPSERTED", ObjectID: "ip:203.0.113.9",
	}
}

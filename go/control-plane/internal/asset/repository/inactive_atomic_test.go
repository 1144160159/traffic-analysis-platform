package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func TestMarkInactiveSinceAtomicCommitsRevisionHistoryAuditOutboxAndRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, _ := NewAssetRepository(db, zap.NewNop())
	command := inactiveCommandFixture()
	lastSeen := command.Cutoff.Add(-time.Hour)
	firstSeen := lastSeen.Add(-24 * time.Hour)

	mock.ExpectBegin()
	expectInactiveIdentityLock(mock, command)
	mock.ExpectQuery("SELECT request_hash,actor,result_payload::text,trace_id").
		WithArgs("tenant-a", command.IdempotencyKey).
		WillReturnRows(sqlmock.NewRows([]string{"request_hash", "actor", "result_payload", "trace_id"}))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(hashtextextended($1,0))")).
		WithArgs("asset-inactive-tenant:tenant-a").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT asset_id,revision,display_code").
		WithArgs("tenant-a", command.Cutoff.UTC()).
		WillReturnRows(sqlmock.NewRows([]string{
			"asset_id", "revision", "display_code", "tenant_id", "asset_type", "status",
			"ip_address", "mac_address", "hostname", "vendor", "os_type", "source",
			"vlan_id", "switch_port", "department", "campus", "owner", "criticality",
			"tags", "metadata", "first_seen", "last_seen",
		}).AddRow(
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", 1, "SRV-AAAA", "tenant-a", "server", "active",
			"10.1.2.3", "00:11:22:33:44:55", "server-a", "vendor", "linux", "manual",
			"10", "Gi1/0/1", "ops", "north", "owner-a", 4,
			[]byte(`{"scope":"test"}`), []byte(`{}`), firstSeen, lastSeen,
		))
	mock.ExpectQuery("UPDATE assets").
		WithArgs("tenant-a", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(2))
	mock.ExpectExec("INSERT INTO asset_events").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO asset_event_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO asset_inactive_requests").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	result, err := repo.MarkInactiveSinceAtomic(context.Background(), "tenant-a", command)
	if err != nil {
		t.Fatalf("MarkInactiveSinceAtomic: %v", err)
	}
	if result.Count != 1 || len(result.EventIDs) != 1 || result.IdempotentReplay || result.TraceID != command.TraceID {
		t.Fatalf("unexpected inactive result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMarkInactiveSinceAtomicHistoryFailureRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, _ := NewAssetRepository(db, zap.NewNop())
	command := inactiveCommandFixture()
	observed := command.Cutoff.Add(-time.Hour)

	mock.ExpectBegin()
	expectInactiveIdentityLock(mock, command)
	mock.ExpectQuery("SELECT request_hash,actor,result_payload::text,trace_id").
		WillReturnRows(sqlmock.NewRows([]string{"request_hash", "actor", "result_payload", "trace_id"}))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT asset_id,revision,display_code").WillReturnRows(sqlmock.NewRows([]string{
		"asset_id", "revision", "display_code", "tenant_id", "asset_type", "status",
		"ip_address", "mac_address", "hostname", "vendor", "os_type", "source",
		"vlan_id", "switch_port", "department", "campus", "owner", "criticality",
		"tags", "metadata", "first_seen", "last_seen",
	}).AddRow(
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", 1, "SRV-AAAA", "tenant-a", "server", "active",
		"10.1.2.3", "00:11:22:33:44:55", "server-a", "vendor", "linux", "manual",
		"", "", "", "", "", 0, []byte(`{}`), []byte(`{}`), observed, observed,
	))
	mock.ExpectQuery("UPDATE assets").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(2))
	mock.ExpectExec("INSERT INTO asset_events").WillReturnError(errors.New("history unavailable"))
	mock.ExpectRollback()

	if _, err := repo.MarkInactiveSinceAtomic(context.Background(), "tenant-a", command); err == nil {
		t.Fatal("expected history failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMarkInactiveSinceAtomicExactReplay(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, _ := NewAssetRepository(db, zap.NewNop())
	command := inactiveCommandFixture()
	eventID := "11111111-1111-4111-8111-111111111111"
	payload, _ := json.Marshal(config.AssetInactiveResult{Count: 1, EventIDs: []string{eventID}, TraceID: command.TraceID})

	mock.ExpectBegin()
	expectInactiveIdentityLock(mock, command)
	mock.ExpectQuery("SELECT request_hash,actor,result_payload::text,trace_id").
		WithArgs("tenant-a", command.IdempotencyKey).
		WillReturnRows(sqlmock.NewRows([]string{"request_hash", "actor", "result_payload", "trace_id"}).
			AddRow(inactiveRequestHash("tenant-a", command), command.Actor, payload, command.TraceID))
	mock.ExpectCommit()

	result, err := repo.MarkInactiveSinceAtomic(context.Background(), "tenant-a", command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !result.IdempotentReplay || result.Count != 1 || len(result.EventIDs) != 1 || result.EventIDs[0] != eventID {
		t.Fatalf("unexpected replay: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func inactiveCommandFixture() config.AssetInactiveCommand {
	return config.AssetInactiveCommand{
		ActionID:       config.AssetInactiveSweepAction,
		IdempotencyKey: "asset-inactive-test-key-0001",
		Actor:          "asset-lifecycle-scheduler",
		Reason:         "mark stale test assets inactive",
		TraceID:        "trace-inactive-2026-08-03",
		RequestID:      "request-inactive-2026-08-03",
		Cutoff:         time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
	}
}

func expectInactiveIdentityLock(mock sqlmock.Sqlmock, command config.AssetInactiveCommand) {
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(hashtextextended($1,0))")).
		WithArgs("asset-inactive-idem:tenant-a:" + command.IdempotencyKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func inactiveRequestHash(tenantID string, command config.AssetInactiveCommand) string {
	payload, _ := json.Marshal(assetInactiveIdentity{
		TenantID: tenantID,
		ActionID: command.ActionID,
		Actor:    command.Actor,
		Reason:   command.Reason,
		Cutoff:   command.Cutoff.UTC().Format(timeFormatRFC3339Nano),
	})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

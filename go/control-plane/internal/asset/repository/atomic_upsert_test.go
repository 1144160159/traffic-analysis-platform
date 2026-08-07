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

func TestUpsertAtomicExactReplayReturnsStoredResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, _ := NewAssetRepository(db, zap.NewNop())
	rec, command := atomicAssetFixture()

	mock.ExpectBegin()
	expectAssetIdempotencyLock(mock, rec, command)
	mock.ExpectQuery("SELECT request_hash,actor,asset_id::text").
		WithArgs(rec.TenantID, command.IdempotencyKey).
		WillReturnRows(sqlmock.NewRows([]string{
			"request_hash", "actor", "asset_id", "created", "resulting_revision",
			"event_id", "outbox_id", "trace_id",
		}).AddRow(atomicRequestHash(rec, command), command.Actor, rec.AssetID, true, 1,
			"11111111-1111-4111-8111-111111111111", 17, command.TraceID))
	mock.ExpectCommit()

	result, err := repo.UpsertAtomic(context.Background(), rec, command)
	if err != nil {
		t.Fatalf("UpsertAtomic replay: %v", err)
	}
	if !result.IdempotentReplay || result.OutboxID != 17 || result.Revision != 1 {
		t.Fatalf("unexpected replay result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertAtomicCreateCommitsAllDurableEffects(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, _ := NewAssetRepository(db, zap.NewNop())
	rec, command := atomicAssetFixture()

	mock.ExpectBegin()
	expectAssetIdempotencyLock(mock, rec, command)
	mock.ExpectQuery("SELECT request_hash,actor,asset_id::text").
		WithArgs(rec.TenantID, command.IdempotencyKey).
		WillReturnError(sqlmock.ErrCancelled)
	mock.ExpectRollback()
	_, err = repo.UpsertAtomic(context.Background(), rec, command)
	if err == nil {
		t.Fatal("expected idempotency lookup failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	db2, mock2, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	repo, _ = NewAssetRepository(db2, zap.NewNop())
	rec, command = atomicAssetFixture()
	mock2.ExpectBegin()
	expectAssetIdempotencyLock(mock2, rec, command)
	mock2.ExpectQuery("SELECT request_hash,actor,asset_id::text").
		WillReturnRows(sqlmock.NewRows([]string{
			"request_hash", "actor", "asset_id", "created", "resulting_revision",
			"event_id", "outbox_id", "trace_id",
		}))
	mock2.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(hashtextextended($1,0))")).
		WithArgs("8:tenant-a:00:11:22:33:44:55").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock2.ExpectQuery("SELECT asset_id,revision,display_code").
		WithArgs(rec.TenantID, rec.MACAddress).
		WillReturnRows(sqlmock.NewRows([]string{
			"asset_id", "revision", "display_code", "tenant_id", "asset_type", "status",
			"ip_address", "mac_address", "hostname", "vendor", "os_type", "source",
			"vlan_id", "switch_port", "department", "campus", "owner", "criticality",
			"tags", "metadata", "first_seen", "last_seen",
		}))
	mock2.ExpectQuery("INSERT INTO assets").
		WillReturnRows(sqlmock.NewRows([]string{"asset_id", "revision"}).AddRow(rec.AssetID, 1))
	mock2.ExpectExec("INSERT INTO asset_events").WillReturnResult(sqlmock.NewResult(1, 1))
	mock2.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))
	mock2.ExpectQuery("INSERT INTO asset_event_outbox").
		WillReturnRows(sqlmock.NewRows([]string{"outbox_id"}).AddRow(23))
	mock2.ExpectExec("INSERT INTO asset_upsert_requests").WillReturnResult(sqlmock.NewResult(1, 1))
	mock2.ExpectCommit()

	result, err := repo.UpsertAtomic(context.Background(), rec, command)
	if err != nil {
		t.Fatalf("UpsertAtomic create: %v", err)
	}
	if !result.Created || result.Revision != 1 || result.OutboxID != 23 || result.IdempotentReplay {
		t.Fatalf("unexpected create result: %+v", result)
	}
	if err := mock2.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertAtomicHistoryFailureRollsBackAsset(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, _ := NewAssetRepository(db, zap.NewNop())
	rec, command := atomicAssetFixture()

	mock.ExpectBegin()
	expectAssetIdempotencyLock(mock, rec, command)
	mock.ExpectQuery("SELECT request_hash,actor,asset_id::text").
		WillReturnRows(sqlmock.NewRows([]string{
			"request_hash", "actor", "asset_id", "created", "resulting_revision",
			"event_id", "outbox_id", "trace_id",
		}))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT asset_id,revision,display_code").
		WillReturnRows(sqlmock.NewRows([]string{
			"asset_id", "revision", "display_code", "tenant_id", "asset_type", "status",
			"ip_address", "mac_address", "hostname", "vendor", "os_type", "source",
			"vlan_id", "switch_port", "department", "campus", "owner", "criticality",
			"tags", "metadata", "first_seen", "last_seen",
		}))
	mock.ExpectQuery("INSERT INTO assets").
		WillReturnRows(sqlmock.NewRows([]string{"asset_id", "revision"}).AddRow(rec.AssetID, 1))
	mock.ExpectExec("INSERT INTO asset_events").WillReturnError(errors.New("history unavailable"))
	mock.ExpectRollback()

	if _, err := repo.UpsertAtomic(context.Background(), rec, command); err == nil {
		t.Fatal("expected atomic history failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func atomicAssetFixture() (*config.AssetRecord, config.AssetUpsertCommand) {
	return &config.AssetRecord{
			AssetID:    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			TenantID:   "tenant-a",
			MACAddress: "00:11:22:33:44:55",
			IPAddress:  "10.1.2.3",
			AssetType:  "server",
			Status:     "active",
			Source:     "manual",
		}, config.AssetUpsertCommand{
			ActionID:         config.AssetUpsertAction,
			ExpectedRevision: 0,
			IdempotencyKey:   "asset-upsert-test-key-0001",
			Actor:            "operator-a",
			Reason:           "asset upsert",
			TraceID:          "trace-asset-0001",
			RequestID:        "request-asset-0001",
		}
}

func atomicRequestHash(rec *config.AssetRecord, command config.AssetUpsertCommand) string {
	requestAsset := *rec
	requestAsset.Revision = 0
	requestAsset.FirstSeen = time.Time{}
	requestAsset.LastSeen = time.Time{}
	payload, _ := json.Marshal(assetUpsertIdentity{
		TenantID:               rec.TenantID,
		ActionID:               command.ActionID,
		ExpectedRevision:       command.ExpectedRevision,
		ResolveCurrentRevision: command.ResolveCurrentRevision,
		Actor:                  command.Actor,
		Reason:                 command.Reason,
		HistoryEventType:       command.HistoryEventType,
		ObservedAt:             command.ObservedAt.UTC(),
		Asset:                  &requestAsset,
	})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func expectAssetIdempotencyLock(mock sqlmock.Sqlmock, rec *config.AssetRecord, command config.AssetUpsertCommand) {
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(hashtextextended($1,0))")).
		WithArgs("asset-upsert-idem:" + rec.TenantID + ":" + command.IdempotencyKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

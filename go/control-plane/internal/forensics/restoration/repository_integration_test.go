package restoration

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func TestRestorationAuthorityTransactionEphemeralPostgres(t *testing.T) {
	if os.Getenv("M03_RESTORATION_PG_INTEGRATION_ENABLED") != "true" {
		t.Skip("owned ephemeral PostgreSQL is not enabled")
	}
	if os.Getenv("M03_RESTORATION_PG_SENTINEL") != "codex_ephemeral_m03_restoration_postgres" {
		t.Fatal("refusing a PostgreSQL instance that is not explicitly owned by this test")
	}
	dsn := os.Getenv("M03_RESTORATION_PG_DSN")
	if dsn == "" {
		t.Fatal("owned ephemeral PostgreSQL DSN is required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var sentinel string
	if err := db.QueryRowContext(ctx, `SELECT marker FROM codex_ephemeral_m03_restoration_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel PostgreSQL: marker=%q err=%v", sentinel, err)
	}
	repository, err := NewRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.VerifySchema(ctx); err != nil {
		t.Fatalf("verify restoration schema: %v", err)
	}

	command := ephemeralRestorationCommand()
	defer cleanupRestorationAuthorityFixture(t, db, command.Manifest.TenantID)
	now := command.Manifest.CompletedAt
	firstClaim, err := repository.ClaimRequest(ctx, command.Manifest.TenantID, command.IdempotencyKey,
		command.RequestSHA256, command.TraceID, now, now.Add(time.Minute), 1)
	if err != nil || firstClaim.Result != AdmissionClaimed || firstClaim.ClaimToken == uuid.Nil {
		t.Fatalf("first claim = %+v, %v", firstClaim, err)
	}
	inProgress, err := repository.ClaimRequest(ctx, command.Manifest.TenantID, command.IdempotencyKey,
		command.RequestSHA256, command.TraceID, now.Add(time.Second), now.Add(2*time.Minute), 1)
	if err != nil || inProgress.Result != AdmissionInProgress {
		t.Fatalf("unexpired duplicate claim = %+v, %v", inProgress, err)
	}

	// Simulate a worker that lost its lease after writing the immutable object.
	// The successor gets a new fencing token; the predecessor must not commit.
	if _, err := db.ExecContext(ctx, `UPDATE file_restoration_requests SET lease_until=$1
		WHERE tenant_id=$2 AND idempotency_key=$3`, now.Add(-time.Second), command.Manifest.TenantID, command.IdempotencyKey); err != nil {
		t.Fatalf("expire first claim: %v", err)
	}
	successor, err := repository.ClaimRequest(ctx, command.Manifest.TenantID, command.IdempotencyKey,
		command.RequestSHA256, command.TraceID, now.Add(2*time.Second), now.Add(2*time.Minute), 1)
	if err != nil || successor.Result != AdmissionClaimed || successor.ClaimToken == uuid.Nil || successor.ClaimToken == firstClaim.ClaimToken {
		t.Fatalf("successor claim = %+v, %v", successor, err)
	}
	if err := repository.RecordOrphan(ctx, command.Manifest.TenantID, command.Manifest.RestorationID, *command.Manifest.Object); err != nil {
		t.Fatalf("record immutable object candidate: %v", err)
	}
	command.ClaimToken = firstClaim.ClaimToken
	if _, err := repository.Commit(ctx, command); err == nil {
		t.Fatal("expired lease predecessor committed restoration authority")
	}
	assertRestorationCounts(t, db, command.Manifest.TenantID, 0, 0, 0, 1, "candidate")

	command.ClaimToken = successor.ClaimToken
	receipt, err := repository.Commit(ctx, command)
	if err != nil {
		t.Fatalf("commit restoration authority: %v", err)
	}
	if receipt.Replayed || receipt.RestorationID != command.Manifest.RestorationID.String() ||
		receipt.EventID != deterministicEventID(command.Manifest.TenantID, command.IdempotencyKey).String() ||
		receipt.ObjectSHA256 != command.Manifest.Object.SHA256 || receipt.OutboxStatus != "pending" {
		t.Fatalf("unexpected commit receipt: %+v", receipt)
	}
	assertRestorationCounts(t, db, command.Manifest.TenantID, 1, 1, 1, 1, "reconciled")

	replay, err := repository.Commit(ctx, command)
	if err != nil || !replay.Replayed || replay.EventID != receipt.EventID || replay.RestorationID != receipt.RestorationID {
		t.Fatalf("commit replay = %+v, %v", replay, err)
	}
	claimReplay, err := repository.ClaimRequest(ctx, command.Manifest.TenantID, command.IdempotencyKey,
		command.RequestSHA256, command.TraceID, now.Add(3*time.Second), now.Add(3*time.Minute), 1)
	if err != nil || claimReplay.Result != AdmissionReplay || claimReplay.Receipt == nil ||
		!claimReplay.Receipt.Replayed || claimReplay.Receipt.EventID != receipt.EventID {
		t.Fatalf("claim replay = %+v, %v", claimReplay, err)
	}
	assertRestorationCounts(t, db, command.Manifest.TenantID, 1, 1, 1, 1, "reconciled")

	if _, found, err := repository.LookupRequest(ctx, "different-tenant", command.IdempotencyKey, command.RequestSHA256); err != nil || found {
		t.Fatalf("cross-tenant request leaked: found=%v err=%v", found, err)
	}
	if _, _, err := repository.LookupRequest(ctx, command.Manifest.TenantID, command.IdempotencyKey,
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency hash collision error = %v", err)
	}
}

func ephemeralRestorationCommand() CommitCommand {
	command := completeCommand()
	now := time.Now().UTC().Truncate(time.Millisecond)
	tenantID := "restoration-" + uuid.NewString()
	idempotencyKey := "restoration-request-" + uuid.NewString()
	restorationID := deterministicRestorationID(tenantID, idempotencyKey)
	command.ClaimToken = uuid.Nil
	command.IdempotencyKey = idempotencyKey
	command.Manifest.TenantID = tenantID
	command.Manifest.RestorationID = restorationID
	command.Manifest.IdempotencyKey = idempotencyKey
	command.Manifest.CaptureTimeStart = now.Add(-time.Minute)
	command.Manifest.CaptureTimeEnd = now
	command.Manifest.CreatedAt = now.Add(-time.Second)
	command.Manifest.CompletedAt = now
	command.Manifest.SessionAuthority.TenantID = tenantID
	command.Manifest.SessionAuthority.TsStart = now.Add(-2 * time.Minute)
	command.Manifest.SessionAuthority.TsEnd = now.Add(time.Minute)
	command.Manifest.SourceObjectReceipts[0].Key = tenantID + "/probe-a/source.pcap"
	command.Manifest.PacketRanges[0].ObjectKey = command.Manifest.SourceObjectReceipts[0].Key
	command.Manifest.TCPSequenceRanges[0].ObjectKey = command.Manifest.SourceObjectReceipts[0].Key
	command.Manifest.Object.Key = "tenants/" + tenantID + "/restorations/" + restorationID.String() + "/1/content.bin"
	command.Manifest.Object.ObservedAt = now.Add(-500 * time.Millisecond)
	command.Manifest.Object.RetentionUntil = now.Add(24 * time.Hour)
	return command
}

func assertRestorationCounts(
	t *testing.T,
	db *sql.DB,
	tenantID string,
	wantManifest, wantOutbox, wantAudit, wantOrphan int,
	wantOrphanStatus string,
) {
	t.Helper()
	var manifests, outbox, audit, requests, orphans int
	var requestState, orphanStatus string
	err := db.QueryRow(`SELECT
		(SELECT count(*) FROM file_restoration_manifests WHERE tenant_id=$1),
		(SELECT count(*) FROM file_restoration_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM file_restoration_audit WHERE tenant_id=$1),
		(SELECT count(*) FROM file_restoration_requests WHERE tenant_id=$1),
		(SELECT count(*) FROM file_restoration_orphans WHERE tenant_id=$1),
		COALESCE((SELECT state FROM file_restoration_requests WHERE tenant_id=$1 LIMIT 1),''),
		COALESCE((SELECT reconciliation_status FROM file_restoration_orphans WHERE tenant_id=$1 LIMIT 1),'')`, tenantID).
		Scan(&manifests, &outbox, &audit, &requests, &orphans, &requestState, &orphanStatus)
	if err != nil {
		t.Fatalf("read restoration authority counts: %v", err)
	}
	wantRequestState := "processing"
	if wantManifest == 1 {
		wantRequestState = "committed"
	}
	if manifests != wantManifest || outbox != wantOutbox || audit != wantAudit || requests != 1 ||
		orphans != wantOrphan || requestState != wantRequestState || orphanStatus != wantOrphanStatus {
		t.Fatalf("authority counts manifest=%d outbox=%d audit=%d requests=%d orphans=%d request=%q orphan=%q",
			manifests, outbox, audit, requests, orphans, requestState, orphanStatus)
	}
}

func cleanupRestorationAuthorityFixture(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	for _, statement := range []string{
		`DELETE FROM file_restoration_audit WHERE tenant_id=$1`,
		`DELETE FROM file_restoration_requests WHERE tenant_id=$1`,
		`DELETE FROM file_restoration_outbox WHERE tenant_id=$1`,
		`DELETE FROM file_restoration_orphans WHERE tenant_id=$1`,
		`DELETE FROM file_restoration_manifests WHERE tenant_id=$1`,
	} {
		if _, err := db.Exec(statement, tenantID); err != nil {
			t.Errorf("cleanup restoration authority fixture: %v", err)
		}
	}
}

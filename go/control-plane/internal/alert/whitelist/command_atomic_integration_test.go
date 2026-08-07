package whitelist

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	rulesconsumer "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/consumer"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func TestWhitelistGovernanceAtomicEphemeralPostgres(t *testing.T) {
	dsn := os.Getenv("WHITELIST_GOVERNANCE_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("WHITELIST_GOVERNANCE_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_whitelist_governance_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", marker, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tenantID := "whitelist-" + uuid.NewString()
	otherTenant := tenantID + "-other"
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES($1,$2),($3,$4)`, tenantID, "Whitelist Integration", otherTenant, "Other Tenant"); err != nil {
		t.Fatal(err)
	}
	defer cleanupWhitelistIntegration(t, db, tenantID, otherTenant)
	repo := NewRepository(db, zap.NewNop())
	creator := "00000000-0000-0000-0000-000000000101"
	reviewer := "00000000-0000-0000-0000-000000000102"

	entryA := integrationWhitelistEntry(tenantID, creator, "atomic-a.example.test", time.Now().Add(24*time.Hour))
	createA := integrationWhitelistMeta(tenantID, creator, ActionCreate, "whitelist-create-a-00001", 0)
	createdA, err := repo.CreateAtomic(ctx, entryA, createA, integrationWhitelistAudit(creator))
	if err != nil || createdA.Entry.Version != 1 || createdA.Receipt.Replayed {
		t.Fatalf("create A result=%+v err=%v", createdA, err)
	}
	replayEntry := integrationWhitelistEntry(tenantID, creator, "atomic-a.example.test", entryA.ExpiresAt.UTC())
	replayA, err := repo.CreateAtomic(ctx, replayEntry, createA, integrationWhitelistAudit(creator))
	if err != nil || !replayA.Receipt.Replayed || replayA.Receipt.EventID != createdA.Receipt.EventID || replayEntry.ID != entryA.ID {
		t.Fatalf("create replay=%+v err=%v", replayA, err)
	}
	collision := integrationWhitelistEntry(tenantID, creator, "different.example.test", time.Now().Add(24*time.Hour))
	if _, err := repo.CreateAtomic(ctx, collision, createA, integrationWhitelistAudit(creator)); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency collision, got %v", err)
	}

	pending := "pending"
	submitReq := UpdateRequest{Status: &pending, ApprovalStatus: &pending}
	submitA := integrationWhitelistMeta(tenantID, creator, ActionSubmitApproval, "whitelist-submit-a-0001", 1)
	submittedA, err := repo.UpdateAtomic(ctx, entryA.ID, submitReq, submitA, integrationWhitelistAudit(creator))
	if err != nil || submittedA.Entry.Version != 2 || submittedA.Entry.Status != "pending" {
		t.Fatalf("submit A=%+v err=%v", submittedA, err)
	}
	submitReplay, err := repo.UpdateAtomic(ctx, entryA.ID, submitReq, submitA, integrationWhitelistAudit(creator))
	if err != nil || !submitReplay.Receipt.Replayed || submitReplay.Receipt.EventID != submittedA.Receipt.EventID {
		t.Fatalf("submit replay=%+v err=%v", submitReplay, err)
	}

	active, approved := "active", "approved"
	approveReq := UpdateRequest{Status: &active, ApprovalStatus: &approved}
	creatorApprove := integrationWhitelistMeta(tenantID, creator, ActionApprove, "whitelist-approve-self-01", 2)
	creatorApprove.ApprovalAuthorized = true
	if _, err := repo.UpdateAtomic(ctx, entryA.ID, approveReq, creatorApprove, integrationWhitelistAudit(creator)); err == nil {
		t.Fatal("creator approval must fail closed")
	}
	approveA := integrationWhitelistMeta(tenantID, reviewer, ActionApprove, "whitelist-approve-a-0001", 2)
	approveA.ApprovalAuthorized = true
	approvedA, err := repo.UpdateAtomic(ctx, entryA.ID, approveReq, approveA, integrationWhitelistAudit(reviewer))
	if err != nil || approvedA.Entry.Version != 3 || approvedA.Entry.RuleEffectStatus != RuleEffectPending {
		t.Fatalf("approve A=%+v err=%v", approvedA, err)
	}
	if repo.IsWhitelisted(ctx, tenantID, entryA.Value) {
		t.Fatal("approved entry must not match before rule ACK")
	}
	projection, err := rulesconsumer.NewPostgresWhitelistRuleProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ApplyWhitelistRuleProjection(ctx, rulesconsumer.WhitelistRuleProjectionInput{
		EventID: approvedA.Receipt.EventID, TenantID: tenantID, EntryID: entryA.ID,
		EntryVersion: 3, DesiredState: "effective", EntryType: entryA.Type,
		MatchValue: entryA.Value, Scope: entryA.Scope, ExpiresAt: entryA.ExpiresAt,
		RuleRevision:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AckEventID:     uuid.NewString(),
		PayloadSHA256:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		KafkaPartition: 0, KafkaOffset: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if !repo.IsWhitelisted(ctx, tenantID, entryA.Value) {
		t.Fatal("approved entry must match after applied ACK")
	}
	if repo.IsWhitelisted(ctx, otherTenant, entryA.Value) {
		t.Fatal("cross-tenant whitelist match leaked")
	}

	disabled := "disabled"
	disableReq := UpdateRequest{Status: &disabled}
	disableA := integrationWhitelistMeta(tenantID, reviewer, ActionDisable, "whitelist-disable-a-001", 3)
	disabledA, err := repo.UpdateAtomic(ctx, entryA.ID, disableReq, disableA, integrationWhitelistAudit(reviewer))
	if err != nil || disabledA.Entry.Version != 4 || disabledA.Entry.RuleEffectStatus != RuleEffectPending {
		t.Fatalf("disable A=%+v err=%v", disabledA, err)
	}
	if repo.IsWhitelisted(ctx, tenantID, entryA.Value) {
		t.Fatal("disabled entry must stop matching while revocation ACK is pending")
	}
	crossTenant := integrationWhitelistMeta(otherTenant, reviewer, ActionDisable, "whitelist-cross-tenant-1", 4)
	if _, err := repo.UpdateAtomic(ctx, entryA.ID, disableReq, crossTenant, integrationWhitelistAudit(reviewer)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant update must be not found, got %v", err)
	}
	archiveA := integrationWhitelistMeta(tenantID, reviewer, ActionArchive, "whitelist-archive-a-001", 4)
	archivedA, err := repo.ArchiveAtomic(ctx, entryA.ID, archiveA, integrationWhitelistAudit(reviewer))
	if err != nil || archivedA.Entry.Version != 5 || archivedA.Entry.ArchivedAt == nil {
		t.Fatalf("archive A=%+v err=%v", archivedA, err)
	}
	if _, err := repo.Get(ctx, tenantID, entryA.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("archived entry remained visible: %v", err)
	}

	entryB := integrationWhitelistEntry(tenantID, creator, "atomic-expiry.example.test", time.Now().Add(-time.Minute))
	if _, err := repo.CreateAtomic(ctx, entryB, integrationWhitelistMeta(tenantID, creator, ActionCreate, "whitelist-create-b-00001", 0), integrationWhitelistAudit(creator)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateAtomic(ctx, entryB.ID, submitReq, integrationWhitelistMeta(tenantID, creator, ActionSubmitApproval, "whitelist-submit-b-0001", 1), integrationWhitelistAudit(creator)); err != nil {
		t.Fatal(err)
	}
	approveB := integrationWhitelistMeta(tenantID, reviewer, ActionApprove, "whitelist-approve-b-0001", 2)
	approveB.ApprovalAuthorized = true
	if _, err := repo.UpdateAtomic(ctx, entryB.ID, approveReq, approveB, integrationWhitelistAudit(reviewer)); err != nil {
		t.Fatal(err)
	}
	expired, err := repo.ExpireDue(ctx, 10)
	if err != nil || expired != 1 {
		t.Fatalf("expire due count=%d err=%v", expired, err)
	}
	expiredB, err := repo.Get(ctx, tenantID, entryB.ID)
	if err != nil || expiredB.Status != "disabled" || expiredB.Version != 4 || expiredB.RuleDesiredState != "revoked" {
		t.Fatalf("expired B=%+v err=%v", expiredB, err)
	}

	var entries, history, outbox, requests, effects, audits int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM whitelist WHERE tenant_id=$1),
		(SELECT count(*) FROM whitelist_entry_versions WHERE tenant_id=$1),
		(SELECT count(*) FROM whitelist_event_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM whitelist_command_requests WHERE tenant_id=$1),
		(SELECT count(*) FROM whitelist_rule_effects WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='whitelist')`, tenantID).
		Scan(&entries, &history, &outbox, &requests, &effects, &audits); err != nil {
		t.Fatal(err)
	}
	if entries != 2 || history != 9 || outbox != 9 || requests != 9 || effects != 5 || audits != 9 {
		t.Fatalf("atomic counts entries=%d history=%d outbox=%d requests=%d effects=%d audits=%d",
			entries, history, outbox, requests, effects, audits)
	}
}

func integrationWhitelistEntry(tenantID, actor, value string, expires time.Time) *Entry {
	expires = expires.UTC().Truncate(time.Microsecond)
	return &Entry{TenantID: tenantID, Type: "domain", Value: value, Reason: "integration false positive",
		Description: "sentinel integration", OwnerRole: "security-operations", Scope: "tenant",
		RiskLevel: "medium", CoveredAlerts: 3, CoveredAssets: 1, CreatedBy: actor, ExpiresAt: &expires}
}

func integrationWhitelistMeta(tenantID, actor, action, key string, version int) CommandMeta {
	return CommandMeta{TenantID: tenantID, ActorID: actor, ActionID: action, IdempotencyKey: key,
		ExpectedVersion: version, Reason: "integration verification", TraceID: "trace-" + key,
		SourceIP: "127.0.0.1", UserAgent: "whitelist-integration"}
}

func integrationWhitelistAudit(actor string) AuditRecord {
	return AuditRecord{UserID: actor, IPAddress: "127.0.0.1", UserAgent: "whitelist-integration",
		Detail: map[string]interface{}{"source": "ephemeral-integration"}}
}

func cleanupWhitelistIntegration(t *testing.T, db *sql.DB, tenantIDs ...string) {
	t.Helper()
	for _, tenantID := range tenantIDs {
		for _, statement := range []string{
			`DELETE FROM whitelist_rule_projection WHERE tenant_id=$1`,
			`DELETE FROM whitelist_rule_effects WHERE tenant_id=$1`,
			`DELETE FROM whitelist_command_requests WHERE tenant_id=$1`,
			`DELETE FROM whitelist_entry_versions WHERE tenant_id=$1`,
			`DELETE FROM whitelist_event_outbox WHERE tenant_id=$1`,
			`DELETE FROM audit_logs WHERE tenant_id=$1 AND object_type='whitelist'`,
			`DELETE FROM whitelist WHERE tenant_id=$1`,
			`DELETE FROM tenants WHERE tenant_id=$1`,
		} {
			if _, err := db.Exec(statement, tenantID); err != nil {
				t.Errorf("cleanup whitelist fixture %s: %v", tenantID, err)
			}
		}
	}
}

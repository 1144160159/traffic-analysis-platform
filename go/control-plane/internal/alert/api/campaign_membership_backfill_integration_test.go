package api

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func TestCampaignMembershipBackfillPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("CAMPAIGN_AGGREGATE_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("CAMPAIGN_AGGREGATE_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_campaign_aggregate_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}

	tenantID := "campaign-backfill-" + time.Now().UTC().Format("150405000000")
	campaignID := "campaign-backfill-main"
	cleanupCampaignAggregateIntegration(t, db, tenantID)
	defer cleanupCampaignAggregateIntegration(t, db, tenantID)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Campaign Backfill Integration')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaign_workbench_state
		(tenant_id,campaign_id,assignee,status,state_version,member_count,updated_by)
		VALUES ($1,$2,'owner','investigating',7,1,'seed')`, tenantID, campaignID); err != nil {
		t.Fatal(err)
	}
	seed := func(alertID, status string, revision, campaignRevision int64) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO campaign_alert_links
			(relation_id,tenant_id,campaign_id,alert_id,status,revision,campaign_revision,reason,
			 idempotency_key,created_by,updated_by)
			VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,'historical seed',$8,'seed','seed')`, uuid.NewString(),
			tenantID, campaignID, alertID, status, revision, campaignRevision,
			"backfill-seed-"+alertID); err != nil {
			t.Fatal(err)
		}
	}
	seed("alert-a", "linked", 1, 0)
	seed("alert-b", "unlinked", 2, 6)
	seed("alert-c", "linked", 5, 6)
	seed("alert-z", "linked", 3, 7)

	manifest := CampaignMembershipBackfillManifest{
		ContractVersion: 1,
		RunID:           uuid.NewString(),
		TenantID:        tenantID,
		Source: CampaignMembershipBackfillSource{
			Kind: "clickhouse_export", URI: "minio://evidence/campaign-members.json",
			SHA256:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SnapshotID: "ch-campaign-snapshot-1", AsOf: time.Now().UTC().Truncate(time.Second),
		},
		Reason:    "bind historical campaign memberships from authorized export",
		CreatedBy: "backfill-integration-operator",
		Campaigns: []CampaignMembershipBackfillCampaign{{
			CampaignID: campaignID, ExpectedRevision: 7,
			AlertIDs: []string{"alert-d", "alert-b", "alert-a", "alert-c"},
		}},
	}
	result, err := RunCampaignMembershipBackfill(context.Background(), db, zap.NewNop(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.CompletedCampaignCount != 1 || result.FailedCampaignCount != 0 ||
		result.InsertedCount != 1 || result.BoundCount != 1 || result.UnchangedCount != 1 ||
		result.SkippedUnlinkedCount != 1 || len(result.ManifestSHA256) != 64 {
		t.Fatalf("backfill result=%+v", result)
	}

	var stateRevision int64
	var stateMembers int
	if err := db.QueryRow(`SELECT state_version,member_count FROM campaign_workbench_state
		WHERE tenant_id=$1 AND campaign_id=$2`, tenantID, campaignID).Scan(&stateRevision, &stateMembers); err != nil {
		t.Fatal(err)
	}
	if stateRevision != 8 || stateMembers != 4 {
		t.Fatalf("campaign state revision=%d members=%d", stateRevision, stateMembers)
	}
	type relationState struct {
		status           string
		revision         int64
		campaignRevision int64
	}
	want := map[string]relationState{
		"alert-a": {status: "linked", revision: 2, campaignRevision: 8},
		"alert-b": {status: "unlinked", revision: 2, campaignRevision: 6},
		"alert-c": {status: "linked", revision: 5, campaignRevision: 6},
		"alert-d": {status: "linked", revision: 1, campaignRevision: 8},
		"alert-z": {status: "linked", revision: 3, campaignRevision: 7},
	}
	for alertID, expected := range want {
		var actual relationState
		if err := db.QueryRow(`SELECT status,revision,campaign_revision FROM campaign_alert_links
			WHERE tenant_id=$1 AND campaign_id=$2 AND alert_id=$3`, tenantID, campaignID, alertID).
			Scan(&actual.status, &actual.revision, &actual.campaignRevision); err != nil {
			t.Fatal(err)
		}
		if actual != expected {
			t.Fatalf("alert %s state=%+v want=%+v", alertID, actual, expected)
		}
	}

	var itemCount, membershipHistory, membershipOutbox, aggregateHistory, aggregateOutbox, auditCount int
	if err := db.QueryRow(`SELECT
		(SELECT count(*) FROM campaign_membership_backfill_items WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_alert_link_history WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_alert_link_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_aggregate_history WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_aggregate_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND action='CAMPAIGN_MEMBERSHIP_BACKFILLED')`, tenantID).
		Scan(&itemCount, &membershipHistory, &membershipOutbox, &aggregateHistory, &aggregateOutbox, &auditCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 4 || membershipHistory != 2 || membershipOutbox != 2 || aggregateHistory != 1 ||
		aggregateOutbox != 1 || auditCount != 1 {
		t.Fatalf("evidence items=%d membership=%d/%d aggregate=%d/%d audit=%d", itemCount,
			membershipHistory, membershipOutbox, aggregateHistory, aggregateOutbox, auditCount)
	}

	replay, err := RunCampaignMembershipBackfill(context.Background(), db, zap.NewNop(), manifest)
	if err != nil || replay != result {
		t.Fatalf("replay=%+v err=%v want=%+v", replay, err, result)
	}
	var replayAuditCount int
	if err := db.QueryRow(`SELECT count(*) FROM audit_logs
		WHERE tenant_id=$1 AND action='CAMPAIGN_MEMBERSHIP_BACKFILLED'`, tenantID).Scan(&replayAuditCount); err != nil {
		t.Fatal(err)
	}
	if replayAuditCount != 1 {
		t.Fatalf("replay duplicated audit count=%d", replayAuditCount)
	}

	manifest.Reason = "attempt to reuse run id with a changed immutable manifest"
	if _, err := RunCampaignMembershipBackfill(context.Background(), db, zap.NewNop(), manifest); err == nil {
		t.Fatal("changed manifest unexpectedly reused an existing run id")
	}
}

func TestCampaignMembershipBackfillResumesFailedCampaignWithoutReplayingCompletedWork(t *testing.T) {
	dsn := os.Getenv("CAMPAIGN_AGGREGATE_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("CAMPAIGN_AGGREGATE_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_campaign_aggregate_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}

	tenantID := "campaign-backfill-resume-" + time.Now().UTC().Format("150405000000")
	campaignID := "campaign-backfill-retry"
	cleanupCampaignAggregateIntegration(t, db, tenantID)
	defer cleanupCampaignAggregateIntegration(t, db, tenantID)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Campaign Backfill Resume')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaign_workbench_state
		(tenant_id,campaign_id,status,state_version,member_count,updated_by)
		VALUES ($1,$2,'active',3,1,'seed')`, tenantID, campaignID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaign_alert_links
		(relation_id,tenant_id,campaign_id,alert_id,status,revision,campaign_revision,reason,
		 idempotency_key,created_by,updated_by)
		VALUES ($1::uuid,$2,$3,'alert-retry','linked',1,0,'historical seed','backfill-resume-seed','seed','seed')`,
		uuid.NewString(), tenantID, campaignID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE OR REPLACE FUNCTION codex_fail_campaign_backfill_audit_once()
		RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.action='CAMPAIGN_MEMBERSHIP_BACKFILLED' AND NEW.object_id='campaign-backfill-retry' THEN
		    RAISE EXCEPTION 'sentinel transient audit failure';
		  END IF;
		  RETURN NEW;
		END $$;
		DROP TRIGGER IF EXISTS codex_fail_campaign_backfill_audit_once ON audit_logs;
		CREATE TRIGGER codex_fail_campaign_backfill_audit_once BEFORE INSERT ON audit_logs
		FOR EACH ROW EXECUTE FUNCTION codex_fail_campaign_backfill_audit_once()`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(`DROP TRIGGER IF EXISTS codex_fail_campaign_backfill_audit_once ON audit_logs`)
		_, _ = db.Exec(`DROP FUNCTION IF EXISTS codex_fail_campaign_backfill_audit_once()`)
	}()

	manifest := CampaignMembershipBackfillManifest{
		ContractVersion: 1, RunID: uuid.NewString(), TenantID: tenantID,
		Source: CampaignMembershipBackfillSource{
			Kind: "clickhouse_export", URI: "minio://evidence/campaign-retry.json",
			SHA256:     "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			SnapshotID: "ch-campaign-retry-1", AsOf: time.Now().UTC().Truncate(time.Second),
		},
		Reason:    "verify resumable campaign membership backfill after transient failure",
		CreatedBy: "backfill-resume-operator",
		Campaigns: []CampaignMembershipBackfillCampaign{{
			CampaignID: campaignID, ExpectedRevision: 3, AlertIDs: []string{"alert-retry"},
		}},
	}
	partial, err := RunCampaignMembershipBackfill(context.Background(), db, zap.NewNop(), manifest)
	if err == nil || partial.Status != "partial" || partial.FailedCampaignCount != 1 || partial.CompletedCampaignCount != 0 {
		t.Fatalf("first run result=%+v err=%v", partial, err)
	}
	var revision, historyCount int
	if err := db.QueryRow(`SELECT
		(SELECT state_version FROM campaign_workbench_state WHERE tenant_id=$1 AND campaign_id=$2),
		(SELECT count(*) FROM campaign_alert_link_history WHERE tenant_id=$1)`, tenantID, campaignID).
		Scan(&revision, &historyCount); err != nil {
		t.Fatal(err)
	}
	if revision != 3 || historyCount != 0 {
		t.Fatalf("failed transaction leaked revision=%d history=%d", revision, historyCount)
	}
	if _, err := db.Exec(`DROP TRIGGER codex_fail_campaign_backfill_audit_once ON audit_logs`); err != nil {
		t.Fatal(err)
	}
	resumed, err := RunCampaignMembershipBackfill(context.Background(), db, zap.NewNop(), manifest)
	if err != nil || resumed.Status != "completed" || resumed.CompletedCampaignCount != 1 ||
		resumed.FailedCampaignCount != 0 || resumed.BoundCount != 1 {
		t.Fatalf("resumed result=%+v err=%v", resumed, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM campaign_alert_link_history WHERE tenant_id=$1`, tenantID).
		Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if historyCount != 1 {
		t.Fatalf("resumed campaign history count=%d", historyCount)
	}
}

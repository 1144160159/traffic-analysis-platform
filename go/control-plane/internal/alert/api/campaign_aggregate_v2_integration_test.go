package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// TestCampaignAggregateV2PostgresIntegration is guarded by an explicit table
// in a disposable PostgreSQL instance. A DSN alone never authorizes mutations.
func TestCampaignAggregateV2PostgresIntegration(t *testing.T) {
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

	tenantID := "campaign-v2-integration-" + time.Now().UTC().Format("150405000000")
	campaignID := "campaign-integration-1"
	cleanupCampaignAggregateIntegration(t, db, tenantID)
	defer cleanupCampaignAggregateIntegration(t, db, tenantID)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Campaign Aggregate Integration')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaign_alert_links
		(relation_id,tenant_id,campaign_id,alert_id,status,revision,reason,idempotency_key,created_by,updated_by)
		VALUES($1,$2,$3,'alert-integration-1','linked',1,'integration member','campaign-member-integration-key','seed','seed')`,
		uuid.NewString(), tenantID, campaignID); err != nil {
		t.Fatal(err)
	}

	handler := NewSystemHandler(nil, db, zap.NewNop())
	ctx := context.Background()
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "campaign-operator")
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-campaign-integration")
	httpRequest := httptest.NewRequest(http.MethodPost, "/campaigns/"+campaignID+"/actions", nil).WithContext(ctx)
	campaign := campaignDTO{CampaignID: campaignID, Status: "active", TsStart: 100, TsEnd: 200, Alerts: []string{"alert-integration-1"}}

	statusRequest := campaignActionRequest{
		ActionID: "campaign-status-change", Target: "推进调查",
		Metadata:   map[string]interface{}{"campaign_id": campaignID, "next_status": "investigating", "dry_run": false},
		Simulation: boolPointer(false), DryRun: boolPointer(false), ExpectedRevision: int64Pointer(0),
		Reason: "集成测试确认进入调查阶段", CompatibilityMode: true,
	}
	statusSHA, err := campaignCommandRequestSHA(campaignID, statusRequest)
	if err != nil {
		t.Fatal(err)
	}
	statusJob, err := handler.commitCampaignAggregateV2Command(
		ctx, httpRequest, statusRequest, campaignActionSpecs[statusRequest.ActionID], campaignID, campaign,
		"campaign-integration-status-key", statusSHA,
	)
	statusCompatibility, _ := statusJob.Result["compatibility_mode"].(bool)
	if err != nil || statusJob.Status != "succeeded" || statusJob.ResourceRevision != 1 || !statusCompatibility {
		t.Fatalf("status job=%+v err=%v", statusJob, err)
	}
	replay, err := handler.commitCampaignAggregateV2Command(
		ctx, httpRequest, statusRequest, campaignActionSpecs[statusRequest.ActionID], campaignID, campaign,
		"campaign-integration-status-key", statusSHA,
	)
	if err != nil || !replay.IdempotentReuse || replay.JobID != statusJob.JobID {
		t.Fatalf("status replay=%+v err=%v", replay, err)
	}

	stale := statusRequest
	stale.Metadata = map[string]interface{}{"campaign_id": campaignID, "next_status": "contained", "dry_run": false}
	stale.Reason = "陈旧版本不得覆盖当前调查状态"
	staleSHA, err := campaignCommandRequestSHA(campaignID, stale)
	if err != nil {
		t.Fatal(err)
	}
	_, err = handler.commitCampaignAggregateV2Command(
		ctx, httpRequest, stale, campaignActionSpecs[stale.ActionID], campaignID, campaign,
		"campaign-integration-stale-key", staleSHA,
	)
	var commandErr *campaignCommandError
	if !errors.As(err, &commandErr) || commandErr.code != "REVISION_CONFLICT" {
		t.Fatalf("stale command err=%v", err)
	}

	reportRequest := campaignActionRequest{
		ActionID: "campaign-report-generate", Target: "战役复盘报告",
		Metadata:   map[string]interface{}{"campaign_id": campaignID, "format": "pdf", "sections": []string{"证据链"}, "evidence_count": float64(1), "dry_run": false},
		Simulation: boolPointer(false), DryRun: boolPointer(false), ExpectedRevision: int64Pointer(1),
		Reason: "冻结当前成员并受理战役复盘报告",
	}
	reportSHA, err := campaignCommandRequestSHA(campaignID, reportRequest)
	if err != nil {
		t.Fatal(err)
	}
	reportJob, err := handler.commitCampaignAggregateV2Command(
		ctx, httpRequest, reportRequest, campaignActionSpecs[reportRequest.ActionID], campaignID, campaign,
		"campaign-integration-report-key", reportSHA,
	)
	if err != nil || reportJob.Status != "accepted" || reportJob.ResourceRevision != 2 {
		t.Fatalf("report job=%+v err=%v", reportJob, err)
	}

	var state, reportStatus, snapshotSHA, historyCompatibility, auditCompatibility string
	var revision int64
	var memberCount, historyCount, outboxCount, jobCount, reportCount, auditCount int
	if err := db.QueryRow(`SELECT s.status,s.state_version,s.member_count,
		(SELECT count(*) FROM campaign_aggregate_history WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_aggregate_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_action_jobs WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_reports WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='campaign'),
		(SELECT status FROM campaign_reports WHERE tenant_id=$1 LIMIT 1),
		(SELECT snapshot_sha256 FROM campaign_reports WHERE tenant_id=$1 LIMIT 1),
		(SELECT payload->>'compatibility_mode' FROM campaign_aggregate_history
		 WHERE tenant_id=$1 AND event_type='traffic.campaign.v2.StatusChanged' LIMIT 1),
		(SELECT detail->>'compatibility_mode' FROM audit_logs
		 WHERE tenant_id=$1 AND action='CAMPAIGN_STATUS_CHANGED' LIMIT 1)
		FROM campaign_workbench_state s WHERE s.tenant_id=$1 AND s.campaign_id=$2`, tenantID, campaignID).
		Scan(&state, &revision, &memberCount, &historyCount, &outboxCount, &jobCount, &reportCount, &auditCount,
			&reportStatus, &snapshotSHA, &historyCompatibility, &auditCompatibility); err != nil {
		t.Fatal(err)
	}
	if state != "investigating" || revision != 2 || memberCount != 1 || historyCount != 2 || outboxCount != 2 ||
		jobCount != 2 || reportCount != 1 || auditCount != 3 || reportStatus != "accepted" || len(snapshotSHA) != 64 ||
		historyCompatibility != "true" || auditCompatibility != "true" {
		t.Fatalf("facts state=%s revision=%d members=%d history=%d outbox=%d jobs=%d reports=%d audits=%d report_status=%s sha=%s compatibility=%s/%s",
			state, revision, memberCount, historyCount, outboxCount, jobCount, reportCount, auditCount,
			reportStatus, snapshotSHA, historyCompatibility, auditCompatibility)
	}
}

func TestCampaignMembershipV2PostgresIntegration(t *testing.T) {
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

	tenantID := "campaign-membership-integration-" + time.Now().UTC().Format("150405000000")
	campaignID := "campaign-membership-integration-1"
	alertID := "alert-membership-integration-1"
	cleanupCampaignAggregateIntegration(t, db, tenantID)
	defer cleanupCampaignAggregateIntegration(t, db, tenantID)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Campaign Membership Integration')`, tenantID); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	ctx := context.Background()
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "campaign-membership-operator")
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-campaign-membership-integration")
	linkHTTP := httptest.NewRequest(http.MethodPost, "/alerts/"+alertID+"/campaign-links", nil).WithContext(ctx)

	expectedCampaignRevision := int64(0)
	linkRequest := alertCampaignLinkRequest{
		CampaignID: campaignID, ExpectedRevision: int64Pointer(0),
		ExpectedCampaignRevision: &expectedCampaignRevision, Reason: "集成测试确认加入战役成员",
	}
	linkSHA, err := campaignMembershipRequestSHA(campaignMembershipLink, tenantID, alertID, linkRequest)
	if err != nil {
		t.Fatal(err)
	}
	link, err := handler.commitCampaignMembershipV2(
		ctx, linkHTTP, campaignMembershipLink, alertID, linkRequest,
		"campaign-membership-integration-link-key", linkSHA,
	)
	if err != nil || link.Status != "linked" || link.Revision != 1 || link.CampaignRevision != 1 {
		t.Fatalf("link=%+v err=%v", link, err)
	}
	replay, err := handler.commitCampaignMembershipV2(
		ctx, linkHTTP, campaignMembershipLink, alertID, linkRequest,
		"campaign-membership-integration-link-key", linkSHA,
	)
	if err != nil || !replay.IdempotentReuse || replay.RelationID != link.RelationID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}

	staleCampaignRevision := int64(0)
	staleRequest := alertCampaignLinkRequest{
		CampaignID: campaignID, ExpectedRevision: int64Pointer(1),
		ExpectedCampaignRevision: &staleCampaignRevision, Reason: "陈旧战役版本不得覆盖成员关系",
	}
	staleSHA, err := campaignMembershipRequestSHA(campaignMembershipUnlink, tenantID, alertID, staleRequest)
	if err != nil {
		t.Fatal(err)
	}
	_, err = handler.commitCampaignMembershipV2(
		ctx, linkHTTP, campaignMembershipUnlink, alertID, staleRequest,
		"campaign-membership-integration-stale-key", staleSHA,
	)
	var membershipErr *campaignMembershipCommandError
	if !errors.As(err, &membershipErr) || membershipErr.code != "CAMPAIGN_REVISION_CONFLICT" {
		t.Fatalf("stale membership err=%v", err)
	}

	expectedCampaignRevision = 1
	unlinkRequest := alertCampaignLinkRequest{
		CampaignID: campaignID, ExpectedRevision: int64Pointer(1),
		ExpectedCampaignRevision: &expectedCampaignRevision, Reason: "集成测试确认移出战役成员",
	}
	unlinkSHA, err := campaignMembershipRequestSHA(campaignMembershipUnlink, tenantID, alertID, unlinkRequest)
	if err != nil {
		t.Fatal(err)
	}
	unlinkHTTP := httptest.NewRequest(http.MethodDelete, "/alerts/"+alertID+"/campaign-links/"+campaignID, nil).WithContext(ctx)
	unlinked, err := handler.commitCampaignMembershipV2(
		ctx, unlinkHTTP, campaignMembershipUnlink, alertID, unlinkRequest,
		"campaign-membership-integration-unlink-key", unlinkSHA,
	)
	if err != nil || unlinked.Status != "unlinked" || unlinked.Revision != 2 || unlinked.CampaignRevision != 2 {
		t.Fatalf("unlink=%+v err=%v", unlinked, err)
	}

	var stateStatus, relationStatus string
	var stateRevision, relationRevision, relationCampaignRevision int64
	var memberCount, relationHistory, relationOutbox, aggregateHistory, aggregateOutbox, commands, audits int
	if err := db.QueryRow(`SELECT s.status,s.state_version,s.member_count,l.status,l.revision,l.campaign_revision,
		(SELECT count(*) FROM campaign_alert_link_history WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_alert_link_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_aggregate_history WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_aggregate_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_membership_commands WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='campaign_alert_link')
		FROM campaign_workbench_state s JOIN campaign_alert_links l
		  ON l.tenant_id=s.tenant_id AND l.campaign_id=s.campaign_id
		WHERE s.tenant_id=$1 AND s.campaign_id=$2 AND l.alert_id=$3`, tenantID, campaignID, alertID).
		Scan(&stateStatus, &stateRevision, &memberCount, &relationStatus, &relationRevision,
			&relationCampaignRevision, &relationHistory, &relationOutbox, &aggregateHistory,
			&aggregateOutbox, &commands, &audits); err != nil {
		t.Fatal(err)
	}
	if stateStatus != "active" || stateRevision != 2 || memberCount != 0 || relationStatus != "unlinked" ||
		relationRevision != 2 || relationCampaignRevision != 2 || relationHistory != 2 || relationOutbox != 2 ||
		aggregateHistory != 2 || aggregateOutbox != 2 || commands != 2 || audits != 3 {
		t.Fatalf("state=%s/%d members=%d relation=%s/%d/%d histories=%d/%d outboxes=%d/%d commands=%d audits=%d",
			stateStatus, stateRevision, memberCount, relationStatus, relationRevision, relationCampaignRevision,
			relationHistory, aggregateHistory, relationOutbox, aggregateOutbox, commands, audits)
	}
}

func cleanupCampaignAggregateIntegration(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	for _, query := range []string{
		`DELETE FROM campaign_membership_backfill_items WHERE tenant_id=$1`,
		`DELETE FROM campaign_membership_backfill_campaigns WHERE tenant_id=$1`,
		`DELETE FROM campaign_membership_backfill_runs WHERE tenant_id=$1`,
		`DELETE FROM campaign_merge_items WHERE tenant_id=$1`,
		`DELETE FROM campaign_merge_receipts WHERE tenant_id=$1`,
		`DELETE FROM campaign_membership_commands WHERE tenant_id=$1`,
		`DELETE FROM campaign_aggregate_outbox WHERE tenant_id=$1`,
		`DELETE FROM campaign_aggregate_history WHERE tenant_id=$1`,
		`DELETE FROM campaign_reports WHERE tenant_id=$1`,
		`DELETE FROM campaign_action_jobs WHERE tenant_id=$1`,
		`DELETE FROM campaign_alert_link_outbox WHERE tenant_id=$1`,
		`DELETE FROM campaign_alert_link_history WHERE tenant_id=$1`,
		`DELETE FROM campaign_alert_links WHERE tenant_id=$1`,
		`DELETE FROM campaign_workbench_state WHERE tenant_id=$1`,
		`DELETE FROM audit_logs WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE tenant_id=$1`,
	} {
		if _, err := db.Exec(query, tenantID); err != nil {
			t.Errorf("cleanup %q: %v", query, err)
		}
	}
}

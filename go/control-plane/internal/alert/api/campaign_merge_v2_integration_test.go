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

func TestCampaignMergeV2PostgresIntegration(t *testing.T) {
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

	tenantID := "campaign-merge-integration-" + time.Now().UTC().Format("150405000000")
	sourceCampaignID := "campaign-merge-source"
	targetCampaignID := "campaign-merge-target"
	cleanupCampaignAggregateIntegration(t, db, tenantID)
	defer cleanupCampaignAggregateIntegration(t, db, tenantID)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Campaign Merge Integration')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaign_workbench_state
		(tenant_id,campaign_id,assignee,status,state_version,member_count,updated_by)
		VALUES ($1,$2,'source-owner','active',2,3,'seed'),
		       ($1,$3,'target-owner','investigating',5,2,'seed')`, tenantID, sourceCampaignID, targetCampaignID); err != nil {
		t.Fatal(err)
	}
	seedMergeRelation := func(campaignID, alertID, status string, revision, campaignRevision int64) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO campaign_alert_links
			(relation_id,tenant_id,campaign_id,alert_id,status,revision,campaign_revision,reason,
			 idempotency_key,created_by,updated_by)
			VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,'integration seed',$8,'seed','seed')`,
			uuid.NewString(), tenantID, campaignID, alertID, status, revision, campaignRevision,
			"merge-seed-"+campaignID+"-"+alertID); err != nil {
			t.Fatal(err)
		}
	}
	seedMergeRelation(sourceCampaignID, "alert-a", "linked", 1, 0)
	seedMergeRelation(sourceCampaignID, "alert-b", "linked", 1, 0)
	seedMergeRelation(sourceCampaignID, "alert-c", "linked", 1, 0)
	seedMergeRelation(targetCampaignID, "alert-a", "linked", 1, 5)
	seedMergeRelation(targetCampaignID, "alert-b", "unlinked", 1, 4)
	seedMergeRelation(targetCampaignID, "alert-d", "linked", 1, 5)

	handler := NewSystemHandler(nil, db, zap.NewNop())
	ctx := context.Background()
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "campaign-merge-operator")
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-campaign-merge-integration")
	httpRequest := httptest.NewRequest(http.MethodPost, "/campaigns/"+sourceCampaignID+"/actions", nil).WithContext(ctx)
	source := campaignDTO{TenantID: tenantID, CampaignID: sourceCampaignID, Status: "active"}
	target := campaignDTO{TenantID: tenantID, CampaignID: targetCampaignID, Status: "investigating"}
	request := campaignActionRequest{
		ActionID: "campaign-merge", Target: "merge source into target",
		Metadata: map[string]interface{}{
			"campaign_id": sourceCampaignID, "target_campaign_id": targetCampaignID, "dry_run": false,
		},
		Simulation: boolPointer(false), DryRun: boolPointer(false), ExpectedRevision: int64Pointer(2),
		TargetExpectedRevision: int64Pointer(4), Reason: "集成测试合并战役并迁移成员关系",
	}
	staleSHA, err := campaignCommandRequestSHA(sourceCampaignID, request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = handler.commitCampaignMergeV2Command(ctx, httpRequest, request,
		campaignActionSpecs[request.ActionID], source, target, "campaign-merge-stale-target-key", staleSHA)
	var commandErr *campaignCommandError
	if !errors.As(err, &commandErr) || commandErr.code != "TARGET_REVISION_CONFLICT" {
		t.Fatalf("stale target merge err=%v", err)
	}

	request.TargetExpectedRevision = int64Pointer(5)
	requestSHA, err := campaignCommandRequestSHA(sourceCampaignID, request)
	if err != nil {
		t.Fatal(err)
	}
	job, err := handler.commitCampaignMergeV2Command(ctx, httpRequest, request,
		campaignActionSpecs[request.ActionID], source, target, "campaign-merge-integration-key", requestSHA)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "succeeded" || job.ResourceRevision != 3 {
		t.Fatalf("merge job=%+v", job)
	}
	replay, err := handler.commitCampaignMergeV2Command(ctx, httpRequest, request,
		campaignActionSpecs[request.ActionID], source, target, "campaign-merge-integration-key", requestSHA)
	if err != nil || !replay.IdempotentReuse || replay.JobID != job.JobID {
		t.Fatalf("merge replay=%+v err=%v", replay, err)
	}

	var sourceStatus, targetStatus string
	var sourceRevision, targetRevision int64
	var sourceMembers, targetMembers int
	if err := db.QueryRow(`SELECT
		(SELECT status FROM campaign_workbench_state WHERE tenant_id=$1 AND campaign_id=$2),
		(SELECT state_version FROM campaign_workbench_state WHERE tenant_id=$1 AND campaign_id=$2),
		(SELECT member_count FROM campaign_workbench_state WHERE tenant_id=$1 AND campaign_id=$2),
		(SELECT status FROM campaign_workbench_state WHERE tenant_id=$1 AND campaign_id=$3),
		(SELECT state_version FROM campaign_workbench_state WHERE tenant_id=$1 AND campaign_id=$3),
		(SELECT member_count FROM campaign_workbench_state WHERE tenant_id=$1 AND campaign_id=$3)`,
		tenantID, sourceCampaignID, targetCampaignID).Scan(
		&sourceStatus, &sourceRevision, &sourceMembers, &targetStatus, &targetRevision, &targetMembers); err != nil {
		t.Fatal(err)
	}
	if sourceStatus != "closed" || sourceRevision != 3 || sourceMembers != 0 ||
		targetStatus != "investigating" || targetRevision != 6 || targetMembers != 4 {
		t.Fatalf("states source=%s/%d/%d target=%s/%d/%d", sourceStatus, sourceRevision, sourceMembers,
			targetStatus, targetRevision, targetMembers)
	}

	var receiptCount, itemCount, moved, relinked, deduplicated, relationHistory, relationOutbox, aggregateHistory, aggregateOutbox, jobCount int
	var manifestSHA string
	if err := db.QueryRow(`SELECT
		(SELECT count(*) FROM campaign_merge_receipts WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_merge_items WHERE tenant_id=$1),
		(SELECT moved_count FROM campaign_merge_receipts WHERE tenant_id=$1),
		(SELECT relinked_count FROM campaign_merge_receipts WHERE tenant_id=$1),
		(SELECT deduplicated_count FROM campaign_merge_receipts WHERE tenant_id=$1),
		(SELECT manifest_sha256 FROM campaign_merge_receipts WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_alert_link_history WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_alert_link_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_aggregate_history WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_aggregate_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_action_jobs WHERE tenant_id=$1)`, tenantID).Scan(
		&receiptCount, &itemCount, &moved, &relinked, &deduplicated, &manifestSHA,
		&relationHistory, &relationOutbox, &aggregateHistory, &aggregateOutbox, &jobCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 1 || itemCount != 3 || moved != 1 || relinked != 1 || deduplicated != 1 ||
		len(manifestSHA) != 64 || relationHistory != 5 || relationOutbox != 5 ||
		aggregateHistory != 2 || aggregateOutbox != 2 || jobCount != 1 {
		t.Fatalf("receipt=%d items=%d outcomes=%d/%d/%d sha=%s events=%d/%d/%d/%d jobs=%d",
			receiptCount, itemCount, moved, relinked, deduplicated, manifestSHA,
			relationHistory, relationOutbox, aggregateHistory, aggregateOutbox, jobCount)
	}
	var inconsistentTargetEventCount int
	if err := db.QueryRow(`SELECT count(*) FROM campaign_alert_link_outbox
		WHERE tenant_id=$1 AND payload->>'campaign_id'=$2
		  AND (payload->>'member_count')::integer<>4`, tenantID, targetCampaignID).
		Scan(&inconsistentTargetEventCount); err != nil {
		t.Fatal(err)
	}
	if inconsistentTargetEventCount != 0 {
		t.Fatalf("target membership events with non-final member_count=%d", inconsistentTargetEventCount)
	}

	postMergeRequest := campaignActionRequest{
		ActionID: "campaign-status-change", Target: "reopen merged source",
		Metadata:   map[string]interface{}{"campaign_id": sourceCampaignID, "next_status": "investigating", "dry_run": false},
		Simulation: boolPointer(false), DryRun: boolPointer(false), ExpectedRevision: int64Pointer(3),
		Reason: "合并后的源战役不得重新开启",
	}
	postMergeSHA, err := campaignCommandRequestSHA(sourceCampaignID, postMergeRequest)
	if err != nil {
		t.Fatal(err)
	}
	_, err = handler.commitCampaignAggregateV2Command(ctx, httpRequest, postMergeRequest,
		campaignActionSpecs[postMergeRequest.ActionID], sourceCampaignID, source,
		"campaign-merge-post-command-key", postMergeSHA)
	if !errors.As(err, &commandErr) || commandErr.code != "CAMPAIGN_MERGED" {
		t.Fatalf("post-merge source command err=%v", err)
	}
}

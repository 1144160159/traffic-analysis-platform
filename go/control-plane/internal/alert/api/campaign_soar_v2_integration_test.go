package api

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

type sentinelCampaignSOARExecutor struct {
	executeCalls    int
	compensateCalls int
}

func (e *sentinelCampaignSOARExecutor) Execute(_ context.Context, request CampaignSOARExecutionRequest) (CampaignSOARReceipt, error) {
	e.executeCalls++
	return CampaignSOARReceipt{
		Provider: "sentinel-soar", ProviderReceiptID: request.JobID + ":execute",
		Status: "succeeded", ExternalEffect: true,
		Detail: map[string]interface{}{"effect_id": "containment-1", "target": request.Target},
	}, nil
}

func (e *sentinelCampaignSOARExecutor) Compensate(_ context.Context, request CampaignSOARExecutionRequest, prior CampaignSOARReceipt) (CampaignSOARReceipt, error) {
	e.compensateCalls++
	if prior.ProviderReceiptID != request.JobID+":execute" {
		return CampaignSOARReceipt{}, fmt.Errorf("unexpected prior receipt %q", prior.ProviderReceiptID)
	}
	return CampaignSOARReceipt{
		Provider: "sentinel-soar", ProviderReceiptID: request.JobID + ":compensate",
		Status: "succeeded", ExternalEffect: true,
		Detail: map[string]interface{}{"reversed_effect_id": "containment-1"},
	}, nil
}

func TestCampaignSOARV2PostgresLifecycleIntegration(t *testing.T) {
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
	tenantID := "campaign-soar-integration-" + time.Now().UTC().Format("150405000000")
	campaignID := "campaign-soar-1"
	cleanupCampaignSOARIntegration(t, db, tenantID)
	defer cleanupCampaignSOARIntegration(t, db, tenantID)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Campaign SOAR Integration')`, tenantID); err != nil {
		t.Fatal(err)
	}

	executor := &sentinelCampaignSOARExecutor{}
	handler := NewSystemHandler(nil, db, zap.NewNop())
	handler.SetCampaignAggregateV2FeatureFlag(true)
	handler.SetCampaignSOARExecutor(executor)
	requesterCtx := campaignSOARContext(tenantID, "soar-requester", authmodel.ScopePlaybookExecute)
	httpRequest := httptest.NewRequest(http.MethodPost, "/campaigns/"+campaignID+"/actions", nil).WithContext(requesterCtx)
	command := campaignActionRequest{
		ActionID: "campaign-soar-response", Target: "asset-critical-1",
		Metadata: map[string]interface{}{
			"campaign_id": campaignID, "playbook_id": "contain-critical-host", "dry_run": false,
		},
		Simulation: boolPointer(false), DryRun: boolPointer(false), ExpectedRevision: int64Pointer(0),
		Reason: "集成测试受理真实SOAR审批流程",
	}
	requestSHA, err := campaignCommandRequestSHA(campaignID, command)
	if err != nil {
		t.Fatal(err)
	}
	job, err := handler.commitCampaignAggregateV2Command(requesterCtx, httpRequest, command,
		campaignActionSpecs[command.ActionID], campaignID, campaignDTO{CampaignID: campaignID, Status: "active"},
		"campaign-soar-request-key-0001", requestSHA)
	if err != nil || job.Status != "pending_approval" || job.ResourceRevision != 1 {
		t.Fatalf("SOAR request job=%+v err=%v", job, err)
	}

	selfApproval := campaignSOARHandlerRequest(http.MethodPost,
		"/campaigns/"+campaignID+"/soar-jobs/"+job.JobID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"申请人不得批准自己的SOAR请求"}`,
		tenantID, campaignID, job.JobID, "soar-requester", "campaign-soar-self-approval-001", authmodel.ScopePlaybookApprove)
	selfRecorder := httptest.NewRecorder()
	handler.DecideCampaignSOARJob(selfRecorder, selfApproval)
	if selfRecorder.Code != http.StatusForbidden {
		t.Fatalf("self approval status=%d body=%s", selfRecorder.Code, selfRecorder.Body.String())
	}
	staleApproval := campaignSOARHandlerRequest(http.MethodPost,
		"/campaigns/"+campaignID+"/soar-jobs/"+job.JobID+"/approval",
		`{"decision":"approve","expected_revision":2,"reason":"陈旧或超前版本不得批准SOAR请求"}`,
		tenantID, campaignID, job.JobID, "soar-approver", "campaign-soar-stale-approval-01", authmodel.ScopePlaybookApprove)
	staleRecorder := httptest.NewRecorder()
	handler.DecideCampaignSOARJob(staleRecorder, staleApproval)
	if staleRecorder.Code != http.StatusConflict {
		t.Fatalf("stale approval status=%d body=%s", staleRecorder.Code, staleRecorder.Body.String())
	}
	crossTenantRead := campaignSOARHandlerRequest(http.MethodGet,
		"/campaigns/"+campaignID+"/soar-jobs/"+job.JobID, "",
		"another-tenant", campaignID, job.JobID, "reader-a", "campaign-soar-cross-tenant-01", authmodel.ScopeCampaignRead)
	crossTenantRecorder := httptest.NewRecorder()
	handler.GetCampaignSOARJob(crossTenantRecorder, crossTenantRead)
	if crossTenantRecorder.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant read status=%d body=%s", crossTenantRecorder.Code, crossTenantRecorder.Body.String())
	}

	approval := campaignSOARHandlerRequest(http.MethodPost,
		"/campaigns/"+campaignID+"/soar-jobs/"+job.JobID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"独立审批人确认执行隔离剧本"}`,
		tenantID, campaignID, job.JobID, "soar-approver", "campaign-soar-approval-key-001", authmodel.ScopePlaybookApprove)
	approvalRecorder := httptest.NewRecorder()
	handler.DecideCampaignSOARJob(approvalRecorder, approval)
	if approvalRecorder.Code != http.StatusAccepted {
		t.Fatalf("approval status=%d body=%s", approvalRecorder.Code, approvalRecorder.Body.String())
	}
	approvalReplay := campaignSOARHandlerRequest(http.MethodPost,
		"/campaigns/"+campaignID+"/soar-jobs/"+job.JobID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"独立审批人确认执行隔离剧本"}`,
		tenantID, campaignID, job.JobID, "soar-approver", "campaign-soar-approval-key-001", authmodel.ScopePlaybookApprove)
	approvalReplayRecorder := httptest.NewRecorder()
	handler.DecideCampaignSOARJob(approvalReplayRecorder, approvalReplay)
	if approvalReplayRecorder.Code != http.StatusAccepted {
		t.Fatalf("approval replay status=%d body=%s", approvalReplayRecorder.Code, approvalReplayRecorder.Body.String())
	}
	if err := handler.processNextCampaignSOAR(requesterCtx); err != nil {
		t.Fatal(err)
	}
	completed, err := handler.loadCampaignSOARJob(requesterCtx, tenantID, campaignID, job.JobID, false)
	if err != nil || completed.Status != "completed" || completed.ExecutorStatus != "succeeded" || completed.Revision != 3 {
		t.Fatalf("completed job=%+v err=%v", completed, err)
	}
	if executor.executeCalls != 1 {
		t.Fatalf("execute calls=%d", executor.executeCalls)
	}
	selfCompensation := campaignSOARHandlerRequest(http.MethodPost,
		"/campaigns/"+campaignID+"/soar-jobs/"+job.JobID+"/compensate",
		`{"expected_revision":3,"reason":"原始申请人不得批准撤销外部隔离效果"}`,
		tenantID, campaignID, job.JobID, "soar-requester", "campaign-soar-self-compensate-1", authmodel.ScopePlaybookApprove)
	selfCompensationRecorder := httptest.NewRecorder()
	handler.CompensateCampaignSOARJob(selfCompensationRecorder, selfCompensation)
	if selfCompensationRecorder.Code != http.StatusForbidden {
		t.Fatalf("self compensation status=%d body=%s", selfCompensationRecorder.Code, selfCompensationRecorder.Body.String())
	}

	compensation := campaignSOARHandlerRequest(http.MethodPost,
		"/campaigns/"+campaignID+"/soar-jobs/"+job.JobID+"/compensate",
		`{"expected_revision":3,"reason":"独立审批人确认撤销外部隔离效果"}`,
		tenantID, campaignID, job.JobID, "soar-compensator", "campaign-soar-compensate-001", authmodel.ScopePlaybookApprove)
	compensationRecorder := httptest.NewRecorder()
	handler.CompensateCampaignSOARJob(compensationRecorder, compensation)
	if compensationRecorder.Code != http.StatusAccepted {
		t.Fatalf("compensation status=%d body=%s", compensationRecorder.Code, compensationRecorder.Body.String())
	}
	if err := handler.processNextCampaignSOAR(requesterCtx); err != nil {
		t.Fatal(err)
	}
	compensated, err := handler.loadCampaignSOARJob(requesterCtx, tenantID, campaignID, job.JobID, false)
	if err != nil || compensated.Status != "compensated" || compensated.ExecutorStatus != "compensated" || compensated.Revision != 5 {
		t.Fatalf("compensated job=%+v err=%v", compensated, err)
	}
	if executor.compensateCalls != 1 {
		t.Fatalf("compensate calls=%d", executor.compensateCalls)
	}

	var actionStatus string
	var stateRevision int64
	var approvals, controls, receipts, history, outbox, audits int
	if err := db.QueryRow(`SELECT
		(SELECT status FROM campaign_action_jobs WHERE tenant_id=$1 AND job_id=$2),
		(SELECT state_version FROM campaign_workbench_state WHERE tenant_id=$1 AND campaign_id=$3),
		(SELECT count(*) FROM campaign_soar_approvals WHERE tenant_id=$1 AND job_id=$2),
		(SELECT count(*) FROM campaign_soar_control_requests WHERE tenant_id=$1 AND job_id=$2),
		(SELECT count(*) FROM campaign_soar_execution_receipts WHERE tenant_id=$1 AND job_id=$2),
		(SELECT count(*) FROM campaign_aggregate_history WHERE tenant_id=$1 AND campaign_id=$3),
		(SELECT count(*) FROM campaign_aggregate_outbox WHERE tenant_id=$1 AND aggregate_id=$3),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_id IN ($2,$3))`,
		tenantID, job.JobID, campaignID).Scan(&actionStatus, &stateRevision, &approvals,
		&controls, &receipts, &history, &outbox, &audits); err != nil {
		t.Fatal(err)
	}
	if actionStatus != "compensated" || stateRevision != 3 || approvals != 1 || controls != 1 ||
		receipts != 2 || history != 3 || outbox != 3 || audits < 5 {
		t.Fatalf("facts action=%s state_revision=%d approvals=%d controls=%d receipts=%d history=%d outbox=%d audits=%d",
			actionStatus, stateRevision, approvals, controls, receipts, history, outbox, audits)
	}

	// An environment without a configured provider may accept and independently approve a
	// request, but it must remain visibly queued and must never synthesize a success receipt.
	noProviderCampaignID := "campaign-soar-no-provider"
	noProviderHandler := NewSystemHandler(nil, db, zap.NewNop())
	noProviderHandler.SetCampaignAggregateV2FeatureFlag(true)
	noProviderCommand := command
	noProviderCommand.Metadata = map[string]interface{}{
		"campaign_id": noProviderCampaignID, "playbook_id": "contain-critical-host", "dry_run": false,
	}
	noProviderSHA, err := campaignCommandRequestSHA(noProviderCampaignID, noProviderCommand)
	if err != nil {
		t.Fatal(err)
	}
	noProviderJob, err := noProviderHandler.commitCampaignAggregateV2Command(requesterCtx, httpRequest,
		noProviderCommand, campaignActionSpecs[noProviderCommand.ActionID], noProviderCampaignID,
		campaignDTO{CampaignID: noProviderCampaignID, Status: "active"},
		"campaign-soar-no-provider-request-01", noProviderSHA)
	if err != nil {
		t.Fatal(err)
	}
	noProviderApproval := campaignSOARHandlerRequest(http.MethodPost,
		"/campaigns/"+noProviderCampaignID+"/soar-jobs/"+noProviderJob.JobID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"独立审批但当前环境未配置SOAR provider"}`,
		tenantID, noProviderCampaignID, noProviderJob.JobID, "soar-approver",
		"campaign-soar-no-provider-approval-01", authmodel.ScopePlaybookApprove)
	noProviderRecorder := httptest.NewRecorder()
	noProviderHandler.DecideCampaignSOARJob(noProviderRecorder, noProviderApproval)
	if noProviderRecorder.Code != http.StatusAccepted {
		t.Fatalf("no-provider approval status=%d body=%s", noProviderRecorder.Code, noProviderRecorder.Body.String())
	}
	if err := noProviderHandler.processNextCampaignSOAR(requesterCtx); err != nil {
		t.Fatal(err)
	}
	noProviderState, err := noProviderHandler.loadCampaignSOARJob(requesterCtx, tenantID,
		noProviderCampaignID, noProviderJob.JobID, false)
	if err != nil {
		t.Fatal(err)
	}
	var noProviderReceipts int
	if err := db.QueryRow(`SELECT count(*) FROM campaign_soar_execution_receipts WHERE tenant_id=$1 AND job_id=$2`,
		tenantID, noProviderJob.JobID).Scan(&noProviderReceipts); err != nil {
		t.Fatal(err)
	}
	if noProviderState.Status != "approved_awaiting_executor" || noProviderState.ExecutorStatus != "not_configured" || noProviderReceipts != 0 {
		t.Fatalf("no-provider state=%s executor=%s receipts=%d", noProviderState.Status,
			noProviderState.ExecutorStatus, noProviderReceipts)
	}
}

func campaignSOARContext(tenantID, actor string, permissions ...string) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, actor)
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-campaign-soar-integration")
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	return ctx
}

func campaignSOARHandlerRequest(method, path, body, tenantID, campaignID, jobID, actor, idempotencyKey string, permissions ...string) *http.Request {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	request = request.WithContext(campaignSOARContext(tenantID, actor, permissions...))
	return mux.SetURLVars(request, map[string]string{"id": campaignID, "job_id": jobID})
}

func cleanupCampaignSOARIntegration(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	statements := []string{
		`DELETE FROM campaign_soar_execution_receipts WHERE tenant_id=$1`,
		`DELETE FROM campaign_soar_approvals WHERE tenant_id=$1`,
		`DELETE FROM campaign_soar_control_requests WHERE tenant_id=$1`,
		`DELETE FROM campaign_soar_jobs WHERE tenant_id=$1`,
		`DELETE FROM campaign_aggregate_outbox WHERE tenant_id=$1`,
		`DELETE FROM campaign_aggregate_history WHERE tenant_id=$1`,
		`DELETE FROM campaign_action_jobs WHERE tenant_id=$1`,
		`DELETE FROM campaign_workbench_state WHERE tenant_id=$1`,
		`DELETE FROM audit_logs WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE tenant_id=$1`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement, tenantID); err != nil {
			t.Fatalf("cleanup %q: %v", statement, err)
		}
	}
}

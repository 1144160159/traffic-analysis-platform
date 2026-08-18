//go:build integration

// live PG 集成测试:lease 过期回收(回 PENDING+重入队+再领取)与
// 报告自动化(AUTO_ASYNC 候选 + 下载票一次性消费)。
package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

func TestLivePGLeaseExpiryAndAutoReport(t *testing.T) {
	db := liveDBP2(t)
	repo := repository.NewRepo(db)
	ctx := context.Background()
	tenant := "integration-lease-" + uuid.NewString()[:8]
	tenantTail := tenant[len(tenant)-8:]

	// ---------- 种子:经真实物化(带队列行) ----------
	defID := uuid.NewString()
	planID := uuid.NewString()
	triggerID := uuid.NewString()
	seedExec(t, db, ctx, `
		INSERT INTO analysis_task_definitions(id, tenant_id, name, state, owner, revision, created_by)
		VALUES($1,$2,$3,'ACTIVE',$2,1,$2)`, defID, tenant, "def-"+defID[:6])
	spec := "spec-" + tenantTail
	seedExec(t, db, ctx, `
		INSERT INTO analysis_plan_revisions(id, tenant_id, task_definition_id, plan_revision, plan_source, source_kind,
			source_spec, selected_feature_ids, feature_set_id, encrypted_recognition_model_ref, threat_detector_refs,
			rule_refs, machine_summary_schema_ref, stage_dag, completion_policy, resource_budget, catalog_revision,
			selection_origins, canonicalization_version, execution_spec_sha256, plan_revision_sha256, created_by)
		VALUES($1,$2,$3,1,'AUTO_DEFAULT','PCAP_REPLAY','{}','[]','fs-1','', '[]','[]','summary-v1','{}','{}','{"cpu":2}',1,'[]','v1',$4,$5,$2)`,
		planID, tenant, defID, spec, "plan-sha-"+spec[:6])
	seedExec(t, db, ctx, `
		INSERT INTO analysis_trigger_instances(id, tenant_id, identity_kind, canonical_identity_hash, request_sha256,
			state, trigger_kind, task_definition_id, plan_revision, actor)
		VALUES($1,$2,'actor',$3,$4,'PENDING_MATERIALIZATION','ON_DEMAND',$5,1,$2)`,
		triggerID, tenant, "cid-"+spec, "req-"+spec, defID)
	receipt, replayed, err := repo.MaterializeAnalysisTaskAtomic(ctx, repository.MaterializeCommand{
		TenantID: tenant, IdentityKind: "actor", CanonicalIdentityHash: "cid-" + spec,
		RequestSHA256: "req-" + spec, TriggerInstanceID: triggerID, TriggerKind: "ON_DEMAND",
		WindowStartMs: time.Now().UnixMilli(), WindowEndMs: time.Now().Add(10 * time.Minute).UnixMilli(),
		TaskDefinitionID: defID, PlanRevision: 1, ExecutionSpecSHA256: spec,
		EffectiveClass: "BASELINE", EffectivePolicySHA256: "policy-1",
		ResourcePool: "analysis-cpu", ResourceVectorJSON: []byte(`{"cpu":2}`),
		QueueCostMilli: 2000, ExpiresAt: time.Now().Add(5 * time.Minute),
		NodesJSON:      service.DefaultNodeExactSet(),
		PlanSpecJSON:   []byte(`{}`),
	})
	if err != nil || replayed || receipt.RunID == "" {
		t.Fatalf("materialize seed: %+v replayed=%v err=%v", receipt, replayed, err)
	}
	runID := receipt.RunID

	// ---------- 1. 领取后立即过期(leaseTTL=1ms)→ 回 PENDING 重入队 ----------
	lease, err := repo.ClaimStageLeaseAtomic(ctx, tenant, time.Millisecond, time.Now())
	if err != nil || lease == nil || lease.RunID != runID {
		t.Fatalf("claim: %+v err=%v", lease, err)
	}
	time.Sleep(20 * time.Millisecond)
	recovered, err := repo.ExpireStageLeasesAtomic(ctx, time.Now(), 20)
	if err != nil || recovered != 1 {
		t.Fatalf("expire sweep: recovered=%d err=%v", recovered, err)
	}
	var st, qst string
	var token interface{}
	if err := db.QueryRowContext(ctx, `SELECT state, fencing_token FROM analysis_stage_attempts WHERE id=$1`, lease.AttemptID).Scan(&st, &token); err != nil || st != "PENDING" || token != nil {
		t.Fatalf("attempt after expiry: state=%s token=%v err=%v", st, token, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT state FROM analysis_stage_queue WHERE id=$1`, lease.QueueID).Scan(&qst); err != nil || qst != "READY" {
		t.Fatalf("queue after expiry: %s err=%v", qst, err)
	}
	// 再领取成功(新 token)
	lease2, err := repo.ClaimStageLeaseAtomic(ctx, tenant, 5*time.Minute, time.Now())
	if err != nil || lease2 == nil || lease2.RunID != runID || lease2.FencingToken == lease.FencingToken {
		t.Fatalf("re-claim: %+v err=%v", lease2, err)
	}

	// ---------- 2. 下载票:签发→消费→一次性 ----------
	reportID := uuid.NewString()
	seedExec(t, db, ctx, `
		INSERT INTO analysis_human_reports(id, tenant_id, run_id, summary_sha256, template_revision, locale, state, object_key)
		VALUES($1,$2,$3,'sum-sha-1','default-v1','zh-CN','AVAILABLE','reports/test/object.html')`,
		reportID, tenant, runID)
	ticketID, _, err := repo.IssueDownloadTicketAtomic(ctx, tenant, reportID, "ticket-sha", 5*time.Minute, "op")
	if err != nil || ticketID == "" {
		t.Fatalf("issue ticket: %v", err)
	}
	key, err := repo.ConsumeDownloadTicketAtomic(ctx, tenant, reportID, ticketID)
	if err != nil || key != "reports/test/object.html" {
		t.Fatalf("consume ticket: key=%q err=%v", key, err)
	}
	if _, err := repo.ConsumeDownloadTicketAtomic(ctx, tenant, reportID, ticketID); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("second consume must be rejected, got %v", err)
	}
	otherReport := uuid.NewString()
	seedExec(t, db, ctx, `
		INSERT INTO analysis_human_reports(id, tenant_id, run_id, summary_sha256, template_revision, locale, state, object_key)
		VALUES($1,$2,$3,'sum-sha-2','default-v1','zh-CN','AVAILABLE','reports/test/o2.html')`,
		otherReport, tenant, runID)
	t2, _, err := repo.IssueDownloadTicketAtomic(ctx, tenant, otherReport, "ticket-sha-2", 5*time.Minute, "op")
	if err != nil {
		t.Fatalf("issue ticket 2: %v", err)
	}
	if _, err := repo.ConsumeDownloadTicketAtomic(ctx, tenant, reportID, t2); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("cross-report ticket must be rejected, got %v", err)
	}

	// ---------- 3. AUTO_ASYNC 候选 ----------
	// 步骤 2 为同一 run 种了两条报告行:清理后 run 恢复"无报告行"候选状态
	seedExec(t, db, ctx, `DELETE FROM analysis_report_download_tickets WHERE tenant_id=$1`, tenant)
	seedExec(t, db, ctx, `DELETE FROM analysis_human_reports WHERE tenant_id=$1`, tenant)
	seedExec(t, db, ctx, `
		INSERT INTO analysis_human_report_policies(id, tenant_id, task_definition_id, revision, mode, template_revision, locale, retention_days, policy_sha256)
		VALUES($1,$2,$3,1,'AUTO_ASYNC','default-v1','zh-CN',30,'pol-sha')`,
		uuid.NewString(), tenant, defID)
	seedExec(t, db, ctx, `
		INSERT INTO analysis_machine_summaries(tenant_id, run_id, finding_conclusion, risk_severity, completeness,
			integrity_state, scope, key_findings, limitations, evidence_manifest_hash, closure_manifest_hash, canonical_sha256, created_at)
		VALUES($1,$2,'THREAT_FOUND','MEDIUM','COMPLETE','VERIFIED','{}'::jsonb,'{}'::jsonb,'{}'::jsonb,'emh','cmh','sum-sha-1',now())`,
		tenant, runID)
	seedExec(t, db, ctx, `UPDATE analysis_runs SET state='SUCCEEDED' WHERE id=$1`, runID)
	cands, err := repo.NextAutoReportCandidates(ctx, 10)
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	found := false
	for _, c := range cands {
		if c.RunID == runID && c.TemplateRevision == "default-v1" && c.Locale == "zh-CN" {
			found = true
		}
	}
	if !found {
		t.Fatalf("run not in auto candidates: %+v", cands)
	}
	// 请求后不再候选(报告行存在)
	svc := service.NewHumanReportService(repo)
	if _, _, err := svc.RequestReport(ctx, tenant, runID, "default-v1", "zh-CN"); err != nil {
		t.Fatalf("request report: %v", err)
	}
	cands2, err := repo.NextAutoReportCandidates(ctx, 10)
	if err != nil {
		t.Fatalf("candidates 2: %v", err)
	}
	for _, c := range cands2 {
		if c.RunID == runID {
			t.Fatalf("run still candidate after request")
		}
	}

	// ---------- 清理 ----------
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM analysis_report_download_tickets WHERE tenant_id=$1`,
			`DELETE FROM analysis_human_reports WHERE tenant_id=$1`,
			`DELETE FROM analysis_human_report_policies WHERE tenant_id=$1`,
			`DELETE FROM analysis_machine_summaries WHERE run_id IN (SELECT id FROM analysis_runs WHERE tenant_id=$1)`,
			`DELETE FROM analysis_stage_queue WHERE tenant_id=$1`,
			`DELETE FROM analysis_stage_receipts WHERE tenant_id=$1`,
			`DELETE FROM analysis_business_phase_projections WHERE run_id IN (SELECT id FROM analysis_runs WHERE tenant_id=$1)`,
			`DELETE FROM analysis_stage_attempts WHERE tenant_id=$1`,
			`DELETE FROM analysis_admission_reservations WHERE tenant_id=$1`,
			`DELETE FROM analysis_drr_state WHERE tenant_id=$1`,
			`DELETE FROM analysis_receipts WHERE tenant_id=$1`,
			`DELETE FROM analysis_outbox WHERE key IN (SELECT id::text FROM analysis_runs WHERE tenant_id=$1)`,
			`DELETE FROM analysis_runs WHERE tenant_id=$1`,
			`DELETE FROM analysis_tasks WHERE tenant_id=$1`,
			`DELETE FROM analysis_trigger_instances WHERE tenant_id=$1`,
			`DELETE FROM analysis_plan_revisions WHERE tenant_id=$1`,
			`DELETE FROM analysis_history WHERE tenant_id=$1`,
			`DELETE FROM analysis_task_definitions WHERE tenant_id=$1`,
			`DELETE FROM analysis_materialization_ledger WHERE identity_hash IN ($2,$3)`,
		} {
			if _, err := db.ExecContext(context.Background(), q, tenant, "cid-"+spec, "pol-key-"+tenantTail); err != nil {
				t.Logf("cleanup warning: %v", err)
			}
		}
		_ = db.Close()
	})
	t.Logf("lease expiry + auto report + ticket consume PASS (run=%s)", runID[:8])
}

//go:build integration

// live PG 集成测试:§20 API 面权威事务(任务定义权威、整 Run 重试、报告策略、
// 报告重试、下载票、触发历史、资源视图、阶段回执投影、task 绑定)。
package repository_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

func TestLivePGAPISurfaceAuthority(t *testing.T) {
	db := liveDBP2(t)
	repo := repository.NewRepo(db)
	ctx := context.Background()
	tenant := "integration-api-" + uuid.NewString()[:8]
	actor := "op-" + uuid.NewString()[:6]
	// 幂等键按租户唯一化:失败残留不跨运行污染。
	keyDef := "api-key-" + tenant[:12]
	keyPol := "pol-key-" + tenant[:12]
	keyRR := "rr-key-" + tenant[:12]
	keyRetryTask := "retry-key-" + tenant[:12]
	// outbox 无租户列,按测试期已知 key(run/report id)清理。
	var runKeys []string

	// 失败也清理:避免残留跨运行污染(幂等键已按租户唯一,此处兜底业务行)。
	t.Cleanup(func() {
		if len(runKeys) > 0 {
			if _, err := db.ExecContext(context.Background(), `DELETE FROM analysis_outbox WHERE key = ANY($1)`, pq.Array(runKeys)); err != nil {
				t.Logf("cleanup outbox warning: %v", err)
			}
		}
		for _, q := range []string{
			`DELETE FROM analysis_report_download_tickets WHERE tenant_id=$1`,
			`DELETE FROM analysis_human_reports WHERE tenant_id=$1`,
			`DELETE FROM analysis_human_report_policies WHERE tenant_id=$1`,
			`DELETE FROM analysis_stage_receipts WHERE tenant_id=$1`,
			`DELETE FROM analysis_business_phase_projections WHERE run_id IN (SELECT id FROM analysis_runs WHERE tenant_id=$1)`,
			`DELETE FROM analysis_stage_queue WHERE tenant_id=$1`,
			`DELETE FROM analysis_stage_attempts WHERE tenant_id=$1`,
			`DELETE FROM analysis_admission_reservations WHERE tenant_id=$1`,
			`DELETE FROM analysis_drr_state WHERE tenant_id=$1`,
			`DELETE FROM analysis_receipts WHERE tenant_id=$1`,
			`DELETE FROM analysis_runs WHERE tenant_id=$1`,
			`DELETE FROM analysis_tasks WHERE tenant_id=$1`,
			`DELETE FROM analysis_trigger_instances WHERE tenant_id=$1`,
			`DELETE FROM analysis_schedule_activation_heads WHERE tenant_id=$1`,
			`DELETE FROM analysis_schedule_revisions WHERE tenant_id=$1`,
			`DELETE FROM analysis_plan_governance_heads WHERE plan_id IN (SELECT id FROM analysis_plan_revisions WHERE tenant_id=$1)`,
			`DELETE FROM analysis_plan_revisions WHERE tenant_id=$1`,
			`DELETE FROM analysis_history WHERE tenant_id=$1`,
			`DELETE FROM analysis_task_definitions WHERE tenant_id=$1`,
		} {
			if _, err := db.ExecContext(context.Background(), q, tenant); err != nil {
				t.Logf("cleanup warning: %v", err)
			}
		}
		if _, err := db.ExecContext(context.Background(), `DELETE FROM analysis_materialization_ledger WHERE identity_hash IN ($1,$2,$3,$4,$5)`,
			keyDef, keyPol, keyRR, keyRetryTask, identityHashForTest(tenant, "actor", actor, keyRetryTask)); err != nil {
			t.Logf("cleanup ledger warning: %v", err)
		}
		_ = db.Close()
	})

	// ---------- 1. 任务定义权威:创建→幂等重放→异载荷 409 ----------
	defSvc := service.NewTaskDefinitionService(repo)
	defID, replayed, err := defSvc.Create(ctx, tenant, "def-api-surface", actor, "BASELINE", keyDef)
	if err != nil || replayed || defID == "" {
		t.Fatalf("create definition: id=%s replayed=%v err=%v", defID, replayed, err)
	}
	replayID, replayed, err := defSvc.Create(ctx, tenant, "def-api-surface", actor, "BASELINE", keyDef)
	if err != nil || !replayed || replayID != defID {
		t.Fatalf("definition replay: id=%s want=%s replayed=%v err=%v", replayID, defID, replayed, err)
	}
	if _, _, err := defSvc.Create(ctx, tenant, "def-api-surface-2", actor, "INTERACTIVE", keyDef); err == nil ||
		!strings.Contains(err.Error(), "IDEMPOTENCY_PAYLOAD_MISMATCH") {
		t.Fatalf("same idempotency key with different payload must map to IDEMPOTENCY_PAYLOAD_MISMATCH, got %v", err)
	}

	// ---------- 2. 激活/挂起 CAS + 列表/详情 ----------
	if _, err := defSvc.Activate(ctx, tenant, defID, 1, actor); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if _, err := defSvc.Activate(ctx, tenant, defID, 1, actor); err == nil || !strings.Contains(err.Error(), "CAS") {
		t.Fatalf("stale revision activate must fail CAS, got %v", err)
	}
	if _, err := defSvc.Suspend(ctx, tenant, defID, 2, actor); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	detail, err := defSvc.Detail(ctx, tenant, defID)
	if err != nil || detail.State != "SUSPENDED" || detail.Revision != 3 {
		t.Fatalf("detail: %+v err=%v", detail, err)
	}
	all, err := repo.ListTaskDefinitions(ctx, tenant)
	if err != nil || len(all) != 1 {
		t.Fatalf("list definitions: n=%d err=%v", len(all), err)
	}

	// ---------- 3. 计划修订投影 ----------
	spec := "spec-" + uuid.NewString()[:12]
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_plan_revisions(id, tenant_id, task_definition_id, plan_revision, plan_source, source_kind,
			source_spec, selected_feature_ids, feature_set_id, encrypted_recognition_model_ref, threat_detector_refs,
			rule_refs, machine_summary_schema_ref, stage_dag, completion_policy, resource_budget, catalog_revision,
			selection_origins, canonicalization_version, execution_spec_sha256, plan_revision_sha256, created_by)
		VALUES($1,$2,$3,1,'AUTO_DEFAULT','PCAP_REPLAY','{}','[]','fs-1','', '[]','[]','','{}','{}','{"cpu":2}',1,'[]','v1',$4,$5,$6)`,
		uuid.NewString(), tenant, defID, spec, "plan-sha-"+spec[:6], actor); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	plans, err := defSvc.Plans(ctx, tenant, defID)
	if err != nil || len(plans) != 1 || plans[0].ExecutionSpecSHA256 != spec {
		t.Fatalf("plans: %+v err=%v", plans, err)
	}

	// ---------- 4. 报告策略:保存→重放→列表→下一修订号 ----------
	policyID, rev, replayed, err := defSvc.SaveReportPolicy(ctx, tenant, defID, "AUTO_ASYNC", "default-v1", "zh-CN", 30, keyPol)
	if err != nil || replayed || policyID == "" || rev != 1 {
		t.Fatalf("save policy: id=%s rev=%d replayed=%v err=%v", policyID, rev, replayed, err)
	}
	replayPID, replayRev, replayed, err := defSvc.SaveReportPolicy(ctx, tenant, defID, "AUTO_ASYNC", "default-v1", "zh-CN", 30, keyPol)
	if err != nil || !replayed || replayPID != policyID || replayRev != 1 {
		t.Fatalf("policy replay: id=%s rev=%d replayed=%v err=%v", replayPID, replayRev, replayed, err)
	}
	pols, err := defSvc.ListReportPolicies(ctx, tenant, defID)
	if err != nil || len(pols) != 1 {
		t.Fatalf("list policies: %+v err=%v", pols, err)
	}
	if next, err := repo.NextHumanReportPolicyRevision(ctx, tenant, defID); err != nil || next != 2 {
		t.Fatalf("next policy revision: %d err=%v", next, err)
	}

	// ---------- 5. 调度触发历史投影 ----------
	schedID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_schedule_revisions(id, tenant_id, task_definition_id, revision, approved_plan_revision,
			execution_spec_sha256, trigger_kind, window_or_cron, schedule_sha256)
		VALUES($1,$2,$3,1,1,$4,'CRON_WINDOW','{}','sched-sha')`, schedID, tenant, defID, spec); err != nil {
		t.Fatalf("seed schedule: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_trigger_instances(id, tenant_id, identity_kind, canonical_identity_hash, request_sha256,
			state, trigger_kind, task_definition_id, plan_revision, actor, schedule_revision)
		VALUES($1,$2,'schedule','sched-id','sched-req','MATERIALIZED','CRON_WINDOW',$3,1,$4,1)`,
		uuid.NewString(), tenant, defID, actor); err != nil {
		t.Fatalf("seed schedule trigger: %v", err)
	}
	hist, err := repo.ListTriggersForSchedule(ctx, tenant, schedID)
	if err != nil || len(hist) != 1 || hist[0].TriggerKind != "CRON_WINDOW" {
		t.Fatalf("trigger history: %+v err=%v", hist, err)
	}

	// ---------- 6. 整 Run 重试(同 task 新 run) ----------
	// 原 task/run(SUCCEEDED,绑定 spec;task.trigger_instance_id FK → 种子触发实例)
	origTask := uuid.NewString()
	origRun := uuid.NewString()
	runKeys = append(runKeys, origRun)
	origTrigger := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_trigger_instances(id, tenant_id, identity_kind, canonical_identity_hash, request_sha256,
			state, trigger_kind, task_definition_id, plan_revision, actor)
		VALUES($1,$2,'actor','orig-id','orig-req','MATERIALIZED','ON_DEMAND',$3,1,$4)`,
		origTrigger, tenant, defID, actor); err != nil {
		t.Fatalf("seed orig trigger: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_tasks(id, tenant_id, task_definition_id, plan_revision, execution_spec_sha256,
			trigger_instance_id, effective_class, effective_policy_sha256, current_run_id)
		VALUES($1,$2,$3,1,$4,$5,'BASELINE','pol-1',$6)`, origTask, tenant, defID, spec, origTrigger, origRun); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_runs(id, tenant_id, task_id, execution_spec_sha256, state, window_start, window_end, created_at)
		VALUES($1,$2,$3,$4,'SUCCEEDED',to_timestamp(1700000000),to_timestamp(1700000600),now())`,
		origRun, tenant, origTask, spec); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	binding, err := repo.GetTaskByRunID(ctx, origRun)
	if err != nil || binding.TaskID != origTask || binding.ExecutionSpecSHA256 != spec {
		t.Fatalf("task binding: %+v err=%v", binding, err)
	}
	retrySvc := service.NewRetryTaskService(repo)
	receipt, err := retrySvc.RetryTask(ctx, tenant, origRun, actor, keyRetryTask)
	if err != nil || receipt.TaskID == "" || receipt.TaskID == origTask || receipt.RunID == "" || receipt.RunID == origRun {
		t.Fatalf("retry task: %+v err=%v", receipt, err)
	}
	runKeys = append(runKeys, receipt.RunID)
	replayReceipt, err := retrySvc.RetryTask(ctx, tenant, origRun, actor, keyRetryTask)
	if err != nil || replayReceipt.TaskID != receipt.TaskID || replayReceipt.RunID != receipt.RunID {
		t.Fatalf("retry task replay: %+v err=%v", replayReceipt, err)
	}
	// 新 run 为 ACCEPTED,种子 9 attempt
	var newRunState string
	if err := db.QueryRowContext(ctx, `SELECT state FROM analysis_runs WHERE id=$1`, receipt.RunID).Scan(&newRunState); err != nil || newRunState != "ACCEPTED" {
		t.Fatalf("new run state: %s err=%v", newRunState, err)
	}
	t.Logf("retry-task PASS: %s → new task %s run %s", origRun, receipt.TaskID, receipt.RunID)

	// ---------- 7. 阶段回执投影 ----------
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_stage_receipts(id, tenant_id, run_id, execution_node_id, attempt, fencing_token, provider,
			input_count, output_count, error_count, fence, payload_hash, received_at)
		VALUES($1,$2,$3,'SOURCE_ACTIVATE',1,'tok-1','probe-agent',10,10,0,'{}','ph-1',now())`,
		uuid.NewString(), tenant, receipt.RunID); err != nil {
		t.Fatalf("seed stage receipt: %v", err)
	}
	receipts, err := repo.ListStageReceiptsForRun(ctx, tenant, receipt.RunID)
	if err != nil || len(receipts) != 1 || receipts[0].Provider != "probe-agent" {
		t.Fatalf("stage receipts: %+v err=%v", receipts, err)
	}

	// ---------- 8. 报告重试(原地 FAILED→QUEUED)+ 下载票 ----------
	reportSvc := service.NewHumanReportService(repo)
	failedReport := uuid.NewString()
	runKeys = append(runKeys, failedReport)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_human_reports(id, tenant_id, run_id, summary_sha256, template_revision, locale, state)
		VALUES($1,$2,$3,'sum-sha-1','default-v1','zh-CN','FAILED')`, failedReport, tenant, receipt.RunID); err != nil {
		t.Fatalf("seed failed report: %v", err)
	}
	retriedID, replayed, err := reportSvc.RetryReport(ctx, tenant, failedReport, keyRR)
	if err != nil || replayed || retriedID != failedReport {
		t.Fatalf("retry report: id=%s replayed=%v err=%v", retriedID, replayed, err)
	}
	// 幂等重放:同台账键不再重排队(不产生第二条 outbox)
	if _, replayed, err := reportSvc.RetryReport(ctx, tenant, failedReport, keyRR); err != nil || !replayed {
		t.Fatalf("retry report replay: replayed=%v err=%v", replayed, err)
	}
	var newState string
	if err := db.QueryRowContext(ctx, `SELECT state FROM analysis_human_reports WHERE id=$1`, failedReport).Scan(&newState); err != nil || newState != "QUEUED" {
		t.Fatalf("report state after retry: %s err=%v", newState, err)
	}
	var outboxCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM analysis_outbox WHERE key=$1 AND topic='analysis.report.requests.v1' AND state='PENDING'`,
		failedReport).Scan(&outboxCount); err != nil || outboxCount != 1 {
		t.Fatalf("retry report outbox: %d err=%v", outboxCount, err)
	}
	// 非 FAILED/CANCELLED 不可重试(QUEUED 行)
	if _, _, err := reportSvc.RetryReport(ctx, tenant, failedReport, keyRR+"-2"); err == nil || !strings.Contains(err.Error(), "not retryable") {
		t.Fatalf("QUEUED report retry must be rejected, got %v", err)
	}
	// 下载票:仅 AVAILABLE
	if _, _, err := reportSvc.IssueDownloadTicket(ctx, tenant, failedReport, 5*time.Minute, actor); err == nil {
		t.Fatal("download ticket on QUEUED report must be rejected")
	}
	if _, err := db.ExecContext(ctx, `UPDATE analysis_human_reports SET state='AVAILABLE' WHERE id=$1`, failedReport); err != nil {
		t.Fatalf("mark available: %v", err)
	}
	ticketID, expiresAt, err := reportSvc.IssueDownloadTicket(ctx, tenant, failedReport, 5*time.Minute, actor)
	if err != nil || ticketID == "" || !expiresAt.After(time.Now()) {
		t.Fatalf("issue ticket: id=%s expires=%v err=%v", ticketID, expiresAt, err)
	}

	// ---------- 9. 资源视图 ----------
	// receipt.RunID 已由物化事务带 RESERVED;此处为 origRun 补一条(不同 run,满足 uq)。
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_admission_reservations(id, tenant_id, run_id, resource_pool, resource_vector, policy_sha256, state, epoch, expires_at, authority_revision)
		VALUES($1,$2,$3,'analysis-cpu','{"cpu":2}','pol-1','RESERVED',1,now()+interval '5 minutes',0)`,
		uuid.NewString(), tenant, origRun); err != nil {
		t.Fatalf("seed reservation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_drr_state(tenant_id, scheduling_class, deficit, quantum, scheduler_epoch)
		VALUES($1,'BASELINE',3,2,1) ON CONFLICT (tenant_id, scheduling_class) DO UPDATE SET deficit=3`, tenant); err != nil {
		t.Fatalf("seed drr: %v", err)
	}
	views, err := repo.GetResourceViews(ctx, tenant)
	if err != nil || len(views.Reservations) < 1 || len(views.Drr) < 1 || len(views.OutboxLedger) < 1 {
		t.Fatalf("resource views: %+v err=%v", views, err)
	}

	// ---------- 9b. DRR 台账累积(RecordDrrServe 两次:deficit 累积、epoch 递增) ----------
	if err := repo.RecordDrrServe(ctx, tenant, "BASELINE", 3, 2); err != nil {
		t.Fatalf("record drr 1: %v", err)
	}
	if err := repo.RecordDrrServe(ctx, tenant, "BASELINE", 3, 2); err != nil {
		t.Fatalf("record drr 2: %v", err)
	}
	var deficit, epoch int64
	// 种子行(deficit=3, epoch=1)+ 两次服务(cost=3 quantum=2)→ deficit=5, epoch=3
	if err := db.QueryRowContext(ctx, `SELECT deficit, scheduler_epoch FROM analysis_drr_state WHERE tenant_id=$1 AND scheduling_class='BASELINE'`,
		tenant).Scan(&deficit, &epoch); err != nil || deficit != 5 || epoch != 3 {
		t.Fatalf("drr accumulate: deficit=%d epoch=%d err=%v", deficit, epoch, err)
	}
	t.Logf("drr accumulate PASS: deficit=5 epoch=3 (seed 3/1 + two serves cost=3 quantum=2)")

	// 清理交由 t.Cleanup(见测试开头),此处仅记录。
	t.Logf("API surface authority integration PASS (tenant=%s)", tenant)
}

// identityHashForTest 与服务层 identityHash 同算法(判别联合)。
func identityHashForTest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

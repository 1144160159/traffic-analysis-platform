//go:build integration

// live PG 集成测试:调度修订权威链(Save/Activate/Pause/List)、有效策略解析
// 输入冻结、Run 状态机行走、AdmissionReservation 生命周期、FORBID_OVERLAP 判定。
package repository_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

func TestLivePGScheduleAuthorityAndStateWalk(t *testing.T) {
	db := liveDBP2(t)
	defer db.Close()
	repo := repository.NewRepo(db)
	ctx := context.Background()

	tenant := "integration-sched-" + uuid.NewString()[:8]
	defID := uuid.NewString()
	planID := uuid.NewString()
	spec := "sched-spec-" + uuid.NewString()[:8]

	// 种子:定义 + AUTO_DEFAULT 已批准计划(APPROVED 即可被 schedule 精确绑定)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_task_definitions(id, tenant_id, name, state, owner, revision, created_by, default_scheduling_class)
		VALUES($1,$2,'def-sched','ACTIVE',$2,1,$2,'BASELINE')`, defID, tenant); err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_plan_revisions(id, tenant_id, task_definition_id, plan_revision, plan_source,
			source_kind, source_spec, selected_feature_ids, feature_set_id, encrypted_recognition_model_ref,
			threat_detector_refs, rule_refs, machine_summary_schema_ref, stage_dag, completion_policy,
			resource_budget, catalog_revision, execution_spec_sha256, plan_revision_sha256, created_by)
		VALUES($1,$2,$3,1,'AUTO_DEFAULT','PCAP_REPLAY','{"pcap_object":"s3://b/base.pcap"}'::jsonb,
			'["f1"]'::jsonb,'fs-v1','enc@v1','["det@v1"]'::jsonb,'["rule@v1"]'::jsonb,'summary-v1',
			'{"stages":["S1","S2","S3","S4","S5"]}'::jsonb,'{"allow_partial":false}'::jsonb,
			'{"cpu":2}'::jsonb,1,$4,$4,$2)`, planID, tenant, defID, spec); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_plan_governance_heads(tenant_id, plan_id, state, authority_revision, approved_by, approved_at)
		VALUES($1,$2,'APPROVED',1,'governor',now())`, tenant, planID); err != nil {
		t.Fatalf("seed head: %v", err)
	}

	svc := service.NewScheduleService(repo)

	// 1. 保存调度修订(精确绑定已批准 plan;修订号自动分配;激活前 DRAFT 不触发)
	resp, replayed, err := svc.Save(ctx, service.SaveScheduleRequest{
		TenantID: tenant, TaskDefinitionID: defID,
		ApprovedPlanRevision: 1, ExecutionSpecSHA256: spec,
		TriggerKind: "CONTINUOUS_WINDOW", Timezone: "UTC",
		WindowOrCron:         json.RawMessage(`{"window_ms":60000}`),
		PrepareLeadTimeMs:    5000,
		MisfirePolicy:        "MISFIRE_FAIL",
		ConcurrencyPolicy:    "FORBID_OVERLAP",
		SchedulingClass:      "BASELINE",
		ResourceRestrictions: json.RawMessage(`{"cpu":8}`),
		ClientIdempotencyKey: "sched-key-1-" + tenant,
	})
	if err != nil {
		t.Fatalf("save schedule: %v", err)
	}
	if replayed || resp.Revision != 1 || resp.ScheduleID == "" || len(resp.ScheduleSHA256) != 64 {
		t.Fatalf("save result mismatch: %+v replayed=%v", resp, replayed)
	}
	head, err := svc.Head(ctx, tenant, resp.ScheduleID)
	if err != nil || head.State != "DRAFT" || head.AuthorityRevision != 0 {
		t.Fatalf("head must start DRAFT@0: %+v err=%v", head, err)
	}
	t.Logf("schedule saved id=%s rev=%d sha=%s", resp.ScheduleID, resp.Revision, resp.ScheduleSHA256)

	// 幂等重放:同 key 同 payload → 同 schedule id
	resp2, replayed2, err := svc.Save(ctx, service.SaveScheduleRequest{
		TenantID: tenant, TaskDefinitionID: defID,
		ApprovedPlanRevision: 1, ExecutionSpecSHA256: spec,
		TriggerKind: "CONTINUOUS_WINDOW", Timezone: "UTC",
		WindowOrCron:         json.RawMessage(`{"window_ms":60000}`),
		PrepareLeadTimeMs:    5000,
		MisfirePolicy:        "MISFIRE_FAIL",
		ConcurrencyPolicy:    "FORBID_OVERLAP",
		SchedulingClass:      "BASELINE",
		ResourceRestrictions: json.RawMessage(`{"cpu":8}`),
		ClientIdempotencyKey: "sched-key-1-" + tenant,
	})
	if err != nil || !replayed2 || resp2.ScheduleID != resp.ScheduleID {
		t.Fatalf("idempotent replay diverged: %+v replayed=%v err=%v", resp2, replayed2, err)
	}
	t.Logf("schedule save idempotent replay confirmed")

	// 2. 激活:expected revision CAS;陈旧 revision 拒绝
	if rev, err := svc.Activate(ctx, tenant, resp.ScheduleID, 0, "sched-admin"); err != nil || rev != 1 {
		t.Fatalf("activate: rev=%d err=%v", rev, err)
	}
	if _, err := svc.Activate(ctx, tenant, resp.ScheduleID, 0, "sched-admin"); err == nil ||
		!strings.Contains(err.Error(), "activation CAS failed") {
		t.Fatalf("stale expected revision must fail CAS, got %v", err)
	}
	rows, err := svc.List(ctx, tenant)
	if err != nil || len(rows) != 1 || rows[0].HeadState != "ACTIVE" || rows[0].AuthorityRevision != 1 {
		t.Fatalf("list after activate: %+v err=%v", rows, err)
	}
	active, err := repo.ListActiveSchedules(ctx, tenant)
	if err != nil || len(active) != 1 || active[0].ResourceRestrictions == "" {
		t.Fatalf("active schedules for tick: %+v err=%v", active, err)
	}
	t.Logf("schedule activated; tick view carries restrictions=%s", active[0].ResourceRestrictions)

	// 3. 暂停:只影响未来触发
	if rev, err := svc.Pause(ctx, tenant, resp.ScheduleID, 1, "sched-admin"); err != nil || rev != 2 {
		t.Fatalf("pause: rev=%d err=%v", rev, err)
	}
	active, err = repo.ListActiveSchedules(ctx, tenant)
	if err != nil || len(active) != 0 {
		t.Fatalf("paused schedule must leave tick view: %+v err=%v", active, err)
	}
	t.Logf("schedule paused (tick view empty)")

	// 4. Run 状态机行走:物化 → ACCEPTED→PREPARING→QUEUED→RUNNING→FINALIZING
	triggerID := uuid.NewString()
	walkID := "walk-id-" + tenant
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_trigger_instances(id, tenant_id, identity_kind, canonical_identity_hash, request_sha256, state, trigger_kind, window_id, task_definition_id, plan_revision, actor, effective_class, resource_restrictions, schedule_revision)
		VALUES($1,$2,'schedule',$3,'walk-req','PENDING_MATERIALIZATION','CONTINUOUS_WINDOW','w-1', $4,1,'scheduler','BASELINE','{"cpu":8}'::jsonb,1)`,
		triggerID, tenant, walkID, defID); err != nil {
		t.Fatalf("seed trigger: %v", err)
	}
	receipt, replayed, err := repo.MaterializeAnalysisTaskAtomic(ctx, repository.MaterializeCommand{
		TenantID: tenant, IdentityKind: "schedule",
		CanonicalIdentityHash: walkID, RequestSHA256: "walk-req",
		TriggerInstanceID: triggerID, TriggerKind: "CONTINUOUS_WINDOW",
		WindowStartMs: time.Now().UnixMilli(), WindowEndMs: time.Now().Add(10 * time.Minute).UnixMilli(),
		TaskDefinitionID: defID, PlanRevision: 1, ExecutionSpecSHA256: spec,
		ScheduleRevision: 1, EffectiveClass: "BASELINE",
		EffectivePolicySHA256: strings.Repeat("a", 64),
		ResourcePool:          "analysis-cpu", ResourceVectorJSON: []byte(`{"cpu":2}`),
		ExpiresAt:    time.Now().Add(5 * time.Minute),
		NodesJSON:    []byte(`[{"business_phase_id":"S1","execution_node_id":"SOURCE_ACTIVATE","provider_mode":"DEDICATED_OPERATION","activation_mode":"PIPELINED_STREAM"}]`),
		PlanSpecJSON: []byte(`{}`),
	})
	if err != nil || replayed || receipt.RunID == "" {
		t.Fatalf("materialize walk run: %v replayed=%v err=%v", receipt, replayed, err)
	}
	runID := receipt.RunID
	walk := service.NewRunStateWalker(repo, nil)

	// 事实 1:订阅已广播 → Sync 推进 ACCEPTED→PREPARING
	if _, err := db.ExecContext(ctx, `UPDATE analysis_outbox SET state='PUBLISHED' WHERE key=$1`, runID); err != nil {
		t.Fatalf("mark outbox published: %v", err)
	}
	walk.Sync(ctx, tenant, runID)
	if s, _ := repo.GetRunState(ctx, tenant, runID); s != "PREPARING" {
		t.Fatalf("after subscription published expect PREPARING, got %s", s)
	}
	// 事实 2:S1 领取(DISPATCHED)+ 准入消费 → Sync 推进 QUEUED→RUNNING
	var attemptID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM analysis_stage_attempts WHERE run_id=$1 AND execution_node_id='SOURCE_ACTIVATE'`, runID).Scan(&attemptID); err != nil {
		t.Fatalf("load source attempt: %v", err)
	}
	if ok, err := repo.MarkAttemptDispatchedAtomic(ctx, tenant, attemptID, "walk-fence"); err != nil || !ok {
		t.Fatalf("mark attempt dispatched: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.ConsumeReservationAtomic(ctx, tenant, runID, time.Now()); err != nil || !ok {
		t.Fatalf("consume reservation: ok=%v err=%v", ok, err)
	}
	walk.Sync(ctx, tenant, runID)
	if s, _ := repo.GetRunState(ctx, tenant, runID); s != "RUNNING" {
		t.Fatalf("after source dispatch claim expect RUNNING, got %s", s)
	}
	var resState string
	if err := db.QueryRowContext(ctx, `SELECT state FROM analysis_admission_reservations WHERE run_id=$1`, runID).Scan(&resState); err != nil || resState != "CONSUMED" {
		t.Fatalf("reservation must be CONSUMED after dispatch claim: %s err=%v", resState, err)
	}
	// 事实 3:业务节点全终态 → Sync 推进 RUNNING→FINALIZING
	if _, err := db.ExecContext(ctx, `UPDATE analysis_stage_attempts SET state='SUCCEEDED', finished_at=now() WHERE run_id=$1`, runID); err != nil {
		t.Fatalf("mark business terminal: %v", err)
	}
	walk.Sync(ctx, tenant, runID)
	if s, _ := repo.GetRunState(ctx, tenant, runID); s != "FINALIZING" {
		t.Fatalf("after all business terminal expect FINALIZING, got %s", s)
	}
	t.Logf("run state walk ACCEPTED→PREPARING→QUEUED→RUNNING→FINALIZING PASS; reservation CONSUMED")

	// 5. 准入过期:新 reservation 过期扫描 → EXPIRED;已消费的过期扫描不动
	trigger2 := uuid.NewString()
	walkID2 := "walk-id-2-" + tenant
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_trigger_instances(id, tenant_id, identity_kind, canonical_identity_hash, request_sha256, state, trigger_kind, task_definition_id, plan_revision, actor)
		VALUES($1,$2,'schedule',$3,'walk-req-2','PENDING_MATERIALIZATION','CONTINUOUS_WINDOW',$4,1,'scheduler')`,
		trigger2, tenant, walkID2, defID); err != nil {
		t.Fatalf("seed trigger 2: %v", err)
	}
	receipt2, _, err := repo.MaterializeAnalysisTaskAtomic(ctx, repository.MaterializeCommand{
		TenantID: tenant, IdentityKind: "schedule",
		CanonicalIdentityHash: walkID2, RequestSHA256: "walk-req-2",
		TriggerInstanceID: trigger2, TriggerKind: "CONTINUOUS_WINDOW",
		WindowStartMs: time.Now().UnixMilli(), WindowEndMs: time.Now().Add(10 * time.Minute).UnixMilli(),
		TaskDefinitionID: defID, PlanRevision: 1, ExecutionSpecSHA256: spec,
		EffectiveClass: "BASELINE", EffectivePolicySHA256: strings.Repeat("b", 64),
		ResourcePool: "analysis-cpu", ResourceVectorJSON: []byte(`{"cpu":2}`),
		ExpiresAt:    time.Now().Add(-time.Minute), // 已过期
		NodesJSON:    []byte(`[{"business_phase_id":"S1","execution_node_id":"SOURCE_ACTIVATE","provider_mode":"DEDICATED_OPERATION","activation_mode":"PIPELINED_STREAM"}]`),
		PlanSpecJSON: []byte(`{}`),
	})
	if err != nil || receipt2.RunID == "" {
		t.Fatalf("materialize expiring run: %v err=%v", receipt2, err)
	}
	if n, err := repo.ExpireReservationsAtomic(ctx, tenant, time.Now()); err != nil || n < 1 {
		t.Fatalf("expire sweep: n=%d err=%v", n, err)
	}
	var res2 string
	if err := db.QueryRowContext(ctx, `SELECT state FROM analysis_admission_reservations WHERE run_id=$1`, receipt2.RunID).Scan(&res2); err != nil || res2 != "EXPIRED" {
		t.Fatalf("expired reservation must be EXPIRED: %s err=%v", res2, err)
	}
	// 过期消费必须被拒(重新准入前不能启动)
	ok, err := repo.ConsumeReservationAtomic(ctx, tenant, receipt2.RunID, time.Now())
	if err != nil || ok {
		t.Fatalf("EXPIRED reservation must not be consumable: ok=%v err=%v", ok, err)
	}
	t.Logf("reservation expiry + re-admission guard PASS")

	// 6. FORBID_OVERLAP 判定:walk run 仍在 FINALIZING(非终态)→ 定义下有活动 run
	hasActive, err := repo.HasActiveRunForDefinition(ctx, tenant, defID)
	if err != nil || !hasActive {
		t.Fatalf("non-terminal run must count as active: %v err=%v", hasActive, err)
	}
	t.Logf("FORBID_OVERLAP active-run detection PASS")

	// 清理
	for _, q := range []string{
		`DELETE FROM analysis_stage_receipts WHERE tenant_id=$1`,
		`DELETE FROM analysis_stage_queue WHERE tenant_id=$1`,
		`DELETE FROM analysis_stage_attempts WHERE tenant_id=$1`,
		`DELETE FROM analysis_admission_reservations WHERE tenant_id=$1`,
		`DELETE FROM analysis_business_phase_projections WHERE run_id IN (SELECT id FROM analysis_runs WHERE tenant_id=$1)`,
		`DELETE FROM analysis_outbox WHERE key IN (SELECT id::text FROM analysis_runs WHERE tenant_id=$1)`,
		`DELETE FROM analysis_runs WHERE tenant_id=$1`,
		`DELETE FROM analysis_tasks WHERE tenant_id=$1`,
		`DELETE FROM analysis_history WHERE entity_id IN (SELECT id FROM analysis_schedule_revisions WHERE tenant_id=$1)`,
		`DELETE FROM analysis_schedule_activation_heads WHERE schedule_id IN (SELECT id FROM analysis_schedule_revisions WHERE tenant_id=$1)`,
		`DELETE FROM analysis_schedule_revisions WHERE tenant_id=$1`,
		`DELETE FROM analysis_trigger_instances WHERE tenant_id=$1`,
		`DELETE FROM analysis_plan_governance_heads WHERE plan_id IN (SELECT id FROM analysis_plan_revisions WHERE tenant_id=$1)`,
		`DELETE FROM analysis_plan_revisions WHERE tenant_id=$1`,
		`DELETE FROM analysis_task_definitions WHERE tenant_id=$1`,
	} {
		if _, err := db.ExecContext(ctx, q, tenant); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM analysis_materialization_ledger WHERE identity_hash LIKE 'sched-key-%' OR identity_hash LIKE 'walk-id-%'`); err != nil {
		t.Logf("cleanup ledger: %v", err)
	}
}

// TestLivePGGetRunSummary 报告渲染输入读取(§10.3)。
func TestLivePGGetRunSummary(t *testing.T) {
	db := liveDBP2(t)
	defer db.Close()
	repo := repository.NewRepo(db)
	v, err := repo.GetRunSummaryContent(context.Background(), "default", "3c8989ab-223b-46da-9c36-0610b061ddff")
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if v.SummarySHA256 == "" || v.RunID == "" || len(v.KeyFindings) == 0 {
		t.Fatalf("summary content incomplete: %+v", v)
	}
	t.Logf("summary content PASS: run=%s sha=%s", v.RunID, v.SummarySHA256[:16])
}

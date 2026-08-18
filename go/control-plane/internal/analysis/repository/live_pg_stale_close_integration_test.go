//go:build integration

// live PG 集成测试(§76.45.4 兜底):窗口早已结束且未启动的 run 权威关闭
// CANCELLED(attempts 取消/队列 EXPIRED/预留 RELEASED/投影取消/审计)。
package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

func TestLivePGCloseStaleRuns(t *testing.T) {
	db := liveDBP2(t)
	repo := repository.NewRepo(db)
	ctx := context.Background()
	tenant := "integration-stale-" + uuid.NewString()[:8]
	tail := tenant[len(tenant)-8:]
	spec := "stale-spec-" + tail

	defID := uuid.NewString()
	planID := uuid.NewString()
	triggerID := uuid.NewString()
	seedExec(t, db, ctx, `
		INSERT INTO analysis_task_definitions(id, tenant_id, name, state, owner, revision, created_by)
		VALUES($1,$2,$3,'ACTIVE',$2,1,$2)`, defID, tenant, "def-"+defID[:6])
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
	// 窗口设为 30 分钟前(已结束且过 grace)
	receipt, replayed, err := repo.MaterializeAnalysisTaskAtomic(ctx, repository.MaterializeCommand{
		TenantID: tenant, IdentityKind: "actor", CanonicalIdentityHash: "cid-" + spec,
		RequestSHA256: "req-" + spec, TriggerInstanceID: triggerID, TriggerKind: "ON_DEMAND",
		WindowStartMs:    time.Now().Add(-40 * time.Minute).UnixMilli(),
		WindowEndMs:      time.Now().Add(-30 * time.Minute).UnixMilli(),
		TaskDefinitionID: defID, PlanRevision: 1, ExecutionSpecSHA256: spec,
		EffectiveClass: "BASELINE", EffectivePolicySHA256: "policy-1",
		ResourcePool: "analysis-cpu", ResourceVectorJSON: []byte(`{"cpu":2}`),
		QueueCostMilli: 2000, ExpiresAt: time.Now().Add(5 * time.Minute),
		NodesJSON:    service.DefaultNodeExactSet(),
		PlanSpecJSON: []byte(`{}`),
	})
	if err != nil || replayed || receipt.RunID == "" {
		t.Fatalf("materialize: %+v replayed=%v err=%v", receipt, replayed, err)
	}
	runID := receipt.RunID

	// 关闭(grace 10 分钟;窗口 30 分钟前结束)。全局扫描:同批可能含其他历史陈旧 run,
	// 断言聚焦本 run 的关闭结果而非批大小。
	closed, err := repo.CloseStaleRunsAtomic(ctx, tenant, time.Now(), 10*time.Minute, 20)
	if err != nil || closed < 1 {
		t.Fatalf("close stale: closed=%d err=%v", closed, err)
	}
	var state, completeness, finding string
	if err := db.QueryRowContext(ctx, `SELECT state, completeness, finding_conclusion FROM analysis_runs WHERE id=$1`, runID).
		Scan(&state, &completeness, &finding); err != nil || state != "CANCELLED" || completeness != "INCOMPLETE" || finding != "NOT_EVALUATED" {
		t.Fatalf("run after close: %s/%s/%s err=%v", state, completeness, finding, err)
	}
	var pendingAtt, cancelledAtt int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE state IN ('PENDING','DISPATCHED')), count(*) FILTER (WHERE state='CANCELLED')
		FROM analysis_stage_attempts WHERE run_id=$1`, runID).Scan(&pendingAtt, &cancelledAtt); err != nil || pendingAtt != 0 || cancelledAtt != 9 {
		t.Fatalf("attempts after close: pending=%d cancelled=%d err=%v", pendingAtt, cancelledAtt, err)
	}
	var readyQueue int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM analysis_stage_queue WHERE run_id=$1 AND state IN ('READY','CLAIMED')`, runID).Scan(&readyQueue); err != nil || readyQueue != 0 {
		t.Fatalf("queue after close: %d err=%v", readyQueue, err)
	}
	var released int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM analysis_admission_reservations WHERE run_id=$1 AND state='RELEASED'`, runID).Scan(&released); err != nil || released != 1 {
		t.Fatalf("reservation after close: %d err=%v", released, err)
	}
	var hist int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM analysis_history WHERE tenant_id=$1 AND entity='run' AND action='STALE_WINDOW_CLOSED'`, tenant).Scan(&hist); err != nil || hist != 1 {
		t.Fatalf("history: %d err=%v", hist, err)
	}
	// 再扫一遍:本 run 不得重复关闭(审计只有一条)
	if _, err := repo.CloseStaleRunsAtomic(ctx, tenant, time.Now(), 10*time.Minute, 20); err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM analysis_history WHERE tenant_id=$1 AND entity='run' AND action='STALE_WINDOW_CLOSED'`, tenant).Scan(&hist); err != nil || hist != 1 {
		t.Fatalf("duplicate closure history: %d err=%v", hist, err)
	}

	t.Cleanup(func() {
		for _, q := range []string{
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
		} {
			if _, err := db.ExecContext(context.Background(), q, tenant); err != nil {
				t.Logf("cleanup warning: %v", err)
			}
		}
		if _, err := db.ExecContext(context.Background(), `DELETE FROM analysis_materialization_ledger WHERE identity_hash=$1`, "cid-"+spec); err != nil {
			t.Logf("cleanup ledger warning: %v", err)
		}
		_ = db.Close()
	})
	t.Logf("stale close PASS: %s → CANCELLED (9 attempts cancelled, queue expired, reservation released)", runID[:8])
}

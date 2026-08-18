//go:build integration

// live PG 集成测试(G04):经端口转发连真实 PostgreSQL 幂等执行权威事务。
// 运行:PGPASSWORD=... ANALYSIS_TEST_PG_DSN="host=127.0.0.1 port=15432 user=postgres dbname=traffic_platform sslmode=disable" go test -tags=integration ./internal/analysis/repository/ -run TestLivePG
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func liveDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("ANALYSIS_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("ANALYSIS_TEST_PG_DSN not set; run with kubectl port-forward postgres-primary-0 15432:5432")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return db
}

func TestLivePGMaterializeReceiptFinalizeCycle(t *testing.T) {
	db := liveDB(t)
	defer db.Close()
	r := NewRepo(db)
	ctx := context.Background()

	tenant := "integration-" + uuid.NewString()[:8]
	defID := uuid.NewString()
	planID := uuid.NewString()
	triggerID := uuid.NewString()

	// 准备:定义+计划(最小插入)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_task_definitions(id, tenant_id, name, state, owner, revision, created_by)
		VALUES($1,$2,$3,'ACTIVE',$4,1,$4)`, defID, tenant, "def-"+defID[:6], tenant); err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	spec := "spec-" + uuid.NewString()[:8]
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_plan_revisions(id, tenant_id, task_definition_id, plan_revision, plan_source,
			source_kind, source_spec, selected_feature_ids, feature_set_id, encrypted_recognition_model_ref,
			threat_detector_refs, rule_refs, machine_summary_schema_ref, stage_dag, completion_policy,
			resource_budget, catalog_revision, execution_spec_sha256, plan_revision_sha256, created_by)
		VALUES($1,$2,$3,1,'AUTO_DEFAULT','LIVE_STREAM_WINDOW','{}'::jsonb,'["pktlen_mean"]'::jsonb,'fs-v1',
			'enc@v1','["rule@v1"]'::jsonb,'["rule@v1"]'::jsonb,'summary-v1','{"stages":["S1","S2","S3","S4","S5"]}'::jsonb,
			'{"allow_partial":false}'::jsonb,'{"cpu":2}'::jsonb,1,$4,$4,$2)`, planID, tenant, defID, spec); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_trigger_instances(id, tenant_id, identity_kind, canonical_identity_hash, request_sha256, state, trigger_kind)
		VALUES($1,$2,'actor',$3,$4,'PENDING_MATERIALIZATION','ON_DEMAND')`,
		triggerID, tenant, "id-"+uuid.NewString()[:8], "req-"+uuid.NewString()[:8]); err != nil {
		t.Fatalf("seed trigger: %v", err)
	}

	// 1. 物化
	cmd := MaterializeCommand{
		TenantID: tenant, IdentityKind: "actor",
		CanonicalIdentityHash: "live-" + uuid.NewString()[:8], RequestSHA256: "live-req-" + uuid.NewString()[:8],
		TriggerInstanceID: triggerID, TriggerKind: "ON_DEMAND",
		WindowStartMs: time.Now().UnixMilli(), WindowEndMs: time.Now().Add(10 * time.Minute).UnixMilli(),
		TaskDefinitionID: defID, PlanRevision: 1, ExecutionSpecSHA256: spec,
		EffectiveClass: "INTERACTIVE", EffectivePolicySHA256: "policy-1",
		ResourcePool: "analysis-cpu", ResourceVectorJSON: []byte(`{"cpu":2}`),
		ExpiresAt:    time.Now().Add(5 * time.Minute),
		NodesJSON:    []byte(`[{"business_phase_id":"S1","execution_node_id":"SOURCE_ACTIVATE","provider_mode":"DEDICATED_OPERATION","activation_mode":"PIPELINED_STREAM"},{"business_phase_id":"S5","execution_node_id":"MACHINE_FINALIZATION","provider_mode":"AUTHORITY_LOCAL","activation_mode":"AUTHORITY_LOCAL"}]`),
		PlanSpecJSON: []byte(`{"execution_spec_sha256":"` + spec + `"}`),
	}
	receipt, replayed, err := r.MaterializeAnalysisTaskAtomic(ctx, cmd)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if replayed || receipt.RunID == "" {
		t.Fatalf("unexpected materialize result: %+v replayed=%v", receipt, replayed)
	}
	t.Logf("materialized task=%s run=%s", receipt.TaskID, receipt.RunID)

	// 2. 精确重放(同 identity 同 hash)
	_, replayed, err = r.MaterializeAnalysisTaskAtomic(ctx, cmd)
	if err != nil || !replayed {
		t.Fatalf("expected exact replay, got err=%v replayed=%v", err, replayed)
	}
	t.Logf("exact replay confirmed")

	// 3. 异 hash → 409 语义
	cmd2 := cmd
	cmd2.RequestSHA256 = "different-hash"
	if _, _, err = r.MaterializeAnalysisTaskAtomic(ctx, cmd2); err != ErrPayloadMismatch {
		t.Fatalf("expected ErrPayloadMismatch, got %v", err)
	}
	t.Logf("payload mismatch rejected")

	// 4. 回执 APPLIED
	fence := "fence-" + uuid.NewString()[:8]
	// 先派发 attempt(RUNNING)
	if _, err := db.ExecContext(ctx, `
		UPDATE analysis_stage_attempts SET state='RUNNING', fencing_token=$2
		WHERE run_id=$1 AND execution_node_id='SOURCE_ACTIVATE' AND attempt=1`, receipt.RunID, fence); err != nil {
		t.Fatalf("dispatch attempt: %v", err)
	}
	out, err := r.ApplyStageReceiptAtomic(ctx, ReceiptCommand{
		TenantID: tenant, RunID: receipt.RunID, EventID: "ev-" + uuid.NewString()[:8], TupleHash: "tuple-1",
		ExecutionNodeID: "SOURCE_ACTIVATE", Attempt: 1, FencingToken: fence,
		Provider: "pcap-replay-adapter", InputCount: 100, OutputCount: 100,
		WatermarkMs: time.Now().UnixMilli(), FenceJSON: []byte(`{"kind":"source_fence","packets":100}`),
		ExpectedState: "RUNNING", NewState: "SUCCEEDED",
	})
	if err != nil || !out.Applied {
		t.Fatalf("receipt: %v out=%+v", err, out)
	}
	// 旧 fence → STALE_FENCE
	stale, err := r.ApplyStageReceiptAtomic(ctx, ReceiptCommand{
		TenantID: tenant, RunID: receipt.RunID, EventID: "ev-" + uuid.NewString()[:8], TupleHash: "tuple-2",
		ExecutionNodeID: "SOURCE_ACTIVATE", Attempt: 1, FencingToken: "old-fence",
		ExpectedState: "SUCCEEDED", NewState: "SUCCEEDED",
	})
	if err != nil || stale.Outcome != "STALE_FENCE" {
		t.Fatalf("stale fence: %v out=%+v", err, stale)
	}
	t.Logf("receipt APPLIED + stale-fence quarantine verified")

	// 5. 终态:FAILED(required 节点失败语义)
	if err := r.TransitionRunAtomic(ctx, tenant, receipt.RunID, "ACCEPTED", "PREPARING"); err != nil {
		t.Fatalf("transition preparing: %v", err)
	}
	if err := r.TransitionRunAtomic(ctx, tenant, receipt.RunID, "PREPARING", "QUEUED"); err != nil {
		t.Fatalf("transition queued: %v", err)
	}
	if err := r.TransitionRunAtomic(ctx, tenant, receipt.RunID, "QUEUED", "RUNNING"); err != nil {
		t.Fatalf("transition running: %v", err)
	}
	if err := r.TransitionRunAtomic(ctx, tenant, receipt.RunID, "RUNNING", "FINALIZING"); err != nil {
		t.Fatalf("transition finalizing: %v", err)
	}
	err = r.FinalizeRunAtomic(ctx, FinalizeCommand{
		TenantID: tenant, RunID: receipt.RunID, ExpectedState: "FINALIZING", NewState: "FAILED",
		FindingConclusion: "NOT_EVALUATED", RiskSeverity: "UNKNOWN",
		Completeness: "INCOMPLETE", IntegrityState: "VERIFIED",
		ScopeJSON: []byte(`{}`), KeyFindingsJSON: []byte(`[]`), LimitationsJSON: []byte(`[]`),
		EvidenceEntriesJSON: []byte(`[]`), DecisionInputsJSON: []byte(`{"required_node_failed":true}`),
		NodeExactSetJSON: []byte(`[]`), DifferencesJSON: []byte(`[]`),
		Priority:       0,
		SummarySHA256:  "live-summary-sha-1",
		ClosureSHA256:  fmt.Sprintf("c-%d", time.Now().UnixNano()),
		EvidenceSHA256: fmt.Sprintf("e-%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	var runState string
	if err := db.QueryRowContext(ctx, `SELECT state FROM analysis_runs WHERE id=$1`, receipt.RunID).Scan(&runState); err != nil {
		t.Fatalf("read run: %v", err)
	}
	if runState != "FAILED" {
		t.Fatalf("run state=%s want FAILED", runState)
	}
	var released string
	_ = db.QueryRowContext(ctx, `SELECT state FROM analysis_admission_reservations WHERE run_id=$1`, receipt.RunID).Scan(&released)
	if released != "RELEASED" {
		t.Fatalf("reservation state=%s want RELEASED", released)
	}
	t.Logf("live PG cycle PASS: materialize→replay→receipt→fence→finalize(%s), reservation=%s", runState, released)

	// 6. 报告链:请求(终态+摘要)→worker 回执(VERIFYING)→verifier 确认(AVAILABLE)
	reportID, replayed, err := r.RequestHumanReportAtomic(ctx, tenant, receipt.RunID,
		"live-summary-sha-1", "default-v1", "zh-CN", "rh-"+uuid.NewString()[:8], "rep-idem-"+uuid.NewString()[:8])
	if err != nil || replayed {
		t.Fatalf("report request: %v replayed=%v", err, replayed)
	}
	objSHA := fmt.Sprintf("%064x", 42)
	if _, err := r.ApplyHumanReportReceiptAtomic(ctx, tenant, reportID, "reports/"+reportID+".pdf", objSHA, 1234, "v1", "live-summary-sha-1"); err != nil {
		t.Fatalf("report receipt: %v", err)
	}
	if err := r.ConfirmHumanReportObjectAtomic(ctx, tenant, reportID, objSHA, 1234); err != nil {
		t.Fatalf("report confirm: %v", err)
	}
	rv, err := r.GetReport(ctx, tenant, reportID)
	if err != nil || rv.State != "AVAILABLE" {
		t.Fatalf("report view: %v state=%v", err, func() string {
			if rv != nil {
				return rv.State
			}
			return "nil"
		}())
	}
	t.Logf("live PG report chain PASS: request→receipt→verify→AVAILABLE")

	// 清理(integration 租户数据)
	if _, err := db.ExecContext(ctx, `DELETE FROM analysis_history WHERE tenant_id=$1`, tenant); err != nil {
		t.Logf("cleanup history: %v", err)
	}
	// 按引用顺序清理
	for _, q := range []string{
		`DELETE FROM analysis_stage_receipts WHERE tenant_id=$1`,
		`DELETE FROM analysis_stage_queue WHERE tenant_id=$1`,
		`DELETE FROM analysis_stage_attempts WHERE tenant_id=$1`,
		`DELETE FROM analysis_business_phase_projections WHERE run_id IN (SELECT id FROM analysis_runs WHERE tenant_id=$1)`,
		`DELETE FROM analysis_admission_reservations WHERE tenant_id=$1`,
		`DELETE FROM analysis_human_reports WHERE tenant_id=$1`,
		`DELETE FROM analysis_machine_summaries WHERE tenant_id=$1`,
		`DELETE FROM analysis_evidence_manifests WHERE tenant_id=$1`,
		`DELETE FROM analysis_run_closure_manifests WHERE tenant_id=$1`,
		`DELETE FROM analysis_receipts WHERE tenant_id=$1`,
		`DELETE FROM analysis_outbox WHERE key IN (SELECT id::text FROM analysis_runs WHERE tenant_id=$1)`,
		`DELETE FROM analysis_runs WHERE tenant_id=$1`,
		`DELETE FROM analysis_tasks WHERE tenant_id=$1`,
		`DELETE FROM analysis_trigger_instances WHERE tenant_id=$1`,
		`DELETE FROM analysis_plan_revisions WHERE tenant_id=$1`,
		`DELETE FROM analysis_task_definitions WHERE tenant_id=$1`,
	} {
		if _, err := db.ExecContext(ctx, q, tenant); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM analysis_materialization_ledger WHERE identity_hash LIKE 'live-%'`); err != nil {
		t.Logf("cleanup ledger: %v", err)
	}
}

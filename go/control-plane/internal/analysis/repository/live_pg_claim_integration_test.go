//go:build integration

// live PG 集成测试(§76.45.3):阶段候选队列种子 + ClaimStageLeaseAtomic 单事务领取
// (queue/attempt CAS + DRR 更新(deficit += cost-quantum)+ 预留消费 + ACTIVE 订阅
// outbox + 审计同事务)+ DRR 稳定排序(先 ready 先选)。
package repository_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

func TestLivePGClaimStageLeaseSingleTransaction(t *testing.T) {
	db := liveDBP2(t)
	repo := repository.NewRepo(db)
	ctx := context.Background()
	tenant := "integration-claim-" + uuid.NewString()[:8]

	// 两个 run(先/后 ready)验证稳定排序;seed 经真实物化事务(带 queue 种子)
	seed := func(spec string, cost int64) (runID string) {
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
		receipt, replayed, err := repo.MaterializeAnalysisTaskAtomic(ctx, repository.MaterializeCommand{
			TenantID: tenant, IdentityKind: "actor", CanonicalIdentityHash: "cid-" + spec,
			RequestSHA256: "req-" + spec, TriggerInstanceID: triggerID, TriggerKind: "ON_DEMAND",
			WindowStartMs: time.Now().UnixMilli(), WindowEndMs: time.Now().Add(10 * time.Minute).UnixMilli(),
			TaskDefinitionID: defID, PlanRevision: 1, ExecutionSpecSHA256: spec,
			EffectiveClass: "BASELINE", EffectivePolicySHA256: "policy-1",
			ResourcePool: "analysis-cpu", ResourceVectorJSON: []byte(`{"cpu":2}`),
			QueueCostMilli: cost, ExpiresAt: time.Now().Add(5 * time.Minute),
			NodesJSON:    service.DefaultNodeExactSet(),
			PlanSpecJSON: []byte(`{}`),
		})
		if err != nil || replayed || receipt.RunID == "" {
			t.Fatalf("materialize seed: %+v replayed=%v err=%v", receipt, replayed, err)
		}
		return receipt.RunID
	}
	// 判别联合按租户尾段唯一化(tenant 前缀固定,不能取头部)。
	tenantTail := tenant[len(tenant)-8:]
	runA := seed("spec-"+tenantTail+"-a", 2500)
	runB := seed("spec-"+tenantTail+"-b", 5000)

	// 1. queue 种子:每 run 9 行、cost 正确
	var queueCount, costA int64
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), MAX(cost_milli) FROM analysis_stage_queue WHERE run_id=$1`, runA).Scan(&queueCount, &costA); err != nil || queueCount != 9 || costA != 2500 {
		t.Fatalf("queue seed runA: n=%d cost=%d err=%v", queueCount, costA, err)
	}

	// 2. 单事务领取:稳定排序先取 runA(先 ready)
	lease1, err := repo.ClaimStageLeaseAtomic(ctx, tenant, 5*time.Minute, time.Now())
	if err != nil || lease1 == nil || lease1.RunID != runA {
		t.Fatalf("claim 1: %+v err=%v", lease1, err)
	}
	if lease1.FencingToken == "" || lease1.CostMilli != 2500 {
		t.Fatalf("claim 1 fields: token=%q cost=%d", lease1.FencingToken, lease1.CostMilli)
	}
	var state, qState string
	if err := db.QueryRowContext(ctx, `SELECT state FROM analysis_stage_attempts WHERE id=$1`, lease1.AttemptID).Scan(&state); err != nil || state != "DISPATCHED" {
		t.Fatalf("attempt after claim: %s err=%v", state, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT state FROM analysis_stage_queue WHERE id=$1`, lease1.QueueID).Scan(&qState); err != nil || qState != "CLAIMED" {
		t.Fatalf("queue after claim: %s err=%v", qState, err)
	}
	var resState string
	if err := db.QueryRowContext(ctx, `SELECT state FROM analysis_admission_reservations WHERE run_id=$1`, runA).Scan(&resState); err != nil || resState != "CONSUMED" {
		t.Fatalf("reservation after claim: %s err=%v", resState, err)
	}
	var deficit, epoch, quantum int64
	if err := db.QueryRowContext(ctx, `
		SELECT deficit, scheduler_epoch, quantum FROM analysis_drr_state WHERE tenant_id=$1 AND scheduling_class='BASELINE'`,
		tenant).Scan(&deficit, &epoch, &quantum); err != nil || deficit != 1500 || epoch != 1 || quantum != 1000 {
		t.Fatalf("drr after claim1: deficit=%d epoch=%d quantum=%d err=%v", deficit, epoch, quantum, err)
	}
	var outboxCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM analysis_outbox WHERE key=$1 AND topic='analysis.run.events.v1' AND payload::text LIKE '%ACTIVE%'`,
		runA).Scan(&outboxCount); err != nil || outboxCount != 1 {
		t.Fatalf("active subscription outbox: %d err=%v", outboxCount, err)
	}
	var histCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM analysis_history WHERE tenant_id=$1 AND entity='stage_attempt' AND action='DISPATCHED' AND detail->>'queue_id'=$2`,
		tenant, lease1.QueueID).Scan(&histCount); err != nil || histCount != 1 {
		t.Fatalf("claim history: %d err=%v", histCount, err)
	}

	// 3. 第二次领取:runB;deficit 再 +5000-1000
	lease2, err := repo.ClaimStageLeaseAtomic(ctx, tenant, 5*time.Minute, time.Now())
	if err != nil || lease2 == nil || lease2.RunID != runB {
		t.Fatalf("claim 2: %+v err=%v", lease2, err)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT deficit FROM analysis_drr_state WHERE tenant_id=$1 AND scheduling_class='BASELINE'`,
		tenant).Scan(&deficit); err != nil || deficit != 5500 {
		t.Fatalf("drr after claim2: %d err=%v", deficit, err)
	}

	// 4. 无候选 → (nil, nil)
	if lease3, err := repo.ClaimStageLeaseAtomic(ctx, tenant, 5*time.Minute, time.Now()); err != nil || lease3 != nil {
		t.Fatalf("empty claim: %+v err=%v", lease3, err)
	}

	// 清理
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
		if _, err := db.ExecContext(context.Background(),
			`DELETE FROM analysis_materialization_ledger WHERE identity_hash IN ($1,$2)`,
			"cid-spec-"+tenant[len(tenant)-8:]+"-a", "cid-spec-"+tenant[len(tenant)-8:]+"-b"); err != nil {
			t.Logf("cleanup ledger warning: %v", err)
		}
		_ = db.Close()
	})
	t.Logf("claim lease single-transaction PASS: deficit=%d (2500-1000 + 5000-1000)", deficit)
}

func seedExec(t *testing.T, db *sql.DB, ctx context.Context, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

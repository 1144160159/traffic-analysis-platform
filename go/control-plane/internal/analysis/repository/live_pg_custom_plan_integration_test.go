//go:build integration

// live PG 集成测试(G04):人工选择列车(P2)全链——
// 草稿保存(修订自动分配/幂等回源)→ maker/checker 审批激活 →
// MANUAL_CUSTOM 触发绑定定制修订(异覆盖 409 拒绝)。
package repository_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/adapters"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

func liveDBP2(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ANALYSIS_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("ANALYSIS_TEST_PG_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLivePGCustomPlanSaveApproveTriggerChain(t *testing.T) {
	db := liveDBP2(t)
	defer db.Close()
	repo := repository.NewRepo(db)
	ctx := context.Background()

	tenant := "integration-p2-" + uuid.NewString()[:8]
	defID := uuid.NewString()
	planID := uuid.NewString()

	// 种子:定义 + AUTO_DEFAULT 激活计划(基座模板)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_task_definitions(id, tenant_id, name, state, owner, revision, created_by)
		VALUES($1,$2,$3,'ACTIVE',$2,1,$2)`, defID, tenant, "def-p2"); err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	spec := "p2-spec-" + uuid.NewString()[:8]
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_plan_revisions(id, tenant_id, task_definition_id, plan_revision, plan_source,
			source_kind, source_spec, selected_feature_ids, feature_set_id, encrypted_recognition_model_ref,
			threat_detector_refs, rule_refs, machine_summary_schema_ref, stage_dag, completion_policy,
			resource_budget, catalog_revision, execution_spec_sha256, plan_revision_sha256, created_by)
		VALUES($1,$2,$3,1,'AUTO_DEFAULT','PCAP_REPLAY','{"pcap_object":"s3://b/base.pcap"}'::jsonb,
			'["f1","f2","f3"]'::jsonb,'fs-v1','enc@v1','["det@v1"]'::jsonb,'["rule@v1"]'::jsonb,'summary-v1',
			'{"stages":["S1","S2","S3","S4","S5"]}'::jsonb,'{"allow_partial":false}'::jsonb,
			'{"cpu":2}'::jsonb,1,$4,$4,$2)`, planID, tenant, defID, spec); err != nil {
		t.Fatalf("seed default plan: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_plan_governance_heads(tenant_id, plan_id, state, authority_revision)
		VALUES($1,$2,'ACTIVE',1)`, tenant, planID); err != nil {
		t.Fatalf("seed head: %v", err)
	}

	compiler := service.NewPlanCompiler()
	loader := adapters.NewPGPlanTemplateProvider(repo).LoadTemplate
	author := service.NewPlanAuthorService(repo, compiler, service.NewCustomPlanResolver(compiler), loader)
	triggers := service.NewTriggerService(repo, service.NewDefaultPlanResolver(compiler), service.NewCustomPlanResolver(compiler), compiler)
	triggers.SetTemplateLoader(loader)

	// 1. 保存定制草稿(覆盖特征与检测器)
	overrides := json.RawMessage(`{"selected_feature_ids":["f1","f2"],"threat_detector_refs":["det@v1"]}`)
	resp, err := author.SaveCustom(ctx, service.SaveCustomPlanRequest{
		TenantID: tenant, TaskDefinitionID: defID, CustomOverrides: overrides,
		Actor: "op-1", ClientIdempotencyKey: "p2-key-1",
	})
	if err != nil {
		t.Fatalf("save custom: %v", err)
	}
	if resp.PlanRevision != 2 {
		t.Fatalf("expected auto-allocated revision 2, got %d", resp.PlanRevision)
	}
	t.Logf("custom draft saved plan=%s revision=%d spec=%s", resp.PlanID, resp.PlanRevision, resp.ExecutionSpecSHA256)

	// 2. 幂等回源:同 key 同覆盖 → 同一 plan id
	resp2, err := author.SaveCustom(ctx, service.SaveCustomPlanRequest{
		TenantID: tenant, TaskDefinitionID: defID, CustomOverrides: overrides,
		Actor: "op-1", ClientIdempotencyKey: "p2-key-1",
	})
	if err != nil {
		t.Fatalf("replay save custom: %v", err)
	}
	if resp2.PlanID != resp.PlanID || resp2.PlanRevision != resp.PlanRevision {
		t.Fatalf("idempotent replay diverged: %+v vs %+v", resp, resp2)
	}
	t.Logf("draft idempotent replay confirmed")

	// 3. 审批激活(maker≠checker)
	if err := author.Approve(ctx, service.ApprovePlanRequest{
		TenantID: tenant, PlanID: resp.PlanID, Maker: "maker-a", Checker: "checker-b",
	}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// 重复审批幂等
	if err := author.Approve(ctx, service.ApprovePlanRequest{
		TenantID: tenant, PlanID: resp.PlanID, Maker: "maker-a", Checker: "checker-b",
	}); err != nil {
		t.Fatalf("idempotent approve: %v", err)
	}
	t.Logf("approve → ACTIVE confirmed")

	// 4. 同覆盖触发 → 绑定定制修订(2)
	sub, err := triggers.Submit(ctx, service.SubmitRequest{
		TenantID: tenant, TaskDefinitionID: defID, PlanSource: "MANUAL_CUSTOM",
		CustomOverrides: overrides, Actor: "op-1", Approved: true, CustomReleased: true,
		ClientIdempotencyKey: "p2-trigger-1",
	})
	if err != nil {
		t.Fatalf("custom trigger: %v", err)
	}
	var boundRev int64
	if err := db.QueryRowContext(ctx, `SELECT plan_revision FROM analysis_tasks WHERE id=$1`, sub.TaskID).Scan(&boundRev); err != nil {
		t.Fatalf("read task plan revision: %v", err)
	}
	if boundRev != 2 {
		t.Fatalf("custom trigger must bind revision 2, got %d", boundRev)
	}
	t.Logf("custom trigger materialized task=%s run=%s bound revision=%d", sub.TaskID, sub.RunID, boundRev)

	// 5. 异覆盖触发 → 409 拒绝(不静默挂默认/旧修订)
	_, err = triggers.Submit(ctx, service.SubmitRequest{
		TenantID: tenant, TaskDefinitionID: defID, PlanSource: "MANUAL_CUSTOM",
		CustomOverrides: json.RawMessage(`{"selected_feature_ids":["f1"]}`), Actor: "op-1",
		Approved: true, CustomReleased: true, ClientIdempotencyKey: "p2-trigger-2",
	})
	if err == nil || !strings.Contains(err.Error(), string(contract.ErrCodePlanNotApproved)) {
		t.Fatalf("expected custom spec mismatch 409, got %v", err)
	}
	t.Logf("mismatched custom trigger rejected")

	// 清理
	for _, q := range []string{
		`DELETE FROM analysis_stage_queue WHERE tenant_id=$1`,
		`DELETE FROM analysis_stage_attempts WHERE tenant_id=$1`,
		`DELETE FROM analysis_business_phase_projections WHERE run_id IN (SELECT id FROM analysis_runs WHERE tenant_id=$1)`,
		`DELETE FROM analysis_admission_reservations WHERE tenant_id=$1`,
		`DELETE FROM analysis_receipts WHERE tenant_id=$1`,
		`DELETE FROM analysis_outbox WHERE key IN (SELECT id::text FROM analysis_runs WHERE tenant_id=$1)`,
		`DELETE FROM analysis_runs WHERE tenant_id=$1`,
		`DELETE FROM analysis_tasks WHERE tenant_id=$1`,
		`DELETE FROM analysis_trigger_instances WHERE tenant_id=$1`,
		`DELETE FROM analysis_plan_approvals WHERE tenant_id=$1`,
		`DELETE FROM analysis_plan_governance_heads WHERE plan_id IN (SELECT id FROM analysis_plan_revisions WHERE tenant_id=$1)`,
		`DELETE FROM analysis_plan_revisions WHERE tenant_id=$1`,
		`DELETE FROM analysis_task_definitions WHERE tenant_id=$1`,
	} {
		if _, err := db.ExecContext(ctx, q, tenant); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM analysis_materialization_ledger WHERE identity_hash LIKE 'p2-%' OR identity_hash LIKE 'plan-%'`); err != nil {
		t.Logf("cleanup ledger: %v", err)
	}
}

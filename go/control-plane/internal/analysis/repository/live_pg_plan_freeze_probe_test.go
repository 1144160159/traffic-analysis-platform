//go:build integration

// live PG 探针:扫描全部计划修订,比较冻结 execution_spec_sha256 与
// 修复后(带 machine_summary_schema_ref)编译器输出,列出漂移行。
// 该测试只读不写;漂移行需人工重冻结(新修订),不得静默改冻结行。
package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/adapters"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

func TestLivePGPlanFreezeDriftProbe(t *testing.T) {
	db := liveDBP2(t)
	defer db.Close()
	repo := repository.NewRepo(db)
	ctx := context.Background()
	compiler := service.NewPlanCompiler()

	rows, err := db.QueryContext(ctx, `
		SELECT tenant_id, task_definition_id::text, plan_revision, plan_source
		FROM analysis_plan_revisions ORDER BY tenant_id, task_definition_id, plan_revision`)
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	defer rows.Close()

	drifts := 0
	for rows.Next() {
		var tenant, defID string
		var rev int64
		var source string
		if err := rows.Scan(&tenant, &defID, &rev, &source); err != nil {
			t.Fatal(err)
		}
		row, err := repo.GetActivePlanForDefinitionBySource(ctx, tenant, defID, source)
		if err != nil {
			continue // 非 ACTIVE(治理头非 ACTIVE)不参与冻结一致性
		}
		if row.PlanRevision != rev {
			continue // 只比较 ACTIVE 修订
		}
		tpl, catalog, err := adapters.BuildTemplateFromPlanRow(defID, row)
		if err != nil {
			t.Fatalf("build template %s rev%d: %v", defID, rev, err)
		}
		var intent *service.NormalizedAnalysisIntent
		if source == "AUTO_DEFAULT" {
			intent, err = service.NewDefaultPlanResolver(compiler).Resolve(ctx, service.ResolveRequest{
				TenantID: tenant, TaskDefinitionID: defID, PlanSource: "AUTO_DEFAULT",
				Catalog: catalog, Template: tpl,
			})
		} else {
			intent, err = service.NewCustomPlanResolver(compiler).Resolve(ctx, service.ResolveRequest{
				TenantID: tenant, TaskDefinitionID: defID, PlanSource: "MANUAL_CUSTOM",
				Catalog: catalog, Template: tpl, CustomOverrides: json.RawMessage(`{"rule_refs":["rule@v1"]}`),
				Approved: true, CustomReleased: true, Actor: "probe",
			})
		}
		if err != nil {
			t.Fatalf("resolve %s rev%d: %v", defID, rev, err)
		}
		compiled, err := compiler.Compile(ctx, *intent)
		if err != nil {
			t.Fatalf("compile %s rev%d: %v", defID, rev, err)
		}
		// 对照:丢字段编译(旧装配 bug 的产物),判断漂移是否同一根因
		intent.MachineSummarySchemaRef = ""
		compiledNoField, _ := compiler.Compile(ctx, *intent)
		status := "OK"
		if compiled.ExecutionSpecSHA256 != row.ExecutionSpecSHA256 {
			status = "DRIFT"
			drifts++
		}
		root := "?"
		switch {
		case compiled.ExecutionSpecSHA256 == row.ExecutionSpecSHA256:
			root = "none"
		case compiledNoField.ExecutionSpecSHA256 == row.ExecutionSpecSHA256:
			root = "missing-schema-field"
		default:
			root = "other"
		}
		t.Logf("[%s|%s] tenant=%s def=%s rev=%d source=%s frozen=%s withField=%s noField=%s",
			status, root, tenant, defID[:8], rev, source, row.ExecutionSpecSHA256[:12], compiled.ExecutionSpecSHA256[:12], compiledNoField.ExecutionSpecSHA256[:12])
	}
	_ = fmt.Sprint
	t.Logf("drift rows: %d (需重冻结)", drifts)
}

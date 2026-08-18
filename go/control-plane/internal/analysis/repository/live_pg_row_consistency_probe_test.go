//go:build integration

// live PG 探针:对每个 ACTIVE 计划修订,直接用行内容构造 intent(不走 resolver
// 的覆盖语义),编译并比较冻结哈希——验证行内容与冻结哈希的内部一致性。
package repository_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

func TestLivePGPlanRowConsistency(t *testing.T) {
	db := liveDBP2(t)
	defer db.Close()
	repo := repository.NewRepo(db)
	ctx := context.Background()
	compiler := service.NewPlanCompiler()

	rows, err := db.QueryContext(ctx, `
		SELECT tenant_id, task_definition_id::text, plan_revision, plan_source
		FROM analysis_plan_revisions WHERE tenant_id='default' ORDER BY task_definition_id, plan_revision`)
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	defer rows.Close()

	type row struct {
		tenant, defID, source string
		rev                   int64
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.tenant, &r.defID, &r.rev, &r.source); err != nil {
			t.Fatal(err)
		}
		all = append(all, r)
	}
	bad := 0
	for _, r := range all {
		// 仅 ACTIVE 修订
		active, err := repo.GetActivePlanForDefinitionBySource(ctx, r.tenant, r.defID, r.source)
		if err != nil || active.PlanRevision != r.rev {
			continue
		}
		// 直接用行内容构造 intent(机器摘要字段来自行)
		intent := service.NormalizedAnalysisIntent{
			TenantID:                     r.tenant,
			TaskDefinitionID:             r.defID,
			PlanSource:                   active.PlanSource,
			SourceKind:                   active.SourceKind,
			SourceSpec:                   active.SourceSpec,
			FeatureSetID:                 active.FeatureSetID,
			EncryptedRecognitionModelRef: active.RecognitionModel,
			MachineSummarySchemaRef:      active.MachineSummarySchemaRef,
			StageDAG:                     active.StageDAG,
			CompletionPolicy:             active.CompletionPolicy,
			ResourceBudget:               active.ResourceBudget,
			CatalogRevision:              active.CatalogRevision,
		}
		var feats, dets, rules []string
		_ = json.Unmarshal(active.SelectedFeatureIDs, &feats)
		_ = json.Unmarshal(active.DetectorRefs, &dets)
		_ = json.Unmarshal(active.RuleRefs, &rules)
		intent.SelectedFeatureIDs = feats
		intent.ThreatDetectorRefs = dets
		intent.RuleRefs = rules
		compiled, err := compiler.Compile(ctx, intent)
		if err != nil {
			t.Fatalf("compile %s rev%d: %v", r.defID[:8], r.rev, err)
		}
		ok := compiled.ExecutionSpecSHA256 == active.ExecutionSpecSHA256
		if !ok {
			bad++
		}
		t.Logf("[%s] def=%s rev=%d source=%s frozen=%s rowContent=%s",
			map[bool]string{true: "CONSISTENT", false: "INCONSISTENT"}[ok],
			r.defID[:8], r.rev, r.source, active.ExecutionSpecSHA256[:12], compiled.ExecutionSpecSHA256[:12])
	}
	if bad > 0 {
		t.Fatalf("%d ACTIVE rows internally inconsistent", bad)
	}
}

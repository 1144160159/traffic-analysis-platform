package service

import (
	"context"
	"errors"
	"testing"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

func validIntent() NormalizedAnalysisIntent {
	return NormalizedAnalysisIntent{
		TenantID:           "tenant-a",
		TaskDefinitionID:   "def-1",
		PlanSource:         "AUTO_DEFAULT",
		SourceKind:         "LIVE_STREAM_WINDOW",
		SourceSpec:         []byte(`{"window_ms":60000}`),
		SelectedFeatureIDs: []string{"pktlen_mean", "iat_mean_ms"},
		FeatureSetID:       "fs-v1",
		ThreatDetectorRefs: []string{"rule-scan-detect@v3", "behavior-known@v7"},
		RuleRefs:           []string{"rule-scan-detect@v3"},
		StageDAG:           []byte(`{"stages":["S1","S2","S3","S4","S5"]}`),
		CompletionPolicy:   []byte(`{"allow_partial":false,"zero_input_policy":"ALLOW_NO_DATA"}`),
		ResourceBudget:     []byte(`{"cpu":2,"mem_mb":4096}`),
		CatalogRevision:    12,
		SelectionOrigins:   []string{"default-template"},
	}
}

func TestPlanCompilerDeterministicDoubleHash(t *testing.T) {
	c := NewPlanCompiler()
	a, err := c.Compile(context.Background(), validIntent())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	b, err := c.Compile(context.Background(), validIntent())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if a.ExecutionSpecSHA256 != b.ExecutionSpecSHA256 || a.PlanRevisionSHA256 != b.PlanRevisionSHA256 {
		t.Fatalf("compilation not deterministic")
	}
}

func TestPlanCompilerFieldOrderInsensitive(t *testing.T) {
	c := NewPlanCompiler()
	i1 := validIntent()
	i1.SelectedFeatureIDs = []string{"pktlen_mean", "iat_mean_ms"}
	i2 := validIntent()
	i2.SelectedFeatureIDs = []string{"iat_mean_ms", "pktlen_mean"}
	a, _ := c.Compile(context.Background(), i1)
	b, _ := c.Compile(context.Background(), i2)
	if a.ExecutionSpecSHA256 != b.ExecutionSpecSHA256 {
		t.Fatalf("field order must not change execution hash")
	}
}

func TestPlanCompilerPlanSourceExcludedFromExecutionHash(t *testing.T) {
	c := NewPlanCompiler()
	i1 := validIntent()
	i1.PlanSource = "AUTO_DEFAULT"
	i1.SelectionOrigins = []string{"default"}
	i2 := validIntent()
	i2.PlanSource = "MANUAL_CUSTOM"
	i2.SelectionOrigins = []string{"user:analyst-1", "field:selected_feature_ids"}
	a, _ := c.Compile(context.Background(), i1)
	b, _ := c.Compile(context.Background(), i2)
	if a.ExecutionSpecSHA256 != b.ExecutionSpecSHA256 {
		t.Fatalf("plan_source/selection_origins must not enter execution_spec_sha256")
	}
	if a.PlanRevisionSHA256 == b.PlanRevisionSHA256 {
		t.Fatalf("plan_revision_sha256 must include plan_source/origins")
	}
}

func TestPlanCompilerValidation(t *testing.T) {
	c := NewPlanCompiler()
	cases := []struct {
		name   string
		mutate func(*NormalizedAnalysisIntent)
	}{
		{"empty tenant", func(i *NormalizedAnalysisIntent) { i.TenantID = "" }},
		{"no feature exact-set", func(i *NormalizedAnalysisIntent) { i.SelectedFeatureIDs = nil; i.FeatureSetID = "" }},
		{"unknown source kind", func(i *NormalizedAnalysisIntent) { i.SourceKind = "WEIRD" }},
		{"no detectors", func(i *NormalizedAnalysisIntent) { i.ThreatDetectorRefs = nil }},
	}
	for _, cse := range cases {
		i := validIntent()
		cse.mutate(&i)
		if _, err := c.Compile(context.Background(), i); err == nil {
			t.Errorf("%s: expected error", cse.name)
		} else {
			var ae *commonerrors.AppError
			if !errors.As(err, &ae) || ae.Code != contract.ErrCodeInvalidTransition {
				t.Errorf("%s: expected analysis error, got %v", cse.name, err)
			}
		}
	}
}

func TestPlanCompilerOverridesChangeHash(t *testing.T) {
	c := NewPlanCompiler()
	i1 := validIntent()
	i2 := validIntent()
	i2.ThreatDetectorRefs = append(i2.ThreatDetectorRefs, "behavior-extra@v1")
	a, _ := c.Compile(context.Background(), i1)
	b, _ := c.Compile(context.Background(), i2)
	if a.ExecutionSpecSHA256 == b.ExecutionSpecSHA256 {
		t.Fatalf("detector override must change execution hash")
	}
}

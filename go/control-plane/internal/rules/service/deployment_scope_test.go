package service

import (
	"reflect"
	"testing"
)

func TestMergeDeploymentScope(t *testing.T) {
	current := map[string]interface{}{
		"tenant":      "租户A",
		"campus":      "华东园区",
		"probe_group": "办公区探针组 (12)",
		"asset_group": "核心业务资产组",
		"percentage":  float64(20),
	}
	merged, err := mergeDeploymentScope(current, map[string]interface{}{
		"campus":     "华南园区",
		"percentage": 50,
	})
	if err != nil {
		t.Fatalf("mergeDeploymentScope returned error: %v", err)
	}
	if got := merged["campus"]; got != "华南园区" {
		t.Fatalf("campus = %v, want 华南园区", got)
	}
	if got := merged["percentage"]; got != float64(50) {
		t.Fatalf("percentage = %v, want 50", got)
	}
	if current["campus"] != "华东园区" {
		t.Fatal("mergeDeploymentScope mutated current scope")
	}
}

func TestMergeDeploymentScopeValidation(t *testing.T) {
	tests := []struct {
		name  string
		scope map[string]interface{}
	}{
		{name: "missing percentage", scope: map[string]interface{}{}},
		{name: "percentage too high", scope: map[string]interface{}{"percentage": 101}},
		{name: "percentage is text", scope: map[string]interface{}{"percentage": "20"}},
		{name: "empty campus", scope: map[string]interface{}{"percentage": 20, "campus": "  "}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if merged, err := mergeDeploymentScope(nil, test.scope); err == nil || !reflect.DeepEqual(merged, map[string]interface{}(nil)) {
				t.Fatalf("mergeDeploymentScope(%v) = %v, %v; want nil, error", test.scope, merged, err)
			}
		})
	}
}

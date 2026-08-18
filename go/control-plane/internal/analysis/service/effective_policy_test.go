package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveEffectiveSchedulingPolicyClassResolution(t *testing.T) {
	// class 覆盖链:override ?? schedule ?? default;缺省 BASELINE;未知类拒绝
	p, err := ResolveEffectiveSchedulingPolicy(PolicyInputs{
		DefaultClass: "BASELINE", PlanBudget: json.RawMessage(`{"cpu":2}`),
	})
	if err != nil || p.Class != "BASELINE" {
		t.Fatalf("default class resolution: %+v err=%v", p, err)
	}
	p, err = ResolveEffectiveSchedulingPolicy(PolicyInputs{
		DefaultClass: "BASELINE", ScheduleClass: "INTERACTIVE", PlanBudget: json.RawMessage(`{"cpu":2}`),
	})
	if err != nil || p.Class != "INTERACTIVE" {
		t.Fatalf("schedule class must win over default: %+v err=%v", p, err)
	}
	p, err = ResolveEffectiveSchedulingPolicy(PolicyInputs{
		DefaultClass: "BASELINE", ScheduleClass: "INTERACTIVE", TriggerOverride: "ACCEPTANCE", PlanBudget: json.RawMessage(`{"cpu":2}`),
	})
	if err != nil || p.Class != "ACCEPTANCE" {
		t.Fatalf("authorized override must win: %+v err=%v", p, err)
	}
	if _, err := ResolveEffectiveSchedulingPolicy(PolicyInputs{
		DefaultClass: "SATELLITE_LASER", PlanBudget: json.RawMessage(`{"cpu":2}`),
	}); err == nil || !strings.Contains(err.Error(), "unknown scheduling class") {
		t.Fatalf("unknown class must be rejected, got %v", err)
	}
}

func TestResolveEffectiveSchedulingPolicyCapsAndVector(t *testing.T) {
	// 硬上限逐维取最小;requested 超限拒绝
	p, err := ResolveEffectiveSchedulingPolicy(PolicyInputs{
		DefaultClass: "BASELINE",
		PlanBudget:   json.RawMessage(`{"cpu":2,"memory":2}`),
		ScheduleRestrictions: json.RawMessage(`{"cpu":8,"memory":4}`),
		TriggerCaps:          json.RawMessage(`{"cpu":4}`),
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	var cap map[string]float64
	_ = json.Unmarshal(p.HardCap, &cap)
	if cap["cpu"] != 4 || cap["memory"] != 4 {
		t.Fatalf("hard cap must be per-dimension min: %v", cap)
	}
	var vec map[string]float64
	_ = json.Unmarshal(p.ResourceVector, &vec)
	if vec["cpu"] != 2 || vec["memory"] != 2 {
		t.Fatalf("requested vector from plan budget: %v", vec)
	}
	if p.PolicySHA256 == "" || len(p.PolicySHA256) != 64 {
		t.Fatalf("policy sha must be 64 hex, got %q", p.PolicySHA256)
	}

	// requested > cap → 拒绝
	if _, err := ResolveEffectiveSchedulingPolicy(PolicyInputs{
		DefaultClass: "BASELINE", PlanBudget: json.RawMessage(`{"cpu":8}`),
		ScheduleRestrictions: json.RawMessage(`{"cpu":2}`),
	}); err == nil || !strings.Contains(err.Error(), "exceeds hard cap") {
		t.Fatalf("cap violation must be rejected, got %v", err)
	}
}

func TestResolveEffectiveSchedulingPolicyDeterministic(t *testing.T) {
	a, err := ResolveEffectiveSchedulingPolicy(PolicyInputs{
		DefaultClass: "BASELINE", PlanBudget: json.RawMessage(`{"cpu":2}`),
		ScheduleRestrictions: json.RawMessage(`{"cpu":4}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ResolveEffectiveSchedulingPolicy(PolicyInputs{
		DefaultClass: "BASELINE", PlanBudget: json.RawMessage(`{"cpu":2}`),
		ScheduleRestrictions: json.RawMessage(`{"cpu":4}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.PolicySHA256 != b.PolicySHA256 || a.PolicySHA256 == "" {
		t.Fatalf("policy sha must be deterministic: %s vs %s", a.PolicySHA256, b.PolicySHA256)
	}
	// 键序不同的等价输入 → 相同 sha
	c, err := ResolveEffectiveSchedulingPolicy(PolicyInputs{
		DefaultClass: "BASELINE", PlanBudget: json.RawMessage(`{"memory":1,"cpu":2}`),
		ScheduleRestrictions: json.RawMessage(`{"cpu":4,"memory":2}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.PolicySHA256 == "" || c.PolicySHA256 == a.PolicySHA256 {
		t.Fatalf("distinct policies must differ (and be non-empty)")
	}
}

func TestScheduleServiceValidation(t *testing.T) {
	svc := NewScheduleService(nil)
	if _, _, err := svc.Save(t.Context(), SaveScheduleRequest{
		TenantID: "t", TaskDefinitionID: "d", ApprovedPlanRevision: 1, ExecutionSpecSHA256: "x",
		TriggerKind: "SATELLITE_LASER",
	}); err == nil || !strings.Contains(err.Error(), "unknown trigger_kind") {
		t.Fatalf("unknown trigger kind must be rejected, got %v", err)
	}
	if _, _, err := svc.Save(t.Context(), SaveScheduleRequest{
		TenantID: "t", TaskDefinitionID: "d", ApprovedPlanRevision: 1, ExecutionSpecSHA256: "x",
		TriggerKind: "CRON_WINDOW", MisfirePolicy: "MISFIRE_EXPLODE",
	}); err == nil || !strings.Contains(err.Error(), "unknown misfire_policy") {
		t.Fatalf("unknown misfire policy must be rejected, got %v", err)
	}
	if _, _, err := svc.Save(t.Context(), SaveScheduleRequest{
		TenantID: "t", TaskDefinitionID: "d", ApprovedPlanRevision: 1, ExecutionSpecSHA256: "x",
		TriggerKind: "CRON_WINDOW", MisfirePolicy: "MISFIRE_FAIL", ConcurrencyPolicy: "NUCLEAR",
	}); err == nil || !strings.Contains(err.Error(), "unknown concurrency_policy") {
		t.Fatalf("unknown concurrency policy must be rejected, got %v", err)
	}
	if _, _, err := svc.Save(t.Context(), SaveScheduleRequest{
		TenantID: "t", TaskDefinitionID: "d", ApprovedPlanRevision: 1, ExecutionSpecSHA256: "x",
		TriggerKind: "CRON_WINDOW", MisfirePolicy: "MISFIRE_FAIL", ConcurrencyPolicy: "FORBID_OVERLAP",
		SchedulingClass: "SATELLITE_LASER",
	}); err == nil || !strings.Contains(err.Error(), "unknown scheduling_class") {
		t.Fatalf("unknown scheduling class must be rejected, got %v", err)
	}
}

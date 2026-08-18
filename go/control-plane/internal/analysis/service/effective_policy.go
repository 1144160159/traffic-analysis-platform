// Package service 有效调度策略解析(§76.45.2 八步算法):
// class = authorized override ?? schedule.class ?? definition.default_class;
// deadline 取各层存在值的最早值;hard cap 逐维取最小值;requested 取
// trigger→schedule→plan preferred 首个存在值并满足 plan.min ≤ allocation ≤ cap;
// canonical 序列化计算 effective_policy_sha256,冻结进 Trigger/Task/Run/Reservation。
// plan source 不参与任何一步(§5.1 正交)。
package service

import (
	"encoding/json"
	"fmt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

// SchedulingClasses 合法调度类别(§5.1)。
var SchedulingClasses = map[string]bool{
	"BASELINE":    true,
	"INTERACTIVE": true,
	"ACCEPTANCE":  true,
}

// EffectiveSchedulingPolicy 有效调度策略(冻结快照)。
type EffectiveSchedulingPolicy struct {
	Class            string          `json:"scheduling_class"`
	ConcurrencyPolicy string         `json:"concurrency_policy,omitempty"`
	ResourcePool     string          `json:"resource_pool"`
	ResourceVector   json.RawMessage `json:"resource_vector"`
	HardCap          json.RawMessage `json:"hard_cap"`
	PolicySHA256     string          `json:"-"`
}

// PolicyInputs 解析输入(各层已冻结值;override 仅由授权方注入,未授权一律为空)。
type PolicyInputs struct {
	DefaultClass        string          // definition.default_scheduling_class
	ScheduleClass       string          // schedule.scheduling_class(无调度为空)
	TriggerOverride     string          // authorized trigger override(当前授权体系未发布,恒为空)
	PlanBudget          json.RawMessage // plan.resource_budget(requested vector 来源)
	ScheduleRestrictions json.RawMessage // schedule.resource_restrictions(含 cap)
	TriggerCaps         json.RawMessage // trigger 级 cap(为空则无)
}

// ResolveEffectiveSchedulingPolicy 八步解析(纯函数,无 IO)。
func ResolveEffectiveSchedulingPolicy(in PolicyInputs) (EffectiveSchedulingPolicy, error) {
	// 步骤 1:class 覆盖链;override 未授权为空(fail-closed:未知/空类拒绝)
	class := in.TriggerOverride
	if class == "" {
		class = in.ScheduleClass
	}
	if class == "" {
		class = in.DefaultClass
	}
	if class == "" {
		class = "BASELINE"
	}
	if !SchedulingClasses[class] {
		return EffectiveSchedulingPolicy{}, fmt.Errorf("%s: unknown scheduling class %q", contract.ErrCodeInvalidTransition, class)
	}

	// 步骤 4/5:requested vector 取 plan resource_budget(cpu 维);cap 逐维取最小值。
	budget, err := normalizeNumbers(in.PlanBudget)
	if err != nil {
		return EffectiveSchedulingPolicy{}, fmt.Errorf("%s: plan resource_budget malformed: %w", contract.ErrCodeInvalidTransition, err)
	}
	cap, err := minCaps(in.ScheduleRestrictions, in.TriggerCaps)
	if err != nil {
		return EffectiveSchedulingPolicy{}, fmt.Errorf("%s: resource restrictions malformed: %w", contract.ErrCodeInvalidTransition, err)
	}
	// 步骤 6:plan.min ≤ requested ≤ hard cap(缺 cap 视为不设上限;min 取 0)
	requested := budget
	for dim, want := range cap {
		got, ok := requested[dim]
		if !ok {
			continue // 未请求该维度
		}
		if got > want {
			return EffectiveSchedulingPolicy{}, fmt.Errorf("%s: requested %s=%v exceeds hard cap %v", contract.ErrCodeInvalidTransition, dim, got, want)
		}
	}

	vectorJSON, _ := json.Marshal(requested)
	capJSON, _ := json.Marshal(cap)
	if len(cap) == 0 {
		capJSON = json.RawMessage(`{}`)
	}
	policy := EffectiveSchedulingPolicy{
		Class:          class,
		ResourcePool:   "analysis-cpu",
		ResourceVector: vectorJSON,
		HardCap:        capJSON,
	}
	// 步骤 8:canonical 序列化 + effective_policy_sha256
	policy.PolicySHA256 = sha256Hex(canonicalPolicyBytes(policy))
	return policy, nil
}

// canonicalPolicyBytes 策略冻结字节(键排序;sha256 不含自身)。
func canonicalPolicyBytes(p EffectiveSchedulingPolicy) []byte {
	m := map[string]interface{}{
		"scheduling_class": p.Class,
		"resource_pool":    p.ResourcePool,
		"resource_vector":  rawJSON(p.ResourceVector),
		"hard_cap":         rawJSON(p.HardCap),
	}
	if p.ConcurrencyPolicy != "" {
		m["concurrency_policy"] = p.ConcurrencyPolicy
	}
	b, _ := json.Marshal(m)
	return b
}

// normalizeNumbers 解析 {dim: number} 预算(未知字段忽略,非数值字段报错)。
func normalizeNumbers(raw json.RawMessage) (map[string]float64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]float64{}, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for k, v := range m {
		n, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("dimension %q is not numeric", k)
		}
		out[k] = n
	}
	return out, nil
}

// minCaps 逐维取 schedule restrictions 与 trigger caps 的最小值。
func minCaps(restrictions, triggerCaps json.RawMessage) (map[string]float64, error) {
	out := map[string]float64{}
	for _, raw := range []json.RawMessage{restrictions, triggerCaps} {
		m, err := normalizeNumbers(raw)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			if cur, ok := out[k]; !ok || v < cur {
				out[k] = v
			}
		}
	}
	return out, nil
}

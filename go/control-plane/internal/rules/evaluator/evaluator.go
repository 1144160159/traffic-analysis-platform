// Rule Evaluation Engine — 规则试运行与命中率评估
//
// 业务价值: 规则上线前可以试运行评估效果，避免误报泛滥
// 功能: Dry-run 模拟、命中率统计、规则优化建议、A/B测试支持
package evaluator

import (
	"context"
	"crypto/md5"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

// EvalResult 单次评估结果
type EvalResult struct {
	RuleID      string                 `json:"rule_id"`
	RuleName    string                 `json:"rule_name"`
	Matched     bool                   `json:"matched"`
	MatchFields map[string]interface{} `json:"match_fields,omitempty"` // 哪些字段匹配了
	Score       float64                `json:"score"`                  // 匹配得分
	Reason      string                 `json:"reason"`                 // 匹配/不匹配原因
	EvalTimeMs  float64                `json:"eval_time_ms"`           // 评估耗时
}

// SimulationInput 模拟输入数据
type SimulationInput struct {
	TenantID   string                 `json:"tenant_id"`
	DataPoints []DataPoint            `json:"data_points"`
	Duration   time.Duration          `json:"duration"`    // 模拟时间跨度
	Rate       int                    `json:"rate"`        // 每秒事件数
}

// DataPoint 数据点
type DataPoint struct {
	Timestamp   time.Time              `json:"timestamp"`
	CommunityID string                 `json:"community_id"`
	Fields      map[string]interface{} `json:"fields"`
}

// SimulationReport 模拟报告
type SimulationReport struct {
	SimulationID   string              `json:"simulation_id"`
	RuleID         string              `json:"rule_id"`
	RuleName       string              `json:"rule_name"`
	TotalEvents    int                 `json:"total_events"`
	MatchedEvents  int                 `json:"matched_events"`
	MatchRate      float64             `json:"match_rate"`       // 命中率
	AvgEvalTimeMs  float64             `json:"avg_eval_time_ms"`
	P50EvalTimeMs  float64             `json:"p50_eval_time_ms"`
	P99EvalTimeMs  float64             `json:"p99_eval_time_ms"`
	FalsePositives int                 `json:"false_positives"`   // 预期误报数
	Precision      float64             `json:"precision"`         // 预估精确度
	Recommendation string              `json:"recommendation"`    // 上线建议
	RiskLevel      string              `json:"risk_level"`        // 风险等级
	TopMatchFields []FieldMatchCount   `json:"top_match_fields"`  // 最常匹配的字段
	Warnings       []string            `json:"warnings,omitempty"`// 告警
}

// FieldMatchCount 字段匹配计数
type FieldMatchCount struct {
	Field string  `json:"field"`
	Count int     `json:"count"`
	Ratio float64 `json:"ratio"`
}

// RuleAnalyzer 规则分析器
type RuleAnalyzer struct {
	EvaluationHistory map[string][]EvalResult // rule_id → 历史评估
	mu                sync.RWMutex
	logger            *zap.Logger
}

// NewRuleAnalyzer 创建规则分析器
func NewRuleAnalyzer(logger *zap.Logger) *RuleAnalyzer {
	return &RuleAnalyzer{
		EvaluationHistory: make(map[string][]EvalResult),
		logger:            logger,
	}
}

// DryRun 试运行规则（不触发告警，仅评估命中情况）
func (a *RuleAnalyzer) DryRun(ctx context.Context, rule *model.Rule, input *SimulationInput) (*SimulationReport, error) {
	startTime := time.Now()

	report := &SimulationReport{
		SimulationID: generateSimID(rule.RuleID, startTime),
		RuleID:       rule.RuleID,
		RuleName:     rule.Name,
		TotalEvents:  len(input.DataPoints),
	}

	var evalTimes []float64
	var fieldMatchCounts = make(map[string]int)

	for _, dp := range input.DataPoints {
		evalStart := time.Now()
		result := a.evaluateRule(rule, dp)
		evalMs := float64(time.Since(evalStart).Microseconds()) / 1000.0
		evalTimes = append(evalTimes, evalMs)

		if result.Matched {
			report.MatchedEvents++
			for field := range result.MatchFields {
				fieldMatchCounts[field]++
			}
		}
	}

	// 统计
	report.MatchRate = float64(report.MatchedEvents) / float64(report.TotalEvents)
	report.AvgEvalTimeMs = average(evalTimes)
	sort.Float64s(evalTimes)
	report.P50EvalTimeMs = percentile(evalTimes, 50)
	report.P99EvalTimeMs = percentile(evalTimes, 99)

	// 最常匹配字段排行
	report.TopMatchFields = topFields(fieldMatchCounts, report.MatchedEvents, 5)

	// 生成建议
	report.Recommendation, report.RiskLevel, report.Warnings = a.generateRecommendation(report, rule)

	// 记录历史
	a.mu.Lock()
	a.EvaluationHistory[rule.RuleID] = append(a.EvaluationHistory[rule.RuleID], EvalResult{
		RuleID:     rule.RuleID,
		RuleName:   rule.Name,
		Matched:    report.MatchedEvents > 0,
		Score:      report.MatchRate,
		Reason:     report.Recommendation,
		EvalTimeMs: float64(time.Since(startTime).Milliseconds()),
	})
	a.mu.Unlock()

	a.logger.Info("Rule dry-run completed",
		zap.String("rule_id", rule.RuleID),
		zap.String("rule_name", rule.Name),
		zap.Int("total", report.TotalEvents),
		zap.Int("matched", report.MatchedEvents),
		zap.Float64("match_rate", report.MatchRate),
		zap.String("recommendation", report.Recommendation))

	return report, nil
}

// evaluateRule 评估单条规则
func (a *RuleAnalyzer) evaluateRule(rule *model.Rule, dp DataPoint) EvalResult {
	result := EvalResult{
		RuleID:      rule.RuleID,
		RuleName:    rule.Name,
		MatchFields: make(map[string]interface{}),
	}

	conditions := rule.Conditions
	if conditions == nil {
		result.Reason = "no conditions defined"
		return result
	}

	// 根据规则类型评估
	switch model.RuleType(rule.Type) {
	case model.RuleTypeThreshold:
		result = a.evalThreshold(rule, dp, conditions)
	case model.RuleTypeSignature:
		result = a.evalSignature(rule, dp, conditions)
	case model.RuleTypeAnomaly:
		result = a.evalAnomaly(rule, dp, conditions)
	case model.RuleTypeCorrelation:
		result = a.evalCorrelation(rule, dp, conditions)
	default:
		result = a.evalGeneric(rule, dp, conditions)
	}

	return result
}

// evalThreshold 阈值规则评估
func (a *RuleAnalyzer) evalThreshold(rule *model.Rule, dp DataPoint, conditions map[string]interface{}) EvalResult {
	result := EvalResult{RuleID: rule.RuleID, RuleName: rule.Name, MatchFields: make(map[string]interface{})}

	threshold, ok := getFloat(conditions, "threshold")
	if !ok {
		result.Reason = "missing threshold value"
		return result
	}

	field, _ := getString(conditions, "field")
	if field == "" {
		field = "value" // 默认字段
	}

	operator, _ := getString(conditions, "operator")
	if operator == "" {
		operator = "gte"
	}

	value, ok := getFloat(dp.Fields, field)
	if !ok {
		result.Reason = fmt.Sprintf("field '%s' not found or not numeric", field)
		return result
	}

	var matched bool
	switch operator {
	case "gte", ">=":
		matched = value >= threshold
	case "gt", ">":
		matched = value > threshold
	case "lte", "<=":
		matched = value <= threshold
	case "lt", "<":
		matched = value < threshold
	case "eq", "==":
		matched = value == threshold
	case "neq", "!=":
		matched = value != threshold
	}

	result.Matched = matched
	result.MatchFields[field] = value
	if matched {
		result.Score = min(1.0, value/threshold)
		result.Reason = fmt.Sprintf("threshold matched: %s(%.2f) %s %.2f", field, value, operator, threshold)
	} else {
		result.Reason = fmt.Sprintf("threshold not met: %s(%.2f) %s %.2f", field, value, operator, threshold)
	}
	return result
}

// evalSignature 特征规则评估
func (a *RuleAnalyzer) evalSignature(rule *model.Rule, dp DataPoint, conditions map[string]interface{}) EvalResult {
	result := EvalResult{RuleID: rule.RuleID, RuleName: rule.Name, MatchFields: make(map[string]interface{})}

	pattern, _ := getString(conditions, "pattern")
	if pattern == "" {
		pattern, _ = getString(conditions, "signature")
	}
	if pattern == "" {
		result.Reason = "missing pattern/signature"
		return result
	}

	field, _ := getString(conditions, "field")
	if field == "" {
		field = "label" // 默认匹配标签
	}

	fieldValue, _ := getString(dp.Fields, field)
	if fieldValue == "" {
		// 尝试多字段模糊匹配
		for k, v := range dp.Fields {
			if strVal, ok := v.(string); ok && containsPattern(strVal, pattern) {
				result.Matched = true
				result.MatchFields[k] = strVal
				result.Score = 0.8
				result.Reason = fmt.Sprintf("pattern '%s' matched in field '%s'", pattern, k)
				return result
			}
		}
		result.Reason = fmt.Sprintf("pattern '%s' not found in any field", pattern)
		return result
	}

	result.Matched = containsPattern(fieldValue, pattern)
	if result.Matched {
		result.MatchFields[field] = fieldValue
		result.Score = 1.0
		result.Reason = fmt.Sprintf("signature matched: field '%s' contains '%s'", field, pattern)
	} else {
		result.Reason = fmt.Sprintf("signature not matched: field '%s'='%s'", field, fieldValue)
	}
	return result
}

// evalAnomaly 异常规则评估
func (a *RuleAnalyzer) evalAnomaly(rule *model.Rule, dp DataPoint, conditions map[string]interface{}) EvalResult {
	result := EvalResult{RuleID: rule.RuleID, RuleName: rule.Name, MatchFields: make(map[string]interface{})}

	baseline, ok := getFloat(conditions, "baseline")
	if !ok {
		result.Reason = "missing baseline for anomaly detection"
		return result
	}

	field, _ := getString(conditions, "field")
	if field == "" {
		field = "value"
	}

	deviation, _ := getFloat(conditions, "deviation")
	if deviation == 0 {
		deviation = 3.0 // 默认3σ
	}

	value, ok := getFloat(dp.Fields, field)
	if !ok {
		result.Reason = fmt.Sprintf("field '%s' not numeric", field)
		return result
	}

	// 计算偏差倍数
	ratio := value / baseline
	if baseline == 0 {
		ratio = value
	}

	matched := ratio > deviation || ratio < 1.0/deviation
	result.Matched = matched
	result.MatchFields[field] = value
	result.MatchFields["baseline"] = baseline
	result.MatchFields["deviation_ratio"] = ratio

	if matched {
		result.Score = min(1.0, ratio/deviation*0.5)
		result.Reason = fmt.Sprintf("anomaly detected: %s=%.2f deviates %.1fx from baseline %.2f (threshold %.1fσ)",
			field, value, ratio, baseline, deviation)
	} else {
		result.Reason = fmt.Sprintf("within normal range: %s=%.2f (baseline=%.2f, ratio=%.2f)",
			field, value, baseline, ratio)
	}
	return result
}

// evalCorrelation 关联规则评估
func (a *RuleAnalyzer) evalCorrelation(rule *model.Rule, dp DataPoint, conditions map[string]interface{}) EvalResult {
	result := EvalResult{RuleID: rule.RuleID, RuleName: rule.Name, MatchFields: make(map[string]interface{})}

	rules, ok := getList(conditions, "rules")
	if !ok || len(rules) == 0 {
		result.Reason = "correlation rule requires 'rules' list"
		return result
	}

	requireAll, _ := getBool(conditions, "require_all")

	var maxScore, matchedCount, totalRequired float64
	if requireAll {
		totalRequired = float64(len(rules))
	} else {
		totalRequired = maxFloat(1.0, getFloatDefault(conditions, "min_matches", 1.0))
	}

	for _, subRule := range rules {
		subCond, ok := subRule.(map[string]interface{})
		if !ok {
			continue
		}
		// 递归评估子规则
		subResult := a.evalGeneric(rule, dp, subCond)
		if subResult.Matched {
			matchedCount++
			maxScore = maxFloat(maxScore, subResult.Score)
			for k, v := range subResult.MatchFields {
				result.MatchFields[k] = v
			}
		}
	}

	result.Matched = matchedCount >= totalRequired
	result.Score = maxScore * (matchedCount / float64(len(rules)))
	if result.Matched {
		result.Reason = fmt.Sprintf("correlation matched: %.0f/%.0f sub-rules (require_all=%v)",
			matchedCount, totalRequired, requireAll)
	} else {
		result.Reason = fmt.Sprintf("correlation not met: %.0f/%.0f sub-rules matched",
			matchedCount, totalRequired)
	}
	return result
}

// evalGeneric 通用规则评估
func (a *RuleAnalyzer) evalGeneric(rule *model.Rule, dp DataPoint, conditions map[string]interface{}) EvalResult {
	result := EvalResult{RuleID: rule.RuleID, RuleName: rule.Name, MatchFields: make(map[string]interface{})}

	field, _ := getString(conditions, "field")
	op, _ := getString(conditions, "operator")
	expected := conditions["value"]

	if field == "" || expected == nil {
		result.Reason = "generic rule requires 'field' and 'value'"
		return result
	}

	actual, exists := dp.Fields[field]
	if !exists {
		result.Reason = fmt.Sprintf("field '%s' not found", field)
		return result
	}

	result.MatchFields[field] = actual
	result.Matched = matchGeneric(actual, expected, op)
	result.Score = boolToFloat(result.Matched)
	result.Reason = fmt.Sprintf("field '%s': actual=%v, expected=%v, op=%s, matched=%v",
		field, actual, expected, op, result.Matched)

	return result
}

// generateRecommendation 生成上线建议
func (a *RuleAnalyzer) generateRecommendation(report *SimulationReport, rule *model.Rule) (string, string, []string) {
	var recommendation, riskLevel string
	var warnings []string

	switch {
	case report.MatchRate == 0:
		recommendation = "规则无命中，建议检查条件或扩大数据范围后再试"
		riskLevel = "info"
		warnings = append(warnings, "zero match rate - rule may be too strict")
	case report.MatchRate > 0.5:
		recommendation = "命中率过高(>50%)，可能是规则过于宽泛，建议缩小条件范围以避免大量误报"
		riskLevel = "high"
		warnings = append(warnings, fmt.Sprintf("very high match rate: %.1f%%", report.MatchRate*100))
	case report.MatchRate > 0.1:
		recommendation = "命中率适中(10-50%)，建议确认匹配质量后灰度上线"
		riskLevel = "medium"
	case report.MatchRate > 0.01:
		recommendation = "命中率较低(<10%)，适合作为精准检测规则上线"
		riskLevel = "low"
	default:
		recommendation = "命中率极低(<1%)，适合上线作为高精准规则"
		riskLevel = "low"
	}

	if report.P99EvalTimeMs > 10 {
		warnings = append(warnings, fmt.Sprintf("P99 evaluation latency too high: %.2fms", report.P99EvalTimeMs))
		riskLevel = maxRisk(riskLevel, "medium")
	}

	if report.AvgEvalTimeMs > 5 {
		warnings = append(warnings, fmt.Sprintf("average evaluation latency >5ms: %.2fms", report.AvgEvalTimeMs))
	}

	return recommendation, riskLevel, warnings
}

// CompareRules A/B 测试：比较两条规则的性能
func (a *RuleAnalyzer) CompareRules(ctx context.Context, ruleA, ruleB *model.Rule, input *SimulationInput) (*ComparisonReport, error) {
	reportA, err := a.DryRun(ctx, ruleA, input)
	if err != nil {
		return nil, fmt.Errorf("evaluate rule A: %w", err)
	}
	reportB, err := a.DryRun(ctx, ruleB, input)
	if err != nil {
		return nil, fmt.Errorf("evaluate rule B: %w", err)
	}

	winner := "B"
	reason := ""
	if reportA.MatchRate > reportB.MatchRate && reportA.P99EvalTimeMs <= reportB.P99EvalTimeMs {
		winner = "A"
		reason = fmt.Sprintf("Rule A has higher match rate (%.2f%% vs %.2f%%) with similar or better performance",
			reportA.MatchRate*100, reportB.MatchRate*100)
	} else if reportA.P99EvalTimeMs < reportB.P99EvalTimeMs {
		winner = "A"
		reason = fmt.Sprintf("Rule A has better P99 latency (%.2fms vs %.2fms)",
			reportA.P99EvalTimeMs, reportB.P99EvalTimeMs)
	}

	return &ComparisonReport{
		RuleA: reportA, RuleB: reportB,
		Winner:    winner,
		Reason:    reason,
		MatchDiff: reportA.MatchRate - reportB.MatchRate,
		LatencyDiff: reportA.P99EvalTimeMs - reportB.P99EvalTimeMs,
	}, nil
}

// ComparisonReport 对比报告
type ComparisonReport struct {
	RuleA       *SimulationReport `json:"rule_a"`
	RuleB       *SimulationReport `json:"rule_b"`
	Winner      string            `json:"winner"`       // "A" or "B"
	Reason      string            `json:"reason"`
	MatchDiff   float64           `json:"match_diff"`
	LatencyDiff float64           `json:"latency_diff"`
}

// GetEffectiveness 获取规则有效性评分 (基于历史评估数据)
func (a *RuleAnalyzer) GetEffectiveness(ruleID string) float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	history := a.EvaluationHistory[ruleID]
	if len(history) == 0 {
		return 0
	}

	var totalScore float64
	for _, r := range history {
		totalScore += r.Score
	}
	return totalScore / float64(len(history))
}

// GetRuleComparisonMatrix 获取规则对比矩阵
func (a *RuleAnalyzer) GetRuleComparisonMatrix(ruleIDs []string) map[string]map[string]float64 {
	matrix := make(map[string]map[string]float64)
	for _, id := range ruleIDs {
		matrix[id] = make(map[string]float64)
		for _, other := range ruleIDs {
			if id != other {
				matrix[id][other] = a.compareEffectiveness(id, other)
			}
		}
	}
	return matrix
}

func (a *RuleAnalyzer) compareEffectiveness(idA, idB string) float64 {
	effA := a.GetEffectiveness(idA)
	effB := a.GetEffectiveness(idB)
	if effA+effB == 0 {
		return 0.5
	}
	return effA / (effA + effB)
}

// ---- helpers ----

func generateSimID(ruleID string, t time.Time) string {
	raw := fmt.Sprintf("%s:%d", ruleID, t.UnixNano())
	h := md5.Sum([]byte(raw))
	return fmt.Sprintf("sim-%x", h[:8])
}

func getFloat(m map[string]interface{}, key string) (float64, bool) {
	v, ok := m[key]
	if !ok { return 0, false }
	switch n := v.(type) {
	case float64: return n, true
	case float32: return float64(n), true
	case int: return float64(n), true
	case int64: return float64(n), true
	}
	return 0, false
}

func getFloatDefault(m map[string]interface{}, key string, def float64) float64 {
	if v, ok := getFloat(m, key); ok { return v }
	return def
}

func getString(m map[string]interface{}, key string) (string, bool) {
	v, ok := m[key]
	if !ok { return "", false }
	s, ok := v.(string)
	return s, ok
}

func getBool(m map[string]interface{}, key string) (bool, bool) {
	v, ok := m[key]
	if !ok { return false, false }
	b, ok := v.(bool)
	return b, ok
}

func getList(m map[string]interface{}, key string) ([]interface{}, bool) {
	v, ok := m[key]
	if !ok { return nil, false }
	l, ok := v.([]interface{})
	return l, ok
}

func containsPattern(s, pattern string) bool {
	return len(pattern) > 0 && (s == pattern ||
		(len(pattern) <= len(s) && findSubstring(s, pattern)))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub { return true }
	}
	return false
}

func matchGeneric(actual interface{}, expected interface{}, op string) bool {
	switch op {
	case "eq", "==", "":
		return fmt.Sprint(actual) == fmt.Sprint(expected)
	case "neq", "!=":
		return fmt.Sprint(actual) != fmt.Sprint(expected)
	case "contains":
		return containsPattern(fmt.Sprint(actual), fmt.Sprint(expected))
	case "prefix":
		s := fmt.Sprint(actual)
		e := fmt.Sprint(expected)
		return len(s) >= len(e) && s[:len(e)] == e
	case "suffix":
		s := fmt.Sprint(actual)
		e := fmt.Sprint(expected)
		return len(s) >= len(e) && s[len(s)-len(e):] == e
	case "regex":
		pattern := fmt.Sprint(expected)
		matched, err := regexp.MatchString(pattern, fmt.Sprint(actual))
		if err != nil {
			return false // invalid regex → no match
		}
		return matched
	}
	return false
}

func average(values []float64) float64 {
	if len(values) == 0 { return 0 }
	sum := 0.0
	for _, v := range values { sum += v }
	return sum / float64(len(values))
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 { return 0 }
	idx := int(float64(len(sorted)) * p / 100.0)
	if idx >= len(sorted) { idx = len(sorted) - 1 }
	return sorted[idx]
}

func topFields(counts map[string]int, totalMatched int, n int) []FieldMatchCount {
	type kv struct { k string; v int }
	var items []kv
	for k, v := range counts { items = append(items, kv{k, v}) }
	sort.Slice(items, func(i, j int) bool { return items[i].v > items[j].v })
	var result []FieldMatchCount
	for i, item := range items {
		if i >= n { break }
		ratio := 0.0
		if totalMatched > 0 { ratio = float64(item.v) / float64(totalMatched) }
		result = append(result, FieldMatchCount{Field: item.k, Count: item.v, Ratio: ratio})
	}
	return result
}

func boolToFloat(b bool) float64 { if b { return 1.0 }; return 0.0 }
func maxRisk(a, b string) string {
	order := map[string]int{"info":0,"low":1,"medium":2,"high":3,"critical":4}
	if order[a] > order[b] { return a }
	return b
}
func min(a, b float64) float64 { if a < b { return a }; return b }
func maxFloat(a, b float64) float64 { if a > b { return a }; return b }

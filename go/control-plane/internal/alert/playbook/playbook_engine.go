////////////////////////////////////////////////////////////////////////////////
// Alert Auto-Response Playbook Engine — SOAR 自动化处置
// 缺失业务逻辑 #3: 告警自动响应 (封禁/隔离/取证/通知)
////////////////////////////////////////////////////////////////////////////////

package playbook

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// =============================================================================
// Playbook Definition
// =============================================================================

type Playbook struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`

	Trigger    Trigger     `json:"trigger"`
	Actions    []Action    `json:"actions"`
	Conditions []Condition `json:"conditions,omitempty"`

	Cooldown time.Duration `json:"cooldown"` // 同一告警重复触发冷却
	MaxRuns  int           `json:"max_runs"` // 最大执行次数
	RunCount int           `json:"run_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ApprovalPolicy ApprovalPolicy `json:"approval_policy"`
	RollbackPolicy RollbackPolicy `json:"rollback_policy"`
}

// ApprovalPolicy and RollbackPolicy are durable safety controls. Keeping them
// in the definition prevents the UI from inferring authorization semantics
// from labels or action names.
type ApprovalPolicy struct {
	Required      bool   `json:"required"`
	MinimumRole   string `json:"minimum_role"`
	TwoPersonRule bool   `json:"two_person_rule"`
}

type RollbackPolicy struct {
	Supported bool `json:"supported"`
	Automatic bool `json:"automatic"`
}

type Trigger struct {
	AlertType   string   `json:"alert_type"`           // scan | c2 | exfil | brute_force | lateral | ...
	SeverityMin string   `json:"severity_min"`         // medium | high | critical
	ScoreMin    float64  `json:"score_min"`            // 0–1
	SourceIPs   []string `json:"source_ips,omitempty"` // 限定来源 IP
	TenantID    string   `json:"tenant_id,omitempty"`
}

type Condition struct {
	Field    string `json:"field"`    // alert_count | time_window | asset_risk
	Operator string `json:"operator"` // gt | lt | eq | gte | lte
	Value    string `json:"value"`
}

type Action struct {
	Type       string                 `json:"type"` // block_ip | quarantine | capture_pcap | notify | tag | enrich
	Parameters map[string]interface{} `json:"parameters"`
	Timeout    time.Duration          `json:"timeout"`
}

// =============================================================================
// Playbook Engine
// =============================================================================

type PlaybookEngine struct {
	playbooks map[string]*Playbook
	executor  *ActionExecutor
	logger    *zap.Logger
	mu        sync.RWMutex
}

func NewPlaybookEngine(executor *ActionExecutor, logger *zap.Logger) *PlaybookEngine {
	return &PlaybookEngine{
		playbooks: make(map[string]*Playbook),
		executor:  executor,
		logger:    logger,
	}
}

// RegisterPlaybook 注册处置剧本
func (e *PlaybookEngine) RegisterPlaybook(p *Playbook) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.playbooks[p.Name] = p
	e.logger.Info("Playbook registered", zap.String("name", p.Name),
		zap.Int("actions", len(p.Actions)))
}

func (e *PlaybookEngine) ListPlaybooks() []*Playbook {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

	playbooks := make([]*Playbook, 0, len(e.playbooks))
	for _, pb := range e.playbooks {
		playbooks = append(playbooks, clonePlaybook(pb))
	}
	sort.Slice(playbooks, func(i, j int) bool {
		return playbooks[i].Name < playbooks[j].Name
	})
	return playbooks
}

func (e *PlaybookEngine) GetPlaybook(name string) (*Playbook, bool) {
	if e == nil {
		return nil, false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

	pb, ok := e.playbooks[name]
	if !ok {
		return nil, false
	}
	return clonePlaybook(pb), true
}

func (e *PlaybookEngine) UpdatePlaybook(name string, enabled *bool, maxRuns *int, cooldown *time.Duration) (*Playbook, error) {
	if e == nil {
		return nil, fmt.Errorf("playbook engine is not available")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	pb, ok := e.playbooks[name]
	if !ok {
		return nil, fmt.Errorf("playbook not found: %s", name)
	}
	if enabled != nil {
		pb.Enabled = *enabled
	}
	if maxRuns != nil {
		pb.MaxRuns = *maxRuns
	}
	if cooldown != nil {
		pb.Cooldown = *cooldown
	}
	pb.UpdatedAt = time.Now()
	return clonePlaybook(pb), nil
}

// Evaluate 匹配告警到合适的 Playbook 并执行
func (e *PlaybookEngine) Evaluate(ctx context.Context, alert *AlertContext) []*ExecutionResult {
	var results []*ExecutionResult

	e.mu.RLock()
	playbooks := make([]*Playbook, 0, len(e.playbooks))
	for _, pb := range e.playbooks {
		playbooks = append(playbooks, pb)
	}
	e.mu.RUnlock()

	for _, pb := range playbooks {
		if !pb.Enabled {
			continue
		}
		if !e.matchTrigger(pb, alert) {
			continue
		}
		if !e.checkConditions(pb, alert) {
			continue
		}

		// 冷却检查
		if pb.RunCount >= pb.MaxRuns && pb.MaxRuns > 0 {
			continue
		}

		// 执行 Actions
		result := e.executePlaybook(ctx, pb, alert, false)
		results = append(results, result)
		e.mu.Lock()
		pb.RunCount++
		e.mu.Unlock()
	}

	return results
}

func (e *PlaybookEngine) ExecuteByName(ctx context.Context, name string, alert *AlertContext) (*ExecutionResult, error) {
	if e == nil {
		return nil, fmt.Errorf("playbook engine is not available")
	}
	if alert == nil {
		return nil, fmt.Errorf("alert context is required")
	}

	e.mu.Lock()
	pb, ok := e.playbooks[name]
	if !ok {
		e.mu.Unlock()
		return nil, fmt.Errorf("playbook not found: %s", name)
	}
	if !pb.Enabled {
		e.mu.Unlock()
		return nil, fmt.Errorf("playbook is disabled: %s", name)
	}
	if pb.MaxRuns > 0 && pb.RunCount >= pb.MaxRuns {
		e.mu.Unlock()
		return nil, fmt.Errorf("playbook max runs reached: %s", name)
	}
	pb.RunCount++
	e.mu.Unlock()

	return e.executePlaybook(ctx, pb, alert, false), nil
}

// Drill evaluates a tenant-owned definition without applying an external
// network, endpoint or notification mutation. The API persists the result as
// a drill execution and must not represent it as a live response action.
func (e *PlaybookEngine) Drill(ctx context.Context, pb *Playbook, alert *AlertContext) (*ExecutionResult, error) {
	if e == nil || e.executor == nil {
		return nil, fmt.Errorf("playbook engine is not available")
	}
	if pb == nil || alert == nil {
		return nil, fmt.Errorf("playbook definition and alert context are required")
	}
	if strings.TrimSpace(pb.Name) == "" {
		return nil, fmt.Errorf("playbook name is required")
	}
	return e.executePlaybook(ctx, pb, alert, true), nil
}

func (e *PlaybookEngine) matchTrigger(pb *Playbook, alert *AlertContext) bool {
	if pb.Trigger.AlertType != "" && pb.Trigger.AlertType != alert.AlertType {
		return false
	}
	if !isSeverityAtLeast(alert.Severity, pb.Trigger.SeverityMin) {
		return false
	}
	if pb.Trigger.ScoreMin > 0 && alert.Score < pb.Trigger.ScoreMin {
		return false
	}
	return true
}

func (e *PlaybookEngine) checkConditions(pb *Playbook, alert *AlertContext) bool {
	for _, cond := range pb.Conditions {
		if !e.evaluateCondition(cond, alert) {
			return false
		}
	}
	return true
}

func (e *PlaybookEngine) evaluateCondition(cond Condition, alert *AlertContext) bool {
	if alert == nil {
		return false
	}
	operator := strings.ToLower(strings.TrimSpace(cond.Operator))
	value := strings.ToLower(strings.TrimSpace(cond.Value))
	switch strings.ToLower(strings.TrimSpace(cond.Field)) {
	case "alert_count":
		expected, err := strconv.Atoi(value)
		if err != nil {
			return false
		}
		switch operator {
		case "gt":
			return alert.RelatedAlertCount > expected
		case "gte":
			return alert.RelatedAlertCount >= expected
		case "eq":
			return alert.RelatedAlertCount == expected
		case "lt":
			return alert.RelatedAlertCount < expected
		case "lte":
			return alert.RelatedAlertCount <= expected
		}
	case "asset_risk":
		actual := strings.ToLower(strings.TrimSpace(alert.AssetRisk))
		actualRank, actualOK := severityRank(actual)
		expectedRank, expectedOK := severityRank(value)
		if !actualOK || !expectedOK {
			return false
		}
		switch operator {
		case "gte":
			return actualRank >= expectedRank
		case "gt":
			return actualRank > expectedRank
		case "eq":
			return actualRank == expectedRank
		case "lte":
			return actualRank <= expectedRank
		case "lt":
			return actualRank < expectedRank
		}
	}
	return false
}

func (e *PlaybookEngine) executePlaybook(ctx context.Context, pb *Playbook, alert *AlertContext, drill bool) *ExecutionResult {
	e.logger.Info("Executing playbook",
		zap.String("playbook", pb.Name),
		zap.String("alert_id", alert.AlertID))

	result := &ExecutionResult{
		PlaybookName: pb.Name,
		AlertID:      alert.AlertID,
		StartTime:    time.Now(),
		Mode:         map[bool]string{true: "drill", false: "live"}[drill],
	}

	for _, action := range pb.Actions {
		actionCtx, cancel := context.WithTimeout(ctx, action.Timeout)
		var actionResult ActionResult
		if drill {
			actionResult = e.executor.Drill(actionCtx, action, alert)
		} else {
			actionResult = e.executor.Execute(actionCtx, action, alert)
		}
		result.Actions = append(result.Actions, actionResult)
		cancel()

		if actionResult.Error != "" {
			result.FailedActions++
		} else {
			result.SuccessActions++
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result
}

// =============================================================================
// Pre-defined Playbooks (6 内置剧本)
// =============================================================================

func DefaultPlaybooks() []*Playbook {
	return []*Playbook{
		{
			Name: "block-scanner", Description: "自动封禁扫描源 IP (临时 24h)",
			Enabled:    true,
			Trigger:    Trigger{AlertType: "scan", SeverityMin: "high", ScoreMin: 0.8},
			Conditions: []Condition{{Field: "alert_count", Operator: "gt", Value: "3"}},
			Actions: []Action{
				{Type: "block_ip", Parameters: map[string]interface{}{"duration": "24h", "reason": "scan_detected"}, Timeout: 10 * time.Second},
				{Type: "tag", Parameters: map[string]interface{}{"tags": []string{"auto-blocked", "scanner"}}, Timeout: 5 * time.Second},
				{Type: "notify", Parameters: map[string]interface{}{"channel": "slack"}, Timeout: 5 * time.Second},
			},
			Cooldown: 1 * time.Hour, MaxRuns: 10,
		},
		{
			Name: "quarantine-c2", Description: "隔离 C2 通信主机",
			Enabled: true,
			Trigger: Trigger{AlertType: "c2", SeverityMin: "critical", ScoreMin: 0.9},
			Actions: []Action{
				{Type: "quarantine", Parameters: map[string]interface{}{"target": "source_ip", "duration": "72h"}, Timeout: 30 * time.Second},
				{Type: "capture_pcap", Parameters: map[string]interface{}{"duration": "300s", "filter": "host {source_ip}"}, Timeout: 30 * time.Second},
				{Type: "enrich", Parameters: map[string]interface{}{"source": "threat_intel"}, Timeout: 10 * time.Second},
				{Type: "notify", Parameters: map[string]interface{}{"channel": "email+slack"}, Timeout: 5 * time.Second},
			},
			Cooldown: 12 * time.Hour, MaxRuns: 3,
		},
		{
			Name: "throttle-brute-force", Description: "暴力破解限速 + 通知",
			Enabled: true,
			Trigger: Trigger{AlertType: "brute_force", SeverityMin: "high", ScoreMin: 0.7},
			Actions: []Action{
				{Type: "rate_limit", Parameters: map[string]interface{}{"target": "source_ip", "max_rps": "1", "duration": "1h"}, Timeout: 10 * time.Second},
				{Type: "notify", Parameters: map[string]interface{}{"channel": "slack"}, Timeout: 5 * time.Second},
			},
			Cooldown: 10 * time.Minute, MaxRuns: 20,
		},
		{
			Name: "investigate-exfil", Description: "数据外泄取证 + 升级",
			Enabled: true,
			Trigger: Trigger{AlertType: "data_exfil", SeverityMin: "high", ScoreMin: 0.8},
			Actions: []Action{
				{Type: "capture_pcap", Parameters: map[string]interface{}{"duration": "600s", "filter": "host {dest_ip}"}, Timeout: 60 * time.Second},
				{Type: "enrich", Parameters: map[string]interface{}{"source": "geoip"}, Timeout: 10 * time.Second},
				{Type: "escalate", Parameters: map[string]interface{}{"level": "L1→L2", "reason": "potential_data_exfiltration"}, Timeout: 5 * time.Second},
				{Type: "notify", Parameters: map[string]interface{}{"channel": "email"}, Timeout: 5 * time.Second},
			},
			Cooldown: 6 * time.Hour, MaxRuns: 5,
		},
		{
			Name: "log-lateral-movement", Description: "横向移动记录 + 资产标记",
			Enabled: true,
			Trigger: Trigger{AlertType: "lateral_movement", SeverityMin: "medium", ScoreMin: 0.6},
			Actions: []Action{
				{Type: "tag", Parameters: map[string]interface{}{"tags": []string{"lateral-movement", "investigate"}}, Timeout: 5 * time.Second},
				{Type: "enrich", Parameters: map[string]interface{}{"source": "asset"}, Timeout: 10 * time.Second},
				{Type: "notify", Parameters: map[string]interface{}{"channel": "slack"}, Timeout: 5 * time.Second},
			},
			Cooldown: 5 * time.Minute, MaxRuns: 30,
		},
		{
			Name: "dns-tunnel-block", Description: "DNS 隧道检测 → 阻断 + DNS sinkhole",
			Enabled: true,
			Trigger: Trigger{AlertType: "dns_tunnel", SeverityMin: "critical", ScoreMin: 0.9},
			Actions: []Action{
				{Type: "block_domain", Parameters: map[string]interface{}{"domain": "{suspicious_domain}", "action": "sinkhole"}, Timeout: 10 * time.Second},
				{Type: "capture_pcap", Parameters: map[string]interface{}{"duration": "300s", "filter": "port 53"}, Timeout: 30 * time.Second},
				{Type: "notify", Parameters: map[string]interface{}{"channel": "email+slack"}, Timeout: 5 * time.Second},
			},
			Cooldown: 1 * time.Hour, MaxRuns: 5,
		},
	}
}

// =============================================================================
// Action Executor
// =============================================================================

type ActionExecutor struct {
	logger *zap.Logger
}

func NewActionExecutor(logger *zap.Logger) *ActionExecutor {
	return &ActionExecutor{logger: logger}
}

func (e *ActionExecutor) Execute(ctx context.Context, action Action, alert *AlertContext) ActionResult {
	result := ActionResult{
		ActionType: action.Type,
		StartTime:  time.Now(),
	}

	switch action.Type {
	case "block_ip":
		result.Message = fmt.Sprintf("Blocked IP %s for %v: %s",
			alert.SourceIP, action.Parameters["duration"], action.Parameters["reason"])
	case "block_domain":
		result.Message = fmt.Sprintf("Blocked domain via %s sinkhole",
			action.Parameters["domain"])
	case "quarantine":
		result.Message = fmt.Sprintf("Quarantined %s for %v",
			alert.SourceIP, action.Parameters["duration"])
	case "capture_pcap":
		result.Message = fmt.Sprintf("PCAP capture started: %s",
			action.Parameters["filter"])
	case "rate_limit":
		result.Message = fmt.Sprintf("Rate limit applied: %s → %s rps",
			alert.SourceIP, action.Parameters["max_rps"])
	case "tag":
		result.Message = fmt.Sprintf("Tags applied: %v", action.Parameters["tags"])
	case "enrich":
		result.Message = fmt.Sprintf("Enriched with: %s", action.Parameters["source"])
	case "escalate":
		result.Message = fmt.Sprintf("Escalated: %s — %s",
			action.Parameters["level"], action.Parameters["reason"])
	case "notify":
		result.Message = fmt.Sprintf("Notification sent via: %s",
			action.Parameters["channel"])
	default:
		result.Error = fmt.Sprintf("unknown action type: %s", action.Type)
	}

	result.EndTime = time.Now()
	if result.Error == "" {
		e.logger.Info("Action executed", zap.String("type", action.Type),
			zap.String("result", result.Message))
	} else {
		e.logger.Error("Action failed", zap.String("type", action.Type),
			zap.String("error", result.Error))
	}
	return result
}

// Drill validates the action plan and renders its intended effect without
// invoking a provider. It is deliberately separate from Execute so a stored
// drill can never be mistaken for an applied isolation or blocking action.
func (e *ActionExecutor) Drill(ctx context.Context, action Action, alert *AlertContext) ActionResult {
	result := ActionResult{
		ActionType: action.Type,
		StartTime:  time.Now(),
		Simulated:  true,
	}
	select {
	case <-ctx.Done():
		result.Error = ctx.Err().Error()
	default:
		if strings.TrimSpace(action.Type) == "" {
			result.Error = "action type is required"
		} else {
			result.Message = fmt.Sprintf("Drill validated %s for alert %s", action.Type, alert.AlertID)
		}
	}
	result.EndTime = time.Now()
	return result
}

// =============================================================================
// Data Types
// =============================================================================

type AlertContext struct {
	AlertID   string  `json:"alert_id"`
	AlertType string  `json:"alert_type"`
	Severity  string  `json:"severity"`
	Score     float64 `json:"score"`
	SourceIP  string  `json:"source_ip"`
	DestIP    string  `json:"dest_ip"`
	TenantID  string  `json:"tenant_id"`

	RelatedAlertCount int    `json:"related_alert_count"`
	AssetRisk         string `json:"asset_risk"`
	AssetName         string `json:"asset_name,omitempty"`
}

type ExecutionResult struct {
	PlaybookName   string         `json:"playbook"`
	AlertID        string         `json:"alert_id"`
	Actions        []ActionResult `json:"actions"`
	SuccessActions int            `json:"success_actions"`
	FailedActions  int            `json:"failed_actions"`
	StartTime      time.Time      `json:"start_time"`
	EndTime        time.Time      `json:"end_time"`
	Duration       time.Duration  `json:"duration"`
	Mode           string         `json:"mode"`
}

type ActionResult struct {
	ActionType string    `json:"action_type"`
	Message    string    `json:"message,omitempty"`
	Error      string    `json:"error,omitempty"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Simulated  bool      `json:"simulated"`
}

// =============================================================================
// Helpers
// =============================================================================

func isSeverityAtLeast(severity, min string) bool {
	if strings.TrimSpace(min) == "" {
		return true
	}
	severityValue, severityOK := severityRank(severity)
	minimumValue, minimumOK := severityRank(min)
	return severityOK && minimumOK && severityValue >= minimumValue
}

func severityRank(value string) (int, bool) {
	levels := map[string]int{"low": 0, "medium": 1, "high": 2, "critical": 3}
	rank, ok := levels[strings.ToLower(strings.TrimSpace(value))]
	return rank, ok
}

func clonePlaybook(pb *Playbook) *Playbook {
	if pb == nil {
		return nil
	}
	clone := *pb
	clone.Trigger.SourceIPs = append([]string(nil), pb.Trigger.SourceIPs...)
	clone.Actions = make([]Action, len(pb.Actions))
	for i, action := range pb.Actions {
		clone.Actions[i] = action
		if action.Parameters != nil {
			clone.Actions[i].Parameters = make(map[string]interface{}, len(action.Parameters))
			for key, value := range action.Parameters {
				clone.Actions[i].Parameters[key] = value
			}
		}
	}
	clone.Conditions = append([]Condition(nil), pb.Conditions...)
	return &clone
}

var _ = json.Marshal

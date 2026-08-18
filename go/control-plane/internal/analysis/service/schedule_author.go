// Package service 调度修订权威服务(SaveScheduleRevisionAtomic + 激活头 CAS):
// 保存=精确绑定已批准 plan revision/hash 的不可变修订(激活前不触发);
// 激活/暂停经 ScheduleActivationHead CAS(expected authority revision)+ 审计;
// 暂停只影响未来触发,不取消当前 Run(§8/§76.45.1)。
package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// TriggerKinds 合法触发方式(§5.1;EVENT_DRIVEN 的事件桥未发布,Tick 只跳过不物化)。
var TriggerKinds = map[string]bool{
	"CRON_WINDOW":      true,
	"CONTINUOUS_WINDOW": true,
	"EVENT_DRIVEN":     true,
}

// MisfirePolicies 合法错过策略(§76.45.4)。
var MisfirePolicies = map[string]bool{
	"MISFIRE_FAIL":           true,
	"MISFIRE_DELAY":          true,
	"MISFIRE_BOUNDED_REPLAY": true,
}

// ConcurrencyPolicies 合法并发策略(§76.45.4;FORBID_OVERLAP 已实现,CANCEL_PREVIOUS 待取消 drain 链)。
var ConcurrencyPolicies = map[string]bool{
	"FORBID_OVERLAP":    true,
	"CANCEL_PREVIOUS":   true,
	"ALLOW_CONCURRENT":  true,
}

// ScheduleService 调度修订权威(命令式 API 的服务层)。
type ScheduleService struct {
	repo *repository.Repo
}

func NewScheduleService(repo *repository.Repo) *ScheduleService {
	return &ScheduleService{repo: repo}
}

// SaveScheduleRequest 保存请求(领域词:task definition + approved plan exact binding)。
type SaveScheduleRequest struct {
	TenantID             string
	TaskDefinitionID     string
	ApprovedPlanRevision int64
	ExecutionSpecSHA256  string
	TriggerKind          string
	Timezone             string
	WindowOrCron         json.RawMessage
	PrepareLeadTimeMs    int64
	MisfirePolicy        string
	ConcurrencyPolicy    string
	SchedulingClass      string
	ResourceRestrictions json.RawMessage
	ClientIdempotencyKey string
}

// SaveScheduleResult 保存结果。
type SaveScheduleResult struct {
	ScheduleID     string
	Revision       int64
	ScheduleSHA256 string
}

// Save 保存不可变调度修订(修订号自动分配;schedule_sha256 规范冻结;幂等经台账)。
func (s *ScheduleService) Save(ctx context.Context, req SaveScheduleRequest) (*SaveScheduleResult, bool, error) {
	if err := validateScheduleRequest(req); err != nil {
		return nil, false, err
	}
	revision, err := s.repo.NextScheduleRevision(ctx, req.TenantID, req.TaskDefinitionID)
	if err != nil {
		return nil, false, fmt.Errorf("next schedule revision: %w", err)
	}
	windowJSON := req.WindowOrCron
	if len(windowJSON) == 0 || string(windowJSON) == "null" {
		windowJSON = json.RawMessage(`{}`)
	}
	restrictions := req.ResourceRestrictions
	if len(restrictions) == 0 || string(restrictions) == "null" {
		restrictions = json.RawMessage(`{}`)
	}
	// schedule_sha256 覆盖冻结 spec 字段(不含 revision:revision 是行身份,
	// 自动分配不能破坏同 payload 幂等回放)。
	scheduleSHA := scheduleCanonicalSHA(req)
	requestSHA := identityHash("schedule-save", req.ClientIdempotencyKey, scheduleSHA)
	idempotencyKey := req.ClientIdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = identityHash("schedule-save", req.TenantID, req.TaskDefinitionID)
	}
	scheduleID, replayed, err := s.repo.SaveScheduleRevisionAtomic(ctx, repository.SaveScheduleCommand{
		TenantID:             req.TenantID,
		TaskDefinitionID:     req.TaskDefinitionID,
		Revision:             revision,
		ApprovedPlanRevision: req.ApprovedPlanRevision,
		ExecutionSpecSHA256:  req.ExecutionSpecSHA256,
		TriggerKind:          req.TriggerKind,
		Timezone:             req.Timezone,
		WindowOrCron:         windowJSON,
		PrepareLeadTimeMs:    req.PrepareLeadTimeMs,
		MisfirePolicy:        req.MisfirePolicy,
		ConcurrencyPolicy:    req.ConcurrencyPolicy,
		SchedulingClass:      req.SchedulingClass,
		ResourceRestrictions: restrictions,
		ScheduleSHA256:       scheduleSHA,
		RequestSHA256:        requestSHA,
		IdempotencyKey:       idempotencyKey,
	})
	if err != nil {
		return nil, false, err
	}
	if replayed {
		// 回放:回源已冻结修订(spec 哈希精确匹配),返回既有句柄。
		existingID, existingRev, err := s.repo.FindScheduleBySHA(ctx, req.TenantID, req.TaskDefinitionID, scheduleSHA)
		if err != nil {
			return nil, false, fmt.Errorf("schedule replay resolution: %w", err)
		}
		return &SaveScheduleResult{ScheduleID: existingID, Revision: existingRev, ScheduleSHA256: scheduleSHA}, true, nil
	}
	return &SaveScheduleResult{ScheduleID: scheduleID, Revision: revision, ScheduleSHA256: scheduleSHA}, false, nil
}

// Activate 激活(DRAFT→ACTIVE;expected authority revision 防并发覆盖)。
func (s *ScheduleService) Activate(ctx context.Context, tenantID, scheduleID string, expectedRevision int64, actor string) (int64, error) {
	if expectedRevision < 0 {
		return 0, fmt.Errorf("%s: expected authority revision is required", contract.ErrCodeInvalidTransition)
	}
	newRevision, err := s.repo.ActivateScheduleAtomic(ctx, tenantID, scheduleID, expectedRevision, actor)
	if err != nil {
		return 0, err
	}
	return newRevision, nil
}

// Pause 暂停(ACTIVE→PAUSED;只影响未来触发)。
func (s *ScheduleService) Pause(ctx context.Context, tenantID, scheduleID string, expectedRevision int64, actor string) (int64, error) {
	if expectedRevision < 0 {
		return 0, fmt.Errorf("%s: expected authority revision is required", contract.ErrCodeInvalidTransition)
	}
	newRevision, err := s.repo.PauseScheduleAtomic(ctx, tenantID, scheduleID, expectedRevision, actor)
	if err != nil {
		return 0, err
	}
	return newRevision, nil
}

// List 调度修订列表(含激活头状态)。
func (s *ScheduleService) List(ctx context.Context, tenantID string) ([]repository.ScheduleView, error) {
	return s.repo.ListSchedules(ctx, tenantID)
}

// Head 读取激活头(API 返回 expected revision 供激活/暂停 If-Match)。
func (s *ScheduleService) Head(ctx context.Context, tenantID, scheduleID string) (*repository.ScheduleHeadRow, error) {
	return s.repo.GetScheduleHead(ctx, tenantID, scheduleID)
}

func validateScheduleRequest(req SaveScheduleRequest) error {
	if req.TenantID == "" || req.TaskDefinitionID == "" {
		return fmt.Errorf("%s: tenant_id and task_definition_id are required", contract.ErrCodeInvalidTransition)
	}
	if req.ApprovedPlanRevision <= 0 || req.ExecutionSpecSHA256 == "" {
		return fmt.Errorf("%s: approved_plan_revision and execution_spec_sha256 are required", contract.ErrCodeInvalidTransition)
	}
	if !TriggerKinds[req.TriggerKind] {
		return fmt.Errorf("%s: unknown trigger_kind %q", contract.ErrCodeInvalidTransition, req.TriggerKind)
	}
	if !MisfirePolicies[req.MisfirePolicy] {
		return fmt.Errorf("%s: unknown misfire_policy %q", contract.ErrCodeInvalidTransition, req.MisfirePolicy)
	}
	if !ConcurrencyPolicies[req.ConcurrencyPolicy] {
		return fmt.Errorf("%s: unknown concurrency_policy %q", contract.ErrCodeInvalidTransition, req.ConcurrencyPolicy)
	}
	if req.SchedulingClass != "" && !SchedulingClasses[req.SchedulingClass] {
		return fmt.Errorf("%s: unknown scheduling_class %q", contract.ErrCodeInvalidTransition, req.SchedulingClass)
	}
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	return nil
}

// scheduleCanonicalSHA 调度修订规范哈希(冻结字段;与保存事务同值)。
// scheduleCanonicalSHA 调度修订规范哈希(冻结 spec 字段;不含 revision,保证
// 同 payload 幂等回放哈希一致)。与保存事务写入值一致。
func scheduleCanonicalSHA(req SaveScheduleRequest) string {
	m := map[string]interface{}{
		"task_definition_id":      req.TaskDefinitionID,
		"approved_plan_revision":  req.ApprovedPlanRevision,
		"execution_spec_sha256":   req.ExecutionSpecSHA256,
		"trigger_kind":            req.TriggerKind,
		"timezone":                req.Timezone,
		"window_or_cron":          rawJSON(req.WindowOrCron),
		"prepare_lead_time_ms":    req.PrepareLeadTimeMs,
		"misfire_policy":          req.MisfirePolicy,
		"concurrency_policy":      req.ConcurrencyPolicy,
		"scheduling_class":        req.SchedulingClass,
		"resource_restrictions":   rawJSON(req.ResourceRestrictions),
	}
	return sha256Hex(mustJSON(m))
}

func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		// canonical 字段均为可序列化值;出错为编程错误,显式暴露
		panic(fmt.Sprintf("schedule canonical marshal: %v", err))
	}
	return b
}

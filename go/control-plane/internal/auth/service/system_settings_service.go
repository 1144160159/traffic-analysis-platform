package service

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

type SystemSettingsService struct {
	store   repository.SystemSettingsStore
	logger  *zap.Logger
	clock   func() time.Time
	checker IntegrationChecker
}

// IntegrationChecker performs the real reachability probe used by settings actions.
type IntegrationChecker interface {
	Check(context.Context, model.IntegrationSetting) error
}

type networkIntegrationChecker struct{}

var defaultIntegrationTargets = map[string]string{
	"keycloak":   "keycloak.middleware.svc.cluster.local:8080",
	"apisix":     "apisix.gateway.svc.cluster.local:9080",
	"kafka":      "kafka-bootstrap.middleware.svc.cluster.local:9092",
	"minio":      "minio.minio.svc.cluster.local:9000",
	"opensearch": "opensearch.middleware.svc.cluster.local:9200",
	"nebula":     "nebula-graph.middleware.svc.cluster.local:9669",
}

func (networkIntegrationChecker) Check(ctx context.Context, integration model.IntegrationSetting) error {
	target := strings.TrimSpace(integration.EndpointHint)
	envKey := "SYSTEM_SETTINGS_INTEGRATION_" + strings.ToUpper(strings.ReplaceAll(integration.ID, "-", "_")) + "_ENDPOINT"
	if override := strings.TrimSpace(os.Getenv(envKey)); override != "" {
		target = override
	}
	if target == "" {
		target = defaultIntegrationTargets[integration.ID]
	}
	if target == "" {
		return fmt.Errorf("未配置可探测 endpoint")
	}
	if parsed, err := url.Parse(target); err == nil && parsed.Host != "" {
		target = parsed.Host
	}
	if !strings.Contains(target, ":") {
		return fmt.Errorf("endpoint 缺少端口")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(probeCtx, "tcp", target)
	if err != nil {
		return fmt.Errorf("%s 不可达: %w", target, err)
	}
	_ = conn.Close()
	return nil
}

type UpdateSystemSettingsRequest struct {
	ExpectedRevision int64                `json:"expected_revision"`
	Settings         model.SystemSettings `json:"settings"`
}

type SystemSettingsActionRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	TargetID         string `json:"target_id,omitempty"`
}

type SystemSettingsActionResult struct {
	Action       string                     `json:"action"`
	Status       string                     `json:"status"`
	Message      string                     `json:"message"`
	Revision     int64                      `json:"revision"`
	UpdatedAt    time.Time                  `json:"updated_at"`
	Findings     []string                   `json:"findings,omitempty"`
	Integrations []model.IntegrationSetting `json:"integrations,omitempty"`
	Tokens       *model.SystemTokenSummary  `json:"tokens,omitempty"`
	Roles        []model.SystemRole         `json:"roles,omitempty"`
}

type SystemSettingsImpact struct {
	TenantID       string   `json:"tenant_id"`
	Revision       int64    `json:"revision"`
	AffectedScopes []string `json:"affected_scopes"`
	Risk           string   `json:"risk"`
	Approval       string   `json:"approval"`
	AuditAction    string   `json:"audit_action"`
	Summary        string   `json:"summary"`
}

type NavigationMissRequest struct {
	EventID string `json:"event_id"`
	Source  string `json:"source"`
}

type NavigationMissStatus struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	State string `json:"state"`
	Value string `json:"value"`
}

type NavigationMissContext struct {
	EventID      string                 `json:"event_id"`
	TraceID      string                 `json:"trace_id"`
	OccurredAt   time.Time              `json:"occurred_at"`
	TenantID     string                 `json:"tenant_id"`
	TenantName   string                 `json:"tenant_name"`
	SiteName     string                 `json:"site_name"`
	AccessSource string                 `json:"access_source"`
	AuditAction  string                 `json:"audit_action"`
	Persisted    bool                   `json:"persisted"`
	Statuses     []NavigationMissStatus `json:"statuses"`
}

type NavigationSupportRequest struct {
	EventID string `json:"event_id"`
}

type NavigationSupportContext struct {
	SupportRequestID  string    `json:"support_request_id"`
	NavigationEventID string    `json:"navigation_event_id"`
	TraceID           string    `json:"trace_id"`
	OccurredAt        time.Time `json:"occurred_at"`
	Queue             string    `json:"queue"`
	Status            string    `json:"status"`
	AuditAction       string    `json:"audit_action"`
	Persisted         bool      `json:"persisted"`
}

func NewSystemSettingsService(store repository.SystemSettingsStore, logger *zap.Logger) *SystemSettingsService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SystemSettingsService{store: store, logger: logger, clock: time.Now, checker: networkIntegrationChecker{}}
}

func (s *SystemSettingsService) WithIntegrationChecker(checker IntegrationChecker) *SystemSettingsService {
	if checker != nil {
		s.checker = checker
	}
	return s
}

func (s *SystemSettingsService) GetWorkbench(ctx context.Context, tenantID string) (*model.SystemSettingsWorkbench, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New(errors.ErrCodeMissingParameter, "tenant_id is required")
	}
	name, status, err := s.store.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	settings, revision, updatedAt, err := s.store.GetSettings(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		defaults := model.DefaultSystemSettings()
		settings = &defaults
		updatedAt = s.clock().UTC()
	}
	roles, err := s.store.ListRoles(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	tokens, err := s.store.GetTokenSummary(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &model.SystemSettingsWorkbench{
		TenantID: tenantID, TenantName: name, TenantStatus: status, Revision: revision,
		Settings: *settings, Roles: roles, Tokens: tokens, UpdatedAt: updatedAt,
	}, nil
}

func (s *SystemSettingsService) UpdateSettings(
	ctx context.Context,
	tenantID string,
	userID uuid.UUID,
	req UpdateSystemSettingsRequest,
) (*model.SystemSettingsWorkbench, error) {
	if err := validateSystemSettings(req.Settings); err != nil {
		return nil, err
	}
	revision, updatedAt, err := s.store.SaveSettingsWithAudit(ctx, tenantID, userID, req.ExpectedRevision, req.Settings, "system_settings_update", map[string]interface{}{
		"previous_revision": req.ExpectedRevision,
		"site_count":        len(req.Settings.Sites),
		"retention_count":   len(req.Settings.RetentionPolicies),
		"integration_count": len(req.Settings.Integrations),
		"feature_count":     len(req.Settings.Security.FeatureFlags),
	})
	if err != nil {
		return nil, err
	}
	workbench, err := s.GetWorkbench(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	workbench.Revision = revision
	workbench.UpdatedAt = updatedAt
	return workbench, nil
}

func (s *SystemSettingsService) RunAction(
	ctx context.Context,
	tenantID string,
	userID uuid.UUID,
	action string,
	req SystemSettingsActionRequest,
) (*SystemSettingsActionResult, error) {
	workbench, err := s.GetWorkbench(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if req.ExpectedRevision != workbench.Revision {
		return nil, errors.New(errors.ErrCodeVersionConflict, "system settings revision conflict")
	}
	now := s.clock().UTC()
	result := &SystemSettingsActionResult{Action: action, Status: "success", Revision: workbench.Revision, UpdatedAt: now}
	auditedWithSave := false

	switch action {
	case "scope-review":
		for _, role := range workbench.Roles {
			for _, permission := range role.Permissions {
				if !model.IsValidScope(permission) {
					result.Findings = append(result.Findings, fmt.Sprintf("角色 %s 包含未知权限 %s", role.Name, permission))
				}
			}
		}
		if len(result.Findings) > 0 {
			result.Status = "warning"
		}
		result.Message = fmt.Sprintf("权限范围复核完成：检查 %d 个角色；当前有效令牌 %d 个；未执行权限变更", len(workbench.Roles), workbench.Tokens.Active)
		result.Roles = workbench.Roles
		result.Tokens = &workbench.Tokens
	case "connection-test", "test-integration":
		matched := false
		failures := 0
		testedCount := 0
		for index := range workbench.Settings.Integrations {
			integration := &workbench.Settings.Integrations[index]
			if action == "test-integration" && req.TargetID != "" && integration.ID != req.TargetID {
				continue
			}
			matched = true
			testedCount++
			integration.LastTestedAt = &now
			if !integration.Enabled {
				integration.Status = "disabled"
				continue
			}
			if checkErr := s.checker.Check(ctx, *integration); checkErr != nil {
				integration.Status = "degraded"
				failures++
				result.Findings = append(result.Findings, integration.Name+": "+checkErr.Error())
			} else {
				integration.Status = "healthy"
			}
		}
		if !matched {
			return nil, errors.New(errors.ErrCodeEntityNotFound, "integration not found")
		}
		if failures > 0 {
			result.Status = "warning"
			result.Message = fmt.Sprintf("真实连接测试已完成：%d 个端点不可达，结果已写入租户配置", failures)
		} else {
			result.Message = "真实连接测试已完成，所有已启用端点可达，结果已写入租户配置"
		}
		auditAction := "system_settings_" + strings.ReplaceAll(action, "-", "_")
		revision, updatedAt, saveErr := s.store.SaveSettingsWithAudit(ctx, tenantID, userID, workbench.Revision, workbench.Settings, auditAction, map[string]interface{}{
			"target_id":    req.TargetID,
			"status":       result.Status,
			"findings":     result.Findings,
			"tested_count": testedCount,
		})
		if saveErr != nil {
			return nil, saveErr
		}
		auditedWithSave = true
		result.Revision = revision
		result.UpdatedAt = updatedAt
		result.Integrations = workbench.Settings.Integrations
	case "security-audit":
		result.Findings = evaluateSecurity(workbench.Settings)
		if len(result.Findings) > 0 {
			result.Status = "warning"
			result.Message = fmt.Sprintf("安全审计完成，发现 %d 项需要关注", len(result.Findings))
		} else {
			result.Message = "安全审计完成，未发现阻断项"
		}
	case "lifecycle-review":
		for _, policy := range workbench.Settings.RetentionPolicies {
			if policy.Status == "expiring" {
				result.Findings = append(result.Findings, fmt.Sprintf("%s 将按 %d 天策略执行%s", policy.DataType, policy.Retention, policy.NextAction))
			}
		}
		result.Message = fmt.Sprintf("生命周期复核完成，%d 项需要确认", len(result.Findings))
	default:
		return nil, errors.New(errors.ErrCodeInvalidParameter, "unsupported system settings action")
	}

	if !auditedWithSave {
		if err := s.store.InsertAudit(ctx, tenantID, userID.String(), "system_settings_"+strings.ReplaceAll(action, "-", "_"), map[string]interface{}{
			"target_id": req.TargetID,
			"revision":  result.Revision,
			"status":    result.Status,
			"findings":  result.Findings,
		}); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *SystemSettingsService) GetImpact(ctx context.Context, tenantID, userID string) (*SystemSettingsImpact, error) {
	workbench, err := s.GetWorkbench(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	impact := &SystemSettingsImpact{
		TenantID: tenantID, Revision: workbench.Revision, Risk: "medium", Approval: "admin:write",
		AuditAction:    "system_settings_update",
		AffectedScopes: []string{"租户与站点隔离", "RBAC 与令牌", "数据生命周期", "外部集成", "安全与显示参数"},
		Summary:        fmt.Sprintf("当前配置包含 %d 个站点范围、%d 个角色、%d 个集成和 %d 个留存策略。", len(workbench.Settings.Sites), len(workbench.Roles), len(workbench.Settings.Integrations), len(workbench.Settings.RetentionPolicies)),
	}
	if err := s.store.InsertAudit(ctx, tenantID, userID, "system_settings_impact_view", map[string]interface{}{
		"revision": workbench.Revision,
		"risk":     impact.Risk,
	}); err != nil {
		return nil, err
	}
	return impact, nil
}

func (s *SystemSettingsService) RecordNavigationMiss(
	ctx context.Context,
	tenantID, userID, traceID, accessSource string,
	req NavigationMissRequest,
) (*NavigationMissContext, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(userID) == "" {
		return nil, errors.New(errors.ErrCodeMissingParameter, "authenticated tenant and user are required")
	}
	if req.Source != "web-ui" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "source must be web-ui")
	}
	eventUUID := strings.TrimPrefix(strings.TrimSpace(req.EventID), "nav-")
	if _, err := uuid.Parse(eventUUID); err != nil || !strings.HasPrefix(req.EventID, "nav-") {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "event_id must be an opaque nav UUID")
	}
	if _, err := uuid.Parse(strings.TrimSpace(traceID)); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "trace_id must be an opaque UUID")
	}

	tenantName, _, err := s.store.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	settings, _, _, err := s.store.GetSettings(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		defaults := model.DefaultSystemSettings()
		settings = &defaults
	}
	siteName := tenantName
	for _, site := range settings.Sites {
		if site.Kind == "campus" || site.Kind == "site" {
			siteName = site.Name
			break
		}
	}
	createdAt, persistedTraceID, err := s.store.RecordNavigationMiss(ctx, tenantID, userID, req.EventID, traceID, req.Source)
	if err != nil {
		return nil, err
	}
	return &NavigationMissContext{
		EventID: req.EventID, TraceID: persistedTraceID, OccurredAt: createdAt.UTC(), TenantID: tenantID,
		TenantName: tenantName, SiteName: siteName, AccessSource: accessSource,
		AuditAction: "navigation_not_found", Persisted: true,
		Statuses: []NavigationMissStatus{
			{ID: "gateway", Label: "网关服务", State: "healthy", Value: "正常"},
			{ID: "auth", Label: "鉴权服务", State: "healthy", Value: "正常"},
			{ID: "frontend-route", Label: "前端路由", State: "healthy", Value: "正常"},
			{ID: "audit-write", Label: "审计写入", State: "healthy", Value: "正常"},
		},
	}, nil
}

func (s *SystemSettingsService) RecordNavigationSupportRequest(
	ctx context.Context,
	tenantID, userID, traceID string,
	req NavigationSupportRequest,
) (*NavigationSupportContext, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(userID) == "" {
		return nil, errors.New(errors.ErrCodeMissingParameter, "authenticated tenant and user are required")
	}
	eventUUID := strings.TrimPrefix(strings.TrimSpace(req.EventID), "nav-")
	if _, err := uuid.Parse(eventUUID); err != nil || !strings.HasPrefix(req.EventID, "nav-") {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "event_id must be an opaque nav UUID")
	}
	if _, err := uuid.Parse(strings.TrimSpace(traceID)); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "trace_id must be an opaque UUID")
	}
	createdAt, persistedTraceID, err := s.store.RecordNavigationSupportRequest(ctx, tenantID, userID, req.EventID, traceID)
	if err != nil {
		return nil, err
	}
	return &NavigationSupportContext{
		SupportRequestID:  "support-" + eventUUID,
		NavigationEventID: req.EventID,
		TraceID:           persistedTraceID,
		OccurredAt:        createdAt.UTC(),
		Queue:             "平台值班管理员",
		Status:            "queued",
		AuditAction:       "navigation_support_requested",
		Persisted:         true,
	}, nil
}

func validateSystemSettings(settings model.SystemSettings) error {
	if len(settings.Sites) == 0 {
		return errors.New(errors.ErrCodeInvalidParameter, "at least one site is required")
	}
	seenSites := make(map[string]struct{}, len(settings.Sites))
	for _, site := range settings.Sites {
		if strings.TrimSpace(site.ID) == "" || strings.TrimSpace(site.Name) == "" {
			return errors.New(errors.ErrCodeInvalidParameter, "site id and name are required")
		}
		if _, exists := seenSites[site.ID]; exists {
			return errors.New(errors.ErrCodeDuplicateValue, "duplicate site id")
		}
		seenSites[site.ID] = struct{}{}
	}
	seenRetention := make(map[string]struct{}, len(settings.RetentionPolicies))
	for _, policy := range settings.RetentionPolicies {
		if policy.Retention < 1 || policy.Retention > 3650 {
			return errors.New(errors.ErrCodeOutOfRange, "retention_days must be between 1 and 3650")
		}
		key := strings.ToLower(strings.TrimSpace(policy.DataType))
		if key == "" {
			return errors.New(errors.ErrCodeInvalidParameter, "retention data_type is required")
		}
		if _, exists := seenRetention[key]; exists {
			return errors.New(errors.ErrCodeDuplicateValue, "duplicate retention data_type")
		}
		seenRetention[key] = struct{}{}
	}
	seenIntegrations := make(map[string]struct{}, len(settings.Integrations))
	for _, integration := range settings.Integrations {
		if strings.TrimSpace(integration.ID) == "" || strings.TrimSpace(integration.Name) == "" {
			return errors.New(errors.ErrCodeInvalidParameter, "integration id and name are required")
		}
		if strings.Contains(strings.ToLower(integration.SecretRef), "password=") || strings.Contains(strings.ToLower(integration.SecretRef), "token=") {
			return errors.New(errors.ErrCodeInvalidParameter, "integration secrets must use secret_ref")
		}
		if _, exists := seenIntegrations[integration.ID]; exists {
			return errors.New(errors.ErrCodeDuplicateValue, "duplicate integration id")
		}
		seenIntegrations[integration.ID] = struct{}{}
	}
	if settings.Security.RefreshIntervalSec < 5 || settings.Security.RefreshIntervalSec > 3600 {
		return errors.New(errors.ErrCodeOutOfRange, "refresh_interval_sec must be between 5 and 3600")
	}
	return nil
}

func evaluateSecurity(settings model.SystemSettings) []string {
	findings := make([]string, 0)
	if !settings.Security.MFAEnabled {
		findings = append(findings, "MFA 未启用")
	}
	if settings.Security.IPAccessRules == 0 {
		findings = append(findings, "未配置 IP 访问控制")
	}
	if !settings.Security.ScreenMasking {
		findings = append(findings, "大屏脱敏未启用")
	}
	for _, site := range settings.Sites {
		if site.IsolationStatus == "unisolated" {
			findings = append(findings, site.Name+" 未隔离")
		}
	}
	for _, integration := range settings.Integrations {
		if integration.Enabled && integration.SecretRef == "" && integration.ID != "apisix" {
			findings = append(findings, integration.Name+" 未绑定 secret_ref")
		}
	}
	sort.Strings(findings)
	return findings
}

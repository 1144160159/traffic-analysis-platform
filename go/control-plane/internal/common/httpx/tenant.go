////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/httpx/tenant.go
// 修复版本 v2：
// 1. 修复 #12：自动从 Header 提取 RunID、ProbeID 等业务字段
// 2. 增强 TenantExtractor 同时提取多个业务字段
// 3. 新增 BusinessContextExtractor 中间件
////////////////////////////////////////////////////////////////////////////////

package httpx

import (
	"context"
	"net/http"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
)

// ==================== 修复 #12：增强的业务上下文提取 ====================

// BusinessContextExtractor 业务上下文提取中间件（修复 #12：新增）
// 自动从 Header 提取所有业务字段并注入 Context
func BusinessContextExtractor() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// 提取租户ID（优先级：Header > Query > Context > 默认值）
			tenantID := r.Header.Get("X-Tenant-ID")
			if tenantID == "" {
				tenantID = r.URL.Query().Get("tenant_id")
			}
			if tenantID == "" {
				tenantID = GetTenantID(ctx)
			}
			if tenantID == "" {
				tenantID = "default"
			}

			// 修复 #12：提取 RunID
			runID := r.Header.Get("X-Run-ID")
			if runID == "" {
				runID = r.URL.Query().Get("run_id")
			}

			// 修复 #12：提取 ProbeID
			probeID := r.Header.Get("X-Probe-ID")
			if probeID == "" {
				probeID = r.URL.Query().Get("probe_id")
			}

			// 修复 #12：提取 FeatureSetID
			featureSetID := r.Header.Get("X-Feature-Set-ID")
			if featureSetID == "" {
				featureSetID = r.URL.Query().Get("feature_set_id")
			}

			// 提取 EventID（用于幂等）
			eventID := r.Header.Get("X-Event-ID")

			// 注入到 Context
			ctx = context.WithValue(ctx, ContextKeyTenantID, tenantID)
			if runID != "" {
				ctx = context.WithValue(ctx, ContextKeyRunID, runID)
			}
			if probeID != "" {
				ctx = context.WithValue(ctx, ContextKeyProbeID, probeID)
			}
			if featureSetID != "" {
				ctx = context.WithValue(ctx, ContextKeyFeatureSetID, featureSetID)
			}
			if eventID != "" {
				ctx = context.WithValue(ctx, ContextKeyEventID, eventID)
			}

			// 同时注入到 logging context
			ctx = logging.WithTenantID(ctx, tenantID)
			if runID != "" {
				ctx = logging.WithRunID(ctx, runID)
			}
			if probeID != "" {
				ctx = logging.WithProbeID(ctx, probeID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantExtractor 租户提取中间件（保持向后兼容）
func TenantExtractor() Middleware {
	return BusinessContextExtractor()
}

// RequireTenant 强制要求租户ID中间件
func RequireTenant() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := r.Header.Get("X-Tenant-ID")

			if tenantID == "" {
				tenantID = r.URL.Query().Get("tenant_id")
			}

			if tenantID == "" {
				tenantID = GetTenantID(r.Context())
			}

			if tenantID == "" {
				err := errors.New(errors.ErrCodeTenantNotFound, "Tenant ID is required")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			ctx := context.WithValue(r.Context(), ContextKeyTenantID, tenantID)
			ctx = logging.WithTenantID(ctx, tenantID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantValidator 租户验证器接口
type TenantValidator interface {
	ValidateTenant(ctx context.Context, tenantID string) (bool, error)
}

// ValidateTenant 租户验证中间件
func ValidateTenant(validator TenantValidator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := GetTenantID(r.Context())
			if tenantID == "" {
				tenantID = r.Header.Get("X-Tenant-ID")
			}

			if tenantID == "" {
				err := errors.New(errors.ErrCodeTenantNotFound, "Tenant ID is required")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			valid, err := validator.ValidateTenant(r.Context(), tenantID)
			if err != nil {
				appErr := errors.Wrap(err, errors.ErrCodeInternal, "Failed to validate tenant")
				errors.WriteError(w, appErr, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			if !valid {
				err := errors.Newf(errors.ErrCodeTenantNotFound, "Invalid tenant: %s", tenantID)
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TenantIsolation 租户隔离检查中间件
// 确保用户只能访问其所属租户的资源
func TenantIsolation() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从JWT claims中获取用户所属租户
			userTenantID := GetTenantID(r.Context())

			// 从请求中获取目标租户
			requestTenantID := r.Header.Get("X-Tenant-ID")
			if requestTenantID == "" {
				requestTenantID = r.URL.Query().Get("tenant_id")
			}

			// 如果请求指定了租户，检查是否有权限
			if requestTenantID != "" && requestTenantID != userTenantID {
				// 检查用户是否有跨租户权限
				permissions := GetPermissions(r.Context())
				hasCrossTenantAccess := false
				for _, p := range permissions {
					if p == "admin:cross_tenant" {
						hasCrossTenantAccess = true
						break
					}
				}

				if !hasCrossTenantAccess {
					err := errors.New(errors.ErrCodePermissionDenied, "Cross-tenant access denied")
					errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ==================== Context 键定义（新增） ====================

const (
	ContextKeyRunID        contextKey = "run_id"
	ContextKeyProbeID      contextKey = "probe_id"
	ContextKeyFeatureSetID contextKey = "feature_set_id"
	ContextKeyEventID      contextKey = "event_id"
)

// ==================== Context 辅助函数（修复 #12：新增） ====================

// GetRunID 从 context 获取 RunID
func GetRunID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyRunID); v != nil {
		return v.(string)
	}
	return ""
}

// GetProbeID 从 context 获取 ProbeID
func GetProbeID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyProbeID); v != nil {
		return v.(string)
	}
	return ""
}

// GetFeatureSetID 从 context 获取 FeatureSetID
func GetFeatureSetID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyFeatureSetID); v != nil {
		return v.(string)
	}
	return ""
}

// GetEventID 从 context 获取 EventID
func GetEventID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyEventID); v != nil {
		return v.(string)
	}
	return ""
}

// ==================== 业务上下文结构体（新增） ====================

// BusinessContext 业务上下文（修复 #12：新增）
type BusinessContext struct {
	TenantID     string
	UserID       string
	Username     string
	RunID        string
	ProbeID      string
	FeatureSetID string
	EventID      string
	TraceID      string
	RequestID    string
}

// GetBusinessContext 从 context 获取完整业务上下文（修复 #12：新增）
func GetBusinessContext(ctx context.Context) BusinessContext {
	return BusinessContext{
		TenantID:     GetTenantID(ctx),
		UserID:       GetUserID(ctx),
		Username:     GetUsername(ctx),
		RunID:        GetRunID(ctx),
		ProbeID:      GetProbeID(ctx),
		FeatureSetID: GetFeatureSetID(ctx),
		EventID:      GetEventID(ctx),
		TraceID:      GetTraceID(ctx),
		RequestID:    GetRequestID(ctx),
	}
}

// ToMap 转换为 map（用于日志）
func (bc BusinessContext) ToMap() map[string]interface{} {
	m := make(map[string]interface{})
	if bc.TenantID != "" {
		m["tenant_id"] = bc.TenantID
	}
	if bc.UserID != "" {
		m["user_id"] = bc.UserID
	}
	if bc.Username != "" {
		m["username"] = bc.Username
	}
	if bc.RunID != "" {
		m["run_id"] = bc.RunID
	}
	if bc.ProbeID != "" {
		m["probe_id"] = bc.ProbeID
	}
	if bc.FeatureSetID != "" {
		m["feature_set_id"] = bc.FeatureSetID
	}
	if bc.EventID != "" {
		m["event_id"] = bc.EventID
	}
	if bc.TraceID != "" {
		m["trace_id"] = bc.TraceID
	}
	if bc.RequestID != "" {
		m["request_id"] = bc.RequestID
	}
	return m
}

// ==================== 场景特定中间件（新增） ====================

// RequireRunID 要求 RunID 存在（修复 #12：新增）
// 用于回放流量、训练任务等场景
func RequireRunID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			runID := GetRunID(r.Context())
			if runID == "" {
				err := errors.New(errors.ErrCodeMissingParameter, "X-Run-ID header is required")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireProbeID 要求 ProbeID 存在（修复 #12：新增）
// 用于探针上报接口
func RequireProbeID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			probeID := GetProbeID(r.Context())
			if probeID == "" {
				err := errors.New(errors.ErrCodeMissingParameter, "X-Probe-ID header is required")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireEventID 要求 EventID 存在（修复 #12：新增）
// 用于需要幂等保证的接口
func RequireEventID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			eventID := GetEventID(r.Context())
			if eventID == "" {
				err := errors.New(errors.ErrCodeMissingParameter, "X-Event-ID header is required for idempotency")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ==================== 调试辅助（新增） ====================

// LogBusinessContext 记录完整业务上下文（修复 #12：新增）
// 用于调试
func LogBusinessContext() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bc := GetBusinessContext(r.Context())
			logger := logging.FromContext(r.Context())

			logger.Debug("Business context extracted",
				logging.FieldTenantID, bc.TenantID,
				logging.FieldUserID, bc.UserID,
				logging.FieldRunID, bc.RunID,
				logging.FieldProbeID, bc.ProbeID,
				"feature_set_id", bc.FeatureSetID,
				logging.FieldEventID, bc.EventID,
			)

			next.ServeHTTP(w, r)
		})
	}
}

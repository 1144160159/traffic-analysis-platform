package api

import (
	"context"
	"net/http"
	"strings"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"go.uber.org/zap"
)

func (h *Handler) requireAlertWritePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasAlertWritePermission(ctx) {
		return true
	}

	if h.logger != nil {
		h.logger.Warn("Alert write permission denied",
			zap.String("required_scope", authmodel.ScopeAlertWrite),
			zap.String("tenant_id", httpx.GetTenantID(ctx)),
			zap.String("user_id", httpx.GetUserID(ctx)))
	}
	errors.WriteErrorWithStatus(w, http.StatusForbidden,
		errors.ErrCodePermissionDenied,
		"Permission denied: alert:write required",
		httpx.GetTraceID(ctx), r.URL.Path)
	return false
}

func (h *Handler) requireAlertReadPermission(w http.ResponseWriter, r *http.Request) bool {
	return h.requireAlertPermission(w, r, authmodel.ScopeAlertRead)
}

func (h *Handler) requireAlertExportPermission(w http.ResponseWriter, r *http.Request) bool {
	return h.requireAlertPermission(w, r, authmodel.ScopeAlertExport)
}

func (h *Handler) requireAlertPermission(w http.ResponseWriter, r *http.Request, required string) bool {
	ctx := r.Context()
	allowed := func(permission string) bool {
		permission = strings.TrimSpace(permission)
		if permission == authmodel.ScopeAll || permission == authmodel.ScopeAdminAll || permission == required {
			return true
		}
		if strings.HasSuffix(permission, ":*") {
			return strings.HasPrefix(required, permission[:len(permission)-1])
		}
		return false
	}
	if claims := httpx.GetExtendedClaims(ctx); claims != nil {
		if claims.HasRole("admin") || claims.HasPermission(required) || claims.HasPermission(authmodel.ScopeAdminAll) {
			return true
		}
	}
	if httpx.HasRole(ctx, "admin") {
		return true
	}
	for _, permission := range httpx.GetPermissions(ctx) {
		if allowed(permission) {
			return true
		}
	}
	errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied, "Permission denied: "+required+" required", httpx.GetTraceID(ctx), r.URL.Path)
	return false
}

func hasAlertWritePermission(ctx context.Context) bool {
	if claims := httpx.GetExtendedClaims(ctx); claims != nil {
		return claims.HasRole("admin") ||
			claims.HasPermission(authmodel.ScopeAlertWrite) ||
			claims.HasPermission(authmodel.ScopeAdminAll)
	}

	if httpx.HasRole(ctx, "admin") {
		return true
	}

	for _, permission := range httpx.GetPermissions(ctx) {
		if permissionAllowsAlertWrite(permission) {
			return true
		}
	}
	return false
}

func hasAlertReadPermission(ctx context.Context) bool {
	if claims := httpx.GetExtendedClaims(ctx); claims != nil {
		return claims.HasRole("admin") || claims.HasPermission(authmodel.ScopeAlertRead) || claims.HasPermission(authmodel.ScopeAdminAll)
	}
	if httpx.HasRole(ctx, "admin") {
		return true
	}
	for _, permission := range httpx.GetPermissions(ctx) {
		permission = strings.TrimSpace(permission)
		if permission == authmodel.ScopeAll || permission == authmodel.ScopeAlertRead || permission == authmodel.ScopeAdminAll {
			return true
		}
		if strings.HasSuffix(permission, ":*") && strings.HasPrefix(authmodel.ScopeAlertRead, permission[:len(permission)-1]) {
			return true
		}
	}
	return false
}

func permissionAllowsAlertWrite(permission string) bool {
	permission = strings.TrimSpace(permission)
	switch permission {
	case authmodel.ScopeAll, authmodel.ScopeAlertWrite, authmodel.ScopeAdminAll:
		return true
	}
	if strings.HasSuffix(permission, ":*") {
		prefix := permission[:len(permission)-1]
		return strings.HasPrefix(authmodel.ScopeAlertWrite, prefix)
	}
	return false
}

package api

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/middleware"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type SystemSettingsHandler struct {
	service        *service.SystemSettingsService
	authMiddleware *middleware.AuthMiddleware
	logger         *zap.Logger
}

func NewSystemSettingsHandler(
	settingsService *service.SystemSettingsService,
	authMiddleware *middleware.AuthMiddleware,
	logger *zap.Logger,
) *SystemSettingsHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SystemSettingsHandler{service: settingsService, authMiddleware: authMiddleware, logger: logger}
}

func (h *SystemSettingsHandler) RegisterRoutes(r *mux.Router) {
	navigationMiss := r.PathPrefix("/api/v1/auth/navigation-miss").Subrouter()
	navigationMiss.Use(h.authMiddleware.Authenticate)
	navigationMiss.HandleFunc("", h.RecordNavigationMiss).Methods(http.MethodPost)
	navigationMiss.HandleFunc("/support", h.RecordNavigationSupportRequest).Methods(http.MethodPost)

	protected := r.PathPrefix("/api/v1/auth/system-settings").Subrouter()
	protected.Use(h.authMiddleware.Authenticate)
	protected.HandleFunc("", h.GetWorkbench).Methods(http.MethodGet)
	protected.HandleFunc("", h.UpdateSettings).Methods(http.MethodPut)
	protected.HandleFunc("/impact", h.GetImpact).Methods(http.MethodGet)
	protected.HandleFunc("/actions/{action}", h.RunAction).Methods(http.MethodPost)
}

func (h *SystemSettingsHandler) RecordNavigationSupportRequest(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		writeSettingsError(w, r, errors.New(errors.ErrCodeUnauthorized, "Unauthorized"))
		return
	}
	var req service.NavigationSupportRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeSettingsError(w, r, errors.New(errors.ErrCodeInvalidRequest, "Invalid request body"))
		return
	}
	context, err := h.service.RecordNavigationSupportRequest(
		r.Context(), claims.TenantID, claims.UserID.String(), httpx.GetTraceID(r.Context()), req,
	)
	if err != nil {
		h.logger.Warn("Failed to record navigation support request", zap.String("tenant_id", claims.TenantID), zap.Error(err))
		writeSettingsError(w, r, err)
		return
	}
	writeSettingsJSON(w, http.StatusCreated, context)
}

func (h *SystemSettingsHandler) RecordNavigationMiss(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		writeSettingsError(w, r, errors.New(errors.ErrCodeUnauthorized, "Unauthorized"))
		return
	}
	var req service.NavigationMissRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeSettingsError(w, r, errors.New(errors.ErrCodeInvalidRequest, "Invalid request body"))
		return
	}
	context, err := h.service.RecordNavigationMiss(
		r.Context(), claims.TenantID, claims.UserID.String(), httpx.GetTraceID(r.Context()), classifyAccessSource(httpx.GetClientIP(r)), req,
	)
	if err != nil {
		h.logger.Warn("Failed to record navigation miss", zap.String("tenant_id", claims.TenantID), zap.Error(err))
		writeSettingsError(w, r, err)
		return
	}
	writeSettingsJSON(w, http.StatusCreated, context)
}

func classifyAccessSource(value string) string {
	host := value
	if parsedHost, _, err := net.SplitHostPort(value); err == nil {
		host = parsedHost
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "受控访问"
	}
	if ip.IsPrivate() || ip.IsLoopback() {
		return "内网访问"
	}
	return "外网访问"
}

func (h *SystemSettingsHandler) GetWorkbench(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		writeSettingsError(w, r, errors.New(errors.ErrCodeUnauthorized, "Unauthorized"))
		return
	}
	if !claims.HasAnyPermission("admin:read", "admin:write") {
		writeSettingsError(w, r, errors.New(errors.ErrCodePermissionDenied, "admin:read or admin:write required"))
		return
	}
	workbench, err := h.service.GetWorkbench(r.Context(), claims.TenantID)
	if err != nil {
		h.logger.Warn("Failed to load system settings", zap.String("tenant_id", claims.TenantID), zap.Error(err))
		writeSettingsError(w, r, err)
		return
	}
	writeSettingsJSON(w, http.StatusOK, workbench)
}

func (h *SystemSettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		writeSettingsError(w, r, errors.New(errors.ErrCodeUnauthorized, "Unauthorized"))
		return
	}
	if !claims.HasPermission("admin:write") {
		writeSettingsError(w, r, errors.New(errors.ErrCodePermissionDenied, "admin:write required"))
		return
	}
	var req service.UpdateSystemSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSettingsError(w, r, errors.New(errors.ErrCodeInvalidRequest, "Invalid request body"))
		return
	}
	workbench, err := h.service.UpdateSettings(r.Context(), claims.TenantID, claims.UserID, req)
	if err != nil {
		h.logger.Warn("Failed to update system settings", zap.String("tenant_id", claims.TenantID), zap.Error(err))
		writeSettingsError(w, r, err)
		return
	}
	writeSettingsJSON(w, http.StatusOK, workbench)
}

func (h *SystemSettingsHandler) RunAction(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		writeSettingsError(w, r, errors.New(errors.ErrCodeUnauthorized, "Unauthorized"))
		return
	}
	if !claims.HasPermission("admin:write") {
		writeSettingsError(w, r, errors.New(errors.ErrCodePermissionDenied, "admin:write required"))
		return
	}
	var req service.SystemSettingsActionRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSettingsError(w, r, errors.New(errors.ErrCodeInvalidRequest, "Invalid request body"))
			return
		}
	}
	result, err := h.service.RunAction(r.Context(), claims.TenantID, claims.UserID, mux.Vars(r)["action"], req)
	if err != nil {
		h.logger.Warn("System settings action failed", zap.String("tenant_id", claims.TenantID), zap.String("action", mux.Vars(r)["action"]), zap.Error(err))
		writeSettingsError(w, r, err)
		return
	}
	writeSettingsJSON(w, http.StatusOK, result)
}

func (h *SystemSettingsHandler) GetImpact(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		writeSettingsError(w, r, errors.New(errors.ErrCodeUnauthorized, "Unauthorized"))
		return
	}
	if !claims.HasAnyPermission("admin:read", "admin:write") {
		writeSettingsError(w, r, errors.New(errors.ErrCodePermissionDenied, "admin:read or admin:write required"))
		return
	}
	impact, err := h.service.GetImpact(r.Context(), claims.TenantID, claims.UserID.String())
	if err != nil {
		writeSettingsError(w, r, err)
		return
	}
	writeSettingsJSON(w, http.StatusOK, impact)
}

func writeSettingsJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeSettingsError(w http.ResponseWriter, r *http.Request, err error) {
	errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
}

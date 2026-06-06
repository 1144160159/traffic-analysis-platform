////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/api/handler.go
// 修复版：修复 #13, #14 - OIDC state 验证与存储错误处理
// 修复内容：
// 1. #13: OIDC Callback 严格验证 state，禁止空 Redis 绕过
// 2. #14: OIDCLogin state 存储失败时返回错误，不允许继续
////////////////////////////////////////////////////////////////////////////////

package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/middleware"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// Handler 认证 API 处理器
type Handler struct {
	authService    *service.AuthService
	authMiddleware *middleware.AuthMiddleware
	auditLogger    *audit.Logger
	redisClient    *storage.RedisClient
	logger         *zap.Logger
}

// NewHandler 创建处理器（基础版本）
func NewHandler(
	authService *service.AuthService,
	authMiddleware *middleware.AuthMiddleware,
	redisClient *storage.RedisClient,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		authService:    authService,
		authMiddleware: authMiddleware,
		redisClient:    redisClient,
		logger:         logger,
	}
}

// NewHandlerWithAudit 创建带审计日志的处理器（推荐）
func NewHandlerWithAudit(
	authService *service.AuthService,
	authMiddleware *middleware.AuthMiddleware,
	auditLogger *audit.Logger,
	redisClient *storage.RedisClient,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		authService:    authService,
		authMiddleware: authMiddleware,
		auditLogger:    auditLogger,
		redisClient:    redisClient,
		logger:         logger,
	}
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(r *mux.Router) {
	// Public routes
	r.HandleFunc("/api/v1/auth/login", h.Login).Methods("POST")
	r.HandleFunc("/api/v1/auth/refresh", h.RefreshToken).Methods("POST")
	r.HandleFunc("/api/v1/auth/oidc/login", h.OIDCLogin).Methods("GET")
	r.HandleFunc("/api/v1/auth/oidc/callback", h.OIDCCallback).Methods("GET")

	// Protected routes
	protected := r.PathPrefix("/api/v1/auth").Subrouter()
	protected.Use(h.authMiddleware.Authenticate)
	protected.HandleFunc("/logout", h.Logout).Methods("POST")
	protected.HandleFunc("/me", h.GetCurrentUser).Methods("GET")
	protected.HandleFunc("/validate", h.ValidateToken).Methods("GET")

	// Health
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")
}

// Login 登录
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidRequest,
			"Invalid request body", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if req.TenantID == "" {
		req.TenantID = r.Header.Get("X-Tenant-ID")
	}
	if req.TenantID == "" {
		req.TenantID = "default"
	}

	clientIP := httpx.GetClientIP(r)
	userAgent := r.UserAgent()

	resp, err := h.authService.Login(r.Context(), &req)
	if err != nil {
		h.logger.Warn("Login failed",
			zap.String("username", req.Username),
			zap.String("tenant_id", req.TenantID),
			zap.String("client_ip", clientIP),
			zap.Error(err))

		// 记录失败审计日志
		if h.auditLogger != nil {
			h.auditLogger.LogLogin(r.Context(), req.TenantID, "", req.Username, clientIP, userAgent, false)
		}

		errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 记录成功审计日志
	if h.auditLogger != nil {
		h.auditLogger.LogLogin(r.Context(), req.TenantID, resp.User.UserID, resp.User.Username, clientIP, userAgent, true)
	}

	h.logger.Info("User logged in",
		zap.String("user_id", resp.User.UserID),
		zap.String("username", resp.User.Username),
		zap.String("tenant_id", req.TenantID),
		zap.String("client_ip", clientIP))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// RefreshRequest 刷新请求
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshToken 刷新令牌
func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidRequest,
			"Invalid request body", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if req.RefreshToken == "" {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"refresh_token is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	resp, err := h.authService.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		h.logger.Warn("Token refresh failed", zap.Error(err))

		// 记录失败审计日志
		if h.auditLogger != nil {
			h.auditLogger.Log(r.Context(), &audit.AuditEvent{
				EventType:    audit.EventTypeTokenRefresh,
				Action:       "token_refresh_failed",
				ResourceType: "session",
				Result:       audit.ResultFailure,
				ErrorMsg:     err.Error(),
				IPAddr:       httpx.GetClientIP(r),
				UserAgent:    r.UserAgent(),
			})
		}

		errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 记录成功审计日志
	if h.auditLogger != nil {
		h.auditLogger.Log(r.Context(), &audit.AuditEvent{
			EventType:    audit.EventTypeTokenRefresh,
			TenantID:     resp.User.TenantID,
			UserID:       resp.User.UserID,
			Username:     resp.User.Username,
			Action:       "token_refresh",
			ResourceType: "session",
			Result:       audit.ResultSuccess,
			IPAddr:       httpx.GetClientIP(r),
			UserAgent:    r.UserAgent(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Logout 登出
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if err := h.authService.Logout(r.Context(), claims.SessionID); err != nil {
		h.logger.Error("Logout failed", zap.Error(err))
		// 不阻止登出流程，继续返回成功
	}

	// 记录登出审计日志
	if h.auditLogger != nil {
		h.auditLogger.Log(r.Context(), &audit.AuditEvent{
			EventType:    audit.EventTypeLogout,
			TenantID:     claims.TenantID,
			UserID:       claims.UserID.String(),
			Username:     claims.Username,
			Action:       "logout",
			ResourceType: "session",
			ResourceID:   claims.SessionID,
			Result:       audit.ResultSuccess,
			IPAddr:       httpx.GetClientIP(r),
			UserAgent:    r.UserAgent(),
		})
	}

	h.logger.Info("User logged out",
		zap.String("user_id", claims.UserID.String()),
		zap.String("username", claims.Username))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out successfully"})
}

// GetCurrentUser 获取当前用户信息
func (h *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":     claims.UserID.String(),
		"tenant_id":   claims.TenantID,
		"username":    claims.Username,
		"email":       claims.Email,
		"roles":       claims.Roles,
		"permissions": claims.Permissions,
	})
}

// ValidateToken 验证令牌
func (h *Handler) ValidateToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid":       true,
		"user_id":     claims.UserID.String(),
		"tenant_id":   claims.TenantID,
		"expires_at":  claims.ExpiresAt.Time.Format(time.RFC3339),
		"permissions": claims.Permissions,
	})
}

// OIDCLogin OIDC 登录（修复 #14：state 存储失败时返回错误）
func (h *Handler) OIDCLogin(w http.ResponseWriter, r *http.Request) {
	// 修复 #14：Redis 不可用时，拒绝 OIDC 登录
	if h.redisClient == nil {
		h.logger.Error("OIDC login requires Redis but Redis is not available")
		errors.WriteErrorWithStatus(w, http.StatusServiceUnavailable, errors.ErrCodeOIDCError,
			"OIDC login is temporarily unavailable (Redis required)", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 生成 state
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		h.logger.Error("Failed to generate OIDC state", zap.Error(err))
		errors.WriteErrorWithStatus(w, http.StatusInternalServerError, errors.ErrCodeInternal,
			"Failed to generate state", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}
	state := hex.EncodeToString(stateBytes)

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "default"
	}

	stateData := map[string]string{
		"tenant_id": tenantID,
		"redirect":  r.URL.Query().Get("redirect"),
		"client_ip": httpx.GetClientIP(r),
	}
	stateJSON, _ := json.Marshal(stateData)

	// 修复 #14：保存 state 到 Redis，失败时返回错误
	err := h.redisClient.Client().Set(r.Context(), "oidc_state:"+state, string(stateJSON), 10*time.Minute).Err()
	if err != nil {
		h.logger.Error("Failed to save OIDC state to Redis", zap.Error(err))
		errors.WriteErrorWithStatus(w, http.StatusServiceUnavailable, errors.ErrCodeInternal,
			"Authentication service temporarily unavailable", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	authURL := h.authService.GetOIDCAuthURL(state)
	if authURL == "" {
		errors.WriteErrorWithStatus(w, http.StatusNotImplemented, errors.ErrCodeOIDCError,
			"OIDC is not configured", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// OIDCCallback OIDC 回调（修复 #13：严格验证 state）
func (h *Handler) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		h.logger.Error("OIDC error",
			zap.String("error", errorParam),
			zap.String("description", errorDesc))

		if h.auditLogger != nil {
			h.auditLogger.Log(r.Context(), &audit.AuditEvent{
				EventType:    audit.EventTypeLoginFailed,
				Action:       "oidc_login_failed",
				ResourceType: "session",
				Result:       audit.ResultFailure,
				ErrorMsg:     errorParam + ": " + errorDesc,
				IPAddr:       httpx.GetClientIP(r),
				UserAgent:    r.UserAgent(),
			})
		}

		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeOIDCError,
			"Authentication failed: "+errorDesc, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if code == "" || state == "" {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"Missing code or state parameter", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 修复 #13：严格验证 Redis 可用性
	if h.redisClient == nil {
		h.logger.Error("OIDC callback failed: Redis is not available for state verification")
		errors.WriteErrorWithStatus(w, http.StatusServiceUnavailable, errors.ErrCodeOIDCError,
			"OIDC state verification not available (Redis disabled)", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 修复 #13：验证 state，不允许空字符串绕过
	stateJSON, err := h.redisClient.Client().Get(r.Context(), "oidc_state:"+state).Result()
	if err != nil {
		if err == redis.Nil {
			h.logger.Warn("Invalid or expired OIDC state", zap.String("state", state))
			errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidParameter,
				"Invalid or expired state parameter", httpx.GetTraceID(r.Context()), r.URL.Path)
		} else {
			h.logger.Error("Failed to verify OIDC state", zap.Error(err))
			errors.WriteErrorWithStatus(w, http.StatusInternalServerError, errors.ErrCodeInternal,
				"Failed to verify state", httpx.GetTraceID(r.Context()), r.URL.Path)
		}

		// 记录失败审计
		if h.auditLogger != nil {
			h.auditLogger.Log(r.Context(), &audit.AuditEvent{
				EventType:    audit.EventTypeLoginFailed,
				Action:       "oidc_invalid_state",
				ResourceType: "session",
				Result:       audit.ResultFailure,
				ErrorMsg:     "invalid state parameter",
				IPAddr:       httpx.GetClientIP(r),
				UserAgent:    r.UserAgent(),
			})
		}
		return
	}

	// 删除已使用的 state
	h.redisClient.Client().Del(r.Context(), "oidc_state:"+state)

	var stateData map[string]string
	if err := json.Unmarshal([]byte(stateJSON), &stateData); err != nil {
		h.logger.Error("Failed to parse state data", zap.Error(err))
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidParameter,
			"Invalid state data", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	tenantID := stateData["tenant_id"]
	redirectURL := stateData["redirect"]
	clientIP := stateData["client_ip"]

	resp, err := h.authService.HandleOIDCCallback(r.Context(), code, tenantID)
	if err != nil {
		h.logger.Error("OIDC callback failed", zap.Error(err))

		if h.auditLogger != nil {
			h.auditLogger.Log(r.Context(), &audit.AuditEvent{
				EventType:    audit.EventTypeLoginFailed,
				TenantID:     tenantID,
				Action:       "oidc_login_failed",
				ResourceType: "session",
				Result:       audit.ResultFailure,
				ErrorMsg:     err.Error(),
				IPAddr:       clientIP,
				UserAgent:    r.UserAgent(),
			})
		}

		errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 记录成功审计
	if h.auditLogger != nil {
		h.auditLogger.LogLogin(r.Context(), tenantID, resp.User.UserID, resp.User.Username, clientIP, r.UserAgent(), true)
	}

	h.logger.Info("User logged in via OIDC",
		zap.String("user_id", resp.User.UserID),
		zap.String("username", resp.User.Username),
		zap.String("tenant_id", tenantID))

	if redirectURL != "" {
		redirectURL += "?access_token=" + resp.AccessToken + "&token_type=" + resp.TokenType
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HealthCheck 健康检查
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	redisOK := true
	if h.redisClient != nil {
		if err := h.redisClient.Ping(r.Context()); err != nil {
			redisOK = false
		}
	}

	status := "healthy"
	statusCode := http.StatusOK
	if !redisOK {
		status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
		"components": map[string]string{
			"redis": boolToStatus(redisOK),
		},
	})
}

func boolToStatus(ok bool) string {
	if ok {
		return "healthy"
	}
	return "unhealthy"
}

package dlq

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/auth"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
)

const defaultReplayPath = "/api/v1/dlq/replay/fallback"

type ReplayTokenValidator interface {
	ValidateWithScopes(ctx context.Context, probeID, token string) (*auth.TokenInfo, error)
}

type ReplayController interface {
	ReplayFallback(ctx context.Context, req ReplayRequest) (*ReplayResult, error)
}

type ReplayHTTPHandler struct {
	controller ReplayController
	validator  ReplayTokenValidator
	logger     *zap.Logger
}

func NewReplayHTTPHandler(controller ReplayController, validator ReplayTokenValidator, logger *zap.Logger) *ReplayHTTPHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReplayHTTPHandler{
		controller: controller,
		validator:  validator,
		logger:     logger,
	}
}

func (h *ReplayHTTPHandler) Register(mux *http.ServeMux) {
	h.RegisterPath(mux, defaultReplayPath)
}

func (h *ReplayHTTPHandler) RegisterPath(mux *http.ServeMux, path string) {
	mux.HandleFunc(path, h.HandleReplayFallback)
}

func (h *ReplayHTTPHandler) HandleReplayFallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeReplayError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if h.controller == nil {
		writeReplayError(w, http.StatusServiceUnavailable, "REPLAY_UNAVAILABLE", "dlq replay controller is not configured")
		return
	}

	tokenInfo, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	var req ReplayRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeReplayError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	if strings.TrimSpace(req.TenantID) == "" {
		req.TenantID = tokenInfo.TenantID
	}
	if strings.TrimSpace(req.RequestedBy) == "" {
		req.RequestedBy = tokenInfo.ProbeID
	}

	result, err := h.controller.ReplayFallback(r.Context(), req)
	if err != nil {
		writeReplayError(w, http.StatusBadRequest, "REPLAY_REJECTED", err.Error())
		return
	}

	writeReplayJSON(w, http.StatusOK, result)
}

func (h *ReplayHTTPHandler) authenticate(w http.ResponseWriter, r *http.Request) (*auth.TokenInfo, bool) {
	if h.validator == nil {
		writeReplayError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "token validator is not configured")
		return nil, false
	}

	token, err := bearerToken(r.Header.Get("Authorization"))
	if err != nil {
		writeReplayError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
		return nil, false
	}

	probeID := strings.TrimSpace(r.Header.Get("X-Probe-ID"))
	tokenInfo, err := h.validator.ValidateWithScopes(r.Context(), probeID, token)
	if err != nil {
		h.logger.Warn("DLQ replay token validation failed", zap.Error(err), zap.String("probe_id", probeID))
		writeReplayError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
		return nil, false
	}

	if !hasReplayScope(tokenInfo.Scopes) {
		h.logger.Warn("DLQ replay permission denied",
			zap.String("tenant_id", tokenInfo.TenantID),
			zap.String("probe_id", tokenInfo.ProbeID),
			zap.Strings("scopes", tokenInfo.Scopes))
		writeReplayError(w, http.StatusForbidden, "FORBIDDEN", "dlq:replay scope required")
		return nil, false
	}

	return tokenInfo, true
}

func bearerToken(header string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("bearer token required")
	}
	return strings.TrimSpace(parts[1]), nil
}

func hasReplayScope(scopes []string) bool {
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == config.ScopeWildcard || scope == config.ScopeDLQReplay || scope == config.ScopeAdminAll || scope == config.ScopeAdminWrite {
			return true
		}
		if strings.HasSuffix(scope, ":*") {
			prefix := scope[:len(scope)-1]
			if strings.HasPrefix(config.ScopeDLQReplay, prefix) {
				return true
			}
		}
	}
	return false
}

func writeReplayJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeReplayError(w http.ResponseWriter, status int, code, message string) {
	writeReplayJSON(w, status, map[string]string{
		"code":    code,
		"message": message,
	})
}

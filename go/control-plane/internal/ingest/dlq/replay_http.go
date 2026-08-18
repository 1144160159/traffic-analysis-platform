package dlq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/auth"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
)

const defaultReplayPath = "/api/v1/dlq/replay/fallback"

const defaultReplayApprovalPath = "/api/v1/dlq/replay/approvals"

type ReplayTokenValidator interface {
	ValidateWithScopes(ctx context.Context, probeID, token string) (*auth.TokenInfo, error)
}

type ReplayController interface {
	ReplayFallback(ctx context.Context, req ReplayRequest) (*ReplayResult, error)
}

type ReplayHTTPHandler struct {
	controller ReplayController
	approvals  ReplayApprovalStore
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

// SetApprovalStore 注入审批台账;未注入时审批创建端点不可用。
func (h *ReplayHTTPHandler) SetApprovalStore(s ReplayApprovalStore) {
	h.approvals = s
}

func (h *ReplayHTTPHandler) Register(mux *http.ServeMux) {
	h.RegisterPath(mux, defaultReplayPath)
	mux.HandleFunc(defaultReplayApprovalPath, h.HandleCreateApproval)
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

	// 租户隔离:回放目标租户必须以令牌租户为准。请求体中的 tenant_id 仅允许
	// 与令牌租户一致(为空时回填令牌租户),禁止跨租户触发全量回放。
	if strings.TrimSpace(req.TenantID) != "" && strings.TrimSpace(req.TenantID) != tokenInfo.TenantID {
		writeReplayError(w, http.StatusForbidden, "FORBIDDEN", "replay tenant must match token tenant")
		return
	}
	req.TenantID = tokenInfo.TenantID
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

// HandleCreateApproval 创建 DLQ 回放审批记录(管理员操作)。
// 审批与执行分离:先由具备 admin 权限的审批者创建台账记录,回放时逐字段校验。
func (h *ReplayHTTPHandler) HandleCreateApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeReplayError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if h.approvals == nil {
		writeReplayError(w, http.StatusServiceUnavailable, "APPROVAL_UNAVAILABLE", "dlq replay approval store is not configured")
		return
	}

	tokenInfo, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !hasApprovalAdminScope(tokenInfo.Scopes) {
		writeReplayError(w, http.StatusForbidden, "FORBIDDEN", "admin scope required to create dlq replay approval")
		return
	}

	var req struct {
		TenantID    string `json:"tenant_id"`
		ApprovalID  string `json:"approval_id"`
		RequestedBy string `json:"requested_by"`
		ApprovedBy  string `json:"approved_by"`
		Reason      string `json:"reason"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeReplayError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	if strings.TrimSpace(req.TenantID) != "" && strings.TrimSpace(req.TenantID) != tokenInfo.TenantID {
		writeReplayError(w, http.StatusForbidden, "FORBIDDEN", "approval tenant must match token tenant")
		return
	}
	req.TenantID = tokenInfo.TenantID
	if strings.TrimSpace(req.ApprovalID) == "" || strings.TrimSpace(req.ApprovedBy) == "" ||
		strings.TrimSpace(req.RequestedBy) == "" {
		writeReplayError(w, http.StatusBadRequest, "INVALID_REQUEST", "approval_id, approved_by and requested_by are required")
		return
	}
	if strings.EqualFold(strings.TrimSpace(req.ApprovedBy), strings.TrimSpace(req.RequestedBy)) {
		writeReplayError(w, http.StatusBadRequest, "INVALID_REQUEST", "approved_by must be different from requested_by")
		return
	}

	// request hash:canonical 字段哈希,同 key 异 hash 将被权威层拒绝(ENG-CMD-002)。
	requestHash := approvalRequestHash(req.TenantID, req.ApprovalID, req.RequestedBy, req.ApprovedBy, req.Reason)
	approval := ReplayApproval{
		TenantID:    strings.TrimSpace(req.TenantID),
		ApprovalID:  strings.TrimSpace(req.ApprovalID),
		RequestedBy: strings.TrimSpace(req.RequestedBy),
		ApprovedBy:  strings.TrimSpace(req.ApprovedBy),
		Status:      ApprovalStatusApproved,
		Reason:      strings.TrimSpace(req.Reason),
		RequestHash: requestHash,
		CreatedAt:   time.Now(),
	}
	if err := h.approvals.CreateApproval(r.Context(), approval); err != nil {
		h.logger.Warn("DLQ replay approval create failed",
			zap.String("tenant_id", approval.TenantID),
			zap.String("approval_id", approval.ApprovalID),
			zap.Error(err))
		writeReplayError(w, http.StatusConflict, "APPROVAL_CONFLICT", err.Error())
		return
	}
	writeReplayJSON(w, http.StatusCreated, approval)
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

func hasApprovalAdminScope(scopes []string) bool {
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == config.ScopeWildcard || scope == config.ScopeAdminAll || scope == config.ScopeAdminWrite {
			return true
		}
	}
	return false
}

// approvalRequestHash 规范化字段 SHA-256(与回放请求绑定同一审批事实)。
func approvalRequestHash(tenantID, approvalID, requestedBy, approvedBy, reason string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(tenantID),
		strings.TrimSpace(approvalID),
		strings.TrimSpace(requestedBy),
		strings.TrimSpace(approvedBy),
		strings.TrimSpace(reason),
	}, "\x1f")))
	return hex.EncodeToString(sum[:])
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

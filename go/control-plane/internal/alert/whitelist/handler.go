package whitelist

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

// Handler 白名单 API 处理器
type Handler struct {
	repo   *Repository
	logger *zap.Logger
}

func NewHandler(repo *Repository, logger *zap.Logger) *Handler {
	return &Handler{repo: repo, logger: logger}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/whitelist", h.List).Methods("GET")
	r.HandleFunc("/whitelist", h.Create).Methods("POST")
	r.HandleFunc("/whitelist/{id}", h.Delete).Methods("DELETE")
	r.HandleFunc("/whitelist/check", h.Check).Methods("POST")
}

// List 列出租户白名单
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" { tenantID = "default" }
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 { limit = 50 }
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	entries, total, err := h.repo.List(ctx, tenantID, limit, offset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"entries": entries, "total": total, "limit": limit, "offset": offset})
}

// Create 创建白名单条目
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var entry Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid body")
		return
	}
	if entry.TenantID == "" { entry.TenantID = "default" }
	if entry.Type == "" || entry.Value == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "MISSING_PARAM", "type and value required")
		return
	}
	if err := h.repo.Create(ctx, &entry); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONCreated(w, ctx, &entry)
}

// Delete 删除白名单条目
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := mux.Vars(r)["id"]
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" { tenantID = "default" }
	if err := h.repo.Delete(ctx, tenantID, id); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]string{"status": "deleted", "id": id})
}

// Check 检查值是否在白名单中
func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		TenantID string `json:"tenant_id"`
		Value    string `json:"value"`
		Type     string `json:"type"` // ip | domain | fingerprint
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.TenantID == "" { req.TenantID = "default" }
	whitelisted := h.repo.IsWhitelisted(ctx, req.TenantID, req.Value)
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"value": req.Value, "whitelisted": whitelisted})
}

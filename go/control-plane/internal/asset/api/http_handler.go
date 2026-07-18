package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
)

type HTTPHandler struct {
	svc           *service.AssetService
	logger        *zap.Logger
	jwtSigningKey string
}

func NewHTTPHandler(svc *service.AssetService, logger *zap.Logger) *HTTPHandler {
	handler := &HTTPHandler{svc: svc, logger: logger}
	if svc != nil {
		handler.jwtSigningKey = svc.JWTSigningKey()
	}
	return handler
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/assets")
	switch {
	case path == "" || path == "/":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.listAssets(w, r)
	case path == "/discovery/credentials":
		switch r.Method {
		case http.MethodGet:
			h.listDiscoveryCredentials(w, r)
		case http.MethodPost:
			h.registerDiscoveryCredential(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case path == "/discovery/runs":
		switch r.Method {
		case http.MethodGet:
			h.listDiscoveryRuns(w, r)
		case http.MethodPost:
			h.runActiveDiscovery(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case path == "/discovery/neighbors":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.listTopologyLinks(w, r)
	case path == "/stats":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.getAssetStats(w, r)
	case strings.HasSuffix(path, "/history"):
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		assetID := strings.TrimSuffix(strings.Trim(path, "/"), "/history")
		h.getAssetHistory(w, r, assetID)
	case strings.HasSuffix(path, "/details"):
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		assetID := strings.TrimSuffix(strings.Trim(path, "/"), "/details")
		h.getAssetDetails(w, r, assetID)
	case strings.HasSuffix(path, "/topology"):
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		assetID := strings.TrimSuffix(strings.Trim(path, "/"), "/topology")
		h.getAssetTopology(w, r, assetID)
	default:
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.getAsset(w, r, strings.Trim(path, "/"))
	}
}

func (h *HTTPHandler) listAssets(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	limit := intQuery(r, "limit", 50)
	offset := intQuery(r, "offset", 0)
	assetType := r.URL.Query().Get("asset_type")
	if assetType != "" && !service.IsAssetType(assetType) {
		writeError(w, http.StatusBadRequest, "invalid asset_type")
		return
	}

	assets, total, err := h.svc.ListAssetsFiltered(r.Context(), identity.TenantID, config.AssetListFilter{
		AssetType:  assetType,
		Status:     strings.TrimSpace(r.URL.Query().Get("status")),
		Search:     strings.TrimSpace(r.URL.Query().Get("search")),
		Department: strings.TrimSpace(r.URL.Query().Get("department")),
		Campus:     strings.TrimSpace(r.URL.Query().Get("campus")),
	}, limit, offset)
	if err != nil {
		h.logger.Warn("list assets failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": assets,
		"pagination": map[string]any{
			"total":    total,
			"limit":    limit,
			"offset":   offset,
			"has_more": offset+len(assets) < total,
		},
	})
}

func (h *HTTPHandler) getAsset(w http.ResponseWriter, r *http.Request, assetID string) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	if assetID == "" {
		writeError(w, http.StatusBadRequest, "asset id required")
		return
	}
	asset, err := h.svc.GetAsset(r.Context(), identity.TenantID, assetID, r.URL.Query().Get("mac_address"))
	if err != nil {
		h.logger.Warn("get asset failed", zap.String("asset_id", assetID), zap.Error(err))
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": asset})
}

func (h *HTTPHandler) getAssetStats(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	stats, err := h.svc.GetAssetStatsFiltered(r.Context(), identity.TenantID, config.AssetListFilter{
		AssetType:  strings.TrimSpace(r.URL.Query().Get("asset_type")),
		Status:     strings.TrimSpace(r.URL.Query().Get("status")),
		Search:     strings.TrimSpace(r.URL.Query().Get("search")),
		Department: strings.TrimSpace(r.URL.Query().Get("department")),
		Campus:     strings.TrimSpace(r.URL.Query().Get("campus")),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": stats})
}

func (h *HTTPHandler) getAssetHistory(w http.ResponseWriter, r *http.Request, assetID string) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	if assetID == "" {
		writeError(w, http.StatusBadRequest, "asset id required")
		return
	}
	events, err := h.svc.GetAssetHistory(r.Context(), identity.TenantID, assetID, intQuery(r, "limit", 20))
	if err != nil {
		h.logger.Warn("get asset history failed", zap.String("asset_id", assetID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": events})
}

func (h *HTTPHandler) getAssetDetails(w http.ResponseWriter, r *http.Request, assetID string) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	if assetID == "" {
		writeError(w, http.StatusBadRequest, "asset id required")
		return
	}
	details, err := h.svc.GetAssetDetails(r.Context(), identity.TenantID, assetID)
	if err != nil {
		h.logger.Warn("get asset details failed", zap.String("asset_id", assetID), zap.Error(err))
		writeError(w, http.StatusNotFound, "asset details not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": details})
}

func (h *HTTPHandler) getAssetTopology(w http.ResponseWriter, r *http.Request, assetID string) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	if assetID == "" {
		writeError(w, http.StatusBadRequest, "asset id required")
		return
	}
	graph, err := h.svc.GetAssetTopology(r.Context(), identity.TenantID, assetID)
	if err != nil {
		h.logger.Warn("get asset topology failed", zap.String("asset_id", assetID), zap.Error(err))
		writeError(w, http.StatusNotFound, "asset topology not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": graph})
}

func (h *HTTPHandler) registerDiscoveryCredential(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireAssetDiscoveryWrite(w, r)
	if !ok {
		return
	}
	var credential config.DiscoveryCredential
	if err := json.NewDecoder(r.Body).Decode(&credential); err != nil {
		writeError(w, http.StatusBadRequest, "invalid discovery credential payload")
		return
	}
	credential.TenantID = identity.TenantID
	credential.CreatedBy = auditActor(identity)
	created, err := h.svc.RegisterDiscoveryCredential(r.Context(), &credential)
	if err != nil {
		h.logger.Warn("register discovery credential failed", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.recordAudit(r, identity, "ASSET_DISCOVERY_CREDENTIAL_REGISTER", "asset_discovery_credential", created.CredentialID, map[string]interface{}{
		"name":               created.Name,
		"protocol":           created.Protocol,
		"endpoint":           created.Endpoint,
		"secret_ref_present": created.SecretRef != "",
	})
	writeJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (h *HTTPHandler) listDiscoveryCredentials(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListDiscoveryCredentials(r.Context(), identity.TenantID, intQuery(r, "limit", 20))
	if err != nil {
		h.logger.Warn("list discovery credentials failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *HTTPHandler) runActiveDiscovery(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireAssetDiscoveryWrite(w, r)
	if !ok {
		return
	}
	var req config.ActiveDiscoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid active discovery payload")
		return
	}
	req.TenantID = identity.TenantID
	req.RequestedBy = auditActor(identity)
	result, err := h.svc.RunActiveDiscovery(r.Context(), &req)
	if err != nil {
		h.logger.Warn("run active discovery failed", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if result != nil && result.Run != nil {
		h.recordAudit(r, identity, "ASSET_ACTIVE_DISCOVERY_RUN", "asset_discovery_run", result.Run.RunID, map[string]interface{}{
			"mode":             result.Run.Mode,
			"target_cidr":      result.Run.TargetCIDR,
			"credential_id":    result.Run.CredentialID,
			"status":           result.Run.Status,
			"accepted_assets":  result.AcceptedAssets,
			"accepted_links":   result.AcceptedLinks,
			"rejected_records": result.RejectedRecords,
		})
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
}

func (h *HTTPHandler) listDiscoveryRuns(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	runs, err := h.svc.ListDiscoveryRuns(r.Context(), identity.TenantID, intQuery(r, "limit", 20))
	if err != nil {
		h.logger.Warn("list discovery runs failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": runs})
}

func (h *HTTPHandler) listTopologyLinks(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	links, err := h.svc.ListTopologyLinks(r.Context(), identity.TenantID, r.URL.Query().Get("asset_id"), intQuery(r, "limit", 50))
	if err != nil {
		h.logger.Warn("list topology links failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": links})
}

func tenantFromRequest(r *http.Request) string {
	if tenantID := r.Header.Get("X-Tenant-ID"); tenantID != "" {
		return tenantID
	}
	return ""
}

func actorFromRequest(r *http.Request) string {
	for _, header := range []string{"X-User-ID", "X-User", "X-Username"} {
		if value := r.Header.Get(header); value != "" {
			return value
		}
	}
	return r.URL.Query().Get("requested_by")
}

func intQuery(r *http.Request, key string, fallback int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"success": false,
		"message": message,
	})
}

func (h *HTTPHandler) recordAudit(r *http.Request, identity requestIdentity, action, objectType, objectID string, detail map[string]interface{}) {
	if h == nil || h.svc == nil {
		return
	}
	if detail == nil {
		detail = map[string]interface{}{}
	}
	detail["actor"] = auditActor(identity)
	if err := h.svc.RecordAuditLog(r.Context(), identity.TenantID, auditUserID(identity), action, objectType, objectID, detail, clientIP(r), r.UserAgent()); err != nil {
		h.logger.Warn("record asset discovery audit failed",
			zap.String("action", action),
			zap.String("object_type", objectType),
			zap.String("object_id", objectID),
			zap.Error(err))
	}
}

func AssetRecordFromRequest(r *http.Request) (*config.AssetRecord, error) {
	var rec config.AssetRecord
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		return nil, err
	}
	if rec.TenantID == "" {
		rec.TenantID = tenantFromRequest(r)
	}
	return &rec, nil
}

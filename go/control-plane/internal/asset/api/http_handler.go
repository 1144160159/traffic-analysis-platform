package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type HTTPHandler struct {
	svc                     *service.AssetService
	logger                  *zap.Logger
	jwtSigningKey           string
	cursorEnabled           bool
	discoveryJobsV2Enabled  bool
	exportJobsV1Enabled     bool
	detailSnapshotV1Enabled bool
	governanceV1Enabled     bool
}

func NewHTTPHandler(svc *service.AssetService, logger *zap.Logger) *HTTPHandler {
	handler := &HTTPHandler{svc: svc, logger: logger}
	if svc != nil {
		handler.jwtSigningKey = svc.JWTSigningKey()
		handler.cursorEnabled = svc.AssetCursorV2Enabled()
		handler.discoveryJobsV2Enabled = svc.DiscoveryJobsV2Enabled()
		handler.exportJobsV1Enabled = svc.AssetExportJobsEnabled()
		handler.detailSnapshotV1Enabled = svc.AssetDetailSnapshotV1Enabled()
		handler.governanceV1Enabled = svc.AssetGovernanceV1Enabled()
	}
	return handler
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/assets")
	switch {
	case path == "" || path == "/":
		switch r.Method {
		case http.MethodGet:
			h.listAssets(w, r)
		case http.MethodPost:
			h.upsertAsset(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
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
	case strings.HasPrefix(path, "/discovery/runs/"):
		h.discoveryRunResource(w, r, strings.TrimPrefix(path, "/discovery/runs/"))
	case path == "/exports":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.createAssetExport(w, r)
	case strings.HasPrefix(path, "/exports/"):
		h.assetExportResource(w, r, strings.TrimPrefix(path, "/exports/"))
	case path == "/preferences/columns":
		switch r.Method {
		case http.MethodGet:
			h.getAssetColumnPreference(w, r)
		case http.MethodPut:
			h.putAssetColumnPreference(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case strings.HasPrefix(path, "/governance/work-orders/"):
		h.assetGovernanceWorkOrderResource(w, r, strings.TrimPrefix(path, "/governance/work-orders/"))
	case strings.HasSuffix(path, "/governance/work-orders"):
		assetID := strings.TrimSuffix(strings.Trim(path, "/"), "/governance/work-orders")
		h.assetGovernanceCollection(w, r, assetID)
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
	case strings.HasSuffix(path, "/snapshot"):
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !h.detailSnapshotV1Enabled {
			httpx.JSONError(w, r.Context(), http.StatusNotFound, "FEATURE_DISABLED", "asset detail snapshot v1 is disabled")
			return
		}
		assetID := strings.TrimSuffix(strings.Trim(path, "/"), "/snapshot")
		h.getAssetDetailSnapshot(w, r, assetID)
	default:
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.getAsset(w, r, strings.Trim(path, "/"))
	}
}

func (h *HTTPHandler) upsertAsset(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireAssetDiscoveryWrite(w, r)
	if !ok {
		return
	}
	traceID := strings.TrimSpace(r.Header.Get("X-Trace-ID"))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = traceID
	}
	var payload struct {
		Asset            config.AssetRecord `json:"asset"`
		ExpectedRevision int64              `json:"expected_revision"`
		Reason           string             `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeAssetCommandError(w, http.StatusBadRequest, traceID, "invalid_request", "invalid asset payload")
		return
	}
	if payload.Asset.TenantID != "" && payload.Asset.TenantID != identity.TenantID {
		writeAssetCommandError(w, http.StatusConflict, traceID, "tenant_conflict", "asset tenant conflicts with authenticated tenant")
		return
	}
	payload.Asset.TenantID = identity.TenantID
	result, err := h.svc.UpsertAssetAtomic(r.Context(), &payload.Asset, config.AssetUpsertCommand{
		ActionID:         config.AssetUpsertAction,
		ExpectedRevision: payload.ExpectedRevision,
		IdempotencyKey:   strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		Actor:            auditActor(identity),
		Reason:           strings.TrimSpace(payload.Reason),
		TraceID:          traceID,
		RequestID:        requestID,
		ClientIP:         clientIP(r),
		UserAgent:        r.UserAgent(),
	})
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrAssetRevisionConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "revision_conflict", "expected_revision does not match the authoritative asset")
		case errors.Is(err, repository.ErrAssetIdempotencyConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "idempotency_conflict", "Idempotency-Key was already used for a different immutable request")
		default:
			h.logger.Warn("atomic asset upsert failed", zap.String("trace_id", traceID), zap.Error(err))
			writeAssetCommandError(w, http.StatusBadRequest, traceID, "asset_upsert_failed", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": result,
		"meta": map[string]any{
			"contract_version":  "1",
			"snapshot_id":       "asset-revision-" + strconv.FormatInt(result.Revision, 10),
			"as_of":             time.Now().UTC().Format(time.RFC3339Nano),
			"trace_id":          result.TraceID,
			"partial":           false,
			"missing_sections":  []string{},
			"source_watermarks": map[string]any{"postgresql.outbox.id": result.OutboxID},
		},
		"error": nil,
	})
}

func writeAssetCommandError(w http.ResponseWriter, status int, traceID, code, message string) {
	now := time.Now().UTC()
	writeJSON(w, status, map[string]any{
		"data": nil,
		"meta": map[string]any{
			"contract_version":  1,
			"snapshot_id":       "asset-command-error-" + traceID,
			"as_of":             now.Format(time.RFC3339Nano),
			"trace_id":          traceID,
			"partial":           false,
			"missing_sections":  []string{},
			"source_watermarks": map[string]any{},
		},
		"error": map[string]any{"code": code, "message": message, "trace_id": traceID},
	})
}

func (h *HTTPHandler) listAssets(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	traceID := strings.TrimSpace(r.Header.Get("X-Trace-ID"))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	query := r.URL.Query()
	limit, limitPresent, err := strictIntQuery(query, "limit", 50, 1, 200)
	if err != nil {
		writeAssetListError(w, http.StatusBadRequest, traceID, "invalid_limit", err.Error())
		return
	}
	offset, offsetPresent, err := strictIntQuery(query, "offset", 0, 0, -1)
	if err != nil {
		writeAssetListError(w, http.StatusBadRequest, traceID, "invalid_offset", err.Error())
		return
	}
	cursorValues, cursorPresent := query["cursor"]
	if cursorPresent && (len(cursorValues) != 1 || strings.TrimSpace(cursorValues[0]) == "") {
		writeAssetListError(w, http.StatusBadRequest, traceID, "invalid_cursor", "cursor must be a single non-empty value")
		return
	}
	cursorToken := ""
	if cursorPresent {
		cursorToken = strings.TrimSpace(cursorValues[0])
	}
	if cursorPresent && offsetPresent {
		writeAssetListError(w, http.StatusBadRequest, traceID, "pagination_conflict", "cursor and offset cannot be used together")
		return
	}

	assetType := strings.TrimSpace(query.Get("asset_type"))
	if assetType != "" && !service.IsAssetType(assetType) {
		writeAssetListError(w, http.StatusBadRequest, traceID, "invalid_asset_type", "invalid asset_type")
		return
	}
	filter := config.AssetListFilter{
		AssetType:  assetType,
		Status:     strings.TrimSpace(query.Get("status")),
		Search:     strings.TrimSpace(query.Get("search")),
		Department: strings.TrimSpace(query.Get("department")),
		Campus:     strings.TrimSpace(query.Get("campus")),
	}

	// Offset remains an explicitly selected compatibility path. Requests that
	// omit offset use the canonical signed cursor contract.
	if (offsetPresent && !cursorPresent) || (!h.cursorEnabled && !cursorPresent) {
		h.listAssetsOffset(w, r, identity.TenantID, traceID, filter, limit, offset)
		return
	}
	if !h.cursorEnabled {
		writeAssetListError(w, http.StatusServiceUnavailable, traceID, "cursor_unavailable", "asset cursor rollout is not enabled")
		return
	}
	h.listAssetsCursor(w, r, identity.TenantID, traceID, filter, limit, limitPresent, cursorToken)
}

func (h *HTTPHandler) listAssetsOffset(
	w http.ResponseWriter,
	r *http.Request,
	tenantID string,
	traceID string,
	filter config.AssetListFilter,
	limit int,
	offset int,
) {
	assets, total, err := h.svc.ListAssetsFiltered(r.Context(), tenantID, filter, limit, offset)
	if err != nil {
		h.logger.Warn("list assets offset failed", zap.String("trace_id", traceID), zap.Error(err))
		writeAssetListError(w, http.StatusInternalServerError, traceID, "asset_list_failed", "asset list query failed")
		return
	}
	asOf := time.Now().UTC()
	pagination := map[string]any{
		"mode":     "offset",
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": offset+len(assets) < total,
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":       assets,
		"pagination": pagination,
		"meta": map[string]any{
			"contract_version":  "1",
			"snapshot_id":       assetSnapshotID(tenantID, assetCursorFilterSHA256(filter), "offset", asOf),
			"as_of":             asOf.Format(time.RFC3339Nano),
			"trace_id":          traceID,
			"partial":           false,
			"missing_sections":  []string{},
			"source_watermarks": map[string]any{"postgresql.assets.updated_at": asOf.Format(time.RFC3339Nano)},
			"pagination":        pagination,
			"deprecation": map[string]any{
				"offset":      true,
				"sunset":      "not-before-G7",
				"replacement": "cursor",
			},
		},
		"error": nil,
	})
}

func (h *HTTPHandler) listAssetsCursor(
	w http.ResponseWriter,
	r *http.Request,
	tenantID string,
	traceID string,
	filter config.AssetListFilter,
	limit int,
	limitPresent bool,
	cursorToken string,
) {
	codec, err := newAssetCursorCodec(h.jwtSigningKey)
	if err != nil {
		h.logger.Error("asset cursor codec unavailable", zap.String("trace_id", traceID), zap.Error(err))
		writeAssetListError(w, http.StatusInternalServerError, traceID, "cursor_unavailable", "asset cursor service is unavailable")
		return
	}
	var position *config.AssetCursorPosition
	if cursorToken != "" {
		var explicitLimit *int
		if limitPresent {
			explicitLimit = &limit
		}
		claims, decodeErr := codec.decode(cursorToken, tenantID, filter, explicitLimit)
		if decodeErr != nil {
			code := "invalid_cursor"
			message := "cursor is invalid for this tenant, filter, sort or page size"
			if errors.Is(decodeErr, errAssetCursorExpired) {
				code = "cursor_expired"
				message = "cursor has expired; restart from the first page"
			}
			writeAssetListError(w, http.StatusBadRequest, traceID, code, message)
			return
		}
		limit = claims.Limit
		position = &config.AssetCursorPosition{
			SnapshotAt:   time.UnixMicro(claims.SnapshotUnixMicro).UTC(),
			SnapshotXIDs: claims.SnapshotXIDs,
			LastSeen:     time.UnixMicro(claims.LastSeenUnixMicro).UTC(),
			LastAssetID:  claims.LastAssetID,
			Total:        claims.Total,
		}
	}
	page, err := h.svc.ListAssetsCursor(r.Context(), tenantID, filter, limit, position)
	if err != nil {
		h.logger.Warn("list assets cursor failed", zap.String("trace_id", traceID), zap.Error(err))
		writeAssetListError(w, http.StatusInternalServerError, traceID, "asset_list_failed", "asset cursor query failed")
		return
	}
	nextCursor := ""
	if page.HasMore {
		nextCursor, err = codec.encode(tenantID, filter, limit, page)
		if err != nil {
			h.logger.Error("encode next asset cursor failed", zap.String("trace_id", traceID), zap.Error(err))
			writeAssetListError(w, http.StatusInternalServerError, traceID, "cursor_unavailable", "next asset cursor could not be created")
			return
		}
	}
	asOf := page.SnapshotAt.UTC()
	pagination := map[string]any{
		"mode":        "cursor",
		"total":       page.Total,
		"limit":       limit,
		"has_more":    page.HasMore,
		"next_cursor": nextCursor,
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":       page.Assets,
		"pagination": pagination,
		"meta": map[string]any{
			"contract_version": "1",
			"snapshot_id": assetSnapshotID(
				tenantID,
				assetCursorFilterSHA256(filter),
				page.SnapshotXIDs,
				asOf,
			),
			"as_of":             asOf.Format(time.RFC3339Nano),
			"trace_id":          traceID,
			"partial":           false,
			"missing_sections":  []string{},
			"source_watermarks": map[string]any{"postgresql.assets.updated_at": asOf.Format(time.RFC3339Nano)},
			"pagination":        pagination,
		},
		"error": nil,
	})
}

func writeAssetListError(w http.ResponseWriter, status int, traceID, code, message string) {
	writeJSON(w, status, map[string]any{
		"data": nil,
		"meta": map[string]any{
			"contract_version":  "1",
			"trace_id":          traceID,
			"partial":           false,
			"missing_sections":  []string{},
			"source_watermarks": map[string]any{},
		},
		"error": map[string]any{"code": code, "message": message},
	})
}

func strictIntQuery(values map[string][]string, key string, fallback, min, max int) (int, bool, error) {
	rawValues, present := values[key]
	if !present {
		return fallback, false, nil
	}
	if len(rawValues) != 1 || strings.TrimSpace(rawValues[0]) == "" {
		return 0, true, fmt.Errorf("%s must be a single integer", key)
	}
	value, err := strconv.Atoi(strings.TrimSpace(rawValues[0]))
	if err != nil || value < min || (max >= 0 && value > max) {
		if max >= 0 {
			return 0, true, fmt.Errorf("%s must be between %d and %d", key, min, max)
		}
		return 0, true, fmt.Errorf("%s must be at least %d", key, min)
	}
	return value, true, nil
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

func (h *HTTPHandler) getAssetDetailSnapshot(w http.ResponseWriter, r *http.Request, assetID string) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	if assetID == "" {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_PARAMETER", "asset id required")
		return
	}
	snapshot, err := h.svc.GetAssetDetailSnapshot(
		r.Context(), identity.TenantID, assetID, intQuery(r, "history_limit", 50),
	)
	if err != nil {
		h.logger.Warn("get asset detail snapshot failed", zap.String("asset_id", assetID), zap.Error(err))
		httpx.JSONError(w, r.Context(), http.StatusNotFound, "ASSET_NOT_FOUND", "asset detail snapshot not found")
		return
	}
	traceID := strings.TrimSpace(r.Header.Get("X-Trace-ID"))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	httpx.JSONContractSuccess(w, r.Context(), snapshot, httpx.ContractMeta{
		ContractVersion:  snapshot.ContractVersion,
		SnapshotID:       snapshot.SnapshotID,
		AsOf:             snapshot.AsOf.Format(time.RFC3339Nano),
		TraceID:          traceID,
		Partial:          snapshot.Partial,
		MissingSections:  snapshot.MissingSections,
		SourceWatermarks: snapshot.SourceWatermarks,
	})
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
	traceID := strings.TrimSpace(r.Header.Get("X-Trace-ID"))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = traceID
	}
	created, err := h.svc.RegisterDiscoveryCredential(r.Context(), &credential, config.DiscoveryResourceCommand{
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		Actor:          auditActor(identity),
		TraceID:        traceID,
		RequestID:      requestID,
		ClientIP:       clientIP(r),
		UserAgent:      r.UserAgent(),
	})
	if err != nil {
		h.logger.Warn("register discovery credential failed", zap.Error(err))
		switch {
		case errors.Is(err, repository.ErrDiscoveryResourceIdempotencyConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "idempotency_conflict", err.Error())
		case errors.Is(err, repository.ErrDiscoveryResourceRevisionConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "revision_conflict", err.Error())
		default:
			writeAssetCommandError(w, http.StatusBadRequest, traceID, "credential_command_rejected", err.Error())
		}
		return
	}
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
	traceID := strings.TrimSpace(r.Header.Get("X-Trace-ID"))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = traceID
	}
	command := config.DiscoveryJobCommand{
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		Actor:          auditActor(identity),
		TraceID:        traceID,
		RequestID:      requestID,
		ClientIP:       clientIP(r),
		UserAgent:      r.UserAgent(),
	}
	if h.discoveryJobsV2Enabled {
		run, err := h.svc.SubmitActiveDiscovery(r.Context(), &req, command)
		if err != nil {
			h.logger.Warn("submit active discovery failed", zap.Error(err))
			switch {
			case errors.Is(err, repository.ErrDiscoveryIdempotencyConflict):
				writeAssetCommandError(w, http.StatusConflict, traceID, "idempotency_conflict", err.Error())
			case errors.Is(err, repository.ErrDiscoveryOverlapConflict):
				writeAssetCommandError(w, http.StatusConflict, traceID, "overlapping_discovery", err.Error())
			default:
				writeAssetCommandError(w, http.StatusBadRequest, traceID, "discovery_job_rejected", err.Error())
			}
			return
		}
		writeDiscoveryEnvelope(w, http.StatusAccepted, traceID, run, map[string]any{
			"job_id":            run.RunID,
			"state":             run.Status,
			"idempotent_replay": run.IdempotentReplay,
		})
		return
	}
	result, err := h.svc.RunActiveDiscovery(r.Context(), &req, command)
	if err != nil {
		h.logger.Warn("run active discovery failed", zap.Error(err))
		switch {
		case errors.Is(err, repository.ErrDiscoveryIdempotencyConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "idempotency_conflict", err.Error())
		case errors.Is(err, repository.ErrDiscoveryStateConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "discovery_run_in_progress", err.Error())
		default:
			writeAssetCommandError(w, http.StatusBadRequest, traceID, "discovery_run_rejected", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
}

func (h *HTTPHandler) discoveryRunResource(w http.ResponseWriter, r *http.Request, suffix string) {
	if !h.discoveryJobsV2Enabled {
		writeError(w, http.StatusNotFound, "discovery job API is not enabled")
		return
	}
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "discovery run_id is required")
		return
	}
	runID := parts[0]
	if _, err := uuid.Parse(runID); err != nil {
		writeError(w, http.StatusBadRequest, "discovery run_id must be a UUID")
		return
	}
	resource := ""
	if len(parts) > 1 {
		resource = parts[1]
	}
	if len(parts) > 2 && !(len(parts) == 4 && resource == "candidates" && parts[3] == "merge") {
		writeError(w, http.StatusNotFound, "unknown discovery job resource")
		return
	}
	if len(parts) == 4 {
		candidateID := parts[2]
		if _, err := uuid.Parse(candidateID); err != nil {
			writeError(w, http.StatusBadRequest, "discovery candidate_id must be a UUID")
			return
		}
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.mergeDiscoveryCandidate(w, r, runID, candidateID)
		return
	}
	switch resource {
	case "":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.getDiscoveryJob(w, r, runID)
	case "cancel":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.cancelDiscoveryJob(w, r, runID)
	case "candidates":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.listDiscoveryCandidates(w, r, runID)
	case "history":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.listDiscoveryJobHistory(w, r, runID)
	default:
		writeError(w, http.StatusNotFound, "unknown discovery job resource")
	}
}

func (h *HTTPHandler) mergeDiscoveryCandidate(
	w http.ResponseWriter,
	r *http.Request,
	runID, candidateID string,
) {
	identity, ok := h.requireAssetDiscoveryWrite(w, r)
	if !ok {
		return
	}
	traceID := strings.TrimSpace(r.Header.Get("X-Trace-ID"))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = traceID
	}
	var command config.DiscoveryCandidateMergeCommand
	if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
		writeAssetCommandError(w, http.StatusBadRequest, traceID, "invalid_request", "invalid candidate merge payload")
		return
	}
	command.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	command.Actor = auditActor(identity)
	command.TraceID = traceID
	command.RequestID = requestID
	command.ClientIP = clientIP(r)
	command.UserAgent = r.UserAgent()
	result, err := h.svc.MergeDiscoveryCandidate(
		r.Context(), identity.TenantID, runID, candidateID, command,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeAssetCommandError(w, http.StatusNotFound, traceID, "not_found", "discovery candidate not found")
		case errors.Is(err, repository.ErrDiscoveryIdempotencyConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "idempotency_conflict", err.Error())
		case errors.Is(err, repository.ErrDiscoveryRevisionConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "candidate_revision_conflict", err.Error())
		case errors.Is(err, repository.ErrAssetRevisionConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "asset_revision_conflict", err.Error())
		case errors.Is(err, repository.ErrDiscoveryStateConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "candidate_state_conflict", err.Error())
		default:
			writeAssetCommandError(w, http.StatusBadRequest, traceID, "candidate_merge_rejected", err.Error())
		}
		return
	}
	writeDiscoveryEnvelope(w, http.StatusOK, traceID, result, map[string]any{
		"job_id":              runID,
		"candidate_id":        candidateID,
		"state":               result.Candidate.Status,
		"asset_id":            result.AssetID,
		"asset_revision":      result.AssetRevision,
		"idempotent_replay":   result.IdempotentReplay,
		"authoritative_write": true,
	})
}

func (h *HTTPHandler) getDiscoveryJob(w http.ResponseWriter, r *http.Request, runID string) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	run, err := h.svc.GetDiscoveryJob(r.Context(), identity.TenantID, runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "discovery job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeDiscoveryEnvelope(w, http.StatusOK, run.TraceID, run, nil)
}

func (h *HTTPHandler) cancelDiscoveryJob(w http.ResponseWriter, r *http.Request, runID string) {
	identity, ok := h.requireAssetDiscoveryWrite(w, r)
	if !ok {
		return
	}
	traceID := strings.TrimSpace(r.Header.Get("X-Trace-ID"))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	var payload struct {
		ExpectedRevision int64  `json:"expected_revision"`
		Reason           string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeAssetCommandError(w, http.StatusBadRequest, traceID, "invalid_request", "invalid cancellation payload")
		return
	}
	run, err := h.svc.CancelDiscoveryJob(
		r.Context(), identity.TenantID, runID, payload.Reason,
		config.DiscoveryJobCommand{
			IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
			Actor:          auditActor(identity),
			TraceID:        traceID,
			RequestID:      strings.TrimSpace(r.Header.Get("X-Request-ID")),
			ClientIP:       clientIP(r),
			UserAgent:      r.UserAgent(),
		},
		payload.ExpectedRevision,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeAssetCommandError(w, http.StatusNotFound, traceID, "not_found", "discovery job not found")
		case errors.Is(err, repository.ErrDiscoveryRevisionConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "revision_conflict", err.Error())
		case errors.Is(err, repository.ErrDiscoveryStateConflict):
			writeAssetCommandError(w, http.StatusConflict, traceID, "state_conflict", err.Error())
		default:
			writeAssetCommandError(w, http.StatusBadRequest, traceID, "cancel_rejected", err.Error())
		}
		return
	}
	writeDiscoveryEnvelope(w, http.StatusAccepted, traceID, run, map[string]any{
		"job_id":            run.RunID,
		"state":             run.Status,
		"idempotent_replay": run.IdempotentReplay,
	})
}

func (h *HTTPHandler) listDiscoveryCandidates(w http.ResponseWriter, r *http.Request, runID string) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.GetDiscoveryJob(r.Context(), identity.TenantID, runID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "discovery job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	candidates, err := h.svc.ListDiscoveryCandidates(
		r.Context(), identity.TenantID, runID, intQuery(r, "limit", 50),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeDiscoveryEnvelope(w, http.StatusOK, traceIDFromRequest(r), candidates, map[string]any{
		"job_id": runID,
		"count":  len(candidates),
	})
}

func (h *HTTPHandler) listDiscoveryJobHistory(w http.ResponseWriter, r *http.Request, runID string) {
	identity, ok := h.requireAssetRead(w, r)
	if !ok {
		return
	}
	history, err := h.svc.ListDiscoveryJobHistory(r.Context(), identity.TenantID, runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeDiscoveryEnvelope(w, http.StatusOK, traceIDFromRequest(r), history, map[string]any{
		"job_id": runID,
		"count":  len(history),
	})
}

func writeDiscoveryEnvelope(w http.ResponseWriter, status int, traceID string, data any, extra map[string]any) {
	if traceID == "" {
		traceID = uuid.NewString()
	}
	now := time.Now().UTC()
	meta := map[string]any{
		"contract_version":  "1",
		"snapshot_id":       "asset-discovery-" + traceID,
		"as_of":             now.Format(time.RFC3339Nano),
		"trace_id":          traceID,
		"partial":           false,
		"missing_sections":  []string{},
		"source_watermarks": map[string]any{"postgresql.asset_discovery_runs": now.Format(time.RFC3339Nano)},
	}
	for key, value := range extra {
		meta[key] = value
	}
	writeJSON(w, status, map[string]any{"data": data, "meta": meta, "error": nil})
}

func traceIDFromRequest(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Trace-ID")); value != "" {
		return value
	}
	return uuid.NewString()
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
	if h.discoveryJobsV2Enabled {
		writeDiscoveryEnvelope(w, http.StatusOK, traceIDFromRequest(r), runs, map[string]any{
			"count": len(runs),
		})
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
	return httpx.GetTenantID(r.Context())
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

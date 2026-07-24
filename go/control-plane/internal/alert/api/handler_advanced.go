////////////////////////////////////////////////////////////////////////////////
// Advanced Alert Features — 高级告警功能集成 Handler
// 集成: Notification + Risk Scoring + Playbook + Data Quality
////////////////////////////////////////////////////////////////////////////////

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/notification"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/playbook"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/risk"
	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/dataquality"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

// AdvancedHandler 高级告警功能处理器
type AdvancedHandler struct {
	notifier     *notification.NotificationService
	scorer       *risk.AssetRiskScorer
	playbook     *playbook.PlaybookEngine
	dqMonitor    *dataquality.Monitor
	advancedRepo *AdvancedRepository
}

func NewAdvancedHandler(
	notifier *notification.NotificationService,
	scorer *risk.AssetRiskScorer,
	playbook *playbook.PlaybookEngine,
	dqMonitor *dataquality.Monitor,
	advancedRepo *AdvancedRepository,
) *AdvancedHandler {
	return &AdvancedHandler{
		notifier:     notifier,
		scorer:       scorer,
		playbook:     playbook,
		dqMonitor:    dqMonitor,
		advancedRepo: advancedRepo,
	}
}

// RegisterRoutes 注册高级功能路由
func (h *AdvancedHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()
	h.RegisterAPIRoutes(api)
}

func (h *AdvancedHandler) RegisterAPIRoutes(api *mux.Router) {
	// 资产风险评分
	riskRouter := api.PathPrefix("/risk").Subrouter()
	riskRouter.HandleFunc("/assets/{ip}", h.GetAssetRisk).Methods("GET")
	riskRouter.HandleFunc("/assets", h.GetRiskSummary).Methods("GET")

	// SOAR 剧本
	playbookRouter := api.PathPrefix("/playbooks").Subrouter()
	playbookRouter.HandleFunc("", h.ListPlaybooks).Methods("GET")
	playbookRouter.HandleFunc("", h.CreatePlaybookDraft).Methods("POST")
	playbookRouter.HandleFunc("/catalog", h.GetPlaybookCatalog).Methods("GET")
	playbookRouter.HandleFunc("/executions", h.GetPlaybookExecutions).Methods("GET")
	playbookRouter.HandleFunc("/audits", h.GetPlaybookAudits).Methods("GET")
	playbookRouter.HandleFunc("/evidence/export", h.ExportPlaybookEvidence).Methods("GET")
	playbookRouter.HandleFunc("/executions/{execution_id}/rollback", h.RollbackPlaybookDrill).Methods("POST")
	playbookRouter.HandleFunc("/{name}/workbench", h.GetPlaybookWorkbench).Methods("GET")
	playbookRouter.HandleFunc("/{name}/draft", h.SavePlaybookDraft).Methods("PUT")
	playbookRouter.HandleFunc("/{name}/submit-approval", h.SubmitPlaybookApproval).Methods("POST")
	playbookRouter.HandleFunc("/{name}/approve", h.ApprovePlaybook).Methods("POST")
	playbookRouter.HandleFunc("/{name}/reject", h.RejectPlaybook).Methods("POST")
	playbookRouter.HandleFunc("/{name}/drill", h.DrillPlaybook).Methods("POST")
	playbookRouter.HandleFunc("/{name}", h.PatchPlaybook).Methods("PATCH")
	playbookRouter.HandleFunc("/{name}/execute", h.ExecutePlaybook).Methods("POST")

	// 数据质量
	dqRouter := api.PathPrefix("/data-quality").Subrouter()
	dqRouter.HandleFunc("", h.GetDataQuality).Methods("GET")
	dqRouter.HandleFunc("/tables/{dataset}", h.GetDataQualityTable).Methods("GET")
	dqRouter.HandleFunc("/reports/daily", h.GetDataQualityDailyReport).Methods("GET")
	dqRouter.HandleFunc("/reports/daily/download", h.DownloadDataQualityDailyReport).Methods("GET")
	dqRouter.HandleFunc("/latency-chain", h.GetLatencyChain).Methods("GET")
	dqRouter.HandleFunc("/baseline", h.UpdateBaseline).Methods("POST")
	dqRouter.HandleFunc("/actions", h.CreateDataQualityAction).Methods("POST")

	// 通知配置与测试
	notificationRouter := api.PathPrefix("/notifications").Subrouter()
	notificationRouter.HandleFunc("/workbench", h.GetNotificationWorkbench).Methods("GET")
	notificationRouter.HandleFunc("/settings", h.GetNotificationSettings).Methods("GET")
	notificationRouter.HandleFunc("/settings", h.UpdateNotificationSettings).Methods("PUT")
	notificationRouter.HandleFunc("/test", h.TestNotification).Methods("POST")
	notificationRouter.HandleFunc("/subscriptions", h.ListNotificationRules).Methods("GET")
	notificationRouter.HandleFunc("/subscriptions", h.CreateNotificationRule).Methods("POST")
	notificationRouter.HandleFunc("/subscriptions/{id}", h.PatchNotificationRule).Methods("PATCH")
	notificationRouter.HandleFunc("/templates", h.ListNotificationTemplates).Methods("GET")
	notificationRouter.HandleFunc("/templates", h.CreateNotificationTemplate).Methods("POST")
	notificationRouter.HandleFunc("/templates/{id}", h.PatchNotificationTemplate).Methods("PATCH")
	notificationRouter.HandleFunc("/templates/{id}/test", h.TestNotificationTemplate).Methods("POST")
	notificationRouter.HandleFunc("/escalation-policies", h.ListNotificationEscalationPolicies).Methods("GET")
	notificationRouter.HandleFunc("/escalation-policies", h.CreateNotificationEscalationPolicy).Methods("POST")
	notificationRouter.HandleFunc("/escalation-policies/{id}", h.PatchNotificationEscalationPolicy).Methods("PATCH")
	notificationRouter.HandleFunc("/deliveries", h.ListNotificationDeliveries).Methods("GET")
	notificationRouter.HandleFunc("/deliveries/{id}/retry", h.RetryNotificationDelivery).Methods("POST")
	notificationRouter.HandleFunc("/silence-rules", h.ListNotificationSilenceRules).Methods("GET")
	notificationRouter.HandleFunc("/silence-rules", h.CreateNotificationSilenceRule).Methods("POST")
	notificationRouter.HandleFunc("/silence-rules/{id}", h.PatchNotificationSilenceRule).Methods("PATCH")
	api.HandleFunc("/notify/test", h.TestNotification).Methods("POST")
}

var dataQualityTableDatasets = map[string]struct{}{
	"consumerRows": {}, "messageSizeTopicRows": {}, "partitionQueueRows": {},
	"flinkJobRows": {}, "flinkWindowRows": {}, "flinkFailureRows": {},
	"fieldQualityRows": {}, "communityCheckRows": {}, "communityMismatchRows": {},
	"fieldAnomalyRows": {}, "fieldLineageRows": {}, "fieldRepairRows": {},
	"storageComponentRows": {}, "storageFailureRows": {}, "storageReplicaRows": {},
	"storagePartitionRows": {}, "storageObjectRows": {}, "replayTaskRows": {},
	"replayIdempotencyRows": {}, "replayDifferenceRows": {}, "replayEvidenceRows": {},
}

func (h *AdvancedHandler) GetDataQualityTable(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityReadPermission(w, r) {
		return
	}
	if h.advancedRepo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "data quality repository is not available"})
		return
	}

	dataset := mux.Vars(r)["dataset"]
	if _, ok := dataQualityTableDatasets[dataset]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "unsupported data quality table dataset"})
		return
	}
	page, pageSize, err := dataQualityPagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}

	result, ok, err := h.advancedRepo.GetDataQualityTablePage(r.Context(), tenantIDFromRequest(r), dataset, page, pageSize)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "DATA_QUALITY_TABLE_LOAD_FAILED", err.Error())
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "message": "active data quality table dataset was not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": result})
}

func dataQualityPagination(r *http.Request) (int, int, error) {
	page, pageSize := 1, 5
	if raw := r.URL.Query().Get("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return 0, 0, errors.New("page must be a positive integer")
		}
		page = value
	}
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			return 0, 0, errors.New("page_size must be between 1 and 100")
		}
		pageSize = value
	}
	return page, pageSize, nil
}

// =============================================================================
// Risk Handlers
// =============================================================================

func (h *AdvancedHandler) GetAssetRisk(w http.ResponseWriter, r *http.Request) {
	if h.scorer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "asset risk scorer is not available"})
		return
	}

	tenantID := tenantIDFromRequest(r)
	ip := mux.Vars(r)["ip"]

	score, err := h.scorer.ScoreAsset(r.Context(), tenantID, ip)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": score})
}

func (h *AdvancedHandler) GetRiskSummary(w http.ResponseWriter, r *http.Request) {
	if h.scorer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "asset risk scorer is not available"})
		return
	}

	tenantID := tenantIDFromRequest(r)
	summary, err := h.scorer.GetRiskSummary(r.Context(), tenantID, 10)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": summary})
}

// =============================================================================
// Playbook Handlers
// =============================================================================

func (h *AdvancedHandler) ListPlaybooks(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookReadPermission(w, r) {
		return
	}
	catalog, err := h.tenantPlaybookCatalog(r.Context(), tenantIDFromRequest(r))
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_CATALOG_UNAVAILABLE", err.Error())
		return
	}
	names := make([]string, 0, len(catalog))
	for _, definition := range catalog {
		names = append(names, definition.Name)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"playbooks": names,
			"catalog":   catalog,
		},
	})
}

func (h *AdvancedHandler) GetPlaybookCatalog(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookReadPermission(w, r) {
		return
	}
	catalog, err := h.tenantPlaybookCatalog(r.Context(), tenantIDFromRequest(r))
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_CATALOG_UNAVAILABLE", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"playbooks": catalog,
			"total":     len(catalog),
		},
	})
}

type playbookPatchRequest struct {
	Enabled         *bool  `json:"enabled"`
	MaxRuns         *int   `json:"max_runs"`
	CooldownSeconds *int64 `json:"cooldown_seconds"`
	ExpectedVersion *int   `json:"expected_version"`
}

func (h *AdvancedHandler) PatchPlaybook(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookWritePermission(w, r) {
		return
	}

	name := mux.Vars(r)["name"]
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "playbook name is required"})
		return
	}

	defer r.Body.Close()
	var patch playbookPatchRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if patch.MaxRuns != nil && *patch.MaxRuns < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "max_runs must be >= 0"})
		return
	}

	var cooldown *time.Duration
	if patch.CooldownSeconds != nil {
		if *patch.CooldownSeconds < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "cooldown_seconds must be >= 0"})
			return
		}
		duration := time.Duration(*patch.CooldownSeconds) * time.Second
		cooldown = &duration
	}

	if h.advancedRepo == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_REPOSITORY_UNAVAILABLE", "playbook repository is not available")
		return
	}
	tenantID := tenantIDFromRequest(r)
	current, ok, err := h.advancedRepo.GetPlaybookDefinition(r.Context(), tenantID, name)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_LOAD_FAILED", err.Error())
		return
	}
	if !ok {
		httpx.JSONError(w, r.Context(), http.StatusNotFound, "PLAYBOOK_NOT_FOUND", "playbook not found")
		return
	}
	if patch.ExpectedVersion == nil || *patch.ExpectedVersion != current.Version {
		httpx.JSONError(w, r.Context(), http.StatusConflict, "PLAYBOOK_VERSION_CONFLICT", "expected_version must match the current definition")
		return
	}
	actor := playbookActor(r.Context())
	if patch.Enabled != nil && patch.MaxRuns == nil && cooldown == nil {
		action := "disable"
		if *patch.Enabled {
			action = "enable"
		}
		updated, err := h.advancedRepo.TransitionPlaybook(r.Context(), r, tenantID, name, action, actor, "", current.Version)
		if err != nil {
			httpx.JSONError(w, r.Context(), http.StatusConflict, "PLAYBOOK_TRANSITION_REJECTED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": updated})
		return
	}
	definition, err := DecodePlaybookDefinition(*current)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_DEFINITION_INVALID", err.Error())
		return
	}
	if patch.MaxRuns != nil {
		definition.MaxRuns = *patch.MaxRuns
	}
	if cooldown != nil {
		definition.Cooldown = *cooldown
	}
	definition.Enabled = false
	payload, _ := json.Marshal(definition)
	draft, err := h.advancedRepo.SavePlaybookDraft(r.Context(), r, PlaybookDefinitionRecord{
		TenantID: tenantID, Name: current.Name, DisplayName: current.DisplayName,
		Description: current.Description, RiskLevel: playbookRiskLevel(definition),
		Definition: decodeJSONMap(payload), CreatedBy: actor,
	}, current.Version)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusConflict, "PLAYBOOK_DRAFT_SAVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": draft})
}

func (h *AdvancedHandler) GetPlaybookExecutions(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookReadPermission(w, r) {
		return
	}
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "limit must be a positive integer"})
			return
		}
		limit = parsed
	}

	if h.advancedRepo == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"executions": []PlaybookExecutionRecord{},
				"total":      0,
			},
		})
		return
	}

	records, err := h.advancedRepo.ListPlaybookExecutions(r.Context(), tenantIDFromRequest(r), limit)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"executions": records,
			"total":      len(records),
		},
	})
}

type playbookDraftRequest struct {
	ExpectedVersion int               `json:"expected_version"`
	DisplayName     string            `json:"display_name"`
	Description     string            `json:"description"`
	Definition      playbook.Playbook `json:"definition"`
}

type playbookTransitionRequest struct {
	ExpectedVersion int    `json:"expected_version"`
	Reason          string `json:"reason"`
}

type playbookDrillRequest struct {
	ExpectedVersion int                    `json:"expected_version"`
	AlertContext    *playbook.AlertContext `json:"alert_context"`
}

type playbookRollbackRequest struct {
	Reason string `json:"reason"`
}

func (h *AdvancedHandler) CreatePlaybookDraft(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookWritePermission(w, r) {
		return
	}
	var payload playbookDraftRequest
	if !decodePlaybookJSON(w, r, &payload) {
		return
	}
	h.savePlaybookDraft(w, r, strings.TrimSpace(payload.Definition.Name), payload)
}

func (h *AdvancedHandler) SavePlaybookDraft(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookWritePermission(w, r) {
		return
	}
	var payload playbookDraftRequest
	if !decodePlaybookJSON(w, r, &payload) {
		return
	}
	h.savePlaybookDraft(w, r, strings.TrimSpace(mux.Vars(r)["name"]), payload)
}

func (h *AdvancedHandler) savePlaybookDraft(w http.ResponseWriter, r *http.Request, name string, payload playbookDraftRequest) {
	if h.advancedRepo == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_REPOSITORY_UNAVAILABLE", "playbook repository is not available")
		return
	}
	if err := validatePlaybookName(name); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "PLAYBOOK_NAME_INVALID", err.Error())
		return
	}
	payload.Definition.Name = name
	payload.Definition.Description = strings.TrimSpace(payload.Description)
	payload.Definition.Enabled = false
	payload.Definition.Trigger.TenantID = ""
	if err := normalizeAndValidatePlaybookDefinition(&payload.Definition); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "PLAYBOOK_DEFINITION_INVALID", err.Error())
		return
	}
	displayName := strings.TrimSpace(payload.DisplayName)
	if len([]rune(displayName)) < 2 || len([]rune(displayName)) > 80 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "PLAYBOOK_DISPLAY_NAME_INVALID", "display_name must contain 2 to 80 characters")
		return
	}
	definitionJSON, _ := json.Marshal(payload.Definition)
	created, err := h.advancedRepo.SavePlaybookDraft(r.Context(), r, PlaybookDefinitionRecord{
		TenantID: tenantIDFromRequest(r), Name: name, DisplayName: displayName,
		Description: payload.Definition.Description, RiskLevel: playbookRiskLevel(&payload.Definition),
		Definition: decodeJSONMap(definitionJSON), CreatedBy: playbookActor(r.Context()),
	}, payload.ExpectedVersion)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusConflict, "PLAYBOOK_DRAFT_SAVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": created})
}

func (h *AdvancedHandler) SubmitPlaybookApproval(w http.ResponseWriter, r *http.Request) {
	h.transitionPlaybook(w, r, "submit", false)
}

func (h *AdvancedHandler) ApprovePlaybook(w http.ResponseWriter, r *http.Request) {
	h.transitionPlaybook(w, r, "approve", true)
}

func (h *AdvancedHandler) RejectPlaybook(w http.ResponseWriter, r *http.Request) {
	h.transitionPlaybook(w, r, "reject", true)
}

func (h *AdvancedHandler) transitionPlaybook(w http.ResponseWriter, r *http.Request, action string, requireApprover bool) {
	if requireApprover {
		if !h.requirePlaybookApprovePermission(w, r) {
			return
		}
	} else if !h.requirePlaybookWritePermission(w, r) {
		return
	}
	if h.advancedRepo == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_REPOSITORY_UNAVAILABLE", "playbook repository is not available")
		return
	}
	var payload playbookTransitionRequest
	if !decodePlaybookJSON(w, r, &payload) {
		return
	}
	updated, err := h.advancedRepo.TransitionPlaybook(
		r.Context(), r, tenantIDFromRequest(r), strings.TrimSpace(mux.Vars(r)["name"]),
		action, playbookActor(r.Context()), payload.Reason, payload.ExpectedVersion,
	)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusConflict, "PLAYBOOK_TRANSITION_REJECTED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": updated})
}

func (h *AdvancedHandler) DrillPlaybook(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookDrillPermission(w, r) {
		return
	}
	if h.advancedRepo == nil || h.playbook == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_SERVICE_UNAVAILABLE", "playbook service is not available")
		return
	}
	var payload playbookDrillRequest
	if !decodePlaybookJSON(w, r, &payload) {
		return
	}
	tenantID := tenantIDFromRequest(r)
	name := strings.TrimSpace(mux.Vars(r)["name"])
	record, ok, err := h.advancedRepo.GetPlaybookDefinition(r.Context(), tenantID, name)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_LOAD_FAILED", err.Error())
		return
	}
	if !ok {
		httpx.JSONError(w, r.Context(), http.StatusNotFound, "PLAYBOOK_NOT_FOUND", "playbook not found")
		return
	}
	if payload.ExpectedVersion != record.Version {
		httpx.JSONError(w, r.Context(), http.StatusConflict, "PLAYBOOK_VERSION_CONFLICT", "expected_version does not match the current definition")
		return
	}
	definition, err := DecodePlaybookDefinition(*record)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_DEFINITION_INVALID", err.Error())
		return
	}
	alertContext := payload.AlertContext
	if alertContext == nil {
		alertContext = defaultPlaybookAlertContext(name, tenantID)
	}
	alertContext.TenantID = tenantID
	if strings.TrimSpace(alertContext.AlertID) == "" {
		alertContext.AlertID = "drill-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	result, err := h.playbook.Drill(r.Context(), definition, alertContext)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "PLAYBOOK_DRILL_FAILED", err.Error())
		return
	}
	execution, err := h.advancedRepo.SavePlaybookExecutionWithMetadata(
		r.Context(), tenantID, playbookActor(r.Context()), alertContext, result,
	)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_DRILL_PERSIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": execution})
}

func (h *AdvancedHandler) RollbackPlaybookDrill(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookDrillPermission(w, r) {
		return
	}
	if h.advancedRepo == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_REPOSITORY_UNAVAILABLE", "playbook repository is not available")
		return
	}
	var payload playbookRollbackRequest
	if !decodePlaybookJSON(w, r, &payload) {
		return
	}
	record, err := h.advancedRepo.RollbackPlaybookDrill(
		r.Context(), r, tenantIDFromRequest(r), strings.TrimSpace(mux.Vars(r)["execution_id"]),
		playbookActor(r.Context()), payload.Reason,
	)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusConflict, "PLAYBOOK_ROLLBACK_REJECTED", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": record})
}

func (h *AdvancedHandler) GetPlaybookAudits(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookReadPermission(w, r) {
		return
	}
	if h.advancedRepo == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_REPOSITORY_UNAVAILABLE", "playbook repository is not available")
		return
	}
	records, err := h.advancedRepo.ListPlaybookAudits(r.Context(), tenantIDFromRequest(r), strings.TrimSpace(r.URL.Query().Get("object_id")), 100)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_AUDIT_LOAD_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]interface{}{"audits": records, "total": len(records)}})
}

func (h *AdvancedHandler) GetPlaybookWorkbench(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookReadPermission(w, r) {
		return
	}
	if h.advancedRepo == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_REPOSITORY_UNAVAILABLE", "playbook repository is not available")
		return
	}
	tenantID := tenantIDFromRequest(r)
	name := strings.TrimSpace(mux.Vars(r)["name"])
	definition, ok, err := h.advancedRepo.GetPlaybookDefinition(r.Context(), tenantID, name)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_LOAD_FAILED", err.Error())
		return
	}
	if !ok {
		httpx.JSONError(w, r.Context(), http.StatusNotFound, "PLAYBOOK_NOT_FOUND", "playbook not found")
		return
	}
	executions, err := h.advancedRepo.ListPlaybookExecutionsByName(r.Context(), tenantID, name, 100)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_EXECUTION_LOAD_FAILED", err.Error())
		return
	}
	audits, err := h.advancedRepo.ListPlaybookAuditsForPlaybook(r.Context(), tenantID, name, 100)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_AUDIT_LOAD_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]interface{}{
		"definition": definition, "executions": executions, "audits": audits,
	}})
}

func (h *AdvancedHandler) ExportPlaybookEvidence(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookExportPermission(w, r) {
		return
	}
	if h.advancedRepo == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_REPOSITORY_UNAVAILABLE", "playbook repository is not available")
		return
	}
	tenantID := tenantIDFromRequest(r)
	definitions, err := h.tenantPlaybookCatalog(r.Context(), tenantID)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_EXPORT_FAILED", err.Error())
		return
	}
	executions, err := h.advancedRepo.ListAllPlaybookExecutions(r.Context(), tenantID)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_EXPORT_FAILED", err.Error())
		return
	}
	audits, err := h.advancedRepo.ListAllPlaybookAudits(r.Context(), tenantID)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PLAYBOOK_EXPORT_FAILED", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="playbook-evidence.json"`)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"generated_at": time.Now().UTC(), "tenant_id": tenantID,
		"definitions": definitions, "executions": executions, "audits": audits,
		"counts":   map[string]int{"definitions": len(definitions), "executions": len(executions), "audits": len(audits)},
		"complete": true,
	})
}

func (h *AdvancedHandler) ExecutePlaybook(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookWritePermission(w, r) {
		return
	}
	// The built-in executor only renders intended action messages; it has no
	// external network, endpoint, capture, or notification provider. Reject the
	// legacy live route explicitly so it cannot bypass the durable tenant-owned
	// approval state or create evidence that claims an effect was applied.
	httpx.JSONError(w, r.Context(), http.StatusNotImplemented, "PLAYBOOK_LIVE_EXECUTION_NOT_CONFIGURED", "live playbook execution is unavailable until a verified external provider is configured; use /drill for simulated validation")
}

// =============================================================================
// Data Quality Handlers
// =============================================================================

func (h *AdvancedHandler) GetDataQuality(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityReadPermission(w, r) {
		return
	}
	if h.dqMonitor == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "data quality monitor is not available"})
		return
	}

	tenantID := tenantIDFromRequest(r)
	report, err := h.dqMonitor.CheckAll(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	data := map[string]interface{}{
		"timestamp": report.Timestamp,
		"tenant_id": report.TenantID,
		"overall":   report.Overall,
		"checks":    report.Checks,
		"metrics":   report.Metrics,
		"data_source": map[string]interface{}{
			"monitor": "clickhouse-live",
			"visuals": "unconfigured",
		},
	}
	if h.advancedRepo != nil {
		fixture, ok, fixtureErr := h.advancedRepo.GetDataQualityUIFixture(r.Context(), tenantID)
		if fixtureErr != nil {
			httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "UI_FIXTURE_LOAD_FAILED", fixtureErr.Error())
			return
		}
		if ok {
			data["visuals"] = map[string]interface{}{"dataQuality": fixture.Payload}
			data["data_source"] = map[string]interface{}{
				"monitor":         "clickhouse-live",
				"visuals":         "postgres-activated-fixture",
				"fixture_version": fixture.FixtureVersion,
				"updated_at":      fixture.UpdatedAt,
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": data})
}

func (h *AdvancedHandler) GetLatencyChain(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityReadPermission(w, r) {
		return
	}
	if h.dqMonitor == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "data quality monitor is not available"})
		return
	}

	lookback := 24 * time.Hour
	if raw := r.URL.Query().Get("lookback_minutes"); raw != "" {
		minutes, err := strconv.Atoi(raw)
		if err != nil || minutes <= 0 || minutes > 7*24*60 {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "lookback_minutes must be between 1 and 10080"})
			return
		}
		lookback = time.Duration(minutes) * time.Minute
	}

	report, err := h.dqMonitor.CheckLatencyChain(r.Context(), tenantIDFromRequest(r), lookback)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": report})
}

func (h *AdvancedHandler) UpdateBaseline(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityWritePermission(w, r) {
		return
	}
	if h.dqMonitor == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "data quality monitor is not available"})
		return
	}

	if err := h.dqMonitor.UpdateBaseline(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "baseline updated"})
}

type dataQualityActionRequest struct {
	View       string                 `json:"view"`
	Action     string                 `json:"action"`
	Target     string                 `json:"target"`
	DryRun     *bool                  `json:"dry_run"`
	Confirmed  bool                   `json:"confirmed"`
	Reason     string                 `json:"reason"`
	Parameters map[string]interface{} `json:"parameters"`
}

var dataQualityViews = map[string]struct{}{
	"overview": {}, "topic-health": {}, "flink-quality": {}, "field-quality": {},
	"storage-quality": {}, "replay-reconcile": {}, "report": {}, "settings": {},
}

func (h *AdvancedHandler) CreateDataQualityAction(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityWritePermission(w, r) {
		return
	}
	ctx := r.Context()
	var payload dataQualityActionRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid data quality action payload")
		return
	}
	payload.View = strings.TrimSpace(payload.View)
	payload.Action = strings.TrimSpace(payload.Action)
	payload.Target = strings.TrimSpace(payload.Target)
	payload.Reason = strings.TrimSpace(payload.Reason)
	if _, ok := dataQualityViews[payload.View]; !ok {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_VIEW", "view must be a supported data quality view")
		return
	}
	if payload.Action == "" || len(payload.Action) > 120 || payload.Target == "" || len(payload.Target) > 240 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ACTION", "action and target are required")
		return
	}
	dryRun := true
	if payload.DryRun != nil {
		dryRun = *payload.DryRun
	}
	if !dryRun && (!payload.Confirmed || len([]rune(payload.Reason)) < 8) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "CONFIRMATION_REQUIRED", "non-dry-run actions require confirmation and a reason of at least 8 characters")
		return
	}
	if h.advancedRepo == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "data quality action repository is not available")
		return
	}
	record, err := h.advancedRepo.CreateDataQualityAction(ctx, r, DataQualityActionRecord{
		TenantID: tenantIDFromRequest(r), View: payload.View, Action: payload.Action,
		Target: payload.Target, DryRun: dryRun, Status: map[bool]string{true: "dry_run", false: "queued"}[dryRun],
		RequestedBy: httpx.GetUserID(ctx),
		Request: map[string]interface{}{
			"reason": payload.Reason, "confirmed": payload.Confirmed, "parameters": payload.Parameters,
		},
	})
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "ACTION_PERSIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{"success": true, "data": record})
}

func (h *AdvancedHandler) requireDataQualityReadPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopeDataQualityRead) || hasSystemPermission(ctx, authmodel.ScopeDataQualityWrite) || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: data-quality:read required")
	return false
}

func (h *AdvancedHandler) requireDataQualityWritePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopeDataQualityWrite) || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: data-quality:write required")
	return false
}

// =============================================================================
// Notification Test
// =============================================================================

func (h *AdvancedHandler) TestNotification(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	if h.notifier == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "notification service is not available"})
		return
	}

	var request struct {
		Channel   string `json:"channel"`
		Target    string `json:"target"`
		AlertType string `json:"alert_type"`
	}
	if r.Body != nil {
		decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		if err := decoder.Decode(&request); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
			return
		}
	}
	request.Channel = strings.ToLower(strings.TrimSpace(request.Channel))
	if request.Channel == "" {
		request.Channel = "email"
	}
	if _, ok := notificationChannelNames[request.Channel]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "unsupported notification channel"})
		return
	}
	if strings.TrimSpace(request.Target) == "" {
		request.Target = "安全值班组"
	}
	if strings.TrimSpace(request.AlertType) == "" {
		request.AlertType = "scan"
	}

	testAlert := &notification.AlertInfo{
		AlertID:     "test-" + uuid.NewString(),
		Title:       "Test Alert — Notification Service Verification",
		Severity:    "high",
		Score:       0.95,
		SourceIP:    "192.168.1.100",
		DestIP:      "10.0.0.1",
		AlertType:   request.AlertType,
		Description: "This is a test alert to verify notification channels",
		TenantID:    tenantIDFromRequest(r),
	}

	settings := defaultNotificationSettings()
	if h.advancedRepo != nil {
		if saved, ok, err := h.advancedRepo.GetNotificationSettings(r.Context(), tenantIDFromRequest(r)); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
			return
		} else if ok {
			settings = mergeSettings(settings, saved)
		}
	}
	if !notificationChannelEnabled(settings, request.Channel) {
		writeJSON(w, http.StatusConflict, map[string]interface{}{"success": false, "message": "notification channel is disabled"})
		return
	}
	dispatchErr := h.notifier.SendChannel(r.Context(), request.Channel, testAlert)
	status := "sent"
	errorMessage := ""
	if dispatchErr != nil {
		status = "failed"
		errorMessage = dispatchErr.Error()
	}
	var delivery *NotificationDeliveryRecord
	if h.advancedRepo != nil {
		created, err := h.advancedRepo.CreateNotificationDelivery(r.Context(), NotificationDeliveryRecord{
			TenantID: tenantIDFromRequest(r), AlertID: testAlert.AlertID, TargetName: request.Target,
			Channel: request.Channel, AlertType: request.AlertType, Status: status, ErrorMessage: errorMessage, TraceID: "trace-" + uuid.NewString(),
		})
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
			return
		}
		delivery = created
	}
	action := "NOTIFICATION_TEST_SENT"
	if dispatchErr != nil {
		action = "NOTIFICATION_TEST_FAILED"
	}
	if err := h.recordNotificationAudit(r, action, "notification_test", testAlert.AlertID, map[string]interface{}{
		"severity":   testAlert.Severity,
		"alert_type": testAlert.AlertType,
		"channel":    request.Channel,
		"target":     request.Target,
		"status":     status,
		"error":      errorMessage,
	}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if dispatchErr != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": dispatchErr.Error(), "data": delivery})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "test notification sent", "data": delivery})
}

func (h *AdvancedHandler) GetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	settings := defaultNotificationSettings()
	if h.advancedRepo != nil {
		saved, ok, err := h.advancedRepo.GetNotificationSettings(r.Context(), tenantIDFromRequest(r))
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
			return
		}
		if ok {
			settings = mergeSettings(settings, saved)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": settings})
}

func (h *AdvancedHandler) UpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	defer r.Body.Close()
	var payload map[string]interface{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if err := rejectInlineSecrets(payload, ""); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}

	base := defaultNotificationSettings()
	if h.advancedRepo != nil {
		if saved, ok, err := h.advancedRepo.GetNotificationSettings(r.Context(), tenantIDFromRequest(r)); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
			return
		} else if ok {
			base = mergeSettings(base, saved)
		}
	}
	settings := mergeSettings(base, payload)
	severity, ok := settings["min_severity"].(string)
	severity = strings.ToLower(strings.TrimSpace(severity))
	if _, valid := map[string]struct{}{"low": {}, "medium": {}, "high": {}, "critical": {}}[severity]; !ok || !valid {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "min_severity must be one of low, medium, high, critical"})
		return
	}
	settings["min_severity"] = severity
	if h.advancedRepo != nil {
		if err := h.advancedRepo.SaveNotificationSettings(r.Context(), tenantIDFromRequest(r), settings); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
			return
		}
	}
	if err := h.recordNotificationAudit(r, "NOTIFICATION_SETTINGS_UPDATED", "notification_settings", tenantIDFromRequest(r), notificationSettingsAuditDetail(settings)); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": settings})
}

type notificationSilenceRuleRequest struct {
	Name            string    `json:"name"`
	Scope           string    `json:"scope"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
	AffectedTargets []string  `json:"affected_targets"`
	Policy          string    `json:"policy"`
	Reason          string    `json:"reason"`
	Enabled         *bool     `json:"enabled"`
}

type notificationSilencePatchRequest struct {
	Name            *string    `json:"name"`
	Scope           *string    `json:"scope"`
	StartsAt        *time.Time `json:"starts_at"`
	EndsAt          *time.Time `json:"ends_at"`
	AffectedTargets *[]string  `json:"affected_targets"`
	Policy          *string    `json:"policy"`
	Reason          *string    `json:"reason"`
	Enabled         *bool      `json:"enabled"`
}

func (h *AdvancedHandler) ListNotificationSilenceRules(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "limit must be a positive integer"})
			return
		}
		limit = parsed
	}
	if h.advancedRepo == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"rules": []NotificationSilenceRule{},
				"total": 0,
			},
		})
		return
	}
	rules, err := h.advancedRepo.ListNotificationSilenceRules(r.Context(), tenantIDFromRequest(r), limit)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"rules": rules,
			"total": len(rules),
		},
	})
}

func (h *AdvancedHandler) CreateNotificationSilenceRule(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}

	defer r.Body.Close()
	var payload notificationSilenceRuleRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}

	rule, err := payload.toRule(tenantIDFromRequest(r), httpx.GetUserID(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if h.advancedRepo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "advanced repository is not available"})
		return
	}
	created, err := h.advancedRepo.CreateNotificationSilenceRule(r.Context(), rule)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if created == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "advanced repository is not available"})
		return
	}
	if err := h.recordNotificationAudit(r, "NOTIFICATION_SILENCE_RULE_CREATED", "notification_silence_rule", created.RuleID, map[string]interface{}{
		"name":             created.Name,
		"scope":            created.Scope,
		"starts_at":        created.StartsAt,
		"ends_at":          created.EndsAt,
		"affected_targets": created.AffectedTargets,
		"policy":           created.Policy,
		"reason":           created.Reason,
		"enabled":          created.Enabled,
	}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": created})
}

func (h *AdvancedHandler) PatchNotificationSilenceRule(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	if h.advancedRepo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "advanced repository is not available"})
		return
	}
	ruleID := strings.TrimSpace(mux.Vars(r)["id"])
	if ruleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "rule id is required"})
		return
	}

	defer r.Body.Close()
	var payload notificationSilencePatchRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if payload.Name == nil && payload.Scope == nil && payload.StartsAt == nil && payload.EndsAt == nil && payload.AffectedTargets == nil && payload.Policy == nil && payload.Reason == nil && payload.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "at least one silence rule field is required"})
		return
	}
	if payload.Name != nil && strings.TrimSpace(*payload.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "name cannot be empty"})
		return
	}
	if payload.StartsAt != nil && payload.EndsAt != nil && !payload.EndsAt.After(*payload.StartsAt) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "ends_at must be after starts_at"})
		return
	}
	updated, ok, err := h.advancedRepo.PatchNotificationSilenceRule(r.Context(), tenantIDFromRequest(r), ruleID, payload)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if !ok || updated == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "message": "notification silence rule not found"})
		return
	}
	if err := h.recordNotificationAudit(r, "NOTIFICATION_SILENCE_RULE_UPDATED", "notification_silence_rule", updated.RuleID, map[string]interface{}{
		"name":    updated.Name,
		"enabled": updated.Enabled,
		"policy":  updated.Policy,
	}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": updated})
}

// =============================================================================
// Helper
// =============================================================================

func (h *AdvancedHandler) tenantPlaybookCatalog(ctx context.Context, tenantID string) ([]PlaybookDefinitionRecord, error) {
	if h == nil || h.advancedRepo == nil {
		return nil, errors.New("playbook repository is not available")
	}
	defaults := playbook.DefaultPlaybooks()
	for _, definition := range defaults {
		if definition == nil {
			continue
		}
		if playbookRiskLevel(definition) == "critical" || playbookRiskLevel(definition) == "high" {
			definition.ApprovalPolicy = playbook.ApprovalPolicy{Required: true, MinimumRole: "安全运营组（L2）", TwoPersonRule: true}
			definition.RollbackPolicy = playbook.RollbackPolicy{Supported: true, Automatic: true}
		}
	}
	if err := h.advancedRepo.EnsurePlaybookDefinitions(ctx, tenantID, defaults); err != nil {
		return nil, err
	}
	return h.advancedRepo.ListPlaybookDefinitions(ctx, tenantID)
}

func decodePlaybookJSON(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return false
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "request body must contain exactly one JSON object")
		return false
	}
	return true
}

func validatePlaybookName(name string) error {
	if len(name) < 3 || len(name) > 64 {
		return errors.New("playbook name must contain 3 to 64 lowercase characters")
	}
	for index, value := range name {
		if (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9') || (value == '-' && index > 0 && index < len(name)-1) {
			continue
		}
		return errors.New("playbook name must use lowercase letters, digits and internal hyphens only")
	}
	return nil
}

func normalizeAndValidatePlaybookDefinition(definition *playbook.Playbook) error {
	if definition == nil {
		return errors.New("playbook definition is required")
	}
	if err := validatePlaybookName(definition.Name); err != nil {
		return err
	}
	if len(definition.Actions) == 0 || len(definition.Actions) > 20 {
		return errors.New("playbook must contain between 1 and 20 actions")
	}
	if len(definition.Conditions) > 20 {
		return errors.New("playbook supports at most 20 conditions")
	}
	for _, condition := range definition.Conditions {
		field := strings.ToLower(strings.TrimSpace(condition.Field))
		operator := strings.ToLower(strings.TrimSpace(condition.Operator))
		value := strings.ToLower(strings.TrimSpace(condition.Value))
		if field != "alert_count" && field != "asset_risk" {
			return errors.New("playbook contains an unsupported condition field")
		}
		if operator != "gt" && operator != "gte" && operator != "eq" && operator != "lt" && operator != "lte" {
			return errors.New("playbook contains an unsupported condition operator")
		}
		if field == "alert_count" {
			if parsed, err := strconv.Atoi(value); err != nil || parsed < 0 {
				return errors.New("alert_count condition value must be a non-negative integer")
			}
		} else if value != "low" && value != "medium" && value != "high" && value != "critical" {
			return errors.New("asset_risk condition value must be low, medium, high or critical")
		}
	}
	allowedActions := map[string]struct{}{
		"block_ip": {}, "block_domain": {}, "quarantine": {}, "capture_pcap": {},
		"rate_limit": {}, "tag": {}, "enrich": {}, "escalate": {}, "notify": {},
	}
	for index := range definition.Actions {
		action := &definition.Actions[index]
		action.Type = strings.TrimSpace(action.Type)
		if _, ok := allowedActions[action.Type]; !ok {
			return errors.New("playbook contains an unsupported action type")
		}
		if action.Timeout <= 0 {
			action.Timeout = 30 * time.Second
		}
		if action.Timeout > 10*time.Minute {
			return errors.New("playbook action timeout cannot exceed 10 minutes")
		}
	}
	riskLevel := playbookRiskLevel(definition)
	if riskLevel == "critical" || riskLevel == "high" {
		definition.ApprovalPolicy.Required = true
		definition.ApprovalPolicy.TwoPersonRule = true
		if strings.TrimSpace(definition.ApprovalPolicy.MinimumRole) == "" {
			definition.ApprovalPolicy.MinimumRole = "安全运营组（L2）"
		}
		definition.RollbackPolicy.Supported = true
	}
	return nil
}

func playbookActor(ctx context.Context) string {
	if value := strings.TrimSpace(httpx.GetUserID(ctx)); value != "" {
		return value
	}
	if value := strings.TrimSpace(httpx.GetUsername(ctx)); value != "" {
		return value
	}
	return "authenticated-user"
}

func (h *AdvancedHandler) requirePlaybookReadPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopePlaybookRead) || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: playbook:read required")
	return false
}

func (h *AdvancedHandler) requirePlaybookWritePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopePlaybookWrite) || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: playbook:write required")
	return false
}

func (h *AdvancedHandler) requirePlaybookDrillPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopePlaybookDrill) || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: playbook:drill required")
	return false
}

func (h *AdvancedHandler) requirePlaybookApprovePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopePlaybookApprove) || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: playbook:approve required for independent approval")
	return false
}

func (h *AdvancedHandler) requirePlaybookExportPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopePlaybookExport) || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: playbook:export required")
	return false
}

func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

func tenantIDFromRequest(r *http.Request) string {
	if tenantID := httpx.GetTenantID(r.Context()); tenantID != "" {
		return tenantID
	}
	if tenantID := r.Header.Get("X-Tenant-ID"); tenantID != "" {
		return tenantID
	}
	if tenantID := r.URL.Query().Get("tenant_id"); tenantID != "" {
		return tenantID
	}
	return "default"
}

func defaultPlaybookAlertContext(name, tenantID string) *playbook.AlertContext {
	alertTypeByName := map[string]string{
		"block-scanner":        "scan",
		"quarantine-c2":        "c2",
		"throttle-brute-force": "brute_force",
		"investigate-exfil":    "data_exfil",
		"log-lateral-movement": "lateral_movement",
		"dns-tunnel-block":     "dns_tunnel",
	}
	alertType := alertTypeByName[name]
	if alertType == "" {
		alertType = "scan"
	}
	return &playbook.AlertContext{
		AlertID:           "manual-" + name,
		AlertType:         alertType,
		Severity:          "critical",
		Score:             0.95,
		SourceIP:          "192.168.1.100",
		DestIP:            "10.0.0.1",
		TenantID:          tenantID,
		RelatedAlertCount: 10,
		AssetRisk:         "high",
	}
}

func defaultNotificationSettings() map[string]interface{} {
	return map[string]interface{}{
		"enabled":            true,
		"min_severity":       "high",
		"rate_limit_per_min": 10,
		"channels": map[string]interface{}{
			"email":    false,
			"slack":    false,
			"webhook":  false,
			"wechat":   false,
			"dingtalk": false,
			"feishu":   false,
		},
		"secret_ref": "",
	}
}

func notificationChannelEnabled(settings map[string]interface{}, channel string) bool {
	channels, ok := settings["channels"].(map[string]interface{})
	if !ok {
		return false
	}
	enabled, ok := channels[strings.ToLower(strings.TrimSpace(channel))].(bool)
	return ok && enabled
}

func mergeSettings(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(base)+len(override))
	for key, value := range base {
		if nested, ok := value.(map[string]interface{}); ok {
			result[key] = mergeSettings(nested, map[string]interface{}{})
		} else {
			result[key] = value
		}
	}
	for key, value := range override {
		if existing, ok := result[key].(map[string]interface{}); ok {
			if nested, ok := value.(map[string]interface{}); ok {
				result[key] = mergeSettings(existing, nested)
				continue
			}
		}
		result[key] = value
	}
	return result
}

func rejectInlineSecrets(value interface{}, path string) error {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			normalized := strings.ToLower(key)
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if isInlineSecretKey(normalized) && child != nil && child != "" {
				return errors.New("sensitive notification value must be stored as a secret reference: " + childPath)
			}
			if err := rejectInlineSecrets(child, childPath); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, child := range typed {
			if err := rejectInlineSecrets(child, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func isInlineSecretKey(key string) bool {
	if strings.Contains(key, "secret_ref") || strings.Contains(key, "secretref") {
		return false
	}
	for _, token := range []string{"password", "token", "secret", "auth_header", "api_key", "access_key"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func (h *AdvancedHandler) requireNotificationAdminPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasAdvancedAdminPermission(ctx) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: admin:* required")
	return false
}

func hasAdvancedAdminPermission(ctx context.Context) bool {
	if claims := httpx.GetExtendedClaims(ctx); claims != nil {
		return claims.HasRole("admin") || claims.HasRole("super_admin") || claims.HasPermission(authmodel.ScopeAdminAll)
	}
	if httpx.HasRole(ctx, "admin") || httpx.HasRole(ctx, "super_admin") {
		return true
	}
	for _, granted := range httpx.GetPermissions(ctx) {
		if permissionMatches(granted, authmodel.ScopeAdminAll) {
			return true
		}
	}
	return false
}

func (h *AdvancedHandler) recordNotificationAudit(r *http.Request, action, objectType, objectID string, detail map[string]interface{}) error {
	if h == nil || h.advancedRepo == nil {
		return nil
	}
	ctx := r.Context()
	err := h.advancedRepo.RecordAuditLog(ctx, r, tenantIDFromRequest(r), httpx.GetUserID(ctx), action, objectType, objectID, detail)
	if err != nil && h.advancedRepo.logger != nil {
		// Every notification governance table has an AFTER trigger that writes a
		// minimal audit row in the same PostgreSQL transaction. This richer
		// request-context audit is supplementary and must not turn a committed
		// business mutation into a false 503 response.
		h.advancedRepo.logger.Warn("Failed to append enriched notification audit; atomic database audit remains authoritative", zap.Error(err), zap.String("action", action), zap.String("object_id", objectID))
	}
	return nil
}

func (req notificationSilenceRuleRequest) toRule(tenantID, userID string) (NotificationSilenceRule, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return NotificationSilenceRule{}, errors.New("name is required")
	}
	if req.StartsAt.IsZero() {
		return NotificationSilenceRule{}, errors.New("starts_at is required")
	}
	if req.EndsAt.IsZero() {
		return NotificationSilenceRule{}, errors.New("ends_at is required")
	}
	if !req.EndsAt.After(req.StartsAt) {
		return NotificationSilenceRule{}, errors.New("ends_at must be after starts_at")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	targets := make([]string, 0, len(req.AffectedTargets))
	for _, target := range req.AffectedTargets {
		target = strings.TrimSpace(target)
		if target != "" {
			targets = append(targets, target)
		}
	}
	return NotificationSilenceRule{
		TenantID:        tenantID,
		Name:            name,
		Scope:           nonEmpty(strings.TrimSpace(req.Scope), "all"),
		StartsAt:        req.StartsAt,
		EndsAt:          req.EndsAt,
		AffectedTargets: targets,
		Policy:          nonEmpty(strings.TrimSpace(req.Policy), "all"),
		Reason:          strings.TrimSpace(req.Reason),
		Enabled:         enabled,
		CreatedBy:       userID,
	}, nil
}

func notificationSettingsAuditDetail(settings map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"enabled":            settings["enabled"],
		"min_severity":       settings["min_severity"],
		"rate_limit_per_min": settings["rate_limit_per_min"],
		"channels":           settings["channels"],
		"secret_ref":         settings["secret_ref"],
	}
}

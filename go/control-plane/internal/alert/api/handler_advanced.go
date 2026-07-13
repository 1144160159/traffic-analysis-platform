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

	"github.com/gorilla/mux"

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
	playbookRouter.HandleFunc("/catalog", h.GetPlaybookCatalog).Methods("GET")
	playbookRouter.HandleFunc("/executions", h.GetPlaybookExecutions).Methods("GET")
	playbookRouter.HandleFunc("/{name}", h.PatchPlaybook).Methods("PATCH")
	playbookRouter.HandleFunc("/{name}/execute", h.ExecutePlaybook).Methods("POST")

	// 数据质量
	dqRouter := api.PathPrefix("/data-quality").Subrouter()
	dqRouter.HandleFunc("", h.GetDataQuality).Methods("GET")
	dqRouter.HandleFunc("/latency-chain", h.GetLatencyChain).Methods("GET")
	dqRouter.HandleFunc("/baseline", h.UpdateBaseline).Methods("POST")

	// 通知配置与测试
	notificationRouter := api.PathPrefix("/notifications").Subrouter()
	notificationRouter.HandleFunc("/settings", h.GetNotificationSettings).Methods("GET")
	notificationRouter.HandleFunc("/settings", h.UpdateNotificationSettings).Methods("PUT")
	notificationRouter.HandleFunc("/test", h.TestNotification).Methods("POST")
	notificationRouter.HandleFunc("/silence-rules", h.ListNotificationSilenceRules).Methods("GET")
	notificationRouter.HandleFunc("/silence-rules", h.CreateNotificationSilenceRule).Methods("POST")
	notificationRouter.HandleFunc("/silence-rules/{id}", h.PatchNotificationSilenceRule).Methods("PATCH")
	api.HandleFunc("/notify/test", h.TestNotification).Methods("POST")
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
	if h.playbook == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"playbooks": []string{},
				"catalog":   []playbook.Playbook{},
			},
		})
		return
	}

	catalog := h.playbook.ListPlaybooks()
	names := make([]string, 0, len(catalog))
	for _, pb := range catalog {
		names = append(names, pb.Name)
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
	if h.playbook == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "playbook engine is not available"})
		return
	}

	catalog := h.playbook.ListPlaybooks()
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
}

func (h *AdvancedHandler) PatchPlaybook(w http.ResponseWriter, r *http.Request) {
	if h.playbook == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "playbook engine is not available"})
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

	updated, err := h.playbook.UpdatePlaybook(name, patch.Enabled, patch.MaxRuns, cooldown)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if h.advancedRepo != nil {
		if err := h.advancedRepo.SavePlaybookOverride(r.Context(), tenantIDFromRequest(r), updated); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": updated})
}

func (h *AdvancedHandler) GetPlaybookExecutions(w http.ResponseWriter, r *http.Request) {
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

func (h *AdvancedHandler) ExecutePlaybook(w http.ResponseWriter, r *http.Request) {
	if h.playbook == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "playbook engine is not available"})
		return
	}

	name := mux.Vars(r)["name"]
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "playbook name is required"})
		return
	}

	tenantID := tenantIDFromRequest(r)
	alertCtx := defaultPlaybookAlertContext(name, tenantID)
	if r.Body != nil {
		defer r.Body.Close()
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(alertCtx); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
			return
		}
		if alertCtx.TenantID == "" {
			alertCtx.TenantID = tenantID
		}
	}

	result, err := h.playbook.ExecuteByName(r.Context(), name, alertCtx)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	var execution *PlaybookExecutionRecord
	if h.advancedRepo != nil {
		execution, err = h.advancedRepo.SavePlaybookExecution(r.Context(), tenantID, alertCtx, result)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": result, "execution": execution})
}

// =============================================================================
// Data Quality Handlers
// =============================================================================

func (h *AdvancedHandler) GetDataQuality(w http.ResponseWriter, r *http.Request) {
	if h.dqMonitor == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "data quality monitor is not available"})
		return
	}

	report, err := h.dqMonitor.CheckAll(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": report})
}

func (h *AdvancedHandler) GetLatencyChain(w http.ResponseWriter, r *http.Request) {
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

	testAlert := &notification.AlertInfo{
		AlertID:     "test-001",
		Title:       "Test Alert — Notification Service Verification",
		Severity:    "high",
		Score:       0.95,
		SourceIP:    "192.168.1.100",
		DestIP:      "10.0.0.1",
		AlertType:   "scan",
		Description: "This is a test alert to verify notification channels",
		TenantID:    tenantIDFromRequest(r),
	}

	if err := h.notifier.Notify(r.Context(), testAlert); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if err := h.recordNotificationAudit(r, "NOTIFICATION_TEST_SENT", "notification_test", "test-alert", map[string]interface{}{
		"severity":   testAlert.Severity,
		"alert_type": testAlert.AlertType,
	}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "test notification sent"})
}

func (h *AdvancedHandler) GetNotificationSettings(w http.ResponseWriter, r *http.Request) {
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

	settings := mergeSettings(defaultNotificationSettings(), payload)
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
	Enabled *bool `json:"enabled"`
}

func (h *AdvancedHandler) ListNotificationSilenceRules(w http.ResponseWriter, r *http.Request) {
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
	if payload.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "enabled is required"})
		return
	}
	updated, ok, err := h.advancedRepo.SetNotificationSilenceRuleEnabled(r.Context(), tenantIDFromRequest(r), ruleID, *payload.Enabled)
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
	return h.advancedRepo.RecordAuditLog(ctx, r, tenantIDFromRequest(r), httpx.GetUserID(ctx), action, objectType, objectID, detail)
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

package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/notification"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type NotificationRuleRecord struct {
	RuleID     string                 `json:"rule_id"`
	TenantID   string                 `json:"tenant_id"`
	Name       string                 `json:"name"`
	Conditions map[string]interface{} `json:"conditions"`
	Channels   []string               `json:"channels"`
	Enabled    bool                   `json:"enabled"`
	CreatedBy  string                 `json:"created_by"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

type NotificationTemplateRecord struct {
	TemplateID      string                 `json:"template_id"`
	TenantID        string                 `json:"tenant_id"`
	TemplateType    string                 `json:"template_type"`
	Name            string                 `json:"name"`
	Version         int                    `json:"version"`
	Subject         string                 `json:"subject"`
	Body            string                 `json:"body"`
	VariableSchema  map[string]interface{} `json:"variable_schema"`
	ValidationState string                 `json:"validation_status"`
	Enabled         bool                   `json:"enabled"`
	CreatedBy       string                 `json:"created_by"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

type NotificationEscalationPolicyRecord struct {
	PolicyID  string                   `json:"policy_id"`
	TenantID  string                   `json:"tenant_id"`
	Name      string                   `json:"name"`
	Stages    []map[string]interface{} `json:"stages"`
	Enabled   bool                     `json:"enabled"`
	CreatedBy string                   `json:"created_by"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

type NotificationDeliveryRecord struct {
	NotificationID int64      `json:"notification_id"`
	TenantID       string     `json:"tenant_id"`
	RuleID         string     `json:"rule_id,omitempty"`
	AlertID        string     `json:"alert_id"`
	TargetName     string     `json:"target_name"`
	Channel        string     `json:"channel"`
	AlertType      string     `json:"alert_type"`
	Status         string     `json:"status"`
	ErrorMessage   string     `json:"error_message,omitempty"`
	RetryCount     int        `json:"retry_count"`
	TraceID        string     `json:"trace_id"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type notificationRuleRequest struct {
	Name       string                 `json:"name"`
	Conditions map[string]interface{} `json:"conditions"`
	Channels   []string               `json:"channels"`
	Enabled    *bool                  `json:"enabled"`
}

type notificationTemplateRequest struct {
	TemplateType   string                 `json:"template_type"`
	Name           string                 `json:"name"`
	Subject        string                 `json:"subject"`
	Body           string                 `json:"body"`
	VariableSchema map[string]interface{} `json:"variable_schema"`
	Enabled        *bool                  `json:"enabled"`
}

type notificationEscalationRequest struct {
	Name    string                   `json:"name"`
	Stages  []map[string]interface{} `json:"stages"`
	Enabled *bool                    `json:"enabled"`
}

func (h *AdvancedHandler) GetNotificationWorkbench(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	if h.advancedRepo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "notification repository is not available"})
		return
	}
	limit, err := notificationLimit(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	tenantID := tenantIDFromRequest(r)
	settings := defaultNotificationSettings()
	if saved, ok, err := h.advancedRepo.GetNotificationSettings(r.Context(), tenantID); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	} else if ok {
		settings = mergeSettings(settings, saved)
	}
	rules, err := h.advancedRepo.ListNotificationRules(r.Context(), tenantID, limit)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "NOTIFICATION_RULES_LOAD_FAILED", err.Error())
		return
	}
	templates, err := h.advancedRepo.ListNotificationTemplates(r.Context(), tenantID, limit)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "NOTIFICATION_TEMPLATES_LOAD_FAILED", err.Error())
		return
	}
	policies, err := h.advancedRepo.ListNotificationEscalationPolicies(r.Context(), tenantID, limit)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "NOTIFICATION_ESCALATION_LOAD_FAILED", err.Error())
		return
	}
	deliveries, err := h.advancedRepo.ListNotificationDeliveries(r.Context(), tenantID, limit)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "NOTIFICATION_DELIVERIES_LOAD_FAILED", err.Error())
		return
	}
	silences, err := h.advancedRepo.ListNotificationSilenceRules(r.Context(), tenantID, limit)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "NOTIFICATION_SILENCES_LOAD_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]interface{}{
		"settings": settings, "rules": rules, "templates": templates,
		"escalation_policies": policies, "deliveries": deliveries, "silence_rules": silences,
	}})
}

func (h *AdvancedHandler) ListNotificationRules(w http.ResponseWriter, r *http.Request) {
	h.listNotificationCollection(w, r, "rules")
}

func (h *AdvancedHandler) ListNotificationTemplates(w http.ResponseWriter, r *http.Request) {
	h.listNotificationCollection(w, r, "templates")
}

func (h *AdvancedHandler) ListNotificationEscalationPolicies(w http.ResponseWriter, r *http.Request) {
	h.listNotificationCollection(w, r, "escalations")
}

func (h *AdvancedHandler) ListNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	h.listNotificationCollection(w, r, "deliveries")
}

func (h *AdvancedHandler) listNotificationCollection(w http.ResponseWriter, r *http.Request, collection string) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	if h.advancedRepo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "notification repository is not available"})
		return
	}
	limit, err := notificationLimit(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	tenantID := tenantIDFromRequest(r)
	var value interface{}
	switch collection {
	case "rules":
		value, err = h.advancedRepo.ListNotificationRules(r.Context(), tenantID, limit)
	case "templates":
		value, err = h.advancedRepo.ListNotificationTemplates(r.Context(), tenantID, limit)
	case "escalations":
		value, err = h.advancedRepo.ListNotificationEscalationPolicies(r.Context(), tenantID, limit)
	case "deliveries":
		value, err = h.advancedRepo.ListNotificationDeliveries(r.Context(), tenantID, limit)
	default:
		err = errors.New("unsupported notification collection")
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]interface{}{"items": value}})
}

func (h *AdvancedHandler) CreateNotificationRule(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	var req notificationRuleRequest
	if err := decodeNotificationJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if err := validateNotificationRuleRequest(req, true); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if !h.requireNotificationRepository(w) {
		return
	}
	record, err := h.advancedRepo.CreateNotificationRule(r.Context(), tenantIDFromRequest(r), httpx.GetUserID(r.Context()), req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if err := h.recordNotificationAudit(r, "NOTIFICATION_RULE_CREATED", "notification_rule", record.RuleID, map[string]interface{}{"name": record.Name, "channels": record.Channels, "enabled": record.Enabled}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": record})
}

func (h *AdvancedHandler) PatchNotificationRule(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	if !h.requireNotificationRepository(w) {
		return
	}
	var req notificationRuleRequest
	if err := decodeNotificationJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if err := validateNotificationRuleRequest(req, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	record, ok, err := h.advancedRepo.PatchNotificationRule(r.Context(), tenantIDFromRequest(r), mux.Vars(r)["id"], req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "message": "notification rule not found"})
		return
	}
	if err := h.recordNotificationAudit(r, "NOTIFICATION_RULE_UPDATED", "notification_rule", record.RuleID, map[string]interface{}{"name": record.Name, "channels": record.Channels, "enabled": record.Enabled}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": record})
}

func (h *AdvancedHandler) CreateNotificationTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	var req notificationTemplateRequest
	if err := decodeNotificationJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if err := validateNotificationTemplateRequest(req, true); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if !h.requireNotificationRepository(w) {
		return
	}
	record, err := h.advancedRepo.CreateNotificationTemplate(r.Context(), tenantIDFromRequest(r), httpx.GetUserID(r.Context()), req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if err := h.recordNotificationAudit(r, "NOTIFICATION_TEMPLATE_CREATED", "notification_template", record.TemplateID, map[string]interface{}{"name": record.Name, "version": record.Version}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": record})
}

func (h *AdvancedHandler) PatchNotificationTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	if !h.requireNotificationRepository(w) {
		return
	}
	var req notificationTemplateRequest
	if err := decodeNotificationJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if err := validateNotificationTemplateRequest(req, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	record, ok, err := h.advancedRepo.PatchNotificationTemplate(r.Context(), tenantIDFromRequest(r), mux.Vars(r)["id"], req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "message": "notification template not found"})
		return
	}
	if err := h.recordNotificationAudit(r, "NOTIFICATION_TEMPLATE_UPDATED", "notification_template", record.TemplateID, map[string]interface{}{"name": record.Name, "version": record.Version, "enabled": record.Enabled}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": record})
}

func (h *AdvancedHandler) TestNotificationTemplate(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	if h.notifier == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "notification service is not available"})
		return
	}
	if !h.requireNotificationRepository(w) {
		return
	}
	record, ok, err := h.advancedRepo.GetNotificationTemplate(r.Context(), tenantIDFromRequest(r), mux.Vars(r)["id"])
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "message": "notification template not found"})
		return
	}
	testData := map[string]interface{}{
		"alert_id":   "template-test-" + record.TemplateID,
		"title":      "通知模板验收告警",
		"severity":   "high",
		"score":      0.95,
		"source_ip":  "192.168.1.100",
		"dest_ip":    "10.0.0.1",
		"alert_type": record.TemplateType,
	}
	for _, name := range notificationTemplateRequiredVariables(record.VariableSchema) {
		if _, exists := testData[name]; !exists {
			testData[name] = "test-" + name
		}
	}
	renderedSubject, renderedBody, renderErr := renderNotificationTemplate(record, testData)
	channel, channelErr := h.firstEnabledImplementedChannel(r.Context(), tenantIDFromRequest(r))
	dispatchErr := renderErr
	if dispatchErr == nil {
		dispatchErr = channelErr
	}
	if dispatchErr == nil {
		dispatchErr = h.notifier.SendChannel(r.Context(), channel, &notification.AlertInfo{
			AlertID: testData["alert_id"].(string), Title: renderedSubject, Severity: "high", Score: 0.95,
			SourceIP: testData["source_ip"].(string), DestIP: testData["dest_ip"].(string),
			AlertType: record.TemplateType, Description: renderedBody, TenantID: tenantIDFromRequest(r), Timestamp: time.Now(),
		})
	}
	status, errorMessage := "sent", ""
	if dispatchErr != nil {
		status, errorMessage = "failed", dispatchErr.Error()
	}
	delivery, err := h.advancedRepo.CreateNotificationDelivery(r.Context(), NotificationDeliveryRecord{TenantID: tenantIDFromRequest(r), AlertID: testData["alert_id"].(string), TargetName: record.Name, Channel: channel, AlertType: record.TemplateType, Status: status, ErrorMessage: errorMessage, TraceID: "trace-" + uuid.NewString()})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	action := "NOTIFICATION_TEMPLATE_TESTED"
	if dispatchErr != nil {
		action = "NOTIFICATION_TEMPLATE_TEST_FAILED"
	}
	if err := h.recordNotificationAudit(r, action, "notification_template", record.TemplateID, map[string]interface{}{"notification_id": delivery.NotificationID, "version": record.Version, "channel": channel, "status": status, "rendered_subject": renderedSubject, "error": errorMessage}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	data := map[string]interface{}{"template": record, "delivery": delivery, "rendered_subject": renderedSubject, "rendered_body": renderedBody}
	if dispatchErr != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": dispatchErr.Error(), "data": data})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": data})
}

func (h *AdvancedHandler) CreateNotificationEscalationPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	var req notificationEscalationRequest
	if err := decodeNotificationJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if err := validateNotificationEscalationRequest(req, true); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if !h.requireNotificationRepository(w) {
		return
	}
	record, err := h.advancedRepo.CreateNotificationEscalationPolicy(r.Context(), tenantIDFromRequest(r), httpx.GetUserID(r.Context()), req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if err := h.recordNotificationAudit(r, "NOTIFICATION_ESCALATION_CREATED", "notification_escalation_policy", record.PolicyID, map[string]interface{}{"name": record.Name, "stages": len(record.Stages)}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": record})
}

func (h *AdvancedHandler) PatchNotificationEscalationPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	if !h.requireNotificationRepository(w) {
		return
	}
	var req notificationEscalationRequest
	if err := decodeNotificationJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if err := validateNotificationEscalationRequest(req, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	record, ok, err := h.advancedRepo.PatchNotificationEscalationPolicy(r.Context(), tenantIDFromRequest(r), mux.Vars(r)["id"], req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "message": "notification escalation policy not found"})
		return
	}
	if err := h.recordNotificationAudit(r, "NOTIFICATION_ESCALATION_UPDATED", "notification_escalation_policy", record.PolicyID, map[string]interface{}{"name": record.Name, "enabled": record.Enabled}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": record})
}

func (h *AdvancedHandler) RetryNotificationDelivery(w http.ResponseWriter, r *http.Request) {
	if !h.requireNotificationAdminPermission(w, r) {
		return
	}
	if h.notifier == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "notification service is not available"})
		return
	}
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil || id < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "notification id must be a positive integer"})
		return
	}
	if !h.requireNotificationRepository(w) {
		return
	}
	original, ok, err := h.advancedRepo.GetNotificationDelivery(r.Context(), tenantIDFromRequest(r), id)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "message": "notification delivery not found"})
		return
	}
	dispatchErr := h.notifier.SendChannel(r.Context(), original.Channel, &notification.AlertInfo{
		AlertID: original.AlertID, Title: "Retry notification " + original.AlertID, Severity: "high", Score: 0.95,
		AlertType: original.AlertType, Description: "Retry requested from notification governance", TenantID: original.TenantID, Timestamp: time.Now(),
	})
	status, errorMessage := "sent", ""
	if dispatchErr != nil {
		status, errorMessage = "failed", dispatchErr.Error()
	}
	record, ok, err := h.advancedRepo.CompleteNotificationDeliveryRetry(r.Context(), tenantIDFromRequest(r), id, status, errorMessage)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "message": "notification delivery not found"})
		return
	}
	action := "NOTIFICATION_DELIVERY_RETRIED"
	if dispatchErr != nil {
		action = "NOTIFICATION_DELIVERY_RETRY_FAILED"
	}
	if err := h.recordNotificationAudit(r, action, "notification_delivery", strconv.FormatInt(id, 10), map[string]interface{}{"retry_count": record.RetryCount, "status": record.Status, "trace_id": record.TraceID, "error": errorMessage}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	if dispatchErr != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": dispatchErr.Error(), "data": record})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": record})
}

func renderNotificationTemplate(record *NotificationTemplateRecord, values map[string]interface{}) (string, string, error) {
	if record == nil {
		return "", "", errors.New("notification template is required")
	}
	for _, name := range notificationTemplateRequiredVariables(record.VariableSchema) {
		if _, ok := values[name]; !ok {
			return "", "", fmt.Errorf("missing required template variable: %s", name)
		}
	}
	render := func(name, source string) (string, error) {
		functions := template.FuncMap{}
		for key, raw := range values {
			value := raw
			functions[key] = func() interface{} { return value }
		}
		parsed, err := template.New(name).Funcs(functions).Option("missingkey=error").Parse(source)
		if err != nil {
			return "", err
		}
		var output bytes.Buffer
		if err := parsed.Execute(&output, values); err != nil {
			return "", err
		}
		return output.String(), nil
	}
	subject, err := render("subject", record.Subject)
	if err != nil {
		return "", "", fmt.Errorf("render subject: %w", err)
	}
	body, err := render("body", record.Body)
	if err != nil {
		return "", "", fmt.Errorf("render body: %w", err)
	}
	return subject, body, nil
}

func notificationTemplateRequiredVariables(schema map[string]interface{}) []string {
	var required []interface{}
	switch raw := schema["required"].(type) {
	case []interface{}:
		required = raw
	case []string:
		for _, item := range raw {
			required = append(required, item)
		}
	}
	variables := make([]string, 0, len(required))
	for _, item := range required {
		if name := strings.TrimSpace(fmt.Sprint(item)); name != "" {
			variables = append(variables, name)
		}
	}
	return variables
}

func (h *AdvancedHandler) firstEnabledImplementedChannel(ctx context.Context, tenantID string) (string, error) {
	settings := defaultNotificationSettings()
	if h.advancedRepo != nil {
		if saved, ok, err := h.advancedRepo.GetNotificationSettings(ctx, tenantID); err != nil {
			return "", err
		} else if ok {
			settings = mergeSettings(settings, saved)
		}
	}
	for _, channel := range []string{"email", "webhook", "wechat", "dingtalk", "slack", "feishu"} {
		if notificationChannelEnabled(settings, channel) {
			return channel, nil
		}
	}
	return "", errors.New("no enabled implemented notification channel")
}

func notificationLimit(r *http.Request) (int, error) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			return 0, errors.New("limit must be between 1 and 200")
		}
		limit = parsed
	}
	return limit, nil
}

func (h *AdvancedHandler) requireNotificationRepository(w http.ResponseWriter) bool {
	if h != nil && h.advancedRepo != nil && h.advancedRepo.db != nil {
		return true
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "message": "notification repository is not available"})
	return false
}

func decodeNotificationJSON(r *http.Request, destination interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid notification payload: %w", err)
	}
	return nil
}

func validateNotificationRuleRequest(req notificationRuleRequest, creating bool) error {
	if creating && strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if creating && len(req.Channels) == 0 {
		return errors.New("at least one channel is required")
	}
	if !creating && req.Channels != nil && len(req.Channels) == 0 {
		return errors.New("at least one channel is required when channels is provided")
	}
	for _, channel := range req.Channels {
		if _, ok := notificationChannelNames[strings.ToLower(strings.TrimSpace(channel))]; !ok {
			return fmt.Errorf("unsupported notification channel: %s", channel)
		}
	}
	return nil
}

func validateNotificationTemplateRequest(req notificationTemplateRequest, creating bool) error {
	if creating && strings.TrimSpace(req.TemplateType) == "" {
		return errors.New("template_type is required")
	}
	if creating && strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if creating && strings.TrimSpace(req.Body) == "" {
		return errors.New("body is required")
	}
	return nil
}

func validateNotificationEscalationRequest(req notificationEscalationRequest, creating bool) error {
	if creating && strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if creating && len(req.Stages) == 0 {
		return errors.New("at least one escalation stage is required")
	}
	if !creating && req.Stages != nil && len(req.Stages) == 0 {
		return errors.New("at least one escalation stage is required when stages is provided")
	}
	for index, stage := range req.Stages {
		minutes, ok := numericNotificationValue(stage["after_minutes"])
		if !ok || minutes < 0 {
			return fmt.Errorf("stages[%d].after_minutes must be a non-negative number", index)
		}
		targetRole, exists := stage["target_role"]
		if !exists || targetRole == nil || strings.TrimSpace(fmt.Sprint(targetRole)) == "" {
			return fmt.Errorf("stages[%d].target_role is required", index)
		}
	}
	return nil
}

func numericNotificationValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

var notificationChannelNames = map[string]struct{}{
	"email": {}, "webhook": {}, "wechat": {}, "dingtalk": {}, "slack": {}, "feishu": {},
}

func (r *AdvancedRepository) ListNotificationRules(ctx context.Context, tenantID string, limit int) ([]NotificationRuleRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT rule_id::text, tenant_id, name, conditions, channels, enabled, COALESCE(created_by::text, ''), created_at, updated_at FROM notification_rules WHERE tenant_id=$1 ORDER BY updated_at DESC, name ASC, rule_id ASC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification rules: %w", err)
	}
	defer rows.Close()
	result := make([]NotificationRuleRecord, 0)
	for rows.Next() {
		record, scanErr := scanNotificationRule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

// ResolveNotificationChannels is the production execution bridge from the
// PostgreSQL governance workbench to the alert notifier. Disabled settings,
// active silence windows and non-matching subscription rules all fail closed.
func (r *AdvancedRepository) ResolveNotificationChannels(ctx context.Context, alert *notification.AlertInfo) ([]notification.ChannelRoute, error) {
	if r == nil || r.db == nil || alert == nil {
		return nil, errors.New("notification governance repository is not available")
	}
	settings := defaultNotificationSettings()
	if saved, ok, err := r.GetNotificationSettings(ctx, alert.TenantID); err != nil {
		return nil, err
	} else if ok {
		settings = mergeSettings(settings, saved)
	}
	if enabled, ok := settings["enabled"].(bool); !ok || !enabled {
		return []notification.ChannelRoute{}, nil
	}
	if !notificationSeverityAtLeast(alert.Severity, notificationText(settings["min_severity"])) {
		return []notification.ChannelRoute{}, nil
	}
	if err := r.enrichNotificationAlertDimensions(ctx, alert); err != nil {
		return nil, err
	}
	rules, err := r.ListNotificationRules(ctx, alert.TenantID, 200)
	if err != nil {
		return nil, err
	}
	policies, err := r.ListNotificationEscalationPolicies(ctx, alert.TenantID, 200)
	if err != nil {
		return nil, err
	}
	enabledPolicies := make(map[string]NotificationEscalationPolicyRecord, len(policies))
	for _, policy := range policies {
		if policy.Enabled {
			enabledPolicies[policy.Name] = policy
		}
	}
	silences, err := r.ListNotificationSilenceRules(ctx, alert.TenantID, 200)
	if err != nil {
		return nil, err
	}
	routes := make([]notification.ChannelRoute, 0)
	type pendingEscalation struct {
		rule   NotificationRuleRecord
		policy NotificationEscalationPolicyRecord
	}
	pendingEscalations := make([]pendingEscalation, 0)
	seen := map[string]struct{}{}
	for _, rule := range rules {
		if !rule.Enabled || !notificationRuleMatchesAlert(rule, alert, enabledPolicies) || notificationRuleIsSilenced(rule, alert, silences) {
			continue
		}
		policyName := notificationText(rule.Conditions["escalation_policy"])
		if policyName != "" {
			pendingEscalations = append(pendingEscalations, pendingEscalation{rule: rule, policy: enabledPolicies[policyName]})
			continue
		}
		target := rule.Name
		for _, channel := range rule.Channels {
			channel = strings.ToLower(strings.TrimSpace(channel))
			if channel == "" || !notificationChannelEnabled(settings, channel) {
				continue
			}
			key := rule.RuleID + "\x00" + channel + "\x00" + target
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			routes = append(routes, notification.ChannelRoute{Channel: channel, RuleID: rule.RuleID, TargetName: target})
		}
	}
	if len(routes) > 0 || len(pendingEscalations) > 0 {
		templates, err := r.ListNotificationTemplates(ctx, alert.TenantID, 200)
		if err != nil {
			return nil, err
		}
		for _, record := range templates {
			if !record.Enabled || (record.TemplateType != alert.AlertType && record.TemplateType != "告警模板") {
				continue
			}
			subject, body, err := renderNotificationTemplate(&record, map[string]interface{}{
				"alert_id": alert.AlertID, "title": alert.Title, "severity": alert.Severity, "score": alert.Score,
				"source_ip": alert.SourceIP, "dest_ip": alert.DestIP, "alert_type": alert.AlertType,
			})
			if err != nil {
				return nil, fmt.Errorf("render governed template %s: %w", record.TemplateID, err)
			}
			alert.Title, alert.Description = subject, body
			break
		}
	}
	for _, item := range pendingEscalations {
		if err := r.scheduleNotificationEscalation(ctx, item.rule, item.policy, alert, settings); err != nil {
			return nil, err
		}
	}
	return routes, nil
}

func notificationSeverityAtLeast(actual, minimum string) bool {
	levels := map[string]int{"low": 0, "medium": 1, "high": 2, "critical": 3}
	minimum = notification.NormalizeSeverity(minimum, 0)
	actual = notification.NormalizeSeverity(actual, 0)
	return levels[actual] >= levels[minimum]
}

func (r *AdvancedRepository) enrichNotificationAlertDimensions(ctx context.Context, alert *notification.AlertInfo) error {
	if alert == nil || (alert.ObjectID == "" && alert.SourceIP == "" && alert.DestIP == "") {
		return nil
	}
	var displayName, assetType, campus, department, tags string
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(NULLIF(display_code,''),NULLIF(hostname,''),asset_id::text),
		       COALESCE(asset_type,''),COALESCE(campus,''),COALESCE(department,''),COALESCE(tags::text,'')
		FROM assets
		WHERE tenant_id=$1 AND (
		  ($2<>'' AND (asset_id::text=$2 OR display_code=$2 OR hostname=$2)) OR
		  ($3<>'' AND (ip=$3 OR ip_address=$3)) OR
		  ($4<>'' AND (ip=$4 OR ip_address=$4))
		)
		ORDER BY CASE WHEN $2<>'' AND (asset_id::text=$2 OR display_code=$2 OR hostname=$2) THEN 0 ELSE 1 END,last_seen DESC
		LIMIT 1`, alert.TenantID, alert.ObjectID, alert.SourceIP, alert.DestIP).Scan(&displayName, &assetType, &campus, &department, &tags)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("enrich notification alert from asset inventory: %w", err)
	}
	alert.AssetName = displayName
	alert.AssetScope = strings.TrimSpace(strings.Join([]string{alert.AssetScope, assetType, department, tags}, " "))
	if strings.TrimSpace(campus) != "" {
		alert.Campus = campus
	}
	return nil
}

func notificationRuleMatchesAlert(rule NotificationRuleRecord, alert *notification.AlertInfo, enabledPolicies map[string]NotificationEscalationPolicyRecord) bool {
	if alert == nil {
		return false
	}
	if configuredType := notificationText(rule.Conditions["alert_type"]); configuredType != "" && notification.NormalizeAlertType(configuredType, "") != notification.NormalizeAlertType(alert.AlertType, alert.Description) {
		return false
	}
	configuredSeverity := strings.ToLower(strings.TrimSpace(fmt.Sprint(rule.Conditions["severity"])))
	levels := map[string]int{"low": 0, "medium": 1, "high": 2, "critical": 3, "低危": 0, "中危": 1, "高危": 2, "严重": 3}
	if minimum, ok := levels[configuredSeverity]; ok {
		actual, exists := levels[strings.ToLower(strings.TrimSpace(alert.Severity))]
		if !exists || actual < minimum {
			return false
		}
	}
	if policy := strings.TrimSpace(fmt.Sprint(rule.Conditions["escalation_policy"])); policy != "" && policy != "<nil>" {
		if _, enabled := enabledPolicies[policy]; !enabled {
			return false
		}
	}
	if !notificationTimeWindowMatches(notificationText(rule.Conditions["window_start"]), notificationText(rule.Conditions["window_end"]), alert.Timestamp) {
		return false
	}
	if !notificationScopeMatches(notificationText(rule.Conditions["asset_scope"]), alert.AssetScope, alert.AssetName, alert.Description, alert.SourceIP, alert.DestIP) {
		return false
	}
	if !notificationScopeMatches(notificationText(rule.Conditions["campus"]), alert.Campus, alert.Description) {
		return false
	}
	return true
}

func notificationText(value interface{}) string {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}

func notificationTimeWindowMatches(start, end string, at time.Time) bool {
	if start == "" || end == "" || at.IsZero() {
		return true
	}
	parse := func(value string) (int, bool) {
		parsed, err := time.Parse("15:04", value)
		if err != nil {
			return 0, false
		}
		return parsed.Hour()*60 + parsed.Minute(), true
	}
	startMinute, startOK := parse(start)
	endMinute, endOK := parse(end)
	if !startOK || !endOK {
		return false
	}
	minute := at.Hour()*60 + at.Minute()
	if startMinute <= endMinute {
		return minute >= startMinute && minute <= endMinute
	}
	return minute >= startMinute || minute <= endMinute
}

func notificationScopeMatches(configured string, candidates ...string) bool {
	configured = strings.ToLower(strings.TrimSpace(configured))
	if configured == "" || configured == "all" || strings.Contains(configured, "全部") || strings.HasPrefix(configured, "全园区") {
		return true
	}
	for _, candidate := range candidates {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate != "" && (strings.Contains(candidate, configured) || strings.Contains(configured, candidate)) {
			return true
		}
	}
	return false
}

func notificationRuleIsSilenced(rule NotificationRuleRecord, alert *notification.AlertInfo, silences []NotificationSilenceRule) bool {
	now := time.Now()
	policyName := notificationText(rule.Conditions["escalation_policy"])
	silenceMode := notificationText(rule.Conditions["silence_mode"])
	for _, silence := range silences {
		if !silence.Enabled || now.Before(silence.StartsAt) || now.After(silence.EndsAt) {
			continue
		}
		policyAll := strings.EqualFold(silence.Policy, "all") || strings.Contains(silence.Policy, "全部")
		policyMatches := policyAll || silence.Policy == policyName || (silenceMode != "" && silenceMode != "无" && silence.Name == silenceMode)
		if !policyMatches || !notificationScopeMatches(silence.Scope, alert.Campus, alert.AssetScope, alert.AssetName, alert.Description) {
			continue
		}
		if len(silence.AffectedTargets) == 0 {
			return true
		}
		for _, target := range silence.AffectedTargets {
			if notificationScopeMatches(target, alert.AssetName, alert.AssetScope, alert.Campus, alert.Description, alert.SourceIP, alert.DestIP, alert.AlertType) {
				return true
			}
		}
	}
	return false
}

// notificationEscalationTarget evaluates stage delays against the alert age.
// The Kafka consumer invokes governance again for subsequent detections, so a
// stage becomes eligible on the first durable alert update after its deadline.
func notificationEscalationTarget(rule NotificationRuleRecord, alert *notification.AlertInfo, policies map[string]NotificationEscalationPolicyRecord) (string, bool) {
	policyName := notificationText(rule.Conditions["escalation_policy"])
	if policyName == "" {
		return rule.Name, true
	}
	policy, ok := policies[policyName]
	if !ok {
		return "", false
	}
	ageMinutes := 0.0
	if !alert.Timestamp.IsZero() {
		ageMinutes = time.Since(alert.Timestamp).Minutes()
		if ageMinutes < 0 {
			ageMinutes = 0
		}
	}
	target := ""
	dueAt := -1.0
	for _, stage := range policy.Stages {
		after, valid := numericNotificationValue(stage["after_minutes"])
		if !valid || after > ageMinutes || after < dueAt || !notificationEscalationConditionMatches(notificationText(stage["condition"]), alert) {
			continue
		}
		target = notificationText(stage["target_role"])
		dueAt = after
	}
	return target, target != ""
}

func notificationEscalationConditionMatches(condition string, alert *notification.AlertInfo) bool {
	condition = strings.ToLower(strings.TrimSpace(condition))
	if condition == "" || condition == "sla 超时" || condition == "未确认" {
		return true
	}
	if strings.Contains(condition, "严重") {
		return alert.Severity == "critical"
	}
	if strings.Contains(condition, "重复") {
		return strings.Contains(strings.ToLower(alert.Description), "repeat") || strings.Contains(alert.Description, "重复")
	}
	if strings.Contains(condition, "失败") {
		return strings.Contains(strings.ToLower(alert.Description), "fail") || strings.Contains(alert.Description, "失败")
	}
	if strings.Contains(condition, "验收") {
		return strings.Contains(alert.Description, "验收") || strings.Contains(strings.ToLower(alert.Description), "acceptance")
	}
	return false
}

func (r *AdvancedRepository) CreateNotificationRule(ctx context.Context, tenantID, createdBy string, req notificationRuleRequest) (*NotificationRuleRecord, error) {
	conditions, _ := json.Marshal(req.Conditions)
	channels, _ := json.Marshal(req.Channels)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	createdBy = validNotificationUUID(createdBy)
	row := r.db.QueryRowContext(ctx, `INSERT INTO notification_rules (tenant_id,name,conditions,channels,enabled,created_by) VALUES ($1,$2,$3::jsonb,$4::jsonb,$5,NULLIF($6,'')::uuid) RETURNING rule_id::text,tenant_id,name,conditions,channels,enabled,COALESCE(created_by::text,''),created_at,updated_at`, tenantID, strings.TrimSpace(req.Name), string(conditions), string(channels), enabled, createdBy)
	record, err := scanNotificationRule(row)
	if err != nil {
		return nil, fmt.Errorf("create notification rule: %w", err)
	}
	return &record, nil
}

func (r *AdvancedRepository) PatchNotificationRule(ctx context.Context, tenantID, ruleID string, req notificationRuleRequest) (*NotificationRuleRecord, bool, error) {
	var conditions, channels interface{}
	if req.Conditions != nil {
		encoded, _ := json.Marshal(req.Conditions)
		conditions = string(encoded)
	}
	if req.Channels != nil {
		encoded, _ := json.Marshal(req.Channels)
		channels = string(encoded)
	}
	row := r.db.QueryRowContext(ctx, `UPDATE notification_rules SET name=COALESCE(NULLIF($3,''),name),conditions=COALESCE($4::jsonb,conditions),channels=COALESCE($5::jsonb,channels),enabled=COALESCE($6,enabled),updated_at=now() WHERE tenant_id=$1 AND rule_id::text=$2 RETURNING rule_id::text,tenant_id,name,conditions,channels,enabled,COALESCE(created_by::text,''),created_at,updated_at`, tenantID, ruleID, strings.TrimSpace(req.Name), conditions, channels, req.Enabled)
	record, err := scanNotificationRule(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("patch notification rule: %w", err)
	}
	return &record, true, nil
}

type notificationRuleScanner interface{ Scan(...interface{}) error }

func scanNotificationRule(scanner notificationRuleScanner) (NotificationRuleRecord, error) {
	var record NotificationRuleRecord
	var conditions, channels []byte
	if err := scanner.Scan(&record.RuleID, &record.TenantID, &record.Name, &conditions, &channels, &record.Enabled, &record.CreatedBy, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return record, err
	}
	_ = json.Unmarshal(conditions, &record.Conditions)
	_ = json.Unmarshal(channels, &record.Channels)
	if record.Conditions == nil {
		record.Conditions = map[string]interface{}{}
	}
	if record.Channels == nil {
		record.Channels = []string{}
	}
	return record, nil
}

func (r *AdvancedRepository) ListNotificationTemplates(ctx context.Context, tenantID string, limit int) ([]NotificationTemplateRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT template_id::text,tenant_id,template_type,name,version,subject,body,variable_schema,validation_status,enabled,created_by,created_at,updated_at FROM notification_templates WHERE tenant_id=$1 ORDER BY updated_at DESC, name ASC, template_id ASC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification templates: %w", err)
	}
	defer rows.Close()
	result := make([]NotificationTemplateRecord, 0)
	for rows.Next() {
		record, scanErr := scanNotificationTemplate(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (r *AdvancedRepository) GetNotificationTemplate(ctx context.Context, tenantID, templateID string) (*NotificationTemplateRecord, bool, error) {
	record, err := scanNotificationTemplate(r.db.QueryRowContext(ctx, `SELECT template_id::text,tenant_id,template_type,name,version,subject,body,variable_schema,validation_status,enabled,created_by,created_at,updated_at FROM notification_templates WHERE tenant_id=$1 AND template_id::text=$2`, tenantID, templateID))
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &record, true, nil
}

func (r *AdvancedRepository) CreateNotificationTemplate(ctx context.Context, tenantID, createdBy string, req notificationTemplateRequest) (*NotificationTemplateRecord, error) {
	variables, _ := json.Marshal(req.VariableSchema)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	record, err := scanNotificationTemplate(r.db.QueryRowContext(ctx, `INSERT INTO notification_templates (tenant_id,template_type,name,subject,body,variable_schema,enabled,created_by) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8) RETURNING template_id::text,tenant_id,template_type,name,version,subject,body,variable_schema,validation_status,enabled,created_by,created_at,updated_at`, tenantID, strings.TrimSpace(req.TemplateType), strings.TrimSpace(req.Name), req.Subject, req.Body, string(variables), enabled, createdBy))
	if err != nil {
		return nil, fmt.Errorf("create notification template: %w", err)
	}
	return &record, nil
}

func (r *AdvancedRepository) PatchNotificationTemplate(ctx context.Context, tenantID, templateID string, req notificationTemplateRequest) (*NotificationTemplateRecord, bool, error) {
	var variables interface{}
	if req.VariableSchema != nil {
		encoded, _ := json.Marshal(req.VariableSchema)
		variables = string(encoded)
	}
	record, err := scanNotificationTemplate(r.db.QueryRowContext(ctx, `UPDATE notification_templates SET template_type=COALESCE(NULLIF($3,''),template_type),name=COALESCE(NULLIF($4,''),name),subject=CASE WHEN $5='' THEN subject ELSE $5 END,body=CASE WHEN $6='' THEN body ELSE $6 END,variable_schema=COALESCE($7::jsonb,variable_schema),enabled=COALESCE($8,enabled),version=version+1,updated_at=now() WHERE tenant_id=$1 AND template_id::text=$2 RETURNING template_id::text,tenant_id,template_type,name,version,subject,body,variable_schema,validation_status,enabled,created_by,created_at,updated_at`, tenantID, templateID, strings.TrimSpace(req.TemplateType), strings.TrimSpace(req.Name), req.Subject, req.Body, variables, req.Enabled))
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("patch notification template: %w", err)
	}
	return &record, true, nil
}

type notificationTemplateScanner interface{ Scan(...interface{}) error }

func scanNotificationTemplate(scanner notificationTemplateScanner) (NotificationTemplateRecord, error) {
	var record NotificationTemplateRecord
	var variables []byte
	err := scanner.Scan(&record.TemplateID, &record.TenantID, &record.TemplateType, &record.Name, &record.Version, &record.Subject, &record.Body, &variables, &record.ValidationState, &record.Enabled, &record.CreatedBy, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return record, err
	}
	_ = json.Unmarshal(variables, &record.VariableSchema)
	if record.VariableSchema == nil {
		record.VariableSchema = map[string]interface{}{}
	}
	return record, nil
}

func (r *AdvancedRepository) ListNotificationEscalationPolicies(ctx context.Context, tenantID string, limit int) ([]NotificationEscalationPolicyRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT policy_id::text,tenant_id,name,stages,enabled,created_by,created_at,updated_at FROM notification_escalation_policies WHERE tenant_id=$1 ORDER BY updated_at DESC, name ASC, policy_id ASC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification escalation policies: %w", err)
	}
	defer rows.Close()
	result := make([]NotificationEscalationPolicyRecord, 0)
	for rows.Next() {
		record, scanErr := scanNotificationEscalationPolicy(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (r *AdvancedRepository) CreateNotificationEscalationPolicy(ctx context.Context, tenantID, createdBy string, req notificationEscalationRequest) (*NotificationEscalationPolicyRecord, error) {
	stages, _ := json.Marshal(req.Stages)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	record, err := scanNotificationEscalationPolicy(r.db.QueryRowContext(ctx, `INSERT INTO notification_escalation_policies (tenant_id,name,stages,enabled,created_by) VALUES ($1,$2,$3::jsonb,$4,$5) RETURNING policy_id::text,tenant_id,name,stages,enabled,created_by,created_at,updated_at`, tenantID, strings.TrimSpace(req.Name), string(stages), enabled, createdBy))
	if err != nil {
		return nil, fmt.Errorf("create notification escalation policy: %w", err)
	}
	return &record, nil
}

func (r *AdvancedRepository) PatchNotificationEscalationPolicy(ctx context.Context, tenantID, policyID string, req notificationEscalationRequest) (*NotificationEscalationPolicyRecord, bool, error) {
	var stages interface{}
	if req.Stages != nil {
		encoded, _ := json.Marshal(req.Stages)
		stages = string(encoded)
	}
	record, err := scanNotificationEscalationPolicy(r.db.QueryRowContext(ctx, `UPDATE notification_escalation_policies SET name=COALESCE(NULLIF($3,''),name),stages=COALESCE($4::jsonb,stages),enabled=COALESCE($5,enabled),updated_at=now() WHERE tenant_id=$1 AND policy_id::text=$2 RETURNING policy_id::text,tenant_id,name,stages,enabled,created_by,created_at,updated_at`, tenantID, policyID, strings.TrimSpace(req.Name), stages, req.Enabled))
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("patch notification escalation policy: %w", err)
	}
	return &record, true, nil
}

type notificationEscalationScanner interface{ Scan(...interface{}) error }

func scanNotificationEscalationPolicy(scanner notificationEscalationScanner) (NotificationEscalationPolicyRecord, error) {
	var record NotificationEscalationPolicyRecord
	var stages []byte
	err := scanner.Scan(&record.PolicyID, &record.TenantID, &record.Name, &stages, &record.Enabled, &record.CreatedBy, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return record, err
	}
	_ = json.Unmarshal(stages, &record.Stages)
	if record.Stages == nil {
		record.Stages = []map[string]interface{}{}
	}
	return record, nil
}

func (r *AdvancedRepository) ListNotificationDeliveries(ctx context.Context, tenantID string, limit int) ([]NotificationDeliveryRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT notification_id,tenant_id,COALESCE(rule_id::text,''),alert_id,target_name,channel,alert_type,status,COALESCE(error_message,''),retry_count,trace_id,sent_at,created_at FROM notification_history WHERE tenant_id=$1 ORDER BY created_at DESC, notification_id DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification deliveries: %w", err)
	}
	defer rows.Close()
	result := make([]NotificationDeliveryRecord, 0)
	for rows.Next() {
		record, scanErr := scanNotificationDelivery(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (r *AdvancedRepository) CreateNotificationDelivery(ctx context.Context, record NotificationDeliveryRecord) (*NotificationDeliveryRecord, error) {
	if strings.TrimSpace(record.TraceID) == "" {
		record.TraceID = "trace-" + uuid.NewString()
	}
	row := r.db.QueryRowContext(ctx, `INSERT INTO notification_history (tenant_id,rule_id,alert_id,target_name,channel,alert_type,status,error_message,retry_count,trace_id,sent_at) VALUES ($1,NULLIF($2,'')::uuid,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,CASE WHEN $7='sent' THEN now() ELSE NULL END) RETURNING notification_id,tenant_id,COALESCE(rule_id::text,''),alert_id,target_name,channel,alert_type,status,COALESCE(error_message,''),retry_count,trace_id,sent_at,created_at`, record.TenantID, record.RuleID, record.AlertID, record.TargetName, record.Channel, record.AlertType, record.Status, record.ErrorMessage, record.RetryCount, record.TraceID)
	created, err := scanNotificationDelivery(row)
	if err != nil {
		return nil, fmt.Errorf("create notification delivery: %w", err)
	}
	return &created, nil
}

// RecordAutomaticNotificationDelivery is the durable sink used by Kafka-driven
// notification execution. The notification_history insert and its audit row
// are committed atomically by the database trigger installed by InitSchema.
func (r *AdvancedRepository) RecordAutomaticNotificationDelivery(ctx context.Context, result notification.DeliveryResult) error {
	if result.Alert == nil {
		return errors.New("automatic notification delivery requires alert context")
	}
	_, err := r.CreateNotificationDelivery(ctx, NotificationDeliveryRecord{
		TenantID:     result.Alert.TenantID,
		RuleID:       result.Route.RuleID,
		AlertID:      result.Alert.AlertID,
		TargetName:   result.Route.TargetName,
		Channel:      result.Route.Channel,
		AlertType:    result.Alert.AlertType,
		Status:       result.Status,
		ErrorMessage: result.ErrorMessage,
		TraceID:      "auto-" + uuid.NewString(),
	})
	return err
}

func (r *AdvancedRepository) GetNotificationDelivery(ctx context.Context, tenantID string, notificationID int64) (*NotificationDeliveryRecord, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT notification_id,tenant_id,COALESCE(rule_id::text,''),alert_id,target_name,channel,alert_type,status,COALESCE(error_message,''),retry_count,trace_id,sent_at,created_at FROM notification_history WHERE tenant_id=$1 AND notification_id=$2`, tenantID, notificationID)
	record, err := scanNotificationDelivery(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get notification delivery: %w", err)
	}
	return &record, true, nil
}

func (r *AdvancedRepository) CompleteNotificationDeliveryRetry(ctx context.Context, tenantID string, notificationID int64, status, errorMessage string) (*NotificationDeliveryRecord, bool, error) {
	if status != "sent" && status != "failed" {
		return nil, false, errors.New("retry status must be sent or failed")
	}
	row := r.db.QueryRowContext(ctx, `UPDATE notification_history SET status=$3,error_message=NULLIF($4,''),retry_count=retry_count+1,trace_id=$5,sent_at=CASE WHEN $3='sent' THEN now() ELSE sent_at END WHERE tenant_id=$1 AND notification_id=$2 RETURNING notification_id,tenant_id,COALESCE(rule_id::text,''),alert_id,target_name,channel,alert_type,status,COALESCE(error_message,''),retry_count,trace_id,sent_at,created_at`, tenantID, notificationID, status, errorMessage, "trace-"+uuid.NewString())
	record, err := scanNotificationDelivery(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("complete notification delivery retry: %w", err)
	}
	return &record, true, nil
}

type notificationDeliveryScanner interface{ Scan(...interface{}) error }

func scanNotificationDelivery(scanner notificationDeliveryScanner) (NotificationDeliveryRecord, error) {
	var record NotificationDeliveryRecord
	var sentAt sql.NullTime
	err := scanner.Scan(&record.NotificationID, &record.TenantID, &record.RuleID, &record.AlertID, &record.TargetName, &record.Channel, &record.AlertType, &record.Status, &record.ErrorMessage, &record.RetryCount, &record.TraceID, &sentAt, &record.CreatedAt)
	if err != nil {
		return record, err
	}
	if sentAt.Valid {
		record.SentAt = &sentAt.Time
	}
	return record, nil
}

func validNotificationUUID(value string) string {
	value = strings.TrimSpace(value)
	if _, err := uuid.Parse(value); err != nil {
		return ""
	}
	return value
}

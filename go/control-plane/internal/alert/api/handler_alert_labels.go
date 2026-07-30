package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/gorilla/mux"
)

type updateAlertLabelsRequest struct {
	Labels []string `json:"labels"`
	Reason string   `json:"reason"`
}

// UpdateAlertLabels provides a real alert-detail edit path instead of
// representing label edits as a generic investigation note.
func (h *Handler) UpdateAlertLabels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireAlertWritePermission(w, r) {
		return
	}
	tenantID := h.extractTenantID(r)
	alertID := strings.TrimSpace(mux.Vars(r)["id"])
	if tenantID == "" || alertID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "ALERT_REQUIRED", "tenant_id and alert id are required")
		return
	}
	var request updateAlertLabelsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	reason := strings.TrimSpace(request.Reason)
	if len(reason) < 4 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "REASON_REQUIRED", "reason must contain at least 4 characters")
		return
	}
	labels := normalizeAlertLabels(request.Labels)
	if len(labels) == 0 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "LABELS_REQUIRED", "at least one label is required")
		return
	}
	if h.alertService == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert service is unavailable")
		return
	}
	if err := h.alertService.UpdateLabels(ctx, tenantID, alertID, labels, h.extractUserID(r)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "LABEL_UPDATE_FAILED", err.Error())
		return
	}
	h.recordAlertActionAudit(ctx, r, AlertActionAuditRecord{
		Action:   "ALERT_LABELS_UPDATED",
		TenantID: tenantID,
		UserID:   h.extractUserID(r),
		AlertID:  alertID,
		Reason:   reason,
		Result:   "success",
		Detail:   map[string]interface{}{"labels": labels},
	})
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"alert_id": alertID,
		"labels":   labels,
		"reason":   reason,
		"status":   "updated",
	})
}

func normalizeAlertLabels(input []string) []string {
	labels := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, raw := range input {
		label := strings.TrimSpace(raw)
		if label == "" {
			continue
		}
		if len([]rune(label)) > 32 {
			label = string([]rune(label)[:32])
		}
		key := strings.ToLower(label)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		labels = append(labels, label)
		if len(labels) == 8 {
			break
		}
	}
	return labels
}

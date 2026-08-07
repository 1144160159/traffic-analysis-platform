package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
)

func (h *HTTPHandler) assetGovernanceCollection(w http.ResponseWriter, r *http.Request, assetID string) {
	if !h.governanceV1Enabled {
		writeAssetCommandError(w, http.StatusNotFound, traceIDFromRequest(r), "feature_disabled", "asset governance v1 is not enabled")
		return
	}
	if _, err := uuid.Parse(assetID); err != nil {
		writeAssetCommandError(w, http.StatusBadRequest, traceIDFromRequest(r), "invalid_asset_id", "asset_id must be a UUID")
		return
	}
	switch r.Method {
	case http.MethodGet:
		identity, ok := h.requireAssetRead(w, r)
		if !ok {
			return
		}
		orders, err := h.svc.ListAssetGovernanceWorkOrders(r.Context(), identity.TenantID, assetID)
		if err != nil {
			writeAssetCommandError(w, http.StatusInternalServerError, traceIDFromRequest(r), "governance_read_failed", err.Error())
			return
		}
		writeGovernanceEnvelope(w, http.StatusOK, traceIDFromRequest(r), orders, map[string]any{"count": len(orders)})
	case http.MethodPost:
		identity, ok := h.requireAssetGovernance(w, r)
		if !ok {
			return
		}
		traceID := traceIDFromRequest(r)
		var command config.AssetGovernanceCreateCommand
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&command); err != nil {
			writeAssetCommandError(w, http.StatusBadRequest, traceID, "invalid_request", "invalid governance work-order payload")
			return
		}
		command.TenantID = identity.TenantID
		command.Actor = auditActor(identity)
		command.TraceID = traceID
		command.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		command.RequestID = requestIDFromRequest(r)
		command.ClientIP = clientIP(r)
		command.UserAgent = r.UserAgent()
		order, err := h.svc.CreateAssetGovernanceWorkOrder(r.Context(), assetID, command)
		if err != nil {
			writeGovernanceError(w, traceID, err)
			return
		}
		writeGovernanceEnvelope(w, http.StatusAccepted, traceID, order, map[string]any{"work_order_revision": order.Revision, "asset_revision": order.ExpectedAssetRevision})
	default:
		writeAssetCommandError(w, http.StatusMethodNotAllowed, traceIDFromRequest(r), "method_not_allowed", "method not allowed")
	}
}

func (h *HTTPHandler) assetGovernanceWorkOrderResource(w http.ResponseWriter, r *http.Request, remainder string) {
	if !h.governanceV1Enabled {
		writeAssetCommandError(w, http.StatusNotFound, traceIDFromRequest(r), "feature_disabled", "asset governance v1 is not enabled")
		return
	}
	parts := strings.Split(strings.Trim(remainder, "/"), "/")
	if len(parts) < 1 || len(parts) > 2 {
		writeAssetCommandError(w, http.StatusNotFound, traceIDFromRequest(r), "not_found", "unknown governance resource")
		return
	}
	if _, err := uuid.Parse(parts[0]); err != nil {
		writeAssetCommandError(w, http.StatusBadRequest, traceIDFromRequest(r), "invalid_work_order_id", "work_order_id must be a UUID")
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeAssetCommandError(w, http.StatusMethodNotAllowed, traceIDFromRequest(r), "method_not_allowed", "method not allowed")
			return
		}
		identity, ok := h.requireAssetRead(w, r)
		if !ok {
			return
		}
		order, err := h.svc.GetAssetGovernanceWorkOrder(r.Context(), identity.TenantID, parts[0])
		if err != nil {
			writeGovernanceError(w, traceIDFromRequest(r), err)
			return
		}
		writeGovernanceEnvelope(w, http.StatusOK, traceIDFromRequest(r), order, map[string]any{"work_order_revision": order.Revision})
		return
	}
	if parts[1] != "actions" || r.Method != http.MethodPost {
		writeAssetCommandError(w, http.StatusMethodNotAllowed, traceIDFromRequest(r), "method_not_allowed", "method not allowed")
		return
	}
	identity, ok := h.requireAssetGovernance(w, r)
	if !ok {
		return
	}
	traceID := traceIDFromRequest(r)
	var command config.AssetGovernanceActionCommand
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&command); err != nil {
		writeAssetCommandError(w, http.StatusBadRequest, traceID, "invalid_request", "invalid governance action payload")
		return
	}
	command.TenantID = identity.TenantID
	command.Actor = auditActor(identity)
	command.TraceID = traceID
	command.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	command.RequestID = requestIDFromRequest(r)
	command.ClientIP = clientIP(r)
	command.UserAgent = r.UserAgent()
	order, err := h.svc.ApplyAssetGovernanceAction(r.Context(), parts[0], command)
	if err != nil {
		writeGovernanceError(w, traceID, err)
		return
	}
	writeGovernanceEnvelope(w, http.StatusAccepted, traceID, order, map[string]any{"work_order_revision": order.Revision, "asset_revision": order.ResultingAssetRevision})
}

func writeGovernanceError(w http.ResponseWriter, traceID string, err error) {
	status, code := http.StatusBadRequest, "governance_command_rejected"
	switch {
	case errors.Is(err, repository.ErrAssetGovernanceNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, repository.ErrAssetGovernanceIdempotencyConflict):
		status, code = http.StatusConflict, "idempotency_conflict"
	case errors.Is(err, repository.ErrAssetGovernanceRevisionConflict):
		status, code = http.StatusConflict, "revision_conflict"
	case errors.Is(err, repository.ErrAssetGovernanceStateConflict):
		status, code = http.StatusConflict, "state_conflict"
	case errors.Is(err, repository.ErrAssetGovernanceSelfApproval):
		status, code = http.StatusForbidden, "self_approval_forbidden"
	case errors.Is(err, repository.ErrAssetGovernanceEvidenceRequired):
		status, code = http.StatusUnprocessableEntity, "evidence_required"
	case errors.Is(err, repository.ErrAssetGovernanceAssetStale):
		status, code = http.StatusConflict, "asset_revision_conflict"
	}
	writeAssetCommandError(w, status, traceID, code, err.Error())
}

func writeGovernanceEnvelope(w http.ResponseWriter, status int, traceID string, data any, watermarks map[string]any) {
	now := time.Now().UTC()
	writeJSON(w, status, map[string]any{"data": data, "meta": map[string]any{
		"contract_version": "1", "snapshot_id": "asset-governance-" + traceID, "as_of": now.Format(time.RFC3339Nano),
		"trace_id": traceID, "partial": false, "missing_sections": []string{}, "source_watermarks": watermarks}, "error": nil})
}

func requestIDFromRequest(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if v == "" {
		return traceIDFromRequest(r)
	}
	return v
}

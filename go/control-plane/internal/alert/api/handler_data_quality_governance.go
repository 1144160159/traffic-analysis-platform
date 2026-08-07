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

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/dataquality"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type dataQualityDatasetCommandRequest struct {
	DisplayName           string   `json:"display_name"`
	Owner                 string   `json:"owner"`
	SchemaVersion         int64    `json:"schema_version"`
	SignalContractVersion string   `json:"signal_contract_version"`
	BusinessKeys          []string `json:"business_keys"`
	AllowedLateness       int64    `json:"allowed_lateness_seconds"`
	RetentionSeconds      int64    `json:"retention_seconds"`
	Upstreams             []string `json:"upstreams"`
	Downstreams           []string `json:"downstreams"`
	SLOTarget             float64  `json:"slo_target"`
	Status                string   `json:"status"`
	ExpectedRevision      int64    `json:"expected_revision"`
	ActionID              string   `json:"action_id"`
	Reason                string   `json:"reason"`
}

type dataQualityRuleCreateRequest struct {
	DatasetID        string                 `json:"dataset_id"`
	RuleKey          string                 `json:"rule_key"`
	Dimension        string                 `json:"dimension"`
	FieldPath        string                 `json:"field_path"`
	Predicate        map[string]interface{} `json:"predicate"`
	Threshold        map[string]interface{} `json:"threshold"`
	WindowSeconds    int64                  `json:"window_seconds"`
	Sampling         map[string]interface{} `json:"sampling"`
	Severity         string                 `json:"severity"`
	Owner            string                 `json:"owner"`
	ExemptionPolicy  map[string]interface{} `json:"exemption_policy"`
	RepairAction     string                 `json:"repair_action"`
	GatePolicy       string                 `json:"gate_policy"`
	ExpectedRevision int64                  `json:"expected_revision"`
	ActionID         string                 `json:"action_id"`
	Reason           string                 `json:"reason"`
}

type dataQualityRuleTransitionRequest struct {
	Action           string `json:"action"`
	ExpectedRevision int64  `json:"expected_revision"`
	ActionID         string `json:"action_id"`
	Reason           string `json:"reason"`
}

type dataQualityRepairCreateRequest struct {
	OperationID    string                 `json:"operation_id"`
	InputScope     map[string]interface{} `json:"input_scope"`
	ResourceBudget map[string]interface{} `json:"resource_budget"`
	ActionID       string                 `json:"action_id"`
	Reason         string                 `json:"reason"`
}

type dataQualityRepairTransitionRequest struct {
	Action           string                 `json:"action"`
	ExpectedRevision int64                  `json:"expected_revision"`
	Summary          map[string]interface{} `json:"summary"`
	ActionID         string                 `json:"action_id"`
	Reason           string                 `json:"reason"`
}

// DataQualityRepairExecutor is a runtime capability, not a request-controlled
// switch. The control-plane refuses to enter executing unless a real executor
// has been registered and reports ready.
type DataQualityRepairExecutor interface {
	Ready(context.Context) error
}

// DataQualityRepairEvidenceProvider derives dry-run and reconciliation facts
// from server-side authorities. Client-provided summary JSON is never trusted
// to advance these two transitions.
type DataQualityRepairEvidenceProvider interface {
	DryRun(context.Context, string, string) (map[string]interface{}, error)
	Reconcile(context.Context, string, string) (map[string]interface{}, error)
}

func (h *AdvancedHandler) SetDataQualityRepairExecutionFeatureFlag(enabled bool) {
	h.dataQualityRepairExecution = enabled
}

func (h *AdvancedHandler) SetDataQualityRepairExecutor(executor DataQualityRepairExecutor) {
	h.dataQualityRepairExecutor = executor
}

func (h *AdvancedHandler) SetDataQualityRepairEvidenceProvider(provider DataQualityRepairEvidenceProvider) {
	h.dataQualityRepairEvidence = provider
}

func (h *AdvancedHandler) ListDataQualityDatasets(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityReadPermission(w, r) {
		return
	}
	if h.dqMonitor == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_GOVERNANCE_UNAVAILABLE", "data quality governance is not available")
		return
	}
	records, err := h.dqMonitor.ListDatasets(r.Context(), tenantIDFromRequest(r))
	if err != nil {
		writeDataQualityGovernanceError(w, r, err)
		return
	}
	httpx.JSONContractSuccess(w, r.Context(), map[string]interface{}{"items": records, "total": len(records)}, dataQualityGovernanceMeta(r, "datasets", 0))
}

func (h *AdvancedHandler) UpsertDataQualityDataset(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityWritePermission(w, r) {
		return
	}
	identity, ok := dataQualityGovernanceIdentity(w, r)
	if !ok {
		return
	}
	datasetID := strings.TrimSpace(mux.Vars(r)["dataset_id"])
	if datasetID == "" || len(datasetID) > 160 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_DATASET_ID", "dataset_id must be 1-160 characters")
		return
	}
	var payload dataQualityDatasetCommandRequest
	if !decodeDataQualityGovernanceRequest(w, r, &payload) {
		return
	}
	if payload.Status == "" {
		payload.Status = "active"
	}
	if h.dqMonitor == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_GOVERNANCE_UNAVAILABLE", "data quality governance is not available")
		return
	}
	record, err := h.dqMonitor.UpsertDataset(r.Context(), dataquality.DatasetCommand{
		TenantID: identity.tenantID, DatasetID: datasetID, DisplayName: strings.TrimSpace(payload.DisplayName),
		Owner: strings.TrimSpace(payload.Owner), SchemaVersion: payload.SchemaVersion,
		SignalContractVersion: strings.TrimSpace(payload.SignalContractVersion), BusinessKeys: payload.BusinessKeys,
		AllowedLateness: payload.AllowedLateness, RetentionSeconds: payload.RetentionSeconds,
		Upstreams: payload.Upstreams, Downstreams: payload.Downstreams, SLOTarget: payload.SLOTarget,
		Status: strings.TrimSpace(payload.Status), ExpectedRevision: payload.ExpectedRevision,
		ActionID: strings.TrimSpace(payload.ActionID), IdempotencyKey: identity.idempotencyKey,
		Reason: strings.TrimSpace(payload.Reason), Actor: identity.actor, TraceID: identity.traceID,
	})
	if err != nil {
		writeDataQualityGovernanceError(w, r, err)
		return
	}
	httpx.JSONContractSuccess(w, r.Context(), record, dataQualityGovernanceMeta(r, record.DatasetID, record.Revision))
}

func (h *AdvancedHandler) ListDataQualityRules(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityReadPermission(w, r) {
		return
	}
	if h.dqMonitor == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_GOVERNANCE_UNAVAILABLE", "data quality governance is not available")
		return
	}
	datasetID := strings.TrimSpace(r.URL.Query().Get("dataset_id"))
	records, err := h.dqMonitor.ListRules(r.Context(), tenantIDFromRequest(r), datasetID)
	if err != nil {
		writeDataQualityGovernanceError(w, r, err)
		return
	}
	httpx.JSONContractSuccess(w, r.Context(), map[string]interface{}{"items": records, "total": len(records)}, dataQualityGovernanceMeta(r, "rules", 0))
}

func (h *AdvancedHandler) CreateDataQualityRule(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityWritePermission(w, r) {
		return
	}
	identity, ok := dataQualityGovernanceIdentity(w, r)
	if !ok {
		return
	}
	var payload dataQualityRuleCreateRequest
	if !decodeDataQualityGovernanceRequest(w, r, &payload) {
		return
	}
	if payload.Predicate == nil {
		payload.Predicate = map[string]interface{}{}
	}
	if payload.Threshold == nil {
		payload.Threshold = map[string]interface{}{}
	}
	if payload.Sampling == nil {
		payload.Sampling = map[string]interface{}{}
	}
	if payload.ExemptionPolicy == nil {
		payload.ExemptionPolicy = map[string]interface{}{}
	}
	if h.dqMonitor == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_GOVERNANCE_UNAVAILABLE", "data quality governance is not available")
		return
	}
	record, err := h.dqMonitor.CreateRule(r.Context(), dataquality.RuleCreateCommand{
		TenantID: identity.tenantID, DatasetID: strings.TrimSpace(payload.DatasetID), RuleKey: strings.TrimSpace(payload.RuleKey),
		Dimension: strings.TrimSpace(payload.Dimension), FieldPath: strings.TrimSpace(payload.FieldPath),
		Predicate: payload.Predicate, Threshold: payload.Threshold, WindowSeconds: payload.WindowSeconds,
		Sampling: payload.Sampling, Severity: strings.TrimSpace(payload.Severity), Owner: strings.TrimSpace(payload.Owner),
		ExemptionPolicy: payload.ExemptionPolicy, RepairAction: strings.TrimSpace(payload.RepairAction),
		GatePolicy: strings.TrimSpace(payload.GatePolicy), ExpectedRevision: payload.ExpectedRevision,
		ActionID: strings.TrimSpace(payload.ActionID), IdempotencyKey: identity.idempotencyKey,
		Reason: strings.TrimSpace(payload.Reason), Actor: identity.actor, TraceID: identity.traceID,
	})
	if err != nil {
		writeDataQualityGovernanceError(w, r, err)
		return
	}
	httpx.JSONContractSuccess(w, r.Context(), record, dataQualityGovernanceMeta(r, record.RuleID, record.Revision))
}

func (h *AdvancedHandler) TransitionDataQualityRule(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityWritePermission(w, r) {
		return
	}
	identity, ok := dataQualityGovernanceIdentity(w, r)
	if !ok {
		return
	}
	var payload dataQualityRuleTransitionRequest
	if !decodeDataQualityGovernanceRequest(w, r, &payload) {
		return
	}
	if h.dqMonitor == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_GOVERNANCE_UNAVAILABLE", "data quality governance is not available")
		return
	}
	record, err := h.dqMonitor.TransitionRule(r.Context(), dataquality.RuleTransitionCommand{
		TenantID: identity.tenantID, RuleID: strings.TrimSpace(mux.Vars(r)["rule_id"]),
		Action: strings.TrimSpace(payload.Action), ExpectedRevision: payload.ExpectedRevision,
		ActionID: strings.TrimSpace(payload.ActionID), IdempotencyKey: identity.idempotencyKey,
		Reason: strings.TrimSpace(payload.Reason), Actor: identity.actor, TraceID: identity.traceID,
	})
	if err != nil {
		writeDataQualityGovernanceError(w, r, err)
		return
	}
	httpx.JSONContractSuccess(w, r.Context(), record, dataQualityGovernanceMeta(r, record.RuleID, record.Revision))
}

func (h *AdvancedHandler) CreateDataQualityRepair(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityWritePermission(w, r) {
		return
	}
	identity, ok := dataQualityGovernanceIdentity(w, r)
	if !ok {
		return
	}
	var payload dataQualityRepairCreateRequest
	if !decodeDataQualityGovernanceRequest(w, r, &payload) {
		return
	}
	if h.dqMonitor == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_GOVERNANCE_UNAVAILABLE", "data quality governance is not available")
		return
	}
	record, err := h.dqMonitor.CreateRepair(r.Context(), dataquality.RepairCreateCommand{
		TenantID: identity.tenantID, QualityEventID: strings.TrimSpace(mux.Vars(r)["quality_event_id"]),
		OperationID: strings.TrimSpace(payload.OperationID), InputScope: payload.InputScope,
		ResourceBudget: payload.ResourceBudget, ActionID: strings.TrimSpace(payload.ActionID),
		IdempotencyKey: identity.idempotencyKey, Reason: strings.TrimSpace(payload.Reason),
		Actor: identity.actor, TraceID: identity.traceID,
	})
	if err != nil {
		writeDataQualityGovernanceError(w, r, err)
		return
	}
	httpx.JSONContractSuccess(w, r.Context(), record, dataQualityGovernanceMeta(r, record.RepairID, record.Revision))
}

func (h *AdvancedHandler) TransitionDataQualityRepair(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityWritePermission(w, r) {
		return
	}
	identity, ok := dataQualityGovernanceIdentity(w, r)
	if !ok {
		return
	}
	var payload dataQualityRepairTransitionRequest
	if !decodeDataQualityGovernanceRequest(w, r, &payload) {
		return
	}
	if payload.Summary == nil {
		payload.Summary = map[string]interface{}{}
	}
	if h.dqMonitor == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_GOVERNANCE_UNAVAILABLE", "data quality governance is not available")
		return
	}
	if strings.TrimSpace(payload.Action) == "complete_dry_run" || strings.TrimSpace(payload.Action) == "reconcile" {
		if h.dataQualityRepairEvidence == nil {
			httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_REPAIR_EVIDENCE_UNAVAILABLE", "server-side data quality repair evidence provider is not registered")
			return
		}
		var summary map[string]interface{}
		var err error
		if strings.TrimSpace(payload.Action) == "complete_dry_run" {
			summary, err = h.dataQualityRepairEvidence.DryRun(r.Context(), identity.tenantID, strings.TrimSpace(mux.Vars(r)["repair_id"]))
		} else {
			summary, err = h.dataQualityRepairEvidence.Reconcile(r.Context(), identity.tenantID, strings.TrimSpace(mux.Vars(r)["repair_id"]))
		}
		if err != nil {
			if errors.Is(err, dataquality.ErrRepairNotFound) || errors.Is(err, dataquality.ErrRepairConflict) {
				writeDataQualityGovernanceError(w, r, err)
				return
			}
			httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_REPAIR_EVIDENCE_UNAVAILABLE", "server-side data quality repair evidence could not be collected")
			return
		}
		payload.Summary = summary
	}
	if strings.TrimSpace(payload.Action) == "start_execution" && h.dataQualityRepairExecution {
		if h.dataQualityRepairExecutor == nil {
			httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_REPAIR_EXECUTOR_UNAVAILABLE", "data quality repair executor is not registered")
			return
		}
		if err := h.dataQualityRepairExecutor.Ready(r.Context()); err != nil {
			httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_REPAIR_EXECUTOR_UNAVAILABLE", "data quality repair executor is not ready")
			return
		}
	}
	record, err := h.dqMonitor.TransitionRepair(r.Context(), dataquality.RepairTransitionCommand{
		TenantID: identity.tenantID, RepairID: strings.TrimSpace(mux.Vars(r)["repair_id"]),
		Action: strings.TrimSpace(payload.Action), ExpectedRevision: payload.ExpectedRevision,
		Summary: payload.Summary, ActionID: strings.TrimSpace(payload.ActionID),
		IdempotencyKey: identity.idempotencyKey, Reason: strings.TrimSpace(payload.Reason),
		Actor: identity.actor, TraceID: identity.traceID,
	}, h.dataQualityRepairExecution)
	if err != nil {
		writeDataQualityGovernanceError(w, r, err)
		return
	}
	meta := dataQualityGovernanceMeta(r, record.RepairID, record.Revision)
	if payload.Action == "start_execution" {
		httpx.JSONContractAccepted(w, r.Context(), record, meta)
		return
	}
	httpx.JSONContractSuccess(w, r.Context(), record, meta)
}

type dataQualityGovernanceRequestIdentity struct {
	tenantID, actor, traceID, idempotencyKey string
}

func dataQualityGovernanceIdentity(w http.ResponseWriter, r *http.Request) (dataQualityGovernanceRequestIdentity, bool) {
	identity := dataQualityGovernanceRequestIdentity{
		tenantID: tenantIDFromRequest(r), actor: strings.TrimSpace(httpx.GetUserID(r.Context())),
		traceID: strings.TrimSpace(httpx.GetTraceID(r.Context())), idempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	}
	if identity.tenantID == "" || identity.actor == "" || identity.traceID == "" {
		httpx.JSONError(w, r.Context(), http.StatusUnauthorized, "AUTHENTICATED_IDENTITY_REQUIRED", "tenant, user and trace identity are required")
		return identity, false
	}
	if len(identity.idempotencyKey) < 16 || len(identity.idempotencyKey) > 200 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must be 16-200 characters")
		return identity, false
	}
	return identity, true
}

func decodeDataQualityGovernanceRequest(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid data quality governance payload")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "request body must contain one JSON object")
		return false
	}
	return true
}

func writeDataQualityGovernanceError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "DATA_QUALITY_GOVERNANCE_FAILED"
	switch {
	case errors.Is(err, dataquality.ErrGovernanceUnavailable):
		status, code = http.StatusServiceUnavailable, "DATA_QUALITY_GOVERNANCE_UNAVAILABLE"
	case errors.Is(err, dataquality.ErrGovernanceNotFound):
		status, code = http.StatusNotFound, "DATA_QUALITY_RESOURCE_NOT_FOUND"
	case errors.Is(err, dataquality.ErrGovernanceConflict):
		status, code = http.StatusConflict, "DATA_QUALITY_REVISION_CONFLICT"
	case errors.Is(err, dataquality.ErrIdempotencyConflict):
		status, code = http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT"
	case errors.Is(err, dataquality.ErrInvalidTransition):
		status, code = http.StatusConflict, "DATA_QUALITY_TRANSITION_REJECTED"
	case errors.Is(err, dataquality.ErrSelfApproval):
		status, code = http.StatusForbidden, "SEPARATE_APPROVER_REQUIRED"
	case errors.Is(err, dataquality.ErrRepairNotFound):
		status, code = http.StatusNotFound, "DATA_QUALITY_REPAIR_NOT_FOUND"
	case errors.Is(err, dataquality.ErrRepairConflict):
		status, code = http.StatusConflict, "DATA_QUALITY_REPAIR_REVISION_CONFLICT"
	case errors.Is(err, dataquality.ErrRepairApprovalSeparation):
		status, code = http.StatusForbidden, "SEPARATE_APPROVER_REQUIRED"
	case errors.Is(err, dataquality.ErrRepairExecutionDisabled):
		status, code = http.StatusServiceUnavailable, "DATA_QUALITY_REPAIR_EXECUTION_DISABLED"
	case strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "must be"):
		status, code = http.StatusBadRequest, "INVALID_REQUEST"
	}
	httpx.JSONError(w, r.Context(), status, code, err.Error())
}

func dataQualityGovernanceMeta(r *http.Request, snapshotID string, revision int64) httpx.ContractMeta {
	watermarks := map[string]string{}
	if revision > 0 {
		watermarks["postgresql_revision"] = strconv.FormatInt(revision, 10)
	}
	return httpx.ContractMeta{ContractVersion: 1, SnapshotID: snapshotID, AsOf: time.Now().UTC().Format(time.RFC3339Nano), TraceID: httpx.GetTraceID(r.Context()), Partial: false, MissingSections: []string{}, SourceWatermarks: watermarks}
}

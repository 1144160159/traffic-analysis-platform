package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/baseline"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type behaviorBaselineBuildV1Request struct {
	BaselineKind        string                 `json:"baseline_kind"`
	ExpectedRevision    int64                  `json:"expected_revision"`
	WindowStart         *time.Time             `json:"window_start,omitempty"`
	WindowEnd           *time.Time             `json:"window_end,omitempty"`
	MinimumEligibleRows int64                  `json:"minimum_eligible_rows,omitempty"`
	AlgorithmVersion    string                 `json:"algorithm_version"`
	SamplePolicy        map[string]interface{} `json:"sample_policy"`
	ThresholdSpec       map[string]interface{} `json:"threshold_spec"`
	ExpectedConsumers   []string               `json:"expected_consumers"`
	IdempotencyKey      string                 `json:"idempotency_key,omitempty"`
	Reason              string                 `json:"reason"`
}

type behaviorBaselineApprovalV1Request struct {
	ExpectedRevision int64     `json:"expected_revision"`
	IdempotencyKey   string    `json:"idempotency_key,omitempty"`
	Reason           string    `json:"reason"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type behaviorBaselineApprovalDecisionV1Request struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Approve          bool   `json:"approve"`
	Reason           string `json:"reason"`
}

type behaviorBaselineRollbackV1Request struct {
	TargetStableVersion int64  `json:"target_stable_version"`
	ExpectedRevision    int64  `json:"expected_revision"`
	IdempotencyKey      string `json:"idempotency_key,omitempty"`
	Reason              string `json:"reason"`
}

type behaviorBaselineEvaluationV1Request struct {
	MetricName    string     `json:"metric_name"`
	ObservedValue float64    `json:"observed_value"`
	ObservedAt    time.Time  `json:"observed_at"`
	WindowStart   *time.Time `json:"window_start,omitempty"`
	WindowEnd     *time.Time `json:"window_end,omitempty"`
	EvidenceRefs  []string   `json:"evidence_refs"`
}

func (h *SystemHandler) RequestBehaviorBaselineBuildV1(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireBehaviorBaselineWritePermission(w, r) || !h.requirePostgres(w, ctx) {
		return
	}
	if !h.baselineV1 || h.baselineRepository == nil || len(h.baselineCandidateSHA256) != 64 {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "BASELINE_V1_DISABLED", "versioned behavior baseline authority is disabled")
		return
	}
	baselineID := strings.TrimSpace(mux.Vars(r)["id"])
	entityType, entityID := parseBaselineID(baselineID)
	if baselineID == "" || len(baselineID) > 255 || entityType == "" || entityID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_BASELINE_ID", "baseline id must be entity_type:entity_id")
		return
	}
	var body behaviorBaselineBuildV1Request
	if !decodeBaselineV1Body(w, r, &body) {
		return
	}
	tenantID := writeTenantID(r)
	requestedBy := strings.TrimSpace(httpx.GetUserID(ctx))
	if _, err := uuid.Parse(requestedBy); err != nil {
		httpx.JSONError(w, ctx, http.StatusForbidden, "AUTHORITY_REQUIRED", "authenticated user identity is required")
		return
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	idempotencyKey := strings.TrimSpace(nonEmpty(r.Header.Get("Idempotency-Key"), body.IdempotencyKey))
	request := baseline.BuildRequest{
		TenantID: tenantID, BaselineID: baselineID, BaselineKind: strings.TrimSpace(body.BaselineKind),
		EntityType: entityType, EntityID: entityID, ExpectedRevision: body.ExpectedRevision,
		WindowStart: body.WindowStart, WindowEnd: body.WindowEnd, MinimumEligibleRows: body.MinimumEligibleRows,
		AlgorithmVersion: strings.TrimSpace(body.AlgorithmVersion), SamplePolicy: body.SamplePolicy,
		ThresholdSpec: body.ThresholdSpec, ExpectedConsumers: body.ExpectedConsumers,
		CandidateSHA256: h.baselineCandidateSHA256, IdempotencyKey: idempotencyKey,
		RequestedBy: requestedBy, Reason: strings.TrimSpace(body.Reason), TraceID: traceID,
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to begin behavior baseline transaction")
		return
	}
	defer tx.Rollback()
	receipt, err := h.baselineRepository.RequestBuildTx(ctx, tx, request)
	if err != nil {
		writeBehaviorBaselineV1Error(w, r, err)
		return
	}
	if !receipt.Replayed {
		if err := insertFusionAuditTx(ctx, tx, tenantID, requestedBy, "BEHAVIOR_BASELINE_BUILD_REQUESTED",
			"baseline", baselineID, map[string]interface{}{
				"job_id": receipt.JobID, "baseline_kind": receipt.BaselineKind,
				"definition_revision": receipt.DefinitionRevision, "target_version": receipt.TargetVersion,
				"candidate_sha256": h.baselineCandidateSHA256, "reason": request.Reason,
			}, r); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_FAILED", "behavior baseline build and outbox were rolled back")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to commit behavior baseline build")
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]interface{}{"success": true, "data": receipt})
}

func (h *SystemHandler) RequestBehaviorBaselineApprovalV1(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireBehaviorBaselineWritePermission(w, r) || !h.requirePostgres(w, ctx) {
		return
	}
	if !h.baselineV1 || h.baselineRepository == nil || len(h.baselineCandidateSHA256) != 64 {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "BASELINE_V1_DISABLED", "versioned behavior baseline authority is disabled")
		return
	}
	baselineID := strings.TrimSpace(mux.Vars(r)["id"])
	version, err := strconv.ParseInt(mux.Vars(r)["version"], 10, 64)
	if baselineID == "" || version <= 0 || err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_VERSION", "positive baseline version is required")
		return
	}
	var body behaviorBaselineApprovalV1Request
	if !decodeBaselineV1Body(w, r, &body) {
		return
	}
	tenantID, requestedBy := writeTenantID(r), strings.TrimSpace(httpx.GetUserID(ctx))
	if _, err := uuid.Parse(requestedBy); err != nil {
		httpx.JSONError(w, ctx, http.StatusForbidden, "AUTHORITY_REQUIRED", "authenticated user identity is required")
		return
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	request := baseline.ApprovalRequest{
		TenantID: tenantID, BaselineID: baselineID, BaselineVersion: version, ExpectedRevision: body.ExpectedRevision,
		CandidateSHA256: h.baselineCandidateSHA256,
		IdempotencyKey:  strings.TrimSpace(nonEmpty(r.Header.Get("Idempotency-Key"), body.IdempotencyKey)),
		RequestedBy:     requestedBy, Reason: strings.TrimSpace(body.Reason), TraceID: traceID, ExpiresAt: body.ExpiresAt,
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to begin behavior baseline approval")
		return
	}
	defer tx.Rollback()
	receipt, err := h.baselineRepository.RequestApprovalTx(ctx, tx, request, time.Now().UTC())
	if err != nil {
		writeBehaviorBaselineV1Error(w, r, err)
		return
	}
	if !receipt.Replayed {
		if err := insertFusionAuditTx(ctx, tx, tenantID, requestedBy, "BEHAVIOR_BASELINE_APPROVAL_REQUESTED",
			"baseline", baselineID, map[string]interface{}{
				"approval_id": receipt.ApprovalID, "baseline_version": version,
				"candidate_sha256": h.baselineCandidateSHA256, "expires_at": body.ExpiresAt,
			}, r); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_FAILED", "behavior baseline approval was rolled back")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to commit behavior baseline approval")
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]interface{}{"success": true, "data": receipt})
}

func (h *SystemHandler) DecideBehaviorBaselineApprovalV1(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireBehaviorBaselineWritePermission(w, r) || !h.requirePostgres(w, ctx) {
		return
	}
	if !h.baselineV1 || h.baselineRepository == nil || len(h.baselineCandidateSHA256) != 64 {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "BASELINE_V1_DISABLED", "versioned behavior baseline authority is disabled")
		return
	}
	approvalID := strings.TrimSpace(mux.Vars(r)["approval_id"])
	if _, err := uuid.Parse(approvalID); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_APPROVAL", "approval id must be a UUID")
		return
	}
	var body behaviorBaselineApprovalDecisionV1Request
	if !decodeBaselineV1Body(w, r, &body) {
		return
	}
	tenantID, decidedBy := writeTenantID(r), strings.TrimSpace(httpx.GetUserID(ctx))
	if _, err := uuid.Parse(decidedBy); err != nil {
		httpx.JSONError(w, ctx, http.StatusForbidden, "AUTHORITY_REQUIRED", "authenticated reviewer identity is required")
		return
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	decision := baseline.ApprovalDecision{TenantID: tenantID, ApprovalID: approvalID,
		ExpectedRevision: body.ExpectedRevision, CandidateSHA256: h.baselineCandidateSHA256,
		DecidedBy: decidedBy, Approve: body.Approve, Reason: strings.TrimSpace(body.Reason), TraceID: traceID}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to begin behavior baseline approval decision")
		return
	}
	defer tx.Rollback()
	receipt, err := h.baselineRepository.DecideApprovalTx(ctx, tx, decision, time.Now().UTC())
	if err != nil {
		writeBehaviorBaselineV1Error(w, r, err)
		return
	}
	auditAction := "BEHAVIOR_BASELINE_ACTIVATION_REJECTED"
	if receipt.Status == "expired" {
		auditAction = "BEHAVIOR_BASELINE_APPROVAL_EXPIRED"
	} else if body.Approve {
		auditAction = "BEHAVIOR_BASELINE_ACTIVATION_APPROVED"
	}
	if err := insertFusionAuditTx(ctx, tx, tenantID, decidedBy, auditAction, "baseline", receipt.BaselineID,
		map[string]interface{}{"approval_id": approvalID, "baseline_version": receipt.BaselineVersion,
			"candidate_sha256": h.baselineCandidateSHA256, "reason": decision.Reason,
			"expected_consumers": receipt.ExpectedConsumers}, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_FAILED", "behavior baseline approval decision was rolled back")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to commit behavior baseline approval decision")
		return
	}
	if receipt.Status == "expired" {
		httpx.JSONError(w, ctx, http.StatusConflict, "BASELINE_APPROVAL_EXPIRED", "behavior baseline approval has expired")
		return
	}
	httpx.JSONSuccess(w, ctx, receipt)
}

// RequestBehaviorBaselineRollbackV1 creates a new immutable frozen version
// from the retained previous-stable snapshot. It deliberately does not switch
// online inference: the rollback version follows the same independent
// approval and all-consumer ACK path as every other version.
func (h *SystemHandler) RequestBehaviorBaselineRollbackV1(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireBehaviorBaselineWritePermission(w, r) || !h.requirePostgres(w, ctx) {
		return
	}
	if !h.baselineV1 || h.baselineRepository == nil || len(h.baselineCandidateSHA256) != 64 {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "BASELINE_V1_DISABLED", "versioned behavior baseline authority is disabled")
		return
	}
	baselineID := strings.TrimSpace(mux.Vars(r)["id"])
	entityType, entityID := parseBaselineID(baselineID)
	if baselineID == "" || len(baselineID) > 255 || entityType == "" || entityID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_BASELINE_ID", "baseline id must be entity_type:entity_id")
		return
	}
	var body behaviorBaselineRollbackV1Request
	if !decodeBaselineV1Body(w, r, &body) {
		return
	}
	tenantID, requestedBy := writeTenantID(r), strings.TrimSpace(httpx.GetUserID(ctx))
	if _, err := uuid.Parse(requestedBy); err != nil {
		httpx.JSONError(w, ctx, http.StatusForbidden, "AUTHORITY_REQUIRED", "authenticated user identity is required")
		return
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	request := baseline.RollbackRequest{
		TenantID: tenantID, BaselineID: baselineID, TargetStableVersion: body.TargetStableVersion,
		ExpectedRevision: body.ExpectedRevision, CandidateSHA256: h.baselineCandidateSHA256,
		IdempotencyKey: strings.TrimSpace(nonEmpty(r.Header.Get("Idempotency-Key"), body.IdempotencyKey)),
		RequestedBy:    requestedBy, Reason: strings.TrimSpace(body.Reason), TraceID: traceID,
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to begin behavior baseline rollback")
		return
	}
	defer tx.Rollback()
	receipt, err := h.baselineRepository.RequestRollbackTx(ctx, tx, request)
	if err != nil {
		writeBehaviorBaselineV1Error(w, r, err)
		return
	}
	if !receipt.Replayed {
		if err := insertFusionAuditTx(ctx, tx, tenantID, requestedBy, "BEHAVIOR_BASELINE_ROLLBACK_REQUESTED",
			"baseline", baselineID, map[string]interface{}{
				"target_stable_version": receipt.TargetStableVersion, "rollback_version": receipt.RollbackVersion,
				"snapshot_sha256": receipt.SnapshotSHA256, "candidate_sha256": h.baselineCandidateSHA256,
				"reason": request.Reason,
			}, r); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_FAILED", "behavior baseline rollback was rolled back")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to commit behavior baseline rollback")
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]interface{}{"success": true, "data": receipt})
}

// EvaluateBehaviorBaselineV1 persists a version-bound inference receipt. A
// missing, stale or partial baseline is returned as an explicit disposition;
// this endpoint never substitutes compatibility-table statistics.
func (h *SystemHandler) EvaluateBehaviorBaselineV1(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireBehaviorBaselineWritePermission(w, r) || !h.requirePostgres(w, ctx) {
		return
	}
	if !h.baselineV1 || h.baselineRepository == nil || len(h.baselineCandidateSHA256) != 64 {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "BASELINE_V1_DISABLED", "versioned behavior baseline authority is disabled")
		return
	}
	baselineID := strings.TrimSpace(mux.Vars(r)["id"])
	entityType, entityID := parseBaselineID(baselineID)
	if baselineID == "" || len(baselineID) > 255 || entityType == "" || entityID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_BASELINE_ID", "baseline id must be entity_type:entity_id")
		return
	}
	var body behaviorBaselineEvaluationV1Request
	if !decodeBaselineV1Body(w, r, &body) {
		return
	}
	tenantID, requestedBy := writeTenantID(r), strings.TrimSpace(httpx.GetUserID(ctx))
	if _, err := uuid.Parse(requestedBy); err != nil {
		httpx.JSONError(w, ctx, http.StatusForbidden, "AUTHORITY_REQUIRED", "authenticated user identity is required")
		return
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	request := baseline.EvaluationRequest{
		TenantID: tenantID, BaselineID: baselineID, MetricName: strings.TrimSpace(body.MetricName),
		ObservedValue: body.ObservedValue, ObservedAt: body.ObservedAt, WindowStart: body.WindowStart,
		WindowEnd: body.WindowEnd, EvidenceRefs: body.EvidenceRefs, TraceID: traceID,
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to begin behavior baseline evaluation")
		return
	}
	defer tx.Rollback()
	receipt, err := h.baselineRepository.EvaluateTx(ctx, tx, request)
	if err != nil {
		writeBehaviorBaselineV1Error(w, r, err)
		return
	}
	if err := insertFusionAuditTx(ctx, tx, tenantID, requestedBy, "BEHAVIOR_BASELINE_EVALUATED",
		"baseline", baselineID, map[string]interface{}{
			"evaluation_id": receipt.EvaluationID, "baseline_version": receipt.BaselineVersion,
			"snapshot_sha256": receipt.SnapshotSHA256, "metric_name": receipt.MetricName,
			"disposition": receipt.Disposition, "quality_status": receipt.QualityStatus,
			"failure_code": receipt.FailureCode, "evidence_refs": receipt.EvidenceRefs,
		}, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_FAILED", "behavior baseline evaluation was rolled back")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to commit behavior baseline evaluation")
		return
	}
	httpx.JSONSuccess(w, ctx, receipt)
}

func decodeBaselineV1Body(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "one strict behavior baseline request object is required")
		return false
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "request must contain one JSON object")
		return false
	}
	return true
}

func writeBehaviorBaselineV1Error(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusInternalServerError, "INTERNAL", "behavior baseline operation failed"
	switch {
	case errors.Is(err, baseline.ErrInvalidRequest):
		status, code = http.StatusBadRequest, "INVALID_BASELINE_REQUEST"
	case errors.Is(err, baseline.ErrIdentityConflict):
		status, code = http.StatusConflict, "BASELINE_IDENTITY_CONFLICT"
	case errors.Is(err, baseline.ErrRevisionConflict):
		status, code = http.StatusConflict, "BASELINE_REVISION_CONFLICT"
	case errors.Is(err, baseline.ErrStateConflict):
		status, code = http.StatusConflict, "BASELINE_STATE_CONFLICT"
	}
	if status != http.StatusInternalServerError {
		message = err.Error()
	}
	httpx.JSONError(w, r.Context(), status, code, message)
}

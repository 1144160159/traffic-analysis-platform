package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const playbookExecutionContractVersion = 2

type PlaybookExecutionProviderRequest struct {
	ExecutionID     string                 `json:"execution_id"`
	TenantID        string                 `json:"tenant_id"`
	PlaybookName    string                 `json:"playbook_name"`
	PlaybookVersion int                    `json:"playbook_version"`
	AlertContext    map[string]interface{} `json:"alert_context"`
	Definition      map[string]interface{} `json:"definition"`
	RequestedBy     string                 `json:"requested_by"`
	ApprovedBy      string                 `json:"approved_by"`
	IdempotencyKey  string                 `json:"idempotency_key"`
}

type PlaybookStepReceipt struct {
	StepIndex         int                    `json:"step_index"`
	ActionType        string                 `json:"action_type"`
	Provider          string                 `json:"provider"`
	ProviderReceiptID string                 `json:"provider_receipt_id"`
	Status            string                 `json:"status"`
	ExternalEffect    bool                   `json:"external_effect"`
	Detail            map[string]interface{} `json:"detail,omitempty"`
}

type PlaybookExecutionProviderReceipt struct {
	Status string                 `json:"status"`
	Steps  []PlaybookStepReceipt  `json:"steps"`
	Detail map[string]interface{} `json:"detail,omitempty"`
}

type PlaybookExecutionProvider interface {
	Execute(context.Context, PlaybookExecutionProviderRequest) (PlaybookExecutionProviderReceipt, error)
	Compensate(context.Context, PlaybookExecutionProviderRequest, PlaybookExecutionProviderReceipt) (PlaybookExecutionProviderReceipt, error)
}

type playbookExecuteV2Request struct {
	ExpectedVersion *int                   `json:"expected_version"`
	Reason          string                 `json:"reason"`
	AlertContext    map[string]interface{} `json:"alert_context"`
	AlertType       string                 `json:"alert_type"`
	Severity        string                 `json:"severity"`
	Score           interface{}            `json:"score"`
	RelatedAlerts   interface{}            `json:"related_alert_count"`
	AssetRisk       string                 `json:"asset_risk"`
}

type playbookExecutionApprovalRequest struct {
	Decision         string `json:"decision"`
	ExpectedRevision *int64 `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type playbookExecutionControlRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type playbookExecutionV2Record struct {
	ExecutionID         string                 `json:"execution_id"`
	TenantID            string                 `json:"tenant_id"`
	PlaybookName        string                 `json:"playbook_name"`
	PlaybookVersion     int                    `json:"playbook_version"`
	AlertID             string                 `json:"alert_id"`
	Mode                string                 `json:"mode"`
	Status              string                 `json:"status"`
	ApprovalStatus      string                 `json:"approval_status"`
	ExecutorStatus      string                 `json:"executor_status"`
	WorkflowRevision    int64                  `json:"workflow_revision"`
	Request             map[string]interface{} `json:"request"`
	ExecutionReceipt    map[string]interface{} `json:"execution_receipt"`
	CompensationReceipt map[string]interface{} `json:"compensation_receipt"`
	ErrorMessage        string                 `json:"error_message"`
	Attempts            int                    `json:"attempts"`
	LeaseOwner          string                 `json:"-"`
	RequestedBy         string                 `json:"requested_by"`
	ApprovedBy          string                 `json:"approved_by"`
	Reason              string                 `json:"reason"`
	TraceID             string                 `json:"trace_id"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
	CompletedAt         *time.Time             `json:"completed_at,omitempty"`
	IdempotentReuse     bool                   `json:"idempotent_reuse,omitempty"`
	RequestSHA256       string                 `json:"-"`
}

func (h *AdvancedHandler) SetPlaybookExecutionV2FeatureFlag(enabled bool) {
	h.playbookExecutionV2 = enabled
}

func (h *AdvancedHandler) SetPlaybookExecutionProvider(provider PlaybookExecutionProvider) {
	h.playbookExecutionProvider = provider
}

func (h *AdvancedHandler) StartPlaybookExecutionWorker(ctx context.Context, interval time.Duration) error {
	if !h.playbookExecutionV2 || h.playbookExecutionProvider == nil {
		return nil
	}
	if h.advancedRepo == nil || h.advancedRepo.db == nil {
		return fmt.Errorf("playbook execution persistence is unavailable")
	}
	if err := verifyPlaybookExecutionV2Schema(ctx, h.advancedRepo.db); err != nil {
		return err
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := h.processNextPlaybookExecution(ctx); err != nil && ctx.Err() == nil && h.advancedRepo.logger != nil {
				h.advancedRepo.logger.Warn("Failed to process playbook execution")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (h *AdvancedHandler) processNextPlaybookExecution(ctx context.Context) error {
	if h.playbookExecutionProvider == nil || h.advancedRepo == nil || h.advancedRepo.db == nil {
		return nil
	}
	workerID := "playbook-worker-" + uuid.NewString()
	tx, err := h.advancedRepo.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var executionID, tenantID string
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
		SELECT execution_id FROM alert_playbook_executions
		WHERE mode='live' AND (
		  (status IN ('approved_awaiting_executor','compensation_queued') AND next_attempt_at<=now()) OR
		  (status IN ('running','compensating') AND locked_until<now())
		) ORDER BY created_at,execution_id LIMIT 1 FOR UPDATE SKIP LOCKED
	)
	UPDATE alert_playbook_executions e SET
		status=CASE WHEN e.status IN ('compensation_queued','compensating') THEN 'compensating' ELSE 'running' END,
		executor_status=CASE WHEN e.status IN ('compensation_queued','compensating') THEN 'compensating' ELSE 'running' END,
		attempts=e.attempts+1,locked_until=now()+interval '5 minutes',locked_by=$1,updated_at=now()
	FROM candidate c WHERE e.execution_id=c.execution_id RETURNING e.execution_id,e.tenant_id`, workerID).Scan(&executionID, &tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	record, _, err := loadPlaybookExecutionByID(ctx, tx, tenantID, executionID, false)
	if err != nil {
		return err
	}
	definition, err := loadPlaybookDefinitionTx(ctx, tx, record.TenantID, record.PlaybookName)
	if err != nil {
		return err
	}
	definitionInvalid := definition.Version != record.PlaybookVersion || definition.Stage != "approved" || !definition.Enabled
	if err := tx.Commit(); err != nil {
		return err
	}
	if definitionInvalid {
		return h.failPlaybookExecutionAttempt(ctx, workerID, record, "definition revision is no longer approved")
	}
	providerRequest := PlaybookExecutionProviderRequest{
		ExecutionID: executionID, TenantID: record.TenantID, PlaybookName: record.PlaybookName,
		PlaybookVersion: record.PlaybookVersion, AlertContext: playbookExecutionAlertContext(record.Request),
		Definition: definition.Definition, RequestedBy: record.RequestedBy, ApprovedBy: record.ApprovedBy,
	}
	phase := "execute"
	providerRequest.IdempotencyKey = executionID + ":execute"
	var receipt PlaybookExecutionProviderReceipt
	if record.Status == "compensating" {
		phase = "compensate"
		providerRequest.IdempotencyKey = executionID + ":compensate"
		prior, priorErr := playbookProviderReceiptFromMap(record.ExecutionReceipt)
		if priorErr != nil {
			return h.failPlaybookExecutionAttempt(ctx, workerID, record, priorErr.Error())
		}
		receipt, err = h.playbookExecutionProvider.Compensate(ctx, providerRequest, prior)
	} else {
		receipt, err = h.playbookExecutionProvider.Execute(ctx, providerRequest)
	}
	if err != nil {
		return h.failPlaybookExecutionAttempt(ctx, workerID, record, err.Error())
	}
	receipt, err = normalizePlaybookProviderReceipt(receipt, definition.Definition, phase, record.ExecutionReceipt)
	if err != nil {
		return h.failPlaybookExecutionAttempt(ctx, workerID, record, err.Error())
	}
	return h.completePlaybookExecutionAttempt(ctx, workerID, record, phase, receipt)
}

func (h *AdvancedHandler) completePlaybookExecutionAttempt(ctx context.Context, workerID string, leased playbookExecutionV2Record, phase string, receipt PlaybookExecutionProviderReceipt) error {
	tx, err := h.advancedRepo.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	record, found, err := loadPlaybookExecutionByID(ctx, tx, leased.TenantID, leased.ExecutionID, true)
	if err != nil {
		return err
	}
	if !found {
		return sql.ErrNoRows
	}
	if record.LockedBy() != workerID || (record.Status != "running" && record.Status != "compensating") {
		return fmt.Errorf("playbook execution lease is no longer owned by this worker")
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	for _, step := range receipt.Steps {
		stepJSON, _ := json.Marshal(step)
		digest := sha256.Sum256(stepJSON)
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_playbook_step_receipts
			(receipt_id,execution_id,tenant_id,playbook_name,phase,attempt,step_index,action_type,
			 provider,provider_receipt_id,status,external_effect,payload,payload_sha256)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14)`, uuid.NewString(),
			record.ExecutionID, record.TenantID, record.PlaybookName, phase, record.Attempts,
			step.StepIndex, step.ActionType, step.Provider, step.ProviderReceiptID, step.Status,
			step.ExternalEffect, string(stepJSON), hex.EncodeToString(digest[:])); err != nil {
			return err
		}
	}
	nextStatus, executorStatus, eventType := playbookExecutionTerminalState(phase, receipt.Status)
	nextRevision := record.WorkflowRevision + 1
	completedAt := time.Now().UTC()
	resultPayload, _ := json.Marshal(map[string]interface{}{
		"receipt": receipt, "final_effect": playbookReceiptHasExternalEffect(receipt), "error_message": "",
	})
	executionReceipt, compensationReceipt := record.ExecutionReceipt, record.CompensationReceipt
	if phase == "execute" {
		_ = json.Unmarshal(receiptJSON, &executionReceipt)
	} else {
		_ = json.Unmarshal(receiptJSON, &compensationReceipt)
	}
	executionJSON, _ := json.Marshal(executionReceipt)
	compensationJSON, _ := json.Marshal(compensationReceipt)
	succeeded, failed := playbookReceiptCounts(receipt)
	result, err := tx.ExecContext(ctx, `UPDATE alert_playbook_executions SET status=$3,executor_status=$4,
		workflow_revision=$5,result_payload=$6::jsonb,execution_receipt=$7::jsonb,
		compensation_receipt=$8::jsonb,success_actions=$9,failed_actions=$10,
		locked_until=NULL,locked_by='',updated_at=now(),completed_at=$11
		WHERE tenant_id=$1 AND execution_id=$2 AND locked_by=$12 AND status IN ('running','compensating')`,
		record.TenantID, record.ExecutionID, nextStatus, executorStatus, nextRevision,
		string(resultPayload), string(executionJSON), string(compensationJSON), succeeded, failed,
		completedAt, workerID)
	if err != nil || affectedRows(result) != 1 {
		return fmt.Errorf("playbook execution changed before terminal receipt commit")
	}
	if err := insertPlaybookExecutionOutboxTx(ctx, tx, record.ExecutionID, record.TenantID,
		record.PlaybookName, eventType, nextRevision, record.TraceID, map[string]interface{}{
			"status": nextStatus, "executor_status": executorStatus, "receipt": receipt,
		}); err != nil {
		return err
	}
	if err := insertPlaybookAuditTx(ctx, tx, nil, record.TenantID, "playbook-executor",
		playbookExecutionTerminalAudit(phase, receipt.Status), record.ExecutionID,
		map[string]interface{}{"playbook": record.PlaybookName, "revision": nextRevision, "receipt": receipt}); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *AdvancedHandler) failPlaybookExecutionAttempt(ctx context.Context, workerID string, leased playbookExecutionV2Record, cause string) error {
	tx, err := h.advancedRepo.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	record, found, err := loadPlaybookExecutionByID(ctx, tx, leased.TenantID, leased.ExecutionID, true)
	if err != nil {
		return err
	}
	if !found {
		return sql.ErrNoRows
	}
	if record.LockedBy() != workerID {
		return fmt.Errorf("playbook execution lease is no longer owned by this worker")
	}
	phase := "execute"
	queuedStatus, terminalStatus, terminalExecutor, terminalEvent := "approved_awaiting_executor", "failed", "failed", "traffic.playbook.v2.ExecutionFailed"
	if record.Status == "compensating" {
		phase, queuedStatus, terminalStatus, terminalExecutor, terminalEvent = "compensate", "compensation_queued", "compensation_failed", "compensation_failed", "traffic.playbook.v2.CompensationFailed"
	}
	resultPayload, _ := json.Marshal(map[string]interface{}{"error_message": cause, "final_effect": false})
	if record.Attempts < 5 {
		delay := time.Duration(1<<playbookMinInt(record.Attempts, 6)) * time.Second
		result, err := tx.ExecContext(ctx, `UPDATE alert_playbook_executions SET status=$3,executor_status='queued',
			result_payload=$4::jsonb,next_attempt_at=$5,locked_until=NULL,locked_by='',updated_at=now()
			WHERE tenant_id=$1 AND execution_id=$2 AND locked_by=$6`, record.TenantID, record.ExecutionID,
			queuedStatus, string(resultPayload), time.Now().UTC().Add(delay), workerID)
		if err != nil || affectedRows(result) != 1 {
			return fmt.Errorf("playbook execution changed before retry scheduling")
		}
		return tx.Commit()
	}
	nextRevision := record.WorkflowRevision + 1
	result, err := tx.ExecContext(ctx, `UPDATE alert_playbook_executions SET status=$3,executor_status=$4,
		workflow_revision=$5,result_payload=$6::jsonb,locked_until=NULL,locked_by='',updated_at=now(),completed_at=now()
		WHERE tenant_id=$1 AND execution_id=$2 AND locked_by=$7`, record.TenantID, record.ExecutionID,
		terminalStatus, terminalExecutor, nextRevision, string(resultPayload), workerID)
	if err != nil || affectedRows(result) != 1 {
		return fmt.Errorf("playbook execution changed before terminal failure commit")
	}
	if err := insertPlaybookExecutionOutboxTx(ctx, tx, record.ExecutionID, record.TenantID,
		record.PlaybookName, terminalEvent, nextRevision, record.TraceID,
		map[string]interface{}{"status": terminalStatus, "phase": phase, "error": cause}); err != nil {
		return err
	}
	if err := insertPlaybookAuditTx(ctx, tx, nil, record.TenantID, "playbook-executor",
		"PLAYBOOK_EXECUTION_FAILED", record.ExecutionID,
		map[string]interface{}{"playbook": record.PlaybookName, "phase": phase, "revision": nextRevision, "error": cause}); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *AdvancedHandler) executePlaybookV2(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePlaybookExecutePermission(w, r) {
		return
	}
	if !h.playbookExecutionV2 {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PLAYBOOK_EXECUTION_V2_DISABLED", "live playbook execution is disabled; use /drill for simulated validation")
		return
	}
	if h.advancedRepo == nil || h.advancedRepo.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PLAYBOOK_REPOSITORY_UNAVAILABLE", "playbook repository is not available")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain 16 to 200 characters")
		return
	}
	var request playbookExecuteV2Request
	if !decodePlaybookJSON(w, r, &request) {
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if request.ExpectedVersion == nil || *request.ExpectedVersion <= 0 || len([]rune(request.Reason)) < 8 || len([]rune(request.Reason)) > 1000 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "positive expected_version and reason (8 to 1000 characters) are required")
		return
	}
	request.AlertContext = normalizedPlaybookAlertContext(request)
	if strings.TrimSpace(playbookAlertID(request.AlertContext, "")) == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "ALERT_ID_REQUIRED", "alert_context.alert_id is required for live playbook execution")
		return
	}
	name := strings.TrimSpace(mux.Vars(r)["name"])
	requestSHA, err := playbookExecutionRequestSHA(name, request)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "playbook execution request is not serializable")
		return
	}
	tx, err := h.advancedRepo.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin playbook execution transaction")
		return
	}
	defer tx.Rollback()
	if existing, found, loadErr := loadPlaybookExecutionByIdempotency(ctx, tx, tenantIDFromRequest(r), idempotencyKey, false); loadErr != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to resolve playbook execution idempotency")
		return
	} else if found {
		if existing.PlaybookName != name || existing.PlaybookVersion != *request.ExpectedVersion || existing.RequestSHA256 != requestSHA {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different playbook execution")
			return
		}
		existing.IdempotentReuse = true
		if err := tx.Commit(); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit idempotent playbook execution read")
			return
		}
		writePlaybookExecutionAccepted(w, ctx, existing)
		return
	}
	definition, err := loadPlaybookDefinitionForUpdateTx(ctx, tx, tenantIDFromRequest(r), name)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.JSONError(w, ctx, http.StatusNotFound, "PLAYBOOK_NOT_FOUND", "playbook not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to lock playbook definition")
		return
	}
	if definition.Version != *request.ExpectedVersion {
		httpx.JSONError(w, ctx, http.StatusConflict, "PLAYBOOK_VERSION_CONFLICT", fmt.Sprintf("expected version %d but current version is %d", *request.ExpectedVersion, definition.Version))
		return
	}
	if definition.Stage != "approved" || !definition.Enabled {
		httpx.JSONError(w, ctx, http.StatusConflict, "PLAYBOOK_NOT_EXECUTABLE", "only an approved and enabled playbook revision may execute")
		return
	}
	decodedDefinition, err := DecodePlaybookDefinition(definition)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusConflict, "PLAYBOOK_DEFINITION_INVALID", "approved playbook definition cannot be decoded")
		return
	}
	if err := enforcePlaybookExecutionBudget(ctx, tx, tenantIDFromRequest(r), name, definition.Version, decodedDefinition.MaxRuns, decodedDefinition.Cooldown); err != nil {
		var budgetErr *playbookExecutionBudgetError
		if errors.As(err, &budgetErr) {
			httpx.JSONError(w, ctx, http.StatusConflict, budgetErr.Code, budgetErr.Error())
		} else {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to verify playbook execution budget")
		}
		return
	}
	approvalRequired := playbookExecutionApprovalRequired(definition)
	status, approvalStatus := "approved_awaiting_executor", "not_required"
	executorStatus := "queued"
	if approvalRequired {
		status, approvalStatus, executorStatus = "pending_approval", "pending", "not_dispatched"
	} else if h.playbookExecutionProvider == nil {
		executorStatus = "not_configured"
	}
	executionID := "playbook-execution-" + uuid.NewString()
	actor := playbookActor(ctx)
	alertID := playbookAlertID(request.AlertContext, executionID)
	requestPayload := map[string]interface{}{
		"playbook_name": name, "playbook_version": definition.Version,
		"alert_context": request.AlertContext, "definition": definition.Definition,
	}
	requestJSON, _ := json.Marshal(requestPayload)
	traceID := httpx.GetTraceID(ctx)
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_playbook_executions
		(execution_id,tenant_id,playbook_name,alert_id,request_payload,result_payload,mode,status,
		 effect_payload,requested_by,playbook_version,workflow_revision,approval_status,executor_status,
		 idempotency_key,request_sha256,reason,trace_id,next_attempt_at,updated_at)
		VALUES ($1,$2,$3,$4,$5::jsonb,'{}'::jsonb,'live',$6,'{}'::jsonb,$7,$8,1,$9,$10,$11,$12,$13,$14,now(),now())`,
		executionID, tenantIDFromRequest(r), name, alertID, string(requestJSON), status, actor,
		definition.Version, approvalStatus, executorStatus, idempotencyKey, requestSHA, request.Reason, traceID); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist playbook execution request")
		return
	}
	eventType := "traffic.playbook.v2.ExecutionRequested"
	if !approvalRequired {
		eventType = "traffic.playbook.v2.ExecutionApproved"
	}
	if err := insertPlaybookExecutionOutboxTx(ctx, tx, executionID, tenantIDFromRequest(r), name, eventType, 1, traceID, map[string]interface{}{
		"status": status, "approval_status": approvalStatus, "executor_status": executorStatus,
		"playbook_version": definition.Version, "alert_id": alertID, "requested_by": actor,
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist playbook execution outbox")
		return
	}
	if err := insertPlaybookAuditTx(ctx, tx, r, tenantIDFromRequest(r), actor, "PLAYBOOK_EXECUTION_REQUESTED", executionID, map[string]interface{}{
		"playbook": name, "version": definition.Version, "status": status,
		"approval_required": approvalRequired, "idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit playbook execution request")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit playbook execution request")
		return
	}
	record, err := loadPlaybookExecution(ctx, h.advancedRepo.db, tenantIDFromRequest(r), executionID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load accepted playbook execution")
		return
	}
	writePlaybookExecutionAccepted(w, ctx, record)
}

func (h *AdvancedHandler) GetPlaybookExecutionV2(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookReadPermission(w, r) {
		return
	}
	if h.advancedRepo == nil || h.advancedRepo.db == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_REPOSITORY_UNAVAILABLE", "playbook repository is not available")
		return
	}
	record, err := loadPlaybookExecution(r.Context(), h.advancedRepo.db, tenantIDFromRequest(r), strings.TrimSpace(mux.Vars(r)["execution_id"]))
	if errors.Is(err, sql.ErrNoRows) {
		httpx.JSONError(w, r.Context(), http.StatusNotFound, "PLAYBOOK_EXECUTION_NOT_FOUND", "playbook execution not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load playbook execution")
		return
	}
	httpx.JSONContractSuccess(w, r.Context(), record, playbookExecutionMeta(record))
}

func (h *AdvancedHandler) DecidePlaybookExecutionV2(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookApprovePermission(w, r) {
		return
	}
	var request playbookExecutionApprovalRequest
	if !decodePlaybookJSON(w, r, &request) {
		return
	}
	request.Decision = strings.ToLower(strings.TrimSpace(request.Decision))
	request.Reason = strings.TrimSpace(request.Reason)
	if (request.Decision != "approve" && request.Decision != "reject") || !validPlaybookWorkflowRequest(request.ExpectedRevision, request.Reason) {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "decision approve/reject, positive expected_revision and reason (8 to 1000 characters) are required")
		return
	}
	h.applyPlaybookExecutionDecision(w, r, request)
}

func (h *AdvancedHandler) CancelPlaybookExecutionV2(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookExecutePermission(w, r) {
		return
	}
	h.applyPlaybookExecutionControl(w, r, "cancel")
}

func (h *AdvancedHandler) CompensatePlaybookExecutionV2(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlaybookApprovePermission(w, r) {
		return
	}
	h.applyPlaybookExecutionControl(w, r, "compensate")
}

func (h *AdvancedHandler) applyPlaybookExecutionDecision(w http.ResponseWriter, r *http.Request, request playbookExecutionApprovalRequest) {
	ctx := r.Context()
	identity, ok := h.playbookExecutionControlIdentity(w, r)
	if !ok {
		return
	}
	tx, err := h.advancedRepo.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin playbook approval transaction")
		return
	}
	defer tx.Rollback()
	var replayExecution, replayDecision, replayReason, replayActor string
	var replayRevision int64
	err = tx.QueryRowContext(ctx, `SELECT execution_id,decision,expected_revision,reason,decided_by
		FROM alert_playbook_execution_approvals WHERE tenant_id=$1 AND idempotency_key=$2`,
		identity.tenantID, identity.idempotencyKey).Scan(&replayExecution, &replayDecision, &replayRevision, &replayReason, &replayActor)
	if err == nil {
		if replayExecution != identity.executionID || replayDecision != request.Decision ||
			replayRevision != *request.ExpectedRevision || replayReason != request.Reason || replayActor != identity.actor {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was used for a different playbook approval")
			return
		}
		replayed, _, loadErr := loadPlaybookExecutionByID(ctx, tx, identity.tenantID, identity.executionID, false)
		if loadErr != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load idempotent playbook approval")
			return
		}
		replayed.IdempotentReuse = true
		if err := tx.Commit(); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit idempotent playbook approval read")
			return
		}
		writePlaybookExecutionAccepted(w, ctx, replayed)
		return
	}
	if err != sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to resolve playbook approval idempotency")
		return
	}
	record, found, err := loadPlaybookExecutionByID(ctx, tx, identity.tenantID, identity.executionID, true)
	if err != nil || !found {
		if !found {
			httpx.JSONError(w, ctx, http.StatusNotFound, "PLAYBOOK_EXECUTION_NOT_FOUND", "playbook execution not found")
		} else {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to lock playbook execution")
		}
		return
	}
	if identity.actor == record.RequestedBy {
		httpx.JSONError(w, ctx, http.StatusForbidden, "INDEPENDENT_APPROVER_REQUIRED", "the requester cannot approve or reject the same playbook execution")
		return
	}
	if record.Status != "pending_approval" || record.ApprovalStatus != "pending" {
		httpx.JSONError(w, ctx, http.StatusConflict, "INVALID_STATE", "only a pending playbook execution can be approved or rejected")
		return
	}
	if record.WorkflowRevision != *request.ExpectedRevision {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", fmt.Sprintf("expected revision %d but current revision is %d", *request.ExpectedRevision, record.WorkflowRevision))
		return
	}
	nextStatus, approvalStatus, executorStatus, eventType := "approved_awaiting_executor", "approved", "queued", "traffic.playbook.v2.ExecutionApproved"
	if h.playbookExecutionProvider == nil {
		executorStatus = "not_configured"
	}
	if request.Decision == "reject" {
		nextStatus, approvalStatus, executorStatus, eventType = "cancelled", "rejected", "cancelled", "traffic.playbook.v2.ExecutionRejected"
	}
	nextRevision := record.WorkflowRevision + 1
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_playbook_execution_approvals
		(approval_id,execution_id,tenant_id,playbook_name,decision,expected_revision,idempotency_key,
		 reason,decided_by,resulting_revision,resulting_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, uuid.NewString(), identity.executionID,
		identity.tenantID, record.PlaybookName, request.Decision, *request.ExpectedRevision,
		identity.idempotencyKey, request.Reason, identity.actor, nextRevision, nextStatus); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist playbook approval")
		return
	}
	result, err := tx.ExecContext(ctx, `UPDATE alert_playbook_executions SET status=$3,approval_status=$4,
		executor_status=$5,workflow_revision=$6,approved_by=CASE WHEN $4='approved' THEN $7 ELSE approved_by END,
		approved_at=CASE WHEN $4='approved' THEN now() ELSE approved_at END,updated_at=now(),
		completed_at=CASE WHEN $3='cancelled' THEN now() ELSE NULL END
		WHERE tenant_id=$1 AND execution_id=$2 AND workflow_revision=$8 AND status='pending_approval'`,
		identity.tenantID, identity.executionID, nextStatus, approvalStatus, executorStatus,
		nextRevision, identity.actor, *request.ExpectedRevision)
	if err != nil || affectedRows(result) != 1 {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "playbook execution changed while approval was applied")
		return
	}
	if err := insertPlaybookExecutionOutboxTx(ctx, tx, identity.executionID, identity.tenantID,
		record.PlaybookName, eventType, nextRevision, record.TraceID, map[string]interface{}{
			"decision": request.Decision, "status": nextStatus, "approval_status": approvalStatus,
			"executor_status": executorStatus, "approved_by": identity.actor,
		}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist playbook approval outbox")
		return
	}
	auditAction := "PLAYBOOK_EXECUTION_APPROVED"
	if request.Decision == "reject" {
		auditAction = "PLAYBOOK_EXECUTION_REJECTED"
	}
	if err := insertPlaybookAuditTx(ctx, tx, r, identity.tenantID, identity.actor,
		auditAction, identity.executionID,
		map[string]interface{}{"playbook": record.PlaybookName, "revision": nextRevision, "reason": request.Reason}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit playbook approval")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit playbook approval")
		return
	}
	updated, err := loadPlaybookExecution(ctx, h.advancedRepo.db, identity.tenantID, identity.executionID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to reload playbook approval result")
		return
	}
	writePlaybookExecutionAccepted(w, ctx, updated)
}

func (h *AdvancedHandler) applyPlaybookExecutionControl(w http.ResponseWriter, r *http.Request, operation string) {
	ctx := r.Context()
	identity, ok := h.playbookExecutionControlIdentity(w, r)
	if !ok {
		return
	}
	var request playbookExecutionControlRequest
	if !decodePlaybookJSON(w, r, &request) {
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if !validPlaybookWorkflowRequest(request.ExpectedRevision, request.Reason) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "positive expected_revision and reason (8 to 1000 characters) are required")
		return
	}
	tx, err := h.advancedRepo.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin playbook control transaction")
		return
	}
	defer tx.Rollback()
	var replayExecution, replayOperation, replayReason, replayActor string
	var replayRevision int64
	err = tx.QueryRowContext(ctx, `SELECT execution_id,operation,expected_revision,reason,requested_by
		FROM alert_playbook_execution_controls WHERE tenant_id=$1 AND idempotency_key=$2`,
		identity.tenantID, identity.idempotencyKey).Scan(&replayExecution, &replayOperation, &replayRevision, &replayReason, &replayActor)
	if err == nil {
		if replayExecution != identity.executionID || replayOperation != operation ||
			replayRevision != *request.ExpectedRevision || replayReason != request.Reason || replayActor != identity.actor {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was used for a different playbook control request")
			return
		}
		replayed, _, loadErr := loadPlaybookExecutionByID(ctx, tx, identity.tenantID, identity.executionID, false)
		if loadErr != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load idempotent playbook control result")
			return
		}
		replayed.IdempotentReuse = true
		if err := tx.Commit(); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit idempotent playbook control read")
			return
		}
		writePlaybookExecutionAccepted(w, ctx, replayed)
		return
	}
	if err != sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to resolve playbook control idempotency")
		return
	}
	record, found, err := loadPlaybookExecutionByID(ctx, tx, identity.tenantID, identity.executionID, true)
	if err != nil || !found {
		if !found {
			httpx.JSONError(w, ctx, http.StatusNotFound, "PLAYBOOK_EXECUTION_NOT_FOUND", "playbook execution not found")
		} else {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to lock playbook execution")
		}
		return
	}
	if record.WorkflowRevision != *request.ExpectedRevision {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", fmt.Sprintf("expected revision %d but current revision is %d", *request.ExpectedRevision, record.WorkflowRevision))
		return
	}
	if operation == "compensate" && identity.actor == record.RequestedBy {
		httpx.JSONError(w, ctx, http.StatusForbidden, "INDEPENDENT_APPROVER_REQUIRED", "the requester cannot approve compensation for the same playbook execution")
		return
	}
	if operation == "cancel" && record.Status != "pending_approval" && record.Status != "approved_awaiting_executor" {
		httpx.JSONError(w, ctx, http.StatusConflict, "INVALID_STATE", "only an undispatched playbook execution can be cancelled")
		return
	}
	if operation == "compensate" && record.Status != "completed" && record.Status != "partial" {
		httpx.JSONError(w, ctx, http.StatusConflict, "INVALID_STATE", "only a completed or partial playbook execution can be compensated")
		return
	}
	if operation == "compensate" && len(record.ExecutionReceipt) == 0 {
		httpx.JSONError(w, ctx, http.StatusConflict, "EXECUTION_RECEIPT_REQUIRED", "compensation requires a durable execution receipt")
		return
	}
	nextStatus, approvalStatus, executorStatus, eventType := "cancelled", "cancelled", "cancelled", "traffic.playbook.v2.ExecutionCancelled"
	if operation == "compensate" {
		nextStatus, approvalStatus, executorStatus, eventType = "compensation_queued", record.ApprovalStatus, "compensating", "traffic.playbook.v2.CompensationRequested"
		if h.playbookExecutionProvider == nil {
			executorStatus = "not_configured"
		}
	}
	nextRevision := record.WorkflowRevision + 1
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_playbook_execution_controls
		(request_id,execution_id,tenant_id,playbook_name,operation,expected_revision,idempotency_key,
		 reason,requested_by,resulting_revision,resulting_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, uuid.NewString(), identity.executionID,
		identity.tenantID, record.PlaybookName, operation, *request.ExpectedRevision,
		identity.idempotencyKey, request.Reason, identity.actor, nextRevision, nextStatus); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist playbook control request")
		return
	}
	result, err := tx.ExecContext(ctx, `UPDATE alert_playbook_executions SET status=$3,approval_status=$4,
		executor_status=$5,workflow_revision=$6,updated_at=now(),completed_at=CASE WHEN $3='cancelled' THEN now() ELSE NULL END
		WHERE tenant_id=$1 AND execution_id=$2 AND workflow_revision=$7`, identity.tenantID,
		identity.executionID, nextStatus, approvalStatus, executorStatus, nextRevision, *request.ExpectedRevision)
	if err != nil || affectedRows(result) != 1 {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "playbook execution changed while control was applied")
		return
	}
	if err := insertPlaybookExecutionOutboxTx(ctx, tx, identity.executionID, identity.tenantID,
		record.PlaybookName, eventType, nextRevision, record.TraceID, map[string]interface{}{
			"operation": operation, "status": nextStatus, "executor_status": executorStatus,
			"requested_by": identity.actor,
		}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist playbook control outbox")
		return
	}
	auditAction := "PLAYBOOK_EXECUTION_CANCELLED"
	if operation == "compensate" {
		auditAction = "PLAYBOOK_COMPENSATION_REQUESTED"
	}
	if err := insertPlaybookAuditTx(ctx, tx, r, identity.tenantID, identity.actor,
		auditAction, identity.executionID,
		map[string]interface{}{"playbook": record.PlaybookName, "revision": nextRevision, "reason": request.Reason}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit playbook control")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit playbook control")
		return
	}
	updated, err := loadPlaybookExecution(ctx, h.advancedRepo.db, identity.tenantID, identity.executionID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to reload playbook control result")
		return
	}
	writePlaybookExecutionAccepted(w, ctx, updated)
}

type playbookExecutionIdentity struct {
	tenantID, executionID, actor, idempotencyKey string
}

func (h *AdvancedHandler) playbookExecutionControlIdentity(w http.ResponseWriter, r *http.Request) (playbookExecutionIdentity, bool) {
	if h.advancedRepo == nil || h.advancedRepo.db == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "PLAYBOOK_REPOSITORY_UNAVAILABLE", "playbook repository is not available")
		return playbookExecutionIdentity{}, false
	}
	identity := playbookExecutionIdentity{
		tenantID: tenantIDFromRequest(r), executionID: strings.TrimSpace(mux.Vars(r)["execution_id"]),
		actor: playbookActor(r.Context()), idempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	}
	if identity.tenantID == "" || identity.executionID == "" || identity.actor == "" || len(identity.idempotencyKey) < 16 || len(identity.idempotencyKey) > 200 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "WORKFLOW_IDENTITY_REQUIRED", "tenant, execution, actor and Idempotency-Key are required")
		return playbookExecutionIdentity{}, false
	}
	return identity, true
}

func loadPlaybookDefinitionTx(ctx context.Context, tx *sql.Tx, tenantID, name string) (PlaybookDefinitionRecord, error) {
	return scanPlaybookDefinition(tx.QueryRowContext(ctx, `SELECT tenant_id,name,display_name,description,version,stage,
		enabled,risk_level,definition_payload,created_by,submitted_by,approved_by,rejection_reason,created_at,updated_at
		FROM alert_playbook_definitions WHERE tenant_id=$1 AND name=$2 FOR SHARE`, tenantID, name))
}

func loadPlaybookDefinitionForUpdateTx(ctx context.Context, tx *sql.Tx, tenantID, name string) (PlaybookDefinitionRecord, error) {
	return scanPlaybookDefinition(tx.QueryRowContext(ctx, `SELECT tenant_id,name,display_name,description,version,stage,
		enabled,risk_level,definition_payload,created_by,submitted_by,approved_by,rejection_reason,created_at,updated_at
		FROM alert_playbook_definitions WHERE tenant_id=$1 AND name=$2 FOR UPDATE`, tenantID, name))
}

type playbookExecutionBudgetError struct {
	Code    string
	Message string
}

func (err *playbookExecutionBudgetError) Error() string {
	return err.Message
}

// enforcePlaybookExecutionBudget runs while the exact definition row is held
// FOR UPDATE. This serializes requests for one tenant/playbook revision so
// concurrent idempotency keys cannot overrun max_runs or the cooldown window.
// Rejected/cancelled requests never reached an external executor and therefore
// do not consume the durable execution budget.
func enforcePlaybookExecutionBudget(
	ctx context.Context,
	tx *sql.Tx,
	tenantID, name string,
	version, maxRuns int,
	cooldown time.Duration,
) error {
	var acceptedRuns int
	var latest sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT count(*),max(created_at)
		FROM alert_playbook_executions
		WHERE tenant_id=$1 AND playbook_name=$2 AND playbook_version=$3 AND mode='live'
		  AND status<>'cancelled'`, tenantID, name, version).Scan(&acceptedRuns, &latest); err != nil {
		return err
	}
	if maxRuns > 0 && acceptedRuns >= maxRuns {
		return &playbookExecutionBudgetError{
			Code:    "PLAYBOOK_RUN_LIMIT_REACHED",
			Message: fmt.Sprintf("playbook version %d reached max_runs=%d", version, maxRuns),
		}
	}
	if cooldown > 0 && latest.Valid {
		nextAllowed := latest.Time.Add(cooldown)
		if time.Now().UTC().Before(nextAllowed) {
			return &playbookExecutionBudgetError{
				Code:    "PLAYBOOK_COOLDOWN_ACTIVE",
				Message: fmt.Sprintf("playbook cooldown remains active until %s", nextAllowed.UTC().Format(time.RFC3339)),
			}
		}
	}
	return nil
}

func loadPlaybookExecution(ctx context.Context, db *sql.DB, tenantID, executionID string) (playbookExecutionV2Record, error) {
	record, _, err := loadPlaybookExecutionByID(ctx, db, tenantID, executionID, false)
	return record, err
}

type playbookExecutionQuerier interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func loadPlaybookExecutionByID(ctx context.Context, query playbookExecutionQuerier, tenantID, executionID string, lock bool) (playbookExecutionV2Record, bool, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	return scanPlaybookExecutionV2(query.QueryRowContext(ctx, playbookExecutionSelect+` WHERE tenant_id=$1 AND execution_id=$2`+suffix, tenantID, executionID))
}

func loadPlaybookExecutionByIdempotency(ctx context.Context, query playbookExecutionQuerier, tenantID, key string, lock bool) (playbookExecutionV2Record, bool, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	record, found, err := scanPlaybookExecutionV2(query.QueryRowContext(ctx, playbookExecutionSelect+` WHERE tenant_id=$1 AND idempotency_key=$2`+suffix, tenantID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return record, false, nil
	}
	return record, found, err
}

const playbookExecutionSelect = `SELECT execution_id,tenant_id,playbook_name,playbook_version,alert_id,status,
	approval_status,executor_status,workflow_revision,request_payload,execution_receipt,compensation_receipt,
	COALESCE((result_payload->>'error_message'),''),attempts,locked_by,requested_by,approved_by,reason,trace_id,
	created_at,updated_at,completed_at,request_sha256 FROM alert_playbook_executions`

func scanPlaybookExecutionV2(row *sql.Row) (playbookExecutionV2Record, bool, error) {
	var record playbookExecutionV2Record
	var requestJSON, executionJSON, compensationJSON []byte
	var completedAt sql.NullTime
	err := row.Scan(&record.ExecutionID, &record.TenantID, &record.PlaybookName, &record.PlaybookVersion,
		&record.AlertID, &record.Status, &record.ApprovalStatus, &record.ExecutorStatus,
		&record.WorkflowRevision, &requestJSON, &executionJSON, &compensationJSON, &record.ErrorMessage,
		&record.Attempts, &record.LeaseOwner, &record.RequestedBy, &record.ApprovedBy, &record.Reason, &record.TraceID,
		&record.CreatedAt, &record.UpdatedAt, &completedAt, &record.RequestSHA256)
	if errors.Is(err, sql.ErrNoRows) {
		return record, false, sql.ErrNoRows
	}
	if err != nil {
		return record, false, err
	}
	if err := json.Unmarshal(requestJSON, &record.Request); err != nil {
		return record, false, err
	}
	_ = json.Unmarshal(executionJSON, &record.ExecutionReceipt)
	_ = json.Unmarshal(compensationJSON, &record.CompensationReceipt)
	if record.ExecutionReceipt == nil {
		record.ExecutionReceipt = map[string]interface{}{}
	}
	if record.CompensationReceipt == nil {
		record.CompensationReceipt = map[string]interface{}{}
	}
	if completedAt.Valid {
		record.CompletedAt = &completedAt.Time
	}
	record.Mode = "live"
	return record, true, nil
}

func insertPlaybookExecutionOutboxTx(ctx context.Context, tx *sql.Tx, executionID, tenantID, playbookName, eventType string, revision int64, traceID string, fields map[string]interface{}) error {
	eventID := uuid.NewString()
	payload := map[string]interface{}{
		"event_id": eventID, "tenant_id": tenantID, "schema_version": 2,
		"aggregate_type": "playbook_execution", "aggregate_id": executionID,
		"aggregate_version": revision, "partition_key": tenantID + ":" + executionID,
		"event_type": eventType, "execution_id": executionID, "playbook_name": playbookName,
		"trace_id": traceID,
	}
	for key, value := range fields {
		payload[key] = value
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO alert_playbook_execution_outbox
		(event_id,execution_id,tenant_id,playbook_name,event_type,schema_version,aggregate_version,partition_key,payload)
		VALUES ($1::uuid,$2,$3,$4,$5,2,$6,$7,$8::jsonb)`, eventID, executionID, tenantID,
		playbookName, eventType, revision, tenantID+":"+executionID, string(encoded))
	return err
}

func normalizedPlaybookAlertContext(request playbookExecuteV2Request) map[string]interface{} {
	if request.AlertContext != nil {
		return request.AlertContext
	}
	context := map[string]interface{}{}
	if request.AlertType != "" {
		context["alert_type"] = request.AlertType
	}
	if request.Severity != "" {
		context["severity"] = request.Severity
	}
	if request.Score != nil {
		context["score"] = request.Score
	}
	if request.RelatedAlerts != nil {
		context["related_alert_count"] = request.RelatedAlerts
	}
	if request.AssetRisk != "" {
		context["asset_risk"] = request.AssetRisk
	}
	return context
}

func playbookExecutionRequestSHA(name string, request playbookExecuteV2Request) (string, error) {
	encoded, err := json.Marshal(map[string]interface{}{
		"playbook_name": name, "expected_version": request.ExpectedVersion,
		"reason": request.Reason, "alert_context": request.AlertContext,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func playbookExecutionApprovalRequired(record PlaybookDefinitionRecord) bool {
	if record.RiskLevel == "high" || record.RiskLevel == "critical" {
		return true
	}
	policy, _ := record.Definition["approval_policy"].(map[string]interface{})
	required, _ := policy["required"].(bool)
	return required
}

func playbookAlertID(context map[string]interface{}, fallback string) string {
	if value, ok := context["alert_id"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func validPlaybookWorkflowRequest(revision *int64, reason string) bool {
	return revision != nil && *revision > 0 && len([]rune(reason)) >= 8 && len([]rune(reason)) <= 1000
}

func affectedRows(result sql.Result) int64 {
	if result == nil {
		return 0
	}
	count, _ := result.RowsAffected()
	return count
}

func playbookExecutionMeta(record playbookExecutionV2Record) httpx.ContractMeta {
	return httpx.ContractMeta{
		ContractVersion: playbookExecutionContractVersion,
		SnapshotID:      fmt.Sprintf("playbook-execution:%s:%d", record.ExecutionID, record.WorkflowRevision),
		SourceWatermarks: map[string]string{
			"postgresql.alert_playbook_executions.workflow_revision": fmt.Sprintf("%d", record.WorkflowRevision),
		},
	}
}

func writePlaybookExecutionAccepted(w http.ResponseWriter, ctx context.Context, record playbookExecutionV2Record) {
	httpx.JSONContractAccepted(w, ctx, record, playbookExecutionMeta(record))
}

func (record playbookExecutionV2Record) LockedBy() string {
	return record.LeaseOwner
}

func verifyPlaybookExecutionV2Schema(ctx context.Context, db *sql.DB) error {
	var version string
	if err := db.QueryRowContext(ctx, `SELECT version FROM alignment_schema_migrations WHERE version='202608021000'`).Scan(&version); err != nil {
		return fmt.Errorf("playbook execution v2 migration is unavailable: %w", err)
	}
	for _, table := range []string{
		"alert_playbook_execution_approvals", "alert_playbook_execution_controls",
		"alert_playbook_step_receipts", "alert_playbook_execution_outbox",
	} {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			return fmt.Errorf("playbook execution v2 table %s is unavailable", table)
		}
	}
	return nil
}

func playbookExecutionAlertContext(request map[string]interface{}) map[string]interface{} {
	context, _ := request["alert_context"].(map[string]interface{})
	if context == nil {
		return map[string]interface{}{}
	}
	return context
}

func playbookProviderReceiptFromMap(value map[string]interface{}) (PlaybookExecutionProviderReceipt, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return PlaybookExecutionProviderReceipt{}, err
	}
	var receipt PlaybookExecutionProviderReceipt
	if err := json.Unmarshal(encoded, &receipt); err != nil {
		return receipt, err
	}
	if len(receipt.Steps) == 0 {
		return receipt, fmt.Errorf("prior execution receipt has no step receipts")
	}
	return receipt, nil
}

func normalizePlaybookProviderReceipt(receipt PlaybookExecutionProviderReceipt, definition map[string]interface{}, phase string, prior map[string]interface{}) (PlaybookExecutionProviderReceipt, error) {
	receipt.Status = strings.ToLower(strings.TrimSpace(receipt.Status))
	if receipt.Status != "succeeded" && receipt.Status != "partial" && receipt.Status != "failed" {
		return receipt, fmt.Errorf("provider receipt status must be succeeded, partial, or failed")
	}
	actions := playbookDefinitionActionTypes(definition)
	expected := make(map[int]string)
	if phase == "execute" {
		for index, actionType := range actions {
			expected[index] = actionType
		}
	} else {
		priorReceipt, err := playbookProviderReceiptFromMap(prior)
		if err != nil {
			return receipt, err
		}
		for _, step := range priorReceipt.Steps {
			if step.ExternalEffect {
				expected[step.StepIndex] = step.ActionType
			}
		}
	}
	if len(expected) == 0 || len(receipt.Steps) != len(expected) {
		return receipt, fmt.Errorf("provider receipt must contain exactly one receipt for every expected external step")
	}
	seenIndexes := make(map[int]bool)
	seenProviderReceipts := make(map[string]bool)
	hasEffect := false
	for index := range receipt.Steps {
		step := &receipt.Steps[index]
		step.ActionType = strings.TrimSpace(step.ActionType)
		step.Provider = strings.TrimSpace(step.Provider)
		step.ProviderReceiptID = strings.TrimSpace(step.ProviderReceiptID)
		step.Status = strings.ToLower(strings.TrimSpace(step.Status))
		if expected[step.StepIndex] == "" || expected[step.StepIndex] != step.ActionType || seenIndexes[step.StepIndex] {
			return receipt, fmt.Errorf("provider receipt step identity does not match the approved definition")
		}
		identity := step.Provider + "\x00" + step.ProviderReceiptID
		if step.Provider == "" || step.ProviderReceiptID == "" || seenProviderReceipts[identity] {
			return receipt, fmt.Errorf("provider receipt requires a unique durable provider identity per step")
		}
		if step.Status != "succeeded" && step.Status != "partial" && step.Status != "failed" {
			return receipt, fmt.Errorf("provider step receipt has an invalid status")
		}
		seenIndexes[step.StepIndex], seenProviderReceipts[identity] = true, true
		hasEffect = hasEffect || step.ExternalEffect
	}
	if receipt.Status == "succeeded" && !hasEffect {
		return receipt, fmt.Errorf("successful provider receipt does not prove an external effect")
	}
	if receipt.Status == "failed" && hasEffect {
		return receipt, fmt.Errorf("failed provider receipt with an external effect must be reported as partial")
	}
	if receipt.Detail == nil {
		receipt.Detail = map[string]interface{}{}
	}
	return receipt, nil
}

func playbookDefinitionActionTypes(definition map[string]interface{}) []string {
	values, _ := definition["actions"].([]interface{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		action, _ := value.(map[string]interface{})
		actionType, _ := action["type"].(string)
		result = append(result, strings.TrimSpace(actionType))
	}
	return result
}

func playbookExecutionTerminalState(phase, receiptStatus string) (string, string, string) {
	if phase == "compensate" {
		if receiptStatus == "succeeded" {
			return "compensated", "compensated", "traffic.playbook.v2.Compensated"
		}
		return "compensation_failed", "compensation_failed", "traffic.playbook.v2.CompensationFailed"
	}
	switch receiptStatus {
	case "succeeded":
		return "completed", "succeeded", "traffic.playbook.v2.ExecutionCompleted"
	case "partial":
		return "partial", "partial", "traffic.playbook.v2.ExecutionPartial"
	default:
		return "failed", "failed", "traffic.playbook.v2.ExecutionFailed"
	}
}

func playbookExecutionTerminalAudit(phase, status string) string {
	if phase == "compensate" {
		if status == "succeeded" {
			return "PLAYBOOK_EXECUTION_COMPENSATED"
		}
		return "PLAYBOOK_COMPENSATION_FAILED"
	}
	return "PLAYBOOK_EXECUTION_" + strings.ToUpper(status)
}

func playbookReceiptHasExternalEffect(receipt PlaybookExecutionProviderReceipt) bool {
	for _, step := range receipt.Steps {
		if step.ExternalEffect {
			return true
		}
	}
	return false
}

func playbookReceiptCounts(receipt PlaybookExecutionProviderReceipt) (int, int) {
	succeeded, failed := 0, 0
	for _, step := range receipt.Steps {
		if step.Status == "succeeded" {
			succeeded++
		} else {
			failed++
		}
	}
	return succeeded, failed
}

func playbookMinInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

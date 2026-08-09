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
	"os"
	"strconv"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const campaignSOARMaxAttempts = 5

// CampaignSOARExecutor is the boundary to a real tenant-aware SOAR provider.
// JobID is the stable provider idempotency key. Implementations must return the
// provider's durable receipt; a rendered command or local simulation is not a
// successful receipt.
type CampaignSOARExecutor interface {
	Execute(context.Context, CampaignSOARExecutionRequest) (CampaignSOARReceipt, error)
	Compensate(context.Context, CampaignSOARExecutionRequest, CampaignSOARReceipt) (CampaignSOARReceipt, error)
}

type CampaignSOARExecutionRequest struct {
	JobID            string                 `json:"job_id"`
	TenantID         string                 `json:"tenant_id"`
	CampaignID       string                 `json:"campaign_id"`
	PlaybookID       string                 `json:"playbook_id"`
	Target           string                 `json:"target"`
	SourceSnapshotID string                 `json:"source_snapshot_id"`
	CampaignRevision int64                  `json:"campaign_revision"`
	RequestedBy      string                 `json:"requested_by"`
	ApprovedBy       string                 `json:"approved_by"`
	Metadata         map[string]interface{} `json:"metadata"`
}

type CampaignSOARReceipt struct {
	Provider          string                 `json:"provider"`
	ProviderReceiptID string                 `json:"provider_receipt_id"`
	Status            string                 `json:"status"`
	ExternalEffect    bool                   `json:"external_effect"`
	Detail            map[string]interface{} `json:"detail"`
}

type campaignSOARJob struct {
	JobID               string                 `json:"job_id"`
	TenantID            string                 `json:"tenant_id"`
	CampaignID          string                 `json:"campaign_id"`
	PlaybookID          string                 `json:"playbook_id"`
	Target              string                 `json:"target"`
	SourceSnapshotID    string                 `json:"source_snapshot_id"`
	CampaignRevision    int64                  `json:"campaign_revision"`
	Status              string                 `json:"status"`
	ApprovalStatus      string                 `json:"approval_status"`
	ExecutorStatus      string                 `json:"executor_status"`
	Revision            int64                  `json:"revision"`
	Request             map[string]interface{} `json:"request"`
	ExecutionReceipt    map[string]interface{} `json:"execution_receipt"`
	CompensationReceipt map[string]interface{} `json:"compensation_receipt"`
	ErrorMessage        string                 `json:"error_message,omitempty"`
	Attempts            int                    `json:"attempts"`
	RequestedBy         string                 `json:"requested_by"`
	ApprovedBy          string                 `json:"approved_by,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
	CompletedAt         *time.Time             `json:"completed_at,omitempty"`
	IdempotentReuse     bool                   `json:"idempotent_reuse,omitempty"`
}

type campaignSOARApprovalRequest struct {
	Decision         string `json:"decision"`
	ExpectedRevision *int64 `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type campaignSOARControlRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type campaignSOARApprovalRecord struct {
	JobID             string
	CampaignID        string
	Decision          string
	ExpectedRevision  int64
	Reason            string
	DecidedBy         string
	ResultingRevision int64
	ResultingStatus   string
	ApprovalStatus    string
}

type campaignSOARControlRecord struct {
	JobID             string
	CampaignID        string
	Operation         string
	ExpectedRevision  int64
	Reason            string
	RequestedBy       string
	ResultingRevision int64
	ResultingStatus   string
}

func (h *SystemHandler) SetCampaignSOARExecutor(executor CampaignSOARExecutor) {
	h.campaignSOARExecutor = executor
}

func (h *SystemHandler) StartCampaignSOARWorker(ctx context.Context, interval time.Duration) error {
	if !h.campaignAggregateV2 || h.campaignSOARExecutor == nil {
		return nil
	}
	if h.pgDB == nil || h.campaignAuditWriter == nil {
		return fmt.Errorf("campaign SOAR persistence is unavailable")
	}
	if err := verifyCampaignAggregateV2Schema(ctx, h.pgDB); err != nil {
		return err
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := h.processNextCampaignSOAR(ctx); err != nil && ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Failed to process campaign SOAR job", zap.Error(err))
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

func (h *SystemHandler) GetCampaignSOARJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !hasAnySystemPermission(ctx, append(campaignReadScopes(), authmodel.ScopePlaybookRead)...) {
		httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "campaign:read or playbook:read required")
		return
	}
	if h.pgDB == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "campaign SOAR persistence is unavailable")
		return
	}
	job, err := h.loadCampaignSOARJob(ctx, queryTenantID(r), strings.TrimSpace(mux.Vars(r)["id"]), strings.TrimSpace(mux.Vars(r)["job_id"]), false)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.JSONError(w, ctx, http.StatusNotFound, "SOAR_JOB_NOT_FOUND", "campaign SOAR job not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load campaign SOAR job")
		return
	}
	httpx.JSONContractSuccess(w, ctx, job, campaignSOARMeta(ctx, job))
}

func (h *SystemHandler) DecideCampaignSOARJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !hasSystemPermission(ctx, authmodel.ScopePlaybookApprove) {
		httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "playbook:approve required")
		return
	}
	if h.pgDB == nil || h.campaignAuditWriter == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "campaign SOAR approval persistence is unavailable")
		return
	}
	tenantID, campaignID, jobID, actor, idempotencyKey, ok := campaignSOARWorkflowIdentity(w, r)
	if !ok {
		return
	}
	var request campaignSOARApprovalRequest
	if !decodeCampaignSOARJSON(w, r, &request) {
		return
	}
	request.Decision = strings.ToLower(strings.TrimSpace(request.Decision))
	request.Reason = strings.TrimSpace(request.Reason)
	if (request.Decision != "approve" && request.Decision != "reject") || request.ExpectedRevision == nil || *request.ExpectedRevision <= 0 || len([]rune(request.Reason)) < 8 || len([]rune(request.Reason)) > 1000 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "decision approve/reject, positive expected_revision and reason (8 to 1000 characters) are required")
		return
	}
	tx, err := h.pgDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin SOAR approval transaction")
		return
	}
	defer tx.Rollback()
	if existing, found, loadErr := loadCampaignSOARApprovalByKey(ctx, tx, tenantID, idempotencyKey); loadErr != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to resolve SOAR approval idempotency")
		return
	} else if found {
		if !campaignSOARApprovalMatches(existing, campaignID, jobID, actor, request) {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different SOAR approval")
			return
		}
		job, loadErr := h.loadCampaignSOARJobTx(ctx, tx, tenantID, campaignID, jobID, false)
		if loadErr != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load idempotent SOAR approval result")
			return
		}
		job.IdempotentReuse = true
		writeCampaignSOARAccepted(w, ctx, job)
		return
	}
	job, err := h.loadCampaignSOARJobTx(ctx, tx, tenantID, campaignID, jobID, true)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.JSONError(w, ctx, http.StatusNotFound, "SOAR_JOB_NOT_FOUND", "campaign SOAR job not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to lock campaign SOAR job")
		return
	}
	if actor == job.RequestedBy {
		httpx.JSONError(w, ctx, http.StatusForbidden, "INDEPENDENT_APPROVER_REQUIRED", "the requester cannot approve or reject the same SOAR job")
		return
	}
	if job.Status != "pending_approval" || job.ApprovalStatus != "pending" {
		httpx.JSONError(w, ctx, http.StatusConflict, "INVALID_STATE", "only a pending SOAR job can be approved or rejected")
		return
	}
	if job.Revision != *request.ExpectedRevision {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", fmt.Sprintf("expected revision %d but current revision is %d", *request.ExpectedRevision, job.Revision))
		return
	}
	job.Revision++
	job.ApprovalStatus = "approved"
	job.Status = "approved_awaiting_executor"
	job.ExecutorStatus = "queued"
	if h.campaignSOARExecutor == nil {
		job.ExecutorStatus = "not_configured"
	}
	if request.Decision == "reject" {
		job.ApprovalStatus = "rejected"
		job.Status = "cancelled"
		job.ExecutorStatus = "cancelled"
		job.ErrorMessage = "rejected: " + request.Reason
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_soar_approvals
		(approval_id,job_id,tenant_id,campaign_id,decision,expected_revision,idempotency_key,reason,
		 decided_by,resulting_revision,resulting_status,approval_status)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, uuid.NewString(), jobID, tenantID,
		campaignID, request.Decision, *request.ExpectedRevision, idempotencyKey, request.Reason, actor,
		job.Revision, job.Status, job.ApprovalStatus); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist SOAR approval")
		return
	}
	completed := interface{}(nil)
	if request.Decision == "reject" {
		completed = time.Now().UTC()
	}
	result, err := tx.ExecContext(ctx, `UPDATE campaign_soar_jobs SET status=$4,approval_status=$5,
		executor_status=$6,revision=$7,approved_by=CASE WHEN $5='approved' THEN $8 ELSE approved_by END,
		approved_at=CASE WHEN $5='approved' THEN now() ELSE approved_at END,error_message=$9,
		next_attempt_at=now(),updated_at=now(),completed_at=$10
		WHERE tenant_id=$1 AND campaign_id=$2 AND job_id=$3 AND revision=$11 AND status='pending_approval'`,
		tenantID, campaignID, jobID, job.Status, job.ApprovalStatus, job.ExecutorStatus, job.Revision,
		actor, job.ErrorMessage, completed, *request.ExpectedRevision)
	if err != nil || requireCampaignSOARRow(result, "apply SOAR approval") != nil {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "SOAR job changed while approval was being applied")
		return
	}
	resultPatch, _ := json.Marshal(map[string]interface{}{
		"approval_status": job.ApprovalStatus, "executor_status": job.ExecutorStatus,
		"workflow_revision": job.Revision, "final_effect": false,
	})
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_action_jobs SET status=$4,
		result=COALESCE(result,'{}'::jsonb)||$5::jsonb,error_message=$6,completed_at=$7
		WHERE tenant_id=$1 AND campaign_id=$2 AND job_id=$3`, tenantID, campaignID, jobID,
		job.Status, string(resultPatch), job.ErrorMessage, completed); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to update campaign action job")
		return
	}
	auditAction := "CAMPAIGN_SOAR_APPROVED"
	if request.Decision == "reject" {
		auditAction = "CAMPAIGN_SOAR_REJECTED"
	}
	if err := h.campaignAuditWriter.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: auditAction, ObjectType: "campaign_soar_job", ObjectID: jobID,
		TenantID: tenantID, UserID: actor, Reason: request.Reason, Result: job.Status,
		Detail: map[string]interface{}{"campaign_id": campaignID, "decision": request.Decision,
			"expected_revision": *request.ExpectedRevision, "revision": job.Revision,
			"idempotency_key_sha256": opaqueKeyDigest(idempotencyKey)},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit SOAR approval")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit SOAR approval")
		return
	}
	job.ApprovedBy = actor
	writeCampaignSOARAccepted(w, ctx, job)
}

func (h *SystemHandler) CancelCampaignSOARJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !hasAnySystemPermission(ctx, authmodel.ScopePlaybookExecute, authmodel.ScopeCampaignWrite) {
		httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "playbook:execute or campaign:write required")
		return
	}
	h.applyCampaignSOARControl(w, r, "cancel")
}

func (h *SystemHandler) CompensateCampaignSOARJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !hasSystemPermission(ctx, authmodel.ScopePlaybookApprove) {
		httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "playbook:approve required")
		return
	}
	h.applyCampaignSOARControl(w, r, "compensate")
}

func (h *SystemHandler) applyCampaignSOARControl(w http.ResponseWriter, r *http.Request, operation string) {
	ctx := r.Context()
	if h.pgDB == nil || h.campaignAuditWriter == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "campaign SOAR control persistence is unavailable")
		return
	}
	tenantID, campaignID, jobID, actor, idempotencyKey, ok := campaignSOARWorkflowIdentity(w, r)
	if !ok {
		return
	}
	var request campaignSOARControlRequest
	if !decodeCampaignSOARJSON(w, r, &request) {
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if request.ExpectedRevision == nil || *request.ExpectedRevision <= 0 || len([]rune(request.Reason)) < 8 || len([]rune(request.Reason)) > 1000 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "positive expected_revision and reason (8 to 1000 characters) are required")
		return
	}
	tx, err := h.pgDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin SOAR control transaction")
		return
	}
	defer tx.Rollback()
	if existing, found, loadErr := loadCampaignSOARControlByKey(ctx, tx, tenantID, idempotencyKey); loadErr != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to resolve SOAR control idempotency")
		return
	} else if found {
		if !campaignSOARControlMatches(existing, campaignID, jobID, actor, operation, request) {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different SOAR control request")
			return
		}
		job, loadErr := h.loadCampaignSOARJobTx(ctx, tx, tenantID, campaignID, jobID, false)
		if loadErr != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load idempotent SOAR control result")
			return
		}
		job.IdempotentReuse = true
		writeCampaignSOARAccepted(w, ctx, job)
		return
	}
	job, err := h.loadCampaignSOARJobTx(ctx, tx, tenantID, campaignID, jobID, true)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.JSONError(w, ctx, http.StatusNotFound, "SOAR_JOB_NOT_FOUND", "campaign SOAR job not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to lock campaign SOAR job")
		return
	}
	if job.Revision != *request.ExpectedRevision {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", fmt.Sprintf("expected revision %d but current revision is %d", *request.ExpectedRevision, job.Revision))
		return
	}
	if operation == "cancel" {
		if job.Status != "pending_approval" && job.Status != "approved_awaiting_executor" {
			httpx.JSONError(w, ctx, http.StatusConflict, "INVALID_STATE", "only a pending or undispatched SOAR job can be cancelled")
			return
		}
		job.Status = "cancelled"
		job.ExecutorStatus = "cancelled"
		job.ApprovalStatus = "cancelled"
		job.ErrorMessage = "cancelled: " + request.Reason
	} else {
		if actor == job.RequestedBy {
			httpx.JSONError(w, ctx, http.StatusForbidden, "INDEPENDENT_APPROVER_REQUIRED", "the original requester cannot approve compensation")
			return
		}
		if job.Status != "completed" && job.Status != "partial" {
			httpx.JSONError(w, ctx, http.StatusConflict, "INVALID_STATE", "only a completed or partial SOAR effect can be compensated")
			return
		}
		var externalEffect bool
		if err := tx.QueryRowContext(ctx, `SELECT external_effect FROM campaign_soar_execution_receipts
			WHERE tenant_id=$1 AND job_id=$2 AND phase='execute' ORDER BY attempt DESC LIMIT 1`, tenantID, jobID).Scan(&externalEffect); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpx.JSONError(w, ctx, http.StatusConflict, "EXECUTION_RECEIPT_REQUIRED", "compensation requires a provider execution receipt")
			} else {
				httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to validate execution receipt")
			}
			return
		}
		if !externalEffect {
			httpx.JSONError(w, ctx, http.StatusConflict, "EXTERNAL_EFFECT_REQUIRED", "receipt does not prove an external effect to compensate")
			return
		}
		job.Status = "compensation_queued"
		job.ExecutorStatus = "queued"
		if h.campaignSOARExecutor == nil {
			job.ExecutorStatus = "not_configured"
		}
		job.ErrorMessage = ""
	}
	job.Revision++
	completed := interface{}(nil)
	if operation == "cancel" {
		completed = time.Now().UTC()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_soar_control_requests
		(request_id,job_id,tenant_id,campaign_id,operation,expected_revision,idempotency_key,reason,
		 requested_by,resulting_revision,resulting_status)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, uuid.NewString(), jobID,
		tenantID, campaignID, operation, *request.ExpectedRevision, idempotencyKey, request.Reason,
		actor, job.Revision, job.Status); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist SOAR control request")
		return
	}
	result, err := tx.ExecContext(ctx, `UPDATE campaign_soar_jobs SET status=$4,approval_status=$5,
		executor_status=$6,revision=$7,error_message=$8,next_attempt_at=now(),locked_until=NULL,
		locked_by='',updated_at=now(),completed_at=$9 WHERE tenant_id=$1 AND campaign_id=$2
		AND job_id=$3 AND revision=$10`, tenantID, campaignID, jobID, job.Status,
		job.ApprovalStatus, job.ExecutorStatus, job.Revision, job.ErrorMessage, completed, *request.ExpectedRevision)
	if err != nil || requireCampaignSOARRow(result, "apply SOAR control") != nil {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "SOAR job changed while control was being applied")
		return
	}
	resultPatch, _ := json.Marshal(map[string]interface{}{"approval_status": job.ApprovalStatus,
		"executor_status": job.ExecutorStatus, "workflow_revision": job.Revision, "final_effect": false})
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_action_jobs SET status=$4,
		result=COALESCE(result,'{}'::jsonb)||$5::jsonb,error_message=$6,completed_at=$7
		WHERE tenant_id=$1 AND campaign_id=$2 AND job_id=$3`, tenantID, campaignID, jobID,
		job.Status, string(resultPatch), job.ErrorMessage, completed); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to update campaign action job")
		return
	}
	auditAction := "CAMPAIGN_SOAR_CANCELLED"
	if operation == "compensate" {
		auditAction = "CAMPAIGN_SOAR_COMPENSATION_APPROVED"
	}
	if err := h.campaignAuditWriter.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: auditAction, ObjectType: "campaign_soar_job", ObjectID: jobID,
		TenantID: tenantID, UserID: actor, Reason: request.Reason, Result: job.Status,
		Detail: map[string]interface{}{"campaign_id": campaignID, "operation": operation,
			"expected_revision": *request.ExpectedRevision, "revision": job.Revision,
			"idempotency_key_sha256": opaqueKeyDigest(idempotencyKey)},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit SOAR control request")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit SOAR control request")
		return
	}
	writeCampaignSOARAccepted(w, ctx, job)
}

func (h *SystemHandler) processNextCampaignSOAR(ctx context.Context) error {
	if h.campaignSOARExecutor == nil {
		return nil
	}
	workerID := fmt.Sprintf("%s-%d", hostnameOrDefault(), os.Getpid())
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var job campaignSOARJob
	var requestJSON, executionJSON, compensationJSON []byte
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
		SELECT job_id FROM campaign_soar_jobs
		WHERE ((status IN ('approved_awaiting_executor','compensation_queued') AND next_attempt_at<=now())
		   OR (status IN ('running','compensating') AND locked_until<now()))
		ORDER BY created_at,job_id LIMIT 1 FOR UPDATE SKIP LOCKED
	)
	UPDATE campaign_soar_jobs s SET
		status=CASE WHEN s.status IN ('compensation_queued','compensating') THEN 'compensating' ELSE 'running' END,
		executor_status=CASE WHEN s.status IN ('compensation_queued','compensating') THEN 'compensating' ELSE 'running' END,
		attempts=s.attempts+1,locked_until=now()+interval '5 minutes',locked_by=$1,updated_at=now()
	FROM candidate c WHERE s.job_id=c.job_id
	RETURNING s.job_id,s.tenant_id,s.campaign_id,s.playbook_id,s.target,s.source_snapshot_id,
		s.campaign_revision,s.status,s.approval_status,s.executor_status,s.revision,s.request::text,
		s.execution_receipt::text,s.compensation_receipt::text,s.error_message,s.attempts,
		s.requested_by,s.approved_by,s.created_at,s.updated_at,s.completed_at`, workerID).Scan(
		&job.JobID, &job.TenantID, &job.CampaignID, &job.PlaybookID, &job.Target,
		&job.SourceSnapshotID, &job.CampaignRevision, &job.Status, &job.ApprovalStatus,
		&job.ExecutorStatus, &job.Revision, &requestJSON, &executionJSON, &compensationJSON,
		&job.ErrorMessage, &job.Attempts, &job.RequestedBy, &job.ApprovedBy,
		&job.CreatedAt, &job.UpdatedAt, &job.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(requestJSON, &job.Request); err != nil {
		return err
	}
	_ = json.Unmarshal(executionJSON, &job.ExecutionReceipt)
	_ = json.Unmarshal(compensationJSON, &job.CompensationReceipt)
	if err := tx.Commit(); err != nil {
		return err
	}
	executionRequest := CampaignSOARExecutionRequest{
		JobID: job.JobID, TenantID: job.TenantID, CampaignID: job.CampaignID,
		PlaybookID: job.PlaybookID, Target: job.Target, SourceSnapshotID: job.SourceSnapshotID,
		CampaignRevision: job.CampaignRevision, RequestedBy: job.RequestedBy,
		ApprovedBy: job.ApprovedBy, Metadata: job.Request,
	}
	phase := "execute"
	var receipt CampaignSOARReceipt
	if job.Status == "compensating" {
		phase = "compensate"
		prior, priorErr := campaignSOARReceiptFromMap(job.ExecutionReceipt)
		if priorErr != nil {
			return h.failCampaignSOARAttempt(ctx, workerID, job, phase, priorErr)
		}
		receipt, err = h.campaignSOARExecutor.Compensate(ctx, executionRequest, prior)
	} else {
		receipt, err = h.campaignSOARExecutor.Execute(ctx, executionRequest)
	}
	if err != nil {
		return h.failCampaignSOARAttempt(ctx, workerID, job, phase, err)
	}
	receipt, err = normalizeCampaignSOARReceipt(receipt)
	if err != nil {
		return h.failCampaignSOARAttempt(ctx, workerID, job, phase, err)
	}
	return h.completeCampaignSOARAttempt(ctx, workerID, job, phase, receipt)
}

func (h *SystemHandler) completeCampaignSOARAttempt(ctx context.Context, workerID string, job campaignSOARJob, phase string, receipt CampaignSOARReceipt) error {
	receiptJSON, receiptSHA, err := canonicalCampaignSOARReceipt(receipt)
	if err != nil {
		return err
	}
	tx, err := h.pgDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentStatus, lockedBy string
	if err := tx.QueryRowContext(ctx, `SELECT status,locked_by FROM campaign_soar_jobs WHERE job_id=$1 FOR UPDATE`, job.JobID).Scan(&currentStatus, &lockedBy); err != nil {
		return err
	}
	expectedStatus := "running"
	if phase == "compensate" {
		expectedStatus = "compensating"
	}
	if currentStatus != expectedStatus || lockedBy != workerID {
		return fmt.Errorf("campaign SOAR lease lost before %s receipt commit", phase)
	}
	state, err := lockCampaignAggregateV2State(ctx, tx, job.TenantID, job.CampaignID)
	if err != nil {
		return err
	}
	state.Revision++
	job.Revision++
	workflowStatus, executorStatus, actionStatus, eventType := campaignSOARTerminalState(phase, receipt.Status)
	job.Status, job.ExecutorStatus = workflowStatus, executorStatus
	eventID := uuid.NewString()
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": eventType, "tenant_id": job.TenantID,
		"schema_version": 2, "aggregate_type": "campaign", "aggregate_id": job.CampaignID,
		"aggregate_version": state.Revision, "partition_key": job.TenantID + ":" + job.CampaignID,
		"campaign_id": job.CampaignID, "status": state.Status, "assignee": state.Assignee,
		"member_count": state.MemberCount, "job_id": job.JobID, "playbook_id": job.PlaybookID,
		"source_snapshot_id": job.SourceSnapshotID, "provider": receipt.Provider,
		"provider_receipt_id": receipt.ProviderReceiptID, "receipt_status": receipt.Status,
		"receipt_sha256": receiptSHA, "external_effect": receipt.ExternalEffect,
		"workflow_revision": job.Revision, "trace_id": "campaign-soar-worker:" + job.JobID,
	}
	payloadJSON, _ := json.Marshal(payload)
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_soar_execution_receipts
		(receipt_id,job_id,tenant_id,campaign_id,phase,attempt,provider,provider_receipt_id,status,
		 external_effect,payload,payload_sha256)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12)`, uuid.NewString(),
		job.JobID, job.TenantID, job.CampaignID, phase, job.Attempts, receipt.Provider,
		receipt.ProviderReceiptID, receipt.Status, receipt.ExternalEffect, string(receiptJSON), receiptSHA); err != nil {
		return err
	}
	receiptColumn := "execution_receipt"
	if phase == "compensate" {
		receiptColumn = "compensation_receipt"
	}
	updateQuery := fmt.Sprintf(`UPDATE campaign_soar_jobs SET status=$2,executor_status=$3,revision=$4,
		%s=$5::jsonb,error_message=CASE WHEN $6='failed' THEN COALESCE($5::jsonb->>'error','provider returned failed receipt') ELSE '' END,
		locked_until=NULL,locked_by='',updated_at=now(),completed_at=now()
		WHERE job_id=$1 AND status=$7`, receiptColumn)
	result, err := tx.ExecContext(ctx, updateQuery, job.JobID, workflowStatus, executorStatus,
		job.Revision, string(receiptJSON), receipt.Status, expectedStatus)
	if err != nil {
		return err
	}
	if err := requireCampaignSOARRow(result, "commit SOAR provider receipt"); err != nil {
		return err
	}
	resultPatch, _ := json.Marshal(map[string]interface{}{
		"approval_status": job.ApprovalStatus, "executor_status": executorStatus,
		"workflow_revision": job.Revision, "provider": receipt.Provider,
		"provider_receipt_id": receipt.ProviderReceiptID, "receipt_sha256": receiptSHA,
		"external_effect": receipt.ExternalEffect, "final_effect": phase == "execute" && receipt.ExternalEffect && receipt.Status != "failed",
	})
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_action_jobs SET status=$3,
		result=COALESCE(result,'{}'::jsonb)||$4::jsonb,error_message=CASE WHEN $3 IN ('failed','compensation_failed') THEN 'provider returned a non-success receipt' ELSE '' END,
		resource_revision=$5,completed_at=now() WHERE tenant_id=$1 AND job_id=$2`, job.TenantID,
		job.JobID, actionStatus, string(resultPatch), state.Revision); err != nil {
		return err
	}
	if err := appendCampaignSOARLifecycle(ctx, tx, eventID, eventType, job, state, string(payloadJSON), "campaign SOAR provider receipt committed"); err != nil {
		return err
	}
	if err := h.campaignAuditWriter.recordWithExecutor(ctx, tx, nil, AlertActionAuditRecord{
		Action: strings.ToUpper(strings.ReplaceAll(eventType, ".", "_")), ObjectType: "campaign_soar_job", ObjectID: job.JobID,
		TenantID: job.TenantID, UserID: job.ApprovedBy, Result: workflowStatus,
		Detail: map[string]interface{}{"campaign_id": job.CampaignID, "event_id": eventID,
			"phase": phase, "provider": receipt.Provider, "provider_receipt_id": receipt.ProviderReceiptID,
			"receipt_sha256": receiptSHA, "external_effect": receipt.ExternalEffect,
			"workflow_revision": job.Revision, "resource_revision": state.Revision},
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *SystemHandler) failCampaignSOARAttempt(ctx context.Context, workerID string, job campaignSOARJob, phase string, cause error) error {
	message := strings.TrimSpace(cause.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	if job.Attempts < campaignSOARMaxAttempts {
		nextStatus, nextExecutor := "approved_awaiting_executor", "queued"
		if phase == "compensate" {
			nextStatus = "compensation_queued"
			nextExecutor = "queued"
		}
		result, err := h.pgDB.ExecContext(ctx, `UPDATE campaign_soar_jobs SET status=$3,
			executor_status=$4,error_message=$5,next_attempt_at=now()+(LEAST(300,POWER(2,LEAST(attempts,8)))::text||' seconds')::interval,
			locked_until=NULL,locked_by='',updated_at=now() WHERE job_id=$1 AND locked_by=$2`,
			job.JobID, workerID, nextStatus, nextExecutor, message)
		if err != nil {
			return fmt.Errorf("campaign SOAR failure %v; retry scheduling failed: %w", cause, err)
		}
		if err := requireCampaignSOARRow(result, "reschedule campaign SOAR job"); err != nil {
			return fmt.Errorf("campaign SOAR failure %v; retry scheduling failed: %w", cause, err)
		}
		return cause
	}
	failureReceipt := CampaignSOARReceipt{
		Provider: "executor-error", ProviderReceiptID: job.JobID + ":" + phase + ":" + strconv.Itoa(job.Attempts),
		Status: "failed", ExternalEffect: false, Detail: map[string]interface{}{"error": message},
	}
	if err := h.completeCampaignSOARAttempt(ctx, workerID, job, phase, failureReceipt); err != nil {
		return fmt.Errorf("campaign SOAR failure %v; terminal commit failed: %w", cause, err)
	}
	return cause
}

func appendCampaignSOARLifecycle(ctx context.Context, tx *sql.Tx, eventID, eventType string, job campaignSOARJob, state campaignAggregateState, payloadJSON, reason string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_workbench_state SET state_version=$3,last_event_id=$4::uuid,
		updated_by=$5,updated_at=now() WHERE tenant_id=$1 AND campaign_id=$2`, job.TenantID,
		job.CampaignID, state.Revision, eventID, job.ApprovedBy); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_aggregate_history
		(event_id,tenant_id,campaign_id,aggregate_revision,event_type,status,assignee,member_count,payload,reason,created_by)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11)`, eventID, job.TenantID,
		job.CampaignID, state.Revision, eventType, state.Status, state.Assignee, state.MemberCount,
		payloadJSON, reason, job.ApprovedBy); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO campaign_aggregate_outbox
		(event_id,tenant_id,aggregate_id,aggregate_revision,event_type,partition_key,payload)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::jsonb)`, eventID, job.TenantID, job.CampaignID,
		state.Revision, eventType, job.TenantID+":"+job.CampaignID, payloadJSON)
	return err
}

func campaignSOARTerminalState(phase, receiptStatus string) (string, string, string, string) {
	if phase == "compensate" {
		if receiptStatus == "succeeded" {
			return "compensated", "compensated", "compensated", "traffic.campaign.v2.SoarCompensated"
		}
		return "compensation_failed", "compensation_failed", "compensation_failed", "traffic.campaign.v2.SoarCompensationFailed"
	}
	switch receiptStatus {
	case "succeeded":
		return "completed", "succeeded", "completed", "traffic.campaign.v2.SoarCompleted"
	case "partial":
		return "partial", "partial", "partial", "traffic.campaign.v2.SoarPartial"
	default:
		return "failed", "failed", "failed", "traffic.campaign.v2.SoarFailed"
	}
}

func normalizeCampaignSOARReceipt(receipt CampaignSOARReceipt) (CampaignSOARReceipt, error) {
	receipt.Provider = strings.TrimSpace(receipt.Provider)
	receipt.ProviderReceiptID = strings.TrimSpace(receipt.ProviderReceiptID)
	receipt.Status = strings.ToLower(strings.TrimSpace(receipt.Status))
	if receipt.Provider == "" || receipt.ProviderReceiptID == "" {
		return CampaignSOARReceipt{}, fmt.Errorf("SOAR provider and provider receipt id are required")
	}
	if receipt.Status != "succeeded" && receipt.Status != "partial" && receipt.Status != "failed" {
		return CampaignSOARReceipt{}, fmt.Errorf("SOAR receipt status must be succeeded, partial, or failed")
	}
	if receipt.Status == "succeeded" && !receipt.ExternalEffect {
		return CampaignSOARReceipt{}, fmt.Errorf("a succeeded SOAR receipt must prove an external effect")
	}
	if receipt.Detail == nil {
		receipt.Detail = map[string]interface{}{}
	}
	return receipt, nil
}

func canonicalCampaignSOARReceipt(receipt CampaignSOARReceipt) ([]byte, string, error) {
	payload, err := json.Marshal(receipt)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(payload)
	return payload, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func campaignSOARReceiptFromMap(value map[string]interface{}) (CampaignSOARReceipt, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return CampaignSOARReceipt{}, err
	}
	var receipt CampaignSOARReceipt
	if err := json.Unmarshal(payload, &receipt); err != nil {
		return CampaignSOARReceipt{}, err
	}
	receipt, err = normalizeCampaignSOARReceipt(receipt)
	if err != nil {
		return CampaignSOARReceipt{}, err
	}
	return receipt, nil
}

func campaignSOARWorkflowIdentity(w http.ResponseWriter, r *http.Request) (tenantID, campaignID, jobID, actor, idempotencyKey string, ok bool) {
	tenantID = queryTenantID(r)
	campaignID = strings.TrimSpace(mux.Vars(r)["id"])
	jobID = strings.TrimSpace(mux.Vars(r)["job_id"])
	actor = strings.TrimSpace(httpx.GetUserID(r.Context()))
	idempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if tenantID == "" || campaignID == "" || jobID == "" {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "tenant, campaign id and job id are required")
		return "", "", "", "", "", false
	}
	if actor == "" {
		httpx.JSONError(w, r.Context(), http.StatusUnauthorized, "ACTOR_REQUIRED", "an authenticated actor is required")
		return "", "", "", "", "", false
	}
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain 16 to 200 characters")
		return "", "", "", "", "", false
	}
	return tenantID, campaignID, jobID, actor, idempotencyKey, true
}

func decodeCampaignSOARJSON(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil || ensureJSONBodyComplete(decoder) != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "request body must contain exactly one valid JSON object")
		return false
	}
	return true
}

func (h *SystemHandler) loadCampaignSOARJob(ctx context.Context, tenantID, campaignID, jobID string, lock bool) (campaignSOARJob, error) {
	if lock {
		return campaignSOARJob{}, fmt.Errorf("locking requires a transaction")
	}
	return scanCampaignSOARJob(h.pgDB.QueryRowContext(ctx, campaignSOARSelect(false), tenantID, campaignID, jobID))
}

func (h *SystemHandler) loadCampaignSOARJobTx(ctx context.Context, tx *sql.Tx, tenantID, campaignID, jobID string, lock bool) (campaignSOARJob, error) {
	return scanCampaignSOARJob(tx.QueryRowContext(ctx, campaignSOARSelect(lock), tenantID, campaignID, jobID))
}

func campaignSOARSelect(lock bool) string {
	query := `SELECT job_id,tenant_id,campaign_id,playbook_id,target,source_snapshot_id,campaign_revision,
		status,approval_status,executor_status,revision,request::text,execution_receipt::text,
		compensation_receipt::text,error_message,attempts,requested_by,approved_by,created_at,updated_at,completed_at
		FROM campaign_soar_jobs WHERE tenant_id=$1 AND campaign_id=$2 AND job_id=$3`
	if lock {
		query += " FOR UPDATE"
	}
	return query
}

type campaignSOARScanner interface{ Scan(...interface{}) error }

func scanCampaignSOARJob(row campaignSOARScanner) (campaignSOARJob, error) {
	var job campaignSOARJob
	var requestJSON, executionJSON, compensationJSON []byte
	if err := row.Scan(&job.JobID, &job.TenantID, &job.CampaignID, &job.PlaybookID, &job.Target,
		&job.SourceSnapshotID, &job.CampaignRevision, &job.Status, &job.ApprovalStatus,
		&job.ExecutorStatus, &job.Revision, &requestJSON, &executionJSON, &compensationJSON,
		&job.ErrorMessage, &job.Attempts, &job.RequestedBy, &job.ApprovedBy,
		&job.CreatedAt, &job.UpdatedAt, &job.CompletedAt); err != nil {
		return campaignSOARJob{}, err
	}
	if err := json.Unmarshal(requestJSON, &job.Request); err != nil {
		return campaignSOARJob{}, err
	}
	if err := json.Unmarshal(executionJSON, &job.ExecutionReceipt); err != nil {
		return campaignSOARJob{}, err
	}
	if err := json.Unmarshal(compensationJSON, &job.CompensationReceipt); err != nil {
		return campaignSOARJob{}, err
	}
	return job, nil
}

func loadCampaignSOARApprovalByKey(ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey string) (campaignSOARApprovalRecord, bool, error) {
	var record campaignSOARApprovalRecord
	err := tx.QueryRowContext(ctx, `SELECT job_id,campaign_id,decision,expected_revision,reason,decided_by,
		resulting_revision,resulting_status,approval_status FROM campaign_soar_approvals
		WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, idempotencyKey).Scan(&record.JobID,
		&record.CampaignID, &record.Decision, &record.ExpectedRevision, &record.Reason,
		&record.DecidedBy, &record.ResultingRevision, &record.ResultingStatus, &record.ApprovalStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return campaignSOARApprovalRecord{}, false, nil
	}
	return record, err == nil, err
}

func loadCampaignSOARControlByKey(ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey string) (campaignSOARControlRecord, bool, error) {
	var record campaignSOARControlRecord
	err := tx.QueryRowContext(ctx, `SELECT job_id,campaign_id,operation,expected_revision,reason,requested_by,
		resulting_revision,resulting_status FROM campaign_soar_control_requests
		WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, idempotencyKey).Scan(&record.JobID,
		&record.CampaignID, &record.Operation, &record.ExpectedRevision, &record.Reason,
		&record.RequestedBy, &record.ResultingRevision, &record.ResultingStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return campaignSOARControlRecord{}, false, nil
	}
	return record, err == nil, err
}

func campaignSOARApprovalMatches(existing campaignSOARApprovalRecord, campaignID, jobID, actor string, request campaignSOARApprovalRequest) bool {
	return existing.JobID == jobID && existing.CampaignID == campaignID && existing.Decision == request.Decision &&
		existing.ExpectedRevision == *request.ExpectedRevision && existing.Reason == request.Reason && existing.DecidedBy == actor
}

func campaignSOARControlMatches(existing campaignSOARControlRecord, campaignID, jobID, actor, operation string, request campaignSOARControlRequest) bool {
	return existing.JobID == jobID && existing.CampaignID == campaignID && existing.Operation == operation &&
		existing.ExpectedRevision == *request.ExpectedRevision && existing.Reason == request.Reason && existing.RequestedBy == actor
}

func requireCampaignSOARRow(result sql.Result, operation string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s row count: %w", operation, err)
	}
	if affected != 1 {
		return fmt.Errorf("%s affected %d rows instead of 1", operation, affected)
	}
	return nil
}

func campaignSOARMeta(ctx context.Context, job campaignSOARJob) httpx.ContractMeta {
	return httpx.ContractMeta{
		ContractVersion: campaignAggregateContractVersion,
		SnapshotID:      fmt.Sprintf("campaign-soar:%s:%d", job.JobID, job.Revision),
		SourceWatermarks: map[string]string{
			"postgresql.campaign_soar_jobs.revision": strconv.FormatInt(job.Revision, 10),
		},
	}
}

func writeCampaignSOARAccepted(w http.ResponseWriter, ctx context.Context, job campaignSOARJob) {
	httpx.JSONContractAccepted(w, ctx, job, campaignSOARMeta(ctx, job))
}

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	alertstate "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/state"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const alertBatchCompensationActionID = "alert-batch-assignment-compensate"

var (
	errAlertBatchCompensationUnavailable = errors.New("alert batch assignment compensation unavailable")
	errAlertBatchCompensationConflict    = errors.New("alert batch assignment compensation conflict")
)

type AlertBatchAssignmentCompensationRequest struct {
	ActionID         string `json:"action_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type AlertBatchAssignmentCompensationReceipt struct {
	RequestID             string `json:"request_id"`
	JobID                 string `json:"job_id"`
	BatchID               string `json:"batch_id"`
	EventID               string `json:"event_id"`
	ActionID              string `json:"action_id"`
	ExpectedBatchRevision int64  `json:"expected_batch_revision"`
	Status                string `json:"status"`
	Revision              int64  `json:"revision"`
	TotalCount            int    `json:"total_count"`
	AcceptedCount         int    `json:"accepted_count"`
	CompensatedCount      int    `json:"compensated_count"`
	ConflictedCount       int    `json:"conflicted_count"`
	FailedCount           int    `json:"failed_count"`
	TraceID               string `json:"trace_id"`
	OutboxStatus          string `json:"outbox_status"`
	Replayed              bool   `json:"replayed"`
}

type AlertBatchAssignmentCompensationItemResult struct {
	AlertID                  string    `json:"alert_id"`
	Position                 int       `json:"position"`
	Status                   string    `json:"status"`
	ItemRevision             int64     `json:"item_revision"`
	ExpectedStateVersion     int64     `json:"expected_state_version"`
	CompensationStateVersion int64     `json:"compensation_state_version,omitempty"`
	RestoreAssignee          string    `json:"restore_assignee"`
	RestoreStatus            string    `json:"restore_status"`
	CurrentAssignee          string    `json:"current_assignee"`
	CurrentStatus            string    `json:"current_status"`
	ErrorCode                string    `json:"error_code,omitempty"`
	ErrorMessage             string    `json:"error_message,omitempty"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type AlertBatchAssignmentCompensationJob struct {
	AlertBatchAssignmentCompensationReceipt
	Reason      string                                       `json:"reason"`
	RequestedBy string                                       `json:"requested_by"`
	CreatedAt   time.Time                                    `json:"created_at"`
	UpdatedAt   time.Time                                    `json:"updated_at"`
	Items       []AlertBatchAssignmentCompensationItemResult `json:"items"`
}

func (h *AlertBatchAssignmentHandler) CreateCompensation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.compensationEnabled {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "FEATURE_DISABLED", "alert batch assignment compensation v1 is disabled")
		return
	}
	command, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	batchID := strings.TrimSpace(mux.Vars(r)["batch_id"])
	if _, err := uuid.Parse(batchID); err != nil {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_BATCH_ID", "batch_id must be a UUID")
		return
	}
	var request AlertBatchAssignmentCompensationRequest
	if !decodeStrictJSON(w, r, &request) {
		return
	}
	request.ActionID = strings.TrimSpace(request.ActionID)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.ActionID != alertBatchCompensationActionID || request.ExpectedRevision <= 0 ||
		len([]rune(request.Reason)) < 8 || len([]rune(request.Reason)) > 1000 {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST",
			"action_id, a positive expected_revision and a reason of 8 to 1000 characters are required")
		return
	}
	receipt, err := h.createCompensation(ctx, command, batchID, request)
	if err != nil {
		h.writeCompensationError(w, ctx, err)
		return
	}
	httpx.JSONContractAccepted(w, ctx, receipt,
		alertBatchContractMeta(ctx, batchID, receipt.TraceID, receipt.Revision, true,
			[]string{"alert.assignment.compensated.v1 consumer receipt"}))
}

func (h *AlertBatchAssignmentHandler) GetCompensation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.compensationEnabled {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "FEATURE_DISABLED", "alert batch assignment compensation v1 is disabled")
		return
	}
	command, ok := h.authorize(w, r, false)
	if !ok {
		return
	}
	batchID := strings.TrimSpace(mux.Vars(r)["batch_id"])
	requestID := strings.TrimSpace(mux.Vars(r)["request_id"])
	if _, err := uuid.Parse(batchID); err != nil {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_BATCH_ID", "batch_id must be a UUID")
		return
	}
	if _, err := uuid.Parse(requestID); err != nil {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST_ID", "request_id must be a UUID")
		return
	}
	job, err := h.getCompensation(ctx, command.TenantID, batchID, requestID)
	if err != nil {
		if errors.Is(err, errAlertBatchNotFound) {
			h.writeError(w, ctx, http.StatusNotFound, "NOT_FOUND", "alert batch assignment compensation not found")
			return
		}
		h.writeCompensationError(w, ctx, err)
		return
	}
	partial := job.Status == "accepted" || job.Status == "running"
	missing := []string{}
	if partial {
		missing = []string{"alert.assignment.compensated.v1 consumer receipt"}
	}
	httpx.JSONContractSuccess(w, ctx, job,
		alertBatchContractMeta(ctx, batchID, job.TraceID, job.Revision, partial, missing))
}

type alertBatchCompensationSourceItem struct {
	AlertID              string
	Position             int
	ExpectedStateVersion int64
	RestoreAssignee      string
	RestoreStatus        string
	CurrentAssignee      string
	CurrentStatus        string
}

func (h *AlertBatchAssignmentHandler) createCompensation(
	ctx context.Context,
	command alertBatchCommandContext,
	batchID string,
	request AlertBatchAssignmentCompensationRequest,
) (*AlertBatchAssignmentCompensationReceipt, error) {
	requestSHA, _, err := alertBatchPayloadSHA(request)
	if err != nil {
		return nil, err
	}
	tx, err := h.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		command.TenantID+":assignment-compensation:"+command.IdempotencyKey); err != nil {
		return nil, h.schemaError(err)
	}
	var existingSHA string
	var existingPayload []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256,response_payload FROM alert_assignment_compensation_requests
		WHERE tenant_id=$1 AND idempotency_key=$2`, command.TenantID, command.IdempotencyKey).Scan(&existingSHA, &existingPayload)
	if err == nil {
		if existingSHA != requestSHA {
			return nil, errAlertBatchIdempotencyConflict
		}
		var receipt AlertBatchAssignmentCompensationReceipt
		if err := json.Unmarshal(existingPayload, &receipt); err != nil {
			return nil, err
		}
		if receipt.BatchID != batchID {
			return nil, errAlertBatchIdempotencyConflict
		}
		receipt.Replayed = true
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &receipt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, h.schemaError(err)
	}
	var batchStatus, assignee string
	var batchRevision int64
	var appliedCount int
	err = tx.QueryRowContext(ctx, `SELECT status,revision,applied_count,assignee FROM alert_assignment_batches
		WHERE tenant_id=$1 AND batch_id=$2 FOR UPDATE`, command.TenantID, batchID).Scan(
		&batchStatus, &batchRevision, &appliedCount, &assignee)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errAlertBatchNotFound
	}
	if err != nil {
		return nil, h.schemaError(err)
	}
	if request.ExpectedRevision != batchRevision {
		return nil, fmt.Errorf("%w: expected batch revision %d but authority is %d",
			errAlertBatchCompensationConflict, request.ExpectedRevision, batchRevision)
	}
	if (batchStatus != "completed" && batchStatus != "partial") || batchRevision != 3 || appliedCount < 1 {
		return nil, fmt.Errorf("%w: batch must be terminal at revision 3 with applied items", errAlertBatchCompensationUnavailable)
	}
	var priorRequest string
	err = tx.QueryRowContext(ctx, `SELECT request_id::text FROM alert_assignment_compensation_requests
		WHERE tenant_id=$1 AND batch_id=$2`, command.TenantID, batchID).Scan(&priorRequest)
	if err == nil {
		return nil, fmt.Errorf("%w: compensation request %s already owns this batch", errAlertBatchCompensationConflict, priorRequest)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT alert_id,position,resulting_state_version,previous_assignee,
		previous_status,resulting_assignee,resulting_status FROM alert_assignment_batch_items
		WHERE tenant_id=$1 AND batch_id=$2 AND status='applied' ORDER BY position FOR SHARE`, command.TenantID, batchID)
	if err != nil {
		return nil, err
	}
	items := make([]alertBatchCompensationSourceItem, 0, appliedCount)
	for rows.Next() {
		var item alertBatchCompensationSourceItem
		if err := rows.Scan(&item.AlertID, &item.Position, &item.ExpectedStateVersion, &item.RestoreAssignee,
			&item.RestoreStatus, &item.CurrentAssignee, &item.CurrentStatus); err != nil {
			rows.Close()
			return nil, err
		}
		canonicalStatus, parseErr := alertstate.ParseStatus(item.RestoreStatus)
		if parseErr != nil || canonicalStatus.String() != item.RestoreStatus || item.ExpectedStateVersion <= 0 ||
			item.CurrentAssignee != assignee || item.CurrentStatus != alertstate.StatusAssigned.String() {
			rows.Close()
			return nil, fmt.Errorf("%w: applied item %s lacks exact pre-assignment authority", errAlertBatchCompensationUnavailable, item.AlertID)
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(items) != appliedCount {
		return nil, fmt.Errorf("%w: applied item accounting differs from the batch authority", errAlertBatchCompensationUnavailable)
	}
	now := h.now().UTC()
	requestID := uuid.NewString()
	eventID := uuid.NewString()
	receipt := AlertBatchAssignmentCompensationReceipt{
		RequestID: requestID, JobID: requestID, BatchID: batchID, EventID: eventID,
		ActionID: alertBatchCompensationActionID, ExpectedBatchRevision: batchRevision,
		Status: "accepted", Revision: 1, TotalCount: len(items), AcceptedCount: len(items),
		TraceID: command.TraceID, OutboxStatus: "pending",
	}
	receiptJSON, _ := json.Marshal(receipt)
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_compensation_requests
		(request_id,tenant_id,batch_id,action_id,expected_batch_revision,status,revision,total_count,
		 accepted_count,idempotency_key,request_sha256,requested_by,reason,trace_id,response_payload,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,'accepted',1,$6,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$13)`,
		requestID, command.TenantID, batchID, alertBatchCompensationActionID, batchRevision, len(items),
		command.IdempotencyKey, requestSHA, command.ActorID, request.Reason, command.TraceID, string(receiptJSON), now); err != nil {
		return nil, h.schemaError(err)
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_compensation_items
			(tenant_id,request_id,batch_id,alert_id,position,status,item_revision,expected_state_version,
			 restore_assignee,restore_status,current_assignee,current_status,updated_at)
			VALUES($1,$2,$3,$4,$5,'accepted',1,$6,$7,$8,$9,$10,$11)`, command.TenantID, requestID,
			batchID, item.AlertID, item.Position, item.ExpectedStateVersion, item.RestoreAssignee,
			item.RestoreStatus, item.CurrentAssignee, item.CurrentStatus, now); err != nil {
			return nil, err
		}
		itemHistoryID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-batch-compensation-item-accepted-v1:"+requestID+":"+item.AlertID)).String()
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_compensation_item_history
			(event_id,tenant_id,request_id,batch_id,alert_id,item_revision,previous_status,resulting_status,
			 expected_state_version,actor_id,reason,trace_id,detail,occurred_at)
			VALUES($1,$2,$3,$4,$5,1,'','accepted',$6,$7,$8,$9,'{}'::jsonb,$10)`, itemHistoryID,
			command.TenantID, requestID, batchID, item.AlertID, item.ExpectedStateVersion,
			command.ActorID, request.Reason, command.TraceID, now); err != nil {
			return nil, err
		}
	}
	snapshot, _ := json.Marshal(map[string]interface{}{
		"request_id": requestID, "batch_id": batchID, "expected_batch_revision": batchRevision,
		"status": "accepted", "revision": 1, "total_count": len(items), "trace_id": command.TraceID,
	})
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_compensation_history
		(event_id,tenant_id,request_id,batch_id,revision,previous_status,resulting_status,actor_id,reason,trace_id,snapshot,occurred_at)
		VALUES($1,$2,$3,$4,1,'','accepted',$5,$6,$7,$8::jsonb,$9)`, eventID, command.TenantID,
		requestID, batchID, command.ActorID, request.Reason, command.TraceID, string(snapshot), now); err != nil {
		return nil, err
	}
	eventPayload, _ := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": "alert.batch-assignment.compensation-requested.v1",
		"schema_version": 1, "aggregate_type": "alert_assignment_compensation", "aggregate_id": requestID,
		"aggregate_version": 1, "partition_key": command.TenantID + ":" + batchID, "tenant_id": command.TenantID,
		"batch_id": batchID, "request_id": requestID, "action_id": alertBatchCompensationActionID,
		"expected_batch_revision": batchRevision, "assignee": assignee, "requested_by": command.ActorID,
		"reason": request.Reason, "status": "accepted", "total_count": len(items),
		"trace_id": command.TraceID, "occurred_at": now.Format(time.RFC3339Nano),
	})
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_outbox
		(event_id,tenant_id,batch_id,aggregate_version,aggregate_type,aggregate_id,event_type,schema_version,
		 partition_key,payload,trace_id,status,occurred_at)
		VALUES($1,$2,$3,1,'alert_assignment_compensation',$4,'alert.batch-assignment.compensation-requested.v1',1,
		 $5,$6::jsonb,$7,'pending',$8)`, eventID, command.TenantID, batchID, requestID,
		command.TenantID+":"+batchID, string(eventPayload), command.TraceID, now); err != nil {
		return nil, err
	}
	if err := insertAlertBatchAudit(ctx, tx, command, "ALERT_BATCH_ASSIGNMENT_COMPENSATION_ACCEPTED",
		"alert_assignment_compensation", requestID, map[string]interface{}{
			"batch_id": batchID, "expected_batch_revision": batchRevision, "event_id": eventID,
			"total_count": len(items), "status": "accepted", "outbox_status": "pending",
		}, now); err != nil {
		return nil, h.schemaError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (h *AlertBatchAssignmentHandler) getCompensation(ctx context.Context, tenantID, batchID, requestID string) (*AlertBatchAssignmentCompensationJob, error) {
	job := &AlertBatchAssignmentCompensationJob{Items: []AlertBatchAssignmentCompensationItemResult{}}
	err := h.db.QueryRowContext(ctx, `SELECT request_id::text,request_id::text,batch_id::text,'',action_id,
		expected_batch_revision,status,revision,total_count,accepted_count,compensated_count,conflicted_count,failed_count,
		trace_id,'',false,reason,requested_by,created_at,updated_at
		FROM alert_assignment_compensation_requests WHERE tenant_id=$1 AND batch_id=$2 AND request_id=$3`,
		tenantID, batchID, requestID).Scan(&job.RequestID, &job.JobID, &job.BatchID, &job.EventID, &job.ActionID,
		&job.ExpectedBatchRevision, &job.Status, &job.Revision, &job.TotalCount, &job.AcceptedCount,
		&job.CompensatedCount, &job.ConflictedCount, &job.FailedCount, &job.TraceID, &job.OutboxStatus,
		&job.Replayed, &job.Reason, &job.RequestedBy, &job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errAlertBatchNotFound
	}
	if err != nil {
		return nil, h.schemaError(err)
	}
	rows, err := h.db.QueryContext(ctx, `SELECT alert_id,position,status,item_revision,expected_state_version,
		compensation_state_version,restore_assignee,restore_status,current_assignee,current_status,error_code,error_message,updated_at
		FROM alert_assignment_compensation_items WHERE tenant_id=$1 AND request_id=$2 ORDER BY position`, tenantID, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AlertBatchAssignmentCompensationItemResult
		if err := rows.Scan(&item.AlertID, &item.Position, &item.Status, &item.ItemRevision,
			&item.ExpectedStateVersion, &item.CompensationStateVersion, &item.RestoreAssignee,
			&item.RestoreStatus, &item.CurrentAssignee, &item.CurrentStatus,
			&item.ErrorCode, &item.ErrorMessage, &item.UpdatedAt); err != nil {
			return nil, err
		}
		job.Items = append(job.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := h.db.QueryRowContext(ctx, `SELECT event_id::text,status FROM alert_assignment_batch_outbox
		WHERE tenant_id=$1 AND batch_id=$2 AND aggregate_type='alert_assignment_compensation' AND aggregate_id=$3
		ORDER BY aggregate_version DESC,outbox_id DESC LIMIT 1`, tenantID, batchID, requestID).Scan(&job.EventID, &job.OutboxStatus); err != nil {
		return nil, h.schemaError(err)
	}
	return job, nil
}

func (h *AlertBatchAssignmentHandler) writeCompensationError(w http.ResponseWriter, ctx context.Context, err error) {
	switch {
	case errors.Is(err, errAlertBatchIdempotencyConflict):
		h.writeError(w, ctx, http.StatusConflict, "IDEMPOTENCY_CONFLICT", err.Error())
	case errors.Is(err, errAlertBatchNotFound):
		h.writeError(w, ctx, http.StatusNotFound, "NOT_FOUND", "alert batch assignment not found")
	case errors.Is(err, errAlertBatchCompensationConflict):
		h.writeError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", err.Error())
	case errors.Is(err, errAlertBatchCompensationUnavailable):
		h.writeError(w, ctx, http.StatusConflict, "COMPENSATION_UNAVAILABLE", err.Error())
	case errors.Is(err, errAlertBatchSchemaMissing):
		h.writeError(w, ctx, http.StatusServiceUnavailable, "SCHEMA_UNAVAILABLE", err.Error())
	default:
		h.writeCommandError(w, ctx, err)
	}
}

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

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const dashboardTaskContractVersion = 2

var (
	errDashboardTaskMetadata      = errors.New("dashboard task command metadata invalid")
	errDashboardTaskIdempotency   = errors.New("dashboard task idempotency conflict")
	errDashboardTaskNotFound      = errors.New("dashboard task not found")
	errDashboardTaskSchemaMissing = errors.New("dashboard task schema unavailable")
)

type dashboardTaskActionSpec struct {
	ActionID   string
	TaskType   string
	Target     string
	AuditEvent string
}

var dashboardTaskActionSpecs = map[string]dashboardTaskActionSpec{
	"":           {"dashboard-task-create", "closure", "dashboard", "DASHBOARD_TASK_CREATED"},
	"evidence":   {"dashboard-evidence-task-create", "evidence_repair", "evidence-gap", "DASHBOARD_EVIDENCE_TASK_CREATED"},
	"feedback":   {"dashboard-feedback-task-create", "feedback_replay", "feedback-gap", "DASHBOARD_FEEDBACK_TASK_CREATED"},
	"audit":      {"dashboard-audit-task-create", "audit_repair", "audit-gap", "DASHBOARD_AUDIT_TASK_CREATED"},
	"sla":        {"dashboard-sla-task-create", "sla_followup", "overdue-ticket", "DASHBOARD_SLA_TASK_CREATED"},
	"compliance": {"dashboard-compliance-task-create", "compliance_repair", "compliance-gap", "DASHBOARD_COMPLIANCE_TASK_CREATED"},
}

type DashboardTaskHandler struct {
	db                  *sql.DB
	logger              *zap.Logger
	enabled             bool
	compensationEnabled bool
}

type DashboardTaskCreateRequest struct {
	Target     string                 `json:"target"`
	Priority   string                 `json:"priority"`
	SnapshotID string                 `json:"snapshot_id"`
	Reason     string                 `json:"reason"`
	Context    map[string]interface{} `json:"context,omitempty"`
}

type DashboardTask struct {
	TaskID       string                 `json:"task_id"`
	JobID        string                 `json:"job_id"`
	TenantID     string                 `json:"-"`
	ActionID     string                 `json:"action_id"`
	TaskType     string                 `json:"task_type"`
	Target       string                 `json:"target"`
	Priority     string                 `json:"priority"`
	Status       string                 `json:"status"`
	Revision     int64                  `json:"revision"`
	SnapshotID   string                 `json:"snapshot_id"`
	Reason       string                 `json:"reason"`
	RequestedBy  string                 `json:"requested_by"`
	TraceID      string                 `json:"trace_id"`
	Input        map[string]interface{} `json:"context"`
	Result       map[string]interface{} `json:"result"`
	ErrorCode    string                 `json:"error_code,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	CancelledAt  *time.Time             `json:"cancelled_at,omitempty"`
}

type DashboardTaskReceipt struct {
	TaskID         string `json:"task_id"`
	JobID          string `json:"job_id"`
	EventID        string `json:"event_id"`
	ActionID       string `json:"action_id"`
	Status         string `json:"status"`
	Revision       int64  `json:"revision"`
	SnapshotID     string `json:"snapshot_id"`
	TraceID        string `json:"trace_id"`
	IdempotencyKey string `json:"idempotency_key"`
	OutboxStatus   string `json:"outbox_status"`
	Replayed       bool   `json:"replayed"`
}

type dashboardTaskCreateCommand struct {
	TenantID       string
	ActorID        string
	IdempotencyKey string
	TraceID        string
	SourceIP       string
	UserAgent      string
	Spec           dashboardTaskActionSpec
	Request        DashboardTaskCreateRequest
}

func NewDashboardTaskHandler(db *sql.DB, logger *zap.Logger, enabled bool) *DashboardTaskHandler {
	return &DashboardTaskHandler{db: db, logger: logger, enabled: enabled}
}

func (h *DashboardTaskHandler) EnableCompensation(enabled bool) { h.compensationEnabled = enabled }

func (h *DashboardTaskHandler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/dashboard/tasks", h.Create).Methods(http.MethodPost)
	for _, kind := range []string{"evidence", "feedback", "audit", "sla", "compliance"} {
		router.HandleFunc("/dashboard/tasks/"+kind, h.Create).Methods(http.MethodPost)
	}
	router.HandleFunc("/dashboard/tasks/{task_id}", h.Get).Methods(http.MethodGet)
	router.HandleFunc("/dashboard/tasks/{task_id}/compensations", h.Compensate).Methods(http.MethodPost)
}

func (h *DashboardTaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.enabled {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "FEATURE_DISABLED", "dashboard task v2 is disabled")
		return
	}
	if h.db == nil {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "PostgreSQL is required for dashboard tasks")
		return
	}
	tenantID, actorID, ok := authenticatedDashboardIdentity(ctx)
	if !ok {
		h.writeError(w, ctx, http.StatusUnauthorized, "UNAUTHENTICATED", "authenticated tenant and user are required")
		return
	}
	if !hasSystemPermission(ctx, authmodel.ScopeDashboardWrite) {
		h.writeError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: dashboard:write required")
		return
	}

	kind := strings.TrimPrefix(r.URL.Path, "/api/v1/dashboard/tasks")
	kind = strings.Trim(kind, "/")
	spec, exists := dashboardTaskActionSpecs[kind]
	if !exists {
		h.writeError(w, ctx, http.StatusNotFound, "NOT_FOUND", "dashboard task action is not registered")
		return
	}

	var request DashboardTaskCreateRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid dashboard task request")
		return
	}
	request.Target = strings.TrimSpace(request.Target)
	if request.Target == "" {
		request.Target = spec.Target
	}
	request.Priority = strings.ToLower(strings.TrimSpace(request.Priority))
	if request.Priority == "" {
		request.Priority = "high"
	}
	request.SnapshotID = strings.TrimSpace(request.SnapshotID)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.SnapshotID == "" || request.Reason == "" || !validDashboardPriority(request.Priority) {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "snapshot_id, reason and a valid priority are required")
		return
	}
	if request.Context == nil {
		request.Context = map[string]interface{}{}
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must contain 16 to 200 characters")
		return
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = strings.TrimSpace(httpx.GetRequestID(ctx))
	}
	if traceID == "" {
		traceID = uuid.NewString()
	}

	receipt, err := h.createTask(ctx, dashboardTaskCreateCommand{
		TenantID: tenantID, ActorID: actorID, IdempotencyKey: idempotencyKey,
		TraceID: traceID, SourceIP: requestSourceIP(r), UserAgent: r.UserAgent(),
		Spec: spec, Request: request,
	})
	if err != nil {
		switch {
		case errors.Is(err, errDashboardTaskIdempotency):
			h.writeError(w, ctx, http.StatusConflict, "IDEMPOTENCY_CONFLICT", err.Error())
		case errors.Is(err, errDashboardTaskMetadata):
			h.writeError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		case errors.Is(err, errDashboardTaskSchemaMissing):
			h.writeError(w, ctx, http.StatusServiceUnavailable, "SCHEMA_UNAVAILABLE", err.Error())
		default:
			h.logger.Error("Failed to create dashboard task", zap.Error(err), zap.String("tenant_id", tenantID), zap.String("action_id", spec.ActionID), zap.String("trace_id", traceID))
			h.writeError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to create dashboard task")
		}
		return
	}
	h.writeAccepted(w, ctx, receipt)
}

func (h *DashboardTaskHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.db == nil || !h.enabled {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "dashboard task service is unavailable")
		return
	}
	tenantID, _, ok := authenticatedDashboardIdentity(ctx)
	if !ok {
		h.writeError(w, ctx, http.StatusUnauthorized, "UNAUTHENTICATED", "authenticated tenant and user are required")
		return
	}
	if !hasAnySystemPermission(ctx, authmodel.ScopeDashboardWrite, authmodel.ScopeDashboardAll, authmodel.ScopeAdminAll) {
		h.writeError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: dashboard:write required")
		return
	}
	taskID := strings.TrimSpace(mux.Vars(r)["task_id"])
	if _, err := uuid.Parse(taskID); err != nil {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_TASK_ID", "task_id must be a UUID")
		return
	}
	task, err := h.getTask(ctx, tenantID, taskID)
	if err != nil {
		if errors.Is(err, errDashboardTaskNotFound) {
			h.writeError(w, ctx, http.StatusNotFound, "NOT_FOUND", "dashboard task not found")
			return
		}
		h.logger.Error("Failed to read dashboard task", zap.Error(err), zap.String("tenant_id", tenantID), zap.String("task_id", taskID))
		h.writeError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to read dashboard task")
		return
	}
	h.writeSuccess(w, ctx, task)
}

func (h *DashboardTaskHandler) createTask(ctx context.Context, command dashboardTaskCreateCommand) (*DashboardTaskReceipt, error) {
	if command.TenantID == "" || command.ActorID == "" || command.Spec.ActionID == "" || command.TraceID == "" {
		return nil, errDashboardTaskMetadata
	}
	normalized, err := json.Marshal(struct {
		ActionID string                     `json:"action_id"`
		Request  DashboardTaskCreateRequest `json:"request"`
	}{command.Spec.ActionID, command.Request})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(normalized)
	requestSHA := hex.EncodeToString(digest[:])

	tx, err := h.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var existingSHA string
	var existingPayload []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256,response_payload FROM dashboard_task_requests
		WHERE tenant_id=$1 AND idempotency_key=$2`, command.TenantID, command.IdempotencyKey).Scan(&existingSHA, &existingPayload)
	if err == nil {
		if existingSHA != requestSHA {
			return nil, errDashboardTaskIdempotency
		}
		var receipt DashboardTaskReceipt
		if err := json.Unmarshal(existingPayload, &receipt); err != nil {
			return nil, fmt.Errorf("decode dashboard task replay receipt: %w", err)
		}
		receipt.Replayed = true
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &receipt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		if isUndefinedTable(err) {
			return nil, fmt.Errorf("%w: apply migration 202608031620", errDashboardTaskSchemaMissing)
		}
		return nil, err
	}

	taskID := uuid.NewString()
	eventID := uuid.NewString()
	now := time.Now().UTC()
	inputJSON, err := json.Marshal(command.Request.Context)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_tasks
		(task_id,tenant_id,action_id,task_type,target,priority,status,revision,snapshot_id,reason,requested_by,trace_id,input,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,'accepted',1,$7,$8,$9,$10,$11::jsonb,$12,$12)`,
		taskID, command.TenantID, command.Spec.ActionID, command.Spec.TaskType, command.Request.Target,
		command.Request.Priority, command.Request.SnapshotID, command.Request.Reason, command.ActorID,
		command.TraceID, string(inputJSON), now); err != nil {
		return nil, fmt.Errorf("insert dashboard task: %w", err)
	}
	snapshot := map[string]interface{}{
		"task_id": taskID, "action_id": command.Spec.ActionID, "task_type": command.Spec.TaskType,
		"target": command.Request.Target, "priority": command.Request.Priority, "status": "accepted",
		"revision": 1, "snapshot_id": command.Request.SnapshotID, "trace_id": command.TraceID,
	}
	snapshotJSON, _ := json.Marshal(snapshot)
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_history
		(event_id,tenant_id,task_id,revision,action_id,previous_status,resulting_status,actor_id,reason,trace_id,snapshot,occurred_at)
		VALUES ($1,$2,$3,1,$4,'','accepted',$5,$6,$7,$8::jsonb,$9)`, eventID, command.TenantID,
		taskID, command.Spec.ActionID, command.ActorID, command.Request.Reason, command.TraceID, string(snapshotJSON), now); err != nil {
		return nil, fmt.Errorf("insert dashboard task history: %w", err)
	}
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": "traffic.dashboard.v1.TaskRequested", "schema_version": 1,
		"aggregate_type": "dashboard_task", "aggregate_id": taskID, "aggregate_version": 1,
		"partition_key": command.TenantID + ":" + taskID,
		"tenant_id":     command.TenantID, "task_id": taskID,
		"action_id": command.Spec.ActionID, "status": "accepted", "snapshot_id": command.Request.SnapshotID,
		"trace_id": command.TraceID, "occurred_at": now.Format(time.RFC3339Nano),
	}
	payloadJSON, _ := json.Marshal(payload)
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_outbox
		(event_id,tenant_id,task_id,aggregate_version,event_type,schema_version,partition_key,payload,trace_id,status,occurred_at)
		VALUES ($1,$2,$3,1,'traffic.dashboard.v1.TaskRequested',1,$4,$5::jsonb,$6,'pending',$7)`,
		eventID, command.TenantID, taskID, command.TenantID+":"+taskID, string(payloadJSON), command.TraceID, now); err != nil {
		return nil, fmt.Errorf("insert dashboard task outbox: %w", err)
	}
	if err := insertDashboardTaskAudit(ctx, tx, command, taskID, eventID, now); err != nil {
		return nil, err
	}
	receipt := DashboardTaskReceipt{
		TaskID: taskID, JobID: taskID, EventID: eventID, ActionID: command.Spec.ActionID,
		Status: "accepted", Revision: 1, SnapshotID: command.Request.SnapshotID, TraceID: command.TraceID,
		IdempotencyKey: command.IdempotencyKey, OutboxStatus: "pending", Replayed: false,
	}
	receiptJSON, _ := json.Marshal(receipt)
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_requests
		(tenant_id,idempotency_key,request_sha256,action_id,task_id,resulting_revision,event_id,trace_id,response_payload,created_at)
		VALUES ($1,$2,$3,$4,$5,1,$6,$7,$8::jsonb,$9)`, command.TenantID, command.IdempotencyKey,
		requestSHA, command.Spec.ActionID, taskID, eventID, command.TraceID, string(receiptJSON), now); err != nil {
		return nil, fmt.Errorf("insert dashboard task request receipt: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (h *DashboardTaskHandler) getTask(ctx context.Context, tenantID, taskID string) (*DashboardTask, error) {
	var task DashboardTask
	var inputJSON, resultJSON []byte
	err := h.db.QueryRowContext(ctx, `SELECT task_id::text,tenant_id,action_id,task_type,target,priority,status,revision,
		snapshot_id,reason,requested_by,trace_id,input,result,error_code,error_message,created_at,updated_at,
		started_at,completed_at,cancelled_at FROM dashboard_tasks WHERE tenant_id=$1 AND task_id=$2`, tenantID, taskID).Scan(
		&task.TaskID, &task.TenantID, &task.ActionID, &task.TaskType, &task.Target, &task.Priority, &task.Status,
		&task.Revision, &task.SnapshotID, &task.Reason, &task.RequestedBy, &task.TraceID, &inputJSON, &resultJSON,
		&task.ErrorCode, &task.ErrorMessage, &task.CreatedAt, &task.UpdatedAt, &task.StartedAt, &task.CompletedAt, &task.CancelledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errDashboardTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	task.JobID = task.TaskID
	if err := json.Unmarshal(inputJSON, &task.Input); err != nil {
		return nil, fmt.Errorf("decode dashboard task input: %w", err)
	}
	if err := json.Unmarshal(resultJSON, &task.Result); err != nil {
		return nil, fmt.Errorf("decode dashboard task result: %w", err)
	}
	return &task, nil
}

func insertDashboardTaskAudit(ctx context.Context, tx *sql.Tx, command dashboardTaskCreateCommand, taskID, eventID string, occurredAt time.Time) error {
	detail := map[string]interface{}{
		"event_id": eventID, "action_id": command.Spec.ActionID, "job_id": taskID,
		"task_type": command.Spec.TaskType, "target": command.Request.Target, "priority": command.Request.Priority,
		"snapshot_id": command.Request.SnapshotID, "trace_id": command.TraceID, "result": "accepted",
		"idempotency_key": command.IdempotencyKey,
	}
	detailJSON, _ := json.Marshal(detail)
	var dataType string
	err := tx.QueryRowContext(ctx, `SELECT data_type FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='audit_logs' AND column_name='user_id'`).Scan(&dataType)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect dashboard audit user column: %w", err)
	}
	userIDExpr := "$3"
	actorID := command.ActorID
	if dataType == "uuid" {
		userIDExpr = "NULLIF($3,'')::uuid"
		if _, parseErr := uuid.Parse(actorID); parseErr != nil {
			actorID = ""
		}
	}
	query := `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent,trace_id,result,success,created_at)
		VALUES ($1,$2,` + userIDExpr + `,$4,'dashboard_task',$5,$6::jsonb,$7,$8,$9,'accepted',true,$10)`
	if _, err := tx.ExecContext(ctx, query, "audit-"+eventID, command.TenantID, actorID, command.Spec.AuditEvent,
		taskID, string(detailJSON), command.SourceIP, command.UserAgent, command.TraceID, occurredAt); err != nil {
		return fmt.Errorf("insert dashboard task audit: %w", err)
	}
	return nil
}

func authenticatedDashboardIdentity(ctx context.Context) (string, string, bool) {
	if claims := httpx.GetExtendedClaims(ctx); claims != nil {
		tenantID := strings.TrimSpace(claims.GetTenantID())
		actorID := strings.TrimSpace(claims.GetUserID())
		return tenantID, actorID, tenantID != "" && actorID != ""
	}
	tenantID := strings.TrimSpace(httpx.GetTenantID(ctx))
	actorID := strings.TrimSpace(httpx.GetUserID(ctx))
	return tenantID, actorID, tenantID != "" && actorID != ""
}

func validDashboardPriority(priority string) bool {
	switch priority {
	case "low", "medium", "high", "critical":
		return true
	default:
		return false
	}
}

func isUndefinedTable(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "does not exist")
}

func requestSourceIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func dashboardTaskContractMeta(ctx context.Context, snapshotID string, revision int64) httpx.ContractMeta {
	return httpx.ContractMeta{
		ContractVersion: dashboardTaskContractVersion, SnapshotID: snapshotID,
		AsOf: time.Now().UTC().Format(time.RFC3339Nano), TraceID: httpx.GetTraceID(ctx),
		Partial: false, MissingSections: []string{},
		SourceWatermarks: map[string]string{"postgresql.dashboard_tasks.revision": fmt.Sprintf("%d", revision)},
	}
}

func (h *DashboardTaskHandler) writeAccepted(w http.ResponseWriter, ctx context.Context, receipt *DashboardTaskReceipt) {
	httpx.JSONContractAccepted(w, ctx, receipt, dashboardTaskContractMeta(ctx, receipt.SnapshotID, receipt.Revision))
}

func (h *DashboardTaskHandler) writeSuccess(w http.ResponseWriter, ctx context.Context, task *DashboardTask) {
	httpx.JSONContractSuccess(w, ctx, task, dashboardTaskContractMeta(ctx, task.SnapshotID, task.Revision))
}

func (h *DashboardTaskHandler) writeError(w http.ResponseWriter, ctx context.Context, status int, code, message string) {
	httpx.JSONError(w, ctx, status, code, message)
}

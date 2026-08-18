package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

const (
	ForensicsTaskCreateAction   = "forensics.pcap-cut.create"
	ForensicsTaskCancelAction   = "forensics.pcap-cut.cancel"
	ForensicsTaskRetryAction    = "forensics.pcap-cut.retry"
	forensicsTaskLeaseAction    = "forensics.pcap-cut.worker-lease"
	forensicsTaskProgressAction = "forensics.pcap-cut.worker-progress"
	forensicsTaskCompleteAction = "forensics.pcap-cut.worker-complete"
	forensicsTaskFailAction     = "forensics.pcap-cut.worker-fail"
	forensicsTaskRecoverAction  = "forensics.pcap-cut.worker-recover"
	forensicsTaskArchiveAction  = "forensics.pcap-cut.retention-archive"
)

// TaskCommandMeta is the immutable identity and concurrency contract carried by
// every task mutation. Old callers are preserved in CompatibilityMode, while
// HTTP callers can provide durable identity and an expected resource revision.
type TaskCommandMeta struct {
	TenantID          string
	ActorID           string
	ActionID          string
	IdempotencyKey    string
	ExpectedRevision  *int64
	Reason            string
	TraceID           string
	SourceIP          string
	UserAgent         string
	CompatibilityMode bool
}

type TaskCommandReceipt struct {
	TaskID            string    `json:"task_id"`
	Status            string    `json:"status"`
	Revision          int64     `json:"revision"`
	EventID           string    `json:"event_id"`
	ActionID          string    `json:"action_id"`
	IdempotencyKey    string    `json:"idempotency_key"`
	OutboxStatus      string    `json:"outbox_status"`
	CreatedAt         time.Time `json:"created_at"`
	Replayed          bool      `json:"replayed"`
	CompatibilityMode bool      `json:"compatibility_mode"`
}

type taskMutation struct {
	Operation     string `json:"operation"`
	Status        string `json:"status,omitempty"`
	Progress      *int   `json:"progress,omitempty"`
	Packets       *int64 `json:"packets,omitempty"`
	Bytes         *int64 `json:"bytes,omitempty"`
	FilesScanned  *int   `json:"files_scanned,omitempty"`
	ResultFileKey string `json:"result_file_key,omitempty"`
	ResultSHA256  string `json:"result_sha256,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	Completed     bool   `json:"completed,omitempty"`
	Archive       bool   `json:"archive,omitempty"`
}

func (r *TaskRepository) CreateAtomic(ctx context.Context, task *Task, meta TaskCommandMeta) (*TaskCommandReceipt, error) {
	if task == nil || strings.TrimSpace(task.TenantID) == "" {
		return nil, commonerrors.New(commonerrors.ErrCodeInvalidParameter, "task and tenant_id are required")
	}
	if task.TaskType == "" {
		task.TaskType = TaskTypePcapCut
	}
	if task.Status == "" {
		task.Status = TaskStatusQueued
	}
	if task.Status != TaskStatusQueued {
		return nil, commonerrors.New(commonerrors.ErrCodeInvalidStateTransition, "new task must start queued")
	}
	if len(task.ParamsJSON) == 0 {
		task.ParamsJSON = []byte(`{}`)
	}
	if !json.Valid(task.ParamsJSON) {
		return nil, commonerrors.New(commonerrors.ErrCodeInvalidFormat, "task params must be valid JSON")
	}
	if task.Progress < 0 || task.Progress > 100 {
		return nil, commonerrors.New(commonerrors.ErrCodeOutOfRange, "task progress must be between 0 and 100")
	}
	meta = normalizeTaskCommandMeta(meta, task.TenantID, task.TaskID, ForensicsTaskCreateAction, "pcap cut task accepted", map[string]interface{}{
		"task_type": task.TaskType, "params": json.RawMessage(task.ParamsJSON), "created_by": task.CreatedBy,
	})
	if err := validateTaskCommandMeta(meta); err != nil {
		return nil, err
	}
	if meta.TenantID != task.TenantID {
		return nil, commonerrors.New(commonerrors.ErrCodePermissionDenied, "task tenant must match authenticated command tenant")
	}
	if !meta.CompatibilityMode && (strings.TrimSpace(meta.ActorID) == "" || strings.TrimSpace(task.CreatedBy) != strings.TrimSpace(meta.ActorID)) {
		return nil, commonerrors.New(commonerrors.ErrCodePermissionDenied, "task creator must match authenticated command actor")
	}
	if meta.ExpectedRevision != nil && *meta.ExpectedRevision != 0 {
		return nil, commonerrors.New(commonerrors.ErrCodeVersionConflict, "new task expected_revision must be 0")
	}
	if !meta.CompatibilityMode {
		task.TaskID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("forensics-task\x00"+meta.TenantID+"\x00"+meta.IdempotencyKey)).String()
	} else if task.TaskID == "" {
		task.TaskID = uuid.New().String()
	}
	requestHash, err := taskCommandHash(meta.ActionID, "create", map[string]interface{}{
		"tenant_id": task.TenantID, "task_type": task.TaskType, "status": task.Status,
		"progress": task.Progress, "params": json.RawMessage(task.ParamsJSON), "created_by": task.CreatedBy,
		"expected_revision": int64(0),
	})
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to hash task creation")
	}

	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to begin task creation")
	}
	defer tx.Rollback()

	if receipt, found, replayErr := loadTaskCommandReplay(ctx, tx, meta.TenantID, meta.IdempotencyKey, requestHash); replayErr != nil {
		return nil, replayErr
	} else if found {
		receipt.Replayed = true
		task.TaskID, task.Status, task.Revision, task.CreatedAt, task.UpdatedAt = receipt.TaskID, receipt.Status, receipt.Revision, receipt.CreatedAt, receipt.CreatedAt
		if err := tx.Commit(); err != nil {
			return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to commit task replay")
		}
		return receipt, nil
	}

	now := time.Now().UTC()
	task.Revision, task.CreatedAt, task.UpdatedAt = 1, now, now
	if _, err = tx.ExecContext(ctx, `INSERT INTO tasks (
		task_id,tenant_id,task_type,status,progress,params,result_file_key,result_sha256,
		result_packets,result_bytes,files_scanned,error_message,run_id,created_by,
		created_at,updated_at,revision,last_action_id,last_trace_id
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$15,1,$16,$17)`,
		task.TaskID, task.TenantID, task.TaskType, task.Status, task.Progress, task.ParamsJSON,
		task.ResultFileKey, task.ResultSHA256, task.ResultPackets, task.ResultBytes, task.FilesScanned,
		task.ErrorMessage, task.RunID, task.CreatedBy, now, meta.ActionID, meta.TraceID); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to create task")
	}
	receipt, err := persistTaskCommand(ctx, tx, task, meta, requestHash, "create", "", now)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to commit task creation")
	}
	return receipt, nil
}

func (r *TaskRepository) mutateTaskAtomic(ctx context.Context, taskID string, meta TaskCommandMeta, mutation taskMutation) (*TaskCommandReceipt, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(mutation.Operation) == "" {
		return nil, commonerrors.New(commonerrors.ErrCodeInvalidParameter, "task_id and operation are required")
	}
	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to begin task command")
	}
	defer tx.Rollback()

	task, err := lockTaskForCommand(ctx, tx, strings.TrimSpace(meta.TenantID), taskID)
	if err != nil {
		return nil, err
	}
	meta = normalizeTaskCommandMeta(meta, task.TenantID, task.TaskID, actionForTaskMutation(mutation), reasonForTaskMutation(mutation), map[string]interface{}{
		"revision": task.Revision, "mutation": mutation,
	})
	if err = validateTaskCommandMeta(meta); err != nil {
		return nil, err
	}
	expectedForHash := task.Revision
	if meta.ExpectedRevision != nil {
		expectedForHash = *meta.ExpectedRevision
	}
	requestHash, err := taskCommandHash(meta.ActionID, mutation.Operation, map[string]interface{}{
		"task_id": task.TaskID, "expected_revision": expectedForHash, "mutation": mutation,
	})
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to hash task command")
	}
	if receipt, found, replayErr := loadTaskCommandReplay(ctx, tx, task.TenantID, meta.IdempotencyKey, requestHash); replayErr != nil {
		return nil, replayErr
	} else if found {
		receipt.Replayed = true
		if err := tx.Commit(); err != nil {
			return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to commit task replay")
		}
		return receipt, nil
	}
	expected := task.Revision
	if meta.ExpectedRevision != nil {
		expected = *meta.ExpectedRevision
	}
	if expected != task.Revision {
		return nil, commonerrors.Newf(commonerrors.ErrCodeVersionConflict,
			"task revision conflict: expected=%d actual=%d", expected, task.Revision)
	}
	meta.ExpectedRevision = &expected
	receipt, err := applyLockedTaskMutation(ctx, tx, task, meta, requestHash, mutation, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to commit task command")
	}
	return receipt, nil
}

// CancelForTenant binds authorization scope, optimistic revision and the
// cancellation effect to the same command transaction.
func (r *TaskRepository) CancelForTenant(ctx context.Context, tenantID, taskID string, meta TaskCommandMeta) (*TaskCommandReceipt, error) {
	meta.TenantID = tenantID
	return r.mutateTaskAtomic(ctx, taskID, meta, taskMutation{Operation: "cancel", Status: TaskStatusCancelled, Completed: true})
}

// RetryForTenant requeues the same immutable request. It never creates a
// second task or rewrites params, so a retry cannot change the original
// evidence selection after admission.
func (r *TaskRepository) RetryForTenant(ctx context.Context, tenantID, taskID string, meta TaskCommandMeta) (*TaskCommandReceipt, error) {
	meta.TenantID = tenantID
	return r.mutateTaskAtomic(ctx, taskID, meta, taskMutation{Operation: "retry", Status: TaskStatusQueued})
}

func (r *TaskRepository) leasePendingTasksAtomic(ctx context.Context, limit int) ([]*Task, error) {
	if limit <= 0 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to begin task lease")
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT task_id,tenant_id,task_type,status,progress,params,
		result_file_key,result_sha256,result_packets,result_bytes,files_scanned,error_message,
		run_id,created_by,created_at,updated_at,completed_at,revision,deleted_at
		FROM tasks WHERE status=$1 AND deleted_at IS NULL
		ORDER BY created_at,task_id LIMIT $2 FOR UPDATE SKIP LOCKED`, TaskStatusQueued, limit)
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to select queued tasks")
	}
	defer rows.Close()
	selected := make([]*Task, 0, limit)
	for rows.Next() {
		task, scanErr := scanTaskRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		selected = append(selected, task)
	}
	if err = rows.Err(); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to scan queued tasks")
	}
	if err = rows.Close(); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to close task lease rows")
	}
	for _, task := range selected {
		mutation := taskMutation{Operation: "status", Status: TaskStatusProcessing}
		meta := internalTaskMeta(task, forensicsTaskLeaseAction, task.Revision, mutation)
		requestHash, hashErr := taskCommandHash(meta.ActionID, mutation.Operation, map[string]interface{}{"task_id": task.TaskID, "expected_revision": task.Revision, "mutation": mutation})
		if hashErr != nil {
			return nil, commonerrors.Wrap(hashErr, commonerrors.ErrCodeSerializationError, "failed to hash task lease")
		}
		if _, err = applyLockedTaskMutation(ctx, tx, task, meta, requestHash, mutation, time.Now().UTC()); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to commit task lease")
	}
	return selected, nil
}

func (r *TaskRepository) archiveOldTasksAtomic(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.batchTaskMutationAtomic(ctx, `status IN ('completed','partial','failed','cancelled') AND completed_at IS NOT NULL AND completed_at < $1`,
		cutoff, taskMutation{Operation: "archive", Archive: true}, forensicsTaskArchiveAction)
}

func (r *TaskRepository) recoverStuckTasksAtomic(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.batchTaskMutationAtomic(ctx, `status='processing' AND updated_at < $1`,
		cutoff, taskMutation{Operation: "recover", Status: TaskStatusQueued}, forensicsTaskRecoverAction)
}

func (r *TaskRepository) batchTaskMutationAtomic(ctx context.Context, predicate string, cutoff time.Time, mutation taskMutation, actionID string) (int64, error) {
	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to begin task batch command")
	}
	defer tx.Rollback()
	query := `SELECT task_id,tenant_id,task_type,status,progress,params,result_file_key,result_sha256,
		result_packets,result_bytes,files_scanned,error_message,run_id,created_by,created_at,updated_at,
		completed_at,revision,deleted_at FROM tasks WHERE deleted_at IS NULL AND ` + predicate + `
		ORDER BY updated_at,task_id LIMIT 500 FOR UPDATE SKIP LOCKED`
	rows, err := tx.QueryContext(ctx, query, cutoff)
	if err != nil {
		return 0, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to select task batch")
	}
	selected := make([]*Task, 0, 64)
	for rows.Next() {
		task, scanErr := scanTaskRecord(rows)
		if scanErr != nil {
			rows.Close()
			return 0, scanErr
		}
		selected = append(selected, task)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to scan task batch")
	}
	if err = rows.Close(); err != nil {
		return 0, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to close task batch rows")
	}
	for _, task := range selected {
		meta := internalTaskMeta(task, actionID, task.Revision, mutation)
		requestHash, hashErr := taskCommandHash(meta.ActionID, mutation.Operation, map[string]interface{}{"task_id": task.TaskID, "expected_revision": task.Revision, "mutation": mutation})
		if hashErr != nil {
			return 0, commonerrors.Wrap(hashErr, commonerrors.ErrCodeSerializationError, "failed to hash task batch command")
		}
		if _, err = applyLockedTaskMutation(ctx, tx, task, meta, requestHash, mutation, time.Now().UTC()); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to commit task batch command")
	}
	return int64(len(selected)), nil
}

func applyLockedTaskMutation(ctx context.Context, tx *sql.Tx, task *Task, meta TaskCommandMeta, requestHash string, mutation taskMutation, now time.Time) (*TaskCommandReceipt, error) {
	previousStatus := task.Status
	if err := applyTaskMutation(task, mutation, now); err != nil {
		return nil, err
	}
	previousRevision := task.Revision
	task.Revision++
	task.UpdatedAt = now
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET
		status=$4,progress=$5,result_file_key=$6,result_sha256=$7,result_packets=$8,
		result_bytes=$9,files_scanned=$10,error_message=$11,updated_at=$12,completed_at=$13,
		deleted_at=$14,revision=$15,last_action_id=$16,last_trace_id=$17
		WHERE tenant_id=$1 AND task_id=$2 AND revision=$3 AND deleted_at IS NULL`,
		task.TenantID, task.TaskID, previousRevision, task.Status, task.Progress,
		task.ResultFileKey, task.ResultSHA256, task.ResultPackets, task.ResultBytes,
		task.FilesScanned, task.ErrorMessage, now, task.CompletedAt, task.DeletedAt,
		task.Revision, meta.ActionID, meta.TraceID)
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to update task command")
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return nil, commonerrors.New(commonerrors.ErrCodeConcurrentModify, "task changed while command was committing")
	}
	return persistTaskCommand(ctx, tx, task, meta, requestHash, mutation.Operation, previousStatus, now)
}

func applyTaskMutation(task *Task, mutation taskMutation, now time.Time) error {
	switch mutation.Operation {
	case "status":
		if err := validateTaskStatusTransition(task.Status, mutation.Status); err != nil {
			return err
		}
		task.Status = mutation.Status
	case "progress":
		if task.Status != TaskStatusProcessing {
			return commonerrors.Newf(commonerrors.ErrCodeInvalidStateTransition, "cannot update progress in status: %s", task.Status)
		}
		if mutation.Progress == nil || *mutation.Progress < 0 || *mutation.Progress > 99 {
			return commonerrors.New(commonerrors.ErrCodeOutOfRange, "processing progress must be between 0 and 99")
		}
		if *mutation.Progress < task.Progress {
			return commonerrors.Newf(commonerrors.ErrCodeInvalidStateTransition,
				"task progress cannot decrease: current=%d requested=%d", task.Progress, *mutation.Progress)
		}
		task.Progress = *mutation.Progress
		if mutation.Packets != nil {
			task.ResultPackets = *mutation.Packets
		}
	case "complete":
		if task.Status != TaskStatusProcessing {
			return commonerrors.Newf(commonerrors.ErrCodeInvalidStateTransition, "cannot complete task in status: %s", task.Status)
		}
		if mutation.Status != TaskStatusCompleted && mutation.Status != TaskStatusPartial {
			return commonerrors.New(commonerrors.ErrCodeInvalidParameter, "task completion status must be completed or partial")
		}
		task.Status, task.Progress = mutation.Status, 100
		task.ResultFileKey, task.ResultSHA256 = mutation.ResultFileKey, mutation.ResultSHA256
		if mutation.Packets != nil {
			task.ResultPackets = *mutation.Packets
		}
		if mutation.Bytes != nil {
			task.ResultBytes = *mutation.Bytes
		}
		if mutation.FilesScanned != nil {
			task.FilesScanned = *mutation.FilesScanned
		}
		task.CompletedAt = &now
	case "fail":
		if task.Status != TaskStatusQueued && task.Status != TaskStatusProcessing {
			return commonerrors.Newf(commonerrors.ErrCodeInvalidStateTransition, "cannot fail task in status: %s", task.Status)
		}
		task.Status, task.ErrorMessage, task.CompletedAt = TaskStatusFailed, mutation.ErrorMessage, &now
	case "cancel":
		if task.Status != TaskStatusQueued && task.Status != TaskStatusProcessing {
			return commonerrors.Newf(commonerrors.ErrCodeInvalidStateTransition, "cannot cancel task in status: %s", task.Status)
		}
		task.Status, task.CompletedAt = TaskStatusCancelled, &now
	case "retry":
		if task.Status != TaskStatusFailed && task.Status != TaskStatusCancelled {
			return commonerrors.Newf(commonerrors.ErrCodeInvalidStateTransition, "cannot retry task in status: %s", task.Status)
		}
		task.Status, task.Progress, task.ErrorMessage, task.CompletedAt = TaskStatusQueued, 0, "", nil
		task.ResultFileKey, task.ResultSHA256 = "", ""
		task.ResultPackets, task.ResultBytes, task.FilesScanned = 0, 0, 0
	case "recover":
		if task.Status != TaskStatusProcessing {
			return commonerrors.Newf(commonerrors.ErrCodeInvalidStateTransition, "cannot recover task in status: %s", task.Status)
		}
		task.Status, task.CompletedAt = TaskStatusQueued, nil
	case "archive":
		if task.Status != TaskStatusCompleted && task.Status != TaskStatusPartial && task.Status != TaskStatusFailed && task.Status != TaskStatusCancelled {
			return commonerrors.Newf(commonerrors.ErrCodeInvalidStateTransition, "cannot archive task in status: %s", task.Status)
		}
		task.DeletedAt = &now
	default:
		return commonerrors.Newf(commonerrors.ErrCodeInvalidParameter, "unknown task operation: %s", mutation.Operation)
	}
	return nil
}

func validateTaskStatusTransition(current, target string) error {
	if current == target && current == TaskStatusProcessing {
		return nil
	}
	allowed := map[string]map[string]bool{
		TaskStatusQueued:     {TaskStatusProcessing: true, TaskStatusCancelled: true, TaskStatusFailed: true},
		TaskStatusProcessing: {TaskStatusCompleted: true, TaskStatusFailed: true, TaskStatusCancelled: true, TaskStatusQueued: true},
	}
	if !allowed[current][target] {
		return commonerrors.Newf(commonerrors.ErrCodeInvalidStateTransition, "cannot transition task from %s to %s", current, target)
	}
	return nil
}

func persistTaskCommand(ctx context.Context, tx *sql.Tx, task *Task, meta TaskCommandMeta, requestHash, operation, previousStatus string, now time.Time) (*TaskCommandReceipt, error) {
	eventID := deterministicTaskEventID(task.TenantID, meta.IdempotencyKey)
	snapshot := taskSnapshot(task)
	payload := map[string]interface{}{
		"event_id": eventID.String(), "tenant_id": task.TenantID, "task_id": task.TaskID,
		"aggregate_version": task.Revision, "event_type": "traffic.forensics.task.v1." + operation,
		"schema_version": 1, "action_id": meta.ActionID, "operation": operation,
		"reason": meta.Reason, "trace_id": meta.TraceID, "task": snapshot,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to marshal task event")
	}
	snapshotJSON, _ := json.Marshal(snapshot)
	if _, err = tx.ExecContext(ctx, `INSERT INTO forensics_task_outbox
		(event_id,tenant_id,task_id,aggregate_version,event_type,schema_version,partition_key,payload,trace_id,occurred_at)
		VALUES ($1,$2,$3,$4,$5,1,$6,$7::jsonb,$8,$9)`, eventID, task.TenantID, task.TaskID,
		task.Revision, "traffic.forensics.task.v1."+operation, task.TenantID+":"+task.TaskID,
		string(payloadJSON), meta.TraceID, now); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist task outbox")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO forensics_task_history
		(event_id,tenant_id,task_id,revision,action_id,operation,previous_status,resulting_status,
		 actor_id,reason,trace_id,compatibility_mode,snapshot,occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14)`,
		eventID, task.TenantID, task.TaskID, task.Revision, meta.ActionID, operation,
		previousStatus, task.Status, meta.ActorID, meta.Reason, meta.TraceID,
		meta.CompatibilityMode, string(snapshotJSON), now); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist task history")
	}
	auditDetail, _ := json.Marshal(map[string]interface{}{
		"action_id": meta.ActionID, "operation": operation, "reason": meta.Reason,
		"revision": task.Revision, "event_id": eventID.String(), "previous_status": previousStatus,
		"resulting_status": task.Status, "compatibility_mode": meta.CompatibilityMode,
	})
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent,
		 request_id,trace_id,success,risk_level,result)
		VALUES ($1,$2,NULLIF($3,''),$4,'pcap_task',$5,$6::jsonb,NULLIF($7,''),NULLIF($8,''),$9,$9,true,'medium','success')`,
		"audit-"+eventID.String(), task.TenantID, meta.ActorID, meta.ActionID, task.TaskID,
		string(auditDetail), meta.SourceIP, meta.UserAgent, meta.TraceID); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist task audit")
	}
	receipt := &TaskCommandReceipt{
		TaskID: task.TaskID, Status: task.Status, Revision: task.Revision, EventID: eventID.String(),
		ActionID: meta.ActionID, IdempotencyKey: meta.IdempotencyKey,
		OutboxStatus: "pending", CreatedAt: task.CreatedAt, CompatibilityMode: meta.CompatibilityMode,
	}
	responseJSON, _ := json.Marshal(receipt)
	expected := int64(0)
	if meta.ExpectedRevision != nil {
		expected = *meta.ExpectedRevision
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO forensics_task_requests
		(tenant_id,idempotency_key,request_sha256,action_id,operation,task_id,expected_revision,
		 resulting_revision,event_id,reason,trace_id,compatibility_mode,response_payload,created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14)`,
		task.TenantID, meta.IdempotencyKey, requestHash, meta.ActionID, operation, task.TaskID,
		expected, task.Revision, eventID, meta.Reason, meta.TraceID, meta.CompatibilityMode,
		string(responseJSON), now); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist task request")
	}
	return receipt, nil
}

func loadTaskCommandReplay(ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey, requestHash string) (*TaskCommandReceipt, bool, error) {
	var priorHash, response string
	err := tx.QueryRowContext(ctx, `SELECT request_sha256,response_payload::text FROM forensics_task_requests
		WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`, tenantID, idempotencyKey).Scan(&priorHash, &response)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to inspect task replay")
	}
	if priorHash != requestHash {
		return nil, false, commonerrors.New(commonerrors.ErrCodeDedupConflict, "Idempotency-Key was used for a different task command")
	}
	var receipt TaskCommandReceipt
	if err = json.Unmarshal([]byte(response), &receipt); err != nil {
		return nil, false, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to decode task replay")
	}
	return &receipt, true, nil
}

func lockTaskForCommand(ctx context.Context, tx *sql.Tx, tenantID, taskID string) (*Task, error) {
	query := `SELECT task_id,tenant_id,task_type,status,progress,params,result_file_key,result_sha256,
		result_packets,result_bytes,files_scanned,error_message,run_id,created_by,created_at,updated_at,
		completed_at,revision,deleted_at FROM tasks WHERE task_id=$1 AND deleted_at IS NULL`
	args := []interface{}{taskID}
	if tenantID != "" {
		query += ` AND tenant_id=$2`
		args = append(args, tenantID)
	}
	query += ` FOR UPDATE`
	return scanTaskRecord(tx.QueryRowContext(ctx, query, args...))
}

type taskRecordScanner interface {
	Scan(dest ...interface{}) error
}

func scanTaskRecord(row taskRecordScanner) (*Task, error) {
	var task Task
	var resultFileKey, resultSHA256, errorMessage, runID, createdBy sql.NullString
	var completedAt, deletedAt sql.NullTime
	err := row.Scan(&task.TaskID, &task.TenantID, &task.TaskType, &task.Status, &task.Progress,
		&task.ParamsJSON, &resultFileKey, &resultSHA256, &task.ResultPackets, &task.ResultBytes,
		&task.FilesScanned, &errorMessage, &runID, &createdBy, &task.CreatedAt, &task.UpdatedAt,
		&completedAt, &task.Revision, &deletedAt)
	if err == sql.ErrNoRows {
		return nil, commonerrors.New(commonerrors.ErrCodeResourceNotFound, "task not found")
	}
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to lock task")
	}
	if resultFileKey.Valid {
		task.ResultFileKey = resultFileKey.String
	}
	if resultSHA256.Valid {
		task.ResultSHA256 = resultSHA256.String
	}
	if errorMessage.Valid {
		task.ErrorMessage = errorMessage.String
	}
	if runID.Valid {
		task.RunID = runID.String
	}
	if createdBy.Valid {
		task.CreatedBy = createdBy.String
	}
	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	if deletedAt.Valid {
		task.DeletedAt = &deletedAt.Time
	}
	return &task, nil
}

func normalizeTaskCommandMeta(meta TaskCommandMeta, tenantID, taskID, actionID, reason string, payload interface{}) TaskCommandMeta {
	meta.TenantID = strings.TrimSpace(meta.TenantID)
	if meta.TenantID == "" {
		meta.TenantID = strings.TrimSpace(tenantID)
		meta.CompatibilityMode = true
	}
	meta.ActionID = strings.TrimSpace(meta.ActionID)
	if meta.ActionID == "" {
		meta.ActionID = actionID
		meta.CompatibilityMode = true
	}
	meta.Reason = strings.TrimSpace(meta.Reason)
	if meta.Reason == "" {
		meta.Reason = reason
		meta.CompatibilityMode = true
	}
	meta.TraceID = strings.TrimSpace(meta.TraceID)
	if meta.TraceID == "" {
		meta.TraceID = meta.ActionID + ":" + taskID
		meta.CompatibilityMode = true
	}
	meta.IdempotencyKey = strings.TrimSpace(meta.IdempotencyKey)
	if meta.IdempotencyKey == "" {
		digest, _ := taskCommandHash(meta.ActionID, taskID, payload)
		meta.IdempotencyKey = "compat-" + digest
		meta.CompatibilityMode = true
	}
	if meta.ExpectedRevision == nil {
		meta.CompatibilityMode = true
	}
	return meta
}

func validateTaskCommandMeta(meta TaskCommandMeta) error {
	if meta.TenantID == "" || meta.ActionID == "" || meta.Reason == "" || meta.TraceID == "" {
		return commonerrors.New(commonerrors.ErrCodeInvalidParameter, "tenant, action_id, reason and trace_id are required")
	}
	if len(meta.IdempotencyKey) < 16 || len(meta.IdempotencyKey) > 200 {
		return commonerrors.New(commonerrors.ErrCodeInvalidParameter, "Idempotency-Key must contain 16 to 200 characters")
	}
	return nil
}

func taskCommandHash(actionID, operation string, payload interface{}) (string, error) {
	encoded, err := json.Marshal(map[string]interface{}{"action_id": actionID, "operation": operation, "payload": payload})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func deterministicTaskEventID(tenantID, idempotencyKey string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("forensics-task-event\x00"+tenantID+"\x00"+idempotencyKey))
}

func actionForTaskMutation(m taskMutation) string {
	switch m.Operation {
	case "progress":
		return forensicsTaskProgressAction
	case "complete":
		return forensicsTaskCompleteAction
	case "fail":
		return forensicsTaskFailAction
	case "cancel":
		return ForensicsTaskCancelAction
	case "retry":
		return ForensicsTaskRetryAction
	case "recover":
		return forensicsTaskRecoverAction
	case "archive":
		return forensicsTaskArchiveAction
	default:
		return forensicsTaskLeaseAction
	}
}

func reasonForTaskMutation(m taskMutation) string {
	switch m.Operation {
	case "progress":
		return "worker progress checkpoint"
	case "complete":
		return "pcap cut result committed"
	case "fail":
		return "pcap cut execution failed"
	case "cancel":
		return "pcap cut cancelled"
	case "retry":
		return "failed or cancelled PCAP cut requeued"
	case "recover":
		return "stuck worker lease recovered"
	case "archive":
		return "retention period elapsed"
	default:
		return "worker acquired queued task"
	}
}

func taskSnapshot(task *Task) map[string]interface{} {
	paramsDigest := sha256.Sum256(task.ParamsJSON)
	return map[string]interface{}{
		"task_id": task.TaskID, "tenant_id": task.TenantID, "task_type": task.TaskType,
		"params": json.RawMessage(task.ParamsJSON), "params_sha256": hex.EncodeToString(paramsDigest[:]),
		"status": task.Status, "progress": task.Progress, "result_file_key": task.ResultFileKey,
		"result_sha256": task.ResultSHA256, "result_packets": task.ResultPackets,
		"result_bytes": task.ResultBytes, "files_scanned": task.FilesScanned,
		"error_message": task.ErrorMessage, "revision": task.Revision,
		"created_by": task.CreatedBy, "created_at": task.CreatedAt, "updated_at": task.UpdatedAt,
		"completed_at": task.CompletedAt, "deleted_at": task.DeletedAt,
	}
}

func taskInt(value int) *int       { return &value }
func taskInt64(value int64) *int64 { return &value }

func internalTaskMeta(task *Task, actionID string, expected int64, payload interface{}) TaskCommandMeta {
	digest, _ := taskCommandHash(actionID, task.TaskID, map[string]interface{}{"revision": expected, "payload": payload})
	return TaskCommandMeta{
		TenantID: task.TenantID, ActorID: "forensics-worker", ActionID: actionID,
		IdempotencyKey: "internal-" + digest, ExpectedRevision: &expected,
		Reason:            reasonForTaskMutation(taskMutation{Operation: operationForAction(actionID)}),
		TraceID:           "forensics-worker:" + task.TaskID + ":" + fmt.Sprint(expected+1),
		CompatibilityMode: true,
	}
}

func operationForAction(actionID string) string {
	switch actionID {
	case forensicsTaskProgressAction:
		return "progress"
	case forensicsTaskCompleteAction:
		return "complete"
	case forensicsTaskFailAction:
		return "fail"
	case ForensicsTaskCancelAction:
		return "cancel"
	case ForensicsTaskRetryAction:
		return "retry"
	case forensicsTaskRecoverAction:
		return "recover"
	case forensicsTaskArchiveAction:
		return "archive"
	default:
		return "status"
	}
}

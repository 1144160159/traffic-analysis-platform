package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

var (
	ErrTaskExecutionLeaseUnavailable = errors.New("forensics task execution lease is unavailable")
	ErrTaskExecutionFenceLost        = errors.New("forensics task execution fencing token is stale")
)

type TaskExecutionClaim struct {
	TenantID           string
	TaskID             string
	RequestSHA256      string
	WorkerID           string
	LeaseToken         uuid.UUID
	LeaseUntil         time.Time
	CheckpointRevision int64
	Phase              string
	Checkpoint         json.RawMessage
}

type VersionedTaskManifest struct {
	TenantID       string
	TaskID         string
	ManifestSHA256 string
	ManifestJSON   json.RawMessage
	Status         string
	ResultObject   s3client.ObjectAuthority
}

type TaskManifestReceipt struct {
	ManifestSHA256 string                   `json:"manifest_sha256"`
	Manifest       json.RawMessage          `json:"manifest"`
	Status         string                   `json:"status"`
	TaskID         string                   `json:"task_id"`
	ResultObject   s3client.ObjectAuthority `json:"result_object"`
	ObjectVersion  string                   `json:"result_object_version"`
	RetentionUntil time.Time                `json:"retention_until"`
}

func (r *TaskRepository) GetVersionedManifestForTenant(ctx context.Context, tenantID, taskID string) (*TaskManifestReceipt, error) {
	return scanTaskManifestReceipt(r.client.QueryRow(ctx, `SELECT manifest_sha256,manifest::text,status,task_id,
		result_bucket,result_object_key,result_object_version,result_etag,result_size_bytes,result_sha256,
		retention_until,created_at
		FROM forensics_job_manifests WHERE tenant_id=$1 AND task_id=$2`, tenantID, taskID))
}

// GetVersionedManifestByResultKey resolves download authority from the final
// PostgreSQL manifest. It never accepts a key without the authenticated tenant.
func (r *TaskRepository) GetVersionedManifestByResultKey(ctx context.Context, tenantID, resultKey string) (*TaskManifestReceipt, error) {
	return scanTaskManifestReceipt(r.client.QueryRow(ctx, `SELECT manifest_sha256,manifest::text,status,task_id,
		result_bucket,result_object_key,result_object_version,result_etag,result_size_bytes,result_sha256,
		retention_until,created_at
		FROM forensics_job_manifests WHERE tenant_id=$1 AND result_object_key=$2`, tenantID, resultKey))
}

func scanTaskManifestReceipt(row *sql.Row) (*TaskManifestReceipt, error) {
	var receipt TaskManifestReceipt
	var manifestText string
	err := row.Scan(&receipt.ManifestSHA256, &manifestText, &receipt.Status, &receipt.TaskID,
		&receipt.ResultObject.Bucket, &receipt.ResultObject.Key, &receipt.ResultObject.VersionID,
		&receipt.ResultObject.ETag, &receipt.ResultObject.SizeBytes, &receipt.ResultObject.SHA256,
		&receipt.RetentionUntil, &receipt.ResultObject.ObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load versioned task manifest: %w", err)
	}
	if !json.Valid([]byte(manifestText)) {
		return nil, errors.New("stored versioned task manifest is not valid JSON")
	}
	receipt.Manifest = json.RawMessage(manifestText)
	receipt.ObjectVersion = receipt.ResultObject.VersionID
	receipt.ResultObject.RetentionUntil = receipt.RetentionUntil
	if err := receipt.ResultObject.Validate(); err != nil {
		return nil, fmt.Errorf("stored versioned task result authority is invalid: %w", err)
	}
	return &receipt, nil
}

func (r *TaskRepository) VerifyVersionedExecutionSchema(ctx context.Context) error {
	var migrationCount, checkpointColumns, manifestColumns int
	err := r.client.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM alignment_schema_migrations WHERE version='202608151930'),
		(SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema()
		 AND table_name='forensics_task_checkpoints' AND column_name IN
		 ('tenant_id','task_id','request_sha256','worker_id','lease_token','lease_until','checkpoint_revision','phase','checkpoint','completed_at')),
		(SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema()
		 AND table_name='forensics_job_manifests' AND column_name IN
		 ('tenant_id','task_id','manifest_version','manifest_sha256','manifest','status','result_bucket','result_object_key','result_object_version','result_sha256','retention_until','executable','automatic_open'))`).
		Scan(&migrationCount, &checkpointColumns, &manifestColumns)
	if err != nil {
		return fmt.Errorf("verify versioned task execution schema: %w", err)
	}
	if migrationCount != 1 || checkpointColumns != 10 || manifestColumns != 13 {
		return errors.New("versioned task execution schema is not exact")
	}
	return nil
}

func taskParamsSHA256(task *Task) string {
	digest := sha256.Sum256(task.ParamsJSON)
	return hex.EncodeToString(digest[:])
}

// ClaimVersionedExecution fences one processing task. An expired lease can be
// resumed only when the immutable params hash is unchanged.
func (r *TaskRepository) ClaimVersionedExecution(
	ctx context.Context,
	task *Task,
	workerID string,
	leaseDuration time.Duration,
) (*TaskExecutionClaim, error) {
	if task == nil || task.Status != TaskStatusProcessing || strings.TrimSpace(task.TenantID) == "" ||
		strings.TrimSpace(task.TaskID) == "" || strings.TrimSpace(workerID) == "" || leaseDuration <= 0 {
		return nil, errors.New("processing task, worker identity and positive lease duration are required")
	}
	now := time.Now().UTC()
	claim := TaskExecutionClaim{
		TenantID: task.TenantID, TaskID: task.TaskID, RequestSHA256: taskParamsSHA256(task),
		WorkerID: workerID, LeaseToken: uuid.New(), LeaseUntil: now.Add(leaseDuration),
	}
	var checkpointText string
	err := r.client.QueryRow(ctx, `INSERT INTO forensics_task_checkpoints
		(tenant_id,task_id,request_sha256,worker_id,lease_token,lease_until,checkpoint_revision,phase,checkpoint,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,1,'leased','{}'::jsonb,$7,$7)
		ON CONFLICT (tenant_id,task_id) DO UPDATE SET
			worker_id=EXCLUDED.worker_id,lease_token=EXCLUDED.lease_token,lease_until=EXCLUDED.lease_until,
			checkpoint_revision=forensics_task_checkpoints.checkpoint_revision+1,updated_at=EXCLUDED.updated_at
		WHERE forensics_task_checkpoints.request_sha256=EXCLUDED.request_sha256
		  AND forensics_task_checkpoints.phase NOT IN ('completed','failed','cancelled')
		  AND (forensics_task_checkpoints.lease_until <= $7 OR forensics_task_checkpoints.worker_id=EXCLUDED.worker_id)
		RETURNING checkpoint_revision,phase,checkpoint::text`,
		claim.TenantID, claim.TaskID, claim.RequestSHA256, claim.WorkerID, claim.LeaseToken, claim.LeaseUntil, now).
		Scan(&claim.CheckpointRevision, &claim.Phase, &checkpointText)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTaskExecutionLeaseUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("claim versioned task execution: %w", err)
	}
	if !json.Valid([]byte(checkpointText)) {
		return nil, errors.New("stored versioned task checkpoint is not valid JSON")
	}
	claim.Checkpoint = json.RawMessage(checkpointText)
	return &claim, nil
}

func (r *TaskRepository) AdvanceVersionedExecution(
	ctx context.Context,
	claim *TaskExecutionClaim,
	phase string,
	checkpoint any,
	leaseDuration time.Duration,
) error {
	if claim == nil || claim.LeaseToken == uuid.Nil || leaseDuration <= 0 {
		return errors.New("execution claim and positive lease duration are required")
	}
	checkpointJSON, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("marshal versioned task checkpoint: %w", err)
	}
	result, err := r.client.Exec(ctx, `UPDATE forensics_task_checkpoints SET
		phase=$5,checkpoint=$6::jsonb,checkpoint_revision=checkpoint_revision+1,
		lease_until=$7,updated_at=$8
		WHERE tenant_id=$1 AND task_id=$2 AND lease_token=$3 AND checkpoint_revision=$4
		  AND lease_until>$8 AND phase NOT IN ('completed','failed','cancelled')`,
		claim.TenantID, claim.TaskID, claim.LeaseToken, claim.CheckpointRevision,
		phase, string(checkpointJSON), time.Now().UTC().Add(leaseDuration), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("advance versioned task checkpoint: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return ErrTaskExecutionFenceLost
	}
	claim.CheckpointRevision++
	claim.Phase = phase
	claim.Checkpoint = append(claim.Checkpoint[:0], checkpointJSON...)
	claim.LeaseUntil = time.Now().UTC().Add(leaseDuration)
	return nil
}

func validateVersionedTaskManifest(manifest VersionedTaskManifest) error {
	if strings.TrimSpace(manifest.TenantID) == "" || strings.TrimSpace(manifest.TaskID) == "" ||
		len(manifest.ManifestJSON) == 0 || !json.Valid(manifest.ManifestJSON) ||
		len(manifest.ManifestSHA256) != 64 || manifest.ResultObject.Validate() != nil {
		return errors.New("versioned task manifest authority is incomplete")
	}
	digest := sha256.Sum256(manifest.ManifestJSON)
	if hex.EncodeToString(digest[:]) != manifest.ManifestSHA256 {
		return errors.New("versioned task manifest SHA-256 differs from payload")
	}
	if manifest.Status != TaskStatusCompleted && manifest.Status != TaskStatusPartial {
		return errors.New("versioned task manifest status must be completed or partial")
	}
	return nil
}

// CompleteVersionedExecution commits the final manifest and the task's
// history/outbox/audit completion in one PostgreSQL transaction.
func (r *TaskRepository) CompleteVersionedExecution(
	ctx context.Context,
	claim *TaskExecutionClaim,
	manifest VersionedTaskManifest,
	packets, byteCount int64,
	filesScanned int,
) error {
	if claim == nil || claim.LeaseToken == uuid.Nil {
		return errors.New("versioned task execution claim is required")
	}
	if err := validateVersionedTaskManifest(manifest); err != nil {
		return err
	}
	if manifest.TenantID != claim.TenantID || manifest.TaskID != claim.TaskID {
		return errors.New("versioned task manifest tenant/task identity mismatch")
	}
	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin versioned task completion: %w", err)
	}
	defer tx.Rollback()
	task, err := lockTaskForCommand(ctx, tx, claim.TenantID, claim.TaskID)
	if err != nil {
		return err
	}
	if task.Status == TaskStatusCompleted || task.Status == TaskStatusPartial {
		var storedSHA string
		if err := tx.QueryRowContext(ctx, `SELECT manifest_sha256 FROM forensics_job_manifests WHERE tenant_id=$1 AND task_id=$2`, claim.TenantID, claim.TaskID).Scan(&storedSHA); err != nil {
			return fmt.Errorf("load replayed versioned task manifest: %w", err)
		}
		if storedSHA != manifest.ManifestSHA256 {
			return errors.New("completed task has a different immutable manifest")
		}
		return tx.Commit()
	}
	var requestSHA string
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM forensics_task_checkpoints
		WHERE tenant_id=$1 AND task_id=$2 AND lease_token=$3 AND checkpoint_revision=$4
		  AND lease_until>now() AND phase NOT IN ('completed','failed','cancelled') FOR UPDATE`,
		claim.TenantID, claim.TaskID, claim.LeaseToken, claim.CheckpointRevision).Scan(&requestSHA)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTaskExecutionFenceLost
	}
	if err != nil {
		return fmt.Errorf("lock versioned task checkpoint: %w", err)
	}
	if requestSHA != taskParamsSHA256(task) || requestSHA != claim.RequestSHA256 {
		return errors.New("versioned task request changed after admission")
	}
	object := manifest.ResultObject
	_, err = tx.ExecContext(ctx, `INSERT INTO forensics_job_manifests
		(tenant_id,task_id,manifest_version,manifest_sha256,manifest,status,
		 result_bucket,result_object_key,result_object_version,result_etag,result_size_bytes,result_sha256,
		 retention_until,executable,automatic_open,created_at)
		VALUES ($1,$2,1,$3,$4::jsonb,$5,$6,$7,$8,$9,$10,$11,$12,false,false,now())`,
		manifest.TenantID, manifest.TaskID, manifest.ManifestSHA256, string(manifest.ManifestJSON), manifest.Status,
		object.Bucket, object.Key, object.VersionID, object.ETag, object.SizeBytes, object.SHA256, object.RetentionUntil)
	if err != nil {
		return fmt.Errorf("insert immutable forensics job manifest: %w", err)
	}
	mutation := taskMutation{
		Operation: "complete", Status: manifest.Status, ResultFileKey: object.Key, ResultSHA256: object.SHA256,
		Packets: taskInt64(packets), Bytes: taskInt64(byteCount), FilesScanned: taskInt(filesScanned), Completed: true,
	}
	meta := internalTaskMeta(task, forensicsTaskCompleteAction, task.Revision, map[string]any{
		"manifest_sha256": manifest.ManifestSHA256, "result_object_version": object.VersionID,
	})
	requestHash, err := taskCommandHash(meta.ActionID, mutation.Operation, map[string]any{
		"task_id": task.TaskID, "expected_revision": task.Revision, "mutation": mutation,
		"manifest_sha256": manifest.ManifestSHA256,
	})
	if err != nil {
		return fmt.Errorf("hash versioned task completion: %w", err)
	}
	if _, err = applyLockedTaskMutation(ctx, tx, task, meta, requestHash, mutation, time.Now().UTC()); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE forensics_task_checkpoints SET
		phase='completed',checkpoint=jsonb_build_object('manifest_sha256',$5::text),
		checkpoint_revision=checkpoint_revision+1,updated_at=now(),completed_at=now()
		WHERE tenant_id=$1 AND task_id=$2 AND lease_token=$3 AND checkpoint_revision=$4`,
		claim.TenantID, claim.TaskID, claim.LeaseToken, claim.CheckpointRevision, manifest.ManifestSHA256)
	if err != nil {
		return fmt.Errorf("finalize versioned task checkpoint: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return ErrTaskExecutionFenceLost
	}
	return tx.Commit()
}

func (r *TaskRepository) FailVersionedExecution(ctx context.Context, claim *TaskExecutionClaim, message string) error {
	if claim == nil || claim.LeaseToken == uuid.Nil || strings.TrimSpace(message) == "" {
		return errors.New("execution claim and failure message are required")
	}
	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin versioned task failure: %w", err)
	}
	defer tx.Rollback()
	task, err := lockTaskForCommand(ctx, tx, claim.TenantID, claim.TaskID)
	if err != nil {
		return err
	}
	phase := "failed"
	if task.Status == TaskStatusCancelled {
		phase = "cancelled"
	} else {
		mutation := taskMutation{Operation: "fail", Status: TaskStatusFailed, ErrorMessage: message, Completed: true}
		meta := internalTaskMeta(task, forensicsTaskFailAction, task.Revision, mutation)
		requestHash, hashErr := taskCommandHash(meta.ActionID, mutation.Operation, map[string]any{
			"task_id": task.TaskID, "expected_revision": task.Revision, "mutation": mutation,
		})
		if hashErr != nil {
			return hashErr
		}
		if _, err = applyLockedTaskMutation(ctx, tx, task, meta, requestHash, mutation, time.Now().UTC()); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE forensics_task_checkpoints SET phase=$5,
		checkpoint=jsonb_build_object('error',$6::text),checkpoint_revision=checkpoint_revision+1,
		updated_at=now(),completed_at=now()
		WHERE tenant_id=$1 AND task_id=$2 AND lease_token=$3 AND checkpoint_revision=$4
		  AND phase NOT IN ('completed','failed','cancelled')`,
		claim.TenantID, claim.TaskID, claim.LeaseToken, claim.CheckpointRevision, phase, message)
	if err != nil {
		return fmt.Errorf("finalize failed versioned task checkpoint: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return ErrTaskExecutionFenceLost
	}
	return tx.Commit()
}

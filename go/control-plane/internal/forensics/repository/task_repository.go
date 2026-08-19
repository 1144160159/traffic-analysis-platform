////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/forensics/repository/task_repository.go
// 修复版：
// - ✅ 修复 P11: GetPendingTasks 返回值错误
// - ✅ 优化并发安全和事务处理
////////////////////////////////////////////////////////////////////////////////

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// 任务状态常量
const (
	TaskStatusQueued     = "queued"
	TaskStatusProcessing = "processing"
	TaskStatusCompleted  = "completed"
	TaskStatusPartial    = "partial"
	TaskStatusFailed     = "failed"
	TaskStatusCancelled  = "cancelled"
)

// 任务类型常量
const (
	TaskTypePcapCut = "pcap_cut"
)

// Task 任务实体
type Task struct {
	TaskID        string     `db:"task_id"`
	TenantID      string     `db:"tenant_id"`
	TaskType      string     `db:"task_type"`
	Status        string     `db:"status"`
	Progress      int        `db:"progress"`
	ParamsJSON    []byte     `db:"params"`
	ResultFileKey string     `db:"result_file_key"`
	ResultSHA256  string     `db:"result_sha256"`
	ResultPackets int64      `db:"result_packets"`
	ResultBytes   int64      `db:"result_bytes"`
	FilesScanned  int        `db:"files_scanned"`
	ErrorMessage  string     `db:"error_message"`
	RunID         string     `db:"run_id"`
	CreatedBy     string     `db:"created_by"`
	CreatedAt     time.Time  `db:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"`
	CompletedAt   *time.Time `db:"completed_at"`
	Revision      int64      `db:"revision"`
	DeletedAt     *time.Time `db:"deleted_at"`
}

// TaskRepository 任务仓库
type TaskRepository struct {
	client *storage.PostgresClient
	logger *zap.Logger
}

// NewTaskRepository 创建任务仓库
func NewTaskRepository(client *storage.PostgresClient, logger *zap.Logger) *TaskRepository {
	return &TaskRepository{
		client: client,
		logger: logger,
	}
}

// Create 创建任务
func (r *TaskRepository) Create(ctx context.Context, task *Task) error {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.Create")
	defer span.End()
	_, err := r.CreateAtomic(ctx, task, TaskCommandMeta{CompatibilityMode: true})
	return err
}

// GetByID 根据 ID 获取任务
func (r *TaskRepository) GetByID(ctx context.Context, taskID string) (*Task, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.GetByID")
	defer span.End()

	query := `
			SELECT
				task_id, tenant_id, task_type, status, progress, params,
				result_file_key, result_sha256, result_packets, result_bytes, files_scanned,
				error_message, run_id, created_by, created_at, updated_at, completed_at, revision, deleted_at
			FROM tasks
		WHERE task_id = $1 AND deleted_at IS NULL
	`

	row := r.client.QueryRow(ctx, query, taskID)
	return r.scanTask(ctx, row)
}

// GetByIDForTenant resolves a live task without exposing whether another
// tenant owns the same identifier.
func (r *TaskRepository) GetByIDForTenant(ctx context.Context, tenantID, taskID string) (*Task, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.GetByIDForTenant")
	defer span.End()
	row := r.client.QueryRow(ctx, `SELECT
		task_id,tenant_id,task_type,status,progress,params,result_file_key,result_sha256,
		result_packets,result_bytes,files_scanned,error_message,run_id,created_by,
		created_at,updated_at,completed_at,revision,deleted_at
		FROM tasks WHERE tenant_id=$1 AND task_id=$2 AND deleted_at IS NULL`, tenantID, taskID)
	return r.scanTask(ctx, row)
}

// GetByResultFileKey 根据结果文件 key 获取任务，用于下载和完整性校验。
func (r *TaskRepository) GetByResultFileKey(ctx context.Context, resultFileKey string) (*Task, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.GetByResultFileKey")
	defer span.End()

	query := `
			SELECT
				task_id, tenant_id, task_type, status, progress, params,
				result_file_key, result_sha256, result_packets, result_bytes, files_scanned,
				error_message, run_id, created_by, created_at, updated_at, completed_at
			FROM tasks
		WHERE task_id = $1
	`

	row := r.client.QueryRow(ctx, query, taskID)
	return r.scanTask(ctx, row)
}

// GetByResultFileKey 根据结果文件 key 获取任务，用于下载和完整性校验。
func (r *TaskRepository) GetByResultFileKey(ctx context.Context, resultFileKey string) (*Task, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.GetByResultFileKey")
	defer span.End()

	query := `
		SELECT
			task_id, tenant_id, task_type, status, progress, params,
			result_file_key, result_sha256, result_packets, result_bytes, files_scanned,
			error_message, run_id, created_by, created_at, updated_at, completed_at, revision, deleted_at
		FROM tasks
		WHERE result_file_key = $1 AND deleted_at IS NULL
		ORDER BY completed_at DESC NULLS LAST, updated_at DESC
		LIMIT 1
	`

	row := r.client.QueryRow(ctx, query, resultFileKey)
	return r.scanTask(ctx, row)
}

// scanTask 扫描单个任务
func (r *TaskRepository) scanTask(ctx context.Context, row *sql.Row) (*Task, error) {
	var task Task
	var resultFileKey, resultSHA256, errorMessage, runID, createdBy sql.NullString
	var completedAt, deletedAt sql.NullTime

	err := row.Scan(
		&task.TaskID,
		&task.TenantID,
		&task.TaskType,
		&task.Status,
		&task.Progress,
		&task.ParamsJSON,
		&resultFileKey,
		&resultSHA256,
		&task.ResultPackets,
		&task.ResultBytes,
		&task.FilesScanned,
		&errorMessage,
		&runID,
		&createdBy,
		&task.CreatedAt,
		&task.UpdatedAt,
		&completedAt,
		&task.Revision,
		&deletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Newf(errors.ErrCodeResourceNotFound, "task not found")
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get task")
	}

	// 处理可空字段
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

// UpdateStatus 更新任务状态
func (r *TaskRepository) UpdateStatus(ctx context.Context, taskID, status string) error {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.UpdateStatus")
	defer span.End()
	_, err := r.mutateTaskAtomic(ctx, taskID, TaskCommandMeta{CompatibilityMode: true}, taskMutation{Operation: "status", Status: status})
	return err
}

// UpdateProgress 更新任务进度
func (r *TaskRepository) UpdateProgress(ctx context.Context, taskID string, progress int, packetsFound int64) error {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.UpdateProgress")
	defer span.End()
	_, err := r.mutateTaskAtomic(ctx, taskID, TaskCommandMeta{CompatibilityMode: true}, taskMutation{
		Operation: "progress", Progress: taskInt(progress), Packets: taskInt64(packetsFound),
	})
	return err
}

// Complete 标记任务完成
func (r *TaskRepository) Complete(ctx context.Context, taskID, resultFileKey, resultSHA256 string, packets, bytes int64, filesScanned int) error {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.Complete")
	defer span.End()
	_, err := r.mutateTaskAtomic(ctx, taskID, TaskCommandMeta{CompatibilityMode: true}, taskMutation{
		Operation: "complete", Status: TaskStatusCompleted, ResultFileKey: resultFileKey,
		ResultSHA256: resultSHA256, Packets: taskInt64(packets), Bytes: taskInt64(bytes),
		FilesScanned: taskInt(filesScanned), Completed: true,
	})
	return err
}

// Fail 标记任务失败
func (r *TaskRepository) Fail(ctx context.Context, taskID, errorMessage string) error {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.Fail")
	defer span.End()
	_, err := r.mutateTaskAtomic(ctx, taskID, TaskCommandMeta{CompatibilityMode: true}, taskMutation{
		Operation: "fail", Status: TaskStatusFailed, ErrorMessage: errorMessage, Completed: true,
	})
	return err
}

// Cancel 取消任务
func (r *TaskRepository) Cancel(ctx context.Context, taskID string) error {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.Cancel")
	defer span.End()
	_, err := r.mutateTaskAtomic(ctx, taskID, TaskCommandMeta{CompatibilityMode: true}, taskMutation{
		Operation: "cancel", Status: TaskStatusCancelled, Completed: true,
	})
	return err
}

type TaskListFilter struct {
	Status       string
	AssetID      string
	AlertID      string
	CampaignID   string
	BaselineID   string
	EvidenceID   string
	EvidenceType string
	SrcIP        string
	DstIP        string
	Protocol     string
	Port         string
	Tuple        string
	TaskID       string
}

// List preserves the worker-facing compact API.
func (r *TaskRepository) List(ctx context.Context, tenantID, status, assetID string, limit, offset int) ([]*Task, int64, error) {
	return r.ListFiltered(ctx, tenantID, TaskListFilter{Status: status, AssetID: assetID}, limit, offset)
}

// ListFiltered lists tenant tasks with the filters exposed by the forensics workbench.
func (r *TaskRepository) ListFiltered(ctx context.Context, tenantID string, filter TaskListFilter, limit, offset int) ([]*Task, int64, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.List")
	defer span.End()

	// 计数查询
	countQuery := `SELECT COUNT(*) FROM tasks WHERE tenant_id = $1 AND deleted_at IS NULL`
	countArgs := []interface{}{tenantID}

	countArgIndex := 2
	if filter.Status != "" {
		countQuery += fmt.Sprintf(` AND status = $%d`, countArgIndex)
		countArgs = append(countArgs, filter.Status)
		countArgIndex++
	}
	if filter.AssetID != "" {
		countQuery += fmt.Sprintf(` AND params->>'asset_id' = $%d`, countArgIndex)
		countArgs = append(countArgs, filter.AssetID)
		countArgIndex++
	}
	countQuery, countArgs, _ = appendTaskFilters(countQuery, countArgs, countArgIndex, filter)

	var total int64
	row := r.client.QueryRow(ctx, countQuery, countArgs...)
	if err := row.Scan(&total); err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count tasks")
	}

	// 列表查询
	listQuery := `
			SELECT
				task_id, tenant_id, task_type, status, progress, params,
				result_file_key, result_sha256, result_packets, result_bytes, files_scanned,
				error_message, run_id, created_by, created_at, updated_at, completed_at, revision, deleted_at
		FROM tasks
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`
	listArgs := []interface{}{tenantID}
	argIndex := 2

	if filter.Status != "" {
		listQuery += fmt.Sprintf(` AND status = $%d`, argIndex)
		listArgs = append(listArgs, filter.Status)
		argIndex++
	}
	if filter.AssetID != "" {
		listQuery += fmt.Sprintf(` AND params->>'asset_id' = $%d`, argIndex)
		listArgs = append(listArgs, filter.AssetID)
		argIndex++
	}
	listQuery, listArgs, argIndex = appendTaskFilters(listQuery, listArgs, argIndex, filter)

	listQuery += fmt.Sprintf(` ORDER BY created_at DESC, task_id DESC LIMIT $%d OFFSET $%d`, argIndex, argIndex+1)
	listArgs = append(listArgs, limit, offset)

	rows, err := r.client.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to list tasks")
	}
	defer rows.Close()

	tasks, _, err := r.scanTasks(rows)
	if err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func appendTaskFilters(query string, args []interface{}, argIndex int, filter TaskListFilter) (string, []interface{}, int) {
	filters := []struct {
		value          string
		sql            string
		repeatedArgRef bool
	}{
		{filter.SrcIP, `params->>'src_ip' = $%d`, false},
		{filter.AlertID, `params->>'alert_id' = $%d`, false},
		{filter.CampaignID, `params->>'campaign_id' = $%d`, false},
		{filter.BaselineID, `params->>'baseline_id' = $%d`, false},
		{filter.EvidenceID, `params->>'evidence_id' = $%d`, false},
		{filter.EvidenceType, `lower(params->>'evidence_type') = lower($%d)`, false},
		{filter.DstIP, `params->>'dst_ip' = $%d`, false},
		{filter.Protocol, `lower(params->>'protocol') = lower($%d)`, false},
		{filter.Port, `(params->>'src_port' = $%d OR params->>'dst_port' = $%d)`, true},
		{filter.TaskID, `task_id::text ILIKE '%%' || $%d || '%%'`, false},
		{filter.Tuple, `concat_ws(' ', params->>'src_ip', params->>'src_port', params->>'dst_ip', params->>'dst_port', params->>'protocol') ILIKE '%%' || $%d || '%%'`, false},
	}
	for _, item := range filters {
		if item.value == "" {
			continue
		}
		if item.repeatedArgRef {
			query += fmt.Sprintf(` AND `+item.sql, argIndex, argIndex)
		} else {
			query += fmt.Sprintf(` AND `+item.sql, argIndex)
		}
		args = append(args, item.value)
		argIndex++
	}
	return query, args, argIndex
}

// scanTasks 扫描多个任务
func (r *TaskRepository) scanTasks(rows *sql.Rows) ([]*Task, int64, error) {
	var tasks []*Task
	for rows.Next() {
		var task Task
		var resultFileKey, resultSHA256, errorMessage, runID, createdBy sql.NullString
		var completedAt, deletedAt sql.NullTime

		err := rows.Scan(
			&task.TaskID,
			&task.TenantID,
			&task.TaskType,
			&task.Status,
			&task.Progress,
			&task.ParamsJSON,
			&resultFileKey,
			&resultSHA256,
			&task.ResultPackets,
			&task.ResultBytes,
			&task.FilesScanned,
			&errorMessage,
			&runID,
			&createdBy,
			&task.CreatedAt,
			&task.UpdatedAt,
			&completedAt,
			&task.Revision,
			&deletedAt,
		)
		if err != nil {
			return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan task")
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

		tasks = append(tasks, &task)
	}

	return tasks, int64(len(tasks)), rows.Err()
}

// GetPendingTasks 获取待处理任务（使用行锁防止重复获取）
// ✅ 修复 P11: 修复返回值错误
func (r *TaskRepository) GetPendingTasks(ctx context.Context, limit int) ([]*Task, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.GetPendingTasks")
	defer span.End()

	// ✅ 直接调用带锁的方法（SQL 中已使用 FOR UPDATE SKIP LOCKED）
	return r.getPendingTasksWithLock(ctx, limit)
}

// getPendingTasksWithLock 使用行锁获取待处理任务
func (r *TaskRepository) getPendingTasksWithLock(ctx context.Context, limit int) ([]*Task, error) {
	return r.leasePendingTasksAtomic(ctx, limit)
}

// CleanupOldTasks 清理旧任务
func (r *TaskRepository) CleanupOldTasks(ctx context.Context, olderThan time.Duration) (int64, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.CleanupOldTasks")
	defer span.End()

	return r.archiveOldTasksAtomic(ctx, time.Now().UTC().Add(-olderThan))
}

// GetTaskStats 获取任务统计
func (r *TaskRepository) GetTaskStats(ctx context.Context, tenantID string) (map[string]int64, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.GetTaskStats")
	defer span.End()

	query := `
		SELECT status, COUNT(*) as count
		FROM tasks
		WHERE tenant_id = $1 AND deleted_at IS NULL
		GROUP BY status
	`

	rows, err := r.client.Query(ctx, query, tenantID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get task stats")
	}
	defer rows.Close()

	stats := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[status] = count
	}

	return stats, rows.Err()
}

// ResetStuckTasks 重置卡住的任务（长时间处于 processing 状态）
func (r *TaskRepository) ResetStuckTasks(ctx context.Context, stuckDuration time.Duration) (int64, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.ResetStuckTasks")
	defer span.End()

	return r.recoverStuckTasksAtomic(ctx, time.Now().UTC().Add(-stuckDuration))
}

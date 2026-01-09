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

	"github.com/google/uuid"
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
	ResultPackets int64      `db:"result_packets"`
	ResultBytes   int64      `db:"result_bytes"`
	FilesScanned  int        `db:"files_scanned"`
	ErrorMessage  string     `db:"error_message"`
	RunID         string     `db:"run_id"`
	CreatedBy     string     `db:"created_by"`
	CreatedAt     time.Time  `db:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"`
	CompletedAt   *time.Time `db:"completed_at"`
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

	if task.TaskID == "" {
		task.TaskID = uuid.New().String()
	}
	if task.Status == "" {
		task.Status = TaskStatusQueued
	}
	task.CreatedAt = time.Now()
	task.UpdatedAt = time.Now()

	query := `
		INSERT INTO tasks (
			task_id, tenant_id, task_type, status, progress, params,
			result_file_key, result_packets, result_bytes, files_scanned,
			error_message, run_id, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	_, err := r.client.Exec(ctx, query,
		task.TaskID,
		task.TenantID,
		task.TaskType,
		task.Status,
		task.Progress,
		task.ParamsJSON,
		task.ResultFileKey,
		task.ResultPackets,
		task.ResultBytes,
		task.FilesScanned,
		task.ErrorMessage,
		task.RunID,
		task.CreatedBy,
		task.CreatedAt,
		task.UpdatedAt,
	)

	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to create task")
	}

	r.logger.Info("Task created",
		zap.String("task_id", task.TaskID),
		zap.String("tenant_id", task.TenantID),
		zap.String("task_type", task.TaskType))

	return nil
}

// GetByID 根据 ID 获取任务
func (r *TaskRepository) GetByID(ctx context.Context, taskID string) (*Task, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.GetByID")
	defer span.End()

	query := `
		SELECT
			task_id, tenant_id, task_type, status, progress, params,
			result_file_key, result_packets, result_bytes, files_scanned,
			error_message, run_id, created_by, created_at, updated_at, completed_at
		FROM tasks
		WHERE task_id = $1
	`

	return r.scanTask(ctx, r.client.QueryRow(ctx, query, taskID))
}

// scanTask 扫描单个任务
func (r *TaskRepository) scanTask(ctx context.Context, row *sql.Row) (*Task, error) {
	var task Task
	var resultFileKey, errorMessage, runID, createdBy sql.NullString
	var completedAt sql.NullTime

	err := row.Scan(
		&task.TaskID,
		&task.TenantID,
		&task.TaskType,
		&task.Status,
		&task.Progress,
		&task.ParamsJSON,
		&resultFileKey,
		&task.ResultPackets,
		&task.ResultBytes,
		&task.FilesScanned,
		&errorMessage,
		&runID,
		&createdBy,
		&task.CreatedAt,
		&task.UpdatedAt,
		&completedAt,
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

	return &task, nil
}

// UpdateStatus 更新任务状态
func (r *TaskRepository) UpdateStatus(ctx context.Context, taskID, status string) error {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.UpdateStatus")
	defer span.End()

	query := `
		UPDATE tasks
		SET status = $1, updated_at = $2
		WHERE task_id = $3
	`

	result, err := r.client.Exec(ctx, query, status, time.Now(), taskID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update task status")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.Newf(errors.ErrCodeResourceNotFound, "task not found: %s", taskID)
	}

	return nil
}

// UpdateProgress 更新任务进度
func (r *TaskRepository) UpdateProgress(ctx context.Context, taskID string, progress int, packetsFound int64) error {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.UpdateProgress")
	defer span.End()

	query := `
		UPDATE tasks
		SET progress = $1, result_packets = $2, updated_at = $3
		WHERE task_id = $4
	`

	_, err := r.client.Exec(ctx, query, progress, packetsFound, time.Now(), taskID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update task progress")
	}

	return nil
}

// Complete 标记任务完成
func (r *TaskRepository) Complete(ctx context.Context, taskID, resultFileKey string, packets, bytes int64, filesScanned int) error {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.Complete")
	defer span.End()

	now := time.Now()

	query := `
		UPDATE tasks
		SET status = $1, progress = 100, result_file_key = $2,
			result_packets = $3, result_bytes = $4, files_scanned = $5,
			updated_at = $6, completed_at = $7
		WHERE task_id = $8
	`

	result, err := r.client.Exec(ctx, query,
		TaskStatusCompleted,
		resultFileKey,
		packets,
		bytes,
		filesScanned,
		now,
		now,
		taskID,
	)

	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to complete task")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.Newf(errors.ErrCodeResourceNotFound, "task not found: %s", taskID)
	}

	r.logger.Info("Task completed",
		zap.String("task_id", taskID),
		zap.Int64("packets", packets),
		zap.Int64("bytes", bytes))

	return nil
}

// Fail 标记任务失败
func (r *TaskRepository) Fail(ctx context.Context, taskID, errorMessage string) error {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.Fail")
	defer span.End()

	now := time.Now()

	query := `
		UPDATE tasks
		SET status = $1, error_message = $2, updated_at = $3, completed_at = $4
		WHERE task_id = $5
	`

	result, err := r.client.Exec(ctx, query,
		TaskStatusFailed,
		errorMessage,
		now,
		now,
		taskID,
	)

	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to fail task")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.Newf(errors.ErrCodeResourceNotFound, "task not found: %s", taskID)
	}

	r.logger.Warn("Task failed",
		zap.String("task_id", taskID),
		zap.String("error", errorMessage))

	return nil
}

// Cancel 取消任务
func (r *TaskRepository) Cancel(ctx context.Context, taskID string) error {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.Cancel")
	defer span.End()

	now := time.Now()

	query := `
		UPDATE tasks
		SET status = $1, updated_at = $2, completed_at = $3
		WHERE task_id = $4 AND status IN ($5, $6)
	`

	result, err := r.client.Exec(ctx, query,
		TaskStatusCancelled,
		now,
		now,
		taskID,
		TaskStatusQueued,
		TaskStatusProcessing,
	)

	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to cancel task")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// 可能任务已完成或不存在
		task, err := r.GetByID(ctx, taskID)
		if err != nil {
			return err
		}
		if task.Status != TaskStatusQueued && task.Status != TaskStatusProcessing {
			return errors.Newf(errors.ErrCodeInvalidStateTransition, "cannot cancel task in status: %s", task.Status)
		}
		return errors.Newf(errors.ErrCodeResourceNotFound, "task not found: %s", taskID)
	}

	r.logger.Info("Task cancelled", zap.String("task_id", taskID))

	return nil
}

// List 列出任务
func (r *TaskRepository) List(ctx context.Context, tenantID, status string, limit, offset int) ([]*Task, int64, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.List")
	defer span.End()

	// 计数查询
	countQuery := `SELECT COUNT(*) FROM tasks WHERE tenant_id = $1`
	countArgs := []interface{}{tenantID}

	if status != "" {
		countQuery += ` AND status = $2`
		countArgs = append(countArgs, status)
	}

	var total int64
	row := r.client.QueryRow(ctx, countQuery, countArgs...)
	if err := row.Scan(&total); err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count tasks")
	}

	// 列表查询
	listQuery := `
		SELECT
			task_id, tenant_id, task_type, status, progress, params,
			result_file_key, result_packets, result_bytes, files_scanned,
			error_message, run_id, created_by, created_at, updated_at, completed_at
		FROM tasks
		WHERE tenant_id = $1
	`
	listArgs := []interface{}{tenantID}
	argIndex := 2

	if status != "" {
		listQuery += fmt.Sprintf(` AND status = $%d`, argIndex)
		listArgs = append(listArgs, status)
		argIndex++
	}

	listQuery += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIndex, argIndex+1)
	listArgs = append(listArgs, limit, offset)

	rows, err := r.client.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to list tasks")
	}
	defer rows.Close()

	return r.scanTasks(rows)
}

// scanTasks 扫描多个任务
func (r *TaskRepository) scanTasks(rows *sql.Rows) ([]*Task, int64, error) {
	var tasks []*Task
	for rows.Next() {
		var task Task
		var resultFileKey, errorMessage, runID, createdBy sql.NullString
		var completedAt sql.NullTime

		err := rows.Scan(
			&task.TaskID,
			&task.TenantID,
			&task.TaskType,
			&task.Status,
			&task.Progress,
			&task.ParamsJSON,
			&resultFileKey,
			&task.ResultPackets,
			&task.ResultBytes,
			&task.FilesScanned,
			&errorMessage,
			&runID,
			&createdBy,
			&task.CreatedAt,
			&task.UpdatedAt,
			&completedAt,
		)
		if err != nil {
			return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan task")
		}

		if resultFileKey.Valid {
			task.ResultFileKey = resultFileKey.String
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
	// 使用 CTE 和 FOR UPDATE SKIP LOCKED
	// 1. 选择待处理任务并加锁
	// 2. 立即更新状态为 processing
	// 3. 返回被选中的任务
	query := `
		WITH selected_tasks AS (
			SELECT task_id
			FROM tasks
			WHERE status = $1
			ORDER BY created_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE tasks t
		SET status = $3, updated_at = $4
		FROM selected_tasks s
		WHERE t.task_id = s.task_id
		RETURNING t.task_id, t.tenant_id, t.task_type, t.status, t.progress, t.params,
			t.result_file_key, t.result_packets, t.result_bytes, t.files_scanned,
			t.error_message, t.run_id, t.created_by, t.created_at, t.updated_at, t.completed_at
	`

	rows, err := r.client.Query(ctx, query, TaskStatusQueued, limit, TaskStatusProcessing, time.Now())
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get pending tasks")
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var task Task
		var resultFileKey, errorMessage, runID, createdBy sql.NullString
		var completedAt sql.NullTime

		err := rows.Scan(
			&task.TaskID,
			&task.TenantID,
			&task.TaskType,
			&task.Status,
			&task.Progress,
			&task.ParamsJSON,
			&resultFileKey,
			&task.ResultPackets,
			&task.ResultBytes,
			&task.FilesScanned,
			&errorMessage,
			&runID,
			&createdBy,
			&task.CreatedAt,
			&task.UpdatedAt,
			&completedAt,
		)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan task")
		}

		if resultFileKey.Valid {
			task.ResultFileKey = resultFileKey.String
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

		tasks = append(tasks, &task)
	}

	if len(tasks) > 0 {
		r.logger.Debug("Acquired pending tasks",
			zap.Int("count", len(tasks)))
	}

	return tasks, rows.Err()
}

// CleanupOldTasks 清理旧任务
func (r *TaskRepository) CleanupOldTasks(ctx context.Context, olderThan time.Duration) (int64, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.CleanupOldTasks")
	defer span.End()

	cutoff := time.Now().Add(-olderThan)

	query := `
		DELETE FROM tasks
		WHERE completed_at IS NOT NULL AND completed_at < $1
	`

	result, err := r.client.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to cleanup old tasks")
	}

	rowsAffected, _ := result.RowsAffected()

	if rowsAffected > 0 {
		r.logger.Info("Cleaned up old tasks",
			zap.Int64("deleted", rowsAffected),
			zap.Duration("older_than", olderThan))
	}

	return rowsAffected, nil
}

// GetTaskStats 获取任务统计
func (r *TaskRepository) GetTaskStats(ctx context.Context, tenantID string) (map[string]int64, error) {
	ctx, span := otel.StartSpan(ctx, "TaskRepository.GetTaskStats")
	defer span.End()

	query := `
		SELECT status, COUNT(*) as count
		FROM tasks
		WHERE tenant_id = $1
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

	cutoff := time.Now().Add(-stuckDuration)

	query := `
		UPDATE tasks
		SET status = $1, updated_at = $2
		WHERE status = $3 AND updated_at < $4
	`

	result, err := r.client.Exec(ctx, query, TaskStatusQueued, time.Now(), TaskStatusProcessing, cutoff)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to reset stuck tasks")
	}

	rowsAffected, _ := result.RowsAffected()

	if rowsAffected > 0 {
		r.logger.Warn("Reset stuck tasks",
			zap.Int64("count", rowsAffected),
			zap.Duration("stuck_duration", stuckDuration))
	}

	return rowsAffected, nil
}

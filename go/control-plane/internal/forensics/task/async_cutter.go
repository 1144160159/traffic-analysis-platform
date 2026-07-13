////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/forensics/task/async_cutter.go
// 完整修复版：
// - ✅ 修复 P9: 添加任务超时控制（30分钟）
// - ✅ 优化优雅关闭逻辑
// - ✅ 改进错误处理和日志记录
////////////////////////////////////////////////////////////////////////////////

package task

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/cutter"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

// CutTaskRequest 裁剪任务请求
type CutTaskRequest struct {
	TenantID    string `json:"tenant_id"`
	UserID      string `json:"user_id"`
	ProbeID     string `json:"probe_id,omitempty"`
	SrcIP       string `json:"src_ip,omitempty"`
	DstIP       string `json:"dst_ip,omitempty"`
	SrcPort     uint16 `json:"src_port,omitempty"`
	DstPort     uint16 `json:"dst_port,omitempty"`
	Protocol    uint8  `json:"protocol,omitempty"`
	CommunityID string `json:"community_id,omitempty"`
	StartTime   int64  `json:"start_time"`
	EndTime     int64  `json:"end_time"`
	MaxPackets  int64  `json:"max_packets,omitempty"`
}

// Validate 验证请求
func (r *CutTaskRequest) Validate() error {
	if r.TenantID == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if r.StartTime == 0 || r.EndTime == 0 {
		return errors.New(errors.ErrCodeInvalidParameter, "start_time and end_time are required")
	}
	if r.EndTime < r.StartTime {
		return errors.New(errors.ErrCodeInvalidParameter, "end_time must be greater than start_time")
	}
	return nil
}

// ToCutQuery 转换为裁剪查询
func (r *CutTaskRequest) ToCutQuery() *cutter.CutQuery {
	return &cutter.CutQuery{
		TenantID:    r.TenantID,
		ProbeID:     r.ProbeID,
		SrcIP:       r.SrcIP,
		DstIP:       r.DstIP,
		SrcPort:     r.SrcPort,
		DstPort:     r.DstPort,
		Protocol:    r.Protocol,
		CommunityID: r.CommunityID,
		StartTime:   r.StartTime,
		EndTime:     r.EndTime,
		MaxPackets:  r.MaxPackets,
	}
}

// CutTaskResponse 裁剪任务响应
type CutTaskResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// AsyncCutterConfig 异步裁剪器配置
type AsyncCutterConfig struct {
	WorkerCount     int
	QueueSize       int
	ResultExpiry    time.Duration
	PollInterval    time.Duration
	ShutdownTimeout time.Duration
	TaskTimeout     time.Duration // 新增：单个任务超时时间
}

// DefaultAsyncCutterConfig 默认配置
func DefaultAsyncCutterConfig() AsyncCutterConfig {
	return AsyncCutterConfig{
		WorkerCount:     3,
		QueueSize:       100,
		ResultExpiry:    24 * time.Hour,
		PollInterval:    5 * time.Second,
		ShutdownTimeout: 30 * time.Second,
		TaskTimeout:     30 * time.Minute, // 默认 30 分钟超时
	}
}

// AsyncCutter 异步裁剪器
type AsyncCutter struct {
	cutter   *cutter.Cutter
	s3Client *s3client.S3Client
	taskRepo *repository.TaskRepository
	config   AsyncCutterConfig
	logger   *zap.Logger

	taskQueue  chan *repository.Task
	cancelMap  map[string]context.CancelFunc
	cancelLock sync.RWMutex
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	running    int32 // 原子标志
}

// NewAsyncCutter 创建异步裁剪器
func NewAsyncCutter(
	cutter *cutter.Cutter,
	s3Client *s3client.S3Client,
	taskRepo *repository.TaskRepository,
	workerCount int,
	queueSize int,
	resultExpiry time.Duration,
	logger *zap.Logger,
) *AsyncCutter {
	cfg := DefaultAsyncCutterConfig()
	if workerCount > 0 {
		cfg.WorkerCount = workerCount
	}
	if queueSize > 0 {
		cfg.QueueSize = queueSize
	}
	if resultExpiry > 0 {
		cfg.ResultExpiry = resultExpiry
	}

	return NewAsyncCutterWithConfig(cutter, s3Client, taskRepo, cfg, logger)
}

// NewAsyncCutterWithConfig 使用配置创建异步裁剪器
func NewAsyncCutterWithConfig(
	cutter *cutter.Cutter,
	s3Client *s3client.S3Client,
	taskRepo *repository.TaskRepository,
	cfg AsyncCutterConfig,
	logger *zap.Logger,
) *AsyncCutter {
	return &AsyncCutter{
		cutter:    cutter,
		s3Client:  s3Client,
		taskRepo:  taskRepo,
		config:    cfg,
		logger:    logger,
		taskQueue: make(chan *repository.Task, cfg.QueueSize),
		cancelMap: make(map[string]context.CancelFunc),
	}
}

// Start 启动异步处理器
func (a *AsyncCutter) Start(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&a.running, 0, 1) {
		a.logger.Warn("AsyncCutter already running")
		return
	}

	a.ctx, a.cancel = context.WithCancel(ctx)

	// 启动 worker
	for i := 0; i < a.config.WorkerCount; i++ {
		a.wg.Add(1)
		go a.worker(i)
	}

	// 启动任务轮询器
	a.wg.Add(1)
	go a.taskPoller()

	// 启动清理器
	a.wg.Add(1)
	go a.cleaner()

	a.logger.Info("AsyncCutter started",
		zap.Int("workers", a.config.WorkerCount),
		zap.Int("queue_size", a.config.QueueSize),
		zap.Duration("task_timeout", a.config.TaskTimeout))
}

// Stop 停止异步处理器（优化版：带超时的优雅关闭）
func (a *AsyncCutter) Stop() {
	if !atomic.CompareAndSwapInt32(&a.running, 1, 0) {
		a.logger.Warn("AsyncCutter not running")
		return
	}

	a.logger.Info("Stopping AsyncCutter...")

	// 取消所有正在运行的任务
	a.cancelLock.Lock()
	runningCount := len(a.cancelMap)
	for taskID, cancelFn := range a.cancelMap {
		a.logger.Info("Cancelling running task", zap.String("task_id", taskID))
		cancelFn()
	}
	a.cancelLock.Unlock()

	// 关闭 context
	if a.cancel != nil {
		a.cancel()
	}

	// 等待所有 worker 完成（带超时）
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		a.logger.Info("AsyncCutter stopped gracefully",
			zap.Int("cancelled_tasks", runningCount))
	case <-time.After(a.config.ShutdownTimeout):
		a.logger.Warn("AsyncCutter shutdown timed out, some tasks may be interrupted",
			zap.Duration("timeout", a.config.ShutdownTimeout),
			zap.Int("running_tasks", runningCount))
	}
}

// SubmitTask 提交任务
func (a *AsyncCutter) SubmitTask(ctx context.Context, req *CutTaskRequest) (*CutTaskResponse, error) {
	ctx, span := otel.StartSpan(ctx, "AsyncCutter.SubmitTask")
	defer span.End()

	// 验证请求
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// 序列化参数
	paramsJSON, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal params")
	}

	// 创建任务
	task := &repository.Task{
		TaskID:     uuid.New().String(),
		TenantID:   req.TenantID,
		TaskType:   repository.TaskTypePcapCut,
		Status:     repository.TaskStatusQueued,
		Progress:   0,
		ParamsJSON: paramsJSON,
		CreatedBy:  req.UserID,
	}

	// 保存到数据库
	if err := a.taskRepo.Create(ctx, task); err != nil {
		return nil, err
	}

	// 尝试加入队列
	select {
	case a.taskQueue <- task:
		a.logger.Info("Task submitted",
			zap.String("task_id", task.TaskID),
			zap.String("tenant_id", task.TenantID))
	default:
		// 队列满了，任务会通过轮询器处理
		a.logger.Warn("Task queue full, task will be picked up by poller",
			zap.String("task_id", task.TaskID))
	}

	return &CutTaskResponse{
		JobID:     task.TaskID,
		Status:    task.Status,
		CreatedAt: task.CreatedAt,
	}, nil
}

// CancelTask 取消任务
func (a *AsyncCutter) CancelTask(ctx context.Context, taskID string) error {
	ctx, span := otel.StartSpan(ctx, "AsyncCutter.CancelTask")
	defer span.End()

	// 取消正在运行的任务
	a.cancelLock.RLock()
	cancelFn, running := a.cancelMap[taskID]
	a.cancelLock.RUnlock()

	if running {
		cancelFn()
		a.logger.Info("Task cancelled", zap.String("task_id", taskID))
	}

	// 更新数据库状态
	return a.taskRepo.Cancel(ctx, taskID)
}

// worker 工作协程
func (a *AsyncCutter) worker(id int) {
	defer a.wg.Done()

	a.logger.Debug("Worker started", zap.Int("worker_id", id))

	for {
		select {
		case <-a.ctx.Done():
			a.logger.Debug("Worker stopping", zap.Int("worker_id", id))
			return

		case task, ok := <-a.taskQueue:
			if !ok {
				a.logger.Debug("Task queue closed", zap.Int("worker_id", id))
				return
			}
			a.processTask(task)
		}
	}
}

// processTask 处理任务（修复版：添加超时控制）
func (a *AsyncCutter) processTask(task *repository.Task) {
	// ✅ 修复 P9: 设置任务超时
	ctx, cancel := context.WithTimeout(a.ctx, a.config.TaskTimeout)

	// 注册取消函数
	a.cancelLock.Lock()
	a.cancelMap[task.TaskID] = cancel
	a.cancelLock.Unlock()

	defer func() {
		// 移除取消函数
		a.cancelLock.Lock()
		delete(a.cancelMap, task.TaskID)
		a.cancelLock.Unlock()
		cancel()
	}()

	a.logger.Info("Processing task",
		zap.String("task_id", task.TaskID),
		zap.String("tenant_id", task.TenantID),
		zap.Duration("timeout", a.config.TaskTimeout))

	// 更新状态为处理中
	if err := a.taskRepo.UpdateStatus(ctx, task.TaskID, repository.TaskStatusProcessing); err != nil {
		a.logger.Error("Failed to update task status", zap.Error(err))
		return
	}

	// 解析参数
	var req CutTaskRequest
	if err := json.Unmarshal(task.ParamsJSON, &req); err != nil {
		a.failTask(ctx, task.TaskID, fmt.Sprintf("failed to parse params: %v", err))
		return
	}

	// 构建查询
	query := req.ToCutQuery()

	// 生成结果文件路径
	outputKey := fmt.Sprintf("results/%s/%s/%s.pcap",
		req.TenantID,
		time.Now().Format("2006/01/02"),
		task.TaskID)

	// 进度回调
	progressCb := func(filesProcessed, totalFiles int, packetsFound int64) {
		progress := 0
		if totalFiles > 0 {
			progress = filesProcessed * 100 / totalFiles
		}
		if progress > 99 {
			progress = 99 // 保留最后 1% 给上传
		}

		// 异步更新进度（不阻塞）
		go func() {
			updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer updateCancel()
			_ = a.taskRepo.UpdateProgress(updateCtx, task.TaskID, progress, packetsFound)
		}()
	}

	// 执行裁剪
	result, err := a.cutter.CutToFile(ctx, query, outputKey, progressCb)
	if err != nil {
		// ✅ 检查取消原因
		if ctx.Err() == context.Canceled {
			a.logger.Info("Task cancelled by user", zap.String("task_id", task.TaskID))
			return
		}
		// ✅ 检查超时
		if ctx.Err() == context.DeadlineExceeded {
			a.failTask(context.Background(), task.TaskID,
				fmt.Sprintf("task timeout after %s", a.config.TaskTimeout))
			a.logger.Warn("Task timed out",
				zap.String("task_id", task.TaskID),
				zap.Duration("timeout", a.config.TaskTimeout))
			return
		}
		// 其他错误
		a.failTask(context.Background(), task.TaskID, fmt.Sprintf("cut failed: %v", err))
		return
	}

	// 标记完成
	completionCtx, completionCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer completionCancel()

	if err := a.taskRepo.Complete(completionCtx, task.TaskID, outputKey, result.SHA256, result.TotalPackets, result.TotalBytes, result.FilesScanned); err != nil {
		a.logger.Error("Failed to complete task",
			zap.String("task_id", task.TaskID),
			zap.Error(err))
		return
	}

	a.logger.Info("Task completed",
		zap.String("task_id", task.TaskID),
		zap.String("sha256", result.SHA256),
		zap.Int64("packets", result.TotalPackets),
		zap.Int64("bytes", result.TotalBytes),
		zap.Int("files", result.FilesScanned),
		zap.Duration("duration", result.Duration))
}

// failTask 标记任务失败
func (a *AsyncCutter) failTask(ctx context.Context, taskID, errorMsg string) {
	a.logger.Error("Task failed",
		zap.String("task_id", taskID),
		zap.String("error", errorMsg))

	// 使用独立的 context 避免超时影响
	failCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.taskRepo.Fail(failCtx, taskID, errorMsg); err != nil {
		a.logger.Error("Failed to mark task as failed",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

// taskPoller 任务轮询器（处理队列遗漏的任务）
func (a *AsyncCutter) taskPoller() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return

		case <-ticker.C:
			a.pollPendingTasks()
		}
	}
}

// pollPendingTasks 轮询待处理任务
func (a *AsyncCutter) pollPendingTasks() {
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()

	tasks, err := a.taskRepo.GetPendingTasks(ctx, 10)
	if err != nil {
		a.logger.Error("Failed to poll pending tasks", zap.Error(err))
		return
	}

	for _, task := range tasks {
		select {
		case a.taskQueue <- task:
			a.logger.Debug("Polled task enqueued", zap.String("task_id", task.TaskID))
		default:
			// 队列满，下次再试
			return
		}
	}
}

// cleaner 清理器（清理过期任务和文件）
func (a *AsyncCutter) cleaner() {
	defer a.wg.Done()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return

		case <-ticker.C:
			a.cleanupExpiredTasks()
		}
	}
}

// cleanupExpiredTasks 清理过期任务
func (a *AsyncCutter) cleanupExpiredTasks() {
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()

	// 清理数据库记录
	deleted, err := a.taskRepo.CleanupOldTasks(ctx, a.config.ResultExpiry)
	if err != nil {
		a.logger.Error("Failed to cleanup old tasks", zap.Error(err))
		return
	}

	if deleted > 0 {
		a.logger.Info("Cleaned up expired tasks", zap.Int64("count", deleted))
	}

	// 清理 S3 中的过期文件: 查询 completed/failed 状态超过 72h 的任务, 清理关联的 S3 对象
	if a.s3Client != nil {
		cutoffTime := time.Now().Add(-72 * time.Hour)
		// 查询已完成的任务 (复用 List API, 后续可优化为专用 expired query)
		completedTasks, _, err := a.taskRepo.List(ctx, "", "completed", 50, 0)
		if err != nil {
			a.logger.Warn("Failed to list tasks for S3 cleanup", zap.Error(err))
		} else {
			cleanedCount := 0
			for _, task := range completedTasks {
				if task.CreatedAt.Before(cutoffTime) && task.ResultFileKey != "" {
					if err := a.s3Client.DeleteObject(ctx, task.ResultFileKey); err != nil {
						a.logger.Warn("Failed to delete S3 object",
							zap.String("key", task.ResultFileKey), zap.Error(err))
						continue
					}
					cleanedCount++
				}
			}
			if cleanedCount > 0 {
				a.logger.Info("Cleaned up expired S3 files",
					zap.Int("file_count", cleanedCount))
			}
		}
	}
}

// GetQueueLength 获取队列长度
func (a *AsyncCutter) GetQueueLength() int {
	return len(a.taskQueue)
}

// GetRunningTaskCount 获取正在运行的任务数
func (a *AsyncCutter) GetRunningTaskCount() int {
	a.cancelLock.RLock()
	defer a.cancelLock.RUnlock()
	return len(a.cancelMap)
}

// IsRunning 检查是否正在运行
func (a *AsyncCutter) IsRunning() bool {
	return atomic.LoadInt32(&a.running) == 1
}

// GetStats 获取统计信息
func (a *AsyncCutter) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"running":        a.IsRunning(),
		"queue_length":   a.GetQueueLength(),
		"queue_capacity": a.config.QueueSize,
		"running_tasks":  a.GetRunningTaskCount(),
		"worker_count":   a.config.WorkerCount,
		"task_timeout":   a.config.TaskTimeout.String(),
	}
}

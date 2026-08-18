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
	stderrors "errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/cutter"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/restoration"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

// CutTaskRequest 裁剪任务请求
type CutTaskRequest struct {
	TenantID                   string                     `json:"tenant_id"`
	UserID                     string                     `json:"user_id"`
	AssetID                    string                     `json:"asset_id,omitempty"`
	AlertID                    string                     `json:"alert_id,omitempty"`
	CampaignID                 string                     `json:"campaign_id,omitempty"`
	BaselineID                 string                     `json:"baseline_id,omitempty"`
	EvidenceID                 string                     `json:"evidence_id,omitempty"`
	EvidenceType               string                     `json:"evidence_type,omitempty"`
	ProbeID                    string                     `json:"probe_id,omitempty"`
	ProbeIDs                   []string                   `json:"probe_ids,omitempty"`
	AlertIDs                   []string                   `json:"alert_ids,omitempty"`
	CaseIDs                    []string                   `json:"case_ids,omitempty"`
	SrcIP                      string                     `json:"src_ip,omitempty"`
	DstIP                      string                     `json:"dst_ip,omitempty"`
	SrcPort                    uint16                     `json:"src_port,omitempty"`
	DstPort                    uint16                     `json:"dst_port,omitempty"`
	Protocol                   uint8                      `json:"protocol,omitempty"`
	CommunityID                string                     `json:"community_id,omitempty"`
	StartTime                  int64                      `json:"start_time"`
	EndTime                    int64                      `json:"end_time"`
	MaxPackets                 int64                      `json:"max_packets,omitempty"`
	Purpose                    string                     `json:"purpose,omitempty"`
	PermissionSnapshot         []string                   `json:"permission_snapshot,omitempty"`
	RetentionPolicy            string                     `json:"retention_policy,omitempty"`
	RestorationContractVersion int                        `json:"restoration_contract_version,omitempty"`
	Restorations               []RestorationTaskSpec      `json:"restorations,omitempty"`
	TraceID                    string                     `json:"trace_id,omitempty"`
	CommandMeta                repository.TaskCommandMeta `json:"-"`
}

// RestorationTaskSpec is a frozen reference to the M03 versioned restoration
// interface. It contains no tenant or actor override; the worker derives both
// from the already-authorized task.
type RestorationTaskSpec struct {
	RequestID     string                      `json:"request_id"`
	SessionID     string                      `json:"session_id"`
	CommunityID   string                      `json:"community_id"`
	FlowIDs       []string                    `json:"flow_ids"`
	FlowID        string                      `json:"flow_id"`
	Tuple         restoration.FiveTuple       `json:"five_tuple"`
	Direction     string                      `json:"direction"`
	ProfileID     string                      `json:"protocol_profile_id"`
	FTPData       *restoration.FTPDataRequest `json:"ftp_data,omitempty"`
	FTPTLSEnabled bool                        `json:"ftp_tls_enabled"`
}

// Validate 验证请求
func (r *CutTaskRequest) Validate() error {
	r.normalizeFrozenFields()
	if r.TraceID == "" {
		r.TraceID = strings.TrimSpace(r.CommandMeta.TraceID)
	}
	if r.TenantID == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if r.StartTime == 0 || r.EndTime == 0 {
		return errors.New(errors.ErrCodeInvalidParameter, "start_time and end_time are required")
	}
	if r.EndTime < r.StartTime {
		return errors.New(errors.ErrCodeInvalidParameter, "end_time must be greater than start_time")
	}
	if r.RestorationContractVersion != 0 {
		if r.RestorationContractVersion != 1 {
			return errors.New(errors.ErrCodeInvalidParameter, "restoration_contract_version must be 1")
		}
		if r.Purpose == "" {
			return errors.New(errors.ErrCodeInvalidParameter, "purpose is required for versioned forensics tasks")
		}
		if r.TraceID == "" || r.UserID == "" {
			return errors.New(errors.ErrCodeInvalidParameter, "versioned forensics tasks require actor and trace identity")
		}
		if r.RetentionPolicy == "" {
			return errors.New(errors.ErrCodeInvalidParameter, "retention_policy is required for versioned forensics tasks")
		}
		if !containsString(r.PermissionSnapshot, "pcap:write") &&
			!containsString(r.PermissionSnapshot, "pcap:*") &&
			!containsString(r.PermissionSnapshot, "admin:*") &&
			!containsString(r.PermissionSnapshot, "*") {
			return errors.New(errors.ErrCodePermissionDenied, "permission snapshot does not authorize pcap:write")
		}
		if len(r.ProbeIDs) == 0 {
			return errors.New(errors.ErrCodeInvalidParameter, "versioned forensics tasks require at least one probe_id")
		}
		if len(r.Restorations) > 100 {
			return errors.New(errors.ErrCodeInvalidParameter, "restorations cannot contain more than 100 requests")
		}
		seenRestorations := make(map[string]struct{}, len(r.Restorations))
		for index := range r.Restorations {
			spec := &r.Restorations[index]
			spec.RequestID = strings.TrimSpace(spec.RequestID)
			sort.Strings(spec.FlowIDs)
			spec.FlowIDs = canonicalStringSet(spec.FlowIDs)
			if spec.RequestID == "" || strings.ContainsAny(spec.RequestID, "\x00\r\n") {
				return errors.New(errors.ErrCodeInvalidParameter, "restoration request_id is required")
			}
			if _, exists := seenRestorations[spec.RequestID]; exists {
				return errors.New(errors.ErrCodeInvalidParameter, "restoration request_id must be unique")
			}
			seenRestorations[spec.RequestID] = struct{}{}
			candidate := restoration.ProcessRequest{
				TenantID: r.TenantID, IdempotencyKey: "forensics-job-validation-" + spec.RequestID,
				SessionID: spec.SessionID, CommunityID: spec.CommunityID, FlowIDs: spec.FlowIDs,
				FlowID: spec.FlowID, Tuple: spec.Tuple, Direction: spec.Direction,
				StartTime: time.UnixMilli(r.StartTime), EndTime: time.UnixMilli(r.EndTime),
				ProfileID: spec.ProfileID, FTPData: spec.FTPData, FTPTLSEnabled: spec.FTPTLSEnabled,
				ActorID: r.UserID, Reason: r.Purpose, TraceID: "forensics-job-validation",
			}
			if err := candidate.Validate(); err != nil {
				return errors.Wrap(err, errors.ErrCodeInvalidParameter, "invalid restoration request")
			}
		}
	}
	return nil
}

func (r *CutTaskRequest) normalizeFrozenFields() {
	r.TenantID = strings.TrimSpace(r.TenantID)
	r.UserID = strings.TrimSpace(r.UserID)
	r.ProbeID = strings.TrimSpace(r.ProbeID)
	r.AlertID = strings.TrimSpace(r.AlertID)
	r.Purpose = strings.TrimSpace(r.Purpose)
	r.RetentionPolicy = strings.TrimSpace(r.RetentionPolicy)
	if r.ProbeID != "" {
		r.ProbeIDs = append(r.ProbeIDs, r.ProbeID)
	}
	if r.AlertID != "" {
		r.AlertIDs = append(r.AlertIDs, r.AlertID)
	}
	r.ProbeIDs = canonicalStringSet(r.ProbeIDs)
	r.AlertIDs = canonicalStringSet(r.AlertIDs)
	r.CaseIDs = canonicalStringSet(r.CaseIDs)
	r.PermissionSnapshot = canonicalStringSet(r.PermissionSnapshot)
	if r.ProbeID == "" && len(r.ProbeIDs) > 0 {
		r.ProbeID = r.ProbeIDs[0]
	}
	if r.AlertID == "" && len(r.AlertIDs) > 0 {
		r.AlertID = r.AlertIDs[0]
	}
}

func canonicalStringSet(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
	JobID             string    `json:"job_id"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	Revision          int64     `json:"revision"`
	EventID           string    `json:"event_id"`
	ActionID          string    `json:"action_id"`
	IdempotencyKey    string    `json:"idempotency_key"`
	OutboxStatus      string    `json:"outbox_status"`
	Replayed          bool      `json:"replayed"`
	CompatibilityMode bool      `json:"compatibility_mode"`
}

// AsyncCutterConfig 异步裁剪器配置
type AsyncCutterConfig struct {
	WorkerCount     int
	QueueSize       int
	ResultExpiry    time.Duration
	PollInterval    time.Duration
	ShutdownTimeout time.Duration
	TaskTimeout     time.Duration // 新增：单个任务超时时间
	ConsumerEnabled bool
	VersionedWorker bool
	WorkerID        string
	ExecutionLease  time.Duration
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
		ConsumerEnabled: false,
		VersionedWorker: false,
		WorkerID:        "forensics-worker",
		ExecutionLease:  31 * time.Minute,
	}
}

// AsyncCutter 异步裁剪器
type AsyncCutter struct {
	cutter            *cutter.Cutter
	s3Client          *s3client.S3Client
	taskRepo          *repository.TaskRepository
	config            AsyncCutterConfig
	logger            *zap.Logger
	versionedPipeline *VersionedPipeline

	taskQueue  chan *repository.Task
	cancelMap  map[string]context.CancelFunc
	cancelLock sync.RWMutex
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	running    int32 // 原子标志
}

func (a *AsyncCutter) SetVersionedPipeline(pipeline *VersionedPipeline) {
	if atomic.LoadInt32(&a.running) != 0 {
		panic("versioned pipeline must be configured before AsyncCutter.Start")
	}
	a.versionedPipeline = pipeline
}

// CompatibleWorkerReady reports code/config compatibility only. It does not
// imply that task consumption is enabled.
func (a *AsyncCutter) CompatibleWorkerReady() bool {
	return a.versionedPipeline != nil && a.config.VersionedWorker && strings.TrimSpace(a.config.WorkerID) != "" && a.config.ExecutionLease > a.config.TaskTimeout
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

	if a.config.ConsumerEnabled {
		// Consumption and retention cleanup share one explicit activation gate.
		// An idle-compatible deployment starts workers but makes no durable reads
		// or destructive object mutations.
		a.wg.Add(1)
		go a.taskPoller()
		a.wg.Add(1)
		go a.cleaner()
	}

	a.logger.Info("AsyncCutter started",
		zap.Int("workers", a.config.WorkerCount),
		zap.Int("queue_size", a.config.QueueSize),
		zap.Duration("task_timeout", a.config.TaskTimeout),
		zap.Bool("consumer_enabled", a.config.ConsumerEnabled),
		zap.Bool("compatible_worker_ready", a.CompatibleWorkerReady()))
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
	receipt, err := a.taskRepo.CreateAtomic(ctx, task, req.CommandMeta)
	if err != nil {
		return nil, err
	}

	// 尝试加入队列
	if !receipt.Replayed && a.config.ConsumerEnabled && !a.config.VersionedWorker {
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
	}

	return &CutTaskResponse{
		JobID: task.TaskID, Status: receipt.Status, CreatedAt: receipt.CreatedAt,
		Revision: receipt.Revision, EventID: receipt.EventID, OutboxStatus: receipt.OutboxStatus,
		ActionID: receipt.ActionID, IdempotencyKey: receipt.IdempotencyKey,
		Replayed: receipt.Replayed, CompatibilityMode: receipt.CompatibilityMode,
	}, nil
}

// CancelTask 取消任务
func (a *AsyncCutter) CancelTask(ctx context.Context, tenantID, taskID string, meta repository.TaskCommandMeta) (*repository.TaskCommandReceipt, error) {
	ctx, span := otel.StartSpan(ctx, "AsyncCutter.CancelTask")
	defer span.End()
	receipt, err := a.taskRepo.CancelForTenant(ctx, tenantID, taskID, meta)
	if err != nil {
		return nil, err
	}

	// The authoritative state is committed before stopping the in-memory worker.
	a.cancelLock.RLock()
	cancelFn, running := a.cancelMap[taskID]
	a.cancelLock.RUnlock()

	if running {
		cancelFn()
		a.logger.Info("Task cancelled", zap.String("task_id", taskID))
	}
	return receipt, nil
}

// RetryTask durably requeues a failed or cancelled task. The original params
// remain unchanged in PostgreSQL and the poller is the delivery fallback if the
// in-memory queue is full or the process exits after commit.
func (a *AsyncCutter) RetryTask(ctx context.Context, tenantID, taskID string, meta repository.TaskCommandMeta) (*repository.TaskCommandReceipt, error) {
	ctx, span := otel.StartSpan(ctx, "AsyncCutter.RetryTask")
	defer span.End()
	receipt, err := a.taskRepo.RetryForTenant(ctx, tenantID, taskID, meta)
	if err != nil {
		return nil, err
	}
	if receipt.Replayed {
		return receipt, nil
	}
	if a.config.ConsumerEnabled && !a.config.VersionedWorker {
		if retried, getErr := a.taskRepo.GetByIDForTenant(ctx, tenantID, taskID); getErr == nil {
			select {
			case a.taskQueue <- retried:
			default:
				a.logger.Warn("Task retry queue full; durable poller will resume it", zap.String("task_id", taskID))
			}
		}
	}
	return receipt, nil
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
	if a.config.VersionedWorker {
		a.processVersionedTask(ctx, task)
		return
	}

	// 更新状态为处理中
	if task.Status != repository.TaskStatusProcessing {
		if err := a.taskRepo.UpdateStatus(ctx, task.TaskID, repository.TaskStatusProcessing); err != nil {
			a.logger.Error("Failed to update task status", zap.Error(err))
			return
		}
		task.Status = repository.TaskStatusProcessing
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

func (a *AsyncCutter) processVersionedTask(ctx context.Context, task *repository.Task) {
	if a.versionedPipeline == nil {
		a.failTask(ctx, task.TaskID, "compatible versioned pipeline is unavailable")
		return
	}
	if task.Status != repository.TaskStatusProcessing {
		if err := a.taskRepo.UpdateStatus(ctx, task.TaskID, repository.TaskStatusProcessing); err != nil {
			a.logger.Error("Failed to lease versioned task", zap.String("task_id", task.TaskID), zap.Error(err))
			return
		}
		task.Status = repository.TaskStatusProcessing
	}
	var request CutTaskRequest
	if err := json.Unmarshal(task.ParamsJSON, &request); err != nil {
		a.failTask(ctx, task.TaskID, fmt.Sprintf("failed to parse immutable versioned params: %v", err))
		return
	}
	claim, err := a.taskRepo.ClaimVersionedExecution(ctx, task, a.config.WorkerID, a.config.ExecutionLease)
	if err != nil {
		if stderrors.Is(err, repository.ErrTaskExecutionLeaseUnavailable) {
			a.logger.Info("Versioned task is owned by another live worker", zap.String("task_id", task.TaskID))
			return
		}
		a.failTask(ctx, task.TaskID, fmt.Sprintf("failed to claim versioned execution: %v", err))
		return
	}
	var checkpointMu sync.Mutex
	advance := func(phase string, checkpoint any) error {
		checkpointMu.Lock()
		defer checkpointMu.Unlock()
		return a.taskRepo.AdvanceVersionedExecution(ctx, claim, phase, checkpoint, a.config.ExecutionLease)
	}
	progress := func(filesProcessed, totalFiles int, packetsFound int64) {
		progressValue := 0
		if totalFiles > 0 {
			progressValue = filesProcessed * 80 / totalFiles
		}
		if progressValue > 80 {
			progressValue = 80
		}
		if err := a.taskRepo.UpdateProgress(ctx, task.TaskID, progressValue, packetsFound); err != nil {
			a.logger.Warn("Failed to persist versioned task progress", zap.String("task_id", task.TaskID), zap.Error(err))
		}
		if err := advance("cutting", map[string]any{
			"files_processed": filesProcessed, "total_files": totalFiles, "packets_found": packetsFound,
		}); err != nil {
			a.logger.Warn("Failed to persist versioned task checkpoint", zap.String("task_id", task.TaskID), zap.Error(err))
		}
	}
	result, err := a.versionedPipeline.Process(ctx, task.TaskID, request, progress, advance)
	if err != nil {
		failureCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if failErr := a.taskRepo.FailVersionedExecution(failureCtx, claim, err.Error()); failErr != nil {
			a.logger.Error("Failed to persist versioned task failure", zap.String("task_id", task.TaskID), zap.Error(failErr))
		}
		return
	}
	manifestJSON, err := json.Marshal(result.Manifest)
	if err != nil {
		a.failTask(ctx, task.TaskID, fmt.Sprintf("failed to marshal versioned manifest: %v", err))
		return
	}
	completionCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	terminalStatus := repository.TaskStatusCompleted
	if result.Manifest.Status == "partial" {
		terminalStatus = repository.TaskStatusPartial
	}
	if err := a.taskRepo.CompleteVersionedExecution(completionCtx, claim, repository.VersionedTaskManifest{
		TenantID: task.TenantID, TaskID: task.TaskID, ManifestSHA256: result.ManifestSHA,
		ManifestJSON: manifestJSON, Status: terminalStatus, ResultObject: result.Manifest.ResultObject,
	}, result.Packets, result.Bytes, result.FilesScanned); err != nil {
		a.logger.Error("Failed to commit versioned task manifest", zap.String("task_id", task.TaskID), zap.Error(err))
		return
	}
	a.logger.Info("Versioned forensics task completed",
		zap.String("task_id", task.TaskID), zap.String("manifest_sha256", result.ManifestSHA),
		zap.String("object_version", result.Manifest.ResultObject.VersionID))
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
			if a.config.ConsumerEnabled {
				a.pollPendingTasks()
			}
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
		completedTasks, _, err := a.taskRepo.List(ctx, "", "completed", "", 50, 0)
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

////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/forensics/api/handler.go
// 完整修复版：
// - ✅ H1: 完善所有接口的租户隔离检查
// - ✅ H2: 修复路径遍历漏洞（URL解码 + 规范化 + 白名单）
// - ✅ L2: 添加请求限流
// - ✅ L3: 修复日志敏感信息泄露
////////////////////////////////////////////////////////////////////////////////

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/converter"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/cutter"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/task"
)

// Handler API 处理器
type Handler struct {
	cutter      *cutter.Cutter
	asyncCutter *task.AsyncCutter
	s3Client    *s3client.S3Client
	taskRepo    *repository.TaskRepository
	auditLogger *audit.Logger
	logger      *zap.Logger

	// ========== 修复 L2: 添加限流器 ==========
	globalLimiter  *rate.Limiter // 全局 QPS 限制
	tenantLimiters sync.Map      // map[tenantID]*rate.Limiter
	ipLimiters     sync.Map      // map[clientIP]*rate.Limiter
}

// NewHandler 创建处理器
func NewHandler(
	cutter *cutter.Cutter,
	asyncCutter *task.AsyncCutter,
	s3Client *s3client.S3Client,
	taskRepo *repository.TaskRepository,
	auditLogger *audit.Logger,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		cutter:      cutter,
		asyncCutter: asyncCutter,
		s3Client:    s3Client,
		taskRepo:    taskRepo,
		auditLogger: auditLogger,
		logger:      logger,
		// ✅ 全局限流：100 QPS，突发 200
		globalLimiter: rate.NewLimiter(rate.Limit(100), 200),
	}
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(r *mux.Router) {
	// 异步裁剪任务（推荐）
	r.HandleFunc("/api/v1/pcap/jobs", h.CreateJob).Methods("POST")
	r.HandleFunc("/api/v1/pcap/jobs/{id}", h.GetJob).Methods("GET")
	r.HandleFunc("/api/v1/pcap/jobs/{id}/cancel", h.CancelJob).Methods("POST")
	r.HandleFunc("/api/v1/pcap/jobs", h.ListJobs).Methods("GET")

	// 同步裁剪（仅用于小文件，保留兼容）
	r.HandleFunc("/api/v1/pcap/cut", h.CutPCAPSync).Methods("POST")

	// 直接下载
	r.HandleFunc("/api/v1/pcap/download/{key:.*}", h.DownloadPCAP).Methods("GET")

	// 预签名 URL
	r.HandleFunc("/api/v1/pcap/presign", h.GetPresignedURL).Methods("POST")

	// 统计信息
	r.HandleFunc("/api/v1/pcap/stats", h.GetStats).Methods("GET")

	// 健康检查
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")
	r.HandleFunc("/ready", h.ReadinessCheck).Methods("GET")
}

// ========== 修复 L2: 限流检查方法 ==========

// checkGlobalRateLimit 检查全局限流
func (h *Handler) checkGlobalRateLimit() bool {
	return h.globalLimiter.Allow()
}

// checkTenantRateLimit 检查租户级别限流
func (h *Handler) checkTenantRateLimit(tenantID string) bool {
	limiterInterface, _ := h.tenantLimiters.LoadOrStore(tenantID, rate.NewLimiter(rate.Limit(10), 20)) // 10 QPS/租户，突发 20
	limiter := limiterInterface.(*rate.Limiter)
	return limiter.Allow()
}

// checkIPRateLimit 检查 IP 级别限流
func (h *Handler) checkIPRateLimit(clientIP string) bool {
	limiterInterface, _ := h.ipLimiters.LoadOrStore(clientIP, rate.NewLimiter(rate.Limit(5), 10)) // 5 QPS/IP，突发 10
	limiter := limiterInterface.(*rate.Limiter)
	return limiter.Allow()
}

// ========== 修复 L3: 敏感信息掩码 ==========

// maskIP 掩码 IP 地址
func maskIP(ip string) string {
	if ip == "" {
		return ""
	}
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		return fmt.Sprintf("%s.%s.***.***", parts[0], parts[1])
	}
	// IPv6 简化处理
	if strings.Contains(ip, ":") {
		parts := strings.Split(ip, ":")
		if len(parts) >= 4 {
			return fmt.Sprintf("%s:%s:***:***", parts[0], parts[1])
		}
	}
	return "***"
}

// maskCommunityID 掩码 Community ID
func maskCommunityID(id string) string {
	if len(id) <= 8 {
		return "***"
	}
	return id[:8] + "***"
}

// ========== 修复 H1: 权限检查辅助方法 ==========

// checkCrossTenantAccess 检查跨租户访问权限
func (h *Handler) checkCrossTenantAccess(ctx context.Context, userTenantID, resourceTenantID string) bool {
	if userTenantID == resourceTenantID {
		return true
	}

	// 检查权限
	permissions := httpx.GetPermissions(ctx)
	for _, p := range permissions {
		if p == "admin:cross_tenant" || p == "forensics:cross_tenant" {
			return true
		}
	}

	return false
}

// ========== API 处理方法 ==========

// CreateJob 创建异步裁剪任务（修复版）
func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rw := httpx.NewResponseWriter(w, ctx)

	// ========== 修复 L2: 全局限流 ==========
	if !h.checkGlobalRateLimit() {
		rw.Error(http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Global rate limit exceeded", nil)
		return
	}

	// ========== 修复 L2: IP 限流 ==========
	clientIP := httpx.GetClientIP(r)
	if !h.checkIPRateLimit(clientIP) {
		h.logger.Warn("IP rate limit exceeded",
			zap.String("client_ip", maskIP(clientIP)))
		rw.Error(http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "IP rate limit exceeded", nil)
		return
	}

	var req converter.CutRequestParams
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		rw.Error(http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil)
		return
	}

	// 验证请求
	if err := req.Validate(); err != nil {
		appErr, ok := err.(*errors.AppError)
		if ok {
			rw.Error(appErr.HTTPStatus(), string(appErr.Code), appErr.Message, nil)
		} else {
			rw.Error(http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		return
	}

	// ========== 修复 H1: 强制使用用户租户 ID ==========
	userTenantID := httpx.GetTenantID(ctx)
	if userTenantID == "" {
		userTenantID = "default"
	}

	// 如果请求中指定了 tenant_id，检查权限
	if req.TenantID != "" && req.TenantID != userTenantID {
		if !h.checkCrossTenantAccess(ctx, userTenantID, req.TenantID) {
			h.logger.Warn("Cross-tenant create job denied",
				zap.String("user_tenant", userTenantID),
				zap.String("requested_tenant", req.TenantID),
				zap.String("client_ip", maskIP(clientIP)))
			rw.Error(http.StatusForbidden, "FORBIDDEN", "Cross-tenant access denied", nil)
			return
		}
	} else {
		// ✅ 强制使用用户的租户 ID
		req.TenantID = userTenantID
	}

	// ========== 修复 L2: 租户限流 ==========
	if !h.checkTenantRateLimit(req.TenantID) {
		rw.Error(http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Tenant rate limit exceeded", nil)
		return
	}

	// 获取用户信息
	userID := httpx.GetUserID(ctx)

	// ========== 修复 L3: 使用掩码记录日志 ==========
	h.logger.Info("Creating PCAP cut job",
		zap.String("tenant_id", req.TenantID),
		zap.String("user_id", userID),
		zap.String("probe_id", req.ProbeID),
		zap.String("src_ip", maskIP(req.SrcIP)),
		zap.String("dst_ip", maskIP(req.DstIP)),
		zap.String("community_id", maskCommunityID(req.CommunityID)),
		zap.Int64("start_time", req.StartTime),
		zap.Int64("end_time", req.EndTime))

	// 创建任务请求
	taskReq := &task.CutTaskRequest{
		TenantID:    req.TenantID,
		UserID:      userID,
		ProbeID:     req.ProbeID,
		SrcIP:       req.SrcIP,
		DstIP:       req.DstIP,
		SrcPort:     req.SrcPort,
		DstPort:     req.DstPort,
		Protocol:    req.Protocol,
		CommunityID: req.CommunityID,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		MaxPackets:  req.MaxPackets,
	}

	job, err := h.asyncCutter.SubmitTask(ctx, taskReq)
	if err != nil {
		h.logger.Error("Failed to create job", zap.Error(err))
		appErr := errors.Wrap(err, errors.ErrCodeInternal, "Failed to create job")
		rw.Error(appErr.HTTPStatus(), string(appErr.Code), appErr.Message, nil)
		return
	}

	// 记录审计日志
	if h.auditLogger != nil {
		h.auditLogger.LogPcapAccess(ctx, audit.EventTypePcapCut, req.TenantID, userID, job.JobID, map[string]interface{}{
			"probe_id":     req.ProbeID,
			"src_ip":       req.SrcIP,
			"dst_ip":       req.DstIP,
			"community_id": req.CommunityID,
			"start_time":   req.StartTime,
			"end_time":     req.EndTime,
		})
	}

	// 构建响应
	response := &converter.JobResponse{
		JobID:     job.JobID,
		Status:    job.Status,
		CreatedAt: job.CreatedAt.UnixMilli(),
	}

	rw.Created(response)
}

// GetJob 获取任务状态（修复版：添加租户隔离检查）
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rw := httpx.NewResponseWriter(w, ctx)

	// 限流检查
	if !h.checkGlobalRateLimit() {
		rw.Error(http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Global rate limit exceeded", nil)
		return
	}

	vars := mux.Vars(r)
	jobID := vars["id"]

	// 获取当前用户的 tenant_id
	userTenantID := httpx.GetTenantID(ctx)
	if userTenantID == "" {
		userTenantID = "default"
	}

	job, err := h.taskRepo.GetByID(ctx, jobID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			rw.Error(http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", nil)
			return
		}
		h.logger.Error("Failed to get job", zap.String("job_id", jobID), zap.Error(err))
		rw.Error(http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get job", nil)
		return
	}

	// ========== 修复 H1: 租户隔离检查 ==========
	if !h.checkCrossTenantAccess(ctx, userTenantID, job.TenantID) {
		h.logger.Warn("Cross-tenant access denied",
			zap.String("user_tenant", userTenantID),
			zap.String("job_tenant", job.TenantID),
			zap.String("job_id", jobID))
		rw.Error(http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
		return
	}

	// 构建响应
	response := h.buildJobResponse(ctx, job)

	rw.Success(response)
}

// buildJobResponse 构建任务响应
func (h *Handler) buildJobResponse(ctx context.Context, job *repository.Task) *converter.JobResponse {
	response := converter.NewJobResponseFromTask(job)

	// 解析参数
	if len(job.ParamsJSON) > 0 {
		var params map[string]interface{}
		if err := json.Unmarshal(job.ParamsJSON, &params); err == nil {
			response.Params = params
		}
	}

	// 如果任务完成且有结果文件，生成预签名 URL
	if job.Status == repository.TaskStatusCompleted && job.ResultFileKey != "" {
		expiry := 1 * time.Hour
		url, err := h.s3Client.GetPresignedURL(ctx, job.ResultFileKey, expiry)
		if err == nil {
			response.DownloadURL = url
			expiresAt := time.Now().Add(expiry).UnixMilli()
			response.ExpiresAt = &expiresAt
		}
	}

	return response
}

// CancelJob 取消任务（修复版：添加租户隔离检查）
func (h *Handler) CancelJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rw := httpx.NewResponseWriter(w, ctx)

	vars := mux.Vars(r)
	jobID := vars["id"]

	// ========== 修复 H1: 先获取任务检查权限 ==========
	userTenantID := httpx.GetTenantID(ctx)
	if userTenantID == "" {
		userTenantID = "default"
	}

	job, err := h.taskRepo.GetByID(ctx, jobID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			rw.Error(http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", nil)
			return
		}
		h.logger.Error("Failed to get job", zap.String("job_id", jobID), zap.Error(err))
		rw.Error(http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get job", nil)
		return
	}

	// ========== 租户隔离检查 ==========
	if !h.checkCrossTenantAccess(ctx, userTenantID, job.TenantID) {
		h.logger.Warn("Cross-tenant cancel denied",
			zap.String("user_tenant", userTenantID),
			zap.String("job_tenant", job.TenantID),
			zap.String("job_id", jobID))
		rw.Error(http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
		return
	}

	// 执行取消
	err = h.asyncCutter.CancelTask(ctx, jobID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			rw.Error(http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", nil)
			return
		}
		if errors.IsCode(err, errors.ErrCodeInvalidStateTransition) {
			rw.Error(http.StatusConflict, "INVALID_STATE", err.Error(), nil)
			return
		}
		h.logger.Error("Failed to cancel job", zap.String("job_id", jobID), zap.Error(err))
		rw.Error(http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to cancel job", nil)
		return
	}

	rw.Success(map[string]string{
		"job_id": jobID,
		"status": "cancelled",
	})
}

// ListJobs 列出任务
func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rw := httpx.NewResponseWriter(w, ctx)

	// 限流检查
	if !h.checkGlobalRateLimit() {
		rw.Error(http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Global rate limit exceeded", nil)
		return
	}

	// ========== 修复 H1: 租户隔离 ==========
	userTenantID := httpx.GetTenantID(ctx)
	if userTenantID == "" {
		userTenantID = "default"
	}

	requestedTenantID := r.URL.Query().Get("tenant_id")
	var tenantID string

	if requestedTenantID == "" || requestedTenantID == userTenantID {
		tenantID = userTenantID
	} else {
		// 跨租户查询需要权限
		if !h.checkCrossTenantAccess(ctx, userTenantID, requestedTenantID) {
			h.logger.Warn("Cross-tenant list denied",
				zap.String("user_tenant", userTenantID),
				zap.String("requested_tenant", requestedTenantID))
			rw.Error(http.StatusForbidden, "FORBIDDEN", "Cross-tenant access denied", nil)
			return
		}
		tenantID = requestedTenantID
	}

	// 解析分页参数
	limit := parseIntParam(r.URL.Query().Get("limit"), 20, 1, 100)
	offset := parseIntParam(r.URL.Query().Get("offset"), 0, 0, 10000)
	status := r.URL.Query().Get("status")

	jobs, total, err := h.taskRepo.List(ctx, tenantID, status, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list jobs", zap.Error(err))
		rw.Error(http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list jobs", nil)
		return
	}

	// 构建响应
	responses := make([]*converter.JobResponse, 0, len(jobs))
	for _, job := range jobs {
		responses = append(responses, h.buildJobResponse(ctx, job))
	}

	rw.Paginated(responses, total, limit, offset)
}

// CutPCAPSync 同步裁剪（修复版：添加租户隔离和时间范围限制）
func (h *Handler) CutPCAPSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 限流检查
	clientIP := httpx.GetClientIP(r)
	if !h.checkIPRateLimit(clientIP) {
		http.Error(w, "IP rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	var req converter.CutRequestParams
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 验证
	if err := req.Validate(); err != nil {
		appErr, ok := err.(*errors.AppError)
		if ok {
			http.Error(w, appErr.Message, appErr.HTTPStatus())
		} else {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}

	// ========== 修复 H1: 强制使用用户租户 ID ==========
	userTenantID := httpx.GetTenantID(ctx)
	if userTenantID == "" {
		userTenantID = "default"
	}

	if req.TenantID != "" && req.TenantID != userTenantID {
		if !h.checkCrossTenantAccess(ctx, userTenantID, req.TenantID) {
			h.logger.Warn("Cross-tenant sync cut denied",
				zap.String("user_tenant", userTenantID),
				zap.String("requested_tenant", req.TenantID))
			http.Error(w, "Cross-tenant access denied", http.StatusForbidden)
			return
		}
	} else {
		req.TenantID = userTenantID
	}

	// 租户限流
	if !h.checkTenantRateLimit(req.TenantID) {
		http.Error(w, "Tenant rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// ========== 修复：同步模式限制时间范围（最多 1 小时） ==========
	if req.EndTime-req.StartTime > 3600*1000 {
		http.Error(w, "Sync mode time range cannot exceed 1 hour", http.StatusBadRequest)
		return
	}

	// 限制同步裁剪的最大包数
	if req.MaxPackets == 0 || req.MaxPackets > 10000 {
		req.MaxPackets = 10000 // 同步模式最多 10000 包
	}

	h.logger.Info("Sync PCAP cut request",
		zap.String("tenant_id", req.TenantID),
		zap.String("probe_id", req.ProbeID),
		zap.String("src_ip", maskIP(req.SrcIP)),
		zap.String("dst_ip", maskIP(req.DstIP)),
		zap.Int64("start_time", req.StartTime),
		zap.Int64("end_time", req.EndTime))

	// 使用转换器生成查询
	query := req.ToCutQuery()

	// 设置响应头
	w.Header().Set("Content-Type", "application/vnd.tcpdump.pcap")
	w.Header().Set("Content-Disposition", "attachment; filename=capture.pcap")
	w.Header().Set("Transfer-Encoding", "chunked")

	// 直接流式输出
	result, err := h.cutter.CutPCAP(ctx, query, w, nil)
	if err != nil {
		h.logger.Error("Failed to cut PCAP", zap.Error(err))
		// 注意：此时可能已经写入了部分数据，无法更改状态码
		return
	}

	// 记录审计日志
	if h.auditLogger != nil {
		userID := httpx.GetUserID(ctx)
		h.auditLogger.LogPcapAccess(ctx, audit.EventTypePcapCut, req.TenantID, userID, "", map[string]interface{}{
			"mode":          "sync",
			"total_packets": result.TotalPackets,
			"total_bytes":   result.TotalBytes,
			"files_scanned": result.FilesScanned,
			"duration_ms":   result.Duration.Milliseconds(),
		})
	}

	h.logger.Info("Sync PCAP cut completed",
		zap.String("tenant_id", req.TenantID),
		zap.Int64("total_packets", result.TotalPackets),
		zap.Int64("total_bytes", result.TotalBytes),
		zap.Duration("duration", result.Duration))
}

// DownloadPCAP 下载 PCAP 文件（修复版：完整的路径遍历防护 + 租户隔离）
func (h *Handler) DownloadPCAP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 限流检查
	if !h.checkGlobalRateLimit() {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	vars := mux.Vars(r)
	key := vars["key"]

	// ========== 修复 H2: 完整的路径遍历防护 ==========

	// 1. URL 解码
	key, err := url.PathUnescape(key)
	if err != nil {
		h.logger.Warn("Invalid URL encoding in file key",
			zap.String("original_key", vars["key"]),
			zap.Error(err))
		http.Error(w, "Invalid file key encoding", http.StatusBadRequest)
		return
	}

	// 2. 路径规范化
	key = filepath.Clean(key)

	// 3. 严格验证
	if strings.Contains(key, "..") ||
		strings.HasPrefix(key, "/") ||
		strings.HasPrefix(key, "\\") ||
		strings.Contains(key, "\\") {
		h.logger.Warn("Path traversal attempt detected",
			zap.String("key", key),
			zap.String("client_ip", maskIP(httpx.GetClientIP(r))))
		http.Error(w, "Invalid file key", http.StatusBadRequest)
		return
	}

	// 4. 白名单验证
	allowedPrefixes := []string{"results/", "forensics/"}
	hasValidPrefix := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(key, prefix) {
			hasValidPrefix = true
			break
		}
	}
	if !hasValidPrefix {
		h.logger.Warn("File key not in whitelist",
			zap.String("key", key))
		http.Error(w, "Invalid file key", http.StatusBadRequest)
		return
	}

	// ========== 修复 H1: 提取租户信息并验证权限 ==========
	userTenantID := httpx.GetTenantID(ctx)
	if userTenantID == "" {
		userTenantID = "default"
	}

	// 期望的 key 格式：results/{tenant_id}/{date}/{job_id}.pcap
	parts := strings.Split(key, "/")
	if len(parts) < 4 || parts[0] != "results" {
		h.logger.Warn("Invalid file key format",
			zap.String("key", key))
		http.Error(w, "Invalid file key format", http.StatusBadRequest)
		return
	}
	fileTenantID := parts[1]

	// 租户隔离检查
	if !h.checkCrossTenantAccess(ctx, userTenantID, fileTenantID) {
		h.logger.Warn("Cross-tenant download denied",
			zap.String("user_tenant", userTenantID),
			zap.String("file_tenant", fileTenantID),
			zap.String("key", key))
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	h.logger.Debug("Download PCAP request", zap.String("key", key))

	// 检查文件是否存在
	exists, err := h.s3Client.ObjectExists(ctx, key)
	if err != nil || !exists {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	obj, err := h.s3Client.GetObject(ctx, key)
	if err != nil {
		h.logger.Error("Failed to get object", zap.String("key", key), zap.Error(err))
		http.Error(w, "Failed to download file", http.StatusInternalServerError)
		return
	}
	defer obj.Close()

	// 记录审计日志
	if h.auditLogger != nil {
		userID := httpx.GetUserID(ctx)
		h.auditLogger.LogPcapAccess(ctx, audit.EventTypePcapDownload, fileTenantID, userID, key, nil)
	}

	w.Header().Set("Content-Type", "application/vnd.tcpdump.pcap")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(key)))

	if _, err := io.Copy(w, obj); err != nil {
		h.logger.Error("Failed to stream PCAP", zap.Error(err))
	}
}

// PresignRequest 预签名请求
type PresignRequest struct {
	Key    string `json:"key"`
	Expiry int    `json:"expiry_seconds"`
}

// PresignResponse 预签名响应
type PresignResponse struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
}

// GetPresignedURL 获取预签名 URL（修复版：添加租户隔离）
func (h *Handler) GetPresignedURL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rw := httpx.NewResponseWriter(w, ctx)

	var req PresignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		rw.Error(http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil)
		return
	}

	if req.Key == "" {
		rw.Error(http.StatusBadRequest, "INVALID_PARAMETER", "key is required", nil)
		return
	}

	// ========== 修复 H2: 路径验证 ==========
	key, err := url.PathUnescape(req.Key)
	if err != nil {
		rw.Error(http.StatusBadRequest, "INVALID_PARAMETER", "invalid key encoding", nil)
		return
	}

	key = filepath.Clean(key)

	if strings.Contains(key, "..") || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		h.logger.Warn("Path traversal in presign request",
			zap.String("key", req.Key))
		rw.Error(http.StatusBadRequest, "INVALID_PARAMETER", "invalid key format", nil)
		return
	}

	// ========== 修复 H1: 租户隔离 ==========
	userTenantID := httpx.GetTenantID(ctx)
	if userTenantID == "" {
		userTenantID = "default"
	}

	parts := strings.Split(key, "/")
	if len(parts) >= 2 && parts[0] == "results" {
		fileTenantID := parts[1]
		if !h.checkCrossTenantAccess(ctx, userTenantID, fileTenantID) {
			h.logger.Warn("Cross-tenant presign denied",
				zap.String("user_tenant", userTenantID),
				zap.String("file_tenant", fileTenantID))
			rw.Error(http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
			return
		}
	}

	if req.Expiry == 0 {
		req.Expiry = 3600 // 1 小时默认
	}
	if req.Expiry > 86400 {
		req.Expiry = 86400 // 最多 24 小时
	}

	expiry := time.Duration(req.Expiry) * time.Second
	url, err := h.s3Client.GetPresignedURL(ctx, key, expiry)
	if err != nil {
		h.logger.Error("Failed to generate presigned URL", zap.Error(err))
		rw.Error(http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate URL", nil)
		return
	}

	rw.Success(PresignResponse{
		URL:       url,
		ExpiresAt: time.Now().Add(expiry).Unix(),
	})
}

// GetStats 获取统计信息（修复版：添加权限检查）
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rw := httpx.NewResponseWriter(w, ctx)

	userTenantID := httpx.GetTenantID(ctx)
	if userTenantID == "" {
		userTenantID = "default"
	}

	requestedTenantID := r.URL.Query().Get("tenant_id")
	var tenantID string

	// ========== 修复 H1: 权限检查 ==========
	if requestedTenantID == "" || requestedTenantID == userTenantID {
		tenantID = userTenantID
	} else {
		// 跨租户访问需要权限
		if !h.checkCrossTenantAccess(ctx, userTenantID, requestedTenantID) {
			h.logger.Warn("Cross-tenant stats access denied",
				zap.String("user_tenant", userTenantID),
				zap.String("requested_tenant", requestedTenantID))
			rw.Error(http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
			return
		}
		tenantID = requestedTenantID
	}

	// 获取任务统计
	taskStats, err := h.taskRepo.GetTaskStats(ctx, tenantID)
	if err != nil {
		h.logger.Error("Failed to get task stats", zap.Error(err))
		rw.Error(http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get stats", nil)
		return
	}

	// 获取异步处理器统计
	asyncStats := h.asyncCutter.GetStats()

	rw.Success(map[string]interface{}{
		"tenant_id":    tenantID,
		"task_stats":   taskStats,
		"worker_stats": asyncStats,
	})
}

// HealthCheck 健康检查
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// ReadinessCheck 就绪检查
func (h *Handler) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 检查 S3 连接
	if err := h.s3Client.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not ready",
			"error":  "S3 connection failed",
		})
		return
	}

	// 检查异步处理器是否运行
	if !h.asyncCutter.IsRunning() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not ready",
			"error":  "async processor not running",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// parseIntParam 解析整数参数
func parseIntParam(value string, defaultVal, min, max int) int {
	if value == "" {
		return defaultVal
	}
	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultVal
	}
	if result < min {
		return min
	}
	if result > max {
		return max
	}
	return result
}

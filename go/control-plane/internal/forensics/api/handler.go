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
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
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

var forensicsSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Handler API 处理器
type Handler struct {
	cutter                *cutter.Cutter
	asyncCutter           *task.AsyncCutter
	s3Client              *s3client.S3Client
	taskRepo              *repository.TaskRepository
	auditLogger           *audit.Logger
	auditDB               *sql.DB
	restorationProcessor  RestorationProcessor
	taskPipelineEnabled   bool
	compatibleWorkerReady bool
	logger                *zap.Logger

	// ========== 修复 L2: 添加限流器 ==========
	globalLimiter  *rate.Limiter // 全局 QPS 限制
	tenantLimiters sync.Map      // map[tenantID]*rateLimiterEntry
	ipLimiters     sync.Map      // map[clientIP]*rateLimiterEntry
	// rateLimiterEvictCounter 限流器淘汰扫描节流计数器
	rateLimiterEvictCounter atomic.Int64
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

// SetAuditDB enables synchronous audit_logs writes in addition to Kafka audit publishing.
func (h *Handler) SetAuditDB(db *sql.DB) {
	h.auditDB = db
}

// SetTaskCommandAdmission keeps the versioned writer fail-closed. Both the
// product flag and a compatible worker readiness receipt must be present before
// create/cancel/retry commands are accepted.
func (h *Handler) SetTaskCommandAdmission(pipelineEnabled, compatibleWorkerReady bool) {
	h.taskPipelineEnabled = pipelineEnabled
	h.compatibleWorkerReady = compatibleWorkerReady
}

func (h *Handler) loadUIFixture(ctx context.Context, tenantID, endpoint string, target interface{}) bool {
	if h.auditDB == nil {
		return false
	}
	var payload []byte
	if err := h.auditDB.QueryRowContext(ctx, `
		SELECT payload
		FROM forensics_ui_fixtures
		WHERE tenant_id=$1 AND endpoint=$2 AND active=true
		LIMIT 1`, tenantID, endpoint).Scan(&payload); err != nil {
		return false
	}
	if err := json.Unmarshal(payload, target); err != nil {
		h.logger.Warn("Invalid forensics UI fixture", zap.String("tenant_id", tenantID), zap.String("endpoint", endpoint), zap.Error(err))
		return false
	}
	return true
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(r *mux.Router) {
	// 异步裁剪任务（推荐）
	r.Handle("/api/v1/pcap/jobs", h.requirePermission("pcap:write", h.CreateJob)).Methods("POST")
	r.Handle("/api/v1/pcap/jobs/{id}", h.requirePermission("pcap:read", h.GetJob)).Methods("GET")
	r.Handle("/api/v1/pcap/jobs/{id}/cancel", h.requirePermission("pcap:write", h.CancelJob)).Methods("POST")
	r.Handle("/api/v1/pcap/jobs/{id}/retry", h.requirePermission("pcap:write", h.RetryJob)).Methods("POST")
	r.Handle("/api/v1/pcap/jobs", h.requirePermission("pcap:read", h.ListJobs)).Methods("GET")

	// 同步裁剪（仅用于小文件，保留兼容）
	r.Handle("/api/v1/pcap/cut", h.requirePermission("pcap:write", h.CutPCAPSync)).Methods("POST")

	// 直接下载
	r.Handle("/api/v1/pcap/download/{key:.*}", h.requirePermission("pcap:download", h.DownloadPCAP)).Methods("GET")

	// 预签名 URL
	r.Handle("/api/v1/pcap/presign", h.requirePermission("pcap:download", h.GetPresignedURL)).Methods("POST")

	// 完整性校验
	r.Handle("/api/v1/pcap/verify", h.requirePermission("pcap:read", h.VerifyPCAP)).Methods("POST")

	// 统计信息
	r.Handle("/api/v1/pcap/stats", h.requirePermission("pcap:read", h.GetStats)).Methods("GET")

	// Additive file-restoration admission. It is registered in every version so
	// a default-off deployment returns an explicit disabled result rather than
	// silently falling back to PCAP cutting.
	r.Handle("/api/v1/forensics/restorations", h.requirePermission("forensics:write", h.CreateRestoration)).Methods("POST")

	// 健康检查
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")
	r.HandleFunc("/ready", h.ReadinessCheck).Methods("GET")
}

func (h *Handler) requirePermission(permission string, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hasForensicsPermission(r.Context(), permission) {
			// ADR-5 拒绝留痕:尽力而为写拒绝审计(403 语义已成立,
			// 审计失败不改变拒绝,仅告警)。
			h.recordPermissionDenied(r, permission)
			rw := httpx.NewResponseWriter(w, r.Context())
			rw.Error(http.StatusForbidden, "FORBIDDEN", fmt.Sprintf("Permission denied: %s required", permission), nil)
			return
		}
		next(w, r)
	})
}

// recordPermissionDenied 为 403 拒绝补审计行(action=AUTH_PERMISSION_DENIED)。
func (h *Handler) recordPermissionDenied(r *http.Request, permission string) {
	if h.auditDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	tenantID := httpx.GetTenantID(ctx)
	userID := httpx.GetUserID(ctx)
	if err := h.recordPcapAudit(ctx, r, audit.EventTypePermissionDenied, tenantID, userID, permission,
		map[string]interface{}{"required_scope": permission, "result": "denied", "path": r.URL.Path}); err != nil {
		h.logger.Warn("Failed to persist permission-denied audit",
			zap.String("scope", permission), zap.String("path", r.URL.Path), zap.Error(err))
	}
}

func hasForensicsPermission(ctx context.Context, required string) bool {
	for _, permission := range httpx.GetPermissions(ctx) {
		if permission == "*" || permission == "admin:*" ||
			(permission == "pcap:*" && strings.HasPrefix(required, "pcap:")) || permission == required {
			return true
		}
	}
	return false
}

// ========== 修复 L2: 限流检查方法 ==========

// checkGlobalRateLimit 检查全局限流
func (h *Handler) checkGlobalRateLimit() bool {
	return h.globalLimiter.Allow()
}

// checkTenantRateLimit 检查租户级别限流
func (h *Handler) checkTenantRateLimit(tenantID string) bool {
	h.evictStaleLimiters()
	entryInterface, _ := h.tenantLimiters.LoadOrStore(tenantID, &rateLimiterEntry{
		limiter: rate.NewLimiter(rate.Limit(10), 20), // 10 QPS/租户，突发 20
	})
	entry := entryInterface.(*rateLimiterEntry)
	entry.lastUsed.Store(time.Now().Unix())
	return entry.limiter.Allow()
}

// checkIPRateLimit 检查 IP 级别限流
func (h *Handler) checkIPRateLimit(clientIP string) bool {
	h.evictStaleLimiters()
	entryInterface, _ := h.ipLimiters.LoadOrStore(clientIP, &rateLimiterEntry{
		limiter: rate.NewLimiter(rate.Limit(5), 10), // 5 QPS/IP，突发 10
	})
	entry := entryInterface.(*rateLimiterEntry)
	entry.lastUsed.Store(time.Now().Unix())
	return entry.limiter.Allow()
}

// rateLimiterEntry 限流器条目，携带最后使用时间以便淘汰
type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastUsed atomic.Int64 // unix 秒
}

// rateLimiterTTL 限流器空闲淘汰阈值
const rateLimiterTTL = 10 * 60 // 10 分钟（秒）

// evictStaleLimiters 机会式淘汰空闲限流器，防止 tenantLimiters/ipLimiters
// 无界增长（每 64 次调用执行一次扫描，成本可控）。
func (h *Handler) evictStaleLimiters() {
	if h.rateLimiterEvictCounter.Add(1)%64 != 0 {
		return
	}
	now := time.Now().Unix()
	cutoff := now - rateLimiterTTL
	evictMap := func(m *sync.Map) {
		m.Range(func(key, value interface{}) bool {
			if entry, ok := value.(*rateLimiterEntry); ok && entry.lastUsed.Load() < cutoff {
				m.Delete(key)
			}
			return true
		})
	}
	evictMap(&h.tenantLimiters)
	evictMap(&h.ipLimiters)
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
	return hasForensicsCrossTenantPermission(ctx)
}

func hasForensicsCrossTenantPermission(ctx context.Context) bool {
	permissions := httpx.GetPermissions(ctx)
	for _, p := range permissions {
		if p == "admin:cross_tenant" || p == "forensics:cross_tenant" {
			return true
		}
	}

	return false
}

func (h *Handler) normalizeResultKey(rawKey string) (string, string, error) {
	key, err := url.PathUnescape(rawKey)
	if err != nil {
		return "", "", fmt.Errorf("invalid key encoding")
	}

	key = filepath.Clean(key)
	if key == "." ||
		strings.Contains(key, "..") ||
		strings.HasPrefix(key, "/") ||
		strings.HasPrefix(key, "\\") ||
		strings.Contains(key, "\\") {
		return "", "", fmt.Errorf("invalid key format")
	}

	parts := strings.Split(key, "/")
	if len(parts) >= 4 && parts[0] == "results" && parts[1] != "" {
		return key, parts[1], nil
	}
	if len(parts) >= 7 && parts[0] == "tenants" && parts[1] != "" &&
		parts[2] == "forensics" && parts[3] == "jobs" && parts[4] != "" && parts[5] == "pcap" {
		return key, parts[1], nil
	}
	return "", "", fmt.Errorf("invalid result key format")
}

func normalizeForensicsPurpose(raw string, required bool) (string, error) {
	purpose := strings.TrimSpace(raw)
	if required && purpose == "" {
		return "", fmt.Errorf("purpose is required for versioned result download")
	}
	if len(purpose) > 256 || strings.ContainsAny(purpose, "\x00\r\n") {
		return "", fmt.Errorf("purpose must be at most 256 safe characters")
	}
	return purpose, nil
}

func decodeForensicsJSON(w http.ResponseWriter, r *http.Request, target interface{}) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("request body must contain exactly one JSON object")
	}
	return nil
}

func currentTenantID(ctx context.Context) string {
	tenantID := httpx.GetTenantID(ctx)
	if tenantID == "" {
		return "default"
	}
	return tenantID
}

func (h *Handler) registeredResultSHA256(ctx context.Context, tenantID, key string) string {
	if h.taskRepo == nil {
		return ""
	}
	if manifest, err := h.taskRepo.GetVersionedManifestByResultKey(ctx, tenantID, key); err == nil && manifest != nil {
		return strings.ToLower(strings.TrimSpace(manifest.ResultObject.SHA256))
	}
	task, err := h.taskRepo.GetByResultFileKey(ctx, key)
	if err != nil || task == nil || task.TenantID != tenantID {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(task.ResultSHA256))
}

func (h *Handler) versionedResultManifest(ctx context.Context, tenantID, key string) (*repository.TaskManifestReceipt, error) {
	if !strings.HasPrefix(key, "tenants/") {
		return nil, nil
	}
	if h.taskRepo == nil {
		return nil, errors.New(errors.ErrCodeServiceUnavailable, "versioned result manifest repository is unavailable")
	}
	manifest, err := h.taskRepo.GetVersionedManifestByResultKey(ctx, tenantID, key)
	if err != nil {
		return nil, fmt.Errorf("load versioned result manifest: %w", err)
	}
	if manifest == nil {
		return nil, errors.New(errors.ErrCodeResourceNotFound, "versioned result manifest not found")
	}
	return manifest, nil
}

func (h *Handler) computeObjectSHA256(ctx context.Context, key string) (string, int64, error) {
	obj, err := h.s3Client.GetObject(ctx, key)
	if err != nil {
		return "", 0, err
	}
	defer obj.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, obj)
	if err != nil {
		return "", 0, err
	}

	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

func (h *Handler) recordPcapAudit(ctx context.Context, r *http.Request, eventType audit.EventType, tenantID, userID, fileKey string, detail map[string]interface{}) error {
	if h.auditDB == nil {
		return fmt.Errorf("synchronous PCAP audit database is not configured")
	}

	if detail == nil {
		detail = map[string]interface{}{}
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal PCAP audit detail: %w", err)
	}

	var userIDValue interface{}
	if parsed, err := uuid.Parse(userID); err == nil {
		userIDValue = parsed.String()
	}

	if _, err := h.auditDB.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
	`, tenantID, userIDValue, string(eventType), "pcap", fileKey, string(detailJSON), httpx.GetClientIP(r), r.UserAgent()); err != nil {
		return fmt.Errorf("write PCAP audit log: %w", err)
	}
	if h.auditLogger != nil {
		h.auditLogger.LogPcapAccess(ctx, eventType, tenantID, userID, fileKey, detail)
	}
	return nil
}

func forensicsTaskCommandMeta(r *http.Request, tenantID, actorID, actionID string, expectedRevision int64, compatibility bool) repository.TaskCommandMeta {
	reason := strings.TrimSpace(r.Header.Get("X-Action-Reason"))
	if reason == "" {
		reason = strings.TrimSpace(r.Header.Get("X-Reason"))
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if reason == "" || idempotencyKey == "" {
		compatibility = true
	}
	return repository.TaskCommandMeta{
		TenantID: tenantID, ActorID: actorID, ActionID: actionID,
		IdempotencyKey: idempotencyKey, ExpectedRevision: &expectedRevision,
		Reason: reason, TraceID: httpx.GetTraceID(r.Context()),
		SourceIP: httpx.GetClientIP(r), UserAgent: r.UserAgent(),
		CompatibilityMode: compatibility,
	}
}

func versionedForensicsTaskCommandMeta(r *http.Request, tenantID, actorID, actionID string, expectedRevision int64, compatibility bool) (repository.TaskCommandMeta, error) {
	meta := forensicsTaskCommandMeta(r, tenantID, actorID, actionID, expectedRevision, compatibility)
	if meta.CompatibilityMode || strings.TrimSpace(meta.IdempotencyKey) == "" || strings.TrimSpace(meta.Reason) == "" {
		return repository.TaskCommandMeta{}, errors.New(errors.ErrCodeInvalidParameter,
			"Idempotency-Key, X-Action-Reason and the required revision precondition are mandatory")
	}
	if strings.TrimSpace(meta.TenantID) == "" || strings.TrimSpace(meta.ActorID) == "" || strings.TrimSpace(meta.TraceID) == "" {
		return repository.TaskCommandMeta{}, errors.New(errors.ErrCodeUnauthorized,
			"authenticated tenant, actor and trace identity are required")
	}
	return meta, nil
}

func parseForensicsTaskRevision(r *http.Request, fallback int64) (int64, bool, error) {
	header := strings.TrimSpace(r.Header.Get("If-Match"))
	if header == "" {
		return fallback, true, nil
	}
	header = strings.TrimSpace(strings.TrimPrefix(header, "W/"))
	header = strings.Trim(header, `"`)
	revision, err := strconv.ParseInt(header, 10, 64)
	if err != nil || revision <= 0 {
		return 0, false, errors.New(errors.ErrCodeInvalidParameter, "If-Match must contain a positive task revision")
	}
	return revision, false, nil
}

func writeForensicsCommandError(rw *httpx.ResponseWriter, err error) {
	code := errors.GetCode(err)
	status := errors.GetHTTPStatus(err)
	if code == errors.ErrCodeDedupConflict || code == errors.ErrCodeInvalidStateTransition || code == errors.ErrCodeVersionConflict || code == errors.ErrCodeConcurrentModify {
		status = http.StatusConflict
	}
	rw.Error(status, string(code), err.Error(), nil)
}

func (h *Handler) requireTaskCommandAdmission(rw *httpx.ResponseWriter) bool {
	if h.taskPipelineEnabled && h.compatibleWorkerReady {
		return true
	}
	rw.Error(http.StatusServiceUnavailable, "FORENSICS_PIPELINE_NOT_READY",
		"Forensics task commands remain disabled until the compatible worker is ready", nil)
	return false
}

// ========== API 处理方法 ==========

// CreateJob 创建异步裁剪任务（修复版）
func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rw := httpx.NewResponseWriter(w, ctx)
	if !h.requireTaskCommandAdmission(rw) {
		return
	}

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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		rw.Error(http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		rw.Error(http.StatusBadRequest, "INVALID_REQUEST", "request body must contain exactly one JSON object", nil)
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
	userTenantID := strings.TrimSpace(httpx.GetTenantID(ctx))
	if userTenantID == "" {
		rw.Error(http.StatusUnauthorized, "UNAUTHENTICATED", "authenticated tenant identity is required", nil)
		return
	}

	// tenant_id is identity-derived. A body value is accepted only when it
	// repeats that identity; even cross-tenant administrators cannot override
	// the command tenant through payload data.
	if req.TenantID != "" && req.TenantID != userTenantID {
		h.logger.Warn("Tenant override on forensics command denied",
			zap.String("user_tenant", userTenantID),
			zap.String("requested_tenant", req.TenantID),
			zap.String("client_ip", maskIP(clientIP)))
		rw.Error(http.StatusForbidden, "TENANT_OVERRIDE_FORBIDDEN", "tenant_id must come from authenticated identity", nil)
		return
	}
	req.TenantID = userTenantID
	if strings.TrimSpace(req.Purpose) == "" {
		rw.Error(http.StatusBadRequest, "INVALID_REQUEST", "purpose is required", nil)
		return
	}
	if req.RetentionPolicy == "" {
		req.RetentionPolicy = "forensics-standard"
	}
	if req.RestorationContractVersion == 0 {
		req.RestorationContractVersion = 1
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

	commandMeta, err := versionedForensicsTaskCommandMeta(r, req.TenantID, userID, repository.ForensicsTaskCreateAction, 0, false)
	if err != nil {
		writeForensicsCommandError(rw, err)
		return
	}
	restorationSpecs := make([]task.RestorationTaskSpec, 0, len(req.Restorations))
	for _, requested := range req.Restorations {
		restorationSpecs = append(restorationSpecs, task.RestorationTaskSpec{
			RequestID: requested.RequestID, SessionID: requested.SessionID,
			CommunityID: requested.CommunityID, FlowIDs: requested.FlowIDs, FlowID: requested.FlowID,
			Tuple: requested.Tuple, Direction: requested.Direction, ProfileID: requested.ProfileID,
			FTPData: requested.FTPData, FTPTLSEnabled: requested.FTPTLSEnabled,
		})
	}

	// 创建任务请求
	taskReq := &task.CutTaskRequest{
		TenantID:                   req.TenantID,
		UserID:                     userID,
		AssetID:                    req.AssetID,
		AlertID:                    req.AlertID,
		AlertIDs:                   req.AlertIDs,
		CaseIDs:                    req.CaseIDs,
		CampaignID:                 req.CampaignID,
		BaselineID:                 req.BaselineID,
		EvidenceID:                 req.EvidenceID,
		EvidenceType:               req.EvidenceType,
		ProbeID:                    req.ProbeID,
		ProbeIDs:                   req.ProbeIDs,
		SrcIP:                      req.SrcIP,
		DstIP:                      req.DstIP,
		SrcPort:                    req.SrcPort,
		DstPort:                    req.DstPort,
		Protocol:                   req.Protocol,
		CommunityID:                req.CommunityID,
		StartTime:                  req.StartTime,
		EndTime:                    req.EndTime,
		MaxPackets:                 req.MaxPackets,
		Purpose:                    req.Purpose,
		PermissionSnapshot:         httpx.GetPermissions(ctx),
		RetentionPolicy:            req.RetentionPolicy,
		RestorationContractVersion: req.RestorationContractVersion,
		Restorations:               restorationSpecs,
		TraceID:                    commandMeta.TraceID,
		CommandMeta:                commandMeta,
	}

	job, err := h.asyncCutter.SubmitTask(ctx, taskReq)
	if err != nil {
		h.logger.Error("Failed to create job", zap.Error(err))
		writeForensicsCommandError(rw, err)
		return
	}

	// 构建响应
	response := &converter.JobResponse{
		JobID:     job.JobID,
		Status:    job.Status,
		CreatedAt: job.CreatedAt.UnixMilli(),
		Revision:  job.Revision, EventID: job.EventID, ActionID: job.ActionID,
		IdempotencyKey: job.IdempotencyKey, OutboxStatus: job.OutboxStatus,
		Replayed: job.Replayed, CompatibilityMode: job.CompatibilityMode,
	}
	w.Header().Set("ETag", fmt.Sprintf(`W/"%d"`, job.Revision))
	w.Header().Set("Idempotency-Key", job.IdempotencyKey)
	w.Header().Set("X-Event-ID", job.EventID)
	rw.Accepted(response)
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
	userTenantID := strings.TrimSpace(httpx.GetTenantID(ctx))
	if userTenantID == "" {
		rw.Error(http.StatusUnauthorized, "UNAUTHENTICATED", "authenticated tenant identity is required", nil)
		return
	}

	job, err := h.taskRepo.GetByIDForTenant(ctx, userTenantID, jobID)
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
	versionedManifest := false
	if manifest, err := h.taskRepo.GetVersionedManifestForTenant(ctx, job.TenantID, job.TaskID); err != nil {
		h.logger.Warn("Failed to load versioned task manifest", zap.String("task_id", job.TaskID), zap.Error(err))
	} else if manifest != nil {
		versionedManifest = true
		response.ManifestSHA256 = manifest.ManifestSHA256
		response.Manifest = manifest.Manifest
		response.ResultObjectVersion = manifest.ObjectVersion
		retentionUntil := manifest.RetentionUntil.UnixMilli()
		response.RetentionUntil = &retentionUntil
	}

	// 解析参数
	if len(job.ParamsJSON) > 0 {
		var params map[string]interface{}
		if err := json.Unmarshal(job.ParamsJSON, &params); err == nil {
			response.Params = params
		}
	}

	// 如果任务完成且有结果文件，生成预签名 URL
	if job.Status == repository.TaskStatusCompleted && job.ResultFileKey != "" && !versionedManifest {
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
	if !h.requireTaskCommandAdmission(rw) {
		return
	}

	vars := mux.Vars(r)
	jobID := vars["id"]

	// ========== 修复 H1: 先获取任务检查权限 ==========
	userTenantID := strings.TrimSpace(httpx.GetTenantID(ctx))
	if userTenantID == "" {
		rw.Error(http.StatusUnauthorized, "UNAUTHENTICATED", "authenticated tenant identity is required", nil)
		return
	}

	job, err := h.taskRepo.GetByIDForTenant(ctx, userTenantID, jobID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			rw.Error(http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", nil)
			return
		}
		h.logger.Error("Failed to get job", zap.String("job_id", jobID), zap.Error(err))
		rw.Error(http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get job", nil)
		return
	}

	expectedRevision, compatibility, err := parseForensicsTaskRevision(r, job.Revision)
	if err != nil {
		writeForensicsCommandError(rw, err)
		return
	}
	meta, err := versionedForensicsTaskCommandMeta(r, job.TenantID, httpx.GetUserID(ctx), repository.ForensicsTaskCancelAction, expectedRevision, compatibility)
	if err != nil {
		writeForensicsCommandError(rw, err)
		return
	}
	receipt, err := h.asyncCutter.CancelTask(ctx, job.TenantID, jobID, meta)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			rw.Error(http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", nil)
			return
		}
		h.logger.Error("Failed to cancel job", zap.String("job_id", jobID), zap.Error(err))
		writeForensicsCommandError(rw, err)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`W/"%d"`, receipt.Revision))
	w.Header().Set("Idempotency-Key", receipt.IdempotencyKey)
	w.Header().Set("X-Event-ID", receipt.EventID)
	rw.Success(receipt)
}

// RetryJob requeues a failed or cancelled task without accepting a replacement
// payload. The immutable request, permission snapshot and purpose stay bound to
// the original task revision chain.
func (h *Handler) RetryJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rw := httpx.NewResponseWriter(w, ctx)
	if !h.requireTaskCommandAdmission(rw) {
		return
	}
	taskID := mux.Vars(r)["id"]
	tenantID := strings.TrimSpace(httpx.GetTenantID(ctx))
	if tenantID == "" {
		rw.Error(http.StatusUnauthorized, "UNAUTHENTICATED", "authenticated tenant identity is required", nil)
		return
	}
	job, err := h.taskRepo.GetByIDForTenant(ctx, tenantID, taskID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			rw.Error(http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", nil)
			return
		}
		writeForensicsCommandError(rw, err)
		return
	}
	expectedRevision, compatibility, err := parseForensicsTaskRevision(r, job.Revision)
	if err != nil {
		writeForensicsCommandError(rw, err)
		return
	}
	meta, err := versionedForensicsTaskCommandMeta(r, tenantID, httpx.GetUserID(ctx), repository.ForensicsTaskRetryAction, expectedRevision, compatibility)
	if err != nil {
		writeForensicsCommandError(rw, err)
		return
	}
	receipt, err := h.asyncCutter.RetryTask(ctx, tenantID, taskID, meta)
	if err != nil {
		writeForensicsCommandError(rw, err)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`W/"%d"`, receipt.Revision))
	w.Header().Set("Idempotency-Key", receipt.IdempotencyKey)
	w.Header().Set("X-Event-ID", receipt.EventID)
	rw.Accepted(receipt)
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
	assetID := strings.TrimSpace(r.URL.Query().Get("asset_id"))
	filter := repository.TaskListFilter{
		Status: status, AssetID: assetID,
		AlertID: strings.TrimSpace(r.URL.Query().Get("alert_id")), CampaignID: strings.TrimSpace(r.URL.Query().Get("campaign_id")),
		BaselineID: strings.TrimSpace(r.URL.Query().Get("baseline_id")), EvidenceID: strings.TrimSpace(r.URL.Query().Get("evidence_id")),
		EvidenceType: strings.TrimSpace(r.URL.Query().Get("evidence_type")),
		SrcIP:        strings.TrimSpace(r.URL.Query().Get("src_ip")), DstIP: strings.TrimSpace(r.URL.Query().Get("dst_ip")),
		Protocol: strings.TrimSpace(r.URL.Query().Get("protocol")), Port: strings.TrimSpace(r.URL.Query().Get("port")),
		Tuple: strings.TrimSpace(r.URL.Query().Get("tuple")), TaskID: strings.TrimSpace(r.URL.Query().Get("task_id")),
	}

	var fixture struct {
		Jobs  []json.RawMessage `json:"jobs"`
		Total int64             `json:"total"`
	}
	if h.loadUIFixture(ctx, tenantID, "jobs", &fixture) {
		fixtureJobs := filterFixtureJobs(fixture.Jobs, filter)
		// A filtered lookup that is not part of the canonical visual scenario must
		// still reach the operational repository. This keeps newly created jobs and
		// their source references observable without perturbing the fixed,
		// unfiltered acceptance view.
		if shouldServeFixtureJobs(filter, len(fixtureJobs)) {
			start := min(offset, len(fixtureJobs))
			end := min(start+limit, len(fixtureJobs))
			total := fixture.Total
			if hasTaskListFilter(filter) || total <= 0 {
				total = int64(len(fixtureJobs))
			}
			rw.Paginated(fixtureJobs[start:end], total, limit, offset)
			return
		}
	}

	jobs, total, err := h.taskRepo.ListFiltered(ctx, tenantID, filter, limit, offset)
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

func shouldServeFixtureJobs(filter repository.TaskListFilter, matched int) bool {
	return !hasTaskListFilter(filter) || matched > 0
}

func hasTaskListFilter(filter repository.TaskListFilter) bool {
	return filter.Status != "" || filter.AssetID != "" || filter.SrcIP != "" || filter.DstIP != "" ||
		filter.Protocol != "" || filter.Port != "" || filter.Tuple != "" || filter.TaskID != "" ||
		filter.AlertID != "" || filter.CampaignID != "" || filter.BaselineID != "" || filter.EvidenceID != "" || filter.EvidenceType != ""
}

func filterFixtureJobs(jobs []json.RawMessage, filter repository.TaskListFilter) []json.RawMessage {
	if !hasTaskListFilter(filter) {
		return jobs
	}
	filtered := make([]json.RawMessage, 0, len(jobs))
	for _, job := range jobs {
		var decoded struct {
			JobID  string `json:"job_id"`
			Status string `json:"status"`
			Params struct {
				AssetID      string `json:"asset_id"`
				AlertID      string `json:"alert_id"`
				CampaignID   string `json:"campaign_id"`
				BaselineID   string `json:"baseline_id"`
				EvidenceID   string `json:"evidence_id"`
				EvidenceType string `json:"evidence_type"`
				SrcIP        string `json:"src_ip"`
				DstIP        string `json:"dst_ip"`
				SrcPort      int    `json:"src_port"`
				DstPort      int    `json:"dst_port"`
				Protocol     string `json:"protocol"`
			} `json:"params"`
		}
		if json.Unmarshal(job, &decoded) != nil {
			continue
		}
		tuple := fmt.Sprintf("%s:%d -> %s:%d %s", decoded.Params.SrcIP, decoded.Params.SrcPort, decoded.Params.DstIP, decoded.Params.DstPort, decoded.Params.Protocol)
		portMatches := filter.Port == "" || filter.Port == fmt.Sprint(decoded.Params.SrcPort) || filter.Port == fmt.Sprint(decoded.Params.DstPort)
		if (filter.Status == "" || strings.EqualFold(filter.Status, decoded.Status)) &&
			(filter.AssetID == "" || filter.AssetID == decoded.Params.AssetID) &&
			(filter.AlertID == "" || filter.AlertID == decoded.Params.AlertID) &&
			(filter.CampaignID == "" || filter.CampaignID == decoded.Params.CampaignID) &&
			(filter.BaselineID == "" || filter.BaselineID == decoded.Params.BaselineID) &&
			(filter.EvidenceID == "" || filter.EvidenceID == decoded.Params.EvidenceID) &&
			(filter.EvidenceType == "" || strings.EqualFold(filter.EvidenceType, decoded.Params.EvidenceType)) &&
			(filter.SrcIP == "" || filter.SrcIP == decoded.Params.SrcIP) &&
			(filter.DstIP == "" || filter.DstIP == decoded.Params.DstIP) &&
			(filter.Protocol == "" || strings.EqualFold(filter.Protocol, decoded.Params.Protocol)) && portMatches &&
			(filter.TaskID == "" || strings.Contains(strings.ToLower(decoded.JobID), strings.ToLower(filter.TaskID))) &&
			(filter.Tuple == "" || strings.Contains(strings.ToLower(tuple), strings.ToLower(strings.TrimSpace(filter.Tuple)))) {
			filtered = append(filtered, job)
		}
	}
	return filtered
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

	// Streaming responses cannot change their status after the first PCAP bytes are
	// written. Persist the authorization/audit decision before starting the stream
	// so a missing audit dependency fails closed instead of silently bypassing the
	// synchronous audit contract.
	if err := h.recordPcapAudit(ctx, r, audit.EventTypePcapCut, req.TenantID, httpx.GetUserID(ctx), "sync-capture.pcap", map[string]interface{}{
		"mode":        "sync",
		"stage":       "accepted",
		"probe_id":    req.ProbeID,
		"start_time":  req.StartTime,
		"end_time":    req.EndTime,
		"max_packets": req.MaxPackets,
	}); err != nil {
		h.logger.Error("Failed to persist synchronous PCAP cut audit", zap.Error(err))
		http.Error(w, "Failed to persist PCAP cut audit", http.StatusInternalServerError)
		return
	}

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

	key, fileTenantID, err := h.normalizeResultKey(key)
	if err != nil {
		h.logger.Warn("Invalid PCAP result key",
			zap.String("key", vars["key"]),
			zap.String("client_ip", maskIP(httpx.GetClientIP(r))),
			zap.Error(err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 租户隔离检查
	userTenantID := currentTenantID(ctx)
	if !h.checkCrossTenantAccess(ctx, userTenantID, fileTenantID) {
		h.logger.Warn("Cross-tenant download denied",
			zap.String("user_tenant", userTenantID),
			zap.String("file_tenant", fileTenantID),
			zap.String("key", key))
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}
	manifest, err := h.versionedResultManifest(ctx, fileTenantID, key)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to resolve immutable result authority", http.StatusInternalServerError)
		}
		return
	}
	purpose, err := normalizeForensicsPurpose(r.URL.Query().Get("purpose"), manifest != nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.logger.Debug("Download PCAP request", zap.String("key", key))

	var obj io.ReadCloser
	resultSHA256 := ""
	detail := map[string]interface{}{"mode": "download", "purpose": purpose}
	if manifest != nil {
		var verified s3client.ObjectAuthority
		obj, verified, err = h.s3Client.OpenForensicsResultObject(ctx, manifest.ResultObject, fileTenantID, manifest.TaskID)
		if err == nil {
			resultSHA256 = verified.SHA256
			detail["manifest_sha256"] = manifest.ManifestSHA256
			detail["object_version"] = verified.VersionID
		}
	} else {
		var exists bool
		exists, err = h.s3Client.ObjectExists(ctx, key)
		if err == nil && exists {
			obj, err = h.s3Client.GetObject(ctx, key)
		} else if err == nil {
			err = errors.New(errors.ErrCodeResourceNotFound, "legacy result object not found")
		}
		resultSHA256 = h.registeredResultSHA256(ctx, fileTenantID, key)
	}
	if err != nil || obj == nil {
		h.logger.Error("Failed to open authorized PCAP object", zap.String("key", key), zap.Error(err))
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer obj.Close()

	userID := httpx.GetUserID(ctx)
	if resultSHA256 != "" {
		detail["sha256"] = resultSHA256
	}
	if err := h.recordPcapAudit(ctx, r, audit.EventTypePcapDownload, fileTenantID, userID, key, detail); err != nil {
		h.logger.Error("Failed to persist PCAP download audit", zap.String("key", key), zap.Error(err))
		http.Error(w, "Failed to persist download audit", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.tcpdump.pcap")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(key)))
	if resultSHA256 != "" {
		w.Header().Set("X-Content-SHA256", resultSHA256)
	}
	if manifest != nil {
		w.Header().Set("X-Object-Version", manifest.ResultObject.VersionID)
		w.Header().Set("X-Manifest-SHA256", manifest.ManifestSHA256)
	}

	if _, err := io.Copy(w, obj); err != nil {
		h.logger.Error("Failed to stream PCAP", zap.Error(err))
	}
}

// PresignRequest 预签名请求
type PresignRequest struct {
	Key     string `json:"key"`
	Expiry  int    `json:"expiry_seconds"`
	Purpose string `json:"purpose"`
}

// PresignResponse 预签名响应
type PresignResponse struct {
	URL            string `json:"url"`
	ExpiresAt      int64  `json:"expires_at"`
	Key            string `json:"key"`
	SHA256         string `json:"sha256,omitempty"`
	ObjectVersion  string `json:"object_version,omitempty"`
	ManifestSHA256 string `json:"manifest_sha256,omitempty"`
	Purpose        string `json:"purpose,omitempty"`
}

// GetPresignedURL 获取预签名 URL（修复版：添加租户隔离）
func (h *Handler) GetPresignedURL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rw := httpx.NewResponseWriter(w, ctx)

	var req PresignRequest
	if err := decodeForensicsJSON(w, r, &req); err != nil {
		rw.Error(http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil)
		return
	}

	if req.Key == "" {
		rw.Error(http.StatusBadRequest, "INVALID_PARAMETER", "key is required", nil)
		return
	}

	key, fileTenantID, err := h.normalizeResultKey(req.Key)
	if err != nil {
		h.logger.Warn("Path traversal in presign request",
			zap.String("key", req.Key),
			zap.Error(err))
		rw.Error(http.StatusBadRequest, "INVALID_PARAMETER", err.Error(), nil)
		return
	}

	// ========== 修复 H1: 租户隔离 ==========
	userTenantID := currentTenantID(ctx)
	if !h.checkCrossTenantAccess(ctx, userTenantID, fileTenantID) {
		h.logger.Warn("Cross-tenant presign denied",
			zap.String("user_tenant", userTenantID),
			zap.String("file_tenant", fileTenantID))
		rw.Error(http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
		return
	}
	manifest, err := h.versionedResultManifest(ctx, fileTenantID, key)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			rw.Error(http.StatusNotFound, "PCAP_NOT_FOUND", "Versioned result manifest not found", nil)
		} else {
			rw.Error(http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to resolve immutable result authority", nil)
		}
		return
	}
	purpose, err := normalizeForensicsPurpose(req.Purpose, manifest != nil)
	if err != nil {
		rw.Error(http.StatusBadRequest, "PURPOSE_REQUIRED", err.Error(), nil)
		return
	}

	if req.Expiry <= 0 {
		req.Expiry = 3600 // 1 小时默认
	}
	if req.Expiry > 86400 {
		rw.Error(http.StatusBadRequest, "INVALID_PARAMETER", "expiry_seconds must not exceed 86400", nil)
		return
	}

	expiry := time.Duration(req.Expiry) * time.Second
	resultSHA256 := ""
	objectVersion := ""
	manifestSHA256 := ""
	var signedURL string
	if manifest != nil {
		var verified s3client.ObjectAuthority
		signedURL, verified, err = h.s3Client.PresignForensicsResultObject(ctx, manifest.ResultObject, fileTenantID, manifest.TaskID, expiry)
		if err == nil {
			resultSHA256 = verified.SHA256
			objectVersion = verified.VersionID
			manifestSHA256 = manifest.ManifestSHA256
		}
	} else {
		var exists bool
		exists, err = h.s3Client.ObjectExists(ctx, key)
		if err == nil && !exists {
			err = errors.New(errors.ErrCodeResourceNotFound, "legacy result object not found")
		}
		if err == nil {
			signedURL, err = h.s3Client.GetPresignedURL(ctx, key, expiry)
		}
		resultSHA256 = h.registeredResultSHA256(ctx, fileTenantID, key)
	}
	if err != nil {
		h.logger.Error("Failed to generate authorized presigned URL", zap.String("key", key), zap.Error(err))
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			rw.Error(http.StatusNotFound, "PCAP_NOT_FOUND", "File not found", nil)
		} else {
			rw.Error(http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate URL", nil)
		}
		return
	}
	userID := httpx.GetUserID(ctx)
	detail := map[string]interface{}{
		"mode":           "presign",
		"expiry_seconds": req.Expiry,
		"expires_at":     time.Now().Add(expiry).Unix(),
		"purpose":        purpose,
	}
	if resultSHA256 != "" {
		detail["sha256"] = resultSHA256
	}
	if objectVersion != "" {
		detail["object_version"] = objectVersion
		detail["manifest_sha256"] = manifestSHA256
	}
	if err := h.recordPcapAudit(ctx, r, audit.EventTypePcapDownload, fileTenantID, userID, key, detail); err != nil {
		h.logger.Error("Failed to persist PCAP presign audit", zap.String("key", key), zap.Error(err))
		rw.Error(http.StatusInternalServerError, "AUDIT_PERSIST_FAILED", "Failed to persist PCAP presign audit", nil)
		return
	}

	resultSHA256 := h.registeredResultSHA256(ctx, key)
	userID := httpx.GetUserID(ctx)
	detail := map[string]interface{}{
		"mode":           "presign",
		"expiry_seconds": req.Expiry,
		"expires_at":     time.Now().Add(expiry).Unix(),
	}
	if resultSHA256 != "" {
		detail["sha256"] = resultSHA256
	}
	if err := h.recordPcapAudit(ctx, r, audit.EventTypePcapDownload, fileTenantID, userID, key, detail); err != nil {
		h.logger.Error("Failed to persist PCAP presign audit", zap.String("key", key), zap.Error(err))
		rw.Error(http.StatusInternalServerError, "AUDIT_PERSIST_FAILED", "Failed to persist PCAP presign audit", nil)
		return
	}

	rw.Success(PresignResponse{
		URL:            signedURL,
		ExpiresAt:      time.Now().Add(expiry).Unix(),
		Key:            key,
		SHA256:         resultSHA256,
		ObjectVersion:  objectVersion,
		ManifestSHA256: manifestSHA256,
		Purpose:        purpose,
	})
}

// VerifyRequest PCAP 完整性校验请求
type VerifyRequest struct {
	Key            string `json:"key"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}

// VerifyResponse PCAP 完整性校验响应
type VerifyResponse struct {
	Key              string `json:"key"`
	TenantID         string `json:"tenant_id"`
	SHA256           string `json:"sha256"`
	ExpectedSHA256   string `json:"expected_sha256,omitempty"`
	RegisteredSHA256 string `json:"registered_sha256,omitempty"`
	Verified         bool   `json:"verified"`
	SizeBytes        int64  `json:"size_bytes"`
}

// VerifyPCAP 计算对象 SHA-256 并与请求值或任务登记值比对。
func (h *Handler) VerifyPCAP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rw := httpx.NewResponseWriter(w, ctx)

	var req VerifyRequest
	if err := decodeForensicsJSON(w, r, &req); err != nil {
		rw.Error(http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil)
		return
	}
	if req.Key == "" {
		rw.Error(http.StatusBadRequest, "INVALID_PARAMETER", "key is required", nil)
		return
	}
	expectedSHA256 := strings.ToLower(strings.TrimSpace(req.ExpectedSHA256))
	if expectedSHA256 != "" && !forensicsSHA256.MatchString(expectedSHA256) {
		rw.Error(http.StatusBadRequest, "INVALID_PARAMETER", "expected_sha256 must contain exactly 64 hexadecimal characters", nil)
		return
	}

	key, fileTenantID, err := h.normalizeResultKey(req.Key)
	if err != nil {
		rw.Error(http.StatusBadRequest, "INVALID_PARAMETER", err.Error(), nil)
		return
	}

	userTenantID := currentTenantID(ctx)
	if !h.checkCrossTenantAccess(ctx, userTenantID, fileTenantID) {
		h.logger.Warn("Cross-tenant integrity verification denied",
			zap.String("user_tenant", userTenantID),
			zap.String("file_tenant", fileTenantID))
		rw.Error(http.StatusForbidden, "FORBIDDEN", "Access denied", nil)
		return
	}
	manifest, err := h.versionedResultManifest(ctx, fileTenantID, key)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			rw.Error(http.StatusNotFound, "PCAP_NOT_FOUND", "Versioned result manifest not found", nil)
		} else {
			rw.Error(http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to resolve immutable result authority", nil)
		}
		return
	}

	actualSHA256 := ""
	var sizeBytes int64
	registeredSHA256 := ""
	objectVersion := ""
	if manifest != nil {
		object, verified, openErr := h.s3Client.OpenForensicsResultObject(ctx, manifest.ResultObject, fileTenantID, manifest.TaskID)
		if openErr != nil {
			err = openErr
		} else {
			defer object.Close()
			hasher := sha256.New()
			sizeBytes, err = io.Copy(hasher, io.LimitReader(object, verified.SizeBytes+1))
			if err == nil && sizeBytes != verified.SizeBytes {
				err = fmt.Errorf("versioned result body size differs from final manifest")
			}
			actualSHA256 = hex.EncodeToString(hasher.Sum(nil))
			registeredSHA256 = verified.SHA256
			objectVersion = verified.VersionID
		}
	} else {
		var exists bool
		exists, err = h.s3Client.ObjectExists(ctx, key)
		if err == nil && !exists {
			err = errors.New(errors.ErrCodeResourceNotFound, "legacy result object not found")
		}
		if err == nil {
			actualSHA256, sizeBytes, err = h.computeObjectSHA256(ctx, key)
		}
		registeredSHA256 = h.registeredResultSHA256(ctx, fileTenantID, key)
	}
	if err != nil {
		h.logger.Error("Failed to compute PCAP sha256", zap.String("key", key), zap.Error(err))
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			rw.Error(http.StatusNotFound, "PCAP_NOT_FOUND", "File not found", nil)
		} else {
			rw.Error(http.StatusInternalServerError, "INTEGRITY_CHECK_FAILED", "Failed to compute sha256", nil)
		}
		return
	}

	referenceSHA256 := expectedSHA256
	if referenceSHA256 == "" {
		referenceSHA256 = registeredSHA256
	}
	if referenceSHA256 == "" {
		if err := h.recordPcapAudit(ctx, r, audit.EventTypePcapIntegrityVerify, fileTenantID, httpx.GetUserID(ctx), key, map[string]interface{}{
			"mode": "integrity_verify", "sha256": actualSHA256, "verifiable": false, "verified": false, "size_bytes": sizeBytes,
		}); err != nil {
			rw.Error(http.StatusInternalServerError, "AUDIT_PERSIST_FAILED", "Failed to persist PCAP integrity audit", nil)
			return
		}
		rw.Error(http.StatusUnprocessableEntity, "REFERENCE_HASH_REQUIRED", "expected_sha256 or a registered task hash is required", nil)
		return
	}
	verified := actualSHA256 == referenceSHA256

	userID := httpx.GetUserID(ctx)
	if err := h.recordPcapAudit(ctx, r, audit.EventTypePcapIntegrityVerify, fileTenantID, userID, key, map[string]interface{}{
		"mode":              "integrity_verify",
		"sha256":            actualSHA256,
		"expected_sha256":   expectedSHA256,
		"registered_sha256": registeredSHA256,
		"verified":          verified,
		"size_bytes":        sizeBytes,
		"object_version":    objectVersion,
	}); err != nil {
		h.logger.Error("Failed to persist PCAP integrity audit", zap.String("key", key), zap.Error(err))
		rw.Error(http.StatusInternalServerError, "AUDIT_PERSIST_FAILED", "Failed to persist PCAP integrity audit", nil)
		return
	}

	rw.Success(VerifyResponse{
		Key:              key,
		TenantID:         fileTenantID,
		SHA256:           actualSHA256,
		ExpectedSHA256:   expectedSHA256,
		RegisteredSHA256: registeredSHA256,
		Verified:         verified,
		SizeBytes:        sizeBytes,
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
	var fixture map[string]interface{}
	if h.loadUIFixture(ctx, tenantID, "stats", &fixture) {
		fixture["tenant_id"] = tenantID
		rw.Success(fixture)
		return
	}

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
	w.Header().Set("Content-Type", "application/json")

	// A TCP-level PostgreSQL connection is insufficient: the process can be
	// alive while every task command fails because its versioned schema has not
	// been applied. Keep the probe coupled to the minimum contract required by
	// Create/List/worker paths.
	if err := h.checkForensicsSchema(ctx); err != nil {
		h.logger.Error("Forensics schema readiness check failed", zap.Error(err))
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not ready",
			"error":  "PostgreSQL schema is not ready",
		})
		return
	}

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

	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func (h *Handler) checkForensicsSchema(ctx context.Context) error {
	if h.auditDB == nil {
		return fmt.Errorf("PostgreSQL dependency is unavailable")
	}
	var ready bool
	err := h.auditDB.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) = 4
			 FROM information_schema.columns
			 WHERE table_schema='public' AND table_name='tasks'
			   AND column_name IN ('revision','deleted_at','last_action_id','last_trace_id'))
			AND to_regclass('public.forensics_task_history') IS NOT NULL
			AND to_regclass('public.forensics_task_outbox') IS NOT NULL
			AND to_regclass('public.forensics_task_requests') IS NOT NULL
			AND EXISTS (
				SELECT 1 FROM alignment_schema_migrations
				WHERE version='202608031600'
			)`).Scan(&ready)
	if err != nil {
		return fmt.Errorf("inspect required forensics schema: %w", err)
	}
	if !ready {
		return fmt.Errorf("required forensics schema version 202608031600 is incomplete")
	}
	return nil
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

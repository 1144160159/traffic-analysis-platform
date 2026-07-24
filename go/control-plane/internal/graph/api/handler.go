////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/graph/api/handler.go
// Graph Service API 处理器（完整监控集成版）
// 修复内容：
// 1. 添加 NewHandlerWithMonitoring 构造函数
// 2. 所有查询方法集成 QueryLogger
// 3. 所有查询方法集成 SlowQueryDetector
// 4. 使用 TenantConfigLoader 动态获取配置
// 5. 添加查询包装器自动记录日志
////////////////////////////////////////////////////////////////////////////////

package api

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/validation"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/cache"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
	graphlogging "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/monitoring"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/query"
)

// 角色常量定义（修复 H3）
const (
	RoleAdmin      = "admin"
	RoleSuperAdmin = "super_admin"
	RoleAnalyst    = "analyst"
	RoleViewer     = "viewer"
)

// Handler API 处理器
type Handler struct {
	graphQuery         *query.GraphQueryWithCircuitBreaker
	cache              *cache.GraphCache
	auditLogger        *audit.Logger
	rateLimiter        *storage.SlidingWindowRateLimiter
	security           config.SecurityConfig
	queryConfig        config.QueryConfig
	logger             *zap.Logger
	ipValidator        *validation.IPValidator
	queryLogger        *graphlogging.QueryLogger     // 新增
	slowQueryDetector  *monitoring.SlowQueryDetector // 新增
	tenantConfigLoader *config.TenantConfigLoader    // 新增
}

// NewHandler 创建处理器（保留向后兼容）
func NewHandler(
	graphQuery *query.GraphQuery,
	cache *cache.GraphCache,
	auditLogger *audit.Logger,
	rateLimiter *storage.SlidingWindowRateLimiter,
	security config.SecurityConfig,
	queryConfig config.QueryConfig,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		graphQuery:  &query.GraphQueryWithCircuitBreaker{GraphQuery: graphQuery},
		cache:       cache,
		auditLogger: auditLogger,
		rateLimiter: rateLimiter,
		security:    security,
		queryConfig: queryConfig,
		logger:      logger,
		ipValidator: validation.NewIPValidator(),
	}
}

// NewHandlerWithMonitoring 创建带监控的处理器（新增）
func NewHandlerWithMonitoring(
	graphQuery *query.GraphQueryWithCircuitBreaker,
	cache *cache.GraphCache,
	auditLogger *audit.Logger,
	rateLimiter *storage.SlidingWindowRateLimiter,
	security config.SecurityConfig,
	queryConfig config.QueryConfig,
	logger *zap.Logger,
	queryLogger *graphlogging.QueryLogger,
	slowQueryDetector *monitoring.SlowQueryDetector,
	tenantConfigLoader *config.TenantConfigLoader,
) *Handler {
	return &Handler{
		graphQuery:         graphQuery,
		cache:              cache,
		auditLogger:        auditLogger,
		rateLimiter:        rateLimiter,
		security:           security,
		queryConfig:        queryConfig,
		logger:             logger,
		ipValidator:        validation.NewIPValidator(),
		queryLogger:        queryLogger,
		slowQueryDetector:  slowQueryDetector,
		tenantConfigLoader: tenantConfigLoader,
	}
}

// 修复 H3：通用权限检查方法
func (h *Handler) requireRole(ctx context.Context, allowedRoles ...string) bool {
	roles := httpx.GetRoles(ctx)
	if len(roles) == 0 {
		return false
	}

	for _, userRole := range roles {
		for _, allowedRole := range allowedRoles {
			if userRole == allowedRole {
				return true
			}
		}
	}
	return false
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(r *mux.Router) {
	// 图探索
	r.HandleFunc("/api/v1/graph/explore", h.ExploreGraph).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/graph/explore/batch", h.BatchExplore).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/graph/workbench", h.GetWorkbenchGraph).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/graph/workbench/path", h.GetWorkbenchPath).Methods("GET", "OPTIONS")

	// 实体详情
	r.HandleFunc("/api/v1/graph/entity/{id}", h.GetEntityDetails).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/graph/entity/{id}/neighbors", h.GetEntityNeighbors).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/graph/entity/{id}/timeline", h.GetEntityTimeline).Methods("GET", "OPTIONS")

	// 路径查询
	r.HandleFunc("/api/v1/graph/path", h.FindPath).Methods("GET", "OPTIONS")

	// 统计
	r.HandleFunc("/api/v1/graph/stats", h.GetStats).Methods("GET", "OPTIONS")

	// 缓存管理（需要管理员权限）
	r.HandleFunc("/api/v1/graph/cache/stats", h.GetCacheStats).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/graph/cache/invalidate", h.InvalidateCache).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/graph/cache/warmup", h.WarmupCache).Methods("POST", "OPTIONS") // 新增
}

// GetWorkbenchGraph returns the persisted multi-entity graph used by the
// analyst workbench. Tenant identity always comes from the authenticated
// context; callers cannot select another tenant through query parameters.
func (h *Handler) GetWorkbenchGraph(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queryStartedAt := time.Now()
	tenantID := httpx.GetTenantID(ctx)
	if tenantID == "" {
		err := errors.New(errors.ErrCodeTenantNotFound, "Tenant ID is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	filter, filterErr := parseWorkbenchFilter(r)
	if filterErr != nil {
		errors.WriteError(w, filterErr, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	tenantQueryConfig := h.getTenantQueryConfig(ctx, tenantID)
	filter.Limit = tenantQueryConfig.MaxNodes
	graph, err := h.graphQuery.GetWorkbenchGraph(ctx, tenantID, filter)
	if err != nil {
		h.logger.Error("Failed to load entity graph workbench",
			zap.String("tenant_id", tenantID),
			zap.String("center_id", filter.CenterID),
			zap.Error(err))
		appErr := errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to load entity graph workbench")
		errors.WriteError(w, appErr, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	queryDurationMS := time.Since(queryStartedAt).Milliseconds()

	if h.auditLogger != nil {
		h.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    "GRAPH_WORKBENCH_VIEW",
			TenantID:     tenantID,
			UserID:       httpx.GetUserID(ctx),
			Action:       "graph_workbench_view",
			ResourceType: "graph",
			ResourceID:   graph.CenterID,
			Detail: map[string]interface{}{
				"node_count":  len(graph.Nodes),
				"edge_count":  len(graph.Edges),
				"depth":       filter.Depth,
				"entity_type": filter.EntityType,
				"site":        filter.Site,
			},
			Result: audit.ResultSuccess,
		})
	}

	errors.WriteSuccess(w, map[string]interface{}{
		"graph": graph,
		"meta": map[string]interface{}{
			"source":            h.graphQuery.WorkbenchSource(),
			"node_count":        len(graph.Nodes),
			"edge_count":        len(graph.Edges),
			"depth":             filter.Depth,
			"entity_type":       filter.EntityType,
			"site":              filter.Site,
			"time_range":        filter.TimeRange,
			"query_duration_ms": queryDurationMS,
			"node_limit":        tenantQueryConfig.MaxNodes,
			"cache_hit_rate":    "N/A",
			"cache_applicable":  false,
			"data_origin":       "nebula_graph_persisted_projection",
			"slow_query":        queryDurationMS >= 500,
		},
	}, httpx.GetTraceID(ctx))
}

func parseWorkbenchFilter(r *http.Request) (query.WorkbenchFilter, *errors.AppError) {
	filter := query.WorkbenchFilter{
		CenterID:   strings.TrimSpace(r.URL.Query().Get("center_id")),
		Depth:      2,
		EntityType: strings.TrimSpace(r.URL.Query().Get("entity_type")),
		Site:       strings.TrimSpace(r.URL.Query().Get("site")),
	}
	if filter.EntityType == "" {
		filter.EntityType = "all"
	}
	if filter.Site == "" {
		filter.Site = "main"
	}
	if rawDepth := strings.TrimSpace(r.URL.Query().Get("depth")); rawDepth != "" {
		depth, err := strconv.Atoi(rawDepth)
		if err != nil || depth < 1 || depth > 3 {
			return filter, errors.New(errors.ErrCodeInvalidParameter, "depth must be between 1 and 3")
		}
		filter.Depth = depth
	}
	allowedTypes := map[string]bool{"all": true, "ip": true, "host": true, "account": true, "domain": true, "service": true, "alert": true, "evidence": true}
	if !allowedTypes[filter.EntityType] {
		return filter, errors.New(errors.ErrCodeInvalidParameter, "unsupported entity_type")
	}
	timeRange := strings.TrimSpace(r.URL.Query().Get("time_range"))
	switch timeRange {
	case "", "24h":
		filter.TimeRange = "24h"
		filter.SinceMS = time.Now().Add(-24 * time.Hour).UnixMilli()
	case "7d":
		filter.TimeRange = "7d"
		filter.SinceMS = time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
	case "all":
		filter.TimeRange = "all"
		filter.SinceMS = 0
	default:
		return filter, errors.New(errors.ErrCodeInvalidParameter, "time_range must be 24h, 7d or all")
	}
	if len(filter.CenterID) > 256 || len(filter.Site) > 64 {
		return filter, errors.New(errors.ErrCodeInvalidParameter, "workbench filter is too long")
	}
	return filter, nil
}

// GetWorkbenchPath returns a persisted NebulaGraph relationship path for one analysis tab.
func (h *Handler) GetWorkbenchPath(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := httpx.GetTenantID(ctx)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "Tenant ID is required"), httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	sourceID := strings.TrimSpace(r.URL.Query().Get("source_id"))
	targetID := strings.TrimSpace(r.URL.Query().Get("target_id"))
	anchorID := strings.TrimSpace(r.URL.Query().Get("anchor_id"))
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "shortest"
	}
	allowedModes := map[string]bool{"shortest": true, "attack": true, "communication": true, "account": true}
	if sourceID == "" || targetID == "" || len(sourceID) > 256 || len(targetID) > 256 || len(anchorID) > 256 || !allowedModes[mode] {
		errors.WriteError(w, errors.New(errors.ErrCodeInvalidParameter, "valid source_id, target_id and mode are required"), httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	maxDepth := 3
	if rawDepth := strings.TrimSpace(r.URL.Query().Get("max_depth")); rawDepth != "" {
		parsed, err := strconv.Atoi(rawDepth)
		if err != nil || parsed < 1 || parsed > 6 {
			errors.WriteError(w, errors.New(errors.ErrCodeInvalidParameter, "max_depth must be between 1 and 6"), httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		maxDepth = parsed
	}
	filter, filterErr := parseWorkbenchFilter(r)
	if filterErr != nil {
		errors.WriteError(w, filterErr, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	filter.CenterID = sourceID
	filter.Depth = maxDepth
	filter.Limit = h.getTenantQueryConfig(ctx, tenantID).MaxNodes
	path, err := h.graphQuery.FindWorkbenchPath(ctx, tenantID, sourceID, targetID, anchorID, mode, filter)
	if err != nil {
		errors.WriteError(w, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to analyze entity graph path"), httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	errors.WriteSuccess(w, map[string]interface{}{
		"path": path,
		"meta": map[string]interface{}{"source": h.graphQuery.WorkbenchSource(), "mode": mode, "anchor_id": anchorID, "site": filter.Site, "entity_type": filter.EntityType, "time_range": filter.TimeRange, "max_depth": filter.Depth},
	}, httpx.GetTraceID(ctx))
}

// logQueryMetrics 记录查询指标（新增辅助方法）
func (h *Handler) logQueryMetrics(
	ctx context.Context,
	tenantID, queryID, userID, queryType, centerIP string,
	centerIPs []string,
	depth int,
	runID string,
	startTime, endTime time.Time,
	graph *query.Graph,
	cacheHit bool,
	status, errorCode, errorMessage string,
	duration time.Duration,
) {
	if h.queryLogger == nil {
		return
	}

	// 计算结果大小（估算）
	var resultSize uint64
	var nodeCount int
	var edgeCount int
	if graph != nil {
		nodeCount = len(graph.Nodes)
		edgeCount = len(graph.Edges)
		// 粗略估算：每个节点 200 bytes，每条边 150 bytes
		resultSize = uint64(nodeCount*200 + edgeCount*150)
	}

	log := &graphlogging.QueryLog{
		TenantID:  tenantID,
		QueryID:   queryID,
		UserID:    userID,
		QueryType: queryType,
		CenterIP:  centerIP,
		CenterIPs: centerIPs,
		Depth:     uint8(depth),
		RunID:     runID,

		QueryStartTime: startTime,
		QueryEndTime:   endTime,

		NodeCount:       uint32(nodeCount),
		EdgeCount:       uint32(edgeCount),
		ResultSizeBytes: resultSize,

		DurationMs: uint32(duration.Milliseconds()),
		CacheHit:   cacheHit,

		Status:       status,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,

		TraceID:   httpx.GetTraceID(ctx),
		ClientIP:  "unknown",
		UserAgent: "unknown",
	}

	h.queryLogger.Log(ctx, log)

	// 检查慢查询
	if h.slowQueryDetector != nil {
		h.slowQueryDetector.CheckAndLog(
			ctx,
			tenantID, queryID,
			queryType,
			centerIP,
			depth,
			runID,
			duration,
			len(graph.Nodes),
			len(graph.Edges),
			0, 0, // ClickHouse 统计（暂未实现）
			errorMessage,
		)
	}
}

// getTenantQueryConfig 获取租户查询配置（新增辅助方法）
func (h *Handler) getTenantQueryConfig(ctx context.Context, tenantID string) *config.TenantQueryConfig {
	if h.tenantConfigLoader == nil {
		// 使用默认配置
		return &config.TenantQueryConfig{
			TenantID:              tenantID,
			MaxDepth:              h.queryConfig.MaxDepth,
			DefaultDepth:          h.queryConfig.DefaultDepth,
			MaxNodes:              h.queryConfig.MaxNodes,
			MaxNeighborsPerHop:    h.queryConfig.MaxNeighborsPerHop,
			DefaultTimeRangeHours: h.queryConfig.DefaultTimeRangeHours,
			MaxBatchExploreIPs:    h.queryConfig.MaxBatchExploreIPs,
			MaxPathSearchHops:     h.queryConfig.MaxPathSearchHops,
			AlertBatchSize:        h.queryConfig.AlertBatchSize,
			QueryTimeoutSec:       int(h.queryConfig.QueryTimeout.Seconds()),
			MaxConcurrentQueries:  h.queryConfig.MaxConcurrentQueries,
		}
	}

	cfg, err := h.tenantConfigLoader.GetQueryConfig(ctx, tenantID)
	if err != nil {
		h.logger.Warn("Failed to load tenant query config, using defaults",
			zap.String("tenant_id", tenantID),
			zap.Error(err))
		return &config.TenantQueryConfig{
			TenantID:              tenantID,
			MaxDepth:              h.queryConfig.MaxDepth,
			DefaultDepth:          h.queryConfig.DefaultDepth,
			MaxNodes:              h.queryConfig.MaxNodes,
			MaxNeighborsPerHop:    h.queryConfig.MaxNeighborsPerHop,
			DefaultTimeRangeHours: h.queryConfig.DefaultTimeRangeHours,
			MaxBatchExploreIPs:    h.queryConfig.MaxBatchExploreIPs,
			MaxPathSearchHops:     h.queryConfig.MaxPathSearchHops,
			AlertBatchSize:        h.queryConfig.AlertBatchSize,
			QueryTimeoutSec:       int(h.queryConfig.QueryTimeout.Seconds()),
			MaxConcurrentQueries:  h.queryConfig.MaxConcurrentQueries,
		}
	}

	return cfg
}

// ExploreGraph 图探索接口（完整修复版）
func (h *Handler) ExploreGraph(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queryID := uuid.New().String()
	queryStart := time.Now()

	tenantID := httpx.GetTenantID(ctx)
	if tenantID == "" {
		err := errors.New(errors.ErrCodeTenantNotFound, "Tenant ID is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	userID := httpx.GetUserID(ctx)

	// 限流检查
	if h.rateLimiter != nil && h.security.RateLimitEnabled {
		key := fmt.Sprintf("graph:explore:%s", tenantID)
		allowed, err := h.rateLimiter.Allow(ctx, key, h.security.RateLimitRPS, h.security.RateLimitWindowSec)
		if err != nil {
			h.logger.Error("Rate limiter error", zap.Error(err))
		} else if !allowed {
			err := errors.New(errors.ErrCodeQuotaExceeded, "Rate limit exceeded")
			errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
	}

	// 获取租户配置
	tenantCfg := h.getTenantQueryConfig(ctx, tenantID)

	// 解析并验证 IP
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		err := errors.New(errors.ErrCodeInvalidParameter, "ip parameter is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	sanitizedIP, ok := validation.SanitizeIP(ip)
	if !ok || !h.ipValidator.IsValidIP(sanitizedIP) {
		err := errors.Newf(errors.ErrCodeInvalidParameter, "invalid ip format: %s", ip)
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	ip = sanitizedIP

	// 解析深度（使用租户配置）
	depth := tenantCfg.DefaultDepth
	if d := r.URL.Query().Get("depth"); d != "" {
		parsed, parseErr := strconv.Atoi(d)
		if parseErr != nil || parsed < 1 || parsed > tenantCfg.MaxDepth {
			err := errors.Newf(errors.ErrCodeInvalidParameter,
				"depth must be between 1 and %d", tenantCfg.MaxDepth)
			errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		depth = parsed
	}

	// 解析时间范围（使用租户配置）
	endTime := time.Now().UnixMilli()
	startTime := endTime - int64(tenantCfg.DefaultTimeRangeHours)*3600*1000

	if s := r.URL.Query().Get("start_time"); s != "" {
		if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
			startTime = ts
		}
	}
	if e := r.URL.Query().Get("end_time"); e != "" {
		if ts, err := strconv.ParseInt(e, 10, 64); err == nil {
			endTime = ts
		}
	}

	// 验证时间范围
	if endTime <= startTime {
		err := errors.New(errors.ErrCodeInvalidParameter, "end_time must be greater than start_time")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	maxRange := int64(7 * 24 * 3600 * 1000)
	if endTime-startTime > maxRange {
		err := errors.New(errors.ErrCodeInvalidParameter, "time range cannot exceed 7 days")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	// 解析 run_id
	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		runID = "realtime"
	}

	h.logger.Info("Graph explore request",
		zap.String("tenant_id", tenantID),
		zap.String("user_id", userID),
		zap.String("query_id", queryID),
		zap.String("ip", ip),
		zap.Int("depth", depth),
		zap.String("run_id", runID))

	// 审计日志
	if h.auditLogger != nil {
		h.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    "GRAPH_EXPLORE",
			TenantID:     tenantID,
			UserID:       userID,
			Action:       "graph_explore",
			ResourceType: "graph",
			ResourceID:   ip,
			Detail: map[string]interface{}{
				"query_id":   queryID,
				"depth":      depth,
				"run_id":     runID,
				"start_time": startTime,
				"end_time":   endTime,
			},
			Result: audit.ResultSuccess,
		})
	}

	// 修复 W2：更新热点 IP
	if h.tenantConfigLoader != nil && h.tenantConfigLoader.GetDB() != nil {
		go func() {
			updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = cache.UpdateHotIP(updateCtx, h.tenantConfigLoader.GetDB(), tenantID, ip)
		}()
	}

	// 执行查询
	graph, err := h.graphQuery.Explore(ctx, tenantID, ip, depth, startTime, endTime, runID)
	duration := time.Since(queryStart)

	// 记录查询日志
	status := "success"
	errorCode := ""
	errorMessage := ""
	var cacheHit bool
	if graph != nil {
		cacheHit = graph.CacheHit
	}

	if err != nil {
		h.logger.Error("Failed to explore graph",
			zap.String("query_id", queryID),
			zap.String("ip", ip),
			zap.Error(err))

		status = "error"
		if appErr, ok := err.(*errors.AppError); ok {
			errorCode = string(appErr.Code)
		}
		errorMessage = err.Error()

		if h.auditLogger != nil {
			h.auditLogger.LogError(ctx, "GRAPH_EXPLORE_FAILED", tenantID, userID, err, nil)
		}
	}

	// 记录指标
	h.logQueryMetrics(
		ctx, tenantID, queryID, userID,
		"explore", ip, nil, depth, runID,
		time.UnixMilli(startTime), time.UnixMilli(endTime),
		graph, cacheHit, status, errorCode, errorMessage,
		duration,
	)

	if err != nil {
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	// 返回结果
	errors.WriteSuccess(w, map[string]interface{}{
		"query_id": queryID,
		"graph":    graph,
		"meta": map[string]interface{}{
			"center_ip":   ip,
			"depth":       depth,
			"run_id":      runID,
			"start_time":  startTime,
			"end_time":    endTime,
			"node_count":  len(graph.Nodes),
			"edge_count":  len(graph.Edges),
			"truncated":   graph.Truncated,
			"duration_ms": duration.Milliseconds(),
		},
	}, httpx.GetTraceID(ctx))
}

// BatchExploreRequest 批量探索请求
type BatchExploreRequest struct {
	TenantID  string   `json:"tenant_id"`
	IPs       []string `json:"ips"`
	Depth     int      `json:"depth"`
	StartTime int64    `json:"start_time"`
	EndTime   int64    `json:"end_time"`
	RunID     string   `json:"run_id"`
}

// BatchExplore 批量图探索（完整修复版）
func (h *Handler) BatchExplore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queryID := uuid.New().String()
	queryStart := time.Now()

	tenantID := httpx.GetTenantID(ctx)
	if tenantID == "" {
		err := errors.New(errors.ErrCodeTenantNotFound, "Tenant ID is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	userID := httpx.GetUserID(ctx)

	// 限流检查
	if h.rateLimiter != nil && h.security.RateLimitEnabled {
		key := fmt.Sprintf("graph:batch_explore:%s", tenantID)
		allowed, err := h.rateLimiter.Allow(ctx, key, h.security.RateLimitRPS/2, h.security.RateLimitWindowSec)
		if err != nil {
			h.logger.Error("Rate limiter error", zap.Error(err))
		} else if !allowed {
			err := errors.New(errors.ErrCodeQuotaExceeded, "Rate limit exceeded")
			errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
	}

	// 获取租户配置
	tenantCfg := h.getTenantQueryConfig(ctx, tenantID)

	var req BatchExploreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appErr := errors.Wrap(err, errors.ErrCodeInvalidRequest, "Invalid request body")
		errors.WriteError(w, appErr, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	// 验证 IPs
	if len(req.IPs) == 0 {
		err := errors.New(errors.ErrCodeInvalidParameter, "ips is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	maxBatchSize := tenantCfg.MaxBatchExploreIPs
	if len(req.IPs) > maxBatchSize {
		err := errors.Newf(errors.ErrCodeInvalidParameter, "maximum %d IPs allowed", maxBatchSize)
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	// 验证所有 IP
	validIPs, invalidIPs := validation.ValidateIPList(req.IPs)
	if len(invalidIPs) > 0 {
		err := errors.Newf(errors.ErrCodeInvalidParameter, "invalid ips: %v", invalidIPs)
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	req.IPs = validIPs
	req.TenantID = tenantID

	// 设置默认值
	if req.Depth == 0 {
		req.Depth = 1
	}
	if req.Depth > 3 {
		req.Depth = 3
	}

	if req.EndTime == 0 {
		req.EndTime = time.Now().UnixMilli()
	}
	if req.StartTime == 0 {
		req.StartTime = req.EndTime - int64(tenantCfg.DefaultTimeRangeHours)*3600*1000
	}

	if req.RunID == "" {
		req.RunID = "realtime"
	}

	h.logger.Info("Batch explore request",
		zap.String("tenant_id", tenantID),
		zap.String("user_id", userID),
		zap.String("query_id", queryID),
		zap.Strings("ips", req.IPs),
		zap.Int("depth", req.Depth),
		zap.String("run_id", req.RunID))

	// 审计日志
	if h.auditLogger != nil {
		h.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    "GRAPH_BATCH_EXPLORE",
			TenantID:     tenantID,
			UserID:       userID,
			Action:       "batch_explore",
			ResourceType: "graph",
			Detail: map[string]interface{}{
				"query_id":   queryID,
				"ips":        req.IPs,
				"depth":      req.Depth,
				"run_id":     req.RunID,
				"start_time": req.StartTime,
				"end_time":   req.EndTime,
			},
			Result: audit.ResultSuccess,
		})
	}

	// 执行批量查询
	graph, err := h.graphQuery.BatchExplore(ctx, tenantID, req.IPs, req.Depth, req.StartTime, req.EndTime, req.RunID)
	duration := time.Since(queryStart)

	// 修复 H5：使用 IPs 的哈希作为 center_ip
	centerIPHash := fmt.Sprintf("batch:%x", md5.Sum([]byte(strings.Join(req.IPs, ","))))

	// 记录查询日志
	status := "success"
	errorCode := ""
	errorMessage := ""
	cacheHit := false

	if err != nil {
		h.logger.Error("Failed to batch explore",
			zap.String("query_id", queryID),
			zap.Strings("ips", req.IPs),
			zap.Error(err))

		status = "error"
		if appErr, ok := err.(*errors.AppError); ok {
			errorCode = string(appErr.Code)
		}
		errorMessage = err.Error()

		if h.auditLogger != nil {
			h.auditLogger.LogError(ctx, "GRAPH_BATCH_EXPLORE_FAILED", tenantID, userID, err, nil)
		}
	}

	// 记录指标
	h.logQueryMetrics(
		ctx, tenantID, queryID, userID,
		"batch_explore", centerIPHash, req.IPs, req.Depth, req.RunID,
		time.UnixMilli(req.StartTime), time.UnixMilli(req.EndTime),
		graph, cacheHit, status, errorCode, errorMessage,
		duration,
	)

	if err != nil {
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	errors.WriteSuccess(w, map[string]interface{}{
		"query_id": queryID,
		"graph":    graph,
		"meta": map[string]interface{}{
			"center_ips":  req.IPs,
			"depth":       req.Depth,
			"run_id":      req.RunID,
			"start_time":  req.StartTime,
			"end_time":    req.EndTime,
			"node_count":  len(graph.Nodes),
			"edge_count":  len(graph.Edges),
			"truncated":   graph.Truncated,
			"duration_ms": duration.Milliseconds(),
		},
	}, httpx.GetTraceID(ctx))
}

// GetEntityDetails 获取实体详情（添加监控）
func (h *Handler) GetEntityDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queryID := uuid.New().String()
	queryStart := time.Now()

	tenantID := httpx.GetTenantID(ctx)
	if tenantID == "" {
		err := errors.New(errors.ErrCodeTenantNotFound, "Tenant ID is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	userID := httpx.GetUserID(ctx)

	vars := mux.Vars(r)
	entityID := vars["id"]

	if entityID == "" {
		err := errors.New(errors.ErrCodeInvalidParameter, "entity id is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	entityType := r.URL.Query().Get("type")
	if entityType == "" {
		entityType = "ip"
	}

	validTypes := map[string]bool{"ip": true, "domain": true, "hash": true}
	if !validTypes[entityType] {
		err := errors.New(errors.ErrCodeInvalidParameter, "invalid entity type")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	// IP 验证
	if entityType == "ip" {
		sanitizedIP, ok := validation.SanitizeIP(entityID)
		if !ok || !h.ipValidator.IsValidIP(sanitizedIP) {
			err := errors.Newf(errors.ErrCodeInvalidParameter, "invalid ip format: %s", entityID)
			errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		entityID = sanitizedIP
	}

	// 获取租户配置
	tenantCfg := h.getTenantQueryConfig(ctx, tenantID)

	endTime := time.Now().UnixMilli()
	startTime := endTime - int64(tenantCfg.DefaultTimeRangeHours)*3600*1000

	if s := r.URL.Query().Get("start_time"); s != "" {
		if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
			startTime = ts
		}
	}
	if e := r.URL.Query().Get("end_time"); e != "" {
		if ts, err := strconv.ParseInt(e, 10, 64); err == nil {
			endTime = ts
		}
	}

	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		runID = "realtime"
	}

	// 审计日志
	if h.auditLogger != nil {
		h.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    "GRAPH_ENTITY_DETAILS",
			TenantID:     tenantID,
			UserID:       userID,
			Action:       "get_entity_details",
			ResourceType: "entity",
			ResourceID:   entityID,
			Detail: map[string]interface{}{
				"query_id":    queryID,
				"entity_type": entityType,
				"run_id":      runID,
				"start_time":  startTime,
				"end_time":    endTime,
			},
			Result: audit.ResultSuccess,
		})
	}

	details, err := h.graphQuery.GetEntityDetails(ctx, tenantID, entityID, entityType, startTime, endTime, runID)
	duration := time.Since(queryStart)

	// 记录查询日志
	status := "success"
	errorCode := ""
	errorMessage := ""

	if err != nil {
		h.logger.Error("Failed to get entity details",
			zap.String("query_id", queryID),
			zap.String("entity_id", entityID),
			zap.Error(err))

		status = "error"
		if appErr, ok := err.(*errors.AppError); ok {
			errorCode = string(appErr.Code)
		}
		errorMessage = err.Error()
	}

	// 记录指标（实体详情返回的不是 Graph 对象）
	emptyGraph := &query.Graph{Nodes: []*query.GraphNode{}, Edges: []*query.GraphEdge{}}
	h.logQueryMetrics(
		ctx, tenantID, queryID, userID,
		"entity_details", entityID, nil, 0, runID,
		time.UnixMilli(startTime), time.UnixMilli(endTime),
		emptyGraph, false, status, errorCode, errorMessage,
		duration,
	)

	if err != nil {
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	errors.WriteSuccess(w, details, httpx.GetTraceID(ctx))
}

// GetEntityNeighbors 获取实体邻居（添加监控）
func (h *Handler) GetEntityNeighbors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queryID := uuid.New().String()
	queryStart := time.Now()

	tenantID := httpx.GetTenantID(ctx)
	if tenantID == "" {
		err := errors.New(errors.ErrCodeTenantNotFound, "Tenant ID is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	userID := httpx.GetUserID(ctx)

	vars := mux.Vars(r)
	entityID := vars["id"]

	if entityID == "" {
		err := errors.New(errors.ErrCodeInvalidParameter, "entity id is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	sanitizedIP, ok := validation.SanitizeIP(entityID)
	if !ok || !h.ipValidator.IsValidIP(sanitizedIP) {
		err := errors.Newf(errors.ErrCodeInvalidParameter, "invalid ip format: %s", entityID)
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	entityID = sanitizedIP

	limit := parseIntParam(r.URL.Query().Get("limit"), 50, 1, 200)

	// 获取租户配置
	tenantCfg := h.getTenantQueryConfig(ctx, tenantID)

	endTime := time.Now().UnixMilli()
	startTime := endTime - int64(tenantCfg.DefaultTimeRangeHours)*3600*1000

	if s := r.URL.Query().Get("start_time"); s != "" {
		if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
			startTime = ts
		}
	}
	if e := r.URL.Query().Get("end_time"); e != "" {
		if ts, err := strconv.ParseInt(e, 10, 64); err == nil {
			endTime = ts
		}
	}

	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		runID = "realtime"
	}

	neighbors, err := h.graphQuery.GetNeighbors(ctx, tenantID, entityID, startTime, endTime, runID, limit)
	duration := time.Since(queryStart)

	// 记录查询日志
	status := "success"
	errorCode := ""
	errorMessage := ""

	if err != nil {
		h.logger.Error("Failed to get neighbors",
			zap.String("query_id", queryID),
			zap.String("entity_id", entityID),
			zap.Error(err))

		status = "error"
		if appErr, ok := err.(*errors.AppError); ok {
			errorCode = string(appErr.Code)
		}
		errorMessage = err.Error()
	}

	// 记录指标
	graph := &query.Graph{
		Nodes: make([]*query.GraphNode, len(neighbors)),
		Edges: []*query.GraphEdge{},
	}
	for i, n := range neighbors {
		graph.Nodes[i] = n
	}

	h.logQueryMetrics(
		ctx, tenantID, queryID, userID,
		"neighbors", entityID, nil, 0, runID,
		time.UnixMilli(startTime), time.UnixMilli(endTime),
		graph, false, status, errorCode, errorMessage,
		duration,
	)

	if err != nil {
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	errors.WriteSuccess(w, map[string]interface{}{
		"query_id":    queryID,
		"entity_id":   entityID,
		"run_id":      runID,
		"neighbors":   neighbors,
		"count":       len(neighbors),
		"duration_ms": duration.Milliseconds(),
	}, httpx.GetTraceID(ctx))
}

// GetEntityTimeline 获取实体时间线（添加监控）
func (h *Handler) GetEntityTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queryID := uuid.New().String()
	queryStart := time.Now()

	tenantID := httpx.GetTenantID(ctx)
	if tenantID == "" {
		err := errors.New(errors.ErrCodeTenantNotFound, "Tenant ID is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	userID := httpx.GetUserID(ctx)

	vars := mux.Vars(r)
	entityID := vars["id"]

	if entityID == "" {
		err := errors.New(errors.ErrCodeInvalidParameter, "entity id is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	sanitizedIP, ok := validation.SanitizeIP(entityID)
	if !ok || !h.ipValidator.IsValidIP(sanitizedIP) {
		err := errors.Newf(errors.ErrCodeInvalidParameter, "invalid ip format: %s", entityID)
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	entityID = sanitizedIP

	// 获取租户配置
	tenantCfg := h.getTenantQueryConfig(ctx, tenantID)

	endTime := time.Now().UnixMilli()
	startTime := endTime - int64(tenantCfg.DefaultTimeRangeHours)*3600*1000

	if s := r.URL.Query().Get("start_time"); s != "" {
		if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
			startTime = ts
		}
	}
	if e := r.URL.Query().Get("end_time"); e != "" {
		if ts, err := strconv.ParseInt(e, 10, 64); err == nil {
			endTime = ts
		}
	}

	granularity := r.URL.Query().Get("granularity")
	if granularity == "" {
		granularity = "hour"
	}

	validGranularities := map[string]bool{"minute": true, "hour": true, "day": true}
	if !validGranularities[granularity] {
		err := errors.New(errors.ErrCodeInvalidParameter, "granularity must be minute, hour, or day")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		runID = "realtime"
	}

	timeline, err := h.graphQuery.GetEntityTimeline(ctx, tenantID, entityID, startTime, endTime, runID, granularity)
	duration := time.Since(queryStart)

	// 记录查询日志
	status := "success"
	errorCode := ""
	errorMessage := ""

	if err != nil {
		h.logger.Error("Failed to get entity timeline",
			zap.String("query_id", queryID),
			zap.String("entity_id", entityID),
			zap.Error(err))

		status = "error"
		if appErr, ok := err.(*errors.AppError); ok {
			errorCode = string(appErr.Code)
		}
		errorMessage = err.Error()
	}

	// 记录指标
	emptyGraph := &query.Graph{Nodes: []*query.GraphNode{}, Edges: []*query.GraphEdge{}}
	h.logQueryMetrics(
		ctx, tenantID, queryID, userID,
		"timeline", entityID, nil, 0, runID,
		time.UnixMilli(startTime), time.UnixMilli(endTime),
		emptyGraph, false, status, errorCode, errorMessage,
		duration,
	)

	if err != nil {
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	errors.WriteSuccess(w, map[string]interface{}{
		"query_id":    queryID,
		"entity_id":   entityID,
		"run_id":      runID,
		"timeline":    timeline,
		"granularity": granularity,
		"start_time":  startTime,
		"end_time":    endTime,
		"duration_ms": duration.Milliseconds(),
	}, httpx.GetTraceID(ctx))
}

// FindPath 查找路径（添加监控）
func (h *Handler) FindPath(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queryID := uuid.New().String()
	queryStart := time.Now()

	tenantID := httpx.GetTenantID(ctx)
	if tenantID == "" {
		err := errors.New(errors.ErrCodeTenantNotFound, "Tenant ID is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	userID := httpx.GetUserID(ctx)

	sourceIP := r.URL.Query().Get("source")
	targetIP := r.URL.Query().Get("target")

	if sourceIP == "" || targetIP == "" {
		err := errors.New(errors.ErrCodeInvalidParameter, "source and target are required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	sanitizedSource, ok := validation.SanitizeIP(sourceIP)
	if !ok || !h.ipValidator.IsValidIP(sanitizedSource) {
		err := errors.Newf(errors.ErrCodeInvalidParameter, "invalid source ip: %s", sourceIP)
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	sourceIP = sanitizedSource

	sanitizedTarget, ok := validation.SanitizeIP(targetIP)
	if !ok || !h.ipValidator.IsValidIP(sanitizedTarget) {
		err := errors.Newf(errors.ErrCodeInvalidParameter, "invalid target ip: %s", targetIP)
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	targetIP = sanitizedTarget

	// 获取租户配置
	tenantCfg := h.getTenantQueryConfig(ctx, tenantID)

	maxHops := parseIntParam(r.URL.Query().Get("max_hops"), 5, 1, tenantCfg.MaxPathSearchHops)

	endTime := time.Now().UnixMilli()
	startTime := endTime - int64(tenantCfg.DefaultTimeRangeHours)*3600*1000

	if s := r.URL.Query().Get("start_time"); s != "" {
		if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
			startTime = ts
		}
	}
	if e := r.URL.Query().Get("end_time"); e != "" {
		if ts, err := strconv.ParseInt(e, 10, 64); err == nil {
			endTime = ts
		}
	}

	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		runID = "realtime"
	}

	h.logger.Info("Find path request",
		zap.String("query_id", queryID),
		zap.String("source", sourceIP),
		zap.String("target", targetIP),
		zap.Int("max_hops", maxHops),
		zap.String("run_id", runID))

	// 审计日志
	if h.auditLogger != nil {
		h.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    "GRAPH_PATH_SEARCH",
			TenantID:     tenantID,
			UserID:       userID,
			Action:       "find_path",
			ResourceType: "graph",
			Detail: map[string]interface{}{
				"query_id": queryID,
				"source":   sourceIP,
				"target":   targetIP,
				"max_hops": maxHops,
				"run_id":   runID,
			},
			Result: audit.ResultSuccess,
		})
	}

	paths, err := h.graphQuery.FindPaths(ctx, tenantID, sourceIP, targetIP, maxHops, startTime, endTime, runID)
	duration := time.Since(queryStart)

	// 记录查询日志
	status := "success"
	errorCode := ""
	errorMessage := ""

	if err != nil {
		h.logger.Error("Failed to find path",
			zap.String("query_id", queryID),
			zap.String("source", sourceIP),
			zap.String("target", targetIP),
			zap.Error(err))

		status = "error"
		if appErr, ok := err.(*errors.AppError); ok {
			errorCode = string(appErr.Code)
		}
		errorMessage = err.Error()
	}

	// 记录指标
	graph := &query.Graph{Nodes: []*query.GraphNode{}, Edges: []*query.GraphEdge{}}
	h.logQueryMetrics(
		ctx, tenantID, queryID, userID,
		"find_path", sourceIP, nil, maxHops, runID,
		time.UnixMilli(startTime), time.UnixMilli(endTime),
		graph, false, status, errorCode, errorMessage,
		duration,
	)

	if err != nil {
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	errors.WriteSuccess(w, map[string]interface{}{
		"query_id":    queryID,
		"source":      sourceIP,
		"target":      targetIP,
		"max_hops":    maxHops,
		"run_id":      runID,
		"paths":       paths,
		"path_count":  len(paths),
		"duration_ms": duration.Milliseconds(),
	}, httpx.GetTraceID(ctx))
}

// GetStats 获取统计信息（保留原实现）
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenantID := httpx.GetTenantID(ctx)
	if tenantID == "" {
		err := errors.New(errors.ErrCodeTenantNotFound, "Tenant ID is required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	// 获取租户配置
	tenantCfg := h.getTenantQueryConfig(ctx, tenantID)

	endTime := time.Now().UnixMilli()
	startTime := endTime - int64(tenantCfg.DefaultTimeRangeHours)*3600*1000

	if s := r.URL.Query().Get("start_time"); s != "" {
		if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
			startTime = ts
		}
	}
	if e := r.URL.Query().Get("end_time"); e != "" {
		if ts, err := strconv.ParseInt(e, 10, 64); err == nil {
			endTime = ts
		}
	}

	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		runID = "realtime"
	}

	stats, err := h.graphQuery.GetStats(ctx, tenantID, startTime, endTime, runID)
	if err != nil {
		h.logger.Error("Failed to get stats", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	// 添加缓存统计
	if h.cache != nil {
		cacheStats := h.cache.GetStats()
		stats["cache"] = map[string]interface{}{
			"hit_rate": cacheStats["hit_rate"],
			"enabled":  true,
		}
	}

	// 添加查询日志统计
	if h.queryLogger != nil {
		stats["query_logger"] = h.queryLogger.GetStats()
	}

	errors.WriteSuccess(w, stats, httpx.GetTraceID(ctx))
}

// GetCacheStats 获取缓存统计（修复 H3：使用通用权限检查）
func (h *Handler) GetCacheStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 修复 H3：使用通用权限检查
	if !h.requireRole(ctx, RoleAdmin, RoleSuperAdmin) {
		err := errors.New(errors.ErrCodePermissionDenied, "Admin permission required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	if h.cache == nil {
		errors.WriteSuccess(w, map[string]interface{}{
			"enabled": false,
			"message": "Cache is not enabled",
		}, httpx.GetTraceID(ctx))
		return
	}

	errors.WriteSuccess(w, map[string]interface{}{
		"enabled": true,
		"stats":   h.cache.GetStats(),
	}, httpx.GetTraceID(ctx))
}

// InvalidateCacheRequest 缓存失效请求
type InvalidateCacheRequest struct {
	TenantID string `json:"tenant_id"`
	EntityID string `json:"entity_id"`
	Type     string `json:"type"` // "entity", "neighbor", "graph", "all"
}

// InvalidateCache 使缓存失效（仅管理员）
func (h *Handler) InvalidateCache(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	roles := httpx.GetRoles(ctx)
	hasAdminRole := false
	for _, role := range roles {
		if role == "admin" || role == "super_admin" {
			hasAdminRole = true
			break
		}
	}

	if !hasAdminRole {
		err := errors.New(errors.ErrCodePermissionDenied, "Admin permission required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	if h.cache == nil {
		err := errors.New(errors.ErrCodeConfigError, "Cache is not enabled")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	var req InvalidateCacheRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appErr := errors.Wrap(err, errors.ErrCodeInvalidRequest, "Invalid request body")
		errors.WriteError(w, appErr, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = httpx.GetTenantID(ctx)
	}

	var err error
	switch req.Type {
	case "entity":
		if req.EntityID == "" {
			appErr := errors.New(errors.ErrCodeInvalidParameter, "entity_id is required for entity invalidation")
			errors.WriteError(w, appErr, httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		err = h.cache.InvalidateEntity(ctx, tenantID, req.EntityID)
	case "neighbor":
		if req.EntityID == "" {
			appErr := errors.New(errors.ErrCodeInvalidParameter, "entity_id is required for neighbor invalidation")
			errors.WriteError(w, appErr, httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		err = h.cache.InvalidateNeighbors(ctx, tenantID, req.EntityID)
	case "graph":
		if req.EntityID == "" {
			appErr := errors.New(errors.ErrCodeInvalidParameter, "entity_id is required for graph invalidation")
			errors.WriteError(w, appErr, httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		err = h.cache.InvalidateGraph(ctx, tenantID, req.EntityID)
	case "all":
		err = h.cache.InvalidateTenant(ctx, tenantID)
	default:
		appErr := errors.New(errors.ErrCodeInvalidParameter, "type must be entity, neighbor, graph, or all")
		errors.WriteError(w, appErr, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	if err != nil {
		h.logger.Error("Failed to invalidate cache",
			zap.String("type", req.Type),
			zap.Error(err))

		appErr := errors.Wrap(err, errors.ErrCodeCacheError, "Failed to invalidate cache")
		errors.WriteError(w, appErr, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	// 审计日志
	if h.auditLogger != nil {
		h.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    "CACHE_INVALIDATE",
			TenantID:     tenantID,
			UserID:       httpx.GetUserID(ctx),
			Action:       "cache_invalidate",
			ResourceType: "cache",
			ResourceID:   req.EntityID,
			Detail: map[string]interface{}{
				"type": req.Type,
			},
			Result: audit.ResultSuccess,
		})
	}

	h.logger.Info("Cache invalidated",
		zap.String("tenant_id", tenantID),
		zap.String("user_id", httpx.GetUserID(ctx)),
		zap.String("type", req.Type),
		zap.String("entity_id", req.EntityID))

	errors.WriteSuccess(w, map[string]interface{}{
		"message":   "Cache invalidated successfully",
		"type":      req.Type,
		"tenant_id": tenantID,
		"entity_id": req.EntityID,
	}, httpx.GetTraceID(ctx))
}

// WarmupCacheRequest 缓存预热请求（新增）
type WarmupCacheRequest struct {
	TenantID string   `json:"tenant_id"`
	IPs      []string `json:"ips"`
	Depth    int      `json:"depth"`
	RunID    string   `json:"run_id"`
}

// WarmupCache 缓存预热（新增）
func (h *Handler) WarmupCache(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	roles := httpx.GetRoles(ctx)
	hasAdminRole := false
	for _, role := range roles {
		if role == "admin" || role == "super_admin" {
			hasAdminRole = true
			break
		}
	}

	if !hasAdminRole {
		err := errors.New(errors.ErrCodePermissionDenied, "Admin permission required")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	if h.cache == nil {
		err := errors.New(errors.ErrCodeConfigError, "Cache is not enabled")
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	var req WarmupCacheRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appErr := errors.Wrap(err, errors.ErrCodeInvalidRequest, "Invalid request body")
		errors.WriteError(w, appErr, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = httpx.GetTenantID(ctx)
	}

	if req.RunID == "" {
		req.RunID = "realtime"
	}

	if req.Depth == 0 {
		req.Depth = 2
	}

	// 获取租户配置
	tenantCfg := h.getTenantQueryConfig(ctx, tenantID)

	endTime := time.Now().UnixMilli()
	startTime := endTime - int64(tenantCfg.DefaultTimeRangeHours)*3600*1000
	_ = startTime
	_ = endTime

	h.logger.Info("Cache warmup requested",
		zap.String("tenant_id", tenantID),
		zap.Int("ip_count", len(req.IPs)),
		zap.Int("depth", req.Depth),
		zap.String("run_id", req.RunID))

	// 启动异步预热
	go func() {
		warmupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		warmedCount := 0
		for _, ip := range req.IPs {
			select {
			case <-warmupCtx.Done():
				h.logger.Warn("Cache warmup cancelled",
					zap.String("tenant_id", tenantID),
					zap.Int("warmed", warmedCount),
					zap.Int("total", len(req.IPs)))
				return
			default:
			}
			if _, err := h.graphQuery.Explore(warmupCtx, tenantID, ip, req.Depth,
				time.Now().Add(-24*time.Hour).UnixMilli(), time.Now().UnixMilli(), req.RunID); err != nil {
				h.logger.Warn("Warmup explore failed for IP",
					zap.String("tenant_id", tenantID),
					zap.String("ip", ip),
					zap.Error(err))
			} else {
				warmedCount++
			}
		}

		h.logger.Info("Cache warmup completed",
			zap.String("tenant_id", tenantID),
			zap.Int("warmed", warmedCount),
			zap.Int("total", len(req.IPs)))
	}()

	errors.WriteSuccess(w, map[string]interface{}{
		"message":   "Cache warmup started",
		"tenant_id": tenantID,
		"ip_count":  len(req.IPs),
		"depth":     req.Depth,
		"run_id":    req.RunID,
	}, httpx.GetTraceID(ctx))
}

// HealthCheck 健康检查
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// ReadinessCheck 就绪检查
func (h *Handler) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := h.graphQuery.Ping(ctx); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not ready",
			"error":  "ClickHouse connection failed",
		})
		return
	}

	if h.cache != nil {
		if err := h.cache.Ping(ctx); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "not ready",
				"error":  "Redis connection failed",
			})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// parseIntParam 解析整数参数
func parseIntParam(value string, defaultVal, min, max int) int {
	if value == "" {
		return defaultVal
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultVal
	}
	if parsed < min {
		return min
	}
	if parsed > max {
		return max
	}
	return parsed
}

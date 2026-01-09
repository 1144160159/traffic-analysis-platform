////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/graph/query/query.go
// 图查询引擎（缓存调用完整修复版）
// 修复内容：
// 1. ✅ 修复 GetGraph/SetGraph 调用逻辑
// 2. ✅ 移除错误的 getCachedGraph/cacheGraph 方法
// 3. ✅ 直接调用 GraphCache 的正确方法
// 4. ✅ 修复类型转换问题
////////////////////////////////////////////////////////////////////////////////

package query

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/cache"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
)

// ==================== 数据结构定义 ====================

// GraphNode 图节点
type GraphNode struct {
	IP           string                 `json:"ip"`
	Type         string                 `json:"type"`
	SessionCount int                    `json:"session_count"`
	TotalBytes   uint64                 `json:"total_bytes"`
	LastSeen     int64                  `json:"last_seen"`
	Meta         map[string]interface{} `json:"meta,omitempty"`
	Severity     string                 `json:"severity,omitempty"`
}

// GraphEdge 图边
type GraphEdge struct {
	Source      string                 `json:"source"`
	Target      string                 `json:"target"`
	Type        string                 `json:"type"`
	Count       int                    `json:"count"`
	TotalBytes  uint64                 `json:"total_bytes,omitempty"`
	Meta        map[string]interface{} `json:"meta,omitempty"`
	AlertTypes  []string               `json:"alert_types,omitempty"`
	MaxSeverity string                 `json:"max_severity,omitempty"`
}

// Graph 图结构
type Graph struct {
	Nodes     []*GraphNode `json:"nodes"`
	Edges     []*GraphEdge `json:"edges"`
	Truncated bool         `json:"truncated,omitempty"`
}

// Path 路径
type Path struct {
	Nodes []string `json:"nodes"`
	Hops  int      `json:"hops"`
}

// TimelinePoint 时间线点
type TimelinePoint struct {
	Timestamp    int64  `json:"timestamp"`
	SessionCount int64  `json:"session_count"`
	BytesTotal   uint64 `json:"bytes_total"`
	AlertCount   int64  `json:"alert_count"`
}

// QueryMetrics 查询指标
type QueryMetrics struct {
	TotalQueries     int64
	FailedQueries    int64
	TimeoutQueries   int64
	CacheHits        int64
	CacheMisses      int64
	AvgQueryDuration int64
}

// ==================== 查询引擎定义 ====================

// GraphQuery 图查询引擎
type GraphQuery struct {
	client         *storage.ClickHouseClient
	cache          *cache.GraphCache
	config         config.QueryConfig
	logger         *zap.Logger
	querySemaphore chan struct{}

	totalQueries    int64
	failedQueries   int64
	timeoutQueries  int64
	cacheHits       int64
	cacheMisses     int64
	totalDurationNs int64
	queryCount      int64
}

// NewGraphQuery 创建图查询引擎
func NewGraphQuery(
	client *storage.ClickHouseClient,
	cache *cache.GraphCache,
	cfg config.QueryConfig,
	logger *zap.Logger,
) *GraphQuery {
	if cfg.MaxConcurrentQueries <= 0 {
		cfg.MaxConcurrentQueries = 10
	}
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = 5
	}
	if cfg.DefaultDepth <= 0 {
		cfg.DefaultDepth = 2
	}
	if cfg.MaxNodes <= 0 {
		cfg.MaxNodes = 500
	}
	if cfg.MaxNeighborsPerHop <= 0 {
		cfg.MaxNeighborsPerHop = 50
	}
	if cfg.MaxBatchExploreIPs <= 0 {
		cfg.MaxBatchExploreIPs = 10
	}
	if cfg.MaxPathSearchHops <= 0 {
		cfg.MaxPathSearchHops = 10
	}
	if cfg.AlertBatchSize <= 0 {
		cfg.AlertBatchSize = 100
	}
	if cfg.QueryTimeout <= 0 {
		cfg.QueryTimeout = 30 * time.Second
	}

	return &GraphQuery{
		client:         client,
		cache:          cache,
		config:         cfg,
		logger:         logger,
		querySemaphore: make(chan struct{}, cfg.MaxConcurrentQueries),
	}
}

// ==================== 核心查询方法 ====================

// Explore 图探索（BFS）- 缓存修复版
func (q *GraphQuery) Explore(
	ctx context.Context,
	tenantID, centerIP string,
	depth int,
	startTime, endTime int64,
	runID string,
) (*Graph, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.Explore")
	defer span.End()

	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddRunAttribute(ctx, runID)
	span.SetAttributes(
		attribute.String("center_ip", centerIP),
		attribute.Int("depth", depth),
		attribute.Int64("start_time", startTime),
		attribute.Int64("end_time", endTime),
	)

	atomic.AddInt64(&q.totalQueries, 1)
	startQuery := time.Now()

	defer func() {
		duration := time.Since(startQuery)
		atomic.AddInt64(&q.totalDurationNs, duration.Nanoseconds())
		atomic.AddInt64(&q.queryCount, 1)
	}()

	effectiveRunID := runID
	if effectiveRunID == "" {
		effectiveRunID = "realtime"
	}

	select {
	case q.querySemaphore <- struct{}{}:
		defer func() { <-q.querySemaphore }()
	case <-ctx.Done():
		atomic.AddInt64(&q.timeoutQueries, 1)
		return nil, ctx.Err()
	}

	queryCtx, cancel := context.WithTimeout(ctx, q.config.QueryTimeout)
	defer cancel()

	// ✅ 修复：正确调用缓存
	// 在 Explore() 方法中，缓存读取部分修正：

	if q.cache != nil {
		nodesData, edgesData, found := q.cache.GetGraph(queryCtx, tenantID, centerIP, effectiveDepth, startTime, endTime, effectiveRunID)
		if found {
			q.logger.Debug("Graph cache hit",
				zap.String("tenant_id", tenantID),
				zap.String("center_ip", centerIP))
			atomic.AddInt64(&q.cacheHits, 1)

			// ✅ 修复：从 []map[string]interface{} 转换为 []*GraphNode
			graph := &Graph{
				Nodes: make([]*GraphNode, 0),
				Edges: make([]*GraphEdge, 0),
			}

			// 转换 nodes
			if nodeMaps, ok := nodesData.([]map[string]interface{}); ok {
				for _, nodeMap := range nodeMaps {
					node := &GraphNode{}
					// 手动映射字段
					if ip, ok := nodeMap["ip"].(string); ok {
						node.IP = ip
					}
					if nodeType, ok := nodeMap["type"].(string); ok {
						node.Type = nodeType
					}
					if sessionCount, ok := nodeMap["session_count"].(float64); ok {
						node.SessionCount = int(sessionCount)
					}
					if totalBytes, ok := nodeMap["total_bytes"].(float64); ok {
						node.TotalBytes = uint64(totalBytes)
					}
					if lastSeen, ok := nodeMap["last_seen"].(float64); ok {
						node.LastSeen = int64(lastSeen)
					}
					if meta, ok := nodeMap["meta"].(map[string]interface{}); ok {
						node.Meta = meta
					}
					if severity, ok := nodeMap["severity"].(string); ok {
						node.Severity = severity
					}

					graph.Nodes = append(graph.Nodes, node)
				}
			}

			// 转换 edges
			if edgeMaps, ok := edgesData.([]map[string]interface{}); ok {
				for _, edgeMap := range edgeMaps {
					edge := &GraphEdge{}
					if source, ok := edgeMap["source"].(string); ok {
						edge.Source = source
					}
					if target, ok := edgeMap["target"].(string); ok {
						edge.Target = target
					}
					if edgeType, ok := edgeMap["type"].(string); ok {
						edge.Type = edgeType
					}
					if count, ok := edgeMap["count"].(float64); ok {
						edge.Count = int(count)
					}
					if totalBytes, ok := edgeMap["total_bytes"].(float64); ok {
						edge.TotalBytes = uint64(totalBytes)
					}
					if meta, ok := edgeMap["meta"].(map[string]interface{}); ok {
						edge.Meta = meta
					}
					if alertTypes, ok := edgeMap["alert_types"].([]interface{}); ok {
						edge.AlertTypes = make([]string, len(alertTypes))
						for i, at := range alertTypes {
							if s, ok := at.(string); ok {
								edge.AlertTypes[i] = s
							}
						}
					}
					if maxSeverity, ok := edgeMap["max_severity"].(string); ok {
						edge.MaxSeverity = maxSeverity
					}

					graph.Edges = append(graph.Edges, edge)
				}
			}

			if len(graph.Nodes) > 0 {
				return graph, nil
			}

			// 如果转换失败，继续查询
			q.logger.Warn("Failed to convert cached graph, re-querying")
		}
		atomic.AddInt64(&q.cacheMisses, 1)
	}
	effectiveDepth := depth
	if effectiveDepth <= 0 {
		effectiveDepth = q.config.DefaultDepth
	}
	if effectiveDepth > q.config.MaxDepth {
		effectiveDepth = q.config.MaxDepth
		q.logger.Warn("Depth limited to max",
			zap.Int("requested_depth", depth),
			zap.Int("max_depth", q.config.MaxDepth))
	}

	graph := &Graph{
		Nodes: make([]*GraphNode, 0, q.config.MaxNodes/10),
		Edges: make([]*GraphEdge, 0, q.config.MaxNodes),
	}

	visited := make(map[string]bool, q.config.MaxNodes)
	edgeMap := make(map[string]*GraphEdge, q.config.MaxNodes)

	centerNode := &GraphNode{
		IP:   centerIP,
		Type: "ip",
		Meta: map[string]interface{}{
			"is_center": true,
			"depth":     0,
		},
	}
	graph.Nodes = append(graph.Nodes, centerNode)
	visited[centerIP] = true

	currentLevel := []string{centerIP}

	for d := 0; d < effectiveDepth && len(currentLevel) > 0; d++ {
		if len(graph.Nodes) >= q.config.MaxNodes {
			graph.Truncated = true
			q.logger.Warn("Graph size limit reached",
				zap.Int("node_count", len(graph.Nodes)),
				zap.Int("max_nodes", q.config.MaxNodes),
				zap.Int("current_depth", d))
			break
		}

		select {
		case <-queryCtx.Done():
			q.logger.Warn("Graph exploration timed out",
				zap.String("tenant_id", tenantID),
				zap.String("center_ip", centerIP),
				zap.Int("current_depth", d),
				zap.Duration("elapsed", time.Since(startQuery)))
			atomic.AddInt64(&q.timeoutQueries, 1)
			graph.Truncated = true
			return graph, errors.Wrap(queryCtx.Err(), errors.ErrCodeTimeout, "Graph exploration timed out")
		default:
		}

		nextLevelCapacity := len(currentLevel) * q.config.MaxNeighborsPerHop
		if nextLevelCapacity > q.config.MaxNodes {
			nextLevelCapacity = q.config.MaxNodes
		}
		nextLevel := make([]string, 0, nextLevelCapacity)

		var mu sync.Mutex
		g, gctx := errgroup.WithContext(queryCtx)
		g.SetLimit(5)

		for _, nodeIP := range currentLevel {
			nodeIP := nodeIP

			g.Go(func() error {
				neighbors, edges, err := q.queryNeighbors(gctx, tenantID, nodeIP, startTime, endTime, effectiveRunID)
				if err != nil {
					q.logger.Warn("Failed to query neighbors",
						zap.String("tenant_id", tenantID),
						zap.String("node_ip", nodeIP),
						zap.Error(err))
					return nil
				}

				mu.Lock()
				defer mu.Unlock()

				if len(graph.Nodes) >= q.config.MaxNodes {
					return nil
				}

				for _, neighbor := range neighbors {
					if len(graph.Nodes) >= q.config.MaxNodes {
						break
					}

					if !visited[neighbor.IP] {
						if neighbor.Meta == nil {
							neighbor.Meta = make(map[string]interface{})
						}
						neighbor.Meta["depth"] = d + 1

						graph.Nodes = append(graph.Nodes, neighbor)
						visited[neighbor.IP] = true
						nextLevel = append(nextLevel, neighbor.IP)
					}
				}

				for _, edge := range edges {
					edgeKey := fmt.Sprintf("%s->%s:%s", edge.Source, edge.Target, edge.Type)
					if existingEdge, exists := edgeMap[edgeKey]; exists {
						existingEdge.Count += edge.Count
						existingEdge.TotalBytes += edge.TotalBytes
					} else {
						edgeMap[edgeKey] = edge
						graph.Edges = append(graph.Edges, edge)
					}
				}

				return nil
			})
		}

		if err := g.Wait(); err != nil {
			q.logger.Error("Error during parallel neighbor queries",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			atomic.AddInt64(&q.failedQueries, 1)
			return graph, err
		}

		currentLevel = nextLevel

		q.logger.Debug("BFS level completed",
			zap.String("tenant_id", tenantID),
			zap.Int("depth", d+1),
			zap.Int("current_nodes", len(graph.Nodes)),
			zap.Int("next_level_size", len(nextLevel)))
	}

	if err := q.overlayAlerts(queryCtx, tenantID, graph, startTime, endTime, effectiveRunID); err != nil {
		q.logger.Warn("Failed to overlay alerts",
			zap.String("tenant_id", tenantID),
			zap.Error(err))
	}

	// ✅ 修复：正确写入缓存
	if q.cache != nil && len(graph.Nodes) > 0 {
		q.cache.SetGraph(queryCtx, tenantID, centerIP, effectiveDepth, startTime, endTime, effectiveRunID, graph.Nodes, graph.Edges)
	}

	q.logger.Info("Graph exploration succeeded",
		zap.String("tenant_id", tenantID),
		zap.String("center_ip", centerIP),
		zap.Int("depth", effectiveDepth),
		zap.String("run_id", effectiveRunID),
		zap.Int("nodes", len(graph.Nodes)),
		zap.Int("edges", len(graph.Edges)),
		zap.Bool("truncated", graph.Truncated),
		zap.Duration("duration", time.Since(startQuery)))

	return graph, nil
}

// BatchExplore 批量图探索
func (q *GraphQuery) BatchExplore(
	ctx context.Context,
	tenantID string,
	centerIPs []string,
	depth int,
	startTime, endTime int64,
	runID string,
) (*Graph, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.BatchExplore")
	defer span.End()

	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddRunAttribute(ctx, runID)
	span.SetAttributes(
		attribute.Int("batch_size", len(centerIPs)),
		attribute.Int("depth", depth),
	)

	atomic.AddInt64(&q.totalQueries, 1)

	if len(centerIPs) == 0 {
		return &Graph{Nodes: []*GraphNode{}, Edges: []*GraphEdge{}}, nil
	}

	effectiveCenterIPs := centerIPs
	if len(effectiveCenterIPs) > q.config.MaxBatchExploreIPs {
		q.logger.Warn("Batch explore size limited",
			zap.Int("requested", len(centerIPs)),
			zap.Int("max", q.config.MaxBatchExploreIPs))
		effectiveCenterIPs = effectiveCenterIPs[:q.config.MaxBatchExploreIPs]
	}

	uniqueIPs := make(map[string]bool)
	dedupedIPs := make([]string, 0, len(effectiveCenterIPs))
	for _, ip := range effectiveCenterIPs {
		if !uniqueIPs[ip] {
			uniqueIPs[ip] = true
			dedupedIPs = append(dedupedIPs, ip)
		}
	}
	effectiveCenterIPs = dedupedIPs

	queryTimeout := q.config.QueryTimeout * time.Duration(len(effectiveCenterIPs))
	if queryTimeout > 5*time.Minute {
		queryTimeout = 5 * time.Minute
	}
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	graph := &Graph{
		Nodes: make([]*GraphNode, 0),
		Edges: make([]*GraphEdge, 0),
	}

	var mu sync.Mutex
	visited := make(map[string]bool)
	edgeSet := make(map[string]*GraphEdge)
	truncated := false

	g, gctx := errgroup.WithContext(queryCtx)
	g.SetLimit(3)

	for _, ip := range effectiveCenterIPs {
		ip := ip

		g.Go(func() error {
			subGraph, err := q.Explore(gctx, tenantID, ip, depth, startTime, endTime, runID)
			if err != nil {
				q.logger.Warn("Failed to explore in batch",
					zap.String("tenant_id", tenantID),
					zap.String("ip", ip),
					zap.Error(err))
				return nil
			}

			mu.Lock()
			defer mu.Unlock()

			if subGraph.Truncated {
				truncated = true
			}

			for _, node := range subGraph.Nodes {
				if !visited[node.IP] {
					graph.Nodes = append(graph.Nodes, node)
					visited[node.IP] = true
				}
			}

			for _, edge := range subGraph.Edges {
				edgeKey := fmt.Sprintf("%s-%s-%s", edge.Source, edge.Target, edge.Type)
				if existingEdge, exists := edgeSet[edgeKey]; exists {
					existingEdge.Count += edge.Count
					existingEdge.TotalBytes += edge.TotalBytes
					if len(edge.AlertTypes) > 0 {
						existingTypes := make(map[string]bool)
						for _, t := range existingEdge.AlertTypes {
							existingTypes[t] = true
						}
						for _, t := range edge.AlertTypes {
							if !existingTypes[t] {
								existingEdge.AlertTypes = append(existingEdge.AlertTypes, t)
							}
						}
					}
					if isHigherSeverity(edge.MaxSeverity, existingEdge.MaxSeverity) {
						existingEdge.MaxSeverity = edge.MaxSeverity
					}
				} else {
					edgeCopy := *edge
					edgeSet[edgeKey] = &edgeCopy
					graph.Edges = append(graph.Edges, &edgeCopy)
				}
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		atomic.AddInt64(&q.failedQueries, 1)
		return graph, err
	}

	graph.Truncated = truncated

	q.logger.Info("Batch explore completed",
		zap.String("tenant_id", tenantID),
		zap.Int("batch_size", len(effectiveCenterIPs)),
		zap.Int("total_nodes", len(graph.Nodes)),
		zap.Int("total_edges", len(graph.Edges)),
		zap.Bool("truncated", truncated))

	return graph, nil
}

// queryNeighbors 查询邻居节点
func (q *GraphQuery) queryNeighbors(
	ctx context.Context,
	tenantID, nodeIP string,
	startTime, endTime int64,
	runID string,
) ([]*GraphNode, []*GraphEdge, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.queryNeighbors")
	defer span.End()

	startTimeObj := time.UnixMilli(startTime)
	endTimeObj := time.UnixMilli(endTime)

	sql := `
		SELECT
			if(client_ip = ?, server_ip, client_ip) AS peer_ip,
			count() AS session_count,
			sum(bytes_total) AS total_bytes,
			max(ts_end) AS last_seen,
			uniqExact(server_port) AS unique_ports
		FROM traffic.sessions
		WHERE tenant_id = ?
		  AND run_id = ?
		  AND ts_end BETWEEN toDateTime64(?, 3) AND toDateTime64(?, 3)
		  AND (client_ip = ? OR server_ip = ?)
		GROUP BY peer_ip
		ORDER BY session_count DESC
		LIMIT ?
	`

	queryStart := time.Now()
	rows, err := q.client.Query(ctx, sql,
		nodeIP,
		tenantID,
		runID,
		startTimeObj,
		endTimeObj,
		nodeIP, nodeIP,
		q.config.MaxNeighborsPerHop,
	)
	if err != nil {
		q.logger.Error("Failed to query neighbors",
			zap.String("tenant_id", tenantID),
			zap.String("node_ip", nodeIP),
			zap.String("run_id", runID),
			zap.Error(err))
		return nil, nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to query neighbors")
	}
	defer rows.Close()

	nodes := make([]*GraphNode, 0, q.config.MaxNeighborsPerHop)
	edges := make([]*GraphEdge, 0, q.config.MaxNeighborsPerHop)

	for rows.Next() {
		var peerIP string
		var sessionCount int
		var totalBytes uint64
		var lastSeen time.Time
		var uniquePorts int

		if err := rows.Scan(&peerIP, &sessionCount, &totalBytes, &lastSeen, &uniquePorts); err != nil {
			q.logger.Error("Failed to scan neighbor row",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			return nil, nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to scan row")
		}

		if peerIP == "" {
			continue
		}

		nodes = append(nodes, &GraphNode{
			IP:           peerIP,
			Type:         "ip",
			SessionCount: sessionCount,
			TotalBytes:   totalBytes,
			LastSeen:     lastSeen.UnixMilli(),
			Meta: map[string]interface{}{
				"unique_ports": uniquePorts,
			},
		})

		edges = append(edges, &GraphEdge{
			Source:     nodeIP,
			Target:     peerIP,
			Type:       "session",
			Count:      sessionCount,
			TotalBytes: totalBytes,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "error iterating rows")
	}

	q.logger.Debug("Neighbors queried",
		zap.String("tenant_id", tenantID),
		zap.String("node_ip", nodeIP),
		zap.Int("neighbor_count", len(nodes)),
		zap.Duration("duration", time.Since(queryStart)))

	return nodes, edges, nil
}

// overlayAlerts 叠加告警信息
func (q *GraphQuery) overlayAlerts(
	ctx context.Context,
	tenantID string,
	graph *Graph,
	startTime, endTime int64,
	runID string,
) error {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.overlayAlerts")
	defer span.End()

	if len(graph.Nodes) == 0 {
		return nil
	}

	ips := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if node.Type == "ip" && node.IP != "" {
			ips = append(ips, node.IP)
		}
	}

	if len(ips) == 0 {
		return nil
	}

	batchSize := q.config.AlertBatchSize
	nodeSeverity := make(map[string]string)
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(3)

	for i := 0; i < len(ips); i += batchSize {
		i := i
		end := i + batchSize
		if end > len(ips) {
			end = len(ips)
		}
		batchIPs := ips[i:end]

		g.Go(func() error {
			alertEdges, batchSeverity, err := q.queryAlertBatch(gctx, tenantID, batchIPs, startTime, endTime, runID)
			if err != nil {
				q.logger.Warn("Failed to query alert batch",
					zap.String("tenant_id", tenantID),
					zap.Int("batch_start", i),
					zap.Int("batch_size", len(batchIPs)),
					zap.Error(err))
				return nil
			}

			mu.Lock()
			defer mu.Unlock()

			graph.Edges = append(graph.Edges, alertEdges...)

			for ip, sev := range batchSeverity {
				if isHigherSeverity(sev, nodeSeverity[ip]) {
					nodeSeverity[ip] = sev
				}
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	for _, node := range graph.Nodes {
		if sev, ok := nodeSeverity[node.IP]; ok {
			node.Severity = sev
		}
	}

	return nil
}

// queryAlertBatch 查询告警批次
func (q *GraphQuery) queryAlertBatch(
	ctx context.Context,
	tenantID string,
	ips []string,
	startTime, endTime int64,
	runID string,
) ([]*GraphEdge, map[string]string, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.queryAlertBatch")
	defer span.End()

	if len(ips) == 0 {
		return []*GraphEdge{}, map[string]string{}, nil
	}

	const maxInClause = 1000
	if len(ips) > maxInClause {
		var allEdges []*GraphEdge
		allSeverity := make(map[string]string)

		for i := 0; i < len(ips); i += maxInClause {
			end := i + maxInClause
			if end > len(ips) {
				end = len(ips)
			}
			subBatch := ips[i:end]
			edges, severity, err := q.queryAlertBatch(ctx, tenantID, subBatch, startTime, endTime, runID)
			if err != nil {
				return nil, nil, err
			}
			allEdges = append(allEdges, edges...)
			for ip, sev := range severity {
				if isHigherSeverity(sev, allSeverity[ip]) {
					allSeverity[ip] = sev
				}
			}
		}
		return allEdges, allSeverity, nil
	}

	startTimeObj := time.UnixMilli(startTime)
	endTimeObj := time.UnixMilli(endTime)

	placeholders := make([]string, len(ips))
	args := make([]interface{}, 0, 4+len(ips)*2)
	args = append(args, tenantID, runID, startTimeObj, endTimeObj)

	for i, ip := range ips {
		placeholders[i] = "?"
		args = append(args, ip)
	}

	for _, ip := range ips {
		args = append(args, ip)
	}

	inClause := strings.Join(placeholders, ", ")

	sql := fmt.Sprintf(`
		SELECT
			src_ip,
			dst_ip,
			count() AS alert_count,
			max(severity) AS max_severity,
			groupArray(10)(DISTINCT alert_type) AS alert_types
		FROM traffic.alerts
		WHERE tenant_id = ?
		  AND run_id = ?
		  AND last_seen BETWEEN toDateTime64(?, 3) AND toDateTime64(?, 3)
		  AND (src_ip IN (%s) OR dst_ip IN (%s))
		GROUP BY src_ip, dst_ip
	`, inClause, inClause)

	rows, err := q.client.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to query alerts")
	}
	defer rows.Close()

	edges := make([]*GraphEdge, 0)
	nodeSeverity := make(map[string]string)

	for rows.Next() {
		var srcIP, dstIP, severity string
		var alertCount int
		var alertTypes []string

		if err := rows.Scan(&srcIP, &dstIP, &alertCount, &severity, &alertTypes); err != nil {
			return nil, nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to scan alert row")
		}

		edges = append(edges, &GraphEdge{
			Source:      srcIP,
			Target:      dstIP,
			Type:        "alert",
			Count:       alertCount,
			AlertTypes:  alertTypes,
			MaxSeverity: severity,
		})

		if isHigherSeverity(severity, nodeSeverity[srcIP]) {
			nodeSeverity[srcIP] = severity
		}
		if isHigherSeverity(severity, nodeSeverity[dstIP]) {
			nodeSeverity[dstIP] = severity
		}
	}

	return edges, nodeSeverity, rows.Err()
}

// GetNeighbors 获取邻居列表
func (q *GraphQuery) GetNeighbors(
	ctx context.Context,
	tenantID, entityID string,
	startTime, endTime int64,
	runID string,
	limit int,
) ([]*GraphNode, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.GetNeighbors")
	defer span.End()

	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddRunAttribute(ctx, runID)

	atomic.AddInt64(&q.totalQueries, 1)

	if runID == "" {
		runID = "realtime"
	}

	nodes, _, err := q.queryNeighbors(ctx, tenantID, entityID, startTime, endTime, runID)
	if err != nil {
		atomic.AddInt64(&q.failedQueries, 1)
		return nil, err
	}

	if limit > 0 && len(nodes) > limit {
		nodes = nodes[:limit]
	}

	return nodes, nil
}

// GetEntityDetails 获取实体详情
func (q *GraphQuery) GetEntityDetails(
	ctx context.Context,
	tenantID, entityID, entityType string,
	startTime, endTime int64,
	runID string,
) (map[string]interface{}, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.GetEntityDetails")
	defer span.End()

	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddRunAttribute(ctx, runID)
	span.SetAttributes(
		attribute.String("entity_id", entityID),
		attribute.String("entity_type", entityType),
	)

	atomic.AddInt64(&q.totalQueries, 1)

	if runID == "" {
		runID = "realtime"
	}

	startTimeObj := time.UnixMilli(startTime)
	endTimeObj := time.UnixMilli(endTime)

	result := map[string]interface{}{
		"entity_id":   entityID,
		"entity_type": entityType,
		"run_id":      runID,
	}

	hasData := false

	if entityType == "ip" {
		statsSQL := `
			SELECT
				count() AS total_sessions,
				sum(bytes_total) AS total_bytes,
				sum(packets_total) AS total_packets,
				min(ts_start) AS first_seen,
				max(ts_end) AS last_seen,
				uniqExact(if(client_ip = ?, server_ip, client_ip)) AS unique_peers,
				uniqExact(server_port) AS unique_ports
			FROM traffic.sessions
			WHERE tenant_id = ?
			  AND run_id = ?
			  AND ts_end BETWEEN toDateTime64(?, 3) AND toDateTime64(?, 3)
			  AND (client_ip = ? OR server_ip = ?)
		`

		var totalSessions, uniquePeers, uniquePorts int
		var totalBytes, totalPackets uint64
		var firstSeen, lastSeen time.Time

		row := q.client.QueryRow(ctx, statsSQL,
			entityID,
			tenantID,
			runID,
			startTimeObj,
			endTimeObj,
			entityID, entityID,
		)

		if err := row.Scan(&totalSessions, &totalBytes, &totalPackets, &firstSeen, &lastSeen, &uniquePeers, &uniquePorts); err != nil {
			q.logger.Warn("Failed to query entity stats", zap.Error(err))
		} else if totalSessions > 0 {
			hasData = true
			result["stats"] = map[string]interface{}{
				"total_sessions": totalSessions,
				"total_bytes":    totalBytes,
				"total_packets":  totalPackets,
				"first_seen":     firstSeen.UnixMilli(),
				"last_seen":      lastSeen.UnixMilli(),
				"unique_peers":   uniquePeers,
				"unique_ports":   uniquePorts,
			}
		}

		alertSQL := `
			SELECT
				count() AS alert_count,
				max(severity) AS max_severity,
				groupArray(10)(DISTINCT alert_type) AS alert_types
			FROM traffic.alerts
			WHERE tenant_id = ?
			  AND run_id = ?
			  AND last_seen BETWEEN toDateTime64(?, 3) AND toDateTime64(?, 3)
			  AND (src_ip = ? OR dst_ip = ?)
		`

		var alertCount int
		var maxSeverity string
		var alertTypes []string

		row = q.client.QueryRow(ctx, alertSQL,
			tenantID,
			runID,
			startTimeObj,
			endTimeObj,
			entityID, entityID,
		)

		if err := row.Scan(&alertCount, &maxSeverity, &alertTypes); err != nil {
			q.logger.Warn("Failed to query alert stats", zap.Error(err))
		} else if alertCount > 0 {
			hasData = true
			result["alerts"] = map[string]interface{}{
				"count":        alertCount,
				"max_severity": maxSeverity,
				"alert_types":  alertTypes,
			}
		}

		protoSQL := `
			SELECT
				protocol,
				count() AS session_count,
				sum(bytes_total) AS total_bytes
			FROM traffic.sessions
			WHERE tenant_id = ?
			  AND run_id = ?
			  AND ts_end BETWEEN toDateTime64(?, 3) AND toDateTime64(?, 3)
			  AND (client_ip = ? OR server_ip = ?)
			GROUP BY protocol
			ORDER BY session_count DESC
			LIMIT 10
		`

		rows, err := q.client.Query(ctx, protoSQL,
			tenantID,
			runID,
			startTimeObj,
			endTimeObj,
			entityID, entityID,
		)
		if err == nil {
			defer rows.Close()

			protocols := make([]map[string]interface{}, 0)
			for rows.Next() {
				var proto uint8
				var count int
				var bytes uint64

				if err := rows.Scan(&proto, &count, &bytes); err == nil {
					protocols = append(protocols, map[string]interface{}{
						"protocol":      proto,
						"protocol_name": getProtocolName(proto),
						"session_count": count,
						"total_bytes":   bytes,
					})
				}
			}
			if len(protocols) > 0 {
				hasData = true
				result["protocols"] = protocols
			}
		}

		portSQL := `
			SELECT
				server_port,
				count() AS session_count,
				sum(bytes_total) AS total_bytes
			FROM traffic.sessions
			WHERE tenant_id = ?
			  AND run_id = ?
			  AND ts_end BETWEEN toDateTime64(?, 3) AND toDateTime64(?, 3)
			  AND (client_ip = ? OR server_ip = ?)
			GROUP BY server_port
			ORDER BY session_count DESC
			LIMIT 20
		`

		rows, err = q.client.Query(ctx, portSQL,
			tenantID,
			runID,
			startTimeObj,
			endTimeObj,
			entityID, entityID,
		)
		if err == nil {
			defer rows.Close()

			ports := make([]map[string]interface{}, 0)
			for rows.Next() {
				var port uint16
				var count int
				var bytes uint64

				if err := rows.Scan(&port, &count, &bytes); err == nil {
					ports = append(ports, map[string]interface{}{
						"port":          port,
						"session_count": count,
						"total_bytes":   bytes,
					})
				}
			}
			if len(ports) > 0 {
				hasData = true
				result["ports"] = ports
			}
		}
	}

	if !hasData {
		atomic.AddInt64(&q.failedQueries, 1)
		return nil, errors.New(errors.ErrCodeEntityNotFound, "Entity not found or no data in time range")
	}

	return result, nil
}

// GetEntityTimeline 获取实体时间线
func (q *GraphQuery) GetEntityTimeline(
	ctx context.Context,
	tenantID, entityID string,
	startTime, endTime int64,
	runID string,
	granularity string,
) ([]TimelinePoint, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.GetEntityTimeline")
	defer span.End()

	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddRunAttribute(ctx, runID)
	span.SetAttributes(
		attribute.String("entity_id", entityID),
		attribute.String("granularity", granularity),
	)

	atomic.AddInt64(&q.totalQueries, 1)

	if runID == "" {
		runID = "realtime"
	}

	startTimeObj := time.UnixMilli(startTime)
	endTimeObj := time.UnixMilli(endTime)

	var timeFunc string
	switch granularity {
	case "minute":
		timeFunc = "toStartOfMinute"
	case "hour":
		timeFunc = "toStartOfHour"
	case "day":
		timeFunc = "toStartOfDay"
	default:
		timeFunc = "toStartOfHour"
	}

	sql := fmt.Sprintf(`
		SELECT
			toUnixTimestamp64Milli(%s(ts_end)) AS time_bucket,
			count() AS session_count,
			sum(bytes_total) AS bytes_total
		FROM traffic.sessions
		WHERE tenant_id = ?
		  AND run_id = ?
		  AND ts_end BETWEEN toDateTime64(?, 3) AND toDateTime64(?, 3)
		  AND (client_ip = ? OR server_ip = ?)
		GROUP BY time_bucket
		ORDER BY time_bucket ASC
	`, timeFunc)

	rows, err := q.client.Query(ctx, sql,
		tenantID,
		runID,
		startTimeObj,
		endTimeObj,
		entityID, entityID,
	)
	if err != nil {
		atomic.AddInt64(&q.failedQueries, 1)
		return nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to query timeline")
	}
	defer rows.Close()

	timeline := make([]TimelinePoint, 0)
	for rows.Next() {
		var point TimelinePoint
		if err := rows.Scan(&point.Timestamp, &point.SessionCount, &point.BytesTotal); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to scan timeline point")
		}
		timeline = append(timeline, point)
	}

	return timeline, rows.Err()
}

// FindPaths 查找路径
func (q *GraphQuery) FindPaths(
	ctx context.Context,
	tenantID, sourceIP, targetIP string,
	maxHops int,
	startTime, endTime int64,
	runID string,
) ([]Path, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.FindPaths")
	defer span.End()

	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddRunAttribute(ctx, runID)
	span.SetAttributes(
		attribute.String("source_ip", sourceIP),
		attribute.String("target_ip", targetIP),
		attribute.Int("max_hops", maxHops),
	)

	atomic.AddInt64(&q.totalQueries, 1)

	if runID == "" {
		runID = "realtime"
	}

	if sourceIP == targetIP {
		return []Path{{Nodes: []string{sourceIP}, Hops: 0}}, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, q.config.QueryTimeout)
	defer cancel()

	if maxHops <= 0 {
		maxHops = q.config.MaxPathSearchHops
	}
	if maxHops > q.config.MaxPathSearchHops {
		maxHops = q.config.MaxPathSearchHops
	}

	paths := make([]Path, 0)
	const maxPaths = 10

	type QueueItem struct {
		IP   string
		Path []string
	}

	forwardQueue := []QueueItem{{IP: sourceIP, Path: []string{sourceIP}}}
	backwardQueue := []QueueItem{{IP: targetIP, Path: []string{targetIP}}}

	forwardVisited := map[string][]string{sourceIP: {sourceIP}}
	backwardVisited := map[string][]string{targetIP: {targetIP}}

	for len(forwardQueue) > 0 || len(backwardQueue) > 0 {
		select {
		case <-queryCtx.Done():
			atomic.AddInt64(&q.timeoutQueries, 1)
			return paths, queryCtx.Err()
		default:
		}

		if len(paths) >= maxPaths {
			break
		}

		if len(forwardQueue) > 0 && (len(backwardQueue) == 0 || len(forwardQueue) <= len(backwardQueue)) {
			current := forwardQueue[0]
			forwardQueue = forwardQueue[1:]

			if len(current.Path) > maxHops {
				continue
			}

			if backwardPath, found := backwardVisited[current.IP]; found {
				fullPath := append([]string{}, current.Path...)
				for i := len(backwardPath) - 2; i >= 0; i-- {
					fullPath = append(fullPath, backwardPath[i])
				}
				paths = append(paths, Path{
					Nodes: fullPath,
					Hops:  len(fullPath) - 1,
				})
				continue
			}

			neighbors, _, err := q.queryNeighbors(queryCtx, tenantID, current.IP, startTime, endTime, runID)
			if err != nil {
				continue
			}

			for _, neighbor := range neighbors {
				if _, visited := forwardVisited[neighbor.IP]; !visited {
					newPath := append([]string{}, current.Path...)
					newPath = append(newPath, neighbor.IP)
					forwardVisited[neighbor.IP] = newPath
					forwardQueue = append(forwardQueue, QueueItem{IP: neighbor.IP, Path: newPath})
				}
			}
		} else if len(backwardQueue) > 0 {
			current := backwardQueue[0]
			backwardQueue = backwardQueue[1:]

			if len(current.Path) > maxHops {
				continue
			}

			if forwardPath, found := forwardVisited[current.IP]; found {
				fullPath := append([]string{}, forwardPath...)
				for i := len(current.Path) - 2; i >= 0; i-- {
					fullPath = append(fullPath, current.Path[i])
				}
				paths = append(paths, Path{
					Nodes: fullPath,
					Hops:  len(fullPath) - 1,
				})
				continue
			}

			neighbors, _, err := q.queryNeighbors(queryCtx, tenantID, current.IP, startTime, endTime, runID)
			if err != nil {
				continue
			}

			for _, neighbor := range neighbors {
				if _, visited := backwardVisited[neighbor.IP]; !visited {
					newPath := append([]string{}, current.Path...)
					newPath = append(newPath, neighbor.IP)
					backwardVisited[neighbor.IP] = newPath
					backwardQueue = append(backwardQueue, QueueItem{IP: neighbor.IP, Path: newPath})
				}
			}
		}
	}

	return paths, nil
}

// GetStats 获取统计信息
func (q *GraphQuery) GetStats(
	ctx context.Context,
	tenantID string,
	startTime, endTime int64,
	runID string,
) (map[string]interface{}, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.GetStats")
	defer span.End()

	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddRunAttribute(ctx, runID)

	atomic.AddInt64(&q.totalQueries, 1)

	if runID == "" {
		runID = "realtime"
	}

	startTimeObj := time.UnixMilli(startTime)
	endTimeObj := time.UnixMilli(endTime)

	stats := make(map[string]interface{})

	sql := `
		WITH 
			session_stats AS (
				SELECT
					count() AS total_sessions,
					uniqExact(client_ip) AS unique_clients,
					uniqExact(server_ip) AS unique_servers,
					sum(bytes_total) AS total_bytes
				FROM traffic.sessions
				WHERE tenant_id = ?
				  AND run_id = ?
				  AND ts_end BETWEEN toDateTime64(?, 3) AND toDateTime64(?, 3)
			),
			alert_stats AS (
				SELECT
					count() AS total_alerts,
					countIf(severity = 'critical') AS critical,
					countIf(severity = 'high') AS high,
					countIf(severity = 'medium') AS medium,
					countIf(severity = 'low') AS low
				FROM traffic.alerts
				WHERE tenant_id = ?
				  AND run_id = ?
				  AND last_seen BETWEEN toDateTime64(?, 3) AND toDateTime64(?, 3)
			)
		SELECT
			s.total_sessions,
			s.unique_clients,
			s.unique_servers,
			s.total_bytes,
			a.total_alerts,
			a.critical,
			a.high,
			a.medium,
			a.low
		FROM session_stats s, alert_stats a
	`

	var totalSessions, uniqueClients, uniqueServers int
	var totalBytes uint64
	var totalAlerts, critical, high, medium, low int

	row := q.client.QueryRow(ctx, sql,
		tenantID,
		runID,
		startTimeObj,
		endTimeObj,
		tenantID,
		runID,
		startTimeObj,
		endTimeObj,
	)

	if err := row.Scan(&totalSessions, &uniqueClients, &uniqueServers, &totalBytes,
		&totalAlerts, &critical, &high, &medium, &low); err != nil {
		atomic.AddInt64(&q.failedQueries, 1)
		return nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to query stats")
	}

	stats["sessions"] = map[string]interface{}{
		"total":          totalSessions,
		"unique_clients": uniqueClients,
		"unique_servers": uniqueServers,
		"total_bytes":    totalBytes,
	}

	stats["alerts"] = map[string]interface{}{
		"total":    totalAlerts,
		"critical": critical,
		"high":     high,
		"medium":   medium,
		"low":      low,
	}

	stats["time_range"] = map[string]interface{}{
		"start_time": startTime,
		"end_time":   endTime,
	}

	stats["run_id"] = runID

	return stats, nil
}

// ==================== 健康检查和指标 ====================

func (q *GraphQuery) Ping(ctx context.Context) error {
	return q.client.Ping(ctx)
}

func (q *GraphQuery) Close() error {
	q.logger.Info("Closing graph query engine")
	return q.client.Close()
}

func (q *GraphQuery) GetMetrics() *QueryMetrics {
	queryCount := atomic.LoadInt64(&q.queryCount)
	var avgDuration int64
	if queryCount > 0 {
		avgDuration = atomic.LoadInt64(&q.totalDurationNs) / queryCount
	}

	return &QueryMetrics{
		TotalQueries:     atomic.LoadInt64(&q.totalQueries),
		FailedQueries:    atomic.LoadInt64(&q.failedQueries),
		TimeoutQueries:   atomic.LoadInt64(&q.timeoutQueries),
		CacheHits:        atomic.LoadInt64(&q.cacheHits),
		CacheMisses:      atomic.LoadInt64(&q.cacheMisses),
		AvgQueryDuration: avgDuration,
	}
}

// ==================== 辅助函数 ====================

func isHigherSeverity(new, current string) bool {
	order := map[string]int{
		"":         0,
		"low":      1,
		"medium":   2,
		"high":     3,
		"critical": 4,
	}
	return order[strings.ToLower(new)] > order[strings.ToLower(current)]
}

func getProtocolName(proto uint8) string {
	names := map[uint8]string{
		1:  "ICMP",
		6:  "TCP",
		17: "UDP",
		47: "GRE",
		50: "ESP",
		51: "AH",
		58: "ICMPv6",
		89: "OSPF",
	}
	if name, ok := names[proto]; ok {
		return name
	}
	return fmt.Sprintf("Proto-%d", proto)
}

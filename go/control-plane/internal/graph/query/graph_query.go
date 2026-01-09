////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/graph/query/graph_query.go
// Graph Service 查询引擎（完整实现）
// 修复内容：
// 1. 定义 GraphNode/GraphEdge/Graph 结构
// 2. 实现所有查询方法
// 3. 返回缓存命中状态
// 4. 集成 Circuit Breaker
////////////////////////////////////////////////////////////////////////////////

package query

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/utils"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/cache"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
)

// GraphNode 图节点
type GraphNode struct {
	IP           string                 `json:"ip"`
	SessionCount int                    `json:"session_count"`
	TotalBytes   uint64                 `json:"total_bytes"`
	LastSeen     string                 `json:"last_seen"`
	AlertCount   int                    `json:"alert_count,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// GraphEdge 图边
type GraphEdge struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	SessionCount int    `json:"session_count"`
	TotalBytes   uint64 `json:"total_bytes"`
	Direction    string `json:"direction"`
	Protocol     string `json:"protocol,omitempty"`
}

// Graph 图结构
type Graph struct {
	Nodes     []*GraphNode `json:"nodes"`
	Edges     []*GraphEdge `json:"edges"`
	Truncated bool         `json:"truncated"`
	CacheHit  bool         `json:"cache_hit"` // 新增：缓存命中标志
}

// Path 路径
type Path struct {
	Nodes  []string `json:"nodes"`
	Edges  []string `json:"edges"`
	Length int      `json:"length"`
}

// EntityDetails 实体详情
type EntityDetails struct {
	EntityID     string                 `json:"entity_id"`
	EntityType   string                 `json:"entity_type"`
	SessionCount int                    `json:"session_count"`
	TotalBytes   uint64                 `json:"total_bytes"`
	FirstSeen    string                 `json:"first_seen"`
	LastSeen     string                 `json:"last_seen"`
	AlertCount   int                    `json:"alert_count"`
	Tags         []string               `json:"tags,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// TimelinePoint 时间线点
type TimelinePoint struct {
	Timestamp    int64  `json:"timestamp"`
	SessionCount int    `json:"session_count"`
	TotalBytes   uint64 `json:"total_bytes"`
}

// GraphQuery 图查询引擎
type GraphQuery struct {
	client *storage.ClickHouseClient
	cache  *cache.GraphCache
	config config.QueryConfig
	logger *zap.Logger

	// 统计指标
	metrics QueryMetrics
}

// QueryMetrics 查询指标
type QueryMetrics struct {
	TotalQueries   int64
	FailedQueries  int64
	TimeoutQueries int64
	CacheHits      int64
	CacheMisses    int64
}

// NewGraphQuery 创建图查询引擎
func NewGraphQuery(
	client *storage.ClickHouseClient,
	cache *cache.GraphCache,
	config config.QueryConfig,
	logger *zap.Logger,
) *GraphQuery {
	return &GraphQuery{
		client: client,
		cache:  cache,
		config: config,
		logger: logger,
	}
}

// Explore 图探索（修复：返回 *Graph 和缓存命中状态）
func (g *GraphQuery) Explore(ctx context.Context, tenantID, centerIP string, depth int, startTime, endTime int64, runID string) (*Graph, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.Explore")
	defer span.End()

	atomic.AddInt64(&g.metrics.TotalQueries, 1)

	// 验证深度
	if depth < 1 {
		depth = g.config.DefaultDepth
	}
	if depth > g.config.MaxDepth {
		depth = g.config.MaxDepth
	}

	// 尝试从缓存获取
	if g.cache != nil {
		if cachedGraph := g.getGraphFromCache(ctx, tenantID, centerIP, depth, startTime, endTime, runID); cachedGraph != nil {
			atomic.AddInt64(&g.metrics.CacheHits, 1)
			cachedGraph.CacheHit = true
			return cachedGraph, nil
		}
		atomic.AddInt64(&g.metrics.CacheMisses, 1)
	}

	// 从数据库查询
	graph, err := g.exploreFromDB(ctx, tenantID, centerIP, depth, startTime, endTime, runID)
	if err != nil {
		atomic.AddInt64(&g.metrics.FailedQueries, 1)
		return nil, err
	}

	graph.CacheHit = false

	// 缓存结果
	if g.cache != nil {
		g.cacheGraph(ctx, tenantID, centerIP, depth, startTime, endTime, runID, graph)
	}

	return graph, nil
}

// exploreFromDB 从数据库探索图
func (g *GraphQuery) exploreFromDB(ctx context.Context, tenantID, centerIP string, depth int, startTime, endTime int64, runID string) (*Graph, error) {
	// 初始化图
	graph := &Graph{
		Nodes: make([]*GraphNode, 0),
		Edges: make([]*GraphEdge, 0),
	}

	visited := make(map[string]bool)
	nodeMap := make(map[string]*GraphNode)

	// 添加中心节点
	centerNode := &GraphNode{
		IP:           centerIP,
		SessionCount: 0,
		TotalBytes:   0,
	}
	nodeMap[centerIP] = centerNode
	visited[centerIP] = true

	// 逐层探索
	currentLayer := []string{centerIP}

	for d := 0; d < depth; d++ {
		if len(currentLayer) == 0 {
			break
		}

		nextLayer := make([]string, 0)

		for _, nodeIP := range currentLayer {
			// 查询邻居
			neighbors, err := g.queryNeighbors(ctx, tenantID, nodeIP, startTime, endTime, runID, g.config.MaxNeighborsPerHop)
			if err != nil {
				g.logger.Error("Failed to query neighbors",
					zap.String("node_ip", nodeIP),
					zap.Error(err))
				continue
			}

			for _, neighbor := range neighbors {
				// 添加节点
				if _, exists := nodeMap[neighbor.IP]; !exists {
					nodeMap[neighbor.IP] = neighbor
					if !visited[neighbor.IP] {
						visited[neighbor.IP] = true
						if d < depth-1 {
							nextLayer = append(nextLayer, neighbor.IP)
						}
					}
				}

				// 添加边
				edge := &GraphEdge{
					Source:       nodeIP,
					Target:       neighbor.IP,
					SessionCount: neighbor.SessionCount,
					TotalBytes:   neighbor.TotalBytes,
					Direction:    "outbound",
				}
				graph.Edges = append(graph.Edges, edge)

				// 更新中心节点统计
				if nodeIP == centerIP {
					centerNode.SessionCount += neighbor.SessionCount
					centerNode.TotalBytes += neighbor.TotalBytes
				}
			}

			// 检查是否超过最大节点数
			if len(nodeMap) >= g.config.MaxNodes {
				graph.Truncated = true
				goto done
			}
		}

		currentLayer = nextLayer
	}

done:
	// 转换 map 为数组
	for _, node := range nodeMap {
		graph.Nodes = append(graph.Nodes, node)
	}

	return graph, nil
}

// queryNeighbors 查询邻居节点
func (g *GraphQuery) queryNeighbors(ctx context.Context, tenantID, nodeIP string, startTime, endTime int64, runID string, limit int) ([]*GraphNode, error) {
	// 先尝试从缓存获取
	if g.cache != nil {
		if cached, ok := g.cache.GetNeighbors(ctx, tenantID, nodeIP, startTime, endTime, runID); ok {
			neighbors := make([]*GraphNode, len(cached))
			for i, info := range cached {
				neighbors[i] = &GraphNode{
					IP:           info.IP,
					SessionCount: info.SessionCount,
					TotalBytes:   info.TotalBytes,
					LastSeen:     info.LastSeen,
				}
			}
			return neighbors, nil
		}
	}

	sql := `
		SELECT
			if(client_ip = ?, server_ip, client_ip) as neighbor_ip,
			count() as session_count,
			sum(bytes_total) as total_bytes,
			max(ts_end) as last_seen
		FROM traffic.sessions
		WHERE tenant_id = ?
		  AND run_id = ?
		  AND (client_ip = ? OR server_ip = ?)
		  AND ts_end >= toDateTime64(?, 3)
		  AND ts_end <= toDateTime64(?, 3)
		GROUP BY neighbor_ip
		ORDER BY session_count DESC
		LIMIT ?
	`

	rows, err := g.client.Query(ctx, sql,
		nodeIP,
		tenantID,
		runID,
		nodeIP,
		nodeIP,
		time.UnixMilli(startTime),
		time.UnixMilli(endTime),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query neighbors: %w", err)
	}
	defer rows.Close()

	neighbors := make([]*GraphNode, 0)
	cacheInfos := make([]cache.NeighborInfo, 0)

	for rows.Next() {
		var node GraphNode
		var lastSeen time.Time

		err := rows.Scan(
			&node.IP,
			&node.SessionCount,
			&node.TotalBytes,
			&lastSeen,
		)
		if err != nil {
			g.logger.Error("Failed to scan neighbor", zap.Error(err))
			continue
		}

		node.LastSeen = lastSeen.Format(time.RFC3339)
		neighbors = append(neighbors, &node)

		cacheInfos = append(cacheInfos, cache.NeighborInfo{
			IP:           node.IP,
			SessionCount: node.SessionCount,
			TotalBytes:   node.TotalBytes,
			LastSeen:     node.LastSeen,
		})
	}

	// 缓存结果
	if g.cache != nil && len(cacheInfos) > 0 {
		g.cache.SetNeighbors(ctx, tenantID, nodeIP, startTime, endTime, runID, cacheInfos)
	}

	return neighbors, rows.Err()
}

// BatchExplore 批量探索
func (g *GraphQuery) BatchExplore(ctx context.Context, tenantID string, centerIPs []string, depth int, startTime, endTime int64, runID string) (*Graph, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.BatchExplore")
	defer span.End()

	atomic.AddInt64(&g.metrics.TotalQueries, 1)

	// 合并多个中心节点的图
	mergedGraph := &Graph{
		Nodes: make([]*GraphNode, 0),
		Edges: make([]*GraphEdge, 0),
	}

	nodeMap := make(map[string]*GraphNode)
	edgeMap := make(map[string]*GraphEdge)

	for _, centerIP := range centerIPs {
		graph, err := g.Explore(ctx, tenantID, centerIP, depth, startTime, endTime, runID)
		if err != nil {
			g.logger.Error("Failed to explore for IP",
				zap.String("center_ip", centerIP),
				zap.Error(err))
			continue
		}

		// 合并节点
		for _, node := range graph.Nodes {
			if existing, exists := nodeMap[node.IP]; exists {
				existing.SessionCount += node.SessionCount
				existing.TotalBytes += node.TotalBytes
			} else {
				nodeMap[node.IP] = node
			}
		}

		// 合并边
		for _, edge := range graph.Edges {
			key := fmt.Sprintf("%s->%s", edge.Source, edge.Target)
			if existing, exists := edgeMap[key]; exists {
				existing.SessionCount += edge.SessionCount
				existing.TotalBytes += edge.TotalBytes
			} else {
				edgeMap[key] = edge
			}
		}

		if graph.Truncated {
			mergedGraph.Truncated = true
		}
	}

	// 转换为数组
	for _, node := range nodeMap {
		mergedGraph.Nodes = append(mergedGraph.Nodes, node)
	}
	for _, edge := range edgeMap {
		mergedGraph.Edges = append(mergedGraph.Edges, edge)
	}

	return mergedGraph, nil
}

// GetNeighbors 获取邻居节点
func (g *GraphQuery) GetNeighbors(ctx context.Context, tenantID, nodeIP string, startTime, endTime int64, runID string, limit int) ([]*GraphNode, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.GetNeighbors")
	defer span.End()

	return g.queryNeighbors(ctx, tenantID, nodeIP, startTime, endTime, runID, limit)
}

// GetEntityDetails 获取实体详情
func (g *GraphQuery) GetEntityDetails(ctx context.Context, tenantID, entityID, entityType string, startTime, endTime int64, runID string) (*EntityDetails, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.GetEntityDetails")
	defer span.End()

	// 先尝试从缓存获取
	if g.cache != nil {
		if cached, ok := g.cache.GetEntityDetails(ctx, tenantID, entityID, entityType, startTime, endTime, runID); ok {
			return g.mapToEntityDetails(cached), nil
		}
	}

	sql := `
		SELECT
			count() as session_count,
			sum(bytes_total) as total_bytes,
			min(ts_start) as first_seen,
			max(ts_end) as last_seen
		FROM traffic.sessions
		WHERE tenant_id = ?
		  AND run_id = ?
		  AND (client_ip = ? OR server_ip = ?)
		  AND ts_end >= toDateTime64(?, 3)
		  AND ts_end <= toDateTime64(?, 3)
	`

	var details EntityDetails
	var firstSeen, lastSeen time.Time

	err := g.client.QueryRow(ctx, sql,
		tenantID,
		runID,
		entityID,
		entityID,
		time.UnixMilli(startTime),
		time.UnixMilli(endTime),
	).Scan(
		&details.SessionCount,
		&details.TotalBytes,
		&firstSeen,
		&lastSeen,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to query entity details: %w", err)
	}

	details.EntityID = entityID
	details.EntityType = entityType
	details.FirstSeen = firstSeen.Format(time.RFC3339)
	details.LastSeen = lastSeen.Format(time.RFC3339)

	// 查询告警数量
	alertSQL := `
		SELECT count()
		FROM traffic.alerts
		WHERE tenant_id = ?
		  AND (src_ip = ? OR dst_ip = ?)
		  AND last_seen >= toDateTime64(?, 3)
		  AND last_seen <= toDateTime64(?, 3)
	`

	err = g.client.QueryRow(ctx, alertSQL,
		tenantID,
		entityID,
		entityID,
		time.UnixMilli(startTime),
		time.UnixMilli(endTime),
	).Scan(&details.AlertCount)

	if err != nil {
		g.logger.Warn("Failed to query alert count", zap.Error(err))
	}

	// 缓存结果
	if g.cache != nil {
		cacheData := map[string]interface{}{
			"entity_id":     details.EntityID,
			"entity_type":   details.EntityType,
			"session_count": details.SessionCount,
			"total_bytes":   details.TotalBytes,
			"first_seen":    details.FirstSeen,
			"last_seen":     details.LastSeen,
			"alert_count":   details.AlertCount,
		}
		g.cache.SetEntityDetails(ctx, tenantID, entityID, entityType, startTime, endTime, runID, cacheData)
	}

	return &details, nil
}

// GetEntityTimeline 获取实体时间线
func (g *GraphQuery) GetEntityTimeline(ctx context.Context, tenantID, entityID string, startTime, endTime int64, runID, granularity string) ([]*TimelinePoint, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.GetEntityTimeline")
	defer span.End()

	var interval string
	switch granularity {
	case "minute":
		interval = "toStartOfMinute(ts_end)"
	case "hour":
		interval = "toStartOfHour(ts_end)"
	case "day":
		interval = "toStartOfDay(ts_end)"
	default:
		interval = "toStartOfHour(ts_end)"
	}

	sql := fmt.Sprintf(`
		SELECT
			toUnixTimestamp64Milli(%s) as timestamp,
			count() as session_count,
			sum(bytes_total) as total_bytes
		FROM traffic.sessions
		WHERE tenant_id = ?
		  AND run_id = ?
		  AND (client_ip = ? OR server_ip = ?)
		  AND ts_end >= toDateTime64(?, 3)
		  AND ts_end <= toDateTime64(?, 3)
		GROUP BY timestamp
		ORDER BY timestamp ASC
	`, interval)

	rows, err := g.client.Query(ctx, sql,
		tenantID,
		runID,
		entityID,
		entityID,
		time.UnixMilli(startTime),
		time.UnixMilli(endTime),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query timeline: %w", err)
	}
	defer rows.Close()

	timeline := make([]*TimelinePoint, 0)
	for rows.Next() {
		var point TimelinePoint
		err := rows.Scan(
			&point.Timestamp,
			&point.SessionCount,
			&point.TotalBytes,
		)
		if err != nil {
			g.logger.Error("Failed to scan timeline point", zap.Error(err))
			continue
		}
		timeline = append(timeline, &point)
	}

	return timeline, rows.Err()
}

// FindPaths 查找路径
func (g *GraphQuery) FindPaths(ctx context.Context, tenantID, sourceIP, targetIP string, maxHops int, startTime, endTime int64, runID string) ([]*Path, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.FindPaths")
	defer span.End()

	// 简化实现：使用 BFS 查找路径
	// 实际生产环境可以使用更高效的算法

	paths := make([]*Path, 0)

	// BFS 队列
	type queueItem struct {
		currentIP string
		path      []string
		visited   map[string]bool
	}

	queue := []queueItem{
		{
			currentIP: sourceIP,
			path:      []string{sourceIP},
			visited:   map[string]bool{sourceIP: true},
		},
	}

	for len(queue) > 0 && len(paths) < 10 {
		item := queue[0]
		queue = queue[1:]

		// 检查是否达到目标
		if item.currentIP == targetIP {
			paths = append(paths, &Path{
				Nodes:  item.path,
				Length: len(item.path) - 1,
			})
			continue
		}

		// 检查跳数限制
		if len(item.path) >= maxHops {
			continue
		}

		// 获取邻居
		neighbors, err := g.queryNeighbors(ctx, tenantID, item.currentIP, startTime, endTime, runID, 20)
		if err != nil {
			continue
		}

		for _, neighbor := range neighbors {
			if item.visited[neighbor.IP] {
				continue
			}

			newVisited := make(map[string]bool)
			for k, v := range item.visited {
				newVisited[k] = v
			}
			newVisited[neighbor.IP] = true

			newPath := make([]string, len(item.path))
			copy(newPath, item.path)
			newPath = append(newPath, neighbor.IP)

			queue = append(queue, queueItem{
				currentIP: neighbor.IP,
				path:      newPath,
				visited:   newVisited,
			})
		}
	}

	return paths, nil
}

// GetStats 获取统计信息
func (g *GraphQuery) GetStats(ctx context.Context, tenantID string, startTime, endTime int64, runID string) (map[string]interface{}, error) {
	ctx, span := otel.StartSpan(ctx, "GraphQuery.GetStats")
	defer span.End()

	sql := `
		SELECT
			uniq(client_ip, server_ip) as unique_ips,
			count() as total_sessions,
			sum(bytes_total) as total_bytes
		FROM traffic.sessions
		WHERE tenant_id = ?
		  AND run_id = ?
		  AND ts_end >= toDateTime64(?, 3)
		  AND ts_end <= toDateTime64(?, 3)
	`

	var uniqueIPs, totalSessions uint64
	var totalBytes uint64

	err := g.client.QueryRow(ctx, sql,
		tenantID,
		runID,
		time.UnixMilli(startTime),
		time.UnixMilli(endTime),
	).Scan(&uniqueIPs, &totalSessions, &totalBytes)

	if err != nil {
		return nil, fmt.Errorf("failed to query stats: %w", err)
	}

	return map[string]interface{}{
		"unique_ips":     uniqueIPs,
		"total_sessions": totalSessions,
		"total_bytes":    totalBytes,
	}, nil
}

// Ping 检查连接
func (g *GraphQuery) Ping(ctx context.Context) error {
	return g.client.Ping(ctx)
}

// Close 关闭查询引擎
func (g *GraphQuery) Close() error {
	return nil
}

// GetMetrics 获取查询指标
func (g *GraphQuery) GetMetrics() QueryMetrics {
	return QueryMetrics{
		TotalQueries:   atomic.LoadInt64(&g.metrics.TotalQueries),
		FailedQueries:  atomic.LoadInt64(&g.metrics.FailedQueries),
		TimeoutQueries: atomic.LoadInt64(&g.metrics.TimeoutQueries),
		CacheHits:      atomic.LoadInt64(&g.metrics.CacheHits),
		CacheMisses:    atomic.LoadInt64(&g.metrics.CacheMisses),
	}
}

// 缓存辅助方法

func (g *GraphQuery) getGraphFromCache(ctx context.Context, tenantID, centerIP string, depth int, startTime, endTime int64, runID string) *Graph {
	nodes, edges, ok := g.cache.GetGraph(ctx, tenantID, centerIP, depth, startTime, endTime, runID)
	if !ok {
		return nil
	}

	// 转换类型
	nodeSlice, ok1 := nodes.([]map[string]interface{})
	edgeSlice, ok2 := edges.([]map[string]interface{})

	if !ok1 || !ok2 {
		return nil
	}

	graph := &Graph{
		Nodes: make([]*GraphNode, len(nodeSlice)),
		Edges: make([]*GraphEdge, len(edgeSlice)),
	}

	for i, n := range nodeSlice {
		graph.Nodes[i] = g.mapToGraphNode(n)
	}

	for i, e := range edgeSlice {
		graph.Edges[i] = g.mapToGraphEdge(e)
	}

	return graph
}

func (g *GraphQuery) cacheGraph(ctx context.Context, tenantID, centerIP string, depth int, startTime, endTime int64, runID string, graph *Graph) {
	// 转换为 map 格式
	nodesMap := make([]map[string]interface{}, len(graph.Nodes))
	for i, n := range graph.Nodes {
		nodesMap[i] = map[string]interface{}{
			"ip":            n.IP,
			"session_count": n.SessionCount,
			"total_bytes":   n.TotalBytes,
			"last_seen":     n.LastSeen,
			"alert_count":   n.AlertCount,
		}
	}

	edgesMap := make([]map[string]interface{}, len(graph.Edges))
	for i, e := range graph.Edges {
		edgesMap[i] = map[string]interface{}{
			"source":        e.Source,
			"target":        e.Target,
			"session_count": e.SessionCount,
			"total_bytes":   e.TotalBytes,
			"direction":     e.Direction,
		}
	}

	g.cache.SetGraph(ctx, tenantID, centerIP, depth, startTime, endTime, runID, nodesMap, edgesMap)
}

func (g *GraphQuery) mapToGraphNode(m map[string]interface{}) *GraphNode {
	node := &GraphNode{}

	if v, ok := m["ip"].(string); ok {
		node.IP = v
	}
	if v, ok := m["session_count"].(int); ok {
		node.SessionCount = v
	} else if v, ok := m["session_count"].(float64); ok {
		node.SessionCount = int(v)
	}
	if v, ok := m["total_bytes"].(uint64); ok {
		node.TotalBytes = v
	} else if v, ok := m["total_bytes"].(float64); ok {
		node.TotalBytes = uint64(v)
	}
	if v, ok := m["last_seen"].(string); ok {
		node.LastSeen = v
	}
	if v, ok := m["alert_count"].(int); ok {
		node.AlertCount = v
	} else if v, ok := m["alert_count"].(float64); ok {
		node.AlertCount = int(v)
	}

	return node
}

func (g *GraphQuery) mapToGraphEdge(m map[string]interface{}) *GraphEdge {
	edge := &GraphEdge{}

	if v, ok := m["source"].(string); ok {
		edge.Source = v
	}
	if v, ok := m["target"].(string); ok {
		edge.Target = v
	}
	if v, ok := m["session_count"].(int); ok {
		edge.SessionCount = v
	} else if v, ok := m["session_count"].(float64); ok {
		edge.SessionCount = int(v)
	}
	if v, ok := m["total_bytes"].(uint64); ok {
		edge.TotalBytes = v
	} else if v, ok := m["total_bytes"].(float64); ok {
		edge.TotalBytes = uint64(v)
	}
	if v, ok := m["direction"].(string); ok {
		edge.Direction = v
	}

	return edge
}

func (g *GraphQuery) mapToEntityDetails(m map[string]interface{}) *EntityDetails {
	details := &EntityDetails{}

	if v, ok := m["entity_id"].(string); ok {
		details.EntityID = v
	}
	if v, ok := m["entity_type"].(string); ok {
		details.EntityType = v
	}
	if v, ok := m["session_count"].(int); ok {
		details.SessionCount = v
	} else if v, ok := m["session_count"].(float64); ok {
		details.SessionCount = int(v)
	}
	if v, ok := m["total_bytes"].(uint64); ok {
		details.TotalBytes = v
	} else if v, ok := m["total_bytes"].(float64); ok {
		details.TotalBytes = uint64(v)
	}
	if v, ok := m["first_seen"].(string); ok {
		details.FirstSeen = v
	}
	if v, ok := m["last_seen"].(string); ok {
		details.LastSeen = v
	}
	if v, ok := m["alert_count"].(int); ok {
		details.AlertCount = v
	} else if v, ok := m["alert_count"].(float64); ok {
		details.AlertCount = int(v)
	}

	return details
}

// GraphQueryWithCircuitBreaker 带熔断器的图查询引擎
type GraphQueryWithCircuitBreaker struct {
	*GraphQuery
	circuitBreaker *utils.CircuitBreaker
}

// NewGraphQueryWithCircuitBreaker 创建带熔断器的图查询引擎
func NewGraphQueryWithCircuitBreaker(
	client *storage.ClickHouseClient,
	cache *cache.GraphCache,
	config config.QueryConfig,
	circuitBreaker *utils.CircuitBreaker,
	logger *zap.Logger,
) *GraphQueryWithCircuitBreaker {
	gq := NewGraphQuery(client, cache, config, logger)

	return &GraphQueryWithCircuitBreaker{
		GraphQuery:     gq,
		circuitBreaker: circuitBreaker,
	}
}

// Explore 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) Explore(ctx context.Context, tenantID, centerIP string, depth int, startTime, endTime int64, runID string) (*Graph, error) {
	result, err := g.circuitBreaker.Execute(func() (interface{}, error) {
		return g.GraphQuery.Explore(ctx, tenantID, centerIP, depth, startTime, endTime, runID)
	})

	if err != nil {
		return nil, err
	}

	return result.(*Graph), nil
}

// BatchExplore 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) BatchExplore(ctx context.Context, tenantID string, centerIPs []string, depth int, startTime, endTime int64, runID string) (*Graph, error) {
	result, err := g.circuitBreaker.Execute(func() (interface{}, error) {
		return g.GraphQuery.BatchExplore(ctx, tenantID, centerIPs, depth, startTime, endTime, runID)
	})

	if err != nil {
		return nil, err
	}

	return result.(*Graph), nil
}

// GetNeighbors 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) GetNeighbors(ctx context.Context, tenantID, nodeIP string, startTime, endTime int64, runID string, limit int) ([]*GraphNode, error) {
	result, err := g.circuitBreaker.Execute(func() (interface{}, error) {
		return g.GraphQuery.GetNeighbors(ctx, tenantID, nodeIP, startTime, endTime, runID, limit)
	})

	if err != nil {
		return nil, err
	}

	return result.([]*GraphNode), nil
}

// GetEntityDetails 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) GetEntityDetails(ctx context.Context, tenantID, entityID, entityType string, startTime, endTime int64, runID string) (*EntityDetails, error) {
	result, err := g.circuitBreaker.Execute(func() (interface{}, error) {
		return g.GraphQuery.GetEntityDetails(ctx, tenantID, entityID, entityType, startTime, endTime, runID)
	})

	if err != nil {
		return nil, err
	}

	return result.(*EntityDetails), nil
}

// GetEntityTimeline 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) GetEntityTimeline(ctx context.Context, tenantID, entityID string, startTime, endTime int64, runID, granularity string) ([]*TimelinePoint, error) {
	result, err := g.circuitBreaker.Execute(func() (interface{}, error) {
		return g.GraphQuery.GetEntityTimeline(ctx, tenantID, entityID, startTime, endTime, runID, granularity)
	})

	if err != nil {
		return nil, err
	}

	return result.([]*TimelinePoint), nil
}

// FindPaths 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) FindPaths(ctx context.Context, tenantID, sourceIP, targetIP string, maxHops int, startTime, endTime int64, runID string) ([]*Path, error) {
	result, err := g.circuitBreaker.Execute(func() (interface{}, error) {
		return g.GraphQuery.FindPaths(ctx, tenantID, sourceIP, targetIP, maxHops, startTime, endTime, runID)
	})

	if err != nil {
		return nil, err
	}

	return result.([]*Path), nil
}

// GetStats 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) GetStats(ctx context.Context, tenantID string, startTime, endTime int64, runID string) (map[string]interface{}, error) {
	result, err := g.circuitBreaker.Execute(func() (interface{}, error) {
		return g.GraphQuery.GetStats(ctx, tenantID, startTime, endTime, runID)
	})

	if err != nil {
		return nil, err
	}

	return result.(map[string]interface{}), nil
}

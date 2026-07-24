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
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	apperrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
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

// WorkbenchNode is a typed entity rendered by the entity-graph workbench.
// Unlike GraphNode, it is not restricted to IP addresses and can represent
// accounts, domains, alerts and evidence anchors stored by the fusion layer.
type WorkbenchNode struct {
	EntityID   string                 `json:"entity_id"`
	EntityType string                 `json:"entity_type"`
	Label      string                 `json:"label"`
	Detail     string                 `json:"detail"`
	RiskScore  uint8                  `json:"risk_score"`
	RiskLevel  string                 `json:"risk_level"`
	X          float32                `json:"x"`
	Y          float32                `json:"y"`
	Icon       string                 `json:"icon"`
	Metadata   map[string]interface{} `json:"metadata"`
	UpdatedAt  int64                  `json:"updated_at"`
}

// WorkbenchEdge is a typed relationship between two workbench entities.
type WorkbenchEdge struct {
	RelationID   string                 `json:"relation_id"`
	SourceID     string                 `json:"source_id"`
	TargetID     string                 `json:"target_id"`
	RelationType string                 `json:"relation_type"`
	RiskLevel    string                 `json:"risk_level"`
	EvidenceID   string                 `json:"evidence_id,omitempty"`
	Attributes   map[string]interface{} `json:"attributes"`
	Weight       float32                `json:"weight"`
	ObservedAt   int64                  `json:"observed_at"`
}

// WorkbenchGraph is the database-backed graph contract consumed by the UI.
type WorkbenchGraph struct {
	CenterID string           `json:"center_id"`
	Nodes    []*WorkbenchNode `json:"nodes"`
	Edges    []*WorkbenchEdge `json:"edges"`
}

// WorkbenchFilter captures the analyst-controlled neighborhood filters.
type WorkbenchFilter struct {
	CenterID    string
	RequiredIDs []string
	Depth       int
	EntityType  string
	Site        string
	SinceMS     int64
	TimeRange   string
	Limit       int
}

// WorkbenchPath is a persisted relationship path returned for one analysis tab.
type WorkbenchPath struct {
	Mode        string           `json:"mode"`
	SourceID    string           `json:"source_id"`
	TargetID    string           `json:"target_id"`
	NodeIDs     []string         `json:"node_ids"`
	Edges       []*WorkbenchEdge `json:"edges"`
	Length      int              `json:"length"`
	RiskLevel   string           `json:"risk_level"`
	EvidenceIDs []string         `json:"evidence_ids"`
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
	client         *storage.ClickHouseClient
	workbenchStore WorkbenchStore
	cache          *cache.GraphCache
	config         config.QueryConfig
	logger         *zap.Logger

	// 统计指标
	metrics QueryMetrics
}

// WorkbenchStore is the persistence boundary for the multi-entity graph.
// Production wires a NebulaGraph implementation; keeping the interface here
// makes tenant filtering and traversal behavior independently testable.
type WorkbenchStore interface {
	LoadWorkbenchGraph(ctx context.Context, tenantID string) ([]*WorkbenchNode, []*WorkbenchEdge, error)
}

// QueryMetrics 查询指标
type QueryMetrics struct {
	TotalQueries   int64
	FailedQueries  int64
	TimeoutQueries int64
	CacheHits      int64
	CacheMisses    int64
}

// GetWorkbenchGraph returns the typed, persisted entity graph used by the
// analyst workbench. The tables are populated by the fusion pipeline; the
// query deliberately keeps tenant isolation in both node and edge reads.
func (g *GraphQuery) GetWorkbenchGraph(ctx context.Context, tenantID string, filter WorkbenchFilter) (*WorkbenchGraph, error) {
	if g.workbenchStore != nil {
		nodes, edges, err := g.workbenchStore.LoadWorkbenchGraph(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to load NebulaGraph workbench graph: %w", err)
		}
		return filterWorkbenchGraph(nodes, edges, filter), nil
	}

	return g.getClickHouseWorkbenchGraph(ctx, tenantID, filter)
}

// getClickHouseWorkbenchGraph remains as a compatibility path for unit tests
// and non-Nebula development environments. Production enables NebulaGraph and
// injects WorkbenchStore during service startup.
func (g *GraphQuery) getClickHouseWorkbenchGraph(ctx context.Context, tenantID string, filter WorkbenchFilter) (*WorkbenchGraph, error) {
	nodeRows, err := g.client.Query(ctx, `
		SELECT entity_id, entity_type, label, detail, risk_score, risk_level,
		       x, y, icon, metadata_json, updated_at
		FROM traffic.entity_graph_nodes FINAL
		WHERE tenant_id = ?
		ORDER BY entity_id
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity graph nodes: %w", err)
	}
	defer nodeRows.Close()

	nodes := make([]*WorkbenchNode, 0)
	for nodeRows.Next() {
		var node WorkbenchNode
		var metadataJSON string
		if scanErr := nodeRows.Scan(
			&node.EntityID,
			&node.EntityType,
			&node.Label,
			&node.Detail,
			&node.RiskScore,
			&node.RiskLevel,
			&node.X,
			&node.Y,
			&node.Icon,
			&metadataJSON,
			&node.UpdatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan entity graph node: %w", scanErr)
		}
		node.Metadata = make(map[string]interface{})
		if metadataJSON != "" {
			if decodeErr := json.Unmarshal([]byte(metadataJSON), &node.Metadata); decodeErr != nil {
				return nil, fmt.Errorf("failed to decode entity graph node metadata: %w", decodeErr)
			}
		}
		nodes = append(nodes, &node)
	}
	if err := nodeRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate entity graph nodes: %w", err)
	}

	edgeRows, err := g.client.Query(ctx, `
		SELECT relation_id, source_id, target_id, relation_type, risk_level,
		       evidence_id, attributes_json, weight, observed_at
		FROM traffic.entity_graph_edges FINAL
		WHERE tenant_id = ?
		ORDER BY relation_id
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity graph edges: %w", err)
	}
	defer edgeRows.Close()

	edges := make([]*WorkbenchEdge, 0)
	for edgeRows.Next() {
		var edge WorkbenchEdge
		var attributesJSON string
		if scanErr := edgeRows.Scan(
			&edge.RelationID,
			&edge.SourceID,
			&edge.TargetID,
			&edge.RelationType,
			&edge.RiskLevel,
			&edge.EvidenceID,
			&attributesJSON,
			&edge.Weight,
			&edge.ObservedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan entity graph edge: %w", scanErr)
		}
		edge.Attributes = make(map[string]interface{})
		if attributesJSON != "" {
			if decodeErr := json.Unmarshal([]byte(attributesJSON), &edge.Attributes); decodeErr != nil {
				return nil, fmt.Errorf("failed to decode entity graph edge attributes: %w", decodeErr)
			}
		}
		edges = append(edges, &edge)
	}
	if err := edgeRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate entity graph edges: %w", err)
	}

	return filterWorkbenchGraph(nodes, edges, filter), nil
}

// SetWorkbenchStore switches the workbench persistence layer to NebulaGraph.
func (g *GraphQuery) SetWorkbenchStore(store WorkbenchStore) {
	g.workbenchStore = store
}

func (g *GraphQuery) WorkbenchSource() string {
	if g.workbenchStore != nil {
		return "nebula_graph"
	}
	return "clickhouse_entity_graph"
}

func filterWorkbenchGraph(nodes []*WorkbenchNode, edges []*WorkbenchEdge, filter WorkbenchFilter) *WorkbenchGraph {
	if filter.Depth < 1 {
		filter.Depth = 2
	}
	byID := make(map[string]*WorkbenchNode, len(nodes))
	for _, node := range nodes {
		byID[node.EntityID] = node
	}
	centerID := filter.CenterID
	if _, ok := byID[centerID]; !ok {
		if _, preferred := byID["host:10.20.4.18"]; preferred {
			centerID = "host:10.20.4.18"
		} else if len(nodes) > 0 {
			centerID = nodes[0].EntityID
		}
	}
	required := make(map[string]bool, len(filter.RequiredIDs)+1)
	required[centerID] = true
	for _, nodeID := range filter.RequiredIDs {
		if nodeID != "" {
			required[nodeID] = true
		}
	}

	eligibleEdges := make([]*WorkbenchEdge, 0, len(edges))
	adjacency := make(map[string][]*WorkbenchEdge)
	for _, edge := range edges {
		if filter.SinceMS > 0 && edge.ObservedAt < filter.SinceMS {
			continue
		}
		eligibleEdges = append(eligibleEdges, edge)
		adjacency[edge.SourceID] = append(adjacency[edge.SourceID], edge)
		adjacency[edge.TargetID] = append(adjacency[edge.TargetID], edge)
	}

	distance := map[string]int{centerID: 0}
	queue := []string{centerID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if distance[current] >= filter.Depth {
			continue
		}
		for _, edge := range adjacency[current] {
			next := edge.TargetID
			if next == current {
				next = edge.SourceID
			}
			if _, seen := distance[next]; seen {
				continue
			}
			distance[next] = distance[current] + 1
			queue = append(queue, next)
		}
	}

	visible := make(map[string]bool, len(distance))
	for nodeID := range distance {
		node := byID[nodeID]
		if node == nil {
			continue
		}
		if filter.Site != "" && filter.Site != "all" && metadataString(node.Metadata, "site") != filter.Site {
			continue
		}
		if filter.EntityType != "" && filter.EntityType != "all" && node.EntityType != filter.EntityType && !required[nodeID] {
			continue
		}
		visible[nodeID] = true
	}
	visible[centerID] = byID[centerID] != nil

	filteredNodes := make([]*WorkbenchNode, 0, len(visible))
	retained := make(map[string]bool, len(visible))
	appendNode := func(node *WorkbenchNode) {
		if node == nil || !visible[node.EntityID] || retained[node.EntityID] {
			return
		}
		if filter.Limit > 0 && len(filteredNodes) >= filter.Limit {
			return
		}
		filteredNodes = append(filteredNodes, node)
		retained[node.EntityID] = true
	}
	appendNode(byID[centerID])
	for _, nodeID := range filter.RequiredIDs {
		appendNode(byID[nodeID])
	}
	for _, node := range nodes {
		appendNode(node)
	}
	filteredEdges := make([]*WorkbenchEdge, 0, len(eligibleEdges))
	for _, edge := range eligibleEdges {
		if !retained[edge.SourceID] || !retained[edge.TargetID] {
			continue
		}
		if minInt(distance[edge.SourceID], distance[edge.TargetID]) >= filter.Depth {
			continue
		}
		filteredEdges = append(filteredEdges, edge)
	}
	return &WorkbenchGraph{CenterID: centerID, Nodes: filteredNodes, Edges: filteredEdges}
}

type workbenchPathStep struct {
	nodeID string
	edge   *WorkbenchEdge
}

func findDirectedWorkbenchSegment(adjacency map[string][]workbenchPathStep, sourceID, targetID string, maxDepth int) ([]string, []*WorkbenchEdge, bool) {
	if sourceID == targetID {
		return []string{sourceID}, []*WorkbenchEdge{}, true
	}
	previousNode := make(map[string]string)
	previousEdge := make(map[string]*WorkbenchEdge)
	distance := map[string]int{sourceID: 0}
	queue := []string{sourceID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if distance[current] >= maxDepth {
			continue
		}
		for _, candidate := range adjacency[current] {
			if _, seen := distance[candidate.nodeID]; seen {
				continue
			}
			distance[candidate.nodeID] = distance[current] + 1
			previousNode[candidate.nodeID] = current
			previousEdge[candidate.nodeID] = candidate.edge
			if candidate.nodeID == targetID {
				queue = nil
				break
			}
			queue = append(queue, candidate.nodeID)
		}
	}
	if _, found := distance[targetID]; !found {
		return []string{}, []*WorkbenchEdge{}, false
	}
	nodeIDs := []string{targetID}
	pathEdges := make([]*WorkbenchEdge, 0, distance[targetID])
	for current := targetID; current != sourceID; current = previousNode[current] {
		pathEdges = append(pathEdges, previousEdge[current])
		nodeIDs = append(nodeIDs, previousNode[current])
	}
	reverseStrings(nodeIDs)
	reverseEdges(pathEdges)
	return nodeIDs, pathEdges, true
}

// FindWorkbenchPath resolves a real persisted relationship path for a path-analysis tab.
func (g *GraphQuery) FindWorkbenchPath(ctx context.Context, tenantID, sourceID, targetID, anchorID, mode string, filter WorkbenchFilter) (*WorkbenchPath, error) {
	filter.CenterID = sourceID
	filter.RequiredIDs = []string{sourceID, targetID, anchorID}
	graph, err := g.GetWorkbenchGraph(ctx, tenantID, filter)
	if err != nil {
		return nil, err
	}
	maxDepth := filter.Depth
	if maxDepth < 1 {
		maxDepth = 3
	}
	adjacency := make(map[string][]workbenchPathStep)
	for _, edge := range graph.Edges {
		if !workbenchEdgeMatchesMode(edge, mode) {
			continue
		}
		adjacency[edge.SourceID] = append(adjacency[edge.SourceID], workbenchPathStep{nodeID: edge.TargetID, edge: edge})
	}
	var nodeIDs []string
	var pathEdges []*WorkbenchEdge
	var found bool
	if anchorID != "" && anchorID != sourceID && anchorID != targetID {
		var firstNodes, secondNodes []string
		var firstEdges, secondEdges []*WorkbenchEdge
		firstNodes, firstEdges, found = findDirectedWorkbenchSegment(adjacency, sourceID, anchorID, maxDepth)
		if found {
			remainingDepth := maxDepth - len(firstEdges)
			secondNodes, secondEdges, found = findDirectedWorkbenchSegment(adjacency, anchorID, targetID, remainingDepth)
		}
		if found {
			nodeIDs = append(firstNodes, secondNodes[1:]...)
			pathEdges = append(firstEdges, secondEdges...)
		}
	} else {
		nodeIDs, pathEdges, found = findDirectedWorkbenchSegment(adjacency, sourceID, targetID, maxDepth)
	}
	if !found {
		return &WorkbenchPath{Mode: mode, SourceID: sourceID, TargetID: targetID, NodeIDs: []string{}, Edges: []*WorkbenchEdge{}, EvidenceIDs: []string{}}, nil
	}
	risk := "low"
	evidenceSet := make(map[string]bool)
	evidenceIDs := make([]string, 0, len(pathEdges))
	for _, edge := range pathEdges {
		if edge.RiskLevel == "high" || edge.RiskLevel == "medium" && risk != "high" {
			risk = edge.RiskLevel
		}
		if edge.EvidenceID != "" && !evidenceSet[edge.EvidenceID] {
			evidenceSet[edge.EvidenceID] = true
			evidenceIDs = append(evidenceIDs, edge.EvidenceID)
		}
	}
	return &WorkbenchPath{Mode: mode, SourceID: sourceID, TargetID: targetID, NodeIDs: nodeIDs, Edges: pathEdges, Length: len(pathEdges), RiskLevel: risk, EvidenceIDs: evidenceIDs}, nil
}

func workbenchEdgeMatchesMode(edge *WorkbenchEdge, mode string) bool {
	switch mode {
	case "attack":
		return edge.RelationType == "关联告警" || attributeString(edge.Attributes, "attack_stage") != "" || attributeString(edge.Attributes, "action") != "" || attributeString(edge.Attributes, "alert_stage") != ""
	case "communication":
		return edge.RelationType == "通信" || edge.RelationType == "DNS解析" || edge.RelationType == "行为服务"
	case "account":
		return (edge.RelationType == "登录" || edge.RelationType == "账号访问") && attributeString(edge.Attributes, "identity_label") != ""
	default:
		return true
	}
}

func attributeString(attributes map[string]interface{}, key string) string {
	value, _ := attributes[key].(string)
	return strings.TrimSpace(value)
}

func metadataString(metadata map[string]interface{}, key string) string {
	value, _ := metadata[key].(string)
	return value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func reverseStrings(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseEdges(values []*WorkbenchEdge) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
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

func buildSessionConditions(tenantID, runID string, startTime, endTime int64) ([]string, []interface{}) {
	conditions := []string{"tenant_id = ?"}
	args := []interface{}{tenantID}

	if runID != "" {
		conditions = append(conditions, "run_id = ?")
		args = append(args, runID)
	}
	if startTime > 0 {
		conditions = append(conditions, "ts_end >= ?")
		args = append(args, startTime)
	}
	if endTime > 0 {
		conditions = append(conditions, "ts_end <= ?")
		args = append(args, endTime)
	}

	return conditions, args
}

func formatMillis(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).Format(time.RFC3339)
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

	queryCtx := ctx
	var cancel context.CancelFunc
	if g.config.QueryTimeout > 0 {
		queryCtx, cancel = context.WithTimeout(ctx, g.config.QueryTimeout)
		defer cancel()
	}

	// 尝试从缓存获取
	if g.cache != nil {
		if cachedGraph := g.getGraphFromCache(queryCtx, tenantID, centerIP, depth, startTime, endTime, runID); cachedGraph != nil {
			atomic.AddInt64(&g.metrics.CacheHits, 1)
			cachedGraph.CacheHit = true
			return cachedGraph, nil
		}
		atomic.AddInt64(&g.metrics.CacheMisses, 1)
	}

	// 从数据库查询
	graph, err := g.exploreFromDB(queryCtx, tenantID, centerIP, depth, startTime, endTime, runID)
	if err != nil {
		atomic.AddInt64(&g.metrics.FailedQueries, 1)
		if queryCtx.Err() != nil {
			atomic.AddInt64(&g.metrics.TimeoutQueries, 1)
			return nil, apperrors.Wrap(queryCtx.Err(), apperrors.ErrCodeTimeout, "Graph exploration timed out")
		}
		return nil, err
	}

	if queryCtx.Err() != nil {
		atomic.AddInt64(&g.metrics.TimeoutQueries, 1)
		graph.Truncated = true
	}

	graph.CacheHit = false

	// 缓存结果
	if g.cache != nil {
		g.cacheGraph(queryCtx, tenantID, centerIP, depth, startTime, endTime, runID, graph)
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
				graph.Truncated = true
				if ctx.Err() != nil {
					goto done
				}
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

	conditions, args := buildSessionConditions(tenantID, runID, startTime, endTime)
	conditions = append(conditions, "(src_ip = ? OR dst_ip = ?)")
	args = append(args, nodeIP, nodeIP)

	sql := fmt.Sprintf(`
		SELECT
			if(src_ip = ?, dst_ip, src_ip) as neighbor_ip,
			count() as session_count,
			sum(bytes_total) as total_bytes,
			max(ts_end) as last_seen
		FROM traffic.sessions
		WHERE %s
		GROUP BY neighbor_ip
		ORDER BY session_count DESC
		LIMIT ?
	`, strings.Join(conditions, " AND "))

	queryArgs := append([]interface{}{nodeIP}, args...)
	queryArgs = append(queryArgs, limit)
	rows, err := g.client.Query(ctx, sql, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query neighbors: %w", err)
	}
	defer rows.Close()

	neighbors := make([]*GraphNode, 0)
	cacheInfos := make([]cache.NeighborInfo, 0)

	for rows.Next() {
		var node GraphNode
		var sessionCount uint64
		var lastSeen int64

		err := rows.Scan(
			&node.IP,
			&sessionCount,
			&node.TotalBytes,
			&lastSeen,
		)
		if err != nil {
			g.logger.Error("Failed to scan neighbor", zap.Error(err))
			continue
		}

		node.SessionCount = int(sessionCount)
		node.LastSeen = formatMillis(lastSeen)
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

	conditions, args := buildSessionConditions(tenantID, runID, startTime, endTime)
	conditions = append(conditions, "(src_ip = ? OR dst_ip = ?)")
	args = append(args, entityID, entityID)

	sql := fmt.Sprintf(`
		SELECT
			count() as session_count,
			sum(bytes_total) as total_bytes,
			min(ts_start) as first_seen,
			max(ts_end) as last_seen
		FROM traffic.sessions
		WHERE %s
	`, strings.Join(conditions, " AND "))

	var details EntityDetails
	var sessionCount uint64
	var firstSeen, lastSeen int64

	row, err := g.client.QueryRow(ctx, sql, args...)
	err = row.Scan(
		&sessionCount,
		&details.TotalBytes,
		&firstSeen,
		&lastSeen)

	if err != nil {
		return nil, fmt.Errorf("failed to query entity details: %w", err)
	}

	details.EntityID = entityID
	details.EntityType = entityType
	details.SessionCount = int(sessionCount)
	details.FirstSeen = formatMillis(firstSeen)
	details.LastSeen = formatMillis(lastSeen)

	// 查询告警数量
	alertConditions := []string{"tenant_id = ?", "(src_ip = ? OR dst_ip = ?)"}
	alertArgs := []interface{}{tenantID, entityID, entityID}
	if startTime > 0 {
		alertConditions = append(alertConditions, "last_seen >= ?")
		alertArgs = append(alertArgs, startTime)
	}
	if endTime > 0 {
		alertConditions = append(alertConditions, "last_seen <= ?")
		alertArgs = append(alertArgs, endTime)
	}

	alertSQL := fmt.Sprintf(`
		SELECT count()
		FROM traffic.alerts
		WHERE %s
	`, strings.Join(alertConditions, " AND "))

	row, err = g.client.QueryRow(ctx, alertSQL, alertArgs...)
	var alertCount uint64
	err = row.Scan(&alertCount)

	if err != nil {
		g.logger.Warn("Failed to query alert count", zap.Error(err))
	} else {
		details.AlertCount = int(alertCount)
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
		interval = "toStartOfMinute(fromUnixTimestamp64Milli(ts_end))"
	case "hour":
		interval = "toStartOfHour(fromUnixTimestamp64Milli(ts_end))"
	case "day":
		interval = "toStartOfDay(fromUnixTimestamp64Milli(ts_end))"
	default:
		interval = "toStartOfHour(fromUnixTimestamp64Milli(ts_end))"
	}

	conditions, args := buildSessionConditions(tenantID, runID, startTime, endTime)
	conditions = append(conditions, "(src_ip = ? OR dst_ip = ?)")
	args = append(args, entityID, entityID)

	sql := fmt.Sprintf(`
		SELECT
			toUnixTimestamp64Milli(%s) as timestamp,
			count() as session_count,
			sum(bytes_total) as total_bytes
		FROM traffic.sessions
		WHERE %s
		GROUP BY timestamp
		ORDER BY timestamp ASC
	`, interval, strings.Join(conditions, " AND "))

	rows, err := g.client.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query timeline: %w", err)
	}
	defer rows.Close()

	timeline := make([]*TimelinePoint, 0)
	for rows.Next() {
		var point TimelinePoint
		var sessionCount uint64
		err := rows.Scan(
			&point.Timestamp,
			&sessionCount,
			&point.TotalBytes,
		)
		if err != nil {
			g.logger.Error("Failed to scan timeline point", zap.Error(err))
			continue
		}
		point.SessionCount = int(sessionCount)
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

	conditions, args := buildSessionConditions(tenantID, runID, startTime, endTime)

	sql := fmt.Sprintf(`
		SELECT
			uniq(src_ip, dst_ip) as unique_ips,
			count() as total_sessions,
			sum(bytes_total) as total_bytes
		FROM traffic.sessions
		WHERE %s
	`, strings.Join(conditions, " AND "))

	var uniqueIPs, totalSessions uint64
	var totalBytes uint64

	row, err := g.client.QueryRow(ctx, sql, args...)
	err = row.Scan(&uniqueIPs, &totalSessions, &totalBytes)
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
	var result *Graph
	var executeErr error

	err := g.circuitBreaker.Execute(ctx, func() error {
		result, executeErr = g.GraphQuery.Explore(ctx, tenantID, centerIP, depth, startTime, endTime, runID)
		return executeErr
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// BatchExplore 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) BatchExplore(ctx context.Context, tenantID string, centerIPs []string, depth int, startTime, endTime int64, runID string) (*Graph, error) {
	var result *Graph
	var executeErr error

	err := g.circuitBreaker.Execute(ctx, func() error {
		result, executeErr = g.GraphQuery.BatchExplore(ctx, tenantID, centerIPs, depth, startTime, endTime, runID)
		return executeErr
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetNeighbors 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) GetNeighbors(ctx context.Context, tenantID, nodeIP string, startTime, endTime int64, runID string, limit int) ([]*GraphNode, error) {
	var result []*GraphNode
	var executeErr error

	err := g.circuitBreaker.Execute(ctx, func() error {
		result, executeErr = g.GraphQuery.GetNeighbors(ctx, tenantID, nodeIP, startTime, endTime, runID, limit)
		return executeErr
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetEntityDetails 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) GetEntityDetails(ctx context.Context, tenantID, entityID, entityType string, startTime, endTime int64, runID string) (*EntityDetails, error) {
	var result *EntityDetails
	var executeErr error

	err := g.circuitBreaker.Execute(ctx, func() error {
		result, executeErr = g.GraphQuery.GetEntityDetails(ctx, tenantID, entityID, entityType, startTime, endTime, runID)
		return executeErr
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetEntityTimeline 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) GetEntityTimeline(ctx context.Context, tenantID, entityID string, startTime, endTime int64, runID, granularity string) ([]*TimelinePoint, error) {
	var result []*TimelinePoint
	var executeErr error

	err := g.circuitBreaker.Execute(ctx, func() error {
		result, executeErr = g.GraphQuery.GetEntityTimeline(ctx, tenantID, entityID, startTime, endTime, runID, granularity)
		return executeErr
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// FindPaths 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) FindPaths(ctx context.Context, tenantID, sourceIP, targetIP string, maxHops int, startTime, endTime int64, runID string) ([]*Path, error) {
	var result []*Path
	var executeErr error

	err := g.circuitBreaker.Execute(ctx, func() error {
		result, executeErr = g.GraphQuery.FindPaths(ctx, tenantID, sourceIP, targetIP, maxHops, startTime, endTime, runID)
		return executeErr
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetStats 使用熔断器包装
func (g *GraphQueryWithCircuitBreaker) GetStats(ctx context.Context, tenantID string, startTime, endTime int64, runID string) (map[string]interface{}, error) {
	var result map[string]interface{}
	var executeErr error

	err := g.circuitBreaker.Execute(ctx, func() error {
		result, executeErr = g.GraphQuery.GetStats(ctx, tenantID, startTime, endTime, runID)
		return executeErr
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

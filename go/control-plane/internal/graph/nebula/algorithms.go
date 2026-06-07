// Graph Algorithm Engine — 图分析算法
//
// 基于 NebulaGraph 存储实现:
//   - Louvain 社区检测: 识别网络中的功能子网/攻击群组
//   - PageRank 中心性: 识别网络中的关键节点
//   - Betweenness 中心性: 识别网络桥梁节点
//   - Attack Path 分析: 重建攻击路径
//   - Degree 分布分析: 识别异常通信模式
package nebula

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CommunityDetection 社区检测结果
type CommunityDetection struct {
	Communities      map[int][]string          `json:"communities"`       // community_id → IP列表
	IPToCommunity    map[string]int            `json:"ip_to_community"`   // IP → community_id
	Modularity       float64                   `json:"modularity"`        // 模块度
	CommunityCount   int                       `json:"community_count"`
	CommunityStats   []CommunityStat           `json:"community_stats"`   // 社区统计
}

// CommunityStat 社区统计
type CommunityStat struct {
	CommunityID int     `json:"community_id"`
	NodeCount   int     `json:"node_count"`
	EdgeCount   int     `json:"edge_count"`
	AvgDegree   float64 `json:"avg_degree"`
	Density     float64 `json:"density"`
	IsAnomalous bool    `json:"is_anomalous"` // 异常社区标记
}

// PageRankResult PageRank 结果
type PageRankResult struct {
	Scores       map[string]float64 `json:"scores"`        // IP → PageRank score
	TopN         []RankedNode       `json:"top_n"`         // Top-N 节点
	Converged    bool               `json:"converged"`
	Iterations   int                `json:"iterations"`
	DampingFactor float64           `json:"damping_factor"`
}

// RankedNode 排名节点
type RankedNode struct {
	IP    string  `json:"ip"`
	Score float64 `json:"score"`
	Rank  int     `json:"rank"`
}

// CentralityResult 中心性结果
type CentralityResult struct {
	Betweenness     map[string]float64 `json:"betweenness"`      // IP → betweenness score
	Closeness       map[string]float64 `json:"closeness"`        // IP → closeness score
	Eigenvector     map[string]float64 `json:"eigenvector"`      // IP → eigenvector score
	BridgeNodes     []string           `json:"bridge_nodes"`     // 桥梁节点 (高 betweenness)
	HubNodes        []string           `json:"hub_nodes"`        // 枢纽节点 (高 degree + eigenvector)
}

// AttackPathResult 攻击路径分析结果
type AttackPathResult struct {
	Paths           []AttackPath       `json:"paths"`
	SourceIP        string             `json:"source_ip"`
	TargetIP        string             `json:"target_ip"`
	MaxHops         int                `json:"max_hops"`
	ShortestPathLen int                `json:"shortest_path_len"`
	TotalPaths      int                `json:"total_paths"`
}

// AttackPath 单条攻击路径
type AttackPath struct {
	Hops       []string `json:"hops"`        // IP 序列
	Length     int      `json:"length"`
	TotalBytes uint64   `json:"total_bytes"`  // 路径上传输的总字节
	RiskScore  float64  `json:"risk_score"`   // 路径风险评分
	Techniques []string `json:"techniques"`   // MITRE 技术
}

// AnomalyPattern 异常通信模式
type AnomalyPattern struct {
	Type        string   `json:"type"`         // "star", "chain", "mesh", "isolated"
	CenterIP    string   `json:"center_ip,omitempty"`
	Members     []string `json:"members"`
	EdgeCount   int      `json:"edge_count"`
	RiskScore   float64  `json:"risk_score"`
	Description string   `json:"description"`
}

// GraphAnalyzer 图分析器
type GraphAnalyzer struct {
	client *Client
	logger *zap.Logger
	mu     sync.RWMutex
	// 缓存
	adjacencyCache map[string][]string   // IP → neighbors
	edgeCache      []graphEdge           // 缓存的边数据
	cacheTTL       time.Duration
	cacheTime      time.Time
}

// NewGraphAnalyzer 创建图分析器
func NewGraphAnalyzer(client *Client, logger *zap.Logger) *GraphAnalyzer {
	return &GraphAnalyzer{
		client:         client,
		logger:         logger,
		adjacencyCache: make(map[string][]string),
		cacheTTL:       5 * time.Minute,
	}
}

// ============================================================================
// Louvain 社区检测
// ============================================================================

// DetectCommunities Louvain 算法社区检测
func (ga *GraphAnalyzer) DetectCommunities(ctx context.Context, tenantID string, minModularityGain float64) (*CommunityDetection, error) {
	if minModularityGain <= 0 { minModularityGain = 0.0001 }

	// 1. 获取图结构 (简化: 从 ClickHouse 获取邻接关系)
	nodes, edges, err := ga.loadNetworkTopology(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("load topology: %w", err)
	}

	// 2. 初始化: 每个节点自成一个社区
	community := make(map[string]int)
	nodeList := make([]string, 0, len(nodes))
	for i, ip := range nodes {
		community[ip] = i
		nodeList = append(nodeList, ip)
	}

	// 3. Louvain 迭代
	m := float64(len(edges)) // 总边数
	if m == 0 { m = 1 }

	// 节点权重 (degree)
	nodeWeight := make(map[string]float64)
	adjList := make(map[string]map[string]float64) // adjacency with weights
	for _, ip := range nodes {
		adjList[ip] = make(map[string]float64)
	}
	for _, e := range edges {
		w := float64(e.sessionCount)
		if w == 0 { w = 1.0 }
		adjList[e.src][e.dst] += w
		adjList[e.dst][e.src] += w
		nodeWeight[e.src] += w
		nodeWeight[e.dst] += w
	}

	changed := true
	iterations := 0
	const maxIterations = 100

	for changed && iterations < maxIterations {
		changed = false
		iterations++

		for _, node := range nodeList {
			currentComm := community[node]
			neighborComms := make(map[int]float64)

			// 计算移动到每个邻接社区的模块度增益
			for neighbor, weight := range adjList[node] {
				neighborComm := community[neighbor]
				neighborComms[neighborComm] += weight
			}

			// 移除节点对当前社区的贡献
			bestComm := currentComm
			bestGain := 0.0
			ki := nodeWeight[node]

			for comm, kiIn := range neighborComms {
				if comm == currentComm { continue }
				// 模块度增益: ΔQ = ki_in/m - Σtot*ki/m²
				sigmaTot := ga.computeCommunityWeight(community, nodeWeight, comm)
				gain := kiIn/m - sigmaTot*ki/(m*m)
				if gain > bestGain && gain > minModularityGain {
					bestGain = gain
					bestComm = comm
				}
			}

			if bestComm != currentComm {
				community[node] = bestComm
				changed = true
			}
		}
	}

	// 4. 组装结果
	communities := make(map[int][]string)
	for ip, comm := range community {
		communities[comm] = append(communities[comm], ip)
	}

	// 5. 计算模块度
	modularity := ga.computeModularity(community, adjList, nodeWeight, m)

	// 6. 社区统计
	communityStats := ga.computeCommunityStats(communities, adjList)

	result := &CommunityDetection{
		Communities:    communities,
		IPToCommunity:  community,
		Modularity:     modularity,
		CommunityCount: len(communities),
		CommunityStats: communityStats,
	}

	ga.logger.Info("Community detection completed",
		zap.Int("communities", result.CommunityCount),
		zap.Float64("modularity", modularity),
		zap.Int("iterations", iterations))

	return result, nil
}

// computeCommunityWeight 计算社区的总权重
func (ga *GraphAnalyzer) computeCommunityWeight(community map[string]int, nodeWeight map[string]float64, commID int) float64 {
	var total float64
	for node, comm := range community {
		if comm == commID {
			total += nodeWeight[node]
		}
	}
	return total
}

// computeModularity 计算模块度
func (ga *GraphAnalyzer) computeModularity(community map[string]int, adjList map[string]map[string]float64, nodeWeight map[string]float64, m float64) float64 {
	if m == 0 { return 0 }
	var Q float64
	for u := range adjList {
		cu := community[u]
		for v, w := range adjList[u] {
			if community[v] == cu {
				// A_uv - k_u * k_v / (2m)
				expected := nodeWeight[u] * nodeWeight[v] / (2 * m)
				Q += (w - expected)
			}
		}
	}
	return Q / (2 * m)
}

// computeCommunityStats 计算社区统计
func (ga *GraphAnalyzer) computeCommunityStats(communities map[int][]string, adjList map[string]map[string]float64) []CommunityStat {
	stats := make([]CommunityStat, 0, len(communities))
	for commID, members := range communities {
		s := CommunityStat{CommunityID: commID, NodeCount: len(members)}
		// 社区内边
		memberSet := make(map[string]bool)
		for _, m := range members { memberSet[m] = true }
		var internalEdges int
		for _, u := range members {
			for v := range adjList[u] {
				if memberSet[v] { internalEdges++ }
			}
		}
		s.EdgeCount = internalEdges / 2 // 无向边去重
		s.AvgDegree = float64(internalEdges) / float64(len(members))
		// Density = 2*E / (N*(N-1))
		n := float64(len(members))
		if n > 1 {
			s.Density = float64(internalEdges) / (n * (n - 1))
		}
		// Mark as anomalous if density > 0.8 (fully connected → possible botnet)
		if s.Density > 0.8 && s.NodeCount >= 3 {
			s.IsAnomalous = true
		}
		stats = append(stats, s)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].NodeCount > stats[j].NodeCount })
	return stats
}

// ============================================================================
// PageRank
// ============================================================================

// ComputePageRank 计算 PageRank
func (ga *GraphAnalyzer) ComputePageRank(ctx context.Context, tenantID string, dampingFactor float64, maxIterations int) (*PageRankResult, error) {
	if dampingFactor <= 0 { dampingFactor = 0.85 }
	if maxIterations <= 0 { maxIterations = 100 }

	nodes, edges, err := ga.loadNetworkTopology(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	N := float64(len(nodes))
	if N == 0 { return nil, fmt.Errorf("empty graph") }

	// 构建邻接表
	adjList := make(map[string][]string)
	outDegree := make(map[string]int)
	for _, ip := range nodes {
		adjList[ip] = make([]string, 0)
		outDegree[ip] = 0
	}
	for _, e := range edges {
		adjList[e.src] = append(adjList[e.src], e.dst)
		outDegree[e.src]++
	}

	// 初始化 PageRank
	pr := make(map[string]float64)
	for _, ip := range nodes {
		pr[ip] = 1.0 / N
	}

	// 迭代
	converged := false
	iter := 0
	for iter < maxIterations {
		newPR := make(map[string]float64)
		var danglingSum float64

		// dangling nodes (no outgoing edges)
		for _, ip := range nodes {
			if outDegree[ip] == 0 {
				danglingSum += pr[ip]
			}
		}

		danglingContribution := dampingFactor * danglingSum / N
		teleport := (1.0 - dampingFactor) / N

		for _, ip := range nodes {
			newPR[ip] = teleport + danglingContribution
			// contribution from incoming edges
			for _, neighbor := range adjList[ip] {
				if outDegree[neighbor] > 0 {
					newPR[ip] += dampingFactor * pr[neighbor] / float64(outDegree[neighbor])
				}
			}
		}

		// check convergence
		var maxDiff float64
		for _, ip := range nodes {
			diff := abs(newPR[ip] - pr[ip])
			if diff > maxDiff { maxDiff = diff }
			pr[ip] = newPR[ip]
		}

		iter++
		if maxDiff < 1e-9 {
			converged = true
			break
		}
	}

	// Top-N
	var ranked []RankedNode
	for ip, score := range pr {
		ranked = append(ranked, RankedNode{IP: ip, Score: score})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })

	topN := 20
	if len(ranked) < topN { topN = len(ranked) }
	for i := range ranked[:topN] {
		ranked[i].Rank = i + 1
	}

	result := &PageRankResult{
		Scores:        pr,
		TopN:          ranked[:topN],
		Converged:     converged,
		Iterations:    iter,
		DampingFactor: dampingFactor,
	}

	ga.logger.Info("PageRank computed",
		zap.Int("iterations", iter),
		zap.Bool("converged", converged),
		zap.Int("nodes", len(nodes)))

	return result, nil
}

// ============================================================================
// Betweenness Centrality (Brandes Algorithm)
// ============================================================================

// ComputeCentrality 计算多种中心性指标
func (ga *GraphAnalyzer) ComputeCentrality(ctx context.Context, tenantID string, sampleSize int) (*CentralityResult, error) {
	nodes, edges, err := ga.loadNetworkTopology(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 { return nil, fmt.Errorf("empty graph") }

	// 构建邻接表
	adjList := make(map[string][]string)
	for _, ip := range nodes {
		adjList[ip] = make([]string, 0)
	}
	for _, e := range edges {
		adjList[e.src] = append(adjList[e.src], e.dst)
		adjList[e.dst] = append(adjList[e.dst], e.src)
	}

	// Betweenness Centrality (Brandes)
	betweenness := make(map[string]float64)
	for _, ip := range nodes {
		betweenness[ip] = 0
	}

	// 采样优化: 大图只采样部分节点计算
	sources := nodes
	if sampleSize > 0 && sampleSize < len(nodes) {
		// 随机采样
		step := len(nodes) / sampleSize
		sources = make([]string, 0, sampleSize)
		for i := 0; i < len(nodes) && len(sources) < sampleSize; i += step {
			sources = append(sources, nodes[i])
		}
	}

	for _, s := range sources {
		// BFS from s
		stack := make([]string, 0)
		pred := make(map[string][]string)
		sigma := make(map[string]int)
		dist := make(map[string]int)

		for _, v := range nodes {
			pred[v] = make([]string, 0)
			sigma[v] = 0
			dist[v] = -1
		}
		sigma[s] = 1
		dist[s] = 0

		queue := []string{s}
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			stack = append(stack, v)

			for _, w := range adjList[v] {
				if dist[w] < 0 {
					dist[w] = dist[v] + 1
					queue = append(queue, w)
				}
				if dist[w] == dist[v]+1 {
					sigma[w] += sigma[v]
					pred[w] = append(pred[w], v)
				}
			}
		}

		// 反向累积
		delta := make(map[string]float64)
		for _, v := range nodes { delta[v] = 0 }
		for i := len(stack) - 1; i >= 0; i-- {
			w := stack[i]
			for _, v := range pred[w] {
				if sigma[w] > 0 && sigma[v] > 0 {
					delta[v] += float64(sigma[v]) / float64(sigma[w]) * (1 + delta[w])
				}
			}
			if w != s {
				betweenness[w] += delta[w]
			}
		}
	}

	// Normalize
	n := float64(len(nodes))
	if n > 2 {
		factor := 1.0 / ((n - 1) * (n - 2))
		for ip := range betweenness {
			betweenness[ip] *= factor
		}
	}

	// Closeness Centrality — computed from BFS distances
	closeness := make(map[string]float64)
	for _, s := range sources {
		distances := bfsDistances(s, adjList)
		var sumDist int
		var reachable int
		for _, d := range distances {
			if d > 0 {
				sumDist += d
				reachable++
			}
		}
		if sumDist > 0 && reachable > 0 {
			closeness[s] = float64(reachable) / float64(sumDist)
		}
	}

	// Eigenvector Centrality (simplified power iteration)
	eigenvector := make(map[string]float64)
	for _, ip := range nodes { eigenvector[ip] = 1.0 }

	// Power iteration for eigenvector
	for iter := 0; iter < 50; iter++ {
		newEig := make(map[string]float64)
		var norm float64
		for _, v := range nodes {
			for _, u := range adjList[v] {
				newEig[v] += eigenvector[u]
			}
			norm += newEig[v] * newEig[v]
		}
		if norm > 0 {
			norm = sqrtFloat(norm)
			for v := range newEig {
				newEig[v] /= norm
			}
		}
		eigenvector = newEig
	}

	// Identify bridge & hub nodes
	var sortedBetweenness []RankedNode
	for ip, score := range betweenness {
		sortedBetweenness = append(sortedBetweenness, RankedNode{IP: ip, Score: score})
	}
	sort.Slice(sortedBetweenness, func(i, j int) bool {
		return sortedBetweenness[i].Score > sortedBetweenness[j].Score
	})

	bridgeCount := max(3, len(nodes)/20)
	hubCount := max(3, len(nodes)/20)

	bridgeNodes := make([]string, 0, bridgeCount)
	hubNodes := make([]string, 0, hubCount)

	for i := 0; i < len(sortedBetweenness) && i < bridgeCount; i++ {
		bridgeNodes = append(bridgeNodes, sortedBetweenness[i].IP)
	}

	// Hubs: high degree + high eigenvector
	type hubCandidate struct {
		ip    string
		score float64
	}
	var hubs []hubCandidate
	for _, ip := range nodes {
		degreeScore := float64(len(adjList[ip])) / float64(len(nodes))
		eigScore := eigenvector[ip]
		hubs = append(hubs, hubCandidate{ip, degreeScore * 0.5 + eigScore * 0.5})
	}
	sort.Slice(hubs, func(i, j int) bool { return hubs[i].score > hubs[j].score })
	for i := 0; i < len(hubs) && i < hubCount; i++ {
		hubNodes = append(hubNodes, hubs[i].ip)
	}

	return &CentralityResult{
		Betweenness: betweenness,
		Closeness:   closeness,
		Eigenvector: eigenvector,
		BridgeNodes: bridgeNodes,
		HubNodes:    hubNodes,
	}, nil
}

// ============================================================================
// Attack Path Analysis
// ============================================================================

// AnalyzeAttackPaths 攻击路径分析
func (ga *GraphAnalyzer) AnalyzeAttackPaths(ctx context.Context, tenantID, sourceIP, targetIP string, maxHops int) (*AttackPathResult, error) {
	if maxHops <= 0 { maxHops = 5 }
	if maxHops > 10 { maxHops = 10 }

	nodes, edges, err := ga.loadNetworkTopology(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	_ = nodes

	// 构建加权邻接表
	adjList := make(map[string]map[string]*graphEdge)
	for _, ip := range nodes {
		adjList[ip] = make(map[string]*graphEdge)
	}
	edgeMap := make(map[string]*graphEdge)
	for _, e := range edges {
		key := e.src + "->" + e.dst
		edgeMap[key] = &e
		adjList[e.src][e.dst] = &e
		adjList[e.dst][e.src] = &e // 无向图
	}

	// DFS 搜索所有路径
	var allPaths []AttackPath
	visited := make(map[string]bool)
	currentPath := []string{sourceIP}

	var dfs func(current string, depth int, totalBytes uint64, risk float64)
	dfs = func(current string, depth int, totalBytes uint64, risk float64) {
		if depth > maxHops { return }
		if current == targetIP && depth > 0 {
			path := make([]string, len(currentPath))
			copy(path, currentPath)
			allPaths = append(allPaths, AttackPath{
				Hops:       path,
				Length:     depth,
				TotalBytes: totalBytes,
				RiskScore:  risk / float64(depth),
			})
			return
		}
		for neighbor, edge := range adjList[current] {
			if visited[neighbor] { continue }
			if len(allPaths) >= 100 { return } // limit
			visited[neighbor] = true
			currentPath = append(currentPath, neighbor)
			newBytes := totalBytes + uint64(edge.totalBytes)
			newRisk := risk + edge.riskScore
			dfs(neighbor, depth+1, newBytes, newRisk)
			currentPath = currentPath[:len(currentPath)-1]
			visited[neighbor] = false
		}
	}

	visited[sourceIP] = true
	dfs(sourceIP, 0, 0, 0)

	// 排序: 最短路径优先
	sort.Slice(allPaths, func(i, j int) bool {
		if allPaths[i].Length != allPaths[j].Length {
			return allPaths[i].Length < allPaths[j].Length
		}
		return allPaths[i].RiskScore > allPaths[j].RiskScore
	})

	shortestLen := 0
	if len(allPaths) > 0 {
		shortestLen = allPaths[0].Length
	}

	result := &AttackPathResult{
		Paths:           allPaths,
		SourceIP:        sourceIP,
		TargetIP:        targetIP,
		MaxHops:         maxHops,
		ShortestPathLen: shortestLen,
		TotalPaths:      len(allPaths),
	}

	ga.logger.Info("Attack path analysis completed",
		zap.String("source", sourceIP),
		zap.String("target", targetIP),
		zap.Int("paths", result.TotalPaths),
		zap.Int("shortest", shortestLen))

	return result, nil
}

// ============================================================================
// Anomaly Pattern Detection
// ============================================================================

// DetectAnomalyPatterns 检测异常通信模式
func (ga *GraphAnalyzer) DetectAnomalyPatterns(ctx context.Context, tenantID string) ([]AnomalyPattern, error) {
	nodes, edges, err := ga.loadNetworkTopology(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// 构建邻接表
	adjList := make(map[string]map[string]bool)
	outDegree := make(map[string]int)
	inDegree := make(map[string]int)
	for _, ip := range nodes {
		adjList[ip] = make(map[string]bool)
		outDegree[ip] = 0
		inDegree[ip] = 0
	}
	for _, e := range edges {
		adjList[e.src][e.dst] = true
		adjList[e.dst][e.src] = true
		outDegree[e.src]++
		inDegree[e.dst]++
	}

	var patterns []AnomalyPattern

	// 1. Star pattern: 一个中心节点连接大量叶节点 (可能 C2 通信)
	for _, ip := range nodes {
		degree := len(adjList[ip])
		if degree >= 10 && degree > len(nodes)/3 {
			members := make([]string, 0, degree)
			for neighbor := range adjList[ip] {
				members = append(members, neighbor)
			}
			patterns = append(patterns, AnomalyPattern{
				Type:        "star",
				CenterIP:    ip,
				Members:     members,
				EdgeCount:   degree,
				RiskScore:   minFloat(1.0, float64(degree)/float64(len(nodes))*2),
				Description: fmt.Sprintf("Star pattern detected: %s connects to %d leaf nodes (possible C2)", ip, degree),
			})
		}
	}

	// 2. Chain pattern: 线性通信链 (横向移动)
	visited := make(map[string]bool)
	for _, ip := range nodes {
		if visited[ip] || len(adjList[ip]) != 2 { continue }
		// DFS 沿链走
		chain := []string{ip}
		visited[ip] = true
		current := ip
		for {
			found := false
			for neighbor := range adjList[current] {
				if !visited[neighbor] && len(adjList[neighbor]) <= 2 {
					chain = append(chain, neighbor)
					visited[neighbor] = true
					current = neighbor
					found = true
					break
				}
			}
			if !found { break }
		}
		if len(chain) >= 3 {
			patterns = append(patterns, AnomalyPattern{
				Type:        "chain",
				Members:     chain,
				EdgeCount:   len(chain) - 1,
				RiskScore:   0.5 + float64(len(chain))*0.1,
				Description: fmt.Sprintf("Chain pattern detected: %d IPs in linear path (possible lateral movement)", len(chain)),
			})
		}
	}

	// 3. Mesh pattern: 密集互连子图 (可能 P2P 僵尸网络)
	// 使用 clique detection 简化版
	communities, _ := ga.DetectCommunities(ctx, tenantID, 0.001)
	if communities != nil {
		for _, stat := range communities.CommunityStats {
			if stat.IsAnomalous && stat.Density > 0.7 && stat.NodeCount >= 3 {
				patterns = append(patterns, AnomalyPattern{
					Type:        "mesh",
					Members:     communities.Communities[stat.CommunityID],
					EdgeCount:   stat.EdgeCount,
					RiskScore:   stat.Density,
					Description: fmt.Sprintf("Mesh pattern detected: %d IPs with high interconnectivity (density=%.2f, possible P2P botnet)", stat.NodeCount, stat.Density),
				})
			}
		}
	}

	// 4. Isolated pattern: 孤立节点 (可能存在隐蔽通道)
	for _, ip := range nodes {
		if len(adjList[ip]) == 1 {
			neighbor := ""
			for n := range adjList[ip] { neighbor = n; break }
			patterns = append(patterns, AnomalyPattern{
				Type:        "isolated",
				CenterIP:    ip,
				Members:     []string{neighbor},
				EdgeCount:   1,
				RiskScore:   0.35,
				Description: fmt.Sprintf("Isolated node: %s only communicates with %s (possible covert channel)", ip, neighbor),
			})
		}
	}

	return patterns, nil
}

// ============================================================================
// 内部图拓扑表示
// ============================================================================

// graphEdge 图边 (内部用)
type graphEdge struct {
	src          string
	dst          string
	sessionCount int
	totalBytes   int64
	riskScore    float64
}

// loadNetworkTopology 从 NebulaGraph 加载网络拓扑
func (ga *GraphAnalyzer) loadNetworkTopology(ctx context.Context, tenantID string) ([]string, []graphEdge, error) {
	// 检查缓存
	ga.mu.RLock()
	if time.Since(ga.cacheTime) < ga.cacheTTL && len(ga.adjacencyCache) > 0 && len(ga.edgeCache) > 0 {
		nodes := make([]string, 0, len(ga.adjacencyCache))
		for ip := range ga.adjacencyCache { nodes = append(nodes, ip) }
		edges := make([]graphEdge, len(ga.edgeCache))
		copy(edges, ga.edgeCache)
		ga.mu.RUnlock()
		ga.logger.Debug("Topology loaded from cache",
			zap.Int("nodes", len(nodes)), zap.Int("edges", len(edges)))
		return nodes, edges, nil
	}
	ga.mu.RUnlock()

	// 从 NebulaGraph 查询
	lookupNGQL := fmt.Sprintf(
		`LOOKUP ON ip_address WHERE ip_address.tenant_id == "%s" YIELD properties(vertex).ip AS ip;`,
		tenantID)

	result, err := ga.client.Execute(ctx, lookupNGQL)
	if err != nil {
		// Fallback: return empty
		ga.logger.Warn("Failed to load topology from NebulaGraph", zap.Error(err))
		return []string{}, []graphEdge{}, nil
	}

	nodeSet := make(map[string]bool)
	nodeList := make([]string, 0)
	for _, row := range result.Rows {
		ip, ok := row["ip"].(string)
		if !ok || ip == "" {
			continue
		}
		if !nodeSet[ip] {
			nodeSet[ip] = true
			nodeList = append(nodeList, ip)
		}
	}

	if len(nodeList) == 0 {
		ga.logger.Debug("No IP nodes found in NebulaGraph for tenant",
			zap.String("tenant_id", tenantID))
		return []string{}, []graphEdge{}, nil
	}

	// 加载边关系: GO FROM 查询所有节点的通信关系
	edges := make([]graphEdge, 0)
	if len(nodeList) > 0 {
		// 批量查询: 对每个 IP 查询其出边关系
		// 使用 GO 语句获取 communicates 边
		for i, src := range nodeList {
			if i%50 == 0 { // 分批，避免单条查询过载
				ngql := fmt.Sprintf(
					`GO FROM "%s" OVER communicates YIELD dst(edge) AS dst, communicates.session_count AS sessions, communicates.total_bytes AS bytes`,
					hashVID(src))
				if result, err := ga.client.Execute(ctx, ngql); err == nil {
					for _, row := range result.Rows {
						dst, _ := row["dst"].(string)
						sessions := intFromRow(row, "sessions")
						b := int64FromRow(row, "bytes")
						if dst != "" {
							edges = append(edges, graphEdge{
								src: src, dst: dst,
								sessionCount: sessions,
								totalBytes:   b,
							})
						}
					}
				}
			}
		}
		ga.logger.Debug("Loaded graph edges",
			zap.Int("edge_count", len(edges)),
			zap.Int("node_count", len(nodeList)))
	}

	// 更新缓存（节点 + 边）
	ga.mu.Lock()
	ga.adjacencyCache = make(map[string][]string, len(nodeSet))
	for _, ip := range nodeList {
		ga.adjacencyCache[ip] = make([]string, 0)
	}
	for _, e := range edges {
		ga.adjacencyCache[e.src] = append(ga.adjacencyCache[e.src], e.dst)
	}
	ga.edgeCache = make([]graphEdge, len(edges))
	copy(ga.edgeCache, edges)
	ga.cacheTime = time.Now()
	ga.mu.Unlock()

	ga.logger.Debug("Loaded network topology",
		zap.Int("node_count", len(nodeList)),
		zap.Int("edge_count", len(edges)),
		zap.String("tenant_id", tenantID))

	return nodeList, edges, nil
}

// ============================================================================
// 辅助函数
// ============================================================================

func abs(x float64) float64 { if x < 0 { return -x }; return x }
func max(a, b int) int { if a > b { return a }; return b }
func minFloat(a, b float64) float64 { if a < b { return a }; return b }
func sqrtFloat(x float64) float64 { return math.Sqrt(x) }

// bfsDistances computes shortest path distances from a source node to all other nodes in an undirected graph.
func bfsDistances(source string, adjList map[string][]string) map[string]int {
	dist := make(map[string]int)
	for node := range adjList {
		dist[node] = -1
	}
	dist[source] = 0
	queue := []string{source}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for _, w := range adjList[v] {
			if dist[w] < 0 {
				dist[w] = dist[v] + 1
				queue = append(queue, w)
			}
		}
	}
	return dist
}

func intFromRow(row map[string]interface{}, key string) int {
	if v, ok := row[key]; ok {
		switch n := v.(type) {
		case int: return n
		case int64: return int(n)
		case float64: return int(n)
		}
	}
	return 0
}

func int64FromRow(row map[string]interface{}, key string) int64 {
	if v, ok := row[key]; ok {
		switch n := v.(type) {
		case int64: return n
		case int: return int64(n)
		case float64: return int64(n)
		}
	}
	return 0
}

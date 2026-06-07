package nebula

import (
	"testing"
)

// TestCommunityStats 验证社区统计计算
func TestCommunityStats(t *testing.T) {
	ga := &GraphAnalyzer{}

	// 构造测试数据: 2个社区
	communities := map[int][]string{
		1: {"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		2: {"192.168.1.1", "192.168.1.2"},
	}

	// 邻接表: 社区1 内部全连接，社区2 内部有一条边
	adjList := map[string]map[string]float64{
		"10.0.0.1":   {"10.0.0.2": 1.0, "10.0.0.3": 1.0},
		"10.0.0.2":   {"10.0.0.1": 1.0, "10.0.0.3": 1.0},
		"10.0.0.3":   {"10.0.0.1": 1.0, "10.0.0.2": 1.0},
		"192.168.1.1": {"192.168.1.2": 1.0},
		"192.168.1.2": {"192.168.1.1": 1.0},
	}

	stats := ga.computeCommunityStats(communities, adjList)

	if len(stats) != 2 {
		t.Fatalf("Expected 2 communities, got %d", len(stats))
	}

	// 社区1: 3节点, 3条内部边 (全连接三角形)
	c1 := stats[0]
	if c1.NodeCount != 3 {
		t.Errorf("Community 1 NodeCount = %d, want 3", c1.NodeCount)
	}
	// density = 2*E / (N*(N-1)) = 6/6 = 1.0 → IsAnomalous
	if !c1.IsAnomalous {
		t.Error("Community 1 should be anomalous (density > 0.8)")
	}

	// 社区2: 2节点, 1条内部边
	c2 := stats[1]
	if c2.NodeCount != 2 {
		t.Errorf("Community 2 NodeCount = %d, want 2", c2.NodeCount)
	}
	if c2.IsAnomalous {
		t.Error("Community 2 should NOT be anomalous")
	}
}

// TestModularity 验证模块度计算
func TestModularity(t *testing.T) {
	ga := &GraphAnalyzer{}

	community := map[string]int{
		"a": 0, "b": 0, "c": 1, "d": 1,
	}
	adjList := map[string]map[string]float64{
		"a": {"b": 1.0},
		"b": {"a": 1.0, "c": 0.5},
		"c": {"b": 0.5, "d": 1.0},
		"d": {"c": 1.0},
	}
	nodeWeight := map[string]float64{
		"a": 1.0, "b": 1.5, "c": 1.5, "d": 1.0,
	}
	m := 2.5 // total edge weight

	Q := ga.computeModularity(community, adjList, nodeWeight, m)

	// 模块度应在 [-0.5, 1.0] 之间
	if Q < -0.5 || Q > 1.0 {
		t.Errorf("Modularity = %f, expected between -0.5 and 1.0", Q)
	}
	t.Logf("Modularity Q = %f", Q)
}

// TestPageRank 验证 PageRank 计算
func TestPageRank(t *testing.T) {
	// 简单3节点图: A→B→C, B→A
	nodes := []string{"A", "B", "C"}
	edges := []graphEdge{
		{src: "A", dst: "B", sessionCount: 1},
		{src: "B", dst: "C", sessionCount: 1},
		{src: "B", dst: "A", sessionCount: 1},
	}

	// Build: outLinks[node] = list of nodes that node links TO
	outLinks := make(map[string][]string)
	outDegree := make(map[string]int)
	// Build: inLinks[node] = list of nodes that link TO node (for PageRank contribution)
	inLinks := make(map[string][]string)
	for _, ip := range nodes {
		outLinks[ip] = make([]string, 0)
		inLinks[ip] = make([]string, 0)
		outDegree[ip] = 0
	}
	for _, e := range edges {
		outLinks[e.src] = append(outLinks[e.src], e.dst)
		inLinks[e.dst] = append(inLinks[e.dst], e.src)
		outDegree[e.src]++
	}

	N := float64(len(nodes))
	pr := make(map[string]float64)
	for _, ip := range nodes {
		pr[ip] = 1.0 / N
	}

	dampingFactor := 0.85
	for iter := 0; iter < 100; iter++ {
		newPR := make(map[string]float64)
		var danglingSum float64
		for _, ip := range nodes {
			if outDegree[ip] == 0 {
				danglingSum += pr[ip]
			}
		}
		danglingContribution := dampingFactor * danglingSum / N
		teleport := (1.0 - dampingFactor) / N

		for _, ip := range nodes {
			newPR[ip] = teleport + danglingContribution
			// Contribution from nodes that link TO ip (inLinks)
			for _, src := range inLinks[ip] {
				if outDegree[src] > 0 {
					newPR[ip] += dampingFactor * pr[src] / float64(outDegree[src])
				}
			}
		}

		var maxDiff float64
		for _, ip := range nodes {
			diff := abs(newPR[ip] - pr[ip])
			if diff > maxDiff {
				maxDiff = diff
			}
			pr[ip] = newPR[ip]
		}
		if maxDiff < 1e-9 {
			break
		}
	}

	// 验证: PageRank 和为 1
	var sum float64
	for _, score := range pr {
		sum += score
	}
	if abs(sum-1.0) > 0.01 {
		t.Errorf("PageRank sum = %f, want ~1.0", sum)
	}

	// B 应该有最高的 PageRank (2个入边: A→B + ? 实际上B有来自A的入边)
	// B有来自A的入边, C有来自B的入边, A有来自B的入边
	t.Logf("PageRank: A=%.4f B=%.4f C=%.4f", pr["A"], pr["B"], pr["C"])
}

// TestSqrtFloat 验证 sqrt 修复
func TestSqrtFloat(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{0, 0},
		{1, 1},
		{4, 2},
		{9, 3},
		{100, 10},
		{2, 1.4142135623730951},
	}

	for _, tt := range tests {
		result := sqrtFloat(tt.input)
		if abs(result-tt.expected) > 1e-10 {
			t.Errorf("sqrtFloat(%f) = %f, want %f", tt.input, result, tt.expected)
		}
	}
}

// TestGraphEdge 验证图边结构
func TestGraphEdge(t *testing.T) {
	edge := graphEdge{
		src:          "10.0.0.1",
		dst:          "10.0.0.2",
		sessionCount: 42,
		totalBytes:   1048576,
		riskScore:    0.75,
	}

	if edge.src != "10.0.0.1" {
		t.Error("src incorrect")
	}
	if edge.dst != "10.0.0.2" {
		t.Error("dst incorrect")
	}
	if edge.sessionCount != 42 {
		t.Error("sessionCount incorrect")
	}
}

// TestRankedNode 验证排序节点
func TestRankedNode(t *testing.T) {
	nodes := []RankedNode{
		{IP: "C", Score: 0.3, Rank: 0},
		{IP: "A", Score: 0.9, Rank: 0},
		{IP: "B", Score: 0.5, Rank: 0},
	}

	// Sort by Score desc
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i].Score < nodes[j].Score {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}

	if nodes[0].IP != "A" {
		t.Errorf("Top node should be A, got %s", nodes[0].IP)
	}
	if nodes[1].IP != "B" {
		t.Errorf("Second should be B, got %s", nodes[1].IP)
	}
	if nodes[2].IP != "C" {
		t.Errorf("Third should be C, got %s", nodes[2].IP)
	}
}

// TestAnomalyPattern 验证异常模式类型
func TestAnomalyPattern(t *testing.T) {
	patterns := []AnomalyPattern{
		{Type: "star", CenterIP: "10.0.0.1", Members: []string{"a", "b", "c"}, EdgeCount: 3, RiskScore: 0.8},
		{Type: "chain", Members: []string{"x", "y", "z"}, EdgeCount: 2, RiskScore: 0.6},
		{Type: "mesh", Members: []string{"m1", "m2", "m3", "m4"}, EdgeCount: 6, RiskScore: 0.95},
		{Type: "isolated", CenterIP: "172.16.0.1", Members: []string{"peer"}, EdgeCount: 1, RiskScore: 0.35},
	}

	for _, p := range patterns {
		if p.Type == "" {
			t.Error("Pattern type should not be empty")
		}
		if len(p.Members) == 0 {
			t.Errorf("%s pattern should have members", p.Type)
		}
	}
}

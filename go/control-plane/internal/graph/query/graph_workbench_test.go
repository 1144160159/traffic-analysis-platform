package query

import "testing"

func TestFilterWorkbenchGraphKeepsFixedDepthNeighborhood(t *testing.T) {
	nodes := []*WorkbenchNode{
		{EntityID: "host:center", EntityType: "host", Metadata: map[string]interface{}{"site": "main"}},
		{EntityID: "account:user", EntityType: "account", Metadata: map[string]interface{}{"site": "main"}},
		{EntityID: "host:web", EntityType: "host", Metadata: map[string]interface{}{"site": "main"}},
		{EntityID: "host:remote", EntityType: "host", Metadata: map[string]interface{}{"site": "remote"}},
	}
	edges := []*WorkbenchEdge{
		{RelationID: "r1", SourceID: "host:center", TargetID: "account:user", ObservedAt: 200},
		{RelationID: "r2", SourceID: "account:user", TargetID: "host:web", ObservedAt: 200},
		{RelationID: "r3", SourceID: "host:web", TargetID: "host:remote", ObservedAt: 200},
	}

	graph := filterWorkbenchGraph(nodes, edges, WorkbenchFilter{
		CenterID: "host:center", Depth: 2, Site: "main", SinceMS: 100,
	})
	if graph.CenterID != "host:center" || len(graph.Nodes) != 3 || len(graph.Edges) != 2 {
		t.Fatalf("unexpected depth-two graph: center=%s nodes=%d edges=%d", graph.CenterID, len(graph.Nodes), len(graph.Edges))
	}

	threeHop := filterWorkbenchGraph(nodes, edges, WorkbenchFilter{
		CenterID: "host:center", Depth: 3, Site: "all", SinceMS: 100,
	})
	if len(threeHop.Nodes) != 4 || len(threeHop.Edges) != 3 {
		t.Fatalf("depth-three graph must add the third-hop node and edge: nodes=%d edges=%d", len(threeHop.Nodes), len(threeHop.Edges))
	}
	if threeHop.Nodes[3].EntityID != "host:remote" || threeHop.Edges[2].RelationID != "r3" {
		t.Fatalf("unexpected third-hop result: node=%s edge=%s", threeHop.Nodes[3].EntityID, threeHop.Edges[2].RelationID)
	}

	accounts := filterWorkbenchGraph(nodes, edges, WorkbenchFilter{
		CenterID: "host:center", Depth: 2, Site: "main", EntityType: "account", SinceMS: 100,
	})
	if len(accounts.Nodes) != 2 || len(accounts.Edges) != 1 {
		t.Fatalf("entity filter must retain center and matching nodes: nodes=%d edges=%d", len(accounts.Nodes), len(accounts.Edges))
	}

	limited := filterWorkbenchGraph(nodes, edges, WorkbenchFilter{
		CenterID: "host:center", Depth: 2, Site: "main", SinceMS: 100, Limit: 2,
	})
	if len(limited.Nodes) != 2 || len(limited.Edges) != 1 {
		t.Fatalf("node limit must be enforced with retained edges only: nodes=%d edges=%d", len(limited.Nodes), len(limited.Edges))
	}
}

func TestFilterWorkbenchGraphKeepsRequiredPathEndpointsAndStrictLimit(t *testing.T) {
	nodes := []*WorkbenchNode{
		{EntityID: "account:user", EntityType: "account", Metadata: map[string]interface{}{"site": "main"}},
		{EntityID: "host:web", EntityType: "host", Metadata: map[string]interface{}{"site": "main"}},
		{EntityID: "host:center", EntityType: "host", Metadata: map[string]interface{}{"site": "main"}},
	}
	edges := []*WorkbenchEdge{
		{RelationID: "r1", SourceID: "account:user", TargetID: "host:center", ObservedAt: 200},
		{RelationID: "r2", SourceID: "host:center", TargetID: "host:web", ObservedAt: 200},
	}

	filtered := filterWorkbenchGraph(nodes, edges, WorkbenchFilter{
		CenterID: "account:user", RequiredIDs: []string{"account:user", "host:center"}, Depth: 2,
		Site: "main", EntityType: "account", SinceMS: 100,
	})
	if len(filtered.Nodes) != 2 || len(filtered.Edges) != 1 {
		t.Fatalf("entity filter must retain required source and target: nodes=%d edges=%d", len(filtered.Nodes), len(filtered.Edges))
	}

	limited := filterWorkbenchGraph(nodes, edges, WorkbenchFilter{
		CenterID: "host:center", Depth: 2, Site: "main", SinceMS: 100, Limit: 2,
	})
	if len(limited.Nodes) != 2 {
		t.Fatalf("strict node limit must never return limit+1 nodes: nodes=%d", len(limited.Nodes))
	}
	if limited.Nodes[0].EntityID != "host:center" {
		t.Fatalf("center must be retained first under a strict limit: first=%s", limited.Nodes[0].EntityID)
	}
}

func TestFindDirectedWorkbenchSegmentHonorsRequiredAnchor(t *testing.T) {
	edgeDirect := &WorkbenchEdge{RelationID: "direct", SourceID: "source", TargetID: "target"}
	edgeToAnchor := &WorkbenchEdge{RelationID: "to-anchor", SourceID: "source", TargetID: "anchor"}
	edgeFromAnchor := &WorkbenchEdge{RelationID: "from-anchor", SourceID: "anchor", TargetID: "target"}
	adjacency := map[string][]workbenchPathStep{
		"source": {
			{nodeID: "target", edge: edgeDirect},
			{nodeID: "anchor", edge: edgeToAnchor},
		},
		"anchor": {{nodeID: "target", edge: edgeFromAnchor}},
	}

	firstNodes, firstEdges, found := findDirectedWorkbenchSegment(adjacency, "source", "anchor", 3)
	if !found {
		t.Fatal("expected source-to-anchor segment")
	}
	secondNodes, secondEdges, found := findDirectedWorkbenchSegment(adjacency, "anchor", "target", 3-len(firstEdges))
	if !found {
		t.Fatal("expected anchor-to-target segment")
	}
	nodeIDs := append(firstNodes, secondNodes[1:]...)
	edges := append(firstEdges, secondEdges...)
	if len(nodeIDs) != 3 || nodeIDs[1] != "anchor" || len(edges) != 2 || edges[0].RelationID != "to-anchor" || edges[1].RelationID != "from-anchor" {
		t.Fatalf("anchored route must not be replaced by a shorter bypass: nodes=%v edges=%v", nodeIDs, []string{edges[0].RelationID, edges[1].RelationID})
	}
}

func TestWorkbenchPathModesUsePersistedRelationshipSemantics(t *testing.T) {
	tests := []struct {
		mode       string
		relation   string
		risk       string
		attributes map[string]interface{}
		want       bool
	}{
		{mode: "shortest", relation: "证据引用", risk: "low", want: true},
		{mode: "attack", relation: "关联告警", risk: "low", want: true},
		{mode: "attack", relation: "登录", risk: "high", attributes: map[string]interface{}{"attack_stage": "凭证访问"}, want: true},
		{mode: "attack", relation: "登录", risk: "high", want: false},
		{mode: "attack", relation: "通信", risk: "low", want: false},
		{mode: "communication", relation: "DNS解析", risk: "medium", want: true},
		{mode: "communication", relation: "行为服务", risk: "low", want: true},
		{mode: "communication", relation: "账号访问", risk: "medium", want: false},
		{mode: "account", relation: "登录", risk: "low", attributes: map[string]interface{}{"identity_label": "高权限"}, want: true},
		{mode: "account", relation: "账号访问", risk: "medium", attributes: map[string]interface{}{"identity_label": "横向访问"}, want: true},
		{mode: "account", relation: "登录", risk: "low", want: false},
		{mode: "account", relation: "通信", risk: "high", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.mode+"/"+tt.relation+"/"+tt.risk, func(t *testing.T) {
			got := workbenchEdgeMatchesMode(&WorkbenchEdge{RelationType: tt.relation, RiskLevel: tt.risk, Attributes: tt.attributes}, tt.mode)
			if got != tt.want {
				t.Fatalf("workbenchEdgeMatchesMode()=%v, want %v", got, tt.want)
			}
		})
	}
}

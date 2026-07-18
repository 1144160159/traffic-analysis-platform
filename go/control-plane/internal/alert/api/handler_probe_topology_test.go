package api

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

func TestBuildProbeTopologyGraphReturnsAPILayouts(t *testing.T) {
	generatedAt := time.Unix(1_700_000_000, 0).UTC()
	graph := buildProbeTopologyGraph([]probeDTO{
		{
			ProbeID: "probe-a", Location: "图书馆", Status: "在线", BandwidthMbps: 18_600,
			TopologyX: 20, TopologyY: 30, TopologyZ: 6, TopologyZone: "教学区", TopologyRole: "接入探针",
			TopologyLinks: []string{"probe-b"}, TopologyLinkBandwidths: []float64{40},
		},
		{
			ProbeID: "probe-b", Location: "数据中心", Status: "告警", BandwidthMbps: 42_000,
			TopologyX: 72, TopologyY: 66, TopologyZ: 10, TopologyZone: "核心区", TopologyRole: "核心交换",
			TopologyLinks: []string{"probe-a"}, TopologyLinkBandwidths: []float64{40},
		},
	}, "3d", generatedAt)

	if graph.Source != "postgres.probes.hardware_info" || graph.CoordinateSystem != "normalized-0-100" {
		t.Fatalf("unexpected graph metadata: %+v", graph)
	}
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 || len(graph.Zones) != 2 {
		t.Fatalf("unexpected graph cardinality: nodes=%d edges=%d zones=%d", len(graph.Nodes), len(graph.Edges), len(graph.Zones))
	}
	if graph.Nodes[0].Position2D == graph.Nodes[0].Position3D {
		t.Fatalf("expected independent API layouts, got %+v", graph.Nodes[0])
	}
	if graph.Edges[0].Kind != "backbone" || graph.Edges[0].Status != "warn" {
		t.Fatalf("unexpected edge semantics: %+v", graph.Edges[0])
	}
	if graph.GeneratedAt != generatedAt {
		t.Fatalf("generated timestamp drifted: %s", graph.GeneratedAt)
	}
}

func TestBuildProbeTopologyGraphSpreadsCollidingNodes(t *testing.T) {
	graph := buildProbeTopologyGraph([]probeDTO{
		{ProbeID: "probe-a", Status: "在线", TopologyX: 10, TopologyY: 18, TopologyZ: 2},
		{ProbeID: "probe-b", Status: "在线", TopologyX: 10.1, TopologyY: 18.1, TopologyZ: 2},
		{ProbeID: "probe-c", Status: "在线", TopologyX: 10.2, TopologyY: 18.2, TopologyZ: 2},
		{ProbeID: "probe-d", Status: "在线", TopologyX: 10.3, TopologyY: 18.3, TopologyZ: 2},
	}, "2d", time.Unix(1, 0).UTC())
	for left := range graph.Nodes {
		for right := left + 1; right < len(graph.Nodes); right++ {
			for _, points := range [][2]probeTopologyPointDTO{{graph.Nodes[left].Position2D, graph.Nodes[right].Position2D}, {graph.Nodes[left].Position3D, graph.Nodes[right].Position3D}} {
				if distance := math.Hypot(points[0].X-points[1].X, points[0].Y-points[1].Y); distance < 6.8 {
					t.Fatalf("positions remain too close (%.2f): %+v", distance, graph.Nodes)
				}
			}
		}
	}
}

func TestBuildProbeTopologyGraphMergesBidirectionalBandwidth(t *testing.T) {
	graph := buildProbeTopologyGraph([]probeDTO{
		{ProbeID: "probe-a", Status: "在线", TopologyX: 10, TopologyY: 18, TopologyLinks: []string{"probe-b"}, TopologyLinkBandwidths: []float64{10}},
		{ProbeID: "probe-b", Status: "告警", TopologyX: 80, TopologyY: 72, TopologyLinks: []string{"probe-a"}, TopologyLinkBandwidths: []float64{40}},
	}, "2d", time.Unix(1, 0).UTC())
	if len(graph.Edges) != 1 || graph.Edges[0].BandwidthGbps != 40 || graph.Edges[0].Kind != "backbone" || graph.Edges[0].Status != "warn" {
		t.Fatalf("bidirectional edge did not merge deterministically: %+v", graph.Edges)
	}
}

func TestProbeTopologyPermissionExcludesMetricsOnly(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	for _, test := range []struct {
		permission string
		allowed    bool
	}{{authmodel.ScopeProbeMetrics, false}, {authmodel.ScopeProbeRead, true}, {authmodel.ScopeProbeWrite, true}} {
		request := httptest.NewRequest(http.MethodGet, "/probes/topology", nil)
		ctx := context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{test.permission})
		recorder := httptest.NewRecorder()
		allowed := handler.requireProbeTopologyReadPermission(recorder, request.WithContext(ctx))
		if allowed != test.allowed {
			t.Fatalf("permission %s allowed=%v, want %v", test.permission, allowed, test.allowed)
		}
		if !test.allowed && recorder.Code != http.StatusForbidden {
			t.Fatalf("permission %s returned %d", test.permission, recorder.Code)
		}
	}
}

func TestNormalizeProbeStatusHonorsExplicitFixtureState(t *testing.T) {
	stale := sql.NullTime{Valid: true, Time: time.Now().Add(-30 * time.Minute)}
	fixture := map[string]interface{}{"fixture": "probes-ui-v1"}
	if got := normalizeProbeStatus("active", stale, fixture); got != "online" {
		t.Fatalf("fixture active status drifted to %s", got)
	}
	if got := normalizeProbeStatus("active", stale, nil); got != "offline" {
		t.Fatalf("production stale heartbeat should be offline, got %s", got)
	}
}

func TestBuildProbeTopologyGraphSkipsUnplacedAndDanglingNodes(t *testing.T) {
	graph := buildProbeTopologyGraph([]probeDTO{
		{ProbeID: "placed", Status: "在线", TopologyX: 10, TopologyY: 20, TopologyZ: 1, TopologyLinks: []string{"missing"}},
		{ProbeID: "unplaced", Status: "在线"},
	}, "2d", time.Unix(1, 0).UTC())
	if len(graph.Nodes) != 1 || graph.Nodes[0].ID != "placed" {
		t.Fatalf("unexpected nodes: %+v", graph.Nodes)
	}
	if len(graph.Edges) != 0 {
		t.Fatalf("dangling edge was not removed: %+v", graph.Edges)
	}
}

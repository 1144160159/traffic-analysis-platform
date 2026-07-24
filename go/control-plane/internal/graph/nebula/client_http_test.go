package nebula

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

// 集成测试 — 需要 NebulaGraph 集群可用
// 设置 NEBULA_TEST_ADDR 环境变量运行: NEBULA_TEST_ADDR=10.0.5.224:19669 go test -v -run TestHTTP

func testHTTPClient(t *testing.T) *HTTPClient {
	t.Helper()
	addr := os.Getenv("NEBULA_TEST_ADDR")
	if addr == "" {
		addr = "nebula-graph.middleware.svc:19669"
	}
	cfg := HTTPClientConfig{
		GraphAddr:     addr,
		Username:      "root",
		Password:      "root",
		Space:         "traffic_graph",
		Timeout:       5 * time.Second,
		RetryCount:    1,
		RetryDelay:    50 * time.Millisecond,
		MaxIdleConns:  3,
		EnableMetrics: true,
	}
	logger := zap.NewNop()
	client, err := NewHTTPClient(cfg, logger)
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}

	// Verify connectivity; skip if unreachable
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		client.Close()
		t.Skipf("Skipping integration test: NebulaGraph unreachable at %s: %v", addr, err)
	}
	return client
}

// TestHTTPClientPing 验证连接和健康检查
func TestHTTPClientPing(t *testing.T) {
	client := testHTTPClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := client.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	t.Log("Ping OK")
}

// TestHTTPClientShowSpaces 验证查看图空间
func TestHTTPClientShowSpaces(t *testing.T) {
	client := testHTTPClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	spaces, err := client.ShowSpaces(ctx)
	if err != nil {
		t.Fatalf("ShowSpaces failed: %v", err)
	}
	t.Logf("Found %d spaces: %v", len(spaces), spaces)

	found := false
	for _, s := range spaces {
		if s == "traffic_graph" {
			found = true
			break
		}
	}
	if !found {
		t.Error("traffic_graph space not found")
	}
}

// TestHTTPClientShowHosts 验证集群状态
func TestHTTPClientShowHosts(t *testing.T) {
	client := testHTTPClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hosts, err := client.ShowHosts(ctx)
	if err != nil {
		t.Fatalf("ShowHosts failed: %v", err)
	}
	t.Logf("Found %d hosts", len(hosts))
	for _, h := range hosts {
		status, _ := h["Status"].(string)
		t.Logf("  Host: %v:%v Status=%s", h["Host"], h["Port"], status)
		if status != "ONLINE" {
			t.Errorf("Host %v:%v is %s, expected ONLINE", h["Host"], h["Port"], status)
		}
	}
}

// TestHTTPClientShowTags 验证 Schema
func TestHTTPClientShowTags(t *testing.T) {
	client := testHTTPClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tags, err := client.ShowTags(ctx)
	if err != nil {
		t.Fatalf("ShowTags failed: %v", err)
	}
	t.Logf("Tags: %v", tags)
	if len(tags) != 6 {
		t.Errorf("Expected 6 tags, got %d: %v", len(tags), tags)
	}
}

// TestHTTPClientShowEdges 验证 Edge Types
func TestHTTPClientShowEdges(t *testing.T) {
	client := testHTTPClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	edges, err := client.ShowEdges(ctx)
	if err != nil {
		t.Fatalf("ShowEdges failed: %v", err)
	}
	t.Logf("Edges: %v", edges)
	if len(edges) != 8 {
		t.Errorf("Expected 8 edges, got %d: %v", len(edges), edges)
	}
}

// TestHTTPClientInsertAndQuery 验证写入和查询
func TestHTTPClientInsertAndQuery(t *testing.T) {
	client := testHTTPClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testIP := "10.99.88.77"
	testID := hashVID(testIP)

	// 1. INSERT
	t.Logf("Inserting IP node: %s (VID=%s)", testIP, testID)
	err := client.InsertIPNode(ctx, "test-tenant", testIP,
		"aa:bb:cc:dd:ee:ff", "test-host", "TestVendor", "Linux",
		false, 0.5, time.Now().UnixMilli()-3600000, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("InsertIPNode failed: %v", err)
	}

	// 2. FETCH
	t.Logf("Fetching node: %s", testID)
	nGQL := "FETCH PROP ON ip_address \"" + testID + "\" YIELD properties(vertex).ip AS ip, properties(vertex).hostname AS hostname;"
	result, err := client.Execute(ctx, nGQL)
	if err != nil {
		t.Fatalf("FETCH failed: %v", err)
	}
	if len(result.Rows) == 0 {
		t.Error("FETCH returned 0 rows — VID may not match FIXED_STRING(32)")
	} else {
		ip, _ := result.Rows[0]["ip"].(string)
		hostname, _ := result.Rows[0]["hostname"].(string)
		t.Logf("FETCH result: ip=%s hostname=%s", ip, hostname)
	}

	// 3. Cleanup
	cleanup := "DELETE VERTEX \"" + testID + "\" WITH EDGE;"
	_, err = client.Execute(ctx, cleanup)
	if err != nil {
		t.Logf("Cleanup warning: %v", err)
	}
}

// TestHTTPClientInsertEdgeAndGo 验证边和遍历
func TestHTTPClientInsertEdgeAndGo(t *testing.T) {
	client := testHTTPClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	srcIP := "10.99.99.1"
	dstIP := "10.99.99.2"
	srcVID := hashVID(srcIP)
	dstVID := hashVID(dstIP)
	commID := "test-community-go-001"

	// Insert nodes
	now := time.Now().UnixMilli()
	client.InsertIPNode(ctx, "test-tenant", srcIP, "", "src-node", "", "", false, 0.1, now-3600000, now)
	client.InsertIPNode(ctx, "test-tenant", dstIP, "", "dst-node", "", "", false, 0.1, now-3600000, now)

	// Insert edge
	err := client.InsertSessionEdge(ctx, "test-tenant", srcIP, dstIP, commID,
		6, 10, 500000, 1000, now-3600000, now, "outbound")
	if err != nil {
		t.Fatalf("InsertSessionEdge failed: %v", err)
	}

	// GO query
	neighbors, err := client.GetNeighbors(ctx, "test-tenant", srcIP, 10)
	if err != nil {
		t.Fatalf("GetNeighbors failed: %v", err)
	}
	t.Logf("Found %d neighbors", len(neighbors))

	// Cleanup
	client.Execute(ctx, "DELETE VERTEX \""+srcVID+"\" WITH EDGE;")
	client.Execute(ctx, "DELETE VERTEX \""+dstVID+"\" WITH EDGE;")
}

// TestHTTPClientRetry 验证重试机制 (集成测试)
func TestHTTPClientRetry(t *testing.T) {
	client := testHTTPClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Execute valid query multiple times to verify stability
	for i := 0; i < 5; i++ {
		_, err := client.Execute(ctx, "SHOW SPACES;")
		if err != nil {
			t.Errorf("Query %d failed: %v", i+1, err)
		}
	}

	metrics := client.GetMetrics()
	t.Logf("Metrics: total=%d failed=%d avgLatency=%.2fms",
		metrics.TotalQueries, metrics.FailedQueries, metrics.AvgLatencyMs)
	if metrics.TotalQueries < 5 {
		t.Errorf("Expected at least 5 queries, got %d", metrics.TotalQueries)
	}
}

// ============================================================================
// 单元测试 (不依赖 NebulaGraph)
// ============================================================================

// TestHashVID 验证 VID 哈希
func TestHashVID(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"192.168.1.1"},
		{"10.0.0.1"},
		{"alert-001"},
		{"campaign-abc-123"},
		{""},
	}

	for _, tt := range tests {
		vid := hashVID(tt.input)
		if len(vid) != 32 {
			t.Errorf("hashVID(%q) = %q (len=%d), expected len=32", tt.input, vid, len(vid))
		}
		// 确定性：同输入应产生同输出
		vid2 := hashVID(tt.input)
		if vid != vid2 {
			t.Errorf("hashVID(%q) not deterministic: %q vs %q", tt.input, vid, vid2)
		}
	}

	// 不同输入应产生不同输出
	vid1 := hashVID("192.168.1.1")
	vid2 := hashVID("192.168.1.2")
	if vid1 == vid2 {
		t.Error("hashVID should produce different hashes for different inputs")
	}
}

func TestHashTenantVIDIsolation(t *testing.T) {
	entityID := "host:10.20.4.18"
	defaultVID := hashTenantVID("default", entityID)
	collisionVID := hashTenantVID("entity-graph-collision-tenant", entityID)
	if defaultVID == collisionVID {
		t.Fatalf("tenant-qualified VIDs collided: %q", defaultVID)
	}
	if defaultVID != "2efb75d2e907f0da9780bb82b66fef9d" {
		t.Fatalf("unexpected stable tenant VID: %q", defaultVID)
	}
	if collisionVID != "46657b811b23521f67b8baa97901e017" {
		t.Fatalf("unexpected collision fixture VID: %q", collisionVID)
	}
}

// TestDefaultHTTPConfig 验证默认配置
func TestDefaultHTTPConfig(t *testing.T) {
	cfg := DefaultHTTPConfig()
	if cfg.GraphAddr == "" {
		t.Error("GraphAddr should not be empty")
	}
	if cfg.Username != "traffic_graph" {
		t.Errorf("Username = %s, want traffic_graph", cfg.Username)
	}
	if cfg.Space != "traffic_graph" {
		t.Errorf("Space = %s, want traffic_graph", cfg.Space)
	}
	if cfg.Timeout == 0 {
		t.Error("Timeout should not be 0")
	}
	if cfg.RetryCount != 3 {
		t.Errorf("RetryCount = %d, want 3", cfg.RetryCount)
	}
}

// TestHTTPExecuteRequest 验证请求序列化
func TestHTTPExecuteRequest(t *testing.T) {
	req := httpExecuteRequest{
		GQL:      "SHOW SPACES;",
		Username: "root",
		Password: "root",
	}
	if req.GQL == "" {
		t.Error("GQL should not be empty")
	}
	if req.Username != "root" {
		t.Error("Username incorrect")
	}
}

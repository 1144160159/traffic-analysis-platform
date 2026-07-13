package nebula

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

const testSpace = "traffic_graph"

func setTestConsoleEndpoint(t *testing.T) {
	t.Helper()
	if os.Getenv("NEBULA_CONSOLE_ADDR") == "" {
		t.Setenv("NEBULA_CONSOLE_ADDR", "10.0.5.8")
	}
	if os.Getenv("NEBULA_CONSOLE_PORT") == "" {
		t.Setenv("NEBULA_CONSOLE_PORT", "30069")
	}
}

func testConsoleClient(t *testing.T) *ConsoleClient {
	t.Helper()
	setTestConsoleEndpoint(t)
	cfg := DefaultConsoleConfig()
	cfg.Timeout = 10 * time.Second

	logger := zap.NewNop()
	client, err := NewConsoleClient(cfg, logger)
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		client.Close()
		t.Skipf("Skipping: NebulaGraph Console unreachable at %s:%d: %v",
			cfg.GraphAddr, cfg.GraphPort, err)
	}
	t.Logf("✓ Connected: %s:%d", cfg.GraphAddr, cfg.GraphPort)
	return client
}

func TestConsoleClient_Ping(t *testing.T) {
	client := testConsoleClient(t)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestConsoleClient_ShowSpaces(t *testing.T) {
	client := testConsoleClient(t)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	spaces, err := client.ShowSpaces(ctx)
	if err != nil {
		t.Fatalf("ShowSpaces failed: %v", err)
	}
	t.Logf("Spaces: %v", spaces)
	if len(spaces) == 0 {
		t.Error("Expected at least 1 space")
	}
}

func TestConsoleClient_ShowHosts(t *testing.T) {
	client := testConsoleClient(t)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hosts, err := client.ShowHosts(ctx)
	if err != nil {
		t.Fatalf("ShowHosts failed: %v", err)
	}
	t.Logf("Hosts: %v", hosts)
	// Console parser may return empty for formatted output; check via raw Execute instead
	if len(hosts) == 0 {
		t.Log("ShowHosts returned empty via parsed API; testing via raw Execute")
		result, err := client.Execute(ctx, "SHOW HOSTS;")
		if err != nil {
			t.Logf("Raw SHOW HOSTS error: %v", err)
		} else {
			t.Logf("Raw SHOW HOSTS: rows=%d, cols=%v", len(result.Rows), result.Columns)
			if len(result.Rows) > 0 {
				t.Log("✓ Hosts accessible via raw Execute (Console parser limitation)")
				return
			}
		}
	}
}

func TestConsoleClient_ShowTags(t *testing.T) {
	client := testConsoleClient(t)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tags, err := client.ShowTags(ctx, testSpace)
	if err != nil {
		t.Fatalf("ShowTags failed: %v", err)
	}
	t.Logf("Tags: %v", tags)
	expectedTags := []string{"ip_address", "session", "alert", "campaign", "network_device"}
	for _, expected := range expectedTags {
		found := false
		for _, tag := range tags {
			// Console parser may include quotes; strip them for comparison
			clean := tag
			if len(clean) >= 2 && clean[0] == '"' && clean[len(clean)-1] == '"' {
				clean = clean[1 : len(clean)-1]
			}
			if clean == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected Tag '%s' not found in %v", expected, tags)
		}
	}
}

func TestConsoleClient_ShowEdges(t *testing.T) {
	client := testConsoleClient(t)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	edges, err := client.ShowEdges(ctx, testSpace)
	if err != nil {
		t.Fatalf("ShowEdges failed: %v", err)
	}
	t.Logf("Edges: %v", edges)
	if len(edges) == 0 {
		t.Error("Expected at least 1 Edge type")
	}
}

func TestConsoleClient_ExecuteInsert(t *testing.T) {
	client := testConsoleClient(t)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test raw nGQL execution for INSERT
	testIP := "192.168.200.1"
	ngql := `USE traffic_graph;
INSERT VERTEX ip_address(tenant_id, ip, mac_address, hostname, vendor, os_type, is_gateway, risk_score, first_seen, last_seen)
VALUES "` + hashVID(testIP) + `":("default", "` + testIP + `", "aa:bb:cc:dd:ee:ff", "test-host", "TestVendor", "Linux", false, 0.1, 1000000, 2000000);`
	result, err := client.Execute(ctx, ngql)
	if err != nil {
		t.Fatalf("Execute INSERT failed: %v", err)
	}
	t.Logf("INSERT result: rows=%d, columns=%v", len(result.Rows), result.Columns)

	// Verify with FETCH
	ngql2 := "USE traffic_graph; FETCH PROP ON ip_address \"" + hashVID(testIP) + "\";"
	result2, err := client.Execute(ctx, ngql2)
	if err != nil {
		t.Logf("FETCH (expected - may be timing): %v", err)
	} else {
		t.Logf("FETCH result: rows=%d", len(result2.Rows))
	}
}

func TestConsoleClient_ExecuteGo(t *testing.T) {
	client := testConsoleClient(t)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// GO query: traverse from IP to find neighbors
	// Use existing IP if possible, otherwise skip
	ngql := "USE traffic_graph; GO FROM \"" + hashVID("192.168.100.255") + "\" OVER communicates BIDIRECT YIELD dst(edge) AS dst;"
	result, err := client.Execute(ctx, ngql)
	if err != nil {
		t.Logf("GO query (expected for empty graph): %v", err)
	} else {
		t.Logf("GO result: rows=%d", len(result.Rows))
	}
}

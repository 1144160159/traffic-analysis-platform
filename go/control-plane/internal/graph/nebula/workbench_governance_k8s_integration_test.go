package nebula

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/query"
)

func TestGovernedWorkbenchNebulaK8sIntegration(t *testing.T) {
	if os.Getenv("RUN_M09_N014_NEBULA_INTEGRATION") != "1" {
		t.Skip("set RUN_M09_N014_NEBULA_INTEGRATION=1 inside the guarded K8s runner")
	}
	prefix := strings.TrimSpace(os.Getenv("M09_N014_EPHEMERAL_TENANT_PREFIX"))
	if !strings.HasPrefix(prefix, "k8s-m09-n014-") {
		t.Fatal("refusing to mutate NebulaGraph without a run-scoped K8s tenant prefix")
	}
	password := os.Getenv("NEBULA_PASSWORD")
	if password == "" {
		t.Fatal("NEBULA_PASSWORD is required")
	}
	store, err := NewWorkbenchStore(config.NebulaConfig{
		Enabled: true, Addresses: []string{envOr("NEBULA_ADDRESS", "nebula-graph.middleware.svc:9669")},
		Username: envOr("NEBULA_USERNAME", "traffic_graph"), Password: password,
		Space: envOr("NEBULA_SPACE", "traffic_graph"), Timeout: 10 * time.Second,
		IdleTime: time.Minute, MaxPoolSize: 4, MinPoolSize: 1,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("connect K8s NebulaGraph: %v", err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := store.Ready(ctx); err != nil {
		t.Fatal(err)
	}

	superTenant := prefix + "-super"
	otherTenant := prefix + "-other"
	cycleTenant := prefix + "-cycle"
	fixtures := make([]governedFixture, 0, 80)
	addNode := func(tenantID, entityID string) {
		fixtures = append(fixtures, governedFixture{tenantID: tenantID, entityID: entityID})
		insertGovernedFixtureNode(t, ctx, store, tenantID, entityID)
	}
	addEdge := func(tenantID, relationID, sourceTenant, sourceID, targetTenant, targetID string) {
		fixtures = append(fixtures, governedFixture{
			tenantID: tenantID, relationID: relationID, sourceTenant: sourceTenant,
			sourceID: sourceID, targetTenant: targetTenant, targetID: targetID,
		})
		insertGovernedFixtureEdge(t, ctx, store, tenantID, relationID, sourceTenant, sourceID, targetTenant, targetID)
	}
	defer func() { cleanupGovernedFixtures(t, store, fixtures) }()

	addNode(superTenant, "host:center")
	for index := 0; index < 60; index++ {
		entityID := fmt.Sprintf("host:neighbor-%03d", index)
		addNode(superTenant, entityID)
		addEdge(superTenant, fmt.Sprintf("relation-%03d", index), superTenant, "host:center", superTenant, entityID)
	}
	addNode(otherTenant, "host:foreign")
	addEdge(otherTenant, "relation-cross-tenant", superTenant, "host:center", otherTenant, "host:foreign")

	nodes, edges, truncated, reason, err := store.LoadWorkbenchGraphBounded(ctx, superTenant, query.WorkbenchFilter{
		CenterID: "host:center", Depth: 2, Limit: 20, EdgeLimit: 100, NeighborLimit: 10,
		UntilMS: time.Now().Add(time.Minute).UnixMilli(),
	})
	if err != nil {
		t.Fatalf("bounded supernode traversal: %v", err)
	}
	if !truncated || reason != "hop_neighbor_budget" || len(nodes) > 11 || len(edges) > 10 {
		t.Fatalf("supernode budget not enforced: nodes=%d edges=%d truncated=%v reason=%s", len(nodes), len(edges), truncated, reason)
	}
	for _, node := range nodes {
		if node.EntityID == "host:foreign" {
			t.Fatal("cross-tenant vertex escaped tenant VID and relation filters")
		}
	}
	for _, edge := range edges {
		if edge.RelationID == "relation-cross-tenant" {
			t.Fatal("cross-tenant relation escaped tenant predicate")
		}
	}

	for _, id := range []string{"a", "b", "c"} {
		addNode(cycleTenant, id)
	}
	addEdge(cycleTenant, "cycle-ab", cycleTenant, "a", cycleTenant, "b")
	addEdge(cycleTenant, "cycle-bc", cycleTenant, "b", cycleTenant, "c")
	addEdge(cycleTenant, "cycle-ca", cycleTenant, "c", cycleTenant, "a")
	cycleNodes, cycleEdges, cycleTruncated, _, err := store.LoadWorkbenchGraphBounded(ctx, cycleTenant, query.WorkbenchFilter{
		CenterID: "a", Depth: 6, Limit: 20, EdgeLimit: 100, NeighborLimit: 100,
		UntilMS: time.Now().Add(time.Minute).UnixMilli(),
	})
	if err != nil {
		t.Fatalf("cycle traversal: %v", err)
	}
	if cycleTruncated || len(cycleNodes) != 3 || len(cycleEdges) != 3 {
		t.Fatalf("cycle traversal was not finite and exact: nodes=%d edges=%d truncated=%v", len(cycleNodes), len(cycleEdges), cycleTruncated)
	}
}

func TestGovernedWorkbenchNebulaK8sCleanupOracle(t *testing.T) {
	if os.Getenv("RUN_M09_N014_NEBULA_INTEGRATION") != "1" {
		t.Skip("set RUN_M09_N014_NEBULA_INTEGRATION=1 inside the guarded K8s runner")
	}
	prefix := strings.TrimSpace(os.Getenv("M09_N014_EPHEMERAL_TENANT_PREFIX"))
	if !strings.HasPrefix(prefix, "k8s-m09-n014-") || os.Getenv("NEBULA_PASSWORD") == "" {
		t.Fatal("cleanup oracle requires the guarded run prefix and NebulaGraph credential")
	}
	store, err := NewWorkbenchStore(config.NebulaConfig{
		Enabled: true, Addresses: []string{envOr("NEBULA_ADDRESS", "nebula-graph.middleware.svc:9669")},
		Username: envOr("NEBULA_USERNAME", "traffic_graph"), Password: os.Getenv("NEBULA_PASSWORD"),
		Space: envOr("NEBULA_SPACE", "traffic_graph"), Timeout: 10 * time.Second,
		IdleTime: time.Minute, MaxPoolSize: 2, MinPoolSize: 1,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, tenantID := range []string{prefix + "-super", prefix + "-other", prefix + "-cycle"} {
		parameters := map[string]interface{}{"tenant_id": tenantID}
		nodes, queryErr := store.execute(ctx, workbenchNodeQuery, parameters)
		if queryErr != nil {
			t.Fatalf("query run-scoped node cleanup for %s: %v", tenantID, queryErr)
		}
		if nodes.GetRowSize() != 0 {
			t.Fatalf("run-scoped nodes remain for %s: rows=%d", tenantID, nodes.GetRowSize())
		}
		edges, queryErr := store.execute(ctx, workbenchEdgeQuery, parameters)
		if queryErr != nil {
			t.Fatalf("query run-scoped edge cleanup for %s: %v", tenantID, queryErr)
		}
		if edges.GetRowSize() != 0 {
			t.Fatalf("run-scoped edges remain for %s: rows=%d", tenantID, edges.GetRowSize())
		}
	}
}

type governedFixture struct {
	tenantID, relationID, sourceTenant, sourceID, targetTenant, targetID, entityID string
}

func insertGovernedFixtureNode(t *testing.T, ctx context.Context, store *WorkbenchStore, tenantID, entityID string) {
	t.Helper()
	statement := fmt.Sprintf(`INSERT VERTEX entity(
tenant_id,entity_id,entity_type,label,detail,risk_score,risk_level,x,y,icon,metadata_json,updated_at
) VALUES %s:($tenant_id,$entity_id,"host",$entity_id,$entity_id,1,"low",0.0,0.0,"host","{}",$updated_at);`, tenantVIDLiteral(tenantID, entityID))
	if _, err := store.execute(ctx, statement, map[string]interface{}{
		"tenant_id": tenantID, "entity_id": entityID, "updated_at": int(time.Now().UnixMilli()),
	}); err != nil {
		t.Fatalf("insert fixture node %s/%s: %v", tenantID, entityID, err)
	}
}

func insertGovernedFixtureEdge(t *testing.T, ctx context.Context, store *WorkbenchStore, tenantID, relationID, sourceTenant, sourceID, targetTenant, targetID string) {
	t.Helper()
	statement := fmt.Sprintf(`INSERT EDGE relation(
tenant_id,relation_id,source_id,target_id,relation_type,risk_level,evidence_id,attributes_json,weight,observed_at
) VALUES %s->%s@0:($tenant_id,$relation_id,$source_id,$target_id,"communication","low","evidence-1","{}",1.0,$observed_at);`,
		tenantVIDLiteral(sourceTenant, sourceID), tenantVIDLiteral(targetTenant, targetID))
	if _, err := store.execute(ctx, statement, map[string]interface{}{
		"tenant_id": tenantID, "relation_id": relationID, "source_id": sourceID,
		"target_id": targetID, "observed_at": int(time.Now().UnixMilli()),
	}); err != nil {
		t.Fatalf("insert fixture edge %s: %v", relationID, err)
	}
}

func cleanupGovernedFixtures(t *testing.T, store *WorkbenchStore, fixtures []governedFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for index := len(fixtures) - 1; index >= 0; index-- {
		fixture := fixtures[index]
		var statement string
		if fixture.relationID != "" {
			statement = fmt.Sprintf("DELETE EDGE relation %s->%s@0;", tenantVIDLiteral(fixture.sourceTenant, fixture.sourceID), tenantVIDLiteral(fixture.targetTenant, fixture.targetID))
		} else {
			statement = fmt.Sprintf("DELETE VERTEX %s WITH EDGE;", tenantVIDLiteral(fixture.tenantID, fixture.entityID))
		}
		if _, err := store.execute(ctx, statement, nil); err != nil {
			t.Errorf("cleanup run-scoped fixture failed: %v", err)
		}
	}
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

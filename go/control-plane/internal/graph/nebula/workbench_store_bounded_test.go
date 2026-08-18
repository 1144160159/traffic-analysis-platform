package nebula

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/query"
)

func TestAssetProjectionQueriesAreTenantAssetAndRelationBounded(t *testing.T) {
	if !strings.Contains(assetProjectionNodeQuery, "FETCH PROP ON entity %s") {
		t.Fatalf("node query is not a direct deterministic-VID fetch: %s", assetProjectionNodeQuery)
	}
	if !strings.Contains(assetProjectionEdgeQuery, "GO FROM %s OVER relation BIDIRECT") || !strings.Contains(assetProjectionEdgeQuery, "relation.tenant_id == $tenant_id") {
		t.Fatalf("edge query is not a tenant-filtered adjacency traversal: %s", assetProjectionEdgeQuery)
	}
}

func TestGovernedWorkbenchQueriesCarryDatabaseBoundsAndTenantPredicates(t *testing.T) {
	if !strings.Contains(boundedWorkbenchLandingQuery, "entity.tenant_id == $tenant_id") ||
		!strings.Contains(boundedWorkbenchLandingQuery, "LIMIT 1") {
		t.Fatalf("landing query is not tenant and result bounded: %s", boundedWorkbenchLandingQuery)
	}
	for _, fragment := range []string{"GO FROM %s", "relation.tenant_id == $tenant_id", "ORDER BY $-.relation_id", "LIMIT %d"} {
		if !strings.Contains(boundedWorkbenchEdgeQuery, fragment) {
			t.Fatalf("bounded edge query missing %q: %s", fragment, boundedWorkbenchEdgeQuery)
		}
	}
}

func TestGovernedWorkbenchRejectsInvalidBudgetsBeforeStoreAccess(t *testing.T) {
	var store *WorkbenchStore
	_, _, _, _, err := store.LoadWorkbenchGraphBounded(context.Background(), "tenant-a", query.WorkbenchFilter{
		Depth: 3, Limit: 0, EdgeLimit: 100, NeighborLimit: 10,
	})
	if err == nil {
		t.Fatal("invalid node budget reached the store")
	}
}

func TestTenantVIDLiteralNeverContainsSourceIdentifiers(t *testing.T) {
	literal := tenantVIDLiteral(`tenant\"; DELETE TAG entity;`, `asset\"; DROP SPACE traffic_graph;`)
	if !regexp.MustCompile(`^"[0-9a-f]{32}"$`).MatchString(literal) {
		t.Fatalf("tenant VID literal is not a fixed quoted hash: %q", literal)
	}
}

func TestLoadAssetProjectionRejectsInvalidBoundsBeforeStoreAccess(t *testing.T) {
	var store *WorkbenchStore
	for _, tc := range []struct {
		tenant, asset string
		limit         int
	}{
		{"", "asset-1", 10},
		{"tenant-a", "", 10},
		{"tenant-a", "asset-1", 0},
		{"tenant-a", "asset-1", 501},
	} {
		if _, _, _, err := store.LoadAssetProjection(context.Background(), tc.tenant, tc.asset, tc.limit); err == nil {
			t.Fatalf("invalid bounds %+v should fail", tc)
		}
	}
}

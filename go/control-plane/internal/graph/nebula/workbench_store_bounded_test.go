package nebula

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

func TestAssetProjectionQueriesAreTenantAssetAndRelationBounded(t *testing.T) {
	if !strings.Contains(assetProjectionNodeQuery, "FETCH PROP ON entity %s") {
		t.Fatalf("node query is not a direct deterministic-VID fetch: %s", assetProjectionNodeQuery)
	}
	if !strings.Contains(assetProjectionEdgeQuery, "GO FROM %s OVER relation BIDIRECT") || !strings.Contains(assetProjectionEdgeQuery, "relation.tenant_id == $tenant_id") {
		t.Fatalf("edge query is not a tenant-filtered adjacency traversal: %s", assetProjectionEdgeQuery)
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

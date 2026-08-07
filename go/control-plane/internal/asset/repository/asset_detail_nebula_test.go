package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	graphQuery "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/query"
)

type fakeAssetNebulaStore struct {
	node      *graphQuery.WorkbenchNode
	edges     []*graphQuery.WorkbenchEdge
	truncated bool
	err       error
	tenantID  string
	assetID   string
	limit     int
}

func (s *fakeAssetNebulaStore) LoadAssetProjection(_ context.Context, tenantID, assetID string, limit int) (*graphQuery.WorkbenchNode, []*graphQuery.WorkbenchEdge, bool, error) {
	s.tenantID, s.assetID, s.limit = tenantID, assetID, limit
	return s.node, s.edges, s.truncated, s.err
}

func TestAssetDetailNebulaReaderReconcilesStableAssetRevision(t *testing.T) {
	asOf := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	store := &fakeAssetNebulaStore{
		node: &graphQuery.WorkbenchNode{
			EntityID: "asset-1", EntityType: "asset", Label: "server-1", Detail: "192.0.2.8",
			Metadata: map[string]interface{}{"revision": float64(7)}, UpdatedAt: asOf.Add(-time.Minute).UnixMilli(),
		},
		edges: []*graphQuery.WorkbenchEdge{{
			RelationID: "relation-1", SourceID: "asset-1", TargetID: "account-1", RelationType: "owned_by",
			Attributes: map[string]interface{}{"source": "iam"}, ObservedAt: asOf.Add(-time.Hour).UnixMilli(),
		}},
	}
	reader, err := NewAssetDetailNebulaReader(store, 25)
	if err != nil {
		t.Fatal(err)
	}
	asset := &config.AssetRecord{AssetID: "asset-1", TenantID: "tenant-a", Revision: 7}
	projection, marks, complete, err := reader.ReadAssetGraphProjection(context.Background(), "tenant-a", asset, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if !complete || projection.Stale || projection.Truncated || projection.ProjectedRevision != 7 || len(projection.Relations) != 1 {
		t.Fatalf("projection=%+v complete=%v", projection, complete)
	}
	if store.tenantID != "tenant-a" || store.assetID != "asset-1" || store.limit != 25 {
		t.Fatalf("store bounds=%q/%q/%d", store.tenantID, store.assetID, store.limit)
	}
	if marks["nebulagraph.entity.asset_revision"] != "7" || marks["nebulagraph.entity.updated_at"] == "" {
		t.Fatalf("marks=%v", marks)
	}
}

func TestAssetDetailNebulaReaderReturnsStaleOrTruncatedAsIncomplete(t *testing.T) {
	asOf := time.Now().UTC()
	for _, tc := range []struct {
		name      string
		revision  interface{}
		truncated bool
	}{
		{name: "stale revision", revision: "6"},
		{name: "truncated relations", revision: int64(7), truncated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeAssetNebulaStore{node: &graphQuery.WorkbenchNode{
				EntityID: "asset-1", EntityType: "asset", Metadata: map[string]interface{}{"revision": tc.revision}, UpdatedAt: asOf.Add(-time.Minute).UnixMilli(),
			}, truncated: tc.truncated}
			reader, err := NewAssetDetailNebulaReader(store, 10)
			if err != nil {
				t.Fatal(err)
			}
			projection, _, complete, err := reader.ReadAssetGraphProjection(context.Background(), "tenant-a", &config.AssetRecord{AssetID: "asset-1", TenantID: "tenant-a", Revision: 7}, asOf)
			if err != nil {
				t.Fatal(err)
			}
			if complete || (projection.Stale != (tc.revision == "6")) || projection.Truncated != tc.truncated {
				t.Fatalf("projection=%+v complete=%v", projection, complete)
			}
		})
	}
}

func TestAssetDetailNebulaReaderRejectsUnrelatedOrFutureProjection(t *testing.T) {
	asOf := time.Now().UTC()
	baseNode := &graphQuery.WorkbenchNode{EntityID: "asset-1", EntityType: "asset", Metadata: map[string]interface{}{"revision": 7}, UpdatedAt: asOf.Add(-time.Minute).UnixMilli()}
	for _, store := range []*fakeAssetNebulaStore{
		{node: baseNode, edges: []*graphQuery.WorkbenchEdge{{SourceID: "other-1", TargetID: "other-2"}}},
		{node: &graphQuery.WorkbenchNode{EntityID: "asset-1", EntityType: "asset", Metadata: map[string]interface{}{"revision": 7}, UpdatedAt: asOf.Add(time.Minute).UnixMilli()}},
		{err: errors.New("Nebula timeout")},
	} {
		reader, err := NewAssetDetailNebulaReader(store, 10)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := reader.ReadAssetGraphProjection(context.Background(), "tenant-a", &config.AssetRecord{AssetID: "asset-1", TenantID: "tenant-a", Revision: 7}, asOf); err == nil {
			t.Fatal("projection should be rejected")
		}
	}
}

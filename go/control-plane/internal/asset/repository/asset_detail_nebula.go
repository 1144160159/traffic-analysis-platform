package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	graphQuery "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/query"
)

type AssetNebulaProjectionStore interface {
	LoadAssetProjection(context.Context, string, string, int) (*graphQuery.WorkbenchNode, []*graphQuery.WorkbenchEdge, bool, error)
}

type AssetDetailNebulaReader struct {
	store         AssetNebulaProjectionStore
	relationLimit int
}

func NewAssetDetailNebulaReader(store AssetNebulaProjectionStore, relationLimit int) (*AssetDetailNebulaReader, error) {
	if store == nil {
		return nil, fmt.Errorf("asset detail NebulaGraph store is required")
	}
	if relationLimit <= 0 || relationLimit > 500 {
		return nil, fmt.Errorf("asset detail NebulaGraph relation limit must be within 1..500")
	}
	return &AssetDetailNebulaReader{store: store, relationLimit: relationLimit}, nil
}

func (r *AssetDetailNebulaReader) ReadAssetGraphProjection(
	ctx context.Context,
	tenantID string,
	asset *config.AssetRecord,
	asOf time.Time,
) (*config.AssetGraphProjection, map[string]string, bool, error) {
	if tenantID == "" || asset == nil || asset.TenantID != tenantID || asset.AssetID == "" {
		return nil, nil, false, fmt.Errorf("tenant-scoped asset is required")
	}
	if asOf.IsZero() {
		return nil, nil, false, fmt.Errorf("PostgreSQL snapshot as_of is required")
	}
	node, edges, truncated, err := r.store.LoadAssetProjection(ctx, tenantID, asset.AssetID, r.relationLimit)
	if err != nil {
		return nil, nil, false, err
	}
	if node == nil || node.EntityID != asset.AssetID || node.EntityType != "asset" {
		return nil, nil, false, fmt.Errorf("NebulaGraph projection identity does not match asset")
	}
	projectedRevision, err := projectionRevision(node.Metadata)
	if err != nil {
		return nil, nil, false, err
	}
	if projectedRevision <= 0 {
		return nil, nil, false, fmt.Errorf("NebulaGraph projection revision must be positive")
	}
	updatedAt := time.UnixMilli(node.UpdatedAt).UTC()
	if node.UpdatedAt <= 0 {
		return nil, nil, false, fmt.Errorf("NebulaGraph projection has no updated_at watermark")
	}
	if node.UpdatedAt > asOf.UTC().UnixMilli() {
		return nil, nil, false, fmt.Errorf("NebulaGraph projection watermark is newer than PostgreSQL snapshot")
	}
	projection := &config.AssetGraphProjection{
		AssetID: asset.AssetID, Source: "nebulagraph.entity_relation_projection_v1",
		Label: node.Label, Detail: node.Detail, RiskScore: node.RiskScore, RiskLevel: node.RiskLevel,
		Icon: node.Icon, Metadata: node.Metadata, ProjectedRevision: projectedRevision,
		PostgresRevision: asset.Revision, UpdatedAt: updatedAt, Truncated: truncated,
		Relations: make([]config.AssetGraphProjectionRelation, 0, len(edges)),
	}
	for _, edge := range edges {
		if edge == nil || (edge.SourceID != asset.AssetID && edge.TargetID != asset.AssetID) {
			return nil, nil, false, fmt.Errorf("NebulaGraph returned an unrelated asset relation")
		}
		observedAt := time.Time{}
		if edge.ObservedAt > 0 {
			observedAt = time.UnixMilli(edge.ObservedAt).UTC()
		}
		projection.Relations = append(projection.Relations, config.AssetGraphProjectionRelation{
			RelationID: edge.RelationID, SourceID: edge.SourceID, TargetID: edge.TargetID,
			RelationType: edge.RelationType, RiskLevel: edge.RiskLevel, EvidenceID: edge.EvidenceID,
			Attributes: edge.Attributes, Weight: edge.Weight, ObservedAt: observedAt,
		})
	}
	projection.Stale = projectedRevision != asset.Revision
	watermarks := map[string]string{
		"nebulagraph.entity.asset_revision": strconv.FormatInt(projectedRevision, 10),
		"nebulagraph.entity.updated_at":     updatedAt.Format(time.RFC3339Nano),
	}
	complete := !projection.Stale && !projection.Truncated
	return projection, watermarks, complete, nil
}

func projectionRevision(metadata map[string]interface{}) (int64, error) {
	value, ok := metadata["revision"]
	if !ok {
		return 0, fmt.Errorf("NebulaGraph projection has no revision watermark")
	}
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	case uint64:
		if typed > math.MaxInt64 {
			return 0, fmt.Errorf("NebulaGraph projection revision overflows int64")
		}
		return int64(typed), nil
	case float64:
		if typed < 1 || typed > math.MaxInt64 || typed != math.Trunc(typed) {
			return 0, fmt.Errorf("NebulaGraph projection revision is not an integer")
		}
		return int64(typed), nil
	case json.Number:
		return typed.Int64()
	case string:
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported NebulaGraph projection revision type %T", value)
	}
}

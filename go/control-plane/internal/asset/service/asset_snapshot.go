package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

func (s *AssetService) GetAssetDetailSnapshot(
	ctx context.Context,
	tenantID, assetID string,
	historyLimit int,
) (*config.AssetDetailSnapshot, error) {
	if tenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if assetID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "asset_id is required")
	}
	read, err := s.repo.LoadAssetSnapshot(ctx, tenantID, assetID, historyLimit, 100)
	if err != nil {
		return nil, fmt.Errorf("load asset detail snapshot: %w", err)
	}
	details, err := assetDetailsFromRecord(read.Asset)
	if err != nil {
		return nil, err
	}
	topology, err := assetTopologyFromRecord(read.Asset, read.TopologyLinks)
	if err != nil {
		return nil, err
	}
	snapshotName := fmt.Sprintf(
		"%s\x00%s\x00%d\x00%s\x00%s",
		tenantID, assetID, read.Asset.Revision, read.SnapshotXIDs, read.AsOf.UTC().Format(time.RFC3339Nano),
	)
	missing := []string{"clickhouse_observations", "alert_context", "evidence_objects", "nebulagraph_projection"}
	watermarks := map[string]string{
		"postgresql.snapshot_xids":                    read.SnapshotXIDs,
		"postgresql.assets.revision":                  strconv.FormatInt(read.Asset.Revision, 10),
		"postgresql.asset_events.max_event_id":        strconv.Itoa(read.MaxEventID),
		"postgresql.asset_topology_links.observed_at": watermarkTime(read.MaxTopologyObserved),
	}
	snapshot := &config.AssetDetailSnapshot{
		ContractVersion:   1,
		SnapshotID:        uuid.NewSHA1(uuid.NameSpaceOID, []byte(snapshotName)).String(),
		Asset:             read.Asset,
		Details:           *details,
		History:           read.History,
		Topology:          *topology,
		AvailableSections: []string{"asset", "details", "history", "postgresql_topology"},
		MissingSections:   missing,
		Partial:           true,
		SourceWatermarks:  watermarks,
		AsOf:              read.AsOf.UTC(),
	}
	s.enrichAssetDetailSnapshot(ctx, tenantID, snapshot)
	return snapshot, nil
}

func (s *AssetService) enrichAssetDetailSnapshot(ctx context.Context, tenantID string, snapshot *config.AssetDetailSnapshot) {
	if snapshot == nil || snapshot.Asset == nil {
		return
	}
	if s.observationReader != nil {
		observations, watermarks, err := s.observationReader.ReadAssetObservations(ctx, tenantID, snapshot.Asset, snapshot.AsOf)
		if err != nil {
			s.logger.Warn("asset detail ClickHouse observations unavailable", zap.String("asset_id", snapshot.Asset.AssetID), zap.Error(err))
		} else if observations != nil {
			snapshot.Observations = observations
			snapshot.AvailableSections = append(snapshot.AvailableSections, "clickhouse_observations")
			snapshot.MissingSections = removeSection(snapshot.MissingSections, "clickhouse_observations")
			mergeWatermarks(snapshot.SourceWatermarks, watermarks)
		}
	}
	if s.alertContextReader != nil {
		alerts, watermarks, err := s.alertContextReader.ReadAssetAlertContext(ctx, tenantID, snapshot.Asset, snapshot.AsOf)
		if err != nil {
			s.logger.Warn("asset detail ClickHouse alert context unavailable", zap.String("asset_id", snapshot.Asset.AssetID), zap.Error(err))
		} else if alerts != nil {
			snapshot.AlertContext = alerts
			snapshot.AvailableSections = append(snapshot.AvailableSections, "alert_context")
			snapshot.MissingSections = removeSection(snapshot.MissingSections, "alert_context")
			mergeWatermarks(snapshot.SourceWatermarks, watermarks)
		}
	}
	if s.evidenceObjectReader != nil {
		evidenceObjects, watermarks, complete, err := s.evidenceObjectReader.ReadAssetEvidenceObjects(ctx, tenantID, snapshot.Asset, snapshot.AsOf, snapshot.AlertContext)
		if err != nil {
			s.logger.Warn("asset detail evidence objects unavailable", zap.String("asset_id", snapshot.Asset.AssetID), zap.Error(err))
		} else if evidenceObjects != nil {
			snapshot.EvidenceObjects = evidenceObjects
			mergeWatermarks(snapshot.SourceWatermarks, watermarks)
			if complete {
				snapshot.AvailableSections = append(snapshot.AvailableSections, "evidence_objects")
				snapshot.MissingSections = removeSection(snapshot.MissingSections, "evidence_objects")
			}
		}
	}
	if s.graphProjectionReader != nil {
		projection, watermarks, complete, err := s.graphProjectionReader.ReadAssetGraphProjection(ctx, tenantID, snapshot.Asset, snapshot.AsOf)
		if err != nil {
			s.logger.Warn("asset detail NebulaGraph projection unavailable", zap.String("asset_id", snapshot.Asset.AssetID), zap.Error(err))
		} else if projection != nil {
			snapshot.GraphProjection = projection
			mergeWatermarks(snapshot.SourceWatermarks, watermarks)
			if complete {
				snapshot.AvailableSections = append(snapshot.AvailableSections, "nebulagraph_projection")
				snapshot.MissingSections = removeSection(snapshot.MissingSections, "nebulagraph_projection")
			}
		}
	}
	snapshot.Partial = len(snapshot.MissingSections) > 0
}

func removeSection(sections []string, target string) []string {
	filtered := sections[:0]
	for _, section := range sections {
		if section != target {
			filtered = append(filtered, section)
		}
	}
	return filtered
}

func mergeWatermarks(target, source map[string]string) {
	for key, value := range source {
		if value != "" {
			target[key] = value
		}
	}
}

func assetDetailsFromRecord(asset *config.AssetRecord) (*config.AssetDetails, error) {
	details := &config.AssetDetails{
		AssetID:      asset.AssetID,
		DataContract: "canonical-asset-detail-v1",
		Ownership: config.AssetOwnership{
			Campus: asset.Campus, Department: asset.Department, Owner: asset.Owner,
		},
		ObservedAt: asset.LastSeen,
	}
	if len(asset.Metadata) == 0 {
		return details, nil
	}
	encoded, err := json.Marshal(asset.Metadata)
	if err != nil {
		return nil, fmt.Errorf("encode asset detail metadata: %w", err)
	}
	var stored struct {
		DataContract      string                         `json:"data_contract"`
		NetworkInterfaces []config.AssetNetworkInterface `json:"network_interfaces"`
		OpenServices      []config.AssetOpenService      `json:"open_services"`
		Ownership         config.AssetOwnership          `json:"ownership"`
	}
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return nil, fmt.Errorf("decode asset detail metadata: %w", err)
	}
	if stored.DataContract != "" {
		details.DataContract = stored.DataContract
	}
	details.NetworkInterfaces = stored.NetworkInterfaces
	details.OpenServices = stored.OpenServices
	if stored.Ownership.Campus != "" {
		details.Ownership.Campus = stored.Ownership.Campus
	}
	if stored.Ownership.Department != "" {
		details.Ownership.Department = stored.Ownership.Department
	}
	if stored.Ownership.Owner != "" {
		details.Ownership.Owner = stored.Ownership.Owner
	}
	details.Ownership.BusinessSystems = stored.Ownership.BusinessSystems
	details.Ownership.AssetGroups = stored.Ownership.AssetGroups
	details.Ownership.DataDomains = stored.Ownership.DataDomains
	details.Ownership.Responsibilities = stored.Ownership.Responsibilities
	details.Ownership.PendingFields = stored.Ownership.PendingFields
	return details, nil
}

func assetTopologyFromRecord(asset *config.AssetRecord, links []*config.TopologyLink) (*config.AssetTopologyGraph, error) {
	graph := &config.AssetTopologyGraph{
		AssetID: asset.AssetID, Source: "empty", FixtureMode: asset.Source == "acceptance-fixture",
		ObservedAt: asset.LastSeen,
		Nodes: []config.AssetTopologyNode{{
			ID: asset.AssetID, Label: firstNonEmpty(asset.Hostname, asset.DisplayCode, asset.AssetID), Kind: asset.AssetType, Status: asset.Status,
		}},
		Edges: []config.AssetTopologyEdge{},
	}
	if len(links) > 0 {
		graph.Source = "discovery_neighbors"
		nodeIDs := map[string]struct{}{asset.AssetID: {}}
		for index, link := range links {
			sourceID := firstNonEmpty(link.SourceAssetID, prefixedIdentity("ip", link.SourceIP), prefixedIdentity("mac", link.SourceMAC), fmt.Sprintf("link-%s-source", link.LinkID))
			targetID := firstNonEmpty(link.NeighborAssetID, prefixedIdentity("ip", link.NeighborIP), prefixedIdentity("mac", link.NeighborMAC), fmt.Sprintf("link-%s-neighbor", link.LinkID))
			for _, node := range []config.AssetTopologyNode{
				{ID: sourceID, Label: firstNonEmpty(link.SourceIP, link.SourceMAC, shortIdentity(sourceID)), Kind: "asset", Status: "observed"},
				{ID: targetID, Label: firstNonEmpty(link.NeighborIP, link.NeighborMAC, shortIdentity(targetID)), Kind: "asset", Status: "observed"},
			} {
				if _, exists := nodeIDs[node.ID]; exists {
					continue
				}
				nodeIDs[node.ID] = struct{}{}
				graph.Nodes = append(graph.Nodes, node)
			}
			health := "healthy"
			if link.Confidence > 0 && link.Confidence < 60 {
				health = "warning"
			}
			graph.Edges = append(graph.Edges, config.AssetTopologyEdge{
				ID: firstNonEmpty(link.LinkID, fmt.Sprintf("discovery-%d", index)), Source: sourceID, Target: targetID,
				Relationship: "neighbor", Direction: "directed", Protocol: link.Protocol, Health: health,
				Confidence: link.Confidence, ObservedAt: link.ObservedAt,
			})
			if link.ObservedAt.After(graph.ObservedAt) {
				graph.ObservedAt = link.ObservedAt
			}
		}
		return graph, nil
	}
	if len(asset.Metadata) == 0 {
		return graph, nil
	}
	encoded, err := json.Marshal(asset.Metadata)
	if err != nil {
		return nil, fmt.Errorf("encode asset topology metadata: %w", err)
	}
	var stored struct {
		TopologyGraph struct {
			Nodes []config.AssetTopologyNode `json:"nodes"`
			Edges []config.AssetTopologyEdge `json:"edges"`
		} `json:"topology_graph"`
		TopologyNodes []string `json:"topology_nodes"`
	}
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return nil, fmt.Errorf("decode asset topology metadata: %w", err)
	}
	if len(stored.TopologyGraph.Nodes) > 0 || len(stored.TopologyGraph.Edges) > 0 {
		graph.Source = "asset_metadata_graph"
		nodeIDs := map[string]struct{}{asset.AssetID: {}}
		for index, node := range stored.TopologyGraph.Nodes {
			if node.ID == "" || node.ID == "self" {
				node.ID = fmt.Sprintf("metadata-node-%d", index)
			}
			if node.Label == "" {
				node.Label = shortIdentity(node.ID)
			}
			if _, exists := nodeIDs[node.ID]; exists {
				continue
			}
			nodeIDs[node.ID] = struct{}{}
			graph.Nodes = append(graph.Nodes, node)
		}
		for index, edge := range stored.TopologyGraph.Edges {
			if edge.Source == "self" {
				edge.Source = asset.AssetID
			}
			if edge.Target == "self" {
				edge.Target = asset.AssetID
			}
			if edge.ID == "" {
				edge.ID = fmt.Sprintf("metadata-edge-%d", index)
			}
			if edge.Relationship == "" {
				edge.Relationship = "related_to"
			}
			if _, sourceOK := nodeIDs[edge.Source]; !sourceOK {
				continue
			}
			if _, targetOK := nodeIDs[edge.Target]; !targetOK {
				continue
			}
			graph.Edges = append(graph.Edges, edge)
		}
		return graph, nil
	}
	if len(stored.TopologyNodes) > 0 {
		graph.Source = "legacy_asset_metadata"
		for index, label := range stored.TopologyNodes {
			nodeID := fmt.Sprintf("metadata-node-%d", index)
			graph.Nodes = append(graph.Nodes, config.AssetTopologyNode{ID: nodeID, Label: label, Kind: "related", Status: "unknown"})
			graph.Edges = append(graph.Edges, config.AssetTopologyEdge{
				ID: fmt.Sprintf("metadata-edge-%d", index), Source: asset.AssetID, Target: nodeID,
				Relationship: "related_to", Direction: "directed", Health: "unknown",
			})
		}
	}
	return graph, nil
}

func watermarkTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

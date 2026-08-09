package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

type AssetSnapshotRead struct {
	Asset               *config.AssetRecord
	History             []*config.AssetEvent
	TopologyLinks       []*config.TopologyLink
	AsOf                time.Time
	SnapshotXIDs        string
	MaxEventID          int
	MaxTopologyObserved time.Time
}

// LoadAssetSnapshot reads the authoritative asset, history and discovery
// topology in one read-only repeatable-read transaction. It intentionally does
// not invent CH, alert, evidence-object or Nebula sections.
func (r *AssetRepository) LoadAssetSnapshot(
	ctx context.Context,
	tenantID, assetID string,
	historyLimit, topologyLimit int,
) (*AssetSnapshotRead, error) {
	if historyLimit <= 0 || historyLimit > 100 {
		historyLimit = 50
	}
	if topologyLimit <= 0 || topologyLimit > 500 {
		topologyLimit = 100
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin asset snapshot: %w", err)
	}
	defer tx.Rollback()
	result := &AssetSnapshotRead{}
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp(),pg_current_snapshot()::text`).Scan(&result.AsOf, &result.SnapshotXIDs); err != nil {
		return nil, fmt.Errorf("read asset snapshot watermark: %w", err)
	}
	result.Asset, err = scanAsset(tx.QueryRowContext(ctx, `
		SELECT asset_id,revision,display_code,tenant_id,asset_type,status,ip_address,mac_address,
		       hostname,vendor,os_type,source,vlan_id,switch_port,department,campus,owner,
		       criticality,tags,metadata,first_seen,last_seen
		FROM assets WHERE tenant_id=$1 AND asset_id=$2`, tenantID, assetID))
	if err != nil {
		return nil, err
	}

	historyRows, err := tx.QueryContext(ctx, `
		SELECT event_id,asset_id,tenant_id,event_type,old_value,new_value,created_at
		FROM asset_events
		WHERE tenant_id=$1 AND asset_id=$2
		ORDER BY created_at DESC,event_id DESC LIMIT $3`, tenantID, assetID, historyLimit)
	if err != nil {
		return nil, fmt.Errorf("query asset snapshot history: %w", err)
	}
	for historyRows.Next() {
		var event config.AssetEvent
		var oldValue, newValue sql.NullString
		if err := historyRows.Scan(&event.EventID, &event.AssetID, &event.TenantID, &event.EventType, &oldValue, &newValue, &event.CreatedAt); err != nil {
			historyRows.Close()
			return nil, fmt.Errorf("scan asset snapshot history: %w", err)
		}
		event.OldValue, event.NewValue = oldValue.String, newValue.String
		result.History = append(result.History, &event)
		if event.EventID > result.MaxEventID {
			result.MaxEventID = event.EventID
		}
	}
	if err := historyRows.Close(); err != nil {
		return nil, err
	}
	if err := historyRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset snapshot history: %w", err)
	}

	topologyRows, err := tx.QueryContext(ctx, `
		SELECT link_id,tenant_id,run_id,source_asset_id,source_mac,source_ip,source_interface,
		       neighbor_asset_id,neighbor_mac,neighbor_ip,neighbor_interface,protocol,confidence,
		       observed_at,created_at
		FROM asset_topology_links
		WHERE tenant_id=$1 AND (source_asset_id=$2 OR neighbor_asset_id=$2)
		ORDER BY observed_at DESC,link_id DESC LIMIT $3`, tenantID, assetID, topologyLimit)
	if err != nil {
		return nil, fmt.Errorf("query asset snapshot topology: %w", err)
	}
	for topologyRows.Next() {
		var link config.TopologyLink
		var runID, sourceAssetID, sourceMAC, sourceIP, sourceInterface sql.NullString
		var neighborAssetID, neighborMAC, neighborIP, neighborInterface sql.NullString
		if err := topologyRows.Scan(
			&link.LinkID, &link.TenantID, &runID, &sourceAssetID, &sourceMAC, &sourceIP, &sourceInterface,
			&neighborAssetID, &neighborMAC, &neighborIP, &neighborInterface, &link.Protocol,
			&link.Confidence, &link.ObservedAt, &link.CreatedAt,
		); err != nil {
			topologyRows.Close()
			return nil, fmt.Errorf("scan asset snapshot topology: %w", err)
		}
		link.RunID, link.SourceAssetID, link.SourceMAC = runID.String, sourceAssetID.String, sourceMAC.String
		link.SourceIP, link.SourceInterface = sourceIP.String, sourceInterface.String
		link.NeighborAssetID, link.NeighborMAC = neighborAssetID.String, neighborMAC.String
		link.NeighborIP, link.NeighborInterface = neighborIP.String, neighborInterface.String
		result.TopologyLinks = append(result.TopologyLinks, &link)
		if link.ObservedAt.After(result.MaxTopologyObserved) {
			result.MaxTopologyObserved = link.ObservedAt
		}
	}
	if err := topologyRows.Close(); err != nil {
		return nil, err
	}
	if err := topologyRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset snapshot topology: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit asset snapshot: %w", err)
	}
	return result, nil
}

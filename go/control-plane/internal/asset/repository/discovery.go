package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/google/uuid"
)

func (r *AssetRepository) RegisterDiscoveryCredential(ctx context.Context, credential *config.DiscoveryCredential) error {
	if credential == nil {
		return fmt.Errorf("discovery credential required")
	}
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO asset_discovery_credentials (
			credential_id, tenant_id, name, protocol, endpoint, secret_ref, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
		ON CONFLICT (credential_id) DO UPDATE SET
			name=EXCLUDED.name,
			protocol=EXCLUDED.protocol,
			endpoint=EXCLUDED.endpoint,
			secret_ref=EXCLUDED.secret_ref,
			updated_at=EXCLUDED.updated_at`,
		credential.CredentialID, credential.TenantID, credential.Name, credential.Protocol,
		credential.Endpoint, credential.SecretRef, credential.CreatedBy, now)
	return err
}

func (r *AssetRepository) ListDiscoveryCredentials(ctx context.Context, tenantID string, limit int) ([]*config.DiscoveryCredential, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT credential_id, tenant_id, name, protocol, endpoint, secret_ref, created_by, created_at, updated_at
		FROM asset_discovery_credentials
		WHERE tenant_id=$1
		ORDER BY updated_at DESC
		LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*config.DiscoveryCredential
	for rows.Next() {
		var item config.DiscoveryCredential
		var endpoint, createdBy sql.NullString
		if err := rows.Scan(&item.CredentialID, &item.TenantID, &item.Name, &item.Protocol, &endpoint, &item.SecretRef, &createdBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Endpoint = endpoint.String
		item.CreatedBy = createdBy.String
		result = append(result, &item)
	}
	return result, rows.Err()
}

func (r *AssetRepository) GetDiscoveryCredential(ctx context.Context, tenantID, credentialID string) (*config.DiscoveryCredential, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT credential_id, tenant_id, name, protocol, endpoint, secret_ref, created_by, created_at, updated_at
		FROM asset_discovery_credentials
		WHERE tenant_id=$1 AND credential_id=$2`, tenantID, credentialID)
	var item config.DiscoveryCredential
	var endpoint, createdBy sql.NullString
	if err := row.Scan(&item.CredentialID, &item.TenantID, &item.Name, &item.Protocol, &endpoint, &item.SecretRef, &createdBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	item.Endpoint = endpoint.String
	item.CreatedBy = createdBy.String
	return &item, nil
}

func (r *AssetRepository) CreateDiscoveryRun(ctx context.Context, run *config.DiscoveryRun) error {
	if run == nil {
		return fmt.Errorf("discovery run required")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO asset_discovery_runs (
			run_id, tenant_id, mode, target_cidr, credential_id, status, requested_by, started_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		run.RunID, run.TenantID, run.Mode, run.TargetCIDR, nullString(run.CredentialID),
		run.Status, run.RequestedBy, run.StartedAt)
	return err
}

func (r *AssetRepository) CompleteDiscoveryRun(ctx context.Context, runID, status, errorMessage string, assets, links int, completedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE asset_discovery_runs
		SET status=$2, discovered_assets=$3, discovered_links=$4, error_message=$5, completed_at=$6
		WHERE run_id=$1`,
		runID, status, assets, links, nullString(errorMessage), completedAt)
	return err
}

func (r *AssetRepository) ListDiscoveryRuns(ctx context.Context, tenantID string, limit int) ([]*config.DiscoveryRun, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT run_id, tenant_id, mode, target_cidr, credential_id, status, requested_by,
		       discovered_assets, discovered_links, error_message, started_at, completed_at
		FROM asset_discovery_runs
		WHERE tenant_id=$1
		ORDER BY started_at DESC
		LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*config.DiscoveryRun
	for rows.Next() {
		var run config.DiscoveryRun
		var targetCIDR, credentialID, requestedBy, errorMessage sql.NullString
		var completedAt sql.NullTime
		if err := rows.Scan(&run.RunID, &run.TenantID, &run.Mode, &targetCIDR, &credentialID, &run.Status, &requestedBy, &run.DiscoveredAssets, &run.DiscoveredLinks, &errorMessage, &run.StartedAt, &completedAt); err != nil {
			return nil, err
		}
		run.TargetCIDR = targetCIDR.String
		run.CredentialID = credentialID.String
		run.RequestedBy = requestedBy.String
		run.ErrorMessage = errorMessage.String
		if completedAt.Valid {
			run.CompletedAt = completedAt.Time
		}
		result = append(result, &run)
	}
	return result, rows.Err()
}

func (r *AssetRepository) UpsertTopologyLink(ctx context.Context, link *config.TopologyLink) error {
	if link == nil {
		return fmt.Errorf("topology link required")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO asset_topology_links (
			link_id, tenant_id, run_id, source_asset_id, source_mac, source_ip, source_interface,
			neighbor_asset_id, neighbor_mac, neighbor_ip, neighbor_interface, protocol, confidence, observed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (tenant_id, source_mac, neighbor_mac, protocol, source_interface, neighbor_interface)
		DO UPDATE SET
			run_id=EXCLUDED.run_id,
			source_asset_id=EXCLUDED.source_asset_id,
			source_ip=EXCLUDED.source_ip,
			neighbor_asset_id=EXCLUDED.neighbor_asset_id,
			neighbor_ip=EXCLUDED.neighbor_ip,
			confidence=EXCLUDED.confidence,
			observed_at=EXCLUDED.observed_at`,
		link.LinkID, link.TenantID, nullString(link.RunID), nullString(link.SourceAssetID),
		nullString(link.SourceMAC), nullString(link.SourceIP), nullString(link.SourceInterface),
		nullString(link.NeighborAssetID), nullString(link.NeighborMAC), nullString(link.NeighborIP),
		nullString(link.NeighborInterface), link.Protocol, link.Confidence, link.ObservedAt)
	return err
}

func (r *AssetRepository) ListTopologyLinks(ctx context.Context, tenantID, assetID string, limit int) ([]*config.TopologyLink, error) {
	query := `
		SELECT link_id, tenant_id, run_id, source_asset_id, source_mac, source_ip, source_interface,
		       neighbor_asset_id, neighbor_mac, neighbor_ip, neighbor_interface, protocol, confidence, observed_at, created_at
		FROM asset_topology_links
		WHERE tenant_id=$1`
	args := []any{tenantID}
	if assetID != "" {
		query += ` AND (source_asset_id=$2 OR neighbor_asset_id=$2)`
		args = append(args, assetID)
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY observed_at DESC LIMIT $%d", len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*config.TopologyLink
	for rows.Next() {
		var link config.TopologyLink
		var runID, sourceAssetID, sourceMAC, sourceIP, sourceInterface sql.NullString
		var neighborAssetID, neighborMAC, neighborIP, neighborInterface sql.NullString
		if err := rows.Scan(&link.LinkID, &link.TenantID, &runID, &sourceAssetID, &sourceMAC, &sourceIP, &sourceInterface, &neighborAssetID, &neighborMAC, &neighborIP, &neighborInterface, &link.Protocol, &link.Confidence, &link.ObservedAt, &link.CreatedAt); err != nil {
			return nil, err
		}
		link.RunID = runID.String
		link.SourceAssetID = sourceAssetID.String
		link.SourceMAC = sourceMAC.String
		link.SourceIP = sourceIP.String
		link.SourceInterface = sourceInterface.String
		link.NeighborAssetID = neighborAssetID.String
		link.NeighborMAC = neighborMAC.String
		link.NeighborIP = neighborIP.String
		link.NeighborInterface = neighborInterface.String
		result = append(result, &link)
	}
	return result, rows.Err()
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (r *AssetRepository) InsertAuditLog(ctx context.Context, tenantID, userID, action, objectType, objectID string, detail map[string]interface{}, ipAddr, userAgent string) error {
	if r == nil || r.db == nil {
		return nil
	}
	detailJSON, _ := json.Marshal(detail)
	if r.pgColumnExists(ctx, "audit_logs", "event_id") {
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7::jsonb, $8, $9)`,
			uuid.New().String(), tenantID, userID, action, objectType, objectID, string(detailJSON), ipAddr, userAgent)
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6::jsonb, $7, $8)`,
		tenantID, userID, action, objectType, objectID, string(detailJSON), ipAddr, userAgent)
	return err
}

func (r *AssetRepository) pgColumnExists(ctx context.Context, tableName, columnName string) bool {
	if r == nil || r.db == nil {
		return false
	}
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = $1
			  AND column_name = $2
		)`, tableName, columnName).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

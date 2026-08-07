package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"go.uber.org/zap"
)

type AssetRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewAssetRepository(db *sql.DB, logger *zap.Logger) (*AssetRepository, error) {
	if db == nil {
		return nil, errors.New(errors.ErrCodeInternal, "db connection required")
	}
	return &AssetRepository{db: db, logger: logger}, nil
}

func ensureAssetDefaults(rec *config.AssetRecord) {
	if rec.AssetType == "" {
		rec.AssetType = "unknown"
	}
	if rec.Status == "" {
		rec.Status = "active"
	}
	if rec.DisplayCode == "" {
		compactID := strings.ToUpper(strings.ReplaceAll(rec.AssetID, "-", ""))
		if len(compactID) > 8 {
			compactID = compactID[:8]
		}
		prefix := map[string]string{
			"endpoint":        "END",
			"server":          "SRV",
			"network-device":  "NET",
			"business-system": "BIZ",
			"unknown":         "UNK",
		}[rec.AssetType]
		if prefix == "" {
			prefix = "AST"
		}
		rec.DisplayCode = prefix + "-" + compactID
	}
}

func mergeAssetGovernance(rec, existing *config.AssetRecord) {
	if rec.DisplayCode == "" {
		rec.DisplayCode = existing.DisplayCode
	}
	if rec.AssetType == "" {
		rec.AssetType = existing.AssetType
	}
	if rec.Status == "" {
		rec.Status = existing.Status
	}
	if rec.Department == "" {
		rec.Department = existing.Department
	}
	if rec.Campus == "" {
		rec.Campus = existing.Campus
	}
	if rec.Owner == "" {
		rec.Owner = existing.Owner
	}
	if rec.Criticality == 0 && existing.Criticality != 0 {
		rec.Criticality = existing.Criticality
	}
	if rec.Tags == nil {
		rec.Tags = existing.Tags
	}
	if rec.Metadata == nil {
		rec.Metadata = existing.Metadata
	}
	ensureAssetDefaults(rec)
}

func (r *AssetRepository) FindByMAC(ctx context.Context, tenantID, mac string) (*config.AssetRecord, error) {
	return r.findByMAC(ctx, tenantID, mac)
}

func (r *AssetRepository) findByMAC(ctx context.Context, tenantID, mac string) (*config.AssetRecord, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT asset_id, revision, display_code, tenant_id, asset_type, status, ip_address, mac_address,
		 hostname, vendor, os_type, source, vlan_id, switch_port, department, campus, owner,
		 criticality, tags, metadata, first_seen, last_seen
		 FROM assets WHERE tenant_id=$1 AND mac_address=$2`, tenantID, mac)
	a, err := scanAsset(row)
	if err == sql.ErrNoRows {
		return nil, errors.New(errors.ErrCodeTenantNotFound, "asset not found: "+mac)
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *AssetRepository) FindByID(ctx context.Context, tenantID, assetID string) (*config.AssetRecord, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT asset_id, revision, display_code, tenant_id, asset_type, status, ip_address, mac_address,
		 hostname, vendor, os_type, source, vlan_id, switch_port, department, campus, owner,
		 criticality, tags, metadata, first_seen, last_seen
		 FROM assets WHERE tenant_id=$1 AND asset_id=$2`, tenantID, assetID)
	a, err := scanAsset(row)
	if err == sql.ErrNoRows {
		return nil, errors.New(errors.ErrCodeTenantNotFound, "asset not found: "+assetID)
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *AssetRepository) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*config.AssetRecord, int, error) {
	return r.ListByTenantAndType(ctx, tenantID, "", limit, offset)
}

func (r *AssetRepository) ListByTenantAndType(ctx context.Context, tenantID, assetType string, limit, offset int) ([]*config.AssetRecord, int, error) {
	return r.ListByTenantFiltered(ctx, tenantID, config.AssetListFilter{AssetType: assetType}, limit, offset)
}

func assetListWhere(tenantID string, filter config.AssetListFilter) ([]string, []any) {
	conditions := []string{"tenant_id=$1"}
	args := []any{tenantID}
	addExact := func(column, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf("%s=$%d", column, len(args)))
	}
	addExact("asset_type", filter.AssetType)
	addExact("status", filter.Status)
	addExact("department", filter.Department)
	addExact("campus", filter.Campus)
	if filter.IPPrefix != "" {
		args = append(args, filter.IPPrefix+"%")
		conditions = append(conditions, fmt.Sprintf("ip_address LIKE $%d", len(args)))
	}
	if filter.Vendor != "" {
		args = append(args, filter.Vendor)
		conditions = append(conditions, fmt.Sprintf("vendor ILIKE $%d", len(args)))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		placeholder := len(args)
		conditions = append(conditions, fmt.Sprintf("(display_code ILIKE $%d OR hostname ILIKE $%d OR ip_address ILIKE $%d OR mac_address ILIKE $%d)", placeholder, placeholder, placeholder, placeholder))
	}
	return conditions, args
}

func (r *AssetRepository) ListByTenantFiltered(ctx context.Context, tenantID string, filter config.AssetListFilter, limit, offset int) ([]*config.AssetRecord, int, error) {
	conditions, args := assetListWhere(tenantID, filter)
	where := strings.Join(conditions, " AND ")
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM assets WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT asset_id, revision, display_code, tenant_id, asset_type, status, ip_address, mac_address,
		 hostname, vendor, os_type, source, vlan_id, switch_port, department, campus, owner,
		 criticality, tags, metadata, first_seen, last_seen
		 FROM assets WHERE `+where+fmt.Sprintf(" ORDER BY last_seen DESC,asset_id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []*config.AssetRecord
	for rows.Next() {
		a, scanErr := scanAsset(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate asset list: %w", err)
	}
	return result, total, nil
}

// ListByTenantCursor returns a stable keyset page. SnapshotAt is sourced from
// PostgreSQL on the first page, and later pages bind to that same high-water
// mark so concurrent inserts cannot appear inside the traversal.
func (r *AssetRepository) ListByTenantCursor(
	ctx context.Context,
	tenantID string,
	filter config.AssetListFilter,
	limit int,
	position *config.AssetCursorPosition,
) (*config.AssetCursorPage, error) {
	snapshotAt := time.Time{}
	snapshotXIDs := ""
	total := 0
	if position != nil {
		snapshotAt = position.SnapshotAt.UTC()
		snapshotXIDs = position.SnapshotXIDs
		total = position.Total
	} else {
		if err := r.db.QueryRowContext(
			ctx,
			`SELECT clock_timestamp(),pg_current_snapshot()::text`,
		).Scan(&snapshotAt, &snapshotXIDs); err != nil {
			return nil, fmt.Errorf("read asset snapshot watermark: %w", err)
		}
		snapshotAt = snapshotAt.UTC()
	}
	if snapshotAt.IsZero() || snapshotXIDs == "" {
		return nil, fmt.Errorf("asset snapshot watermark and MVCC snapshot are required")
	}

	conditions, args := assetListWhere(tenantID, filter)
	args = append(args, snapshotAt)
	conditions = append(conditions, fmt.Sprintf("updated_at<=$%d", len(args)))
	args = append(args, snapshotXIDs)
	conditions = append(
		conditions,
		fmt.Sprintf("pg_visible_in_snapshot(xmin::text::xid8,$%d::pg_snapshot)", len(args)),
	)
	where := strings.Join(conditions, " AND ")
	if position == nil {
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM assets WHERE "+where, args...).Scan(&total); err != nil {
			return nil, fmt.Errorf("count asset cursor snapshot: %w", err)
		}
	} else {
		if position.LastSeen.IsZero() || position.LastAssetID == "" || position.Total < 0 {
			return nil, fmt.Errorf("invalid asset cursor position")
		}
		args = append(args, position.LastSeen.UTC(), position.LastAssetID)
		conditions = append(
			conditions,
			fmt.Sprintf("(last_seen,asset_id)<($%d,$%d::uuid)", len(args)-1, len(args)),
		)
		where = strings.Join(conditions, " AND ")
	}

	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx,
		`SELECT asset_id, revision, display_code, tenant_id, asset_type, status, ip_address, mac_address,
		 hostname, vendor, os_type, source, vlan_id, switch_port, department, campus, owner,
		 criticality, tags, metadata, first_seen, last_seen
		 FROM assets WHERE `+where+fmt.Sprintf(
			" ORDER BY last_seen DESC,asset_id DESC LIMIT $%d",
			len(args),
		),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("query asset cursor snapshot: %w", err)
	}
	defer rows.Close()

	assets := make([]*config.AssetRecord, 0, limit+1)
	for rows.Next() {
		asset, scanErr := scanAsset(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan asset cursor snapshot: %w", scanErr)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset cursor snapshot: %w", err)
	}
	hasMore := len(assets) > limit
	if hasMore {
		assets = assets[:limit]
	}
	page := &config.AssetCursorPage{
		Assets:       assets,
		Total:        total,
		SnapshotAt:   snapshotAt,
		SnapshotXIDs: snapshotXIDs,
		HasMore:      hasMore,
	}
	if len(assets) > 0 {
		last := assets[len(assets)-1]
		page.LastSeen = last.LastSeen.UTC()
		page.LastAssetID = last.AssetID
	}
	return page, nil
}

func (r *AssetRepository) GetStats(ctx context.Context, tenantID, assetType string) (*config.AssetStats, error) {
	return r.GetStatsFiltered(ctx, tenantID, config.AssetListFilter{AssetType: assetType})
}

// GetStatsFiltered keeps KPI aggregation on the same tenant and filter scope as the asset list.
func (r *AssetRepository) GetStatsFiltered(ctx context.Context, tenantID string, filter config.AssetListFilter) (*config.AssetStats, error) {
	conditions := []string{"tenant_id=$1"}
	args := []any{tenantID}
	addExact := func(column, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf("%s=$%d", column, len(args)))
	}
	addExact("asset_type", filter.AssetType)
	addExact("status", filter.Status)
	addExact("department", filter.Department)
	addExact("campus", filter.Campus)
	if filter.IPPrefix != "" {
		args = append(args, filter.IPPrefix+"%")
		conditions = append(conditions, fmt.Sprintf("ip_address LIKE $%d", len(args)))
	}
	if filter.Vendor != "" {
		args = append(args, filter.Vendor)
		conditions = append(conditions, fmt.Sprintf("vendor ILIKE $%d", len(args)))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		placeholder := len(args)
		conditions = append(conditions, fmt.Sprintf("(display_code ILIKE $%d OR hostname ILIKE $%d OR ip_address ILIKE $%d OR mac_address ILIKE $%d)", placeholder, placeholder, placeholder, placeholder))
	}
	where := strings.Join(conditions, " AND ")
	var stats config.AssetStats
	err := r.db.QueryRowContext(ctx, `WITH filtered_assets AS (
		SELECT * FROM assets WHERE `+where+`
	) SELECT
		COUNT(*),
		COUNT(*) FILTER (WHERE status='active'),
		COUNT(*) FILTER (WHERE status='inactive'),
		COUNT(*) FILTER (WHERE asset_type='unknown' OR status='unknown'),
		COUNT(*) FILTER (WHERE CASE WHEN (metadata->>'risk_score') ~ '^[0-9]+$' THEN (metadata->>'risk_score')::INT ELSE 0 END >=80),
		COUNT(*) FILTER (WHERE criticality >= 80),
		COUNT(*) FILTER (WHERE owner IS NULL OR owner=''),
		COALESCE(SUM(jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'open_services')='array' THEN metadata->'open_services' ELSE '[]'::jsonb END)),0),
		COALESCE((SELECT COUNT(*) FROM filtered_assets f CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(f.metadata->'open_services')='array' THEN f.metadata->'open_services' ELSE '[]'::jsonb END) service WHERE COALESCE(service->>'risk_level','') LIKE '%高%'),0),
		COALESCE(SUM(CASE WHEN COALESCE(metadata->'exposure'->>'weak_password','') ~ '^[0-9]+$' THEN (metadata->'exposure'->>'weak_password')::INT ELSE 0 END),0),
		COALESCE(SUM(jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'network_interfaces')='array' THEN metadata->'network_interfaces' ELSE '[]'::jsonb END)),0),
		COALESCE(SUM(jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'config_changes')='array' THEN metadata->'config_changes' ELSE '[]'::jsonb END)),0),
		COALESCE((SELECT SUM(CASE WHEN COALESCE(dependency->>'total','') ~ '^[0-9]+$' THEN (dependency->>'total')::INT ELSE 0 END) FROM filtered_assets f CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(f.metadata->'dependency_health')='array' THEN f.metadata->'dependency_health' ELSE '[]'::jsonb END) dependency),0),
		COALESCE(SUM(jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'key_services')='array' THEN metadata->'key_services' ELSE '[]'::jsonb END)),0),
		COUNT(*) FILTER (WHERE COALESCE(metadata->>'sla_current','') ~ '^[0-9]+(\.[0-9]+)?%$' AND COALESCE(metadata->>'sla_target','') ~ '^[0-9]+(\.[0-9]+)?%$' AND trim(trailing '%' from metadata->>'sla_current')::NUMERIC < trim(trailing '%' from metadata->>'sla_target')::NUMERIC),
		COALESCE(SUM(jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'ownership_candidates')='array' THEN metadata->'ownership_candidates' ELSE '[]'::jsonb END)),0),
		COUNT(*) FILTER (WHERE COALESCE(metadata->>'ticket_status','') <> '' AND COALESCE(metadata->>'ticket_status','') NOT IN ('已关闭','closed')),
		COALESCE(SUM(CASE asset_type
			WHEN 'endpoint' THEN jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'traffic_profile')='array' THEN metadata->'traffic_profile' ELSE '[]'::jsonb END)
			WHEN 'server' THEN jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'open_services')='array' THEN metadata->'open_services' ELSE '[]'::jsonb END)
			WHEN 'network-device' THEN jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'network_interfaces')='array' THEN metadata->'network_interfaces' ELSE '[]'::jsonb END)
			WHEN 'business-system' THEN jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'key_services')='array' THEN metadata->'key_services' ELSE '[]'::jsonb END)
			WHEN 'unknown' THEN jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'ownership_candidates')='array' THEN metadata->'ownership_candidates' ELSE '[]'::jsonb END)
			ELSE 0 END),0)
		FROM filtered_assets`, args...).Scan(
		&stats.Total, &stats.Active, &stats.Inactive, &stats.Unknown,
		&stats.HighCriticality, &stats.CriticalAssets, &stats.Unowned, &stats.OpenServices,
		&stats.HighRiskServices, &stats.WeakPasswords, &stats.NetworkInterfaces, &stats.ConfigurationChanges,
		&stats.DependencyAssets, &stats.KeyServices, &stats.SLAAtRisk, &stats.OwnershipCandidates,
		&stats.PendingTickets, &stats.ContextRecords,
	)
	if err != nil {
		return nil, fmt.Errorf("get asset stats: %w", err)
	}
	return &stats, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAsset(scanner rowScanner) (*config.AssetRecord, error) {
	var asset config.AssetRecord
	var displayCode, ip, mac, host, vendor, osType, vlan, swPort, department, campus, owner sql.NullString
	var tagsJSON, metadataJSON []byte
	if err := scanner.Scan(
		&asset.AssetID, &asset.Revision, &displayCode, &asset.TenantID, &asset.AssetType, &asset.Status,
		&ip, &mac, &host, &vendor, &osType, &asset.Source, &vlan, &swPort,
		&department, &campus, &owner, &asset.Criticality, &tagsJSON, &metadataJSON,
		&asset.FirstSeen, &asset.LastSeen,
	); err != nil {
		return nil, err
	}
	asset.DisplayCode = displayCode.String
	asset.IPAddress = ip.String
	asset.MACAddress = mac.String
	asset.Hostname = host.String
	asset.Vendor = vendor.String
	asset.OSType = osType.String
	asset.VlanID = vlan.String
	asset.SwitchPort = swPort.String
	asset.Department = department.String
	asset.Campus = campus.String
	asset.Owner = owner.String
	_ = json.Unmarshal(tagsJSON, &asset.Tags)
	_ = json.Unmarshal(metadataJSON, &asset.Metadata)
	return &asset, nil
}

func jsonObject(value map[string]any) []byte {
	if len(value) == 0 {
		return []byte("{}")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}

func (r *AssetRepository) GetHistory(ctx context.Context, tenantID, assetID string, limit int) ([]*config.AssetEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT event_id, asset_id, tenant_id, event_type, old_value, new_value, created_at
		 FROM asset_events WHERE tenant_id=$1 AND asset_id=$2 ORDER BY created_at DESC LIMIT $3`, tenantID, assetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*config.AssetEvent
	for rows.Next() {
		var e config.AssetEvent
		var oldVal, newVal sql.NullString
		if err := rows.Scan(&e.EventID, &e.AssetID, &e.TenantID, &e.EventType, &oldVal, &newVal, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan asset history: %w", err)
		}
		e.OldValue = oldVal.String
		e.NewValue = newVal.String
		result = append(result, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset history: %w", err)
	}
	return result, nil
}

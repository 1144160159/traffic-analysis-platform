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

func (r *AssetRepository) InitSchema(ctx context.Context) error {
	ddl := `
	CREATE TABLE IF NOT EXISTS assets (
		asset_id    TEXT PRIMARY KEY,
		display_code TEXT,
		tenant_id   TEXT NOT NULL,
		asset_type  TEXT NOT NULL DEFAULT 'unknown',
		status      TEXT NOT NULL DEFAULT 'active',
		ip          TEXT,
		ip_address  TEXT,
		mac_address TEXT NOT NULL,
		hostname    TEXT,
		vendor      TEXT,
		os_type     TEXT,
		source      TEXT NOT NULL DEFAULT 'manual',
		vlan_id     TEXT,
		switch_port TEXT,
		department  TEXT,
		campus      TEXT,
		owner       TEXT,
		tags        JSONB NOT NULL DEFAULT '{}'::jsonb,
		criticality INT NOT NULL DEFAULT 0,
		metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
		first_seen  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS ip TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS display_code TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS asset_type TEXT NOT NULL DEFAULT 'unknown';
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS ip_address TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS mac_address TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS hostname TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS vendor TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS os_type TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual';
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS vlan_id TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS switch_port TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS department TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS campus TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS owner TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '{}'::jsonb;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS criticality INT NOT NULL DEFAULT 0;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW();
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW();
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
	ALTER TABLE assets ALTER COLUMN ip DROP NOT NULL;
	UPDATE assets SET ip_address = ip WHERE (ip_address IS NULL OR ip_address = '') AND ip IS NOT NULL;
	UPDATE assets AS candidate
	SET ip = candidate.ip_address
	WHERE candidate.ip IS NULL
	  AND candidate.ip_address IS NOT NULL
	  AND (SELECT COUNT(*) FROM assets AS peer WHERE peer.tenant_id = candidate.tenant_id AND peer.ip_address = candidate.ip_address) = 1
	  AND NOT EXISTS (SELECT 1 FROM assets AS peer WHERE peer.tenant_id = candidate.tenant_id AND peer.ip = candidate.ip_address);
	CREATE TABLE IF NOT EXISTS asset_events (
		event_id   SERIAL PRIMARY KEY,
		asset_id   TEXT NOT NULL,
		tenant_id  TEXT NOT NULL,
		event_type TEXT NOT NULL,
		old_value  JSONB,
		new_value  JSONB,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS asset_discovery_credentials (
		credential_id TEXT PRIMARY KEY,
		tenant_id     TEXT NOT NULL,
		name          TEXT NOT NULL,
		protocol      TEXT NOT NULL,
		endpoint      TEXT,
		secret_ref    TEXT NOT NULL,
		created_by    TEXT,
		created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE (tenant_id, name)
	);
	CREATE TABLE IF NOT EXISTS asset_discovery_runs (
		run_id            TEXT PRIMARY KEY,
		tenant_id         TEXT NOT NULL,
		mode              TEXT NOT NULL,
		target_cidr       TEXT,
		credential_id     TEXT,
		status            TEXT NOT NULL DEFAULT 'queued',
		requested_by      TEXT,
		discovered_assets INT NOT NULL DEFAULT 0,
		discovered_links  INT NOT NULL DEFAULT 0,
		error_message     TEXT,
		started_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		completed_at      TIMESTAMPTZ
	);
	CREATE TABLE IF NOT EXISTS asset_topology_links (
		link_id            TEXT PRIMARY KEY,
		tenant_id          TEXT NOT NULL,
		run_id             TEXT,
		source_asset_id    TEXT,
		source_mac         TEXT,
		source_ip          TEXT,
		source_interface   TEXT NOT NULL DEFAULT '',
		neighbor_asset_id  TEXT,
		neighbor_mac       TEXT NOT NULL DEFAULT '',
		neighbor_ip        TEXT,
		neighbor_interface TEXT NOT NULL DEFAULT '',
		protocol           TEXT NOT NULL,
		confidence         INT NOT NULL DEFAULT 80,
		observed_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE (tenant_id, source_mac, neighbor_mac, protocol, source_interface, neighbor_interface)
	);
	CREATE INDEX IF NOT EXISTS idx_assets_tenant ON assets(tenant_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_display_code_unique ON assets(tenant_id, display_code) WHERE display_code IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_assets_tenant_type_status ON assets(tenant_id, asset_type, status, last_seen DESC);
	CREATE INDEX IF NOT EXISTS idx_assets_ip ON assets(tenant_id, ip_address);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_mac_unique ON assets(tenant_id, mac_address) WHERE mac_address IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_asset_events_asset ON asset_events(asset_id);
	CREATE INDEX IF NOT EXISTS idx_asset_discovery_runs_tenant ON asset_discovery_runs(tenant_id, started_at DESC);
	CREATE INDEX IF NOT EXISTS idx_asset_topology_links_tenant ON asset_topology_links(tenant_id, observed_at DESC);
	CREATE INDEX IF NOT EXISTS idx_asset_topology_links_asset ON asset_topology_links(tenant_id, source_asset_id, neighbor_asset_id);
	`
	_, err := r.db.ExecContext(ctx, ddl)
	return err
}

func (r *AssetRepository) Upsert(ctx context.Context, rec *config.AssetRecord) (string, bool, error) {
	existing, _ := r.findByMAC(ctx, rec.TenantID, rec.MACAddress)
	if existing != nil {
		mergeAssetGovernance(rec, existing)
		// Update
		oldJSON := assetToJSON(existing)
		_, err := r.db.ExecContext(ctx,
			`UPDATE assets SET display_code=$1, asset_type=$2, status=$3, ip_address=$4, hostname=$5,
			 vendor=$6, os_type=$7, source=$8, vlan_id=$9, switch_port=$10, department=$11,
			 campus=$12, owner=$13, criticality=$14, tags=$15, metadata=$16, last_seen=$17,
			 updated_at=NOW() WHERE tenant_id=$18 AND asset_id=$19`,
			rec.DisplayCode, rec.AssetType, rec.Status, rec.IPAddress, rec.Hostname,
			rec.Vendor, rec.OSType, rec.Source, rec.VlanID, rec.SwitchPort, rec.Department,
			rec.Campus, rec.Owner, rec.Criticality, jsonObject(rec.Tags), jsonObject(rec.Metadata),
			time.Now(), rec.TenantID, existing.AssetID)
		if err != nil {
			return "", false, fmt.Errorf("update asset: %w", err)
		}
		// Record change event
		newJSON := assetToJSON(rec)
		r.insertEvent(ctx, existing.AssetID, rec.TenantID, "updated", oldJSON, newJSON)
		return existing.AssetID, false, nil
	}

	// Insert
	ensureAssetDefaults(rec)
	rec.FirstSeen = time.Now()
	rec.LastSeen = time.Now()
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO assets (asset_id, display_code, tenant_id, asset_type, status, ip_address,
		 mac_address, hostname, vendor, os_type, source, vlan_id, switch_port, department,
		 campus, owner, criticality, tags, metadata, first_seen, last_seen)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		 RETURNING asset_id`,
		rec.AssetID, rec.DisplayCode, rec.TenantID, rec.AssetType, rec.Status, rec.IPAddress,
		rec.MACAddress, rec.Hostname, rec.Vendor, rec.OSType, rec.Source, rec.VlanID,
		rec.SwitchPort, rec.Department, rec.Campus, rec.Owner, rec.Criticality,
		jsonObject(rec.Tags), jsonObject(rec.Metadata), rec.FirstSeen, rec.LastSeen).Scan(&rec.AssetID)
	if err != nil {
		return "", false, fmt.Errorf("insert asset: %w", err)
	}
	r.insertEvent(ctx, rec.AssetID, rec.TenantID, "first_seen", "", assetToJSON(rec))
	return rec.AssetID, true, nil
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
		`SELECT asset_id, display_code, tenant_id, asset_type, status, ip_address, mac_address,
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
		`SELECT asset_id, display_code, tenant_id, asset_type, status, ip_address, mac_address,
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

func (r *AssetRepository) ListByTenantFiltered(ctx context.Context, tenantID string, filter config.AssetListFilter, limit, offset int) ([]*config.AssetRecord, int, error) {
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
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		placeholder := len(args)
		conditions = append(conditions, fmt.Sprintf("(display_code ILIKE $%d OR hostname ILIKE $%d OR ip_address ILIKE $%d OR mac_address ILIKE $%d)", placeholder, placeholder, placeholder, placeholder))
	}
	where := strings.Join(conditions, " AND ")
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM assets WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT asset_id, display_code, tenant_id, asset_type, status, ip_address, mac_address,
		 hostname, vendor, os_type, source, vlan_id, switch_port, department, campus, owner,
		 criticality, tags, metadata, first_seen, last_seen
		 FROM assets WHERE `+where+fmt.Sprintf(" ORDER BY last_seen DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
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
	return result, total, nil
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
		&asset.AssetID, &displayCode, &asset.TenantID, &asset.AssetType, &asset.Status,
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

func (r *AssetRepository) insertEvent(ctx context.Context, assetID, tenantID, eventType, oldVal, newVal string) {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO asset_events (asset_id, tenant_id, event_type, old_value, new_value) VALUES ($1,$2,$3,$4,$5)`,
		assetID, tenantID, eventType, jsonOrNil(oldVal), jsonOrNil(newVal))
	if err != nil {
		r.logger.Warn("failed to insert asset event", zap.String("asset_id", assetID), zap.Error(err))
	}
}

func (r *AssetRepository) InsertEvent(ctx context.Context, assetID, tenantID, eventType, oldVal, newVal string) {
	r.insertEvent(ctx, assetID, tenantID, eventType, oldVal, newVal)
}

func jsonOrNil(value string) any {
	if value == "" {
		return nil
	}
	return []byte(value)
}

// MarkInactiveSince 标记指定时间之前最后活跃的资产为 inactive
func (r *AssetRepository) MarkInactiveSince(ctx context.Context, tenantID string, since time.Time) (int, error) {
	rows, err := r.db.QueryContext(ctx,
		`WITH candidates AS (
			SELECT asset_id, status FROM assets
			WHERE tenant_id = $1 AND last_seen < $2 AND status IS DISTINCT FROM 'inactive'
			FOR UPDATE
		), updated AS (
			UPDATE assets AS asset
			SET status = 'inactive', updated_at = NOW()
			FROM candidates
			WHERE asset.asset_id = candidates.asset_id AND asset.tenant_id = $1
			RETURNING asset.asset_id, candidates.status AS old_status
		)
		SELECT asset_id, old_status FROM updated`,
		tenantID, since)
	if err != nil {
		return 0, fmt.Errorf("mark inactive: %w", err)
	}
	defer rows.Close()
	type inactiveChange struct {
		assetID   string
		oldStatus sql.NullString
	}
	changes := make([]inactiveChange, 0)
	for rows.Next() {
		var change inactiveChange
		if err := rows.Scan(&change.assetID, &change.oldStatus); err != nil {
			return 0, fmt.Errorf("scan inactive asset: %w", err)
		}
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate inactive assets: %w", err)
	}
	for _, change := range changes {
		oldValue, _ := json.Marshal(map[string]string{"status": change.oldStatus.String})
		r.insertEvent(ctx, change.assetID, tenantID, "inactive", string(oldValue), `{"status":"inactive"}`)
	}
	return len(changes), nil
}

func assetToJSON(a *config.AssetRecord) string {
	return fmt.Sprintf(`{"ip":"%s","mac":"%s","hostname":"%s","vendor":"%s"}`,
		a.IPAddress, a.MACAddress, a.Hostname, a.Vendor)
}

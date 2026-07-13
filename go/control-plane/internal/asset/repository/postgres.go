package repository

import (
	"context"
	"database/sql"
	"fmt"
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
		tenant_id   TEXT NOT NULL,
		ip          TEXT,
		ip_address  TEXT,
		mac_address TEXT NOT NULL,
		hostname    TEXT,
		vendor      TEXT,
		os_type     TEXT,
		source      TEXT NOT NULL DEFAULT 'manual',
		vlan_id     TEXT,
		switch_port TEXT,
		tags        JSONB NOT NULL DEFAULT '{}'::jsonb,
		criticality INT NOT NULL DEFAULT 0,
		metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
		first_seen  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS ip TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS ip_address TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS mac_address TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS hostname TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS vendor TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS os_type TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual';
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS vlan_id TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS switch_port TEXT;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '{}'::jsonb;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS criticality INT NOT NULL DEFAULT 0;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW();
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW();
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
	ALTER TABLE assets ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
	ALTER TABLE assets ALTER COLUMN ip DROP NOT NULL;
	UPDATE assets SET ip_address = ip WHERE (ip_address IS NULL OR ip_address = '') AND ip IS NOT NULL;
	UPDATE assets SET ip = ip_address WHERE ip IS NULL AND ip_address IS NOT NULL;
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
		// Update
		oldJSON := assetToJSON(existing)
		_, err := r.db.ExecContext(ctx,
			`UPDATE assets SET ip_address=$1, hostname=$2, vendor=$3, os_type=$4, source=$5,
			 vlan_id=$6, switch_port=$7, last_seen=$8 WHERE asset_id=$9`,
			rec.IPAddress, rec.Hostname, rec.Vendor, rec.OSType, rec.Source,
			rec.VlanID, rec.SwitchPort, time.Now(), existing.AssetID)
		if err != nil {
			return "", false, fmt.Errorf("update asset: %w", err)
		}
		// Record change event
		newJSON := assetToJSON(rec)
		r.insertEvent(ctx, existing.AssetID, rec.TenantID, "updated", oldJSON, newJSON)
		return existing.AssetID, false, nil
	}

	// Insert
	rec.FirstSeen = time.Now()
	rec.LastSeen = time.Now()
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO assets (asset_id, tenant_id, ip_address, mac_address, hostname, vendor, os_type, source, vlan_id, switch_port, first_seen, last_seen)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING asset_id`,
		rec.AssetID, rec.TenantID, rec.IPAddress, rec.MACAddress, rec.Hostname,
		rec.Vendor, rec.OSType, rec.Source, rec.VlanID, rec.SwitchPort,
		rec.FirstSeen, rec.LastSeen).Scan(&rec.AssetID)
	if err != nil {
		return "", false, fmt.Errorf("insert asset: %w", err)
	}
	r.insertEvent(ctx, rec.AssetID, rec.TenantID, "first_seen", "", assetToJSON(rec))
	return rec.AssetID, true, nil
}

func (r *AssetRepository) FindByMAC(ctx context.Context, tenantID, mac string) (*config.AssetRecord, error) {
	return r.findByMAC(ctx, tenantID, mac)
}

func (r *AssetRepository) findByMAC(ctx context.Context, tenantID, mac string) (*config.AssetRecord, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT asset_id, tenant_id, ip_address, mac_address, hostname, vendor, os_type, source, vlan_id, switch_port, first_seen, last_seen
		 FROM assets WHERE tenant_id=$1 AND mac_address=$2`, tenantID, mac)
	var a config.AssetRecord
	var ip, host, vendor, osType, vlan, swPort sql.NullString
	var fs, ls time.Time
	err := row.Scan(&a.AssetID, &a.TenantID, &ip, &a.MACAddress, &host, &vendor, &osType, &a.Source, &vlan, &swPort, &fs, &ls)
	if err == sql.ErrNoRows {
		return nil, errors.New(errors.ErrCodeTenantNotFound, "asset not found: "+mac)
	}
	if err != nil {
		return nil, err
	}
	a.IPAddress = ip.String
	a.Hostname = host.String
	a.Vendor = vendor.String
	a.OSType = osType.String
	a.VlanID = vlan.String
	a.SwitchPort = swPort.String
	a.FirstSeen = fs
	a.LastSeen = ls
	return &a, nil
}

func (r *AssetRepository) FindByID(ctx context.Context, assetID string) (*config.AssetRecord, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT asset_id, tenant_id, ip_address, mac_address, hostname, vendor, os_type, source, vlan_id, switch_port, first_seen, last_seen
		 FROM assets WHERE asset_id=$1`, assetID)
	var a config.AssetRecord
	var ip, host, vendor, osType, vlan, swPort sql.NullString
	var fs, ls time.Time
	err := row.Scan(&a.AssetID, &a.TenantID, &ip, &a.MACAddress, &host, &vendor, &osType, &a.Source, &vlan, &swPort, &fs, &ls)
	if err == sql.ErrNoRows {
		return nil, errors.New(errors.ErrCodeTenantNotFound, "asset not found: "+assetID)
	}
	if err != nil {
		return nil, err
	}
	a.IPAddress = ip.String
	a.Hostname = host.String
	a.Vendor = vendor.String
	a.OSType = osType.String
	a.VlanID = vlan.String
	a.SwitchPort = swPort.String
	a.FirstSeen = fs
	a.LastSeen = ls
	return &a, nil
}

func (r *AssetRepository) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*config.AssetRecord, int, error) {
	var total int
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM assets WHERE tenant_id=$1", tenantID).Scan(&total)

	rows, err := r.db.QueryContext(ctx,
		`SELECT asset_id, tenant_id, ip_address, mac_address, hostname, vendor, os_type, source, vlan_id, switch_port, first_seen, last_seen
		 FROM assets WHERE tenant_id=$1 ORDER BY last_seen DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []*config.AssetRecord
	for rows.Next() {
		var a config.AssetRecord
		var ip, host, vendor, osType, vlan, swPort sql.NullString
		var fs, ls time.Time
		rows.Scan(&a.AssetID, &a.TenantID, &ip, &a.MACAddress, &host, &vendor, &osType, &a.Source, &vlan, &swPort, &fs, &ls)
		a.IPAddress = ip.String
		a.Hostname = host.String
		a.Vendor = vendor.String
		a.OSType = osType.String
		a.VlanID = vlan.String
		a.SwitchPort = swPort.String
		a.FirstSeen = fs
		a.LastSeen = ls
		result = append(result, &a)
	}
	return result, total, nil
}

func (r *AssetRepository) GetHistory(ctx context.Context, assetID string, limit int) ([]*config.AssetEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT event_id, asset_id, tenant_id, event_type, old_value, new_value, created_at
		 FROM asset_events WHERE asset_id=$1 ORDER BY created_at DESC LIMIT $2`, assetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*config.AssetEvent
	for rows.Next() {
		var e config.AssetEvent
		var oldVal, newVal sql.NullString
		rows.Scan(&e.EventID, &e.AssetID, &e.TenantID, &e.EventType, &oldVal, &newVal, &e.CreatedAt)
		e.OldValue = oldVal.String
		e.NewValue = newVal.String
		result = append(result, &e)
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
	result, err := r.db.ExecContext(ctx,
		`UPDATE assets SET last_seen = last_seen WHERE tenant_id = $1 AND last_seen < $2`,
		tenantID, since)
	if err != nil {
		return 0, fmt.Errorf("mark inactive: %w", err)
	}
	n, _ := result.RowsAffected()
	// 为每个被标记的资产记录 inactive 事件
	if n > 0 {
		rows, err := r.db.QueryContext(ctx,
			`SELECT asset_id FROM assets WHERE tenant_id = $1 AND last_seen < $2`, tenantID, since)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var assetID string
				if rows.Scan(&assetID) == nil {
					r.insertEvent(ctx, assetID, tenantID, "inactive", "", "")
				}
			}
		}
	}
	return int(n), nil
}

func assetToJSON(a *config.AssetRecord) string {
	return fmt.Sprintf(`{"ip":"%s","mac":"%s","hostname":"%s","vendor":"%s"}`,
		a.IPAddress, a.MACAddress, a.Hostname, a.Vendor)
}

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
		ip_address  TEXT,
		mac_address TEXT NOT NULL,
		hostname    TEXT,
		vendor      TEXT,
		os_type     TEXT,
		source      TEXT NOT NULL DEFAULT 'manual',
		vlan_id     TEXT,
		switch_port TEXT,
		first_seen  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(tenant_id, mac_address)
	);
	CREATE TABLE IF NOT EXISTS asset_events (
		event_id   SERIAL PRIMARY KEY,
		asset_id   TEXT NOT NULL REFERENCES assets(asset_id),
		tenant_id  TEXT NOT NULL,
		event_type TEXT NOT NULL,
		old_value  JSONB,
		new_value  JSONB,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_assets_tenant ON assets(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_assets_mac ON assets(tenant_id, mac_address);
	CREATE INDEX IF NOT EXISTS idx_asset_events_asset ON asset_events(asset_id);
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
	if err != nil { return nil, err }
	a.IPAddress = ip.String; a.Hostname = host.String; a.Vendor = vendor.String
	a.OSType = osType.String; a.VlanID = vlan.String; a.SwitchPort = swPort.String
	a.FirstSeen = fs; a.LastSeen = ls
	return &a, nil
}

func (r *AssetRepository) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*config.AssetRecord, int, error) {
	var total int
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM assets WHERE tenant_id=$1", tenantID).Scan(&total)

	rows, err := r.db.QueryContext(ctx,
		`SELECT asset_id, tenant_id, ip_address, mac_address, hostname, vendor, os_type, source, vlan_id, switch_port, first_seen, last_seen
		 FROM assets WHERE tenant_id=$1 ORDER BY last_seen DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil { return nil, 0, err }
	defer rows.Close()

	var result []*config.AssetRecord
	for rows.Next() {
		var a config.AssetRecord
		var ip, host, vendor, osType, vlan, swPort sql.NullString
		var fs, ls time.Time
		rows.Scan(&a.AssetID, &a.TenantID, &ip, &a.MACAddress, &host, &vendor, &osType, &a.Source, &vlan, &swPort, &fs, &ls)
		a.IPAddress = ip.String; a.Hostname = host.String; a.Vendor = vendor.String
		a.OSType = osType.String; a.VlanID = vlan.String; a.SwitchPort = swPort.String
		a.FirstSeen = fs; a.LastSeen = ls
		result = append(result, &a)
	}
	return result, total, nil
}

func (r *AssetRepository) GetHistory(ctx context.Context, assetID string, limit int) ([]*config.AssetEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT event_id, asset_id, tenant_id, event_type, old_value, new_value, created_at
		 FROM asset_events WHERE asset_id=$1 ORDER BY created_at DESC LIMIT $2`, assetID, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	var result []*config.AssetEvent
	for rows.Next() {
		var e config.AssetEvent
		var oldVal, newVal sql.NullString
		rows.Scan(&e.EventID, &e.AssetID, &e.TenantID, &e.EventType, &oldVal, &newVal, &e.CreatedAt)
		e.OldValue = oldVal.String; e.NewValue = newVal.String
		result = append(result, &e)
	}
	return result, nil
}

func (r *AssetRepository) insertEvent(ctx context.Context, assetID, tenantID, eventType, oldVal, newVal string) {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO asset_events (asset_id, tenant_id, event_type, old_value, new_value) VALUES ($1,$2,$3,$4,$5)`,
		assetID, tenantID, eventType, oldVal, newVal)
	if err != nil {
		r.logger.Warn("failed to insert asset event", zap.String("asset_id", assetID), zap.Error(err))
	}
}

func assetToJSON(a *config.AssetRecord) string {
	return fmt.Sprintf(`{"ip":"%s","mac":"%s","hostname":"%s","vendor":"%s"}`,
		a.IPAddress, a.MACAddress, a.Hostname, a.Vendor)
}


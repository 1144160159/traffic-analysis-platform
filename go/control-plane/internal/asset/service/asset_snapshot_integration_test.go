package service_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"slices"
	"testing"

	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
)

const assetSnapshotIntegrationTenant = "asset-snapshot-integration"
const assetSnapshotIntegrationID = "20000000-0000-4000-8000-000000000001"

func TestAssetDetailSnapshotRepeatableReadAndExplicitPartial(t *testing.T) {
	dsn := os.Getenv("ASSET_ATOMIC_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("ASSET_ATOMIC_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_asset_atomic_test_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	cleanup := func() {
		for _, table := range []string{"asset_topology_links", "asset_events", "assets", "tenants"} {
			if _, cleanupErr := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE tenant_id=$1", table), assetSnapshotIntegrationTenant); cleanupErr != nil {
				t.Errorf("cleanup %s: %v", table, cleanupErr)
			}
		}
	}
	cleanup()
	defer cleanup()
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Asset Snapshot Integration')`, assetSnapshotIntegrationTenant); err != nil {
		t.Fatal(err)
	}
	metadata := `{
		"data_contract":"canonical-asset-detail-v1",
		"network_interfaces":[{"name":"eth0","adapter":"Intel X710","ip_address":"192.0.2.80","mac_address":"02:00:00:00:00:80","vlan_id":"80","mirror_mode":"no","status":"up","speed":"10Gbps","duplex":"full","ingress_bytes":100,"egress_bytes":200,"packet_loss_pct":0,"error_count":0,"probe_id":"probe-80"}],
		"open_services":[{"port":443,"protocol":"tcp","service":"https","version":"1.3","exposure_scope":"园区","access_source_count":3,"risk_level":"低危","alert_count":0}],
		"ownership":{"business_systems":[{"name":"portal","role":"member","owner":"dana","status":"confirmed"}],"asset_groups":[],"data_domains":[],"responsibilities":[{"role":"owner","owner":"dana","status":"confirmed"}],"pending_fields":[]}
	}`
	if _, err := db.Exec(`
		INSERT INTO assets(
		  asset_id,tenant_id,display_code,asset_type,status,ip_address,mac_address,
		  hostname,department,campus,owner,criticality,source,metadata,last_seen
		) VALUES($1,$2,'SNAP-001','server','active','192.0.2.80','02:00:00:00:00:80',
		  'snapshot-server','security','east','dana',80,'integration',$3::jsonb,now())`,
		assetSnapshotIntegrationID, assetSnapshotIntegrationTenant, metadata); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO asset_events(asset_id,tenant_id,event_type,revision,trace_id,new_value)
		VALUES($1,$2,'updated',1,'trace-asset-snapshot','{"status":"active"}'::jsonb)`,
		assetSnapshotIntegrationID, assetSnapshotIntegrationTenant); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO asset_topology_links(
		  link_id,tenant_id,source_asset_id,source_mac,source_ip,source_interface,
		  neighbor_asset_id,neighbor_mac,neighbor_ip,neighbor_interface,protocol,confidence,observed_at
		) VALUES(
		  'snapshot-link-1',$1,$2,'02:00:00:00:00:80','192.0.2.80','eth0',
		  NULL,'02:00:00:00:00:81','192.0.2.81','eth1','lldp',92,now())`,
		assetSnapshotIntegrationTenant, assetSnapshotIntegrationID); err != nil {
		t.Fatal(err)
	}

	repo, err := repository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(&config.Config{}, repo, zap.NewNop())
	snapshot, err := svc.GetAssetDetailSnapshot(context.Background(), assetSnapshotIntegrationTenant, assetSnapshotIntegrationID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContractVersion != 1 || snapshot.SnapshotID == "" || snapshot.Asset.AssetID != assetSnapshotIntegrationID || snapshot.Asset.TenantID != assetSnapshotIntegrationTenant {
		t.Fatalf("snapshot identity=%+v", snapshot)
	}
	if !snapshot.Partial || !slices.Contains(snapshot.MissingSections, "clickhouse_observations") || !slices.Contains(snapshot.MissingSections, "evidence_objects") {
		t.Fatalf("partial=%v missing=%v", snapshot.Partial, snapshot.MissingSections)
	}
	if len(snapshot.Details.NetworkInterfaces) != 1 || len(snapshot.Details.OpenServices) != 1 || snapshot.Details.Ownership.Owner != "dana" {
		t.Fatalf("details=%+v", snapshot.Details)
	}
	if len(snapshot.History) != 1 || snapshot.SourceWatermarks["postgresql.asset_events.max_event_id"] == "0" {
		t.Fatalf("history=%+v watermarks=%v", snapshot.History, snapshot.SourceWatermarks)
	}
	if snapshot.Topology.Source != "discovery_neighbors" || len(snapshot.Topology.Edges) != 1 || snapshot.Topology.Edges[0].Protocol != "lldp" {
		t.Fatalf("topology=%+v", snapshot.Topology)
	}
	if snapshot.SourceWatermarks["postgresql.snapshot_xids"] == "" || snapshot.SourceWatermarks["postgresql.assets.revision"] != "1" || snapshot.SourceWatermarks["postgresql.asset_topology_links.observed_at"] == "" {
		t.Fatalf("watermarks=%v", snapshot.SourceWatermarks)
	}
	if _, err := svc.GetAssetDetailSnapshot(context.Background(), "asset-snapshot-other", assetSnapshotIntegrationID, 50); err == nil {
		t.Fatal("cross-tenant snapshot read should fail")
	}
}

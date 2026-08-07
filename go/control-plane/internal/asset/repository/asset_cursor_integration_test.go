package repository_test

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	assetRepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
)

// This test is guarded by the same explicit DSN and sentinel as the atomic
// upsert integration. It proves cursor boundaries against real PostgreSQL
// tuple ordering and snapshot timestamps without touching a shared database.
func TestAssetCursorPostgresSnapshotStability(t *testing.T) {
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

	const tenantA = "asset-cursor-integration-a"
	const tenantB = "asset-cursor-integration-b"
	if _, err := db.Exec(`DELETE FROM tenants WHERE tenant_id IN ($1,$2)`, tenantA, tenantB); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO tenants(tenant_id,name)
		VALUES ($1,'Asset Cursor A'),($2,'Asset Cursor B')`, tenantA, tenantB); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(`DELETE FROM tenants WHERE tenant_id IN ($1,$2)`, tenantA, tenantB); err != nil {
			t.Errorf("cleanup cursor tenants: %v", err)
		}
	}()

	lastSeen := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	originalIDs := []string{
		"00000000-0000-4000-8000-000000000005",
		"00000000-0000-4000-8000-000000000004",
		"00000000-0000-4000-8000-000000000003",
		"00000000-0000-4000-8000-000000000002",
		"00000000-0000-4000-8000-000000000001",
	}
	for index, assetID := range originalIDs {
		if _, err := db.Exec(`
			INSERT INTO assets(
			  asset_id,tenant_id,asset_type,status,source,mac_address,
			  display_code,first_seen,last_seen,updated_at
			) VALUES ($1,$2,'server','active','cursor-test',$3,$4,$5,$5,$5)`,
			assetID,
			tenantA,
			"02:00:00:00:00:0"+string(rune('1'+index)),
			"CURSOR-"+assetID[len(assetID)-3:],
			lastSeen,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO assets(
		  asset_id,tenant_id,asset_type,status,source,mac_address,
		  display_code,first_seen,last_seen,updated_at
		) VALUES (
		  '10000000-0000-4000-8000-000000000001',$1,'server','active',
		  'cursor-test','02:00:00:00:01:01','OTHER-TENANT',$2,$2,$2
		)`,
		tenantB, lastSeen,
	); err != nil {
		t.Fatal(err)
	}

	// Start an insert transaction before the first page, but do not commit it
	// until after the first page snapshot has been captured. A timestamp-only
	// watermark would allow this row into a later page because now() reflects
	// the transaction start. The signed PostgreSQL MVCC snapshot must exclude
	// it for the entire traversal.
	longTransaction, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	const longTransactionID = "00000000-0000-4000-8000-000000000000"
	if _, err := longTransaction.Exec(`
		INSERT INTO assets(
		  asset_id,tenant_id,asset_type,status,source,mac_address,
		  display_code,first_seen,last_seen
		) VALUES (
		  $1,$2,'server','active','cursor-test','02:00:00:00:00:98',
		  'LONG-TRANSACTION-INSERT',$3,$3
		)`,
		longTransactionID, tenantA, lastSeen,
	); err != nil {
		_ = longTransaction.Rollback()
		t.Fatal(err)
	}

	repo, err := assetRepository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	filter := config.AssetListFilter{AssetType: "server", Status: "active"}
	first, err := repo.ListByTenantCursor(context.Background(), tenantA, filter, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != len(originalIDs) || !first.HasMore {
		t.Fatalf("first page total=%d has_more=%v", first.Total, first.HasMore)
	}
	if first.SnapshotXIDs == "" {
		t.Fatal("first page is missing PostgreSQL MVCC snapshot")
	}
	if err := longTransaction.Commit(); err != nil {
		t.Fatal(err)
	}

	// This row sorts ahead of all original rows but is inserted after the
	// snapshot watermark. It must not appear on any remaining page.
	time.Sleep(2 * time.Millisecond)
	const concurrentID = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	if _, err := db.Exec(`
		INSERT INTO assets(
		  asset_id,tenant_id,asset_type,status,source,mac_address,
		  display_code,first_seen,last_seen
		) VALUES (
		  $1,$2,'server','active','cursor-test','02:00:00:00:00:99',
		  'CONCURRENT-INSERT',now(),now()
		)`,
		concurrentID, tenantA,
	); err != nil {
		t.Fatal(err)
	}

	got := assetIDs(first.Assets)
	page := first
	for page.HasMore {
		page, err = repo.ListByTenantCursor(context.Background(), tenantA, filter, 2, &config.AssetCursorPosition{
			SnapshotAt:   page.SnapshotAt,
			SnapshotXIDs: page.SnapshotXIDs,
			LastSeen:     page.LastSeen,
			LastAssetID:  page.LastAssetID,
			Total:        page.Total,
		})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != len(originalIDs) {
			t.Fatalf("cursor total changed to %d", page.Total)
		}
		got = append(got, assetIDs(page.Assets)...)
	}
	if !reflect.DeepEqual(got, originalIDs) {
		t.Fatalf("snapshot traversal=%v want=%v", got, originalIDs)
	}
	for _, assetID := range got {
		if assetID == concurrentID || assetID == longTransactionID {
			t.Fatalf("concurrent insert %s leaked into the fixed snapshot", assetID)
		}
	}
}

func assetIDs(assets []*config.AssetRecord) []string {
	result := make([]string, 0, len(assets))
	for _, asset := range assets {
		result = append(result, asset.AssetID)
	}
	return result
}

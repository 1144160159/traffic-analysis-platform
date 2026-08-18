package attackchain

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

func TestAttackChainRepositoryPostgresAtomicSnapshot(t *testing.T) {
	dsn := os.Getenv("ATTACK_CHAIN_POSTGRES_TEST_DSN")
	if dsn == "" { t.Skip("ATTACK_CHAIN_POSTGRES_TEST_DSN is not set") }
	db, err := sql.Open("postgres", dsn)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil { t.Fatal(err) }
	repository, err := NewRepository(db)
	if err != nil { t.Fatal(err) }
	if err := repository.VerifySchema(ctx); err != nil { t.Fatal(err) }
	input := validAssembleInput()
	snapshot, err := Assemble(input)
	if err != nil { t.Fatal(err) }
	if err := repository.Save(ctx, snapshot); err != nil { t.Fatal(err) }
	if err := repository.Save(ctx, snapshot); err != nil { t.Fatalf("exact replay failed: %v", err) }
	stored, err := repository.LoadCurrent(ctx, snapshot.TenantID, snapshot.ChainID)
	if err != nil { t.Fatal(err) }
	if stored.SnapshotSHA256 != snapshot.SnapshotSHA256 { t.Fatalf("stored snapshot hash drifted: %s", stored.SnapshotSHA256) }
	items, total, err := repository.ListCurrent(ctx, snapshot.TenantID, 10, 0)
	if err != nil || total != 1 || len(items) != 1 { t.Fatalf("unexpected list result total=%d items=%d err=%v", total, len(items), err) }
	if _, err := repository.LoadCurrent(ctx, "tenant-b", snapshot.ChainID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant snapshot was visible: %v", err)
	}
	gap := validAssembleInput()
	gap.Version, gap.SnapshotID, gap.GraphSnapshot.SnapshotID = 3, "snapshot-3", "graph-snapshot-3"
	gapSnapshot, err := Assemble(gap)
	if err != nil { t.Fatal(err) }
	if err := repository.Save(ctx, gapSnapshot); err == nil || !strings.Contains(err.Error(), "advance exactly once") {
		t.Fatalf("version gap was not rejected: %v", err)
	}
	var snapshots, graphs, evidence int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM attack_chain_snapshots_v1 WHERE tenant_id=$1`, snapshot.TenantID).Scan(&snapshots); err != nil { t.Fatal(err) }
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gnn_graph_snapshots_v1 WHERE tenant_id=$1`, snapshot.TenantID).Scan(&graphs); err != nil { t.Fatal(err) }
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM attack_chain_evidence_manifest_v1 WHERE tenant_id=$1`, snapshot.TenantID).Scan(&evidence); err != nil { t.Fatal(err) }
	if snapshots != 1 || graphs != 1 || evidence != 3 { t.Fatalf("unexpected immutable row counts snapshots=%d graphs=%d evidence=%d", snapshots, graphs, evidence) }
}

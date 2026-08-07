package persistence

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestProjectionWatermarkReceiptRealPostgres(t *testing.T) {
	dsn := os.Getenv("ALERT_PROJECTION_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("ALERT_PROJECTION_EPHEMERAL_PG_DSN is not set")
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	host, _, err := net.SplitHostPort(parsed.Host)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || parsed.Query().Get("sslmode") != "disable" {
		t.Fatalf("refusing non-loopback ephemeral PostgreSQL DSN: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var marker string
	if err := db.QueryRowContext(ctx, `SELECT marker FROM codex_ephemeral_alert_projection_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing PostgreSQL without ephemeral sentinel: marker=%q err=%v", marker, err)
	}
	store := NewProjectionDebtStore(db)
	if err := store.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	alert := &Alert{
		TenantID: "tenant-alert-watermark-g1", AlertID: "alert-watermark-g1", EventID: "event-watermark-g1",
		Status: "new", FirstSeen: time.Date(2026, 8, 7, 13, 30, 0, 0, time.UTC),
		LastSeen: time.Date(2026, 8, 7, 13, 31, 0, 0, time.UTC), UpdatedTs: time.Date(2026, 8, 7, 13, 32, 0, 0, time.UTC),
	}
	defer db.ExecContext(ctx, `DELETE FROM alert_opensearch_projection_watermarks WHERE tenant_id=$1`, alert.TenantID)

	assertMismatches := func(want int) {
		t.Helper()
		mismatches, err := store.ListProjectionWatermarkMismatches(ctx, []*Alert{alert}, "alerts-v2-write")
		if err != nil {
			t.Fatal(err)
		}
		if len(mismatches) != want || (want == 1 && mismatches[0] != alert.AlertID) {
			t.Fatalf("watermark mismatches=%v, want count %d", mismatches, want)
		}
	}
	assertMismatches(1)
	if err := store.RecordProjectionApplied(ctx, alert, "alerts-v2-write"); err != nil {
		t.Fatal(err)
	}
	assertMismatches(0)
	otherTargetMismatches, err := store.ListProjectionWatermarkMismatches(ctx, []*Alert{alert}, "alerts-v2-canary")
	if err != nil {
		t.Fatal(err)
	}
	if len(otherTargetMismatches) != 1 || otherTargetMismatches[0] != alert.AlertID {
		t.Fatalf("watermark from another target version leaked into comparison: %v", otherTargetMismatches)
	}

	// A same-millisecond authoritative content change must be detected by SHA,
	// then safely replace the same external_gte receipt and converge again.
	alert.Status = "closed"
	assertMismatches(1)
	if err := store.RecordProjectionApplied(ctx, alert, "alerts-v2-write"); err != nil {
		t.Fatal(err)
	}
	assertMismatches(0)
}

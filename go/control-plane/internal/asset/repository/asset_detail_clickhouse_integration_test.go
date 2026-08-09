package repository

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func TestAssetDetailRealClickHouseFactsWatermarksAndFailureBoundary(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ASSET_DETAIL_EPHEMERAL_CLICKHOUSE_DSN"))
	if dsn == "" {
		t.Skip("ASSET_DETAIL_EPHEMERAL_CLICKHOUSE_DSN is not set")
	}
	if os.Getenv("ASSET_DETAIL_EPHEMERAL_CLICKHOUSE_SENTINEL") != "ephemeral-only" {
		t.Fatal("ASSET_DETAIL_EPHEMERAL_CLICKHOUSE_SENTINEL must equal ephemeral-only")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "clickhouse" {
		t.Fatalf("invalid ephemeral ClickHouse DSN: %q", dsn)
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "localhost" {
		t.Fatalf("ephemeral ClickHouse must use loopback, got %q", host)
	}
	if port := parsed.Port(); port == "" {
		t.Fatal("ephemeral ClickHouse DSN must contain an explicit mapped port")
	} else if _, err := net.LookupPort("tcp", port); err != nil {
		t.Fatalf("invalid ephemeral ClickHouse port %q: %v", port, err)
	}

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var marker string
	if err := db.QueryRowContext(ctx, `SELECT marker FROM traffic.codex_ephemeral_asset_detail_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel ClickHouse: marker=%q err=%v", marker, err)
	}
	for _, table := range []string{"traffic.sessions", "traffic.alerts"} {
		if _, err := db.ExecContext(ctx, "TRUNCATE TABLE "+table); err != nil {
			t.Fatal(err)
		}
	}

	asOf := time.Now().UTC().Truncate(time.Millisecond).Add(-10 * time.Second)
	asset := &config.AssetRecord{
		AssetID: "asset-clickhouse-g1", TenantID: "tenant-a",
		IPAddress: "192.0.2.8", Revision: 19,
	}
	insertSession := func(tenant, sessionID, src, dst string, protocol uint8, start, end time.Time, bytes uint64, packets uint32) {
		t.Helper()
		_, err := db.ExecContext(ctx, `
			INSERT INTO traffic.sessions
				(session_id, tenant_id, community_id, ts_start, ts_end, event_id, src_ip, dst_ip, protocol, bytes_total, num_pkts)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sessionID, tenant, "community-g1", start.UnixMilli(), end.UnixMilli(), "event-"+sessionID,
			src, dst, protocol, bytes, packets,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	insertSession("tenant-a", "session-1", asset.IPAddress, "203.0.113.9", 6, asOf.Add(-2*time.Hour), asOf.Add(-119*time.Minute), 1024, 10)
	insertSession("tenant-a", "session-2", "198.51.100.2", asset.IPAddress, 17, asOf.Add(-time.Hour), asOf.Add(-59*time.Minute), 2048, 20)
	insertSession("tenant-b", "session-other-tenant", asset.IPAddress, "203.0.113.10", 6, asOf.Add(-time.Hour), asOf.Add(-59*time.Minute), 9999, 99)
	insertSession("tenant-a", "session-future-end", asset.IPAddress, "203.0.113.11", 6, asOf.Add(-time.Minute), asOf.Add(time.Minute), 8888, 88)
	insertSession("tenant-a", "session-future-start", asset.IPAddress, "203.0.113.12", 6, asOf.Add(time.Minute), asOf.Add(2*time.Minute), 7777, 77)
	insertSession("tenant-a", "session-before-window", asset.IPAddress, "203.0.113.13", 6, asOf.Add(-25*time.Hour), asOf.Add(-25*time.Hour+time.Minute), 6666, 66)

	reader, err := NewAssetDetailClickHouseReader(db, config.AssetDetailConfig{
		ClickHouseQuery: 10 * time.Second, ClickHouseLookback: 24 * time.Hour, ClickHouseAlertLimit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	observations, observationMarks, err := reader.ReadAssetObservations(ctx, asset.TenantID, asset, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if observations.SessionCount != 2 || observations.BytesTotal != 3072 || observations.PacketsTotal != 30 || observations.DistinctPeers != 2 {
		t.Fatalf("observation tenant/as_of bounds failed: %+v", observations)
	}
	if len(observations.Protocols) != 2 || observations.Protocols[0] != 6 || observations.Protocols[1] != 17 {
		t.Fatalf("protocols=%v", observations.Protocols)
	}
	if observations.ResolvedIdentity.Value != asset.IPAddress || observations.ResolvedIdentity.AssetRevision != asset.Revision {
		t.Fatalf("resolved_identity=%+v", observations.ResolvedIdentity)
	}
	if observationMarks["clickhouse.sessions.query_as_of"] != asOf.Format(time.RFC3339Nano) ||
		observationMarks["clickhouse.sessions.max_ts_end"] != asOf.Add(-59*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("observation watermarks=%v", observationMarks)
	}

	insertAlert := func(tenant, alertID, severity, status string, version uint64, lastSeen time.Time, eventID string) {
		t.Helper()
		_, err := db.ExecContext(ctx, `
			INSERT INTO traffic.alerts
				(tenant_id, alert_id, src_ip, dst_ip, src_port, dst_port, protocol, alert_type, severity, score, status,
				 evidence_ids, first_seen, last_seen, state_version, event_id, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tenant, alertID, asset.IPAddress, "203.0.113.20", uint32(44000), uint32(443), uint32(6), "exfil",
			severity, float32(90), status, []string{"evidence-" + alertID}, asOf.Add(-3*time.Hour).UnixMilli(),
			lastSeen.UnixMilli(), version, eventID, lastSeen.UnixMilli(),
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	insertAlert("tenant-a", "alert-1", "medium", "open", 1, asOf.Add(-90*time.Minute), "alert-event-1")
	insertAlert("tenant-a", "alert-1", "high", "investigating", 2, asOf.Add(-30*time.Minute), "alert-event-2")
	insertAlert("tenant-a", "alert-1", "critical", "closed", 3, asOf.Add(time.Minute), "alert-event-future")
	insertAlert("tenant-a", "alert-2", "high", "open", 1, asOf.Add(-20*time.Minute), "alert-event-3")
	insertAlert("tenant-a", "alert-3", "low", "open", 1, asOf.Add(-10*time.Minute), "alert-event-4")
	insertAlert("tenant-b", "alert-cross-tenant", "critical", "open", 1, asOf.Add(-5*time.Minute), "alert-event-other")

	alerts, alertMarks, err := reader.ReadAssetAlertContext(ctx, asset.TenantID, asset, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts.Alerts) != 2 || !alerts.Truncated {
		t.Fatalf("bounded alert result=%+v", alerts)
	}
	if alerts.Alerts[0].AlertID != "alert-3" || alerts.Alerts[1].AlertID != "alert-2" {
		t.Fatalf("alert order or tenant isolation failed: %+v", alerts.Alerts)
	}
	for _, item := range alerts.Alerts {
		if item.AlertID == "alert-cross-tenant" || item.EventID == "alert-event-future" {
			t.Fatalf("future or cross-tenant state leaked: %+v", item)
		}
	}
	if alertMarks["clickhouse.alerts.query_as_of"] != asOf.Format(time.RFC3339Nano) ||
		alertMarks["clickhouse.alerts.max_last_seen"] != asOf.Add(-10*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("alert watermarks=%v", alertMarks)
	}

	if _, err := db.ExecContext(ctx, `RENAME TABLE traffic.sessions TO traffic.sessions_unavailable`); err != nil {
		t.Fatal(err)
	}
	if result, marks, err := reader.ReadAssetObservations(ctx, asset.TenantID, asset, asOf); err == nil || result != nil || marks != nil {
		t.Fatalf("unavailable fact source must fail, result=%+v marks=%v err=%v", result, marks, err)
	}
}

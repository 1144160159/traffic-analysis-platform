package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func newAssetDetailClickHouseTestReader(t *testing.T, alertLimit int) (*AssetDetailClickHouseReader, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewAssetDetailClickHouseReader(db, config.AssetDetailConfig{
		ClickHouseQuery: time.Second, ClickHouseLookback: 24 * time.Hour, ClickHouseAlertLimit: alertLimit,
	})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	return reader, mock, func() { db.Close() }
}

func TestAssetDetailClickHouseObservationsBindTenantIdentityAndPostgresAsOf(t *testing.T) {
	reader, mock, closeDB := newAssetDetailClickHouseTestReader(t, 2)
	defer closeDB()
	asOf := time.Date(2026, 8, 1, 5, 0, 0, 123000000, time.UTC)
	asset := &config.AssetRecord{AssetID: "asset-1", TenantID: "tenant-a", IPAddress: "192.0.2.8", Revision: 7}
	windowStart := asOf.Add(-24 * time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("FROM traffic.sessions")).
		WithArgs(asset.IPAddress, asset.TenantID, windowStart.UnixMilli(), asOf.UnixMilli(), asOf.UnixMilli(), asset.IPAddress, asset.IPAddress).
		WillReturnRows(sqlmock.NewRows([]string{
			"session_count", "bytes_total", "packets_total", "distinct_peers", "protocols_json", "first_observed", "last_observed",
		}).AddRow(int64(9), int64(1024), int64(88), int64(3), `[6,17]`, windowStart.Add(time.Minute).UnixMilli(), asOf.Add(-time.Second).UnixMilli()))

	result, watermarks, err := reader.ReadAssetObservations(context.Background(), asset.TenantID, asset, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssetID != asset.AssetID || result.ResolvedIdentity.Value != asset.IPAddress || result.ResolvedIdentity.AssetRevision != 7 {
		t.Fatalf("identity=%+v", result)
	}
	if result.SessionCount != 9 || result.BytesTotal != 1024 || result.DistinctPeers != 3 || len(result.Protocols) != 2 {
		t.Fatalf("summary=%+v", result)
	}
	if watermarks["clickhouse.sessions.query_as_of"] != asOf.Format(time.RFC3339Nano) || watermarks["clickhouse.sessions.max_ts_end"] == "" {
		t.Fatalf("watermarks=%v", watermarks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAssetDetailClickHouseAlertContextUsesArgMaxWithoutFinalAndHardLimit(t *testing.T) {
	reader, mock, closeDB := newAssetDetailClickHouseTestReader(t, 2)
	defer closeDB()
	asOf := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	windowStart := asOf.Add(-24 * time.Hour)
	asset := &config.AssetRecord{AssetID: "asset-2", TenantID: "tenant-b", IPAddress: "198.51.100.9", Revision: 11}
	rows := sqlmock.NewRows([]string{
		"alert_id", "severity", "status", "alert_type", "src_ip", "dst_ip", "src_port", "dst_port", "protocol", "score",
		"evidence_ids_json", "first_seen", "last_seen", "latest_state_version", "latest_event_id",
	})
	for index := 0; index < 3; index++ {
		rows.AddRow(
			"alert-"+string(rune('a'+index)), "high", "open", "exfil", asset.IPAddress, "203.0.113.4",
			int64(44000+index), int64(443), int64(6), float64(88.5), `["evidence-1"]`,
			windowStart.Add(time.Hour).UnixMilli(), asOf.Add(-time.Duration(index+1)*time.Minute).UnixMilli(), int64(index+1), "event-1",
		)
	}
	mock.ExpectQuery(`FROM traffic\.alerts\s+WHERE`).
		WithArgs(asset.TenantID, windowStart.UnixMilli(), asOf.UnixMilli(), asset.IPAddress, asset.IPAddress, 3).
		WillReturnRows(rows)

	result, watermarks, err := reader.ReadAssetAlertContext(context.Background(), asset.TenantID, asset, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Alerts) != 2 || !result.Truncated || result.Alerts[0].AlertID != "alert-a" {
		t.Fatalf("context=%+v", result)
	}
	if result.Alerts[0].EvidenceIDs[0] != "evidence-1" || result.ResolvedIdentity.AssetRevision != 11 {
		t.Fatalf("alert=%+v identity=%+v", result.Alerts[0], result.ResolvedIdentity)
	}
	if watermarks["clickhouse.alerts.query_as_of"] != asOf.Format(time.RFC3339Nano) || watermarks["clickhouse.alerts.max_last_seen"] == "" {
		t.Fatalf("watermarks=%v", watermarks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAssetDetailClickHouseReaderRejectsUnownedOrIdentitylessAssetWithoutQuery(t *testing.T) {
	reader, mock, closeDB := newAssetDetailClickHouseTestReader(t, 2)
	defer closeDB()
	for _, asset := range []*config.AssetRecord{
		{AssetID: "asset-cross", TenantID: "tenant-other", IPAddress: "192.0.2.1"},
		{AssetID: "asset-no-ip", TenantID: "tenant-a"},
	} {
		if _, _, err := reader.ReadAssetObservations(context.Background(), "tenant-a", asset, time.Now()); err == nil {
			t.Fatalf("asset %+v should be rejected", asset)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

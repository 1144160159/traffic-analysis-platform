package persistence

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

func TestAlertWriterRealCanonicalClickHouseTimestampsAndTrace(t *testing.T) {
	host := strings.TrimSpace(os.Getenv("ALERT_PERSISTENCE_EPHEMERAL_CLICKHOUSE_HOST"))
	if host == "" {
		t.Skip("ALERT_PERSISTENCE_EPHEMERAL_CLICKHOUSE_HOST is not set")
	}
	if os.Getenv("ALERT_PERSISTENCE_EPHEMERAL_CLICKHOUSE_SENTINEL") != "ephemeral-only" {
		t.Fatal("ALERT_PERSISTENCE_EPHEMERAL_CLICKHOUSE_SENTINEL must equal ephemeral-only")
	}
	loopback, _, err := net.SplitHostPort(host)
	if err != nil || net.ParseIP(loopback) == nil || !net.ParseIP(loopback).IsLoopback() {
		t.Fatalf("ephemeral ClickHouse must use a numeric loopback address: %q", host)
	}
	client, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts: []string{host}, Database: "traffic",
		Username:     os.Getenv("ALERT_PERSISTENCE_EPHEMERAL_CLICKHOUSE_USER"),
		Password:     os.Getenv("ALERT_PERSISTENCE_EPHEMERAL_CLICKHOUSE_PASSWORD"),
		MaxOpenConns: 2, MaxIdleConns: 1, DialTimeout: 5 * time.Second,
		CompressionLZ4: true, EnableAutoReconnect: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	row, err := client.QueryRow(ctx, `SELECT marker FROM traffic.codex_ephemeral_asset_detail_sentinel LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	var marker string
	if err := row.Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel ClickHouse: marker=%q err=%v", marker, err)
	}

	writer, err := NewClickHouseWriter(client, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Minute)
	makeAlert := func(id string, offset time.Duration) *Alert {
		observed := base.Add(offset)
		return &Alert{
			TenantID: "tenant-alert-writer-g1", AlertID: id, Fingerprint: "fingerprint-" + id,
			CommunityID: "community-" + id, SessionID: "session-" + id,
			SrcIP: "192.0.2.80", DstIP: "203.0.113.80", SrcPort: 44000, DstPort: 443,
			Protocol: 6, AlertType: "trace-reconcile", Labels: []string{"integration"},
			Score: 0.98, Severity: "high", FirstSeen: observed.Add(-time.Second), LastSeen: observed,
			Count: 1, Status: "new", UpdatedTs: observed, ModelVersion: "model-g1",
			StateVersion: 7,
			RuleVersion:  "rule-g1", FeatureSetID: "feature-g1", EvidenceIDs: []string{"evidence-" + id},
			EventID: "event-" + id, TraceID: "0123456789abcdef0123456789abcdef",
		}
	}
	first := makeAlert("single", 0)
	if err := writer.WriteAlert(ctx, first); err != nil {
		t.Fatal(err)
	}
	batch := []*Alert{makeAlert("batch-a", time.Second), makeAlert("batch-b", 2*time.Second)}
	if err := writer.WriteBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}

	rows, err := client.Query(ctx, `
		SELECT alert_id,event_id,trace_id,state_version,first_seen,last_seen,updated_at
		FROM traffic.alerts
		WHERE tenant_id=?
		ORDER BY alert_id`, first.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := map[string]*Alert{first.AlertID: first, batch[0].AlertID: batch[0], batch[1].AlertID: batch[1]}
	count := 0
	for rows.Next() {
		var alertID, eventID, traceID string
		var stateVersion uint64
		var firstSeen, lastSeen, updatedAt int64
		if err := rows.Scan(&alertID, &eventID, &traceID, &stateVersion, &firstSeen, &lastSeen, &updatedAt); err != nil {
			t.Fatal(err)
		}
		expected := found[alertID]
		if expected == nil || eventID != expected.EventID || traceID != expected.TraceID || stateVersion != expected.StateVersion ||
			firstSeen != expected.FirstSeen.UnixMilli() || lastSeen != expected.LastSeen.UnixMilli() ||
			updatedAt != expected.UpdatedTs.UnixMilli() {
			t.Fatalf("unexpected persisted alert id=%q event=%q trace=%q times=%d/%d/%d", alertID, eventID, traceID, firstSeen, lastSeen, updatedAt)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("persisted alerts=%d want=3", count)
	}
}

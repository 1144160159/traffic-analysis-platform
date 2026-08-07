package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/threatintel"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func TestThreatIntelCommandAtomicEphemeralPostgres(t *testing.T) {
	dsn := os.Getenv("THREAT_INTEL_COMMAND_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("THREAT_INTEL_COMMAND_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	tenantID := "sentinel-ti-" + uuid.NewString()
	srv := &server{
		intel: threatintel.NewService(db, zap.NewNop()), auditDB: db,
		threatIntelTopic: "threat.intel.v1", logger: zap.NewNop(),
	}
	defer cleanupThreatIntelCommandFixture(t, db, tenantID)

	create := integrationThreatIntelEntryCommand(tenantID, "idem-ti-create-000000000001", 0, "create")
	first, err := srv.commitThreatIntelCommand(ctx, nil, create)
	if err != nil {
		t.Fatal(err)
	}
	if first.AggregateVersion != 1 || first.Replayed {
		t.Fatalf("unexpected first receipt: %+v", first)
	}
	replay, err := srv.commitThreatIntelCommand(ctx, nil, create)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.EventID != first.EventID || replay.AggregateVersion != 1 {
		t.Fatalf("unexpected replay: %+v", replay)
	}

	update := integrationThreatIntelEntryCommand(tenantID, "idem-ti-update-000000000001", 1, "update")
	updated, err := srv.commitThreatIntelCommand(ctx, nil, update)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AggregateVersion != 2 || updated.Entries[0].Revision != 2 {
		t.Fatalf("unexpected update receipt: %+v", updated)
	}
	collision := update
	collision.Meta.RequestSHA256 = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if _, err := srv.commitThreatIntelCommand(ctx, nil, collision); !errors.Is(err, errThreatIntelIdempotencyConflict) {
		t.Fatalf("expected idempotency collision, got %v", err)
	}
	crossTenant := integrationThreatIntelEntryCommand(tenantID, "idem-ti-cross-0000000000001", 2, "cross")
	crossTenant.Event.TenantID = tenantID + "-other"
	if _, err := srv.commitThreatIntelCommand(ctx, nil, crossTenant); err == nil {
		t.Fatal("expected cross-tenant command rejection")
	}

	now := time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC)
	feed := &threatintel.FeedSource{
		TenantID: tenantID, Name: "sentinel-feed", Enabled: true,
		IntervalSeconds: 300, Entries: updated.Entries, NextRunAt: &now,
		LastStatus: "configured",
	}
	feedCommand := integrationThreatIntelFeedCommand(
		tenantID, "idem-ti-feed-config-00000001", "feed_upsert", feed,
		"threat_intel.feed_configured", "THREAT_INTEL_FEED_CONFIGURED",
	)
	configured, err := srv.commitThreatIntelCommand(ctx, nil, feedCommand)
	if err != nil {
		t.Fatal(err)
	}
	if configured.Feed == nil || configured.Feed.Revision != 1 {
		t.Fatalf("unexpected configured feed: %+v", configured)
	}
	runFeed := *configured.Feed
	runFeed.LastRunAt = &now
	next := now.Add(5 * time.Minute)
	runFeed.NextRunAt = &next
	runFeed.LastStatus = "success"
	runFeed.RunCount = 1
	runEntry := updated.Entries[0]
	runCommand := integrationThreatIntelFeedCommand(
		tenantID, "idem-ti-feed-run-00000000001", "feed_run", &runFeed,
		"threat_intel.feed_source_run", "THREAT_INTEL_FEED_SOURCE_RUN",
	)
	runCommand.Entries = []threatintel.IntelEntry{runEntry}
	runResult, err := srv.commitThreatIntelCommand(ctx, nil, runCommand)
	if err != nil {
		t.Fatal(err)
	}
	if runResult.Feed.Revision != 2 || runResult.Entries[0].Revision != 3 {
		t.Fatalf("unexpected feed run revisions: %+v", runResult)
	}

	assertThreatIntelCommandCount(t, db, "threat_intel_command_history", tenantID, 5)
	assertThreatIntelCommandCount(t, db, "threat_intel_command_requests", tenantID, 4)
	assertThreatIntelCommandCount(t, db, "threat_intel_event_outbox", tenantID, 4)
	assertThreatIntelCommandCount(t, db, "audit_logs", tenantID, 4)
}

func integrationThreatIntelEntryCommand(tenantID, idempotencyKey string, revision int64, marker string) threatIntelCommand {
	occurredAt := time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC)
	entry := threatintel.IntelEntry{
		TenantID: tenantID, Revision: revision, Type: "ip", Value: "203.0.113.77",
		Reputation: threatintel.RepMalicious, Category: "c2", Source: "sentinel",
		Description: marker, LastSeen: occurredAt,
	}
	return threatIntelCommand{
		Entries: []threatintel.IntelEntry{entry},
		Event: threatIntelEvent{
			EventType: "threat_intel.entry_upserted", Version: 1, SchemaVersion: 1,
			TenantID: tenantID, Source: "sentinel", TraceID: "trace-" + marker, OccurredAt: occurredAt,
		},
		Meta: threatIntelCommandMeta{
			ActionID: "action-" + marker, IdempotencyKey: idempotencyKey,
			RequestSHA256: integrationThreatIntelHash(marker), CommandType: "entry_upsert",
			ExpectedRevision: revision, Reason: "sentinel", TraceID: "trace-" + marker,
		},
		Action: "THREAT_INTEL_ENTRY_UPSERTED", ObjectID: "ip:203.0.113.77",
	}
}

func integrationThreatIntelFeedCommand(
	tenantID, idempotencyKey, commandType string,
	feed *threatintel.FeedSource,
	eventType, action string,
) threatIntelCommand {
	return threatIntelCommand{
		Feed: feed,
		Event: threatIntelEvent{
			EventType: eventType, Version: 1, SchemaVersion: 1,
			TenantID: tenantID, Source: feed.Name, TraceID: "trace-" + commandType,
			OccurredAt: time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC),
		},
		Meta: threatIntelCommandMeta{
			ActionID: "action-" + commandType, IdempotencyKey: idempotencyKey,
			RequestSHA256: integrationThreatIntelHash(commandType), CommandType: commandType,
			ExpectedRevision: feed.Revision, Reason: "sentinel", TraceID: "trace-" + commandType,
		},
		Action: action, ObjectID: feed.Name,
	}
}

func integrationThreatIntelHash(marker string) string {
	value := fmt.Sprintf("%064s", marker)
	return strings.ReplaceAll(value, " ", "0")[:64]
}

func assertThreatIntelCommandCount(t *testing.T, db *sql.DB, table, tenantID string, want int) {
	t.Helper()
	var got int
	query := "SELECT count(*) FROM " + table + " WHERE tenant_id=$1" // table is test-owned constant.
	if err := db.QueryRow(query, tenantID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count=%d want=%d", table, got, want)
	}
}

func cleanupThreatIntelCommandFixture(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	for _, statement := range []string{
		"DELETE FROM threat_intel_command_requests WHERE tenant_id=$1",
		"DELETE FROM threat_intel_command_history WHERE tenant_id=$1",
		"DELETE FROM threat_intel_event_outbox WHERE tenant_id=$1",
		"DELETE FROM audit_logs WHERE tenant_id=$1 AND object_type='threat_intel'",
		"DELETE FROM threat_intel_feeds WHERE tenant_id=$1",
		"DELETE FROM threat_intel WHERE tenant_id=$1",
	} {
		if _, err := db.Exec(statement, tenantID); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}
}

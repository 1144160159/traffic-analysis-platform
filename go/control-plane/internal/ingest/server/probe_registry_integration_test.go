package server

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func TestProbeRegistryAtomicPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("PROBE_REGISTRY_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("PROBE_REGISTRY_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var sentinel string
	if err := db.QueryRowContext(ctx,
		"SELECT marker FROM codex_ephemeral_probe_registry_sentinel WHERE marker='ephemeral-only'",
	).Scan(&sentinel); err != nil {
		t.Fatalf("sentinel unavailable: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO tenants(tenant_id,tenant_name,name)
		VALUES ('probe-other','Probe Other','Probe Other')
		ON CONFLICT (tenant_id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}

	registry := NewPostgresProbeRegistry(db)
	const probeID = "probe-registry-integration"
	if err := registry.Register(ctx, "default", probeID, "1.0.0", "build-a", nil); err != nil {
		t.Fatal(err)
	}
	// An exact retry returns the durable result without creating a new revision.
	if err := registry.Register(ctx, "default", probeID, "1.0.0", "build-a", nil); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(ctx, "default", probeID, "1.1.0", "build-b", nil); err != nil {
		t.Fatal(err)
	}
	if err := registry.Heartbeat(ctx, "default", probeID); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(ctx, "probe-other", probeID, "1.1.0", "build-b", nil); err == nil {
		t.Fatal("expected cross-tenant probe rebind rejection")
	}

	var revision int64
	var tenantID, softwareVersion string
	var heartbeatPresent bool
	if err := db.QueryRowContext(ctx, `
		SELECT tenant_id,software_version,revision,last_heartbeat IS NOT NULL
		FROM probes WHERE probe_id=$1`, probeID,
	).Scan(&tenantID, &softwareVersion, &revision, &heartbeatPresent); err != nil {
		t.Fatal(err)
	}
	if tenantID != "default" || softwareVersion != "1.1.0" || revision != 2 || !heartbeatPresent {
		t.Fatalf("unexpected probe authority: tenant=%s version=%s revision=%d heartbeat=%v",
			tenantID, softwareVersion, revision, heartbeatPresent)
	}
	for name, query := range map[string]string{
		"history":  "SELECT count(*) FROM probe_registry_history WHERE tenant_id='default' AND probe_id=$1",
		"audit":    "SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND object_type='probe' AND object_id=$1 AND action='register_probe'",
		"outbox":   "SELECT count(*) FROM probe_registry_outbox WHERE tenant_id='default' AND probe_id=$1 AND status='pending'",
		"requests": "SELECT count(*) FROM probe_registry_requests WHERE tenant_id='default' AND probe_id=$1",
	} {
		var count int
		if err := db.QueryRowContext(ctx, query, probeID).Scan(&count); err != nil {
			t.Fatalf("query %s evidence: %v", name, err)
		}
		if count != 2 {
			t.Fatalf("expected two %s records, got %d", name, count)
		}
	}
}

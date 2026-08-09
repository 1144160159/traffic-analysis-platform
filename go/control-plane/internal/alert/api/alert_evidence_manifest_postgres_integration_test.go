package api

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// TestAlertEvidenceManifestPostgresIntegration writes only to a disposable,
// sentinel-protected PostgreSQL instance created by the owned G1 runner.
func TestAlertEvidenceManifestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ALERT_EVIDENCE_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("ALERT_EVIDENCE_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_alert_evidence_sentinel WHERE marker='ephemeral-only'`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing to run without alert-evidence ephemeral sentinel: marker=%q err=%v", marker, err)
	}

	suffix := uuid.NewString()
	tenantA := "evidence-a-" + suffix
	tenantB := "evidence-b-" + suffix
	alertID := "alert-" + suffix
	evidenceID := "evidence-" + suffix
	digest := strings.Repeat("a", 64)
	store := &postgresAlertEvidenceManifestStore{db: db}
	defer cleanupAlertEvidenceManifestIntegration(t, db, tenantA, tenantB)

	if err := store.VerifySchema(context.Background()); err != nil {
		t.Fatalf("verify schema: %v", err)
	}
	insert := `INSERT INTO alert_evidence_manifests(
		tenant_id,alert_id,evidence_id,event_id,evidence_type,source_store,object_bucket,object_key,
		object_version,object_sha256,size_bytes,content_type,state,revision,source_watermarks,observed_at)
		VALUES($1,$2,$3,$4,'pcap','minio','pcap-archive',$5,'version-1',$6,7,'application/vnd.tcpdump.pcap','available',1,$7::jsonb,now())`
	if _, err := db.Exec(insert, tenantA, alertID, evidenceID, "event-"+suffix, "tenants/"+tenantA+"/pcap/capture-1/"+digest, digest, `{"clickhouse":"partition:0/offset:7"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(insert, tenantB, alertID, evidenceID, "event-other-"+suffix, "tenants/"+tenantB+"/pcap/capture-1/"+digest, digest, `{"clickhouse":"partition:0/offset:8"}`); err != nil {
		t.Fatal(err)
	}

	manifest, err := store.Get(context.Background(), tenantA, alertID, evidenceID)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.TenantID != tenantA || manifest.Revision != 1 || manifest.SourceWatermarks["clickhouse"] != "partition:0/offset:7" {
		t.Fatalf("unexpected tenant-bound manifest: %#v", manifest)
	}
	other, err := store.Get(context.Background(), tenantB, alertID, evidenceID)
	if err != nil || other.TenantID != tenantB || other.EventID == manifest.EventID {
		t.Fatalf("cross-tenant manifest isolation failed: manifest=%#v err=%v", other, err)
	}

	if _, err := db.Exec(`UPDATE alert_evidence_manifests SET state='expired',revision=2,expires_at=now()-interval '1 second' WHERE tenant_id=$1 AND alert_id=$2 AND evidence_id=$3`, tenantA, alertID, evidenceID); err != nil {
		t.Fatalf("monotonic state update: %v", err)
	}
	if _, err := db.Exec(`UPDATE alert_evidence_manifests SET state='available',revision=2 WHERE tenant_id=$1 AND alert_id=$2 AND evidence_id=$3`, tenantA, alertID, evidenceID); err == nil {
		t.Fatal("stale manifest revision unexpectedly committed")
	}
	if _, err := db.Exec(`UPDATE alert_evidence_manifests SET object_key=$4,revision=3 WHERE tenant_id=$1 AND alert_id=$2 AND evidence_id=$3`, tenantA, alertID, evidenceID, "tenants/"+tenantA+"/pcap/changed/"+digest); err == nil {
		t.Fatal("immutable object identity unexpectedly changed")
	}

	var currentCount, historyCount, otherHistoryCount int
	if err := db.QueryRow(`SELECT
		(SELECT count(*) FROM alert_evidence_manifests WHERE tenant_id=$1),
		(SELECT count(*) FROM alert_evidence_manifest_history WHERE tenant_id=$1),
		(SELECT count(*) FROM alert_evidence_manifest_history WHERE tenant_id=$2)`, tenantA, tenantB).Scan(&currentCount, &historyCount, &otherHistoryCount); err != nil {
		t.Fatal(err)
	}
	if currentCount != 1 || historyCount != 2 || otherHistoryCount != 1 {
		t.Fatalf("unexpected current/history facts current=%d history=%d other_history=%d", currentCount, historyCount, otherHistoryCount)
	}
	t.Log("alert_evidence_postgres_manifest=pass")
}

func cleanupAlertEvidenceManifestIntegration(t *testing.T, db *sql.DB, tenantIDs ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, tenantID := range tenantIDs {
		if _, err := db.ExecContext(ctx, `DELETE FROM alert_evidence_manifest_history WHERE tenant_id=$1`, tenantID); err != nil {
			t.Errorf("cleanup alert evidence history for %s: %v", tenantID, err)
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM alert_evidence_manifests WHERE tenant_id=$1`, tenantID); err != nil {
			t.Errorf("cleanup alert evidence manifests for %s: %v", tenantID, err)
		}
	}
}

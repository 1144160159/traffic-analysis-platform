package api

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	alertservice "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestReconcileAlertEvidenceManifestsExposesPartialSources(t *testing.T) {
	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	digest := strings.Repeat("a", 64)
	evidences := []*alertservice.EvidenceDTO{
		{TenantID: "tenant-a", AlertID: "alert-1", EvidenceID: "matched", EventID: "event-1", Type: "pcap"},
		{TenantID: "tenant-a", AlertID: "alert-1", EvidenceID: "manifest-missing", Type: "session"},
	}
	manifests := []AlertEvidenceManifest{
		{TenantID: "tenant-a", AlertID: "alert-1", EvidenceID: "matched", EventID: "event-1", EvidenceType: "pcap", SourceStore: "minio", ObjectBucket: "pcap-archive", ObjectKey: "tenants/tenant-a/pcap/c1/" + digest, ObjectSHA256: digest, SizeBytes: 10, State: "available", Revision: 3},
		{TenantID: "tenant-a", AlertID: "alert-1", EvidenceID: "read-model-missing", EvidenceType: "session", SourceStore: "clickhouse", State: "available", Revision: 4},
	}
	items, partial, missing, watermarks := reconcileAlertEvidenceManifests(evidences, manifests, now)
	if !partial || len(items) != 3 {
		t.Fatalf("unexpected reconciliation partial=%v items=%d", partial, len(items))
	}
	joined := strings.Join(missing, ",")
	for _, expected := range []string{"postgresql.manifest:manifest-missing", "clickhouse.evidence:read-model-missing"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing section %q absent from %v", expected, missing)
		}
	}
	if watermarks["postgresql.alert_evidence_manifests.max_revision"] != "4" || watermarks["clickhouse.evidence.count"] != "2" {
		t.Fatalf("unexpected watermarks: %#v", watermarks)
	}
}

func TestPostgresAlertEvidenceManifestStoreTenantBoundGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	digest := strings.Repeat("a", 64)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT tenant_id,alert_id,evidence_id,event_id,evidence_type,source_store,")).
		WithArgs("tenant-a", "alert-1", "evidence-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "alert_id", "evidence_id", "event_id", "evidence_type", "source_store", "object_bucket", "object_key", "object_version", "object_sha256", "size_bytes", "content_type", "state", "revision", "source_watermarks", "observed_at", "expires_at",
		}).AddRow("tenant-a", "alert-1", "evidence-1", "event-1", "pcap", "minio", "pcap-archive", "tenants/tenant-a/pcap/c1/"+digest, "v1", digest, 7, "application/vnd.tcpdump.pcap", "available", 2, `{"clickhouse":"offset:7"}`, now, nil))
	store := &postgresAlertEvidenceManifestStore{db: db}
	manifest, err := store.Get(context.Background(), "tenant-a", "alert-1", "evidence-1")
	if err != nil {
		t.Fatalf("get manifest: %v", err)
	}
	if manifest.Revision != 2 || manifest.ObjectVersion != "v1" || manifest.SourceWatermarks["clickhouse"] != "offset:7" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

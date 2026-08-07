package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

type fakeAssetEvidenceObjectStore struct {
	objects map[string]AssetEvidenceObjectInfo
	errors  map[string]error
	calls   []string
}

func (s *fakeAssetEvidenceObjectStore) StatObject(_ context.Context, bucket, key string) (AssetEvidenceObjectInfo, error) {
	objectID := bucket + "/" + key
	s.calls = append(s.calls, objectID)
	if err := s.errors[objectID]; err != nil {
		return AssetEvidenceObjectInfo{}, err
	}
	info, ok := s.objects[objectID]
	if !ok {
		return AssetEvidenceObjectInfo{}, errors.New("object not found")
	}
	return info, nil
}

func TestAssetDetailEvidenceReaderReconcilesClickHouseReferencesAndMinIOMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	asOf := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	store := &fakeAssetEvidenceObjectStore{objects: map[string]AssetEvidenceObjectInfo{
		"evidence-bucket/tenant-a/evidence-a.pcap": {
			Size: 4096, ContentType: "application/vnd.tcpdump.pcap", ETag: "etag-a", VersionID: "version-a",
			LastModified: asOf.Add(-time.Minute), UserMetadata: map[string]string{"sha256": sha},
		},
	}, errors: map[string]error{}}
	reader, err := NewAssetDetailEvidenceReader(db, store, time.Second, 10)
	if err != nil {
		t.Fatal(err)
	}
	asset := &config.AssetRecord{AssetID: "asset-1", TenantID: "tenant-a", Revision: 5}
	alerts := &config.AssetAlertContext{AssetID: "asset-1", Alerts: []config.AssetAlertSummary{{
		AlertID: "alert-1", EvidenceIDs: []string{"evidence-b", "evidence-a", "evidence-a"},
	}}}
	mock.ExpectQuery(regexp.QuoteMeta("FROM traffic.evidence")).
		WithArgs("evidence-a", "evidence-b", "tenant-a", asOf.UnixMilli()).
		WillReturnRows(sqlmock.NewRows([]string{
			"evidence_id", "latest_alert_id", "latest_evidence_ts", "latest_evidence_type", "latest_summary", "latest_metrics_json", "latest_snippet_ref_json",
		}).
			AddRow("evidence-a", "alert-1", asOf.Add(-time.Hour).UnixMilli(), "pcap", "packet capture", `{}`, `{"bucket":"evidence-bucket","object":"tenant-a/evidence-a.pcap"}`).
			AddRow("evidence-b", "alert-1", asOf.Add(-2*time.Hour).UnixMilli(), "session", "session metadata", `{}`, `{"session_link":"/arkime/sessions/1"}`))

	result, marks, complete, err := reader.ReadAssetEvidenceObjects(context.Background(), "tenant-a", asset, asOf, alerts)
	if err != nil {
		t.Fatal(err)
	}
	if complete || !result.Partial || len(result.Objects) != 1 || len(result.MissingEvidenceIDs) != 1 || result.MissingEvidenceIDs[0] != "evidence-b" {
		t.Fatalf("result=%+v complete=%v", result, complete)
	}
	manifest := result.Objects[0]
	if manifest.SHA256 != sha || manifest.IntegrityStatus != "verified_metadata" || manifest.SizeBytes != 4096 || manifest.ObjectVersion != "version-a" {
		t.Fatalf("manifest=%+v", manifest)
	}
	if marks["clickhouse.evidence.query_as_of"] != asOf.Format(time.RFC3339Nano) || marks["clickhouse.evidence.max_ts"] == "" || marks["minio.evidence.max_last_modified"] == "" {
		t.Fatalf("marks=%v", marks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAssetDetailEvidenceReaderTreatsMissingSHAAsUnverifiedPartial(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	asOf := time.Now().UTC()
	store := &fakeAssetEvidenceObjectStore{objects: map[string]AssetEvidenceObjectInfo{
		"bucket/key": {Size: 1, LastModified: asOf.Add(-time.Second)},
	}, errors: map[string]error{}}
	reader, err := NewAssetDetailEvidenceReader(db, store, time.Second, 1)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta("FROM traffic.evidence")).
		WithArgs("evidence-1", "tenant-a", asOf.UnixMilli()).
		WillReturnRows(sqlmock.NewRows([]string{
			"evidence_id", "latest_alert_id", "latest_evidence_ts", "latest_evidence_type", "latest_summary", "latest_metrics_json", "latest_snippet_ref_json",
		}).AddRow("evidence-1", "alert-1", asOf.Add(-time.Minute).UnixMilli(), "pcap", "pcap", `{}`, `{"bucket":"bucket","object":"key"}`))
	result, _, complete, err := reader.ReadAssetEvidenceObjects(context.Background(), "tenant-a",
		&config.AssetRecord{AssetID: "asset-1", TenantID: "tenant-a"}, asOf,
		&config.AssetAlertContext{AssetID: "asset-1", Alerts: []config.AssetAlertSummary{{EvidenceIDs: []string{"evidence-1"}}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if complete || !result.Partial || result.Objects[0].IntegrityStatus != "unverified_missing_sha256" || len(result.MissingEvidenceIDs) != 1 {
		t.Fatalf("result=%+v complete=%v", result, complete)
	}
}

func TestAssetDetailEvidenceReaderClosesEmptyReconciledSetWithoutQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reader, err := NewAssetDetailEvidenceReader(db, &fakeAssetEvidenceObjectStore{}, time.Second, 10)
	if err != nil {
		t.Fatal(err)
	}
	result, _, complete, err := reader.ReadAssetEvidenceObjects(context.Background(), "tenant-a",
		&config.AssetRecord{AssetID: "asset-1", TenantID: "tenant-a"}, time.Now(),
		&config.AssetAlertContext{AssetID: "asset-1", Alerts: []config.AssetAlertSummary{}},
	)
	if err != nil || !complete || result.Partial || len(result.Objects) != 0 {
		t.Fatalf("result=%+v complete=%v err=%v", result, complete, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

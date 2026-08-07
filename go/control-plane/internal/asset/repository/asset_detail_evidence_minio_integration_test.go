package repository

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func TestAssetDetailEvidenceReaderRealEphemeralMinIOMetadata(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("ASSET_DETAIL_EPHEMERAL_MINIO_ENDPOINT"))
	accessKey := os.Getenv("ASSET_DETAIL_EPHEMERAL_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("ASSET_DETAIL_EPHEMERAL_MINIO_SECRET_KEY")
	bucket := strings.TrimSpace(os.Getenv("ASSET_DETAIL_EPHEMERAL_MINIO_BUCKET"))
	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("explicit ephemeral MinIO settings are required")
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	objectKey := "tenant-a/evidence-real.pcap"
	defer func() {
		if err := client.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
			t.Errorf("remove object: %v", err)
		}
		if err := client.RemoveBucket(ctx, bucket); err != nil {
			t.Errorf("remove bucket: %v", err)
		}
	}()
	content := []byte("ephemeral-pcap-object")
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := client.PutObject(ctx, bucket, objectKey, bytes.NewReader(content), int64(len(content)), minio.PutObjectOptions{
		ContentType: "application/vnd.tcpdump.pcap", UserMetadata: map[string]string{"sha256": sha},
	}); err != nil {
		t.Fatal(err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	asOf := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("FROM traffic.evidence")).
		WithArgs("evidence-real", "tenant-a", asOf.UnixMilli()).
		WillReturnRows(sqlmock.NewRows([]string{
			"evidence_id", "latest_alert_id", "latest_evidence_ts", "latest_evidence_type", "latest_summary", "latest_metrics_json", "latest_snippet_ref_json",
		}).AddRow("evidence-real", "alert-real", asOf.Add(-time.Minute).UnixMilli(), "pcap", "real object metadata", `{}`, `{"bucket":"`+bucket+`","object":"`+objectKey+`"}`))
	objectStore, err := NewMinIOAssetEvidenceObjectStore(client)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewAssetDetailEvidenceReader(db, objectStore, 5*time.Second, 10)
	if err != nil {
		t.Fatal(err)
	}
	result, marks, complete, err := reader.ReadAssetEvidenceObjects(ctx, "tenant-a",
		&config.AssetRecord{AssetID: "asset-real", TenantID: "tenant-a", Revision: 1}, asOf,
		&config.AssetAlertContext{AssetID: "asset-real", Alerts: []config.AssetAlertSummary{{AlertID: "alert-real", EvidenceIDs: []string{"evidence-real"}}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !complete || result.Partial || len(result.Objects) != 1 {
		t.Fatalf("result=%+v complete=%v", result, complete)
	}
	manifest := result.Objects[0]
	if manifest.SHA256 != sha || manifest.SizeBytes != int64(len(content)) || manifest.ContentType != "application/vnd.tcpdump.pcap" || manifest.IntegrityStatus != "verified_metadata" {
		t.Fatalf("manifest=%+v", manifest)
	}
	if marks["minio.evidence.max_last_modified"] == "" {
		t.Fatalf("marks=%v", marks)
	}
}

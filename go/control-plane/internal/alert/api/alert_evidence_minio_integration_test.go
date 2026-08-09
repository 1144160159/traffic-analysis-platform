package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// TestAlertEvidenceMinIOIntegrityIntegration mutates only the loopback MinIO
// created by scripts/alignment/verify_alert_evidence_ephemeral.py.
func TestAlertEvidenceMinIOIntegrityIntegration(t *testing.T) {
	if os.Getenv("ALERT_EVIDENCE_EPHEMERAL_MINIO") != "ephemeral-only" {
		t.Skip("ALERT_EVIDENCE_EPHEMERAL_MINIO is not set to ephemeral-only")
	}
	endpoint := strings.TrimSpace(os.Getenv("S3_ENDPOINT"))
	if !strings.HasPrefix(endpoint, "127.0.0.1:") && !strings.HasPrefix(endpoint, "localhost:") {
		t.Fatalf("refusing non-loopback MinIO endpoint %q", endpoint)
	}
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bucket := "codex-alert-evidence-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.RemoveBucket(context.Background(), bucket); err != nil {
			t.Errorf("remove ephemeral evidence bucket: %v", err)
		}
	}()
	if err := client.EnableVersioning(ctx, bucket); err != nil {
		t.Fatal(err)
	}

	data := []byte("owned-alert-evidence-pcap-v1")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	key := "tenants/tenant-a/pcap/capture-1/" + digest
	upload, err := client.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType:  "application/vnd.tcpdump.pcap",
		UserMetadata: map[string]string{"sha256": digest},
	})
	if err != nil {
		t.Fatal(err)
	}
	if upload.VersionID == "" {
		t.Fatal("versioned evidence upload returned no version id")
	}
	defer func() {
		if err := client.RemoveObject(context.Background(), bucket, key, minio.RemoveObjectOptions{VersionID: upload.VersionID}); err != nil {
			t.Errorf("remove ephemeral evidence object version: %v", err)
		}
	}()

	store := &minioAlertEvidenceObjectStore{client: client}
	info, err := store.Stat(ctx, bucket, key, upload.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	manifest := &AlertEvidenceManifest{ObjectVersion: upload.VersionID, ObjectSHA256: digest, SizeBytes: int64(len(data))}
	ref := alertEvidenceObjectReference{Bucket: bucket, Key: key, VersionID: upload.VersionID}
	if err := verifyAlertEvidenceObjectIntegrity(ctx, store, ref, info, manifest); err != nil {
		t.Fatalf("real MinIO version/checksum validation failed: %v", err)
	}

	manifest.ObjectSHA256 = strings.Repeat("b", 64)
	if err := verifyAlertEvidenceObjectIntegrity(ctx, store, ref, info, manifest); err == nil {
		t.Fatal("real MinIO checksum mismatch unexpectedly passed")
	}
	t.Logf("alert_evidence_minio_integrity=pass version_id=%s sha256=%s", upload.VersionID, digest)
}

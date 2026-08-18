package s3client

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"go.uber.org/zap"
)

func TestRestorationObjectAuthorityRoundTrip(t *testing.T) {
	if os.Getenv("M03_RESTORATION_MINIO_INTEGRATION_ENABLED") != "true" {
		t.Skip("owned ephemeral MinIO is not enabled")
	}
	if os.Getenv("M03_RESTORATION_MINIO_SENTINEL") != "codex_ephemeral_m03_restoration_minio" {
		t.Fatal("refusing a MinIO instance that is not explicitly owned by this test")
	}
	endpoint := os.Getenv("M03_RESTORATION_MINIO_ENDPOINT")
	accessKey := os.Getenv("M03_RESTORATION_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("M03_RESTORATION_MINIO_SECRET_KEY")
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Fatal("owned ephemeral MinIO endpoint and credentials are required")
	}
	client, err := NewS3Client(endpoint, accessKey, secretKey, "pcap-archive", false, "", "forensics-quarantine", zap.NewNop())
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.VerifyRestorationBucket(ctx, "forensics-quarantine"); err != nil {
		t.Fatalf("verify governed quarantine bucket: %v", err)
	}

	tenantID := "tenant-minio-integration"
	// Use a fresh identity on every invocation because prior versions are under
	// governance retention and deliberately cannot be removed by this test.
	restorationID := "integration-" + time.Now().UTC().Format("20060102T150405.000000000")
	key := "tenants/" + tenantID + "/restorations/" + restorationID + "/1/content.bin"
	content := []byte("inert restoration bytes\x00not executable")
	digest := sha256Bytes(content)
	retentionUntil := time.Now().UTC().Add(2 * time.Hour)
	authority, err := client.PutQuarantineObject(ctx, "forensics-quarantine", key, tenantID, restorationID,
		content, "application/octet-stream", digest, retentionUntil)
	if err != nil {
		t.Fatalf("put quarantine object: %v", err)
	}
	if authority.VersionID == "" || authority.SHA256 != digest || authority.SizeBytes != int64(len(content)) ||
		!authority.RetentionUntil.After(time.Now().UTC()) {
		t.Fatalf("invalid quarantine authority: %#v", authority)
	}
	recovered, found, err := client.FindQuarantineObject(ctx, "forensics-quarantine", key, tenantID,
		restorationID, digest, int64(len(content)))
	if err != nil || !found || recovered.VersionID != authority.VersionID || recovered.SHA256 != digest {
		t.Fatalf("recover exact quarantine authority = %#v, %v, %v", recovered, found, err)
	}

	_, err = client.PutQuarantineObject(ctx, "forensics-quarantine", key, tenantID, restorationID,
		[]byte("different bytes"), "application/octet-stream", sha256Bytes([]byte("different bytes")), retentionUntil)
	if err == nil {
		t.Fatal("write-once quarantine key accepted a second object version")
	}
	if _, found, err := client.FindQuarantineObject(ctx, "forensics-quarantine", key, tenantID,
		restorationID, strings.Repeat("f", 64), int64(len(content))); !errors.Is(err, ErrQuarantineObjectConflict) || found {
		t.Fatalf("conflicting quarantine authority = found %v, error %v", found, err)
	}

	source := []byte("immutable pcap source bytes")
	sourceSHA := sha256Bytes(source)
	put, err := client.client.PutObject(ctx, "pcap-archive", "tenant/source.pcap", strings.NewReader(string(source)), int64(len(source)), minio.PutObjectOptions{})
	if err != nil {
		t.Fatalf("put source object: %v", err)
	}
	loaded, receipt, err := client.ReadVerifiedObject(ctx, "pcap-archive", "tenant/source.pcap", put.VersionID,
		put.ETag, sourceSHA, int64(len(source)), int64(len(source)))
	if err != nil || string(loaded) != string(source) || receipt.VersionID != put.VersionID {
		t.Fatalf("read immutable source = %q, %#v, %v", loaded, receipt, err)
	}
	if _, _, err := client.ReadVerifiedObject(ctx, "pcap-archive", "tenant/source.pcap", put.VersionID,
		put.ETag, strings.Repeat("0", 64), int64(len(source)), int64(len(source))); err == nil {
		t.Fatal("source SHA mismatch was accepted")
	}

	taskID := "task-" + time.Now().UTC().Format("20060102T150405.000000000")
	resultKey := "tenants/" + tenantID + "/forensics/jobs/" + taskID + "/pcap/result.pcap"
	resultBytes := []byte("inert versioned PCAP result bytes")
	resultSHA := sha256Bytes(resultBytes)
	resultAuthority, err := client.PutForensicsResultObject(ctx, resultKey, tenantID, taskID,
		strings.NewReader(string(resultBytes)), int64(len(resultBytes)), resultSHA, retentionUntil)
	if err != nil {
		t.Fatalf("put immutable forensics result: %v", err)
	}
	if resultAuthority.VersionID == "" || resultAuthority.SHA256 != resultSHA || resultAuthority.SizeBytes != int64(len(resultBytes)) {
		t.Fatalf("invalid forensics result authority: %#v", resultAuthority)
	}
	recoveredResult, found, err := client.FindForensicsResultObject(ctx, resultKey, tenantID, taskID, resultSHA, int64(len(resultBytes)))
	if err != nil || !found || recoveredResult.VersionID != resultAuthority.VersionID {
		t.Fatalf("recover immutable forensics result = %#v found=%v err=%v", recoveredResult, found, err)
	}
	if _, _, err := client.FindForensicsResultObject(ctx, resultKey, "tenant-other", taskID, resultSHA, int64(len(resultBytes))); err == nil {
		t.Fatal("cross-tenant result lookup was accepted")
	}
	if _, err := client.PutForensicsResultObject(ctx, resultKey, tenantID, taskID,
		strings.NewReader("different"), int64(len("different")), sha256Bytes([]byte("different")), retentionUntil); err == nil {
		t.Fatal("write-once forensics result key accepted a duplicate version")
	}
	opened, verifiedResult, err := client.OpenForensicsResultObject(ctx, resultAuthority, tenantID, taskID)
	if err != nil {
		t.Fatalf("open exact forensics result version: %v", err)
	}
	openedBytes, readErr := io.ReadAll(opened)
	closeErr := opened.Close()
	if readErr != nil || closeErr != nil || string(openedBytes) != string(resultBytes) || verifiedResult.VersionID != resultAuthority.VersionID {
		t.Fatalf("exact forensics result read bytes=%q receipt=%#v read=%v close=%v", openedBytes, verifiedResult, readErr, closeErr)
	}
	signed, signedAuthority, err := client.PresignForensicsResultObject(ctx, resultAuthority, tenantID, taskID, 5*time.Minute)
	if err != nil {
		t.Fatalf("presign exact forensics result version: %v", err)
	}
	signedURL, err := url.Parse(signed)
	if err != nil || signedURL.Query().Get("versionId") != resultAuthority.VersionID || signedAuthority.VersionID != resultAuthority.VersionID {
		t.Fatalf("presign is not version-bound url=%q authority=%#v err=%v", signed, signedAuthority, err)
	}
	tampered := resultAuthority
	tampered.VersionID = "wrong-version"
	if _, err := client.VerifyForensicsResultAuthority(ctx, tampered, tenantID, taskID); err == nil {
		t.Fatal("tampered result object version was accepted")
	}
}

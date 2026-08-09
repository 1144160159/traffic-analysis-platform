package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	alertservice "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
)

type fakeAlertEvidenceObjectStore struct {
	data []byte
	info alertEvidenceObjectInfo
}

func (s *fakeAlertEvidenceObjectStore) Stat(context.Context, string, string, string) (alertEvidenceObjectInfo, error) {
	return s.info, nil
}

func (s *fakeAlertEvidenceObjectStore) Open(context.Context, string, string, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.data)), nil
}

func TestAlertEvidenceDownloadSignatureBindsTenantAlertEvidenceAndExpiry(t *testing.T) {
	const secret = "test-secret"
	signature := signAlertEvidenceDownload(secret, "tenant-a", "alert-1", "evidence-1", 12345)
	if signature == "" {
		t.Fatal("expected signature")
	}
	for _, changed := range []string{
		signAlertEvidenceDownload(secret, "tenant-b", "alert-1", "evidence-1", 12345),
		signAlertEvidenceDownload(secret, "tenant-a", "alert-2", "evidence-1", 12345),
		signAlertEvidenceDownload(secret, "tenant-a", "alert-1", "evidence-2", 12345),
		signAlertEvidenceDownload(secret, "tenant-a", "alert-1", "evidence-1", 12346),
	} {
		if signature == changed {
			t.Fatal("signature must change when a bound field changes")
		}
	}
}

func TestAlertEvidenceDownloadV2SignatureBindsManifestRevisionAndDigest(t *testing.T) {
	const secret = "test-secret"
	digest := strings.Repeat("a", 64)
	signature := signAlertEvidenceDownloadV2(secret, "tenant-a", "alert-1", "evidence-1", 12345, 7, digest)
	for _, changed := range []string{
		signAlertEvidenceDownloadV2(secret, "tenant-a", "alert-1", "evidence-1", 12345, 8, digest),
		signAlertEvidenceDownloadV2(secret, "tenant-a", "alert-1", "evidence-1", 12345, 7, strings.Repeat("b", 64)),
	} {
		if signature == changed {
			t.Fatal("v2 signature must change with manifest revision or object digest")
		}
	}
}

func TestStrictAlertEvidenceSigningDoesNotReuseJWTSecret(t *testing.T) {
	t.Setenv("ALERT_EVIDENCE_DOWNLOAD_SECRET", "")
	t.Setenv("JWT_SECRET_KEY", "jwt-secret-must-not-sign-evidence")
	handler := NewHandler(nil, nil, nil)
	if secret := handler.alertEvidenceSigningSecret(); secret != "jwt-secret-must-not-sign-evidence" {
		t.Fatalf("legacy compatibility secret=%q", secret)
	}
	handler.SetAlertEvidenceChainEnabled(true)
	if secret := handler.alertEvidenceSigningSecret(); secret != "" {
		t.Fatalf("strict evidence path reused JWT secret: %q", secret)
	}
	t.Setenv("ALERT_EVIDENCE_DOWNLOAD_SECRET", "dedicated-evidence-secret")
	if secret := handler.alertEvidenceSigningSecret(); secret != "dedicated-evidence-secret" {
		t.Fatalf("strict evidence path did not use dedicated secret: %q", secret)
	}
}

func TestValidateAlertEvidenceManifestFailsClosed(t *testing.T) {
	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	evidence := &alertservice.EvidenceDTO{TenantID: "tenant-a", AlertID: "alert-1", EvidenceID: "evidence-1", EventID: "event-1", Type: "pcap"}
	digest := strings.Repeat("a", 64)
	manifest := &AlertEvidenceManifest{
		TenantID: "tenant-a", AlertID: "alert-1", EvidenceID: "evidence-1", EventID: "event-1", EvidenceType: "pcap",
		SourceStore: "minio", ObjectBucket: "pcap-archive", ObjectKey: "tenants/tenant-a/pcap/capture-1/" + digest,
		ObjectSHA256: digest, SizeBytes: 7, State: "available", Revision: 1,
	}
	if err := validateAlertEvidenceManifest(manifest, evidence, now); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	changed := *manifest
	changed.ObjectKey = "tenants/tenant-b/pcap/capture-1/" + digest
	if err := validateAlertEvidenceManifest(&changed, evidence, now); err == nil {
		t.Fatal("cross-tenant object prefix must be rejected")
	}
	expired := now.Add(-time.Second)
	changed = *manifest
	changed.ExpiresAt = &expired
	if err := validateAlertEvidenceManifest(&changed, evidence, now); err != errEvidenceManifestExpired {
		t.Fatalf("expired manifest error=%v", err)
	}
}

func TestVerifyAlertEvidenceObjectIntegrityHashesWhenMetadataMissing(t *testing.T) {
	data := []byte("pcap-v1")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	manifest := &AlertEvidenceManifest{ObjectSHA256: digest, SizeBytes: int64(len(data))}
	store := &fakeAlertEvidenceObjectStore{data: data, info: alertEvidenceObjectInfo{Size: int64(len(data))}}
	ref := alertEvidenceObjectReference{Bucket: "pcap-archive", Key: "tenants/tenant-a/pcap/capture-1/" + digest}
	if err := verifyAlertEvidenceObjectIntegrity(context.Background(), store, ref, store.info, manifest); err != nil {
		t.Fatalf("valid streamed checksum rejected: %v", err)
	}
	manifest.ObjectSHA256 = strings.Repeat("b", 64)
	if err := verifyAlertEvidenceObjectIntegrity(context.Background(), store, ref, store.info, manifest); err == nil {
		t.Fatal("checksum mismatch must fail closed")
	}
}

func TestNormalizeObjectSHA256AcceptsMinIOBase64Checksum(t *testing.T) {
	digest := sha256.Sum256([]byte("evidence"))
	encoded := base64.StdEncoding.EncodeToString(digest[:])
	if actual := normalizeObjectSHA256(encoded); actual != hex.EncodeToString(digest[:]) {
		t.Fatalf("normalized digest=%q", actual)
	}
}

func TestAlertEvidenceAliasMatchesExactIdentifiersAndObjectNames(t *testing.T) {
	tests := []struct {
		evidence *alertservice.EvidenceDTO
		request  string
	}{
		{evidence: &alertservice.EvidenceDTO{EvidenceID: "alert-detail-r802-pcap", Type: "PCAP"}, request: "alert-detail-r802-pcap"},
		{evidence: &alertservice.EvidenceDTO{EvidenceID: "ev-1", Type: "附件", SnippetRef: map[string]string{"object": "alerts/r802/report.zip"}}, request: "report.zip"},
	}
	for _, item := range tests {
		if !alertEvidenceAliasMatches(item.evidence, item.request) {
			t.Fatalf("expected %q to match evidence %q (%s)", item.request, item.evidence.EvidenceID, item.evidence.Type)
		}
	}
}

func TestAlertEvidenceAliasDoesNotAcceptTypeOnlyFileNames(t *testing.T) {
	pcap := &alertservice.EvidenceDTO{EvidenceID: "alert-detail-r802-pcap", Type: "PCAP"}
	if alertEvidenceAliasMatches(pcap, "AL-20260620-000123.pcap") {
		t.Fatal("type-only filenames must not be accepted as exact aliases")
	}
}

func TestAlertEvidenceOriginalObjectUsesMinioReference(t *testing.T) {
	evidence := &alertservice.EvidenceDTO{
		EvidenceID: "alert-detail-r802-pcap",
		Metrics: map[string]interface{}{
			"object_path": "minio://traffic-evidence/alerts/r802/c2-tunnel.pcap",
		},
		SnippetRef: map[string]string{
			"bucket": "traffic-evidence",
			"object": "alerts/r802/c2-tunnel.pcap",
		},
	}
	ref, err := alertEvidenceOriginalObject(evidence)
	if err != nil {
		t.Fatalf("unexpected object reference error: %v", err)
	}
	if ref.Bucket != "traffic-evidence" || ref.Key != "alerts/r802/c2-tunnel.pcap" {
		t.Fatalf("unexpected object reference: %#v", ref)
	}
	if ref.FileName != "c2-tunnel.pcap" || ref.ContentType != "application/vnd.tcpdump.pcap" {
		t.Fatalf("unexpected original file metadata: %#v", ref)
	}
}

func TestEvidenceDownloadFileNameIsSafe(t *testing.T) {
	tests := map[string]string{
		"capture.pcap":     "capture.pcap",
		"../tenant/secret": "tenant-secret.json",
		"证据 01":            "01.json",
		"":                 "evidence.json",
	}
	for input, expected := range tests {
		if actual := evidenceDownloadFileName(input); actual != expected {
			t.Fatalf("evidenceDownloadFileName(%q)=%q, want %q", input, actual, expected)
		}
	}
}

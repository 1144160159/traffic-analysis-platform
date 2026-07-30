package api

import (
	"testing"

	alertservice "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
)

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

package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func evidenceInt64Pointer(value int64) *int64 { return &value }

func validEvidenceLinkRequestForTest() alertEvidenceLinkRequest {
	return alertEvidenceLinkRequest{
		ExpectedRevision: evidenceInt64Pointer(0), ExpectedManifestRevision: evidenceInt64Pointer(3),
		SourceStore: "minio", ObjectBucket: "evidence",
		ObjectKey: "tenants/tenant-a/evidence/a.pcap", ObjectVersion: "version-1",
		ObjectSHA256: strings.Repeat("a", 64), Reason: "incident investigation",
	}
}

func TestAlertEvidenceLinkRequestRequiresExactObjectIdentity(t *testing.T) {
	request := validEvidenceLinkRequestForTest()
	if !validAlertEvidenceLinkRequest(request) {
		t.Fatal("valid immutable object request was rejected")
	}
	request.ObjectVersion = ""
	if validAlertEvidenceLinkRequest(request) {
		t.Fatal("missing object version must fail closed")
	}
	request = validEvidenceLinkRequestForTest()
	request.ObjectSHA256 = strings.Repeat("A", 64)
	if validAlertEvidenceLinkRequest(request) {
		t.Fatal("non-canonical digest must fail closed")
	}
}

func TestAlertEvidenceLinkRequestSHABindsOperationRevisionAndDigest(t *testing.T) {
	request := validEvidenceLinkRequestForTest()
	digest, err := alertEvidenceLinkRequestSHA(alertEvidenceLink, "tenant-a", "alert-a", "evidence-a", request)
	if err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.ObjectSHA256 = strings.Repeat("b", 64)
	changedDigest, err := alertEvidenceLinkRequestSHA(alertEvidenceLink, "tenant-a", "alert-a", "evidence-a", changed)
	if err != nil {
		t.Fatal(err)
	}
	unlinkDigest, err := alertEvidenceLinkRequestSHA(alertEvidenceUnlink, "tenant-a", "alert-a", "evidence-a", request)
	if err != nil {
		t.Fatal(err)
	}
	if digest == changedDigest || digest == unlinkDigest {
		t.Fatal("request identity must bind digest and operation")
	}
}

func TestValidateAlertEvidenceLinkOutboxItemRejectsIdentityDrift(t *testing.T) {
	item := alertEvidenceLinkOutboxItem{
		EventID: "11111111-1111-4111-8111-111111111111", TenantID: "tenant-a",
		AggregateID: "22222222-2222-4222-8222-222222222222", AggregateVersion: 2,
		EventType: "traffic.alert-evidence.v1.Linked", PartitionKey: "tenant-a:alert-a", SchemaVersion: 1,
	}
	payload := alertEvidenceLinkEnvelope{
		EventID: item.EventID, EventType: item.EventType, TenantID: item.TenantID,
		AggregateType: "alert_evidence_link", AggregateID: item.AggregateID,
		AggregateVersion: item.AggregateVersion, PartitionKey: item.PartitionKey,
		SchemaVersion: 1, AlertID: "alert-a", EvidenceID: "evidence-a",
		ObjectVersion: "version-1", ObjectSHA256: strings.Repeat("a", 64), TraceID: "trace-1",
	}
	item.Payload, _ = json.Marshal(payload)
	if err := validateAlertEvidenceLinkOutboxItem(&item); err != nil {
		t.Fatalf("valid outbox item rejected: %v", err)
	}
	item.AggregateVersion++
	if err := validateAlertEvidenceLinkOutboxItem(&item); err == nil {
		t.Fatal("aggregate version drift must fail closed")
	}
}

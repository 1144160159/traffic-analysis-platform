package task

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCutTaskRequestFreezesCanonicalCommandContext(t *testing.T) {
	req := &CutTaskRequest{
		TenantID: " tenant-a ", UserID: " analyst-a ",
		ProbeID: "probe-b", ProbeIDs: []string{"probe-a", "probe-b", " probe-a "},
		AlertID: "alert-b", AlertIDs: []string{"alert-a", "alert-b"},
		CaseIDs: []string{"case-b", " case-a ", "case-b"},
		SrcIP:   "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 1234, DstPort: 443, Protocol: 6,
		StartTime: 1, EndTime: 2, Purpose: " incident-response ",
		PermissionSnapshot: []string{"pcap:read", "pcap:write", "pcap:read"},
		RetentionPolicy:    " legal-review ", RestorationContractVersion: 1,
		TraceID: "trace-a",
	}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	if req.TenantID != "tenant-a" || req.UserID != "analyst-a" || req.Purpose != "incident-response" || req.RetentionPolicy != "legal-review" {
		t.Fatalf("scalar command context was not normalized: %+v", req)
	}
	if !reflect.DeepEqual(req.ProbeIDs, []string{"probe-a", "probe-b"}) ||
		!reflect.DeepEqual(req.AlertIDs, []string{"alert-a", "alert-b"}) ||
		!reflect.DeepEqual(req.CaseIDs, []string{"case-a", "case-b"}) ||
		!reflect.DeepEqual(req.PermissionSnapshot, []string{"pcap:read", "pcap:write"}) {
		t.Fatalf("frozen sets were not canonical: %+v", req)
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"\"probe_ids\"", "\"alert_ids\"", "\"case_ids\"", "\"permission_snapshot\"", "\"purpose\"", "\"retention_policy\"", "\"restoration_contract_version\"", "\"trace_id\""} {
		if !containsJSONMarker(encoded, marker) {
			t.Fatalf("frozen task params missing %s: %s", marker, encoded)
		}
	}
}

func TestCutTaskRequestRejectsUnprivilegedVersionedCommand(t *testing.T) {
	req := &CutTaskRequest{
		TenantID: "tenant-a", StartTime: 1, EndTime: 2, Purpose: "investigate",
		RetentionPolicy: "standard", RestorationContractVersion: 1,
		PermissionSnapshot: []string{"pcap:read"},
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected versioned task without pcap:write permission to fail")
	}
}

func containsJSONMarker(encoded []byte, marker string) bool {
	for index := 0; index+len(marker) <= len(encoded); index++ {
		if string(encoded[index:index+len(marker)]) == marker {
			return true
		}
	}
	return false
}

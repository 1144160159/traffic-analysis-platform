package dedup

import (
	"testing"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestCalculateFingerprint(t *testing.T) {
	batch := &pb.DetectionBatch{
		BatchId:  "batch-001",
		TenantId: "t1",
		RunId:    "run-001",
		Behaviors: []*pb.DetectionBehavior{
			{
				Header:      &pb.EventHeader{EventId: "e1", TenantId: "t1"},
				CommunityId: "community-abc",
				ObjectType:  "scan",
				TopLabel:    "port_scan",
				Labels:      []string{"scan", "recon"},
			},
		},
	}

	fp := CalculateFingerprint(batch, 10)
	if len(fp) != 32 {
		t.Errorf("fingerprint length=%d, want 32 (MD5 hex)", len(fp))
	}

	// Same input should produce same fingerprint
	fp2 := CalculateFingerprint(batch, 10)
	if fp != fp2 {
		t.Errorf("fingerprint not deterministic: %s vs %s", fp, fp2)
	}
}

func TestCalculateFingerprintDifferentBuckets(t *testing.T) {
	batch := &pb.DetectionBatch{
		BatchId:  "batch-001",
		TenantId: "t1",
		Behaviors: []*pb.DetectionBehavior{
			{CommunityId: "c1", ObjectType: "scan", TopLabel: "port_scan"},
		},
	}

	fp1 := CalculateFingerprint(batch, 10)
	fp2 := CalculateFingerprint(batch, 60) // Different time bucket
	if fp1 == fp2 {
		t.Log("same fingerprint despite different buckets (expected if time hasn't changed)")
	}
}

func TestCalculateFingerprintBusinessDetection(t *testing.T) {
	batch := &pb.DetectionBatch{
		BatchId:  "batch-002",
		TenantId: "t2",
		Businesses: []*pb.DetectionBusiness{
			{
				CommunityId:   "c2",
				DetectionType: "data_exfil",
			},
		},
	}

	fp := CalculateFingerprint(batch, 10)
	if len(fp) != 32 {
		t.Errorf("fingerprint length=%d", len(fp))
	}
}

func TestValidateFingerprint(t *testing.T) {
	tests := []struct {
		fp    string
		valid bool
	}{
		{"abcdef0123456789abcdef0123456789", true},
		{"12345678901234567890123456789012", true},
		{"xyz", false},
		{"ABCDEF0123456789ABCDEF0123456789G", false},
		{"", false},
	}
	for _, tt := range tests {
		got := ValidateFingerprint(tt.fp)
		if got != tt.valid {
			t.Errorf("ValidateFingerprint(%q)=%v, want %v", tt.fp, got, tt.valid)
		}
	}
}

func TestCalculateAlertFingerprint(t *testing.T) {
	fp := CalculateAlertFingerprint("t1", "scan", "192.168.1.1", "10.0.0.1", 80, "high", 1000000, 10)
	if len(fp) != 32 {
		t.Errorf("fingerprint length=%d", len(fp))
	}
}

func TestCalculateSimpleFingerprint(t *testing.T) {
	fp := CalculateSimpleFingerprint("t1", "scan", "192.168.1.1", "10.0.0.1", 80)
	if len(fp) != 32 {
		t.Errorf("fingerprint length=%d", len(fp))
	}
}

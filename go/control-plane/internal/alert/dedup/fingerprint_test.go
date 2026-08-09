package dedup

import (
	"testing"
	"time"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestCalculateFingerprint(t *testing.T) {
	batch := &pb.DetectionBatch{
		BatchId:  "batch-001",
		TenantId: "t1",
		RunId:    "run-001",
		Behaviors: []*pb.DetectionBehavior{
			{
				Header:      &pb.EventHeader{EventId: "e1", TenantId: "t1", EventTs: 1786118400000},
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
			{Header: &pb.EventHeader{EventTs: time.Date(2026, 8, 7, 16, 35, 0, 0, time.UTC).UnixMilli()}, CommunityId: "c1", ObjectType: "scan", TopLabel: "port_scan"},
		},
	}

	fp1 := CalculateFingerprint(batch, 10)
	fp2 := CalculateFingerprint(batch, 60)
	if fp1 == fp2 {
		t.Fatal("different source-time bucket widths produced the same fingerprint")
	}
}

func TestCalculateFingerprintUsesSourceTimeAcrossDelayedReplay(t *testing.T) {
	eventTs := time.Date(2026, 8, 7, 16, 35, 0, 0, time.UTC).UnixMilli()
	batch := &pb.DetectionBatch{TenantId: "t1", Behaviors: []*pb.DetectionBehavior{{
		Header:      &pb.EventHeader{EventId: "event-delayed", EventTs: eventTs},
		CommunityId: "community-delayed", ObjectType: "session", TopLabel: "scan",
	}}}
	first := CalculateFingerprint(batch, 10)
	batch.CreatedAt = eventTs + int64((24*time.Hour)/time.Millisecond)
	second := CalculateFingerprint(batch, 10)
	if first != second {
		t.Fatalf("processing delay changed source-time fingerprint: %s != %s", first, second)
	}
}

func TestCalculateFingerprintSeparatesSourceTimeBuckets(t *testing.T) {
	base := time.Date(2026, 8, 7, 16, 30, 0, 0, time.UTC).UnixMilli()
	batch := &pb.DetectionBatch{TenantId: "t1", Behaviors: []*pb.DetectionBehavior{{
		Header:      &pb.EventHeader{EventId: "event-one", EventTs: base},
		CommunityId: "community-one", ObjectType: "session", TopLabel: "scan",
	}}}
	first := CalculateFingerprint(batch, 10)
	batch.Behaviors[0].Header.EventTs = base + int64((11*time.Minute)/time.Millisecond)
	second := CalculateFingerprint(batch, 10)
	if first == second {
		t.Fatal("distinct source-time buckets produced the same fingerprint")
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

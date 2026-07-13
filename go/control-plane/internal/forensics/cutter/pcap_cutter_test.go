package cutter

import (
	"testing"
)

// =============================================================================
// CutQuery.Validate
// =============================================================================

func TestCutQuery_Validate_EmptyTenant(t *testing.T) {
	q := &CutQuery{}
	err := q.Validate()
	if err == nil {
		t.Fatal("expected error for empty tenant")
	}
}

func TestCutQuery_Validate_MissingTimeRange(t *testing.T) {
	q := &CutQuery{
		TenantID: "test-tenant",
	}
	err := q.Validate()
	if err == nil {
		t.Fatal("expected error for missing time range or community_id")
	}
}

func TestCutQuery_Validate_ReversedTime(t *testing.T) {
	q := &CutQuery{
		TenantID:  "test-tenant",
		StartTime: 2000,
		EndTime:   1000,
	}
	err := q.Validate()
	if err == nil {
		t.Fatal("expected error for reversed time range")
	}
}

func TestCutQuery_Validate_TimeRangeTooLarge(t *testing.T) {
	// 25 hours > 24 hour max
	q := &CutQuery{
		TenantID:  "test-tenant",
		StartTime: 0,
		EndTime:   25 * 3600 * 1000,
	}
	err := q.Validate()
	if err == nil {
		t.Fatal("expected error for time range > 24h")
	}
}

func TestCutQuery_Validate_ValidTimeRange(t *testing.T) {
	q := &CutQuery{
		TenantID:  "test-tenant",
		StartTime: 1000,
		EndTime:   2000,
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCutQuery_Validate_WithCommunityID(t *testing.T) {
	// CommunityID alone is insufficient — Validate requires StartTime + EndTime
	q := &CutQuery{
		TenantID:    "test-tenant",
		CommunityID: "1:CpuULklTENbGdRpvp7gNcQd5ZqA=",
		StartTime:   1000,
		EndTime:     2000,
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("expected no error with community_id + time range, got: %v", err)
	}
}

func TestCutQuery_Validate_WithMaxPackets(t *testing.T) {
	q := &CutQuery{
		TenantID:   "test-tenant",
		StartTime:  1000,
		EndTime:    2000,
		MaxPackets: 50000,
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// =============================================================================
// CutQuery → IndexQuery conversion
// =============================================================================

func TestCutQuery_ToIndexQuery(t *testing.T) {
	q := &CutQuery{
		TenantID:    "test-tenant",
		ProbeID:     "probe-01",
		SrcIP:       "10.0.0.1",
		DstIP:       "10.0.0.2",
		CommunityID: "1:abc=",
		StartTime:   1000,
		EndTime:     2000,
		MaxPackets:  1000,
	}
	iq := q.ToIndexQuery()
	if iq.TenantID != q.TenantID {
		t.Errorf("TenantID: want %q, got %q", q.TenantID, iq.TenantID)
	}
	if iq.ProbeID != q.ProbeID {
		t.Errorf("ProbeID: want %q, got %q", q.ProbeID, iq.ProbeID)
	}
	if iq.CommunityID != q.CommunityID {
		t.Errorf("CommunityID: want %q, got %q", q.CommunityID, iq.CommunityID)
	}
	if iq.StartTime != q.StartTime {
		t.Errorf("StartTime: want %d, got %d", q.StartTime, iq.StartTime)
	}
	// ToIndexQuery maps fields 1:1; MaxPackets is NOT set as Limit
	if iq.CommunityID != q.CommunityID {
		t.Errorf("CommunityID: want %q, got %q", q.CommunityID, iq.CommunityID)
	}
}

// =============================================================================
// CutterConfig defaults
// =============================================================================

func TestDefaultCutterConfig(t *testing.T) {
	cfg := DefaultCutterConfig()
	if cfg.MaxConcurrent <= 0 {
		t.Error("MaxConcurrent should be positive")
	}
	if cfg.MaxPackets <= 0 {
		t.Error("MaxPackets should be positive")
	}
	if cfg.PerFileTimeout <= 0 {
		t.Error("PerFileTimeout should be positive")
	}
	if cfg.BufferSize <= 0 {
		t.Error("BufferSize should be positive")
	}
}

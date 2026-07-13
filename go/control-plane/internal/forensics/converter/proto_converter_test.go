package converter

import (
	"testing"
	"time"
)

// =============================================================================
// CutRequestParams.Validate — time validation
// =============================================================================

func validBaseParams() *CutRequestParams {
	now := time.Now().UnixMilli()
	oneHourAgo := now - 3600*1000
	return &CutRequestParams{
		TenantID:   "test-tenant",
		StartTime:  oneHourAgo,
		EndTime:    now,
		MaxPackets: 1000,
		SrcIP:      "10.0.0.1", // at least one filter condition required
	}
}

func TestCutRequestParams_Validate_NegativeTimestamp(t *testing.T) {
	p := validBaseParams()
	p.StartTime = -100
	err := p.Validate()
	if err == nil {
		t.Error("expected error for negative start_time")
	}
	p.StartTime = validBaseParams().StartTime
	p.EndTime = -100
	err = p.Validate()
	if err == nil {
		t.Error("expected error for negative end_time")
	}
}

func TestCutRequestParams_Validate_ZeroTimestamp(t *testing.T) {
	p := validBaseParams()
	p.StartTime = 0
	err := p.Validate()
	if err == nil {
		t.Error("expected error for zero start_time")
	}
}

func TestCutRequestParams_Validate_FutureTimestamp(t *testing.T) {
	p := validBaseParams()
	farFuture := time.Now().UnixMilli() + 3*3600*1000 // 3 hours future
	p.StartTime = farFuture
	p.EndTime = farFuture + 1000
	err := p.Validate()
	if err == nil {
		t.Error("expected error for future timestamp (more than 1h offset)")
	}
}

func TestCutRequestParams_Validate_ReversedTime(t *testing.T) {
	p := validBaseParams()
	p.StartTime, p.EndTime = p.EndTime, p.StartTime
	err := p.Validate()
	if err == nil {
		t.Error("expected error for reversed time range")
	}
}

func TestCutRequestParams_Validate_TimeRangeTooShort(t *testing.T) {
	p := validBaseParams()
	p.EndTime = p.StartTime + 500 // < 1 second
	err := p.Validate()
	if err == nil {
		t.Error("expected error for time range < 1 second")
	}
}

func TestCutRequestParams_Validate_TimeRangeTooLong(t *testing.T) {
	p := validBaseParams()
	p.EndTime = p.StartTime + 25*3600*1000 // 25 hours
	err := p.Validate()
	if err == nil {
		t.Error("expected error for time range > 24 hours")
	}
}

// =============================================================================
// MaxPackets validation
// =============================================================================

func TestCutRequestParams_Validate_NegativeMaxPackets(t *testing.T) {
	p := validBaseParams()
	p.MaxPackets = -1
	err := p.Validate()
	if err == nil {
		t.Error("expected error for negative max_packets")
	}
}

func TestCutRequestParams_Validate_MaxPacketsExceedsLimit(t *testing.T) {
	p := validBaseParams()
	p.MaxPackets = MaxPacketsLimit + 1
	err := p.Validate()
	if err == nil {
		t.Error("expected error for max_packets exceeding limit")
	}
}

func TestCutRequestParams_Validate_MaxPacketsAtLimit(t *testing.T) {
	p := validBaseParams()
	p.MaxPackets = MaxPacketsLimit // exactly at limit
	err := p.Validate()
	if err != nil {
		t.Errorf("max_packets at limit should be valid: %v", err)
	}
}

// =============================================================================
// IP validation
// =============================================================================

func TestCutRequestParams_Validate_InvalidSrcIP(t *testing.T) {
	p := validBaseParams()
	p.SrcIP = "not-an-ip"
	err := p.Validate()
	if err == nil {
		t.Error("expected error for invalid src_ip")
	}
}

func TestCutRequestParams_Validate_ValidIPv4(t *testing.T) {
	p := validBaseParams()
	p.SrcIP = "10.0.0.1"
	p.DstIP = "10.0.0.2"
	err := p.Validate()
	if err != nil {
		t.Errorf("valid IPv4 should pass: %v", err)
	}
}

func TestCutRequestParams_Validate_ValidIPv6(t *testing.T) {
	p := validBaseParams()
	p.SrcIP = "::1"
	p.DstIP = "fe80::1"
	err := p.Validate()
	if err != nil {
		t.Errorf("valid IPv6 should pass: %v", err)
	}
}

func TestCutRequestParams_Validate_InvalidDstIP(t *testing.T) {
	p := validBaseParams()
	p.DstIP = "999.999.999.999"
	err := p.Validate()
	if err == nil {
		t.Error("expected error for invalid dst_ip")
	}
}

// =============================================================================
// Port validation
// =============================================================================

func TestCutRequestParams_Validate_PortAtMaxBoundary(t *testing.T) {
	// SrcPort/DstPort are uint16, so >65535 is prevented by Go type system.
	// Verify Validate accepts max boundary value.
	p := validBaseParams()
	p.SrcPort = uint16(MaxPort) // 65535
	p.DstPort = uint16(MaxPort)
	err := p.Validate()
	if err != nil {
		t.Errorf("max port value (65535) should pass: %v", err)
	}
}

func TestCutRequestParams_Validate_ValidPortRange(t *testing.T) {
	p := validBaseParams()
	p.SrcPort = 80
	p.DstPort = 443
	err := p.Validate()
	if err != nil {
		t.Errorf("valid ports should pass: %v", err)
	}
}

// =============================================================================
// ProbeID and TenantID format validation
// =============================================================================

func TestCutRequestParams_Validate_ProbeID_Valid(t *testing.T) {
	p := validBaseParams()
	p.ProbeID = "probe-agent-01"
	err := p.Validate()
	if err != nil {
		t.Errorf("valid ProbeID should pass: %v", err)
	}
}

func TestCutRequestParams_Validate_ProbeID_InvalidChars(t *testing.T) {
	p := validBaseParams()
	p.ProbeID = "probe agent!" // space and special chars
	err := p.Validate()
	if err == nil {
		t.Error("expected error for ProbeID with invalid chars")
	}
}

func TestCutRequestParams_Validate_ProbeID_TooShort(t *testing.T) {
	p := validBaseParams()
	p.ProbeID = "ab" // < 3 chars
	err := p.Validate()
	if err == nil {
		t.Error("expected error for ProbeID shorter than 3 chars")
	}
}

func TestCutRequestParams_Validate_ProbeID_TooLong(t *testing.T) {
	p := validBaseParams()
	p.ProbeID = "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz1234567890XXXXX" // >64 chars
	err := p.Validate()
	if err == nil {
		t.Error("expected error for ProbeID longer than 64 chars")
	}
}

func TestCutRequestParams_Validate_TenantID_Valid(t *testing.T) {
	p := validBaseParams()
	p.TenantID = "test-tenant" // alphanumeric + hyphen, 2-64 chars
	err := p.Validate()
	if err != nil {
		t.Errorf("valid TenantID should pass: %v", err)
	}
}

func TestCutRequestParams_Validate_TenantID_TooShort(t *testing.T) {
	p := validBaseParams()
	p.TenantID = "a" // < 2 chars
	err := p.Validate()
	if err == nil {
		t.Error("expected error for TenantID shorter than 2 chars")
	}
}

// =============================================================================
// CommunityID validation
// =============================================================================

func TestCutRequestParams_Validate_CommunityID_Valid(t *testing.T) {
	p := validBaseParams()
	p.CommunityID = "1:CpuULklTENbGdRpvp7gNcQd5ZqA="
	err := p.Validate()
	if err != nil {
		t.Errorf("valid CommunityID should pass: %v", err)
	}
}

func TestCutRequestParams_Validate_CommunityID_TooLong(t *testing.T) {
	p := validBaseParams()
	// Build a CommunityID > 128 chars
	tooLong := "1:"
	for i := 0; i < 130; i++ {
		tooLong += "A"
	}
	p.CommunityID = tooLong
	err := p.Validate()
	if err == nil {
		t.Error("expected error for CommunityID > 128 chars")
	}
}

func TestCutRequestParams_Validate_CommunityID_InvalidChars(t *testing.T) {
	p := validBaseParams()
	p.CommunityID = "1:hash with spaces!"
	err := p.Validate()
	if err == nil {
		t.Error("expected error for CommunityID with invalid chars")
	}
}

// =============================================================================
// ValidateAndNormalize
// =============================================================================

func TestCutRequestParams_ValidateAndNormalize_SpaceTrim(t *testing.T) {
	p := validBaseParams()
	p.SrcIP = "  10.0.0.1  "
	p.TenantID = "  test-tenant  "
	err := p.ValidateAndNormalize()
	if err != nil {
		t.Errorf("ValidateAndNormalize should pass: %v", err)
	}
	if p.SrcIP != "10.0.0.1" {
		t.Errorf("SrcIP should be trimmed: got %q", p.SrcIP)
	}
	if p.TenantID != "test-tenant" {
		t.Errorf("TenantID should be trimmed: got %q", p.TenantID)
	}
}

// =============================================================================
// Protocol name helpers
// =============================================================================

func TestGetProtocolName(t *testing.T) {
	tests := []struct {
		proto uint8
		name  string
	}{
		{6, "TCP"},
		{17, "UDP"},
		{1, "ICMP"},
		{132, "SCTP"},
		{0, "UNKNOWN(0)"},
	}
	for _, tt := range tests {
		if got := GetProtocolName(tt.proto); got != tt.name {
			t.Errorf("protocol %d: want %q, got %q", tt.proto, tt.name, got)
		}
	}
}

func TestParseTimeRange_ExplicitValues(t *testing.T) {
	// Explicit start/end are returned as-is
	gotStart, gotEnd := ParseTimeRange(1000, 2000, 1)
	if gotStart != 1000 || gotEnd != 2000 {
		t.Errorf("explicit values: want (1000, 2000), got (%d, %d)", gotStart, gotEnd)
	}
}

func TestParseTimeRange_EndZero(t *testing.T) {
	// When end=0, end is set to now and start computed from defaultHours
	gotStart, gotEnd := ParseTimeRange(1000, 0, 1)
	if gotStart != 1000 {
		t.Errorf("start should remain 1000: got %d", gotStart)
	}
	if gotEnd <= gotStart {
		t.Errorf("end should be after start: start=%d, end=%d", gotStart, gotEnd)
	}
}

func TestParseTimeRange_StartZero(t *testing.T) {
	// When start=0, start is computed from end-defaultHours
	end := int64(10000000)
	gotStart, gotEnd := ParseTimeRange(0, end, 1)
	if gotEnd != end {
		t.Errorf("end should remain %d: got %d", end, gotEnd)
	}
	expectedStart := end - int64(1)*3600*1000
	if gotStart != expectedStart {
		t.Errorf("start: want %d, got %d", expectedStart, gotStart)
	}
}

func TestParseTimeRange_BothZero(t *testing.T) {
	// Both zero → both use now
	gotStart, gotEnd := ParseTimeRange(0, 0, 2)
	if gotStart == 0 || gotEnd == 0 {
		t.Error("both zero should yield non-zero timestamps")
	}
	if gotStart >= gotEnd {
		t.Errorf("start should be before end: %d >= %d", gotStart, gotEnd)
	}
}

func TestNormalizeIP(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"10.0.0.1", "10.0.0.1"},
		{"  10.0.0.1  ", "10.0.0.1"},
		{"::1", "::1"},
	}
	for _, tt := range tests {
		got := NormalizeIP(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeIP(%q): want %q, got %q", tt.input, tt.want, got)
		}
	}
}

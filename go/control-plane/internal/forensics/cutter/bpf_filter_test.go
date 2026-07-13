package cutter

import (
	"net"
	"testing"
)

// =============================================================================
// BuildBPFFilter tests
// =============================================================================

func TestBuildBPFFilter_EmptyQuery(t *testing.T) {
	q := &CutQuery{}
	f := BuildBPFFilter(q)
	if f == nil {
		t.Fatal("BuildBPFFilter returned nil")
	}
	if !f.IsEmpty() {
		t.Errorf("empty query should produce empty filter, got: %s", f.String())
	}
	if f.HasFilter() {
		t.Error("empty query filter should report no filters")
	}
}

func TestBuildBPFFilter_IPv4Only(t *testing.T) {
	q := &CutQuery{
		SrcIP: "10.0.0.1",
		DstIP: "10.0.0.2",
	}
	f := BuildBPFFilter(q)
	if f.IsEmpty() || f.HasFilter() == false {
		t.Fatal("filter should be non-empty")
	}
	if len(f.SrcIPBytes) != 4 || len(f.DstIPBytes) != 4 {
		t.Errorf("expected 4-byte IPv4, got src=%d dst=%d", len(f.SrcIPBytes), len(f.DstIPBytes))
	}
	if f.SrcIPv6 || f.DstIPv6 {
		t.Error("IPv4 addresses should not set IPv6 flag")
	}
}

func TestBuildBPFFilter_IPv6Only(t *testing.T) {
	q := &CutQuery{
		SrcIP: "::1",
		DstIP: "fe80::1",
	}
	f := BuildBPFFilter(q)
	if !f.SrcIPv6 || !f.DstIPv6 {
		t.Error("IPv6 addresses should set IPv6 flag")
	}
	if len(f.SrcIPBytes) != 16 || len(f.DstIPBytes) != 16 {
		t.Errorf("expected 16-byte IPv6, got src=%d dst=%d", len(f.SrcIPBytes), len(f.DstIPBytes))
	}
}

func TestBuildBPFFilter_WithPorts(t *testing.T) {
	q := &CutQuery{
		SrcIP:   "192.168.1.1",
		SrcPort: 443,
		DstPort: 8080,
	}
	f := BuildBPFFilter(q)
	if !f.hasPortFilter {
		t.Error("port filter should be active")
	}
}

func TestBuildBPFFilter_WithTimeRange(t *testing.T) {
	q := &CutQuery{
		StartTime: 1000,
		EndTime:   2000,
	}
	f := BuildBPFFilter(q)
	if !f.hasTimeFilter {
		t.Error("time filter should be active")
	}
}

func TestBuildBPFFilter_ToBPFString(t *testing.T) {
	q := &CutQuery{
		SrcIP:    "10.0.0.1",
		DstIP:    "10.0.0.2",
		SrcPort:  12345,
		DstPort:  80,
		Protocol: 6, // TCP
	}
	f := BuildBPFFilter(q)
	bpf := f.ToBPFString()
	if bpf == "" {
		t.Error("ToBPFString should not be empty")
	}
	// Should contain key elements
	for _, want := range []string{"10.0.0.1", "10.0.0.2", "tcp", "12345", "80"} {
		if !containsString(bpf, want) && !containsString(bpf, toLower(want)) {
			t.Errorf("BPF string missing %q: %s", want, bpf)
		}
	}
}

func TestBuildBPFFilter_ProtocolName(t *testing.T) {
	tests := []struct {
		proto uint8
		name  string
	}{
		{6, "TCP"},
		{17, "UDP"},
		{132, "SCTP"},
		{1, "ICMP"},
		{0, "ANY"},
	}
	for _, tt := range tests {
		q := &CutQuery{Protocol: tt.proto}
		f := BuildBPFFilter(q)
		if got := f.GetProtocolName(); got != tt.name {
			t.Errorf("protocol %d: want %q, got %q", tt.proto, tt.name, got)
		}
	}
}

// =============================================================================
// BPFFilter.Match — packet matching
// =============================================================================

// buildTestEthernetFrame constructs a minimal Ethernet+IPv4+TCP packet
func buildTestEthernetFrame(srcIP, dstIP net.IP, srcPort, dstPort uint16) []byte {
	src := srcIP.To4()
	dst := dstIP.To4()
	if src == nil || dst == nil {
		return nil
	}

	pkt := make([]byte, 14+20+20) // Eth + IPv4 + TCP
	// Ethernet header (dst MAC + src MAC + EtherType)
	copy(pkt[0:6], []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
	copy(pkt[6:12], []byte{0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb})
	pkt[12] = 0x08 // EtherType IPv4
	pkt[13] = 0x00

	// IPv4 header
	pkt[14] = 0x45                 // Version + IHL
	pkt[23] = 6                     // Protocol TCP
	copy(pkt[26:30], src)           // Src IP
	copy(pkt[30:34], dst)           // Dst IP

	// TCP header
	tcpOff := 34
	pkt[tcpOff] = byte(srcPort >> 8)
	pkt[tcpOff+1] = byte(srcPort)
	pkt[tcpOff+2] = byte(dstPort >> 8)
	pkt[tcpOff+3] = byte(dstPort)
	// Data offset (5 * 4 = 20 bytes)
	pkt[tcpOff+12] = 0x50

	return pkt
}

func TestBPFFilter_Match_IPv4_TCP(t *testing.T) {
	srcIP := net.ParseIP("10.0.0.1")
	dstIP := net.ParseIP("10.0.0.2")
	pkt := buildTestEthernetFrame(srcIP, dstIP, 12345, 80)

	q := &CutQuery{
		SrcIP:   "10.0.0.1",
		DstIP:   "10.0.0.2",
		SrcPort: 12345,
		DstPort: 80,
		Protocol: 6,
	}
	f := BuildBPFFilter(q)
	if !f.Match(pkt, 0) {
		t.Error("filter should match exact TCP packet")
	}
}

func TestBPFFilter_Match_Bidirectional_IP(t *testing.T) {
	srcIP := net.ParseIP("10.0.0.2") // swapped
	dstIP := net.ParseIP("10.0.0.1")
	pkt := buildTestEthernetFrame(srcIP, dstIP, 12345, 80)

	q := &CutQuery{
		SrcIP: "10.0.0.1", // original direction
		DstIP: "10.0.0.2",
	}
	f := BuildBPFFilter(q)
	if !f.Match(pkt, 0) {
		t.Error("filter should match bidirectional IP")
	}
}

func TestBPFFilter_Match_WrongIP(t *testing.T) {
	srcIP := net.ParseIP("192.168.1.1")
	dstIP := net.ParseIP("192.168.1.2")
	pkt := buildTestEthernetFrame(srcIP, dstIP, 22, 443)

	q := &CutQuery{
		SrcIP: "10.0.0.1",
		DstIP: "10.0.0.2",
	}
	f := BuildBPFFilter(q)
	if f.Match(pkt, 0) {
		t.Error("filter should NOT match wrong IP packet")
	}
}

func TestBPFFilter_Match_WrongPort(t *testing.T) {
	srcIP := net.ParseIP("10.0.0.1")
	dstIP := net.ParseIP("10.0.0.2")
	pkt := buildTestEthernetFrame(srcIP, dstIP, 22, 443)

	q := &CutQuery{
		SrcIP:   "10.0.0.1",
		DstIP:   "10.0.0.2",
		SrcPort: 12345,
	}
	f := BuildBPFFilter(q)
	if f.Match(pkt, 0) {
		t.Error("filter should NOT match wrong port packet")
	}
}

func TestBPFFilter_Match_TimeFilter(t *testing.T) {
	srcIP := net.ParseIP("10.0.0.1")
	dstIP := net.ParseIP("10.0.0.2")
	pkt := buildTestEthernetFrame(srcIP, dstIP, 80, 443)

	q := &CutQuery{
		StartTime: 1000,
		EndTime:   2000,
	}
	f := BuildBPFFilter(q)

	if f.Match(pkt, 500) {
		t.Error("filter should reject timestamp before range")
	}
	if !f.Match(pkt, 1500) {
		t.Error("filter should accept timestamp within range")
	}
	if f.Match(pkt, 2500) {
		t.Error("filter should reject timestamp after range")
	}
}

func TestBPFFilter_Match_UDP_Protocol(t *testing.T) {
	srcIP := net.ParseIP("10.0.0.1")
	dstIP := net.ParseIP("10.0.0.2")
	pkt := buildTestEthernetFrame(srcIP, dstIP, 53, 12345)
	// Override IPv4 protocol field to UDP (17)
	if len(pkt) > 23 {
		pkt[23] = 17
	}

	q := &CutQuery{Protocol: 17}
	f := BuildBPFFilter(q)
	if !f.Match(pkt, 0) {
		t.Error("filter should match UDP packet")
	}
}

func TestBPFFilter_Match_EmptyFilter(t *testing.T) {
	srcIP := net.ParseIP("10.0.0.1")
	dstIP := net.ParseIP("10.0.0.2")
	pkt := buildTestEthernetFrame(srcIP, dstIP, 80, 443)

	f := BuildBPFFilter(&CutQuery{})
	if !f.Match(pkt, 0) {
		t.Error("empty filter should match all packets")
	}
}

// =============================================================================
// BPFFilterWithStats
// =============================================================================

func TestBPFFilterWithStats(t *testing.T) {
	srcIP := net.ParseIP("10.0.0.1")
	dstIP := net.ParseIP("10.0.0.2")
	pkt := buildTestEthernetFrame(srcIP, dstIP, 80, 443)

	q := &CutQuery{SrcIP: "10.0.0.1"}
	fs := NewBPFFilterWithStats(q)

	// Match 3 packets
	for i := 0; i < 3; i++ {
		fs.Match(pkt, 0)
	}

	stats := fs.GetStats()
	if stats.TotalPackets != 3 {
		t.Errorf("TotalPackets: want 3, got %d", stats.TotalPackets)
	}
	if stats.MatchedPackets != 3 {
		t.Errorf("MatchedPackets: want 3, got %d", stats.MatchedPackets)
	}

	// Reset and verify
	fs.ResetStats()
	stats = fs.GetStats()
	if stats.TotalPackets != 0 {
		t.Errorf("after reset: want 0, got %d", stats.TotalPackets)
	}
}

func TestBPFFilterWithStats_TimeFiltered(t *testing.T) {
	srcIP := net.ParseIP("10.0.0.1")
	dstIP := net.ParseIP("10.0.0.2")
	pkt := buildTestEthernetFrame(srcIP, dstIP, 80, 443)

	q := &CutQuery{StartTime: 1000, EndTime: 2000}
	fs := NewBPFFilterWithStats(q)

	fs.Match(pkt, 500)
	fs.Match(pkt, 1500)

	stats := fs.GetStats()
	if stats.TimeFiltered < 1 {
		t.Errorf("expected at least 1 time-filtered, got %d", stats.TimeFiltered)
	}
}

// =============================================================================
// BPFFilter.IsEmpty / HasFilter / String
// =============================================================================

func TestBPFFilter_IsEmpty_AllCases(t *testing.T) {
	tests := []struct {
		name   string
		query  *CutQuery
		empty  bool
	}{
		{"empty query", &CutQuery{}, true},
		{"IP only", &CutQuery{SrcIP: "10.0.0.1"}, false},
		{"port only", &CutQuery{SrcPort: 80}, false},
		{"time only", &CutQuery{StartTime: 1, EndTime: 2}, false},
		{"protocol only", &CutQuery{Protocol: 6}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := BuildBPFFilter(tt.query)
			if got := f.IsEmpty(); got != tt.empty {
				t.Errorf("IsEmpty: want %v, got %v", tt.empty, got)
			}
		})
	}
}

func TestBPFFilter_String(t *testing.T) {
	q := &CutQuery{
		SrcIP:   "10.0.0.1",
		DstIP:   "10.0.0.2",
		SrcPort: 80,
		Protocol: 6,
	}
	f := BuildBPFFilter(q)
	s := f.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

// =============================================================================
// helpers
// =============================================================================

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i, c := range []byte(s) {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else {
			b[i] = c
		}
	}
	return string(b)
}

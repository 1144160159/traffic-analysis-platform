package communityid

import (
	"net/netip"
	"testing"
)

// 跨语言 parity 向量(Rust community_id.rs / Java CommunityIdUtilTest 同值)。
func TestComputeParityVectors(t *testing.T) {
	cases := []struct {
		name         string
		ipA, ipB     string
		portA, portB uint16
		protocol     uint8
		want         string
	}{
		{"tcp", "10.0.0.1", "10.0.0.2", 12345, 80, 6, "1:CpuULklTENbGdRpvp7gNcQd5ZqA="},
		{"tcp-swapped", "192.168.1.1", "192.168.1.100", 443, 54321, 6, "1:yvabNgZAlWzo8wcUZ6B9cSRJQ9Q="},
		{"udp", "10.0.0.1", "10.0.0.2", 53, 12345, 17, "1:JrhaqgS2mu6o+Lu2/yWyT0ECe6E="},
		{"ipv6", "::1", "::2", 8080, 9090, 6, "1:/Q8HrtOQusOw7LFS4Ju3LeGLJu0="},
	}
	for _, c := range cases {
		got := Compute(netip.MustParseAddr(c.ipA), netip.MustParseAddr(c.ipB), c.protocol, c.portA, c.portB)
		if got != c.want {
			t.Fatalf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestComputeRequiresCanonicalEndpointOrder(t *testing.T) {
	// 契约:Compute 是规范序哈希(调用方先按端点序/方向归一);乱序输入产出不同值
	// —— 这是 community-id-spec 的调用契约,与 Rust canonicalize_observation 一致。
	a := Compute(netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2"), 6, 12345, 80)
	b := Compute(netip.MustParseAddr("10.0.0.2"), netip.MustParseAddr("10.0.0.1"), 6, 80, 12345)
	if a == b {
		t.Fatalf("unordered inputs must not collide by accident")
	}
}

func TestIcmpRequestTypeNormalization(t *testing.T) {
	// ICMPv4 echo request(8)/reply(0) 归一为 request type
	if req, ok := IcmpRequestType(1, 0); !ok || req != 8 {
		t.Fatalf("icmpv4 echo reply must normalize to request type 8, got %d ok=%v", req, ok)
	}
	if req, ok := IcmpRequestType(58, 129); !ok || req != 128 {
		t.Fatalf("icmpv6 echo reply must normalize to request type 128, got %d ok=%v", req, ok)
	}
	if _, ok := IcmpRequestType(1, 42); ok {
		t.Fatalf("unpaired icmp type must not normalize")
	}
}

func TestComputeICMPPairSameHashBothDirections(t *testing.T) {
	// ICMP echo 请求(8,0)与应答(0,0):调用方先经 IcmpRequestType 归一后哈希一致
	reqType, ok := IcmpRequestType(1, 8)
	if !ok {
		t.Fatal("echo request must normalize")
	}
	a := Compute(netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2"), 1, uint16(reqType), 0)
	replyType, ok := IcmpRequestType(1, 0)
	if !ok {
		t.Fatal("echo reply must normalize")
	}
	b := Compute(netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2"), 1, uint16(replyType), 0)
	if a != b {
		t.Fatalf("icmp pair must hash equal after normalization: %s vs %s", a, b)
	}
}

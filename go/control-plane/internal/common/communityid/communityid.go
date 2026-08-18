// Package communityid community-id-spec v1 规范实现(Go 移植;与
// rust/probe-agent aggregator/community_id.rs、java/flink-common CommunityIdUtil 对齐,
// 跨语言 parity 向量见 communityid_test.go)。全控制面复用本包,禁止另立实现。
package communityid

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"net/netip"
)

// icmpRequestReplyV4/V6 请求/应答类型对(与 Rust icmp_mapping 一致)。
var icmpRequestReplyV4 = [][2]uint8{{8, 0}, {13, 14}, {15, 16}, {17, 18}}
var icmpRequestReplyV6 = [][2]uint8{{128, 129}, {130, 131}, {133, 134}, {135, 136}}

// IcmpRequestType 请求/应答归一:应答类型映射为请求类型;未配对返回 (0,false)。
func IcmpRequestType(protocol uint8, icmpType uint8) (uint8, bool) {
	var table [][2]uint8
	switch protocol {
	case 1:
		table = icmpRequestReplyV4
	case 58:
		table = icmpRequestReplyV6
	default:
		return 0, false
	}
	for _, pair := range table {
		if icmpType == pair[0] || icmpType == pair[1] {
			return pair[0], true
		}
	}
	return 0, false
}

// Compute 按 community-id-spec v1 计算(Rust compute_community_id 移植,ICMP 对齐):
// seed(2B) + ipA + ipB + protocol(1B) + padding(1B) + portA(2B) + portB(2B) → SHA1 → base64,
// 前缀 "1:"。ICMP(protocol 1/58)两端口取同一 16-bit 值 (type<<8)|code。
func Compute(ipA, ipB netip.Addr, protocol uint8, portA, portB uint16) string {
	h := sha1.New()
	var zero [2]byte
	_, _ = h.Write(zero[:])
	_, _ = h.Write(ipA.AsSlice())
	_, _ = h.Write(ipB.AsSlice())
	_, _ = h.Write([]byte{protocol})
	_, _ = h.Write([]byte{0})
	switch protocol {
	case 1, 58:
		port := uint16((portA&0xff)<<8) | (portB & 0xff)
		var buf [2]byte
		binary.BigEndian.PutUint16(buf[:], port)
		_, _ = h.Write(buf[:])
		_, _ = h.Write(buf[:])
	default:
		var b1, b2 [2]byte
		binary.BigEndian.PutUint16(b1[:], portA)
		binary.BigEndian.PutUint16(b2[:], portB)
		_, _ = h.Write(b1[:])
		_, _ = h.Write(b2[:])
	}
	return "1:" + base64.StdEncoding.EncodeToString(h.Sum(nil))
}

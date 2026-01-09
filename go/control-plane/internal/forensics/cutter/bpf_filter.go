////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/forensics/cutter/bpf_filter.go
// 性能优化版：
// - ✅ M2: 使用零拷贝字节解析替代 gopacket
// - ✅ 减少内存分配和字符串转换
// - ✅ 优化包匹配逻辑（快速路径优先）
// - ✅ 支持 IPv4/IPv6
////////////////////////////////////////////////////////////////////////////////

package cutter

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

// BPFFilter BPF 过滤器（高性能版）
type BPFFilter struct {
	// 字符串形式（用于日志）
	SrcIP     string
	DstIP     string
	SrcPort   uint16
	DstPort   uint16
	Protocol  uint8
	StartTime int64
	EndTime   int64

	// ========== 优化：预计算字节表示 ==========
	SrcIPBytes []byte // 预计算的 IP 字节表示（4 字节 IPv4 或 16 字节 IPv6）
	DstIPBytes []byte
	SrcIPv6    bool // 是否为 IPv6
	DstIPv6    bool

	// ========== 优化：位掩码 ==========
	hasIPFilter   bool
	hasPortFilter bool
	hasTimeFilter bool
}

// BuildBPFFilter 从查询构建 BPF 过滤器（优化版）
func BuildBPFFilter(query *CutQuery) *BPFFilter {
	f := &BPFFilter{
		SrcIP:     query.SrcIP,
		DstIP:     query.DstIP,
		SrcPort:   query.SrcPort,
		DstPort:   query.DstPort,
		Protocol:  query.Protocol,
		StartTime: query.StartTime,
		EndTime:   query.EndTime,
	}

	// ========== 预计算 IP 字节表示 ==========
	if query.SrcIP != "" {
		if ip := net.ParseIP(query.SrcIP); ip != nil {
			if ipv4 := ip.To4(); ipv4 != nil {
				f.SrcIPBytes = ipv4
				f.SrcIPv6 = false
			} else {
				f.SrcIPBytes = ip.To16()
				f.SrcIPv6 = true
			}
		}
	}

	if query.DstIP != "" {
		if ip := net.ParseIP(query.DstIP); ip != nil {
			if ipv4 := ip.To4(); ipv4 != nil {
				f.DstIPBytes = ipv4
				f.DstIPv6 = false
			} else {
				f.DstIPBytes = ip.To16()
				f.DstIPv6 = true
			}
		}
	}

	// ========== 设置位掩码（优化快速路径判断） ==========
	f.hasIPFilter = len(f.SrcIPBytes) > 0 || len(f.DstIPBytes) > 0
	f.hasPortFilter = query.SrcPort != 0 || query.DstPort != 0
	f.hasTimeFilter = query.StartTime > 0 || query.EndTime > 0

	return f
}

// Match 检查数据包是否匹配过滤条件（高性能版）
func (f *BPFFilter) Match(data []byte, timestamp int64) bool {
	// ========== 快速路径 1: 时间过滤 ==========
	if f.hasTimeFilter {
		if f.StartTime > 0 && timestamp < f.StartTime {
			return false
		}
		if f.EndTime > 0 && timestamp > f.EndTime {
			return false
		}
	}

	// ========== 快速路径 2: 包长检查 ==========
	if len(data) < 14 { // 最小以太网帧头
		return false
	}

	// ========== 解析以太网帧 ==========
	etherType := binary.BigEndian.Uint16(data[12:14])

	var ipStart int
	var ipVersion byte

	switch etherType {
	case 0x0800: // IPv4
		ipStart = 14
		ipVersion = 4
	case 0x8100: // VLAN (802.1Q)
		if len(data) < 18 {
			return false
		}
		// 跳过 VLAN 标签（4 字节）
		innerEtherType := binary.BigEndian.Uint16(data[16:18])
		if innerEtherType == 0x0800 {
			ipStart = 18
			ipVersion = 4
		} else if innerEtherType == 0x86DD {
			ipStart = 18
			ipVersion = 6
		} else {
			return false
		}
	case 0x86DD: // IPv6
		ipStart = 14
		ipVersion = 6
	default:
		// 不支持的协议类型
		return false
	}

	// ========== 根据 IP 版本进行匹配 ==========
	if ipVersion == 4 {
		return f.matchIPv4(data[ipStart:])
	} else if ipVersion == 6 {
		return f.matchIPv6(data[ipStart:])
	}

	return false
}

// matchIPv4 匹配 IPv4 数据包（零拷贝优化版）
func (f *BPFFilter) matchIPv4(ipData []byte) bool {
	// ========== IPv4 头部最小长度检查 ==========
	if len(ipData) < 20 {
		return false
	}

	// ========== 协议过滤（快速路径） ==========
	protocol := ipData[9]
	if f.Protocol != 0 && protocol != f.Protocol {
		return false
	}

	// ========== IP 地址匹配（字节比较，零拷贝） ==========
	if f.hasIPFilter {
		srcIP := ipData[12:16]
		dstIP := ipData[16:20]

		// 只有 IPv4 过滤器时才匹配
		if len(f.SrcIPBytes) == 4 {
			// 双向匹配：src_ip 可能是源或目的
			if !bytes.Equal(srcIP, f.SrcIPBytes) && !bytes.Equal(dstIP, f.SrcIPBytes) {
				return false
			}
		}

		if len(f.DstIPBytes) == 4 {
			// 双向匹配：dst_ip 可能是源或目的
			if !bytes.Equal(srcIP, f.DstIPBytes) && !bytes.Equal(dstIP, f.DstIPBytes) {
				return false
			}
		}
	}

	// ========== 端口匹配（仅对 TCP/UDP/SCTP） ==========
	if f.hasPortFilter {
		// 只有特定协议才有端口
		if protocol != 6 && protocol != 17 && protocol != 132 { // TCP, UDP, SCTP
			return false
		}

		// 计算 IP 头部长度
		ihl := (ipData[0] & 0x0F) * 4
		if len(ipData) < int(ihl)+4 {
			return false
		}

		transportStart := ipData[ihl:]

		// 读取源端口和目的端口（网络字节序，大端）
		srcPort := binary.BigEndian.Uint16(transportStart[0:2])
		dstPort := binary.BigEndian.Uint16(transportStart[2:4])

		// 端口双向匹配
		if f.SrcPort != 0 {
			if srcPort != f.SrcPort && dstPort != f.SrcPort {
				return false
			}
		}

		if f.DstPort != 0 {
			if srcPort != f.DstPort && dstPort != f.DstPort {
				return false
			}
		}
	}

	return true
}

// matchIPv6 匹配 IPv6 数据包（零拷贝优化版）
func (f *BPFFilter) matchIPv6(ipData []byte) bool {
	// ========== IPv6 头部最小长度检查 ==========
	if len(ipData) < 40 {
		return false
	}

	// ========== 获取下一个头部类型（协议） ==========
	nextHeader := ipData[6]

	// ========== 协议过滤 ==========
	if f.Protocol != 0 && nextHeader != f.Protocol {
		return false
	}

	// ========== IP 地址匹配（字节比较，零拷贝） ==========
	if f.hasIPFilter {
		srcIP := ipData[8:24]  // IPv6 源地址（16 字节）
		dstIP := ipData[24:40] // IPv6 目的地址（16 字节）

		// 只有 IPv6 过滤器时才匹配
		if len(f.SrcIPBytes) == 16 {
			if !bytes.Equal(srcIP, f.SrcIPBytes) && !bytes.Equal(dstIP, f.SrcIPBytes) {
				return false
			}
		}

		if len(f.DstIPBytes) == 16 {
			if !bytes.Equal(srcIP, f.DstIPBytes) && !bytes.Equal(dstIP, f.DstIPBytes) {
				return false
			}
		}
	}

	// ========== 端口匹配（仅对 TCP/UDP/SCTP） ==========
	if f.hasPortFilter {
		// IPv6 扩展头处理（简化版：跳过扩展头）
		headerStart := 40
		currentHeader := nextHeader

		// 跳过扩展头（最多处理 10 个）
		for i := 0; i < 10; i++ {
			switch currentHeader {
			case 6, 17, 132: // TCP, UDP, SCTP
				// 找到传输层协议
				if len(ipData) < headerStart+4 {
					return false
				}

				transportData := ipData[headerStart:]
				srcPort := binary.BigEndian.Uint16(transportData[0:2])
				dstPort := binary.BigEndian.Uint16(transportData[2:4])

				// 端口双向匹配
				if f.SrcPort != 0 {
					if srcPort != f.SrcPort && dstPort != f.SrcPort {
						return false
					}
				}

				if f.DstPort != 0 {
					if srcPort != f.DstPort && dstPort != f.DstPort {
						return false
					}
				}

				return true

			case 0, 43, 44, 51, 50, 60: // 扩展头类型
				if len(ipData) < headerStart+2 {
					return false
				}
				nextHdr := ipData[headerStart]
				hdrLen := int(ipData[headerStart+1]+1) * 8
				currentHeader = nextHdr
				headerStart += hdrLen

			default:
				// 未知头类型，停止处理
				return false
			}
		}

		// 未找到传输层协议
		return false
	}

	return true
}

// ToBPFString 转换为 BPF 过滤表达式字符串（用于日志）
func (f *BPFFilter) ToBPFString() string {
	var parts []string

	// 协议
	switch f.Protocol {
	case 6:
		parts = append(parts, "tcp")
	case 17:
		parts = append(parts, "udp")
	case 1:
		parts = append(parts, "icmp")
	case 58:
		parts = append(parts, "icmp6")
	case 132:
		parts = append(parts, "sctp")
	}

	// IP 过滤
	if f.SrcIP != "" && f.DstIP != "" {
		parts = append(parts, fmt.Sprintf("(host %s and host %s)", f.SrcIP, f.DstIP))
	} else if f.SrcIP != "" {
		parts = append(parts, fmt.Sprintf("host %s", f.SrcIP))
	} else if f.DstIP != "" {
		parts = append(parts, fmt.Sprintf("host %s", f.DstIP))
	}

	// 端口过滤
	if f.SrcPort != 0 && f.DstPort != 0 {
		parts = append(parts, fmt.Sprintf("(port %d and port %d)", f.SrcPort, f.DstPort))
	} else if f.SrcPort != 0 {
		parts = append(parts, fmt.Sprintf("port %d", f.SrcPort))
	} else if f.DstPort != 0 {
		parts = append(parts, fmt.Sprintf("port %d", f.DstPort))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " and ")
}

// IsEmpty 检查过滤器是否为空
func (f *BPFFilter) IsEmpty() bool {
	return !f.hasIPFilter && !f.hasPortFilter && f.Protocol == 0 && !f.hasTimeFilter
}

// String 返回过滤器的字符串表示
func (f *BPFFilter) String() string {
	var parts []string

	if f.SrcIP != "" {
		parts = append(parts, fmt.Sprintf("src_ip=%s", f.SrcIP))
	}
	if f.DstIP != "" {
		parts = append(parts, fmt.Sprintf("dst_ip=%s", f.DstIP))
	}
	if f.SrcPort != 0 {
		parts = append(parts, fmt.Sprintf("src_port=%d", f.SrcPort))
	}
	if f.DstPort != 0 {
		parts = append(parts, fmt.Sprintf("dst_port=%d", f.DstPort))
	}
	if f.Protocol != 0 {
		parts = append(parts, fmt.Sprintf("protocol=%d", f.Protocol))
	}
	if f.StartTime != 0 {
		parts = append(parts, fmt.Sprintf("start=%d", f.StartTime))
	}
	if f.EndTime != 0 {
		parts = append(parts, fmt.Sprintf("end=%d", f.EndTime))
	}

	if len(parts) == 0 {
		return "BPFFilter{empty}"
	}

	return fmt.Sprintf("BPFFilter{%s}", strings.Join(parts, ", "))
}

// GetProtocolName 获取协议名称（辅助方法）
func (f *BPFFilter) GetProtocolName() string {
	switch f.Protocol {
	case 1:
		return "ICMP"
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 47:
		return "GRE"
	case 50:
		return "ESP"
	case 51:
		return "AH"
	case 58:
		return "ICMPv6"
	case 132:
		return "SCTP"
	default:
		if f.Protocol == 0 {
			return "ANY"
		}
		return fmt.Sprintf("UNKNOWN(%d)", f.Protocol)
	}
}

// HasFilter 检查是否有任何过滤条件
func (f *BPFFilter) HasFilter() bool {
	return !f.IsEmpty()
}

// MatchStats 匹配统计（用于性能分析）
type MatchStats struct {
	TotalPackets     uint64
	MatchedPackets   uint64
	TimeFiltered     uint64
	ProtocolFiltered uint64
	IPFiltered       uint64
	PortFiltered     uint64
}

// BPFFilterWithStats 带统计的过滤器（调试用）
type BPFFilterWithStats struct {
	*BPFFilter
	Stats MatchStats
}

// NewBPFFilterWithStats 创建带统计的过滤器
func NewBPFFilterWithStats(query *CutQuery) *BPFFilterWithStats {
	return &BPFFilterWithStats{
		BPFFilter: BuildBPFFilter(query),
		Stats:     MatchStats{},
	}
}

// Match 带统计的匹配（调试用）
func (f *BPFFilterWithStats) Match(data []byte, timestamp int64) bool {
	f.Stats.TotalPackets++

	// 时间过滤统计
	if f.hasTimeFilter {
		if f.StartTime > 0 && timestamp < f.StartTime {
			f.Stats.TimeFiltered++
			return false
		}
		if f.EndTime > 0 && timestamp > f.EndTime {
			f.Stats.TimeFiltered++
			return false
		}
	}

	matched := f.BPFFilter.Match(data, timestamp)
	if matched {
		f.Stats.MatchedPackets++
	}

	return matched
}

// GetStats 获取统计信息
func (f *BPFFilterWithStats) GetStats() MatchStats {
	return f.Stats
}

// ResetStats 重置统计
func (f *BPFFilterWithStats) ResetStats() {
	f.Stats = MatchStats{}
}

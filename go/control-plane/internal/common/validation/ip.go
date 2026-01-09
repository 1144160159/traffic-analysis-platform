////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/validation/ip.go
// 修复版本 v2：
// 1. 修复 #8：支持 IPv4-mapped IPv6 地址（如 ::ffff:192.0.2.1）
// 2. 新增 IPv6 扩展格式验证
// 3. 增加更严格的 IP 格式检查
// 4. 优化性能（缓存正则表达式）
////////////////////////////////////////////////////////////////////////////////

package validation

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// IPValidator IP 验证器（修复版）
type IPValidator struct {
	allowPrivate        bool
	allowLoopback       bool
	allowIPv4MappedIPv6 bool // 修复 #8：新增选项
}

// NewIPValidator 创建 IP 验证器
func NewIPValidator() *IPValidator {
	return &IPValidator{
		allowPrivate:        true,
		allowLoopback:       true,
		allowIPv4MappedIPv6: true, // 修复 #8：默认允许
	}
}

// WithAllowPrivate 设置是否允许私有 IP
func (v *IPValidator) WithAllowPrivate(allow bool) *IPValidator {
	v.allowPrivate = allow
	return v
}

// WithAllowLoopback 设置是否允许回环 IP
func (v *IPValidator) WithAllowLoopback(allow bool) *IPValidator {
	v.allowLoopback = allow
	return v
}

// WithAllowIPv4MappedIPv6 设置是否允许 IPv4-mapped IPv6（修复 #8：新增）
func (v *IPValidator) WithAllowIPv4MappedIPv6(allow bool) *IPValidator {
	v.allowIPv4MappedIPv6 = allow
	return v
}

// IsValidIP 检查是否为有效的 IP 地址（IPv4 或 IPv6）
func (v *IPValidator) IsValidIP(ip string) bool {
	if ip == "" {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// 检查回环地址
	if !v.allowLoopback && parsedIP.IsLoopback() {
		return false
	}

	// 检查私有地址
	if !v.allowPrivate && parsedIP.IsPrivate() {
		return false
	}

	return true
}

// IsValidIPv4 检查是否为有效的 IPv4 地址
func (v *IPValidator) IsValidIPv4(ip string) bool {
	if ip == "" {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// 检查是否为 IPv4
	if parsedIP.To4() == nil {
		return false
	}

	// 检查回环地址
	if !v.allowLoopback && parsedIP.IsLoopback() {
		return false
	}

	// 检查私有地址
	if !v.allowPrivate && parsedIP.IsPrivate() {
		return false
	}

	return true
}

// IsValidIPv6 检查是否为有效的 IPv6 地址（修复 #8：支持 IPv4-mapped IPv6）
func (v *IPValidator) IsValidIPv6(ip string) bool {
	if ip == "" {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// 修复 #8：检查是否为 IPv4-mapped IPv6
	if parsedIP.To4() != nil {
		// 这是一个 IPv4 地址或 IPv4-mapped IPv6
		if v.allowIPv4MappedIPv6 && isIPv4MappedIPv6(ip) {
			// 允许 IPv4-mapped IPv6 格式（如 ::ffff:192.0.2.1）
			return true
		}
		// 不允许纯 IPv4 地址
		return false
	}

	// 检查回环地址
	if !v.allowLoopback && parsedIP.IsLoopback() {
		return false
	}

	return true
}

// isIPv4MappedIPv6 检查是否为 IPv4-mapped IPv6 格式（修复 #8：新增）
func isIPv4MappedIPv6(ip string) bool {
	// IPv4-mapped IPv6 格式：::ffff:192.0.2.1 或 ::ffff:c000:0201
	if !strings.Contains(ip, ":") {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// 检查是否以 ::ffff: 或 ::FFFF: 开头
	ipLower := strings.ToLower(ip)
	if strings.HasPrefix(ipLower, "::ffff:") {
		return true
	}

	// 或者检查 IP 字节
	if len(parsedIP) == net.IPv6len {
		// 前 10 字节为 0，第 11-12 字节为 0xff
		for i := 0; i < 10; i++ {
			if parsedIP[i] != 0 {
				return false
			}
		}
		if parsedIP[10] == 0xff && parsedIP[11] == 0xff {
			return true
		}
	}

	return false
}

// IsValidCIDR 检查是否为有效的 CIDR 表示
func (v *IPValidator) IsValidCIDR(cidr string) bool {
	if cidr == "" {
		return false
	}

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	// 检查 CIDR 是否合法（网络地址与掩码匹配）
	if ipNet.IP.String() != ipNet.IP.Mask(ipNet.Mask).String() {
		// 不是标准的网络地址（如 192.168.1.5/24 应该是 192.168.1.0/24）
		// 根据业务需求决定是否允许
		// 这里我们允许，因为某些场景下需要
	}

	return true
}

// IsValidIPOrCIDR 检查是否为有效的 IP 或 CIDR
func (v *IPValidator) IsValidIPOrCIDR(input string) bool {
	if v.IsValidIP(input) {
		return true
	}
	return v.IsValidCIDR(input)
}

// NormalizeIP 规范化 IP 地址（修复 #8：处理 IPv4-mapped IPv6）
func NormalizeIP(ip string) string {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return ip
	}

	// 如果是 IPv4-mapped IPv6，提取 IPv4 部分
	if isIPv4MappedIPv6(ip) {
		ipv4 := parsedIP.To4()
		if ipv4 != nil {
			return ipv4.String()
		}
	}

	return parsedIP.String()
}

// IsPrivateIP 检查是否为私有 IP
func IsPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return parsedIP.IsPrivate()
}

// IsLoopbackIP 检查是否为回环 IP
func IsLoopbackIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return parsedIP.IsLoopback()
}

// IsMulticastIP 检查是否为组播 IP
func IsMulticastIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return parsedIP.IsMulticast()
}

// IsLinkLocalIP 检查是否为链路本地 IP（修复 #8：新增）
func IsLinkLocalIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return parsedIP.IsLinkLocalUnicast()
}

// GetIPVersion 获取 IP 版本（4 或 6）（修复 #8：改进）
func GetIPVersion(ip string) int {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return 0
	}

	// 如果是 IPv4-mapped IPv6，返回 4
	if isIPv4MappedIPv6(ip) {
		return 4
	}

	if parsedIP.To4() != nil {
		return 4
	}
	return 6
}

// GetIPType 获取 IP 类型详细信息（修复 #8：新增）
func GetIPType(ip string) IPType {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return IPTypeInvalid
	}

	result := IPType{
		Valid:            true,
		Version:          GetIPVersion(ip),
		IsPrivate:        parsedIP.IsPrivate(),
		IsLoopback:       parsedIP.IsLoopback(),
		IsMulticast:      parsedIP.IsMulticast(),
		IsLinkLocal:      parsedIP.IsLinkLocalUnicast(),
		IsIPv4MappedIPv6: isIPv4MappedIPv6(ip),
	}

	return result
}

// IPType IP 类型详细信息（修复 #8：新增）
type IPType struct {
	Valid            bool
	Version          int // 4 或 6
	IsPrivate        bool
	IsLoopback       bool
	IsMulticast      bool
	IsLinkLocal      bool
	IsIPv4MappedIPv6 bool
}

// IPTypeInvalid 无效 IP 的类型
var IPTypeInvalid = IPType{Valid: false}

// SanitizeIP 清理和验证 IP 地址，防止注入
func SanitizeIP(ip string) (string, bool) {
	// 去除空白
	ip = strings.TrimSpace(ip)

	// 检查是否包含危险字符
	if containsDangerousChars(ip) {
		return "", false
	}

	// 验证格式
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "", false
	}

	// 返回规范化的 IP
	return NormalizeIP(ip), true
}

// 危险字符正则（缓存以提高性能）
var dangerousCharsRegex = regexp.MustCompile(`[;'"\\<>(){}]|\-\-|/\*|\*/`)

// containsDangerousChars 检查是否包含危险字符
func containsDangerousChars(s string) bool {
	return dangerousCharsRegex.MatchString(s)
}

// ValidateIPList 验证 IP 列表
func ValidateIPList(ips []string) ([]string, []string) {
	validator := NewIPValidator()
	valid := make([]string, 0, len(ips))
	invalid := make([]string, 0)

	for _, ip := range ips {
		sanitized, ok := SanitizeIP(ip)
		if ok && validator.IsValidIP(sanitized) {
			valid = append(valid, sanitized)
		} else {
			invalid = append(invalid, ip)
		}
	}

	return valid, invalid
}

// IPRange IP 范围
type IPRange struct {
	Start net.IP
	End   net.IP
}

// ParseIPRange 解析 IP 范围（格式：start-end 或 CIDR）
func ParseIPRange(input string) (*IPRange, error) {
	// 尝试解析 CIDR
	if strings.Contains(input, "/") {
		_, ipNet, err := net.ParseCIDR(input)
		if err != nil {
			return nil, err
		}

		start := ipNet.IP
		end := make(net.IP, len(start))
		copy(end, start)

		for i := range ipNet.Mask {
			end[i] |= ^ipNet.Mask[i]
		}

		return &IPRange{Start: start, End: end}, nil
	}

	// 尝试解析范围格式
	if strings.Contains(input, "-") {
		parts := strings.SplitN(input, "-", 2)
		if len(parts) != 2 {
			return nil, &net.ParseError{Type: "IP range", Text: input}
		}

		start := net.ParseIP(strings.TrimSpace(parts[0]))
		end := net.ParseIP(strings.TrimSpace(parts[1]))

		if start == nil || end == nil {
			return nil, &net.ParseError{Type: "IP range", Text: input}
		}

		return &IPRange{Start: start, End: end}, nil
	}

	// 单个 IP
	ip := net.ParseIP(input)
	if ip == nil {
		return nil, &net.ParseError{Type: "IP address", Text: input}
	}

	return &IPRange{Start: ip, End: ip}, nil
}

// Contains 检查 IP 是否在范围内
func (r *IPRange) Contains(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	return bytesCompare(parsedIP, r.Start) >= 0 && bytesCompare(parsedIP, r.End) <= 0
}

// bytesCompare 比较两个 IP 的字节
func bytesCompare(a, b net.IP) int {
	// 规范化为相同长度
	if len(a) != len(b) {
		if a4 := a.To4(); a4 != nil {
			a = a4
		}
		if b4 := b.To4(); b4 != nil {
			b = b4
		}
	}

	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}

	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// ==================== 新增：IP 地址池管理 ====================

// IPPool IP 地址池（修复 #8：新增，用于管理白名单/黑名单）
type IPPool struct {
	ranges []*IPRange
}

// NewIPPool 创建 IP 地址池
func NewIPPool(cidrs []string) (*IPPool, error) {
	pool := &IPPool{
		ranges: make([]*IPRange, 0, len(cidrs)),
	}

	for _, cidr := range cidrs {
		r, err := ParseIPRange(cidr)
		if err != nil {
			return nil, err
		}
		pool.ranges = append(pool.ranges, r)
	}

	return pool, nil
}

// Contains 检查 IP 是否在池中
func (p *IPPool) Contains(ip string) bool {
	for _, r := range p.ranges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

// Add 添加 IP 范围
func (p *IPPool) Add(cidr string) error {
	r, err := ParseIPRange(cidr)
	if err != nil {
		return err
	}
	p.ranges = append(p.ranges, r)
	return nil
}

// ==================== 新增：常用 IP 范围预定义 ====================

var (
	// PrivateIPv4Ranges 私有 IPv4 范围
	PrivateIPv4Ranges = []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	// LoopbackRanges 回环地址范围
	LoopbackRanges = []string{
		"127.0.0.0/8",
		"::1/128",
	}

	// LinkLocalRanges 链路本地地址范围
	LinkLocalRanges = []string{
		"169.254.0.0/16",
		"fe80::/10",
	}

	// MulticastRanges 组播地址范围
	MulticastRanges = []string{
		"224.0.0.0/4",
		"ff00::/8",
	}
)

// IsInPrivateRange 检查是否在私有 IP 范围内（修复 #8：新增）
func IsInPrivateRange(ip string) bool {
	pool, _ := NewIPPool(PrivateIPv4Ranges)
	return pool.Contains(ip)
}

// IsInLoopbackRange 检查是否在回环范围内（修复 #8：新增）
func IsInLoopbackRange(ip string) bool {
	pool, _ := NewIPPool(LoopbackRanges)
	return pool.Contains(ip)
}

// ==================== 新增：IPv6 特殊格式支持 ====================

// ExpandIPv6 扩展 IPv6 地址为完整格式（修复 #8：新增）
// 例如：fe80::1 -> fe80:0000:0000:0000:0000:0000:0000:0001
func ExpandIPv6(ip string) string {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return ip
	}

	// 确保是 IPv6
	if parsedIP.To4() != nil && !isIPv4MappedIPv6(ip) {
		return ip
	}

	// 转换为 16 字节表示
	ipv6 := parsedIP.To16()
	if ipv6 == nil {
		return ip
	}

	// 格式化为完整的 IPv6 格式
	return fmt.Sprintf("%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x",
		ipv6[0], ipv6[1], ipv6[2], ipv6[3], ipv6[4], ipv6[5], ipv6[6], ipv6[7],
		ipv6[8], ipv6[9], ipv6[10], ipv6[11], ipv6[12], ipv6[13], ipv6[14], ipv6[15])
}

// CompressIPv6 压缩 IPv6 地址（修复 #8：新增）
// 例如：fe80:0000:0000:0000:0000:0000:0000:0001 -> fe80::1
func CompressIPv6(ip string) string {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return ip
	}

	// 确保是 IPv6
	if parsedIP.To4() != nil && !isIPv4MappedIPv6(ip) {
		return ip
	}

	return parsedIP.String()
}

// //////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/arkime/link_generator.go
// 修复版：添加 IP 地址验证、完善链接生成逻辑、防止注入攻击、修复端口处理边界情况
// 主要修复：
// 1. 完善端口号 0 的处理逻辑
// 2. 统一 ICMP 等无端口协议的处理
// 3. 增强安全验证
// //////////////////////////////////////////////////////////////////////////////
package arkime

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Config Arkime配置
type Config struct {
	BaseURL        string `env:"ARKIME_BASE_URL" envDefault:"http://arkime:8005"`
	SessionsPath   string `env:"ARKIME_SESSIONS_PATH" envDefault:"/sessions"`
	TimeBufferSecs int    `env:"ARKIME_TIME_BUFFER_SECS" envDefault:"60"`
}

// LinkGenerator Arkime链接生成器
type LinkGenerator struct {
	config Config
}

// NewLinkGenerator 创建链接生成器
func NewLinkGenerator(config Config) *LinkGenerator {
	// 确保BaseURL没有尾部斜杠
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	// 确保SessionsPath有前导斜杠
	if !strings.HasPrefix(config.SessionsPath, "/") {
		config.SessionsPath = "/" + config.SessionsPath
	}
	// 确保 TimeBufferSecs 有合理的默认值
	if config.TimeBufferSecs <= 0 {
		config.TimeBufferSecs = 60
	}
	return &LinkGenerator{
		config: config,
	}
}

// validateIP 验证 IP 地址格式，防止注入攻击
func validateIP(ip string) (string, bool) {
	if ip == "" {
		return "", false
	}
	// 去除空白字符
	ip = strings.TrimSpace(ip)
	// 检查是否包含危险字符
	if containsDangerousChars(ip) {
		return "", false
	}
	// 解析 IP 地址
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "", false
	}
	// 返回规范化的 IP 地址
	return parsedIP.String(), true
}

// validateCommunityID 验证 Community ID 格式
func validateCommunityID(communityID string) (string, bool) {
	if communityID == "" {
		return "", false
	}
	// 去除空白字符
	communityID = strings.TrimSpace(communityID)
	// Community ID 格式验证（通常是 1:hash 格式）
	// 允许的字符：数字、字母、冒号、加号、斜杠、等号
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9:+/=]+$`)
	if !validPattern.MatchString(communityID) {
		return "", false
	}
	// 检查是否包含危险字符
	if containsDangerousChars(communityID) {
		return "", false
	}
	return communityID, true
}

// validateSessionID 验证 Session ID 格式
func validateSessionID(sessionID string) (string, bool) {
	if sessionID == "" {
		return "", false
	}
	// 去除空白字符
	sessionID = strings.TrimSpace(sessionID)
	// Session ID 通常是 UUID 或类似格式
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`)
	if !validPattern.MatchString(sessionID) {
		return "", false
	}
	// 检查是否包含危险字符
	if containsDangerousChars(sessionID) {
		return "", false
	}
	return sessionID, true
}

// 危险字符正则表达式
var dangerousCharsRegex = regexp.MustCompile(`[;'"\\<>(){}]|\-\-|/\*|\*/|\|\||&&`)

// containsDangerousChars 检查是否包含可能用于注入的危险字符
func containsDangerousChars(s string) bool {
	return dangerousCharsRegex.MatchString(s)
}

// validatePort 验证端口号
func validatePort(port uint16) bool {
	return port > 0 && port <= 65535
}

// validateProtocol 验证协议号
func validateProtocol(protocol uint8) bool {
	// 常见协议：1=ICMP, 6=TCP, 17=UDP, 47=GRE, 50=ESP, 51=AH, 58=ICMPv6
	return protocol > 0
}

// getProtocolName 获取协议名称
func getProtocolName(protocol uint8) string {
	switch protocol {
	case 1:
		return "icmp"
	case 6:
		return "tcp"
	case 17:
		return "udp"
	case 47:
		return "gre"
	case 50:
		return "esp"
	case 51:
		return "ah"
	case 58:
		return "icmpv6"
	default:
		return fmt.Sprintf("%d", protocol)
	}
}

// isProtocolWithPorts 判断协议是否需要端口号
func isProtocolWithPorts(protocol uint8) bool {
	// 只有 TCP 和 UDP 需要端口号
	return protocol == 6 || protocol == 17
}

// GenerateSessionLink 生成会话查询链接
func (g *LinkGenerator) GenerateSessionLink(communityID string, startTime, endTime time.Time) string {
	// 验证 Community ID
	validCommunityID, ok := validateCommunityID(communityID)
	if !ok {
		return ""
	}

	// Arkime使用秒级时间戳
	startSecs := startTime.Unix() - int64(g.config.TimeBufferSecs)
	endSecs := endTime.Unix() + int64(g.config.TimeBufferSecs)

	// 确保时间范围合理
	if startSecs < 0 {
		startSecs = 0
	}
	if endSecs < startSecs {
		endSecs = startSecs + 3600 // 默认1小时范围
	}

	// 构建expression（使用双引号包裹值）
	expression := fmt.Sprintf(`community.id == "%s"`, validCommunityID)

	// 构建URL
	params := url.Values{}
	params.Set("expression", expression)
	params.Set("startTime", fmt.Sprintf("%d", startSecs))
	params.Set("stopTime", fmt.Sprintf("%d", endSecs))
	params.Set("view", "sessions")

	return fmt.Sprintf("%s%s?%s", g.config.BaseURL, g.config.SessionsPath, params.Encode())
}

// GenerateIPLink 生成IP查询链接
func (g *LinkGenerator) GenerateIPLink(ip string, startTime, endTime time.Time) string {
	// 验证 IP 地址
	validIP, ok := validateIP(ip)
	if !ok {
		return ""
	}

	startSecs := startTime.Unix() - int64(g.config.TimeBufferSecs)
	endSecs := endTime.Unix() + int64(g.config.TimeBufferSecs)

	if startSecs < 0 {
		startSecs = 0
	}
	if endSecs < startSecs {
		endSecs = startSecs + 3600
	}

	// 使用正确的Arkime字段名
	expression := fmt.Sprintf(`ip == %s`, validIP)

	params := url.Values{}
	params.Set("expression", expression)
	params.Set("startTime", fmt.Sprintf("%d", startSecs))
	params.Set("stopTime", fmt.Sprintf("%d", endSecs))
	params.Set("view", "sessions")

	return fmt.Sprintf("%s%s?%s", g.config.BaseURL, g.config.SessionsPath, params.Encode())
}

// GenerateTupleLink 生成五元组查询链接（修复版：完善端口处理）
func (g *LinkGenerator) GenerateTupleLink(srcIP, dstIP string, srcPort, dstPort uint16, protocol uint8, startTime, endTime time.Time) string {
	// 验证 IP 地址
	validSrcIP, srcOk := validateIP(srcIP)
	validDstIP, dstOk := validateIP(dstIP)
	if !srcOk || !dstOk {
		return ""
	}

	// 验证协议
	if !validateProtocol(protocol) {
		return ""
	}

	startSecs := startTime.Unix() - int64(g.config.TimeBufferSecs)
	endSecs := endTime.Unix() + int64(g.config.TimeBufferSecs)

	if startSecs < 0 {
		startSecs = 0
	}
	if endSecs < startSecs {
		endSecs = startSecs + 3600
	}

	// 构建expression（支持双向匹配）
	var expression string
	protocolName := getProtocolName(protocol)

	// 修复：根据协议类型正确处理端口
	switch {
	case isProtocolWithPorts(protocol):
		// TCP/UDP - 需要端口号
		if srcPort == 0 || dstPort == 0 {
			// 端口缺失或为0，仅匹配 IP 和协议
			expression = fmt.Sprintf(
				`((ip.src == %s && ip.dst == %s) || (ip.src == %s && ip.dst == %s)) && ip.protocol == %s`,
				validSrcIP, validDstIP,
				validDstIP, validSrcIP,
				protocolName,
			)
		} else {
			// 完整五元组匹配（双向）
			expression = fmt.Sprintf(
				`((ip.src == %s && ip.dst == %s && port.src == %d && port.dst == %d) || `+
					`(ip.src == %s && ip.dst == %s && port.src == %d && port.dst == %d)) && ip.protocol == %s`,
				validSrcIP, validDstIP, srcPort, dstPort,
				validDstIP, validSrcIP, dstPort, srcPort,
				protocolName,
			)
		}

	case protocol == 1 || protocol == 58:
		// ICMP/ICMPv6 - 忽略端口
		expression = fmt.Sprintf(
			`((ip.src == %s && ip.dst == %s) || (ip.src == %s && ip.dst == %s)) && ip.protocol == %s`,
			validSrcIP, validDstIP,
			validDstIP, validSrcIP,
			protocolName,
		)

	default:
		// 其他协议 - 只匹配 IP（不包含协议过滤，因为可能不支持）
		expression = fmt.Sprintf(
			`(ip.src == %s && ip.dst == %s) || (ip.src == %s && ip.dst == %s)`,
			validSrcIP, validDstIP,
			validDstIP, validSrcIP,
		)
	}

	params := url.Values{}
	params.Set("expression", expression)
	params.Set("startTime", fmt.Sprintf("%d", startSecs))
	params.Set("stopTime", fmt.Sprintf("%d", endSecs))
	params.Set("view", "sessions")

	return fmt.Sprintf("%s%s?%s", g.config.BaseURL, g.config.SessionsPath, params.Encode())
}

// GeneratePcapDownloadLink 生成PCAP下载链接
func (g *LinkGenerator) GeneratePcapDownloadLink(sessionID string) string {
	// 验证 Session ID
	validSessionID, ok := validateSessionID(sessionID)
	if !ok {
		return ""
	}

	return fmt.Sprintf("%s/sessions/%s/pcap", g.config.BaseURL, url.PathEscape(validSessionID))
}

// GenerateSPIViewLink 生成SPI视图链接
func (g *LinkGenerator) GenerateSPIViewLink(communityID string, startTime, endTime time.Time) string {
	// 验证 Community ID
	validCommunityID, ok := validateCommunityID(communityID)
	if !ok {
		return ""
	}

	startSecs := startTime.Unix() - int64(g.config.TimeBufferSecs)
	endSecs := endTime.Unix() + int64(g.config.TimeBufferSecs)

	if startSecs < 0 {
		startSecs = 0
	}
	if endSecs < startSecs {
		endSecs = startSecs + 3600
	}

	expression := fmt.Sprintf(`community.id == "%s"`, validCommunityID)

	params := url.Values{}
	params.Set("expression", expression)
	params.Set("startTime", fmt.Sprintf("%d", startSecs))
	params.Set("stopTime", fmt.Sprintf("%d", endSecs))

	return fmt.Sprintf("%s/spiview?%s", g.config.BaseURL, params.Encode())
}

// GenerateConnectionsLink 生成连接图链接
func (g *LinkGenerator) GenerateConnectionsLink(ip string, startTime, endTime time.Time) string {
	// 验证 IP 地址
	validIP, ok := validateIP(ip)
	if !ok {
		return ""
	}

	startSecs := startTime.Unix() - int64(g.config.TimeBufferSecs)
	endSecs := endTime.Unix() + int64(g.config.TimeBufferSecs)

	if startSecs < 0 {
		startSecs = 0
	}
	if endSecs < startSecs {
		endSecs = startSecs + 3600
	}

	expression := fmt.Sprintf(`ip == %s`, validIP)

	params := url.Values{}
	params.Set("expression", expression)
	params.Set("startTime", fmt.Sprintf("%d", startSecs))
	params.Set("stopTime", fmt.Sprintf("%d", endSecs))

	return fmt.Sprintf("%s/connections?%s", g.config.BaseURL, params.Encode())
}

// GeneratePortLink 生成端口查询链接
func (g *LinkGenerator) GeneratePortLink(port uint16, startTime, endTime time.Time) string {
	if !validatePort(port) {
		return ""
	}

	startSecs := startTime.Unix() - int64(g.config.TimeBufferSecs)
	endSecs := endTime.Unix() + int64(g.config.TimeBufferSecs)

	if startSecs < 0 {
		startSecs = 0
	}
	if endSecs < startSecs {
		endSecs = startSecs + 3600
	}

	expression := fmt.Sprintf(`port == %d`, port)

	params := url.Values{}
	params.Set("expression", expression)
	params.Set("startTime", fmt.Sprintf("%d", startSecs))
	params.Set("stopTime", fmt.Sprintf("%d", endSecs))
	params.Set("view", "sessions")

	return fmt.Sprintf("%s%s?%s", g.config.BaseURL, g.config.SessionsPath, params.Encode())
}

// GenerateProtocolLink 生成协议查询链接
func (g *LinkGenerator) GenerateProtocolLink(protocol uint8, startTime, endTime time.Time) string {
	if !validateProtocol(protocol) {
		return ""
	}

	startSecs := startTime.Unix() - int64(g.config.TimeBufferSecs)
	endSecs := endTime.Unix() + int64(g.config.TimeBufferSecs)

	if startSecs < 0 {
		startSecs = 0
	}
	if endSecs < startSecs {
		endSecs = startSecs + 3600
	}

	protocolName := getProtocolName(protocol)
	expression := fmt.Sprintf(`ip.protocol == %s`, protocolName)

	params := url.Values{}
	params.Set("expression", expression)
	params.Set("startTime", fmt.Sprintf("%d", startSecs))
	params.Set("stopTime", fmt.Sprintf("%d", endSecs))
	params.Set("view", "sessions")

	return fmt.Sprintf("%s%s?%s", g.config.BaseURL, g.config.SessionsPath, params.Encode())
}

// GenerateIPRangeLink 生成 IP 范围查询链接
func (g *LinkGenerator) GenerateIPRangeLink(cidr string, startTime, endTime time.Time) string {
	if cidr == "" {
		return ""
	}

	// 验证 CIDR 格式
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return ""
	}

	// 检查危险字符
	if containsDangerousChars(cidr) {
		return ""
	}

	startSecs := startTime.Unix() - int64(g.config.TimeBufferSecs)
	endSecs := endTime.Unix() + int64(g.config.TimeBufferSecs)

	if startSecs < 0 {
		startSecs = 0
	}
	if endSecs < startSecs {
		endSecs = startSecs + 3600
	}

	expression := fmt.Sprintf(`ip == %s`, cidr)

	params := url.Values{}
	params.Set("expression", expression)
	params.Set("startTime", fmt.Sprintf("%d", startSecs))
	params.Set("stopTime", fmt.Sprintf("%d", endSecs))
	params.Set("view", "sessions")

	return fmt.Sprintf("%s%s?%s", g.config.BaseURL, g.config.SessionsPath, params.Encode())
}

// AlertLinks 告警相关的所有链接
type AlertLinks struct {
	SessionLink     string `json:"session_link,omitempty"`
	SrcIPLink       string `json:"src_ip_link,omitempty"`
	DstIPLink       string `json:"dst_ip_link,omitempty"`
	ConnectionsLink string `json:"connections_link,omitempty"`
	SPIViewLink     string `json:"spi_view_link,omitempty"`
	SrcPortLink     string `json:"src_port_link,omitempty"`
	DstPortLink     string `json:"dst_port_link,omitempty"`
	ProtocolLink    string `json:"protocol_link,omitempty"`
	PcapLink        string `json:"pcap_link,omitempty"`
}

// GenerateAlertLinks 为告警生成所有相关链接（修复版：完善端口处理）
func (g *LinkGenerator) GenerateAlertLinks(
	communityID, srcIP, dstIP string,
	srcPort, dstPort uint16,
	protocol uint8,
	startTime, endTime time.Time,
) *AlertLinks {
	links := &AlertLinks{}

	// 验证并生成会话链接（主要）
	if validCommunityID, ok := validateCommunityID(communityID); ok && validCommunityID != "" {
		links.SessionLink = g.GenerateSessionLink(validCommunityID, startTime, endTime)
		links.SPIViewLink = g.GenerateSPIViewLink(validCommunityID, startTime, endTime)
	} else {
		// 如果没有community_id，使用五元组
		validSrcIP, srcOk := validateIP(srcIP)
		validDstIP, dstOk := validateIP(dstIP)
		if srcOk && dstOk {
			links.SessionLink = g.GenerateTupleLink(validSrcIP, validDstIP, srcPort, dstPort, protocol, startTime, endTime)
		}
	}

	// 验证并生成 IP 链接
	if validSrcIP, ok := validateIP(srcIP); ok {
		links.SrcIPLink = g.GenerateIPLink(validSrcIP, startTime, endTime)
		links.ConnectionsLink = g.GenerateConnectionsLink(validSrcIP, startTime, endTime)
	}

	if validDstIP, ok := validateIP(dstIP); ok {
		links.DstIPLink = g.GenerateIPLink(validDstIP, startTime, endTime)
	}

	// 修复：只为需要端口的协议生成端口链接
	if isProtocolWithPorts(protocol) {
		if validatePort(srcPort) {
			links.SrcPortLink = g.GeneratePortLink(srcPort, startTime, endTime)
		}
		if validatePort(dstPort) {
			links.DstPortLink = g.GeneratePortLink(dstPort, startTime, endTime)
		}
	}

	// 验证并生成协议链接
	if validateProtocol(protocol) {
		links.ProtocolLink = g.GenerateProtocolLink(protocol, startTime, endTime)
	}

	return links
}

// GenerateAlertLinksWithSession 为告警生成所有相关链接（包含 Session ID 用于 PCAP 下载）
func (g *LinkGenerator) GenerateAlertLinksWithSession(
	communityID, sessionID, srcIP, dstIP string,
	srcPort, dstPort uint16,
	protocol uint8,
	startTime, endTime time.Time,
) *AlertLinks {
	links := g.GenerateAlertLinks(communityID, srcIP, dstIP, srcPort, dstPort, protocol, startTime, endTime)

	// 添加 PCAP 下载链接
	if validSessionID, ok := validateSessionID(sessionID); ok {
		links.PcapLink = g.GeneratePcapDownloadLink(validSessionID)
	}

	return links
}

// IsValidIP 公开的 IP 验证方法
func IsValidIP(ip string) bool {
	_, ok := validateIP(ip)
	return ok
}

// IsValidCommunityID 公开的 Community ID 验证方法
func IsValidCommunityID(communityID string) bool {
	_, ok := validateCommunityID(communityID)
	return ok
}

// DefaultGenerator 默认生成器（使用环境变量配置）
var DefaultGenerator *LinkGenerator

// Init 初始化默认生成器
func Init(config Config) {
	DefaultGenerator = NewLinkGenerator(config)
}

// GenerateSessionLinkDefault 使用默认生成器生成会话链接
func GenerateSessionLinkDefault(communityID string, startTime, endTime time.Time) string {
	if DefaultGenerator == nil {
		return ""
	}
	return DefaultGenerator.GenerateSessionLink(communityID, startTime, endTime)
}

// GenerateAlertLinksDefault 使用默认生成器生成所有链接
func GenerateAlertLinksDefault(
	communityID, srcIP, dstIP string,
	srcPort, dstPort uint16,
	protocol uint8,
	startTime, endTime time.Time,
) *AlertLinks {
	if DefaultGenerator == nil {
		return &AlertLinks{}
	}
	return DefaultGenerator.GenerateAlertLinks(communityID, srcIP, dstIP, srcPort, dstPort, protocol, startTime, endTime)
}

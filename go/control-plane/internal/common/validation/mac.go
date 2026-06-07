// MAC 地址与主机名校验
package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// 规范 MAC 格式: xx:xx:xx:xx:xx:xx (6 组十六进制)
var macRegex = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)

// 主机名: 字母数字 + 连字符 + 点, 1-253 字符
var hostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)*$`)

// Vendor 名称: 可包含字母数字、空格、常见标点（用于 OUI 厂商名）
var vendorRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\s\-\.\,\&\(\)\+]*[a-zA-Z0-9\.\)\+]$`)

// IsValidMAC 验证 MAC 地址格式 (xx:xx:xx:xx:xx:xx)
func IsValidMAC(mac string) bool {
	if mac == "" {
		return false
	}
	return macRegex.MatchString(mac)
}

// NormalizeMAC 标准化 MAC 地址 (小写, 去空白)
func NormalizeMAC(mac string) (string, error) {
	mac = strings.TrimSpace(mac)
	mac = strings.ToLower(mac)
	if !IsValidMAC(mac) {
		return "", fmt.Errorf("invalid MAC address: %s", mac)
	}
	return mac, nil
}

// IsValidHostname 验证主机名 (RFC 952)
func IsValidHostname(hostname string) bool {
	if hostname == "" || len(hostname) > 253 {
		return false
	}
	// 每个标签不超过 63 字符
	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
	}
	return hostnameRegex.MatchString(hostname)
}

// IsValidVendor 验证厂商名称
func IsValidVendor(vendor string) bool {
	if vendor == "" || len(vendor) > 100 {
		return false
	}
	return vendorRegex.MatchString(vendor)
}

// OUI 前缀 (MAC 前 3 字节)
func OUIFromMAC(mac string) string {
	mac = strings.ToLower(strings.TrimSpace(mac))
	if !IsValidMAC(mac) {
		return ""
	}
	parts := strings.Split(mac, ":")
	if len(parts) < 3 {
		return ""
	}
	return parts[0] + ":" + parts[1] + ":" + parts[2]
}

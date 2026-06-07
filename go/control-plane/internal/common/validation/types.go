// 通用类型验证器: UUID, 端口, 时间戳, 枚举值
package validation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ==================== UUID 验证 ====================

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsValidUUID 验证字符串是否为有效的 UUID v4 格式
func IsValidUUID(s string) bool {
	if s == "" {
		return false
	}
	return uuidRegex.MatchString(s)
}

// NormalizeUUID 标准化 UUID (小写, 去空白)
func NormalizeUUID(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	if !IsValidUUID(s) {
		return "", fmt.Errorf("invalid UUID format: %s", s)
	}
	return s, nil
}

// ==================== 端口验证 ====================

// IsValidPort 验证端口号是否在有效范围 (1-65535)
func IsValidPort(port uint32) bool {
	return port >= 1 && port <= 65535
}

// IsValidPortOrZero 验证端口号 (允许 0 表示未指定/动态端口)
func IsValidPortOrZero(port uint32) bool {
	return port <= 65535
}

// IsValidPortInt 从整数验证端口号
func IsValidPortInt(port int) bool {
	return port >= 1 && port <= 65535
}

// ParsePort 解析端口字符串
func ParsePort(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty port string")
	}
	p, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid port number: %s", s)
	}
	if p == 0 {
		return 0, fmt.Errorf("port 0 is reserved")
	}
	return uint32(p), nil
}

// IsValidPortRange 验证端口范围
func IsValidPortRange(start, end uint32) bool {
	return IsValidPort(start) && IsValidPort(end) && start <= end
}

// ==================== 时间戳验证 ====================

const (
	MinTimestampMs int64 = 946684800000  // 2000-01-01
	MaxTimestampMs int64 = 4102444800000 // 2100-01-01
)

// IsValidTimestampMs 验证毫秒时间戳是否在合理范围 (2000-2100)
func IsValidTimestampMs(ts int64) bool {
	return ts >= MinTimestampMs && ts <= MaxTimestampMs
}

// IsValidTimestampSec 验证秒级时间戳
func IsValidTimestampSec(ts int64) bool {
	return IsValidTimestampMs(ts * 1000)
}

// IsFutureTimestamp 验证时间戳是否在未来
func IsFutureTimestamp(ts int64) bool {
	return ts > 0
}

// ==================== 枚举验证 ====================

// IsValidEnum 检查值是否在有效枚举集合中
func IsValidEnum(value string, validValues []string) bool {
	if value == "" {
		return false
	}
	for _, v := range validValues {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}

// IsValidEnumExact 检查值是否在有效枚举集合中 (大小写敏感)
func IsValidEnumExact(value string, validValues []string) bool {
	for _, v := range validValues {
		if v == value {
			return true
		}
	}
	return false
}

// ==================== 长度验证 ====================

// IsValidStringLength 验证字符串长度在 [min, max] 范围内
func IsValidStringLength(s string, minLen, maxLen int) bool {
	l := len(s)
	return l >= minLen && l <= maxLen
}

// IsValidTenantID 验证租户ID格式: 字母数字 + 下划线/连字符, 1-64 字符
var tenantIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func IsValidTenantID(id string) bool {
	return tenantIDRegex.MatchString(id)
}

// ==================== 组合验证器 ====================

// ValidationError 验证错误列表
type ValidationErrors []string

func (e ValidationErrors) Error() string {
	return fmt.Sprintf("validation errors: %s", strings.Join(e, "; "))
}

// ValidateRequired 验证必填字段
func ValidateRequired(fields map[string]string) ValidationErrors {
	var errs ValidationErrors
	for name, value := range fields {
		if strings.TrimSpace(value) == "" {
			errs = append(errs, fmt.Sprintf("%s is required", name))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

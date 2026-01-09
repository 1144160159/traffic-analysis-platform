////////////////////////////////////////////////////////////////////////////////
// FILE PATH: internal/auth/security/cors.go
// CORS 配置验证和安全检查
////////////////////////////////////////////////////////////////////////////////

package security

import (
	"net/url"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

// ValidateCORSConfig 验证 CORS 配置的安全性
func ValidateCORSConfig(config *httpx.CORSConfig) error {
	if config == nil {
		return errors.New(errors.ErrCodeConfigError, "CORS config is nil")
	}

	// 检查通配符与凭证的兼容性
	hasWildcard := false
	for _, origin := range config.AllowedOrigins {
		if origin == "*" {
			hasWildcard = true
			break
		}
	}

	if hasWildcard && config.AllowCredentials {
		return errors.New(errors.ErrCodeConfigError,
			"CORS: AllowedOrigins cannot contain '*' when AllowCredentials is true")
	}

	// 验证每个 origin 格式
	for _, origin := range config.AllowedOrigins {
		if origin == "*" {
			continue // 通配符是合法的
		}

		if err := validateOrigin(origin); err != nil {
			return errors.Wrapf(err, errors.ErrCodeConfigError,
				"Invalid CORS origin: %s", origin)
		}
	}

	// 验证暴露的头部
	if len(config.ExposedHeaders) > 0 {
		for _, header := range config.ExposedHeaders {
			if header == "*" {
				return errors.New(errors.ErrCodeConfigError,
					"CORS: ExposedHeaders cannot contain '*'")
			}
		}
	}

	return nil
}

// validateOrigin 验证单个 origin 格式
func validateOrigin(origin string) error {
	// 空字符串不合法
	if origin == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "Origin cannot be empty")
	}

	// 解析 URL
	u, err := url.Parse(origin)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInvalidParameter, "Invalid origin URL")
	}

	// 必须包含 scheme
	if u.Scheme == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "Origin must include scheme (http:// or https://)")
	}

	// 只允许 http 和 https
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid origin scheme: %s (only http and https are allowed)", u.Scheme)
	}

	// 必须包含 host
	if u.Host == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "Origin must include host")
	}

	// 不应包含 path、query、fragment
	if u.Path != "" && u.Path != "/" {
		return errors.New(errors.ErrCodeInvalidParameter, "Origin should not include path")
	}
	if u.RawQuery != "" {
		return errors.New(errors.ErrCodeInvalidParameter, "Origin should not include query")
	}
	if u.Fragment != "" {
		return errors.New(errors.ErrCodeInvalidParameter, "Origin should not include fragment")
	}

	return nil
}

// SanitizeCORSConfig 清理和规范化 CORS 配置
func SanitizeCORSConfig(config *httpx.CORSConfig) *httpx.CORSConfig {
	if config == nil {
		return httpx.DefaultCORSConfig()
	}

	sanitized := &httpx.CORSConfig{
		AllowedOrigins:   sanitizeOrigins(config.AllowedOrigins),
		AllowedMethods:   sanitizeMethods(config.AllowedMethods),
		AllowedHeaders:   sanitizeHeaders(config.AllowedHeaders),
		ExposedHeaders:   sanitizeHeaders(config.ExposedHeaders),
		AllowCredentials: config.AllowCredentials,
		MaxAge:           config.MaxAge,
	}

	// 如果包含通配符，禁用凭证
	for _, origin := range sanitized.AllowedOrigins {
		if origin == "*" {
			sanitized.AllowCredentials = false
			break
		}
	}

	return sanitized
}

// sanitizeOrigins 清理 origin 列表
func sanitizeOrigins(origins []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(origins))

	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}

		// 去重
		if seen[origin] {
			continue
		}
		seen[origin] = true

		// 规范化
		if origin != "*" {
			// 移除尾部斜杠
			origin = strings.TrimSuffix(origin, "/")

			// 如果没有 scheme，添加 https
			if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
				origin = "https://" + origin
			}
		}

		result = append(result, origin)
	}

	if len(result) == 0 {
		return []string{"*"} // 默认允许所有
	}

	return result
}

// sanitizeMethods 清理 HTTP 方法列表
func sanitizeMethods(methods []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(methods))

	for _, method := range methods {
		method = strings.ToUpper(strings.TrimSpace(method))
		if method == "" {
			continue
		}

		if seen[method] {
			continue
		}
		seen[method] = true

		result = append(result, method)
	}

	if len(result) == 0 {
		return []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	}

	return result
}

// sanitizeHeaders 清理 HTTP 头部列表
func sanitizeHeaders(headers []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(headers))

	for _, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}

		// 规范化为 Title-Case
		header = canonicalizeHeader(header)

		if seen[header] {
			continue
		}
		seen[header] = true

		result = append(result, header)
	}

	return result
}

// canonicalizeHeader 规范化 HTTP 头部名称
func canonicalizeHeader(header string) string {
	parts := strings.Split(header, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, "-")
}

// IsSafeOrigin 检查 origin 是否安全
func IsSafeOrigin(origin string) bool {
	// localhost 和 127.0.0.1 只在开发环境允许
	if strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1") {
		return false // 生产环境应该禁止
	}

	// 必须使用 HTTPS（除了 localhost）
	if !strings.HasPrefix(origin, "https://") {
		return false
	}

	return true
}

// GetRecommendedCORSConfig 获取推荐的生产环境 CORS 配置
func GetRecommendedCORSConfig(allowedDomains []string) *httpx.CORSConfig {
	origins := make([]string, 0, len(allowedDomains))
	for _, domain := range allowedDomains {
		if domain != "" {
			origins = append(origins, "https://"+domain)
		}
	}

	if len(origins) == 0 {
		// 如果没有指定域名，使用严格模式（不允许任何跨域）
		origins = []string{"null"}
	}

	return &httpx.CORSConfig{
		AllowedOrigins: origins,
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-Tenant-ID",
			"X-Request-ID",
			"X-Trace-ID",
		},
		ExposedHeaders: []string{
			"X-Request-ID",
			"X-Trace-ID",
		},
		AllowCredentials: true,
		MaxAge:           86400, // 24 hours
	}
}

// GetDevelopmentCORSConfig 获取开发环境 CORS 配置（宽松）
func GetDevelopmentCORSConfig() *httpx.CORSConfig {
	return &httpx.CORSConfig{
		AllowedOrigins: []string{
			"http://localhost:3000",
			"http://localhost:8080",
			"http://127.0.0.1:3000",
			"http://127.0.0.1:8080",
		},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders: []string{"*"},
		ExposedHeaders: []string{
			"X-Request-ID",
			"X-Trace-ID",
		},
		AllowCredentials: true,
		MaxAge:           3600,
	}
}

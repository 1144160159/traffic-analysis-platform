////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/httpx/body_limit.go
// 新增文件：修复 #9 - 请求体大小限制中间件
// 功能：
// 1. 限制 HTTP 请求体大小，防止大文件攻击
// 2. 支持按 Content-Type 设置不同的限制
// 3. 支持按路径模式设置不同的限制
// 4. 提供友好的错误提示
////////////////////////////////////////////////////////////////////////////////

package httpx

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"go.uber.org/zap"
)

const (
	// 默认限制大小
	DefaultMaxBodySize = 10 * 1024 * 1024 // 10MB

	// 常见 Content-Type 的推荐限制
	JSONMaxSize      = 10 * 1024 * 1024  // 10MB
	ProtobufMaxSize  = 50 * 1024 * 1024  // 50MB（Protobuf 通常更紧凑）
	MultipartMaxSize = 100 * 1024 * 1024 // 100MB（文件上传）
	PlainTextMaxSize = 1 * 1024 * 1024   // 1MB
)

// BodyLimitConfig 请求体限制配置
type BodyLimitConfig struct {
	// 默认最大大小（字节）
	DefaultMaxSize int64

	// 按 Content-Type 设置不同的限制
	// 示例：{"application/json": 10MB, "application/x-protobuf": 50MB}
	SizeByContentType map[string]int64

	// 按路径模式设置不同的限制
	// 示例：{"/api/upload": 100MB, "/api/alerts": 1MB}
	SizeByPath map[string]int64

	// 路径正则表达式匹配（优先级高于 SizeByPath）
	SizeByPathRegex map[*regexp.Regexp]int64

	// 是否在超限时返回 413 Payload Too Large（默认 true）
	Return413OnExceed bool

	// 是否记录超限日志
	LogExceeded bool

	// 日志记录器
	Logger *zap.Logger
}

// DefaultBodyLimitConfig 默认配置
func DefaultBodyLimitConfig() *BodyLimitConfig {
	return &BodyLimitConfig{
		DefaultMaxSize: DefaultMaxBodySize,
		SizeByContentType: map[string]int64{
			"application/json":       JSONMaxSize,
			"application/x-protobuf": ProtobufMaxSize,
			"application/protobuf":   ProtobufMaxSize,
			"multipart/form-data":    MultipartMaxSize,
			"text/plain":             PlainTextMaxSize,
		},
		SizeByPath:        make(map[string]int64),
		SizeByPathRegex:   make(map[*regexp.Regexp]int64),
		Return413OnExceed: true,
		LogExceeded:       true,
	}
}

// BodyLimit 请求体大小限制中间件
func BodyLimit(maxSize int64) Middleware {
	cfg := &BodyLimitConfig{
		DefaultMaxSize:    maxSize,
		Return413OnExceed: true,
		LogExceeded:       true,
	}
	return BodyLimitWithConfig(cfg)
}

// BodyLimitWithConfig 使用配置创建请求体大小限制中间件
func BodyLimitWithConfig(cfg *BodyLimitConfig) Middleware {
	if cfg == nil {
		cfg = DefaultBodyLimitConfig()
	}

	// 设置默认值
	if cfg.DefaultMaxSize <= 0 {
		cfg.DefaultMaxSize = DefaultMaxBodySize
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 跳过不需要检查请求体的方法
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// 确定当前请求的限制大小
			maxSize := determineMaxSize(r, cfg)

			// 如果没有请求体或 Content-Length 为 0，跳过
			if r.ContentLength == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// 检查 Content-Length（快速失败）
			if r.ContentLength > 0 && r.ContentLength > maxSize {
				handleExceeded(w, r, r.ContentLength, maxSize, cfg)
				return
			}

			// 包装 Body 以限制读取大小
			r.Body = &limitedReadCloser{
				rc:       r.Body,
				maxSize:  maxSize,
				exceeded: false,
			}

			// 处理请求
			next.ServeHTTP(w, r)

			// 检查是否在处理过程中超限
			if lrc, ok := r.Body.(*limitedReadCloser); ok && lrc.exceeded {
				if cfg.LogExceeded && cfg.Logger != nil {
					cfg.Logger.Warn("Request body size exceeded during processing",
						zap.String("path", r.URL.Path),
						zap.String("method", r.Method),
						zap.Int64("max_size", maxSize),
						zap.String("client_ip", GetClientIP(r)))
				}
			}
		})
	}
}

// determineMaxSize 确定当前请求的最大大小
func determineMaxSize(r *http.Request, cfg *BodyLimitConfig) int64 {
	// 1. 优先检查路径正则匹配
	if len(cfg.SizeByPathRegex) > 0 {
		for regex, size := range cfg.SizeByPathRegex {
			if regex.MatchString(r.URL.Path) {
				return size
			}
		}
	}

	// 2. 检查精确路径匹配
	if len(cfg.SizeByPath) > 0 {
		if size, ok := cfg.SizeByPath[r.URL.Path]; ok {
			return size
		}
	}

	// 3. 检查 Content-Type
	if len(cfg.SizeByContentType) > 0 {
		contentType := r.Header.Get("Content-Type")
		// 去除 charset 等参数
		if idx := strings.Index(contentType, ";"); idx > 0 {
			contentType = strings.TrimSpace(contentType[:idx])
		}

		if size, ok := cfg.SizeByContentType[contentType]; ok {
			return size
		}
	}

	// 4. 返回默认值
	return cfg.DefaultMaxSize
}

// handleExceeded 处理超限情况
func handleExceeded(w http.ResponseWriter, r *http.Request, actualSize, maxSize int64, cfg *BodyLimitConfig) {
	if cfg.LogExceeded && cfg.Logger != nil {
		cfg.Logger.Warn("Request body size exceeded",
			zap.String("path", r.URL.Path),
			zap.String("method", r.Method),
			zap.Int64("actual_size", actualSize),
			zap.Int64("max_size", maxSize),
			zap.String("content_type", r.Header.Get("Content-Type")),
			zap.String("client_ip", GetClientIP(r)))
	}

	statusCode := http.StatusBadRequest
	if cfg.Return413OnExceed {
		statusCode = http.StatusRequestEntityTooLarge
	}

	err := errors.Newf(
		errors.ErrCodeInvalidRequest,
		"Request body too large: %s (max: %s)",
		formatSize(actualSize),
		formatSize(maxSize),
	)

	errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
	w.WriteHeader(statusCode)
}

// limitedReadCloser 限制读取大小的 ReadCloser
type limitedReadCloser struct {
	rc       io.ReadCloser
	maxSize  int64
	readSize int64
	exceeded bool
}

func (lrc *limitedReadCloser) Read(p []byte) (n int, err error) {
	// 检查是否已经超限
	if lrc.exceeded {
		return 0, fmt.Errorf("request body size exceeded")
	}

	// 读取数据
	n, err = lrc.rc.Read(p)
	lrc.readSize += int64(n)

	// 检查是否超限
	if lrc.readSize > lrc.maxSize {
		lrc.exceeded = true
		return n, fmt.Errorf("request body size exceeded: read %d bytes (max: %d)",
			lrc.readSize, lrc.maxSize)
	}

	return n, err
}

func (lrc *limitedReadCloser) Close() error {
	return lrc.rc.Close()
}

// formatSize 格式化大小（字节转为人类可读格式）
func formatSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}

// WithPathLimit 为特定路径设置限制（辅助函数）
func (cfg *BodyLimitConfig) WithPathLimit(path string, maxSize int64) *BodyLimitConfig {
	if cfg.SizeByPath == nil {
		cfg.SizeByPath = make(map[string]int64)
	}
	cfg.SizeByPath[path] = maxSize
	return cfg
}

// WithPathRegexLimit 为匹配正则的路径设置限制（辅助函数）
func (cfg *BodyLimitConfig) WithPathRegexLimit(pattern string, maxSize int64) *BodyLimitConfig {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		// 忽略无效的正则表达式
		return cfg
	}

	if cfg.SizeByPathRegex == nil {
		cfg.SizeByPathRegex = make(map[*regexp.Regexp]int64)
	}
	cfg.SizeByPathRegex[regex] = maxSize
	return cfg
}

// WithContentTypeLimit 为特定 Content-Type 设置限制（辅助函数）
func (cfg *BodyLimitConfig) WithContentTypeLimit(contentType string, maxSize int64) *BodyLimitConfig {
	if cfg.SizeByContentType == nil {
		cfg.SizeByContentType = make(map[string]int64)
	}
	cfg.SizeByContentType[contentType] = maxSize
	return cfg
}

// ==================== 预设配置 ====================

// StrictBodyLimitConfig 严格限制配置（适用于生产环境）
func StrictBodyLimitConfig() *BodyLimitConfig {
	return &BodyLimitConfig{
		DefaultMaxSize: 5 * 1024 * 1024, // 5MB
		SizeByContentType: map[string]int64{
			"application/json":       5 * 1024 * 1024,  // 5MB
			"application/x-protobuf": 20 * 1024 * 1024, // 20MB
			"application/protobuf":   20 * 1024 * 1024, // 20MB
			"multipart/form-data":    50 * 1024 * 1024, // 50MB
			"text/plain":             512 * 1024,       // 512KB
		},
		SizeByPath:        make(map[string]int64),
		SizeByPathRegex:   make(map[*regexp.Regexp]int64),
		Return413OnExceed: true,
		LogExceeded:       true,
	}
}

// RelaxedBodyLimitConfig 宽松限制配置（适用于开发环境）
func RelaxedBodyLimitConfig() *BodyLimitConfig {
	return &BodyLimitConfig{
		DefaultMaxSize: 50 * 1024 * 1024, // 50MB
		SizeByContentType: map[string]int64{
			"application/json":       50 * 1024 * 1024,  // 50MB
			"application/x-protobuf": 100 * 1024 * 1024, // 100MB
			"application/protobuf":   100 * 1024 * 1024, // 100MB
			"multipart/form-data":    500 * 1024 * 1024, // 500MB
			"text/plain":             10 * 1024 * 1024,  // 10MB
		},
		SizeByPath:        make(map[string]int64),
		SizeByPathRegex:   make(map[*regexp.Regexp]int64),
		Return413OnExceed: true,
		LogExceeded:       true,
	}
}

// TrafficAnalysisPlatformBodyLimitConfig 针对本项目的推荐配置
func TrafficAnalysisPlatformBodyLimitConfig() *BodyLimitConfig {
	cfg := &BodyLimitConfig{
		DefaultMaxSize: 10 * 1024 * 1024, // 10MB
		SizeByContentType: map[string]int64{
			"application/json":       10 * 1024 * 1024,  // 10MB（告警查询等）
			"application/x-protobuf": 50 * 1024 * 1024,  // 50MB（Flow 批量上报）
			"application/protobuf":   50 * 1024 * 1024,  // 50MB
			"multipart/form-data":    100 * 1024 * 1024, // 100MB（PCAP 上传）
		},
		SizeByPath: map[string]int64{
			"/api/v1/ingest/flows": 100 * 1024 * 1024, // 100MB（Flow 批量上报）
			"/api/v1/pcap/upload":  500 * 1024 * 1024, // 500MB（PCAP 上传）
		},
		SizeByPathRegex:   make(map[*regexp.Regexp]int64),
		Return413OnExceed: true,
		LogExceeded:       true,
	}

	// 添加正则匹配：所有告警相关的 API 限制为 5MB
	alertRegex, _ := regexp.Compile(`^/api/v\d+/alerts/.*`)
	cfg.SizeByPathRegex[alertRegex] = 5 * 1024 * 1024

	// 图查询 API 限制为 2MB（防止复杂查询）
	graphRegex, _ := regexp.Compile(`^/api/v\d+/graph/.*`)
	cfg.SizeByPathRegex[graphRegex] = 2 * 1024 * 1024

	return cfg
}

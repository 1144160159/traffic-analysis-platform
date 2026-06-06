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
	DefaultMaxBodySize = 10 * 1024 * 1024

	JSONMaxSize      = 10 * 1024 * 1024
	ProtobufMaxSize  = 50 * 1024 * 1024
	MultipartMaxSize = 100 * 1024 * 1024
	PlainTextMaxSize = 1 * 1024 * 1024
)

type BodyLimitConfig struct {
	DefaultMaxSize int64

	SizeByContentType map[string]int64

	SizeByPath map[string]int64

	SizeByPathRegex map[*regexp.Regexp]int64

	Return413OnExceed bool

	LogExceeded bool

	Logger *zap.Logger
}

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

func BodyLimit(maxSize int64) Middleware {
	cfg := &BodyLimitConfig{
		DefaultMaxSize:    maxSize,
		Return413OnExceed: true,
		LogExceeded:       true,
	}
	return BodyLimitWithConfig(cfg)
}

func BodyLimitWithConfig(cfg *BodyLimitConfig) Middleware {
	if cfg == nil {
		cfg = DefaultBodyLimitConfig()
	}

	if cfg.DefaultMaxSize <= 0 {
		cfg.DefaultMaxSize = DefaultMaxBodySize
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			maxSize := determineMaxSize(r, cfg)

			if r.ContentLength == 0 {
				next.ServeHTTP(w, r)
				return
			}

			if r.ContentLength > 0 && r.ContentLength > maxSize {
				handleExceeded(w, r, r.ContentLength, maxSize, cfg)
				return
			}

			r.Body = &limitedReadCloser{
				rc:       r.Body,
				maxSize:  maxSize,
				exceeded: false,
			}

			next.ServeHTTP(w, r)

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

func determineMaxSize(r *http.Request, cfg *BodyLimitConfig) int64 {

	if len(cfg.SizeByPathRegex) > 0 {
		for regex, size := range cfg.SizeByPathRegex {
			if regex.MatchString(r.URL.Path) {
				return size
			}
		}
	}

	if len(cfg.SizeByPath) > 0 {
		if size, ok := cfg.SizeByPath[r.URL.Path]; ok {
			return size
		}
	}

	if len(cfg.SizeByContentType) > 0 {
		contentType := r.Header.Get("Content-Type")

		if idx := strings.Index(contentType, ";"); idx > 0 {
			contentType = strings.TrimSpace(contentType[:idx])
		}

		if size, ok := cfg.SizeByContentType[contentType]; ok {
			return size
		}
	}

	return cfg.DefaultMaxSize
}

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

type limitedReadCloser struct {
	rc       io.ReadCloser
	maxSize  int64
	readSize int64
	exceeded bool
}

func (lrc *limitedReadCloser) Read(p []byte) (n int, err error) {

	if lrc.exceeded {
		return 0, fmt.Errorf("request body size exceeded")
	}

	n, err = lrc.rc.Read(p)
	lrc.readSize += int64(n)

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

func (cfg *BodyLimitConfig) WithPathLimit(path string, maxSize int64) *BodyLimitConfig {
	if cfg.SizeByPath == nil {
		cfg.SizeByPath = make(map[string]int64)
	}
	cfg.SizeByPath[path] = maxSize
	return cfg
}

func (cfg *BodyLimitConfig) WithPathRegexLimit(pattern string, maxSize int64) *BodyLimitConfig {
	regex, err := regexp.Compile(pattern)
	if err != nil {

		return cfg
	}

	if cfg.SizeByPathRegex == nil {
		cfg.SizeByPathRegex = make(map[*regexp.Regexp]int64)
	}
	cfg.SizeByPathRegex[regex] = maxSize
	return cfg
}

func (cfg *BodyLimitConfig) WithContentTypeLimit(contentType string, maxSize int64) *BodyLimitConfig {
	if cfg.SizeByContentType == nil {
		cfg.SizeByContentType = make(map[string]int64)
	}
	cfg.SizeByContentType[contentType] = maxSize
	return cfg
}

func StrictBodyLimitConfig() *BodyLimitConfig {
	return &BodyLimitConfig{
		DefaultMaxSize: 5 * 1024 * 1024,
		SizeByContentType: map[string]int64{
			"application/json":       5 * 1024 * 1024,
			"application/x-protobuf": 20 * 1024 * 1024,
			"application/protobuf":   20 * 1024 * 1024,
			"multipart/form-data":    50 * 1024 * 1024,
			"text/plain":             512 * 1024,
		},
		SizeByPath:        make(map[string]int64),
		SizeByPathRegex:   make(map[*regexp.Regexp]int64),
		Return413OnExceed: true,
		LogExceeded:       true,
	}
}

func RelaxedBodyLimitConfig() *BodyLimitConfig {
	return &BodyLimitConfig{
		DefaultMaxSize: 50 * 1024 * 1024,
		SizeByContentType: map[string]int64{
			"application/json":       50 * 1024 * 1024,
			"application/x-protobuf": 100 * 1024 * 1024,
			"application/protobuf":   100 * 1024 * 1024,
			"multipart/form-data":    500 * 1024 * 1024,
			"text/plain":             10 * 1024 * 1024,
		},
		SizeByPath:        make(map[string]int64),
		SizeByPathRegex:   make(map[*regexp.Regexp]int64),
		Return413OnExceed: true,
		LogExceeded:       true,
	}
}

func TrafficAnalysisPlatformBodyLimitConfig() *BodyLimitConfig {
	cfg := &BodyLimitConfig{
		DefaultMaxSize: 10 * 1024 * 1024,
		SizeByContentType: map[string]int64{
			"application/json":       10 * 1024 * 1024,
			"application/x-protobuf": 50 * 1024 * 1024,
			"application/protobuf":   50 * 1024 * 1024,
			"multipart/form-data":    100 * 1024 * 1024,
		},
		SizeByPath: map[string]int64{
			"/api/v1/ingest/flows": 100 * 1024 * 1024,
			"/api/v1/pcap/upload":  500 * 1024 * 1024,
		},
		SizeByPathRegex:   make(map[*regexp.Regexp]int64),
		Return413OnExceed: true,
		LogExceeded:       true,
	}

	alertRegex, _ := regexp.Compile(`^/api/v\d+/alerts/.*`)
	cfg.SizeByPathRegex[alertRegex] = 5 * 1024 * 1024

	graphRegex, _ := regexp.Compile(`^/api/v\d+/graph/.*`)
	cfg.SizeByPathRegex[graphRegex] = 2 * 1024 * 1024

	return cfg
}

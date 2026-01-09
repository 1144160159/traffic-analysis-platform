package httpx

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
)

// responseWriter 包装的ResponseWriter，用于捕获状态码和响应大小
type responseWriter struct {
	http.ResponseWriter
	statusCode    int
	bytesWritten  int
	headerWritten bool
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.headerWritten {
		rw.statusCode = code
		rw.headerWritten = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.headerWritten {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// Flush 实现Flusher接口
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Logging 请求日志中间件
func Logging(logger *zap.Logger) Middleware {
	if logger == nil {
		logger = zap.L()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// 包装ResponseWriter
			rw := newResponseWriter(w)

			// 获取请求信息
			requestID := GetRequestID(r.Context())
			traceID := GetTraceID(r.Context())
			tenantID := r.Header.Get("X-Tenant-ID")
			if tenantID == "" {
				tenantID = GetTenantID(r.Context())
			}

			// 处理请求
			next.ServeHTTP(rw, r)

			// 计算耗时
			duration := time.Since(start)

			// 记录日志
			fields := []zap.Field{
				zap.String(logging.FieldMethod, r.Method),
				zap.String(logging.FieldPath, r.URL.Path),
				zap.Int(logging.FieldStatus, rw.statusCode),
				zap.Float64(logging.FieldLatency, float64(duration.Milliseconds())),
				zap.Int("bytes", rw.bytesWritten),
				zap.String(logging.FieldClientIP, GetClientIP(r)),
			}

			if requestID != "" {
				fields = append(fields, zap.String(logging.FieldRequestID, requestID))
			}
			if traceID != "" {
				fields = append(fields, zap.String(logging.FieldTraceID, traceID))
			}
			if tenantID != "" {
				fields = append(fields, zap.String(logging.FieldTenantID, tenantID))
			}
			if r.Header.Get("User-Agent") != "" {
				fields = append(fields, zap.String(logging.FieldUserAgent, r.Header.Get("User-Agent")))
			}

			// 根据状态码选择日志级别
			switch {
			case rw.statusCode >= 500:
				logger.Error("Request completed with server error", fields...)
			case rw.statusCode >= 400:
				logger.Warn("Request completed with client error", fields...)
			default:
				logger.Info("Request completed", fields...)
			}
		})
	}
}

// LoggingWithBody 带请求体记录的日志中间件（慎用，仅用于调试）
func LoggingWithBody(logger *zap.Logger, maxBodySize int) Middleware {
	if logger == nil {
		logger = zap.L()
	}
	if maxBodySize <= 0 {
		maxBodySize = 1024 // 默认最大1KB
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// 读取请求体
			var requestBody []byte
			if r.Body != nil && r.ContentLength > 0 && r.ContentLength <= int64(maxBodySize) {
				requestBody, _ = io.ReadAll(io.LimitReader(r.Body, int64(maxBodySize)))
				r.Body = io.NopCloser(bytes.NewBuffer(requestBody))
			}

			rw := newResponseWriter(w)
			next.ServeHTTP(rw, r)

			duration := time.Since(start)

			fields := []zap.Field{
				zap.String(logging.FieldMethod, r.Method),
				zap.String(logging.FieldPath, r.URL.Path),
				zap.Int(logging.FieldStatus, rw.statusCode),
				zap.Float64(logging.FieldLatency, float64(duration.Milliseconds())),
				zap.String(logging.FieldClientIP, GetClientIP(r)),
			}

			if len(requestBody) > 0 {
				fields = append(fields, zap.ByteString("request_body", requestBody))
			}

			logger.Debug("Request with body", fields...)
		})
	}
}

// SkipPaths 跳过指定路径的日志记录
func SkipPaths(logger *zap.Logger, skipPaths ...string) Middleware {
	skipMap := make(map[string]bool)
	for _, path := range skipPaths {
		skipMap[path] = true
	}

	loggingMiddleware := Logging(logger)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skipMap[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			loggingMiddleware(next).ServeHTTP(w, r)
		})
	}
}

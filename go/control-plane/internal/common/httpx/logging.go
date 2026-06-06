package httpx

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
)

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

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func Logging(logger *zap.Logger) Middleware {
	if logger == nil {
		logger = zap.L()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rw := newResponseWriter(w)

			requestID := GetRequestID(r.Context())
			traceID := GetTraceID(r.Context())
			tenantID := r.Header.Get("X-Tenant-ID")
			if tenantID == "" {
				tenantID = GetTenantID(r.Context())
			}

			next.ServeHTTP(rw, r)

			duration := time.Since(start)

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

func LoggingWithBody(logger *zap.Logger, maxBodySize int) Middleware {
	if logger == nil {
		logger = zap.L()
	}
	if maxBodySize <= 0 {
		maxBodySize = 1024
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

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

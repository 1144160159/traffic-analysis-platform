package httpx

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
)

const (
	HeaderRequestID = "X-Request-ID"
	HeaderTraceID   = "X-Trace-ID"
	HeaderSpanID    = "X-Span-ID"
)

// RequestID 请求ID中间件
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 获取或生成Request ID
			requestID := r.Header.Get(HeaderRequestID)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// 获取Trace ID（可能由网关或上游服务设置）
			traceID := r.Header.Get(HeaderTraceID)
			if traceID == "" {
				traceID = requestID // 如果没有trace ID，使用request ID
			}

			// 获取Span ID
			spanID := r.Header.Get(HeaderSpanID)
			if spanID == "" {
				spanID = uuid.New().String()[:8]
			}

			// 设置响应头
			w.Header().Set(HeaderRequestID, requestID)
			w.Header().Set(HeaderTraceID, traceID)

			// 注入到Context
			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextKeyRequestID, requestID)
			ctx = context.WithValue(ctx, ContextKeyTraceID, traceID)

			// 同时注入到logging context
			ctx = logging.WithRequestID(ctx, requestID)
			ctx = logging.WithTraceID(ctx, traceID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestIDFromHeader 从指定Header获取请求ID
func RequestIDFromHeader(headerName string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(headerName)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			w.Header().Set(headerName, requestID)

			ctx := context.WithValue(r.Context(), ContextKeyRequestID, requestID)
			ctx = logging.WithRequestID(ctx, requestID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// PropagateHeaders 传播指定的Headers到响应
func PropagateHeaders(headers ...string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, header := range headers {
				if value := r.Header.Get(header); value != "" {
					w.Header().Set(header, value)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

package httpx

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// Recovery panic恢复中间件
func Recovery(logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// 获取堆栈信息
					stack := debug.Stack()

					// 获取请求信息
					requestID := GetRequestID(r.Context())
					traceID := GetTraceID(r.Context())

					// 记录日志
					if logger != nil {
						logger.Error("Panic recovered",
							zap.Any("error", err),
							zap.String("request_id", requestID),
							zap.String("trace_id", traceID),
							zap.String("method", r.Method),
							zap.String("path", r.URL.Path),
							zap.String("client_ip", GetClientIP(r)),
							zap.ByteString("stack", stack),
						)
					}

					// 返回500错误
					appErr := errors.New(errors.ErrCodeInternal, "Internal server error")
					appErr.WithTraceID(traceID)

					errors.WriteError(w, appErr, traceID, r.URL.Path)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// RecoveryWithCallback 带回调的panic恢复中间件
func RecoveryWithCallback(logger *zap.Logger, callback func(r *http.Request, err interface{}, stack []byte)) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					stack := debug.Stack()

					// 执行回调
					if callback != nil {
						callback(r, err, stack)
					}

					requestID := GetRequestID(r.Context())
					traceID := GetTraceID(r.Context())

					if logger != nil {
						logger.Error("Panic recovered",
							zap.Any("error", err),
							zap.String("request_id", requestID),
							zap.String("trace_id", traceID),
							zap.String("method", r.Method),
							zap.String("path", r.URL.Path),
							zap.ByteString("stack", stack),
						)
					}

					appErr := errors.New(errors.ErrCodeInternal, "Internal server error")
					errors.WriteError(w, appErr, traceID, r.URL.Path)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// PanicError 封装panic信息
type PanicError struct {
	Value interface{}
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.Value)
}

// GetClientIP 获取客户端IP
func GetClientIP(r *http.Request) string {
	// 优先从X-Forwarded-For获取
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// 取第一个IP
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// 其次从X-Real-IP获取
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// 最后从RemoteAddr获取
	return r.RemoteAddr
}

package httpx

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

func Recovery(logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {

					stack := debug.Stack()

					requestID := GetRequestID(r.Context())
					traceID := GetTraceID(r.Context())

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

					appErr := errors.New(errors.ErrCodeInternal, "Internal server error")
					appErr.WithTraceID(traceID)

					errors.WriteError(w, appErr, traceID, r.URL.Path)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func RecoveryWithCallback(logger *zap.Logger, callback func(r *http.Request, err interface{}, stack []byte)) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					stack := debug.Stack()

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

type PanicError struct {
	Value interface{}
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.Value)
}

func GetClientIP(r *http.Request) string {

	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {

		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	return r.RemoteAddr
}

////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/httpx/timeout.go
// 修复：超时中间件防止 goroutine 泄漏和重复写入
////////////////////////////////////////////////////////////////////////////////

package httpx

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// timeoutResponseWriter 带超时保护的 ResponseWriter
type timeoutResponseWriter struct {
	http.ResponseWriter
	mu          sync.Mutex
	wroteHeader bool
	timedOut    bool
}

func (tw *timeoutResponseWriter) WriteHeader(code int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if tw.timedOut || tw.wroteHeader {
		return
	}
	tw.wroteHeader = true
	tw.ResponseWriter.WriteHeader(code)
}

func (tw *timeoutResponseWriter) Write(b []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if tw.timedOut {
		return 0, context.DeadlineExceeded
	}

	if !tw.wroteHeader {
		tw.wroteHeader = true
		tw.ResponseWriter.WriteHeader(http.StatusOK)
	}

	return tw.ResponseWriter.Write(b)
}

func (tw *timeoutResponseWriter) markTimedOut() {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	tw.timedOut = true
}

func (tw *timeoutResponseWriter) hasWritten() bool {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	return tw.wroteHeader
}

// TimeoutWithConfig 带配置的超时中间件（修复版）
func TimeoutWithConfig(seconds int, onTimeout func(w http.ResponseWriter, r *http.Request)) Middleware {
	if seconds <= 0 {
		seconds = 30
	}
	timeout := time.Duration(seconds) * time.Second

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			// 使用带保护的 ResponseWriter
			tw := &timeoutResponseWriter{ResponseWriter: w}

			// 使用 channel 协调完成状态
			done := make(chan struct{})
			panicChan := make(chan interface{}, 1)

			go func() {
				defer func() {
					if p := recover(); p != nil {
						panicChan <- p
					}
				}()
				next.ServeHTTP(tw, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				// 正常完成
				return

			case p := <-panicChan:
				// handler panic
				panic(p)

			case <-ctx.Done():
				// 超时
				tw.markTimedOut()

				// 只在未写入响应时写入超时错误
				if !tw.hasWritten() {
					if onTimeout != nil {
						onTimeout(w, r)
					} else {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusGatewayTimeout)
						w.Write([]byte(`{"error":"request timeout"}`))
					}
				}
				return
			}
		})
	}
}

// TimeoutHandler 返回一个设置了超时的 http.Handler
func TimeoutHandler(h http.Handler, timeout time.Duration, message string) http.Handler {
	return http.TimeoutHandler(h, timeout, message)
}

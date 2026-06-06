package httpx

import (
	"context"
	"net/http"
	"sync"
	"time"
)

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

func TimeoutWithConfig(seconds int, onTimeout func(w http.ResponseWriter, r *http.Request)) Middleware {
	if seconds <= 0 {
		seconds = 30
	}
	timeout := time.Duration(seconds) * time.Second

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			tw := &timeoutResponseWriter{ResponseWriter: w}

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

				return

			case p := <-panicChan:

				panic(p)

			case <-ctx.Done():

				tw.markTimedOut()

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

func TimeoutHandler(h http.Handler, timeout time.Duration, message string) http.Handler {
	return http.TimeoutHandler(h, timeout, message)
}

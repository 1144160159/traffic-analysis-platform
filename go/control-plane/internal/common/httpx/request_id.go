package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
)

const (
	HeaderRequestID = "X-Request-ID"
	HeaderTraceID   = "X-Trace-ID"
	HeaderSpanID    = "X-Span-ID"
)

func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			requestID := r.Header.Get(HeaderRequestID)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			ctx, spanContext := correlationSpanContext(r.Context(), r.Header)
			traceID := spanContext.TraceID().String()
			spanID := spanContext.SpanID().String()
			// Ensure services without an enabled exporter still propagate the same
			// W3C identity to HTTP clients, Kafka producers and downstream calls.
			if strings.TrimSpace(r.Header.Get("traceparent")) == "" {
				r.Header.Set("traceparent", formatTraceparent(spanContext))
			}

			w.Header().Set(HeaderRequestID, requestID)
			w.Header().Set(HeaderTraceID, traceID)
			w.Header().Set(HeaderSpanID, spanID)

			ctx = context.WithValue(ctx, ContextKeyRequestID, requestID)
			ctx = context.WithValue(ctx, ContextKeyTraceID, traceID)

			ctx = logging.WithRequestID(ctx, requestID)
			ctx = logging.WithTraceID(ctx, traceID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func correlationSpanContext(ctx context.Context, header http.Header) (context.Context, trace.SpanContext) {
	// traceparent is authoritative. A conflicting X-Trace-ID cannot fork the
	// business correlation identity.
	extracted := propagation.TraceContext{}.Extract(ctx, propagation.HeaderCarrier(header))
	if spanContext := trace.SpanContextFromContext(extracted); spanContext.IsValid() {
		return extracted, spanContext
	}

	traceID, err := trace.TraceIDFromHex(strings.TrimSpace(header.Get(HeaderTraceID)))
	if err != nil || !traceID.IsValid() {
		traceID = newTraceID()
	}
	spanID, err := trace.SpanIDFromHex(strings.TrimSpace(header.Get(HeaderSpanID)))
	if err != nil || !spanID.IsValid() {
		spanID = newSpanID()
	}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, Remote: true,
	})
	return trace.ContextWithRemoteSpanContext(ctx, spanContext), spanContext
}

func newTraceID() trace.TraceID {
	for {
		var value trace.TraceID
		if _, err := rand.Read(value[:]); err != nil {
			panic("crypto/rand failed while generating trace ID: " + err.Error())
		}
		if value.IsValid() {
			return value
		}
	}
}

func newSpanID() trace.SpanID {
	for {
		var value trace.SpanID
		if _, err := rand.Read(value[:]); err != nil {
			panic("crypto/rand failed while generating span ID: " + err.Error())
		}
		if value.IsValid() {
			return value
		}
	}
}

func formatTraceparent(spanContext trace.SpanContext) string {
	flags := byte(spanContext.TraceFlags() & trace.FlagsSampled)
	return "00-" + spanContext.TraceID().String() + "-" + spanContext.SpanID().String() + "-" + hex.EncodeToString([]byte{flags})
}

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

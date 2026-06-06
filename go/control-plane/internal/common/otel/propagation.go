package otel

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
)

type HTTPHeadersCarrier http.Header

func (c HTTPHeadersCarrier) Get(key string) string {
	return http.Header(c).Get(key)
}

func (c HTTPHeadersCarrier) Set(key string, value string) {
	http.Header(c).Set(key, value)
}

func (c HTTPHeadersCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

type MetadataCarrier metadata.MD

func (c MetadataCarrier) Get(key string) string {
	values := metadata.MD(c).Get(key)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

func (c MetadataCarrier) Set(key string, value string) {
	metadata.MD(c).Set(key, value)
}

func (c MetadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

func ExtractFromHTTP(ctx context.Context, r *http.Request) context.Context {
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(ctx, HTTPHeadersCarrier(r.Header))
}

func InjectToHTTP(ctx context.Context, r *http.Request) {
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, HTTPHeadersCarrier(r.Header))
}

func ExtractFromGRPC(ctx context.Context, md metadata.MD) context.Context {
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(ctx, MetadataCarrier(md))
}

func InjectToGRPC(ctx context.Context) metadata.MD {
	md := metadata.MD{}
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, MetadataCarrier(md))
	return md
}

func ExtractFromMap(ctx context.Context, carrier map[string]string) context.Context {
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(ctx, propagation.MapCarrier(carrier))
}

func InjectToMap(ctx context.Context) map[string]string {
	carrier := make(map[string]string)
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, propagation.MapCarrier(carrier))
	return carrier
}

func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}

func ContextWithRemoteSpanContext(ctx context.Context, sc trace.SpanContext) context.Context {
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

func NewSpanContext(traceID, spanID string, sampled bool) (trace.SpanContext, error) {
	tid, err := trace.TraceIDFromHex(traceID)
	if err != nil {
		return trace.SpanContext{}, err
	}

	sid, err := trace.SpanIDFromHex(spanID)
	if err != nil {
		return trace.SpanContext{}, err
	}

	var flags trace.TraceFlags
	if sampled {
		flags = trace.FlagsSampled
	}

	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: flags,
		Remote:     true,
	}), nil
}

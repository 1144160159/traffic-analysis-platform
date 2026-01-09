package otel

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
)

// HTTPHeadersCarrier HTTP头载体
type HTTPHeadersCarrier http.Header

// Get 获取值
func (c HTTPHeadersCarrier) Get(key string) string {
	return http.Header(c).Get(key)
}

// Set 设置值
func (c HTTPHeadersCarrier) Set(key string, value string) {
	http.Header(c).Set(key, value)
}

// Keys 获取所有键
func (c HTTPHeadersCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// MetadataCarrier gRPC metadata载体
type MetadataCarrier metadata.MD

// Get 获取值
func (c MetadataCarrier) Get(key string) string {
	values := metadata.MD(c).Get(key)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

// Set 设置值
func (c MetadataCarrier) Set(key string, value string) {
	metadata.MD(c).Set(key, value)
}

// Keys 获取所有键
func (c MetadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// ExtractFromHTTP 从HTTP请求中提取trace context
func ExtractFromHTTP(ctx context.Context, r *http.Request) context.Context {
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(ctx, HTTPHeadersCarrier(r.Header))
}

// InjectToHTTP 注入trace context到HTTP请求
func InjectToHTTP(ctx context.Context, r *http.Request) {
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, HTTPHeadersCarrier(r.Header))
}

// ExtractFromGRPC 从gRPC metadata中提取trace context
func ExtractFromGRPC(ctx context.Context, md metadata.MD) context.Context {
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(ctx, MetadataCarrier(md))
}

// InjectToGRPC 注入trace context到gRPC metadata
func InjectToGRPC(ctx context.Context) metadata.MD {
	md := metadata.MD{}
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, MetadataCarrier(md))
	return md
}

// ExtractFromMap 从map中提取trace context
func ExtractFromMap(ctx context.Context, carrier map[string]string) context.Context {
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(ctx, propagation.MapCarrier(carrier))
}

// InjectToMap 注入trace context到map
func InjectToMap(ctx context.Context) map[string]string {
	carrier := make(map[string]string)
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, propagation.MapCarrier(carrier))
	return carrier
}

// ContextWithSpan 创建包含Span的Context
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}

// ContextWithRemoteSpanContext 创建包含远程SpanContext的Context
func ContextWithRemoteSpanContext(ctx context.Context, sc trace.SpanContext) context.Context {
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

// NewSpanContext 创建新的SpanContext
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

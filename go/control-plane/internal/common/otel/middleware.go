package otel

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
)

type AttributeExtractor interface {
	ExtractFromHTTP(r *http.Request) []attribute.KeyValue

	ExtractFromContext(ctx context.Context) []attribute.KeyValue

	ExtractFromGRPCMetadata(md metadata.MD) []attribute.KeyValue
}

type DefaultAttributeExtractor struct{}

func (e *DefaultAttributeExtractor) ExtractFromHTTP(r *http.Request) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 8)

	if tenantID := r.Header.Get("X-Tenant-ID"); tenantID != "" {
		attrs = append(attrs, attribute.String("tenant.id", tenantID))
	}

	if userID := r.Header.Get("X-User-ID"); userID != "" {
		attrs = append(attrs, attribute.String("user.id", userID))
	}

	if runID := r.Header.Get("X-Run-ID"); runID != "" {
		attrs = append(attrs, attribute.String("run.id", runID))
	}

	if probeID := r.Header.Get("X-Probe-ID"); probeID != "" {
		attrs = append(attrs, attribute.String("probe.id", probeID))
	}

	if featureSetID := r.Header.Get("X-Feature-Set-ID"); featureSetID != "" {
		attrs = append(attrs, attribute.String("feature_set.id", featureSetID))
	}

	return attrs
}

func (e *DefaultAttributeExtractor) ExtractFromContext(ctx context.Context) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 8)

	lc := logging.LogContextFromContext(ctx)
	if lc != nil {
		if lc.TenantID != "" {
			attrs = append(attrs, attribute.String("tenant.id", lc.TenantID))
		}
		if lc.UserID != "" {
			attrs = append(attrs, attribute.String("user.id", lc.UserID))
		}
		if lc.Username != "" {
			attrs = append(attrs, attribute.String("user.name", lc.Username))
		}
		if lc.ProbeID != "" {
			attrs = append(attrs, attribute.String("probe.id", lc.ProbeID))
		}
		if lc.RunID != "" {
			attrs = append(attrs, attribute.String("run.id", lc.RunID))
		}
		if lc.Component != "" {
			attrs = append(attrs, attribute.String("component", lc.Component))
		}
		if lc.Service != "" {
			attrs = append(attrs, attribute.String("service.name", lc.Service))
		}
	}

	return attrs
}

func (e *DefaultAttributeExtractor) ExtractFromGRPCMetadata(md metadata.MD) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 8)

	if vals := md.Get("x-tenant-id"); len(vals) > 0 {
		attrs = append(attrs, attribute.String("tenant.id", vals[0]))
	}

	if vals := md.Get("x-user-id"); len(vals) > 0 {
		attrs = append(attrs, attribute.String("user.id", vals[0]))
	}

	if vals := md.Get("x-run-id"); len(vals) > 0 {
		attrs = append(attrs, attribute.String("run.id", vals[0]))
	}

	if vals := md.Get("x-probe-id"); len(vals) > 0 {
		attrs = append(attrs, attribute.String("probe.id", vals[0]))
	}

	if vals := md.Get("x-feature-set-id"); len(vals) > 0 {
		attrs = append(attrs, attribute.String("feature_set.id", vals[0]))
	}

	return attrs
}

type MiddlewareConfig struct {
	ServiceName        string
	AttributeExtractor AttributeExtractor

	RecordRequestSize bool

	RecordResponseSize bool

	RecordErrorStack bool
}

func DefaultMiddlewareConfig(serviceName string) *MiddlewareConfig {
	return &MiddlewareConfig{
		ServiceName:        serviceName,
		AttributeExtractor: &DefaultAttributeExtractor{},
		RecordRequestSize:  true,
		RecordResponseSize: true,
		RecordErrorStack:   false,
	}
}

func HTTPMiddleware(serviceName string) func(http.Handler) http.Handler {
	config := DefaultMiddlewareConfig(serviceName)
	return HTTPMiddlewareWithConfig(config)
}

func HTTPMiddlewareWithConfig(config *MiddlewareConfig) func(http.Handler) http.Handler {
	tracer := otel.Tracer(config.ServiceName)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			ctx := ExtractFromHTTP(r.Context(), r)

			spanName := r.Method + " " + r.URL.Path
			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.method", r.Method),
					attribute.String("http.url", r.URL.String()),
					attribute.String("http.host", r.Host),
					attribute.String("http.scheme", r.URL.Scheme),
					attribute.String("http.target", r.URL.Path),
					attribute.String("http.user_agent", r.UserAgent()),
					attribute.String("http.client_ip", getClientIP(r)),
				),
			)
			defer span.End()

			if config.AttributeExtractor != nil {
				headerAttrs := config.AttributeExtractor.ExtractFromHTTP(r)
				if len(headerAttrs) > 0 {
					span.SetAttributes(headerAttrs...)
				}

				ctxAttrs := config.AttributeExtractor.ExtractFromContext(ctx)
				if len(ctxAttrs) > 0 {
					span.SetAttributes(ctxAttrs...)
				}
			}

			if config.RecordRequestSize && r.ContentLength > 0 {
				span.SetAttributes(attribute.Int64("http.request_content_length", r.ContentLength))
			}

			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			start := time.Now()
			next.ServeHTTP(rw, r.WithContext(ctx))
			duration := time.Since(start)

			span.SetAttributes(
				attribute.Int("http.status_code", rw.statusCode),
				attribute.Float64("http.duration_ms", float64(duration.Milliseconds())),
			)

			if config.RecordResponseSize {
				span.SetAttributes(attribute.Int64("http.response_content_length", int64(rw.bytesWritten)))
			}

			if rw.statusCode >= 400 {
				span.SetStatus(codes.Error, http.StatusText(rw.statusCode))

				span.SetAttributes(
					attribute.Bool("error", true),
					attribute.String("error.type", "http_error"),
				)
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

func UnaryServerInterceptor(serviceName string) grpc.UnaryServerInterceptor {
	config := DefaultMiddlewareConfig(serviceName)
	return UnaryServerInterceptorWithConfig(config)
}

func UnaryServerInterceptorWithConfig(config *MiddlewareConfig) grpc.UnaryServerInterceptor {
	tracer := otel.Tracer(config.ServiceName)

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = ExtractFromGRPC(ctx, md)
		}

		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
				attribute.String("rpc.service", config.ServiceName),
			),
		)
		defer span.End()

		if config.AttributeExtractor != nil {

			if ok {
				mdAttrs := config.AttributeExtractor.ExtractFromGRPCMetadata(md)
				if len(mdAttrs) > 0 {
					span.SetAttributes(mdAttrs...)
				}
			}

			ctxAttrs := config.AttributeExtractor.ExtractFromContext(ctx)
			if len(ctxAttrs) > 0 {
				span.SetAttributes(ctxAttrs...)
			}
		}

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		span.SetAttributes(
			attribute.Float64("rpc.duration_ms", float64(duration.Milliseconds())),
		)

		if err != nil {
			span.RecordError(err)
			st, _ := status.FromError(err)
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", st.Code().String()),
				attribute.Bool("error", true),
				attribute.String("error.type", "grpc_error"),
			)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return resp, err
	}
}

func StreamServerInterceptor(serviceName string) grpc.StreamServerInterceptor {
	config := DefaultMiddlewareConfig(serviceName)
	return StreamServerInterceptorWithConfig(config)
}

func StreamServerInterceptorWithConfig(config *MiddlewareConfig) grpc.StreamServerInterceptor {
	tracer := otel.Tracer(config.ServiceName)

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()

		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = ExtractFromGRPC(ctx, md)
		}

		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
				attribute.String("rpc.service", config.ServiceName),
				attribute.Bool("rpc.is_client_stream", info.IsClientStream),
				attribute.Bool("rpc.is_server_stream", info.IsServerStream),
			),
		)
		defer span.End()

		if config.AttributeExtractor != nil {

			if ok {
				mdAttrs := config.AttributeExtractor.ExtractFromGRPCMetadata(md)
				if len(mdAttrs) > 0 {
					span.SetAttributes(mdAttrs...)
				}
			}

			ctxAttrs := config.AttributeExtractor.ExtractFromContext(ctx)
			if len(ctxAttrs) > 0 {
				span.SetAttributes(ctxAttrs...)
			}
		}

		wrappedStream := &wrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		err := handler(srv, wrappedStream)

		if err != nil {
			span.RecordError(err)
			st, _ := status.FromError(err)
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", st.Code().String()),
				attribute.Bool("error", true),
				attribute.String("error.type", "grpc_error"),
			)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

func UnaryClientInterceptor(serviceName string) grpc.UnaryClientInterceptor {
	config := DefaultMiddlewareConfig(serviceName)
	return UnaryClientInterceptorWithConfig(config)
}

func UnaryClientInterceptorWithConfig(config *MiddlewareConfig) grpc.UnaryClientInterceptor {
	tracer := otel.Tracer(config.ServiceName)

	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {

		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", method),
				attribute.String("rpc.service", config.ServiceName),
			),
		)
		defer span.End()

		if config.AttributeExtractor != nil {
			ctxAttrs := config.AttributeExtractor.ExtractFromContext(ctx)
			if len(ctxAttrs) > 0 {
				span.SetAttributes(ctxAttrs...)
			}
		}

		md := InjectToGRPC(ctx)
		ctx = metadata.NewOutgoingContext(ctx, md)

		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		span.SetAttributes(
			attribute.Float64("rpc.duration_ms", float64(duration.Milliseconds())),
		)

		if err != nil {
			span.RecordError(err)
			st, _ := status.FromError(err)
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", st.Code().String()),
				attribute.Bool("error", true),
			)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

type HTTPClient struct {
	client    *http.Client
	tracer    trace.Tracer
	extractor AttributeExtractor
}

func NewHTTPClient(serviceName string) *HTTPClient {
	return &HTTPClient{
		client:    &http.Client{},
		tracer:    otel.Tracer(serviceName),
		extractor: &DefaultAttributeExtractor{},
	}
}

func NewHTTPClientWithConfig(serviceName string, extractor AttributeExtractor) *HTTPClient {
	return &HTTPClient{
		client:    &http.Client{},
		tracer:    otel.Tracer(serviceName),
		extractor: extractor,
	}
}

func (c *HTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	spanName := req.Method + " " + req.URL.Path
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.method", req.Method),
			attribute.String("http.url", req.URL.String()),
			attribute.String("http.scheme", req.URL.Scheme),
			attribute.String("http.host", req.URL.Host),
			attribute.String("http.target", req.URL.Path),
		),
	)
	defer span.End()

	if c.extractor != nil {
		ctxAttrs := c.extractor.ExtractFromContext(ctx)
		if len(ctxAttrs) > 0 {
			span.SetAttributes(ctxAttrs...)
		}
	}

	InjectToHTTP(ctx, req)

	start := time.Now()
	resp, err := c.client.Do(req.WithContext(ctx))
	duration := time.Since(start)

	span.SetAttributes(
		attribute.Float64("http.duration_ms", float64(duration.Milliseconds())),
	)

	if err != nil {
		span.RecordError(err)
		span.SetAttributes(
			attribute.Bool("error", true),
			attribute.String("error.type", "http_client_error"),
		)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
	)

	if resp.StatusCode >= 400 {
		span.SetAttributes(
			attribute.Bool("error", true),
			attribute.String("error.type", "http_error"),
		)
		span.SetStatus(codes.Error, http.StatusText(resp.StatusCode))
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return resp, nil
}

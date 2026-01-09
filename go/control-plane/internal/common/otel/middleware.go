////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/otel/middleware.go
// 修复版本 v2：
// 1. 修复 #10：HTTP 中间件自动添加 tenant_id、user_id 等业务属性
// 2. gRPC 中间件同步增强业务属性
// 3. 增加可配置的属性提取器
// 4. 支持从 Context 或 Header 自动提取业务字段
////////////////////////////////////////////////////////////////////////////////

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

// ==================== 修复 #10：增强的业务属性提取 ====================

// AttributeExtractor 属性提取器接口
type AttributeExtractor interface {
	// ExtractFromHTTP 从 HTTP 请求中提取属性
	ExtractFromHTTP(r *http.Request) []attribute.KeyValue
	// ExtractFromContext 从 Context 中提取属性
	ExtractFromContext(ctx context.Context) []attribute.KeyValue
	// ExtractFromGRPCMetadata 从 gRPC metadata 中提取属性
	ExtractFromGRPCMetadata(md metadata.MD) []attribute.KeyValue
}

// DefaultAttributeExtractor 默认属性提取器（修复 #10：新增）
type DefaultAttributeExtractor struct{}

// ExtractFromHTTP 从 HTTP 请求中提取业务属性
func (e *DefaultAttributeExtractor) ExtractFromHTTP(r *http.Request) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 8)

	// 从 Header 提取租户信息
	if tenantID := r.Header.Get("X-Tenant-ID"); tenantID != "" {
		attrs = append(attrs, attribute.String("tenant.id", tenantID))
	}

	// 从 Header 提取用户信息
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		attrs = append(attrs, attribute.String("user.id", userID))
	}

	// 从 Header 提取运行批次 ID
	if runID := r.Header.Get("X-Run-ID"); runID != "" {
		attrs = append(attrs, attribute.String("run.id", runID))
	}

	// 从 Header 提取探针 ID
	if probeID := r.Header.Get("X-Probe-ID"); probeID != "" {
		attrs = append(attrs, attribute.String("probe.id", probeID))
	}

	// 从 Header 提取特征集 ID
	if featureSetID := r.Header.Get("X-Feature-Set-ID"); featureSetID != "" {
		attrs = append(attrs, attribute.String("feature_set.id", featureSetID))
	}

	return attrs
}

// ExtractFromContext 从 Context 中提取业务属性
func (e *DefaultAttributeExtractor) ExtractFromContext(ctx context.Context) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 8)

	// 从日志上下文提取
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

// ExtractFromGRPCMetadata 从 gRPC metadata 中提取业务属性
func (e *DefaultAttributeExtractor) ExtractFromGRPCMetadata(md metadata.MD) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 8)

	// 提取租户 ID
	if vals := md.Get("x-tenant-id"); len(vals) > 0 {
		attrs = append(attrs, attribute.String("tenant.id", vals[0]))
	}

	// 提取用户 ID
	if vals := md.Get("x-user-id"); len(vals) > 0 {
		attrs = append(attrs, attribute.String("user.id", vals[0]))
	}

	// 提取运行批次 ID
	if vals := md.Get("x-run-id"); len(vals) > 0 {
		attrs = append(attrs, attribute.String("run.id", vals[0]))
	}

	// 提取探针 ID
	if vals := md.Get("x-probe-id"); len(vals) > 0 {
		attrs = append(attrs, attribute.String("probe.id", vals[0]))
	}

	// 提取特征集 ID
	if vals := md.Get("x-feature-set-id"); len(vals) > 0 {
		attrs = append(attrs, attribute.String("feature_set.id", vals[0]))
	}

	return attrs
}

// MiddlewareConfig 中间件配置（修复 #10：新增）
type MiddlewareConfig struct {
	ServiceName        string
	AttributeExtractor AttributeExtractor
	// 是否记录请求体大小
	RecordRequestSize bool
	// 是否记录响应体大小
	RecordResponseSize bool
	// 是否记录详细的错误堆栈
	RecordErrorStack bool
}

// DefaultMiddlewareConfig 默认中间件配置
func DefaultMiddlewareConfig(serviceName string) *MiddlewareConfig {
	return &MiddlewareConfig{
		ServiceName:        serviceName,
		AttributeExtractor: &DefaultAttributeExtractor{},
		RecordRequestSize:  true,
		RecordResponseSize: true,
		RecordErrorStack:   false, // 默认不记录堆栈（减少开销）
	}
}

// ==================== HTTP 中间件（修复 #10：增强版） ====================

// HTTPMiddleware OpenTelemetry HTTP中间件（修复 #10：增强业务属性）
func HTTPMiddleware(serviceName string) func(http.Handler) http.Handler {
	config := DefaultMiddlewareConfig(serviceName)
	return HTTPMiddlewareWithConfig(config)
}

// HTTPMiddlewareWithConfig 使用配置创建 HTTP 中间件（修复 #10：新增）
func HTTPMiddlewareWithConfig(config *MiddlewareConfig) func(http.Handler) http.Handler {
	tracer := otel.Tracer(config.ServiceName)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 提取父context
			ctx := ExtractFromHTTP(r.Context(), r)

			// 创建Span
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

			// 修复 #10：添加业务属性（从 Header）
			if config.AttributeExtractor != nil {
				headerAttrs := config.AttributeExtractor.ExtractFromHTTP(r)
				if len(headerAttrs) > 0 {
					span.SetAttributes(headerAttrs...)
				}

				// 从 Context 提取额外属性
				ctxAttrs := config.AttributeExtractor.ExtractFromContext(ctx)
				if len(ctxAttrs) > 0 {
					span.SetAttributes(ctxAttrs...)
				}
			}

			// 记录请求大小
			if config.RecordRequestSize && r.ContentLength > 0 {
				span.SetAttributes(attribute.Int64("http.request_content_length", r.ContentLength))
			}

			// 包装ResponseWriter以捕获状态码
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// 处理请求
			start := time.Now()
			next.ServeHTTP(rw, r.WithContext(ctx))
			duration := time.Since(start)

			// 记录响应信息
			span.SetAttributes(
				attribute.Int("http.status_code", rw.statusCode),
				attribute.Float64("http.duration_ms", float64(duration.Milliseconds())),
			)

			// 记录响应大小
			if config.RecordResponseSize {
				span.SetAttributes(attribute.Int64("http.response_content_length", int64(rw.bytesWritten)))
			}

			// 设置状态
			if rw.statusCode >= 400 {
				span.SetStatus(codes.Error, http.StatusText(rw.statusCode))

				// 记录错误属性
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

// ==================== gRPC 中间件（修复 #10：增强版） ====================

// UnaryServerInterceptor gRPC一元拦截器（修复 #10：增强业务属性）
func UnaryServerInterceptor(serviceName string) grpc.UnaryServerInterceptor {
	config := DefaultMiddlewareConfig(serviceName)
	return UnaryServerInterceptorWithConfig(config)
}

// UnaryServerInterceptorWithConfig 使用配置创建一元拦截器（修复 #10：新增）
func UnaryServerInterceptorWithConfig(config *MiddlewareConfig) grpc.UnaryServerInterceptor {
	tracer := otel.Tracer(config.ServiceName)

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// 从metadata中提取context
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = ExtractFromGRPC(ctx, md)
		}

		// 创建Span
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
				attribute.String("rpc.service", config.ServiceName),
			),
		)
		defer span.End()

		// 修复 #10：添加业务属性
		if config.AttributeExtractor != nil {
			// 从 metadata 提取
			if ok {
				mdAttrs := config.AttributeExtractor.ExtractFromGRPCMetadata(md)
				if len(mdAttrs) > 0 {
					span.SetAttributes(mdAttrs...)
				}
			}

			// 从 Context 提取
			ctxAttrs := config.AttributeExtractor.ExtractFromContext(ctx)
			if len(ctxAttrs) > 0 {
				span.SetAttributes(ctxAttrs...)
			}
		}

		// 处理请求
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		// 记录耗时
		span.SetAttributes(
			attribute.Float64("rpc.duration_ms", float64(duration.Milliseconds())),
		)

		// 记录错误
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

// StreamServerInterceptor gRPC流式拦截器（修复 #10：增强业务属性）
func StreamServerInterceptor(serviceName string) grpc.StreamServerInterceptor {
	config := DefaultMiddlewareConfig(serviceName)
	return StreamServerInterceptorWithConfig(config)
}

// StreamServerInterceptorWithConfig 使用配置创建流式拦截器（修复 #10：新增）
func StreamServerInterceptorWithConfig(config *MiddlewareConfig) grpc.StreamServerInterceptor {
	tracer := otel.Tracer(config.ServiceName)

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()

		// 从metadata中提取context
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = ExtractFromGRPC(ctx, md)
		}

		// 创建Span
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

		// 修复 #10：添加业务属性
		if config.AttributeExtractor != nil {
			// 从 metadata 提取
			if ok {
				mdAttrs := config.AttributeExtractor.ExtractFromGRPCMetadata(md)
				if len(mdAttrs) > 0 {
					span.SetAttributes(mdAttrs...)
				}
			}

			// 从 Context 提取
			ctxAttrs := config.AttributeExtractor.ExtractFromContext(ctx)
			if len(ctxAttrs) > 0 {
				span.SetAttributes(ctxAttrs...)
			}
		}

		// 包装stream
		wrappedStream := &wrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		// 处理请求
		err := handler(srv, wrappedStream)

		// 记录错误
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

// ==================== gRPC 客户端中间件（修复 #10：增强版） ====================

// UnaryClientInterceptor gRPC客户端一元拦截器（修复 #10：增强业务属性）
func UnaryClientInterceptor(serviceName string) grpc.UnaryClientInterceptor {
	config := DefaultMiddlewareConfig(serviceName)
	return UnaryClientInterceptorWithConfig(config)
}

// UnaryClientInterceptorWithConfig 使用配置创建客户端一元拦截器（修复 #10：新增）
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
		// 创建Span
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", method),
				attribute.String("rpc.service", config.ServiceName),
			),
		)
		defer span.End()

		// 修复 #10：添加业务属性（从 Context）
		if config.AttributeExtractor != nil {
			ctxAttrs := config.AttributeExtractor.ExtractFromContext(ctx)
			if len(ctxAttrs) > 0 {
				span.SetAttributes(ctxAttrs...)
			}
		}

		// 注入context到metadata
		md := InjectToGRPC(ctx)
		ctx = metadata.NewOutgoingContext(ctx, md)

		// 调用
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

// ==================== HTTP 客户端（修复 #10：增强版） ====================

// HTTPClient 带追踪的HTTP客户端（修复 #10：增强业务属性）
type HTTPClient struct {
	client    *http.Client
	tracer    trace.Tracer
	extractor AttributeExtractor
}

// NewHTTPClient 创建带追踪的HTTP客户端
func NewHTTPClient(serviceName string) *HTTPClient {
	return &HTTPClient{
		client:    &http.Client{},
		tracer:    otel.Tracer(serviceName),
		extractor: &DefaultAttributeExtractor{},
	}
}

// NewHTTPClientWithConfig 使用配置创建HTTP客户端（修复 #10：新增）
func NewHTTPClientWithConfig(serviceName string, extractor AttributeExtractor) *HTTPClient {
	return &HTTPClient{
		client:    &http.Client{},
		tracer:    otel.Tracer(serviceName),
		extractor: extractor,
	}
}

// Do 执行HTTP请求（修复 #10：增强业务属性）
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

	// 修复 #10：添加业务属性（从 Context）
	if c.extractor != nil {
		ctxAttrs := c.extractor.ExtractFromContext(ctx)
		if len(ctxAttrs) > 0 {
			span.SetAttributes(ctxAttrs...)
		}
	}

	// 注入context
	InjectToHTTP(ctx, req)

	// 执行请求
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

// ==================== 辅助函数 ====================

// AddBusinessAttributes 手动添加业务属性到 Span（修复 #10：新增）
func AddBusinessAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// AddTenantAttribute 添加租户属性（修复 #10：新增）
func AddTenantAttribute(ctx context.Context, tenantID string) {
	AddBusinessAttributes(ctx, attribute.String("tenant.id", tenantID))
}

// AddUserAttribute 添加用户属性（修复 #10：新增）
func AddUserAttribute(ctx context.Context, userID, username string) {
	attrs := []attribute.KeyValue{
		attribute.String("user.id", userID),
	}
	if username != "" {
		attrs = append(attrs, attribute.String("user.name", username))
	}
	AddBusinessAttributes(ctx, attrs...)
}

// AddRunAttribute 添加运行批次属性（修复 #10：新增）
func AddRunAttribute(ctx context.Context, runID string) {
	AddBusinessAttributes(ctx, attribute.String("run.id", runID))
}

// AddProbeAttribute 添加探针属性（修复 #10：新增）
func AddProbeAttribute(ctx context.Context, probeID string) {
	AddBusinessAttributes(ctx, attribute.String("probe.id", probeID))
}

// AddFeatureSetAttribute 添加特征集属性（修复 #10：新增）
func AddFeatureSetAttribute(ctx context.Context, featureSetID string) {
	AddBusinessAttributes(ctx, attribute.String("feature_set.id", featureSetID))
}

// AddAlertAttribute 添加告警属性（修复 #10：新增）
func AddAlertAttribute(ctx context.Context, alertID, severity string) {
	attrs := []attribute.KeyValue{
		attribute.String("alert.id", alertID),
	}
	if severity != "" {
		attrs = append(attrs, attribute.String("alert.severity", severity))
	}
	AddBusinessAttributes(ctx, attrs...)
}

// AddSessionAttribute 添加会话属性（修复 #10：新增）
func AddSessionAttribute(ctx context.Context, sessionID, communityID string) {
	attrs := []attribute.KeyValue{
		attribute.String("session.id", sessionID),
	}
	if communityID != "" {
		attrs = append(attrs, attribute.String("session.community_id", communityID))
	}
	AddBusinessAttributes(ctx, attrs...)
}

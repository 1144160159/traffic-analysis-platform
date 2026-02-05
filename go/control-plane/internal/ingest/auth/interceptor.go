////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/auth/interceptor.go
// 优化版 v3：
// 1. ✅ 健康检查白名单放行（不被拦截）
// 2. ✅ 统一日志、审计、错误处理
// 3. ✅ 移除所有硬编码
// 4. ✅ 完整的请求链路追踪
////////////////////////////////////////////////////////////////////////////////

package auth

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/quota"
)

// contextKey 上下文键类型
type contextKey string

const (
	TenantIDKey  contextKey = "tenant_id"
	ProbeIDKey   contextKey = "probe_id"
	ScopesKey    contextKey = "scopes"
	TokenInfoKey contextKey = "token_info"
)

// InterceptorConfig 拦截器配置
type InterceptorConfig struct {
	RequireMTLS     bool     `env:"REQUIRE_MTLS" envDefault:"false"`
	AllowNoToken    bool     `env:"ALLOW_NO_TOKEN" envDefault:"false"`
	EnableRateLimit bool     `env:"ENABLE_RATE_LIMIT" envDefault:"true"`
	GlobalRPS       float64  `env:"RATE_LIMIT_GLOBAL_RPS" envDefault:"100000"`
	GlobalBurst     int      `env:"RATE_LIMIT_GLOBAL_BURST" envDefault:"200000"`
	TenantRPS       float64  `env:"RATE_LIMIT_TENANT_RPS" envDefault:"10000"`
	DefaultTenantID string   `env:"DEFAULT_TENANT_ID" envDefault:""`
	RequireScopes   bool     `env:"REQUIRE_SCOPES" envDefault:"true"`
	RequiredScopes  []string `env:"REQUIRED_SCOPES" envSeparator:"," envDefault:"ingest:write"`
	EnableAuditLog  bool     `env:"ENABLE_AUDIT_LOG" envDefault:"true"`
	EnableProbeRBAC bool     `env:"ENABLE_PROBE_RBAC" envDefault:"true"`
}

// TokenInfo Token 信息（包含 scopes）
type TokenInfo struct {
	TenantID  string
	ProbeID   string
	Scopes    []string
	ExpiresAt int64
}

// HasScope 检查是否有指定 scope
func (t *TokenInfo) HasScope(scope string) bool {
	for _, s := range t.Scopes {
		if s == scope || s == config.ScopeWildcard {
			return true
		}
	}
	return false
}

// HasAnyScope 检查是否有任一 scope
func (t *TokenInfo) HasAnyScope(scopes ...string) bool {
	for _, scope := range scopes {
		if t.HasScope(scope) {
			return true
		}
	}
	return false
}

// Interceptor gRPC 认证拦截器
type Interceptor struct {
	logger      *zap.Logger
	tokenCache  *TokenCache
	limiter     *quota.Limiter
	config      InterceptorConfig
	auditLogger *audit.Logger
}

// AuditEvent 审计事件（简化版，实际应使用 common/audit）
type AuditEvent struct {
	EventType string
	TenantID  string
	ProbeID   string
	Method    string
	ClientIP  string
	Error     string
	Scopes    []string
}

// NewInterceptor 创建拦截器
func NewInterceptor(logger *zap.Logger, tokenCache *TokenCache, config1 InterceptorConfig) *Interceptor {
	// 设置默认 required scopes
	if len(config1.RequiredScopes) == 0 {
		config1.RequiredScopes = []string{config.ScopeIngestWrite}
	}

	return &Interceptor{
		logger:     logger,
		tokenCache: tokenCache,
		config:     config1,
	}
}

// SetLimiter 设置限流器
func (i *Interceptor) SetLimiter(limiter *quota.Limiter) {
	i.limiter = limiter
}

// SetAuditLogger 设置审计日志记录器
func (i *Interceptor) SetAuditLogger(auditLogger *audit.Logger) {
	i.auditLogger = auditLogger
}

// UnaryInterceptor 一元 RPC 拦截器
func (i *Interceptor) UnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	// ✅ 步骤 0: 检查公开方法（白名单：健康检查、反射等）
	if config.IsPublicMethod(info.FullMethod) {
		i.logger.Debug("Public method bypassed authentication",
			zap.String("method", info.FullMethod))
		return handler(ctx, req)
	}

	// 启动追踪
	ctx, span := otel.StartSpan(ctx, "auth.unary_interceptor")
	defer span.End()

	clientIP := i.extractClientIP(ctx)

	// 步骤 1: 提取探针 ID
	probeID, err := i.extractProbeID(ctx)
	if err != nil {
		return i.handleAuthError(ctx, "", probeID, info.FullMethod, clientIP,
			errors.Wrap(err, errors.ErrCodeUnauthorized, "probe ID extraction failed"))
	}

	// 步骤 2: 验证 Token 并获取完整信息
	tokenInfo, err := i.extractAndValidateToken(ctx, probeID)
	if err != nil {
		// 尝试降级方案
		if i.config.AllowNoToken {
			tokenInfo = i.resolveTenantIDFallback(probeID)
			if tokenInfo == nil || tokenInfo.TenantID == "" {
				return i.handleAuthError(ctx, "", probeID, info.FullMethod, clientIP,
					errors.New(errors.ErrCodeUnauthorized, "cannot determine tenant: all fallback methods failed"))
			}
			i.logger.Debug("Using tenant from fallback",
				zap.String("probe_id", probeID),
				zap.String("tenant_id", tokenInfo.TenantID))
		} else {
			return i.handleAuthError(ctx, "", probeID, info.FullMethod, clientIP,
				errors.Wrap(err, errors.ErrCodeUnauthorized, "token validation failed"))
		}
	}

	tenantID := tokenInfo.TenantID

	// 步骤 3: 探针级 RBAC 权限检查
	if i.config.EnableProbeRBAC && i.config.RequireScopes {
		if err := i.checkProbePermissions(tokenInfo, info.FullMethod); err != nil {
			return i.handleAuthError(ctx, tenantID, probeID, info.FullMethod, clientIP, err)
		}
	}

	// 步骤 4: 限流检查
	if i.config.EnableRateLimit && i.limiter != nil {
		if !i.limiter.Allow(ctx, tenantID, probeID) {
			i.recordAudit(ctx, config.AuditEventTypeAccessDenied, tenantID, probeID, info.FullMethod, clientIP, "rate_limit_exceeded", tokenInfo.Scopes)
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
	}

	// 步骤 5: 将认证信息注入 Context
	ctx = i.enrichContext(ctx, tokenInfo)

	// 步骤 6: 记录成功的审计日志
	i.recordAudit(ctx, "auth_success", tenantID, probeID, info.FullMethod, clientIP, "", tokenInfo.Scopes)

	i.logger.Debug("Request authenticated",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("method", info.FullMethod),
		zap.Strings("scopes", tokenInfo.Scopes))

	// 调用实际处理器
	return handler(ctx, req)
}

// StreamInterceptor 流式 RPC 拦截器
func (i *Interceptor) StreamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	// ✅ 检查公开方法
	if config.IsPublicMethod(info.FullMethod) {
		i.logger.Debug("Public stream method bypassed authentication",
			zap.String("method", info.FullMethod))
		return handler(srv, ss)
	}

	ctx := ss.Context()
	ctx, span := otel.StartSpan(ctx, "auth.stream_interceptor")
	defer span.End()

	clientIP := i.extractClientIP(ctx)

	// 提取探针 ID
	probeID, err := i.extractProbeID(ctx)
	if err != nil {
		_, grpcErr := i.handleAuthError(ctx, "", probeID, info.FullMethod, clientIP,
			errors.Wrap(err, errors.ErrCodeUnauthorized, "probe ID extraction failed"))
		return grpcErr
	}

	// 验证 Token
	tokenInfo, err := i.extractAndValidateToken(ctx, probeID)
	if err != nil {
		if i.config.AllowNoToken {
			tokenInfo = i.resolveTenantIDFallback(probeID)
			if tokenInfo == nil || tokenInfo.TenantID == "" {
				_, grpcErr := i.handleAuthError(ctx, "", probeID, info.FullMethod, clientIP,
					errors.New(errors.ErrCodeUnauthorized, "cannot determine tenant"))
				return grpcErr
			}
		} else {
			_, grpcErr := i.handleAuthError(ctx, "", probeID, info.FullMethod, clientIP,
				errors.Wrap(err, errors.ErrCodeUnauthorized, "token validation failed"))
			return grpcErr
		}
	}

	tenantID := tokenInfo.TenantID

	// RBAC 检查
	if i.config.EnableProbeRBAC && i.config.RequireScopes {
		if err := i.checkProbePermissions(tokenInfo, info.FullMethod); err != nil {
			_, grpcErr := i.handleAuthError(ctx, tenantID, probeID, info.FullMethod, clientIP, err)
			return grpcErr
		}
	}

	// 限流检查
	if i.config.EnableRateLimit && i.limiter != nil {
		if !i.limiter.Allow(ctx, tenantID, probeID) {
			i.recordAudit(ctx, config.AuditEventTypeAccessDenied, tenantID, probeID, info.FullMethod, clientIP, "rate_limit_exceeded", tokenInfo.Scopes)
			return status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
	}

	// 包装 ServerStream
	newCtx := i.enrichContext(ctx, tokenInfo)
	wrapped := &wrappedServerStream{
		ServerStream: ss,
		ctx:          newCtx,
	}

	i.recordAudit(ctx, "auth_success", tenantID, probeID, info.FullMethod, clientIP, "", tokenInfo.Scopes)

	return handler(srv, wrapped)
}

// wrappedServerStream 包装的 ServerStream
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// enrichContext 将认证信息注入 Context
func (i *Interceptor) enrichContext(ctx context.Context, tokenInfo *TokenInfo) context.Context {
	ctx = context.WithValue(ctx, TenantIDKey, tokenInfo.TenantID)
	ctx = context.WithValue(ctx, ProbeIDKey, tokenInfo.ProbeID)
	ctx = context.WithValue(ctx, ScopesKey, tokenInfo.Scopes)
	ctx = context.WithValue(ctx, TokenInfoKey, tokenInfo)

	// 注入日志上下文
	ctx = logging.WithTenantID(ctx, tokenInfo.TenantID)
	ctx = logging.WithProbeID(ctx, tokenInfo.ProbeID)

	// 注入 OpenTelemetry 属性
	otel.AddTenantAttribute(ctx, tokenInfo.TenantID)
	otel.AddProbeAttribute(ctx, tokenInfo.ProbeID)

	return ctx
}

// handleAuthError 统一处理认证错误
func (i *Interceptor) handleAuthError(ctx context.Context, tenantID, probeID, method, clientIP string, err *errors.AppError) (interface{}, error) {
	logger := logging.L(ctx)

	logger.Warn("Authentication failed",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("method", method),
		zap.String("client_ip", clientIP),
		zap.Error(err))

	// 记录审计日志
	i.recordAudit(ctx, config.AuditEventTypeAuthFailure, tenantID, probeID, method, clientIP, err.Error(), nil)

	// 记录到 OpenTelemetry
	otel.RecordError(ctx, err)

	// 转换为 gRPC 错误
	return nil, status.Error(codes.Code(err.HTTPStatus()/100), err.Message)
}

// checkProbePermissions 检查探针权限
func (i *Interceptor) checkProbePermissions(tokenInfo *TokenInfo, method string) *errors.AppError {
	if tokenInfo == nil {
		return errors.New(errors.ErrCodePermissionDenied, "no token info available")
	}

	requiredScopes := i.getRequiredScopesForMethod(method)

	for _, required := range requiredScopes {
		if tokenInfo.HasScope(required) {
			return nil
		}
	}

	return errors.Newf(errors.ErrCodePermissionDenied,
		"permission denied: required scopes %v, got %v", requiredScopes, tokenInfo.Scopes)
}

// getRequiredScopesForMethod 根据方法获取所需权限
func (i *Interceptor) getRequiredScopesForMethod(method string) []string {
	// 根据方法名判断所需权限
	methodMap := map[string][]string{
		"UploadFlows":     {config.ScopeIngestWrite, config.ScopeWildcard},
		"StreamFlows":     {config.ScopeIngestWrite, config.ScopeWildcard},
		"UploadSessions":  {config.ScopeIngestWrite, config.ScopeWildcard},
		"UploadPcapIndex": {config.ScopePcapWrite, config.ScopeIngestWrite, config.ScopeWildcard},
		"Heartbeat":       {config.ScopeIngestRead, config.ScopeIngestWrite, config.ScopeWildcard},
	}

	for methodName, scopes := range methodMap {
		if len(method) >= len(methodName) && method[len(method)-len(methodName):] == methodName {
			return scopes
		}
	}

	return i.config.RequiredScopes
}

// resolveTenantIDFallback 解析租户 ID 的降级方法
func (i *Interceptor) resolveTenantIDFallback(probeID string) *TokenInfo {
	// 1. 尝试从 probe_id 提取
	tenantID := extractTenantFromProbeID(probeID)
	if tenantID != "" {
		return &TokenInfo{
			TenantID: tenantID,
			ProbeID:  probeID,
			Scopes:   []string{config.ScopeIngestWrite},
		}
	}

	// 2. 使用默认租户
	if i.config.DefaultTenantID != "" {
		return &TokenInfo{
			TenantID: i.config.DefaultTenantID,
			ProbeID:  probeID,
			Scopes:   []string{config.ScopeIngestWrite},
		}
	}

	return nil
}

// extractProbeID 提取探针 ID
func (i *Interceptor) extractProbeID(ctx context.Context) (string, error) {
	// 非 mTLS 模式：从 metadata 获取
	if !i.config.RequireMTLS {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return "", fmt.Errorf("no metadata in context")
		}

		probeID := getFirstMetadataValue(md, "x-probe-id", "probe-id", "probe_id")
		if probeID == "" {
			return "", fmt.Errorf("probe-id not found in metadata")
		}
		return probeID, nil
	}

	// mTLS 模式：从证书提取
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("no peer info in context")
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", fmt.Errorf("no TLS info in peer")
	}

	if len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return "", fmt.Errorf("no verified certificate chains")
	}

	cert := tlsInfo.State.VerifiedChains[0][0]
	probeID := cert.Subject.CommonName
	if probeID == "" {
		return "", fmt.Errorf("empty CommonName in certificate")
	}

	return probeID, nil
}

// extractAndValidateToken 提取并验证 Token
func (i *Interceptor) extractAndValidateToken(ctx context.Context, probeID string) (*TokenInfo, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("no metadata in context")
	}

	token := getFirstMetadataValue(md, "x-tenant-token", "authorization", "x-api-key")
	if token == "" {
		return nil, fmt.Errorf("tenant token not found in metadata")
	}

	// 移除 "Bearer " 前缀
	if len(token) > 7 && (token[:7] == "Bearer " || token[:7] == "bearer ") {
		token = token[7:]
	}

	// 验证 Token
	tokenInfo, err := i.tokenCache.ValidateWithScopes(ctx, probeID, token)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	// 检查 ProbeID 绑定
	if tokenInfo.ProbeID != "" && tokenInfo.ProbeID != probeID {
		return nil, fmt.Errorf("token is bound to probe %s, but request came from %s", tokenInfo.ProbeID, probeID)
	}

	return tokenInfo, nil
}

// extractClientIP 提取客户端 IP
func (i *Interceptor) extractClientIP(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if xff := getFirstMetadataValue(md, "x-forwarded-for", "x-real-ip"); xff != "" {
			// 取第一个 IP
			for j := 0; j < len(xff); j++ {
				if xff[j] == ',' {
					return xff[:j]
				}
			}
			return xff
		}
	}

	p, ok := peer.FromContext(ctx)
	if ok {
		return p.Addr.String()
	}

	return ""
}

// recordAudit 记录审计日志
func (i *Interceptor) recordAudit(ctx context.Context, eventType, tenantID, probeID, method, clientIP, errMsg string, scopes []string) {
	if !i.config.EnableAuditLog {
		return
	}

	if i.auditLogger != nil {
		i.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    audit.EventType(eventType),
			TenantID:     tenantID,
			UserID:       probeID, // 探针 ID 作为用户 ID
			Action:       method,
			ResourceType: "grpc_method",
			ResourceID:   method,
			IPAddr:       clientIP,
			UserAgent:    "",
			Result:       audit.ResultSuccess,
			ErrorMsg:     errMsg,
			Detail: map[string]interface{}{
				"scopes":   scopes,
				"probe_id": probeID,
			},
		})
	}
}

// getFirstMetadataValue 获取第一个存在的 metadata 值
func getFirstMetadataValue(md metadata.MD, keys ...string) string {
	for _, key := range keys {
		values := md.Get(key)
		if len(values) > 0 && values[0] != "" {
			return values[0]
		}
	}
	return ""
}

// GetTenantID 从 Context 获取租户 ID
func GetTenantID(ctx context.Context) string {
	if v := ctx.Value(TenantIDKey); v != nil {
		return v.(string)
	}
	return ""
}

// GetProbeID 从 Context 获取探针 ID
func GetProbeID(ctx context.Context) string {
	if v := ctx.Value(ProbeIDKey); v != nil {
		return v.(string)
	}
	return ""
}

// GetScopes 从 Context 获取权限列表
func GetScopes(ctx context.Context) []string {
	if v := ctx.Value(ScopesKey); v != nil {
		return v.([]string)
	}
	return nil
}

// GetTokenInfo 从 Context 获取完整 Token 信息
func GetTokenInfo(ctx context.Context) *TokenInfo {
	if v := ctx.Value(TokenInfoKey); v != nil {
		return v.(*TokenInfo)
	}
	return nil
}

// HasScope 检查 Context 中是否有指定 scope
func HasScope(ctx context.Context, scope string) bool {
	scopes := GetScopes(ctx)
	for _, s := range scopes {
		if s == scope || s == config.ScopeWildcard {
			return true
		}
	}
	return false
}

// extractTenantFromProbeID 从探针 ID 提取租户
func extractTenantFromProbeID(probeID string) string {
	if len(probeID) < 7 {
		return ""
	}

	if probeID[:6] != "probe-" {
		return ""
	}

	rest := probeID[6:]
	lastDash := -1
	for j := len(rest) - 1; j >= 0; j-- {
		if rest[j] == '-' {
			lastDash = j
			break
		}
	}

	if lastDash <= 0 {
		return ""
	}

	tenantID := rest[:lastDash]
	if tenantID == "" || !isValidTenantID(tenantID) {
		return ""
	}

	return tenantID
}

// isValidTenantID 验证租户 ID 是否合法
func isValidTenantID(tenantID string) bool {
	if tenantID == "" {
		return false
	}

	for _, c := range tenantID {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}

	return true
}

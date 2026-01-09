////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/auth/interceptor.go
// 修复版：添加探针级 RBAC 权限检查、scopes 验证、审计集成
////////////////////////////////////////////////////////////////////////////////

package auth

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/quota"
)

type contextKey string

const (
	TenantIDKey  contextKey = "tenant_id"
	ProbeIDKey   contextKey = "probe_id"
	ScopesKey    contextKey = "scopes"
	TokenInfoKey contextKey = "token_info"
)

// 探针权限 scopes
const (
	ScopeIngestWrite = "ingest:write"
	ScopeIngestRead  = "ingest:read"
	ScopePcapWrite   = "pcap:write"
	ScopePcapRead    = "pcap:read"
)

// InterceptorConfig 拦截器配置
type InterceptorConfig struct {
	RequireMTLS     bool `env:"REQUIRE_MTLS" envDefault:"false"`
	AllowNoToken    bool `env:"ALLOW_NO_TOKEN" envDefault:"false"`
	EnableRateLimit bool `env:"ENABLE_RATE_LIMIT" envDefault:"true"`

	// 限流配置
	GlobalRPS   float64 `env:"RATE_LIMIT_GLOBAL_RPS" envDefault:"100000"`
	GlobalBurst int     `env:"RATE_LIMIT_GLOBAL_BURST" envDefault:"200000"`
	TenantRPS   float64 `env:"RATE_LIMIT_TENANT_RPS" envDefault:"10000"`

	// 默认租户（用于开发测试）
	DefaultTenantID string `env:"DEFAULT_TENANT_ID" envDefault:""`

	// 权限检查
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
		if s == scope || s == "*" {
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
	logger     *zap.Logger
	tokenCache *TokenCache
	limiter    *quota.Limiter
	config     InterceptorConfig

	// 审计日志回调（可选）
	auditCallback AuditCallback
}

// AuditCallback 审计日志回调函数
type AuditCallback func(ctx context.Context, event *AuditEvent)

// AuditEvent 审计事件
type AuditEvent struct {
	EventType string // auth_success, auth_fail, rate_limit, permission_denied
	TenantID  string
	ProbeID   string
	Method    string
	ClientIP  string
	Error     string
	Scopes    []string
}

// NewInterceptor 创建拦截器
func NewInterceptor(logger *zap.Logger, tokenCache *TokenCache, config InterceptorConfig) *Interceptor {
	// 设置默认 required scopes
	if len(config.RequiredScopes) == 0 {
		config.RequiredScopes = []string{ScopeIngestWrite}
	}

	return &Interceptor{
		logger:     logger,
		tokenCache: tokenCache,
		config:     config,
	}
}

// SetLimiter 设置限流器
func (i *Interceptor) SetLimiter(limiter *quota.Limiter) {
	i.limiter = limiter
}

// SetAuditCallback 设置审计回调
func (i *Interceptor) SetAuditCallback(callback AuditCallback) {
	i.auditCallback = callback
}

// UnaryInterceptor 一元 RPC 拦截器
func (i *Interceptor) UnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	// 获取客户端 IP
	clientIP := i.extractClientIP(ctx)

	// 1. 提取探针 ID
	probeID, err := i.extractProbeID(ctx)
	if err != nil {
		i.logger.Warn("Probe ID extraction failed",
			zap.String("method", info.FullMethod),
			zap.String("client_ip", clientIP),
			zap.Error(err))
		i.recordAudit(ctx, "auth_fail", "", probeID, info.FullMethod, clientIP, err.Error(), nil)
		return nil, status.Error(codes.Unauthenticated, "invalid client certificate or missing probe-id")
	}

	// 2. 验证 Token 并获取完整信息（包括 scopes）
	tokenInfo, err := i.extractAndValidateToken(ctx, probeID)
	if err != nil {
		// 尝试降级方案
		if i.config.AllowNoToken {
			tokenInfo = i.resolveTenantIDFallback(probeID)
			if tokenInfo == nil || tokenInfo.TenantID == "" {
				i.logger.Warn("Cannot determine tenant: all fallback methods failed",
					zap.String("probe_id", probeID),
					zap.String("method", info.FullMethod),
					zap.String("client_ip", clientIP),
					zap.Error(err))
				i.recordAudit(ctx, "auth_fail", "", probeID, info.FullMethod, clientIP, "no_token_fallback_failed", nil)
				return nil, status.Error(codes.Unauthenticated,
					"cannot determine tenant: token validation failed and no fallback available")
			}
			i.logger.Debug("Using tenant from fallback",
				zap.String("probe_id", probeID),
				zap.String("tenant_id", tokenInfo.TenantID),
				zap.String("source", "fallback"))
		} else {
			i.logger.Warn("Token validation failed",
				zap.String("probe_id", probeID),
				zap.String("method", info.FullMethod),
				zap.String("client_ip", clientIP),
				zap.Error(err))
			i.recordAudit(ctx, "auth_fail", "", probeID, info.FullMethod, clientIP, err.Error(), nil)
			return nil, status.Error(codes.Unauthenticated, "invalid tenant token")
		}
	}

	tenantID := tokenInfo.TenantID

	// 3. 探针级 RBAC 权限检查
	if i.config.EnableProbeRBAC && i.config.RequireScopes {
		if err := i.checkProbePermissions(tokenInfo, info.FullMethod); err != nil {
			i.logger.Warn("Permission denied",
				zap.String("tenant_id", tenantID),
				zap.String("probe_id", probeID),
				zap.String("method", info.FullMethod),
				zap.Strings("scopes", tokenInfo.Scopes),
				zap.Strings("required", i.config.RequiredScopes),
				zap.Error(err))
			i.recordAudit(ctx, "permission_denied", tenantID, probeID, info.FullMethod, clientIP, err.Error(), tokenInfo.Scopes)
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	// 4. 限流检查
	if i.config.EnableRateLimit && i.limiter != nil {
		if !i.limiter.Allow(ctx, tenantID, probeID) {
			i.logger.Warn("Rate limit exceeded",
				zap.String("tenant_id", tenantID),
				zap.String("probe_id", probeID),
				zap.String("method", info.FullMethod),
				zap.String("client_ip", clientIP))
			i.recordAudit(ctx, "rate_limit", tenantID, probeID, info.FullMethod, clientIP, "rate_limit_exceeded", tokenInfo.Scopes)
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
	}

	// 5. 将认证信息注入 Context
	ctx = context.WithValue(ctx, TenantIDKey, tenantID)
	ctx = context.WithValue(ctx, ProbeIDKey, probeID)
	ctx = context.WithValue(ctx, ScopesKey, tokenInfo.Scopes)
	ctx = context.WithValue(ctx, TokenInfoKey, tokenInfo)

	i.logger.Debug("Request authenticated",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("method", info.FullMethod),
		zap.Strings("scopes", tokenInfo.Scopes))

	i.recordAudit(ctx, "auth_success", tenantID, probeID, info.FullMethod, clientIP, "", tokenInfo.Scopes)

	// 6. 调用实际处理器
	return handler(ctx, req)
}

// StreamInterceptor 流式 RPC 拦截器
func (i *Interceptor) StreamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	ctx := ss.Context()
	clientIP := i.extractClientIP(ctx)

	// 1. 提取探针 ID
	probeID, err := i.extractProbeID(ctx)
	if err != nil {
		i.logger.Warn("Probe ID extraction failed (stream)",
			zap.String("method", info.FullMethod),
			zap.String("client_ip", clientIP),
			zap.Error(err))
		i.recordAudit(ctx, "auth_fail", "", probeID, info.FullMethod, clientIP, err.Error(), nil)
		return status.Error(codes.Unauthenticated, "invalid client certificate or missing probe-id")
	}

	// 2. 验证 Token 并获取完整信息
	tokenInfo, err := i.extractAndValidateToken(ctx, probeID)
	if err != nil {
		if i.config.AllowNoToken {
			tokenInfo = i.resolveTenantIDFallback(probeID)
			if tokenInfo == nil || tokenInfo.TenantID == "" {
				i.logger.Warn("Cannot determine tenant: all fallback methods failed (stream)",
					zap.String("probe_id", probeID),
					zap.String("method", info.FullMethod),
					zap.String("client_ip", clientIP),
					zap.Error(err))
				i.recordAudit(ctx, "auth_fail", "", probeID, info.FullMethod, clientIP, "no_token_fallback_failed", nil)
				return status.Error(codes.Unauthenticated,
					"cannot determine tenant: token validation failed and no fallback available")
			}
			i.logger.Debug("Using tenant from fallback (stream)",
				zap.String("probe_id", probeID),
				zap.String("tenant_id", tokenInfo.TenantID),
				zap.String("source", "fallback"))
		} else {
			i.logger.Warn("Token validation failed (stream)",
				zap.String("probe_id", probeID),
				zap.String("method", info.FullMethod),
				zap.String("client_ip", clientIP),
				zap.Error(err))
			i.recordAudit(ctx, "auth_fail", "", probeID, info.FullMethod, clientIP, err.Error(), nil)
			return status.Error(codes.Unauthenticated, "invalid tenant token")
		}
	}

	tenantID := tokenInfo.TenantID

	// 3. 探针级 RBAC 权限检查
	if i.config.EnableProbeRBAC && i.config.RequireScopes {
		if err := i.checkProbePermissions(tokenInfo, info.FullMethod); err != nil {
			i.logger.Warn("Permission denied (stream)",
				zap.String("tenant_id", tenantID),
				zap.String("probe_id", probeID),
				zap.String("method", info.FullMethod),
				zap.Strings("scopes", tokenInfo.Scopes),
				zap.Error(err))
			i.recordAudit(ctx, "permission_denied", tenantID, probeID, info.FullMethod, clientIP, err.Error(), tokenInfo.Scopes)
			return status.Error(codes.PermissionDenied, err.Error())
		}
	}

	// 4. 限流检查
	if i.config.EnableRateLimit && i.limiter != nil {
		if !i.limiter.Allow(ctx, tenantID, probeID) {
			i.logger.Warn("Rate limit exceeded (stream)",
				zap.String("tenant_id", tenantID),
				zap.String("probe_id", probeID),
				zap.String("method", info.FullMethod),
				zap.String("client_ip", clientIP))
			i.recordAudit(ctx, "rate_limit", tenantID, probeID, info.FullMethod, clientIP, "rate_limit_exceeded", tokenInfo.Scopes)
			return status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
	}

	// 5. 包装 ServerStream 以注入认证信息
	newCtx := context.WithValue(ctx, TenantIDKey, tenantID)
	newCtx = context.WithValue(newCtx, ProbeIDKey, probeID)
	newCtx = context.WithValue(newCtx, ScopesKey, tokenInfo.Scopes)
	newCtx = context.WithValue(newCtx, TokenInfoKey, tokenInfo)

	wrapped := &wrappedServerStream{
		ServerStream: ss,
		ctx:          newCtx,
	}

	i.logger.Debug("Stream authenticated",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("method", info.FullMethod),
		zap.Strings("scopes", tokenInfo.Scopes))

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

// checkProbePermissions 检查探针权限
func (i *Interceptor) checkProbePermissions(tokenInfo *TokenInfo, method string) error {
	if tokenInfo == nil {
		return fmt.Errorf("no token info available")
	}

	// 根据方法确定所需权限
	requiredScopes := i.getRequiredScopesForMethod(method)

	// 检查是否有任一所需权限
	for _, required := range requiredScopes {
		if tokenInfo.HasScope(required) {
			return nil
		}
	}

	return fmt.Errorf("permission denied: required scopes %v, got %v", requiredScopes, tokenInfo.Scopes)
}

// getRequiredScopesForMethod 根据方法获取所需权限
func (i *Interceptor) getRequiredScopesForMethod(method string) []string {
	// 根据方法名判断所需权限
	switch {
	case strings.Contains(method, "UploadFlows"):
		return []string{ScopeIngestWrite, "*"}
	case strings.Contains(method, "StreamFlows"):
		return []string{ScopeIngestWrite, "*"}
	case strings.Contains(method, "UploadPcapIndex"):
		return []string{ScopePcapWrite, ScopeIngestWrite, "*"}
	case strings.Contains(method, "Heartbeat"):
		return []string{ScopeIngestRead, ScopeIngestWrite, "*"}
	default:
		return i.config.RequiredScopes
	}
}

// resolveTenantIDFallback 解析租户 ID 的降级方法
func (i *Interceptor) resolveTenantIDFallback(probeID string) *TokenInfo {
	// 1. 尝试从 probe_id 提取
	tenantID := extractTenantFromProbeID(probeID)
	if tenantID != "" {
		return &TokenInfo{
			TenantID: tenantID,
			ProbeID:  probeID,
			Scopes:   []string{ScopeIngestWrite}, // 降级时给予默认权限
		}
	}

	// 2. 使用默认租户（如果配置了）
	if i.config.DefaultTenantID != "" {
		i.logger.Debug("Using default tenant ID",
			zap.String("probe_id", probeID),
			zap.String("default_tenant_id", i.config.DefaultTenantID))
		return &TokenInfo{
			TenantID: i.config.DefaultTenantID,
			ProbeID:  probeID,
			Scopes:   []string{ScopeIngestWrite},
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

	if len(tlsInfo.State.VerifiedChains) == 0 ||
		len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return "", fmt.Errorf("no verified certificate chains")
	}

	cert := tlsInfo.State.VerifiedChains[0][0]
	probeID := cert.Subject.CommonName
	if probeID == "" {
		return "", fmt.Errorf("empty CommonName in certificate")
	}

	return probeID, nil
}

// extractAndValidateToken 提取并验证 Token（返回完整信息）
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
	if len(token) > 7 && strings.EqualFold(token[:7], "Bearer ") {
		token = token[7:]
	}

	// 验证 Token 并获取完整信息
	tokenInfo, err := i.tokenCache.ValidateWithScopes(ctx, probeID, token)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	return tokenInfo, nil
}

// extractClientIP 提取客户端 IP
func (i *Interceptor) extractClientIP(ctx context.Context) string {
	// 从 metadata 获取（如果有代理）
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if xff := getFirstMetadataValue(md, "x-forwarded-for", "x-real-ip"); xff != "" {
			// 取第一个 IP
			if idx := strings.Index(xff, ","); idx > 0 {
				return strings.TrimSpace(xff[:idx])
			}
			return xff
		}
	}

	// 从 peer 获取
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

	if i.auditCallback != nil {
		i.auditCallback(ctx, &AuditEvent{
			EventType: eventType,
			TenantID:  tenantID,
			ProbeID:   probeID,
			Method:    method,
			ClientIP:  clientIP,
			Error:     errMsg,
			Scopes:    scopes,
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
		if s == scope || s == "*" {
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

	if !strings.HasPrefix(probeID, "probe-") {
		return ""
	}

	rest := probeID[6:]
	lastDash := strings.LastIndex(rest, "-")
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
		if !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' ||
			c == '-') {
			return false
		}
	}

	return true
}

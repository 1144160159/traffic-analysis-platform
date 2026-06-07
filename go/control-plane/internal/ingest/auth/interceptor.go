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

type contextKey string

const (
	TenantIDKey  contextKey = "tenant_id"
	ProbeIDKey   contextKey = "probe_id"
	ScopesKey    contextKey = "scopes"
	TokenInfoKey contextKey = "token_info"
)

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

type TokenInfo struct {
	TenantID  string
	ProbeID   string
	Scopes    []string
	ExpiresAt int64
}

func (t *TokenInfo) HasScope(scope string) bool {
	for _, s := range t.Scopes {
		if s == scope || s == config.ScopeWildcard {
			return true
		}
	}
	return false
}

func (t *TokenInfo) HasAnyScope(scopes ...string) bool {
	for _, scope := range scopes {
		if t.HasScope(scope) {
			return true
		}
	}
	return false
}

type Interceptor struct {
	logger      *zap.Logger
	tokenCache  *TokenCache
	limiter     *quota.Limiter
	config      InterceptorConfig
	auditLogger *audit.Logger
}

type AuditEvent struct {
	EventType string
	TenantID  string
	ProbeID   string
	Method    string
	ClientIP  string
	Error     string
	Scopes    []string
}

func NewInterceptor(logger *zap.Logger, tokenCache *TokenCache, config1 InterceptorConfig) *Interceptor {

	if len(config1.RequiredScopes) == 0 {
		config1.RequiredScopes = []string{config.ScopeIngestWrite}
	}

	return &Interceptor{
		logger:     logger,
		tokenCache: tokenCache,
		config:     config1,
	}
}

func (i *Interceptor) SetLimiter(limiter *quota.Limiter) {
	i.limiter = limiter
}

func (i *Interceptor) SetAuditLogger(auditLogger *audit.Logger) {
	i.auditLogger = auditLogger
}

func (i *Interceptor) UnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {

	if config.IsPublicMethod(info.FullMethod) {
		i.logger.Debug("Public method bypassed authentication",
			zap.String("method", info.FullMethod))
		return handler(ctx, req)
	}

	ctx, span := otel.StartSpan(ctx, "auth.unary_interceptor")
	defer span.End()

	clientIP := i.extractClientIP(ctx)

	probeID, err := i.extractProbeID(ctx)
	if err != nil {
		return i.handleAuthError(ctx, "", probeID, info.FullMethod, clientIP,
			errors.Wrap(err, errors.ErrCodeUnauthorized, "probe ID extraction failed"))
	}

	tokenInfo, err := i.extractAndValidateToken(ctx, probeID)
	if err != nil {

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

	if i.config.EnableProbeRBAC && i.config.RequireScopes {
		if err := i.checkProbePermissions(tokenInfo, info.FullMethod); err != nil {
			return i.handleAuthError(ctx, tenantID, probeID, info.FullMethod, clientIP, err)
		}
	}

	if i.config.EnableRateLimit && i.limiter != nil {
		if !i.limiter.Allow(ctx, tenantID, probeID) {
			i.recordAudit(ctx, config.AuditEventTypeAccessDenied, tenantID, probeID, info.FullMethod, clientIP, "rate_limit_exceeded", tokenInfo.Scopes)
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
	}

	ctx = i.enrichContext(ctx, tokenInfo)

	i.recordAudit(ctx, "auth_success", tenantID, probeID, info.FullMethod, clientIP, "", tokenInfo.Scopes)

	i.logger.Debug("Request authenticated",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("method", info.FullMethod),
		zap.Strings("scopes", tokenInfo.Scopes))

	return handler(ctx, req)
}

func (i *Interceptor) StreamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {

	if config.IsPublicMethod(info.FullMethod) {
		i.logger.Debug("Public stream method bypassed authentication",
			zap.String("method", info.FullMethod))
		return handler(srv, ss)
	}

	ctx := ss.Context()
	ctx, span := otel.StartSpan(ctx, "auth.stream_interceptor")
	defer span.End()

	clientIP := i.extractClientIP(ctx)

	probeID, err := i.extractProbeID(ctx)
	if err != nil {
		_, grpcErr := i.handleAuthError(ctx, "", probeID, info.FullMethod, clientIP,
			errors.Wrap(err, errors.ErrCodeUnauthorized, "probe ID extraction failed"))
		return grpcErr
	}

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

	if i.config.EnableProbeRBAC && i.config.RequireScopes {
		if err := i.checkProbePermissions(tokenInfo, info.FullMethod); err != nil {
			_, grpcErr := i.handleAuthError(ctx, tenantID, probeID, info.FullMethod, clientIP, err)
			return grpcErr
		}
	}

	if i.config.EnableRateLimit && i.limiter != nil {
		if !i.limiter.Allow(ctx, tenantID, probeID) {
			i.recordAudit(ctx, config.AuditEventTypeAccessDenied, tenantID, probeID, info.FullMethod, clientIP, "rate_limit_exceeded", tokenInfo.Scopes)
			return status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
	}

	newCtx := i.enrichContext(ctx, tokenInfo)
	wrapped := &wrappedServerStream{
		ServerStream: ss,
		ctx:          newCtx,
	}

	i.recordAudit(ctx, "auth_success", tenantID, probeID, info.FullMethod, clientIP, "", tokenInfo.Scopes)

	return handler(srv, wrapped)
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

func (i *Interceptor) enrichContext(ctx context.Context, tokenInfo *TokenInfo) context.Context {
	ctx = context.WithValue(ctx, TenantIDKey, tokenInfo.TenantID)
	ctx = context.WithValue(ctx, ProbeIDKey, tokenInfo.ProbeID)
	ctx = context.WithValue(ctx, ScopesKey, tokenInfo.Scopes)
	ctx = context.WithValue(ctx, TokenInfoKey, tokenInfo)

	ctx = logging.WithTenantID(ctx, tokenInfo.TenantID)
	ctx = logging.WithProbeID(ctx, tokenInfo.ProbeID)

	otel.AddTenantAttribute(ctx, tokenInfo.TenantID)
	otel.AddProbeAttribute(ctx, tokenInfo.ProbeID)

	return ctx
}

func (i *Interceptor) handleAuthError(ctx context.Context, tenantID, probeID, method, clientIP string, err *errors.AppError) (interface{}, error) {
	logger := logging.L(ctx)

	logger.Warn("Authentication failed",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("method", method),
		zap.String("client_ip", clientIP),
		zap.Error(err))

	i.recordAudit(ctx, config.AuditEventTypeAuthFailure, tenantID, probeID, method, clientIP, err.Error(), nil)

	otel.RecordError(ctx, err)

	return nil, status.Error(grpcCodeFromHTTPStatus(err.HTTPStatus()), err.Message)
}

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

func (i *Interceptor) getRequiredScopesForMethod(method string) []string {

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

func (i *Interceptor) resolveTenantIDFallback(probeID string) *TokenInfo {

	tenantID := extractTenantFromProbeID(probeID)
	if tenantID != "" {
		return &TokenInfo{
			TenantID: tenantID,
			ProbeID:  probeID,
			Scopes:   []string{config.ScopeIngestWrite},
		}
	}

	if i.config.DefaultTenantID != "" {
		return &TokenInfo{
			TenantID: i.config.DefaultTenantID,
			ProbeID:  probeID,
			Scopes:   []string{config.ScopeIngestWrite},
		}
	}

	return nil
}

func (i *Interceptor) extractProbeID(ctx context.Context) (string, error) {

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

func (i *Interceptor) extractAndValidateToken(ctx context.Context, probeID string) (*TokenInfo, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("no metadata in context")
	}

	token := getFirstMetadataValue(md, "x-tenant-token", "authorization", "x-api-key")
	if token == "" {
		return nil, fmt.Errorf("tenant token not found in metadata")
	}

	if len(token) > 7 && (token[:7] == "Bearer " || token[:7] == "bearer ") {
		token = token[7:]
	}

	tokenInfo, err := i.tokenCache.ValidateWithScopes(ctx, probeID, token)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	if tokenInfo.ProbeID != "" && tokenInfo.ProbeID != probeID {
		return nil, fmt.Errorf("token is bound to probe %s, but request came from %s", tokenInfo.ProbeID, probeID)
	}

	return tokenInfo, nil
}

func (i *Interceptor) extractClientIP(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if xff := getFirstMetadataValue(md, "x-forwarded-for", "x-real-ip"); xff != "" {

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

func (i *Interceptor) recordAudit(ctx context.Context, eventType, tenantID, probeID, method, clientIP, errMsg string, scopes []string) {
	if !i.config.EnableAuditLog {
		return
	}

	if i.auditLogger != nil {
		i.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    audit.EventType(eventType),
			TenantID:     tenantID,
			UserID:       probeID,
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

func getFirstMetadataValue(md metadata.MD, keys ...string) string {
	for _, key := range keys {
		values := md.Get(key)
		if len(values) > 0 && values[0] != "" {
			return values[0]
		}
	}
	return ""
}

func GetTenantID(ctx context.Context) string {
	if v := ctx.Value(TenantIDKey); v != nil {
		return v.(string)
	}
	return ""
}

func GetProbeID(ctx context.Context) string {
	if v := ctx.Value(ProbeIDKey); v != nil {
		return v.(string)
	}
	return ""
}

func GetScopes(ctx context.Context) []string {
	if v := ctx.Value(ScopesKey); v != nil {
		return v.([]string)
	}
	return nil
}

func GetTokenInfo(ctx context.Context) *TokenInfo {
	if v := ctx.Value(TokenInfoKey); v != nil {
		return v.(*TokenInfo)
	}
	return nil
}

func HasScope(ctx context.Context, scope string) bool {
	scopes := GetScopes(ctx)
	for _, s := range scopes {
		if s == scope || s == config.ScopeWildcard {
			return true
		}
	}
	return false
}

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

// grpcCodeFromHTTPStatus maps HTTP status codes to gRPC status codes correctly.
// This replaces the incorrect codes.Code(httpStatus/100) pattern.
func grpcCodeFromHTTPStatus(httpStatus int) codes.Code {
	switch {
	case httpStatus == 400:
		return codes.InvalidArgument
	case httpStatus == 401:
		return codes.Unauthenticated
	case httpStatus == 403:
		return codes.PermissionDenied
	case httpStatus == 404:
		return codes.NotFound
	case httpStatus == 409:
		return codes.AlreadyExists
	case httpStatus == 429:
		return codes.ResourceExhausted
	case httpStatus >= 500 && httpStatus < 600:
		if httpStatus == 503 {
			return codes.Unavailable
		}
		if httpStatus == 504 {
			return codes.DeadlineExceeded
		}
		return codes.Internal
	default:
		return codes.Unknown
	}
}

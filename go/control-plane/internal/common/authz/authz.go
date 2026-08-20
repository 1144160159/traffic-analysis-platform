// Package authz 共享鉴权中间件(权限体系方案 §4.3 第二层防御):
// 统一校验 Keycloak 访问令牌(JWKS 验签)并强制租户绑定——
// token 的 tenant_id 声明(缺省 DefaultTenant)必须等于 X-Tenant-ID 头,
// 防止"合法用户 + 伪造租户头"的横向越权(P1)。
// Mode=shadow 时只记录拒绝、不阻断(灰度观察);enforce 时 401/403 fail-closed。
package authz

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"go.uber.org/zap"
)

// Config 中间件配置。
type Config struct {
	JWKSURL       string        // Keycloak 证书端点(公网可匿名)
	Issuer        string        // 为空跳过 issuer 严格校验
	DefaultTenant string        // token 无 tenant_id 声明时的缺省租户
	Mode          string        // shadow | enforce(默认 shadow)
	JWKSRefresh   time.Duration // 密钥缓存刷新(默认 5m)
	ExemptPaths   []string      // 免鉴权精确路径(健康探针等)
	// Fallback 机器凭证校验器(P2-c/ADR-6:api_tokens 统一纳入判定链);
	// 仅当凭证非 JWT(API Key 形态)且 JWKS 校验失败时尝试。
	Fallback TokenValidator
	// RequireTenantClaim ADR-2 零信任:true 时 token 缺失 tenant_id 声明直接拒绝,
	// 不再回落 DefaultTenant(防止"租户属性未配置的用户静默落入 default 租户")。
	RequireTenantClaim bool
	// DenyAuditor 认证拒绝留痕回调(审计三联:401/403 拒绝→审计落库)。
	DenyAuditor DenyAuditFunc
	// AllowedAZP 客户端白名单(非空时强制 azp 声明命中,防任意客户端令牌混用)。
	AllowedAZP []string
}

// DenyAuditFunc 认证拒绝审计回调:r 原始请求、status 拒绝码、reason 拒绝原因、
// principal 可能为 nil(验签失败场景)。
type DenyAuditFunc func(r *http.Request, status int, reason string, principal *Principal)

// TokenValidator 机器凭证回退校验入口(实现方如 internal/auth/apitoken.Validator)。
// 携带原始请求以便执行 IP 白名单等请求级约束。
type TokenValidator func(ctx context.Context, r *http.Request, tokenString string) (*Principal, error)

// Middleware 鉴权中间件。
type Middleware struct {
	cfg    Config
	logger *zap.Logger

	mu      sync.Mutex
	jwks    map[string]*rsa.PublicKey
	fetched time.Time
}

// New 构造中间件。
func New(cfg Config, logger *zap.Logger) *Middleware {
	if cfg.JWKSRefresh <= 0 {
		cfg.JWKSRefresh = 5 * time.Minute
	}
	if cfg.DefaultTenant == "" {
		cfg.DefaultTenant = "default"
	}
	if cfg.Mode == "" {
		cfg.Mode = "shadow"
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Middleware{cfg: cfg, logger: logger, jwks: map[string]*rsa.PublicKey{}}
}

type tenantKey struct{}

// TenantFromContext 读取已绑定租户(业务层使用,不读请求头)。
func TenantFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(tenantKey{}).(string); ok {
		return v
	}
	return ""
}

type principalKey struct{}

// PrincipalFromContext 读取判定主体(共享中间件 Handler 产出)。
func PrincipalFromContext(ctx context.Context) *Principal {
	if v, ok := ctx.Value(principalKey{}).(*Principal); ok {
		return v
	}
	return nil
}

// PrincipalFromRequest 共享中间件场景的 PrincipalProvider。
func PrincipalFromRequest(r *http.Request) *Principal {
	return PrincipalFromContext(r.Context())
}

// MapOIDCRoles 契约驱动的角色映射(ADR-3):语义与 auth-service 完全一致,
// 唯一权威为 contracts/authz/oidc-role-map.v1.json(内嵌副本)。
// 契约不可用时 fail-closed 落到 viewer(零信任:缺契约不放大权限)。
func MapOIDCRoles(oidcRoles []string) []string {
	if m, err := DefaultRoleMap(); err == nil {
		return m.MapRoles(oidcRoles)
	}
	return []string{"viewer"}
}

// Handler 包装 HTTP 处理链。
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerTenant := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
		for _, p := range m.cfg.ExemptPaths {
			if r.URL.Path == p {
				next.ServeHTTP(w, r)
				return
			}
		}
		authHeader := r.Header.Get("Authorization")
		deny := func(status int, msg string) {
			m.logger.Warn("authz deny", zap.Int("status", status), zap.String("reason", msg),
				zap.String("path", r.URL.Path), zap.String("header_tenant", headerTenant))
			if m.cfg.DenyAuditor != nil {
				m.cfg.DenyAuditor(r, status, msg, nil)
			}
			if m.cfg.Mode == "enforce" {
				http.Error(w, msg, status)
				return
			}
			// shadow:不阻断,但把请求租户置空(业务层按空租户 fail-closed 处理由各服务决定;
			// 此处至少保留现状行为并留审计日志)。
			next.ServeHTTP(w, r)
		}

		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			deny(http.StatusUnauthorized, "missing bearer token")
			return
		}
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := m.verifyClaims(tokenString)
		if err != nil {
			// P2-c/ADR-6:JWT 校验失败且凭证为 API Key 形态时回退机器凭证校验,
			// 机器 token 与人类 token 经同一中间件产出 Principal(租户绑定同样强制)。
			if m.cfg.Fallback != nil && !looksLikeJWT(tokenString) {
				if principal, ferr := m.cfg.Fallback(r.Context(), r, tokenString); ferr == nil && principal != nil {
					m.finishWithPrincipal(w, r, next, principal, headerTenant, deny)
					return
				} else {
					m.logger.Warn("api token fallback rejected",
						zap.String("path", r.URL.Path),
						zap.Error(ferr))
					deny(http.StatusUnauthorized, "invalid api token: "+fmt.Sprint(ferr))
					return
				}
			}
			deny(http.StatusUnauthorized, "invalid token: "+err.Error())
			return
		}
		if m.cfg.RequireTenantClaim {
			if t, ok := claims["tenant_id"].(string); !ok || strings.TrimSpace(t) == "" {
				deny(http.StatusForbidden, "token missing required tenant_id claim")
				return
			}
		}
		tokenTenant := tenantFromClaims(claims, m.cfg.DefaultTenant)
		principal := m.principalFromClaims(claims, tokenTenant)
		principal.Kind = PrincipalKindHuman
		m.finishWithPrincipal(w, r, next, principal, headerTenant, deny)
	})
}

// finishWithPrincipal 租户绑定强校验后把主体写入上下文并放行/拒绝。
func (m *Middleware) finishWithPrincipal(w http.ResponseWriter, r *http.Request, next http.Handler, principal *Principal, headerTenant string, deny func(int, string)) {
	tokenTenant := principal.TenantID
	if tokenTenant == "" {
		tokenTenant = m.cfg.DefaultTenant
		principal.TenantID = tokenTenant
	}
	if headerTenant == "" {
		headerTenant = m.cfg.DefaultTenant
	}
	if tokenTenant != headerTenant {
		deny(http.StatusForbidden, fmt.Sprintf("tenant mismatch: token=%s header=%s", tokenTenant, headerTenant))
		return
	}
	ctx := context.WithValue(r.Context(), tenantKey{}, tokenTenant)
	ctx = context.WithValue(ctx, principalKey{}, principal)
	// 业务层/审计链兼容:主体信息同时注入 httpx 上下文键,
	// 使审计 actor 直接呈现 api-token:<id>(ADR-6)。
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, principal.Subject)
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, tokenTenant)
	ctx = context.WithValue(ctx, httpx.ContextKeyUsername, principal.Username)
	ctx = context.WithValue(ctx, httpx.ContextKeyRoles, principal.Roles)
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, principal.Permissions)
	next.ServeHTTP(w, r.WithContext(ctx))
}

// looksLikeJWT 启发式:JWT 由三段 base64url 组成,API Key 不是。
func looksLikeJWT(tokenString string) bool {
	return strings.Count(tokenString, ".") == 2
}

// principalFromClaims 由 OIDC 声明构建判定主体:traffic-* 领域角色 -> 内部角色,
// 内部角色的权限集合直接取自 m10 契约(判定合一 P1:权限是唯一执行语言)。
func (m *Middleware) principalFromClaims(claims jwt.MapClaims, tenant string) *Principal {
	oidcRoles := extractOIDCRoles(claims)
	roles := MapOIDCRoles(oidcRoles)
	principal := &Principal{
		Subject:  stringClaim(claims, "sub"),
		Username: stringClaim(claims, "preferred_username"),
		TenantID: tenant,
		Roles:    roles,
	}
	policy, err := DefaultPolicy()
	if err != nil {
		m.logger.Warn("contract policy unavailable, permissions empty (contract ops fail-closed)",
			zap.Error(err))
		return principal
	}
	seen := map[string]struct{}{}
	for _, role := range roles {
		if r, ok := policy.Roles[role]; ok {
			for _, s := range r.Scopes {
				if _, dup := seen[s]; !dup {
					seen[s] = struct{}{}
					principal.Permissions = append(principal.Permissions, s)
				}
			}
		}
	}
	return principal
}

// extractOIDCRoles 收集 realm_access.roles 与全部 resource_access 客户端角色
// (该 Keycloak 部署的角色位于访问令牌,ID 令牌无角色声明)。
func extractOIDCRoles(claims jwt.MapClaims) []string {
	var roles []string
	if ra, ok := claims["realm_access"].(map[string]interface{}); ok {
		if list, ok := ra["roles"].([]interface{}); ok {
			for _, r := range list {
				if s, ok := r.(string); ok {
					roles = append(roles, s)
				}
			}
		}
	}
	if res, ok := claims["resource_access"].(map[string]interface{}); ok {
		for _, v := range res {
			if ra, ok := v.(map[string]interface{}); ok {
				if list, ok := ra["roles"].([]interface{}); ok {
					for _, r := range list {
						if s, ok := r.(string); ok {
							roles = append(roles, s)
						}
					}
				}
			}
		}
	}
	return roles
}

func tenantFromClaims(claims jwt.MapClaims, defaultTenant string) string {
	if tenant, ok := claims["tenant_id"].(string); ok && tenant != "" {
		return tenant
	}
	return defaultTenant
}

func stringClaim(claims jwt.MapClaims, key string) string {
	if v, ok := claims[key].(string); ok {
		return v
	}
	return ""
}

// verifyClaims JWKS 验签并返回声明(供租户绑定与主体构建共用)。
func (m *Middleware) verifyClaims(tokenString string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, claims)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	kid, _ := token.Header["kid"].(string)
	key, err := m.keyFor(kid)
	if err != nil {
		return nil, err
	}
	if _, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected alg %s", t.Method.Alg())
		}
		return key, nil
	}); err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}
	// 令牌形态约束:仅接受访问令牌。Keycloak ID 令牌(typ=ID)与刷新令牌
	// (typ=Refresh)即使 RS256 验签通过也不得作为业务凭证;typ 缺省(部分
	// 部署不携带该声明)时放行以保持兼容,由 iss/租户/过期校验兜底。
	if typ, ok := claims["typ"].(string); ok && typ != "" && typ != "Bearer" {
		return nil, fmt.Errorf("unexpected token type %q: access token required", typ)
	}
	if m.cfg.Issuer != "" {
		if iss, _ := claims["iss"].(string); iss != m.cfg.Issuer {
			return nil, fmt.Errorf("issuer mismatch: %s", iss)
		}
	}
	if len(m.cfg.AllowedAZP) > 0 {
		azp, _ := claims["azp"].(string)
		ok := false
		for _, a := range m.cfg.AllowedAZP {
			if azp == a {
				ok = true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("azp %q not in allowed client whitelist", azp)
		}
	}
	return claims, nil
}

// verifyAndTenant 兼容旧入口:JWKS 验签并提取租户声明(缺失按 DefaultTenant)。
func (m *Middleware) verifyAndTenant(tokenString string) (string, error) {
	claims, err := m.verifyClaims(tokenString)
	if err != nil {
		return "", err
	}
	return tenantFromClaims(claims, m.cfg.DefaultTenant), nil
}

// keyFor 按 kid 取公钥(缓存过期刷新)。
// 修复:此前持锁执行 JWKS 网络 IO(最长 10s),刷新期间阻塞所有验签请求;
// 现改为锁内只做缓存判定/读取,网络刷新在锁外执行(refresh 内部自锁写缓存)。
func (m *Middleware) keyFor(kid string) (*rsa.PublicKey, error) {
	m.mu.Lock()
	stale := time.Since(m.fetched) > m.cfg.JWKSRefresh || len(m.jwks) == 0
	if !stale {
		if k, ok := m.jwks[kid]; ok {
			m.mu.Unlock()
			return k, nil
		}
	}
	m.mu.Unlock()

	if err := m.refresh(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if k, ok := m.jwks[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("no key for kid %s", kid)
}

func (m *Middleware) refresh() error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(m.cfg.JWKSURL)
	if err != nil {
		return fmt.Errorf("jwks fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return fmt.Errorf("jwks decode: %w", err)
	}
	fresh := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 | int(b)
		}
		fresh[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	}
	if len(fresh) == 0 {
		return fmt.Errorf("jwks empty")
	}
	m.mu.Lock()
	m.jwks = fresh
	m.fetched = time.Now()
	m.mu.Unlock()
	return nil
}

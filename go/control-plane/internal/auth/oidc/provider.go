// //////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/oidc/provider.go
// OIDC Provider - 修复版
// 修复内容：
// 1. JWKS 刷新竞态条件修复，使用 sync.Once 模式
// 2. 增强错误处理
// //////////////////////////////////////////////////////////////////////////////
package oidc

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// Provider OIDC身份提供者
type Provider struct {
	config     config.OIDCConfig
	httpClient *http.Client
	logger     *zap.Logger
	// OIDC Discovery
	issuer      string
	authURL     string
	tokenURL    string
	userInfoURL string
	jwksURL     string
	// JWKS cache（修复：使用原子操作和互斥锁）
	jwks           atomic.Value // *JWKS
	jwksExpiry     atomic.Value // time.Time
	jwksMu         sync.Mutex   // 用于刷新操作的互斥锁
	jwksRefreshing int32        // 原子标志，防止并发刷新
}

// JWKS JSON Web Key Set
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK JSON Web Key
type JWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Alg string `json:"alg"`
}

// OIDCDiscovery OIDC发现文档
type OIDCDiscovery struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserInfoEndpoint      string   `json:"userinfo_endpoint"`
	JwksURI               string   `json:"jwks_uri"`
	ScopesSupported       []string `json:"scopes_supported"`
}

// TokenResponse Token响应
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// NewProvider 创建OIDC Provider
func NewProvider(cfg config.OIDCConfig, logger *zap.Logger) (*Provider, error) {
	p := &Provider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
	// 初始化原子值
	p.jwksExpiry.Store(time.Time{})
	// Discover OIDC endpoints
	if err := p.discover(); err != nil {
		return nil, fmt.Errorf("OIDC discovery failed: %w", err)
	}
	// Load initial JWKS
	if err := p.refreshJWKS(); err != nil {
		logger.Warn("Failed to load initial JWKS", zap.Error(err))
	}
	return p, nil
}

// discover 发现OIDC端点
func (p *Provider) discover() error {
	discoveryURL := strings.TrimSuffix(p.config.IssuerURL, "/") + "/.well-known/openid-configuration"
	resp, err := p.httpClient.Get(discoveryURL)
	if err != nil {
		return fmt.Errorf("failed to fetch discovery document: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery endpoint returned status %d", resp.StatusCode)
	}
	var discovery OIDCDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return fmt.Errorf("failed to decode discovery document: %w", err)
	}
	p.issuer = discovery.Issuer
	p.authURL = discovery.AuthorizationEndpoint
	p.tokenURL = discovery.TokenEndpoint
	p.userInfoURL = discovery.UserInfoEndpoint
	p.jwksURL = discovery.JwksURI
	p.logger.Info("OIDC discovery completed",
		zap.String("issuer", p.issuer),
		zap.String("auth_url", p.authURL))
	return nil
}

// refreshJWKS 刷新JWKS（修复：使用互斥锁和原子标志防止并发刷新）
func (p *Provider) refreshJWKS() error {
	// 使用原子操作检查是否已有刷新在进行
	if !atomic.CompareAndSwapInt32(&p.jwksRefreshing, 0, 1) {
		// 已有刷新在进行，等待完成
		p.jwksMu.Lock()
		p.jwksMu.Unlock()
		return nil
	}
	// 获取锁并确保在完成后释放
	p.jwksMu.Lock()
	defer func() {
		atomic.StoreInt32(&p.jwksRefreshing, 0)
		p.jwksMu.Unlock()
	}()
	// 双重检查：在获取锁后再次检查是否需要刷新
	expiry, ok := p.jwksExpiry.Load().(time.Time)
	if ok && time.Now().Before(expiry) {
		return nil
	}
	resp, err := p.httpClient.Get(p.jwksURL)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}
	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode JWKS: %w", err)
	}
	// 原子更新 JWKS 和过期时间
	p.jwks.Store(&jwks)
	p.jwksExpiry.Store(time.Now().Add(1 * time.Hour))
	p.logger.Debug("JWKS refreshed", zap.Int("key_count", len(jwks.Keys)))
	return nil
}

// ensureJWKS 确保 JWKS 已加载且未过期
func (p *Provider) ensureJWKS() error {
	expiry, ok := p.jwksExpiry.Load().(time.Time)
	needsRefresh := !ok || time.Now().After(expiry)
	if needsRefresh {
		return p.refreshJWKS()
	}
	return nil
}

// getJWKS 获取当前 JWKS
func (p *Provider) getJWKS() *JWKS {
	if jwks, ok := p.jwks.Load().(*JWKS); ok {
		return jwks
	}
	return nil
}

// GetAuthURL 获取认证URL
func (p *Provider) GetAuthURL(state string) string {
	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", p.config.RedirectURL)
	params.Set("response_type", "code")
	params.Set("scope", normalizeScopes(p.config.Scopes))
	params.Set("state", state)
	return p.authURL + "?" + params.Encode()
}

func normalizeScopes(scopes string) string {
	parts := strings.Fields(strings.ReplaceAll(scopes, ",", " "))
	return strings.Join(parts, " ")
}

// ExchangeCode 用授权码换取令牌
func (p *Provider) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", p.config.RedirectURL)
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, "POST", p.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}
	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	return &tokenResp, nil
}

// RefreshToken 刷新令牌
func (p *Provider) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, "POST", p.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("refresh endpoint returned %d: %s", resp.StatusCode, string(body))
	}
	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	return &tokenResp, nil
}

// ValidateIDToken 验证ID Token
func (p *Provider) ValidateIDToken(tokenString string) (*model.OIDCClaims, error) {
	// 确保 JWKS 已加载
	if err := p.ensureJWKS(); err != nil {
		p.logger.Error("Failed to ensure JWKS", zap.Error(err))
	}
	// Parse token without validation to get kid
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &model.OIDCClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("token missing kid header")
	}
	// Find matching key
	jwks := p.getJWKS()
	if jwks == nil {
		return nil, fmt.Errorf("JWKS not available")
	}
	var matchingKey *JWK
	for i := range jwks.Keys {
		if jwks.Keys[i].Kid == kid {
			matchingKey = &jwks.Keys[i]
			break
		}
	}
	if matchingKey == nil {
		// Key not found, try refreshing JWKS
		p.jwksExpiry.Store(time.Time{})
		if err := p.refreshJWKS(); err != nil {
			return nil, fmt.Errorf("failed to refresh JWKS: %w", err)
		}
		jwks = p.getJWKS()
		if jwks != nil {
			for i := range jwks.Keys {
				if jwks.Keys[i].Kid == kid {
					matchingKey = &jwks.Keys[i]
					break
				}
			}
		}
		if matchingKey == nil {
			return nil, fmt.Errorf("no matching key found for kid: %s", kid)
		}
	}
	// Parse RSA public key
	publicKey, err := parseRSAPublicKey(matchingKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}
	// Parse and validate token
	claims := &model.OIDCClaims{}
	token, err = jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		return publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("token is invalid")
	}
	// Verify issuer
	if claims.Issuer != p.issuer {
		return nil, fmt.Errorf("invalid issuer: expected %s, got %s", p.issuer, claims.Issuer)
	}
	return claims, nil
}

// GetUserInfo 获取用户信息
func (p *Provider) GetUserInfo(ctx context.Context, accessToken string) (*model.OIDCClaims, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned %d", resp.StatusCode)
	}
	var claims model.OIDCClaims
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo: %w", err)
	}
	return &claims, nil
}

// parseRSAPublicKey 从JWK解析RSA公钥
func parseRSAPublicKey(jwk *JWK) (*rsa.PublicKey, error) {
	if jwk.Kty != "RSA" {
		return nil, fmt.Errorf("unsupported key type: %s", jwk.Kty)
	}
	// Decode N (modulus)
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode N: %w", err)
	}
	// Decode E (exponent)
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode E: %w", err)
	}
	// Convert bytes to big integers
	n := new(big.Int).SetBytes(nBytes)
	// Convert exponent bytes to int
	var eInt int
	for _, b := range eBytes {
		eInt = eInt<<8 + int(b)
	}
	return &rsa.PublicKey{
		N: n,
		E: eInt,
	}, nil
}

// GetIssuer 获取Issuer
func (p *Provider) GetIssuer() string {
	return p.issuer
}

// GetAuthEndpoint 获取认证端点
func (p *Provider) GetAuthEndpoint() string {
	return p.authURL
}

// GetTokenEndpoint 获取Token端点
func (p *Provider) GetTokenEndpoint() string {
	return p.tokenURL
}

// GetUserInfoEndpoint 获取UserInfo端点
func (p *Provider) GetUserInfoEndpoint() string {
	return p.userInfoURL
}

// IsConfigured 检查是否已配置
func (p *Provider) IsConfigured() bool {
	return p.issuer != "" && p.authURL != "" && p.tokenURL != ""
}

// ForceRefreshJWKS 强制刷新 JWKS（用于管理操作）
func (p *Provider) ForceRefreshJWKS() error {
	// 清除过期时间，强制刷新
	p.jwksExpiry.Store(time.Time{})
	return p.refreshJWKS()
}

// GetJWKSInfo 获取 JWKS 信息（用于调试）
func (p *Provider) GetJWKSInfo() map[string]interface{} {
	jwks := p.getJWKS()
	expiry, _ := p.jwksExpiry.Load().(time.Time)
	info := map[string]interface{}{
		"expiry":     expiry,
		"is_expired": time.Now().After(expiry),
	}
	if jwks != nil {
		keyIDs := make([]string, 0, len(jwks.Keys))
		for _, key := range jwks.Keys {
			keyIDs = append(keyIDs, key.Kid)
		}
		info["key_count"] = len(jwks.Keys)
		info["key_ids"] = keyIDs
	} else {
		info["key_count"] = 0
		info["key_ids"] = []string{}
	}
	return info
}

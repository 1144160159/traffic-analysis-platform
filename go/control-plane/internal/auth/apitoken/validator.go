// Package apitoken API Token 统一校验器(P2-c / ADR-6):
// 把 api_tokens 表纳入共享鉴权中间件的判定链——机器凭证与人类 OIDC 凭证
// 经同一 authz 中间件产出 Principal(token_type 区分),审计 actor=api-token:<id>。
package apitoken

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/security"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/authz"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

// apiKeyFormat 限定回退尝试的凭证形态:前缀字母段 + 随机段(可再带一段)。
// JWT 形态的凭证不会进入该路径。
var apiKeyFormat = regexp.MustCompile(`^[a-z]{2,6}_[a-z0-9]{4,24}(_[a-z0-9]{4,24}){0,2}$`)

// LooksLikeAPIKey 判断凭证是否为 API Key 形态(供中间件决定是否回退)。
func LooksLikeAPIKey(tokenString string) bool {
	return apiKeyFormat.MatchString(tokenString)
}

// Validator 基于 api_tokens 表的机器凭证校验器(实现 authz.TokenValidator)。
type Validator struct {
	repo       *repository.TokenRepository
	shaHasher  *security.TokenHasher  // 现役契约:api_tokens.token_hash = sha256 十六进制
	bcryptHash *security.APIKeyHasher // 历史行:$2a$ bcrypt(仅 2 行存量)
	logger     *zap.Logger
}

// NewValidator 构造校验器。
func NewValidator(repo *repository.TokenRepository, logger *zap.Logger) *Validator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Validator{repo: repo, shaHasher: security.NewTokenHasher(), bcryptHash: security.NewAPIKeyHasher(), logger: logger}
}

// Validate 校验 API Key 并产出判定主体(实现 authz.TokenValidator):
// 前缀行查询 + sha256/bcrypt 双方案比对,命中后执行 IP 白名单约束
// (api_tokens.ip_whitelist,空白名单=不限;失败关闭:未命中一律无效)。
func (v *Validator) Validate(ctx context.Context, r *http.Request, tokenString string) (*authz.Principal, error) {
	if v.repo == nil || tokenString == "" {
		return nil, fmt.Errorf("api token validation unavailable")
	}
	tokens, err := v.repo.GetActiveByTokenPrefixMatch(ctx, tokenString)
	if err != nil {
		return nil, fmt.Errorf("api token lookup: %w", err)
	}
	for _, token := range tokens {
		if token == nil || token.TokenHash == "" {
			continue
		}
		if len(token.TokenHash) == 64 {
			if err := v.shaHasher.VerifyToken(token.TokenHash, tokenString); err != nil {
				continue
			}
		} else {
			// 存量 bcrypt 行兼容
			if err := v.bcryptHash.VerifyToken(token.TokenHash, tokenString); err != nil {
				continue
			}
		}
		if token.IsExpired() {
			return nil, fmt.Errorf("api token expired")
		}
		if r != nil && !token.CanAccessFromIP(httpx.GetClientIP(r)) {
			return nil, fmt.Errorf("api token rejected by ip whitelist")
		}
		// 使用统计(尽力而为,不影响判定)
		updateCtx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		_ = v.repo.UpdateUsageStats(updateCtx, token.TokenID)
		cancel()
		return &authz.Principal{
			Kind:        authz.PrincipalKindAPIToken,
			Subject:     "api-token:" + token.TokenID.String(),
			Username:    token.Name,
			TenantID:    token.TenantID,
			Roles:       nil,
			Permissions: token.Scopes,
		}, nil
	}
	return nil, fmt.Errorf("api token not found or invalid")
}

// candidatePrefixes 形态校验辅助:合法 API Key 至少为前缀段+随机段(≥8 位)。
func candidatePrefixes(tokenString string) []string {
	parts := strings.SplitN(tokenString, "_", 3)
	if len(parts) < 2 || len(parts[1]) < 8 {
		return nil
	}
	return []string{parts[0] + "_" + parts[1][:8]}
}

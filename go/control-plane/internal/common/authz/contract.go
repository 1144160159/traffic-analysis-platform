// 契约解释器(P2):m10-minimal-role-policy.v1.json 是运行时唯一判权依据。
// 每个 operation 的 required_scope 是执行语言(判定合一 P1),authorized_roles
// 保留为审计/交叉校验信息。中间件 EnforceContract 对"命中契约"的请求做
// 逐操作判定;未命中契约的请求不受影响(灰度/逐步接入)。
package authz

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/zap"
)

//go:embed policy/m10-minimal-role-policy.v1.json
var embeddedPolicyJSON []byte

// policySourceSHA256 是契约真源 contracts/authz/m10-minimal-role-policy.v1.json
// 的 SHA-256。DefaultPolicy 在启动期校验内嵌副本与真源哈希一致,防止双副本漂移
// (fail-closed);真源文件本身由 policy_integrity_test.go 在仓库内逐字节校验。
const policySourceSHA256 = "035d3ae718a40aba766054318ed3bcf1c502934fddebf904e4f0c328aec85548"

// Operation 契约中的单个操作(运行时判定所需的字段)。
type Operation struct {
	OperationID     string   `json:"operation_id"`
	Path            string   `json:"path"`
	Method          string   `json:"method"`
	Action          string   `json:"action"`
	RequiredScope   string   `json:"required_scope"`
	AuthorizedRoles []string `json:"authorized_roles"`
}

// Role 契约角色:role_id -> scopes(用于把角色映射为权限集合)。
type Role struct {
	RoleID      string   `json:"role_id"`
	Scopes      []string `json:"scopes"`
	CrossTenant bool     `json:"cross_tenant"`
}

// Policy m10 契约的运行时表示。
type Policy struct {
	SchemaVersion string
	Operations    map[string]*Operation // key: METHOD + " " + path
	Roles         map[string]*Role      // key: role_id
	OpCount       int
}

var (
	policyOnce    sync.Once
	defaultPolicy *Policy
	policyErr     error
)

// DefaultPolicy 进程内单例契约(go:embed 内置,与 contracts/authz 副本同源)。
// 启动期校验内嵌副本与契约真源哈希一致,漂移时 fail-closed。
func DefaultPolicy() (*Policy, error) {
	policyOnce.Do(func() {
		sum := sha256.Sum256(embeddedPolicyJSON)
		if got := hex.EncodeToString(sum[:]); got != policySourceSHA256 {
			policyErr = fmt.Errorf("embedded policy hash %s does not match source contract %s (dual-copy drift)", got, policySourceSHA256)
			return
		}
		defaultPolicy, policyErr = ParsePolicy(embeddedPolicyJSON)
	})
	return defaultPolicy, policyErr
}

// ParsePolicy 解析契约 JSON。
func ParsePolicy(raw []byte) (*Policy, error) {
	var doc struct {
		SchemaVersion int `json:"schema_version"`
		Operations    []struct {
			OperationID     string   `json:"operation_id"`
			Path            string   `json:"path"`
			Method          string   `json:"method"`
			Action          string   `json:"action"`
			RequiredScope   string   `json:"required_scope"`
			AuthorizedRoles []string `json:"authorized_roles"`
		} `json:"operations"`
		Roles []struct {
			RoleID      string   `json:"role_id"`
			Scopes      []string `json:"scopes"`
			CrossTenant bool     `json:"cross_tenant"`
		} `json:"roles"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("policy decode: %w", err)
	}
	p := &Policy{
		SchemaVersion: strconv.Itoa(doc.SchemaVersion),
		Operations:    make(map[string]*Operation, len(doc.Operations)),
		Roles:         make(map[string]*Role, len(doc.Roles)),
		OpCount:       len(doc.Operations),
	}
	for i := range doc.Operations {
		o := doc.Operations[i]
		if o.RequiredScope == "" {
			return nil, fmt.Errorf("operation %s missing required_scope", o.OperationID)
		}
		if err := validateScopeGrammar(o.RequiredScope); err != nil {
			return nil, fmt.Errorf("operation %s: %w", o.OperationID, err)
		}
		p.Operations[opKey(o.Method, o.Path)] = &Operation{
			OperationID:     o.OperationID,
			Path:            o.Path,
			Method:          o.Method,
			Action:          o.Action,
			RequiredScope:   o.RequiredScope,
			AuthorizedRoles: o.AuthorizedRoles,
		}
	}
	for _, r := range doc.Roles {
		for _, sc := range r.Scopes {
			if err := validateScopeGrammar(sc); err != nil {
				return nil, fmt.Errorf("role %s: %w", r.RoleID, err)
			}
		}
		p.Roles[r.RoleID] = &Role{RoleID: r.RoleID, Scopes: r.Scopes, CrossTenant: r.CrossTenant}
	}
	return p, nil
}

// flatScopeGrammar 是表驱动解释器支持的扁平 scope 文法:svc:res[:sub...],
// 段允许小写字母/数字/-/_ 与整段通配 *。出现括号/运算符等表达式语法说明
// 契约已超出解释器适用范围,必须转向表达式引擎而不是继续膨胀解释器分支。
var flatScopeGrammar = regexp.MustCompile(`^[a-z][a-z0-9_-]*(:[a-z0-9*_-]+)+$`)

func validateScopeGrammar(scope string) error {
	if scope == "" {
		return errors.New("empty scope")
	}
	if flatScopeGrammar.MatchString(scope) {
		return nil
	}
	return fmt.Errorf("scope %q is outside the approved flat grammar; complex expressions must move to an expression engine", scope)
}

func opKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + path
}

// OperationFor 按方法+路径查契约操作。服务侧路径带 /api 前缀而契约不带,
// 因此 /api/v1/... 命中失败时自动剥 /api 重试。契约路径支持 {param} 模板
// 段(与任意非空段匹配,段数必须一致),如 /v1/alerts/{id}/feedback。
func (p *Policy) OperationFor(method, path string) *Operation {
	if op, ok := p.Operations[opKey(method, path)]; ok {
		return op
	}
	candidates := []string{path}
	if strings.HasPrefix(path, "/api") {
		candidates = append(candidates, strings.TrimPrefix(path, "/api"))
	}
	for _, cand := range candidates {
		if op := p.matchTemplated(method, cand); op != nil {
			return op
		}
	}
	return nil
}

// matchTemplated 对同一方法的所有契约操作做模板段匹配(段数一致 +
// 逐段相等或契约段为 {param});优先精确段多的模板(最长前缀匹配)。
func (p *Policy) matchTemplated(method, path string) *Operation {
	reqSegs := splitPathSegments(path)
	keyPrefix := strings.ToUpper(strings.TrimSpace(method)) + " "
	var best *Operation
	bestStatic := -1
	for key, op := range p.Operations {
		if !strings.HasPrefix(key, keyPrefix) {
			continue
		}
		opSegs := splitPathSegments(op.Path)
		if len(opSegs) != len(reqSegs) {
			continue
		}
		static := 0
		match := true
		for i := range opSegs {
			if isTemplateSegment(opSegs[i]) {
				if reqSegs[i] == "" {
					match = false
					break
				}
				continue
			}
			if opSegs[i] != reqSegs[i] {
				match = false
				break
			}
			static++
		}
		if match && static > bestStatic {
			best, bestStatic = op, static
		}
	}
	return best
}

func splitPathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "/")
}

func isTemplateSegment(seg string) bool {
	return len(seg) > 2 && seg[0] == '{' && seg[len(seg)-1] == '}'
}

// Principal 判定主体(认证层产出,判定层消费)。
type Principal struct {
	Kind        string // human(Keycloak OIDC)| api-token(机器凭证,ADR-6)
	Subject     string
	Username    string
	TenantID    string
	Roles       []string
	Permissions []string
}

// PrincipalKind 常量。
const (
	PrincipalKindHuman    = "human"
	PrincipalKindAPIToken = "api-token"
)

// ScopeCovers held 权限是否覆盖 required 作用域:
// 精确相等、"svc:*" 通配前缀、全量 "*" 三种形式。
func ScopeCovers(held, required string) bool {
	if required == "admin:cross-tenant" {
		// 设计明确:该作用域不对任何角色下发(leastPrivilegeAdminScopes 已排除),
		// 通配符亦不覆盖 —— 跨租户特权的纵深防御。
		return false
	}
	if held == required || held == "*" {
		return true
	}
	if strings.HasSuffix(held, ":*") {
		prefix := strings.TrimSuffix(held, ":*")
		return strings.HasPrefix(required, prefix+":")
	}
	return false
}

// HasScope 主体是否持有覆盖 required 的任意权限。
func (p *Principal) HasScope(required string) bool {
	for _, perm := range p.Permissions {
		if ScopeCovers(perm, required) {
			return true
		}
	}
	return false
}

// PrincipalProvider 从请求还原判定主体(共享中间件或各服务自有认证层适配)。
type PrincipalProvider func(*http.Request) *Principal

// ContractDenyAuditor 契约拒绝审计回调(审计三联:拒绝→留痕)。
type ContractDenyAuditor func(r *http.Request, op *Operation, principal *Principal, status int)

// EnforceContract 逐操作判定中间件:命中契约操作时按 required_scope 判定,
// 未命中契约的路径原样放行。mode: enforce(拒绝) | shadow(只审计)。
// 可选的 auditDeny 回调在拒绝(403/401)发生时被调用(best-effort 审计留痕)。
func EnforceContract(provider PrincipalProvider, mode string, ignorePaths []string, logger *zap.Logger, auditDeny ...ContractDenyAuditor) func(http.Handler) http.Handler {
	policy, err := DefaultPolicy()
	if logger == nil {
		logger = zap.NewNop()
	}
	if mode == "" {
		mode = "shadow"
	}
	ignore := map[string]struct{}{}
	for _, p := range ignorePaths {
		ignore[p] = struct{}{}
	}
	if err != nil {
		logger.Error("contract policy unavailable, fail-closed on matched ops", zap.Error(err))
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			op := (*Operation)(nil)
			if err == nil {
				op = policy.OperationFor(r.Method, r.URL.Path)
			}
			if op == nil {
				next.ServeHTTP(w, r)
				return
			}
			if _, skipped := ignore[r.URL.Path]; skipped {
				next.ServeHTTP(w, r)
				return
			}
			principal := (*Principal)(nil)
			if provider != nil {
				principal = provider(r)
			}
			if principal == nil {
				logContract(logger, r, op, nil, "no principal", http.StatusUnauthorized)
				emitContractDenyAudit(auditDeny, r, op, nil, http.StatusUnauthorized)
				if mode == "enforce" {
					http.Error(w, `{"code":"AUTHZ_401","message":"contract operation requires authenticated principal"}`, http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if !principal.HasScope(op.RequiredScope) {
				logContract(logger, r, op, principal, fmt.Sprintf("missing scope %s", op.RequiredScope), http.StatusForbidden)
				emitContractDenyAudit(auditDeny, r, op, principal, http.StatusForbidden)
				if mode == "enforce" {
					http.Error(w, `{"code":"AUTHZ_403","message":"contract scope required: `+op.RequiredScope+`"}`, http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func logContract(logger *zap.Logger, r *http.Request, op *Operation, principal *Principal, reason string, status int) {
	fields := []zap.Field{
		zap.String("operation_id", op.OperationID),
		zap.String("required_scope", op.RequiredScope),
		zap.String("path", r.URL.Path),
		zap.String("reason", reason),
		zap.Int("status", status),
	}
	if principal != nil {
		fields = append(fields,
			zap.String("subject", principal.Subject),
			zap.String("tenant_id", principal.TenantID),
			zap.Strings("roles", principal.Roles),
			zap.Strings("permissions", principal.Permissions))
	}
	logger.Warn("contract deny", fields...)
}

func emitContractDenyAudit(auditors []ContractDenyAuditor, r *http.Request, op *Operation, principal *Principal, status int) {
	for _, a := range auditors {
		if a != nil {
			a(r, op, principal, status)
		}
	}
}

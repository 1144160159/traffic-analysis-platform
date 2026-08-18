// OIDC 角色映射契约解释(ADR-3):contracts/authz/oidc-role-map.v1.json
// 是 Keycloak OIDC 角色 -> 平台内部角色的唯一权威,运行时按此解释,
// 消除 auth-service 与共享中间件的双份硬编码漂移。
package authz

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

//go:embed policy/oidc-role-map.v1.json
var embeddedRoleMapJSON []byte

// OIDCRoleMap 契约的运行时表示。
type OIDCRoleMap struct {
	SchemaVersion int                      `json:"schema_version"`
	RolePrefix    string                   `json:"role_prefix"`
	DefaultRole   string                   `json:"default_role"`
	Mapping       map[string]OIDCRoleEntry `json:"mapping"`
}

// OIDCRoleEntry 单条映射。
type OIDCRoleEntry struct {
	InternalRole string `json:"internal_role"`
}

var (
	roleMapOnce    sync.Once
	defaultRoleMap *OIDCRoleMap
	roleMapErr     error
)

// DefaultRoleMap 进程内单例角色映射契约(go:embed,与 contracts/authz 副本同源)。
func DefaultRoleMap() (*OIDCRoleMap, error) {
	roleMapOnce.Do(func() {
		defaultRoleMap, roleMapErr = ParseOIDCRoleMap(embeddedRoleMapJSON)
	})
	return defaultRoleMap, roleMapErr
}

// ParseOIDCRoleMap 解析角色映射契约。
func ParseOIDCRoleMap(raw []byte) (*OIDCRoleMap, error) {
	var m OIDCRoleMap
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("oidc role map decode: %w", err)
	}
	if m.RolePrefix == "" || m.DefaultRole == "" || len(m.Mapping) == 0 {
		return nil, fmt.Errorf("oidc role map incomplete: prefix=%q default=%q entries=%d",
			m.RolePrefix, m.DefaultRole, len(m.Mapping))
	}
	return &m, nil
}

// MapRoles 按契约把 OIDC 角色映射为内部角色:
// 仅接受契约 role_prefix 前缀的领域角色(大小写不敏感,与历史行为一致);
// 输出去重且字典序稳定(确定性,防角色集合顺序漂移);无命中回落 default_role。
func (m *OIDCRoleMap) MapRoles(oidcRoles []string) []string {
	internal := make([]string, 0, len(oidcRoles))
	seen := map[string]struct{}{}
	for _, r := range oidcRoles {
		lower := strings.ToLower(r)
		if !strings.HasPrefix(lower, m.RolePrefix) {
			continue
		}
		entry, ok := m.Mapping[lower]
		if !ok || entry.InternalRole == "" {
			continue
		}
		if _, dup := seen[entry.InternalRole]; dup {
			continue
		}
		seen[entry.InternalRole] = struct{}{}
		internal = append(internal, entry.InternalRole)
	}
	if len(internal) == 0 {
		return []string{m.DefaultRole}
	}
	sort.Strings(internal)
	return internal
}

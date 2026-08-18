package authz

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestEmbeddedOIDCRoleMapParity 内嵌角色映射副本必须与 contracts/authz 权威副本一致。
func TestEmbeddedOIDCRoleMapParity(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	contractPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..", "contracts", "authz", "oidc-role-map.v1.json")
	contractRaw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	if sum(contractRaw) != sum(embeddedRoleMapJSON) {
		t.Fatal("embedded oidc role map differs from contracts/authz copy; re-sync policy/oidc-role-map.v1.json")
	}
}

// TestOIDCRoleMapContractSemantics 锁定契约语义:前缀过滤、未知后缀跳过、
// 空命中回落 viewer —— auth-service 与共享中间件共用同一解释器,防漂移。
func TestOIDCRoleMapContractSemantics(t *testing.T) {
	m, err := DefaultRoleMap()
	if err != nil {
		t.Fatal(err)
	}
	if m.RolePrefix != "traffic-" || m.DefaultRole != "viewer" {
		t.Fatalf("prefix=%q default=%q", m.RolePrefix, m.DefaultRole)
	}
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"traffic-admin", "offline_access", "uma_authorization"}, []string{"admin"}},
		{[]string{"offline_access"}, []string{"viewer"}},
		{[]string{"traffic-analyst", "traffic-operator"}, []string{"analyst", "operator"}},
		{[]string{"traffic-unknown-role"}, []string{"viewer"}},
		{[]string{"admin"}, []string{"viewer"}},                                          // 无前缀的裸角色名不映射
		{[]string{"TRAFFIC-OPERATOR", "traffic-viewer"}, []string{"operator", "viewer"}}, // 大小写不敏感+字典序
	}
	for _, c := range cases {
		got := MapOIDCRoles(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("MapOIDCRoles(%v)=%v want %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("MapOIDCRoles(%v)=%v want %v", c.in, got, c.want)
			}
		}
	}
	// 契约条目与内部角色枚举一致(五个内部角色,含 super_admin)
	internal := map[string]bool{}
	for _, e := range m.Mapping {
		internal[e.InternalRole] = true
	}
	for _, r := range []string{"admin", "super_admin", "analyst", "viewer", "operator"} {
		if !internal[r] {
			t.Fatalf("contract missing internal role %s", r)
		}
	}
}

package authz

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestPolicyEmbeddedMatchesSource 校验内嵌契约副本与 contracts/authz 真源
// 逐字节一致,并校验 policySourceSHA256 常量未过期,防止双副本漂移。
// 任一契约变更必须同步两份副本并更新常量,否则本测试失败。
func TestPolicyEmbeddedMatchesSource(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	// package dir = go/control-plane/internal/common/authz -> 5 层上溯到仓库根
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..")
	sourcePath := filepath.Join(root, "contracts", "authz", "m10-minimal-role-policy.v1.json")

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source contract %s: %v", sourcePath, err)
	}

	sourceSum := sha256.Sum256(source)
	if got := hex.EncodeToString(sourceSum[:]); got != policySourceSHA256 {
		t.Fatalf("policySourceSHA256 常量过期: 真源 %s 实际哈希 %s,常量 %s", sourcePath, got, policySourceSHA256)
	}

	embeddedSum := sha256.Sum256(embeddedPolicyJSON)
	if got := hex.EncodeToString(embeddedSum[:]); got != policySourceSHA256 {
		t.Fatalf("内嵌副本与真源哈希不一致: embedded=%s source=%s", got, policySourceSHA256)
	}

	if string(embeddedPolicyJSON) != string(source) {
		t.Fatal("内嵌副本与真源逐字节不一致(双副本漂移)")
	}
}

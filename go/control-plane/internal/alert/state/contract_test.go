package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestValidTransitionsMatchContract 校验 Go 状态机与单一真源
// contracts/alert/alert-status-transitions.v1.json 完全一致(双向)。
// 契约变更必须同步 state_machine.go 与 alertStatus.ts,否则本测试失败。
func TestValidTransitionsMatchContract(t *testing.T) {
	var doc struct {
		SchemaVersion int `json:"schema_version"`
		Statuses      []struct {
			Status      string   `json:"status"`
			Transitions []string `json:"transitions"`
		} `json:"statuses"`
		Aliases map[string][]string `json:"aliases"`
	}

	raw, err := os.ReadFile(contractPath(t))
	if err != nil {
		t.Fatalf("read alert status contract: %v", err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode alert status contract: %v", err)
	}

	// 1) 状态与转移(Go -> 契约)
	if len(doc.Statuses) != len(ValidTransitions) {
		t.Fatalf("状态数不一致: contract=%d go=%d", len(doc.Statuses), len(ValidTransitions))
	}
	for _, s := range doc.Statuses {
		want := ValidTransitions[AlertStatus(s.Status)]
		if want == nil {
			t.Fatalf("契约状态 %q 在 Go ValidTransitions 中缺失", s.Status)
		}
		if len(want) != len(s.Transitions) {
			t.Fatalf("状态 %q 转移数不一致: contract=%v go=%v", s.Status, s.Transitions, want)
		}
		gotSet := make(map[AlertStatus]struct{}, len(want))
		for _, w := range want {
			gotSet[w] = struct{}{}
		}
		for _, tr := range s.Transitions {
			if _, ok := gotSet[AlertStatus(tr)]; !ok {
				t.Fatalf("契约转移 %q->%q 在 Go 中缺失", s.Status, tr)
			}
		}
	}

	// 2) 别名(双向):契约按 canonical 分组,Go 为扁平映射,比较总数与逐项映射。
	totalAliases := 0
	for _, list := range doc.Aliases {
		totalAliases += len(list)
	}
	if totalAliases != len(statusAliases) {
		t.Fatalf("别名总数不一致: contract=%d go=%d", totalAliases, len(statusAliases))
	}
	for canonical, aliasList := range doc.Aliases {
		expected := AlertStatus(canonical)
		for _, a := range aliasList {
			got, ok := statusAliases[a]
			if !ok {
				t.Fatalf("契约别名 %q 在 Go statusAliases 中缺失", a)
			}
			if got != expected {
				t.Fatalf("别名 %q 映射不一致: contract=%s go=%s", a, expected, got)
			}
		}
	}
	for a := range statusAliases {
		found := false
		for _, list := range doc.Aliases {
			for _, item := range list {
				if item == a {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("Go statusAliases 中的别名 %q 不在契约中", a)
		}
	}
}

func contractPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	// package dir = go/control-plane/internal/alert/state -> 5 层上溯到仓库根
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..")
	return filepath.Join(root, "contracts", "alert", "alert-status-transitions.v1.json")
}

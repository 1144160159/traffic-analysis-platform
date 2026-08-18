package entityresolution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type contractDocument struct {
	RuleVersion     string `json:"rule_version"`
	IdentifierRules []struct {
		RuleID        string `json:"rule_id"`
		ConfidencePPM int    `json:"confidence_ppm"`
		MaxLinkAgeMS  int64  `json:"max_link_age_ms"`
	} `json:"identifier_rules"`
}

func TestImplementationConstantsMatchFrozenContract(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", "..", ".."))
	payload, err := os.ReadFile(filepath.Join(root, "contracts", "entity", "entity-resolution.v1.json"))
	if err != nil {
		t.Fatalf("read entity resolution contract: %v", err)
	}
	var contract contractDocument
	if err := json.Unmarshal(payload, &contract); err != nil {
		t.Fatalf("decode entity resolution contract: %v", err)
	}
	if contract.RuleVersion != RuleVersion {
		t.Fatalf("rule version = %q, implementation = %q", contract.RuleVersion, RuleVersion)
	}
	want := map[string]struct {
		confidence int
		maxAge     int64
	}{
		ruleAssetExact: {AssetExactConfidencePPM, 0},
		ruleUserExact:  {UserExactConfidencePPM, 0},
		ruleProbeExact: {ProbeExactConfidencePPM, 0},
		ruleMACAsset:   {MACAssetConfidencePPM, MACMaximumLinkAgeMS},
		ruleIPAsset:    {IPAssetConfidencePPM, IPMaximumLinkAgeMS},
		ruleCommunity:  {CommunityConfidencePPM, 0},
	}
	if len(contract.IdentifierRules) != len(want) {
		t.Fatalf("contract rules = %d, want %d", len(contract.IdentifierRules), len(want))
	}
	for _, rule := range contract.IdentifierRules {
		expected, found := want[rule.RuleID]
		if !found {
			t.Fatalf("unexpected contract rule %q", rule.RuleID)
		}
		if rule.ConfidencePPM != expected.confidence || rule.MaxLinkAgeMS != expected.maxAge {
			t.Fatalf("rule %s drifted: contract confidence=%d max_age=%d implementation confidence=%d max_age=%d",
				rule.RuleID, rule.ConfidencePPM, rule.MaxLinkAgeMS,
				expected.confidence, expected.maxAge)
		}
	}
}

package model

import "testing"

func TestRuleTypeContractAcceptsEveryFlinkWireType(t *testing.T) {
	types := []string{
		"threshold", "blacklist", "port_scan", "brute_force", "data_exfil",
		"dga", "tunnel", "c2", "anomaly", "protocol_anomaly",
		"tls_fingerprint", "custom", "signature", "correlation", "ml",
	}
	for _, ruleType := range types {
		if !IsValidRuleType(ruleType) {
			t.Fatalf("cross-language rule type %q is not accepted", ruleType)
		}
	}
	if IsValidRuleType("unknown") {
		t.Fatal("unknown rule type must fail closed")
	}
}

package service

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func TestNormalizeDiscoveryMode(t *testing.T) {
	cases := map[string]string{
		"SNMP":      config.DiscoveryModeSNMP,
		"lldp":      config.DiscoveryModeLLDP,
		"snmp-lldp": config.DiscoveryModeSNMPLLDP,
		"snmp+lldp": config.DiscoveryModeSNMPLLDP,
	}
	for input, want := range cases {
		if got := normalizeDiscoveryMode(input); got != want {
			t.Fatalf("normalizeDiscoveryMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRegisterDiscoveryCredentialRejectsPlainMissingSecretRef(t *testing.T) {
	svc := New(nil, nil, zap.NewNop())
	_, err := svc.RegisterDiscoveryCredential(context.Background(), &config.DiscoveryCredential{
		TenantID: "default",
		Name:     "core-switches",
		Protocol: "snmp_lldp",
	})
	if err == nil {
		t.Fatal("expected missing secret_ref to fail")
	}
	if !strings.Contains(err.Error(), "secret_ref") {
		t.Fatalf("expected secret_ref error, got %v", err)
	}
}

func TestRegisterDiscoveryCredentialRejectsInvalidProtocol(t *testing.T) {
	svc := New(nil, nil, zap.NewNop())
	_, err := svc.RegisterDiscoveryCredential(context.Background(), &config.DiscoveryCredential{
		TenantID:  "default",
		Name:      "core-switches",
		Protocol:  "telnet",
		SecretRef: "k8s://traffic-analysis/traffic-credentials#ASSET_DISCOVERY_SNMP",
	})
	if err == nil {
		t.Fatal("expected invalid protocol to fail")
	}
	if !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("expected protocol error, got %v", err)
	}
}

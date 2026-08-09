package main

import (
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
)

func TestRepairRequiresExplicitV2Target(t *testing.T) {
	if err := validateModeConfiguration("plan", config.OpenSearchConfig{}); err != nil {
		t.Fatalf("plan must remain available for read-only legacy discovery: %v", err)
	}
	if err := validateModeConfiguration("repair", config.OpenSearchConfig{}); err == nil {
		t.Fatal("repair unexpectedly accepted a legacy target")
	}
	if err := validateModeConfiguration("repair", config.OpenSearchConfig{V2Enabled: true}); err != nil {
		t.Fatalf("approved V2 repair target was rejected: %v", err)
	}
}

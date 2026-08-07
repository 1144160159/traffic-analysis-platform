package main

import (
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
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

func TestRepairRequiresExactObservedTargetBinding(t *testing.T) {
	metadata := persistence.OpenSearchProjectionMetadata{
		ClusterUUID: "cluster-a", ReadTarget: "alerts-v2-read", WriteAlias: "alerts-v2-write",
		WriteIndices: []persistence.OpenSearchProjectionWriteIndex{{Index: "alerts-v2-000001", IsWriteIndex: true}},
	}
	if err := validateRepairTargetBinding("plan", metadata, "", "", "", ""); err != nil {
		t.Fatalf("read-only plan unexpectedly required repair identity: %v", err)
	}
	if err := validateRepairTargetBinding("repair", metadata, "cluster-a", "alerts-v2-read", "alerts-v2-write", "alerts-v2-000001"); err != nil {
		t.Fatalf("exact observed repair identity was rejected: %v", err)
	}
	tests := map[string]struct {
		cluster, read, alias, index string
		metadata                    persistence.OpenSearchProjectionMetadata
	}{
		"missing binding": {metadata: metadata},
		"cluster drift":   {cluster: "cluster-b", read: "alerts-v2-read", alias: "alerts-v2-write", index: "alerts-v2-000001", metadata: metadata},
		"read wildcard":   {cluster: "cluster-a", read: "alerts-*", alias: "alerts-v2-write", index: "alerts-v2-000001", metadata: metadata},
		"index drift":     {cluster: "cluster-a", read: "alerts-v2-read", alias: "alerts-v2-write", index: "alerts-v2-000002", metadata: metadata},
		"alias ambiguity": {
			cluster: "cluster-a", read: "alerts-v2-read", alias: "alerts-v2-write", index: "alerts-v2-000001",
			metadata: persistence.OpenSearchProjectionMetadata{
				ClusterUUID: "cluster-a", ReadTarget: "alerts-v2-read", WriteAlias: "alerts-v2-write",
				WriteIndices: []persistence.OpenSearchProjectionWriteIndex{
					{Index: "alerts-v2-000001", IsWriteIndex: true}, {Index: "alerts-v2-000002", IsWriteIndex: true},
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateRepairTargetBinding("repair", test.metadata, test.cluster, test.read, test.alias, test.index); err == nil {
				t.Fatal("unsafe repair target binding was accepted")
			}
		})
	}
}

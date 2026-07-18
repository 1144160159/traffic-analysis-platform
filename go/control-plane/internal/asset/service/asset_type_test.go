package service

import "testing"

func TestIsAssetType(t *testing.T) {
	for _, assetType := range []string{"endpoint", "server", "network-device", "business-system", "unknown"} {
		if !IsAssetType(assetType) {
			t.Fatalf("expected %q to be a supported asset type", assetType)
		}
	}
	for _, assetType := range []string{"", "device", "open-services", "SERVER"} {
		if IsAssetType(assetType) {
			t.Fatalf("expected %q to be rejected", assetType)
		}
	}
}

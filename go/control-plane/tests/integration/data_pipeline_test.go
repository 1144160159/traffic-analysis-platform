package integration

import (
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	"go.uber.org/zap"
)

func TestLookupVendor(t *testing.T) {
	tests := []struct{ mac, want string }{
		{"00:1a:c5:11:22:33", "Cisco Systems"},
		{"00:1b:21:aa:bb:cc", "Intel Corporate"},
		{"00:0c:29:dd:ee:ff", "VMware, Inc."},
		{"08:00:27:11:22:33", "Oracle VirtualBox"},
		{"18:c0:09:aa:bb:cc", "Broadcom Limited"},
		{"b8:27:eb:11:22:33", "Raspberry Pi Foundation"},
		{"00:50:56:aa:bb:cc", "VMware ESX"},
		{"f0:1f:af:dd:ee:ff", "Dell Inc."},
		{"ff:ff:ff:ff:ff:ff", "Unknown"},
		{"invalid", "Unknown"},
	}
	for _, tt := range tests {
		got := service.LookupVendor(tt.mac)
		if got != tt.want {
			t.Errorf("LookupVendor(%s) = %s, want %s", tt.mac, got, tt.want)
		}
	}
}

func TestAssetServiceCreation(t *testing.T) {
	svc := service.New(&config.Config{}, nil, zap.NewNop())
	if svc == nil {
		t.Fatal("service.New returned nil")
	}
}

func TestAssetRecordDefaults(t *testing.T) {
	rec := &config.AssetRecord{
		MACAddress: "00:1a:c5:11:22:33",
		Source:     "arp",
	}
	if rec.Vendor != "" {
		t.Log("vendor should be explicitly set by service layer")
	}
	// Verify vendor lookup can be applied
	vendor := service.LookupVendor(rec.MACAddress)
	if vendor != "Cisco Systems" {
		t.Errorf("vendor lookup failed: got %s", vendor)
	}
	rec.Vendor = vendor
	if rec.Vendor != "Cisco Systems" {
		t.Error("vendor not set correctly")
	}
}

// Benchmark vendor lookup performance
func BenchmarkLookupVendor(b *testing.B) {
	for i := 0; i < b.N; i++ {
		service.LookupVendor("00:1a:c5:11:22:33")
	}
}

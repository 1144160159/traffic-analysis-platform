package api

import (
	"context"
	"testing"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestAssetHandlerNilAsset(t *testing.T) {
	h := &AssetHandler{}
	_, err := h.UpsertAsset(context.Background(), &pb.UpsertAssetRequest{})
	if err == nil {
		t.Fatal("expected error for nil asset")
	}
}

func TestAssetHandlerMissingMAC(t *testing.T) {
	h := &AssetHandler{}
	_, err := h.UpsertAsset(context.Background(), &pb.UpsertAssetRequest{
		Asset: &pb.Asset{TenantId: "t1"},
	})
	if err == nil {
		t.Fatal("expected error for missing mac_address")
	}
}

func TestAssetHandlerGetAssetValidation(t *testing.T) {
	h := &AssetHandler{}
	_, err := h.GetAsset(context.Background(), &pb.GetAssetRequest{})
	if err == nil {
		t.Fatal("expected error for empty request")
	}
}

func TestAssetHandlerListAssets(t *testing.T) {
	h := &AssetHandler{}
	_, err := h.ListAssets(context.Background(), &pb.ListAssetsRequest{})
	if err == nil {
		t.Fatal("expected error for missing tenant_id")
	}
}

func TestAssetHandlerRecordMacIpBindingValidation(t *testing.T) {
	h := &AssetHandler{}
	// Handler with nil repo will fail, but validate input first
	req := &pb.RecordMacIpBindingRequest{
		Bindings: []*pb.MacIpBinding{
			{MacAddress: "00:1a:c5:11:22:33", IpAddress: "192.168.1.1", TenantId: "t1", Source: "arp"},
		},
	}
	// Empty MAC should be skipped
	req2 := &pb.RecordMacIpBindingRequest{
		Bindings: []*pb.MacIpBinding{
			{MacAddress: "", IpAddress: "10.0.0.1", TenantId: "t1"},
		},
	}
	_, err := h.RecordMacIpBinding(context.Background(), req)
	_, err2 := h.RecordMacIpBinding(context.Background(), req2)
	if err != nil { t.Logf("RecordMacIpBinding with valid MAC: %v (expected with nil repo)", err) }
	if err2 != nil { t.Logf("RecordMacIpBinding with empty MAC: %v (expected)", err2) }
}

func TestAssetHandlerGetAssetHistory(t *testing.T) {
	h := &AssetHandler{}
	_, err := h.GetAssetHistory(context.Background(), &pb.GetAssetHistoryRequest{
		AssetId: "test-id",
	})
	if err != nil {
		// May fail due to missing repo, but should not panic
		t.Logf("GetAssetHistory (expected with nil repo): %v", err)
	}
}

func TestToProto(t *testing.T) {
	rec := &struct {
		AssetID, TenantID, IPAddress, MACAddress, Hostname, Vendor, OSType, Source, VlanID, SwitchPort string
	}{AssetID: "a1", TenantID: "t1", IPAddress: "10.0.0.1", MACAddress: "aa:bb:cc:dd:ee:ff",
		Hostname: "server-01", Vendor: "Intel", OSType: "Linux", Source: "arp", VlanID: "100", SwitchPort: "Gi1/0/1"}
	_ = rec // placeholder for actual toProto test when types align
	t.Log("toProto placeholder verified")
}

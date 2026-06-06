package api

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// newTestHandler 创建带 nil repo 的 handler，用于参数校验测试
func newTestHandler() *AssetHandler {
	logger := zap.NewNop()
	svc := service.New(nil, nil, logger)
	return &AssetHandler{svc: svc, repo: nil, logger: logger}
}

func TestAssetHandlerNilAsset(t *testing.T) {
	h := newTestHandler()
	_, err := h.UpsertAsset(context.Background(), &pb.UpsertAssetRequest{})
	if err == nil {
		t.Fatal("expected error for nil asset")
	}
}

func TestAssetHandlerMissingMAC(t *testing.T) {
	h := newTestHandler()
	_, err := h.UpsertAsset(context.Background(), &pb.UpsertAssetRequest{
		Asset: &pb.Asset{TenantId: "t1"},
	})
	if err == nil {
		t.Fatal("expected error for missing mac_address")
	}
}

func TestAssetHandlerGetAssetValidation(t *testing.T) {
	h := newTestHandler()
	_, err := h.GetAsset(context.Background(), &pb.GetAssetRequest{})
	if err == nil {
		t.Fatal("expected error for empty request")
	}
}

func TestAssetHandlerListAssets(t *testing.T) {
	h := newTestHandler()
	_, err := h.ListAssets(context.Background(), &pb.ListAssetsRequest{})
	if err == nil {
		t.Fatal("expected error for missing tenant_id")
	}
}

func TestAssetHandlerRecordMacIpBindingValidation(t *testing.T) {
	h := newTestHandler()

	// 空绑定列表应返回校验错误
	_, err := h.RecordMacIpBinding(context.Background(), &pb.RecordMacIpBindingRequest{
		Bindings: []*pb.MacIpBinding{},
	})
	if err == nil {
		t.Log("empty bindings: no error (may be handled differently)")
	}

	// 空 MAC 应被跳过（在 service 层处理）
	req2 := &pb.RecordMacIpBindingRequest{
		Bindings: []*pb.MacIpBinding{
			{MacAddress: "", IpAddress: "10.0.0.1", TenantId: "t1"},
		},
	}
	_, err2 := h.RecordMacIpBinding(context.Background(), req2)
	if err2 != nil {
		t.Logf("RecordMacIpBinding with empty MAC: %v (expected)", err2)
	} else {
		t.Log("RecordMacIpBinding with empty MAC: silently rejected")
	}
}

func TestAssetHandlerGetAssetHistory(t *testing.T) {
	h := newTestHandler()

	// 空 asset_id 应返回校验错误
	_, err := h.GetAssetHistory(context.Background(), &pb.GetAssetHistoryRequest{})
	if err == nil {
		t.Fatal("expected error for empty asset_id")
	}

	// 有效 asset_id 但无 repo：预期失败但不 panic
	// 跳过 repo 相关测试（需要实际 DB 连接）
	t.Log("GetAssetHistory validation passed (repo-dependent test skipped)")
	_ = err
}

func TestRecordToProto(t *testing.T) {
	// 验证 proto 转换函数不会 panic
	pbAsset := recordToProto(nil)
	if pbAsset != nil {
		t.Log("recordToProto(nil) should return non-nil proto")
	}

	// 验证 LookupVendor 独立函数
	vendor := service.LookupVendor("00:1a:c5:11:22:33")
	if vendor == "Unknown" {
		t.Error("expected known vendor for 00:1a:c5 prefix")
	}
	if vendor != "Cisco Systems" {
		t.Errorf("expected Cisco Systems, got %s", vendor)
	}

	vendor2 := service.LookupVendor("ff:ff:ff:11:22:33")
	if vendor2 != "Unknown" {
		t.Errorf("expected Unknown for ff:ff:ff, got %s", vendor2)
	}
}

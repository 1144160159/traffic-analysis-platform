package api

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// AssetHandler gRPC AssetService 处理器。
// 职责：参数校验、序列化/反序列化，委托业务逻辑给 service 层。
type AssetHandler struct {
	pb.UnimplementedAssetServiceServer
	svc    *service.AssetService
	repo   *repository.AssetRepository
	logger *zap.Logger
}

func NewAssetHandler(svc *service.AssetService, repo *repository.AssetRepository, logger *zap.Logger) *AssetHandler {
	return &AssetHandler{svc: svc, repo: repo, logger: logger}
}

// logError 条件日志（nil-safe）
func (h *AssetHandler) logError(msg string, fields ...zap.Field) {
	if h.logger != nil {
		h.logger.Error(msg, fields...)
	}
}

// =============================================================================
// UpsertAsset
// =============================================================================

func (h *AssetHandler) UpsertAsset(ctx context.Context, req *pb.UpsertAssetRequest) (*pb.UpsertAssetResponse, error) {
	a := req.GetAsset()
	if a == nil || a.MacAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "asset with mac_address is required")
	}

	rec := protoToRecord(a)
	if rec.AssetID == "" {
		rec.AssetID = uuid.New().String()
	}
	if rec.Source == "" {
		rec.Source = "manual"
	}

	id, created, err := h.svc.UpsertAsset(ctx, rec)
	if err != nil {
		h.logError("UpsertAsset failed", zap.String("mac", rec.MACAddress), zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.UpsertAssetResponse{AssetId: id, Created: created}, nil
}

// =============================================================================
// GetAsset
// =============================================================================

func (h *AssetHandler) GetAsset(ctx context.Context, req *pb.GetAssetRequest) (*pb.GetAssetResponse, error) {
	if req.AssetId == "" && req.MacAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "asset_id or mac_address required")
	}

	rec, err := h.svc.GetAsset(ctx, req.TenantId, req.AssetId, req.MacAddress)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeTenantNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		h.logError("GetAsset failed", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.GetAssetResponse{Asset: recordToProto(rec)}, nil
}

// =============================================================================
// ListAssets
// =============================================================================

func (h *AssetHandler) ListAssets(ctx context.Context, req *pb.ListAssetsRequest) (*pb.ListAssetsResponse, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id required")
	}

	limit := int(req.PageSize)
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	recs, total, err := h.svc.ListAssets(ctx, req.TenantId, limit, 0)
	if err != nil {
		h.logError("ListAssets failed", zap.String("tenant", req.TenantId), zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	assets := make([]*pb.Asset, len(recs))
	for i, r := range recs {
		assets[i] = recordToProto(r)
	}
	return &pb.ListAssetsResponse{Assets: assets, TotalCount: int32(total)}, nil
}

// =============================================================================
// RecordMacIpBinding
// =============================================================================

func (h *AssetHandler) RecordMacIpBinding(ctx context.Context, req *pb.RecordMacIpBindingRequest) (*pb.RecordMacIpBindingResponse, error) {
	bindings := make([]*config.MacIpBinding, 0, len(req.Bindings))
	for _, b := range req.Bindings {
		bindings = append(bindings, &config.MacIpBinding{
			MACAddress: b.MacAddress,
			IPAddress:  b.IpAddress,
			TenantID:   b.TenantId,
			Source:     b.Source,
			ObservedAt: b.ObservedAt,
		})
	}

	accepted, rejected, err := h.svc.RecordMacIpBinding(ctx, bindings)
	if err != nil {
		h.logError("RecordMacIpBinding failed", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.RecordMacIpBindingResponse{Accepted: accepted, Rejected: rejected}, nil
}

// =============================================================================
// GetAssetHistory
// =============================================================================

func (h *AssetHandler) GetAssetHistory(ctx context.Context, req *pb.GetAssetHistoryRequest) (*pb.GetAssetHistoryResponse, error) {
	if req.AssetId == "" {
		return nil, status.Error(codes.InvalidArgument, "asset_id required")
	}

	limit := int(req.PageSize)
	if limit <= 0 {
		limit = 20
	}

	events, err := h.svc.GetAssetHistory(ctx, req.TenantId, req.AssetId, limit)
	if err != nil {
		h.logError("GetAssetHistory failed", zap.String("asset_id", req.AssetId), zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	pbEvents := make([]*pb.AssetEvent, len(events))
	for i, e := range events {
		pbEvents[i] = &pb.AssetEvent{
			EventId:   fmt.Sprintf("%d", e.EventID),
			AssetId:   e.AssetID,
			TenantId:  e.TenantID,
			EventType: e.EventType,
			OldValue:  e.OldValue,
			NewValue:  e.NewValue,
			CreatedAt: e.CreatedAt.UnixMilli(),
		}
	}
	return &pb.GetAssetHistoryResponse{Events: pbEvents}, nil
}

// =============================================================================
// Proto 转换
// =============================================================================

func protoToRecord(a *pb.Asset) *config.AssetRecord {
	return &config.AssetRecord{
		AssetID:    a.AssetId,
		TenantID:   a.TenantId,
		IPAddress:  a.IpAddress,
		MACAddress: a.MacAddress,
		Hostname:   a.Hostname,
		Vendor:     a.Vendor,
		OSType:     a.OsType,
		Source:     a.Source,
		VlanID:     a.VlanId,
		SwitchPort: a.SwitchPort,
	}
}

func recordToProto(a *config.AssetRecord) *pb.Asset {
	if a == nil {
		return nil
	}
	return &pb.Asset{
		AssetId:    a.AssetID,
		TenantId:   a.TenantID,
		IpAddress:  a.IPAddress,
		MacAddress: a.MACAddress,
		Hostname:   a.Hostname,
		Vendor:     a.Vendor,
		OsType:     a.OSType,
		Source:     a.Source,
		VlanId:     a.VlanID,
		SwitchPort: a.SwitchPort,
		FirstSeen:  a.FirstSeen.UnixMilli(),
		LastSeen:   a.LastSeen.UnixMilli(),
	}
}

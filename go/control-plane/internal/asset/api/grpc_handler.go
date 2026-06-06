package api

import (
	"context"
	"fmt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AssetHandler struct {
	pb.UnimplementedAssetServiceServer
	svc    *service.AssetService
	repo   *repository.AssetRepository
	logger *zap.Logger
}

func NewAssetHandler(svc *service.AssetService, repo *repository.AssetRepository, logger *zap.Logger) *AssetHandler {
	return &AssetHandler{svc: svc, repo: repo, logger: logger}
}

func (h *AssetHandler) UpsertAsset(ctx context.Context, req *pb.UpsertAssetRequest) (*pb.UpsertAssetResponse, error) {
	a := req.GetAsset()
	if a == nil || a.MacAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "asset with mac_address is required")
	}
	rec := &config.AssetRecord{
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
	if rec.AssetID == "" {
		rec.AssetID = uuid.New().String()
	}
	if rec.Source == "" {
		rec.Source = "manual"
	}
	rec.Vendor = service.LookupVendor(rec.MACAddress)

	id, created, err := h.repo.Upsert(ctx, rec)
	if err != nil {
		h.logger.Error("UpsertAsset failed", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.UpsertAssetResponse{AssetId: id, Created: created}, nil
}

func (h *AssetHandler) GetAsset(ctx context.Context, req *pb.GetAssetRequest) (*pb.GetAssetResponse, error) {
	if req.AssetId == "" && req.MacAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "asset_id or mac_address required")
	}
	var rec *config.AssetRecord
	var err error
	if req.AssetId != "" {
		rec, err = h.repo.FindByID(ctx, req.AssetId)
	} else {
		rec, err = h.repo.FindByMAC(ctx, req.TenantId, req.MacAddress)
	}
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeTenantNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.GetAssetResponse{Asset: toProto(rec)}, nil
}

func (h *AssetHandler) ListAssets(ctx context.Context, req *pb.ListAssetsRequest) (*pb.ListAssetsResponse, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id required")
	}
	limit := int(req.PageSize)
	if limit <= 0 || limit > 100 { limit = 50 }
	offset := 0
	recs, total, err := h.repo.ListByTenant(ctx, req.TenantId, limit, offset)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	assets := make([]*pb.Asset, len(recs))
	for i, r := range recs {
		assets[i] = toProto(r)
	}
	return &pb.ListAssetsResponse{Assets: assets, TotalCount: int32(total)}, nil
}

func (h *AssetHandler) RecordMacIpBinding(ctx context.Context, req *pb.RecordMacIpBindingRequest) (*pb.RecordMacIpBindingResponse, error) {
	accepted, rejected := int32(0), int32(0)
	for _, b := range req.Bindings {
		rec := &config.AssetRecord{
			AssetID:    uuid.New().String(),
			TenantID:   b.TenantId,
			IPAddress:  b.IpAddress,
			MACAddress: b.MacAddress,
			Source:     b.Source,
			Vendor:     service.LookupVendor(b.MacAddress),
		}
		if rec.Source == "" { rec.Source = "passive" }
		_, _, err := h.repo.Upsert(ctx, rec)
		if err != nil {
			h.logger.Warn("RecordMacIpBinding upsert failed", zap.String("mac", b.MacAddress), zap.Error(err))
			rejected++
		} else {
			accepted++
		}
	}
	return &pb.RecordMacIpBindingResponse{Accepted: accepted, Rejected: rejected}, nil
}

func (h *AssetHandler) GetAssetHistory(ctx context.Context, req *pb.GetAssetHistoryRequest) (*pb.GetAssetHistoryResponse, error) {
	limit := int(req.PageSize)
	if limit <= 0 { limit = 20 }
	events, err := h.repo.GetHistory(ctx, req.AssetId, limit)
	if err != nil {
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

func toProto(a *config.AssetRecord) *pb.Asset {
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


package api

import (
	"context"
	stderrors "errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// AssetHandler gRPC AssetService 处理器。
// 职责：参数校验、序列化/反序列化，委托业务逻辑给 service 层。
type AssetHandler struct {
	pb.UnimplementedAssetServiceServer
	svc           *service.AssetService
	repo          *repository.AssetRepository
	logger        *zap.Logger
	jwtSigningKey string
	cursorEnabled bool
	cursorCodec   *assetCursorCodec
}

func NewAssetHandler(svc *service.AssetService, repo *repository.AssetRepository, logger *zap.Logger) *AssetHandler {
	handler := &AssetHandler{svc: svc, repo: repo, logger: logger}
	if svc != nil {
		handler.jwtSigningKey = svc.JWTSigningKey()
		handler.cursorEnabled = svc.AssetCursorV2Enabled()
		if handler.cursorEnabled {
			codec, err := newAssetCursorCodec(svc.JWTSigningKey())
			if err != nil {
				if logger != nil {
					logger.Error("asset gRPC cursor codec unavailable", zap.Error(err))
				}
			} else {
				handler.cursorCodec = codec
			}
		}
	}
	return handler
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

	command, identity, commandErr := h.assetUpsertCommandFromGRPC(ctx)
	if commandErr != nil {
		return nil, commandErr
	}
	rec := protoToRecord(a)
	if rec.TenantID != "" && rec.TenantID != identity.TenantID {
		return nil, status.Error(codes.PermissionDenied, "asset tenant conflicts with authenticated tenant")
	}
	rec.TenantID = identity.TenantID
	if rec.Source == "" {
		rec.Source = "manual"
	}

	result, err := h.svc.UpsertAssetAtomic(ctx, rec, command)
	if err != nil {
		h.logError("UpsertAsset failed", zap.String("mac", rec.MACAddress), zap.Error(err))
		if stderrors.Is(err, repository.ErrAssetRevisionConflict) {
			return nil, status.Error(codes.Aborted, "asset revision conflict")
		}
		if stderrors.Is(err, repository.ErrAssetIdempotencyConflict) {
			return nil, status.Error(codes.AlreadyExists, "asset idempotency conflict")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.UpsertAssetResponse{AssetId: result.AssetID, Created: result.Created}, nil
}

// =============================================================================
// GetAsset
// =============================================================================

func (h *AssetHandler) GetAsset(ctx context.Context, req *pb.GetAssetRequest) (*pb.GetAssetResponse, error) {
	if req.AssetId == "" && req.MacAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "asset_id or mac_address required")
	}
	tenantID, err := h.grpcReadTenant(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	rec, err := h.svc.GetAsset(ctx, tenantID, req.AssetId, req.MacAddress)
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
	tenantID, err := h.grpcReadTenant(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	limit := int(req.PageSize)
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	if h.cursorEnabled {
		if h.cursorCodec == nil {
			return nil, status.Error(codes.FailedPrecondition, "asset cursor service is unavailable")
		}
		filter := config.AssetListFilter{
			IPPrefix: strings.TrimSpace(req.IpPrefix),
			Vendor:   strings.TrimSpace(req.VendorFilter),
		}
		var position *config.AssetCursorPosition
		if token := strings.TrimSpace(req.PageToken); token != "" {
			var explicitLimit *int
			if req.PageSize > 0 {
				explicitLimit = &limit
			}
			claims, err := h.cursorCodec.decode(token, tenantID, filter, explicitLimit)
			if err != nil {
				return nil, status.Error(codes.InvalidArgument, "page_token is invalid or expired")
			}
			limit = claims.Limit
			position = &config.AssetCursorPosition{
				SnapshotAt:   time.UnixMicro(claims.SnapshotUnixMicro).UTC(),
				SnapshotXIDs: claims.SnapshotXIDs,
				LastSeen:     time.UnixMicro(claims.LastSeenUnixMicro).UTC(),
				LastAssetID:  claims.LastAssetID,
				Total:        claims.Total,
			}
		}
		page, err := h.svc.ListAssetsCursor(ctx, tenantID, filter, limit, position)
		if err != nil {
			h.logError("ListAssets cursor failed", zap.String("tenant", tenantID), zap.Error(err))
			return nil, status.Error(codes.Internal, err.Error())
		}
		nextPageToken := ""
		if page.HasMore {
			nextPageToken, err = h.cursorCodec.encode(tenantID, filter, limit, page)
			if err != nil {
				h.logError("ListAssets next page token failed", zap.String("tenant", tenantID), zap.Error(err))
				return nil, status.Error(codes.Internal, "next page token could not be created")
			}
		}
		assets := make([]*pb.Asset, len(page.Assets))
		for index, record := range page.Assets {
			assets[index] = recordToProto(record)
		}
		total := page.Total
		const maxInt32 = int(^uint32(0) >> 1)
		if total > maxInt32 {
			total = maxInt32
		}
		return &pb.ListAssetsResponse{
			Assets:        assets,
			NextPageToken: nextPageToken,
			TotalCount:    int32(total),
		}, nil
	}
	if strings.TrimSpace(req.PageToken) != "" {
		return nil, status.Error(codes.FailedPrecondition, "asset cursor rollout is not enabled")
	}
	recs, total, err := h.svc.ListAssets(ctx, tenantID, limit, 0)
	if err != nil {
		h.logError("ListAssets failed", zap.String("tenant", tenantID), zap.Error(err))
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

	identity, traceID, requestID, identityErr := h.grpcMutationIdentity(ctx)
	if identityErr != nil {
		return nil, identityErr
	}
	for _, binding := range bindings {
		if binding.TenantID != "" && binding.TenantID != identity.TenantID {
			return nil, status.Error(codes.PermissionDenied, "binding tenant conflicts with authenticated tenant")
		}
		binding.TenantID = identity.TenantID
	}
	accepted, rejected, err := h.svc.RecordMacIpBinding(ctx, bindings, service.BindingProvenance{
		Channel:   service.BindingChannelGRPC,
		Actor:     auditActor(identity),
		TraceID:   traceID,
		RequestID: requestID,
	})
	if err != nil {
		h.logError("RecordMacIpBinding failed", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.RecordMacIpBindingResponse{Accepted: accepted, Rejected: rejected}, nil
}

func (h *AssetHandler) assetUpsertCommandFromGRPC(ctx context.Context) (config.AssetUpsertCommand, requestIdentity, error) {
	identity, traceID, requestID, err := h.grpcMutationIdentity(ctx)
	if err != nil {
		return config.AssetUpsertCommand{}, requestIdentity{}, err
	}
	md, _ := metadata.FromIncomingContext(ctx)
	idempotencyKey := firstGRPCMetadata(md, "idempotency-key")
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		return config.AssetUpsertCommand{}, requestIdentity{}, status.Error(codes.InvalidArgument, "idempotency-key metadata must be 16-200 characters")
	}
	expectedText := firstGRPCMetadata(md, "x-expected-revision")
	if expectedText == "" {
		return config.AssetUpsertCommand{}, requestIdentity{}, status.Error(codes.InvalidArgument, "x-expected-revision metadata is required")
	}
	expectedRevision, parseErr := strconv.ParseInt(expectedText, 10, 64)
	if parseErr != nil || expectedRevision < 0 {
		return config.AssetUpsertCommand{}, requestIdentity{}, status.Error(codes.InvalidArgument, "x-expected-revision must be a non-negative integer")
	}
	reason := firstGRPCMetadata(md, "x-reason")
	if reason == "" {
		return config.AssetUpsertCommand{}, requestIdentity{}, status.Error(codes.InvalidArgument, "x-reason metadata is required")
	}
	return config.AssetUpsertCommand{
		ActionID:         config.AssetUpsertAction,
		ExpectedRevision: expectedRevision,
		IdempotencyKey:   idempotencyKey,
		Actor:            auditActor(identity),
		Reason:           reason,
		TraceID:          traceID,
		RequestID:        requestID,
	}, identity, nil
}

func (h *AssetHandler) grpcMutationIdentity(ctx context.Context) (identity requestIdentity, traceID, requestID string, err error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return requestIdentity{}, "", "", status.Error(codes.Unauthenticated, "authorization metadata is required")
	}
	tokenString := bearerTokenValue(firstGRPCMetadata(md, "authorization"))
	identity, identityStatus, identityMessage := accessTokenIdentity(tokenString, h.jwtSigningKey)
	if identityStatus != 0 {
		return requestIdentity{}, "", "", status.Error(codes.Unauthenticated, identityMessage)
	}
	if !hasDiscoveryWriteScope(identity.Scopes) {
		return requestIdentity{}, "", "", status.Error(codes.PermissionDenied, requiredAssetDiscoverScope+" scope required")
	}
	if identity.TenantID == "" {
		return requestIdentity{}, "", "", status.Error(codes.PermissionDenied, "verified tenant identity required")
	}
	traceID = firstGRPCMetadata(md, "x-trace-id")
	requestID = firstGRPCMetadata(md, "x-request-id")
	if traceID == "" {
		return requestIdentity{}, "", "", status.Error(codes.InvalidArgument, "x-trace-id metadata is required")
	}
	if requestID == "" {
		requestID = traceID
	}
	return identity, traceID, requestID, nil
}

func firstGRPCMetadata(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

// grpcReadTenant 校验 gRPC 读接口身份并返回认证租户。
// 读接口与写接口一样要求 Bearer 令牌与已验证的租户身份；请求体携带的
// tenant_id 只允许与认证租户一致，不允许作为身份来源。
func (h *AssetHandler) grpcReadTenant(ctx context.Context, requestedTenant string) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "authorization metadata is required")
	}
	tokenString := bearerTokenValue(firstGRPCMetadata(md, "authorization"))
	identity, identityStatus, identityMessage := accessTokenIdentity(tokenString, h.jwtSigningKey)
	if identityStatus != 0 {
		return "", status.Error(codes.Unauthenticated, identityMessage)
	}
	if !hasAssetReadScope(identity.Scopes) {
		return "", status.Error(codes.PermissionDenied, authmodel.ScopeAssetRead+" scope required")
	}
	if identity.TenantID == "" {
		return "", status.Error(codes.PermissionDenied, "verified tenant identity required")
	}
	if requestedTenant != "" && requestedTenant != identity.TenantID {
		return "", status.Error(codes.PermissionDenied, "request tenant conflicts with authenticated tenant")
	}
	return identity.TenantID, nil
}

// =============================================================================
// GetAssetHistory
// =============================================================================

func (h *AssetHandler) GetAssetHistory(ctx context.Context, req *pb.GetAssetHistoryRequest) (*pb.GetAssetHistoryResponse, error) {
	if req.AssetId == "" {
		return nil, status.Error(codes.InvalidArgument, "asset_id required")
	}
	tenantID, err := h.grpcReadTenant(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	limit := int(req.PageSize)
	if limit <= 0 {
		limit = 20
	}

	events, err := h.svc.GetAssetHistory(ctx, tenantID, req.AssetId, limit)
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

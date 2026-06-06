package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// AssetService 资产服务：MAC→IP 映射、设备发现、资产清单管理。
// 数据来源：被动（ARP/DHCP 探针上报）+ 主动（SNMP/LLDP 可选）+ 人工录入。
type AssetService struct {
	cfg    *config.Config
	repo   *repository.AssetRepository
	logger *zap.Logger
	// ouiCache 可选的 OUI 缓存（Redis），nil 时使用本地内置表
	ouiCache OUILookup
}

// OUILookup OUI 厂商查询接口
type OUILookup interface {
	LookupVendor(mac string) string
}

// localOUICache 本地 OUI 表（开发/单机环境）
type localOUICache struct{}

func (l *localOUICache) LookupVendor(mac string) string {
	return LookupVendor(mac)
}

// New 创建 AssetService
func New(cfg *config.Config, repo *repository.AssetRepository, logger *zap.Logger) *AssetService {
	return &AssetService{
		cfg:      cfg,
		repo:     repo,
		logger:   logger,
		ouiCache: &localOUICache{},
	}
}

// NewWithOUICache 创建带 Redis OUI 缓存的 AssetService
func NewWithOUICache(cfg *config.Config, repo *repository.AssetRepository, logger *zap.Logger, ouiCache OUILookup) *AssetService {
	return &AssetService{
		cfg:      cfg,
		repo:     repo,
		logger:   logger,
		ouiCache: ouiCache,
	}
}

// =============================================================================
// 业务方法
// =============================================================================

// UpsertAsset 创建或更新资产
func (s *AssetService) UpsertAsset(ctx context.Context, rec *config.AssetRecord) (string, bool, error) {
	if rec == nil || rec.MACAddress == "" {
		return "", false, errors.New(errors.ErrCodeInvalidParameter, "mac_address is required")
	}
	if rec.TenantID == "" {
		return "", false, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}

	// 规范化 MAC 地址
	rec.MACAddress = normalizeMAC(rec.MACAddress)

	// OUI 厂商识别
	if rec.Vendor == "" || rec.Vendor == "Unknown" {
		rec.Vendor = s.ouiCache.LookupVendor(rec.MACAddress)
	}

	// 默认来源
	if rec.Source == "" {
		rec.Source = "manual"
	}

	// 生成 AssetID
	if rec.AssetID == "" {
		rec.AssetID = uuid.New().String()
	}

	now := time.Now()
	rec.LastSeen = now
	if rec.FirstSeen.IsZero() {
		rec.FirstSeen = now
	}

	id, created, err := s.repo.Upsert(ctx, rec)
	if err != nil {
		s.logger.Error("UpsertAsset failed",
			zap.String("mac", rec.MACAddress),
			zap.String("tenant", rec.TenantID),
			zap.Error(err))
		return "", false, fmt.Errorf("upsert asset: %w", err)
	}

	if created {
		s.logger.Info("Asset created",
			zap.String("asset_id", id),
			zap.String("mac", rec.MACAddress),
			zap.String("ip", rec.IPAddress))
	} else {
		s.logger.Debug("Asset updated",
			zap.String("asset_id", id),
			zap.String("mac", rec.MACAddress))
	}

	return id, created, nil
}

// GetAsset 获取单个资产（按 ID 或 MAC）
func (s *AssetService) GetAsset(ctx context.Context, tenantID, assetID, macAddress string) (*config.AssetRecord, error) {
	if assetID == "" && macAddress == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "asset_id or mac_address required")
	}

	var rec *config.AssetRecord
	var err error

	if assetID != "" {
		rec, err = s.repo.FindByID(ctx, assetID)
	} else {
		rec, err = s.repo.FindByMAC(ctx, tenantID, macAddress)
	}

	if err != nil {
		return nil, err
	}

	return rec, nil
}

// ListAssets 列出租户资产
func (s *AssetService) ListAssets(ctx context.Context, tenantID string, limit, offset int) ([]*config.AssetRecord, int, error) {
	if tenantID == "" {
		return nil, 0, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	recs, total, err := s.repo.ListByTenant(ctx, tenantID, limit, offset)
	if err != nil {
		s.logger.Error("ListAssets failed",
			zap.String("tenant", tenantID),
			zap.Error(err))
		return nil, 0, fmt.Errorf("list assets: %w", err)
	}

	return recs, total, nil
}

// RecordMacIpBinding 批量记录 MAC→IP 绑定（来自探针 ARP/DHCP 被动发现）
func (s *AssetService) RecordMacIpBinding(ctx context.Context, bindings []*config.MacIpBinding) (accepted, rejected int32, err error) {
	if len(bindings) == 0 {
		return 0, 0, errors.New(errors.ErrCodeInvalidParameter, "at least one binding required")
	}

	for _, b := range bindings {
		if b.MACAddress == "" || b.IPAddress == "" {
			rejected++
			continue
		}

		b.MACAddress = normalizeMAC(b.MACAddress)
		if b.TenantID == "" {
			b.TenantID = "default"
		}
		if b.Source == "" {
			b.Source = "passive"
		}

		rec := &config.AssetRecord{
			AssetID:    uuid.New().String(),
			TenantID:   b.TenantID,
			IPAddress:  b.IPAddress,
			MACAddress: b.MACAddress,
			Source:     b.Source,
			Vendor:     s.ouiCache.LookupVendor(b.MACAddress),
		}

		if _, _, err := s.repo.Upsert(ctx, rec); err != nil {
			s.logger.Warn("RecordMacIpBinding upsert failed",
				zap.String("mac", b.MACAddress),
				zap.String("ip", b.IPAddress),
				zap.Error(err))
			rejected++
		} else {
			accepted++
		}
	}

	return accepted, rejected, nil
}

// GetAssetHistory 获取资产变更历史
func (s *AssetService) GetAssetHistory(ctx context.Context, assetID string, limit int) ([]*config.AssetEvent, error) {
	if assetID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "asset_id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	events, err := s.repo.GetHistory(ctx, assetID, limit)
	if err != nil {
		s.logger.Error("GetAssetHistory failed",
			zap.String("asset_id", assetID),
			zap.Error(err))
		return nil, fmt.Errorf("get asset history: %w", err)
	}

	return events, nil
}

// MarkInactiveAssets 标记 7 天无活跃的资产为 inactive（定时任务调用）
func (s *AssetService) MarkInactiveAssets(ctx context.Context, tenantID string) (int, error) {
	threshold := time.Now().Add(-7 * 24 * time.Hour)
	count, err := s.repo.MarkInactiveSince(ctx, tenantID, threshold)
	if err != nil {
		s.logger.Error("MarkInactiveAssets failed",
			zap.String("tenant", tenantID),
			zap.Error(err))
		return 0, fmt.Errorf("mark inactive: %w", err)
	}

	if count > 0 {
		s.logger.Info("Marked inactive assets",
			zap.String("tenant", tenantID),
			zap.Int("count", count))
	}

	return count, nil
}

// InitSchema 初始化数据库 Schema
func (s *AssetService) InitSchema(ctx context.Context) error {
	return s.repo.InitSchema(ctx)
}

// =============================================================================
// 辅助函数
// =============================================================================

// LookupVendor 根据 MAC 地址返回 OUI 厂商名称（独立函数，供 handler 直接调用）
func LookupVendor(mac string) string {
	if len(mac) < 8 {
		return "Unknown"
	}
	oui := mac[:8]
	vendors := map[string]string{
		"00:1a:c5": "Cisco Systems", "00:1b:21": "Intel Corporate",
		"00:0c:29": "VMware, Inc.", "08:00:27": "Oracle VirtualBox",
		"18:c0:09": "Broadcom Limited", "b8:27:eb": "Raspberry Pi Foundation",
		"dc:a6:32": "Raspberry Pi Trading", "00:50:56": "VMware ESX",
		"00:1b:63": "Apple, Inc.", "3c:15:c2": "Apple, Inc.",
		"00:1e:67": "Intel Corporate", "f0:1f:af": "Dell Inc.",
	}
	if v, ok := vendors[oui]; ok {
		return v
	}
	return "Unknown"
}

// normalizeMAC 规范化 MAC 地址为小写 xx:xx:xx:xx:xx:xx 格式
func normalizeMAC(mac string) string {
	// 移除分隔符并统一小写
	s := strings.ToLower(mac)
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, ".", "")

	if len(s) != 12 {
		return strings.ToLower(mac) // 无法规范化，返回原值
	}

	// 格式化为 xx:xx:xx:xx:xx:xx
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s",
		s[0:2], s[2:4], s[4:6], s[6:8], s[8:10], s[10:12])
}

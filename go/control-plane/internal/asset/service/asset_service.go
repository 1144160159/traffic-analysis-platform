package service

import (
	"context"
	"encoding/json"
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
	ouiCache              OUILookup
	scanner               DiscoveryScanner
	exportObjects         AssetExportObjectStore
	observationReader     AssetObservationReader
	alertContextReader    AssetAlertContextReader
	graphProjectionReader AssetGraphProjectionReader
	evidenceObjectReader  AssetEvidenceObjectReader
}

type AssetObservationReader interface {
	ReadAssetObservations(context.Context, string, *config.AssetRecord, time.Time) (*config.AssetObservationSummary, map[string]string, error)
}

type AssetAlertContextReader interface {
	ReadAssetAlertContext(context.Context, string, *config.AssetRecord, time.Time) (*config.AssetAlertContext, map[string]string, error)
}

type AssetGraphProjectionReader interface {
	ReadAssetGraphProjection(context.Context, string, *config.AssetRecord, time.Time) (*config.AssetGraphProjection, map[string]string, bool, error)
}

type AssetEvidenceObjectReader interface {
	ReadAssetEvidenceObjects(context.Context, string, *config.AssetRecord, time.Time, *config.AssetAlertContext) (*config.AssetEvidenceObjectSet, map[string]string, bool, error)
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
	svc := &AssetService{
		cfg:      cfg,
		repo:     repo,
		logger:   logger,
		ouiCache: &localOUICache{},
	}
	if cfg != nil {
		svc.scanner = NewSNMPDiscoveryScanner(cfg.Discovery, logger)
	}
	return svc
}

// NewWithOUICache 创建带 Redis OUI 缓存的 AssetService
func NewWithOUICache(cfg *config.Config, repo *repository.AssetRepository, logger *zap.Logger, ouiCache OUILookup) *AssetService {
	svc := &AssetService{
		cfg:      cfg,
		repo:     repo,
		logger:   logger,
		ouiCache: ouiCache,
	}
	if cfg != nil {
		svc.scanner = NewSNMPDiscoveryScanner(cfg.Discovery, logger)
	}
	return svc
}

func (s *AssetService) WithDiscoveryScanner(scanner DiscoveryScanner) *AssetService {
	s.scanner = scanner
	return s
}

func (s *AssetService) WithAssetExportObjectStore(store AssetExportObjectStore) *AssetService {
	s.exportObjects = store
	return s
}

// WithAssetDetailReaders installs optional, independently degradable
// cross-store readers. A failed reader never turns missing data into a
// zero-valued success section.
func (s *AssetService) WithAssetDetailReaders(observations AssetObservationReader, alerts AssetAlertContextReader) *AssetService {
	s.observationReader = observations
	s.alertContextReader = alerts
	return s
}

func (s *AssetService) WithAssetGraphProjectionReader(reader AssetGraphProjectionReader) *AssetService {
	s.graphProjectionReader = reader
	return s
}

func (s *AssetService) WithAssetEvidenceObjectReader(reader AssetEvidenceObjectReader) *AssetService {
	s.evidenceObjectReader = reader
	return s
}

func (s *AssetService) JWTSigningKey() string {
	if s == nil || s.cfg == nil {
		return ""
	}
	return s.cfg.Auth.JWTSigningKey
}

func (s *AssetService) AssetCursorV2Enabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Cursor.Enabled
}

func (s *AssetService) AssetDetailSnapshotV1Enabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Detail.SnapshotV1Enabled
}

func (s *AssetService) AssetGovernanceV1Enabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Governance.Enabled
}

// =============================================================================
// 业务方法
// =============================================================================

// UpsertAssetAtomic is the single authoritative asset mutation boundary.
// Human commands use an explicit expected revision. Trusted observation paths
// may resolve the current revision only when they also carry a stable source
// event identity, so replay is still decided before another revision is made.
func (s *AssetService) UpsertAssetAtomic(
	ctx context.Context,
	rec *config.AssetRecord,
	command config.AssetUpsertCommand,
) (*config.AssetUpsertResult, error) {
	if rec == nil || strings.TrimSpace(rec.MACAddress) == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "mac_address is required")
	}
	if strings.TrimSpace(rec.TenantID) == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "Idempotency-Key must be 16-200 characters")
	}
	if command.ExpectedRevision < 0 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "expected_revision must be non-negative")
	}
	if command.ActionID == "" {
		command.ActionID = config.AssetUpsertAction
	}
	if command.ActionID != config.AssetUpsertAction && command.ActionID != config.AssetObservationUpsertAction {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "unsupported asset upsert action_id")
	}
	if command.ActionID == config.AssetUpsertAction && strings.TrimSpace(command.Reason) == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "reason is required for an asset upsert command")
	}
	if command.ResolveCurrentRevision && command.ActionID != config.AssetObservationUpsertAction {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "current revision resolution is restricted to observation commands")
	}
	if command.HistoryEventType != "" && command.ActionID != config.AssetObservationUpsertAction {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "custom history event is restricted to observation commands")
	}
	if command.ObservedAt.After(time.Now().UTC().Add(5 * time.Minute)) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "observed_at is too far in the future")
	}
	if strings.TrimSpace(command.Actor) == "" || strings.TrimSpace(command.TraceID) == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "authenticated actor and trace_id are required")
	}
	rec.MACAddress = normalizeMAC(rec.MACAddress)
	if rec.AssetID != "" {
		if _, err := uuid.Parse(rec.AssetID); err != nil {
			return nil, errors.New(errors.ErrCodeInvalidParameter, "asset_id must be a UUID")
		}
	}
	if rec.Vendor == "" || rec.Vendor == "Unknown" {
		rec.Vendor = s.ouiCache.LookupVendor(rec.MACAddress)
	}
	if rec.Source == "" {
		rec.Source = "manual"
	}
	result, err := s.repo.UpsertAtomic(ctx, rec, command)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetAsset 获取单个资产（按 ID 或 MAC）
func (s *AssetService) GetAsset(ctx context.Context, tenantID, assetID, macAddress string) (*config.AssetRecord, error) {
	if tenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if assetID == "" && macAddress == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "asset_id or mac_address required")
	}

	var rec *config.AssetRecord
	var err error

	if assetID != "" {
		rec, err = s.repo.FindByID(ctx, tenantID, assetID)
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
	return s.ListAssetsByType(ctx, tenantID, "", limit, offset)
}

// ListAssetsByType 按 canonical 资产类型列出租户资产；空类型表示全部资产。
func (s *AssetService) ListAssetsByType(ctx context.Context, tenantID, assetType string, limit, offset int) ([]*config.AssetRecord, int, error) {
	return s.ListAssetsFiltered(ctx, tenantID, config.AssetListFilter{AssetType: assetType}, limit, offset)
}

// ListAssetsFiltered 按租户、类型和治理字段查询资产，不允许客户端覆盖租户范围。
func (s *AssetService) ListAssetsFiltered(ctx context.Context, tenantID string, filter config.AssetListFilter, limit, offset int) ([]*config.AssetRecord, int, error) {
	if tenantID == "" {
		return nil, 0, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if filter.AssetType != "" && !IsAssetType(filter.AssetType) {
		return nil, 0, errors.New(errors.ErrCodeInvalidParameter, "invalid asset_type")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	recs, total, err := s.repo.ListByTenantFiltered(ctx, tenantID, filter, limit, offset)
	if err != nil {
		s.logger.Error("ListAssets failed",
			zap.String("tenant", tenantID),
			zap.Error(err))
		return nil, 0, fmt.Errorf("list assets: %w", err)
	}

	return recs, total, nil
}

// ListAssetsCursor returns a stable PostgreSQL snapshot traversal. Cursor
// authenticity and tenant/filter binding are enforced by the HTTP boundary;
// the service independently validates the resulting authoritative scope.
func (s *AssetService) ListAssetsCursor(
	ctx context.Context,
	tenantID string,
	filter config.AssetListFilter,
	limit int,
	position *config.AssetCursorPosition,
) (*config.AssetCursorPage, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if filter.AssetType != "" && !IsAssetType(filter.AssetType) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid asset_type")
	}
	if limit < 1 || limit > 200 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "limit must be between 1 and 200")
	}
	page, err := s.repo.ListByTenantCursor(ctx, tenantID, filter, limit, position)
	if err != nil {
		s.logger.Error("ListAssetsCursor failed",
			zap.String("tenant", tenantID),
			zap.Error(err))
		return nil, fmt.Errorf("list assets cursor: %w", err)
	}
	return page, nil
}

func IsAssetType(assetType string) bool {
	switch assetType {
	case "endpoint", "server", "network-device", "business-system", "unknown":
		return true
	default:
		return false
	}
}

// GetAssetHistory 获取资产变更历史
func (s *AssetService) GetAssetHistory(ctx context.Context, tenantID, assetID string, limit int) ([]*config.AssetEvent, error) {
	if tenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if assetID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "asset_id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	events, err := s.repo.GetHistory(ctx, tenantID, assetID, limit)
	if err != nil {
		s.logger.Error("GetAssetHistory failed",
			zap.String("asset_id", assetID),
			zap.Error(err))
		return nil, fmt.Errorf("get asset history: %w", err)
	}

	return events, nil
}

// GetAssetDetails returns persisted interface, service and ownership context.
// The detail payload is stored with the canonical asset record so tenant and
// asset identity checks stay identical to the base detail endpoint.
func (s *AssetService) GetAssetDetails(ctx context.Context, tenantID, assetID string) (*config.AssetDetails, error) {
	asset, err := s.GetAsset(ctx, tenantID, assetID, "")
	if err != nil {
		return nil, err
	}
	details := &config.AssetDetails{
		AssetID:      asset.AssetID,
		DataContract: "canonical-asset-detail-v1",
		Ownership: config.AssetOwnership{
			Campus:     asset.Campus,
			Department: asset.Department,
			Owner:      asset.Owner,
		},
		ObservedAt: asset.LastSeen,
	}
	if len(asset.Metadata) == 0 {
		return details, nil
	}
	encoded, marshalErr := json.Marshal(asset.Metadata)
	if marshalErr != nil {
		return nil, fmt.Errorf("encode asset detail metadata: %w", marshalErr)
	}
	var stored struct {
		DataContract      string                         `json:"data_contract"`
		NetworkInterfaces []config.AssetNetworkInterface `json:"network_interfaces"`
		OpenServices      []config.AssetOpenService      `json:"open_services"`
		Ownership         config.AssetOwnership          `json:"ownership"`
	}
	if unmarshalErr := json.Unmarshal(encoded, &stored); unmarshalErr != nil {
		return nil, fmt.Errorf("decode asset detail metadata: %w", unmarshalErr)
	}
	if stored.DataContract != "" {
		details.DataContract = stored.DataContract
	}
	details.NetworkInterfaces = stored.NetworkInterfaces
	details.OpenServices = stored.OpenServices
	if stored.Ownership.Campus != "" {
		details.Ownership.Campus = stored.Ownership.Campus
	}
	if stored.Ownership.Department != "" {
		details.Ownership.Department = stored.Ownership.Department
	}
	if stored.Ownership.Owner != "" {
		details.Ownership.Owner = stored.Ownership.Owner
	}
	details.Ownership.BusinessSystems = stored.Ownership.BusinessSystems
	details.Ownership.AssetGroups = stored.Ownership.AssetGroups
	details.Ownership.DataDomains = stored.Ownership.DataDomains
	details.Ownership.Responsibilities = stored.Ownership.Responsibilities
	details.Ownership.PendingFields = stored.Ownership.PendingFields
	return details, nil
}

// GetAssetTopology returns a render-neutral graph for one tenant-scoped asset.
// Discovery links win when available; persisted topology_graph metadata is the
// fallback for asset types whose relationships originate in a CMDB/business API.
func (s *AssetService) GetAssetTopology(ctx context.Context, tenantID, assetID string) (*config.AssetTopologyGraph, error) {
	asset, err := s.GetAsset(ctx, tenantID, assetID, "")
	if err != nil {
		return nil, err
	}
	graph := &config.AssetTopologyGraph{
		AssetID:     asset.AssetID,
		Source:      "empty",
		FixtureMode: asset.Source == "acceptance-fixture",
		ObservedAt:  asset.LastSeen,
		Nodes: []config.AssetTopologyNode{{
			ID: asset.AssetID, Label: firstNonEmpty(asset.Hostname, asset.DisplayCode, asset.AssetID), Kind: asset.AssetType, Status: asset.Status,
		}},
		Edges: []config.AssetTopologyEdge{},
	}

	links, linkErr := s.repo.ListTopologyLinks(ctx, tenantID, assetID, 100)
	if linkErr != nil {
		return nil, fmt.Errorf("list topology links: %w", linkErr)
	}
	if len(links) > 0 {
		graph.Source = "discovery_neighbors"
		nodeIDs := map[string]struct{}{asset.AssetID: {}}
		for index, link := range links {
			sourceID := firstNonEmpty(link.SourceAssetID, prefixedIdentity("ip", link.SourceIP), prefixedIdentity("mac", link.SourceMAC), fmt.Sprintf("link-%s-source", link.LinkID))
			targetID := firstNonEmpty(link.NeighborAssetID, prefixedIdentity("ip", link.NeighborIP), prefixedIdentity("mac", link.NeighborMAC), fmt.Sprintf("link-%s-neighbor", link.LinkID))
			for _, node := range []config.AssetTopologyNode{
				{ID: sourceID, Label: firstNonEmpty(link.SourceIP, link.SourceMAC, shortIdentity(sourceID)), Kind: "asset", Status: "observed"},
				{ID: targetID, Label: firstNonEmpty(link.NeighborIP, link.NeighborMAC, shortIdentity(targetID)), Kind: "asset", Status: "observed"},
			} {
				if _, exists := nodeIDs[node.ID]; exists {
					continue
				}
				nodeIDs[node.ID] = struct{}{}
				graph.Nodes = append(graph.Nodes, node)
			}
			health := "healthy"
			if link.Confidence > 0 && link.Confidence < 60 {
				health = "warning"
			}
			graph.Edges = append(graph.Edges, config.AssetTopologyEdge{
				ID: firstNonEmpty(link.LinkID, fmt.Sprintf("discovery-%d", index)), Source: sourceID, Target: targetID,
				Relationship: "neighbor", Direction: "directed", Protocol: link.Protocol, Health: health,
				Confidence: link.Confidence, ObservedAt: link.ObservedAt,
			})
			if link.ObservedAt.After(graph.ObservedAt) {
				graph.ObservedAt = link.ObservedAt
			}
		}
		return graph, nil
	}

	if len(asset.Metadata) == 0 {
		return graph, nil
	}
	encoded, marshalErr := json.Marshal(asset.Metadata)
	if marshalErr != nil {
		return nil, fmt.Errorf("encode asset topology metadata: %w", marshalErr)
	}
	var stored struct {
		TopologyGraph struct {
			Nodes []config.AssetTopologyNode `json:"nodes"`
			Edges []config.AssetTopologyEdge `json:"edges"`
		} `json:"topology_graph"`
		TopologyNodes []string `json:"topology_nodes"`
	}
	if unmarshalErr := json.Unmarshal(encoded, &stored); unmarshalErr != nil {
		return nil, fmt.Errorf("decode asset topology metadata: %w", unmarshalErr)
	}
	if len(stored.TopologyGraph.Nodes) > 0 || len(stored.TopologyGraph.Edges) > 0 {
		graph.Source = "asset_metadata_graph"
		nodeIDs := map[string]struct{}{asset.AssetID: {}}
		for index, node := range stored.TopologyGraph.Nodes {
			if node.ID == "" || node.ID == "self" {
				node.ID = fmt.Sprintf("metadata-node-%d", index)
			}
			if node.Label == "" {
				node.Label = shortIdentity(node.ID)
			}
			if _, exists := nodeIDs[node.ID]; exists {
				continue
			}
			nodeIDs[node.ID] = struct{}{}
			graph.Nodes = append(graph.Nodes, node)
		}
		for index, edge := range stored.TopologyGraph.Edges {
			if edge.Source == "self" {
				edge.Source = asset.AssetID
			}
			if edge.Target == "self" {
				edge.Target = asset.AssetID
			}
			if edge.ID == "" {
				edge.ID = fmt.Sprintf("metadata-edge-%d", index)
			}
			if edge.Relationship == "" {
				edge.Relationship = "related_to"
			}
			if _, sourceOK := nodeIDs[edge.Source]; !sourceOK {
				continue
			}
			if _, targetOK := nodeIDs[edge.Target]; !targetOK {
				continue
			}
			graph.Edges = append(graph.Edges, edge)
		}
		return graph, nil
	}

	if len(stored.TopologyNodes) > 0 {
		graph.Source = "legacy_asset_metadata"
		for index, label := range stored.TopologyNodes {
			nodeID := fmt.Sprintf("metadata-node-%d", index)
			graph.Nodes = append(graph.Nodes, config.AssetTopologyNode{ID: nodeID, Label: label, Kind: "related", Status: "unknown"})
			graph.Edges = append(graph.Edges, config.AssetTopologyEdge{
				ID: fmt.Sprintf("metadata-edge-%d", index), Source: asset.AssetID, Target: nodeID,
				Relationship: "related_to", Direction: "directed", Health: "unknown",
			})
		}
	}
	return graph, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func prefixedIdentity(prefix, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return prefix + ":" + strings.TrimSpace(value)
}

func shortIdentity(value string) string {
	if len(value) <= 18 {
		return value
	}
	return value[:8]
}

func (s *AssetService) GetAssetStats(ctx context.Context, tenantID, assetType string) (*config.AssetStats, error) {
	return s.GetAssetStatsFiltered(ctx, tenantID, config.AssetListFilter{AssetType: assetType})
}

// GetAssetStatsFiltered returns KPIs for exactly the same scope as the visible list.
func (s *AssetService) GetAssetStatsFiltered(ctx context.Context, tenantID string, filter config.AssetListFilter) (*config.AssetStats, error) {
	if tenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if filter.AssetType != "" && !IsAssetType(filter.AssetType) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid asset_type")
	}
	return s.repo.GetStatsFiltered(ctx, tenantID, filter)
}

// MarkInactiveAssets 标记 7 天无活跃的资产为 inactive（定时任务调用）
func (s *AssetService) MarkInactiveAssets(ctx context.Context, tenantID string) (int, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return 0, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	// A daily UTC boundary makes scheduler retries identify the same logical
	// sweep while keeping the seven-day lifecycle policy deterministic.
	sweepAt := time.Now().UTC().Truncate(24 * time.Hour)
	threshold := sweepAt.Add(-7 * 24 * time.Hour)
	identity := fmt.Sprintf("%s:%s", tenantID, sweepAt.Format("2006-01-02"))
	result, err := s.repo.MarkInactiveSinceAtomic(ctx, tenantID, config.AssetInactiveCommand{
		ActionID:       config.AssetInactiveSweepAction,
		IdempotencyKey: stableAssetCommandKey("asset-inactive-sweep", identity),
		Actor:          "asset-lifecycle-scheduler",
		Reason:         "mark assets inactive after seven days without observation",
		TraceID:        "asset-inactive:" + identity,
		RequestID:      "asset-inactive:" + identity,
		Cutoff:         threshold,
	})
	if err != nil {
		s.logger.Error("MarkInactiveAssets failed",
			zap.String("tenant", tenantID),
			zap.Error(err))
		return 0, fmt.Errorf("mark inactive: %w", err)
	}

	if result.Count > 0 {
		s.logger.Info("Marked inactive assets",
			zap.String("tenant", tenantID),
			zap.Int("count", result.Count),
			zap.Bool("idempotent_replay", result.IdempotentReplay))
	}

	return result.Count, nil
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
		// 网络设备
		"00:1a:c5": "Cisco Systems", "00:1b:53": "Cisco Systems",
		"00:0d:bc": "Cisco Systems", "00:1e:49": "Cisco Systems",
		"00:1a:a1": "Cisco Systems", "00:18:19": "Cisco Systems",
		"00:1a:30": "Cisco-Linksys", "00:1e:e5": "Cisco-Linksys",
		"00:1f:6c": "Cisco Systems",
		"00:e0:4c": "Huawei Technologies", "00:18:82": "Huawei Technologies",
		"28:6e:d4": "Huawei Technologies", "00:1e:10": "Huawei Technologies",
		"00:14:6c": "Netgear", "00:1b:2f": "Netgear",
		"00:1f:33": "Netgear", "00:14:bf": "Cisco-Linksys",
		"00:0c:41": "Cisco-Linksys", "00:1a:70": "Cisco-Linksys",
		"00:1f:a7": "Juniper Networks", "00:12:1e": "Juniper Networks",
		"00:1e:58": "Arista Networks", "00:1b:21": "Intel Corporate",
		"00:1e:67": "Intel Corporate", "00:1b:77": "Intel Corporate",
		"d4:ae:52": "Dell Inc.", "f0:1f:af": "Dell Inc.",
		"00:1e:c9": "Dell Inc.", "b8:ca:3a": "Dell Inc.",
		"00:1b:78": "Hewlett Packard", "00:1f:29": "Hewlett Packard",
		"00:17:a4": "Hewlett Packard", "14:58:d0": "Hewlett Packard Enterprise",
		"00:0c:29": "VMware, Inc.", "00:50:56": "VMware, Inc.",
		"00:1c:14": "VMware, Inc.", "08:00:27": "Oracle VirtualBox",
		// 服务器/存储
		"00:1b:63": "Apple, Inc.", "3c:15:c2": "Apple, Inc.",
		"00:1e:c2": "Apple, Inc.", "00:1f:f3": "Apple, Inc.",
		"00:1d:4f": "Apple, Inc.", "a4:b1:97": "Apple, Inc.",
		"b8:27:eb": "Raspberry Pi Foundation", "dc:a6:32": "Raspberry Pi Trading",
		"18:c0:09": "Broadcom Limited", "00:10:18": "Broadcom",
		// 网络芯片/设备
		"00:15:5d": "Microsoft Corporation", "00:1d:d8": "Microsoft Corporation",
		"00:15:99": "Samsung Electronics", "00:1e:3d": "Samsung Electronics",
		"00:1e:df": "Sony Corporation", "00:01:4a": "Sony Corporation",
		"00:1a:80": "Nokia Corporation", "00:1e:3a": "Nokia Corporation",
		"00:16:3e": "XenSource (Citrix)", "00:1b:fc": "Citrix Systems",
		"00:1a:4a": "Oracle Corporation", "00:14:4f": "Oracle Corporation",
		"00:11:43": "IBM Corporation", "00:14:5e": "IBM Corporation",
		// IoT/嵌入式
		"00:1e:c0": "Espressif Inc. (ESP32)", "b4:e6:2d": "Amazon Technologies (Alexa)",
		"00:17:f2": "Apple (HomePod)", "00:04:a3": "Microchip Technology",
		"78:21:84": "ARRIS Group", "00:1d:cf": "ARRIS Group",
		// 打印机
		"00:1b:a9": "Brother Industries", "00:1e:8f": "Canon Inc.",
		"00:1a:a2": "Xerox Corporation", "00:00:74": "Ricoh Company",
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

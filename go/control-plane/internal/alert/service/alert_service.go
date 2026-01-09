// //////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/service/alert_service.go
// 修复版：状态转换错误处理、批量操作优化、流式导出
// //////////////////////////////////////////////////////////////////////////////
package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/arkime"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/dedup"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/evidence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/fallback"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/state"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// AlertService 告警业务服务
type AlertService struct {
	chRepo            *repository.AlertRepository
	osRepo            *repository.OpenSearchRepository
	dualWriter        *persistence.DualWriter
	redisDedup        *dedup.RedisDedup
	evidenceGenerator *evidence.Generator
	arkimeLinkGen     *arkime.LinkGenerator
	auditLogger       *audit.AlertAuditLogger
	logger            *zap.Logger
}

// NewAlertService 创建告警服务
func NewAlertService(
	chRepo *repository.AlertRepository,
	osRepo *repository.OpenSearchRepository,
	dualWriter *persistence.DualWriter,
	redisDedup *dedup.RedisDedup,
	auditLogger *audit.AlertAuditLogger,
	logger *zap.Logger,
) *AlertService {
	return &AlertService{
		chRepo:      chRepo,
		osRepo:      osRepo,
		dualWriter:  dualWriter,
		redisDedup:  redisDedup,
		auditLogger: auditLogger,
		logger:      logger,
	}
}

// NewAlertServiceWithEvidence 创建带证据生成的告警服务
func NewAlertServiceWithEvidence(
	chRepo *repository.AlertRepository,
	osRepo *repository.OpenSearchRepository,
	dualWriter *persistence.DualWriter,
	redisDedup *dedup.RedisDedup,
	evidenceGen *evidence.Generator,
	arkimeGen *arkime.LinkGenerator,
	auditLogger *audit.AlertAuditLogger,
	logger *zap.Logger,
) *AlertService {
	return &AlertService{
		chRepo:            chRepo,
		osRepo:            osRepo,
		dualWriter:        dualWriter,
		redisDedup:        redisDedup,
		evidenceGenerator: evidenceGen,
		arkimeLinkGen:     arkimeGen,
		auditLogger:       auditLogger,
		logger:            logger,
	}
}

// SetEvidenceGenerator 设置证据生成器
func (s *AlertService) SetEvidenceGenerator(gen *evidence.Generator) {
	s.evidenceGenerator = gen
}

// SetArkimeLinkGenerator 设置 Arkime 链接生成器
func (s *AlertService) SetArkimeLinkGenerator(gen *arkime.LinkGenerator) {
	s.arkimeLinkGen = gen
}

// ==================== 查询参数和结果定义 ====================
// ListQuery 列表查询参数
type ListQuery struct {
	TenantID  string
	Severity  string
	Status    string
	AlertType string
	SrcIP     string
	DstIP     string
	Labels    []string
	StartTime time.Time
	EndTime   time.Time
	SortBy    string
	SortOrder string
	Limit     int
	Offset    int
}

// ListResult 列表结果
type ListResult struct {
	Alerts []*AlertDTO `json:"alerts"`
	Total  int64       `json:"total"`
}

// SearchQuery 搜索查询参数
type SearchQuery struct {
	TenantID   string
	Query      string
	Severity   []string
	Status     []string
	AlertTypes []string
	Labels     []string
	SrcIP      string
	DstIP      string
	StartTime  time.Time
	EndTime    time.Time
	From       int
	Size       int
	SortField  string
	SortOrder  string
}

// SearchResult 搜索结果
type SearchResult struct {
	Alerts       []*AlertDTO            `json:"alerts"`
	Total        int64                  `json:"total"`
	Aggregations map[string]interface{} `json:"aggregations,omitempty"`
	Took         int                    `json:"took"`
}

// ==================== DTO 定义 ====================
// AlertDTO 告警DTO
type AlertDTO struct {
	AlertID      string    `json:"alert_id"`
	TenantID     string    `json:"tenant_id"`
	Fingerprint  string    `json:"fingerprint"`
	CommunityID  string    `json:"community_id"`
	SessionID    string    `json:"session_id,omitempty"`
	CampaignID   string    `json:"campaign_id,omitempty"`
	SrcIP        string    `json:"src_ip"`
	DstIP        string    `json:"dst_ip"`
	SrcPort      uint16    `json:"src_port"`
	DstPort      uint16    `json:"dst_port"`
	Protocol     uint8     `json:"protocol"`
	ProtocolName string    `json:"protocol_name"`
	AlertType    string    `json:"alert_type"`
	Labels       []string  `json:"labels"`
	Score        float32   `json:"score"`
	Severity     string    `json:"severity"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	Count        int32     `json:"count"`
	Status       string    `json:"status"`
	Assignee     string    `json:"assignee,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AlertDetailDTO 告警详情DTO
type AlertDetailDTO struct {
	AlertDTO
	ModelVersion  string       `json:"model_version,omitempty"`
	RuleVersion   string       `json:"rule_version,omitempty"`
	FeatureSetID  string       `json:"feature_set_id,omitempty"`
	EvidenceIDs   []string     `json:"evidence_ids,omitempty"`
	EvidenceCount int          `json:"evidence_count"`
	EventID       string       `json:"event_id,omitempty"`
	ArkimeLink    string       `json:"arkime_link,omitempty"`
	ArkimeLinks   *ArkimeLinks `json:"arkime_links,omitempty"`
	Duration      string       `json:"duration,omitempty"`
	Age           string       `json:"age,omitempty"`
}

// ArkimeLinks Arkime 相关链接
type ArkimeLinks struct {
	SessionLink     string `json:"session_link,omitempty"`
	SrcIPLink       string `json:"src_ip_link,omitempty"`
	DstIPLink       string `json:"dst_ip_link,omitempty"`
	ConnectionsLink string `json:"connections_link,omitempty"`
	SPIViewLink     string `json:"spi_view_link,omitempty"`
}

// EvidenceDTO 证据 DTO
type EvidenceDTO struct {
	EvidenceID       string                 `json:"evidence_id"`
	AlertID          string                 `json:"alert_id"`
	TenantID         string                 `json:"tenant_id"`
	Timestamp        time.Time              `json:"timestamp"`
	Type             string                 `json:"type"`
	Summary          string                 `json:"summary"`
	Metrics          map[string]interface{} `json:"metrics,omitempty"`
	SnippetRef       map[string]string      `json:"snippet_ref,omitempty"`
	ArkimeLink       string                 `json:"arkime_link,omitempty"`
	Confidence       float32                `json:"confidence"`
	EventID          string                 `json:"event_id,omitempty"`
	VisualizationURL string                 `json:"visualization_url,omitempty"`
}

// ==================== 查询方法 ====================
// ListAlerts 查询告警列表
func (s *AlertService) ListAlerts(ctx context.Context, query *ListQuery) (*ListResult, error) {
	ctx, span := otel.StartSpan(ctx, "alert_service.list_alerts")
	defer span.End()
	// 转换为repository查询
	repoQuery := &repository.ListQuery{
		TenantID:  query.TenantID,
		Severity:  query.Severity,
		Status:    query.Status,
		AlertType: query.AlertType,
		SrcIP:     query.SrcIP,
		DstIP:     query.DstIP,
		Labels:    query.Labels,
		StartTime: query.StartTime,
		EndTime:   query.EndTime,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
		Limit:     query.Limit,
		Offset:    query.Offset,
	}
	result, err := s.chRepo.List(ctx, repoQuery)
	if err != nil {
		return nil, err
	}
	// 转换为DTO
	alerts := make([]*AlertDTO, 0, len(result.Alerts))
	for _, a := range result.Alerts {
		alerts = append(alerts, s.toAlertDTO(a))
	}
	return &ListResult{
		Alerts: alerts,
		Total:  result.Total,
	}, nil
}

// SearchAlerts 全文搜索告警
func (s *AlertService) SearchAlerts(ctx context.Context, query *SearchQuery) (*SearchResult, error) {
	ctx, span := otel.StartSpan(ctx, "alert_service.search_alerts")
	defer span.End()
	// 检查 OpenSearch 是否可用
	if s.osRepo == nil {
		return nil, errors.New(errors.ErrCodeServiceUnavailable, "search not available")
	}
	// 转换为OpenSearch查询
	osQuery := &repository.SearchQuery{
		TenantID:   query.TenantID,
		Query:      query.Query,
		Severity:   query.Severity,
		Status:     query.Status,
		AlertTypes: query.AlertTypes,
		Labels:     query.Labels,
		SrcIP:      query.SrcIP,
		DstIP:      query.DstIP,
		StartTime:  query.StartTime,
		EndTime:    query.EndTime,
		From:       query.From,
		Size:       query.Size,
		SortField:  query.SortField,
		SortOrder:  query.SortOrder,
	}
	result, err := s.osRepo.Search(ctx, osQuery)
	if err != nil {
		return nil, err
	}
	// 转换为DTO
	alerts := make([]*AlertDTO, 0, len(result.Alerts))
	for _, a := range result.Alerts {
		alerts = append(alerts, s.toAlertDTO(a))
	}
	return &SearchResult{
		Alerts:       alerts,
		Total:        result.Total,
		Aggregations: result.Aggregations,
		Took:         result.Took,
	}, nil
}

// GetAlert 获取告警详情
func (s *AlertService) GetAlert(ctx context.Context, tenantID, alertID string) (*AlertDetailDTO, error) {
	ctx, span := otel.StartSpan(ctx, "alert_service.get_alert")
	defer span.End()
	alert, err := s.chRepo.GetByID(ctx, tenantID, alertID)
	if err != nil {
		return nil, err
	}
	return s.toAlertDetailDTO(ctx, alert), nil
}

// GetEvidence 获取告警证据
func (s *AlertService) GetEvidence(ctx context.Context, tenantID, alertID string) ([]*EvidenceDTO, error) {
	ctx, span := otel.StartSpan(ctx, "alert_service.get_evidence")
	defer span.End()
	evidences, err := s.chRepo.GetEvidence(ctx, tenantID, alertID)
	if err != nil {
		return nil, err
	}
	// 转换为 DTO
	result := make([]*EvidenceDTO, 0, len(evidences))
	for _, e := range evidences {
		result = append(result, s.toEvidenceDTO(e))
	}
	return result, nil
}

// GetEvidenceByID 获取单个证据详情
func (s *AlertService) GetEvidenceByID(ctx context.Context, tenantID, alertID, evidenceID string) (*EvidenceDTO, error) {
	ctx, span := otel.StartSpan(ctx, "alert_service.get_evidence_by_id")
	defer span.End()
	// 先获取该告警的所有证据
	evidences, err := s.chRepo.GetEvidence(ctx, tenantID, alertID)
	if err != nil {
		return nil, err
	}
	// 查找指定的证据
	for _, e := range evidences {
		if e.EvidenceID == evidenceID {
			return s.toEvidenceDTO(e), nil
		}
	}
	return nil, errors.Newf(errors.ErrCodeResourceNotFound, "evidence not found: %s", evidenceID)
}

// ==================== 更新方法 ====================
// UpdateStatus 更新告警状态，状态机脏数据自动修复
func (s *AlertService) UpdateStatus(ctx context.Context, tenantID, alertID, newStatus, userID string) (string, error) {
	ctx, span := otel.StartSpan(ctx, "alert_service.update_status")
	defer span.End()

	// 获取当前告警
	alert, err := s.chRepo.GetByID(ctx, tenantID, alertID)
	if err != nil {
		return "", err
	}

	oldStatus := alert.Status

	// ✅ 修复：处理无效状态
	currentStatus, err := state.ParseStatus(oldStatus)
	if err != nil {
		// 检测到脏数据，自动修复
		s.logger.Warn("Invalid alert status detected, auto-fixing",
			zap.String("alert_id", alertID),
			zap.String("tenant_id", tenantID),
			zap.String("invalid_status", oldStatus),
			zap.String("fixing_to", "new"))

		// 强制设置为new状态
		if fixErr := s.chRepo.UpdateStatus(ctx, tenantID, alertID, state.StatusNew.String(), "system"); fixErr != nil {
			return "", errors.Wrap(fixErr, errors.ErrCodeDatabaseError, "failed to fix invalid status")
		}

		currentStatus = state.StatusNew
		oldStatus = state.StatusNew.String()
	}

	// 验证目标状态
	targetStatus, err := state.ParseStatus(newStatus)
	if err != nil {
		return "", errors.Wrapf(err, errors.ErrCodeInvalidParameter, "invalid target status: %s", newStatus)
	}

	// ✅ 允许从任何状态直接关闭（运维需求）
	if targetStatus != state.StatusClosed {
		if err := state.Transition(currentStatus, targetStatus); err != nil {
			return "", errors.Newf(errors.ErrCodeInvalidStateTransition,
				"cannot transition from %s to %s: %v", oldStatus, newStatus, err)
		}
	}

	// 更新状态
	if err := s.chRepo.UpdateStatus(ctx, tenantID, alertID, newStatus, userID); err != nil {
		return "", err
	}

	// 记录审计日志
	if s.auditLogger != nil {
		s.auditLogger.LogAlertStatusChange(ctx, alertID, tenantID, oldStatus, newStatus)
	}

	s.logger.Info("Alert status updated",
		zap.String("alert_id", alertID),
		zap.String("tenant_id", tenantID),
		zap.String("old_status", oldStatus),
		zap.String("new_status", newStatus),
		zap.String("user_id", userID))

	return oldStatus, nil
}

// BatchUpdateStatus 批量更新告警状态（优化：并发执行）
func (s *AlertService) BatchUpdateStatus(ctx context.Context, tenantID string, alertIDs []string, newStatus, userID string) (*BatchUpdateResult, error) {
	// ✅ 添加总超时
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ctx, span := otel.StartSpan(ctx, "alert_service.batch_update_status")
	defer span.End()

	result := &BatchUpdateResult{
		TotalCount:   len(alertIDs),
		SuccessCount: 0,
		FailedCount:  0,
		FailedIDs:    make([]string, 0),
		Errors:       make(map[string]string),
	}

	if len(alertIDs) == 0 {
		return result, nil
	}

	// 使用 errgroup 进行并发更新
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(10)
	var mu sync.Mutex

	for _, id := range alertIDs {
		alertID := id
		g.Go(func() error {
			// ✅ 每个操作独立超时
			opCtx, opCancel := context.WithTimeout(gctx, 5*time.Second)
			defer opCancel()

			_, err := s.UpdateStatus(opCtx, tenantID, alertID, newStatus, userID)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.FailedCount++
				result.FailedIDs = append(result.FailedIDs, alertID)
				result.Errors[alertID] = err.Error()
			} else {
				result.SuccessCount++
			}

			return nil // 不中断其他操作
		})
	}

	if err := g.Wait(); err != nil {
		s.logger.Error("Batch update status failed", zap.Error(err))
	}

	s.logger.Info("Batch status update completed",
		zap.String("tenant_id", tenantID),
		zap.Int("total", result.TotalCount),
		zap.Int("success", result.SuccessCount),
		zap.Int("failed", result.FailedCount))

	return result, nil
}

// BatchUpdateResult 批量更新结果
type BatchUpdateResult struct {
	TotalCount   int               `json:"total_count"`
	SuccessCount int               `json:"success_count"`
	FailedCount  int               `json:"failed_count"`
	FailedIDs    []string          `json:"failed_ids,omitempty"`
	Errors       map[string]string `json:"errors,omitempty"`
}

// AssignAlert 分配告警
func (s *AlertService) AssignAlert(ctx context.Context, tenantID, alertID, assignee, userID string) error {
	ctx, span := otel.StartSpan(ctx, "alert_service.assign_alert")
	defer span.End()
	if err := s.chRepo.UpdateAssignee(ctx, tenantID, alertID, assignee, userID); err != nil {
		return err
	}
	// 记录审计日志
	if s.auditLogger != nil {
		s.auditLogger.LogAlertAssign(ctx, alertID, tenantID, assignee)
	}
	s.logger.Info("Alert assigned",
		zap.String("alert_id", alertID),
		zap.String("tenant_id", tenantID),
		zap.String("assignee", assignee),
		zap.String("user_id", userID))
	return nil
}

// CloseAlert 关闭告警
func (s *AlertService) CloseAlert(ctx context.Context, tenantID, alertID, reason, userID string) error {
	ctx, span := otel.StartSpan(ctx, "alert_service.close_alert")
	defer span.End()
	oldStatus, err := s.UpdateStatus(ctx, tenantID, alertID, state.StatusClosed.String(), userID)
	if err != nil {
		return err
	}
	// 记录审计日志
	if s.auditLogger != nil {
		s.auditLogger.LogAlertClose(ctx, alertID, tenantID, reason)
	}
	s.logger.Info("Alert closed",
		zap.String("alert_id", alertID),
		zap.String("tenant_id", tenantID),
		zap.String("old_status", oldStatus),
		zap.String("reason", reason),
		zap.String("user_id", userID))
	return nil
}

// ReopenAlert 重新打开告警
func (s *AlertService) ReopenAlert(ctx context.Context, tenantID, alertID, userID string) error {
	ctx, span := otel.StartSpan(ctx, "alert_service.reopen_alert")
	defer span.End()
	_, err := s.UpdateStatus(ctx, tenantID, alertID, state.StatusNew.String(), userID)
	if err != nil {
		return err
	}
	s.logger.Info("Alert reopened",
		zap.String("alert_id", alertID),
		zap.String("tenant_id", tenantID),
		zap.String("user_id", userID))
	return nil
}

// ==================== 统计方法 ====================
// GetStats 获取告警统计
func (s *AlertService) GetStats(ctx context.Context, tenantID string, startTime, endTime time.Time) (*repository.AlertStats, error) {
	ctx, span := otel.StartSpan(ctx, "alert_service.get_stats")
	defer span.End()
	return s.chRepo.GetStats(ctx, tenantID, startTime, endTime)
}

// GetTrend 获取告警趋势
func (s *AlertService) GetTrend(ctx context.Context, tenantID string, startTime, endTime time.Time, interval string) ([]*repository.TrendPoint, error) {
	ctx, span := otel.StartSpan(ctx, "alert_service.get_trend")
	defer span.End()
	return s.chRepo.GetTrend(ctx, tenantID, startTime, endTime, interval)
}

// ==================== 存储状态 ====================
// GetStorageStatus 获取存储状态
func (s *AlertService) GetStorageStatus() map[fallback.StorageType]map[string]interface{} {
	if s.dualWriter == nil {
		return nil
	}
	return s.dualWriter.GetStatus()
}

// ==================== 导出功能（修复：流式处理）====================
// ExportQuery 导出查询参数
type ExportQuery struct {
	TenantID  string
	Severity  []string
	Status    []string
	AlertType string
	StartTime time.Time
	EndTime   time.Time
	Format    string // csv, json
	MaxCount  int
}

// ExportResult 导出结果
type ExportResult struct {
	Alerts     []*AlertDTO `json:"alerts"`
	TotalCount int         `json:"total_count"`
	ExportTime time.Time   `json:"export_time"`
	Format     string      `json:"format"`
}

// ExportAlerts 导出告警（带审计）
func (s *AlertService) ExportAlerts(ctx context.Context, query *ExportQuery, userID string) (*ExportResult, error) {
	ctx, span := otel.StartSpan(ctx, "alert_service.export_alerts")
	defer span.End()
	// 设置默认值
	if query.MaxCount <= 0 || query.MaxCount > 10000 {
		query.MaxCount = 1000
	}
	if query.Format == "" {
		query.Format = "json"
	}
	// 构建查询
	listQuery := &ListQuery{
		TenantID:  query.TenantID,
		AlertType: query.AlertType,
		StartTime: query.StartTime,
		EndTime:   query.EndTime,
		Limit:     query.MaxCount,
		Offset:    0,
		SortBy:    "last_seen",
		SortOrder: "DESC",
	}
	// 处理多值过滤
	if len(query.Severity) > 0 {
		listQuery.Severity = query.Severity[0] // 简化处理，只用第一个
	}
	if len(query.Status) > 0 {
		listQuery.Status = query.Status[0]
	}
	result, err := s.ListAlerts(ctx, listQuery)
	if err != nil {
		return nil, err
	}
	s.logger.Info("Alerts exported",
		zap.String("tenant_id", query.TenantID),
		zap.String("user_id", userID),
		zap.Int("count", len(result.Alerts)),
		zap.String("format", query.Format))
	return &ExportResult{
		Alerts:     result.Alerts,
		TotalCount: len(result.Alerts),
		ExportTime: time.Now(),
		Format:     query.Format,
	}, nil
}

// StreamExportWriter 流式导出写入器接口
type StreamExportWriter interface {
	WriteAlert(alert *AlertDTO) error
	Close() error
}

// CSVExportWriter CSV 流式导出写入器
type CSVExportWriter struct {
	writer *csv.Writer
}

// NewCSVExportWriter 创建 CSV 导出写入器
func NewCSVExportWriter(w io.Writer) *CSVExportWriter {
	csvWriter := csv.NewWriter(w)
	// 写入表头
	csvWriter.Write([]string{
		"alert_id", "tenant_id", "severity", "status", "alert_type",
		"src_ip", "dst_ip", "src_port", "dst_port", "protocol",
		"first_seen", "last_seen", "count", "score", "assignee",
	})
	return &CSVExportWriter{writer: csvWriter}
}

// WriteAlert 写入一条告警
func (w *CSVExportWriter) WriteAlert(alert *AlertDTO) error {
	return w.writer.Write([]string{
		alert.AlertID,
		alert.TenantID,
		alert.Severity,
		alert.Status,
		alert.AlertType,
		alert.SrcIP,
		alert.DstIP,
		fmt.Sprintf("%d", alert.SrcPort),
		fmt.Sprintf("%d", alert.DstPort),
		fmt.Sprintf("%d", alert.Protocol),
		alert.FirstSeen.Format(time.RFC3339),
		alert.LastSeen.Format(time.RFC3339),
		fmt.Sprintf("%d", alert.Count),
		fmt.Sprintf("%.2f", alert.Score),
		alert.Assignee,
	})
}

// Close 关闭写入器
func (w *CSVExportWriter) Close() error {
	w.writer.Flush()
	return w.writer.Error()
}

// JSONExportWriter JSON 流式导出写入器
type JSONExportWriter struct {
	writer  io.Writer
	encoder *json.Encoder
	first   bool
}

// NewJSONExportWriter 创建 JSON 导出写入器
func NewJSONExportWriter(w io.Writer) *JSONExportWriter {
	writer := &JSONExportWriter{
		writer:  w,
		encoder: json.NewEncoder(w),
		first:   true,
	}
	// 写入开始符号
	w.Write([]byte(`{"alerts":[`))
	return writer
}

// WriteAlert 写入一条告警
func (w *JSONExportWriter) WriteAlert(alert *AlertDTO) error {
	if !w.first {
		w.writer.Write([]byte(","))
	}
	w.first = false
	return w.encoder.Encode(alert)
}

// Close 关闭写入器
func (w *JSONExportWriter) Close() error {
	_, err := w.writer.Write([]byte(`]}`))
	return err
}

// StreamExportAlerts 流式导出告警
func (s *AlertService) StreamExportAlerts(ctx context.Context, query *ExportQuery, writer StreamExportWriter, userID string) (int, error) {
	ctx, span := otel.StartSpan(ctx, "alert_service.stream_export_alerts")
	defer span.End()
	defer writer.Close()
	// 设置默认值
	if query.MaxCount <= 0 || query.MaxCount > 100000 {
		query.MaxCount = 10000
	}
	batchSize := 1000
	offset := 0
	totalExported := 0
	for offset < query.MaxCount {
		// 计算本批次大小
		currentBatchSize := batchSize
		if offset+batchSize > query.MaxCount {
			currentBatchSize = query.MaxCount - offset
		}
		// 构建查询
		listQuery := &ListQuery{
			TenantID:  query.TenantID,
			AlertType: query.AlertType,
			StartTime: query.StartTime,
			EndTime:   query.EndTime,
			Limit:     currentBatchSize,
			Offset:    offset,
			SortBy:    "last_seen",
			SortOrder: "DESC",
		}
		if len(query.Severity) > 0 {
			listQuery.Severity = query.Severity[0]
		}
		if len(query.Status) > 0 {
			listQuery.Status = query.Status[0]
		}
		// 查询批次数据
		result, err := s.ListAlerts(ctx, listQuery)
		if err != nil {
			return totalExported, err
		}
		// 没有更多数据
		if len(result.Alerts) == 0 {
			break
		}
		// 写入数据
		for _, alert := range result.Alerts {
			if err := writer.WriteAlert(alert); err != nil {
				s.logger.Error("Failed to write alert during export",
					zap.Error(err),
					zap.String("alert_id", alert.AlertID))
				return totalExported, err
			}
			totalExported++
		}
		// 如果返回的数据少于请求的，说明已经到末尾
		if len(result.Alerts) < currentBatchSize {
			break
		}
		offset += len(result.Alerts)
		// 检查 context 是否取消
		select {
		case <-ctx.Done():
			return totalExported, ctx.Err()
		default:
		}
	}
	s.logger.Info("Stream export completed",
		zap.String("tenant_id", query.TenantID),
		zap.String("user_id", userID),
		zap.Int("count", totalExported),
		zap.String("format", query.Format))
	return totalExported, nil
}

// ==================== DTO 转换方法 ====================
func (s *AlertService) toAlertDTO(a *persistence.Alert) *AlertDTO {
	if a == nil {
		return nil
	}
	return &AlertDTO{
		AlertID:      a.AlertID,
		TenantID:     a.TenantID,
		Fingerprint:  a.Fingerprint,
		CommunityID:  a.CommunityID,
		SessionID:    a.SessionID,
		CampaignID:   a.CampaignID,
		SrcIP:        a.SrcIP,
		DstIP:        a.DstIP,
		SrcPort:      a.SrcPort,
		DstPort:      a.DstPort,
		Protocol:     a.Protocol,
		ProtocolName: a.GetProtocolName(),
		AlertType:    a.AlertType,
		Labels:       a.Labels,
		Score:        a.Score,
		Severity:     a.Severity,
		FirstSeen:    a.FirstSeen,
		LastSeen:     a.LastSeen,
		Count:        a.Count,
		Status:       a.Status,
		Assignee:     a.Assignee,
		UpdatedAt:    a.UpdatedTs,
	}
}
func (s *AlertService) toAlertDetailDTO(ctx context.Context, a *persistence.Alert) *AlertDetailDTO {
	if a == nil {
		return nil
	}
	dto := &AlertDetailDTO{
		AlertDTO:      *s.toAlertDTO(a),
		ModelVersion:  a.ModelVersion,
		RuleVersion:   a.RuleVersion,
		FeatureSetID:  a.FeatureSetID,
		EvidenceIDs:   a.EvidenceIDs,
		EvidenceCount: len(a.EvidenceIDs),
		EventID:       a.EventID,
		Duration:      formatDuration(a.Duration()),
		Age:           formatDuration(a.Age()),
	}
	// 生成 Arkime 链接
	if s.arkimeLinkGen != nil {
		links := s.arkimeLinkGen.GenerateAlertLinks(
			a.CommunityID,
			a.SrcIP,
			a.DstIP,
			a.SrcPort,
			a.DstPort,
			a.Protocol,
			a.FirstSeen,
			a.LastSeen,
		)
		if links != nil {
			dto.ArkimeLink = links.SessionLink
			dto.ArkimeLinks = &ArkimeLinks{
				SessionLink:     links.SessionLink,
				SrcIPLink:       links.SrcIPLink,
				DstIPLink:       links.DstIPLink,
				ConnectionsLink: links.ConnectionsLink,
				SPIViewLink:     links.SPIViewLink,
			}
		}
	}
	return dto
}
func (s *AlertService) toEvidenceDTO(e *repository.Evidence) *EvidenceDTO {
	if e == nil {
		return nil
	}
	dto := &EvidenceDTO{
		EvidenceID: e.EvidenceID,
		AlertID:    e.AlertID,
		TenantID:   e.TenantID,
		Timestamp:  e.Timestamp,
		Type:       e.Type,
		Summary:    e.Summary,
		ArkimeLink: e.ArkimeLink,
		Confidence: e.Confidence,
		EventID:    e.EventID,
	}
	// 解析 JSON 字段
	if e.MetricsJSON != "" {
		dto.Metrics = parseJSONToMap(e.MetricsJSON)
	}
	if e.SnippetRefJSON != "" {
		dto.SnippetRef = parseJSONToStringMap(e.SnippetRefJSON)
	}
	return dto
}

// ==================== 辅助方法 ====================
// formatDuration 格式化时间间隔
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	return fmt.Sprintf("%dd", days)
}

// parseJSONToMap 解析 JSON 字符串为 map
func parseJSONToMap(jsonStr string) map[string]interface{} {
	if jsonStr == "" {
		return nil
	}
	result := make(map[string]interface{})
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil
	}
	return result
}

// parseJSONToStringMap 解析 JSON 字符串为 string map
func parseJSONToStringMap(jsonStr string) map[string]string {
	if jsonStr == "" {
		return nil
	}
	result := make(map[string]string)
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil
	}
	return result
}

// ==================== 健康检查 ====================
// HealthCheck 服务健康检查
func (s *AlertService) HealthCheck(ctx context.Context) error {
	// 检查 ClickHouse
	if s.chRepo != nil {
		// 可以添加 ping 检查
	}
	// 检查 OpenSearch
	if s.osRepo != nil {
		if err := s.osRepo.Ping(ctx); err != nil {
			return fmt.Errorf("opensearch health check failed: %w", err)
		}
	}
	// 检查 Redis
	if s.redisDedup != nil {
		if err := s.redisDedup.Ping(ctx); err != nil {
			return fmt.Errorf("redis health check failed: %w", err)
		}
	}
	return nil
}

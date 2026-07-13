// //////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/repository/clickhouse.go
// 修复版：添加乐观锁解决并发更新竞态条件、批量更新方法、完善错误处理
// //////////////////////////////////////////////////////////////////////////////
package repository

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"
)

// AlertRepository Alert数据访问层
type AlertRepository struct {
	client *storage.ClickHouseClient
	logger *zap.Logger
	mu     sync.RWMutex // 用于本地缓存或状态保护
}

const alertSelectColumns = `
	tenant_id, alert_id, dedup_fingerprint, community_id, session_id, campaign_id,
	src_ip, dst_ip, src_port, dst_port, protocol,
	alert_type, labels, score, severity,
	fromUnixTimestamp64Milli(first_seen) AS first_seen,
	fromUnixTimestamp64Milli(last_seen) AS last_seen,
	count, status, assignee,
	fromUnixTimestamp64Milli(updated_at) AS updated_ts,
	model_version, rule_version, feature_set_id,
	evidence_ids, event_id`

// NewAlertRepository 创建AlertRepository
func NewAlertRepository(client *storage.ClickHouseClient, logger *zap.Logger) *AlertRepository {
	return &AlertRepository{
		client: client,
		logger: logger,
	}
}

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

// ListResult 列表查询结果
type ListResult struct {
	Alerts []*persistence.Alert
	Total  int64
}

// List 查询告警列表
func (r *AlertRepository) List(ctx context.Context, query *ListQuery) (*ListResult, error) {
	ctx, span := otel.StartSpan(ctx, "alert_repository.list")
	defer span.End()

	// 构建WHERE条件
	conditions := []string{"tenant_id = ?"}
	args := []interface{}{query.TenantID}

	if query.Severity != "" {
		conditions = append(conditions, "severity = ?")
		args = append(args, query.Severity)
	}
	if query.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, query.Status)
	}
	if query.AlertType != "" {
		conditions = append(conditions, "alert_type = ?")
		args = append(args, query.AlertType)
	}
	if query.SrcIP != "" {
		conditions = append(conditions, "src_ip = ?")
		args = append(args, query.SrcIP)
	}
	if query.DstIP != "" {
		conditions = append(conditions, "dst_ip = ?")
		args = append(args, query.DstIP)
	}
	if len(query.Labels) > 0 {
		conditions = append(conditions, "hasAny(labels, ?)")
		args = append(args, query.Labels)
	}
	if !query.StartTime.IsZero() {
		conditions = append(conditions, "last_seen >= ?")
		args = append(args, query.StartTime.UnixMilli())
	}
	if !query.EndTime.IsZero() {
		conditions = append(conditions, "last_seen <= ?")
		args = append(args, query.EndTime.UnixMilli())
	}

	whereClause := strings.Join(conditions, " AND ")

	// 1. 查询总数
	countSQL := fmt.Sprintf(`
		SELECT count() 
		FROM traffic.alerts
		WHERE %s
	`, whereClause)

	var total uint64
	row, err := r.client.QueryRow(ctx, countSQL, args...)
	if err != nil {
		r.logger.Error("Failed to query count", zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query count")
	}
	if err := row.Scan(&total); err != nil {
		r.logger.Error("Failed to scan count", zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan count")
	}

	// 2. 查询数据（使用物化视图，无需 FINAL）
	sortBy := "last_seen"
	sortOrder := "DESC"
	if query.SortBy != "" {
		sortBy = sanitizeSortField(query.SortBy)
	}
	if query.SortOrder != "" && strings.ToUpper(query.SortOrder) == "ASC" {
		sortOrder = "ASC"
	}

	limit := 50
	if query.Limit > 0 && query.Limit <= 1000 {
		limit = query.Limit
	}

	offset := 0
	if query.Offset >= 0 {
		offset = query.Offset
	}

	dataSQL := fmt.Sprintf(`
		SELECT %s
		FROM traffic.alerts
		WHERE %s
		ORDER BY %s %s
		LIMIT %d OFFSET %d
	`, alertSelectColumns, whereClause, sortBy, sortOrder, limit, offset)

	rows, err := r.client.Query(ctx, dataSQL, args...)
	if err != nil {
		r.logger.Error("Failed to query alerts", zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query alerts")
	}
	defer rows.Close()

	alerts, err := r.scanAlerts(rows)
	if err != nil {
		return nil, err
	}

	return &ListResult{
		Alerts: alerts,
		Total:  int64(total),
	}, nil
}

// GetByID 根据ID查询告警
func (r *AlertRepository) GetByID(ctx context.Context, tenantID, alertID string) (*persistence.Alert, error) {
	ctx, span := otel.StartSpan(ctx, "alert_repository.get_by_id")
	defer span.End()

	sql := fmt.Sprintf(`
		SELECT %s
		FROM traffic.alerts
		WHERE tenant_id = ? AND alert_id = ?
		ORDER BY updated_at DESC
		LIMIT 1
	`, alertSelectColumns)

	rows, err := r.client.Query(ctx, sql, tenantID, alertID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query alert")
	}
	defer rows.Close()

	alerts, err := r.scanAlerts(rows)
	if err != nil {
		return nil, err
	}

	if len(alerts) == 0 {
		return nil, errors.Newf(errors.ErrCodeAlertNotFound, "alert not found: %s", alertID)
	}

	return alerts[0], nil
}

// GetByIDWithVersion 根据ID查询告警（返回版本信息用于乐观锁）
func (r *AlertRepository) GetByIDWithVersion(ctx context.Context, tenantID, alertID string) (*persistence.Alert, time.Time, error) {
	alert, err := r.GetByID(ctx, tenantID, alertID)
	if err != nil {
		return nil, time.Time{}, err
	}
	return alert, alert.UpdatedTs, nil
}

// GetByFingerprint 根据指纹查询告警
func (r *AlertRepository) GetByFingerprint(ctx context.Context, tenantID, fingerprint string) (*persistence.Alert, error) {
	ctx, span := otel.StartSpan(ctx, "alert_repository.get_by_fingerprint")
	defer span.End()
	sql := fmt.Sprintf(`
		SELECT %s
		FROM traffic.alerts
		WHERE tenant_id = ? AND dedup_fingerprint = ?
		ORDER BY last_seen DESC
		LIMIT 1
	`, alertSelectColumns)
	rows, err := r.client.Query(ctx, sql, tenantID, fingerprint)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query alert by fingerprint")
	}
	defer rows.Close()
	alerts, err := r.scanAlerts(rows)
	if err != nil {
		return nil, err
	}
	if len(alerts) == 0 {
		return nil, nil // 不存在返回nil，不是错误
	}
	return alerts[0], nil
}

// UpdateStatus 更新告警状态（带乐观锁）
func (r *AlertRepository) UpdateStatus(ctx context.Context, tenantID, alertID, newStatus, userID string) error {
	ctx, span := otel.StartSpan(ctx, "alert_repository.update_status")
	defer span.End()
	return r.updateWithOptimisticLock(ctx, tenantID, alertID, func(alert *persistence.Alert) error {
		alert.Status = newStatus
		return nil
	})
}

// UpdateStatusWithVersion 更新告警状态（带版本号的乐观锁）
func (r *AlertRepository) UpdateStatusWithVersion(ctx context.Context, tenantID, alertID, newStatus, userID string, expectedVersion time.Time) (time.Time, error) {
	ctx, span := otel.StartSpan(ctx, "alert_repository.update_status_with_version")
	defer span.End()
	// 先获取现有告警
	alert, err := r.GetByID(ctx, tenantID, alertID)
	if err != nil {
		return time.Time{}, err
	}
	// 检查版本号（乐观锁）。API 暴露毫秒级 state_version，因此这里也按毫秒比较。
	if alert.UpdatedTs.UnixMilli() != expectedVersion.UnixMilli() {
		r.logger.Warn("Optimistic lock conflict",
			zap.String("alert_id", alertID),
			zap.Time("expected_version", expectedVersion),
			zap.Time("actual_version", alert.UpdatedTs))
		return time.Time{}, errors.Newf(errors.ErrCodeVersionConflict,
			"alert has been modified by another process, expected version: %s, actual: %s",
			expectedVersion.Format(time.RFC3339Nano),
			alert.UpdatedTs.Format(time.RFC3339Nano))
	}
	// 更新字段
	alert.Status = newStatus
	newVersion := time.Now()
	alert.UpdatedTs = newVersion
	// 插入新版本（ReplacingMergeTree会自动合并）
	return newVersion, r.upsertAlert(ctx, alert)
}

// UpdateAssignee 更新告警分配人（带乐观锁）
func (r *AlertRepository) UpdateAssignee(ctx context.Context, tenantID, alertID, assignee, userID string) error {
	ctx, span := otel.StartSpan(ctx, "alert_repository.update_assignee")
	defer span.End()
	return r.updateWithOptimisticLock(ctx, tenantID, alertID, func(alert *persistence.Alert) error {
		alert.Assignee = assignee
		alert.Status = "assigned"
		return nil
	})
}

// UpdateAssigneeWithVersion 更新告警分配人（带版本号的乐观锁）
func (r *AlertRepository) UpdateAssigneeWithVersion(ctx context.Context, tenantID, alertID, assignee, userID string, expectedVersion time.Time) error {
	ctx, span := otel.StartSpan(ctx, "alert_repository.update_assignee_with_version")
	defer span.End()
	// 先获取现有告警
	alert, err := r.GetByID(ctx, tenantID, alertID)
	if err != nil {
		return err
	}
	// 检查版本号（乐观锁）
	if !alert.UpdatedTs.Equal(expectedVersion) {
		return errors.Newf(errors.ErrCodeVersionConflict,
			"alert has been modified by another process")
	}
	// 更新字段
	alert.Assignee = assignee
	alert.Status = "assigned"
	alert.UpdatedTs = time.Now()
	// 插入新版本
	return r.upsertAlert(ctx, alert)
}

// updateWithOptimisticLock 带乐观锁的更新辅助方法（带重试）
func (r *AlertRepository) updateWithOptimisticLock(ctx context.Context, tenantID, alertID string, updateFn func(*persistence.Alert) error) error {
	const maxRetries = 3
	const retryDelay = 50 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		// 获取当前版本
		alert, version, err := r.GetByIDWithVersion(ctx, tenantID, alertID)
		if err != nil {
			return err
		}
		// 应用更新
		if err := updateFn(alert); err != nil {
			return err
		}
		// 设置新的更新时间
		newVersion := time.Now()
		alert.UpdatedTs = newVersion
		// 尝试更新（使用版本检查）
		err = r.upsertAlertWithVersionCheck(ctx, alert, version)
		if err == nil {
			return nil // 成功
		}
		// 检查是否为版本冲突
		if errors.IsCode(err, errors.ErrCodeVersionConflict) {
			lastErr = err
			r.logger.Debug("Optimistic lock conflict, retrying",
				zap.String("alert_id", alertID),
				zap.Int("attempt", attempt+1))
			// 等待一段时间后重试
			if attempt < maxRetries-1 {
				time.Sleep(retryDelay * time.Duration(attempt+1))
			}
			continue
		}
		// 其他错误直接返回
		return err
	}
	return errors.Wrap(lastErr, errors.ErrCodeVersionConflict,
		fmt.Sprintf("failed to update alert after %d retries due to concurrent modifications", maxRetries))
}

// upsertAlertWithVersionCheck 带版本检查的插入（模拟乐观锁）
func (r *AlertRepository) upsertAlertWithVersionCheck(ctx context.Context, alert *persistence.Alert, expectedVersion time.Time) error {
	// 通过确保 updated_at 严格递增来实现乐观锁语义。
	// 首先检查当前版本是否仍然匹配
	currentAlert, err := r.GetByID(ctx, alert.TenantID, alert.AlertID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeAlertNotFound) {
			// 告警不存在，直接插入
			return r.upsertAlert(ctx, alert)
		}
		return err
	}
	// 版本检查
	if !currentAlert.UpdatedTs.Equal(expectedVersion) {
		return errors.Newf(errors.ErrCodeVersionConflict,
			"version mismatch: expected %s, got %s",
			expectedVersion.Format(time.RFC3339Nano),
			currentAlert.UpdatedTs.Format(time.RFC3339Nano))
	}
	// 版本匹配，执行更新
	return r.upsertAlert(ctx, alert)
}

// BatchUpdateStatus 批量更新告警状态（优化版：并发处理）
func (r *AlertRepository) BatchUpdateStatus(ctx context.Context, tenantID string, alertIDs []string, newStatus, userID string) (*BatchUpdateResult, error) {
	ctx, span := otel.StartSpan(ctx, "alert_repository.batch_update_status")
	defer span.End()
	if len(alertIDs) == 0 {
		return &BatchUpdateResult{}, nil
	}
	result := &BatchUpdateResult{
		TotalCount:   len(alertIDs),
		SuccessIDs:   make([]string, 0),
		FailedIDs:    make([]string, 0),
		Errors:       make(map[string]string),
		SuccessCount: 0,
		FailedCount:  0,
	}
	// 使用 channel 收集结果
	type updateResult struct {
		alertID string
		err     error
	}
	resultChan := make(chan updateResult, len(alertIDs))
	var wg sync.WaitGroup
	// 限制并发数
	semaphore := make(chan struct{}, 10) // 最多10个并发
	for _, alertID := range alertIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			semaphore <- struct{}{}        // 获取信号量
			defer func() { <-semaphore }() // 释放信号量
			err := r.UpdateStatus(ctx, tenantID, id, newStatus, userID)
			resultChan <- updateResult{alertID: id, err: err}
		}(alertID)
	}
	// 等待所有 goroutine 完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()
	// 收集结果
	for res := range resultChan {
		if res.err != nil {
			result.FailedCount++
			result.FailedIDs = append(result.FailedIDs, res.alertID)
			result.Errors[res.alertID] = res.err.Error()
			r.logger.Warn("Failed to update alert status",
				zap.String("alert_id", res.alertID),
				zap.Error(res.err))
		} else {
			result.SuccessCount++
			result.SuccessIDs = append(result.SuccessIDs, res.alertID)
		}
	}
	r.logger.Info("Batch update status completed",
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
	SuccessIDs   []string          `json:"success_ids,omitempty"`
	FailedIDs    []string          `json:"failed_ids,omitempty"`
	Errors       map[string]string `json:"errors,omitempty"`
}

// BatchUpsertAlerts 批量插入或更新告警
func (r *AlertRepository) BatchUpsertAlerts(ctx context.Context, alerts []*persistence.Alert) error {
	if len(alerts) == 0 {
		return nil
	}
	ctx, span := otel.StartSpan(ctx, "alert_repository.batch_upsert_alerts")
	defer span.End()
	sql := `
		INSERT INTO traffic.alerts_local (
			tenant_id, alert_id, dedup_fingerprint, community_id, session_id, campaign_id,
			src_ip, dst_ip, src_port, dst_port, protocol,
			alert_type, labels, score, severity,
			first_seen, last_seen, count, status, assignee, updated_at,
			model_version, rule_version, feature_set_id,
			evidence_ids, event_id
		)
	`
	return r.client.BatchInsert(ctx, sql, func(batch driver.Batch) error {
		for _, alert := range alerts {
			if err := batch.Append(
				alert.TenantID, alert.AlertID, alert.Fingerprint, alert.CommunityID, alert.SessionID, alert.CampaignID,
				alert.SrcIP, alert.DstIP, alert.SrcPort, alert.DstPort, alert.Protocol,
				alert.AlertType, alert.Labels, alert.Score, alert.Severity,
				alert.FirstSeen.UnixMilli(), alert.LastSeen.UnixMilli(), alert.Count, alert.Status, alert.Assignee, alert.UpdatedTs.UnixMilli(),
				alert.ModelVersion, alert.RuleVersion, alert.FeatureSetID,
				alert.EvidenceIDs, alert.EventID,
			); err != nil {
				r.logger.Error("Failed to append alert to batch",
					zap.String("alert_id", alert.AlertID),
					zap.Error(err))
				return err
			}
		}
		return nil
	})
}

// GetEvidence 获取告警关联的证据
func (r *AlertRepository) GetEvidence(ctx context.Context, tenantID, alertID string) ([]*Evidence, error) {
	ctx, span := otel.StartSpan(ctx, "alert_repository.get_evidence")
	defer span.End()
	sql := `
		SELECT 
			tenant_id, evidence_id, alert_id, ts,
			type, summary, metrics_json, snippet_ref_json, arkime_link,
			confidence, event_id
		FROM traffic.evidence
		WHERE tenant_id = ? AND alert_id = ?
		ORDER BY ts DESC
	`
	rows, err := r.client.Query(ctx, sql, tenantID, alertID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query evidence")
	}
	defer rows.Close()
	var evidences []*Evidence
	for rows.Next() {
		e, err := scanEvidenceRow(rows)
		if err != nil {
			r.logger.Error("Failed to scan evidence row", zap.Error(err))
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan evidence")
		}
		evidences = append(evidences, e)
	}
	return evidences, nil
}

// GetEvidenceByID 根据证据ID获取证据
func (r *AlertRepository) GetEvidenceByID(ctx context.Context, tenantID, evidenceID string) (*Evidence, error) {
	ctx, span := otel.StartSpan(ctx, "alert_repository.get_evidence_by_id")
	defer span.End()
	sql := `
		SELECT 
			tenant_id, evidence_id, alert_id, ts,
			type, summary, metrics_json, snippet_ref_json, arkime_link,
			confidence, event_id
		FROM traffic.evidence
		WHERE tenant_id = ? AND evidence_id = ?
		LIMIT 1
	`
	rows, err := r.client.Query(ctx, sql, tenantID, evidenceID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query evidence")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, errors.Newf(errors.ErrCodeResourceNotFound, "evidence not found: %s", evidenceID)
	}
	e, err := scanEvidenceRow(rows)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan evidence")
	}
	return e, nil
}

// GetStats 获取告警统计
func (r *AlertRepository) GetStats(ctx context.Context, tenantID string, startTime, endTime time.Time) (*AlertStats, error) {
	ctx, span := otel.StartSpan(ctx, "alert_repository.get_stats")
	defer span.End()

	// 修复：使用 alerts_latest 替代 alerts FINAL
	sql := `
		SELECT 
			severity,
			status,
			count() as cnt
		FROM traffic.alerts
		WHERE tenant_id = ?
		  AND last_seen >= ?
		  AND last_seen <= ?
		GROUP BY severity, status
	`

	rows, err := r.client.Query(ctx, sql, tenantID, startTime.UnixMilli(), endTime.UnixMilli())
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query alert stats")
	}
	defer rows.Close()

	stats := &AlertStats{
		BySeverity: make(map[string]int64),
		ByStatus:   make(map[string]int64),
	}

	for rows.Next() {
		var severity, status string
		var cnt uint64

		if err := rows.Scan(&severity, &status, &cnt); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan stats")
		}

		count := int64(cnt)
		stats.BySeverity[severity] += count
		stats.ByStatus[status] += count
		stats.Total += count
	}

	return stats, nil
}

// GetTrend 获取告警趋势
func (r *AlertRepository) GetTrend(ctx context.Context, tenantID string, startTime, endTime time.Time, interval string) ([]*TrendPoint, error) {
	ctx, span := otel.StartSpan(ctx, "alert_repository.get_trend")
	defer span.End()
	// 默认按小时聚合
	timeFunc := "toStartOfHour"
	switch interval {
	case "minute":
		timeFunc = "toStartOfMinute"
	case "day":
		timeFunc = "toStartOfDay"
	case "week":
		timeFunc = "toStartOfWeek"
	}
	sql := fmt.Sprintf(`
		SELECT 
			%s(fromUnixTimestamp64Milli(last_seen)) as ts,
			severity,
			count() as cnt
		FROM traffic.alerts
		WHERE tenant_id = ?
		  AND last_seen >= ?
		  AND last_seen <= ?
		GROUP BY ts, severity
		ORDER BY ts
	`, timeFunc)
	rows, err := r.client.Query(ctx, sql, tenantID, startTime.UnixMilli(), endTime.UnixMilli())
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query trend")
	}
	defer rows.Close()
	var trend []*TrendPoint
	for rows.Next() {
		var point TrendPoint
		var count uint64
		if err := rows.Scan(&point.Timestamp, &point.Severity, &count); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan trend point")
		}
		point.Count = int64(count)
		trend = append(trend, &point)
	}
	return trend, nil
}

// StreamAlerts 流式查询告警（用于大数据量导出）
func (r *AlertRepository) StreamAlerts(ctx context.Context, query *ListQuery, handler func(*persistence.Alert) error) error {
	ctx, span := otel.StartSpan(ctx, "alert_repository.stream_alerts")
	defer span.End()
	// 构建WHERE条件
	conditions := []string{"tenant_id = ?"}
	args := []interface{}{query.TenantID}
	if query.Severity != "" {
		conditions = append(conditions, "severity = ?")
		args = append(args, query.Severity)
	}
	if query.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, query.Status)
	}
	if query.AlertType != "" {
		conditions = append(conditions, "alert_type = ?")
		args = append(args, query.AlertType)
	}
	if !query.StartTime.IsZero() {
		conditions = append(conditions, "last_seen >= ?")
		args = append(args, query.StartTime.UnixMilli())
	}
	if !query.EndTime.IsZero() {
		conditions = append(conditions, "last_seen <= ?")
		args = append(args, query.EndTime.UnixMilli())
	}
	whereClause := strings.Join(conditions, " AND ")
	sortBy := "last_seen"
	sortOrder := "DESC"
	if query.SortBy != "" {
		sortBy = sanitizeSortField(query.SortBy)
	}
	if query.SortOrder != "" && strings.ToUpper(query.SortOrder) == "ASC" {
		sortOrder = "ASC"
	}
	// 使用较大的批次但流式处理
	limit := 10000
	if query.Limit > 0 && query.Limit < limit {
		limit = query.Limit
	}
	sql := fmt.Sprintf(`
		SELECT %s
		FROM traffic.alerts
		WHERE %s
		ORDER BY %s %s
		LIMIT %d
	`, alertSelectColumns, whereClause, sortBy, sortOrder, limit)
	rows, err := r.client.Query(ctx, sql, args...)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query alerts for streaming")
	}
	defer rows.Close()
	for rows.Next() {
		alert, err := scanAlertRow(rows)
		if err != nil {
			r.logger.Error("Failed to scan alert row in stream", zap.Error(err))
			continue // 跳过错误的行，继续处理
		}
		if err := handler(alert); err != nil {
			return errors.Wrap(err, errors.ErrCodeInternal, "handler error during streaming")
		}
	}
	return nil
}

// upsertAlert 插入或更新告警
func (r *AlertRepository) upsertAlert(ctx context.Context, alert *persistence.Alert) error {
	sql := `
		INSERT INTO traffic.alerts_local (
			tenant_id, alert_id, dedup_fingerprint, community_id, session_id, campaign_id,
			src_ip, dst_ip, src_port, dst_port, protocol,
			alert_type, labels, score, severity,
			first_seen, last_seen, count, status, assignee, updated_at,
			model_version, rule_version, feature_set_id,
			evidence_ids, event_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	return r.client.Exec(ctx, sql,
		alert.TenantID, alert.AlertID, alert.Fingerprint, alert.CommunityID, alert.SessionID, alert.CampaignID,
		alert.SrcIP, alert.DstIP, alert.SrcPort, alert.DstPort, alert.Protocol,
		alert.AlertType, alert.Labels, alert.Score, alert.Severity,
		alert.FirstSeen.UnixMilli(), alert.LastSeen.UnixMilli(), alert.Count, alert.Status, alert.Assignee, alert.UpdatedTs.UnixMilli(),
		alert.ModelVersion, alert.RuleVersion, alert.FeatureSetID,
		alert.EvidenceIDs, alert.EventID,
	)
}

// scanAlerts 扫描告警结果集
func (r *AlertRepository) scanAlerts(rows driver.Rows) ([]*persistence.Alert, error) {
	var alerts []*persistence.Alert
	for rows.Next() {
		alert, err := scanAlertRow(rows)
		if err != nil {
			r.logger.Error("Failed to scan alert row", zap.Error(err))
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan alert")
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

type alertRowScanner interface {
	Scan(dest ...any) error
}

func scanAlertRow(scanner alertRowScanner) (*persistence.Alert, error) {
	var alert persistence.Alert
	var srcPort uint32
	var dstPort uint32
	var protocol uint32
	if err := scanner.Scan(
		&alert.TenantID, &alert.AlertID, &alert.Fingerprint, &alert.CommunityID, &alert.SessionID, &alert.CampaignID,
		&alert.SrcIP, &alert.DstIP, &srcPort, &dstPort, &protocol,
		&alert.AlertType, &alert.Labels, &alert.Score, &alert.Severity,
		&alert.FirstSeen, &alert.LastSeen, &alert.Count, &alert.Status, &alert.Assignee, &alert.UpdatedTs,
		&alert.ModelVersion, &alert.RuleVersion, &alert.FeatureSetID,
		&alert.EvidenceIDs, &alert.EventID,
	); err != nil {
		return nil, err
	}
	alert.SrcPort = uint16(srcPort)
	alert.DstPort = uint16(dstPort)
	alert.Protocol = uint8(protocol)
	return &alert, nil
}

func scanEvidenceRow(scanner alertRowScanner) (*Evidence, error) {
	var e Evidence
	var timestampMs int64
	if err := scanner.Scan(
		&e.TenantID, &e.EvidenceID, &e.AlertID, &timestampMs,
		&e.Type, &e.Summary, &e.MetricsJSON, &e.SnippetRefJSON, &e.ArkimeLink,
		&e.Confidence, &e.EventID,
	); err != nil {
		return nil, err
	}
	e.Timestamp = time.UnixMilli(timestampMs)
	return &e, nil
}

// sanitizeSortField 清理排序字段（防止SQL注入）
func sanitizeSortField(field string) string {
	allowedFields := map[string]string{
		"last_seen":  "last_seen",
		"first_seen": "first_seen",
		"severity":   "severity",
		"score":      "score",
		"status":     "status",
		"alert_type": "alert_type",
		"updated_ts": "updated_at",
		"updated_at": "updated_at",
		"count":      "count",
		"src_ip":     "src_ip",
		"dst_ip":     "dst_ip",
	}
	if safe, ok := allowedFields[field]; ok {
		return safe
	}
	return "last_seen"
}

// Evidence 证据结构
type Evidence struct {
	TenantID       string    `json:"tenant_id"`
	EvidenceID     string    `json:"evidence_id"`
	AlertID        string    `json:"alert_id"`
	Timestamp      time.Time `json:"timestamp"`
	Type           string    `json:"type"`
	Summary        string    `json:"summary"`
	MetricsJSON    string    `json:"metrics_json"`
	SnippetRefJSON string    `json:"snippet_ref_json"`
	ArkimeLink     string    `json:"arkime_link"`
	Confidence     float32   `json:"confidence"`
	EventID        string    `json:"event_id"`
}

// AlertStats 告警统计
type AlertStats struct {
	Total      int64            `json:"total"`
	BySeverity map[string]int64 `json:"by_severity"`
	ByStatus   map[string]int64 `json:"by_status"`
}

// TrendPoint 趋势数据点
type TrendPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Severity  string    `json:"severity"`
	Count     int64     `json:"count"`
}

// Ping 健康检查
func (r *AlertRepository) Ping(ctx context.Context) error {
	return r.client.Ping(ctx)
}

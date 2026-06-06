package api

import (
	"context"
	"fmt"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"
)

// FeedbackRecord 反馈持久化记录 — 对应 ClickHouse alert_feedback 表
type FeedbackRecord struct {
	FeedbackID     string    `json:"feedback_id" ch:"feedback_id"`
	AlertID        string    `json:"alert_id" ch:"alert_id"`
	TenantID       string    `json:"tenant_id" ch:"tenant_id"`
	UserID         string    `json:"user_id" ch:"user_id"`
	Label          string    `json:"label" ch:"label"`                 // TP | FP
	ReasonCode     string    `json:"reason_code" ch:"reason_code"`     // FP 原因码
	Comment        string    `json:"comment" ch:"comment"`
	AddToWhitelist bool      `json:"add_to_whitelist" ch:"add_to_whitelist"`
	AlertType      string    `json:"alert_type" ch:"alert_type"`       // 告警类型（冗余，方便分析）
	Severity       string    `json:"severity" ch:"severity"`           // 严重程度（冗余）
	ModelVersion   string    `json:"model_version" ch:"model_version"` // 模型版本（用于评估模型效果）
	RuleVersion    string    `json:"rule_version" ch:"rule_version"`   // 规则版本
	CreatedAt      time.Time `json:"created_at" ch:"created_at"`
}

// FeedbackRepository 反馈持久化仓库
type FeedbackRepository struct {
	client *storage.ClickHouseClient
	logger *zap.Logger
}

// NewFeedbackRepository 创建反馈仓库
func NewFeedbackRepository(client *storage.ClickHouseClient, logger *zap.Logger) *FeedbackRepository {
	return &FeedbackRepository{client: client, logger: logger}
}

// InitSchema 初始化反馈表
func (r *FeedbackRepository) InitSchema(ctx context.Context) error {
	ddl := `
	CREATE TABLE IF NOT EXISTS traffic.alert_feedback_local ON CLUSTER traffic_cluster (
		feedback_id     String,
		alert_id        String,
		tenant_id       String,
		user_id         String,
		label           LowCardinality(String),
		reason_code     String,
		comment         String,
		add_to_whitelist UInt8,
		alert_type      String,
		severity        LowCardinality(String),
		model_version   String,
		rule_version    String,
		created_at      DateTime64(3)
	) ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/alert_feedback_local', '{replica}')
	PARTITION BY toYYYYMM(created_at)
	ORDER BY (tenant_id, alert_id, created_at)
	TTL created_at + INTERVAL 365 DAY
	`
	return r.client.Exec(ctx, ddl)
}

// Insert 插入反馈记录
func (r *FeedbackRepository) Insert(ctx context.Context, record *FeedbackRecord) error {
	sql := `INSERT INTO traffic.alert_feedback_local (feedback_id, alert_id, tenant_id, user_id, label, reason_code, comment, add_to_whitelist, alert_type, severity, model_version, rule_version, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	return r.client.Exec(ctx, sql,
		record.FeedbackID, record.AlertID, record.TenantID, record.UserID,
		record.Label, record.ReasonCode, record.Comment, record.AddToWhitelist,
		record.AlertType, record.Severity, record.ModelVersion, record.RuleVersion,
		record.CreatedAt,
	)
}

// GetByAlertID 查询告警的所有反馈记录
func (r *FeedbackRepository) GetByAlertID(ctx context.Context, tenantID, alertID string) ([]*FeedbackRecord, error) {
	sql := `SELECT feedback_id, alert_id, tenant_id, user_id, label, reason_code, comment, add_to_whitelist, alert_type, severity, model_version, rule_version, created_at FROM traffic.alert_feedback FINAL WHERE tenant_id = ? AND alert_id = ? ORDER BY created_at DESC`
	rows, err := r.client.Query(ctx, sql, tenantID, alertID)
	if err != nil {
		return nil, fmt.Errorf("query feedback: %w", err)
	}
	defer rows.Close()
	return r.scanRows(rows)
}

// GetStats 获取反馈统计（按模型版本/规则版本聚合 TP/FP 分布）
func (r *FeedbackRepository) GetStats(ctx context.Context, tenantID string, since time.Duration) (map[string]interface{}, error) {
	sql := `SELECT label, count() as cnt, avg(if(label='FP',1,0)) as fp_rate FROM traffic.alert_feedback FINAL WHERE tenant_id = ? AND created_at >= now() - INTERVAL 30 DAY GROUP BY label`
	rows, err := r.client.Query(ctx, sql, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query feedback stats: %w", err)
	}
	defer rows.Close()

	stats := map[string]interface{}{"tp_count": int64(0), "fp_count": int64(0), "total": int64(0), "fp_rate": float64(0)}
	for rows.Next() {
		var label string
		var cnt int64
		if err := rows.Scan(&label, &cnt); err != nil {
			continue
		}
		if label == "TP" {
			stats["tp_count"] = cnt
		} else if label == "FP" {
			stats["fp_count"] = cnt
		}
	}
	tp := stats["tp_count"].(int64)
	fp := stats["fp_count"].(int64)
	total := tp + fp
	stats["total"] = total
	if total > 0 {
		stats["fp_rate"] = float64(fp) / float64(total)
	}
	return stats, nil
}

// GetFPRanking 获取 Top-N 误报原因排行
func (r *FeedbackRepository) GetFPRanking(ctx context.Context, tenantID string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 10
	}
	sql := `SELECT reason_code, count() as cnt FROM traffic.alert_feedback FINAL WHERE tenant_id = ? AND label = 'FP' AND created_at >= now() - INTERVAL 30 DAY GROUP BY reason_code ORDER BY cnt DESC LIMIT ?`
	rows, err := r.client.Query(ctx, sql, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("query fp ranking: %w", err)
	}
	defer rows.Close()

	var ranking []map[string]interface{}
	for rows.Next() {
		var code string
		var cnt int64
		if err := rows.Scan(&code, &cnt); err != nil {
			continue
		}
		ranking = append(ranking, map[string]interface{}{
			"reason_code": code,
			"count":       cnt,
			"description": FPReasonCodes[code],
		})
	}
	return ranking, nil
}

func (r *FeedbackRepository) scanRows(rows driver.Rows) ([]*FeedbackRecord, error) {
	var records []*FeedbackRecord
	for rows.Next() {
		var rec FeedbackRecord
		if err := rows.Scan(&rec.FeedbackID, &rec.AlertID, &rec.TenantID, &rec.UserID,
			&rec.Label, &rec.ReasonCode, &rec.Comment, &rec.AddToWhitelist,
			&rec.AlertType, &rec.Severity, &rec.ModelVersion, &rec.RuleVersion, &rec.CreatedAt,
		); err != nil {
			r.logger.Error("Failed to scan feedback row", zap.Error(err))
			continue
		}
		records = append(records, &rec)
	}
	return records, nil
}

// Ensure errors package is used
var _ = errors.ErrCodeAlertNotFound

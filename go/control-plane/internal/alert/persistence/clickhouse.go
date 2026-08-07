////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/persistence/clickhouse.go
// 修复版：添加 count 字段到 SQL 语句
////////////////////////////////////////////////////////////////////////////////

package persistence

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// ClickHouseWriter ClickHouse写入器
type ClickHouseWriter struct {
	client     *storage.ClickHouseClient
	logger     *zap.Logger
	schemaOnce sync.Once
	schema     alertWriteSchema
	schemaErr  error
}

type alertTimestampMode uint8

const (
	alertTimestampInt64Millis alertTimestampMode = iota + 1
	alertTimestampDateTime64
)

type alertWriteSchema struct {
	updatedColumn string
	timestampMode alertTimestampMode
}

// NewClickHouseWriter 创建ClickHouse写入器（使用common封装）
func NewClickHouseWriter(client *storage.ClickHouseClient, logger *zap.Logger) (*ClickHouseWriter, error) {
	if client == nil {
		return nil, fmt.Errorf("ClickHouse client is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ClickHouseWriter{
		client: client,
		logger: logger,
	}, nil
}

// WriteAlert 写入单个告警
func (w *ClickHouseWriter) WriteAlert(ctx context.Context, alert *Alert) error {
	ctx, span := otel.StartSpan(ctx, "clickhouse_writer.write_alert")
	defer span.End()

	if alert == nil {
		return fmt.Errorf("alert is required")
	}
	schema, err := w.ensureAlertSchema(ctx)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`
		INSERT INTO traffic.alerts (
			tenant_id, alert_id, dedup_fingerprint, community_id, session_id, campaign_id,
			src_ip, dst_ip, src_port, dst_port, protocol,
			alert_type, labels, score, severity,
			first_seen, last_seen, count, status, assignee, %s,
			model_version, rule_version, feature_set_id,
			evidence_ids, event_id, trace_id
		) VALUES (
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?
		)
	`, schema.updatedColumn)

	start := time.Now()
	err = w.client.Exec(ctx, query,
		alert.TenantID,
		alert.AlertID,
		alert.Fingerprint,
		alert.CommunityID,
		alert.SessionID,
		alert.CampaignID,
		alert.SrcIP,
		alert.DstIP,
		alert.SrcPort,
		alert.DstPort,
		alert.Protocol,
		alert.AlertType,
		alert.Labels,
		alert.Score,
		alert.Severity,
		schema.timestamp(alert.FirstSeen),
		schema.timestamp(alert.LastSeen),
		alert.Count,
		alert.Status,
		alert.Assignee,
		schema.timestamp(alert.UpdatedTs),
		alert.ModelVersion,
		alert.RuleVersion,
		alert.FeatureSetID,
		alert.EvidenceIDs,
		alert.EventID,
		alert.TraceID,
	)

	if err != nil {
		w.logger.Error("Failed to write alert to ClickHouse",
			zap.String("alert_id", alert.AlertID),
			zap.Duration("duration", time.Since(start)),
			zap.Error(err))
		otel.RecordError(ctx, err)
		return fmt.Errorf("write alert failed: %w", err)
	}

	w.logger.Debug("Alert written to ClickHouse",
		zap.String("alert_id", alert.AlertID),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// WriteBatch 批量写入告警
func (w *ClickHouseWriter) WriteBatch(ctx context.Context, alerts []*Alert) error {
	if len(alerts) == 0 {
		return nil
	}

	ctx, span := otel.StartSpan(ctx, "clickhouse_writer.write_batch")
	defer span.End()

	for _, alert := range alerts {
		if alert == nil {
			return fmt.Errorf("batch contains nil alert")
		}
	}
	schema, err := w.ensureAlertSchema(ctx)
	if err != nil {
		return err
	}
	start := time.Now()

	// 使用正确的 driver.Batch 类型
	err = w.client.BatchInsert(ctx, fmt.Sprintf(`
		INSERT INTO traffic.alerts (
			tenant_id, alert_id, dedup_fingerprint, community_id, session_id, campaign_id,
			src_ip, dst_ip, src_port, dst_port, protocol,
			alert_type, labels, score, severity,
			first_seen, last_seen, count, status, assignee, %s,
			model_version, rule_version, feature_set_id,
			evidence_ids, event_id, trace_id
		)
	`, schema.updatedColumn), func(batch driver.Batch) error {
		for _, alert := range alerts {
			if err := batch.Append(
				alert.TenantID,
				alert.AlertID,
				alert.Fingerprint,
				alert.CommunityID,
				alert.SessionID,
				alert.CampaignID,
				alert.SrcIP,
				alert.DstIP,
				alert.SrcPort,
				alert.DstPort,
				alert.Protocol,
				alert.AlertType,
				alert.Labels,
				alert.Score,
				alert.Severity,
				schema.timestamp(alert.FirstSeen),
				schema.timestamp(alert.LastSeen),
				alert.Count,
				alert.Status,
				alert.Assignee,
				schema.timestamp(alert.UpdatedTs),
				alert.ModelVersion,
				alert.RuleVersion,
				alert.FeatureSetID,
				alert.EvidenceIDs,
				alert.EventID,
				alert.TraceID,
			); err != nil {
				w.logger.Error("Failed to append alert to batch",
					zap.String("alert_id", alert.AlertID),
					zap.Error(err))
				return err
			}
		}
		return nil
	})

	if err != nil {
		w.logger.Error("Batch write failed",
			zap.Int("count", len(alerts)),
			zap.Duration("duration", time.Since(start)),
			zap.Error(err))
		otel.RecordError(ctx, err)
		return fmt.Errorf("batch write failed: %w", err)
	}

	w.logger.Info("Batch write completed",
		zap.Int("count", len(alerts)),
		zap.Duration("duration", time.Since(start)))

	return nil
}

func (w *ClickHouseWriter) ensureAlertSchema(ctx context.Context) (alertWriteSchema, error) {
	w.schemaOnce.Do(func() {
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		rows, err := w.client.Query(queryCtx, `
			SELECT name,type
			FROM system.columns
			WHERE database='traffic' AND table='alerts'
			  AND name IN ('first_seen','last_seen','updated_at','updated_ts')`)
		if err != nil {
			w.schemaErr = fmt.Errorf("inspect traffic.alerts timestamp schema: %w", err)
			return
		}
		defer rows.Close()
		columns := make(map[string]string, 4)
		for rows.Next() {
			var name, dataType string
			if err := rows.Scan(&name, &dataType); err != nil {
				w.schemaErr = fmt.Errorf("scan traffic.alerts timestamp schema: %w", err)
				return
			}
			columns[name] = dataType
		}
		if err := rows.Err(); err != nil {
			w.schemaErr = fmt.Errorf("iterate traffic.alerts timestamp schema: %w", err)
			return
		}
		w.schema, w.schemaErr = resolveAlertWriteSchema(columns)
	})
	return w.schema, w.schemaErr
}

func resolveAlertWriteSchema(columns map[string]string) (alertWriteSchema, error) {
	firstType, firstOK := columns["first_seen"]
	lastType, lastOK := columns["last_seen"]
	if !firstOK || !lastOK {
		return alertWriteSchema{}, fmt.Errorf("traffic.alerts misses first_seen or last_seen")
	}
	if updatedType, ok := columns["updated_at"]; ok {
		if !strings.HasPrefix(firstType, "Int64") || !strings.HasPrefix(lastType, "Int64") || !strings.HasPrefix(updatedType, "Int64") {
			return alertWriteSchema{}, fmt.Errorf("traffic.alerts updated_at schema must use Int64 millisecond timestamps")
		}
		return alertWriteSchema{updatedColumn: "updated_at", timestampMode: alertTimestampInt64Millis}, nil
	}
	if updatedType, ok := columns["updated_ts"]; ok {
		if !strings.HasPrefix(firstType, "DateTime64") || !strings.HasPrefix(lastType, "DateTime64") || !strings.HasPrefix(updatedType, "DateTime64") {
			return alertWriteSchema{}, fmt.Errorf("traffic.alerts updated_ts compatibility schema must use DateTime64 timestamps")
		}
		return alertWriteSchema{updatedColumn: "updated_ts", timestampMode: alertTimestampDateTime64}, nil
	}
	return alertWriteSchema{}, fmt.Errorf("traffic.alerts has neither canonical updated_at nor compatibility updated_ts")
}

func (schema alertWriteSchema) timestamp(value time.Time) any {
	if schema.timestampMode == alertTimestampInt64Millis {
		return value.UTC().UnixMilli()
	}
	return value.UTC()
}

// WriteEvidence 写入证据
func (w *ClickHouseWriter) WriteEvidence(ctx context.Context, evidence *EvidenceRecord) error {
	ctx, span := otel.StartSpan(ctx, "clickhouse_writer.write_evidence")
	defer span.End()

	query := `
		INSERT INTO traffic.evidence (
			tenant_id, evidence_id, alert_id, ts,
			type, summary, metrics_json, snippet_ref_json, arkime_link,
			confidence, event_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	err := w.client.Exec(ctx, query,
		evidence.TenantID,
		evidence.EvidenceID,
		evidence.AlertID,
		evidence.Timestamp,
		evidence.Type,
		evidence.Summary,
		evidence.MetricsJSON,
		evidence.SnippetRefJSON,
		evidence.ArkimeLink,
		evidence.Confidence,
		evidence.EventID,
	)

	if err != nil {
		w.logger.Error("Failed to write evidence",
			zap.String("evidence_id", evidence.EvidenceID),
			zap.Error(err))
		return fmt.Errorf("write evidence failed: %w", err)
	}

	return nil
}

// WriteEvidenceBatch 批量写入证据
func (w *ClickHouseWriter) WriteEvidenceBatch(ctx context.Context, evidences []*EvidenceRecord) error {
	if len(evidences) == 0 {
		return nil
	}

	ctx, span := otel.StartSpan(ctx, "clickhouse_writer.write_evidence_batch")
	defer span.End()

	err := w.client.BatchInsert(ctx, `
		INSERT INTO traffic.evidence (
			tenant_id, evidence_id, alert_id, ts,
			type, summary, metrics_json, snippet_ref_json, arkime_link,
			confidence, event_id
		)
	`, func(batch driver.Batch) error {
		for _, evidence := range evidences {
			if err := batch.Append(
				evidence.TenantID,
				evidence.EvidenceID,
				evidence.AlertID,
				evidence.Timestamp,
				evidence.Type,
				evidence.Summary,
				evidence.MetricsJSON,
				evidence.SnippetRefJSON,
				evidence.ArkimeLink,
				evidence.Confidence,
				evidence.EventID,
			); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		w.logger.Error("Batch evidence write failed",
			zap.Int("count", len(evidences)),
			zap.Error(err))
		return fmt.Errorf("batch evidence write failed: %w", err)
	}

	return nil
}

// Ping 健康检查
func (w *ClickHouseWriter) Ping(ctx context.Context) error {
	return w.client.Ping(ctx)
}

// Close 关闭连接
func (w *ClickHouseWriter) Close() error {
	return w.client.Close()
}

// GetStats 获取连接统计
func (w *ClickHouseWriter) GetStats() interface{} {
	return w.client.Stats()
}

// EvidenceRecord 证据记录（用于写入ClickHouse）
type EvidenceRecord struct {
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

package dataquality

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	maxRuleWindow  = 24 * time.Hour
	evaluatorActor = "system:data-quality-evaluator"
)

// RuleMeasurementReader is deliberately narrower than database/sql. It keeps
// the evaluator testable while the production implementation remains a
// bounded, tenant-scoped ClickHouse query.
type RuleMeasurementReader interface {
	MeasureRule(context.Context, string, string, string, json.RawMessage, time.Time, time.Time) (RuleMeasurement, error)
}

type RuleMeasurement struct {
	TotalCount       int64
	PassedCount      int64
	SourceWatermarks map[string]interface{}
}

type RuleEvaluation struct {
	EvaluationID    string                 `json:"evaluation_id"`
	TenantID        string                 `json:"tenant_id"`
	RuleID          string                 `json:"rule_id"`
	RuleVersion     int64                  `json:"rule_version"`
	DatasetID       string                 `json:"dataset_id"`
	WindowStart     time.Time              `json:"window_start"`
	WindowEnd       time.Time              `json:"window_end"`
	Status          string                 `json:"status"`
	TotalCount      int64                  `json:"total_count"`
	PassedCount     int64                  `json:"passed_count"`
	AffectedCount   int64                  `json:"affected_count"`
	MeasuredValue   map[string]interface{} `json:"measured_value"`
	SourceWatermark map[string]interface{} `json:"source_watermarks"`
	QualityEventID  string                 `json:"quality_event_id,omitempty"`
	TraceID         string                 `json:"trace_id"`
	EvaluatedAt     time.Time              `json:"evaluated_at"`
	Replayed        bool                   `json:"replayed"`
}

type activeRule struct {
	RuleID        string
	RuleVersion   int64
	DatasetID     string
	FieldPath     string
	Predicate     json.RawMessage
	Threshold     json.RawMessage
	WindowSeconds int64
	Severity      string
	Owner         string
}

type ClickHouseRuleMeasurementReader struct{ db *sql.DB }

func NewClickHouseRuleMeasurementReader(db *sql.DB) *ClickHouseRuleMeasurementReader {
	return &ClickHouseRuleMeasurementReader{db: db}
}

func (r *ClickHouseRuleMeasurementReader) MeasureRule(ctx context.Context, tenantID, datasetID, fieldPath string, predicate json.RawMessage, windowStart, windowEnd time.Time) (RuleMeasurement, error) {
	if r == nil || r.db == nil {
		return RuleMeasurement{}, fmt.Errorf("ClickHouse rule measurement reader is unavailable")
	}
	if datasetID != "flows_raw" {
		return RuleMeasurement{}, fmt.Errorf("unsupported data quality dataset %q", datasetID)
	}
	expression, err := safeFlowPredicate(fieldPath, predicate)
	if err != nil {
		return RuleMeasurement{}, err
	}
	query := `SELECT count(), countIf(` + expression + `), if(count()=0, 0, max(ingest_ts))
		FROM traffic.flows_raw
		WHERE tenant_id = ? AND ingest_ts >= ? AND ingest_ts < ?`
	var measurement RuleMeasurement
	var maxIngestMillis int64
	err = r.db.QueryRowContext(ctx, query, tenantID, windowStart.UnixMilli(), windowEnd.UnixMilli()).Scan(
		&measurement.TotalCount, &measurement.PassedCount, &maxIngestMillis,
	)
	if err != nil {
		return RuleMeasurement{}, fmt.Errorf("measure active data quality rule: %w", err)
	}
	measurement.SourceWatermarks = map[string]interface{}{
		"clickhouse_max_ingest_ts": maxIngestMillis,
		"window_start":             windowStart.UTC().Format(time.RFC3339Nano),
		"window_end":               windowEnd.UTC().Format(time.RFC3339Nano),
	}
	return measurement, nil
}

func safeFlowPredicate(fieldPath string, raw json.RawMessage) (string, error) {
	stringFields := map[string]bool{
		"event_id": true, "probe_id": true, "community_id": true, "src_ip": true,
		"dst_ip": true, "run_id": true, "feature_set_id": true, "direction": true,
	}
	numericFields := map[string]bool{
		"src_port": true, "dst_port": true, "protocol": true, "duration_ms": true,
		"packets_fwd": true, "packets_bwd": true, "bytes_fwd": true, "bytes_bwd": true,
	}
	type predicateDocument struct {
		Op string `json:"op"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var predicate predicateDocument
	if err := decoder.Decode(&predicate); err != nil {
		return "", fmt.Errorf("invalid data quality predicate: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("invalid trailing data quality predicate content")
	}
	switch predicate.Op {
	case "not_empty":
		if !stringFields[fieldPath] {
			return "", fmt.Errorf("not_empty is not allowed for field %q", fieldPath)
		}
		return "lengthUTF8(trim(" + fieldPath + ")) > 0", nil
	case "nonzero":
		if !numericFields[fieldPath] {
			return "", fmt.Errorf("nonzero is not allowed for field %q", fieldPath)
		}
		return fieldPath + " != 0", nil
	default:
		return "", fmt.Errorf("unsupported data quality predicate operator %q", predicate.Op)
	}
}

// EvaluateActiveRules evaluates only approved active rules. Draft, shadow and
// approval-pending definitions are excluded by the PostgreSQL query itself.
func (m *Monitor) EvaluateActiveRules(ctx context.Context, tenantID string, at time.Time, traceID string, reader RuleMeasurementReader) ([]RuleEvaluation, error) {
	if m == nil || m.controlDB == nil {
		return nil, ErrGovernanceUnavailable
	}
	if reader == nil {
		return nil, fmt.Errorf("data quality rule measurement reader is required")
	}
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(traceID) == "" {
		return nil, fmt.Errorf("tenant and trace are required for rule evaluation")
	}
	if at.IsZero() {
		at = time.Now()
	}
	rules, err := m.listActiveRules(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]RuleEvaluation, 0, len(rules))
	for _, rule := range rules {
		window, err := checkedRuleWindow(rule.WindowSeconds)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", rule.RuleID, err)
		}
		windowEnd := at.UTC().Truncate(window)
		windowStart := windowEnd.Add(-window)
		measurement, err := reader.MeasureRule(ctx, tenantID, rule.DatasetID, rule.FieldPath, rule.Predicate, windowStart, windowEnd)
		if err != nil {
			return nil, fmt.Errorf("rule %s measurement: %w", rule.RuleID, err)
		}
		evaluation, err := m.persistRuleEvaluation(ctx, tenantID, traceID, rule, windowStart, windowEnd, measurement)
		if err != nil {
			return nil, err
		}
		result = append(result, evaluation)
	}
	return result, nil
}

func (m *Monitor) listActiveRules(ctx context.Context, tenantID string) ([]activeRule, error) {
	rows, err := m.controlDB.QueryContext(ctx, `
		SELECT rule_id::text,rule_version,dataset_id,field_path,predicate,threshold,
			window_seconds,severity,owner
		FROM data_quality_rules
		WHERE tenant_id=$1 AND status='active'
		ORDER BY rule_id,rule_version
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list active data quality rules: %w", err)
	}
	defer rows.Close()
	var result []activeRule
	for rows.Next() {
		var rule activeRule
		if err := rows.Scan(&rule.RuleID, &rule.RuleVersion, &rule.DatasetID, &rule.FieldPath,
			&rule.Predicate, &rule.Threshold, &rule.WindowSeconds, &rule.Severity, &rule.Owner); err != nil {
			return nil, fmt.Errorf("scan active data quality rule: %w", err)
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

func (m *Monitor) persistRuleEvaluation(ctx context.Context, tenantID, traceID string, rule activeRule, windowStart, windowEnd time.Time, measurement RuleMeasurement) (RuleEvaluation, error) {
	minimum, err := minimumThreshold(rule.Threshold)
	if err != nil {
		return RuleEvaluation{}, fmt.Errorf("rule %s threshold: %w", rule.RuleID, err)
	}
	if measurement.TotalCount < 0 || measurement.PassedCount < 0 || measurement.PassedCount > measurement.TotalCount {
		return RuleEvaluation{}, fmt.Errorf("rule %s returned invalid counts", rule.RuleID)
	}
	status := "unknown"
	ratio := 0.0
	if measurement.TotalCount > 0 {
		ratio = float64(measurement.PassedCount) / float64(measurement.TotalCount)
		status = "pass"
		if ratio < minimum {
			status = "fail"
		}
	}
	if measurement.SourceWatermarks == nil {
		measurement.SourceWatermarks = map[string]interface{}{}
	}
	evaluationID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("dq-evaluation:%s:%s:%d:%s:%s", tenantID, rule.RuleID, rule.RuleVersion, windowStart.UTC().Format(time.RFC3339Nano), windowEnd.UTC().Format(time.RFC3339Nano))))
	qualityEventID := uuid.Nil
	if status == "fail" {
		qualityEventID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("dq-quality-event:"+evaluationID.String()))
	}
	measured := map[string]interface{}{"ratio": ratio, "minimum": minimum}
	measuredJSON, _ := json.Marshal(measured)
	watermarksJSON, err := json.Marshal(measurement.SourceWatermarks)
	if err != nil {
		return RuleEvaluation{}, fmt.Errorf("marshal rule source watermarks: %w", err)
	}
	tx, err := m.controlDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return RuleEvaluation{}, fmt.Errorf("begin rule evaluation: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, evaluationID.String()); err != nil {
		return RuleEvaluation{}, fmt.Errorf("lock rule evaluation: %w", err)
	}
	if replay, found, err := loadRuleEvaluation(ctx, tx, tenantID, evaluationID.String()); err != nil {
		return RuleEvaluation{}, err
	} else if found {
		replay.Replayed = true
		return replay, nil
	}
	if status == "fail" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data_quality_events (
				quality_event_id,tenant_id,dataset_id,rule_id,rule_version,status,severity,
				window_start,window_end,affected_count,measured_value,source_watermarks,
				owner,revision,trace_id,detected_at,updated_at
			) VALUES ($1,$2,$3,$4,$5,'detected',$6,$7,$8,$9,$10::jsonb,$11::jsonb,$12,1,$13,$8,$8)
		`, qualityEventID, tenantID, rule.DatasetID, rule.RuleID, rule.RuleVersion, rule.Severity,
			windowStart, windowEnd, measurement.TotalCount-measurement.PassedCount, string(measuredJSON), string(watermarksJSON), rule.Owner, traceID); err != nil {
			return RuleEvaluation{}, fmt.Errorf("insert detected quality event: %w", err)
		}
	}
	qualityEventArg := interface{}(nil)
	if qualityEventID != uuid.Nil {
		qualityEventArg = qualityEventID
	}
	var evaluation RuleEvaluation
	var eventID sql.NullString
	var measuredStored, watermarksStored []byte
	err = tx.QueryRowContext(ctx, `
		INSERT INTO data_quality_rule_evaluations (
			evaluation_id,tenant_id,rule_id,rule_version,dataset_id,window_start,window_end,status,
			total_count,passed_count,affected_count,measured_value,source_watermarks,quality_event_id,trace_id,evaluated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13::jsonb,$14,$15,$7)
		RETURNING evaluation_id::text,tenant_id,rule_id::text,rule_version,dataset_id,window_start,window_end,status,
			total_count,passed_count,affected_count,measured_value,source_watermarks,quality_event_id::text,trace_id,evaluated_at
	`, evaluationID, tenantID, rule.RuleID, rule.RuleVersion, rule.DatasetID, windowStart, windowEnd, status,
		measurement.TotalCount, measurement.PassedCount, measurement.TotalCount-measurement.PassedCount,
		string(measuredJSON), string(watermarksJSON), qualityEventArg, traceID).Scan(
		&evaluation.EvaluationID, &evaluation.TenantID, &evaluation.RuleID, &evaluation.RuleVersion,
		&evaluation.DatasetID, &evaluation.WindowStart, &evaluation.WindowEnd, &evaluation.Status,
		&evaluation.TotalCount, &evaluation.PassedCount, &evaluation.AffectedCount, &measuredStored,
		&watermarksStored, &eventID, &evaluation.TraceID, &evaluation.EvaluatedAt,
	)
	if err != nil {
		return RuleEvaluation{}, fmt.Errorf("insert rule evaluation: %w", err)
	}
	_ = json.Unmarshal(measuredStored, &evaluation.MeasuredValue)
	_ = json.Unmarshal(watermarksStored, &evaluation.SourceWatermark)
	if eventID.Valid {
		evaluation.QualityEventID = eventID.String
	}
	evaluationPayload, _ := json.Marshal(evaluation)
	evaluationOutboxID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("dq-evaluation-outbox:"+evaluationID.String()))
	if err := insertGovernanceOutbox(ctx, tx, evaluationOutboxID, tenantID, "rule", rule.RuleID, rule.RuleVersion,
		"DATA_QUALITY_RULE_EVALUATED", traceID, evaluationPayload); err != nil {
		return RuleEvaluation{}, err
	}
	if qualityEventID != uuid.Nil {
		eventPayload, _ := json.Marshal(map[string]interface{}{
			"quality_event_id": qualityEventID.String(), "evaluation_id": evaluationID.String(),
			"tenant_id": tenantID, "dataset_id": rule.DatasetID, "rule_id": rule.RuleID,
			"rule_version": rule.RuleVersion, "status": "detected", "trace_id": traceID,
		})
		eventOutboxID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("dq-quality-event-outbox:"+qualityEventID.String()))
		if err := insertGovernanceOutbox(ctx, tx, eventOutboxID, tenantID, "quality_event", qualityEventID.String(), 1,
			"DATA_QUALITY_EVENT_DETECTED", traceID, eventPayload); err != nil {
			return RuleEvaluation{}, err
		}
	}
	auditID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("dq-evaluation-audit:"+evaluationID.String()))
	if err := insertGovernanceAudit(ctx, tx, auditID, tenantID, evaluatorActor, "data_quality.rule.evaluated",
		"data_quality_rule_evaluation", evaluationID.String(), traceID, evaluationPayload); err != nil {
		return RuleEvaluation{}, err
	}
	if err := tx.Commit(); err != nil {
		return RuleEvaluation{}, fmt.Errorf("commit rule evaluation: %w", err)
	}
	return evaluation, nil
}

func loadRuleEvaluation(ctx context.Context, tx *sql.Tx, tenantID, evaluationID string) (RuleEvaluation, bool, error) {
	var evaluation RuleEvaluation
	var eventID sql.NullString
	var measured, watermarks []byte
	err := tx.QueryRowContext(ctx, `
		SELECT evaluation_id::text,tenant_id,rule_id::text,rule_version,dataset_id,window_start,window_end,status,
			total_count,passed_count,affected_count,measured_value,source_watermarks,quality_event_id::text,trace_id,evaluated_at
		FROM data_quality_rule_evaluations WHERE tenant_id=$1 AND evaluation_id=$2
	`, tenantID, evaluationID).Scan(
		&evaluation.EvaluationID, &evaluation.TenantID, &evaluation.RuleID, &evaluation.RuleVersion,
		&evaluation.DatasetID, &evaluation.WindowStart, &evaluation.WindowEnd, &evaluation.Status,
		&evaluation.TotalCount, &evaluation.PassedCount, &evaluation.AffectedCount, &measured,
		&watermarks, &eventID, &evaluation.TraceID, &evaluation.EvaluatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return evaluation, false, nil
	}
	if err != nil {
		return evaluation, false, fmt.Errorf("load existing rule evaluation: %w", err)
	}
	if err := json.Unmarshal(measured, &evaluation.MeasuredValue); err != nil {
		return evaluation, false, err
	}
	if err := json.Unmarshal(watermarks, &evaluation.SourceWatermark); err != nil {
		return evaluation, false, err
	}
	if eventID.Valid {
		evaluation.QualityEventID = eventID.String
	}
	return evaluation, true, nil
}

func minimumThreshold(raw json.RawMessage) (float64, error) {
	var document struct {
		Minimum *float64 `json:"minimum"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return 0, err
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return 0, fmt.Errorf("invalid trailing threshold content")
	}
	if document.Minimum == nil || math.IsNaN(*document.Minimum) || math.IsInf(*document.Minimum, 0) || *document.Minimum < 0 || *document.Minimum > 1 {
		return 0, fmt.Errorf("minimum must be a finite ratio between 0 and 1")
	}
	return *document.Minimum, nil
}

func checkedRuleWindow(seconds int64) (time.Duration, error) {
	if seconds <= 0 || seconds > int64(maxRuleWindow/time.Second) {
		return 0, fmt.Errorf("window_seconds must be between 1 and %d", int64(maxRuleWindow/time.Second))
	}
	return time.Duration(seconds) * time.Second, nil
}

func RunRuleEvaluationLoop(ctx context.Context, monitor *Monitor, reader RuleMeasurementReader, interval, timeout time.Duration, logger *zap.Logger) {
	if monitor == nil || reader == nil {
		return
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	evaluate := func() {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		tenantIDs, err := monitor.activeTenantIDs(runCtx)
		if err != nil {
			logger.Warn("Data quality rule evaluator could not list tenants", zap.Error(err))
			return
		}
		for _, tenantID := range tenantIDs {
			traceID := "dq-evaluate-" + uuid.NewString()
			results, err := monitor.EvaluateActiveRules(runCtx, tenantID, time.Now(), traceID, reader)
			if err != nil {
				logger.Warn("Data quality rule evaluation failed", zap.String("tenant_id", tenantID), zap.String("trace_id", traceID), zap.Error(err))
				continue
			}
			logger.Info("Data quality active rules evaluated", zap.String("tenant_id", tenantID), zap.Int("count", len(results)), zap.String("trace_id", traceID))
		}
	}
	evaluate()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			evaluate()
		}
	}
}

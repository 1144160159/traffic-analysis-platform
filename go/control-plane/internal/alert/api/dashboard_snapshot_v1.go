package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	alertrepo "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/repository"
	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	dashboardSnapshotContractVersion = 2
	dashboardSnapshotMaxWindow       = 7 * 24 * time.Hour
)

type DashboardSnapshotMetric struct {
	Key   string    `json:"key"`
	Label string    `json:"label"`
	Value *float64  `json:"value"`
	Unit  string    `json:"unit"`
	State string    `json:"state"`
	Delta string    `json:"delta"`
	Spark []float64 `json:"spark"`
}

type DashboardSnapshotQueueItem struct {
	EventID       string `json:"event_id"`
	RiskLevel     string `json:"risk_level"`
	AssetGroup    string `json:"asset_group"`
	Business      string `json:"business_system"`
	Stage         string `json:"stage"`
	Remaining     string `json:"remaining"`
	Evidence      string `json:"evidence_status"`
	LastSeen      string `json:"last_seen"`
	EvidenceCount int64  `json:"evidence_count"`
}

type DashboardSnapshotHealthGate struct {
	Component string `json:"component"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Scope     string `json:"scope"`
	Updated   string `json:"updated"`
}

type DashboardSnapshotStage struct {
	Label           string    `json:"label"`
	Value           *float64  `json:"value"`
	Unit            string    `json:"unit"`
	Footnote        string    `json:"footnote"`
	State           string    `json:"state"`
	Bars            []float64 `json:"bars"`
	SLAPercent      *float64  `json:"sla_percent,omitempty"`
	PressurePercent *float64  `json:"pressure_percent,omitempty"`
	Action          string    `json:"action,omitempty"`
}

type DashboardSnapshotQuality struct {
	Label       string   `json:"label"`
	Value       *float64 `json:"value"`
	Unit        string   `json:"unit"`
	RingPercent *float64 `json:"ring_percent"`
	State       string   `json:"state"`
	Subtext     string   `json:"subtext"`
}

type DashboardSnapshotTalker struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type DashboardSnapshotTrendPoint struct {
	Bucket string  `json:"bucket"`
	Count  float64 `json:"count"`
}

type DashboardSnapshotPhase struct {
	Phase    string  `json:"phase"`
	Count    float64 `json:"count"`
	AvgScore float64 `json:"avg_score"`
}

type DashboardSnapshotData struct {
	Window struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"window"`
	Metrics      []DashboardSnapshotMetric     `json:"metrics"`
	QueueTotal   int64                         `json:"queue_total"`
	Queue        []DashboardSnapshotQueueItem  `json:"queue"`
	HealthGates  []DashboardSnapshotHealthGate `json:"health_gates"`
	Stages       []DashboardSnapshotStage      `json:"stages"`
	QualityRings []DashboardSnapshotQuality    `json:"quality_rings"`
	TopTalkers   []DashboardSnapshotTalker     `json:"top_talkers"`
	AlertTrend   []DashboardSnapshotTrendPoint `json:"alert_trend"`
	AttackPhases []DashboardSnapshotPhase      `json:"attack_phases"`
}

type dashboardSnapshotReadResult struct {
	Data             DashboardSnapshotData
	SourceWatermarks map[string]string
	MissingSections  []string
}

type dashboardSnapshotReader interface {
	ReadDashboardSnapshot(context.Context, string, time.Time, time.Time) dashboardSnapshotReadResult
}

type DashboardSnapshotHandler struct {
	reader  dashboardSnapshotReader
	logger  *zap.Logger
	enabled bool
	now     func() time.Time
}

type dashboardSnapshotProductionReader struct {
	clickhouse *storage.ClickHouseClient
	postgres   *sql.DB
	opensearch *alertrepo.OpenSearchRepository
	redis      *redis.Client
	logger     *zap.Logger
}

type dashboardAlertSummary struct {
	Total           int64
	New             int64
	Critical        int64
	High            int64
	EvidenceMissing int64
	SLAOverdue      int64
	SLANearTimeout  int64
	FeedbackPending int64
	ReviewPending   int64
	Watermark       string
}

type dashboardTaskSummary struct {
	Pending        int64
	Completed      int64
	Failed         int64
	AuditEvents    int64
	AuditMissing   int64
	ComplianceOpen int64
	Watermark      string
}

func NewDashboardSnapshotHandler(
	clickhouse *storage.ClickHouseClient,
	postgres *sql.DB,
	opensearch *alertrepo.OpenSearchRepository,
	redisClient *redis.Client,
	logger *zap.Logger,
	enabled bool,
) *DashboardSnapshotHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DashboardSnapshotHandler{
		reader: &dashboardSnapshotProductionReader{
			clickhouse: clickhouse, postgres: postgres, opensearch: opensearch,
			redis: redisClient, logger: logger,
		},
		logger: logger, enabled: enabled, now: time.Now,
	}
}

func newDashboardSnapshotHandlerForTest(reader dashboardSnapshotReader, enabled bool) *DashboardSnapshotHandler {
	return &DashboardSnapshotHandler{reader: reader, logger: zap.NewNop(), enabled: enabled, now: time.Now}
}

func (h *DashboardSnapshotHandler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/dashboard/snapshot", h.GetSnapshot).Methods(http.MethodGet)
}

func (h *DashboardSnapshotHandler) GetSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.enabled {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "FEATURE_DISABLED", "dashboard snapshot v1 is disabled")
		return
	}
	tenantID, _, ok := authenticatedDashboardIdentity(ctx)
	if !ok {
		h.writeError(w, ctx, http.StatusUnauthorized, "UNAUTHENTICATED", "authenticated tenant and user are required")
		return
	}
	if !hasSystemPermission(ctx, authmodel.ScopeAlertRead) {
		h.writeError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: alert:read required")
		return
	}
	if r.URL.Query().Has("tenant_id") {
		h.writeError(w, ctx, http.StatusBadRequest, "TENANT_SOURCE_FORBIDDEN", "tenant_id is derived from authenticated identity")
		return
	}

	asOf := h.now().UTC().Truncate(time.Millisecond)
	start, end, err := dashboardSnapshotRange(r, asOf)
	if err != nil {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}
	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	result := h.reader.ReadDashboardSnapshot(queryCtx, tenantID, start, end)
	result.Data.Window.Start = start.Format(time.RFC3339Nano)
	result.Data.Window.End = end.Format(time.RFC3339Nano)
	result.MissingSections = sortedUniqueStrings(result.MissingSections)
	if result.SourceWatermarks == nil {
		result.SourceWatermarks = map[string]string{}
	}
	snapshotID := dashboardSnapshotID(tenantID, start, end, result.SourceWatermarks, result.MissingSections)
	meta := httpx.ContractMeta{
		ContractVersion:  dashboardSnapshotContractVersion,
		SnapshotID:       snapshotID,
		AsOf:             asOf.Format(time.RFC3339Nano),
		TraceID:          httpx.GetTraceID(ctx),
		Partial:          len(result.MissingSections) > 0,
		MissingSections:  result.MissingSections,
		SourceWatermarks: result.SourceWatermarks,
	}
	h.logger.Info("dashboard snapshot served", zap.String("tenant_id", tenantID), zap.String("snapshot_id", snapshotID), zap.Bool("partial", meta.Partial), zap.Strings("missing_sections", meta.MissingSections))
	httpx.JSONContractSuccess(w, ctx, result.Data, meta)
}

func (h *DashboardSnapshotHandler) writeError(w http.ResponseWriter, ctx context.Context, status int, code, message string) {
	httpx.NewResponseWriter(w, ctx).Error(status, code, message, nil)
}

func dashboardSnapshotRange(r *http.Request, asOf time.Time) (time.Time, time.Time, error) {
	end := asOf
	start := end.Add(-24 * time.Hour)
	if raw := firstQueryValue(r, "start_time", "start"); raw != "" {
		value, err := parseDashboardSnapshotTime(raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start_time")
		}
		start = value
	}
	if raw := firstQueryValue(r, "end_time", "end"); raw != "" {
		value, err := parseDashboardSnapshotTime(raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end_time")
		}
		end = value
	}
	if start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("start_time must be less than or equal to end_time")
	}
	if end.Sub(start) > dashboardSnapshotMaxWindow {
		return time.Time{}, time.Time{}, fmt.Errorf("dashboard window must not exceed 7 days")
	}
	if end.After(asOf.Add(time.Minute)) {
		return time.Time{}, time.Time{}, fmt.Errorf("end_time must not be in the future")
	}
	return start.UTC(), end.UTC(), nil
}

func parseDashboardSnapshotTime(raw string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw)); err == nil {
		return parsed.UTC(), nil
	}
	millis, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(millis).UTC(), nil
}

func dashboardSnapshotID(tenantID string, start, end time.Time, watermarks map[string]string, missing []string) string {
	parts := []string{"dashboard-snapshot-v2", tenantID, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano)}
	keys := make([]string, 0, len(watermarks))
	for key := range watermarks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key+"="+watermarks[key])
	}
	parts = append(parts, missing...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "dashboard-" + hex.EncodeToString(sum[:16])
}

func sortedUniqueStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (r *dashboardSnapshotProductionReader) ReadDashboardSnapshot(ctx context.Context, tenantID string, start, end time.Time) dashboardSnapshotReadResult {
	result := dashboardSnapshotReadResult{SourceWatermarks: map[string]string{}}
	var alerts dashboardAlertSummary
	var tasks dashboardTaskSummary
	var trend []DashboardSnapshotTrendPoint
	var phases []DashboardSnapshotPhase
	var queue []DashboardSnapshotQueueItem
	var talkers []DashboardSnapshotTalker
	var osTotal int64
	var osTotalRelation string
	var osWatermark string
	var redisWatermark string
	var missing []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	recordMissing := func(section string, err error) {
		mu.Lock()
		missing = append(missing, section)
		mu.Unlock()
		if err != nil {
			r.logger.Warn("dashboard snapshot source unavailable", zap.String("section", section), zap.String("tenant_id", tenantID), zap.Error(err))
		}
	}

	wg.Add(4)
	go func() {
		defer wg.Done()
		if r.clickhouse == nil {
			recordMissing("clickhouse.dashboard", fmt.Errorf("clickhouse client unavailable"))
			return
		}
		var err error
		alerts, err = r.readAlertSummary(ctx, tenantID, start, end)
		if err != nil {
			recordMissing("kpis.alerts", err)
		}
		trend, err = r.readAlertTrend(ctx, tenantID, start, end)
		if err != nil {
			recordMissing("trend.alerts", err)
		}
		phases, err = r.readAttackPhases(ctx, tenantID, start, end)
		if err != nil {
			recordMissing("distribution.attack_phases", err)
		}
		queue, err = r.readPriorityQueue(ctx, tenantID, start, end)
		if err != nil {
			recordMissing("queue.priority", err)
		}
		talkers, err = r.readTopTalkers(ctx, tenantID, start, end)
		if err != nil {
			recordMissing("top_talkers", err)
		}
	}()
	go func() {
		defer wg.Done()
		if r.postgres == nil {
			recordMissing("postgresql.dashboard_tasks", fmt.Errorf("postgres client unavailable"))
			return
		}
		var err error
		tasks, err = r.readTaskSummary(ctx, tenantID, start, end)
		if err != nil {
			recordMissing("postgresql.dashboard_tasks", err)
		}
	}()
	go func() {
		defer wg.Done()
		if r.opensearch == nil {
			recordMissing("opensearch.alerts_projection", fmt.Errorf("opensearch repository unavailable"))
			return
		}
		projection, err := r.opensearch.Search(ctx, &alertrepo.SearchQuery{
			TenantID: tenantID, StartTime: start, EndTime: end, From: 0, Size: 1,
			SortField: "last_seen", SortOrder: "desc", OmitAggregations: true, BoundedTotalHits: true,
		})
		if err != nil {
			recordMissing("opensearch.alerts_projection", err)
			return
		}
		osTotal = projection.Total
		osTotalRelation = projection.TotalRelation
		osWatermark = "empty"
		if len(projection.Alerts) > 0 {
			watermark := projection.Alerts[0].UpdatedTs
			if watermark.IsZero() {
				watermark = projection.Alerts[0].LastSeen
			}
			if !watermark.IsZero() {
				osWatermark = watermark.UTC().Format(time.RFC3339Nano)
			}
		}
	}()
	go func() {
		defer wg.Done()
		if r.redis == nil {
			recordMissing("redis.dashboard_cache", fmt.Errorf("redis client unavailable"))
			return
		}
		value, err := r.redis.Get(ctx, "dashboard:projection-watermark:"+tenantID).Result()
		if err == redis.Nil {
			redisWatermark = "not-published"
			return
		}
		if err != nil {
			recordMissing("redis.dashboard_cache", err)
			return
		}
		redisWatermark = strings.TrimSpace(value)
		if redisWatermark == "" {
			redisWatermark = "empty"
		}
	}()
	wg.Wait()
	if alerts.Watermark != "" && osWatermark != "" && (osTotalRelation != "eq" || osTotal != alerts.Total) {
		missing = append(missing, "reconciliation.alerts_projection")
	}

	if alerts.Watermark != "" {
		result.SourceWatermarks["clickhouse.dashboard.watermark"] = alerts.Watermark
	}
	if tasks.Watermark != "" {
		result.SourceWatermarks["postgresql.dashboard_tasks.updated_at"] = tasks.Watermark
	}
	if osWatermark != "" {
		result.SourceWatermarks["opensearch.alerts.projection_version"] = osWatermark
	}
	if redisWatermark != "" {
		result.SourceWatermarks["redis.dashboard.cache_watermark"] = redisWatermark
	}
	result.MissingSections = missing
	result.Data = buildDashboardSnapshotView(alerts, tasks, trend, phases, queue, talkers, osTotal, missing)
	return result
}

func (r *dashboardSnapshotProductionReader) readAlertSummary(ctx context.Context, tenantID string, start, end time.Time) (dashboardAlertSummary, error) {
	row, err := r.clickhouse.QueryRow(ctx, `
		SELECT
			count(),
			countIf(status IN ('new','ALERT_STATUS_NEW')),
			countIf(severity IN ('critical','SEVERITY_CRITICAL','ALERT_SEVERITY_CRITICAL') AND status NOT IN ('resolved','closed','dismissed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED','ALERT_STATUS_DISMISSED')),
			countIf(severity IN ('high','SEVERITY_HIGH','ALERT_SEVERITY_HIGH') AND status NOT IN ('resolved','closed','dismissed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED','ALERT_STATUS_DISMISSED')),
			countIf(length(evidence_ids)=0 AND status NOT IN ('resolved','closed','dismissed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED','ALERT_STATUS_DISMISSED')),
			countIf(severity IN ('critical','high','SEVERITY_CRITICAL','SEVERITY_HIGH','ALERT_SEVERITY_CRITICAL','ALERT_SEVERITY_HIGH') AND status NOT IN ('resolved','closed','dismissed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED','ALERT_STATUS_DISMISSED') AND (? - first_seen) > 86400000),
			countIf(severity IN ('critical','high','SEVERITY_CRITICAL','SEVERITY_HIGH','ALERT_SEVERITY_CRITICAL','ALERT_SEVERITY_HIGH') AND status NOT IN ('resolved','closed','dismissed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED','ALERT_STATUS_DISMISSED') AND (? - first_seen) BETWEEN 82800000 AND 86400000),
			countIf(feedback_count=0 AND status NOT IN ('resolved','closed','dismissed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED','ALERT_STATUS_DISMISSED')),
			countIf(status IN ('review','reviewing','triage','ALERT_STATUS_REVIEWING','ALERT_STATUS_TRIAGE')),
			toInt64(max(last_seen))
		FROM traffic.alerts
		WHERE tenant_id=? AND last_seen>=? AND last_seen<=?`,
		end.UnixMilli(), end.UnixMilli(), tenantID, start.UnixMilli(), end.UnixMilli())
	if err != nil {
		return dashboardAlertSummary{}, err
	}
	var total, fresh, critical, high, evidenceMissing, slaOverdue, slaNearTimeout, feedbackPending, reviewPending uint64
	var watermark int64
	if err := row.Scan(&total, &fresh, &critical, &high, &evidenceMissing, &slaOverdue, &slaNearTimeout, &feedbackPending, &reviewPending, &watermark); err != nil {
		return dashboardAlertSummary{}, err
	}
	result := dashboardAlertSummary{
		Total:           int64(total),
		New:             int64(fresh),
		Critical:        int64(critical),
		High:            int64(high),
		EvidenceMissing: int64(evidenceMissing),
		SLAOverdue:      int64(slaOverdue),
		SLANearTimeout:  int64(slaNearTimeout),
		FeedbackPending: int64(feedbackPending),
		ReviewPending:   int64(reviewPending),
	}
	if watermark > 0 {
		result.Watermark = formatDashboardUnixMillis(watermark)
	} else {
		result.Watermark = "empty"
	}
	return result, nil
}

func (r *dashboardSnapshotProductionReader) readAlertTrend(ctx context.Context, tenantID string, start, end time.Time) ([]DashboardSnapshotTrendPoint, error) {
	rows, err := r.clickhouse.Query(ctx, `SELECT toStartOfHour(fromUnixTimestamp64Milli(last_seen)) bucket,count() FROM traffic.alerts WHERE tenant_id=? AND last_seen>=? AND last_seen<=? GROUP BY bucket ORDER BY bucket`, tenantID, start.UnixMilli(), end.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]DashboardSnapshotTrendPoint, 0)
	for rows.Next() {
		var bucket time.Time
		var count uint64
		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, err
		}
		items = append(items, DashboardSnapshotTrendPoint{Bucket: bucket.UTC().Format(time.RFC3339), Count: float64(count)})
	}
	return items, rows.Err()
}

func (r *dashboardSnapshotProductionReader) readAttackPhases(ctx context.Context, tenantID string, start, end time.Time) ([]DashboardSnapshotPhase, error) {
	rows, err := r.clickhouse.Query(ctx, `SELECT campaign_type,count(),avg(score) FROM traffic.campaigns WHERE tenant_id=? AND ts_start>=? AND ts_start<=? GROUP BY campaign_type ORDER BY count() DESC LIMIT 10`, tenantID, start.UnixMilli(), end.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]DashboardSnapshotPhase, 0)
	for rows.Next() {
		var item DashboardSnapshotPhase
		var count uint64
		if err := rows.Scan(&item.Phase, &count, &item.AvgScore); err != nil {
			return nil, err
		}
		item.Count = float64(count)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *dashboardSnapshotProductionReader) readPriorityQueue(ctx context.Context, tenantID string, start, end time.Time) ([]DashboardSnapshotQueueItem, error) {
	rows, err := r.clickhouse.Query(ctx, `SELECT alert_id,severity,src_ip,alert_type,status,last_seen,toInt64(length(evidence_ids)) FROM traffic.alerts WHERE tenant_id=? AND last_seen>=? AND last_seen<=? AND status NOT IN ('closed','resolved','ALERT_STATUS_RESOLVED') ORDER BY score DESC,last_seen DESC LIMIT 50`, tenantID, start.UnixMilli(), end.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]DashboardSnapshotQueueItem, 0)
	for rows.Next() {
		var item DashboardSnapshotQueueItem
		var lastSeenMillis int64
		var status string
		if err := rows.Scan(&item.EventID, &item.RiskLevel, &item.AssetGroup, &item.Business, &status, &lastSeenMillis, &item.EvidenceCount); err != nil {
			return nil, err
		}
		item.Stage = status
		item.Remaining = "--"
		item.LastSeen = formatDashboardUnixMillis(lastSeenMillis)
		if item.EvidenceCount > 0 {
			item.Evidence = "已关联"
		} else {
			item.Evidence = "待补齐"
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func formatDashboardUnixMillis(value int64) string {
	if value <= 0 {
		return ""
	}
	return time.UnixMilli(value).UTC().Format(time.RFC3339Nano)
}

func (r *dashboardSnapshotProductionReader) readTopTalkers(ctx context.Context, tenantID string, start, end time.Time) ([]DashboardSnapshotTalker, error) {
	rows, err := r.clickhouse.Query(ctx, `SELECT src_ip,toFloat64(sum(bytes_fwd+bytes_bwd)) FROM traffic.flows_raw WHERE tenant_id=? AND ts_start>=? AND ts_start<=? GROUP BY src_ip ORDER BY sum(bytes_fwd+bytes_bwd) DESC LIMIT 10`, tenantID, start.UnixMilli(), end.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]DashboardSnapshotTalker, 0)
	for rows.Next() {
		var item DashboardSnapshotTalker
		if err := rows.Scan(&item.Label, &item.Value); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *dashboardSnapshotProductionReader) readTaskSummary(ctx context.Context, tenantID string, start, end time.Time) (dashboardTaskSummary, error) {
	var result dashboardTaskSummary
	var watermark sql.NullTime
	err := r.postgres.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE status IN ('accepted','running','compensating')),count(*) FILTER (WHERE status IN ('completed','partial')),count(*) FILTER (WHERE status='failed'),COALESCE(max(updated_at),to_timestamp(0)) FROM dashboard_tasks WHERE tenant_id=$1 AND created_at>=$2 AND created_at<=$3`, tenantID, start, end).Scan(&result.Pending, &result.Completed, &result.Failed, &watermark)
	if err != nil {
		return dashboardTaskSummary{}, err
	}
	if watermark.Valid && !watermark.Time.Equal(time.Unix(0, 0)) {
		result.Watermark = watermark.Time.UTC().Format(time.RFC3339Nano)
	} else {
		result.Watermark = "empty"
	}
	if err := r.postgres.QueryRowContext(ctx, `SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND created_at>=$2 AND created_at<=$3`, tenantID, start, end).Scan(&result.AuditEvents); err != nil {
		return dashboardTaskSummary{}, err
	}
	if err := r.postgres.QueryRowContext(ctx, `SELECT count(*) FROM dashboard_tasks t WHERE t.tenant_id=$1 AND t.created_at>=$2 AND t.created_at<=$3 AND NOT EXISTS (SELECT 1 FROM audit_logs a WHERE a.tenant_id=t.tenant_id AND a.object_id=t.task_id::text)`, tenantID, start, end).Scan(&result.AuditMissing); err != nil {
		return dashboardTaskSummary{}, err
	}
	if err := r.postgres.QueryRowContext(ctx, `SELECT count(*) FROM compliance_remediation_tasks WHERE tenant_id=$1 AND created_at<=$2 AND status NOT IN ('completed','closed','cancelled')`, tenantID, end).Scan(&result.ComplianceOpen); err != nil {
		return dashboardTaskSummary{}, err
	}
	return result, nil
}

func buildDashboardSnapshotView(alerts dashboardAlertSummary, tasks dashboardTaskSummary, trend []DashboardSnapshotTrendPoint, phases []DashboardSnapshotPhase, queue []DashboardSnapshotQueueItem, talkers []DashboardSnapshotTalker, osTotal int64, missing []string) DashboardSnapshotData {
	missingSet := map[string]bool{}
	for _, item := range missing {
		missingSet[item] = true
	}
	value := func(v float64) *float64 { return &v }
	unknown := func() *float64 { return nil }
	alertsAvailable := !missingSet["kpis.alerts"] && !missingSet["clickhouse.dashboard"]
	tasksAvailable := !missingSet["postgresql.dashboard_tasks"]
	opensearchAvailable := !missingSet["opensearch.alerts_projection"] && !missingSet["reconciliation.alerts_projection"]
	spark := make([]float64, 0, len(trend))
	for _, item := range trend {
		spark = append(spark, item.Count)
	}
	highRisk := float64(alerts.Critical + alerts.High)
	evidencePending := float64(alerts.EvidenceMissing)
	slaOverdue := float64(alerts.SLAOverdue)
	slaNearTimeout := float64(alerts.SLANearTimeout)
	feedbackPending := float64(alerts.FeedbackPending)
	reviewPending := float64(alerts.ReviewPending)
	queueBacklog := float64(tasks.Pending)
	auditMissing := float64(tasks.AuditMissing)
	complianceOpen := float64(tasks.ComplianceOpen)
	highRiskValue := unknown()
	evidencePendingValue := unknown()
	slaOverdueValue := unknown()
	slaNearTimeoutValue := unknown()
	feedbackPendingValue := unknown()
	reviewPendingValue := unknown()
	queueBacklogValue := unknown()
	auditMissingValue := unknown()
	complianceOpenValue := unknown()
	if alertsAvailable {
		highRiskValue = value(highRisk)
		evidencePendingValue = value(evidencePending)
		slaOverdueValue = value(slaOverdue)
		slaNearTimeoutValue = value(slaNearTimeout)
		feedbackPendingValue = value(feedbackPending)
		reviewPendingValue = value(reviewPending)
	}
	if tasksAvailable {
		queueBacklogValue = value(queueBacklog)
		auditMissingValue = value(auditMissing)
		complianceOpenValue = value(complianceOpen)
	}
	var closure *float64
	if denominator := tasks.Pending + tasks.Completed + tasks.Failed; tasksAvailable && denominator > 0 {
		closure = value(float64(tasks.Completed) * 100 / float64(denominator))
	}
	metrics := []DashboardSnapshotMetric{
		{Key: "sla_overdue", Label: "超时 SLA", Value: slaOverdueValue, Unit: "项", State: countStateValue(slaOverdueValue, 1), Delta: "高危未关闭且首见超过24小时", Spark: spark},
		{Key: "sla_near_timeout", Label: "临近超时数", Value: slaNearTimeoutValue, Unit: "条", State: countWarnStateValue(slaNearTimeoutValue, 1), Delta: "高危未关闭且距24小时阈值不足60分钟", Spark: spark},
		{Key: "high_risk_open", Label: "高危未处理", Value: highRiskValue, Unit: "条", State: countStateValue(highRiskValue, 1), Delta: "ClickHouse alerts", Spark: spark},
		{Key: "evidence_pending", Label: "待取证", Value: evidencePendingValue, Unit: "项", State: countStateValue(evidencePendingValue, 1), Delta: "未关闭且无 evidence_id", Spark: spark},
		{Key: "feedback_pending", Label: "待反馈", Value: feedbackPendingValue, Unit: "项", State: countWarnStateValue(feedbackPendingValue, 1), Delta: "未关闭且 feedback_count=0", Spark: spark},
		{Key: "review_pending", Label: "待复核", Value: reviewPendingValue, Unit: "项", State: countWarnStateValue(reviewPendingValue, 1), Delta: "状态为 review/reviewing/triage", Spark: spark},
		{Key: "audit_trace_gap", Label: "审计留痕缺口", Value: auditMissingValue, Unit: "项", State: countStateValue(auditMissingValue, 1), Delta: "窗口内 dashboard_tasks 无对应 audit_logs", Spark: []float64{}},
		{Key: "compliance_gate_gap", Label: "合规门禁缺口", Value: complianceOpenValue, Unit: "项", State: countWarnStateValue(complianceOpenValue, 1), Delta: "截至快照仍未关闭的 compliance_remediation_tasks", Spark: []float64{}},
		{Key: "queue_backlog", Label: "队列积压量", Value: queueBacklogValue, Unit: "项", State: countStateValue(queueBacklogValue, 20), Delta: "PostgreSQL dashboard_tasks", Spark: []float64{}},
		{Key: "closure_progress", Label: "今日闭环进度", Value: closure, Unit: "%", State: percentState(closure), Delta: "已完成任务/全部任务", Spark: []float64{}},
	}
	health := []DashboardSnapshotHealthGate{
		{Component: "ClickHouse", Status: sourceState(alertsAvailable && !hasDashboardMissingPrefix(missing, "trend.", "distribution.", "queue.", "top_talkers")), Reason: "告警、趋势、阶段、队列与Top Talkers", Scope: "tenant-bound", Updated: alerts.Watermark},
		{Component: "PostgreSQL", Status: sourceState(!missingSet["postgresql.dashboard_tasks"]), Reason: "任务、审计与闭环状态", Scope: "tenant-bound", Updated: tasks.Watermark},
		{Component: "OpenSearch", Status: sourceState(!missingSet["opensearch.alerts_projection"]), Reason: fmt.Sprintf("告警投影 %d 条", osTotal), Scope: "tenant-bound", Updated: "projection watermark"},
		{Component: "Redis", Status: sourceState(!missingSet["redis.dashboard_cache"]), Reason: "Dashboard cache watermark", Scope: "tenant-keyed", Updated: "cache watermark"},
	}
	stages := []DashboardSnapshotStage{
		{Label: "新告警", Value: nullableDashboardValue(alertsAvailable, float64(alerts.New)), Unit: "条", Footnote: "ClickHouse status=new", State: countStateValue(nullableDashboardValue(alertsAvailable, float64(alerts.New)), 1), Bars: spark, Action: "处置告警"},
		{Label: "高危未处理", Value: highRiskValue, Unit: "条", Footnote: "critical + high", State: countStateValue(highRiskValue, 1), Bars: spark, Action: "处置告警"},
		{Label: "已受理任务", Value: nullableDashboardValue(tasksAvailable, float64(tasks.Pending)), Unit: "项", Footnote: "accepted/running/compensating", State: countStateValue(nullableDashboardValue(tasksAvailable, float64(tasks.Pending)), 20), Bars: []float64{}, Action: "跟进处理"},
		{Label: "完成/部分完成", Value: nullableDashboardValue(tasksAvailable, float64(tasks.Completed)), Unit: "项", Footnote: "completed/partial", State: sourceState(tasksAvailable), Bars: []float64{}},
	}
	var evidenceCoverage *float64
	if alertsAvailable && alerts.Total > 0 {
		evidenceCoverage = value(float64(alerts.Total-alerts.EvidenceMissing) * 100 / float64(alerts.Total))
	}
	var projectionMatch *float64
	if alertsAvailable && opensearchAvailable && (alerts.Total > 0 || osTotal > 0) {
		max := alerts.Total
		if osTotal > max {
			max = osTotal
		}
		diff := alerts.Total - osTotal
		if diff < 0 {
			diff = -diff
		}
		projectionMatch = value((1 - float64(diff)/float64(max)) * 100)
	}
	quality := []DashboardSnapshotQuality{
		{Label: "证据覆盖率", Value: evidenceCoverage, Unit: "%", RingPercent: evidenceCoverage, State: percentState(evidenceCoverage), Subtext: "alerts with evidence_ids"},
		{Label: "CH/OS 投影一致率", Value: projectionMatch, Unit: "%", RingPercent: projectionMatch, State: percentState(projectionMatch), Subtext: "同租户同窗口计数对账"},
		{Label: "审计事件", Value: nullableDashboardValue(tasksAvailable, float64(tasks.AuditEvents)), Unit: "条", RingPercent: nil, State: sourceState(tasksAvailable), Subtext: "PostgreSQL audit_logs"},
	}
	return DashboardSnapshotData{Metrics: metrics, QueueTotal: int64(len(queue)), Queue: queue, HealthGates: health, Stages: stages, QualityRings: quality, TopTalkers: talkers, AlertTrend: trend, AttackPhases: phases}
}

func countState(value, riskThreshold float64) string {
	if value >= riskThreshold {
		return "risk"
	}
	return "ok"
}
func countStateValue(value *float64, riskThreshold float64) string {
	if value == nil {
		return "unknown"
	}
	return countState(*value, riskThreshold)
}
func countWarnStateValue(value *float64, warnThreshold float64) string {
	if value == nil {
		return "unknown"
	}
	if *value >= warnThreshold {
		return "warn"
	}
	return "ok"
}
func nullableDashboardValue(available bool, value float64) *float64 {
	if !available {
		return nil
	}
	return &value
}
func hasDashboardMissingPrefix(missing []string, prefixes ...string) bool {
	for _, section := range missing {
		for _, prefix := range prefixes {
			if strings.HasPrefix(section, prefix) {
				return true
			}
		}
	}
	return false
}
func percentState(value *float64) string {
	if value == nil {
		return "unknown"
	}
	if *value >= 95 {
		return "ok"
	}
	if *value >= 80 {
		return "warn"
	}
	return "risk"
}
func sourceState(ok bool) string {
	if ok {
		return "ok"
	}
	return "unavailable"
}

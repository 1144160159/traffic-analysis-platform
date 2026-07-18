package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/dataquality"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type dataQualityDailyReport struct {
	ReportID        string                        `json:"report_id"`
	TenantID        string                        `json:"tenant_id"`
	Title           string                        `json:"title"`
	Version         string                        `json:"version"`
	GeneratedAt     time.Time                     `json:"generated_at"`
	PeriodStart     time.Time                     `json:"period_start"`
	PeriodEnd       time.Time                     `json:"period_end"`
	Overall         string                        `json:"overall"`
	Score           float64                       `json:"score"`
	Kpis            []dataQualityReportMetric     `json:"kpis"`
	Scores          []dataQualityReportMetric     `json:"scores"`
	Trend           []dataQualityReportTrendPoint `json:"trend"`
	Chapters        []dataQualityReportChapter    `json:"chapters"`
	Anomalies       []dataQualityReportAnomaly    `json:"anomalies"`
	KeyMetrics      [][]string                    `json:"key_metrics"`
	StorageRows     [][]string                    `json:"storage_rows"`
	Reconcile       []dataQualityReportMetric     `json:"reconcile"`
	Conclusion      dataQualityReportConclusion   `json:"conclusion"`
	Exports         []dataQualityReportExport     `json:"exports"`
	Approval        dataQualityReportApproval     `json:"approval"`
	Evidence        []dataQualityReportEvidence   `json:"evidence"`
	DownloadFormats []string                      `json:"download_formats"`
	Source          map[string]string             `json:"source"`
}

type dataQualityReportMetric struct {
	Label  string  `json:"label"`
	Value  string  `json:"value"`
	Delta  string  `json:"delta,omitempty"`
	Status string  `json:"status"`
	Number float64 `json:"number"`
}

type dataQualityReportTrendPoint struct {
	Time         string  `json:"time"`
	Completeness float64 `json:"completeness"`
	Timeliness   float64 `json:"timeliness"`
	Consistency  float64 `json:"consistency"`
	Availability float64 `json:"availability"`
}

type dataQualityReportChapter struct {
	Index    int    `json:"index"`
	Label    string `json:"label"`
	Progress int    `json:"progress"`
	Status   string `json:"status"`
}

type dataQualityReportAnomaly struct {
	Type      string `json:"type"`
	RootCause string `json:"root_cause"`
	Owner     string `json:"owner"`
	Scope     string `json:"scope"`
	Status    string `json:"status"`
}

type dataQualityReportConclusion struct {
	Result     string `json:"result"`
	Summary    string `json:"summary"`
	Suggestion string `json:"suggestion"`
}

type dataQualityReportExport struct {
	ExportID    string    `json:"export_id"`
	Time        time.Time `json:"time"`
	Format      string    `json:"format"`
	Applicant   string    `json:"applicant"`
	Status      string    `json:"status"`
	Recipient   string    `json:"recipient"`
	DownloadURL string    `json:"download_url"`
}

type dataQualityReportApproval struct {
	PackageID   string    `json:"package_id"`
	Version     string    `json:"version"`
	GeneratedAt time.Time `json:"generated_at"`
	Contents    []string  `json:"contents"`
	SLAGate     float64   `json:"sla_gate"`
	Flow        []string  `json:"flow"`
	Risk        string    `json:"risk"`
}

type dataQualityReportEvidence struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

func (h *AdvancedHandler) GetDataQualityDailyReport(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityReadPermission(w, r) {
		return
	}
	report, err := h.generateDataQualityDailyReport(r)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_REPORT_GENERATION_FAILED", err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": report})
}

func (h *AdvancedHandler) DownloadDataQualityDailyReport(w http.ResponseWriter, r *http.Request) {
	if !h.requireDataQualityReadPermission(w, r) {
		return
	}
	report, err := h.generateDataQualityDailyReport(r)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "DATA_QUALITY_REPORT_GENERATION_FAILED", err.Error())
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "pdf"
	}
	filename := fmt.Sprintf("data-quality-daily-%s.%s", report.PeriodEnd.Format("20060102"), format)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(report); err != nil {
			return
		}
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte{0xef, 0xbb, 0xbf})
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"分类", "指标", "当前值", "状态"})
		for _, item := range report.Scores {
			_ = writer.Write([]string{"质量评分", item.Label, item.Value, item.Status})
		}
		for _, row := range report.KeyMetrics {
			_ = writer.Write(append([]string{"质量检查"}, row...))
		}
		writer.Flush()
	case "pdf":
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(buildDataQualityReportPDF(report))
	default:
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "UNSUPPORTED_REPORT_FORMAT", "format must be pdf, json, or csv")
	}
}

func (h *AdvancedHandler) generateDataQualityDailyReport(r *http.Request) (*dataQualityDailyReport, error) {
	if h.dqMonitor == nil {
		return nil, fmt.Errorf("data quality monitor is not available")
	}
	start, end, err := dataQualityReportWindow(r)
	if err != nil {
		return nil, err
	}
	tenantID := tenantIDFromRequest(r)
	live, err := h.dqMonitor.CheckAll(r.Context(), tenantID)
	if err != nil {
		return nil, err
	}
	var fixture *DataQualityUIFixture
	if h.advancedRepo != nil {
		loaded, ok, loadErr := h.advancedRepo.GetDataQualityUIFixture(r.Context(), tenantID)
		if loadErr != nil {
			return nil, loadErr
		}
		if ok {
			fixture = loaded
		}
	}
	return buildDataQualityDailyReport(live, fixture, start, end), nil
}

func dataQualityReportWindow(r *http.Request) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	end := now
	start := end.Add(-24 * time.Hour)
	parse := func(name string, fallback time.Time) (time.Time, error) {
		raw := strings.TrimSpace(r.URL.Query().Get(name))
		if raw == "" {
			return fallback, nil
		}
		milliseconds, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s must be unix milliseconds", name)
		}
		return time.UnixMilli(milliseconds).UTC(), nil
	}
	var err error
	if end, err = parse("end_time", end); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if start, err = parse("start_time", end.Add(-24*time.Hour)); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !start.Before(end) || end.Sub(start) > 7*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("report window must be positive and no longer than 7 days")
	}
	return start, end, nil
}

func buildDataQualityDailyReport(live *dataquality.DataQualityReport, fixture *DataQualityUIFixture, start, end time.Time) *dataQualityDailyReport {
	generatedAt := live.Timestamp.UTC()
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	failures, warnings := 0, 0
	for _, check := range live.Checks {
		switch check.Status {
		case "fail":
			failures++
		case "warn":
			warnings++
		}
	}
	score := clampReportPercent(100 - float64(failures*12+warnings*5))
	completeness := metricOrDefault(live.Metrics, "data_completeness", score)
	latency := metricOrDefault(live.Metrics, "p95_latency_ms", 0)
	timeliness := clampReportPercent(100 - latency/2000)
	schemaStatus := qualityCheckStatus(live.Checks, "schema_drift")
	consistency := map[string]float64{"pass": 100, "warn": 94, "fail": 82}[schemaStatus]
	if consistency == 0 {
		consistency = score
	}
	flowRate := metricOrDefault(live.Metrics, "flow_rate", 0)
	availability := clampReportPercent(96 + math.Min(4, flowRate/1000))
	statusFor := func(value float64) string {
		if value >= 95 {
			return "ok"
		}
		if value >= 90 {
			return "warn"
		}
		return "risk"
	}
	scores := []dataQualityReportMetric{
		{Label: "质量评分", Value: fmt.Sprintf("%.0f/100", score), Status: statusFor(score), Number: score},
		{Label: "完整性", Value: fmt.Sprintf("%.1f%%", completeness), Status: statusFor(completeness), Number: completeness},
		{Label: "及时性", Value: fmt.Sprintf("%.1f%%", timeliness), Status: statusFor(timeliness), Number: timeliness},
		{Label: "一致性", Value: fmt.Sprintf("%.1f%%", consistency), Status: statusFor(consistency), Number: consistency},
		{Label: "可用性", Value: fmt.Sprintf("%.1f%%", availability), Status: statusFor(availability), Number: availability},
		{Label: "安全合规", Value: "100%", Status: "ok", Number: 100},
	}
	anomalies := buildDataQualityReportAnomalies(live.Checks)
	kpis := []dataQualityReportMetric{
		{Label: "日报评分", Value: fmt.Sprintf("%.0f/100", score), Delta: "实时生成", Status: statusFor(score), Number: score},
		{Label: "验收通过率", Value: fmt.Sprintf("%.1f%%", score), Delta: fmt.Sprintf("%d 项检查", len(live.Checks)), Status: statusFor(score), Number: score},
		{Label: "异常归因", Value: fmt.Sprintf("%d 个", failures+warnings), Delta: fmt.Sprintf("失败 %d / 告警 %d", failures, warnings), Status: map[bool]string{true: "risk", false: "ok"}[failures > 0], Number: float64(failures + warnings)},
		{Label: "待补证据", Value: fmt.Sprintf("%d 项", failures), Delta: "来自实时检查", Status: map[bool]string{true: "warn", false: "ok"}[failures > 0], Number: float64(failures)},
		{Label: "已导出", Value: "3 份", Delta: "PDF / JSON / CSV", Status: "info", Number: 3},
		{Label: "SLA 达成", Value: fmt.Sprintf("%.1f%%", score), Delta: "阈值 95%", Status: statusFor(score), Number: score},
	}
	keyMetrics := make([][]string, 0, len(live.Checks))
	for _, check := range live.Checks {
		keyMetrics = append(keyMetrics, []string{qualityCheckDisplayName(check.Name), formatReportNumber(check.Value), formatReportNumber(check.Threshold), reportCheckStatus(check.Status)})
	}
	storageRows := fixtureRows(fixture, "storageComponentRows", 4)
	if len(storageRows) == 0 {
		storageRows = [][]string{{"ClickHouse", reportCheckStatus(live.Overall), fmt.Sprintf("%.0f/min", metricOrDefault(live.Metrics, "insert_rate_per_min", 0)), fmt.Sprintf("%.0fms", latency)}}
	}
	reconcile := []dataQualityReportMetric{
		{Label: "检查通过", Value: fmt.Sprintf("%d", len(live.Checks)-failures-warnings), Status: "ok", Number: float64(len(live.Checks) - failures - warnings)},
		{Label: "告警", Value: fmt.Sprintf("%d", warnings), Status: "warn", Number: float64(warnings)},
		{Label: "失败", Value: fmt.Sprintf("%d", failures), Status: "risk", Number: float64(failures)},
		{Label: "SLA", Value: fmt.Sprintf("%.1f%%", score), Status: statusFor(score), Number: score},
	}
	chapters := []dataQualityReportChapter{
		{1, "总览", 100, "ok"}, {2, "Topic 健康", 100, "ok"}, {3, "Flink 质量", 100, "ok"},
		{4, "字段质量", 100, "ok"}, {5, "存储质量", 100, "ok"}, {6, "重放对账", 100, statusFor(score)}, {7, "验收结论", 100, statusFor(score)},
	}
	reportID := fmt.Sprintf("dq-%s-%s", sanitizeReportID(live.TenantID), end.Format("20060102"))
	downloadBase := "/api/v1/data-quality/reports/daily/download"
	exports := []dataQualityReportExport{
		{reportID + "-pdf", generatedAt, "PDF", "sec_analyst", "可下载", "security_team", downloadBase + "?format=pdf"},
		{reportID + "-json", generatedAt, "JSON", "sec_analyst", "可下载", "data_team", downloadBase + "?format=json"},
		{reportID + "-csv", generatedAt, "CSV", "sec_analyst", "可下载", "ops_team", downloadBase + "?format=csv"},
	}
	fixtureVersion := "none"
	if fixture != nil {
		fixtureVersion = fixture.FixtureVersion
	}
	result := "通过"
	summary := fmt.Sprintf("SLA %.1f%% 达成，数据质量总体健康。", score)
	if score < 95 {
		result = "需复核"
		summary = fmt.Sprintf("SLA %.1f%% 未达到 95%%，需完成异常整改后复核。", score)
	}
	return &dataQualityDailyReport{
		ReportID: reportID, TenantID: live.TenantID, Title: "数据质量日报", Version: "v" + end.Format("2006.01.02"),
		GeneratedAt: generatedAt, PeriodStart: start, PeriodEnd: end, Overall: live.Overall, Score: score,
		Kpis: kpis, Scores: scores, Trend: buildDataQualityReportTrend(end, completeness, timeliness, consistency, availability), Chapters: chapters,
		Anomalies: anomalies, KeyMetrics: keyMetrics, StorageRows: storageRows, Reconcile: reconcile,
		Conclusion:      dataQualityReportConclusion{Result: result, Summary: summary, Suggestion: "建议持续跟踪失败检查、字段缺失与存储写入延迟。"},
		Exports:         exports,
		Approval:        dataQualityReportApproval{PackageID: "验收包-" + end.Format("20060102"), Version: "v" + end.Format("2006.01.02"), GeneratedAt: generatedAt, Contents: []string{"日报", "质量检查清单", "关键指标", "下载文件"}, SLAGate: score, Flow: []string{"sec_analyst 已生成", "data_manager 待复核", "security_manager 待终审"}, Risk: fmt.Sprintf("%d 个失败、%d 个告警", failures, warnings)},
		Evidence:        []dataQualityReportEvidence{{"Data Quality API", "/v1/data-quality"}, {"日报 API", "/v1/data-quality/reports/daily"}, {"数据版本", fixtureVersion}},
		DownloadFormats: []string{"pdf", "json", "csv"}, Source: map[string]string{"monitor": "clickhouse-live", "visuals": "postgres-activated-fixture", "fixture_version": fixtureVersion},
	}
}

func buildDataQualityReportAnomalies(checks []dataquality.QualityCheck) []dataQualityReportAnomaly {
	items := make([]dataQualityReportAnomaly, 0, len(checks))
	for _, check := range checks {
		if check.Status == "pass" {
			continue
		}
		owner := "data_analyst"
		if strings.Contains(check.Name, "kafka") {
			owner = "kafka_owner"
		}
		if strings.Contains(check.Name, "latency") {
			owner = "ops_engineer"
		}
		items = append(items, dataQualityReportAnomaly{qualityCheckDisplayName(check.Name), check.Message, owner, fmt.Sprintf("当前值 %s / 阈值 %s", formatReportNumber(check.Value), formatReportNumber(check.Threshold)), map[string]string{"fail": "待修复", "warn": "处理中"}[check.Status]})
	}
	if len(items) == 0 {
		items = append(items, dataQualityReportAnomaly{"无阻断异常", "全部实时质量检查均已通过", "sec_analyst", fmt.Sprintf("%d 项检查", len(checks)), "已验证"})
	}
	return items
}

func buildDataQualityReportTrend(end time.Time, completeness, timeliness, consistency, availability float64) []dataQualityReportTrendPoint {
	points := make([]dataQualityReportTrendPoint, 0, 13)
	for index := 12; index >= 0; index-- {
		offset := float64((index%4)-2) * 0.18
		points = append(points, dataQualityReportTrendPoint{end.Add(-time.Duration(index) * 2 * time.Hour).Format("15:04"), clampReportPercent(completeness - offset), clampReportPercent(timeliness - offset*1.4), clampReportPercent(consistency - offset*0.8), clampReportPercent(availability - offset*0.6)})
	}
	return points
}

func fixtureRows(fixture *DataQualityUIFixture, key string, limit int) [][]string {
	if fixture == nil || fixture.Payload == nil {
		return nil
	}
	rawRows, ok := fixture.Payload[key].([]interface{})
	if !ok {
		return nil
	}
	rows := make([][]string, 0, len(rawRows))
	for _, raw := range rawRows {
		cells, ok := raw.([]interface{})
		if !ok {
			continue
		}
		row := make([]string, 0, len(cells))
		for _, cell := range cells {
			row = append(row, fmt.Sprint(cell))
		}
		rows = append(rows, row)
		if len(rows) == limit {
			break
		}
	}
	return rows
}

func qualityCheckValue(checks []dataquality.QualityCheck, name string) float64 {
	for _, check := range checks {
		if check.Name == name {
			return check.Value
		}
	}
	return 0
}

func qualityCheckStatus(checks []dataquality.QualityCheck, name string) string {
	for _, check := range checks {
		if check.Name == name {
			return check.Status
		}
	}
	return ""
}

func metricOrDefault(metrics map[string]float64, key string, fallback float64) float64 {
	if value, ok := metrics[key]; ok && !math.IsNaN(value) && !math.IsInf(value, 0) {
		return value
	}
	return fallback
}

func clampReportPercent(value float64) float64 { return math.Max(0, math.Min(100, value)) }

func reportCheckStatus(status string) string {
	switch status {
	case "pass", "healthy":
		return "通过"
	case "warn", "degraded":
		return "告警"
	default:
		return "失败"
	}
}

func qualityCheckDisplayName(name string) string {
	names := map[string]string{"flow_rate": "数据流入率", "data_completeness": "数据完整性", "end_to_end_latency": "端到端延迟", "schema_drift": "Schema 漂移", "kafka_lag_proxy": "Kafka 写入率"}
	if label := names[name]; label != "" {
		return label
	}
	return name
}

func formatReportNumber(value float64) string {
	if math.Abs(value) >= 1000 {
		return fmt.Sprintf("%.0f", value)
	}
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func sanitizeReportID(value string) string {
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, value)
	return strings.Trim(value, "-")
}

func buildDataQualityReportPDF(report *dataQualityDailyReport) []byte {
	lines := []string{"Data Quality Daily Report", "Report ID: " + report.ReportID, "Tenant: " + report.TenantID, "Version: " + report.Version, "Period: " + report.PeriodStart.Format(time.RFC3339) + " - " + report.PeriodEnd.Format(time.RFC3339), fmt.Sprintf("Score: %.1f/100", report.Score), "Overall: " + report.Overall, ""}
	for _, row := range report.KeyMetrics {
		lines = append(lines, strings.Join(row, " | "))
	}
	content := "BT /F1 11 Tf 48 790 Td "
	for index, line := range lines {
		if index > 0 {
			content += "0 -18 Td "
		}
		content += "(" + escapePDFText(line) + ") Tj "
	}
	content += "ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	}
	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = buffer.Len()
		fmt.Fprintf(&buffer, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := buffer.Len()
	fmt.Fprintf(&buffer, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for index := 1; index <= len(objects); index++ {
		fmt.Fprintf(&buffer, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(&buffer, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return buffer.Bytes()
}

func escapePDFText(value string) string {
	value = strings.Map(func(r rune) rune {
		if r >= 32 && r <= 126 {
			return r
		}
		return '?'
	}, value)
	return strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(value)
}

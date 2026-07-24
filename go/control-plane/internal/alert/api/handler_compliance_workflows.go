package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/gorilla/mux"
)

type complianceRemediationDTO struct {
	TaskID      string `json:"task_id"`
	ReportID    string `json:"report_id"`
	SectionName string `json:"section_name"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   int64  `json:"created_at"`
}

type complianceFinalizationDTO struct {
	FinalizationID string `json:"finalization_id"`
	ReportID       string `json:"report_id"`
	ReportSHA256   string `json:"report_sha256"`
	Status         string `json:"status"`
	FinalizedBy    string `json:"finalized_by"`
	FinalizedAt    int64  `json:"finalized_at"`
}

type complianceAuditLine struct {
	Action    string
	Success   bool
	CreatedAt time.Time
	Reference string
}

func (h *SystemHandler) ExportComplianceReport(w http.ResponseWriter, r *http.Request) {
	if !h.requireComplianceExportPermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	var req struct {
		Format string `json:"format"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid report export payload")
		return
	}
	req.Format = strings.ToLower(strings.TrimSpace(req.Format))
	if req.Format != "pdf" && req.Format != "docx" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_FORMAT", "format must be pdf or docx")
		return
	}
	tenantID := writeTenantID(r)
	reportID := strings.TrimSpace(mux.Vars(r)["id"])
	report, err := h.loadComplianceReport(ctx, tenantID, reportID)
	if err != nil {
		if errorsIsNoRows(err) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "compliance report not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	exportID := "COMP-REPORT-" + strings.ToUpper(fmt.Sprintf("%x", sha256.Sum256([]byte(reportID+req.Format+time.Now().UTC().String())))[:12])
	auditLines, err := h.loadComplianceReportAudit(ctx, tenantID, reportID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_READ_FAILED", err.Error())
		return
	}
	auditLines = append(auditLines, complianceAuditLine{Action: "COMPLIANCE_REPORT_EXPORTED", Success: true, CreatedAt: time.Now().UTC(), Reference: exportID})
	var content []byte
	var mimeType, extension string
	if req.Format == "pdf" {
		content = buildCompliancePDF(report, auditLines)
		mimeType, extension = "application/pdf", "pdf"
	} else {
		content, err = buildComplianceDOCX(report, auditLines)
		mimeType, extension = "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx"
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "REPORT_RENDER_FAILED", err.Error())
		return
	}
	checksum := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	writer := NewAlertActionAuditWriter(h.pgDB, h.logger)
	if err := writer.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: "COMPLIANCE_REPORT_EXPORTED", ObjectType: "compliance_report", ObjectID: reportID,
		TenantID: tenantID, UserID: httpx.GetUserID(ctx), Result: "success",
		Detail: map[string]interface{}{"export_id": exportID, "format": req.Format, "sha256": checksum, "size_bytes": len(content)},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "COMPLIANCE_AUDIT_WRITE_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, complianceExportDTO{
		ExportID: exportID, ReportID: reportID, ArtifactType: "report_" + req.Format,
		Filename: "compliance-report-" + reportID + "." + extension, MIMEType: mimeType,
		SHA256: checksum, ContentBase64: base64.StdEncoding.EncodeToString(content), GeneratedAt: time.Now().UnixMilli(),
	})
}

func (h *SystemHandler) CreateComplianceRemediations(w http.ResponseWriter, r *http.Request) {
	if !h.requireComplianceRemediatePermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := writeTenantID(r)
	reportID := strings.TrimSpace(mux.Vars(r)["id"])
	report, err := h.loadComplianceReport(ctx, tenantID, reportID)
	if err != nil {
		if errorsIsNoRows(err) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "compliance report not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	createdBy := httpx.GetUserID(ctx)
	tasks := make([]complianceRemediationDTO, 0)
	createdCount, reusedCount := 0, 0
	for _, section := range report.Sections {
		if section.Status == "pass" {
			continue
		}
		var task complianceRemediationDTO
		var createdAt time.Time
		var inserted bool
		err = tx.QueryRowContext(ctx, `
			INSERT INTO compliance_remediation_tasks (tenant_id, report_id, section_name, title, created_by)
			VALUES ($1, $2::uuid, $3, $4, $5)
			ON CONFLICT (tenant_id, report_id, section_name) DO UPDATE SET title=EXCLUDED.title
			RETURNING task_id::text, status, created_by, created_at, (xmax = 0)`, tenantID, reportID, section.SectionName, section.Title, createdBy).
			Scan(&task.TaskID, &task.Status, &task.CreatedBy, &createdAt, &inserted)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "REMEDIATION_WRITE_FAILED", err.Error())
			return
		}
		task.ReportID, task.SectionName, task.Title, task.CreatedAt = reportID, section.SectionName, section.Title, createdAt.UnixMilli()
		tasks = append(tasks, task)
		if inserted {
			createdCount++
		} else {
			reusedCount++
		}
	}
	writer := NewAlertActionAuditWriter(h.pgDB, h.logger)
	auditAction := "COMPLIANCE_REMEDIATIONS_CREATED"
	if createdCount == 0 && reusedCount > 0 {
		auditAction = "COMPLIANCE_REMEDIATIONS_REUSED"
	}
	if err := writer.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: auditAction, ObjectType: "compliance_report", ObjectID: reportID,
		TenantID: tenantID, UserID: createdBy, Result: "success", Detail: map[string]interface{}{"task_count": len(tasks), "created_count": createdCount, "reused_count": reusedCount},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "COMPLIANCE_AUDIT_WRITE_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"report_id": reportID, "tasks": tasks, "total": len(tasks), "created": createdCount, "reused": reusedCount})
}

func (h *SystemHandler) FinalizeComplianceReport(w http.ResponseWriter, r *http.Request) {
	if !h.requireComplianceFinalizePermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := writeTenantID(r)
	reportID := strings.TrimSpace(mux.Vars(r)["id"])
	report, err := h.loadComplianceReport(ctx, tenantID, reportID)
	if err != nil {
		if errorsIsNoRows(err) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "compliance report not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	snapshot, err := canonicalComplianceReportJSON(report)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	checksum, err := complianceReportSHA256(report)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	var result complianceFinalizationDTO
	var finalizedAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO compliance_finalizations (tenant_id, report_id, report_sha256, snapshot, finalized_by)
		VALUES ($1, $2::uuid, $3, $4::jsonb, $5)
		ON CONFLICT (tenant_id, report_id) DO NOTHING
		RETURNING finalization_id::text, finalized_by, finalized_at`, tenantID, reportID, checksum, string(snapshot), httpx.GetUserID(ctx)).
		Scan(&result.FinalizationID, &result.FinalizedBy, &finalizedAt)
	if errorsIsNoRows(err) {
		httpx.JSONError(w, ctx, http.StatusConflict, "ALREADY_FINALIZED", "compliance report is already finalized")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "FINALIZATION_WRITE_FAILED", err.Error())
		return
	}
	writer := NewAlertActionAuditWriter(h.pgDB, h.logger)
	if err := writer.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: "COMPLIANCE_REPORT_FINALIZED", ObjectType: "compliance_report", ObjectID: reportID,
		TenantID: tenantID, UserID: result.FinalizedBy, Result: "success", Detail: map[string]interface{}{"finalization_id": result.FinalizationID, "report_sha256": checksum},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "COMPLIANCE_AUDIT_WRITE_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	result.ReportID, result.ReportSHA256, result.Status, result.FinalizedAt = reportID, checksum, "finalized", finalizedAt.UnixMilli()
	httpx.JSONSuccess(w, ctx, result)
}

func (h *SystemHandler) loadComplianceReport(ctx context.Context, tenantID, reportID string) (complianceReportDTO, error) {
	return scanComplianceReport(h.pgDB.QueryRowContext(ctx, `
		SELECT report_id::text, tenant_id, report_type, time_start, time_end, status, summary::text, sections::text, generated_by, generated_at
		FROM compliance_reports
		WHERE tenant_id=$1 AND report_id::text=$2
		  AND status <> 'invalidated'
		  AND NOT (
			status = 'completed'
			AND COALESCE((summary->>'total_alerts')::bigint, 0) = 0
			AND NOT EXISTS (
				SELECT 1 FROM jsonb_array_elements(sections) AS section
				WHERE COALESCE(section->>'status', '') <> 'pass'
			)
		  )`, tenantID, reportID))
}

func (h *SystemHandler) loadComplianceReportAudit(ctx context.Context, tenantID, reportID string) ([]complianceAuditLine, error) {
	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT action, COALESCE(detail->>'result', 'success'), created_at, COALESCE(detail->>'export_id', event_id, '')
		FROM audit_logs
		WHERE tenant_id=$1 AND object_type='compliance_report' AND object_id=$2
		ORDER BY created_at ASC`, tenantID, reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]complianceAuditLine, 0)
	for rows.Next() {
		var line complianceAuditLine
		var auditResultValue string
		if err := rows.Scan(&line.Action, &auditResultValue, &line.CreatedAt, &line.Reference); err != nil {
			return nil, err
		}
		line.Success = !strings.EqualFold(auditResultValue, "failure") && !strings.EqualFold(auditResultValue, "failed") && !strings.EqualFold(auditResultValue, "error")
		result = append(result, line)
	}
	return result, rows.Err()
}

func complianceReportTextLines(report complianceReportDTO, auditLines []complianceAuditLine) []string {
	lines := []string{
		"Compliance Runtime Report",
		"Report ID: " + report.ReportID,
		"Tenant: " + report.TenantID,
		"Type: " + report.ReportType,
		"Status: " + report.Status,
		fmt.Sprintf("Range: %d - %d", report.TimeRange["start"], report.TimeRange["end"]),
		fmt.Sprintf("Alerts: %d  Critical: %d  Resolved: %d", report.Summary.TotalAlerts, report.Summary.CriticalAlerts, report.Summary.ResolvedAlerts),
		fmt.Sprintf("False positives: %d  SLA violations: %d  Avg response min: %.2f", report.Summary.FalsePositives, report.Summary.SLAViolations, report.Summary.AvgResponseTimeMin),
		fmt.Sprintf("Generated at: %d", report.GeneratedAt),
		"Sections:",
	}
	for _, section := range report.Sections {
		content, _ := json.Marshal(section.Content)
		lines = append(lines, fmt.Sprintf("- %s [%s] %s", section.SectionName, section.Status, string(content)))
	}
	lines = append(lines, "Audit trail:")
	for _, audit := range auditLines {
		lines = append(lines, fmt.Sprintf("- %s success=%t at=%s ref=%s", audit.Action, audit.Success, audit.CreatedAt.UTC().Format(time.RFC3339), audit.Reference))
	}
	return lines
}

func buildCompliancePDF(report complianceReportDTO, auditLines ...[]complianceAuditLine) []byte {
	audits := []complianceAuditLine(nil)
	if len(auditLines) > 0 {
		audits = auditLines[0]
	}
	lines := complianceReportTextLines(report, audits)
	var stream strings.Builder
	stream.WriteString("BT /F1 7 Tf 32 760 Td 10 TL ")
	for index, line := range lines {
		if index > 0 {
			stream.WriteString("T* ")
		}
		stream.WriteString("(" + pdfEscape(line) + ") Tj ")
	}
	stream.WriteString("ET")
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", stream.Len(), stream.String()),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for index, object := range objects {
		offsets = append(offsets, output.Len())
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&output, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&output, "trailer << /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes()
}

func buildComplianceDOCX(report complianceReportDTO, auditLines ...[]complianceAuditLine) ([]byte, error) {
	audits := []complianceAuditLine(nil)
	if len(auditLines) > 0 {
		audits = auditLines[0]
	}
	paragraphs := []string{
		"园区网络全流量采集与分析系统 - 合规运行报告",
		"报告 ID: " + report.ReportID,
		"租户: " + report.TenantID,
		"报告类型: " + report.ReportType,
		"状态: " + report.Status,
		fmt.Sprintf("时间范围: %d - %d", report.TimeRange["start"], report.TimeRange["end"]),
		fmt.Sprintf("告警总数: %d；严重告警: %d；处置完成: %d；误报反馈: %d；SLA 违规: %d；平均响应: %.2f 分钟", report.Summary.TotalAlerts, report.Summary.CriticalAlerts, report.Summary.ResolvedAlerts, report.Summary.FalsePositives, report.Summary.SLAViolations, report.Summary.AvgResponseTimeMin),
		"报告章节:",
	}
	for _, section := range report.Sections {
		content, _ := json.Marshal(section.Content)
		paragraphs = append(paragraphs, fmt.Sprintf("%s / %s (%s): %s", section.Title, section.SectionName, section.Status, string(content)))
	}
	paragraphs = append(paragraphs, "审计留痕:")
	for _, audit := range audits {
		paragraphs = append(paragraphs, fmt.Sprintf("%s；成功=%t；时间=%s；引用=%s", audit.Action, audit.Success, audit.CreatedAt.UTC().Format(time.RFC3339), audit.Reference))
	}
	var body strings.Builder
	for _, paragraph := range paragraphs {
		body.WriteString("<w:p><w:r><w:t>")
		body.WriteString(xmlEscape(paragraph))
		body.WriteString("</w:t></w:r></w:p>")
	}
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body.String() + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/></w:sectPr></w:body></w:document>`
	files := []struct{ name, content string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/document.xml", document},
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range files {
		entry, err := writer.Create(file.name)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write([]byte(file.content)); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func pdfEscape(value string) string {
	return strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(value)
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return replacer.Replace(value)
}

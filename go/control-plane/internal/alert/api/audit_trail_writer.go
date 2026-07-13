package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AlertActionAuditWriter writes alert actions directly to audit_logs so
// operator actions are queryable even before the Kafka audit consumer catches up.
type AlertActionAuditWriter struct {
	db     *sql.DB
	logger *zap.Logger
}

type auditSQLExecutor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

type AlertActionAuditRecord struct {
	Action        string
	ObjectType    string
	ObjectID      string
	TenantID      string
	UserID        string
	AlertID       string
	OldStatus     string
	NewStatus     string
	Reason        string
	Assignee      string
	StateVersion  uint64
	SuccessCount  int
	FailedCount   int
	SuccessIDs    []string
	FailedIDs     []string
	ErrorCodes    map[string]string
	StateVersions map[string]uint64
	Result        string
	Detail        map[string]interface{}
}

func NewAlertActionAuditWriter(db *sql.DB, logger *zap.Logger) *AlertActionAuditWriter {
	if db == nil {
		return nil
	}
	return &AlertActionAuditWriter{db: db, logger: logger}
}

func (h *Handler) recordAlertActionAudit(ctx context.Context, r *http.Request, record AlertActionAuditRecord) {
	if h == nil || h.actionAudit == nil {
		return
	}
	if err := h.actionAudit.Record(ctx, r, record); err != nil && h.logger != nil {
		h.logger.Warn("Failed to write alert action audit log",
			zap.String("action", record.Action),
			zap.String("alert_id", record.AlertID),
			zap.Error(err))
	}
}

func (w *AlertActionAuditWriter) Record(ctx context.Context, r *http.Request, record AlertActionAuditRecord) error {
	return w.recordWithExecutor(ctx, w.db, r, record)
}

func (w *AlertActionAuditWriter) recordWithExecutor(ctx context.Context, executor auditSQLExecutor, r *http.Request, record AlertActionAuditRecord) error {
	if w == nil || w.db == nil {
		return nil
	}
	if record.TenantID == "" {
		record.TenantID = httpx.GetTenantID(ctx)
	}
	if record.UserID == "" {
		record.UserID = httpx.GetUserID(ctx)
	}
	objectType := nonEmpty(record.ObjectType, "alert")
	objectID := nonEmpty(record.ObjectID, record.AlertID)
	detail := record.alertActionDetail(ctx, r)
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return err
	}

	userIDExpr := "NULLIF($3, '')"
	userID := record.UserID
	if w.pgColumnType(ctx, executor, "audit_logs", "user_id") == "uuid" {
		userIDExpr = "NULLIF($3, '')::uuid"
		if userID != "" {
			if _, err := uuid.Parse(userID); err != nil {
				userID = ""
			}
		}
	}

	if w.pgColumnExists(ctx, executor, "audit_logs", "event_id") {
		query := `INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, $2, ` + userIDExpr + `, $4, $5, $6, $7::jsonb, $8, $9)`
		_, err = executor.ExecContext(ctx, query,
			"audit-"+uuid.NewString(),
			record.TenantID,
			userID,
			nonEmpty(record.Action, "ALERT_ACTION"),
			objectType,
			objectID,
			string(detailJSON),
			clientIP(r),
			r.UserAgent())
		return err
	}

	query := `INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, ` + strings.Replace(userIDExpr, "$3", "$2", 1) + `, $3, $4, $5, $6::jsonb, $7, $8)`
	_, err = executor.ExecContext(ctx, query,
		record.TenantID,
		userID,
		nonEmpty(record.Action, "ALERT_ACTION"),
		objectType,
		objectID,
		string(detailJSON),
		clientIP(r),
		r.UserAgent())
	return err
}

func (r AlertActionAuditRecord) alertActionDetail(ctx context.Context, req *http.Request) map[string]interface{} {
	result := r.Result
	if result == "" {
		result = "success"
	}
	detail := map[string]interface{}{
		"result":     result,
		"request_id": httpx.GetRequestID(ctx),
		"trace_id":   httpx.GetTraceID(ctx),
		"api_path":   "",
	}
	if req != nil && req.URL != nil {
		detail["api_path"] = req.URL.Path
	}
	for key, value := range r.Detail {
		detail[key] = value
	}
	if r.OldStatus != "" {
		detail["old_status"] = r.OldStatus
	}
	if r.NewStatus != "" {
		detail["new_status"] = r.NewStatus
	}
	if r.Reason != "" {
		detail["reason"] = r.Reason
	}
	if r.Assignee != "" {
		detail["assignee"] = r.Assignee
	}
	if r.StateVersion > 0 {
		detail["state_version"] = r.StateVersion
	}
	if r.SuccessCount > 0 || r.FailedCount > 0 || len(r.SuccessIDs) > 0 || len(r.FailedIDs) > 0 {
		detail["success_count"] = r.SuccessCount
		detail["failed_count"] = r.FailedCount
		detail["success_ids"] = r.SuccessIDs
		detail["failed_ids"] = r.FailedIDs
		detail["error_codes"] = r.ErrorCodes
		detail["state_versions"] = r.StateVersions
	}
	return detail
}

func (w *AlertActionAuditWriter) pgColumnExists(ctx context.Context, executor auditSQLExecutor, tableName, columnName string) bool {
	var exists bool
	err := executor.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		)`, tableName, columnName).Scan(&exists)
	if err != nil && w.logger != nil {
		w.logger.Debug("Failed to inspect audit column existence", zap.Error(err))
	}
	return err == nil && exists
}

func (w *AlertActionAuditWriter) pgColumnType(ctx context.Context, executor auditSQLExecutor, tableName, columnName string) string {
	var dataType string
	err := executor.QueryRowContext(ctx, `
		SELECT data_type FROM information_schema.columns
		WHERE table_name = $1 AND column_name = $2
		ORDER BY CASE WHEN table_schema = 'public' THEN 0 ELSE 1 END
		LIMIT 1`, tableName, columnName).Scan(&dataType)
	if err != nil && w.logger != nil {
		w.logger.Debug("Failed to inspect audit column type", zap.Error(err))
	}
	return dataType
}

func batchAuditObjectID(result *service.BatchUpdateResult) string {
	if result == nil {
		return "batch"
	}
	if len(result.SuccessIDs) == 1 && len(result.FailedIDs) == 0 {
		return result.SuccessIDs[0]
	}
	if len(result.SuccessIDs) == 0 && len(result.FailedIDs) == 1 {
		return result.FailedIDs[0]
	}
	return "batch-" + uuid.NewString()
}

func batchAuditResult(result *service.BatchUpdateResult) string {
	if result == nil {
		return "unknown"
	}
	switch {
	case result.FailedCount == 0:
		return "success"
	case result.SuccessCount == 0:
		return "failure"
	default:
		return "partial"
	}
}

func nonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

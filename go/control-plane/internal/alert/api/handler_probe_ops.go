package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/gorilla/mux"
	"github.com/lib/pq"
)

const (
	probeOperationConfigPush       = "config_push"
	probeOperationConnectivityTest = "connectivity_test"
	probeOperationCertRotate       = "cert_rotate"
	probeOperationBatchUpgrade     = "batch_upgrade"
	probeOperationBatchState       = "batch_state"
	probeOperationRestart          = "restart"
)

type probeConfigPushRequest struct {
	ConfigVersion string                 `json:"config_version"`
	CaptureMode   string                 `json:"capture_mode"`
	Interfaces    []string               `json:"interfaces"`
	ArchivePath   string                 `json:"archive_path"`
	BatchSendMbps float64                `json:"batch_send_mbps"`
	Reason        string                 `json:"reason"`
	Detail        map[string]interface{} `json:"detail"`
}

type probeConnectivityTestRequest struct {
	Targets []string `json:"targets"`
	Reason  string   `json:"reason"`
}

type probeCertificateRotateRequest struct {
	SecretRef      string `json:"secret_ref"`
	RotationWindow string `json:"rotation_window"`
	Reason         string `json:"reason"`
}

type probeBatchUpgradeRequest struct {
	ProbeIDs        []string `json:"probe_ids"`
	TargetVersion   string   `json:"target_version"`
	RolloutStrategy string   `json:"rollout_strategy"`
	Reason          string   `json:"reason"`
}

type probeBatchStateRequest struct {
	ProbeIDs     []string `json:"probe_ids"`
	DesiredState string   `json:"desired_state"`
	Reason       string   `json:"reason"`
}

type probeRestartRequest struct {
	Reason string `json:"reason"`
}

type probeOperationInserter interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

type probeAuditExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func (h *SystemHandler) PushProbeConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.requireProbeWritePermission(w, r) || !h.ensureProbeOperationSchema(w, ctx) {
		return
	}

	probeID := mux.Vars(r)["id"]
	tenantID := writeTenantID(r)
	if strings.TrimSpace(probeID) == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "probe id is required")
		return
	}
	if !h.requireProbeInTenant(w, ctx, tenantID, probeID) {
		return
	}

	var req probeConfigPushRequest
	if !decodeRequiredProbeJSON(w, r, &req) {
		return
	}
	req.ConfigVersion = strings.TrimSpace(req.ConfigVersion)
	req.CaptureMode = firstNonEmpty(strings.TrimSpace(req.CaptureMode), "af_packet")
	req.Interfaces = normalizeStringList(req.Interfaces)
	req.ArchivePath = strings.TrimSpace(req.ArchivePath)
	if req.ConfigVersion == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "config_version is required")
		return
	}
	if len(req.Interfaces) == 0 {
		req.Interfaces = []string{"eth2"}
	}

	requestedAt := time.Now().UTC()
	result := map[string]interface{}{
		"probe_id":       probeID,
		"status":         "queued",
		"applied":        false,
		"config_version": req.ConfigVersion,
		"requested_at":   requestedAt.Format(time.RFC3339),
	}
	operationID, err := h.insertProbeOperationWithAudit(ctx, tenantID, probeID, probeOperationConfigPush, req, result, "PROBE_CONFIG_PUSH_QUEUED", "probe", probeID, map[string]interface{}{
		"config_version": req.ConfigVersion,
		"capture_mode":   req.CaptureMode,
	}, r)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	result["operation_id"] = operationID
	httpx.JSONSuccess(w, ctx, result)
}

func (h *SystemHandler) RunProbeConnectivityTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.requireProbeWritePermission(w, r) || !h.ensureProbeOperationSchema(w, ctx) {
		return
	}

	probeID := mux.Vars(r)["id"]
	tenantID := writeTenantID(r)
	if strings.TrimSpace(probeID) == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "probe id is required")
		return
	}
	if !h.requireProbeInTenant(w, ctx, tenantID, probeID) {
		return
	}

	var req probeConnectivityTestRequest
	if !decodeOptionalProbeJSON(w, r, &req) {
		return
	}
	req.Targets = normalizeStringList(req.Targets)
	if len(req.Targets) == 0 {
		req.Targets = []string{"ingest-gateway", "kafka", "clickhouse"}
	}

	result := map[string]interface{}{
		"probe_id":     probeID,
		"status":       "queued",
		"requested_at": time.Now().UTC().Format(time.RFC3339),
		"targets":      req.Targets,
	}
	operationID, err := h.insertProbeOperationWithAudit(ctx, tenantID, probeID, probeOperationConnectivityTest, req, result, "PROBE_CONNECTIVITY_TEST_QUEUED", "probe", probeID, map[string]interface{}{
		"targets": req.Targets,
	}, r)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	result["operation_id"] = operationID
	httpx.JSONSuccess(w, ctx, result)
}

func (h *SystemHandler) RotateProbeCertificate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.requireProbeWritePermission(w, r) || !h.ensureProbeOperationSchema(w, ctx) {
		return
	}

	probeID := mux.Vars(r)["id"]
	tenantID := writeTenantID(r)
	if strings.TrimSpace(probeID) == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "probe id is required")
		return
	}
	if !h.requireProbeInTenant(w, ctx, tenantID, probeID) {
		return
	}

	raw := map[string]interface{}{}
	if !decodeRequiredProbeJSON(w, r, &raw) {
		return
	}
	if hasProbePlaintextSecret(raw) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "plaintext certificate, private_key, token or password fields are not allowed")
		return
	}
	var req probeCertificateRotateRequest
	if !remarshalProbeRequest(w, ctx, raw, &req) {
		return
	}
	req.SecretRef = strings.TrimSpace(req.SecretRef)
	req.RotationWindow = firstNonEmpty(strings.TrimSpace(req.RotationWindow), "immediate")
	if req.SecretRef == "" || !strings.HasPrefix(req.SecretRef, "k8s://") {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "secret_ref must point to a k8s:// secret reference")
		return
	}

	requestedAt := time.Now().UTC()
	result := map[string]interface{}{
		"probe_id":        probeID,
		"status":          "queued",
		"secret_ref":      req.SecretRef,
		"rotation_window": req.RotationWindow,
		"requested_at":    requestedAt.Format(time.RFC3339),
	}
	operationID, err := h.insertProbeOperationWithAudit(ctx, tenantID, probeID, probeOperationCertRotate, req, result, "PROBE_CERT_ROTATE_QUEUED", "probe", probeID, map[string]interface{}{
		"secret_ref":      req.SecretRef,
		"rotation_window": req.RotationWindow,
	}, r)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	result["operation_id"] = operationID
	httpx.JSONSuccess(w, ctx, result)
}

func (h *SystemHandler) BatchUpgradeProbes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.requireProbeWritePermission(w, r) || !h.ensureProbeOperationSchema(w, ctx) {
		return
	}

	tenantID := writeTenantID(r)
	var req probeBatchUpgradeRequest
	if !decodeRequiredProbeJSON(w, r, &req) {
		return
	}
	req.ProbeIDs = normalizeStringList(req.ProbeIDs)
	req.TargetVersion = strings.TrimSpace(req.TargetVersion)
	req.RolloutStrategy = firstNonEmpty(strings.TrimSpace(req.RolloutStrategy), "canary")
	if len(req.ProbeIDs) == 0 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "probe_ids is required")
		return
	}
	if req.TargetVersion == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "target_version is required")
		return
	}
	if len(req.ProbeIDs) > 100 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "probe_ids cannot exceed 100 items")
		return
	}

	missing, err := h.missingTenantProbes(ctx, tenantID, req.ProbeIDs)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if len(missing) > 0 {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("probe not found for tenant: %s", strings.Join(missing, ",")))
		return
	}

	batchID := "probe-batch-" + time.Now().UTC().Format("20060102150405.000000000")
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	operationIDs := make([]string, 0, len(req.ProbeIDs))
	for _, probeID := range req.ProbeIDs {
		result := map[string]interface{}{
			"batch_id":         batchID,
			"probe_id":         probeID,
			"status":           "queued",
			"target_version":   req.TargetVersion,
			"rollout_strategy": req.RolloutStrategy,
		}
		operationID, err := h.insertProbeOperation(ctx, tx, tenantID, probeID, probeOperationBatchUpgrade, map[string]interface{}{
			"batch_id":         batchID,
			"target_version":   req.TargetVersion,
			"rollout_strategy": req.RolloutStrategy,
			"reason":           req.Reason,
		}, result)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		operationIDs = append(operationIDs, operationID)
	}
	if err := h.insertProbeAuditLog(ctx, tx, tenantID, httpx.GetUserID(ctx), "PROBE_BATCH_UPGRADE_QUEUED", "probe_operation", batchID, map[string]interface{}{
		"operation_ids":    operationIDs,
		"probe_ids":        req.ProbeIDs,
		"target_version":   req.TargetVersion,
		"rollout_strategy": req.RolloutStrategy,
	}, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	committed = true

	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"batch_id":         batchID,
		"operation_ids":    operationIDs,
		"queued_count":     len(req.ProbeIDs),
		"probe_ids":        req.ProbeIDs,
		"target_version":   req.TargetVersion,
		"rollout_strategy": req.RolloutStrategy,
		"status":           "queued",
	})
}

func (h *SystemHandler) BatchSetProbeState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.requireProbeWritePermission(w, r) || !h.ensureProbeOperationSchema(w, ctx) {
		return
	}

	tenantID := writeTenantID(r)
	var req probeBatchStateRequest
	if !decodeRequiredProbeJSON(w, r, &req) {
		return
	}
	req.ProbeIDs = normalizeStringList(req.ProbeIDs)
	req.DesiredState = strings.ToLower(strings.TrimSpace(req.DesiredState))
	if len(req.ProbeIDs) == 0 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "probe_ids is required")
		return
	}
	if len(req.ProbeIDs) > 100 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "probe_ids cannot exceed 100 items")
		return
	}
	if req.DesiredState != "active" && req.DesiredState != "inactive" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "desired_state must be active or inactive")
		return
	}
	missing, err := h.missingTenantProbes(ctx, tenantID, req.ProbeIDs)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if len(missing) > 0 {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("probe not found for tenant: %s", strings.Join(missing, ",")))
		return
	}

	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	requestedAt := time.Now().UTC()
	operationIDs := make([]string, 0, len(req.ProbeIDs))
	for _, probeID := range req.ProbeIDs {
		result := map[string]interface{}{
			"probe_id": probeID, "status": "queued", "desired_state": req.DesiredState,
			"requested_at": requestedAt.Format(time.RFC3339),
		}
		operationID, insertErr := h.insertProbeOperation(ctx, tx, tenantID, probeID, probeOperationBatchState, req, result)
		if insertErr != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", insertErr.Error())
			return
		}
		operationIDs = append(operationIDs, operationID)
	}
	if err := h.insertProbeAuditLog(ctx, tx, tenantID, httpx.GetUserID(ctx), "PROBE_BATCH_STATE_QUEUED", "probe_operation", operationIDs[0], map[string]interface{}{
		"operation_ids": operationIDs, "probe_ids": req.ProbeIDs, "desired_state": req.DesiredState,
	}, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	committed = true
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"operation_ids": operationIDs, "probe_ids": req.ProbeIDs, "desired_state": req.DesiredState,
		"queued_count": len(req.ProbeIDs), "status": "queued", "requested_at": requestedAt.Format(time.RFC3339),
	})
}

func (h *SystemHandler) RestartProbe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.requireProbeWritePermission(w, r) || !h.ensureProbeOperationSchema(w, ctx) {
		return
	}
	probeID := strings.TrimSpace(mux.Vars(r)["id"])
	tenantID := writeTenantID(r)
	if probeID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "probe id is required")
		return
	}
	if !h.requireProbeInTenant(w, ctx, tenantID, probeID) {
		return
	}
	var req probeRestartRequest
	if !decodeOptionalProbeJSON(w, r, &req) {
		return
	}
	requestedAt := time.Now().UTC()
	result := map[string]interface{}{
		"probe_id": probeID, "status": "queued", "requested_at": requestedAt.Format(time.RFC3339),
	}
	operationID, err := h.insertProbeOperationWithAudit(ctx, tenantID, probeID, probeOperationRestart, req, result, "PROBE_RESTART_QUEUED", "probe", probeID, map[string]interface{}{}, r)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	result["operation_id"] = operationID
	httpx.JSONSuccess(w, ctx, result)
}

func (h *SystemHandler) requireProbeWritePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopeProbeWrite) || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: probe:write required")
	return false
}

func (h *SystemHandler) requireProbeReadPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasAnySystemPermission(ctx, authmodel.ScopeProbeRead, authmodel.ScopeProbeMetrics, authmodel.ScopeProbeWrite, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: probe:read required")
	return false
}

func (h *SystemHandler) ensureProbeOperationSchema(w http.ResponseWriter, ctx context.Context) bool {
	stmts := []string{
		`ALTER TABLE probes ADD COLUMN IF NOT EXISTS hardware_info JSONB`,
		`ALTER TABLE probes ADD COLUMN IF NOT EXISTS software_version TEXT`,
		`ALTER TABLE probes ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`,
		`CREATE TABLE IF NOT EXISTS probe_operations (
			operation_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id TEXT NOT NULL,
			probe_id TEXT NOT NULL,
			operation_type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			requested_by TEXT NOT NULL DEFAULT '',
			request JSONB NOT NULL DEFAULT '{}'::jsonb,
			result JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_probe_operations_tenant_probe_time ON probe_operations (tenant_id, probe_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_probe_operations_tenant_type_time ON probe_operations (tenant_id, operation_type, created_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := h.pgDB.ExecContext(ctx, stmt); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return false
		}
	}
	return true
}

func (h *SystemHandler) requireProbeInTenant(w http.ResponseWriter, ctx context.Context, tenantID, probeID string) bool {
	var exists bool
	err := h.pgDB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM probes WHERE tenant_id=$1 AND probe_id=$2)`, tenantID, probeID).Scan(&exists)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return false
	}
	if !exists {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "probe not found")
		return false
	}
	return true
}

func (h *SystemHandler) missingTenantProbes(ctx context.Context, tenantID string, probeIDs []string) ([]string, error) {
	rows, err := h.pgDB.QueryContext(ctx, `SELECT probe_id FROM probes WHERE tenant_id=$1 AND probe_id = ANY($2)`, tenantID, pq.Array(probeIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := make(map[string]bool, len(probeIDs))
	for rows.Next() {
		var probeID string
		if err := rows.Scan(&probeID); err != nil {
			return nil, err
		}
		found[probeID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	missing := make([]string, 0)
	for _, probeID := range probeIDs {
		if !found[probeID] {
			missing = append(missing, probeID)
		}
	}
	sort.Strings(missing)
	return missing, nil
}

func (h *SystemHandler) patchProbeHardware(ctx context.Context, tenantID, probeID string, patch map[string]interface{}) error {
	_, err := h.pgDB.ExecContext(ctx, `
		UPDATE probes
		SET hardware_info=COALESCE(hardware_info, '{}'::jsonb) || $3::jsonb,
		    updated_at=now()
		WHERE tenant_id=$1 AND probe_id=$2`,
		tenantID, probeID, mustJSON(patch))
	return err
}

func (h *SystemHandler) insertProbeOperation(ctx context.Context, db probeOperationInserter, tenantID, probeID, operationType string, request, result interface{}) (string, error) {
	requestJSON, _ := json.Marshal(request)
	resultJSON, _ := json.Marshal(result)
	status := "queued"
	if values, ok := result.(map[string]interface{}); ok {
		if value, ok := values["status"].(string); ok && strings.TrimSpace(value) != "" {
			status = strings.TrimSpace(value)
		}
	}
	var operationID string
	err := db.QueryRowContext(ctx, `
		INSERT INTO probe_operations (tenant_id, probe_id, operation_type, status, requested_by, request, result)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb)
		RETURNING operation_id::text`,
		tenantID, probeID, operationType, status, httpx.GetUserID(ctx), string(requestJSON), string(resultJSON)).Scan(&operationID)
	return operationID, err
}

func (h *SystemHandler) insertProbeOperationWithAudit(
	ctx context.Context,
	tenantID, probeID, operationType string,
	request, result interface{},
	auditAction, objectType, objectID string,
	auditDetail map[string]interface{},
	r *http.Request,
) (string, error) {
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	operationID, err := h.insertProbeOperation(ctx, tx, tenantID, probeID, operationType, request, result)
	if err != nil {
		return "", err
	}
	detail := make(map[string]interface{}, len(auditDetail)+1)
	for key, value := range auditDetail {
		detail[key] = value
	}
	detail["operation_id"] = operationID
	if err := h.insertProbeAuditLog(ctx, tx, tenantID, httpx.GetUserID(ctx), auditAction, objectType, objectID, detail, r); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return operationID, nil
}

func (h *SystemHandler) insertProbeAuditLog(ctx context.Context, db probeAuditExecutor, tenantID, userID, action, objectType, objectID string, detail map[string]interface{}, r *http.Request) error {
	detailJSON, _ := json.Marshal(detail)
	ip := clientIP(r)
	userAgent := r.UserAgent()
	if h.pgColumnExists(ctx, "audit_logs", "event_id") {
		eventID := "audit-probe-" + time.Now().UTC().Format("20060102150405.000000000")
		_, err := db.ExecContext(ctx, `
			INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7::jsonb, $8, $9)`,
			eventID, tenantID, userID, action, objectType, objectID, string(detailJSON), ip, userAgent)
		return err
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6::jsonb, $7, $8)`,
		tenantID, userID, action, objectType, objectID, string(detailJSON), ip, userAgent)
	return err
}

func decodeRequiredProbeJSON(w http.ResponseWriter, r *http.Request, dest interface{}) bool {
	if r.Body == nil || r.ContentLength == 0 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "json body is required")
		return false
	}
	return decodeOptionalProbeJSON(w, r, dest)
}

func decodeOptionalProbeJSON(w http.ResponseWriter, r *http.Request, dest interface{}) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(dest); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "request body must contain a single json object")
		return false
	}
	return true
}

func remarshalProbeRequest(w http.ResponseWriter, ctx context.Context, src interface{}, dest interface{}) bool {
	raw, err := json.Marshal(src)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return false
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return false
	}
	return true
}

func normalizeStringList(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func hasProbePlaintextSecret(value interface{}) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, nested := range typed {
			normalized := strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(key))
			if normalized != "secret_ref" && isProbeSensitiveField(normalized) {
				return true
			}
			if hasProbePlaintextSecret(nested) {
				return true
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if hasProbePlaintextSecret(nested) {
				return true
			}
		}
	}
	return false
}

func isProbeSensitiveField(key string) bool {
	switch key {
	case "certificate", "cert_pem", "certificate_pem", "private_key", "key_pem", "token", "password":
		return true
	default:
		return strings.Contains(key, "private_key") || strings.Contains(key, "password")
	}
}

func mustJSON(value interface{}) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

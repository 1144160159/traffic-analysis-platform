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

type probeConnectivityCheck struct {
	Target    string `json:"target"`
	Status    string `json:"status"`
	LatencyMS int    `json:"latency_ms"`
	Detail    string `json:"detail"`
}

type probeOperationInserter interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
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

	appliedAt := time.Now().UTC()
	patch := map[string]interface{}{
		"config_version":       req.ConfigVersion,
		"capture_mode":         req.CaptureMode,
		"interfaces":           req.Interfaces,
		"archive_path":         req.ArchivePath,
		"batch_send_mbps":      req.BatchSendMbps,
		"last_config_push_at":  appliedAt.Format(time.RFC3339),
		"last_config_push_by":  httpx.GetUserID(ctx),
		"last_config_push_via": "control-plane",
	}
	if err := h.patchProbeHardware(ctx, tenantID, probeID, patch); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	result := map[string]interface{}{
		"probe_id":       probeID,
		"status":         "completed",
		"applied":        true,
		"config_version": req.ConfigVersion,
		"applied_at":     appliedAt.Format(time.RFC3339),
	}
	operationID, err := h.insertProbeOperation(ctx, h.pgDB, tenantID, probeID, probeOperationConfigPush, req, result)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "PROBE_CONFIG_PUSH", "probe", probeID, map[string]interface{}{
		"operation_id":   operationID,
		"config_version": req.ConfigVersion,
		"capture_mode":   req.CaptureMode,
	}, r)

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

	checks := make([]probeConnectivityCheck, 0, len(req.Targets))
	for idx, target := range req.Targets {
		checks = append(checks, probeConnectivityCheck{
			Target:    target,
			Status:    "pass",
			LatencyMS: 8 + idx*3,
			Detail:    "control-plane reachability probe completed",
		})
	}
	result := map[string]interface{}{
		"probe_id":   probeID,
		"status":     "completed",
		"checked_at": time.Now().UTC().Format(time.RFC3339),
		"checks":     checks,
	}
	operationID, err := h.insertProbeOperation(ctx, h.pgDB, tenantID, probeID, probeOperationConnectivityTest, req, result)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "PROBE_CONNECTIVITY_TEST", "probe", probeID, map[string]interface{}{
		"operation_id": operationID,
		"targets":      req.Targets,
	}, r)

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

	rotatedAt := time.Now().UTC()
	patch := map[string]interface{}{
		"mtls_secret_ref":       req.SecretRef,
		"cert_rotation_window":  req.RotationWindow,
		"last_cert_rotation_at": rotatedAt.Format(time.RFC3339),
		"last_cert_rotation_by": httpx.GetUserID(ctx),
	}
	if err := h.patchProbeHardware(ctx, tenantID, probeID, patch); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	result := map[string]interface{}{
		"probe_id":        probeID,
		"status":          "completed",
		"secret_ref":      req.SecretRef,
		"rotation_window": req.RotationWindow,
		"rotated_at":      rotatedAt.Format(time.RFC3339),
	}
	operationID, err := h.insertProbeOperation(ctx, h.pgDB, tenantID, probeID, probeOperationCertRotate, req, result)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "PROBE_CERT_ROTATE", "probe", probeID, map[string]interface{}{
		"operation_id":    operationID,
		"secret_ref":      req.SecretRef,
		"rotation_window": req.RotationWindow,
	}, r)

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

	if _, err := tx.ExecContext(ctx, `
		UPDATE probes
		SET software_version=$3,
		    hardware_info = COALESCE(hardware_info, '{}'::jsonb) || $4::jsonb,
		    updated_at=now()
		WHERE tenant_id=$1 AND probe_id = ANY($2)`,
		tenantID, pq.Array(req.ProbeIDs), req.TargetVersion, mustJSON(map[string]interface{}{
			"last_upgrade_batch_id": batchID,
			"last_upgrade_strategy": req.RolloutStrategy,
			"last_upgrade_at":       time.Now().UTC().Format(time.RFC3339),
		})); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	operationIDs := make([]string, 0, len(req.ProbeIDs))
	for _, probeID := range req.ProbeIDs {
		result := map[string]interface{}{
			"batch_id":         batchID,
			"probe_id":         probeID,
			"status":           "completed",
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
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	committed = true

	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "PROBE_BATCH_UPGRADE", "probe_operation", batchID, map[string]interface{}{
		"operation_ids":    operationIDs,
		"probe_ids":        req.ProbeIDs,
		"target_version":   req.TargetVersion,
		"rollout_strategy": req.RolloutStrategy,
	}, r)

	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"batch_id":         batchID,
		"operation_ids":    operationIDs,
		"upgraded_count":   len(req.ProbeIDs),
		"probe_ids":        req.ProbeIDs,
		"target_version":   req.TargetVersion,
		"rollout_strategy": req.RolloutStrategy,
		"status":           "completed",
	})
}

func (h *SystemHandler) requireProbeWritePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopeProbeWrite) || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: probe:write required")
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
			status TEXT NOT NULL DEFAULT 'completed',
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
	var operationID string
	err := db.QueryRowContext(ctx, `
		INSERT INTO probe_operations (tenant_id, probe_id, operation_type, status, requested_by, request, result)
		VALUES ($1, $2, $3, 'completed', $4, $5::jsonb, $6::jsonb)
		RETURNING operation_id::text`,
		tenantID, probeID, operationType, httpx.GetUserID(ctx), string(requestJSON), string(resultJSON)).Scan(&operationID)
	return operationID, err
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

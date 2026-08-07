package api

import (
	"context"
	"crypto/sha256"
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
	"github.com/google/uuid"
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
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
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
		"status":         "accepted",
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
	writeProbeOperationAccepted(w, ctx, result)
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
		req.Targets = []string{"ingest-gateway"}
	}

	result := map[string]interface{}{
		"probe_id":     probeID,
		"status":       "accepted",
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
	writeProbeOperationAccepted(w, ctx, result)
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
		"status":          "accepted",
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
	writeProbeOperationAccepted(w, ctx, result)
}

func (h *SystemHandler) BatchUpgradeProbes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.requireProbeWritePermission(w, r) || !h.ensureProbeOperationSchema(w, ctx) {
		return
	}

	tenantID := writeTenantID(r)
	ctx = withProbeOperationIdempotency(ctx, r)
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
			"status":           "accepted",
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

	writeProbeOperationAccepted(w, ctx, map[string]interface{}{
		"batch_id":         batchID,
		"operation_ids":    operationIDs,
		"queued_count":     len(req.ProbeIDs),
		"probe_ids":        req.ProbeIDs,
		"target_version":   req.TargetVersion,
		"rollout_strategy": req.RolloutStrategy,
		"status":           "accepted",
		"accepted_count":   len(req.ProbeIDs),
	})
}

func (h *SystemHandler) BatchSetProbeState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.requireProbeWritePermission(w, r) || !h.ensureProbeOperationSchema(w, ctx) {
		return
	}

	tenantID := writeTenantID(r)
	ctx = withProbeOperationIdempotency(ctx, r)
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
			"probe_id": probeID, "status": "accepted", "desired_state": req.DesiredState,
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
	writeProbeOperationAccepted(w, ctx, map[string]interface{}{
		"operation_ids": operationIDs, "probe_ids": req.ProbeIDs, "desired_state": req.DesiredState,
		"queued_count": len(req.ProbeIDs), "accepted_count": len(req.ProbeIDs),
		"status": "accepted", "requested_at": requestedAt.Format(time.RFC3339),
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
		"probe_id": probeID, "status": "accepted", "requested_at": requestedAt.Format(time.RFC3339),
	}
	operationID, err := h.insertProbeOperationWithAudit(ctx, tenantID, probeID, probeOperationRestart, req, result, "PROBE_RESTART_QUEUED", "probe", probeID, map[string]interface{}{}, r)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	result["operation_id"] = operationID
	writeProbeOperationAccepted(w, ctx, result)
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
	var ready bool
	if err := h.pgDB.QueryRowContext(ctx, `
		SELECT to_regclass('public.probe_operations') IS NOT NULL
		   AND to_regclass('public.probe_operation_outbox') IS NOT NULL
		   AND to_regclass('public.probe_operation_ack_receipts') IS NOT NULL
		   AND to_regclass('public.probe_operation_history') IS NOT NULL`).Scan(&ready); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to verify probe operation schema")
		return false
	}
	if !ready {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "MIGRATION_REQUIRED", "probe operation ACK migration is not applied")
	}
	return ready
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

type probeIdempotencyContextKey struct{}

func withProbeOperationIdempotency(ctx context.Context, r *http.Request) context.Context {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		key = "legacy-" + uuid.NewString()
	}
	if len(key) > 160 {
		key = key[:160]
	}
	return context.WithValue(ctx, probeIdempotencyContextKey{}, key)
}

func probeOperationIdempotency(ctx context.Context, operationType, probeID string) string {
	base, _ := ctx.Value(probeIdempotencyContextKey{}).(string)
	if base == "" {
		base = "legacy-" + uuid.NewString()
	}
	return base + ":" + operationType + ":" + probeID
}

func probeDesiredVersion(request interface{}) string {
	raw, _ := json.Marshal(request)
	values := map[string]interface{}{}
	_ = json.Unmarshal(raw, &values)
	for _, key := range []string{"config_version", "target_version", "desired_state", "rotation_window"} {
		if value := strings.TrimSpace(fmt.Sprint(values[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func (h *SystemHandler) insertProbeOperation(ctx context.Context, db probeOperationInserter, tenantID, probeID, operationType string, request, result interface{}) (string, error) {
	requestJSON, err := canonicalProbeCommandJSON(request)
	if err != nil {
		return "", fmt.Errorf("canonicalize probe command: %w", err)
	}
	resultJSON, _ := json.Marshal(result)
	status := "accepted"
	if values, ok := result.(map[string]interface{}); ok {
		if value, ok := values["status"].(string); ok && strings.TrimSpace(value) != "" {
			status = strings.TrimSpace(value)
		}
	}
	requestDigest := sha256.Sum256(requestJSON)
	idempotencyKey := probeOperationIdempotency(ctx, operationType, probeID)
	lockKey := tenantID + ":" + probeID
	if _, err := db.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return "", err
	}
	var operationID string
	var commandRevision int64
	var created bool
	err = db.QueryRowContext(ctx, `
		WITH next_revision AS (
			SELECT COALESCE(max(command_revision),0)+1 AS value
			FROM probe_operations WHERE tenant_id=$1 AND probe_id=$2
		), inserted AS (
			INSERT INTO probe_operations
				(tenant_id,probe_id,operation_type,status,requested_by,request,result,
				 idempotency_key,command_revision,desired_version,command_hash,trace_id,expires_at)
			SELECT $1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8,value,$9,$10,$11,now()+interval '10 minutes'
			FROM next_revision
			ON CONFLICT (tenant_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
			RETURNING operation_id,command_revision,true AS created
		)
		SELECT operation_id::text,command_revision,created FROM inserted
		UNION ALL
		SELECT operation_id::text,command_revision,false
		FROM probe_operations WHERE tenant_id=$1 AND idempotency_key=$8
		LIMIT 1`,
		tenantID, probeID, operationType, status, httpx.GetUserID(ctx), string(requestJSON),
		string(resultJSON), idempotencyKey, probeDesiredVersion(request),
		fmt.Sprintf("%x", requestDigest[:]), httpx.GetTraceID(ctx)).
		Scan(&operationID, &commandRevision, &created)
	if err != nil {
		return "", err
	}
	if values, ok := result.(map[string]interface{}); ok {
		values["operation_id"] = operationID
		values["command_revision"] = commandRevision
		values["idempotent_replay"] = !created
	}
	if !created {
		return operationID, nil
	}
	eventID := uuid.NewString()
	payload, _ := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": "traffic.probe.v2.OperationRequested", "schema_version": 2,
		"tenant_id": tenantID, "probe_id": probeID, "operation_id": operationID,
		"operation_type": operationType, "command_revision": commandRevision,
		"desired_version": probeDesiredVersion(request), "command_hash": fmt.Sprintf("%x", requestDigest[:]),
		"expires_at": time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339Nano),
		"trace_id":   httpx.GetTraceID(ctx), "command": json.RawMessage(requestJSON),
	})
	if _, err := db.ExecContext(ctx, `
		INSERT INTO probe_operation_outbox
			(event_id,operation_id,tenant_id,event_type,partition_key,aggregate_version,payload)
		VALUES ($1::uuid,$2::uuid,$3,'traffic.probe.v2.OperationRequested',$4,$5,$6::jsonb)`,
		eventID, operationID, tenantID, tenantID+":"+probeID, commandRevision, string(payload)); err != nil {
		return "", err
	}
	return operationID, nil
}

// canonicalProbeCommandJSON makes command_hash independent of Go struct field
// order and PostgreSQL jsonb object ordering. Rust Agent validation uses the
// same recursively key-sorted JSON representation.
func canonicalProbeCommandJSON(value interface{}) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized interface{}
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

func (h *SystemHandler) insertProbeOperationWithAudit(
	ctx context.Context,
	tenantID, probeID, operationType string,
	request, result interface{},
	auditAction, objectType, objectID string,
	auditDetail map[string]interface{},
	r *http.Request,
) (string, error) {
	ctx = withProbeOperationIdempotency(ctx, r)
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	operationID, err := h.insertProbeOperation(ctx, tx, tenantID, probeID, operationType, request, result)
	if err != nil {
		return "", err
	}
	if values, ok := result.(map[string]interface{}); ok {
		if replay, _ := values["idempotent_replay"].(bool); replay {
			if err := tx.Commit(); err != nil {
				return "", err
			}
			return operationID, nil
		}
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

func writeProbeOperationAccepted(w http.ResponseWriter, ctx context.Context, result map[string]interface{}) {
	snapshotID := strings.TrimSpace(fmt.Sprint(result["operation_id"]))
	if snapshotID == "<nil>" {
		snapshotID = ""
	}
	if snapshotID == "" {
		if ids, ok := result["operation_ids"].([]string); ok && len(ids) > 0 {
			snapshotID = ids[0]
		}
	}
	httpx.JSONContractAccepted(w, ctx, result, httpx.ContractMeta{
		ContractVersion:  1,
		SnapshotID:       snapshotID,
		TraceID:          httpx.GetTraceID(ctx),
		Partial:          false,
		MissingSections:  []string{},
		SourceWatermarks: map[string]string{"postgresql.probe_operations.revision": fmt.Sprint(result["command_revision"])},
	})
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

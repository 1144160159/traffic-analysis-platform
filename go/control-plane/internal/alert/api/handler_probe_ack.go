package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type probeOperationDTO struct {
	OperationID          string                 `json:"operation_id"`
	TenantID             string                 `json:"tenant_id"`
	ProbeID              string                 `json:"probe_id"`
	OperationType        string                 `json:"operation_type"`
	Status               string                 `json:"status"`
	CommandRevision      int64                  `json:"command_revision"`
	StateRevision        int64                  `json:"state_revision"`
	DesiredVersion       string                 `json:"desired_version"`
	CommandHash          string                 `json:"command_hash"`
	ReportedVersion      string                 `json:"reported_version"`
	ReportedHash         string                 `json:"reported_hash"`
	AgentVersion         string                 `json:"agent_version"`
	AckError             string                 `json:"ack_error"`
	RequestedBy          string                 `json:"requested_by"`
	Request              map[string]interface{} `json:"request"`
	Result               map[string]interface{} `json:"result"`
	TraceID              string                 `json:"trace_id"`
	ExpiresAt            time.Time              `json:"-"`
	AcknowledgedAt       sql.NullTime           `json:"-"`
	CreatedAt            time.Time              `json:"-"`
	UpdatedAt            time.Time              `json:"-"`
	OutboxPublished      bool                   `json:"outbox_published"`
	ControlEventID       string                 `json:"control_event_id"`
	ControlPublishedAt   sql.NullTime           `json:"-"`
	LifecycleEventID     string                 `json:"lifecycle_event_id,omitempty"`
	LifecyclePublishedAt sql.NullTime           `json:"-"`
	AgentAckEventID      string                 `json:"agent_ack_event_id,omitempty"`
}

func (operation probeOperationDTO) response() map[string]interface{} {
	status := operation.Status
	if (status == "accepted" || status == "delivered") && time.Now().UTC().After(operation.ExpiresAt) {
		status = "expired"
	}
	response := map[string]interface{}{
		"operation_id": operation.OperationID, "tenant_id": operation.TenantID,
		"probe_id": operation.ProbeID, "operation_type": operation.OperationType,
		"status": status, "command_revision": operation.CommandRevision,
		"state_revision": operation.StateRevision, "desired_version": operation.DesiredVersion,
		"command_hash": operation.CommandHash, "reported_version": operation.ReportedVersion,
		"reported_hash": operation.ReportedHash, "agent_version": operation.AgentVersion,
		"ack_error": operation.AckError, "requested_by": operation.RequestedBy,
		"request": operation.Request, "result": operation.Result, "trace_id": operation.TraceID,
		"expires_at": operation.ExpiresAt.UnixMilli(), "created_at": operation.CreatedAt.UnixMilli(),
		"updated_at": operation.UpdatedAt.UnixMilli(), "outbox_published": operation.OutboxPublished,
		"control_event_id": operation.ControlEventID,
	}
	if operation.AcknowledgedAt.Valid {
		response["acknowledged_at"] = operation.AcknowledgedAt.Time.UnixMilli()
	}
	if operation.ControlPublishedAt.Valid {
		response["control_published_at"] = operation.ControlPublishedAt.Time.UnixMilli()
	}
	if operation.LifecycleEventID != "" {
		response["lifecycle_event_id"] = operation.LifecycleEventID
	}
	if operation.LifecyclePublishedAt.Valid {
		response["lifecycle_published_at"] = operation.LifecyclePublishedAt.Time.UnixMilli()
	}
	if operation.AgentAckEventID != "" {
		response["agent_ack_event_id"] = operation.AgentAckEventID
	}
	return response
}

type probeOperationAckRequest struct {
	CommandRevision int64                  `json:"command_revision"`
	ReportedVersion string                 `json:"reported_version"`
	ReportedHash    string                 `json:"reported_hash"`
	AgentVersion    string                 `json:"agent_version"`
	Applied         bool                   `json:"applied"`
	Error           string                 `json:"error"`
	AcknowledgedAt  string                 `json:"acknowledged_at"`
	Detail          map[string]interface{} `json:"detail"`
}

type ProbeOperationAckInput struct {
	CommandRevision int64                  `json:"command_revision"`
	ReportedVersion string                 `json:"reported_version"`
	ReportedHash    string                 `json:"reported_hash"`
	AgentVersion    string                 `json:"agent_version"`
	Applied         bool                   `json:"applied"`
	Error           string                 `json:"error"`
	AcknowledgedAt  time.Time              `json:"acknowledged_at"`
	Detail          map[string]interface{} `json:"detail"`
}

var (
	ErrProbeOperationNotFound         = errors.New("probe operation not found")
	ErrProbeAckRevisionMismatch       = errors.New("probe ACK revision mismatch")
	ErrProbeAckPersistenceUnavailable = errors.New("probe ACK persistence unavailable")
)

const probeOperationSelectSQL = `
	SELECT o.operation_id::text,o.tenant_id,o.probe_id,o.operation_type,o.status,
	       o.command_revision,o.state_revision,o.desired_version,o.command_hash,
	       o.reported_version,o.reported_hash,o.agent_version,o.ack_error,o.requested_by,
	       o.request::text,o.result::text,o.trace_id,o.expires_at,o.acknowledged_at,
	       o.created_at,o.updated_at,
	       CASE
	         WHEN o.status IN ('completed','failed','expired','stale') THEN EXISTS (
	           SELECT 1 FROM probe_operation_outbox x
	           WHERE x.operation_id=o.operation_id
	             AND x.event_type='traffic.probe.v2.OperationAcknowledged' AND x.published=true
	         )
	         ELSE EXISTS (
	           SELECT 1 FROM probe_operation_outbox x
	           WHERE x.operation_id=o.operation_id
	             AND x.event_type='traffic.probe.v2.OperationRequested' AND x.published=true
	           )
	       END,
	       COALESCE((
	         SELECT x.event_id::text FROM probe_operation_outbox x
	         WHERE x.operation_id=o.operation_id
	           AND x.event_type='traffic.probe.v2.OperationRequested'
	         ORDER BY x.created_at DESC LIMIT 1
	       ),''),
	       (
	         SELECT x.published_at FROM probe_operation_outbox x
	         WHERE x.operation_id=o.operation_id
	           AND x.event_type='traffic.probe.v2.OperationRequested'
	         ORDER BY x.created_at DESC LIMIT 1
	       ),
	       COALESCE((
	         SELECT x.event_id::text FROM probe_operation_outbox x
	         WHERE x.operation_id=o.operation_id
	           AND x.event_type='traffic.probe.v2.OperationAcknowledged'
	         ORDER BY x.created_at DESC LIMIT 1
	       ),''),
	       (
	         SELECT x.published_at FROM probe_operation_outbox x
	         WHERE x.operation_id=o.operation_id
	           AND x.event_type='traffic.probe.v2.OperationAcknowledged'
	         ORDER BY x.created_at DESC LIMIT 1
	       ),
	       COALESCE(o.result->>'source_event_id','')
	FROM probe_operations o `

type probeOperationScanner interface {
	Scan(...interface{}) error
}

func scanProbeOperation(scanner probeOperationScanner) (probeOperationDTO, error) {
	var operation probeOperationDTO
	var requestJSON, resultJSON []byte
	err := scanner.Scan(
		&operation.OperationID, &operation.TenantID, &operation.ProbeID, &operation.OperationType,
		&operation.Status, &operation.CommandRevision, &operation.StateRevision,
		&operation.DesiredVersion, &operation.CommandHash, &operation.ReportedVersion,
		&operation.ReportedHash, &operation.AgentVersion, &operation.AckError,
		&operation.RequestedBy, &requestJSON, &resultJSON, &operation.TraceID,
		&operation.ExpiresAt, &operation.AcknowledgedAt, &operation.CreatedAt,
		&operation.UpdatedAt, &operation.OutboxPublished,
		&operation.ControlEventID, &operation.ControlPublishedAt,
		&operation.LifecycleEventID, &operation.LifecyclePublishedAt,
		&operation.AgentAckEventID,
	)
	if err != nil {
		return probeOperationDTO{}, err
	}
	operation.Request = map[string]interface{}{}
	operation.Result = map[string]interface{}{}
	_ = json.Unmarshal(requestJSON, &operation.Request)
	_ = json.Unmarshal(resultJSON, &operation.Result)
	return operation, nil
}

func (h *SystemHandler) GetProbeOperation(w http.ResponseWriter, r *http.Request) {
	if !h.probeOperationAckV2 {
		httpx.JSONError(w, r.Context(), http.StatusNotFound, "FEATURE_DISABLED", "probe operation lifecycle is disabled")
		return
	}
	if !h.requireProbeReadPermission(w, r) {
		return
	}
	ctx := r.Context()
	if h.pgDB == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "probe operation persistence is unavailable")
		return
	}
	operationID := strings.TrimSpace(mux.Vars(r)["operation_id"])
	if operationID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "operation_id is required")
		return
	}
	operation, err := scanProbeOperation(h.pgDB.QueryRowContext(ctx, probeOperationSelectSQL+`
		WHERE o.tenant_id=$1 AND o.operation_id=$2::uuid`, writeTenantID(r), operationID))
	if err == sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusNotFound, "OPERATION_NOT_FOUND", "probe operation not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load probe operation")
		return
	}
	writeProbeOperationState(w, ctx, operation)
}

// AcknowledgeProbeOperation is the authenticated control-channel callback.
// It records late receipts, but only the newest matching command revision can
// advance the probe's reported state.
func (h *SystemHandler) AcknowledgeProbeOperation(w http.ResponseWriter, r *http.Request) {
	if !h.probeOperationAckV2 {
		httpx.JSONError(w, r.Context(), http.StatusNotFound, "FEATURE_DISABLED", "probe operation ACK is disabled")
		return
	}
	if !h.requireProbeAckIdentity(w, r) {
		return
	}
	ctx := r.Context()
	if h.pgDB == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "probe operation persistence is unavailable")
		return
	}
	probeID := strings.TrimSpace(mux.Vars(r)["id"])
	operationID := strings.TrimSpace(mux.Vars(r)["operation_id"])
	if probeID == "" || operationID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "probe id and operation_id are required")
		return
	}
	var request probeOperationAckRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid probe ACK payload")
		return
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "probe ACK payload must contain exactly one JSON object")
		return
	}
	request.ReportedVersion = strings.TrimSpace(request.ReportedVersion)
	request.ReportedHash = strings.TrimSpace(request.ReportedHash)
	request.AgentVersion = strings.TrimSpace(request.AgentVersion)
	request.Error = strings.TrimSpace(request.Error)
	if request.CommandRevision <= 0 || request.ReportedHash == "" || request.AgentVersion == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "command_revision, reported_hash and agent_version are required")
		return
	}
	acknowledgedAt := time.Now().UTC()
	if request.AcknowledgedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, request.AcknowledgedAt)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "acknowledged_at must be RFC3339")
			return
		}
		acknowledgedAt = parsed.UTC()
	}
	tenantID := writeTenantID(r)
	input := ProbeOperationAckInput{
		CommandRevision: request.CommandRevision,
		ReportedVersion: request.ReportedVersion,
		ReportedHash:    request.ReportedHash,
		AgentVersion:    request.AgentVersion,
		Applied:         request.Applied,
		Error:           request.Error,
		AcknowledgedAt:  acknowledgedAt,
		Detail:          request.Detail,
	}
	if err := h.applyProbeOperationAck(ctx, tenantID, probeID, operationID, "", input, r); err != nil {
		switch {
		case errors.Is(err, ErrProbeOperationNotFound):
			httpx.JSONError(w, ctx, http.StatusNotFound, "OPERATION_NOT_FOUND", err.Error())
		case errors.Is(err, ErrProbeAckRevisionMismatch):
			httpx.JSONError(w, ctx, http.StatusConflict, "ACK_REVISION_MISMATCH", err.Error())
		default:
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", err.Error())
		}
		return
	}
	operation, err := scanProbeOperation(h.pgDB.QueryRowContext(ctx, probeOperationSelectSQL+`
		WHERE o.tenant_id=$1 AND o.operation_id=$2::uuid`, tenantID, operationID))
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to reload probe ACK result")
		return
	}
	writeProbeOperationState(w, ctx, operation)
}

// ApplyProbeOperationAck is the Kafka-facing entrypoint. The same transaction
// implementation is used by the authenticated HTTP callback.
func (h *SystemHandler) ApplyProbeOperationAck(
	ctx context.Context,
	tenantID, probeID, operationID, sourceEventID string,
	input ProbeOperationAckInput,
) error {
	return h.applyProbeOperationAck(ctx, tenantID, probeID, operationID, sourceEventID, input, nil)
}

func (h *SystemHandler) applyProbeOperationAck(
	ctx context.Context,
	tenantID, probeID, operationID, sourceEventID string,
	input ProbeOperationAckInput,
	auditRequest *http.Request,
) error {
	if h.pgDB == nil {
		return ErrProbeAckPersistenceUnavailable
	}
	tenantID, probeID, operationID = strings.TrimSpace(tenantID), strings.TrimSpace(probeID), strings.TrimSpace(operationID)
	input.ReportedVersion = strings.TrimSpace(input.ReportedVersion)
	input.ReportedHash = strings.TrimSpace(input.ReportedHash)
	input.AgentVersion = strings.TrimSpace(input.AgentVersion)
	input.Error = strings.TrimSpace(input.Error)
	if tenantID == "" || probeID == "" || operationID == "" ||
		input.CommandRevision <= 0 || input.ReportedHash == "" || input.AgentVersion == "" {
		return fmt.Errorf("invalid probe ACK contract")
	}
	if input.AcknowledgedAt.IsZero() {
		input.AcknowledgedAt = time.Now().UTC()
	} else {
		input.AcknowledgedAt = input.AcknowledgedAt.UTC()
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin probe ACK transaction: %w", err)
	}
	defer tx.Rollback()
	operation, err := scanProbeOperation(tx.QueryRowContext(ctx, probeOperationSelectSQL+`
		WHERE o.tenant_id=$1 AND o.probe_id=$2 AND o.operation_id=$3::uuid
		FOR UPDATE OF o`, tenantID, probeID, operationID))
	if err == sql.ErrNoRows {
		return ErrProbeOperationNotFound
	}
	if err != nil {
		return fmt.Errorf("lock probe operation: %w", err)
	}
	if input.CommandRevision != operation.CommandRevision {
		return ErrProbeAckRevisionMismatch
	}
	var receiptExists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM probe_operation_ack_receipts WHERE operation_id=$1::uuid)`,
		operationID).Scan(&receiptExists); err != nil {
		return fmt.Errorf("check duplicate probe ACK: %w", err)
	}
	if receiptExists {
		return tx.Commit()
	}

	var newestAppliedRevision int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(max(command_revision),0) FROM probe_operations
		WHERE tenant_id=$1 AND probe_id=$2 AND status='completed'`,
		tenantID, probeID).Scan(&newestAppliedRevision); err != nil {
		return fmt.Errorf("validate probe ACK ordering: %w", err)
	}
	statusValue := "completed"
	accepted := true
	rejectionReason := ""
	switch {
	case time.Now().UTC().After(operation.ExpiresAt):
		statusValue, accepted, rejectionReason = "expired", false, "operation expired before ACK"
	case newestAppliedRevision > operation.CommandRevision:
		statusValue, accepted, rejectionReason = "stale", false, "newer command revision is already applied"
	case operation.DesiredVersion != "" && input.ReportedVersion != operation.DesiredVersion:
		statusValue, accepted, rejectionReason = "failed", false, "reported version does not match desired version"
	case !input.Applied:
		statusValue, accepted = "failed", true
		if input.Error == "" {
			input.Error = "agent reported operation failure"
		}
	}
	if input.Error == "" && rejectionReason != "" {
		input.Error = rejectionReason
	}
	ackID := sourceEventID
	if _, err := uuid.Parse(ackID); err != nil {
		ackID = uuid.NewString()
	}
	rawInput, _ := json.Marshal(input)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO probe_operation_ack_receipts
			(ack_id,operation_id,tenant_id,probe_id,command_revision,reported_version,
			 reported_hash,agent_version,applied,error,acknowledged_at,accepted,rejection_reason,payload)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb)`,
		ackID, operationID, tenantID, probeID, input.CommandRevision,
		input.ReportedVersion, input.ReportedHash, input.AgentVersion,
		input.Applied, input.Error, input.AcknowledgedAt, accepted, rejectionReason,
		string(rawInput)); err != nil {
		return fmt.Errorf("persist probe ACK receipt: %w", err)
	}
	resultPatch, _ := json.Marshal(map[string]interface{}{
		"ack_id": ackID, "applied": input.Applied, "accepted": accepted,
		"reported_version": input.ReportedVersion, "reported_hash": input.ReportedHash,
		"agent_version": input.AgentVersion, "error": input.Error, "detail": input.Detail,
		"rejection_reason": rejectionReason, "acknowledged_at": input.AcknowledgedAt.Format(time.RFC3339Nano),
		"source_event_id": sourceEventID,
	})
	var stateRevision int64
	if err := tx.QueryRowContext(ctx, `
		UPDATE probe_operations
		SET status=$2,state_revision=state_revision+1,reported_version=$3,reported_hash=$4,
		    agent_version=$5,ack_error=$6,acknowledged_at=$7,
		    result=COALESCE(result,'{}'::jsonb)||$8::jsonb,updated_at=now()
		WHERE operation_id=$1::uuid
		RETURNING state_revision`,
		operationID, statusValue, input.ReportedVersion, input.ReportedHash,
		input.AgentVersion, input.Error, input.AcknowledgedAt, string(resultPatch)).Scan(&stateRevision); err != nil {
		return fmt.Errorf("advance probe operation state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO probe_operation_history
			(operation_id,tenant_id,state_revision,from_status,to_status,detail)
		VALUES ($1::uuid,$2,$3,$4,$5,$6::jsonb)`,
		operationID, tenantID, stateRevision, operation.Status, statusValue, string(resultPatch)); err != nil {
		return fmt.Errorf("persist probe operation history: %w", err)
	}
	if statusValue == "completed" {
		reportedState, _ := json.Marshal(map[string]interface{}{
			"reported_config_version": input.ReportedVersion,
			"reported_config_hash":    input.ReportedHash, "reported_agent_version": input.AgentVersion,
			"reported_operation_revision": input.CommandRevision, "reported_operation_id": operationID,
			"reported_at": input.AcknowledgedAt.Format(time.RFC3339Nano),
		})
		if _, err := tx.ExecContext(ctx, `
			UPDATE probes SET hardware_info=COALESCE(hardware_info,'{}'::jsonb)||$3::jsonb,updated_at=now()
			WHERE tenant_id=$1 AND probe_id=$2`, tenantID, probeID, string(reportedState)); err != nil {
			return fmt.Errorf("persist probe reported state: %w", err)
		}
	}
	lifecycleEventID := uuid.NewString()
	eventPayload, _ := json.Marshal(map[string]interface{}{
		"event_id": lifecycleEventID, "event_type": probeOperationAcknowledgedEvent,
		"tenant_id": tenantID, "probe_id": probeID, "operation_id": operationID,
		"command_revision": input.CommandRevision, "state_revision": stateRevision,
		"revision": stateRevision,
		"status":   statusValue, "accepted": accepted, "reported_version": input.ReportedVersion,
		"reported_hash": input.ReportedHash, "agent_version": input.AgentVersion,
		"trace_id": operation.TraceID, "source_event_id": sourceEventID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO probe_operation_outbox
			(event_id,operation_id,tenant_id,event_type,partition_key,aggregate_version,payload)
		VALUES ($1::uuid,$2::uuid,$3,'traffic.probe.v2.OperationAcknowledged',$4,$5,$6::jsonb)`,
		lifecycleEventID, operationID, tenantID, tenantID+":"+probeID, stateRevision, string(eventPayload)); err != nil {
		return fmt.Errorf("enqueue probe ACK lifecycle event: %w", err)
	}
	writer := NewAlertActionAuditWriter(h.pgDB, h.logger)
	if err := writer.recordWithExecutor(ctx, tx, auditRequest, AlertActionAuditRecord{
		Action: "PROBE_OPERATION_ACKNOWLEDGED", ObjectType: "probe_operation", ObjectID: operationID,
		TenantID: tenantID, UserID: httpx.GetUserID(ctx), Result: statusValue,
		Detail: map[string]interface{}{
			"probe_id": probeID, "command_revision": input.CommandRevision,
			"state_revision": stateRevision, "accepted": accepted, "ack_id": ackID,
			"reported_version": input.ReportedVersion, "reported_hash": input.ReportedHash,
			"agent_version": input.AgentVersion, "rejection_reason": rejectionReason,
			"source_event_id": sourceEventID,
		},
	}); err != nil {
		return fmt.Errorf("persist probe ACK audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit probe ACK transaction: %w", err)
	}
	return nil
}

func writeProbeOperationState(w http.ResponseWriter, ctx context.Context, operation probeOperationDTO) {
	effectiveStatus := operation.Status
	if (effectiveStatus == "accepted" || effectiveStatus == "delivered") && time.Now().UTC().After(operation.ExpiresAt) {
		effectiveStatus = "expired"
	}
	httpx.JSONContractSuccess(w, ctx, operation.response(), httpx.ContractMeta{
		ContractVersion: 1,
		SnapshotID:      operation.OperationID,
		AsOf:            operation.UpdatedAt.UTC().Format(time.RFC3339Nano),
		TraceID:         operation.TraceID,
		Partial:         effectiveStatus == "accepted" || effectiveStatus == "delivered" || effectiveStatus == "stale",
		MissingSections: func() []string {
			if effectiveStatus == "accepted" || effectiveStatus == "delivered" {
				return []string{"agent_ack"}
			}
			return []string{}
		}(),
		SourceWatermarks: map[string]string{
			"postgresql.probe_operations.revision": fmt.Sprint(operation.StateRevision),
			"kafka.probe.control.event_id":         probeOutboxWatermark(operation.ControlEventID, operation.ControlPublishedAt),
			"kafka.probe.acks.event_id":            nonEmpty(operation.AgentAckEventID, "not_received"),
			"kafka.probe.events.event_id":          probeOutboxWatermark(operation.LifecycleEventID, operation.LifecyclePublishedAt),
		},
	})
}

func probeOutboxWatermark(eventID string, publishedAt sql.NullTime) string {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return "not_created"
	}
	if !publishedAt.Valid {
		return eventID + "@outbox_pending"
	}
	return eventID + "@" + publishedAt.Time.UTC().Format(time.RFC3339Nano)
}

type probeBoundClaims interface {
	GetProbeID() string
}

func (h *SystemHandler) requireProbeAckIdentity(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if !hasSystemPermission(ctx, authmodel.ScopeProbeIngest) {
		httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: probe:ingest agent token required")
		return false
	}
	claims := httpx.GetClaims(ctx)
	bound, ok := claims.(probeBoundClaims)
	claimedProbeID := ""
	if ok {
		claimedProbeID = strings.TrimSpace(bound.GetProbeID())
	}
	pathProbeID := strings.TrimSpace(mux.Vars(r)["id"])
	if claimedProbeID == "" || claimedProbeID != pathProbeID {
		httpx.JSONError(w, ctx, http.StatusForbidden, "PROBE_IDENTITY_MISMATCH", "authenticated probe identity does not match ACK target")
		return false
	}
	return true
}

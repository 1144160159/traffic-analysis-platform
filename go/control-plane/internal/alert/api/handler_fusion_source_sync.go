package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/fusion"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
)

type fusionSourceSyncRequest struct {
	WindowStart           time.Time `json:"window_start"`
	WindowEnd             time.Time `json:"window_end"`
	ExpectedSourceVersion *int64    `json:"expected_source_version,omitempty"`
	IdempotencyKey        string    `json:"idempotency_key,omitempty"`
	Reason                string    `json:"reason"`
}

type fusionSourceSyncReceipt struct {
	JobID             string `json:"job_id"`
	EventID           string `json:"event_id"`
	SourceID          string `json:"source_id"`
	Status            string `json:"status"`
	Revision          int64  `json:"revision"`
	TraceID           string `json:"trace_id"`
	OutboxStatus      string `json:"outbox_status"`
	SourceSnapshotID  string `json:"source_snapshot_id,omitempty"`
	DataSnapshotID    string `json:"data_snapshot_id,omitempty"`
	FeatureSnapshotID string `json:"feature_snapshot_id,omitempty"`
	Replayed          bool   `json:"replayed"`
}

func (h *SystemHandler) syncFusionSourceV1(
	w http.ResponseWriter,
	r *http.Request,
	tenantID string,
	sourceID string,
) {
	ctx := r.Context()
	sourceKind, supported := fusion.SourceKind(sourceID)
	if !supported {
		httpx.JSONError(w, ctx, http.StatusConflict, "SOURCE_NOT_VERSIONED", "source is not part of the four-source fusion v1 contract")
		return
	}
	if h.fusionReadinessGate == nil || h.fusionCommandPublish == nil || len(h.fusionCandidateSHA256) != 64 {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "FUSION_PIPELINE_NOT_READY", "fusion authority is not bound to a ready projection candidate")
		return
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	var request fusionSourceSyncRequest
	if err := decoder.Decode(&request); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "a versioned source-sync request body is required")
		return
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "request must contain one JSON object")
		return
	}
	request.IdempotencyKey = strings.TrimSpace(nonEmpty(r.Header.Get("Idempotency-Key"), request.IdempotencyKey))
	request.Reason = strings.TrimSpace(request.Reason)
	if request.IdempotencyKey == "" || len(request.IdempotencyKey) > 200 || request.Reason == "" ||
		request.WindowStart.IsZero() || request.WindowEnd.IsZero() || !request.WindowStart.Before(request.WindowEnd) ||
		request.WindowEnd.Sub(request.WindowStart) > 31*24*time.Hour ||
		(request.ExpectedSourceVersion != nil && *request.ExpectedSourceVersion < 0) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "idempotency_key, reason and a valid window of at most 31 days are required")
		return
	}
	requestedBy := strings.TrimSpace(httpx.GetUserID(ctx))
	if _, err := uuid.Parse(requestedBy); err != nil {
		httpx.JSONError(w, ctx, http.StatusForbidden, "AUTHORITY_REQUIRED", "authenticated user identity is required")
		return
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	idempotencyInput := map[string]interface{}{
		"tenant_id": tenantID, "source_id": sourceID, "source_kind": sourceKind,
		"window_start":            request.WindowStart.UTC().Format(time.RFC3339Nano),
		"window_end":              request.WindowEnd.UTC().Format(time.RFC3339Nano),
		"expected_source_version": request.ExpectedSourceVersion,
		"requested_by":            requestedBy, "reason": request.Reason,
	}
	idempotencySHA, err := fusionCanonicalSHA256(idempotencyInput)
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
	if err := h.fusionReadinessGate.AssertReadyTx(ctx, tx, h.fusionCandidateSHA256); err != nil {
		if errors.Is(err, fusion.ErrProjectionNotReady) {
			httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "FUSION_CONSUMER_NOT_READY", err.Error())
		} else {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		}
		return
	}
	if existing, found, err := loadFusionSourceSyncReplayTx(ctx, tx, tenantID, request.IdempotencyKey, idempotencySHA); err != nil {
		if errors.Is(err, errFusionIdempotencyConflict) {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_CONFLICT", err.Error())
		} else {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		}
		return
	} else if found {
		if err := tx.Commit(); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		existing.Replayed = true
		httpx.JSON(w, http.StatusAccepted, map[string]interface{}{"success": true, "data": existing})
		return
	}
	now := time.Now().UTC()
	jobID, eventID := uuid.NewString(), uuid.NewString()
	command := fusion.SourceSyncCommand{
		EventID: eventID, EventType: fusion.SourceSyncEventType, SchemaVersion: 1,
		AggregateType: "source_sync_job", AggregateID: jobID, AggregateVersion: 1,
		PartitionKey: tenantID + ":" + jobID, TenantID: tenantID, JobID: jobID,
		SourceID: sourceID, SourceKind: sourceKind, WindowStart: request.WindowStart.UTC(),
		WindowEnd: request.WindowEnd.UTC(), ExpectedSourceVersion: request.ExpectedSourceVersion,
		RequestedBy: requestedBy, Reason: request.Reason, TraceID: traceID, OccurredAt: now,
	}
	payload, err := json.Marshal(command)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	payloadHash := sha256.Sum256(payload)
	payloadSHA := hex.EncodeToString(payloadHash[:])
	_, err = tx.ExecContext(ctx, `INSERT INTO fusion_source_sync_jobs (
		job_id,tenant_id,source_id,source_kind,idempotency_key,idempotency_request_sha256,request_sha256,
		requested_window_start,requested_window_end,expected_source_version,status,revision,reason,requested_by,trace_id
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'queued',1,$11,$12,$13)`, jobID, tenantID, sourceID,
		sourceKind, request.IdempotencyKey, idempotencySHA, payloadSHA, command.WindowStart, command.WindowEnd,
		request.ExpectedSourceVersion, request.Reason, requestedBy, traceID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO fusion_projection_outbox (
		event_id,tenant_id,aggregate_type,aggregate_id,aggregate_version,event_type,partition_key,
		payload,payload_sha256,publish_state,trace_id
	) VALUES ($1,$2,'source_sync_job',$3,1,$4,$5,$6::jsonb,$7,'PENDING',$8)`,
		eventID, tenantID, jobID, fusion.SourceSyncEventType, command.PartitionKey, string(payload), payloadSHA, traceID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if err := insertFusionAuditTx(ctx, tx, tenantID, requestedBy, "FUSION_SOURCE_SYNC_REQUESTED", "fusion_source_sync_job", jobID,
		map[string]interface{}{
			"event_id": eventID, "source_id": sourceID, "source_kind": sourceKind,
			"window_start":            command.WindowStart.Format(time.RFC3339Nano),
			"window_end":              command.WindowEnd.Format(time.RFC3339Nano),
			"expected_source_version": request.ExpectedSourceVersion,
			"candidate_sha256":        h.fusionCandidateSHA256, "status": "queued", "outbox_status": "pending",
		}, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]interface{}{"success": true, "data": fusionSourceSyncReceipt{
		JobID: jobID, EventID: eventID, SourceID: sourceID, Status: "queued", Revision: 1,
		TraceID: traceID, OutboxStatus: "pending",
	}})
}

var errFusionIdempotencyConflict = errors.New("fusion source-sync idempotency key was reused with different request bytes")

func loadFusionSourceSyncReplayTx(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	idempotencyKey string,
	idempotencySHA string,
) (fusionSourceSyncReceipt, bool, error) {
	var receipt fusionSourceSyncReceipt
	var storedSHA string
	var sourceSnapshotID, dataSnapshotID, featureSnapshotID sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT j.job_id::text,o.event_id::text,j.source_id,j.status,j.revision,j.trace_id,
		j.idempotency_request_sha256,j.result_source_snapshot_id::text,j.result_data_snapshot_id::text,
		j.result_feature_snapshot_id::text,o.publish_state
		FROM fusion_source_sync_jobs j JOIN fusion_projection_outbox o
		  ON o.tenant_id=j.tenant_id AND o.aggregate_type='source_sync_job' AND o.aggregate_id=j.job_id
		 AND o.aggregate_version=1 AND o.event_type=$1
		WHERE j.tenant_id=$2 AND j.idempotency_key=$3 FOR UPDATE OF j,o`,
		fusion.SourceSyncEventType, tenantID, idempotencyKey).Scan(
		&receipt.JobID, &receipt.EventID, &receipt.SourceID, &receipt.Status, &receipt.Revision, &receipt.TraceID,
		&storedSHA, &sourceSnapshotID, &dataSnapshotID, &featureSnapshotID, &receipt.OutboxStatus,
	)
	if err == sql.ErrNoRows {
		return fusionSourceSyncReceipt{}, false, nil
	}
	if err != nil {
		return fusionSourceSyncReceipt{}, false, fmt.Errorf("load fusion source-sync replay: %w", err)
	}
	if storedSHA != idempotencySHA {
		return fusionSourceSyncReceipt{}, false, errFusionIdempotencyConflict
	}
	receipt.SourceSnapshotID, receipt.DataSnapshotID = sourceSnapshotID.String, dataSnapshotID.String
	receipt.FeatureSnapshotID = featureSnapshotID.String
	return receipt, true, nil
}

func fusionCanonicalSHA256(value interface{}) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal fusion request identity: %w", err)
	}
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}

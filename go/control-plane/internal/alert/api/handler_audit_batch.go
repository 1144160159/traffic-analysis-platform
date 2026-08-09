package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/protobuf/proto"
)

const (
	auditBatchOperationID = "ingestAuditLogBatch"
	auditBatchActionID    = "audit-batch-ingest"
	auditBatchMaxEvents   = 200
	auditBatchMaxBody     = 2 << 20
)

type auditBatchPublisher interface {
	SendProto(context.Context, string, proto.Message, ...commonkafka.MessageHeader) error
}

type auditBatchRequest struct {
	ActionID string                   `json:"action_id"`
	Reason   string                   `json:"reason"`
	Events   []auditBatchRequestEvent `json:"events"`
}

type auditBatchRequestEvent struct {
	EventID    string          `json:"event_id"`
	TenantID   string          `json:"tenant_id,omitempty"`
	UserID     string          `json:"user_id,omitempty"`
	Action     string          `json:"action"`
	ObjectType string          `json:"object_type,omitempty"`
	ObjectID   string          `json:"object_id,omitempty"`
	Detail     json.RawMessage `json:"detail,omitempty"`
	IPAddress  string          `json:"ip_addr,omitempty"`
	UserAgent  string          `json:"user_agent,omitempty"`
	CreatedAt  int64           `json:"created_at,omitempty"`
}

type normalizedAuditBatch struct {
	ActionID string                   `json:"action_id"`
	Reason   string                   `json:"reason"`
	TenantID string                   `json:"tenant_id"`
	Events   []auditBatchRequestEvent `json:"events"`
}

// IngestAuditLogBatch validates the entire authenticated-tenant batch before
// publishing one protobuf AuditLogBatch message. A 202 means Kafka acknowledged
// the immutable batch; it does not mean PostgreSQL or ClickHouse projection is
// complete. Final state is queried through the returned event status URLs.
func (h *SystemHandler) IngestAuditLogBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !hasAnySystemPermission(ctx, authmodel.ScopeAuditWrite, authmodel.ScopeAdminAll) {
		h.auditBatchError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: audit:write required", false, nil)
		return
	}
	tenantID := strings.TrimSpace(httpx.GetTenantID(ctx))
	if tenantID == "" {
		h.auditBatchError(w, r, http.StatusUnauthorized, "AUTHENTICATED_TENANT_REQUIRED", "authenticated tenant identity is required", false, nil)
		return
	}

	request, fieldErrors, err := decodeAndNormalizeAuditBatch(w, r, tenantID)
	if err != nil {
		h.auditBatchError(w, r, http.StatusBadRequest, "INVALID_AUDIT_BATCH", err.Error(), false, fieldErrors)
		return
	}
	if h.auditBatchPublisher == nil {
		h.auditBatchError(w, r, http.StatusServiceUnavailable, "AUDIT_BATCH_INGRESS_DISABLED", "audit batch ingress is not enabled for this candidate", true, nil)
		return
	}

	batch, jobID, eventIDs, err := buildAuditProtoBatch(request)
	if err != nil {
		h.auditBatchError(w, r, http.StatusBadRequest, "INVALID_AUDIT_BATCH", err.Error(), false, nil)
		return
	}
	digest := sha256.Sum256([]byte(strings.Join(eventIDs, "\n")))
	if err := h.auditBatchPublisher.SendProto(
		ctx,
		jobID,
		batch,
		commonkafka.MessageHeader{Key: "event_id", Value: jobID},
		commonkafka.MessageHeader{Key: "event_type", Value: "traffic.v1.AuditLogBatch"},
		commonkafka.MessageHeader{Key: "schema_version", Value: "1"},
		commonkafka.MessageHeader{Key: "tenant_id", Value: tenantID},
		commonkafka.MessageHeader{Key: "action_id", Value: request.ActionID},
		commonkafka.MessageHeader{Key: "reason", Value: request.Reason},
		commonkafka.MessageHeader{Key: "audit_batch_job_id", Value: jobID},
		commonkafka.MessageHeader{Key: "audit_event_count", Value: fmt.Sprint(len(eventIDs))},
		commonkafka.MessageHeader{Key: "audit_event_ids_sha256", Value: hex.EncodeToString(digest[:])},
	); err != nil {
		if h.logger != nil {
			h.logger.Warn("Audit batch Kafka acknowledgement failed")
		}
		h.auditBatchError(w, r, http.StatusServiceUnavailable, "AUDIT_BATCH_NOT_ACCEPTED", "audit batch was not acknowledged by Kafka", true, nil)
		return
	}

	statusURLs := make([]string, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		statusURLs = append(statusURLs, "/api/v1/audit/logs/"+url.PathEscape(eventID))
	}
	httpx.JSONContractAccepted(w, ctx, map[string]interface{}{
		"job_id":            jobID,
		"status":            "accepted",
		"final":             false,
		"accepted_count":    len(eventIDs),
		"event_ids":         eventIDs,
		"status_urls":       statusURLs,
		"compensation":      "redeliver the uncommitted batch or replay an approved DLQ range with a new repair identity",
		"consistency_state": "pending_materialization",
	}, httpx.ContractMeta{
		ContractVersion:  1,
		SchemaVersion:    1,
		SnapshotID:       jobID,
		ResultCode:       "ACCEPTED",
		OperationID:      auditBatchOperationID,
		TenantID:         tenantID,
		ProjectionStatus: "pending",
		SourceWatermarks: map[string]string{
			"kafka.audit.logs.offset":        "broker_acknowledged_offset_pending_observation",
			"postgresql.audit_logs.event_id": "pending",
			"clickhouse.audit.watermark":     "pending",
		},
	})
}

func decodeAndNormalizeAuditBatch(w http.ResponseWriter, r *http.Request, tenantID string) (normalizedAuditBatch, []httpx.FieldError, error) {
	r.Body = http.MaxBytesReader(w, r.Body, auditBatchMaxBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request auditBatchRequest
	if err := decoder.Decode(&request); err != nil {
		return normalizedAuditBatch{}, nil, fmt.Errorf("decode audit batch: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return normalizedAuditBatch{}, nil, fmt.Errorf("audit batch must contain exactly one JSON object")
	}
	request.ActionID = strings.TrimSpace(request.ActionID)
	request.Reason = strings.TrimSpace(request.Reason)
	fieldErrors := make([]httpx.FieldError, 0)
	if request.ActionID != auditBatchActionID {
		fieldErrors = append(fieldErrors, httpx.FieldError{Field: "action_id", Code: "INVALID_ACTION_ID", Message: "action_id must be audit-batch-ingest"})
	}
	if len(request.Reason) < 3 || len(request.Reason) > 1000 {
		fieldErrors = append(fieldErrors, httpx.FieldError{Field: "reason", Code: "INVALID_REASON", Message: "reason must contain 3 to 1000 characters"})
	}
	if len(request.Events) == 0 || len(request.Events) > auditBatchMaxEvents {
		fieldErrors = append(fieldErrors, httpx.FieldError{Field: "events", Code: "INVALID_BATCH_SIZE", Message: "events must contain 1 to 200 items"})
	}
	if len(fieldErrors) > 0 {
		return normalizedAuditBatch{}, fieldErrors, fmt.Errorf("audit batch validation failed")
	}

	events := make([]auditBatchRequestEvent, len(request.Events))
	seen := make(map[string]struct{}, len(request.Events))
	for index, source := range request.Events {
		event := source
		event.EventID = strings.TrimSpace(event.EventID)
		event.TenantID = strings.TrimSpace(event.TenantID)
		event.UserID = strings.TrimSpace(event.UserID)
		event.Action = strings.TrimSpace(event.Action)
		event.ObjectType = strings.TrimSpace(event.ObjectType)
		event.ObjectID = strings.TrimSpace(event.ObjectID)
		if event.EventID == "" || len(event.EventID) > 200 {
			fieldErrors = append(fieldErrors, httpx.FieldError{Field: fmt.Sprintf("events[%d].event_id", index), Code: "INVALID_EVENT_ID", Message: "event_id is required and must not exceed 200 characters"})
		}
		if _, duplicate := seen[event.EventID]; event.EventID != "" && duplicate {
			fieldErrors = append(fieldErrors, httpx.FieldError{Field: fmt.Sprintf("events[%d].event_id", index), Code: "DUPLICATE_EVENT_ID", Message: "event_id must be unique within the batch"})
		}
		seen[event.EventID] = struct{}{}
		if event.TenantID != "" && event.TenantID != tenantID {
			fieldErrors = append(fieldErrors, httpx.FieldError{Field: fmt.Sprintf("events[%d].tenant_id", index), Code: "TENANT_OVERRIDE_FORBIDDEN", Message: "event tenant must match the authenticated tenant"})
		}
		event.TenantID = tenantID
		if event.Action == "" || len(event.Action) > 200 {
			fieldErrors = append(fieldErrors, httpx.FieldError{Field: fmt.Sprintf("events[%d].action", index), Code: "INVALID_ACTION", Message: "action is required and must not exceed 200 characters"})
		}
		if event.ObjectType == "" {
			event.ObjectType = "unknown"
		}
		if len(event.Detail) == 0 || string(event.Detail) == "null" {
			event.Detail = json.RawMessage(`{}`)
		}
		if !json.Valid(event.Detail) {
			fieldErrors = append(fieldErrors, httpx.FieldError{Field: fmt.Sprintf("events[%d].detail", index), Code: "INVALID_DETAIL", Message: "detail must be valid JSON"})
		}
		if event.CreatedAt <= 0 {
			fieldErrors = append(fieldErrors, httpx.FieldError{Field: fmt.Sprintf("events[%d].created_at", index), Code: "INVALID_CREATED_AT", Message: "created_at must be a stable positive Unix millisecond timestamp"})
		}
		events[index] = event
	}
	if len(fieldErrors) > 0 {
		return normalizedAuditBatch{}, fieldErrors, fmt.Errorf("audit batch validation failed")
	}
	sort.Slice(events, func(i, j int) bool { return events[i].EventID < events[j].EventID })
	return normalizedAuditBatch{ActionID: request.ActionID, Reason: request.Reason, TenantID: tenantID, Events: events}, nil, nil
}

func buildAuditProtoBatch(request normalizedAuditBatch) (*pb.AuditLogBatch, string, []string, error) {
	canonical, err := json.Marshal(request)
	if err != nil {
		return nil, "", nil, fmt.Errorf("canonicalize audit batch: %w", err)
	}
	digest := sha256.Sum256(canonical)
	jobID := "audit-batch-" + hex.EncodeToString(digest[:16])
	eventIDs := make([]string, 0, len(request.Events))
	batch := &pb.AuditLogBatch{Events: make([]*pb.AuditLog, 0, len(request.Events))}
	for _, event := range request.Events {
		eventIDs = append(eventIDs, event.EventID)
		batch.Events = append(batch.Events, &pb.AuditLog{
			EventId: event.EventID, TenantId: request.TenantID, UserId: event.UserID,
			Action: event.Action, ObjectType: event.ObjectType, ObjectId: event.ObjectID,
			Detail: string(event.Detail), IpAddr: event.IPAddress, UserAgent: event.UserAgent,
			CreatedAt: event.CreatedAt,
		})
	}
	return batch, jobID, eventIDs, nil
}

func (h *SystemHandler) auditBatchError(w http.ResponseWriter, r *http.Request, status int, code, message string, retryable bool, fieldErrors []httpx.FieldError) {
	httpx.JSONContractError(w, r.Context(), status, code, message, httpx.ErrorOptions{
		Retryable: retryable, FieldErrors: fieldErrors, OperationID: auditBatchOperationID,
		ProjectionStatus: "rejected",
	})
}

package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"go.uber.org/zap"
)

const (
	alertBatchAssignmentContractVersion = 1
	alertBatchSelectionTTL              = 15 * time.Minute
	alertBatchMaximumItems              = 100
)

var (
	errAlertBatchSchemaMissing       = errors.New("alert batch assignment schema unavailable")
	errAlertBatchInvalidRequest      = errors.New("alert batch assignment request invalid")
	errAlertBatchIdempotencyConflict = errors.New("alert batch assignment idempotency conflict")
	errAlertBatchSelectionInvalid    = errors.New("alert batch selection is invalid or expired")
	errAlertBatchNotFound            = errors.New("alert batch assignment not found")
)

type AlertBatchAssignmentHandler struct {
	db                  *sql.DB
	logger              *zap.Logger
	enabled             bool
	selectionSigningKey []byte
	now                 func() time.Time
}

type AlertBatchSelectionItem struct {
	AlertID      string `json:"alert_id"`
	StateVersion int64  `json:"state_version"`
}

type AlertBatchSelectionRequest struct {
	SnapshotID string                    `json:"snapshot_id"`
	Items      []AlertBatchSelectionItem `json:"items"`
}

type AlertBatchSelectionReceipt struct {
	SelectionID     string    `json:"selection_id"`
	SelectionToken  string    `json:"selection_token"`
	SnapshotID      string    `json:"snapshot_id"`
	SelectionSHA256 string    `json:"selection_sha256"`
	ItemCount       int       `json:"item_count"`
	ExpiresAt       time.Time `json:"expires_at"`
	TraceID         string    `json:"trace_id"`
	Replayed        bool      `json:"replayed"`
}

type AlertBatchAssignmentRequest struct {
	SelectionToken string `json:"selection_token"`
	Assignee       string `json:"assignee"`
	Reason         string `json:"reason"`
}

type AlertBatchAssignmentReceipt struct {
	BatchID             string `json:"batch_id"`
	JobID               string `json:"job_id"`
	EventID             string `json:"event_id"`
	ActionID            string `json:"action_id"`
	Status              string `json:"status"`
	Revision            int64  `json:"revision"`
	SelectionID         string `json:"selection_id"`
	SelectionSnapshotID string `json:"selection_snapshot_id"`
	SelectionSHA256     string `json:"selection_sha256"`
	TotalCount          int    `json:"total_count"`
	AcceptedCount       int    `json:"accepted_count"`
	AppliedCount        int    `json:"applied_count"`
	ConflictedCount     int    `json:"conflicted_count"`
	ForbiddenCount      int    `json:"forbidden_count"`
	FailedCount         int    `json:"failed_count"`
	TraceID             string `json:"trace_id"`
	OutboxStatus        string `json:"outbox_status"`
	Replayed            bool   `json:"replayed"`
}

type AlertBatchAssignmentItemResult struct {
	AlertID               string    `json:"alert_id"`
	Position              int       `json:"position"`
	ExpectedStateVersion  int64     `json:"expected_state_version"`
	Status                string    `json:"status"`
	ItemRevision          int64     `json:"item_revision"`
	ResultingStateVersion int64     `json:"resulting_state_version,omitempty"`
	ErrorCode             string    `json:"error_code,omitempty"`
	ErrorMessage          string    `json:"error_message,omitempty"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type AlertBatchAssignmentJob struct {
	AlertBatchAssignmentReceipt
	Assignee    string                           `json:"assignee"`
	Reason      string                           `json:"reason"`
	RequestedBy string                           `json:"requested_by"`
	CreatedAt   time.Time                        `json:"created_at"`
	UpdatedAt   time.Time                        `json:"updated_at"`
	Items       []AlertBatchAssignmentItemResult `json:"items"`
}

type alertBatchCommandContext struct {
	TenantID       string
	ActorID        string
	IdempotencyKey string
	TraceID        string
	SourceIP       string
	UserAgent      string
}

func NewAlertBatchAssignmentHandler(db *sql.DB, logger *zap.Logger, enabled bool, selectionSigningSecret string) *AlertBatchAssignmentHandler {
	return &AlertBatchAssignmentHandler{db: db, logger: logger, enabled: enabled, selectionSigningKey: []byte(selectionSigningSecret), now: time.Now}
}

func (h *AlertBatchAssignmentHandler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/alerts/batches/selections", h.CreateSelection).Methods(http.MethodPost)
	router.HandleFunc("/alerts/batches/assign", h.CreateAssignment).Methods(http.MethodPost)
	router.HandleFunc("/alerts/batches/assign/{batch_id}", h.GetAssignment).Methods(http.MethodGet)
}

func (h *AlertBatchAssignmentHandler) VerifySchema(ctx context.Context) error {
	if h.db == nil {
		return errAlertBatchSchemaMissing
	}
	if len(h.selectionSigningKey) < 32 {
		return fmt.Errorf("%w: selection signing secret must contain at least 32 bytes", errAlertBatchSchemaMissing)
	}
	required := []string{
		"alert_assignment_selections", "alert_assignment_selection_requests", "alert_assignment_batches",
		"alert_assignment_batch_items", "alert_assignment_batch_history", "alert_assignment_batch_item_history",
		"alert_assignment_batch_outbox", "alert_assignment_batch_requests", "audit_logs",
	}
	for _, table := range required {
		var found sql.NullString
		if err := h.db.QueryRowContext(ctx, `SELECT to_regclass($1)::text`, table).Scan(&found); err != nil {
			return err
		}
		if !found.Valid || found.String == "" {
			return fmt.Errorf("%w: missing %s", errAlertBatchSchemaMissing, table)
		}
	}
	return nil
}

func (h *AlertBatchAssignmentHandler) CreateSelection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	command, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	var request AlertBatchSelectionRequest
	if !decodeStrictJSON(w, r, &request) {
		return
	}
	request.SnapshotID = strings.TrimSpace(request.SnapshotID)
	items, err := normalizeAlertBatchSelection(request.SnapshotID, request.Items)
	if err != nil {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_SELECTION", err.Error())
		return
	}
	request.Items = items
	receipt, err := h.createSelection(ctx, command, request)
	if err != nil {
		h.writeCommandError(w, ctx, err)
		return
	}
	h.writeSelectionCreated(w, ctx, receipt)
}

func (h *AlertBatchAssignmentHandler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	command, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	var request AlertBatchAssignmentRequest
	if !decodeStrictJSON(w, r, &request) {
		return
	}
	request.SelectionToken = strings.TrimSpace(request.SelectionToken)
	request.Assignee = strings.TrimSpace(request.Assignee)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.SelectionToken == "" || len(request.Assignee) > 128 || request.Assignee == "" || len([]rune(request.Reason)) < 4 || len([]rune(request.Reason)) > 1000 {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "selection_token, assignee and a reason of 4 to 1000 characters are required")
		return
	}
	receipt, err := h.createAssignment(ctx, command, request)
	if err != nil {
		h.writeCommandError(w, ctx, err)
		return
	}
	h.writeAssignmentAccepted(w, ctx, receipt)
}

func (h *AlertBatchAssignmentHandler) GetAssignment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	command, ok := h.authorize(w, r, false)
	if !ok {
		return
	}
	batchID := strings.TrimSpace(mux.Vars(r)["batch_id"])
	if _, err := uuid.Parse(batchID); err != nil {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_BATCH_ID", "batch_id must be a UUID")
		return
	}
	job, err := h.getAssignment(ctx, command.TenantID, batchID)
	if err != nil {
		if errors.Is(err, errAlertBatchNotFound) {
			h.writeError(w, ctx, http.StatusNotFound, "NOT_FOUND", "alert batch assignment not found")
			return
		}
		h.writeCommandError(w, ctx, err)
		return
	}
	h.writeAssignmentSuccess(w, ctx, job)
}

func (h *AlertBatchAssignmentHandler) authorize(w http.ResponseWriter, r *http.Request, requireIdempotencyKey bool) (alertBatchCommandContext, bool) {
	ctx := r.Context()
	if !h.enabled {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "FEATURE_DISABLED", "alert batch assignment v1 is disabled")
		return alertBatchCommandContext{}, false
	}
	if h.db == nil {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "PostgreSQL is required for alert batch assignment")
		return alertBatchCommandContext{}, false
	}
	tenantID, actorID, authenticated := authenticatedDashboardIdentity(ctx)
	if !authenticated {
		h.writeError(w, ctx, http.StatusUnauthorized, "UNAUTHENTICATED", "authenticated tenant and user are required")
		return alertBatchCommandContext{}, false
	}
	if !hasAlertWritePermission(ctx) && !hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		h.writeError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: alert:write required")
		return alertBatchCommandContext{}, false
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if requireIdempotencyKey && (len(idempotencyKey) < 16 || len(idempotencyKey) > 200) {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must contain 16 to 200 characters")
		return alertBatchCommandContext{}, false
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = strings.TrimSpace(httpx.GetRequestID(ctx))
	}
	if traceID == "" {
		traceID = uuid.NewString()
	}
	return alertBatchCommandContext{
		TenantID: tenantID, ActorID: actorID, IdempotencyKey: idempotencyKey,
		TraceID: traceID, SourceIP: requestSourceIP(r), UserAgent: r.UserAgent(),
	}, true
}

func decodeStrictJSON(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "request body must contain one JSON value")
		return false
	}
	return true
}

func normalizeAlertBatchSelection(snapshotID string, items []AlertBatchSelectionItem) ([]AlertBatchSelectionItem, error) {
	if len(snapshotID) < 8 || len(snapshotID) > 256 || len(items) == 0 || len(items) > alertBatchMaximumItems {
		return nil, fmt.Errorf("%w: snapshot_id and 1 to %d items are required", errAlertBatchInvalidRequest, alertBatchMaximumItems)
	}
	seen := make(map[string]struct{}, len(items))
	normalized := make([]AlertBatchSelectionItem, 0, len(items))
	for _, item := range items {
		item.AlertID = strings.TrimSpace(item.AlertID)
		if item.AlertID == "" || len(item.AlertID) > 200 || item.StateVersion <= 0 {
			return nil, fmt.Errorf("%w: each item requires alert_id and a positive state_version", errAlertBatchInvalidRequest)
		}
		if _, exists := seen[item.AlertID]; exists {
			return nil, fmt.Errorf("%w: duplicate alert_id %s", errAlertBatchInvalidRequest, item.AlertID)
		}
		seen[item.AlertID] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized, nil
}

func alertBatchPayloadSHA(value interface{}) (string, []byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", nil, err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), payload, nil
}

func alertBatchTokenSHA(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func (h *AlertBatchAssignmentHandler) deriveSelectionToken(tenantID, selectionID string) (string, error) {
	if len(h.selectionSigningKey) < 32 {
		return "", fmt.Errorf("%w: selection signing secret must contain at least 32 bytes", errAlertBatchSchemaMissing)
	}
	mac := hmac.New(sha256.New, h.selectionSigningKey)
	_, _ = mac.Write([]byte("alert-batch-selection-v1\x00" + tenantID + "\x00" + selectionID))
	raw := append([]byte(nil), mac.Sum(nil)[:16]...)
	raw[6] = (raw[6] & 0x0f) | 0x50
	raw[8] = (raw[8] & 0x3f) | 0x80
	token, err := uuid.FromBytes(raw)
	if err != nil {
		return "", err
	}
	return token.String(), nil
}

func (h *AlertBatchAssignmentHandler) createSelection(ctx context.Context, command alertBatchCommandContext, request AlertBatchSelectionRequest) (*AlertBatchSelectionReceipt, error) {
	requestSHA, _, err := alertBatchPayloadSHA(request)
	if err != nil {
		return nil, err
	}
	itemsJSON, err := json.Marshal(request.Items)
	if err != nil {
		return nil, err
	}
	selectionSHA, _, err := alertBatchPayloadSHA(struct {
		SnapshotID string                    `json:"snapshot_id"`
		Items      []AlertBatchSelectionItem `json:"items"`
	}{request.SnapshotID, request.Items})
	if err != nil {
		return nil, err
	}
	tx, err := h.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, command.TenantID+":selection:"+command.IdempotencyKey); err != nil {
		return nil, h.schemaError(err)
	}
	var existingSHA, existingSelectionID, existingTokenSHA string
	var existingPayload []byte
	err = tx.QueryRowContext(ctx, `SELECT request.request_sha256,request.response_payload,request.selection_id::text,selection.token_sha256
		FROM alert_assignment_selection_requests request
		JOIN alert_assignment_selections selection ON selection.tenant_id=request.tenant_id AND selection.selection_id=request.selection_id
		WHERE request.tenant_id=$1 AND request.idempotency_key=$2`, command.TenantID, command.IdempotencyKey).Scan(&existingSHA, &existingPayload, &existingSelectionID, &existingTokenSHA)
	if err == nil {
		if existingSHA != requestSHA {
			return nil, errAlertBatchIdempotencyConflict
		}
		var receipt AlertBatchSelectionReceipt
		if err := json.Unmarshal(existingPayload, &receipt); err != nil {
			return nil, err
		}
		receipt.SelectionToken, err = h.deriveSelectionToken(command.TenantID, existingSelectionID)
		if err != nil {
			return nil, err
		}
		if alertBatchTokenSHA(receipt.SelectionToken) != existingTokenSHA {
			return nil, fmt.Errorf("%w: persisted selection token digest mismatch", errAlertBatchSchemaMissing)
		}
		receipt.Replayed = true
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &receipt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, h.schemaError(err)
	}
	selectionID := uuid.NewString()
	selectionToken, err := h.deriveSelectionToken(command.TenantID, selectionID)
	if err != nil {
		return nil, err
	}
	now := h.now().UTC()
	expiresAt := now.Add(alertBatchSelectionTTL)
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_selections
		(selection_id,tenant_id,token_sha256,snapshot_id,selection_sha256,items,item_count,created_by,trace_id,expires_at,created_at)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11)`, selectionID, command.TenantID,
		alertBatchTokenSHA(selectionToken), request.SnapshotID, selectionSHA, string(itemsJSON), len(request.Items),
		command.ActorID, command.TraceID, expiresAt, now); err != nil {
		return nil, h.schemaError(err)
	}
	receipt := AlertBatchSelectionReceipt{SelectionID: selectionID, SelectionToken: selectionToken, SnapshotID: request.SnapshotID, SelectionSHA256: selectionSHA, ItemCount: len(request.Items), ExpiresAt: expiresAt, TraceID: command.TraceID}
	storedReceipt := receipt
	storedReceipt.SelectionToken = ""
	receiptJSON, _ := json.Marshal(storedReceipt)
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_selection_requests
		(tenant_id,idempotency_key,request_sha256,selection_id,trace_id,response_payload,created_at)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7)`, command.TenantID, command.IdempotencyKey, requestSHA, selectionID, command.TraceID, string(receiptJSON), now); err != nil {
		return nil, h.schemaError(err)
	}
	if err := insertAlertBatchAudit(ctx, tx, command, "ALERT_BATCH_SELECTION_FROZEN", "alert_assignment_selection", selectionID, map[string]interface{}{
		"snapshot_id": request.SnapshotID, "selection_sha256": selectionSHA, "item_count": len(request.Items), "expires_at": expiresAt,
	}, now); err != nil {
		return nil, h.schemaError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (h *AlertBatchAssignmentHandler) createAssignment(ctx context.Context, command alertBatchCommandContext, request AlertBatchAssignmentRequest) (*AlertBatchAssignmentReceipt, error) {
	requestSHA, _, err := alertBatchPayloadSHA(request)
	if err != nil {
		return nil, err
	}
	tx, err := h.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, command.TenantID+":assignment:"+command.IdempotencyKey); err != nil {
		return nil, h.schemaError(err)
	}
	var existingSHA string
	var existingPayload []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256,response_payload FROM alert_assignment_batch_requests WHERE tenant_id=$1 AND idempotency_key=$2`, command.TenantID, command.IdempotencyKey).Scan(&existingSHA, &existingPayload)
	if err == nil {
		if existingSHA != requestSHA {
			return nil, errAlertBatchIdempotencyConflict
		}
		var receipt AlertBatchAssignmentReceipt
		if err := json.Unmarshal(existingPayload, &receipt); err != nil {
			return nil, err
		}
		receipt.Replayed = true
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &receipt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, h.schemaError(err)
	}
	var selectionID, snapshotID, selectionSHA string
	var itemsJSON []byte
	var itemCount int
	var expiresAt time.Time
	var consumedBatch sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT selection_id::text,snapshot_id,selection_sha256,items,item_count,expires_at,consumed_by_batch_id::text
		FROM alert_assignment_selections WHERE tenant_id=$1 AND token_sha256=$2 FOR UPDATE`, command.TenantID, alertBatchTokenSHA(request.SelectionToken)).Scan(
		&selectionID, &snapshotID, &selectionSHA, &itemsJSON, &itemCount, &expiresAt, &consumedBatch)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errAlertBatchSelectionInvalid
	}
	if err != nil {
		return nil, h.schemaError(err)
	}
	now := h.now().UTC()
	if !expiresAt.After(now) || consumedBatch.Valid {
		return nil, errAlertBatchSelectionInvalid
	}
	var items []AlertBatchSelectionItem
	if err := json.Unmarshal(itemsJSON, &items); err != nil || len(items) != itemCount {
		return nil, errAlertBatchSelectionInvalid
	}
	batchID := uuid.NewString()
	eventID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batches
		(batch_id,tenant_id,action_id,selection_id,selection_snapshot_id,selection_sha256,assignee,reason,status,revision,total_count,accepted_count,requested_by,trace_id,created_at,updated_at)
		VALUES ($1,$2,'alert-batch-assignment-create',$3,$4,$5,$6,$7,'accepted',1,$8,$8,$9,$10,$11,$11)`,
		batchID, command.TenantID, selectionID, snapshotID, selectionSHA, request.Assignee, request.Reason, itemCount, command.ActorID, command.TraceID, now); err != nil {
		return nil, h.schemaError(err)
	}
	for position, item := range items {
		itemEventID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_items
			(tenant_id,batch_id,position,alert_id,expected_state_version,status,item_revision,event_id,updated_at)
			VALUES ($1,$2,$3,$4,$5,'accepted',1,$6,$7)`, command.TenantID, batchID, position, item.AlertID, item.StateVersion, itemEventID, now); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_item_history
			(event_id,tenant_id,batch_id,alert_id,item_revision,previous_status,resulting_status,expected_state_version,resulting_state_version,actor_id,reason,trace_id,detail,occurred_at)
			VALUES ($1,$2,$3,$4,1,'','accepted',$5,0,$6,$7,$8,'{}'::jsonb,$9)`, itemEventID, command.TenantID, batchID, item.AlertID, item.StateVersion, command.ActorID, request.Reason, command.TraceID, now); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE alert_assignment_selections SET consumed_by_batch_id=$1,consumed_at=$2 WHERE tenant_id=$3 AND selection_id=$4`, batchID, now, command.TenantID, selectionID); err != nil {
		return nil, err
	}
	snapshotJSON, _ := json.Marshal(map[string]interface{}{"batch_id": batchID, "status": "accepted", "revision": 1, "selection_id": selectionID, "selection_snapshot_id": snapshotID, "selection_sha256": selectionSHA, "assignee": request.Assignee, "total_count": itemCount, "trace_id": command.TraceID})
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_history
		(event_id,tenant_id,batch_id,revision,action_id,previous_status,resulting_status,actor_id,reason,trace_id,snapshot,occurred_at)
		VALUES ($1,$2,$3,1,'alert-batch-assignment-create','','accepted',$4,$5,$6,$7::jsonb,$8)`, eventID, command.TenantID, batchID, command.ActorID, request.Reason, command.TraceID, string(snapshotJSON), now); err != nil {
		return nil, err
	}
	outboxPayload, _ := json.Marshal(map[string]interface{}{"event_id": eventID, "event_type": "alert.batch-assignment.requested.v1", "schema_version": 1, "aggregate_type": "alert_assignment_batch", "aggregate_id": batchID, "aggregate_version": 1, "partition_key": command.TenantID + ":" + batchID, "tenant_id": command.TenantID, "batch_id": batchID, "selection_id": selectionID, "selection_snapshot_id": snapshotID, "selection_sha256": selectionSHA, "assignee": request.Assignee, "requested_by": command.ActorID, "reason": request.Reason, "status": "accepted", "total_count": itemCount, "trace_id": command.TraceID, "occurred_at": now.Format(time.RFC3339Nano)})
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_outbox
		(event_id,tenant_id,batch_id,aggregate_version,event_type,schema_version,partition_key,payload,trace_id,status,occurred_at)
		VALUES ($1,$2,$3,1,'alert.batch-assignment.requested.v1',1,$4,$5::jsonb,$6,'pending',$7)`, eventID, command.TenantID, batchID, command.TenantID+":"+batchID, string(outboxPayload), command.TraceID, now); err != nil {
		return nil, err
	}
	receipt := AlertBatchAssignmentReceipt{BatchID: batchID, JobID: batchID, EventID: eventID, ActionID: "alert-batch-assignment-create", Status: "accepted", Revision: 1, SelectionID: selectionID, SelectionSnapshotID: snapshotID, SelectionSHA256: selectionSHA, TotalCount: itemCount, AcceptedCount: itemCount, TraceID: command.TraceID, OutboxStatus: "pending"}
	receiptJSON, _ := json.Marshal(receipt)
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_requests
		(tenant_id,idempotency_key,request_sha256,batch_id,resulting_revision,event_id,trace_id,response_payload,created_at)
		VALUES ($1,$2,$3,$4,1,$5,$6,$7::jsonb,$8)`, command.TenantID, command.IdempotencyKey, requestSHA, batchID, eventID, command.TraceID, string(receiptJSON), now); err != nil {
		return nil, err
	}
	if err := insertAlertBatchAudit(ctx, tx, command, "ALERT_BATCH_ASSIGNMENT_ACCEPTED", "alert_assignment_batch", batchID, map[string]interface{}{"event_id": eventID, "selection_id": selectionID, "selection_snapshot_id": snapshotID, "selection_sha256": selectionSHA, "assignee": request.Assignee, "total_count": itemCount, "status": "accepted", "outbox_status": "pending"}, now); err != nil {
		return nil, h.schemaError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (h *AlertBatchAssignmentHandler) getAssignment(ctx context.Context, tenantID, batchID string) (*AlertBatchAssignmentJob, error) {
	var job AlertBatchAssignmentJob
	job.Items = []AlertBatchAssignmentItemResult{}
	err := h.db.QueryRowContext(ctx, `SELECT batch_id::text,batch_id::text,'',action_id,status,revision,selection_id::text,selection_snapshot_id,selection_sha256,total_count,accepted_count,applied_count,conflicted_count,forbidden_count,failed_count,trace_id,'',false,assignee,reason,requested_by,created_at,updated_at
		FROM alert_assignment_batches WHERE tenant_id=$1 AND batch_id=$2`, tenantID, batchID).Scan(
		&job.BatchID, &job.JobID, &job.EventID, &job.ActionID, &job.Status, &job.Revision, &job.SelectionID, &job.SelectionSnapshotID, &job.SelectionSHA256,
		&job.TotalCount, &job.AcceptedCount, &job.AppliedCount, &job.ConflictedCount, &job.ForbiddenCount, &job.FailedCount, &job.TraceID, &job.OutboxStatus, &job.Replayed,
		&job.Assignee, &job.Reason, &job.RequestedBy, &job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errAlertBatchNotFound
	}
	if err != nil {
		return nil, h.schemaError(err)
	}
	rows, err := h.db.QueryContext(ctx, `SELECT alert_id,position,expected_state_version,status,item_revision,resulting_state_version,error_code,error_message,updated_at
		FROM alert_assignment_batch_items WHERE tenant_id=$1 AND batch_id=$2 ORDER BY position`, tenantID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AlertBatchAssignmentItemResult
		if err := rows.Scan(&item.AlertID, &item.Position, &item.ExpectedStateVersion, &item.Status, &item.ItemRevision, &item.ResultingStateVersion, &item.ErrorCode, &item.ErrorMessage, &item.UpdatedAt); err != nil {
			return nil, err
		}
		job.Items = append(job.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var outboxStatus string
	var eventID string
	if err := h.db.QueryRowContext(ctx, `SELECT event_id::text,status FROM alert_assignment_batch_outbox
		WHERE tenant_id=$1 AND batch_id=$2 ORDER BY aggregate_version DESC,outbox_id DESC LIMIT 1`, tenantID, batchID).Scan(&eventID, &outboxStatus); err != nil {
		return nil, h.schemaError(err)
	}
	job.EventID = eventID
	job.OutboxStatus = outboxStatus
	return &job, nil
}

func insertAlertBatchAudit(ctx context.Context, tx *sql.Tx, command alertBatchCommandContext, action, objectType, objectID string, detail map[string]interface{}, occurredAt time.Time) error {
	detail["trace_id"] = command.TraceID
	detail["idempotency_key"] = command.IdempotencyKey
	detailJSON, _ := json.Marshal(detail)
	var dataType string
	err := tx.QueryRowContext(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='audit_logs' AND column_name='user_id'`).Scan(&dataType)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	userIDExpr := "$3"
	actorID := command.ActorID
	if dataType == "uuid" {
		userIDExpr = "NULLIF($3,'')::uuid"
		if _, parseErr := uuid.Parse(actorID); parseErr != nil {
			actorID = ""
		}
	}
	eventID := "audit-" + uuid.NewString()
	query := `INSERT INTO audit_logs (event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent,trace_id,result,success,created_at)
		VALUES ($1,$2,` + userIDExpr + `,$4,$5,$6,$7::jsonb,$8,$9,$10,'accepted',true,$11)`
	_, err = tx.ExecContext(ctx, query, eventID, command.TenantID, actorID, action, objectType, objectID, string(detailJSON), command.SourceIP, command.UserAgent, command.TraceID, occurredAt)
	return err
}

func (h *AlertBatchAssignmentHandler) schemaError(err error) error {
	if isUndefinedTable(err) {
		return fmt.Errorf("%w: apply migration 202608091900", errAlertBatchSchemaMissing)
	}
	return err
}

func (h *AlertBatchAssignmentHandler) writeCommandError(w http.ResponseWriter, ctx context.Context, err error) {
	switch {
	case errors.Is(err, errAlertBatchIdempotencyConflict):
		h.writeError(w, ctx, http.StatusConflict, "IDEMPOTENCY_CONFLICT", err.Error())
	case errors.Is(err, errAlertBatchSelectionInvalid):
		h.writeError(w, ctx, http.StatusConflict, "SELECTION_INVALID", "selection token is invalid, expired, already consumed or belongs to another tenant")
	case errors.Is(err, errAlertBatchSchemaMissing):
		h.writeError(w, ctx, http.StatusServiceUnavailable, "SCHEMA_UNAVAILABLE", err.Error())
	default:
		if h.logger != nil {
			h.logger.Error("Alert batch assignment command failed", zap.Error(err))
		}
		h.writeError(w, ctx, http.StatusInternalServerError, "INTERNAL", "alert batch assignment command failed")
	}
}

func alertBatchContractMeta(ctx context.Context, snapshotID, fallbackTraceID string, revision int64, partial bool, missing []string) httpx.ContractMeta {
	if missing == nil {
		missing = []string{}
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = fallbackTraceID
	}
	return httpx.ContractMeta{ContractVersion: alertBatchAssignmentContractVersion, SnapshotID: snapshotID, AsOf: time.Now().UTC().Format(time.RFC3339Nano), TraceID: traceID, Partial: partial, MissingSections: missing, SourceWatermarks: map[string]string{"postgresql.alert_assignment_batches.revision": fmt.Sprintf("%d", revision), "postgresql.alert_assignment_projection_receipts": fmt.Sprintf("batch_revision:%d", revision)}}
}

func (h *AlertBatchAssignmentHandler) writeSelectionCreated(w http.ResponseWriter, ctx context.Context, receipt *AlertBatchSelectionReceipt) {
	httpx.JSONContractCreated(w, ctx, receipt, alertBatchContractMeta(ctx, receipt.SnapshotID, receipt.TraceID, 1, false, nil))
}

func (h *AlertBatchAssignmentHandler) writeAssignmentAccepted(w http.ResponseWriter, ctx context.Context, receipt *AlertBatchAssignmentReceipt) {
	httpx.JSONContractAccepted(w, ctx, receipt, alertBatchContractMeta(ctx, receipt.SelectionSnapshotID, receipt.TraceID, receipt.Revision, true, []string{"alert.assignment.changed.v1 consumer receipt"}))
}

func (h *AlertBatchAssignmentHandler) writeAssignmentSuccess(w http.ResponseWriter, ctx context.Context, job *AlertBatchAssignmentJob) {
	// meta.partial describes response/source completeness. A terminal business
	// outcome named "partial" still has a complete per-item receipt set.
	partial := job.Status == "accepted" || job.Status == "running"
	missing := []string{}
	if partial {
		missing = []string{"alert.assignment.changed.v1 consumer receipt"}
	}
	httpx.JSONContractSuccess(w, ctx, job, alertBatchContractMeta(ctx, job.SelectionSnapshotID, job.TraceID, job.Revision, partial, missing))
}

func (h *AlertBatchAssignmentHandler) writeError(w http.ResponseWriter, ctx context.Context, status int, code, message string) {
	httpx.JSONError(w, ctx, status, code, message)
}

// stableAlertBatchItems is used only by tests and evidence tooling to compare
// frozen selections without making request order part of the assertion.
func stableAlertBatchItems(items []AlertBatchSelectionItem) []AlertBatchSelectionItem {
	copyItems := append([]AlertBatchSelectionItem(nil), items...)
	sort.Slice(copyItems, func(i, j int) bool { return copyItems[i].AlertID < copyItems[j].AlertID })
	return copyItems
}

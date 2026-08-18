package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/whitelist"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
)

const modelFeedbackRevisionEventType = "model.feedback.v1"

var (
	errModelFeedbackRevisionConflict = stderrors.New("model feedback revision conflict")
	errModelFeedbackAlreadyRetracted = stderrors.New("model feedback already retracted")
)

// ModelFeedbackAdjudicatedV1 is the exact producer-side mirror of the
// consumer-first JSON contract. FeedbackID identifies the immutable aggregate;
// EventID identifies one command/revision.
type ModelFeedbackAdjudicatedV1 struct {
	EventID           string `json:"event_id"`
	EventType         string `json:"event_type"`
	SchemaVersion     int    `json:"schema_version"`
	AggregateVersion  int64  `json:"aggregate_version"`
	FeedbackID        string `json:"feedback_id"`
	TenantID          string `json:"tenant_id"`
	PredictionID      string `json:"prediction_id"`
	AlertID           string `json:"alert_id"`
	Label             string `json:"label"`
	LabelRevision     int64  `json:"label_revision"`
	AdjudicationState string `json:"adjudication_state"`
	ReasonCode        string `json:"reason_code"`
	ModelVersion      string `json:"model_version"`
	RuleVersion       string `json:"rule_version"`
	AdjudicatedBy     string `json:"adjudicated_by"`
	PreviousEventID   string `json:"previous_event_id,omitempty"`
	OccurredAtMS      int64  `json:"occurred_at_ms"`
	TraceID           string `json:"trace_id"`
}

type modelFeedbackRevisionCommitResult struct {
	Event            ModelFeedbackAdjudicatedV1
	CreatedAt        time.Time
	IdempotentReplay bool
}

func modelFeedbackAggregateIdentity(tenantID, predictionID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(strings.Join([]string{
		modelFeedbackRevisionEventType, tenantID, predictionID,
	}, "\x00"))).String()
}

func modelFeedbackRevisionEventIdentity(tenantID, predictionID, idempotencyKey string) string {
	if strings.TrimSpace(idempotencyKey) == "" {
		return uuid.NewString()
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(strings.Join([]string{
		modelFeedbackRevisionEventType, tenantID, predictionID, idempotencyKey,
	}, "\x00"))).String()
}

func modelFeedbackTraceID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if len(raw) == 32 {
		if _, err := hex.DecodeString(raw); err == nil {
			return raw
		}
	}
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:16])
}

func (h *FeedbackHandler) submitModelFeedbackRevision(
	w http.ResponseWriter,
	r *http.Request,
	alert *service.AlertDetailDTO,
	req FeedbackRequest,
	tenantID, alertID, userID, idempotencyKey string,
) {
	ctx := r.Context()
	state := strings.ToUpper(strings.TrimSpace(req.AdjudicationState))
	if state == "" {
		state = "ADJUDICATED"
	}
	if state != "PROPOSED" && state != "ADJUDICATED" && state != "RETRACTED" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ADJUDICATION_STATE", "adjudication_state must be PROPOSED, ADJUDICATED or RETRACTED")
		return
	}
	if req.ExpectedLabelRevision < 0 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_EXPECTED_LABEL_REVISION", "expected_label_revision must be non-negative")
		return
	}
	if state == "RETRACTED" && strings.TrimSpace(req.ReasonCode) == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "MISSING_RETRACTION_REASON", "reason_code is required for RETRACTED feedback")
		return
	}
	if alert == nil || strings.TrimSpace(alert.EventID) == "" ||
		strings.TrimSpace(alert.ModelVersion) == "" || strings.TrimSpace(alert.RuleVersion) == "" {
		httpx.JSONError(w, ctx, http.StatusConflict, "PREDICTION_AUTHORITY_INCOMPLETE", "alert prediction_id, model_version and rule_version must be materialized before feedback")
		return
	}
	actor := strings.TrimSpace(userID)
	if actor == "" {
		actor = "feedback-system"
	}
	aggregateID := modelFeedbackAggregateIdentity(tenantID, alert.EventID)
	event := ModelFeedbackAdjudicatedV1{
		EventID:   modelFeedbackRevisionEventIdentity(tenantID, alert.EventID, idempotencyKey),
		EventType: modelFeedbackRevisionEventType, SchemaVersion: 1,
		FeedbackID: aggregateID, TenantID: tenantID, PredictionID: alert.EventID,
		AlertID: alertID, Label: req.Label, AdjudicationState: state,
		ReasonCode: req.ReasonCode, ModelVersion: alert.ModelVersion,
		RuleVersion: alert.RuleVersion, AdjudicatedBy: actor,
		OccurredAtMS: time.Now().UnixMilli(), TraceID: modelFeedbackTraceID(httpx.GetTraceID(ctx)),
	}
	var whitelistEntry *whitelist.Entry
	var err error
	if req.AddToWhitelist && req.Label == "FP" && state != "RETRACTED" {
		if h.whitelistRepo == nil {
			httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "WHITELIST_PERSISTENCE_UNAVAILABLE", "feedback whitelist repository is unavailable")
			return
		}
		whitelistEntry, err = buildWhitelistDraftEntry(tenantID, alertID, aggregateID, userID, req.ReasonCode, alert)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "WHITELIST_DRAFT_INVALID", "feedback whitelist draft could not be constructed")
			return
		}
	}
	result, err := h.commitModelFeedbackRevision(ctx, r, &event, req, idempotencyKey, whitelistEntry)
	if err != nil {
		switch {
		case stderrors.Is(err, errModelFeedbackAlreadyRetracted):
			httpx.JSONError(w, ctx, http.StatusConflict, "MODEL_FEEDBACK_RETRACTED", err.Error())
		case stderrors.Is(err, errModelFeedbackRevisionConflict):
			httpx.JSONError(w, ctx, http.StatusConflict, "MODEL_FEEDBACK_REVISION_CONFLICT", err.Error())
		default:
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "MODEL_FEEDBACK_TRANSACTION_FAILED", "feedback state, audit and outbox were not committed")
		}
		return
	}
	committed := result.Event
	response := &FeedbackResponse{
		FeedbackID: committed.FeedbackID, EventID: committed.EventID,
		AlertID: committed.AlertID, TenantID: committed.TenantID,
		PredictionID: committed.PredictionID, Label: committed.Label,
		ReasonCode: committed.ReasonCode, Comment: req.Comment, UserID: userID,
		Timestamp: result.CreatedAt, AddToWhitelist: req.AddToWhitelist,
		Status: "accepted", OutboxStatus: "pending",
		LabelRevision:     committed.LabelRevision,
		AdjudicationState: committed.AdjudicationState,
		PreviousEventID:   committed.PreviousEventID,
		IdempotentReplay:  result.IdempotentReplay,
	}
	if result.IdempotentReplay {
		response.OutboxStatus = "existing"
	} else if whitelistEntry != nil {
		response.WhitelistDraft = feedbackWhitelistDraftResponse(whitelistEntry)
	}
	httpx.JSONCreated(w, ctx, response)
}

func (h *FeedbackHandler) commitModelFeedbackRevision(
	ctx context.Context,
	request *http.Request,
	event *ModelFeedbackAdjudicatedV1,
	command FeedbackRequest,
	idempotencyKey string,
	whitelistEntry *whitelist.Entry,
) (modelFeedbackRevisionCommitResult, error) {
	if h.actionAudit == nil || h.actionAudit.db == nil || event == nil {
		return modelFeedbackRevisionCommitResult{}, fmt.Errorf("model feedback PostgreSQL transaction database is unavailable")
	}
	if err := validateModelFeedbackAuthorityEvent(*event, command); err != nil {
		return modelFeedbackRevisionCommitResult{}, err
	}
	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		return modelFeedbackRevisionCommitResult{}, err
	}
	defer tx.Rollback()
	lockMaterial := sha256.Sum256([]byte(event.TenantID + "\x00" + event.PredictionID))
	lockKey := hex.EncodeToString(lockMaterial[:])
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return modelFeedbackRevisionCommitResult{}, fmt.Errorf("lock model feedback aggregate: %w", err)
	}
	var existingPayload string
	var existingCreatedAt time.Time
	var existingComment string
	var existingAddToWhitelist bool
	err = tx.QueryRowContext(ctx, `SELECT payload::text,created_at,COALESCE(comment,''),add_to_whitelist FROM alert_feedback WHERE feedback_id=$1::uuid`, event.EventID).
		Scan(&existingPayload, &existingCreatedAt, &existingComment, &existingAddToWhitelist)
	if err == nil {
		var existing ModelFeedbackAdjudicatedV1
		if json.Unmarshal([]byte(existingPayload), &existing) != nil ||
			existing.EventType != modelFeedbackRevisionEventType || existing.TenantID != event.TenantID ||
			existing.PredictionID != event.PredictionID || existing.AlertID != event.AlertID ||
			existing.Label != event.Label || existing.ReasonCode != event.ReasonCode ||
			existing.AdjudicationState != event.AdjudicationState ||
			existing.ModelVersion != event.ModelVersion || existing.RuleVersion != event.RuleVersion ||
			existing.AdjudicatedBy != event.AdjudicatedBy || existingComment != command.Comment ||
			existingAddToWhitelist != command.AddToWhitelist ||
			existing.LabelRevision != command.ExpectedLabelRevision+1 {
			return modelFeedbackRevisionCommitResult{}, fmt.Errorf("%w: Idempotency-Key was used for a different command", errModelFeedbackRevisionConflict)
		}
		if err := tx.Commit(); err != nil {
			return modelFeedbackRevisionCommitResult{}, err
		}
		return modelFeedbackRevisionCommitResult{Event: existing, CreatedAt: existingCreatedAt, IdempotentReplay: true}, nil
	}
	if !stderrors.Is(err, sql.ErrNoRows) {
		return modelFeedbackRevisionCommitResult{}, err
	}
	var headPayload string
	err = tx.QueryRowContext(ctx, `
		SELECT payload::text FROM alert_feedback
		WHERE tenant_id=$1 AND payload->>'event_type'=$2 AND payload->>'prediction_id'=$3
		ORDER BY (payload->>'label_revision')::bigint DESC,event_id DESC LIMIT 1 FOR UPDATE`,
		event.TenantID, modelFeedbackRevisionEventType, event.PredictionID).Scan(&headPayload)
	var head ModelFeedbackAdjudicatedV1
	switch {
	case stderrors.Is(err, sql.ErrNoRows):
		if command.ExpectedLabelRevision != 0 || event.AdjudicationState == "RETRACTED" {
			return modelFeedbackRevisionCommitResult{}, fmt.Errorf("%w: first revision requires expected_label_revision=0 and cannot be retracted", errModelFeedbackRevisionConflict)
		}
		event.LabelRevision = 1
	case err != nil:
		return modelFeedbackRevisionCommitResult{}, err
	default:
		if err := json.Unmarshal([]byte(headPayload), &head); err != nil {
			return modelFeedbackRevisionCommitResult{}, fmt.Errorf("decode model feedback head: %w", err)
		}
		if head.AdjudicationState == "RETRACTED" {
			return modelFeedbackRevisionCommitResult{}, errModelFeedbackAlreadyRetracted
		}
		if command.ExpectedLabelRevision != head.LabelRevision {
			return modelFeedbackRevisionCommitResult{}, fmt.Errorf("%w: expected revision %d, current revision %d", errModelFeedbackRevisionConflict, command.ExpectedLabelRevision, head.LabelRevision)
		}
		if head.FeedbackID != event.FeedbackID || head.AlertID != event.AlertID ||
			head.ModelVersion != event.ModelVersion || head.RuleVersion != event.RuleVersion {
			return modelFeedbackRevisionCommitResult{}, fmt.Errorf("%w: immutable prediction, model or rule identity changed", errModelFeedbackRevisionConflict)
		}
		if event.AdjudicationState == "RETRACTED" && event.Label != head.Label {
			return modelFeedbackRevisionCommitResult{}, fmt.Errorf("%w: retraction label must match the current head", errModelFeedbackRevisionConflict)
		}
		event.LabelRevision = head.LabelRevision + 1
		event.PreviousEventID = head.EventID
	}
	event.AggregateVersion = event.LabelRevision
	payload, err := json.Marshal(event)
	if err != nil {
		return modelFeedbackRevisionCommitResult{}, err
	}
	createdAt := time.UnixMilli(event.OccurredAtMS).UTC()
	userID := event.AdjudicatedBy
	if _, err := uuid.Parse(userID); err != nil {
		userID = ""
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO alert_feedback
			(feedback_id,event_id,tenant_id,alert_id,user_id,label,reason_code,comment,
			 add_to_whitelist,alert_type,severity,model_version,rule_version,
			 idempotency_key,trace_id,payload,status,created_at,updated_at)
		VALUES ($1::uuid,$1::uuid,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,'accepted',$16,$16)`,
		event.EventID, event.TenantID, event.AlertID, userID, event.Label,
		event.ReasonCode, command.Comment, command.AddToWhitelist, "", "",
		event.ModelVersion, event.RuleVersion, idempotencyKey, event.TraceID,
		string(payload), createdAt); err != nil {
		return modelFeedbackRevisionCommitResult{}, fmt.Errorf("insert model feedback revision: %w", err)
	}
	if whitelistEntry != nil {
		if _, err := h.whitelistRepo.CreateGovernedTx(ctx, tx, whitelistEntry, whitelist.CommandMeta{
			TenantID: event.TenantID, ActorID: event.AdjudicatedBy, ActionID: whitelist.ActionCreate,
			IdempotencyKey: "model-feedback-whitelist-" + event.EventID, ExpectedVersion: 0,
			Reason:  "adjudicated false-positive feedback requested a governed whitelist draft",
			TraceID: event.TraceID, SourceIP: clientIP(request), UserAgent: request.UserAgent(),
		}, whitelist.AuditRecord{
			UserID: event.AdjudicatedBy, Action: "WHITELIST_DRAFT_CREATED", ObjectID: whitelistEntry.ID,
			IPAddress: clientIP(request), UserAgent: request.UserAgent(),
			RequestID: httpx.GetRequestID(ctx), TraceID: event.TraceID,
			Detail: map[string]interface{}{"feedback_id": event.FeedbackID, "event_id": event.EventID, "alert_id": event.AlertID},
		}); err != nil {
			return modelFeedbackRevisionCommitResult{}, err
		}
	}
	if err := h.actionAudit.recordWithExecutor(ctx, tx, request, AlertActionAuditRecord{
		Action: "MODEL_FEEDBACK_ADJUDICATED", ObjectType: "model_feedback",
		ObjectID: event.FeedbackID, TenantID: event.TenantID, UserID: event.AdjudicatedBy,
		AlertID: event.AlertID, Result: "success",
		Detail: map[string]interface{}{
			"event_id": event.EventID, "prediction_id": event.PredictionID,
			"label": event.Label, "label_revision": event.LabelRevision,
			"adjudication_state": event.AdjudicationState,
			"model_version":      event.ModelVersion, "rule_version": event.RuleVersion,
			"previous_event_id": event.PreviousEventID,
		},
	}); err != nil {
		return modelFeedbackRevisionCommitResult{}, fmt.Errorf("insert model feedback audit: %w", err)
	}
	// aggregate_version remains 1 in this legacy outbox table because its
	// expand-only CHECK predates revisions. The versioned payload and publisher
	// headers carry the real label revision.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO alert_feedback_outbox
			(event_id,feedback_id,tenant_id,alert_id,partition_key,schema_version,aggregate_version,payload)
		VALUES ($1::uuid,$1::uuid,$2,$3,$4,1,1,$5::jsonb)`,
		event.EventID, event.TenantID, event.AlertID,
		event.TenantID+":"+event.FeedbackID, string(payload)); err != nil {
		return modelFeedbackRevisionCommitResult{}, fmt.Errorf("insert model feedback outbox: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return modelFeedbackRevisionCommitResult{}, err
	}
	return modelFeedbackRevisionCommitResult{Event: *event, CreatedAt: createdAt}, nil
}

func validateModelFeedbackAuthorityEvent(event ModelFeedbackAdjudicatedV1, command FeedbackRequest) error {
	if event.EventType != modelFeedbackRevisionEventType || event.SchemaVersion != 1 ||
		command.ExpectedLabelRevision < 0 {
		return fmt.Errorf("invalid model feedback authority command")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid model feedback event_id")
	}
	if _, err := uuid.Parse(event.FeedbackID); err != nil ||
		event.FeedbackID != modelFeedbackAggregateIdentity(event.TenantID, event.PredictionID) {
		return fmt.Errorf("invalid model feedback aggregate identity")
	}
	if strings.TrimSpace(event.TenantID) == "" || strings.TrimSpace(event.PredictionID) == "" ||
		strings.TrimSpace(event.AlertID) == "" || strings.TrimSpace(event.ModelVersion) == "" ||
		strings.TrimSpace(event.RuleVersion) == "" || strings.TrimSpace(event.AdjudicatedBy) == "" {
		return fmt.Errorf("incomplete model feedback authority identity")
	}
	if event.Label != "TP" && event.Label != "FP" {
		return fmt.Errorf("invalid model feedback label")
	}
	if event.AdjudicationState != "PROPOSED" && event.AdjudicationState != "ADJUDICATED" &&
		event.AdjudicationState != "RETRACTED" {
		return fmt.Errorf("invalid model feedback adjudication state")
	}
	if (event.Label == "FP" || event.AdjudicationState == "RETRACTED") && strings.TrimSpace(event.ReasonCode) == "" {
		return fmt.Errorf("FP or retracted model feedback requires reason_code")
	}
	if len(event.TraceID) != 32 {
		return fmt.Errorf("invalid model feedback trace_id")
	}
	if _, err := hex.DecodeString(event.TraceID); err != nil || event.TraceID != strings.ToLower(event.TraceID) {
		return fmt.Errorf("invalid model feedback trace_id")
	}
	return nil
}

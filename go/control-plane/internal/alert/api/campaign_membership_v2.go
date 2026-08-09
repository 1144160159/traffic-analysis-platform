package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const campaignMembershipContractVersion = 2

type campaignMembershipOperation string

const (
	campaignMembershipLink   campaignMembershipOperation = "link"
	campaignMembershipUnlink campaignMembershipOperation = "unlink"
)

type alertCampaignUnlinkRequest struct {
	ExpectedRevision         *int64 `json:"expected_revision"`
	ExpectedCampaignRevision *int64 `json:"expected_campaign_revision,omitempty"`
	Reason                   string `json:"reason"`
}

type campaignMembershipCommandError struct {
	status  int
	code    string
	message string
}

func (e *campaignMembershipCommandError) Error() string { return e.message }

func (h *Handler) LinkAlertToCampaign(w http.ResponseWriter, r *http.Request) {
	if !h.campaignAggregateV2 {
		h.linkAlertToCampaignLegacy(w, r)
		return
	}
	request, ok := decodeAlertCampaignLinkRequest(w, r)
	if !ok {
		return
	}
	h.mutateAlertCampaignMembershipV2(w, r, campaignMembershipLink, request)
}

func (h *Handler) UnlinkAlertFromCampaign(w http.ResponseWriter, r *http.Request) {
	if !h.campaignLinkEnabled || !h.campaignAggregateV2 {
		http.NotFound(w, r)
		return
	}
	request, ok := decodeAlertCampaignUnlinkRequest(w, r)
	if !ok {
		return
	}
	h.mutateAlertCampaignMembershipV2(w, r, campaignMembershipUnlink, request)
}

func decodeAlertCampaignUnlinkRequest(w http.ResponseWriter, r *http.Request) (alertCampaignLinkRequest, bool) {
	var body alertCampaignUnlinkRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid campaign unlink request")
		return alertCampaignLinkRequest{}, false
	}
	if err := ensureJSONBodyComplete(decoder); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid campaign unlink request")
		return alertCampaignLinkRequest{}, false
	}
	request := alertCampaignLinkRequest{
		CampaignID:               strings.TrimSpace(mux.Vars(r)["campaign_id"]),
		ExpectedRevision:         body.ExpectedRevision,
		ExpectedCampaignRevision: body.ExpectedCampaignRevision,
		Reason:                   strings.TrimSpace(body.Reason),
	}
	if !validCampaignMembershipRequest(request) {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "campaign id, non-negative revisions and reason (4-1000 characters) are required")
		return alertCampaignLinkRequest{}, false
	}
	return request, true
}

func (h *Handler) mutateAlertCampaignMembershipV2(
	w http.ResponseWriter,
	r *http.Request,
	operation campaignMembershipOperation,
	request alertCampaignLinkRequest,
) {
	ctx := r.Context()
	if !h.campaignLinkEnabled {
		http.NotFound(w, r)
		return
	}
	if !h.requireAlertCampaignLinkPermission(w, r) {
		return
	}
	if !validCampaignMembershipRequest(request) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "campaign id, non-negative revisions and reason (4-1000 characters) are required")
		return
	}
	tenantID := h.extractTenantID(r)
	alertID := strings.TrimSpace(mux.Vars(r)["id"])
	if tenantID == "" || alertID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "tenant and alert id are required")
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "campaign membership persistence is unavailable")
		return
	}
	if h.campaignLookup == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "CAMPAIGN_SOURCE_UNAVAILABLE", "campaign authority is unavailable")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain 16-200 characters")
		return
	}
	if h.alertService != nil {
		if _, err := h.alertService.GetAlert(ctx, tenantID, alertID); err != nil {
			writeAlertMembershipAuthorityError(w, ctx, err)
			return
		}
	}
	exists, err := h.campaignLookup.Exists(ctx, tenantID, request.CampaignID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadGateway, "CAMPAIGN_SOURCE_UNAVAILABLE", "failed to validate campaign")
		return
	}
	if !exists {
		httpx.JSONError(w, ctx, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "campaign not found")
		return
	}
	if err := verifyCampaignAggregateV2Schema(ctx, h.actionAudit.db); err != nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "SCHEMA_UNAVAILABLE", "campaign aggregate v2 schema is unavailable")
		return
	}
	requestSHA, err := campaignMembershipRequestSHA(operation, tenantID, alertID, request)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to fingerprint campaign membership command")
		return
	}
	link, err := h.commitCampaignMembershipV2(ctx, r, operation, alertID, request, idempotencyKey, requestSHA)
	if err != nil {
		var commandErr *campaignMembershipCommandError
		if errors.As(err, &commandErr) {
			httpx.JSONError(w, ctx, commandErr.status, commandErr.code, commandErr.message)
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit campaign membership command")
		return
	}
	writeAlertCampaignLinkResponse(w, ctx, link, link.CampaignRevision)
}

func validCampaignMembershipRequest(request alertCampaignLinkRequest) bool {
	if request.CampaignID == "" || request.ExpectedRevision == nil || *request.ExpectedRevision < 0 {
		return false
	}
	if request.ExpectedCampaignRevision != nil && *request.ExpectedCampaignRevision < 0 {
		return false
	}
	reasonLength := len([]rune(strings.TrimSpace(request.Reason)))
	return reasonLength >= 4 && reasonLength <= 1000
}

func writeAlertMembershipAuthorityError(w http.ResponseWriter, ctx context.Context, err error) {
	// The caller deliberately receives the same not-found response for a
	// missing alert in this tenant and an identifier belonging to another one.
	if commonerrors.IsCode(err, commonerrors.ErrCodeAlertNotFound) {
		httpx.JSONError(w, ctx, http.StatusNotFound, "ALERT_NOT_FOUND", "alert not found")
		return
	}
	httpx.JSONError(w, ctx, http.StatusBadGateway, "ALERT_SOURCE_UNAVAILABLE", "failed to validate alert")
}

func campaignMembershipRequestSHA(
	operation campaignMembershipOperation,
	tenantID string,
	alertID string,
	request alertCampaignLinkRequest,
) (string, error) {
	payload := struct {
		Operation                campaignMembershipOperation `json:"operation"`
		TenantID                 string                      `json:"tenant_id"`
		CampaignID               string                      `json:"campaign_id"`
		AlertID                  string                      `json:"alert_id"`
		ExpectedRevision         int64                       `json:"expected_revision"`
		ExpectedCampaignRevision *int64                      `json:"expected_campaign_revision"`
		Reason                   string                      `json:"reason"`
	}{
		Operation: operation, TenantID: tenantID, CampaignID: request.CampaignID, AlertID: alertID,
		ExpectedRevision: *request.ExpectedRevision, ExpectedCampaignRevision: request.ExpectedCampaignRevision,
		Reason: strings.TrimSpace(request.Reason),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (h *Handler) commitCampaignMembershipV2(
	ctx context.Context,
	httpRequest *http.Request,
	operation campaignMembershipOperation,
	alertID string,
	request alertCampaignLinkRequest,
	idempotencyKey string,
	requestSHA string,
) (alertCampaignLinkDTO, error) {
	tx, err := h.actionAudit.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return alertCampaignLinkDTO{}, err
	}
	defer tx.Rollback()
	tenantID := h.extractTenantID(httpRequest)
	userID := h.extractUserID(httpRequest)

	if replay, found, replaySHA, err := loadCampaignMembershipCommand(ctx, tx, tenantID, idempotencyKey); err != nil {
		return alertCampaignLinkDTO{}, err
	} else if found {
		if replaySHA != requestSHA {
			return alertCampaignLinkDTO{}, &campaignMembershipCommandError{
				status: http.StatusConflict, code: "IDEMPOTENCY_KEY_CONFLICT",
				message: "Idempotency-Key was already used for a different campaign membership command",
			}
		}
		replay.IdempotentReuse = true
		if err := h.actionAudit.recordWithExecutor(ctx, tx, httpRequest, AlertActionAuditRecord{
			Action: "ALERT_CAMPAIGN_MEMBERSHIP_REUSED", ObjectType: "campaign_alert_link", ObjectID: replay.RelationID,
			TenantID: tenantID, UserID: userID, AlertID: alertID, Reason: request.Reason, Result: "idempotent_replay",
			Detail: map[string]interface{}{
				"campaign_id": request.CampaignID, "operation": operation,
				"relation_revision": replay.Revision, "campaign_revision": replay.CampaignRevision,
				"idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
			},
		}); err != nil {
			return alertCampaignLinkDTO{}, err
		}
		if err := tx.Commit(); err != nil {
			return alertCampaignLinkDTO{}, err
		}
		return replay, nil
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_workbench_state
		(tenant_id,campaign_id,assignee,status,state_version,member_count,updated_by,updated_at)
		VALUES ($1,$2,'','active',0,0,$3,now()) ON CONFLICT (tenant_id,campaign_id) DO NOTHING`,
		tenantID, request.CampaignID, userID); err != nil {
		return alertCampaignLinkDTO{}, err
	}
	state, err := lockCampaignAggregateV2State(ctx, tx, tenantID, request.CampaignID)
	if err != nil {
		return alertCampaignLinkDTO{}, err
	}
	if mergedInto, merged, err := loadCampaignMergeTarget(ctx, tx, tenantID, request.CampaignID); err != nil {
		return alertCampaignLinkDTO{}, err
	} else if merged {
		return alertCampaignLinkDTO{}, &campaignMembershipCommandError{
			status: http.StatusConflict, code: "CAMPAIGN_MERGED",
			message: fmt.Sprintf("campaign was merged into %s and no longer accepts membership commands", mergedInto),
		}
	}
	if request.ExpectedCampaignRevision != nil && state.Revision != *request.ExpectedCampaignRevision {
		return alertCampaignLinkDTO{}, &campaignMembershipCommandError{
			status: http.StatusConflict, code: "CAMPAIGN_REVISION_CONFLICT",
			message: fmt.Sprintf("expected campaign revision %d but current revision is %d", *request.ExpectedCampaignRevision, state.Revision),
		}
	}
	if operation == campaignMembershipLink && state.Status == "closed" {
		return alertCampaignLinkDTO{}, &campaignMembershipCommandError{
			status: http.StatusConflict, code: "CAMPAIGN_CLOSED", message: "closed campaign cannot accept new alerts",
		}
	}

	link, found, err := loadAlertCampaignLinkV2ForUpdate(ctx, tx, tenantID, request.CampaignID, alertID)
	if err != nil {
		return alertCampaignLinkDTO{}, err
	}
	if !found && operation == campaignMembershipUnlink {
		return alertCampaignLinkDTO{}, &campaignMembershipCommandError{
			status: http.StatusNotFound, code: "CAMPAIGN_MEMBERSHIP_NOT_FOUND", message: "campaign membership not found",
		}
	}
	if found && link.Revision != *request.ExpectedRevision {
		return alertCampaignLinkDTO{}, &campaignMembershipCommandError{
			status: http.StatusConflict, code: "REVISION_CONFLICT",
			message: fmt.Sprintf("expected relation revision %d but current revision is %d", *request.ExpectedRevision, link.Revision),
		}
	}
	if !found && *request.ExpectedRevision != 0 {
		return alertCampaignLinkDTO{}, &campaignMembershipCommandError{
			status: http.StatusConflict, code: "REVISION_CONFLICT", message: "new campaign membership requires expected_revision=0",
		}
	}

	desiredStatus := "linked"
	if operation == campaignMembershipUnlink {
		desiredStatus = "unlinked"
	}
	// A legacy linked row with campaign_revision=0 is deliberately rebound to
	// the aggregate so member_count and all subsequent report snapshots have a
	// provable campaign revision.
	stateChanged := !found || link.Status != desiredStatus || link.CampaignRevision == 0
	if !stateChanged {
		link.CampaignRevision = state.Revision
		link.CurrentCampaignRevision = state.Revision
		if err := insertCampaignMembershipCommand(ctx, tx, tenantID, operation, idempotencyKey, requestSHA, request, link, userID); err != nil {
			return alertCampaignLinkDTO{}, err
		}
		if err := h.actionAudit.recordWithExecutor(ctx, tx, httpRequest, AlertActionAuditRecord{
			Action: "ALERT_CAMPAIGN_MEMBERSHIP_UNCHANGED", ObjectType: "campaign_alert_link", ObjectID: link.RelationID,
			TenantID: tenantID, UserID: userID, AlertID: alertID, Reason: request.Reason, Result: "no_change",
			Detail: map[string]interface{}{
				"campaign_id": request.CampaignID, "operation": operation,
				"relation_revision": link.Revision, "campaign_revision": link.CampaignRevision,
			},
		}); err != nil {
			return alertCampaignLinkDTO{}, err
		}
		if err := tx.Commit(); err != nil {
			return alertCampaignLinkDTO{}, err
		}
		return link, nil
	}

	eventID := uuid.NewString()
	now := time.Now().UTC()
	nextCampaignRevision := state.Revision + 1
	if found {
		link.Status = desiredStatus
		link.Revision++
		link.CampaignRevision = nextCampaignRevision
		link.UpdatedAt = now
		if _, err := tx.ExecContext(ctx, `UPDATE campaign_alert_links
			SET status=$4,revision=$5,campaign_revision=$6,reason=$7,idempotency_key=$8,updated_by=$9,updated_at=$10
			WHERE tenant_id=$1 AND campaign_id=$2 AND alert_id=$3`,
			tenantID, request.CampaignID, alertID, desiredStatus, link.Revision, nextCampaignRevision,
			request.Reason, idempotencyKey, userID, now); err != nil {
			return alertCampaignLinkDTO{}, err
		}
	} else {
		link = alertCampaignLinkDTO{
			RelationID: uuid.NewString(), TenantID: tenantID, AlertID: alertID, CampaignID: request.CampaignID,
			Status: desiredStatus, Revision: 1, CampaignRevision: nextCampaignRevision,
			EventID: eventID, CreatedAt: now, UpdatedAt: now,
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_alert_links
			(relation_id,tenant_id,campaign_id,alert_id,status,revision,campaign_revision,reason,idempotency_key,created_by,updated_by,created_at,updated_at)
			VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10,$11,$11)`,
			link.RelationID, tenantID, request.CampaignID, alertID, desiredStatus, link.Revision,
			nextCampaignRevision, request.Reason, idempotencyKey, userID, now); err != nil {
			return alertCampaignLinkDTO{}, err
		}
	}
	link.EventID = eventID
	link.CurrentCampaignRevision = nextCampaignRevision

	var memberCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM campaign_alert_links
		WHERE tenant_id=$1 AND campaign_id=$2 AND status='linked'`, tenantID, request.CampaignID).Scan(&memberCount); err != nil {
		return alertCampaignLinkDTO{}, err
	}
	state.MemberCount = memberCount
	state.Revision = nextCampaignRevision
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_workbench_state
		SET state_version=$3,member_count=$4,last_event_id=$5::uuid,updated_by=$6,updated_at=$7
		WHERE tenant_id=$1 AND campaign_id=$2`, tenantID, request.CampaignID, state.Revision,
		state.MemberCount, eventID, userID, now); err != nil {
		return alertCampaignLinkDTO{}, err
	}

	eventType := "traffic.campaign.v2.AlertLinked"
	historyType := "linked"
	auditAction := "ALERT_CAMPAIGN_LINKED"
	if operation == campaignMembershipUnlink {
		eventType = "traffic.campaign.v2.AlertUnlinked"
		historyType = "unlinked"
		auditAction = "ALERT_CAMPAIGN_UNLINKED"
	}
	payload := map[string]interface{}{
		"event_id": eventID, "tenant_id": tenantID, "schema_version": 2,
		"aggregate_type": "campaign", "aggregate_id": request.CampaignID,
		"aggregate_version": state.Revision, "partition_key": tenantID + ":" + request.CampaignID,
		"event_type": eventType, "campaign_id": request.CampaignID, "alert_id": alertID,
		"relation_id": link.RelationID, "relation_revision": link.Revision,
		"campaign_revision": state.Revision, "status": state.Status, "assignee": state.Assignee,
		"member_count": state.MemberCount, "reason": request.Reason, "trace_id": httpx.GetTraceID(ctx),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return alertCampaignLinkDTO{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_alert_link_history
		(event_id,relation_id,tenant_id,campaign_id,alert_id,event_type,revision,campaign_revision,payload,created_by)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)`,
		eventID, link.RelationID, tenantID, request.CampaignID, alertID, historyType,
		link.Revision, state.Revision, string(payloadJSON), userID); err != nil {
		return alertCampaignLinkDTO{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_alert_link_outbox
		(event_id,tenant_id,aggregate_id,aggregate_version,event_type,partition_key,payload)
		VALUES ($1::uuid,$2,$3::uuid,$4,$5,$6,$7::jsonb)`,
		eventID, tenantID, link.RelationID, link.Revision, eventType,
		tenantID+":"+request.CampaignID, string(payloadJSON)); err != nil {
		return alertCampaignLinkDTO{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_aggregate_history
		(event_id,tenant_id,campaign_id,aggregate_revision,event_type,status,assignee,member_count,payload,reason,created_by)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11)`,
		eventID, tenantID, request.CampaignID, state.Revision, eventType, state.Status,
		state.Assignee, state.MemberCount, string(payloadJSON), request.Reason, userID); err != nil {
		return alertCampaignLinkDTO{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_aggregate_outbox
		(event_id,tenant_id,aggregate_id,aggregate_revision,event_type,partition_key,payload)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::jsonb)`, eventID, tenantID, request.CampaignID,
		state.Revision, eventType, tenantID+":"+request.CampaignID, string(payloadJSON)); err != nil {
		return alertCampaignLinkDTO{}, err
	}
	if err := insertCampaignMembershipCommand(ctx, tx, tenantID, operation, idempotencyKey, requestSHA, request, link, userID); err != nil {
		return alertCampaignLinkDTO{}, err
	}
	if err := h.actionAudit.recordWithExecutor(ctx, tx, httpRequest, AlertActionAuditRecord{
		Action: auditAction, ObjectType: "campaign_alert_link", ObjectID: link.RelationID,
		TenantID: tenantID, UserID: userID, AlertID: alertID, Reason: request.Reason, Result: desiredStatus,
		Detail: map[string]interface{}{
			"campaign_id": request.CampaignID, "operation": operation, "event_id": eventID,
			"relation_revision": link.Revision, "campaign_revision": link.CampaignRevision,
			"member_count": state.MemberCount, "idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
		},
	}); err != nil {
		return alertCampaignLinkDTO{}, err
	}
	if err := tx.Commit(); err != nil {
		return alertCampaignLinkDTO{}, err
	}
	return link, nil
}

func loadCampaignMembershipCommand(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	idempotencyKey string,
) (alertCampaignLinkDTO, bool, string, error) {
	var requestSHA string
	var resultJSON []byte
	err := tx.QueryRowContext(ctx, `SELECT request_sha256,result
		FROM campaign_membership_commands WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`,
		tenantID, idempotencyKey).Scan(&requestSHA, &resultJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return alertCampaignLinkDTO{}, false, "", nil
	}
	if err != nil {
		return alertCampaignLinkDTO{}, false, "", err
	}
	var result alertCampaignLinkDTO
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return alertCampaignLinkDTO{}, false, "", err
	}
	return result, true, requestSHA, nil
}

func loadAlertCampaignLinkV2ForUpdate(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	campaignID string,
	alertID string,
) (alertCampaignLinkDTO, bool, error) {
	var link alertCampaignLinkDTO
	err := tx.QueryRowContext(ctx, `SELECT relation_id::text,tenant_id,alert_id,campaign_id,status,revision,campaign_revision,created_at,updated_at
		FROM campaign_alert_links WHERE tenant_id=$1 AND campaign_id=$2 AND alert_id=$3 FOR UPDATE`,
		tenantID, campaignID, alertID).Scan(
		&link.RelationID, &link.TenantID, &link.AlertID, &link.CampaignID, &link.Status,
		&link.Revision, &link.CampaignRevision, &link.CreatedAt, &link.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return alertCampaignLinkDTO{}, false, nil
	}
	return link, err == nil, err
}

func insertCampaignMembershipCommand(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	operation campaignMembershipOperation,
	idempotencyKey string,
	requestSHA string,
	request alertCampaignLinkRequest,
	link alertCampaignLinkDTO,
	userID string,
) error {
	resultJSON, err := json.Marshal(link)
	if err != nil {
		return err
	}
	var expectedCampaignRevision interface{}
	if request.ExpectedCampaignRevision != nil {
		expectedCampaignRevision = *request.ExpectedCampaignRevision
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO campaign_membership_commands
		(command_id,tenant_id,relation_id,campaign_id,alert_id,operation,idempotency_key,request_sha256,
		 expected_relation_revision,expected_campaign_revision,relation_revision,campaign_revision,result,created_by)
		VALUES ($1::uuid,$2,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14)`,
		uuid.NewString(), tenantID, link.RelationID, link.CampaignID, link.AlertID, string(operation),
		idempotencyKey, requestSHA, *request.ExpectedRevision, expectedCampaignRevision,
		link.Revision, link.CampaignRevision, string(resultJSON), userID)
	return err
}

func (h *SystemHandler) ListCampaignMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.campaignAggregateV2 {
		http.NotFound(w, r)
		return
	}
	if !h.requireCampaignReadPermission(w, r) {
		return
	}
	if h.pgDB == nil || h.lookupCampaign == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "campaign membership persistence is unavailable")
		return
	}
	tenantID := queryTenantID(r)
	campaignID := strings.TrimSpace(mux.Vars(r)["id"])
	if campaignID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "campaign id is required")
		return
	}
	campaign, err := h.lookupCampaign(ctx, tenantID, campaignID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "campaign not found")
		} else {
			httpx.JSONError(w, ctx, http.StatusBadGateway, "CAMPAIGN_SOURCE_UNAVAILABLE", "failed to validate campaign")
		}
		return
	}
	campaign.TenantID = tenantID
	campaign.CampaignID = campaignID
	if err := verifyCampaignAggregateV2Schema(ctx, h.pgDB); err != nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "SCHEMA_UNAVAILABLE", "campaign aggregate v2 schema is unavailable")
		return
	}
	limit, offset := parseLimitOffset(r, 200, 1000)
	tx, err := h.pgDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin campaign membership snapshot")
		return
	}
	defer tx.Rollback()
	var state campaignAggregateState
	var lastEventID string
	var stateUpdatedAt time.Time
	stateErr := tx.QueryRowContext(ctx, `SELECT status,assignee,state_version,member_count,
		COALESCE(last_event_id::text,''),updated_at
		FROM campaign_workbench_state WHERE tenant_id=$1 AND campaign_id=$2`, tenantID, campaignID).
		Scan(&state.Status, &state.Assignee, &state.Revision, &state.MemberCount, &lastEventID, &stateUpdatedAt)
	if stateErr != nil && !errors.Is(stateErr, sql.ErrNoRows) {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to read campaign aggregate state")
		return
	}
	if !errors.Is(stateErr, sql.ErrNoRows) {
		campaign.Status = state.Status
		campaign.Assignee = state.Assignee
		campaign.StateVersion = state.Revision
		campaign.MemberCount = state.MemberCount
		campaign.LastEventID = lastEventID
		campaign.WorkbenchUpdatedAt = stateUpdatedAt.UTC().Format(time.RFC3339Nano)
	} else {
		campaign.MemberCount = len(campaign.Alerts)
	}
	if err := stampCampaignLifecycleSnapshot(&campaign); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "SNAPSHOT_FAILED", "failed to identify campaign membership snapshot")
		return
	}
	if !campaignSnapshotMatches(r.URL.Query().Get("snapshot_id"), campaign) {
		httpx.JSONError(w, ctx, http.StatusConflict, "CAMPAIGN_SNAPSHOT_CONFLICT", "campaign changed after the requested snapshot")
		return
	}
	rows, err := tx.QueryContext(ctx, `SELECT relation_id::text,tenant_id,alert_id,campaign_id,status,revision,campaign_revision,created_at,updated_at
		FROM campaign_alert_links WHERE tenant_id=$1 AND campaign_id=$2 AND status='linked'
		ORDER BY alert_id LIMIT $3 OFFSET $4`, tenantID, campaignID, limit+1, offset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to list campaign members")
		return
	}
	defer rows.Close()
	members := make([]alertCampaignLinkDTO, 0, limit)
	var maxRelationRevision int64
	for rows.Next() {
		var member alertCampaignLinkDTO
		if err := rows.Scan(&member.RelationID, &member.TenantID, &member.AlertID, &member.CampaignID,
			&member.Status, &member.Revision, &member.CampaignRevision, &member.CreatedAt, &member.UpdatedAt); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to scan campaign member")
			return
		}
		if member.Revision > maxRelationRevision {
			maxRelationRevision = member.Revision
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to read campaign members")
		return
	}
	truncated := len(members) > limit
	if truncated {
		members = members[:limit]
	}
	var actualMemberCount, legacyUnboundCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(max(revision),0),
		count(*) FILTER (WHERE campaign_revision=0) FROM campaign_alert_links
		WHERE tenant_id=$1 AND campaign_id=$2 AND status='linked'`, tenantID, campaignID).
		Scan(&actualMemberCount, &maxRelationRevision, &legacyUnboundCount); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to reconcile campaign member count")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit campaign membership snapshot")
		return
	}
	missingSections := make([]string, 0, 2)
	if errors.Is(stateErr, sql.ErrNoRows) || state.MemberCount != actualMemberCount {
		missingSections = append(missingSections, "campaign_state_reconcile")
	}
	if legacyUnboundCount > 0 {
		missingSections = append(missingSections, "membership_backfill")
	}
	if truncated {
		missingSections = append(missingSections, "members_truncated")
	}
	httpx.JSONContractSuccess(w, ctx, map[string]interface{}{
		"campaign_id": campaignID, "members": members, "member_count": actualMemberCount,
		"aggregate_member_count": state.MemberCount, "campaign_revision": state.Revision,
		"snapshot_id": campaign.SnapshotID, "snapshot_sha256": campaign.SnapshotSHA256,
		"limit": limit, "offset": offset, "truncated": truncated,
	}, httpx.ContractMeta{
		ContractVersion: campaignLifecycleContractVersion,
		SnapshotID:      campaign.SnapshotID,
		Partial:         len(missingSections) > 0,
		MissingSections: missingSections,
		SourceWatermarks: map[string]string{
			"postgresql.campaign_workbench_state.revision": strconv.FormatInt(state.Revision, 10),
			"postgresql.campaign_alert_links.revision":     strconv.FormatInt(maxRelationRevision, 10),
		},
	})
}

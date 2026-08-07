package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const campaignLinkContractVersion = 1

// AlertCampaignLookup checks the immutable ClickHouse campaign aggregate. The
// tenant predicate is mandatory and is part of every implementation.
type AlertCampaignLookup interface {
	Exists(context.Context, string, string) (bool, error)
}

type clickHouseAlertCampaignLookup struct {
	db *sql.DB
}

func NewClickHouseAlertCampaignLookup(db *sql.DB) AlertCampaignLookup {
	if db == nil {
		return nil
	}
	return &clickHouseAlertCampaignLookup{db: db}
}

func (l *clickHouseAlertCampaignLookup) Exists(ctx context.Context, tenantID, campaignID string) (bool, error) {
	var count uint64
	err := l.db.QueryRowContext(ctx,
		`SELECT count() FROM traffic.campaigns WHERE tenant_id=? AND campaign_id=?`,
		tenantID, campaignID,
	).Scan(&count)
	return count > 0, err
}

type alertCampaignLinkRequest struct {
	CampaignID               string `json:"campaign_id"`
	ExpectedRevision         *int64 `json:"expected_revision"`
	ExpectedCampaignRevision *int64 `json:"expected_campaign_revision,omitempty"`
	Reason                   string `json:"reason"`
}

type alertCampaignLinkDTO struct {
	RelationID              string    `json:"relation_id"`
	TenantID                string    `json:"tenant_id"`
	AlertID                 string    `json:"alert_id"`
	CampaignID              string    `json:"campaign_id"`
	Status                  string    `json:"status"`
	Revision                int64     `json:"revision"`
	CampaignRevision        int64     `json:"campaign_revision"`
	CurrentCampaignRevision int64     `json:"current_campaign_revision"`
	IdempotentReuse         bool      `json:"idempotent_reuse"`
	EventID                 string    `json:"event_id,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

func (h *Handler) linkAlertToCampaignLegacy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.campaignLinkEnabled {
		http.NotFound(w, r)
		return
	}
	if !h.requireAlertCampaignLinkPermission(w, r) {
		return
	}
	tenantID := h.extractTenantID(r)
	alertID := strings.TrimSpace(mux.Vars(r)["id"])
	if tenantID == "" || alertID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "tenant and alert id are required")
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "campaign link persistence is unavailable")
		return
	}
	if h.campaignLookup == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "CAMPAIGN_SOURCE_UNAVAILABLE", "campaign authority is unavailable")
		return
	}
	request, ok := decodeAlertCampaignLinkRequest(w, r)
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain at least 16 characters")
		return
	}
	if h.alertService != nil {
		if _, err := h.alertService.GetAlert(ctx, tenantID, alertID); err != nil {
			if commonerrors.IsCode(err, commonerrors.ErrCodeAlertNotFound) {
				httpx.JSONError(w, ctx, http.StatusNotFound, "ALERT_NOT_FOUND", "alert not found")
			} else {
				httpx.JSONError(w, ctx, http.StatusBadGateway, "ALERT_SOURCE_UNAVAILABLE", "failed to validate alert")
			}
			return
		}
	}
	exists, err := h.campaignLookup.Exists(ctx, tenantID, request.CampaignID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadGateway, "CAMPAIGN_SOURCE_UNAVAILABLE", "failed to validate campaign")
		return
	}
	if !exists {
		// Do not reveal whether the identifier exists in another tenant.
		httpx.JSONError(w, ctx, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "campaign not found")
		return
	}

	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin campaign link transaction")
		return
	}
	defer tx.Rollback()

	campaignState, campaignRevision, err := lockCampaignWorkbenchState(ctx, tx, tenantID, request.CampaignID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to lock campaign state")
		return
	}
	if campaignState == "closed" {
		httpx.JSONError(w, ctx, http.StatusConflict, "CAMPAIGN_CLOSED", "closed campaign cannot accept new alerts")
		return
	}

	link, found, err := loadAlertCampaignLinkByIdempotencyKey(ctx, tx, tenantID, idempotencyKey)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load campaign link")
		return
	}
	if found && (link.CampaignID != request.CampaignID || link.AlertID != alertID) {
		httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different campaign link")
		return
	}
	if !found {
		link, found, err = loadAlertCampaignLinkForUpdate(ctx, tx, tenantID, request.CampaignID, alertID)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load campaign link")
			return
		}
	}
	if found && link.Status == "linked" {
		link.IdempotentReuse = true
		if err := h.actionAudit.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
			Action: "ALERT_CAMPAIGN_LINK_REUSED", ObjectType: "campaign_alert_link", ObjectID: link.RelationID,
			TenantID: tenantID, UserID: h.extractUserID(r), AlertID: alertID, Reason: request.Reason,
			Result: "idempotent_replay", Detail: map[string]interface{}{
				"action_id": "alert-campaign-link", "campaign_id": request.CampaignID,
				"revision": link.Revision, "idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
			},
		}); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit idempotent campaign link")
			return
		}
		if err := tx.Commit(); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit idempotent campaign link")
			return
		}
		writeAlertCampaignLinkResponse(w, ctx, link, campaignRevision)
		return
	}

	expectedRevision := *request.ExpectedRevision
	if found && link.Revision != expectedRevision {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", fmt.Sprintf("expected revision %d but current revision is %d", expectedRevision, link.Revision))
		return
	}
	if !found && expectedRevision != 0 {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "new campaign link requires expected_revision=0")
		return
	}

	eventID := uuid.NewString()
	now := time.Now().UTC()
	if found {
		link.Status = "linked"
		link.Revision++
		link.UpdatedAt = now
		_, err = tx.ExecContext(ctx, `UPDATE campaign_alert_links
			SET status='linked', revision=$4, reason=$5, idempotency_key=$6, updated_by=$7, updated_at=$8
			WHERE tenant_id=$1 AND campaign_id=$2 AND alert_id=$3`,
			tenantID, request.CampaignID, alertID, link.Revision, request.Reason, idempotencyKey, h.extractUserID(r), now)
	} else {
		link = alertCampaignLinkDTO{
			RelationID: uuid.NewString(), TenantID: tenantID, AlertID: alertID, CampaignID: request.CampaignID,
			Status: "linked", Revision: 1, EventID: eventID, CreatedAt: now, UpdatedAt: now,
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO campaign_alert_links
			(relation_id,tenant_id,campaign_id,alert_id,status,revision,reason,idempotency_key,created_by,updated_by,created_at,updated_at)
			VALUES ($1::uuid,$2,$3,$4,'linked',$5,$6,$7,$8,$8,$9,$9)`,
			link.RelationID, tenantID, request.CampaignID, alertID, link.Revision, request.Reason, idempotencyKey, h.extractUserID(r), now)
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist campaign link")
		return
	}
	link.EventID = eventID
	payload := map[string]interface{}{
		"event_id": eventID, "tenant_id": tenantID, "schema_version": 2,
		"aggregate_type": "campaign_alert_link", "aggregate_id": link.RelationID,
		"aggregate_version": link.Revision, "partition_key": tenantID + ":" + request.CampaignID,
		"campaign_id": request.CampaignID, "alert_id": alertID, "status": link.Status,
	}
	payloadJSON, _ := json.Marshal(payload)
	if _, err = tx.ExecContext(ctx, `INSERT INTO campaign_alert_link_history
		(event_id,relation_id,tenant_id,campaign_id,alert_id,event_type,revision,payload,created_by)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,'linked',$6,$7::jsonb,$8)`,
		eventID, link.RelationID, tenantID, request.CampaignID, alertID, link.Revision, string(payloadJSON), h.extractUserID(r)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist campaign link history")
		return
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO campaign_alert_link_outbox
		(event_id,tenant_id,aggregate_id,aggregate_version,event_type,partition_key,payload)
		VALUES ($1::uuid,$2,$3::uuid,$4,'traffic.campaign.v2.AlertLinked',$5,$6::jsonb)`,
		eventID, tenantID, link.RelationID, link.Revision, tenantID+":"+request.CampaignID, string(payloadJSON)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to enqueue campaign link projection")
		return
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: "ALERT_CAMPAIGN_LINKED", ObjectType: "campaign_alert_link", ObjectID: link.RelationID,
		TenantID: tenantID, UserID: h.extractUserID(r), AlertID: alertID, Reason: request.Reason, Result: "linked",
		Detail: map[string]interface{}{
			"action_id": "alert-campaign-link", "campaign_id": request.CampaignID, "revision": link.Revision,
			"event_id": eventID, "idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
		},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit campaign link")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit campaign link")
		return
	}
	writeAlertCampaignLinkResponse(w, ctx, link, campaignRevision)
}

func (h *Handler) ListAlertCampaignLinks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.campaignLinkEnabled {
		http.NotFound(w, r)
		return
	}
	if !h.requireAlertReadPermission(w, r) {
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "campaign link persistence is unavailable")
		return
	}
	tenantID := h.extractTenantID(r)
	alertID := strings.TrimSpace(mux.Vars(r)["id"])
	rows, err := h.actionAudit.db.QueryContext(ctx, `SELECT l.relation_id::text,l.tenant_id,l.alert_id,l.campaign_id,l.status,l.revision,
		l.campaign_revision,COALESCE(s.state_version,0),l.created_at,l.updated_at
		FROM campaign_alert_links l LEFT JOIN campaign_workbench_state s
		  ON s.tenant_id=l.tenant_id AND s.campaign_id=l.campaign_id
		WHERE l.tenant_id=$1 AND l.alert_id=$2 AND l.status='linked' ORDER BY l.updated_at DESC`, tenantID, alertID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to list campaign links")
		return
	}
	defer rows.Close()
	links := make([]alertCampaignLinkDTO, 0)
	var maxRevision int64
	for rows.Next() {
		var link alertCampaignLinkDTO
		if err := rows.Scan(&link.RelationID, &link.TenantID, &link.AlertID, &link.CampaignID,
			&link.Status, &link.Revision, &link.CampaignRevision, &link.CurrentCampaignRevision,
			&link.CreatedAt, &link.UpdatedAt); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to scan campaign link")
			return
		}
		if link.Revision > maxRevision {
			maxRevision = link.Revision
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to list campaign links")
		return
	}
	httpx.JSONContractSuccess(w, ctx, map[string]interface{}{
		"links": links, "total": len(links), "unlink_available": h.campaignAggregateV2,
	}, httpx.ContractMeta{
		ContractVersion: campaignLinkContractVersion,
		SnapshotID:      fmt.Sprintf("campaign-links:%s:%d", alertID, maxRevision),
		SourceWatermarks: map[string]string{
			"postgresql.campaign_alert_links.revision": strconv.FormatInt(maxRevision, 10),
		},
	})
}

func decodeAlertCampaignLinkRequest(w http.ResponseWriter, r *http.Request) (alertCampaignLinkRequest, bool) {
	var request alertCampaignLinkRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid campaign link request")
		return request, false
	}
	if err := ensureJSONBodyComplete(decoder); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid campaign link request")
		return request, false
	}
	request.CampaignID = strings.TrimSpace(request.CampaignID)
	request.Reason = strings.TrimSpace(request.Reason)
	if !validCampaignMembershipRequest(request) {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "campaign_id, non-negative revisions and reason (4-1000 characters) are required")
		return request, false
	}
	return request, true
}

func lockCampaignWorkbenchState(ctx context.Context, tx *sql.Tx, tenantID, campaignID string) (string, int64, error) {
	var status string
	var revision int64
	err := tx.QueryRowContext(ctx, `SELECT status,state_version FROM campaign_workbench_state
		WHERE tenant_id=$1 AND campaign_id=$2 FOR UPDATE`, tenantID, campaignID).Scan(&status, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return "active", 0, nil
	}
	return status, revision, err
}

func loadAlertCampaignLinkForUpdate(ctx context.Context, tx *sql.Tx, tenantID, campaignID, alertID string) (alertCampaignLinkDTO, bool, error) {
	var link alertCampaignLinkDTO
	err := tx.QueryRowContext(ctx, `SELECT relation_id::text,tenant_id,alert_id,campaign_id,status,revision,created_at,updated_at
		FROM campaign_alert_links WHERE tenant_id=$1 AND campaign_id=$2 AND alert_id=$3 FOR UPDATE`,
		tenantID, campaignID, alertID,
	).Scan(&link.RelationID, &link.TenantID, &link.AlertID, &link.CampaignID, &link.Status, &link.Revision, &link.CreatedAt, &link.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return alertCampaignLinkDTO{}, false, nil
	}
	return link, err == nil, err
}

func loadAlertCampaignLinkByIdempotencyKey(ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey string) (alertCampaignLinkDTO, bool, error) {
	var link alertCampaignLinkDTO
	err := tx.QueryRowContext(ctx, `SELECT relation_id::text,tenant_id,alert_id,campaign_id,status,revision,created_at,updated_at
		FROM campaign_alert_links WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`,
		tenantID, idempotencyKey,
	).Scan(&link.RelationID, &link.TenantID, &link.AlertID, &link.CampaignID, &link.Status, &link.Revision, &link.CreatedAt, &link.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return alertCampaignLinkDTO{}, false, nil
	}
	return link, err == nil, err
}

func writeAlertCampaignLinkResponse(w http.ResponseWriter, ctx context.Context, link alertCampaignLinkDTO, campaignRevision int64) {
	httpx.JSONContractSuccess(w, ctx, link, httpx.ContractMeta{
		ContractVersion: campaignLinkContractVersion,
		SnapshotID:      fmt.Sprintf("campaign-alert-link:%s:%d", link.RelationID, link.Revision),
		SourceWatermarks: map[string]string{
			"postgresql.campaign_alert_links.revision":     strconv.FormatInt(link.Revision, 10),
			"postgresql.campaign_workbench_state.revision": strconv.FormatInt(campaignRevision, 10),
		},
	})
}

func (h *Handler) requireAlertCampaignLinkPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopeAlertWrite) &&
		hasSystemPermission(ctx, authmodel.ScopeCampaignWrite) {
		return true
	}
	commonerrors.WriteErrorWithStatus(w, http.StatusForbidden, commonerrors.ErrCodePermissionDenied,
		"Permission denied: alert:write and campaign:write required",
		httpx.GetTraceID(ctx), r.URL.Path)
	return false
}

func opaqueKeyDigest(value string) string {
	// Audit records never expose the caller-supplied idempotency key.
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(value)))
}

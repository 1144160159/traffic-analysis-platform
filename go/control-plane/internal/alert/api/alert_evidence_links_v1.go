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
	"strings"
	"time"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const (
	alertEvidenceLinkSchemaVersion = "202608160030"
	AlertEvidenceLinkEventTopic    = "alert.evidence-links.v1"
	alertEvidenceLinkMaxAttempts   = 8
)

type alertEvidenceLinkOperation string

const (
	alertEvidenceLink   alertEvidenceLinkOperation = "link"
	alertEvidenceUnlink alertEvidenceLinkOperation = "unlink"
)

type AlertEvidenceLink struct {
	RelationID       string    `json:"relation_id"`
	TenantID         string    `json:"tenant_id"`
	AlertID          string    `json:"alert_id"`
	EvidenceID       string    `json:"evidence_id"`
	EvidenceType     string    `json:"evidence_type"`
	SourceStore      string    `json:"source_store"`
	ObjectBucket     string    `json:"object_bucket,omitempty"`
	ObjectKey        string    `json:"object_key,omitempty"`
	ObjectVersion    string    `json:"object_version,omitempty"`
	ObjectSHA256     string    `json:"object_sha256,omitempty"`
	SizeBytes        int64     `json:"size_bytes"`
	ContentType      string    `json:"content_type,omitempty"`
	Status           string    `json:"status"`
	Revision         int64     `json:"revision"`
	EventID          string    `json:"event_id"`
	ManifestRevision int64     `json:"manifest_revision"`
	OutboxStatus     string    `json:"outbox_status"`
	Changed          bool      `json:"changed"`
	IdempotentReuse  bool      `json:"idempotent_reuse"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type alertEvidenceLinkRequest struct {
	ExpectedRevision         *int64 `json:"expected_revision"`
	ExpectedManifestRevision *int64 `json:"expected_manifest_revision"`
	SourceStore              string `json:"source_store"`
	ObjectBucket             string `json:"object_bucket"`
	ObjectKey                string `json:"object_key"`
	ObjectVersion            string `json:"object_version"`
	ObjectSHA256             string `json:"object_sha256"`
	Reason                   string `json:"reason"`
}

type alertEvidenceUnlinkRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type alertEvidenceLinkCommandError struct {
	status  int
	code    string
	message string
}

func (e *alertEvidenceLinkCommandError) Error() string { return e.message }

type alertEvidenceLinkPublisher interface {
	Send(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error)
}

func (h *Handler) SetAlertEvidenceLinkRuntime(
	enabled bool,
	consumerReady func(context.Context) error,
	publisher alertEvidenceLinkPublisher,
) {
	h.alertEvidenceLinkEnabled = enabled
	h.alertEvidenceLinkConsumerReady = consumerReady
	h.alertEvidenceLinkPublisher = publisher
}

func (h *Handler) LinkAlertEvidence(w http.ResponseWriter, r *http.Request) {
	var request alertEvidenceLinkRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || ensureJSONBodyComplete(decoder) != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid alert evidence link request")
		return
	}
	request.SourceStore = strings.TrimSpace(request.SourceStore)
	request.ObjectBucket = strings.TrimSpace(request.ObjectBucket)
	request.ObjectKey = strings.TrimSpace(request.ObjectKey)
	request.ObjectVersion = strings.TrimSpace(request.ObjectVersion)
	request.ObjectSHA256 = strings.ToLower(strings.TrimSpace(request.ObjectSHA256))
	request.Reason = strings.TrimSpace(request.Reason)
	if !validAlertEvidenceLinkRequest(request) {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "expected revisions, manifest object identity, sha256 and reason (4-1000 characters) are required")
		return
	}
	h.mutateAlertEvidenceLink(w, r, alertEvidenceLink, request)
}

func (h *Handler) UnlinkAlertEvidence(w http.ResponseWriter, r *http.Request) {
	var body alertEvidenceUnlinkRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || ensureJSONBodyComplete(decoder) != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid alert evidence unlink request")
		return
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if body.ExpectedRevision == nil || *body.ExpectedRevision < 1 || !validAlertEvidenceReason(body.Reason) {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "positive expected_revision and reason (4-1000 characters) are required")
		return
	}
	request := alertEvidenceLinkRequest{ExpectedRevision: body.ExpectedRevision, Reason: body.Reason}
	h.mutateAlertEvidenceLink(w, r, alertEvidenceUnlink, request)
}

func validAlertEvidenceLinkRequest(request alertEvidenceLinkRequest) bool {
	if request.ExpectedRevision == nil || *request.ExpectedRevision < 0 ||
		request.ExpectedManifestRevision == nil || *request.ExpectedManifestRevision < 1 ||
		request.SourceStore == "" || !validAlertEvidenceReason(request.Reason) {
		return false
	}
	if request.SourceStore == "minio" {
		return request.ObjectBucket != "" && request.ObjectKey != "" && request.ObjectVersion != "" &&
			validLowerSHA256(request.ObjectSHA256)
	}
	return request.ObjectBucket == "" && request.ObjectKey == "" && request.ObjectVersion == "" && request.ObjectSHA256 == ""
}

func validAlertEvidenceReason(value string) bool {
	n := len([]rune(strings.TrimSpace(value)))
	return n >= 4 && n <= 1000 && !strings.ContainsAny(value, "\x00\r\n")
}

func validLowerSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (h *Handler) mutateAlertEvidenceLink(
	w http.ResponseWriter,
	r *http.Request,
	operation alertEvidenceLinkOperation,
	request alertEvidenceLinkRequest,
) {
	ctx := r.Context()
	if !h.alertEvidenceLinkEnabled {
		http.NotFound(w, r)
		return
	}
	if !h.requireAlertWritePermission(w, r) {
		return
	}
	if h.alertEvidenceLinkConsumerReady == nil || h.alertEvidenceLinkConsumerReady(ctx) != nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "EVIDENCE_LINK_CONSUMER_NOT_READY", "alert evidence link projection consumer is not ready")
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert evidence link persistence is unavailable")
		return
	}
	tenantID := h.extractTenantID(r)
	alertID := strings.TrimSpace(mux.Vars(r)["id"])
	evidenceID := strings.TrimSpace(mux.Vars(r)["evidence_id"])
	if tenantID == "" || alertID == "" || evidenceID == "" || strings.EqualFold(tenantID, "unknown") {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "authenticated tenant, alert id and evidence id are required")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 || strings.ContainsAny(idempotencyKey, "\x00\r\n") {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain 16-200 safe characters")
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
	requestSHA, err := alertEvidenceLinkRequestSHA(operation, tenantID, alertID, evidenceID, request)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to fingerprint alert evidence link command")
		return
	}
	result, err := h.commitAlertEvidenceLink(ctx, r, operation, tenantID, alertID, evidenceID, idempotencyKey, requestSHA, request)
	if err != nil {
		var commandErr *alertEvidenceLinkCommandError
		if errors.As(err, &commandErr) {
			httpx.JSONError(w, ctx, commandErr.status, commandErr.code, commandErr.message)
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit alert evidence link command")
		return
	}
	httpx.JSONSuccess(w, ctx, result)
}

func alertEvidenceLinkRequestSHA(
	operation alertEvidenceLinkOperation,
	tenantID, alertID, evidenceID string,
	request alertEvidenceLinkRequest,
) (string, error) {
	payload := struct {
		Operation                alertEvidenceLinkOperation `json:"operation"`
		TenantID                 string                     `json:"tenant_id"`
		AlertID                  string                     `json:"alert_id"`
		EvidenceID               string                     `json:"evidence_id"`
		ExpectedRevision         int64                      `json:"expected_revision"`
		ExpectedManifestRevision *int64                     `json:"expected_manifest_revision,omitempty"`
		SourceStore              string                     `json:"source_store,omitempty"`
		ObjectBucket             string                     `json:"object_bucket,omitempty"`
		ObjectKey                string                     `json:"object_key,omitempty"`
		ObjectVersion            string                     `json:"object_version,omitempty"`
		ObjectSHA256             string                     `json:"object_sha256,omitempty"`
		Reason                   string                     `json:"reason"`
	}{operation, tenantID, alertID, evidenceID, *request.ExpectedRevision, request.ExpectedManifestRevision,
		request.SourceStore, request.ObjectBucket, request.ObjectKey, request.ObjectVersion,
		request.ObjectSHA256, request.Reason}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (h *Handler) commitAlertEvidenceLink(
	ctx context.Context,
	httpRequest *http.Request,
	operation alertEvidenceLinkOperation,
	tenantID, alertID, evidenceID, idempotencyKey, requestSHA string,
	request alertEvidenceLinkRequest,
) (AlertEvidenceLink, error) {
	tx, err := h.actionAudit.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return AlertEvidenceLink{}, err
	}
	defer tx.Rollback()
	if err := verifyAlertEvidenceLinkSchemaTx(ctx, tx); err != nil {
		return AlertEvidenceLink{}, &alertEvidenceLinkCommandError{http.StatusServiceUnavailable, "SCHEMA_UNAVAILABLE", "alert evidence link schema is unavailable"}
	}
	if replay, found, replaySHA, err := loadAlertEvidenceLinkCommand(ctx, tx, tenantID, idempotencyKey); err != nil {
		return AlertEvidenceLink{}, err
	} else if found {
		if replaySHA != requestSHA {
			return AlertEvidenceLink{}, &alertEvidenceLinkCommandError{http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different alert evidence command"}
		}
		replay.IdempotentReuse = true
		if err := h.recordAlertEvidenceLinkAudit(ctx, tx, httpRequest, "ALERT_EVIDENCE_LINK_COMMAND_REUSED", replay, request.Reason, "idempotent_replay", idempotencyKey); err != nil {
			return AlertEvidenceLink{}, err
		}
		if err := tx.Commit(); err != nil {
			return AlertEvidenceLink{}, err
		}
		return replay, nil
	}

	manifest, err := loadAlertEvidenceManifestForShare(ctx, tx, tenantID, alertID, evidenceID)
	if errors.Is(err, sql.ErrNoRows) {
		return AlertEvidenceLink{}, &alertEvidenceLinkCommandError{http.StatusNotFound, "EVIDENCE_MANIFEST_NOT_FOUND", "alert evidence manifest not found"}
	}
	if err != nil {
		return AlertEvidenceLink{}, err
	}
	if manifest.State != "available" || (manifest.ExpiresAt != nil && !manifest.ExpiresAt.After(time.Now().UTC())) {
		return AlertEvidenceLink{}, &alertEvidenceLinkCommandError{http.StatusConflict, "EVIDENCE_NOT_LINKABLE", "only an available, unexpired evidence manifest can be linked or unlinked"}
	}
	if operation == alertEvidenceLink {
		if manifest.Revision != *request.ExpectedManifestRevision {
			return AlertEvidenceLink{}, &alertEvidenceLinkCommandError{http.StatusConflict, "MANIFEST_REVISION_CONFLICT", fmt.Sprintf("expected manifest revision %d but current revision is %d", *request.ExpectedManifestRevision, manifest.Revision)}
		}
		if !alertEvidenceRequestMatchesManifest(request, manifest) {
			return AlertEvidenceLink{}, &alertEvidenceLinkCommandError{http.StatusConflict, "OBJECT_IDENTITY_CONFLICT", "requested object identity or digest does not match the immutable evidence manifest"}
		}
	}

	link, found, err := loadAlertEvidenceLinkForUpdate(ctx, tx, tenantID, alertID, evidenceID)
	if err != nil {
		return AlertEvidenceLink{}, err
	}
	if !found && operation == alertEvidenceUnlink {
		return AlertEvidenceLink{}, &alertEvidenceLinkCommandError{http.StatusNotFound, "EVIDENCE_LINK_NOT_FOUND", "alert evidence link not found"}
	}
	if found && link.Revision != *request.ExpectedRevision {
		return AlertEvidenceLink{}, &alertEvidenceLinkCommandError{http.StatusConflict, "REVISION_CONFLICT", fmt.Sprintf("expected relation revision %d but current revision is %d", *request.ExpectedRevision, link.Revision)}
	}
	if !found && *request.ExpectedRevision != 0 {
		return AlertEvidenceLink{}, &alertEvidenceLinkCommandError{http.StatusConflict, "REVISION_CONFLICT", "a new alert evidence link requires expected_revision=0"}
	}
	if found && !alertEvidenceLinkMatchesManifest(link, manifest) {
		return AlertEvidenceLink{}, &alertEvidenceLinkCommandError{http.StatusConflict, "OBJECT_IDENTITY_CONFLICT", "existing link object identity differs from the immutable evidence manifest"}
	}

	desiredStatus := "linked"
	if operation == alertEvidenceUnlink {
		desiredStatus = "unlinked"
	}
	userID := h.extractUserID(httpRequest)
	if found && link.Status == desiredStatus {
		link.ManifestRevision = manifest.Revision
		link.OutboxStatus = "unchanged"
		link.Changed = false
		if err := insertAlertEvidenceLinkCommand(ctx, tx, tenantID, operation, idempotencyKey, requestSHA, *request.ExpectedRevision, link, userID); err != nil {
			return AlertEvidenceLink{}, err
		}
		if err := h.recordAlertEvidenceLinkAudit(ctx, tx, httpRequest, "ALERT_EVIDENCE_LINK_UNCHANGED", link, request.Reason, "no_change", idempotencyKey); err != nil {
			return AlertEvidenceLink{}, err
		}
		if err := tx.Commit(); err != nil {
			return AlertEvidenceLink{}, err
		}
		return link, nil
	}

	now := time.Now().UTC()
	eventID := uuid.NewString()
	if !found {
		link = AlertEvidenceLink{
			RelationID: uuid.NewString(), TenantID: tenantID, AlertID: alertID, EvidenceID: evidenceID,
			EvidenceType: manifest.EvidenceType, SourceStore: manifest.SourceStore,
			ObjectBucket: manifest.ObjectBucket, ObjectKey: manifest.ObjectKey,
			ObjectVersion: manifest.ObjectVersion, ObjectSHA256: manifest.ObjectSHA256,
			SizeBytes: manifest.SizeBytes, ContentType: manifest.ContentType,
			Status: desiredStatus, Revision: 1, CreatedAt: now, UpdatedAt: now,
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO alert_evidence_links
			(relation_id,tenant_id,alert_id,evidence_id,evidence_type,source_store,object_bucket,object_key,
			 object_version,object_sha256,size_bytes,content_type,status,revision,last_event_id,reason,
			 created_by,updated_by,created_at,updated_at)
			VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,1,$14::uuid,$15,$16,$16,$17,$17)`,
			link.RelationID, tenantID, alertID, evidenceID, link.EvidenceType, link.SourceStore,
			link.ObjectBucket, link.ObjectKey, link.ObjectVersion, link.ObjectSHA256, link.SizeBytes,
			link.ContentType, desiredStatus, eventID, request.Reason, userID, now)
	} else {
		link.Status = desiredStatus
		link.Revision++
		link.UpdatedAt = now
		_, err = tx.ExecContext(ctx, `UPDATE alert_evidence_links
			SET status=$4,revision=$5,last_event_id=$6::uuid,reason=$7,updated_by=$8,updated_at=$9
			WHERE tenant_id=$1 AND alert_id=$2 AND evidence_id=$3`, tenantID, alertID, evidenceID,
			desiredStatus, link.Revision, eventID, request.Reason, userID, now)
	}
	if err != nil {
		return AlertEvidenceLink{}, err
	}
	link.EventID = eventID
	link.ManifestRevision = manifest.Revision
	link.OutboxStatus = "pending"
	link.Changed = true
	eventType := "traffic.alert-evidence.v1.Linked"
	historyType := "linked"
	auditAction := "ALERT_EVIDENCE_LINKED"
	if operation == alertEvidenceUnlink {
		eventType = "traffic.alert-evidence.v1.Unlinked"
		historyType = "unlinked"
		auditAction = "ALERT_EVIDENCE_UNLINKED"
	}
	partitionKey := tenantID + ":" + alertID
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": eventType, "schema_version": 1,
		"tenant_id": tenantID, "aggregate_type": "alert_evidence_link", "aggregate_id": link.RelationID,
		"aggregate_version": link.Revision, "partition_key": partitionKey,
		"alert_id": alertID, "evidence_id": evidenceID, "evidence_type": link.EvidenceType,
		"status": desiredStatus, "source_store": link.SourceStore, "object_bucket": link.ObjectBucket,
		"object_key": link.ObjectKey, "object_version": link.ObjectVersion,
		"object_sha256": link.ObjectSHA256, "size_bytes": link.SizeBytes,
		"content_type": link.ContentType, "manifest_revision": manifest.Revision,
		"reason": request.Reason, "trace_id": httpx.GetTraceID(ctx), "occurred_at": now.Format(time.RFC3339Nano),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return AlertEvidenceLink{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_evidence_link_history
		(event_id,relation_id,tenant_id,alert_id,evidence_id,event_type,relation_revision,source_store,
		 object_bucket,object_key,object_version,object_sha256,request_sha256,payload,reason,created_by,created_at)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15,$16,$17)`,
		eventID, link.RelationID, tenantID, alertID, evidenceID, historyType, link.Revision,
		link.SourceStore, link.ObjectBucket, link.ObjectKey, link.ObjectVersion, link.ObjectSHA256,
		requestSHA, string(payloadJSON), request.Reason, userID, now); err != nil {
		return AlertEvidenceLink{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_evidence_link_outbox
		(event_id,tenant_id,aggregate_id,aggregate_version,event_type,partition_key,payload)
		VALUES ($1::uuid,$2,$3::uuid,$4,$5,$6,$7::jsonb)`, eventID, tenantID, link.RelationID,
		link.Revision, eventType, partitionKey, string(payloadJSON)); err != nil {
		return AlertEvidenceLink{}, err
	}
	if err := insertAlertEvidenceLinkCommand(ctx, tx, tenantID, operation, idempotencyKey, requestSHA, *request.ExpectedRevision, link, userID); err != nil {
		return AlertEvidenceLink{}, err
	}
	if err := h.recordAlertEvidenceLinkAudit(ctx, tx, httpRequest, auditAction, link, request.Reason, desiredStatus, idempotencyKey); err != nil {
		return AlertEvidenceLink{}, err
	}
	if err := tx.Commit(); err != nil {
		return AlertEvidenceLink{}, err
	}
	return link, nil
}

func loadAlertEvidenceManifestForShare(
	ctx context.Context, tx *sql.Tx, tenantID, alertID, evidenceID string,
) (AlertEvidenceManifest, error) {
	row := tx.QueryRowContext(ctx, `SELECT tenant_id,alert_id,evidence_id,event_id,evidence_type,source_store,
		object_bucket,object_key,object_version,object_sha256,size_bytes,content_type,state,revision,
		source_watermarks::text,observed_at,expires_at
		FROM alert_evidence_manifests WHERE tenant_id=$1 AND alert_id=$2 AND evidence_id=$3 FOR SHARE`,
		tenantID, alertID, evidenceID)
	manifest, err := scanAlertEvidenceManifest(row)
	if err != nil {
		return AlertEvidenceManifest{}, err
	}
	return *manifest, nil
}

func alertEvidenceRequestMatchesManifest(request alertEvidenceLinkRequest, manifest AlertEvidenceManifest) bool {
	return request.SourceStore == manifest.SourceStore && request.ObjectBucket == manifest.ObjectBucket &&
		request.ObjectKey == manifest.ObjectKey && request.ObjectVersion == manifest.ObjectVersion &&
		request.ObjectSHA256 == manifest.ObjectSHA256
}

func alertEvidenceLinkMatchesManifest(link AlertEvidenceLink, manifest AlertEvidenceManifest) bool {
	return link.EvidenceType == manifest.EvidenceType && link.SourceStore == manifest.SourceStore &&
		link.ObjectBucket == manifest.ObjectBucket && link.ObjectKey == manifest.ObjectKey &&
		link.ObjectVersion == manifest.ObjectVersion && link.ObjectSHA256 == manifest.ObjectSHA256 &&
		link.SizeBytes == manifest.SizeBytes && link.ContentType == manifest.ContentType
}

func loadAlertEvidenceLinkForUpdate(
	ctx context.Context, tx *sql.Tx, tenantID, alertID, evidenceID string,
) (AlertEvidenceLink, bool, error) {
	var link AlertEvidenceLink
	err := tx.QueryRowContext(ctx, `SELECT relation_id::text,tenant_id,alert_id,evidence_id,evidence_type,
		source_store,object_bucket,object_key,object_version,object_sha256,size_bytes,content_type,status,
		revision,last_event_id::text,created_at,updated_at
		FROM alert_evidence_links WHERE tenant_id=$1 AND alert_id=$2 AND evidence_id=$3 FOR UPDATE`,
		tenantID, alertID, evidenceID).Scan(&link.RelationID, &link.TenantID, &link.AlertID, &link.EvidenceID,
		&link.EvidenceType, &link.SourceStore, &link.ObjectBucket, &link.ObjectKey, &link.ObjectVersion,
		&link.ObjectSHA256, &link.SizeBytes, &link.ContentType, &link.Status, &link.Revision,
		&link.EventID, &link.CreatedAt, &link.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AlertEvidenceLink{}, false, nil
	}
	return link, err == nil, err
}

func loadAlertEvidenceLinkCommand(
	ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey string,
) (AlertEvidenceLink, bool, string, error) {
	var requestSHA string
	var resultJSON []byte
	err := tx.QueryRowContext(ctx, `SELECT request_sha256,result FROM alert_evidence_link_commands
		WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`, tenantID, idempotencyKey).Scan(&requestSHA, &resultJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return AlertEvidenceLink{}, false, "", nil
	}
	if err != nil {
		return AlertEvidenceLink{}, false, "", err
	}
	var result AlertEvidenceLink
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return AlertEvidenceLink{}, false, "", err
	}
	return result, true, requestSHA, nil
}

func insertAlertEvidenceLinkCommand(
	ctx context.Context, tx *sql.Tx, tenantID string, operation alertEvidenceLinkOperation,
	idempotencyKey, requestSHA string, expectedRevision int64, link AlertEvidenceLink, userID string,
) error {
	resultJSON, err := json.Marshal(link)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO alert_evidence_link_commands
		(command_id,tenant_id,relation_id,alert_id,evidence_id,operation,idempotency_key,request_sha256,
		 expected_revision,relation_revision,result,created_by)
		VALUES ($1::uuid,$2,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12)`,
		uuid.NewString(), tenantID, link.RelationID, link.AlertID, link.EvidenceID, string(operation),
		idempotencyKey, requestSHA, expectedRevision, link.Revision, string(resultJSON), userID)
	return err
}

func (h *Handler) recordAlertEvidenceLinkAudit(
	ctx context.Context, tx *sql.Tx, request *http.Request, action string,
	link AlertEvidenceLink, reason, result, idempotencyKey string,
) error {
	return h.actionAudit.recordWithExecutor(ctx, tx, request, AlertActionAuditRecord{
		Action: action, ObjectType: "alert_evidence_link", ObjectID: link.RelationID,
		TenantID: link.TenantID, UserID: h.extractUserID(request), AlertID: link.AlertID,
		Reason: reason, Result: result,
		Detail: map[string]interface{}{
			"evidence_id": link.EvidenceID, "event_id": link.EventID, "revision": link.Revision,
			"object_version": link.ObjectVersion, "object_sha256": link.ObjectSHA256,
			"idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
		},
	})
}

func verifyAlertEvidenceLinkSchemaTx(ctx context.Context, tx *sql.Tx) error {
	var ready bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM alignment_schema_migrations WHERE version=$1
	) AND to_regclass('public.alert_evidence_links') IS NOT NULL
	  AND to_regclass('public.alert_evidence_link_history') IS NOT NULL
	  AND to_regclass('public.alert_evidence_link_commands') IS NOT NULL
	  AND to_regclass('public.alert_evidence_link_outbox') IS NOT NULL
	  AND to_regclass('public.alert_evidence_link_projection_inbox') IS NOT NULL
	  AND to_regclass('public.alert_evidence_link_projection_deliveries') IS NOT NULL
	  AND to_regclass('public.alert_evidence_link_projection_watermarks') IS NOT NULL`, alertEvidenceLinkSchemaVersion).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("alert evidence link schema %s is unavailable", alertEvidenceLinkSchemaVersion)
	}
	return nil
}

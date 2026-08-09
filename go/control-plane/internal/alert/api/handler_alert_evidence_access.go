package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"strconv"
	"strings"
	"time"

	alertservice "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/miniohttp"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

const alertEvidenceDownloadTTL = 5 * time.Minute

var (
	errAlertEvidenceAmbiguous         = stderrors.New("alert evidence request is ambiguous")
	errEvidenceObjectRefUnavailable   = stderrors.New("evidence does not reference an original object")
	errEvidenceObjectStoreUnavailable = stderrors.New("evidence object store is unavailable")
	errEvidenceManifestMissing        = stderrors.New("evidence manifest is missing")
	errEvidenceManifestExpired        = stderrors.New("evidence manifest has expired")
	errEvidenceIntegrityFailed        = stderrors.New("evidence object integrity validation failed")
)

type alertEvidenceObjectInfo struct {
	Size        int64
	ContentType string
	VersionID   string
	SHA256      string
}

type alertEvidenceObjectStore interface {
	Stat(context.Context, string, string, string) (alertEvidenceObjectInfo, error)
	Open(context.Context, string, string, string) (io.ReadCloser, error)
}

type minioAlertEvidenceObjectStore struct {
	client *minio.Client
}

type alertEvidenceObjectReference struct {
	Bucket      string
	Key         string
	VersionID   string
	FileName    string
	ContentType string
}

type alertEvidenceAccessRequest struct {
	Target string                 `json:"target"`
	Reason string                 `json:"reason"`
	Detail map[string]interface{} `json:"detail,omitempty"`
}

func (h *Handler) CreateAlertEvidenceAccess(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireAlertReadPermission(w, r) {
		return
	}
	tenantID := h.extractTenantID(r)
	alertID := strings.TrimSpace(mux.Vars(r)["id"])
	if tenantID == "" || alertID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "ALERT_REQUIRED", "tenant_id and alert id are required")
		return
	}
	var request alertEvidenceAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if len(request.Reason) < 4 || len(request.Reason) > 1000 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "REASON_REQUIRED", "reason must contain between 4 and 1000 characters")
		return
	}
	evidenceID := strings.TrimSpace(request.Target)
	if value, ok := request.Detail["evidence_id"].(string); ok && strings.TrimSpace(value) != "" {
		evidenceID = strings.TrimSpace(value)
	}
	if evidenceID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "EVIDENCE_REQUIRED", "evidence id is required")
		return
	}
	if h.alertService == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "EVIDENCE_UNAVAILABLE", "alert evidence service is unavailable")
		return
	}
	evidence, err := h.resolveAlertEvidence(ctx, tenantID, alertID, evidenceID)
	if err != nil {
		if stderrors.Is(err, errAlertEvidenceAmbiguous) {
			httpx.JSONError(w, ctx, http.StatusConflict, "EVIDENCE_AMBIGUOUS", "multiple evidence records match this file; select the exact evidence id")
		} else if commonerrors.IsCode(err, commonerrors.ErrCodeResourceNotFound) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "EVIDENCE_NOT_FOUND", "evidence not found")
		} else {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "EVIDENCE_LOOKUP_FAILED", "failed to validate evidence")
		}
		return
	}
	var manifest *AlertEvidenceManifest
	var objectRef alertEvidenceObjectReference
	if h.alertEvidenceChainEnabled {
		if h.evidenceManifests == nil {
			h.writeAlertEvidenceAccessFailure(w, r, tenantID, alertID, evidence.EvidenceID, http.StatusServiceUnavailable, "EVIDENCE_MANIFEST_UNAVAILABLE", "evidence manifest storage is unavailable")
			return
		}
		manifest, err = h.evidenceManifests.Get(ctx, tenantID, alertID, evidence.EvidenceID)
		if err != nil {
			if stderrors.Is(err, sql.ErrNoRows) {
				h.writeAlertEvidenceAccessFailure(w, r, tenantID, alertID, evidence.EvidenceID, http.StatusUnprocessableEntity, "EVIDENCE_MANIFEST_MISSING", "the evidence object has no authoritative manifest")
			} else {
				h.writeAlertEvidenceAccessFailure(w, r, tenantID, alertID, evidence.EvidenceID, http.StatusServiceUnavailable, "EVIDENCE_MANIFEST_UNAVAILABLE", "evidence manifest storage is unavailable")
			}
			return
		}
		if err = validateAlertEvidenceManifest(manifest, evidence, time.Now().UTC()); err != nil {
			code := "EVIDENCE_INTEGRITY_FAILED"
			status := http.StatusUnprocessableEntity
			if stderrors.Is(err, errEvidenceManifestExpired) {
				code = "EVIDENCE_EXPIRED"
				status = http.StatusGone
			}
			h.writeAlertEvidenceAccessFailure(w, r, tenantID, alertID, evidence.EvidenceID, status, code, err.Error())
			return
		}
		objectRef, err = alertEvidenceManifestObject(manifest)
	} else {
		objectRef, err = alertEvidenceOriginalObject(evidence)
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusUnprocessableEntity, "EVIDENCE_OBJECT_UNAVAILABLE", "the original evidence object is not available for download")
		return
	}
	objectStore, err := h.alertEvidenceObjectStore()
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "EVIDENCE_STORAGE_UNAVAILABLE", "evidence object storage is unavailable")
		return
	}
	objectInfo, err := objectStore.Stat(ctx, objectRef.Bucket, objectRef.Key, objectRef.VersionID)
	if err != nil {
		if alertEvidenceObjectNotFound(err) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "EVIDENCE_OBJECT_NOT_FOUND", "the original evidence file was not found in object storage")
		} else {
			httpx.JSONError(w, ctx, http.StatusBadGateway, "EVIDENCE_STORAGE_FAILED", "object storage could not validate the original evidence file")
		}
		return
	}
	if manifest != nil {
		if err := verifyAlertEvidenceObjectIntegrity(ctx, objectStore, objectRef, objectInfo, manifest); err != nil {
			h.writeAlertEvidenceAccessFailure(w, r, tenantID, alertID, evidence.EvidenceID, http.StatusConflict, "EVIDENCE_INTEGRITY_FAILED", err.Error())
			return
		}
	}
	if objectInfo.ContentType != "" {
		objectRef.ContentType = objectInfo.ContentType
	}
	secret := h.alertEvidenceSigningSecret()
	if secret == "" {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "SIGNING_UNAVAILABLE", "evidence download signing is unavailable")
		return
	}
	expiresAt := time.Now().UTC().Add(alertEvidenceDownloadTTL)
	expires := expiresAt.Unix()
	query := url.Values{"expires": []string{strconv.FormatInt(expires, 10)}}
	if manifest != nil {
		query.Set("manifest_revision", strconv.FormatInt(manifest.Revision, 10))
		query.Set("object_sha256", manifest.ObjectSHA256)
		query.Set("signature", signAlertEvidenceDownloadV2(secret, tenantID, alertID, evidence.EvidenceID, expires, manifest.Revision, manifest.ObjectSHA256))
	} else {
		query.Set("signature", signAlertEvidenceDownload(secret, tenantID, alertID, evidence.EvidenceID, expires))
	}
	downloadURL := fmt.Sprintf("/v1/alerts/%s/evidence/%s/download?%s", url.PathEscape(alertID), url.PathEscape(evidence.EvidenceID), query.Encode())
	jobID := "evidence-access-" + uuid.NewString()
	detail := cloneActionDetail(request.Detail)
	detail["job_id"] = jobID
	detail["evidence_id"] = evidence.EvidenceID
	detail["access_mode"] = "download"
	detail["expires_at"] = expiresAt.Format(time.RFC3339)
	if manifest != nil {
		detail["manifest_revision"] = manifest.Revision
		detail["object_sha256"] = manifest.ObjectSHA256
		detail["object_version"] = manifest.ObjectVersion
	}
	auditRecord := AlertActionAuditRecord{
		Action:     "ALERT_EVIDENCE_ACCESS_REQUESTED",
		ObjectType: "alert_evidence",
		ObjectID:   evidence.EvidenceID,
		TenantID:   tenantID,
		UserID:     h.extractUserID(r),
		AlertID:    alertID,
		Reason:     request.Reason,
		Result:     "recorded",
		Detail:     detail,
	}
	if h.alertEvidenceChainEnabled {
		if h.actionAudit == nil {
			httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "evidence access audit storage is unavailable")
			return
		}
		if err := h.actionAudit.Record(ctx, r, auditRecord); err != nil {
			if h.logger != nil {
				h.logger.Warn("Failed to durably authorize alert evidence access", zap.Error(err))
			}
			httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "evidence access could not be durably audited")
			return
		}
	} else {
		h.recordAlertActionAudit(ctx, r, auditRecord)
	}
	httpx.JSONCreated(w, ctx, map[string]interface{}{
		"job_id":       jobID,
		"status":       "recorded",
		"target":       evidence.EvidenceID,
		"evidence_id":  evidence.EvidenceID,
		"file_name":    objectRef.FileName,
		"content_type": objectRef.ContentType,
		"size_bytes":   objectInfo.Size,
		"object_sha256": func() string {
			if manifest != nil {
				return manifest.ObjectSHA256
			}
			return objectInfo.SHA256
		}(),
		"manifest_revision": func() int64 {
			if manifest != nil {
				return manifest.Revision
			}
			return 0
		}(),
		"download_url": downloadURL,
		"expires_at":   expiresAt.Format(time.RFC3339),
		"audit_event":  "ALERT_EVIDENCE_ACCESS_REQUESTED",
	})
}

func (h *Handler) resolveAlertEvidence(ctx context.Context, tenantID, alertID, requested string) (*alertservice.EvidenceDTO, error) {
	evidence, err := h.alertService.GetEvidenceByID(ctx, tenantID, alertID, requested)
	if err == nil {
		return evidence, nil
	}
	items, listErr := h.alertService.GetEvidence(ctx, tenantID, alertID)
	if listErr != nil {
		return nil, listErr
	}
	aliasMatches := make([]*alertservice.EvidenceDTO, 0, 1)
	for _, item := range items {
		if alertEvidenceAliasMatches(item, requested) {
			aliasMatches = append(aliasMatches, item)
		}
	}
	if len(aliasMatches) == 1 {
		return aliasMatches[0], nil
	}
	if len(aliasMatches) > 1 {
		return nil, errAlertEvidenceAmbiguous
	}
	return nil, err
}

func alertEvidenceAliasMatches(evidence *alertservice.EvidenceDTO, requested string) bool {
	if evidence == nil {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(requested))
	if normalized == "" {
		return false
	}
	candidates := []string{evidence.EvidenceID, evidence.ArkimeLink, evidence.VisualizationURL}
	for _, value := range evidence.SnippetRef {
		candidates = append(candidates, value)
	}
	for key, value := range evidence.Metrics {
		switch typed := value.(type) {
		case string:
			candidates = append(candidates, typed)
		default:
			if strings.Contains(strings.ToLower(key), "file") || strings.Contains(strings.ToLower(key), "path") {
				candidates = append(candidates, fmt.Sprint(value))
			}
		}
	}
	for _, candidate := range candidates {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == normalized || strings.HasSuffix(candidate, "/"+normalized) {
			return true
		}
	}
	return false
}

func (h *Handler) DownloadAlertEvidence(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireAlertReadPermission(w, r) {
		return
	}
	tenantID := h.extractTenantID(r)
	alertID := strings.TrimSpace(mux.Vars(r)["id"])
	evidenceID := strings.TrimSpace(mux.Vars(r)["evidence_id"])
	expires, err := strconv.ParseInt(r.URL.Query().Get("expires"), 10, 64)
	if err != nil || expires <= time.Now().UTC().Unix() || expires > time.Now().UTC().Add(10*time.Minute).Unix() {
		httpx.JSONError(w, ctx, http.StatusForbidden, "DOWNLOAD_EXPIRED", "evidence download link has expired")
		return
	}
	secret := h.alertEvidenceSigningSecret()
	provided := r.URL.Query().Get("signature")
	manifestRevision := int64(0)
	manifestSHA256 := ""
	expected := ""
	if h.alertEvidenceChainEnabled {
		manifestRevision, err = strconv.ParseInt(r.URL.Query().Get("manifest_revision"), 10, 64)
		manifestSHA256 = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("object_sha256")))
		if err != nil || manifestRevision < 1 || !evidenceValidSHA256Hex(manifestSHA256) {
			httpx.JSONError(w, ctx, http.StatusForbidden, "INVALID_SIGNATURE", "invalid evidence download signature")
			return
		}
		expected = signAlertEvidenceDownloadV2(secret, tenantID, alertID, evidenceID, expires, manifestRevision, manifestSHA256)
	} else {
		expected = signAlertEvidenceDownload(secret, tenantID, alertID, evidenceID, expires)
	}
	if secret == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		httpx.JSONError(w, ctx, http.StatusForbidden, "INVALID_SIGNATURE", "invalid evidence download signature")
		return
	}
	if h.alertService == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "EVIDENCE_UNAVAILABLE", "alert evidence service is unavailable")
		return
	}
	evidence, err := h.alertService.GetEvidenceByID(ctx, tenantID, alertID, evidenceID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusNotFound, "EVIDENCE_NOT_FOUND", "evidence not found")
		return
	}
	var manifest *AlertEvidenceManifest
	var objectRef alertEvidenceObjectReference
	if h.alertEvidenceChainEnabled {
		if h.evidenceManifests == nil {
			h.writeAlertEvidenceAccessFailure(w, r, tenantID, alertID, evidenceID, http.StatusServiceUnavailable, "EVIDENCE_MANIFEST_UNAVAILABLE", "evidence manifest storage is unavailable")
			return
		}
		manifest, err = h.evidenceManifests.Get(ctx, tenantID, alertID, evidenceID)
		if err != nil {
			h.writeAlertEvidenceAccessFailure(w, r, tenantID, alertID, evidenceID, http.StatusNotFound, "EVIDENCE_MANIFEST_MISSING", "the evidence object has no authoritative manifest")
			return
		}
		if manifest.Revision != manifestRevision || manifest.ObjectSHA256 != manifestSHA256 {
			h.writeAlertEvidenceAccessFailure(w, r, tenantID, alertID, evidenceID, http.StatusConflict, "EVIDENCE_MANIFEST_CHANGED", "the evidence manifest changed after access was granted")
			return
		}
		if err = validateAlertEvidenceManifest(manifest, evidence, time.Now().UTC()); err != nil {
			h.writeAlertEvidenceAccessFailure(w, r, tenantID, alertID, evidenceID, http.StatusConflict, "EVIDENCE_INTEGRITY_FAILED", err.Error())
			return
		}
		objectRef, err = alertEvidenceManifestObject(manifest)
	} else {
		objectRef, err = alertEvidenceOriginalObject(evidence)
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusUnprocessableEntity, "EVIDENCE_OBJECT_UNAVAILABLE", "the original evidence object is not available for download")
		return
	}
	objectStore, err := h.alertEvidenceObjectStore()
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "EVIDENCE_STORAGE_UNAVAILABLE", "evidence object storage is unavailable")
		return
	}
	objectInfo, err := objectStore.Stat(ctx, objectRef.Bucket, objectRef.Key, objectRef.VersionID)
	if err != nil {
		h.recordAlertEvidenceDownloadAudit(ctx, r, tenantID, alertID, evidenceID, objectRef, 0, "failed", "stat_failed")
		if alertEvidenceObjectNotFound(err) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "EVIDENCE_OBJECT_NOT_FOUND", "the original evidence file was not found in object storage")
		} else {
			httpx.JSONError(w, ctx, http.StatusBadGateway, "EVIDENCE_STORAGE_FAILED", "object storage could not validate the original evidence file")
		}
		return
	}
	if manifest != nil {
		if err := verifyAlertEvidenceObjectIntegrity(ctx, objectStore, objectRef, objectInfo, manifest); err != nil {
			h.recordAlertEvidenceDownloadAudit(ctx, r, tenantID, alertID, evidenceID, objectRef, 0, "failed", "integrity_failed")
			httpx.JSONError(w, ctx, http.StatusConflict, "EVIDENCE_INTEGRITY_FAILED", err.Error())
			return
		}
	}
	if h.alertEvidenceChainEnabled {
		if h.actionAudit == nil {
			httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "evidence download audit storage is unavailable")
			return
		}
		if err := h.actionAudit.Record(ctx, r, AlertActionAuditRecord{
			Action: "ALERT_EVIDENCE_DOWNLOAD_STARTED", ObjectType: "alert_evidence", ObjectID: evidenceID,
			TenantID: tenantID, UserID: h.extractUserID(r), AlertID: alertID, Result: "recorded",
			Detail: map[string]interface{}{"evidence_id": evidenceID, "manifest_revision": manifestRevision, "object_sha256": manifestSHA256},
		}); err != nil {
			httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "evidence download could not be durably audited")
			return
		}
	}
	reader, err := objectStore.Open(ctx, objectRef.Bucket, objectRef.Key, objectRef.VersionID)
	if err != nil {
		h.recordAlertEvidenceDownloadAudit(ctx, r, tenantID, alertID, evidenceID, objectRef, 0, "failed", "open_failed")
		httpx.JSONError(w, ctx, http.StatusBadGateway, "EVIDENCE_DOWNLOAD_FAILED", "failed to open the original evidence file")
		return
	}
	defer reader.Close()
	contentType := objectInfo.ContentType
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = objectRef.ContentType
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, objectRef.FileName))
	w.Header().Set("Content-Length", strconv.FormatInt(objectInfo.Size, 10))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	bytesWritten, streamErr := io.Copy(w, reader)
	auditResult := "success"
	if streamErr != nil {
		auditResult = "failed"
	}
	h.recordAlertEvidenceDownloadAudit(ctx, r, tenantID, alertID, evidenceID, objectRef, bytesWritten, auditResult, "")
	if streamErr != nil && h.logger != nil {
		h.logger.Warn("failed to stream original alert evidence",
			zap.String("alert_id", alertID),
			zap.String("evidence_id", evidenceID),
			zap.Error(streamErr),
		)
	}
}

func (h *Handler) recordAlertEvidenceDownloadAudit(
	ctx context.Context,
	r *http.Request,
	tenantID, alertID, evidenceID string,
	objectRef alertEvidenceObjectReference,
	bytes int64,
	result, errorCode string,
) {
	detail := map[string]interface{}{
		"evidence_id": evidenceID,
		"bytes":       bytes,
		"bucket":      objectRef.Bucket,
		"object":      objectRef.Key,
	}
	if errorCode != "" {
		detail["error_code"] = errorCode
	}
	h.recordAlertActionAudit(ctx, r, AlertActionAuditRecord{
		Action:     "ALERT_EVIDENCE_DOWNLOADED",
		ObjectType: "alert_evidence",
		ObjectID:   evidenceID,
		TenantID:   tenantID,
		UserID:     h.extractUserID(r),
		AlertID:    alertID,
		Result:     result,
		Detail:     detail,
	})
}

func (h *Handler) writeAlertEvidenceAccessFailure(
	w http.ResponseWriter,
	r *http.Request,
	tenantID, alertID, evidenceID string,
	status int,
	code, message string,
) {
	ctx := r.Context()
	h.recordAlertActionAudit(ctx, r, AlertActionAuditRecord{
		Action:     "ALERT_EVIDENCE_ACCESS_DENIED",
		ObjectType: "alert_evidence",
		ObjectID:   evidenceID,
		TenantID:   tenantID,
		UserID:     h.extractUserID(r),
		AlertID:    alertID,
		Result:     "failed",
		Detail: map[string]interface{}{
			"evidence_id": evidenceID,
			"error_code":  code,
		},
	})
	httpx.JSONError(w, ctx, status, code, message)
}

func validateAlertEvidenceManifest(manifest *AlertEvidenceManifest, evidence *alertservice.EvidenceDTO, now time.Time) error {
	if manifest == nil {
		return errEvidenceManifestMissing
	}
	if evidence == nil || manifest.TenantID != evidence.TenantID || manifest.AlertID != evidence.AlertID || manifest.EvidenceID != evidence.EvidenceID {
		return fmt.Errorf("%w: tenant, alert or evidence identity mismatch", errEvidenceIntegrityFailed)
	}
	if manifest.Revision < 1 || strings.TrimSpace(manifest.EvidenceType) == "" || strings.TrimSpace(manifest.SourceStore) == "" {
		return fmt.Errorf("%w: manifest identity or revision is incomplete", errEvidenceIntegrityFailed)
	}
	if evidence.EventID != "" && manifest.EventID != "" && evidence.EventID != manifest.EventID {
		return fmt.Errorf("%w: detection event identity mismatch", errEvidenceIntegrityFailed)
	}
	if manifest.State != "available" {
		if manifest.State == "expired" {
			return errEvidenceManifestExpired
		}
		return fmt.Errorf("%w: manifest state is %s", errEvidenceIntegrityFailed, manifest.State)
	}
	if manifest.ExpiresAt != nil && !now.Before(*manifest.ExpiresAt) {
		return errEvidenceManifestExpired
	}
	if manifest.SourceStore == "minio" {
		if strings.TrimSpace(manifest.ObjectBucket) == "" || strings.TrimSpace(manifest.ObjectKey) == "" || !evidenceValidSHA256Hex(manifest.ObjectSHA256) || manifest.SizeBytes < 0 {
			return fmt.Errorf("%w: MinIO identity, size or checksum is incomplete", errEvidenceIntegrityFailed)
		}
		tenantPrefix := "tenants/" + manifest.TenantID + "/"
		if !strings.HasPrefix(manifest.ObjectKey, tenantPrefix) || strings.Contains(manifest.ObjectKey, "..") {
			return fmt.Errorf("%w: object key is outside the tenant prefix", errEvidenceIntegrityFailed)
		}
	}
	return nil
}

func alertEvidenceManifestObject(manifest *AlertEvidenceManifest) (alertEvidenceObjectReference, error) {
	if manifest == nil || manifest.SourceStore != "minio" {
		return alertEvidenceObjectReference{}, errEvidenceObjectRefUnavailable
	}
	fileName := evidenceDownloadFileName(pathpkg.Base(manifest.ObjectKey))
	contentType := strings.TrimSpace(manifest.ContentType)
	if contentType == "" {
		contentType = mime.TypeByExtension(strings.ToLower(pathpkg.Ext(fileName)))
	}
	if strings.HasSuffix(strings.ToLower(fileName), ".pcap") {
		contentType = "application/vnd.tcpdump.pcap"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return alertEvidenceObjectReference{
		Bucket:      manifest.ObjectBucket,
		Key:         manifest.ObjectKey,
		VersionID:   manifest.ObjectVersion,
		FileName:    fileName,
		ContentType: contentType,
	}, nil
}

func verifyAlertEvidenceObjectIntegrity(
	ctx context.Context,
	store alertEvidenceObjectStore,
	ref alertEvidenceObjectReference,
	info alertEvidenceObjectInfo,
	manifest *AlertEvidenceManifest,
) error {
	if manifest == nil || store == nil {
		return errEvidenceIntegrityFailed
	}
	if info.Size != manifest.SizeBytes {
		return fmt.Errorf("%w: object size is %d, manifest requires %d", errEvidenceIntegrityFailed, info.Size, manifest.SizeBytes)
	}
	if manifest.ObjectVersion != "" && info.VersionID != "" && manifest.ObjectVersion != info.VersionID {
		return fmt.Errorf("%w: object version changed", errEvidenceIntegrityFailed)
	}
	actualSHA256 := normalizeObjectSHA256(info.SHA256)
	if actualSHA256 == "" {
		reader, err := store.Open(ctx, ref.Bucket, ref.Key, ref.VersionID)
		if err != nil {
			return fmt.Errorf("%w: open object for checksum: %v", errEvidenceIntegrityFailed, err)
		}
		hasher := sha256.New()
		readBytes, readErr := io.Copy(hasher, io.LimitReader(reader, manifest.SizeBytes+1))
		closeErr := reader.Close()
		if readErr != nil {
			return fmt.Errorf("%w: hash object: %v", errEvidenceIntegrityFailed, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("%w: close object after checksum: %v", errEvidenceIntegrityFailed, closeErr)
		}
		if readBytes != manifest.SizeBytes {
			return fmt.Errorf("%w: streamed object size is %d, manifest requires %d", errEvidenceIntegrityFailed, readBytes, manifest.SizeBytes)
		}
		actualSHA256 = hex.EncodeToString(hasher.Sum(nil))
	}
	if subtle.ConstantTimeCompare([]byte(actualSHA256), []byte(manifest.ObjectSHA256)) != 1 {
		return fmt.Errorf("%w: object sha256 mismatch", errEvidenceIntegrityFailed)
	}
	return nil
}

func alertEvidenceObjectNotFound(err error) bool {
	response := minio.ToErrorResponse(err)
	switch response.Code {
	case "NoSuchBucket", "NoSuchKey", "NoSuchObject", "NotFound":
		return true
	default:
		return response.StatusCode == http.StatusNotFound
	}
}

func alertEvidenceOriginalObject(evidence *alertservice.EvidenceDTO) (alertEvidenceObjectReference, error) {
	if evidence == nil {
		return alertEvidenceObjectReference{}, errEvidenceObjectRefUnavailable
	}
	bucket := strings.TrimSpace(evidence.SnippetRef["bucket"])
	key := strings.TrimSpace(evidence.SnippetRef["object"])
	versionID := strings.TrimSpace(evidence.SnippetRef["version_id"])
	for _, candidate := range []interface{}{
		evidence.Metrics["object_path"],
		evidence.Metrics["objectPath"],
		evidence.Metrics["minio_path"],
		evidence.Metrics["minioPath"],
	} {
		value := strings.TrimSpace(fmt.Sprint(candidate))
		if !strings.HasPrefix(value, "minio://") {
			continue
		}
		parsed, err := url.Parse(value)
		if err == nil && parsed.Host != "" && strings.TrimPrefix(parsed.Path, "/") != "" {
			if bucket == "" {
				bucket = parsed.Host
			}
			if key == "" {
				key = strings.TrimPrefix(parsed.Path, "/")
			}
		}
	}
	if bucket == "" || key == "" || strings.Contains(key, "..") {
		return alertEvidenceObjectReference{}, errEvidenceObjectRefUnavailable
	}
	fileName := evidenceDownloadFileName(pathpkg.Base(key))
	extension := strings.ToLower(pathpkg.Ext(fileName))
	contentType := mime.TypeByExtension(extension)
	if extension == ".pcap" {
		contentType = "application/vnd.tcpdump.pcap"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return alertEvidenceObjectReference{
		Bucket:      bucket,
		Key:         key,
		VersionID:   versionID,
		FileName:    fileName,
		ContentType: contentType,
	}, nil
}

func (h *Handler) alertEvidenceObjectStore() (alertEvidenceObjectStore, error) {
	if h.evidenceObjects != nil {
		return h.evidenceObjects, nil
	}
	endpoint := strings.TrimSpace(os.Getenv("S3_ENDPOINT"))
	if endpoint == "" {
		endpoint = "minio.minio.svc:9000"
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	accessKey := strings.TrimSpace(os.Getenv("S3_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("S3_SECRET_KEY"))
	if accessKey == "" || secretKey == "" {
		return nil, errEvidenceObjectStoreUnavailable
	}
	secure := strings.EqualFold(strings.TrimSpace(os.Getenv("S3_USE_SSL")), "true")
	transport, err := miniohttp.NewTransport(secure, os.Getenv("S3_CA_CERT"))
	if err != nil {
		return nil, fmt.Errorf("%w: configure MinIO TLS: %v", errEvidenceObjectStoreUnavailable, err)
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:    secure,
		Transport: transport,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errEvidenceObjectStoreUnavailable, err)
	}
	return &minioAlertEvidenceObjectStore{client: client}, nil
}

func (store *minioAlertEvidenceObjectStore) Stat(ctx context.Context, bucket, key, versionID string) (alertEvidenceObjectInfo, error) {
	options := minio.StatObjectOptions{VersionID: strings.TrimSpace(versionID), Checksum: true}
	info, err := store.client.StatObject(ctx, bucket, key, options)
	if err != nil {
		return alertEvidenceObjectInfo{}, err
	}
	digest := normalizeObjectSHA256(info.ChecksumSHA256)
	if digest == "" {
		for _, key := range []string{"sha256", "object-sha256", "content-sha256", "x-amz-meta-sha256"} {
			if digest = normalizeObjectSHA256(info.UserMetadata[key]); digest != "" {
				break
			}
			if digest = normalizeObjectSHA256(info.Metadata.Get(key)); digest != "" {
				break
			}
		}
	}
	return alertEvidenceObjectInfo{Size: info.Size, ContentType: info.ContentType, VersionID: info.VersionID, SHA256: digest}, nil
}

func (store *minioAlertEvidenceObjectStore) Open(ctx context.Context, bucket, key, versionID string) (io.ReadCloser, error) {
	object, err := store.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{VersionID: strings.TrimSpace(versionID), Checksum: true})
	if err != nil {
		return nil, err
	}
	return object, nil
}

func alertEvidenceDownloadSecret() string {
	for _, key := range []string{"ALERT_EVIDENCE_DOWNLOAD_SECRET", "JWT_SECRET_KEY", "JWT_SIGNING_KEY"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func (h *Handler) alertEvidenceSigningSecret() string {
	if h != nil && h.alertEvidenceChainEnabled {
		return strings.TrimSpace(os.Getenv("ALERT_EVIDENCE_DOWNLOAD_SECRET"))
	}
	return alertEvidenceDownloadSecret()
}

func signAlertEvidenceDownload(secret, tenantID, alertID, evidenceID string, expires int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(fmt.Sprintf("%s\n%s\n%s\n%d", tenantID, alertID, evidenceID, expires)))
	return hex.EncodeToString(mac.Sum(nil))
}

func signAlertEvidenceDownloadV2(secret, tenantID, alertID, evidenceID string, expires, manifestRevision int64, objectSHA256 string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(fmt.Sprintf("v2\n%s\n%s\n%s\n%d\n%d\n%s", tenantID, alertID, evidenceID, expires, manifestRevision, strings.ToLower(strings.TrimSpace(objectSHA256)))))
	return hex.EncodeToString(mac.Sum(nil))
}

func normalizeObjectSHA256(value string) string {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	if evidenceValidSHA256Hex(value) {
		return strings.ToLower(value)
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil && len(decoded) == sha256.Size {
		return hex.EncodeToString(decoded)
	}
	return ""
}

func evidenceValidSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func evidenceDownloadFileName(evidenceID string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(evidenceID) {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '-' || char == '_' {
			builder.WriteRune(char)
		} else {
			builder.WriteRune('-')
		}
	}
	name := strings.Trim(builder.String(), ".-")
	if name == "" {
		name = "evidence"
	}
	if !strings.Contains(name, ".") {
		name += ".json"
	}
	return name
}

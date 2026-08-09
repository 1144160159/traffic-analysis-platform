package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"strconv"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const campaignReportMaxAttempts = 5

// CampaignReportModel is the one deterministic source used by every campaign
// report format. It is frozen in PostgreSQL before the command transaction is
// acknowledged, so the worker never rebuilds a report from moving projections.
type CampaignReportModel struct {
	SchemaVersion    int                      `json:"schema_version"`
	ContractVersion  int                      `json:"contract_version"`
	SnapshotID       string                   `json:"snapshot_id"`
	SourceSnapshotID string                   `json:"source_snapshot_id"`
	TenantID         string                   `json:"tenant_id"`
	CampaignID       string                   `json:"campaign_id"`
	CampaignRevision int64                    `json:"campaign_revision"`
	Status           string                   `json:"status"`
	Assignee         string                   `json:"assignee"`
	Summary          string                   `json:"summary"`
	Score            float64                  `json:"score"`
	CampaignType     string                   `json:"campaign_type"`
	Entities         []string                 `json:"entities"`
	AttackPhases     []string                 `json:"attack_phases"`
	RuleIDs          []string                 `json:"rule_ids"`
	ModelIDs         []string                 `json:"model_ids"`
	MemberAlertIDs   []string                 `json:"member_alert_ids"`
	MemberCount      int                      `json:"member_count"`
	TimeWindow       CampaignReportTimeWindow `json:"time_window"`
	Sections         []string                 `json:"sections"`
	EvidenceCount    int                      `json:"evidence_count"`
	MembershipSource string                   `json:"membership_source"`
	SourceWatermarks map[string]string        `json:"source_watermarks"`
}

type CampaignReportTimeWindow struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type campaignReportJob struct {
	ReportID         string                 `json:"report_id"`
	JobID            string                 `json:"job_id"`
	TenantID         string                 `json:"tenant_id"`
	CampaignID       string                 `json:"campaign_id"`
	Format           string                 `json:"format"`
	Status           string                 `json:"status"`
	CampaignRevision int64                  `json:"campaign_revision"`
	SnapshotID       string                 `json:"snapshot_id"`
	SnapshotSHA256   string                 `json:"snapshot_sha256"`
	ObjectManifest   map[string]interface{} `json:"object_manifest"`
	ObjectBucket     string                 `json:"-"`
	ObjectKey        string                 `json:"-"`
	MIMEType         string                 `json:"mime_type,omitempty"`
	ArtifactSHA256   string                 `json:"artifact_sha256,omitempty"`
	SizeBytes        int64                  `json:"size_bytes"`
	Attempts         int                    `json:"attempts"`
	ErrorMessage     string                 `json:"error_message,omitempty"`
	CreatedBy        string                 `json:"created_by"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	CompletedAt      *time.Time             `json:"completed_at"`
	SourceWatermarks map[string]string      `json:"-"`
}

func (h *SystemHandler) StartCampaignReportWorker(ctx context.Context, interval time.Duration) error {
	if !h.campaignAggregateV2 {
		return nil
	}
	if h.pgDB == nil || h.campaignAuditWriter == nil {
		return fmt.Errorf("campaign report persistence is unavailable")
	}
	if err := verifyCampaignAggregateV2Schema(ctx, h.pgDB); err != nil {
		return err
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := h.processNextCampaignReport(ctx); err != nil && ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Failed to process campaign report", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (h *SystemHandler) processNextCampaignReport(ctx context.Context) error {
	workerID := fmt.Sprintf("%s-%d", hostnameOrDefault(), os.Getpid())
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var job campaignReportJob
	var snapshot []byte
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
		SELECT report_id FROM campaign_reports
		WHERE (status='accepted' AND next_attempt_at <= now())
		   OR (status='running' AND locked_until < now())
		ORDER BY created_at,report_id
		LIMIT 1 FOR UPDATE SKIP LOCKED
	)
	UPDATE campaign_reports r
	SET status='running',attempts=r.attempts+1,locked_until=now()+interval '5 minutes',
	    locked_by=$1,updated_at=now()
	FROM candidate c WHERE r.report_id=c.report_id
	RETURNING r.report_id,COALESCE(r.job_id,''),r.tenant_id,r.campaign_id,r.format,
	          r.campaign_revision,r.snapshot_id::text,r.snapshot::text,r.snapshot_sha256,
	          r.attempts,r.created_by`, workerID).Scan(
		&job.ReportID, &job.JobID, &job.TenantID, &job.CampaignID, &job.Format,
		&job.CampaignRevision, &job.SnapshotID, &snapshot, &job.SnapshotSHA256,
		&job.Attempts, &job.CreatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	var model CampaignReportModel
	if err := json.Unmarshal(snapshot, &model); err != nil {
		return h.failCampaignReportJob(ctx, workerID, job, fmt.Errorf("invalid frozen campaign report model: %w", err))
	}
	canonical, digest, err := canonicalCampaignSnapshot(&model)
	if err != nil {
		return h.failCampaignReportJob(ctx, workerID, job, err)
	}
	if digest != job.SnapshotSHA256 || model.TenantID != job.TenantID || model.CampaignID != job.CampaignID ||
		model.CampaignRevision != job.CampaignRevision || model.SnapshotID != job.SnapshotID {
		return h.failCampaignReportJob(ctx, workerID, job, fmt.Errorf("frozen campaign report identity or checksum mismatch"))
	}
	content, mimeType, extension, err := buildCampaignReportArtifact(job.Format, model, canonical)
	if err != nil {
		return h.failCampaignReportJob(ctx, workerID, job, err)
	}
	store, err := h.campaignReportObjectStore()
	if err != nil {
		return h.failCampaignReportJob(ctx, workerID, job, err)
	}
	job.ObjectBucket = campaignReportBucket()
	job.ObjectKey = pathpkg.Join(safeObjectSegment(job.TenantID), "campaigns", safeObjectSegment(job.CampaignID), job.ReportID+"."+extension)
	job.MIMEType = mimeType
	job.SizeBytes = int64(len(content))
	job.ArtifactSHA256 = "sha256:" + hex.EncodeToString(sha256Sum(content))
	if err := store.Put(ctx, job.ObjectBucket, job.ObjectKey, bytes.NewReader(content), job.SizeBytes, mimeType); err != nil {
		return h.failCampaignReportJob(ctx, workerID, job, err)
	}
	return h.completeCampaignReportJob(ctx, workerID, job, model)
}

func (h *SystemHandler) completeCampaignReportJob(ctx context.Context, workerID string, job campaignReportJob, model CampaignReportModel) error {
	tx, err := h.pgDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status, lockedBy string
	if err := tx.QueryRowContext(ctx, `SELECT status,locked_by FROM campaign_reports WHERE report_id=$1 FOR UPDATE`, job.ReportID).Scan(&status, &lockedBy); err != nil {
		return err
	}
	if status != "running" || lockedBy != workerID {
		return fmt.Errorf("campaign report lease lost before manifest commit")
	}
	state, err := lockCampaignAggregateV2State(ctx, tx, job.TenantID, job.CampaignID)
	if err != nil {
		return err
	}
	state.Revision++
	eventID := uuid.NewString()
	manifest := map[string]interface{}{
		"status": "completed", "bucket": job.ObjectBucket, "key": job.ObjectKey,
		"mime_type": job.MIMEType, "sha256": job.ArtifactSHA256, "size_bytes": job.SizeBytes,
		"snapshot_id": job.SnapshotID, "snapshot_sha256": job.SnapshotSHA256,
	}
	manifestJSON, _ := json.Marshal(manifest)
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"report_id": job.ReportID, "report_status": "completed", "final_effect": true,
		"snapshot_id": job.SnapshotID, "snapshot_sha256": job.SnapshotSHA256,
		"artifact_sha256": job.ArtifactSHA256, "size_bytes": job.SizeBytes,
		"object_manifest_status": "completed", "resource_revision": state.Revision,
	})
	payload := campaignReportLifecyclePayload(eventID, "traffic.campaign.v2.ReportCompleted", job, state, map[string]interface{}{
		"artifact_sha256": job.ArtifactSHA256, "size_bytes": job.SizeBytes,
		"object_bucket": job.ObjectBucket, "object_key": job.ObjectKey,
	})
	payloadJSON, _ := json.Marshal(payload)
	reportUpdate, err := tx.ExecContext(ctx, `UPDATE campaign_reports SET
		status='completed',object_manifest=$2::jsonb,object_bucket=$3,object_key=$4,mime_type=$5,
		artifact_sha256=$6,size_bytes=$7,error_message='',locked_until=NULL,locked_by='',
		updated_at=now(),completed_at=now()
		WHERE report_id=$1 AND status='running'`, job.ReportID, string(manifestJSON), job.ObjectBucket,
		job.ObjectKey, job.MIMEType, job.ArtifactSHA256, job.SizeBytes)
	if err != nil {
		return err
	}
	if err := requireCampaignReportRow(reportUpdate, "complete campaign report"); err != nil {
		return err
	}
	actionUpdate, err := tx.ExecContext(ctx, `UPDATE campaign_action_jobs SET
		status='completed',result=COALESCE(result,'{}'::jsonb)||$3::jsonb,error_message='',
		resource_revision=$4,completed_at=now()
		WHERE tenant_id=$1 AND job_id=$2 AND action_id='campaign-report-generate'`,
		job.TenantID, job.JobID, string(resultJSON), state.Revision)
	if err != nil {
		return err
	}
	if err := requireCampaignReportRow(actionUpdate, "complete campaign report action job"); err != nil {
		return err
	}
	if err := appendCampaignReportLifecycle(ctx, tx, eventID, "traffic.campaign.v2.ReportCompleted", job, state, string(payloadJSON), "campaign report artifact committed"); err != nil {
		return err
	}
	if err := h.campaignAuditWriter.recordWithExecutor(ctx, tx, nil, AlertActionAuditRecord{
		Action: "CAMPAIGN_REPORT_COMPLETED", ObjectType: "campaign_report", ObjectID: job.ReportID,
		TenantID: job.TenantID, UserID: job.CreatedBy, Result: "completed",
		Detail: map[string]interface{}{
			"campaign_id": job.CampaignID, "job_id": job.JobID, "event_id": eventID,
			"report_campaign_revision": job.CampaignRevision, "resource_revision": state.Revision,
			"snapshot_id": model.SnapshotID, "snapshot_sha256": job.SnapshotSHA256,
			"artifact_sha256": job.ArtifactSHA256, "size_bytes": job.SizeBytes,
			"object_bucket": job.ObjectBucket, "object_key": job.ObjectKey,
		},
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *SystemHandler) failCampaignReportJob(ctx context.Context, workerID string, job campaignReportJob, cause error) error {
	message := strings.TrimSpace(cause.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	if job.Attempts < campaignReportMaxAttempts {
		result, err := h.pgDB.ExecContext(ctx, `UPDATE campaign_reports SET status='accepted',error_message=$3,
			next_attempt_at=now()+(LEAST(300,POWER(2,LEAST(attempts,8)))::text||' seconds')::interval,
			locked_until=NULL,locked_by='',updated_at=now()
			WHERE report_id=$1 AND status='running' AND locked_by=$2`, job.ReportID, workerID, message)
		if err != nil {
			return fmt.Errorf("campaign report failure %v; retry scheduling failed: %w", cause, err)
		}
		if err := requireCampaignReportRow(result, "reschedule campaign report"); err != nil {
			return fmt.Errorf("campaign report failure %v; retry scheduling failed: %w", cause, err)
		}
		return cause
	}
	tx, err := h.pgDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status, lockedBy string
	if err := tx.QueryRowContext(ctx, `SELECT status,locked_by FROM campaign_reports WHERE report_id=$1 FOR UPDATE`, job.ReportID).Scan(&status, &lockedBy); err != nil {
		return err
	}
	if status != "running" || lockedBy != workerID {
		return fmt.Errorf("campaign report lease lost before failure commit")
	}
	state, err := lockCampaignAggregateV2State(ctx, tx, job.TenantID, job.CampaignID)
	if err != nil {
		return err
	}
	state.Revision++
	eventID := uuid.NewString()
	payload := campaignReportLifecyclePayload(eventID, "traffic.campaign.v2.ReportFailed", job, state, map[string]interface{}{"error": message})
	payloadJSON, _ := json.Marshal(payload)
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"report_id": job.ReportID, "report_status": "failed", "final_effect": false,
		"error": message, "resource_revision": state.Revision,
	})
	reportUpdate, err := tx.ExecContext(ctx, `UPDATE campaign_reports SET status='failed',error_message=$3,
		locked_until=NULL,locked_by='',updated_at=now(),completed_at=now()
		WHERE report_id=$1 AND status='running' AND locked_by=$2`, job.ReportID, workerID, message)
	if err != nil {
		return err
	}
	if err := requireCampaignReportRow(reportUpdate, "fail campaign report"); err != nil {
		return err
	}
	actionUpdate, err := tx.ExecContext(ctx, `UPDATE campaign_action_jobs SET status='failed',
		result=COALESCE(result,'{}'::jsonb)||$3::jsonb,error_message=$4,resource_revision=$5,completed_at=now()
		WHERE tenant_id=$1 AND job_id=$2 AND action_id='campaign-report-generate'`, job.TenantID, job.JobID, string(resultJSON), message, state.Revision)
	if err != nil {
		return err
	}
	if err := requireCampaignReportRow(actionUpdate, "fail campaign report action job"); err != nil {
		return err
	}
	if err := appendCampaignReportLifecycle(ctx, tx, eventID, "traffic.campaign.v2.ReportFailed", job, state, string(payloadJSON), message); err != nil {
		return err
	}
	if err := h.campaignAuditWriter.recordWithExecutor(ctx, tx, nil, AlertActionAuditRecord{
		Action: "CAMPAIGN_REPORT_FAILED", ObjectType: "campaign_report", ObjectID: job.ReportID,
		TenantID: job.TenantID, UserID: job.CreatedBy, Result: "failed",
		Detail: map[string]interface{}{"campaign_id": job.CampaignID, "job_id": job.JobID,
			"event_id": eventID, "error": message, "resource_revision": state.Revision},
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return cause
}

func campaignReportLifecyclePayload(eventID, eventType string, job campaignReportJob, state campaignAggregateState, extra map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": eventType, "tenant_id": job.TenantID, "schema_version": 2,
		"aggregate_type": "campaign", "aggregate_id": job.CampaignID, "aggregate_version": state.Revision,
		"partition_key": job.TenantID + ":" + job.CampaignID, "campaign_id": job.CampaignID,
		"status": state.Status, "assignee": state.Assignee, "member_count": state.MemberCount,
		"job_id": job.JobID, "report_id": job.ReportID, "report_campaign_revision": job.CampaignRevision,
		"snapshot_id": job.SnapshotID, "snapshot_sha256": job.SnapshotSHA256,
		"trace_id": "campaign-report-worker:" + job.ReportID,
	}
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

func requireCampaignReportRow(result sql.Result, operation string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s row count: %w", operation, err)
	}
	if affected != 1 {
		return fmt.Errorf("%s affected %d rows instead of 1", operation, affected)
	}
	return nil
}

func appendCampaignReportLifecycle(ctx context.Context, tx *sql.Tx, eventID, eventType string, job campaignReportJob, state campaignAggregateState, payloadJSON, reason string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_workbench_state SET state_version=$3,last_event_id=$4::uuid,
		updated_by=$5,updated_at=now() WHERE tenant_id=$1 AND campaign_id=$2`,
		job.TenantID, job.CampaignID, state.Revision, eventID, job.CreatedBy); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_aggregate_history
		(event_id,tenant_id,campaign_id,aggregate_revision,event_type,status,assignee,member_count,payload,reason,created_by)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11)`,
		eventID, job.TenantID, job.CampaignID, state.Revision, eventType, state.Status,
		state.Assignee, state.MemberCount, payloadJSON, reason, job.CreatedBy); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO campaign_aggregate_outbox
		(event_id,tenant_id,aggregate_id,aggregate_revision,event_type,partition_key,payload)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::jsonb)`, eventID, job.TenantID, job.CampaignID,
		state.Revision, eventType, job.TenantID+":"+job.CampaignID, payloadJSON)
	return err
}

func (h *SystemHandler) GetCampaignReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireCampaignReadPermission(w, r) {
		return
	}
	job, err := h.loadCampaignReport(ctx, queryTenantID(r), strings.TrimSpace(mux.Vars(r)["id"]), strings.TrimSpace(mux.Vars(r)["report_id"]))
	if errors.Is(err, sql.ErrNoRows) {
		httpx.JSONError(w, ctx, http.StatusNotFound, "REPORT_NOT_FOUND", "campaign report not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load campaign report")
		return
	}
	writeCampaignReportResponse(w, ctx, job)
}

func (h *SystemHandler) DownloadCampaignReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !hasAnySystemPermission(ctx, authmodel.ScopeCampaignReport, authmodel.ScopeCampaignWrite, authmodel.ScopeAlertWrite) {
		httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "campaign:report required")
		return
	}
	job, err := h.loadCampaignReport(ctx, queryTenantID(r), strings.TrimSpace(mux.Vars(r)["id"]), strings.TrimSpace(mux.Vars(r)["report_id"]))
	if errors.Is(err, sql.ErrNoRows) {
		httpx.JSONError(w, ctx, http.StatusNotFound, "REPORT_NOT_FOUND", "campaign report not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load campaign report")
		return
	}
	if job.Status != "completed" || job.ObjectKey == "" {
		httpx.JSONError(w, ctx, http.StatusConflict, "REPORT_NOT_READY", "campaign report is not completed")
		return
	}
	if job.SizeBytes < 0 || job.SizeBytes > alertReportMaxDownloadSize {
		httpx.JSONError(w, ctx, http.StatusUnprocessableEntity, "REPORT_MANIFEST_INVALID", "campaign report size is outside the download budget")
		return
	}
	store, err := h.campaignReportObjectStore()
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "REPORT_STORAGE_UNAVAILABLE", "campaign report object storage is unavailable")
		return
	}
	reader, err := store.Open(ctx, job.ObjectBucket, job.ObjectKey)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadGateway, "REPORT_STORAGE_FAILED", "failed to open campaign report artifact")
		return
	}
	defer reader.Close()
	content, readErr := io.ReadAll(io.LimitReader(reader, alertReportMaxDownloadSize+1))
	actualSHA := "sha256:" + hex.EncodeToString(sha256Sum(content))
	if readErr != nil || int64(len(content)) != job.SizeBytes || actualSHA != job.ArtifactSHA256 {
		httpx.JSONError(w, ctx, http.StatusBadGateway, "REPORT_MANIFEST_MISMATCH", "campaign report artifact does not match its manifest")
		return
	}
	if h.campaignAuditWriter != nil {
		_ = h.campaignAuditWriter.recordWithExecutor(ctx, h.pgDB, r, AlertActionAuditRecord{
			Action: "CAMPAIGN_REPORT_DOWNLOADED", ObjectType: "campaign_report", ObjectID: job.ReportID,
			TenantID: job.TenantID, UserID: httpx.GetUserID(ctx), Result: "success",
			Detail: map[string]interface{}{"campaign_id": job.CampaignID, "artifact_sha256": job.ArtifactSHA256, "size_bytes": job.SizeBytes},
		})
	}
	filename := pathpkg.Base(job.ObjectKey)
	w.Header().Set("Content-Type", job.MIMEType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("X-Content-SHA256", job.ArtifactSHA256)
	http.ServeContent(w, r, filename, job.UpdatedAt, bytes.NewReader(content))
}

func (h *SystemHandler) loadCampaignReport(ctx context.Context, tenantID, campaignID, reportID string) (campaignReportJob, error) {
	if h.pgDB == nil {
		return campaignReportJob{}, fmt.Errorf("campaign report persistence is unavailable")
	}
	var job campaignReportJob
	var snapshotID sql.NullString
	var manifestJSON []byte
	var completedAt sql.NullTime
	err := h.pgDB.QueryRowContext(ctx, `SELECT report_id,COALESCE(job_id,''),tenant_id,campaign_id,format,status,
		campaign_revision,snapshot_id::text,snapshot_sha256,object_manifest::text,object_bucket,object_key,
		mime_type,artifact_sha256,size_bytes,attempts,error_message,created_by,created_at,updated_at,completed_at
		FROM campaign_reports WHERE tenant_id=$1 AND campaign_id=$2 AND report_id=$3`, tenantID, campaignID, reportID).Scan(
		&job.ReportID, &job.JobID, &job.TenantID, &job.CampaignID, &job.Format, &job.Status,
		&job.CampaignRevision, &snapshotID, &job.SnapshotSHA256, &manifestJSON, &job.ObjectBucket,
		&job.ObjectKey, &job.MIMEType, &job.ArtifactSHA256, &job.SizeBytes, &job.Attempts,
		&job.ErrorMessage, &job.CreatedBy, &job.CreatedAt, &job.UpdatedAt, &completedAt,
	)
	if err != nil {
		return campaignReportJob{}, err
	}
	job.SnapshotID = snapshotID.String
	job.ObjectManifest = map[string]interface{}{}
	if err := json.Unmarshal(manifestJSON, &job.ObjectManifest); err != nil {
		return campaignReportJob{}, err
	}
	if completedAt.Valid {
		value := completedAt.Time
		job.CompletedAt = &value
	}
	job.SourceWatermarks = map[string]string{
		"postgresql.campaign_reports.attempts":          strconv.Itoa(job.Attempts),
		"postgresql.campaign_reports.campaign_revision": strconv.FormatInt(job.CampaignRevision, 10),
	}
	return job, nil
}

func writeCampaignReportResponse(w http.ResponseWriter, ctx context.Context, job campaignReportJob) {
	httpx.JSONContractSuccess(w, ctx, job, httpx.ContractMeta{
		ContractVersion: campaignAggregateContractVersion, SnapshotID: job.SnapshotID,
		AsOf: job.UpdatedAt.UTC().Format(time.RFC3339Nano), Partial: false,
		MissingSections: []string{}, SourceWatermarks: job.SourceWatermarks,
	})
}

func (h *SystemHandler) campaignReportObjectStore() (AlertReportObjectStore, error) {
	if h.campaignReportObjects != nil {
		return h.campaignReportObjects, nil
	}
	return (&Handler{}).alertReportObjectStore()
}

func campaignReportBucket() string {
	if value := strings.TrimSpace(os.Getenv("CAMPAIGN_REPORT_BUCKET")); value != "" {
		return value
	}
	return alertReportBucket()
}

func buildCampaignReportArtifact(format string, model CampaignReportModel, canonical []byte) ([]byte, string, string, error) {
	switch format {
	case "json":
		return append(canonical, '\n'), "application/json", "json", nil
	case "pdf":
		return buildCampaignReportPDF(model), "application/pdf", "pdf", nil
	case "word":
		content, err := buildCampaignReportDOCX(model)
		return content, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx", err
	default:
		return nil, "", "", fmt.Errorf("unsupported campaign report format %q", format)
	}
}

func campaignReportLines(model CampaignReportModel) []string {
	return []string{
		"Traffic Analysis Campaign Report", "Campaign ID: " + model.CampaignID,
		"Snapshot ID: " + model.SnapshotID, fmt.Sprintf("Campaign Revision: %d", model.CampaignRevision),
		"Status: " + model.Status, "Assignee: " + model.Assignee,
		fmt.Sprintf("Score: %.4f", model.Score), fmt.Sprintf("Members: %d", model.MemberCount),
		fmt.Sprintf("Evidence: %d", model.EvidenceCount), "Summary: " + model.Summary,
	}
}

func buildCampaignReportPDF(model CampaignReportModel) []byte {
	var stream strings.Builder
	stream.WriteString("BT /F1 11 Tf 48 780 Td ")
	for index, line := range campaignReportLines(model) {
		if index > 0 {
			stream.WriteString("0 -18 Td ")
		}
		stream.WriteString("(" + escapePDFText(line) + ") Tj ")
	}
	stream.WriteString("ET")
	body := stream.String()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(body), body),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for index := 1; index <= len(objects); index++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(&output, "trailer << /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes()
}

func buildCampaignReportDOCX(model CampaignReportModel) ([]byte, error) {
	var document strings.Builder
	document.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, line := range campaignReportLines(model) {
		document.WriteString(`<w:p><w:r><w:t>` + html.EscapeString(line) + `</w:t></w:r></w:p>`)
	}
	document.WriteString(`<w:sectPr/></w:body></w:document>`)
	entries := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   document.String(),
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"} {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(time.Unix(0, 0).UTC())
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(entry, entries[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func sha256Sum(content []byte) []byte {
	digest := sha256.Sum256(content)
	return digest[:]
}

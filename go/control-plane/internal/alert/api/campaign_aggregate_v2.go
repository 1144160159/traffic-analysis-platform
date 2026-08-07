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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
)

const campaignAggregateContractVersion = 4

type campaignAggregateState struct {
	Status      string
	Assignee    string
	Revision    int64
	MemberCount int
	LastEventID string
}

type campaignCommandError struct {
	status  int
	code    string
	message string
}

func (e *campaignCommandError) Error() string { return e.message }

type campaignRowScanner interface {
	Scan(...interface{}) error
}

func normalizeCampaignCommandCompatibility(
	w http.ResponseWriter,
	r *http.Request,
	request *campaignActionRequest,
	currentRevision int64,
) {
	compatible := false
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		r.Header.Set("Idempotency-Key", "compat-campaign-"+uuid.NewString())
		compatible = true
	}
	if request.ExpectedRevision == nil {
		revision := currentRevision
		if revision < 0 {
			revision = 0
		}
		request.ExpectedRevision = &revision
		compatible = true
	}
	if strings.TrimSpace(request.Reason) == "" {
		request.Reason = "compatibility campaign action"
		compatible = true
	}
	request.CompatibilityMode = compatible
	if compatible {
		w.Header().Set("X-Compatibility-Mode", "true")
		w.Header().Set("Idempotency-Key", r.Header.Get("Idempotency-Key"))
	}
}

func (h *SystemHandler) submitCampaignAggregateV2Action(
	w http.ResponseWriter,
	r *http.Request,
	request campaignActionRequest,
	spec campaignActionSpec,
	campaignID string,
	campaign campaignDTO,
) {
	ctx := r.Context()
	if h.pgDB == nil || h.campaignAuditWriter == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "campaign aggregate persistence is unavailable")
		return
	}
	if err := verifyCampaignAggregateV2Schema(ctx, h.pgDB); err != nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "SCHEMA_UNAVAILABLE", "campaign aggregate v2 schema is unavailable")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain 16 to 200 characters")
		return
	}
	if request.ExpectedRevision == nil || *request.ExpectedRevision < 0 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "EXPECTED_REVISION_REQUIRED", "non-negative expected_revision is required")
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if len([]rune(request.Reason)) < 8 || len([]rune(request.Reason)) > 1000 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "REASON_REQUIRED", "reason must contain 8 to 1000 characters")
		return
	}

	requestSHA, err := campaignCommandRequestSHA(campaignID, request)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "campaign command metadata is not serializable")
		return
	}
	var job campaignActionJob
	if request.ActionID == "campaign-merge" {
		targetCampaignID := campaignMetadataString(request.Metadata, "target_campaign_id")
		if h.lookupCampaign == nil {
			httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "CAMPAIGN_SOURCE_UNAVAILABLE", "campaign authority is unavailable")
			return
		}
		targetCampaign, lookupErr := h.lookupCampaign(ctx, httpx.GetTenantID(ctx), targetCampaignID)
		if lookupErr != nil {
			if errors.Is(lookupErr, sql.ErrNoRows) {
				httpx.JSONError(w, ctx, http.StatusNotFound, "TARGET_CAMPAIGN_NOT_FOUND", "target campaign not found")
			} else {
				httpx.JSONError(w, ctx, http.StatusBadGateway, "CAMPAIGN_SOURCE_UNAVAILABLE", "failed to validate target campaign")
			}
			return
		}
		job, err = h.commitCampaignMergeV2Command(ctx, r, request, spec, campaign, targetCampaign, idempotencyKey, requestSHA)
	} else {
		job, err = h.commitCampaignAggregateV2Command(ctx, r, request, spec, campaignID, campaign, idempotencyKey, requestSHA)
	}
	if err != nil {
		var commandErr *campaignCommandError
		if errors.As(err, &commandErr) {
			httpx.JSONError(w, ctx, commandErr.status, commandErr.code, commandErr.message)
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "CAMPAIGN_COMMAND_FAILED", "failed to atomically accept campaign command")
		return
	}
	writeCampaignAggregateV2Response(w, ctx, job)
}

func (h *SystemHandler) commitCampaignAggregateV2Command(
	ctx context.Context,
	requestHTTP *http.Request,
	request campaignActionRequest,
	spec campaignActionSpec,
	campaignID string,
	campaign campaignDTO,
	idempotencyKey string,
	requestSHA string,
) (campaignActionJob, error) {
	tx, err := h.pgDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return campaignActionJob{}, err
	}
	defer tx.Rollback()

	existing, found, err := loadCampaignV2JobByIdempotency(ctx, tx, httpx.GetTenantID(ctx), idempotencyKey)
	if err != nil {
		return campaignActionJob{}, err
	}
	if found {
		if existing.CampaignID != campaignID || existing.ActionID != request.ActionID ||
			existing.ExpectedRevision != *request.ExpectedRevision || existing.RequestSHA256 != requestSHA {
			return campaignActionJob{}, &campaignCommandError{
				status: http.StatusConflict, code: "IDEMPOTENCY_KEY_CONFLICT",
				message: "Idempotency-Key was already used for a different campaign command",
			}
		}
		existing.IdempotentReuse = true
		if err := h.campaignAuditWriter.recordWithExecutor(ctx, tx, requestHTTP, AlertActionAuditRecord{
			Action: "CAMPAIGN_COMMAND_REUSED", ObjectType: "campaign", ObjectID: campaignID,
			TenantID: existing.TenantID, UserID: httpx.GetUserID(ctx), Reason: request.Reason,
			Result: "idempotent_replay", Detail: map[string]interface{}{
				"action_id": request.ActionID, "job_id": existing.JobID,
				"resource_revision":      existing.ResourceRevision,
				"idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
			},
		}); err != nil {
			return campaignActionJob{}, err
		}
		if err := tx.Commit(); err != nil {
			return campaignActionJob{}, err
		}
		return existing, nil
	}
	if mergedInto, merged, err := loadCampaignMergeTarget(ctx, tx, httpx.GetTenantID(ctx), campaignID); err != nil {
		return campaignActionJob{}, err
	} else if merged {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusConflict, code: "CAMPAIGN_MERGED",
			message: fmt.Sprintf("campaign was merged into %s and no longer accepts commands", mergedInto),
		}
	}

	tenantID := httpx.GetTenantID(ctx)
	userID := httpx.GetUserID(ctx)
	initialStatus := normalizedCampaignState(campaign.Status)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO campaign_workbench_state
		(tenant_id,campaign_id,assignee,status,state_version,member_count,updated_by,updated_at)
		VALUES ($1,$2,'',$3,0,0,$4,now())
		ON CONFLICT (tenant_id,campaign_id) DO NOTHING`,
		tenantID, campaignID, initialStatus, userID,
	); err != nil {
		return campaignActionJob{}, err
	}

	state, err := lockCampaignAggregateV2State(ctx, tx, tenantID, campaignID)
	if err != nil {
		return campaignActionJob{}, err
	}
	if state.Revision != *request.ExpectedRevision {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusConflict, code: "REVISION_CONFLICT",
			message: fmt.Sprintf("expected revision %d but current revision is %d", *request.ExpectedRevision, state.Revision),
		}
	}
	members, err := loadCampaignAggregateMembers(ctx, tx, tenantID, campaignID)
	if err != nil {
		return campaignActionJob{}, err
	}
	state.MemberCount = len(members)
	commandSnapshot := campaign
	commandSnapshot.TenantID = tenantID
	commandSnapshot.CampaignID = campaignID
	commandSnapshot.Status = state.Status
	commandSnapshot.Assignee = state.Assignee
	commandSnapshot.StateVersion = state.Revision
	commandSnapshot.MemberCount = state.MemberCount
	commandSnapshot.LastEventID = state.LastEventID
	if err := stampCampaignLifecycleSnapshot(&commandSnapshot); err != nil {
		return campaignActionJob{}, err
	}
	requestedSnapshot := campaignMetadataString(request.Metadata, "snapshot_id")
	if requestedSnapshot != "" && !campaignSnapshotMatches(requestedSnapshot, commandSnapshot) {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusConflict, code: "CAMPAIGN_SNAPSHOT_CONFLICT",
			message: "campaign changed after the requested snapshot",
		}
	}

	eventType, terminalStatus, err := applyCampaignAggregateV2Command(&state, request, campaign, members)
	if err != nil {
		return campaignActionJob{}, err
	}
	state.Revision++
	eventID := uuid.NewString()
	jobID := "campaign-" + uuid.NewString()
	result := map[string]interface{}{
		"action_id": request.ActionID, "audit_event": spec.AuditEvent,
		"event_id": eventID, "campaign_id": campaignID,
		"campaign_status": state.Status, "assignee": state.Assignee,
		"member_count": state.MemberCount, "resource_revision": state.Revision,
		"source_snapshot_id":     commandSnapshot.SnapshotID,
		"source_snapshot_sha256": commandSnapshot.SnapshotSHA256,
		"accepted":               true, "final_effect": terminalStatus == "succeeded",
		"compatibility_mode": request.CompatibilityMode,
	}

	var reportSnapshot *CampaignReportModel
	var reportID, reportSnapshotID, reportSnapshotSHA string
	if request.ActionID == "campaign-report-generate" {
		if len(members) == 0 && len(campaign.Alerts) > 0 {
			return campaignActionJob{}, &campaignCommandError{
				status: http.StatusConflict, code: "CAMPAIGN_MEMBERSHIP_BACKFILL_REQUIRED",
				message: "authoritative PostgreSQL campaign membership has not been backfilled",
			}
		}
		reportID = "campaign-report-" + uuid.NewString()
		reportSnapshotID = uuid.NewString()
		reportSnapshot = buildCampaignReportSnapshot(tenantID, reportSnapshotID, commandSnapshot.SnapshotID, campaign, state, members, request.Metadata)
		snapshotJSON, snapshotSHA, marshalErr := canonicalCampaignSnapshot(reportSnapshot)
		if marshalErr != nil {
			return campaignActionJob{}, marshalErr
		}
		format := normalizedCampaignReportFormat(campaignMetadataString(request.Metadata, "format"))
		sections, _ := json.Marshal(request.Metadata["sections"])
		if string(sections) == "null" {
			sections = []byte(`[]`)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO campaign_reports
			(report_id,job_id,tenant_id,campaign_id,format,status,sections,evidence_count,created_by,
			 campaign_revision,snapshot_id,snapshot,snapshot_sha256,idempotency_key)
			VALUES ($1,$2,$3,$4,$5,'accepted',$6::jsonb,$7,$8,$9,$10::uuid,$11::jsonb,$12,$13)`,
			reportID, jobID, tenantID, campaignID, format, string(sections),
			campaignMetadataInt(request.Metadata, "evidence_count"), userID, state.Revision,
			reportSnapshotID, string(snapshotJSON), snapshotSHA, idempotencyKey,
		); err != nil {
			return campaignActionJob{}, err
		}
		result["report_id"] = reportID
		result["report_status"] = "accepted"
		reportSnapshotSHA = snapshotSHA
		result["snapshot_id"] = reportSnapshotID
		result["snapshot_sha256"] = snapshotSHA
		result["object_manifest_status"] = "awaiting_executor"
	}
	if request.ActionID == "campaign-soar-response" {
		terminalStatus = "pending_approval"
		result["approval_status"] = "pending"
		result["executor_status"] = "not_dispatched"
		result["workflow_revision"] = int64(1)
		result["playbook_id"] = campaignMetadataString(request.Metadata, "playbook_id")
		result["final_effect"] = false
	}

	payload := map[string]interface{}{
		"event_id": eventID, "tenant_id": tenantID, "schema_version": 2,
		"aggregate_type": "campaign", "aggregate_id": campaignID,
		"aggregate_version": state.Revision, "partition_key": tenantID + ":" + campaignID,
		"event_type": eventType, "status": state.Status, "assignee": state.Assignee,
		"member_count": state.MemberCount, "job_id": jobID, "action_id": request.ActionID,
		"trace_id": httpx.GetTraceID(ctx), "reason": request.Reason,
		"compatibility_mode": request.CompatibilityMode,
	}
	if reportSnapshot != nil {
		payload["report_id"] = reportID
		payload["snapshot_id"] = reportSnapshotID
		payload["snapshot_sha256"] = reportSnapshotSHA
		payload["report_snapshot"] = reportSnapshot
	}
	if request.ActionID == "campaign-soar-response" {
		payload["playbook_id"] = campaignMetadataString(request.Metadata, "playbook_id")
		payload["source_snapshot_id"] = commandSnapshot.SnapshotID
		payload["workflow_revision"] = int64(1)
		payload["approval_status"] = "pending"
		payload["executor_status"] = "not_dispatched"
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return campaignActionJob{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE campaign_workbench_state
		SET assignee=$3,status=$4,state_version=$5,member_count=$6,last_event_id=$7::uuid,
		    updated_by=$8,updated_at=now()
		WHERE tenant_id=$1 AND campaign_id=$2`,
		tenantID, campaignID, state.Assignee, state.Status, state.Revision,
		state.MemberCount, eventID, userID,
	); err != nil {
		return campaignActionJob{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO campaign_aggregate_history
		(event_id,tenant_id,campaign_id,aggregate_revision,event_type,status,assignee,member_count,payload,reason,created_by)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11)`,
		eventID, tenantID, campaignID, state.Revision, eventType, state.Status,
		state.Assignee, state.MemberCount, string(payloadJSON), request.Reason, userID,
	); err != nil {
		return campaignActionJob{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO campaign_aggregate_outbox
		(event_id,tenant_id,aggregate_id,aggregate_revision,event_type,partition_key,payload)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::jsonb)`,
		eventID, tenantID, campaignID, state.Revision, eventType,
		tenantID+":"+campaignID, string(payloadJSON),
	); err != nil {
		return campaignActionJob{}, err
	}

	metadataJSON, err := json.Marshal(request.Metadata)
	if err != nil {
		return campaignActionJob{}, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return campaignActionJob{}, err
	}
	completedAt := interface{}(nil)
	if terminalStatus == "succeeded" {
		completedAt = time.Now().UTC()
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO campaign_action_jobs
		(job_id,tenant_id,campaign_id,action_id,target,metadata,simulation,dry_run,status,result,
		 created_by,completed_at,idempotency_key,request_sha256,expected_revision,resource_revision,reason)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,false,false,$7,$8::jsonb,$9,$10,$11,$12,$13,$14,$15)`,
		jobID, tenantID, campaignID, request.ActionID, request.Target, string(metadataJSON),
		terminalStatus, string(resultJSON), userID, completedAt, idempotencyKey, requestSHA,
		*request.ExpectedRevision, state.Revision, request.Reason,
	); err != nil {
		return campaignActionJob{}, err
	}
	if request.ActionID == "campaign-soar-response" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_soar_jobs
			(job_id,tenant_id,campaign_id,playbook_id,target,source_snapshot_id,campaign_revision,
			 status,approval_status,executor_status,revision,request,requested_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'pending_approval','pending','not_dispatched',1,$8::jsonb,$9)`,
			jobID, tenantID, campaignID, campaignMetadataString(request.Metadata, "playbook_id"),
			request.Target, commandSnapshot.SnapshotID, state.Revision, string(metadataJSON), userID,
		); err != nil {
			return campaignActionJob{}, err
		}
	}

	job := campaignActionJob{
		JobID: jobID, TenantID: tenantID, CampaignID: campaignID, ActionID: request.ActionID,
		Target: request.Target, Metadata: request.Metadata, Simulation: false, DryRun: false,
		Status: terminalStatus, Result: result, CreatedBy: userID,
		IdempotencyKey: idempotencyKey, RequestSHA256: requestSHA,
		ExpectedRevision: *request.ExpectedRevision, ResourceRevision: state.Revision, Reason: request.Reason,
	}
	if terminalStatus == "succeeded" {
		job.CompletedAt = completedAt.(time.Time)
	}
	if err := h.campaignAuditWriter.recordWithExecutor(ctx, tx, requestHTTP, AlertActionAuditRecord{
		Action: spec.AuditEvent, ObjectType: "campaign", ObjectID: campaignID,
		TenantID: tenantID, UserID: userID, Reason: request.Reason, Result: terminalStatus,
		Detail: map[string]interface{}{
			"action_id": request.ActionID, "job_id": jobID, "event_id": eventID,
			"expected_revision": *request.ExpectedRevision, "resource_revision": state.Revision,
			"member_count": state.MemberCount, "idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
			"compatibility_mode": request.CompatibilityMode,
		},
	}); err != nil {
		return campaignActionJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return campaignActionJob{}, err
	}
	return job, nil
}

func applyCampaignAggregateV2Command(
	state *campaignAggregateState,
	request campaignActionRequest,
	campaign campaignDTO,
	members []string,
) (string, string, error) {
	switch request.ActionID {
	case "campaign-assign-owner":
		assignee := campaignMetadataString(request.Metadata, "assignee")
		if assignee == "" || len([]rune(assignee)) > 128 {
			return "", "", &campaignCommandError{status: http.StatusBadRequest, code: "INVALID_ASSIGNEE", message: "assignee is required and must not exceed 128 characters"}
		}
		state.Assignee = assignee
		return "traffic.campaign.v2.OwnerAssigned", "succeeded", nil
	case "campaign-status-change":
		next := strings.ToLower(strings.TrimSpace(campaignMetadataString(request.Metadata, "next_status")))
		if !validCampaignWorkbenchStatus(next) {
			return "", "", &campaignCommandError{
				status: http.StatusBadRequest, code: "INVALID_CAMPAIGN_STATE",
				message: "next_status must be active, investigating, contained, or closed",
			}
		}
		if !validCampaignStateTransition(state.Status, next) {
			return "", "", &campaignCommandError{
				status: http.StatusConflict, code: "INVALID_STATE_TRANSITION",
				message: fmt.Sprintf("campaign state cannot transition from %s to %s", state.Status, next),
			}
		}
		state.Status = next
		return "traffic.campaign.v2.StatusChanged", "succeeded", nil
	case "campaign-report-generate":
		format := normalizedCampaignReportFormat(campaignMetadataString(request.Metadata, "format"))
		if format == "" {
			return "", "", &campaignCommandError{status: http.StatusBadRequest, code: "INVALID_REPORT_FORMAT", message: "format must be pdf, word, or json"}
		}
		if len(members) == 0 && len(campaign.Alerts) > 0 {
			return "", "", &campaignCommandError{status: http.StatusConflict, code: "CAMPAIGN_MEMBERSHIP_BACKFILL_REQUIRED", message: "authoritative PostgreSQL campaign membership has not been backfilled"}
		}
		return "traffic.campaign.v2.ReportRequested", "accepted", nil
	case "campaign-soar-response":
		if campaignMetadataString(request.Metadata, "playbook_id") == "" {
			return "", "", &campaignCommandError{status: http.StatusBadRequest, code: "PLAYBOOK_ID_REQUIRED", message: "playbook_id is required for a SOAR response"}
		}
		return "traffic.campaign.v2.SoarRequested", "accepted", nil
	default:
		return "", "", &campaignCommandError{status: http.StatusBadRequest, code: "UNSUPPORTED_CAMPAIGN_COMMAND", message: "campaign command is not supported by aggregate v2"}
	}
}

func lockCampaignAggregateV2State(ctx context.Context, tx *sql.Tx, tenantID, campaignID string) (campaignAggregateState, error) {
	var state campaignAggregateState
	err := tx.QueryRowContext(ctx, `
		SELECT status,assignee,state_version,member_count,COALESCE(last_event_id::text,'')
		FROM campaign_workbench_state
		WHERE tenant_id=$1 AND campaign_id=$2 FOR UPDATE`, tenantID, campaignID,
	).Scan(&state.Status, &state.Assignee, &state.Revision, &state.MemberCount, &state.LastEventID)
	return state, err
}

func loadCampaignAggregateMembers(ctx context.Context, tx *sql.Tx, tenantID, campaignID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT alert_id FROM campaign_alert_links
		WHERE tenant_id=$1 AND campaign_id=$2 AND status='linked'
		ORDER BY alert_id`, tenantID, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := make([]string, 0)
	for rows.Next() {
		var alertID string
		if err := rows.Scan(&alertID); err != nil {
			return nil, err
		}
		members = append(members, alertID)
	}
	return members, rows.Err()
}

func loadCampaignV2JobByIdempotency(ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey string) (campaignActionJob, bool, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT job_id,tenant_id,campaign_id,action_id,target,metadata,simulation,dry_run,status,result,
		       error_message,created_by,created_at,completed_at,idempotency_key,request_sha256,
		       expected_revision,resource_revision,reason
		FROM campaign_action_jobs
		WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`, tenantID, idempotencyKey)
	job, err := scanCampaignV2Job(row)
	if errors.Is(err, sql.ErrNoRows) {
		return campaignActionJob{}, false, nil
	}
	return job, err == nil, err
}

func scanCampaignV2Job(row campaignRowScanner) (campaignActionJob, error) {
	var job campaignActionJob
	var metadataJSON, resultJSON []byte
	var completedAt sql.NullTime
	err := row.Scan(
		&job.JobID, &job.TenantID, &job.CampaignID, &job.ActionID, &job.Target,
		&metadataJSON, &job.Simulation, &job.DryRun, &job.Status, &resultJSON,
		&job.ErrorMessage, &job.CreatedBy, &job.CreatedAt, &completedAt,
		&job.IdempotencyKey, &job.RequestSHA256, &job.ExpectedRevision, &job.ResourceRevision, &job.Reason,
	)
	if err != nil {
		return campaignActionJob{}, err
	}
	if err := json.Unmarshal(metadataJSON, &job.Metadata); err != nil {
		return campaignActionJob{}, err
	}
	if err := json.Unmarshal(resultJSON, &job.Result); err != nil {
		return campaignActionJob{}, err
	}
	if completedAt.Valid {
		job.CompletedAt = completedAt.Time
	}
	return job, nil
}

func campaignCommandRequestSHA(campaignID string, request campaignActionRequest) (string, error) {
	payload := map[string]interface{}{
		"campaign_id": campaignID, "action_id": request.ActionID, "target": request.Target,
		"metadata": request.Metadata, "simulation": request.Simulation, "dry_run": request.DryRun,
		"expected_revision": request.ExpectedRevision, "target_expected_revision": request.TargetExpectedRevision,
		"reason":             strings.TrimSpace(request.Reason),
		"compatibility_mode": request.CompatibilityMode,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func buildCampaignReportSnapshot(tenantID, snapshotID, sourceSnapshotID string, campaign campaignDTO, state campaignAggregateState, members []string, metadata map[string]interface{}) *CampaignReportModel {
	memberCopy := append([]string(nil), members...)
	sort.Strings(memberCopy)
	sections := campaignMetadataStrings(metadata, "sections")
	return &CampaignReportModel{
		SchemaVersion: 2, ContractVersion: campaignAggregateContractVersion,
		SnapshotID: snapshotID, SourceSnapshotID: sourceSnapshotID,
		TenantID: tenantID, CampaignID: campaign.CampaignID,
		CampaignRevision: state.Revision, Status: state.Status, Assignee: state.Assignee,
		Summary: campaign.Summary, Score: campaign.Score, CampaignType: campaign.CampaignType,
		Entities:     append([]string(nil), campaign.Entities...),
		AttackPhases: append([]string(nil), campaign.AttackPhases...),
		RuleIDs:      append([]string(nil), campaign.RuleIDs...), ModelIDs: append([]string(nil), campaign.ModelIDs...),
		MemberAlertIDs: memberCopy, MemberCount: len(memberCopy),
		TimeWindow: CampaignReportTimeWindow{Start: campaign.TsStart, End: campaign.TsEnd},
		Sections:   sections, EvidenceCount: campaignMetadataInt(metadata, "evidence_count"),
		MembershipSource: "postgresql.campaign_alert_links",
		SourceWatermarks: map[string]string{
			"campaign.lifecycle.snapshot_id":               sourceSnapshotID,
			"postgresql.campaign_workbench_state.revision": strconv.FormatInt(state.Revision, 10),
			"postgresql.campaign_alert_links.member_count": strconv.Itoa(len(memberCopy)),
			"clickhouse.campaigns.event_id":                campaign.EventID,
			"clickhouse.campaigns.ingest_ts":               strconv.FormatInt(campaign.IngestTs, 10),
		},
	}
}

func canonicalCampaignSnapshot(snapshot *CampaignReportModel) ([]byte, string, error) {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(encoded)
	return encoded, hex.EncodeToString(digest[:]), nil
}

func campaignMetadataStrings(metadata map[string]interface{}, key string) []string {
	values, ok := metadata[key].([]interface{})
	if !ok {
		if typed, typedOK := metadata[key].([]string); typedOK {
			return append([]string(nil), typed...)
		}
		return []string{}
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	return result
}

func normalizedCampaignState(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if validCampaignWorkbenchStatus(value) {
		return value
	}
	return "active"
}

func validCampaignStateTransition(current, next string) bool {
	if current == next || !validCampaignWorkbenchStatus(next) {
		return false
	}
	allowed := map[string]map[string]bool{
		"active":        {"investigating": true, "closed": true},
		"investigating": {"contained": true, "closed": true},
		"contained":     {"investigating": true, "closed": true},
		"closed":        {"investigating": true},
	}
	return allowed[current][next]
}

func normalizedCampaignReportFormat(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "docx" {
		value = "word"
	}
	switch value {
	case "pdf", "word", "json":
		return value
	default:
		return ""
	}
}

func writeCampaignAggregateV2Response(w http.ResponseWriter, ctx context.Context, job campaignActionJob) {
	if compatible, _ := job.Result["compatibility_mode"].(bool); compatible {
		w.Header().Set("X-Compatibility-Mode", "true")
		w.Header().Set("Idempotency-Key", job.IdempotencyKey)
	}
	httpx.JSONContractAccepted(w, ctx, map[string]interface{}{
		"action_id": job.ActionID, "audit_event": campaignActionSpecs[job.ActionID].AuditEvent,
		"status": job.Status, "job_id": job.JobID, "job_status": job.Status,
		"simulation": false, "dry_run": false, "result": job.Result,
		"expected_revision": job.ExpectedRevision, "resource_revision": job.ResourceRevision,
		"idempotent_reuse": job.IdempotentReuse,
	}, httpx.ContractMeta{
		ContractVersion: campaignAggregateContractVersion,
		SnapshotID:      fmt.Sprintf("campaign-command:%s:%d", job.JobID, job.ResourceRevision),
		SourceWatermarks: map[string]string{
			"postgresql.campaign_workbench_state.revision": strconv.FormatInt(job.ResourceRevision, 10),
		},
	})
}

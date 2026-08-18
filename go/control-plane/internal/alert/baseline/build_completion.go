package baseline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type lockedBuildJob struct {
	JobID              string
	BaselineID         string
	BaselineKind       string
	DefinitionRevision int64
	TargetVersion      int64
	CandidateSHA256    string
	WindowStart        sql.NullTime
	WindowEnd          sql.NullTime
	Status             string
	RequestedBy        string
	Reason             string
	TraceID            string
}

func (repository *Repository) CompleteDynamicBuildTx(
	ctx context.Context,
	tx *sql.Tx,
	result DynamicSampleResult,
) (VersionReceipt, error) {
	if tx == nil {
		return VersionReceipt{}, fmt.Errorf("%w: transaction is required", ErrInvalidRequest)
	}
	if err := result.Validate(); err != nil {
		return VersionReceipt{}, err
	}
	job, err := lockBuildJobTx(ctx, tx, result.TenantID, result.JobID)
	if err != nil {
		return VersionReceipt{}, err
	}
	if job.BaselineKind != "dynamic" || !job.WindowStart.Valid || !job.WindowEnd.Valid || job.CandidateSHA256 != result.CandidateSHA256 {
		return VersionReceipt{}, fmt.Errorf("%w: dynamic build result does not match its job", ErrIdentityConflict)
	}
	if job.Status == "succeeded" || job.Status == "failed" {
		return lookupCompletedVersionTx(ctx, tx, result.TenantID, job)
	}
	if job.Status != "queued" && job.Status != "running" {
		return VersionReceipt{}, fmt.Errorf("%w: build job is %s", ErrStateConflict, job.Status)
	}
	if result.MaxEventTime != nil && result.MaxEventTime.After(job.WindowEnd.Time) {
		return VersionReceipt{}, fmt.Errorf("%w: sample contains future data after the closed window", ErrStateConflict)
	}
	var state, algorithm, thresholdJSON, policyJSON string
	var revision int64
	err = tx.QueryRowContext(ctx, `SELECT lifecycle_state,revision,algorithm_version,threshold_spec::text,sample_policy::text
		FROM behavior_baseline_definitions_v1 WHERE tenant_id=$1 AND baseline_id=$2 FOR UPDATE`,
		result.TenantID, job.BaselineID).Scan(&state, &revision, &algorithm, &thresholdJSON, &policyJSON)
	if err != nil {
		return VersionReceipt{}, fmt.Errorf("lock dynamic baseline definition: %w", err)
	}
	if revision != job.DefinitionRevision {
		return VersionReceipt{}, fmt.Errorf("%w: build definition revision %d is stale, current %d", ErrRevisionConflict, job.DefinitionRevision, revision)
	}
	var policy struct {
		MinimumEligibleRows int64 `json:"minimum_eligible_rows"`
	}
	if json.Unmarshal([]byte(policyJSON), &policy) != nil || policy.MinimumEligibleRows <= 0 {
		return VersionReceipt{}, fmt.Errorf("%w: dynamic sample policy lacks minimum_eligible_rows", ErrIdentityConflict)
	}
	quality := result.QualityStatus
	partialReasons := append([]string(nil), result.PartialReasons...)
	if result.EligibleRowCount < policy.MinimumEligibleRows {
		quality = "partial"
		partialReasons = uniqueSorted(append(partialReasons, "insufficient_eligible_rows"))
	}
	if quality != "complete" && len(partialReasons) == 0 {
		partialReasons = []string{"source_quality_not_complete"}
	}
	sampleCanonical := map[string]interface{}{
		"algorithm": "behavior-baseline-sample-v1", "tenant_id": result.TenantID, "baseline_id": job.BaselineID,
		"window_start": job.WindowStart.Time.UTC().Format(time.RFC3339Nano),
		"window_end":   job.WindowEnd.Time.UTC().Format(time.RFC3339Nano), "max_event_time": result.MaxEventTime,
		"row_count": result.RowCount, "eligible_row_count": result.EligibleRowCount,
		"minimum_eligible_rows": policy.MinimumEligibleRows, "quality_status": quality,
		"partial_reasons": partialReasons, "source_watermark": result.SourceWatermark,
		"source_query_sha256": result.SourceQuerySHA256, "sample_object_uri": result.SampleObjectURI,
		"sample_object_sha256": result.SampleObjectSHA256, "provenance": result.Provenance,
	}
	sampleSHA, err := canonicalSHA256(sampleCanonical)
	if err != nil {
		return VersionReceipt{}, err
	}
	sampleID := uuid.NewString()
	watermarkJSON, _ := json.Marshal(result.SourceWatermark)
	provenanceJSON, _ := json.Marshal(result.Provenance)
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_sample_snapshots_v1 (
		sample_snapshot_id,tenant_id,baseline_id,window_start,window_end,as_of,max_event_time,row_count,
		eligible_row_count,minimum_eligible_rows,quality_status,partial_reasons,source_watermark,
		source_query_sha256,canonical_sha256,sample_object_uri,sample_object_sha256,provenance
	) VALUES ($1,$2,$3,$4,$5,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$14,$15,$16,$17::jsonb)`, sampleID,
		result.TenantID, job.BaselineID, job.WindowStart.Time, job.WindowEnd.Time, result.MaxEventTime, result.RowCount,
		result.EligibleRowCount, policy.MinimumEligibleRows, quality, pq.Array(partialReasons), string(watermarkJSON),
		result.SourceQuerySHA256, sampleSHA, result.SampleObjectURI, result.SampleObjectSHA256, string(provenanceJSON))
	if err != nil {
		return VersionReceipt{}, fmt.Errorf("insert behavior baseline sample snapshot: %w", err)
	}
	versionState := "frozen"
	if quality != "complete" {
		versionState = "failed"
	}
	var thresholdSpec map[string]interface{}
	if err := json.Unmarshal([]byte(thresholdJSON), &thresholdSpec); err != nil {
		return VersionReceipt{}, fmt.Errorf("decode behavior baseline threshold spec: %w", err)
	}
	versionCanonical := map[string]interface{}{
		"algorithm": "behavior-baseline-version-v1", "tenant_id": result.TenantID, "baseline_id": job.BaselineID,
		"baseline_kind": "dynamic", "baseline_version": job.TargetVersion, "definition_revision": revision,
		"sample_snapshot_id": sampleID, "sample_sha256": sampleSHA, "window_start": job.WindowStart.Time,
		"window_end": job.WindowEnd.Time, "algorithm_version": algorithm, "threshold_spec": thresholdSpec,
		"statistics": result.Statistics, "quality_status": quality, "candidate_sha256": result.CandidateSHA256,
	}
	versionSHA, err := canonicalSHA256(versionCanonical)
	if err != nil {
		return VersionReceipt{}, err
	}
	versionID := uuid.NewString()
	statisticsJSON, _ := json.Marshal(result.Statistics)
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_versions_v1 (
		version_id,tenant_id,baseline_id,baseline_version,baseline_kind,definition_revision,lifecycle_state,
		sample_snapshot_id,window_start,window_end,algorithm_version,threshold_spec,statistics,quality_status,
		snapshot_sha256,candidate_sha256,created_by,frozen_at
	) VALUES ($1,$2,$3,$4,'dynamic',$5,$6,$7,$8,$9,$10,$11::jsonb,$12::jsonb,$13,$14,$15,$16,
		CASE WHEN $6='frozen' THEN now() ELSE NULL END)`, versionID, result.TenantID, job.BaselineID, job.TargetVersion,
		revision, versionState, sampleID, job.WindowStart.Time, job.WindowEnd.Time, algorithm, thresholdJSON,
		string(statisticsJSON), quality, versionSHA, result.CandidateSHA256, result.CompletedBy)
	if err != nil {
		return VersionReceipt{}, fmt.Errorf("insert dynamic behavior baseline version: %w", err)
	}
	eventType, jobStatus, errorCode := "baseline.version.frozen.v1", "succeeded", ""
	if versionState == "failed" {
		eventType, jobStatus, errorCode = "baseline.version.failed.v1", "failed", "SAMPLE_NOT_ELIGIBLE"
	}
	eventID := uuid.NewString()
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": eventType, "schema_version": 1, "tenant_id": result.TenantID,
		"baseline_id": job.BaselineID, "baseline_kind": "dynamic", "baseline_version": job.TargetVersion,
		"version_id": versionID, "sample_snapshot_id": sampleID, "snapshot_sha256": versionSHA,
		"quality_status": quality, "candidate_sha256": result.CandidateSHA256, "trace_id": job.TraceID,
	}
	if errorCode != "" {
		payload["error_code"] = errorCode
	}
	if err := appendOutboxTx(ctx, tx, result.TenantID, job.BaselineID, "baseline_version", versionID,
		job.TargetVersion, eventType, eventID, job.TraceID, payload); err != nil {
		return VersionReceipt{}, err
	}
	jobUpdate, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_build_jobs_v1 SET status=$1,
		result_sample_snapshot_id=$2,result_version_id=$3,error_code=$4,error_detail=$5,started_at=COALESCE(started_at,now()),completed_at=now()
		WHERE tenant_id=$6 AND job_id=$7 AND status IN ('queued','running')`, jobStatus, sampleID, versionID, errorCode,
		firstReason(partialReasons), result.TenantID, result.JobID)
	if err != nil {
		return VersionReceipt{}, fmt.Errorf("complete dynamic behavior baseline build: %w", err)
	}
	if affected, _ := jobUpdate.RowsAffected(); affected != 1 {
		return VersionReceipt{}, fmt.Errorf("%w: dynamic build completion lost", ErrStateConflict)
	}
	toState := state
	if state != "active" {
		toState = versionState
		if _, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_definitions_v1 SET lifecycle_state=$1,
			updated_by=$2,updated_at=now() WHERE tenant_id=$3 AND baseline_id=$4 AND revision=$5`, toState,
			result.CompletedBy, result.TenantID, job.BaselineID, revision); err != nil {
			return VersionReceipt{}, fmt.Errorf("update dynamic behavior baseline definition state: %w", err)
		}
	}
	if err := appendHistoryTx(ctx, tx, result.TenantID, job.BaselineID, revision, &job.TargetVersion, state, toState,
		eventType, job.Reason, result.CompletedBy, job.TraceID, map[string]interface{}{
			"job_id": result.JobID, "sample_snapshot_id": sampleID, "snapshot_sha256": versionSHA,
			"quality_status": quality, "partial_reasons": partialReasons,
		}); err != nil {
		return VersionReceipt{}, err
	}
	return VersionReceipt{JobID: result.JobID, BaselineID: job.BaselineID, BaselineVersion: job.TargetVersion,
		VersionID: versionID, SampleSnapshotID: sampleID, LifecycleState: versionState, QualityStatus: quality,
		SnapshotSHA256: versionSHA, EventID: eventID}, nil
}

func lockBuildJobTx(ctx context.Context, tx *sql.Tx, tenantID, jobID string) (lockedBuildJob, error) {
	var job lockedBuildJob
	err := tx.QueryRowContext(ctx, `SELECT baseline_id,baseline_kind,definition_revision,target_version,candidate_sha256,
		requested_window_start,requested_window_end,status,requested_by,reason,trace_id
		FROM behavior_baseline_build_jobs_v1 WHERE tenant_id=$1 AND job_id=$2 FOR UPDATE`, tenantID, jobID).Scan(
		&job.BaselineID, &job.BaselineKind, &job.DefinitionRevision, &job.TargetVersion, &job.CandidateSHA256,
		&job.WindowStart, &job.WindowEnd, &job.Status, &job.RequestedBy, &job.Reason, &job.TraceID)
	if err == sql.ErrNoRows {
		return lockedBuildJob{}, fmt.Errorf("%w: build job not found", ErrStateConflict)
	}
	if err != nil {
		return lockedBuildJob{}, fmt.Errorf("lock behavior baseline build job: %w", err)
	}
	job.JobID = jobID
	return job, nil
}

func lookupCompletedVersionTx(ctx context.Context, tx *sql.Tx, tenantID string, job lockedBuildJob) (VersionReceipt, error) {
	var receipt VersionReceipt
	err := tx.QueryRowContext(ctx, `SELECT j.job_id::text,j.baseline_id,v.baseline_version,v.version_id::text,
		COALESCE(v.sample_snapshot_id::text,''),v.lifecycle_state,v.quality_status,v.snapshot_sha256,
		COALESCE((SELECT event_id::text FROM behavior_baseline_lifecycle_outbox_v1 o
		 WHERE o.tenant_id=j.tenant_id AND o.aggregate_type='baseline_version' AND o.aggregate_id=v.version_id::text
		 ORDER BY o.created_at DESC LIMIT 1),'')
		FROM behavior_baseline_build_jobs_v1 j JOIN behavior_baseline_versions_v1 v
		 ON v.tenant_id=j.tenant_id AND v.baseline_id=j.baseline_id AND v.version_id=j.result_version_id
		WHERE j.tenant_id=$1 AND j.job_id=$2`, tenantID, job.JobID).Scan(&receipt.JobID, &receipt.BaselineID,
		&receipt.BaselineVersion, &receipt.VersionID, &receipt.SampleSnapshotID, &receipt.LifecycleState,
		&receipt.QualityStatus, &receipt.SnapshotSHA256, &receipt.EventID)
	if err != nil {
		return VersionReceipt{}, fmt.Errorf("read completed behavior baseline version: %w", err)
	}
	return receipt, nil
}

func firstReason(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	return reasons[0]
}

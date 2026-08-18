package baseline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

func (repository *Repository) CompleteStaticBuildTx(
	ctx context.Context,
	tx *sql.Tx,
	result StaticVersionResult,
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
	if job.BaselineKind != "static" || job.WindowStart.Valid || job.WindowEnd.Valid || job.CandidateSHA256 != result.CandidateSHA256 {
		return VersionReceipt{}, fmt.Errorf("%w: static build result does not match its job", ErrIdentityConflict)
	}
	if job.Status == "succeeded" || job.Status == "failed" {
		return lookupCompletedVersionTx(ctx, tx, result.TenantID, job)
	}
	if job.Status != "queued" && job.Status != "running" {
		return VersionReceipt{}, fmt.Errorf("%w: build job is %s", ErrStateConflict, job.Status)
	}
	var state, algorithm, thresholdJSON string
	var revision int64
	err = tx.QueryRowContext(ctx, `SELECT lifecycle_state,revision,algorithm_version,threshold_spec::text
		FROM behavior_baseline_definitions_v1 WHERE tenant_id=$1 AND baseline_id=$2 FOR UPDATE`,
		result.TenantID, job.BaselineID).Scan(&state, &revision, &algorithm, &thresholdJSON)
	if err != nil {
		return VersionReceipt{}, fmt.Errorf("lock static baseline definition: %w", err)
	}
	if revision != job.DefinitionRevision {
		return VersionReceipt{}, fmt.Errorf("%w: static build definition is stale", ErrRevisionConflict)
	}
	var thresholdSpec map[string]interface{}
	if err := json.Unmarshal([]byte(thresholdJSON), &thresholdSpec); err != nil {
		return VersionReceipt{}, fmt.Errorf("decode static baseline threshold spec: %w", err)
	}
	versionCanonical := map[string]interface{}{
		"algorithm": "behavior-baseline-version-v1", "tenant_id": result.TenantID, "baseline_id": job.BaselineID,
		"baseline_kind": "static", "baseline_version": job.TargetVersion, "definition_revision": revision,
		"algorithm_version": algorithm, "threshold_spec": thresholdSpec, "statistics": result.Statistics,
		"quality_status": "complete", "candidate_sha256": result.CandidateSHA256, "provenance": result.Provenance,
	}
	versionSHA, err := canonicalSHA256(versionCanonical)
	if err != nil {
		return VersionReceipt{}, err
	}
	versionID := uuid.NewString()
	statisticsJSON, _ := json.Marshal(result.Statistics)
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_versions_v1 (
		version_id,tenant_id,baseline_id,baseline_version,baseline_kind,definition_revision,lifecycle_state,
		algorithm_version,threshold_spec,statistics,quality_status,snapshot_sha256,candidate_sha256,created_by,frozen_at
	) VALUES ($1,$2,$3,$4,'static',$5,'frozen',$6,$7::jsonb,$8::jsonb,'complete',$9,$10,$11,now())`,
		versionID, result.TenantID, job.BaselineID, job.TargetVersion, revision, algorithm, thresholdJSON,
		string(statisticsJSON), versionSHA, result.CandidateSHA256, result.CompletedBy)
	if err != nil {
		return VersionReceipt{}, fmt.Errorf("insert static behavior baseline version: %w", err)
	}
	eventID := uuid.NewString()
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": "baseline.version.frozen.v1", "schema_version": 1,
		"tenant_id": result.TenantID, "baseline_id": job.BaselineID, "baseline_kind": "static",
		"baseline_version": job.TargetVersion, "version_id": versionID, "snapshot_sha256": versionSHA,
		"quality_status": "complete", "candidate_sha256": result.CandidateSHA256, "trace_id": job.TraceID,
	}
	if err := appendOutboxTx(ctx, tx, result.TenantID, job.BaselineID, "baseline_version", versionID,
		job.TargetVersion, "baseline.version.frozen.v1", eventID, job.TraceID, payload); err != nil {
		return VersionReceipt{}, err
	}
	update, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_build_jobs_v1 SET status='succeeded',
		result_version_id=$1,error_code='',error_detail='',started_at=COALESCE(started_at,now()),completed_at=now()
		WHERE tenant_id=$2 AND job_id=$3 AND status IN ('queued','running')`, versionID, result.TenantID, result.JobID)
	if err != nil {
		return VersionReceipt{}, fmt.Errorf("complete static behavior baseline build: %w", err)
	}
	if affected, _ := update.RowsAffected(); affected != 1 {
		return VersionReceipt{}, fmt.Errorf("%w: static build completion lost", ErrStateConflict)
	}
	toState := state
	if state != "active" {
		toState = "frozen"
		if _, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_definitions_v1 SET lifecycle_state='frozen',
			updated_by=$1,updated_at=now() WHERE tenant_id=$2 AND baseline_id=$3 AND revision=$4`,
			result.CompletedBy, result.TenantID, job.BaselineID, revision); err != nil {
			return VersionReceipt{}, fmt.Errorf("freeze static behavior baseline definition: %w", err)
		}
	}
	if err := appendHistoryTx(ctx, tx, result.TenantID, job.BaselineID, revision, &job.TargetVersion, state, toState,
		"baseline.version.frozen.v1", job.Reason, result.CompletedBy, job.TraceID,
		map[string]interface{}{"job_id": result.JobID, "snapshot_sha256": versionSHA}); err != nil {
		return VersionReceipt{}, err
	}
	return VersionReceipt{JobID: result.JobID, BaselineID: job.BaselineID, BaselineVersion: job.TargetVersion,
		VersionID: versionID, LifecycleState: "frozen", QualityStatus: "complete", SnapshotSHA256: versionSHA,
		EventID: eventID}, nil
}

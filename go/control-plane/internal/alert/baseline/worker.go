package baseline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type BuildJob struct {
	JobID              string
	TenantID           string
	BaselineID         string
	BaselineKind       string
	EntityType         string
	EntityID           string
	DefinitionRevision int64
	TargetVersion      int64
	CandidateSHA256    string
	WindowStart        *time.Time
	WindowEnd          *time.Time
	RequestedBy        string
	Reason             string
	TraceID            string
}

type DynamicSampleReader interface {
	ReadDynamicSample(context.Context, BuildJob) (DynamicSampleResult, error)
}

type Worker struct {
	pg              *sql.DB
	repository      *Repository
	reader          DynamicSampleReader
	candidateSHA256 string
	workerID        string
}

func NewWorker(pg *sql.DB, reader DynamicSampleReader, candidateSHA256, workerID string) (*Worker, error) {
	if pg == nil || reader == nil || !sha256Pattern.MatchString(strings.TrimSpace(candidateSHA256)) || strings.TrimSpace(workerID) == "" {
		return nil, fmt.Errorf("%w: baseline worker dependencies or candidate are invalid", ErrInvalidRequest)
	}
	return &Worker{pg: pg, repository: NewRepository(), reader: reader,
		candidateSHA256: strings.TrimSpace(candidateSHA256), workerID: strings.TrimSpace(workerID)}, nil
}

func (worker *Worker) RunOnce(ctx context.Context) (bool, error) {
	job, found, err := worker.claimNext(ctx)
	if err != nil || !found {
		return found, err
	}
	if job.BaselineKind == "static" {
		tx, err := worker.pg.BeginTx(ctx, nil)
		if err != nil {
			return true, fmt.Errorf("begin static baseline completion: %w", err)
		}
		defer tx.Rollback()
		_, err = worker.repository.CompleteStaticBuildTx(ctx, tx, StaticVersionResult{
			TenantID: job.TenantID, JobID: job.JobID, CandidateSHA256: job.CandidateSHA256,
			Statistics:  map[string]interface{}{"semantics": "normative_thresholds_no_learned_sample"},
			Provenance:  map[string]interface{}{"worker_id": worker.workerID, "definition_revision": job.DefinitionRevision},
			CompletedBy: worker.workerID,
		})
		if err != nil {
			return true, err
		}
		return true, tx.Commit()
	}
	sample, err := worker.reader.ReadDynamicSample(ctx, job)
	if err != nil {
		if failErr := worker.failJob(ctx, job, "SAMPLE_READ_FAILED", err.Error()); failErr != nil {
			return true, fmt.Errorf("read dynamic sample: %v; persist failure: %w", err, failErr)
		}
		return true, nil
	}
	sample.TenantID = job.TenantID
	sample.JobID = job.JobID
	sample.CandidateSHA256 = job.CandidateSHA256
	sample.CompletedBy = worker.workerID
	tx, err := worker.pg.BeginTx(ctx, nil)
	if err != nil {
		return true, fmt.Errorf("begin dynamic baseline completion: %w", err)
	}
	defer tx.Rollback()
	if _, err := worker.repository.CompleteDynamicBuildTx(ctx, tx, sample); err != nil {
		return true, err
	}
	if err := tx.Commit(); err != nil {
		return true, fmt.Errorf("commit dynamic baseline completion: %w", err)
	}
	return true, nil
}

func (worker *Worker) claimNext(ctx context.Context) (BuildJob, bool, error) {
	tx, err := worker.pg.BeginTx(ctx, nil)
	if err != nil {
		return BuildJob{}, false, fmt.Errorf("begin behavior baseline claim: %w", err)
	}
	defer tx.Rollback()
	// Stale reaper：把运行超时（worker 崩溃遗留 running）的任务回收为 queued，
	// 避免卡死。无 attempts 列，靠超时窗口约束重试频率。
	if _, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_build_jobs_v1
		SET status='queued', started_at=NULL,
		    error_code='requeued_stale', error_detail='running lease expired; requeued by reaper'
		WHERE status='running' AND started_at IS NOT NULL AND started_at < now() - interval '30 minutes'`); err != nil {
		return BuildJob{}, false, fmt.Errorf("reap stale behavior baseline builds: %w", err)
	}
	var job BuildJob
	var windowStart, windowEnd sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT j.job_id::text,j.tenant_id,j.baseline_id,j.baseline_kind,d.entity_type,d.entity_id,
		j.definition_revision,j.target_version,j.candidate_sha256,j.requested_window_start,j.requested_window_end,
		j.requested_by,j.reason,j.trace_id
		FROM behavior_baseline_build_jobs_v1 j JOIN behavior_baseline_definitions_v1 d
		 ON d.tenant_id=j.tenant_id AND d.baseline_id=j.baseline_id
		WHERE j.status='queued' AND j.candidate_sha256=$1 ORDER BY j.created_at,j.job_id
		FOR UPDATE OF j SKIP LOCKED LIMIT 1`, worker.candidateSHA256).Scan(&job.JobID, &job.TenantID, &job.BaselineID,
		&job.BaselineKind, &job.EntityType, &job.EntityID, &job.DefinitionRevision, &job.TargetVersion,
		&job.CandidateSHA256, &windowStart, &windowEnd, &job.RequestedBy, &job.Reason, &job.TraceID)
	if err == sql.ErrNoRows {
		return BuildJob{}, false, nil
	}
	if err != nil {
		return BuildJob{}, false, fmt.Errorf("claim behavior baseline build: %w", err)
	}
	if windowStart.Valid {
		value := windowStart.Time.UTC()
		job.WindowStart = &value
	}
	if windowEnd.Valid {
		value := windowEnd.Time.UTC()
		job.WindowEnd = &value
	}
	update, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_build_jobs_v1 SET status='running',started_at=now()
		WHERE tenant_id=$1 AND job_id=$2 AND status='queued' AND candidate_sha256=$3`,
		job.TenantID, job.JobID, worker.candidateSHA256)
	if err != nil {
		return BuildJob{}, false, fmt.Errorf("mark behavior baseline build running: %w", err)
	}
	if affected, _ := update.RowsAffected(); affected != 1 {
		return BuildJob{}, false, fmt.Errorf("%w: behavior baseline claim lost", ErrStateConflict)
	}
	if err := tx.Commit(); err != nil {
		return BuildJob{}, false, fmt.Errorf("commit behavior baseline claim: %w", err)
	}
	return job, true, nil
}

func (worker *Worker) failJob(ctx context.Context, job BuildJob, code, detail string) error {
	if len(detail) > 4000 {
		detail = detail[:4000]
	}
	tx, err := worker.pg.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	update, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_build_jobs_v1 SET status='failed',error_code=$1,
		error_detail=$2,completed_at=now() WHERE tenant_id=$3 AND job_id=$4 AND status='running'
		AND candidate_sha256=$5`, code, detail, job.TenantID, job.JobID, worker.candidateSHA256)
	if err != nil {
		return fmt.Errorf("fail behavior baseline job: %w", err)
	}
	if affected, _ := update.RowsAffected(); affected != 1 {
		return fmt.Errorf("%w: behavior baseline failure receipt lost", ErrStateConflict)
	}
	eventID := uuid.NewString()
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": "baseline.version.failed.v1", "schema_version": 1,
		"tenant_id": job.TenantID, "baseline_id": job.BaselineID, "job_id": job.JobID,
		"target_version": job.TargetVersion, "candidate_sha256": job.CandidateSHA256,
		"error_code": code, "trace_id": job.TraceID,
	}
	if err := appendOutboxTx(ctx, tx, job.TenantID, job.BaselineID, "baseline_build_job", job.JobID,
		job.TargetVersion, "baseline.version.failed.v1", eventID, job.TraceID, payload); err != nil {
		return err
	}
	if err := appendHistoryTx(ctx, tx, job.TenantID, job.BaselineID, job.DefinitionRevision, &job.TargetVersion,
		"learning", "failed", "baseline.version.failed.v1", job.Reason, worker.workerID, job.TraceID,
		map[string]interface{}{"job_id": job.JobID, "error_code": code}); err != nil {
		return err
	}
	return tx.Commit()
}

func RunWorkerLoop(ctx context.Context, worker *Worker, interval time.Duration) error {
	if worker == nil || interval <= 0 {
		return fmt.Errorf("%w: worker and positive interval are required", ErrInvalidRequest)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		processed, err := worker.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return ctx.Err()
			}
			// 单轮失败不退出整个 worker：记录并退避后继续，
			// 避免 DB 抖动导致基线构建永久停摆。
			fmt.Printf("baseline worker loop error (retrying in 5s): %v\n", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
			continue
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

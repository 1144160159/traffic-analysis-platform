package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

type campaignActionJob struct {
	JobID        string                 `json:"job_id"`
	TenantID     string                 `json:"tenant_id"`
	CampaignID   string                 `json:"campaign_id"`
	ActionID     string                 `json:"action_id"`
	Target       string                 `json:"target"`
	Metadata     map[string]interface{} `json:"metadata"`
	Simulation   bool                   `json:"simulation"`
	DryRun       bool                   `json:"dry_run"`
	Status       string                 `json:"status"`
	Result       map[string]interface{} `json:"result"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	CreatedBy    string                 `json:"created_by"`
	CreatedAt    time.Time              `json:"created_at"`
	CompletedAt  time.Time              `json:"completed_at"`
}

type postgresCampaignActionJobStore struct {
	db *sql.DB
}

type campaignSQLExecutor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

type campaignTransaction interface {
	auditSQLExecutor
	Commit() error
	Rollback() error
}

func newPostgresCampaignActionJobStore(db *sql.DB) *postgresCampaignActionJobStore {
	return &postgresCampaignActionJobStore{db: db}
}

func (s *postgresCampaignActionJobStore) Record(ctx context.Context, job campaignActionJob) error {
	return s.recordWithExecutor(ctx, s.db, job)
}

func (s *postgresCampaignActionJobStore) recordWithExecutor(ctx context.Context, executor campaignSQLExecutor, job campaignActionJob) error {
	metadata, err := json.Marshal(job.Metadata)
	if err != nil {
		return err
	}
	result, err := json.Marshal(job.Result)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `
		INSERT INTO campaign_action_jobs
		(job_id, tenant_id, campaign_id, action_id, target, metadata, simulation, dry_run, status, result, created_by, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10::jsonb, $11, now())`,
		job.JobID, job.TenantID, job.CampaignID, job.ActionID, job.Target, string(metadata),
		job.Simulation, job.DryRun, job.Status, string(result), job.CreatedBy)
	return err
}

func commitCampaignActionTransaction(
	ctx context.Context,
	db *sql.DB,
	jobs *postgresCampaignActionJobStore,
	audit *AlertActionAuditWriter,
	request *http.Request,
	job campaignActionJob,
	record AlertActionAuditRecord,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	return runCampaignActionTransaction(
		tx,
		func(executor campaignTransaction) error { return jobs.recordWithExecutor(ctx, executor, job) },
		func(executor campaignTransaction) error {
			return audit.recordWithExecutor(ctx, executor, request, record)
		},
	)
}

func runCampaignActionTransaction(
	tx campaignTransaction,
	recordJob func(campaignTransaction) error,
	recordAudit func(campaignTransaction) error,
) error {
	defer tx.Rollback()
	if err := recordJob(tx); err != nil {
		return err
	}
	if err := recordAudit(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresCampaignActionJobStore) MarkFailed(ctx context.Context, tenantID, jobID, message string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE campaign_action_jobs SET status='failed', error_message=$3, completed_at=now()
		WHERE tenant_id=$1 AND job_id=$2`, tenantID, jobID, message)
	return err
}

func (s *postgresCampaignActionJobStore) Get(ctx context.Context, tenantID, jobID string) (campaignActionJob, error) {
	var job campaignActionJob
	var metadata, result []byte
	var completedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT job_id, tenant_id, campaign_id, action_id, target, metadata, simulation, dry_run,
		       status, result, error_message, created_by, created_at, completed_at
		FROM campaign_action_jobs WHERE tenant_id=$1 AND job_id=$2`, tenantID, jobID).Scan(
		&job.JobID, &job.TenantID, &job.CampaignID, &job.ActionID, &job.Target, &metadata,
		&job.Simulation, &job.DryRun, &job.Status, &result, &job.ErrorMessage,
		&job.CreatedBy, &job.CreatedAt, &completedAt,
	)
	if err != nil {
		return campaignActionJob{}, err
	}
	if err := json.Unmarshal(metadata, &job.Metadata); err != nil {
		return campaignActionJob{}, err
	}
	if err := json.Unmarshal(result, &job.Result); err != nil {
		return campaignActionJob{}, err
	}
	if completedAt.Valid {
		job.CompletedAt = completedAt.Time
	}
	return job, nil
}

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func (r *AssetRepository) StartLegacyDiscoveryRunAtomic(
	ctx context.Context,
	run *config.DiscoveryRun,
	command config.DiscoveryJobCommand,
) (*config.DiscoveryRun, error) {
	if run == nil {
		return nil, fmt.Errorf("discovery run required")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, err := scanDiscoveryRun(tx.QueryRowContext(ctx, `
		SELECT `+discoveryRunColumns+`
		FROM asset_discovery_runs
		WHERE tenant_id=$1 AND run_id=$2
		FOR UPDATE`, run.TenantID, run.RunID))
	if err != nil {
		return nil, err
	}
	if current.Status != config.DiscoveryStatusQueued || current.Revision != 1 {
		return nil, ErrDiscoveryStateConflict
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE asset_discovery_runs
		SET status='running',revision=2,started_at=$3,updated_at=$3
		WHERE tenant_id=$1 AND run_id=$2 AND status='queued' AND revision=1`,
		run.TenantID, run.RunID, now); err != nil {
		return nil, err
	}
	detailJSON, _ := json.Marshal(map[string]any{
		"action_id": run.ActionID, "request_id": command.RequestID,
		"execution_mode": "legacy_synchronous",
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_discovery_run_history(
			run_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail
		) VALUES ($1,$2,'queued','running',2,$3,$4,$5,$6::jsonb)`,
		run.RunID, run.TenantID, command.Actor, run.Reason, run.TraceID, string(detailJSON)); err != nil {
		return nil, err
	}
	if err := insertDiscoveryStateOutbox(ctx, tx, run, config.DiscoveryStatusRunning, 2, run.TraceID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetDiscoveryRun(ctx, run.TenantID, run.RunID)
}

func (r *AssetRepository) CompleteLegacyDiscoveryRunAtomic(
	ctx context.Context,
	run *config.DiscoveryRun,
	status, errorMessage string,
	assets, links, rejected int,
	command config.DiscoveryJobCommand,
) (*config.DiscoveryRun, error) {
	if run == nil {
		return nil, fmt.Errorf("discovery run required")
	}
	if status != config.DiscoveryStatusSucceeded &&
		status != config.DiscoveryStatusPartial && status != config.DiscoveryStatusFailed {
		return nil, fmt.Errorf("invalid legacy discovery terminal status %q", status)
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, err := scanDiscoveryRun(tx.QueryRowContext(ctx, `
		SELECT `+discoveryRunColumns+`
		FROM asset_discovery_runs
		WHERE tenant_id=$1 AND run_id=$2
		FOR UPDATE`, run.TenantID, run.RunID))
	if err != nil {
		return nil, err
	}
	if current.Status != config.DiscoveryStatusRunning || current.Revision != 2 {
		return nil, ErrDiscoveryStateConflict
	}
	now := time.Now().UTC()
	watermark := fmt.Sprintf("%s:assets=%d:links=%d:rejected=%d", run.RunID, assets, links, rejected)
	if _, err := tx.ExecContext(ctx, `
		UPDATE asset_discovery_runs
		SET status=$3,revision=3,discovered_assets=$4,discovered_links=$5,
		    rejected_records=$6,result_watermark=$7,error_message=NULLIF($8,''),
		    updated_at=$9,completed_at=$9
		WHERE tenant_id=$1 AND run_id=$2 AND status='running' AND revision=2`,
		run.TenantID, run.RunID, status, assets, links, rejected, watermark, errorMessage, now); err != nil {
		return nil, err
	}
	detailJSON, _ := json.Marshal(map[string]any{
		"accepted_assets": assets, "accepted_links": links,
		"rejected_records": rejected, "error": errorMessage,
		"execution_mode": "legacy_synchronous",
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_discovery_run_history(
			run_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail
		) VALUES ($1,$2,'running',$3,3,$4,$5,$6,$7::jsonb)`,
		run.RunID, run.TenantID, status, command.Actor, run.Reason, run.TraceID, string(detailJSON)); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(
			event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent
		) VALUES ($1,$2,NULL,'ASSET_ACTIVE_DISCOVERY_COMPLETED','asset_discovery_run',$3,
			$4::jsonb,NULLIF($5,''),NULLIF($6,''))`,
		uuid.NewString(), run.TenantID, run.RunID, string(detailJSON), command.ClientIP, command.UserAgent); err != nil {
		return nil, err
	}
	if err := insertDiscoveryStateOutbox(ctx, tx, run, status, 3, run.TraceID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetDiscoveryRun(ctx, run.TenantID, run.RunID)
}

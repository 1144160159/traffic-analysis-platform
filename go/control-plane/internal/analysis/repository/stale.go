// Package repository 陈旧 run 关闭(§76.45.4 late activation 的兜底闭包):
// 窗口早已越过且从未启动(ACCEPTED/PREPARING)的 run 不可能再获订阅/准入,
// 权威关闭为 CANCELLED(attempts 一并取消、队列 EXPIRED、预留 RELEASED、审计),
// 与 MISFIRE_FAIL 的 fail-closed 语义一致;不伪造执行回执。
package repository

import (
	"context"
	"fmt"
	"time"
)

// CloseStaleRunsAtomic 关闭 window_end 已过 grace 的未启动 run(单批单事务,
// FOR UPDATE SKIP LOCKED,多副本安全);返回关闭条数。
// tenantScope 非空时仅关闭该租户;为空时排除测试/演示租户(生产面)。
func (r *Repo) CloseStaleRunsAtomic(ctx context.Context, tenantScope string, now time.Time, grace time.Duration, limit int) (int, error) {
	if grace < 0 {
		grace = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	cutoff := now.Add(-grace)
	q := `
		SELECT id, tenant_id
		FROM analysis_runs
		WHERE state IN ('ACCEPTED','PREPARING')
		  AND window_end IS NOT NULL AND window_end < $1`
	args := []interface{}{cutoff}
	if tenantScope != "" {
		q += ` AND tenant_id=$2`
		args = append(args, tenantScope)
	} else {
		q += ` AND tenant_id NOT LIKE 'integration-%' AND tenant_id <> 'live-gw-demo'`
	}
	q += `
		ORDER BY created_at
		LIMIT $` + fmt.Sprint(len(args)+1) + `
		FOR UPDATE SKIP LOCKED`
	args = append(args, limit)
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("scan stale runs: %w", err)
	}
	type staleRun struct {
		id, tenant string
	}
	var batch []staleRun
	for rows.Next() {
		var s staleRun
		if err := rows.Scan(&s.id, &s.tenant); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, s)
	}
	rows.Close()

	closed := 0
	for _, s := range batch {
		res, err := tx.ExecContext(ctx, `
			UPDATE analysis_runs SET state='CANCELLED', completeness='INCOMPLETE', integrity_state='UNVERIFIED',
				finding_conclusion='NOT_EVALUATED', cancel_manifest_sha256=$1, finalized_at=now()
			WHERE id=$2 AND state IN ('ACCEPTED','PREPARING')`, "stale-"+s.id, s.id)
		if err != nil {
			return closed, fmt.Errorf("close run: %w", err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE analysis_stage_attempts SET state='CANCELLED'
			WHERE run_id=$1 AND state IN ('PENDING','DISPATCHED')`, s.id); err != nil {
			return closed, fmt.Errorf("cancel attempts: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE analysis_stage_queue SET state='EXPIRED'
			WHERE run_id=$1 AND state IN ('READY','CLAIMED')`, s.id); err != nil {
			return closed, fmt.Errorf("expire queue: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE analysis_admission_reservations SET state='RELEASED'
			WHERE run_id=$1 AND state IN ('RESERVED','CONSUMED')`, s.id); err != nil {
			return closed, fmt.Errorf("release reservation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE analysis_business_phase_projections SET state='CANCELLED'
			WHERE run_id=$1`, s.id); err != nil {
			return closed, fmt.Errorf("cancel projections: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
			VALUES($1,'run',$2::uuid,'STALE_WINDOW_CLOSED','scheduler',
				jsonb_build_object('reason','window ended before activation','grace',$3::bigint))`,
			s.tenant, s.id, grace.Milliseconds()); err != nil {
			return closed, fmt.Errorf("stale history: %w", err)
		}
		closed++
	}
	if err := tx.Commit(); err != nil {
		return closed, fmt.Errorf("commit stale close: %w", err)
	}
	return closed, nil
}

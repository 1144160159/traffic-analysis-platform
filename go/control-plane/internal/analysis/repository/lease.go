// Package repository §76.45.3 lease 过期回收:派发租约(接受窗口)超时未获接受的
// DISPATCHED attempt 回退 PENDING 并重入队(队列 CLAIMED→READY),释放 fencing token,
// 与重新领取同资源账(多副本 SKIP LOCKED 安全)。RUNNING 不接受回退(命令已被执行器
// 接受,终态由回执流驱动)。
package repository

import (
	"context"
	"fmt"
	"time"
)

// ExpireStageLeasesAtomic 扫描 lease 过期的 DISPATCHED attempt,回退并重入队;
// 返回本次回收条数。每批单事务(FOR UPDATE SKIP LOCKED),多副本安全。
func (r *Repo) ExpireStageLeasesAtomic(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT sa.id, sa.tenant_id, sa.run_id, sa.execution_node_id, sa.attempt, q.id
		FROM analysis_stage_attempts sa
		JOIN analysis_stage_queue q ON q.run_id = sa.run_id AND q.execution_node_id = sa.execution_node_id AND q.attempt = sa.attempt
		WHERE sa.state='DISPATCHED' AND sa.lease_expires_at IS NOT NULL AND sa.lease_expires_at < $1
		ORDER BY sa.lease_expires_at
		LIMIT $2
		FOR UPDATE OF sa, q SKIP LOCKED`, now, limit)
	if err != nil {
		return 0, fmt.Errorf("scan expired leases: %w", err)
	}
	type expired struct {
		attemptID, tenantID, runID, nodeID string
		attempt                            int32
		queueID                            string
	}
	var batch []expired
	for rows.Next() {
		var e expired
		if err := rows.Scan(&e.attemptID, &e.tenantID, &e.runID, &e.nodeID, &e.attempt, &e.queueID); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, e)
	}
	rows.Close()

	recovered := 0
	for _, e := range batch {
		// attempt DISPATCHED→PENDING,清 token 与 lease(新领取发新 token)
		res, err := tx.ExecContext(ctx, `
			UPDATE analysis_stage_attempts SET state='PENDING', fencing_token=NULL, lease_expires_at=NULL
			WHERE id=$1 AND state='DISPATCHED' AND lease_expires_at < $2`, e.attemptID, now)
		if err != nil {
			return recovered, fmt.Errorf("revert attempt: %w", err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			continue // 并发已被处理
		}
		// 队列 CLAIMED→READY(重入队;稳定排序保留原 ready_at)
		res, err = tx.ExecContext(ctx, `
			UPDATE analysis_stage_queue SET state='READY', claimed_at=NULL
			WHERE id=$1 AND state='CLAIMED'`, e.queueID)
		if err != nil {
			return recovered, fmt.Errorf("requeue: %w", err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
			VALUES($1,'stage_attempt',$2::uuid,'LEASE_EXPIRED','scheduler',
				jsonb_build_object('execution_node_id',$3::text,'attempt',$4::int,'requeued',true))`,
			e.tenantID, e.attemptID, e.nodeID, e.attempt); err != nil {
			return recovered, fmt.Errorf("lease expiry history: %w", err)
		}
		recovered++
	}
	if err := tx.Commit(); err != nil {
		return recovered, fmt.Errorf("commit lease expiry: %w", err)
	}
	return recovered, nil
}

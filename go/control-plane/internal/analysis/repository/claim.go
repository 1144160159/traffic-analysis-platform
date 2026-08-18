// Package repository §76.45.3 ClaimStageLeaseAtomic:阶段候选经 DRR 稳定排序
// 单事务领取——CAS queue READY→CLAIMED、CAS attempt PENDING→DISPATCHED(新 fencing
// token + lease)、更新 DRR(deficit += cost - quantum)、消费准入预留、插入 ACTIVE
// 订阅 dispatch outbox 与审计,全部同事务提交。Kafka 投递由 outbox 中继承接。
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ClaimedStageLease 单事务领取结果(派发循环输入)。
type ClaimedStageLease struct {
	PendingSourceAttempt
	FencingToken string
	CostMilli    int64
	QueueID      string
}

// ClaimStageLeaseAtomic 按 DRR 稳定排序领取一条 READY 的 SOURCE_ACTIVATE 候选并
// 在同一事务内完成:queue CAS、attempt CAS DISPATCHED+lease、DRR 更新、准入预留
// 消费、ACTIVE 订阅 dispatch outbox、审计。无候选返回 (nil, nil)。
// tenantScope 非空时仅领取该租户;多副本安全(FOR UPDATE SKIP LOCKED)。
func (r *Repo) ClaimStageLeaseAtomic(ctx context.Context, tenantScope string, leaseTTL time.Duration, now time.Time) (*ClaimedStageLease, error) {
	if leaseTTL <= 0 {
		leaseTTL = 5 * time.Minute
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 稳定排序候选(deadline NULLS LAST, ready_at, run_id, execution_node_id, attempt)
	q := `
		SELECT q.id, q.tenant_id, q.run_id::text, q.execution_node_id, q.attempt, q.cost_milli,
			sa.id, rn.execution_spec_sha256, t.id, COALESCE(p.source_kind,''), COALESCE(p.source_spec,'{}'::jsonb),
			(EXTRACT(EPOCH FROM rn.window_start)*1000)::bigint, (EXTRACT(EPOCH FROM rn.window_end)*1000)::bigint,
			rn.state, COALESCE(t.effective_class,'BASELINE'), COALESCE(t.effective_policy_sha256,''),
			t.plan_revision, rn.revision
		FROM analysis_stage_queue q
		JOIN analysis_stage_attempts sa ON sa.run_id = q.run_id AND sa.execution_node_id = q.execution_node_id AND sa.attempt = q.attempt
		JOIN analysis_runs rn ON rn.id = q.run_id
		JOIN analysis_tasks t ON t.id = rn.task_id
		LEFT JOIN analysis_plan_revisions p ON p.task_definition_id = t.task_definition_id AND p.plan_revision = t.plan_revision
		WHERE q.state='READY' AND sa.state='PENDING' AND q.execution_node_id='SOURCE_ACTIVATE'`
	args := []interface{}{}
	if tenantScope != "" {
		q += ` AND q.tenant_id=$1`
		args = append(args, tenantScope)
	}
	q += `
		ORDER BY q.deadline NULLS LAST, q.ready_at, q.run_id, q.execution_node_id, q.attempt
		LIMIT 1
		FOR UPDATE OF q, sa SKIP LOCKED`

	var c ClaimedStageLease
	var spec []byte
	var ws, we int64
	err = tx.QueryRowContext(ctx, q, args...).Scan(
		&c.QueueID, &c.TenantID, &c.RunID, &c.ExecutionNodeID, &c.Attempt, &c.CostMilli,
		&c.AttemptID, &c.ExecutionSpecSHA256, &c.TaskID, &c.SourceKind, &spec,
		&ws, &we, &c.RunState, &c.EffectiveClass, &c.EffectivePolicySHA, &c.PlanRevision, &c.RunRevision)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim queue candidate: %w", err)
	}
	c.SourceSpec = spec
	c.WindowStartMs, c.WindowEndMs = ws, we

	// 2. queue CAS READY→CLAIMED
	res, err := tx.ExecContext(ctx, `
		UPDATE analysis_stage_queue SET state='CLAIMED', claimed_at=now()
		WHERE id=$1 AND state='READY'`, c.QueueID)
	if err != nil {
		return nil, fmt.Errorf("queue cas: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, fmt.Errorf("queue CAS lost (concurrent claimant)")
	}

	// 3. attempt CAS PENDING→DISPATCHED + 新 fencing token + lease
	token := uuid.NewString()
	leaseExpires := now.Add(leaseTTL)
	res, err = tx.ExecContext(ctx, `
		UPDATE analysis_stage_attempts SET state='DISPATCHED', fencing_token=$1, lease_expires_at=$2
		WHERE id=$3 AND state='PENDING'`, token, leaseExpires, c.AttemptID)
	if err != nil {
		return nil, fmt.Errorf("attempt cas: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, fmt.Errorf("attempt CAS lost (concurrent claimant)")
	}

	// 4. DRR 更新(deficit += cost - quantum;quantum 1000 milli)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_drr_state(tenant_id, scheduling_class, deficit, quantum, last_served_at, scheduler_epoch, updated_at)
		VALUES($1,$2,GREATEST($3::bigint - 1000,0),1000,now(),1,now())
		ON CONFLICT (tenant_id, scheduling_class) DO UPDATE
		SET deficit = GREATEST(analysis_drr_state.deficit + $3::bigint - COALESCE(NULLIF(analysis_drr_state.quantum,0),1000), 0),
		    last_served_at = now(),
		    scheduler_epoch = analysis_drr_state.scheduler_epoch + 1,
		    updated_at = now()`,
		c.TenantID, c.EffectiveClass, c.CostMilli); err != nil {
		return nil, fmt.Errorf("drr update: %w", err)
	}

	// 5. 准入预留消费(RESERVED→CONSUMED;同一资源账,过期不可消费)
	res, err = tx.ExecContext(ctx, `
		UPDATE analysis_admission_reservations SET state='CONSUMED'
		WHERE run_id=$1 AND state='RESERVED' AND expires_at > now()`, c.RunID)
	if err != nil {
		return nil, fmt.Errorf("consume reservation: %w", err)
	}
	consumed, _ := res.RowsAffected()

	// 6. ACTIVE 订阅 dispatch outbox(中继投递;rev=run.revision+1 由载荷状态区分)
	pipelinedFence, err := pipelinedFenceInTx(ctx, tx, c.RunID)
	if err != nil {
		return nil, fmt.Errorf("pipeline fence: %w", err)
	}
	sub := map[string]interface{}{
		"tenant_id":               c.TenantID,
		"run_id":                  c.RunID,
		"task_id":                 c.TaskID,
		"revision":                c.RunRevision,
		"state":                   "ACTIVE",
		"execution_spec_sha256":   c.ExecutionSpecSHA256,
		"plan_revision":           c.PlanRevision,
		"source_kind":             c.SourceKind,
		"window_start_ms":         c.WindowStartMs,
		"window_end_ms":           c.WindowEndMs,
		"prepare_at_ms":           c.WindowStartMs,
		"lease_epoch":             1,
		"effective_policy_sha256": c.EffectivePolicySHA,
		"expires_at_ms":           c.WindowEndMs + 60_000,
	}
	if pipelinedFence != "" {
		sub["fence"] = pipelinedFence
	}
	subJSON, err := json.Marshal(sub)
	if err != nil {
		return nil, fmt.Errorf("marshal active subscription: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_outbox(event_id, topic, key, payload)
		VALUES($1,'analysis.run.events.v1',$2,$3)`,
		uuid.NewString(), c.RunID, subJSON); err != nil {
		return nil, fmt.Errorf("dispatch outbox: %w", err)
	}

	// 7. 审计(选中/折算/预留消费同事务事实)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'stage_attempt',$2::uuid,'DISPATCHED','scheduler',
			jsonb_build_object('queue_id',$3::text,'cost_milli',$4::bigint,'lease_expires_at',$5::text,'reservation_consumed',$6::boolean))`,
		c.TenantID, c.AttemptID, c.QueueID, c.CostMilli, leaseExpires.Format(time.RFC3339), consumed > 0); err != nil {
		return nil, fmt.Errorf("claim history: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim: %w", err)
	}
	c.FencingToken = token
	return &c, nil
}

// pipelinedFenceInTx 事务内读取流水线 fencing token(S1 领取后其 token 即流水线 token)。
func pipelinedFenceInTx(ctx context.Context, tx *sql.Tx, runID string) (string, error) {
	var token string
	err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(fencing_token,'') FROM analysis_stage_attempts
		WHERE run_id=$1 AND activation_mode='PIPELINED_STREAM'
		ORDER BY created_at LIMIT 1`, runID).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return token, nil
}

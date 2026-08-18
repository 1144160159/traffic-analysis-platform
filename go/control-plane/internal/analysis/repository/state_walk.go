// Package repository 状态机行走与准入生命周期(对齐方案 §7.2 / 76.45.3):
// Run 状态 CAS 推进(由 service 侧 ValidateRunTransition 裁决后调用)、
// StageAttempt DISPATCHED 领取、AdmissionReservation 消费/过期。
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AdvanceRunStateAtomic CAS 推进 run 状态:当前状态命中 from 任一值时推进到 to。
// 终态不回退;并发推进由单行 UPDATE 原子裁决。返回 false 表示未命中(已推进/已终态)。
func (r *Repo) AdvanceRunStateAtomic(ctx context.Context, tenantID, runID string, from []string, to string) (bool, error) {
	if len(from) == 0 {
		return false, fmt.Errorf("advance run state: from list is empty")
	}
	args := make([]interface{}, 0, len(from)+3)
	args = append(args, to, runID, tenantID)
	ph := ""
	for i, f := range from {
		if i > 0 {
			ph += ","
		}
		ph += fmt.Sprintf("$%d", i+4)
		args = append(args, f)
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE analysis_runs SET state=$1, revision=revision+1
		WHERE id=$2 AND ($3='' OR tenant_id=$3) AND state IN (`+ph+`)`, args...)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// RecordTriggerSuppression 登记 SUPPRESSED 原因(审计事实;触发表不承载 reason 列)。
func (r *Repo) RecordTriggerSuppression(ctx context.Context, triggerID, reason string) (bool, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		SELECT tenant_id, 'trigger', $1, 'SUPPRESSED', 'scheduler', jsonb_build_object('reason', $2)
		FROM analysis_trigger_instances WHERE id=$1`, triggerID, reason)
	return err == nil, err
}

// HasPublishedRunEvent run 订阅是否已广播(outbox state=PUBLISHED;PlanReady 事实代理)。
func (r *Repo) HasPublishedRunEvent(ctx context.Context, runID string) (bool, error) {
	var one sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM analysis_outbox
		WHERE key=$1 AND topic='analysis.run.events.v1' AND state='PUBLISHED' LIMIT 1`, runID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ReservationState 读取 run 的准入预留状态。
func (r *Repo) ReservationState(ctx context.Context, runID string) (string, error) {
	var s string
	err := r.db.QueryRowContext(ctx, `
		SELECT state FROM analysis_admission_reservations WHERE run_id=$1 LIMIT 1`, runID).Scan(&s)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return s, nil
}

// HasStartedBusinessNode run 是否已有业务节点领取(DISPATCHED/RUNNING/终态)。
func (r *Repo) HasStartedBusinessNode(ctx context.Context, runID string) (bool, error) {
	var one sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM analysis_stage_attempts
		WHERE run_id=$1 AND business_phase_id IN ('S1','S2','S3','S4')
		  AND state IN ('DISPATCHED','RUNNING','SUCCEEDED','FAILED','PARTIAL')
		LIMIT 1`, runID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// AllBusinessNodesTerminal S1-S4 全部业务节点是否终态(terminal/skip/cancel)。
func (r *Repo) AllBusinessNodesTerminal(ctx context.Context, runID string) (bool, error) {
	var one sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM analysis_stage_attempts
		WHERE run_id=$1 AND business_phase_id IN ('S1','S2','S3','S4')
		  AND state NOT IN ('SUCCEEDED','FAILED','PARTIAL','CANCELLED','SKIPPED')
		LIMIT 1`, runID).Scan(&one)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

// GetRunState 读取 run 当前状态(状态机行走裁决输入)。tenantID 可空(run_id 为唯一键)。
func (r *Repo) GetRunState(ctx context.Context, tenantID, runID string) (string, error) {
	var s string
	err := r.db.QueryRowContext(ctx, `
		SELECT state FROM analysis_runs WHERE id=$1 AND ($2='' OR tenant_id=$2)`, runID, tenantID).Scan(&s)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("run not found")
	}
	if err != nil {
		return "", err
	}
	return s, nil
}

// MarkAttemptAcceptedAtomic 执行器已接受命令(DISPATCHED→RUNNING)。
// 这是生命周期事实而非终态回执,不写 stage receipt(对账不变式:每终态
// attempt 恰一条 fence 匹配回执;写双回执会使 RECONCILE 误判差异)。
func (r *Repo) MarkAttemptAcceptedAtomic(ctx context.Context, tenantID, attemptID, fencingToken string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE analysis_stage_attempts SET state='RUNNING'
		WHERE id=$2 AND tenant_id=$1 AND state='DISPATCHED' AND COALESCE(fencing_token,'')=$3`,
		tenantID, attemptID, fencingToken)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// MarkAttemptDispatchedAtomic CAS PENDING→DISPATCHED 并写入 fencing token
// (DEDICATED_OPERATION 节点派发领取;SHARED_STREAM 节点仍走逻辑准入 PENDING→RUNNING)。
func (r *Repo) MarkAttemptDispatchedAtomic(ctx context.Context, tenantID, attemptID, fencingToken string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE analysis_stage_attempts SET state='DISPATCHED', fencing_token=$3, started_at=now()
		WHERE id=$2 AND tenant_id=$1 AND state='PENDING'`, tenantID, attemptID, fencingToken)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ConsumeReservationAtomic RESERVED→CONSUMED(未过期),返回 false 表示不存在/已过期/已消费。
// 语义:调度前置准备冻结的容量准入事实在首个执行节点领取时消费;
// 过期保留须重新准入(ADMISSION_EXPIRED),不能直接启动。
func (r *Repo) ConsumeReservationAtomic(ctx context.Context, tenantID, runID string, now time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE analysis_admission_reservations SET state='CONSUMED', authority_revision=authority_revision+1
		WHERE tenant_id=$1 AND run_id=$2 AND state='RESERVED' AND expires_at > $3`,
		tenantID, runID, now)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ExpireReservationsAtomic 过期扫描:RESERVED 且超过 expires_at → EXPIRED。
// 过期必须重新准入,不能直接启动或直接失败(§76.45.3)。
func (r *Repo) ExpireReservationsAtomic(ctx context.Context, tenantID string, now time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE analysis_admission_reservations SET state='EXPIRED', authority_revision=authority_revision+1
		WHERE tenant_id=$1 AND state='RESERVED' AND expires_at <= $2`, tenantID, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// HasActiveRunForDefinition 定义下是否存在非终态 run(FORBID_OVERLAP 判定用)。
func (r *Repo) HasActiveRunForDefinition(ctx context.Context, tenantID, taskDefinitionID string) (bool, error) {
	var one sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM analysis_runs
		WHERE tenant_id=$1 AND task_id IN (SELECT id FROM analysis_tasks WHERE task_definition_id=$2)
		  AND state NOT IN ('SUCCEEDED','PARTIALLY_SUCCEEDED','FAILED','CANCELLED')
		LIMIT 1`, tenantID, taskDefinitionID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetDefinitionDefaultClass 读取定义默认调度类别(有效策略解析步骤 1)。
func (r *Repo) GetDefinitionDefaultClass(ctx context.Context, tenantID, taskDefinitionID string) (string, error) {
	var class string
	err := r.db.QueryRowContext(ctx, `
		SELECT default_scheduling_class FROM analysis_task_definitions
		WHERE tenant_id=$1 AND id=$2`, tenantID, taskDefinitionID).Scan(&class)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("definition not found")
	}
	if err != nil {
		return "", err
	}
	return class, nil
}

// FindScheduleBySHA 按冻结 spec 哈希回源调度修订(幂等回放回源;取最高修订)。
func (r *Repo) FindScheduleBySHA(ctx context.Context, tenantID, taskDefinitionID, scheduleSHA string) (string, int64, error) {
	var id string
	var revision int64
	err := r.db.QueryRowContext(ctx, `
		SELECT id, revision FROM analysis_schedule_revisions
		WHERE tenant_id=$1 AND task_definition_id=$2 AND schedule_sha256=$3
		ORDER BY revision DESC LIMIT 1`, tenantID, taskDefinitionID, scheduleSHA).Scan(&id, &revision)
	if err == sql.ErrNoRows {
		return "", 0, fmt.Errorf("schedule revision by sha not found")
	}
	if err != nil {
		return "", 0, err
	}
	return id, revision, nil
}

// NextScheduleRevision 定义下下一个调度修订号(COALESCE(MAX,0)+1)。
func (r *Repo) NextScheduleRevision(ctx context.Context, tenantID, taskDefinitionID string) (int64, error) {
	var revision int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(revision),0)+1 FROM analysis_schedule_revisions
		WHERE tenant_id=$1 AND task_definition_id=$2`, tenantID, taskDefinitionID).Scan(&revision)
	if err != nil {
		return 0, err
	}
	return revision, nil
}

// ScheduleHeadRow 调度激活头(只读视图)。
type ScheduleHeadRow struct {
	State             string
	AuthorityRevision int64
}

// GetScheduleHead 读取调度激活头(激活/暂停 CAS 的 expected revision 来源)。
func (r *Repo) GetScheduleHead(ctx context.Context, tenantID, scheduleID string) (*ScheduleHeadRow, error) {
	var h ScheduleHeadRow
	err := r.db.QueryRowContext(ctx, `
		SELECT state, authority_revision FROM analysis_schedule_activation_heads
		WHERE tenant_id=$1 AND schedule_id=$2`, tenantID, scheduleID).Scan(&h.State, &h.AuthorityRevision)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("schedule head not found")
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// ActivateScheduleAtomic CAS 激活头 DRAFT|PAUSED→ACTIVE(expected authority revision 校验)+ 审计。
func (r *Repo) ActivateScheduleAtomic(ctx context.Context, tenantID, scheduleID string, expectedRevision int64, actor string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		UPDATE analysis_schedule_activation_heads SET state='ACTIVE', authority_revision=authority_revision+1, updated_at=now()
		WHERE tenant_id=$1 AND schedule_id=$2 AND state IN ('DRAFT','PAUSED') AND authority_revision=$3`, tenantID, scheduleID, expectedRevision)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return 0, fmt.Errorf("schedule activation CAS failed (expected DRAFT|PAUSED@%d)", expectedRevision)
	}
	newRevision := expectedRevision + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'schedule',$2,'ACTIVATED',$3,jsonb_build_object('authority_revision', $4::bigint))`,
		tenantID, scheduleID, actor, newRevision); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return newRevision, nil
}

// PauseScheduleAtomic CAS 激活头 ACTIVE→PAUSED(只影响未来触发,不取消当前 Run)+ 审计。
func (r *Repo) PauseScheduleAtomic(ctx context.Context, tenantID, scheduleID string, expectedRevision int64, actor string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		UPDATE analysis_schedule_activation_heads SET state='PAUSED', authority_revision=authority_revision+1, updated_at=now()
		WHERE tenant_id=$1 AND schedule_id=$2 AND state='ACTIVE' AND authority_revision=$3`, tenantID, scheduleID, expectedRevision)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return 0, fmt.Errorf("schedule pause CAS failed (expected ACTIVE@%d)", expectedRevision)
	}
	newRevision := expectedRevision + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'schedule',$2,'PAUSED',$3,jsonb_build_object('authority_revision', $4::bigint))`,
		tenantID, scheduleID, actor, newRevision); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return newRevision, nil
}

// ScheduleView 调度修订列表视图(含激活头状态)。
type ScheduleView struct {
	ScheduleID           string
	TaskDefinitionID     string
	Revision             int64
	ApprovedPlanRevision int64
	ExecutionSpecSHA256  string
	TriggerKind          string
	WindowOrCron         []byte
	MisfirePolicy        string
	ConcurrencyPolicy    string
	SchedulingClass      string
	HeadState            string
	AuthorityRevision    int64
	CreatedAt            time.Time
}

// ListSchedules 调度修订列表(UI/审计;激活头状态随行)。
func (r *Repo) ListSchedules(ctx context.Context, tenantID string) ([]ScheduleView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.task_definition_id, s.revision, s.approved_plan_revision, s.execution_spec_sha256,
			s.trigger_kind, s.window_or_cron, s.misfire_policy, s.concurrency_policy, s.scheduling_class,
			h.state, h.authority_revision, s.created_at
		FROM analysis_schedule_revisions s
		JOIN analysis_schedule_activation_heads h ON h.schedule_id = s.id
		WHERE s.tenant_id=$1 ORDER BY s.created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScheduleView
	for rows.Next() {
		var s ScheduleView
		if err := rows.Scan(&s.ScheduleID, &s.TaskDefinitionID, &s.Revision, &s.ApprovedPlanRevision,
			&s.ExecutionSpecSHA256, &s.TriggerKind, &s.WindowOrCron, &s.MisfirePolicy, &s.ConcurrencyPolicy,
			&s.SchedulingClass, &s.HeadState, &s.AuthorityRevision, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

var _ = uuid.NewString

// RecordDrrServe DRR 台账:按 (tenant, scheduling_class) 记录一次服务
// (deficit += cost - quantum;scheduler_epoch+1)。cost 当前取 1 单位/领取
// (向量折算 CPU/memory/GPU/IO 的冻结权重待目录补齐,如实登记)。
func (r *Repo) RecordDrrServe(ctx context.Context, tenantID, class string, cost, quantum int64) error {
	if class == "" {
		class = "BASELINE"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO analysis_drr_state(tenant_id, scheduling_class, deficit, quantum, last_served_at, scheduler_epoch, updated_at)
		VALUES($1,$2,GREATEST($3::bigint - $4::bigint,0),$4::bigint,now(),1,now())
		ON CONFLICT (tenant_id, scheduling_class) DO UPDATE
		SET deficit = GREATEST(analysis_drr_state.deficit + $3::bigint - $4::bigint, 0),
		    last_served_at = now(),
		    scheduler_epoch = analysis_drr_state.scheduler_epoch + 1,
		    updated_at = now()`,
		tenantID, class, cost, quantum)
	return err
}

// GetRunSchedulingClass 读取 run 的 effective_class(经 task 冻结快照)。
func (r *Repo) GetRunSchedulingClass(ctx context.Context, runID string) (string, error) {
	var class string
	err := r.db.QueryRowContext(ctx, `
		SELECT t.effective_class FROM analysis_tasks t
		JOIN analysis_runs rn ON rn.task_id = t.id
		WHERE rn.id=$1`, runID).Scan(&class)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("run task binding not found")
	}
	if err != nil {
		return "", err
	}
	return class, nil
}

// GetRunSubscriptionFacts 订阅信封富化事实:task/plan/source/effective policy 快照。
func (r *Repo) GetRunSubscriptionFacts(ctx context.Context, runID string) (taskID string, planRevision int64, sourceKind, effectivePolicySHA string, err error) {
	err = r.db.QueryRowContext(ctx, `
		SELECT t.id, t.plan_revision, COALESCE(p.source_kind,''), COALESCE(t.effective_policy_sha256,'')
		FROM analysis_tasks t
		JOIN analysis_runs rn ON rn.task_id = t.id
		LEFT JOIN analysis_plan_revisions p ON p.task_definition_id = t.task_definition_id AND p.plan_revision = t.plan_revision
		WHERE rn.id=$1`, runID).Scan(&taskID, &planRevision, &sourceKind, &effectivePolicySHA)
	if err == sql.ErrNoRows {
		return "", 0, "", "", fmt.Errorf("run facts not found")
	}
	if err != nil {
		return "", 0, "", "", err
	}
	return taskID, planRevision, sourceKind, effectivePolicySHA, nil
}

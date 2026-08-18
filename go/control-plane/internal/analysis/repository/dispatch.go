package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// PendingSourceAttempt 可派发的 SOURCE_ACTIVATE 尝试(含冻结计划源字段)。
type PendingSourceAttempt struct {
	TenantID            string
	RunID               string
	TaskID              string
	AttemptID           string
	ExecutionNodeID     string
	Attempt             int32
	SourceKind          string
	SourceSpec          json.RawMessage
	WindowStartMs       int64
	WindowEndMs         int64
	ExecutionSpecSHA256 string
	RunState            string
	// 单事务领取(§76.45.3)同事务事实:有效类别/策略/计划修订/run 修订。
	EffectiveClass     string
	EffectivePolicySHA string
	PlanRevision       int64
	RunRevision        int64
}

// NextPendingSourceAttempt 领取一条 PENDING 的 SOURCE_ACTIVATE 尝试(FOR UPDATE SKIP LOCKED)。
// tenantScope 非空时仅领取该租户;只领取运行状态处于可执行区间的尝试;无候选返回 (nil, nil)。
func (r *Repo) NextPendingSourceAttempt(ctx context.Context, tenantScope string) (*PendingSourceAttempt, error) {
	var a PendingSourceAttempt
	var spec json.RawMessage
	var windowStart, windowEnd sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT sa.tenant_id, sa.run_id, rn.task_id, sa.id, sa.execution_node_id, sa.attempt,
			COALESCE(p.source_kind,''), COALESCE(p.source_spec,'{}'::jsonb),
			rn.window_start, rn.window_end, rn.execution_spec_sha256, rn.state
		FROM analysis_stage_attempts sa
		JOIN analysis_runs rn ON rn.id = sa.run_id
		JOIN analysis_tasks tk ON tk.id = rn.task_id
		LEFT JOIN analysis_plan_revisions p ON p.tenant_id = sa.tenant_id
			AND p.task_definition_id = tk.task_definition_id
			AND p.plan_revision = tk.plan_revision
		WHERE sa.execution_node_id='SOURCE_ACTIVATE' AND sa.state='PENDING'
			AND rn.state IN ('ACCEPTED','PREPARING','QUEUED','RUNNING')
			AND ($1::text='' OR sa.tenant_id=$1::text)
		ORDER BY sa.created_at LIMIT 1
		FOR UPDATE OF sa SKIP LOCKED`, tenantScope).Scan(
		&a.TenantID, &a.RunID, &a.TaskID, &a.AttemptID, &a.ExecutionNodeID, &a.Attempt,
		&a.SourceKind, &spec, &windowStart, &windowEnd, &a.ExecutionSpecSHA256, &a.RunState)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.SourceSpec = spec
	if windowStart.Valid {
		a.WindowStartMs = windowStart.Time.UnixMilli()
	}
	if windowEnd.Valid {
		a.WindowEndMs = windowEnd.Time.UnixMilli()
	}
	return &a, nil
}

// MarkAttemptRunningAtomic CAS PENDING→RUNNING 并写入 fencing token。
// 返回 false 表示 CAS 未命中(已被其他副本领取)。
func (r *Repo) MarkAttemptRunningAtomic(ctx context.Context, tenantID, attemptID, fencingToken string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE analysis_stage_attempts SET state='RUNNING', fencing_token=$3, started_at=now()
		WHERE id=$2 AND tenant_id=$1 AND state='PENDING'`, tenantID, attemptID, fencingToken)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// NewFencingToken 生成 fencing token(调度派发用)。
func NewFencingToken() string { return uuid.NewString() }

// NextProbeCommandRevision 按 (tenant, probe) 原子递增命令 revision。
// 探针侧要求命令 revision 严格单调(否则 stale_command_revision 拒绝执行);
// 调度中心是唯一命令源,必须保证同一探针上每个命令的 revision 递增。
func (r *Repo) NextProbeCommandRevision(ctx context.Context, tenantID, probeID string) (int64, error) {
	var revision int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO analysis_probe_command_revisions (tenant_id, probe_id, command_revision)
		VALUES ($1, $2, 1)
		ON CONFLICT (tenant_id, probe_id)
		DO UPDATE SET command_revision = analysis_probe_command_revisions.command_revision + 1,
		              updated_at = now()
		RETURNING command_revision`, tenantID, probeID).Scan(&revision)
	if err != nil {
		return 0, err
	}
	return revision, nil
}

// GatingRun 闸门候选:SOURCE_ACTIVATE 已 SUCCEEDED 且后续阶段存在 PENDING 尝试。
type GatingRun struct {
	TenantID        string
	RunID           string
	RunState        string
	AttemptID       string
	ExecutionNodeID string
	BusinessPhaseID string
	Attempt         int32
	FencingToken    string
}

// NextGatingBatch 领取一个可开闸的流水线批次:SOURCE_ACTIVATE 已离开 PENDING
// (已派发:RUNNING/SUCCEEDED/FAILED)且尚未开闸的 run,其全部 PIPELINED_STREAM
// 数据面阶段(S2/S3/S4 共享流)一次性批量开闸,共享一个 run 级 fencing token——
// envelope 回显同一 token,各阶段回执与各自 attempt 的 token 匹配(流水线阶段
// 并发消费同一 run envelope 流)。
// 开闸以"源阶段已派发"为准而非"源阶段已 SUCCEEDED":流水线阶段回执按窗口
// 时钟在 window_end+grace 独立触发,若等 S1 SUCCEEDED(长回放/延迟派发可晚于
// window_end),回执会先于开闸到达而被确定性丢弃,run 永久卡死。S1 失败时
// S2-S4 照常开闸,终局由闭包真值表按 RequiredNodeFailed 裁决。
func (r *Repo) NextGatingBatch(ctx context.Context) (*GatingBatch, error) {
	var lead GatingRun
	var fence sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT sa.tenant_id, sa.run_id, rn.state, sa.id, sa.execution_node_id, sa.business_phase_id, sa.attempt,
			COALESCE(sa.fencing_token,'')
		FROM analysis_stage_attempts sa
		JOIN analysis_runs rn ON rn.id = sa.run_id
		WHERE sa.state='PENDING'
			AND sa.activation_mode='PIPELINED_STREAM'
			AND rn.state IN ('ACCEPTED','PREPARING','QUEUED','RUNNING')
			AND NOT EXISTS (
				SELECT 1 FROM analysis_stage_attempts src
				WHERE src.run_id = sa.run_id
					AND src.execution_node_id = 'SOURCE_ACTIVATE'
					AND src.state = 'PENDING')
			AND NOT EXISTS (
				SELECT 1 FROM analysis_stage_attempts started
				WHERE started.run_id = sa.run_id
					AND started.business_phase_id IN ('S2','S3','S4')
					AND started.state IN ('RUNNING','SUCCEEDED','FAILED'))
		ORDER BY sa.created_at LIMIT 1
		FOR UPDATE OF sa SKIP LOCKED`).Scan(
		&lead.TenantID, &lead.RunID, &lead.RunState, &lead.AttemptID, &lead.ExecutionNodeID, &lead.BusinessPhaseID, &lead.Attempt, &fence)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lead.FencingToken = fence.String

	// 同 run 全部 PIPELINED PENDING 尝试(S2/S3/S4 共享流阶段,同一 token 批量开闸)
	rows, err := r.db.QueryContext(ctx, `
		SELECT sa.id, sa.execution_node_id, sa.business_phase_id, sa.attempt
		FROM analysis_stage_attempts sa
		WHERE sa.run_id=$1 AND sa.state='PENDING' AND sa.activation_mode='PIPELINED_STREAM' AND sa.id <> $2
		ORDER BY sa.created_at FOR UPDATE OF sa SKIP LOCKED`, lead.RunID, lead.AttemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batch := &GatingBatch{TenantID: lead.TenantID, RunID: lead.RunID, RunState: lead.RunState, Phase: lead.BusinessPhaseID}
	batch.Attempts = append(batch.Attempts, GatingRun{
		TenantID: lead.TenantID, RunID: lead.RunID, RunState: lead.RunState,
		AttemptID: lead.AttemptID, ExecutionNodeID: lead.ExecutionNodeID,
		BusinessPhaseID: lead.BusinessPhaseID, Attempt: lead.Attempt, FencingToken: lead.FencingToken,
	})
	for rows.Next() {
		var a GatingRun
		if err := rows.Scan(&a.AttemptID, &a.ExecutionNodeID, &a.BusinessPhaseID, &a.Attempt); err != nil {
			return nil, err
		}
		a.TenantID = lead.TenantID
		a.RunID = lead.RunID
		a.RunState = lead.RunState
		batch.Attempts = append(batch.Attempts, a)
	}
	return batch, rows.Err()
}

// GatingBatch 阶段批量开闸候选。
type GatingBatch struct {
	TenantID string
	RunID    string
	RunState string
	Phase    string
	Attempts []GatingRun
}

// NextRunForFinalize 领取一条全部业务阶段终态且未终局的 run(FOR UPDATE SKIP LOCKED)。
func (r *Repo) NextRunForFinalize(ctx context.Context) (*PendingSourceAttempt, error) {
	var a PendingSourceAttempt
	err := r.db.QueryRowContext(ctx, `
		SELECT rn.tenant_id, rn.id, rn.task_id, rn.state, rn.execution_spec_sha256
		FROM analysis_runs rn
		WHERE rn.state IN ('RUNNING','QUEUED')
			AND NOT EXISTS (
				SELECT 1 FROM analysis_stage_attempts sa
				WHERE sa.run_id = rn.id AND sa.state NOT IN ('SUCCEEDED','FAILED'))
		ORDER BY rn.created_at LIMIT 1
		FOR UPDATE OF rn SKIP LOCKED`).Scan(
		&a.TenantID, &a.RunID, &a.TaskID, &a.RunState, &a.ExecutionSpecSHA256)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

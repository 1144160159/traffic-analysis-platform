// Package repository 调度域 PostgreSQL 权威事务(ENG-CMD-001:同事务 state+history+audit+outbox+receipt)。
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

// Repo 调度权威仓储。
type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// MaterializeCommand 物化命令(卷A §3.2)。
type MaterializeCommand struct {
	TenantID              string
	IdentityKind          string // actor|schedule|event
	CanonicalIdentityHash string
	RequestSHA256         string
	TriggerInstanceID     string
	TriggerKind           string
	WindowID              string
	WindowStartMs         int64
	WindowEndMs           int64
	TaskDefinitionID      string
	PlanRevision          int64
	ExecutionSpecSHA256   string
	ScheduleRevision      int64
	EffectiveClass        string
	EffectivePolicySHA256 string
	ResourcePool          string
	ResourceVectorJSON    []byte
	// QueueCostMilli 阶段队列候选的 DRR cost(冻结权重折算,milli);
	// <=0 时按 1 quantum(1000)计。
	QueueCostMilli int64
	ExpiresAt      time.Time
	NodesJSON      []byte // required ExecutionNode exact-set: [{business_phase_id, execution_node_id, provider_mode, activation_mode}]
	PlanSpecJSON   []byte
}

// MaterializedReceipt 物化回执。
type MaterializedReceipt struct {
	TaskID string
	RunID  string
}

// ErrPayloadMismatch 同 identity 异 request hash(稳定 409)。
var ErrPayloadMismatch = errors.New("idempotency payload mismatch")

// ErrTriggerConflict 触发实例非 PENDING_MATERIALIZATION。
var ErrTriggerConflict = errors.New("trigger instance state conflict")

type nodeSeed struct {
	BusinessPhaseID string `json:"business_phase_id"`
	ExecutionNodeID string `json:"execution_node_id"`
	ProviderMode    string `json:"provider_mode"`
	ActivationMode  string `json:"activation_mode"`
}

// MaterializeAnalysisTaskAtomic 物化事务:ledger 查重→锁 trigger→校验计划→插 Task/Run/attempts/投影种子
// →CAS trigger→history/audit/outbox/receipt→COMMIT。精确重放返回原 receipt。
func (r *Repo) MaterializeAnalysisTaskAtomic(ctx context.Context, cmd MaterializeCommand) (*MaterializedReceipt, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin materialize: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. advisory lock + ledger
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, cmd.CanonicalIdentityHash); err != nil {
		return nil, false, fmt.Errorf("identity lock: %w", err)
	}
	var ledgerSHA string
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM analysis_materialization_ledger WHERE identity_hash=$1`, cmd.CanonicalIdentityHash).Scan(&ledgerSHA)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// 首见:写入台账(未提交;本事务内其他并发由 advisory lock 串行化)
		if _, err := tx.ExecContext(ctx, `INSERT INTO analysis_materialization_ledger(identity_hash, request_sha256) VALUES($1,$2)`, cmd.CanonicalIdentityHash, cmd.RequestSHA256); err != nil {
			return nil, false, fmt.Errorf("insert ledger: %w", err)
		}
	case err != nil:
		return nil, false, fmt.Errorf("query ledger: %w", err)
	case ledgerSHA == cmd.RequestSHA256:
		return nil, true, nil // 精确重放:由上层回放原 receipt
	default:
		return nil, false, ErrPayloadMismatch
	}

	// 2. 锁 trigger 并校验状态
	var triggerState string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM analysis_trigger_instances WHERE id=$1 FOR UPDATE`, cmd.TriggerInstanceID).Scan(&triggerState); err != nil {
		return nil, false, fmt.Errorf("lock trigger: %w", err)
	}
	if triggerState != "PENDING_MATERIALIZATION" {
		return nil, false, ErrTriggerConflict
	}

	// 3. 校验计划绑定(def+plan 双哈希)
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM analysis_plan_revisions p
		JOIN analysis_task_definitions d ON d.id = p.task_definition_id
		WHERE d.id=$1 AND d.tenant_id=$2 AND p.plan_revision=$3 AND p.execution_spec_sha256=$4`,
		cmd.TaskDefinitionID, cmd.TenantID, cmd.PlanRevision, cmd.ExecutionSpecSHA256).Scan(&count); err != nil {
		return nil, false, fmt.Errorf("verify plan: %w", err)
	}
	if count != 1 {
		return nil, false, fmt.Errorf("%s: plan binding mismatch", contract.ErrCodePlanNotApproved)
	}

	// 4. 插入 Task + Run
	taskID := uuid.NewString()
	runID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_tasks(id, tenant_id, task_definition_id, plan_revision, execution_spec_sha256,
			schedule_revision, trigger_instance_id, effective_class, effective_policy_sha256, current_run_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		taskID, cmd.TenantID, cmd.TaskDefinitionID, cmd.PlanRevision, cmd.ExecutionSpecSHA256,
		cmd.ScheduleRevision, cmd.TriggerInstanceID, cmd.EffectiveClass, cmd.EffectivePolicySHA256, runID); err != nil {
		return nil, false, fmt.Errorf("insert task: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_runs(id, tenant_id, task_id, execution_spec_sha256, state,
			window_start, window_end, created_at)
		VALUES($1,$2,$3,$4,'ACCEPTED', to_timestamp($5/1000.0), to_timestamp($6/1000.0), now())`,
		runID, cmd.TenantID, taskID, cmd.ExecutionSpecSHA256, cmd.WindowStartMs, cmd.WindowEndMs); err != nil {
		return nil, false, fmt.Errorf("insert run: %w", err)
	}

	// 5. 种子:required ExecutionNode attempts + 五段业务投影
	var nodes []nodeSeed
	if err := json.Unmarshal(cmd.NodesJSON, &nodes); err != nil {
		return nil, false, fmt.Errorf("decode nodes: %w", err)
	}
	// 预分配 fencing token:流水线阶段(S2/S3/S4 共享流)共享一个 run 级 token,
	// 权威阶段(S5)共享另一个;采集阶段(S1)由派发时分配。预分配使 rev-1 订阅
	// 即可携带 fence,消除"流先于订阅广播被处理"的竞态(否则回执回显空 fence
	// 被 stale-fence 隔离)。
	pipelinedToken := uuid.NewString()
	s5Token := uuid.NewString()
	costMilli := cmd.QueueCostMilli
	if costMilli <= 0 {
		costMilli = 1000
	}
	for _, n := range nodes {
		var fence *string
		switch n.ActivationMode {
		case "PIPELINED_STREAM":
			fence = &pipelinedToken
		case "AUTHORITY_LOCAL":
			fence = &s5Token
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO analysis_stage_attempts(id, tenant_id, run_id, business_phase_id, execution_node_id, attempt, state, provider_mode, activation_mode, fencing_token)
			VALUES($1,$2,$3,$4,$5,1,'PENDING',$6,$7,$8)`,
			uuid.NewString(), cmd.TenantID, runID, n.BusinessPhaseID, n.ExecutionNodeID, n.ProviderMode, n.ActivationMode, fence); err != nil {
			return nil, false, fmt.Errorf("insert stage attempt: %w", err)
		}
		// 阶段候选队列(§76.45.3):稳定排序 + DRR cost 同事务种子。
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO analysis_stage_queue(id, tenant_id, run_id, execution_node_id, attempt, state, cost_milli, ready_at)
			VALUES($1,$2,$3,$4,1,'READY',$5,now())`,
			uuid.NewString(), cmd.TenantID, runID, n.ExecutionNodeID, costMilli); err != nil {
			return nil, false, fmt.Errorf("insert stage queue: %w", err)
		}
	}
	for _, phase := range []string{"S1", "S2", "S3", "S4", "S5"} {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO analysis_business_phase_projections(run_id, phase_id, state) VALUES($1,$2,'PENDING')`,
			runID, phase); err != nil {
			return nil, false, fmt.Errorf("insert phase projection: %w", err)
		}
	}

	// 6. 容量准入预留(RESERVED)
	if cmd.ResourcePool != "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO analysis_admission_reservations(id, tenant_id, run_id, resource_pool, resource_vector, policy_sha256, state, epoch, expires_at, authority_revision)
			VALUES($1,$2,$3,$4,$5,$6,'RESERVED',1,$7,0)`,
			uuid.NewString(), cmd.TenantID, runID, cmd.ResourcePool, cmd.ResourceVectorJSON, cmd.EffectivePolicySHA256, cmd.ExpiresAt); err != nil {
			return nil, false, fmt.Errorf("insert admission reservation: %w", err)
		}
	}

	// 7. CAS trigger → MATERIALIZED
	res, err := tx.ExecContext(ctx, `
		UPDATE analysis_trigger_instances SET materialized_task_id=$1, state='MATERIALIZED'
		WHERE id=$2 AND state='PENDING_MATERIALIZATION'`, taskID, cmd.TriggerInstanceID)
	if err != nil {
		return nil, false, fmt.Errorf("cas trigger: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, false, ErrTriggerConflict
	}

	// 8. history/audit/outbox/receipt 同事务
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'task',$2,'MATERIALIZED',$3,$4)`,
		cmd.TenantID, taskID, cmd.TenantID, cmd.PlanSpecJSON); err != nil {
		return nil, false, fmt.Errorf("insert history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_outbox(event_id, topic, key, payload)
		VALUES($1,$2,$3,$4)`,
		uuid.NewString(), contract.TopicRunEvents, runID, runSubscriptionSeedJSON(cmd, runID)); err != nil {
		return nil, false, fmt.Errorf("insert outbox: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_receipts(id, tenant_id, operation_id, operation, idempotency_key, request_hash, state)
		VALUES($1,$2,$3,'MATERIALIZE_TASK',$4,$5,'accepted')`,
		uuid.NewString(), cmd.TenantID, taskID, cmd.CanonicalIdentityHash, cmd.RequestSHA256); err != nil {
		return nil, false, fmt.Errorf("insert receipt: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit materialize: %w", err)
	}
	return &MaterializedReceipt{TaskID: taskID, RunID: runID}, false, nil
}

// runSubscriptionSeedJSON 物化时写入的订阅种子(PREPARE,revision 1)。
// 中继器按 payload 直发;S1 领取时再发布更高 revision 的 ACTIVE 订阅。
func runSubscriptionSeedJSON(cmd MaterializeCommand, runID string) []byte {
	seed := map[string]interface{}{
		"tenant_id":             cmd.TenantID,
		"run_id":                runID,
		"state":                 "PREPARE",
		"revision":              int64(1),
		"execution_spec_sha256": cmd.ExecutionSpecSHA256,
		"window_start_ms":       cmd.WindowStartMs,
		"window_end_ms":         cmd.WindowEndMs,
	}
	b, _ := json.Marshal(seed)
	return b
}

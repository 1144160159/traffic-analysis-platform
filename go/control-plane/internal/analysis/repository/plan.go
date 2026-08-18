package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

// SavePlanDraftCommand 计划草稿保存命令(保存不激活)。
type SavePlanDraftCommand struct {
	TenantID           string
	TaskDefinitionID   string
	PlanRevision       int64
	PlanSource         string
	SourceKind         string
	SourceSpec         []byte
	SelectedFeatureIDs []byte
	FeatureSetID       string
	RecognitionModel   string
	DetectorRefs       []byte
	RuleRefs           []byte
	// MachineSummarySchemaRef 冻结字段:执行哈希组成部分,必须随行持久化
	// (live 计划冻结哈希漂移修复:此前保存路径丢弃该字段导致同内容异哈希)。
	MachineSummarySchemaRef string
	StageDAG                []byte
	CompletionPolicy        []byte
	ResourceBudget          []byte
	CatalogRevision         int64
	SelectionOrigins        []byte
	ExecutionSpecSHA256     string
	PlanRevisionSHA256      string
	CreatedBy               string
	RequestSHA256           string
	IdempotencyKey          string
}

// SavePlanDraftAtomic 保存不可变计划修订+治理头(DRAFT)。
// PlanRevision==0 时在事务内按 (tenant, definition) 分配下一修订号(定义级 advisory lock 串行化)。
// 同 identity 同 hash 精确重放;同 identity 异 hash 完整性冲突。
func (r *Repo) SavePlanDraftAtomic(ctx context.Context, cmd SavePlanDraftCommand) (planID string, planRevision int64, replayed bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, false, fmt.Errorf("begin plan draft: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, cmd.IdempotencyKey); err != nil {
		return "", 0, false, fmt.Errorf("plan lock: %w", err)
	}
	var existingHash string
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM analysis_materialization_ledger WHERE identity_hash=$1`, cmd.IdempotencyKey).Scan(&existingHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO analysis_materialization_ledger(identity_hash, request_sha256) VALUES($1,$2)`, cmd.IdempotencyKey, cmd.RequestSHA256); err != nil {
			return "", 0, false, fmt.Errorf("plan ledger: %w", err)
		}
	case err != nil:
		return "", 0, false, fmt.Errorf("plan ledger query: %w", err)
	case existingHash == cmd.RequestSHA256:
		_ = tx.Commit()
		return "", 0, true, nil
	default:
		return "", 0, false, ErrPayloadMismatch
	}

	planRevision = cmd.PlanRevision
	if planRevision == 0 {
		// 修订号分配:定义级 advisory lock + 事务内 MAX+1(并发保存串行化,唯一约束兜底)
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`,
			"plan-rev:"+cmd.TenantID+"/"+cmd.TaskDefinitionID); err != nil {
			return "", 0, false, fmt.Errorf("plan revision lock: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(plan_revision),0)+1 FROM analysis_plan_revisions
			WHERE tenant_id=$1 AND task_definition_id=$2`, cmd.TenantID, cmd.TaskDefinitionID).Scan(&planRevision); err != nil {
			return "", 0, false, fmt.Errorf("allocate plan revision: %w", err)
		}
	}

	// 唯一 (tenant, definition, plan_revision):旧修订号不复用
	planID = uuid.NewString()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_plan_revisions(id, tenant_id, task_definition_id, plan_revision, plan_source, source_kind, source_spec,
			selected_feature_ids, feature_set_id, encrypted_recognition_model_ref, threat_detector_refs, rule_refs,
			machine_summary_schema_ref, stage_dag, completion_policy, resource_budget, catalog_revision,
			selection_origins, canonicalization_version, execution_spec_sha256, plan_revision_sha256, created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,'v1',$19,$20,$21)
		ON CONFLICT (tenant_id, task_definition_id, plan_revision) DO NOTHING`,
		planID, cmd.TenantID, cmd.TaskDefinitionID, planRevision, cmd.PlanSource, cmd.SourceKind, cmd.SourceSpec,
		cmd.SelectedFeatureIDs, cmd.FeatureSetID, cmd.RecognitionModel, cmd.DetectorRefs, cmd.RuleRefs,
		cmd.MachineSummarySchemaRef, cmd.StageDAG, cmd.CompletionPolicy, cmd.ResourceBudget, cmd.CatalogRevision,
		cmd.SelectionOrigins, cmd.ExecutionSpecSHA256, cmd.PlanRevisionSHA256, cmd.CreatedBy)
	if err != nil {
		return "", 0, false, fmt.Errorf("insert plan: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", 0, false, fmt.Errorf("plan revision %d already exists", planRevision)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_plan_governance_heads(tenant_id, plan_id, state, authority_revision)
		VALUES($1,$2,'DRAFT',0)`, cmd.TenantID, planID); err != nil {
		return "", 0, false, fmt.Errorf("insert governance head: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'plan',$2,'PLAN_DRAFT_SAVED',$3,$4)`, cmd.TenantID, planID, cmd.CreatedBy, cmd.SelectionOrigins); err != nil {
		return "", 0, false, fmt.Errorf("plan history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", 0, false, fmt.Errorf("commit plan draft: %w", err)
	}
	return planID, planRevision, false, nil
}

// ApprovePlanCommand 审批命令(maker/checker)。
type ApprovePlanCommand struct {
	TenantID         string
	PlanID           string
	Maker            string
	Checker          string
	ExpectedRevision int64
	NewState         string // APPROVED|ACTIVE
}

// ApproveOrActivatePlanAtomic maker/checker + CAS 治理头;审批历史追加。
func (r *Repo) ApproveOrActivatePlanAtomic(ctx context.Context, cmd ApprovePlanCommand) error {
	if cmd.Maker == cmd.Checker {
		return fmt.Errorf("maker and checker must differ")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin approve: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE analysis_plan_governance_heads SET state=$3, authority_revision=authority_revision+1,
			approved_by=$4, approved_at=now(), updated_at=now()
		WHERE plan_id=$1 AND tenant_id=$2 AND authority_revision=$5`,
		cmd.PlanID, cmd.TenantID, cmd.NewState, cmd.Checker, cmd.ExpectedRevision)
	if err != nil {
		return fmt.Errorf("cas governance head: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("%s: governance revision conflict", contract.ErrCodeRevisionConflict)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_plan_approvals(id, tenant_id, plan_id, requested_by, approved_by, state, decided_at)
		VALUES(gen_random_uuid(),$1,$2,$3,$4,'APPROVED', now())`, cmd.TenantID, cmd.PlanID, cmd.Maker, cmd.Checker); err != nil {
		return fmt.Errorf("insert approval: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'plan',$2,$3,$4,$5)`,
		cmd.TenantID, cmd.PlanID, "PLAN_"+cmd.NewState, cmd.Checker, []byte(`{}`)); err != nil {
		return fmt.Errorf("approval history: %w", err)
	}
	if cmd.NewState == "ACTIVE" {
		// 激活即成为定义的当前激活计划修订(ListTasks/绑定语义一致)
		if _, err := tx.ExecContext(ctx, `
			UPDATE analysis_task_definitions d SET active_plan_revision=p.plan_revision, updated_at=now()
			FROM analysis_plan_revisions p
			WHERE p.id=$1 AND d.id=p.task_definition_id AND d.tenant_id=$2`,
			cmd.PlanID, cmd.TenantID); err != nil {
			return fmt.Errorf("activate definition binding: %w", err)
		}
	}
	return tx.Commit()
}

// GetPlanGovernanceHead 读取计划治理头(state + authority_revision),跨租户返回 NOT FOUND。
func (r *Repo) GetPlanGovernanceHead(ctx context.Context, tenantID, planID string) (state string, authorityRevision int64, err error) {
	err = r.db.QueryRowContext(ctx, `
		SELECT g.state, g.authority_revision
		FROM analysis_plan_governance_heads g
		JOIN analysis_plan_revisions p ON p.id=g.plan_id
		WHERE g.plan_id=$1 AND p.tenant_id=$2`, planID, tenantID).Scan(&state, &authorityRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, fmt.Errorf("%s: plan not found", contract.ErrCodeNotFound)
	}
	if err != nil {
		return "", 0, err
	}
	return state, authorityRevision, nil
}

// FindPlanByExecutionSpec 按 (tenant, definition, plan_source, execution_spec_sha256) 回源计划
// (幂等重放回源用;取最高修订)。
func (r *Repo) FindPlanByExecutionSpec(ctx context.Context, tenantID, taskDefinitionID, planSource, specSHA string) (planID string, planRevision int64, err error) {
	err = r.db.QueryRowContext(ctx, `
		SELECT id, plan_revision FROM analysis_plan_revisions
		WHERE tenant_id=$1 AND task_definition_id=$2 AND plan_source=$3 AND execution_spec_sha256=$4
		ORDER BY plan_revision DESC LIMIT 1`, tenantID, taskDefinitionID, planSource, specSHA).Scan(&planID, &planRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, fmt.Errorf("%s: plan not found", contract.ErrCodeNotFound)
	}
	if err != nil {
		return "", 0, err
	}
	return planID, planRevision, nil
}

// SaveScheduleCommand 调度修订保存(激活前不触发)。
type SaveScheduleCommand struct {
	TenantID             string
	TaskDefinitionID     string
	Revision             int64
	ApprovedPlanRevision int64
	ExecutionSpecSHA256  string
	TriggerKind          string
	Timezone             string
	WindowOrCron         []byte
	PrepareLeadTimeMs    int64
	MisfirePolicy        string
	ConcurrencyPolicy    string
	SchedulingClass      string
	ResourceRestrictions []byte
	ScheduleSHA256       string
	RequestSHA256        string
	IdempotencyKey       string
}

// SaveScheduleRevisionAtomic 保存不可变调度修订(精确绑定已批准 plan)+激活头(DRAFT)。
func (r *Repo) SaveScheduleRevisionAtomic(ctx context.Context, cmd SaveScheduleCommand) (scheduleID string, replayed bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("begin schedule: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 精确绑定:计划必须已 APPROVED/ACTIVE
	var govState string
	if err := tx.QueryRowContext(ctx, `
		SELECT g.state FROM analysis_plan_governance_heads g
		JOIN analysis_plan_revisions p ON p.id = g.plan_id
		WHERE p.tenant_id=$1 AND p.task_definition_id=$2 AND p.plan_revision=$3
		  AND p.execution_spec_sha256=$4 FOR UPDATE`,
		cmd.TenantID, cmd.TaskDefinitionID, cmd.ApprovedPlanRevision, cmd.ExecutionSpecSHA256).Scan(&govState); err != nil {
		return "", false, fmt.Errorf("%s: approved plan binding not found", contract.ErrCodePlanNotApproved)
	}
	if govState != "APPROVED" && govState != "ACTIVE" {
		return "", false, fmt.Errorf("%s: plan not approved", contract.ErrCodePlanNotApproved)
	}

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, cmd.IdempotencyKey); err != nil {
		return "", false, fmt.Errorf("schedule lock: %w", err)
	}
	var existingHash string
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM analysis_materialization_ledger WHERE identity_hash=$1`, cmd.IdempotencyKey).Scan(&existingHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO analysis_materialization_ledger(identity_hash, request_sha256) VALUES($1,$2)`, cmd.IdempotencyKey, cmd.RequestSHA256); err != nil {
			return "", false, fmt.Errorf("schedule ledger: %w", err)
		}
	case err != nil:
		return "", false, fmt.Errorf("schedule ledger query: %w", err)
	case existingHash == cmd.RequestSHA256:
		_ = tx.Commit()
		return "", true, nil
	default:
		return "", false, ErrPayloadMismatch
	}

	scheduleID = uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_schedule_revisions(id, tenant_id, task_definition_id, revision, approved_plan_revision,
			execution_spec_sha256, trigger_kind, timezone, window_or_cron, prepare_lead_time_ms, misfire_policy,
			concurrency_policy, scheduling_class, resource_restrictions, schedule_sha256)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		scheduleID, cmd.TenantID, cmd.TaskDefinitionID, cmd.Revision, cmd.ApprovedPlanRevision,
		cmd.ExecutionSpecSHA256, cmd.TriggerKind, cmd.Timezone, cmd.WindowOrCron, cmd.PrepareLeadTimeMs,
		cmd.MisfirePolicy, cmd.ConcurrencyPolicy, cmd.SchedulingClass, cmd.ResourceRestrictions, cmd.ScheduleSHA256); err != nil {
		return "", false, fmt.Errorf("insert schedule: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_schedule_activation_heads(tenant_id, schedule_id, state, authority_revision)
		VALUES($1,$2,'DRAFT',0)`, cmd.TenantID, scheduleID); err != nil {
		return "", false, fmt.Errorf("insert schedule head: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("commit schedule: %w", err)
	}
	return scheduleID, false, nil
}

// ListActiveSchedules 供 Scheduler.Tick 使用(只读)。
type ScheduleRow struct {
	ScheduleID           string
	TenantID             string
	TaskDefinitionID     string
	Revision             int64
	ApprovedPlanRevision int64
	ExecutionSpecSHA256  string
	TriggerKind          string
	Timezone             string
	WindowOrCron         []byte
	PrepareLeadTimeMs    int64
	MisfirePolicy        string
	ConcurrencyPolicy    string
	SchedulingClass      string
	ResourceRestrictions string
}

func (r *Repo) ListActiveSchedules(ctx context.Context, tenantID string) ([]ScheduleRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.tenant_id, s.task_definition_id, s.revision, s.approved_plan_revision,
			s.execution_spec_sha256, s.trigger_kind, s.timezone, s.window_or_cron, s.prepare_lead_time_ms,
			s.misfire_policy, s.concurrency_policy, s.scheduling_class, COALESCE(s.resource_restrictions::text,'{}')
		FROM analysis_schedule_revisions s
		JOIN analysis_schedule_activation_heads h ON h.schedule_id = s.id AND h.state='ACTIVE'
		WHERE s.tenant_id=$1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScheduleRow
	for rows.Next() {
		var s ScheduleRow
		if err := rows.Scan(&s.ScheduleID, &s.TenantID, &s.TaskDefinitionID, &s.Revision, &s.ApprovedPlanRevision,
			&s.ExecutionSpecSHA256, &s.TriggerKind, &s.Timezone, &s.WindowOrCron, &s.PrepareLeadTimeMs,
			&s.MisfirePolicy, &s.ConcurrencyPolicy, &s.SchedulingClass, &s.ResourceRestrictions); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// InsertTriggerInstance 触发实例登记(identity 判重)。
// effectiveClass/resourceRestrictions 为有效调度策略解析输入(§76.45.2),
// 与触发事实同事务冻结;物化 worker 只读取,不重新解析。
func (r *Repo) InsertTriggerInstance(ctx context.Context, tenantID, identityKind, canonicalHash, requestSHA, triggerKind, windowID, taskDefinitionID string, planRevision int64, actor, effectiveClass, resourceRestrictions string, scheduleRevision int64) (triggerID string, created bool, err error) {
	triggerID = uuid.NewString()
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO analysis_trigger_instances(id, tenant_id, identity_kind, canonical_identity_hash, request_sha256, state, trigger_kind, window_id, task_definition_id, plan_revision, actor, effective_class, resource_restrictions, schedule_revision)
		VALUES($1,$2,$3,$4,$5,'PENDING_MATERIALIZATION',$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (tenant_id, identity_kind, canonical_identity_hash) DO NOTHING`,
		triggerID, tenantID, identityKind, canonicalHash, requestSHA, triggerKind, windowID, taskDefinitionID, planRevision, actor, effectiveClass, resourceRestrictions, scheduleRevision)
	if err != nil {
		return "", false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", false, nil // 已存在:跳过
	}
	return triggerID, true, nil
}

// TriggerInstanceRow 触发实例定位行(按判别联合查询)。
type TriggerInstanceRow struct {
	TriggerID          string
	State              string
	MaterializedTaskID string
}

// FindTriggerInstanceByIdentity 按 (tenant, identity_kind, canonical_identity_hash) 定位触发实例;
// 未找到返回 (nil, nil)。
func (r *Repo) FindTriggerInstanceByIdentity(ctx context.Context, tenantID, identityKind, canonicalHash string) (*TriggerInstanceRow, error) {
	var row TriggerInstanceRow
	var mid sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id, state, materialized_task_id FROM analysis_trigger_instances
		WHERE tenant_id=$1 AND identity_kind=$2 AND canonical_identity_hash=$3`,
		tenantID, identityKind, canonicalHash).Scan(&row.TriggerID, &row.State, &mid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.MaterializedTaskID = mid.String
	return &row, nil
}

// GetTaskRunBinding 任务当前运行绑定(task_id → current_run_id),供幂等重放回源。
func (r *Repo) GetTaskRunBinding(ctx context.Context, tenantID, taskID string) (runID string, err error) {
	err = r.db.QueryRowContext(ctx, `
		SELECT current_run_id FROM analysis_tasks WHERE id=$1 AND tenant_id=$2`, taskID, tenantID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%s: task not found", contract.ErrCodeNotFound)
	}
	if err != nil {
		return "", err
	}
	return runID, nil
}

var _ = time.Now

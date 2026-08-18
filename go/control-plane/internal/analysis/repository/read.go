package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

// RunView 运行五轴视图(读 API)。
// JSON 契约统一 snake_case(与 §20/前端 AnalysisRunView/详情端点一致;
// 此前列表端点为 Go 风格键,与详情端点/前端类型不一致——live 视觉排查发现)。
type RunView struct {
	TenantID            string     `json:"tenant_id"`
	RunID               string     `json:"run_id"`
	TaskID              string     `json:"task_id"`
	ExecutionSpecSHA256 string     `json:"execution_spec_sha256"`
	State               string     `json:"state"`
	Completeness        string     `json:"completeness"`
	IntegrityState      string     `json:"integrity_state"`
	FindingConclusion   string     `json:"finding_conclusion"`
	RiskSeverity        string     `json:"risk_severity"`
	ReportState         string     `json:"report_state"` // 六轴正交:人读报告状态(NOT_REQUESTED 由无报告行表达)
	WindowStartMs       int64      `json:"window_start_ms"`
	WindowEndMs         int64      `json:"window_end_ms"`
	Revision            int64      `json:"revision"`
	CreatedAt           time.Time  `json:"created_at"`
	FinalizedAt         *time.Time `json:"finalized_at"`
}

// GetRun 读运行六轴(正交状态,不合并;ReportState 由报告行投影,无报告行=NOT_REQUESTED)。
func (r *Repo) GetRun(ctx context.Context, tenantID, runID string) (*RunView, error) {
	var v RunView
	var ws, we sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT ru.tenant_id, ru.id, ru.task_id, ru.execution_spec_sha256, ru.state, ru.completeness, ru.integrity_state,
			ru.finding_conclusion, ru.risk_severity,
			(EXTRACT(EPOCH FROM ru.window_start)*1000)::bigint, (EXTRACT(EPOCH FROM ru.window_end)*1000)::bigint,
			ru.revision, ru.created_at, ru.finalized_at,
			COALESCE((SELECT hr.state FROM analysis_human_reports hr
				WHERE hr.run_id = ru.id ORDER BY hr.created_at DESC LIMIT 1), 'NOT_REQUESTED')
		FROM analysis_runs ru WHERE ru.id=$1 AND ru.tenant_id=$2`,
		runID, tenantID).Scan(&v.TenantID, &v.RunID, &v.TaskID, &v.ExecutionSpecSHA256, &v.State,
		&v.Completeness, &v.IntegrityState, &v.FindingConclusion, &v.RiskSeverity,
		&ws, &we, &v.Revision, &v.CreatedAt, &v.FinalizedAt, &v.ReportState)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run not found")
	}
	if err != nil {
		return nil, err
	}
	if ws.Valid {
		v.WindowStartMs = ws.Int64
	}
	if we.Valid {
		v.WindowEndMs = we.Int64
	}
	return &v, nil
}

// PendingTrigger 待物化触发。
type PendingTrigger struct {
	TriggerID             string
	TenantID              string
	IdentityKind          string
	CanonicalIdentityHash string
	RequestSHA256         string
	TriggerKind           string
	WindowID              string
	TaskDefinitionID      string
	PlanRevision          int64
	Actor                 string
	EffectiveClass        string
	ResourceRestrictions  string
	ScheduleRevision      int64
}

// LoadPendingTrigger 读取单个待物化触发(FOR UPDATE SKIP LOCKED 领取)。
func (r *Repo) LoadPendingTrigger(ctx context.Context, triggerID string) (*PendingTrigger, error) {
	var t PendingTrigger
	err := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, identity_kind, canonical_identity_hash, request_sha256, trigger_kind, COALESCE(window_id,''),
			COALESCE(task_definition_id,''), COALESCE(plan_revision,0), COALESCE(actor,'')
		FROM analysis_trigger_instances WHERE id=$1 AND state='PENDING_MATERIALIZATION'`,
		triggerID).Scan(&t.TriggerID, &t.TenantID, &t.IdentityKind, &t.CanonicalIdentityHash, &t.RequestSHA256, &t.TriggerKind, &t.WindowID, &t.TaskDefinitionID, &t.PlanRevision, &t.Actor)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// NextPendingTrigger 领取下一个待物化触发(多副本安全)。
func (r *Repo) NextPendingTrigger(ctx context.Context) (*PendingTrigger, error) {
	var t PendingTrigger
	err := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, identity_kind, canonical_identity_hash, request_sha256, trigger_kind, COALESCE(window_id,''),
			COALESCE(task_definition_id,''), COALESCE(plan_revision,0), COALESCE(actor,''),
			COALESCE(effective_class,'BASELINE'), COALESCE(resource_restrictions::text,'{}'), COALESCE(schedule_revision,0)
		FROM analysis_trigger_instances
		WHERE state='PENDING_MATERIALIZATION'
		ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(
		&t.TriggerID, &t.TenantID, &t.IdentityKind, &t.CanonicalIdentityHash, &t.RequestSHA256, &t.TriggerKind, &t.WindowID, &t.TaskDefinitionID, &t.PlanRevision, &t.Actor, &t.EffectiveClass, &t.ResourceRestrictions, &t.ScheduleRevision)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ActivePlanRow 定义当前激活计划。
type ActivePlanRow struct {
	PlanID                  string
	PlanRevision            int64
	PlanSource              string
	SourceKind              string
	SourceSpec              json.RawMessage
	SelectedFeatureIDs      json.RawMessage
	FeatureSetID            string
	RecognitionModel        string
	DetectorRefs            json.RawMessage
	RuleRefs                json.RawMessage
	MachineSummarySchemaRef string
	StageDAG                json.RawMessage
	CompletionPolicy        json.RawMessage
	ResourceBudget          json.RawMessage
	CatalogRevision         int64
	ExecutionSpecSHA256     string
}

// GetActivePlanForDefinition 读取定义的激活计划(经治理头 ACTIVE)。
func (r *Repo) GetActivePlanForDefinition(ctx context.Context, tenantID, defID string) (*ActivePlanRow, error) {
	return r.GetActivePlanForDefinitionBySource(ctx, tenantID, defID, "")
}

// GetActivePlanForDefinitionBySource 读取定义某 plan_source 的激活计划(经治理头 ACTIVE);
// planSource 为空则不区分来源(取最高修订)。
func (r *Repo) GetActivePlanForDefinitionBySource(ctx context.Context, tenantID, defID, planSource string) (*ActivePlanRow, error) {
	var p ActivePlanRow
	q := `
		SELECT p.id, p.plan_revision, p.plan_source, p.source_kind, p.source_spec,
			p.selected_feature_ids, p.feature_set_id, p.encrypted_recognition_model_ref,
			p.threat_detector_refs, p.rule_refs, p.machine_summary_schema_ref,
			p.stage_dag, p.completion_policy, p.resource_budget,
			p.catalog_revision, p.execution_spec_sha256
		FROM analysis_plan_revisions p
		JOIN analysis_plan_governance_heads g ON g.plan_id = p.id AND g.state='ACTIVE'
		WHERE p.tenant_id=$1 AND p.task_definition_id=$2`
	args := []interface{}{tenantID, defID}
	if planSource != "" {
		q += ` AND p.plan_source=$3`
		args = append(args, planSource)
	}
	q += ` ORDER BY p.plan_revision DESC LIMIT 1`
	err := r.db.QueryRowContext(ctx, q, args...).Scan(
		&p.PlanID, &p.PlanRevision, &p.PlanSource, &p.SourceKind, &p.SourceSpec,
		&p.SelectedFeatureIDs, &p.FeatureSetID, &p.RecognitionModel,
		&p.DetectorRefs, &p.RuleRefs, &p.MachineSummarySchemaRef,
		&p.StageDAG, &p.CompletionPolicy, &p.ResourceBudget,
		&p.CatalogRevision, &p.ExecutionSpecSHA256)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%s: no active plan for definition", contract.ErrCodePlanNotApproved)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// EnqueueMaterializeEvent 物化入队事件(outbox)。
func (r *Repo) EnqueueMaterializeEvent(ctx context.Context, tenantID, triggerID string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO analysis_outbox(event_id, topic, key, payload)
		VALUES(gen_random_uuid()::text, $1, $2, jsonb_build_object('trigger_instance_id', $2, 'tenant_id', $3))`,
		contract.TopicRunEvents, triggerID, tenantID)
	return err
}

// GetPlanByDefinitionAndRevision 按 (tenant, definition, plan_revision) 读取冻结计划修订
// (调度触发绑定冻结修订,不随激活头漂移)。
func (r *Repo) GetPlanByDefinitionAndRevision(ctx context.Context, tenantID, defID string, planRevision int64) (*ActivePlanRow, error) {
	var p ActivePlanRow
	err := r.db.QueryRowContext(ctx, `
		SELECT p.id, p.plan_revision, p.plan_source, p.source_kind, p.source_spec,
			p.selected_feature_ids, p.feature_set_id, p.encrypted_recognition_model_ref,
			p.threat_detector_refs, p.rule_refs, p.stage_dag, p.completion_policy, p.resource_budget,
			p.catalog_revision, p.execution_spec_sha256
		FROM analysis_plan_revisions p
		WHERE p.tenant_id=$1 AND p.task_definition_id=$2 AND p.plan_revision=$3`,
		tenantID, defID, planRevision).Scan(
		&p.PlanID, &p.PlanRevision, &p.PlanSource, &p.SourceKind, &p.SourceSpec,
		&p.SelectedFeatureIDs, &p.FeatureSetID, &p.RecognitionModel,
		&p.DetectorRefs, &p.RuleRefs, &p.StageDAG, &p.CompletionPolicy, &p.ResourceBudget,
		&p.CatalogRevision, &p.ExecutionSpecSHA256)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%s: plan revision not found", contract.ErrCodePlanNotApproved)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// RunStageView 阶段视图(读 API)。
type RunStageView struct {
	BusinessPhaseID string `json:"business_phase_id"`
	ExecutionNodeID string `json:"execution_node_id"`
	Attempt         int32  `json:"attempt"`
	State           string `json:"state"`
	ProviderMode    string `json:"provider_mode"`
	ActivationMode  string `json:"activation_mode"`
	SkipReason      string `json:"skip_reason"`
}

func (r *Repo) ListRunStages(ctx context.Context, tenantID, runID string) ([]RunStageView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT business_phase_id, execution_node_id, attempt, state, provider_mode, activation_mode, COALESCE(skip_reason,'')
		FROM analysis_stage_attempts WHERE run_id=$1 AND tenant_id=$2 ORDER BY business_phase_id, execution_node_id, attempt`,
		runID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunStageView
	for rows.Next() {
		var s RunStageView
		if err := rows.Scan(&s.BusinessPhaseID, &s.ExecutionNodeID, &s.Attempt, &s.State, &s.ProviderMode, &s.ActivationMode, &s.SkipReason); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SuppressTrigger 无激活计划的触发置 SUPPRESSED(不创建假 CANCELLED Task)。
func (r *Repo) SuppressTrigger(ctx context.Context, triggerID, reason string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE analysis_trigger_instances SET state='SUPPRESSED'
		WHERE id=$1 AND state='PENDING_MATERIALIZATION'`, triggerID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ListRunsParams 运行列表参数。
type ListRunsParams struct {
	TenantID         string
	State            string
	Limit            int
	Offset           int
	TaskDefinitionID string
}

// ListRuns 运行列表(只读投影;UI 八列)。
func (r *Repo) ListRuns(ctx context.Context, p ListRunsParams) ([]RunView, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT ru.tenant_id, ru.id, ru.task_id, ru.execution_spec_sha256, ru.state, ru.completeness, ru.integrity_state,
			ru.finding_conclusion, ru.risk_severity,
			COALESCE((EXTRACT(EPOCH FROM ru.window_start)*1000)::bigint,0), COALESCE((EXTRACT(EPOCH FROM ru.window_end)*1000)::bigint,0),
			ru.revision, ru.created_at, ru.finalized_at,
			COALESCE((SELECT hr.state FROM analysis_human_reports hr
				WHERE hr.run_id = ru.id ORDER BY hr.created_at DESC LIMIT 1), 'NOT_REQUESTED')
		FROM analysis_runs ru
		WHERE ru.tenant_id=$1 AND ($2='' OR ru.state=$2)
		ORDER BY ru.created_at DESC
		LIMIT $3 OFFSET $4`, p.TenantID, p.State, p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunView
	for rows.Next() {
		var v RunView
		var fin sql.NullTime
		if err := rows.Scan(&v.TenantID, &v.RunID, &v.TaskID, &v.ExecutionSpecSHA256, &v.State,
			&v.Completeness, &v.IntegrityState, &v.FindingConclusion, &v.RiskSeverity,
			&v.WindowStartMs, &v.WindowEndMs, &v.Revision, &v.CreatedAt, &fin, &v.ReportState); err != nil {
			return nil, err
		}
		if fin.Valid {
			t := fin.Time
			v.FinalizedAt = &t
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// TaskView 任务视图(UI 七列;JSON 契约统一 snake_case)。
type TaskView struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	State                  string    `json:"state"`
	Owner                  string    `json:"owner"`
	ActivePlanRevision     int64     `json:"active_plan_revision"`
	ActiveScheduleRevision int64     `json:"active_schedule_revision"`
	Revision               int64     `json:"revision"`
	CreatedAt              time.Time `json:"created_at"`
}

// ListTasks 任务列表。
func (r *Repo) ListTasks(ctx context.Context, tenantID string, limit, offset int) ([]TaskView, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, state, owner, COALESCE(active_plan_revision,0), COALESCE(active_schedule_revision,0), revision, created_at
		FROM analysis_task_definitions
		WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskView
	for rows.Next() {
		var v TaskView
		if err := rows.Scan(&v.ID, &v.Name, &v.State, &v.Owner, &v.ActivePlanRevision, &v.ActiveScheduleRevision, &v.Revision, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetRunByID 按 run_id 全租户回源(仅遗留事件富化回退用;正常路径走 GetRun 租户过滤)。
func (r *Repo) GetRunByID(ctx context.Context, runID string) (*RunView, error) {
	var v RunView
	var ws, we sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT tenant_id, id, task_id, execution_spec_sha256, state, completeness, integrity_state,
			finding_conclusion, risk_severity,
			(EXTRACT(EPOCH FROM window_start)*1000)::bigint, (EXTRACT(EPOCH FROM window_end)*1000)::bigint,
			revision, created_at, finalized_at
		FROM analysis_runs WHERE id=$1`, runID).Scan(&v.TenantID, &v.RunID, &v.TaskID, &v.ExecutionSpecSHA256, &v.State,
		&v.Completeness, &v.IntegrityState, &v.FindingConclusion, &v.RiskSeverity,
		&ws, &we, &v.Revision, &v.CreatedAt, &v.FinalizedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run not found")
	}
	if err != nil {
		return nil, err
	}
	if ws.Valid {
		v.WindowStartMs = ws.Int64
	}
	if we.Valid {
		v.WindowEndMs = we.Int64
	}
	return &v, nil
}

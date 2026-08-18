// Package repository §20 页面到 API 唯一映射的权威事务补齐:
// 任务定义权威(Create/Activate/Suspend CAS+审计)、整 Run 重试(同 task 新 run)、
// 报告重试、下载票、调度触发历史投影、资源视图、阶段回执投影。
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

// ---------------------------------------------------------------------------
// 任务定义权威(§76.45.1 同型 CAS:保存不联动计划;激活/挂起走 expected revision)
// ---------------------------------------------------------------------------

// CreateTaskDefinitionAtomic 保存任务定义(幂等台账 + DRAFT rev1 + history)。
// 保存定义绝不创建 Plan/激活(§10.1)。
func (r *Repo) CreateTaskDefinitionAtomic(ctx context.Context, tenantID, name, owner, defaultClass, idempotencyKey, requestSHA string) (defID string, replayed bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, idempotencyKey); err != nil {
		return "", false, fmt.Errorf("definition lock: %w", err)
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM analysis_materialization_ledger WHERE identity_hash=$1`, idempotencyKey).Scan(&existing)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO analysis_materialization_ledger(identity_hash, request_sha256) VALUES($1,$2)`, idempotencyKey, requestSHA); err != nil {
			return "", false, fmt.Errorf("definition ledger: %w", err)
		}
	case err != nil:
		return "", false, fmt.Errorf("definition ledger query: %w", err)
	case existing == requestSHA:
		var id string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM analysis_task_definitions WHERE tenant_id=$1 AND name=$2`, tenantID, name).Scan(&id); err != nil {
			return "", false, fmt.Errorf("definition replay lookup: %w", err)
		}
		_ = tx.Commit()
		return id, true, nil
	default:
		return "", false, ErrPayloadMismatch
	}

	if defaultClass == "" {
		defaultClass = "BASELINE"
	}
	defID = uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_task_definitions(id, tenant_id, name, state, owner, default_scheduling_class, created_by)
		VALUES($1,$2,$3,'DRAFT',$4,$5,$4)`, defID, tenantID, name, owner, defaultClass); err != nil {
		return "", false, fmt.Errorf("insert definition: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'task_definition',$2::uuid,'CREATED',$3,jsonb_build_object('name',$4::text))`,
		tenantID, defID, owner, name); err != nil {
		return "", false, fmt.Errorf("definition history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return defID, false, nil
}

// ActivateTaskDefinitionAtomic CAS DRAFT→ACTIVE(expected revision)+ 审计。
func (r *Repo) ActivateTaskDefinitionAtomic(ctx context.Context, tenantID, defID string, expectedRevision int64, actor string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		UPDATE analysis_task_definitions SET state='ACTIVE', revision=revision+1, updated_at=now()
		WHERE tenant_id=$1 AND id=$2 AND state='DRAFT' AND revision=$3`, tenantID, defID, expectedRevision)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return 0, fmt.Errorf("definition activation CAS failed (expected DRAFT@%d)", expectedRevision)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'task_definition',$2::uuid,'ACTIVATED',$3,jsonb_build_object('authority_revision',$4::bigint))`,
		tenantID, defID, actor, expectedRevision+1); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return expectedRevision + 1, nil
}

// SuspendTaskDefinitionAtomic CAS ACTIVE→SUSPENDED(expected revision)+ 审计。
func (r *Repo) SuspendTaskDefinitionAtomic(ctx context.Context, tenantID, defID string, expectedRevision int64, actor string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		UPDATE analysis_task_definitions SET state='SUSPENDED', revision=revision+1, updated_at=now()
		WHERE tenant_id=$1 AND id=$2 AND state='ACTIVE' AND revision=$3`, tenantID, defID, expectedRevision)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return 0, fmt.Errorf("definition suspension CAS failed (expected ACTIVE@%d)", expectedRevision)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'task_definition',$2::uuid,'SUSPENDED',$3,jsonb_build_object('authority_revision',$4::bigint))`,
		tenantID, defID, actor, expectedRevision+1); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return expectedRevision + 1, nil
}

// AuditRecordView 审计记录投影(任务定义详情五 Tab 之"审计记录")。
type AuditRecordView struct {
	Action    string    `json:"action"`
	Actor     string    `json:"actor"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

// TaskDefinitionDetail 任务定义详情(五 Tab 数据源)。
type TaskDefinitionDetail struct {
	ID                     string                  `json:"id"`
	Name                   string                  `json:"name"`
	State                  string                  `json:"state"`
	Owner                  string                  `json:"owner"`
	Revision               int64                   `json:"revision"`
	DefaultClass           string                  `json:"default_scheduling_class"`
	ActivePlanRevision     int64                   `json:"active_plan_revision"`
	ActiveScheduleRevision int64                   `json:"active_schedule_revision"`
	Plans                  []PlanRevisionView      `json:"plans"`
	Schedules              []ScheduleView          `json:"schedules"`
	ReportPolicies         []HumanReportPolicyView `json:"report_policies"`
	AuditRecords           []AuditRecordView       `json:"audit_records"`
}

// TaskDefinitionView 任务定义列表投影(任务管理页 7 列)。
type TaskDefinitionView struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	State                  string    `json:"state"`
	Owner                  string    `json:"owner"`
	DefaultSchedulingClass string    `json:"default_scheduling_class"`
	Revision               int64     `json:"revision"`
	ActivePlanRevision     *int64    `json:"active_plan_revision"`
	ActiveScheduleRevision *int64    `json:"active_schedule_revision"`
	ReportPolicyRevision   *int64    `json:"report_policy_revision"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// ListTaskDefinitions 任务定义列表(权威行只读投影)。
func (r *Repo) ListTaskDefinitions(ctx context.Context, tenantID string) ([]TaskDefinitionView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, state, owner, default_scheduling_class, revision,
			active_plan_revision, active_schedule_revision, report_policy_revision, created_at, updated_at
		FROM analysis_task_definitions WHERE tenant_id=$1
		ORDER BY created_at DESC LIMIT 200`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskDefinitionView
	for rows.Next() {
		var v TaskDefinitionView
		if err := rows.Scan(&v.ID, &v.Name, &v.State, &v.Owner, &v.DefaultSchedulingClass, &v.Revision,
			&v.ActivePlanRevision, &v.ActiveScheduleRevision, &v.ReportPolicyRevision, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// PlanRevisionView 计划修订投影。
type PlanRevisionView struct {
	PlanID              string    `json:"plan_id"`
	PlanRevision        int64     `json:"plan_revision"`
	PlanSource          string    `json:"plan_source"`
	SourceKind          string    `json:"source_kind"`
	ExecutionSpecSHA256 string    `json:"execution_spec_sha256"`
	GovernanceState     string    `json:"governance_state"`
	CreatedAt           time.Time `json:"created_at"`
}

// HumanReportPolicyView 报告策略投影。
type HumanReportPolicyView struct {
	PolicyID         string `json:"policy_id"`
	Revision         int64  `json:"revision"`
	Mode             string `json:"mode"`
	TemplateRevision string `json:"template_revision"`
	Locale           string `json:"locale"`
	RetentionDays    int64  `json:"retention_days"`
	PolicySHA256     string `json:"policy_sha256"`
}

// GetTaskDefinitionDetail 任务定义详情(含方案/调度/报告策略引用)。
func (r *Repo) GetTaskDefinitionDetail(ctx context.Context, tenantID, defID string) (*TaskDefinitionDetail, error) {
	var d TaskDefinitionDetail
	var apr, asr sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `
		SELECT id, name, state, owner, revision, default_scheduling_class,
			active_plan_revision, active_schedule_revision
		FROM analysis_task_definitions WHERE tenant_id=$1 AND id=$2`, tenantID, defID).Scan(
		&d.ID, &d.Name, &d.State, &d.Owner, &d.Revision, &d.DefaultClass, &apr, &asr); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("definition not found")
		}
		return nil, err
	}
	d.ActivePlanRevision, d.ActiveScheduleRevision = apr.Int64, asr.Int64

	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id, p.plan_revision, p.plan_source, p.source_kind, p.execution_spec_sha256,
			COALESCE(g.state,'DRAFT'), p.created_at
		FROM analysis_plan_revisions p
		LEFT JOIN analysis_plan_governance_heads g ON g.plan_id = p.id
		WHERE p.tenant_id=$1 AND p.task_definition_id=$2
		ORDER BY p.plan_revision DESC`, tenantID, defID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p PlanRevisionView
		if err := rows.Scan(&p.PlanID, &p.PlanRevision, &p.PlanSource, &p.SourceKind, &p.ExecutionSpecSHA256, &p.GovernanceState, &p.CreatedAt); err != nil {
			return nil, err
		}
		d.Plans = append(d.Plans, p)
	}

	scheds, err := r.ListSchedules(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	for _, s := range scheds {
		if s.TaskDefinitionID == defID {
			d.Schedules = append(d.Schedules, s)
		}
	}
	pols, err := r.ListHumanReportPolicies(ctx, tenantID, defID)
	if err != nil {
		return nil, err
	}
	d.ReportPolicies = pols
	arows, err := r.db.QueryContext(ctx, `
		SELECT action, actor, detail::text, created_at
		FROM analysis_history
		WHERE tenant_id=$1 AND entity='task_definition' AND entity_id=$2
		ORDER BY created_at DESC LIMIT 200`, tenantID, defID)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var a AuditRecordView
		if err := arows.Scan(&a.Action, &a.Actor, &a.Detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		d.AuditRecords = append(d.AuditRecords, a)
	}
	return &d, nil
}

// ListPlansForDefinition 方案修订列表(任务编排页左列)。
func (r *Repo) ListPlansForDefinition(ctx context.Context, tenantID, defID string) ([]PlanRevisionView, error) {
	d, err := r.GetTaskDefinitionDetail(ctx, tenantID, defID)
	if err != nil {
		return nil, err
	}
	return d.Plans, nil
}

// ---------------------------------------------------------------------------
// 报告策略(§8:独立冻结,不进执行 plan hash)
// ---------------------------------------------------------------------------

// SaveHumanReportPolicyAtomic 保存报告策略修订(幂等台账)。
// 重放返回冻结的原始修订号(修订号自动分配,不能依赖新计算值)。
func (r *Repo) SaveHumanReportPolicyAtomic(ctx context.Context, tenantID, defID, mode, templateRevision, locale string, retentionDays int64, revision int64, policySHA, idempotencyKey, requestSHA string) (policyID string, frozenRevision int64, replayed bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, idempotencyKey); err != nil {
		return "", 0, false, fmt.Errorf("policy lock: %w", err)
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM analysis_materialization_ledger WHERE identity_hash=$1`, idempotencyKey).Scan(&existing)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO analysis_materialization_ledger(identity_hash, request_sha256) VALUES($1,$2)`, idempotencyKey, requestSHA); err != nil {
			return "", 0, false, fmt.Errorf("policy ledger: %w", err)
		}
	case err != nil:
		return "", 0, false, fmt.Errorf("policy ledger query: %w", err)
	case existing == requestSHA:
		// 修订号自动分配,重放按冻结 policy_sha256 回源(返回原始修订号)。
		var id string
		var frozenRev int64
		if err := tx.QueryRowContext(ctx, `
			SELECT id, revision FROM analysis_human_report_policies
			WHERE tenant_id=$1 AND task_definition_id=$2 AND policy_sha256=$3`,
			tenantID, defID, policySHA).Scan(&id, &frozenRev); err != nil {
			return "", 0, false, fmt.Errorf("policy replay lookup: %w", err)
		}
		_ = tx.Commit()
		return id, frozenRev, true, nil
	default:
		return "", 0, false, ErrPayloadMismatch
	}
	policyID = uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_human_report_policies(id, tenant_id, task_definition_id, revision, mode, template_revision, locale, retention_days, policy_sha256)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		policyID, tenantID, defID, revision, mode, templateRevision, locale, retentionDays, policySHA); err != nil {
		return "", 0, false, fmt.Errorf("insert policy: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", 0, false, err
	}
	return policyID, revision, false, nil
}

// ListHumanReportPolicies 报告策略列表。
func (r *Repo) ListHumanReportPolicies(ctx context.Context, tenantID, defID string) ([]HumanReportPolicyView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, revision, mode, template_revision, locale, retention_days, policy_sha256
		FROM analysis_human_report_policies WHERE tenant_id=$1 AND task_definition_id=$2
		ORDER BY revision DESC`, tenantID, defID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HumanReportPolicyView
	for rows.Next() {
		var p HumanReportPolicyView
		if err := rows.Scan(&p.PolicyID, &p.Revision, &p.Mode, &p.TemplateRevision, &p.Locale, &p.RetentionDays, &p.PolicySHA256); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// NextHumanReportPolicyRevision 定义下下一个报告策略修订号。
func (r *Repo) NextHumanReportPolicyRevision(ctx context.Context, tenantID, defID string) (int64, error) {
	var revision int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(revision),0)+1 FROM analysis_human_report_policies
		WHERE tenant_id=$1 AND task_definition_id=$2`, tenantID, defID).Scan(&revision); err != nil {
		return 0, err
	}
	return revision, nil
}

// ---------------------------------------------------------------------------
// 报告重试与下载票(§10.3:报告状态独立,失败不回退 Run)
// ---------------------------------------------------------------------------

// RetryReportAtomic 失败/取消的报告按原参数重新排队(§10.3:报告状态独立,失败不回退 Run)。
// uq_analysis_human_reports(run_id, summary_sha256, template_revision, locale) 决定一个
// run+参数组合只允许一行报告,故重试为原地 FAILED/CANCELLED→QUEUED(同一报告行);
// 每次重试发独立 outbox 事件(event_id 独立唯一,key=report_id)。
func (r *Repo) RetryReportAtomic(ctx context.Context, tenantID, reportID, idempotencyKey, requestSHA string) (replayed bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	// 台账先裁决:同键重放直接返回(报告可能已被重试推进为 QUEUED)。
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, idempotencyKey); err != nil {
		return false, err
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT request_sha256 FROM analysis_materialization_ledger WHERE identity_hash=$1`, idempotencyKey).Scan(&existing)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO analysis_materialization_ledger(identity_hash, request_sha256) VALUES($1,$2)`, idempotencyKey, requestSHA); err != nil {
			return false, fmt.Errorf("retry ledger: %w", err)
		}
	case err != nil:
		return false, fmt.Errorf("retry ledger query: %w", err)
	case existing == requestSHA:
		_ = tx.Commit()
		return true, nil
	default:
		return false, ErrPayloadMismatch
	}

	var oldState, runID, summarySHA, templateRevision, locale string
	if err := tx.QueryRowContext(ctx, `
		SELECT state, run_id::text, summary_sha256, template_revision, locale
		FROM analysis_human_reports WHERE tenant_id=$1 AND id=$2 FOR UPDATE`,
		tenantID, reportID).Scan(&oldState, &runID, &summarySHA, &templateRevision, &locale); err != nil {
		return false, fmt.Errorf("%s: report not found", contract.ErrCodeNotFound)
	}
	if oldState != "FAILED" && oldState != "CANCELLED" {
		return false, fmt.Errorf("%s: report state %s is not retryable (only FAILED/CANCELLED)", contract.ErrCodeInvalidTransition, oldState)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE analysis_human_reports SET state='QUEUED', object_key=NULL, object_sha256=NULL,
			object_size=NULL, updated_at=now()
		WHERE tenant_id=$1 AND id=$2 AND state IN ('FAILED','CANCELLED')`, tenantID, reportID); err != nil {
		return false, fmt.Errorf("requeue report: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_outbox(event_id, topic, key, payload)
		VALUES($1,$2,$3,jsonb_build_object(
			'report_id',$3::text,'run_id',$4::text,'tenant_id',$5::text,
			'summary_sha256',$6::text,'template_revision',$7::text,'locale',$8::text))`,
		uuid.NewString(), "analysis.report.requests.v1", reportID, runID, tenantID, summarySHA, templateRevision, locale); err != nil {
		return false, fmt.Errorf("retry report outbox: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'human_report',$2::uuid,'RETRIED',$1,jsonb_build_object('source_report_id',$2::text))`,
		tenantID, reportID); err != nil {
		return false, fmt.Errorf("retry history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return false, nil
}

// IssueDownloadTicketAtomic 下载票:仅 AVAILABLE 可签发;短期有效 + 使用审计。
func (r *Repo) IssueDownloadTicketAtomic(ctx context.Context, tenantID, reportID, ticketSHA string, ttl time.Duration, actor string) (ticketID string, expiresAt time.Time, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	if err := tx.QueryRowContext(ctx, `
		SELECT state FROM analysis_human_reports WHERE tenant_id=$1 AND id=$2 FOR UPDATE`,
		tenantID, reportID).Scan(&state); err != nil {
		return "", time.Time{}, fmt.Errorf("%s: report not found", contract.ErrCodeNotFound)
	}
	if state != "AVAILABLE" {
		return "", time.Time{}, fmt.Errorf("%s: download ticket requires AVAILABLE report (state=%s)", contract.ErrCodeInvalidTransition, state)
	}
	ticketID = uuid.NewString()
	expiresAt = time.Now().Add(ttl)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_report_download_tickets(id, tenant_id, report_id, ticket_sha256, expires_at)
		VALUES($1,$2,$3,$4,$5)`, ticketID, tenantID, reportID, ticketSHA, expiresAt); err != nil {
		return "", time.Time{}, fmt.Errorf("insert ticket: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'report_download_ticket',$2::uuid,'ISSUED',$3,jsonb_build_object('report_id',$4::text,'expires_at',$5::text))`,
		tenantID, ticketID, actor, reportID, expiresAt.Format(time.RFC3339)); err != nil {
		return "", time.Time{}, fmt.Errorf("ticket history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", time.Time{}, err
	}
	return ticketID, expiresAt, nil
}

// ---------------------------------------------------------------------------
// 调度触发历史投影 + 资源视图 + 阶段回执投影
// ---------------------------------------------------------------------------

// TriggerHistoryView 调度触发历史(按 schedule_revision 过滤)。
type TriggerHistoryView struct {
	TriggerID          string
	State              string
	TriggerKind        string
	WindowID           string
	MaterializedTaskID string
	CreatedAt          time.Time
}

// ListTriggersForSchedule 调度修订的触发实例历史投影。
func (r *Repo) ListTriggersForSchedule(ctx context.Context, tenantID, scheduleID string) ([]TriggerHistoryView, error) {
	var defID string
	var revision int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT task_definition_id, revision FROM analysis_schedule_revisions
		WHERE tenant_id=$1 AND id=$2`, tenantID, scheduleID).Scan(&defID, &revision); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("schedule not found")
		}
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, state, trigger_kind, COALESCE(window_id,''), COALESCE(materialized_task_id::text,''), created_at
		FROM analysis_trigger_instances
		WHERE tenant_id=$1 AND task_definition_id=$2 AND schedule_revision=$3
		ORDER BY created_at DESC LIMIT 200`, tenantID, defID, revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TriggerHistoryView
	for rows.Next() {
		var v TriggerHistoryView
		if err := rows.Scan(&v.TriggerID, &v.State, &v.TriggerKind, &v.WindowID, &v.MaterializedTaskID, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ResourceViews 调度资源视图(容量配额/队列/租约/执行器四 Tab 数据源)。
type ResourceViews struct {
	Reservations []ReservationView `json:"reservations"`
	Drr          []DrrView         `json:"drr"`
	OutboxLedger []OutboxLedgerRow `json:"outbox_ledger"`
	Queue        []QueueView       `json:"queue"`
}

// ReservationView 准入预留视图。
type ReservationView struct {
	RunID        string    `json:"run_id"`
	ResourcePool string    `json:"resource_pool"`
	State        string    `json:"state"`
	Epoch        int64     `json:"epoch"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// DrrView DRR 公平队列状态视图。
type DrrView struct {
	TenantID        string    `json:"tenant_id"`
	SchedulingClass string    `json:"scheduling_class"`
	Deficit         int64     `json:"deficit"`
	Quantum         int64     `json:"quantum"`
	LastServedAt    time.Time `json:"last_served_at"`
	SchedulerEpoch  int64     `json:"scheduler_epoch"`
}

// QueueView 队列视图(非终态 run 按类别/状态聚合)。
type QueueView struct {
	SchedulingClass string `json:"scheduling_class"`
	State           string `json:"state"`
	Count           int64  `json:"count"`
}

// GetResourceViews 调度资源视图(只读聚合)。
func (r *Repo) GetResourceViews(ctx context.Context, tenantID string) (*ResourceViews, error) {
	v := &ResourceViews{}
	rows, err := r.db.QueryContext(ctx, `
		SELECT run_id::text, resource_pool, state, epoch, expires_at
		FROM analysis_admission_reservations WHERE tenant_id=$1
		ORDER BY expires_at LIMIT 200`, tenantID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var x ReservationView
		if err := rows.Scan(&x.RunID, &x.ResourcePool, &x.State, &x.Epoch, &x.ExpiresAt); err != nil {
			rows.Close()
			return nil, err
		}
		v.Reservations = append(v.Reservations, x)
	}
	rows.Close()

	drows, err := r.db.QueryContext(ctx, `
		SELECT tenant_id, scheduling_class, deficit, quantum, last_served_at, scheduler_epoch
		FROM analysis_drr_state WHERE tenant_id=$1`, tenantID)
	if err != nil {
		return nil, err
	}
	for drows.Next() {
		var x DrrView
		if err := drows.Scan(&x.TenantID, &x.SchedulingClass, &x.Deficit, &x.Quantum, &x.LastServedAt, &x.SchedulerEpoch); err != nil {
			drows.Close()
			return nil, err
		}
		v.Drr = append(v.Drr, x)
	}
	drows.Close()

	qrows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(t.effective_class,'BASELINE'), ru.state, count(*)
		FROM analysis_runs ru
		LEFT JOIN analysis_tasks t ON t.id = ru.task_id
		WHERE ru.tenant_id=$1 AND ru.state NOT IN ('SUCCEEDED','PARTIALLY_SUCCEEDED','FAILED','CANCELLED')
		GROUP BY 1,2 ORDER BY 1,2`, tenantID)
	if err != nil {
		return nil, err
	}
	for qrows.Next() {
		var x QueueView
		if err := qrows.Scan(&x.SchedulingClass, &x.State, &x.Count); err != nil {
			qrows.Close()
			return nil, err
		}
		v.Queue = append(v.Queue, x)
	}
	qrows.Close()

	ledger, err := r.OutboxLedger(ctx)
	if err != nil {
		return nil, err
	}
	v.OutboxLedger = ledger
	return v, nil
}

// StageReceiptProjection 阶段回执投影(运行详情"技术详情"页签)。
type StageReceiptProjection struct {
	ExecutionNodeID string          `json:"execution_node_id"`
	Attempt         int32           `json:"attempt"`
	Provider        string          `json:"provider"`
	InputCount      int64           `json:"input_count"`
	OutputCount     int64           `json:"output_count"`
	ErrorCount      int64           `json:"error_count"`
	Fence           json.RawMessage `json:"fence"`
	PayloadHash     string          `json:"payload_hash"`
	ReceivedAt      time.Time       `json:"received_at"`
}

// ListStageReceiptsForRun 阶段回执投影。
func (r *Repo) ListStageReceiptsForRun(ctx context.Context, tenantID, runID string) ([]StageReceiptProjection, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT execution_node_id, attempt, provider, input_count, output_count, error_count,
			fence, payload_hash, received_at
		FROM analysis_stage_receipts WHERE tenant_id=$1 AND run_id=$2
		ORDER BY execution_node_id, attempt`, tenantID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StageReceiptProjection
	for rows.Next() {
		var p StageReceiptProjection
		if err := rows.Scan(&p.ExecutionNodeID, &p.Attempt, &p.Provider, &p.InputCount, &p.OutputCount,
			&p.ErrorCount, &p.Fence, &p.PayloadHash, &p.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// TaskBindingRow run→task 绑定(整 Run 重试输入)。
type TaskBindingRow struct {
	TaskID              string
	TaskDefinitionID    string
	PlanRevision        int64
	ExecutionSpecSHA256 string
	EffectiveClass      string
	EffectivePolicySHA  string
	ScheduleRevision    int64
}

// GetTaskByRunID 读取 run 的 task 绑定(整 Run 重试/查询投影)。
func (r *Repo) GetTaskByRunID(ctx context.Context, runID string) (*TaskBindingRow, error) {
	var b TaskBindingRow
	err := r.db.QueryRowContext(ctx, `
		SELECT t.id, t.task_definition_id, t.plan_revision, t.execution_spec_sha256,
			COALESCE(t.effective_class,'BASELINE'), COALESCE(t.effective_policy_sha256,''),
			COALESCE(t.schedule_revision,0)
		FROM analysis_tasks t JOIN analysis_runs ru ON ru.task_id = t.id
		WHERE ru.id=$1`, runID).Scan(&b.TaskID, &b.TaskDefinitionID, &b.PlanRevision,
		&b.ExecutionSpecSHA256, &b.EffectiveClass, &b.EffectivePolicySHA, &b.ScheduleRevision)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run task binding not found")
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ---------------------------------------------------------------------------
// 下载票消费(§20 DownloadTicket 生命周期:签发→校验→使用一次→审计)
// ---------------------------------------------------------------------------

// ConsumeDownloadTicketAtomic 校验并消费下载票(报告 AVAILABLE + 票未过期未使用 +
// 票归属该报告),成功返回对象键并置 used_at(一次性)。校验失败按契约码拒绝。
func (r *Repo) ConsumeDownloadTicketAtomic(ctx context.Context, tenantID, reportID, ticketID string) (objectKey string, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var usedAt sql.NullTime
	var expiresAt time.Time
	var ownerReport string
	var key sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT t.report_id, t.expires_at, t.used_at, r.object_key
		FROM analysis_report_download_tickets t
		JOIN analysis_human_reports r ON r.id = t.report_id
		WHERE t.id=$1 AND t.tenant_id=$2 FOR UPDATE OF t`,
		ticketID, tenantID).Scan(&ownerReport, &expiresAt, &usedAt, &key); err != nil {
		return "", fmt.Errorf("%s: download ticket not found", contract.ErrCodeNotFound)
	}
	if ownerReport != reportID {
		return "", fmt.Errorf("%s: download ticket does not belong to report", contract.ErrCodeInvalidTransition)
	}
	if usedAt.Valid {
		return "", fmt.Errorf("%s: download ticket already used", contract.ErrCodeInvalidTransition)
	}
	if time.Now().After(expiresAt) {
		return "", fmt.Errorf("%s: download ticket expired", contract.ErrCodeInvalidTransition)
	}
	if !key.Valid || key.String == "" {
		return "", fmt.Errorf("%s: report object not ready", contract.ErrCodeInvalidTransition)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE analysis_report_download_tickets SET used_at=now()
		WHERE id=$1 AND used_at IS NULL`, ticketID); err != nil {
		return "", fmt.Errorf("consume ticket: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'report_download_ticket',$2::uuid,'USED',$1,
			jsonb_build_object('report_id',$3::text,'object_key',$4::text))`,
		tenantID, ticketID, reportID, key.String); err != nil {
		return "", fmt.Errorf("ticket use history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return key.String, nil
}

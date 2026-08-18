// 统一分析任务调度中心 API 客户端(P6 前端执行链)。
// 约定:API 统一封装、React Query 处理 loading/error、query key 全参数化。
import { api } from '@/services/httpClient';

export type AnalysisRunState =
  | 'ACCEPTED' | 'PREPARING' | 'QUEUED' | 'RUNNING' | 'FINALIZING'
  | 'SUCCEEDED' | 'PARTIALLY_SUCCEEDED' | 'FAILED' | 'CANCEL_REQUESTED' | 'CANCELLED';

export type AnalysisRunView = {
  run_id: string;
  task_id: string;
  state: AnalysisRunState;
  completeness: string;
  integrity_state: string;
  finding_conclusion: string;
  risk_severity: string;
  report_state: string;
  execution_spec_sha256: string;
  window_start_ms: number;
  window_end_ms: number;
  revision: number;
  stages?: AnalysisStageView[];
};

export type AnalysisStageView = {
  business_phase_id: string;
  execution_node_id: string;
  attempt: number;
  state: string;
  provider_mode: string;
  activation_mode: string;
  skip_reason: string;
};

export type AnalysisTaskView = {
  id: string;
  name: string;
  state: string;
  owner: string;
  active_plan_revision: number;
  active_schedule_revision: number;
  revision: number;
  created_at: string;
};

export type AnalysisTriggerOverrides = {
  source_kind?: string;
  source_spec?: Record<string, unknown>;
  probe_id?: string;
  selected_feature_ids?: string[];
  encrypted_recognition_model_ref?: string;
  threat_detector_refs?: string[];
  rule_refs?: string[];
  thresholds?: Record<string, number>;
};

export type SubmitTriggerInput = {
  task_definition_id: string;
  plan_source: 'AUTO_DEFAULT' | 'MANUAL_CUSTOM';
  custom_overrides?: AnalysisTriggerOverrides;
  source_kind?: string;
  source_spec?: Record<string, unknown>;
  client_idempotency_key: string;
};

export type SubmitTriggerResult = {
  task_id: string;
  run_id: string;
  status_url: string;
  execution_spec_sha256: string;
};

export type SaveCustomPlanResult = {
  plan_id: string;
  plan_revision: number;
  execution_spec_sha256: string;
  plan_revision_sha256: string;
};

export type ApprovePlanResult = {
  plan_id: string;
  state: string;
};

// —— 查询函数(React Query) ——

export const fetchAnalysisTasks = async () => {
  const response = await api.get<{ success: boolean; data: AnalysisTaskView[] }>('/v1/analysis/tasks');
  return response.data.data ?? [];
};

export const fetchAnalysisRuns = async (state?: string) => {
  const response = await api.get<{ success: boolean; data: AnalysisRunView[] }>('/v1/analysis/runs', {
    params: state ? { state } : {},
  });
  return response.data.data ?? [];
};

export const fetchAnalysisRun = async (runId: string) => {
  const response = await api.get<{ success: boolean; data: AnalysisRunView }>(`/v1/analysis/runs/${runId}`);
  return response.data.data;
};

// —— 变更函数(带幂等键;transport timeout 复用原 key 查 receipt) ——

export const submitAnalysisTrigger = async (input: SubmitTriggerInput) => {
  const response = await api.post<{ success: boolean; data: SubmitTriggerResult }>(
    '/v1/analysis/triggers',
    input,
    { headers: { 'Idempotency-Key': input.client_idempotency_key } },
  );
  return response.data.data;
};

/** P2 人工选择列车:保存定制计划草稿(DRAFT 治理头;同 key 幂等回源)。 */
export const saveAnalysisCustomPlan = async (
  taskDefinitionId: string,
  customOverrides: AnalysisTriggerOverrides,
  idempotencyKey: string,
) => {
  const response = await api.post<{ success: boolean; data: SaveCustomPlanResult }>(
    `/v1/analysis/tasks/${encodeURIComponent(taskDefinitionId)}/plans`,
    { custom_overrides: customOverrides, client_idempotency_key: idempotencyKey },
    { headers: { 'Idempotency-Key': idempotencyKey } },
  );
  return response.data.data;
};

export type SavePlanInput = {
  task_definition_id: string;
  plan_source: 'AUTO_DEFAULT' | 'MANUAL_CUSTOM';
  custom_overrides?: Record<string, unknown>;
  client_idempotency_key: string;
};

/** §20 任务编排:保存计划修订(AUTO_DEFAULT=默认计划;MANUAL_CUSTOM=定制计划,需覆盖项)。 */
export const saveAnalysisPlan = async (input: SavePlanInput) => {
  const response = await api.post<{ success: boolean; data: { plan_id: string; plan_revision: number; execution_spec_sha256: string } }>(
    `/v1/analysis/tasks/${encodeURIComponent(input.task_definition_id)}/plans`,
    {
      plan_source: input.plan_source,
      custom_overrides: input.custom_overrides ?? undefined,
      client_idempotency_key: input.client_idempotency_key,
    },
    { headers: { 'Idempotency-Key': input.client_idempotency_key } },
  );
  return response.data.data;
};

/** §20 任务编排:预检(只解析+编译,不物化;返回冻结 sha 与兼容性)。 */
export const preflightAnalysisPlan = async (input: SavePlanInput) => {
  const response = await api.post<{
    success: boolean;
    data: { execution_spec_sha256: string; canonical_spec: Record<string, unknown>; source_kind: string; compatible: boolean };
  }>(`/v1/analysis/triggers/preflight`, {
    task_definition_id: input.task_definition_id,
    plan_source: input.plan_source,
    custom_overrides: input.custom_overrides ?? undefined,
  });
  return response.data.data;
};

/** P2 人工选择列车:maker/checker 审批并激活草稿。 */
export const approveAnalysisPlan = async (planId: string, maker: string, checker: string) => {
  const response = await api.post<{ success: boolean; data: ApprovePlanResult }>(
    `/v1/analysis/plans/${encodeURIComponent(planId)}/approve`,
    { maker, checker },
  );
  return response.data.data;
};

export const cancelAnalysisRun = async (runId: string, idempotencyKey: string) => {
  const response = await api.post<{ success: boolean }>(`/v1/analysis/runs/${encodeURIComponent(runId)}:cancel`, undefined, {
    headers: { 'Idempotency-Key': idempotencyKey },
  });
  return response.data;
};

export const requestAnalysisReport = async (runId: string) => {
  const response = await api.post<{ success: boolean; data: { report_id?: string; state?: string } }>(
    `/v1/analysis/runs/${encodeURIComponent(runId)}/report`,
  );
  return response.data.data;
};

// —— 调度修订权威(§76.45.1) ——
export type AnalysisScheduleView = {
  ScheduleID: string;
  TaskDefinitionID: string;
  Revision: number;
  ApprovedPlanRevision: number;
  ExecutionSpecSHA256: string;
  TriggerKind: string;
  WindowOrCron: string;
  MisfirePolicy: string;
  ConcurrencyPolicy: string;
  SchedulingClass: string;
  HeadState: string;
  AuthorityRevision: number;
  CreatedAt: string;
};

export const fetchAnalysisSchedules = async () => {
  const response = await api.get<{ success: boolean; data: AnalysisScheduleView[] }>('/v1/analysis/schedules');
  return response.data.data ?? [];
};

export type SaveScheduleInput = {
  task_definition_id: string;
  approved_plan_revision: number;
  execution_spec_sha256: string;
  trigger_kind: string;
  timezone: string;
  window_or_cron: Record<string, unknown>;
  prepare_lead_time_ms: number;
  misfire_policy: string;
  concurrency_policy: string;
  scheduling_class: string;
  resource_restrictions: Record<string, unknown>;
  client_idempotency_key: string;
};

export const saveAnalysisSchedule = async (input: SaveScheduleInput) => {
  const response = await api.post<{ success: boolean; data: { ScheduleID: string; Revision: number; ScheduleSHA256: string } }>(
    '/v1/analysis/schedules', input, { headers: { 'Idempotency-Key': input.client_idempotency_key } });
  return response.data.data;
};

export const activateAnalysisSchedule = async (scheduleId: string, expectedRevision: number, actor: string) => {
  const response = await api.post<{ success: boolean; data: { authority_revision: number } }>(
    `/v1/analysis/schedules/${encodeURIComponent(scheduleId)}/activate`,
    { expected_revision: expectedRevision, actor });
  return response.data.data;
};

export const pauseAnalysisSchedule = async (scheduleId: string, expectedRevision: number, actor: string) => {
  const response = await api.post<{ success: boolean; data: { authority_revision: number } }>(
    `/v1/analysis/schedules/${encodeURIComponent(scheduleId)}/pause`,
    { expected_revision: expectedRevision, actor });
  return response.data.data;
};

// —— 人读报告(§10.3) ——
export type AnalysisReportView = {
  ReportID: string;
  RunID: string;
  SummarySHA256: string;
  TemplateRevision: string;
  Locale: string;
  State: string;
  ObjectKey: string;
  ObjectSHA256: string;
  ObjectSize: number;
  CreatedAt: string;
};

export const fetchAnalysisReports = async () => {
  const response = await api.get<{ success: boolean; data: AnalysisReportView[] }>('/v1/analysis/reports');
  return response.data.data ?? [];
};

// —— 节点级重试(§76.47.3) ——
export const retryAnalysisStage = async (runId: string, executionNodeId: string, actor: string) => {
  const response = await api.post<{ success: boolean; data: { NewAttemptID: string; Attempt: number } }>(
    `/v1/analysis/runs/${encodeURIComponent(runId)}/retry-stage`,
    { execution_node_id: executionNodeId, actor });
  return response.data.data;
};

// —— 调度资源视图(§20:容量配额/队列/租约/执行器;后端 /resources 已发布) ——
export type AnalysisReservationView = {
  run_id: string;
  resource_pool: string;
  state: string;
  epoch: number;
  expires_at: string;
};

export type AnalysisDrrView = {
  tenant_id: string;
  scheduling_class: string;
  deficit: number;
  quantum: number;
  last_served_at: string;
  scheduler_epoch: number;
};

export type AnalysisOutboxLedgerRow = {
  topic: string;
  state: string;
  count: number;
  oldest_pending_age_seconds: number;
};

export type AnalysisQueueView = {
  scheduling_class: string;
  state: string;
  count: number;
};

export type AnalysisResourceViews = {
  reservations: AnalysisReservationView[];
  drr: AnalysisDrrView[];
  outbox_ledger: AnalysisOutboxLedgerRow[];
  queue: AnalysisQueueView[];
};

export const fetchAnalysisResources = async () => {
  const response = await api.get<{ success: boolean; data: AnalysisResourceViews }>('/v1/analysis/resources');
  return response.data.data;
};

// —— allowedActions 服务端驱动(§20/§21:前端只渲染,不自行判定) ——
export type RunAllowedActions = {
  run_id: string;
  state: string;
  cancel: boolean;
  retry_stage: boolean;
  retry_task: boolean;
  request_report: boolean;
};

export const fetchRunAllowedActions = async (runId: string) => {
  const response = await api.get<{ success: boolean; data: RunAllowedActions }>(
    `/v1/analysis/runs/${encodeURIComponent(runId)}/allowed-actions`);
  return response.data.data;
};

export const retryAnalysisTask = async (runId: string, clientIdempotencyKey: string, actor: string) => {
  const response = await api.post<{ success: boolean; data: { task_id: string; run_id: string; status_url: string } }>(
    `/v1/analysis/runs/${encodeURIComponent(runId)}/retry-task`,
    { client_idempotency_key: clientIdempotencyKey, actor });
  return response.data.data;
};

// —— 任务定义权威(§20 任务管理:列表 + 详情五 Tab) ——
export type AnalysisTaskDefinitionView = {
  id: string;
  name: string;
  state: string;
  owner: string;
  default_scheduling_class: string;
  revision: number;
  active_plan_revision: number | null;
  active_schedule_revision: number | null;
  report_policy_revision: number | null;
  created_at: string;
  updated_at: string;
};

export type AnalysisPlanRevisionView = {
  plan_id: string;
  plan_revision: number;
  plan_source: string;
  source_kind: string;
  execution_spec_sha256: string;
  governance_state: string;
  created_at: string;
};

export type AnalysisReportPolicyView = {
  policy_id: string;
  revision: number;
  mode: string;
  template_revision: string;
  locale: string;
  retention_days: number;
  policy_sha256: string;
};

export type AnalysisAuditRecordView = {
  action: string;
  actor: string;
  detail: string;
  created_at: string;
};

export type AnalysisTaskDefinitionDetail = {
  id: string;
  name: string;
  state: string;
  owner: string;
  revision: number;
  default_scheduling_class: string;
  active_plan_revision: number | null;
  active_schedule_revision: number | null;
  plans: AnalysisPlanRevisionView[];
  schedules: AnalysisScheduleView[];
  report_policies: AnalysisReportPolicyView[];
  audit_records: AnalysisAuditRecordView[];
};

export type DefinitionAllowedActions = {
  task_definition_id: string;
  state: string;
  revision: number;
  activate: boolean;
  suspend: boolean;
};

export const fetchTaskDefinitionDetail = async (taskDefinitionId: string) => {
  const response = await api.get<{ success: boolean; data: AnalysisTaskDefinitionDetail }>(
    `/v1/analysis/task-definitions/${encodeURIComponent(taskDefinitionId)}`);
  return response.data.data;
};

export const fetchDefinitionAllowedActions = async (taskDefinitionId: string) => {
  const response = await api.get<{ success: boolean; data: DefinitionAllowedActions }>(
    `/v1/analysis/task-definitions/${encodeURIComponent(taskDefinitionId)}/allowed-actions`);
  return response.data.data;
};

export const activateTaskDefinition = async (taskDefinitionId: string, expectedRevision: number, actor: string) => {
  const response = await api.post<{ success: boolean; data: { authority_revision: number } }>(
    `/v1/analysis/task-definitions/${encodeURIComponent(taskDefinitionId)}/activate`,
    { expected_revision: expectedRevision, actor });
  return response.data.data;
};

export const suspendTaskDefinition = async (taskDefinitionId: string, expectedRevision: number, actor: string) => {
  const response = await api.post<{ success: boolean; data: { authority_revision: number } }>(
    `/v1/analysis/task-definitions/${encodeURIComponent(taskDefinitionId)}/suspend`,
    { expected_revision: expectedRevision, actor });
  return response.data.data;
};

export const saveTaskDefinitionReportPolicy = async (
  taskDefinitionId: string,
  input: { mode: string; template_revision: string; locale: string; retention_days: number; client_idempotency_key: string },
) => {
  const response = await api.post<{ success: boolean; data: { policy_id: string; revision: number } }>(
    `/v1/analysis/task-definitions/${encodeURIComponent(taskDefinitionId)}/report-policies`, input);
  return response.data.data;
};

/** 面向用户的错误文案:原始 axios 信息(如 "Request failed with status code 401")不上屏。 */
export const friendlyAnalysisError = (e: unknown): string => {
  const status = (e as { response?: { status?: number } } | null)?.response?.status;
  if (status === 401) return '会话未认证或已过期,请重新登录';
  if (status === 403) return '无权限执行该操作';
  if (status === 409) return '并发冲突或状态不允许该操作,请刷新后重试';
  if (status !== undefined && status >= 500) return '调度服务暂不可用,请稍后重试';
  return e instanceof Error ? e.message : '请求失败';
};

// query key 工厂(tenant/session epoch 全参数,由 auth 层在 key 前缀补齐)
export const analysisQueryKeys = {
  tasks: ['analysis', 'tasks'] as const,
  runs: (state?: string) => ['analysis', 'runs', { state }] as const,
  run: (runId: string) => ['analysis', 'run', runId] as const,
  schedules: ['analysis', 'schedules'] as const,
  reports: ['analysis', 'reports'] as const,
};

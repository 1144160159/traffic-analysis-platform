import { api } from '@/services/api';
import { getPageActionPlan } from '@/services/pageApiPlans';

export type DashboardTaskActionId =
  | 'dashboard-task-create'
  | 'dashboard-evidence-task-create'
  | 'dashboard-feedback-task-create'
  | 'dashboard-audit-task-create'
  | 'dashboard-sla-task-create'
  | 'dashboard-compliance-task-create';

export type DashboardTaskStatus = 'accepted' | 'running' | 'completed' | 'partial' | 'failed' | 'cancelled';

export type DashboardTaskReceipt = {
  taskId: string;
  jobId: string;
  eventId: string;
  actionId: DashboardTaskActionId;
  status: DashboardTaskStatus;
  revision: number;
  snapshotId: string;
  traceId: string;
  idempotencyKey: string;
  outboxStatus: string;
  replayed: boolean;
};

export type DashboardTask = DashboardTaskReceipt & {
  taskType: string;
  target: string;
  priority: string;
  reason: string;
  requestedBy: string;
  context: Record<string, unknown>;
  result: Record<string, unknown>;
  errorCode?: string;
  errorMessage?: string;
  createdAt: string;
  updatedAt: string;
};

export type SubmitDashboardTaskInput = {
  actionId: DashboardTaskActionId;
  target: string;
  priority: 'low' | 'medium' | 'high' | 'critical';
  snapshotId: string;
  reason: string;
  context?: Record<string, unknown>;
};

type Envelope<T> = { data?: T } | T;

export async function submitDashboardTask(input: SubmitDashboardTaskInput): Promise<DashboardTaskReceipt> {
  const plan = getPageActionPlan('dashboard', input.actionId);
  if (!plan) throw new Error(`未找到仪表盘动作契约：${input.actionId}`);
  const target = input.target.trim();
  const snapshotId = input.snapshotId.trim();
  const reason = input.reason.trim();
  if (!target || !snapshotId || !reason) throw new Error('任务目标、页面数据版本和提交原因不能为空');
  const idempotencyKey = dashboardTaskIdempotencyKey(input.actionId, snapshotId, target, reason);
  const response = await api.post<Envelope<Record<string, unknown>>>(plan.endpoint, {
    target,
    priority: input.priority,
    snapshot_id: snapshotId,
    reason,
    context: input.context ?? {},
  }, {
    headers: {
      'X-Action-ID': input.actionId,
      'Idempotency-Key': idempotencyKey,
    },
  });
  return normalizeDashboardTaskReceipt(unwrap(response.data), idempotencyKey);
}

export async function fetchDashboardTask(taskId: string): Promise<DashboardTask> {
  const normalizedID = taskId.trim();
  if (!normalizedID) throw new Error('任务ID不能为空');
  const response = await api.get<Envelope<Record<string, unknown>>>(`/v1/dashboard/tasks/${encodeURIComponent(normalizedID)}`);
  const payload = unwrap(response.data);
  const receipt = normalizeDashboardTaskReceipt(payload, '');
  return {
    ...receipt,
    taskType: text(payload.task_type),
    target: text(payload.target),
    priority: text(payload.priority),
    reason: text(payload.reason),
    requestedBy: text(payload.requested_by),
    context: record(payload.context),
    result: record(payload.result),
    errorCode: text(payload.error_code) || undefined,
    errorMessage: text(payload.error_message) || undefined,
    createdAt: text(payload.created_at),
    updatedAt: text(payload.updated_at),
  };
}

function dashboardTaskRequestFingerprint(input: unknown): string {
  const serialized = JSON.stringify(input ?? {});
  let hash = 2166136261;
  for (let index = 0; index < serialized.length; index += 1) {
    hash ^= serialized.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return `request-fingerprint:${(hash >>> 0).toString(16).padStart(8, '0')}`;
}

function dashboardTaskIdempotencyKey(actionId: string, snapshotId: string, target: string, reason: string): string {
  return `dashboard:${actionId}:${dashboardTaskRequestFingerprint({ snapshotId, target, reason })}`;
}

function normalizeDashboardTaskReceipt(payload: Record<string, unknown>, fallbackIdempotencyKey: string): DashboardTaskReceipt {
  const taskId = text(payload.task_id);
  const jobId = text(payload.job_id) || taskId;
  const actionId = text(payload.action_id) as DashboardTaskActionId;
  const status = normalizeStatus(payload.status);
  if (!taskId || !jobId || !actionId || !text(payload.trace_id) || !text(payload.snapshot_id)) {
    throw new Error('仪表盘任务响应缺少稳定 task_id、action_id、snapshot_id 或 trace_id');
  }
  return {
    taskId,
    jobId,
    eventId: text(payload.event_id),
    actionId,
    status,
    revision: Number(payload.revision ?? 0),
    snapshotId: text(payload.snapshot_id),
    traceId: text(payload.trace_id),
    idempotencyKey: text(payload.idempotency_key) || fallbackIdempotencyKey,
    outboxStatus: text(payload.outbox_status),
    replayed: Boolean(payload.replayed),
  };
}

function unwrap(value: Envelope<Record<string, unknown>>): Record<string, unknown> {
  const candidate = value as { data?: Record<string, unknown> };
  return record(candidate.data ?? value);
}

function normalizeStatus(value: unknown): DashboardTaskStatus {
  const status = text(value) as DashboardTaskStatus;
  return ['accepted', 'running', 'completed', 'partial', 'failed', 'cancelled'].includes(status) ? status : 'accepted';
}

const text = (value: unknown) => typeof value === 'string' ? value.trim() : '';
const record = (value: unknown): Record<string, unknown> => value && typeof value === 'object' && !Array.isArray(value)
  ? value as Record<string, unknown>
  : {};

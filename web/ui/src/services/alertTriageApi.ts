import { api } from '@/services/api';

export type AlertTriageActionKind = 'saved-view' | 'response-action' | 'investigation-note';

export type AlertTriageActionInput = {
  kind: AlertTriageActionKind;
  actionId: string;
  alertId?: string;
  action: string;
  target: string;
  reason: string;
  dryRun?: boolean;
  expectedRevision?: number;
  idempotencyKey?: string;
  detail?: Record<string, unknown>;
};

export type AlertTriageActionResult = {
  job_id?: string;
  view_id?: string;
  status?: 'recorded' | 'accepted' | 'pending_approval' | 'approved_awaiting_executor' | 'blocked_external_executor' | 'cancelled' | 'compensation_blocked_external_executor';
  approval_status?: 'not_required' | 'pending' | 'approved' | 'rejected' | 'cancelled';
  outbox_status?: 'not_required' | 'awaiting_approval' | 'pending_retry' | 'published' | 'cancelled' | 'not_enqueued';
  revision?: number;
  idempotent_reuse?: boolean;
  action: string;
  target: string;
  dry_run: boolean;
  audit_event: string;
};

const responseIdempotencyKeys = new Map<string, { key: string; expiresAt: number }>();

export async function submitAlertTriageAction(input: AlertTriageActionInput): Promise<AlertTriageActionResult> {
  const alertId = input.alertId?.trim();
  const endpoint = input.kind === 'saved-view'
    ? '/v1/alerts/views'
    : `/v1/alerts/${encodeURIComponent(alertId || '')}/${input.kind === 'response-action' ? 'response-actions' : 'investigation-notes'}`;
  if (input.kind !== 'saved-view' && !alertId) throw new Error('请选择告警后再提交操作');
  const dryRun = input.dryRun ?? input.kind === 'response-action';
  const expectedRevision = input.expectedRevision ?? 0;
  const requestBody = {
    action_id: input.actionId,
    action: input.action,
    target: input.target,
    reason: input.reason,
    dry_run: dryRun,
    ...(input.kind === 'response-action' ? { expected_revision: expectedRevision } : {}),
    detail: input.detail,
  };
  const requestConfig = input.kind === 'response-action'
    ? { headers: { 'Idempotency-Key': input.idempotencyKey ?? responseActionIdempotencyKey(input, dryRun, expectedRevision) } }
    : input.kind === 'saved-view'
      ? { headers: { 'Idempotency-Key': input.idempotencyKey ?? savedViewIdempotencyKey(input) } }
      : undefined;
  const response = await api.post<{ data?: AlertTriageActionResult } & Partial<AlertTriageActionResult>>(endpoint, requestBody, requestConfig);
  const payload = response.data.data ?? response.data;
  if (!payload.job_id && !payload.view_id) throw new Error('告警操作未返回持久化记录编号');
  return payload as AlertTriageActionResult;
}

export type AlertResponseWorkflowResult = {
  job_id: string;
  status: string;
  approval_status?: string;
  revision: number;
  idempotent_reuse: boolean;
  outbox_status: string;
};

export async function decideAlertResponseAction(
  alertId: string,
  jobId: string,
  input: { decision: 'approve' | 'reject'; expectedRevision: number; reason: string; idempotencyKey: string },
): Promise<AlertResponseWorkflowResult> {
  const response = await api.post(`/v1/alerts/${encodeURIComponent(alertId)}/response-actions/${encodeURIComponent(jobId)}/approval`, {
    decision: input.decision,
    expected_revision: input.expectedRevision,
    reason: input.reason,
  }, { headers: { 'Idempotency-Key': input.idempotencyKey } });
  return (response.data?.data ?? response.data) as AlertResponseWorkflowResult;
}

export async function cancelAlertResponseAction(
  alertId: string,
  jobId: string,
  input: { expectedRevision: number; reason: string; idempotencyKey: string },
): Promise<AlertResponseWorkflowResult> {
  const response = await api.post(`/v1/alerts/${encodeURIComponent(alertId)}/response-actions/${encodeURIComponent(jobId)}/cancel`, {
    expected_revision: input.expectedRevision,
    reason: input.reason,
  }, { headers: { 'Idempotency-Key': input.idempotencyKey } });
  return (response.data?.data ?? response.data) as AlertResponseWorkflowResult;
}

export async function requestAlertResponseCompensation(
  alertId: string,
  jobId: string,
  input: { expectedRevision: number; reason: string; idempotencyKey: string },
): Promise<AlertResponseWorkflowResult> {
  const response = await api.post(`/v1/alerts/${encodeURIComponent(alertId)}/response-actions/${encodeURIComponent(jobId)}/compensations`, {
    expected_revision: input.expectedRevision,
    reason: input.reason,
  }, { headers: { 'Idempotency-Key': input.idempotencyKey } });
  return (response.data?.data ?? response.data) as AlertResponseWorkflowResult;
}

function responseActionIdempotencyKey(input: AlertTriageActionInput, dryRun: boolean, expectedRevision: number): string {
  const now = Date.now();
  for (const [fingerprint, entry] of responseIdempotencyKeys) {
    if (entry.expiresAt <= now) responseIdempotencyKeys.delete(fingerprint);
  }
  const fingerprint = JSON.stringify([
    input.alertId?.trim(), input.actionId, input.action, input.target,
    input.reason, dryRun, expectedRevision, input.detail ?? {},
  ]);
  const existing = responseIdempotencyKeys.get(fingerprint);
  if (existing) return existing.key;
  const randomPart = globalThis.crypto?.randomUUID?.()
    ?? `${now.toString(36)}-${Math.random().toString(36).slice(2)}`;
  const key = `alert-response:${randomPart}`;
  responseIdempotencyKeys.set(fingerprint, { key, expiresAt: now + 5 * 60_000 });
  return key;
}

function savedViewIdempotencyKey(input: AlertTriageActionInput): string {
  const now = Date.now();
  for (const [fingerprint, entry] of responseIdempotencyKeys) {
    if (entry.expiresAt <= now) responseIdempotencyKeys.delete(fingerprint);
  }
  const fingerprint = `saved-view:${JSON.stringify([
    input.actionId, input.target, input.reason, input.detail ?? {},
  ])}`;
  const existing = responseIdempotencyKeys.get(fingerprint);
  if (existing) return existing.key;
  const randomPart = globalThis.crypto?.randomUUID?.()
    ?? `${now.toString(36)}-${Math.random().toString(36).slice(2)}`;
  const key = `alert-saved-view:${randomPart}`;
  responseIdempotencyKeys.set(fingerprint, { key, expiresAt: now + 5 * 60_000 });
  return key;
}

export type AlertSavedView = {
  view_id: string;
  name: string;
  filters: Record<string, unknown>;
  revision: number;
  created_at: string;
  updated_at: string;
};

export async function fetchAlertSavedViews(): Promise<AlertSavedView[]> {
  const response = await api.get('/v1/alerts/views');
  const envelope = response.data?.data ?? response.data;
  return Array.isArray(envelope?.views) ? envelope.views : [];
}

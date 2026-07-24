import { api } from '@/services/api';

export type AlertTriageActionKind = 'saved-view' | 'response-action' | 'investigation-note';

export type AlertTriageActionInput = {
  kind: AlertTriageActionKind;
  alertId?: string;
  action: string;
  target: string;
  reason: string;
  dryRun?: boolean;
  detail?: Record<string, unknown>;
};

export type AlertTriageActionResult = {
  job_id?: string;
  view_id?: string;
  status?: 'recorded' | 'pending_approval';
  outbox_status?: 'not_required' | 'pending_retry' | 'published';
  action: string;
  target: string;
  dry_run: boolean;
  audit_event: string;
};

export async function submitAlertTriageAction(input: AlertTriageActionInput): Promise<AlertTriageActionResult> {
  const alertId = input.alertId?.trim();
  const endpoint = input.kind === 'saved-view'
    ? '/v1/alerts/views'
    : `/v1/alerts/${encodeURIComponent(alertId || '')}/${input.kind === 'response-action' ? 'response-actions' : 'investigation-notes'}`;
  if (input.kind !== 'saved-view' && !alertId) throw new Error('请选择告警后再提交操作');
  const response = await api.post<{ data?: AlertTriageActionResult } & Partial<AlertTriageActionResult>>(endpoint, {
    action: input.action,
    target: input.target,
    reason: input.reason,
    dry_run: input.dryRun ?? input.kind === 'response-action',
    detail: input.detail,
  });
  const payload = response.data.data ?? response.data;
  if (!payload.job_id && !payload.view_id) throw new Error('告警操作未返回持久化记录编号');
  return payload as AlertTriageActionResult;
}

export type AlertSavedView = {
  view_id: string;
  name: string;
  filters: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export async function fetchAlertSavedViews(): Promise<AlertSavedView[]> {
  const response = await api.get('/v1/alerts/views');
  const envelope = response.data?.data ?? response.data;
  return Array.isArray(envelope?.views) ? envelope.views : [];
}

import { getPageActionPlan } from '@/services/pageApiPlans';
import { submitAlertTriageAction } from '@/services/alertTriageApi';
import { api } from '@/services/api';

export type AlertDetailActionId =
  | 'alert-report-export'
  | 'alert-campaign-link'
  | 'alert-label-update'
  | 'alert-evidence-access'
  | 'alert-response-request'
  | 'alert-investigation-note';

export type AlertDetailActionInput = {
  alertId: string;
  actionId: AlertDetailActionId;
  target: string;
  reason?: string;
  detail?: Record<string, unknown>;
};

export type AlertDetailActionResult = {
  actionId: AlertDetailActionId;
  action: string;
  apiContract: string;
  auditEvent: string;
  jobId: string;
  status: 'recorded' | 'pending_approval';
  target: string;
  mode: 'live';
  downloadUrl?: string;
  fileName?: string;
  expiresAt?: string;
};

export async function submitAlertDetailAction({
  alertId,
  actionId,
  target,
  reason,
  detail,
}: AlertDetailActionInput): Promise<AlertDetailActionResult> {
  const plan = getPageActionPlan('alert-detail', actionId);
  if (!plan) throw new Error(`未找到告警详情动作契约：${actionId}`);

  if (actionId === 'alert-label-update') {
    const labels = Array.isArray(detail?.labels)
      ? detail.labels.map((item) => String(item).trim()).filter(Boolean)
      : target.split(/[,，]/).map((item) => item.trim()).filter(Boolean);
    const response = await api.put(`/v1/alerts/${encodeURIComponent(alertId)}/labels`, {
      labels,
      reason: reason?.trim() || '告警详情标签编辑',
    });
    const payload = response.data?.data ?? response.data;
    return {
      actionId,
      action: plan.label,
      apiContract: `/v1/alerts/${encodeURIComponent(alertId)}/labels`,
      auditEvent: plan.auditEvent,
      jobId: `label-update-${payload?.alert_id ?? alertId}`,
      status: 'recorded',
      target: Array.isArray(payload?.labels) ? payload.labels.join('，') : target,
      mode: 'live',
    };
  }

  if (actionId === 'alert-evidence-access') {
    const executedEndpoint = `/v1/alerts/${encodeURIComponent(alertId)}/evidence/access`;
    const response = await api.post(executedEndpoint, {
      action: plan.label,
      target,
      reason: reason?.trim() || `下载告警证据：${target}`,
      detail: {
        action_id: actionId,
        source: 'alert-detail',
        requested_contract: plan.endpoint,
        executed_endpoint: executedEndpoint,
        evidence_id: target,
        ...detail,
      },
    });
    const payload = response.data?.data ?? response.data;
    return {
      actionId,
      action: plan.label,
      apiContract: executedEndpoint,
      auditEvent: String(payload?.audit_event || plan.auditEvent),
      jobId: String(payload?.job_id || ''),
      status: payload?.status === 'pending_approval' ? 'pending_approval' : 'recorded',
      target: String(payload?.target || target),
      mode: 'live',
      downloadUrl: String(payload?.download_url || ''),
      fileName: String(payload?.file_name || target),
      expiresAt: String(payload?.expires_at || ''),
    };
  }

  const isResponse = actionId === 'alert-response-request';
  const executedEndpoint = isResponse
    ? `/v1/alerts/${encodeURIComponent(alertId)}/response-actions`
    : `/v1/alerts/${encodeURIComponent(alertId)}/investigation-notes`;
  const submission = await submitAlertTriageAction({
    kind: isResponse ? 'response-action' : 'investigation-note',
    alertId,
    action: plan.label,
    target,
    reason: reason?.trim() || `告警详情提交：${plan.label}`,
    dryRun: isResponse,
    detail: {
      action_id: actionId,
      source: 'alert-detail',
      requested_contract: plan.endpoint,
      executed_endpoint: executedEndpoint,
      ...detail,
    },
  });
  return {
    actionId,
    action: plan.label,
    apiContract: executedEndpoint,
    auditEvent: plan.auditEvent,
    jobId: submission.job_id ?? submission.view_id ?? '',
    status: submission.status ?? 'recorded',
    target,
    mode: 'live',
  };
}

export async function downloadAlertEvidenceFile(downloadUrl: string, fileName: string): Promise<void> {
  if (!downloadUrl.startsWith('/v1/alerts/')) {
    throw new Error('证据下载地址无效');
  }
  const response = await api.get(downloadUrl, { responseType: 'blob' });
  const blob = response.data instanceof Blob
    ? response.data
    : new Blob([response.data], { type: 'application/octet-stream' });
  const objectUrl = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = objectUrl;
  anchor.download = fileName || 'evidence.json';
  anchor.style.display = 'none';
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(objectUrl);
}

export function alertDetailActionErrorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === 'object') {
    const response = 'response' in error ? (error as { response?: unknown }).response : undefined;
    if (response && typeof response === 'object' && 'data' in response) {
      const data = (response as { data?: unknown }).data;
      if (data && typeof data === 'object') {
        const nested = 'error' in data ? (data as { error?: unknown }).error : undefined;
        if (nested && typeof nested === 'object' && 'message' in nested) {
          const message = String((nested as { message?: unknown }).message ?? '').trim();
          if (message) return message;
        }
        if ('message' in data) {
          const message = String((data as { message?: unknown }).message ?? '').trim();
          if (message) return message;
        }
      }
    }
    if ('message' in error) {
      const message = String((error as { message?: unknown }).message ?? '').trim();
      if (message && !/^Request failed with status code \d+$/i.test(message)) return message;
    }
  }
  return fallback;
}

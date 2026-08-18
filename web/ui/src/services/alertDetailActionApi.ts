import { getPageActionPlan } from '@/services/pageApiPlans';
import { submitAlertTriageAction } from '@/services/alertTriageApi';
import { api } from '@/services/api';

export type AlertDetailActionId =
  | 'alert-report-export'
  | 'alert-report-cancel'
  | 'alert-report-compensate'
  | 'alert-campaign-link'
  | 'alert-campaign-unlink'
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

export type AlertResponseExecutionReceipt = {
  eventId: string;
  state: 'simulated_completed' | 'blocked_external_executor' | 'completed' | 'partial' | 'failed';
  simulated: boolean;
  externalEffect: boolean;
  aggregateVersion: number;
  result: Record<string, unknown>;
  error: string;
  kafkaPartition: number;
  kafkaOffset: number;
  provider: string;
  providerReceiptId: string;
  effectState: 'confirmed' | 'none' | 'unknown';
  effectIds: string[];
  traceId: string;
  receiptSHA256: string;
  authorityLookup: Record<string, unknown>;
  executedAt: string;
};

export type AlertDetailActionResult = {
  actionId: AlertDetailActionId;
  action: string;
  apiContract: string;
  auditEvent: string;
  jobId: string;
  status: 'recorded' | 'pending_approval' | 'approved_awaiting_executor' | 'simulated_completed' | 'blocked_external_executor' | 'compensation_queued' | 'compensation_blocked_external_executor' | 'linked' | 'unlinked' | 'accepted' | 'running' | 'cancel_requested' | 'completed' | 'partial' | 'failed' | 'cancelled' | 'compensating' | 'compensated' | 'compensation_failed';
  target: string;
  mode: 'live';
  downloadUrl?: string;
  fileName?: string;
  expiresAt?: string;
  artifactExpired?: boolean;
  manifestVersion?: number;
  objectFormatVersion?: number;
  snapshotSHA256?: string;
  artifactSHA256?: string;
  revision?: number;
  receiptAvailable?: boolean;
  executionReceipt?: AlertResponseExecutionReceipt;
};

export function alertDetailActionErrorCode(error: unknown): string {
  if (!error || typeof error !== 'object' || !('response' in error)) return '';
  const response = (error as { response?: unknown }).response;
  if (!response || typeof response !== 'object' || !('data' in response)) return '';
  const data = (response as { data?: unknown }).data;
  if (!data || typeof data !== 'object' || !('error' in data)) return '';
  const nested = (data as { error?: unknown }).error;
  if (!nested || typeof nested !== 'object' || !('code' in nested)) return '';
  return String((nested as { code?: unknown }).code ?? '').trim();
}

const reportStatuses: AlertDetailActionResult['status'][] = ['accepted', 'running', 'cancel_requested', 'completed', 'partial', 'failed', 'cancelled', 'compensating', 'compensated', 'compensation_failed'];

function normalizeReportStatus(value: unknown): AlertDetailActionResult['status'] {
  const status = String(value || 'accepted') as AlertDetailActionResult['status'];
  return reportStatuses.includes(status) ? status : 'accepted';
}

const responseStatuses: AlertDetailActionResult['status'][] = [
  'pending_approval', 'approved_awaiting_executor', 'simulated_completed',
  'blocked_external_executor', 'compensation_queued', 'compensation_blocked_external_executor',
  'completed', 'partial', 'failed', 'cancelled', 'compensated', 'compensation_failed',
];

function normalizeResponseStatus(value: unknown): AlertDetailActionResult['status'] {
  const status = String(value || 'recorded') as AlertDetailActionResult['status'];
  return responseStatuses.includes(status) ? status : 'recorded';
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {};
}

function normalizeAlertResponseReceipt(value: unknown): AlertResponseExecutionReceipt | undefined {
  const receipt = recordValue(value);
  if (!String(receipt.event_id || '').trim()) return undefined;
  const state = String(receipt.state || 'failed') as AlertResponseExecutionReceipt['state'];
  const effectState = String(receipt.effect_state || 'unknown') as AlertResponseExecutionReceipt['effectState'];
  return {
    eventId: String(receipt.event_id || ''),
    state: ['simulated_completed', 'blocked_external_executor', 'completed', 'partial', 'failed'].includes(state) ? state : 'failed',
    simulated: Boolean(receipt.simulated),
    externalEffect: Boolean(receipt.external_effect),
    aggregateVersion: Number(receipt.aggregate_version || 0),
    result: recordValue(receipt.result),
    error: String(receipt.error || ''),
    kafkaPartition: Number(receipt.kafka_partition || 0),
    kafkaOffset: Number(receipt.kafka_offset || 0),
    provider: String(receipt.provider || ''),
    providerReceiptId: String(receipt.provider_receipt_id || ''),
    effectState: ['confirmed', 'none', 'unknown'].includes(effectState) ? effectState : 'unknown',
    effectIds: Array.isArray(receipt.effect_ids) ? receipt.effect_ids.map(String) : [],
    traceId: String(receipt.trace_id || ''),
    receiptSHA256: String(receipt.receipt_sha256 || ''),
    authorityLookup: recordValue(receipt.authority_lookup),
    executedAt: String(receipt.executed_at || ''),
  };
}

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

  if (actionId === 'alert-report-export') {
    const format = String(detail?.format || 'pdf').toLowerCase();
    const snapshotId = String(detail?.snapshotId || `alert:${alertId}:current`);
    const executedEndpoint = `/v1/alerts/${encodeURIComponent(alertId)}/reports/export`;
    const idempotencyKey = `alert-report-export:${alertId}:${snapshotId}:${format}`;
    const response = await api.post(executedEndpoint, {
      action_id: actionId,
      format,
      snapshot_id: snapshotId,
      reason: reason?.trim() || `导出告警 ${alertId} 报告`,
    }, {
      headers: { 'Idempotency-Key': idempotencyKey },
    });
    const payload = response.data?.data ?? response.data;
    return {
      actionId,
      action: plan.label,
      apiContract: executedEndpoint,
      auditEvent: plan.auditEvent,
      jobId: String(payload?.job_id || ''),
      status: normalizeReportStatus(payload?.status),
      target: alertId,
      mode: 'live',
      downloadUrl: String(payload?.download_url || ''),
      fileName: `alert-${alertId}.${format}`,
      expiresAt: String(payload?.artifact_expires_at || ''),
      artifactExpired: Boolean(payload?.artifact_expired),
      manifestVersion: Number(payload?.manifest?.manifest_version || 0),
      objectFormatVersion: Number(payload?.manifest?.object_format_version || 0),
      snapshotSHA256: String(payload?.manifest?.snapshot_sha256 || payload?.snapshot_sha256 || ''),
      artifactSHA256: String(payload?.manifest?.artifact_sha256 || payload?.artifact_sha256 || ''),
      revision: Number(payload?.revision || 0),
    };
  }

  if (actionId === 'alert-campaign-link') {
    const campaignId = target.trim();
    const expectedRevision = Number(detail?.expectedRevision ?? 0);
    const executedEndpoint = `/v1/alerts/${encodeURIComponent(alertId)}/campaign-links`;
    const expectedCampaignRevision = numberOrUndefined(detail?.expectedCampaignRevision);
    const idempotencyKey = `alert-campaign-link:${alertId}:${campaignId}:${expectedRevision}:${expectedCampaignRevision ?? 'compat'}`;
    const response = await api.post(executedEndpoint, {
      campaign_id: campaignId,
      expected_revision: expectedRevision,
      ...(expectedCampaignRevision !== undefined ? { expected_campaign_revision: expectedCampaignRevision } : {}),
      reason: reason?.trim() || `关联告警 ${alertId} 至战役 ${campaignId}`,
    }, {
      headers: { 'Idempotency-Key': idempotencyKey },
    });
    const payload = response.data?.data ?? response.data;
    return {
      actionId,
      action: plan.label,
      apiContract: executedEndpoint,
      auditEvent: payload?.idempotent_reuse ? 'ALERT_CAMPAIGN_LINK_REUSED' : 'ALERT_CAMPAIGN_LINKED',
      jobId: String(payload?.relation_id || ''),
      status: 'linked',
      target: String(payload?.campaign_id || campaignId),
      mode: 'live',
    };
  }

  if (actionId === 'alert-campaign-unlink') {
    const campaignId = target.trim();
    const expectedRevision = Number(detail?.expectedRevision);
    const expectedCampaignRevision = numberOrUndefined(detail?.expectedCampaignRevision);
    if (!Number.isSafeInteger(expectedRevision) || expectedRevision < 1) {
      throw new Error('解除战役关系需要当前关系 revision');
    }
    const executedEndpoint = `/v1/alerts/${encodeURIComponent(alertId)}/campaign-links/${encodeURIComponent(campaignId)}`;
    const idempotencyKey = `alert-campaign-unlink:${alertId}:${campaignId}:${expectedRevision}:${expectedCampaignRevision ?? 'compat'}`;
    const response = await api.delete(executedEndpoint, {
      data: {
        expected_revision: expectedRevision,
        ...(expectedCampaignRevision !== undefined ? { expected_campaign_revision: expectedCampaignRevision } : {}),
        reason: reason?.trim() || `解除告警 ${alertId} 与战役 ${campaignId} 的关系`,
      },
      headers: { 'Idempotency-Key': idempotencyKey },
    });
    const payload = response.data?.data ?? response.data;
    return {
      actionId,
      action: plan.label,
      apiContract: executedEndpoint,
      auditEvent: payload?.idempotent_reuse ? 'ALERT_CAMPAIGN_MEMBERSHIP_REUSED' : 'ALERT_CAMPAIGN_UNLINKED',
      jobId: String(payload?.relation_id || ''),
      status: 'unlinked',
      target: String(payload?.campaign_id || campaignId),
      mode: 'live',
    };
  }

  const isResponse = actionId === 'alert-response-request';
  const executedEndpoint = isResponse
    ? `/v1/alerts/${encodeURIComponent(alertId)}/response-actions`
    : `/v1/alerts/${encodeURIComponent(alertId)}/investigation-notes`;
  const submission = await submitAlertTriageAction({
    kind: isResponse ? 'response-action' : 'investigation-note',
    actionId,
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

export async function submitAlertReportWithSnapshotRetry(
  input: AlertDetailActionInput,
  refreshSnapshotId: () => Promise<string>,
  maxAttempts = 3,
): Promise<AlertDetailActionResult> {
  if (input.actionId !== 'alert-report-export') return submitAlertDetailAction(input);
  const boundedAttempts = Math.max(1, Math.min(3, Math.trunc(maxAttempts)));
  let current = input;
  const initialSnapshotId = String(input.detail?.snapshotId ?? '').trim();
  const canonicalPrefix = `alert:${input.alertId}:revision:`;
  const initialRevision = initialSnapshotId.startsWith(canonicalPrefix)
    ? Number(initialSnapshotId.slice(canonicalPrefix.length))
    : Number.NaN;
  if (!Number.isSafeInteger(initialRevision) || initialRevision < 1) {
    const refreshedSnapshotId = String(await refreshSnapshotId()).trim();
    if (!refreshedSnapshotId) throw new Error('刷新告警快照后未返回可用 revision');
    current = { ...current, detail: { ...(current.detail ?? {}), snapshotId: refreshedSnapshotId } };
  }
  let lastError: unknown;
  for (let attempt = 0; attempt < boundedAttempts; attempt += 1) {
    try {
      return await submitAlertDetailAction(current);
    } catch (error) {
      lastError = error;
      if (alertDetailActionErrorCode(error) !== 'SNAPSHOT_CONFLICT' || attempt + 1 >= boundedAttempts) throw error;
      const snapshotId = String(await refreshSnapshotId()).trim();
      if (!snapshotId) throw new Error('刷新告警快照后未返回可用 revision');
      current = { ...current, detail: { ...(current.detail ?? {}), snapshotId } };
    }
  }
  throw lastError;
}

export type AlertCampaignLink = {
  relationId: string;
  alertId: string;
  campaignId: string;
  status: 'linked' | 'unlinked';
  revision: number;
  campaignRevision: number;
  currentCampaignRevision: number;
};

export type AlertCampaignLinksSnapshot = {
  links: AlertCampaignLink[];
  total: number;
  unlinkAvailable: boolean;
  partial: boolean;
  missingSections: string[];
};

export async function fetchAlertCampaignLinks(alertId: string): Promise<AlertCampaignLinksSnapshot> {
  const endpoint = `/v1/alerts/${encodeURIComponent(alertId)}/campaign-links`;
  const response = await api.get(endpoint);
  const envelope = response.data;
  const payload = envelope?.data ?? envelope;
  const links = Array.isArray(payload?.links) ? payload.links : [];
  return {
    links: links.map((item: Record<string, unknown>) => ({
      relationId: String(item.relation_id || ''),
      alertId: String(item.alert_id || alertId),
      campaignId: String(item.campaign_id || ''),
      status: item.status === 'unlinked' ? 'unlinked' : 'linked',
      revision: Number(item.revision || 0),
      campaignRevision: Number(item.campaign_revision || 0),
      currentCampaignRevision: Number(item.current_campaign_revision || 0),
    })).filter((item: AlertCampaignLink) => item.relationId && item.campaignId),
    total: Number(payload?.total || links.length),
    unlinkAvailable: Boolean(payload?.unlink_available),
    partial: Boolean(envelope?.meta?.partial),
    missingSections: Array.isArray(envelope?.meta?.missing_sections)
      ? envelope.meta.missing_sections.map(String)
      : [],
  };
}

function numberOrUndefined(value: unknown): number | undefined {
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed >= 0 ? parsed : undefined;
}

export async function fetchAlertReportJob(alertId: string, jobId: string): Promise<AlertDetailActionResult> {
  const endpoint = `/v1/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(jobId)}`;
  const response = await api.get(endpoint);
  const payload = response.data?.data ?? response.data;
  const format = String(payload?.format || 'pdf');
  return {
    actionId: 'alert-report-export',
    action: 'Export alert report',
    apiContract: endpoint,
    auditEvent: payload?.status === 'completed' ? 'ALERT_REPORT_EXPORT_COMPLETED' : 'ALERT_REPORT_EXPORT_REQUESTED',
    jobId: String(payload?.job_id || jobId),
    status: normalizeReportStatus(payload?.status),
    target: String(payload?.alert_id || alertId),
    mode: 'live',
    downloadUrl: String(payload?.download_url || ''),
    fileName: `alert-${alertId}.${format}`,
    expiresAt: String(payload?.artifact_expires_at || ''),
    artifactExpired: Boolean(payload?.artifact_expired),
    manifestVersion: Number(payload?.manifest?.manifest_version || 0),
    objectFormatVersion: Number(payload?.manifest?.object_format_version || 0),
    snapshotSHA256: String(payload?.manifest?.snapshot_sha256 || payload?.snapshot_sha256 || ''),
    artifactSHA256: String(payload?.manifest?.artifact_sha256 || payload?.artifact_sha256 || ''),
    revision: Number(payload?.revision || 0),
  };
}

export async function fetchAlertResponseAction(alertId: string, jobId: string): Promise<AlertDetailActionResult> {
  const endpoint = `/v1/alerts/${encodeURIComponent(alertId)}/response-actions/${encodeURIComponent(jobId)}`;
  const response = await api.get(endpoint);
  const payload = response.data?.data ?? response.data;
  const executionReceipt = normalizeAlertResponseReceipt(payload?.execution_receipt);
  return {
    actionId: 'alert-response-request',
    action: String(payload?.action || 'Response recommendation'),
    apiContract: endpoint,
    auditEvent: executionReceipt
      ? `ALERT_RESPONSE_EXECUTION_${executionReceipt.state.toUpperCase()}`
      : 'ALERT_RESPONSE_ACTION_REQUESTED',
    jobId: String(payload?.job_id || jobId),
    status: normalizeResponseStatus(payload?.status),
    target: String(payload?.target || alertId),
    mode: 'live',
    revision: Number(payload?.revision || 0),
    receiptAvailable: Boolean(payload?.receipt_available && executionReceipt),
    executionReceipt,
  };
}

export async function cancelAlertReport(
  alertId: string,
  jobId: string,
  expectedRevision: number,
  reason: string,
): Promise<AlertDetailActionResult> {
  if (!Number.isSafeInteger(expectedRevision) || expectedRevision < 1) {
    throw new Error('取消报告需要当前任务 revision');
  }
  const plan = getPageActionPlan('alert-detail', 'alert-report-cancel');
  if (!plan) throw new Error('未找到告警报告取消契约');
  const endpoint = `/v1/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(jobId)}/cancel`;
  const normalizedReason = reason.trim();
  const response = await api.post(endpoint, {
    action_id: 'alert-report-cancel',
    expected_revision: expectedRevision,
    reason: normalizedReason,
  }, {
    headers: {
      'Idempotency-Key': `alert-report-cancel:${jobId}:${expectedRevision}`,
    },
  });
  const payload = response.data?.data ?? response.data;
  return {
    actionId: 'alert-report-cancel',
    action: plan.label,
    apiContract: endpoint,
    auditEvent: 'ALERT_REPORT_EXPORT_CANCEL_REQUESTED',
    jobId: String(payload?.job_id || jobId),
    status: normalizeReportStatus(payload?.status),
    target: String(payload?.alert_id || alertId),
    mode: 'live',
    revision: Number(payload?.revision || expectedRevision),
  };
}

export async function cancelAlertReportWithRevisionRefresh(
  alertId: string,
  jobId: string,
  expectedRevision: number,
  reason: string,
  maxAttempts = 2,
): Promise<AlertDetailActionResult> {
  const boundedAttempts = Math.max(1, Math.min(2, Math.trunc(maxAttempts)));
  let revision = expectedRevision;
  let lastError: unknown;
  for (let attempt = 0; attempt < boundedAttempts; attempt += 1) {
    try {
      return await cancelAlertReport(alertId, jobId, revision, reason);
    } catch (error) {
      lastError = error;
      const code = alertDetailActionErrorCode(error);
      if (!['REVISION_CONFLICT', 'REPORT_STATE_CONFLICT'].includes(code)) throw error;
      const current = await fetchAlertReportJob(alertId, jobId);
      if (!['accepted', 'running'].includes(current.status)) return current;
      if (!Number.isSafeInteger(current.revision) || Number(current.revision) < 1 || attempt + 1 >= boundedAttempts) {
        throw error;
      }
      revision = Number(current.revision);
    }
  }
  throw lastError;
}

export async function compensateAlertReport(
  alertId: string,
  jobId: string,
  expectedRevision: number,
  reason: string,
): Promise<AlertDetailActionResult> {
  if (!Number.isSafeInteger(expectedRevision) || expectedRevision < 1) {
    throw new Error('报告对象补偿需要当前任务 revision');
  }
  const plan = getPageActionPlan('alert-detail', 'alert-report-compensate');
  if (!plan) throw new Error('未找到告警报告补偿契约');
  const endpoint = `/v1/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(jobId)}/compensations`;
  const normalizedReason = reason.trim();
  const response = await api.post(endpoint, {
    action_id: 'alert-report-compensate',
    expected_revision: expectedRevision,
    reason: normalizedReason,
  }, {
    headers: {
      'Idempotency-Key': `alert-report-compensate:${jobId}:${expectedRevision}`,
    },
  });
  const payload = response.data?.data ?? response.data;
  return {
    actionId: 'alert-report-compensate',
    action: plan.label,
    apiContract: endpoint,
    auditEvent: 'ALERT_REPORT_EXPORT_COMPENSATION_REQUESTED',
    jobId: String(payload?.job_id || jobId),
    status: normalizeReportStatus(payload?.status),
    target: String(payload?.alert_id || alertId),
    mode: 'live',
    revision: Number(payload?.revision || expectedRevision),
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

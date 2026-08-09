import { getAuthToken } from '@/services/authStorage';
import { api } from '@/services/api';
import { getPageActionPlan, type EndpointMethod } from '@/services/pageApiPlans';

export type CampaignActionId =
  | 'campaign-export'
  | 'campaign-list-settings'
  | 'campaign-assign-owner'
  | 'campaign-status-change'
  | 'campaign-context-action'
  | 'campaign-detail-view'
  | 'campaign-phase-inspect'
  | 'campaign-impact-inspect'
  | 'campaign-evidence-view'
  | 'campaign-report-generate'
  | 'campaign-attack-chain-view'
  | 'campaign-graph-view'
  | 'campaign-soar-response';

type CampaignActionTarget =
  | { campaignId: string; targetId?: string }
  | { campaignId?: string; targetId: string };

export type CampaignActionInput = CampaignActionTarget & {
  actionId: CampaignActionId;
  target: string;
  phase?: string;
  scope?: string;
  metadata?: Record<string, unknown>;
  requestBody?: Record<string, unknown>;
  expectedRevision?: number;
  reason?: string;
  idempotencyKey?: string;
};

export type CampaignActionStatus =
  | 'accepted'
  | 'pending_approval'
  | 'approved_awaiting_executor'
  | 'running'
  | 'completed'
  | 'succeeded'
  | 'partial'
  | 'failed'
  | 'cancelled'
  | 'compensation_queued'
  | 'compensating'
  | 'compensated'
  | 'compensation_failed';

export type CampaignActionStatusClass =
  | 'in_progress'
  | 'succeeded'
  | 'partial'
  | 'failed'
  | 'cancelled'
  | 'compensated';

const campaignActionStatuses: CampaignActionStatus[] = [
  'accepted',
  'pending_approval',
  'approved_awaiting_executor',
  'running',
  'completed',
  'succeeded',
  'partial',
  'failed',
  'cancelled',
  'compensation_queued',
  'compensating',
  'compensated',
  'compensation_failed',
];

export function classifyCampaignActionStatus(status: CampaignActionStatus): CampaignActionStatusClass {
  if (status === 'completed' || status === 'succeeded') return 'succeeded';
  if (status === 'partial') return 'partial';
  if (status === 'failed' || status === 'compensation_failed') return 'failed';
  if (status === 'cancelled') return 'cancelled';
  if (status === 'compensated') return 'compensated';
  return 'in_progress';
}

export type CampaignActionAuditRecord = {
  actionId: CampaignActionId;
  event: string;
  method: EndpointMethod;
  endpoint: string;
  requiredScopes: string[];
  requestBody: Record<string, unknown>;
  jobId: string;
  status: CampaignActionStatus;
  targetId: string;
  target: string;
  timestamp: string;
};

export type CampaignActionResult = {
  actionId: CampaignActionId;
  method: EndpointMethod;
  endpoint: string;
  auditEvent: string;
  auditRecord: CampaignActionAuditRecord;
  requestBody: Record<string, unknown>;
  jobId: string;
  status: CampaignActionStatus;
  jobStatus: CampaignActionStatus;
  mode: 'server-persisted-read' | 'server-persisted-mutation';
  result: Record<string, unknown>;
};

export type CampaignReportState =
  | 'accepted'
  | 'running'
  | 'completed'
  | 'partial'
  | 'failed'
  | 'cancelled'
  | 'compensated';

export type CampaignReportStatus = {
  reportId: string;
  jobId: string;
  tenantId: string;
  campaignId: string;
  format: 'json' | 'pdf' | 'word';
  status: CampaignReportState;
  campaignRevision: number;
  snapshotId: string;
  snapshotSHA256: string;
  objectManifest: Record<string, unknown>;
  mimeType: string;
  artifactSHA256: string;
  sizeBytes: number;
  attempts: number;
  errorMessage: string;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
  contractVersion?: number;
  traceId?: string;
  sourceWatermarks: Record<string, string>;
};

export type CampaignReportArtifact = {
  blob: Blob;
  filename: string;
  sha256: string;
};

export type CampaignSOARState =
  | 'pending_approval'
  | 'approved_awaiting_executor'
  | 'running'
  | 'completed'
  | 'partial'
  | 'failed'
  | 'cancelled'
  | 'compensation_queued'
  | 'compensating'
  | 'compensated'
  | 'compensation_failed';

export type CampaignSOARJob = {
  jobId: string;
  tenantId: string;
  campaignId: string;
  playbookId: string;
  target: string;
  sourceSnapshotId: string;
  campaignRevision: number;
  status: CampaignSOARState;
  approvalStatus: 'pending' | 'approved' | 'rejected' | 'cancelled';
  executorStatus: string;
  revision: number;
  request: Record<string, unknown>;
  executionReceipt: Record<string, unknown>;
  compensationReceipt: Record<string, unknown>;
  errorMessage: string;
  attempts: number;
  requestedBy: string;
  approvedBy: string;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
  contractVersion?: number;
  traceId?: string;
  sourceWatermarks: Record<string, string>;
};

export type CampaignSOAROperation = 'approve' | 'reject' | 'cancel' | 'compensate';

export type WaitForCampaignReportOptions = {
  intervalMs?: number;
  timeoutMs?: number;
  signal?: AbortSignal;
  onStatus?: (status: CampaignReportStatus) => void;
};

export class CampaignReportTerminalError extends Error {
  readonly report: CampaignReportStatus;

  constructor(report: CampaignReportStatus) {
    super(report.errorMessage || `战役报告以 ${report.status} 状态结束`);
    this.name = 'CampaignReportTerminalError';
    this.report = report;
  }
}

type CampaignActionServerResponse = {
  action_id?: string;
  audit_event?: string;
  endpoint?: string;
  job_id?: string;
  status?: CampaignActionStatus;
  job_status?: CampaignActionStatus;
  simulation?: boolean;
  dry_run?: boolean;
  result?: Record<string, unknown>;
};

const CAMPAIGN_AUDIT_KEY = 'taf:campaign-action-audit';
const MAX_AUDIT_RECORDS = 20;
const mutatingActions = new Set<CampaignActionId>([
  'campaign-assign-owner',
  'campaign-status-change',
  'campaign-report-generate',
  'campaign-soar-response',
]);

export async function submitCampaignAction(input: CampaignActionInput): Promise<CampaignActionResult> {
  const plan = getPageActionPlan('campaigns', input.actionId);
  if (!plan) throw new Error(`未找到 Campaign 动作契约：${input.actionId}`);
  if (!plan.auditEvent) throw new Error(`Campaign 契约不是可提交动作：${input.actionId}`);

  const token = getAuthToken();
  if (token && !hasAcceptedScope(readJwtPermissions(token), plan.acceptedScopes ?? plan.requiredScopes)) {
    throw new Error(`缺少 Campaign 动作权限：${plan.requiredScopes.join(', ')}`);
  }

  const targetId = input.campaignId ?? input.targetId;
  if (!targetId) throw new Error('Campaign 动作缺少战役 ID');
  const phase = input.phase ?? stringMetadata(input.metadata, 'phase');
  const scope = input.scope ?? stringMetadata(input.metadata, 'scope');
  const endpoint = resolveEndpoint(plan.endpoint, {
    id: targetId,
    phase: phase ?? 'current',
    scope: scope ?? 'assets',
  });
  const isCollectionAction = !plan.endpoint.includes('{id}');
  const isMutation = mutatingActions.has(input.actionId);
  const expectedRevision = input.expectedRevision ?? numberMetadata(input.metadata, 'expected_revision') ?? 0;
  const reason = input.reason?.trim() || stringMetadata(input.metadata, 'reason')?.trim() || `执行战役动作：${input.target}`;
  const metadata = {
    ...(plan.defaultBody ?? {}),
    ...(input.metadata ?? {}),
    ...(input.requestBody ?? {}),
    ...(!isCollectionAction ? { campaign_id: targetId } : {}),
    phase,
    scope,
    dry_run: !isMutation,
  };
  if (isCollectionAction) delete metadata.campaign_id;
  const requestBody = {
    action_id: input.actionId,
    target: input.target,
    metadata,
    simulation: !isMutation,
    dry_run: !isMutation,
    ...(isMutation ? { expected_revision: expectedRevision, reason } : {}),
  };
  const idempotencyKey = input.idempotencyKey ?? campaignCommandIdempotencyKey(targetId, input.actionId, expectedRevision, requestBody);
  const response = await api.request<{ data?: CampaignActionServerResponse } & CampaignActionServerResponse>({
    url: endpoint,
    method: plan.method,
    data: requestBody,
    ...(isMutation ? { headers: { 'Idempotency-Key': idempotencyKey } } : {}),
  });
  const serverResult = response.data.data ?? response.data;
  const status = serverResult.status;
  const jobStatus = serverResult.job_status;
  const acceptedStatuses: CampaignActionStatus[] = isMutation ? campaignActionStatuses : ['completed'];
  if (serverResult.simulation !== !isMutation || serverResult.dry_run !== !isMutation || !status || !jobStatus || !acceptedStatuses.includes(status) || !acceptedStatuses.includes(jobStatus)) {
    throw new Error('Campaign 动作服务未返回可验证的受理或最终结果');
  }
  const jobId = serverResult.job_id?.trim();
  if (!jobId) throw new Error('Campaign 动作服务未返回持久化作业编号');
  const auditRecord: CampaignActionAuditRecord = {
    actionId: input.actionId,
    event: serverResult.audit_event ?? plan.auditEvent,
    method: plan.method,
    endpoint,
    requiredScopes: plan.requiredScopes,
    requestBody,
    jobId,
    status,
    targetId,
    target: input.target,
    timestamp: new Date().toISOString(),
  };

  persistAuditRecord(auditRecord);

  return {
    actionId: input.actionId,
    method: plan.method,
    endpoint: serverResult.endpoint ?? endpoint,
    auditEvent: serverResult.audit_event ?? plan.auditEvent,
    auditRecord,
    requestBody,
    jobId,
    status,
    jobStatus,
    mode: isMutation ? 'server-persisted-mutation' : 'server-persisted-read',
    result: serverResult.result ?? {},
  };
}

export async function getCampaignSOARJob(campaignId: string, jobId: string, signal?: AbortSignal): Promise<CampaignSOARJob> {
  const endpoint = campaignSOAREndpoint('campaign-soar-status', campaignId, jobId);
  const response = await api.request<Record<string, unknown>>({ url: endpoint, method: 'GET', signal });
  const envelope = recordValue(response.data);
  const job = normalizeCampaignSOARJob(recordValue(envelope.data ?? envelope), recordValue(envelope.meta));
  if (job.campaignId !== campaignId || job.jobId !== jobId) {
    throw new Error('SOAR 作业状态响应与请求资源不一致');
  }
  return job;
}

export async function applyCampaignSOAROperation(
  campaignId: string,
  jobId: string,
  operation: CampaignSOAROperation,
  expectedRevision: number,
  reason: string,
): Promise<CampaignSOARJob> {
  const actionId = operation === 'approve' || operation === 'reject'
    ? 'campaign-soar-approval'
    : operation === 'cancel'
      ? 'campaign-soar-cancel'
      : 'campaign-soar-compensate';
  const endpoint = campaignSOAREndpoint(actionId, campaignId, jobId);
  const body = operation === 'approve' || operation === 'reject'
    ? { decision: operation, expected_revision: expectedRevision, reason: reason.trim() }
    : { expected_revision: expectedRevision, reason: reason.trim() };
  if (!Number.isSafeInteger(expectedRevision) || expectedRevision <= 0 || reason.trim().length < 8) {
    throw new Error('SOAR 操作需要当前正整数 revision 和至少 8 个字符的原因');
  }
  const response = await api.request<Record<string, unknown>>({
    url: endpoint,
    method: 'POST',
    data: body,
    headers: { 'Idempotency-Key': campaignSOARIdempotencyKey(jobId, operation, expectedRevision, body) },
  });
  const envelope = recordValue(response.data);
  const job = normalizeCampaignSOARJob(recordValue(envelope.data ?? envelope), recordValue(envelope.meta));
  if (job.campaignId !== campaignId || job.jobId !== jobId) {
    throw new Error('SOAR 操作响应与请求资源不一致');
  }
  return job;
}

export async function getCampaignReport(
  campaignId: string,
  reportId: string,
  signal?: AbortSignal,
): Promise<CampaignReportStatus> {
  const endpoint = campaignReportEndpoint('campaign-report-status', campaignId, reportId);
  const response = await api.request<Record<string, unknown>>({
    url: endpoint,
    method: 'GET',
    signal,
  });
  const envelope = recordValue(response.data);
  const payload = recordValue(envelope.data ?? envelope);
  const report = normalizeCampaignReport(payload, recordValue(envelope.meta));
  if (report.campaignId !== campaignId || report.reportId !== reportId) {
    throw new Error('战役报告状态响应与请求资源不一致');
  }
  return report;
}

export async function waitForCampaignReport(
  campaignId: string,
  reportId: string,
  options: WaitForCampaignReportOptions = {},
): Promise<CampaignReportStatus> {
  const intervalMs = Math.max(0, options.intervalMs ?? 1_000);
  const timeoutMs = Math.max(1, options.timeoutMs ?? 120_000);
  const deadline = Date.now() + timeoutMs;
  let lastStatus: CampaignReportStatus | undefined;
  while (Date.now() <= deadline) {
    if (options.signal?.aborted) throw new Error('战役报告状态轮询已取消');
    lastStatus = await getCampaignReport(campaignId, reportId, options.signal);
    options.onStatus?.(lastStatus);
    if (lastStatus.status === 'completed') return lastStatus;
    if (['partial', 'failed', 'cancelled', 'compensated'].includes(lastStatus.status)) {
      throw new CampaignReportTerminalError(lastStatus);
    }
    const remaining = deadline - Date.now();
    if (remaining <= 0) break;
    await waitForDelay(Math.min(intervalMs, remaining), options.signal);
  }
  throw new Error(`战役报告等待超时，最后状态：${lastStatus?.status ?? 'unknown'}`);
}

export async function downloadCampaignReport(
  campaignId: string,
  report: CampaignReportStatus,
): Promise<CampaignReportArtifact> {
  if (report.campaignId !== campaignId || report.status !== 'completed') {
    throw new Error('战役报告尚未完成，不能下载');
  }
  const endpoint = campaignReportEndpoint('campaign-report-download', campaignId, report.reportId);
  const response = await api.request<Blob>({
    url: endpoint,
    method: 'GET',
    responseType: 'blob',
  });
  const blob = response.data;
  if (!(blob instanceof Blob) || blob.size !== report.sizeBytes) {
    throw new Error('战役报告下载大小与权威 manifest 不一致');
  }
  const responseSHA = responseHeader(response.headers, 'x-content-sha256');
  if (responseSHA && report.artifactSHA256 && responseSHA !== report.artifactSHA256) {
    throw new Error('战役报告下载摘要与权威 manifest 不一致');
  }
  const contentDisposition = responseHeader(response.headers, 'content-disposition');
  return {
    blob,
    filename: campaignReportFilename(contentDisposition, report),
    sha256: responseSHA || report.artifactSHA256,
  };
}

export function saveCampaignReportArtifact(artifact: CampaignReportArtifact) {
  const objectURL = URL.createObjectURL(artifact.blob);
  const anchor = document.createElement('a');
  anchor.href = objectURL;
  anchor.download = artifact.filename;
  anchor.click();
  URL.revokeObjectURL(objectURL);
}

const stringMetadata = (metadata: Record<string, unknown> | undefined, key: string) => {
  const value = metadata?.[key];
  return typeof value === 'string' ? value : undefined;
};

const campaignReportEndpoint = (actionId: 'campaign-report-status' | 'campaign-report-download', campaignId: string, reportId: string) => {
  const plan = getPageActionPlan('campaigns', actionId);
  if (!plan || plan.method !== 'GET') throw new Error(`未找到 Campaign 报告接口契约：${actionId}`);
  return resolveEndpoint(plan.endpoint, { id: campaignId, report_id: reportId });
};

const campaignSOAREndpoint = (
  actionId: 'campaign-soar-status' | 'campaign-soar-approval' | 'campaign-soar-cancel' | 'campaign-soar-compensate',
  campaignId: string,
  jobId: string,
) => {
  const plan = getPageActionPlan('campaigns', actionId);
  if (!plan) throw new Error(`未找到 Campaign SOAR 接口契约：${actionId}`);
  return resolveEndpoint(plan.endpoint, { id: campaignId, job_id: jobId });
};

const normalizeCampaignSOARJob = (payload: Record<string, unknown>, meta: Record<string, unknown>): CampaignSOARJob => {
  const status = stringValue(payload.status) as CampaignSOARState;
  const approvalStatus = stringValue(payload.approval_status) as CampaignSOARJob['approvalStatus'];
  const validStates: CampaignSOARState[] = [
    'pending_approval', 'approved_awaiting_executor', 'running', 'completed', 'partial', 'failed',
    'cancelled', 'compensation_queued', 'compensating', 'compensated', 'compensation_failed',
  ];
  if (!validStates.includes(status)) throw new Error('SOAR 作业响应缺少合法状态');
  if (!['pending', 'approved', 'rejected', 'cancelled'].includes(approvalStatus)) {
    throw new Error('SOAR 作业响应缺少合法审批状态');
  }
  const job: CampaignSOARJob = {
    jobId: stringValue(payload.job_id),
    tenantId: stringValue(payload.tenant_id),
    campaignId: stringValue(payload.campaign_id),
    playbookId: stringValue(payload.playbook_id),
    target: stringValue(payload.target),
    sourceSnapshotId: stringValue(payload.source_snapshot_id),
    campaignRevision: nonNegativeInteger(payload.campaign_revision),
    status,
    approvalStatus,
    executorStatus: stringValue(payload.executor_status),
    revision: nonNegativeInteger(payload.revision),
    request: recordValue(payload.request),
    executionReceipt: recordValue(payload.execution_receipt),
    compensationReceipt: recordValue(payload.compensation_receipt),
    errorMessage: stringValue(payload.error_message),
    attempts: nonNegativeInteger(payload.attempts),
    requestedBy: stringValue(payload.requested_by),
    approvedBy: stringValue(payload.approved_by),
    createdAt: stringValue(payload.created_at),
    updatedAt: stringValue(payload.updated_at),
    completedAt: stringValue(payload.completed_at) || undefined,
    contractVersion: optionalNonNegativeInteger(meta.contract_version),
    traceId: stringValue(meta.trace_id) || undefined,
    sourceWatermarks: stringRecord(meta.source_watermarks),
  };
  if (!job.jobId || !job.tenantId || !job.campaignId || !job.playbookId || !job.sourceSnapshotId ||
      !job.executorStatus || job.campaignRevision <= 0 || job.revision <= 0) {
    throw new Error('SOAR 作业响应缺少稳定身份、快照或 revision');
  }
  if ((job.status === 'completed' || job.status === 'partial') && !stringValue(job.executionReceipt.provider_receipt_id)) {
    throw new Error('SOAR 执行终态缺少 provider 回执');
  }
  if (job.status === 'compensated' && !stringValue(job.compensationReceipt.provider_receipt_id)) {
    throw new Error('SOAR 补偿终态缺少 provider 回执');
  }
  return job;
};

const normalizeCampaignReport = (payload: Record<string, unknown>, meta: Record<string, unknown>): CampaignReportStatus => {
  const status = stringValue(payload.status) as CampaignReportState;
  const format = stringValue(payload.format) as CampaignReportStatus['format'];
  if (!['accepted', 'running', 'completed', 'partial', 'failed', 'cancelled', 'compensated'].includes(status)) {
    throw new Error('战役报告状态响应缺少合法状态');
  }
  if (!['json', 'pdf', 'word'].includes(format)) throw new Error('战役报告状态响应缺少合法格式');
  const report: CampaignReportStatus = {
    reportId: stringValue(payload.report_id),
    jobId: stringValue(payload.job_id),
    tenantId: stringValue(payload.tenant_id),
    campaignId: stringValue(payload.campaign_id),
    format,
    status,
    campaignRevision: nonNegativeInteger(payload.campaign_revision),
    snapshotId: stringValue(payload.snapshot_id),
    snapshotSHA256: stringValue(payload.snapshot_sha256),
    objectManifest: recordValue(payload.object_manifest),
    mimeType: stringValue(payload.mime_type),
    artifactSHA256: stringValue(payload.artifact_sha256),
    sizeBytes: nonNegativeInteger(payload.size_bytes),
    attempts: nonNegativeInteger(payload.attempts),
    errorMessage: stringValue(payload.error_message),
    createdAt: stringValue(payload.created_at),
    updatedAt: stringValue(payload.updated_at),
    completedAt: stringValue(payload.completed_at) || undefined,
    contractVersion: optionalNonNegativeInteger(meta.contract_version),
    traceId: stringValue(meta.trace_id) || undefined,
    sourceWatermarks: stringRecord(meta.source_watermarks),
  };
  if (!report.reportId || !report.jobId || !report.tenantId || !report.campaignId || !report.snapshotId || !report.snapshotSHA256) {
    throw new Error('战役报告状态响应缺少稳定身份或快照字段');
  }
  if (report.status === 'completed' && (!report.artifactSHA256 || !report.mimeType || report.sizeBytes <= 0)) {
    throw new Error('已完成的战役报告缺少对象 manifest 字段');
  }
  return report;
};

const recordValue = (value: unknown): Record<string, unknown> =>
  value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};

const stringValue = (value: unknown) => typeof value === 'string' ? value.trim() : '';

const nonNegativeInteger = (value: unknown) =>
  typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : 0;

const optionalNonNegativeInteger = (value: unknown) =>
  typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : undefined;

const stringRecord = (value: unknown): Record<string, string> =>
  Object.fromEntries(Object.entries(recordValue(value)).filter((entry): entry is [string, string] => typeof entry[1] === 'string'));

const waitForDelay = (milliseconds: number, signal?: AbortSignal) => new Promise<void>((resolve, reject) => {
  if (milliseconds <= 0) {
    resolve();
    return;
  }
  const timeout = globalThis.setTimeout(resolve, milliseconds);
  signal?.addEventListener('abort', () => {
    globalThis.clearTimeout(timeout);
    reject(new Error('战役报告状态轮询已取消'));
  }, { once: true });
});

const responseHeader = (headers: unknown, name: string) => {
  const candidate = headers as { get?: (key: string) => unknown } | Record<string, unknown> | undefined;
  const fromGetter = candidate && 'get' in candidate && typeof candidate.get === 'function' ? candidate.get(name) : undefined;
  if (typeof fromGetter === 'string') return fromGetter;
  if (!candidate || typeof candidate !== 'object') return '';
  const matched = Object.entries(candidate).find(([key]) => key.toLowerCase() === name.toLowerCase());
  return typeof matched?.[1] === 'string' ? matched[1] : '';
};

const campaignReportFilename = (contentDisposition: string, report: CampaignReportStatus) => {
  const encoded = contentDisposition.match(/filename\*=UTF-8''([^;]+)/i)?.[1];
  const plain = contentDisposition.match(/filename="?([^";]+)"?/i)?.[1];
  let filename = encoded ? decodeURIComponent(encoded) : plain;
  if (!filename) {
    const extension = report.format === 'word' ? 'docx' : report.format;
    filename = `${report.reportId}.${extension}`;
  }
  return filename.replace(/[\\/\r\n]/g, '_');
};

const numberMetadata = (metadata: Record<string, unknown> | undefined, key: string) => {
  const value = metadata?.[key];
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : undefined;
};

const campaignCommandIdempotencyKey = (
  campaignId: string,
  actionId: CampaignActionId,
  expectedRevision: number,
  requestBody: Record<string, unknown>,
) => {
  const material = stableJSONStringify(requestBody);
  let hash = 2166136261;
  for (let index = 0; index < material.length; index += 1) {
    hash ^= material.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return `campaign:${actionId}:${expectedRevision}:${(hash >>> 0).toString(16).padStart(8, '0')}`;
};

const campaignSOARIdempotencyKey = (
  jobId: string,
  operation: CampaignSOAROperation,
  expectedRevision: number,
  body: Record<string, unknown>,
) => {
  const material = `${jobId}:${operation}:${expectedRevision}:${stableJSONStringify(body)}`;
  let hash = 2166136261;
  for (let index = 0; index < material.length; index += 1) {
    hash ^= material.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return `campaign-soar:${operation}:${expectedRevision}:${(hash >>> 0).toString(16).padStart(8, '0')}`;
};

const stableJSONStringify = (value: unknown): string => {
  if (Array.isArray(value)) return `[${value.map(stableJSONStringify).join(',')}]`;
  if (value && typeof value === 'object') {
    return `{${Object.entries(value as Record<string, unknown>)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${JSON.stringify(key)}:${stableJSONStringify(item)}`)
      .join(',')}}`;
  }
  return JSON.stringify(value) ?? 'null';
};

const resolveEndpoint = (template: string, replacements: Record<string, string>) =>
  template.replace(/\{([^}]+)\}/g, (placeholder, key: string) => {
    const value = replacements[key];
    return value === undefined ? placeholder : encodeURIComponent(value);
  });

const hasAcceptedScope = (permissions: string[], acceptedScopes: string[]) =>
  acceptedScopes.some((accepted) => permissions.some((permission) => scopesOverlap(permission, accepted)));

const scopesOverlap = (permission: string, accepted: string) =>
  permission === accepted || scopePatternMatches(permission, accepted);

const scopePatternMatches = (pattern: string, scope: string) => {
  if (pattern === '*') return true;
  if (!pattern.endsWith(':*')) return false;
  return scope.startsWith(pattern.slice(0, -1));
};

const readJwtPermissions = (token: string): string[] => {
  try {
    const encodedPayload = token.split('.')[1];
    if (!encodedPayload) return [];
    const base64 = encodedPayload.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(encodedPayload.length / 4) * 4, '=');
    const payload = JSON.parse(globalThis.atob(base64)) as { permissions?: unknown };
    return Array.isArray(payload.permissions)
      ? payload.permissions.filter((item): item is string => typeof item === 'string')
      : [];
  } catch {
    return [];
  }
};

const persistAuditRecord = (auditRecord: CampaignActionAuditRecord) => {
  if (typeof window === 'undefined') return;
  try {
    let entries: unknown[] = [];
    const raw = window.sessionStorage.getItem(CAMPAIGN_AUDIT_KEY);
    if (raw) {
      const stored = JSON.parse(raw) as unknown;
      if (Array.isArray(stored)) entries = stored;
    }
    window.sessionStorage.setItem(
      CAMPAIGN_AUDIT_KEY,
      JSON.stringify([...entries.slice(-(MAX_AUDIT_RECORDS - 1)), auditRecord]),
    );
  } catch {
    try {
      window.sessionStorage.setItem(CAMPAIGN_AUDIT_KEY, JSON.stringify([auditRecord]));
    } catch {
      // Restricted browser contexts can expose sessionStorage while denying access.
    }
  }
};

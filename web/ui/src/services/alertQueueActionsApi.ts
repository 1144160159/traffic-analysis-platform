import { api } from '@/services/api';

export type AlertBatchAssignmentItem = {
  alertId: string;
  stateVersion?: number;
};

export type AlertBatchAssignmentResult = {
  batchId: string;
  status: string;
  total: number;
  accepted: number;
  replayed: boolean;
};

export type AlertBatchAssignmentJob = AlertBatchAssignmentResult & {
  applied: number;
  conflicted: number;
  forbidden: number;
  failed: number;
  items: Array<{
    alertId: string;
    position: number;
    expectedStateVersion: number;
    resultingStateVersion: number;
    status: string;
    errorCode: string;
    errorMessage: string;
  }>;
};

const terminalAlertBatchStatuses = new Set(['completed', 'partial', 'failed', 'cancelled']);
const knownAlertBatchStatuses = new Set(['accepted', 'running', ...terminalAlertBatchStatuses]);

export async function batchAssignAlerts(alertIds: string[], assignee: string) {
  const targets = alertIds.filter(Boolean);
  if (!targets.length) throw new Error('请至少勾选一条告警');
  if (!assignee.trim()) throw new Error('请输入指派对象');
  const settled = await Promise.allSettled(targets.map((alertId) => api.put(`/v1/alerts/${encodeURIComponent(alertId)}/assign`, { assignee: assignee.trim() })));
  const failed = settled.filter((item) => item.status === 'rejected').length;
  if (failed) throw new Error(`批量指派失败 ${failed} 条，成功 ${targets.length - failed} 条`);
  return { total: targets.length, success: targets.length };
}

export async function createDurableAlertBatchAssignment(
  items: AlertBatchAssignmentItem[],
  assignee: string,
  reason: string,
  snapshotId: string,
  operationId: string,
): Promise<AlertBatchAssignmentResult> {
  const frozenItems = items.map((item) => ({
    alert_id: item.alertId.trim(),
    state_version: Number(item.stateVersion),
  }));
  if (!snapshotId.trim()) throw new Error('当前告警快照缺少 selection snapshot，不能批量指派');
  if (!operationId.trim()) throw new Error('批量指派缺少稳定操作标识');
  if (!frozenItems.length || frozenItems.some((item) => !item.alert_id || !Number.isFinite(item.state_version) || item.state_version <= 0)) {
    throw new Error('所选告警缺少有效 state revision，请刷新后重试');
  }
  if (!assignee.trim() || reason.trim().length < 4) throw new Error('指派对象和至少 4 个字符的原因必填');

  const selection = await api.post('/v1/alerts/batches/selections', {
    snapshot_id: snapshotId.trim(),
    items: frozenItems,
  }, {
    headers: { 'Idempotency-Key': `${operationId}:selection` },
  });
  const selectionPayload = unwrapEnvelope(selection.data);
  const selectionToken = stringAt(selectionPayload, ['selection_token', 'selectionToken']);
  if (!selectionToken) throw new Error('服务端未返回批量选择令牌');

  const assignment = await api.post('/v1/alerts/batches/assign', {
    selection_token: selectionToken,
    assignee: assignee.trim(),
    reason: reason.trim(),
  }, {
    headers: { 'Idempotency-Key': `${operationId}:assignment` },
  });
  const payload = unwrapEnvelope(assignment.data);
  const batchId = stringAt(payload, ['batch_id', 'batchId']);
  const status = stringAt(payload, ['status']);
  const total = numberAt(payload, ['total_count', 'totalCount']);
  const accepted = numberAt(payload, ['accepted_count', 'acceptedCount']);
  if (!batchId || status !== 'accepted' || total !== frozenItems.length || accepted !== total) {
    throw new Error('服务端批量指派受理回执不完整，请查询任务状态后重试');
  }
  return {
    batchId,
    status,
    total,
    accepted,
    replayed: Boolean(valueAt(payload, ['replayed'])),
  };
}

export async function getDurableAlertBatchAssignment(batchId: string): Promise<AlertBatchAssignmentJob> {
  const normalizedBatchId = batchId.trim();
  if (!normalizedBatchId) throw new Error('批量指派任务标识不能为空');
  const response = await api.get(`/v1/alerts/batches/assign/${encodeURIComponent(normalizedBatchId)}`);
  const payload = unwrapEnvelope(response.data);
  const returnedBatchId = stringAt(payload, ['batch_id', 'batchId']);
  const status = stringAt(payload, ['status']);
  const total = numberAt(payload, ['total_count', 'totalCount']);
  const accepted = numberAt(payload, ['accepted_count', 'acceptedCount']);
  const applied = numberAt(payload, ['applied_count', 'appliedCount']);
  const conflicted = numberAt(payload, ['conflicted_count', 'conflictedCount']);
  const forbidden = numberAt(payload, ['forbidden_count', 'forbiddenCount']);
  const failed = numberAt(payload, ['failed_count', 'failedCount']);
  if (returnedBatchId !== normalizedBatchId || !knownAlertBatchStatuses.has(status) || total < 1 || accepted + applied + conflicted + forbidden + failed > total) {
    throw new Error('服务端批量指派任务状态不完整，请保留任务标识并联系管理员');
  }
  const rawItems = valueAt(payload, ['items']);
  const items = Array.isArray(rawItems) ? rawItems.filter(isRecord).map((item) => ({
    alertId: stringAt(item, ['alert_id', 'alertId']),
    position: numberAt(item, ['position']),
    expectedStateVersion: numberAt(item, ['expected_state_version', 'expectedStateVersion']),
    resultingStateVersion: numberAt(item, ['resulting_state_version', 'resultingStateVersion']),
    status: stringAt(item, ['status']),
    errorCode: stringAt(item, ['error_code', 'errorCode']),
    errorMessage: stringAt(item, ['error_message', 'errorMessage']),
  })) : [];
  if (items.length !== total || items.some((item) => !item.alertId || item.expectedStateVersion <= 0)) {
    throw new Error('服务端批量指派逐项回执不完整，请保留任务标识并联系管理员');
  }
  return {
    batchId: returnedBatchId,
    status,
    total,
    accepted,
    applied,
    conflicted,
    forbidden,
    failed,
    replayed: Boolean(valueAt(payload, ['replayed'])),
    items,
  };
}

export async function waitForDurableAlertBatchAssignment(
  batchId: string,
  options: { intervalMs?: number; maxAttempts?: number; signal?: AbortSignal } = {},
): Promise<AlertBatchAssignmentJob> {
  const intervalMs = Math.max(0, options.intervalMs ?? 1_000);
  const maxAttempts = Math.max(1, Math.min(options.maxAttempts ?? 30, 120));
  let latest: AlertBatchAssignmentJob | undefined;
  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    if (options.signal?.aborted) throw new DOMException('批量指派状态查询已取消', 'AbortError');
    latest = await getDurableAlertBatchAssignment(batchId);
    if (terminalAlertBatchStatuses.has(latest.status)) return latest;
    if (attempt + 1 < maxAttempts && intervalMs > 0) {
      await new Promise<void>((resolve, reject) => {
        const timer = window.setTimeout(resolve, intervalMs);
        options.signal?.addEventListener('abort', () => {
          window.clearTimeout(timer);
          reject(new DOMException('批量指派状态查询已取消', 'AbortError'));
        }, { once: true });
      });
    }
  }
  if (!latest) throw new Error('批量指派任务状态不可用');
  return latest;
}

const unwrapEnvelope = (value: unknown): unknown => {
  if (!isRecord(value)) return value;
  return 'data' in value ? unwrapEnvelope(value.data) : value;
};

const valueAt = (value: unknown, keys: string[]) => {
  if (!isRecord(value)) return undefined;
  for (const key of keys) if (key in value) return value[key];
  return undefined;
};

const stringAt = (value: unknown, keys: string[]) => String(valueAt(value, keys) ?? '');

const numberAt = (value: unknown, keys: string[]) => {
  const number = Number(valueAt(value, keys));
  return Number.isFinite(number) ? number : 0;
};

const isRecord = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);

export type AlertExportFilters = {
  status?: string;
  sourceIp?: string;
  ruleVersion?: string;
  modelVersion?: string;
  attackPhase?: string;
  assetIp?: string;
  destinationIp?: string;
  minScore?: number;
  startTime?: number;
  endTime?: number;
};

export async function exportAlertQueueCsv(filters: AlertExportFilters) {
  const response = await api.post('/v1/alerts/export/csv', {
    status: filters.status ? [filters.status] : [],
    src_ip: filters.sourceIp ?? '',
    rule_version: filters.ruleVersion ?? '',
    model_version: filters.modelVersion ?? '',
    attack_phase: filters.attackPhase ?? '',
    asset_ip: filters.assetIp ?? '',
    dst_ip: filters.destinationIp ?? '',
    min_score: filters.minScore ?? 0,
    start_time: filters.startTime,
    end_time: filters.endTime,
    max_count: 10_000,
  }, { responseType: 'blob' });
  const url = URL.createObjectURL(response.data);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `alerts-${new Date().toISOString().slice(0, 10)}.csv`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

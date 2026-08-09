import { api } from '@/services/api';

export type PlaybookStage = 'draft' | 'approval_pending' | 'approved' | 'rejected';
export type PlaybookRisk = 'low' | 'medium' | 'high' | 'critical';

export type PlaybookTrigger = {
  alert_type: string;
  severity_min: string;
  score_min: number;
  source_ips?: string[];
};

export type PlaybookCondition = { field: string; operator: string; value: string };
export type PlaybookAction = { type: string; parameters: Record<string, unknown>; timeout: number };

export type PlaybookDefinition = {
  name: string;
  description: string;
  enabled: boolean;
  trigger: PlaybookTrigger;
  actions: PlaybookAction[];
  conditions?: PlaybookCondition[];
  cooldown: number;
  max_runs: number;
  run_count: number;
  approval_policy: { required: boolean; minimum_role: string; two_person_rule: boolean };
  rollback_policy: { supported: boolean; automatic: boolean };
};

export type PlaybookDefinitionRecord = {
  tenant_id: string;
  name: string;
  display_name: string;
  description: string;
  version: number;
  stage: PlaybookStage;
  enabled: boolean;
  risk_level: PlaybookRisk;
  definition: PlaybookDefinition;
  created_by: string;
  submitted_by?: string;
  approved_by?: string;
  rejection_reason?: string;
  created_at: string;
  updated_at: string;
};

export type PlaybookExecutionRecord = {
  execution_id: string;
  tenant_id: string;
  playbook_name: string;
  alert_id: string;
  success_actions: number;
  failed_actions: number;
  duration_ms: number;
  request_payload: Record<string, unknown>;
  result: { actions?: Array<{ action_type: string; simulated: boolean; message?: string; error?: string }> };
  mode: string;
  status: string;
  rollback_of?: string;
  effect: Record<string, unknown>;
  requested_by: string;
  rolled_back_at?: string;
  created_at: string;
  playbook_version?: number;
  workflow_revision?: number;
  approval_status?: 'not_required' | 'pending' | 'approved' | 'rejected' | 'cancelled';
  executor_status?: string;
  execution_receipt?: Record<string, unknown>;
  compensation_receipt?: Record<string, unknown>;
  error_message?: string;
  approved_by?: string;
  updated_at?: string;
  completed_at?: string;
  trace_id?: string;
};

export type PlaybookExecutionOperation = 'approve' | 'reject' | 'cancel' | 'compensate';

export type PlaybookAuditRecord = {
  event_id: string;
  action: string;
  object_id: string;
  detail: Record<string, unknown>;
  created_at: string;
};

export type PlaybookWorkbench = {
  definition: PlaybookDefinitionRecord;
  executions: PlaybookExecutionRecord[];
  audits: PlaybookAuditRecord[];
};

type Envelope<T> = { success: boolean; data: T };

export async function fetchPlaybookCatalog(): Promise<PlaybookDefinitionRecord[]> {
  const response = await api.get<Envelope<{ playbooks: PlaybookDefinitionRecord[]; total: number }>>('/v1/playbooks/catalog');
  return response.data.data.playbooks ?? [];
}

export async function fetchPlaybookWorkbench(name: string): Promise<PlaybookWorkbench> {
  const response = await api.get<Envelope<PlaybookWorkbench>>(`/v1/playbooks/${encodeURIComponent(name)}/workbench`);
  return response.data.data;
}

export type SavePlaybookDraftInput = {
  name: string;
  expectedVersion: number;
  displayName: string;
  description: string;
  definition: PlaybookDefinition;
  create?: boolean;
};

export async function savePlaybookDraft(input: SavePlaybookDraftInput): Promise<PlaybookDefinitionRecord> {
  const body = {
    expected_version: input.expectedVersion,
    display_name: input.displayName,
    description: input.description,
    definition: { ...input.definition, name: input.name, description: input.description, enabled: false },
  };
  const endpoint = input.create ? '/v1/playbooks' : `/v1/playbooks/${encodeURIComponent(input.name)}/draft`;
  const response = input.create
    ? await api.post<Envelope<PlaybookDefinitionRecord>>(endpoint, body)
    : await api.put<Envelope<PlaybookDefinitionRecord>>(endpoint, body);
  return response.data.data;
}

export async function transitionPlaybook(name: string, action: 'submit-approval' | 'approve' | 'reject', expectedVersion: number, reason = ''): Promise<PlaybookDefinitionRecord> {
  const response = await api.post<Envelope<PlaybookDefinitionRecord>>(
    `/v1/playbooks/${encodeURIComponent(name)}/${action}`,
    { expected_version: expectedVersion, reason },
  );
  return response.data.data;
}

export async function setPlaybookEnabled(name: string, enabled: boolean, expectedVersion: number): Promise<PlaybookDefinitionRecord> {
  const response = await api.patch<Envelope<PlaybookDefinitionRecord>>(
    `/v1/playbooks/${encodeURIComponent(name)}`,
    { enabled, expected_version: expectedVersion },
  );
  return response.data.data;
}

export async function drillPlaybook(name: string, expectedVersion: number): Promise<PlaybookExecutionRecord> {
  const response = await api.post<Envelope<PlaybookExecutionRecord>>(
    `/v1/playbooks/${encodeURIComponent(name)}/drill`,
    { expected_version: expectedVersion },
  );
  const execution = response.data.data;
  if (execution.mode !== 'drill') throw new Error('服务端未返回可验证的演练记录');
  const actions = execution.result?.actions ?? [];
  if (actions.some((action) => action.simulated !== true)) throw new Error('演练结果包含未标记为 simulated 的动作');
  return execution;
}

export async function requestPlaybookExecution(
  name: string,
  expectedVersion: number,
  reason: string,
  alertContext: Record<string, unknown>,
): Promise<PlaybookExecutionRecord> {
  if (!Number.isSafeInteger(expectedVersion) || expectedVersion <= 0 || reason.trim().length < 8) {
    throw new Error('实行动作需要当前正整数版本和至少 8 个字符的原因');
  }
  const body = { expected_version: expectedVersion, reason: reason.trim(), alert_context: alertContext };
  const response = await api.request<Envelope<PlaybookExecutionRecord>>({
    url: `/v1/playbooks/${encodeURIComponent(name)}/execute`,
    method: 'POST',
    data: body,
    headers: { 'Idempotency-Key': playbookExecutionIdempotencyKey(name, 'request', expectedVersion, body) },
  });
  return validatePlaybookExecution(response.data.data);
}

export async function getPlaybookExecution(executionId: string): Promise<PlaybookExecutionRecord> {
  const response = await api.get<Envelope<PlaybookExecutionRecord>>(`/v1/playbooks/executions/${encodeURIComponent(executionId)}`);
  return validatePlaybookExecution(response.data.data, executionId);
}

export async function applyPlaybookExecutionOperation(
  executionId: string,
  operation: PlaybookExecutionOperation,
  expectedRevision: number,
  reason: string,
): Promise<PlaybookExecutionRecord> {
  if (!Number.isSafeInteger(expectedRevision) || expectedRevision <= 0 || reason.trim().length < 8) {
    throw new Error('执行控制需要当前正整数 revision 和至少 8 个字符的原因');
  }
  const endpoint = operation === 'approve' || operation === 'reject'
    ? `/v1/playbooks/executions/${encodeURIComponent(executionId)}/approval`
    : `/v1/playbooks/executions/${encodeURIComponent(executionId)}/${operation}`;
  const body = operation === 'approve' || operation === 'reject'
    ? { decision: operation, expected_revision: expectedRevision, reason: reason.trim() }
    : { expected_revision: expectedRevision, reason: reason.trim() };
  const response = await api.request<Envelope<PlaybookExecutionRecord>>({
    url: endpoint,
    method: 'POST',
    data: body,
    headers: { 'Idempotency-Key': playbookExecutionIdempotencyKey(executionId, operation, expectedRevision, body) },
  });
  return validatePlaybookExecution(response.data.data, executionId);
}

export async function rollbackPlaybookDrill(executionId: string, reason: string): Promise<PlaybookExecutionRecord> {
  const response = await api.post<Envelope<PlaybookExecutionRecord>>(
    `/v1/playbooks/executions/${encodeURIComponent(executionId)}/rollback`,
    { reason },
  );
  return response.data.data;
}

export async function downloadPlaybookEvidence(): Promise<{ blob: Blob; filename: string }> {
  const response = await api.get<Blob>('/v1/playbooks/evidence/export', { responseType: 'blob' });
  const disposition = String(response.headers['content-disposition'] ?? '');
  const filename = disposition.match(/filename="?([^";]+)"?/i)?.[1] ?? 'playbook-evidence.json';
  return { blob: response.data, filename };
}

export const newPlaybookDraft = (): PlaybookDefinition => ({
  name: 'new-response-playbook',
  description: '新建的安全响应演练剧本',
  enabled: false,
  trigger: { alert_type: 'scan', severity_min: 'high', score_min: 0.8 },
  conditions: [{ field: 'alert_count', operator: 'gt', value: '3' }],
  actions: [
    { type: 'capture_pcap', parameters: { duration: '300s' }, timeout: 30_000_000_000 },
    { type: 'notify', parameters: { channel: 'security-operations' }, timeout: 5_000_000_000 },
  ],
  cooldown: 1_800_000_000_000,
  max_runs: 5,
  run_count: 0,
  approval_policy: { required: true, minimum_role: '安全运营组（L2）', two_person_rule: true },
  rollback_policy: { supported: true, automatic: false },
});

const validatePlaybookExecution = (execution: PlaybookExecutionRecord, expectedId?: string): PlaybookExecutionRecord => {
  if (!execution?.execution_id || execution.mode !== 'live' || !execution.playbook_name ||
      !Number.isSafeInteger(execution.playbook_version) || Number(execution.playbook_version) <= 0 ||
      !Number.isSafeInteger(execution.workflow_revision) || Number(execution.workflow_revision) <= 0) {
    throw new Error('服务端未返回可验证的实行动作工作流');
  }
  if (expectedId && execution.execution_id !== expectedId) throw new Error('执行状态响应与请求资源不一致');
  if (['completed', 'partial', 'compensated'].includes(execution.status) &&
      !Object.keys(execution.status === 'compensated' ? execution.compensation_receipt ?? {} : execution.execution_receipt ?? {}).length) {
    throw new Error('实行动作终态缺少 provider 步骤回执');
  }
  return execution;
};

const playbookExecutionIdempotencyKey = (
  identity: string,
  operation: string,
  revision: number,
  body: Record<string, unknown>,
) => {
  const material = `${identity}:${operation}:${revision}:${stableJSONStringify(body)}`;
  let hash = 2166136261;
  for (let index = 0; index < material.length; index += 1) {
    hash ^= material.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return `playbook-v2:${operation}:${revision}:${(hash >>> 0).toString(16).padStart(8, '0')}`;
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

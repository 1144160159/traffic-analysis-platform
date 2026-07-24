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
};

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

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
};

export type CampaignActionAuditRecord = {
  actionId: CampaignActionId;
  event: string;
  method: EndpointMethod;
  endpoint: string;
  requiredScopes: string[];
  requestBody: Record<string, unknown>;
  jobId: string;
  status: 'completed';
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
  status: 'completed';
  jobStatus: 'completed';
  mode: 'server-persisted-read' | 'server-persisted-mutation';
  result: Record<string, unknown>;
};

type CampaignActionServerResponse = {
  action_id?: string;
  audit_event?: string;
  endpoint?: string;
  job_id?: string;
  status?: 'completed';
  job_status?: 'completed';
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
]);

export async function submitCampaignAction(input: CampaignActionInput): Promise<CampaignActionResult> {
  const plan = getPageActionPlan('campaigns', input.actionId);
  if (!plan) throw new Error(`未找到 Campaign 动作契约：${input.actionId}`);

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
  };
  const response = await api.request<{ data?: CampaignActionServerResponse } & CampaignActionServerResponse>({
    url: endpoint,
    method: plan.method,
    data: requestBody,
  });
  const serverResult = response.data.data ?? response.data;
  if (serverResult.simulation !== !isMutation || serverResult.dry_run !== !isMutation || serverResult.status !== 'completed' || serverResult.job_status !== 'completed') {
    throw new Error('Campaign 动作服务未返回已持久化并审计的完成结果');
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
    status: 'completed',
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
    status: 'completed',
    jobStatus: 'completed',
    mode: isMutation ? 'server-persisted-mutation' : 'server-persisted-read',
    result: serverResult.result ?? {},
  };
}

const stringMetadata = (metadata: Record<string, unknown> | undefined, key: string) => {
  const value = metadata?.[key];
  return typeof value === 'string' ? value : undefined;
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

import { getPageActionPlan } from '@/services/pageApiPlans';
import { getAuthToken } from '@/services/authStorage';

export type MlopsActionId =
  | 'mlops-pipeline-create'
  | 'mlops-training-submit'
  | 'mlops-task-retry'
  | 'mlops-task-stop'
  | 'mlops-label-request'
  | 'mlops-model-register'
  | 'mlops-task-inspect'
  | 'mlops-context-action'
  | 'mlops-feedback-inspect'
  | 'mlops-model-version-inspect';

export async function submitMlopsAction(input: { actionId: MlopsActionId; targetId: string; target: string; version?: string }) {
  const plan = getPageActionPlan('mlops', input.actionId);
  if (!plan) throw new Error(`未找到 MLOps 动作契约：${input.actionId}`);
  const token = getAuthToken();
  if (token) {
    const permissions = readJwtPermissions(token);
    const allowed = (plan.acceptedScopes ?? plan.requiredScopes).some((scope) => permissions.includes(scope) || (scope.endsWith(':*') && permissions.some((permission) => permission.startsWith(scope.slice(0, -1)))));
    if (!allowed) throw new Error(`缺少 MLOps 动作权限：${plan.requiredScopes.join(', ')}`);
  }
  await new Promise<void>((resolve) => globalThis.setTimeout(resolve, 180));
  const endpoint = plan.endpoint.replace('{id}', encodeURIComponent(input.targetId)).replace('{version}', encodeURIComponent(input.version ?? 'current'));
  const requestBody = { ...(plan.defaultBody ?? {}), target_id: input.targetId, version: input.version, target: input.target, simulation: true };
  const jobId = `SIM-MLOPS-${Date.now()}`;
  const auditRecord = { event: plan.auditEvent, endpoint, requiredScopes: plan.requiredScopes, requestBody, jobId, status: 'queued', targetId: input.targetId, target: input.target, timestamp: new Date().toISOString() };
  if (typeof window !== 'undefined') {
    let entries: unknown[] = [];
    try {
      const stored = JSON.parse(sessionStorage.getItem('taf:mlops-action-audit') ?? '[]') as unknown;
      if (Array.isArray(stored)) entries = stored;
    } catch {
      entries = [];
    }
    sessionStorage.setItem('taf:mlops-action-audit', JSON.stringify([...entries.slice(-19), auditRecord]));
  }
  return {
    actionId: input.actionId,
    endpoint,
    auditEvent: plan.auditEvent,
    auditRecord,
    requestBody,
    jobId,
    status: 'queued' as const,
  };
}

const readJwtPermissions = (token: string): string[] => {
  try {
    const payload = JSON.parse(atob(token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/'))) as { permissions?: unknown };
    return Array.isArray(payload.permissions) ? payload.permissions.filter((item): item is string => typeof item === 'string') : [];
  } catch {
    return [];
  }
};

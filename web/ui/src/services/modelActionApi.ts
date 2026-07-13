import { getPageActionPlan } from '@/services/pageApiPlans';

export type ModelActionId =
  | 'model-version-register'
  | 'model-version-activate'
  | 'model-version-deprecate'
  | 'model-feedback-append'
  | 'model-retrain-request'
  | 'model-evaluation-request'
  | 'model-version-rollback'
  | 'model-context-action';

export type ModelActionInput = {
  actionId: ModelActionId;
  modelId: string;
  version: string;
  target: string;
};

export type ModelActionResult = {
  actionId: ModelActionId;
  apiContract: string;
  auditEvent: string;
  jobId: string;
  mode: 'simulated';
  status: 'queued';
  target: string;
  requestBody: Record<string, unknown>;
  auditRecord: { event: string; modelId: string; target: string; timestamp: string };
};

export async function submitModelAction({ actionId, modelId, version, target }: ModelActionInput): Promise<ModelActionResult> {
  const plan = getPageActionPlan('models', actionId);
  if (!plan) throw new Error(`未找到模型动作契约：${actionId}`);

  await new Promise<void>((resolve) => globalThis.setTimeout(resolve, 180));
  const now = new Date().toISOString();
  const timestamp = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14);
  const result: ModelActionResult = {
    actionId,
    apiContract: plan.endpoint
      .replace('{id}', encodeURIComponent(modelId))
      .replace('{version}', encodeURIComponent(version)),
    auditEvent: plan.auditEvent,
    jobId: `SIM-MODEL-${timestamp}`,
    mode: 'simulated',
    status: 'queued',
    target,
    requestBody: { ...(plan.defaultBody ?? {}), model_id: modelId, version, target, simulation: true },
    auditRecord: { event: plan.auditEvent, modelId, target, timestamp: now },
  };
  if (typeof window !== 'undefined') {
    let entries: unknown[] = [];
    try {
      const stored = JSON.parse(window.sessionStorage.getItem('taf:model-action-audit') ?? '[]') as unknown;
      if (Array.isArray(stored)) entries = stored;
    } catch {
      entries = [];
    }
    window.sessionStorage.setItem('taf:model-action-audit', JSON.stringify([...entries.slice(-19), result.auditRecord]));
  }
  return result;
}

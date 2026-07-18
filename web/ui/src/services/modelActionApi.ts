import { getPageActionPlan } from '@/services/pageApiPlans';
import { api } from '@/services/api';

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
  payload?: Record<string, unknown>;
};

export type ModelActionResult = {
  actionId: ModelActionId;
  apiContract: string;
  auditEvent: string;
  jobId: string;
  mode: 'live';
  status: string;
  target: string;
  requestBody: Record<string, unknown>;
  auditRecord: { event: string; modelId: string; target: string; timestamp: string };
};

export function buildModelActionRequestBody({ actionId, version, target, payload }: ModelActionInput): Record<string, unknown> {
  const plan = getPageActionPlan('models', actionId);
  if (!plan) throw new Error(`未找到模型动作契约：${actionId}`);

  return {
    ...(plan.defaultBody ?? {}),
    action: actionSlug(actionId),
    target,
    version,
    ...(payload ?? {}),
  };
}

export async function submitModelAction(input: ModelActionInput): Promise<ModelActionResult> {
  const { actionId, modelId, version, target } = input;
  const plan = getPageActionPlan('models', actionId);
  if (!plan) throw new Error(`未找到模型动作契约：${actionId}`);

  const endpoint = plan.endpoint
    .replace('{id}', encodeURIComponent(modelId))
    .replace('{version}', encodeURIComponent(version));
  const requestBody = buildModelActionRequestBody(input);
  const response = await api.request<{ success?: boolean; data?: Record<string, unknown>; message?: string }>({
    method: plan.method,
    url: endpoint,
    data: requestBody,
  });
  const responsePayload = response.data.data ?? {};
  const now = new Date().toISOString();
  const result: ModelActionResult = {
    actionId,
    apiContract: endpoint,
    auditEvent: plan.auditEvent,
    jobId: String(responsePayload.job_id ?? responsePayload.action_id ?? responsePayload.model_version ?? `HTTP-${response.status}`),
    mode: 'live',
    status: String(responsePayload.status ?? (response.status === 202 ? 'queued' : 'completed')),
    target,
    requestBody,
    auditRecord: { event: plan.auditEvent, modelId, target, timestamp: now },
  };
  return result;
}

const actionSlug = (actionId: ModelActionId) => ({
  'model-version-register': 'register-version',
  'model-version-activate': 'activate-version',
  'model-version-deprecate': 'deprecate-version',
  'model-feedback-append': 'append-feedback-samples',
  'model-retrain-request': 'request-retraining',
  'model-evaluation-request': 'request-evaluation',
  'model-version-rollback': 'rollback-version',
  'model-context-action': 'inspect-context',
}[actionId]);

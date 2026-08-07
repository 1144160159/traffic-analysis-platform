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

const VERSIONED_MODEL_ACTIONS = new Set<ModelActionId>([
  'model-version-activate',
  'model-version-deprecate',
  'model-evaluation-request',
  'model-version-rollback',
]);

const placeholderReference = (value: string) => /^(?:selected-model|current|unknown|v-next|previous-active)$/i.test(value.trim());

const requiredPayloadText = (payload: Record<string, unknown>, key: string, label: string) => {
  const value = String(payload[key] ?? '').trim();
  if (!value || placeholderReference(value)) throw new Error(`${label}必须来自权威数据，不能使用占位值`);
  return value;
};

export function modelActionRequiresVersion(actionId: ModelActionId): boolean {
  return VERSIONED_MODEL_ACTIONS.has(actionId);
}

export function validateModelActionInput(input: ModelActionInput): void {
  const modelId = input.modelId.trim();
  const target = input.target.trim();
  const version = input.version.trim();
  const payload = input.payload ?? {};
  if (!modelId || placeholderReference(modelId)) throw new Error('模型动作缺少权威 model_id');
  if (!target || placeholderReference(target)) throw new Error('模型动作缺少明确目标');
  if (modelActionRequiresVersion(input.actionId) && (!version || placeholderReference(version))) {
    throw new Error('模型动作缺少权威 model_version');
  }
  switch (input.actionId) {
    case 'model-version-register':
      requiredPayloadText(payload, 'version', '模型版本');
      requiredPayloadText(payload, 'artifact_uri', '模型制品 URI');
      requiredPayloadText(payload, 'feature_set_id', '特征集 ID');
      requiredPayloadText(payload, 'model_type', '模型类型');
      break;
    case 'model-feedback-append': {
      requiredPayloadText(payload, 'dataset_id', '反馈数据集 ID');
      const sampleCount = Number(payload.sample_count);
      if (!Number.isInteger(sampleCount) || sampleCount <= 0) throw new Error('反馈样本数必须是正整数');
      break;
    }
    case 'model-retrain-request':
      requiredPayloadText(payload, 'dataset_id', '训练数据集 ID');
      if (!['incremental', 'full'].includes(String(payload.strategy ?? ''))) throw new Error('重训策略必须是 incremental 或 full');
      requiredPayloadText(payload, 'reason', '重训原因');
      break;
    case 'model-evaluation-request':
      requiredPayloadText(payload, 'dataset_id', '评估数据集 ID');
      break;
    case 'model-version-rollback':
      requiredPayloadText(payload, 'reason', '回滚原因');
      break;
    default:
      break;
  }
}

export function buildModelActionRequestBody({ actionId, version, target, payload }: ModelActionInput): Record<string, unknown> {
  const plan = getPageActionPlan('models', actionId);
  if (!plan) throw new Error(`未找到模型动作契约：${actionId}`);

  return {
    ...(plan.defaultBody ?? {}),
    ...(payload ?? {}),
    action: actionSlug(actionId),
    target: target.trim(),
    ...(version.trim() ? { version: version.trim() } : {}),
    ...(actionId === 'model-version-rollback' ? { target_version: version.trim() } : {}),
  };
}

export async function submitModelAction(input: ModelActionInput): Promise<ModelActionResult> {
  validateModelActionInput(input);
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
  const jobId = String(responsePayload.job_id ?? responsePayload.action_id ?? responsePayload.model_version ?? '').trim();
  if (!jobId) throw new Error('模型动作响应缺少稳定 job_id/action_id/model_version，不能确认已受理');
  const now = new Date().toISOString();
  const result: ModelActionResult = {
    actionId,
    apiContract: endpoint,
    auditEvent: plan.auditEvent,
    jobId,
    mode: 'live',
    status: String(responsePayload.status ?? (response.status === 202 ? 'accepted' : 'unknown')),
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

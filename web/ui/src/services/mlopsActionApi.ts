import { api } from '@/services/api';
import { getAuthToken } from '@/services/authStorage';
import { fetchModelWorkbench, type ModelWorkbench } from '@/services/modelWorkbenchApi';
import { getPageActionPlan } from '@/services/pageApiPlans';

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

export type MlopsModelRecord = {
  model_id: string;
  tenant_id: string;
  name: string;
  model_type: string;
};

export type MlopsWorkflow = {
  name: string;
  namespace: string;
  phase: string;
  progress: string;
  message?: string;
  started_at?: string;
  finished_at?: string;
  created_at?: string;
  workflow_template?: string;
  parameters?: Record<string, string>;
  can_stop: boolean;
  can_retry: boolean;
};

export type MlopsWorkspace = {
  model: MlopsModelRecord;
  workbench: ModelWorkbench;
  workflows: MlopsWorkflow[];
  orchestrator: Record<string, unknown>;
  conditions: Record<string, unknown>;
};

export type MlopsActionInput = {
  actionId: MlopsActionId;
  modelId: string;
  targetId: string;
  target: string;
  version?: string;
  featureSetId?: string;
  artifactUri?: string;
  payload?: Record<string, unknown>;
};

export type MlopsActionRequest = {
  endpoint: string;
  method: 'GET' | 'POST';
  body?: Record<string, unknown>;
  supported: boolean;
};

export type MlopsActionResult = {
  actionId: MlopsActionId;
  endpoint: string;
  auditEvent: string;
  requestBody?: Record<string, unknown>;
  jobId: string;
  status: string;
  data: unknown;
};

export async function fetchMlopsWorkspace(): Promise<MlopsWorkspace> {
  const modelsResponse = await api.get<{ success: boolean; data: MlopsModelRecord[] }>('/v1/models', {
    params: { limit: 100, offset: 0, order_by: 'updated_at', order_dir: 'desc' },
  });
  const models = modelsResponse.data.data ?? [];
  const model = models.find((item) => item.name === 'behavior-classifier') ?? models[0];
  if (!model?.model_id) throw new Error('没有可用于 MLOps 编排的租户模型');

  const [workbench, statusResponse, conditionsResponse, workflowsResponse] = await Promise.all([
    fetchModelWorkbench(model.model_id),
    api.get<{ success: boolean; data: Record<string, unknown> }>('/v1/mlops/status'),
    api.get<{ success: boolean; data: Record<string, unknown> }>('/v1/mlops/conditions'),
    api.get<{ success: boolean; data: MlopsWorkflow[] }>('/v1/mlops/workflows'),
  ]);
  return {
    model,
    workbench,
    workflows: workflowsResponse.data.data ?? [],
    orchestrator: statusResponse.data.data ?? {},
    conditions: conditionsResponse.data.data ?? {},
  };
}

export function buildMlopsActionRequest(input: MlopsActionInput): MlopsActionRequest {
  const workflowName = encodeURIComponent(input.targetId);
  switch (input.actionId) {
    case 'mlops-pipeline-create':
    case 'mlops-training-submit':
      return {
        endpoint: '/v1/mlops/retrain',
        method: 'POST',
        supported: true,
        body: {
          model_type: String(input.payload?.model_type ?? 'xgboost'),
          lookback_days: Number(input.payload?.lookback_days ?? 7),
          feature_set_id: input.featureSetId ?? 'v1',
          params: {
            'model-id': input.modelId,
            'trigger-reason': input.actionId === 'mlops-pipeline-create' ? 'operator_create_pipeline' : 'operator_training_submit',
          },
        },
      };
    case 'mlops-task-retry':
      return { endpoint: `/v1/mlops/workflows/${workflowName}/retry`, method: 'POST', supported: true, body: {} };
    case 'mlops-task-stop':
      return { endpoint: `/v1/mlops/workflows/${workflowName}/stop`, method: 'POST', supported: true, body: {} };
    case 'mlops-task-inspect':
    case 'mlops-context-action':
      return { endpoint: `/v1/mlops/workflows/${workflowName}`, method: 'GET', supported: true };
    case 'mlops-model-version-inspect':
      return { endpoint: `/v1/models/${encodeURIComponent(input.modelId)}/versions/${encodeURIComponent(input.version ?? '')}`, method: 'GET', supported: Boolean(input.version) };
    case 'mlops-feedback-inspect':
      return { endpoint: 'local://postgresql-feedback-sample', method: 'GET', supported: false };
    case 'mlops-label-request':
      return { endpoint: 'unavailable://label-job-service', method: 'POST', supported: false };
    case 'mlops-model-register':
      return { endpoint: 'unavailable://signed-model-candidate', method: 'POST', supported: false };
  }
}

export async function submitMlopsAction(input: MlopsActionInput): Promise<MlopsActionResult> {
  const plan = getPageActionPlan('mlops', input.actionId);
  if (!plan) throw new Error(`未找到 MLOps 动作契约：${input.actionId}`);
  const token = getAuthToken();
  if (token) {
    const permissions = readJwtPermissions(token);
    const allowed = (plan.acceptedScopes ?? plan.requiredScopes).some((scope) => permissions.some((permission) => permission === scope || permission === '*' || (permission.endsWith(':*') && scope.startsWith(permission.slice(0, -1)))));
    if (!allowed) throw new Error(`缺少 MLOps 动作权限：${plan.requiredScopes.join(', ')}`);
  }

  const request = buildMlopsActionRequest(input);
  if (!request.supported) throw new Error('该动作缺少可验证的生产执行链，当前已安全禁用');
  const responseData = request.method === 'GET'
    ? (await api.get(request.endpoint)).data
    : (await api.post(request.endpoint, request.body ?? {})).data;
  const envelope = asRecord(responseData);
  const data = asRecord(envelope.data);
  const workflow = asRecord(data.workflow);
  const auditEvent = String(data.audit_event ?? (request.method === 'GET' ? 'READ_ONLY' : 'SERVER_AUDIT_PENDING'));
  return {
    actionId: input.actionId,
    endpoint: request.endpoint,
    auditEvent,
    requestBody: request.body,
    jobId: String(data.workflow_name ?? workflow.name ?? data.model_version ?? `READ-${input.targetId}`),
    status: String(data.status ?? workflow.phase ?? (request.method === 'GET' ? 'read' : 'accepted')),
    data: responseData,
  };
}

const asRecord = (value: unknown): Record<string, unknown> => value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};

const readJwtPermissions = (token: string): string[] => {
  try {
    const payload = JSON.parse(atob(token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/'))) as { permissions?: unknown };
    return Array.isArray(payload.permissions) ? payload.permissions.filter((item): item is string => typeof item === 'string') : [];
  } catch {
    return [];
  }
};

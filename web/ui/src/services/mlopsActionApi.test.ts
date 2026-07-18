import { beforeEach, describe, expect, it, vi } from 'vitest';
import { clearAuthTokens, setAuthTokens } from './authStorage';
import { buildMlopsActionRequest, submitMlopsAction, type MlopsActionId } from './mlopsActionApi';

const mocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));

vi.mock('@/services/api', () => ({ api: { get: mocks.get, post: mocks.post } }));

const input = (actionId: MlopsActionId) => ({ actionId, modelId: 'model-1', targetId: 'mlops-manual-live', target: '训练流水线', featureSetId: 'v1' });

describe('MLOps real workflow actions', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.post.mockReset();
    mocks.get.mockResolvedValue({ data: { success: true, data: { name: 'mlops-manual-live', phase: 'Running' } } });
    mocks.post.mockResolvedValue({ data: { success: true, data: { workflow_name: 'mlops-manual-live', status: 'Running', audit_event: 'MLOPS_RETRAIN_SUBMITTED' } } });
    clearAuthTokens();
    setAuthTokens(tokenWithPermissions(['*', 'model:*']));
  });

  it.each([
    ['mlops-pipeline-create', '/v1/mlops/retrain'],
    ['mlops-training-submit', '/v1/mlops/retrain'],
    ['mlops-task-retry', '/v1/mlops/workflows/mlops-manual-live/retry'],
    ['mlops-task-stop', '/v1/mlops/workflows/mlops-manual-live/stop'],
  ] as const)('submits %s to the real Argo-backed API', async (actionId, endpoint) => {
    const result = await submitMlopsAction(input(actionId));
    expect(mocks.post).toHaveBeenCalledTimes(1);
    expect(mocks.post.mock.calls[0][0]).toBe(endpoint);
    expect(mocks.post.mock.calls[0][1]).not.toHaveProperty('simulation');
    expect(result).toMatchObject({ actionId, endpoint, jobId: 'mlops-manual-live', status: 'Running', auditEvent: 'MLOPS_RETRAIN_SUBMITTED' });
  });

  it('builds the exact retrain body shown in the confirmation drawer', () => {
    expect(buildMlopsActionRequest(input('mlops-training-submit'))).toEqual({
      endpoint: '/v1/mlops/retrain',
      method: 'POST',
      supported: true,
      body: {
        model_type: 'xgboost',
        lookback_days: 7,
        feature_set_id: 'v1',
        params: { 'model-id': 'model-1', 'trigger-reason': 'operator_training_submit' },
      },
    });
  });

  it.each(['mlops-task-inspect', 'mlops-context-action'] as const)('reads %s without creating a job', async (actionId) => {
    const result = await submitMlopsAction(input(actionId));
    expect(mocks.get).toHaveBeenCalledWith('/v1/mlops/workflows/mlops-manual-live');
    expect(mocks.post).not.toHaveBeenCalled();
    expect(result.auditEvent).toBe('READ_ONLY');
  });

  it.each(['mlops-label-request', 'mlops-model-register', 'mlops-feedback-inspect'] as const)('fails closed for unsupported %s', async (actionId) => {
    await expect(submitMlopsAction(input(actionId))).rejects.toThrow('安全禁用');
    expect(mocks.get).not.toHaveBeenCalled();
    expect(mocks.post).not.toHaveBeenCalled();
  });

  it('reads the exact displayed model version with server-side RBAC', async () => {
    mocks.get.mockResolvedValue({ data: { success: true, data: { model_version: 'v1', status: 'active' } } });
    const result = await submitMlopsAction({ ...input('mlops-model-version-inspect'), version: 'v1' });
    expect(mocks.get).toHaveBeenCalledWith('/v1/models/model-1/versions/v1');
    expect(result.status).toBe('active');
  });

  it('rejects a principal without model:write before transmitting', async () => {
    setAuthTokens(tokenWithPermissions(['model:read']));
    await expect(submitMlopsAction(input('mlops-training-submit'))).rejects.toThrow('缺少 MLOps 动作权限');
    expect(mocks.post).not.toHaveBeenCalled();
  });
});

function tokenWithPermissions(permissions: string[]) {
  const payload = btoa(JSON.stringify({ permissions })).replace(/=/g, '').replace(/\+/g, '-').replace(/\//g, '_');
  return `e30.${payload}.signature`;
}

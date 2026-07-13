import { beforeEach, describe, expect, it } from 'vitest';
import { clearAuthTokens, setAuthTokens } from './authStorage';
import { submitMlopsAction, type MlopsActionId } from './mlopsActionApi';

describe('submitMlopsAction', () => {
  beforeEach(() => { sessionStorage.clear(); clearAuthTokens(); });

  it.each([
    ['mlops-pipeline-create', '/v1/mlops/pipelines', 'MLOPS_PIPELINE_CREATED'],
    ['mlops-training-submit', '/v1/mlops/retrain', 'MLOPS_RETRAIN_REQUESTED'],
    ['mlops-task-retry', '/v1/mlops/tasks/TR-001/retry', 'MLOPS_TASK_RETRIED'],
    ['mlops-task-stop', '/v1/mlops/tasks/TR-001/stop', 'MLOPS_TASK_STOPPED'],
    ['mlops-label-request', '/v1/mlops/labels', 'MLOPS_LABELING_REQUESTED'],
    ['mlops-model-register', '/v1/models/TR-001/versions', 'MODEL_VERSION_CREATE'],
    ['mlops-task-inspect', '/v1/mlops/tasks/TR-001', 'MLOPS_TASK_VIEWED'],
    ['mlops-context-action', '/v1/mlops/tasks/TR-001/actions', 'MLOPS_CONTEXT_ACTION_REQUESTED'],
    ['mlops-feedback-inspect', '/v1/feedback/TR-001', 'MLOPS_FEEDBACK_VIEWED'],
    ['mlops-model-version-inspect', '/v1/models/TR-001/versions/current', 'MODEL_VERSION_VIEWED'],
  ] as const)('maps %s to an audited simulation', async (actionId, endpoint, auditEvent) => {
    const result = await submitMlopsAction({ actionId: actionId as MlopsActionId, targetId: 'TR-001', target: '训练任务' });
    expect(result.endpoint).toBe(endpoint);
    expect(result.auditEvent).toBe(auditEvent);
    expect(result.requestBody).toMatchObject({ target_id: 'TR-001', simulation: true });
    expect(result.auditRecord).toMatchObject({ endpoint, jobId: result.jobId, status: 'queued', requestBody: result.requestBody });
    expect(JSON.parse(sessionStorage.getItem('taf:mlops-action-audit') ?? '[]')).toContainEqual(result.auditRecord);
  });

  it('rejects an authenticated principal without a model action scope', async () => {
    const payload = btoa(JSON.stringify({ permissions: ['rule:read'] })).replace(/=/g, '').replace(/\+/g, '-').replace(/\//g, '_');
    setAuthTokens(`e30.${payload}.signature`);
    await expect(submitMlopsAction({ actionId: 'mlops-training-submit', targetId: 'TR-001', target: '训练任务' })).rejects.toThrow('缺少 MLOps 动作权限');
  });
});

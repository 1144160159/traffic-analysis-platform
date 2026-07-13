import { beforeEach, describe, expect, it } from 'vitest';
import { submitModelAction } from './modelActionApi';

describe('submitModelAction', () => {
  beforeEach(() => sessionStorage.clear());
  it('returns an asynchronous audited simulation bound to the model API contract', async () => {
    const result = await submitModelAction({
      actionId: 'model-version-rollback',
      modelId: 'model/001',
      version: 'v2.1',
      target: 'UEBA 行为分析',
    });

    expect(result.status).toBe('queued');
    expect(result.mode).toBe('simulated');
    expect(result.jobId).toMatch(/^SIM-MODEL-/);
    expect(result.apiContract).toBe('/v1/models/model%2F001/versions/v2.1/rollback');
    expect(result.auditEvent).toBe('MODEL_VERSION_ROLLED_BACK');
    expect(result.requestBody).toMatchObject({ model_id: 'model/001', version: 'v2.1', simulation: true });
    expect(result.auditRecord).toMatchObject({ event: 'MODEL_VERSION_ROLLED_BACK', modelId: 'model/001' });
    expect(JSON.parse(sessionStorage.getItem('taf:model-action-audit') ?? '[]')).toContainEqual(result.auditRecord);
  });

  it.each([
    ['model-version-register', '/v1/models/model-001/versions', 'MODEL_VERSION_CREATE'],
    ['model-version-activate', '/v1/models/model-001/versions/v2/activate', 'MODEL_VERSION_ACTIVATE'],
    ['model-version-deprecate', '/v1/models/model-001/versions/v2/deprecate', 'MODEL_VERSION_DEPRECATE'],
    ['model-feedback-append', '/v1/models/model-001/feedback-samples', 'MODEL_FEEDBACK_SAMPLES_APPENDED'],
    ['model-retrain-request', '/v1/models/model-001/retrain', 'MODEL_RETRAIN_REQUESTED'],
    ['model-evaluation-request', '/v1/models/model-001/versions/v2/evaluate', 'MODEL_EVALUATION_REQUESTED'],
    ['model-version-rollback', '/v1/models/model-001/versions/v2/rollback', 'MODEL_VERSION_ROLLED_BACK'],
    ['model-context-action', '/v1/models/model-001/actions', 'MODEL_CONTEXT_ACTION_REQUESTED'],
  ] as const)('maps %s to its endpoint and audit event', async (actionId, endpoint, auditEvent) => {
    const result = await submitModelAction({ actionId, modelId: 'model-001', version: 'v2', target: 'test-model' });
    expect(result.apiContract).toBe(endpoint);
    expect(result.auditEvent).toBe(auditEvent);
    expect(result.requestBody).toMatchObject({ model_id: 'model-001', version: 'v2', target: 'test-model', simulation: true });
    expect(JSON.parse(sessionStorage.getItem('taf:model-action-audit') ?? '[]')).toContainEqual(result.auditRecord);
  });

  it('recovers from a malformed session audit value', async () => {
    sessionStorage.setItem('taf:model-action-audit', 'not-json');
    const result = await submitModelAction({ actionId: 'model-context-action', modelId: 'model-001', version: 'v2', target: 'test-model' });
    expect(JSON.parse(sessionStorage.getItem('taf:model-action-audit') ?? '[]')).toEqual([result.auditRecord]);
  });

  it('rejects an unknown action contract', async () => {
    await expect(submitModelAction({
      actionId: 'unknown-model-action' as never,
      modelId: 'model-001',
      version: 'v2',
      target: 'test-model',
    })).rejects.toThrow('未找到模型动作契约');
  });
});

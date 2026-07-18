import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import { submitModelAction } from './modelActionApi';

describe('submitModelAction', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(api, 'request').mockResolvedValue({
      status: 202,
      data: { success: true, data: { job_id: 'job-001', status: 'queued' } },
    } as never);
  });
  it('returns a live server job bound to the model API contract', async () => {
    const result = await submitModelAction({
      actionId: 'model-version-rollback',
      modelId: 'model/001',
      version: 'v2.1',
      target: 'UEBA 行为分析',
    });

    expect(result.status).toBe('queued');
    expect(result.mode).toBe('live');
    expect(result.jobId).toBe('job-001');
    expect(result.apiContract).toBe('/v1/models/model%2F001/versions/v2.1/rollback');
    expect(result.auditEvent).toBe('MODEL_VERSION_ROLLBACK_REQUESTED');
    expect(result.requestBody).toMatchObject({ action: 'rollback-version', version: 'v2.1', target: 'UEBA 行为分析' });
    expect(result.auditRecord).toMatchObject({ event: 'MODEL_VERSION_ROLLBACK_REQUESTED', modelId: 'model/001' });
    expect(api.request).toHaveBeenCalledWith(expect.objectContaining({ method: 'POST', url: '/v1/models/model%2F001/versions/v2.1/rollback' }));
  });

  it.each([
    ['model-version-register', '/v1/models/model-001/versions', 'MODEL_VERSION_CREATE'],
    ['model-version-activate', '/v1/models/model-001/versions/v2/activate', 'MODEL_VERSION_ACTIVATE'],
    ['model-version-deprecate', '/v1/models/model-001/versions/v2/deprecate', 'MODEL_VERSION_DEPRECATE'],
    ['model-feedback-append', '/v1/models/model-001/feedback-samples', 'MODEL_FEEDBACK_INGEST_REQUESTED'],
    ['model-retrain-request', '/v1/models/model-001/retrain', 'MODEL_RETRAIN_REQUESTED'],
    ['model-evaluation-request', '/v1/models/model-001/versions/v2/evaluate', 'MODEL_EVALUATION_REQUESTED'],
    ['model-version-rollback', '/v1/models/model-001/versions/v2/rollback', 'MODEL_VERSION_ROLLBACK_REQUESTED'],
    ['model-context-action', '/v1/models/model-001/actions', 'MODEL_CONTEXT_ACTION_REQUESTED'],
  ] as const)('maps %s to its endpoint and audit event', async (actionId, endpoint, auditEvent) => {
    const result = await submitModelAction({ actionId, modelId: 'model-001', version: 'v2', target: 'test-model' });
    expect(result.apiContract).toBe(endpoint);
    expect(result.auditEvent).toBe(auditEvent);
    expect(result.requestBody).toMatchObject({ version: 'v2', target: 'test-model' });
  });

  it('merges the actual activation percentage into the submitted and displayed request body', async () => {
    const result = await submitModelAction({
      actionId: 'model-version-activate',
      modelId: 'model-001',
      version: 'v3',
      target: 'candidate',
      payload: { gray_percent: 100 },
    });
    expect(result.requestBody).toMatchObject({ action: 'activate-version', version: 'v3', gray_percent: 100 });
    expect(api.request).toHaveBeenCalledWith(expect.objectContaining({ data: expect.objectContaining({ gray_percent: 100 }) }));
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

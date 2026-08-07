import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import { buildModelActionRequestBody, submitModelAction } from './modelActionApi';

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
      payload: { reason: 'independently approved rollback' },
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
    ['model-version-register', '/v1/models/model-001/versions', 'MODEL_VERSION_CREATE', { version: 'v2', artifact_uri: 's3://models/model-001/v2.bin', feature_set_id: 'features-v2', model_type: 'xgboost' }],
    ['model-version-activate', '/v1/models/model-001/versions/v2/activate', 'MODEL_VERSION_ACTIVATE', {}],
    ['model-version-deprecate', '/v1/models/model-001/versions/v2/deprecate', 'MODEL_VERSION_DEPRECATE', {}],
    ['model-feedback-append', '/v1/models/model-001/feedback-samples', 'MODEL_FEEDBACK_INGEST_REQUESTED', { dataset_id: 'feedback-v2', sample_count: 3 }],
    ['model-retrain-request', '/v1/models/model-001/retrain', 'MODEL_RETRAIN_REQUESTED', { dataset_id: 'train-v2', strategy: 'incremental', reason: 'confirmed drift' }],
    ['model-evaluation-request', '/v1/models/model-001/versions/v2/evaluate', 'MODEL_EVALUATION_REQUESTED', { dataset_id: 'validation-v2' }],
    ['model-version-rollback', '/v1/models/model-001/versions/v2/rollback', 'MODEL_VERSION_ROLLBACK_REQUESTED', { reason: 'approved rollback' }],
    ['model-context-action', '/v1/models/model-001/actions', 'MODEL_CONTEXT_ACTION_REQUESTED', {}],
  ] as const)('maps %s to its endpoint and audit event', async (actionId, endpoint, auditEvent, payload) => {
    const result = await submitModelAction({ actionId, modelId: 'model-001', version: 'v2', target: 'test-model', payload });
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

  it('fails closed instead of dispatching placeholder model or version references', async () => {
    await expect(submitModelAction({
      actionId: 'model-context-action', modelId: 'selected-model', version: '', target: 'test-model',
    })).rejects.toThrow('权威 model_id');
    await expect(submitModelAction({
      actionId: 'model-evaluation-request', modelId: 'model-001', version: 'current', target: 'test-model', payload: { dataset_id: 'validation-v2' },
    })).rejects.toThrow('权威 model_version');
    expect(api.request).not.toHaveBeenCalled();
  });

  it('requires explicit authoritative action parameters and prevents payload identity overrides', async () => {
    await expect(submitModelAction({
      actionId: 'model-feedback-append', modelId: 'model-001', version: '', target: 'test-model', payload: {},
    })).rejects.toThrow('反馈数据集 ID');
    const body = buildModelActionRequestBody({
      actionId: 'model-version-rollback',
      modelId: 'model-001',
      version: 'v2',
      target: 'test-model',
      payload: { action: 'activate-version', target: 'another-model', version: 'v999', target_version: 'v999', reason: 'approved rollback' },
    });
    expect(body).toMatchObject({ action: 'rollback-version', target: 'test-model', version: 'v2', target_version: 'v2' });
  });

  it('rejects a 202 response that does not carry a stable operation identity', async () => {
    vi.mocked(api.request).mockResolvedValueOnce({ status: 202, data: { success: true, data: { status: 'queued' } } } as never);
    await expect(submitModelAction({
      actionId: 'model-context-action', modelId: 'model-001', version: '', target: 'test-model',
    })).rejects.toThrow('缺少稳定 job_id');
  });
});

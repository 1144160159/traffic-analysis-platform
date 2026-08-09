import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createDurableAlertBatchAssignment, getDurableAlertBatchAssignment, waitForDurableAlertBatchAssignment } from '@/services/alertQueueActionsApi';

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));
vi.mock('@/services/api', () => ({ api: { get, post, put: vi.fn() } }));

describe('durable alert batch assignment', () => {
  beforeEach(() => { post.mockReset(); get.mockReset(); });

  it('freezes a revisioned server-side selection before accepting the assignment job', async () => {
    post
      .mockResolvedValueOnce({ data: { data: { selection_token: 'selection-token-1' } } })
      .mockResolvedValueOnce({ data: { data: { batch_id: 'batch-1', status: 'accepted', total_count: 2, accepted_count: 2, replayed: false } } });

    const result = await createDurableAlertBatchAssignment(
      [{ alertId: ' alert-1 ', stateVersion: 11 }, { alertId: 'alert-2', stateVersion: 12 }],
      ' analyst-a ',
      ' approved batch assignment ',
      'alerts:snapshot:42',
      'alert-batch:operation-0001',
    );

    expect(post.mock.calls[0]).toEqual([
      '/v1/alerts/batches/selections',
      { snapshot_id: 'alerts:snapshot:42', items: [{ alert_id: 'alert-1', state_version: 11 }, { alert_id: 'alert-2', state_version: 12 }] },
      { headers: { 'Idempotency-Key': 'alert-batch:operation-0001:selection' } },
    ]);
    expect(post.mock.calls[1]).toEqual([
      '/v1/alerts/batches/assign',
      { selection_token: 'selection-token-1', assignee: 'analyst-a', reason: 'approved batch assignment' },
      { headers: { 'Idempotency-Key': 'alert-batch:operation-0001:assignment' } },
    ]);
    expect(result).toEqual({ batchId: 'batch-1', status: 'accepted', total: 2, accepted: 2, replayed: false });
  });

  it('fails before network access when any selected row lacks a positive revision', async () => {
    await expect(createDurableAlertBatchAssignment(
      [{ alertId: 'alert-1' }], 'analyst-a', 'approved reason', 'alerts:snapshot:42', 'alert-batch:operation-0002',
    )).rejects.toThrow('state revision');
    expect(post).not.toHaveBeenCalled();
  });

  it('fails closed when the 202 acceptance receipt is incomplete', async () => {
    post
      .mockResolvedValueOnce({ data: { data: { selection_token: 'selection-token-1' } } })
      .mockResolvedValueOnce({ data: { data: { status: 'accepted', total_count: 1, accepted_count: 1 } } });

    await expect(createDurableAlertBatchAssignment(
      [{ alertId: 'alert-1', stateVersion: 11 }],
      'analyst-a',
      'approved reason',
      'alerts:snapshot:42',
      'alert-batch:operation-0003',
    )).rejects.toThrow('受理回执不完整');
  });

  it('queries a complete terminal per-item receipt', async () => {
    get.mockResolvedValueOnce({ data: { data: {
      batch_id: 'batch-1', status: 'partial', total_count: 2, accepted_count: 0,
      applied_count: 1, conflicted_count: 1, forbidden_count: 0, failed_count: 0,
      items: [
        { alert_id: 'alert-1', position: 0, expected_state_version: 11, resulting_state_version: 21, status: 'applied' },
        { alert_id: 'alert-2', position: 1, expected_state_version: 12, resulting_state_version: 0, status: 'conflicted', error_code: 'REVISION_CONFLICT' },
      ],
    } } });
    await expect(getDurableAlertBatchAssignment('batch-1')).resolves.toMatchObject({
      batchId: 'batch-1', status: 'partial', total: 2, applied: 1, conflicted: 1,
    });
    expect(get).toHaveBeenCalledWith('/v1/alerts/batches/assign/batch-1');
  });

  it('polls accepted and running states until a terminal receipt without inventing success', async () => {
    const job = (status: string) => ({ data: { data: {
      batch_id: 'batch-2', status, total_count: 1,
      accepted_count: status === 'accepted' ? 1 : 0,
      applied_count: status === 'completed' ? 1 : 0,
      conflicted_count: 0, forbidden_count: 0, failed_count: 0,
      items: [{ alert_id: 'alert-1', position: 0, expected_state_version: 11, resulting_state_version: status === 'completed' ? 21 : 0, status }],
    } } });
    get.mockResolvedValueOnce(job('accepted')).mockResolvedValueOnce(job('running')).mockResolvedValueOnce(job('completed'));
    await expect(waitForDurableAlertBatchAssignment('batch-2', { intervalMs: 0, maxAttempts: 3 })).resolves.toMatchObject({ status: 'completed', applied: 1 });
    expect(get).toHaveBeenCalledTimes(3);
  });
});

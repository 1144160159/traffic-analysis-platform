import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createDurableAlertBatchAssignment } from '@/services/alertQueueActionsApi';

const { post } = vi.hoisted(() => ({ post: vi.fn() }));
vi.mock('@/services/api', () => ({ api: { post, put: vi.fn() } }));

describe('durable alert batch assignment', () => {
  beforeEach(() => post.mockReset());

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
});

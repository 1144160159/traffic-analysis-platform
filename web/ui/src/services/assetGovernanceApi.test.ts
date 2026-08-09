import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '@/services/api';
import { applyAssetGovernanceAction, createAssetGovernanceWorkOrder, listAssetGovernanceWorkOrders } from './assetGovernanceApi';

vi.mock('@/services/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }));

describe('assetGovernanceApi', () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it('creates a durable work order with stable action and idempotency key', async () => {
    vi.mocked(api.post).mockResolvedValue({ data: { data: { work_order_id: 'order-1' } } } as never);
    await createAssetGovernanceWorkOrder({
      assetID: 'asset/1', targetLifecycleState: 'isolated', owner: 'owner-a',
      dueAt: '2026-08-03T00:00:00Z', evidenceRequired: true, reason: 'verified compromise response',
      expectedAssetRevision: 7, idempotencyKey: 'asset-governance:create:key-1',
    });
    expect(api.post).toHaveBeenCalledWith('/v1/assets/asset%2F1/governance/work-orders', expect.objectContaining({
      action_id: 'asset-governance-work-order-create', expected_asset_revision: 7,
    }), { headers: { 'Idempotency-Key': 'asset-governance:create:key-1' } });
  });

  it('lists authoritative orders and submits revisioned actions', async () => {
    vi.mocked(api.get).mockResolvedValue({ data: { data: [] } } as never);
    await expect(listAssetGovernanceWorkOrders('asset-1')).resolves.toEqual([]);
    vi.mocked(api.post).mockResolvedValue({ data: { data: { work_order_id: 'order-1' } } } as never);
    await applyAssetGovernanceAction({ workOrderID: 'order-1', actionID: 'asset-governance-complete',
      expectedRevision: 3, reason: 'evidence verified before completion', evidenceRefs: ['minio://evidence/a#sha256=x'],
      idempotencyKey: 'asset-governance:complete:key-1' });
    expect(api.post).toHaveBeenCalledWith('/v1/assets/governance/work-orders/order-1/actions', expect.objectContaining({
      action_id: 'asset-governance-complete', expected_revision: 3, evidence_refs: ['minio://evidence/a#sha256=x'],
    }), { headers: { 'Idempotency-Key': 'asset-governance:complete:key-1' } });
  });
});

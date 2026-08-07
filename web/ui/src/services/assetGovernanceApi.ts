import { api } from '@/services/api';

export type AssetLifecycleState = 'candidate' | 'confirmed' | 'managed' | 'isolated' | 'retired' | 'merged';
export type AssetGovernanceStatus = 'pending_approval' | 'approved' | 'rejected' | 'executing' | 'completed' | 'failed' | 'cancelled' | 'compensated';
export type AssetGovernanceAction = 'asset-governance-approve' | 'asset-governance-reject' | 'asset-governance-start' | 'asset-governance-complete' | 'asset-governance-fail' | 'asset-governance-cancel' | 'asset-governance-compensate';

export type AssetGovernanceHistory = {
  revision: number;
  action_id: string;
  from_status: string;
  to_status: string;
  actor: string;
  reason: string;
  evidence_refs: string[];
  trace_id: string;
  created_at: string;
};

export type AssetGovernanceWorkOrder = {
  work_order_id: string;
  asset_id: string;
  action_id: 'asset-governance-work-order-create';
  source_lifecycle_state: AssetLifecycleState;
  target_lifecycle_state: AssetLifecycleState;
  target_asset_id?: string;
  current_lifecycle_state: AssetLifecycleState;
  status: AssetGovernanceStatus;
  revision: number;
  expected_asset_revision: number;
  resulting_asset_revision?: number;
  owner: string;
  requested_by: string;
  approved_by?: string;
  due_at: string;
  evidence_required: boolean;
  evidence_refs: string[];
  reason: string;
  trace_id: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  idempotent_replay?: boolean;
  history?: AssetGovernanceHistory[];
};

type Envelope<T> = { data: T };

export async function listAssetGovernanceWorkOrders(assetID: string): Promise<AssetGovernanceWorkOrder[]> {
  const response = await api.get<Envelope<AssetGovernanceWorkOrder[]>>(`/v1/assets/${encodeURIComponent(assetID)}/governance/work-orders`);
  return response.data.data;
}

export async function createAssetGovernanceWorkOrder(input: {
  assetID: string;
  targetLifecycleState: AssetLifecycleState;
  targetAssetID?: string;
  owner: string;
  dueAt: string;
  evidenceRequired: boolean;
  reason: string;
  expectedAssetRevision: number;
  idempotencyKey: string;
}): Promise<AssetGovernanceWorkOrder> {
  const response = await api.post<Envelope<AssetGovernanceWorkOrder>>(
    `/v1/assets/${encodeURIComponent(input.assetID)}/governance/work-orders`,
    {
      action_id: 'asset-governance-work-order-create',
      target_lifecycle_state: input.targetLifecycleState,
      ...(input.targetAssetID ? { target_asset_id: input.targetAssetID } : {}),
      owner: input.owner,
      due_at: input.dueAt,
      evidence_required: input.evidenceRequired,
      reason: input.reason,
      expected_asset_revision: input.expectedAssetRevision,
    },
    { headers: { 'Idempotency-Key': input.idempotencyKey } },
  );
  return response.data.data;
}

export async function applyAssetGovernanceAction(input: {
  workOrderID: string;
  actionID: AssetGovernanceAction;
  expectedRevision: number;
  reason: string;
  evidenceRefs?: string[];
  idempotencyKey: string;
}): Promise<AssetGovernanceWorkOrder> {
  const response = await api.post<Envelope<AssetGovernanceWorkOrder>>(
    `/v1/assets/governance/work-orders/${encodeURIComponent(input.workOrderID)}/actions`,
    {
      action_id: input.actionID,
      expected_revision: input.expectedRevision,
      reason: input.reason,
      evidence_refs: input.evidenceRefs ?? [],
    },
    { headers: { 'Idempotency-Key': input.idempotencyKey } },
  );
  return response.data.data;
}

export function newAssetGovernanceIdempotencyKey(action = 'command'): string {
  const randomPart = globalThis.crypto?.randomUUID?.()
    ?? `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  return `asset-governance:${action}:${randomPart}`;
}

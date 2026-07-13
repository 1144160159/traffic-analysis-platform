import { api } from '@/services/api';

export type DLQReplayFallbackRequest = {
  tenant_id?: string;
  requested_by?: string;
  approved_by: string;
  approval_id: string;
  reason: string;
  repair_summary: string;
  idempotency_key: string;
  dry_run?: boolean;
  requested_at_unix?: number;
};

export type DLQReplayAuditEntry = {
  action: string;
  actor: string;
  tenant_id: string;
  result: string;
  detail?: Record<string, unknown>;
  created_at: string;
};

export type DLQReplayFallbackResult = {
  replay_id: string;
  status: 'dry_run' | 'completed' | 'partial' | string;
  duplicate: boolean;
  tenant_id: string;
  requested_by: string;
  approved_by: string;
  approval_id: string;
  idempotency_key: string;
  reason: string;
  repair_summary: string;
  started_at: string;
  finished_at: string;
  pre_fallback_files: number;
  pre_fallback_bytes: number;
  replayed_files: number;
  failed_files: number;
  remaining_fallback_files: number;
  remaining_fallback_bytes: number;
  audit_trail: DLQReplayAuditEntry[];
  errors?: string[];
};

export const buildDLQReplayDryRunRequest = (
  payload: Omit<DLQReplayFallbackRequest, 'dry_run'> & { dry_run?: boolean },
): DLQReplayFallbackRequest => ({
  ...payload,
  dry_run: payload.dry_run ?? true,
});

export const requestDLQFallbackReplay = async (
  payload: DLQReplayFallbackRequest,
): Promise<DLQReplayFallbackResult> => {
  const response = await api.post<DLQReplayFallbackResult>(
    '/v1/dlq/replay/fallback',
    buildDLQReplayDryRunRequest(payload),
  );
  return response.data;
};

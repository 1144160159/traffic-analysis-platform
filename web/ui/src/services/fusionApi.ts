import { api } from '@/services/api';

type ApiEnvelope<T> = {
  success?: boolean;
  data?: T;
};

export type FusionConflictResolveRequest = {
  object_id: string;
  object_type?: string;
  field_name: string;
  selected_source: string;
  selected_value: string;
  strategy?: string;
  note?: string;
  rule_id?: string;
  detail?: Record<string, unknown>;
};

export type FusionConflictResolution = FusionConflictResolveRequest & {
  tenant_id: string;
  conflict_id: string;
  object_type: string;
  strategy: string;
  state_version: number;
  resolved_by: string;
  resolved_at: number;
};

export type FusionRuleUpdateRequest = {
  rule_name?: string;
  status?: string;
  strategy?: string;
  confidence_threshold?: number;
  note?: string;
  detail?: Record<string, unknown>;
};

export type FusionRuleOverride = FusionRuleUpdateRequest & {
  tenant_id: string;
  rule_id: string;
  rule_name: string;
  version: number;
  status: string;
  strategy: string;
  confidence_threshold: number;
  updated_by: string;
  updated_at: number;
};

export type FusionConflictResolveResult = {
  resolution: FusionConflictResolution;
  audit_written: boolean;
};

export type FusionRuleUpdateResult = {
  rule: FusionRuleOverride;
  audit_written: boolean;
};

const unwrap = <T>(payload: ApiEnvelope<T> | T): T => {
  if (payload && typeof payload === 'object' && 'data' in payload) {
    return (payload as ApiEnvelope<T>).data as T;
  }
  return payload as T;
};

export const resolveFusionConflict = async (
  conflictId: string,
  payload: FusionConflictResolveRequest,
): Promise<FusionConflictResolveResult> => {
  const response = await api.post<ApiEnvelope<FusionConflictResolveResult> | FusionConflictResolveResult>(
    `/v1/fusion/conflicts/${encodeURIComponent(conflictId)}/resolve`,
    payload,
  );
  return unwrap(response.data);
};

export const updateFusionRule = async (
  ruleId: string,
  payload: FusionRuleUpdateRequest,
): Promise<FusionRuleUpdateResult> => {
  const response = await api.patch<ApiEnvelope<FusionRuleUpdateResult> | FusionRuleUpdateResult>(
    `/v1/fusion/rules/${encodeURIComponent(ruleId)}`,
    payload,
  );
  return unwrap(response.data);
};

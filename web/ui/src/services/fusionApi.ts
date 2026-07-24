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
  expected_state_version: number;
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
  expected_version: number;
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
  repair_task: {
    task_id: string;
    tenant_id: string;
    conflict_id: string;
    task_type: 'fusion_conflict_repair';
    status: string;
    created_by: string;
    created_at: number;
  } | null;
  audit_written: boolean;
};

export type FusionRuleUpdateResult = {
  rule: FusionRuleOverride;
  audit_written: boolean;
};

export type FusionSource = {
  source_id: string;
  name: string;
  source_type: string;
  status: string;
  last_ingest_at: number;
  records_per_minute: number;
  error_rate: number | null;
  field_coverage: number | null;
  recent_trend: number[];
  config: Record<string, unknown>;
};

export type ThreatIntelEntry = {
  id: string;
  type: string;
  value: string;
  reputation: string;
  category: string;
  source: string;
  last_seen: string;
};

export type FusionConflictValue = { source: string; value: string; confidence: number; observed_at?: number };

export type FusionConflict = {
  tenant_id: string;
  conflict_id: string;
  object_id: string;
  object_type: string;
  field_name: string;
  source_values: FusionConflictValue[];
  source_count: number;
  confidence: number;
  severity: string;
  status: string;
  rule_id: string;
  state_version: number;
  origin: string;
  detail: Record<string, unknown>;
  detected_at: number;
  updated_at: number;
};

export type FusionAuditEvent = {
  log_id: string;
  user_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  timestamp: number;
  result: string;
  details: Record<string, unknown>;
};

export type FusionStats = {
  total_events: number;
  entities_aligned: number;
  alignment_rate: number;
  quality_metrics: {
    completeness: number;
    accuracy: number;
    freshness: number;
    duplication_rate: number;
  };
};

export type FusionWorkbench = {
  sources: FusionSource[];
  stats: FusionStats;
  rules: FusionRuleOverride[];
  pipeline_rules: FusionRuleOverride[];
  rule_total: number;
  rule_limit: number;
  rule_offset: number;
  conflicts: FusionConflict[];
  conflict_total: number;
  conflict_limit: number;
  conflict_offset: number;
  audit_events: FusionAuditEvent[];
  audit_total: number;
  audit_limit: number;
  audit_offset: number;
  entity_counts: Record<string, number>;
  pending_count: number;
  resolved_count: number;
  pending_risk_counts: { high: number; medium: number; low: number };
  threat_intel_entries: ThreatIntelEntry[];
};

export type FusionEvidencePackage = {
  filename: string;
  sha256: string;
  content_base64: string;
};

const unwrap = <T>(payload: ApiEnvelope<T> | T): T => {
  if (payload && typeof payload === 'object' && 'data' in payload) {
    return (payload as ApiEnvelope<T>).data as T;
  }
  return payload as T;
};

export type FusionWorkbenchPageRequest = {
  rulePage?: number;
  rulePageSize?: number;
  conflictPage?: number;
  conflictPageSize?: number;
  auditPage?: number;
  auditPageSize?: number;
};

export const fetchFusionWorkbench = async ({
  rulePage = 1,
  rulePageSize = 6,
  conflictPage = 1,
  conflictPageSize = 3,
  auditPage = 1,
  auditPageSize = 5,
}: FusionWorkbenchPageRequest = {}): Promise<FusionWorkbench> => {
  const params = new URLSearchParams({
    rule_limit: String(rulePageSize),
    rule_offset: String(Math.max(0, rulePage - 1) * rulePageSize),
    conflict_limit: String(conflictPageSize),
    conflict_offset: String(Math.max(0, conflictPage - 1) * conflictPageSize),
    audit_limit: String(auditPageSize),
    audit_offset: String(Math.max(0, auditPage - 1) * auditPageSize),
  });
  const [workbenchResponse, threatIntelResponse] = await Promise.all([
    api.get<ApiEnvelope<Omit<FusionWorkbench, 'threat_intel_entries'>> | Omit<FusionWorkbench, 'threat_intel_entries'>>(`/v1/fusion/workbench?${params.toString()}`),
    api.get<ApiEnvelope<ThreatIntelEntry[]> | ThreatIntelEntry[]>('/v1/threat-intel/entries?limit=12'),
  ]);
  return {
    ...unwrap(workbenchResponse.data),
    threat_intel_entries: unwrap(threatIntelResponse.data) ?? [],
  };
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

export const exportFusionEvidencePackage = async (conflictId: string): Promise<FusionEvidencePackage> => {
  const response = await api.post<ApiEnvelope<FusionEvidencePackage> | FusionEvidencePackage>('/v1/fusion/evidence-packages', {
    conflict_id: conflictId,
  });
  return unwrap(response.data);
};

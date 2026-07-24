import { api } from '@/services/api';

type ApiEnvelope<T> = { data?: T };

export type BaselineTabType = 'asset' | 'account' | 'port' | 'protocol' | 'time';

export type BehaviorMetric = {
  metric_name: string;
  unit: string;
  normal_range: [number, number];
  mean: number;
  std_dev: number;
  current_value?: number;
  deviation_score?: number;
  threshold_config: {
    warning_multiplier: number;
    alert_multiplier: number;
  };
};

export type BehaviorBaseline = {
  baseline_id: string;
  tenant_id: string;
  name: string;
  entity_type: string;
  entity_id: string;
  baseline_type: string;
  metrics: BehaviorMetric[];
  status: string;
  created_at: number;
  updated_at: number;
  version: number;
  frozen?: boolean;
  drift_watch?: boolean;
};

export type BehaviorBaselineList = {
  baselines: BehaviorBaseline[];
  total: number;
  summary: {
    scope: string;
    total: number;
    learning: number;
    active: number;
    drift: number;
    frozen: number;
    alerts: number;
    rebuild: number;
  };
};

export type BehaviorBaselineAnalytics = {
  baseline_id: string;
  window_days: number;
  metric_name: string;
  unit: string;
  distributions: Array<{ metric_name: string; unit: string; values: [number, number, number, number, number] }>;
  series: Array<{
    timestamp: number;
    mean: number;
    p50: number;
    p95: number;
    p99: number;
    upper: number;
    lower: number;
    samples: number[];
  }>;
};

export type BehaviorBaselineOverview = {
  baseline_type: BaselineTabType;
  window_days: number;
  source: string;
  kpis: Array<{ key: string; value: number; unit?: string; source: string }>;
  boxplots: Array<{ entity_id: string; values: [number, number, number, number, number]; samples: number }>;
  heatmap: { x: string[]; y: string[]; values: Array<{ x: number; y: number; value: number }> };
  calendar: { x: string[]; y: string[]; values: Array<{ x: number; y: number; value: number }> };
  series: Array<{ timestamp: number; key: string; value: number }>;
  shares: Array<{ key: string; sessions: number; bytes: number; share: number; first_seen: number }>;
  links: Array<{ source: string; target: string; count: number; denied: number }>;
  facts: Array<{
    kind: string;
    entity_id: string;
    related_id?: string;
    label?: string;
    value: number;
    count: number;
    denied?: number;
    timestamp?: number;
    status?: string;
  }>;
  availability: Record<string, string>;
};

export type BehaviorBaselineActionRequest = {
  action: 'create_alert' | 'adjust_threshold' | 'freeze' | 'unfreeze' | 'forensics' | 'feedback_model' | 'cold_start' | 'drift_watch' | 'rebuild' | 'rollback' | 'audit_trace';
  reason?: string;
  warning_multiplier?: number;
  alert_multiplier?: number;
  target_version?: number;
  detail?: Record<string, unknown>;
};

export type BehaviorBaselineAction = {
  action_id: string;
  baseline_id: string;
  action: string;
  status: 'applied' | 'queued' | string;
  local_state_applied?: boolean;
  downstream_status?: string;
  downstream_attempts?: number;
  downstream_error?: string;
  reason: string;
  request?: Record<string, unknown>;
  requested_by: string;
  created_at: number;
};

export type BehaviorBaselineVersion = {
  baseline_id: string;
  version: number;
  snapshot: {
    warning_multiplier?: number;
    alert_multiplier?: number;
    frozen?: boolean;
    drift_watch?: boolean;
  };
  source_action_id?: string;
  created_by: string;
  created_at: number;
};

export type BehaviorBaselineVersionList = { versions: BehaviorBaselineVersion[]; total: number };
export type BehaviorBaselineActionList = { actions: BehaviorBaselineAction[]; total: number };

const unwrap = <T>(payload: ApiEnvelope<T> | T): T => {
  if (payload && typeof payload === 'object' && 'data' in payload) {
    return (payload as ApiEnvelope<T>).data as T;
  }
  return payload as T;
};

export const fetchBehaviorBaselines = async (
  baselineType: BaselineTabType,
  windowDays = 30,
  limit = 5000,
  maxItems = Number.POSITIVE_INFINITY,
): Promise<BehaviorBaselineList> => {
  const pageSize = Math.max(1, Math.min(5000, limit));
  let offset = 0;
  let aggregate: BehaviorBaselineList | undefined;
  do {
    const response = await api.get<ApiEnvelope<BehaviorBaselineList> | BehaviorBaselineList>('/v1/baselines', {
      params: { baseline_type: baselineType, window_days: windowDays, limit: pageSize, offset },
    });
    const page = unwrap(response.data);
    aggregate = aggregate
      ? { ...aggregate, baselines: [...aggregate.baselines, ...page.baselines].slice(0, maxItems), total: page.total, summary: page.summary }
      : { ...page, baselines: page.baselines.slice(0, maxItems) };
    offset += page.baselines.length;
    if (!page.baselines.length) break;
  } while (aggregate && offset < aggregate.total && offset < maxItems);
  return aggregate ?? { baselines: [], total: 0, summary: { scope: 'all_entities_in_window', total: 0, learning: 0, active: 0, drift: 0, frozen: 0, alerts: 0, rebuild: 0 } };
};

export const fetchBehaviorBaselineOverview = async (baselineType: BaselineTabType, windowDays: number): Promise<BehaviorBaselineOverview> => {
  const response = await api.get<ApiEnvelope<BehaviorBaselineOverview> | BehaviorBaselineOverview>('/v1/baselines/overview', {
    params: { baseline_type: baselineType, window_days: windowDays },
  });
  return unwrap(response.data);
};

export const fetchBehaviorBaselineVersions = async (baselineId: string): Promise<BehaviorBaselineVersionList> => {
  const response = await api.get<ApiEnvelope<BehaviorBaselineVersionList> | BehaviorBaselineVersionList>(`/v1/baselines/${encodeURIComponent(baselineId)}/versions`);
  return unwrap(response.data);
};

export const fetchBehaviorBaselineActions = async (baselineId: string): Promise<BehaviorBaselineActionList> => {
  const response = await api.get<ApiEnvelope<BehaviorBaselineActionList> | BehaviorBaselineActionList>(`/v1/baselines/${encodeURIComponent(baselineId)}/actions`);
  return unwrap(response.data);
};

export const fetchBehaviorBaseline = async (baselineId: string): Promise<BehaviorBaseline> => {
  const response = await api.get<ApiEnvelope<BehaviorBaseline> | BehaviorBaseline>(`/v1/baselines/${encodeURIComponent(baselineId)}`);
  return unwrap(response.data);
};

export const fetchBehaviorBaselineAnalytics = async (baselineId: string, windowDays: number): Promise<BehaviorBaselineAnalytics> => {
  const response = await api.get<ApiEnvelope<BehaviorBaselineAnalytics> | BehaviorBaselineAnalytics>(`/v1/baselines/${encodeURIComponent(baselineId)}/analytics`, { params: { window_days: windowDays } });
  return unwrap(response.data);
};

export const submitBehaviorBaselineAction = async (
  baselineId: string,
  payload: BehaviorBaselineActionRequest,
): Promise<{ action: BehaviorBaselineAction; audit_written: boolean }> => {
  const response = await api.post<ApiEnvelope<{ action: BehaviorBaselineAction; audit_written: boolean }> | { action: BehaviorBaselineAction; audit_written: boolean }>(
    `/v1/baselines/${encodeURIComponent(baselineId)}/actions`,
    payload,
  );
  return unwrap(response.data);
};

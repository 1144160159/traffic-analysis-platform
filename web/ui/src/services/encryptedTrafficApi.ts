import { api } from '@/services/httpClient';
import { getPageActionPlan } from '@/services/pageApiPlans';

export type EncryptedTrafficTimeRange = '近 1 小时' | '近 24 小时' | '近 7 天';

export type EncryptedTrafficAvailability =
  | 'available'
  | 'zero'
  | 'no_sample'
  | 'not_computable'
  | 'unavailable'
  | 'forbidden';

export type EncryptedTrafficSnapshotFact = Record<string, unknown>;

export type EncryptedTrafficSnapshotSection = {
  availability: EncryptedTrafficAvailability;
  sample_count: number;
  source: string;
  source_watermark: string;
  rule_versions: string[];
  model_versions: string[];
  partial: boolean;
  missing_reasons: string[];
  facts: EncryptedTrafficSnapshotFact[];
};

export type EncryptedTrafficSnapshot = {
  snapshot_id: string;
  tenant_id: string;
  as_of: string;
  window_start: string;
  window_end: string;
  flow_metadata: EncryptedTrafficSnapshotSection;
  plaintext_visible: EncryptedTrafficSnapshotSection;
  side_channel: EncryptedTrafficSnapshotSection;
  raw_reference: EncryptedTrafficSnapshotSection;
  randomness_statistics: EncryptedTrafficSnapshotSection;
  next_continuation?: string;
};

export type EncryptedTrafficSnapshotMeta = {
  contract_version: number;
  schema_version: number;
  snapshot_id: string;
  as_of: string;
  trace_id: string;
  result_code: string;
  partial: boolean;
  missing_sections: string[];
  source_watermarks: Record<string, string>;
};

export type EncryptedTrafficSnapshotResult = {
  snapshot: EncryptedTrafficSnapshot;
  meta: EncryptedTrafficSnapshotMeta;
};

export type EncryptedTrafficEgressActionId =
  | 'egress-create-alert'
  | 'egress-evidence-lookup'
  | 'egress-entity-graph'
  | 'egress-audit-write'
  | 'egress-response-request';

export type EncryptedTrafficEvidenceActionId =
  | 'evidence-create-task'
  | 'evidence-download-pcap'
  | 'evidence-verify-hash'
  | 'evidence-export-package'
  | 'evidence-associate-analysis'
  | 'evidence-preserve'
  | 'evidence-link-alert'
  | 'evidence-expert-review'
  | 'evidence-gap-mark'
  | 'evidence-submit-recommendation'
  | 'evidence-export-report'
  | 'evidence-write-audit';

type EncryptedTrafficDataMode = 'live' | 'partial' | 'simulated' | 'unavailable';

export type EncryptedTrafficEgressActionInput = {
  actionId: EncryptedTrafficEgressActionId;
  target: string;
  dataMode: EncryptedTrafficDataMode;
};

export type EncryptedTrafficEvidenceActionInput = {
  actionId: EncryptedTrafficEvidenceActionId;
  target: string;
  dataMode: EncryptedTrafficDataMode;
};

export type EncryptedTrafficEgressActionResult = {
  action_id: string;
  action: string;
  audit_event: string;
  status: 'recorded';
  target: string;
};

export type EncryptedTrafficEvidenceActionResult = {
  action_id: string;
  action: string;
  audit_event: string;
  status: 'recorded';
  target: string;
};

const encryptedTrafficRangeMilliseconds: Record<EncryptedTrafficTimeRange, number> = {
  '近 1 小时': 60 * 60 * 1_000,
  '近 24 小时': 24 * 60 * 60 * 1_000,
  '近 7 天': 7 * 24 * 60 * 60 * 1_000,
};

export const buildEncryptedTrafficRangeParams = (
  timeRange: EncryptedTrafficTimeRange = '近 24 小时',
  endTime = Date.now(),
) => ({
  start_time: endTime - encryptedTrafficRangeMilliseconds[timeRange],
  end_time: endTime,
});

export const fetchEncryptedTrafficSnapshot = async (
  timeRange: EncryptedTrafficTimeRange = '近 24 小时',
  endTime = Date.now(),
): Promise<EncryptedTrafficSnapshotResult> => {
  const response = await api.get<{
    data: EncryptedTrafficSnapshot;
    meta: EncryptedTrafficSnapshotMeta;
  }>('/v1/encrypted-traffic/snapshot', {
    params: buildEncryptedTrafficRangeParams(timeRange, endTime),
  });
  return { snapshot: response.data.data, meta: response.data.meta };
};

export const submitEncryptedTrafficEgressAction = async ({
  actionId,
  target,
  dataMode,
}: EncryptedTrafficEgressActionInput): Promise<EncryptedTrafficEgressActionResult> => {
  const plan = getPageActionPlan('encrypted-traffic', actionId);
  if (!plan || plan.method !== 'POST') throw new Error(`未找到外联处置 API：${actionId}`);
  const response = await api.post<
    { data?: EncryptedTrafficEgressActionResult } & EncryptedTrafficEgressActionResult
  >(plan.endpoint, {
    ...(plan.defaultBody ?? {}),
    target,
    data_mode: dataMode,
  });
  return response.data.data ?? response.data;
};

export const submitEncryptedTrafficEvidenceAction = async ({
  actionId,
  target,
  dataMode,
}: EncryptedTrafficEvidenceActionInput): Promise<EncryptedTrafficEvidenceActionResult> => {
  const plan = getPageActionPlan('encrypted-traffic', actionId);
  if (!plan || plan.method !== 'POST') throw new Error(`未找到证据中心动作 API：${actionId}`);
  const response = await api.post<
    { data?: EncryptedTrafficEvidenceActionResult } & EncryptedTrafficEvidenceActionResult
  >(plan.endpoint, {
    ...(plan.defaultBody ?? {}),
    target,
    data_mode: dataMode,
  });
  return response.data.data ?? response.data;
};

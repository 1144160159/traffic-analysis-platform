import axios from 'axios';
import { appConfig } from '@/config/runtime';
import { buildVisualBreakdownSnapshot, type PageSnapshot } from '@/services/mockData';
import { findRouteById } from '@/routes/routeManifest';
import type { PageSpec } from '@/routes/routeManifest';
import { getPageActionPlan, getPageApiPlan, getPageLoadSecondaryEndpoints } from '@/services/pageApiPlans';
import { adaptKnownPageSnapshot } from '@/services/pageSnapshotAdapters';
import { clearAuthTokens, getAuthToken } from '@/services/authStorage';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

export type LoginPayload = {
  tenant_id?: string;
  username: string;
  password: string;
  captcha_id?: string;
  captcha_code?: string;
};

export type LoginResult = {
  token: string;
  refreshToken?: string;
  expiresIn?: number;
  username: string;
  role: string;
  user: CurrentUser;
};

export type CurrentUser = {
  userId?: string;
  tenantId?: string;
  username: string;
  email?: string;
  role: string;
  roles: string[];
  permissions: string[];
};

export type CaptchaChallenge = {
  captchaId: string;
  imageData: string;
  expiresIn: number;
};

export type OidcLoginOptions = {
  tenantId?: string;
  redirectUrl: string;
};

type AuthUserResponse = {
  user_id?: string;
  tenant_id?: string;
  username?: string;
  email?: string;
  role?: string;
  roles?: string[];
  permissions?: string[];
};

type AuthLoginResponse = {
  access_token?: string;
  token?: string;
  refresh_token?: string;
  expires_in?: number;
  token_type?: string;
  user?: AuthUserResponse;
  username?: string;
  role?: string;
  roles?: string[];
  permissions?: string[];
};

type CaptchaResponse = {
  captcha_id: string;
  image_data: string;
  expires_in: number;
};

export const api = axios.create({
  baseURL: appConfig.apiBaseUrl,
  timeout: 30_000,
});

api.interceptors.request.use((config) => {
  const token = getAuthToken();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (axios.isAxiosError(error) && error.response?.status === 401) {
      clearAuthTokens();
    }
    return Promise.reject(error);
  },
);

export const localBypassUser: CurrentUser = {
  username: 'sec_analyst',
  role: '安全分析师',
  roles: ['admin'],
  permissions: ['*'],
};

export const fetchCaptcha = async (): Promise<CaptchaChallenge> => {
  if (!appConfig.authEnabled || appConfig.useMock) {
    return {
      captchaId: 'mock-captcha',
      imageData: '',
      expiresIn: 120,
    };
  }
  const response = await api.get<CaptchaResponse>('/v1/auth/captcha');
  return {
    captchaId: response.data.captcha_id,
    imageData: response.data.image_data,
    expiresIn: response.data.expires_in,
  };
};

export const login = async (payload: LoginPayload): Promise<LoginResult> => {
  if (!appConfig.authEnabled || appConfig.useMock) {
    const user = { ...localBypassUser, username: payload.username };
    return {
      token: `mock-token-${payload.username}`,
      username: user.username,
      role: user.role,
      user,
    };
  }
  const response = await api.post<AuthLoginResponse>('/v1/auth/login', payload);
  return normalizeLoginResponse(response.data);
};

export const buildOidcLoginUrl = ({ tenantId, redirectUrl }: OidcLoginOptions) => {
  const baseUrl = appConfig.apiBaseUrl.replace(/\/$/, '');
  const origin = typeof window === 'undefined' ? 'http://localhost' : window.location.origin;
  const url = new URL(`${baseUrl}/v1/auth/oidc/login`, origin);
  url.searchParams.set('redirect', redirectUrl);
  if (tenantId?.trim()) {
    url.searchParams.set('tenant_id', tenantId.trim());
  }
  return url.toString();
};

export const fetchCurrentUser = async (): Promise<CurrentUser> => {
  if (!appConfig.authEnabled || appConfig.useMock) {
    return localBypassUser;
  }
  const response = await api.get<AuthUserResponse>('/v1/auth/me');
  return normalizeCurrentUser(response.data);
};

export const logout = async () => {
  if (appConfig.authEnabled && !appConfig.useMock) {
    await api.post('/v1/auth/logout');
  }
  clearAuthTokens();
};

export type EncryptedTrafficTimeRange = '近 1 小时' | '近 24 小时' | '近 7 天';
export type DataQualityTimeRange = '近 24 小时' | '近 7 天';
export type CampaignSnapshotFilters = {
  risk: string;
  status: string;
  phase: string;
  keyword: string;
};

export type AssetSnapshotFilters = {
  status?: string;
  search?: string;
  department?: string;
  campus?: string;
};

export type PageSnapshotRequestOptions = {
  timeRange?: EncryptedTrafficTimeRange;
  dataQualityTimeRange?: DataQualityTimeRange;
  page?: number;
  pageSize?: number;
  campaignFilters?: CampaignSnapshotFilters;
  assetFilters?: AssetSnapshotFilters;
  assetType?: 'endpoint' | 'server' | 'network-device' | 'business-system' | 'unknown';
  sourceAssetId?: string;
  sourceAssetIp?: string;
  forensicsFilters?: {
    assetId?: string;
    srcIp?: string;
    dstIp?: string;
    protocol?: string;
    port?: string;
    tuple?: string;
    taskId?: string;
  };
};

export type RuleRecord = {
  rule_id: string;
  tenant_id: string;
  name: string;
  type: string;
  engine: string;
  description?: string;
  conditions?: Record<string, unknown>;
  labels?: string[];
  severity: string;
  enabled: boolean;
  priority: number;
  version: number;
  status: string;
  created_by: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
};

export type RuleListResult = {
  items: RuleRecord[];
  total: number;
  limit: number;
  offset: number;
  hasMore: boolean;
};

export type RuleVersionRecord = {
  rule_version_id?: string;
  rule_version?: string;
  rule_id: string;
  tenant_id: string;
  version: number;
  status: string;
  change_log?: string;
  created_by: string;
  created_at: string;
};

export type RuleWorkbench = {
  rule: RuleRecord;
  versions: RuleVersionRecord[];
  items: Record<string, Array<Record<string, unknown>>>;
  source: 'postgresql' | string;
};

export type RuleActionJob = {
  job_id: string;
  action_id: string;
  tenant_id: string;
  rule_id: string;
  action: string;
  target: string;
  status: string;
  requested_by: string;
  created_at: string;
};

export const fetchRulesPage = async ({
  page,
  pageSize,
  keyword,
  type,
  enabled,
  labels,
}: {
  page: number;
  pageSize: number;
  keyword?: string;
  type?: string;
  enabled?: boolean;
  labels?: string;
}): Promise<RuleListResult> => {
  const offset = Math.max(0, page - 1) * pageSize;
  const response = await api.get<{
    data: RuleRecord[];
    pagination: { total: number; limit: number; offset: number; has_more: boolean };
  }>('/v1/rules', { params: { limit: pageSize, offset, keyword: keyword || undefined, type, enabled, labels } });
  return {
    items: response.data.data ?? [],
    total: response.data.pagination?.total ?? 0,
    limit: response.data.pagination?.limit ?? pageSize,
    offset: response.data.pagination?.offset ?? offset,
    hasMore: response.data.pagination?.has_more ?? false,
  };
};

export const fetchRuleWorkbench = async (ruleId: string): Promise<RuleWorkbench> => {
  if (!ruleId) throw new Error('rule id required');
  const response = await api.get<{ data: RuleWorkbench }>(`/v1/rules/${encodeURIComponent(ruleId)}/workbench`);
  return response.data.data;
};

export const submitRuleWorkbenchAction = async ({
  ruleId,
  action,
  target,
  payload,
}: {
  ruleId: string;
  action: string;
  target: string;
  payload?: Record<string, unknown>;
}): Promise<RuleActionJob> => {
  const response = await api.post<{ data: RuleActionJob }>(`/v1/rules/${encodeURIComponent(ruleId)}/actions`, {
    action_id: globalThis.crypto.randomUUID(),
    action,
    target,
    payload,
  });
  return response.data.data;
};

export type DeploymentRecord = {
  deployment_id: string;
  tenant_id: string;
  name: string;
  description?: string;
  rule_version?: string;
  model_version?: string;
  feature_set_id?: string;
  scope: Record<string, unknown>;
  status: string;
  metadata?: Record<string, unknown>;
  gray_started_at?: string;
  gray_expired_at?: string;
  activated_at?: string;
  rolled_back_at?: string;
  rollback_from?: string;
  rollback_reason?: string;
  error_message?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type DeploymentListResult = {
  items: DeploymentRecord[];
  total: number;
  limit: number;
  offset: number;
  hasMore: boolean;
};

export type DeploymentHistoryRecord = {
  id: number;
  deployment_id: string;
  action: string;
  operator_id: string;
  created_at: string;
  detail?: Record<string, unknown>;
};

export type DeploymentWorkbench = {
  deployment: DeploymentRecord;
  history: DeploymentHistoryRecord[];
  items: Record<string, Array<Record<string, unknown>>>;
  source: 'postgresql' | string;
};

export type DeploymentEvidenceBundle = {
  export_id: string;
  generated_at: string;
  generated_by: string;
  deployment: DeploymentRecord;
  history: DeploymentHistoryRecord[];
  evidence: Array<Record<string, unknown>>;
  source: string;
  bundle_checksum: string;
  download_content: string;
};

export type DeploymentAction = 'gray' | 'activate' | 'pause' | 'resume' | 'rollback';

export type DeploymentWorkflow = {
  stage: 'draft_saved' | 'precheck_completed' | 'approval_pending' | 'approved' | 'rejected';
  operation: 'deploy' | 'rollback';
  configuration: Record<string, unknown>;
  precheck_status?: string;
  precheck_results?: Array<Record<string, unknown>>;
  precheck_snapshot_hash?: string;
  precheck_completed_at?: string;
  approval_id?: string;
  approval_snapshot?: Record<string, unknown>;
  approval_snapshot_hash?: string;
  requested_by?: string;
  requested_at?: string;
  approved_by?: string;
  approved_at?: string;
  rejected_by?: string;
  rejected_at?: string;
};

export const fetchDeploymentsPage = async ({
  page,
  pageSize,
  status,
}: {
  page: number;
  pageSize: number;
  status?: string;
}): Promise<DeploymentListResult> => {
  const offset = Math.max(0, page - 1) * pageSize;
  const response = await api.get<{
    data: DeploymentRecord[];
    pagination: { total: number; limit: number; offset: number; has_more: boolean };
  }>('/v1/deployments', { params: { limit: pageSize, offset, status: status || undefined } });
  return {
    items: response.data.data ?? [],
    total: response.data.pagination?.total ?? 0,
    limit: response.data.pagination?.limit ?? pageSize,
    offset: response.data.pagination?.offset ?? offset,
    hasMore: response.data.pagination?.has_more ?? false,
  };
};

export const fetchDeploymentWorkbench = async (deploymentId: string): Promise<DeploymentWorkbench> => {
  if (!deploymentId) throw new Error('deployment id required');
  const response = await api.get<{ data: DeploymentWorkbench }>(`/v1/deployments/${encodeURIComponent(deploymentId)}/workbench`);
  return response.data.data;
};

export const createDeployment = async (payload: {
  name: string;
  description?: string;
  rule_version?: string;
  model_version?: string;
  feature_set_id?: string;
  scope: Record<string, unknown>;
}): Promise<DeploymentRecord> => {
  const response = await api.post<{ data: DeploymentRecord }>('/v1/deployments', payload);
  return response.data.data;
};

export const submitDeploymentAction = async ({
  deploymentId,
  action,
  reason,
  targetDeploymentId,
}: {
  deploymentId: string;
  action: DeploymentAction;
  reason?: string;
  targetDeploymentId?: string;
}): Promise<{ success: boolean; message?: string }> => {
  if (!deploymentId) throw new Error('deployment id required');
  const response = await api.post<{ success: boolean; message?: string }>(
    `/v1/deployments/${encodeURIComponent(deploymentId)}/${action}`,
    action === 'rollback' ? { reason: reason?.trim() ?? '', target_deployment_id: targetDeploymentId?.trim() ?? '' } : undefined,
  );
  return response.data;
};

export const updateDeploymentScope = async ({
  deploymentId,
  scope,
}: {
  deploymentId: string;
  scope: Record<string, unknown>;
}): Promise<DeploymentRecord> => {
  if (!deploymentId) throw new Error('deployment id required');
  const response = await api.put<{ data: DeploymentRecord }>(
    `/v1/deployments/${encodeURIComponent(deploymentId)}/scope`,
    { scope },
  );
  return response.data.data;
};

export const updateDeploymentWorkflow = async ({
  deploymentId,
  stage,
  operation,
  configuration,
}: {
  deploymentId: string;
  stage: 'draft' | 'precheck' | 'submit_approval' | 'approve' | 'reject';
  operation: 'deploy' | 'rollback';
  configuration?: Record<string, unknown>;
}): Promise<DeploymentWorkflow> => {
  if (!deploymentId) throw new Error('deployment id required');
  const response = await api.post<{ data: DeploymentWorkflow }>(`/v1/deployments/${encodeURIComponent(deploymentId)}/workflow`, { stage, operation, ...(configuration ? { configuration } : {}) });
  return response.data.data;
};

export const exportDeploymentEvidence = async (deploymentId: string): Promise<DeploymentEvidenceBundle> => {
  if (!deploymentId) throw new Error('deployment id required');
  const response = await api.post<{ data: DeploymentEvidenceBundle }>(
    `/v1/deployments/${encodeURIComponent(deploymentId)}/evidence/export`,
  );
  return response.data.data;
};

export type AssetRecord = {
  asset_id: string;
  display_code: string;
  tenant_id: string;
  asset_type: 'endpoint' | 'server' | 'network-device' | 'business-system' | 'unknown';
  status: string;
  ip_address: string;
  mac_address: string;
  hostname?: string;
  vendor?: string;
  os_type?: string;
  source: string;
  vlan_id?: string;
  switch_port?: string;
  department?: string;
  campus?: string;
  owner?: string;
  criticality: number;
  tags?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  first_seen: string;
  last_seen: string;
};

export type AssetEvent = {
  event_id: number;
  asset_id: string;
  tenant_id: string;
  event_type: string;
  old_value?: string;
  new_value?: string;
  created_at: string;
};

export type AssetNetworkInterface = {
  name: string;
  adapter: string;
  ip_address: string;
  mac_address: string;
  vlan_id: string;
  mirror_mode: string;
  status: string;
  speed: string;
  duplex: string;
  ingress_bytes: number;
  egress_bytes: number;
  packet_loss_pct: number;
  error_count: number;
  probe_id: string;
};

export type AssetOpenService = {
  port: number;
  protocol: string;
  service: string;
  version: string;
  exposure_scope: string;
  access_source_count: number;
  risk_level: string;
  alert_count: number;
};

export type AssetOwnershipLink = {
  name: string;
  role: string;
  owner: string;
  status: string;
};

export type AssetResponsibility = {
  role: string;
  owner: string;
  status: string;
};

export type AssetOwnership = {
  campus: string;
  department: string;
  owner: string;
  business_systems: AssetOwnershipLink[];
  asset_groups: AssetOwnershipLink[];
  data_domains: AssetOwnershipLink[];
  responsibilities: AssetResponsibility[];
  pending_fields: string[];
};

export type AssetDetails = {
  asset_id: string;
  data_contract: string;
  network_interfaces: AssetNetworkInterface[];
  open_services: AssetOpenService[];
  ownership: AssetOwnership;
  observed_at: string;
};

export type AssetTopologyNode = {
  id: string;
  label: string;
  kind?: string;
  status?: string;
  risk?: string;
};

export type AssetTopologyEdge = {
  id: string;
  source: string;
  target: string;
  relationship: string;
  direction?: string;
  protocol?: string;
  health?: string;
  confidence?: number;
  observed_at?: string;
};

export type AssetTopologyGraph = {
  asset_id: string;
  source: 'discovery_neighbors' | 'asset_metadata_graph' | 'legacy_asset_metadata' | 'empty' | string;
  fixture_mode: boolean;
  nodes: AssetTopologyNode[];
  edges: AssetTopologyEdge[];
  observed_at: string;
};

export type ProbeTopologyPoint = {
  x: number;
  y: number;
};

export type ProbeTopologyNode = {
  id: string;
  probe_id: string;
  kind: 'probe' | 'core' | 'switch' | 'mirror' | string;
  label: string;
  detail: string;
  status: 'ok' | 'warn' | 'risk' | string;
  zone: string;
  role: string;
  bandwidth_gbps: number;
  elevation: number;
  position_2d: ProbeTopologyPoint;
  position_3d: ProbeTopologyPoint;
};

export type ProbeTopologyEdge = {
  id: string;
  source: string;
  target: string;
  kind: 'access' | 'uplink' | 'backbone' | string;
  status: 'ok' | 'warn' | 'risk' | string;
  bandwidth_gbps: number;
};

export type ProbeTopologyZone = {
  id: string;
  label: string;
  status: 'ok' | 'warn' | 'risk' | string;
  polygon_2d: ProbeTopologyPoint[];
  polygon_3d: ProbeTopologyPoint[];
};

export type ProbeTopologyGraph = {
  revision: string;
  source: string;
  active_mode: '2d' | '3d';
  coordinate_system: 'normalized-0-100' | string;
  generated_at: string;
  nodes: ProbeTopologyNode[];
  edges: ProbeTopologyEdge[];
  zones: ProbeTopologyZone[];
};

export const fetchAsset = async (assetId: string): Promise<AssetRecord> => {
  if (!assetId) throw new Error('asset id required');
  const response = await api.get<{ data: AssetRecord }>(`/v1/assets/${encodeURIComponent(assetId)}`);
  return response.data.data;
};

export const fetchAssetHistory = async (assetId: string, limit = 50): Promise<AssetEvent[]> => {
  if (!assetId) throw new Error('asset id required');
  const response = await api.get<{ data: AssetEvent[] }>(`/v1/assets/${encodeURIComponent(assetId)}/history`, {
    params: { limit },
  });
  return response.data.data ?? [];
};

export const fetchAssetDetails = async (assetId: string): Promise<AssetDetails> => {
  if (!assetId) throw new Error('asset id required');
  const response = await api.get<{ data: AssetDetails }>(`/v1/assets/${encodeURIComponent(assetId)}/details`);
  return response.data.data;
};

export const fetchAssetTopology = async (assetId: string): Promise<AssetTopologyGraph> => {
  if (!assetId) throw new Error('asset id required');
  const response = await api.get<{ data: AssetTopologyGraph }>(`/v1/assets/${encodeURIComponent(assetId)}/topology`);
  return response.data.data;
};

export const fetchProbeTopology = async (mode: '2d' | '3d'): Promise<ProbeTopologyGraph> => {
  const response = await api.get<{ data: ProbeTopologyGraph }>('/v1/probes/topology', { params: { mode } });
  return response.data.data;
};

export const fetchPageSnapshot = async (pageId: string, options: PageSnapshotRequestOptions = {}): Promise<PageSnapshot> => {
  const route = findRouteById(pageId);
  if (!route) throw new Error(`Unknown page: ${pageId}`);

  if (isVisualBreakdownMode() && pageId !== 'assets') {
    return buildVisualBreakdownSnapshot(route.page);
  }

  if (appConfig.useMock) {
    const response = await api.get<PageSnapshot>(`/v1/ui/pages/${pageId}`);
    return response.data;
  }

  return fetchRealPageSnapshot(route.page, options);
};

type ApiEnvelope = {
  data?: unknown;
  total?: number;
  pagination?: { total?: number };
  meta?: { page?: { total?: number } };
  [key: string]: unknown;
};

type EncryptedTrafficEgressActionId = 'egress-create-alert' | 'egress-evidence-lookup' | 'egress-entity-graph' | 'egress-audit-write' | 'egress-response-request';

export type EncryptedTrafficEgressActionInput = {
  actionId: EncryptedTrafficEgressActionId;
  target: string;
  dataMode: 'live' | 'partial' | 'simulated' | 'unavailable';
};

export type EncryptedTrafficEgressActionResult = {
  action_id: string;
  action: string;
  audit_event: string;
  status: 'recorded';
  target: string;
};

export const submitEncryptedTrafficEgressAction = async ({ actionId, target, dataMode }: EncryptedTrafficEgressActionInput): Promise<EncryptedTrafficEgressActionResult> => {
  const plan = getPageActionPlan('encrypted-traffic', actionId);
  if (!plan || plan.method !== 'POST') throw new Error(`未找到外联处置 API：${actionId}`);
  const response = await api.post<
    {
      data?: EncryptedTrafficEgressActionResult;
    } & EncryptedTrafficEgressActionResult
  >(plan.endpoint, {
    ...(plan.defaultBody ?? {}),
    target,
    data_mode: dataMode,
  });
  return response.data.data ?? response.data;
};

type EncryptedTrafficEvidenceActionId = 'evidence-create-task' | 'evidence-download-pcap' | 'evidence-verify-hash' | 'evidence-export-package' | 'evidence-associate-analysis' | 'evidence-preserve' | 'evidence-link-alert' | 'evidence-expert-review' | 'evidence-gap-mark' | 'evidence-submit-recommendation' | 'evidence-export-report' | 'evidence-write-audit';

export type EncryptedTrafficEvidenceActionInput = {
  actionId: EncryptedTrafficEvidenceActionId;
  target: string;
  dataMode: 'live' | 'partial' | 'simulated' | 'unavailable';
};

export type EncryptedTrafficEvidenceActionResult = {
  action_id: string;
  action: string;
  audit_event: string;
  status: 'recorded';
  target: string;
};

export const submitEncryptedTrafficEvidenceAction = async ({ actionId, target, dataMode }: EncryptedTrafficEvidenceActionInput): Promise<EncryptedTrafficEvidenceActionResult> => {
  const plan = getPageActionPlan('encrypted-traffic', actionId);
  if (!plan || plan.method !== 'POST') throw new Error(`未找到证据中心动作 API：${actionId}`);
  const response = await api.post<
    {
      data?: EncryptedTrafficEvidenceActionResult;
    } & EncryptedTrafficEvidenceActionResult
  >(plan.endpoint, {
    ...(plan.defaultBody ?? {}),
    target,
    data_mode: dataMode,
  });
  return response.data.data ?? response.data;
};

export type ProbeOperationActionId = 'probe-batch-upgrade' | 'probe-batch-state' | 'probe-config-push' | 'probe-connectivity-test' | 'probe-cert-rotate' | 'probe-restart';

export type ProbeOperationResult = {
  operation_id?: string;
  operation_ids?: string[];
  batch_id?: string;
  probe_id?: string;
  probe_ids?: string[];
  status: string;
  changed_count?: number;
  upgraded_count?: number;
  desired_state?: string;
  target_version?: string;
  checks?: Array<{
    target: string;
    status: string;
    latency_ms: number;
    detail: string;
  }>;
};

export const submitProbeOperation = async (actionId: ProbeOperationActionId, probeIds: string[], overrides: Record<string, unknown> = {}): Promise<ProbeOperationResult> => {
  const plan = getPageActionPlan('probes', actionId);
  if (!plan || plan.method !== 'POST') throw new Error(`未找到探针运维 API：${actionId}`);
  const normalizedProbeIds = [...new Set(probeIds.map((value) => value.trim()).filter(Boolean))];
  if (!normalizedProbeIds.length) throw new Error('至少选择一台探针');
  const endpoint = plan.endpoint.replace('{id}', encodeURIComponent(normalizedProbeIds[0]));
  const body = {
    ...(plan.defaultBody ?? {}),
    ...overrides,
    ...(actionId === 'probe-batch-upgrade' || actionId === 'probe-batch-state' ? { probe_ids: normalizedProbeIds } : {}),
  };
  const response = await api.post<{ data?: ProbeOperationResult } & Partial<ProbeOperationResult>>(endpoint, body);
  return (response.data.data ?? response.data) as ProbeOperationResult;
};

export type DataQualityActionRequest = {
  view: 'overview' | 'topic-health' | 'flink-quality' | 'field-quality' | 'storage-quality' | 'replay-reconcile' | 'report' | 'settings';
  action: string;
  target: string;
  dry_run: boolean;
  confirmed?: boolean;
  reason?: string;
  parameters?: Record<string, unknown>;
};

export type DataQualityActionResult = {
  action_id: string;
  tenant_id: string;
  view: DataQualityActionRequest['view'];
  action: string;
  target: string;
  dry_run: boolean;
  status: 'dry_run' | 'queued';
  requested_by: string;
  created_at: string;
};

export type DataQualityTableDataset = 'consumerRows' | 'messageSizeTopicRows' | 'partitionQueueRows' | 'flinkJobRows' | 'flinkWindowRows' | 'flinkFailureRows' | 'fieldQualityRows' | 'communityCheckRows' | 'communityMismatchRows' | 'fieldAnomalyRows' | 'fieldLineageRows' | 'fieldRepairRows' | 'storageComponentRows' | 'storageFailureRows' | 'storageReplicaRows' | 'storagePartitionRows' | 'storageObjectRows' | 'replayTaskRows' | 'replayIdempotencyRows' | 'replayDifferenceRows' | 'replayEvidenceRows';

export type DataQualityTablePage<T> = {
  tenant_id: string;
  fixture_version: string;
  dataset: DataQualityTableDataset;
  items: T[];
  total: number;
  page: number;
  page_size: number;
};

export const fetchDataQualityTablePage = async <T>(dataset: DataQualityTableDataset, page: number, pageSize: number): Promise<DataQualityTablePage<T>> => {
  const response = await api.get<{ data?: DataQualityTablePage<T> } & Partial<DataQualityTablePage<T>>>(`/v1/data-quality/tables/${encodeURIComponent(dataset)}`, {
    params: { page, page_size: pageSize },
  });
  return (response.data.data ?? response.data) as DataQualityTablePage<T>;
};

export const submitDataQualityAction = async (request: DataQualityActionRequest): Promise<DataQualityActionResult> => {
  const plan = getPageActionPlan('data-quality', 'data-quality-context-action');
  if (!plan || plan.method !== 'POST') throw new Error('未找到数据质量操作 API');
  const response = await api.post<{ data?: DataQualityActionResult } & Partial<DataQualityActionResult>>(plan.endpoint, request);
  return (response.data.data ?? response.data) as DataQualityActionResult;
};

export type DataQualityDailyReportMetric = {
  label: string;
  value: string;
  delta?: string;
  status: 'ok' | 'warn' | 'risk' | 'info';
  number: number;
};

export type DataQualityDailyReport = {
  report_id: string;
  tenant_id: string;
  title: string;
  version: string;
  generated_at: string;
  period_start: string;
  period_end: string;
  overall: string;
  score: number;
  kpis: DataQualityDailyReportMetric[];
  scores: DataQualityDailyReportMetric[];
  trend: Array<{ time: string; completeness: number; timeliness: number; consistency: number; availability: number }>;
  chapters: Array<{ index: number; label: string; progress: number; status: 'ok' | 'warn' | 'risk' }>;
  anomalies: Array<{ type: string; root_cause: string; owner: string; scope: string; status: string }>;
  key_metrics: string[][];
  storage_rows: string[][];
  reconcile: DataQualityDailyReportMetric[];
  conclusion: { result: string; summary: string; suggestion: string };
  exports: Array<{ export_id: string; time: string; format: 'PDF' | 'JSON' | 'CSV'; applicant: string; status: string; recipient: string; download_url: string }>;
  approval: { package_id: string; version: string; generated_at: string; contents: string[]; sla_gate: number; flow: string[]; risk: string };
  evidence: Array<{ label: string; value: string }>;
  download_formats: Array<'pdf' | 'json' | 'csv'>;
  source: { monitor: string; visuals: string; fixture_version: string };
};

const dataQualityReportRangeParams = (timeRange: DataQualityTimeRange) => {
  const endTime = Date.now();
  return {
    start_time: endTime - dataQualityRangeMilliseconds[timeRange],
    end_time: endTime,
  };
};

export const fetchDataQualityDailyReport = async (timeRange: DataQualityTimeRange): Promise<DataQualityDailyReport> => {
  const response = await api.get<{ data?: DataQualityDailyReport } & Partial<DataQualityDailyReport>>('/v1/data-quality/reports/daily', {
    params: dataQualityReportRangeParams(timeRange),
  });
  return (response.data.data ?? response.data) as DataQualityDailyReport;
};

export const downloadDataQualityDailyReport = async (timeRange: DataQualityTimeRange, format: 'pdf' | 'json' | 'csv') => {
  const response = await api.get<Blob>('/v1/data-quality/reports/daily/download', {
    params: { ...dataQualityReportRangeParams(timeRange), format },
    responseType: 'blob',
  });
  const disposition = String(response.headers['content-disposition'] ?? '');
  const filename = disposition.match(/filename="?([^";]+)"?/i)?.[1] ?? `data-quality-daily.${format}`;
  return { blob: response.data, filename };
};

export type ForensicsJobInput = {
  assetId?: string;
  probeId?: string;
  srcIp?: string;
  dstIp?: string;
  srcPort?: number;
  dstPort?: number;
  protocol?: number;
  startTime: number;
  endTime: number;
  maxPackets?: number;
};

export type ForensicsJobActionResult = {
  job_id: string;
  status: string;
  created_at?: number;
};

export type ForensicsVerifyResult = {
  key: string;
  tenant_id: string;
  sha256: string;
  expected_sha256?: string;
  registered_sha256?: string;
  verified: boolean;
  size_bytes: number;
};

export type ForensicsPresignResult = {
  key: string;
  url: string;
  expires_at: number;
  sha256?: string;
};

const actionPayload = <T>(payload: { data?: T } & Partial<T>): T => (payload.data ?? payload) as T;

export const createForensicsJob = async (input: ForensicsJobInput): Promise<ForensicsJobActionResult> => {
  const plan = getPageActionPlan('forensics', 'forensics-create-job');
  if (!plan || plan.method !== 'POST') throw new Error('未找到取证任务创建 API');
  const response = await api.post<{ data?: ForensicsJobActionResult } & Partial<ForensicsJobActionResult>>(plan.endpoint, {
    asset_id: input.assetId || undefined,
    probe_id: input.probeId || undefined,
    src_ip: input.srcIp || undefined,
    dst_ip: input.dstIp || undefined,
    src_port: input.srcPort || undefined,
    dst_port: input.dstPort || undefined,
    protocol: input.protocol || undefined,
    start_time: input.startTime,
    end_time: input.endTime,
    max_packets: input.maxPackets ?? 100_000,
  });
  return actionPayload(response.data);
};

export const verifyForensicsPcap = async (key: string, expectedSha256?: string): Promise<ForensicsVerifyResult> => {
  const plan = getPageActionPlan('forensics', 'forensics-verify-pcap');
  if (!plan || plan.method !== 'POST') throw new Error('未找到 PCAP 完整性校验 API');
  const response = await api.post<{ data?: ForensicsVerifyResult } & Partial<ForensicsVerifyResult>>(plan.endpoint, {
    key,
    expected_sha256: expectedSha256 || undefined,
  });
  return actionPayload(response.data);
};

export const presignForensicsPcap = async (key: string, expirySeconds = 3600): Promise<ForensicsPresignResult> => {
  const plan = getPageActionPlan('forensics', 'forensics-presign-pcap');
  if (!plan || plan.method !== 'POST') throw new Error('未找到 PCAP 签名 URL API');
  const response = await api.post<{ data?: ForensicsPresignResult } & Partial<ForensicsPresignResult>>(plan.endpoint, {
    key,
    expiry_seconds: expirySeconds,
  });
  return actionPayload(response.data);
};

export const cancelForensicsJob = async (jobId: string): Promise<ForensicsJobActionResult> => {
  const plan = getPageActionPlan('forensics', 'forensics-cancel-job');
  if (!plan || plan.method !== 'POST') throw new Error('未找到取证任务取消 API');
  const response = await api.post<{ data?: ForensicsJobActionResult } & Partial<ForensicsJobActionResult>>(plan.endpoint.replace('{id}', encodeURIComponent(jobId)));
  return actionPayload(response.data);
};

const fetchRealPageSnapshot = async (page: PageSpec, options: PageSnapshotRequestOptions): Promise<PageSnapshot> => {
  const plan = getPageApiPlan(page.id);
  const requestParams = getPageRequestParams(page.id, options);
  const secondaryEndpoints = getPageLoadSecondaryEndpoints(page.id);
  const [primary, ...secondary] = await Promise.all([
    api.get<ApiEnvelope>(plan.primary, {
      params: { limit: 8, page_size: 8, ...requestParams },
    }),
    ...secondaryEndpoints.map((endpoint) =>
      api
        .get<ApiEnvelope>(endpoint, {
          params: {
            limit: 50,
            page_size: 50,
            ...getSecondaryRequestParams(page.id, endpoint, options),
          },
        })
        .then((response) => response)
        .catch((error: unknown) => {
          if (page.id === 'encrypted-traffic' || page.id === 'forensics') throw error;
          return { data: { secondary_error: normalizeError(error) } };
        }),
    ),
  ]);

  return normalizeRealSnapshot(
    page,
    primary.data,
    secondary.map((response) => response.data),
  );
};

const encryptedTrafficRangeMilliseconds: Record<EncryptedTrafficTimeRange, number> = {
  '近 1 小时': 60 * 60 * 1_000,
  '近 24 小时': 24 * 60 * 60 * 1_000,
  '近 7 天': 7 * 24 * 60 * 60 * 1_000,
};

const buildEncryptedTrafficRangeParams = (timeRange: EncryptedTrafficTimeRange = '近 24 小时') => {
  const endTime = Date.now();
  return {
    start_time: endTime - encryptedTrafficRangeMilliseconds[timeRange],
    end_time: endTime,
  };
};

const dataQualityRangeMilliseconds: Record<DataQualityTimeRange, number> = {
  '近 24 小时': 24 * 60 * 60 * 1_000,
  '近 7 天': 7 * 24 * 60 * 60 * 1_000,
};

const getPageRequestParams = (pageId: string, options: PageSnapshotRequestOptions) => {
  const pagination =
    options.page && options.pageSize
      ? {
          page: options.page,
          limit: options.pageSize,
          page_size: options.pageSize,
          offset: (options.page - 1) * options.pageSize,
        }
      : {};
  if (pageId === 'graph')
    return {
      ip: options.sourceAssetIp || '10.20.4.18',
      depth: 2,
      run_id: 'realtime',
    };
  if (pageId === 'forensics')
    return {
      ...pagination,
      ...(options.forensicsFilters?.assetId || options.sourceAssetId
        ? {
            asset_id: options.forensicsFilters?.assetId || options.sourceAssetId,
          }
        : {}),
      ...(options.forensicsFilters?.srcIp ? { src_ip: options.forensicsFilters.srcIp } : {}),
      ...(options.forensicsFilters?.dstIp ? { dst_ip: options.forensicsFilters.dstIp } : {}),
      ...(options.forensicsFilters?.protocol && options.forensicsFilters.protocol !== '全部' ? { protocol: options.forensicsFilters.protocol } : {}),
      ...(options.forensicsFilters?.port && options.forensicsFilters.port !== '全部' ? { port: options.forensicsFilters.port } : {}),
      ...(options.forensicsFilters?.tuple ? { tuple: options.forensicsFilters.tuple } : {}),
      ...(options.forensicsFilters?.taskId ? { task_id: options.forensicsFilters.taskId } : {}),
    };
  if (pageId === 'campaigns')
    return {
      ...pagination,
      ...buildCampaignRequestParams(options.campaignFilters),
    };
  if (pageId === 'assets')
    return {
      ...pagination,
      ...(options.assetType ? { asset_type: options.assetType } : {}),
      ...(options.assetFilters?.status ? { status: options.assetFilters.status } : {}),
      ...(options.assetFilters?.search ? { search: options.assetFilters.search } : {}),
      ...(options.assetFilters?.department ? { department: options.assetFilters.department } : {}),
      ...(options.assetFilters?.campus ? { campus: options.assetFilters.campus } : {}),
    };
  if (pageId === 'encrypted-traffic') {
    return buildEncryptedTrafficRangeParams(options.timeRange);
  }
  if (pageId === 'probes') return { limit: 50, page_size: 50, offset: 0 };
  if (pageId === 'data-quality') {
    const endTime = Date.now();
    const timeRange = options.dataQualityTimeRange ?? '近 24 小时';
    return {
      time_range: timeRange,
      start_time: endTime - dataQualityRangeMilliseconds[timeRange],
      end_time: endTime,
    };
  }
  return pagination;
};

const getSecondaryRequestParams = (pageId: string, endpoint: string, options: PageSnapshotRequestOptions) => {
  if (pageId === 'encrypted-traffic') return buildEncryptedTrafficRangeParams(options.timeRange);
  if (pageId === 'forensics' && (endpoint === '/v1/encrypted-traffic/sessions' || endpoint === '/v1/encrypted-traffic/evidence')) {
    return buildEncryptedTrafficRangeParams('近 24 小时');
  }
  if (pageId === 'forensics' && endpoint === '/v1/audit/logs') return { object_type: 'pcap' };
  if (pageId === 'assets' && endpoint === '/v1/assets/stats')
    return {
      ...(options.assetType ? { asset_type: options.assetType } : {}),
      ...(options.assetFilters?.status ? { status: options.assetFilters.status } : {}),
      ...(options.assetFilters?.search ? { search: options.assetFilters.search } : {}),
      ...(options.assetFilters?.department ? { department: options.assetFilters.department } : {}),
      ...(options.assetFilters?.campus ? { campus: options.assetFilters.campus } : {}),
    };
  return {};
};

const campaignRiskParams: Record<string, string> = {
  高风险: 'high',
  中风险: 'medium',
  低风险: 'low',
};
const campaignStatusParams: Record<string, string> = {
  活跃中: 'active',
  调查中: 'investigating',
  已结束: 'closed',
};
const campaignPhaseParams: Record<string, string> = {
  初始访问: 'initial_access',
  执行: 'execution',
  持久化: 'persistence',
  横向移动: 'lateral_movement',
  外联通信: 'command_and_control',
  数据外传: 'exfiltration',
  影响达成: 'impact',
};

export const buildCampaignRequestParams = (filters?: CampaignSnapshotFilters) => {
  if (!filters) return {};
  const keyword = filters.keyword.trim();
  return {
    ...(campaignRiskParams[filters.risk] ? { risk: campaignRiskParams[filters.risk] } : {}),
    ...(campaignStatusParams[filters.status] ? { status: campaignStatusParams[filters.status] } : {}),
    ...(campaignPhaseParams[filters.phase] ? { phase: campaignPhaseParams[filters.phase] } : {}),
    ...(keyword ? { keyword } : {}),
  };
};

const normalizeRealSnapshot = (page: PageSpec, payload: ApiEnvelope, secondaryPayloads: unknown[]): PageSnapshot => {
  const adapted = adaptKnownPageSnapshot(page, payload, secondaryPayloads);
  if (adapted) return adapted;

  const data = unwrapPayload(payload);
  const rows = toRows(data, page);
  const total = extractTotal(payload, rows.length);
  const numericFacts = collectNumericFacts(data, page.kpis);

  return {
    id: page.id,
    metrics: page.kpis.slice(0, 8).map((label, index) => ({
      label,
      value: formatMetricValue(label, numericFacts[index] ?? (index === 0 ? total : rows.length + index * 3)),
      delta: index % 2 === 0 ? '实时' : 'API',
      status: index === 0 ? 'info' : index % 3 === 0 ? 'warn' : 'ok',
    })),
    rows,
    timeline: [
      {
        title: '真实 API 已接入',
        description: `${page.title} 生产态数据来自 ${getPageApiPlan(page.id).primary}`,
        status: 'ok',
      },
      ...secondaryPayloads.slice(0, 4).map((item, index) => ({
        title: `关联接口 ${index + 1}`,
        description: summarizePayload(item),
        status: 'info' as const,
      })),
    ],
    evidence: [
      {
        label: 'API 来源',
        value: getPageApiPlan(page.id).primary,
        status: 'ok',
      },
      {
        label: '返回记录',
        value: String(rows.length),
        status: rows.length > 0 ? 'ok' : 'warn',
      },
      {
        label: '数据模式',
        value: Array.isArray(data) ? '列表' : '对象',
        status: 'info',
      },
    ],
  };
};

const unwrapPayload = (payload: unknown): unknown => {
  if (!isRecord(payload)) return payload;
  if ('data' in payload) return unwrapPayload(payload.data);
  return payload;
};

const toRows = (payload: unknown, page: PageSpec) => {
  const source = Array.isArray(payload) ? payload : inferList(payload);
  if (source.length === 0 && isRecord(payload)) {
    return [payloadToRow(payload, page, 0)];
  }
  return source.slice(0, 8).map((item, index) => payloadToRow(item, page, index));
};

const inferList = (payload: unknown): unknown[] => {
  if (!isRecord(payload)) return [];
  for (const value of Object.values(payload)) {
    if (Array.isArray(value)) return value;
    if (isRecord(value)) {
      const nested = inferList(value);
      if (nested.length) return nested;
    }
  }
  return [];
};

const payloadToRow = (item: unknown, page: PageSpec, index: number) => {
  const record = isRecord(item) ? item : { value: item };
  const keys = Object.keys(record);
  return Object.fromEntries(
    page.tableColumns.map((column, columnIndex) => {
      const key = findMatchingKey(column, keys) ?? keys[columnIndex % Math.max(keys.length, 1)];
      const value = key ? record[key] : undefined;
      return [column, formatCell(value, page.id, column, index)];
    }),
  );
};

const findMatchingKey = (column: string, keys: string[]) => {
  const normalizedColumn = normalizeText(column);
  return keys.find((key) => normalizedColumn.includes(normalizeText(key)) || normalizeText(key).includes(normalizedColumn));
};

const collectNumericFacts = (payload: unknown, labels: string[]): number[] => {
  const numbers: number[] = [];
  const visit = (value: unknown) => {
    if (typeof value === 'number' && Number.isFinite(value)) numbers.push(value);
    if (Array.isArray(value)) value.forEach(visit);
    if (isRecord(value)) Object.values(value).forEach(visit);
  };
  visit(payload);
  return numbers.length ? numbers : labels.map((_, index) => index + 1);
};

const extractTotal = (payload: ApiEnvelope, fallback: number) => payload.total ?? payload.pagination?.total ?? payload.meta?.page?.total ?? fallback;

const formatMetricValue = (label: string, value: number) => {
  if (label.includes('率') || label.includes('健康') || label.includes('完整') || label.includes('通过')) {
    return `${Number(value).toFixed(value > 1 ? 1 : 2)}%`;
  }
  return Number.isInteger(value) ? String(value) : Number(value).toFixed(2);
};

const formatCell = (value: unknown, pageId: string, column: string, index: number) => {
  if (value === undefined || value === null || value === '') {
    if (column.includes('ID') || column.includes('对象')) return `${pageId.toUpperCase()}-${String(index + 1).padStart(4, '0')}`;
    if (column.includes('状态')) return '已接入';
    return '-';
  }
  if (typeof value === 'object') return JSON.stringify(value).slice(0, 80);
  return String(value);
};

const summarizePayload = (payload: unknown) => {
  if (isRecord(payload) && payload.secondary_error) return `非阻断关联接口失败：${payload.secondary_error}`;
  if (Array.isArray(payload)) return `返回 ${payload.length} 条关联记录`;
  if (isRecord(payload)) return `返回字段：${Object.keys(payload).slice(0, 4).join(', ')}`;
  return '关联接口已返回';
};

const normalizeText = (value: string) => value.toLowerCase().replace(/[_\-\s:/]/g, '');

const normalizeError = (error: unknown) => {
  if (axios.isAxiosError(error)) {
    return `${error.response?.status ?? 'network'} ${error.config?.url ?? ''}`.trim();
  }
  return error instanceof Error ? error.message : 'unknown error';
};

const isRecord = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);

const normalizeLoginResponse = (payload: AuthLoginResponse): LoginResult => {
  const user = normalizeCurrentUser({
    ...payload.user,
    username: payload.user?.username ?? payload.username,
    role: payload.user?.role ?? payload.role,
    roles: payload.user?.roles ?? payload.roles,
    permissions: payload.user?.permissions ?? payload.permissions,
  });
  return {
    token: payload.access_token ?? payload.token ?? '',
    refreshToken: payload.refresh_token,
    expiresIn: payload.expires_in,
    username: user.username,
    role: user.role,
    user,
  };
};

const roleLabel = (roles: string[]) => {
  if (roles.includes('admin')) return '系统管理员';
  if (roles.includes('operator')) return '安全运营员';
  if (roles.includes('analyst')) return '安全分析师';
  if (roles.includes('viewer')) return '只读观察员';
  return roles[0] ?? '安全分析师';
};

const normalizeCurrentUser = (payload: AuthUserResponse): CurrentUser => {
  const roles = payload.roles?.length ? payload.roles : payload.role ? [payload.role] : ['viewer'];
  return {
    userId: payload.user_id,
    tenantId: payload.tenant_id,
    username: payload.username ?? 'sec_analyst',
    email: payload.email,
    role: payload.role ?? roleLabel(roles),
    roles,
    permissions: payload.permissions ?? [],
  };
};

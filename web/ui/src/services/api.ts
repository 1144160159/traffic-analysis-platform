import axios from 'axios';
import { appConfig } from '@/config/runtime';
import type { PageSnapshot } from '@/services/mockData';
import { findRouteById } from '@/routes/routeManifest';
import type { PageSpec } from '@/routes/routeManifest';
import { getPageActionPlan, getPageApiPlan, getPageLoadSecondaryEndpoints } from '@/services/pageApiPlans';
import { adaptKnownPageSnapshot } from '@/services/pageSnapshotAdapters';
import { normalizeDashboardSnapshot } from '@/services/dashboardSnapshotApi';
import { clearAuthTokens, getAuthToken } from '@/services/authStorage';

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

export type AlertSnapshotFilters = {
  severity?: string;
  status?: string;
  alertType?: string;
  srcIp?: string;
  dstIp?: string;
  assetIp?: string;
  ruleVersion?: string;
  modelVersion?: string;
  attackPhase?: string;
  minScore?: number;
  startTime?: number;
  endTime?: number;
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
  sourceEntity?: string;
  sourceAlertId?: string;
  sourceCampaignId?: string;
  sourceBaselineId?: string;
  sourceEvidenceId?: string;
  sourceEvidenceType?: string;
  alertFilters?: AlertSnapshotFilters;
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

export type EntityGraphWorkbenchNode = {
  entity_id: string;
  entity_type: 'ip' | 'host' | 'account' | 'domain' | 'service' | 'alert' | 'evidence' | string;
  label: string;
  detail: string;
  risk_score: number;
  risk_level: 'high' | 'medium' | 'low' | string;
  x: number;
  y: number;
  icon: string;
  metadata: Record<string, unknown>;
  updated_at: number;
};

export type EntityGraphWorkbenchEdge = {
  relation_id: string;
  source_id: string;
  target_id: string;
  relation_type: string;
  risk_level: 'high' | 'medium' | 'low' | string;
  evidence_id?: string;
  attributes: Record<string, unknown>;
  weight: number;
  observed_at: number;
};

export type EntityGraphWorkbench = {
  center_id: string;
  nodes: EntityGraphWorkbenchNode[];
  edges: EntityGraphWorkbenchEdge[];
  meta: {
    source: string;
    node_count: number;
    edge_count: number;
    depth: number;
    entity_type: string;
    site: string;
    time_range: string;
    query_duration_ms: number;
    node_limit: number;
    cache_hit_rate: string;
    cache_applicable: boolean;
    data_origin: string;
    slow_query: boolean;
  };
};

export type EntityGraphWorkbenchFilters = {
  timeRange: '24h' | '7d' | 'all';
  site: 'main' | 'all';
  entityType: 'all' | 'ip' | 'host' | 'account' | 'domain' | 'service' | 'alert' | 'evidence';
  depth: 1 | 2 | 3;
};

export type EntityGraphWorkbenchPath = {
  mode: 'shortest' | 'attack' | 'communication' | 'account';
  source_id: string;
  target_id: string;
  node_ids: string[];
  edges: EntityGraphWorkbenchEdge[];
  length: number;
  risk_level: string;
  evidence_ids: string[];
};

export const fetchEntityGraphWorkbench = async (
  centerId: string | undefined,
  filters: EntityGraphWorkbenchFilters = { timeRange: '24h', site: 'main', entityType: 'all', depth: 2 },
): Promise<EntityGraphWorkbench> => {
  const response = await api.get<{
    data?: { graph?: EntityGraphWorkbench; meta?: EntityGraphWorkbench['meta'] };
    graph?: EntityGraphWorkbench;
    meta?: EntityGraphWorkbench['meta'];
  }>('/v1/graph/workbench', {
    params: {
      ...(centerId ? { center_id: centerId } : {}),
      time_range: filters.timeRange,
      site: filters.site,
      entity_type: filters.entityType,
      depth: filters.depth,
    },
  });
  const graph = response.data.data?.graph ?? response.data.graph;
  const meta = response.data.data?.meta ?? response.data.meta;
  if (!graph) throw new Error('实体图谱工作台接口未返回 graph 数据');
  if (!meta?.source || !meta.time_range || !meta.node_limit) throw new Error('实体图谱工作台接口未返回完整查询治理元数据');
  return {
    center_id: graph.center_id,
    nodes: Array.isArray(graph.nodes) ? graph.nodes : [],
    edges: Array.isArray(graph.edges) ? graph.edges : [],
    meta: {
      source: meta.source,
      node_count: Number(meta?.node_count ?? graph.nodes?.length ?? 0),
      edge_count: Number(meta?.edge_count ?? graph.edges?.length ?? 0),
      depth: Number(meta?.depth ?? filters.depth),
      entity_type: meta?.entity_type ?? filters.entityType,
      site: meta?.site ?? filters.site,
      time_range: meta?.time_range ?? filters.timeRange,
      query_duration_ms: Number(meta?.query_duration_ms ?? 0),
      node_limit: Number(meta.node_limit),
      cache_hit_rate: meta.cache_hit_rate,
      cache_applicable: Boolean(meta.cache_applicable),
      data_origin: meta.data_origin,
      slow_query: Boolean(meta?.slow_query),
    },
  };
};

export const fetchEntityGraphWorkbenchPath = async (params: {
  sourceId: string;
  targetId: string;
  anchorId?: string;
  mode: EntityGraphWorkbenchPath['mode'];
  maxDepth?: number;
  filters?: EntityGraphWorkbenchFilters;
}): Promise<EntityGraphWorkbenchPath> => {
  const response = await api.get<{
    data?: { path?: EntityGraphWorkbenchPath };
    path?: EntityGraphWorkbenchPath;
  }>('/v1/graph/workbench/path', {
    params: {
      source_id: params.sourceId,
      target_id: params.targetId,
      anchor_id: params.anchorId,
      mode: params.mode,
      max_depth: params.maxDepth ?? 6,
      time_range: params.filters?.timeRange,
      site: params.filters?.site,
      entity_type: params.filters?.entityType,
    },
  });
  const path = response.data.data?.path ?? response.data.path;
  if (!path) throw new Error('路径分析接口未返回 path 数据');
  return path;
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
  revision?: number;
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

export type ContractMeta = {
  contract_version: number;
  snapshot_id: string;
  as_of: string;
  trace_id: string;
  partial: boolean;
  missing_sections: string[];
  source_watermarks: Record<string, string>;
};

export type AssetResolvedIdentity = {
  kind: string;
  value: string;
  asset_revision: number;
};

export type AssetObservationSummary = {
  asset_id: string;
  resolved_identity: AssetResolvedIdentity;
  source: string;
  window_start: string;
  window_end: string;
  first_observed_at?: string;
  last_observed_at?: string;
  session_count: number;
  bytes_total: number;
  packets_total: number;
  distinct_peers: number;
  protocols: number[];
};

export type AssetAlertSummary = {
  alert_id: string;
  severity: string;
  status: string;
  alert_type: string;
  src_ip: string;
  dst_ip: string;
  src_port: number;
  dst_port: number;
  protocol: number;
  score: number;
  evidence_ids: string[];
  first_seen: string;
  last_seen: string;
  state_version: number;
  event_id: string;
};

export type AssetAlertContext = {
  asset_id: string;
  resolved_identity: AssetResolvedIdentity;
  source: string;
  window_start: string;
  window_end: string;
  alerts: AssetAlertSummary[];
  truncated: boolean;
};

export type AssetGraphProjectionRelation = {
  relation_id: string;
  source_id: string;
  target_id: string;
  relation_type: string;
  risk_level?: string;
  evidence_id?: string;
  attributes?: Record<string, unknown>;
  weight: number;
  observed_at?: string;
};

export type AssetGraphProjection = {
  asset_id: string;
  source: string;
  label: string;
  detail: string;
  risk_score: number;
  risk_level: string;
  icon: string;
  metadata: Record<string, unknown>;
  projected_revision: number;
  postgres_revision: number;
  updated_at: string;
  relations: AssetGraphProjectionRelation[];
  truncated: boolean;
  stale: boolean;
};

export type AssetEvidenceObjectManifest = {
  evidence_id: string;
  alert_id: string;
  evidence_type: string;
  summary: string;
  bucket: string;
  object_key: string;
  object_version?: string;
  content_type: string;
  size_bytes: number;
  etag?: string;
  sha256?: string;
  integrity_status: string;
  evidence_at: string;
  last_modified: string;
};

export type AssetEvidenceObjectSet = {
  asset_id: string;
  source: string;
  objects: AssetEvidenceObjectManifest[];
  missing_evidence_ids: string[];
  truncated: boolean;
  partial: boolean;
};

export type AssetDetailSnapshot = {
  contract_version: number;
  snapshot_id: string;
  asset: AssetRecord;
  details: AssetDetails;
  history: AssetEvent[];
  topology: AssetTopologyGraph;
  observations?: AssetObservationSummary;
  alert_context?: AssetAlertContext;
  graph_projection?: AssetGraphProjection;
  evidence_objects?: AssetEvidenceObjectSet;
  available_sections: string[];
  missing_sections: string[];
  partial: boolean;
  source_watermarks: Record<string, string>;
  as_of: string;
};

export type AssetDetailSnapshotEnvelope = {
  success: boolean;
  data: AssetDetailSnapshot;
  meta: ContractMeta;
  error: null;
  timestamp: string;
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

export const fetchAssetDetailSnapshot = async (assetId: string, historyLimit = 50): Promise<AssetDetailSnapshotEnvelope> => {
  if (!assetId) throw new Error('asset id required');
  const response = await api.get<AssetDetailSnapshotEnvelope>(`/v1/assets/${encodeURIComponent(assetId)}/snapshot`, {
    params: { history_limit: historyLimit },
  });
  return response.data;
};

export const fetchProbeTopology = async (mode: '2d' | '3d'): Promise<ProbeTopologyGraph> => {
  const response = await api.get<{ data: ProbeTopologyGraph }>('/v1/probes/topology', { params: { mode } });
  return response.data.data;
};

export const fetchPageSnapshot = async (pageId: string, options: PageSnapshotRequestOptions = {}): Promise<PageSnapshot> => {
  const route = findRouteById(pageId);
  if (!route) throw new Error(`Unknown page: ${pageId}`);

  if (appConfig.useMock) {
    const response = await api.get<PageSnapshot>(`/v1/ui/pages/${pageId}`);
    return response.data;
  }

  return fetchRealPageSnapshot(route.page, options);
};

export type TopicActionResult = {
  action_id: string;
  job_id?: string;
  tenant_id: string;
  topic: 'tunnel' | 'exfil' | 'apt';
  action?: string;
  label?: string;
  target: string;
  data_mode?: 'live' | 'partial' | 'simulated';
  status: string;
  snapshot_id?: string;
  expected_revision?: number;
  revision?: number;
  executor?: string;
  trace_id?: string;
  receipt?: Record<string, unknown>;
  error?: Record<string, unknown>;
  business_effect?: {
    operation: string;
    state: string;
    result_type: string;
    message: string;
    next_route?: string;
    evidence_ref?: string;
  };
  requested_by: string;
  created_at: number;
};

const topicActionKey = (topic: string): TopicActionResult['topic'] => {
  const normalized = topic.trim().toLowerCase();
  if (normalized.startsWith('exfil')) return 'exfil';
  if (normalized.startsWith('apt') || normalized.startsWith('campaign')) return 'apt';
  return 'tunnel';
};

const topicActionCode = (label: string) => {
  const mappings: Array<[RegExp, string]> = [
    [/PCAP|取证/u, 'extract_pcap'],
    [/Session|会话/u, 'inspect_session'],
    [/证书/u, 'inspect_certificate'],
    [/回溯路径/u, 'trace_path'],
    [/阻断|隔离|停止/u, 'contain'],
    [/白名单|例外/u, 'review_exception'],
    [/审计/u, 'write_audit'],
    [/规则|模型/u, 'link_rule'],
    [/攻击链|下钻|溯源/u, 'trace'],
    [/全量详情|详情/u, 'inspect_detail'],
    [/关联告警/u, 'inspect_alerts'],
    [/观察|监控/u, 'monitor'],
    [/复核/u, 'review'],
    [/订阅/u, 'subscribe'],
    [/静默/u, 'mute'],
    [/分享/u, 'share'],
    [/收藏/u, 'favorite'],
    [/布局/u, 'change_layout'],
    [/全屏/u, 'focus_view'],
  ];
  return mappings.find(([pattern]) => pattern.test(label))?.[1] ?? 'inspect_detail';
};

export type TopicDataContext = {
  data_mode: 'live' | 'partial' | 'simulated';
  snapshot_id?: string;
  expected_revision?: number;
  simulation_id?: string;
  simulation_version?: string;
  scope_snapshot?: Record<string, unknown>;
  view_state?: Record<string, unknown>;
};

export const submitTopicAction = async (
  topic: string,
  label: string,
  target: string,
  context?: TopicDataContext,
): Promise<TopicActionResult> => {
  const topicKey = topicActionKey(topic);
  const actionId = topicActionCode(label);
  if (context?.snapshot_id && context.expected_revision) {
    const idempotencyKey = globalThis.crypto?.randomUUID?.()
      ?? `${topicKey}-${actionId}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const response = await api.post<{ data: TopicActionResult }>(
      `/v1/topics/${topicKey}/actions`,
      {
        action_id: actionId,
        label,
        target,
        snapshot_id: context.snapshot_id,
        expected_revision: context.expected_revision,
        reason: `topic-workbench:${label}`,
        detail: {
          source: 'topic-workbench',
          view_state: context.view_state,
        },
      },
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    let result = response.data.data;
    if (!result.job_id || !['accepted', 'running'].includes(result.status)) return result;
    const jobId = result.job_id;
    for (let attempt = 0; attempt < 60; attempt += 1) {
      await new Promise((resolve) => window.setTimeout(resolve, 500));
      result = await fetchTopicActionJob(topicKey, jobId);
      if (!['accepted', 'running', 'compensating'].includes(result.status)) break;
    }
    if (!['completed', 'partial'].includes(result.status)) {
      const message = typeof result.error?.message === 'string' ? result.error.message : `专题任务状态：${result.status}`;
      throw new Error(message);
    }
    return {
      ...result,
      business_effect: {
        operation: result.action_id,
        state: result.status,
        result_type: 'executor_receipt',
        message: result.status === 'completed' ? '专题任务已由执行器完成并生成回执' : '专题任务部分完成',
      },
    };
  }
  const response = await api.post<{ data?: TopicActionResult } & Partial<TopicActionResult>>(`/v1/topics/${topicKey}/actions`, {
    action: actionId,
    label,
    target,
    data_mode: context?.data_mode ?? 'live',
    detail: {
      source: 'topic-workbench',
      simulation_id: context?.simulation_id,
      simulation_version: context?.simulation_version,
      scope_snapshot: context?.scope_snapshot,
      view_state: context?.view_state,
    },
  });
  return (response.data.data ?? response.data) as TopicActionResult;
};

export const fetchTopicActionJob = async (topic: string, jobId: string): Promise<TopicActionResult> => {
  const topicKey = topicActionKey(topic);
  const response = await api.get<{ data: TopicActionResult }>(
    `/v1/topics/${topicKey}/actions/${encodeURIComponent(jobId)}`,
  );
  return response.data.data;
};

export type TopicSavedView = {
  view_id: string;
  topic: TopicActionResult['topic'];
  name: string;
  filters: Record<string, unknown>;
  visibility: string;
  favorite: boolean;
  shared: boolean;
  share_token?: string;
  created_at: number;
  updated_at: number;
};

export type TopicScope = {
  topic: TopicActionResult['topic'];
  scope_name: string;
  included_assets: string[];
  excluded_assets: string[];
  risk_levels: string[];
  time_window: string;
  detail: Record<string, unknown>;
  updated_at: number;
};

export const fetchTopicScope = async (topic: string): Promise<TopicScope> => {
  const topicKey = topicActionKey(topic);
  const response = await api.get<{ data: TopicScope }>(`/v1/topics/scopes/${topicKey}`);
  return response.data.data;
};

export type TopicSubscription = {
  subscription_id: string;
  topic: TopicActionResult['topic'];
  channel: string;
  threshold: string;
  schedule: string;
  recipients: string[];
  enabled: boolean;
  created_at: number;
  updated_at: number;
};

export type TopicExport = {
  export_id: string;
  topic: TopicActionResult['topic'];
  export_type: 'report' | 'evidence_package';
  status: string;
  parameters: Record<string, unknown>;
  result: Record<string, unknown>;
  generated_at: number;
};

export const saveTopicView = async (
  topic: string,
  input: { name: string; visibility: string; favorite: boolean; filters: Record<string, unknown> },
): Promise<TopicSavedView> => {
  const response = await api.post<{ data: TopicSavedView }>('/v1/topics/views', {
    topic: topicActionKey(topic),
    ...input,
  });
  const view = response.data.data;
  if (typeof window !== 'undefined') {
    window.sessionStorage.setItem(`taf:topic:${view.topic}:current-view-id`, view.view_id);
  }
  return view;
};

export const updateTopicScope = async (
  topic: string,
  input: {
    scope_name: string;
    included_assets: string[];
    excluded_assets: string[];
    risk_levels: string[];
    time_window: string;
    detail: Record<string, unknown>;
  },
): Promise<TopicScope> => {
  const topicKey = topicActionKey(topic);
  const response = await api.put<{ data: TopicScope }>(`/v1/topics/scopes/${topicKey}`, input);
  return response.data.data;
};

export const createTopicSubscription = async (
  topic: string,
  input: { channel: string; threshold: string; schedule: string; recipients: string[] },
): Promise<TopicSubscription> => {
  const response = await api.post<{ data: TopicSubscription }>('/v1/topics/subscriptions', {
    topic: topicActionKey(topic),
    enabled: true,
    ...input,
  });
  return response.data.data;
};

export const exportTopicArtifact = async (
  topic: string,
  exportType: 'report' | 'evidence_package',
  format: string,
  context?: TopicDataContext,
  sourceExportId?: string,
): Promise<TopicExport> => {
  const endpoint = exportType === 'report' ? '/v1/topics/reports/export' : '/v1/topics/evidence-packages/export';
  const response = await api.post<{ data: TopicExport }>(endpoint, {
    topic: topicActionKey(topic),
    format,
    source_export_id: sourceExportId || undefined,
    parameters: {
      source: 'topic-workbench',
      visual_state: topicActionKey(topic),
      data_mode: context?.data_mode ?? 'live',
      simulation_id: context?.simulation_id,
      simulation_version: context?.simulation_version,
      scope_snapshot: context?.scope_snapshot,
      view_state: context?.view_state,
    },
  });
  return response.data.data;
};

export const updateTopicViewPreference = async (
  topic: string,
  preference: 'favorite' | 'shared',
): Promise<TopicSavedView> => {
  const topicKey = topicActionKey(topic);
  const storageKey = `taf:topic:${topicKey}:current-view-id`;
  let viewID = typeof window !== 'undefined' ? window.sessionStorage.getItem(storageKey) : '';
  if (!viewID) {
    const view = await saveTopicView(topicKey, {
      name: `${topicKey}-当前专题视图`,
      visibility: 'private',
      favorite: false,
      filters: { topic: topicKey, source: 'topic-workbench' },
    });
    viewID = view.view_id;
  }
  const response = await api.patch<{ data: TopicSavedView }>(`/v1/topics/views/${encodeURIComponent(viewID)}`, {
    [preference]: true,
  });
  return response.data.data;
};

export type ApiEnvelope = {
  data?: unknown;
  total?: number;
  pagination?: { total?: number };
  meta?: {
    page?: { total?: number };
    contract_version?: number;
    snapshot_id?: string;
    as_of?: string;
    trace_id?: string;
    partial?: boolean;
    missing_sections?: string[];
    source_watermarks?: Record<string, string>;
  };
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
  operation_type?: string;
  command_revision?: number;
  state_revision?: number;
  desired_version?: string;
  command_hash?: string;
  reported_version?: string;
  reported_hash?: string;
  agent_version?: string;
  ack_error?: string;
  trace_id?: string;
  expires_at?: number;
  acknowledged_at?: number;
  outbox_published?: boolean;
  accepted_count?: number;
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
  const requestNonce = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  const response = await api.post<{ data?: ProbeOperationResult } & Partial<ProbeOperationResult>>(endpoint, body, {
    headers: { 'Idempotency-Key': `probe:${actionId}:${requestNonce}` },
  });
  return (response.data.data ?? response.data) as ProbeOperationResult;
};

export const fetchProbeOperation = async (operationId: string): Promise<ProbeOperationResult> => {
  const normalized = operationId.trim();
  if (!normalized) throw new Error('operation_id 不能为空');
  const response = await api.get<{ data?: ProbeOperationResult } & Partial<ProbeOperationResult>>(
    `/v1/probes/operations/${encodeURIComponent(normalized)}`,
  );
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
  alertId?: string;
  campaignId?: string;
  baselineId?: string;
  evidenceId?: string;
  evidenceType?: string;
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
    alert_id: input.alertId || undefined,
    campaign_id: input.campaignId || undefined,
    baseline_id: input.baselineId || undefined,
    evidence_id: input.evidenceId || undefined,
    evidence_type: input.evidenceType || undefined,
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
  if (page.id === 'dashboard') {
    const response = await api.get<ApiEnvelope>(plan.primary, { params: requestParams });
    return normalizeDashboardSnapshot(response.data);
  }
  const secondaryEndpoints = getPageLoadSecondaryEndpoints(page.id);
  const isTopicSnapshot = page.id === 'topic-tunnel' || page.id === 'topic-exfil' || page.id === 'topic-apt';
  const primaryPageSize = isTopicSnapshot ? 200 : 8;
  const [primary, ...secondary] = await Promise.all([
    api.get<ApiEnvelope>(plan.primary, {
      params: { limit: primaryPageSize, page_size: primaryPageSize, ...requestParams },
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
      ...(options.sourceAssetIp ? { ip: options.sourceAssetIp } : {}),
      depth: 2,
      run_id: 'realtime',
    };
  if (pageId === 'alerts')
    return {
      ...pagination,
      ...(options.alertFilters?.severity ? { severity: options.alertFilters.severity } : {}),
      ...(options.alertFilters?.status ? { status: options.alertFilters.status } : {}),
      ...(options.alertFilters?.alertType ? { alert_type: options.alertFilters.alertType } : {}),
      ...(options.alertFilters?.srcIp ? { src_ip: options.alertFilters.srcIp } : {}),
      ...(options.alertFilters?.dstIp ? { dst_ip: options.alertFilters.dstIp } : {}),
      ...(options.alertFilters?.assetIp ? { asset_ip: options.alertFilters.assetIp } : {}),
      ...(options.alertFilters?.ruleVersion ? { rule_version: options.alertFilters.ruleVersion } : {}),
      ...(options.alertFilters?.modelVersion ? { model_version: options.alertFilters.modelVersion } : {}),
      ...(options.alertFilters?.attackPhase ? { attack_phase: options.alertFilters.attackPhase } : {}),
      ...(options.alertFilters?.minScore ? { min_score: options.alertFilters.minScore } : {}),
      ...(options.alertFilters?.startTime ? { start_time: options.alertFilters.startTime } : {}),
      ...(options.alertFilters?.endTime ? { end_time: options.alertFilters.endTime } : {}),
      ...(options.sourceEntity && /^\d{1,3}(?:\.\d{1,3}){3}$/.test(options.sourceEntity)
        ? { src_ip: options.sourceEntity }
        : {}),
    };
  if (pageId === 'forensics')
    return {
      ...pagination,
      ...(options.forensicsFilters?.assetId || options.sourceAssetId
        ? {
            asset_id: options.forensicsFilters?.assetId || options.sourceAssetId,
          }
        : {}),
      ...(options.sourceAlertId ? { alert_id: options.sourceAlertId } : {}),
      ...(options.sourceCampaignId ? { campaign_id: options.sourceCampaignId } : {}),
      ...(options.sourceBaselineId ? { baseline_id: options.sourceBaselineId } : {}),
      ...(options.sourceEvidenceId ? { evidence_id: options.sourceEvidenceId } : {}),
      ...(options.sourceEvidenceType ? { evidence_type: options.sourceEvidenceType } : {}),
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
  if (pageId === 'alerts')
    return {
      ...(options.alertFilters?.startTime ? { start_time: options.alertFilters.startTime } : {}),
      ...(options.alertFilters?.endTime ? { end_time: options.alertFilters.endTime } : {}),
      ...(endpoint === '/v1/alerts/trend' ? { interval: 'hour' } : {}),
    };
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

  return normalizeUnadaptedPageSnapshot(page, payload, secondaryPayloads);
};

export const normalizeUnadaptedPageSnapshot = (
  page: PageSpec,
  payload: ApiEnvelope,
  secondaryPayloads: unknown[],
): PageSnapshot => {
  const explicitTotal = payload.total ?? payload.pagination?.total ?? payload.meta?.page?.total;
  const meta = payload.meta;
  const missingSections = [...new Set([...(meta?.missing_sections ?? []), 'typed_page_adapter'])];
  const hasSnapshotMeta = Boolean(
    meta?.contract_version
      && meta.snapshot_id
      && meta.as_of
      && meta.trace_id,
  );

  return {
    id: page.id,
    ...(explicitTotal !== undefined ? { total: explicitTotal } : {}),
    metrics: page.kpis.slice(0, 8).map((label) => ({
      label,
      value: '暂不可用',
      delta: '缺少类型化页面适配器',
      status: 'warn',
    })),
    rows: [],
    timeline: [
      {
        title: '类型化页面适配器缺失',
        description: `${page.title} 已收到 ${getPageApiPlan(page.id).primary} 响应，但未将未知字段猜测为 KPI、表格或业务关系。`,
        status: 'warn',
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
        value: explicitTotal === undefined ? '暂不可用' : String(explicitTotal),
        status: explicitTotal === undefined ? 'warn' : 'info',
      },
      {
        label: '页面状态',
        value: 'partial / typed adapter missing',
        status: 'warn',
      },
    ],
    ...(hasSnapshotMeta
      ? {
          snapshot: {
            contractVersion: Number(meta?.contract_version),
            snapshotId: String(meta?.snapshot_id),
            asOf: String(meta?.as_of),
            traceId: String(meta?.trace_id),
            partial: true,
            missingSections,
            sourceWatermarks: meta?.source_watermarks ?? {},
          },
        }
      : {}),
  };
};

const summarizePayload = (payload: unknown) => {
  if (isRecord(payload) && payload.secondary_error) return `非阻断关联接口失败：${payload.secondary_error}`;
  if (Array.isArray(payload)) return `返回 ${payload.length} 条关联记录`;
  if (isRecord(payload)) return `返回字段：${Object.keys(payload).slice(0, 4).join(', ')}`;
  return '关联接口已返回';
};


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

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

export type PageSnapshotRequestOptions = {
  timeRange?: EncryptedTrafficTimeRange;
  dataQualityTimeRange?: DataQualityTimeRange;
  page?: number;
  pageSize?: number;
  campaignFilters?: CampaignSnapshotFilters;
};

export const fetchPageSnapshot = async (pageId: string, options: PageSnapshotRequestOptions = {}): Promise<PageSnapshot> => {
  const route = findRouteById(pageId);
  if (!route) throw new Error(`Unknown page: ${pageId}`);

  if (isVisualBreakdownMode()) {
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

type EncryptedTrafficEgressActionId =
  | 'egress-create-alert'
  | 'egress-evidence-lookup'
  | 'egress-entity-graph'
  | 'egress-audit-write'
  | 'egress-response-request';

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

export const submitEncryptedTrafficEgressAction = async ({
  actionId,
  target,
  dataMode,
}: EncryptedTrafficEgressActionInput): Promise<EncryptedTrafficEgressActionResult> => {
  const plan = getPageActionPlan('encrypted-traffic', actionId);
  if (!plan || plan.method !== 'POST') throw new Error(`未找到外联处置 API：${actionId}`);
  const response = await api.post<{ data?: EncryptedTrafficEgressActionResult } & EncryptedTrafficEgressActionResult>(plan.endpoint, {
    ...(plan.defaultBody ?? {}),
    target,
    data_mode: dataMode,
  });
  return response.data.data ?? response.data;
};

type EncryptedTrafficEvidenceActionId =
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

export const submitEncryptedTrafficEvidenceAction = async ({
  actionId,
  target,
  dataMode,
}: EncryptedTrafficEvidenceActionInput): Promise<EncryptedTrafficEvidenceActionResult> => {
  const plan = getPageActionPlan('encrypted-traffic', actionId);
  if (!plan || plan.method !== 'POST') throw new Error(`未找到证据中心动作 API：${actionId}`);
  const response = await api.post<{ data?: EncryptedTrafficEvidenceActionResult } & EncryptedTrafficEvidenceActionResult>(plan.endpoint, {
    ...(plan.defaultBody ?? {}),
    target,
    data_mode: dataMode,
  });
  return response.data.data ?? response.data;
};

const fetchRealPageSnapshot = async (page: PageSpec, options: PageSnapshotRequestOptions): Promise<PageSnapshot> => {
  const plan = getPageApiPlan(page.id);
  const requestParams = getPageRequestParams(page.id, options);
  const secondaryEndpoints = getPageLoadSecondaryEndpoints(page.id);
  const [primary, ...secondary] = await Promise.all([
    api.get<ApiEnvelope>(plan.primary, { params: { limit: 8, page_size: 8, ...requestParams } }),
    ...secondaryEndpoints.map((endpoint) =>
      api
        .get<ApiEnvelope>(endpoint, { params: { limit: 8, page_size: 8, ...requestParams } })
        .then((response) => response)
        .catch((error: unknown) => ({ data: { secondary_error: normalizeError(error) } })),
    ),
  ]);

  return normalizeRealSnapshot(page, primary.data, secondary.map((response) => response.data));
};

const encryptedTrafficRangeMilliseconds: Record<EncryptedTrafficTimeRange, number> = {
  '近 1 小时': 60 * 60 * 1_000,
  '近 24 小时': 24 * 60 * 60 * 1_000,
  '近 7 天': 7 * 24 * 60 * 60 * 1_000,
};

const dataQualityRangeMilliseconds: Record<DataQualityTimeRange, number> = {
  '近 24 小时': 24 * 60 * 60 * 1_000,
  '近 7 天': 7 * 24 * 60 * 60 * 1_000,
};

const getPageRequestParams = (pageId: string, options: PageSnapshotRequestOptions) => {
  const pagination = options.page && options.pageSize
    ? { page: options.page, limit: options.pageSize, page_size: options.pageSize, offset: (options.page - 1) * options.pageSize }
    : {};
  if (pageId === 'graph') return { ip: '10.20.4.18', depth: 2, run_id: 'realtime' };
  if (pageId === 'campaigns') return { ...pagination, ...buildCampaignRequestParams(options.campaignFilters) };
  if (pageId === 'encrypted-traffic') {
    const endTime = Date.now();
    const timeRange = options.timeRange ?? '近 24 小时';
    return {
      start_time: endTime - encryptedTrafficRangeMilliseconds[timeRange],
      end_time: endTime,
    };
  }
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

const campaignRiskParams: Record<string, string> = { 高风险: 'high', 中风险: 'medium', 低风险: 'low' };
const campaignStatusParams: Record<string, string> = { 活跃中: 'active', 调查中: 'investigating', 已结束: 'closed' };
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

const extractTotal = (payload: ApiEnvelope, fallback: number) =>
  payload.total ?? payload.pagination?.total ?? payload.meta?.page?.total ?? fallback;

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

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

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

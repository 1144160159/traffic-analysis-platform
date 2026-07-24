import { api } from '@/services/api';

export type SystemSite = {
  id: string;
  parent_id?: string;
  name: string;
  cidr?: string;
  kind: string;
  isolation_status: 'isolated' | 'partial' | 'unisolated' | string;
};

export type RetentionPolicy = {
  data_type: string;
  retention_days: number;
  action: 'delete' | 'archive' | string;
  status: 'healthy' | 'expiring' | string;
  next_action: string;
  description?: string;
};

export type IntegrationSetting = {
  id: string;
  name: string;
  enabled: boolean;
  status: 'healthy' | 'disabled' | 'degraded' | string;
  secret_ref?: string;
  endpoint_hint?: string;
  last_tested_at?: string;
};

export type SecuritySettings = {
  login_policy: string;
  password_policy: string;
  mfa_enabled: boolean;
  ip_access_rules: number;
  masking_policy: string;
  default_time_range: string;
  alert_threshold: string;
  refresh_interval_sec: number;
  screen_masking: boolean;
  feature_flags: string[];
};

export type TenantSystemSettings = {
  sites: SystemSite[];
  retention_policies: RetentionPolicy[];
  integrations: IntegrationSetting[];
  security: SecuritySettings;
};

export type SystemRole = {
  id: string;
  name: string;
  description: string;
  permissions: string[];
  system: boolean;
};

export type SystemTokenSummary = {
  total: number;
  active: number;
  expiring_soon: number;
  revoked: number;
};

export type SystemSettingsWorkbench = {
  tenant_id: string;
  tenant_name: string;
  tenant_status: string;
  revision: number;
  settings: TenantSystemSettings;
  roles: SystemRole[];
  tokens: SystemTokenSummary;
  updated_at: string;
};

export type SystemSettingsAction = 'scope-review' | 'connection-test' | 'test-integration' | 'security-audit' | 'lifecycle-review';

export type SystemSettingsActionResult = {
  action: SystemSettingsAction;
  status: 'success' | 'warning' | string;
  message: string;
  revision: number;
  updated_at: string;
  findings?: string[];
  integrations?: IntegrationSetting[];
  tokens?: SystemTokenSummary;
  roles?: SystemRole[];
};

export type SystemSettingsImpact = {
  tenant_id: string;
  revision: number;
  affected_scopes: string[];
  risk: string;
  approval: string;
  audit_action: string;
  summary: string;
};

export type ApiTokenCreateInput = {
  name: string;
  description?: string;
  scopes: string[];
  expires_in_sec: number;
};

export type ApiTokenCreated = {
  token_id: string;
  token: string;
  token_prefix: string;
  name: string;
  scopes: string[];
  expires_at?: string;
  created_at: string;
};

export type ApiTokenRegenerated = ApiTokenCreated;

export type TokenScopeOption = {
  name: string;
  description?: string;
  category?: string;
};

export const fetchSystemSettingsWorkbench = async () => {
  const response = await api.get<SystemSettingsWorkbench>('/v1/auth/system-settings');
  return response.data;
};

export const fetchSettingsTokenScopes = async () => {
  const response = await api.get<{ scopes?: TokenScopeOption[] }>('/v1/tokens/scopes');
  return response.data.scopes ?? [];
};

export const saveSystemSettings = async (revision: number, settings: TenantSystemSettings) => {
  const response = await api.put<SystemSettingsWorkbench>('/v1/auth/system-settings', {
    expected_revision: revision,
    settings,
  });
  return response.data;
};

export const runSystemSettingsAction = async (action: SystemSettingsAction, revision: number, targetId?: string) => {
  const response = await api.post<SystemSettingsActionResult>(`/v1/auth/system-settings/actions/${action}`, {
    expected_revision: revision,
    target_id: targetId,
  });
  return response.data;
};

export const fetchSystemSettingsImpact = async () => {
  const response = await api.get<SystemSettingsImpact>('/v1/auth/system-settings/impact');
  return response.data;
};

export const createSettingsToken = async (input: ApiTokenCreateInput) => {
  const response = await api.post<ApiTokenCreated>('/v1/tokens', input);
  return response.data;
};

export const regenerateSettingsToken = async (tokenId: string) => {
  const response = await api.post<ApiTokenRegenerated>(`/v1/tokens/${encodeURIComponent(tokenId)}/regenerate`);
  return response.data;
};

export const revokeSettingsToken = async (tokenId: string) => {
  const response = await api.post<{ message: string }>(`/v1/tokens/${encodeURIComponent(tokenId)}/revoke`);
  return response.data;
};

export const updateSettingsTokenScopes = async (tokenId: string, scopes: string[]) => {
  const response = await api.put<{ message: string }>(`/v1/tokens/${encodeURIComponent(tokenId)}/scopes`, { scopes });
  return response.data;
};

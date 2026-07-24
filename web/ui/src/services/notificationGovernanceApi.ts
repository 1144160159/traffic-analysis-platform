import { api } from '@/services/api';

export type NotificationChannelKey = 'email' | 'webhook' | 'wechat' | 'dingtalk' | 'slack' | 'feishu';

export type NotificationSettings = {
  enabled: boolean;
  min_severity: string;
  rate_limit_per_min: number;
  secret_ref: string;
  channels: Record<NotificationChannelKey, boolean>;
};

export type NotificationRule = {
  rule_id: string;
  tenant_id: string;
  name: string;
  conditions: Record<string, unknown>;
  channels: NotificationChannelKey[];
  enabled: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type NotificationTemplate = {
  template_id: string;
  tenant_id: string;
  template_type: string;
  name: string;
  version: number;
  subject: string;
  body: string;
  variable_schema: Record<string, unknown>;
  validation_status: string;
  enabled: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type NotificationEscalationPolicy = {
  policy_id: string;
  tenant_id: string;
  name: string;
  stages: Array<{ after_minutes?: number; condition?: string; target_role?: string }>;
  enabled: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type NotificationDelivery = {
  notification_id: number;
  tenant_id: string;
  rule_id?: string;
  alert_id: string;
  target_name: string;
  channel: NotificationChannelKey | string;
  alert_type: string;
  status: string;
  error_message?: string;
  retry_count: number;
  trace_id: string;
  sent_at?: string;
  created_at: string;
};

export type NotificationSilenceRule = {
  rule_id: string;
  tenant_id: string;
  name: string;
  scope: string;
  starts_at: string;
  ends_at: string;
  affected_targets: string[];
  policy: string;
  reason: string;
  enabled: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type NotificationWorkbench = {
  settings: NotificationSettings;
  rules: NotificationRule[];
  templates: NotificationTemplate[];
  escalation_policies: NotificationEscalationPolicy[];
  deliveries: NotificationDelivery[];
  silence_rules: NotificationSilenceRule[];
};

export type NotificationAuditEvent = {
  log_id: string;
  action: string;
  object_type: string;
  object_id: string;
  result: string;
  timestamp: number;
  details?: Record<string, unknown>;
};

type Envelope<T> = { success: boolean; data: T };

const unwrap = <T>(payload: Envelope<T>) => payload.data;

export const fetchNotificationWorkbench = async (): Promise<NotificationWorkbench> =>
  unwrap((await api.get<Envelope<NotificationWorkbench>>('/v1/notifications/workbench', { params: { limit: 100 } })).data);

export const updateNotificationSettings = async (settings: Partial<NotificationSettings>): Promise<NotificationSettings> =>
  unwrap((await api.put<Envelope<NotificationSettings>>('/v1/notifications/settings', settings)).data);

export const testNotificationChannel = async (channel: NotificationChannelKey, target: string, alertType = 'scan'): Promise<NotificationDelivery | null> =>
  unwrap((await api.post<Envelope<NotificationDelivery | null> & { message: string }>('/v1/notifications/test', { channel, target, alert_type: alertType })).data);

export const createNotificationRule = async (payload: Pick<NotificationRule, 'name' | 'conditions' | 'channels' | 'enabled'>): Promise<NotificationRule> =>
  unwrap((await api.post<Envelope<NotificationRule>>('/v1/notifications/subscriptions', payload)).data);

export const patchNotificationRule = async (ruleId: string, payload: Partial<Pick<NotificationRule, 'name' | 'conditions' | 'channels' | 'enabled'>>): Promise<NotificationRule> =>
  unwrap((await api.patch<Envelope<NotificationRule>>(`/v1/notifications/subscriptions/${encodeURIComponent(ruleId)}`, payload)).data);

export const createNotificationTemplate = async (payload: Pick<NotificationTemplate, 'template_type' | 'name' | 'subject' | 'body' | 'variable_schema' | 'enabled'>): Promise<NotificationTemplate> =>
  unwrap((await api.post<Envelope<NotificationTemplate>>('/v1/notifications/templates', payload)).data);

export const patchNotificationTemplate = async (templateId: string, payload: Partial<Pick<NotificationTemplate, 'template_type' | 'name' | 'subject' | 'body' | 'variable_schema' | 'enabled'>>): Promise<NotificationTemplate> =>
  unwrap((await api.patch<Envelope<NotificationTemplate>>(`/v1/notifications/templates/${encodeURIComponent(templateId)}`, payload)).data);

export const testNotificationTemplate = async (templateId: string): Promise<{ template: NotificationTemplate; delivery: NotificationDelivery }> =>
  unwrap((await api.post<Envelope<{ template: NotificationTemplate; delivery: NotificationDelivery }>>(`/v1/notifications/templates/${encodeURIComponent(templateId)}/test`)).data);

export const createNotificationEscalationPolicy = async (payload: Pick<NotificationEscalationPolicy, 'name' | 'stages' | 'enabled'>): Promise<NotificationEscalationPolicy> =>
  unwrap((await api.post<Envelope<NotificationEscalationPolicy>>('/v1/notifications/escalation-policies', payload)).data);

export const patchNotificationEscalationPolicy = async (policyId: string, payload: Partial<Pick<NotificationEscalationPolicy, 'name' | 'stages' | 'enabled'>>): Promise<NotificationEscalationPolicy> =>
  unwrap((await api.patch<Envelope<NotificationEscalationPolicy>>(`/v1/notifications/escalation-policies/${encodeURIComponent(policyId)}`, payload)).data);

export const retryNotificationDelivery = async (notificationId: number): Promise<NotificationDelivery> =>
  unwrap((await api.post<Envelope<NotificationDelivery>>(`/v1/notifications/deliveries/${notificationId}/retry`)).data);

export const createNotificationSilenceRule = async (payload: Pick<NotificationSilenceRule, 'name' | 'scope' | 'starts_at' | 'ends_at' | 'affected_targets' | 'policy' | 'reason'> & { enabled?: boolean }): Promise<NotificationSilenceRule> =>
  unwrap((await api.post<Envelope<NotificationSilenceRule>>('/v1/notifications/silence-rules', payload)).data);

export const patchNotificationSilenceRule = async (
  ruleId: string,
  payload: Partial<Pick<NotificationSilenceRule, 'name' | 'scope' | 'starts_at' | 'ends_at' | 'affected_targets' | 'policy' | 'reason' | 'enabled'>>,
): Promise<NotificationSilenceRule> =>
  unwrap((await api.patch<Envelope<NotificationSilenceRule>>(`/v1/notifications/silence-rules/${encodeURIComponent(ruleId)}`, payload)).data);

export const fetchNotificationAudits = async (): Promise<NotificationAuditEvent[]> => {
  const response = await api.get<{ data?: { trails?: NotificationAuditEvent[] } }>('/v1/audit/logs', { params: { limit: 100 } });
  return (response.data.data?.trails ?? []).filter((event) => event.action.startsWith('NOTIFICATION_'));
};

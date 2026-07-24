import { beforeEach, describe, expect, it, vi } from 'vitest';

const get = vi.fn();
const post = vi.fn();
const put = vi.fn();
const patch = vi.fn();

vi.mock('@/services/api', () => ({ api: { get, post, put, patch } }));

describe('notification governance API', () => {
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
    put.mockReset();
    patch.mockReset();
  });

  it('loads the complete PostgreSQL-backed notification workbench', async () => {
    const workbench = { settings: { channels: {} }, rules: [], templates: [], escalation_policies: [], deliveries: [], silence_rules: [] };
    get.mockResolvedValue({ data: { success: true, data: workbench } });
    const client = await import('./notificationGovernanceApi');
    expect(await client.fetchNotificationWorkbench()).toBe(workbench);
    expect(get).toHaveBeenCalledWith('/v1/notifications/workbench', { params: { limit: 100 } });
  });

  it('persists channel settings and sends a channel-specific test', async () => {
    put.mockResolvedValue({ data: { success: true, data: { channels: { email: true } } } });
    post.mockResolvedValue({ data: { success: true, data: { notification_id: 42 } } });
    const client = await import('./notificationGovernanceApi');
    await client.updateNotificationSettings({ channels: { email: true } as never });
    await client.testNotificationChannel('email', '安全值班组', 'scan');
    expect(put).toHaveBeenCalledWith('/v1/notifications/settings', { channels: { email: true } });
    expect(post).toHaveBeenCalledWith('/v1/notifications/test', { channel: 'email', target: '安全值班组', alert_type: 'scan' });
  });

  it('binds rule, template, escalation, delivery and silence mutations to encoded ids', async () => {
    patch.mockResolvedValue({ data: { success: true, data: {} } });
    post.mockResolvedValue({ data: { success: true, data: {} } });
    const client = await import('./notificationGovernanceApi');
    await client.patchNotificationRule('rule/1', { enabled: false });
    await client.patchNotificationTemplate('template/1', { enabled: false });
    await client.patchNotificationEscalationPolicy('policy/1', { enabled: false });
    await client.retryNotificationDelivery(41);
    await client.patchNotificationSilenceRule('silence/1', { enabled: false });
    expect(patch).toHaveBeenNthCalledWith(1, '/v1/notifications/subscriptions/rule%2F1', { enabled: false });
    expect(patch).toHaveBeenNthCalledWith(2, '/v1/notifications/templates/template%2F1', { enabled: false });
    expect(patch).toHaveBeenNthCalledWith(3, '/v1/notifications/escalation-policies/policy%2F1', { enabled: false });
    expect(post).toHaveBeenCalledWith('/v1/notifications/deliveries/41/retry');
    expect(patch).toHaveBeenNthCalledWith(4, '/v1/notifications/silence-rules/silence%2F1', { enabled: false });
  });

  it('filters the audit log to notification actions', async () => {
    get.mockResolvedValue({ data: { data: { trails: [{ log_id: 'a1', action: 'NOTIFICATION_RULE_UPDATED' }, { log_id: 'a2', action: 'AUDIT_EXPORTED' }] } } });
    const client = await import('./notificationGovernanceApi');
    expect(await client.fetchNotificationAudits()).toEqual([{ log_id: 'a1', action: 'NOTIFICATION_RULE_UPDATED' }]);
    expect(get).toHaveBeenCalledWith('/v1/audit/logs', { params: { limit: 100 } });
  });
});

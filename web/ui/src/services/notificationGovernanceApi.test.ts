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
    await client.updateNotificationSettings({ channels: { email: true } as never }, 5, '启用邮件通知');
    await client.testNotificationChannel('email', '安全值班组', 'scan');
    const settingsCall = put.mock.calls[0];
    expect(settingsCall[0]).toBe('/v1/notifications/settings');
    expect(settingsCall[1]).toEqual(expect.objectContaining({ channels: { email: true }, expected_revision: 5, reason: '启用邮件通知', action_id: expect.any(String) }));
    expect(settingsCall[2]).toEqual({ headers: { 'Idempotency-Key': settingsCall[1].action_id } });
    expect(post).toHaveBeenCalledWith('/v1/notifications/test', { channel: 'email', target: '安全值班组', alert_type: 'scan' });
  });

  it('binds rule, template, escalation, delivery and silence mutations to encoded ids', async () => {
    patch.mockResolvedValue({ data: { success: true, data: {} } });
    post.mockResolvedValue({ data: { success: true, data: {} } });
    const client = await import('./notificationGovernanceApi');
    await client.patchNotificationRule('rule/1', { enabled: false }, 7, '停用规则');
    await client.patchNotificationTemplate('template/1', { enabled: false }, 4, '停用模板');
    await client.patchNotificationEscalationPolicy('policy/1', { enabled: false }, 3, '停用升级策略');
    await client.retryNotificationDelivery(41);
    await client.patchNotificationSilenceRule('silence/1', { enabled: false }, 9, '停用静默窗口');
    const ruleCall = patch.mock.calls[0];
    expect(ruleCall[0]).toBe('/v1/notifications/subscriptions/rule%2F1');
    expect(ruleCall[1]).toEqual(expect.objectContaining({ enabled: false, expected_revision: 7, reason: '停用规则', action_id: expect.any(String) }));
    expect(ruleCall[2]).toEqual({ headers: { 'Idempotency-Key': ruleCall[1].action_id } });
    const templateCall = patch.mock.calls[1];
    expect(templateCall[0]).toBe('/v1/notifications/templates/template%2F1');
    expect(templateCall[1]).toEqual(expect.objectContaining({ enabled: false, expected_version: 4, reason: '停用模板', action_id: expect.any(String) }));
    expect(templateCall[2]).toEqual({ headers: { 'Idempotency-Key': templateCall[1].action_id } });
    const escalationCall = patch.mock.calls[2];
    expect(escalationCall[0]).toBe('/v1/notifications/escalation-policies/policy%2F1');
    expect(escalationCall[1]).toEqual(expect.objectContaining({ enabled: false, expected_revision: 3, reason: '停用升级策略', action_id: expect.any(String) }));
    expect(escalationCall[2]).toEqual({ headers: { 'Idempotency-Key': escalationCall[1].action_id } });
    expect(post).toHaveBeenCalledWith('/v1/notifications/deliveries/41/retry');
    const silenceCall = patch.mock.calls[3];
    expect(silenceCall[0]).toBe('/v1/notifications/silence-rules/silence%2F1');
    expect(silenceCall[1]).toEqual(expect.objectContaining({ enabled: false, expected_revision: 9, action_reason: '停用静默窗口', action_id: expect.any(String) }));
    expect(silenceCall[2]).toEqual({ headers: { 'Idempotency-Key': silenceCall[1].action_id } });
  });

  it('filters the audit log to notification actions', async () => {
    get.mockResolvedValue({ data: { data: { trails: [{ log_id: 'a1', action: 'NOTIFICATION_RULE_UPDATED' }, { log_id: 'a2', action: 'AUDIT_EXPORTED' }] } } });
    const client = await import('./notificationGovernanceApi');
    expect(await client.fetchNotificationAudits()).toEqual([{ log_id: 'a1', action: 'NOTIFICATION_RULE_UPDATED' }]);
    expect(get).toHaveBeenCalledWith('/v1/audit/logs', { params: { limit: 100 } });
  });
});

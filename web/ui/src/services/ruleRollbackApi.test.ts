import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api, fetchRuleApplicationStatus, rollbackRuleVersion } from './api';

describe('rollbackRuleVersion', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('submits the selected target with optimistic current version and reason', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        data: {
          rule: { rule_id: 'rule/001', version: 6, name: 'restored' },
          event_id: '11111111-1111-4111-8111-111111111111',
          runtime_status: 'pending',
          expected_acks: 4,
          target_version: 2,
          previous_version: 5,
          new_version: 6,
        },
      },
    } as never);

    const result = await rollbackRuleVersion({
      ruleId: 'rule/001',
      targetVersion: 2,
      expectedVersion: 5,
      reason: '  restore verified DNS behavior  ',
    });

    expect(result.rule.version).toBe(6);
    expect(result.runtime_status).toBe('pending');
    expect(api.post).toHaveBeenCalledWith('/v1/rules/rule%2F001/rollback', {
      target_version: 2,
      expected_version: 5,
      reason: 'restore verified DNS behavior',
    });
  });

  it('fails closed on invalid input or a non-monotonic response', async () => {
    const post = vi.spyOn(api, 'post');
    await expect(rollbackRuleVersion({ ruleId: 'rule-1', targetVersion: 0, expectedVersion: 5, reason: 'reason' }))
      .rejects.toThrow('target version');
    expect(post).not.toHaveBeenCalled();

    post.mockResolvedValue({ data: { success: true, data: { rule: { rule_id: 'rule-1', version: 5 }, event_id: 'e1', runtime_status: 'pending', expected_acks: 4, new_version: 5 } } } as never);
    await expect(rollbackRuleVersion({ ruleId: 'rule-1', targetVersion: 2, expectedVersion: 5, reason: 'reason' }))
      .rejects.toThrow('monotonic version');
  });

  it('reads broker and exact runtime acknowledgement progress separately', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: {
      event_id: '11111111-1111-4111-8111-111111111111',
      rule_id: 'rule-1',
      version: 6,
      action: 'update',
      broker_published: true,
      runtime_status: 'partial',
      expected_acks: 4,
      received_acks: 2,
      successful_acks: 2,
      stale_acks: 0,
      conflict_acks: 0,
      consumer_parallelism: 4,
      current_version: 6,
    } } } as never);
    const status = await fetchRuleApplicationStatus('rule-1', '11111111-1111-4111-8111-111111111111');
    expect(status.broker_published).toBe(true);
    expect(status.runtime_status).toBe('partial');
    expect(status.successful_acks).toBe(2);
    expect(api.get).toHaveBeenCalledWith('/v1/rules/rule-1/operations/11111111-1111-4111-8111-111111111111');
  });
});

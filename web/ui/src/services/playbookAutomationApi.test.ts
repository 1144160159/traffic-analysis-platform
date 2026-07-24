import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import { drillPlaybook, savePlaybookDraft, setPlaybookEnabled, transitionPlaybook } from './playbookAutomationApi';

describe('playbookAutomationApi', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('saves an existing draft with optimistic version and a tenant-neutral definition', async () => {
    vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true, data: { name: 'block-scanner', version: 4 } } } as never);
    const definition = {
      name: 'block-scanner', description: 'scanner response', enabled: true,
      trigger: { alert_type: 'scan', severity_min: 'high', score_min: 0.8 },
      actions: [{ type: 'capture_pcap', parameters: {}, timeout: 30_000_000_000 }],
      cooldown: 60_000_000_000, max_runs: 5, run_count: 1,
      approval_policy: { required: true, minimum_role: 'L2', two_person_rule: true },
      rollback_policy: { supported: true, automatic: false },
    };
    await savePlaybookDraft({ name: 'block-scanner', expectedVersion: 3, displayName: '扫描源响应', description: 'updated', definition });
    expect(api.put).toHaveBeenCalledWith('/v1/playbooks/block-scanner/draft', expect.objectContaining({
      expected_version: 3,
      definition: expect.objectContaining({ name: 'block-scanner', description: 'updated', enabled: false }),
    }));
  });

  it('maps lifecycle transitions to explicit endpoints', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: { name: 'block-scanner', version: 3 } } } as never);
    await transitionPlaybook('block/scanner', 'approve', 2, 'independent review');
    expect(api.post).toHaveBeenCalledWith('/v1/playbooks/block%2Fscanner/approve', { expected_version: 2, reason: 'independent review' });
  });

  it('sends durable enable and disable transitions with optimistic version', async () => {
    vi.spyOn(api, 'patch').mockResolvedValue({ data: { success: true, data: { name: 'block-scanner', version: 5, enabled: false } } } as never);
    await setPlaybookEnabled('block/scanner', false, 4);
    expect(api.patch).toHaveBeenCalledWith('/v1/playbooks/block%2Fscanner', { enabled: false, expected_version: 4 });
  });

  it('accepts only drill executions whose actions are all simulated', async () => {
    const post = vi.spyOn(api, 'post');
    post.mockResolvedValueOnce({ data: { success: true, data: { mode: 'drill', result: { actions: [{ action_type: 'block_ip', simulated: true }] } } } } as never);
    await expect(drillPlaybook('block-scanner', 3)).resolves.toMatchObject({ mode: 'drill' });
    post.mockResolvedValueOnce({ data: { success: true, data: { mode: 'drill', result: { actions: [{ action_type: 'block_ip', simulated: false }] } } } } as never);
    await expect(drillPlaybook('block-scanner', 3)).rejects.toThrow('未标记为 simulated');
  });
});

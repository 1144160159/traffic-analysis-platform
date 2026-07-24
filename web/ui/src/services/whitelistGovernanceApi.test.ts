import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import { createWhitelistDraft, transitionWhitelistEntry, type WhitelistEntry } from './whitelistGovernanceApi';

const entry: WhitelistEntry = {
  id: 'wl/001', tenant_id: 'default', type: 'domain', value: 'update.campus.local', reason: '误报', description: '系统更新',
  status: 'draft', approval_status: 'draft', owner_role: '安全运营', scope: '全网', risk_level: 'medium', covered_alerts: 42,
  covered_assets: 8, version: 3, created_by: 'author-1', created_at: '2026-07-19T00:00:00Z', updated_at: '2026-07-19T00:00:00Z',
};

describe('whitelistGovernanceApi', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('creates a draft without trusting client tenant identity', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: entry } } as never);
    await createWhitelistDraft({
      type: 'domain', value: entry.value, reason: entry.reason, description: entry.description, owner_role: '安全运营', scope: '全网',
      risk_level: 'medium', covered_alerts: 42, covered_assets: 8, expires_at: '2026-08-19T00:00:00Z',
    });
    expect(api.post).toHaveBeenCalledWith('/v1/whitelist', expect.objectContaining({ status: 'draft', approval_status: 'draft' }));
    expect(api.post).not.toHaveBeenCalledWith('/v1/whitelist', expect.objectContaining({ tenant_id: expect.anything() }));
  });

  it('submits and approves with optimistic versions', async () => {
    const patch = vi.spyOn(api, 'patch').mockResolvedValue({ data: { success: true, data: { ...entry, version: 4 } } } as never);
    await transitionWhitelistEntry({ entry, action: 'submit' });
    expect(patch).toHaveBeenCalledWith('/v1/whitelist/wl%2F001', { expected_version: 3, status: 'pending', approval_status: 'pending' });
    await transitionWhitelistEntry({ entry: { ...entry, status: 'pending', approval_status: 'pending' }, action: 'approve', reason: '独立复核通过' });
    expect(patch).toHaveBeenLastCalledWith('/v1/whitelist/wl%2F001', expect.objectContaining({ expected_version: 3, status: 'active', approval_status: 'approved', reason: '独立复核通过' }));
  });

  it('preserves audit-safe disable and expiry actions', async () => {
    const patch = vi.spyOn(api, 'patch').mockResolvedValue({ data: { success: true, data: { ...entry, version: 4 } } } as never);
    await transitionWhitelistEntry({ entry, action: 'extend', expiresAt: '2026-09-01T00:00:00Z', reason: '复审延期' });
    expect(patch).toHaveBeenCalledWith('/v1/whitelist/wl%2F001', expect.objectContaining({ expected_version: 3, expires_at: '2026-09-01T00:00:00Z' }));
    await transitionWhitelistEntry({ entry, action: 'disable', reason: '停止抑制' });
    expect(patch).toHaveBeenLastCalledWith('/v1/whitelist/wl%2F001', expect.objectContaining({ expected_version: 3, status: 'disabled', reason: '停止抑制' }));
  });
});

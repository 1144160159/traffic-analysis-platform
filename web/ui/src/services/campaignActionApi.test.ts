import { beforeEach, describe, expect, it, vi } from 'vitest';
import { clearAuthTokens, setAuthTokens } from './authStorage';
import { submitCampaignAction, type CampaignActionId } from './campaignActionApi';

const AUDIT_KEY = 'taf:campaign-action-audit';
const { requestMock } = vi.hoisted(() => ({ requestMock: vi.fn() }));

vi.mock('@/services/api', () => ({ api: { request: requestMock } }));

describe('submitCampaignAction', () => {
  beforeEach(() => {
    sessionStorage.clear();
    clearAuthTokens();
    requestMock.mockReset();
    requestMock.mockImplementation(({ url, data }: { url: string; data: { action_id: string } }) => Promise.resolve({
      data: { data: { action_id: data.action_id, endpoint: url, job_id: 'campaign-job-001', status: 'recorded', job_status: 'completed', simulation: true } },
    }));
  });

  it.each([
    ['campaign-export', '/v1/campaigns/actions', 'CAMPAIGN_EXPORT_REQUESTED'],
    ['campaign-list-settings', '/v1/campaigns/actions', 'CAMPAIGN_LIST_SETTINGS_UPDATED'],
    ['campaign-assign-owner', '/v1/campaigns/APT%2F2026-001/actions', 'CAMPAIGN_OWNER_ASSIGNED'],
    ['campaign-status-change', '/v1/campaigns/APT%2F2026-001/actions', 'CAMPAIGN_STATUS_CHANGED'],
    ['campaign-context-action', '/v1/campaigns/APT%2F2026-001/actions', 'CAMPAIGN_CONTEXT_ACTION_REQUESTED'],
    ['campaign-detail-view', '/v1/campaigns/APT%2F2026-001/actions', 'CAMPAIGN_DETAIL_VIEWED'],
    ['campaign-phase-inspect', '/v1/campaigns/APT%2F2026-001/actions', 'CAMPAIGN_PHASE_VIEWED'],
    ['campaign-impact-inspect', '/v1/campaigns/APT%2F2026-001/actions', 'CAMPAIGN_IMPACT_VIEWED'],
    ['campaign-evidence-view', '/v1/campaigns/APT%2F2026-001/actions', 'CAMPAIGN_EVIDENCE_VIEWED'],
    ['campaign-report-generate', '/v1/campaigns/APT%2F2026-001/actions', 'CAMPAIGN_REPORT_REQUESTED'],
    ['campaign-attack-chain-view', '/v1/campaigns/APT%2F2026-001/actions', 'CAMPAIGN_ATTACK_CHAIN_VIEWED'],
    ['campaign-graph-view', '/v1/campaigns/APT%2F2026-001/actions', 'CAMPAIGN_GRAPH_VIEWED'],
    ['campaign-soar-response', '/v1/campaigns/APT%2F2026-001/actions', 'CAMPAIGN_SOAR_RESPONSE_REQUESTED'],
  ] as const)('maps %s to an audited asynchronous simulation', async (actionId, endpoint, auditEvent) => {
    const result = await submitCampaignAction({
      actionId: actionId as CampaignActionId,
      targetId: 'APT/2026-001',
      target: 'APT campaign',
      phase: 'lateral movement',
      scope: 'business systems',
    });

    expect(result.endpoint).toBe(endpoint);
    expect(result.auditEvent).toBe(auditEvent);
    expect(result.jobId).toBe('campaign-job-001');
    expect(result.mode).toBe('server-persisted-simulation');
    expect(result.jobStatus).toBe('completed');
    expect(result.requestBody).toMatchObject({ target: 'APT campaign', simulation: true, metadata: { dry_run: true } });
    if (endpoint === '/v1/campaigns/actions') {
      expect(result.requestBody.metadata).not.toHaveProperty('campaign_id');
    } else {
      expect(result.requestBody.metadata).toMatchObject({ campaign_id: 'APT/2026-001' });
    }
    expect(result.auditRecord).toMatchObject({ actionId, endpoint, jobId: result.jobId, status: 'recorded' });
    expect(requestMock).toHaveBeenLastCalledWith(expect.objectContaining({
      url: endpoint,
      method: 'POST',
      data: expect.objectContaining({ action_id: actionId, simulation: true, dry_run: true }),
    }));
    expect(JSON.parse(sessionStorage.getItem(AUDIT_KEY) ?? '[]')).toContainEqual(result.auditRecord);
  });

  it.each([
    ['alert:*', 'campaign-detail-view'],
    ['admin:*', 'campaign-graph-view'],
    ['*', 'campaign-soar-response'],
  ] as const)('accepts wildcard permission %s for %s', async (permission, actionId) => {
    setAuthTokens(jwtWithPermissions([permission]));

    await expect(submitCampaignAction({ actionId, targetId: 'APT-001', target: 'APT campaign' })).resolves.toMatchObject({ actionId });
  });

  it('supports the page campaignId and metadata shape', async () => {
    const result = await submitCampaignAction({
      actionId: 'campaign-phase-inspect',
      campaignId: 'APT-001',
      target: '横向移动',
      metadata: { phase: '横向移动', source: 'phase-node' },
    });

    expect(result.endpoint).toBe('/v1/campaigns/APT-001/actions');
    expect(result.requestBody).toMatchObject({ metadata: { campaign_id: 'APT-001', phase: '横向移动', source: 'phase-node' } });
  });

  it('removes caller-supplied campaign_id from collection actions', async () => {
    const result = await submitCampaignAction({
      actionId: 'campaign-export',
      targetId: 'APT-001',
      target: 'current page',
      metadata: { campaign_id: 'other-campaign' },
      requestBody: { campaign_id: 'request-body-campaign' },
    });

    expect(result.requestBody.metadata).not.toHaveProperty('campaign_id');
  });

  it('rejects a response without a persisted server job id', async () => {
    requestMock.mockResolvedValueOnce({
      data: { data: { status: 'recorded', job_status: 'completed', simulation: true } },
    });

    await expect(submitCampaignAction({ actionId: 'campaign-detail-view', targetId: 'APT-001', target: 'APT campaign' }))
      .rejects.toThrow('未返回持久化作业编号');
    expect(sessionStorage.getItem(AUDIT_KEY)).toBeNull();
  });

  it('enforces graph and SOAR scopes independently from alert read access', async () => {
    setAuthTokens(jwtWithPermissions(['alert:read']));

    await expect(submitCampaignAction({ actionId: 'campaign-graph-view', targetId: 'APT-001', target: 'APT campaign' }))
      .rejects.toThrow('缺少 Campaign 动作权限：graph:read');
    await expect(submitCampaignAction({ actionId: 'campaign-soar-response', targetId: 'APT-001', target: 'APT campaign' }))
      .rejects.toThrow('缺少 Campaign 动作权限：playbook:execute');
    expect(sessionStorage.getItem(AUDIT_KEY)).toBeNull();
  });

  it('rejects malformed JWTs instead of bypassing RBAC', async () => {
    setAuthTokens('not-a-jwt');

    await expect(submitCampaignAction({ actionId: 'campaign-detail-view', targetId: 'APT-001', target: 'APT campaign' }))
      .rejects.toThrow('缺少 Campaign 动作权限：alert:read');
  });

  it('does not treat a concrete admin permission as the admin wildcard grant', async () => {
    setAuthTokens(jwtWithPermissions(['admin:read']));

    await expect(submitCampaignAction({ actionId: 'campaign-status-change', targetId: 'APT-001', target: 'APT campaign' }))
      .rejects.toThrow('缺少 Campaign 动作权限：alert:write');
  });

  it('recovers a malformed audit value and retains the newest 20 records', async () => {
    sessionStorage.setItem(AUDIT_KEY, '{broken-json');
    await submitCampaignAction({ actionId: 'campaign-detail-view', targetId: 'APT-000', target: 'APT campaign' });

    expect(JSON.parse(sessionStorage.getItem(AUDIT_KEY) ?? '[]')).toHaveLength(1);

    for (let index = 1; index <= 21; index += 1) {
      await submitCampaignAction({ actionId: 'campaign-detail-view', targetId: `APT-${index}`, target: 'APT campaign' });
    }
    const records = JSON.parse(sessionStorage.getItem(AUDIT_KEY) ?? '[]') as Array<{ targetId: string }>;
    expect(records).toHaveLength(20);
    expect(records[0].targetId).toBe('APT-2');
    expect(records[records.length - 1]?.targetId).toBe('APT-21');
  });
});

const jwtWithPermissions = (permissions: string[]) => {
  const payload = globalThis.btoa(JSON.stringify({ permissions })).replace(/=/g, '').replace(/\+/g, '-').replace(/\//g, '_');
  return `e30.${payload}.signature`;
};

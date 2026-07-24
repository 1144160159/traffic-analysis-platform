import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import { submitCampaignAction } from './campaignActionApi';

vi.mock('./authStorage', () => ({ getAuthToken: () => null }));
vi.mock('./api', () => ({ api: { request: vi.fn() } }));

const requestMock = vi.mocked(api.request);

describe('submitCampaignAction', () => {
  beforeEach(() => {
    requestMock.mockReset();
  });

  it('submits status changes as real persisted mutations', async () => {
    requestMock.mockResolvedValue({
      data: {
        data: {
          action_id: 'campaign-status-change', audit_event: 'CAMPAIGN_STATUS_CHANGED',
          endpoint: '/v1/campaigns/campaign-a/actions', job_id: 'job-a', status: 'completed',
          job_status: 'completed', simulation: false, dry_run: false,
          result: { campaign_status: 'investigating', state_version: 2 },
        },
      },
    } as never);

    const result = await submitCampaignAction({
      actionId: 'campaign-status-change', campaignId: 'campaign-a', target: '变更状态',
      metadata: { next_status: 'investigating' },
    });

    expect(result.mode).toBe('server-persisted-mutation');
    expect(result.result).toMatchObject({ campaign_status: 'investigating', state_version: 2 });
    expect(requestMock).toHaveBeenCalledWith(expect.objectContaining({
      url: '/v1/campaigns/campaign-a/actions',
      data: expect.objectContaining({
        simulation: false,
        dry_run: false,
        metadata: expect.objectContaining({ campaign_id: 'campaign-a', next_status: 'investigating', dry_run: false }),
      }),
    }));
  });

  it('keeps view actions read-only while persisting their audit record', async () => {
    requestMock.mockResolvedValue({
      data: {
        data: {
          action_id: 'campaign-detail-view', audit_event: 'CAMPAIGN_DETAIL_VIEWED',
          endpoint: '/v1/campaigns/campaign-a/actions', job_id: 'job-b', status: 'completed',
          job_status: 'completed', simulation: true, dry_run: true, result: {},
        },
      },
    } as never);

    const result = await submitCampaignAction({
      actionId: 'campaign-detail-view', campaignId: 'campaign-a', target: '查看详情',
    });

    expect(result.mode).toBe('server-persisted-read');
    expect(requestMock).toHaveBeenCalledWith(expect.objectContaining({
      data: expect.objectContaining({ simulation: true, dry_run: true }),
    }));
  });
});

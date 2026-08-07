import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import {
  CampaignReportTerminalError,
  applyCampaignSOAROperation,
  classifyCampaignActionStatus,
  downloadCampaignReport,
  getCampaignReport,
  getCampaignSOARJob,
  submitCampaignAction,
  waitForCampaignReport,
} from './campaignActionApi';

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
      metadata: { next_status: 'investigating', snapshot_id: 'campaign:campaign-a:revision:1:0123456789abcdef' },
    });

    expect(result.mode).toBe('server-persisted-mutation');
    expect(result.result).toMatchObject({ campaign_status: 'investigating', state_version: 2 });
    expect(requestMock).toHaveBeenCalledWith(expect.objectContaining({
      url: '/v1/campaigns/campaign-a/actions',
      data: expect.objectContaining({
        simulation: false,
        dry_run: false,
        expected_revision: 0,
        reason: '执行战役动作：变更状态',
        metadata: expect.objectContaining({
          campaign_id: 'campaign-a', next_status: 'investigating', dry_run: false,
          snapshot_id: 'campaign:campaign-a:revision:1:0123456789abcdef',
        }),
      }),
      headers: { 'Idempotency-Key': expect.stringMatching(/^campaign:campaign-status-change:0:/) },
    }));
  });

  it('keeps accepted report commands distinct from final success', async () => {
    requestMock.mockResolvedValue({
      data: {
        data: {
          action_id: 'campaign-report-generate', audit_event: 'CAMPAIGN_REPORT_REQUESTED',
          job_id: 'job-report', status: 'accepted', job_status: 'accepted',
          simulation: false, dry_run: false,
          result: { report_status: 'accepted', object_manifest_status: 'awaiting_executor' },
        },
      },
    } as never);

    const result = await submitCampaignAction({
      actionId: 'campaign-report-generate', campaignId: 'campaign-a', target: '生成报告',
      expectedRevision: 7, reason: '冻结当前成员并生成战役复盘报告',
      metadata: { format: 'pdf', sections: ['证据链'] },
    });

    expect(result.status).toBe('accepted');
    expect(result.jobStatus).toBe('accepted');
    expect(result.result).toMatchObject({ report_status: 'accepted', object_manifest_status: 'awaiting_executor' });
    expect(requestMock).toHaveBeenCalledWith(expect.objectContaining({
      data: expect.objectContaining({ expected_revision: 7, reason: '冻结当前成员并生成战役复盘报告' }),
      headers: { 'Idempotency-Key': expect.stringMatching(/^campaign:campaign-report-generate:7:/) },
    }));
  });

  it('keeps a SOAR request in pending approval instead of reporting final success', async () => {
    requestMock.mockResolvedValue({
      data: { data: {
        action_id: 'campaign-soar-response', audit_event: 'CAMPAIGN_SOAR_RESPONSE_REQUESTED',
        job_id: 'job-soar', status: 'pending_approval', job_status: 'pending_approval',
        simulation: false, dry_run: false,
        result: { approval_status: 'pending', executor_status: 'not_dispatched', final_effect: false },
      } },
    } as never);

    const result = await submitCampaignAction({
      actionId: 'campaign-soar-response', campaignId: 'campaign-a', target: 'asset-a',
      expectedRevision: 8, reason: '请求隔离受影响的关键业务终端',
      metadata: { playbook_id: 'contain-host', snapshot_id: 'snapshot-a' },
    });

    expect(result.jobStatus).toBe('pending_approval');
    expect(result.result).toMatchObject({ approval_status: 'pending', final_effect: false });
    expect(requestMock).toHaveBeenCalledWith(expect.objectContaining({
      data: expect.objectContaining({ simulation: false, dry_run: false, metadata: expect.objectContaining({ dry_run: false }) }),
    }));
  });

  it('preserves partial and failed terminal outcomes instead of converting them to success or transport errors', async () => {
    requestMock
      .mockResolvedValueOnce({ data: { data: {
        action_id: 'campaign-status-change', audit_event: 'CAMPAIGN_STATUS_CHANGED',
        job_id: 'job-partial', status: 'partial', job_status: 'partial',
        simulation: false, dry_run: false, result: { succeeded: 2, failed: 1 },
      } } } as never)
      .mockResolvedValueOnce({ data: { data: {
        action_id: 'campaign-status-change', audit_event: 'CAMPAIGN_STATUS_CHANGED',
        job_id: 'job-failed', status: 'failed', job_status: 'failed',
        simulation: false, dry_run: false, result: { error: 'executor unavailable' },
      } } } as never);

    const partial = await submitCampaignAction({
      actionId: 'campaign-status-change', campaignId: 'campaign-a', target: '变更状态',
    });
    const failed = await submitCampaignAction({
      actionId: 'campaign-status-change', campaignId: 'campaign-a', target: '变更状态',
    });

    expect(partial.jobStatus).toBe('partial');
    expect(failed.jobStatus).toBe('failed');
    expect(classifyCampaignActionStatus(partial.jobStatus)).toBe('partial');
    expect(classifyCampaignActionStatus(failed.jobStatus)).toBe('failed');
    expect(classifyCampaignActionStatus('cancelled')).toBe('cancelled');
    expect(classifyCampaignActionStatus('compensated')).toBe('compensated');
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

const campaignSOAREnvelope = (status: string, overrides: Record<string, unknown> = {}) => ({
  data: {
    job_id: 'job-soar-a', tenant_id: 'tenant-a', campaign_id: 'campaign-a',
    playbook_id: 'contain-host', target: 'asset-a', source_snapshot_id: 'snapshot-a',
    campaign_revision: 8, status, approval_status: status === 'pending_approval' ? 'pending' : 'approved',
    executor_status: status === 'pending_approval' ? 'not_dispatched' : 'queued', revision: 1,
    request: { playbook_id: 'contain-host' }, execution_receipt: {}, compensation_receipt: {},
    error_message: '', attempts: 0, requested_by: 'requester-a', approved_by: '',
    created_at: '2026-08-02T01:00:00Z', updated_at: '2026-08-02T01:00:00Z',
    ...overrides,
  },
  meta: { contract_version: 4, trace_id: 'trace-soar-a', source_watermarks: { 'postgresql.campaign_soar_jobs.revision': '1' } },
});

describe('campaign SOAR lifecycle', () => {
  beforeEach(() => {
    requestMock.mockReset();
  });

  it('reads the durable approval and executor states separately', async () => {
    requestMock.mockResolvedValue({ data: campaignSOAREnvelope('pending_approval') } as never);
    const job = await getCampaignSOARJob('campaign-a', 'job-soar-a');
    expect(job).toMatchObject({ status: 'pending_approval', approvalStatus: 'pending', executorStatus: 'not_dispatched', revision: 1 });
    expect(requestMock).toHaveBeenCalledWith(expect.objectContaining({
      method: 'GET', url: '/v1/campaigns/campaign-a/soar-jobs/job-soar-a',
    }));
  });

  it('submits an independently auditable approval with optimistic revision', async () => {
    requestMock.mockResolvedValue({ data: campaignSOAREnvelope('approved_awaiting_executor', {
      approval_status: 'approved', executor_status: 'queued', revision: 2, approved_by: 'approver-a',
    }) } as never);
    const job = await applyCampaignSOAROperation('campaign-a', 'job-soar-a', 'approve', 1, '独立审批人确认执行隔离剧本');
    expect(job).toMatchObject({ status: 'approved_awaiting_executor', approvalStatus: 'approved', revision: 2 });
    expect(requestMock).toHaveBeenCalledWith(expect.objectContaining({
      method: 'POST', url: '/v1/campaigns/campaign-a/soar-jobs/job-soar-a/approval',
      data: { decision: 'approve', expected_revision: 1, reason: '独立审批人确认执行隔离剧本' },
      headers: { 'Idempotency-Key': expect.stringMatching(/^campaign-soar:approve:1:/) },
    }));
  });

  it('rejects a terminal effect that lacks a provider receipt', async () => {
    requestMock.mockResolvedValue({ data: campaignSOAREnvelope('completed', {
      approval_status: 'approved', executor_status: 'succeeded', revision: 3,
    }) } as never);
    await expect(getCampaignSOARJob('campaign-a', 'job-soar-a')).rejects.toThrow('缺少 provider 回执');
  });
});

const campaignReportEnvelope = (status: 'accepted' | 'running' | 'completed' | 'failed', overrides: Record<string, unknown> = {}) => ({
  data: {
    report_id: 'campaign-report-a',
    job_id: 'job-report-a',
    tenant_id: 'tenant-a',
    campaign_id: 'campaign-a',
    format: 'pdf',
    status,
    campaign_revision: 8,
    snapshot_id: 'snapshot-a',
    snapshot_sha256: 'sha256:snapshot-a',
    object_manifest: status === 'completed' ? { status: 'completed' } : {},
    mime_type: status === 'completed' ? 'application/pdf' : '',
    artifact_sha256: status === 'completed' ? 'sha256:artifact-a' : '',
    size_bytes: status === 'completed' ? 4 : 0,
    attempts: status === 'accepted' ? 0 : 1,
    error_message: status === 'failed' ? 'executor exhausted retries' : '',
    created_at: '2026-08-01T01:00:00Z',
    updated_at: '2026-08-01T01:00:01Z',
    ...overrides,
  },
  meta: {
    contract_version: 2,
    trace_id: 'trace-report-a',
    source_watermarks: { 'postgresql.campaign_reports.attempts': status === 'accepted' ? '0' : '1' },
  },
});

describe('campaign report lifecycle', () => {
  beforeEach(() => {
    requestMock.mockReset();
  });

  it('reads an authoritative tenant-scoped report status', async () => {
    requestMock.mockResolvedValue({ data: campaignReportEnvelope('running') } as never);

    const report = await getCampaignReport('campaign-a', 'campaign-report-a');

    expect(report).toMatchObject({
      reportId: 'campaign-report-a', campaignId: 'campaign-a', status: 'running', attempts: 1,
      snapshotId: 'snapshot-a', snapshotSHA256: 'sha256:snapshot-a', traceId: 'trace-report-a',
    });
    expect(requestMock).toHaveBeenCalledWith(expect.objectContaining({
      method: 'GET',
      url: '/v1/campaigns/campaign-a/reports/campaign-report-a',
    }));
  });

  it('polls accepted and running states until explicit completion', async () => {
    requestMock
      .mockResolvedValueOnce({ data: campaignReportEnvelope('accepted') } as never)
      .mockResolvedValueOnce({ data: campaignReportEnvelope('running') } as never)
      .mockResolvedValueOnce({ data: campaignReportEnvelope('completed') } as never);
    const observed: string[] = [];

    const report = await waitForCampaignReport('campaign-a', 'campaign-report-a', {
      intervalMs: 0,
      timeoutMs: 1_000,
      onStatus: (current) => observed.push(current.status),
    });

    expect(observed).toEqual(['accepted', 'running', 'completed']);
    expect(report.status).toBe('completed');
    expect(requestMock).toHaveBeenCalledTimes(3);
  });

  it('surfaces a failed terminal state instead of reporting success', async () => {
    requestMock.mockResolvedValue({ data: campaignReportEnvelope('failed') } as never);

    await expect(waitForCampaignReport('campaign-a', 'campaign-report-a', { intervalMs: 0, timeoutMs: 100 }))
      .rejects.toMatchObject({
        name: 'CampaignReportTerminalError',
        message: 'executor exhausted retries',
      } satisfies Partial<CampaignReportTerminalError>);
  });

  it('downloads only a completed artifact and checks response manifest headers', async () => {
    const statusResponse = campaignReportEnvelope('completed').data;
    const report = await (async () => {
      requestMock.mockResolvedValueOnce({ data: campaignReportEnvelope('completed') } as never);
      return getCampaignReport('campaign-a', 'campaign-report-a');
    })();
    const blob = new Blob(['%PDF'], { type: 'application/pdf' });
    expect(blob.size).toBe(statusResponse.size_bytes);
    requestMock.mockResolvedValueOnce({
      data: blob,
      headers: {
        'content-disposition': 'attachment; filename="campaign-report-a.pdf"',
        'x-content-sha256': 'sha256:artifact-a',
      },
    } as never);

    const artifact = await downloadCampaignReport('campaign-a', report);

    expect(artifact).toMatchObject({ filename: 'campaign-report-a.pdf', sha256: 'sha256:artifact-a' });
    expect(artifact.blob).toBe(blob);
    expect(requestMock).toHaveBeenLastCalledWith(expect.objectContaining({
      method: 'GET', responseType: 'blob',
      url: '/v1/campaigns/campaign-a/reports/campaign-report-a/download',
    }));
  });

  it('rejects a status response for another campaign', async () => {
    requestMock.mockResolvedValue({
      data: campaignReportEnvelope('running', { campaign_id: 'campaign-b' }),
    } as never);

    await expect(getCampaignReport('campaign-a', 'campaign-report-a'))
      .rejects.toThrow('战役报告状态响应与请求资源不一致');
  });
});

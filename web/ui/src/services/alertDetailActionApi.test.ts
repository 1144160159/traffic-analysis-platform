import { beforeEach, describe, expect, it, vi } from 'vitest';
import { submitAlertTriageAction } from './alertTriageApi';
import {
  alertDetailActionErrorMessage,
  cancelAlertReport,
  cancelAlertReportWithRevisionRefresh,
  compensateAlertReport,
  downloadAlertEvidenceFile,
  fetchAlertCampaignLinks,
  submitAlertDetailAction,
  submitAlertReportWithSnapshotRetry,
} from './alertDetailActionApi';
import { api } from './api';

vi.mock('./alertTriageApi', () => ({ submitAlertTriageAction: vi.fn() }));
vi.mock('./api', () => ({ api: { delete: vi.fn(), get: vi.fn(), post: vi.fn(), put: vi.fn() } }));

const submitAlertTriageActionMock = vi.mocked(submitAlertTriageAction);
const apiPutMock = vi.mocked(api.put);
const apiPostMock = vi.mocked(api.post);
const apiGetMock = vi.mocked(api.get);
const apiDeleteMock = vi.mocked(api.delete);

describe('submitAlertDetailAction', () => {
  beforeEach(() => {
    submitAlertTriageActionMock.mockReset();
    apiPutMock.mockReset();
    apiPostMock.mockReset();
    apiGetMock.mockReset();
    apiDeleteMock.mockReset();
    submitAlertTriageActionMock.mockResolvedValue({ job_id: 'alert-action-live-1', status: 'recorded', action: '导出告警报告', target: 'AL-20260620-000123', dry_run: false, audit_event: 'ALERT_INVESTIGATION_NOTE_RECORDED' });
  });

  it('returns a typed live record for a registered report export contract', async () => {
    apiPostMock.mockResolvedValue({
      data: {
        data: {
          job_id: 'alert-report-job-1',
          alert_id: 'AL-20260620-000123',
          format: 'pdf',
          status: 'accepted',
          snapshot_sha256: 'sha256:frozen',
        },
      },
    });
    const result = await submitAlertDetailAction({
      alertId: 'AL-20260620-000123',
      actionId: 'alert-report-export',
      target: 'AL-20260620-000123',
      reason: '确认导出当前告警报告',
      detail: { format: 'pdf', snapshotId: 'alert:AL-20260620-000123:revision:7' },
    });

    expect(result.status).toBe('accepted');
    expect(result.mode).toBe('live');
    expect(result.auditEvent).toBe('ALERT_REPORT_EXPORT_REQUESTED');
    expect(result.apiContract).toBe('/v1/alerts/AL-20260620-000123/reports/export');
    expect(result.jobId).toBe('alert-report-job-1');
    expect(apiPostMock).toHaveBeenCalledWith(
      '/v1/alerts/AL-20260620-000123/reports/export',
      {
        action_id: 'alert-report-export',
        format: 'pdf',
        snapshot_id: 'alert:AL-20260620-000123:revision:7',
        reason: '确认导出当前告警报告',
      },
      {
        headers: {
          'Idempotency-Key': 'alert-report-export:AL-20260620-000123:alert:AL-20260620-000123:revision:7:pdf',
        },
      },
    );
    expect(submitAlertTriageActionMock).not.toHaveBeenCalled();
  });

  it('refreshes the authoritative snapshot and retries only a bounded snapshot conflict', async () => {
    apiPostMock
      .mockRejectedValueOnce({ response: { data: { error: { code: 'SNAPSHOT_CONFLICT', message: 'revision changed' } } } })
      .mockResolvedValueOnce({
        data: { data: { job_id: 'alert-report-job-retried', alert_id: 'AL-20260620-000123', status: 'accepted', revision: 1 } },
      });
    const refreshSnapshotId = vi.fn().mockResolvedValue('alert:AL-20260620-000123:revision:8');
    const input = {
      alertId: 'AL-20260620-000123',
      actionId: 'alert-report-export' as const,
      target: 'AL-20260620-000123',
      reason: '确认导出当前告警报告',
      detail: { format: 'pdf', snapshotId: 'alert:AL-20260620-000123:revision:7' },
    };

    const result = await submitAlertReportWithSnapshotRetry(input, refreshSnapshotId);

    expect(result.jobId).toBe('alert-report-job-retried');
    expect(refreshSnapshotId).toHaveBeenCalledTimes(1);
    expect(apiPostMock).toHaveBeenCalledTimes(2);
    expect(apiPostMock.mock.calls[1]?.[1]).toMatchObject({
      snapshot_id: 'alert:AL-20260620-000123:revision:8',
    });
    expect(apiPostMock.mock.calls[1]?.[2]).toEqual({
      headers: {
        'Idempotency-Key': 'alert-report-export:AL-20260620-000123:alert:AL-20260620-000123:revision:8:pdf',
      },
    });
  });

  it('does not retry unrelated report failures', async () => {
    const failure = { response: { data: { error: { code: 'PERSISTENCE_FAILED', message: 'database unavailable' } } } };
    apiPostMock.mockRejectedValueOnce(failure);
    const refreshSnapshotId = vi.fn();

    await expect(submitAlertReportWithSnapshotRetry({
      alertId: 'AL-20260620-000123',
      actionId: 'alert-report-export',
      target: 'AL-20260620-000123',
      detail: { snapshotId: 'alert:AL-20260620-000123:revision:7' },
    }, refreshSnapshotId)).rejects.toBe(failure);
    expect(refreshSnapshotId).not.toHaveBeenCalled();
    expect(apiPostMock).toHaveBeenCalledTimes(1);
  });

  it('refreshes an unavailable snapshot before sending the first report request', async () => {
    apiPostMock.mockResolvedValueOnce({
      data: { data: { job_id: 'alert-report-job-preflight', alert_id: 'AL-20260620-000123', status: 'accepted', revision: 1 } },
    });
    const refreshSnapshotId = vi.fn().mockResolvedValue('alert:AL-20260620-000123:revision:9');

    const result = await submitAlertReportWithSnapshotRetry({
      alertId: 'AL-20260620-000123',
      actionId: 'alert-report-export',
      target: 'AL-20260620-000123',
      detail: { format: 'pdf', snapshotId: 'alert:AL-20260620-000123:revision:undefined' },
    }, refreshSnapshotId);

    expect(result.jobId).toBe('alert-report-job-preflight');
    expect(refreshSnapshotId).toHaveBeenCalledTimes(1);
    expect(apiPostMock).toHaveBeenCalledTimes(1);
    expect(apiPostMock.mock.calls[0]?.[1]).toMatchObject({
      snapshot_id: 'alert:AL-20260620-000123:revision:9',
    });
  });

  it('cancels an alert report with revision and a stable idempotency key', async () => {
    apiPostMock.mockResolvedValue({
      data: {
        data: {
          job_id: 'alert-report-job-1',
          alert_id: 'AL-20260620-000123',
          status: 'cancel_requested',
          revision: 3,
        },
      },
    });

    const result = await cancelAlertReport(
      'AL-20260620-000123',
      'alert-report-job-1',
      2,
      '用户确认取消当前报告导出任务',
    );

    expect(apiPostMock).toHaveBeenCalledWith(
      '/v1/alerts/AL-20260620-000123/reports/alert-report-job-1/cancel',
      {
        action_id: 'alert-report-cancel',
        expected_revision: 2,
        reason: '用户确认取消当前报告导出任务',
      },
      {
        headers: {
          'Idempotency-Key': 'alert-report-cancel:alert-report-job-1:2',
        },
      },
    );
    expect(result.status).toBe('cancel_requested');
    expect(result.revision).toBe(3);
    expect(submitAlertTriageActionMock).not.toHaveBeenCalled();
  });

  it('refreshes a raced cancellation and returns the authoritative terminal state without retrying it', async () => {
    apiPostMock.mockRejectedValueOnce({ response: { data: { error: { code: 'REVISION_CONFLICT' } } } });
    apiGetMock.mockResolvedValueOnce({
      data: { data: { job_id: 'alert-report-job-1', alert_id: 'AL-20260620-000123', status: 'completed', revision: 3, download_url: '/download' } },
    });

    const result = await cancelAlertReportWithRevisionRefresh(
      'AL-20260620-000123',
      'alert-report-job-1',
      1,
      '用户确认取消当前报告任务',
    );

    expect(result.status).toBe('completed');
    expect(result.revision).toBe(3);
    expect(apiPostMock).toHaveBeenCalledTimes(1);
    expect(apiGetMock).toHaveBeenCalledTimes(1);
  });

  it('retries cancellation once when the refreshed report remains cancellable', async () => {
    apiPostMock
      .mockRejectedValueOnce({ response: { data: { error: { code: 'REVISION_CONFLICT' } } } })
      .mockResolvedValueOnce({
        data: { data: { job_id: 'alert-report-job-1', alert_id: 'AL-20260620-000123', status: 'cancel_requested', revision: 3 } },
      });
    apiGetMock.mockResolvedValueOnce({
      data: { data: { job_id: 'alert-report-job-1', alert_id: 'AL-20260620-000123', status: 'running', revision: 2 } },
    });

    const result = await cancelAlertReportWithRevisionRefresh(
      'AL-20260620-000123',
      'alert-report-job-1',
      1,
      '用户确认取消当前报告任务',
    );

    expect(result.status).toBe('cancel_requested');
    expect(apiPostMock).toHaveBeenCalledTimes(2);
    expect(apiPostMock.mock.calls[1]?.[1]).toMatchObject({ expected_revision: 2 });
  });

  it('requests residual object compensation without substituting another action', async () => {
    apiPostMock.mockResolvedValue({
      data: { data: { job_id: 'alert-report-job-1', alert_id: 'AL-20260620-000123', status: 'compensating', revision: 6 } },
    });

    const result = await compensateAlertReport(
      'AL-20260620-000123',
      'alert-report-job-1',
      5,
      '确认重试报告残留对象清理',
    );

    expect(apiPostMock).toHaveBeenCalledWith(
      '/v1/alerts/AL-20260620-000123/reports/alert-report-job-1/compensations',
      {
        action_id: 'alert-report-compensate',
        expected_revision: 5,
        reason: '确认重试报告残留对象清理',
      },
      { headers: { 'Idempotency-Key': 'alert-report-compensate:alert-report-job-1:5' } },
    );
    expect(result.status).toBe('compensating');
    expect(result.revision).toBe(6);
    expect(submitAlertTriageActionMock).not.toHaveBeenCalled();
  });

  it('updates alert labels through the dedicated real API contract', async () => {
    apiPutMock.mockResolvedValue({
      data: {
        data: {
          alert_id: 'AL-20260620-000123',
          labels: ['C2通信', '可疑外联'],
          status: 'updated',
        },
      },
    });

    const result = await submitAlertDetailAction({
      alertId: 'AL-20260620-000123',
      actionId: 'alert-label-update',
      target: 'C2通信，可疑外联',
      reason: '研判后修正标签',
      detail: { labels: ['C2通信', '可疑外联'] },
    });

    expect(apiPutMock).toHaveBeenCalledWith('/v1/alerts/AL-20260620-000123/labels', {
      labels: ['C2通信', '可疑外联'],
      reason: '研判后修正标签',
    });
    expect(result.apiContract).toBe('/v1/alerts/AL-20260620-000123/labels');
    expect(result.target).toBe('C2通信，可疑外联');
  });

  it('links an alert to a campaign through the relation contract with a stable idempotency key', async () => {
    apiPostMock.mockResolvedValue({
      data: {
        data: {
          relation_id: '7c98cba7-7ccd-42e2-b2a1-77d4db5ff001',
          alert_id: 'AL-20260620-000123',
          campaign_id: 'CAM-20260730-001',
          status: 'linked',
          revision: 1,
          idempotent_reuse: false,
        },
      },
    });

    const result = await submitAlertDetailAction({
      alertId: 'AL-20260620-000123',
      actionId: 'alert-campaign-link',
      target: 'CAM-20260730-001',
      reason: '确认归入同一攻击战役',
      detail: { expectedRevision: 0 },
    });

    expect(apiPostMock).toHaveBeenCalledWith(
      '/v1/alerts/AL-20260620-000123/campaign-links',
      {
        campaign_id: 'CAM-20260730-001',
        expected_revision: 0,
        reason: '确认归入同一攻击战役',
      },
      {
        headers: {
          'Idempotency-Key': 'alert-campaign-link:AL-20260620-000123:CAM-20260730-001:0:compat',
        },
      },
    );
    expect(submitAlertTriageActionMock).not.toHaveBeenCalled();
    expect(result.apiContract).toBe('/v1/alerts/AL-20260620-000123/campaign-links');
    expect(result.auditEvent).toBe('ALERT_CAMPAIGN_LINKED');
    expect(result.jobId).toBe('7c98cba7-7ccd-42e2-b2a1-77d4db5ff001');
    expect(result.status).toBe('linked');
  });

  it('unlinks a campaign using both relation and current campaign revisions', async () => {
    apiDeleteMock.mockResolvedValue({
      data: {
        data: {
          relation_id: '7c98cba7-7ccd-42e2-b2a1-77d4db5ff001',
          alert_id: 'AL-20260620-000123',
          campaign_id: 'CAM-20260730-001',
          status: 'unlinked',
          revision: 2,
          campaign_revision: 8,
        },
      },
    });

    const result = await submitAlertDetailAction({
      alertId: 'AL-20260620-000123',
      actionId: 'alert-campaign-unlink',
      target: 'CAM-20260730-001',
      reason: '确认解除当前战役关系',
      detail: { expectedRevision: 1, expectedCampaignRevision: 7 },
    });

    expect(apiDeleteMock).toHaveBeenCalledWith(
      '/v1/alerts/AL-20260620-000123/campaign-links/CAM-20260730-001',
      {
        data: {
          expected_revision: 1,
          expected_campaign_revision: 7,
          reason: '确认解除当前战役关系',
        },
        headers: {
          'Idempotency-Key': 'alert-campaign-unlink:AL-20260620-000123:CAM-20260730-001:1:7',
        },
      },
    );
    expect(submitAlertTriageActionMock).not.toHaveBeenCalled();
    expect(result.status).toBe('unlinked');
    expect(result.auditEvent).toBe('ALERT_CAMPAIGN_UNLINKED');
  });

  it('reads alert-side campaign links with both relation and aggregate revisions', async () => {
    apiGetMock.mockResolvedValue({
      data: {
        data: {
          links: [{
            relation_id: 'relation-1',
            alert_id: 'AL-20260620-000123',
            campaign_id: 'CAM-20260730-001',
            status: 'linked',
            revision: 3,
            campaign_revision: 6,
            current_campaign_revision: 9,
          }],
          total: 1,
          unlink_available: true,
        },
        meta: { partial: false, missing_sections: [] },
      },
    });

    const snapshot = await fetchAlertCampaignLinks('AL-20260620-000123');
    expect(apiGetMock).toHaveBeenCalledWith('/v1/alerts/AL-20260620-000123/campaign-links');
    expect(snapshot.links[0]).toEqual(expect.objectContaining({
      campaignId: 'CAM-20260730-001',
      revision: 3,
      campaignRevision: 6,
      currentCampaignRevision: 9,
    }));
    expect(snapshot.partial).toBe(false);
    expect(snapshot.unlinkAvailable).toBe(true);
  });

  it('persists a download request as a distinct audited evidence access action', async () => {
    apiPostMock.mockResolvedValue({
      data: {
        data: {
          job_id: 'evidence-access-1',
          status: 'recorded',
          target: 'AL-20260620-000123.pcap',
          download_url: '/v1/alerts/AL-20260620-000123/evidence/ev-1/download?expires=123&signature=sig',
          file_name: 'AL-20260620-000123.pcap',
          expires_at: '2026-07-27T13:30:00Z',
          audit_event: 'ALERT_EVIDENCE_ACCESS_REQUESTED',
        },
      },
    });
    const result = await submitAlertDetailAction({
      alertId: 'AL-20260620-000123',
      actionId: 'alert-evidence-access',
      target: 'AL-20260620-000123.pcap',
      reason: '下载告警证据：AL-20260620-000123.pcap',
      detail: {
        access_mode: 'download',
        evidence_kind: 'PCAP',
        signed_url_requested: true,
      },
    });

    expect(apiPostMock).toHaveBeenCalledWith('/v1/alerts/AL-20260620-000123/evidence/access', expect.objectContaining({
      target: 'AL-20260620-000123.pcap',
      detail: expect.objectContaining({
        action_id: 'alert-evidence-access',
        evidence_id: 'AL-20260620-000123.pcap',
        access_mode: 'download',
        signed_url_requested: true,
      }),
    }));
    expect(submitAlertTriageActionMock).not.toHaveBeenCalled();
    expect(result.apiContract).toBe('/v1/alerts/AL-20260620-000123/evidence/access');
    expect(result.downloadUrl).toContain('/download?');
    expect(result.fileName).toBe('AL-20260620-000123.pcap');
  });

  it('downloads the signed evidence URL as a browser file', async () => {
    apiGetMock.mockResolvedValue({ data: new Blob(['evidence']) });
    const click = vi.fn();
    const remove = vi.fn();
    const anchor = { href: '', download: '', style: { display: '' }, click, remove } as unknown as HTMLAnchorElement;
    const createElement = vi.spyOn(document, 'createElement').mockReturnValue(anchor);
    const appendChild = vi.spyOn(document.body, 'appendChild').mockImplementation((node) => node);
    const createObjectURL = vi.fn(() => 'blob:evidence');
    const revokeObjectURL = vi.fn();
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL });
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL });

    await downloadAlertEvidenceFile(
      '/v1/alerts/AL-1/evidence/EV-1/download?expires=123&signature=sig',
      'EV-1.json',
    );

    expect(apiGetMock).toHaveBeenCalledWith(expect.stringContaining('/download?'), { responseType: 'blob' });
    expect(anchor.download).toBe('EV-1.json');
    expect(click).toHaveBeenCalledOnce();
    expect(remove).toHaveBeenCalledOnce();
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:evidence');
    createElement.mockRestore();
    appendChild.mockRestore();
  });

  it('prefers backend evidence errors and hides generic Axios status text', () => {
    expect(alertDetailActionErrorMessage({
      message: 'Request failed with status code 404',
      response: {
        data: {
          error: {
            code: 'EVIDENCE_OBJECT_NOT_FOUND',
            message: 'the original evidence file was not found in object storage',
          },
        },
      },
    }, '证据下载失败，请稍后重试')).toBe('the original evidence file was not found in object storage');
    expect(alertDetailActionErrorMessage(
      new Error('Request failed with status code 502'),
      '证据下载失败，请稍后重试',
    )).toBe('证据下载失败，请稍后重试');
  });
});

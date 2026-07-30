import { beforeEach, describe, expect, it, vi } from 'vitest';
import { submitAlertTriageAction } from './alertTriageApi';
import {
  alertDetailActionErrorMessage,
  downloadAlertEvidenceFile,
  submitAlertDetailAction,
} from './alertDetailActionApi';
import { api } from './api';

vi.mock('./alertTriageApi', () => ({ submitAlertTriageAction: vi.fn() }));
vi.mock('./api', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn() } }));

const submitAlertTriageActionMock = vi.mocked(submitAlertTriageAction);
const apiPutMock = vi.mocked(api.put);
const apiPostMock = vi.mocked(api.post);
const apiGetMock = vi.mocked(api.get);

describe('submitAlertDetailAction', () => {
  beforeEach(() => {
    submitAlertTriageActionMock.mockReset();
    apiPutMock.mockReset();
    apiPostMock.mockReset();
    apiGetMock.mockReset();
    submitAlertTriageActionMock.mockResolvedValue({ job_id: 'alert-action-live-1', status: 'recorded', action: '导出告警报告', target: 'AL-20260620-000123', dry_run: false, audit_event: 'ALERT_INVESTIGATION_NOTE_RECORDED' });
  });

  it('returns a typed live record for a registered report export contract', async () => {
    const result = await submitAlertDetailAction({
      alertId: 'AL-20260620-000123',
      actionId: 'alert-report-export',
      target: 'AL-20260620-000123',
    });

    expect(result.status).toBe('recorded');
    expect(result.mode).toBe('live');
    expect(result.auditEvent).toBe('ALERT_REPORT_EXPORT_REQUESTED');
    expect(result.apiContract).toBe('/v1/alerts/AL-20260620-000123/investigation-notes');
    expect(result.jobId).toBe('alert-action-live-1');
    expect(submitAlertTriageActionMock).toHaveBeenCalledWith(expect.objectContaining({ kind: 'investigation-note', alertId: 'AL-20260620-000123' }));
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

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { submitAlertTriageAction } from './alertTriageApi';
import { submitAlertDetailAction } from './alertDetailActionApi';

vi.mock('./alertTriageApi', () => ({ submitAlertTriageAction: vi.fn() }));

const submitAlertTriageActionMock = vi.mocked(submitAlertTriageAction);

describe('submitAlertDetailAction', () => {
  beforeEach(() => {
    submitAlertTriageActionMock.mockReset();
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
    expect(result.apiContract).toBe('/v1/alerts/AL-20260620-000123/reports/export');
    expect(result.jobId).toBe('alert-action-live-1');
    expect(submitAlertTriageActionMock).toHaveBeenCalledWith(expect.objectContaining({ kind: 'investigation-note', alertId: 'AL-20260620-000123' }));
  });
});

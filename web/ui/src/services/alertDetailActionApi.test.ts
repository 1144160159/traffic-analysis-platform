import { describe, expect, it } from 'vitest';
import { submitAlertDetailAction } from './alertDetailActionApi';

describe('submitAlertDetailAction', () => {
  it('returns a typed simulated task for a registered report export contract', async () => {
    const result = await submitAlertDetailAction({
      alertId: 'AL-20260620-000123',
      actionId: 'alert-report-export',
      target: 'AL-20260620-000123',
    });

    expect(result.status).toBe('queued');
    expect(result.mode).toBe('simulated');
    expect(result.auditEvent).toBe('ALERT_REPORT_EXPORT_REQUESTED');
    expect(result.apiContract).toBe('/v1/alerts/AL-20260620-000123/reports/export');
    expect(result.jobId).toMatch(/^SIM-ALERT-/);
  });
});

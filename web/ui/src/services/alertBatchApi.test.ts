import { describe, expect, it } from 'vitest';
import { buildBatchUpdateAlertStatusRequest } from '@/services/alertBatchApi';

describe('alertBatchApi', () => {
  it('builds versioned batch status update requests for alert-service', () => {
    expect(
      buildBatchUpdateAlertStatusRequest(
        [
          { alertId: ' alert-1 ', stateVersion: 1782712345678 },
          { alertId: 'alert-2' },
        ],
        'closed',
        '  reviewed stale-version contract  ',
      ),
    ).toEqual({
      status: 'closed',
      reason: 'reviewed stale-version contract',
      items: [
        { alert_id: 'alert-1', state_version: 1782712345678 },
        { alert_id: 'alert-2' },
      ],
    });
  });
});

import { describe, expect, it } from 'vitest';
import { buildDLQReplayDryRunRequest } from '@/services/dlqReplayApi';

describe('dlqReplayApi', () => {
  it('defaults fallback replay requests to dry-run mode', () => {
    const request = buildDLQReplayDryRunRequest({
      approved_by: 'operator-2',
      approval_id: 'APPROVAL-20260628-002',
      reason: 'recover after schema repair',
      repair_summary: 'fixed malformed event payloads',
      idempotency_key: 'tenant-a:APPROVAL-20260628-002:dry-run',
    });

    expect(request.dry_run).toBe(true);
    expect(request.idempotency_key).toBe('tenant-a:APPROVAL-20260628-002:dry-run');
  });

  it('preserves explicit execution mode for approved replay requests', () => {
    const request = buildDLQReplayDryRunRequest({
      approved_by: 'operator-2',
      approval_id: 'APPROVAL-20260628-003',
      reason: 'recover after schema repair',
      repair_summary: 'fixed malformed event payloads',
      idempotency_key: 'tenant-a:APPROVAL-20260628-003:execute',
      dry_run: false,
    });

    expect(request.dry_run).toBe(false);
  });
});

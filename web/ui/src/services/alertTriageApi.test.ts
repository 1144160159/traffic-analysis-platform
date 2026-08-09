import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  cancelAlertResponseAction,
  decideAlertResponseAction,
  requestAlertResponseCompensation,
  submitAlertTriageAction,
} from './alertTriageApi';

const { post } = vi.hoisted(() => ({ post: vi.fn() }));
vi.mock('@/services/api', () => ({ api: { post, get: vi.fn() } }));

describe('alert response action contract', () => {
  beforeEach(() => post.mockReset());

  it('sends stable action identity, initial revision and idempotency key', async () => {
    post.mockResolvedValueOnce({
      data: {
        data: {
          job_id: 'job-1',
          status: 'pending_approval',
          revision: 1,
          action: 'block_ip',
          target: '198.51.100.10',
          dry_run: false,
          audit_event: 'ALERT_RESPONSE_ACTION_REQUESTED',
        },
      },
    });

    await submitAlertTriageAction({
      kind: 'response-action',
      actionId: 'alert-response-block-ip',
      alertId: 'AL-1',
      action: 'block_ip',
      target: '198.51.100.10',
      reason: 'confirmed malicious source',
      dryRun: false,
      expectedRevision: 0,
      idempotencyKey: 'alert-response-ui-key-0001',
    });

    expect(post).toHaveBeenCalledWith('/v1/alerts/AL-1/response-actions', expect.objectContaining({
      action_id: 'alert-response-block-ip',
      expected_revision: 0,
      dry_run: false,
    }), {
      headers: { 'Idempotency-Key': 'alert-response-ui-key-0001' },
    });
  });

  it('sends an idempotency key for an atomic saved-view transaction', async () => {
    post.mockResolvedValueOnce({
      data: { data: { view_id: 'view-1', revision: 1, action: 'save_view', target: 'critical-alerts', dry_run: false, audit_event: 'ALERT_VIEW_SAVED' } },
    });

    await submitAlertTriageAction({
      kind: 'saved-view',
      actionId: 'alert-view-save',
      action: 'save_view',
      target: 'critical-alerts',
      reason: 'operator workspace',
      idempotencyKey: 'alert-saved-view-key-0001',
      detail: { filters: { severity: 'critical' } },
    });

    expect(post).toHaveBeenCalledWith('/v1/alerts/views', expect.objectContaining({
      action_id: 'alert-view-save',
      target: 'critical-alerts',
      expected_revision: 0,
    }), {
      headers: { 'Idempotency-Key': 'alert-saved-view-key-0001' },
    });
  });

  it('keeps approval, cancellation and compensation as separate versioned operations', async () => {
    post.mockResolvedValue({ data: { data: { job_id: 'job-1', status: 'accepted', revision: 2, idempotent_reuse: false, outbox_status: 'pending_retry' } } });

    await decideAlertResponseAction('AL-1', 'job-1', {
      decision: 'approve',
      expectedRevision: 1,
      reason: 'independent approval',
      idempotencyKey: 'alert-approval-ui-key-0001',
    });
    await cancelAlertResponseAction('AL-1', 'job-1', {
      expectedRevision: 2,
      reason: 'cancel before delivery',
      idempotencyKey: 'alert-cancel-ui-key-000001',
    });
    await requestAlertResponseCompensation('AL-1', 'job-1', {
      expectedRevision: 3,
      reason: 'restore prior network access',
      idempotencyKey: 'alert-compensate-ui-key-1',
    });

    expect(post.mock.calls.map(([path]) => path)).toEqual([
      '/v1/alerts/AL-1/response-actions/job-1/approval',
      '/v1/alerts/AL-1/response-actions/job-1/cancel',
      '/v1/alerts/AL-1/response-actions/job-1/compensations',
    ]);
    expect(post.mock.calls[0][1]).toEqual({
      decision: 'approve',
      expected_revision: 1,
      reason: 'independent approval',
    });
    expect(post.mock.calls[1][2]).toEqual({
      headers: { 'Idempotency-Key': 'alert-cancel-ui-key-000001' },
    });
  });
});

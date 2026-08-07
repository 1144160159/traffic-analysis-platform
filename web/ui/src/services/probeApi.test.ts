import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api, fetchProbeOperation, submitProbeOperation } from './api';

describe('probe operation API contract', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('submits an asynchronous probe command with an idempotency key', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        data: {
          operation_id: '11111111-1111-1111-1111-111111111111',
          status: 'accepted',
          command_revision: 8,
        },
      },
    } as never);

    const result = await submitProbeOperation('probe-config-push', ['probe-a'], {
      config_version: 'cfg-8',
      reason: 'approved rollout',
    });

    expect(result.status).toBe('accepted');
    expect(post).toHaveBeenCalledWith(
      '/v1/probes/probe-a/config',
      expect.objectContaining({ config_version: 'cfg-8' }),
      { headers: { 'Idempotency-Key': expect.stringMatching(/^probe:probe-config-push:.{8,}$/) } },
    );
  });

  it('loads the authoritative lifecycle state instead of treating HTTP 202 as success', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        data: {
          operation_id: '22222222-2222-2222-2222-222222222222',
          status: 'completed',
          command_revision: 9,
          state_revision: 2,
          reported_version: 'cfg-9',
          reported_hash: 'sha256:cfg-9',
          agent_version: '1.2.0',
        },
      },
    } as never);

    const result = await fetchProbeOperation('22222222-2222-2222-2222-222222222222');

    expect(get).toHaveBeenCalledWith('/v1/probes/operations/22222222-2222-2222-2222-222222222222');
    expect(result).toMatchObject({ status: 'completed', state_revision: 2, reported_version: 'cfg-9' });
  });
});

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api, createForensicsJob } from './api';

describe('forensics source-context API contract', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('persists inbound alert campaign baseline and evidence references with a new task', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { data: { job_id: 'job-1', status: 'queued' } },
    } as never);

    await createForensicsJob({
      assetId: '96343e2f-b391-4bc4-95e2-3343ab0ea94d',
      alertId: 'AL-1',
      campaignId: 'CP-1',
      baselineId: 'BL-1',
      evidenceId: 'EV-1',
      evidenceType: 'pcap',
      startTime: 1_700_000_000_000,
      endTime: 1_700_000_060_000,
      maxPackets: 1000,
    });

    expect(post).toHaveBeenCalledWith('/v1/pcap/jobs', expect.objectContaining({
      asset_id: '96343e2f-b391-4bc4-95e2-3343ab0ea94d',
      alert_id: 'AL-1',
      campaign_id: 'CP-1',
      baseline_id: 'BL-1',
      evidence_id: 'EV-1',
      evidence_type: 'pcap',
      max_packets: 1000,
    }));
  });
});

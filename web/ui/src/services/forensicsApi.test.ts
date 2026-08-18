import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './httpClient';
import {
  cancelForensicsJob,
  createForensicsJob,
  getForensicsJob,
  listForensicsJobs,
  presignForensicsPcap,
  retryForensicsJob,
} from './forensicsApi';

const command = {
  idempotencyKey: 'forensics-create-000001',
  reason: 'investigate encrypted exfiltration',
};

describe('versioned forensics client contract', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('freezes source references, probe set, purpose and command headers on create', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { data: { job_id: 'job-1', status: 'queued', revision: 1 } },
    } as never);

    await createForensicsJob({
      assetId: '96343e2f-b391-4bc4-95e2-3343ab0ea94d',
      alertIds: ['AL-1'],
      campaignId: 'CP-1',
      baselineId: 'BL-1',
      evidenceId: 'EV-1',
      evidenceType: 'pcap',
      probeIds: ['probe-z', 'probe-a', 'probe-z'],
      startTime: 1_700_000_000_000,
      endTime: 1_700_000_060_000,
      maxPackets: 1000,
      purpose: 'investigate encrypted exfiltration',
    }, command);

    expect(post).toHaveBeenCalledWith('/v1/pcap/jobs', expect.objectContaining({
      asset_id: '96343e2f-b391-4bc4-95e2-3343ab0ea94d',
      alert_ids: ['AL-1'],
      campaign_id: 'CP-1',
      baseline_id: 'BL-1',
      evidence_id: 'EV-1',
      evidence_type: 'pcap',
      probe_ids: ['probe-a', 'probe-z'],
      purpose: 'investigate encrypted exfiltration',
      retention_policy: 'forensics-standard',
      restoration_contract_version: 1,
      max_packets: 1000,
    }), {
      headers: {
        'Idempotency-Key': 'forensics-create-000001',
        'X-Action-Reason': 'investigate encrypted exfiltration',
      },
    });
  });

  it('sends revision, idempotency and reason for cancel and retry', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { data: { task_id: 'job-1', status: 'cancelled', revision: 8 } },
    } as never);
    const options = { idempotencyKey: 'forensics-command-000002', reason: 'analyst requested rollback' };
    await cancelForensicsJob('job-1', 7, options);
    await retryForensicsJob('job-1', 8, options);
    const headers = {
      'Idempotency-Key': options.idempotencyKey,
      'X-Action-Reason': options.reason,
      'If-Match': 'W/"7"',
    };
    expect(post).toHaveBeenNthCalledWith(1, '/v1/pcap/jobs/job-1/cancel', undefined, { headers });
    expect(post).toHaveBeenNthCalledWith(2, '/v1/pcap/jobs/job-1/retry', undefined, {
      headers: { ...headers, 'If-Match': 'W/"8"' },
    });
  });

  it('restores the selected job after refresh and keeps partial distinct', async () => {
    const get = vi.spyOn(api, 'get')
      .mockResolvedValueOnce({ data: { data: [{ job_id: 'job-partial', status: 'partial', revision: 4 }], pagination: { total: 1, limit: 20, offset: 0 } } } as never)
      .mockResolvedValueOnce({ data: { data: { job_id: 'job-partial', status: 'partial', revision: 4, manifest_sha256: 'a'.repeat(64) } } } as never);
    const listed = await listForensicsJobs({ taskId: 'job-partial' });
    const selected = await getForensicsJob('job-partial');
    expect(listed.jobs[0].status).toBe('partial');
    expect(selected.status).toBe('partial');
    expect(get).toHaveBeenNthCalledWith(2, '/v1/pcap/jobs/job-partial');
  });

  it('purpose-binds short lived result presigning', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { data: { key: 'tenants/t/forensics/jobs/j/pcap/result.pcap', url: 'https://minio/signed', expires_at: 1 } },
    } as never);
    await presignForensicsPcap('tenants/t/forensics/jobs/j/pcap/result.pcap', 'case CASE-1 evidence review', 900);
    expect(post).toHaveBeenCalledWith('/v1/pcap/presign', {
      key: 'tenants/t/forensics/jobs/j/pcap/result.pcap',
      purpose: 'case CASE-1 evidence review',
      expiry_seconds: 900,
    });
  });
});

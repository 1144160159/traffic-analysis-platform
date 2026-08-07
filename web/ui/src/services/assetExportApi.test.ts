import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import {
  createAssetExportJob,
  fetchAssetColumnPreference,
  fetchAssetExportJob,
  updateAssetColumnPreference,
} from './assetExportApi';

describe('asset export and preference API contract', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('submits a frozen server-side export with explicit idempotency', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { data: { job_id: 'job-1', status: 'accepted' } } } as never);
    const job = await createAssetExportJob({
      format: 'csv',
      columns: ['display_code', 'ip_address'],
      filter: { asset_type: 'server', campus: '主园区' },
      reason: '导出当前服务器筛选结果',
      idempotencyKey: 'asset-export:stable-0001',
    });
    expect(job.status).toBe('accepted');
    expect(post).toHaveBeenCalledWith('/v1/assets/exports', {
      action_id: 'asset-inventory-export',
      format: 'csv',
      columns: ['display_code', 'ip_address'],
      filter: { asset_type: 'server', campus: '主园区' },
      reason: '导出当前服务器筛选结果',
    }, { headers: { 'Idempotency-Key': 'asset-export:stable-0001' } });
  });

  it('reads authoritative job state instead of treating 202 as completion', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { data: { job_id: 'job-1', status: 'completed', artifact_sha256: 'sha256:abc' } } } as never);
    const job = await fetchAssetExportJob('job-1');
    expect(get).toHaveBeenCalledWith('/v1/assets/exports/job-1');
    expect(job).toMatchObject({ status: 'completed', artifact_sha256: 'sha256:abc' });
  });

  it('loads and revision-updates the authenticated user column preference', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { data: { view_id: 'asset-inventory', columns: ['display_code'], revision: 3 } } } as never);
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { data: { view_id: 'asset-inventory', columns: ['display_code', 'hostname'], revision: 4 } } } as never);
    expect((await fetchAssetColumnPreference()).revision).toBe(3);
    expect(get).toHaveBeenCalledWith('/v1/assets/preferences/columns', { params: { view_id: 'asset-inventory' } });
    const updated = await updateAssetColumnPreference({ columns: ['display_code', 'hostname'], expectedRevision: 3, reason: '调整资产清单可见列' });
    expect(updated.revision).toBe(4);
    expect(put).toHaveBeenCalledWith('/v1/assets/preferences/columns', {
      view_id: 'asset-inventory',
      columns: ['display_code', 'hostname'],
      expected_revision: 3,
      reason: '调整资产清单可见列',
    });
  });
});

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { findRouteById } from '@/routes/routeManifest';
import { api } from '@/services/httpClient';
import { closeAlertSearchCursor, fetchAlertSearchCursorSnapshot } from './alertSearchCursorApi';

const alertsPage = () => {
  const route = findRouteById('alerts');
  if (!route) throw new Error('alerts route missing');
  return route.page;
};

const searchEnvelope = (overrides: Record<string, unknown> = {}) => {
  const sourceWatermarks: Record<string, string> = {
    'opensearch.alerts.search': '2026-08-16T01:00:00Z',
    'opensearch.alerts.target_sha256': 'a'.repeat(64),
  };
  return ({
  success: true,
  data: {
    alerts: [{ tenant_id: 'tenant-a', alert_id: 'alert-2', severity: 'high', status: 'new', last_seen: '2026-08-16T01:00:00Z' }],
    total: 2,
    total_relation: 'eq',
    took: 3,
    next_cursor: 'signed-next-cursor',
    has_more: true,
    cursor_mode: 'pit',
    snapshot_id: 'alert-os-pit-1',
    as_of: '2026-08-16T01:00:00Z',
    partial: false,
    ...overrides,
  },
  meta: {
    contract_version: 1,
    snapshot_id: 'alert-os-pit-1',
    as_of: '2026-08-16T01:00:00Z',
    trace_id: 'trace-search-1',
    partial: false,
    missing_sections: [],
    source_watermarks: sourceWatermarks,
  },
  });
};

describe('alert OpenSearch PIT cursor client', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('sends the frozen filter set and preserves cursor and source-watermark evidence', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: searchEnvelope() } as never);
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => ({
      data: path.endsWith('/stats')
        ? { data: { by_severity: { high: 2 }, by_status: { new: 2 }, total: 2 } }
        : { data: [] },
    }) as never);

    const result = await fetchAlertSearchCursorSnapshot({
      page: alertsPage(),
      size: 20,
      cursor: 'signed-current-cursor',
      filters: {
        status: 'new', srcIp: '192.0.2.1', dstIp: '198.51.100.2', assetIp: '192.0.2.8',
        ruleVersion: 'rule-v2', modelVersion: 'model-v3', attackPhase: 'execution', minScore: 0.9,
        startTime: 1_765_000_000_000, endTime: 1_765_086_400_000,
      },
    });

    expect(post).toHaveBeenCalledWith('/v1/alerts/search', expect.objectContaining({
      cursor_mode: 'pit', cursor: 'signed-current-cursor', size: 20,
      status: ['new'], asset_ip: '192.0.2.8', rule_version: 'rule-v2', model_version: 'model-v3',
      attack_phase: 'execution', min_score: 0.9, sort_field: 'last_seen', sort_order: 'desc',
    }));
    expect(result.rows).toHaveLength(1);
    expect(result.cursorSearch).toEqual({
      mode: 'pit', nextCursor: 'signed-next-cursor', hasMore: true,
      totalRelation: 'eq', targetSHA256: 'a'.repeat(64),
    });
    expect(result.snapshot?.sourceWatermarks['opensearch.alerts.target_sha256']).toBe('a'.repeat(64));
  });

  it('fails closed when the cursor contract omits its physical-target watermark', async () => {
    const invalid = searchEnvelope();
    invalid.meta.source_watermarks = { 'opensearch.alerts.search': '2026-08-16T01:00:00Z' };
    vi.spyOn(api, 'post').mockResolvedValue({ data: invalid } as never);
    vi.spyOn(api, 'get').mockResolvedValue({ data: { data: {} } } as never);
    await expect(fetchAlertSearchCursorSnapshot({
      page: alertsPage(), size: 20,
      filters: { startTime: 1, endTime: 2 },
    })).rejects.toThrow('source watermark');
  });

  it('requires an explicit close receipt', async () => {
    const deleteRequest = vi.spyOn(api, 'delete').mockResolvedValue({ data: { data: { closed: true }, meta: {} } } as never);
    await closeAlertSearchCursor('signed-cursor');
    expect(deleteRequest).toHaveBeenCalledWith('/v1/alerts/search/cursor', { data: { cursor: 'signed-cursor' } });
  });
});

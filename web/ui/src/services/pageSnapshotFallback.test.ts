import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { PageSpec } from '@/routes/routeManifest';
import {
  api,
  fetchEntityGraphWorkbench,
  normalizeUnadaptedPageSnapshot,
} from '@/services/api';

const page: PageSpec = {
  id: 'unadapted-domain',
  title: '未适配页面',
  subtitle: '',
  variant: 'quality',
  background: 'overview',
  tabs: [],
  kpis: ['业务总数', '完成率'],
  tableColumns: ['业务ID', '状态'],
  tableTitle: '业务列表',
  rightRailTitle: '摘要',
  actions: [],
  evidence: [],
  apiHints: [],
};

describe('unadapted page snapshot safety', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('preserves explicit zero metadata without mapping arbitrary payload facts into business UI', () => {
    const snapshot = normalizeUnadaptedPageSnapshot(
      page,
      {
        data: { arbitrary_count: 934, rows: [{ id: 'server-value', status: 'server-status' }] },
        total: 0,
        meta: {
          contract_version: 1,
          snapshot_id: 'snapshot-real-1',
          as_of: '2026-08-04T00:00:00Z',
          trace_id: 'trace-real-1',
          partial: false,
          missing_sections: [],
          source_watermarks: { postgresql: 'revision:7' },
        },
      },
      [],
    );

    expect(snapshot.total).toBe(0);
    expect(snapshot.rows).toEqual([]);
    expect(snapshot.metrics).toEqual([
      { label: '业务总数', value: '暂不可用', delta: '缺少类型化页面适配器', status: 'warn' },
      { label: '完成率', value: '暂不可用', delta: '缺少类型化页面适配器', status: 'warn' },
    ]);
    expect(JSON.stringify(snapshot)).not.toContain('934');
    expect(JSON.stringify(snapshot)).not.toContain('server-value');
    expect(snapshot.snapshot).toMatchObject({
      contractVersion: 1,
      snapshotId: 'snapshot-real-1',
      partial: true,
      missingSections: ['typed_page_adapter'],
      sourceWatermarks: { postgresql: 'revision:7' },
    });
  });

  it('keeps missing response metadata absent instead of inventing snapshot identity or totals', () => {
    const snapshot = normalizeUnadaptedPageSnapshot(page, { data: { value: 8 } }, []);

    expect(snapshot.total).toBeUndefined();
    expect(snapshot.snapshot).toBeUndefined();
    expect(snapshot.evidence).toEqual(expect.arrayContaining([
      expect.objectContaining({ label: '返回记录', value: '暂不可用', status: 'warn' }),
    ]));
  });

  it('lets the graph service select an authoritative landing center when no asset was supplied', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        data: {
          graph: { center_id: 'host:authoritative', nodes: [], edges: [] },
          meta: {
            source: 'nebula_graph',
            node_count: 0,
            edge_count: 0,
            depth: 2,
            entity_type: 'all',
            site: 'main',
            time_range: '24h',
            query_duration_ms: 1,
            node_limit: 500,
            cache_hit_rate: 'N/A',
            cache_applicable: false,
            data_origin: 'nebula_graph_persisted_projection',
            slow_query: false,
          },
        },
      },
    } as never);

    await fetchEntityGraphWorkbench(undefined);

    expect(get).toHaveBeenCalledWith('/v1/graph/workbench', {
      params: {
        time_range: '24h',
        site: 'main',
        entity_type: 'all',
        depth: 2,
      },
    });
  });

  it('preserves governed graph continuation, budgets and redaction metadata', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: { data: {
        graph: { center_id: 'host:a', nodes: [], edges: [], truncated: true, truncation_reason: 'node_budget' },
        meta: {
          source: 'nebula_graph', node_count: 100, edge_count: 99, depth: 2, entity_type: 'all', site: 'main', time_range: '24h',
          query_duration_ms: 4, node_limit: 500, response_node_limit: 100, edge_limit: 1000, neighbors_per_hop_limit: 50,
          truncated: true, truncation_reason: 'node_budget', next_continuation: 'opaque-token', continuation_mode: 'replace_accumulated',
          redacted_fields: ['edge.evidence_id'], query_fingerprint: 'fingerprint', as_of_ms: 1234,
          cache_hit_rate: 'N/A', cache_applicable: false, data_origin: 'nebula_graph_persisted_projection', slow_query: false,
        },
      } },
    } as never);

    const result = await fetchEntityGraphWorkbench('host:a', {
      timeRange: '24h', site: 'main', entityType: 'all', depth: 2, continuation: 'prior-token',
    });

    expect(get).toHaveBeenCalledWith('/v1/graph/workbench', { params: expect.objectContaining({ continuation: 'prior-token' }) });
    expect(result.meta).toMatchObject({
      truncated: true, truncation_reason: 'node_budget', next_continuation: 'opaque-token',
      continuation_mode: 'replace_accumulated', redacted_fields: ['edge.evidence_id'], as_of_ms: 1234,
    });
  });
});

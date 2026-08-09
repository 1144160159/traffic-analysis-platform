import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api, fetchAssetDetailSnapshot } from './api';

describe('asset detail snapshot API contract', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('requests one bounded snapshot and preserves cross-store metadata', async () => {
    const envelope = {
      success: true,
      data: {
        contract_version: 1,
        snapshot_id: 'snapshot-1',
        asset: { asset_id: 'asset/with space' },
        details: { asset_id: 'asset/with space' },
        history: [],
        topology: { asset_id: 'asset/with space', nodes: [], edges: [] },
        observations: {
          asset_id: 'asset/with space',
          resolved_identity: { kind: 'ip', value: '192.0.2.8', asset_revision: 7 },
          source: 'clickhouse.sessions',
          window_start: '2026-07-31T00:00:00Z',
          window_end: '2026-08-01T00:00:00Z',
          session_count: 3,
          bytes_total: 2048,
          packets_total: 20,
          distinct_peers: 2,
          protocols: [6],
        },
        alert_context: {
          asset_id: 'asset/with space',
          resolved_identity: { kind: 'ip', value: '192.0.2.8', asset_revision: 7 },
          source: 'clickhouse.alerts.argmax_state_v1',
          window_start: '2026-07-31T00:00:00Z',
          window_end: '2026-08-01T00:00:00Z',
          alerts: [],
          truncated: false,
        },
        graph_projection: {
          asset_id: 'asset/with space',
          source: 'nebulagraph.entity_relation_projection_v1',
          label: 'asset-1',
          detail: '192.0.2.8',
          risk_score: 20,
          risk_level: 'low',
          icon: 'server',
          metadata: { revision: 7 },
          projected_revision: 7,
          postgres_revision: 7,
          updated_at: '2026-08-01T00:00:00Z',
          relations: [],
          truncated: false,
          stale: false,
        },
        evidence_objects: {
          asset_id: 'asset/with space',
          source: 'clickhouse.evidence+minio.stat.v1',
          objects: [],
          missing_evidence_ids: [],
          truncated: false,
          partial: false,
        },
        available_sections: ['asset', 'details', 'history', 'postgresql_topology', 'clickhouse_observations', 'alert_context', 'nebulagraph_projection', 'evidence_objects'],
        missing_sections: [],
        partial: false,
        source_watermarks: { 'postgresql.assets.revision': '7' },
        as_of: '2026-08-01T00:00:00Z',
      },
      meta: {
        contract_version: 1,
        snapshot_id: 'snapshot-1',
        as_of: '2026-08-01T00:00:00Z',
        trace_id: 'trace-1',
        partial: false,
        missing_sections: [],
        source_watermarks: { 'postgresql.assets.revision': '7' },
      },
      error: null,
      timestamp: '2026-08-01T00:00:00Z',
    };
    const get = vi.spyOn(api, 'get').mockResolvedValue({ data: envelope } as never);

    const result = await fetchAssetDetailSnapshot('asset/with space', 25);

    expect(get).toHaveBeenCalledWith('/v1/assets/asset%2Fwith%20space/snapshot', {
      params: { history_limit: 25 },
    });
    expect(result.meta).toMatchObject({ partial: false, trace_id: 'trace-1' });
    expect(result.data.observations?.resolved_identity).toMatchObject({ value: '192.0.2.8', asset_revision: 7 });
    expect(result.data.missing_sections).not.toContain('clickhouse_observations');
  });

  it('rejects an empty asset identifier before issuing a request', async () => {
    const get = vi.spyOn(api, 'get');
    await expect(fetchAssetDetailSnapshot('')).rejects.toThrow('asset id required');
    expect(get).not.toHaveBeenCalled();
  });
});

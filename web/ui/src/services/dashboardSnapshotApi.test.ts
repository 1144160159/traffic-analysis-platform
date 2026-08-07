import { describe, expect, it } from 'vitest';
import { normalizeDashboardSnapshot } from './dashboardSnapshotApi';

describe('dashboardSnapshotApi', () => {
  it('preserves server snapshot metadata, real zero and unknown as distinct values', () => {
    const snapshot = normalizeDashboardSnapshot({
      data: {
        metrics: [
          { key: 'high', label: '高危未处理', value: 0, unit: '条', state: 'ok', delta: 'ClickHouse', spark: [0, 1] },
          { key: 'sla', label: '超时 SLA', value: null, unit: '项', state: 'unknown', delta: '尚无权威水位', spark: [] },
        ],
        queue_total: 1,
        queue: [{ event_id: 'alert-1', risk_level: '高危', asset_group: '10.0.0.1', business_system: 'scan', stage: 'new', remaining: '--', evidence_status: '待补齐' }],
        health_gates: [{ component: 'ClickHouse', status: 'ok', reason: '真实查询', scope: 'tenant-bound', updated: 'wm-1' }],
        stages: [], quality_rings: [], top_talkers: [],
      },
      meta: {
        contract_version: 2, snapshot_id: 'dashboard-snapshot-1', as_of: '2026-08-03T10:00:00Z', trace_id: 'trace-1',
        partial: true, missing_sections: ['kpis.sla'], source_watermarks: { 'clickhouse.dashboard.watermark': 'wm-1' },
      },
      error: null,
    });
    expect(snapshot.metrics[0].value).toBe('0 条');
    expect(snapshot.metrics[1].value).toBe('--');
    expect(snapshot.snapshot).toMatchObject({ snapshotId: 'dashboard-snapshot-1', partial: true, missingSections: ['kpis.sla'] });
    expect(snapshot.rows).toHaveLength(1);
  });

  it('fails closed when canonical snapshot metadata is absent', () => {
    expect(() => normalizeDashboardSnapshot({ data: { metrics: [] }, meta: {} })).toThrow('缺少');
  });
});

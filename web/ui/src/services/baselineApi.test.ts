import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import {
  fetchBehaviorBaselineActions,
  fetchBehaviorBaselineAnalytics,
  fetchBehaviorBaselineOverview,
  fetchBehaviorBaselines,
  fetchBehaviorBaselineVersions,
  submitBehaviorBaselineAction,
} from './baselineApi';

describe('baselineApi', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('sends the selected real history window to the list API', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { baselines: [], total: 0 } } } as never);
    await fetchBehaviorBaselines('protocol', 90, 50);
    expect(api.get).toHaveBeenCalledWith('/v1/baselines', { params: { baseline_type: 'protocol', window_days: 90, limit: 50, offset: 0 } });
  });

  it('loads every aggregate page before applying workbench filters', async () => {
    const summary = { scope: 'all_entities_in_window', total: 3, learning: 0, active: 3, drift: 0, frozen: 0, alerts: 0, rebuild: 0 };
    const baseline = (entityId: string) => ({ baseline_id: `asset:${entityId}`, tenant_id: 'default', name: entityId, entity_type: 'asset', entity_id: entityId, baseline_type: 'asset', metrics: [], status: 'active', created_at: 1, updated_at: 1, version: 1 });
    const get = vi.spyOn(api, 'get')
      .mockResolvedValueOnce({ data: { success: true, data: { baselines: [baseline('a'), baseline('b')], total: 3, summary } } } as never)
      .mockResolvedValueOnce({ data: { success: true, data: { baselines: [baseline('c')], total: 3, summary } } } as never);
    const result = await fetchBehaviorBaselines('asset', 30, 2);
    expect(result.baselines.map((item) => item.entity_id)).toEqual(['a', 'b', 'c']);
    expect(get).toHaveBeenNthCalledWith(2, '/v1/baselines', { params: { baseline_type: 'asset', window_days: 30, limit: 2, offset: 2 } });
  });

  it('can bound the real object sample without changing the server aggregate total', async () => {
    const summary = { scope: 'all_entities_in_window', total: 50_000, learning: 0, active: 50_000, drift: 0, frozen: 0, alerts: 0, rebuild: 0 };
    const baselines = Array.from({ length: 500 }, (_, index) => ({ baseline_id: `port:${index}`, tenant_id: 'default', name: String(index), entity_type: 'port', entity_id: String(index), baseline_type: 'port', metrics: [], status: 'active', created_at: 1, updated_at: 1, version: 1 }));
    const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { baselines, total: 50_000, summary } } } as never);
    const result = await fetchBehaviorBaselines('port', 30, 500, 500);
    expect(result.baselines).toHaveLength(500);
    expect(result.total).toBe(50_000);
    expect(get).toHaveBeenCalledTimes(1);
  });

  it('reads the independent page-level overview contract', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { baseline_type: 'protocol', window_days: 30, source: 'ClickHouse', kpis: [], boxplots: [], heatmap: { x: [], y: [], values: [] }, calendar: { x: [], y: [], values: [] }, series: [], shares: [], links: [], facts: [], availability: { protocol_17: 'included' } } } } as never);
    const result = await fetchBehaviorBaselineOverview('protocol', 30);
    expect(result.availability.protocol_17).toBe('included');
    expect(api.get).toHaveBeenCalledWith('/v1/baselines/overview', { params: { baseline_type: 'protocol', window_days: 30 } });
  });

  it('reads persisted versions and actions for the exact baseline id', async () => {
    const get = vi.spyOn(api, 'get')
      .mockResolvedValueOnce({ data: { success: true, data: { versions: [], total: 0 } } } as never)
      .mockResolvedValueOnce({ data: { success: true, data: { actions: [], total: 0 } } } as never);
    await fetchBehaviorBaselineVersions('asset:10.0.0.1');
    await fetchBehaviorBaselineActions('asset:10.0.0.1');
    expect(get).toHaveBeenNthCalledWith(1, '/v1/baselines/asset%3A10.0.0.1/versions');
    expect(get).toHaveBeenNthCalledWith(2, '/v1/baselines/asset%3A10.0.0.1/actions');
  });

  it('reads real analytics using the selected window', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { baseline_id: 'asset:10.0.0.1', window_days: 7, metric_name: 'bytes_per_session', unit: 'bytes', distributions: [], series: [] } } } as never);
    await fetchBehaviorBaselineAnalytics('asset:10.0.0.1', 7);
    expect(api.get).toHaveBeenCalledWith('/v1/baselines/asset%3A10.0.0.1/analytics', { params: { window_days: 7 } });
  });

  it('keeps the audited governance payload server scoped', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: { action: { action_id: 'a1', baseline_id: 'asset:10.0.0.1', action: 'adjust_threshold', status: 'applied', reason: 'review', requested_by: 'u1', created_at: 1 }, audit_written: true } } } as never);
    await submitBehaviorBaselineAction('asset:10.0.0.1', { action: 'adjust_threshold', reason: 'review', warning_multiplier: 2.1, alert_multiplier: 3.2 });
    expect(api.post).toHaveBeenCalledWith('/v1/baselines/asset%3A10.0.0.1/actions', { action: 'adjust_threshold', reason: 'review', warning_multiplier: 2.1, alert_multiplier: 3.2 });
  });
});

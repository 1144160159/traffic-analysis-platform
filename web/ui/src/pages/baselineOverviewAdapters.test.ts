import { describe, expect, it } from 'vitest';
import type { BehaviorBaselineOverview } from '@/services/baselineApi';
import { baselineAccountNetwork, baselineOverviewHeatmap, baselineOverviewSeries } from './baselineOverviewAdapters';

const overview = {
  baseline_type: 'protocol', window_days: 30, source: 'ClickHouse', kpis: [], boxplots: [],
  heatmap: { x: ['17'], y: ['10.0.0.1'], values: [{ x: 0, y: 0, value: 12 }] },
  calendar: { x: [], y: [], values: [] },
  series: [
    { timestamp: 1000, key: '17', value: 30 }, { timestamp: 1000, key: '6', value: 70 },
    { timestamp: 2000, key: '17', value: 40 }, { timestamp: 2000, key: '6', value: 60 },
  ],
  shares: [],
  links: [{ source: 'ops_admin', target: 'OpenSearch', count: 9, denied: 1 }],
  facts: [], availability: {},
} satisfies BehaviorBaselineOverview;

describe('baseline overview adapters', () => {
  it('keeps heatmap coordinates and observed values unchanged', () => {
    expect(baselineOverviewHeatmap(overview.heatmap)).toEqual({ x: ['17'], y: ['10.0.0.1'], values: [[0, 0, 12]] });
  });

  it('calculates protocol percentages only from the same real time bucket', () => {
    const result = baselineOverviewSeries(overview, (key) => key, true);
    expect(result.series.find((item) => item.name === '17')?.values).toEqual([30, 40]);
    expect(result.series.find((item) => item.name === '6')?.values).toEqual([70, 60]);
  });

  it('builds account-resource edges from API links without generated assets', () => {
    const result = baselineAccountNetwork(overview, 'ops_admin');
    expect(result.nodes.map((item) => item.name)).toEqual(['ops_admin', 'OpenSearch']);
    expect(result.links).toEqual([{ source: 'account:ops_admin', target: 'resource:OpenSearch', value: 9 }]);
  });
});

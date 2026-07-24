import type { BaselineHeatmapDatum, BaselineMultiSeriesDatum, BaselineNetworkDatum } from '@/components/charts';
import type { BehaviorBaselineOverview } from '@/services/baselineApi';

export function baselineOverviewHeatmap(data?: BehaviorBaselineOverview['heatmap']): BaselineHeatmapDatum {
  return {
    x: data?.x ?? [],
    y: data?.y ?? [],
    values: (data?.values ?? []).map((item) => [item.x, item.y, item.value] as [number, number, number]),
  };
}

export function baselineOverviewSeries(
  overview: BehaviorBaselineOverview | undefined,
  label: (key: string) => string,
  percentages = false,
): BaselineMultiSeriesDatum {
  const source = overview?.series ?? [];
  const timestamps = [...new Set(source.map((item) => item.timestamp))].sort((left, right) => left - right);
  const totalsByKey = new Map<string, number>();
  source.forEach((item) => totalsByKey.set(item.key, (totalsByKey.get(item.key) ?? 0) + item.value));
  const keys = [...totalsByKey.entries()].sort((left, right) => right[1] - left[1]).slice(0, 6).map(([key]) => key);
  const totalsByTimestamp = new Map<number, number>();
  source.forEach((item) => totalsByTimestamp.set(item.timestamp, (totalsByTimestamp.get(item.timestamp) ?? 0) + item.value));
  const values = new Map(source.map((item) => [`${item.timestamp}:${item.key}`, item.value]));
  return {
    labels: timestamps.map((timestamp) => new Date(timestamp).toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' })),
    series: keys.map((key) => ({
      name: label(key),
      values: timestamps.map((timestamp) => {
        const value = values.get(`${timestamp}:${key}`) ?? 0;
        return percentages ? value / Math.max(1, totalsByTimestamp.get(timestamp) ?? 0) * 100 : value;
      }),
    })),
  };
}

export function baselineAccountNetwork(overview: BehaviorBaselineOverview | undefined, selectedEntityId: string): BaselineNetworkDatum {
  const sourceLinks = overview?.links ?? [];
  const accountTotals = new Map<string, number>();
  const resourceTotals = new Map<string, number>();
  sourceLinks.forEach((item) => {
    accountTotals.set(item.source, (accountTotals.get(item.source) ?? 0) + item.count);
    resourceTotals.set(item.target, (resourceTotals.get(item.target) ?? 0) + item.count);
  });
  const nodes = [
    ...[...accountTotals.entries()].map(([id, value]) => ({ id: `account:${id}`, name: id, value, category: 0 })),
    ...[...resourceTotals.entries()].map(([id, value]) => ({ id: `resource:${id}`, name: id, value, category: 2 })),
  ];
  if (!accountTotals.has(selectedEntityId)) nodes.unshift({ id: `account:${selectedEntityId}`, name: selectedEntityId, value: 1, category: 0 });
  return {
    nodes,
    links: sourceLinks.map((item) => ({ source: `account:${item.source}`, target: `resource:${item.target}`, value: item.count })),
  };
}

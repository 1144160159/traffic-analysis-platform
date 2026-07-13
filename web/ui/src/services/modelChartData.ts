import type { SnapshotRow } from '@/services/mockData';

const numericValue = (row: SnapshotRow | undefined, key: string, fallback: number) => {
  const value = Number(row?.[key]);
  return Number.isFinite(value) ? value : fallback;
};

const rowSeed = (row?: SnapshotRow) => Array.from(String(row?.__model_id ?? row?.模型名 ?? 'model'))
  .reduce((hash, character) => ((hash * 31) + character.charCodeAt(0)) >>> 0, 7);

export function buildModelMetricTrend(selected: SnapshotRow | undefined, metricIndex: number): number[] {
  const f1 = numericValue(selected, '__f1_score', 0.948);
  const auc = numericValue(selected, '__auc', 0.982);
  const drift = numericValue(selected, '__drift', 0.12);
  const fpDelta = numericValue(selected, '__false_positive_delta', -6.2);
  const bases = [Math.min(99.5, (f1 + 0.023) * 100), Math.max(70, (f1 - 0.023) * 100), f1 * 100, auc * 100, Math.abs(fpDelta), drift * 100, f1 * 100];
  const base = bases[metricIndex] ?? f1 * 100;
  const seed = rowSeed(selected);
  return Array.from({ length: 7 }, (_, point) => Number((base + (((seed + point * (metricIndex + 3)) % 9) - 4) * 0.35).toFixed(3)));
}

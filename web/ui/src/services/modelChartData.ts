export function modelMetricTrendFrom(metric: Record<string, unknown> | undefined): number[] {
  const raw = metric?.trend ?? metric?.values ?? metric?.series;
  if (!Array.isArray(raw)) return [];
  return raw.flatMap((value) => {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? [parsed] : [];
  });
}

export function modelMetricDeltaDisplay(metric: Record<string, unknown>): string {
  const raw = metric.delta;
  if (raw === null || raw === undefined || raw === '') return '变化暂不可用';
  const parsed = Number(raw);
  if (!Number.isFinite(parsed)) return '变化暂不可用';
  const unit = typeof metric.delta_unit === 'string' ? metric.delta_unit : '';
  return `${parsed > 0 ? '+' : ''}${parsed}${unit}`;
}

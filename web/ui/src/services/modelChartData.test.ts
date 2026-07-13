import { describe, expect, it } from 'vitest';
import { buildModelMetricTrend } from './modelChartData';

describe('buildModelMetricTrend', () => {
  it('returns seven points and changes when metrics change for the same model id', () => {
    const first = buildModelMetricTrend({ __model_id: 'MODEL-001', __f1_score: 0.9 }, 2);
    const updated = buildModelMetricTrend({ __model_id: 'MODEL-001', __f1_score: 0.96 }, 2);
    expect(first).toHaveLength(7);
    expect(updated).toHaveLength(7);
    expect(updated).not.toEqual(first);
  });
});

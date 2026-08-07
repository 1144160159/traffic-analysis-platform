import { describe, expect, it } from 'vitest';
import { modelMetricDeltaDisplay, modelMetricTrendFrom } from './modelChartData';

describe('modelMetricTrendFrom', () => {
  it('preserves authoritative numeric points and drops invalid entries', () => {
    expect(modelMetricTrendFrom({ trend: [0.9, '0.92', 'invalid', 0.96] })).toEqual([0.9, 0.92, 0.96]);
  });

  it('returns an empty series when the API does not provide a trend', () => {
    expect(modelMetricTrendFrom({ value: 0.96 })).toEqual([]);
    expect(modelMetricTrendFrom(undefined)).toEqual([]);
  });

  it('does not turn an absent metric delta into a factual zero', () => {
    expect(modelMetricDeltaDisplay({ value: 0.984 })).toBe('变化暂不可用');
    expect(modelMetricDeltaDisplay({ value: 0.984, delta: null })).toBe('变化暂不可用');
    expect(modelMetricDeltaDisplay({ value: 0.984, delta: 'invalid' })).toBe('变化暂不可用');
  });

  it('preserves an authoritative zero and optional delta unit', () => {
    expect(modelMetricDeltaDisplay({ delta: 0 })).toBe('0');
    expect(modelMetricDeltaDisplay({ delta: 0.018 })).toBe('+0.018');
    expect(modelMetricDeltaDisplay({ delta: -0.4, delta_unit: 'pp' })).toBe('-0.4pp');
  });
});

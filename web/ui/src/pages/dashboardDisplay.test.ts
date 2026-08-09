import { describe, expect, it } from 'vitest';
import { formatDashboardDeficitContext, formatDashboardMetricDisplay } from './dashboardDisplay';

describe('formatDashboardMetricDisplay', () => {
  it('compacts large live counters without losing their business unit', () => {
    expect(formatDashboardMetricDisplay('3,442,570 条')).toBe('344.3万条');
    expect(formatDashboardMetricDisplay('939,659 项')).toBe('93.97万项');
    expect(formatDashboardMetricDisplay('201,025 条')).toBe('20.1万条');
    expect(formatDashboardMetricDisplay('128,000,000 条')).toBe('1.28亿条');
  });

  it('keeps small counters, percentages and unavailable values unchanged', () => {
    expect(formatDashboardMetricDisplay('7,432 项')).toBe('7,432 项');
    expect(formatDashboardMetricDisplay('72.7%')).toBe('72.7%');
    expect(formatDashboardMetricDisplay('--')).toBe('--');
  });

  it('turns storage field names into compact operator-facing deficit context', () => {
    expect(formatDashboardDeficitContext('ClickHouse alerts')).toBe('ClickHouse 告警源');
    expect(formatDashboardDeficitContext('未关闭且无 evidence_id')).toBe('evidence_id 缺失');
    expect(formatDashboardDeficitContext('窗口内 dashboard_tasks 无对应 audit_logs')).toBe('audit_log 缺失');
    expect(formatDashboardDeficitContext('高危未关闭且首见超过24小时')).toBe('首见已超过 24h');
  });
});

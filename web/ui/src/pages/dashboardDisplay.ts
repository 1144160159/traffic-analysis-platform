const dashboardNumberPattern = /^([+-]?\d[\d,]*(?:\.\d+)?)\s*(.*)$/u;

const trimTrailingZeroes = (value: string) => value.replace(/\.0+$/u, '').replace(/(\.\d*?)0+$/u, '$1');

export const formatDashboardMetricDisplay = (rawValue: string) => {
  const value = rawValue.trim();
  const match = value.match(dashboardNumberPattern);
  if (!match) return value;

  const numericValue = Number.parseFloat(match[1].replace(/,/gu, ''));
  if (!Number.isFinite(numericValue)) return value;

  const absoluteValue = Math.abs(numericValue);
  const unit = match[2];
  if (absoluteValue >= 100_000_000) {
    const scaledValue = numericValue / 100_000_000;
    return `${trimTrailingZeroes(scaledValue.toFixed(Math.abs(scaledValue) >= 10 ? 1 : 2))}亿${unit}`;
  }
  if (absoluteValue >= 10_000) {
    const scaledValue = numericValue / 10_000;
    return `${trimTrailingZeroes(scaledValue.toFixed(Math.abs(scaledValue) >= 100 ? 1 : 2))}万${unit}`;
  }
  return value;
};

export const formatDashboardDeficitContext = (rawContext: string) => {
  const context = rawContext.trim();
  if (context.includes('dashboard_tasks') || context.includes('audit_logs')) return 'audit_log 缺失';
  if (context.includes('feedback_count')) return 'feedback 缺失';
  if (context.includes('evidence_id')) return 'evidence_id 缺失';
  if (context.includes('ClickHouse') && context.includes('alert')) return 'ClickHouse 告警源';
  if (context.includes('距24小时阈值不足60分钟')) return '距 24h 不足 60min';
  if (context.includes('首见超过24小时')) return '首见已超过 24h';
  if (context.toLowerCase().includes('compliance')) return '合规项未关闭';
  return context;
};

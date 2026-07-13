export const baselineTabLabels = {
  asset: '资产基线',
  account: '账号基线',
  port: '端口基线',
  protocol: '协议基线',
  'time-window': '时间段基线',
} as const;

export type BaselineTabSlug = keyof typeof baselineTabLabels;

export const auditDetailTabLabels = {
  diff: '字段变更对比',
  'operation-context': '操作上下文',
  'related-chain': '关联链路',
} as const;

export type AuditDetailTabSlug = keyof typeof auditDetailTabLabels;

export function resolveBaselineTab(value: string | null, availableTabs: string[]): string {
  const label = baselineTabLabels[(value ?? '') as BaselineTabSlug] ?? baselineTabLabels.asset;
  return availableTabs.includes(label) ? label : availableTabs[0] ?? baselineTabLabels.asset;
}

export function baselineTabSlug(label: string): BaselineTabSlug {
  const entry = Object.entries(baselineTabLabels).find(([, value]) => value === label);
  return (entry?.[0] as BaselineTabSlug | undefined) ?? 'asset';
}

export function resolveAuditDetailTab(value: string | null): string {
  return auditDetailTabLabels[(value ?? '') as AuditDetailTabSlug] ?? auditDetailTabLabels.diff;
}

export function auditDetailTabSlug(label: string): AuditDetailTabSlug {
  const entry = Object.entries(auditDetailTabLabels).find(([, value]) => value === label);
  return (entry?.[0] as AuditDetailTabSlug | undefined) ?? 'diff';
}

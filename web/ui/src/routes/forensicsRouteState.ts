export type ForensicsFocus = 'all' | 'pcap' | 'session';

export type ForensicsSourceContext = {
  assetId: string;
  alertId: string;
  campaignId: string;
  baselineId: string;
  evidenceId: string;
  evidenceType: string;
  focus: ForensicsFocus;
  createRequested: boolean;
};

const first = (params: URLSearchParams, ...keys: string[]) => {
  for (const key of keys) {
    const value = params.get(key)?.trim();
    if (value) return value;
  }
  return '';
};

const resolveFocus = (value: string): ForensicsFocus => {
  const normalized = value.trim().toLowerCase();
  if (normalized === 'pcap' || normalized.includes('pcap')) return 'pcap';
  if (normalized === 'session' || normalized.includes('session')) return 'session';
  return 'all';
};

export function resolveForensicsSourceContext(params: URLSearchParams): ForensicsSourceContext {
  return {
    assetId: first(params, 'asset_id', 'assetId'),
    alertId: first(params, 'alert_id', 'alert'),
    campaignId: first(params, 'campaign_id', 'campaign'),
    baselineId: first(params, 'baseline_id', 'baselineId'),
    evidenceId: first(params, 'evidence_id', 'evidence'),
    evidenceType: first(params, 'evidence_type', 'type'),
    focus: resolveFocus(first(params, 'tab', 'focus')),
    createRequested: first(params, 'create') === '1',
  };
}

export function forensicsSourceLabel(context: ForensicsSourceContext): string {
  if (context.alertId) return `告警 ${context.alertId}`;
  if (context.campaignId) return `战役 ${context.campaignId}`;
  if (context.baselineId) return `基线 ${context.baselineId}`;
  if (context.assetId) return `资产 ${context.assetId}`;
  if (context.evidenceId) return `证据 ${context.evidenceId}`;
  return '未指定来源';
}

export const assetTabs = [
  { label: '终端', slug: 'endpoint' },
  { label: '服务器', slug: 'server' },
  { label: '网络设备', slug: 'network-device' },
  { label: '业务系统', slug: 'business-system' },
  { label: '未知资产', slug: 'unknown' },
] as const;

export type AssetTabSlug = (typeof assetTabs)[number]['slug'];

export const assetDetailTabs = [
  { label: '基础信息', slug: 'basic' },
  { label: '网络接口', slug: 'network-interface' },
  { label: '开放服务', slug: 'open-services' },
  { label: '归属信息', slug: 'ownership' },
  { label: '历史变更', slug: 'history' },
] as const;

export type AssetDetailSlug = (typeof assetDetailTabs)[number]['slug'];

export const resolveAssetTab = (value: string | null): AssetTabSlug =>
  assetTabs.find((item) => item.slug === value)?.slug ?? 'endpoint';

export const resolveAssetDetail = (value: string | null): AssetDetailSlug | null =>
  assetDetailTabs.find((item) => item.slug === value)?.slug ?? null;

export function assetSearchParams(input: {
  tab: AssetTabSlug;
  assetId?: string;
  detail?: AssetDetailSlug | null;
}) {
  const params = new URLSearchParams();
  params.set('tab', input.tab);
  if (input.assetId) params.set('assetId', input.assetId);
  if (input.detail) params.set('detail', input.detail);
  return params;
}

export const canOpenAssetDetail = (tab: AssetTabSlug, assetId: string) =>
  tab === 'server' && assetId.length > 0;

export const assetBreakdownId = (detail: AssetDetailSlug) => `assets-detail-${detail}`;

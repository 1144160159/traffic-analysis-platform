import { describe, expect, it } from 'vitest';
import {
  assetBreakdownId,
  assetSearchParams,
  canOpenAssetDetail,
  resolveAssetDetail,
  resolveAssetTab,
} from './assetInventoryState';

describe('asset inventory route state', () => {
  it('keeps asset category and detail state independent', () => {
    const assetId = '96343e2f-b391-4bc4-95e2-3343ab0ea94d';
    const params = assetSearchParams({ tab: 'server', assetId, detail: 'open-services' });
    expect(params.get('tab')).toBe('server');
    expect(params.get('assetId')).toBe(assetId);
    expect(params.get('detail')).toBe('open-services');
  });

  it('does not invent a display id before the API selects a canonical asset', () => {
    expect(assetSearchParams({ tab: 'unknown' }).toString()).toBe('tab=unknown');
  });

  it('rejects unknown category and detail values without coupling them', () => {
    expect(resolveAssetTab('open-services')).toBe('endpoint');
    expect(resolveAssetDetail('server')).toBeNull();
    expect(resolveAssetDetail('history')).toBe('history');
    expect(assetBreakdownId('network-interface')).toBe('assets-detail-network-interface');
  });

  it('only opens the server detail workspace for a selected server', () => {
    expect(canOpenAssetDetail('server', '96343e2f-b391-4bc4-95e2-3343ab0ea94d')).toBe(true);
    expect(canOpenAssetDetail('server', '')).toBe(false);
    expect(canOpenAssetDetail('endpoint', 'PC-0082')).toBe(false);
    expect(canOpenAssetDetail('unknown', 'UNK-10.12.88.45')).toBe(false);
  });
});

import { describe, expect, it } from 'vitest';
import {
  assetBreakdownId,
  assetSearchParams,
  canOpenAssetDetail,
  defaultAssetIdByTab,
  resolveAssetDetail,
  resolveAssetTab,
} from './assetInventoryState';

describe('asset inventory route state', () => {
  it('keeps asset category and detail state independent', () => {
    const params = assetSearchParams({ tab: 'server', assetId: 'SRV-0007', detail: 'open-services' });
    expect(params.get('tab')).toBe('server');
    expect(params.get('assetId')).toBe('SRV-0007');
    expect(params.get('detail')).toBe('open-services');
  });

  it('uses deterministic selected assets for every category', () => {
    expect(defaultAssetIdByTab).toEqual({
      endpoint: 'PC-0082',
      server: 'SRV-0007',
      'network-device': 'NET-0001',
      'business-system': 'BIZ-0001',
      unknown: 'UNK-10.12.88.45',
    });
  });

  it('rejects unknown category and detail values without coupling them', () => {
    expect(resolveAssetTab('open-services')).toBe('endpoint');
    expect(resolveAssetDetail('server')).toBeNull();
    expect(resolveAssetDetail('history')).toBe('history');
    expect(assetBreakdownId('network-interface')).toBe('assets-detail-network-interface');
  });

  it('only opens the server detail workspace for a selected server', () => {
    expect(canOpenAssetDetail('server', 'SRV-0007')).toBe(true);
    expect(canOpenAssetDetail('server', 'NET-0001')).toBe(false);
    expect(canOpenAssetDetail('endpoint', 'PC-0082')).toBe(false);
    expect(canOpenAssetDetail('unknown', 'UNK-10.12.88.45')).toBe(false);
  });
});

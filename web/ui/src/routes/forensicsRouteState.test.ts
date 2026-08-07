import { describe, expect, it } from 'vitest';
import { forensicsSourceLabel, resolveForensicsSourceContext } from './forensicsRouteState';

describe('forensics route state', () => {
  it('accepts canonical source identifiers and evidence focus', () => {
    const context = resolveForensicsSourceContext(new URLSearchParams(
      'asset_id=asset-1&alert_id=alert-1&campaign_id=campaign-1&baseline_id=baseline-1&evidence_id=evidence-1&evidence_type=pcap&tab=pcap&create=1',
    ));
    expect(context).toEqual({
      assetId: 'asset-1',
      alertId: 'alert-1',
      campaignId: 'campaign-1',
      baselineId: 'baseline-1',
      evidenceId: 'evidence-1',
      evidenceType: 'pcap',
      focus: 'pcap',
      createRequested: true,
    });
    expect(forensicsSourceLabel(context)).toBe('告警 alert-1');
  });

  it('keeps legacy inbound aliases compatible without inventing context', () => {
    expect(resolveForensicsSourceContext(new URLSearchParams('alert=AL-1&baselineId=BL-1&tab=Session%E8%AE%B0%E5%BD%95'))).toMatchObject({
      alertId: 'AL-1',
      baselineId: 'BL-1',
      focus: 'session',
    });
    expect(forensicsSourceLabel(resolveForensicsSourceContext(new URLSearchParams()))).toBe('未指定来源');
  });
});

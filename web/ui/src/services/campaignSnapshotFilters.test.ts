import { describe, expect, it } from 'vitest';
import { buildCampaignRequestParams } from '@/services/api';

describe('campaign snapshot server filters', () => {
  it('maps visible Chinese filters to the bounded backend contract', () => {
    expect(buildCampaignRequestParams({
      risk: '高风险',
      status: '调查中',
      phase: '横向移动',
      keyword: '  RedLync  ',
    })).toEqual({
      risk: 'high',
      status: 'investigating',
      phase: 'lateral_movement',
      keyword: 'RedLync',
    });
  });

  it('omits inactive filters instead of sending display labels', () => {
    expect(buildCampaignRequestParams({ risk: '全部', status: '全部', phase: '全部', keyword: '  ' })).toEqual({});
  });
});

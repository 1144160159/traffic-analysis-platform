import { describe, expect, it } from 'vitest';
import { compactTopicTopologyLabel, safeTopicTopologyRichText } from './topicTopologyLabel';

describe('topic topology labels', () => {
  it('keeps short business labels unchanged', () => {
    expect(compactTopicTopologyLabel('initial_access')).toBe('initial_access');
  });

  it('makes long stable identifiers readable without losing both identity ends', () => {
    expect(compactTopicTopologyLabel('campaign-exfil-default-178597563176-217755ba', 18))
      .toBe('campaign-ex…7755ba');
  });

  it('prevents API values from breaking ECharts rich-text fragments', () => {
    expect(safeTopicTopologyRichText('asset{admin}|root}')).toBe('asset｛admin｝｜root｝');
  });
});

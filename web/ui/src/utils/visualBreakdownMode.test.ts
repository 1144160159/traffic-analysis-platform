import { describe, expect, it } from 'vitest';
import { isVisualBreakdownModeForSearch } from './visualBreakdownMode';

describe('visual breakdown runtime gate', () => {
  it('cannot activate fixture data in a production bundle', () => {
    expect(isVisualBreakdownModeForSearch('?__codex_ui_breakdown_production=1', false)).toBe(false);
  });

  it('allows the explicit visual target only in development', () => {
    expect(isVisualBreakdownModeForSearch('?__codex_ui_breakdown_production=1', true)).toBe(true);
    expect(isVisualBreakdownModeForSearch('?__codex_ui_breakdown_production=0', true)).toBe(false);
    expect(isVisualBreakdownModeForSearch('', true)).toBe(false);
  });
});

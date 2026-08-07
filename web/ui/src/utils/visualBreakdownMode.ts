const visualBreakdownParameter = '__codex_ui_breakdown_production';

export function isVisualBreakdownModeForSearch(search: string, developmentMode: boolean) {
  if (!developmentMode) return false;
  return new URLSearchParams(search).get(visualBreakdownParameter) === '1';
}

export function isVisualBreakdownMode() {
  if (typeof window === 'undefined') return false;
  return isVisualBreakdownModeForSearch(window.location.search, import.meta.env.DEV);
}

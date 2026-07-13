export function isVisualBreakdownMode() {
  if (typeof window === 'undefined') return false;
  return new URLSearchParams(window.location.search).get('__codex_ui_breakdown_production') === '1';
}

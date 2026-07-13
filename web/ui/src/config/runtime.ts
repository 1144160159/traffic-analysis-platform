const runtime = window.__RUNTIME_CONFIG__ ?? {};

const toBoolean = (value: unknown, fallback: boolean) => {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'string') {
    return ['1', 'true', 'yes', 'on'].includes(value.toLowerCase());
  }
  return fallback;
};

const toScreenAccessMode = (value: unknown) => {
  if (value === 'masked-demo') return 'masked-demo';
  return 'protected';
};

export const appConfig = {
  productName: '园区网络全流量采集与分析系统',
  apiBaseUrl: runtime.API_BASE_URL || import.meta.env.VITE_API_BASE_URL || '/api',
  wsUrl: runtime.WS_URL || import.meta.env.VITE_WS_URL || '/ws/events',
  arkimeBaseUrl: runtime.ARKIME_BASE_URL || import.meta.env.VITE_ARKIME_BASE_URL || '',
  authEnabled: toBoolean(runtime.AUTH_ENABLED ?? import.meta.env.VITE_AUTH_ENABLED, true),
  useMock: toBoolean(runtime.USE_MOCK ?? import.meta.env.VITE_USE_MOCK, false),
  enableAlertDetailApi: toBoolean(
    runtime.ALERT_DETAIL_API_ENABLED ?? import.meta.env.VITE_ALERT_DETAIL_API_ENABLED,
    false,
  ),
  enableRealtime: toBoolean(runtime.ENABLE_REALTIME ?? import.meta.env.VITE_ENABLE_REALTIME, false),
  screenAccessMode: toScreenAccessMode(runtime.SCREEN_ACCESS_MODE ?? import.meta.env.VITE_SCREEN_ACCESS_MODE),
  desktopSmokeTokenEnabled: toBoolean(
    runtime.DESKTOP_SMOKE_TOKEN_ENABLED ?? import.meta.env.VITE_DESKTOP_SMOKE_TOKEN_ENABLED,
    false,
  ),
};

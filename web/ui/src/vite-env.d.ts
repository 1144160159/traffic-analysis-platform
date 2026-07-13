/// <reference types="vite/client" />

interface Window {
  __RUNTIME_CONFIG__?: {
    API_BASE_URL?: string;
    WS_URL?: string;
    ARKIME_BASE_URL?: string;
    AUTH_ENABLED?: string | boolean;
    USE_MOCK?: string | boolean;
    ALERT_DETAIL_API_ENABLED?: string | boolean;
    ENABLE_REALTIME?: string | boolean;
    SCREEN_ACCESS_MODE?: 'protected' | 'masked-demo';
    DESKTOP_SMOKE_TOKEN_ENABLED?: string | boolean;
  };
}

import axios from 'axios';
import { appConfig } from '@/config/runtime';
import { clearAuthTokens, getKCRefreshToken, getKCToken, setKCRefreshToken, setKCToken } from '@/services/authStorage';

export const api = axios.create({
  baseURL: appConfig.apiBaseUrl,
  timeout: 30_000,
});

api.interceptors.request.use((config) => {
  // 统一令牌模型(P1 终结态):全站唯一凭证 = Keycloak 访问令牌。
  // 应用 HMAC 令牌桥已下线(全部服务 KC 化:共享中间件或 OIDC 回退均可验)。
  const token = getKCToken();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const config = error.config;
    const url = config?.url ?? '';
    const isAuthFlowUrl = url.includes('/auth/login') || url.includes('/auth/oidc/') || url.includes('/auth/refresh');
    // KC 令牌过期:服务端兑换 refresh(单飞)后重试原请求。
    if (
      axios.isAxiosError(error) &&
      error.response?.status === 401 &&
      !config?._retriedWithKCRefresh &&
      !isAuthFlowUrl
    ) {
      const kcRefresh = getKCRefreshToken();
      if (kcRefresh) {
        config._retriedWithKCRefresh = true;
        try {
          const refreshResp = await axios.post(
            `${appConfig.apiBaseUrl}/v1/auth/refresh`,
            { refresh_token: kcRefresh },
          );
          const data = refreshResp.data as { access_token?: string; refresh_token?: string };
          if (data.access_token) {
            setKCToken(data.access_token);
            if (data.refresh_token) setKCRefreshToken(data.refresh_token);
            config.headers.Authorization = `Bearer ${data.access_token}`;
            return api.request(config);
          }
        } catch {
          // 兑换失败:走原有失效跳转
        }
      }
    }
    if (axios.isAxiosError(error) && error.response?.status === 401) {
      clearAuthTokens();
      // 会话失效统一跳回登录页;登录/回调端点自身的 401(如密码错误)不触发跳转,
      // 避免打断表单错误提示与 OIDC 回调处理。
      const failedUrl = error.config?.url ?? '';
      const isAuthEndpoint = failedUrl.includes('/auth/login') || failedUrl.includes('/auth/oidc/');
      if (!isAuthEndpoint && !appConfig.useMock && typeof window !== 'undefined') {
        const path = window.location.pathname;
        if (path !== '/login' && !path.startsWith('/oidc/callback')) {
          window.location.replace('/login?expired=1');
        }
      }
    }
    return Promise.reject(error);
  },
);

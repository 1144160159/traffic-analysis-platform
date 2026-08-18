const TOKEN_KEY = 'traffic-ui-token';
const REFRESH_TOKEN_KEY = 'traffic-ui-refresh-token';
// Keycloak 访问令牌:网关数据路由按 OIDC 校验,应用 HMAC 令牌过不去;
// 数据 API 用 KC 令牌,鉴权端点(/v1/auth/*)用应用令牌。
const KC_TOKEN_KEY = 'traffic-ui-kc-token';
const KC_REFRESH_KEY = 'traffic-ui-kc-refresh-token';
const SMOKE_TOKEN_PARAM = 'codex_smoke_token';
const SMOKE_REFRESH_PARAM = 'codex_smoke_refresh';
const OIDC_ACCESS_TOKEN_PARAM = 'access_token';
const OIDC_REFRESH_TOKEN_PARAM = 'refresh_token';
const OIDC_TOKEN_TYPE_PARAM = 'token_type';
const OIDC_EXPIRES_IN_PARAM = 'expires_in';
const OIDC_KC_TOKEN_PARAM = 'kc_access_token';
const OIDC_KC_REFRESH_PARAM = 'kc_refresh_token';

let volatileToken: string | null = null;

const getStorage = (remember: boolean) => {
  try {
    if (typeof window === 'undefined') return undefined;
    // 勾选"记住登录"才写入 localStorage;未勾选写入 sessionStorage,关闭会话即失效。
    return remember ? window.localStorage : window.sessionStorage;
  } catch {
    return undefined;
  }
};

export const getKCToken = () => {
  try {
    return getStorage(true)?.getItem(KC_TOKEN_KEY) ?? getStorage(false)?.getItem(KC_TOKEN_KEY) ?? '';
  } catch {
    return '';
  }
};

export const getAuthToken = () => {
  try {
    return getStorage(true)?.getItem(TOKEN_KEY) ?? getStorage(false)?.getItem(TOKEN_KEY) ?? volatileToken;
  } catch {
    return volatileToken;
  }
};

export const setAuthTokens = (token: string, refreshToken?: string, remember = true) => {
  volatileToken = token;

  const storage = getStorage(remember);
  if (!storage) return;
  try {
    storage.setItem(TOKEN_KEY, token);
    if (refreshToken) storage.setItem(REFRESH_TOKEN_KEY, refreshToken);
    if (!remember) getStorage(true)?.removeItem(TOKEN_KEY);
  } catch {
    // Some Desktop Chrome bridge contexts expose storage objects that throw on use.
  }
};

export const setKCToken = (token: string, remember = true) => {
  const storage = getStorage(remember);
  if (!storage) return;
  try {
    storage.setItem(KC_TOKEN_KEY, token);
  } catch {
    // 受限存储环境忽略
  }
};

export const getKCRefreshToken = () => {
  try {
    return getStorage(true)?.getItem(KC_REFRESH_KEY) ?? getStorage(false)?.getItem(KC_REFRESH_KEY) ?? '';
  } catch {
    return '';
  }
};

export const setKCRefreshToken = (token: string, remember = true) => {
  const storage = getStorage(remember);
  if (!storage) return;
  try {
    storage.setItem(KC_REFRESH_KEY, token);
  } catch {
    // 受限存储环境忽略
  }
};

export const clearAuthTokens = () => {
  volatileToken = null;

  for (const storage of [getStorage(true), getStorage(false)]) {
    if (!storage) continue;
    try {
      storage.removeItem(TOKEN_KEY);
      storage.removeItem(REFRESH_TOKEN_KEY);
      storage.removeItem(KC_TOKEN_KEY);
      storage.removeItem(KC_REFRESH_KEY);
    } catch {
      // Clearing the volatile token is enough for restricted browser contexts.
    }
  }
};

export const consumeDesktopSmokeToken = (enabled: boolean) => {
  if (!enabled || typeof window === 'undefined' || !window.location.hash) return false;

  const params = new URLSearchParams(window.location.hash.replace(/^#/, ''));
  const token = params.get(SMOKE_TOKEN_PARAM);
  if (!token) return false;

  setAuthTokens(token, params.get(SMOKE_REFRESH_PARAM) ?? undefined);
  params.delete(SMOKE_TOKEN_PARAM);
  params.delete(SMOKE_REFRESH_PARAM);

  const remainingHash = params.toString();
  const cleanUrl = `${window.location.pathname}${window.location.search}${remainingHash ? `#${remainingHash}` : ''}`;
  window.history.replaceState(window.history.state, document.title, cleanUrl);
  return true;
};

export const consumeOidcCallbackTokens = () => {
  if (typeof window === 'undefined') return false;

  const searchParams = new URLSearchParams(window.location.search);
  const hashParams = new URLSearchParams(window.location.hash.replace(/^#/, ''));
  const token = hashParams.get(OIDC_ACCESS_TOKEN_PARAM) ?? searchParams.get(OIDC_ACCESS_TOKEN_PARAM);
  if (!token) return false;

  // OIDC 回调片段现只携带统一 KC 令牌(access_token=KC access,kc_refresh_token=KC refresh);
  // 兼容历史片段:kc_access_token 存在时以其为准。
  const kcToken = hashParams.get(OIDC_KC_TOKEN_PARAM) ?? searchParams.get(OIDC_KC_TOKEN_PARAM) ?? token;
  const kcRefresh = hashParams.get(OIDC_KC_REFRESH_PARAM) ?? searchParams.get(OIDC_KC_REFRESH_PARAM) ?? '';
  setKCToken(kcToken);
  if (kcRefresh) setKCRefreshToken(kcRefresh);
  // 历史兼容:应用令牌键保留(双 token 桥已下线,不再用于请求)
  const refreshToken = hashParams.get(OIDC_REFRESH_TOKEN_PARAM) ?? searchParams.get(OIDC_REFRESH_TOKEN_PARAM) ?? undefined;
  if (refreshToken) setAuthTokens(token, refreshToken);

  [
    OIDC_ACCESS_TOKEN_PARAM,
    OIDC_REFRESH_TOKEN_PARAM,
    OIDC_TOKEN_TYPE_PARAM,
    OIDC_EXPIRES_IN_PARAM,
    OIDC_KC_TOKEN_PARAM,
    OIDC_KC_REFRESH_PARAM,
  ].forEach((key) => {
    searchParams.delete(key);
    hashParams.delete(key);
  });

  const remainingSearch = searchParams.toString();
  const remainingHash = hashParams.toString();
  const cleanUrl = `${window.location.pathname}${remainingSearch ? `?${remainingSearch}` : ''}${
    remainingHash ? `#${remainingHash}` : ''
  }`;
  window.history.replaceState(window.history.state, document.title, cleanUrl);
  return true;
};

const TOKEN_KEY = 'traffic-ui-token';
const REFRESH_TOKEN_KEY = 'traffic-ui-refresh-token';
const SMOKE_TOKEN_PARAM = 'codex_smoke_token';
const SMOKE_REFRESH_PARAM = 'codex_smoke_refresh';
const OIDC_ACCESS_TOKEN_PARAM = 'access_token';
const OIDC_REFRESH_TOKEN_PARAM = 'refresh_token';
const OIDC_TOKEN_TYPE_PARAM = 'token_type';
const OIDC_EXPIRES_IN_PARAM = 'expires_in';

let volatileToken: string | null = null;

const getStorage = () => {
  try {
    return typeof window === 'undefined' ? undefined : window.localStorage;
  } catch {
    return undefined;
  }
};

export const getAuthToken = () => {
  try {
    return getStorage()?.getItem(TOKEN_KEY) ?? volatileToken;
  } catch {
    return volatileToken;
  }
};

export const setAuthTokens = (token: string, refreshToken?: string) => {
  volatileToken = token;

  const storage = getStorage();
  if (!storage) return;
  try {
    storage.setItem(TOKEN_KEY, token);
    if (refreshToken) storage.setItem(REFRESH_TOKEN_KEY, refreshToken);
  } catch {
    // Some Desktop Chrome bridge contexts expose storage objects that throw on use.
  }
};

export const clearAuthTokens = () => {
  volatileToken = null;

  const storage = getStorage();
  if (!storage) return;
  try {
    storage.removeItem(TOKEN_KEY);
    storage.removeItem(REFRESH_TOKEN_KEY);
  } catch {
    // Clearing the volatile token is enough for restricted browser contexts.
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

  const refreshToken = hashParams.get(OIDC_REFRESH_TOKEN_PARAM) ?? searchParams.get(OIDC_REFRESH_TOKEN_PARAM) ?? undefined;
  setAuthTokens(token, refreshToken);

  [
    OIDC_ACCESS_TOKEN_PARAM,
    OIDC_REFRESH_TOKEN_PARAM,
    OIDC_TOKEN_TYPE_PARAM,
    OIDC_EXPIRES_IN_PARAM,
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

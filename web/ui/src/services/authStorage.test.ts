import { beforeEach, describe, expect, it } from 'vitest';
import {
  clearAuthTokens,
  consumeDesktopSmokeToken,
  consumeOidcCallbackTokens,
  getAuthToken,
  setAuthTokens,
} from '@/services/authStorage';

describe('authStorage', () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.history.replaceState({}, '', '/dashboard');
  });

  it('stores and clears access tokens through the existing local storage keys', () => {
    setAuthTokens('access-token-1', 'refresh-token-1');

    expect(getAuthToken()).toBe('access-token-1');
    expect(window.localStorage.getItem('traffic-ui-refresh-token')).toBe('refresh-token-1');

    clearAuthTokens();
    expect(getAuthToken()).toBeNull();
    expect(window.localStorage.getItem('traffic-ui-refresh-token')).toBeNull();
  });

  it('does not consume desktop smoke fragments unless the runtime flag is enabled', () => {
    window.history.replaceState({}, '', '/dashboard#codex_smoke_token=access-token-1');

    expect(consumeDesktopSmokeToken(false)).toBe(false);
    expect(getAuthToken()).toBeNull();
    expect(window.location.hash).toBe('#codex_smoke_token=access-token-1');
  });

  it('consumes desktop smoke token fragments and removes sensitive hash parameters', () => {
    window.history.replaceState(
      {},
      '',
      '/dashboard?tab=ops#codex_smoke_token=access-token-1&codex_smoke_refresh=refresh-token-1&panel=alerts',
    );

    expect(consumeDesktopSmokeToken(true)).toBe(true);
    expect(getAuthToken()).toBe('access-token-1');
    expect(window.localStorage.getItem('traffic-ui-refresh-token')).toBe('refresh-token-1');
    expect(window.location.pathname).toBe('/dashboard');
    expect(window.location.search).toBe('?tab=ops');
    expect(window.location.hash).toBe('#panel=alerts');
  });

  it('consumes OIDC callback tokens from fragments and keeps non-sensitive route state', () => {
    window.history.replaceState(
      {},
      '',
      '/oidc/callback?next=%2Fdashboard#access_token=oidc-access&refresh_token=oidc-refresh&token_type=Bearer&panel=done',
    );

    expect(consumeOidcCallbackTokens()).toBe(true);
    expect(getAuthToken()).toBe('oidc-access');
    expect(window.localStorage.getItem('traffic-ui-refresh-token')).toBe('oidc-refresh');
    expect(window.location.pathname).toBe('/oidc/callback');
    expect(window.location.search).toBe('?next=%2Fdashboard');
    expect(window.location.hash).toBe('#panel=done');
  });

  it('also supports legacy OIDC callback tokens from query strings', () => {
    window.history.replaceState(
      {},
      '',
      '/oidc/callback?next=%2Fdashboard&access_token=legacy-access&refresh_token=legacy-refresh&expires_in=900',
    );

    expect(consumeOidcCallbackTokens()).toBe(true);
    expect(getAuthToken()).toBe('legacy-access');
    expect(window.localStorage.getItem('traffic-ui-refresh-token')).toBe('legacy-refresh');
    expect(window.location.search).toBe('?next=%2Fdashboard');
  });
});

import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';

type WebSocketRecorderWindow = Window & {
  __WS_ATTEMPTS__?: string[];
};

const installWebSocketRecorder = async (page: Page) => {
  await page.addInitScript(() => {
    const recorderWindow = window as WebSocketRecorderWindow;
    recorderWindow.__WS_ATTEMPTS__ = [];
    class MockSocket extends EventTarget {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSING = 2;
      static readonly CLOSED = 3;
      readonly url: string;
      readyState = MockSocket.CONNECTING;

      constructor(url: string | URL) {
        super();
        this.url = String(url);
        recorderWindow.__WS_ATTEMPTS__?.push(this.url);
        setTimeout(() => {
          this.readyState = MockSocket.OPEN;
          this.dispatchEvent(new Event('open'));
        }, 0);
      }

      close() {
        this.readyState = MockSocket.CLOSED;
        this.dispatchEvent(new Event('close'));
      }

      send() {}
    }
    window.WebSocket = MockSocket as unknown as typeof WebSocket;
  });
};

test('protected routes redirect anonymous users to login', async ({ page }) => {
  await page.addInitScript(() => {
    window.__RUNTIME_CONFIG__ = { ...(window.__RUNTIME_CONFIG__ ?? {}), AUTH_ENABLED: true, USE_MOCK: true };
    window.localStorage.removeItem('traffic-ui-token');
  });

  await page.goto('/dashboard');

  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole('heading', { name: '园区网络全流量采集与分析系统' })).toBeVisible();
  await expect(page.getByLabel('租户')).toBeVisible();
});

test('protected routes show scope evidence when user lacks permission', async ({ page }) => {
  await page.addInitScript(() => {
    window.__RUNTIME_CONFIG__ = { ...(window.__RUNTIME_CONFIG__ ?? {}), AUTH_ENABLED: true, USE_MOCK: false };
    window.localStorage.setItem('traffic-ui-token', 'viewer-token');
  });
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        user_id: 'viewer-1',
        tenant_id: 'default',
        username: 'viewer',
        roles: ['viewer'],
        permissions: ['alert:read', 'rule:read', 'graph:read'],
      }),
    });
  });

  await page.goto('/settings');

  await expect(page.getByRole('heading', { name: '权限不足' })).toBeVisible();
  await expect(page.getByText('当前账号缺少访问「系统设置」所需权限。')).toBeVisible();
  await expect(page.getByText('admin:*')).toBeVisible();
  await expect(page.getByText('token:read')).toBeVisible();
});

test('screen masked demo mode allows anonymous readonly access', async ({ page }) => {
  await page.addInitScript(() => {
    window.__RUNTIME_CONFIG__ = {
      ...(window.__RUNTIME_CONFIG__ ?? {}),
      AUTH_ENABLED: true,
      USE_MOCK: true,
      SCREEN_ACCESS_MODE: 'masked-demo',
    };
    window.localStorage.removeItem('traffic-ui-token');
  });

  await page.goto('/screen');

  await expect(page).toHaveURL(/\/screen$/);
  await expect(page.getByText('脱敏公开演示', { exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: '园区数字孪生拓扑' })).toBeVisible();
  await expect(page.getByText('screen_demo')).toBeVisible();
  await expect(page.locator('body')).not.toContainText('权限不足');
});

test('realtime channel waits for authorization before connecting', async ({ page }) => {
  await installWebSocketRecorder(page);
  await page.addInitScript(() => {
    window.__RUNTIME_CONFIG__ = {
      ...(window.__RUNTIME_CONFIG__ ?? {}),
      AUTH_ENABLED: true,
      USE_MOCK: true,
      ENABLE_REALTIME: true,
    };
    window.localStorage.removeItem('traffic-ui-token');
  });

  await page.goto('/dashboard');

  await expect(page).toHaveURL(/\/login$/);
  await expect.poll(() => page.evaluate(() => (window as WebSocketRecorderWindow).__WS_ATTEMPTS__ ?? [])).toEqual([]);
});

test('realtime channel connects only inside an authorized app shell', async ({ page }) => {
  await installWebSocketRecorder(page);
  await page.addInitScript(() => {
    window.__RUNTIME_CONFIG__ = {
      ...(window.__RUNTIME_CONFIG__ ?? {}),
      AUTH_ENABLED: true,
      USE_MOCK: true,
      ENABLE_REALTIME: true,
      WS_URL: '/ws/events',
    };
    window.localStorage.setItem('traffic-ui-token', 'e2e-token');
  });

  await page.goto('/dashboard');

  await expect(page.getByText('实时通道')).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => (window as WebSocketRecorderWindow).__WS_ATTEMPTS__ ?? []))
    .toEqual([expect.stringContaining('/ws/events?token=e2e-token')]);
});

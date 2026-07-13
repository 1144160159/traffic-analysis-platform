#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = 'http://10.0.5.8:30180';
const cdpUrl = 'http://127.0.0.1:9224';
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/alert-detail/interaction-r240.json');
const screenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/alert-detail/interaction-r240.png');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const encoded = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
    { encoding: 'utf8', env: process.env, timeout: 15_000 },
  );
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-windows-cdp-admin',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:read', 'alert:write'],
    token_type: 'access',
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const signingInput = `${header}.${claims}`;
  const signature = crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(signingInput).digest('base64url');
  return `${signingInput}.${signature}`;
}

function redact(value) {
  return String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
}

function alertRequest(url) {
  return url.includes('/api/v1/alerts/') || url.includes('/v1/alerts/');
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const traffic = [];
const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
page.on('request', (request) => {
  if (alertRequest(request.url())) traffic.push({ method: request.method(), url: redact(request.url()) });
});
page.on('response', (response) => {
  if (alertRequest(response.url()) && response.status() >= 400) {
    badResponses.push({ method: response.request().method(), status: response.status(), url: redact(response.url()) });
  }
});
page.on('console', (entry) => {
  if (entry.type() === 'error') consoleErrors.push(entry.text());
});
page.on('pageerror', (error) => pageErrors.push(error.message));

const liveApiTraffic = [];
const liveApiBadResponses = [];
const liveApiConsoleErrors = [];
const liveApiPageErrors = [];
const livePage = await context.newPage();
await livePage.setViewportSize({ width: 1920, height: 1080 });
livePage.on('request', (request) => {
  if (alertRequest(request.url())) liveApiTraffic.push({ method: request.method(), url: redact(request.url()) });
});
livePage.on('response', (response) => {
  if (alertRequest(response.url()) && response.status() >= 400) {
    liveApiBadResponses.push({ method: response.request().method(), status: response.status(), url: redact(response.url()) });
  }
});
livePage.on('console', (entry) => {
  if (entry.type() === 'error') liveApiConsoleErrors.push(entry.text());
});
livePage.on('pageerror', (error) => liveApiPageErrors.push(error.message));
const liveRouteUrl = new URL(`/alerts/AL-20260620-000123?windowsCdpLiveApiTs=${Date.now()}`, baseUrl);
liveRouteUrl.hash = `codex_smoke_token=${smokeToken()}`;
await livePage.goto(liveRouteUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await livePage.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await livePage.locator('.taf-alert-detail-page').waitFor({ state: 'visible', timeout: 15_000 });
await livePage.locator('.taf-alert-detail-evidence-panel .ant-table').waitFor({ state: 'visible', timeout: 15_000 });
const liveEvidenceRows = await livePage.locator('.taf-alert-detail-evidence-panel .ant-table-tbody > tr').count();
await livePage.close();

const routeUrl = new URL(`/alerts/AL-20260620-000123?__codex_ui_breakdown_production=1&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-alert-detail-page').waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-alert-detail-evidence-panel .ant-table').waitFor({ state: 'visible', timeout: 15_000 });

const actionDrawer = page.locator('.taf-alert-detail-action-drawer:visible');
await page.getByRole('button', { name: '导出报告' }).click();
await actionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const exportDrawerVisible = await actionDrawer.isVisible();
await actionDrawer.getByRole('button', { name: '确认提交' }).click();
await actionDrawer.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 5_000 });
const exportTaskText = await actionDrawer.locator('.ant-alert-success').textContent();
await actionDrawer.locator('.ant-drawer-close').click();
await actionDrawer.waitFor({ state: 'hidden', timeout: 5_000 });

const tableRowsPageOne = await page.locator('.taf-alert-detail-evidence-panel .ant-table-tbody > tr').count();
const tableNext = page.locator('.taf-alert-detail-evidence-panel .ant-pagination-next');
await tableNext.click();
const evidencePageTwo = await page.locator('.taf-alert-detail-evidence-panel .ant-pagination-item-active').textContent();
const tableRowsPageTwo = await page.locator('.taf-alert-detail-evidence-panel .ant-table-tbody > tr').count();

await page.locator('.taf-alert-detail-evidence-panel .taf-alert-detail-evidence-action').first().click();
await actionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const evidenceActionDrawerVisible = await actionDrawer.isVisible();
await actionDrawer.locator('.ant-drawer-close').click();

await page.locator('.taf-alert-detail-response button').first().click();
await actionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const responseActionDrawerVisible = await actionDrawer.isVisible();
await actionDrawer.locator('.ant-drawer-close').click();

const scrollSupport = await page.evaluate(() => {
  const panel = document.querySelector('.taf-alert-detail-evidence-panel .taf-panel__body');
  const tableBody = document.querySelector('.taf-alert-detail-evidence-panel .ant-table-body');
  if (!panel || !tableBody) return null;
  return {
    panelOverflowY: window.getComputedStyle(panel).overflowY,
    tableOverflowX: window.getComputedStyle(tableBody).overflowX,
  };
});

fs.mkdirSync(path.dirname(outputPath), { recursive: true });
await page.screenshot({ path: screenshotPath, fullPage: false });
const result = {
  result: exportDrawerVisible
    && Boolean(exportTaskText?.includes('SIM-ALERT-'))
    && evidencePageTwo === '2'
    && tableRowsPageOne >= 4
    && tableRowsPageTwo >= 1
    && tableRowsPageTwo < tableRowsPageOne
    && evidenceActionDrawerVisible
    && responseActionDrawerVisible
    && scrollSupport?.panelOverflowY === 'auto'
    && ['auto', 'scroll'].includes(scrollSupport?.tableOverflowX ?? '')
    && liveApiTraffic.length === 0
    && liveApiBadResponses.length === 0
    && liveApiConsoleErrors.length === 0
    && liveApiPageErrors.length === 0
    && badResponses.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0 ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  data_mode: 'typed visual-breakdown fallback served by the production web-ui route',
  live_api_smoke: {
    data_mode: 'typed fallback because ALERT_DETAIL_API_ENABLED=false',
    route: redact(liveRouteUrl.toString()),
    evidence_row_count: liveEvidenceRows,
    request_count: liveApiTraffic.length,
    requests: liveApiTraffic,
    bad_responses: liveApiBadResponses,
    console_errors: liveApiConsoleErrors,
    page_errors: liveApiPageErrors,
  },
  route: redact(routeUrl.toString()),
  timestamp: new Date().toISOString(),
  export_drawer_visible: exportDrawerVisible,
  export_task_text: exportTaskText,
  evidence_page_two: evidencePageTwo,
  table_rows: { page_one: tableRowsPageOne, page_two: tableRowsPageTwo },
  evidence_action_drawer_visible: evidenceActionDrawerVisible,
  response_action_drawer_visible: responseActionDrawerVisible,
  scroll_support: scrollSupport,
  api_request_count: traffic.length,
  api_requests: traffic,
  bad_responses: badResponses,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  screenshot: path.relative(root, screenshotPath),
};
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await browser.close();
if (result.result !== 'pass') process.exitCode = 1;

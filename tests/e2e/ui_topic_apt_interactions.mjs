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
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/topics-apt-campaign/interaction-r241.json');
const screenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/topics-apt-campaign/interaction-r241.png');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const encoded = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(), tenant_id: 'default',
    username: 'codex-windows-cdp-admin', roles: ['admin'], permissions: ['*', 'admin:*', 'topic:read', 'topic:write'], token_type: 'access', iat: now, exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
const consoleErrors = [];
const pageErrors = [];
const badResponses = [];
page.on('console', (entry) => { if (entry.type() === 'error') consoleErrors.push(entry.text()); });
page.on('pageerror', (error) => pageErrors.push(error.message));
page.on('response', (response) => { if (response.status() >= 400) badResponses.push({ status: response.status(), url: response.url() }); });

const routeUrl = new URL(`/topics?topic=apt&tab=apt&__codex_ui_breakdown_production=1&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-topic-apt-layout').waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-topic-apt-trend-echart canvas').waitFor({ state: 'visible', timeout: 10_000 });
await page.locator('.taf-topic-apt-response-echart canvas').waitFor({ state: 'visible', timeout: 10_000 });
const trendCanvasCount = await page.locator('.taf-topic-apt-trend-echart canvas').count();
const responseCanvasCount = await page.locator('.taf-topic-apt-response-echart canvas').count();

await page.getByRole('button', { name: '战役耗时线' }).click();
const activeAnalysisTab = await page.locator('.taf-topic-apt-tabs button.is-active').textContent();
await page.getByRole('button', { name: 'APT 证据下一页' }).click();
const evidencePageTwo = await page.locator('.taf-topic-apt-table-footer button[aria-current="page"]').textContent();
const drawer = page.locator('.taf-topic-action-drawer:visible');
await page.locator('.taf-topic-apt-table-actions button').first().click();
await drawer.waitFor({ state: 'visible', timeout: 5_000 });
await drawer.getByRole('button', { name: '确认提交' }).click();
await drawer.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 5_000 });
const actionResultVisible = await drawer.locator('.ant-alert-success').isVisible();
await drawer.locator('.ant-drawer-close').click();
const tableOverflow = await page.evaluate(() => window.getComputedStyle(document.querySelector('.taf-topic-apt-evidence-table')).overflowY);
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
await page.screenshot({ path: screenshotPath, fullPage: false });
const result = {
  result: trendCanvasCount === 1
    && responseCanvasCount === 1
    && activeAnalysisTab === '战役耗时线'
    && evidencePageTwo === '2'
    && actionResultVisible
    && tableOverflow === 'auto'
    && badResponses.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0 ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP', browser: version.Browser, route: routeUrl.toString().replace(/codex_smoke_token=[^&#]+/, 'codex_smoke_token=<redacted>'),
  trend_canvas_count: trendCanvasCount, response_canvas_count: responseCanvasCount, active_analysis_tab: activeAnalysisTab,
  evidence_page_two: evidencePageTwo, action_result_visible: actionResultVisible, table_overflow_y: tableOverflow,
  bad_responses: badResponses, console_errors: consoleErrors, page_errors: pageErrors, screenshot: path.relative(root, screenshotPath), timestamp: new Date().toISOString(),
};
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await browser.close();
if (result.result !== 'pass') process.exitCode = 1;

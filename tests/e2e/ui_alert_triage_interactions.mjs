#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const baseUrl = 'http://10.0.5.8:30180';
const cdpUrl = 'http://127.0.0.1:9224';
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/alerts/interaction-r255.json');
const screenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/alerts/interaction-r255.png');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
const redact = (value) => String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
function smokeToken() {
  const secret = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({ iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(), tenant_id: 'default', username: 'codex-windows-cdp-admin', roles: ['admin'], permissions: ['*', 'admin:*', 'alert:read', 'alert:write'], token_type: 'access', iat: now, exp: now + 1_800 })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(secret, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
page.setDefaultTimeout(10_000);
const badResponses = []; const consoleErrors = []; const pageErrors = []; const requestFailures = [];
page.on('response', (response) => { if (response.status() >= 400) badResponses.push({ status: response.status(), url: redact(response.url()) }); });
page.on('console', (entry) => { if (entry.type() === 'error') consoleErrors.push(entry.text()); });
page.on('pageerror', (error) => pageErrors.push(error.message));
page.on('requestfailed', (request) => requestFailures.push(`${request.method()} ${redact(request.url())} ${request.failure()?.errorText ?? ''}`));

const routeUrl = new URL(`/alerts?__codex_ui_breakdown_production=1&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-alert-triage').waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-alert-risk-echart canvas').waitFor({ state: 'visible', timeout: 10_000 });
const gaugeCanvasCount = await page.locator('.taf-alert-risk-echart canvas').count();
await page.locator('.taf-alert-table-panel .ant-pagination-item-2').click();
const tablePageTwo = await page.locator('.taf-alert-table-panel .ant-pagination-item-active').textContent();
const tableOverflowY = await page.evaluate(() => window.getComputedStyle(document.querySelector('.taf-alert-table-panel .ant-table-body')).overflowY);

const drawer = page.locator('.ant-drawer-content-wrapper:visible');
await page.getByRole('button', { name: '保存视图' }).click();
await drawer.waitFor({ state: 'visible', timeout: 5_000 });
await drawer.getByRole('button', { name: '确认提交' }).click();
await drawer.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 5_000 });
const viewActionVisible = await drawer.locator('.ant-alert-success').isVisible();
await drawer.locator('.ant-drawer-close').click();
await drawer.waitFor({ state: 'hidden', timeout: 5_000 });

await page.locator('.taf-alert-row-actions .ant-btn').first().click();
await drawer.waitFor({ state: 'visible', timeout: 5_000 });
const rowActionVisible = await drawer.isVisible();
await drawer.locator('.ant-drawer-close').click();
await drawer.waitFor({ state: 'hidden', timeout: 5_000 });

await page.getByRole('button', { name: /同源 IP/ }).click();
await drawer.waitFor({ state: 'visible', timeout: 5_000 });
const clusterActionVisible = await drawer.isVisible();
await drawer.locator('.ant-drawer-close').click();
await drawer.waitFor({ state: 'hidden', timeout: 5_000 });

await page.screenshot({ path: screenshotPath, fullPage: false });
const result = { result: gaugeCanvasCount === 1 && tablePageTwo === '2' && ['auto', 'scroll'].includes(tableOverflowY) && viewActionVisible && rowActionVisible && clusterActionVisible && badResponses.length === 0 && consoleErrors.length === 0 && pageErrors.length === 0 && requestFailures.length === 0 ? 'pass' : 'fail', browser_backend: 'Windows Chrome CDP', browser: version.Browser, route: redact(routeUrl.toString()), gauge_canvas_count: gaugeCanvasCount, table_page_two: tablePageTwo, table_overflow_y: tableOverflowY, view_action_visible: viewActionVisible, row_action_visible: rowActionVisible, cluster_action_visible: clusterActionVisible, bad_responses: badResponses, console_errors: consoleErrors, page_errors: pageErrors, request_failures: requestFailures, screenshot: path.relative(root, screenshotPath), timestamp: new Date().toISOString() };
fs.mkdirSync(path.dirname(outputPath), { recursive: true }); fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`); console.log(JSON.stringify(result, null, 2)); await page.close().catch(() => {}); process.exit(result.result === 'pass' ? 0 : 1);

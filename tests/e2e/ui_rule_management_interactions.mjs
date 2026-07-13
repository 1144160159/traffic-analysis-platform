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
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/rules/interaction-r250.json');
const screenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/rules/interaction-r250.png');

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
    permissions: ['*', 'admin:*', 'rule:read', 'rule:write', 'rule:enable'],
    token_type: 'access',
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

function redact(value) {
  return String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
page.setDefaultTimeout(10_000);

const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
page.on('response', (response) => {
  if (response.status() >= 400) badResponses.push({ status: response.status(), url: redact(response.url()) });
});
page.on('console', (entry) => { if (entry.type() === 'error') consoleErrors.push(entry.text()); });
page.on('pageerror', (error) => pageErrors.push(error.message));
page.on('requestfailed', (request) => requestFailures.push(`${request.method()} ${redact(request.url())} ${request.failure()?.errorText ?? ''}`));

const routeUrl = new URL(`/rules?__codex_ui_breakdown_production=1&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-rules').waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-rules-performance-echart canvas').first().waitFor({ state: 'visible', timeout: 10_000 });

const performanceCanvasCount = await page.locator('.taf-rules-performance-echart canvas').count();
await page.getByRole('button', { name: '规则第 2 页' }).click();
const listPageTwo = await page.locator('.taf-rules-pagination button.is-active').textContent();
const listOverflowY = await page.evaluate(() => window.getComputedStyle(document.querySelector('.taf-rules-list-panel .ant-table-body')).overflowY);

const actionDrawer = page.locator('.ant-drawer-content-wrapper:visible');
await page.getByRole('button', { name: '新建规则' }).click();
await actionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
await actionDrawer.getByRole('button', { name: '确认提交' }).click();
await actionDrawer.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 5_000 });
const createActionVisible = await actionDrawer.locator('.ant-alert-success').isVisible();
await actionDrawer.locator('.ant-drawer-close').click();
await actionDrawer.waitFor({ state: 'hidden', timeout: 5_000 });

await page.getByRole('button', { name: '测试验证', exact: true }).click();
const editorTabActive = await page.getByRole('button', { name: '测试验证', exact: true }).evaluate((button) => button.classList.contains('is-active'));
await page.getByRole('button', { name: 'Session 样本 128', exact: true }).click();
const sampleTabActive = await page.getByRole('button', { name: 'Session 样本 128', exact: true }).evaluate((button) => button.classList.contains('is-active'));

await page.getByRole('button', { name: '全量发布', exact: true }).click();
await actionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const publishActionVisible = await actionDrawer.isVisible();
await actionDrawer.locator('.ant-drawer-close').click();
await actionDrawer.waitFor({ state: 'hidden', timeout: 5_000 });

await page.screenshot({ path: screenshotPath, fullPage: false });
const result = {
  result: performanceCanvasCount === 4
    && listPageTwo === '2'
    && listOverflowY === 'auto'
    && createActionVisible
    && editorTabActive
    && sampleTabActive
    && publishActionVisible
    && badResponses.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0
    && requestFailures.length === 0 ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  route: redact(routeUrl.toString()),
  performance_canvas_count: performanceCanvasCount,
  list_page_two: listPageTwo,
  list_overflow_y: listOverflowY,
  create_action_visible: createActionVisible,
  editor_tab_active: editorTabActive,
  sample_tab_active: sampleTabActive,
  publish_action_visible: publishActionVisible,
  bad_responses: badResponses,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  request_failures: requestFailures,
  screenshot: path.relative(root, screenshotPath),
  timestamp: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);

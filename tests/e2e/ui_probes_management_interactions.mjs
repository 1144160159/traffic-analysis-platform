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
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/probes/interaction-r259-topology-svg.json');
const screenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/probes/interaction-r259-topology-svg.png');

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
    permissions: ['*', 'admin:*', 'probe:read', 'probe:write'],
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

const routeUrl = new URL(`/probes?__codex_ui_breakdown_production=1&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-probes').waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-probes-topology-svg .taf-api-topology-svg').waitFor({ state: 'visible', timeout: 10_000 });
await page.locator('.taf-probes-trend-echart canvas').first().waitFor({ state: 'visible', timeout: 10_000 });

const topologySvgCount = await page.locator('.taf-probes-topology-svg .taf-api-topology-svg').count();
const topologyNodeCount = await page.locator('.taf-probes-topology-svg .taf-api-topology-svg__node').count();
const trendCanvasCount = await page.locator('.taf-probes-trend-echart canvas').count();
await page.getByRole('button', { name: '2D', exact: true }).click();
const topology2dActive = await page.getByRole('button', { name: '2D', exact: true }).getAttribute('aria-pressed') === 'true';

await page.locator('.taf-probes-trends-panel .ant-select').click();
await page.getByText('近 24 小时', { exact: true }).last().evaluate((option) => option.click());
const selectedTrendRange = await page.locator('.taf-probes-trends-panel .ant-select-selection-item').textContent();

await page.getByRole('button', { name: '探针状态第 2 页' }).click();
const matrixPageTwo = await page.locator('.taf-probes-pagination button.is-active').textContent();
const matrixOverflowY = await page.evaluate(() => window.getComputedStyle(document.querySelector('.taf-probes-status-matrix')).overflowY);

const actionDrawer = page.locator('.ant-drawer-content-wrapper:visible');
await page.getByRole('button', { name: '批量升级' }).first().evaluate((button) => button.click());
await actionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
await actionDrawer.getByRole('button', { name: '确认提交' }).click();
await actionDrawer.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 5_000 });
const batchActionVisible = await actionDrawer.locator('.ant-alert-success').isVisible();
await actionDrawer.locator('.ant-drawer-close').click();
await actionDrawer.waitFor({ state: 'hidden', timeout: 5_000 });

await page.locator('button[aria-label="配置探针"]').first().click();
await actionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const rowActionVisible = await actionDrawer.isVisible();
await actionDrawer.locator('.ant-drawer-close').click();
await actionDrawer.waitFor({ state: 'hidden', timeout: 5_000 });

await page.getByRole('button', { name: '全屏查看矩阵' }).click();
const matrixDrawer = page.locator('.ant-drawer-content-wrapper:visible');
await matrixDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const fullscreenMatrixVisible = await matrixDrawer.isVisible();
await matrixDrawer.locator('.ant-drawer-close').click();
await matrixDrawer.waitFor({ state: 'hidden', timeout: 5_000 });

await page.screenshot({ path: screenshotPath, fullPage: false });
const result = {
  result: topologySvgCount === 1
    && topologyNodeCount >= 8
    && trendCanvasCount === 2
    && topology2dActive
    && selectedTrendRange === '近 24 小时'
    && matrixPageTwo === '2'
    && matrixOverflowY === 'auto'
    && batchActionVisible
    && rowActionVisible
    && fullscreenMatrixVisible
    && badResponses.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0
    && requestFailures.length === 0 ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  route: redact(routeUrl.toString()),
  topology_svg_count: topologySvgCount,
  topology_node_count: topologyNodeCount,
  trend_canvas_count: trendCanvasCount,
  topology_2d_active: topology2dActive,
  selected_trend_range: selectedTrendRange,
  matrix_page_two: matrixPageTwo,
  matrix_overflow_y: matrixOverflowY,
  batch_action_visible: batchActionVisible,
  row_action_visible: rowActionVisible,
  fullscreen_matrix_visible: fullscreenMatrixVisible,
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

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
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/dashboard/interaction-r260.json');
const screenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/dashboard/interaction-r260.png');

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
    permissions: ['*', 'admin:*', 'dashboard:read', 'dashboard:write'],
    token_type: 'access',
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
page.on('response', (response) => {
  if (response.status() >= 400) badResponses.push({ status: response.status(), url: response.url() });
});
page.on('console', (entry) => {
  if (entry.type() === 'error') consoleErrors.push(entry.text());
});
page.on('pageerror', (error) => pageErrors.push(error.message));
page.on('requestfailed', (request) => requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? 'unknown' }));

const routeUrl = new URL(`/dashboard?__codex_ui_breakdown_production=1&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-dashboard-workbench').waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-dashboard-workbench canvas').first().waitFor({ state: 'visible', timeout: 15_000 });

const chartCanvasCount = await page.locator('.taf-dashboard-workbench canvas').count();
await page.locator('.taf-deficit-action').first().click();
const drawer = page.locator('.taf-dashboard-action-drawer:visible');
await drawer.waitFor({ state: 'visible', timeout: 5_000 });
const deficitActionVisible = await drawer.isVisible();
const endpointVisible = await drawer.getByText('/v1/dashboard/tasks/evidence', { exact: true }).isVisible();
const auditEventVisible = await drawer.getByText('DASHBOARD_EVIDENCE_TASK_CREATED', { exact: true }).isVisible();
await drawer.getByRole('button', { name: '确认提交' }).click();
await drawer.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 5_000 });
const submitFeedbackVisible = await drawer.locator('.ant-alert-success').isVisible();
await drawer.locator('.ant-drawer-close').click();

await page.locator('.taf-dashboard-pages button[title="第 2 页"]').click();
const queuePageTwo = await page.locator('.taf-dashboard-pages button[aria-current="page"]').textContent();
const queueRowCount = await page.locator('.taf-dashboard-queue-table__row').count();
await page.screenshot({ path: screenshotPath, fullPage: false });

const result = {
  result: chartCanvasCount >= 20
    && deficitActionVisible
    && endpointVisible
    && auditEventVisible
    && submitFeedbackVisible
    && queuePageTwo === '2'
    && queueRowCount === 8
    && badResponses.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0
    && requestFailures.length === 0 ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  route: routeUrl.toString().replace(/codex_smoke_token=[^&#]+/, 'codex_smoke_token=<redacted>'),
  chart_canvas_count: chartCanvasCount,
  deficit_action_visible: deficitActionVisible,
  endpoint_visible: endpointVisible,
  audit_event_visible: auditEventVisible,
  submit_feedback_visible: submitFeedbackVisible,
  queue_page_two: queuePageTwo,
  queue_row_count: queueRowCount,
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

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
const evidenceRevision = process.env.DQ_EVIDENCE_REVISION?.trim() || 'r264';
const outputPath = path.join(root, `evidence/ui-image-breakdowns/pages/data-quality/interaction-${evidenceRevision}-all-tabs.json`);
const screenshotDir = path.join(root, 'evidence/ui-image-breakdowns/pages/data-quality');

const tabs = [
  { slug: 'overview', label: '质量总览', selector: '.taf-data-quality-anomalies button' },
  { slug: 'topic-health', label: 'Topic 健康', selector: '.taf-data-quality-topic-rail-alerts button' },
  { slug: 'flink-quality', label: 'Flink 质量', selector: '.taf-data-quality-flink-failure-table button' },
  { slug: 'field-quality', label: '字段质量', selector: '.taf-data-quality-field-sample-table button' },
  { slug: 'storage-quality', label: '存储质量', selector: '.taf-data-quality-storage-component-table button' },
  { slug: 'replay-reconcile', label: '重放对账', selector: '.taf-data-quality-replay-task-table button' },
  { slug: 'report', label: '质量报告', selector: '.taf-data-quality-report-viewerbar button' },
  { slug: 'settings', label: '质量设置', selector: '.taf-data-quality-settings-threshold button' },
];

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
    permissions: ['*', 'admin:*', 'data-quality:read', 'data-quality:write', 'dlq:replay'],
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

const requestedTab = process.env.DQ_TAB?.trim();
const selectedTabs = requestedTab ? tabs.filter((tab) => tab.slug === requestedTab) : tabs;
if (!selectedTabs.length) throw new Error(`Unknown DQ_TAB: ${requestedTab}`);
const previous = process.env.DQ_RESET === '1' || !fs.existsSync(outputPath)
  ? { tab_results: [], bad_responses: [], console_errors: [], page_errors: [], request_failures: [] }
  : JSON.parse(fs.readFileSync(outputPath, 'utf8'));
const token = smokeToken();
const tabResults = Array.isArray(previous.tab_results) ? previous.tab_results.filter((tab) => !selectedTabs.some((selected) => selected.slug === tab.slug)) : [];
fs.mkdirSync(screenshotDir, { recursive: true });

for (const tab of selectedTabs) {
  const routeUrl = new URL(`/data-quality?tab=${tab.slug}&__codex_ui_breakdown_production=1&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
  routeUrl.hash = `codex_smoke_token=${token}`;
  await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  await page.locator('.taf-data-quality').waitFor({ state: 'visible', timeout: 15_000 });
  await page.locator(`.taf-data-quality-tabs button[data-tab-slug="${tab.slug}"][aria-selected="true"]`).waitFor({ state: 'visible', timeout: 10_000 });
  const actionControl = page.locator(tab.selector).first();
  await actionControl.waitFor({ state: 'visible', timeout: 10_000 });
  const chartCanvasCount = await page.locator('.taf-data-quality canvas').count();
  const tabGeometry = await page.locator('.taf-data-quality-tabs button').evaluateAll((buttons) => buttons.map((button) => {
    const box = button.getBoundingClientRect();
    return {
      slug: button.getAttribute('data-tab-slug'),
      x: Number(box.x.toFixed(3)),
      y: Number(box.y.toFixed(3)),
      width: Number(box.width.toFixed(3)),
      height: Number(box.height.toFixed(3)),
    };
  }));
  await actionControl.click();
  const drawer = page.locator('.taf-data-quality-field-detail-drawer:visible');
  await drawer.waitFor({ state: 'visible', timeout: 5_000 });
  const drawerVisible = await drawer.isVisible();
  const endpointVisible = await drawer.getByText('POST /v1/data-quality/actions', { exact: true }).isVisible();
  const auditEventVisible = await drawer.getByText('DATA_QUALITY_ACTION_REQUESTED', { exact: true }).isVisible();
  await drawer.locator('.ant-drawer-close').evaluate((button) => button.click());

  const screenshotPath = path.join(screenshotDir, `interaction-${evidenceRevision}-${tab.slug}.png`);
  await page.screenshot({ path: screenshotPath, fullPage: false });
  let fieldQualityClickGeometryDelta = null;
  if (tab.slug === 'overview') {
    await page.locator('.taf-data-quality-tabs button[data-tab-slug="field-quality"]').click();
    await page.locator('.taf-data-quality-tabs button[data-tab-slug="field-quality"][aria-selected="true"]').waitFor({ state: 'visible', timeout: 10_000 });
    const afterClickGeometry = await page.locator('.taf-data-quality-tabs button').evaluateAll((buttons) => buttons.map((button) => {
      const box = button.getBoundingClientRect();
      return {
        slug: button.getAttribute('data-tab-slug'),
        x: Number(box.x.toFixed(3)),
        y: Number(box.y.toFixed(3)),
        width: Number(box.width.toFixed(3)),
        height: Number(box.height.toFixed(3)),
      };
    }));
    fieldQualityClickGeometryDelta = Math.max(...afterClickGeometry.slice(0, 3).flatMap((box, index) => {
      const baseline = tabGeometry[index];
      return ['x', 'y', 'width', 'height'].map((key) => Math.abs(box[key] - baseline[key]));
    }));
  }
  tabResults.push({
    slug: tab.slug,
    label: tab.label,
    active: true,
    chart_canvas_count: chartCanvasCount,
    action_drawer_visible: drawerVisible,
    endpoint_visible: endpointVisible,
    audit_event_visible: auditEventVisible,
    tab_geometry: tabGeometry,
    field_quality_click_first_three_max_delta: fieldQualityClickGeometryDelta,
    screenshot: path.relative(root, screenshotPath),
  });
}

const mergedBadResponses = [...(previous.bad_responses ?? []), ...badResponses];
const mergedConsoleErrors = [...(previous.console_errors ?? []), ...consoleErrors];
const mergedPageErrors = [...(previous.page_errors ?? []), ...pageErrors];
const mergedRequestFailures = [...(previous.request_failures ?? []), ...requestFailures];
const allTabsPassed = tabs.every((expected) => tabResults.some((tab) => (
  tab.slug === expected.slug && tab.active && tab.chart_canvas_count >= 1 && tab.action_drawer_visible && tab.endpoint_visible && tab.audit_event_visible
)));
const geometryBaseline = tabResults.find((tab) => tab.slug === 'overview')?.tab_geometry ?? tabResults[0]?.tab_geometry ?? [];
const maxTabGeometryDelta = tabResults.reduce((maxDelta, tab) => Math.max(maxDelta, ...(tab.tab_geometry ?? []).flatMap((box, index) => {
  const baseline = geometryBaseline[index];
  if (!baseline || baseline.slug !== box.slug) return [Number.POSITIVE_INFINITY];
  return ['x', 'y', 'width', 'height'].map((key) => Math.abs(box[key] - baseline[key]));
})), 0);
const allTabsGeometryStable = tabResults.length === tabs.length && maxTabGeometryDelta <= 0.01;
const fieldQualityClickFirstThreeMaxDelta = tabResults.find((tab) => tab.slug === 'overview')?.field_quality_click_first_three_max_delta;
const fieldQualityClickGeometryStable = typeof fieldQualityClickFirstThreeMaxDelta === 'number' && fieldQualityClickFirstThreeMaxDelta <= 0.01;
const selectedTabsPassed = selectedTabs.every((expected) => tabResults.some((tab) => (
  tab.slug === expected.slug && tab.active && tab.chart_canvas_count >= 1 && tab.action_drawer_visible && tab.endpoint_visible && tab.audit_event_visible
)));
const currentRunPassed = selectedTabsPassed
  && badResponses.length === 0
  && consoleErrors.length === 0
  && pageErrors.length === 0
  && requestFailures.length === 0;
const result = {
  result: allTabsPassed
    && allTabsGeometryStable
    && fieldQualityClickGeometryStable
    && mergedBadResponses.length === 0
    && mergedConsoleErrors.length === 0
    && mergedPageErrors.length === 0
    && mergedRequestFailures.length === 0 ? 'pass' : 'partial',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  all_tabs_geometry_stable: allTabsGeometryStable,
  max_tab_geometry_delta: Number.isFinite(maxTabGeometryDelta) ? maxTabGeometryDelta : null,
  field_quality_click_geometry_stable: fieldQualityClickGeometryStable,
  field_quality_click_first_three_max_delta: fieldQualityClickFirstThreeMaxDelta ?? null,
  tab_results: tabResults.sort((left, right) => tabs.findIndex((tab) => tab.slug === left.slug) - tabs.findIndex((tab) => tab.slug === right.slug)),
  bad_responses: mergedBadResponses,
  console_errors: mergedConsoleErrors,
  page_errors: mergedPageErrors,
  request_failures: mergedRequestFailures,
  timestamp: new Date().toISOString(),
};

fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(currentRunPassed ? 0 : 1);

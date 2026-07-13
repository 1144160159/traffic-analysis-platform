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
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/data-quality-field-quality/interaction-r236.json');

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
    permissions: ['*', 'admin:*', 'screen:view'],
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

function dataQualityRequest(url) {
  return url.includes('/api/v1/data-quality') || url.includes('/v1/data-quality');
}

function sameRect(left, right) {
  return Boolean(left && right) && ['x', 'y', 'width', 'height'].every((key) => Math.abs(left[key] - right[key]) < 2);
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
  if (dataQualityRequest(request.url())) traffic.push({ at: Date.now(), method: request.method(), url: redact(request.url()) });
});
page.on('response', (response) => {
  if (dataQualityRequest(response.url()) && response.status() >= 400) {
    badResponses.push({ method: response.request().method(), status: response.status(), url: redact(response.url()) });
  }
});
page.on('console', (entry) => {
  if (entry.type() === 'error') consoleErrors.push(entry.text());
});
page.on('pageerror', (error) => pageErrors.push(error.message));

const routeUrl = new URL(`/data-quality?tab=field-quality&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-data-quality-shell.is-field-quality-tab').waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-data-quality-field-trend-chart canvas').waitFor({ state: 'visible', timeout: 15_000 });

const initialSampleText = await page.locator('.taf-data-quality-field-sample-table .taf-data-quality-field-table-row').first().textContent();
await page.locator('.taf-data-quality-field-sample-table .taf-data-quality-field-table-row').first().click();
await page.locator('.taf-data-quality-field-detail-drawer').waitFor({ state: 'visible', timeout: 5_000 });
const sampleDetailVisible = await page.locator('.taf-data-quality-field-detail-drawer').isVisible();
const sampleDetailTitle = await page.locator('.taf-data-quality-field-detail-drawer .ant-drawer-title').textContent();
await page.locator('.taf-data-quality-field-detail-drawer .ant-drawer-close').click();
await page.locator('.taf-data-quality-field-detail-drawer').waitFor({ state: 'hidden', timeout: 5_000 });

await page.getByRole('button', { name: '异常样本下一页' }).click();
const samplePageTwo = await page.locator('.taf-data-quality-field-sample-table button[aria-current="page"]').textContent();
const pagedSampleText = await page.locator('.taf-data-quality-field-sample-table .taf-data-quality-field-table-row').first().textContent();

await page.locator('.taf-data-quality-field-repair-table .taf-data-quality-field-table-row').first().click();
await page.locator('.taf-data-quality-field-detail-drawer').waitFor({ state: 'visible', timeout: 5_000 });
const repairDetailVisible = await page.locator('.taf-data-quality-field-detail-drawer').isVisible();
const repairDetailTitle = await page.locator('.taf-data-quality-field-detail-drawer .ant-drawer-title').textContent();
await page.locator('.taf-data-quality-field-detail-drawer .ant-drawer-close').click();

await page.getByRole('button', { name: '创建字段修复任务' }).click();
await page.locator('.taf-data-quality-field-detail-drawer').waitFor({ state: 'visible', timeout: 5_000 });
const railActionVisible = await page.getByRole('button', { name: '提交操作' }).isVisible();
await page.getByRole('button', { name: '提交操作' }).click();
await page.locator('.taf-data-quality-field-detail-drawer').waitFor({ state: 'hidden', timeout: 5_000 });

const tableScrollSupport = await page.evaluate(() => Array.from(document.querySelectorAll('.taf-data-quality-field-table-rows')).every((element) => {
  const styles = window.getComputedStyle(element);
  return styles.overflowY === 'auto' && styles.scrollbarGutter === 'stable';
}));

const initialApiRequests = traffic.filter((item) => item.method === 'GET');
const rangeStart = Date.now();
const rangeResponse = page.waitForResponse(
  (response) => dataQualityRequest(response.url()) && new URL(response.url()).searchParams.get('time_range') === '近 7 天',
  { timeout: 15_000 },
);
await page.locator('.taf-data-quality-filterbar .ant-select').click();
await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option').filter({ hasText: '近 7 天' }).click();
const rangeStatus = (await rangeResponse).status();
const rangeRequests = traffic.filter((item) => item.at >= rangeStart && item.method === 'GET');

const automaticRefreshStart = Date.now();
const automaticRefreshResponse = page.waitForResponse(
  (response) => dataQualityRequest(response.url()) && response.request().method() === 'GET' && Date.now() - automaticRefreshStart >= 25_000,
  { timeout: 38_000 },
);
const automaticRefreshStatus = (await automaticRefreshResponse).status();
const automaticRefreshRequests = traffic.filter((item) => item.at >= automaticRefreshStart && item.method === 'GET');

const initialAutoRefresh = await page.locator('.taf-data-quality-auto-toggle').getAttribute('aria-pressed');
await page.locator('.taf-data-quality-auto-toggle').click();
const disabledAutoRefresh = await page.locator('.taf-data-quality-auto-toggle').getAttribute('aria-pressed');
const disabledRefreshStart = Date.now();
await page.waitForTimeout(31_500);
const disabledRefreshRequests = traffic.filter((item) => item.at >= disabledRefreshStart && item.method === 'GET');
await page.locator('.taf-data-quality-auto-toggle').click();
const reenabledAutoRefresh = await page.locator('.taf-data-quality-auto-toggle').getAttribute('aria-pressed');

const refreshStart = Date.now();
const refreshResponse = page.waitForResponse(
  (response) => dataQualityRequest(response.url()) && response.request().method() === 'GET',
  { timeout: 15_000 },
);
await page.locator('.taf-data-quality-filterbar .ant-btn').click();
const refreshStatus = (await refreshResponse).status();
const refreshRequests = traffic.filter((item) => item.at >= refreshStart && item.method === 'GET');

const tabRects = async () => page.evaluate(() => {
  const rect = (selector) => {
    const value = document.querySelector(selector)?.getBoundingClientRect();
    return value ? { x: value.x, y: value.y, width: value.width, height: value.height } : null;
  };
  return {
    tabTrack: rect('.taf-data-quality-tabs'),
    titlebar: rect('.taf-data-quality-titlebar'),
    business: rect('.taf-data-quality'),
    viewport: { width: window.innerWidth, height: window.innerHeight },
    horizontalOverflow: document.documentElement.scrollWidth > window.innerWidth,
    verticalOverflow: document.documentElement.scrollHeight > window.innerHeight,
    tabSlots: Array.from(document.querySelectorAll('.taf-data-quality-tabs button')).map((button) => ({
      slot: button.getAttribute('data-tab-slot'),
      slug: button.getAttribute('data-tab-slug'),
      width: button.getBoundingClientRect().width,
    })),
  };
});

const fieldGeometry = await tabRects();
await page.locator('.taf-data-quality-tabs button').filter({ hasText: 'Flink 质量' }).click();
await page.locator('.taf-data-quality-shell.is-flink-quality-tab').waitFor({ state: 'visible', timeout: 5_000 });
const flinkGeometry = await tabRects();
await page.locator('.taf-data-quality-tabs button').filter({ hasText: '字段质量' }).click();
await page.locator('.taf-data-quality-field-trend-chart canvas').waitFor({ state: 'visible', timeout: 5_000 });
const returnedGeometry = await tabRects();
const activeFieldTab = await page.locator('.taf-data-quality-tabs button.is-active').getAttribute('data-tab-slug');
const echartCanvasCount = await page.locator('.taf-data-quality-field-trend-chart canvas').count();
const fieldKpiEchartCanvasCount = await page.locator('.taf-data-quality-field-kpi-echart canvas').count();

await page.locator('.taf-data-quality-tabs button').filter({ hasText: 'Topic 健康' }).click();
await page.locator('.taf-data-quality-latency-echart canvas').waitFor({ state: 'visible', timeout: 8_000 });
await page.getByRole('button', { name: 'Topic 健康下一页' }).click();
const topicPageTwo = await page.locator('.taf-data-quality-topic-health-footer button[aria-current="page"]').textContent();
await page.getByRole('button', { name: '查看全部告警（3）' }).click();
await page.locator('.taf-data-quality-field-detail-drawer').waitFor({ state: 'visible', timeout: 5_000 });
const topicActionDrawerVisible = await page.locator('.taf-data-quality-field-detail-drawer').isVisible();
await page.locator('.taf-data-quality-field-detail-drawer .ant-drawer-close').click();

await page.locator('.taf-data-quality-tabs button').filter({ hasText: 'Flink 质量' }).click();
await page.locator('.taf-data-quality-flink-checkpoint-echart canvas').waitFor({ state: 'visible', timeout: 8_000 });
await page.getByRole('button', { name: 'Flink 作业下一页' }).click();
const flinkPageTwo = await page.locator('.taf-data-quality-flink-job-footer button[aria-current="page"]').textContent();

await page.locator('.taf-data-quality-tabs button').filter({ hasText: '存储质量' }).click();
await page.locator('.taf-data-quality-storage-echart canvas').first().waitFor({ state: 'visible', timeout: 8_000 });
const storageEchartCanvasCount = await page.locator('.taf-data-quality-storage-echart canvas').count();

await page.locator('.taf-data-quality-tabs button').filter({ hasText: '重放对账' }).click();
await page.locator('.taf-data-quality-replay-echart canvas').waitFor({ state: 'visible', timeout: 8_000 });
const replayEchartCanvasCount = await page.locator('.taf-data-quality-replay-echart canvas').count();

await page.locator('.taf-data-quality-tabs button').filter({ hasText: '字段质量' }).click();
await page.locator('.taf-data-quality-field-trend-chart canvas').waitFor({ state: 'visible', timeout: 8_000 });
await page.screenshot({ path: path.join(root, 'evidence/ui-image-breakdowns/pages/data-quality-field-quality/interaction-r236.png'), fullPage: false });

const fixedTabTrack = sameRect(fieldGeometry.tabTrack, flinkGeometry.tabTrack) && sameRect(fieldGeometry.tabTrack, returnedGeometry.tabTrack);
const fixedTitlebar = sameRect(fieldGeometry.titlebar, flinkGeometry.titlebar) && sameRect(fieldGeometry.titlebar, returnedGeometry.titlebar);
const equalTabSlots = fieldGeometry.tabSlots.length === 8 && fieldGeometry.tabSlots.every((item) => Math.abs(item.width - fieldGeometry.tabSlots[0].width) < 1);
const result = {
  result: initialApiRequests.length >= 1
    && rangeRequests.some((item) => new URL(item.url).searchParams.get('time_range') === '近 7 天')
    && rangeStatus < 400
    && automaticRefreshRequests.length >= 1
    && automaticRefreshStatus < 400
    && disabledRefreshRequests.length === 0
    && refreshRequests.length >= 1
    && refreshStatus < 400
    && initialAutoRefresh === 'true'
    && disabledAutoRefresh === 'false'
    && reenabledAutoRefresh === 'true'
    && activeFieldTab === 'field-quality'
    && echartCanvasCount === 1
    && fieldKpiEchartCanvasCount === 6
    && sampleDetailVisible
    && sampleDetailTitle?.includes('字段异常详情')
    && samplePageTwo === '2'
    && Boolean(pagedSampleText && pagedSampleText !== initialSampleText)
    && repairDetailVisible
    && repairDetailTitle?.includes('字段修复任务')
    && railActionVisible
    && tableScrollSupport
    && topicPageTwo === '2'
    && topicActionDrawerVisible
    && flinkPageTwo === '2'
    && storageEchartCanvasCount >= 2
    && replayEchartCanvasCount === 1
    && fixedTabTrack
    && fixedTitlebar
    && equalTabSlots
    && !fieldGeometry.horizontalOverflow
    && !fieldGeometry.verticalOverflow
    && badResponses.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0 ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  route: redact(routeUrl.toString()),
  timestamp: new Date().toISOString(),
  api: {
    initial_get_count: initialApiRequests.length,
    range_get_count: rangeRequests.length,
    range_status: rangeStatus,
    range_requests: rangeRequests,
    automatic_refresh_get_count: automaticRefreshRequests.length,
    automatic_refresh_status: automaticRefreshStatus,
    automatic_refresh_requests: automaticRefreshRequests,
    disabled_refresh_get_count: disabledRefreshRequests.length,
    refresh_get_count: refreshRequests.length,
    refresh_status: refreshStatus,
    requests: traffic,
  },
  auto_refresh: {
    initial: initialAutoRefresh,
    disabled: disabledAutoRefresh,
    reenabled: reenabledAutoRefresh,
    enabled_request_observed: automaticRefreshRequests.length >= 1,
    disabled_request_suppressed: disabledRefreshRequests.length === 0,
  },
  echart_canvas_count: echartCanvasCount,
  field_kpi_echart_canvas_count: fieldKpiEchartCanvasCount,
  field_interactions: {
    sample_detail_visible: sampleDetailVisible,
    sample_detail_title: sampleDetailTitle,
    sample_page: samplePageTwo,
    initial_sample_text: initialSampleText,
    paged_sample_text: pagedSampleText,
    repair_detail_visible: repairDetailVisible,
    repair_detail_title: repairDetailTitle,
    rail_action_visible: railActionVisible,
    table_scroll_support: tableScrollSupport,
    topic_page: topicPageTwo,
    topic_action_drawer_visible: topicActionDrawerVisible,
    flink_page: flinkPageTwo,
    storage_echart_canvas_count: storageEchartCanvasCount,
    replay_echart_canvas_count: replayEchartCanvasCount,
  },
  active_tab: activeFieldTab,
  geometry: {
    field: fieldGeometry,
    flink: flinkGeometry,
    returned_field: returnedGeometry,
    fixed_tab_track: fixedTabTrack,
    fixed_titlebar: fixedTitlebar,
    equal_tab_slots: equalTabSlots,
  },
  bad_responses: badResponses,
  console_errors: consoleErrors,
  page_errors: pageErrors,
};

fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
await page.close();
await browser.close();
console.log(JSON.stringify(result, null, 2));
if (result.result !== 'pass') process.exit(1);

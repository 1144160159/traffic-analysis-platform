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
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/alerts/interaction-r651.json');
const screenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/alerts/interaction-r651.png');
const compactScreenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/alerts/interaction-r651-1600x900.png');

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
page.on('console', (entry) => { if (entry.type() === 'error' && !entry.text().includes('net::ERR_CONNECTION_CLOSED')) consoleErrors.push(entry.text()); });
page.on('pageerror', (error) => { if (error.message !== 'Object') pageErrors.push(error.message); });
page.on('requestfailed', (request) => { if (new URL(request.url()).origin === new URL(baseUrl).origin) requestFailures.push(`${request.method()} ${redact(request.url())} ${request.failure()?.errorText ?? ''}`); });

const routeUrl = new URL(`/alerts?windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-alert-triage').waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-alert-risk-echart canvas').waitFor({ state: 'visible', timeout: 10_000 });
await page.locator('.taf-alert-table tbody tr.ant-table-row').first().waitFor({ state: 'visible', timeout: 15_000 });
const gaugeCanvasCount = await page.locator('.taf-alert-risk-echart canvas').count();
const simulatedRowCount = await page.locator('.taf-alert-table tbody tr.ant-table-row', { hasText: '-SIM' }).count();
await page.locator('.taf-alert-table-panel .ant-pagination-item-2').click();
const tablePageTwo = await page.locator('.taf-alert-table-panel .ant-pagination-item-active').textContent();
const tableOverflowY = await page.evaluate(() => window.getComputedStyle(document.querySelector('.taf-alert-table-panel .ant-table-body')).overflowY);

await page.locator('.taf-alert-table-panel tbody .ant-table-selection-column input').first().check();
const selectedCount = await page.locator('.taf-alert-table-panel .ant-table-row-selected').count();
await page.waitForFunction(() => Array.from(document.querySelectorAll('button')).some((button) => button.textContent?.includes('批量指派') && !button.disabled));
const batchAssignEnabled = await page.getByRole('button', { name: '批量指派' }).isEnabled();
const batchAssignResponsePromise = page.waitForResponse((response) => response.url().endsWith('/assign') && response.request().method() === 'PUT');
await page.getByRole('button', { name: '批量指派' }).click();
const batchAssignResponse = await batchAssignResponsePromise;
const batchAssignStatus = batchAssignResponse.status();
const batchAssignBody = await batchAssignResponse.json().catch(() => ({}));
const batchAssignData = batchAssignBody?.data ?? batchAssignBody;
const batchAssignVerified = batchAssignStatus === 200 && batchAssignData?.assignee === 'security-analyst' && Boolean(batchAssignData?.alert_id);
await page.reload({ waitUntil: 'domcontentloaded' });
await page.locator('.taf-alert-table tbody tr.ant-table-row').first().waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-alert-table-panel tbody .ant-table-selection-column input').first().check();
await page.waitForFunction(() => Array.from(document.querySelectorAll('button')).some((button) => button.textContent?.includes('批量状态变更') && !button.disabled));
const batchStatusResponsePromise = page.waitForResponse((response) => response.url().endsWith('/alerts/batch/status') && response.request().method() === 'PUT');
await page.getByRole('button', { name: '批量状态变更' }).click();
const batchStatusResponse = await batchStatusResponsePromise;
const batchStatusCode = batchStatusResponse.status();
const batchStatusBody = await batchStatusResponse.json().catch(() => ({}));
const batchStatusData = batchStatusBody?.data ?? batchStatusBody;
const batchStatusVerified = batchStatusCode === 200 && Number(batchStatusData?.success_count) === 1 && Number(batchStatusData?.failed_count) === 0 && Array.isArray(batchStatusData?.success_ids) && batchStatusData.success_ids.length === 1 && Boolean(batchStatusData.success_ids[0]);
await page.waitForTimeout(800);

const drawer = page.locator('.ant-drawer-content-wrapper:visible');
await page.getByRole('button', { name: '保存视图' }).click();
await drawer.waitFor({ state: 'visible', timeout: 5_000 });
await drawer.getByRole('button', { name: '确认提交' }).click();
await drawer.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 5_000 });
const viewActionVisible = await drawer.locator('.ant-alert-success').isVisible();
await drawer.locator('.ant-drawer-close').click();
await drawer.waitFor({ state: 'hidden', timeout: 5_000 });

await page.locator('.taf-alert-row-actions .ant-btn').nth(1).click();
await drawer.waitFor({ state: 'visible', timeout: 5_000 });
const rowActionVisible = await drawer.isVisible();
const rowActionResponsePromise = page.waitForResponse((response) => response.url().includes('/response-actions') && response.request().method() === 'POST');
await drawer.getByRole('button', { name: '确认提交' }).click();
const rowActionStatus = (await rowActionResponsePromise).status();
await drawer.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 5_000 });
await drawer.locator('.ant-drawer-close').click();
await drawer.waitFor({ state: 'hidden', timeout: 5_000 });

const feedbackResponse = page.waitForResponse((response) => response.url().includes('/api/v1/alerts/') && response.url().endsWith('/feedback') && response.request().method() === 'POST');
await page.getByRole('button', { name: '提交反馈' }).click();
const feedbackStatus = (await feedbackResponse).status();
await page.waitForTimeout(800);

const clusterResponsePromise = page.waitForResponse((response) => response.url().includes('/api/v1/alerts?') && response.url().includes('src_ip=') && response.request().method() === 'GET');
await page.getByRole('button', { name: /同源 IP/ }).click();
const clusterResponse = await clusterResponsePromise;
const clusterActionStatus = clusterResponse.status();
const clusterBody = await clusterResponse.json().catch(() => ({}));
const clusterRows = Array.isArray(clusterBody?.data) ? clusterBody.data : [];
const clusterSource = new URL(clusterResponse.url()).searchParams.get('src_ip');
const clusterVerified = clusterActionStatus === 200 && Boolean(clusterSource) && clusterRows.length > 0 && clusterRows.every((row) => row.src_ip === clusterSource);
const resetResponsePromise = page.waitForResponse((response) => response.url().includes('/api/v1/alerts?') && !response.url().includes('src_ip=') && response.request().method() === 'GET');
await page.getByRole('button', { name: /重\s*置/ }).click();
await resetResponsePromise;

await page.waitForTimeout(4500);
await page.evaluate(() => { const body = document.querySelector('.taf-alert-table-panel .ant-table-body'); if (body) body.scrollLeft = 0; });

await page.screenshot({ path: screenshotPath, fullPage: false });
const desktopLayout = await page.evaluate(() => ({ viewportWidth: innerWidth, bodyScrollWidth: document.body.scrollWidth, documentScrollWidth: document.documentElement.scrollWidth, pageHeight: document.querySelector('.taf-alert-triage')?.getBoundingClientRect().height ?? 0 }));
await page.setViewportSize({ width: 1600, height: 900 });
await page.waitForTimeout(500);
await page.keyboard.press('Escape').catch(() => {});
await page.mouse.move(2, 2);
await page.screenshot({ path: compactScreenshotPath, fullPage: false });
const compactLayout = await page.evaluate(() => {
  const rect = (selector) => document.querySelector(selector)?.getBoundingClientRect();
  const tableBody = rect('.taf-alert-table-panel .ant-table-body');
  const pagination = rect('.taf-alert-table-panel .ant-pagination');
  const detail = document.querySelector('.taf-alert-detail');
  const feedbackButton = document.querySelector('.taf-alert-feedback__footer .ant-btn-primary');
  return { viewportWidth: innerWidth, bodyScrollWidth: document.body.scrollWidth, documentScrollWidth: document.documentElement.scrollWidth, pageHeight: rect('.taf-alert-triage')?.height ?? 0, mainWidth: rect('.taf-alert-main')?.width ?? 0, detailWidth: detail?.getBoundingClientRect().width ?? 0, tableBodyBottom: tableBody?.bottom ?? 0, paginationTop: pagination?.top ?? 0, tablePaginationSeparated: Boolean(tableBody && pagination && tableBody.bottom <= pagination.top + 1), detailScrollable: Boolean(detail && detail.scrollHeight > detail.clientHeight), feedbackInitiallyVisible: Boolean(feedbackButton && feedbackButton.getBoundingClientRect().bottom <= innerHeight) };
});
const feedbackReachable = await page.evaluate(() => {
  const detail = document.querySelector('.taf-alert-detail');
  const button = document.querySelector('.taf-alert-feedback__footer .ant-btn-primary');
  if (!detail || !button) return false;
  detail.scrollTop = detail.scrollHeight;
  const buttonRect = button.getBoundingClientRect();
  const detailRect = detail.getBoundingClientRect();
  return buttonRect.top >= detailRect.top && buttonRect.bottom <= Math.min(detailRect.bottom, innerHeight);
});
const noViewportOverflow = [desktopLayout, compactLayout].every((layout) => Math.max(layout.bodyScrollWidth, layout.documentScrollWidth) <= layout.viewportWidth + 2);
const result = { result: gaugeCanvasCount === 1 && simulatedRowCount === 0 && tablePageTwo === '2' && ['auto', 'scroll'].includes(tableOverflowY) && viewActionVisible && rowActionVisible && rowActionStatus === 201 && clusterVerified && feedbackStatus === 201 && selectedCount === 1 && batchAssignEnabled && batchAssignVerified && batchStatusVerified && noViewportOverflow && compactLayout.tablePaginationSeparated && compactLayout.detailScrollable && feedbackReachable && badResponses.length === 0 && consoleErrors.length === 0 && pageErrors.length === 0 && requestFailures.length === 0 ? 'pass' : 'fail', browser_backend: 'Windows Chrome CDP over Xshell tunnel', browser: version.Browser, route: redact(routeUrl.toString()), data_mode: 'live', gauge_canvas_count: gaugeCanvasCount, simulated_row_count: simulatedRowCount, table_page_two: tablePageTwo, table_overflow_y: tableOverflowY, view_action_visible: viewActionVisible, row_action_visible: rowActionVisible, row_action_status: rowActionStatus, cluster_action_status: clusterActionStatus, cluster_source: clusterSource, cluster_rows: clusterRows.length, cluster_verified: clusterVerified, feedback_status: feedbackStatus, selected_count: selectedCount, batch_assign_enabled: batchAssignEnabled, batch_assign_status: batchAssignStatus, batch_assign_verified: batchAssignVerified, batch_status_code: batchStatusCode, batch_status_verified: batchStatusVerified, compact_feedback_reachable: feedbackReachable, no_viewport_overflow: noViewportOverflow, desktop_layout: desktopLayout, compact_layout: compactLayout, bad_responses: badResponses, console_errors: consoleErrors, page_errors: pageErrors, request_failures: requestFailures, screenshot: path.relative(root, screenshotPath), compact_screenshot: path.relative(root, compactScreenshotPath), timestamp: new Date().toISOString() };
fs.mkdirSync(path.dirname(outputPath), { recursive: true }); fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`); console.log(JSON.stringify(result, null, 2)); await page.close().catch(() => {}); process.exit(result.result === 'pass' ? 0 : 1);

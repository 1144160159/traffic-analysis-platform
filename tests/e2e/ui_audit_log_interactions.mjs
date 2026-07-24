#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const baseUrl = process.env.AUDIT_UI_BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.AUDIT_UI_CDP_URL || 'http://127.0.0.1:9224';
const runId = process.env.AUDIT_UI_RUN_ID || 'r373';
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/audit-log');
const outputPath = path.join(evidenceDir, `interaction-${runId}.json`);
const screenshots = {
  main: path.join(evidenceDir, `actual-${runId}-main-1920.png`),
  operationContext: path.join(evidenceDir, `actual-${runId}-operation-context-1920.png`),
  relatedChain: path.join(evidenceDir, `actual-${runId}-related-chain-1920.png`),
  inlineDetail: path.join(evidenceDir, `actual-${runId}-inline-detail-1920.png`),
  inlineReview: path.join(evidenceDir, `actual-${runId}-inline-review-1920.png`),
  exportModal: path.join(evidenceDir, `actual-${runId}-export-modal-1920.png`),
};

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
fs.mkdirSync(evidenceDir, { recursive: true });

const redact = (value) => String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
const sleep = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds));

function smokeToken({ permissions = ['*', 'admin:*', 'audit:read', 'audit:write', 'audit:export'], roles = ['admin'], username = 'codex-windows-cdp-audit-admin' } = {}) {
  const secret = execFileSync('kubectl', [
    '--server=https://127.0.0.1:6443', '--tls-server-name=10.0.5.8',
    '-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}',
  ], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(),
    tenant_id: 'default', username, roles,
    permissions, token_type: 'access',
    session_id: `windows-cdp-audit-${runId}`, iat: now, exp: now + 1800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(secret, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

async function capture(page, key) {
  await page.screenshot({ path: screenshots[key], fullPage: false });
  const size = fs.statSync(screenshots[key]).size;
  if (size < 10_000) throw new Error(`screenshot ${key} is unexpectedly small: ${size}`);
}

async function nativePointerClick(page, locator, name) {
  const box = await locator.boundingBox();
  if (!box) throw new Error(`${name} has no pointer hitbox`);
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  const hit = await locator.evaluate((button, point) => {
    const top = document.elementFromPoint(point.x, point.y);
    return {
      disabled: button.disabled,
      center_hits_button: top === button || button.contains(top),
      top_tag: top?.tagName ?? null,
      top_text: top?.textContent?.trim().slice(0, 32) ?? null,
    };
  }, { x, y });
  if (hit.disabled || !hit.center_hits_button) throw new Error(`${name} pointer center is not actionable: ${JSON.stringify(hit)}`);
  await page.mouse.click(x, y);
  return { name, box, ...hit };
}

async function closeDrawer(page) {
  const drawer = page.locator('.ant-drawer-content-wrapper:visible');
  await drawer.locator('.ant-drawer-close').click();
  await drawer.waitFor({ state: 'hidden' });
}

async function openAndSubmitDrawer(page, buttonName, successText, endpoint) {
  await page.locator('.taf-auditlog-titlebar button').filter({ hasText: buttonName }).click({ force: true });
  const drawer = page.locator('.ant-drawer-content-wrapper:visible');
  await drawer.waitFor({ state: 'visible' });
  const responsePromise = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return response.request().method() === 'POST' && url.pathname.endsWith(endpoint);
  }, { timeout: 45_000 });
  await drawer.getByRole('button', { name: '确认提交', exact: true }).click();
  const response = await responsePromise;
  const responseBody = await response.json().catch(() => null);
  const responseData = responseBody && typeof responseBody === 'object' && 'data' in responseBody
    ? responseBody.data
    : responseBody;
  const success = drawer.getByText(successText, { exact: false });
  const uiFeedbackVisible = await success.waitFor({ state: 'attached', timeout: 7_000 }).then(() => true).catch(() => false);
  if (uiFeedbackVisible) await success.scrollIntoViewIfNeeded();
  const text = await drawer.isVisible().catch(() => false) ? await drawer.textContent() : '';
  if (await drawer.isVisible().catch(() => false)) await closeDrawer(page);
  return {
    endpoint,
    status: response.status(),
    ok: response.ok(),
    response_data: responseData,
    ui_feedback_visible: uiFeedbackVisible,
    text: text || '',
  };
}

const [versionResponse, listResponse] = await Promise.all([
  fetch(`${cdpUrl}/json/version`), fetch(`${cdpUrl}/json/list`),
]);
if (!versionResponse.ok || !listResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}/${listResponse.status}`);
const version = await versionResponse.json();
const initialTargets = await listResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
page.setDefaultTimeout(12_000);

const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
const ignoredExternalFailures = [];
const auditApiResponses = [];
page.on('response', (response) => {
  const url = response.url();
  if (url.includes('/api/v1/audit/')) auditApiResponses.push({ status: response.status(), method: response.request().method(), url: redact(url) });
  if (response.status() >= 400 && (url.startsWith(baseUrl) || url.includes('/api/'))) badResponses.push({ status: response.status(), url: redact(url) });
});
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const item = { text: entry.text(), url: entry.location().url || '' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com/ip') || item.text.includes('ERR_CONNECTION_CLOSED')) ignoredExternalFailures.push(item);
  else consoleErrors.push(item);
});
page.on('pageerror', (error) => {
  if (error.message === 'Object') ignoredExternalFailures.push({ text: error.message, url: 'chrome-extension://isolated-world' });
  else pageErrors.push({ message: error.message, stack: error.stack || '' });
});
page.on('requestfailed', (request) => {
  const item = { method: request.method(), url: redact(request.url()), error: request.failure()?.errorText ?? '' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com/ip')) ignoredExternalFailures.push(item);
  else requestFailures.push(item);
});

const routeUrl = new URL(`/audit-log?__codex_ui_breakdown_production=1&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});
await page.locator('.taf-auditlog').waitFor({ state: 'visible', timeout: 20_000 });
const tableRows = page.locator('.taf-auditlog-table-panel tbody tr.ant-table-row');
await tableRows.first().waitFor({ state: 'visible', timeout: 20_000 });
await page.getByRole('button', { name: '字段变更对比', exact: true }).click();
await page.locator('.taf-auditlog-retention-echart canvas').first().waitFor({ state: 'visible', timeout: 15_000 });

const viewport = page.viewportSize();
const devicePixelRatio = await page.evaluate(() => window.devicePixelRatio);
const title = await page.locator('.taf-auditlog-titlebar h1').textContent();
const rowCount = await tableRows.count();
const totalText = await page.locator('.taf-auditlog-table-panel .taf-work-panel-title, .taf-auditlog-table-panel h2').first().textContent().catch(() => '');
const retentionCanvasCount = await page.locator('.taf-auditlog-retention-echart canvas').count();
let firstRowId = await tableRows.first().getAttribute('data-row-key');
const detailPanel = page.locator('.taf-auditlog-detail-panel');
const detailPanelBoxBefore = await detailPanel.boundingBox();
const tabStartUrl = page.url();
await capture(page, 'main');

let pagination = { exercised: false, firstRowChanged: null, activePage: '1' };
if (await page.getByRole('button', { name: '审计日志第 2 页' }).count()) {
  const secondPageResponse = page.waitForResponse((response) => response.url().includes('/api/v1/audit/logs') && response.url().includes('offset=10') && response.status() === 200, { timeout: 15_000 });
  await page.getByRole('button', { name: '审计日志第 2 页' }).click();
  await secondPageResponse;
  const secondRowId = await tableRows.first().getAttribute('data-row-key');
  pagination = {
    exercised: true,
    firstRowChanged: firstRowId !== secondRowId,
    activePage: await page.locator('.taf-auditlog-pagination button.is-active').textContent(),
  };
  await page.getByRole('button', { name: '审计日志第 1 页' }).click();
  await page.waitForFunction((expected) => document.querySelector('.taf-auditlog-table-panel tbody tr.ant-table-row')?.getAttribute('data-row-key') === expected, firstRowId, { timeout: 15_000 });
}

await page.getByRole('button', { name: '操作上下文', exact: true }).click();
await page.locator('[data-audit-detail-state="operation-context"]').waitFor({ state: 'visible' });
const operationContextUrl = page.url();
const detailPanelBoxOperationContext = await detailPanel.boundingBox();
await capture(page, 'operationContext');

await page.getByRole('button', { name: '关联链路', exact: true }).click();
await page.locator('[data-audit-detail-state="related-chain"]').waitFor({ state: 'visible' });
const relatedChainUrl = page.url();
const detailPanelBoxRelatedChain = await detailPanel.boundingBox();
await capture(page, 'relatedChain');

const sameBox = (left, right) => left && right && ['x', 'y', 'width', 'height'].every((key) => Math.abs(left[key] - right[key]) < 1);
const tabNavigationContained = operationContextUrl === tabStartUrl && relatedChainUrl === tabStartUrl
  && sameBox(detailPanelBoxBefore, detailPanelBoxOperationContext)
  && sameBox(detailPanelBoxBefore, detailPanelBoxRelatedChain);

await page.getByRole('button', { name: '字段变更对比', exact: true }).click();
const firstRowActions = tableRows.first().locator('.taf-auditlog-row-actions');
const rowActionCounts = {
  detail: await firstRowActions.getByRole('button', { name: /查看审计详情/ }).count(),
  relation: await firstRowActions.getByRole('button', { name: /查看关联链路/ }).count(),
  review: await firstRowActions.getByRole('button', { name: /复核审计记录/ }).count(),
};
const rowActionLayout = await firstRowActions.evaluate((element) => {
  const container = element.getBoundingClientRect();
  const buttons = [...element.querySelectorAll('button')].map((button) => {
    const box = button.getBoundingClientRect();
    return {
      text: button.textContent,
      width: box.width,
      height: box.height,
      disabled: button.disabled,
      center_hits_button: document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2) === button,
      fully_inside: box.left >= container.left - 1 && box.right <= container.right + 1,
    };
  });
  return {
    client_width: element.clientWidth,
    scroll_width: element.scrollWidth,
    clipped: element.scrollWidth > element.clientWidth + 1,
    buttons,
  };
});
const nativePointerActions = [];
let releaseBackgroundRefetch;
let markBackgroundRefetchStarted;
const backgroundRefetchStarted = new Promise((resolve) => { markBackgroundRefetchStarted = resolve; });
const backgroundRefetchRelease = new Promise((resolve) => { releaseBackgroundRefetch = resolve; });
let delayNextListRequest = true;
const delayedListHandler = async (route) => {
  const url = new URL(route.request().url());
  if (delayNextListRequest && url.pathname.endsWith('/api/v1/audit/logs')) {
    delayNextListRequest = false;
    markBackgroundRefetchStarted();
    await backgroundRefetchRelease;
  }
  await route.continue();
};
await page.route('**/api/v1/audit/logs**', delayedListHandler);
const refetchFinished = page.waitForResponse((response) => new URL(response.url()).pathname.endsWith('/api/v1/audit/logs') && response.status() === 200, { timeout: 15_000 });
await page.locator('.taf-auditlog-titlebar button:has(.anticon-reload)').click();
await backgroundRefetchStarted;
const detailButtonDuringRefetch = firstRowActions.getByRole('button', { name: /查看审计详情/ });
const disabledDuringBackgroundRefetch = await detailButtonDuringRefetch.isDisabled();
nativePointerActions.push(await nativePointerClick(page, detailButtonDuringRefetch, '详情'));
await page.locator('.taf-auditlog-detail-tabs button.is-active', { hasText: '操作详情' }).waitFor({ state: 'visible' });
const backgroundRefetchActionable = {
  list_request_started: true,
  detail_button_disabled: disabledDuringBackgroundRefetch,
  active_tab_after_native_click: (await page.locator('.taf-auditlog-detail-tabs button.is-active').textContent())?.trim(),
  row_id_before_refetch: firstRowId,
};
releaseBackgroundRefetch();
await refetchFinished;
await page.unroute('**/api/v1/audit/logs**', delayedListHandler);
firstRowId = await tableRows.first().getAttribute('data-row-key');
backgroundRefetchActionable.row_id_after_refetch = firstRowId;
const inlineDetail = page.locator('[data-overlay-contract="drawer-audit-operation-detail"]');
await inlineDetail.waitFor({ state: 'visible', timeout: 15_000 });
await page.waitForFunction((expected) => document.querySelector('[data-overlay-contract="drawer-audit-operation-detail"]')?.textContent?.includes(expected), firstRowId, { timeout: 15_000 });
const detailHasRealId = Boolean(firstRowId) && ((await inlineDetail.textContent())?.includes(String(firstRowId).trim()) ?? false);
const detailPanelBox = await detailPanel.boundingBox();
const detailPanelLayout = await detailPanel.locator('.taf-panel__body').evaluate((body) => {
  const content = body.querySelector('[data-overlay-contract="drawer-audit-operation-detail"]');
  const bodyBox = body.getBoundingClientRect();
  const contentBox = content?.getBoundingClientRect();
  return {
    client_width: body.clientWidth,
    scroll_width: body.scrollWidth,
    horizontal_overflow: body.scrollWidth > body.clientWidth + 2,
    content_inside: Boolean(contentBox) && contentBox.left >= bodyBox.left - 1 && contentBox.right <= bodyBox.right + 1,
  };
});
await capture(page, 'inlineDetail');

const secondRowId = await tableRows.nth(1).getAttribute('data-row-key');
await tableRows.nth(1).click();
await page.waitForFunction((expected) => document.querySelector('.taf-auditlog-table-panel tbody tr.ant-table-row-selected')?.getAttribute('data-row-key') === expected, secondRowId, { timeout: 5_000 });
nativePointerActions.push(await nativePointerClick(page, tableRows.first().locator('.taf-auditlog-row-actions').getByRole('button', { name: /查看关联链路/ }), '关联'));
await page.locator('.taf-auditlog-detail-tabs button.is-active', { hasText: '关联链路' }).waitFor({ state: 'visible' });
const selectedRowKeyAfterRelation = await page.locator('.taf-auditlog-table-panel tbody tr.ant-table-row-selected').getAttribute('data-row-key');
const rowAssociationWorked = Boolean(secondRowId) && secondRowId !== firstRowId && selectedRowKeyAfterRelation === firstRowId;

nativePointerActions.push(await nativePointerClick(page, tableRows.first().locator('.taf-auditlog-row-actions').getByRole('button', { name: /复核审计记录/ }), '复核'));
const inlineReview = page.locator('[data-audit-detail-state="review"]');
await inlineReview.waitFor({ state: 'visible' });
const rowReviewTargeted = Boolean(firstRowId) && ((await inlineReview.textContent())?.includes(String(firstRowId).trim()) ?? false);
await tableRows.nth(1).click();
await page.waitForFunction((expected) => document.querySelector('.taf-auditlog-table-panel tbody tr.ant-table-row-selected')?.getAttribute('data-row-key') === expected, secondRowId, { timeout: 5_000 });
const rowReviewRetargeted = Boolean(secondRowId) && ((await inlineReview.textContent())?.includes(String(secondRowId).trim()) ?? false);
const reviewPanelBox = await detailPanel.boundingBox();
const reviewLayout = await detailPanel.locator('.taf-panel__body').evaluate((body) => {
  const content = body.querySelector('[data-audit-detail-state="review"]');
  const bodyBox = body.getBoundingClientRect();
  const contentBox = content?.getBoundingClientRect();
  return {
    content_inside: Boolean(contentBox) && contentBox.left >= bodyBox.left - 1 && contentBox.right <= bodyBox.right + 1 && contentBox.top >= bodyBox.top - 1 && contentBox.bottom <= body.scrollHeight + bodyBox.top + 2,
    horizontal_overflow: body.scrollWidth > body.clientWidth + 2,
    visible_drawers: document.querySelectorAll('.ant-drawer-content-wrapper:not([style*="display: none"])').length,
    visible_modals: document.querySelectorAll('.ant-modal-content').length ? [...document.querySelectorAll('.ant-modal-content')].filter((element) => element.getBoundingClientRect().width > 0 && getComputedStyle(element).visibility !== 'hidden').length : 0,
  };
});
const reviewResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname.endsWith('/api/v1/audit/reviews'), { timeout: 30_000 });
await inlineReview.getByRole('button', { name: '确认提交复核', exact: true }).click();
const reviewResponse = await reviewResponsePromise;
const reviewRequestBody = reviewResponse.request().postDataJSON();
const reviewResponseBody = await reviewResponse.json();
const reviewResponseData = reviewResponseBody?.data ?? reviewResponseBody;
const rowReviewRequestMatched = reviewRequestBody?.log_id === secondRowId;
const rowReviewResponseMatched = reviewResponse.ok() && reviewResponseData?.log_id === secondRowId;
const rowReviewEvidence = {
  status: reviewResponse.status(),
  request: { log_id: reviewRequestBody?.log_id, reason: reviewRequestBody?.reason },
  response: { review_id: reviewResponseData?.review_id, log_id: reviewResponseData?.log_id, status: reviewResponseData?.status },
};
const rowReviewSuccess = inlineReview.getByText('真实业务操作完成', { exact: false });
await rowReviewSuccess.waitFor({ state: 'attached', timeout: 30_000 });
const reviewText = await inlineReview.textContent() || '';
await capture(page, 'inlineReview');
const originalFirstRow = page.locator(`.taf-auditlog-table-panel tbody tr.ant-table-row[data-row-key="${firstRowId}"]`);
await originalFirstRow.waitFor({ state: 'visible', timeout: 15_000 });
await originalFirstRow.locator('.taf-auditlog-row-actions').getByRole('button', { name: /查看关联链路/ }).click();
await page.locator('.taf-auditlog-detail-tabs button.is-active', { hasText: '关联链路' }).waitFor({ state: 'visible' });
await page.locator('.taf-auditlog-titlebar button').filter({ hasText: '触发复核' }).click();
await page.locator('.taf-auditlog-detail-tabs button.is-active', { hasText: '复核操作' }).waitFor({ state: 'visible' });
const titleReviewWorked = Boolean(firstRowId) && ((await inlineReview.textContent())?.includes(String(firstRowId).trim()) ?? false);

await page.locator('.taf-auditlog-titlebar button').filter({ hasText: '导出取证' }).click({ force: true });
const exportModal = page.locator('.ant-modal-content:visible');
await exportModal.waitFor({ state: 'visible' });
const modalBox = await exportModal.boundingBox();
const modalLayout = await exportModal.locator('.ant-modal-body').evaluate((body) => ({
  client_width: body.clientWidth,
  scroll_width: body.scrollWidth,
  horizontal_overflow: body.scrollWidth > body.clientWidth + 2,
}));
await capture(page, 'exportModal');
console.log('audit-ui: export modal captured');
await exportModal.getByText('我确认导出范围与脱敏策略', { exact: false }).click();
console.log('audit-ui: export confirmation checked');
const exportResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname.endsWith('/api/v1/audit/exports'), { timeout: 30_000 });
await exportModal.getByRole('button', { name: '生成并下载', exact: true }).click();
console.log('audit-ui: export submitted');
const exportResponse = await exportResponsePromise;
const exportRequestBody = exportResponse.request().postDataJSON();
const exportResponseBody = await exportResponse.json();
const exportResponseData = exportResponseBody?.data ?? exportResponseBody;
const exportArtifactBytes = Buffer.from(exportResponseData?.content_base64 || '', 'base64');
const exportArtifact = exportArtifactBytes.toString('latin1');
const exportArtifactSha256 = crypto.createHash('sha256').update(exportArtifactBytes).digest('hex');
const exportServerSha256 = String(exportResponseData?.sha256 || '').replace(/^sha256:/, '');
const exportSignature = exportArtifactBytes.subarray(0, 8).toString('ascii');
const selectedExportMatched = exportResponse.ok()
  && exportRequestBody?.filters?.log_id === firstRowId
  && exportResponseData?.row_count === 1
  && exportResponseData?.total_matching === 1
  && exportResponseData?.truncated === false
  && exportArtifact.includes(firstRowId)
  && exportArtifactSha256 === exportServerSha256
  && exportSignature.startsWith('%PDF-');
await exportModal.getByText('真实操作已完成', { exact: false }).waitFor({ state: 'visible', timeout: 20_000 });
console.log('audit-ui: export success observed');
const exportText = await exportModal.textContent();
if (await exportModal.isVisible().catch(() => false)) {
  await page.keyboard.press('Escape');
  await exportModal.waitFor({ state: 'hidden', timeout: 5_000 }).catch(async () => {
    await page.locator('.ant-modal-close:visible').click({ force: true });
    await exportModal.waitFor({ state: 'hidden' });
  });
}

const bottomExportButton = page.locator('.taf-auditlog-bottom > .taf-panel:last-child button').filter({ hasText: '导出审计材料' });
await bottomExportButton.scrollIntoViewIfNeeded();
await bottomExportButton.click();
const bottomExportModal = page.locator('.ant-modal-content:visible');
await bottomExportModal.waitFor({ state: 'visible' });
const bottomExportDisplayedID = await bottomExportModal.locator('input').evaluateAll(
  (inputs, expected) => inputs.filter((input) => input.value === expected).length === 1,
  firstRowId,
);
await bottomExportModal.getByText('我确认导出范围与脱敏策略', { exact: false }).click();
const bottomExportResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname.endsWith('/api/v1/audit/exports'), { timeout: 30_000 });
await bottomExportModal.getByRole('button', { name: '生成并下载', exact: true }).click();
const bottomExportResponse = await bottomExportResponsePromise;
const bottomExportRequestBody = bottomExportResponse.request().postDataJSON();
const bottomExportResponseBody = await bottomExportResponse.json();
const bottomExportResponseData = bottomExportResponseBody?.data ?? bottomExportResponseBody;
const bottomExportArtifact = Buffer.from(bottomExportResponseData?.content_base64 || '', 'base64').toString('latin1');
const bottomExportMatched = bottomExportResponse.ok()
  && bottomExportDisplayedID
  && bottomExportRequestBody?.filters?.log_id === firstRowId
  && bottomExportResponseData?.row_count === 1
  && bottomExportResponseData?.total_matching === 1
  && bottomExportResponseData?.truncated === false
  && bottomExportArtifact.includes(firstRowId);
await bottomExportModal.getByText('真实操作已完成', { exact: false }).waitFor({ state: 'visible', timeout: 20_000 });
await page.locator('.ant-modal-close:visible').click({ force: true });
await bottomExportModal.waitFor({ state: 'hidden' });

const saveAction = await openAndSubmitDrawer(page, '保存查询', '真实业务操作完成', '/api/v1/audit/saved-queries');
const integrityAction = await openAndSubmitDrawer(page, '归档校验', '完整性检查', '/api/v1/audit/integrity-checks');

const viewerPage = await context.newPage();
await viewerPage.setViewportSize({ width: 1920, height: 1080 });
const viewerUrl = new URL(`/audit-log?__codex_ui_breakdown_production=1&viewerCheckTs=${Date.now()}`, baseUrl);
viewerUrl.hash = `codex_smoke_token=${smokeToken({ permissions: ['audit:read'], roles: ['viewer'], username: 'codex-windows-cdp-audit-viewer' })}`;
await viewerPage.goto(viewerUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await viewerPage.locator('.taf-auditlog-table-panel tbody tr.ant-table-row').first().waitFor({ state: 'visible', timeout: 20_000 });
const viewerPermissionGate = {
  row_review_disabled: await viewerPage.locator('.taf-auditlog-row-actions button').filter({ hasText: '复核' }).first().isDisabled(),
  title_review_disabled: await viewerPage.locator('.taf-auditlog-titlebar button').filter({ hasText: '触发复核' }).isDisabled(),
  review_tab_disabled: await viewerPage.locator('.taf-auditlog-detail-tabs button').filter({ hasText: '复核操作' }).isDisabled(),
};
await viewerPage.close();

await sleep(500);
const layout = await page.evaluate(() => {
  const body = document.documentElement;
  const tableBody = document.querySelector('.taf-auditlog-table-panel .ant-table-body');
  const shell = document.querySelector('.taf-auditlog-shell');
  const tablePanel = document.querySelector('.taf-auditlog-table-panel');
  const tableTitle = tablePanel?.querySelector('.taf-panel__header h2');
  const paginationElement = tablePanel?.querySelector('.taf-auditlog-pagination');
  const exportBody = document.querySelector('.taf-auditlog-bottom > .taf-panel:last-child > .taf-panel__body');
  const exportButton = exportBody?.querySelector('.ant-btn');
  const within = (child, parent, tolerance = 1) => {
    if (!child || !parent) return false;
    const c = child.getBoundingClientRect();
    const p = parent.getBoundingClientRect();
    return c.top >= p.top - tolerance && c.left >= p.left - tolerance
      && c.right <= p.right + tolerance && c.bottom <= p.bottom + tolerance;
  };
  const bottomBodies = [...document.querySelectorAll('.taf-auditlog-bottom > .taf-panel > .taf-panel__body')];
  const bottomBodyMetrics = bottomBodies.map((element) => ({
    client_height: element.clientHeight,
    scroll_height: element.scrollHeight,
    overflow_y: getComputedStyle(element).overflowY,
  }));
  const tableTitleInside = within(tableTitle, tablePanel);
  const paginationInside = within(paginationElement, tablePanel);
  const exportButtonInside = within(exportButton, exportBody);
  const bottomContentFits = bottomBodyMetrics.every((item) => item.scroll_height <= item.client_height + 2);
  return {
    document_scroll_width: body.scrollWidth,
    document_client_width: body.clientWidth,
    horizontal_overflow: body.scrollWidth > body.clientWidth + 1,
    table_overflow_y: tableBody ? getComputedStyle(tableBody).overflowY : null,
    shell_box: shell ? shell.getBoundingClientRect().toJSON() : null,
    table_title_inside: tableTitleInside,
    pagination_inside: paginationInside,
    export_button_inside: exportButtonInside,
    bottom_content_fits: bottomContentFits,
    bottom_body_metrics: bottomBodyMetrics,
    clip_free: tableTitleInside && paginationInside && exportButtonInside && bottomContentFits,
  };
});

// The viewer gate uses the same origin and therefore replaces the shared
// smoke token. Restore the admin token before leaving the final Windows Chrome
// tab open for handoff so its visible write controls and API credentials agree.
const handoffUrl = new URL(`/audit-log?__codex_ui_breakdown_production=1&windowsCdpHandoffTs=${Date.now()}`, baseUrl);
handoffUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(handoffUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-auditlog-table-panel tbody tr.ant-table-row').first().waitFor({ state: 'visible', timeout: 20_000 });

const result = {
  result: 'fail',
  run_id: runId,
  browser_backend: 'Windows Chrome CDP over Xshell reverse tunnel',
  browser: version.Browser,
  initial_target_count: initialTargets.length,
  route: redact(routeUrl.toString()),
  viewport,
  device_pixel_ratio: devicePixelRatio,
  title,
  total_text: totalText,
  row_count: rowCount,
  retention_canvas_count: retentionCanvasCount,
  pagination,
  operation_context_url: redact(operationContextUrl),
  related_chain_url: redact(relatedChainUrl),
  tab_navigation_contained: tabNavigationContained,
  detail_panel_boxes: {
    before: detailPanelBoxBefore,
    operation_context: detailPanelBoxOperationContext,
    related_chain: detailPanelBoxRelatedChain,
  },
  detail_has_real_id: detailHasRealId,
  row_action_counts: rowActionCounts,
  row_action_layout: rowActionLayout,
  native_pointer_actions: nativePointerActions,
  background_refetch_actionable: backgroundRefetchActionable,
  row_association_worked: rowAssociationWorked,
  row_review_targeted: rowReviewTargeted,
  row_review_retargeted: rowReviewRetargeted,
  row_review_request_matched: rowReviewRequestMatched,
  row_review_response_matched: rowReviewResponseMatched,
  title_review_worked: titleReviewWorked,
  review_panel_box: reviewPanelBox,
  review_layout: reviewLayout,
  viewer_permission_gate: viewerPermissionGate,
  row_review_evidence: rowReviewEvidence,
  selected_export_matched: selectedExportMatched,
  selected_export_response: {
    status: exportResponse.status(),
    row_count: exportResponseData?.row_count,
    total_matching: exportResponseData?.total_matching,
    truncated: exportResponseData?.truncated,
    request_log_id: exportRequestBody?.filters?.log_id,
    artifact_contains_log_id: exportArtifact.includes(firstRowId),
    artifact_sha256: exportArtifactSha256,
    server_sha256: exportResponseData?.sha256,
    artifact_sha256_matched: exportArtifactSha256 === exportServerSha256,
    content_type: exportResponseData?.content_type,
    file_signature: exportSignature,
  },
  bottom_export_matched: bottomExportMatched,
  bottom_export_response: {
    status: bottomExportResponse.status(),
    request_log_id: bottomExportRequestBody?.filters?.log_id,
    row_count: bottomExportResponseData?.row_count,
    total_matching: bottomExportResponseData?.total_matching,
    truncated: bottomExportResponseData?.truncated,
    artifact_contains_log_id: bottomExportArtifact.includes(firstRowId),
  },
  detail_panel_box: detailPanelBox,
  detail_panel_layout: detailPanelLayout,
  modal_box: modalBox,
  modal_layout: modalLayout,
  export_has_sha256: Boolean(exportText?.includes('sha256:')),
  save_persisted: saveAction.ok && saveAction.text.includes('PostgreSQL'),
  review_persisted: reviewText.includes('PostgreSQL'),
  integrity_honest_status: integrityAction.ok && (integrityAction.text.includes('逐条防篡改基线') || integrityAction.text.includes('历史基线比对通过')),
  mutation_actions: {
    save: saveAction,
    integrity: integrityAction,
  },
  layout,
  audit_api_responses: auditApiResponses,
  bad_responses: badResponses,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  request_failures: requestFailures,
  ignored_external_failures: ignoredExternalFailures,
  screenshots: Object.fromEntries(Object.entries(screenshots).map(([key, value]) => [key, path.relative(root, value)])),
  timestamp: new Date().toISOString(),
};

const required = [
  viewport?.width === 1920 && viewport?.height === 1080,
  Math.abs(devicePixelRatio - 1) < 0.001,
  title?.includes('审计'),
  rowCount > 0,
  retentionCanvasCount === 4,
  !pagination.exercised || (pagination.firstRowChanged && pagination.activePage === '2'),
  tabNavigationContained,
  detailHasRealId,
  Object.values(rowActionCounts).every((count) => count === 1),
  !rowActionLayout.clipped && rowActionLayout.buttons.every((button) => button.fully_inside && button.width >= 40 && button.height >= 30 && !button.disabled && button.center_hits_button),
  nativePointerActions.length === 3 && nativePointerActions.every((action) => action.center_hits_button && !action.disabled && action.box.width >= 40 && action.box.height >= 30),
  backgroundRefetchActionable.list_request_started && !backgroundRefetchActionable.detail_button_disabled && backgroundRefetchActionable.active_tab_after_native_click === '操作详情',
  rowAssociationWorked,
  rowReviewTargeted,
  rowReviewRetargeted,
  rowReviewRequestMatched,
  rowReviewResponseMatched,
  titleReviewWorked,
  reviewPanelBox && sameBox(reviewPanelBox, detailPanelBoxBefore),
  reviewLayout.content_inside && !reviewLayout.horizontal_overflow && reviewLayout.visible_drawers === 0 && reviewLayout.visible_modals === 0,
  Object.values(viewerPermissionGate).every(Boolean),
  selectedExportMatched,
  bottomExportMatched,
  detailPanelBox && detailPanelBox.width >= 560 && detailPanelBox.width <= 650,
  detailPanelLayout.content_inside && !detailPanelLayout.horizontal_overflow,
  modalBox && modalBox.width >= 920 && modalBox.width <= 980 && modalBox.height >= 480 && modalBox.height <= 820,
  !modalLayout.horizontal_overflow,
  result.export_has_sha256,
  result.save_persisted,
  result.review_persisted,
  result.integrity_honest_status,
  !layout.horizontal_overflow,
  layout.clip_free,
  badResponses.length === 0,
  consoleErrors.length === 0,
  pageErrors.length === 0,
  requestFailures.length === 0,
];
result.result = required.every(Boolean) ? 'pass' : 'fail';
result.required_checks = required;
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
// Keep the final audit-log tab open in the user's Windows Chrome for handoff.
process.exit(result.result === 'pass' ? 0 : 1);

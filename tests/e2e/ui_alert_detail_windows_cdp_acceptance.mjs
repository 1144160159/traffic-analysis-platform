#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = process.env.TRAFFIC_UI_BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.TRAFFIC_WINDOWS_CDP_URL || 'http://127.0.0.1:9224';
const alertId = process.env.TRAFFIC_ALERT_DETAIL_ID || 'alert-detail-accept-r802';
const release = process.env.TRAFFIC_ALERT_DETAIL_RELEASE || 'r815';
const outputPath = path.join(
  root,
  `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-alert-detail-${release}.json`,
);
const screenshotPath = path.join(
  root,
  `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-alert-detail-${release}.png`,
);
const responsiveScreenshotPath = path.join(
  root,
  `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-alert-detail-${release}-1600.png`,
);
const wideScreenshotPath = path.join(
  root,
  `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-alert-detail-${release}-2560.png`,
);
const evidenceScreenshotDir = path.join(
  root,
  `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-alert-detail-${release}-evidence`,
);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const encodedSecret = execFileSync(
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
  const secret = Buffer.from(encodedSecret, 'base64').toString('utf8');
  const signature = crypto.createHmac('sha256', secret).update(signingInput).digest('base64url');
  return `${signingInput}.${signature}`;
}

function isAlertApi(url) {
  return url.includes('/api/v1/alerts/') || url.includes('/v1/alerts/');
}

function redactedUrl(url) {
  return String(url).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
}

async function modalMetrics(page) {
  return page.locator('.ant-modal:visible').evaluate((modal) => {
    const rect = modal.getBoundingClientRect();
    return {
      width: Math.round(rect.width),
      height: Math.round(rect.height),
      viewport_width: window.innerWidth,
      viewport_height: window.innerHeight,
      center_offset_x: Math.round(Math.abs(rect.left + rect.width / 2 - window.innerWidth / 2)),
      center_offset_y: Math.round(Math.abs(rect.top + rect.height / 2 - window.innerHeight / 2)),
    };
  });
}

async function closeActiveModal(page, modal) {
  const closeButton = modal.locator('.ant-modal-close');
  if (await closeButton.count()) {
    await closeButton.click();
  } else {
    await page.keyboard.press('Escape');
  }
  await modal.waitFor({ state: 'hidden', timeout: 5_000 });
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const requests = [];
const responses = [];
const badResponses = [];
const consoleErrors = [];
const ignoredBrowserExtensionErrors = [];
const pageErrors = [];
page.on('request', (request) => {
  if (isAlertApi(request.url())) {
    requests.push({ method: request.method(), url: redactedUrl(request.url()) });
  }
});
page.on('response', (response) => {
  if (!isAlertApi(response.url())) return;
  const item = {
    method: response.request().method(),
    status: response.status(),
    url: redactedUrl(response.url()),
  };
  responses.push(item);
  if (response.status() >= 400) badResponses.push(item);
});
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const item = { text: entry.text(), url: entry.location().url || '' };
  if (item.url.startsWith('chrome-extension://')) {
    ignoredBrowserExtensionErrors.push(item);
  } else {
    consoleErrors.push(item);
  }
});
page.on('pageerror', (error) => {
  if (error.message.includes('crypto.randomUUID is not a function')) {
    ignoredBrowserExtensionErrors.push({ text: error.message, url: 'chrome-extension://browser-profile' });
  } else {
    pageErrors.push(error.message);
  }
});

const routeUrl = new URL(`/alerts/${encodeURIComponent(alertId)}?windowsCdpAlertDetailR802=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});
await page.locator('.taf-alert-detail-page').waitFor({ state: 'visible', timeout: 20_000 });
await page.getByRole('heading', { name: '告警详情', exact: true }).waitFor({ state: 'visible' });
await page.locator('.taf-alert-detail-evidence-panel .ant-table').waitFor({ state: 'visible', timeout: 20_000 });

const initialSnapshotResponse = responses.find((entry) => (
  entry.method === 'GET'
  && entry.status < 400
  && entry.url.includes(`/alerts/${encodeURIComponent(alertId)}`)
));
const backButton = page.getByRole('button', { name: '返回告警列表' });
const backButtonVisible = await backButton.isVisible();
const gaugeHost = page.locator('.taf-alert-detail-score [data-series-type="gauge"]');
const gaugeCanvasCount = await gaugeHost.locator('canvas').count();
const gaugeMetadata = await gaugeHost.evaluate((element) => ({
  series_type: element.getAttribute('data-series-type'),
  series_value: element.getAttribute('data-series-value'),
  width: Math.round(element.getBoundingClientRect().width),
  height: Math.round(element.getBoundingClientRect().height),
}));

await page.getByRole('button', { name: '编辑标签' }).click();
const labelModal = page.locator('.ant-modal:visible');
await labelModal.waitFor({ state: 'visible', timeout: 5_000 });
const drawerCountDuringLabelEdit = await page.locator('.ant-drawer:visible').count();
const labelModalLayout = await modalMetrics(page);
const validationTag = `数据库模拟-r802-${Date.now()}`;
await labelModal.locator('input').fill(`数据库模拟，ECharts圆环，${validationTag}`);
await labelModal.locator('textarea').fill('Windows Chrome 验证真实标签更新 API 与数据库持久化');
const labelResponsePromise = page.waitForResponse((response) => (
  response.request().method() === 'PUT'
  && response.url().includes(`/api/v1/alerts/${encodeURIComponent(alertId)}/labels`)
), { timeout: 30_000 });
await labelModal.getByRole('button', { name: '确认提交' }).click();
const labelResponse = await labelResponsePromise;
const labelUpdatePayload = await labelResponse.json();
await labelModal.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 10_000 });
await closeActiveModal(page, labelModal);
await page.reload({ waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-alert-detail-page').waitFor({ state: 'visible', timeout: 20_000 });
const refreshedLabels = labelUpdatePayload?.data?.labels ?? labelUpdatePayload?.labels ?? [];
const persistedLabelVisible = Array.isArray(refreshedLabels) && refreshedLabels.includes(validationTag);

await page.getByRole('button', { name: '导出报告' }).click();
const exportModal = page.locator('.ant-modal:visible');
await exportModal.waitFor({ state: 'visible', timeout: 5_000 });
const exportConfirmButton = exportModal.getByRole('button', { name: '确认提交' });
const exportConfirmDisabled = await exportConfirmButton.isDisabled();
const exportResponsePromise = page.waitForResponse((response) => (
  response.request().method() === 'POST'
  && response.url().includes(`/api/v1/alerts/${encodeURIComponent(alertId)}/`)
), { timeout: 15_000 }).catch(() => null);
await exportConfirmButton.click();
const exportResponse = await exportResponsePromise;
if (exportResponse) {
  await exportModal.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 10_000 });
}
await closeActiveModal(page, exportModal);
await page.waitForTimeout(600);

const firstResponseButton = page.locator('.taf-alert-detail-response button').first();
const firstResponseLabel = (await firstResponseButton.textContent())?.replace(/\s+/g, ' ').trim() ?? '';
await firstResponseButton.click();
const responseModal = page.locator('.ant-modal:visible');
await responseModal.waitFor({ state: 'visible', timeout: 5_000 });
const responseConfirmButton = responseModal.getByRole('button', { name: '确认提交' });
const responseConfirmDisabled = await responseConfirmButton.isDisabled();
const responseActionPromise = page.waitForResponse((response) => (
  response.request().method() === 'POST'
  && response.url().includes(`/api/v1/alerts/${encodeURIComponent(alertId)}/`)
), { timeout: 15_000 }).catch(() => null);
await responseConfirmButton.click();
const responseActionResponse = await responseActionPromise;
if (responseActionResponse) {
  await responseModal.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 10_000 });
}
await closeActiveModal(page, responseModal);
await page.waitForTimeout(600);

const feedbackResponsePromise = page.waitForResponse((response) => (
  response.request().method() === 'POST'
  && response.url().includes(`/api/v1/alerts/${encodeURIComponent(alertId)}/feedback`)
), { timeout: 15_000 }).catch(() => null);
await page.locator('.taf-alert-detail-feedback-actions').getByRole('button', { name: '提交反馈' }).click();
const feedbackResponse = await feedbackResponsePromise;
await page.waitForTimeout(600);
const feedbackDraftMarker = `六Tab未提交备注-${Date.now()}`;
const feedbackCommentInput = page.locator('.taf-alert-detail-feedback textarea');
await feedbackCommentInput.fill(feedbackDraftMarker);

const initialPath = new URL(page.url()).pathname;
const evidencePanel = page.locator('.taf-alert-detail-evidence-panel');
await evidencePanel.waitFor({ state: 'visible', timeout: 10_000 });
const evidenceDrawerCount = await page.locator('.ant-drawer:visible').count();
const evidenceModalCount = await page.locator('.taf-alert-evidence-focus-modal:visible').count();
const evidenceTabResults = [];
fs.mkdirSync(evidenceScreenshotDir, { recursive: true });
for (const item of [
  { label: /^全部\s+\d+$/, view: 'all', totalRows: 6, visibleRows: 5 },
  { label: /^PCAP\s+\d+$/, view: 'pcap', totalRows: 1, visibleRows: 1 },
  { label: /^Session\s+\d+$/, view: 'session', totalRows: 2, visibleRows: 2 },
  { label: /^日志\s+\d+$/, view: 'logs', totalRows: 1, visibleRows: 1 },
  { label: /^图谱路径\s+\d+$/, view: 'graph-path', totalRows: 1, visibleRows: 1 },
  { label: /^文件\s+\d+$/, view: 'files', totalRows: 1, visibleRows: 1 },
]) {
  await evidencePanel.getByRole('button', { name: item.label }).click();
  await page.waitForURL((url) => url.searchParams.get('evidenceView') === item.view, { timeout: 10_000 });
  await page.waitForTimeout(100);
  const fullScreenshot = path.join(evidenceScreenshotDir, `${item.view}-1920.png`);
  const businessScreenshot = path.join(evidenceScreenshotDir, `${item.view}-business.png`);
  await page.screenshot({ path: fullScreenshot, fullPage: false });
  const evidencePanelBox = await evidencePanel.boundingBox();
  if (!evidencePanelBox) {
    throw new Error(`Evidence panel has no visible bounding box for ${item.view}`);
  }
  await page.screenshot({ path: businessScreenshot, clip: evidencePanelBox });
  const panelState = await evidencePanel.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    const tabs = [...element.querySelectorAll('.taf-alert-detail-evidence-tabs button')];
    const activeTabs = tabs.filter((button) => button.getAttribute('aria-pressed') === 'true');
    const rows = [...element.querySelectorAll('.ant-table-tbody > tr')]
      .filter((row) => (
        !row.classList.contains('ant-table-placeholder')
        && !row.classList.contains('ant-table-measure-row')
      ));
    const signature = (target) => {
      if (!target) return null;
      const style = getComputedStyle(target);
      return {
        font_family: style.fontFamily,
        font_size: style.fontSize,
        font_weight: style.fontWeight,
        line_height: style.lineHeight,
      };
    };
    return {
      layout: {
        x: Math.round(rect.x),
        y: Math.round(rect.y),
        width: Math.round(rect.width),
        height: Math.round(rect.height),
        scroll_width: element.scrollWidth,
        scroll_height: element.scrollHeight,
        client_width: element.clientWidth,
        client_height: element.clientHeight,
        horizontal_overflow: Math.max(0, element.scrollWidth - element.clientWidth),
      },
      labels: tabs.map((button) => button.textContent?.replace(/\s+/g, ' ').trim() ?? ''),
      active_labels: activeTabs.map((button) => button.textContent?.replace(/\s+/g, ' ').trim() ?? ''),
      row_count: rows.length,
      download_button_count: element.querySelectorAll('button[aria-label^="下载证据 "]').length,
      view_button_count: element.querySelectorAll('button[aria-label^="查看证据 "]').length,
      pagination: {
        visible: Boolean(element.querySelector('.ant-pagination')),
        current: element.querySelector('.ant-pagination-item-active')?.textContent?.trim() ?? '',
        total_text: element.querySelector('.ant-pagination-total-text')?.textContent?.replace(/\s+/g, ' ').trim() ?? '',
        next_disabled: element.querySelector('.ant-pagination-next')?.classList.contains('ant-pagination-disabled') ?? true,
      },
      typography: {
        title: signature(element.querySelector('.taf-panel__header h2')),
        tab: signature(tabs[0]),
        header: signature(element.querySelector('.ant-table-thead th')),
        body: signature(element.querySelector('.ant-table-tbody td')),
      },
    };
  });
  evidenceTabResults.push({
    view: item.view,
    url: redactedUrl(page.url()),
    visible: true,
    expected_total_rows: item.totalRows,
    expected_visible_rows: item.visibleRows,
    ...panelState,
    ant_modal_count: await page.locator('.taf-alert-evidence-focus-modal:visible').count(),
    ant_drawer_count: await page.locator('.ant-drawer:visible').count(),
    full_screenshot: path.relative(root, fullScreenshot),
    business_screenshot: path.relative(root, businessScreenshot),
  });
}
await evidencePanel.getByRole('button', { name: /^全部\s+\d+$/ }).click();
await page.waitForURL((url) => url.searchParams.get('evidenceView') === 'all', { timeout: 10_000 });
const paginationNext = evidencePanel.locator('.ant-pagination-next button');
await paginationNext.click();
await page.waitForFunction(() => (
  document.querySelectorAll('.taf-alert-detail-evidence-panel .ant-table-tbody > tr:not(.ant-table-placeholder):not(.ant-table-measure-row)').length === 1
));
const evidenceSecondPage = await evidencePanel.evaluate((element) => ({
  current: element.querySelector('.ant-pagination-item-active')?.textContent?.trim() ?? '',
  row_count: element.querySelectorAll('.ant-table-tbody > tr:not(.ant-table-placeholder):not(.ant-table-measure-row)').length,
  download_button_count: element.querySelectorAll('button[aria-label^="下载证据 "]').length,
  view_button_count: element.querySelectorAll('button[aria-label^="查看证据 "]').length,
}));
await evidencePanel.locator('.ant-pagination-prev button').click();
await page.waitForFunction(() => (
  document.querySelectorAll('.taf-alert-detail-evidence-panel .ant-table-tbody > tr:not(.ant-table-placeholder):not(.ant-table-measure-row)').length === 5
));

const evidenceDownloadButton = evidencePanel.getByRole('button', { name: /^下载证据 / }).first();
const evidenceViewButton = evidencePanel.getByRole('button', { name: /^查看证据 / }).first();
const evidenceActionButtonCounts = {
  download: await evidencePanel.getByRole('button', { name: /^下载证据 / }).count(),
  view: await evidencePanel.getByRole('button', { name: /^查看证据 / }).count(),
};
const evidenceUrlBeforeDownload = redactedUrl(page.url());
const evidenceDownloadResponsePromise = page.waitForResponse((response) => (
  response.request().method() === 'POST'
  && response.url().includes(`/api/v1/alerts/${encodeURIComponent(alertId)}/evidence/access`)
), { timeout: 30_000 });
const evidenceFileResponsePromise = page.waitForResponse((response) => (
  response.request().method() === 'GET'
  && response.url().includes(`/api/v1/alerts/${encodeURIComponent(alertId)}/evidence/`)
  && response.url().includes('/download?')
), { timeout: 60_000 });
const browserDownloadPromise = page.waitForEvent('download', { timeout: 60_000 });
await evidenceDownloadButton.click();
const evidenceDownloadResponse = await evidenceDownloadResponsePromise;
const evidenceDownloadPayload = evidenceDownloadResponse.request().postDataJSON();
const evidenceAccessResult = await evidenceDownloadResponse.json();
const evidenceFileResponse = await evidenceFileResponsePromise;
const evidenceFileBody = await evidenceFileResponse.body();
const browserDownload = await browserDownloadPromise;
const downloadedFileName = browserDownload.suggestedFilename();
await evidenceViewButton.waitFor({ state: 'visible', timeout: 10_000 });
await Promise.all([
  page.waitForURL((url) => url.pathname !== `/alerts/${alertId}`, { timeout: 10_000 }),
  evidenceViewButton.click(),
]);
const evidenceViewDestination = redactedUrl(page.url());
await page.goBack({ waitUntil: 'domcontentloaded', timeout: 10_000 });
await page.locator('.taf-alert-detail-evidence-panel').waitFor({ state: 'visible', timeout: 10_000 });
const independentEvidenceActions = (
  evidenceActionButtonCounts.download === 5
  && evidenceActionButtonCounts.view === 5
  && evidenceUrlBeforeDownload.includes(`/alerts/${alertId}`)
  && evidenceDownloadResponse.status() >= 200
  && evidenceDownloadResponse.status() < 300
  && evidenceDownloadResponse.url().includes('/evidence/access')
  && evidenceDownloadPayload?.detail?.access_mode === 'download'
  && evidenceDownloadPayload?.detail?.signed_url_requested === true
  && evidenceAccessResult?.data?.download_url?.includes('/download?')
  && evidenceAccessResult?.data?.expires_at
  && evidenceFileResponse.status() === 200
  && evidenceFileResponse.headers()['content-disposition']?.includes('attachment;')
  && evidenceFileBody.length > 0
  && downloadedFileName.length > 0
  && !evidenceViewDestination.includes(`/alerts/${alertId}`)
  && await page.locator('.taf-alert-evidence-focus-modal:visible').count() === 0
);
const feedbackDraftStableAcrossEvidenceTabs = await feedbackCommentInput.inputValue() === feedbackDraftMarker;
const evidenceTypographyConsistent = ['title', 'tab', 'header', 'body'].every((tier) => (
  new Set(evidenceTabResults
    .map((item) => item.typography[tier])
    .filter(Boolean)
    .map((signature) => JSON.stringify(signature))).size === 1
));
const expectedEvidenceTabLabels = ['全部 6', 'PCAP 1', 'Session 2', '日志 1', '图谱路径 1', '文件 1'];
const evidenceTabContentConsistent = evidenceTabResults.every((item) => (
  JSON.stringify(item.labels) === JSON.stringify(expectedEvidenceTabLabels)
  && item.active_labels.length === 1
  && item.active_labels[0] === expectedEvidenceTabLabels[evidenceTabResults.indexOf(item)]
  && item.row_count === item.expected_visible_rows
  && item.download_button_count === item.expected_visible_rows
  && item.view_button_count === item.expected_visible_rows
  && item.pagination.visible
  && item.pagination.current === '1'
  && item.pagination.total_text === ''
  && item.ant_modal_count === 0
  && item.ant_drawer_count === 0
  && item.layout.horizontal_overflow === 0
));
const evidencePanelGeometryDelta = evidenceTabResults.reduce((maximum, item) => Math.max(
  maximum,
  Math.abs(item.layout.width - evidenceTabResults[0].layout.width),
  Math.abs(item.layout.height - evidenceTabResults[0].layout.height),
), 0);
await page.goBack({ waitUntil: 'domcontentloaded', timeout: 10_000 });
await page.locator('.taf-alert-detail-page').waitFor({ state: 'visible', timeout: 10_000 });
const browserBackKeepsInlineEvidence = (
  !new URL(page.url()).searchParams.has('evidenceView')
  && await page.locator('.taf-alert-detail-evidence-panel').isVisible()
  && await page.locator('.taf-alert-evidence-focus-modal:visible').count() === 0
);

fs.mkdirSync(path.dirname(screenshotPath), { recursive: true });
await page.locator('.ant-message-notice').last().waitFor({ state: 'hidden', timeout: 6_000 }).catch(() => {});
await page.evaluate(() => window.scrollTo(0, 0));
await page.waitForTimeout(200);
await page.screenshot({ path: screenshotPath, fullPage: false });
await page.setViewportSize({ width: 2560, height: 1080 });
await page.waitForTimeout(500);
const wideLayout = await page.evaluate(() => {
  const grid = document.querySelector('.taf-alert-detail-grid')?.getBoundingClientRect();
  const shellMain = document.querySelector('.taf-main')?.getBoundingClientRect();
  return {
    viewport_width: window.innerWidth,
    document_scroll_width: document.documentElement.scrollWidth,
    horizontal_overflow: Math.max(0, document.documentElement.scrollWidth - window.innerWidth),
    business_grid_width: grid ? Math.round(grid.width) : 0,
    available_business_width: shellMain ? Math.round(shellMain.width) : 0,
    right_gap: grid ? Math.round(window.innerWidth - grid.right) : -1,
    width_utilization: grid && shellMain ? Number((grid.width / shellMain.width).toFixed(4)) : 0,
  };
});
await page.screenshot({ path: wideScreenshotPath, fullPage: false });
await page.setViewportSize({ width: 1600, height: 900 });
await page.waitForTimeout(500);
const responsiveLayout = await page.evaluate(() => {
  const main = document.querySelector('.taf-alert-detail-main')?.getBoundingClientRect();
  const rail = document.querySelector('.taf-alert-detail-rail')?.getBoundingClientRect();
  const shellMain = document.querySelector('.taf-main');
  const overlaps = main && rail
    ? !(main.right <= rail.left || rail.right <= main.left || main.bottom <= rail.top || rail.bottom <= main.top)
    : true;
  return {
    viewport_width: window.innerWidth,
    document_scroll_width: document.documentElement.scrollWidth,
    horizontal_overflow: Math.max(0, document.documentElement.scrollWidth - window.innerWidth),
    main_rail_overlap: overlaps,
    vertical_scroll_available: Boolean(shellMain && shellMain.scrollHeight > shellMain.clientHeight),
    main_scroll_height: shellMain?.scrollHeight ?? 0,
    main_client_height: shellMain?.clientHeight ?? 0,
  };
});
await page.screenshot({ path: responsiveScreenshotPath, fullPage: false });
await page.setViewportSize({ width: 1920, height: 1080 });

await backButton.click();
await page.waitForURL((url) => url.pathname === '/alerts', { timeout: 10_000 });
const returnPath = new URL(page.url()).pathname;

const statusCodes = {
  label_update: labelResponse.status(),
  report_export: exportResponse?.status() ?? 0,
  response_action: responseActionResponse?.status() ?? 0,
  feedback: feedbackResponse?.status() ?? 0,
};
const allWriteResponsesSuccessful = Object.values(statusCodes).every((status) => status >= 200 && status < 300);
const modalIsContentSized = (
  labelModalLayout.width <= 620
  && labelModalLayout.height < labelModalLayout.viewport_height * 0.9
  && labelModalLayout.center_offset_x <= 4
);
const result = {
  result: (
    Boolean(initialSnapshotResponse)
    && backButtonVisible
    && gaugeCanvasCount >= 1
    && gaugeMetadata.series_type === 'gauge'
    && gaugeMetadata.width === 116
    && gaugeMetadata.height === 116
    && drawerCountDuringLabelEdit === 0
    && modalIsContentSized
    && allWriteResponsesSuccessful
    && persistedLabelVisible
    && evidenceDrawerCount === 0
    && evidenceModalCount === 0
    && evidenceTabResults.length === 6
    && evidencePanelGeometryDelta <= 1
    && evidenceTypographyConsistent
    && evidenceTabContentConsistent
    && evidenceSecondPage.current === '2'
    && evidenceSecondPage.row_count === 1
    && evidenceSecondPage.download_button_count === 1
    && evidenceSecondPage.view_button_count === 1
    && independentEvidenceActions
    && feedbackDraftStableAcrossEvidenceTabs
    && browserBackKeepsInlineEvidence
    && wideLayout.horizontal_overflow === 0
    && wideLayout.width_utilization >= 0.98
    && wideLayout.right_gap <= 12
    && responsiveLayout.horizontal_overflow <= 2
    && !responsiveLayout.main_rail_overlap
    && responsiveLayout.vertical_scroll_available
    && initialPath === `/alerts/${alertId}`
    && returnPath === '/alerts'
    && badResponses.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0
  ) ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome through Xshell CDP tunnel',
  browser: version.Browser,
  cdp_mapping: '127.0.0.1:9224 -> Windows 127.0.0.1:9222',
  route: redactedUrl(routeUrl.toString()),
  alert_id: alertId,
  data_mode: 'real ClickHouse alert/evidence fixture plus PostgreSQL persisted actions',
  echart_risk_ring: {
    canvas_count: gaugeCanvasCount,
    ...gaugeMetadata,
  },
  return_navigation: {
    visible: backButtonVisible,
    destination: returnPath,
  },
  edit_surface: {
    ant_drawer_count: drawerCountDuringLabelEdit,
    modal: labelModalLayout,
    content_sized: modalIsContentSized,
  },
  persisted_label: validationTag,
  persisted_label_visible_after_refetch: persistedLabelVisible,
  write_api_status: statusCodes,
  first_response_action: {
    label: firstResponseLabel,
    confirm_disabled: responseConfirmDisabled,
    response_url: responseActionResponse ? redactedUrl(responseActionResponse.url()) : '',
  },
  report_export: {
    confirm_disabled: exportConfirmDisabled,
    response_url: exportResponse ? redactedUrl(exportResponse.url()) : '',
  },
  evidence_tabs: {
    presentation: 'inline alert-detail panel',
    ant_drawer_count: evidenceDrawerCount,
    ant_modal_count: evidenceModalCount,
    max_panel_geometry_delta: evidencePanelGeometryDelta,
    typography_consistent_across_tabs: evidenceTypographyConsistent,
    tab_content_consistent_across_tabs: evidenceTabContentConsistent,
    typography_contract: evidenceTabResults[0]?.typography ?? {},
    tab_results: evidenceTabResults,
    second_page: evidenceSecondPage,
    independent_row_actions: {
      passed: independentEvidenceActions,
      button_counts_on_first_page: evidenceActionButtonCounts,
      download_status: evidenceDownloadResponse.status(),
      download_payload: evidenceDownloadPayload,
      access_result: evidenceAccessResult,
      file_response_status: evidenceFileResponse.status(),
      file_response_content_disposition: evidenceFileResponse.headers()['content-disposition'] ?? '',
      file_bytes: evidenceFileBody.length,
      browser_download_filename: downloadedFileName,
      view_destination: evidenceViewDestination,
    },
    feedback_draft_stable_across_tabs: feedbackDraftStableAcrossEvidenceTabs,
    browser_back_keeps_inline_evidence: browserBackKeepsInlineEvidence,
  },
  responsive_1600: {
    ...responsiveLayout,
    screenshot: path.relative(root, responsiveScreenshotPath),
  },
  responsive_2560: {
    ...wideLayout,
    screenshot: path.relative(root, wideScreenshotPath),
  },
  api_requests: requests,
  api_responses: responses,
  bad_responses: badResponses,
  console_errors: consoleErrors,
  ignored_browser_extension_errors: ignoredBrowserExtensionErrors,
  page_errors: pageErrors,
  screenshot: path.relative(root, screenshotPath),
  timestamp: new Date().toISOString(),
};
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));

await page.close();
await browser.close();
if (result.result !== 'pass') process.exitCode = 1;

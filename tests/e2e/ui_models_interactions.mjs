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
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/models');
const evidenceRevision = process.env.MODEL_EVIDENCE_REVISION?.trim() || 'r273';
const outputPath = path.join(evidenceDir, `interaction-${evidenceRevision}.json`);
const screenshotPath = path.join(evidenceDir, `interaction-${evidenceRevision}.png`);
const implementationPath = path.join(evidenceDir, `implementation-${evidenceRevision}.png`);

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
    permissions: ['*', 'admin:*', 'model:*'],
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
const modelRequestUrls = [];
page.on('response', (response) => { if (response.status() >= 400) badResponses.push({ status: response.status(), url: response.url() }); });
page.on('console', (entry) => { if (entry.type() === 'error') consoleErrors.push(entry.text()); });
page.on('pageerror', (error) => pageErrors.push(error.message));
page.on('requestfailed', (request) => requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? 'unknown' }));
page.on('request', (request) => { if (request.url().includes('/v1/models')) modelRequestUrls.push(request.url()); });

const routeUrl = new URL(`/models?windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-models').waitFor({ state: 'visible', timeout: 15_000 });

const chartCanvasCount = await page.locator('.taf-models canvas').count();
fs.mkdirSync(evidenceDir, { recursive: true });
await page.screenshot({ path: implementationPath, fullPage: false });
const listScroll = await page.locator('.taf-models-list-panel .taf-panel__body').evaluate((element) => {
  const style = getComputedStyle(element);
  const before = element.scrollTop;
  element.scrollTop = element.scrollHeight;
  const after = element.scrollTop;
  element.scrollTop = before;
  return {
    overflowY: style.overflowY,
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
    scrollable: element.scrollHeight > element.clientHeight && after > before,
  };
});
const pagination = page.locator('.taf-models-pagination');
const secondPageButton = pagination.getByRole('button', { name: '2', exact: true });
const hasSecondPage = await secondPageButton.count() > 0;
const pageTwoResponsePromise = hasSecondPage
  ? page.waitForResponse((response) => response.url().includes('/v1/models') && new URL(response.url()).searchParams.get('page') === '2')
  : undefined;
if (hasSecondPage) await secondPageButton.click();
const pageTwoResponse = pageTwoResponsePromise ? await pageTwoResponsePromise : undefined;
if (hasSecondPage) await page.waitForTimeout(500);
const pageTwoUrl = modelRequestUrls.map((url) => new URL(url)).find((url) => url.searchParams.get('page') === '2');
const pageTwoParamsValid = Boolean(pageTwoUrl
  && pageTwoUrl.searchParams.get('page') === '2'
  && pageTwoUrl.searchParams.get('limit') === '8'
  && pageTwoUrl.searchParams.get('page_size') === '8'
  && pageTwoUrl.searchParams.get('offset') === '8');
const pageTwoPayload = pageTwoResponse ? await pageTwoResponse.json() : undefined;
const pageTwoData = pageTwoPayload?.data;
const pageTwoRecords = Array.isArray(pageTwoData)
  ? pageTwoData
  : Array.isArray(pageTwoData?.models)
    ? pageTwoData.models
    : Array.isArray(pageTwoPayload?.models)
      ? pageTwoPayload.models
      : [];
const pageTwoApiName = String(pageTwoRecords[0]?.name ?? pageTwoRecords[0]?.model_name ?? '');
const pageTwoFirstRowText = await page.locator('.taf-models-list-panel tbody tr').first().textContent();
const pageTwoApiRowVisible = pageTwoRecords.length === 0 || Boolean(pageTwoApiName && pageTwoFirstRowText?.includes(pageTwoApiName));
const secondPageActive = hasSecondPage && await pagination.locator('button[aria-current="page"]').textContent() === '2';

const metricCanvas = page.locator('.taf-models-metric-echart canvas').first();
const metricHeading = page.locator('.taf-models-left-bottom .taf-panel__header h2').first();
const metricHeadingBefore = await metricHeading.textContent();
const metricCanvasBefore = await metricCanvas.evaluate((canvas) => canvas.toDataURL());
await page.locator('.taf-models-list-panel tbody tr').nth(1).locator('td').nth(1).click();
await page.waitForTimeout(450);
const metricHeadingAfter = await metricHeading.textContent();
const metricCanvasAfter = await metricCanvas.evaluate((canvas) => canvas.toDataURL());
const chartUpdatesWithSelection = metricHeadingBefore !== metricHeadingAfter && metricCanvasBefore !== metricCanvasAfter;

const importButton = page.locator('.taf-models-titlebar button').filter({ hasText: '导入模型' }).first();
await importButton.waitFor({ state: 'visible', timeout: 10_000 });
await importButton.click();
const actionDrawer = page.locator('.taf-models-action-drawer:visible');
await actionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const explicitEndpointVisible = await actionDrawer.getByText(/POST \/v1\/models\/.*\/versions/).isVisible();
const explicitAuditVisible = await actionDrawer.getByText('MODEL_VERSION_CREATE', { exact: true }).isVisible();
await actionDrawer.getByRole('button', { name: '确认提交', exact: true }).click();
const simulatedResult = actionDrawer.getByText(/已进入仿真任务队列/);
await simulatedResult.waitFor({ state: 'visible', timeout: 5_000 });
const simulatedResultVisible = await simulatedResult.isVisible();
await actionDrawer.locator('.ant-drawer-close').click();

const rowEvaluationButton = page.locator('.taf-models-row-actions').first().getByRole('button', { name: '评估模型', exact: true });
await rowEvaluationButton.click();
const rowActionDrawer = page.locator('.taf-models-action-drawer:visible');
await rowActionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const rowActionEndpointVisible = await rowActionDrawer.getByText(/POST \/v1\/models\/.*\/versions\/.*\/evaluate/).isVisible();
const rowActionAuditVisible = await rowActionDrawer.getByText('MODEL_EVALUATION_REQUESTED', { exact: true }).isVisible();
await rowActionDrawer.getByRole('button', { name: '确认提交', exact: true }).click();
await rowActionDrawer.getByText(/已进入仿真任务队列/).waitFor({ state: 'visible', timeout: 5_000 });
await rowActionDrawer.locator('.ant-drawer-close').click();

await page.locator('.taf-models-tabs').getByRole('button', { name: '规则贡献', exact: true }).click();
const explanationTabActive = await page.locator('.taf-models-tabs button.is-active').textContent() === '规则贡献';

const bottomActions = [
  { name: '激活到线上', endpoint: /\/activate/, audit: 'MODEL_VERSION_ACTIVATE' },
  { name: '停用模型', endpoint: /\/deprecate/, audit: 'MODEL_VERSION_DEPRECATE' },
  { name: /^回滚到 /, endpoint: /\/rollback/, audit: 'MODEL_VERSION_ROLLED_BACK' },
];
const bottomActionChecks = [];
const auditCountBefore = await page.evaluate(() => JSON.parse(sessionStorage.getItem('taf:model-action-audit') ?? '[]').length);
for (const action of bottomActions) {
  await page.getByRole('button', { name: action.name, exact: typeof action.name === 'string' }).click();
  const drawer = page.locator('.taf-models-action-drawer:visible');
  await drawer.waitFor({ state: 'visible', timeout: 5_000 });
  const endpoint = await drawer.getByText(action.endpoint).isVisible();
  const audit = await drawer.getByText(action.audit, { exact: true }).isVisible();
  await drawer.getByRole('button', { name: '确认提交', exact: true }).click();
  const queued = drawer.getByText(/已进入仿真任务队列/);
  await queued.waitFor({ state: 'visible', timeout: 5_000 });
  bottomActionChecks.push({
    endpoint,
    audit,
    queued: await queued.isVisible(),
  });
  await drawer.locator('.ant-drawer-close').click();
}
const auditCountAfter = await page.evaluate(() => JSON.parse(sessionStorage.getItem('taf:model-action-audit') ?? '[]').length);
const bottomActionsMapped = bottomActionChecks.every((check) => check.endpoint && check.audit && check.queued)
  && auditCountAfter >= auditCountBefore + bottomActions.length;

const activationPanel = page.locator('.taf-models-activation');
const activationSlider = activationPanel.locator('.ant-slider').first();
const sliderBefore = await activationPanel.locator('.taf-models-activation-controls > span b').textContent();
const sliderBox = await activationSlider.boundingBox();
if (sliderBox) await activationSlider.click({ position: { x: sliderBox.width * 0.7, y: sliderBox.height / 2 } });
const sliderAfter = await activationPanel.locator('.taf-models-activation-controls > span b').textContent();
const sliderStateful = sliderBefore !== sliderAfter;

const search = page.getByRole('textbox', { name: '搜索模型' });
await search.fill('UEBA');
const searchValue = await search.inputValue();

await page.screenshot({ path: screenshotPath, fullPage: false });
const result = {
  result: chartCanvasCount >= 9
    && listScroll.scrollable
    && secondPageActive
    && pageTwoParamsValid
    && pageTwoApiRowVisible
    && chartUpdatesWithSelection
    && explicitEndpointVisible
    && explicitAuditVisible
    && simulatedResultVisible
    && rowActionEndpointVisible
    && rowActionAuditVisible
    && explanationTabActive
    && bottomActionsMapped
    && sliderStateful
    && searchValue === 'UEBA'
    && badResponses.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0
    && requestFailures.length === 0 ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  chart_canvas_count: chartCanvasCount,
  list_scroll: listScroll,
  second_page_active: secondPageActive,
  page_two_params_valid: pageTwoParamsValid,
  page_two_api_row_visible: pageTwoApiRowVisible,
  page_two_api_name: pageTwoApiName,
  model_request_urls: modelRequestUrls,
  chart_updates_with_selection: chartUpdatesWithSelection,
  explicit_endpoint_visible: explicitEndpointVisible,
  explicit_audit_visible: explicitAuditVisible,
  simulated_result_visible: simulatedResultVisible,
  row_action_endpoint_visible: rowActionEndpointVisible,
  row_action_audit_visible: rowActionAuditVisible,
  explanation_tab_active: explanationTabActive,
  bottom_action_checks: bottomActionChecks,
  bottom_actions_mapped: bottomActionsMapped,
  slider_stateful: sliderStateful,
  search_value: searchValue,
  bad_responses: badResponses,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  request_failures: requestFailures,
  implementation_screenshot: path.relative(root, implementationPath),
  screenshot: path.relative(root, screenshotPath),
  timestamp: new Date().toISOString(),
};

fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);

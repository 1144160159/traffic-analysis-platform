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
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/mlops');
const revision = process.env.MLOPS_EVIDENCE_REVISION?.trim() || 'r284';
const outputPath = path.join(evidenceDir, `interaction-${revision}.json`);
const screenshotPath = path.join(evidenceDir, `interaction-${revision}.png`);
const implementationPath = path.join(evidenceDir, `implementation-${revision}.png`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const encoded = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(), tenant_id: 'default',
    username: 'codex-windows-cdp-admin', roles: ['admin'], permissions: ['*', 'admin:*', 'model:*'], token_type: 'access', iat: now, exp: now + 1_800,
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
page.on('response', (response) => { if (response.status() >= 400) badResponses.push({ status: response.status(), url: response.url() }); });
page.on('console', (entry) => { if (entry.type() === 'error') consoleErrors.push(entry.text()); });
page.on('pageerror', (error) => pageErrors.push(error.message));
page.on('requestfailed', (request) => requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? 'unknown' }));

const routeUrl = new URL(`/mlops?windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-mlops').waitFor({ state: 'visible', timeout: 15_000 });

fs.mkdirSync(evidenceDir, { recursive: true });
await page.screenshot({ path: implementationPath, fullPage: false });
const chartCanvasCount = await page.locator('.taf-mlops canvas').count();
const chartBounds = await page.locator('.taf-mlops canvas').evaluateAll((canvases) => canvases.map((canvas) => {
  const rect = canvas.getBoundingClientRect();
  return { width: rect.width, height: rect.height, top: rect.top, bottom: rect.bottom, visible: rect.width >= 20 && rect.height >= 15 && rect.top >= 80 && rect.bottom <= 997 };
}));
const chartsVisible = chartBounds.length >= 7 && chartBounds.every((item) => item.visible);

const pagination = page.locator('.taf-mlops-pagination');
const firstPageFirstTask = await page.locator('.taf-mlops .ant-table-tbody tr').first().textContent();
await pagination.getByRole('button', { name: '2', exact: true }).click();
const secondPageActive = await pagination.locator('button[aria-current="page"]').textContent() === '2';
const secondPageFirstTask = await page.locator('.taf-mlops .ant-table-tbody tr').first().textContent();
const laterPageChecks = [];
for (const pageNumber of [3, 4, 5, 6]) {
  await pagination.getByRole('button', { name: String(pageNumber), exact: true }).click();
  laterPageChecks.push({
    page: pageNumber,
    active: await pagination.locator('button[aria-current="page"]').textContent() === String(pageNumber),
    simulationMarked: Boolean((await page.locator('.taf-mlops .ant-table-tbody tr').first().textContent())?.includes('仿真')),
  });
}
await pagination.getByRole('button', { name: '2', exact: true }).click();

async function verifyAndSubmit(button, endpointPattern, auditEvent, detailsPatterns = []) {
  await button.click();
  const drawer = page.locator('.taf-mlops-action-drawer:visible');
  await drawer.waitFor({ state: 'visible', timeout: 5_000 });
  const endpoint = await drawer.getByText(endpointPattern).isVisible();
  const audit = await drawer.getByText(auditEvent, { exact: true }).isVisible();
  const drawerText = await drawer.textContent();
  const details = detailsPatterns.every((pattern) => drawerText?.includes(pattern));
  await drawer.getByRole('button', { name: '确认提交', exact: true }).click();
  const queued = drawer.getByText(/已进入仿真任务队列/);
  await queued.waitFor({ state: 'visible', timeout: 5_000 });
  const result = { endpoint, audit, details, queued: await queued.isVisible(), drawerText: await drawer.textContent() };
  await drawer.locator('.ant-drawer-close').click();
  return result;
}

const titleButtons = page.locator('.taf-mlops-titlebar button');
const actionChecks = [];
actionChecks.push(await verifyAndSubmit(titleButtons.filter({ hasText: '新建流水线' }).first(), /POST \/v1\/mlops\/pipelines/, 'MLOPS_PIPELINE_CREATED'));
actionChecks.push(await verifyAndSubmit(titleButtons.filter({ hasText: '投递训练任务' }).first(), /POST \/v1\/mlops\/retrain/, 'MLOPS_RETRAIN_REQUESTED'));
actionChecks.push(await verifyAndSubmit(titleButtons.filter({ hasText: '失败重试' }).first(), /POST \/v1\/mlops\/tasks\/.*\/retry/, 'MLOPS_TASK_RETRIED'));
actionChecks.push(await verifyAndSubmit(titleButtons.filter({ hasText: '停止任务' }).first(), /POST \/v1\/mlops\/tasks\/.*\/stop/, 'MLOPS_TASK_STOPPED'));
actionChecks.push(await verifyAndSubmit(titleButtons.filter({ hasText: '注册模型' }).first(), /POST \/v1\/models\/.*\/versions/, 'MODEL_VERSION_CREATE'));
actionChecks.push(await verifyAndSubmit(titleButtons.filter({ hasText: '任务详情' }).first(), /GET \/v1\/mlops\/tasks\/[^/]+$/, 'MLOPS_TASK_VIEWED', ['阶段', '数据集', '算法配置', '资源占用', '任务状态', '数据模式']))

const refreshResponse = page.waitForResponse((response) => response.url().includes('/v1/mlops/status'));
await titleButtons.nth(6).click();
const refreshWorked = (await refreshResponse).ok();

actionChecks.push(await verifyAndSubmit(page.locator('.taf-mlops-bottom .taf-panel').first().getByRole('button', { name: '发起标注', exact: true }), /POST \/v1\/mlops\/labels/, 'MLOPS_LABELING_REQUESTED'));

const rowActions = page.locator('.taf-mlops-row-actions').first();
actionChecks.push(await verifyAndSubmit(rowActions.getByRole('button', { name: '查看任务', exact: true }), /GET \/v1\/mlops\/tasks\/[^/]+$/, 'MLOPS_TASK_VIEWED', ['阶段', '数据集', '资源占用', '数据模式']));
actionChecks.push(await verifyAndSubmit(rowActions.getByRole('button', { name: '停止任务', exact: true }), /POST \/v1\/mlops\/tasks\/.*\/stop/, 'MLOPS_TASK_STOPPED'));
actionChecks.push(await verifyAndSubmit(rowActions.getByRole('button', { name: '重试任务', exact: true }), /POST \/v1\/mlops\/tasks\/.*\/retry/, 'MLOPS_TASK_RETRIED'));
actionChecks.push(await verifyAndSubmit(page.locator('.taf-mlops-feedback-pool button').first(), /GET \/v1\/feedback\/FB-/, 'MLOPS_FEEDBACK_VIEWED'));
actionChecks.push(await verifyAndSubmit(page.locator('.taf-mlops-register button').first(), /GET \/v1\/models\/abnorm_pkt_v3\/versions\/1.2.3/, 'MODEL_VERSION_VIEWED'));

const taskFilter = page.locator('.taf-mlops-bottom .ant-select').first();
await taskFilter.click();
await page.getByText('失败', { exact: true }).last().click();
const filterWorked = Boolean((await taskFilter.textContent())?.includes('失败')) && (await page.locator('.taf-mlops .ant-table-tbody tr').count()) > 0;
await taskFilter.click();
await page.getByText('全部任务', { exact: true }).last().click();

await page.screenshot({ path: screenshotPath, fullPage: false });
const interactionScreenshotValid = new URL(page.url()).pathname === '/mlops'
  && await page.locator('.taf-mlops').isVisible()
  && fs.statSync(screenshotPath).size > 100_000;

await page.locator('.taf-mlops-titlebar button').filter({ hasText: '进入部署管理' }).first().click();
await page.waitForURL(/\/deployments(?:\?|$)/, { timeout: 10_000 });
const deploymentNavigation = new URL(page.url()).pathname === '/deployments';

const result = {
  result: chartCanvasCount >= 7
    && chartsVisible
    && secondPageActive
    && Boolean(secondPageFirstTask && secondPageFirstTask !== firstPageFirstTask)
    && laterPageChecks.every((item) => item.active && item.simulationMarked)
    && actionChecks.every((item) => item.endpoint && item.audit && item.details && item.queued)
    && refreshWorked
    && filterWorked
    && interactionScreenshotValid
    && deploymentNavigation
    && badResponses.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0
    && requestFailures.length === 0 ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  chart_canvas_count: chartCanvasCount,
  chart_bounds: chartBounds,
  charts_visible: chartsVisible,
  second_page_active: secondPageActive,
  first_page_first_task: firstPageFirstTask,
  second_page_first_task: secondPageFirstTask,
  later_page_checks: laterPageChecks,
  action_checks: actionChecks,
  refresh_worked: refreshWorked,
  filter_worked: filterWorked,
  interaction_screenshot_valid: interactionScreenshotValid,
  deployment_navigation: deploymentNavigation,
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

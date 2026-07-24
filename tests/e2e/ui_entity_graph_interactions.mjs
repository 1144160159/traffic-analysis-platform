#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

const root = process.cwd();
const require = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = require('@playwright/test');
const baseUrl = process.env.UI_BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.UI_CDP_URL || 'http://127.0.0.1:9224';
const outputPath = path.join(root, 'doc/02_acceptance/02-regression/entity-graph-interactions-r586.json');
const screenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/graph/entity-graph-r586-interactions.png');
const accountDetailScreenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/graph/entity-graph-r586-account-detail.png');
const pathStateDir = path.join(root, 'evidence/ui-image-breakdowns/pages/graph/entity-graph-r586-path-tabs');

const jwt = () => {
  const secret = process.env.JWT_SECRET_OVERRIDE || Buffer.from(execFileSync('ssh', ['root@10.0.5.9', 'kubectl', '--kubeconfig=/tmp/codex-nebula-kubeconfig', '-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], {
    encoding: 'utf8',
    env: process.env,
    timeout: 15_000,
  }), 'base64').toString('utf8');
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-windows-cdp-entity-graph',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'graph:read', 'alert:read', 'asset:read', 'audit:read'],
    token_type: 'access',
    session_id: `entity-graph-${crypto.randomUUID()}`,
    iat: now,
    exp: now + 1800,
  })).toString('base64url');
  const signingInput = `${header}.${claims}`;
  const signature = crypto.createHmac('sha256', secret).update(signingInput).digest('base64url');
  return `${signingInput}.${signature}`;
};

const redact = (value) => String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
let persistedWindowsToken = '';
for (const candidate of context.pages()) {
  try {
    if (new URL(candidate.url()).hostname !== '10.0.5.8') continue;
    persistedWindowsToken = await candidate.evaluate(() => localStorage.getItem('traffic-ui-token') || '');
    if (persistedWindowsToken) break;
  } catch {
    // Ignore extension/devtools pages that do not expose the application origin storage.
  }
}
for (const stalePage of context.pages()) {
  if (/windowsCdp.*Ts=/.test(stalePage.url())) await stalePage.close().catch(() => {});
}
const page = await context.newPage();
await page.bringToFront();
await page.setViewportSize({ width: 1920, height: 1080 });
page.setDefaultTimeout(12_000);

const badResponses = [];
const requestFailures = [];
const consoleErrors = [];
const pageErrors = [];
page.on('response', (response) => {
  if (response.status() >= 400 && response.url().startsWith(baseUrl)) badResponses.push({ status: response.status(), url: redact(response.url()) });
});
page.on('requestfailed', (request) => requestFailures.push({ url: redact(request.url()), failure: request.failure()?.errorText ?? '' }));
page.on('console', (message) => {
  if (message.type() === 'error') consoleErrors.push(message.text());
});
page.on('pageerror', (error) => pageErrors.push(error.message));

const routeUrl = new URL(`/graph?windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${process.env.JWT_SECRET_OVERRIDE ? jwt() : persistedWindowsToken || jwt()}`;
const graphResponsePromise = page.waitForResponse((response) => new URL(response.url()).pathname.endsWith('/api/v1/graph/workbench') && response.status() === 200);
const initialPathResponsePromise = page.waitForResponse((response) => response.url().includes('/api/v1/graph/workbench/path') && response.url().includes('mode=shortest') && response.status() === 200);
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.bringToFront();
const graphResponse = await graphResponsePromise;
const initialPathPayload = await (await initialPathResponsePromise).json();
const graphPayload = await graphResponse.json();
const graphMeta = graphPayload?.data?.meta ?? graphPayload?.meta ?? {};
const graphData = graphPayload?.data?.graph ?? graphPayload?.graph ?? {};
await page.locator('.taf-graph-entity').waitFor({ state: 'visible' });
await page.getByText('节点 11', { exact: true }).waitFor({ state: 'visible' });
await page.getByText('关系 13', { exact: true }).waitFor({ state: 'visible' });
const graphTitleOnly = await page.locator('.taf-graph-titlebar').getByRole('heading', { name: '实体图谱', exact: true }).isVisible()
  && await page.locator('.taf-graph-titlebar').getByText('资产图谱', { exact: false }).count() === 0;
const openServices = await page.locator('.taf-graph-service-chips span').allTextContents();
const entityDetailSections = await page.locator('.taf-graph-detail').locator('.taf-graph-detail-section > strong, .taf-graph-activity-metrics small').allTextContents();
const entityMetricEcharts = await page.locator('.taf-graph-activity-metrics .taf-echarts-sparkline canvas').count();
const riskScoreEcharts = await page.locator('.taf-graph-risk-ring-echart canvas').count();
const initialEntityChartContract = await page.locator('.taf-graph-detail').evaluate((detail) => ({
  risk: {
    type: detail.querySelector('.taf-graph-risk-ring-echart')?.getAttribute('data-series-type'),
    value: Number(detail.querySelector('.taf-graph-risk-ring-echart')?.getAttribute('data-series-value')),
    color: detail.querySelector('.taf-graph-risk-ring-echart')?.getAttribute('data-series-color'),
  },
  trends: Array.from(detail.querySelectorAll('.taf-echarts-sparkline-source')).map((element) => ({
    type: element.getAttribute('data-series-type'),
    values: JSON.parse(element.getAttribute('data-series-values') || '[]'),
    source: element.getAttribute('data-series-source'),
  })),
}));
const timeRangeSelect = page.locator('.taf-graph-toolbar .ant-select').nth(0);
const sevenDayResponse = page.waitForResponse((response) => response.url().includes('/api/v1/graph/workbench?') && response.url().includes('time_range=7d') && response.status() === 200);
await timeRangeSelect.click();
await page.getByText('近7天', { exact: true }).last().click();
await sevenDayResponse;
const timeRangeLabelTracksFilter = await page.locator('.taf-graph-detail').getByText('最近7天流量', { exact: true }).isVisible();
await timeRangeSelect.click();
await page.getByText('近24小时', { exact: true }).last().click();
await page.locator('.taf-graph-detail').getByText('最近24小时流量', { exact: true }).waitFor({ state: 'visible' });
const queryGovernance = page.locator('.taf-graph-governance');
for (const label of ['慢查询数', '节点上限', '图缓存状态', '平均查询耗时', '查询历史', '最近查询']) {
  await queryGovernance.getByText(label, { exact: true }).waitFor({ state: 'visible' });
}
const queryGovernanceComplete = await queryGovernance.locator('.taf-graph-query-chart').isVisible()
  && await queryGovernance.locator('.taf-graph-recent-queries').isVisible();

const topologyCanvas = page.locator('.taf-graph-echart canvas').first();
await topologyCanvas.waitFor({ state: 'attached' });
const topologyCanvasCount = await page.locator('.taf-graph-echart canvas').count();
const topologyCanvasBox = await topologyCanvas.boundingBox();
const topologyNodeAnchorAlignment = await page.locator('.taf-entity-topology-chart').evaluate((chart) => {
  const chartBox = chart.getBoundingClientRect();
  const deltas = Array.from(chart.querySelectorAll('.taf-entity-topology-node')).map((button) => {
    const icon = button.querySelector('i');
    if (!(button instanceof HTMLElement) || !(icon instanceof HTMLElement)) return 999;
    const iconBox = icon.getBoundingClientRect();
    const leftPercent = Number.parseFloat(button.style.left);
    const topPercent = Number.parseFloat(button.style.top);
    const expectedX = chartBox.left + chartBox.width * leftPercent / 100;
    const expectedY = chartBox.top + chartBox.height * topPercent / 100;
    const actualX = iconBox.left + iconBox.width / 2;
    const actualY = iconBox.top + iconBox.height / 2;
    return Math.max(Math.abs(expectedX - actualX), Math.abs(expectedY - actualY));
  });
  return { nodes: deltas.length, max_center_delta_px: deltas.length ? Math.max(...deltas) : 999 };
});
const zoomScene = page.locator('.taf-entity-topology-scene');
const initialTopologyZoom = await zoomScene.getAttribute('data-zoom');
await page.getByRole('button', { name: '放大邻居图谱', exact: true }).click();
const enlargedTopologyZoom = await zoomScene.getAttribute('data-zoom');
await page.getByRole('button', { name: '缩小邻居图谱', exact: true }).click();
await page.getByRole('button', { name: '复位邻居图谱缩放', exact: true }).click();
const resetTopologyZoom = await zoomScene.getAttribute('data-zoom');
const topologyZoomControlsWork = initialTopologyZoom === '1.00' && enlargedTopologyZoom === '1.10' && resetTopologyZoom === '1.00';
const topologyRelationLabels = await page.locator('.taf-entity-topology-relation-label').evaluateAll((labels) => labels.map((label) => {
  const style = getComputedStyle(label);
  return {
    text: label.textContent?.trim() ?? '',
    font_family: style.fontFamily,
    font_size: style.fontSize,
    font_style: style.fontStyle,
    font_stretch: style.fontStretch,
    transform: style.transform,
  };
}));
const nodeSelect = page.locator('.taf-graph-node-select');
await nodeSelect.click();
await page.getByText('biz_admin', { exact: true }).last().click();
const selectedNodeChanged = await page.locator('.taf-graph-detail').getByText('biz_admin', { exact: true }).first().isVisible().catch(() => false);
const selectedEntityServices = await page.locator('.taf-graph-detail .taf-graph-service-chips span').allTextContents();
const accountEntityChartContract = await page.locator('.taf-graph-detail').evaluate((detail) => ({
  risk: {
    type: detail.querySelector('.taf-graph-risk-ring-echart')?.getAttribute('data-series-type'),
    value: Number(detail.querySelector('.taf-graph-risk-ring-echart')?.getAttribute('data-series-value')),
    color: detail.querySelector('.taf-graph-risk-ring-echart')?.getAttribute('data-series-color'),
  },
  trends: Array.from(detail.querySelectorAll('.taf-echarts-sparkline-source')).map((element) => ({
    type: element.getAttribute('data-series-type'),
    values: JSON.parse(element.getAttribute('data-series-values') || '[]'),
    source: element.getAttribute('data-series-source'),
  })),
}));
fs.mkdirSync(path.dirname(accountDetailScreenshotPath), { recursive: true });
await page.locator('.taf-graph-detail').screenshot({ path: accountDetailScreenshotPath });

const search = page.getByPlaceholder('搜索 IP / 账号 / 主机 / 域名 / 服务 / 告警 ID / 资产 ID');
await search.fill('biz_admin');
await page.getByText('节点 1', { exact: true }).waitFor({ state: 'visible' });
const searchFilterApplied = await page.getByText('节点 1', { exact: true }).isVisible();
const selectedEntityServicesAfterSearch = await page.locator('.taf-graph-detail .taf-graph-service-chips span').allTextContents();
await search.clear();
await page.getByText('节点 11', { exact: true }).waitFor({ state: 'visible' });

const entityTypeSelect = page.locator('.taf-graph-toolbar .ant-select').nth(2);
const accountFilterResponse = page.waitForResponse((response) => response.url().includes('/api/v1/graph/workbench?') && response.url().includes('entity_type=account') && response.status() === 200);
await entityTypeSelect.click();
await page.getByText('账号', { exact: true }).last().click();
const accountFilterPayload = await (await accountFilterResponse).json();
await page.getByText('节点 2', { exact: true }).waitFor({ state: 'visible' });
await page.getByText('关系 1', { exact: true }).waitFor({ state: 'visible' });
await entityTypeSelect.click();
await page.getByText('实体类型：全部', { exact: true }).last().click();
await page.getByText('节点 11', { exact: true }).waitFor({ state: 'visible' });
await page.locator('.taf-graph-titlebar button').filter({ hasText: '定位中心节点' }).click();
await page.locator('.taf-graph-detail').getByText('核心业务服务器', { exact: true }).waitFor({ state: 'visible' });

await page.getByRole('button', { name: '保存视图' }).click();
const savedView = await page.evaluate(() => JSON.parse(localStorage.getItem('traffic-graph-saved-view') || 'null'));
const downloadPromise = page.waitForEvent('download');
await page.getByRole('button', { name: '导出证据' }).click();
const download = await downloadPromise;

await page.getByRole('button', { name: '路径分析' }).click();
const pathPanel = page.locator('.taf-graph-path-results');
const pathPanelVisible = await pathPanel.isVisible();
const initialPathPanelBox = await pathPanel.boundingBox();
if (!initialPathPanelBox) throw new Error('Path result panel has no bounding box');
const governancePanelBox = await page.locator('.taf-graph-bottom > .taf-panel').nth(1).boundingBox();
if (!governancePanelBox) throw new Error('Query governance panel has no bounding box');
const governanceLayout = await page.locator('.taf-graph-query-governance-panel').evaluate((panel) => {
  const body = panel.querySelector('.taf-panel__body');
  const governance = panel.querySelector('.taf-graph-governance');
  const history = panel.querySelector('.taf-graph-query-history-grid');
  const chart = panel.querySelector('.taf-graph-query-chart');
  const recent = panel.querySelector('.taf-graph-recent-queries');
  const panelBox = panel.getBoundingClientRect();
  const boxes = [history, chart, recent].map((element) => element?.getBoundingClientRect());
  return {
    body_client_height: body?.clientHeight ?? 0,
    body_scroll_height: body?.scrollHeight ?? 0,
    governance_client_height: governance?.clientHeight ?? 0,
    governance_scroll_height: governance?.scrollHeight ?? 0,
    children_inside_panel: boxes.every((box) => box && box.top >= panelBox.top && box.bottom <= panelBox.bottom),
  };
});
const pathTabs = [
  { tab: '最短路径', mode: 'shortest', relation: '通信', min_length: 2, required_text: '路径长度', required_secondary_text: '路径 ID', required_attribute: 'service' },
  { tab: '攻击路径', mode: 'attack', relation: '关联告警', min_length: 2, required_text: '攻击阶段', required_secondary_text: '横向移动', required_attribute: 'attack_stage' },
  { tab: '通信路径', mode: 'communication', relation: 'DNS解析', min_length: 2, required_text: '通信频次', required_secondary_text: 'Top 对端', required_attribute: 'frequency' },
  { tab: '账号访问路径', mode: 'account', relation: '登录', min_length: 1, required_text: '身份标签', required_secondary_text: '异常访问', required_attribute: 'identity_label' },
];
const pathTabChecks = [];
fs.mkdirSync(pathStateDir, { recursive: true });
let currentPathPayload = initialPathPayload;
for (const [index, expected] of pathTabs.entries()) {
  if (index > 0) {
    const pathResponse = page.waitForResponse((response) => response.url().includes('/api/v1/graph/workbench/path') && response.url().includes(`mode=${expected.mode}`) && response.status() === 200);
    await pathPanel.getByRole('tab', { name: expected.tab, exact: true }).click();
    currentPathPayload = await (await pathResponse).json();
  }
  const modeView = pathPanel.locator(`[data-path-mode="${expected.mode}"]`);
  await modeView.waitFor({ state: 'visible' });
  await modeView.getByText(expected.required_text, { exact: false }).first().waitFor({ state: 'visible' });
  await modeView.getByText(expected.required_secondary_text, { exact: false }).first().waitFor({ state: 'visible' });
  if (expected.mode === 'attack') await modeView.getByText('告警锚点', { exact: true }).waitFor({ state: 'visible' });
  const actualPath = currentPathPayload?.data?.path ?? currentPathPayload?.path ?? {};
  const visual = modeView.locator('.taf-graph-pathline');
  const box = await pathPanel.boundingBox();
  const stateScreenshot = path.join(pathStateDir, `${expected.mode}.png`);
  await pathPanel.screenshot({ path: stateScreenshot });
  pathTabChecks.push({
    ...expected,
    actual_mode: actualPath.mode,
    actual_relation: expected.mode === 'attack'
      ? actualPath.edges?.find((edge) => edge.relation_type === expected.relation)?.relation_type
      : actualPath.edges?.[0]?.relation_type,
    actual_length: actualPath.length,
    required_attribute_value: actualPath.edges?.find((edge) => edge.attributes?.[expected.required_attribute] !== undefined)?.attributes?.[expected.required_attribute],
    box,
    panel_stable: Boolean(box)
      && Math.abs(box.x - initialPathPanelBox.x) <= 1
      && Math.abs(box.y - initialPathPanelBox.y) <= 1
      && Math.abs(box.width - initialPathPanelBox.width) <= 1
      && Math.abs(box.height - initialPathPanelBox.height) <= 1,
    inside_panel: await modeView.evaluate((element) => Boolean(element.closest('.taf-graph-path-results'))),
    ui_class: await modeView.getAttribute('class'),
    middle_visual_inside_panel: await visual.evaluate((element) => Boolean(element.closest('.taf-graph-path-results'))),
    middle_node_count: await visual.locator('.taf-graph-path-node').count(),
    middle_connector_count: await visual.locator('.taf-graph-path-connector').count(),
    screenshot: path.relative(root, stateScreenshot),
  });
}
console.error('[entity-graph-cdp] path tabs verified');

const pathModeActionNavigation = [];
const pathModeActions = [
  { tab: '攻击路径', mode: 'attack', button: '跳转攻击链', expected: /\/attack-chains(?:\?|$)/ },
  { tab: '通信路径', mode: 'communication', button: '查看证据', expected: /\/forensics(?:\?|$)/ },
  { tab: '账号访问路径', mode: 'account', button: '账号画像', expected: /\/assets(?:\?|$)/ },
  { tab: '账号访问路径', mode: 'account', button: '查看审计', expected: /\/audit-log(?:\?|$)/ },
];
for (const action of pathModeActions) {
  await page.goto(`${baseUrl}/graph?windowsCdpPathActionTs=${Date.now()}`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-graph-path-results').waitFor({ state: 'visible' });
  await page.locator('.taf-graph-path-results').getByRole('tab', { name: action.tab, exact: true }).click();
  await page.locator(`[data-path-mode="${action.mode}"]`).waitFor({ state: 'visible' });
  await page.locator(`[data-path-mode="${action.mode}"]`).getByRole('button', { name: action.button, exact: true }).click();
  await page.waitForURL(action.expected);
  await page.locator('.taf-page').first().waitFor({ state: 'visible' });
  const rendered = await page.locator('.taf-page').first().isVisible().catch(() => false);
  const accessDenied = await page.locator('.taf-access-denied').isVisible().catch(() => false);
  pathModeActionNavigation.push({
    tab: action.tab,
    button: action.button,
    passed: action.expected.test(new URL(page.url()).pathname + new URL(page.url()).search) && rendered && !accessDenied,
    rendered,
    access_denied: accessDenied,
    url: redact(page.url()),
  });
}
console.error('[entity-graph-cdp] path actions verified');
await page.goto(`${baseUrl}/graph?windowsCdpPathActionReturnTs=${Date.now()}`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-graph-entity').waitFor({ state: 'visible' });
console.error('[entity-graph-cdp] returned from path actions');

await page.getByRole('button', { name: '适配当前图谱视图' }).click();
await page.locator('.taf-graph-echart canvas').first().waitFor({ state: 'attached' });
const topologyResetKeptSingleCanvas = await page.locator('.taf-graph-echart canvas').count() === 1;
console.error('[entity-graph-cdp] topology reset verified');

if (!await page.locator('.taf-graph-detail').isVisible().catch(() => false)) {
  await page.getByRole('button', { name: '显示实体详情', exact: true }).click();
  await page.locator('.taf-graph-detail').waitFor({ state: 'visible' });
}
await page.locator('.taf-graph-detail').getByRole('button', { name: '关闭实体详情', exact: true }).click();
const detailClosed = await page.locator('.taf-graph-detail').count() === 0;
const expandedGridColumns = await page.locator('.taf-graph-grid').evaluate((element) => getComputedStyle(element).gridTemplateColumns);
await page.getByRole('button', { name: '显示实体详情', exact: true }).click();
const detailRestored = await page.locator('.taf-graph-detail').isVisible();

const verifyDestinationContext = async (label, expectedTarget) => {
  if (label === '查看资产') return (await page.getByPlaceholder('资产 ID / IP / 名称').inputValue()) === expectedTarget;
  if (label === '查看告警' || label === '跳转攻击链') return (await page.locator('.taf-source-context').textContent())?.includes(expectedTarget) ?? false;
  if (label === '进入取证') return (await page.locator('.taf-forensics-source').textContent())?.includes(expectedTarget) ?? false;
  if (label === '审计日志') return (await page.locator('.taf-source-context').textContent())?.includes(expectedTarget) ?? false;
  return false;
};
const destinations = [
  ['查看资产', /\/assets(?:\?|$)/, '10.20.4.18'],
  ['查看告警', /\/alerts(?:\?|$)/, '10.20.4.18'],
  ['进入取证', /\/forensics(?:\?|$)/, '10.20.4.18'],
  ['跳转攻击链', /\/attack-chains(?:\?|$)/, '10.20.4.18'],
  ['审计日志', /\/audit-log(?:\?|$)/, 'host:10.20.4.18'],
];
const navigation = [];
for (const [label, expected, expectedTarget] of destinations) {
  await page.getByRole('button', { name: label }).click();
  await page.waitForURL(expected);
  await page.locator('.taf-page').first().waitFor({ state: 'visible' });
  const rendered = await page.locator('.taf-page').first().isVisible().catch(() => false);
  const accessDenied = await page.locator('.taf-access-denied').isVisible().catch(() => false);
  const heading = (await page.locator('h1').first().textContent().catch(() => ''))?.trim() ?? '';
  const businessContext = await verifyDestinationContext(label, expectedTarget).catch(() => false);
  navigation.push({
    label,
    passed: expected.test(new URL(page.url()).pathname + new URL(page.url()).search) && rendered && !accessDenied && Boolean(heading) && businessContext,
    rendered,
    access_denied: accessDenied,
    heading,
    business_context: businessContext,
    url: redact(page.url()),
  });
  await page.goto(`${baseUrl}/graph?windowsCdpReturnTs=${Date.now()}`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-graph-entity').waitFor({ state: 'visible' });
}
console.error('[entity-graph-cdp] entity navigation verified');

const accountNavigation = [];
const accountDestinations = [
  ['查看资产', /\/assets(?:\?|$)/, 'biz_admin'],
  ['查看告警', /\/alerts(?:\?|$)/, 'biz_admin'],
  ['进入取证', /\/forensics(?:\?|$)/, 'biz_admin'],
  ['跳转攻击链', /\/attack-chains(?:\?|$)/, 'biz_admin'],
  ['审计日志', /\/audit-log(?:\?|$)/, 'account%3Abiz_admin'],
];
for (const [label, expected, expectedTarget] of accountDestinations) {
  const select = page.locator('.taf-graph-node-select');
  await select.click();
  await page.getByText('biz_admin', { exact: true }).last().click();
  await page.locator('.taf-graph-detail').getByText('biz_admin', { exact: true }).first().waitFor({ state: 'visible' });
  await page.locator('.taf-graph-action-rail button').filter({ hasText: label }).click();
  await page.waitForURL(expected);
  const rendered = await page.locator('.taf-page').first().isVisible().catch(() => false);
  const accessDenied = await page.locator('.taf-access-denied').isVisible().catch(() => false);
  const decodedTarget = decodeURIComponent(expectedTarget);
  const heading = (await page.locator('h1').first().textContent().catch(() => ''))?.trim() ?? '';
  const businessContext = await verifyDestinationContext(label, decodedTarget).catch(() => false);
  accountNavigation.push({
    label,
    passed: expected.test(new URL(page.url()).pathname + new URL(page.url()).search) && page.url().includes(expectedTarget) && rendered && !accessDenied && Boolean(heading) && businessContext,
    rendered,
    access_denied: accessDenied,
    heading,
    business_context: businessContext,
    url: redact(page.url()),
  });
  await page.goto(`${baseUrl}/graph?windowsCdpAccountReturnTs=${Date.now()}`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-graph-entity').waitFor({ state: 'visible' });
}
console.error('[entity-graph-cdp] account navigation verified');
await page.locator('.taf-graph-titlebar button').filter({ hasText: '定位中心节点' }).click();

await page.mouse.move(1000, 1000);
await page.waitForTimeout(350);
await page.screenshot({ path: screenshotPath, fullPage: false });
const businessRequestFailures = requestFailures.filter((item) => item.url.startsWith(baseUrl));
const knownExternalNoise = {
  request_failures: requestFailures.filter((item) => !item.url.startsWith(baseUrl)),
  console_errors: consoleErrors.filter((item) => item.includes('ERR_CONNECTION_CLOSED')),
  page_errors: pageErrors.filter((item) => item === 'Object'),
};
const unexpectedConsoleErrors = consoleErrors.filter((item) => !item.includes('ERR_CONNECTION_CLOSED'));
const unexpectedPageErrors = pageErrors.filter((item) => item !== 'Object');
const centerNode = graphData.nodes?.find((node) => node.entity_id === graphData.center_id) ?? graphData.nodes?.find((node) => node.entity_id === 'host:10.20.4.18');
const accountNode = graphData.nodes?.find((node) => node.entity_id === 'account:biz_admin');
const expectedCenterTraffic = centerNode?.metadata?.traffic_trend_24h ?? [];
const expectedCenterAlerts = centerNode?.metadata?.alert_trend_24h ?? [];
const expectedAccountTraffic = accountNode?.metadata?.traffic_trend_24h ?? [];
const expectedAccountAlerts = accountNode?.metadata?.alert_trend_24h ?? [];
const checks = {
  source_is_nebula_graph: graphMeta.source === 'nebula_graph',
  node_count: graphMeta.node_count === 11 && graphData.nodes?.length === 11,
  edge_count: graphMeta.edge_count === 13 && graphData.edges?.length === 13,
  graph_title_has_no_breadcrumb: graphTitleOnly,
  open_services_complete: ['TCP/22', 'TCP/80', 'TCP/443', 'TCP/3306'].every((service) => openServices.includes(service)),
  entity_detail_sections_complete: ['标签', '开放服务', '最近24小时流量', '相关告警', '最近活跃时间'].every((label) => entityDetailSections.includes(label)),
  entity_metric_trends_are_echarts: entityMetricEcharts === 2,
  entity_risk_score_is_echarts_ring: riskScoreEcharts === 1,
  entity_chart_types_values_and_sources_match_nebula: initialEntityChartContract.risk.type === 'gauge'
    && initialEntityChartContract.risk.value === centerNode?.risk_score
    && initialEntityChartContract.risk.color === '#ff4d4f'
    && initialEntityChartContract.trends.length === 2
    && initialEntityChartContract.trends.every((trend) => trend.type === 'line' && trend.source?.startsWith('nebula_graph:metadata.'))
    && JSON.stringify(initialEntityChartContract.trends[0]?.values) === JSON.stringify(expectedCenterTraffic)
    && JSON.stringify(initialEntityChartContract.trends[1]?.values) === JSON.stringify(expectedCenterAlerts),
  zero_alert_series_is_flat_and_not_synthetic: accountEntityChartContract.risk.type === 'gauge'
    && accountEntityChartContract.risk.value === accountNode?.risk_score
    && accountEntityChartContract.risk.color === '#ffb020'
    && JSON.stringify(accountEntityChartContract.trends[0]?.values) === JSON.stringify(expectedAccountTraffic)
    && JSON.stringify(accountEntityChartContract.trends[1]?.values) === JSON.stringify(expectedAccountAlerts)
    && accountEntityChartContract.trends[1]?.values.length > 1
    && accountEntityChartContract.trends[1]?.values.every((value) => value === 0),
  entity_detail_time_range_label_tracks_filter: timeRangeLabelTracksFilter,
  selected_account_services_complete: ['RDP/3389', 'SMB/445'].every((service) => selectedEntityServices.includes(service)),
  selected_account_services_stable_after_search: ['RDP/3389', 'SMB/445'].every((service) => selectedEntityServicesAfterSearch.includes(service)),
  query_governance_complete: queryGovernanceComplete
    && Number(graphMeta.node_limit) > 0
    && Number(graphMeta.query_duration_ms) >= 0
    && graphMeta.cache_applicable === false
    && graphMeta.cache_hit_rate === 'N/A'
    && graphMeta.data_origin === 'nebula_graph_persisted_projection',
  topology_is_echarts_canvas: topologyCanvasCount === 1 && Boolean(topologyCanvasBox?.width && topologyCanvasBox?.height),
  topology_arrows_share_visible_circle_centres: topologyNodeAnchorAlignment.nodes === graphData.nodes?.length && topologyNodeAnchorAlignment.max_center_delta_px <= 1,
  topology_zoom_controls_work: topologyZoomControlsWork,
  topology_relation_labels_use_normal_horizontal_font: topologyRelationLabels.length === graphData.edges?.length
    && topologyRelationLabels.every((label) => label.text && label.font_style === 'normal' && label.font_stretch === '100%' && label.font_size === '11px'),
  selected_node_changed: selectedNodeChanged,
  search_filter_count: searchFilterApplied,
  backend_entity_filter: accountFilterPayload?.data?.meta?.node_count === 2 && accountFilterPayload?.data?.meta?.edge_count === 1,
  saved_view_persisted: savedView?.center_id === 'host:10.20.4.18',
  export_download: download.suggestedFilename().endsWith('.json'),
  path_panel_visible: pathPanelVisible,
  path_panel_uses_larger_left_column: Number(initialPathPanelBox?.width ?? 0) >= 760
    && Number(initialPathPanelBox?.width ?? 0) > Number(governancePanelBox?.width ?? 0)
    && Number(initialPathPanelBox?.height ?? 0) >= 330
    && Number(initialPathPanelBox?.height ?? 0) <= 350
    && Math.abs(Number(initialPathPanelBox?.y ?? 0) - Number(governancePanelBox?.y ?? 0)) <= 1
    && Number(governancePanelBox?.x ?? 0) > Number(initialPathPanelBox?.x ?? 0),
  query_governance_is_fully_contained: governanceLayout.body_scroll_height <= governanceLayout.body_client_height
    && governanceLayout.governance_scroll_height <= governanceLayout.governance_client_height
    && governanceLayout.children_inside_panel,
  four_path_tabs_stay_in_fixed_panel: pathTabChecks.length === 4 && pathTabChecks.every((item) => item.panel_stable && item.inside_panel),
  four_path_tabs_have_distinct_real_relations: new Set(pathTabChecks.map((item) => item.actual_relation)).size === 4
    && pathTabChecks.every((item) => item.actual_relation === item.relation && item.actual_length >= item.min_length),
  four_path_tabs_have_mode_specific_real_fields: pathTabChecks.every((item) => item.required_attribute_value !== undefined && item.required_attribute_value !== null && item.required_attribute_value !== ''),
  four_path_tabs_have_distinct_ui_contracts: new Set(pathTabChecks.map((item) => item.ui_class)).size === 4,
  four_path_middle_visuals_are_data_backed_and_contained: pathTabChecks.every((item) => item.middle_visual_inside_panel
    && item.middle_node_count === item.actual_length + 1
    && item.middle_connector_count === item.actual_length),
  four_path_mode_actions_work: pathModeActionNavigation.length === pathModeActions.length && pathModeActionNavigation.every((item) => item.passed),
  topology_reset_keeps_single_canvas: topologyResetKeptSingleCanvas,
  detail_close_and_restore: detailClosed && detailRestored && !expandedGridColumns.includes('320px'),
  destinations_work: navigation.every((item) => item.passed),
  account_destinations_use_stable_business_target: accountNavigation.length === accountDestinations.length && accountNavigation.every((item) => item.passed),
  no_business_bad_responses: badResponses.length === 0,
  no_business_request_failures: businessRequestFailures.length === 0,
  no_unexpected_console_errors: unexpectedConsoleErrors.length === 0,
  no_unexpected_page_errors: unexpectedPageErrors.length === 0,
  no_horizontal_overflow: await page.evaluate(() => document.documentElement.scrollWidth === document.documentElement.clientWidth),
};
const result = {
  result: Object.values(checks).every(Boolean) ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP over Xshell 9224',
  browser: version.Browser,
  route: redact(routeUrl.toString()),
  checks,
  graph: { source: graphMeta.source, node_count: graphMeta.node_count, edge_count: graphMeta.edge_count },
  path_tabs: pathTabChecks,
  lower_workspace_layout: {
    path_panel_box: initialPathPanelBox,
    governance_panel_box: governancePanelBox,
    left_to_right_width_ratio: Number(initialPathPanelBox?.width ?? 0) / Number(governancePanelBox?.width ?? 1),
  },
  governance_layout: governanceLayout,
  path_mode_action_navigation: pathModeActionNavigation,
  topology: {
    canvas_count: topologyCanvasCount,
    reset_canvas_count: await page.locator('.taf-graph-echart canvas').count(),
    node_anchor_alignment: topologyNodeAnchorAlignment,
    zoom: { initial: initialTopologyZoom, enlarged: enlargedTopologyZoom, reset: resetTopologyZoom },
    relation_labels: topologyRelationLabels,
  },
  detail_trends: {
    sparkline_echarts_canvas_count: entityMetricEcharts,
    risk_ring_echarts_canvas_count: riskScoreEcharts,
    initial_entity_chart_contract: initialEntityChartContract,
    account_entity_chart_contract: accountEntityChartContract,
  },
  saved_view: savedView,
  download: download.suggestedFilename(),
  navigation,
  account_navigation: accountNavigation,
  bad_responses: badResponses,
  business_request_failures: businessRequestFailures,
  unexpected_console_errors: unexpectedConsoleErrors,
  unexpected_page_errors: unexpectedPageErrors,
  known_external_noise: knownExternalNoise,
  screenshot: path.relative(root, screenshotPath),
  selected_account_detail_screenshot: path.relative(root, accountDetailScreenshotPath),
  generated_at: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.mkdirSync(path.dirname(screenshotPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`, 'utf8');
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);

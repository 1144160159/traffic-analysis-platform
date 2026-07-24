#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const requireFromUi = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = requireFromUi('@playwright/test');
const cdpUrl = process.env.BASELINE_CDP_URL || 'http://127.0.0.1:9224';
const baseUrl = process.env.BASELINE_BASE_URL || 'http://10.0.5.8:30180';
const runId = process.env.BASELINE_RUN_ID || 'baseline-windows-xshell-r657';
const evidenceDir = path.join(root, process.env.BASELINE_EVIDENCE_DIR || 'evidence/learning/baselines/20260723-windows-xshell-r657');
const output = path.join(root, 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-baselines-latest.json');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8,10.0.5.9';

function token() {
  const encoded = execFileSync('kubectl', [
    '--server=https://127.0.0.1:6443', '--insecure-skip-tls-verify=true',
    '-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}',
  ], { encoding: 'utf8', timeout: 15_000, env: process.env });
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  const now = Math.floor(Date.now() / 1000);
  const header = { alg: 'HS256', typ: 'JWT' };
  const claims = {
    iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(),
    tenant_id: 'default', username: 'codex-baseline-windows-cdp', email: 'codex-baseline-windows-cdp@local',
    roles: ['admin'], permissions: ['*', 'alert:read', 'alert:write', 'audit:read', 'screen:view'],
    token_type: 'access', session_id: `baseline-cdp-${crypto.randomUUID()}`, iat: now, exp: now + 1800,
  };
  const b64 = (value) => Buffer.from(JSON.stringify(value)).toString('base64url');
  const input = `${b64(header)}.${b64(claims)}`;
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const preflight = await Promise.all([fetch(`${cdpUrl}/json/version`), fetch(`${cdpUrl}/json/list`)]);
if (preflight.some((response) => !response.ok)) throw new Error('Xshell Windows Chrome 9224 tunnel is unavailable');

const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
for (const existing of context.pages()) {
  if (existing !== page && existing.url().includes('/baselines')) await existing.close();
}
const session = await context.newCDPSession(page);
const checks = [];
const states = [];
const runtimeErrors = [];
page.on('response', (response) => {
  if (response.url().includes('/api/v1/baselines') && response.status() >= 400) runtimeErrors.push({ type: 'response', status: response.status(), url: response.url() });
});
page.on('requestfailed', (request) => {
  if (request.url().includes('/api/v1/baselines')) runtimeErrors.push({ type: 'requestfailed', url: request.url(), error: request.failure()?.errorText });
});
page.on('pageerror', (error) => runtimeErrors.push({ type: 'pageerror', error: error.message, stack: error.stack }));
page.on('console', (entry) => {
  if (entry.type() === 'error') runtimeErrors.push({ type: 'console', error: entry.text(), location: entry.location() });
});

async function viewport(width, height) {
  await session.send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile: false });
  await session.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
  await page.evaluate(() => { document.documentElement.style.zoom = '1'; });
  await page.waitForTimeout(250);
}

async function metrics() {
  return page.evaluate(() => {
    const rect = (selector) => {
      const element = document.querySelector(selector);
      if (!element) return null;
      const box = element.getBoundingClientRect();
      return { x: box.x, y: box.y, width: box.width, height: box.height, right: box.right, bottom: box.bottom };
    };
    const root = document.scrollingElement || document.documentElement;
    const workbench = rect('.taf-baseline-workbench');
    const left = rect('.taf-baseline-left');
    const right = rect('.taf-baseline-detail');
    const titleTabs = rect('.taf-baseline-title-tabs');
    const filters = rect('.taf-baseline-filter');
    const kpis = rect('.taf-baseline-kpis');
    const upper = rect('.taf-baseline-upper');
    const lower = rect('.taf-baseline-lower');
    const tertiary = rect('.taf-baseline-tertiary');
    const kpiBoxes = Array.from(document.querySelectorAll('.taf-baseline-kpi')).map((element) => {
      const box = element.getBoundingClientRect();
      return { y: box.y, bottom: box.bottom, height: box.height };
    });
    const modal = rect('.taf-baseline-workbench .ant-modal');
    const columnsNonOverlap = !left || !right || (
      right.x >= left.right + 6 ||
      right.y >= left.bottom + 6
    );
    const workbenchInsideViewport = !workbench || (
      workbench.x >= -2 &&
      workbench.right <= innerWidth + 2
    );
    const bandSelectors = [
      '.taf-baseline-title-tabs',
      '.taf-baseline-filter',
      '.taf-baseline-kpis',
      '.taf-baseline-upper',
      '.taf-baseline-lower',
      '.taf-baseline-tertiary',
      '.taf-baseline-evidence-shortcuts',
    ];
    const bands = bandSelectors.map((selector) => ({ selector, box: rect(selector) })).filter((item) => item.box);
    const bandOrder = bands.every((item, index) => index === 0 || bands[index - 1].box.bottom <= item.box.y + 1);
    const bandInsideLeft = bands.every((item) => !left || (
      item.box.x >= left.x - 2 &&
      item.box.right <= left.right + 2
    ));
    const panelGroups = ['.taf-baseline-upper', '.taf-baseline-lower', '.taf-baseline-tertiary'];
    const panelContainment = panelGroups.flatMap((selector) => {
      const group = document.querySelector(selector);
      const groupBox = group?.getBoundingClientRect();
      return Array.from(group?.children ?? []).filter((element) => element.classList.contains('taf-panel')).map((panel) => {
        const panelBox = panel.getBoundingClientRect();
        const body = panel.querySelector(':scope > .taf-panel__body');
        const bodyBox = body?.getBoundingClientRect();
        const style = getComputedStyle(panel);
        const groupStyle = group ? getComputedStyle(group) : null;
        const bodyStyle = body ? getComputedStyle(body) : null;
        return {
          group: selector,
          panel: { x: panelBox.x, y: panelBox.y, right: panelBox.right, bottom: panelBox.bottom, width: panelBox.width, height: panelBox.height },
          computed: {
            height: style.height,
            min_height: style.minHeight,
            max_height: style.maxHeight,
            box_sizing: style.boxSizing,
            display: style.display,
            align_self: style.alignSelf,
            group_height: groupStyle?.height,
            group_grid_columns: groupStyle?.gridTemplateColumns,
            group_grid_rows: groupStyle?.gridTemplateRows,
          },
          inside_group: Boolean(groupBox &&
            panelBox.x >= groupBox.x - 2 &&
            panelBox.y >= groupBox.y - 2 &&
            panelBox.right <= groupBox.right + 2 &&
            panelBox.bottom <= groupBox.bottom + 2),
          clips_paint: ['hidden', 'clip'].includes(style.overflowX) && ['hidden', 'clip'].includes(style.overflowY),
          body_clips_paint: Boolean(bodyStyle && ['hidden', 'clip'].includes(bodyStyle.overflowX) && ['hidden', 'clip'].includes(bodyStyle.overflowY)),
          body_inside_panel: Boolean(bodyBox &&
            bodyBox.x >= panelBox.x - 2 &&
            bodyBox.y >= panelBox.y - 2 &&
            bodyBox.right <= panelBox.right + 2 &&
            bodyBox.bottom <= panelBox.bottom + 2),
        };
      });
    });
    const stateMachine = document.querySelector('.taf-baseline-state-machine');
    const stateMachineFit = !stateMachine || stateMachine.scrollHeight <= stateMachine.clientHeight + 2;
    const chartContainment = Array.from(document.querySelectorAll('[data-chart-engine="echarts"]')).map((chart) => {
      const chartBox = chart.getBoundingClientRect();
      const panelBodyBox = chart.closest('.taf-panel__body')?.getBoundingClientRect();
      return {
        type: chart.getAttribute('data-series-type'),
        inside_panel_body: Boolean(panelBodyBox &&
          chartBox.x >= panelBodyBox.x - 2 &&
          chartBox.y >= panelBodyBox.y - 2 &&
          chartBox.right <= panelBodyBox.right + 2 &&
          chartBox.bottom <= panelBodyBox.bottom + 2),
      };
    });
    const lowerPanels = Array.from(document.querySelectorAll('.taf-baseline-lower > .taf-panel')).map((panel) => {
      const panelBox = panel.getBoundingClientRect();
      const visibleItems = Array.from(panel.querySelectorAll('.taf-baseline-dense-head > *, .taf-baseline-dense-table > button > *, .taf-baseline-version-head > *, .taf-baseline-version-row > *, .taf-baseline-insight-row, .taf-baseline-mini-stat, .taf-baseline-discovery-cards article, .taf-baseline-portrait-grid article, .taf-baseline-protocol-mirrors article, .taf-baseline-time-profiles article, .taf-baseline-periodic, .taf-baseline-matrix > div, .taf-panel__extra button')).filter((element) => {
        const style = getComputedStyle(element);
        const box = element.getBoundingClientRect();
        return style.visibility !== 'hidden' && style.display !== 'none' && box.width > 1 && box.height > 1;
      });
      return {
        panel: { x: panelBox.x, right: panelBox.right, width: panelBox.width },
        item_count: visibleItems.length,
        inside_left: Boolean(left && panelBox.x >= left.x - 2 && panelBox.right <= left.right + 2),
        separated_from_right: Boolean(!right || panelBox.right <= right.x - 6),
        contained: visibleItems.every((element) => {
          const box = element.getBoundingClientRect();
          return box.x >= panelBox.x - 2 && box.right <= panelBox.right + 2;
        }),
      };
    });
    const chartTypes = Array.from(document.querySelectorAll('[data-chart-engine="echarts"]')).map((element) => element.getAttribute('data-series-type'));
    const buttons = Array.from(document.querySelectorAll('.taf-baseline-workbench button')).filter((element) => {
      const box = element.getBoundingClientRect();
      return box.width > 1 && box.height > 1;
    });
    return {
      viewport: { width: innerWidth, height: innerHeight },
      workbench, left, right, title_tabs: titleTabs, filters, kpis, upper, lower, tertiary, kpi_boxes: kpiBoxes,
      layout_contract: {
        bands,
        band_order: bandOrder,
        bands_inside_left: bandInsideLeft,
        columns_non_overlap: columnsNonOverlap,
        workbench_inside_viewport: workbenchInsideViewport,
        panels: panelContainment,
        panels_contained: panelContainment.every((panel) => panel.inside_group && panel.clips_paint && panel.body_clips_paint && panel.body_inside_panel),
        state_machine_fit: stateMachineFit,
        charts: chartContainment,
        charts_contained: chartContainment.every((chart) => chart.inside_panel_body),
      },
      kpis_before_upper: !upper || kpiBoxes.every((box) => box.bottom <= upper.y + 2),
      lower_panels: lowerPanels,
      tertiary_panel_count: document.querySelectorAll('.taf-baseline-tertiary > .taf-panel').length,
      tertiary_type: document.querySelector('.taf-baseline-tertiary')?.getAttribute('data-baseline-tertiary') ?? null,
      charts: chartTypes,
      tab_count: document.querySelectorAll('.taf-baseline-title-tabs .ant-tabs-tab').length,
      board_type: document.querySelector('[data-baseline-board]')?.getAttribute('data-baseline-board'),
      filter_count: document.querySelectorAll('.taf-baseline-filter label').length,
      kpi_count: document.querySelectorAll('.taf-baseline-kpi').length,
      horizontal_overflow: root.scrollWidth > root.clientWidth + 2,
      modal_contained: !modal || Boolean(workbench && modal.x >= workbench.x && modal.right <= workbench.right + 2 && modal.y >= workbench.y && modal.bottom <= workbench.bottom + 2),
      visible_disabled_buttons: buttons.filter((button) => button.disabled).map((button) => button.textContent?.trim()).filter(Boolean),
      error_alerts: Array.from(document.querySelectorAll('.ant-alert-error')).map((element) => element.textContent?.replace(/\s+/g, ' ').trim()).filter(Boolean),
    };
  });
}

async function capture(id, width, height) {
  await viewport(width, height);
  await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  const state = await metrics();
  const result = await session.send('Page.captureScreenshot', { format: 'png', fromSurface: true });
  const file = path.join(evidenceDir, `${id}-${width}x${height}.png`);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, Buffer.from(result.data, 'base64'));
  states.push({ id, screenshot: path.relative(root, file), metrics: state });
  return state;
}

await viewport(1920, 1080);
const url = new URL('/baselines', baseUrl);
url.hash = new URLSearchParams({ codex_smoke_token: token() }).toString();
await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 30_000 });
await page.locator('.taf-baseline-workbench').waitFor({ state: 'visible', timeout: 30_000 });
await page.locator('[data-chart-engine="echarts"]').first().waitFor({ state: 'visible', timeout: 60_000 });
await page.waitForFunction(() => {
  const boxplot = document.querySelector('[data-series-type="boxplot"]');
  if (!boxplot) return false;
  try { return JSON.parse(boxplot.getAttribute('data-series-values') || '[]').length > 0; } catch { return false; }
}, undefined, { timeout: 30_000 });
await page.waitForTimeout(1200);

const desktop = await capture('baseline-default', 1920, 1080);
checks.push({ id: 'target-structure', pass: desktop.tab_count === 5 && desktop.filter_count === 5 && desktop.kpi_count === 5 && desktop.left && desktop.right && desktop.right.y <= desktop.left.y + 2, detail: desktop });
checks.push({ id: 'real-echarts', pass: ['boxplot', 'scatter', 'line'].every((type) => desktop.charts.includes(type)), detail: desktop.charts });
checks.push({ id: 'desktop-no-horizontal-overflow', pass: !desktop.horizontal_overflow, detail: desktop.horizontal_overflow });
checks.push({ id: 'desktop-no-api-error', pass: desktop.error_alerts.length === 0, detail: desktop.error_alerts });

const tabCases = [
  { label: '账号基线', slug: 'account', kpis: 8, filters: 6, charts: ['boxplot', 'graph'], lowerPanels: 3 },
  { label: '端口基线', slug: 'port', kpis: 8, filters: 6, charts: ['heatmap', 'multi-line'], lowerPanels: 3, tertiaryPanels: 2 },
  { label: '协议基线', slug: 'protocol', kpis: 8, filters: 6, charts: ['pie', 'multi-line'], lowerPanels: 3 },
  { label: '时间段基线', slug: 'time', kpis: 8, filters: 6, charts: ['heatmap', 'heatmap'], lowerPanels: 3 },
];
for (const tabCase of tabCases) {
  await page.getByRole('tab', { name: tabCase.label }).click();
  await page.locator(`[data-baseline-board="${tabCase.slug}"]`).waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForFunction(({ chartTypes }) => {
    const actual = Array.from(document.querySelectorAll('[data-chart-engine="echarts"]')).map((element) => element.getAttribute('data-series-type'));
    return chartTypes.every((type, index) => actual.filter((value) => value === type).length >= chartTypes.slice(0, index + 1).filter((value) => value === type).length);
  }, { chartTypes: tabCase.charts }, { timeout: 45_000 }).catch(() => {});
  await page.waitForTimeout(550);
  const state = await capture(`baseline-${tabCase.slug}`, 1920, 1080);
  const active = await page.getByRole('tab', { name: tabCase.label }).getAttribute('aria-selected') === 'true';
  const chartContract = tabCase.charts.every((type, index) => state.charts.filter((value) => value === type).length >= tabCase.charts.slice(0, index + 1).filter((value) => value === type).length);
  checks.push({ id: `tab-${tabCase.slug}-distinct-board`, pass: active && state.board_type === tabCase.slug && state.kpi_count === tabCase.kpis && state.filter_count === tabCase.filters && chartContract && state.lower_panels.length === tabCase.lowerPanels && state.tertiary_panel_count === (tabCase.tertiaryPanels ?? 3) && state.tertiary_type === tabCase.slug, detail: state });
  checks.push({ id: `tab-${tabCase.slug}-desktop-contained`, pass: !state.horizontal_overflow && state.lower_panels.every((panel) => panel.inside_left && panel.separated_from_right && panel.contained && panel.item_count > 0), detail: state.lower_panels });
  checks.push({ id: `tab-${tabCase.slug}-desktop-no-panel-overlap`, pass: state.layout_contract.band_order && state.layout_contract.bands_inside_left && state.layout_contract.panels_contained && state.layout_contract.state_machine_fit && state.layout_contract.charts_contained, detail: state.layout_contract });
  const compactTab = await capture(`baseline-${tabCase.slug}-compact`, 1600, 900);
  checks.push({ id: `tab-${tabCase.slug}-compact-contained`, pass: !compactTab.horizontal_overflow && compactTab.kpis_before_upper && compactTab.board_type === tabCase.slug && compactTab.charts.length >= 2 && compactTab.lower_panels.every((panel) => panel.inside_left && panel.separated_from_right && panel.contained && panel.item_count > 0), detail: compactTab });
  checks.push({ id: `tab-${tabCase.slug}-compact-no-panel-overlap`, pass: compactTab.layout_contract.band_order && compactTab.layout_contract.bands_inside_left && compactTab.layout_contract.panels_contained && compactTab.layout_contract.state_machine_fit && compactTab.layout_contract.charts_contained, detail: compactTab.layout_contract });
}

const responsiveSizes = [
  { width: 2560, height: 1440 },
  { width: 2048, height: 1152 },
  { width: 1920, height: 1080 },
  { width: 1728, height: 972 },
  { width: 1600, height: 900 },
  { width: 1440, height: 900 },
  { width: 1366, height: 768 },
  { width: 1280, height: 720 },
];
for (const size of responsiveSizes) {
  const samples = [];
  for (const tabCase of tabCases) {
    await page.getByRole('tab', { name: tabCase.label }).click();
    await page.locator(`[data-baseline-board="${tabCase.slug}"]`).waitFor({ state: 'visible', timeout: 20_000 });
    await viewport(size.width, size.height);
    const sample = await metrics();
    const contract = sample.layout_contract;
    const pass = !sample.horizontal_overflow &&
      contract.band_order &&
      contract.bands_inside_left &&
      contract.columns_non_overlap &&
      contract.workbench_inside_viewport &&
      contract.panels_contained &&
      contract.state_machine_fit &&
      contract.charts_contained;
    samples.push({
      tab: tabCase.slug,
      pass,
      viewport: sample.viewport,
      workbench: sample.workbench,
      left: sample.left,
      right: sample.right,
      horizontal_overflow: sample.horizontal_overflow,
      layout_contract: contract,
    });
  }
  checks.push({
    id: `responsive-${size.width}x${size.height}`,
    pass: samples.every((sample) => sample.pass),
    detail: samples,
  });
}
const responsiveRuntimeErrors = runtimeErrors.filter((error) => error.type === 'response' || (error.type === 'requestfailed' && error.error !== 'net::ERR_ABORTED') || (error.type === 'console' && error.location?.url?.startsWith(baseUrl)) || (error.type === 'pageerror' && error.stack?.includes(new URL(baseUrl).host)));
checks.push({ id: 'responsive-resize-no-runtime-error', pass: responsiveRuntimeErrors.length === 0, detail: responsiveRuntimeErrors });
runtimeErrors.length = 0;

await viewport(1920, 1080);
await page.getByRole('tab', { name: '资产基线' }).click();
await page.waitForTimeout(900);
// Tab switches intentionally cancel the previous specialist analytics request.
// Start the mutation check after that navigation cancellation has settled.
runtimeErrors.length = 0;

const compactAssetLayout = await capture('baseline-asset-compact-layout', 1600, 900);
checks.push({
  id: 'asset-compact-no-panel-overlap',
  pass: !compactAssetLayout.horizontal_overflow &&
    compactAssetLayout.layout_contract.band_order &&
    compactAssetLayout.layout_contract.bands_inside_left &&
    compactAssetLayout.layout_contract.columns_non_overlap &&
    compactAssetLayout.layout_contract.workbench_inside_viewport &&
    compactAssetLayout.layout_contract.panels_contained &&
    compactAssetLayout.layout_contract.charts_contained,
  detail: compactAssetLayout.layout_contract,
});
await viewport(1920, 1080);
runtimeErrors.length = 0;

await page.locator('.taf-baseline-explain-actions').getByRole('button', { name: '调整阈值' }).click({ timeout: 10_000 });
await page.getByText('影响范围').waitFor({ state: 'visible' });
const modalState = await capture('baseline-threshold-modal', 1920, 1080);
checks.push({ id: 'threshold-modal-contained', pass: modalState.modal_contained, detail: modalState.modal });
const compactModal = await capture('baseline-threshold-modal', 1600, 900);
checks.push({ id: 'compact-threshold-modal-contained', pass: compactModal.modal_contained, detail: compactModal.modal });
await page.getByLabel('操作原因').fill('Windows Chrome 验收：验证阈值治理写后刷新');
await page.locator('.ant-modal-footer .ant-btn-primary').click();
await page.locator('.ant-modal').waitFor({ state: 'hidden', timeout: 15_000 });
await page.locator('.taf-baseline-workbench > .ant-modal-root .ant-modal-wrap').waitFor({ state: 'hidden', timeout: 15_000 });
await page.waitForTimeout(600);
const productRuntimeErrors = runtimeErrors.filter((error) => error.type === 'response' || (error.type === 'requestfailed' && error.error !== 'net::ERR_ABORTED') || (error.type === 'console' && error.location?.url?.startsWith(baseUrl)) || (error.type === 'pageerror' && error.stack?.includes(new URL(baseUrl).host)));
checks.push({ id: 'threshold-submit-no-runtime-error', pass: productRuntimeErrors.length === 0, detail: { product: productRuntimeErrors, ambient_browser: runtimeErrors.filter((error) => !productRuntimeErrors.includes(error)) } });

const compact = await capture('baseline-compact', 1600, 900);
checks.push({ id: 'compact-no-horizontal-overflow', pass: !compact.horizontal_overflow, detail: compact });
checks.push({ id: 'compact-core-visible', pass: compact.kpi_count === 5 && ['boxplot', 'scatter', 'line'].every((type) => compact.charts.includes(type)), detail: compact });
checks.push({ id: 'compact-lower-panels-contained', pass: compact.lower_panels.length === 2 && compact.lower_panels.every((panel) => panel.contained && panel.inside_left && panel.separated_from_right && panel.item_count > 0), detail: compact.lower_panels });

await viewport(1920, 1080);
const objectSelector = page.locator('.taf-baseline-filter .ant-select').first();
await objectSelector.click();
await objectSelector.locator('input').fill('10.0.5.8');
await page.locator('.ant-select-dropdown:visible .ant-select-item-option-content').filter({ hasText: /^10\.0\.5\.8$/ }).click();
await page.waitForFunction(() => document.querySelectorAll('.taf-baseline-action-log > span > .ant-tag').length >= 2, undefined, { timeout: 15_000 });
const persistedActionMetrics = await page.evaluate(() => {
  const panel = document.querySelector('.taf-baseline-action-log');
  const panelBox = panel?.getBoundingClientRect();
  return Array.from(document.querySelectorAll('.taf-baseline-action-log > span > .ant-tag')).map((tag) => {
    const box = tag.getBoundingClientRect();
    const style = getComputedStyle(tag);
    return {
      text: tag.textContent?.replace(/\s+/g, '').trim(),
      x: box.x, y: box.y, width: box.width, height: box.height,
      writing_mode: style.writingMode,
      white_space: style.whiteSpace,
      inside_panel: Boolean(panelBox && box.x >= panelBox.x - 2 && box.right <= panelBox.right + 2 && box.y >= panelBox.y - 2 && box.bottom <= panelBox.bottom + 2),
      single_line: box.width > box.height && box.height <= 28,
    };
  });
});
await capture('baseline-persisted-actions', 1920, 1080);
checks.push({ id: 'persisted-action-status-tags-readable', pass: persistedActionMetrics.length >= 2 && persistedActionMetrics.every((tag) => tag.text && tag.inside_panel && tag.single_line && tag.writing_mode === 'horizontal-tb' && tag.white_space === 'nowrap'), detail: persistedActionMetrics });

await viewport(1600, 900);
await page.evaluate(() => {
  const root = document.querySelector('.taf-main') || document.scrollingElement || document.documentElement;
  root.scrollTop = root.scrollHeight;
});
await page.waitForTimeout(350);
const compactPersistedActions = await capture('baseline-persisted-actions-compact-scrolled', 1600, 900);
const compactActionTags = await page.evaluate(() => Array.from(document.querySelectorAll('.taf-baseline-action-log > span > .ant-tag')).map((tag) => {
  const box = tag.getBoundingClientRect();
  const panelBox = tag.closest('.taf-baseline-action-log')?.getBoundingClientRect();
  const bottomBarTop = document.querySelector('.taf-bottombar')?.getBoundingClientRect().top ?? innerHeight;
  const style = getComputedStyle(tag);
  return { text: tag.textContent?.replace(/\s+/g, '').trim(), width: box.width, height: box.height, writing_mode: style.writingMode, white_space: style.whiteSpace, visible_in_viewport: box.bottom > 0 && box.y < innerHeight, above_bottom_bar: box.bottom <= bottomBarTop, inside_panel: Boolean(panelBox && box.x >= panelBox.x - 2 && box.right <= panelBox.right + 2 && box.y >= panelBox.y - 2 && box.bottom <= panelBox.bottom + 2) };
}));
checks.push({ id: 'compact-persisted-action-status-tags-readable', pass: !compactPersistedActions.horizontal_overflow && compactActionTags.length >= 2 && compactActionTags.every((tag) => tag.text && tag.visible_in_viewport && tag.above_bottom_bar && tag.inside_panel && tag.width > tag.height && tag.height <= 28 && tag.writing_mode === 'horizontal-tb' && tag.white_space === 'nowrap'), detail: compactActionTags });

const result = {
  run_id: runId,
  timestamp: new Date().toISOString(),
  cdp: { url: cdpUrl, browser: (await preflight[0].json()).Browser },
  result: checks.every((check) => check.pass) ? 'pass' : 'fail',
  checks,
  states,
};
fs.mkdirSync(path.dirname(output), { recursive: true });
fs.writeFileSync(output, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify({ result: result.result, output: path.relative(root, output), checks: checks.length, failed: checks.filter((check) => !check.pass).map((check) => check.id) }, null, 2));
await page.close();
await browser.close();
if (result.result !== 'pass') process.exitCode = 1;

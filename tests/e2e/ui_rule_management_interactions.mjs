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
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/rules');
const outputPath = path.join(evidenceDir, 'interaction-r260-rules-lifecycle-typography.json');
const screenshotPaths = {
  overview: path.join(evidenceDir, 'interaction-r260-overview.png'),
  validation: path.join(evidenceDir, 'interaction-r260-test-validation-embedded.png'),
  dependencies: path.join(evidenceDir, 'interaction-r260-dependencies-embedded.png'),
  pcap: path.join(evidenceDir, 'interaction-r260-sample-pcap-embedded.png'),
  session: path.join(evidenceDir, 'interaction-r260-sample-session-embedded.png'),
  logs: path.join(evidenceDir, 'interaction-r260-sample-logs-embedded.png'),
};
const screenshotOptions = {
  fullPage: false,
  scale: 'css',
  clip: { x: 0, y: 0, width: 1920, height: 1080 },
};

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
fs.mkdirSync(evidenceDir, { recursive: true });

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
    permissions: ['*', 'admin:*', 'rule:read', 'rule:write', 'rule:enable'],
    token_type: 'access',
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

function redact(value) {
  return String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
}

function isExternalNoise(url) {
  return url.includes('api.yhchj.com/ip') || url.startsWith('chrome-extension://');
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const cdp = await context.newCDPSession(page);
await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
await cdp.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
page.setDefaultTimeout(12_000);

const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
const actionMutationRequests = [];
const ignoredExternalFailures = [];
page.on('response', (response) => {
  if (response.status() < 400) return;
  const item = { status: response.status(), url: redact(response.url()) };
  if (isExternalNoise(response.url())) ignoredExternalFailures.push(item);
  else badResponses.push(item);
});
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const location = entry.location().url ?? '';
  const item = { text: entry.text(), location: redact(location) };
  if (isExternalNoise(location) || entry.text().includes('api.yhchj.com/ip')) ignoredExternalFailures.push(item);
  else consoleErrors.push(item);
});
page.on('pageerror', (error) => {
  if (error.message === 'Object') ignoredExternalFailures.push({ page_error: error.message });
  else pageErrors.push(error.message);
});
page.on('requestfailed', (request) => {
  const item = `${request.method()} ${redact(request.url())} ${request.failure()?.errorText ?? ''}`;
  if (isExternalNoise(request.url())) ignoredExternalFailures.push(item);
  else requestFailures.push(item);
});
page.on('request', (request) => {
  if (/\/api\/v1\/rules\/[^/]+\/actions(?:\?|$)/.test(request.url())) actionMutationRequests.push(`${request.method()} ${redact(request.url())}`);
});

const routeUrl = new URL(`/rules?__codex_ui_breakdown_production=1&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-rules').waitFor({ state: 'visible', timeout: 15_000 });
await page.bringToFront();
await page.keyboard.press('Control+0').catch(() => {});
await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
await cdp.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
await page.waitForTimeout(150);
const firstViewport = await page.evaluate(() => ({ width: innerWidth, height: innerHeight }));
const zoomCalibration = 1920 / firstViewport.width;
if (firstViewport.width !== 1920 || firstViewport.height !== 1080) {
  await cdp.send('Emulation.setDeviceMetricsOverride', { width: Math.round(1920 * zoomCalibration), height: Math.round(1080 * zoomCalibration), deviceScaleFactor: 1 / zoomCalibration, mobile: false });
  await page.waitForTimeout(150);
}
await page.locator('.taf-rules-performance-echart canvas').first().waitFor({ state: 'visible', timeout: 10_000 });

const performanceCanvasCount = await page.locator('.taf-rules-performance-echart canvas').count();
const captureViewport = async (targetPath) => {
  const capture = await cdp.send('Page.captureScreenshot', {
    format: 'png',
    fromSurface: true,
    captureBeyondViewport: false,
    clip: { x: 0, y: 0, width: screenshotOptions.clip.width, height: screenshotOptions.clip.height, scale: 0.9 },
  });
  fs.writeFileSync(targetPath, Buffer.from(capture.data, 'base64'));
};
const originalModuleGeometry = await page.evaluate(() => {
  const rect = (selector) => {
    const box = document.querySelector(selector)?.getBoundingClientRect();
    return box ? { x: box.x, y: box.y, width: box.width, height: box.height } : null;
  };
  return {
    editor: rect('.taf-rules-editor-panel'),
    list: rect('.taf-rules-list-panel'),
    rail: rect('.taf-rules-rail'),
    samples: rect('.taf-rules-bottom > .taf-panel:first-child'),
    bottom: rect('.taf-rules-bottom'),
  };
});
const overviewLayout = await page.evaluate(() => {
  const listPanel = document.querySelector('.taf-rules-list-panel');
  const tableContent = document.querySelector('.taf-rules-list-panel .ant-table-content');
  const table = document.querySelector('.taf-rules-list-panel table');
  const pagination = document.querySelector('.taf-rules-pagination');
  const bottom = document.querySelector('.taf-rules-bottom');
  const rail = document.querySelector('.taf-rules-rail');
  const railPanels = [...document.querySelectorAll('.taf-rules-rail > .taf-panel')];
  const releaseButtons = [...document.querySelectorAll('.taf-rules-rail > .taf-panel:nth-child(4) button')].filter((item) => {
    const rect = item.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0 && getComputedStyle(item).visibility !== 'hidden';
  });
  const performancePanel = document.querySelector('.taf-rules-bottom > .taf-panel:nth-child(4)');
  const versionPanel = document.querySelector('.taf-rules-rail > .taf-panel:nth-child(2)');
  const versionMore = document.querySelector('.taf-rules-version .ant-btn');
  const performanceCards = [...document.querySelectorAll('.taf-rules-bottom > .taf-panel:nth-child(4) .taf-rules-performance > div')];
  const lifecycleStages = [...document.querySelectorAll('.taf-rules-lifecycle-stage')];
  const lifecycleConnectors = [...document.querySelectorAll('.taf-rules-lifecycle-connector')];
  const lifecycleCurrent = [...document.querySelectorAll('.taf-rules-lifecycle-segment.is-current')];
  const lifecycleRoot = document.querySelector('.taf-rules-lifecycle');
  const conditionRows = [...document.querySelectorAll('.taf-rules-condition-row')];
  const conditionSelects = [...document.querySelectorAll('.taf-rules-condition-row .ant-select')];
  const exceptionSelects = [...document.querySelectorAll('.taf-rules-exception .ant-select')];
  const allDefinitionSelects = [...conditionSelects, ...exceptionSelects];
  const dslEditor = document.querySelector('.taf-rules-dsl-editor');
  const mitreTag = document.querySelector('.taf-rules-mitre > span');
  const mitreTitle = document.querySelector('.taf-rules-mitre-title');
  const mitreDelete = document.querySelector('.taf-rules-mitre button[aria-label^="删除 MITRE"]');
  const mitreAdd = document.querySelector('.taf-rules-mitre > .ant-btn');
  const kpis = [...document.querySelectorAll('.taf-rules-kpis > .taf-metric')];
  const kpiIcons = [...document.querySelectorAll('.taf-rules-kpis .taf-metric__icon')];
  const kpiIconSvgs = [...document.querySelectorAll('.taf-rules-kpis .taf-metric__icon svg')];
  const panelRect = listPanel?.getBoundingClientRect();
  const tableRect = table?.getBoundingClientRect();
  const bottomRect = bottom?.getBoundingClientRect();
  const railRect = rail?.getBoundingClientRect();
  const performanceRect = performancePanel?.getBoundingClientRect();
  const inside = (outer, inner) => Boolean(outer && inner && inner.left >= outer.left - 1 && inner.right <= outer.right + 1 && inner.top >= outer.top - 1 && inner.bottom <= outer.bottom + 1);
  return {
    list_panel_contains_table: Boolean(panelRect && tableRect && tableRect.left >= panelRect.left - 1 && tableRect.right <= panelRect.right + 1),
    table_has_no_horizontal_overflow: Boolean(tableContent && tableContent.scrollWidth <= tableContent.clientWidth + 1),
    table_overflow_x: tableContent ? getComputedStyle(tableContent).overflowX : 'missing',
    pagination_top: pagination?.getBoundingClientRect().top ?? -1,
    bottom_visible: Boolean(bottomRect && bottomRect.top < innerHeight && bottomRect.bottom <= innerHeight + 1),
    rail_panels_visible: Boolean(railRect && railPanels.length === 4 && railPanels.every((item) => inside(railRect, item.getBoundingClientRect()))),
    release_control_visible: releaseButtons.length >= 5 && releaseButtons.every((item) => inside(railRect, item.getBoundingClientRect())),
    release_button_geometry: releaseButtons.map((item) => ({ text: item.textContent?.trim() ?? '', ...(() => { const rect = item.getBoundingClientRect(); return { left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom }; })() })),
    performance_cards_visible: Boolean(performanceRect && performanceCards.length === 4 && performanceCards.every((item) => inside(performanceRect, item.getBoundingClientRect()))),
    version_history_complete: Boolean(versionPanel && versionMore && inside(versionPanel.getBoundingClientRect(), versionMore.getBoundingClientRect())),
    lifecycle_matches_reference: lifecycleStages.length === 6 && lifecycleConnectors.length === 5 && lifecycleCurrent.length === 1 && lifecycleRoot?.dataset.currentStatus === lifecycleCurrent[0]?.dataset.stage && document.querySelector('.taf-rules-lifecycle-meta')?.textContent?.includes('操作人') && lifecycleConnectors.every((connector, index) => { const rail = connector.getBoundingClientRect(); const lines = [...connector.querySelectorAll('.taf-rules-lifecycle-line')].map((item) => item.getBoundingClientRect()); const arrow = connector.querySelector('.anticon-arrow-right')?.getBoundingClientRect(); const currentCircle = lifecycleStages[index]?.querySelector('i')?.getBoundingClientRect(); const nextCircle = lifecycleStages[index + 1]?.querySelector('i')?.getBoundingClientRect(); const lineCenter = lines[0] ? lines[0].top + lines[0].height / 2 : -1; return Boolean(currentCircle && nextCircle && arrow && lines.length === 2 && getComputedStyle(connector.querySelector('.taf-rules-lifecycle-line')).backgroundColor !== 'rgba(0, 0, 0, 0)' && Math.abs(lineCenter - (currentCircle.top + currentCircle.height / 2)) <= 2 && Math.abs(lineCenter - (nextCircle.top + nextCircle.height / 2)) <= 2 && Math.abs(rail.left - currentCircle.right) <= 2 && Math.abs(rail.right - nextCircle.left) <= 2 && Math.abs((arrow.left + arrow.width / 2) - (rail.left + rail.width / 2)) <= 1.5 && lines[0].left <= currentCircle.right + 2 && lines[1].right >= nextCircle.left - 2 && rail.width > 20); }),
    lifecycle_connector_geometry: lifecycleConnectors.map((connector, index) => { const box = (node) => { const rect = node?.getBoundingClientRect(); return rect ? { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom, width: rect.width, height: rect.height } : null; }; const lines = [...connector.querySelectorAll('.taf-rules-lifecycle-line')]; const arrow = connector.querySelector('.anticon-arrow-right'); return { rail: box(connector), lines: lines.map(box), arrow: box(arrow), current_circle: box(lifecycleStages[index]?.querySelector('i')), next_circle: box(lifecycleStages[index + 1]?.querySelector('i')), background: lines[0] ? getComputedStyle(lines[0]).backgroundColor : 'missing' }; }),
    definition_controls_match_reference: conditionRows.length === 4 && conditionSelects.length === 12 && exceptionSelects.length === 3 && [...conditionRows, document.querySelector('.taf-rules-exception')].filter(Boolean).every((row) => { const rect = row.getBoundingClientRect(); return [...row.querySelectorAll('.ant-select')].every((item) => { const itemRect = item.getBoundingClientRect(); return itemRect.left >= rect.left - 1 && itemRect.right <= rect.right + 1 && itemRect.top >= rect.top - 1 && itemRect.bottom <= rect.bottom + 1; }); }) && allDefinitionSelects.every((select) => { const borderRect = select.getBoundingClientRect(); const arrowRect = select.querySelector('.ant-select-arrow')?.getBoundingClientRect(); const text = select.querySelector('.ant-select-selection-item'); return Boolean(arrowRect && text && select.getAttribute('title')?.trim() && getComputedStyle(select).borderRightWidth !== '0px' && arrowRect.left >= borderRect.left + 1 && arrowRect.right <= borderRect.right - 1 && text.scrollWidth <= text.clientWidth + 8 && text.textContent?.trim()); }),
    definition_select_text_geometry: allDefinitionSelects.map((select) => { const text = select.querySelector('.ant-select-selection-item'); return { text: text?.textContent?.trim() ?? '', client_width: text?.clientWidth ?? 0, scroll_width: text?.scrollWidth ?? 0, font_size: text ? getComputedStyle(text).fontSize : 'missing' }; }),
    dsl_and_mitre_match_reference: Boolean(dslEditor && !dslEditor.readOnly && !dslEditor.disabled && mitreTitle?.textContent?.includes('MITRE 阶段（点击删除）') && mitreTag?.textContent?.includes('TA0011 指挥与控制') && mitreDelete && mitreAdd?.textContent?.includes('添加阶段')),
    kpi_strip_matches_reference: kpis.length === 6 && kpis.every((item, index) => index === kpis.length - 1 || Math.abs(item.getBoundingClientRect().right - kpis[index + 1].getBoundingClientRect().left) <= 1),
    kpi_icon_geometry: kpiIcons.map((item, index) => { const rect = item.getBoundingClientRect(); const svgRect = kpiIconSvgs[index]?.getBoundingClientRect(); return { container_width: rect.width, container_height: rect.height, svg_width: svgRect?.width ?? 0, svg_height: svgRect?.height ?? 0 }; }),
    kpi_icons_enlarged: kpiIcons.length === 6 && kpiIconSvgs.length === 6 && kpiIconSvgs.every((item) => { const rect = item.getBoundingClientRect(); return rect.width >= 44 && rect.height >= 44; }),
    document_has_no_horizontal_overflow: document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
  };
});
await captureViewport(screenshotPaths.overview);

const dslLocator = page.getByRole('textbox', { name: 'DSL 表达式编辑器' });
const dslBefore = await dslLocator.inputValue();
await dslLocator.fill(`${dslBefore}\n# editable-check`);
const dslEditable = (await dslLocator.inputValue()).endsWith('# editable-check');
await dslLocator.fill(dslBefore);
await page.getByRole('button', { name: '删除 MITRE 阶段 TA0011' }).click();
const mitreActionDrawer = page.locator('.ant-drawer-content-wrapper:visible');
await mitreActionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const mitreRemoved = await page.getByText('尚未选择 MITRE 阶段', { exact: true }).isVisible();
await mitreActionDrawer.locator('.ant-drawer-close').click();
await mitreActionDrawer.waitFor({ state: 'hidden', timeout: 5_000 });
await page.getByRole('button', { name: '添加阶段', exact: true }).click();
await mitreActionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const mitreRestored = await page.getByText('TA0011 指挥与控制', { exact: true }).isVisible();
await mitreActionDrawer.locator('.ant-drawer-close').click();
await mitreActionDrawer.waitFor({ state: 'hidden', timeout: 5_000 });
const definitionInteraction = { dsl_editable: dslEditable, mitre_deletable: mitreRemoved, mitre_addable: mitreRestored };

const lifecycleBeforeSelection = await page.locator('.taf-rules-lifecycle').getAttribute('data-current-status');
const grayRuleRow = page.locator('.taf-rules-list-panel .ant-table-tbody > tr').filter({ hasText: '灰度' }).first();
const grayRuleAvailable = await grayRuleRow.count() === 1;
if (grayRuleAvailable) await grayRuleRow.click();
const lifecycleAfterGraySelection = await page.locator('.taf-rules-lifecycle').getAttribute('data-current-status');
const grayStageSelected = await page.locator('.taf-rules-lifecycle-segment.is-current').getAttribute('data-stage');
const firstRuleRow = page.locator('.taf-rules-list-panel .ant-table-tbody > tr').filter({ hasText: 'C2_Tunnel_v3' }).first();
await firstRuleRow.click();
const lifecycleAfterRestore = await page.locator('.taf-rules-lifecycle').getAttribute('data-current-status');
const lifecycleInteraction = {
  initial_status: lifecycleBeforeSelection,
  gray_rule_available: grayRuleAvailable,
  selected_gray_status: lifecycleAfterGraySelection,
  selected_gray_stage: grayStageSelected,
  restored_status: lifecycleAfterRestore,
  api_selection_dynamic: grayRuleAvailable && lifecycleAfterGraySelection === '灰度' && grayStageSelected === '灰度' && lifecycleBeforeSelection !== lifecycleAfterGraySelection && lifecycleAfterRestore === lifecycleBeforeSelection,
};

await page.getByRole('button', { name: '规则第 2 页' }).click();
const listPageTwo = await page.locator('.taf-rules-pagination button.is-active').textContent();
const paginationTopAfter = await page.locator('.taf-rules-pagination').evaluate((node) => node.getBoundingClientRect().top);
const paginationStable = Math.abs(paginationTopAfter - overviewLayout.pagination_top) <= 1;
await page.getByRole('button', { name: '规则第 1 页' }).click();

const actionDrawer = page.locator('.ant-drawer-content-wrapper:visible');
await page.getByRole('button', { name: '新建规则' }).click();
await actionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const createActionVisible = await actionDrawer.isVisible() && await actionDrawer.getByRole('button', { name: '视觉模式不可提交' }).isDisabled();
await actionDrawer.locator('.ant-drawer-close').click();
await actionDrawer.waitFor({ state: 'hidden', timeout: 5_000 });

await page.getByRole('button', { name: '测试验证', exact: true }).click();
await page.locator('.taf-rules-editor-panel .taf-rules-hit-diff canvas').waitFor({ state: 'visible' });
const validationState = {
  active: await page.getByRole('button', { name: '测试验证', exact: true }).evaluate((button) => button.classList.contains('is-active')),
  hit_chart_visible: await page.locator('.taf-rules-editor-panel .taf-rules-hit-diff canvas').isVisible(),
  performance_charts: await page.locator('.taf-rules-editor-panel .taf-rules-test-performance canvas').count(),
  result_rows: await page.locator('.taf-rules-editor-panel .taf-rules-validation-scroll > div').count(),
  result_scrollbar: await page.locator('.taf-rules-editor-panel .taf-rules-validation-scroll').evaluate((node) => ['auto', 'scroll'].includes(getComputedStyle(node).overflowY)),
  layout: await page.evaluate(() => {
    const panel = document.querySelector('.taf-rules-editor-panel');
    const content = document.querySelector('.taf-rules-editor-panel .taf-rules-test-validation');
    const parts = [...document.querySelectorAll('.taf-rules-editor-panel .taf-rules-test-toolbar, .taf-rules-editor-panel .taf-rules-test-upper, .taf-rules-editor-panel .taf-rules-validation-results, .taf-rules-editor-panel .taf-rules-focus-actions')];
    const panelRect = panel?.getBoundingClientRect();
    return {
      no_horizontal_overflow: Boolean(content && content.scrollWidth <= content.clientWidth + 1),
      no_vertical_overflow: Boolean(content && content.scrollHeight <= content.clientHeight + 1),
      parts_inside_editor: Boolean(panelRect && parts.every((part) => {
        const rect = part.getBoundingClientRect();
        return rect.left >= panelRect.left - 1 && rect.right <= panelRect.right + 1 && rect.top >= panelRect.top - 1 && rect.bottom <= panelRect.bottom + 1;
      })),
      parts_do_not_overlap: parts.every((part, index) => index === 0 || part.getBoundingClientRect().top >= parts[index - 1].getBoundingClientRect().bottom - 1),
    };
  }),
};
await captureViewport(screenshotPaths.validation);
await page.getByRole('button', { name: '规则定义', exact: true }).click();

await page.getByRole('button', { name: '依赖引用', exact: true }).click();
await page.locator('.taf-rules-editor-panel .taf-rules-dependency-upper canvas').waitFor({ state: 'visible' });
const dependencyState = {
  active: await page.getByRole('button', { name: '依赖引用', exact: true }).evaluate((button) => button.classList.contains('is-active')),
  graph_visible: await page.locator('.taf-rules-editor-panel .taf-rules-dependency-upper canvas').isVisible(),
  impact_rows: await page.locator('.taf-rules-editor-panel .taf-rules-impact-list > div').count(),
  dependency_rows: await page.locator('.taf-rules-editor-panel .taf-rules-dependency-scroll > div').count(),
  dependency_scrollbar: await page.locator('.taf-rules-editor-panel .taf-rules-dependency-scroll').evaluate((node) => ['auto', 'scroll'].includes(getComputedStyle(node).overflowY)),
  layout: await page.evaluate(() => {
    const panel = document.querySelector('.taf-rules-editor-panel');
    const content = document.querySelector('.taf-rules-editor-panel .taf-rules-dependencies');
    const parts = [...document.querySelectorAll('.taf-rules-editor-panel .taf-rules-dependency-summary, .taf-rules-editor-panel .taf-rules-dependency-upper, .taf-rules-editor-panel .taf-rules-dependency-table, .taf-rules-editor-panel .taf-rules-focus-actions')];
    const panelRect = panel?.getBoundingClientRect();
    return {
      no_horizontal_overflow: Boolean(content && content.scrollWidth <= content.clientWidth + 1),
      no_vertical_overflow: Boolean(content && content.scrollHeight <= content.clientHeight + 1),
      parts_inside_editor: Boolean(panelRect && parts.every((part) => {
        const rect = part.getBoundingClientRect();
        return rect.left >= panelRect.left - 1 && rect.right <= panelRect.right + 1 && rect.top >= panelRect.top - 1 && rect.bottom <= panelRect.bottom + 1;
      })),
      parts_do_not_overlap: parts.every((part, index) => index === 0 || part.getBoundingClientRect().top >= parts[index - 1].getBoundingClientRect().bottom - 1),
    };
  }),
};
await captureViewport(screenshotPaths.dependencies);
const embeddedGeometry = await page.evaluate((originalModuleGeometry) => {
  const rect = (selector) => {
    const box = document.querySelector(selector)?.getBoundingClientRect();
    return box ? { x: box.x, y: box.y, width: box.width, height: box.height } : null;
  };
  const stable = (before, after) => before && after
    && Math.abs(before.x - after.x) <= 1
    && Math.abs(before.y - after.y) <= 1
    && Math.abs(before.width - after.width) <= 1
    && Math.abs(before.height - after.height) <= 1;
  const current = {
    editor: rect('.taf-rules-editor-panel'),
    list: rect('.taf-rules-list-panel'),
    rail: rect('.taf-rules-rail'),
    samples: rect('.taf-rules-bottom > .taf-panel:first-child'),
    bottom: rect('.taf-rules-bottom'),
  };
  return {
    current,
    editor_stable: stable(originalModuleGeometry.editor, current.editor),
    list_stable: stable(originalModuleGeometry.list, current.list),
    rail_stable: stable(originalModuleGeometry.rail, current.rail),
    samples_stable: stable(originalModuleGeometry.samples, current.samples),
    bottom_stable: stable(originalModuleGeometry.bottom, current.bottom),
    no_focus_workspace: !document.querySelector('.taf-rules-focus-panel'),
  };
}, originalModuleGeometry);
await page.getByRole('button', { name: '规则定义', exact: true }).click();

await page.getByRole('button', { name: 'PCAP 样本 32', exact: true }).click();
await page.locator('.taf-rules-samples.is-pcap').waitFor({ state: 'visible' });
const sampleState = {
  pcap_rows: await page.locator('.taf-rules-samples.is-pcap .taf-rules-sample-row').count(),
  pcap_headers: await page.locator('.taf-rules-samples.is-pcap .taf-rules-sample-head > span').count(),
  footer_visible: await page.getByRole('button', { name: '查看全部样本 >', exact: true }).isVisible(),
  pcap_layout: await page.locator('.taf-rules-samples.is-pcap').evaluate((node) => ({ no_horizontal_overflow: node.scrollWidth <= node.clientWidth + 1, no_vertical_overflow: node.scrollHeight <= node.clientHeight + 1, cells_do_not_overlap: [...node.querySelectorAll('.taf-rules-sample-row')].every((row) => { const cells = [...row.children].map((cell) => cell.getBoundingClientRect()); return cells.every((cell, index) => index === cells.length - 1 || cell.right <= cells[index + 1].left + 1); }) })),
  table_font_reduced: await page.locator('.taf-rules-samples.is-pcap').evaluate((node) => [...node.querySelectorAll('.taf-rules-sample-tabs button, .taf-rules-sample-head, .taf-rules-sample-row')].every((item) => Number.parseFloat(getComputedStyle(item).fontSize) <= 7 && getComputedStyle(item).transform === 'none')),
  title_font_reduced: await page.locator('.taf-rules-bottom > .taf-panel:first-child > .taf-panel__header h2').evaluate((node) => Number.parseFloat(getComputedStyle(node).fontSize) <= 8),
  table_font_sizes: await page.locator('.taf-rules-samples.is-pcap').evaluate((node) => [...node.querySelectorAll('.taf-rules-sample-head, .taf-rules-sample-row')].map((item) => getComputedStyle(item).fontSize)),
};
await captureViewport(screenshotPaths.pcap);
await page.getByRole('button', { name: 'Session 样本 128', exact: true }).click();
await page.locator('.taf-rules-samples.is-session').waitFor({ state: 'visible' });
sampleState.session_rows = await page.locator('.taf-rules-samples.is-session .taf-rules-sample-row').count();
sampleState.session_content = await page.getByText('TLS / JA3命中', { exact: true }).isVisible();
sampleState.session_headers = await page.locator('.taf-rules-samples.is-session .taf-rules-sample-head > span').count();
sampleState.session_field_tags = await page.locator('.taf-rules-samples.is-session .taf-rules-sample-tags > i').count();
sampleState.session_fields_inside_boxes = await page.locator('.taf-rules-samples.is-session').evaluate((node) => [...node.querySelectorAll('.taf-rules-sample-tags > i')].every((item) => { const tag = item.getBoundingClientRect(); const parent = item.parentElement?.getBoundingClientRect(); return Boolean(parent && tag.left >= parent.left - 1 && tag.right <= parent.right + 1 && getComputedStyle(item).overflow === 'hidden'); }));
sampleState.session_actions = await page.locator('.taf-rules-samples.is-session .taf-rules-sample-actions .ant-btn').count();
sampleState.session_layout = await page.locator('.taf-rules-samples.is-session').evaluate((node) => ({ no_horizontal_overflow: node.scrollWidth <= node.clientWidth + 1, no_vertical_overflow: node.scrollHeight <= node.clientHeight + 1, cells_do_not_overlap: [...node.querySelectorAll('.taf-rules-sample-row')].every((row) => { const cells = [...row.children].map((cell) => cell.getBoundingClientRect()); return cells.every((cell, index) => index === cells.length - 1 || cell.right <= cells[index + 1].left + 1); }) }));
await captureViewport(screenshotPaths.session);
await page.getByRole('button', { name: '日志样本 256', exact: true }).click();
await page.locator('.taf-rules-samples.is-logs').waitFor({ state: 'visible' });
sampleState.log_rows = await page.locator('.taf-rules-samples.is-logs .taf-rules-sample-row').count();
sampleState.log_content = await page.getByText('C2端口命中', { exact: true }).isVisible();
sampleState.false_positive_switches = await page.locator('.taf-rules-samples.is-logs .ant-switch').count();
sampleState.log_headers = await page.locator('.taf-rules-samples.is-logs .taf-rules-sample-head > span').count();
sampleState.log_field_tags = await page.locator('.taf-rules-samples.is-logs .taf-rules-sample-tags > i').count();
sampleState.log_fields_inside_boxes = await page.locator('.taf-rules-samples.is-logs').evaluate((node) => [...node.querySelectorAll('.taf-rules-sample-tags > i')].every((item) => { const tag = item.getBoundingClientRect(); const parent = item.parentElement?.getBoundingClientRect(); return Boolean(parent && tag.left >= parent.left - 1 && tag.right <= parent.right + 1 && getComputedStyle(item).overflow === 'hidden'); }));
sampleState.log_actions = await page.locator('.taf-rules-samples.is-logs .taf-rules-sample-actions .ant-btn').count();
sampleState.log_layout = await page.locator('.taf-rules-samples.is-logs').evaluate((node) => ({ no_horizontal_overflow: node.scrollWidth <= node.clientWidth + 1, no_vertical_overflow: node.scrollHeight <= node.clientHeight + 1, cells_do_not_overlap: [...node.querySelectorAll('.taf-rules-sample-row')].every((row) => { const cells = [...row.children].map((cell) => cell.getBoundingClientRect()); return cells.every((cell, index) => index === cells.length - 1 || cell.right <= cells[index + 1].left + 1); }) }));
await captureViewport(screenshotPaths.logs);
await page.getByRole('button', { name: 'PCAP 样本 32', exact: true }).click();

await page.getByRole('button', { name: '全量发布', exact: true }).click();
await actionDrawer.waitFor({ state: 'visible', timeout: 5_000 });
const publishActionVisible = await actionDrawer.isVisible() && await actionDrawer.getByRole('button', { name: '视觉模式不可提交' }).isDisabled();
await actionDrawer.locator('.ant-drawer-close').click();
await actionDrawer.waitFor({ state: 'hidden', timeout: 5_000 });

const assertions = {
  overview_performance_charts: performanceCanvasCount === 4,
  list_page_two: listPageTwo === '2',
  pagination_stable: paginationStable,
  list_table_fits: overviewLayout.list_panel_contains_table && overviewLayout.table_has_no_horizontal_overflow && overviewLayout.document_has_no_horizontal_overflow,
  overview_bottom_visible: overviewLayout.bottom_visible && overviewLayout.rail_panels_visible && overviewLayout.release_control_visible && overviewLayout.performance_cards_visible && overviewLayout.version_history_complete && overviewLayout.lifecycle_matches_reference && overviewLayout.kpi_strip_matches_reference,
  rule_definition_and_lifecycle: overviewLayout.definition_controls_match_reference && overviewLayout.dsl_and_mitre_match_reference && overviewLayout.lifecycle_matches_reference && lifecycleInteraction.api_selection_dynamic && Object.values(definitionInteraction).every(Boolean),
  kpi_icons_enlarged: overviewLayout.kpi_icons_enlarged,
  create_action: createActionVisible,
  validation_view: validationState.active && validationState.hit_chart_visible && validationState.performance_charts === 4 && validationState.result_rows === 5 && validationState.result_scrollbar && Object.values(validationState.layout).every(Boolean),
  dependency_view: dependencyState.active && dependencyState.graph_visible && dependencyState.impact_rows === 4 && dependencyState.dependency_rows === 6 && dependencyState.dependency_scrollbar && Object.values(dependencyState.layout).every(Boolean),
  sample_views: sampleState.pcap_rows === 4 && sampleState.session_rows === 4 && sampleState.log_rows === 4 && sampleState.pcap_headers === 5 && sampleState.session_headers === 5 && sampleState.log_headers === 5 && sampleState.footer_visible && sampleState.table_font_reduced && sampleState.title_font_reduced && sampleState.session_content && sampleState.session_field_tags === 8 && sampleState.session_fields_inside_boxes && sampleState.session_actions === 8 && sampleState.log_content && sampleState.log_field_tags === 8 && sampleState.log_fields_inside_boxes && sampleState.log_actions === 8 && sampleState.false_positive_switches === 4 && Object.values(sampleState.pcap_layout).every(Boolean) && Object.values(sampleState.session_layout).every(Boolean) && Object.values(sampleState.log_layout).every(Boolean),
  tabs_stay_in_original_modules: embeddedGeometry.editor_stable && embeddedGeometry.list_stable && embeddedGeometry.rail_stable && embeddedGeometry.samples_stable && embeddedGeometry.bottom_stable && embeddedGeometry.no_focus_workspace,
  tabs_stay_on_original_route: await page.evaluate(() => location.pathname === '/rules' && !new URLSearchParams(location.search).has('view')),
  publish_action: publishActionVisible,
  visual_mode_has_zero_mutations: actionMutationRequests.length === 0,
  runtime_clean: badResponses.length === 0 && consoleErrors.length === 0 && pageErrors.length === 0 && requestFailures.length === 0,
};
const result = {
  result: Object.values(assertions).every(Boolean) ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  viewport: { width: 1920, height: 1080 },
  route: redact(routeUrl.toString()),
  assertions,
  overview_layout: overviewLayout,
  definition_interaction: definitionInteraction,
  lifecycle_interaction: lifecycleInteraction,
  pagination_top_after: paginationTopAfter,
  validation_state: validationState,
  dependency_state: dependencyState,
  original_module_geometry: originalModuleGeometry,
  embedded_geometry: embeddedGeometry,
  sample_state: sampleState,
  bad_responses: badResponses,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  request_failures: requestFailures,
  action_mutation_requests: actionMutationRequests,
  ignored_external_failures: ignoredExternalFailures,
  screenshots: Object.fromEntries(Object.entries(screenshotPaths).map(([key, value]) => [key, path.relative(root, value)])),
  timestamp: new Date().toISOString(),
};
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);

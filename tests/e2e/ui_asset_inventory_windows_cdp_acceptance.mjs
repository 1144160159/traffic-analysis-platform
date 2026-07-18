#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const requireFromUi = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = requireFromUi('@playwright/test');

const defaults = {
  baseUrl: 'http://10.0.5.8:30180',
  cdpUrl: 'http://127.0.0.1:9224',
  width: 1920,
  height: 1080,
  tenant: 'default',
  username: 'codex-windows-cdp-admin',
  evidenceDir: 'evidence/learning/asset-inventory/20260713-windows-xshell-acceptance-01',
  output: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-asset-interactions-latest.json',
  kubectlSshHost: '10.0.5.9',
  remoteKubeconfig: '/tmp/codex-assets10-kubeconfig',
};

const args = { ...defaults, ...parseArgs(process.argv.slice(2)) };
clearProxyEnv();

function parseArgs(argv) {
  const parsed = {};
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index];
    if (!item.startsWith('--')) throw new Error(`unexpected argument: ${item}`);
    const key = item.slice(2).replace(/-([a-z])/g, (_, char) => char.toUpperCase());
    const next = argv[index + 1];
    if (next === undefined || next.startsWith('--')) parsed[key] = true;
    else {
      parsed[key] = next;
      index += 1;
    }
  }
  return parsed;
}

function clearProxyEnv() {
  ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy'].forEach((key) => delete process.env[key]);
  process.env.NO_PROXY = process.env.NO_PROXY || '127.0.0.1,localhost,10.0.5.8';
}

function noProxyEnv() {
  const env = { ...process.env };
  ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy'].forEach((key) => delete env[key]);
  return env;
}

function b64url(value) {
  return Buffer.from(value).toString('base64url');
}

function makeJwt(secret) {
  const now = Math.floor(Date.now() / 1000);
  const header = { alg: 'HS256', typ: 'JWT' };
  const claims = {
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: String(args.tenant),
    username: String(args.username),
    email: `${args.username}@local`,
    roles: ['admin'],
    permissions: ['*', 'asset:read', 'graph:read', 'screen:view'],
    token_type: 'access',
    session_id: `asset-windows-cdp-${crypto.randomUUID()}`,
    iat: now,
    exp: now + 1800,
  };
  const signingInput = `${b64url(JSON.stringify(header))}.${b64url(JSON.stringify(claims))}`;
  const signature = crypto.createHmac('sha256', secret).update(signingInput).digest();
  return `${signingInput}.${b64url(signature)}`;
}

function loadSmokeToken() {
  const kubectlArgs = ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'];
  const encoded = args.kubectlSshHost
    ? execFileSync('ssh', [String(args.kubectlSshHost), `KUBECONFIG=${String(args.remoteKubeconfig)} kubectl ${kubectlArgs.join(' ')}`], { encoding: 'utf8', env: noProxyEnv(), timeout: 20_000 })
    : execFileSync('kubectl', kubectlArgs, { encoding: 'utf8', env: noProxyEnv(), timeout: 15_000 });
  return makeJwt(Buffer.from(encoded, 'base64').toString('utf8'));
}

function authenticatedUrl(route, token) {
  const url = new URL(route, `${String(args.baseUrl).replace(/\/+$/, '')}/`);
  url.hash = new URLSearchParams({ codex_smoke_token: token }).toString();
  return url.toString();
}

function absolute(file) {
  return path.isAbsolute(file) ? file : path.join(root, file);
}

function writeJson(file, value) {
  const destination = absolute(file);
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  fs.writeFileSync(destination, `${JSON.stringify(value, null, 2)}\n`, 'utf8');
}

async function cdpPreflight() {
  const base = String(args.cdpUrl).replace(/\/+$/, '');
  const [versionResponse, targetsResponse] = await Promise.all([
    fetch(`${base}/json/version`),
    fetch(`${base}/json/list`),
  ]);
  if (!versionResponse.ok || !targetsResponse.ok) {
    throw new Error(`Xshell Windows Chrome tunnel unavailable: version=${versionResponse.status}, targets=${targetsResponse.status}`);
  }
  return { version: await versionResponse.json(), targets: await targetsResponse.json() };
}

function addCheck(checks, id, pass, details = {}) {
  checks.push({ id, status: pass ? 'pass' : 'fail', ...details });
}

function detailDialogFitsViewport(metrics) {
  const rect = metrics.detail_workspace;
  const viewport = metrics.viewport;
  if (!rect || !viewport) return false;
  return rect.x >= viewport.width * 0.3
    && rect.y >= 0
    && rect.right <= viewport.width + 2
    && rect.bottom <= viewport.height + 2
    && rect.width <= viewport.width * 0.68
    && rect.height <= viewport.height;
}

async function waitForAssets(page) {
  await page.locator('.taf-asset-main .ant-table-tbody tr.ant-table-row').first().waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForFunction(() => !document.querySelector('.taf-asset-main .ant-spin-spinning'), null, { timeout: 20_000 }).catch(() => {});
}

async function layoutMetrics(page) {
  return page.evaluate(() => {
    const root = document.scrollingElement || document.documentElement;
    const rect = (selector) => {
      const element = document.querySelector(selector);
      if (!element) return null;
      const box = element.getBoundingClientRect();
      return { x: box.x, y: box.y, width: box.width, height: box.height, right: box.right, bottom: box.bottom };
    };
    const activeDetail = document.querySelector('[data-breakdown-page-id^="assets-detail-"]');
    const geometryRoot = activeDetail || document;
    const visibleBadGeometry = Array.from(geometryRoot.querySelectorAll('button, a, input, h1, h2, h3, .ant-table, .taf-panel'))
      .map((element) => {
        const box = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        return { text: element.textContent?.replace(/\s+/g, ' ').trim().slice(0, 60), x: box.x, y: box.y, width: box.width, height: box.height, display: style.display, visibility: style.visibility };
      })
      .filter((item) => item.display !== 'none' && item.visibility !== 'hidden' && item.width > 1 && item.height > 1 && (item.x < -2 || item.y < -2 || item.width > innerWidth + 4))
      .slice(0, 20);
    const surfaces = Array.from(document.querySelectorAll('.taf-asset-titlebar, .taf-asset-main > .taf-panel, .taf-asset-observability, .taf-asset-work-grid, .taf-asset-network-ledger-layout, .taf-asset-network-modules, .taf-asset-unknown-main-grid, .taf-asset-lower-grid, .taf-asset-evidence-panel, .taf-asset-summary-rail, .taf-asset-action-rail, .taf-asset-detail-workspace'))
      .map((element) => element.getBoundingClientRect())
      .filter((box) => box.width > 1 && box.height > 1);
    const contentTop = rect('.taf-asset-titlebar')?.y ?? 0;
    const contentBottom = Math.min(innerHeight, rect('.taf-asset-grid')?.bottom ?? innerHeight);
    const footer = rect('.taf-bottombar');
    const lowerGrid = rect('.taf-asset-lower-grid');
    const railEvidence = rect('.taf-asset-rail-evidence-grid');
    const evidencePanel = rect('.taf-asset-evidence-panel');
    const evidenceItem = rect('.taf-asset-main .taf-asset-evidence-panel .taf-asset-evidence-item');
    const categoryWorkspace = rect('.taf-asset-category-workspace');
    const ledgerPanel = rect('.taf-asset-ledger-panel');
    const summaryRail = rect('.taf-asset-summary-rail');
    const actionRail = rect('.taf-asset-action-rail');
    const lastAction = rect('.taf-asset-action-rail .ant-btn:last-child');
    const tableBodyElement = document.querySelector('.taf-asset-ledger-panel .ant-table-body');
    const tableContentElement = document.querySelector('.taf-asset-ledger-panel .ant-table-content');
    const paginationElement = document.querySelector('.taf-asset-ledger-panel .ant-pagination');
    const tableRows = Array.from(document.querySelectorAll('.taf-asset-ledger-panel .ant-table-tbody tr.ant-table-row'));
    const tableRowHeights = tableRows.map((row) => Number(row.getBoundingClientRect().height.toFixed(2)));
    const lastTableRow = tableRows.at(-1)?.getBoundingClientRect() ?? null;
    const foldBottom = footer?.y ?? innerHeight;
    const mainScrollElement = document.querySelector('.taf-main');
    const clippedContent = Array.from(document.querySelectorAll('.taf-asset-grid .taf-panel__body > *'))
      .map((element) => {
        const box = element.getBoundingClientRect();
        const body = element.parentElement?.getBoundingClientRect();
        return body ? { class_name: element.className, right_overflow: Number((box.right - body.right).toFixed(2)), bottom_overflow: Number((box.bottom - body.bottom).toFixed(2)) } : null;
      })
      .filter((item) => item && (item.right_overflow > 2 || item.bottom_overflow > 2));
    let sampled = 0;
    let covered = 0;
    for (let y = contentTop; y < contentBottom; y += 28) {
      for (let x = rect('.taf-page')?.x ?? 0; x < innerWidth; x += 32) {
        sampled += 1;
        if (surfaces.some((box) => x >= box.left && x <= box.right && y >= box.top && y <= box.bottom)) covered += 1;
      }
    }
    return {
      viewport: { width: innerWidth, height: innerHeight },
      document: { scroll_width: root.scrollWidth, client_width: root.clientWidth, scroll_height: root.scrollHeight, client_height: root.clientHeight },
      horizontal_overflow: root.scrollWidth > root.clientWidth + 2,
      page: rect('.taf-page'),
      grid: rect('.taf-asset-grid'),
      main: rect('.taf-asset-main'),
      rail: rect('.taf-asset-detail'),
      detail_workspace: activeDetail ? rect(`[data-breakdown-page-id="${activeDetail.getAttribute('data-breakdown-page-id')}"]`) : null,
      table: rect('.taf-asset-main .ant-table-wrapper'),
      ledger_contract: {
        panel: ledgerPanel,
        row_count: tableRows.length,
        row_heights: tableRowHeights,
        minimum_row_height: tableRowHeights.length ? Math.min(...tableRowHeights) : 0,
        has_internal_vertical_scroll: Boolean(tableBodyElement && tableBodyElement.scrollHeight > tableBodyElement.clientHeight + 2),
        has_internal_horizontal_scroll: Boolean(tableContentElement && tableContentElement.scrollWidth > tableContentElement.clientWidth + 2),
        table_body_scroll_height: tableBodyElement?.scrollHeight ?? null,
        table_body_client_height: tableBodyElement?.clientHeight ?? null,
        pagination_visible: Boolean(paginationElement && paginationElement.getBoundingClientRect().height > 0),
        pagination: paginationElement ? rect('.taf-asset-ledger-panel .ant-pagination') : null,
        last_row_bottom: lastTableRow?.bottom ?? null,
        last_row_visible: Boolean(lastTableRow && ledgerPanel && lastTableRow.bottom <= ledgerPanel.bottom + 1),
      },
      business_flow: {
        workspace: categoryWorkspace,
        evidence_panel: evidencePanel,
        evidence_item: evidenceItem,
        trailing_gap: categoryWorkspace && evidencePanel ? Number((categoryWorkspace.bottom - evidencePanel.bottom).toFixed(2)) : null,
        category_complete: Boolean(categoryWorkspace && evidencePanel && Math.abs(categoryWorkspace.bottom - evidencePanel.bottom) <= 2),
        evidence_rendered: Boolean(evidenceItem && evidenceItem.width >= 70 && evidenceItem.height >= 36),
        clipped_content: clippedContent,
      },
      above_fold: {
        fold_bottom: foldBottom,
        lower_grid: lowerGrid,
        rail_evidence: railEvidence,
        evidence_panel: evidencePanel,
        evidence_item: evidenceItem,
        business_bottom_gap: evidencePanel ? Number((foldBottom - evidencePanel.bottom).toFixed(2)) : null,
        category_complete: (!lowerGrid || lowerGrid.bottom <= foldBottom + 2) && Boolean(evidencePanel && evidencePanel.bottom <= foldBottom + 2 && evidencePanel.bottom >= foldBottom - 6),
        evidence_visible: Boolean(evidenceItem && evidenceItem.bottom <= foldBottom + 2),
      },
      bilateral_alignment: {
        summary_rail: summaryRail,
        action_rail: actionRail,
        last_action: lastAction,
        left_right_bottom_delta: evidencePanel && summaryRail ? Number((evidencePanel.bottom - summaryRail.bottom).toFixed(2)) : null,
        right_action_contained: Boolean(lastAction && actionRail && lastAction.bottom <= actionRail.bottom + 2),
        left_complete: Boolean(evidencePanel && evidenceItem && evidenceItem.bottom <= evidencePanel.bottom + 2),
        right_complete: Boolean(summaryRail && actionRail && lastAction && actionRail.bottom <= summaryRail.bottom + 2 && lastAction.bottom <= actionRail.bottom + 2),
        whole_region_scrollable: Boolean(mainScrollElement && mainScrollElement.scrollHeight > mainScrollElement.clientHeight + 2),
        scroll_top: mainScrollElement?.scrollTop ?? null,
        scroll_height: mainScrollElement?.scrollHeight ?? null,
        client_height: mainScrollElement?.clientHeight ?? null,
      },
      surface_fill_ratio: sampled ? Number((covered / sampled).toFixed(4)) : 0,
      visible_bad_geometry: visibleBadGeometry,
      error_alerts: Array.from(document.querySelectorAll('.ant-alert-error')).map((item) => item.textContent?.replace(/\s+/g, ' ').trim()).filter(Boolean),
    };
  });
}

async function capture(page, id, states, { enforceViewport = true } = {}) {
  const screenshot = path.join(String(args.evidenceDir), `${id}-1920x1080.png`);
  fs.mkdirSync(path.dirname(absolute(screenshot)), { recursive: true });
  // Playwright's screenshot helper reapplies its own viewport on a connected
  // browser and can reintroduce Chrome's per-origin 90% zoom. Capture through
  // the calibrated CDP session so the pixels and the measured CSS viewport use
  // the same contract.
  if (enforceViewport) await enforceCssViewport();
  await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  await page.waitForTimeout(300);
  const metrics = await layoutMetrics(page);
  // The first CDP frame can be an incompletely composited surface after a
  // viewport override or Drawer transition. Discard it and persist the stable frame.
  await cdpSession.send('Page.captureScreenshot', { format: 'png', fromSurface: true });
  await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  await page.waitForTimeout(120);
  const captured = await cdpSession.send('Page.captureScreenshot', { format: 'png', fromSurface: true });
  fs.writeFileSync(absolute(screenshot), Buffer.from(captured.data, 'base64'));
  states.push({ id, url: page.url().replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>'), screenshot, metrics });
  return metrics;
}

const tabCases = [
  { slug: 'endpoint', label: '终端', prefix: 'END-', expectedTotal: 10, captureId: 'assets', kpiCount: 5, requiredColumns: ['资产 ID', 'IP/MAC', '主机名', '操作系统', '风险标签'] },
  { slug: 'server', label: '服务器', prefix: 'SRV-', expectedTotal: 10, captureId: 'assets-server', kpiCount: 6, requiredColumns: ['资产 ID', '业务系统', '资产状态', '暴露端口', '高危服务'], minimumTopologyNodes: 5 },
  { slug: 'network-device', label: '网络设备', prefix: 'NET-', expectedTotal: 10, captureId: 'assets-network-device', kpiCount: 7, requiredColumns: ['资产 ID', '厂商', '管理IP', '设备角色', '接口数'], minimumTopologyNodes: 5 },
  { slug: 'business-system', label: '业务系统', prefix: 'BIZ-', expectedTotal: 10, captureId: 'assets-business-system', kpiCount: 7, requiredColumns: ['资产 ID', '业务域', '系统等级', '关键服务', '依赖资产', 'SLA'], minimumTopologyNodes: 5 },
  { slug: 'unknown', label: '未知资产', prefix: 'UNK-', minimumTotal: 10, captureId: 'assets-unknown', kpiCount: 7, requiredColumns: ['资产 ID', '来源', '疑似类型', '置信度', '首次发现', '工单状态'] },
];

const preflight = await cdpPreflight();
const token = loadSmokeToken();
const browser = await chromium.connectOverCDP(String(args.cdpUrl));
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const cdpSession = await context.newCDPSession(page);
await cdpSession.send('Emulation.setDeviceMetricsOverride', { width: Number(args.width), height: Number(args.height), deviceScaleFactor: 1, mobile: false });
await cdpSession.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });

async function enforceCssViewport() {
  // Chrome stores zoom per origin. Calibrating on about:blank can therefore
  // produce a false 1920x1080 pass followed by 2133x1200 after navigation.
  await page.bringToFront();
  await page.keyboard.press('Control+0').catch(() => {});
  await cdpSession.send('Emulation.setDeviceMetricsOverride', {
    width: Number(args.width),
    height: Number(args.height),
    deviceScaleFactor: 1,
    mobile: false,
  });
  await cdpSession.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
  await page.waitForTimeout(150);
  const observed = await page.evaluate(() => ({ width: innerWidth, height: innerHeight, dpr: devicePixelRatio }));
  const browserZoomFactor = Number(args.width) / observed.width;
  const calibrated = browserZoomFactor > 0.5 && browserZoomFactor < 1.5 ? browserZoomFactor : 1;
  if (observed.width !== Number(args.width) || observed.height !== Number(args.height)) {
    await cdpSession.send('Emulation.setDeviceMetricsOverride', {
      width: Math.round(Number(args.width) * calibrated),
      height: Math.round(Number(args.height) * calibrated),
      deviceScaleFactor: 1 / calibrated,
      mobile: false,
    });
    await page.waitForTimeout(150);
  }
  const enforced = await page.evaluate(() => ({ width: innerWidth, height: innerHeight, dpr: devicePixelRatio }));
  return { observed, browser_zoom_factor: calibrated, enforced };
}

const checks = [];
const states = [];
const badResponses = [];
const requestFailures = [];
const requestUrls = [];
const consoleErrors = [];
const pageErrors = [];
const externalExtensionErrors = [];
const topologyApiResponses = new Map();
let endpointPaginationY = null;
page.on('response', (response) => {
  if (response.status() >= 400 && (response.url().startsWith(String(args.baseUrl)) || response.url().includes('/api/'))) {
    badResponses.push({ status: response.status(), method: response.request().method(), url: response.url() });
  }
  const topologyMatch = response.url().match(/\/api\/v1\/assets\/([^/?]+)\/topology(?:\?|$)/);
  if (topologyMatch && response.status() === 200) {
    response.json().then((payload) => topologyApiResponses.set(decodeURIComponent(topologyMatch[1]), payload?.data ?? payload)).catch(() => {});
  }
});
page.on('requestfailed', (request) => requestFailures.push({ method: request.method(), url: request.url(), error: request.failure()?.errorText || '' }));
page.on('request', (request) => {
  if (request.url().includes('/api/')) requestUrls.push(request.url());
});
page.on('console', (message) => {
  if (message.type() === 'error') consoleErrors.push({ text: message.text().slice(0, 1000) });
});
page.on('pageerror', (error) => {
  const item = { message: error.message, stack: error.stack || '' };
  if (item.stack.includes('chrome-extension://')) externalExtensionErrors.push(item);
  else pageErrors.push(item);
});

try {
  await page.goto(authenticatedUrl('/assets?tab=endpoint', token), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await waitForAssets(page);
  const viewportCalibration = await enforceCssViewport();
  addCheck(
    checks,
    'css-viewport-1920x1080',
    viewportCalibration.enforced.width === Number(args.width) && viewportCalibration.enforced.height === Number(args.height),
    viewportCalibration,
  );
  addCheck(checks, 'windows-chrome-target', String(preflight.version['User-Agent'] || '').includes('Windows'), { browser: preflight.version.Browser, user_agent: preflight.version['User-Agent'] });

  for (const [index, tab] of tabCases.entries()) {
    if (index > 0) {
      const tabList = page.locator('.taf-asset-tabs');
      await tabList.getByRole('tab', { name: tab.label, exact: true }).click();
      await page.waitForURL((url) => url.searchParams.get('tab') === tab.slug, { timeout: 15_000 });
      await waitForAssets(page);
    }
    const rows = page.locator('.taf-asset-main .ant-table-tbody tr.ant-table-row');
    const rowCount = await rows.count();
    const firstCode = (await rows.first().locator('.taf-asset-id').textContent())?.trim() || '';
    const heading = (await page.locator('.taf-asset-main .taf-panel__header h2').filter({ hasText: '资产清单' }).first().textContent()) || '';
    const total = Number(heading.match(/共\s*(\d+)\s*条/)?.[1] || 0);
    const params = new URL(page.url()).searchParams;
    const canonicalId = params.get('assetId') || '';
    addCheck(checks, `tab-${tab.slug}-route`, params.get('tab') === tab.slug, { actual: params.get('tab') });
    const assetListRequest = requestUrls
      .filter((requestUrl) => {
        const parsed = new URL(requestUrl);
        return parsed.pathname.endsWith('/api/v1/assets') && parsed.searchParams.get('asset_type') === tab.slug;
      })
      .at(-1);
    const requestedPageSize = assetListRequest ? Number(new URL(assetListRequest).searchParams.get('page_size')) : 0;
    addCheck(checks, `tab-${tab.slug}-rows`, rowCount === Math.min(total, 10) && requestedPageSize >= 10, { row_count: rowCount, total, requested_page_size: requestedPageSize });
    addCheck(checks, `tab-${tab.slug}-identity`, firstCode.startsWith(tab.prefix) && /^[0-9a-f]{8}-[0-9a-f-]{27}$/i.test(canonicalId), { first_display_code: firstCode, canonical_asset_id: canonicalId });
    addCheck(checks, `tab-${tab.slug}-total`, tab.expectedTotal ? total === tab.expectedTotal : total >= tab.minimumTotal, { total });
    const iconContract = await page.evaluate(() => {
      const visibleIcon = (element) => {
        const box = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        return box.width >= 14 && box.height >= 14 && style.visibility !== 'hidden' && style.display !== 'none';
      };
      const kpiIcons = [...document.querySelectorAll('.taf-asset-kpis .taf-metric__icon')];
      const summaryIcons = [...document.querySelectorAll('.taf-asset-detail-card__icon .anticon')];
      const evidenceIcons = [...document.querySelectorAll('.taf-asset-rail-evidence-grid .anticon')];
      return {
        kpi_icons: kpiIcons.length,
        visible_kpi_icons: kpiIcons.filter(visibleIcon).length,
        summary_icons: summaryIcons.length,
        evidence_icons: evidenceIcons.length,
      };
    });
    addCheck(checks, `tab-${tab.slug}-icon-contract`, iconContract.kpi_icons === tab.kpiCount && iconContract.visible_kpi_icons === tab.kpiCount && iconContract.summary_icons >= 1 && iconContract.evidence_icons >= 6, { expected_kpi_icons: tab.kpiCount, ...iconContract });
    const presentationContract = await page.evaluate(({ slug, requiredColumns }) => {
      const headers = [...document.querySelectorAll('.taf-asset-ledger-panel .ant-table-thead th')].map((item) => item.textContent?.replace(/\s+/g, ' ').trim() || '');
      const count = (selector) => document.querySelectorAll(selector).length;
      const networkLedger = document.querySelector('.taf-asset-network-ledger-layout > .taf-asset-ledger-panel')?.getBoundingClientRect();
      const networkInterface = document.querySelector('.taf-asset-network-interface-panel')?.getBoundingClientRect();
      const metricRings = document.querySelector('.taf-asset-summary-rail .taf-asset-metric-rings');
      const protocolChart = document.querySelector('.taf-asset-protocol');
      const distributionCharts = [...document.querySelectorAll('.taf-asset-distribution')];
      const viewAllLabels = [...document.querySelectorAll('.taf-asset-inventory .taf-asset-panel-view-all')].map((item) => item.textContent?.replace(/\s+/g, ' ').trim() || '');
      return {
        active_presentation: document.querySelector('.taf-asset-inventory')?.getAttribute('data-asset-presentation'),
        headers,
        required_columns_present: requiredColumns.every((column) => headers.includes(column)),
        endpoint_panels: count('.taf-asset-observability > .taf-panel'),
        work_panels: count('.taf-asset-work-grid > .taf-panel'),
        lower_panels: count('.taf-asset-lower-grid > .taf-panel'),
        network_modules: count('.taf-asset-network-modules > .taf-panel'),
        unknown_main_panels: count('.taf-asset-unknown-main-grid > .taf-panel'),
        interface_cells: count('.taf-asset-interface-matrix__ports > span'),
        metric_ring_radius: metricRings?.getAttribute('data-ring-radius') || '',
        metric_ring_title_offset: metricRings?.getAttribute('data-title-offset') || '',
        protocol_legend_contract: !protocolChart || (protocolChart.getAttribute('data-chart-center') === '27%' && protocolChart.getAttribute('data-chart-radius') === '42%-64%' && protocolChart.getAttribute('data-legend-region') === 'right'),
        distribution_legend_contract: distributionCharts.every((chart) => chart.getAttribute('data-chart-center') === '21%' && chart.getAttribute('data-chart-radius') === '40%-58%' && chart.getAttribute('data-legend-region') === 'right' && Number(chart.getAttribute('data-legend-safe-gap') || 0) >= 12),
        evidence_chevrons: count('.taf-asset-evidence-item .taf-asset-item-chevron') + count('.taf-asset-rail-evidence-grid > button > .anticon:last-child'),
        evidence_buttons: count('button.taf-asset-evidence-item') + count('.taf-asset-rail-evidence-grid > button'),
        view_all_labels: viewAllLabels,
        network_horizontal: slug !== 'network-device' || Boolean(networkLedger && networkInterface && networkInterface.x > networkLedger.x && networkInterface.y >= networkLedger.y && networkInterface.bottom <= networkLedger.bottom + 2),
      };
    }, { slug: tab.slug, requiredColumns: tab.requiredColumns });
    const panelSignaturePass = tab.slug === 'endpoint'
      ? presentationContract.endpoint_panels === 4
      : tab.slug === 'server' || tab.slug === 'business-system'
        ? presentationContract.work_panels === 3 && presentationContract.lower_panels === 3
        : tab.slug === 'network-device'
          ? presentationContract.network_horizontal && presentationContract.interface_cells > 0 && presentationContract.network_modules === 4
          : presentationContract.unknown_main_panels === 4 && presentationContract.lower_panels === 3;
    const viewAllPass = tab.slug !== 'network-device' || ['查看全部接口', '查看全部镜像口', '查看全部变更记录', '查看全部业务影响', '查看全部证据'].every((label) => presentationContract.view_all_labels.includes(label));
    addCheck(checks, `tab-${tab.slug}-presentation-contract`, presentationContract.active_presentation === tab.slug && presentationContract.required_columns_present && presentationContract.metric_ring_radius === '50%' && presentationContract.metric_ring_title_offset === '145%' && presentationContract.protocol_legend_contract && presentationContract.distribution_legend_contract && presentationContract.evidence_chevrons >= 12 && presentationContract.evidence_buttons >= 12 && viewAllPass && panelSignaturePass, presentationContract);
    const removedCapabilityBoundary = await page.evaluate(() => ({
      nodes: document.querySelectorAll('.taf-asset-capability-boundary').length,
      stale_text: /分类、筛选、分页来自|详情与历史仅在服务器|分类画像、拓扑、服务与证据读取持久化/.test(document.body.innerText),
    }));
    addCheck(checks, `tab-${tab.slug}-capability-boundary-removed`, removedCapabilityBoundary.nodes === 0 && !removedCapabilityBoundary.stale_text, removedCapabilityBoundary);
    if (tab.slug === 'endpoint') {
      await page.waitForFunction(() => document.querySelectorAll('.taf-asset-observability canvas').length >= 2, null, { timeout: 15_000 });
      const chartContract = await page.evaluate(() => {
        const traffic = document.querySelector('.taf-asset-traffic');
        const protocol = document.querySelector('.taf-asset-protocol');
        const periodic = document.querySelector('.taf-asset-periodic-chart');
        const trafficCanvas = traffic?.querySelector('canvas')?.getBoundingClientRect();
        const protocolCanvas = protocol?.querySelector('canvas')?.getBoundingClientRect();
        const periodicCanvas = periodic?.querySelector('canvas')?.getBoundingClientRect();
        return {
          traffic_series: Number(traffic?.getAttribute('data-series-count') || 0),
          traffic_series_types: traffic?.getAttribute('data-series-types') || '',
          traffic_points: Number(traffic?.getAttribute('data-point-count') || 0),
          protocol_items: Number(protocol?.getAttribute('data-protocol-count') || 0),
          total_label: protocol?.getAttribute('data-total-label') || '',
          periodic_days: Number(periodic?.getAttribute('data-day-count') || 0),
          periodic_axis_fill: periodic?.getAttribute('data-y-axis-fill') || '',
          traffic_canvas: trafficCanvas ? { width: trafficCanvas.width, height: trafficCanvas.height } : null,
          protocol_canvas: protocolCanvas ? { width: protocolCanvas.width, height: protocolCanvas.height } : null,
          periodic_canvas: periodicCanvas ? { width: periodicCanvas.width, height: periodicCanvas.height } : null,
        };
      });
      addCheck(checks, 'tab-endpoint-echarts-content', chartContract.traffic_series === 3 && chartContract.traffic_series_types === 'line,line,line' && chartContract.traffic_points >= 12 && chartContract.protocol_items === 7 && chartContract.total_label === '68.4 Gbps' && chartContract.periodic_days === 7 && chartContract.periodic_axis_fill === 'monday-to-sunday' && (chartContract.traffic_canvas?.height || 0) >= 140 && (chartContract.protocol_canvas?.height || 0) >= 140 && (chartContract.periodic_canvas?.height || 0) >= 180, chartContract);
    }
    if (tab.minimumTopologyNodes) {
      const topology = page.locator('.taf-asset-topology');
      await page.waitForFunction(() => {
        const source = document.querySelector('.taf-asset-topology')?.getAttribute('data-api-source');
        return Boolean(source && !['loading', 'error', 'empty'].includes(source));
      }, null, { timeout: 15_000 });
      await page.waitForTimeout(200);
      const apiGraph = topologyApiResponses.get(canonicalId);
      const nodeCount = Number(await topology.getAttribute('data-node-count'));
      const edgeCount = Number(await topology.getAttribute('data-edge-count'));
      const svgNodeIDs = await topology.locator('svg [data-node-id]').evaluateAll((items) => items.map((item) => item.getAttribute('data-node-id')).filter(Boolean).sort());
      const svgNodeIconCount = await topology.locator('svg [data-node-icon]').count();
      const svgEdges = await topology.locator('svg .taf-asset-topology__edge').evaluateAll((items) => items.map((item) => ({
        id: item.getAttribute('data-edge-id'), source: item.getAttribute('data-source'), target: item.getAttribute('data-target'), relationship: item.getAttribute('data-relationship'),
      })).sort((a, b) => String(a.id).localeCompare(String(b.id))));
      const apiNodeIDs = (apiGraph?.nodes ?? []).map((node) => node.id).sort();
      const apiEdges = (apiGraph?.edges ?? []).map((edge) => ({ id: edge.id, source: edge.source, target: edge.target, relationship: edge.relationship })).sort((a, b) => String(a.id).localeCompare(String(b.id)));
      addCheck(
        checks,
        `tab-${tab.slug}-dynamic-svg-topology`,
        Boolean(apiGraph) && apiGraph.asset_id === canonicalId && await topology.locator('svg[role="img"]').isVisible() && nodeCount === apiNodeIDs.length && nodeCount >= tab.minimumTopologyNodes && svgNodeIconCount === svgNodeIDs.length && edgeCount === apiEdges.length && JSON.stringify(svgNodeIDs) === JSON.stringify(apiNodeIDs) && JSON.stringify(svgEdges) === JSON.stringify(apiEdges),
        { endpoint: `/api/v1/assets/${canonicalId}/topology`, api_source: apiGraph?.source, fixture_mode: apiGraph?.fixture_mode, api_node_ids: apiNodeIDs, svg_node_ids: svgNodeIDs, svg_node_icon_count: svgNodeIconCount, api_edges: apiEdges, svg_edges: svgEdges },
      );
    }
    const metrics = await capture(page, tab.captureId, states);
    if (tab.slug === 'endpoint') endpointPaginationY = metrics.ledger_contract.pagination?.y ?? null;
    const minimumSurfaceFill = tab.slug === 'network-device' ? 0.44 : tab.slug === 'unknown' ? 0.68 : 0.72;
    addCheck(checks, `tab-${tab.slug}-layout`, metrics.viewport.width === Number(args.width) && metrics.viewport.height === Number(args.height) && !metrics.horizontal_overflow && metrics.visible_bad_geometry.length === 0 && metrics.error_alerts.length === 0 && (metrics.main?.width || 0) >= 900 && (metrics.rail?.width || 0) >= 320 && metrics.surface_fill_ratio >= minimumSurfaceFill, { metrics, minimum_surface_fill: minimumSurfaceFill });
    addCheck(checks, `tab-${tab.slug}-ledger-capacity`, metrics.ledger_contract.row_count === Math.min(total, 10) && metrics.ledger_contract.minimum_row_height >= 21.9 && metrics.ledger_contract.last_row_visible && !metrics.ledger_contract.has_internal_vertical_scroll && !metrics.ledger_contract.has_internal_horizontal_scroll && (metrics.ledger_contract.panel?.height || 0) >= 320, { ledger_contract: metrics.ledger_contract });
    addCheck(checks, `tab-${tab.slug}-pagination-contract`, metrics.ledger_contract.pagination_visible, { expected_pagination: true, pagination_visible: metrics.ledger_contract.pagination_visible });
    addCheck(checks, `tab-${tab.slug}-business-flow`, metrics.business_flow.category_complete && metrics.business_flow.evidence_rendered && metrics.business_flow.clipped_content.length === 0, { business_flow: metrics.business_flow });
    addCheck(checks, `tab-${tab.slug}-full-business-region-complete`, metrics.bilateral_alignment.left_complete && metrics.bilateral_alignment.right_complete, { bilateral_alignment: metrics.bilateral_alignment });
    addCheck(checks, `tab-${tab.slug}-bottom-alignment`, Math.abs(metrics.bilateral_alignment.left_right_bottom_delta ?? Number.POSITIVE_INFINITY) <= 2, { expected_alignment: 'left-business-bottom-to-right-business-bottom', tolerance_px: 2, left_right_bottom_delta: metrics.bilateral_alignment.left_right_bottom_delta });
    addCheck(checks, `tab-${tab.slug}-bilateral-bottom-alignment`, metrics.bilateral_alignment.right_action_contained && metrics.bilateral_alignment.left_complete && metrics.bilateral_alignment.right_complete && Math.abs(metrics.bilateral_alignment.left_right_bottom_delta ?? Number.POSITIVE_INFINITY) <= 2, { expected_alignment: 'left-business-bottom-equals-right-business-bottom-without-footer-binding', tolerance_px: 2, bilateral_alignment: metrics.bilateral_alignment });
    if (tab.slug === 'server' || tab.slug === 'network-device') {
      const moduleContract = await page.evaluate((slug) => {
        const topology = document.querySelector('.taf-asset-category-workspace .taf-asset-topology')?.getBoundingClientRect();
        const panel = document.querySelector('.taf-asset-category-workspace .taf-asset-work-grid > .taf-panel:nth-child(2)')?.getBoundingClientRect();
        const riskRows = slug === 'server' ? Array.from(document.querySelectorAll('.taf-asset-port-risk-matrix > div:not(.taf-asset-port-risk-matrix__head, .taf-asset-port-risk-matrix__legend)')).map((row) => row.getBoundingClientRect()) : [];
        return {
          topology: topology ? { width: topology.width, height: topology.height } : null,
          panel: panel ? { width: panel.width, height: panel.height } : null,
          risk_rows: riskRows.map((row) => ({ x: row.x, y: row.y, width: row.width, height: row.height })),
          risk_rows_are_stacked: riskRows.every((row, index) => index === 0 || row.y > riskRows[index - 1].y + riskRows[index - 1].height - 1),
        };
      }, tab.slug);
      const minimumTopologyWidth = tab.slug === 'network-device' ? 340 : 400;
      addCheck(checks, `tab-${tab.slug}-topology-size`, Boolean(moduleContract.topology) && moduleContract.topology.width >= minimumTopologyWidth && moduleContract.topology.height >= 189.9, moduleContract);
      if (tab.slug === 'server') addCheck(checks, 'tab-server-port-risk-row-layout', moduleContract.risk_rows.length >= 5 && moduleContract.risk_rows_are_stacked && moduleContract.risk_rows.every((row) => row.width >= 380 && row.height >= 20), moduleContract);
    }
    if (tab.slug === 'network-device') {
      const viewAllButton = page.locator('.taf-asset-network-modules .taf-asset-panel-view-all').filter({ hasText: '查看全部镜像口' });
      await viewAllButton.click();
      const recordsModal = page.getByRole('dialog').filter({ hasText: '镜像口与采集链路 · 全部记录' });
      await recordsModal.waitFor({ state: 'visible', timeout: 10_000 });
      const modalRowCount = await recordsModal.locator('.taf-asset-data-table__row').count();
      addCheck(checks, 'tab-network-device-view-all-interaction', modalRowCount > 0, { label: '查看全部镜像口', row_count: modalRowCount });
      await recordsModal.getByRole('button', { name: 'Close' }).click();
    }
    await enforceCssViewport();
    await page.locator('.taf-main').evaluate((element) => { element.scrollTop = element.scrollHeight; });
    await page.waitForTimeout(180);
    const bottomMetrics = await capture(page, `${tab.captureId}-scroll-bottom`, states, { enforceViewport: false });
    addCheck(
      checks,
      `tab-${tab.slug}-scroll-bottom-reachable`,
      bottomMetrics.business_flow.clipped_content.length === 0
        && bottomMetrics.business_flow.evidence_panel
        && bottomMetrics.business_flow.evidence_panel.y < bottomMetrics.above_fold.fold_bottom
        && bottomMetrics.business_flow.evidence_panel.bottom <= bottomMetrics.above_fold.fold_bottom + 2
        && bottomMetrics.bilateral_alignment.last_action
        && bottomMetrics.bilateral_alignment.last_action.bottom <= bottomMetrics.above_fold.fold_bottom + 2
        && Math.abs(bottomMetrics.bilateral_alignment.left_right_bottom_delta ?? Number.POSITIVE_INFINITY) <= 2,
      { expected: 'whole-business-region-scroll-reveals-complete-aligned-left-and-right-bottoms', metrics: bottomMetrics },
    );
    await page.locator('.taf-main').evaluate((element) => { element.scrollTop = 0; });
    await page.waitForTimeout(100);
  }

  const endpointTab = page.locator('.taf-asset-tabs').getByRole('tab', { name: '终端', exact: true });
  await endpointTab.click();
  await page.waitForURL((url) => url.searchParams.get('tab') === 'endpoint', { timeout: 15_000 });
  await waitForAssets(page);
  const statusLabel = page.locator('.taf-asset-filter label').filter({ hasText: '状态' });
  await statusLabel.locator('.ant-select-selector').click();
  await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option[title="离线"]').click();
  await page.locator('.taf-asset-filter__actions .ant-btn-primary').click();
  await page.waitForFunction(() => document.querySelectorAll('.taf-asset-main .ant-table-tbody tr.ant-table-row').length === 1, null, { timeout: 15_000 });
  const filteredRows = await page.locator('.taf-asset-main .ant-table-tbody tr.ant-table-row').count();
  const filteredHeading = (await page.locator('.taf-asset-main .taf-panel__header h2').filter({ hasText: '资产清单' }).first().textContent()) || '';
  addCheck(checks, 'status-filter-real-api', filteredRows === 1 && /共\s*1\s*条/.test(filteredHeading), { row_count: filteredRows, heading: filteredHeading.trim() });
  const filteredMetrics = await capture(page, 'assets-endpoint-filter-inactive', states);
  addCheck(checks, 'status-filter-pagination-position-stable', endpointPaginationY !== null && filteredMetrics.ledger_contract.pagination && Math.abs(filteredMetrics.ledger_contract.pagination.y - endpointPaginationY) <= 1, { initial_pagination_y: endpointPaginationY, filtered_pagination_y: filteredMetrics.ledger_contract.pagination?.y ?? null, tolerance_px: 1 });
  await page.locator('.taf-asset-filter__actions .ant-btn').first().click();
  await waitForAssets(page);
  addCheck(checks, 'status-filter-reset', await page.locator('.taf-asset-main .ant-table-tbody tr.ant-table-row').count() === 10);

  await page.locator('.taf-asset-tabs').getByRole('tab', { name: '服务器', exact: true }).click();
  await page.waitForURL((url) => url.searchParams.get('tab') === 'server', { timeout: 15_000 });
  await waitForAssets(page);
  const serverId = new URL(page.url()).searchParams.get('assetId') || '';
  await page.locator('.taf-asset-action-rail button').filter({ hasText: '打开资产详情' }).click();
  const drawer = page.locator('[data-breakdown-page-id="assets-detail-basic"]');
  await drawer.waitFor({ state: 'visible', timeout: 15_000 });
  await drawer.getByText('规范资产 ID', { exact: true }).waitFor({ state: 'visible', timeout: 15_000 });
  addCheck(checks, 'server-basic-detail-real-api', (await drawer.textContent())?.includes(serverId) === true, { canonical_asset_id: serverId });
  for (const enabledLabel of ['网络接口', '开放服务', '归属信息', '历史变更']) {
    const disabled = await drawer.getByRole('tab', { name: enabledLabel, exact: true }).getAttribute('aria-disabled');
    addCheck(checks, `server-detail-${enabledLabel}-enabled`, disabled !== 'true', { aria_disabled: disabled });
  }
  const basicDetailMetrics = await capture(page, 'assets-detail-basic', states);
  addCheck(checks, 'server-basic-detail-layout', detailDialogFitsViewport(basicDetailMetrics) && !basicDetailMetrics.horizontal_overflow && basicDetailMetrics.visible_bad_geometry.length === 0 && basicDetailMetrics.error_alerts.length === 0, { metrics: basicDetailMetrics });
  const detailAssetIconCount = await page.locator('[data-breakdown-page-id="assets-detail-basic"] .taf-asset-detail-workspace__asset-icon .anticon').count();
  addCheck(checks, 'server-basic-detail-icon-contract', detailAssetIconCount === 1, { asset_type_icon_count: detailAssetIconCount });

  const detailCases = [
    { label: '网络接口', slug: 'network-interface', expectedRows: 6, heading: '网络接口表' },
    { label: '开放服务', slug: 'open-services', expectedRows: 7, heading: '开放服务表' },
    { label: '归属信息', slug: 'ownership', heading: '归属信息卡' },
  ];
  for (const detailCase of detailCases) {
    await page.locator('[data-breakdown-page-id^="assets-detail-"]').getByRole('tab', { name: detailCase.label, exact: true }).click();
    await page.waitForURL((url) => url.searchParams.get('detail') === detailCase.slug, { timeout: 15_000 });
    const detailPage = page.locator(`[data-breakdown-page-id="assets-detail-${detailCase.slug}"]`);
    await detailPage.waitFor({ state: 'visible', timeout: 15_000 });
    await detailPage.getByText(detailCase.heading, { exact: false }).first().waitFor({ state: 'visible', timeout: 15_000 });
    if (detailCase.expectedRows) {
      await page.waitForFunction(({ slug, expectedRows }) => document.querySelectorAll(`[data-breakdown-page-id="assets-detail-${slug}"] .ant-table-tbody tr.ant-table-row`).length === expectedRows, { slug: detailCase.slug, expectedRows: detailCase.expectedRows }, { timeout: 15_000 });
      const rowCount = await detailPage.locator('.ant-table-tbody tr.ant-table-row').count();
      addCheck(checks, `server-detail-${detailCase.slug}-real-api`, rowCount === detailCase.expectedRows, { row_count: rowCount });
    } else {
      const ownershipText = await detailPage.textContent();
      addCheck(checks, 'server-detail-ownership-real-api', ownershipText?.includes('平台运维组') === true && ownershipText?.includes('教学管理系统') === true, { contract: '/v1/assets/{id}/details' });
    }
    const detailMetrics = await capture(page, `assets-detail-${detailCase.slug}`, states);
    addCheck(checks, `server-detail-${detailCase.slug}-layout`, detailDialogFitsViewport(detailMetrics) && !detailMetrics.horizontal_overflow && detailMetrics.visible_bad_geometry.length === 0 && detailMetrics.error_alerts.length === 0, { metrics: detailMetrics });
  }

  await page.locator('[data-breakdown-page-id="assets-detail-ownership"]').getByRole('tab', { name: '历史变更', exact: true }).click();
  await page.waitForURL((url) => url.searchParams.get('detail') === 'history', { timeout: 15_000 });
  const history = page.locator('[data-breakdown-page-id="assets-detail-history"]');
  await history.waitFor({ state: 'visible', timeout: 15_000 });
  await history.getByText('真实变更记录', { exact: true }).waitFor({ state: 'visible', timeout: 15_000 });
  await page.waitForFunction(() => document.querySelectorAll('[data-breakdown-page-id="assets-detail-history"] .ant-table-tbody tr.ant-table-row').length === 2, null, { timeout: 15_000 });
  const historyRows = await history.locator('.ant-table-tbody tr.ant-table-row').count();
  addCheck(checks, 'server-history-real-api', historyRows === 2, { row_count: historyRows });
  const historyDetailMetrics = await capture(page, 'assets-detail-history', states);
  addCheck(checks, 'server-history-detail-layout', detailDialogFitsViewport(historyDetailMetrics) && !historyDetailMetrics.horizontal_overflow && historyDetailMetrics.visible_bad_geometry.length === 0 && historyDetailMetrics.error_alerts.length === 0, { metrics: historyDetailMetrics });

  await history.locator('button').filter({ hasText: '跳转实体图谱' }).click();
  await page.waitForURL((url) => url.pathname === '/graph' && url.searchParams.get('assetId') === serverId, { timeout: 15_000 });
  addCheck(checks, 'cross-page-graph-asset-id', new URL(page.url()).searchParams.get('assetId') === serverId, { final_url: page.url() });
  await page.waitForFunction(() => performance.getEntriesByType('resource').some((entry) => entry.name.includes('/api/v1/graph') && new URL(entry.name).searchParams.has('ip')), null, { timeout: 15_000 });
  addCheck(checks, 'cross-page-graph-request-scoped', requestUrls.some((url) => url.includes(`/api/v1/assets/${serverId}`)) && requestUrls.some((url) => url.includes('/api/v1/graph') && Boolean(new URL(url).searchParams.get('ip'))), { canonical_asset_id: serverId });
  await capture(page, 'assets-cross-page-graph', states);

  await page.goto(authenticatedUrl(`/assets?tab=server&assetId=${encodeURIComponent(serverId)}&detail=basic`, token), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('[data-breakdown-page-id="assets-detail-basic"]').waitFor({ state: 'visible', timeout: 15_000 });
  await page.locator('[data-breakdown-page-id="assets-detail-basic"] button').filter({ hasText: '进入取证分析' }).click();
  await page.waitForURL((url) => url.pathname === '/forensics' && url.searchParams.get('assetId') === serverId, { timeout: 15_000 });
  addCheck(checks, 'cross-page-forensics-asset-id', new URL(page.url()).searchParams.get('assetId') === serverId && (await page.locator('body').innerText()).includes(serverId), { final_url: page.url() });
  await page.waitForFunction((assetId) => performance.getEntriesByType('resource').some((entry) => entry.name.includes('/api/v1/pcap/jobs') && entry.name.includes(`asset_id=${encodeURIComponent(assetId)}`)), serverId, { timeout: 15_000 });
  addCheck(checks, 'cross-page-forensics-request-scoped', requestUrls.some((url) => url.includes('/api/v1/pcap/jobs') && new URL(url).searchParams.get('asset_id') === serverId), { canonical_asset_id: serverId });
  const forensicsRows = page.locator('.taf-forensics-task-panel .ant-table-tbody tr.ant-table-row');
  const forensicsAssets = await forensicsRows.locator('td:nth-child(3)').allTextContents();
  const forensicsRowCount = await forensicsRows.count();
  addCheck(checks, 'cross-page-forensics-response-scoped', forensicsRowCount > 0 && forensicsAssets.every((asset) => asset.trim() === serverId) && !(await page.locator('.taf-forensics-task-panel').innerText()).includes('办公区-WS-1024'), { canonical_asset_id: serverId, row_count: forensicsRowCount, assets: forensicsAssets });
  await capture(page, 'assets-cross-page-forensics', states);
} catch (error) {
  addCheck(checks, 'acceptance-script-completed', false, { error: error instanceof Error ? error.stack || error.message : String(error) });
} finally {
  await page.close().catch(() => {});
  await browser.close().catch(() => {});
}

const failures = checks.filter((check) => check.status !== 'pass');
const report = {
  package_id: 'asset_inventory_windows_chrome_xshell_acceptance',
  generated_at: new Date().toISOString(),
  result: failures.length || badResponses.length || requestFailures.length || consoleErrors.length || pageErrors.length ? 'fail' : 'pass',
  browser_path: 'Xshell tunnel -> 127.0.0.1:9224 -> Windows Chrome',
  cdp_url: args.cdpUrl,
  browser: preflight.version.Browser,
  user_agent: preflight.version['User-Agent'],
  cdp_targets_count: preflight.targets.length,
  viewport_requested: { width: Number(args.width), height: Number(args.height) },
  viewport_contract: { css_width: Number(args.width), css_height: Number(args.height), device_scale_factor: 1, page_scale_factor: 1 },
  evidence_dir: args.evidenceDir,
  summary: { checks: checks.length, pass: checks.length - failures.length, fail: failures.length, states: states.length },
  failures,
  checks,
  states,
  runtime_errors: { bad_responses: badResponses, request_failures: requestFailures, console_errors: consoleErrors, page_errors: pageErrors, external_extension_errors: externalExtensionErrors },
  topology_api_summaries: Object.fromEntries([...topologyApiResponses.entries()].map(([assetId, graph]) => [assetId, { source: graph.source, fixture_mode: graph.fixture_mode, nodes: graph.nodes, edges: graph.edges }])),
  reference_image_policy: 'primary visual baseline; documented business-logic corrections are allowed after logic/layout review and main-thread adjudication',
  token_material_redacted: true,
};
writeJson(String(args.output), report);
writeJson(path.join(String(args.evidenceDir), 'acceptance-report.json'), report);
console.log(JSON.stringify({ result: report.result, summary: report.summary, output: args.output, evidence_dir: args.evidenceDir, failures: failures.map((item) => item.id) }, null, 2));
if (report.result !== 'pass') process.exit(1);

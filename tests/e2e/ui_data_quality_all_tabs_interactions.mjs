#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = process.env.DQ_BASE_URL?.trim() || 'http://10.0.5.8:30180';
const cdpUrl = process.env.DQ_CDP_URL?.trim() || 'http://127.0.0.1:9224';
const evidenceRevision = process.env.DQ_EVIDENCE_REVISION?.trim() || 'r264';
const outputPath = path.join(root, `evidence/ui-image-breakdowns/pages/data-quality/interaction-${evidenceRevision}-all-tabs.json`);
const screenshotDir = path.join(root, 'evidence/ui-image-breakdowns/pages/data-quality');

const tabs = [
  { slug: 'overview', label: '质量总览', selector: '.taf-data-quality-anomalies button' },
  { slug: 'topic-health', label: 'Topic 健康', selector: '.taf-data-quality-topic-rail-alerts button' },
  { slug: 'flink-quality', label: 'Flink 质量', selector: '.taf-data-quality-flink-failure-table button' },
  { slug: 'field-quality', label: '字段质量', selector: '.taf-data-quality-field-sample-table button' },
  { slug: 'storage-quality', label: '存储质量', selector: '.taf-data-quality-storage-component-table button' },
  { slug: 'replay-reconcile', label: '重放对账', selector: '.taf-data-quality-replay-task-table button' },
  { slug: 'report', label: '质量报告', selector: '.taf-data-quality-report-viewerbar button' },
  { slug: 'settings', label: '质量设置', selector: '.taf-data-quality-settings-threshold button' },
];

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken(permissions = ['*', 'admin:*', 'data-quality:read', 'data-quality:write', 'dlq:replay'], userId = crypto.randomUUID(), roles = ['admin']) {
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
    user_id: userId,
    tenant_id: 'default',
    username: 'codex-windows-cdp-admin',
    roles,
    permissions,
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
const cdpSession = await context.newCDPSession(page);
await cdpSession.send('Emulation.setDeviceMetricsOverride', {
  width: 1920,
  height: 1080,
  deviceScaleFactor: 1,
  mobile: false,
});

async function clickStable(locator, attempts = 4) {
  let lastError;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      await locator.first().waitFor({ state: 'visible', timeout: 10_000 });
      await locator.first().evaluate((button) => button.click());
      return;
    } catch (error) {
      lastError = error;
      await page.waitForTimeout(250);
    }
  }
  throw lastError;
}

async function ensureDataQualityTab(slug) {
  const selected = page.locator(`.taf-data-quality-tabs button[data-tab-slug="${slug}"][aria-selected="true"]`);
  for (let attempt = 0; attempt < 4; attempt += 1) {
    if (!await selected.count()) {
      await clickStable(page.locator(`.taf-data-quality-tabs button[data-tab-slug="${slug}"]`));
      await selected.waitFor({ state: 'visible', timeout: 5_000 }).catch(() => {});
    }
    await page.waitForTimeout(300);
    if (await selected.count()) return;
  }
  await selected.waitFor({ state: 'visible', timeout: 10_000 });
}

const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
const dataTableRequests = [];
let expectedPermissionProbe = false;
page.on('response', (response) => {
  if (expectedPermissionProbe && response.status() === 403) return;
  if (response.status() >= 400 && response.url().startsWith(baseUrl)) badResponses.push({ status: response.status(), url: response.url() });
});
page.on('console', (entry) => {
  if (expectedPermissionProbe && entry.text().includes('403')) return;
  if (entry.type() === 'error' && !entry.text().includes('api.yhchj.com') && !entry.text().includes('ERR_CONNECTION_CLOSED')) consoleErrors.push(entry.text());
});
page.on('pageerror', (error) => {
  if (error.message !== 'Object') pageErrors.push(error.message);
});
page.on('requestfailed', (request) => {
  if (request.url().startsWith(baseUrl)) requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? 'unknown' });
});
page.on('request', (request) => {
  if (request.url().includes('/api/v1/data-quality/tables/')) dataTableRequests.push(request.url());
});

const requestedTab = process.env.DQ_TAB?.trim();
const selectedTabs = requestedTab ? tabs.filter((tab) => tab.slug === requestedTab) : tabs;
if (!selectedTabs.length) throw new Error(`Unknown DQ_TAB: ${requestedTab}`);
const previous = process.env.DQ_RESET === '1' || !fs.existsSync(outputPath)
  ? { tab_results: [], bad_responses: [], console_errors: [], page_errors: [], request_failures: [] }
  : JSON.parse(fs.readFileSync(outputPath, 'utf8'));
const actionRunUserId = crypto.randomUUID();
const token = smokeToken(undefined, actionRunUserId);
const tabResults = Array.isArray(previous.tab_results) ? previous.tab_results.filter((tab) => !selectedTabs.some((selected) => selected.slug === tab.slug)) : [];
fs.mkdirSync(screenshotDir, { recursive: true });

for (const tab of selectedTabs) {
  const routeUrl = new URL(`/data-quality?tab=${tab.slug}&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
  routeUrl.hash = `codex_smoke_token=${token}`;
  await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  await page.locator('.taf-data-quality').waitFor({ state: 'visible', timeout: 15_000 });
  await page.locator(`.taf-data-quality-tabs button[data-tab-slug="${tab.slug}"][aria-selected="true"]`).waitFor({ state: 'visible', timeout: 10_000 });
  const actionControl = page.locator(tab.selector).first();
  await actionControl.waitFor({ state: 'visible', timeout: 10_000 });
  const chartCanvasCount = await page.locator('.taf-data-quality canvas').count();
  const tabGeometry = await page.locator('.taf-data-quality-tabs button').evaluateAll((buttons) => buttons.map((button) => {
    const box = button.getBoundingClientRect();
    return {
      slug: button.getAttribute('data-tab-slug'),
      x: Number(box.x.toFixed(3)),
      y: Number(box.y.toFixed(3)),
      width: Number(box.width.toFixed(3)),
      height: Number(box.height.toFixed(3)),
    };
  }));
  await clickStable(actionControl);
  const drawer = page.locator('.taf-data-quality-field-detail-drawer:visible');
  await drawer.waitFor({ state: 'visible', timeout: 5_000 });
  const drawerVisible = await drawer.isVisible();
  const endpointVisible = await drawer.getByText('POST /v1/data-quality/actions', { exact: true }).isVisible();
  const auditEventVisible = await drawer.getByText('DATA_QUALITY_ACTION_REQUESTED', { exact: true }).isVisible();
  await drawer.locator('.ant-drawer-close').evaluate((button) => button.click());
  const apiVerification = await page.evaluate(async ({ origin, accessToken, slug }) => {
    const headers = { Authorization: `Bearer ${accessToken}`, 'Content-Type': 'application/json', 'X-Tenant-ID': 'default' };
    const qualityResponse = await fetch(`${origin}/api/v1/data-quality`, { headers });
    const qualityBody = await qualityResponse.json();
    const actionResponse = await fetch(`${origin}/api/v1/data-quality/actions`, {
      method: 'POST',
      headers,
      body: JSON.stringify({
        view: slug,
        action: 'windows-chrome-acceptance',
        target: `${slug}-workspace`,
        dry_run: true,
        reason: 'Windows Chrome 数据质量验收预检',
        parameters: { evidence_revision: 'r275+' },
      }),
    });
    const actionBody = await actionResponse.json();
    return {
      get_status: qualityResponse.status,
      tenant_id: qualityBody?.data?.tenant_id,
      visual_source: qualityBody?.data?.data_source?.visuals,
      fixture_version: qualityBody?.data?.data_source?.fixture_version,
      action_status: actionResponse.status,
      action_id: actionBody?.data?.action_id,
      action_result_status: actionBody?.data?.status,
    };
  }, { origin: baseUrl, accessToken: token, slug: tab.slug });

  const reportVerification = tab.slug === 'report'
    ? await page.evaluate(async ({ origin, accessToken }) => {
      const headers = { Authorization: `Bearer ${accessToken}`, 'X-Tenant-ID': 'default' };
      const reportResponse = await fetch(`${origin}/api/v1/data-quality/reports/daily`, { headers });
      const reportBody = await reportResponse.json();
      const downloadResults = {};
      for (const format of ['pdf', 'json', 'csv']) {
        const response = await fetch(`${origin}/api/v1/data-quality/reports/daily/download?format=${format}`, { headers });
        const payload = new Uint8Array(await response.arrayBuffer());
        downloadResults[format] = {
          status: response.status,
          content_type: response.headers.get('content-type'),
          content_disposition: response.headers.get('content-disposition'),
          byte_length: payload.byteLength,
          signature: String.fromCharCode(...payload.slice(0, format === 'pdf' ? 8 : 3)),
        };
      }
      const workspace = document.querySelector('.taf-data-quality-report-workspace');
      const exportPanel = workspace?.querySelector('.taf-data-quality-report-export-panel');
      const approvalPanel = workspace?.querySelector('.taf-data-quality-report-approval-panel');
      const exportBox = exportPanel?.getBoundingClientRect();
      const approvalBox = approvalPanel?.getBoundingClientRect();
      const tables = [...(workspace?.querySelectorAll('.taf-data-quality-report-mini-table, .taf-data-quality-report-api-table') ?? [])].map((table) => {
        const style = getComputedStyle(table);
        return {
          class_name: table.className,
          is_export: table.classList.contains('is-export'),
          overflow_y: style.overflowY,
          client_height: table.clientHeight,
          scroll_height: table.scrollHeight,
          has_internal_scroll: ['auto', 'scroll'].includes(style.overflowY) && table.scrollHeight > table.clientHeight + 1,
        };
      });
      const exportTable = workspace?.querySelector('.taf-data-quality-report-api-table.is-export');
      const exportTableBox = exportTable?.getBoundingClientRect();
      const exportButtons = [...(exportTable?.querySelectorAll('button') ?? [])].map((button) => button.getBoundingClientRect());
      return {
        report_status: reportResponse.status,
        report_id: reportBody?.data?.report_id,
        generated_at: reportBody?.data?.generated_at,
        monitor_source: reportBody?.data?.source?.monitor,
        fixture_version: reportBody?.data?.source?.fixture_version,
        export_above_approval: Boolean(exportBox && approvalBox && approvalBox.top >= exportBox.bottom - 1),
        export_approval_width_delta: Number((exportBox && approvalBox ? Math.abs(exportBox.width - approvalBox.width) : Number.POSITIVE_INFINITY).toFixed(3)),
        report_table_count: tables.length,
        report_table_internal_scroll_count: tables.filter((table) => table.has_internal_scroll).length,
        export_table_internal_scroll: tables.some((table) => table.is_export && table.has_internal_scroll),
        export_actions_contained: Boolean(exportTableBox && exportButtons.every((box) => box.right <= exportTableBox.right + 1 && box.left >= exportTableBox.left - 1)),
        tables,
        downloads: downloadResults,
      };
    }, { origin: baseUrl, accessToken: token })
    : null;

  // The shared Windows Chrome profile can finish a delayed auth return while a
  // tab is under test. Reassert the intended business tab before reading its
  // table and geometry contracts.
  await ensureDataQualityTab(tab.slug);
  await page.mouse.move(2, 2);

  const allPagination = [];
  await page.locator('.taf-data-quality-shell').evaluate((shell) => { shell.scrollTop = 0; });
  const pagers = page.locator('.taf-data-quality-paged-table > .taf-data-quality-table-pagination');
  const readTableRows = async (table) => table.evaluate((element) => [...element.children]
    .filter((child) => !child.classList.contains('taf-data-quality-table-pagination'))
    .map((child) => child.textContent ?? '')
    .join(' ')
    .replace(/\s+/g, ' ')
    .trim());
  for (let index = 0; index < await pagers.count(); index += 1) {
    const pager = pagers.nth(index);
    const dataset = await pager.getAttribute('data-pagination-dataset');
    const source = await pager.getAttribute('data-pagination-source');
    const label = await pager.getAttribute('aria-label');
    const table = pager.locator('..');
    const next = pager.getByTitle('下一页');
    const canAdvance = await next.isEnabled();
    const before = await readTableRows(table);
    const beforeTop = await pager.evaluate((element) => {
      const table = element.parentElement;
      return element.getBoundingClientRect().top - (table?.getBoundingClientRect().top ?? 0);
    });
    let after = before;
    let responseStatus = null;
    if (canAdvance) {
      if (source === 'server' && dataset) {
        const [response] = await Promise.all([
          page.waitForResponse((candidate) => candidate.url().includes(`/api/v1/data-quality/tables/${dataset}`)
            && new URL(candidate.url()).searchParams.get('page') === '2', { timeout: 15_000 }),
          next.evaluate((button) => button.click()),
        ]);
        responseStatus = response.status();
      } else {
        await next.evaluate((button) => button.click());
      }
      await pager.locator('button[aria-current="page"]').filter({ hasText: /^2$/ }).waitFor({ state: 'visible', timeout: 10_000 });
      const signatureDeadline = Date.now() + 10_000;
      do {
        after = await readTableRows(table);
        if (after !== before) break;
        await page.waitForTimeout(100);
      } while (Date.now() < signatureDeadline);
    }
    const afterTop = await pager.evaluate((element) => {
      const table = element.parentElement;
      return element.getBoundingClientRect().top - (table?.getBoundingClientRect().top ?? 0);
    });
    allPagination.push({
      label,
      source,
      dataset,
      can_advance: canAdvance,
      response_status: responseStatus,
      row_signature_changed: before !== after,
      position_delta: Number(Math.abs(afterTop - beforeTop).toFixed(3)),
    });
  }
  const serverPagination = allPagination.filter((pager) => pager.source === 'server');

  await ensureDataQualityTab(tab.slug);
  const businessScroll = await page.locator('.taf-data-quality-shell').evaluate((shell) => {
    shell.scrollTop = 0;
    const rail = shell.querySelector('.taf-data-quality-rail');
    const main = shell.querySelector('.taf-data-quality-main');
    const titlebar = shell.querySelector('.taf-data-quality-titlebar');
    const heading = shell.querySelector('.taf-data-quality-heading h1');
    const tabs = shell.querySelector('.taf-data-quality-tabs');
    const style = getComputedStyle(shell);
    const railBox = rail?.getBoundingClientRect();
    const mainBox = main?.getBoundingClientRect();
    const titlebarBox = titlebar?.getBoundingClientRect();
    const headingBox = heading?.getBoundingClientRect();
    const overallScore = shell.querySelector('.taf-data-quality-metric.is-overall-score');
    const overallScoreCanvas = overallScore?.querySelector('canvas');
    const overallScoreRingBox = overallScore?.querySelector('.taf-data-quality-overall-score-ring')?.getBoundingClientRect();
    const overallScoreCopyBox = overallScore?.querySelector('.taf-data-quality-overall-score-copy')?.getBoundingClientRect();
    const pagers = [...shell.querySelectorAll('.taf-data-quality-paged-table')].map((table) => {
      const pager = table.querySelector(':scope > .taf-data-quality-table-pagination, :scope > .taf-data-quality-field-pagination, :scope > .taf-data-quality-topic-health-footer, :scope > .taf-data-quality-flink-job-footer');
      const previous = pager?.previousElementSibling;
      const pagerBox = pager?.getBoundingClientRect();
      const previousBox = previous?.getBoundingClientRect();
      return {
        present: Boolean(pager),
        height: Number((pagerBox?.height ?? 0).toFixed(3)),
        overlap: Number((pagerBox && previousBox ? Math.max(0, previousBox.bottom - pagerBox.top) : 0).toFixed(3)),
        position: pager ? getComputedStyle(pager).position : 'missing',
      };
    });
    const scrollTables = [...shell.querySelectorAll('.taf-data-quality-scroll-table')].map((table) => {
      const body = table.querySelector(':scope > .taf-data-quality-scroll-body');
      return {
        class_name: table.className,
        overflow_y: body ? getComputedStyle(body).overflowY : 'missing',
        client_height: body?.clientHeight ?? 0,
        scroll_height: body?.scrollHeight ?? 0,
      };
    });
    const requestedNoPagerCount = shell.classList.contains('is-field-quality-tab')
      ? shell.querySelectorAll('.taf-data-quality-field-matrix .taf-data-quality-table-pagination, .taf-data-quality-community-check .taf-data-quality-table-pagination').length
      : shell.querySelectorAll('.taf-data-quality-overview-topic .taf-data-quality-table-pagination, .taf-data-quality-message-size .taf-data-quality-table-pagination').length;
    const scrollTableMode = shell.classList.contains('is-scroll-table-mode');
    const contractTables = [...new Set(shell.querySelectorAll('.taf-data-quality-paged-table, .taf-data-quality-scroll-table, .taf-data-quality-flink-failure-table'))];
    const moreFooterCount = shell.querySelectorAll('.taf-data-quality-more-footer').length;
    shell.scrollTop = shell.scrollHeight;
    const stickyTitlebarBox = titlebar?.getBoundingClientRect();
    return {
      overflow_y: style.overflowY,
      client_height: shell.clientHeight,
      scroll_height: shell.scrollHeight,
      scroll_top: shell.scrollTop,
      bottom_reachable: shell.scrollTop + shell.clientHeight >= shell.scrollHeight - 2,
      rail_width: Number((railBox?.width ?? 0).toFixed(3)),
      main_bottom: Number((mainBox?.bottom ?? 0).toFixed(3)),
      rail_bottom: Number((railBox?.bottom ?? 0).toFixed(3)),
      main_rail_top_delta: Number((railBox && mainBox ? Math.abs(railBox.top - mainBox.top) : Number.POSITIVE_INFINITY).toFixed(3)),
      main_rail_bottom_delta: Number((railBox && mainBox ? Math.abs(railBox.bottom - mainBox.bottom) : Number.POSITIVE_INFINITY).toFixed(3)),
      rail_below_tabs: Boolean(railBox && titlebarBox && railBox.top >= titlebarBox.bottom - 1),
      menu_heading_visible: Boolean(heading && headingBox && headingBox.width > 1 && headingBox.height > 1 && getComputedStyle(heading).display !== 'none'),
      menu_heading_text: heading?.textContent?.trim() ?? '',
      overall_score_engine: overallScoreCanvas ? 'echarts-canvas' : overallScore ? 'non-echarts' : null,
      overall_score_text_overlap: Number((overallScoreRingBox && overallScoreCopyBox ? Math.max(0, Math.min(overallScoreRingBox.right, overallScoreCopyBox.right) - Math.max(overallScoreRingBox.left, overallScoreCopyBox.left)) : 0).toFixed(3)),
      tab_overflow_x: tabs ? getComputedStyle(tabs).overflowX : 'missing',
      tabs_fit_without_scroll: Boolean(tabs && tabs.scrollWidth <= tabs.clientWidth + 1),
      sticky_tab_delta: Number((titlebarBox && stickyTitlebarBox ? Math.abs(stickyTitlebarBox.top - titlebarBox.top) : Number.POSITIVE_INFINITY).toFixed(3)),
      paged_table_count: pagers.length,
      missing_pager_count: pagers.filter((pager) => !pager.present).length,
      minimum_pager_height: Math.min(...pagers.map((pager) => pager.height)),
      maximum_pager_overlap: Math.max(0, ...pagers.map((pager) => pager.overlap)),
      overlay_positioned_pager_count: pagers.filter((pager) => ['absolute', 'fixed'].includes(pager.position)).length,
      requested_no_pager_count: requestedNoPagerCount,
      scroll_table_mode: scrollTableMode,
      legacy_pagination_count: shell.querySelectorAll('.taf-data-quality-table-pagination, .taf-data-quality-field-pagination, .taf-data-quality-topic-health-footer, .taf-data-quality-flink-job-footer').length,
      contract_table_count: contractTables.length,
      more_footer_count: moreFooterCount,
      scroll_tables: scrollTables,
    };
  });
  await page.waitForTimeout(120);
  const bottomScreenshotPath = path.join(screenshotDir, `interaction-${evidenceRevision}-${tab.slug}-bottom.png`);
  const bottomScreenshot = await cdpSession.send('Page.captureScreenshot', { format: 'png', fromSurface: true, captureBeyondViewport: false });
  fs.writeFileSync(bottomScreenshotPath, Buffer.from(bottomScreenshot.data, 'base64'));
  await page.locator('.taf-data-quality-shell').evaluate((shell) => { shell.scrollTop = 0; });
  await ensureDataQualityTab(tab.slug);

  const heatmapLocator = page.locator('.taf-data-quality-heatmap, .taf-data-quality-flink-backpressure-echart, .taf-data-quality-field-matrix-echart');
  const heatmapGeometry = await heatmapLocator.count() > 0
    ? await heatmapLocator.first().evaluate((container) => {
      const canvas = container.querySelector('canvas');
      const containerBox = container.getBoundingClientRect();
      const canvasBox = canvas?.getBoundingClientRect();
      return {
        engine: canvas ? 'echarts-canvas' : 'missing',
        container_width: Number(containerBox.width.toFixed(3)),
        container_height: Number(containerBox.height.toFixed(3)),
        canvas_width: Number((canvasBox?.width ?? 0).toFixed(3)),
        canvas_height: Number((canvasBox?.height ?? 0).toFixed(3)),
        width_fill_ratio: Number(((canvasBox?.width ?? 0) / Math.max(containerBox.width, 1)).toFixed(4)),
        height_fill_ratio: Number(((canvasBox?.height ?? 0) / Math.max(containerBox.height, 1)).toFixed(4)),
      };
    })
    : null;

  const screenshotPath = path.join(screenshotDir, `interaction-${evidenceRevision}-${tab.slug}.png`);
  const screenshot = await cdpSession.send('Page.captureScreenshot', { format: 'png', fromSurface: true, captureBeyondViewport: false });
  fs.writeFileSync(screenshotPath, Buffer.from(screenshot.data, 'base64'));
  let fieldQualityClickGeometryDelta = null;
  if (tab.slug === 'overview') {
    await page.locator('.taf-data-quality-tabs button[data-tab-slug="field-quality"]').click();
    await page.locator('.taf-data-quality-tabs button[data-tab-slug="field-quality"][aria-selected="true"]').waitFor({ state: 'visible', timeout: 10_000 });
    const afterClickGeometry = await page.locator('.taf-data-quality-tabs button').evaluateAll((buttons) => buttons.map((button) => {
      const box = button.getBoundingClientRect();
      return {
        slug: button.getAttribute('data-tab-slug'),
        x: Number(box.x.toFixed(3)),
        y: Number(box.y.toFixed(3)),
        width: Number(box.width.toFixed(3)),
        height: Number(box.height.toFixed(3)),
      };
    }));
    fieldQualityClickGeometryDelta = Math.max(...afterClickGeometry.slice(0, 3).flatMap((box, index) => {
      const baseline = tabGeometry[index];
      return ['x', 'y', 'width', 'height'].map((key) => Math.abs(box[key] - baseline[key]));
    }));
  }
  tabResults.push({
    slug: tab.slug,
    label: tab.label,
    active: true,
    chart_canvas_count: chartCanvasCount,
    action_drawer_visible: drawerVisible,
    endpoint_visible: endpointVisible,
    audit_event_visible: auditEventVisible,
    api_verification: apiVerification,
    report_verification: reportVerification,
    all_pagination: allPagination,
    server_pagination: serverPagination,
    business_scroll: businessScroll,
    heatmap_geometry: heatmapGeometry,
    bottom_screenshot: path.relative(root, bottomScreenshotPath),
    tab_geometry: tabGeometry,
    field_quality_click_first_three_max_delta: fieldQualityClickGeometryDelta,
    screenshot: path.relative(root, screenshotPath),
  });
}

const readOnlyToken = smokeToken(['data-quality:read'], crypto.randomUUID(), ['viewer']);
const noScopeToken = smokeToken(['alerts:read'], crypto.randomUUID(), ['viewer']);
expectedPermissionProbe = true;
const permissionVerification = await page.evaluate(async ({ origin, readToken, deniedToken }) => {
  const requestStatus = async (tokenValue, method, route, body) => {
    const response = await fetch(`${origin}${route}`, {
      method,
      headers: { Authorization: `Bearer ${tokenValue}`, 'Content-Type': 'application/json', 'X-Tenant-ID': 'default' },
      body: body ? JSON.stringify(body) : undefined,
    });
    return response.status;
  };
  return {
    read_status: await requestStatus(readToken, 'GET', '/api/v1/data-quality'),
    report_read_status: await requestStatus(readToken, 'GET', '/api/v1/data-quality/reports/daily'),
    read_only_write_status: await requestStatus(readToken, 'POST', '/api/v1/data-quality/actions', { view: 'overview', action: 'denied-write', target: 'overview', dry_run: true }),
    no_scope_read_status: await requestStatus(deniedToken, 'GET', '/api/v1/data-quality'),
    no_scope_report_status: await requestStatus(deniedToken, 'GET', '/api/v1/data-quality/reports/daily'),
  };
}, { origin: baseUrl, readToken: readOnlyToken, deniedToken: noScopeToken });
expectedPermissionProbe = false;

const persistenceOutput = execFileSync(
  'kubectl',
  ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', `SELECT (SELECT count(*) FROM data_quality_actions WHERE tenant_id='default' AND requested_by='${actionRunUserId}'), (SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND action='DATA_QUALITY_ACTION_REQUESTED' AND object_id IN (SELECT action_id::text FROM data_quality_actions WHERE requested_by='${actionRunUserId}'));`],
  { encoding: 'utf8', env: process.env, timeout: 15_000 },
).trim();
const [persistedActions, persistedAudits] = persistenceOutput.split('|').map((value) => Number(value));

const mergedBadResponses = [...(previous.bad_responses ?? []), ...badResponses];
const mergedConsoleErrors = [...(previous.console_errors ?? []), ...consoleErrors];
const mergedPageErrors = [...(previous.page_errors ?? []), ...pageErrors];
const mergedRequestFailures = [...(previous.request_failures ?? []), ...requestFailures];
const tableContractPassed = (tab) => tab.slug === 'report'
  ? tab.business_scroll?.legacy_pagination_count === 0
    && tab.business_scroll?.more_footer_count === 0
    && (tab.all_pagination ?? []).length === 0
  : tab.business_scroll?.scroll_table_mode
  ? tab.business_scroll?.legacy_pagination_count === 0
    && tab.business_scroll?.contract_table_count > 0
    && tab.business_scroll?.more_footer_count === 0
    && (tab.all_pagination ?? []).length === 0
  : tab.business_scroll?.paged_table_count > 0
    && tab.business_scroll?.missing_pager_count === 0
    && tab.business_scroll?.minimum_pager_height >= 32
    && tab.business_scroll?.maximum_pager_overlap <= 0.5
    && tab.business_scroll?.overlay_positioned_pager_count === 0
    && (tab.all_pagination ?? []).length === tab.business_scroll?.paged_table_count
    && (tab.all_pagination ?? []).every((pager) => pager.can_advance && pager.row_signature_changed && pager.position_delta <= 0.5
      && (pager.source !== 'server' || pager.response_status === 200));
const reportContractPassed = (tab) => tab.slug !== 'report' || (
  tab.report_verification?.report_status === 200
  && tab.report_verification?.report_id
  && tab.report_verification?.generated_at
  && tab.report_verification?.monitor_source === 'clickhouse-live'
  && tab.report_verification?.fixture_version
  && tab.report_verification?.export_above_approval
  && tab.report_verification?.export_approval_width_delta <= 1
  && tab.report_verification?.report_table_count >= 4
  && tab.report_verification?.report_table_internal_scroll_count === 1
  && tab.report_verification?.export_table_internal_scroll
  && tab.report_verification?.export_actions_contained
  && tab.report_verification?.downloads?.pdf?.status === 200
  && tab.report_verification?.downloads?.pdf?.content_type === 'application/pdf'
  && tab.report_verification?.downloads?.pdf?.signature.startsWith('%PDF-')
  && tab.report_verification?.downloads?.json?.status === 200
  && tab.report_verification?.downloads?.json?.content_type?.startsWith('application/json')
  && tab.report_verification?.downloads?.csv?.status === 200
  && tab.report_verification?.downloads?.csv?.content_type?.startsWith('text/csv')
  && ['pdf', 'json', 'csv'].every((format) => tab.report_verification?.downloads?.[format]?.byte_length > 100)
);
const allTabsPassed = tabs.every((expected) => tabResults.some((tab) => (
  tab.slug === expected.slug && tab.active && tab.chart_canvas_count >= 1 && tab.action_drawer_visible && tab.endpoint_visible && tab.audit_event_visible
    && tab.business_scroll?.bottom_reachable && ['auto', 'scroll'].includes(tab.business_scroll?.overflow_y) && tab.business_scroll?.rail_width >= 300
    && tab.business_scroll?.rail_below_tabs && tab.business_scroll?.tab_overflow_x === 'hidden' && tab.business_scroll?.tabs_fit_without_scroll
    && tab.business_scroll?.menu_heading_visible && tab.business_scroll?.menu_heading_text === '数据质量'
    && (tab.slug !== 'overview' || (tab.business_scroll?.overall_score_engine === 'echarts-canvas' && tab.business_scroll?.overall_score_text_overlap === 0))
    && tab.business_scroll?.sticky_tab_delta <= 0.01 && tableContractPassed(tab)
    && reportContractPassed(tab)
    && (!tab.heatmap_geometry || (tab.heatmap_geometry.engine === 'echarts-canvas' && tab.heatmap_geometry.width_fill_ratio >= 0.98 && tab.heatmap_geometry.height_fill_ratio >= 0.98))
    && tab.api_verification?.get_status === 200 && tab.api_verification?.tenant_id === 'default'
    && tab.api_verification?.visual_source === 'postgres-activated-fixture' && tab.api_verification?.fixture_version
    && tab.api_verification?.action_status === 202 && tab.api_verification?.action_id && tab.api_verification?.action_result_status === 'dry_run'
)));
const geometryBaseline = tabResults.find((tab) => tab.slug === 'overview')?.tab_geometry ?? tabResults[0]?.tab_geometry ?? [];
const maxTabGeometryDelta = tabResults.reduce((maxDelta, tab) => Math.max(maxDelta, ...(tab.tab_geometry ?? []).flatMap((box, index) => {
  const baseline = geometryBaseline[index];
  if (!baseline || baseline.slug !== box.slug) return [Number.POSITIVE_INFINITY];
  return ['x', 'y', 'width', 'height'].map((key) => Math.abs(box[key] - baseline[key]));
})), 0);
const allTabsGeometryStable = tabResults.length === tabs.length && maxTabGeometryDelta <= 0.01;
const fieldQualityClickFirstThreeMaxDelta = tabResults.find((tab) => tab.slug === 'overview')?.field_quality_click_first_three_max_delta;
const fieldQualityClickGeometryStable = typeof fieldQualityClickFirstThreeMaxDelta === 'number' && fieldQualityClickFirstThreeMaxDelta <= 0.01;
const selectedTabsPassed = selectedTabs.every((expected) => tabResults.some((tab) => (
  tab.slug === expected.slug && tab.active && tab.chart_canvas_count >= 1 && tab.action_drawer_visible && tab.endpoint_visible && tab.audit_event_visible
    && tab.business_scroll?.bottom_reachable && ['auto', 'scroll'].includes(tab.business_scroll?.overflow_y) && tab.business_scroll?.rail_width >= 300
    && tab.business_scroll?.rail_below_tabs && tab.business_scroll?.tab_overflow_x === 'hidden' && tab.business_scroll?.tabs_fit_without_scroll
    && tab.business_scroll?.menu_heading_visible && tab.business_scroll?.menu_heading_text === '数据质量'
    && (tab.slug !== 'overview' || (tab.business_scroll?.overall_score_engine === 'echarts-canvas' && tab.business_scroll?.overall_score_text_overlap === 0))
    && tab.business_scroll?.sticky_tab_delta <= 0.01 && tableContractPassed(tab)
    && reportContractPassed(tab)
    && (!tab.heatmap_geometry || (tab.heatmap_geometry.engine === 'echarts-canvas' && tab.heatmap_geometry.width_fill_ratio >= 0.98 && tab.heatmap_geometry.height_fill_ratio >= 0.98))
    && tab.api_verification?.get_status === 200 && tab.api_verification?.tenant_id === 'default'
    && tab.api_verification?.visual_source === 'postgres-activated-fixture' && tab.api_verification?.fixture_version
    && tab.api_verification?.action_status === 202 && tab.api_verification?.action_id && tab.api_verification?.action_result_status === 'dry_run'
  )));
const currentRunPassed = selectedTabsPassed
  && permissionVerification.read_status === 200
  && permissionVerification.report_read_status === 200
  && permissionVerification.read_only_write_status === 403
  && permissionVerification.no_scope_read_status === 403
  && permissionVerification.no_scope_report_status === 403
  && persistedActions === tabs.length
  && persistedAudits === tabs.length
  && badResponses.length === 0
  && consoleErrors.length === 0
  && pageErrors.length === 0
  && requestFailures.length === 0;
const result = {
  result: allTabsPassed
    && allTabsGeometryStable
    && fieldQualityClickGeometryStable
    && permissionVerification.read_status === 200
    && permissionVerification.report_read_status === 200
    && permissionVerification.read_only_write_status === 403
    && permissionVerification.no_scope_read_status === 403
    && permissionVerification.no_scope_report_status === 403
    && persistedActions === tabs.length
    && persistedAudits === tabs.length
    && mergedBadResponses.length === 0
    && mergedConsoleErrors.length === 0
    && mergedPageErrors.length === 0
    && mergedRequestFailures.length === 0 ? 'pass' : 'partial',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  all_tabs_geometry_stable: allTabsGeometryStable,
  max_tab_geometry_delta: Number.isFinite(maxTabGeometryDelta) ? maxTabGeometryDelta : null,
  field_quality_click_geometry_stable: fieldQualityClickGeometryStable,
  field_quality_click_first_three_max_delta: fieldQualityClickFirstThreeMaxDelta ?? null,
  permission_verification: permissionVerification,
  persistence_verification: { requested_by: actionRunUserId, actions: persistedActions, audits: persistedAudits },
  tab_results: tabResults.sort((left, right) => tabs.findIndex((tab) => tab.slug === left.slug) - tabs.findIndex((tab) => tab.slug === right.slug)),
  bad_responses: mergedBadResponses,
  console_errors: mergedConsoleErrors,
  page_errors: mergedPageErrors,
  request_failures: mergedRequestFailures,
  data_table_requests: dataTableRequests,
  timestamp: new Date().toISOString(),
};

fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(currentRunPassed ? 0 : 1);

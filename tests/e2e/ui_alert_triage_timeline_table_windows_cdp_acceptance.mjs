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
const revision = process.env.TRAFFIC_UI_REVISION || 'r822';
const evidenceDir = path.join(root, 'doc/02_acceptance/02-regression/ui-visual-interaction');
const outputPath = path.join(evidenceDir, `windows-chrome-cdp-alert-triage-${revision}-timeline-table.json`);
const desktopScreenshotPath = path.join(evidenceDir, `windows-chrome-cdp-alert-triage-${revision}-timeline-table.png`);
const constrainedScreenshotPath = path.join(evidenceDir, `windows-chrome-cdp-alert-triage-${revision}-timeline-table-constrained.png`);

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

async function inspectLayout(page) {
  return page.evaluate(() => {
    const timeline = document.querySelector('.taf-alert-timeline');
    const nodes = timeline
      ? Array.from(timeline.querySelectorAll('.taf-alert-timeline__item i'))
      : [];
    const rows = Array.from(document.querySelectorAll('.taf-alert-table .ant-table-tbody > tr.ant-table-row'));
    const table = document.querySelector('.taf-alert-table');
    const tableBody = document.querySelector('.taf-alert-table .ant-table-body');
    const tableContent = document.querySelector('.taf-alert-table .ant-table-content');
    const tableSurface = tableBody ?? tableContent;
    const pagination = document.querySelector('.taf-alert-table .ant-table-pagination');
    const pseudo = timeline ? getComputedStyle(timeline, '::before') : null;
    const timelineRect = timeline?.getBoundingClientRect();
    const nodeRects = nodes.map((node) => node.getBoundingClientRect());
    const firstNodeRect = nodeRects[0];
    const lastNodeRect = nodeRects.at(-1);
    const lineTop = timelineRect && pseudo ? timelineRect.top + Number.parseFloat(pseudo.top) : null;
    const lineBottom = timelineRect && pseudo
      ? timelineRect.bottom - Number.parseFloat(pseudo.bottom)
      : null;
    const firstNodeCenter = firstNodeRect ? firstNodeRect.top + firstNodeRect.height / 2 : null;
    const lastNodeCenter = lastNodeRect ? lastNodeRect.top + lastNodeRect.height / 2 : null;
    const nodeCentersX = nodeRects.map((rect) => rect.left + rect.width / 2);
    const lineCenterX = timelineRect && pseudo
      ? timelineRect.left + Number.parseFloat(pseudo.left) + Number.parseFloat(pseudo.width) / 2
      : null;
    const tableRect = table?.getBoundingClientRect();
    const paginationRect = pagination?.getBoundingClientRect();
    const firstRowRect = rows[0]?.getBoundingClientRect();
    const lastRowRect = rows.at(-1)?.getBoundingClientRect();
    const surfaceStyle = tableSurface ? getComputedStyle(tableSurface) : null;
    const originalScrollTop = tableSurface?.scrollTop ?? 0;
    if (tableSurface) tableSurface.scrollTop = originalScrollTop + 1;
    const canScrollVertically = Boolean(tableSurface && tableSurface.scrollTop > originalScrollTop);
    if (tableSurface) tableSurface.scrollTop = originalScrollTop;
    const visibleNodeCount = nodes.filter((node, index) => {
      const style = getComputedStyle(node);
      const rect = nodeRects[index];
      return style.display !== 'none'
        && style.visibility !== 'hidden'
        && Number.parseFloat(style.opacity) > 0
        && rect.width > 0
        && rect.height > 0;
    }).length;
    return {
      viewport: { width: innerWidth, height: innerHeight },
      user_agent: navigator.userAgent,
      platform: navigator.platform,
      page_size_text: document.querySelector('.taf-alert-table .ant-select-selection-item')?.textContent?.trim() ?? '',
      row_count: rows.length,
      timeline_node_count: nodes.length,
      visible_timeline_node_count: visibleNodeCount,
      axis: {
        content: pseudo?.content ?? '',
        display: pseudo?.display ?? '',
        visibility: pseudo?.visibility ?? '',
        opacity: pseudo ? Number.parseFloat(pseudo.opacity) : 0,
        width: pseudo ? Number.parseFloat(pseudo.width) : 0,
        height: lineTop !== null && lineBottom !== null ? lineBottom - lineTop : 0,
        background: pseudo?.backgroundColor ?? '',
        background_alpha: pseudo
          ? Number.parseFloat(pseudo.backgroundColor.match(/rgba?\([^,]+,[^,]+,[^,]+(?:,\s*([^)]+))?\)/)?.[1] ?? '1')
          : 0,
        line_top: lineTop,
        line_bottom: lineBottom,
        line_center_x: lineCenterX,
        first_node_center: firstNodeCenter,
        last_node_center: lastNodeCenter,
        node_centers_x: nodeCentersX,
        aligned_x: lineCenterX !== null
          && nodeCentersX.length === 5
          && nodeCentersX.every((center) => Math.abs(lineCenterX - center) <= 1),
        aligned_first: lineTop !== null && firstNodeCenter !== null && Math.abs(lineTop - firstNodeCenter) <= 1,
        aligned_last: lineBottom !== null && lastNodeCenter !== null && Math.abs(lineBottom - lastNodeCenter) <= 1,
      },
      table_present: Boolean(table),
      table_body_present: Boolean(tableBody),
      table_content_present: Boolean(tableContent),
      table_surface_present: Boolean(tableSurface),
      table_body_client_height: tableBody?.clientHeight ?? null,
      table_body_scroll_height: tableBody?.scrollHeight ?? null,
      table_surface_client_height: tableSurface?.clientHeight ?? null,
      table_surface_scroll_height: tableSurface?.scrollHeight ?? null,
      table_vertical_scroll: Boolean(tableSurface && tableSurface.scrollHeight > tableSurface.clientHeight + 1),
      table_surface_overflow_y: surfaceStyle?.overflowY ?? '',
      table_surface_can_scroll_vertically: canScrollVertically,
      rows_geometry: {
        first_top: firstRowRect?.top ?? null,
        last_bottom: lastRowRect?.bottom ?? null,
        table_bottom: tableRect?.bottom ?? null,
        pagination_top: paginationRect?.top ?? null,
        all_rows_inside_table: Boolean(
          firstRowRect
          && lastRowRect
          && tableRect
          && firstRowRect.top >= tableRect.top - 1
          && lastRowRect.bottom <= tableRect.bottom + 1,
        ),
        rows_before_pagination: Boolean(lastRowRect && paginationRect && lastRowRect.bottom <= paginationRect.top + 1),
      },
      document_horizontal_overflow: document.documentElement.scrollWidth > innerWidth + 1,
    };
  });
}

const layoutSettled = async (page, predicate) => {
  await page.evaluate(async () => {
    await document.fonts.ready;
    return true;
  });
  await page.waitForFunction(predicate, undefined, { timeout: 15_000, polling: 100 });
  await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
};

const axisPasses = (snapshot) => snapshot.timeline_node_count === 5
  && snapshot.visible_timeline_node_count === 5
  && snapshot.axis.content !== 'none'
  && snapshot.axis.display !== 'none'
  && snapshot.axis.visibility !== 'hidden'
  && snapshot.axis.opacity > 0
  && snapshot.axis.background_alpha > 0
  && snapshot.axis.width >= 1
  && snapshot.axis.height > 40
  && snapshot.axis.aligned_x
  && snapshot.axis.aligned_first
  && snapshot.axis.aligned_last;

const version = await (await fetch(`${cdpUrl}/json/version`)).json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const badResponses = [];
const requestFailures = [];
const consoleErrors = [];
const pageErrors = [];

page.on('response', (response) => {
  if (response.status() >= 400 && !response.url().startsWith('chrome-extension://')) {
    badResponses.push({ status: response.status(), url: response.url() });
  }
});
page.on('requestfailed', (request) => requestFailures.push({
  url: request.url(),
  error: request.failure()?.errorText ?? 'unknown',
}));
page.on('console', (entry) => {
  if (entry.type() === 'error' && !entry.location().url.startsWith('chrome-extension://')) {
    consoleErrors.push(entry.text());
  }
});
page.on('pageerror', (error) => pageErrors.push(error.message));

await page.setViewportSize({ width: 2560, height: 1080 });
const route = new URL(`/alerts?timelineTableAcceptance=${Date.now()}`, baseUrl);
route.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(route.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-alert-table .ant-table-tbody > tr.ant-table-row').nth(9).waitFor({ state: 'visible', timeout: 30_000 });
await page.waitForFunction(() => document.querySelectorAll('.taf-alert-timeline__item i').length === 5);
await layoutSettled(page, () => {
  const rows = document.querySelectorAll('.taf-alert-table .ant-table-tbody > tr.ant-table-row');
  const surface = document.querySelector('.taf-alert-table .ant-table-body')
    ?? document.querySelector('.taf-alert-table .ant-table-content');
  return rows.length === 10
    && Boolean(surface)
    && surface.scrollHeight <= surface.clientHeight + 1;
});
const desktop = await inspectLayout(page);
await page.screenshot({ path: desktopScreenshotPath, fullPage: false });

await page.setViewportSize({ width: 1600, height: 720 });
await layoutSettled(page, () => {
  const rows = document.querySelectorAll('.taf-alert-table .ant-table-tbody > tr.ant-table-row');
  const body = document.querySelector('.taf-alert-table .ant-table-body');
  return rows.length === 10
    && Boolean(body)
    && body.scrollHeight > body.clientHeight + 1;
});
const constrained = await inspectLayout(page);
await page.screenshot({ path: constrainedScreenshotPath, fullPage: false });

const windowsChromePass = /Windows NT/i.test(desktop.user_agent)
  && /^Win/i.test(desktop.platform)
  && /^Chrome\//.test(version.Browser);
const viewportPass = desktop.viewport.width === 2560
  && desktop.viewport.height === 1080
  && constrained.viewport.width === 1600
  && constrained.viewport.height === 720;
const axisPass = axisPasses(desktop) && axisPasses(constrained);
const desktopTablePass = desktop.table_present
  && desktop.table_surface_present
  && desktop.row_count === 10
  && /10\s*条\s*\/\s*页/.test(desktop.page_size_text)
  && !desktop.table_vertical_scroll
  && desktop.rows_geometry.all_rows_inside_table
  && desktop.rows_geometry.rows_before_pagination;
const constrainedTablePass = constrained.row_count === 10
  && constrained.table_body_present
  && constrained.table_surface_present
  && constrained.table_vertical_scroll
  && /^(auto|scroll)$/.test(constrained.table_surface_overflow_y)
  && constrained.table_surface_can_scroll_vertically;

const result = {
  result: windowsChromePass
    && viewportPass
    && axisPass
    && desktopTablePass
    && constrainedTablePass
    && !desktop.document_horizontal_overflow
    && !constrained.document_horizontal_overflow
    && badResponses.length === 0
    && requestFailures.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0
      ? 'pass'
      : 'fail',
  browser: version.Browser,
  browser_backend: 'Windows Chrome CDP over Xshell tunnel',
  cdp_mapping: '127.0.0.1:9224 -> Windows 127.0.0.1:9222',
  route: '/alerts',
  checks: {
    windows_chrome: windowsChromePass,
    exact_viewports: viewportPass,
    axis_pass: axisPass,
    desktop_ten_rows_without_vertical_scroll: desktopTablePass,
    constrained_ten_rows_with_vertical_scroll: constrainedTablePass,
  },
  desktop,
  constrained,
  bad_responses: badResponses,
  request_failures: requestFailures,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  screenshots: {
    desktop: path.relative(root, desktopScreenshotPath),
    constrained: path.relative(root, constrainedScreenshotPath),
  },
  timestamp: new Date().toISOString(),
};

fs.mkdirSync(evidenceDir, { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close();
await browser.close();
if (result.result !== 'pass') process.exitCode = 1;

#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = process.env.UI_BASE_URL ?? 'http://10.0.5.8:30180';
const cdpUrl = process.env.UI_CDP_URL ?? 'http://127.0.0.1:9224';
const viewports = [
  { width: 1600, height: 900 },
  { width: 1366, height: 768 },
];
const tabs = ['overview', 'topic-health', 'flink-quality', 'field-quality', 'storage-quality', 'replay-reconcile', 'report', 'settings'];
const evidenceDir = path.join(root, 'evidence/ui-responsive/probes-data-quality');
const outputPath = path.join(evidenceDir, 'windows-chrome-responsive-latest.json');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const encoded = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
    { encoding: 'utf8', env: process.env, timeout: 30_000 },
  );
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-responsive-acceptance',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'probe:read', 'probe:write', 'data-quality:read', 'data-quality:write', 'dlq:replay'],
    token_type: 'access',
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

function routeWithToken(route, token) {
  const url = new URL(route, baseUrl);
  url.searchParams.set('responsiveAcceptanceTs', String(Date.now()));
  url.hash = `codex_smoke_token=${token}`;
  return url.toString();
}

function record(checks, name, passed, details = undefined) {
  checks.push({ name, passed: Boolean(passed), ...(details === undefined ? {} : { details }) });
}

async function setViewport(page, cdp, viewport) {
  await page.bringToFront();
  await cdp.send('Emulation.setDeviceMetricsOverride', { ...viewport, deviceScaleFactor: 1, mobile: false });
  await page.waitForTimeout(150);
  const uncalibrated = await page.evaluate(() => ({ width: innerWidth, height: innerHeight }));
  const hostZoom = viewport.width / uncalibrated.width;
  await cdp.send('Emulation.setDeviceMetricsOverride', {
    width: Math.round(viewport.width * hostZoom),
    height: Math.round(viewport.height * hostZoom),
    deviceScaleFactor: 1 / hostZoom,
    mobile: false,
  });
  await page.waitForTimeout(250);
  return page.evaluate(() => ({ width: innerWidth, height: innerHeight }));
}

async function capture(cdp, filePath) {
  const shot = await cdp.send('Page.captureScreenshot', {
    format: 'png',
    fromSurface: true,
    captureBeyondViewport: false,
  });
  fs.writeFileSync(filePath, Buffer.from(shot.data, 'base64'));
}

async function waitForRoute(page, rootSelector) {
  await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});
  await page.locator(rootSelector).waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForTimeout(500);
}

async function probeTargetReachability(page, selector) {
  return page.locator(selector).first().evaluate((target) => {
    const business = document.querySelector('.taf-probes');
    if (!business) return { found: false, visible_ratio: 0 };
    target.scrollIntoView({ block: 'center', inline: 'nearest' });
    const targetBox = target.getBoundingClientRect();
    const businessBox = business.getBoundingClientRect();
    const visibleHeight = Math.max(0, Math.min(targetBox.bottom, businessBox.bottom) - Math.max(targetBox.top, businessBox.top));
    return {
      found: true,
      visible_ratio: Number((visibleHeight / Math.min(targetBox.height, businessBox.height)).toFixed(4)),
      target: { top: targetBox.top, bottom: targetBox.bottom, height: targetBox.height },
      business: { top: businessBox.top, bottom: businessBox.bottom, height: businessBox.height },
    };
  });
}

async function verifyProbes(page, cdp, token, viewport) {
  const checks = [];
  await page.goto(routeWithToken('/probes', token), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await waitForRoute(page, '.taf-probes');
  await setViewport(page, cdp, viewport);
  await page.waitForTimeout(350);
  await page.locator('.taf-probes-trend-echart canvas').first().waitFor({ state: 'visible', timeout: 15_000 });
  const prefix = `probes-${viewport.width}x${viewport.height}`;
  await capture(cdp, path.join(evidenceDir, `${prefix}-top.png`));

  const layout = await page.locator('.taf-probes').evaluate((business) => {
    const businessBox = business.getBoundingClientRect();
    const overlapPairs = [];
    for (const groupSelector of ['.taf-probes-main', '.taf-probes-bottom', '.taf-probes-rail']) {
      const group = business.querySelector(groupSelector);
      if (!group) continue;
      const children = [...group.children].map((element) => ({ element, box: element.getBoundingClientRect() }))
        .filter(({ box }) => box.width > 1 && box.height > 1)
        .sort((left, right) => left.box.top - right.box.top);
      for (let index = 1; index < children.length; index += 1) {
        const verticalOverlap = children[index - 1].box.bottom - children[index].box.top;
        const horizontalOverlap = Math.min(children[index - 1].box.right, children[index].box.right)
          - Math.max(children[index - 1].box.left, children[index].box.left);
        if (verticalOverlap > 1 && horizontalOverlap > 1) {
          overlapPairs.push({
            group: groupSelector,
            previous: children[index - 1].element.className,
            next: children[index].element.className,
            vertical_overlap: Number(verticalOverlap.toFixed(3)),
            horizontal_overlap: Number(horizontalOverlap.toFixed(3)),
          });
        }
      }
    }
    const clipped = [...business.querySelectorAll('.taf-panel, button, canvas')].filter((element) => {
      const box = element.getBoundingClientRect();
      return box.width > 1 && box.height > 1 && (box.left < businessBox.left - 2 || box.right > businessBox.right + 2);
    }).map((element) => ({ class_name: element.className, text: element.textContent?.trim().slice(0, 80) ?? '' }));
    const statusRowHeights = [...business.querySelectorAll('.taf-probes-status-row')].map((row) => row.getBoundingClientRect().height);
    const topologySvgHeight = business.querySelector('.taf-probes-topology-svg')?.getBoundingClientRect().height ?? 0;
    const contentContainment = ['.taf-probes-detail', '.taf-probes-batch'].map((selector) => {
      const content = business.querySelector(selector);
      const body = content?.closest('.taf-panel__body');
      const contentBox = content?.getBoundingClientRect();
      const bodyBox = body?.getBoundingClientRect();
      return {
        selector,
        contained: Boolean(contentBox && bodyBox && contentBox.top >= bodyBox.top - 1 && contentBox.bottom <= bodyBox.bottom + 1),
        content_bottom: contentBox?.bottom,
        body_bottom: bodyBox?.bottom,
      };
    });
    const groupContainment = ['.taf-probes-main', '.taf-probes-bottom', '.taf-probes-rail'].map((selector) => {
      const group = business.querySelector(selector);
      const groupBox = group?.getBoundingClientRect();
      const children = [...(group?.children ?? [])]
        .map((element) => element.getBoundingClientRect())
        .filter((box) => box.width > 1 && box.height > 1);
      return {
        selector,
        contained: Boolean(groupBox && children.every((box) => (
          box.top >= groupBox.top - 1
          && box.bottom <= groupBox.bottom + 1
          && box.left >= groupBox.left - 1
          && box.right <= groupBox.right + 1
        ))),
        group_bottom: groupBox?.bottom,
        max_child_bottom: children.length ? Math.max(...children.map((box) => box.bottom)) : null,
      };
    });
    const style = getComputedStyle(business);
    return {
      viewport: { width: innerWidth, height: innerHeight },
      root_horizontal_overflow: document.documentElement.scrollWidth > document.documentElement.clientWidth + 2,
      business_horizontal_overflow: business.scrollWidth > business.clientWidth + 2,
      horizontal_overflow_delta: business.scrollWidth - business.clientWidth,
      client_width: business.clientWidth,
      scroll_width: business.scrollWidth,
      overflow_y: style.overflowY,
      client_height: business.clientHeight,
      scroll_height: business.scrollHeight,
      topology_svg_height: topologySvgHeight,
      status_row_min_height: statusRowHeights.length ? Math.min(...statusRowHeights) : 0,
      overlap_pairs: overlapPairs,
      content_containment: contentContainment,
      group_containment: groupContainment,
      shell_box: (() => { const box = business.querySelector('.taf-probes-shell')?.getBoundingClientRect(); return box ? { top: box.top, bottom: box.bottom, height: box.height } : null; })(),
      main_box: (() => { const box = business.querySelector('.taf-probes-main')?.getBoundingClientRect(); return box ? { top: box.top, bottom: box.bottom, height: box.height } : null; })(),
      rail_box: (() => { const box = business.querySelector('.taf-probes-rail')?.getBoundingClientRect(); return box ? { top: box.top, bottom: box.bottom, height: box.height } : null; })(),
      clipped,
    };
  });
  record(checks, 'viewport-exact', layout.viewport.width === viewport.width && layout.viewport.height === viewport.height, layout.viewport);
  record(checks, 'no-horizontal-overflow', !layout.root_horizontal_overflow && layout.clipped.length === 0 && layout.horizontal_overflow_delta <= 12, layout);
  record(checks, 'business-scroll-owner', ['auto', 'scroll'].includes(layout.overflow_y) && layout.scroll_height > layout.client_height, layout);
  record(
    checks,
    'modules-have-stable-height-and-no-overlap',
    layout.topology_svg_height >= 250
      && layout.status_row_min_height >= 20
      && layout.overlap_pairs.length === 0
      && layout.content_containment.every((item) => item.contained)
      && layout.group_containment.every((item) => item.contained)
      && layout.shell_box
      && layout.main_box
      && layout.rail_box
      && layout.shell_box.bottom >= Math.max(layout.main_box.bottom, layout.rail_box.bottom) - 1
      && (viewport.width >= 1440 || layout.rail_box.top >= layout.main_box.bottom + 6),
    layout,
  );

  for (const [name, selector] of [
    ['status-pagination-reachable', '.taf-probes-pagination'],
    ['trend-panel-reachable', '.taf-probes-trends-panel'],
    ['threshold-chart-reachable', '.taf-probes-threshold'],
    ['heartbeat-panel-reachable', '.taf-probes-rail .taf-panel:last-child'],
  ]) {
    const reachability = await probeTargetReachability(page, selector);
    record(checks, name, reachability.found && reachability.visible_ratio >= 0.95, reachability);
  }

  await page.locator('.taf-probes').evaluate((business) => { business.scrollTop = business.scrollHeight; });
  await page.waitForTimeout(300);
  await capture(cdp, path.join(evidenceDir, `${prefix}-bottom.png`));
  await page.locator('.taf-probes').evaluate((business) => { business.scrollTop = 0; });
  return { viewport, checks, passed: checks.every((check) => check.passed) };
}

async function verifyDataQualityTab(page, cdp, token, viewport, tab) {
  const checks = [];
  await page.goto(routeWithToken(`/data-quality?tab=${tab}`, token), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await waitForRoute(page, '.taf-data-quality-shell');
  await setViewport(page, cdp, viewport);
  await page.waitForTimeout(350);
  await page.locator(`.taf-data-quality-tabs button[data-tab-slug="${tab}"][aria-selected="true"]`).waitFor({ state: 'visible', timeout: 15_000 });
  const prefix = `data-quality-${tab}-${viewport.width}x${viewport.height}`;
  await capture(cdp, path.join(evidenceDir, `${prefix}-top.png`));

  const layout = await page.locator('.taf-data-quality-shell').evaluate((shell, activeTab) => {
    const shellBox = shell.getBoundingClientRect();
    const titlebar = shell.querySelector('.taf-data-quality-titlebar');
    const titleBox = titlebar?.getBoundingClientRect();
    const heading = shell.querySelector('.taf-data-quality-heading h1');
    const tabs = shell.querySelector('.taf-data-quality-tabs');
    const tabsBox = tabs?.getBoundingClientRect();
    const mainBox = shell.querySelector('.taf-data-quality-main')?.getBoundingClientRect();
    const railBox = shell.querySelector('.taf-data-quality-rail')?.getBoundingClientRect();
    const tabButtons = [...(tabs?.querySelectorAll('button') ?? [])].map((button) => button.getBoundingClientRect());
    const horizontalClips = [...shell.querySelectorAll('.taf-panel, button, canvas')].filter((element) => {
      const box = element.getBoundingClientRect();
      const tolerance = element.tagName === 'CANVAS' ? 5 : 2;
      return box.width > 1 && box.height > 1 && (box.left < shellBox.left - tolerance || box.right > shellBox.right + tolerance);
    }).map((element) => {
      const box = element.getBoundingClientRect();
      return {
        tag_name: element.tagName,
        class_name: element.className,
        text: element.textContent?.trim().slice(0, 80) ?? '',
        parent_class_name: element.parentElement?.className ?? '',
        box: { left: box.left, right: box.right, width: box.width },
        shell: { left: shellBox.left, right: shellBox.right, width: shellBox.width },
      };
    });
    const style = getComputedStyle(shell);
    const score = shell.querySelector('.taf-data-quality-metric.is-overall-score');
    const ringBox = score?.querySelector('.taf-data-quality-overall-score-ring')?.getBoundingClientRect();
    const copyBox = score?.querySelector('.taf-data-quality-overall-score-copy')?.getBoundingClientRect();
    const overlap = ringBox && copyBox
      ? Math.max(0, Math.min(ringBox.right, copyBox.right) - Math.max(ringBox.left, copyBox.left))
        * Math.max(0, Math.min(ringBox.bottom, copyBox.bottom) - Math.max(ringBox.top, copyBox.top))
      : 0;
    const exportTable = shell.querySelector('.taf-data-quality-report-api-table.is-export');
    const exportBox = exportTable?.getBoundingClientRect();
    const exportStyle = exportTable ? getComputedStyle(exportTable) : null;
    const exportButtons = [...(exportTable?.querySelectorAll('button') ?? [])].map((button) => button.getBoundingClientRect());
    const approvalBox = shell.querySelector('.taf-data-quality-report-approval-panel')?.getBoundingClientRect();
    return {
      active_tab: activeTab,
      viewport: { width: innerWidth, height: innerHeight },
      overflow_y: style.overflowY,
      client_height: shell.clientHeight,
      scroll_height: shell.scrollHeight,
      root_horizontal_overflow: document.documentElement.scrollWidth > document.documentElement.clientWidth + 2,
      shell_horizontal_overflow: shell.scrollWidth > shell.clientWidth + 2,
      horizontal_clips: horizontalClips,
      heading_visible: Boolean(heading && getComputedStyle(heading).display !== 'none' && heading.getBoundingClientRect().height > 0),
      heading_text: heading?.textContent?.trim() ?? '',
      tabs_horizontal_scroll: Boolean(tabs && tabs.scrollWidth > tabs.clientWidth + 2),
      tabs_contained: Boolean(tabsBox && tabButtons.length === 8 && tabButtons.every((box) => box.left >= tabsBox.left - 1 && box.right <= tabsBox.right + 1)),
      title_starts_at_shell: Boolean(titleBox && titleBox.top >= shellBox.top - 1),
      content_below_title: Boolean(titleBox && mainBox && mainBox.top >= titleBox.bottom - 1 && (!railBox || railBox.top >= titleBox.bottom - 1)),
      ring_engine: score?.querySelector('canvas') ? 'echarts-canvas' : 'missing',
      ring_copy_overlap_area: Number(overlap.toFixed(3)),
      export_internal_scroll: Boolean(exportTable && exportStyle && ['auto', 'scroll'].includes(exportStyle.overflowY) && exportTable.scrollHeight > exportTable.clientHeight + 1),
      export_actions_contained: Boolean(exportBox && exportButtons.length > 0 && exportButtons.every((box) => box.left >= exportBox.left - 1 && box.right <= exportBox.right + 1)),
      export_box: exportBox ? { left: exportBox.left, right: exportBox.right, width: exportBox.width } : null,
      export_button_boxes: exportButtons.map((box) => ({ left: box.left, right: box.right, width: box.width })),
      approval_contained: Boolean(!approvalBox || (approvalBox.left >= shellBox.left - 1 && approvalBox.right <= shellBox.right + 1)),
    };
  }, tab);

  record(checks, 'viewport-exact', layout.viewport.width === viewport.width && layout.viewport.height === viewport.height, layout.viewport);
  record(checks, 'no-horizontal-overflow', !layout.root_horizontal_overflow && !layout.shell_horizontal_overflow && layout.horizontal_clips.length === 0, layout);
  record(checks, 'menu-heading-visible', layout.heading_visible && layout.heading_text === '数据质量', { visible: layout.heading_visible, text: layout.heading_text });
  record(checks, 'tabs-fixed-contained', !layout.tabs_horizontal_scroll && layout.tabs_contained, layout);
  record(checks, 'content-below-titlebar', layout.title_starts_at_shell && layout.content_below_title, layout);
  record(checks, 'business-scroll-owner', ['auto', 'scroll'].includes(layout.overflow_y) && layout.scroll_height > layout.client_height, layout);
  if (tab === 'overview') record(checks, 'quality-score-echarts-no-overlap', layout.ring_engine === 'echarts-canvas' && layout.ring_copy_overlap_area === 0, layout);
  if (tab === 'report') {
    record(checks, 'export-scroll-and-actions-contained', layout.export_internal_scroll && layout.export_actions_contained, layout);
    record(checks, 'approval-right-edge-contained', layout.approval_contained, layout);
  }

  await page.locator('.taf-data-quality-shell').evaluate((shell) => { shell.scrollTop = shell.scrollHeight; });
  await page.waitForTimeout(250);
  const bottomReachable = await page.locator('.taf-data-quality-shell').evaluate((shell) => Math.abs(shell.scrollTop + shell.clientHeight - shell.scrollHeight) <= 3);
  record(checks, 'business-bottom-reachable', bottomReachable);
  if (tab === 'overview' || tab === 'report') await capture(cdp, path.join(evidenceDir, `${prefix}-bottom.png`));
  await page.locator('.taf-data-quality-shell').evaluate((shell) => { shell.scrollTop = 0; });
  return { tab, checks, passed: checks.every((check) => check.passed) };
}

fs.mkdirSync(evidenceDir, { recursive: true });
const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const cdp = await context.newCDPSession(page);
const token = smokeToken();
const runtime = { bad_responses: [], console_errors: [], page_errors: [], request_failures: [] };
page.on('response', (response) => {
  if (response.url().startsWith(baseUrl) && response.status() >= 400) runtime.bad_responses.push({ status: response.status(), url: response.url() });
});
page.on('console', (entry) => {
  if (entry.type() === 'error' && !entry.text().includes('api.yhchj.com') && !entry.text().includes('ERR_CONNECTION_CLOSED')) runtime.console_errors.push(entry.text());
});
page.on('pageerror', (error) => {
  if (error.message !== 'Object' && !error.stack?.includes('chrome-extension://')) runtime.page_errors.push({ message: error.message, stack: error.stack });
});
page.on('requestfailed', (request) => {
  if (request.url().startsWith(baseUrl)) runtime.request_failures.push({ url: request.url(), error: request.failure()?.errorText ?? 'unknown' });
});

const results = [];
for (const viewport of viewports) {
  const observed = await setViewport(page, cdp, viewport);
  const probes = await verifyProbes(page, cdp, token, viewport);
  const dataQuality = [];
  for (const tab of tabs) dataQuality.push(await verifyDataQualityTab(page, cdp, token, viewport, tab));
  results.push({ viewport, observed, probes, data_quality: dataQuality });
}

await page.close();
await browser.close();
const checksPassed = results.every((result) => result.probes.passed && result.data_quality.every((tab) => tab.passed));
const runtimePassed = Object.values(runtime).every((entries) => entries.length === 0);
const report = {
  generated_at: new Date().toISOString(),
  browser: version.Browser,
  base_url: baseUrl,
  viewports,
  results,
  runtime,
  checks_passed: checksPassed,
  runtime_passed: runtimePassed,
  result: checksPassed && runtimePassed ? 'pass' : 'fail',
};
fs.writeFileSync(outputPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify({ output: outputPath, result: report.result, checks_passed: checksPassed, runtime_passed: runtimePassed }, null, 2));
if (report.result !== 'pass') process.exitCode = 1;

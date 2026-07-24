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
const revision = process.env.CAMPAIGN_EVIDENCE_REVISION ?? 'r716';
const viewports = [1920, 1728, 1600, 1440, 1366, 1280].map((width) => ({ width, height: 1080 }));
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/campaigns', `responsive-${revision}`);
const outputPath = path.join(root, 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-campaign-responsive-latest.json');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function token() {
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
    username: 'codex-campaign-responsive',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:*', 'graph:read', 'playbook:execute'],
    token_type: 'access',
    session_id: crypto.randomUUID(),
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

async function setViewport(page, cdp, viewport) {
  await page.bringToFront();
  await cdp.send('Emulation.setDeviceMetricsOverride', { ...viewport, deviceScaleFactor: 1, mobile: false });
  await page.waitForTimeout(150);
  const inner = await page.evaluate(() => ({ width: innerWidth, height: innerHeight }));
  const hostZoom = viewport.width / inner.width;
  await cdp.send('Emulation.setDeviceMetricsOverride', {
    width: Math.round(viewport.width * hostZoom),
    height: Math.round(viewport.height * hostZoom),
    deviceScaleFactor: 1 / hostZoom,
    mobile: false,
  });
  await page.waitForTimeout(300);
}

const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const cdp = await page.context().newCDPSession(page);
fs.mkdirSync(evidenceDir, { recursive: true });
const accessToken = token();
const runs = [];

for (const viewport of viewports) {
  const url = new URL('/campaigns', baseUrl);
  url.searchParams.set('campaignResponsiveTs', String(Date.now()));
  url.hash = `codex_smoke_token=${accessToken}`;
  await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  await page.locator('.taf-campaign-workbench').waitFor({ state: 'visible', timeout: 20_000 });
  await setViewport(page, cdp, viewport);
  await page.waitForTimeout(500);
  const metrics = await page.evaluate(() => {
    const box = (selector) => {
      const rect = document.querySelector(selector)?.getBoundingClientRect();
      return rect ? { x: rect.x, y: rect.y, width: rect.width, height: rect.height, right: rect.right, bottom: rect.bottom } : null;
    };
    const business = document.querySelector('.taf-main');
    const campaign = document.querySelector('.taf-campaign-workbench');
    const table = document.querySelector('.taf-campaign-list-panel .ant-table-content');
    const main = box('.taf-campaign-main');
    const rail = box('.taf-campaign-rail');
    const list = box('.taf-campaign-list-panel');
    const attack = box('.taf-campaign-attack-panel');
    const overflowY = business ? getComputedStyle(business).overflowY : '';
    const businessRect = business?.getBoundingClientRect();
    const campaignRect = campaign?.getBoundingClientRect();
    const businessStyle = business ? getComputedStyle(business) : null;
    const attackGraph = document.querySelector('[data-chart-engine="echarts"][data-series-type="graph"]');
    const attackGraphCanvas = attackGraph?.querySelector('canvas');
    const attackGraphRect = attackGraph?.getBoundingClientRect();
    const attackGraphCanvasRect = attackGraphCanvas?.getBoundingClientRect();
    const overflowCandidates = campaign
      ? [...campaign.querySelectorAll('*')]
        .filter((element) => element.scrollWidth > element.clientWidth + 2)
        .slice(0, 20)
        .map((element) => ({
          tag: element.tagName,
          class_name: typeof element.className === 'string' ? element.className : '',
          client_width: element.clientWidth,
          scroll_width: element.scrollWidth,
          overflow_x: getComputedStyle(element).overflowX,
        }))
      : [];
    return {
      viewport: { width: innerWidth, height: innerHeight },
      root_horizontal_overflow: document.documentElement.scrollWidth > document.documentElement.clientWidth + 2,
      campaign_horizontal_overflow: Boolean(campaign && campaign.scrollWidth > campaign.clientWidth + 2),
      business_scroll: {
        overflow_y: overflowY,
        client_height: business?.clientHeight ?? 0,
        scroll_height: business?.scrollHeight ?? 0,
      },
      public_business_spacing: {
        actual_left: campaignRect?.left ?? 0,
        expected_left: (businessRect?.left ?? 0) + Number.parseFloat(businessStyle?.paddingLeft ?? '0'),
        actual_top: campaignRect?.top ?? 0,
        expected_top: (businessRect?.top ?? 0) + Number.parseFloat(businessStyle?.paddingTop ?? '0'),
      },
      attack_graph: {
        engine: attackGraph?.getAttribute('data-chart-engine') ?? '',
        series_type: attackGraph?.getAttribute('data-series-type') ?? '',
        canvas_count: attackGraph?.querySelectorAll('canvas').length ?? 0,
        width: attackGraphRect?.width ?? 0,
        height: attackGraphRect?.height ?? 0,
        canvas_width: attackGraphCanvasRect?.width ?? 0,
        canvas_height: attackGraphCanvasRect?.height ?? 0,
      },
      table_scroll: {
        client_width: table?.clientWidth ?? 0,
        scroll_width: table?.scrollWidth ?? 0,
        overflow: Boolean(table && table.scrollWidth > table.clientWidth + 2),
      },
      main,
      rail,
      list,
      attack,
      overflow_candidates: overflowCandidates,
      visible_rows: document.querySelectorAll('.taf-campaign-list-panel .ant-table-tbody > tr').length,
      visible_bad_geometry: [...document.querySelectorAll('.taf-campaign-workbench button, .taf-campaign-workbench canvas, .taf-campaign-workbench .taf-panel')]
        .map((element) => ({ element, rect: element.getBoundingClientRect() }))
        .filter(({ rect }) => rect.width > 1 && rect.height > 1 && (rect.left < -2 || rect.right > innerWidth + 2))
        .map(({ element, rect }) => ({ class_name: element.className, text: element.textContent?.trim().slice(0, 60) ?? '', left: rect.left, right: rect.right })),
    };
  });
  const compact = viewport.width < 1800;
  const checks = [
    { name: 'viewport-exact', passed: metrics.viewport.width === viewport.width && metrics.viewport.height === viewport.height },
    { name: 'no-root-or-campaign-horizontal-overflow', passed: !metrics.root_horizontal_overflow && !metrics.campaign_horizontal_overflow },
    { name: 'table-has-no-horizontal-scroll', passed: !metrics.table_scroll.overflow },
    { name: 'no-visible-horizontal-clipping', passed: metrics.visible_bad_geometry.length === 0 },
    { name: 'eight-table-rows-visible', passed: metrics.visible_rows === 8 },
    { name: 'list-and-attack-do-not-overlap', passed: Boolean(metrics.list && metrics.attack && metrics.list.right <= metrics.attack.x + 1) },
    {
      name: 'uses-public-business-area-spacing',
      passed: Math.abs(metrics.public_business_spacing.actual_left - metrics.public_business_spacing.expected_left) <= 1
        && Math.abs(metrics.public_business_spacing.actual_top - metrics.public_business_spacing.expected_top) <= 1,
    },
    {
      name: 'attack-association-is-visible-echarts-graph',
      passed: metrics.attack_graph.engine === 'echarts'
        && metrics.attack_graph.series_type === 'graph'
        && metrics.attack_graph.canvas_count === 1
        && metrics.attack_graph.width >= 300
        && metrics.attack_graph.height >= 300
        && metrics.attack_graph.canvas_width >= 300
        && metrics.attack_graph.canvas_height >= 300,
    },
    {
      name: 'responsive-rail-and-business-scroll',
      passed: compact
        ? Boolean(metrics.main && metrics.rail && metrics.rail.y >= metrics.main.bottom + 6
          && ['auto', 'scroll'].includes(metrics.business_scroll.overflow_y)
          && metrics.business_scroll.scroll_height > metrics.business_scroll.client_height)
        : Boolean(metrics.main && metrics.rail && metrics.main.right <= metrics.rail.x + 1),
    },
  ];
  const screenshot = path.join(evidenceDir, `campaigns-${viewport.width}x${viewport.height}.png`);
  const shot = await cdp.send('Page.captureScreenshot', { format: 'png', fromSurface: true, captureBeyondViewport: false });
  fs.writeFileSync(screenshot, Buffer.from(shot.data, 'base64'));
  runs.push({
    viewport,
    passed: checks.every((check) => check.passed),
    checks,
    metrics,
    screenshot: path.relative(root, screenshot),
  });
}

const result = {
  run_id: `campaign-responsive-${revision}`,
  result: runs.every((run) => run.passed) ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP over Xshell 9224',
  base_url: baseUrl,
  check_count: runs.reduce((total, run) => total + run.checks.length, 0),
  passed: runs.reduce((total, run) => total + run.checks.filter((check) => check.passed).length, 0),
  failed: runs.reduce((total, run) => total + run.checks.filter((check) => !check.passed).length, 0),
  runs,
  timestamp: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close();
process.exit(result.result === 'pass' ? 0 : 1);

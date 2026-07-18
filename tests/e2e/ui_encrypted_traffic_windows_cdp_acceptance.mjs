#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const requireFromUi = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = requireFromUi('@playwright/test');

const defaults = {
  baseUrl: 'http://10.0.5.8:30180',
  cdpUrl: 'http://127.0.0.1:9224',
  width: 1920,
  height: 1080,
  channelTolerance: 64,
  maxPixelRatio: 0.18,
  scoringRegion: '198,78,1712,920',
  scoringRegionId: 'encrypted-traffic-business-roi-v1',
  acceptanceDir: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/encrypted-traffic',
  output: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-encrypted-traffic-latest.json',
};
const args = { ...defaults, ...parseArgs(process.argv.slice(2)) };
const tabs = [
  { slug: 'overview', label: '总览', baseline: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic.png' },
  { slug: 'fingerprint', label: '指纹分析', baseline: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-fingerprint.png' },
  { slug: 'tunnel-detection', label: '隧道检测', baseline: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-tunnel-detection.png' },
  { slug: 'egress-profile', label: '外联画像', baseline: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-egress-profile.png' },
  { slug: 'evidence-center', label: '证据中心', baseline: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-evidence-center.png' },
];

clearProxyEnv();

function parseArgs(argv) {
  const values = {};
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index];
    if (!item.startsWith('--')) throw new Error(`unexpected argument: ${item}`);
    const key = item.slice(2).replace(/-([a-z])/g, (_, char) => char.toUpperCase());
    const next = argv[index + 1];
    if (next === undefined || next.startsWith('--')) values[key] = true;
    else {
      values[key] = next;
      index += 1;
    }
  }
  return values;
}

function clearProxyEnv() {
  for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
  process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
}

function b64url(value) {
  return Buffer.from(value).toString('base64url');
}

function smokeToken() {
  const encoded = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
    { encoding: 'utf8', env: process.env, timeout: 15_000 },
  );
  const now = Math.floor(Date.now() / 1_000);
  const header = { alg: 'HS256', typ: 'JWT' };
  const claims = {
    iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(),
    tenant_id: 'default', username: 'codex-encrypted-windows-cdp', email: 'codex-encrypted-windows-cdp@local',
    roles: ['admin'], permissions: ['*', 'admin:*', 'alert:read', 'alert:write', 'screen:view'], token_type: 'access',
    session_id: `encrypted-windows-cdp-${crypto.randomUUID()}`, iat: now, exp: now + 1_800,
  };
  const signingInput = `${b64url(JSON.stringify(header))}.${b64url(JSON.stringify(claims))}`;
  const signature = crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(signingInput).digest('base64url');
  return `${signingInput}.${signature}`;
}

function authenticatedUrl(route, token) {
  const url = new URL(route, `${String(args.baseUrl).replace(/\/+$/, '')}/`);
  url.hash = new URLSearchParams({ codex_smoke_token: token }).toString();
  return url.toString();
}

function redact(value) {
  return String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
}

function absolute(file) {
  return path.isAbsolute(file) ? file : path.join(root, file);
}

function writeJson(file, value) {
  const destination = absolute(file);
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  fs.writeFileSync(destination, `${JSON.stringify(value, null, 2)}\n`, 'utf8');
}

async function writeSideBySide(page, source, actual, output) {
  const comparisonDataUrl = await page.evaluate(async ({ sourceBase64, actualBase64 }) => {
    const decode = async (base64) => {
      const binary = atob(base64);
      const bytes = new Uint8Array(binary.length);
      for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
      return createImageBitmap(new Blob([bytes], { type: 'image/png' }));
    };
    const [sourceImage, actualImage] = await Promise.all([decode(sourceBase64), decode(actualBase64)]);
    const width = Math.max(sourceImage.width, actualImage.width);
    const height = Math.max(sourceImage.height, actualImage.height);
    const canvas = document.createElement('canvas');
    canvas.width = width * 2;
    canvas.height = height;
    const context = canvas.getContext('2d');
    context.drawImage(sourceImage, 0, 0, width, height);
    context.drawImage(actualImage, width, 0, width, height);
    return canvas.toDataURL('image/png');
  }, {
    sourceBase64: fs.readFileSync(absolute(source)).toString('base64'),
    actualBase64: fs.readFileSync(actual).toString('base64'),
  });
  fs.writeFileSync(output, Buffer.from(comparisonDataUrl.split(',')[1], 'base64'));
}

async function withTimeout(promise, timeoutMs, label) {
  let timer;
  try {
    return await Promise.race([
      promise,
      new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error(`${label} timed out after ${timeoutMs}ms`)), timeoutMs);
      }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}

function addCheck(checks, id, pass, details = {}) {
  checks.push({ id, status: pass ? 'pass' : 'fail', ...details });
}

async function cdpPreflight() {
  const base = String(args.cdpUrl).replace(/\/+$/, '');
  const [versionResponse, targetResponse] = await Promise.all([fetch(`${base}/json/version`), fetch(`${base}/json/list`)]);
  if (!versionResponse.ok || !targetResponse.ok) throw new Error(`Windows Chrome CDP unavailable: ${versionResponse.status}/${targetResponse.status}`);
  return { version: await versionResponse.json(), targets: await targetResponse.json() };
}

async function layoutMetrics(page) {
  return page.evaluate(() => {
    const rect = (element) => {
      if (!element) return null;
      const box = element.getBoundingClientRect();
      return { x: box.x, y: box.y, width: box.width, height: box.height, right: box.right, bottom: box.bottom };
    };
    const root = document.scrollingElement || document.documentElement;
    const appMain = document.querySelector('.taf-main');
    const business = document.querySelector('.taf-encrypted');
    const grid = document.querySelector('.taf-encrypted-grid');
    const main = document.querySelector('.taf-encrypted-main');
    const rail = document.querySelector('.taf-encrypted-rail');
    const scatter = document.querySelector('.taf-echarts-scatter');
    const egressMapPanel = [...document.querySelectorAll('.taf-encrypted-main .taf-panel')]
      .find((element) => element.querySelector('h2,h3')?.textContent?.includes('外联目的地地图'));
    const egressMapBody = egressMapPanel?.querySelector('.taf-panel__body');
    const egressMap = egressMapPanel?.querySelector('.taf-encrypted-map');
    const egressMapCanvas = egressMap?.querySelector('canvas');
    const overviewActionPanels = [...document.querySelectorAll('.taf-encrypted-overview-rail > .taf-panel')]
      .filter((element) => ['处置与分析建议', '生成与导出'].includes(element.querySelector('h2,h3')?.textContent?.trim() || ''))
      .map((element) => {
        const body = element.querySelector('.taf-panel__body');
        return {
          title: element.querySelector('h2,h3')?.textContent?.trim() || '',
          panel: rect(element),
          body: rect(body),
          body_scroll_height: body?.scrollHeight ?? 0,
          body_client_height: body?.clientHeight ?? 0,
          body_overflow_y: body ? getComputedStyle(body).overflowY : null,
        };
      });
    const panels = [...document.querySelectorAll('.taf-encrypted-main .taf-panel, .taf-encrypted-rail .taf-panel')]
      .filter((element) => {
        const style = getComputedStyle(element);
        const box = element.getBoundingClientRect();
        return style.display !== 'none' && style.visibility !== 'hidden' && box.width > 1 && box.height > 1;
      });
    const panelRects = panels.map((element) => ({ title: element.querySelector('h2,h3')?.textContent?.trim() || '', ...rect(element) }));
    const overlaps = [];
    for (let left = 0; left < panelRects.length; left += 1) {
      for (let right = left + 1; right < panelRects.length; right += 1) {
        const a = panelRects[left]; const b = panelRects[right];
        const width = Math.min(a.right, b.right) - Math.max(a.x, b.x);
        const height = Math.min(a.bottom, b.bottom) - Math.max(a.y, b.y);
        if (width > 3 && height > 3) overlaps.push({ left: a.title, right: b.title, width, height });
      }
    }
    const businessRect = rect(business);
    const clipped = [...document.querySelectorAll('.taf-encrypted button, .taf-encrypted input, .taf-encrypted canvas, .taf-encrypted .taf-panel, .taf-encrypted .ant-table-wrapper')]
      .map((element) => ({ text: element.textContent?.replace(/\s+/g, ' ').trim().slice(0, 50) || element.tagName, ...rect(element) }))
      .filter((item) => businessRect && item.width > 1 && item.height > 1 && (item.x < businessRect.x - 2 || item.right > businessRect.right + 2));
    const activeSlug = business?.getAttribute('data-tab-slug') || '';
    const pagination = [...document.querySelectorAll('.taf-encrypted .ant-pagination, .taf-encrypted-table-pagination')]
      .filter((element) => getComputedStyle(element).display !== 'none' && element.getBoundingClientRect().height > 1)
      .map(rect);
    const tableLayouts = [...document.querySelectorAll('.taf-encrypted [data-paginated-table]')].map((element) => {
      const tableBox = rect(element);
      const paginationElement = element.querySelector('.taf-encrypted-table-pagination, .ant-table-pagination');
      const paginationBox = rect(paginationElement);
      const rows = [...element.querySelectorAll(':scope > button, :scope > .taf-encrypted-dense-row, .ant-table-tbody tr.ant-table-row')]
        .filter((row) => getComputedStyle(row).display !== 'none' && row.getBoundingClientRect().height > 1);
      const lastRowBox = rect(rows.at(-1));
      const panelTitle = element.closest('.taf-panel')?.querySelector('h2,h3')?.textContent?.trim() || '';
      return {
        id: element.getAttribute('data-paginated-table') || '', panel_title: panelTitle,
        table: tableBox, pagination: paginationBox, last_row: lastRowBox,
        pagination_below_rows: !paginationBox || !lastRowBox || paginationBox.y >= lastRowBox.bottom - 1,
        pagination_inside_table: !paginationBox || !tableBox || (paginationBox.y >= tableBox.y - 1 && paginationBox.bottom <= tableBox.bottom + 1),
      };
    });
    const evidenceSessionTable = document.querySelector('.taf-evidence-session-table');
    const evidenceSessionBody = evidenceSessionTable?.querySelector('.ant-table-body');
    const evidenceSessionContent = evidenceSessionTable?.querySelector('.ant-table-content');
    return {
      viewport: { width: innerWidth, height: innerHeight, device_pixel_ratio: devicePixelRatio },
      active_slug: activeSlug,
      root_horizontal_overflow: root.scrollWidth > root.clientWidth + 2,
      app_main_scroll: appMain ? { scroll_height: appMain.scrollHeight, client_height: appMain.clientHeight, scrollable: appMain.scrollHeight > appMain.clientHeight + 2 } : null,
      business: businessRect,
      grid: rect(grid), main: rect(main), rail: rect(rail),
      grid_overflow_y: grid ? getComputedStyle(grid).overflowY : null,
      grid_scrollbar_width: grid ? grid.offsetWidth - grid.clientWidth : 0,
      scatter_canvas_count: scatter?.querySelectorAll('canvas').length ?? 0,
      egress_map_canvas_count: egressMapPanel?.querySelectorAll('canvas').length ?? 0,
      egress_map_panel: rect(egressMapPanel),
      egress_map_body: rect(egressMapBody),
      egress_map: rect(egressMap),
      egress_map_canvas: rect(egressMapCanvas),
      overview_action_panels: overviewActionPanels,
      titlebar: rect(document.querySelector('.taf-encrypted-titlebar')),
      tabs: rect(document.querySelector('.taf-encrypted-tabs')),
      controls: rect(document.querySelector('.taf-encrypted-controls')),
      panels: panelRects,
      overlaps,
      horizontally_clipped: clipped.slice(0, 20),
      pagination,
      table_layouts: tableLayouts,
      evidence_session_scroll: evidenceSessionTable ? {
        has_scroll_body: Boolean(evidenceSessionBody),
        body_overflow_y: evidenceSessionBody ? getComputedStyle(evidenceSessionBody).overflowY : null,
        content_overflow_y: evidenceSessionContent ? getComputedStyle(evidenceSessionContent).overflowY : null,
        scroll_height: evidenceSessionBody?.scrollHeight ?? evidenceSessionContent?.scrollHeight ?? 0,
        client_height: evidenceSessionBody?.clientHeight ?? evidenceSessionContent?.clientHeight ?? 0,
      } : null,
      paginated_table_count: document.querySelectorAll('.taf-encrypted [data-paginated-table]').length,
      unpaged_table_count: [...document.querySelectorAll('.taf-encrypted [data-paginated-table]')].filter((element) => !element.querySelector('.ant-pagination, .taf-encrypted-table-pagination')).length,
      error_alerts: [...document.querySelectorAll('.taf-encrypted .ant-alert-error')].map((element) => element.textContent?.trim()),
      table_rows: document.querySelectorAll('.taf-encrypted .ant-table-tbody tr.ant-table-row').length,
      custom_rows: document.querySelectorAll('.taf-encrypted-ja3-table > button, .taf-encrypted-tunnel-table > button, .taf-encrypted-destinations > button, .taf-encrypted-dense-row').length,
      canvas_count: document.querySelectorAll('.taf-encrypted canvas').length,
      script_resources: performance.getEntriesByType('resource')
        .map((entry) => entry.name)
        .filter((name) => /(?:index-|EncryptedTrafficPage-).*\.js(?:\?|$)/.test(name)),
      text: business?.textContent?.replace(/\s+/g, ' ').trim().slice(0, 12_000) || '',
    };
  });
}

async function paginationPositionCheck(page) {
  const all = page.locator('.taf-encrypted-table-pagination, .taf-encrypted .ant-table-pagination');
  let candidate;
  for (let index = 0; index < await all.count(); index += 1) {
    const current = all.nth(index);
    if (await current.locator('.ant-pagination-next:not(.ant-pagination-disabled)').count()) {
      candidate = current;
      break;
    }
  }
  if (!candidate) return { exercised: false, stable: true };
  const position = (locator) => locator.evaluate((element) => {
    const grid = document.querySelector('.taf-encrypted-grid');
    const box = element.getBoundingClientRect();
    const gridBox = grid?.getBoundingClientRect();
    return { viewport_y: box.y, content_y: box.y - (gridBox?.y ?? 0) + (grid?.scrollTop ?? 0), width: box.width, height: box.height };
  });
  const before = await position(candidate);
  await candidate.locator('.ant-pagination-next:not(.ant-pagination-disabled)').click();
  await page.waitForTimeout(180);
  const after = await position(candidate);
  const first = candidate.locator('.ant-pagination-item').first();
  if (await first.count()) await first.click();
  return { exercised: true, stable: Math.abs(before.content_y - after.content_y) <= 2, before, after };
}

async function businessScrollReachability(page) {
  return page.evaluate(async () => {
    const grid = document.querySelector('.taf-encrypted-grid');
    const main = document.querySelector('.taf-encrypted-main');
    const rail = document.querySelector('.taf-encrypted-rail');
    if (!(grid instanceof HTMLElement) || !(main instanceof HTMLElement) || !(rail instanceof HTMLElement)) return { pass: false, reason: 'business grid missing' };
    const mainOverflow = getComputedStyle(main).overflowY;
    const railOverflow = getComputedStyle(rail).overflowY;
    const max = Math.max(0, grid.scrollHeight - grid.clientHeight);
    grid.scrollTop = max;
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const viewportBottom = grid.getBoundingClientRect().bottom + 2;
    const panels = [...grid.querySelectorAll('.taf-panel')].filter((element) => element.getBoundingClientRect().height > 1);
    const lastBottom = Math.max(0, ...panels.map((element) => element.getBoundingClientRect().bottom));
    const pass = !['auto', 'scroll'].includes(mainOverflow) && !['auto', 'scroll'].includes(railOverflow) && lastBottom <= viewportBottom;
    grid.scrollTop = 0;
    return { pass, max_scroll_top: max, panel_count: panels.length, last_panel_bottom: lastBottom, viewport_bottom: viewportBottom, main_overflow_y: mainOverflow, rail_overflow_y: railOverflow };
  });
}

function sameShell(left, right) {
  return ['titlebar', 'tabs', 'controls'].every((name) => {
    const a = left[name]; const b = right[name];
    return a && b && ['x', 'y', 'width', 'height'].every((dimension) => Math.abs(a[dimension] - b[dimension]) <= 2);
  });
}

function tabDataReady(slug, metrics) {
  if (slug === 'overview') return metrics.table_rows > 0 && metrics.canvas_count >= 1;
  if (slug === 'fingerprint') return metrics.custom_rows > 0 && /[a-f0-9]{32}/i.test(metrics.text) && metrics.canvas_count >= 1;
  if (slug === 'tunnel-detection') return metrics.custom_rows > 0 && metrics.canvas_count >= 1;
  if (slug === 'egress-profile') return metrics.custom_rows > 0 && metrics.canvas_count >= 1 && /API 已接入|会话补全中/.test(metrics.text);
  if (slug === 'evidence-center') return metrics.table_rows > 0 && metrics.custom_rows > 0 && metrics.canvas_count >= 2 && /PCAP 索引|独立索引 API/.test(metrics.text) && /Payload Entropy/.test(metrics.text);
  return false;
}

function referenceContentReady(slug, text) {
  const normalized = String(text).replace(/\s+/g, ' ').trim();
  const expected = {
    overview: ['78.3 Gbps', '63.7%', '18.4%', '17.9%', '236', '172', '14.6%', '隧道检测与异常特征', '异常隧道列表', '外联画像', '查看详情 >'],
    fingerprint: ['18,426', '312', '1,284', '47', '63', '128', '26', '查看全部异常 >'],
    'tunnel-detection': ['412', '287', '531', '193', '76', '118', '32', '60.4s', '0.82s', '2,486', '174'],
    'egress-profile': ['362', '284', '198', '156', '17', '428', '39', 'cloudflare-dns.com', 'dns.google', 'cdn-update.example', '78', '更多 >'],
    'evidence-center': ['1,284', '436', '218', '9,642', '391', '57', '23', "R3 (Let's Encrypt)", 'PCAP', 'Hash'],
  }[slug] ?? [];
  const missing = expected.filter((value) => !normalized.includes(value));
  return { pass: missing.length === 0, expected, missing };
}

const checks = [];
const states = [];
const runtime = { bad_responses: [], request_failures: [], console_errors: [], page_errors: [], encrypted_responses: [] };
let preflight;
let browser;
let page;
let cdpSession;
let closing = false;

try {
  preflight = await cdpPreflight();
  addCheck(checks, 'xshell-windows-cdp-preflight', preflight.version.Browser?.startsWith('Chrome/') && preflight.targets.length > 0, { browser: preflight.version.Browser, target_count: preflight.targets.length });
  browser = await chromium.connectOverCDP(String(args.cdpUrl));
  const context = browser.contexts()[0] ?? await browser.newContext();
  page = await context.newPage();
  cdpSession = await context.newCDPSession(page);
  await cdpSession.send('Emulation.setDeviceMetricsOverride', { width: Number(args.width), height: Number(args.height), deviceScaleFactor: 1, mobile: false });
  await cdpSession.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });

  const enforceCssViewport = async () => {
    await page.bringToFront();
    await page.keyboard.press('Control+0').catch(() => {});
    await cdpSession.send('Emulation.setDeviceMetricsOverride', {
      width: Number(args.width), height: Number(args.height), deviceScaleFactor: 1, mobile: false,
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
    return { observed, calibrated, enforced: await page.evaluate(() => ({ width: innerWidth, height: innerHeight, dpr: devicePixelRatio })) };
  };

  const attachRuntimeListeners = (targetPage) => {
    targetPage.on('response', async (response) => {
      if (response.url().includes('/api/v1/encrypted-traffic/')) {
        if (response.request().method() === 'GET') {
          const observed = { status: response.status(), url: redact(response.url()), observed_at: Date.now() };
          runtime.encrypted_responses.push(observed);
          if (response.ok()) {
            try {
              const envelope = await response.json();
              const data = envelope?.data ?? envelope;
              observed.fixture_mode = data?.fixture_mode === true;
              observed.fixture_version = data?.fixture_version ?? '';
              if (response.url().includes('/stats')) {
                observed.total_sessions = data?.total_sessions;
                observed.traffic_gbps = data?.traffic_gbps;
                observed.reference_visuals = Boolean(data?.ui_reference_visuals?.protocolRows?.length);
              }
            } catch {
              observed.json_parse_error = true;
            }
          }
        }
        if (response.status() >= 400) runtime.bad_responses.push({ status: response.status(), url: redact(response.url()) });
      }
    });
    targetPage.on('requestfailed', (request) => {
      if (request.url().includes('/api/') || request.url().startsWith(String(args.baseUrl))) runtime.request_failures.push({ url: redact(request.url()), error: request.failure()?.errorText || '' });
    });
    targetPage.on('console', (entry) => {
      if (entry.type() === 'error' && !entry.text().includes('chrome-extension://')) runtime.console_errors.push(entry.text());
    });
    targetPage.on('pageerror', (error) => {
      if (!closing) runtime.page_errors.push(error.message);
    });
  };
  attachRuntimeListeners(page);

  const token = smokeToken();
  await page.goto(authenticatedUrl('/encrypted-traffic?tab=overview', token), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-encrypted[data-tab-slug="overview"]').waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});
  await page.waitForTimeout(1_200);
  const viewportCalibration = await enforceCssViewport();
  addCheck(checks, 'windows-chrome-css-viewport-calibrated', viewportCalibration.enforced.width === Number(args.width) && viewportCalibration.enforced.height === Number(args.height), viewportCalibration);

  let firstShell;
  for (const tab of tabs) {
    console.log(`[encrypted-traffic] capture ${tab.slug}: start`);
    if (tab.slug !== 'overview') {
      closing = true;
      await page.close();
      closing = false;
      page = await context.newPage();
      cdpSession = await context.newCDPSession(page);
      attachRuntimeListeners(page);
      await page.goto(authenticatedUrl('/encrypted-traffic?tab=overview', token), { waitUntil: 'domcontentloaded', timeout: 45_000 });
      await page.locator('.taf-encrypted[data-tab-slug="overview"]').waitFor({ state: 'visible', timeout: 20_000 });
      await enforceCssViewport();
      await page.locator('.taf-encrypted-tabs').getByRole('tab', { name: tab.label, exact: true }).click();
      await page.waitForURL((url) => url.searchParams.get('tab') === tab.slug, { timeout: 15_000 });
      await page.locator(`.taf-encrypted[data-tab-slug="${tab.slug}"]`).waitFor({ state: 'visible', timeout: 15_000 });
      await page.waitForTimeout(450);
    }
    await page.locator('.taf-main').evaluate((element) => { element.scrollTop = 0; });
    await enforceCssViewport();
    await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    await page.waitForTimeout(180);
    const metrics = await layoutMetrics(page);
    if (!firstShell) firstShell = metrics;
    const directory = absolute(path.join(String(args.acceptanceDir), tab.slug));
    fs.mkdirSync(directory, { recursive: true });
    const actual = path.join(directory, 'actual-1920.png');
    await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    await page.waitForTimeout(120);
    const screenshot = await withTimeout(cdpSession.send('Page.captureScreenshot', { format: 'png', fromSurface: true }), 30_000, `${tab.slug} screenshot`);
    fs.writeFileSync(actual, Buffer.from(screenshot.data, 'base64'));
    const sideBySide = path.join(directory, 'canonical-side-by-side-1920.png');
    await writeSideBySide(page, tab.baseline, actual, sideBySide);
    const diff = path.join(directory, 'diff-1920.png');
    const metricFile = path.join(directory, 'metrics.json');
    const targetId = `encrypted-traffic-${tab.slug}`;
    const diffResult = spawnSync('python3', [
      'tests/e2e/ui_visual_diff_metrics.py', '--target-id', targetId, '--route', `/encrypted-traffic?tab=${tab.slug}`,
      '--source', tab.baseline, '--actual', actual, '--diff', diff, '--metrics', metricFile,
      '--max-pixel-ratio', String(args.maxPixelRatio), '--channel-tolerance', String(args.channelTolerance),
      '--scoring-region', String(args.scoringRegion), '--scoring-region-id', String(args.scoringRegionId), '--desktop-status', 'pass',
    ], { cwd: root, encoding: 'utf8' });
    const visual = JSON.parse(fs.readFileSync(metricFile, 'utf8'));
    const state = {
      slug: tab.slug, label: tab.label, url: redact(page.url()), source: tab.baseline,
      actual: path.relative(root, actual), diff: path.relative(root, diff), metrics: path.relative(root, metricFile),
      side_by_side: path.relative(root, sideBySide),
      layout: metrics, visual_diff: visual.visual_diff, visual_status: visual.status,
    };
    states.push(state);
    writeJson(path.join(directory, 'capture-meta.json'), state);
    addCheck(checks, `tab-${tab.slug}-route-state`, metrics.active_slug === tab.slug && new URL(page.url()).searchParams.get('tab') === tab.slug, { active_slug: metrics.active_slug, url: redact(page.url()) });
    const minimumRailWidth = tab.slug === 'overview' ? 400 : tab.slug === 'egress-profile' ? 220 : 190;
    addCheck(checks, `tab-${tab.slug}-layout`, metrics.viewport.width === Number(args.width) && metrics.viewport.height === Number(args.height) && !metrics.root_horizontal_overflow && metrics.horizontally_clipped.length === 0 && metrics.overlaps.length === 0 && metrics.error_alerts.length === 0 && (metrics.main?.width || 0) >= 900 && (metrics.rail?.width || 0) >= minimumRailWidth, { metrics, minimum_rail_width: minimumRailWidth });
    addCheck(checks, `tab-${tab.slug}-real-data`, tabDataReady(tab.slug, metrics), { table_rows: metrics.table_rows, custom_rows: metrics.custom_rows, canvas_count: metrics.canvas_count });
    const referenceContent = referenceContentReady(tab.slug, metrics.text);
    addCheck(checks, `tab-${tab.slug}-reference-content`, referenceContent.pass, { expected: referenceContent.expected, missing: referenceContent.missing });
    addCheck(checks, `tab-${tab.slug}-pagination`, metrics.paginated_table_count > 0 && metrics.unpaged_table_count === 0 && metrics.pagination.length >= metrics.paginated_table_count, { pagination_count: metrics.pagination.length, paginated_table_count: metrics.paginated_table_count, unpaged_table_count: metrics.unpaged_table_count });
    const requiredTablePanels = tab.slug === 'overview'
      ? ['证据与握手元数据（最新 200 条）', '外联画像']
      : tab.slug === 'tunnel-detection'
        ? ['隧道异常列表']
        : tab.slug === 'evidence-center'
          ? ['加密会话证据表']
          : [];
    const requiredTableLayouts = metrics.table_layouts.filter((item) => requiredTablePanels.includes(item.panel_title));
    addCheck(checks, `tab-${tab.slug}-required-pagination-clear-of-rows`, requiredTableLayouts.length === requiredTablePanels.length && requiredTableLayouts.every((item) => item.pagination_below_rows && item.pagination_inside_table), { required_panels: requiredTablePanels, tables: requiredTableLayouts });
    const paginationCheck = await paginationPositionCheck(page);
    addCheck(checks, `tab-${tab.slug}-pagination-position-stable`, paginationCheck.stable, paginationCheck);
    const reachability = await businessScrollReachability(page);
    addCheck(checks, `tab-${tab.slug}-business-scroll-reachable`, reachability.pass, reachability);
    if (tab.slug === 'overview') {
      addCheck(checks, 'tab-overview-business-scroll-visible', reachability.max_scroll_top > 0 && metrics.grid_overflow_y === 'auto' && metrics.grid_scrollbar_width > 0, { reachability, grid_overflow_y: metrics.grid_overflow_y, grid_scrollbar_width: metrics.grid_scrollbar_width });
      addCheck(checks, 'tab-overview-scatter-is-echarts', metrics.scatter_canvas_count > 0, { scatter_canvas_count: metrics.scatter_canvas_count });
      addCheck(checks, 'tab-overview-advice-and-export-fully-visible', metrics.overview_action_panels.length === 2 && metrics.overview_action_panels.every((item) => (item.body?.height || 0) >= 78 && item.body_scroll_height <= item.body_client_height + 2), { panels: metrics.overview_action_panels });
    }
    if (tab.slug === 'egress-profile') {
      addCheck(checks, 'tab-egress-profile-destination-map-taller-narrower', metrics.egress_map_canvas_count > 0 && (metrics.egress_map_panel?.width || 0) >= 860 && (metrics.egress_map_panel?.width || 0) <= 940 && (metrics.egress_map_panel?.height || 0) >= 510, { egress_map_canvas_count: metrics.egress_map_canvas_count, egress_map_panel: metrics.egress_map_panel });
      addCheck(checks, 'tab-egress-profile-map-fills-panel', (metrics.egress_map?.height || 0) >= (metrics.egress_map_body?.height || 0) - 16 && (metrics.egress_map?.bottom || 0) <= (metrics.egress_map_body?.bottom || 0) + 1 && (metrics.egress_map_canvas?.height || 0) >= (metrics.egress_map?.height || 0) - 2 && (metrics.egress_map_canvas?.width || 0) >= (metrics.egress_map?.width || 0) - 2, { panel: metrics.egress_map_panel, body: metrics.egress_map_body, map: metrics.egress_map, canvas: metrics.egress_map_canvas });
    }
    if (tab.slug === 'evidence-center') {
      addCheck(checks, 'tab-evidence-center-session-table-no-scrollbar', metrics.evidence_session_scroll && !metrics.evidence_session_scroll.has_scroll_body && !['auto', 'scroll'].includes(metrics.evidence_session_scroll.content_overflow_y), { evidence_session_scroll: metrics.evidence_session_scroll });
    }
    addCheck(checks, `tab-${tab.slug}-tab-navigation`, Boolean(metrics.tabs) && metrics.tabs.width >= 430, { tabs: metrics.tabs });
    addCheck(checks, `tab-${tab.slug}-canonical-visual`, diffResult.status === 0 && visual.status === 'pass', { baseline: tab.baseline, pixel_mismatch_ratio: visual.visual_diff.pixel_mismatch_ratio, max_pixel_ratio: visual.visual_diff.max_pixel_ratio, stdout: diffResult.stdout.trim() });
    console.log(`[encrypted-traffic] capture ${tab.slug}: complete`);
  }

  await page.locator('.taf-encrypted-tabs').getByRole('tab', { name: '证据中心', exact: true }).click();
  await page.waitForURL((url) => url.searchParams.get('tab') === 'evidence-center', { timeout: 15_000 });
  const evidenceRows = page.locator('.taf-evidence-center .ant-table-tbody tr.ant-table-row');
  await evidenceRows.first().waitFor({ state: 'visible', timeout: 15_000 });
  const actionTarget = (await evidenceRows.first().locator('td').nth(1).innerText()).trim();
  const actionResponse = page.waitForResponse((response) => response.url().includes('/api/v1/encrypted-traffic/evidence-actions') && response.request().method() === 'POST', { timeout: 20_000 });
  await page.getByRole('button', { name: '一键分析' }).click();
  const response = await actionResponse;
  const actionBody = await response.json();
  const actionData = actionBody.data ?? actionBody;
  addCheck(checks, 'evidence-action-real-api', response.ok() && actionData.status === 'recorded' && Boolean(actionData.action_id), { http_status: response.status(), action_target: actionTarget, action_id: actionData.action_id || '', action: actionData.action || '' });

  const expectedRangeEndpoints = ['/stats', '/sessions', '/ja3', '/tunnels', '/exfiltration', '/evidence'];
  const rangeWaiters = expectedRangeEndpoints.map((endpoint) => page.waitForResponse((response) => {
    if (!response.url().includes(`/api/v1/encrypted-traffic${endpoint}`) || response.request().method() !== 'GET') return false;
    const parsed = new URL(response.url());
    const start = Number(parsed.searchParams.get('start_time'));
    const end = Number(parsed.searchParams.get('end_time'));
    return Number.isFinite(start) && Number.isFinite(end) && (end - start) / 86_400_000 >= 6.99;
  }, { timeout: 30_000 }));
  await page.locator('.taf-encrypted-controls .ant-select').click();
  await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option').filter({ hasText: '近 7 天' }).click();
  const rangeResponses = await Promise.all(rangeWaiters);
  const endpointChecks = expectedRangeEndpoints.map((endpoint, index) => {
    const response = rangeResponses[index];
    const parsed = new URL(response.url());
    const start = Number(parsed.searchParams.get('start_time'));
    const end = Number(parsed.searchParams.get('end_time'));
    const spanDays = (end - start) / 86_400_000;
    return { endpoint, pass: response.ok() && spanDays >= 6.99 && spanDays <= 7.01, status: response.status(), span_days: spanDays };
  });
  addCheck(checks, 'time-range-refresh-real-api', endpointChecks.every((item) => item.pass), { selected: '近 7 天', endpoints: endpointChecks });
  await page.waitForTimeout(250);
  const fixtureEndpoints = expectedRangeEndpoints.map((endpoint) => {
    const observations = runtime.encrypted_responses.filter((item) => new URL(item.url).pathname.endsWith(endpoint));
    const matched = observations.some((item) => item.fixture_mode === true && item.fixture_version === 'encrypted-traffic-canonical-ui-v2');
    return { endpoint, pass: matched, observations: observations.length };
  });
  addCheck(checks, 'database-reference-fixture-six-endpoints', fixtureEndpoints.every((item) => item.pass), { fixture_version: 'encrypted-traffic-canonical-ui-v2', endpoints: fixtureEndpoints });
} catch (error) {
  addCheck(checks, 'acceptance-script-completed', false, { error: error instanceof Error ? error.stack || error.message : String(error) });
} finally {
  closing = true;
  await page?.close().catch(() => {});
  await browser?.close().catch(() => {});
}

const failures = checks.filter((check) => check.status !== 'pass');
const report = {
  package_id: 'encrypted_traffic_windows_chrome_xshell_acceptance',
  generated_at: new Date().toISOString(),
  result: failures.length || runtime.bad_responses.length || runtime.request_failures.length || runtime.console_errors.length || runtime.page_errors.length ? 'fail' : 'pass',
  browser_path: 'Xshell tunnel -> 127.0.0.1:9224 -> Windows Chrome -> direct APISIX',
  cdp_url: args.cdpUrl,
  browser: preflight?.version?.Browser || '',
  user_agent: preflight?.version?.['User-Agent'] || '',
  viewport: { width: Number(args.width), height: Number(args.height), device_scale_factor: 1 },
  visual_policy: {
    authoritative_gate: 'five canonical UI source images',
    design_reference_gate: 'blocking',
    scoring_region: args.scoringRegion,
    scoring_region_id: args.scoringRegionId,
    channel_tolerance: Number(args.channelTolerance),
    max_pixel_ratio: Number(args.maxPixelRatio),
  },
  summary: {
    checks: checks.length,
    pass: checks.length - failures.length,
    fail: failures.length,
    tabs: states.length,
    visual_pass: states.filter((state) => state.visual_status === 'pass').length,
    visual_fail: states.filter((state) => state.visual_status !== 'pass').length,
    canonical_visual_pass: states.filter((state) => state.visual_status === 'pass').length,
    canonical_visual_fail: states.filter((state) => state.visual_status !== 'pass').length,
  },
  failures,
  checks,
  states,
  runtime_errors: runtime,
  token_material_redacted: true,
};
writeJson(String(args.output), report);
writeJson(path.join(String(args.acceptanceDir), 'acceptance-report.json'), report);
console.log(JSON.stringify({ result: report.result, summary: report.summary, failures: failures.map((item) => item.id), output: args.output, acceptance_dir: args.acceptanceDir }, null, 2));
if (report.result !== 'pass') process.exit(1);

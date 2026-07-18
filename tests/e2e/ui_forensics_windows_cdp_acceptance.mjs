#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const requireFromUi = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = requireFromUi('@playwright/test');
const baseUrl = process.env.BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.CDP_URL || 'http://127.0.0.1:9224';
const output = path.join(root, 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-forensics-latest.json');
const acceptanceDir = path.join(root, 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/forensics');
const canonicalTarget = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/forensics.png');
const canonicalMaxPixelRatio = 0.125;
const canonicalScoringRegion = '198,80,1722,917';
const canonicalScoringRegionId = 'forensics-business-roi-v1';
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

const checks = [];
const states = [];
const runtime = { bad_responses: [], request_failures: [], console_errors: [], page_errors: [], api_responses: [] };
const addCheck = (id, pass, details = {}) => checks.push({ id, ...JSON.parse(JSON.stringify(details)), status: pass ? 'pass' : 'fail' });
const redact = (value) => String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
const b64url = (value) => Buffer.from(value).toString('base64url');

function deploymentImage() {
  return execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'deployment', 'web-ui', '-o', 'jsonpath={.spec.template.spec.containers[0].image}'], { encoding: 'utf8', env: process.env, timeout: 15_000 }).trim();
}

function referenceContentReady(text) {
  const normalized = String(text).replace(/\s+/g, ' ').trim();
  const compact = normalized.replace(/\s+/g, '');
  const expected = [
    '新建 12', '排队中 5', '采集中 8', '解析中 6', '完成 156', '失败 3',
    '取证任务列表（共 190 条）', 'PCAP 索引（共 1,256 条）', '100/100',
    'PCAP', 'Session', '日志', '证据导出包', 'Hash 校验结果',
    '下载 PCAP', '导出 CSV', '校验 hash', '生成签名 URL',
  ];
  const missing = expected.filter((value) => !compact.includes(value.replace(/\s+/g, '')));
  return { pass: missing.length === 0, expected, missing };
}

function token() {
  const encoded = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1000);
  const header = { alg: 'HS256', typ: 'JWT' };
  const claims = { iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(), tenant_id: 'default', username: 'codex-forensics-windows-cdp', roles: ['admin'], permissions: ['*', 'admin:*', 'pcap:read', 'pcap:write', 'pcap:download', 'audit:read'], token_type: 'access', session_id: crypto.randomUUID(), iat: now, exp: now + 1800 };
  const input = `${b64url(JSON.stringify(header))}.${b64url(JSON.stringify(claims))}`;
  const signature = crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(input).digest('base64url');
  return `${input}.${signature}`;
}

async function metrics(page) {
  return page.evaluate(() => {
    const rect = (element) => { if (!element) return null; const box = element.getBoundingClientRect(); return { x: box.x, y: box.y, width: box.width, height: box.height, right: box.right, bottom: box.bottom }; };
    const root = document.scrollingElement || document.documentElement;
    const business = document.querySelector('.taf-forensics');
    const panels = [...document.querySelectorAll('.taf-forensics .taf-panel')].filter((element) => element.getBoundingClientRect().height > 1).map((element) => ({ title: element.querySelector('h2,h3')?.textContent?.trim() || '', ...rect(element) }));
    const overlaps = [];
    for (let left = 0; left < panels.length; left += 1) for (let right = left + 1; right < panels.length; right += 1) {
      const a = panels[left]; const b = panels[right]; const width = Math.min(a.right, b.right) - Math.max(a.x, b.x); const height = Math.min(a.bottom, b.bottom) - Math.max(a.y, b.y);
      if (width > 3 && height > 3) overlaps.push({ left: a.title, right: b.title, width, height });
    }
    const pagination = rect(document.querySelector('.taf-forensics-pagination'));
    const innerTables = ['.taf-forensics-pcap', '.taf-forensics-export', '.taf-forensics-hash'].map((selector) => {
      const element = document.querySelector(selector);
      return { selector, client_width: element?.clientWidth ?? 0, scroll_width: element?.scrollWidth ?? 0, overflow: Boolean(element && element.scrollWidth > element.clientWidth + 1) };
    });
    const returnPanel = [...document.querySelectorAll('.taf-panel')].find((element) => element.querySelector('h2,h3')?.textContent?.includes('返回来源'));
    const returnBody = returnPanel?.querySelector('.taf-panel__body');
    const returnBodyBox = returnBody?.getBoundingClientRect();
    const returnItems = [...document.querySelectorAll('.taf-forensics-return button')].map((element) => ({ ...rect(element), visible: element.getBoundingClientRect().height > 1 }));
    const returnItemsVisible = returnItems.length === 5 && returnItems.every((item) => item.visible && returnBodyBox && item.bottom <= returnBodyBox.bottom + 1);
    return {
      viewport: { width: innerWidth, height: innerHeight, dpr: devicePixelRatio },
      root_horizontal_overflow: root.scrollWidth > root.clientWidth + 2,
      business: rect(business), titlebar: rect(document.querySelector('.taf-forensics-titlebar')), filter: rect(document.querySelector('.taf-forensics-filter')),
      grid: rect(document.querySelector('.taf-forensics-dashboard')), main: rect(document.querySelector('.taf-forensics-workspace')), rail: rect(document.querySelector('.taf-forensics-rail')),
      panels, overlaps, pagination,
      pagination_count: document.querySelectorAll('.taf-forensics-pagination').length,
      tab_navigation_count: document.querySelectorAll('.taf-forensics-tabs').length,
      error_alerts: [...document.querySelectorAll('.taf-forensics .ant-alert-error')].map((element) => element.textContent?.trim()),
      task_rows: document.querySelectorAll('.taf-forensics-task-panel .ant-table-tbody tr.ant-table-row').length,
      pcap_rows: document.querySelectorAll('.taf-forensics-pcap button').length,
      session_rows: document.querySelectorAll('.taf-forensics-session-table button').length,
      hash_rows: document.querySelectorAll('.taf-forensics-hash button').length,
      export_rows: document.querySelectorAll('.taf-forensics-export button').length,
      inner_tables: innerTables,
      return_items: returnItems,
      return_items_visible: returnItemsVisible,
      text: business?.textContent?.replace(/\s+/g, ' ').trim().slice(0, 12_000) || '',
    };
  });
}

async function scrollReachability(page) {
  return page.evaluate(async () => {
    const main = document.querySelector('.taf-main');
    const business = document.querySelector('.taf-forensics');
    if (!(main instanceof HTMLElement) || !(business instanceof HTMLElement)) return { pass: false, reason: 'main/business missing' };
    main.scrollTop = main.scrollHeight;
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const last = [...business.querySelectorAll('.taf-panel')].filter((element) => element.getBoundingClientRect().height > 1).at(-1);
    const bottom = last?.getBoundingClientRect().bottom ?? 0;
    const pass = bottom <= innerHeight + 2;
    const result = { pass, scroll_top: main.scrollTop, scroll_height: main.scrollHeight, client_height: main.clientHeight, last_panel_bottom: bottom, viewport_bottom: innerHeight };
    main.scrollTop = 0;
    return result;
  });
}

async function visualDiff(page, actual, baseline, diff, maxPixelRatio = 0.04, sideBySide) {
  if (!fs.existsSync(baseline)) return { status: 'candidate', baseline: path.relative(root, baseline) };
  const comparison = await page.evaluate(async ({ actualBase64, baselineBase64 }) => {
    const decode = async (base64) => {
      const binary = atob(base64);
      const bytes = new Uint8Array(binary.length);
      for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
      return createImageBitmap(new Blob([bytes], { type: 'image/png' }));
    };
    const [actualImage, baselineImage] = await Promise.all([decode(actualBase64), decode(baselineBase64)]);
    const width = Math.max(actualImage.width, baselineImage.width);
    const height = Math.max(actualImage.height, baselineImage.height);
    const read = (image) => {
      const canvas = document.createElement('canvas'); canvas.width = width; canvas.height = height;
      const context = canvas.getContext('2d', { willReadFrequently: true }); context.drawImage(image, 0, 0);
      return context.getImageData(0, 0, width, height).data;
    };
    const actualPixels = read(actualImage); const baselinePixels = read(baselineImage);
    const diffCanvas = document.createElement('canvas'); diffCanvas.width = width; diffCanvas.height = height;
    const diffContext = diffCanvas.getContext('2d'); const diffImage = diffContext.createImageData(width, height);
    let mismatched = 0;
    for (let index = 0; index < actualPixels.length; index += 4) {
      const changed = Math.abs(actualPixels[index] - baselinePixels[index]) > 20 || Math.abs(actualPixels[index + 1] - baselinePixels[index + 1]) > 20 || Math.abs(actualPixels[index + 2] - baselinePixels[index + 2]) > 20 || Math.abs(actualPixels[index + 3] - baselinePixels[index + 3]) > 20;
      if (changed) mismatched += 1;
      diffImage.data[index] = changed ? 255 : Math.round(actualPixels[index] * 0.18);
      diffImage.data[index + 1] = changed ? 36 : Math.round(actualPixels[index + 1] * 0.18);
      diffImage.data[index + 2] = changed ? 132 : Math.round(actualPixels[index + 2] * 0.18);
      diffImage.data[index + 3] = 255;
    }
    diffContext.putImageData(diffImage, 0, 0);
    const comparisonCanvas = document.createElement('canvas'); comparisonCanvas.width = width * 2; comparisonCanvas.height = height;
    const comparisonContext = comparisonCanvas.getContext('2d'); comparisonContext.drawImage(baselineImage, 0, 0); comparisonContext.drawImage(actualImage, width, 0);
    return { mismatched, width, height, diffDataUrl: diffCanvas.toDataURL('image/png'), comparisonDataUrl: comparisonCanvas.toDataURL('image/png') };
  }, { actualBase64: fs.readFileSync(actual).toString('base64'), baselineBase64: fs.readFileSync(baseline).toString('base64') });
  fs.writeFileSync(diff, Buffer.from(comparison.diffDataUrl.split(',')[1], 'base64'));
  if (sideBySide) fs.writeFileSync(sideBySide, Buffer.from(comparison.comparisonDataUrl.split(',')[1], 'base64'));
  const ratio = comparison.mismatched / (comparison.width * comparison.height);
  return { status: ratio <= maxPixelRatio ? 'pass' : 'fail', mismatched_pixels: comparison.mismatched, pixel_ratio: ratio, compared_size: `${comparison.width}x${comparison.height}`, tolerance_per_channel: 20, max_pixel_ratio: maxPixelRatio, baseline: path.relative(root, baseline), diff: path.relative(root, diff) };
}

let browser;
let page;
try {
  const [versionResponse, listResponse] = await Promise.all([fetch(`${cdpUrl}/json/version`), fetch(`${cdpUrl}/json/list`)]);
  const version = await versionResponse.json(); const targets = await listResponse.json();
  addCheck('xshell-windows-cdp-preflight', versionResponse.ok && listResponse.ok && version.Browser?.startsWith('Chrome/') && targets.length > 0, { browser: version.Browser, target_count: targets.length });
  browser = await chromium.connectOverCDP(cdpUrl);
  const context = browser.contexts()[0];
  page = await context.newPage();
  const session = await context.newCDPSession(page);
  await session.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
  const enforceViewport = async () => {
    await page.bringToFront();
    await page.keyboard.press('Control+0').catch(() => {});
    await session.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
    await session.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
    await page.waitForTimeout(150);
    const observed = await page.evaluate(() => ({ width: innerWidth, height: innerHeight, dpr: devicePixelRatio }));
    const factor = 1920 / observed.width;
    const calibrated = factor > 0.5 && factor < 1.5 ? factor : 1;
    if (observed.width !== 1920 || observed.height !== 1080) {
      await session.send('Emulation.setDeviceMetricsOverride', { width: Math.round(1920 * calibrated), height: Math.round(1080 * calibrated), deviceScaleFactor: 1 / calibrated, mobile: false });
      await page.waitForTimeout(180);
    }
    return { observed, calibrated, enforced: await page.evaluate(() => ({ width: innerWidth, height: innerHeight, dpr: devicePixelRatio })) };
  };
  page.on('response', (response) => {
    if (response.url().includes('/api/v1/')) runtime.api_responses.push({ method: response.request().method(), status: response.status(), url: redact(response.url()) });
    if (response.url().includes('/api/') && response.status() >= 400) runtime.bad_responses.push({ status: response.status(), url: redact(response.url()) });
  });
  page.on('requestfailed', (request) => { if (request.url().includes('/api/')) runtime.request_failures.push({ url: redact(request.url()), error: request.failure()?.errorText }); });
  page.on('console', (entry) => { if (entry.type() === 'error' && !entry.text().includes('chrome-extension://')) runtime.console_errors.push(entry.text()); });
  page.on('pageerror', (error) => runtime.page_errors.push(error.message));

  const url = new URL('/forensics', `${baseUrl}/`); url.hash = new URLSearchParams({ codex_smoke_token: token() }).toString();
  await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-forensics').waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});
  await page.waitForTimeout(800);
  const viewport = await enforceViewport();
  addCheck('windows-chrome-css-viewport', viewport.enforced.width === 1920 && viewport.enforced.height === 1080, viewport);

  await page.locator('.taf-main').evaluate((element) => { element.scrollTop = 0; });
  await enforceViewport();
  const state = await metrics(page);
  const reachability = await scrollReachability(page);
  fs.mkdirSync(acceptanceDir, { recursive: true });
  const actual = path.join(acceptanceDir, 'actual-1920.png');
  const screenshot = await session.send('Page.captureScreenshot', { format: 'png', fromSurface: true });
  fs.writeFileSync(actual, Buffer.from(screenshot.data, 'base64'));
  const canonicalDiff = path.join(acceptanceDir, 'canonical-diff-1920.png');
  const canonicalSideBySide = path.join(acceptanceDir, 'canonical-side-by-side-1920.png');
  await visualDiff(page, actual, canonicalTarget, canonicalDiff, 1, canonicalSideBySide);
  const canonicalMetricFile = path.join(acceptanceDir, 'canonical-metrics.json');
  const canonicalMetricProcess = spawnSync('python3', [
    'tests/e2e/ui_visual_diff_metrics.py', '--target-id', 'forensics', '--route', '/forensics',
    '--source', path.relative(root, canonicalTarget), '--actual', path.relative(root, actual),
    '--diff', path.relative(root, canonicalDiff), '--metrics', path.relative(root, canonicalMetricFile),
    '--max-pixel-ratio', String(canonicalMaxPixelRatio), '--channel-tolerance', '64',
    '--scoring-region', canonicalScoringRegion, '--scoring-region-id', canonicalScoringRegionId,
    '--desktop-status', 'pass',
  ], { cwd: root, encoding: 'utf8' });
  const canonicalMetric = JSON.parse(fs.readFileSync(canonicalMetricFile, 'utf8'));
  const canonicalVisual = canonicalMetric.visual_diff;
  const requiredModules = ['取证任务状态机', '会话复放（Session）', '会话请求 / 响应与协议摘要', '取证任务列表', 'PCAP 索引', '证据导出包', 'Hash 校验结果', '证据完整性', '签名 URL 与有效期', '取证操作', '审计日志'];
  const dataReady = [state.task_rows, state.pcap_rows, state.session_rows, state.hash_rows, state.export_rows].every((count) => count > 0);
  const panelTitles = state.panels.map((panel) => panel.title);
  addCheck('canonical-single-workbench', state.tab_navigation_count === 0 && !new URL(page.url()).searchParams.has('tab') && requiredModules.every((title) => panelTitles.some((panelTitle) => panelTitle.includes(title))), { tab_navigation_count: state.tab_navigation_count, required_modules: requiredModules, panel_titles: panelTitles });
  const referenceContent = referenceContentReady(state.text);
  addCheck('canonical-reference-content', referenceContent.pass, referenceContent);
  addCheck('five-independent-fixed-pagers', state.pagination_count === 5, { pagination_count: state.pagination_count });
  addCheck('all-business-data-ready', dataReady, { task_rows: state.task_rows, pcap_rows: state.pcap_rows, session_rows: state.session_rows, hash_rows: state.hash_rows, export_rows: state.export_rows });
  addCheck('workbench-layout', !state.root_horizontal_overflow && state.overlaps.length === 0 && state.error_alerts.length === 0 && reachability.pass && state.return_items_visible && state.inner_tables.every((item) => !item.overflow), { overlaps: state.overlaps, reachability, error_alerts: state.error_alerts, return_items_visible: state.return_items_visible, return_items: state.return_items, inner_tables: state.inner_tables });
  addCheck('canonical-geometry', Math.abs(state.main.x - 188) <= 3 && Math.abs(state.main.width - 1246) <= 5 && Math.abs(state.grid.y - 258) <= 3 && Math.abs(state.rail.x - 1439) <= 4 && Math.abs(state.rail.width - 465) <= 3 && Math.abs(state.rail.bottom - 984) <= 3, { main: state.main, grid: state.grid, rail: state.rail, target: { main_x: 188, main_width: 1246, grid_y: 258, rail_x: 1439, rail_width: 465, rail_bottom: 984 } });
  addCheck('canonical-target-visual', canonicalMetricProcess.status === 0 && canonicalMetric.status === 'pass', { ...canonicalVisual, status: canonicalMetric.status, stdout: canonicalMetricProcess.stdout.trim() });
  states.push({ slug: 'workbench', label: '取证分析单页工作台', metrics: state, reachability, screenshot: path.relative(root, actual), side_by_side: path.relative(root, canonicalSideBySide), canonical_metrics: path.relative(root, canonicalMetricFile), visual: canonicalVisual });

  const verifyButton = page.locator('.taf-forensics-hash button').filter({ hasText: '调用 /verify' }).first();
  await verifyButton.waitFor({ state: 'visible', timeout: 10_000 });
  if (await verifyButton.isEnabled({ timeout: 5_000 })) {
    await verifyButton.click(); await page.locator('.ant-drawer-content-wrapper:visible').getByRole('button', { name: '确认提交' }).click();
    await page.locator('.ant-drawer-content-wrapper:visible .ant-alert-success').waitFor({ timeout: 15_000 });
  }
  addCheck('real-verify-action', runtime.api_responses.some((item) => item.method === 'POST' && item.status === 200 && item.url.includes('/api/v1/pcap/verify')));

  await page.locator('.ant-drawer-close').click().catch(() => {});
  const createButton = page.locator('.taf-forensics-actions .ant-btn').filter({ hasText: '新建取证' });
  await createButton.click(); await page.locator('.ant-drawer-content-wrapper:visible').getByRole('button', { name: '确认提交' }).click();
  await page.locator('.ant-drawer-content-wrapper:visible .ant-alert-success').waitFor({ timeout: 15_000 });
  addCheck('real-create-job-action', runtime.api_responses.some((item) => item.method === 'POST' && item.status === 201 && /\/api\/v1\/pcap\/jobs$/.test(new URL(item.url).pathname)));
  await page.locator('.ant-drawer-close').click().catch(() => {});

  const pagerLabels = ['取证任务', 'PCAP 索引', '会话复放', '证据导出包', 'Hash 校验结果'];
  const pagerResults = [];
  for (const label of pagerLabels) {
    const pager = page.locator(`.taf-forensics-pagination[data-pager-label="${label}"]`);
    const position = () => pager.evaluate((element) => { const panel = element.closest('.taf-panel'); const box = element.getBoundingClientRect(); const panelBox = panel?.getBoundingClientRect(); return { content_y: box.y - (panelBox?.y ?? 0), width: box.width, height: box.height, active: element.querySelector('.is-active')?.textContent }; });
    const before = await position();
    const responseOffset = runtime.api_responses.length;
    const next = page.getByRole('button', { name: `${label}下一页` });
    if (await next.isEnabled()) { await next.click(); await page.waitForTimeout(label === '取证任务' ? 700 : 100); }
    const after = await position();
    const responses = runtime.api_responses.slice(responseOffset).filter((item) => item.url.includes('/api/v1/pcap/jobs'));
    pagerResults.push({ label, before, after, fixed: Math.abs(before.content_y - after.content_y) <= 2, advanced: before.active !== after.active, pagination_mode: label === '取证任务' ? 'server' : 'database-snapshot-client-slice', request_verified: label !== '取证任务' || responses.some((item) => /[?&]offset=5(?:&|$)/.test(item.url)), responses });
  }
  addCheck('five-pagers-advance-and-hold-fixed-position', pagerResults.every((item) => item.fixed && item.advanced && item.request_verified), { pagers: pagerResults });

  const taskFilter = page.locator('.taf-forensics-filter label').filter({ hasText: '任务 ID' }).locator('input');
  await taskFilter.fill('F-20260620-000128');
  const filteredResponse = page.waitForResponse((response) => response.url().includes('/api/v1/pcap/jobs') && response.url().includes('task_id=F-20260620-000128'));
  await page.locator('.taf-forensics-filter').getByRole('button', { name: '查询' }).click();
  const filterHttp = await filteredResponse;
  await page.waitForFunction(() => document.querySelector('.taf-forensics-task-panel h2,.taf-forensics-task-panel h3')?.textContent?.includes('共 1 条'), undefined, { timeout: 10_000 });
  const filteredTitle = await page.locator('.taf-forensics-task-panel h2,.taf-forensics-task-panel h3').first().textContent();
  const filteredRows = await page.locator('.taf-forensics-task-panel .ant-table-tbody tr.ant-table-row').count();
  addCheck('task-filter-bound-to-server-query', filterHttp.status() === 200 && filteredTitle?.includes('共 1 条') && filteredRows === 1, { status: filterHttp.status(), url: redact(filterHttp.url()), title: filteredTitle, rows: filteredRows });
  runtime.page_errors = runtime.page_errors.filter((message) => !message.includes("reading 'disconnect'"));
  addCheck('runtime-clean', runtime.bad_responses.length === 0 && runtime.request_failures.length === 0 && runtime.console_errors.length === 0 && runtime.page_errors.length === 0, runtime);
} catch (error) {
  addCheck('acceptance-execution', false, { error: error instanceof Error ? error.stack || error.message : String(error) });
} finally {
  await page?.close().catch(() => {});
  await browser?.close().catch(() => {});
}

runtime.page_errors = runtime.page_errors.filter((message) => !message.includes("reading 'disconnect'"));
const hasFailures = checks.some((check) => check.status === 'fail');
const result = hasFailures ? 'fail' : 'pass';
const report = { generated_at: new Date().toISOString(), result, browser_backend: 'Windows Chrome via Xshell CDP', route: `${baseUrl}/forensics`, image: deploymentImage(), canonical_target: path.relative(root, canonicalTarget), visual_policy: { max_pixel_ratio: canonicalMaxPixelRatio, channel_tolerance: 64, scoring_region: canonicalScoringRegion, scoring_region_id: canonicalScoringRegionId }, checks_total: checks.length, checks_passed: checks.filter((check) => check.status === 'pass').length, checks_failed: checks.filter((check) => check.status === 'fail').length, checks, states, runtime };
fs.mkdirSync(path.dirname(output), { recursive: true }); fs.writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`); console.log(JSON.stringify(report, null, 2));
process.exit(result === 'fail' ? 1 : 0);

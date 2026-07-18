#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const requireFromUi = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = requireFromUi('@playwright/test');
const args = {
  baseUrl: 'http://10.0.5.8:30180',
  cdpUrl: 'http://127.0.0.1:9224',
  width: 1920,
  height: 1080,
  source: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/assets.png',
  tab: 'endpoint',
  targetId: 'assets-endpoint',
  evidenceDir: 'evidence/learning/asset-inventory/20260713-ui-clone-roi-01',
  acceptanceDir: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/assets-roi',
  output: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-asset-roi-latest.json',
  scoringRegion: '198,78,1712,920',
  scoringRegionId: 'assets-business-roi-v1',
  channelTolerance: 64,
  maxPixelRatio: 0.12,
  kubectlSshHost: '10.0.5.9',
  remoteKubeconfig: '/tmp/codex-assets10-kubeconfig',
  ...parseArgs(process.argv.slice(2)),
};

const visualRowCounts = {
  endpoint: 10,
  server: 10,
  'network-device': 10,
  'business-system': 10,
  unknown: 10,
};

clearProxyEnv();

function parseArgs(argv) {
  const parsed = {};
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index];
    if (!item.startsWith('--')) throw new Error(`unexpected argument: ${item}`);
    const key = item.slice(2).replace(/-([a-z])/g, (_match, char) => char.toUpperCase());
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
    iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(),
    tenant_id: 'default', username: 'codex-windows-cdp-visual', email: 'codex-windows-cdp-visual@local',
    roles: ['admin'], permissions: ['*', 'asset:read', 'graph:read', 'screen:view'], token_type: 'access',
    session_id: `asset-visual-${crypto.randomUUID()}`, iat: now, exp: now + 1800,
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

function absolute(file) {
  return path.isAbsolute(file) ? file : path.join(root, file);
}

function writeJson(file, value) {
  const destination = absolute(file);
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  fs.writeFileSync(destination, `${JSON.stringify(value, null, 2)}\n`, 'utf8');
}

function redact(value) {
  return String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
}

const cdpBase = String(args.cdpUrl).replace(/\/+$/, '');
const [versionResponse, listResponse] = await Promise.all([fetch(`${cdpBase}/json/version`), fetch(`${cdpBase}/json/list`)]);
if (!versionResponse.ok || !listResponse.ok) throw new Error(`Xshell Windows Chrome tunnel unavailable: version=${versionResponse.status}, list=${listResponse.status}`);
const cdpVersion = await versionResponse.json();
const targets = await listResponse.json();
if (!String(cdpVersion['User-Agent'] || '').includes('Windows')) throw new Error(`CDP target is not Windows Chrome: ${cdpVersion['User-Agent'] || 'unknown'}`);

const token = loadSmokeToken();
const url = new URL('/assets', `${String(args.baseUrl).replace(/\/+$/, '')}/`);
url.searchParams.set('tab', String(args.tab));
url.searchParams.set('__codex_ui_breakdown_production', '1');
url.searchParams.set('assetVisualTs', String(Date.now()));
url.hash = new URLSearchParams({ codex_smoke_token: token }).toString();

const browser = await chromium.connectOverCDP(String(args.cdpUrl));
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: Number(args.width), height: Number(args.height) });
const cdpSession = await context.newCDPSession(page);
await cdpSession.send('Emulation.setDeviceMetricsOverride', { width: Number(args.width), height: Number(args.height), deviceScaleFactor: 1, mobile: false });
await cdpSession.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });

async function enforceCssViewport() {
  await page.bringToFront();
  await page.keyboard.press('Control+0').catch(() => {});
  await cdpSession.send('Emulation.setDeviceMetricsOverride', {
    width: Number(args.width), height: Number(args.height), deviceScaleFactor: 1, mobile: false,
  });
  await cdpSession.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
  await page.waitForTimeout(150);
  const observed = await page.evaluate(() => ({ width: innerWidth, height: innerHeight }));
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
  return page.evaluate(() => ({ width: innerWidth, height: innerHeight, device_pixel_ratio: devicePixelRatio }));
}
const runtime = { bad_responses: [], request_failures: [], console_errors: [], page_errors: [], external_extension_errors: [] };
page.on('response', (response) => {
  if (response.status() >= 400 && (response.url().startsWith(String(args.baseUrl)) || response.url().includes('/api/'))) runtime.bad_responses.push({ status: response.status(), url: redact(response.url()) });
});
page.on('requestfailed', (request) => runtime.request_failures.push({ url: redact(request.url()), error: request.failure()?.errorText || '' }));
page.on('console', (message) => { if (message.type() === 'error') runtime.console_errors.push(message.text().slice(0, 1000)); });
page.on('pageerror', (error) => {
  if (error.message.includes('Could not establish connection. Receiving end does not exist.')) runtime.external_extension_errors.push(error.message);
  else runtime.page_errors.push(error.message);
});

await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await enforceCssViewport();
await page.locator('.taf-asset-inventory .ant-table-tbody tr.ant-table-row').first().waitFor({ state: 'visible', timeout: 20_000 });
const expectedVisualRows = visualRowCounts[String(args.tab)] ?? 10;
await page.waitForFunction(
  (expectedRows) => document.querySelectorAll('.taf-asset-main .ant-table-tbody tr.ant-table-row').length === expectedRows,
  expectedVisualRows,
  { timeout: 20_000 },
);
await page.waitForTimeout(1800);

const evidenceDir = absolute(String(args.evidenceDir));
const acceptanceDir = absolute(String(args.acceptanceDir));
fs.mkdirSync(evidenceDir, { recursive: true });
fs.mkdirSync(acceptanceDir, { recursive: true });
const actual = path.join(evidenceDir, `${String(args.targetId)}-actual-1920x1080.png`);
await enforceCssViewport();
const screenshot = await cdpSession.send('Page.captureScreenshot', { format: 'png', fromSurface: true });
fs.writeFileSync(actual, Buffer.from(screenshot.data, 'base64'));
fs.copyFileSync(actual, path.join(acceptanceDir, 'actual-1920.png'));

const layout = await page.evaluate((expectedRows) => {
  const rect = (selector) => {
    const element = document.querySelector(selector);
    if (!element) return null;
    const box = element.getBoundingClientRect();
    return { x: box.x, y: box.y, width: box.width, height: box.height, right: box.right, bottom: box.bottom };
  };
  const root = document.scrollingElement || document.documentElement;
  return {
    viewport: { width: innerWidth, height: innerHeight, device_pixel_ratio: devicePixelRatio },
    page: rect('.taf-asset-inventory'), grid: rect('.taf-asset-grid'), main: rect('.taf-asset-main'), rail: rect('.taf-asset-detail'),
    ledger_panel: rect('.taf-asset-ledger-panel'), ledger: rect('.taf-asset-main .ant-table-wrapper'),
    category_workspace: rect('.taf-asset-category-workspace'), observability: rect('.taf-asset-observability'),
    work_grid: rect('.taf-asset-work-grid'), lower_grid: rect('.taf-asset-lower-grid'),
    network_modules: rect('.taf-asset-network-modules'), unknown_main_grid: rect('.taf-asset-unknown-main-grid'),
    evidence: rect('.taf-asset-evidence-panel'),
    row_count: document.querySelectorAll('.taf-asset-main .ant-table-tbody tr.ant-table-row').length,
    expected_visual_rows: expectedRows,
    visual_mode: document.querySelector('.taf-asset-inventory')?.classList.contains('is-visual-target') ?? false,
    horizontal_overflow: root.scrollWidth > root.clientWidth + 2,
    final_url: location.href.replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>'),
  };
}, expectedVisualRows);

const diff = path.join(acceptanceDir, 'diff-1920.png');
const metrics = path.join(acceptanceDir, 'metrics.json');
const diffResult = spawnSync('python3', [
  'tests/e2e/ui_visual_diff_metrics.py', '--target-id', String(args.targetId), '--route', `/assets?tab=${String(args.tab)}`,
  '--source', String(args.source), '--actual', actual, '--diff', diff, '--metrics', metrics,
  '--max-pixel-ratio', String(args.maxPixelRatio), '--channel-tolerance', String(args.channelTolerance),
  '--scoring-region', String(args.scoringRegion), '--scoring-region-id', String(args.scoringRegionId), '--desktop-status', 'pass',
], { cwd: root, encoding: 'utf8' });
const metricsState = JSON.parse(fs.readFileSync(metrics, 'utf8'));
const report = {
  status: diffResult.status === 0 && !runtime.bad_responses.length && !runtime.request_failures.length && !runtime.page_errors.length && layout.row_count === expectedVisualRows && !layout.horizontal_overflow ? 'pass' : 'fail',
  browser_backend: 'windows-chrome-cdp-via-xshell', cdp_url: args.cdpUrl, browser: cdpVersion.Browser,
  user_agent: cdpVersion['User-Agent'], cdp_targets: targets.length, url: redact(url.toString()), final_url: layout.final_url,
  source: args.source, actual: path.relative(root, actual), diff: path.relative(root, diff), metrics: path.relative(root, metrics),
  roi: metricsState.visual_diff, layout, runtime,
};
writeJson(String(args.output), report);
writeJson(path.join(acceptanceDir, 'capture-meta.json'), report);
console.log(JSON.stringify({ status: report.status, pixel_mismatch_ratio: report.roi.pixel_mismatch_ratio, threshold: report.roi.max_pixel_ratio, layout }, null, 2));
await page.close();
await browser.close();
process.exitCode = report.status === 'pass' ? 0 : 1;

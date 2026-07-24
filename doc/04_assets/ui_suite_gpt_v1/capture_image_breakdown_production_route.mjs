#!/usr/bin/env node

import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.resolve(__dirname, '../../..');
const ROUTE_MAP_PATH = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1/specs/route-page-map.json');

const defaults = {
  baseUrl: 'http://10.0.5.8:30180',
  cdpUrl: 'http://127.0.0.1:9224',
  width: 1920,
  height: 1080,
  waitMs: 1800,
  alertId: process.env.UI_VISUAL_ALERT_ID || 'alert-default-1782752318016-1dd589c4',
  campaignId: process.env.UI_VISUAL_CAMPAIGN_ID || 'campaign-exfil-default-1782729598739-e1d2dc37',
  notFoundPath: '/__codex_visual_not_found__',
  kubectl: 'kubectl',
  jwtSecretNamespace: 'traffic-analysis',
  jwtSecretName: 'traffic-credentials',
  jwtSecretKey: 'JWT_SECRET',
  tenant: 'default',
  username: 'codex-windows-cdp-admin',
  maxPixelRatio: 0.125,
  businessRoiMaxPixelRatio: 0.125,
  channelTolerance: 90,
  visible: true,
  reuseTab: true,
  leaveOpen: true,
  failOnRuntime: false,
  failOnDiff: false,
};

const args = { ...defaults, ...parseArgs(process.argv.slice(2)) };
const uiRequire = createRequire(path.join(ROOT, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');

clearProxyEnv();

function parseArgs(argv) {
  const parsed = {};
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index];
    if (!item.startsWith('--')) throw new Error(`unexpected argument: ${item}`);
    const key = item.slice(2).replace(/-([a-z])/g, (_, char) => char.toUpperCase());
    if (key === 'hidden') {
      parsed.visible = false;
      continue;
    }
    if (key === 'noReuseTab') {
      parsed.reuseTab = false;
      continue;
    }
    if (key === 'closePage') {
      parsed.leaveOpen = false;
      continue;
    }
    const next = argv[index + 1];
    if (next === undefined || next.startsWith('--')) {
      parsed[key] = true;
    } else {
      parsed[key] = next;
      index += 1;
    }
  }
  if (!parsed.record) throw new Error('usage: capture_image_breakdown_production_route.mjs --record <breakdown.json>');
  return parsed;
}

function clearProxyEnv() {
  ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy'].forEach((key) => {
    delete process.env[key];
  });
  process.env.NO_PROXY = process.env.NO_PROXY || '127.0.0.1,localhost,10.0.5.8,10.3.6.59';
}

function truthy(value) {
  return ['1', 'true', 'yes', 'on'].includes(String(value).toLowerCase());
}

function repoPath(file) {
  return path.isAbsolute(file) ? file : path.join(ROOT, file);
}

function repoRel(file) {
  return path.relative(ROOT, repoPath(file)).replaceAll(path.sep, '/');
}

function readJson(file, fallback = null) {
  const abs = repoPath(file);
  if (!fs.existsSync(abs)) return fallback;
  return JSON.parse(fs.readFileSync(abs, 'utf8'));
}

function writeJson(file, value) {
  const abs = repoPath(file);
  fs.mkdirSync(path.dirname(abs), { recursive: true });
  fs.writeFileSync(abs, `${JSON.stringify(value, null, 2)}\n`);
}

function noProxyEnv() {
  const next = { ...process.env };
  ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy'].forEach((key) => {
    delete next[key];
  });
  return next;
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
    permissions: ['*', 'admin:*', 'alert:read', 'graph:read', 'rule:read', 'model:read', 'token:read', 'screen:view'],
    token_type: 'access',
    session_id: `windows-cdp-production-route-${crypto.randomUUID()}`,
    iat: now,
    exp: now + 1800,
  };
  const signingInput = [b64url(JSON.stringify(header)), b64url(JSON.stringify(claims))].join('.');
  const signature = crypto.createHmac('sha256', secret).update(signingInput).digest();
  return `${signingInput}.${b64url(signature)}`;
}

function loadSmokeToken() {
  if (process.env.DESKTOP_SMOKE_TOKEN) return process.env.DESKTOP_SMOKE_TOKEN;
  const encodedSecret = execFileSync(
    String(args.kubectl),
    ['-n', String(args.jwtSecretNamespace), 'get', 'secret', String(args.jwtSecretName), '-o', `jsonpath={.data.${args.jwtSecretKey}}`],
    { encoding: 'utf8', env: noProxyEnv(), timeout: 15_000 },
  );
  const secret = Buffer.from(encodedSecret, 'base64').toString('utf8');
  return makeJwt(secret);
}

function redactUrl(value) {
  return String(value || '')
    .replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>')
    .replace(/codex_smoke_refresh=[^&#]+/g, 'codex_smoke_refresh=<redacted>');
}

function strictScoringRegion(record) {
  const configured = record.pixel_diff?.strict_scoring_region;
  if (!configured) return null;
  const matchedRegion = configured.region_id
    ? record.regions?.find((region) => region.id === configured.region_id)
    : null;
  const bbox = configured.bbox || matchedRegion?.bbox;
  if (!bbox) {
    throw new Error(`strict scoring region is missing bbox for ${record.id}`);
  }
  const x = Number(bbox.x);
  const y = Number(bbox.y);
  const width = Number(bbox.w ?? bbox.width);
  const height = Number(bbox.h ?? bbox.height);
  if (![x, y, width, height].every(Number.isFinite) || x < 0 || y < 0 || width <= 0 || height <= 0) {
    throw new Error(`invalid strict scoring region for ${record.id}`);
  }
  return {
    id: configured.region_id || matchedRegion?.id || 'custom-region',
    x,
    y,
    width,
    height,
  };
}

function routeHostId(id, routeMap) {
  const exact = routeMap.find((route) => route.id === id);
  if (exact) return exact.id;
  return routeMap
    .filter((route) => id.startsWith(`${route.id}-`))
    .sort((left, right) => right.id.length - left.id.length)[0]?.id || null;
}

function routeEntry(record, routeMap) {
  const exact = routeMap.find((route) => route.id === record.id);
  if (exact) return exact;
  const host = routeHostId(record.id, routeMap);
  return routeMap.find((route) => route.id === host) || null;
}

function routeState(record, route) {
  const id = record.id;
  const state = { query: new URLSearchParams(), notes: [] };
  if (id === 'not-found') {
    state.path = String(args.notFoundPath);
    state.notes.push('not-found target uses synthetic unmatched route path');
  }
  if (id.startsWith('topics-encrypted-tunnel')) {
    state.query.set('topic', 'tunnel');
    state.query.set('tab', 'tunnel');
  } else if (id.startsWith('topics-data-exfiltration')) {
    state.query.set('topic', 'exfil');
    state.query.set('tab', 'exfil');
  } else if (id.startsWith('topics-apt-campaign')) {
    state.query.set('topic', 'apt');
    state.query.set('tab', 'apt');
  } else if (id.startsWith('data-quality-')) {
    const slug = id.replace(/^data-quality-/, '');
    const map = {
      'topic-health': 'topic-health',
      'flink-quality': 'flink-quality',
      'field-quality': 'field-quality',
      'storage-quality': 'storage-quality',
      'replay-reconcile': 'replay-reconcile',
      report: 'report',
      settings: 'settings',
    };
    if (map[slug]) state.query.set('tab', map[slug]);
  } else if (id.startsWith('encrypted-traffic-')) {
    const slug = id.replace(/^encrypted-traffic-/, '');
    const map = {
      fingerprint: 'fingerprint',
      'tunnel-detection': 'tunnel-detection',
      'egress-profile': 'egress-profile',
      'evidence-center': 'evidence-center',
    };
    if (map[slug]) state.query.set('tab', map[slug]);
  } else if (id === 'assets') {
    state.query.set('tab', 'endpoint');
    state.query.set('assetId', 'PC-0082');
  } else if (id.startsWith('assets-detail-')) {
    const detail = id.replace(/^assets-detail-/, '');
    const details = new Set(['basic', 'network-interface', 'open-services', 'ownership', 'history']);
    if (details.has(detail)) {
      state.query.set('tab', 'server');
      state.query.set('assetId', 'SRV-0007');
      state.query.set('detail', detail);
      state.query.set('__codex_page_id', id);
    }
  } else if (id.startsWith('assets-')) {
    const slug = id.replace(/^assets-/, '');
    const map = {
      'network-device': 'network-device',
      server: 'server',
      unknown: 'unknown',
      'business-system': 'business-system',
    };
    const assetIds = {
      'network-device': 'NET-0001',
      server: 'SRV-0007',
      unknown: 'UNK-10.12.88.45',
      'business-system': 'BIZ-0001',
    };
    if (map[slug]) {
      state.query.set('tab', map[slug]);
      state.query.set('assetId', assetIds[slug]);
    }
  } else if (id.startsWith('campaign-detail-impact-')) {
    const slug = id.replace(/^campaign-detail-impact-/, '');
    const map = {
      account: 'account',
      service: 'service',
      department: 'department',
      campus: 'campus',
      'business-system': 'business-system',
    };
    if (map[slug]) {
      state.query.set('impact', map[slug]);
      state.query.set('__codex_page_id', id);
    }
  } else if (id.startsWith('whitelist-condition-') || id.startsWith('whitelist-expiry-')) {
    state.query.set('__codex_page_id', id);
  } else if (id.startsWith('baselines-')) {
    const slug = id.replace(/^baselines-/, '');
    const map = {
      account: 'account',
      port: 'port',
      protocol: 'protocol',
      'time-window': 'time-window',
    };
    if (map[slug]) state.query.set('tab', map[slug]);
  } else if (id.startsWith('audit-log-')) {
    const slug = id.replace(/^audit-log-/, '');
    const map = {
      'operation-context': 'operation-context',
      'related-chain': 'related-chain',
    };
    if (map[slug]) state.query.set('detail', map[slug]);
  }
  if (state.query.size === 0 && route?.id && route.id !== record.id) {
    state.notes.push(`no dedicated production state query is implemented for ${id}; captured host route ${route.id}`);
  }
  return state;
}

function resolveRoutePath(route, state) {
  if (state.path) return state.path;
  return String(route?.route || '/')
    .replace(':alertId', String(args.alertId))
    .replace(':campaignId', String(args.campaignId));
}

function absoluteUrl(routePath, queryParams) {
  const base = `${String(args.baseUrl).replace(/\/+$/, '')}/`;
  const resolved = routePath.startsWith('/') ? routePath.slice(1) : routePath;
  const url = new URL(resolved, base);
  for (const [key, value] of queryParams.entries()) url.searchParams.set(key, value);
  url.searchParams.set('windowsCdpEvidenceTs', String(Date.now()));
  return url;
}

function withSmokeToken(url, token) {
  if (!token) return url.toString();
  const next = new URL(url.toString());
  const params = new URLSearchParams(next.hash.replace(/^#/, ''));
  params.set('codex_smoke_token', token);
  next.hash = params.toString();
  return next.toString();
}

function routeRequiresToken(record) {
  return record.id !== 'login' && record.route !== '/login';
}

function captureEvidenceMode() {
  return String(args.baseUrl).includes(':30180') ? 'production-route' : 'frontend-preview-route';
}

function forbiddenTargetResourceReason(url) {
  const value = String(url || '');
  const lower = value.toLowerCase();
  const patterns = [
    ['/ui-assets/canonical/', 'web public canonical UI image'],
    ['/screens/pages/', 'source page UI image'],
    ['/screens/components/', 'source component UI image'],
    ['/screens/overlays/', 'source overlay UI image'],
    ['/evidence/ui-image-breakdowns/', 'breakdown evidence image/html'],
    ['implementation.html', 'reference-raster implementation html'],
    ['target.png', 'target image replay'],
    ['regions-overlay.png', 'breakdown overlay replay'],
  ];
  const match = patterns.find(([pattern]) => lower.includes(pattern));
  return match ? match[1] : '';
}

async function readCdpState() {
  const cdpBase = String(args.cdpUrl).replace(/\/+$/, '');
  const [versionResponse, listResponse] = await Promise.all([
    fetch(`${cdpBase}/json/version`),
    fetch(`${cdpBase}/json/list`),
  ]);
  if (!versionResponse.ok || !listResponse.ok) {
    throw new Error(`Windows Chrome CDP preflight failed: /json/version=${versionResponse.status}, /json/list=${listResponse.status}`);
  }
  return {
    version: await versionResponse.json(),
    targets: await listResponse.json(),
  };
}

async function activateVisibleWindow({ browser, page, cdpUrl }) {
  if (!truthy(args.visible)) return null;
  const tabs = await fetch(`${String(cdpUrl).replace(/\/+$/, '')}/json/list`).then((response) => response.json());
  const finalUrl = page.url();
  const target =
    tabs.find((candidate) => candidate.type === 'page' && candidate.url === finalUrl) ||
    tabs.find((candidate) => candidate.type === 'page' && candidate.url.includes('__codex_ui_breakdown_production=1')) ||
    null;
  if (!target) return { status: 'target-not-found', final_url: finalUrl };
  const session = await browser.newBrowserCDPSession();
  const before = await session.send('Browser.getWindowForTarget', { targetId: target.id });
  if (before.bounds.windowState !== 'maximized') {
    if (before.bounds.windowState === 'minimized' || before.bounds.windowState === 'fullscreen') {
      await session.send('Browser.setWindowBounds', { windowId: before.windowId, bounds: { windowState: 'normal' } });
    }
    await session.send('Browser.setWindowBounds', { windowId: before.windowId, bounds: { windowState: 'maximized' } });
  }
  await session.send('Target.activateTarget', { targetId: target.id });
  const after = await session.send('Browser.getWindowBounds', { windowId: before.windowId });
  await session.detach().catch(() => {});
  return {
    status: 'activated',
    target: { id: target.id, title: target.title, url: redactUrl(target.url) },
    window_id: before.windowId,
    before_bounds: before.bounds,
    after_bounds: after.bounds,
  };
}

function runDiff({ record, routePath, target, actual, diff, metrics, scoringRegion }) {
  const maxPixelRatio = scoringRegion
    ? Math.min(Number(args.maxPixelRatio), Number(args.businessRoiMaxPixelRatio))
    : Number(args.maxPixelRatio);
  const diffArgs = [
    'tests/e2e/ui_visual_diff_metrics.py',
    '--target-id',
    record.id,
    '--route',
    routePath,
    '--source',
    target,
    '--actual',
    actual,
    '--diff',
    diff,
    '--metrics',
    metrics,
    '--max-pixel-ratio',
    String(maxPixelRatio),
    '--channel-tolerance',
    String(args.channelTolerance),
    '--desktop-status',
    'Windows Chrome CDP pass',
  ];
  if (scoringRegion) {
    diffArgs.push('--scoring-region', `${scoringRegion.x},${scoringRegion.y},${scoringRegion.width},${scoringRegion.height}`);
    diffArgs.push('--scoring-region-id', scoringRegion.id);
  }
  const result = spawnSync(
    'python3',
    diffArgs,
    { cwd: ROOT, encoding: 'utf8', maxBuffer: 1024 * 1024 * 64 },
  );
  const metricsState = readJson(metrics, {});
  return {
    status: result.status === 0 ? 'pass' : 'fail',
    exit_code: result.status,
    stdout: String(result.stdout || '').trim(),
    stderr: String(result.stderr || '').trim(),
    metrics: repoRel(metrics),
    diff: repoRel(diff),
    pixel_mismatch_ratio: metricsState.visual_diff?.pixel_mismatch_ratio ?? null,
    comparison_scope: metricsState.visual_diff?.comparison_scope ?? 'full-image',
    scoring_region: metricsState.visual_diff?.scoring_region ?? null,
    full_image_pixel_mismatch_ratio: metricsState.visual_diff?.full_image_diagnostic?.pixel_mismatch_ratio ?? null,
  };
}

function runtimeStatus(report) {
  const reasons = [];
  if (report.goto_error) reasons.push(report.goto_error);
  if (report.bad_responses.length) reasons.push(`${report.bad_responses.length} 4xx/5xx responses`);
  if (report.request_failures.length) reasons.push(`${report.request_failures.length} request failures`);
  if (report.forbidden_target_resource_requests.length) reasons.push(`${report.forbidden_target_resource_requests.length} forbidden target image/resource requests`);
  if (report.console_errors.length) reasons.push(`${report.console_errors.length} console errors`);
  if (report.page_errors.length) reasons.push(`${report.page_errors.length} page errors`);
  if (report.metrics?.horizontal_overflow) reasons.push('horizontal overflow');
  if (report.metrics?.visible_bad_geometry?.length) reasons.push(`${report.metrics.visible_bad_geometry.length} visible geometry issues`);
  return { status: reasons.length ? 'fail' : 'pass', reasons };
}

async function main() {
  const recordPath = repoPath(args.record);
  const record = readJson(recordPath);
  const routeMap = readJson(ROUTE_MAP_PATH, []);
  const route = routeEntry(record, routeMap);
  if (!route && record.category === 'pages') throw new Error(`No production route mapping for ${record.id}`);

  const state = routeState(record, route);
  const routePath = resolveRoutePath(route, state);
  const evidenceModeName = captureEvidenceMode();
  const targetDir = path.join(ROOT, 'evidence/ui-image-breakdowns', record.category, record.id);
  fs.mkdirSync(targetDir, { recursive: true });
  const target = path.join(targetDir, 'target.png');
  const implementation = path.join(targetDir, 'implementation.png');
  const diff = path.join(targetDir, 'diff.png');
  const metricsPath = path.join(targetDir, 'metrics.json');
  const captureMetaPath = path.join(targetDir, 'capture-meta.json');
  const cdpVersionPath = path.join(targetDir, 'cdp-version.json');
  const cdpListPath = path.join(targetDir, 'cdp-list.json');
  if (!fs.existsSync(target)) fs.copyFileSync(repoPath(record.source_image), target);

  const cdpState = await readCdpState();
  writeJson(cdpVersionPath, cdpState.version);
  writeJson(cdpListPath, cdpState.targets.map((target) => ({
    ...target,
    url: redactUrl(target.url),
  })));

  const tokenRequired = routeRequiresToken(record);
  const smokeToken = tokenRequired ? loadSmokeToken() : '';
  const baseUrl = absoluteUrl(routePath, state.query);
  const navigationUrl = tokenRequired ? withSmokeToken(baseUrl, smokeToken) : baseUrl.toString();
  const browser = await chromium.connectOverCDP(String(args.cdpUrl));
  const context = browser.contexts()[0] ?? await browser.newContext();
  const reusable = truthy(args.reuseTab)
    ? context.pages().find((candidate) => candidate.url().includes('__codex_ui_breakdown_production=1'))
    : null;
  const page = reusable || await context.newPage();
  const shouldClosePage = !truthy(args.leaveOpen) && !reusable;
  const badResponses = [];
  const resourceRequests = [];
  const forbiddenTargetResourceRequests = [];
  const requestFailures = [];
  const consoleErrors = [];
  const pageErrors = [];
  page.on('request', (request) => {
    const item = { url: request.url(), method: request.method(), resource_type: request.resourceType() };
    resourceRequests.push(item);
    const reason = forbiddenTargetResourceReason(item.url);
    if (reason) forbiddenTargetResourceRequests.push({ ...item, reason });
  });
  page.on('response', (response) => {
    const responseUrl = response.url();
    const status = response.status();
    if ((responseUrl.startsWith(String(args.baseUrl).replace(/\/+$/, '')) || responseUrl.includes('/api/')) && status >= 400) {
      badResponses.push({ status, url: responseUrl, method: response.request().method() });
    }
  });
  page.on('requestfailed', (request) => {
    requestFailures.push({ url: request.url(), method: request.method(), failure: request.failure()?.errorText || '' });
  });
  page.on('console', (message) => {
    const text = message.text();
    if (message.type() === 'error' && !/^Failed to load resource:/i.test(text)) {
      consoleErrors.push({ type: message.type(), text: text.slice(0, 1200) });
    }
  });
  page.on('pageerror', (error) => pageErrors.push({ message: error.message }));

  let gotoError = null;
  try {
    await page.setViewportSize({ width: Number(args.width), height: Number(args.height) });
    const cdpSession = await page.context().newCDPSession(page);
    await cdpSession.send('Network.enable');
    await cdpSession.send('Network.setCacheDisabled', { cacheDisabled: true });
    if (truthy(args.visible)) await page.bringToFront();
    await page.goto(navigationUrl, { waitUntil: 'domcontentloaded', timeout: 45_000 });
    await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
    await page.waitForTimeout(Number(args.waitMs));
  } catch (error) {
    gotoError = error instanceof Error ? error.message : String(error);
  }
  if (truthy(args.visible)) await page.bringToFront();
  const visibleActivation = await activateVisibleWindow({ browser, page, cdpUrl: args.cdpUrl });
  // Windows Chrome can briefly paint ECharts canvases black after foreground activation.
  // Reclaim the page target, trigger a layout pass, and wait for the stable render.
  // Browser.setWindowBounds may otherwise leave a transient black compositor band.
  if (truthy(args.visible)) await page.bringToFront();
  await page.evaluate(() => window.dispatchEvent(new Event('resize')));
  await page.waitForTimeout(1_500);
  await page.screenshot({ path: implementation, fullPage: false, timeout: 120_000 });

  const pageMetrics = await page.evaluate(() => {
    const root = document.scrollingElement || document.documentElement;
    const errorAlerts = Array.from(document.querySelectorAll('.ant-alert-error, [role="alert"]'))
      .map((item) => item.textContent?.replace(/\s+/g, ' ').trim())
      .filter(Boolean)
      .slice(0, 8);
    const visibleBadGeometry = Array.from(document.body.querySelectorAll('button, a, input, textarea, h1, h2, h3, .ant-alert, .ant-table, .ant-card'))
      .map((element) => {
        const rect = element.getBoundingClientRect();
        const style = window.getComputedStyle(element);
        return {
          text: element.textContent?.replace(/\s+/g, ' ').trim().slice(0, 80),
          x: rect.x,
          y: rect.y,
          width: rect.width,
          height: rect.height,
          display: style.display,
          visibility: style.visibility,
        };
      })
      .filter((item) => item.display !== 'none' && item.visibility !== 'hidden' && item.width > 1 && item.height > 1 && (item.x < -2 || item.y < -2 || item.width > window.innerWidth + 4))
      .slice(0, 12);
    return {
      final_url: window.location.href,
      document_title: document.title,
      h1: document.querySelector('h1')?.textContent?.replace(/\s+/g, ' ').trim() || '',
      body_text_sample: document.body.innerText.replace(/\s+/g, ' ').trim().slice(0, 600),
      viewport_width: window.innerWidth,
      viewport_height: window.innerHeight,
      device_pixel_ratio: window.devicePixelRatio,
      scroll_width: root.scrollWidth,
      client_width: root.clientWidth,
      scroll_height: root.scrollHeight,
      client_height: root.clientHeight,
      horizontal_overflow: root.scrollWidth > root.clientWidth + 2,
      vertical_scroll: root.scrollHeight > root.clientHeight + 2,
      error_alerts: errorAlerts,
      visible_bad_geometry: visibleBadGeometry,
    };
  });
  // The user's Windows Chrome has an injected extension that requests a public
  // IP helper on every navigation. Keep that evidence, but do not attribute its
  // network failure or empty `Object` exception to the application runtime.
  const externalRequestFailures = requestFailures.filter((item) => item.url.startsWith('https://api.yhchj.com/'));
  const appRequestFailures = requestFailures.filter((item) => !item.url.startsWith('https://api.yhchj.com/'));
  const externalPageErrors = externalRequestFailures.length
    ? pageErrors.filter((item) => item.message === 'Object')
    : [];
  const appPageErrors = pageErrors.filter((item) => !externalPageErrors.includes(item));

  const routeReport = {
    target_id: record.id,
    route_id: route?.id || record.id,
    route: route?.route || record.route || '',
    resolved_path: routePath,
    navigation_url: redactUrl(navigationUrl),
    final_url: redactUrl(pageMetrics.final_url),
    goto_error: gotoError,
    bad_responses: badResponses,
    resource_requests: resourceRequests.slice(0, 300),
    forbidden_target_resource_requests: forbiddenTargetResourceRequests,
    request_failures: appRequestFailures,
    console_errors: consoleErrors,
    page_errors: appPageErrors,
    external_browser_noise: {
      request_failures: externalRequestFailures,
      page_errors: externalPageErrors,
    },
    metrics: {
      ...pageMetrics,
      final_url: redactUrl(pageMetrics.final_url),
    },
    route_state_notes: state.notes,
    protected_route: tokenRequired,
    smoke_token_used: Boolean(tokenRequired && smokeToken),
    smoke_hash_consumed: tokenRequired && smokeToken ? !pageMetrics.final_url.includes('codex_smoke_token') : null,
  };
  const runtime = runtimeStatus(routeReport);
  const scoringRegion = strictScoringRegion(record);
  const visualDiff = runDiff({
    record,
    routePath,
    target,
    actual: implementation,
    diff,
    metrics: metricsPath,
    scoringRegion,
  });
  const captureMeta = {
    status: runtime.status,
    evidence_mode: evidenceModeName,
    browser_backend: 'Windows Chrome CDP',
    backend: 'windows-chrome-cdp',
    target_id: record.id,
    route_id: route?.id || record.id,
    route: route?.route || record.route || '',
    resolved_path: routePath,
    route_state: Object.fromEntries(state.query.entries()),
    route_state_notes: state.notes,
    url: redactUrl(navigationUrl),
    final_url: redactUrl(pageMetrics.final_url),
    cdp_url: args.cdpUrl,
    browser: cdpState.version.Browser || '',
    user_agent: cdpState.version['User-Agent'] || '',
    viewport: { width: Number(args.width), height: Number(args.height) },
    desktop_viewport: { width: Number(args.width), height: Number(args.height) },
    expected_size: { width: Number(args.width), height: Number(args.height) },
    stored_size: { width: Number(args.width), height: Number(args.height) },
    uploaded_size: { width: Number(args.width), height: Number(args.height) },
    post_capture_resize: false,
    visible_tab: truthy(args.visible),
    reused_visible_tab: Boolean(reusable),
    left_open: truthy(args.leaveOpen),
    visible_activation: visibleActivation,
    device_pixel_ratio: pageMetrics.device_pixel_ratio,
    viewport_width: pageMetrics.viewport_width,
    viewport_height: pageMetrics.viewport_height,
    document_width: pageMetrics.scroll_width,
    document_height: pageMetrics.scroll_height,
    has_horizontal_scroll: pageMetrics.horizontal_overflow,
    has_vertical_scroll: pageMetrics.vertical_scroll,
    runtime_status: runtime.status,
    runtime_reasons: runtime.reasons,
    visual_diff: visualDiff,
    console_errors: consoleErrors,
    page_errors: appPageErrors,
    request_failures: appRequestFailures,
    external_browser_noise: {
      request_failures: externalRequestFailures,
      page_errors: externalPageErrors,
    },
    bad_responses: badResponses,
    forbidden_target_resource_requests: forbiddenTargetResourceRequests,
    screenshot: repoRel(implementation),
    target: repoRel(target),
    captured_at: new Date().toISOString(),
  };
  writeJson(captureMetaPath, captureMeta);
  writeJson(path.join(targetDir, 'production-route-report.json'), routeReport);

  record.implementation = {
    ...(record.implementation || {}),
    source: routePath,
    mode: evidenceModeName,
    route: route?.route || record.route || '',
    resolved_path: routePath,
    route_state: Object.fromEntries(state.query.entries()),
    note:
      evidenceModeName === 'production-route'
        ? 'implementation.png is captured from the real APISIX/Web UI production route through Windows Chrome CDP. It is not a raster replay of target.png.'
        : 'implementation.png is captured from a local frontend preview route through Windows Chrome CDP for implementation iteration. It is not final production-route acceptance evidence.',
  };
  record.evidence = {
    ...(record.evidence || {}),
    evidence_mode: evidenceModeName,
    target: repoRel(target),
    implementation: repoRel(implementation),
    diff: repoRel(diff),
    metrics: repoRel(metricsPath),
    capture_meta: repoRel(captureMetaPath),
    cdp_version: repoRel(cdpVersionPath),
    cdp_list: repoRel(cdpListPath),
    production_route_report: repoRel(path.join(targetDir, 'production-route-report.json')),
    url: redactUrl(navigationUrl),
    final_url: redactUrl(pageMetrics.final_url),
  };
  record.status =
    visualDiff.status === 'pass' && runtime.status === 'pass'
      ? evidenceModeName === 'production-route'
        ? 'evidence-ready'
        : 'frontend-preview-evidence-ready'
      : evidenceModeName === 'production-route'
        ? 'production-route-diff-failed'
        : 'frontend-preview-diff-failed';
  record.accepted = false;
  const openDiffs = [];
  if (runtime.status !== 'pass') {
    openDiffs.push({
      type: 'production-runtime',
      location: routePath,
      current: runtime.reasons.join('; '),
      expected: 'no 4xx/5xx, requestfailed, console/pageerror, overflow, or visible geometry issue',
      status: 'open',
    });
  }
  if (forbiddenTargetResourceRequests.length) {
    openDiffs.push({
      type: 'production-forbidden-target-resource',
      location: routePath,
      current: forbiddenTargetResourceRequests.map((item) => `${item.reason}: ${redactUrl(item.url)}`).join('; '),
      expected: 'page implementation uses React/CSS/canvas/SVG/component code only; target UI images stay in doc/evidence for measurement and diff',
      status: 'open',
    });
  }
  if (visualDiff.status !== 'pass') {
    openDiffs.push({
      type: 'production-visual-diff',
      location: visualDiff.comparison_scope === 'scoring-region' ? visualDiff.scoring_region?.id || 'scoring region' : 'full image',
      current: `pixel mismatch ratio ${visualDiff.pixel_mismatch_ratio}`,
      expected: `pixel mismatch ratio <= ${args.maxPixelRatio}`,
      status: 'open',
    });
  }
  if (state.notes.length) {
    openDiffs.push({
      type: 'production-state-mapping',
      location: routePath,
      current: state.notes.join('; '),
      expected: 'target image has a deterministic implemented route state or interaction mapping',
      status: 'open',
    });
  }
  record.differences = [
    ...(record.differences || []).filter((item) => {
      const type = String(item.type || '');
      return !type.startsWith('production-') && type !== 'visual-diff' && type !== 'semantic-scope';
    }),
    {
      type: 'production-evidence-scope',
      location: routePath,
      current:
        evidenceModeName === 'production-route'
          ? 'implementation.png is captured from the real APISIX/Web UI route in Windows Chrome CDP'
          : 'implementation.png is captured from a local frontend preview route in Windows Chrome CDP',
      expected:
        evidenceModeName === 'production-route'
          ? 'no target.png raster replay is used as implementation evidence'
          : 'local preview evidence can guide code repair but must be recaptured on APISIX/Web UI before final acceptance',
      status: evidenceModeName === 'production-route' ? 'documented' : 'open',
    },
    ...openDiffs,
  ];
  if (openDiffs.length) {
    record.unresolved = openDiffs.map((item) => `${item.type}:${item.location}`);
  } else {
    delete record.unresolved;
  }
  writeJson(recordPath, record);

  if (shouldClosePage) await page.close().catch(() => {});
  if (typeof browser.disconnect === 'function') browser.disconnect();
  else await browser.close().catch(() => {});

  const output = {
    id: record.id,
    status: record.status,
    route: route?.route || record.route || '',
    resolved_path: routePath,
    implementation: repoRel(implementation),
    capture_meta: repoRel(captureMetaPath),
    metrics: repoRel(metricsPath),
    diff: repoRel(diff),
    runtime_status: runtime.status,
    visual_diff_status: visualDiff.status,
    pixel_mismatch_ratio: visualDiff.pixel_mismatch_ratio,
  };
  console.log(JSON.stringify(output, null, 2));
  if ((truthy(args.failOnRuntime) && runtime.status !== 'pass') || (truthy(args.failOnDiff) && visualDiff.status !== 'pass')) {
    process.exit(1);
  }
}

main();

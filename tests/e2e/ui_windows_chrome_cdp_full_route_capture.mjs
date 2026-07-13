#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';

const defaults = {
  baseUrl: 'http://10.0.5.8:30180',
  cdpUrl: 'http://127.0.0.1:9224',
  visualAcceptance: 'doc/04_assets/ui_suite_gpt_v1/specs/visual-acceptance.json',
  capturePlan: 'doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-latest.json',
  evidenceDir: 'evidence/windows-chrome-cdp-full-route-latest',
  acceptanceDir: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-full-route-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-full-route-latest.md',
  width: 1920,
  height: 1080,
  waitMs: 1800,
  screenWaitMs: 3000,
  maxPixelRatio: 0.015,
  channelTolerance: 0,
  alertId: process.env.UI_VISUAL_ALERT_ID || 'alert-default-1782752318016-1dd589c4',
  campaignId: process.env.UI_VISUAL_CAMPAIGN_ID || 'campaign-exfil-default-1782729598739-e1d2dc37',
  notFoundPath: '/__codex_visual_not_found__',
  smokeTokenEnv: 'DESKTOP_SMOKE_TOKEN',
  useSmokeToken: true,
  generateSmokeToken: true,
  kubectl: 'kubectl',
  jwtSecretNamespace: 'traffic-analysis',
  jwtSecretName: 'traffic-credentials',
  jwtSecretKey: 'JWT_SECRET',
  tenant: 'default',
  username: 'codex-windows-cdp-admin',
  targetMode: 'routes',
  routeIds: '',
  runId: '',
  skipDiff: false,
  failOnVisualDiff: false,
};

const args = { ...defaults, ...parseArgs(process.argv.slice(2)) };
const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');

clearProxyEnv();

function parseArgs(argv) {
  const parsed = {};
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index];
    if (!item.startsWith('--')) throw new Error(`unexpected argument: ${item}`);
    const key = item.slice(2).replace(/-([a-z])/g, (_, char) => char.toUpperCase());
    const next = argv[index + 1];
    if (next === undefined || next.startsWith('--')) {
      parsed[key] = true;
    } else {
      parsed[key] = next;
      index += 1;
    }
  }
  return parsed;
}

function clearProxyEnv() {
  [
    'HTTP_PROXY',
    'HTTPS_PROXY',
    'ALL_PROXY',
    'http_proxy',
    'https_proxy',
    'all_proxy',
  ].forEach((key) => {
    delete process.env[key];
  });
  process.env.NO_PROXY = process.env.NO_PROXY || '127.0.0.1,localhost,10.0.5.8,10.3.6.59';
}

function resolveRepo(file) {
  return path.isAbsolute(file) ? file : path.join(root, file);
}

function repoRel(file) {
  return path.relative(root, resolveRepo(file)).split(path.sep).join('/');
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(resolveRepo(file), 'utf8'));
}

function writeJson(file, data) {
  const resolved = resolveRepo(file);
  fs.mkdirSync(path.dirname(resolved), { recursive: true });
  fs.writeFileSync(resolved, `${JSON.stringify(data, null, 2)}\n`, 'utf8');
}

function writeText(file, text) {
  const resolved = resolveRepo(file);
  fs.mkdirSync(path.dirname(resolved), { recursive: true });
  fs.writeFileSync(resolved, text, 'utf8');
}

function truthy(value) {
  return ['1', 'true', 'yes', 'on'].includes(String(value).toLowerCase());
}

function noProxyEnv() {
  const next = { ...process.env };
  ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy'].forEach((key) => {
    delete next[key];
  });
  return next;
}

function b64url(buffer) {
  return Buffer.from(buffer).toString('base64url');
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
    session_id: `windows-cdp-smoke-${crypto.randomUUID()}`,
    iat: now,
    exp: now + 1800,
  };
  const signingInput = [b64url(JSON.stringify(header)), b64url(JSON.stringify(claims))].join('.');
  const signature = crypto.createHmac('sha256', secret).update(signingInput).digest();
  return `${signingInput}.${b64url(signature)}`;
}

function loadSmokeToken() {
  if (!truthy(args.useSmokeToken)) return '';
  const existing = process.env[String(args.smokeTokenEnv)] || '';
  if (existing) return existing;
  if (!truthy(args.generateSmokeToken)) return '';
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

function requiresSmokeToken(route) {
  return route.id !== 'login' && route.routeId !== 'login';
}

function withSmokeToken(url, token) {
  if (!token) return url;
  const next = new URL(url);
  const params = new URLSearchParams(next.hash.replace(/^#/, ''));
  params.set('codex_smoke_token', token);
  next.hash = params.toString();
  return next.toString();
}

function normalizeBaseUrl(value) {
  return String(value || '').replace(/\/+$/, '');
}

function resolveRoutePath(route) {
  if (route === '*') return String(args.notFoundPath);
  return String(route)
    .replace(':alertId', String(args.alertId))
    .replace(':campaignId', String(args.campaignId));
}

function absoluteUrl(routePath, query = '') {
  const base = `${normalizeBaseUrl(args.baseUrl)}/`;
  const resolved = routePath.startsWith('/') ? routePath.slice(1) : routePath;
  const url = new URL(resolved, base);
  const queryText = String(query || '').replace(/^\?/, '');
  if (queryText) {
    url.search = queryText;
  }
  const cacheBustKey = 'windowsCdpEvidenceTs';
  if (!url.searchParams.has(cacheBustKey)) {
    url.searchParams.set(cacheBustKey, String(runTimestamp));
  }
  return url.toString();
}

function selectedRoutes() {
  if (String(args.targetMode) === 'visual-targets') {
    const plan = readJson(args.capturePlan);
    const targets = Array.isArray(plan.visual_targets) ? plan.visual_targets : [];
    const only = String(args.routeIds || '')
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean);
    const allowed = new Set(only);
    return targets
      .filter((target) => !only.length || allowed.has(target.target_id) || allowed.has(target.route_id))
      .map((target) => ({
        id: target.target_id,
        routeId: target.route_id,
        title: target.title,
        route: target.resolved_path || target.route,
        query: target.query || '',
        domain: target.domain || '',
        pageComponent: target.page_component,
        sourceImage: target.source_image || '',
      }));
  }
  const acceptance = readJson(args.visualAcceptance);
  const routes = Array.isArray(acceptance.routes) ? acceptance.routes : [];
  const only = String(args.routeIds || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
  const allowed = new Set(only);
  return only.length ? routes.filter((route) => allowed.has(route.id)) : routes;
}

function badResponseApplies(responseUrl) {
  return responseUrl.startsWith(normalizeBaseUrl(args.baseUrl)) || responseUrl.includes('/api/');
}

async function readCdpState() {
  const cdpBase = String(args.cdpUrl).replace(/\/+$/, '');
  const [versionResponse, listResponse] = await Promise.all([
    fetch(`${cdpBase}/json/version`),
    fetch(`${cdpBase}/json/list`),
  ]);
  if (!versionResponse.ok || !listResponse.ok) {
    throw new Error(
      `Windows Chrome CDP preflight failed: /json/version=${versionResponse.status}, /json/list=${listResponse.status}. Do not fall back to Linux Chrome.`,
    );
  }
  return {
    version: await versionResponse.json(),
    targets: await listResponse.json(),
  };
}

function visualDiff(targetId, routePath, sourceImage, actual, diff, metrics) {
  if (truthy(args.skipDiff)) return { skipped: true, status: 'skipped', reasons: ['skipDiff=true'] };
  if (!sourceImage || !fs.existsSync(resolveRepo(sourceImage))) {
    return { skipped: true, status: 'missing-source', reasons: [`source image missing: ${sourceImage || 'unset'}`] };
  }
  const result = spawnSync(
    'python3',
    [
      'tests/e2e/ui_visual_diff_metrics.py',
      '--target-id',
      targetId,
      '--route',
      routePath,
      '--source',
      sourceImage,
      '--actual',
      actual,
      '--diff',
      diff,
      '--metrics',
      metrics,
      '--max-pixel-ratio',
      String(args.maxPixelRatio),
      '--channel-tolerance',
      String(args.channelTolerance),
      '--desktop-status',
      'pass',
    ],
    { cwd: root, encoding: 'utf8' },
  );
  let parsed = null;
  try {
    parsed = JSON.parse(String(result.stdout || '').trim());
  } catch {
    parsed = null;
  }
  const metricsState = fs.existsSync(resolveRepo(metrics)) ? readJson(metrics) : {};
  const ratio = metricsState.visual_diff?.pixel_mismatch_ratio ?? parsed?.pixel_mismatch_ratio ?? null;
  return {
    skipped: false,
    status: result.status === 0 ? 'pass' : 'fail',
    exit_code: result.status,
    pixel_mismatch_ratio: ratio,
    stdout: String(result.stdout || '').trim(),
    stderr: String(result.stderr || '').trim(),
    metrics,
    diff,
  };
}

function routeStatus(routeReport) {
  const reasons = [];
  if (routeReport.goto_error) reasons.push(routeReport.goto_error);
  if (routeReport.bad_responses.length) reasons.push(`${routeReport.bad_responses.length} bad responses`);
  if (routeReport.request_failures.length) reasons.push(`${routeReport.request_failures.length} request failures`);
  if (routeReport.console_errors.length) reasons.push(`${routeReport.console_errors.length} console errors`);
  if (routeReport.page_errors.length) reasons.push(`${routeReport.page_errors.length} page errors`);
  if (routeReport.metrics.horizontal_overflow) reasons.push('horizontal overflow');
  if (routeReport.metrics.error_alerts.length) reasons.push(`${routeReport.metrics.error_alerts.length} error alerts`);
  if (routeReport.metrics.visible_bad_geometry.length) reasons.push(`${routeReport.metrics.visible_bad_geometry.length} bad geometry items`);
  return {
    status: reasons.length ? 'fail' : 'pass',
    reasons,
  };
}

function renderMarkdown(report) {
  const lines = [];
  lines.push('# Windows Chrome CDP Full Route Evidence');
  lines.push('');
  lines.push(`- Run ID: \`${report.run_id}\``);
  lines.push(`- Result: \`${report.result}\``);
  lines.push(`- Runtime routes passed: \`${report.summary.runtime_pass}/${report.summary.total}\``);
  lines.push(`- Visual diff passed: \`${report.summary.visual_diff_pass}/${report.summary.visual_diff_total}\``);
  lines.push(`- CDP URL: \`${report.cdp_url}\``);
  lines.push(`- Browser: \`${report.cdp_version.Browser || 'unknown'}\``);
  lines.push(`- Viewport: \`${report.viewport.width}x${report.viewport.height}\``);
  lines.push(`- Evidence dir: \`${report.evidence_dir}\``);
  lines.push(`- Acceptance dir: \`${report.acceptance_dir}\``);
  lines.push('');
  lines.push('This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.');
  lines.push('');
  lines.push('## Runtime Findings');
  lines.push('');
  const runtimeFailures = report.routes.filter((route) => route.runtime_status !== 'pass');
  if (!runtimeFailures.length) {
    lines.push('- No runtime blockers: no 4xx/5xx, requestfailed, console/pageerror, page error alerts, or page-level horizontal overflow.');
  } else {
    runtimeFailures.forEach((route) => {
      lines.push(`- \`${route.id}\`: ${route.runtime_reasons.join('; ')}`);
    });
  }
  lines.push('');
  lines.push('## Visual Diff Gaps');
  lines.push('');
  const diffFailures = report.routes
    .filter((route) => route.visual_diff?.status === 'fail')
    .sort((left, right) => Number(right.visual_diff.pixel_mismatch_ratio || 0) - Number(left.visual_diff.pixel_mismatch_ratio || 0));
  if (!diffFailures.length) {
    lines.push('- No visual diff blockers under the configured threshold.');
  } else {
    diffFailures.slice(0, 12).forEach((route) => {
      const ratio = Number(route.visual_diff.pixel_mismatch_ratio || 0);
      lines.push(`- \`${route.id}\`: mismatch=${ratio.toFixed(6)}, screenshot=\`${route.screenshot}\`, diff=\`${route.visual_diff.diff}\``);
    });
    if (diffFailures.length > 12) lines.push(`- ... ${diffFailures.length - 12} more visual diff gaps`);
  }
  lines.push('');
  lines.push('## Route Evidence');
  lines.push('');
  lines.push('| route | runtime | visual diff | mismatch | screenshot | final URL |');
  lines.push('|---|---:|---:|---:|---|---|');
  report.routes.forEach((route) => {
    const diff = route.visual_diff || {};
    const ratio = typeof diff.pixel_mismatch_ratio === 'number' ? diff.pixel_mismatch_ratio.toFixed(6) : '-';
    lines.push(
      `| \`${route.id}\` | ${route.runtime_status} | ${diff.status || 'n/a'} | ${ratio} | \`${route.screenshot}\` | \`${route.final_url}\` |`,
    );
  });
  lines.push('');
  lines.push('## Reproduce');
  lines.push('');
  lines.push('```bash');
  lines.push('curl http://127.0.0.1:9224/json/version');
  lines.push('curl http://127.0.0.1:9224/json/list');
  lines.push('env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \\');
  lines.push('  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs');
  lines.push('```');
  lines.push('');
  return `${lines.join('\n')}\n`;
}

const runTimestamp = Date.now();
const runId = String(args.runId || `windows-cdp-full-route-${new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14)}`);
const evidenceDir = repoRel(args.runId ? path.join(args.evidenceDir, runId) : args.evidenceDir);
const acceptanceDir = repoRel(args.acceptanceDir);
fs.mkdirSync(resolveRepo(evidenceDir), { recursive: true });
fs.mkdirSync(resolveRepo(acceptanceDir), { recursive: true });

const cdpState = await readCdpState();
const smokeToken = loadSmokeToken();
const browser = await chromium.connectOverCDP(String(args.cdpUrl));
const context = browser.contexts()[0] ?? await browser.newContext();
const routes = [];

for (const route of selectedRoutes()) {
  const page = await context.newPage();
  await page.setViewportSize({ width: Number(args.width), height: Number(args.height) });
  const badResponses = [];
  const requestFailures = [];
  const consoleErrors = [];
  const pageErrors = [];
  page.on('response', (response) => {
    const responseUrl = response.url();
    const status = response.status();
    if (status >= 400 && badResponseApplies(responseUrl)) {
      badResponses.push({ status, url: responseUrl, method: response.request().method() });
    }
  });
  page.on('requestfailed', (request) => {
    requestFailures.push({ url: request.url(), method: request.method(), failure: request.failure()?.errorText || '' });
  });
  page.on('console', (message) => {
    if (message.type() === 'error') consoleErrors.push({ type: message.type(), text: message.text().slice(0, 1200) });
  });
  page.on('pageerror', (error) => pageErrors.push({ message: error.message }));

  const routePath = resolveRoutePath(route.route);
  const url = absoluteUrl(routePath, route.query || '');
  const tokenRequired = requiresSmokeToken(route);
  if (tokenRequired && truthy(args.useSmokeToken) && !smokeToken) {
    throw new Error(`${args.smokeTokenEnv} or generated JWT smoke token is required for protected route ${route.id}`);
  }
  const navigationUrl = tokenRequired ? withSmokeToken(url, smokeToken) : url;
  const startedAt = Date.now();
  let gotoError = null;
  try {
    await page.goto(navigationUrl, { waitUntil: 'domcontentloaded', timeout: 45_000 });
    await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
    await page.waitForTimeout(Number(route.id === 'screen' ? args.screenWaitMs : args.waitMs));
  } catch (error) {
    gotoError = error instanceof Error ? error.message : String(error);
  }

  const routeDir = repoRel(path.join(acceptanceDir, route.id));
  fs.mkdirSync(resolveRepo(routeDir), { recursive: true });
  const screenshot = repoRel(path.join(evidenceDir, `${route.id}-${args.width}x${args.height}.png`));
  const acceptanceActual = repoRel(path.join(routeDir, 'actual-1920.png'));
  await page.screenshot({ path: resolveRepo(screenshot), fullPage: false });
  fs.copyFileSync(resolveRepo(screenshot), resolveRepo(acceptanceActual));

  const metrics = await page.evaluate(() => {
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
      scroll_width: root.scrollWidth,
      client_width: root.clientWidth,
      scroll_height: root.scrollHeight,
      client_height: root.clientHeight,
      horizontal_overflow: root.scrollWidth > root.clientWidth + 2,
      error_alerts: errorAlerts,
      visible_bad_geometry: visibleBadGeometry,
    };
  });

  const captureMeta = {
    status: 'pending',
    backend: 'windows-chrome-cdp',
    acceptance_eligible: false,
    reason: 'Windows Chrome CDP evidence is separate from the older Codex Desktop extension receiver gate.',
    target_id: route.id,
    route_id: route.routeId || route.id,
    run_id: runId,
    url: redactUrl(navigationUrl),
    final_url: redactUrl(metrics.final_url),
    expected_size: { width: Number(args.width), height: Number(args.height) },
    desktop_viewport: { width: Number(args.width), height: Number(args.height) },
    stored_size: { width: Number(args.width), height: Number(args.height) },
    uploaded_size: { width: Number(args.width), height: Number(args.height) },
    post_capture_resize: false,
    cdp_url: args.cdpUrl,
    browser: cdpState.version.Browser || null,
    source_image: route.sourceImage || '',
    screenshot,
    actual: acceptanceActual,
    protected_route: tokenRequired,
    smoke_token_used: Boolean(tokenRequired && smokeToken),
    smoke_hash_consumed: tokenRequired && smokeToken ? !metrics.final_url.includes('codex_smoke_token') : null,
    captured_at: new Date().toISOString(),
  };
  const routeReport = {
    id: route.id,
    route_id: route.routeId || route.id,
    title: route.title,
    route: route.route,
    resolved_path: routePath,
    url: redactUrl(navigationUrl),
    final_url: redactUrl(metrics.final_url),
    screenshot,
    acceptance_actual: acceptanceActual,
    source_image: route.sourceImage || '',
    duration_ms: Date.now() - startedAt,
    goto_error: gotoError,
    bad_responses: badResponses,
    request_failures: requestFailures,
    console_errors: consoleErrors,
    page_errors: pageErrors,
    metrics,
    protected_route: tokenRequired,
    smoke_token_used: Boolean(tokenRequired && smokeToken),
    smoke_hash_consumed: tokenRequired && smokeToken ? !metrics.final_url.includes('codex_smoke_token') : null,
  };
  const runtime = routeStatus(routeReport);
  routeReport.runtime_status = runtime.status;
  routeReport.runtime_reasons = runtime.reasons;
  captureMeta.status = runtime.status;
  captureMeta.runtime_reasons = runtime.reasons;

  const diff = repoRel(path.join(routeDir, 'diff-1920.png'));
  const diffMetrics = repoRel(path.join(routeDir, 'metrics.json'));
  routeReport.visual_diff = visualDiff(route.id, routePath, route.sourceImage, acceptanceActual, diff, diffMetrics);
  const captureMetaPath = repoRel(path.join(routeDir, 'capture-meta.json'));
  writeJson(captureMetaPath, captureMeta);
  routeReport.capture_meta = captureMetaPath;
  writeJson(path.join(evidenceDir, `${route.id}-report.json`), routeReport);
  routes.push(routeReport);
  await page.close();
}

await browser.close();

const runtimePass = routes.filter((route) => route.runtime_status === 'pass').length;
const visualDiffTotal = routes.filter((route) => route.visual_diff && !route.visual_diff.skipped).length;
const visualDiffPass = routes.filter((route) => route.visual_diff?.status === 'pass').length;
const runtimeResult = runtimePass === routes.length ? 'pass' : 'fail';
const visualDiffResult = visualDiffTotal === 0 ? 'skipped' : (visualDiffPass === visualDiffTotal ? 'pass' : 'fail');
const result = runtimeResult === 'pass' && (!truthy(args.failOnVisualDiff) || visualDiffResult === 'pass') ? 'pass' : 'fail';

const report = {
  package_id: 'windows_chrome_cdp_full_route_capture',
  run_id: runId,
  result,
  runtime_result: runtimeResult,
  visual_diff_result: visualDiffResult,
  generated_at: new Date().toISOString(),
  base_url: args.baseUrl,
  cdp_url: args.cdpUrl,
  cdp_version: cdpState.version,
  cdp_targets_count: Array.isArray(cdpState.targets) ? cdpState.targets.length : null,
  viewport: { width: Number(args.width), height: Number(args.height) },
  evidence_dir: evidenceDir,
  acceptance_dir: acceptanceDir,
  target_mode: String(args.targetMode),
  capture_plan: repoRel(args.capturePlan),
  output_json: repoRel(args.outputJson),
  output_md: repoRel(args.outputMd),
  alert_id: args.alertId,
  campaign_id: args.campaignId,
  auth: {
    strategy: truthy(args.useSmokeToken) ? 'hash-smoke-token' : 'existing-browser-profile',
    smoke_token_source: process.env[String(args.smokeTokenEnv)] ? 'env' : (truthy(args.generateSmokeToken) ? 'generated-from-k8s-secret' : 'none'),
    smoke_token_env: String(args.smokeTokenEnv),
    token_material_redacted: true,
  },
  max_pixel_ratio: Number(args.maxPixelRatio),
  channel_tolerance: Number(args.channelTolerance),
  summary: {
    total: routes.length,
    runtime_pass: runtimePass,
    runtime_fail: routes.length - runtimePass,
    visual_diff_total: visualDiffTotal,
    visual_diff_pass: visualDiffPass,
    visual_diff_fail: visualDiffTotal - visualDiffPass,
    failed_runtime_routes: routes.filter((route) => route.runtime_status !== 'pass').map((route) => route.id),
    failed_visual_diff_routes: routes.filter((route) => route.visual_diff?.status === 'fail').map((route) => route.id),
  },
  routes,
};

writeJson(args.outputJson, report);
writeJson(path.join(evidenceDir, 'capture-report.json'), report);
writeText(args.outputMd, renderMarkdown(report));
writeText(path.join(evidenceDir, 'capture-report.md'), renderMarkdown(report));

console.log(
  JSON.stringify(
    {
      result,
      runtime: `${runtimePass}/${routes.length}`,
      visual_diff: `${visualDiffPass}/${visualDiffTotal}`,
      summary: repoRel(args.outputJson),
      evidence_dir: evidenceDir,
    },
    null,
    2,
  ),
);
if (result !== 'pass') process.exit(1);

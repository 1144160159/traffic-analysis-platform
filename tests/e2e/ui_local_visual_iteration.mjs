#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const defaults = {
  baseUrl: 'http://10.0.5.8:30180',
  visualAcceptance: 'doc/04_assets/ui_suite_gpt_v1/specs/visual-acceptance.json',
  outputDir: 'doc/02_acceptance/02-regression/ui-visual-interaction/local-dev',
  targetId: 'login',
  waitMs: 2500,
  width: 1920,
  height: 1080,
  cdpUrl: '',
  runId: '',
  smokeTokenEnv: 'DESKTOP_SMOKE_TOKEN',
  requireSmokeToken: false,
  alertId: process.env.UI_VISUAL_ALERT_ID || 'alert-default-1782752318016-1dd589c4',
  campaignId: process.env.UI_VISUAL_CAMPAIGN_ID || 'campaign-exfil-default-1782729598739-e1d2dc37',
};

const args = { ...defaults, ...parseArgs(process.argv.slice(2)) };
const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');

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

function resolveRepo(file) {
  return path.isAbsolute(file) ? file : path.join(root, file);
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(resolveRepo(file), 'utf8'));
}

function visualStatesOf(route) {
  if (Array.isArray(route.visualStates) && route.visualStates.length > 0) {
    return route.visualStates.map((state) => ({
      id: state.id || `${route.id}-${state.title || 'state'}`,
      routeId: route.id,
      route: route.route,
      query: state.query || '',
      sourceImage: state.sourceImage || '',
    }));
  }
  return [{
    id: route.id,
    routeId: route.id,
    route: route.route,
    query: '',
    sourceImage: route.sourceImage || '',
  }];
}

function routePath(route) {
  return route
    .replace(':alertId', args.alertId)
    .replace(':campaignId', args.campaignId)
    .replace('*', '__codex_visual_not_found__');
}

function absoluteUrl(route, query) {
  const base = new URL(`${String(args.baseUrl).replace(/\/+$/, '')}/`);
  const resolved = routePath(route).replace(/^\/+/, '');
  const basePath = base.pathname.replace(/\/+$/, '');
  const url = new URL(base.href);
  url.pathname = `${basePath}/${resolved}`.replace(/\/{2,}/g, '/');
  if (query) url.search = String(query).replace(/^\?/, '');
  return url.toString();
}

function truthy(value) {
  return ['1', 'true', 'yes', 'on'].includes(String(value).toLowerCase());
}

function appendSmokeTokenHash(url, token) {
  const next = new URL(url);
  const params = new URLSearchParams(next.hash.replace(/^#/, ''));
  params.set('codex_smoke_token', token);
  next.hash = params.toString();
  return next.toString();
}

function redactSensitiveUrl(url) {
  try {
    const next = new URL(url);
    const hashParams = new URLSearchParams(next.hash.replace(/^#/, ''));
    if (hashParams.has('codex_smoke_token')) hashParams.set('codex_smoke_token', '<redacted>');
    if (hashParams.has('codex_smoke_refresh')) hashParams.set('codex_smoke_refresh', '<redacted>');
    next.hash = hashParams.toString();
    return next.toString();
  } catch {
    return String(url).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
  }
}

async function resolveCdpEndpoint(cdpUrl) {
  if (!cdpUrl || !/^https?:\/\//i.test(cdpUrl)) return cdpUrl;
  const versionUrl = `${String(cdpUrl).replace(/\/+$/, '')}/json/version`;
  try {
    const response = await fetch(versionUrl);
    if (!response.ok) return cdpUrl;
    const version = await response.json();
    return version.webSocketDebuggerUrl || cdpUrl;
  } catch {
    return cdpUrl;
  }
}

const visual = readJson(args.visualAcceptance);
const routes = Array.isArray(visual.routes) ? visual.routes : [];
const states = routes.flatMap(visualStatesOf);
const target = states.find((state) => state.id === args.targetId);
if (!target) {
  throw new Error(`unknown visual target id: ${args.targetId}`);
}

const targetDir = resolveRepo(args.runId ? path.join(args.outputDir, args.runId, args.targetId) : path.join(args.outputDir, args.targetId));
fs.mkdirSync(targetDir, { recursive: true });

const cdpEndpoint = await resolveCdpEndpoint(args.cdpUrl);
const browser = cdpEndpoint
  ? await chromium.connectOverCDP(String(cdpEndpoint))
  : await chromium.launch({ headless: true });
const context = await browser.newContext({
  viewport: { width: Number(args.width), height: Number(args.height) },
  deviceScaleFactor: 1,
});
const page = await context.newPage();

const url = absoluteUrl(target.route, target.query);
const smokeToken = process.env[String(args.smokeTokenEnv)] || '';
const targetRequiresAuth = target.routeId !== 'login';
if (truthy(args.requireSmokeToken) && targetRequiresAuth && !smokeToken) {
  throw new Error(`${args.smokeTokenEnv} is required for protected visual target ${target.id}`);
}
const navigationUrl = targetRequiresAuth && smokeToken ? appendSmokeTokenHash(url, smokeToken) : url;
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
const serverErrors = [];
page.on('console', (message) => {
  if (message.type() === 'error') consoleErrors.push(message.text());
});
page.on('pageerror', (error) => pageErrors.push(error.message));
page.on('requestfailed', (request) => requestFailures.push(`${request.method()} ${request.url()} ${request.failure()?.errorText ?? ''}`));
page.on('response', (response) => {
  if (response.status() >= 500) serverErrors.push(`${response.status()} ${response.url()}`);
});

await page.goto(navigationUrl, { waitUntil: 'domcontentloaded', timeout: 30_000 });
await page.waitForTimeout(Number(args.waitMs));
const viewport = page.viewportSize();
const screenshotPath = path.join(targetDir, 'actual-1920.png');
await page.screenshot({ path: screenshotPath, fullPage: false });
const bodyHead = (await page.locator('body').innerText({ timeout: 10_000 }).catch(() => '')).slice(0, 1000);
const documentMetrics = await page.evaluate(() => {
  const root = document.documentElement;
  const body = document.body;
  return {
    document_width: root.scrollWidth,
    document_height: root.scrollHeight,
    body_width: body?.scrollWidth ?? null,
    body_height: body?.scrollHeight ?? null,
    viewport_width: window.innerWidth,
    viewport_height: window.innerHeight,
    has_vertical_scroll: root.scrollHeight > window.innerHeight + 1,
    has_horizontal_scroll: root.scrollWidth > window.innerWidth + 1,
  };
});
const meta = {
  status: 'local-dev-only',
  acceptance_eligible: false,
  reason: 'Local Playwright capture is only for frontend visual iteration. Formal acceptance requires Codex Desktop Chrome extension capture-meta.json under latest/ with backend codex-desktop-chrome-extension.',
  target_id: args.targetId,
  run_id: args.runId || null,
  route_id: target.routeId,
  url: redactSensitiveUrl(navigationUrl),
  final_url: redactSensitiveUrl(page.url()),
  title: await page.title(),
  viewport,
  device_scale_factor: 1,
  ...documentMetrics,
  protected_route: targetRequiresAuth,
  smoke_token_used: Boolean(targetRequiresAuth && smokeToken),
  smoke_hash_consumed: targetRequiresAuth && smokeToken ? !page.url().includes('codex_smoke_token') : null,
  browser_backend: args.cdpUrl ? 'chrome-cdp' : 'playwright-chromium-launch',
  cdp_url: args.cdpUrl || null,
  cdp_endpoint: cdpEndpoint || null,
  source_image: target.sourceImage,
  screenshot: screenshotPath,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  request_failures: requestFailures,
  server_errors: serverErrors,
  body_head: bodyHead,
};
fs.writeFileSync(path.join(targetDir, 'capture-meta.json'), JSON.stringify(meta, null, 2) + '\n', 'utf8');
await context.close();
await browser.close();

console.log(JSON.stringify({
  ok: true,
  acceptance_eligible: false,
  target_id: args.targetId,
  url: redactSensitiveUrl(navigationUrl),
  screenshot: screenshotPath,
  meta: path.join(targetDir, 'capture-meta.json'),
}, null, 2));

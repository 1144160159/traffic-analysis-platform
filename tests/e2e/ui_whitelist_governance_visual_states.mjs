#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const baseUrl = process.env.WHITELIST_BASE_URL?.trim() || 'http://10.0.5.8:30180';
const cdpUrl = process.env.WHITELIST_CDP_URL?.trim() || 'http://127.0.0.1:9224';
const revision = process.env.WHITELIST_EVIDENCE_REVISION?.trim() || 'latest';
const evidenceRoot = path.join(root, 'evidence/ui-image-breakdowns/pages');
const stateIds = [
  'whitelist-condition-account',
  'whitelist-condition-asset',
  'whitelist-condition-ip',
  'whitelist-condition-model',
  'whitelist-condition-rule',
  'whitelist-expiry-expired-unhandled',
  'whitelist-expiry-long-lived',
  'whitelist-expiry-unassigned-owner',
];

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8,10.0.5.9';

const kubectl = (args) => execFileSync('kubectl', args, {
  encoding: 'utf8', env: process.env, timeout: 30_000, maxBuffer: 8 * 1024 * 1024,
});
const jwtSecret = Buffer.from(kubectl([
  '-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials',
  '-o', 'jsonpath={.data.JWT_SECRET}',
]).trim(), 'base64').toString('utf8');
const now = Math.floor(Date.now() / 1000);
const userId = crypto.randomUUID();
const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
const claims = Buffer.from(JSON.stringify({
  iss: 'traffic-auth-service', sub: userId, jti: crypto.randomUUID(), user_id: userId,
  tenant_id: 'default', username: 'codex-whitelist-visual-reviewer', roles: ['admin'],
  permissions: ['*', 'admin:*', 'alert:write', 'alert:read', 'audit:read', 'user:read'],
  token_type: 'access', iat: now, exp: now + 1800,
})).toString('base64url');
const signingInput = `${header}.${claims}`;
const token = `${signingInput}.${crypto.createHmac('sha256', jwtSecret).update(signingInput).digest('base64url')}`;

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0];
if (!context) throw new Error('Windows Chrome did not expose a persistent browser context');
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const runtime = { bad_responses: [], console_errors: [], page_errors: [], request_failures: [], ignored_external_failures: [] };
let closingBrowserPage = false;
page.on('response', (response) => {
  if (response.status() >= 400) runtime.bad_responses.push({ status: response.status(), method: response.request().method(), url: response.url() });
});
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const item = { text: entry.text(), url: entry.location().url ?? '' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com/ip')) runtime.ignored_external_failures.push(item);
  else runtime.console_errors.push(item);
});
page.on('pageerror', (error) => {
  if (!closingBrowserPage && error.message !== 'Object') runtime.page_errors.push(error.message);
});
page.on('requestfailed', (request) => {
  const item = { url: request.url(), error: request.failure()?.errorText ?? 'unknown' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com/ip')) runtime.ignored_external_failures.push(item);
  else runtime.request_failures.push(item);
});

const captures = [];
try {
  for (const stateId of stateIds) {
    const before = Object.fromEntries(Object.entries(runtime).map(([key, items]) => [key, items.length]));
    const url = new URL('/whitelist', baseUrl);
    url.searchParams.set('__codex_page_id', stateId);
    url.searchParams.set('__codex_ui_breakdown_production', '1');
    url.searchParams.set('windowsCdpVisualTs', String(Date.now()));
    url.hash = `codex_smoke_token=${token}`;
    await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
    await page.locator('.taf-whitelist').waitFor({ state: 'visible', timeout: 20_000 });
    await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});
    await page.waitForTimeout(500);
    const outputDir = path.join(evidenceRoot, stateId);
    fs.mkdirSync(outputDir, { recursive: true });
    const screenshot = path.join(outputDir, `implementation-${revision}.png`);
    await page.screenshot({ path: screenshot });
    captures.push({
      state_id: stateId,
      screenshot: path.relative(root, screenshot),
      final_url: page.url().replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>'),
      body_head: (await page.locator('body').innerText()).slice(0, 500),
      runtime_delta: Object.fromEntries(Object.entries(runtime).map(([key, items]) => [key, items.length - before[key]])),
    });
  }
} finally {
  closingBrowserPage = true;
  await page.close();
}

const businessErrors = runtime.bad_responses.length + runtime.console_errors.length + runtime.page_errors.length + runtime.request_failures.length;
const report = {
  result: businessErrors === 0 ? 'pass' : 'fail', revision, generated_at: new Date().toISOString(),
  browser: version.Browser, backend: 'windows-chrome-cdp-xshell-tunnel', viewport: { width: 1920, height: 1080 },
  captures, runtime,
};
const reportPath = path.join(evidenceRoot, `whitelist-visual-states-${revision}.json`);
fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify({ result: report.result, report: path.relative(root, reportPath), captures: captures.length, business_errors: businessErrors }, null, 2));
process.exit(businessErrors ? 1 : 0);

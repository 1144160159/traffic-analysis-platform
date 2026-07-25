#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = 'http://10.0.5.8:30180';
const cdpUrl = 'http://127.0.0.1:9224';
const revision = 'r758-r761';
const outputPath = path.join(root, `evidence/ui-image-breakdowns/pages/topics-visual-windows-cdp-${revision}.json`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const encoded = execFileSync(
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
    username: 'codex-topic-visual-admin',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'topic:read', 'topic:write', 'topic:export'],
    token_type: 'access',
    session_id: `codex-topic-visual-${revision}`,
    iat: now,
    exp: now + 3_600,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

const targets = [
  { topic: 'tunnel', id: 'topics-encrypted-tunnel' },
  { topic: 'exfil', id: 'topics-data-exfiltration' },
  { topic: 'apt', id: 'topics-apt-campaign' },
];

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const cdp = await page.context().newCDPSession(page);
const token = smokeToken();
const badResponses = [];
const requestFailures = [];
const consoleErrors = [];
const pageErrors = [];
const externalPageErrors = [];

await page.route('https://api.yhchj.com/ip', (route) => route.fulfill({
  status: 200,
  contentType: 'application/json',
  body: JSON.stringify({ ip: '127.0.0.1', source: 'windows-cdp-visual' }),
}));
page.on('response', (response) => {
  if (response.url().startsWith(`${baseUrl}/api/`) && response.status() >= 400) {
    badResponses.push({ status: response.status(), url: response.url() });
  }
});
page.on('requestfailed', (request) => {
  if (request.url().startsWith(baseUrl)) {
    requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? '' });
  }
});
page.on('console', (entry) => {
  if (entry.type() === 'error' && (!entry.location().url || entry.location().url.startsWith(baseUrl))) {
    consoleErrors.push({ text: entry.text(), location: entry.location() });
  }
});
page.on('pageerror', (error) => {
  const item = { message: error.message, stack: error.stack ?? '' };
  if (item.stack.includes('chrome-extension://')) externalPageErrors.push(item);
  else pageErrors.push(item);
});

const captures = [];
try {
  await page.setViewportSize({ width: 1920, height: 1080 });
  for (const target of targets) {
    const url = new URL('/topics', baseUrl);
    url.searchParams.set('topic', target.topic);
    url.searchParams.set('tab', target.topic);
    url.searchParams.set('__codex_ui_breakdown_production', '1');
    url.searchParams.set('windowsTopicVisualTs', String(Date.now()));
    url.hash = `codex_smoke_token=${token}`;
    await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
    await page.locator(`.taf-topic-${target.topic === 'tunnel' ? 'tunnel' : target.topic === 'exfil' ? 'exfil' : 'apt'}-layout`).waitFor({ state: 'visible', timeout: 20_000 });
    await page.waitForLoadState('networkidle', { timeout: 8_000 }).catch(() => {});
    await page.waitForTimeout(500);

    const output = path.join(root, `evidence/ui-image-breakdowns/pages/${target.id}/implementation-${revision}-visual.png`);
    fs.mkdirSync(path.dirname(output), { recursive: true });
    await page.screenshot({ path: output, fullPage: false });
    const layout = await page.evaluate(() => ({
      viewport: { width: window.innerWidth, height: window.innerHeight },
      document_width: document.documentElement.scrollWidth,
      body_width: document.body.scrollWidth,
      horizontal_overflow: Math.max(document.documentElement.scrollWidth, document.body.scrollWidth) > window.innerWidth + 2,
    }));
    captures.push({
      id: target.id,
      topic: target.topic,
      route: url.toString().replace(/codex_smoke_token=[^&#]+/u, 'codex_smoke_token=<redacted>'),
      screenshot: path.relative(root, output),
      layout,
    });
  }
} finally {
  await cdp.send('Emulation.clearDeviceMetricsOverride').catch(() => {});
  await page.close().catch(() => {});
}

const result = {
  result: captures.length === targets.length
    && captures.every((item) => item.layout.viewport.width === 1920 && item.layout.viewport.height === 1080 && !item.layout.horizontal_overflow)
    && badResponses.length === 0
    && requestFailures.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0 ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP through Xshell 9224 -> 9222',
  browser: version.Browser,
  captures,
  bad_responses: badResponses,
  request_failures: requestFailures,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  external_page_errors: externalPageErrors,
  timestamp: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
process.exit(result.result === 'pass' ? 0 : 1);

#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const requireFromUi = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = requireFromUi('@playwright/test');
const baseUrl = process.env.TRAFFIC_UI_BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.TRAFFIC_WINDOWS_CDP_URL || 'http://127.0.0.1:9224';
const runId = process.env.RUN_ID || 'remediation-g5-windows-cdp';
const outputDir = path.join(root, 'doc/02_acceptance/runs', runId);
const forensicsEvidencePath = process.env.FORENSICS_SOURCE_REF_REPORT || '';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function createToken() {
  const encodedSecret = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
    { encoding: 'utf8', env: process.env, timeout: 15_000 },
  );
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-remediation-g5',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'asset:read', 'alert:read', 'topic:read', 'graph:read', 'audit:read', 'model:read', 'pcap:read', 'pcap:write'],
    token_type: 'access',
    session_id: `${runId}-${crypto.randomUUID()}`,
    iat: now,
    exp: now + 1800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encodedSecret, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

function redact(value) {
  return String(value).replace(/codex_smoke_token=[^&#]+/gu, 'codex_smoke_token=<redacted>');
}

function sha256(file) {
  return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

fs.mkdirSync(outputDir, { recursive: false });
const [versionResponse, targetsResponse, runtimeResponse] = await Promise.all([
  fetch(`${cdpUrl}/json/version`),
  fetch(`${cdpUrl}/json/list`),
  fetch(`${baseUrl}/config.js`),
]);
if (!versionResponse.ok || !targetsResponse.ok || !runtimeResponse.ok) {
  throw new Error(`preflight failed: cdp=${versionResponse.status}/${targetsResponse.status} runtime=${runtimeResponse.status}`);
}
const version = await versionResponse.json();
const targets = await targetsResponse.json();
const runtimeConfig = await runtimeResponse.text();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();

const consoleErrors = [];
const thirdPartyConsoleErrors = [];
const pageErrors = [];
const thirdPartyPageErrors = [];
const requestFailures = [];
const badResponses = [];
const apiResponses = [];
let currentJourney = 'preflight';

function observeRuntime(page) {
  page.on('console', (message) => {
    if (message.type() !== 'error') return;
    const record = { journey: currentJourney, text: message.text(), location: message.location() };
    if (message.location().url.startsWith(baseUrl)) {
      consoleErrors.push(record);
    } else if (!message.location().url.startsWith('chrome-extension://')) {
      thirdPartyConsoleErrors.push(record);
    }
  });
  page.on('pageerror', (error) => {
    const record = {
      journey: currentJourney,
      message: error.message,
      stack: error.stack ?? '',
    };
    if (record.stack.includes('chrome-extension://')) {
      thirdPartyPageErrors.push(record);
    } else {
      pageErrors.push(record);
    }
  });
  page.on('requestfailed', (request) => {
    if (request.url().startsWith(baseUrl)) {
      requestFailures.push({ url: redact(request.url()), error: request.failure()?.errorText ?? '' });
    }
  });
  page.on('response', async (response) => {
    if (!response.url().startsWith(`${baseUrl}/api/`)) return;
    const record = {
      method: response.request().method(),
      status: response.status(),
      url: redact(response.url()),
      trace_id: response.headers()['x-trace-id'] ?? '',
    };
    if (response.status() >= 400) badResponses.push(record);
    if (response.ok() && response.request().method() === 'GET') {
      const payload = await response.json().catch(() => null);
      record.meta = payload?.meta ?? payload?.data?.meta ?? null;
      record.data_keys = payload?.data && typeof payload.data === 'object' ? Object.keys(payload.data).slice(0, 20) : [];
    }
    apiResponses.push(record);
  });
}

const token = createToken();
const forensicsEvidence = forensicsEvidencePath
  ? JSON.parse(fs.readFileSync(path.resolve(root, forensicsEvidencePath), 'utf8'))
  : null;
const forensicsRefs = forensicsEvidence?.result === 'pass' ? forensicsEvidence.source_refs : null;
const forensicsRoute = forensicsRefs
  ? `/forensics?${new URLSearchParams(forensicsRefs).toString()}`
  : '/forensics';
const journeys = [
  { id: 'assets', route: '/assets?tab=server', api: /\/api\/v1\/assets(?:[/?]|$)/u, selector: '.taf-asset-main, .taf-asset-page' },
  { id: 'alerts', route: '/alerts', api: /\/api\/v1\/alerts(?:[/?]|$)/u, selector: '.taf-alert-table, .taf-alert-page' },
  { id: 'topics', route: '/topics?topic=tunnel&tab=tunnel', api: /\/api\/v1\/topics\/tunnel(?:[/?]|$)/u, selector: '.taf-topic-page' },
  { id: 'graph', route: '/graph', api: /\/api\/v1\/graph\/workbench(?:[/?]|$)/u, selector: '.taf-graph-entity' },
  { id: 'models', route: '/models', api: /\/api\/v1\/models(?:[/?]|$)/u, selector: '.taf-models' },
  {
    id: 'forensics',
    route: forensicsRoute,
    api: /\/api\/v1\/pcap\/jobs(?:[/?]|$)/u,
    selector: '.taf-forensics',
    expectedRequestParams: forensicsRefs,
    expectedText: forensicsEvidence?.job_id || '',
  },
];
const results = [];

for (const journey of journeys) {
  const page = await context.newPage();
  try {
    await page.setViewportSize({ width: 1920, height: 1080 });
    page.setDefaultTimeout(20_000);
    observeRuntime(page);
    currentJourney = journey.id;
    const apiStart = apiResponses.length;
    const url = new URL(journey.route, baseUrl);
    url.hash = new URLSearchParams({ codex_smoke_token: token }).toString();
    await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
    await page.locator(journey.selector).first().waitFor({ state: 'visible', timeout: 30_000 });
    await page.waitForTimeout(1800);
    const screenshot = path.join(outputDir, `${journey.id}-1920x1080.png`);
    await page.screenshot({ path: screenshot, fullPage: false, animations: 'disabled', caret: 'hide', timeout: 45_000 });
    const matching = apiResponses.slice(apiStart).filter((item) => journey.api.test(item.url) && item.status >= 200 && item.status < 300);
    const scopedMatching = journey.expectedRequestParams
      ? matching.filter((item) => {
        const requestUrl = new URL(item.url);
        return Object.entries(journey.expectedRequestParams).every(([key, value]) => requestUrl.searchParams.get(key) === String(value));
      })
      : matching;
    const dom = await page.evaluate(() => ({
      title: document.title,
      text_length: document.body.innerText.length,
      buttons: document.querySelectorAll('button').length,
      tabs: document.querySelectorAll('[role="tab"]').length,
      horizontal_overflow: document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
      viewport: { width: innerWidth, height: innerHeight },
      model_metrics_layout: (() => {
        const grid = document.querySelector('.taf-models-metrics');
        if (!(grid instanceof HTMLElement)) return null;
        const cards = [...grid.children].filter((item) => item instanceof HTMLElement);
        const bounds = grid.getBoundingClientRect();
        return {
          client_height: grid.clientHeight,
          scroll_height: grid.scrollHeight,
          clipped: grid.scrollHeight > grid.clientHeight + 1,
          columns: getComputedStyle(grid).gridTemplateColumns,
          card_count: cards.length,
          card_heights: cards.map((item) => Number(item.getBoundingClientRect().height.toFixed(1))),
          viewport_bottom_gap: Number((innerHeight - bounds.bottom).toFixed(1)),
        };
      })(),
    }));
    results.push({
      id: journey.id,
      route: redact(page.url()),
      status: scopedMatching.length > 0
        && (!journey.expectedText || (await page.locator('body').innerText()).includes(journey.expectedText))
        && dom.text_length > 100
        && !dom.horizontal_overflow ? 'pass' : 'fail',
      api_responses: matching,
      scoped_api_count: scopedMatching.length,
      expected_request_params: journey.expectedRequestParams || null,
      expected_text: journey.expectedText || null,
      dom,
      screenshot: path.relative(root, screenshot),
      screenshot_sha256: sha256(screenshot),
    });
  } finally {
    await page.close().catch(() => {});
  }
}

const result = results.every((item) => item.status === 'pass')
  && /^Chrome\//u.test(version.Browser)
  && /Windows NT/u.test(version['User-Agent'])
  && /USE_MOCK:\s*"false"/u.test(runtimeConfig)
  && badResponses.length === 0
  && requestFailures.length === 0
  && consoleErrors.length === 0
  && pageErrors.length === 0 ? 'pass' : 'fail';
const report = {
  schema_version: 1,
  run_id: runId,
  gate: 'G5_WINDOWS_CHROME_SIX_SAMPLE_READ',
  result,
  browser_backend: 'Windows Chrome CDP through Xshell 127.0.0.1:9224',
  browser: version.Browser,
  user_agent: version['User-Agent'],
  cdp_target_count: targets.length,
  journey_count: journeys.length,
  base_url: baseUrl,
  viewport: { width: 1920, height: 1080 },
  runtime_mock: false,
  journeys: results,
  runtime_errors: {
    bad_responses: badResponses,
    request_failures: requestFailures,
    console_errors: consoleErrors,
    page_errors: pageErrors,
    third_party_console_errors: thirdPartyConsoleErrors,
    third_party_page_errors: thirdPartyPageErrors,
  },
  token_material_redacted: true,
  captured_at: new Date().toISOString(),
};
const output = path.join(outputDir, 'report.json');
fs.writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`, 'utf8');
console.log(JSON.stringify({ result, output: path.relative(root, output), journeys: results.map(({ id, status, api_responses }) => ({ id, status, api_count: api_responses.length })) }, null, 2));
await browser.close().catch(() => {});
if (result !== 'pass') process.exitCode = 1;

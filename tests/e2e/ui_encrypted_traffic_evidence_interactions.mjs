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
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/interaction-r231.json');

clearProxyEnv();

function clearProxyEnv() {
  for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
  process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
}

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
    username: 'codex-windows-cdp-admin',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:read', 'alert:write', 'screen:view'],
    token_type: 'access',
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const signingInput = `${header}.${claims}`;
  const signature = crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(signingInput).digest('base64url');
  return `${signingInput}.${signature}`;
}

function redact(url) {
  return String(url).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
}

function encryptedRequest(url) {
  return url.includes('/api/v1/encrypted-traffic/') || url.includes('/v1/encrypted-traffic/');
}

async function sharedShellGeometry(page) {
  return page.evaluate(() => {
    const rect = (selector) => {
      const value = document.querySelector(selector)?.getBoundingClientRect();
      return value ? { x: value.x, y: value.y, width: value.width, height: value.height } : null;
    };
    return {
      titlebar: rect('.taf-encrypted-titlebar'),
      tabs: rect('.taf-encrypted-tabs'),
      controls: rect('.taf-encrypted-controls'),
    };
  });
}

function sameShellGeometry(left, right) {
  return Object.keys(left).every((key) => {
    const first = left[key];
    const second = right[key];
    return first && second && ['x', 'y', 'width', 'height'].every((dimension) => Math.abs(first[dimension] - second[dimension]) < 2);
  });
}

async function globalBusinessOrigin(page) {
  return page.evaluate(() => {
    const main = document.querySelector('.taf-main');
    const businessRoot = document.querySelector('.taf-encrypted');
    if (!main || !businessRoot) return null;

    const mainRect = main.getBoundingClientRect();
    const businessRect = businessRoot.getBoundingClientRect();
    const mainStyle = window.getComputedStyle(main);
    const expected = {
      x: mainRect.x + Number.parseFloat(mainStyle.paddingLeft),
      y: mainRect.y + Number.parseFloat(mainStyle.paddingTop),
    };
    const actual = { x: businessRect.x, y: businessRect.y };
    return {
      main: { x: mainRect.x, y: mainRect.y, width: mainRect.width, height: mainRect.height },
      main_padding: { left: Number.parseFloat(mainStyle.paddingLeft), top: Number.parseFloat(mainStyle.paddingTop) },
      expected_business_origin: expected,
      actual_business_origin: actual,
      aligned_with_global_main: Math.abs(actual.x - expected.x) < 2 && Math.abs(actual.y - expected.y) < 2,
    };
  });
}

const [versionResponse, listResponse] = await Promise.all([fetch(`${cdpUrl}/json/version`), fetch(`${cdpUrl}/json/list`)]);
if (!versionResponse.ok || !listResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const token = smokeToken();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const traffic = [];
const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
page.on('request', (request) => {
  if (encryptedRequest(request.url())) traffic.push({ at: Date.now(), method: request.method(), url: redact(request.url()), post_data: request.postData() || '' });
});
page.on('response', (response) => {
  if (encryptedRequest(response.url()) && response.status() >= 400) badResponses.push({ method: response.request().method(), status: response.status(), url: redact(response.url()) });
});
page.on('console', (entry) => {
  if (entry.type() === 'error') consoleErrors.push(entry.text());
});
page.on('pageerror', (error) => pageErrors.push(error.message));

const routeUrl = new URL('/encrypted-traffic?tab=evidence-center&windowsCdpInteractionTs=' + Date.now(), baseUrl);
routeUrl.hash = `codex_smoke_token=${token}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.waitForTimeout(1_800);

const tableRows = page.locator('.taf-evidence-center .ant-table-tbody > tr:not(.ant-table-measure-row)');
await tableRows.first().waitFor({ state: 'visible', timeout: 15_000 });
const businessLayout = await page.evaluate(() => {
  const selectors = {
    titlebar: '.taf-encrypted-titlebar',
    title: '.taf-encrypted-titlebar h1',
    tabs: '.taf-encrypted-tabs',
    controls: '.taf-encrypted-controls',
    kpis: '.taf-encrypted-kpis--evidence',
    grid: '.taf-encrypted-grid',
    mainGrid: '.taf-evidence-main-grid',
    bottomGrid: '.taf-evidence-bottom-grid',
    rail: '.taf-encrypted-rail',
  };
  const rect = (element) => {
    const value = element.getBoundingClientRect();
    return { x: value.x, y: value.y, width: value.width, height: value.height };
  };
  const group = (selector) => Array.from(document.querySelectorAll(selector)).map(rect);
  return {
    device_pixel_ratio: window.devicePixelRatio,
    viewport: { width: window.innerWidth, height: window.innerHeight },
    regions: Object.fromEntries(Object.entries(selectors).map(([name, selector]) => {
      const element = document.querySelector(selector);
      return [name, element ? rect(element) : null];
    })),
    main_panels: group('.taf-evidence-main-grid > .taf-panel'),
    bottom_panels: group('.taf-evidence-bottom-grid > .taf-panel'),
    rail_panels: group('.taf-evidence-action-rail > .taf-panel'),
  };
});
const evidenceShellGeometry = await sharedShellGeometry(page);
const globalBusinessLayout = await globalBusinessOrigin(page);
const initialRowCount = await tableRows.count();
if (initialRowCount < 2) throw new Error('expected at least two evidence rows to verify a non-first quick-location target');
const sessionId = (await tableRows.nth(1).locator('td').nth(1).innerText()).trim();

const rangeStart = Date.now();
await page.locator('.taf-encrypted-controls .ant-select').first().click();
await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option').filter({ hasText: '近 7 天' }).click();
await page.waitForTimeout(1_800);
const rangeRequests = traffic.filter((item) => item.at >= rangeStart && item.method === 'GET');
const rangeParameters = rangeRequests.map((item) => {
  const url = new URL(item.url);
  return { path: url.pathname, start_time: Number(url.searchParams.get('start_time')), end_time: Number(url.searchParams.get('end_time')) };
});

await page.locator('.taf-evidence-locate-form input.ant-input').fill(sessionId);
const locateButton = page.locator('.taf-evidence-locate-form button');
await locateButton.scrollIntoViewIfNeeded();
await locateButton.evaluate((button) => button.click());
await page.waitForFunction((targetSessionId) => {
  const rows = Array.from(document.querySelectorAll('.taf-evidence-center .ant-table-tbody > tr:not(.ant-table-measure-row)'));
  return rows.length === 1 && rows[0]?.querySelector('td:nth-child(2)')?.textContent?.trim() === targetSessionId;
}, sessionId, { timeout: 5_000 });
const locatedRowCount = await tableRows.count();
const locatedSessionIds = await tableRows.locator('td:nth-child(2)').allInnerTexts();
const analysisStart = Date.now();
const analysisResponsePromise = page.waitForResponse(
  (response) => response.request().method() === 'POST' && response.url().includes('/encrypted-traffic/evidence-actions'),
  { timeout: 15_000 },
);
await page.getByRole('button', { name: '一键分析' }).click();
const analysisResponse = await analysisResponsePromise;
const analysisResponseBody = await analysisResponse.json();
await page.waitForTimeout(300);
const analysisRequests = traffic.filter((item) => item.at >= analysisStart && item.method === 'POST' && item.url.includes('/evidence-actions'));
const analysisSuccessToast = await page.locator('.ant-message-notice').filter({ hasText: '关联证据分析请求' }).count() > 0;
await page.locator('.taf-encrypted-tabs button').filter({ hasText: '总览' }).click();
await page.waitForTimeout(250);
const overviewShellGeometry = await sharedShellGeometry(page);
await page.locator('.taf-encrypted-tabs button').filter({ hasText: '证据中心' }).click();
await page.waitForTimeout(250);
const returnedEvidenceShellGeometry = await sharedShellGeometry(page);
await page.screenshot({ path: path.join(root, 'evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/interaction-r231.png'), fullPage: false });

const validRangeRequests = rangeParameters.filter((item) => item.start_time > 0 && item.end_time > item.start_time && item.end_time - item.start_time >= 7 * 24 * 60 * 60 * 1_000 - 10_000);
const actionResult = analysisResponseBody.data ?? analysisResponseBody;
const validAnalysisAction = analysisRequests.some((item) => {
  const payload = JSON.parse(item.post_data || '{}');
  return payload.action === 'associate_analysis' && payload.target === sessionId;
}) && analysisResponse.ok() && Boolean(actionResult.action_id) && actionResult.status === 'recorded';
const result = {
  result: validRangeRequests.length >= 6 && validAnalysisAction && analysisSuccessToast && globalBusinessLayout?.aligned_with_global_main && sameShellGeometry(evidenceShellGeometry, overviewShellGeometry) && sameShellGeometry(evidenceShellGeometry, returnedEvidenceShellGeometry) && initialRowCount > 1 && locatedRowCount === 1 && locatedSessionIds[0] === sessionId ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  route: redact(routeUrl.toString()),
  timestamp: new Date().toISOString(),
  business_layout: businessLayout,
  global_business_layout: globalBusinessLayout,
  shared_shell_geometry: {
    evidence: evidenceShellGeometry,
    overview: overviewShellGeometry,
    returned_evidence: returnedEvidenceShellGeometry,
    fixed_across_tabs: sameShellGeometry(evidenceShellGeometry, overviewShellGeometry) && sameShellGeometry(evidenceShellGeometry, returnedEvidenceShellGeometry),
  },
  initial_row_count: initialRowCount,
  located_row_count: locatedRowCount,
  located_session_ids: locatedSessionIds,
  range_requests: rangeParameters,
  valid_range_request_count: validRangeRequests.length,
  analysis_requests: analysisRequests,
  analysis_response: {
    status: analysisResponse.status(),
    action_id: actionResult.action_id ?? '',
    action: actionResult.action ?? '',
    audit_event: actionResult.audit_event ?? '',
    result_status: actionResult.status ?? '',
  },
  analysis_success_toast: analysisSuccessToast,
  bad_responses: badResponses,
  console_errors: consoleErrors,
  page_errors: pageErrors,
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
await page.close();
await browser.close();
console.log(JSON.stringify(result, null, 2));
if (result.result !== 'pass') process.exit(1);

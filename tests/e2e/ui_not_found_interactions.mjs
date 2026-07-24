#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const baseUrl = process.env.NOT_FOUND_BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.NOT_FOUND_CDP_URL || 'http://127.0.0.1:9224';
const revision = process.env.NOT_FOUND_REVISION || 'r512';
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/not-found');
const screenshotPath = path.join(evidenceDir, `actual-${revision}-main-1920.png`);
const contactScreenshotPath = path.join(evidenceDir, `actual-${revision}-contact-1920.png`);
const failureScreenshotPath = path.join(evidenceDir, `actual-${revision}-failure-1920.png`);
const reportPath = path.join(evidenceDir, `interaction-${revision}.json`);
const targetPath = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/not-found.png');
const diffPath = path.join(evidenceDir, `compare-${revision}-main.png`);
const metricsPath = path.join(evidenceDir, `metrics-${revision}-main.json`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
fs.mkdirSync(evidenceDir, { recursive: true });

function token(permissions = ['*', 'admin:*', 'audit:read', 'alert:read', 'screen:view']) {
  const encoded = execFileSync('kubectl', [
    '--server=https://127.0.0.1:6443', '--tls-server-name=10.0.5.8', '-n', 'traffic-analysis',
    'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}',
  ], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1000);
  const userId = crypto.randomUUID();
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: userId, jti: crypto.randomUUID(), user_id: userId,
    tenant_id: 'default', username: 'codex-not-found-windows-cdp', roles: ['admin'],
    permissions, token_type: 'access',
    session_id: `not-found-${revision}`, iat: now, exp: now + 1800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

function safeUrl(value) {
  return String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
}

function targetUrl(smokeToken) {
  const url = new URL(`/__codex_visual_not_found__?windowsCdpInteractionTs=${Date.now()}`, baseUrl);
  url.hash = `codex_smoke_token=${smokeToken}`;
  return url;
}

async function nativeClick(page, locator, name) {
  await locator.waitFor({ state: 'visible' });
  await locator.scrollIntoViewIfNeeded();
  const box = await locator.boundingBox();
  if (!box) throw new Error(`${name} has no hitbox`);
  const center = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
  const hit = await locator.evaluate((element, point) => {
    const top = document.elementFromPoint(point.x, point.y);
    return { center_hits_control: top === element || element.contains(top), disabled: 'disabled' in element && Boolean(element.disabled), top_tag: top?.tagName ?? null };
  }, center);
  if (!hit.center_hits_control || hit.disabled) throw new Error(`${name} is not natively actionable: ${JSON.stringify(hit)}`);
  await page.mouse.click(center.x, center.y);
  return { name, box, ...hit };
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
const listResponse = await fetch(`${cdpUrl}/json/list`);
if (!versionResponse.ok || !listResponse.ok) throw new Error('Windows Chrome CDP tunnel 9224 is unavailable');
const version = await versionResponse.json();
if (!String(version['User-Agent'] || '').includes('Windows')) throw new Error(`Expected Windows Chrome, received ${version['User-Agent'] || 'unknown'}`);
const targets = await listResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(baseUrl).origin }).catch(() => {});
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
const cdp = await context.newCDPSession(page);
await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
await cdp.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });

const runtime = { bad_responses: [], request_failures: [], console_errors: [], page_errors: [], ignored_extension_errors: [] };
page.on('response', (response) => {
  if (response.status() >= 400 && response.url().startsWith(baseUrl)) runtime.bad_responses.push({ status: response.status(), method: response.request().method(), url: safeUrl(response.url()) });
});
page.on('requestfailed', (request) => {
  const item = { method: request.method(), url: safeUrl(request.url()), error: request.failure()?.errorText ?? '' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com/ip')) runtime.ignored_extension_errors.push(item);
  else runtime.request_failures.push(item);
});
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const item = { text: entry.text(), url: entry.location().url || '' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com/ip') || item.text.includes('ERR_CONNECTION_CLOSED')) runtime.ignored_extension_errors.push(item);
  else runtime.console_errors.push(item);
});
page.on('pageerror', (error) => {
  if (error.message === 'Object' || error.message.includes("reading 'disconnect'")) runtime.ignored_extension_errors.push({ message: error.message });
  else runtime.page_errors.push({ message: error.message });
});

const smokeToken = token();
const navigationMissResponses = [];
page.on('response', async (response) => {
  if (!response.url().includes('/api/v1/auth/navigation-miss')) return;
  navigationMissResponses.push({ status: response.status(), method: response.request().method(), body: await response.json().catch(() => null) });
});

await page.goto(targetUrl(smokeToken).toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-notfound').waitFor({ state: 'visible', timeout: 20_000 });
await page.waitForFunction(() => /^[0-9a-f-]{36}$/i.test(document.querySelector('[data-notfound-fact="trace-id"]')?.textContent?.trim() || ''), null, { timeout: 20_000 });
await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});
await page.waitForTimeout(600);
await page.screenshot({ path: screenshotPath, fullPage: false });

const checks = [];
const pointerActions = [];
const initial = await page.evaluate(() => {
  const rect = (selector) => {
    const element = document.querySelector(selector);
    if (!element) return null;
    const box = element.getBoundingClientRect();
    return { x: box.x, y: box.y, width: box.width, height: box.height, right: box.right, bottom: box.bottom };
  };
  const root = document.scrollingElement || document.documentElement;
  const body = document.body.innerText;
  return {
    url: location.href.replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>'),
    token_consumed: !location.href.includes('codex_smoke_token'),
    trace_id: document.querySelector('[data-notfound-fact="trace-id"]')?.textContent?.trim() || '',
    no_raw_path: !body.includes('/__codex_visual_not_found__'),
    page: rect('.taf-notfound'), main: rect('.taf-notfound__main'), rail: rect('.taf-notfound__rail'),
    horizontal_overflow: root.scrollWidth > root.clientWidth + 2,
    vertical_overflow: root.scrollHeight > root.clientHeight + 2,
    action_count: document.querySelectorAll('[data-notfound-action], [data-notfound-entry]').length,
    status_values: Array.from(document.querySelectorAll('.taf-notfound__status-row strong')).map((item) => item.textContent?.trim()),
  };
});
checks.push({ name: 'smoke token is consumed', passed: initial.token_consumed });
checks.push({ name: 'opaque runtime trace replaces hardcoded trace', passed: /^[0-9a-f-]{36}$/i.test(initial.trace_id) && initial.trace_id !== 'trace-20260621-8F3A' });
checks.push({ name: 'raw unknown path is not exposed', passed: initial.no_raw_path });
checks.push({ name: 'page stays inside 1920x1080 viewport', passed: !initial.horizontal_overflow && !initial.vertical_overflow && initial.page?.bottom <= 1080 });
checks.push({ name: 'four live status values are healthy', passed: initial.status_values.length === 4 && initial.status_values.every((value) => value === '正常') });
checks.push({ name: 'navigation miss API returns persisted database context', passed: navigationMissResponses.some((item) => item.status === 201 && item.body?.persisted === true && item.body?.audit_action === 'navigation_not_found') });

pointerActions.push(await nativeClick(page, page.locator('[data-notfound-action="contact-admin"]'), '联系管理员'));
await page.locator('[data-notfound-support-request]').waitFor({ state: 'visible', timeout: 20_000 });
await page.screenshot({ path: contactScreenshotPath, fullPage: false });
const contact = await page.evaluate(() => {
  const panel = document.querySelector('[data-notfound-contact-panel]');
  const main = document.querySelector('.taf-notfound__main');
  const support = document.querySelector('[data-notfound-support-request]');
  const panelBox = panel?.getBoundingClientRect();
  const mainBox = main?.getBoundingClientRect();
  return {
    request_id: support?.getAttribute('data-notfound-support-request') || '',
    panel_inside_business_region: Boolean(panelBox && mainBox && panelBox.left >= mainBox.left && panelBox.top >= mainBox.top && panelBox.right <= mainBox.right && panelBox.bottom <= mainBox.bottom),
  };
});
checks.push({ name: 'contact administrator persists a support request', passed: /^support-[0-9a-f-]{36}$/i.test(contact.request_id) && navigationMissResponses.some((item) => item.status === 201 && item.body?.persisted === true && item.body?.audit_action === 'navigation_support_requested') });
checks.push({ name: 'contact interface stays inside the business region', passed: contact.panel_inside_business_region });
pointerActions.push(await nativeClick(page, page.locator('[data-notfound-action="close-contact"]'), '关闭管理员联系界面'));
await page.locator('[data-notfound-contact-panel]').waitFor({ state: 'detached' });

await page.evaluate(() => {
  window.__notFoundCopiedValue = '';
  document.addEventListener('copy', (event) => {
    window.__notFoundCopiedValue = event.clipboardData?.getData('text/plain') || document.activeElement?.value || window.getSelection()?.toString() || '';
  }, { once: true });
});
pointerActions.push(await nativeClick(page, page.locator('[data-notfound-action="copy-trace"]'), '复制追踪 ID'));
await page.getByText('追踪 ID 已复制').waitFor({ state: 'visible' });
let clipboard = await page.evaluate(async () => {
  try {
    return await navigator.clipboard?.readText();
  } catch {
    return window.__notFoundCopiedValue || '';
  }
});
if (!clipboard) {
  await page.evaluate(() => {
    const probe = document.createElement('textarea');
    probe.id = 'taf-notfound-clipboard-probe';
    probe.style.position = 'fixed';
    probe.style.left = '2px';
    probe.style.top = '2px';
    document.body.appendChild(probe);
    probe.focus();
  });
  await page.keyboard.press('Control+V');
  clipboard = await page.locator('#taf-notfound-clipboard-probe').inputValue();
  await page.locator('#taf-notfound-clipboard-probe').evaluate((element) => element.remove());
}
checks.push({ name: 'copy trace writes the live trace id', passed: clipboard === initial.trace_id });

async function verifyNavigation(selector, expectedPath, name) {
  pointerActions.push(await nativeClick(page, page.locator(selector), name));
  await page.waitForURL((url) => url.pathname === expectedPath, { timeout: 20_000 });
  await page.waitForFunction(() => !document.querySelector('.taf-notfound') && !document.querySelector('.taf-access-denied'), null, { timeout: 20_000 });
  checks.push({ name: `${name} renders authorized business route ${expectedPath}`, passed: new URL(page.url()).pathname === expectedPath && !(await page.locator('.taf-access-denied').count()) });
  await page.goBack({ waitUntil: 'domcontentloaded' });
  await page.locator('.taf-notfound').waitFor({ state: 'visible', timeout: 20_000 });
}

await verifyNavigation('[data-notfound-action="return-dashboard"]', '/dashboard', '返回仪表盘');
await verifyNavigation('[data-notfound-action="return-screen"]', '/screen', '返回态势大屏');
await verifyNavigation('[data-notfound-action="return-alerts"]', '/alerts', '返回告警中心');
await verifyNavigation('[data-notfound-action="audit-log"]', '/audit-log', '查看审计日志');
for (const [id, expectedPath, name] of [
  ['dashboard', '/dashboard', '最近入口仪表盘'], ['screen', '/screen', '最近入口态势大屏'], ['alerts', '/alerts', '最近入口告警中心'], ['audit-log', '/audit-log', '最近入口审计日志'],
]) await verifyNavigation(`[data-notfound-entry="${id}"]`, expectedPath, name);

await page.goto(new URL('/dashboard', baseUrl).toString(), { waitUntil: 'domcontentloaded' });
await page.goto(new URL(`/__codex_visual_not_found__?previousTs=${Date.now()}`, baseUrl).toString(), { waitUntil: 'domcontentloaded' });
await page.locator('.taf-notfound').waitFor({ state: 'visible', timeout: 20_000 });
pointerActions.push(await nativeClick(page, page.locator('[data-notfound-action="previous"]'), '返回上一页'));
await page.waitForURL((url) => url.pathname === '/dashboard', { timeout: 20_000 });
checks.push({ name: 'return previous uses real browser history', passed: new URL(page.url()).pathname === '/dashboard' });

const failurePage = await context.newPage();
await failurePage.setViewportSize({ width: 1920, height: 1080 });
await failurePage.route('**/api/v1/auth/navigation-miss', async (route) => {
  if (new URL(route.request().url()).pathname === '/api/v1/auth/navigation-miss') {
    await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'TEMPORARY_UNAVAILABLE', message: 'planned not-found failure-state verification' }) });
  } else await route.continue();
});
await failurePage.goto(new URL(`/__codex_visual_not_found_failure__?failureTs=${Date.now()}`, baseUrl).toString(), { waitUntil: 'domcontentloaded' });
await failurePage.locator('[data-notfound-state="context-error"]').waitFor({ state: 'visible', timeout: 20_000 });
await failurePage.screenshot({ path: failureScreenshotPath, fullPage: false });
const failureContained = await failurePage.evaluate(() => {
  const state = document.querySelector('[data-notfound-state="context-error"]')?.getBoundingClientRect();
  const main = document.querySelector('.taf-notfound__main')?.getBoundingClientRect();
  const root = document.scrollingElement || document.documentElement;
  return Boolean(state && main && state.left >= main.left && state.top >= main.top && state.right <= main.right && state.bottom <= main.bottom && root.scrollWidth <= root.clientWidth + 2 && root.scrollHeight <= root.clientHeight + 2);
});
checks.push({ name: 'failed audit request renders explicit contained error state', passed: failureContained });
await failurePage.unroute('**/api/v1/auth/navigation-miss');
await nativeClick(failurePage, failurePage.locator('[data-notfound-action="retry-context"]'), '失败态重试');
await failurePage.waitForFunction(() => /^[0-9a-f-]{36}$/i.test(document.querySelector('[data-notfound-fact="trace-id"]')?.textContent?.trim() || ''), null, { timeout: 20_000 });
checks.push({ name: 'failed audit request recovers through retry', passed: !(await failurePage.locator('[data-notfound-state="context-error"]').count()) });
await failurePage.close();

const lowPermissionPage = await context.newPage();
await lowPermissionPage.setViewportSize({ width: 1920, height: 1080 });
await lowPermissionPage.goto(targetUrl(token(['alert:read'])).toString(), { waitUntil: 'domcontentloaded' });
await lowPermissionPage.locator('.taf-notfound').waitFor({ state: 'visible', timeout: 20_000 });
await lowPermissionPage.waitForFunction(() => /^[0-9a-f-]{36}$/i.test(document.querySelector('[data-notfound-fact="trace-id"]')?.textContent?.trim() || ''), null, { timeout: 20_000 });
const lowPermissionActions = await lowPermissionPage.locator('[data-notfound-action], [data-notfound-entry]').evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-notfound-action') || `entry:${node.getAttribute('data-notfound-entry')}`));
checks.push({ name: 'recent entries are filtered by current-user route permissions', passed: lowPermissionActions.includes('return-dashboard') && lowPermissionActions.includes('return-alerts') && !lowPermissionActions.includes('return-screen') && !lowPermissionActions.includes('audit-log') && !lowPermissionActions.includes('entry:screen') && !lowPermissionActions.includes('entry:audit-log') });
await lowPermissionPage.close();

const diff = spawnSync('python3', [
  'tests/e2e/ui_visual_diff_metrics.py', '--target-id', 'not-found', '--route', '/__codex_visual_not_found__',
  '--source', targetPath, '--actual', screenshotPath, '--diff', diffPath, '--metrics', metricsPath,
  '--max-pixel-ratio', '0.125', '--channel-tolerance', '64', '--desktop-status', 'pass',
], { cwd: root, encoding: 'utf8' });
const metrics = JSON.parse(fs.readFileSync(metricsPath, 'utf8'));
checks.push({ name: 'visual mismatch stays under page threshold', passed: diff.status === 0 });

const businessErrors = runtime.bad_responses.length + runtime.request_failures.length + runtime.console_errors.length + runtime.page_errors.length;
checks.push({ name: 'not-found flow has no business runtime errors', passed: businessErrors === 0 });
const passed = checks.filter((item) => item.passed).length;
const report = {
  schema_version: 1, result: passed === checks.length ? 'pass' : 'fail', revision, generated_at: new Date().toISOString(),
  browser: version.Browser, user_agent: version['User-Agent'], cdp_url: cdpUrl, cdp_targets: targets.length,
  base_url: baseUrl, viewport: { width: 1920, height: 1080 }, initial,
  screenshot: path.relative(root, screenshotPath), contact_screenshot: path.relative(root, contactScreenshotPath), failure_screenshot: path.relative(root, failureScreenshotPath), diff: path.relative(root, diffPath), metrics: path.relative(root, metricsPath),
  visual_diff: metrics.visual_diff, checks, passed, total: checks.length, pointer_actions: pointerActions,
  navigation_miss_responses: navigationMissResponses.map((item) => ({ status: item.status, method: item.method, persisted: item.body?.persisted, audit_action: item.body?.audit_action })),
  runtime,
};
fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify({ result: report.result, checks: `${passed}/${checks.length}`, native_clicks: pointerActions.length, mismatch: metrics.visual_diff?.pixel_mismatch_ratio, report: path.relative(root, reportPath) }, null, 2));
await page.close();
await browser.close();
process.exit(report.result === 'pass' ? 0 : 1);

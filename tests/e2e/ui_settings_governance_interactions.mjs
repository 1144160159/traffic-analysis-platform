#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const baseUrl = process.env.SETTINGS_UI_BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.SETTINGS_UI_CDP_URL || 'http://127.0.0.1:9224';
const runId = process.env.SETTINGS_UI_RUN_ID || 'r489';
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/settings');
const outputPath = path.join(evidenceDir, `interaction-${runId}.json`);
const screenshots = Object.fromEntries(['main', 'site', 'integration', 'impact', 'token-created', 'token-scopes'].map((state) => [state, path.join(evidenceDir, `actual-${runId}-${state}-1920.png`)]));

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
fs.mkdirSync(evidenceDir, { recursive: true });

function smokeToken(permissions = ['*', 'admin:*', 'admin:read', 'admin:write', 'token:read', 'token:write']) {
  const secret = execFileSync('kubectl', [
    '--server=https://127.0.0.1:6443', '--tls-server-name=10.0.5.8', '-n', 'traffic-analysis',
    'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}',
  ], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(),
    tenant_id: 'default', username: 'codex-windows-cdp-settings-admin', roles: ['admin'],
    permissions, token_type: 'access',
    session_id: `windows-cdp-settings-${runId}`, iat: now, exp: now + 1800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(secret, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

const sameOrInside = (child, parent, tolerance = 2) => Boolean(child && parent)
  && child.x >= parent.x - tolerance && child.y >= parent.y - tolerance
  && child.x + child.width <= parent.x + parent.width + tolerance
  && child.y + child.height <= parent.y + parent.height + tolerance;

async function nativeClick(page, locator, name) {
  await locator.scrollIntoViewIfNeeded();
  const box = await locator.boundingBox();
  if (!box) throw new Error(`${name} has no hitbox`);
  const point = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
  const hit = await locator.evaluate((button, center) => {
    const top = document.elementFromPoint(center.x, center.y);
    return { disabled: button.disabled, center_hits_button: top === button || button.contains(top), top_tag: top?.tagName ?? null };
  }, point);
  if (hit.disabled || !hit.center_hits_button) throw new Error(`${name} is not natively actionable: ${JSON.stringify(hit)}`);
  await page.mouse.click(point.x, point.y);
  return { name, box, ...hit };
}

async function capture(page, state) {
  await page.screenshot({ path: screenshots[state], fullPage: false });
  if (fs.statSync(screenshots[state]).size < 10_000) throw new Error(`${state} screenshot unexpectedly small`);
}

async function closeDrawer(page) {
  const drawer = page.locator('.taf-settings-detail-drawer .ant-drawer-content-wrapper:visible');
  await drawer.locator('.ant-drawer-close').click();
  await drawer.waitFor({ state: 'hidden' });
}

async function drawerEvidence(page, state, expectedText) {
  const pageRegion = page.locator('.taf-settings-page');
  const drawer = page.locator('.taf-settings-detail-drawer .ant-drawer-content-wrapper:visible');
  await drawer.waitFor({ state: 'visible' });
  await page.waitForTimeout(450);
  if (expectedText) await drawer.getByText(expectedText, { exact: false }).first().waitFor({ state: 'visible' });
  const [pageBox, drawerBox] = await Promise.all([pageRegion.boundingBox(), drawer.boundingBox()]);
  const layout = await drawer.evaluate((element) => ({ client_width: element.clientWidth, scroll_width: element.scrollWidth, horizontal_overflow: element.scrollWidth > element.clientWidth + 2 }));
  await capture(page, state);
  return { page_box: pageBox, drawer_box: drawerBox, contained_in_page_region: sameOrInside(drawerBox, pageBox), ...layout };
}

function responseFor(page, method, suffix, accepted = [200]) {
  return page.waitForResponse((response) => {
    const url = new URL(response.url());
    return response.request().method() === method && url.pathname.endsWith(suffix) && accepted.includes(response.status());
  }, { timeout: 30_000 });
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP unavailable: ${versionResponse.status}`);
const version = await versionResponse.json();
const restrictedChecks = [];
const tokenReaderResponse = await fetch(new URL('/api/v1/auth/system-settings', baseUrl), { headers: { Authorization: `Bearer ${smokeToken(['token:read'])}` } });
restrictedChecks.push({ name: 'token:read cannot read system settings', passed: tokenReaderResponse.status === 403, status: tokenReaderResponse.status });
const adminReaderResponse = await fetch(new URL('/api/v1/auth/system-settings', baseUrl), { headers: { Authorization: `Bearer ${smokeToken(['admin:read'])}` } });
restrictedChecks.push({ name: 'admin:read can read system settings', passed: adminReaderResponse.status === 200, status: adminReaderResponse.status });
const delegatedAdminResponse = await fetch(new URL('/api/v1/tokens', baseUrl), {
  method: 'POST',
  headers: { Authorization: `Bearer ${smokeToken(['token:write'])}`, 'Content-Type': 'application/json' },
  body: JSON.stringify({ name: `codex-forbidden-delegation-${runId}`, scopes: ['admin:*'], expires_in_sec: 300 }),
});
restrictedChecks.push({ name: 'token:write cannot delegate admin:*', passed: delegatedAdminResponse.status === 403, status: delegatedAdminResponse.status });
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();

for (const routeCase of [
  { name: 'token:read is denied by settings route', permissions: ['token:read'], allowed: false },
  { name: 'admin:read enters read-only settings route', permissions: ['admin:read'], allowed: true },
  { name: 'admin:* enters settings but cannot manage tokens', permissions: ['admin:*'], allowed: true, adminWildcard: true },
]) {
  const routePage = await context.newPage();
  await routePage.setViewportSize({ width: 1920, height: 1080 });
  const routeCaseUrl = new URL(`/settings?windowsCdpRouteScopeTs=${Date.now()}`, baseUrl);
  routeCaseUrl.hash = `codex_smoke_token=${smokeToken(routeCase.permissions)}`;
  await routePage.goto(routeCaseUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await routePage.waitForLoadState('networkidle', { timeout: 20_000 }).catch(() => {});
  if (routeCase.allowed) {
    await routePage.locator('.taf-settings-page').waitFor({ state: 'visible', timeout: 20_000 });
    await routePage.waitForFunction(() => document.querySelector('.taf-settings-titlebar small')?.textContent?.includes('revision'));
    const controls = await routePage.locator('[data-settings-action="save"], [data-settings-action="create-token"], [data-settings-action="impact"]').evaluateAll((items) => Object.fromEntries(items.map((item) => [item.getAttribute('data-settings-action'), item.disabled])));
    const passed = routeCase.adminWildcard
      ? controls.save === false && controls['create-token'] === true && controls.impact === false
      : controls.save === true && controls['create-token'] === true && controls.impact === false;
    restrictedChecks.push({ name: routeCase.name, passed, controls });
  } else {
    await routePage.getByText('403', { exact: true }).waitFor({ state: 'visible', timeout: 20_000 });
    restrictedChecks.push({ name: routeCase.name, passed: await routePage.locator('.taf-settings-page').count() === 0 });
  }
  await routePage.close();
}

const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
page.setDefaultTimeout(15_000);

const apiResponses = [];
const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
const ignoredExternalFailures = [];
page.on('response', (response) => {
  const item = { method: response.request().method(), status: response.status(), url: response.url().replace(/#[^#]+$/, '#<redacted>') };
  if (item.url.includes('/api/v1/auth/system-settings') || item.url.includes('/api/v1/tokens')) apiResponses.push(item);
  if (item.status >= 400 && item.url.startsWith(baseUrl)) badResponses.push(item);
});
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const item = { text: entry.text(), url: entry.location().url || '' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com/ip') || item.text.includes('ERR_CONNECTION_CLOSED')) ignoredExternalFailures.push(item);
  else consoleErrors.push(item);
});
page.on('pageerror', (error) => {
  if (error.message === 'Object') ignoredExternalFailures.push({ text: error.message, url: 'chrome-extension://isolated-world' });
  else pageErrors.push({ message: error.message, stack: error.stack || '' });
});
page.on('requestfailed', (request) => {
  const item = { method: request.method(), url: request.url(), error: request.failure()?.errorText ?? '' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com/ip')) ignoredExternalFailures.push(item);
  else requestFailures.push(item);
});

const routeUrl = new URL(`/settings?windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 20_000 }).catch(() => {});
const settingsPage = page.locator('.taf-settings-page');
await settingsPage.waitFor({ state: 'visible', timeout: 20_000 });
await page.getByRole('heading', { name: '系统设置', exact: true }).waitFor();
await page.waitForFunction(() => document.querySelector('.taf-settings-titlebar small')?.textContent?.includes('revision'));
await capture(page, 'main');

const checks = [];
checks.push(...restrictedChecks);
const pointerActions = [];
const drawers = {};
const initialUrl = page.url();

const siteButton = page.locator('.taf-settings-tenant-tree button').first();
pointerActions.push(await nativeClick(page, siteButton, '租户与站点首行'));
drawers.site = await drawerEvidence(page, 'site', '修改保存在当前草稿中');
checks.push({ name: 'site drawer stays in business region', passed: drawers.site.contained_in_page_region && !drawers.site.horizontal_overflow });
await closeDrawer(page);

const integrationButton = page.locator('.taf-settings-integrations button').first();
pointerActions.push(await nativeClick(page, integrationButton, '集成配置首行'));
drawers.integration = await drawerEvidence(page, 'integration', '敏感信息只允许填写');
const integrationResponsePromise = responseFor(page, 'POST', '/api/v1/auth/system-settings/actions/test-integration');
pointerActions.push(await nativeClick(page, page.locator('.taf-settings-detail-drawer button:has-text("测试此连接"):visible'), '测试此连接'));
const integrationResponse = await integrationResponsePromise;
await page.locator('.taf-settings-detail-drawer .ant-drawer-content-wrapper:visible').getByText('连接测试已完成', { exact: false }).waitFor();
checks.push({ name: 'integration test action', passed: integrationResponse.status() === 200 });
await closeDrawer(page);

const impactResponsePromise = responseFor(page, 'GET', '/api/v1/auth/system-settings/impact');
pointerActions.push(await nativeClick(page, page.locator('[data-settings-action="impact"]'), '查看影响范围'));
const impactResponse = await impactResponsePromise;
drawers.impact = await drawerEvidence(page, 'impact', '影响范围');
checks.push({ name: 'impact drawer stays in business region', passed: impactResponse.status() === 200 && drawers.impact.contained_in_page_region && !drawers.impact.horizontal_overflow });
await closeDrawer(page);

for (const [selector, name, suffix] of [
  ['[data-settings-action="save"]', '保存配置', '/api/v1/auth/system-settings'],
  ['[data-settings-action="connection-test"]', '连接测试', '/api/v1/auth/system-settings/actions/connection-test'],
  ['[data-settings-action="security-audit"]', '触发安全审计', '/api/v1/auth/system-settings/actions/security-audit'],
]) {
  const method = name === '保存配置' ? 'PUT' : 'POST';
  const responsePromise = responseFor(page, method, suffix);
  pointerActions.push(await nativeClick(page, page.locator(selector), name));
  const response = await responsePromise;
  checks.push({ name: `${name} reaches live API`, passed: response.status() === 200, status: response.status() });
  if (name !== '保存配置') await closeDrawer(page);
}

const reloadResponses = Promise.all([
  responseFor(page, 'GET', '/api/v1/auth/system-settings'),
  responseFor(page, 'GET', '/api/v1/tokens/scopes'),
]);
pointerActions.push(await nativeClick(page, page.getByRole('button', { name: '刷新系统设置', exact: true }), '刷新系统设置'));
await reloadResponses;
checks.push({ name: 'refresh reloads workbench and token scopes', passed: true });

pointerActions.push(await nativeClick(page, page.locator('[data-settings-action="create-token"]'), '创建令牌'));
const modal = page.locator('.ant-modal-content:visible');
await modal.waitFor();
await modal.getByLabel('令牌名称').fill(`codex-settings-ui-${runId}-${Date.now()}`);
await modal.getByLabel('用途说明').fill('Windows Chrome settings interaction evidence');
await capture(page, 'token-created');
const createResponsePromise = responseFor(page, 'POST', '/api/v1/tokens', [201]);
pointerActions.push(await nativeClick(page, modal.getByRole('button', { name: '创建并显示一次', exact: true }), '创建并显示一次'));
const createResponse = await createResponsePromise;
const created = await createResponse.json();
await page.locator('.taf-settings-detail-drawer .ant-drawer-content-wrapper:visible').getByText('一次性令牌明文', { exact: true }).waitFor();
drawers.tokenCreated = await drawerEvidence(page, 'token-created', '一次性令牌明文');
checks.push({ name: 'create token and one-time secret drawer', passed: createResponse.status() === 201 && Boolean(created.token_id) && drawers.tokenCreated.contained_in_page_region });
await closeDrawer(page);

const createdTokenRefresh = responseFor(page, 'GET', '/api/v1/tokens');
pointerActions.push(await nativeClick(page, page.getByRole('button', { name: '刷新系统设置', exact: true }), '创建令牌后刷新'));
await createdTokenRefresh;
await page.waitForFunction((tokenName) => [...document.querySelectorAll('.taf-settings-token-panel tbody tr')].some((row) => row.textContent?.includes(tokenName)), created.name, { timeout: 20_000 });
const activeRow = page.locator('.taf-settings-token-panel tbody tr').filter({ hasText: created.name }).first();
await activeRow.click();
const permissionButton = activeRow.getByRole('button', { name: '权限', exact: true });
pointerActions.push(await nativeClick(page, permissionButton, '令牌权限'));
drawers.tokenScopes = await drawerEvidence(page, 'token-scopes', 'API 令牌权限配置');
const scopesResponsePromise = responseFor(page, 'PUT', `/api/v1/tokens/${created.token_id}/scopes`);
pointerActions.push(await nativeClick(page, page.locator('.taf-settings-detail-drawer button:has-text("保存令牌权限"):visible'), '保存令牌权限'));
pointerActions.push(await nativeClick(page, page.locator('.ant-popconfirm:visible button:has-text("确认更新")'), '确认更新令牌权限'));
const scopesResponse = await scopesResponsePromise;
checks.push({ name: 'token scopes persisted', passed: scopesResponse.status() === 200 && drawers.tokenScopes.contained_in_page_region });

await page.waitForFunction((tokenName) => [...document.querySelectorAll('.taf-settings-token-panel tbody tr')].some((row) => row.textContent?.includes(tokenName)), created.name, { timeout: 20_000 });
const selectedRow = page.locator('.taf-settings-token-panel tbody tr').filter({ hasText: created.name }).first();
await selectedRow.click();
const rotateResponsePromise = responseFor(page, 'POST', `/api/v1/tokens/${created.token_id}/regenerate`, [201]);
pointerActions.push(await nativeClick(page, page.locator('[data-settings-action="rotate-token"]'), '轮换令牌'));
pointerActions.push(await nativeClick(page, page.locator('.ant-popconfirm:visible button:has-text("确认轮换")'), '确认轮换令牌'));
const rotateResponse = await rotateResponsePromise;
const regenerated = await rotateResponse.json();
await page.locator('.taf-settings-detail-drawer .ant-drawer-content-wrapper:visible').getByText('一次性令牌明文', { exact: true }).waitFor();
checks.push({ name: 'selected token rotates through top action', passed: rotateResponse.status() === 201 && Boolean(regenerated.token_id) });
await closeDrawer(page);

await page.waitForFunction((tokenName) => [...document.querySelectorAll('.taf-settings-token-panel tbody tr')].some((row) => row.textContent?.includes(tokenName)), regenerated.name, { timeout: 20_000 });
const regeneratedRow = page.locator('.taf-settings-token-panel tbody tr').filter({ hasText: regenerated.name }).first();
await regeneratedRow.click();
const revokeButton = regeneratedRow.getByRole('button', { name: '吊销', exact: true });
pointerActions.push(await nativeClick(page, revokeButton, '吊销令牌'));
const revokeResponsePromise = responseFor(page, 'POST', `/api/v1/tokens/${regenerated.token_id}/revoke`);
pointerActions.push(await nativeClick(page, page.locator('.ant-popconfirm:visible button:has-text("确认吊销")'), '确认吊销'));
const revokeResponse = await revokeResponsePromise;
checks.push({ name: 'regenerated token revoked for cleanup', passed: revokeResponse.status() === 200 });

let staleEvidenceTokensRevoked = 0;
for (let attempt = 0; attempt < 10; attempt += 1) {
  const rowsNow = page.locator('.taf-settings-token-panel tbody tr');
  let revokedOne = false;
  for (let index = 0; index < await rowsNow.count(); index += 1) {
    const row = rowsNow.nth(index);
    const rowText = await row.textContent();
    if (!rowText?.includes('codex-settings-ui-')) continue;
    const button = row.getByRole('button', { name: '吊销', exact: true });
    if (!await button.count() || await button.isDisabled()) continue;
    await nativeClick(page, button, `清理失败运行令牌 ${index + 1}`);
    const cleanupResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname.endsWith('/revoke') && response.status() === 200, { timeout: 30_000 });
    await nativeClick(page, page.locator('.ant-popconfirm:visible button:has-text("确认吊销")'), `确认清理失败运行令牌 ${index + 1}`);
    await cleanupResponsePromise;
    staleEvidenceTokensRevoked += 1;
    revokedOne = true;
    break;
  }
  if (!revokedOne) break;
  await page.waitForTimeout(500);
}
const apiCleanup = await page.evaluate(async () => {
  const token = window.localStorage.getItem('traffic-ui-token');
  if (!token) return { listed: false, revoked: [], failed: ['missing browser auth token'] };
  const headers = { Authorization: `Bearer ${token}` };
  const listResponse = await fetch('/api/v1/tokens?limit=100&offset=0', { headers });
  if (!listResponse.ok) return { listed: false, revoked: [], failed: [`list:${listResponse.status}`] };
  const payload = await listResponse.json();
  const stale = (payload.tokens ?? []).filter((item) => item.status === 'active' && String(item.name ?? '').startsWith('codex-settings-ui-'));
  const revoked = [];
  const failed = [];
  for (const item of stale) {
    const response = await fetch(`/api/v1/tokens/${encodeURIComponent(item.token_id)}/revoke`, { method: 'POST', headers });
    if (response.ok) revoked.push(item.token_id);
    else failed.push(`${item.token_id}:${response.status}`);
  }
  return { listed: true, revoked, failed };
});
checks.push({
  name: 'failed-run evidence tokens cleaned through audited revoke API',
  passed: apiCleanup.listed && apiCleanup.failed.length === 0,
  ui_revoked: staleEvidenceTokensRevoked,
  api_revoked: apiCleanup.revoked.length,
});

const pageBox = await settingsPage.boundingBox();
const workbenchOverflow = await page.locator('.taf-settings-workbench').evaluate((element) => ({ client_width: element.clientWidth, scroll_width: element.scrollWidth, horizontal_overflow: element.scrollWidth > element.clientWidth + 2 }));
checks.push({ name: 'route and page region remain stable', passed: page.url() === initialUrl && Boolean(pageBox) && !workbenchOverflow.horizontal_overflow });
checks.push({ name: 'no application runtime errors', passed: badResponses.length === 0 && consoleErrors.length === 0 && pageErrors.length === 0 && requestFailures.length === 0 });

const result = {
  run_id: runId,
  result: checks.every((check) => check.passed) ? 'pass' : 'fail',
  browser: version.Browser,
  protocol_version: version['Protocol-Version'],
  route: page.url().replace(/#[^#]+$/, '#<redacted>'),
  viewport: page.viewportSize(),
  device_pixel_ratio: await page.evaluate(() => window.devicePixelRatio),
  page_box: pageBox,
  workbench_layout: workbenchOverflow,
  checks,
  pointer_actions: pointerActions,
  drawers,
  api_responses: apiResponses,
  bad_responses: badResponses,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  request_failures: requestFailures,
  ignored_external_failures: ignoredExternalFailures,
  created_token_id: created.token_id,
  regenerated_token_id: regenerated.token_id,
  screenshots,
};

fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
await page.close();
await browser.close();
console.log(JSON.stringify({ output: outputPath, result: result.result, passed: checks.filter((item) => item.passed).length, total: checks.length }));
if (result.result !== 'pass') process.exitCode = 1;

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
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/rules');
const outputPath = path.join(evidenceDir, 'interaction-r254-normal-api.json');
const screenshotPath = path.join(evidenceDir, 'interaction-r254-normal-api.png');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
fs.mkdirSync(evidenceDir, { recursive: true });

const encodedSecret = execFileSync(
  'kubectl',
  ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
  { encoding: 'utf8', env: process.env, timeout: 15_000 },
);
const jwtSecret = Buffer.from(encodedSecret, 'base64').toString('utf8');

function accessToken({ tenantId = 'default', permissions, roles }) {
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: tenantId,
    username: `codex-rules-${roles[0]}`,
    roles,
    permissions,
    token_type: 'access',
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', jwtSecret).update(input).digest('base64url')}`;
}

async function api(pathname, token, init = {}) {
  const response = await fetch(`${baseUrl}${pathname}`, {
    ...init,
    headers: {
      authorization: `Bearer ${token}`,
      'content-type': 'application/json',
      ...(init.headers ?? {}),
    },
  });
  const text = await response.text();
  let body;
  try { body = text ? JSON.parse(text) : null; } catch { body = text; }
  return { status: response.status, body };
}

function categoryCount(workbench, category) {
  return Array.isArray(workbench?.items?.[category]) ? workbench.items[category].length : 0;
}

function databaseProof(jobId, ruleId) {
  const encodedPassword = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.PG_PASSWORD}'],
    { encoding: 'utf8', env: process.env, timeout: 15_000 },
  );
  const password = Buffer.from(encodedPassword, 'base64').toString('utf8');
  const query = `SELECT (SELECT count(*) FROM rule_action_jobs WHERE job_id='${jobId}' AND rule_id='${ruleId}' AND tenant_id='default') || ':' || (SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND object_id='${ruleId}' AND action='RULE_WORKBENCH_ACTION' AND detail->>'job_id'='${jobId}');`;
  return execFileSync(
    'kubectl',
    ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'env', `PGPASSWORD=${password}`, 'psql', '-At', '-U', 'postgres', '-d', 'traffic_platform', '-c', query],
    { encoding: 'utf8', env: process.env, timeout: 20_000 },
  ).trim().split('\n').at(-1);
}

const adminToken = accessToken({ permissions: ['*', 'admin:*', 'rule:read', 'rule:write', 'rule:enable'], roles: ['admin'] });
const viewerToken = accessToken({ permissions: ['rule:read'], roles: ['viewer'] });
const otherTenantToken = accessToken({ tenantId: 'tenant-other', permissions: ['rule:read'], roles: ['viewer'] });

const firstPage = await api('/api/v1/rules?limit=7&offset=0', adminToken);
if (firstPage.status !== 200 || !Array.isArray(firstPage.body?.data) || firstPage.body.data.length === 0) {
  throw new Error(`rules list failed: ${firstPage.status}`);
}
const selectedRule = firstPage.body.data[0];
const secondPage = await api('/api/v1/rules?limit=7&offset=7', adminToken);
const workbenchResponse = await api(`/api/v1/rules/${encodeURIComponent(selectedRule.rule_id)}/workbench`, adminToken);
const workbench = workbenchResponse.body?.data;

const deniedAction = await api(`/api/v1/rules/${encodeURIComponent(selectedRule.rule_id)}/actions`, viewerToken, {
  method: 'POST',
  body: JSON.stringify({ action_id: crypto.randomUUID(), action: 'rule-validate', target: 'viewer-denied' }),
});
const crossTenantWorkbench = await api(`/api/v1/rules/${encodeURIComponent(selectedRule.rule_id)}/workbench`, otherTenantToken);
const acceptedAction = await api(`/api/v1/rules/${encodeURIComponent(selectedRule.rule_id)}/actions`, adminToken, {
  method: 'POST',
  body: JSON.stringify({ action_id: crypto.randomUUID(), action: 'rule-validate', target: 'windows-chrome-normal-api-proof', payload: { source: 'ui_rule_management_full_stack' } }),
});
const job = acceptedAction.body?.data;
const durableProof = job?.job_id ? databaseProof(job.job_id, selectedRule.rule_id) : '0:0';

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const cdp = await context.newCDPSession(page);
await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
await cdp.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
page.setDefaultTimeout(15_000);

const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const ruleResponses = [];
page.on('response', (response) => {
  const url = response.url();
  if (url.includes('/api/v1/rules')) ruleResponses.push({ status: response.status(), url: url.replace(baseUrl, '') });
  if (response.status() >= 400 && !url.includes('api.yhchj.com/ip')) badResponses.push({ status: response.status(), url: url.replace(baseUrl, '') });
});
page.on('console', (entry) => {
  if (entry.type() === 'error' && !entry.text().includes('api.yhchj.com/ip') && !entry.text().includes('ERR_CONNECTION_CLOSED') && !entry.location().url?.startsWith('chrome-extension://')) consoleErrors.push(entry.text());
});
page.on('pageerror', (error) => {
  if (error.message !== 'Object') pageErrors.push(error.message);
});

const routeUrl = new URL(`/rules?windowsCdpNormalApiTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${adminToken}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-rules').waitFor({ state: 'visible' });
await page.locator('.taf-rules-list-panel .ant-table-tbody .ant-table-row').first().waitFor({ state: 'visible' });
await page.locator('.taf-rules-sample-row').first().waitFor({ state: 'visible' });
await page.waitForTimeout(500);

const lifecycleLabel = (status) => {
  const normalized = String(status ?? '').toLowerCase();
  if (['disabled', 'inactive', 'deprecated', 'archived'].some((value) => normalized.includes(value))) return '停用';
  if (['active', 'enabled'].some((value) => normalized.includes(value))) return '启用';
  if (['gray', 'canary'].some((value) => normalized.includes(value))) return '灰度';
  if (['pending', 'review'].some((value) => normalized.includes(value))) return '待审';
  if (normalized.includes('rollback')) return '回滚';
  return '草稿';
};

const expectedStatus = lifecycleLabel(selectedRule.status);
const alternateRule = firstPage.body.data.find((item) => lifecycleLabel(item.status) !== expectedStatus);
let alternateLifecycle = null;
if (alternateRule) {
  const alternateExpected = lifecycleLabel(alternateRule.status);
  await page.locator('.taf-rules-list-panel .ant-table-tbody .ant-table-row').filter({ hasText: alternateRule.rule_id }).click();
  await page.waitForFunction((expected) => document.querySelector('.taf-rules-lifecycle')?.getAttribute('data-current-status') === expected, alternateExpected);
  alternateLifecycle = {
    rule_id: alternateRule.rule_id,
    api_status: alternateRule.status,
    expected: alternateExpected,
    actual: await page.locator('.taf-rules-lifecycle').getAttribute('data-current-status'),
    selected_stage: await page.locator('.taf-rules-lifecycle-segment.is-current').getAttribute('data-stage'),
  };
  await page.locator('.taf-rules-list-panel .ant-table-tbody .ant-table-row').filter({ hasText: selectedRule.rule_id }).click();
  await page.waitForFunction((expected) => document.querySelector('.taf-rules-lifecycle')?.getAttribute('data-current-status') === expected, expectedStatus);
}

const normalApiSampleTabs = {};
for (const [buttonName, kind] of [['PCAP 样本 32', 'pcap'], ['Session 样本 128', 'session'], ['日志样本 256', 'logs']]) {
  await page.getByRole('button', { name: buttonName, exact: true }).click();
  const root = page.locator(`.taf-rules-samples.is-${kind}`);
  await root.waitFor({ state: 'visible' });
  normalApiSampleTabs[kind] = await root.evaluate((node) => ({
    rows: node.querySelectorAll('.taf-rules-sample-row').length,
    headers: node.querySelectorAll('.taf-rules-sample-head > span').length,
    field_tags: node.querySelectorAll('.taf-rules-sample-tags > i').length,
    actions: node.querySelectorAll('.taf-rules-sample-actions .ant-btn').length,
    switches: node.querySelectorAll('.taf-rules-sample-actions .ant-switch').length,
    no_horizontal_overflow: node.scrollWidth <= node.clientWidth + 1,
    no_vertical_overflow: node.scrollHeight <= node.clientHeight + 1,
    cells_do_not_overlap: [...node.querySelectorAll('.taf-rules-sample-row')].every((row) => {
      const cells = [...row.children].map((cell) => cell.getBoundingClientRect());
      return cells.every((cell, index) => index === cells.length - 1 || cell.right <= cells[index + 1].left + 1);
    }),
  }));
}
await page.getByRole('button', { name: 'PCAP 样本 32', exact: true }).click();

const browserState = await page.evaluate(() => {
  const lifecycle = document.querySelector('.taf-rules-lifecycle');
  const rows = [...document.querySelectorAll('.taf-rules-list-panel .ant-table-tbody .ant-table-row')];
  const samples = [...document.querySelectorAll('.taf-rules-sample-row')];
  const alertText = [...document.querySelectorAll('.ant-alert')].map((item) => item.textContent ?? '').join(' | ');
  return {
    route: location.pathname,
    visual_mode: new URLSearchParams(location.search).has('__codex_ui_breakdown_production'),
    row_count: rows.length,
    row_text: rows.map((row) => row.textContent ?? ''),
    lifecycle_status: lifecycle?.getAttribute('data-current-status') ?? '',
    lifecycle_selected_stage: document.querySelector('.taf-rules-lifecycle-segment.is-current')?.getAttribute('data-stage') ?? '',
    sample_row_count: samples.length,
    sample_text: samples.map((row) => row.textContent ?? ''),
    has_fixture_id: document.body.textContent?.includes('-SIM') ?? false,
    has_api_error: alertText.includes('真实 API 数据加载失败') || alertText.includes('规则工作台数据加载失败') || alertText.includes('规则列表分页加载失败'),
  };
});

const screenshot = await cdp.send('Page.captureScreenshot', {
  format: 'png',
  fromSurface: true,
  captureBeyondViewport: false,
  clip: { x: 0, y: 0, width: 1920, height: 1080, scale: 1 },
});
fs.writeFileSync(screenshotPath, Buffer.from(screenshot.data, 'base64'));

const assertions = {
  rules_page_one_real: firstPage.status === 200 && firstPage.body.pagination?.offset === 0 && firstPage.body.data.length <= 7,
  rules_server_pagination: secondPage.status === 200 && secondPage.body.pagination?.offset === 7 && secondPage.body.data.every((item) => !firstPage.body.data.some((first) => first.rule_id === item.rule_id)),
  workbench_postgresql: workbenchResponse.status === 200 && workbench?.source === 'postgresql',
  workbench_categories_complete: categoryCount(workbench, 'pcap_samples') === 4 && categoryCount(workbench, 'session_samples') === 4 && categoryCount(workbench, 'log_samples') === 4 && categoryCount(workbench, 'validation_results') === 5 && categoryCount(workbench, 'dependencies') === 6,
  viewer_write_denied: deniedAction.status === 403,
  cross_tenant_denied: crossTenantWorkbench.status === 403,
  action_accepted: acceptedAction.status === 202 && job?.status === 'queued' && Boolean(job?.job_id),
  action_and_audit_atomic: durableProof === '1:1',
  browser_normal_mode: browserState.route === '/rules' && browserState.visual_mode === false && browserState.has_fixture_id === false,
  browser_uses_real_rows: browserState.row_count > 0 && browserState.row_text.some((text) => text.includes(selectedRule.rule_id)),
  browser_uses_workbench_rows: browserState.sample_row_count === 4 && browserState.sample_text.some((text) => text.length > 0),
  browser_lifecycle_matches_api: browserState.lifecycle_status === expectedStatus && browserState.lifecycle_selected_stage === expectedStatus,
  browser_lifecycle_changes_with_api_selection: Boolean(alternateLifecycle && alternateLifecycle.actual === alternateLifecycle.expected && alternateLifecycle.selected_stage === alternateLifecycle.expected),
  browser_all_sample_tabs_use_real_workbench: normalApiSampleTabs.pcap?.rows === 4 && normalApiSampleTabs.pcap?.headers === 5 && normalApiSampleTabs.session?.rows === 4 && normalApiSampleTabs.session?.headers === 5 && normalApiSampleTabs.session?.field_tags === 8 && normalApiSampleTabs.session?.actions === 8 && normalApiSampleTabs.logs?.rows === 4 && normalApiSampleTabs.logs?.headers === 5 && normalApiSampleTabs.logs?.field_tags === 8 && normalApiSampleTabs.logs?.actions === 8 && normalApiSampleTabs.logs?.switches === 4 && Object.values(normalApiSampleTabs).every((tab) => tab.no_horizontal_overflow && tab.no_vertical_overflow && tab.cells_do_not_overlap),
  browser_runtime_clean: browserState.has_api_error === false && badResponses.length === 0 && consoleErrors.length === 0 && pageErrors.length === 0 && ruleResponses.some((item) => item.url.includes('/workbench') && item.status === 200),
};

const result = {
  result: Object.values(assertions).every(Boolean) ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  viewport: { width: 1920, height: 1080 },
  route: '/rules',
  assertions,
  api: {
    selected_rule: { rule_id: selectedRule.rule_id, status: selectedRule.status, expected_lifecycle: expectedStatus },
    first_page: { status: firstPage.status, total: firstPage.body.pagination?.total, count: firstPage.body.data.length, offset: firstPage.body.pagination?.offset },
    second_page: { status: secondPage.status, count: secondPage.body?.data?.length ?? 0, offset: secondPage.body?.pagination?.offset },
    workbench: { status: workbenchResponse.status, source: workbench?.source, category_counts: Object.fromEntries(Object.keys(workbench?.items ?? {}).sort().map((key) => [key, categoryCount(workbench, key)])) },
    viewer_action_status: deniedAction.status,
    cross_tenant_workbench_status: crossTenantWorkbench.status,
    accepted_action: { status: acceptedAction.status, job_id: job?.job_id, job_status: job?.status },
    durable_job_audit_counts: durableProof,
  },
  browser_state: browserState,
  alternate_lifecycle: alternateLifecycle,
  normal_api_sample_tabs: normalApiSampleTabs,
  browser_rule_responses: ruleResponses,
  bad_responses: badResponses,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  screenshot: path.relative(root, screenshotPath),
  timestamp: new Date().toISOString(),
};

fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);

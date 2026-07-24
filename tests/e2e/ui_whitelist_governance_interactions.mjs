#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const baseUrl = 'http://10.0.5.8:30180';
const cdpUrl = 'http://127.0.0.1:9224';
const revision = process.env.WHITELIST_EVIDENCE_REVISION?.trim() || 'r348';
const pageEvidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/whitelist');
const modalEvidenceDir = path.join(root, 'evidence/ui-image-breakdowns/overlays/modal-whitelist-add');
const drawerEvidenceDir = path.join(root, 'evidence/ui-image-breakdowns/overlays/drawer-whitelist-approval');
const outputPath = path.join(pageEvidenceDir, `interaction-${revision}.json`);
const mainScreenshotPath = path.join(pageEvidenceDir, `implementation-${revision}.png`);
const interactionScreenshotPath = path.join(pageEvidenceDir, `interaction-${revision}.png`);
const modalScreenshotPath = path.join(modalEvidenceDir, `implementation-${revision}.png`);
const drawerScreenshotPath = path.join(drawerEvidenceDir, `implementation-${revision}.png`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8,10.0.5.9';

const kubectl = (args) => execFileSync('kubectl', args, { encoding: 'utf8', env: process.env, timeout: 30_000, maxBuffer: 8 * 1024 * 1024 });
const secret = (key) => Buffer.from(kubectl(['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', `jsonpath={.data.${key}}`]).trim(), 'base64').toString('utf8');
const smokeToken = ({ username, userId = crypto.randomUUID() }) => {
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: userId, jti: crypto.randomUUID(), user_id: userId,
    tenant_id: 'default', username, roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:write', 'alert:read', 'audit:read', 'user:read'],
    token_type: 'access', iat: now, exp: now + 1800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', secret('JWT_SECRET')).update(input).digest('base64url')}`;
};
const headers = (token) => ({ Authorization: `Bearer ${token}`, 'X-Tenant-ID': 'default' });
const expectStatus = async (response, expected, label) => {
  const body = await response.json().catch(async () => ({ text: await response.text().catch(() => '') }));
  if (!expected.includes(response.status())) throw new Error(`${label}: status=${response.status()} expected=${expected.join(',')} body=${JSON.stringify(body)}`);
  return body;
};
const routeFor = (token, suffix = '') => {
  const route = new URL(`/whitelist?windowsCdpInteractionTs=${Date.now()}${suffix}`, baseUrl);
  route.hash = `codex_smoke_token=${token}`;
  return route.toString();
};

for (const dir of [pageEvidenceDir, modalEvidenceDir, drawerEvidenceDir]) fs.mkdirSync(dir, { recursive: true });

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0];
if (!context) throw new Error('Windows Chrome did not expose a persistent browser context');
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const creatorToken = smokeToken({ username: 'codex-whitelist-ui-creator' });
const approverToken = smokeToken({ username: 'codex-whitelist-ui-approver' });
const uniqueValue = `ui-whitelist-${Date.now()}.example.test`;
const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
const ignoredExternalFailures = [];
let entryId = '';
let cleanupVersion = 0;
let closingBrowserPage = false;

page.on('response', (response) => {
  if (response.status() >= 400) badResponses.push({ status: response.status(), method: response.request().method(), url: response.url() });
});
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const url = entry.location().url ?? '';
  const item = { text: entry.text(), url };
  if (url.startsWith('chrome-extension://') || url.includes('api.yhchj.com/ip')) ignoredExternalFailures.push(item);
  else consoleErrors.push(item);
});
page.on('pageerror', (error) => {
  if (!closingBrowserPage && error.message !== 'Object') pageErrors.push(error.message);
});
page.on('requestfailed', (request) => {
  const url = request.url();
  const item = { url, error: request.failure()?.errorText ?? 'unknown' };
  if (url.startsWith('chrome-extension://') || url.includes('api.yhchj.com/ip')) ignoredExternalFailures.push(item);
  else requestFailures.push(item);
});

let report;
try {
  await page.goto(routeFor(creatorToken), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-whitelist').waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});

  await page.getByRole('button', { name: '新增白名单' }).click();
  const modal = page.getByRole('dialog', { name: '新增白名单草案' });
  await modal.waitFor({ state: 'visible', timeout: 10_000 });
  const valueInput = modal.locator('.taf-whitelist-form input').first();
  await valueInput.fill('');
  const saveButton = modal.getByRole('button', { name: '保存草案' });
  if (!(await saveButton.isDisabled())) throw new Error('required-value validation did not disable 保存草案');
  await valueInput.fill(uniqueValue);
  const expiryInput = modal.locator('input[type="date"]');
  const originalExpiry = await expiryInput.inputValue();
  await expiryInput.fill('');
  if (!(await saveButton.isDisabled())) throw new Error('empty expiry validation did not disable 保存草案');
  await expiryInput.fill(originalExpiry);
  await modal.getByText('新增白名单草案', { exact: true }).click();
  await page.screenshot({ path: modalScreenshotPath });
  await saveButton.click();
  await modal.waitFor({ state: 'hidden', timeout: 15_000 });
  await page.getByText(/白名单草案 v\d+ 已创建/).waitFor({ state: 'visible', timeout: 10_000 });

  await page.getByLabel('搜索白名单').fill(uniqueValue);
  const row = page.locator('.ant-table-row').filter({ hasText: uniqueValue });
  await row.waitFor({ state: 'visible', timeout: 10_000 });
  await row.click();
  await page.locator('.taf-whitelist-titlebar').getByRole('button', { name: '提交审批' }).click();
  await page.getByText(/提交审批成功/).waitFor({ state: 'visible', timeout: 10_000 });

  const listResponse = await page.request.get(`${baseUrl}/api/v1/whitelist?limit=200`, { headers: headers(creatorToken) });
  const listBody = await expectStatus(listResponse, [200], 'list created whitelist');
  const pendingEntry = (listBody.data?.entries ?? []).find((entry) => entry.value === uniqueValue);
  if (!pendingEntry || pendingEntry.status !== 'pending') throw new Error(`created entry is not pending: ${JSON.stringify(pendingEntry)}`);
  entryId = pendingEntry.id;

  await page.goto(routeFor(approverToken), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-whitelist').waitFor({ state: 'visible', timeout: 20_000 });
  await page.getByLabel('搜索白名单').fill(uniqueValue);
  const approverRow = page.locator('.ant-table-row').filter({ hasText: uniqueValue });
  await approverRow.waitFor({ state: 'visible', timeout: 10_000 });
  await approverRow.getByLabel('查看白名单详情').click();
  const drawer = page.getByRole('dialog', { name: '白名单审批详情' });
  await drawer.waitFor({ state: 'visible', timeout: 10_000 });
  await drawer.getByLabel('审批意见').fill('独立审计员已复核范围、来源告警、到期策略与潜在漏报风险');
  await page.screenshot({ path: drawerScreenshotPath });
  await drawer.getByRole('button', { name: '审批通过' }).click();
  await page.getByText(/审批通过成功/).waitFor({ state: 'visible', timeout: 10_000 });
  await drawer.getByRole('button', { name: '缩短/延长有效期' }).click();
  await page.getByText(/延期成功/).waitFor({ state: 'visible', timeout: 10_000 });

  await drawer.locator('.ant-drawer-close').click();
  await page.getByRole('button', { name: '长期生效（>180天）' }).click();
  await page.getByRole('button', { name: '未归属责任角色' }).click();
  await page.getByRole('button', { name: '即将到期（7天内）' }).click();
  await page.getByRole('button', { name: '规则', exact: true }).click();
  await page.getByRole('button', { name: '模型', exact: true }).click();
  if (await page.locator('canvas').count() < 2) throw new Error('expected two live governance charts');
  await page.screenshot({ path: interactionScreenshotPath });

  await page.getByLabel('搜索白名单').fill(uniqueValue);
  const finalRow = page.locator('.ant-table-row').filter({ hasText: uniqueValue });
  await finalRow.waitFor({ state: 'visible', timeout: 10_000 });
  await finalRow.click();
  await page.locator('.taf-whitelist-titlebar').getByRole('button', { name: '停用' }).click();
  await page.getByRole('button', { name: '确认停用' }).click();
  const disableToast = page.getByText(/批量停用 1 条成功/);
  await disableToast.waitFor({ state: 'visible', timeout: 10_000 });
  await disableToast.waitFor({ state: 'hidden', timeout: 10_000 });
  await page.screenshot({ path: mainScreenshotPath });

  const finalListResponse = await page.request.get(`${baseUrl}/api/v1/whitelist?limit=200`, { headers: headers(approverToken) });
  const finalListBody = await expectStatus(finalListResponse, [200], 'list final whitelist');
  const disabledEntry = (finalListBody.data?.entries ?? []).find((entry) => entry.id === entryId);
  if (!disabledEntry || disabledEntry.status !== 'disabled' || disabledEntry.approval_status !== 'approved' || !disabledEntry.approved_by || disabledEntry.version < 5) {
    throw new Error(`final lifecycle evidence is incomplete: ${JSON.stringify(disabledEntry)}`);
  }
  cleanupVersion = disabledEntry.version;

  const auditResponse = await page.request.get(`${baseUrl}/api/v1/audit/logs?object_id=${encodeURIComponent(entryId)}&limit=50`, { headers: headers(approverToken) });
  const auditBody = await expectStatus(auditResponse, [200], 'whitelist audit trail');
  const auditActions = (auditBody.data?.trails ?? []).map((trail) => trail.action).filter(Boolean);
  const expectedActions = ['WHITELIST_CREATED', 'WHITELIST_APPROVAL_SUBMITTED', 'WHITELIST_APPROVED', 'WHITELIST_EXTENDED', 'WHITELIST_DISABLED'];
  const missingActions = expectedActions.filter((action) => !auditActions.includes(action));
  if (missingActions.length) throw new Error(`audit trail missing actions: ${missingActions.join(', ')}`);

  report = {
    result: 'pass', revision, generated_at: new Date().toISOString(), browser: version.Browser,
    route: '/whitelist', backend: 'windows-chrome-cdp-xshell-tunnel', viewport: { width: 1920, height: 1080 },
    workflow: {
      value: uniqueValue, entry_id: entryId, final_version: disabledEntry.version,
      final_status: disabledEntry.status, approval_status: disabledEntry.approval_status,
      created_by: disabledEntry.created_by, approved_by: disabledEntry.approved_by,
      two_person_rule_observed: disabledEntry.created_by !== disabledEntry.approved_by,
      audit_actions: auditActions, acceptance_audit_retention: 'intentional and append-only',
    },
    ui: {
      required_field_validation: true, modal_opened: true, drawer_opened: true,
      create_submit_approve_extend_disable: true, expiry_tabs_exercised: true,
      builder_tabs_exercised: true, chart_canvases: await page.locator('canvas').count(),
    },
    browser_errors: { bad_responses: badResponses, console_errors: consoleErrors, page_errors: pageErrors, request_failures: requestFailures, ignored_external_failures: ignoredExternalFailures },
    screenshots: {
      main: path.relative(root, mainScreenshotPath), interaction: path.relative(root, interactionScreenshotPath),
      modal: path.relative(root, modalScreenshotPath), drawer: path.relative(root, drawerScreenshotPath),
    },
  };
  if (badResponses.length || consoleErrors.length || pageErrors.length || requestFailures.length) {
    throw new Error(`browser runtime errors: ${JSON.stringify(report.browser_errors)}`);
  }
} finally {
  if (entryId) {
    if (!cleanupVersion) {
      const cleanupListResponse = await page.request.get(`${baseUrl}/api/v1/whitelist?limit=200`, { headers: headers(approverToken) });
      const cleanupListBody = await expectStatus(cleanupListResponse, [200], 'cleanup version lookup');
      cleanupVersion = (cleanupListBody.data?.entries ?? []).find((entry) => entry.id === entryId)?.version ?? 0;
    }
    const cleanupResponse = await page.request.delete(`${baseUrl}/api/v1/whitelist/${encodeURIComponent(entryId)}?expected_version=${cleanupVersion}`, { headers: headers(approverToken) });
    const cleanupStatus = cleanupResponse.status();
    if (cleanupStatus !== 200) throw new Error(`cleanup failed: ${cleanupStatus} ${await cleanupResponse.text()}`);
    const verifyResponse = await page.request.get(`${baseUrl}/api/v1/whitelist?limit=200`, { headers: headers(approverToken) });
    const verifyBody = await expectStatus(verifyResponse, [200], 'cleanup verification');
    if ((verifyBody.data?.entries ?? []).some((entry) => entry.id === entryId)) throw new Error('cleanup verification found the deleted whitelist entry');
    const cleanupAuditResponse = await page.request.get(`${baseUrl}/api/v1/audit/logs?object_id=${encodeURIComponent(entryId)}&limit=50`, { headers: headers(approverToken) });
    const cleanupAuditBody = await expectStatus(cleanupAuditResponse, [200], 'cleanup audit verification');
    const cleanupAuditActions = (cleanupAuditBody.data?.trails ?? []).map((trail) => trail.action).filter(Boolean);
    const expectedCleanupActions = ['WHITELIST_CREATED', 'WHITELIST_APPROVAL_SUBMITTED', 'WHITELIST_APPROVED', 'WHITELIST_EXTENDED', 'WHITELIST_DISABLED', 'WHITELIST_DELETED'];
    const missingCleanupActions = expectedCleanupActions.filter((action) => !cleanupAuditActions.includes(action));
    if (missingCleanupActions.length || cleanupAuditActions.length < expectedCleanupActions.length) {
      throw new Error(`cleanup audit verification failed: actions=${JSON.stringify(cleanupAuditActions)} missing=${missingCleanupActions.join(',')}`);
    }
    if (report) report.cleanup = {
      entry_id: entryId, expected_version: cleanupVersion, status: cleanupStatus, entry_removed: true,
      audit_records_retained: true, audit_record_count: cleanupAuditActions.length,
      audit_actions_after_delete: cleanupAuditActions, expected_audit_records_after_delete: expectedCleanupActions.length,
    };
  }
  closingBrowserPage = true;
  await page.close();
}

fs.writeFileSync(outputPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify({ result: report.result, output: path.relative(root, outputPath), workflow: report.workflow, ui: report.ui, cleanup: report.cleanup }, null, 2));
process.exit(0);

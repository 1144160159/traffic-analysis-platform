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
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/deployments');
const outputPath = path.join(evidenceDir, 'interaction-r264-normal-api.json');
const fullStackOutputPath = path.join(evidenceDir, 'full-stack-r264.json');
const screenshotPath = path.join(evidenceDir, 'interaction-r264-normal-api.png');
const createInitialScreenshotPath = path.join(evidenceDir, 'interaction-r264-create-initial.png');
const createScreenshotPath = path.join(evidenceDir, 'interaction-r264-create-modal.png');
const createWorkflowScreenshotPath = path.join(evidenceDir, 'interaction-r264-create-workflow.png');
const rollbackInitialScreenshotPath = path.join(evidenceDir, 'interaction-r264-rollback-initial.png');
const rollbackScreenshotPath = path.join(evidenceDir, 'interaction-r264-rollback-modal.png');
const rollbackWorkflowScreenshotPath = path.join(evidenceDir, 'interaction-r264-rollback-workflow.png');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
fs.mkdirSync(evidenceDir, { recursive: true });

const encodedSecret = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
const jwtSecret = Buffer.from(encodedSecret, 'base64').toString('utf8');
const encodedPgPassword = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.PG_PASSWORD}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
const pgPassword = Buffer.from(encodedPgPassword, 'base64').toString('utf8');
const psql = (sql) => execFileSync('kubectl', ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'env', 'PGPASSWORD=' + pgPassword, 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-v', 'ON_ERROR_STOP=1', '-Atc', sql], { encoding: 'utf8', env: process.env, timeout: 25_000 });
const defaultUserId = execFileSync('kubectl', ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', "SELECT user_id FROM users WHERE tenant_id='default' ORDER BY created_at LIMIT 1"], { encoding: 'utf8', env: process.env, timeout: 15_000 }).trim().split('\n').find((line) => /^[0-9a-f-]{36}$/i.test(line.trim()))?.trim();
if (!defaultUserId) throw new Error('no default tenant user available for deployment create acceptance');

function accessToken({ tenantId = 'default', permissions, roles, userId = crypto.randomUUID() }) {
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({ iss: 'traffic-auth-service', sub: userId, jti: crypto.randomUUID(), user_id: userId, tenant_id: tenantId, username: `codex-deploy-${roles[0]}`, roles, permissions, token_type: 'access', iat: now, exp: now + 1_800 })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', jwtSecret).update(input).digest('base64url')}`;
}

async function api(pathname, token, init = {}) {
  const response = await fetch(`${baseUrl}${pathname}`, { ...init, headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json', ...(init.headers ?? {}) } });
  const text = await response.text();
  let body;
  try { body = text ? JSON.parse(text) : null; } catch { body = text; }
  return { status: response.status, body };
}

function categoryCount(workbench, category) {
  return Array.isArray(workbench?.items?.[category]) ? workbench.items[category].length : 0;
}

function validSevenItemPrecheck(payload) {
	const rows = payload?.data?.precheck_results;
	if (!Array.isArray(rows) || rows.length !== 7 || new Set(rows.map((item) => item?.label)).size !== 7) return false;
	const now = Date.now();
	return rows.every((item) => {
	  const checkedAt = Date.parse(String(item?.checked_at ?? ''));
	  const observedAt = Date.parse(String(item?.source_observed_at ?? ''));
	  const freshUntil = Date.parse(String(item?.fresh_until ?? ''));
	  return Number.isFinite(checkedAt) && Number.isFinite(observedAt) && Number.isFinite(freshUntil) && Math.abs(now - checkedAt) < 120_000 && freshUntil > now;
	});
}

async function capture(cdp, target) {
  const image = await cdp.send('Page.captureScreenshot', { format: 'png', fromSurface: true, captureBeyondViewport: false });
  const buffer = Buffer.from(image.data, 'base64');
  const width = buffer.readUInt32BE(16);
  const height = buffer.readUInt32BE(20);
  if (width !== 1920 || height !== 1080) throw new Error(`unexpected Windows Chrome screenshot size: ${width}x${height}`);
  fs.writeFileSync(target, buffer);
}

async function settleVisibleSurface(page, locator) {
  await locator.waitFor({ state: 'visible' });
  await page.waitForTimeout(500);
}

async function expectEnabled(locator) {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    if (await locator.isEnabled()) return;
    await locator.page().waitForTimeout(100);
  }
  throw new Error(`control did not become enabled: ${await locator.textContent()}`);
}

const adminToken = accessToken({ permissions: ['*', 'admin:*', 'deploy:*', 'audit:read'], roles: ['admin'], userId: defaultUserId });
const viewerToken = accessToken({ permissions: ['deploy:read'], roles: ['viewer'] });
const otherTenantToken = accessToken({ tenantId: 'tenant-other', permissions: ['deploy:read'], roles: ['viewer'] });
const isolatedTenant = 'codex-ui-deploy-' + Date.now();
const isolatedUserId = crypto.randomUUID();
const isolatedApproverUserId = crypto.randomUUID();
const isolatedRuleId = crypto.randomUUID();
const isolatedRuleVersion = isolatedRuleId + '-v1';
const isolatedActiveId = crypto.randomUUID();
const isolatedPlannedId = crypto.randomUUID();
const isolatedCrossLineId = crypto.randomUUID();
const isolatedScopeId = crypto.randomUUID();
const isolatedToken = accessToken({ tenantId: isolatedTenant, permissions: ['deploy:read', 'deploy:create', 'deploy:gray', 'deploy:approve', 'deploy:activate', 'deploy:rollback', 'audit:read'], roles: ['operator'], userId: isolatedUserId });
const isolatedViewerToken = accessToken({ tenantId: isolatedTenant, permissions: ['deploy:read'], roles: ['viewer'] });
const isolatedApproverToken = accessToken({ tenantId: isolatedTenant, permissions: ['deploy:read', 'deploy:approve'], roles: ['operator'], userId: isolatedApproverUserId });
let isolatedFixtureSeeded = false;
let isolatedFixtureCleaned = false;

function cleanupIsolatedFixture() {
  if (!isolatedFixtureSeeded || isolatedFixtureCleaned) return true;
  try {
		psql("DELETE FROM deployment_outbox WHERE tenant_id = '" + isolatedTenant + "'; DELETE FROM deployment_history WHERE deployment_id IN (SELECT deployment_id FROM deployments WHERE tenant_id = '" + isolatedTenant + "'); DELETE FROM deployment_workbench_items WHERE tenant_id = '" + isolatedTenant + "'; DELETE FROM deployments WHERE tenant_id = '" + isolatedTenant + "'; DELETE FROM rule_versions WHERE tenant_id = '" + isolatedTenant + "'; DELETE FROM rules WHERE tenant_id = '" + isolatedTenant + "'; DELETE FROM users WHERE tenant_id = '" + isolatedTenant + "'; DELETE FROM tenants WHERE tenant_id = '" + isolatedTenant + "';");
    isolatedFixtureCleaned = true;
    return true;
  } catch {
    return false;
  }
}

function cleanupAfterUnhandled(reason) {
  cleanupIsolatedFixture();
  console.error(reason instanceof Error ? reason.stack ?? reason.message : reason);
  process.exit(1);
}

process.once('uncaughtException', cleanupAfterUnhandled);
process.once('unhandledRejection', cleanupAfterUnhandled);

psql([
  "INSERT INTO tenants (tenant_id, tenant_name, name, description, status) VALUES ('" + isolatedTenant + "', '" + isolatedTenant + "', '" + isolatedTenant + "', 'Windows Chrome deployment acceptance', 'active')",
  "INSERT INTO users (user_id, tenant_id, username, email, status) VALUES ('" + isolatedUserId + "', '" + isolatedTenant + "', 'codex-ui-deploy-admin', 'codex-ui-deploy-admin@local', 'active')",
	"INSERT INTO users (user_id, tenant_id, username, email, status) VALUES ('" + isolatedApproverUserId + "', '" + isolatedTenant + "', 'codex-ui-deploy-approver', 'codex-ui-deploy-approver@local', 'active')",
  "INSERT INTO rules (rule_id, tenant_id, name, rule_type, engine, description, conditions, labels, severity, enabled, priority, version, status, created_by, updated_by, created_at, updated_at) VALUES ('" + isolatedRuleId + "', '" + isolatedTenant + "', 'Windows Chrome 验收规则', 'custom', 'internal', 'deployment browser acceptance', '{\"source\":\"windows-chrome\"}'::jsonb, ARRAY['codex','windows-chrome']::text[], 'medium', true, 50, 1, 'active', '" + isolatedUserId + "', '" + isolatedUserId + "', now(), now())",
  "INSERT INTO rule_versions (rule_version, rule_id, tenant_id, version, content_uri, status, created_by) VALUES ('" + isolatedRuleVersion + "', '" + isolatedRuleId + "', '" + isolatedTenant + "', 1, 'inline:{\"source\":\"windows-chrome\"}', 'active', '" + isolatedUserId + "')",
	"INSERT INTO deployments (deployment_id, tenant_id, name, description, rule_version, scope, status, created_by, created_at, updated_at, metadata) VALUES ('" + isolatedActiveId + "', '" + isolatedTenant + "', '浏览器验收稳定版本', 'rollback target', '" + isolatedRuleVersion + "', '{\"percentage\":100,\"campus\":\"验收园区\",\"tenant\":\"隔离租户\",\"release_line\":\"ruleset\"}'::jsonb, 'active', '" + isolatedUserId + "', now() - interval '10 minutes', now() - interval '10 minutes', '{\"case\":\"browser-active\"}'::jsonb), ('" + isolatedPlannedId + "', '" + isolatedTenant + "', '浏览器验收待发布版本', 'create source', '" + isolatedRuleVersion + "', '{\"percentage\":10,\"campus\":\"验收园区\",\"tenant\":\"隔离租户\",\"release_line\":\"ruleset\"}'::jsonb, 'planned', '" + isolatedUserId + "', now(), now(), '{\"case\":\"browser-planned\"}'::jsonb), ('" + isolatedCrossLineId + "', '" + isolatedTenant + "', '浏览器验收异发布线版本', 'must stay active', '" + isolatedRuleVersion + "', '{\"percentage\":100,\"campus\":\"验收园区\",\"tenant\":\"隔离租户\",\"release_line\":\"other-line\"}'::jsonb, 'active', '" + isolatedUserId + "', now() - interval '20 minutes', now() - interval '20 minutes', '{\"case\":\"browser-cross-line\"}'::jsonb), ('" + isolatedScopeId + "', '" + isolatedTenant + "', '浏览器验收范围编辑版本', 'isolated scope mutation target', '" + isolatedRuleVersion + "', '{\"percentage\":22,\"campus\":\"验收园区\",\"tenant\":\"隔离租户\",\"release_line\":\"scope-test\"}'::jsonb, 'planned', '" + isolatedUserId + "', now(), now(), '{\"case\":\"browser-scope\"}'::jsonb)",
	"INSERT INTO deployment_workbench_items (item_id, tenant_id, deployment_id, category, ordinal, payload, scenario_id, occurred_at) VALUES ('" + isolatedTenant + "-checkpoint', '" + isolatedTenant + "', '*', 'health', 1, '{\"label\":\"Flink Checkpoint 成功率\",\"tone\":\"ok\",\"value\":\"100%\"}'::jsonb, 'browser', now()), ('" + isolatedTenant + "-topic', '" + isolatedTenant + "', '*', 'evidence', 1, '{\"label\":\"topic\",\"status\":\"passed\",\"checksum\":\"browser-topic\"}'::jsonb, 'browser', now())"
].join('; ') + ';');
isolatedFixtureSeeded = true;

const directDeployConfiguration = { gray_percentage: 10, traffic_copy_strategy: '镜像复制（推荐）', probe_coverage_strategy: '强制升级' };
const directDraft = await api(`/api/v1/deployments/${isolatedPlannedId}/workflow`, isolatedToken, { method: 'POST', body: JSON.stringify({ stage: 'draft', operation: 'deploy', configuration: directDeployConfiguration }) });
const directPrecheck = await api(`/api/v1/deployments/${isolatedPlannedId}/workflow`, isolatedToken, { method: 'POST', body: JSON.stringify({ stage: 'precheck', operation: 'deploy', configuration: directDeployConfiguration }) });
const directSubmit = await api(`/api/v1/deployments/${isolatedPlannedId}/workflow`, isolatedToken, { method: 'POST', body: JSON.stringify({ stage: 'submit_approval', operation: 'deploy', configuration: directDeployConfiguration }) });
const directSelfApprove = await api(`/api/v1/deployments/${isolatedPlannedId}/workflow`, isolatedToken, { method: 'POST', body: JSON.stringify({ stage: 'approve', operation: 'deploy' }) });
const directTamperedApprove = await api(`/api/v1/deployments/${isolatedPlannedId}/workflow`, isolatedApproverToken, { method: 'POST', body: JSON.stringify({ stage: 'approve', operation: 'deploy', configuration: { ...directDeployConfiguration, gray_percentage: 90 } }) });
const directApprove = await api(`/api/v1/deployments/${isolatedPlannedId}/workflow`, isolatedApproverToken, { method: 'POST', body: JSON.stringify({ stage: 'approve', operation: 'deploy' }) });
const directScopeReset = await api(`/api/v1/deployments/${isolatedPlannedId}/scope`, isolatedToken, { method: 'PUT', body: JSON.stringify({ scope: { percentage: 15 } }) });
const directGrayWithStaleApproval = await api(`/api/v1/deployments/${isolatedPlannedId}/gray`, isolatedToken, { method: 'POST' });
const directReDeployConfiguration = { ...directDeployConfiguration, gray_percentage: 15 };
const directRePrecheck = await api(`/api/v1/deployments/${isolatedPlannedId}/workflow`, isolatedToken, { method: 'POST', body: JSON.stringify({ stage: 'precheck', operation: 'deploy', configuration: directReDeployConfiguration }) });
const directReSubmit = await api(`/api/v1/deployments/${isolatedPlannedId}/workflow`, isolatedToken, { method: 'POST', body: JSON.stringify({ stage: 'submit_approval', operation: 'deploy', configuration: directReDeployConfiguration }) });
const directReApprove = await api(`/api/v1/deployments/${isolatedPlannedId}/workflow`, isolatedApproverToken, { method: 'POST', body: JSON.stringify({ stage: 'approve', operation: 'deploy' }) });
const directGray = await api(`/api/v1/deployments/${isolatedPlannedId}/gray`, isolatedToken, { method: 'POST' });
const directActivate = await api(`/api/v1/deployments/${isolatedPlannedId}/activate`, isolatedToken, { method: 'POST' });
const directReleaseLineWorkbench = await api(`/api/v1/deployments/${isolatedPlannedId}/workbench`, isolatedToken);
const directCrossLineRollback = await api(`/api/v1/deployments/${isolatedPlannedId}/workflow`, isolatedToken, { method: 'POST', body: JSON.stringify({ stage: 'draft', operation: 'rollback', configuration: { target_deployment_id: isolatedCrossLineId, reason: '验证跨发布线回滚必须被拒绝' } }) });
const directCrossLineStatus = psql("SELECT status FROM deployments WHERE deployment_id = '" + isolatedCrossLineId + "'").trim().split('\n').at(-1);

const allResponse = await api('/api/v1/deployments?limit=100&offset=0', adminToken);
if (allResponse.status !== 200 || !Array.isArray(allResponse.body?.data)) throw new Error(`deployment list failed: ${allResponse.status}`);
const records = allResponse.body.data;
const plannedIndex = records.findIndex((item) => ['planned', 'gray', 'paused'].includes(item.status));
const rollbackIndex = records.findIndex((item) => ['gray', 'active', 'paused', 'failed'].includes(item.status) && item.metadata?.workflow?.operation !== 'rollback');
if (plannedIndex < 0 || rollbackIndex < 0) throw new Error('no eligible deployment fixtures found');
const selected = records[plannedIndex];
const rollbackTarget = records[rollbackIndex];
const firstPage = await api('/api/v1/deployments?limit=10&offset=0', adminToken);
const secondPage = await api('/api/v1/deployments?limit=10&offset=10', adminToken);
const workbenchResponse = await api(`/api/v1/deployments/${encodeURIComponent(selected.deployment_id)}/workbench`, adminToken);
const viewerDenied = await api(`/api/v1/deployments/${encodeURIComponent(isolatedScopeId)}/scope`, isolatedViewerToken, { method: 'PUT', body: JSON.stringify({ scope: { percentage: 31 } }) });
const crossTenantDenied = await api(`/api/v1/deployments/${encodeURIComponent(selected.deployment_id)}/workbench`, otherTenantToken);
const invalidScope = await api(`/api/v1/deployments/${encodeURIComponent(isolatedScopeId)}/scope`, isolatedToken, { method: 'PUT', body: JSON.stringify({ scope: { percentage: 131 } }) });
const desiredPercentage = 37;
const scopeResponse = await api(`/api/v1/deployments/${encodeURIComponent(isolatedScopeId)}/scope`, isolatedToken, { method: 'PUT', body: JSON.stringify({ scope: { tenant: '隔离租户', campus: '验收园区', probe_group: '办公区探针组 (12)', asset_group: '核心业务资产组', percentage: desiredPercentage } }) });
const evidenceResponse = await api(`/api/v1/deployments/${encodeURIComponent(selected.deployment_id)}/evidence/export`, adminToken, { method: 'POST' });
const evidenceDownloadContent = evidenceResponse.body?.data?.download_content ?? '';
const evidenceContentChecksum = `sha256:${crypto.createHash('sha256').update(evidenceDownloadContent).digest('hex')}`;

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
if (!version.webSocketDebuggerUrl) throw new Error('Windows Chrome CDP metadata missing webSocketDebuggerUrl');
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const cdp = await context.newCDPSession(page);
await cdp.send('Browser.setDownloadBehavior', { behavior: 'allow', downloadPath: 'C:\\Users\\18229\\Downloads', eventsEnabled: true });
await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
await cdp.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
page.setDefaultTimeout(30_000);

const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const deploymentResponses = [];
page.on('response', (response) => {
  const url = response.url();
  if (url.includes('/api/v1/deployments')) deploymentResponses.push({ status: response.status(), method: response.request().method(), url: url.replace(baseUrl, '') });
  if (response.status() >= 400 && !url.includes('api.yhchj.com/ip')) badResponses.push({ status: response.status(), url: url.replace(baseUrl, '') });
});
page.on('console', (entry) => {
  const text = entry.text();
  const location = entry.location().url ?? '';
  const ignoredExternalFailure = text.includes('api.yhchj.com/ip') || location.includes('api.yhchj.com/ip');
  if (entry.type() === 'error' && !ignoredExternalFailure && !text.includes('ERR_CONNECTION_CLOSED') && !location.startsWith('chrome-extension://')) {
    consoleErrors.push({ text, location });
  }
});
page.on('pageerror', (error) => { if (error.message !== 'Object') pageErrors.push(error.message); });

const routeUrl = new URL(`/deployments?windowsCdpNormalApiTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${adminToken}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.bringToFront();
const initialViewport = await page.evaluate(() => ({ width: innerWidth, height: innerHeight, device_pixel_ratio: devicePixelRatio }));
const pageZoomFactor = initialViewport.device_pixel_ratio > 0 ? initialViewport.device_pixel_ratio : 1920 / initialViewport.width;
const calibratedMetrics = {
  width: Math.round(1920 * pageZoomFactor),
  height: Math.round(1080 * pageZoomFactor),
  deviceScaleFactor: 1 / pageZoomFactor,
  mobile: false,
};
await cdp.send('Emulation.setDeviceMetricsOverride', calibratedMetrics);
await page.waitForTimeout(300);
const observedViewport = await page.evaluate(() => ({ width: innerWidth, height: innerHeight, device_pixel_ratio: devicePixelRatio, visual_scale: visualViewport?.scale ?? 1 }));
await page.locator('.taf-deployments').waitFor({ state: 'visible' });
await page.locator('.taf-deployments-list-panel .ant-table-row').first().waitFor({ state: 'visible' });
await page.locator('.taf-deployments-health-echart canvas').first().waitFor({ state: 'visible' });
const browserHealthCanvasCount = await page.locator('.taf-deployments-health-echart canvas').count();
const browserPageSize = 6;

async function selectRecord(record, index) {
  const targetPage = Math.floor(index / browserPageSize) + 1;
  if (targetPage > 1) await page.getByRole('button', { name: `发布清单第 ${targetPage} 页` }).click();
  else await page.getByRole('button', { name: '发布清单第 1 页' }).click();
  const row = page.locator('.taf-deployments-list-panel .ant-table-row', { hasText: record.name }).first();
  await row.waitFor({ state: 'visible' });
  await row.click();
  await page.waitForTimeout(250);
}

await selectRecord(selected, plannedIndex);
let browserScopeResponse;
let browserScopePercentage = Number.NaN;

const browserDownload = page.waitForEvent('download', { timeout: 10_000 });
const browserDownloadCompleted = new Promise((resolve, reject) => {
  const onProgress = (event) => {
    if (event.state === 'completed') {
      cdp.off('Browser.downloadProgress', onProgress);
      resolve(event);
    } else if (event.state === 'canceled') {
      cdp.off('Browser.downloadProgress', onProgress);
      reject(new Error('Windows Chrome reported a canceled evidence download'));
    }
  };
  cdp.on('Browser.downloadProgress', onProgress);
});
const browserExportResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith(`/api/v1/deployments/${selected.deployment_id}/evidence/export`));
await page.locator('.taf-deployments-titlebar button', { hasText: '导出证据' }).click();
const downloadedEvidence = await browserDownload;
const browserExportResponse = await browserExportResponsePromise;
const browserExportBody = await browserExportResponse.json();
const browserDownloadProgress = await browserDownloadCompleted;
const browserDownloadedChecksum = 'sha256:' + crypto.createHash('sha256').update(String(browserExportBody?.data?.download_content ?? '')).digest('hex');
const browserDownloadedBytes = Number(browserDownloadProgress.totalBytes ?? 0);
const browserDownloadExpectedBytes = Buffer.byteLength(String(browserExportBody?.data?.download_content ?? ''));
await page.locator('.ant-drawer-content-wrapper:visible').waitFor({ state: 'visible' });
const browserExportRecord = deploymentResponses.findLast((item) => item.method === 'POST' && item.url.includes('/evidence/export'));
await page.locator('.ant-drawer-close').click();

const browserVisibleRowCount = await page.locator('.taf-deployments-list-panel .ant-table-row').count();
const rollbackReasonBox = await page.locator('.taf-deployments-rollback input').boundingBox();
const rollbackExecuteBox = await page.locator('.taf-deployments-rollback-actions button', { hasText: '执行回滚' }).boundingBox();
await capture(cdp, screenshotPath);
const isolatedRouteUrl = new URL('/deployments?windowsCdpIsolatedApiTs=' + Date.now(), baseUrl);
isolatedRouteUrl.hash = 'codex_smoke_token=' + isolatedToken;
await page.goto(isolatedRouteUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-deployments').waitFor({ state: 'visible' });
await page.locator('.taf-deployments-list-panel .ant-table-row').first().waitFor({ state: 'visible' });

const isolatedScopeRow = page.locator('.taf-deployments-list-panel .ant-table-row', { hasText: '浏览器验收范围编辑版本' }).first();
await isolatedScopeRow.waitFor({ state: 'visible' });
await isolatedScopeRow.click();
const scopeSliderHandle = page.locator('.taf-deployments-slider .ant-slider-handle');
await scopeSliderHandle.focus();
await scopeSliderHandle.press('ArrowRight');
const scopePut = page.waitForResponse((response) => response.request().method() === 'PUT' && response.url().endsWith(`/api/v1/deployments/${isolatedScopeId}/scope`));
await page.getByRole('button', { name: '编辑策略' }).click();
browserScopeResponse = await scopePut;
const browserScopePayload = browserScopeResponse.request().postDataJSON();
browserScopePercentage = Number(browserScopePayload?.scope?.percentage);
await page.locator('.taf-deployments-gray-feedback').waitFor({ state: 'visible' });

const isolatedPlannedRow = page.locator('.taf-deployments-list-panel .ant-table-row', { hasText: '浏览器验收待发布版本' }).first();
await isolatedPlannedRow.click();
await page.waitForTimeout(250);

await page.locator('.taf-deployments-titlebar button', { hasText: '新建发布' }).click();
const createModal = page.locator('.taf-deployments-operation-modal:visible');
await settleVisibleSurface(page, createModal);
const createModalText = await createModal.textContent();
await capture(cdp, createInitialScreenshotPath);
const createPost = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname.endsWith('/api/v1/deployments'));
await createModal.getByRole('button', { name: '保存草案' }).click();
const browserCreateResponse = await createPost;
const browserCreateBody = await browserCreateResponse.json();
const browserCreatedDeploymentId = String(browserCreateBody?.data?.deployment_id ?? '');
if (browserCreateResponse.status() !== 201 || !browserCreatedDeploymentId) throw new Error(`browser create failed: ${browserCreateResponse.status()} ${JSON.stringify(browserCreateBody)}`);
const createPrecheck = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith(`/api/v1/deployments/${browserCreatedDeploymentId}/workflow`));
await expectEnabled(createModal.getByRole('button', { name: '运行预检查' }));
await createModal.getByRole('button', { name: '运行预检查' }).click();
const browserCreatePrecheckResponse = await createPrecheck;
const browserCreatePrecheckBody = await browserCreatePrecheckResponse.json();
await createModal.getByRole('button', { name: '提交部署审批' }).waitFor({ state: 'visible' });
await expectEnabled(createModal.getByRole('button', { name: '提交部署审批' }));
await settleVisibleSurface(page, createModal);
await capture(cdp, createScreenshotPath);
const createApproval = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith(`/api/v1/deployments/${browserCreatedDeploymentId}/workflow`));
await createModal.getByRole('button', { name: '提交部署审批' }).click();
const browserCreateApprovalResponse = await createApproval;
const browserCreateApprovalBody = await browserCreateApprovalResponse.json();
await createModal.getByRole('button', { name: '等待其他审批人' }).waitFor({ state: 'visible' });
const browserCreateSelfApprove = await api(`/api/v1/deployments/${browserCreatedDeploymentId}/workflow`, isolatedToken, { method: 'POST', body: JSON.stringify({ stage: 'approve', operation: 'deploy' }) });
const browserCreateTamperedApprove = await api(`/api/v1/deployments/${browserCreatedDeploymentId}/workflow`, isolatedApproverToken, { method: 'POST', body: JSON.stringify({ stage: 'approve', operation: 'deploy', configuration: { gray_percentage: 95 } }) });
await createModal.locator('.ant-modal-close').click();
await createModal.waitFor({ state: 'hidden' });
const approverCreateRoute = new URL('/deployments?windowsCdpApproverCreateTs=' + Date.now(), baseUrl);
approverCreateRoute.hash = 'codex_smoke_token=' + isolatedApproverToken;
await page.goto(approverCreateRoute.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
const approverCreatedRow = page.locator('.taf-deployments-list-panel .ant-table-row', { hasText: '攻击检测能力生产灰度' }).first();
await approverCreatedRow.waitFor({ state: 'visible' });
await approverCreatedRow.getByRole('button', { name: '处理部署审批' }).click();
await createModal.getByRole('button', { name: '批准审批' }).waitFor({ state: 'visible' });
const createWorkflowRestored = await createModal.getByRole('button', { name: '批准审批' }).isVisible();
const createApprove = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/api/v1/deployments/' + browserCreatedDeploymentId + '/workflow'));
await createModal.getByRole('button', { name: '批准审批' }).click();
const browserCreateApproveResponse = await createApprove;
const browserCreateApproveBody = await browserCreateApproveResponse.json();
const createModalBox = await createModal.boundingBox();
await capture(cdp, createWorkflowScreenshotPath);
await createModal.locator('.ant-modal-close').click();
await createModal.waitFor({ state: 'hidden' });
const requesterCreateRoute = new URL('/deployments?windowsCdpRequesterCreateTs=' + Date.now(), baseUrl);
requesterCreateRoute.hash = 'codex_smoke_token=' + isolatedToken;
await page.goto(requesterCreateRoute.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
const requesterCreatedRow = page.locator('.taf-deployments-list-panel .ant-table-row', { hasText: '攻击检测能力生产灰度' }).first();
await requesterCreatedRow.waitFor({ state: 'visible' });
await requesterCreatedRow.getByRole('button', { name: '启动灰度' }).click();
await createModal.getByRole('button', { name: '启动灰度' }).waitFor({ state: 'visible' });
const createExecute = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/api/v1/deployments/' + browserCreatedDeploymentId + '/gray'));
await createModal.getByRole('button', { name: '启动灰度' }).click();
const browserCreateExecuteResponse = await createExecute;
await createModal.locator('.ant-modal-close').click();
await createModal.waitFor({ state: 'hidden' });

await page.reload({ waitUntil: 'domcontentloaded' });
await page.locator('.taf-deployments-list-panel .ant-table-row').first().waitFor({ state: 'visible' });
await page.locator('.taf-deployments-list-panel .ant-table-row', { hasText: '浏览器验收稳定版本' }).first().click();
const createdRow = page.locator('.taf-deployments-list-panel .ant-table-row', { hasText: '攻击检测能力生产灰度' }).first();
await createdRow.waitFor({ state: 'visible' });
await createdRow.getByRole('button', { name: '回滚发布' }).click();
const rollbackModal = page.locator('.taf-deployments-operation-modal:visible');
await settleVisibleSurface(page, rollbackModal);
const rollbackModalText = await rollbackModal.textContent();
const rowActionTargetsCreatedDeployment = rollbackModalText?.includes('攻击检测能力生产灰度') === true;
await expectEnabled(rollbackModal.getByRole('button', { name: '保存回滚单' }));
await capture(cdp, rollbackInitialScreenshotPath);
const rollbackDraft = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/api/v1/deployments/' + browserCreatedDeploymentId + '/workflow'));
await rollbackModal.getByRole('button', { name: '保存回滚单' }).click();
const browserRollbackDraftResponse = await rollbackDraft;
const rollbackPrecheck = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/api/v1/deployments/' + browserCreatedDeploymentId + '/workflow'));
await expectEnabled(rollbackModal.getByRole('button', { name: '运行回滚检查' }));
await rollbackModal.getByRole('button', { name: '运行回滚检查' }).click();
const browserRollbackPrecheckResponse = await rollbackPrecheck;
const browserRollbackPrecheckBody = await browserRollbackPrecheckResponse.json();
await expectEnabled(rollbackModal.getByRole('button', { name: '提交回滚审批' }));
await settleVisibleSurface(page, rollbackModal);
await capture(cdp, rollbackScreenshotPath);
const rollbackApproval = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/api/v1/deployments/' + browserCreatedDeploymentId + '/workflow'));
await rollbackModal.getByRole('button', { name: '提交回滚审批' }).click();
const browserRollbackApprovalResponse = await rollbackApproval;
const browserRollbackApprovalBody = await browserRollbackApprovalResponse.json();
await rollbackModal.getByRole('button', { name: '等待其他审批人' }).waitFor({ state: 'visible' });
const browserRollbackSelfApprove = await api(`/api/v1/deployments/${browserCreatedDeploymentId}/workflow`, isolatedToken, { method: 'POST', body: JSON.stringify({ stage: 'approve', operation: 'rollback' }) });
await rollbackModal.locator('.ant-modal-close').click();
await rollbackModal.waitFor({ state: 'hidden' });
const approverRollbackRoute = new URL('/deployments?windowsCdpApproverRollbackTs=' + Date.now(), baseUrl);
approverRollbackRoute.hash = 'codex_smoke_token=' + isolatedApproverToken;
await page.goto(approverRollbackRoute.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
const approverRollbackRow = page.locator('.taf-deployments-list-panel .ant-table-row', { hasText: '攻击检测能力生产灰度' }).first();
await approverRollbackRow.waitFor({ state: 'visible' });
await approverRollbackRow.getByRole('button', { name: '处理回滚审批' }).click();
await rollbackModal.waitFor({ state: 'visible' });
await rollbackModal.getByRole('button', { name: '批准审批' }).waitFor({ state: 'visible' });
const rollbackWorkflowRestored = await rollbackModal.getByRole('button', { name: '批准审批' }).isVisible();
const rollbackApprove = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/api/v1/deployments/' + browserCreatedDeploymentId + '/workflow'));
await rollbackModal.getByRole('button', { name: '批准审批' }).click();
const browserRollbackApproveResponse = await rollbackApprove;
const browserRollbackApproveBody = await browserRollbackApproveResponse.json();
const rollbackModalBox = await rollbackModal.boundingBox();
await capture(cdp, rollbackWorkflowScreenshotPath);
await rollbackModal.locator('.ant-modal-close').click();
await rollbackModal.waitFor({ state: 'hidden' });
const requesterRollbackRoute = new URL('/deployments?windowsCdpRequesterRollbackTs=' + Date.now(), baseUrl);
requesterRollbackRoute.hash = 'codex_smoke_token=' + isolatedToken;
await page.goto(requesterRollbackRoute.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
const requesterRollbackRow = page.locator('.taf-deployments-list-panel .ant-table-row', { hasText: '攻击检测能力生产灰度' }).first();
await requesterRollbackRow.waitFor({ state: 'visible' });
await requesterRollbackRow.getByRole('button', { name: '回滚发布' }).click();
await rollbackModal.getByRole('button', { name: '执行回滚' }).waitFor({ state: 'visible' });
const browserRollbackMutationDenied = await api(`/api/v1/deployments/${browserCreatedDeploymentId}/rollback`, isolatedToken, { method: 'POST', body: JSON.stringify({ target_deployment_id: isolatedActiveId, reason: '与审批快照不同的回滚原因必须拒绝' }) });
const rollbackExecute = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/api/v1/deployments/' + browserCreatedDeploymentId + '/rollback'));
await rollbackModal.getByRole('button', { name: '执行回滚' }).click();
const browserRollbackExecuteResponse = await rollbackExecute;
await rollbackModal.locator('.ant-modal-close').click();
await rollbackModal.waitFor({ state: 'hidden' });

const persistedWorkbench = await api(`/api/v1/deployments/${encodeURIComponent(isolatedScopeId)}/workbench`, isolatedToken);
const auditScope = await api(`/api/v1/audit/logs?action=DEPLOY_SCOPE_UPDATE&object_id=${encodeURIComponent(isolatedScopeId)}&limit=10`, isolatedToken);
const auditExport = await api(`/api/v1/audit/logs?action=DEPLOY_EVIDENCE_EXPORT&object_id=${encodeURIComponent(selected.deployment_id)}&limit=10`, adminToken);
const createdWorkbench = await api('/api/v1/deployments/' + encodeURIComponent(browserCreatedDeploymentId) + '/workbench', isolatedToken);
const auditWorkflow = await api('/api/v1/audit/logs?action=DEPLOY_WORKFLOW_UPDATE&limit=50', isolatedToken);
const workbench = workbenchResponse.body?.data;
let outboxSummary = { total: 0, published: 0, dead: 0, topicOk: false, eventIdOk: false, statusOk: false };
for (let attempt = 0; attempt < 20; attempt += 1) {
	const raw = psql([
		"SELECT COUNT(*)",
		"COUNT(*) FILTER (WHERE status = 'published')",
		"COUNT(*) FILTER (WHERE status = 'dead')",
		"COALESCE(BOOL_AND(topic = 'deployment.events.v1'), false)",
		"COALESCE(BOOL_AND(event_id = payload->>'event_id'), false)",
		"COALESCE(BOOL_AND(COALESCE(payload#>>'{deployment,status}', '') <> ''), false) FROM deployment_outbox WHERE tenant_id = '" + isolatedTenant + "'",
	].join(', '));
	const line = raw.trim().split('\n').findLast((candidate) => /^\d+\|\d+\|\d+\|[tf]\|[tf]\|[tf]$/.test(candidate.trim()));
	if (line) {
		const [total, published, dead, topicOk, eventIdOk, statusOk] = line.trim().split('|');
		outboxSummary = { total: Number(total), published: Number(published), dead: Number(dead), topicOk: topicOk === 't', eventIdOk: eventIdOk === 't', statusOk: statusOk === 't' };
	}
	if (outboxSummary.total > 0 && outboxSummary.published === outboxSummary.total) break;
	await new Promise((resolve) => setTimeout(resolve, 1_000));
}
const directResetWorkflow = directScopeReset.body?.data?.metadata?.workflow;
const directApprovalWorkflow = directApprove.body?.data;
const directRollbackCandidates = directReleaseLineWorkbench.body?.data?.items?.rollback_versions ?? [];
const browserCreateApprovalWorkflow = browserCreateApprovalBody?.data;
const browserCreateApprovedWorkflow = browserCreateApproveBody?.data;
const browserRollbackApprovalWorkflow = browserRollbackApprovalBody?.data;
const browserRollbackApprovedWorkflow = browserRollbackApproveBody?.data;
const createdWorkflow = createdWorkbench.body?.data?.deployment?.metadata?.workflow;
const assertions = {
  list_has_48_rows: firstPage.status === 200 && firstPage.body?.pagination?.total === 48,
  visible_rows_match_page_size: browserVisibleRowCount === browserPageSize,
  rollback_controls_in_viewport: Boolean(rollbackReasonBox && rollbackExecuteBox && rollbackReasonBox.y + rollbackReasonBox.height <= 997 && rollbackExecuteBox.y + rollbackExecuteBox.height <= 997),
  server_pagination_unique: secondPage.status === 200 && secondPage.body?.pagination?.offset === 10 && secondPage.body.data.every((item) => !firstPage.body.data.some((first) => first.deployment_id === item.deployment_id)),
  workbench_postgresql: workbenchResponse.status === 200 && workbench?.source === 'postgresql',
  workbench_categories_complete: categoryCount(workbench, 'health') === 6 && categoryCount(workbench, 'evidence') === 6 && categoryCount(workbench, 'change_summary') === 5 && categoryCount(workbench, 'rollback_versions') === 3,
  viewer_write_denied: viewerDenied.status === 403,
  cross_tenant_denied: crossTenantDenied.status === 403,
  invalid_scope_rejected: invalidScope.status === 400,
  scope_persisted: scopeResponse.status === 200 && Number.isFinite(browserScopePercentage) && browserScopePercentage !== desiredPercentage && Number(persistedWorkbench.body?.data?.deployment?.scope?.percentage) === browserScopePercentage,
  server_evidence_bundle: evidenceResponse.status === 200 && evidenceResponse.body?.data?.source === 'postgresql' && evidenceResponse.body?.data?.evidence?.length === 6 && evidenceContentChecksum === evidenceResponse.body?.data?.bundle_checksum,
  browser_scope_real_api: browserScopeResponse?.status() === 200,
  browser_export_real_api: browserExportRecord?.status === 200,
  browser_download_observed: downloadedEvidence.suggestedFilename().startsWith('DEP-EVIDENCE-'),
  browser_download_checksum: browserDownloadedChecksum === browserExportBody?.data?.bundle_checksum,
  browser_saved_download_completed: browserDownloadProgress.state === 'completed' && browserDownloadedBytes > 0 && browserDownloadedBytes === browserDownloadExpectedBytes,
  direct_workflow_draft_saved: directDraft.status === 200 && directDraft.body?.data?.stage === 'draft_saved',
  direct_precheck_exactly_seven_fresh: directPrecheck.status === 200 && validSevenItemPrecheck(directPrecheck.body),
  direct_self_approval_denied: directSelfApprove.status === 403,
  direct_tampered_approval_denied: directTamperedApprove.status === 409,
  direct_independent_approval: directSubmit.status === 200 && directApprove.status === 200 && directApprovalWorkflow?.requested_by === isolatedUserId && directApprovalWorkflow?.approved_by === isolatedApproverUserId && typeof directApprovalWorkflow?.approval_snapshot_hash === 'string',
  scope_change_invalidates_approval: directScopeReset.status === 200 && directResetWorkflow?.stage === 'draft_saved' && !directResetWorkflow?.approval_snapshot_hash && !directResetWorkflow?.approved_by,
  stale_approval_execution_denied: directGrayWithStaleApproval.status === 409,
  reapproval_execution_succeeds: validSevenItemPrecheck(directRePrecheck.body) && [directReSubmit.status, directReApprove.status, directGray.status, directActivate.status].every((status) => status === 200),
  release_line_isolated: [400, 409].includes(directCrossLineRollback.status) && directCrossLineStatus === 'active' && directReleaseLineWorkbench.status === 200 && !JSON.stringify(directRollbackCandidates).includes(isolatedCrossLineId),
  browser_create_workflow_real_api: browserCreateResponse.status() === 201 && browserCreatePrecheckResponse.status() === 200 && browserCreateApprovalResponse.status() === 200 && browserCreateApproveResponse.status() === 200 && browserCreateExecuteResponse.status() === 200,
  browser_create_precheck_exactly_seven_fresh: validSevenItemPrecheck(browserCreatePrecheckBody),
  browser_create_approval_separation: browserCreateSelfApprove.status === 403 && browserCreateTamperedApprove.status === 409 && browserCreateApprovalWorkflow?.requested_by === isolatedUserId && browserCreateApprovedWorkflow?.approved_by === isolatedApproverUserId && browserCreateApprovalWorkflow?.approval_snapshot_hash === browserCreateApprovedWorkflow?.approval_snapshot_hash,
  browser_rollback_workflow_real_api: browserRollbackDraftResponse.status() === 200 && browserRollbackPrecheckResponse.status() === 200 && browserRollbackApprovalResponse.status() === 200 && browserRollbackApproveResponse.status() === 200 && browserRollbackExecuteResponse.status() === 200 && createdWorkbench.body?.data?.deployment?.status === 'rolled_back' && createdWorkflow?.stage === 'approved' && createdWorkflow?.operation === 'rollback',
  browser_rollback_precheck_exactly_seven_fresh: validSevenItemPrecheck(browserRollbackPrecheckBody),
  browser_rollback_approval_separation: browserRollbackSelfApprove.status === 403 && browserRollbackMutationDenied.status === 409 && browserRollbackApprovalWorkflow?.requested_by === isolatedUserId && browserRollbackApprovedWorkflow?.approved_by === isolatedApproverUserId && browserRollbackApprovalWorkflow?.approval_snapshot_hash === browserRollbackApprovedWorkflow?.approval_snapshot_hash,
  persisted_workflow_restored: createWorkflowRestored && rollbackWorkflowRestored,
  durable_outbox_published: outboxSummary.total >= 5 && outboxSummary.published === outboxSummary.total && outboxSummary.dead === 0 && outboxSummary.topicOk && outboxSummary.eventIdOk && outboxSummary.statusOk,
  direct_row_action_targets_record: rowActionTargetsCreatedDeployment,
  create_modal_complete: ['部署配置', '发布编排预览', '预检查矩阵', '影响范围（预计）', '回滚策略', '权限与审批', '审计留痕'].every((label) => createModalText?.includes(label)),
  rollback_modal_complete: ['回滚配置', '回滚前检查矩阵', '影响范围', '观测窗口与回滚策略', '审批链', '审计留痕'].every((label) => rollbackModalText?.includes(label)),
  browser_health_echarts: browserHealthCanvasCount === 6,
  audit_scope_queryable: auditScope.status === 200 && JSON.stringify(auditScope.body).includes(isolatedScopeId) && JSON.stringify(auditScope.body).includes('new_scope'),
  audit_export_queryable: auditExport.status === 200 && JSON.stringify(auditExport.body).includes(selected.deployment_id) && JSON.stringify(auditExport.body).includes('bundle_checksum'),
  audit_workflow_queryable: auditWorkflow.status === 200 && JSON.stringify(auditWorkflow.body).includes(browserCreatedDeploymentId) && JSON.stringify(auditWorkflow.body).includes('approved'),
  exact_1920x1080_css_viewport: observedViewport.width === 1920 && observedViewport.height === 1080 && Math.abs(observedViewport.device_pixel_ratio - 1) < 0.01,
  browser_runtime_clean: badResponses.length === 0 && consoleErrors.length === 0 && pageErrors.length === 0,
};

assertions.cleanup_created_deployment = cleanupIsolatedFixture();

const result = { result: Object.values(assertions).every(Boolean) ? 'pass' : 'fail', browser_backend: 'Windows Chrome CDP', browser: version.Browser, viewport: { width: 1920, height: 1080, initial: initialViewport, calibrated_metrics: calibratedMetrics, observed: observedViewport }, route: '/deployments', assertions, modal_boxes: { create: createModalBox, rollback: rollbackModalBox }, api: { selected: { deployment_id: selected.deployment_id, name: selected.name, status: selected.status }, rollback_target: { deployment_id: rollbackTarget.deployment_id, name: rollbackTarget.name, status: rollbackTarget.status }, browser_created_deployment_id: browserCreatedDeploymentId, workbench_categories: Object.fromEntries(['health', 'evidence', 'change_summary', 'rollback_versions'].map((key) => [key, categoryCount(workbench, key)])), viewer_scope_status: viewerDenied.status, cross_tenant_status: crossTenantDenied.status, invalid_scope_status: invalidScope.status, requested_scope_percentage: desiredPercentage, browser_scope_percentage: browserScopePercentage, persisted_scope_percentage: Number(persistedWorkbench.body?.data?.deployment?.scope?.percentage), bundle_checksum: evidenceResponse.body?.data?.bundle_checksum, browser_download_checksum: browserDownloadedChecksum, direct_workflow_statuses: { draft: directDraft.status, precheck: directPrecheck.status, submit: directSubmit.status, self_approve: directSelfApprove.status, tampered_approve: directTamperedApprove.status, approve: directApprove.status, scope_reset: directScopeReset.status, stale_gray: directGrayWithStaleApproval.status, reapprove: directReApprove.status, gray: directGray.status, activate: directActivate.status, cross_line_rollback: directCrossLineRollback.status }, outbox: outboxSummary }, browser_deployment_responses: deploymentResponses, bad_responses: badResponses, console_errors: consoleErrors, page_errors: pageErrors, screenshots: [path.relative(root, screenshotPath), path.relative(root, createInitialScreenshotPath), path.relative(root, createScreenshotPath), path.relative(root, createWorkflowScreenshotPath), path.relative(root, rollbackInitialScreenshotPath), path.relative(root, rollbackScreenshotPath), path.relative(root, rollbackWorkflowScreenshotPath)], timestamp: new Date().toISOString() };
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
fs.writeFileSync(fullStackOutputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);

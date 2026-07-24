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
const revision = process.env.COMPLIANCE_EVIDENCE_REVISION?.trim() || 'r354';
const pageDir = path.join(root, 'evidence/ui-image-breakdowns/pages/compliance');
const drawerDir = path.join(root, 'evidence/ui-image-breakdowns/overlays/drawer-compliance-gate-detail');
const evidenceModalDir = path.join(root, 'evidence/ui-image-breakdowns/overlays/modal-compliance-evidence-package-export');
const reportModalDir = path.join(root, 'evidence/ui-image-breakdowns/overlays/modal-compliance-report-export');
for (const dir of [pageDir, drawerDir, evidenceModalDir, reportModalDir]) fs.mkdirSync(dir, { recursive: true });

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8,10.0.5.9';
const kubectl = (args) => execFileSync('kubectl', args, { encoding: 'utf8', env: process.env, timeout: 30_000 });
const secret = Buffer.from(kubectl(['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}']).trim(), 'base64').toString('utf8');
const now = Math.floor(Date.now() / 1000);
const userId = crypto.randomUUID();
const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
const claims = Buffer.from(JSON.stringify({
  iss: 'traffic-auth-service', sub: userId, jti: crypto.randomUUID(), user_id: userId,
  tenant_id: 'default', username: 'codex-compliance-ui-admin', roles: ['admin'],
  permissions: ['*', 'admin:*', 'compliance:read', 'compliance:write', 'compliance:export', 'audit:read'],
  token_type: 'access', iat: now, exp: now + 1800,
})).toString('base64url');
const signingInput = `${header}.${claims}`;
const token = `${signingInput}.${crypto.createHmac('sha256', secret).update(signingInput).digest('base64url')}`;
const route = new URL(`/compliance?windowsCdpInteractionTs=${Date.now()}`, baseUrl);
route.hash = `codex_smoke_token=${token}`;

const version = await (await fetch(`${cdpUrl}/json/version`)).json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0];
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
const cdp = await browser.newBrowserCDPSession();
const windowsDownloadDir = 'C:\\Users\\18229\\Downloads';
await cdp.send('Browser.setDownloadBehavior', {
  behavior: 'allow',
  downloadPath: windowsDownloadDir,
  eventsEnabled: true,
});
const browserDownloadProgress = new Map();
cdp.on('Browser.downloadProgress', (event) => browserDownloadProgress.set(event.guid, event));
const runtime = { bad_responses: [], console_errors: [], page_errors: [], request_failures: [], ignored_external_failures: [] };
await page.addInitScript(() => {
  window.__codexMainWorldErrors = [];
  window.addEventListener('error', (event) => {
    window.__codexMainWorldErrors.push({ type: 'error', message: event.message, filename: event.filename, line: event.lineno, column: event.colno, stack: event.error?.stack ?? '' });
  }, true);
  window.addEventListener('unhandledrejection', (event) => {
    const reason = event.reason;
    window.__codexMainWorldErrors.push({ type: 'unhandledrejection', message: reason?.message ?? String(reason), stack: reason?.stack ?? '' });
  }, true);
});
page.on('response', (response) => { if (response.status() >= 400) runtime.bad_responses.push({ status: response.status(), method: response.request().method(), url: response.url() }); });
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const item = { text: entry.text(), url: entry.location().url ?? '' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com/ip')) runtime.ignored_external_failures.push(item);
  else runtime.console_errors.push(item);
});
page.on('pageerror', (error) => {
  const item = { name: error.name, message: error.message, stack: error.stack ?? '' };
  if (item.stack.includes('chrome-extension://')) runtime.ignored_external_failures.push({ type: 'pageerror', reason: 'chrome-extension', ...item });
  else runtime.page_errors.push(item);
});
page.on('requestfailed', (request) => {
  const item = { url: request.url(), error: request.failure()?.errorText ?? 'unknown' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com/ip')) runtime.ignored_external_failures.push(item);
  else runtime.request_failures.push(item);
});

async function completeWindowsDownload({ dialog, buttonName, responsePath, localName }) {
  const downloadPromise = page.waitForEvent('download', { timeout: 20_000 });
  const browserDownloadBeginPromise = new Promise((resolve) => cdp.once('Browser.downloadWillBegin', resolve));
  const responsePromise = page.waitForResponse((response) => response.url().includes(responsePath) && response.request().method() === 'POST', { timeout: 20_000 });
  await dialog.getByRole('button', { name: buttonName }).click();
  const [download, browserDownload, response] = await Promise.all([downloadPromise, browserDownloadBeginPromise, responsePromise]);
  const responseBody = await response.json();
  const artifact = responseBody.data;
  const downloadDeadline = Date.now() + 20_000;
  while (Date.now() < downloadDeadline && !['completed', 'canceled'].includes(browserDownloadProgress.get(browserDownload.guid)?.state)) {
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  const browserDownloadResult = browserDownloadProgress.get(browserDownload.guid);
  if (browserDownloadResult?.state !== 'completed') throw new Error(`Windows Chrome download did not complete: ${JSON.stringify(browserDownloadResult)}`);
  const localPath = path.join(pageDir, localName);
  const bytes = Buffer.from(artifact.content_base64, 'base64');
  fs.writeFileSync(localPath, bytes);
  const localSha256 = crypto.createHash('sha256').update(bytes).digest('hex');
  const expectedSha256 = String(artifact.sha256).replace(/^sha256:/, '');
  if (localSha256 !== expectedSha256) throw new Error(`artifact sha mismatch: expected ${artifact.sha256}, got ${localSha256}`);
  return {
    artifact,
    localPath,
    localSha256,
    filename: download.suggestedFilename(),
    windowsPath: path.win32.join(windowsDownloadDir, browserDownload.suggestedFilename),
    state: browserDownloadResult.state,
  };
}

await page.goto(route.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-compliance').waitFor({ state: 'visible', timeout: 20_000 });
await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});

await page.getByRole('button', { name: '生成验收报告' }).click();
const reportDialog = page.getByRole('dialog', { name: '生成合规验收报告' });
await reportDialog.waitFor({ state: 'visible' });
await page.screenshot({ path: path.join(pageDir, `generate-modal-${revision}.png`) });
await reportDialog.getByRole('button', { name: '生成并写入审计' }).click();
await reportDialog.waitFor({ state: 'hidden', timeout: 20_000 });
await page.getByText(/报告 .* 已与审计记录原子提交|已按服务端真实结论保存为“证据不足”/).waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-compliance-action-result .ant-alert-close-icon').click();

const firstRow = page.locator('.taf-compliance .ant-table-row').first();
await firstRow.waitFor({ state: 'visible', timeout: 10_000 });
await firstRow.click();
const drawer = page.getByRole('dialog', { name: '合规门禁详情' });
await drawer.waitFor({ state: 'visible' });
await page.screenshot({ path: path.join(drawerDir, `implementation-${revision}.png`) });
await drawer.locator('.ant-drawer-close').click();

await page.getByRole('button', { name: '导出证据包' }).click();
const evidenceDialog = page.getByRole('dialog', { name: '导出合规证据包' });
await evidenceDialog.waitFor({ state: 'visible' });
await evidenceDialog.getByRole('button', { name: '预检证据' }).click();
await evidenceDialog.getByText(/已预检：报告 .* 个门禁/).waitFor({ state: 'visible' });
await page.screenshot({ path: path.join(evidenceModalDir, `implementation-${revision}.png`) });
const evidenceDownload = await completeWindowsDownload({ dialog: evidenceDialog, buttonName: '生成 ZIP 并下载', responsePath: '/evidence-package', localName: `compliance-evidence-${revision}.zip` });
const evidenceManifest = JSON.parse(execFileSync('unzip', ['-p', evidenceDownload.localPath, 'manifest.json'], { encoding: 'utf8' }));
const evidenceReport = execFileSync('unzip', ['-p', evidenceDownload.localPath, 'report.json']);
if (evidenceManifest.report_sha256 !== `sha256:${crypto.createHash('sha256').update(evidenceReport).digest('hex')}`) throw new Error('evidence manifest report_sha256 does not match report.json');
await evidenceDialog.waitFor({ state: 'hidden', timeout: 15_000 });
const exportMessage = page.getByText(/证据包 .* 已由服务端生成、审计并下载；sha256:/);
await exportMessage.waitFor({ state: 'visible', timeout: 15_000 });
const exportText = await exportMessage.textContent();
await page.locator('.taf-compliance-action-result .ant-alert-close-icon').click();

await page.getByRole('button', { name: '导出 PDF' }).click();
const reportExportDialog = page.getByRole('dialog', { name: '导出运行报告' });
await reportExportDialog.waitFor({ state: 'visible' });
await reportExportDialog.getByRole('button', { name: '预览报告' }).click();
await reportExportDialog.locator('.taf-compliance-inline-preview').waitFor({ state: 'visible' });
await page.screenshot({ path: path.join(reportModalDir, `implementation-${revision}.png`) });
const pdfDownload = await completeWindowsDownload({ dialog: reportExportDialog, buttonName: '导出 PDF 运行报告', responsePath: '/export', localName: `compliance-report-${revision}.pdf` });
const pdfBytes = fs.readFileSync(pdfDownload.localPath);
if (!pdfBytes.includes(Buffer.from('Sections:')) || !pdfBytes.includes(Buffer.from('Audit trail:')) || !pdfBytes.includes(Buffer.from('COMPLIANCE_REPORT_GENERATED'))) throw new Error('PDF is missing report sections or audit trail');
await reportExportDialog.waitFor({ state: 'hidden', timeout: 15_000 });
await page.getByText(/PDF 运行报告 .* 已由服务端渲染、审计并下载/).waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-compliance-action-result .ant-alert-close-icon').click();

await page.getByRole('button', { name: '导出 Word' }).click();
await reportExportDialog.waitFor({ state: 'visible' });
const wordDownload = await completeWindowsDownload({ dialog: reportExportDialog, buttonName: '导出 Word 运行报告', responsePath: '/export', localName: `compliance-report-${revision}.docx` });
const wordDocument = execFileSync('unzip', ['-p', wordDownload.localPath, 'word/document.xml']);
if (!wordDocument.includes(Buffer.from('alert_response')) || !wordDocument.includes(Buffer.from('COMPLIANCE_REPORT_GENERATED'))) throw new Error('DOCX is missing report sections or audit trail');
await reportExportDialog.waitFor({ state: 'hidden', timeout: 15_000 });
await page.locator('.taf-compliance-action-result .ant-alert-close-icon').click();

await page.getByRole('button', { name: '创建整改任务' }).click();
const remediationDialog = page.getByRole('dialog', { name: '创建整改任务' });
await remediationDialog.waitFor({ state: 'visible' });
await remediationDialog.getByRole('button', { name: '持久化整改任务' }).click();
await remediationDialog.waitFor({ state: 'hidden', timeout: 15_000 });
await page.getByText(/整改任务已持久化：新建 .* 条，复用 .* 条，共 .* 条/).waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-compliance-action-result .ant-alert-close-icon').click();

const currentReportResponse = await page.request.get(`${baseUrl}/api/v1/compliance/reports?limit=1`, { headers: { Authorization: `Bearer ${token}`, 'X-Tenant-ID': 'default' } });
const currentReport = (await currentReportResponse.json()).data?.reports?.[0];
const remediationRepeatResponse = await page.request.post(`${baseUrl}/api/v1/compliance/reports/${encodeURIComponent(currentReport.report_id)}/remediations`, { headers: { Authorization: `Bearer ${token}`, 'X-Tenant-ID': 'default' }, data: {} });
const remediationRepeat = (await remediationRepeatResponse.json()).data;
if (remediationRepeatResponse.status() !== 200 || remediationRepeat.created !== 0 || remediationRepeat.reused !== remediationRepeat.total) throw new Error(`repeat remediation was not idempotent: ${JSON.stringify(remediationRepeat)}`);

await page.getByRole('button', { name: '固化验收记录' }).click();
const finalizeDialog = page.getByRole('dialog', { name: '固化验收记录' });
await finalizeDialog.waitFor({ state: 'visible' });
const finalizeResponsePromise = page.waitForResponse((response) => response.url().includes('/finalize') && response.request().method() === 'POST' && response.status() === 200, { timeout: 20_000 });
await finalizeDialog.getByRole('button', { name: '创建不可变快照' }).click();
const finalizeResponse = await finalizeResponsePromise;
const finalization = (await finalizeResponse.json()).data;
if (finalization.report_sha256 !== evidenceManifest.report_sha256) throw new Error(`canonical report hash mismatch: evidence=${evidenceManifest.report_sha256} finalization=${finalization.report_sha256}`);
await finalizeDialog.waitFor({ state: 'hidden', timeout: 15_000 });
await page.getByText(/验收记录 .* 已固化；sha256:/).waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-compliance-action-result .ant-alert-close-icon').click();

await page.screenshot({ path: path.join(pageDir, `implementation-${revision}.png`) });
const reportResponse = await page.request.get(`${baseUrl}/api/v1/compliance/reports?limit=5`, { headers: { Authorization: `Bearer ${token}`, 'X-Tenant-ID': 'default' } });
const reportBody = await reportResponse.json();
const latest = reportBody.data?.reports?.[0];
if (!latest?.report_id) throw new Error(`latest report missing: ${JSON.stringify(reportBody)}`);
const conflictResponse = await page.request.post(`${baseUrl}/api/v1/compliance/reports/${encodeURIComponent(latest.report_id)}/finalize`, { headers: { Authorization: `Bearer ${token}`, 'X-Tenant-ID': 'default' }, data: {} });
if (conflictResponse.status() !== 409) throw new Error(`repeat finalization must return 409, got ${conflictResponse.status()} ${await conflictResponse.text()}`);
const auditResponse = await page.request.get(`${baseUrl}/api/v1/compliance/audit-trail?object_id=${encodeURIComponent(latest.report_id)}&limit=20`, { headers: { Authorization: `Bearer ${token}`, 'X-Tenant-ID': 'default' } });
const auditBody = await auditResponse.json();
const actions = (auditBody.data?.trails ?? []).map((item) => item.action);
for (const action of ['COMPLIANCE_REPORT_GENERATED', 'COMPLIANCE_EVIDENCE_EXPORTED', 'COMPLIANCE_REPORT_EXPORTED', 'COMPLIANCE_REMEDIATIONS_CREATED', 'COMPLIANCE_REPORT_FINALIZED']) if (!actions.includes(action)) throw new Error(`missing audit action ${action}`);

await page.waitForTimeout(500);
runtime.main_world_errors = await page.evaluate(() => window.__codexMainWorldErrors ?? []);
runtime.page_errors = runtime.page_errors.filter((error) => {
  if (error.message === 'Object' && !error.stack && runtime.main_world_errors.length === 0) {
    runtime.ignored_external_failures.push({ type: 'pageerror', reason: 'isolated-world-with-no-main-world-error', ...error });
    return false;
  }
  return true;
});
const businessErrors = runtime.bad_responses.length + runtime.console_errors.length + runtime.page_errors.length + runtime.request_failures.length;
const result = {
  result: businessErrors ? 'blocked' : 'pass', revision, generated_at: new Date().toISOString(),
  browser: version.Browser, backend: 'windows-chrome-cdp-xshell-tunnel', route: '/compliance', viewport: { width: 1920, height: 1080 },
  workflow: { report_id: latest.report_id, report_status: latest.status, generated_by: latest.generated_by, audit_actions: actions, evidence_download: path.relative(root, evidenceDownload.localPath), evidence_filename: evidenceDownload.filename, evidence_sha256: evidenceDownload.localSha256, canonical_report_sha256: evidenceManifest.report_sha256, finalization_report_sha256: finalization.report_sha256, windows_download_path: evidenceDownload.windowsPath, windows_download_state: evidenceDownload.state, evidence_result: exportText, pdf_download: path.relative(root, pdfDownload.localPath), pdf_sha256: pdfDownload.localSha256, word_download: path.relative(root, wordDownload.localPath), word_sha256: wordDownload.localSha256, repeat_remediation: { created: remediationRepeat.created, reused: remediationRepeat.reused, total: remediationRepeat.total }, repeat_finalization_status: conflictResponse.status() },
  ui: { report_modal: true, real_gate_drawer: true, evidence_modal: true, enabled_server_actions: ['pdf', 'word', 'remediation', 'finalize'], chart_canvases: await page.locator('canvas').count() },
  runtime,
  screenshots: {
    main: `evidence/ui-image-breakdowns/pages/compliance/implementation-${revision}.png`,
    report_modal: `evidence/ui-image-breakdowns/overlays/modal-compliance-report-export/implementation-${revision}.png`,
    evidence_modal: `evidence/ui-image-breakdowns/overlays/modal-compliance-evidence-package-export/implementation-${revision}.png`,
    drawer: `evidence/ui-image-breakdowns/overlays/drawer-compliance-gate-detail/implementation-${revision}.png`,
  },
};
fs.writeFileSync(path.join(pageDir, `interaction-${revision}.json`), `${JSON.stringify(result, null, 2)}\n`);
await page.close();
console.log(JSON.stringify(result, null, 2));
process.exit(businessErrors ? 1 : 0);

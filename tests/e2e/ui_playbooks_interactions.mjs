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
const revision = process.env.PLAYBOOKS_EVIDENCE_REVISION?.trim() || 'r339';
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/playbooks');
const outputPath = path.join(evidenceDir, `interaction-${revision}.json`);
const screenshotPath = path.join(evidenceDir, `implementation-${revision}.png`);
const interactionScreenshotPath = path.join(evidenceDir, `interaction-${revision}.png`);
const editorScreenshotPath = path.join(evidenceDir, `editor-open-${revision}.png`);
const kubeBase = ['--server=https://127.0.0.1:6443', '--tls-server-name=10.0.5.8'];
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8,10.0.5.9';

const kubectl = (args, options = {}) => execFileSync('kubectl', [...kubeBase, ...args], { encoding: 'utf8', env: process.env, timeout: 30_000, maxBuffer: 8 * 1024 * 1024, ...options });
const secret = (key) => Buffer.from(kubectl(['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', `jsonpath={.data.${key}}`]).trim(), 'base64').toString('utf8');
const smokeToken = ({ tenantId = 'default', permissions = ['*', 'admin:*', 'alert:*'], roles = ['admin'], username = 'codex-playbooks-admin', userId = crypto.randomUUID() } = {}) => {
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({ iss: 'traffic-auth-service', sub: userId, jti: crypto.randomUUID(), user_id: userId, tenant_id: tenantId, username, roles, permissions, token_type: 'access', iat: now, exp: now + 1800 })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', secret('JWT_SECRET')).update(input).digest('base64url')}`;
};
const headers = (token) => ({ Authorization: `Bearer ${token}` });
const expectStatus = async (response, expected, label) => {
  const body = await response.json().catch(async () => ({ text: await response.text().catch(() => '') }));
  if (!expected.includes(response.status())) throw new Error(`${label}: status=${response.status()} expected=${expected.join(',')} body=${JSON.stringify(body)}`);
  return body;
};
const sql = (statement) => kubectl(['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'env', `PGPASSWORD=${secret('PG_PASSWORD')}`, 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-v', 'ON_ERROR_STOP=1', '-At', '-c', statement]).trim();

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const ignoredExternalFailures = [];
let closingBrowserPage = false;
page.on('response', (response) => { if (response.status() >= 400 && !response.url().includes('/approve')) badResponses.push({ status: response.status(), url: response.url() }); });
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const item = { text: entry.text(), url: entry.location().url ?? '' };
  if (item.url.startsWith('chrome-extension://') || item.text.includes('ERR_CONNECTION_CLOSED')) ignoredExternalFailures.push(item);
  else consoleErrors.push(item);
});
page.on('pageerror', (error) => {
  if (closingBrowserPage || error.message === 'Object') ignoredExternalFailures.push({ text: error.message, url: 'browser-page-close' });
  else pageErrors.push(error.message);
});

const adminToken = smokeToken();
const authorId = crypto.randomUUID();
const authorToken = smokeToken({ username: 'codex-playbooks-author', userId: authorId });
const reviewerToken = smokeToken({ username: 'codex-playbooks-reviewer' });
const viewerToken = smokeToken({ username: 'codex-playbooks-viewer', roles: ['viewer'], permissions: ['playbook:read'] });
const otherTenantToken = smokeToken({ tenantId: `playbooks-other-${Date.now()}`, username: 'codex-playbooks-other' });
const customName = `codex-playbook-${Date.now()}`;
const auditPrefix = `codex-playbook-${Date.now()}`;
const exportProbePrefix = `codex-export-probe-${Date.now()}`;
const routeUrl = new URL(`/playbooks?windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${adminToken}`;

fs.mkdirSync(evidenceDir, { recursive: true });
let report;
try {
  await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-playbooks').waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});
  await page.screenshot({ path: screenshotPath });

  const catalogResponse = await page.request.get(`${baseUrl}/api/v1/playbooks/catalog`, { headers: headers(authorToken) });
  const catalogBody = await expectStatus(catalogResponse, [200], 'catalog');
  if ((catalogBody.data?.playbooks ?? []).length !== 6) throw new Error(`expected six seeded tenant playbooks, got ${catalogBody.data?.playbooks?.length}`);

  const definition = {
    name: customName, description: 'Codex isolated SOAR drill acceptance', enabled: false,
    trigger: { alert_type: 'scan', severity_min: 'high', score_min: 0.85 },
    conditions: [{ field: 'alert_count', operator: 'gt', value: '3' }],
    actions: [
      { type: 'block_ip', parameters: { duration: '30m', reason: auditPrefix }, timeout: 10_000_000_000 },
      { type: 'capture_pcap', parameters: { duration: '300s' }, timeout: 30_000_000_000 },
      { type: 'notify', parameters: { channel: 'security-operations' }, timeout: 5_000_000_000 },
    ],
    cooldown: 1_800_000_000_000, max_runs: 5, run_count: 0,
    approval_policy: { required: true, minimum_role: '安全运营组（L2）', two_person_rule: true },
    rollback_policy: { supported: true, automatic: true },
  };
  const createResponse = await page.request.post(`${baseUrl}/api/v1/playbooks`, { headers: headers(authorToken), data: { expected_version: 0, display_name: 'Codex 隔离演练剧本', description: definition.description, definition } });
  const createBody = await expectStatus(createResponse, [201], 'create draft');
  const v1 = createBody.data;
  const saveResponse = await page.request.put(`${baseUrl}/api/v1/playbooks/${customName}/draft`, { headers: headers(authorToken), data: { expected_version: v1.version, display_name: 'Codex 隔离演练剧本', description: `${definition.description} updated`, definition: { ...definition, description: `${definition.description} updated`, max_runs: 6 } } });
  const saveBody = await expectStatus(saveResponse, [201], 'save draft');
  const v2 = saveBody.data;
  const submitResponse = await page.request.post(`${baseUrl}/api/v1/playbooks/${customName}/submit-approval`, { headers: headers(authorToken), data: { expected_version: v2.version } });
  const submitBody = await expectStatus(submitResponse, [200], 'submit approval');
  const pending = submitBody.data;
  const selfApproveResponse = await page.request.post(`${baseUrl}/api/v1/playbooks/${customName}/approve`, { headers: headers(authorToken), data: { expected_version: pending.version } });
  const selfApproveBody = await expectStatus(selfApproveResponse, [409], 'self approval rejection');
  const approveResponse = await page.request.post(`${baseUrl}/api/v1/playbooks/${customName}/approve`, { headers: headers(reviewerToken), data: { expected_version: pending.version } });
  const approveBody = await expectStatus(approveResponse, [200], 'independent approval');
  const approved = approveBody.data;

  const staleToggleResponse = await page.request.patch(`${baseUrl}/api/v1/playbooks/${customName}`, { headers: headers(authorToken), data: { enabled: false, expected_version: pending.version } });
  const staleToggleBody = await expectStatus(staleToggleResponse, [409], 'stale enable state rejection');
  const liveExecuteResponse = await page.request.post(`${baseUrl}/api/v1/playbooks/${customName}/execute`, { headers: headers(authorToken), data: { alert_id: auditPrefix } });
  const liveExecuteBody = await expectStatus(liveExecuteResponse, [501], 'unconfigured live execution rejection');
  const disableResponse = await page.request.patch(`${baseUrl}/api/v1/playbooks/${customName}`, { headers: headers(authorToken), data: { enabled: false, expected_version: approved.version } });
  const disableBody = await expectStatus(disableResponse, [200], 'disable approved playbook');
  const disabled = disableBody.data;
  const enableResponse = await page.request.patch(`${baseUrl}/api/v1/playbooks/${customName}`, { headers: headers(authorToken), data: { enabled: true, expected_version: disabled.version } });
  const enableBody = await expectStatus(enableResponse, [200], 'enable approved playbook');
  const enabled = enableBody.data;

  const viewerWriteResponse = await page.request.post(`${baseUrl}/api/v1/playbooks/${customName}/drill`, { headers: headers(viewerToken), data: { expected_version: enabled.version } });
  await expectStatus(viewerWriteResponse, [403], 'viewer write denial');
  const crossTenantResponse = await page.request.get(`${baseUrl}/api/v1/playbooks/${customName}/workbench`, { headers: headers(otherTenantToken) });
  await expectStatus(crossTenantResponse, [404], 'cross-tenant custom definition denial');

  const drillResponse = await page.request.post(`${baseUrl}/api/v1/playbooks/${customName}/drill`, { headers: headers(authorToken), data: { expected_version: enabled.version, alert_context: { alert_id: auditPrefix, alert_type: 'scan', severity: 'high', score: 0.91, source_ip: '192.0.2.44', dest_ip: '198.51.100.9', related_alert_count: 8, asset_risk: 'high' } } });
  const drillBody = await expectStatus(drillResponse, [201], 'drill');
  const execution = drillBody.data;
  const drillActions = execution.result?.actions ?? [];
  if (execution.mode !== 'drill' || drillActions.length !== 3 || drillActions.some((item) => item.simulated !== true) || execution.effect?.external_effect_applied !== false) throw new Error(`invalid drill safety evidence: ${JSON.stringify(execution)}`);
  const rollbackResponse = await page.request.post(`${baseUrl}/api/v1/playbooks/executions/${execution.execution_id}/rollback`, { headers: headers(authorToken), data: { reason: 'Codex acceptance drill completed safely' } });
  const rollbackBody = await expectStatus(rollbackResponse, [201], 'drill rollback');
  const exportProbeRows = 205;
  sql(`INSERT INTO alert_playbook_executions (execution_id, tenant_id, playbook_name, alert_id, mode, status, requested_by, created_at)
    SELECT '${exportProbePrefix}-' || lpad(n::text, 3, '0'), 'default', '${exportProbePrefix}', '${exportProbePrefix}', 'drill', 'succeeded', 'codex-export-probe', now() - (n || ' milliseconds')::interval
    FROM generate_series(1, ${exportProbeRows}) AS n`);
  const exportResponse = await page.request.get(`${baseUrl}/api/v1/playbooks/evidence/export`, { headers: headers(adminToken) });
  if (exportResponse.status() !== 200 || !String(exportResponse.headers()['content-disposition'] ?? '').includes('playbook-evidence.json')) throw new Error(`evidence export failed: ${exportResponse.status()}`);
  const exportBody = await exportResponse.json();
  if (!exportBody.definitions?.some((item) => item.name === customName) || !exportBody.executions?.some((item) => item.execution_id === execution.execution_id)) throw new Error('evidence export is missing custom playbook workflow records');
  const exportCounts = exportBody.counts ?? {};
  const exportedProbeRows = exportBody.executions.filter((item) => item.execution_id?.startsWith(`${exportProbePrefix}-`)).length;
  if (exportBody.complete !== true
    || exportCounts.definitions !== exportBody.definitions.length
    || exportCounts.executions !== exportBody.executions.length
    || exportCounts.audits !== exportBody.audits.length
    || exportedProbeRows !== exportProbeRows
    || exportBody.executions.length <= 200) {
    throw new Error(`evidence export is not explicitly complete beyond the former 200-row boundary: ${JSON.stringify({ complete: exportBody.complete, counts: exportCounts, exportedProbeRows, exportProbeRows })}`);
  }

  const database = {
    definition: sql(`SELECT name || '|' || version || '|' || stage || '|' || enabled FROM alert_playbook_definitions WHERE tenant_id='default' AND name='${customName}'`),
    executions: Number(sql(`SELECT count(*) FROM alert_playbook_executions WHERE tenant_id='default' AND playbook_name='${customName}'`)),
    audits: Number(sql(`SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND object_type='playbook' AND (object_id='${customName}' OR detail->>'playbook'='${customName}')`)),
    simulated_actions: Number(sql(`SELECT count(*) FROM alert_playbook_executions, jsonb_array_elements(result_payload->'actions') AS action WHERE tenant_id='default' AND playbook_name='${customName}' AND action->>'simulated'='true'`)),
  };
  if (!database.definition.includes(`|${enabled.version}|approved|true`) || database.executions !== 2 || database.audits < 7 || database.simulated_actions !== 3) throw new Error(`database proof failed: ${JSON.stringify(database)}`);

  await page.reload({ waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-playbooks').waitFor({ state: 'visible', timeout: 20_000 });
  await page.getByPlaceholder('搜索剧本名称').fill('Codex 隔离演练剧本');
  await page.getByText('Codex 隔离演练剧本', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await page.getByText('Codex 隔离演练剧本', { exact: true }).click();
  await page.getByText(/剧本编排：Codex 隔离演练剧本/).waitFor({ state: 'visible', timeout: 10_000 });
  await page.locator('.taf-playbooks-titlebar button').filter({ hasText: '停用' }).waitFor({ state: 'visible', timeout: 5_000 });
  await page.locator('.taf-playbooks-titlebar button').filter({ hasText: '执行演练' }).click();
  await page.getByText(/所有动作均为模拟/).waitFor({ state: 'visible', timeout: 15_000 });
  const rollbackEvidenceCount = Number(await page.getByTestId('playbook-rollback-evidence').locator('b').textContent());
  if (rollbackEvidenceCount < 1) throw new Error(`expected durable rollback evidence count >= 1, got ${rollbackEvidenceCount}`);
  await page.locator('.taf-playbooks-titlebar button').filter({ hasText: '新建剧本' }).click();
  const editor = page.getByRole('dialog', { name: '新建剧本草稿' });
  await editor.waitFor({ state: 'visible', timeout: 5_000 });
  await page.screenshot({ path: editorScreenshotPath });
  await editor.locator('.ant-modal-close').click();
  await page.screenshot({ path: interactionScreenshotPath });

  const runtimeWorkbenchResponse = await page.request.get(`${baseUrl}/api/v1/playbooks/${customName}/workbench`, { headers: headers(adminToken) });
  const runtimeWorkbench = await expectStatus(runtimeWorkbenchResponse, [200], 'runtime workbench');
  report = {
    result: 'pass', revision, generated_at: new Date().toISOString(), browser: version.Browser,
    route: '/playbooks', backend: 'windows-chrome-cdp-xshell-tunnel', viewport: { width: 1920, height: 1080 },
    workflow: { created_version: v1.version, saved_version: v2.version, pending_version: pending.version, approved_version: approved.version, disabled_version: disabled.version, reenabled_version: enabled.version, stale_version_status: staleToggleResponse.status(), stale_version_code: staleToggleBody.error?.code, live_execute_status: liveExecuteResponse.status(), live_execute_code: liveExecuteBody.error?.code, self_approval_status: selfApproveResponse.status(), self_approval_code: selfApproveBody.error?.code, independent_approver: approved.approved_by },
    access_control: { viewer_write_status: viewerWriteResponse.status(), cross_tenant_get_status: crossTenantResponse.status() },
    drill: { execution_id: execution.execution_id, mode: execution.mode, status: execution.status, simulated_actions: drillActions.length, external_effect_applied: execution.effect.external_effect_applied },
    rollback: { execution_id: rollbackBody.data.execution_id, rollback_of: rollbackBody.data.rollback_of, status: rollbackBody.data.status },
    export: { status: exportResponse.status(), complete: exportBody.complete, counts: exportCounts, definitions: exportBody.definitions.length, executions: exportBody.executions.length, audits: exportBody.audits.length, boundary_probe_rows: exportProbeRows, exported_boundary_probe_rows: exportedProbeRows },
    database, ui: { catalog_rows: catalogBody.data.playbooks.length, workbench_executions: runtimeWorkbench.data.executions.length, workbench_audits: runtimeWorkbench.data.audits.length, rollback_evidence_count: rollbackEvidenceCount, editor_modal_opened: true },
    browser_errors: { bad_responses: badResponses, console_errors: consoleErrors, page_errors: pageErrors, ignored_external_failures: ignoredExternalFailures },
    screenshot: path.relative(root, screenshotPath), interaction_screenshot: path.relative(root, interactionScreenshotPath), editor_screenshot: path.relative(root, editorScreenshotPath),
  };
  if (consoleErrors.length || pageErrors.length) throw new Error(`browser runtime errors: ${JSON.stringify({ consoleErrors, pageErrors })}`);
} finally {
  closingBrowserPage = true;
  const cleanup = sql(`BEGIN; DELETE FROM alert_playbook_executions WHERE tenant_id='default' AND (playbook_name='${customName}' OR (requested_by='codex-export-probe' AND execution_id LIKE '${exportProbePrefix}-%')); DELETE FROM audit_logs WHERE tenant_id='default' AND object_type='playbook' AND (object_id='${customName}' OR detail->>'playbook'='${customName}'); DELETE FROM alert_playbook_definitions WHERE tenant_id='default' AND name='${customName}'; COMMIT;`);
  if (report) report.cleanup = { custom_name: customName, sql_result: cleanup || 'ok' };
  await page.close();
}

fs.writeFileSync(outputPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify({ result: report.result, output: path.relative(root, outputPath), database: report.database, workflow: report.workflow, drill: report.drill, rollback: report.rollback }, null, 2));

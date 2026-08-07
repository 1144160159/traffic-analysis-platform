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
const runId = process.env.RUN_ID || '20260805-remediation-dashboard-read-task-v1';
const outputDir = path.join(root, 'doc/02_acceptance/runs', runId);
const outputPath = path.join(outputDir, 'windows-chrome-dashboard.json');
const screenshotPath = path.join(outputDir, 'windows-chrome-dashboard.png');

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
  const encode = (value) => Buffer.from(JSON.stringify(value)).toString('base64url');
  const header = encode({ alg: 'HS256', typ: 'JWT' });
  const claims = encode({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-dashboard-windows-g5',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:read', 'dashboard:read', 'dashboard:write', 'compliance:write'],
    token_type: 'access',
    session_id: `${runId}-${crypto.randomUUID()}`,
    iat: now,
    exp: now + 1800,
  });
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encodedSecret, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

function deploymentCandidate(name) {
  const raw = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'deployment', name, '-o', 'json'],
    { encoding: 'utf8', env: process.env, timeout: 15_000 },
  );
  const deployment = JSON.parse(raw);
  const container = deployment.spec.template.spec.containers[0];
  const annotations = deployment.spec.template.metadata.annotations || {};
  return {
    image: container.image,
    image_id: annotations['traffic.analysis/image-id'] || '',
    image_manifest_digest: annotations['traffic.analysis/image-manifest-digest'] || annotations['traffic.analysis/image-digest'] || '',
    source_sha256: annotations['traffic.analysis/source-sha256'] || annotations['traffic.analysis/source-digest'] || '',
    build_id: annotations['traffic.analysis/build-id'] || '',
    ready_replicas: deployment.status.readyReplicas || 0,
    replicas: deployment.status.replicas || 0,
  };
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP unavailable: ${versionResponse.status}`);
const version = await versionResponse.json();
const candidates = {
  web_ui: deploymentCandidate('web-ui'),
  alert_service: deploymentCandidate('alert-service'),
};
fs.mkdirSync(outputDir, { recursive: true });

const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const badResponses = [];
const requestFailures = [];
const consoleErrors = [];
const thirdPartyConsoleErrors = [];
const pageErrors = [];
page.on('response', (response) => {
  if (response.url().startsWith(`${baseUrl}/api/`) && response.status() >= 400) {
    badResponses.push({ method: response.request().method(), status: response.status(), url: response.url() });
  }
});
page.on('requestfailed', (request) => {
  if (request.url().startsWith(baseUrl)) {
    requestFailures.push({ url: request.url(), error: request.failure()?.errorText || 'unknown' });
  }
});
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const record = { text: entry.text(), location: entry.location() };
  if (record.location.url.startsWith(baseUrl)) consoleErrors.push(record);
  else thirdPartyConsoleErrors.push(record);
});
page.on('pageerror', (error) => pageErrors.push(error.message));

const route = new URL(`/dashboard?windowsDashboardAcceptance=${Date.now()}`, baseUrl);
route.hash = `codex_smoke_token=${createToken()}`;
const snapshotResponsePromise = page.waitForResponse(
  (response) => response.request().method() === 'GET' && response.url().includes('/api/v1/dashboard/snapshot'),
  { timeout: 45_000 },
);
await page.goto(route.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
const snapshotResponse = await snapshotResponsePromise;
const snapshotPayload = await snapshotResponse.json();
await page.locator('.taf-dashboard-workbench').waitFor({ state: 'visible', timeout: 20_000 });
await page.locator('.taf-dashboard-kpis .taf-dashboard-kpi').first().waitFor({ state: 'visible', timeout: 20_000 });

const partialAlert = page.locator('.ant-alert-warning').filter({ hasText: '统一快照部分可用' }).first();
await partialAlert.waitFor({ state: 'visible', timeout: 10_000 });
const partialText = (await partialAlert.innerText()).trim();
const metricValue = async (label) => {
  const card = page.locator('.taf-dashboard-kpi').filter({ hasText: label }).first();
  await card.waitFor({ state: 'visible', timeout: 10_000 });
  return (await card.locator('strong').innerText()).trim();
};
const deficit = (label) => page.locator('.taf-deficit-item').filter({ hasText: label }).first();
const auditMetric = await metricValue('审计留痕缺口');
const complianceMetric = await metricValue('合规门禁缺口');
const auditDeficit = deficit('审计留痕缺口');
const complianceDeficit = deficit('合规门禁缺口');
const auditButton = auditDeficit.locator('button');
const complianceButton = complianceDeficit.locator('button');
const auditDisabled = await auditButton.isDisabled();
const complianceDisabled = await complianceButton.isDisabled();
const auditContext = (await auditDeficit.locator('em').innerText()).trim();
const complianceContext = (await complianceDeficit.locator('em').innerText()).trim();
const syntheticYesterdayCount = await page.getByText(/较昨日\s*[+-]\d+/u).count();

const taskPostPromise = page.waitForResponse(
  (response) => response.request().method() === 'POST' && response.url().includes('/api/v1/dashboard/tasks/compliance'),
  { timeout: 20_000 },
);
const taskStatusPromise = page.waitForResponse(
  (response) => response.request().method() === 'GET' && /\/api\/v1\/dashboard\/tasks\/[^/?]+(?:\?|$)/u.test(response.url()),
  { timeout: 20_000 },
);
await complianceButton.click();
const drawer = page.locator('.taf-dashboard-action-drawer:visible');
await drawer.waitFor({ state: 'visible', timeout: 10_000 });
const endpointVisible = await drawer.getByText('/v1/dashboard/tasks/compliance', { exact: true }).isVisible();
const auditEventVisible = await drawer.getByText('DASHBOARD_COMPLIANCE_TASK_CREATED', { exact: true }).isVisible();
await drawer.getByRole('button', { name: '确认提交' }).click();
const taskPostResponse = await taskPostPromise;
const taskPostPayload = await taskPostResponse.json();
const taskStatusResponse = await taskStatusPromise;
const taskStatusPayload = await taskStatusResponse.json();
const acceptedAlert = drawer.locator('.ant-alert-info').filter({ hasText: '任务已受理，尚未最终完成' });
await acceptedAlert.waitFor({ state: 'visible', timeout: 10_000 });
const acceptedText = (await acceptedAlert.innerText()).trim();

const candidateAssets = await page.evaluate(() => performance.getEntriesByType('resource')
  .map((entry) => entry.name)
  .filter((name) => name.includes('/assets/') && (name.includes('DashboardOperationsPage-') || /\/index-[^/]+\.js$/u.test(name))));
await page.screenshot({ path: screenshotPath, fullPage: true });

const receipt = taskPostPayload?.data || {};
const task = taskStatusPayload?.data || {};
const snapshotMeta = snapshotPayload?.meta || {};
const result = {
  result: snapshotResponse.status() === 200
    && snapshotPayload?.success === true
    && snapshotMeta.partial === true
    && Array.isArray(snapshotMeta.missing_sections)
    && snapshotMeta.missing_sections.includes('reconciliation.alerts_projection')
    && partialText.includes('reconciliation.alerts_projection')
    && auditMetric === '0 项'
    && complianceMetric === '78 项'
    && auditDisabled
    && !complianceDisabled
    && auditContext.includes('dashboard_tasks')
    && complianceContext.includes('compliance_remediation_tasks')
    && syntheticYesterdayCount === 0
    && endpointVisible
    && auditEventVisible
    && taskPostResponse.status() === 202
    && receipt.status === 'accepted'
    && task.status === 'accepted'
    && receipt.task_id === task.task_id
    && receipt.trace_id === task.trace_id
    && receipt.action_id === 'dashboard-compliance-task-create'
    && Boolean(receipt.event_id)
    && Boolean(receipt.snapshot_id)
    && acceptedText.includes(receipt.task_id)
    && candidateAssets.some((asset) => asset.includes('DashboardOperationsPage-'))
    && badResponses.length === 0
    && requestFailures.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0
    ? 'pass'
    : 'fail',
  browser_backend: 'Windows Chrome over Xshell CDP tunnel',
  browser: version.Browser,
  protocol_version: version['Protocol-Version'],
  viewport: { width: 1920, height: 1080 },
  route: route.toString().replace(/codex_smoke_token=[^&#]+/u, 'codex_smoke_token=<redacted>'),
  candidates,
  candidate_assets: candidateAssets,
  snapshot: {
    http_status: snapshotResponse.status(),
    contract_version: snapshotMeta.contract_version,
    snapshot_id: snapshotMeta.snapshot_id,
    trace_id: snapshotMeta.trace_id,
    as_of: snapshotMeta.as_of,
    partial: snapshotMeta.partial,
    missing_sections: snapshotMeta.missing_sections,
    source_watermarks: snapshotMeta.source_watermarks,
  },
  ui_assertions: {
    partial_text: partialText,
    audit_metric: auditMetric,
    compliance_metric: complianceMetric,
    audit_action_disabled: auditDisabled,
    compliance_action_disabled: complianceDisabled,
    audit_context: auditContext,
    compliance_context: complianceContext,
    synthetic_yesterday_count: syntheticYesterdayCount,
    endpoint_visible: endpointVisible,
    audit_event_visible: auditEventVisible,
    accepted_alert: acceptedText,
  },
  task_receipt: {
    http_status: taskPostResponse.status(),
    task_id: receipt.task_id,
    job_id: receipt.job_id,
    event_id: receipt.event_id,
    action_id: receipt.action_id,
    status: receipt.status,
    revision: receipt.revision,
    snapshot_id: receipt.snapshot_id,
    trace_id: receipt.trace_id,
    idempotency_key: receipt.idempotency_key,
    outbox_status: receipt.outbox_status,
    replayed: receipt.replayed,
  },
  task_status: {
    http_status: taskStatusResponse.status(),
    task_id: task.task_id,
    action_id: task.action_id,
    status: task.status,
    revision: task.revision,
    trace_id: task.trace_id,
  },
  bad_responses: badResponses,
  request_failures: requestFailures,
  console_errors: consoleErrors,
  third_party_console_errors: thirdPartyConsoleErrors,
  page_errors: pageErrors,
  screenshot: path.relative(root, screenshotPath),
  timestamp: new Date().toISOString(),
};

fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);

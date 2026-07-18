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
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/mlops');
const revision = process.env.MLOPS_EVIDENCE_REVISION?.trim() || 'r331';
const retryTerminalTimeoutMs = Number(process.env.MLOPS_RETRY_TERMINAL_TIMEOUT_MS || 360_000);
const outputPath = path.join(evidenceDir, `interaction-${revision}.json`);
const screenshotPath = path.join(evidenceDir, `interaction-${revision}.png`);
const implementationPath = path.join(evidenceDir, `implementation-${revision}.png`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken({ tenantId = 'default', permissions = ['*', 'admin:*', 'model:*'], roles = ['admin'], username = 'codex-windows-cdp-admin' } = {}) {
  const encoded = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1_000);
  const userId = crypto.randomUUID();
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: userId, jti: crypto.randomUUID(), user_id: userId, tenant_id: tenantId,
    username, roles, permissions, token_type: 'access', iat: now, exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const delay = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds));
const authHeaders = (token) => ({ Authorization: `Bearer ${token}` });
const rawArgoManualWorkflowNames = () => {
  const output = execFileSync('kubectl', ['-n', 'argo', 'get', 'workflows.argoproj.io', '-o', 'jsonpath={range .items[*]}{.metadata.name}{"\\n"}{end}'], { encoding: 'utf8', env: process.env, timeout: 20_000, maxBuffer: 4 * 1024 * 1024 });
  return output.split(/\r?\n/).map((item) => item.trim()).filter((name) => name.startsWith('mlops-manual-'));
};

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
const ignoredExternalFailures = [];
page.on('response', (response) => { if (response.status() >= 400) badResponses.push({ status: response.status(), url: response.url() }); });
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const item = { text: entry.text(), url: entry.location().url ?? '' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com') || item.text.includes('ERR_CONNECTION_CLOSED')) ignoredExternalFailures.push(item);
  else consoleErrors.push(item.text);
});
page.on('pageerror', (error) => {
  if (error.message === 'Object') ignoredExternalFailures.push({ text: error.message, url: 'chrome-extension://unknown' });
  else pageErrors.push(error.message);
});
page.on('requestfailed', (request) => {
  const item = { url: request.url(), error: request.failure()?.errorText ?? 'unknown' };
  if (item.url.startsWith('chrome-extension://') || item.url.includes('api.yhchj.com')) ignoredExternalFailures.push(item);
  else requestFailures.push(item);
});

const token = smokeToken();
const routeUrl = new URL(`/mlops?windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${token}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-mlops').waitFor({ state: 'visible', timeout: 15_000 });

const workspace = await page.evaluate(async () => {
  const activeToken = localStorage.getItem('traffic-ui-token');
  const modelsResponse = await fetch('/api/v1/models?limit=100&offset=0', { headers: { Authorization: `Bearer ${activeToken}` } });
  const modelsBody = await modelsResponse.json();
  const model = modelsBody.data.find((item) => item.name === 'behavior-classifier') ?? modelsBody.data[0];
  const versionsResponse = await fetch(`/api/v1/models/${model.model_id}/versions?limit=100&offset=0`, { headers: { Authorization: `Bearer ${activeToken}` } });
  const versionsBody = await versionsResponse.json();
  return { model, versions: versionsBody.data };
});
const modelId = workspace.model?.model_id;
const activeVersion = workspace.versions.find((item) => item.status === 'active') ?? workspace.versions[0];
if (!modelId || !activeVersion?.model_version) throw new Error('MLOps acceptance model and active version are required');

const viewerToken = smokeToken({ permissions: ['model:read'], roles: ['viewer'], username: 'codex-mlops-viewer' });
const viewerWriteResponse = await page.request.post(`${baseUrl}/api/v1/mlops/retrain`, {
  headers: authHeaders(viewerToken),
  data: { model_type: 'xgboost', lookback_days: 7, feature_set_id: 'v1', params: { 'model-id': modelId } },
});
const crossTenantToken = smokeToken({ tenantId: 'tenant-mlops-isolated', permissions: ['model:read'], roles: ['viewer'], username: 'codex-mlops-cross-tenant' });
const crossTenantResponse = await page.request.get(`${baseUrl}/api/v1/models/${modelId}/workbench`, { headers: authHeaders(crossTenantToken) });
const unauthenticatedResponse = await page.request.get(`${baseUrl}/api/v1/mlops/status`);
const accessControl = {
  viewer_write_status: viewerWriteResponse.status(),
  cross_tenant_read_status: crossTenantResponse.status(),
  unauthenticated_status: unauthenticatedResponse.status(),
};

let ownedAutomaticListResponse;
let ownedAutomaticWorkflow;
const automaticOwnershipDeadline = Date.now() + 45_000;
while (Date.now() < automaticOwnershipDeadline && !ownedAutomaticWorkflow) {
  ownedAutomaticListResponse = await page.request.get(`${baseUrl}/api/v1/mlops/workflows`, { headers: authHeaders(token) });
  const ownedAutomaticListBody = await ownedAutomaticListResponse.json().catch(() => ({}));
  ownedAutomaticWorkflow = (ownedAutomaticListBody?.data ?? []).find((item) => /^mlops-(feedback|fp-rate|drift|scheduled)-/.test(item.name)
    && item.parameters?.['tenant-id'] === 'default'
    && item.parameters?.['model-id'] === modelId
    && item.parameters?.['feature-set-id']);
  if (!ownedAutomaticWorkflow) await delay(1_000);
}
const ownedAutomaticGetResponse = ownedAutomaticWorkflow
  ? await page.request.get(`${baseUrl}/api/v1/mlops/workflows/${encodeURIComponent(ownedAutomaticWorkflow.name)}`, { headers: authHeaders(token) })
  : null;
const automaticOwnership = {
  list_status: ownedAutomaticListResponse?.status() ?? 0,
  workflow_name: ownedAutomaticWorkflow?.name ?? '',
  tenant_id: ownedAutomaticWorkflow?.parameters?.['tenant-id'] ?? '',
  model_id: ownedAutomaticWorkflow?.parameters?.['model-id'] ?? '',
  feature_set_id: ownedAutomaticWorkflow?.parameters?.['feature-set-id'] ?? '',
  get_status: ownedAutomaticGetResponse?.status() ?? 0,
};
const automaticOwnershipPass = automaticOwnership.list_status === 200
  && Boolean(automaticOwnership.workflow_name)
  && automaticOwnership.tenant_id === 'default'
  && automaticOwnership.model_id === modelId
  && Boolean(automaticOwnership.feature_set_id)
  && automaticOwnership.get_status === 200;

// Prove a globally duplicated version cannot overwrite another tenant's artifact or metrics.
const isolatedAdminToken = smokeToken({ tenantId: 'tenant-mlops-isolated', username: 'codex-mlops-conflict-probe' });
const isolatedModelResponse = await page.request.post(`${baseUrl}/api/v1/models`, {
  headers: authHeaders(isolatedAdminToken),
  data: { name: `mlops-conflict-probe-${Date.now()}`, model_type: 'xgboost', description: 'ephemeral r331 duplicate-version guard probe', metadata: { acceptance_revision: revision } },
});
if (isolatedModelResponse.status() !== 201) throw new Error(`isolated model setup failed: ${isolatedModelResponse.status()} ${await isolatedModelResponse.text()}`);
const isolatedModel = (await isolatedModelResponse.json()).data;
let duplicateVersionProtection;
try {
  const originalBeforeResponse = await page.request.get(`${baseUrl}/api/v1/models/${modelId}/versions/${encodeURIComponent(activeVersion.model_version)}`, { headers: authHeaders(token) });
  const originalBefore = (await originalBeforeResponse.json()).data;
  const conflictResponse = await page.request.post(`${baseUrl}/api/v1/models/${isolatedModel.model_id}/versions`, {
    headers: authHeaders(isolatedAdminToken),
    data: {
      model_type: 'xgboost', version: activeVersion.model_version,
      artifact_uri: 's3://must-not-overwrite/cross-tenant/model.bin', feature_set_id: 'isolated-v1',
      metrics: { accuracy: 0.01, marker: 'must-not-overwrite' }, status: 'registered',
    },
  });
  const conflictBody = await conflictResponse.json().catch(() => ({}));
  const originalAfterResponse = await page.request.get(`${baseUrl}/api/v1/models/${modelId}/versions/${encodeURIComponent(activeVersion.model_version)}`, { headers: authHeaders(token) });
  const originalAfter = (await originalAfterResponse.json()).data;
  duplicateVersionProtection = {
    conflict_status: conflictResponse.status(),
    conflict_code: conflictBody?.error?.code ?? conflictBody?.code ?? '',
    original_artifact_unchanged: originalBefore.artifact_uri === originalAfter.artifact_uri,
    original_metrics_unchanged: JSON.stringify(originalBefore.metrics) === JSON.stringify(originalAfter.metrics),
    attempted_tenant: 'tenant-mlops-isolated',
    protected_version: activeVersion.model_version,
  };
} finally {
  const cleanupResponse = await page.request.delete(`${baseUrl}/api/v1/models/${isolatedModel.model_id}`, { headers: authHeaders(isolatedAdminToken) });
  if (cleanupResponse.status() !== 200) throw new Error(`isolated model cleanup failed: ${cleanupResponse.status()} ${await cleanupResponse.text()}`);
}

fs.mkdirSync(evidenceDir, { recursive: true });

async function waitForWorkflow(name, predicate, timeout = 45_000) {
  const deadline = Date.now() + timeout;
  let last;
  while (Date.now() < deadline) {
    const response = await page.request.get(`${baseUrl}/api/v1/mlops/workflows/${encodeURIComponent(name)}`, { headers: authHeaders(token) });
    if (response.ok()) {
      const body = await response.json();
      last = body.data;
      if (predicate(last)) return last;
    }
    await delay(1_000);
  }
  throw new Error(`workflow ${name} did not reach the expected state; last=${JSON.stringify(last)}`);
}

async function refreshWorkspace() {
  const responsePromise = page.waitForResponse((response) => response.url().includes('/api/v1/mlops/workflows') && response.request().method() === 'GET');
  await page.locator('.taf-mlops-titlebar button').nth(6).click();
  const response = await responsePromise;
  if (!response.ok()) throw new Error(`workspace refresh failed: ${response.status()}`);
  await page.waitForTimeout(350);
}

async function closeDrawer(drawer) {
  await drawer.locator('.ant-drawer-close').click();
  await drawer.waitFor({ state: 'hidden', timeout: 5_000 });
}

async function submitDrawerAction({ button, responsePattern, method = 'POST' }) {
  await button.click();
  const drawer = page.locator('.taf-mlops-action-drawer:visible');
  await drawer.waitFor({ state: 'visible', timeout: 5_000 });
  const beforeText = await drawer.textContent();
  const responsePromise = page.waitForResponse((response) => response.url().includes(responsePattern) && response.request().method() === method);
  await drawer.getByRole('button', { name: method === 'GET' ? '查询详情' : '确认提交', exact: true }).click();
  const response = await responsePromise;
  const body = await response.json().catch(() => ({}));
  await drawer.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 10_000 });
  const afterText = await drawer.textContent();
  return { drawer, response, body, beforeText, afterText };
}

const titleButtons = page.locator('.taf-mlops-titlebar button');

// A model writer may choose only the model and trigger reason. Product callers
// must not be able to override the trusted trainer image, gate thresholds, or
// automatic activation policy that are owned by the WorkflowTemplate.
const unsafeWorkflowNamesBefore = new Set(rawArgoManualWorkflowNames());
const unsafeOverrideResponse = await page.request.post(`${baseUrl}/api/v1/mlops/retrain`, {
  headers: authHeaders(token),
  data: {
    model_type: 'xgboost', lookback_days: 7, feature_set_id: 'v1',
    params: {
      'model-id': modelId,
      ' model-id ': 'must-not-normalize-or-override',
      'trigger-reason': `windows_chrome_${revision}_unsafe_override_probe`,
      'trainer-image': 'attacker.invalid/untrusted:latest',
      'min-feedback-count': 0,
      'min-feature-count': 0,
      'min-f1-score': 0,
      'auto-activate': true,
    },
  },
});
const unsafeOverrideBody = await unsafeOverrideResponse.json();
if (unsafeOverrideResponse.status() !== 400) throw new Error(`unsafe override probe was not rejected: ${unsafeOverrideResponse.status()} ${JSON.stringify(unsafeOverrideBody)}`);
const unsafeOverrideMessage = unsafeOverrideBody?.message ?? unsafeOverrideBody?.error?.message ?? '';
const unsafeOverridesRejected = ['trainer-image', 'min-feedback-count', 'min-feature-count', 'min-f1-score', 'auto-activate']
  .every((key) => unsafeOverrideMessage.includes(key));
if (!unsafeOverridesRejected) throw new Error(`unsafe override rejection did not identify every key: ${JSON.stringify(unsafeOverrideBody)}`);
const unsafeWorkflowNamesAfter = rawArgoManualWorkflowNames();
const unsafeWorkflowNamesCreated = unsafeWorkflowNamesAfter.filter((name) => !unsafeWorkflowNamesBefore.has(name));
if (unsafeWorkflowNamesCreated.length > 0) throw new Error(`unsafe override request created workflows: ${unsafeWorkflowNamesCreated.join(', ')}`);

const createAction = await submitDrawerAction({
  button: titleButtons.filter({ hasText: '投递训练任务' }).first(),
  responsePattern: '/api/v1/mlops/retrain',
});
if (createAction.response.status() !== 202) throw new Error(`retrain submit failed: ${createAction.response.status()} ${JSON.stringify(createAction.body)}`);
const uiWorkflowName = createAction.body?.data?.workflow_name;
const uiCreateAuditEvent = createAction.body?.data?.audit_event;
if (!uiWorkflowName) throw new Error(`retrain response did not return workflow_name: ${JSON.stringify(createAction.body)}`);
await closeDrawer(createAction.drawer);

const workflowName = uiWorkflowName;
const createAuditEvent = uiCreateAuditEvent;
const submittedWorkflow = await waitForWorkflow(workflowName, (item) => item.can_stop === true);

const crossTenantWriteToken = smokeToken({ tenantId: 'tenant-mlops-isolated', permissions: ['model:write'], roles: ['editor'], username: 'codex-mlops-cross-tenant-writer' });
const crossTenantWorkflowListResponse = await page.request.get(`${baseUrl}/api/v1/mlops/workflows`, { headers: authHeaders(crossTenantToken) });
const crossTenantWorkflowListBody = await crossTenantWorkflowListResponse.json().catch(() => ({}));
const crossTenantWorkflowGetResponse = await page.request.get(`${baseUrl}/api/v1/mlops/workflows/${encodeURIComponent(workflowName)}`, { headers: authHeaders(crossTenantToken) });
const crossTenantWorkflowStopResponse = await page.request.post(`${baseUrl}/api/v1/mlops/workflows/${encodeURIComponent(workflowName)}/stop`, { headers: authHeaders(crossTenantWriteToken) });
const crossTenantWorkflowRetryResponse = await page.request.post(`${baseUrl}/api/v1/mlops/workflows/${encodeURIComponent(workflowName)}/retry`, { headers: authHeaders(crossTenantWriteToken) });
accessControl.workflow_list_status = crossTenantWorkflowListResponse.status();
accessControl.workflow_list_count = crossTenantWorkflowListBody?.data?.length ?? -1;
accessControl.workflow_get_status = crossTenantWorkflowGetResponse.status();
accessControl.workflow_stop_status = crossTenantWorkflowStopResponse.status();
accessControl.workflow_retry_status = crossTenantWorkflowRetryResponse.status();
await refreshWorkspace();
const workflowRow = page.locator('.taf-mlops .ant-table-tbody tr', { hasText: workflowName }).first();
await workflowRow.waitFor({ state: 'visible', timeout: 10_000 });

const inspectAction = await submitDrawerAction({
  button: workflowRow.locator('.taf-mlops-row-actions button').nth(0),
  responsePattern: `/api/v1/mlops/workflows/${workflowName}`,
  method: 'GET',
});
const inspectReadOnly = inspectAction.response.status() === 200
  && inspectAction.beforeText.includes('只读操作，不伪造审计事件')
  && inspectAction.afterText.includes('READ_ONLY');
await closeDrawer(inspectAction.drawer);

const stopAction = await submitDrawerAction({
  button: workflowRow.locator('.taf-mlops-row-actions button').nth(1),
  responsePattern: `/api/v1/mlops/workflows/${workflowName}/stop`,
});
const stopAuditEvent = stopAction.body?.data?.audit_event;
await closeDrawer(stopAction.drawer);
const stoppedWorkflow = await waitForWorkflow(workflowName, (item) => ['Failed', 'Error'].includes(item.phase) && item.can_retry === true);
await refreshWorkspace();

const failedWorkflowRow = page.locator('.taf-mlops .ant-table-tbody tr', { hasText: workflowName }).first();
await failedWorkflowRow.waitFor({ state: 'visible', timeout: 10_000 });
const retryAction = await submitDrawerAction({
  button: failedWorkflowRow.locator('.taf-mlops-row-actions button').nth(2),
  responsePattern: `/api/v1/mlops/workflows/${workflowName}/retry`,
});
const retryAuditEvent = retryAction.body?.data?.audit_event;
const retryWorkflowName = retryAction.body?.data?.workflow_name;
if (!retryWorkflowName || retryWorkflowName === workflowName) throw new Error(`retry did not resubmit a fresh workflow: ${JSON.stringify(retryAction.body)}`);
await closeDrawer(retryAction.drawer);
await waitForWorkflow(retryWorkflowName, (item) => ['Pending', 'Running', 'Succeeded'].includes(item.phase));
// A successful resubmit response proves mutation and audit semantics, but not
// the business pipeline. Keep the Windows Chrome gate open until the retried
// workflow reaches a terminal state and require the full pipeline to succeed.
const retriedWorkflow = await waitForWorkflow(
  retryWorkflowName,
  (item) => ['Succeeded', 'Failed', 'Error'].includes(item.phase),
  retryTerminalTimeoutMs,
);
await refreshWorkspace();

const registerRows = page.locator('.taf-mlops-register > button');
const activeRegisterRow = registerRows.filter({ hasText: '已上线' }).first();
await activeRegisterRow.waitFor({ state: 'visible', timeout: 5_000 });
const displayedActiveVersion = (await activeRegisterRow.locator('span').nth(1).textContent())?.trim();
const versionAction = await submitDrawerAction({
  button: activeRegisterRow,
  responsePattern: `/api/v1/models/${modelId}/versions/${encodeURIComponent(displayedActiveVersion)}`,
  method: 'GET',
});
const versionReadExact = versionAction.response.status() === 200
  && versionAction.beforeText.includes(`/v1/models/${modelId}/versions/${displayedActiveVersion}`)
  && versionAction.afterText.includes('READ_ONLY');
await closeDrawer(versionAction.drawer);

const disabledControls = {
  labeling: await page.getByRole('button', { name: '发起标注', exact: true }).isDisabled(),
  manual_register: await titleButtons.filter({ hasText: '注册模型' }).first().isDisabled(),
  auto_register: await page.getByRole('button', { name: '自动注册', exact: true }).isDisabled(),
};

const taskFilter = page.locator('.taf-mlops-bottom .ant-select').first();
await taskFilter.click();
await page.getByText('失败', { exact: true }).last().click();
const filterWorked = Boolean((await taskFilter.textContent())?.includes('失败'));
await taskFilter.click();
await page.getByText('全部任务', { exact: true }).last().click();

const responsiveChecks = [];
for (const width of [1440, 1536, 1600, 1920]) {
  await page.setViewportSize({ width, height: 1080 });
  await page.waitForTimeout(250);
  const check = await page.evaluate(() => {
    const root = document.documentElement;
    const workbench = document.querySelector('.taf-mlops-workbench');
    const feedbackRows = [...document.querySelectorAll('.taf-mlops-feedback-pool > button')];
    const taskActions = [...document.querySelectorAll('.taf-mlops-row-actions')];
    const gateResult = document.querySelector('.taf-mlops-confusion strong');
    const gateHeader = document.querySelector('.taf-mlops-gate-table > div');
    const contained = (element) => {
      if (!element) return false;
      const rect = element.getBoundingClientRect();
      const panel = element.closest('.taf-panel')?.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0 && (!panel || (
        rect.left >= panel.left - 1 && rect.right <= panel.right + 1
        && rect.top >= panel.top - 1 && rect.bottom <= panel.bottom + 1
      ));
    };
    const actionButtonsVisible = taskActions.every((item) => {
      const buttons = [...item.querySelectorAll('button')];
      const rect = item.getBoundingClientRect();
      return buttons.length === 3 && buttons.every((button) => {
        const buttonRect = button.getBoundingClientRect();
        return buttonRect.width >= 24 && buttonRect.height >= 24 && buttonRect.left >= rect.left - 1 && buttonRect.right <= rect.right + 1;
      });
    });
    return {
      width: root.clientWidth,
      no_horizontal_scroll: root.scrollWidth <= root.clientWidth + 1,
      workbench_columns: workbench ? getComputedStyle(workbench).gridTemplateColumns : '',
      feedback_rows_contained: feedbackRows.length === 10 && feedbackRows.every(contained),
      task_action_groups: taskActions.length,
      task_three_actions_visible: taskActions.length > 0 && actionButtonsVisible,
      register_rows: [...document.querySelectorAll('.taf-mlops-register > button')].length,
      register_rows_contained: [...document.querySelectorAll('.taf-mlops-register > button')].length === 4
        && [...document.querySelectorAll('.taf-mlops-register > button')].every(contained),
      version_titles_present: [...document.querySelectorAll('.taf-mlops-register > button span:nth-child(2)')]
        .every((item) => Boolean(item.getAttribute('title')?.trim())),
      gate_result_present: Boolean(gateResult?.textContent?.includes('产品阈值结果') && gateResult?.textContent?.includes('未通过')),
      gate_result_contained: contained(gateResult),
      gate_trend_uses_version_history: Boolean(gateHeader?.textContent?.includes('版本趋势') && !gateHeader?.textContent?.includes('7日')),
    };
  });
  responsiveChecks.push(check);
}
await page.setViewportSize({ width: 1920, height: 1080 });
await page.evaluate(() => window.scrollTo(0, 0));
await page.waitForTimeout(300);

const chartCanvasCount = await page.locator('.taf-mlops canvas').count();
const chartBounds = await page.locator('.taf-mlops canvas').evaluateAll((canvases) => canvases.map((canvas) => {
  const rect = canvas.getBoundingClientRect();
  return { width: rect.width, height: rect.height, top: rect.top, bottom: rect.bottom, has_size: rect.width >= 20 && rect.height >= 15 };
}));
const chartsVisible = chartBounds.length >= 4 && chartBounds.every((item) => item.has_size);

await page.screenshot({ path: implementationPath, fullPage: false });
await page.screenshot({ path: screenshotPath, fullPage: false });
const interactionScreenshotValid = new URL(page.url()).pathname === '/mlops'
  && await page.locator('.taf-mlops').isVisible()
  && fs.statSync(screenshotPath).size > 100_000;

const sql = `SELECT COALESCE(json_agg(row_to_json(a) ORDER BY a.created_at), '[]'::json)::text FROM (SELECT event_id, action, object_type, object_id, detail, created_at FROM audit_logs WHERE tenant_id = 'default' AND object_type = 'mlops_workflow' AND object_id IN ('${workflowName.replaceAll("'", "''")}','${retryWorkflowName.replaceAll("'", "''")}') AND action IN ('MLOPS_RETRAIN_SUBMIT_REQUESTED','MLOPS_RETRAIN_SUBMITTED','MLOPS_WORKFLOW_STOP_INTENT','MLOPS_WORKFLOW_STOP_REQUESTED','MLOPS_WORKFLOW_RESUBMIT_INTENT','MLOPS_WORKFLOW_RESUBMITTED')) a;`;
const auditOutput = execFileSync('kubectl', ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', sql], { encoding: 'utf8', env: process.env, timeout: 20_000 }).trim();
const auditRows = JSON.parse(auditOutput || '[]');
const workflowResource = JSON.parse(execFileSync('kubectl', ['-n', 'argo', 'get', 'workflow', retryWorkflowName, '-o', 'json'], { encoding: 'utf8', env: process.env, timeout: 20_000 }));
const auditEvents = new Set(auditRows.map((row) => row.action));
const auditRow = (action) => auditRows.find((row) => row.action === action);
const auditIntentOrder = {
  create: Boolean(auditRow('MLOPS_RETRAIN_SUBMIT_REQUESTED')?.event_id
    && auditRow('MLOPS_RETRAIN_SUBMITTED')?.detail?.intent_event_id === auditRow('MLOPS_RETRAIN_SUBMIT_REQUESTED')?.event_id
    && Date.parse(auditRow('MLOPS_RETRAIN_SUBMIT_REQUESTED')?.created_at) <= Date.parse(auditRow('MLOPS_RETRAIN_SUBMITTED')?.created_at)),
  stop: Boolean(auditRow('MLOPS_WORKFLOW_STOP_INTENT')?.event_id
    && auditRow('MLOPS_WORKFLOW_STOP_REQUESTED')?.detail?.intent_event_id === auditRow('MLOPS_WORKFLOW_STOP_INTENT')?.event_id
    && Date.parse(auditRow('MLOPS_WORKFLOW_STOP_INTENT')?.created_at) <= Date.parse(auditRow('MLOPS_WORKFLOW_STOP_REQUESTED')?.created_at)),
  retry: Boolean(auditRow('MLOPS_WORKFLOW_RESUBMIT_INTENT')?.event_id
    && auditRow('MLOPS_WORKFLOW_RESUBMITTED')?.detail?.intent_event_id === auditRow('MLOPS_WORKFLOW_RESUBMIT_INTENT')?.event_id
    && Date.parse(auditRow('MLOPS_WORKFLOW_RESUBMIT_INTENT')?.created_at) <= Date.parse(auditRow('MLOPS_WORKFLOW_RESUBMITTED')?.created_at)),
};
const authoritativeChain = {
  workflow_name: workflowName,
  retry_workflow_name: retryWorkflowName,
  retry_terminal_phase: retriedWorkflow.phase,
  create_event: createAuditEvent,
  stop_event: stopAuditEvent,
  retry_event: retryAuditEvent,
  audit_rows: auditRows,
  audit_intent_order: auditIntentOrder,
  argo_uid: workflowResource.metadata?.uid,
  argo_phase: workflowResource.status?.phase,
  argo_template: workflowResource.spec?.workflowTemplateRef?.name,
  argo_parameters: Object.fromEntries((workflowResource.spec?.arguments?.parameters ?? []).map((item) => [item.name, item.value])),
  submitted_phase: submittedWorkflow.phase,
  stopped_phase: stoppedWorkflow.phase,
  retried_phase: retriedWorkflow.phase,
};

const deploymentHref = await titleButtons.filter({ hasText: '进入部署管理' }).first().evaluate((button) => {
  button.click();
  return true;
});
await page.waitForURL(/\/deployments(?:\?|$)/, { timeout: 10_000 });
const deploymentUrl = new URL(page.url());
const deploymentNavigation = deploymentHref
  && deploymentUrl.pathname === '/deployments'
  && deploymentUrl.searchParams.get('model_id') === modelId;

const actionChecks = {
  create_status: createAction.response.status(),
  create_audit_event: uiCreateAuditEvent,
  ui_workflow_name: uiWorkflowName,
  unsafe_override_probe_status: unsafeOverrideResponse.status(),
  unsafe_overrides_rejected: unsafeOverridesRejected,
  unsafe_override_response: unsafeOverrideBody,
  unsafe_override_workflow_created: unsafeWorkflowNamesCreated.length > 0,
  unsafe_override_workflow_names_before: [...unsafeWorkflowNamesBefore].sort(),
  unsafe_override_workflow_names_after: [...unsafeWorkflowNamesAfter].sort(),
  inspect_read_only: inspectReadOnly,
  stop_status: stopAction.response.status(),
  stop_audit_event: stopAuditEvent,
  retry_status: retryAction.response.status(),
  retry_audit_event: retryAuditEvent,
  retry_workflow_name: retryWorkflowName,
  displayed_version: displayedActiveVersion,
  version_read_exact: versionReadExact,
};
const responsivePass = responsiveChecks.every((item) => item.no_horizontal_scroll
  && item.feedback_rows_contained
  && item.task_three_actions_visible
  && item.gate_result_present
  && item.gate_result_contained
  && item.gate_trend_uses_version_history
  && item.register_rows_contained
  && item.version_titles_present
  && (item.width > 1680 ? item.workbench_columns.split(' ').length >= 2 : item.workbench_columns.split(' ').length === 1));
const authoritativeChainPass = createAuditEvent === 'MLOPS_RETRAIN_SUBMITTED'
  && stopAuditEvent === 'MLOPS_WORKFLOW_STOP_REQUESTED'
  && retryAuditEvent === 'MLOPS_WORKFLOW_RESUBMITTED'
  && ['MLOPS_RETRAIN_SUBMITTED', 'MLOPS_WORKFLOW_STOP_REQUESTED', 'MLOPS_WORKFLOW_RESUBMITTED'].every((item) => auditEvents.has(item))
  && Object.values(auditIntentOrder).every(Boolean)
  && retryAction.body?.data?.status === 'Submitted'
  && retriedWorkflow.phase === 'Succeeded'
  && workflowResource.metadata?.name === retryWorkflowName
  && workflowResource.spec?.workflowTemplateRef?.name === 'mlops-training-template';

const result = {
  result: chartsVisible
    && Object.values(disabledControls).every(Boolean)
    && filterWorked
    && interactionScreenshotValid
    && deploymentNavigation
    && accessControl.viewer_write_status === 403
    && [403, 404].includes(accessControl.cross_tenant_read_status)
    && accessControl.workflow_list_status === 200
    && accessControl.workflow_list_count === 0
    && accessControl.workflow_get_status === 403
    && accessControl.workflow_stop_status === 403
    && accessControl.workflow_retry_status === 403
    && accessControl.unauthenticated_status === 401
    && automaticOwnershipPass
    && duplicateVersionProtection.conflict_status === 409
    && duplicateVersionProtection.original_artifact_unchanged
    && duplicateVersionProtection.original_metrics_unchanged
    && actionChecks.create_status === 202
    && actionChecks.create_audit_event === 'MLOPS_RETRAIN_SUBMITTED'
    && actionChecks.unsafe_override_probe_status === 400
    && actionChecks.unsafe_overrides_rejected
    && actionChecks.inspect_read_only
    && actionChecks.stop_status === 202
    && actionChecks.retry_status === 202
    && actionChecks.version_read_exact
    && authoritativeChainPass
    && responsivePass
    && badResponses.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0
    && requestFailures.length === 0 ? 'pass' : 'fail',
  revision,
  browser_backend: 'Windows Chrome CDP over Xshell tunnel',
  cdp_url: cdpUrl,
  product_url: baseUrl,
  browser: version.Browser,
  viewport: { width: 1920, height: 1080 },
  action_checks: actionChecks,
  authoritative_chain: authoritativeChain,
  authoritative_chain_pass: authoritativeChainPass,
  duplicate_version_protection: duplicateVersionProtection,
  disabled_controls: disabledControls,
  filter_worked: filterWorked,
  responsive_checks: responsiveChecks,
  responsive_pass: responsivePass,
  chart_canvas_count: chartCanvasCount,
  chart_bounds: chartBounds,
  charts_visible: chartsVisible,
  interaction_screenshot_valid: interactionScreenshotValid,
  deployment_navigation: deploymentNavigation,
  access_control: accessControl,
  automatic_ownership: automaticOwnership,
  automatic_ownership_pass: automaticOwnershipPass,
  bad_responses: badResponses,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  request_failures: requestFailures,
  ignored_external_failures: ignoredExternalFailures,
  implementation_screenshot: path.relative(root, implementationPath),
  screenshot: path.relative(root, screenshotPath),
  timestamp: new Date().toISOString(),
};

fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);

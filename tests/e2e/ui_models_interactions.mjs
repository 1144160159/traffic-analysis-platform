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
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/models');
const evidenceRevision = process.env.MODEL_EVIDENCE_REVISION?.trim() || 'r288';
const skipActions = process.env.MODEL_SKIP_ACTIONS === '1';
const outputPath = path.join(evidenceDir, `interaction-${evidenceRevision}.json`);
const screenshotPath = path.join(evidenceDir, `interaction-${evidenceRevision}.png`);
const stateScreenshotPaths = {
  activationAuditGate: path.join(evidenceDir, `state-${evidenceRevision}-activation-audit-gate.png`),
  importantFeatures: path.join(evidenceDir, `state-${evidenceRevision}-important-features.png`),
  ruleContribution: path.join(evidenceDir, `state-${evidenceRevision}-rule-contribution.png`),
  anomalyExplanation: path.join(evidenceDir, `state-${evidenceRevision}-anomaly-explanation.png`),
  sampleExamples: path.join(evidenceDir, `state-${evidenceRevision}-sample-examples.png`),
};

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken({ tenantId = 'default', permissions = ['*', 'admin:*', 'model:*'], roles = ['admin'], username = 'codex-windows-cdp-admin' } = {}) {
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
    tenant_id: tenantId,
    username,
    roles,
    permissions,
    token_type: 'access',
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
page.setDefaultTimeout(12_000);
const cdpSession = await context.newCDPSession(page);
await cdpSession.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
await cdpSession.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
await cdpSession.send('Network.enable');
await cdpSession.send('Runtime.enable');

const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
const externalRequestInitiators = [];
const runtimeExceptions = [];
const executionContexts = [];
cdpSession.on('Network.requestWillBeSent', (event) => {
  if (event.request.url.startsWith('https://api.yhchj.com/')) {
    externalRequestInitiators.push({ url: event.request.url, initiator: event.initiator });
  }
});
cdpSession.on('Runtime.exceptionThrown', (event) => runtimeExceptions.push(event.exceptionDetails));
cdpSession.on('Runtime.executionContextCreated', (event) => executionContexts.push(event.context));
page.on('response', (response) => {
  const url = new URL(response.url());
  if (url.origin === baseUrl && response.status() >= 400) badResponses.push({ status: response.status(), url: response.url() });
});
page.on('console', (entry) => { if (entry.type() === 'error') consoleErrors.push({ text: entry.text(), location: entry.location() }); });
page.on('pageerror', (error) => pageErrors.push({ message: error.message, stack: error.stack ?? '' }));
page.on('requestfailed', (request) => {
  const url = new URL(request.url());
  if (url.origin === baseUrl) requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? 'unknown' });
});

const authToken = smokeToken();
const startedAt = new Date().toISOString();
const routeUrl = new URL(`/models?windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${authToken}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await page.locator('.taf-models').waitFor({ state: 'visible' });
// The Xshell-connected Chrome profile can retain per-origin zoom. Reset it so
// CSS viewport evidence is genuinely 1920x1080, not a 90% zoomed 2133x1200.
await page.keyboard.press('Control+Digit0');
await page.waitForTimeout(250);

const modelApiResponse = await page.request.get(`${baseUrl}/api/v1/models?limit=8&offset=0`, {
  headers: { Authorization: `Bearer ${smokeToken()}` },
});
const modelPayload = await modelApiResponse.json();
const apiModels = Array.isArray(modelPayload?.data) ? modelPayload.data : [];
const apiTotal = Number(modelPayload?.pagination?.total ?? apiModels.length);
const tableRows = page.locator('.taf-models-list-panel tbody tr');
const renderedRows = await tableRows.count();
const renderedText = await page.locator('.taf-models').innerText();
const noSimulationMarkers = !/SIM-MODEL|仿真任务|仿真执行/.test(renderedText);
const listTitle = await page.locator('.taf-models-list-panel .taf-panel__header h2').textContent();
const totalMatchesApi = listTitle?.includes(`共 ${apiTotal} 条`) ?? false;
const firstApiModel = apiModels[0];
const firstRowText = renderedRows ? await tableRows.first().textContent() : '';
const firstRowMatchesApi = Boolean(firstApiModel?.name && firstRowText?.includes(firstApiModel.name));
const firstVersionMatchesApi = Boolean(firstApiModel?.model_version && firstRowText?.includes(firstApiModel.model_version));

const workbenchApiResponse = firstApiModel?.model_id ? await page.request.get(`${baseUrl}/api/v1/models/${encodeURIComponent(firstApiModel.model_id)}/workbench`, {
  headers: { Authorization: `Bearer ${smokeToken()}` },
}) : undefined;
const workbenchPayload = workbenchApiResponse ? await workbenchApiResponse.json() : {};
const workbench = workbenchPayload?.data ?? {};
const activeVersion = workbench?.versions?.find((item) => item.status === 'active')?.model_version ?? workbench?.versions?.[0]?.model_version ?? '';
const activationGateResponse = activeVersion ? await page.request.post(`${baseUrl}/api/v1/models/${encodeURIComponent(firstApiModel.model_id)}/versions/${encodeURIComponent(activeVersion)}/activate`, {
  headers: { Authorization: `Bearer ${smokeToken()}` },
  data: { gray_percent: 100 },
}) : undefined;
const activationGatePayload = activationGateResponse ? await activationGateResponse.json().catch(() => ({})) : {};
const serverActivationGateEnforced = !activeVersion || (activationGateResponse?.status() === 409 && /pending review gates/.test(JSON.stringify(activationGatePayload)));

const originalModelName = String(firstApiModel?.name ?? '');
const crossTenantUpdateResponse = firstApiModel?.model_id ? await page.request.put(`${baseUrl}/api/v1/models/${encodeURIComponent(firstApiModel.model_id)}`, {
  headers: { Authorization: `Bearer ${smokeToken({ tenantId: 'cross-tenant-regression', permissions: ['model:write'], roles: ['analyst'], username: 'cross-tenant-regression' })}` },
  data: { name: `${originalModelName}-forbidden`, model_type: firstApiModel.model_type, description: 'cross-tenant regression probe' },
}) : undefined;
const modelAfterCrossTenantProbe = firstApiModel?.model_id ? await page.request.get(`${baseUrl}/api/v1/models/${encodeURIComponent(firstApiModel.model_id)}`, {
  headers: { Authorization: `Bearer ${smokeToken()}` },
}) : undefined;
const modelAfterCrossTenantPayload = modelAfterCrossTenantProbe ? await modelAfterCrossTenantProbe.json().catch(() => ({})) : {};
const crossTenantUpdateDenied = !firstApiModel?.model_id || (crossTenantUpdateResponse?.status() === 403 && modelAfterCrossTenantPayload?.data?.name === originalModelName);
const expectedWorkbenchCounts = {
  features: 6,
  rule_contributions: 5,
  anomaly_causes: 3,
  samples: 5,
  similar_samples: 4,
  datasets: 4,
  review_gates: 4,
  metrics: 7,
  distribution: 4,
};
const workbenchCounts = Object.fromEntries(Object.keys(expectedWorkbenchCounts).map((key) => [key, Array.isArray(workbench?.items?.[key]) ? workbench.items[key].length : 0]));
const workbenchBackedByPostgresql = workbenchApiResponse?.ok() === true
  && String(workbench?.source ?? '').startsWith('postgresql')
  && Object.entries(expectedWorkbenchCounts).every(([key, count]) => workbenchCounts[key] === count);

let selectedModelWorkbenchSwitch = renderedRows < 2;
if (renderedRows >= 2) {
  const secondApiModel = apiModels[1];
  const secondWorkbenchResponse = page.waitForResponse((response) => response.url().includes(`/api/v1/models/${secondApiModel.model_id}/workbench`));
  await tableRows.nth(1).click();
  const switchResponse = await secondWorkbenchResponse;
  const switchPayload = await switchResponse.json();
  selectedModelWorkbenchSwitch = switchResponse.ok()
    && String(switchPayload?.data?.source ?? '').startsWith('postgresql')
    && switchPayload?.data?.model?.model_id === secondApiModel.model_id;
  await tableRows.first().click();
  await page.locator('.taf-models-left-bottom').getByText(firstApiModel.name, { exact: false }).first().waitFor({ state: 'visible' });
}

const chartCanvasCount = await page.locator('.taf-models canvas').count();
const liveActionChecks = [];

async function submitAction(openAction, endpointPattern, auditEvent) {
  await openAction();
  const drawer = page.locator('.taf-models-action-drawer:visible');
  await drawer.waitFor({ state: 'visible' });
  const endpointText = await drawer.locator('.ant-descriptions-item-content').first().textContent();
  const endpointVisible = /POST \/v1\/models\/.+/.test(endpointText ?? '');
  const auditVisible = await drawer.getByText(auditEvent, { exact: true }).isVisible();
  const responsePromise = page.waitForResponse((response) => endpointPattern.test(new URL(response.url()).pathname));
  await page.locator('button').filter({ hasText: '确认提交' }).last().click();
  const response = await responsePromise;
  const payload = await response.json().catch(() => ({}));
  const accepted = response.status() === 202 && Boolean(payload?.data?.job_id);
  const success = drawer.getByText(/已由服务端受理/);
  if (accepted) await success.waitFor({ state: 'visible' });
  liveActionChecks.push({ auditEvent, endpointVisible, auditVisible, status: response.status(), jobId: payload?.data?.job_id ?? '', accepted });
  await drawer.locator('.ant-drawer-close').click();
  await drawer.waitFor({ state: 'hidden' });
}

if (renderedRows > 0 && !skipActions) {
  await submitAction(
    () => page.locator('.taf-models-row-actions').first().getByRole('button', { name: '查看模型上下文', exact: true }).click(),
    /\/api\/v1\/models\/[^/]+\/actions$/,
    'MODEL_CONTEXT_ACTION_REQUESTED',
  );
  await submitAction(
    () => page.locator('.taf-models-titlebar button').filter({ hasText: '追加反馈样本' }).first().click(),
    /\/api\/v1\/models\/[^/]+\/feedback-samples$/,
    'MODEL_FEEDBACK_INGEST_REQUESTED',
  );
  if (firstApiModel?.model_version) {
    await submitAction(
      () => page.locator('.taf-models-row-actions').first().getByRole('button', { name: '评估模型', exact: true }).click(),
      /\/api\/v1\/models\/[^/]+\/versions\/[^/]+\/evaluate$/,
      'MODEL_EVALUATION_REQUESTED',
    );
  }
}

const jobIds = liveActionChecks.map((check) => check.jobId).filter(Boolean);
const terminalActions = [];
for (const jobId of jobIds) {
  let terminal;
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const response = await page.request.get(`${baseUrl}/api/v1/models/${encodeURIComponent(firstApiModel.model_id)}/workbench`, {
      headers: { Authorization: `Bearer ${smokeToken()}` },
    });
    const payload = await response.json();
    terminal = payload?.data?.actions?.find((action) => action.job_id === jobId);
    if (terminal && ['completed', 'failed'].includes(terminal.status)) break;
    await page.waitForTimeout(500);
  }
  terminalActions.push({ jobId, status: terminal?.status ?? 'missing', action: terminal?.action ?? '' });
}
const auditRows = jobIds.length ? execFileSync(
  'kubectl',
  ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', `SELECT detail->>'job_id', action FROM audit_logs WHERE detail->>'job_id' IN (${jobIds.map((id) => `'${id}'`).join(',')}) ORDER BY created_at`],
  { encoding: 'utf8', env: process.env, timeout: 15_000 },
).trim().split('\n').filter(Boolean).map((line) => {
  const [jobId, action] = line.split('|');
  return { jobId, action };
}) : [];
const actionAuditClosed = jobIds.every((jobId) => auditRows.some((row) => row.jobId === jobId && /DISPATCHED|COMPLETED/.test(row.action)));

fs.mkdirSync(evidenceDir, { recursive: true });
const explanationStateChecks = [];
const explanationStates = [
  { tab: '重要特征', selector: '.taf-models-explain' },
  { tab: '规则贡献', selector: '.taf-models-contributions' },
  { tab: '异常解释', selector: '.taf-models-anomaly-explain' },
  { tab: '样本示例', selector: '.taf-models-sample-examples' },
];
for (const state of explanationStates) {
  await page.locator('.taf-models-tabs').getByRole('button', { name: state.tab, exact: true }).click();
  const stateRoot = page.locator(state.selector);
  await stateRoot.waitFor({ state: 'visible' });
  const stateText = await stateRoot.innerText().catch(() => state.tab);
  explanationStateChecks.push({ tab: state.tab, visible: await stateRoot.isVisible(), contentHash: crypto.createHash('sha256').update(stateText).digest('hex') });
}
await page.locator('.taf-models-tabs').getByRole('button', { name: '重要特征', exact: true }).click();
const explanationTabsStateful = await page.locator('.taf-models-tabs button.is-active').textContent() === '重要特征';
const explanationStatesDistinct = explanationStateChecks.length === explanationStates.length
  && explanationStateChecks.every((check) => check.visible)
  && new Set(explanationStateChecks.map((check) => check.contentHash)).size === explanationStates.length;

await page.screenshot({ path: screenshotPath, fullPage: false, scale: 'css' });

const focusStates = [
  { kind: 'activation-audit-gate', screenshot: stateScreenshotPaths.activationAuditGate },
  { kind: 'feature-important-features', screenshot: stateScreenshotPaths.importantFeatures },
  { kind: 'feature-rule-contribution', screenshot: stateScreenshotPaths.ruleContribution },
  { kind: 'feature-anomaly-explanation', screenshot: stateScreenshotPaths.anomalyExplanation },
  { kind: 'feature-sample-examples', screenshot: stateScreenshotPaths.sampleExamples },
];
const focusStateChecks = [];
const semanticVisualChecks = {};
for (const focusState of focusStates) {
  const focusUrl = new URL(`/models?ui_focus=${focusState.kind}&windowsCdpFocusTs=${Date.now()}`, baseUrl);
  focusUrl.hash = `codex_smoke_token=${authToken}`;
  await page.goto(focusUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  const focusRoot = page.locator(`.taf-models-focus[data-focus-state="${focusState.kind}"]`);
  await focusRoot.waitFor({ state: 'visible' });
  await focusRoot.getByText('postgresql', { exact: false }).waitFor({ state: 'visible' });
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  const box = await focusRoot.boundingBox();
  focusStateChecks.push({
    kind: focusState.kind,
    visible: await focusRoot.isVisible(),
    box,
    viewportCovered: Boolean(box && Math.abs(box.x) <= 1 && Math.abs(box.y) <= 1 && box.width >= 1919 && box.height >= 1079),
  });
  if (focusState.kind === 'feature-rule-contribution') {
    const text = await focusRoot.innerText();
    const canvasHash = await focusRoot.locator('canvas').first().evaluate((canvas) => {
      const bytes = canvas.getContext('2d')?.getImageData(0, 0, canvas.width, canvas.height).data ?? [];
      let colored = 0;
      for (let index = 0; index < bytes.length; index += 4) if (bytes[index] + bytes[index + 1] + bytes[index + 2] > 30) colored += 1;
      return { width: canvas.width, height: canvas.height, coloredPixels: colored };
    });
    semanticVisualChecks.ruleContribution = { positiveVisible: text.includes('+0.310'), negativeVisible: text.includes('-0.090') && text.includes('-0.060'), canvas: canvasHash };
  }
  if (focusState.kind === 'feature-sample-examples') {
    const colors = await focusRoot.locator('.ant-tag').evaluateAll((tags) => Object.fromEntries(tags.map((tag) => [tag.textContent?.trim() ?? '', getComputedStyle(tag).color])));
    semanticVisualChecks.sampleLabels = { colors, distinct: Boolean(colors.TP && colors.FP && colors.TP !== colors.FP) };
  }
  if (focusState.kind === 'activation-audit-gate') {
    const colors = await focusRoot.locator('.ant-tag').evaluateAll((tags) => Object.fromEntries(tags.map((tag) => [tag.textContent?.trim() ?? '', getComputedStyle(tag).color])));
    semanticVisualChecks.reviewGates = { colors, distinct: Boolean(colors['通过'] && colors['待审批'] && colors['通过'] !== colors['待审批']) };
  }
  await page.screenshot({ path: focusState.screenshot, fullPage: false, scale: 'css' });
}
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-models').waitFor({ state: 'visible' });
const mainLayoutChecks = await page.evaluate(() => {
  const main = document.querySelector('.taf-main');
  const right = document.querySelector('.taf-models-right');
  const recent = document.querySelector('.taf-models-ops');
  const bottom = document.querySelector('.taf-bottombar');
  const mainRect = main?.getBoundingClientRect();
  const rightRect = right?.getBoundingClientRect();
  const recentRect = recent?.getBoundingClientRect();
  const bottomRect = bottom?.getBoundingClientRect();
  return {
    mainScrollFree: Boolean(main && main.scrollHeight <= main.clientHeight + 1),
    rightWithinMain: Boolean(mainRect && rightRect && rightRect.bottom <= mainRect.bottom + 1),
    recentWithinMain: Boolean(mainRect && recentRect && recentRect.bottom <= mainRect.bottom + 1),
    rightAboveBottomBar: Boolean(rightRect && bottomRect && rightRect.bottom <= bottomRect.top + 1),
    main: mainRect ? { top: mainRect.top, bottom: mainRect.bottom, height: mainRect.height, scrollHeight: main?.scrollHeight, clientHeight: main?.clientHeight } : null,
    right: rightRect ? { top: rightRect.top, bottom: rightRect.bottom, height: rightRect.height } : null,
    recent: recentRect ? { top: recentRect.top, bottom: recentRect.bottom, height: recentRect.height } : null,
    bottomBar: bottomRect ? { top: bottomRect.top, bottom: bottomRect.bottom, height: bottomRect.height } : null,
  };
});
const focusStatesValid = focusStateChecks.length === focusStates.length && focusStateChecks.every((check) => check.visible && check.viewportCovered);
const semanticVisualsValid = semanticVisualChecks.ruleContribution?.positiveVisible
  && semanticVisualChecks.ruleContribution?.negativeVisible
  && semanticVisualChecks.ruleContribution?.canvas?.coloredPixels > 500
  && semanticVisualChecks.sampleLabels?.distinct
  && semanticVisualChecks.reviewGates?.distinct;
const expectedLiveActionCount = skipActions ? 0 : firstApiModel?.model_version ? 3 : 2;
const viewportMetrics = await page.evaluate(() => ({ innerWidth: window.innerWidth, innerHeight: window.innerHeight, devicePixelRatio: window.devicePixelRatio, visualViewportScale: window.visualViewport?.scale ?? 1 }));
const sourceFiles = [
  'go/control-plane/internal/rules/api/handler_models.go',
  'go/control-plane/internal/rules/repository/model_repository.go',
  'go/control-plane/internal/rules/service/model_service.go',
  'go/control-plane/internal/rules/service/model_applied_ack_contract_test.go',
  'go/control-plane/internal/rules/publisher/kafka_publisher.go',
  'go/control-plane/internal/common/kafka/consumer.go',
  'go/control-plane/internal/rules/config/config.go',
  'go/control-plane/cmd/rule-manager/main.go',
  'go/control-plane/internal/rules/model/model_registry.go',
  'go/control-plane/internal/rules/model/model_registry_test.go',
  'go/control-plane/internal/rules/service/model_action_validation_test.go',
  'web/ui/src/pages/ModelManagementPage.tsx',
  'web/ui/src/services/modelActionApi.ts',
  'web/ui/src/services/modelActionApi.test.ts',
  'web/ui/src/services/pageApiPlans.ts',
  'web/ui/src/services/pageApiPlans.test.ts',
  'web/ui/src/styles/pages.css',
  'web/ui/src/components/charts.tsx',
  'web/ui/src/components/StatusTag.tsx',
  'common/sql/pg/03-models-deploy.sql',
  'common/kafka/create-topics.sh',
  'java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/model/ModelUpdateEvent.java',
  'java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/model/ModelUpdateAppliedAck.java',
  'java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/model/MinioModelLoader.java',
  'java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/model/XGBoostModelWrapper.java',
  'java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/ModelRegistry.java',
  'java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/ModelUpdateBroadcastHandler.java',
  'java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/sink/ModelUpdateAckKafkaSinkFactory.java',
  'java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/BehaviorDetectionJob.java',
  'java/flink-jobs/flink-behavior-job/src/test/java/com/traffic/flink/behavior/detector/ModelUpdateBroadcastHandlerTest.java',
  'deployments/kubernetes/init-jobs/02-postgres-schema.yaml',
  'deployments/kubernetes/applications/go-services.yaml',
  'deployments/kubernetes/applications/web-ui.yaml',
  'deployments/kubernetes/infrastructure/07-flink.yaml',
  'deployments/kubernetes/image-digests.lock.json',
  'tests/e2e/ui_models_interactions.mjs',
  'tests/e2e/model_state_machine_preflight.mjs',
  'tests/e2e/model_visual_compare_windows_cdp.mjs',
];
const sourceDigest = crypto.createHash('sha256').update(sourceFiles.map((file) => fs.readFileSync(path.join(root, file))).join('\n')).digest('hex');
const deploymentImages = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'deploy', 'rule-manager', 'web-ui', '-o', 'jsonpath={range .items[*]}{.metadata.name}={.spec.template.spec.containers[0].image}{"\\n"}{end}'], { encoding: 'utf8', env: process.env, timeout: 15_000 }).trim().split('\n');
const deploymentProvenanceRaw = JSON.parse(execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'pods', '-l', 'app in (rule-manager,web-ui)', '-o', 'json'], { encoding: 'utf8', env: process.env, timeout: 15_000 }));
const deploymentProvenance = deploymentProvenanceRaw.items.filter((item) => item.status?.phase === 'Running').map((item) => ({
  pod: item.metadata.name,
  node: item.spec.nodeName,
  image: item.spec.containers[0]?.image,
  image_id: item.status.containerStatuses?.[0]?.imageID,
  ready: Boolean(item.status.containerStatuses?.[0]?.ready),
  restart_count: item.status.containerStatuses?.[0]?.restartCount ?? 0,
  build_id: item.metadata.annotations?.['traffic.analysis/build-id'] ?? '',
  source_digest: item.metadata.annotations?.['traffic.analysis/source-digest'] ?? '',
  image_digest: item.metadata.annotations?.['traffic.analysis/image-digest'] ?? '',
}));
const executionContextById = new Map(executionContexts.map((contextItem) => [contextItem.id, contextItem]));
const applicationRuntimeExceptions = runtimeExceptions.filter((exception) => {
  const contextItem = executionContextById.get(exception.executionContextId);
  return !String(contextItem?.origin ?? '').startsWith('chrome-extension://');
});
const applicationConsoleErrors = consoleErrors.filter((entry) => {
  const locationUrl = String(entry.location?.url ?? '');
  if (!locationUrl) return true;
  if (locationUrl.startsWith(baseUrl)) return true;
  return !externalRequestInitiators.some((request) => request.url === locationUrl
    && request.initiator?.stack?.callFrames?.some((frame) => String(frame.url ?? '').startsWith('chrome-extension://')));
});
const extensionNoise = {
  console_errors: consoleErrors.length - applicationConsoleErrors.length,
  runtime_exceptions: runtimeExceptions.length - applicationRuntimeExceptions.length,
};
const result = {
  result: modelApiResponse.ok()
    && apiTotal === renderedRows
    && noSimulationMarkers
    && totalMatchesApi
    && firstRowMatchesApi
    && (!firstApiModel?.model_version || firstVersionMatchesApi)
    && workbenchBackedByPostgresql
    && serverActivationGateEnforced
    && crossTenantUpdateDenied
    && selectedModelWorkbenchSwitch
    && chartCanvasCount >= 9
    && liveActionChecks.length === expectedLiveActionCount
    && liveActionChecks.every((check) => check.endpointVisible && check.auditVisible && check.accepted)
    && terminalActions.every((job) => job.status === 'completed')
    && actionAuditClosed
    && explanationTabsStateful
    && explanationStatesDistinct
    && focusStatesValid
    && semanticVisualsValid
    && mainLayoutChecks.mainScrollFree
    && mainLayoutChecks.rightWithinMain
    && mainLayoutChecks.recentWithinMain
    && mainLayoutChecks.rightAboveBottomBar
    && viewportMetrics.innerWidth === 1920
    && viewportMetrics.innerHeight === 1080
    && Math.abs(viewportMetrics.devicePixelRatio - 1) < 0.01
    && badResponses.length === 0
    && applicationConsoleErrors.length === 0
    && applicationRuntimeExceptions.length === 0
    && requestFailures.length === 0 ? 'pass' : 'fail',
  browser_path: 'Xshell tunnel -> 127.0.0.1:9224 -> Windows Chrome -> direct APISIX',
  browser: version.Browser,
  viewport: '1920x1080',
  viewport_metrics: viewportMetrics,
  started_at: startedAt,
  completed_at: new Date().toISOString(),
  skip_actions: skipActions,
  code_revision: execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim(),
  source_digest_sha256: sourceDigest,
  source_files: sourceFiles,
  deployment_images: deploymentImages,
  deployment_provenance: deploymentProvenance,
  model_api_status: modelApiResponse.status(),
  api_total: apiTotal,
  rendered_rows: renderedRows,
  no_simulation_markers: noSimulationMarkers,
  total_matches_api: totalMatchesApi,
  first_row_matches_api: firstRowMatchesApi,
  first_version_matches_api: firstVersionMatchesApi,
  workbench_api_status: workbenchApiResponse?.status() ?? 0,
  workbench_source: workbench?.source ?? '',
  workbench_counts: workbenchCounts,
  workbench_backed_by_postgresql: workbenchBackedByPostgresql,
  server_activation_gate: { enforced: serverActivationGateEnforced, status: activationGateResponse?.status() ?? 0, active_version: activeVersion, response_code: activationGatePayload?.code ?? '' },
  cross_tenant_update: { denied: crossTenantUpdateDenied, status: crossTenantUpdateResponse?.status() ?? 0, model_unchanged: modelAfterCrossTenantPayload?.data?.name === originalModelName },
  selected_model_workbench_switch: selectedModelWorkbenchSwitch,
  chart_canvas_count: chartCanvasCount,
  live_action_checks: liveActionChecks,
  terminal_action_checks: terminalActions,
  database_action_audits: auditRows,
  action_audit_closed: actionAuditClosed,
  explanation_tabs_stateful: explanationTabsStateful,
  explanation_states_distinct: explanationStatesDistinct,
  explanation_state_checks: explanationStateChecks,
  focus_states_valid: focusStatesValid,
  focus_state_checks: focusStateChecks,
  semantic_visuals_valid: semanticVisualsValid,
  semantic_visual_checks: semanticVisualChecks,
  main_layout_checks: mainLayoutChecks,
  bad_responses: badResponses,
  console_errors: consoleErrors,
  application_console_errors: applicationConsoleErrors,
  page_errors: pageErrors,
  application_runtime_exceptions: applicationRuntimeExceptions,
  extension_noise: extensionNoise,
  request_failures: requestFailures,
  external_request_initiators: externalRequestInitiators,
  runtime_exceptions: runtimeExceptions,
  execution_contexts: executionContexts,
  screenshot: path.relative(root, screenshotPath),
  state_screenshots: Object.fromEntries(Object.entries(stateScreenshotPaths).map(([key, value]) => [key, path.relative(root, value)])),
};
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify({ outputPath, result: result.result, apiTotal, renderedRows, liveActions: liveActionChecks.length }, null, 2));
await page.bringToFront();
await browser.close();
if (result.result !== 'pass') process.exitCode = 1;

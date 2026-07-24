#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

const root = process.cwd();
const require = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = require('@playwright/test');
const baseUrl = process.env.UI_BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.UI_CDP_URL || 'http://127.0.0.1:9224';
const artifactRevision = process.env.FUSION_ARTIFACT_REVISION || 'r601';
const outputDir = path.join(root, 'evidence/ui-image-breakdowns/pages/fusion');
const outputPath = path.join(root, `doc/02_acceptance/02-regression/fusion-interactions-${artifactRevision}.json`);
const screenshotPath = path.join(outputDir, `fusion-${artifactRevision}-interactions.png`);
const responsiveScreenshotPath = path.join(outputDir, `fusion-${artifactRevision}-responsive-1536.png`);
const compactScreenshotPath = path.join(outputDir, `fusion-${artifactRevision}-responsive-1366.png`);
const ruleModalScreenshotPath = path.join(outputDir, `fusion-${artifactRevision}-rule-modal.png`);
const conflictDetailScreenshotPath = path.join(outputDir, `fusion-${artifactRevision}-conflict-detail.png`);
const repairRetainedScreenshotPath = path.join(outputDir, `fusion-${artifactRevision}-repair-retained.png`);
const postActionScreenshotPath = path.join(outputDir, `fusion-${artifactRevision}-post-actions.png`);
fs.mkdirSync(outputDir, { recursive: true });

const commandEnv = { ...process.env };
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete commandEnv[key];
const fusionFixtureSql = fs.readFileSync(path.join(root, 'common/sql/pg/07-fusion-acceptance.sql'), 'utf8');

const secretValue = (key) => Buffer.from(execFileSync('kubectl', [
  '-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', `jsonpath={.data.${key}}`,
], { encoding: 'utf8', env: commandEnv, timeout: 15_000 }), 'base64').toString('utf8');

const resetAcceptanceFixture = () => execFileSync('kubectl', [
  '-n', 'databases', 'exec', '-i', 'postgres-primary-0', '--', 'env',
  `PGPASSWORD=${secretValue('PG_PASSWORD')}`,
  'PGOPTIONS=-c traffic.enable_fusion_acceptance_fixture=on -c traffic.fusion_acceptance_tenant_id=default -c traffic.fusion_acceptance_fixture_action=seed',
  'psql', '-U', 'postgres', '-d', 'traffic_platform', '-v', 'ON_ERROR_STOP=1',
], { input: fusionFixtureSql, encoding: 'utf8', env: commandEnv, timeout: 45_000 });

const queryPostgres = (sql) => {
  try {
    return execFileSync('kubectl', [
      '-n', 'databases', 'exec', '-i', 'postgres-primary-0', '--', 'env',
      `PGPASSWORD=${secretValue('PG_PASSWORD')}`,
      'psql', '-U', 'postgres', '-d', 'traffic_platform', '-At', '-v', 'ON_ERROR_STOP=1', '-c', sql,
    ], { encoding: 'utf8', env: commandEnv, timeout: 30_000, stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  } catch {
    throw new Error('Fusion repair-task database verification failed');
  }
};

const sqlLiteral = (value) => String(value).replaceAll("'", "''");

const jwt = ({
  username = 'codex-windows-cdp-fusion',
  roles = ['admin'],
  permissions = ['*', 'admin:*', 'alert:read', 'rule:read', 'rule:write', 'audit:read', 'graph:read'],
} = {}) => {
  const secret = Buffer.from(execFileSync('kubectl', [
    '-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}',
  ], { encoding: 'utf8', env: commandEnv, timeout: 15_000 }), 'base64').toString('utf8');
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username,
    roles,
    permissions,
    token_type: 'access',
    session_id: `fusion-${artifactRevision}-${crypto.randomUUID()}`,
    iat: now,
    exp: now + 1800,
  })).toString('base64url');
  const signingInput = `${header}.${claims}`;
  const signature = crypto.createHmac('sha256', secret).update(signingInput).digest('base64url');
  return `${signingInput}.${signature}`;
};

const redact = (value) => String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
const unwrap = (payload) => payload?.data ?? payload;

const [versionResponse, targetsResponse] = await Promise.all([
  fetch(`${cdpUrl}/json/version`),
  fetch(`${cdpUrl}/json/list`),
]);
if (!versionResponse.ok || !targetsResponse.ok) {
  throw new Error(`Windows Chrome CDP 9224 preflight failed: ${versionResponse.status}/${targetsResponse.status}`);
}
const version = await versionResponse.json();
const targets = await targetsResponse.json();
if (!version.webSocketDebuggerUrl) throw new Error('Windows Chrome CDP metadata missing webSocketDebuggerUrl');
const fixtureSeedOutput = resetAcceptanceFixture();
let fixtureMutated = false;
process.on('exit', () => {
  if (!fixtureMutated) return;
  try {
    resetAcceptanceFixture();
  } catch (error) {
    process.stderr.write(`Failed to restore Fusion acceptance fixture during process exit: ${error instanceof Error ? error.message : String(error)}\n`);
  }
});

const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: baseUrl }).catch(() => {});
for (const stalePage of context.pages()) {
  if (/fusionCdpTs=/.test(stalePage.url())) await stalePage.close().catch(() => {});
}
const page = await context.newPage();
await page.bringToFront();
await page.setViewportSize({ width: 1920, height: 1080 });
page.setDefaultTimeout(15_000);
const pageCDP = await context.newCDPSession(page);
const verifiedExtensionRequests = [];
pageCDP.on('Network.requestWillBeSent', (event) => {
  if (event.request.url !== 'https://api.yhchj.com/ip') return;
  const frames = event.initiator?.stack?.callFrames ?? [];
  const extensionFrame = frames.find((frame) => frame.url.startsWith('chrome-extension://'));
  if (extensionFrame) {
    verifiedExtensionRequests.push({
      url: event.request.url,
      extension_url: extensionFrame.url,
      function_name: extensionFrame.functionName,
    });
  }
});
await pageCDP.send('Network.enable');
await page.goto(new URL(`/login?fusionSessionPreserveTs=${Date.now()}`, baseUrl).toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
const preservedSession = await page.evaluate(() => ({
  token: window.localStorage.getItem('traffic-ui-token'),
  refreshToken: window.localStorage.getItem('traffic-ui-refresh-token'),
}));

const badResponses = [];
const requestFailures = [];
const consoleErrors = [];
const pageErrors = [];
const businessResponses = [];
page.on('response', (response) => {
  if (!response.url().startsWith(baseUrl)) return;
  if (response.url().includes('/api/v1/fusion') || response.url().includes('/api/v1/threat-intel')) {
    businessResponses.push({ status: response.status(), method: response.request().method(), url: redact(response.url()) });
  }
  if (response.status() >= 400) badResponses.push({ status: response.status(), method: response.request().method(), url: redact(response.url()) });
});
page.on('requestfailed', (request) => requestFailures.push({ url: redact(request.url()), failure: request.failure()?.errorText ?? '' }));
page.on('console', (message) => {
  if (message.type() !== 'error') return;
  const location = message.location();
  consoleErrors.push({ text: message.text(), url: location.url ?? '', line: location.lineNumber ?? location.line ?? 0 });
});
page.on('pageerror', (error) => pageErrors.push(error.message));

const routeUrl = new URL(`/fusion?fusionCdpTs=${Date.now()}`, baseUrl);
const adminToken = jwt();
routeUrl.hash = `codex_smoke_token=${adminToken}`;
const workbenchResponsePromise = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/fusion/workbench' && response.status() === 200, { timeout: 45_000 });
const threatIntelResponsePromise = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/threat-intel/entries' && response.status() === 200, { timeout: 45_000 });
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.bringToFront();
const [workbenchResponse] = await Promise.all([workbenchResponsePromise, threatIntelResponsePromise]);
const workbench = unwrap(await workbenchResponse.json());

await page.getByRole('heading', { name: '数据融合', exact: true }).waitFor({ state: 'visible' });
await page.getByText('多源融合编排（映射与对齐流程）', { exact: true }).waitFor({ state: 'visible' });
await page.getByText('融合规则管理（共 26 条）', { exact: true }).waitFor({ state: 'visible' });
await page.getByText('冲突队列（待处理 18 条）', { exact: true }).waitFor({ state: 'visible' });
await page.screenshot({ path: screenshotPath, fullPage: false });
const acceptanceFixtureBadgeVisible = await page.locator('.taf-fusion-fixture-badge').getByText('验收数据', { exact: true }).isVisible();

const sourceCards = page.locator('.taf-fusion-source');
const sourceCanvasCount = await sourceCards.locator('.taf-fusion-source-echart canvas').count();
const sourceEchartsDeclaredCount = await sourceCards.locator('.taf-fusion-source-echart[data-chart-engine="echarts"][data-series-type="line"]').count();
const pipelineCanvasCount = await page.locator('.taf-fusion-pipeline-connections canvas').count();
const inspectPipelineGeometry = () => page.locator('.taf-fusion-pipeline-connections').evaluate((chart) => {
  const stage = chart.closest('.taf-fusion-pipeline-stage');
  const stageRect = stage?.getBoundingClientRect();
  const payload = JSON.parse(chart.getAttribute('data-geometry-links') || '{}');
  const rects = (selector) => [...(stage?.querySelectorAll(selector) ?? [])].map((node) => node.getBoundingClientRect());
  const sources = rects('[data-fusion-source]');
  const rules = rects('[data-fusion-rule]');
  const outputs = rects('[data-fusion-output]');
  if (!stageRect || !sources.length || !rules.length || !outputs.length) return { valid: false, width: 0, height: 0 };
  const close = (left, right) => Math.abs(left - right) <= 1.25;
  const sourceValid = payload.sourceLinks?.every((link, index) => close(link.start[0], sources[index].right - stageRect.left) && close(link.end[0], rules[0].left - stageRect.left));
  const ruleValid = payload.ruleLinks?.every((link, index) => close(link.start[0], rules[index].right - stageRect.left) && close(link.end[0], rules[index + 1].left - stageRect.left));
  const outputValid = payload.outputLinks?.every((link, index) => close(link.start[0], rules.at(-1).right - stageRect.left) && close(link.end[0], outputs[index].left - stageRect.left));
  const allRects = [...sources, ...rules, ...outputs];
  const contained = allRects.every((rect) => rect.left >= stageRect.left - 1.25 && rect.right <= stageRect.right + 1.25 && rect.top >= stageRect.top - 1.25 && rect.bottom <= stageRect.bottom + 1.25);
  const overlapCount = (items) => items.reduce((count, rect, index) => count + items.slice(index + 1).filter((other) => (
    rect.left < other.right - 1 && rect.right > other.left + 1 && rect.top < other.bottom - 1 && rect.bottom > other.top + 1
  )).length, 0);
  return {
    valid: Boolean(sourceValid && ruleValid && outputValid && contained),
    contained,
    overlapPairs: overlapCount(sources) + overlapCount(rules) + overlapCount(outputs),
    width: Number(chart.getAttribute('data-geometry-width')),
    height: Number(chart.getAttribute('data-geometry-height')),
    sourceLinks: payload.sourceLinks?.length ?? 0,
    ruleLinks: payload.ruleLinks?.length ?? 0,
    outputLinks: payload.outputLinks?.length ?? 0,
  };
});
const pipelineGeometryWide = await inspectPipelineGeometry();
const pipelineRuleNames = await page.locator('.taf-fusion-rule-node strong').allTextContents();
const pipelineRuleMiniCharts = page.locator('.taf-fusion-rule-mini-chart[data-chart-engine="echarts"][data-series-type="bar"]');
const pipelineRuleMiniChartCanvasCount = await pipelineRuleMiniCharts.locator('canvas').count();
const pipelineRuleMiniChartValues = await pipelineRuleMiniCharts.evaluateAll((charts) => charts.map((chart) => JSON.parse(chart.getAttribute('data-series-values') || '[]')));
const inspectResponsiveLayout = () => page.evaluate(() => {
  const stage = document.querySelector('.taf-fusion-pipeline-stage');
  const grid = document.querySelector('.taf-fusion-grid');
  const main = document.querySelector('.taf-fusion-main');
  const detail = document.querySelector('.taf-fusion-detail');
  const labels = [...document.querySelectorAll('.taf-fusion-pipeline-labels > strong')];
  const inViewport = (node) => {
    const rect = node?.getBoundingClientRect();
    return Boolean(rect && rect.left >= -1 && rect.right <= window.innerWidth + 1);
  };
  return {
    viewport: { width: window.innerWidth, height: window.innerHeight },
    noDocumentHorizontalOverflow: document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
    noStageHorizontalOverflow: Boolean(stage && stage.scrollWidth <= stage.clientWidth + 1),
    labelsInViewport: labels.length === 3 && labels.every(inViewport),
    gridInViewport: inViewport(grid),
    mainInViewport: inViewport(main),
    detailInViewport: !detail || inViewport(detail),
    stageWidth: stage?.getBoundingClientRect().width ?? 0,
  };
});
await page.setViewportSize({ width: 1536, height: 864 });
await page.waitForFunction((wideWidth) => Number(document.querySelector('.taf-fusion-pipeline-connections')?.getAttribute('data-geometry-width') || 0) < wideWidth - 10, pipelineGeometryWide.width);
const pipelineGeometryMedium = await inspectPipelineGeometry();
const responsiveLayoutMedium = await inspectResponsiveLayout();
await page.screenshot({ path: responsiveScreenshotPath, fullPage: false });
await page.setViewportSize({ width: 1366, height: 768 });
await page.waitForFunction((previousWidth) => Math.abs(Number(document.querySelector('.taf-fusion-pipeline-connections')?.getAttribute('data-geometry-width') || 0) - previousWidth) > 10, pipelineGeometryMedium.width);
const pipelineGeometryCompact = await inspectPipelineGeometry();
const responsiveLayoutCompact = await inspectResponsiveLayout();
await page.screenshot({ path: compactScreenshotPath, fullPage: false });
await page.setViewportSize({ width: 1920, height: 1080 });
await page.waitForFunction((wideWidth) => Math.abs(Number(document.querySelector('.taf-fusion-pipeline-connections')?.getAttribute('data-geometry-width') || 0) - wideWidth) <= 1.25, pipelineGeometryWide.width);
const initialRuleRowCount = await page.locator('.taf-fusion-workbench .ant-table-tbody > tr.ant-table-row').count();
const initialRuleIds = await page.locator('.taf-fusion-workbench .ant-table-tbody > tr.ant-table-row .taf-fusion-object').allTextContents();
const longestRuleIdIndex = initialRuleIds.reduce((longest, value, index, values) => value.length > values[longest].length ? index : longest, 0);
await page.locator('.taf-fusion-workbench .ant-table-tbody > tr.ant-table-row .taf-fusion-object').nth(longestRuleIdIndex).hover();
const ruleIdTooltip = page.locator('.ant-tooltip-inner').getByText(initialRuleIds[longestRuleIdIndex], { exact: true });
await ruleIdTooltip.waitFor({ state: 'visible', timeout: 3_000 });
const ruleIdTooltipVisible = await ruleIdTooltip.isVisible();
const riskLabels = await page.locator('.taf-fusion-risk-counts b').allTextContents();
const auditHeaders = await page.locator('.taf-fusion-audit-head > span').allTextContents();
const auditRowCount = await page.locator('.taf-fusion-audit > button').count();
const detailActionLabels = await page.locator('.taf-fusion-drawer-actions button').allTextContents();
await page.locator('.taf-fusion-detail').screenshot({ path: conflictDetailScreenshotPath });

const pageTwo = page.locator('.taf-fusion-rule-pagination .ant-pagination-item-2');
const rulePageTwoResponsePromise = page.waitForResponse((response) => {
  const url = new URL(response.url());
  return url.pathname === '/api/v1/fusion/workbench' && url.searchParams.get('rule_offset') === '6' && response.status() === 200;
});
await pageTwo.click();
const rulePageTwoResponse = await rulePageTwoResponsePromise;
const rulePageTwoPayload = unwrap(await rulePageTwoResponse.json());
await page.locator('.taf-fusion-rule-pagination .ant-pagination-item-2.ant-pagination-item-active').waitFor({ state: 'visible' });
const pageTwoRuleRowCount = await page.locator('.taf-fusion-workbench .ant-table-tbody > tr.ant-table-row').count();
const pageTwoRuleIds = await page.locator('.taf-fusion-workbench .ant-table-tbody > tr.ant-table-row .taf-fusion-object').allTextContents();
await page.locator('.taf-fusion-rule-pagination .ant-pagination-item-1').click();
await page.locator('.taf-fusion-rule-pagination .ant-pagination-item-1.ant-pagination-item-active').waitFor({ state: 'visible' });
await page.waitForFunction((expected) => document.querySelector('.taf-fusion-workbench .ant-table-tbody > tr.ant-table-row .taf-fusion-object')?.textContent?.trim() === expected, initialRuleIds[0]);

const firstRuleActions = page.locator('.taf-fusion-workbench .ant-table-tbody > tr.ant-table-row').first().locator('.taf-fusion-rule-actions button');
await firstRuleActions.nth(1).click();
await page.getByText('规则 ID 已复制', { exact: true }).waitFor({ state: 'visible' });
const ruleCopyWorking = true;
const ruleRefreshResponsePromise = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/fusion/workbench' && response.status() === 200);
await firstRuleActions.nth(3).click();
await ruleRefreshResponsePromise;
const ruleRefreshWorking = true;
await firstRuleActions.nth(2).click();
await page.waitForURL((url) => url.pathname === '/audit-log' && url.searchParams.get('object_type') === 'fusion_rule', { timeout: 15_000 });
const ruleAuditNavigationWorking = new URLSearchParams(new URL(page.url()).search).get('object_id') === initialRuleIds[0];
await page.goBack({ waitUntil: 'domcontentloaded' });
await page.getByRole('heading', { name: '数据融合', exact: true }).waitFor({ state: 'visible' });
await page.waitForFunction((expected) => document.querySelector('.taf-fusion-workbench .ant-table-tbody > tr.ant-table-row .taf-fusion-object')?.textContent?.trim() === expected, initialRuleIds[0]);

const sourceHeaderNoOverlap = await sourceCards.evaluateAll((cards) => cards.every((card) => {
  const title = card.querySelector('strong')?.getBoundingClientRect();
  const status = card.querySelector('em')?.getBoundingClientRect();
  return Boolean(title && status && title.right <= status.left - 2);
}));

const initialConflictRows = page.locator('.taf-fusion-conflicts > button');
const initialConflictRowCount = await initialConflictRows.count();
const initialConflictIds = await initialConflictRows.locator('strong').allTextContents();
const conflictPageTwoResponsePromise = page.waitForResponse((response) => {
  const url = new URL(response.url());
  return url.pathname === '/api/v1/fusion/workbench' && url.searchParams.get('conflict_offset') === '3' && response.status() === 200;
});
await page.locator('.taf-fusion-conflicts .ant-pagination-item-2').click();
await conflictPageTwoResponsePromise;
await page.locator('.taf-fusion-conflicts .ant-pagination-item-2.ant-pagination-item-active').waitFor({ state: 'visible' });
await page.waitForFunction((expected) => document.querySelector('.taf-fusion-conflicts > button strong')?.textContent?.trim() !== expected, initialConflictIds[0]);
const pageTwoConflictRowCount = await page.locator('.taf-fusion-conflicts > button').count();
const pageTwoConflictIds = await page.locator('.taf-fusion-conflicts > button strong').allTextContents();
const conflictLastPageResponsePromise = page.waitForResponse((response) => {
  const url = new URL(response.url());
  return url.pathname === '/api/v1/fusion/workbench' && url.searchParams.get('conflict_offset') === '15' && response.status() === 200;
});
await page.locator('.taf-fusion-conflicts .ant-pagination-item-6').click();
await conflictLastPageResponsePromise;
await page.locator('.taf-fusion-conflicts .ant-pagination-item-6.ant-pagination-item-active').waitFor({ state: 'visible' });
const lastPageConflictRowCount = await page.locator('.taf-fusion-conflicts > button').count();
const lastPageConflictIds = await page.locator('.taf-fusion-conflicts > button strong').allTextContents();
await page.locator('.taf-fusion-conflicts .ant-pagination-item-1').click();
await page.locator('.taf-fusion-conflicts .ant-pagination-item-1.ant-pagination-item-active').waitFor({ state: 'visible' });
await page.waitForFunction((expected) => document.querySelector('.taf-fusion-conflicts > button strong')?.textContent?.trim() === expected, initialConflictIds[0]);

const sourceChoices = page.locator('.taf-fusion-value-compare button');
const firstSourceLabel = await sourceChoices.first().locator('span').textContent();
if (await sourceChoices.count() > 1) await sourceChoices.nth(1).click();
await page.locator('.taf-fusion-resolution textarea').fill('本地未保存状态，必须由恢复操作清除。');
const restoreResponsePromise = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/fusion/workbench' && response.status() === 200);
await page.getByRole('button', { name: '重置页面状态', exact: true }).click();
await restoreResponsePromise;
const restoredSourceLabel = await page.locator('.taf-fusion-value-compare button.is-selected span').textContent();
const restoredNote = await page.locator('.taf-fusion-resolution textarea').inputValue();
const resetPageStateWorking = restoredSourceLabel === firstSourceLabel && restoredNote === '按权威来源确认主值，保留原始来源与审计链。';

fixtureMutated = true;
await page.locator('.taf-fusion-workbench .ant-table-tbody > tr.ant-table-row').first().locator('.taf-fusion-rule-actions button').first().click();
const modal = page.locator('.ant-modal.taf-fusion-rule-modal');
await modal.waitFor({ state: 'visible' });
await page.screenshot({ path: ruleModalScreenshotPath, fullPage: false });
const ruleVersionText = await modal.locator('.taf-fusion-rule-modal-title span').textContent();
const previousRuleVersion = Number(ruleVersionText.match(/v(\d+)/)?.[1] ?? 0);
const note = `Windows Chrome ${artifactRevision} ${Date.now()}`;
await modal.locator('textarea').fill(note);
const ruleUpdateResponsePromise = page.waitForResponse((response) => response.request().method() === 'PATCH' && /\/api\/v1\/fusion\/rules\//.test(response.url()));
await modal.getByRole('button', { name: '保存新版本', exact: true }).click();
const ruleUpdateResponse = await ruleUpdateResponsePromise;
const ruleUpdatePayload = unwrap(await ruleUpdateResponse.json());
await modal.waitFor({ state: 'hidden' });
const persistedRule = ruleUpdatePayload?.rule;
const ruleDatabaseRecord = persistedRule?.rule_id
  ? queryPostgres(`SELECT rule_id || '|' || version::text || '|' || COALESCE(note, '') FROM fusion_rule_overrides WHERE tenant_id='default' AND rule_id='${sqlLiteral(persistedRule.rule_id)}'`)
  : '';
const staleRuleResponse = persistedRule?.rule_id
  ? await context.request.patch(new URL(`/api/v1/fusion/rules/${encodeURIComponent(persistedRule.rule_id)}`, baseUrl).toString(), {
      headers: { Authorization: `Bearer ${adminToken}`, 'X-Tenant-ID': 'default', 'Content-Type': 'application/json' },
      data: {
        status: persistedRule.status,
        strategy: persistedRule.strategy,
        confidence_threshold: persistedRule.confidence_threshold,
        note,
        expected_version: previousRuleVersion,
      },
    })
  : undefined;

const processedConflictId = initialConflictIds[0]?.trim();
const conflictResolveResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST' && /\/api\/v1\/fusion\/conflicts\/.+\/resolve$/.test(new URL(response.url()).pathname));
const conflictRefreshResponsePromise = page.waitForResponse((response) => {
  const url = new URL(response.url());
  return response.request().method() === 'GET' && url.pathname === '/api/v1/fusion/workbench' && url.searchParams.get('conflict_offset') === '0' && response.status() === 200;
});
await page.locator('.taf-fusion-detail').getByRole('button', { name: '创建修复任务', exact: true }).click();
const conflictResolveResponse = await conflictResolveResponsePromise;
const conflictResolvePayload = unwrap(await conflictResolveResponse.json());
await conflictRefreshResponsePromise;
await page.waitForFunction(() => !document.querySelector('.taf-fusion-detail button.ant-btn-loading'), undefined, { timeout: 45_000 });
await page.locator('.taf-fusion-detail').getByText(`冲突处理 #${processedConflictId}`, { exact: true }).waitFor({ state: 'visible' });
const processedConflictRetained = (await page.locator('.taf-fusion-detail').textContent())?.includes('修复中') === true;
const repairButtonDisabledAfterWrite = await page.locator('.taf-fusion-detail').getByRole('button', { name: '修复任务已创建', exact: true }).isDisabled();
const confirmButtonDisabledAfterWrite = await page.locator('.taf-fusion-detail').getByRole('button', { name: '确认主值', exact: true }).isDisabled();
await page.screenshot({ path: repairRetainedScreenshotPath, fullPage: false });
const repairTaskId = conflictResolvePayload?.repair_task?.task_id ?? '';
const repairTaskDatabaseRecord = /^[0-9a-f-]{36}$/i.test(repairTaskId)
  ? queryPostgres(`SELECT task_id::text || '|fusion_conflict_repair|' || status || '|' || conflict_id FROM fusion_repair_tasks WHERE tenant_id='default' AND task_id='${repairTaskId}'::uuid`)
  : '';

const evidenceButton = page.locator('.taf-fusion-detail .taf-fusion-drawer-actions button').filter({ hasText: '输出到验收证据' });
const evidenceButtonHitTarget = await evidenceButton.evaluate((button) => {
  const rect = button.getBoundingClientRect();
  const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2);
  return rect.width > 0 && rect.height > 0 && rect.top >= 0 && rect.bottom <= window.innerHeight && hit?.closest('button') === button;
});
const exportEvidence = async () => {
  await page.waitForFunction(() => {
    const button = [...document.querySelectorAll('.taf-fusion-detail button')].find((item) => item.textContent?.includes('输出到验收证据'));
    return button instanceof HTMLButtonElement && !button.disabled && !button.classList.contains('ant-btn-loading');
  });
  const responsePromise = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/fusion/evidence-packages');
  const browserDownloadPromise = page.waitForEvent('download');
  await evidenceButton.click({ force: true });
  const [response, browserDownload] = await Promise.all([responsePromise, browserDownloadPromise]);
  await page.waitForFunction(() => {
    const button = [...document.querySelectorAll('.taf-fusion-detail button')].find((item) => item.textContent?.includes('输出到验收证据'));
    return button instanceof HTMLButtonElement && !button.disabled && !button.classList.contains('ant-btn-loading');
  });
  return { response, browserDownload };
};
const { response: evidenceResponse, browserDownload: download } = await exportEvidence();
const evidencePayload = unwrap(await evidenceResponse.json());
const evidenceDocument = JSON.parse(Buffer.from(evidencePayload.content_base64, 'base64').toString('utf8'));
const downloadFilename = download.suggestedFilename();
for (let index = 0; index < 3; index += 1) {
  const { response } = await exportEvidence();
  if (response.status() !== 200) throw new Error(`extra evidence export ${index + 1} failed with ${response.status()}`);
}

const auditListResponsePromise = page.waitForResponse((response) => {
  const url = new URL(response.url());
  return response.request().method() === 'GET'
    && url.pathname === '/api/v1/audit/logs'
    && url.searchParams.get('object_type') === 'fusion_conflict'
    && url.searchParams.get('object_id') === processedConflictId;
});
await page.locator('.taf-fusion-detail').getByRole('button', { name: '查看审计记录', exact: true }).click();
await page.waitForURL((url) => url.pathname === '/audit-log' && url.searchParams.get('object_type') === 'fusion_conflict', { timeout: 15_000 });
const auditNavigation = { pathname: new URL(page.url()).pathname, search: new URL(page.url()).search };
const auditListResponse = await auditListResponsePromise;
const auditListPayload = unwrap(await auditListResponse.json());
const auditListRecords = auditListPayload?.trails ?? [];
const auditObjectFilterVisible = await page.locator('.taf-auditlog-search label').filter({ hasText: '对象类型' }).locator('.ant-select-selection-item').getByText('融合冲突', { exact: true }).isVisible();
await page.goBack({ waitUntil: 'domcontentloaded' });
await page.getByRole('heading', { name: '数据融合', exact: true }).waitFor({ state: 'visible' });
await page.locator('.taf-fusion-detail').waitFor({ state: 'visible' });
await page.getByText(/融合事件审计（近 \d+ 条）/).waitFor({ state: 'visible' });
const confirmConflictId = page.locator('.taf-fusion-conflicts > button strong').nth(1);
const confirmConflictIdText = (await confirmConflictId.textContent())?.trim() ?? '';
await confirmConflictId.click();
const confirmResponsePromise = page.waitForResponse((response) => response.request().method() === 'POST' && /\/api\/v1\/fusion\/conflicts\/.+\/resolve$/.test(new URL(response.url()).pathname));
await page.locator('.taf-fusion-detail').getByRole('button', { name: '确认主值', exact: true }).click();
const confirmResponse = await confirmResponsePromise;
const confirmPayload = unwrap(await confirmResponse.json());
const authoritativeConfirmWorking = confirmResponse.status() === 200 && confirmPayload?.resolution?.conflict_id === confirmConflictIdText && confirmPayload?.resolution?.strategy === 'authoritative-source';
const postActionAuditRowCount = await page.locator('.taf-fusion-audit > button').count();
const auditPageTwoButton = page.locator('.taf-fusion-audit .ant-pagination-item-2');
const auditPaginationVisible = await auditPageTwoButton.isVisible();
let pageTwoAuditRowCount = 0;
if (auditPaginationVisible) {
  const auditPageTwoResponsePromise = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === '/api/v1/fusion/workbench' && url.searchParams.get('audit_offset') === '5' && response.status() === 200;
  });
  await auditPageTwoButton.click();
  await auditPageTwoResponsePromise;
  await page.locator('.taf-fusion-audit .ant-pagination-item-2.ant-pagination-item-active').waitFor({ state: 'visible' });
  pageTwoAuditRowCount = await page.locator('.taf-fusion-audit > button').count();
  await page.locator('.taf-fusion-audit .ant-pagination-item-1').click();
  await page.locator('.taf-fusion-audit .ant-pagination-item-1.ant-pagination-item-active').waitFor({ state: 'visible' });
}
await page.screenshot({ path: postActionScreenshotPath, fullPage: false });

const readerPage = await context.newPage();
await readerPage.setViewportSize({ width: 1920, height: 1080 });
readerPage.setDefaultTimeout(15_000);
const readerRouteUrl = new URL(`/fusion?fusionCdpTs=${Date.now()}-reader`, baseUrl);
const readerToken = jwt({
  username: 'codex-windows-cdp-fusion-reader',
  roles: ['auditor'],
  permissions: ['alert:read', 'audit:read', 'graph:read', 'rule:read'],
});
readerRouteUrl.hash = `codex_smoke_token=${readerToken}`;
await readerPage.goto(readerRouteUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await readerPage.getByRole('heading', { name: '数据融合', exact: true }).waitFor({ state: 'visible' });
const readerRuleEditDisabled = await readerPage.locator('.taf-fusion-titlebar button').filter({ hasText: '规则编辑' }).isDisabled();
const readerTableEditDisabled = await readerPage.locator('.taf-fusion-workbench .ant-table-tbody > tr.ant-table-row').first().locator('.taf-fusion-rule-actions button').first().isDisabled();
const readerDetailActions = readerPage.locator('.taf-fusion-detail .taf-fusion-drawer-actions button');
const readerConfirmDisabled = await readerDetailActions.filter({ hasText: '确认主值' }).isDisabled();
const readerRepairDisabled = await readerDetailActions.filter({ hasText: '创建修复任务' }).isDisabled();
const readerExportDisabled = await readerDetailActions.filter({ hasText: '输出到验收证据' }).isDisabled();
const readerAuditEnabled = await readerDetailActions.filter({ hasText: '查看审计记录' }).isEnabled();
const readerHeaders = { Authorization: `Bearer ${readerToken}`, 'X-Tenant-ID': 'default', 'Content-Type': 'application/json' };
const readerRuleResponse = await context.request.patch(new URL(`/api/v1/fusion/rules/${encodeURIComponent(persistedRule?.rule_id ?? initialRuleIds[0])}`, baseUrl).toString(), {
  headers: readerHeaders,
  data: { status: persistedRule?.status ?? 'active', strategy: persistedRule?.strategy ?? 'weighted-confidence', confidence_threshold: persistedRule?.confidence_threshold ?? 0.9, note: 'reader must be denied', expected_version: persistedRule?.version ?? 1 },
});
const readerConflict = workbench.conflicts?.[1] ?? workbench.conflicts?.[0];
const readerSource = readerConflict?.source_values?.[0] ?? { source: 'unknown', value: 'unknown' };
const readerConflictResponse = await context.request.post(new URL(`/api/v1/fusion/conflicts/${encodeURIComponent(readerConflict?.conflict_id ?? processedConflictId)}/resolve`, baseUrl).toString(), {
  headers: readerHeaders,
  data: {
    object_id: readerConflict?.object_id ?? 'reader-denied',
    object_type: readerConflict?.object_type ?? 'entity',
    field_name: readerConflict?.field_name ?? 'name',
    selected_source: readerSource.source,
    selected_value: readerSource.value,
    strategy: 'authoritative-source',
    note: 'reader must be denied',
    rule_id: readerConflict?.rule_id,
    expected_state_version: readerConflict?.state_version ?? 1,
  },
});
const readerEvidenceResponse = await context.request.post(new URL('/api/v1/fusion/evidence-packages', baseUrl).toString(), {
  headers: readerHeaders,
  data: { conflict_id: readerConflict?.conflict_id ?? processedConflictId },
});
await readerPage.close().catch(() => {});
await page.evaluate(({ token, refreshToken }) => {
  if (token === null) window.localStorage.removeItem('traffic-ui-token');
  else window.localStorage.setItem('traffic-ui-token', token);
  if (refreshToken === null) window.localStorage.removeItem('traffic-ui-refresh-token');
  else window.localStorage.setItem('traffic-ui-refresh-token', refreshToken);
}, preservedSession);
const sessionRestored = await page.evaluate(({ token, refreshToken }) => (
  window.localStorage.getItem('traffic-ui-token') === token
  && window.localStorage.getItem('traffic-ui-refresh-token') === refreshToken
), preservedSession);

const layout = await page.evaluate(() => {
  const detail = document.querySelector('.taf-fusion-detail')?.getBoundingClientRect();
  const grid = document.querySelector('.taf-fusion-grid')?.getBoundingClientRect();
  const main = document.querySelector('.taf-fusion-main')?.getBoundingClientRect();
  const shell = document.querySelector('.taf-app-content') ?? document.querySelector('main') ?? document.body;
  const rootStyle = getComputedStyle(document.documentElement);
  return {
    viewport: { width: window.innerWidth, height: window.innerHeight },
    document: { scrollWidth: document.documentElement.scrollWidth, clientWidth: document.documentElement.clientWidth, scrollHeight: document.documentElement.scrollHeight },
    detail: detail && { left: detail.left, top: detail.top, right: detail.right, bottom: detail.bottom, width: detail.width, height: detail.height },
    grid: grid && { left: grid.left, top: grid.top, right: grid.right, bottom: grid.bottom, width: grid.width, height: grid.height },
    main: main && { left: main.left, top: main.top, right: main.right, bottom: main.bottom, width: main.width, height: main.height },
    shellOverflowY: getComputedStyle(shell).overflowY,
    windowInnerWidthToken: rootStyle.getPropertyValue('--taf-window-inner-width').trim(),
  };
});

const riskDistribution = workbench.pending_risk_counts ?? { high: 0, medium: 0, low: 0 };
const verifiedExtensionNoise = verifiedExtensionRequests.length > 0;
const actionableRequestFailures = requestFailures.filter((item) => !(verifiedExtensionNoise && item.url === 'https://api.yhchj.com/ip'));
const ignoredExtensionConsoleErrors = consoleErrors.filter((item) => (
  (verifiedExtensionNoise && item.url === 'https://api.yhchj.com/ip')
  || item.url.startsWith('chrome-extension://')
  || (item.text.startsWith('grm ERROR [iterable]') && !item.url.startsWith(baseUrl))
));
const actionableConsoleErrors = consoleErrors.filter((item) => !ignoredExtensionConsoleErrors.includes(item));
const actionablePageErrors = pageErrors.filter((item) => !(verifiedExtensionNoise && item === 'Object'));

const assertions = {
  windows_chrome_9224: String(version.Browser || '').startsWith('Chrome/'),
  non_fullscreen_viewport: layout.viewport.width === 1920 && layout.viewport.height === 1080,
  source_cards_six: await sourceCards.count() === 6,
  source_status_charts_are_echarts: sourceCanvasCount === 6 && sourceEchartsDeclaredCount === 6,
  source_titles_do_not_overlap_status: sourceHeaderNoOverlap,
  source_storage_is_truthful: workbench.sources?.find((item) => item.source_id === 'threat_intel')?.config?.storage === 'postgres.threat_intel'
    && workbench.sources?.find((item) => item.source_id === 'vulnerability')?.config?.storage === 'postgres.assets.metadata.vulnerabilities',
  pipeline_connections_are_echarts: pipelineCanvasCount === 1,
  pipeline_connections_adapt_to_measured_node_edges: pipelineGeometryWide.valid && pipelineGeometryMedium.valid && pipelineGeometryCompact.valid
    && pipelineGeometryWide.width > pipelineGeometryMedium.width
    && pipelineGeometryWide.sourceLinks === 6 && pipelineGeometryWide.ruleLinks === 5 && pipelineGeometryWide.outputLinks === 6,
  pipeline_nodes_never_overlap_or_escape: [pipelineGeometryWide, pipelineGeometryMedium, pipelineGeometryCompact].every((geometry) => geometry.contained && geometry.overlapPairs === 0),
  responsive_business_layout_no_horizontal_overflow: [responsiveLayoutMedium, responsiveLayoutCompact].every((state) => state.noDocumentHorizontalOverflow && state.noStageHorizontalOverflow && state.labelsInViewport && state.gridInViewport && state.mainInViewport && state.detailInViewport),
  pipeline_rule_trends_are_database_backed_echarts: await pipelineRuleMiniCharts.count() === 6 && pipelineRuleMiniChartCanvasCount === 6
    && pipelineRuleMiniChartValues.every((values, index) => JSON.stringify(values) === JSON.stringify(workbench.pipeline_rules?.[index]?.detail?.recent_hits ?? [])),
  canonical_pipeline_rule_order: JSON.stringify(pipelineRuleNames) === JSON.stringify(['IP-MAC 对齐', '账号-主机关联', '资产-部门补全', '域名-IP 解析', '告警-资产关联', '漏洞-服务命中']),
  acceptance_rule_count: workbench.rule_total === 26,
  rule_server_pagination_reachable: workbench.rules?.length === 6 && workbench.rule_limit === 6 && workbench.rule_offset === 0
    && rulePageTwoPayload?.rule_total === 26 && rulePageTwoPayload?.rule_offset === 6
    && initialRuleRowCount === 6 && pageTwoRuleRowCount === 6
    && initialRuleIds.every((id) => !pageTwoRuleIds.includes(id)),
  conflict_queue_pagination_reachable: initialConflictRowCount === 3 && pageTwoConflictRowCount === 3 && lastPageConflictRowCount === 3
    && initialConflictIds.every((id) => !pageTwoConflictIds.includes(id) && !lastPageConflictIds.includes(id))
    && pageTwoConflictIds.every((id) => !lastPageConflictIds.includes(id)),
  pending_conflict_count: workbench.pending_count === 18,
  acceptance_conflicts_are_explicit: workbench.conflict_total === 18 && workbench.conflicts?.length === 3 && workbench.conflicts.every((item) => item.origin === 'acceptance_fixture' && item.detail?.fixture === 'fusion-workbench-v1'),
  acceptance_fixture_is_disclosed_in_ui: acceptanceFixtureBadgeVisible,
  truncated_rule_ids_have_tooltips: ruleIdTooltipVisible,
  pending_risk_distribution: riskDistribution.high === 6 && riskDistribution.medium === 9 && riskDistribution.low === 3 && riskLabels.join('|').includes('高 6') && riskLabels.join('|').includes('中 9') && riskLabels.join('|').includes('低 3'),
  audit_count_and_density: auditRowCount === Math.min(workbench.audit_events?.length ?? 0, 5),
  audit_matches_reference_columns: JSON.stringify(auditHeaders) === JSON.stringify(['时间', '事件类型', '规则 ID', '实体类型 → 变更后', '操作者', '结果']),
  audit_pagination_reachable_after_writes: postActionAuditRowCount === 5 && auditPaginationVisible && pageTwoAuditRowCount >= 1,
  detail_actions_complete: ['确认主值', '创建修复任务', '查看审计记录', '输出到验收证据'].every((label) => detailActionLabels.some((value) => value.includes(label))),
  rule_copy_refresh_and_audit_actions_work: ruleCopyWorking && ruleRefreshWorking && ruleAuditNavigationWorking,
  authoritative_confirm_action_working: authoritativeConfirmWorking,
  reset_page_state_complete: resetPageStateWorking,
  reader_ui_write_controls_disabled: readerRuleEditDisabled && readerTableEditDisabled && readerConfirmDisabled && readerRepairDisabled && readerExportDisabled && readerAuditEnabled,
  reader_server_write_denied: readerRuleResponse.status() === 403 && readerConflictResponse.status() === 403 && readerEvidenceResponse.status() === 403,
  existing_chrome_session_restored: sessionRestored,
  rule_write_persisted: ruleUpdateResponse.status() === 200 && ruleUpdatePayload?.audit_written === true && Number(persistedRule?.version) === previousRuleVersion + 1
    && ruleDatabaseRecord === `${persistedRule?.rule_id}|${persistedRule?.version}|${note}`,
  stale_rule_write_rejected: staleRuleResponse?.status() === 409,
  repair_task_persisted: conflictResolveResponse.status() === 200 && conflictResolvePayload?.audit_written === true
    && conflictResolvePayload?.resolution?.strategy === 'manual-repair-task'
    && conflictResolvePayload?.repair_task?.task_type === 'fusion_conflict_repair'
    && conflictResolvePayload?.repair_task?.conflict_id === processedConflictId
    && repairTaskDatabaseRecord === `${repairTaskId}|fusion_conflict_repair|queued|${processedConflictId}`,
  processed_conflict_retained_after_write: Boolean(processedConflictId) && conflictResolvePayload?.resolution?.conflict_id === processedConflictId && processedConflictRetained,
  duplicate_repair_task_blocked_in_ui: repairButtonDisabledAfterWrite,
  repair_pending_conflict_locked_in_ui: confirmButtonDisabledAfterWrite,
  evidence_button_is_visible_hit_target: evidenceButtonHitTarget,
  evidence_downloaded: evidenceResponse.status() === 200 && evidencePayload?.sha256?.startsWith('sha256:') && downloadFilename === evidencePayload?.filename
    && evidenceDocument?.schema_version === 2
    && evidenceDocument?.conflict?.conflict_id === processedConflictId
    && evidenceDocument?.resolution?.conflict_id === processedConflictId
    && evidenceDocument?.resolution?.strategy === 'manual-repair-task'
    && evidenceDocument?.repair_tasks?.[0]?.task_id === repairTaskId
    && evidenceDocument?.rule_snapshot?.rule_id === conflictResolvePayload?.resolution?.rule_id
    && evidenceDocument?.audit_events?.some((item) => item.action === 'FUSION_CONFLICT_RESOLVED' && item.resource_id === processedConflictId),
  audit_navigation_working: auditNavigation.pathname === '/audit-log' && auditNavigation.search.includes('object_type=fusion_conflict')
    && new URLSearchParams(auditNavigation.search).get('object_id') === processedConflictId
    && auditListResponse.status() === 200
    && auditObjectFilterVisible
    && auditListRecords.length > 0
    && auditListRecords.every((item) => item.resource_type === 'fusion_conflict' && item.resource_id === processedConflictId),
  right_detail_contained: Boolean(layout.detail && layout.grid && layout.detail.left >= layout.grid.left && layout.detail.right <= layout.grid.right + 1 && layout.detail.top >= layout.grid.top),
  no_horizontal_overflow: layout.document.scrollWidth <= layout.document.clientWidth + 1,
  no_business_http_failures: badResponses.length === 0,
  no_request_failures: actionableRequestFailures.length === 0,
  no_console_errors: actionableConsoleErrors.length === 0,
  no_page_errors: actionablePageErrors.length === 0,
};

const deploymentImage = execFileSync('kubectl', [
  '-n', 'traffic-analysis', 'get', 'deployment', 'web-ui', '-o', 'jsonpath={.spec.template.spec.containers[0].image}',
], { encoding: 'utf8', env: commandEnv, timeout: 15_000 }).trim();
const result = Object.values(assertions).every(Boolean) ? 'pass' : 'fail';
const report = {
  result,
  browser_path: 'Xshell tunnel -> 127.0.0.1:9224 -> Windows Chrome -> direct APISIX',
  browser: version.Browser,
  cdp_targets: targets.length,
  route: redact(routeUrl.toString()),
  viewport: { width: 1920, height: 1080 },
  deployment_image: deploymentImage,
  counts: {
    sources: workbench.sources?.length ?? 0,
    rules: workbench.rules?.length ?? 0,
    pending_conflicts: workbench.pending_count ?? 0,
    risk_distribution: riskDistribution,
    audits: workbench.audit_events?.length ?? 0,
    initial_conflict_rows: initialConflictRowCount,
    page_two_conflict_rows: pageTwoConflictRowCount,
    last_page_conflict_rows: lastPageConflictRowCount,
    initial_conflict_ids: initialConflictIds,
    page_two_conflict_ids: pageTwoConflictIds,
    last_page_conflict_ids: lastPageConflictIds,
    post_action_audit_rows: postActionAuditRowCount,
    page_two_audit_rows: pageTwoAuditRowCount,
    source_echarts: sourceCanvasCount,
    source_echarts_declared: sourceEchartsDeclaredCount,
    pipeline_echarts: pipelineCanvasCount,
    pipeline_bar_echarts: pipelineRuleMiniChartCanvasCount,
    pipeline_geometry_wide: pipelineGeometryWide,
    pipeline_geometry_medium: pipelineGeometryMedium,
    pipeline_geometry_compact: pipelineGeometryCompact,
    responsive_layout_medium: responsiveLayoutMedium,
    responsive_layout_compact: responsiveLayoutCompact,
  },
  assertions,
  layout,
  writes: {
    rule_id: ruleUpdatePayload?.rule?.rule_id,
    rule_version: ruleUpdatePayload?.rule?.version,
    conflict_id: conflictResolvePayload?.resolution?.conflict_id,
    conflict_state_version: conflictResolvePayload?.resolution?.state_version,
    repair_task_id: repairTaskId,
    repair_task_database_record: repairTaskDatabaseRecord,
    evidence_filename: evidencePayload?.filename,
    evidence_sha256: evidencePayload?.sha256,
  },
  audit_navigation: {
    ...auditNavigation,
    response_status: auditListResponse.status(),
    object_filter_visible: auditObjectFilterVisible,
    record_count: auditListRecords.length,
    records: auditListRecords.map((item) => ({ resource_type: item.resource_type, resource_id: item.resource_id })),
  },
  business_responses: businessResponses,
  bad_responses: badResponses,
  request_failures: actionableRequestFailures,
  console_errors: actionableConsoleErrors,
  page_errors: actionablePageErrors,
  ignored_browser_extension_noise: {
    verified: verifiedExtensionNoise,
    requests: verifiedExtensionRequests,
    request_failures: requestFailures.filter((item) => item.url === 'https://api.yhchj.com/ip'),
    console_errors: ignoredExtensionConsoleErrors,
    page_errors: verifiedExtensionNoise ? pageErrors.filter((item) => item === 'Object') : [],
  },
  screenshots: [
    path.relative(root, screenshotPath),
    path.relative(root, responsiveScreenshotPath),
    path.relative(root, compactScreenshotPath),
    path.relative(root, ruleModalScreenshotPath),
    path.relative(root, conflictDetailScreenshotPath),
    path.relative(root, repairRetainedScreenshotPath),
    path.relative(root, postActionScreenshotPath),
  ],
  timestamp: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify(report, null, 2));
await pageCDP.detach().catch(() => {});
await page.close().catch(() => {});
process.exit(result === 'pass' ? 0 : 1);

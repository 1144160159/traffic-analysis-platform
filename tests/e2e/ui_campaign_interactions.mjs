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
const revision = process.env.CAMPAIGN_EVIDENCE_REVISION?.trim() || 'r293';
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/campaigns');
const outputPath = path.join(evidenceDir, `interaction-${revision}.json`);
const implementationPath = path.join(evidenceDir, `implementation-${revision}.png`);
const interactionPath = path.join(evidenceDir, `interaction-${revision}.png`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken(permissions = ['*', 'admin:*', 'alert:*', 'graph:read', 'playbook:execute']) {
  const encoded = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1_000);
  const userId = crypto.randomUUID();
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: userId, jti: crypto.randomUUID(), user_id: userId, tenant_id: 'default',
    username: 'codex-windows-cdp-admin', roles: permissions.includes('*') ? ['admin'] : ['viewer'], permissions, token_type: 'access', session_id: crypto.randomUUID(), iat: now, exp: now + 1_800,
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
for (const stalePage of context.pages()) {
  if (stalePage.url().includes('/campaigns?windowsCdpInteractionTs=')) await stalePage.close().catch(() => {});
}
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const badResponses = [];
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
const campaignActionResponses = [];
const campaignPageResponses = [];
page.on('response', (response) => {
  if (response.status() >= 400) badResponses.push({ status: response.status(), url: response.url() });
  if (response.request().method() === 'POST' && /\/v1\/campaigns\/(?:actions|[^/]+\/actions)/.test(response.url())) {
    void response.json()
      .then((payload) => {
        const data = payload?.data ?? payload;
        campaignActionResponses.push({ status: response.status(), url: response.url(), job_id: data?.job_id ?? null });
      })
      .catch(() => campaignActionResponses.push({ status: response.status(), url: response.url(), job_id: null }));
  }
  if (response.request().method() === 'GET' && response.url().includes('/v1/campaigns?')) campaignPageResponses.push({ status: response.status(), url: response.url() });
});
page.on('console', (entry) => { if (entry.type() === 'error') consoleErrors.push(entry.text()); });
page.on('pageerror', (error) => pageErrors.push(error.message));
page.on('requestfailed', (request) => requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? 'unknown' }));

const token = smokeToken();
await page.goto(`${baseUrl}/login`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.evaluate((accessToken) => {
  localStorage.removeItem('traffic-ui-refresh-token');
  localStorage.setItem('traffic-ui-token', accessToken);
}, token);
badResponses.length = 0;
consoleErrors.length = 0;
pageErrors.length = 0;
requestFailures.length = 0;
const productionUrl = new URL(`/campaigns?windowsCdpInteractionTs=${Date.now()}`, baseUrl);
const visualUrl = new URL(productionUrl);
visualUrl.searchParams.set('__codex_ui_breakdown_production', '1');

async function openCampaign(visual = false) {
  await page.goto((visual ? visualUrl : productionUrl).toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  await page.locator('.taf-campaign-workbench').waitFor({ state: 'visible', timeout: 15_000 });
  const authStatus = await page.evaluate(async () => {
    const accessToken = localStorage.getItem('traffic-ui-token') ?? '';
    return fetch('/api/v1/campaigns?limit=1', { headers: { Authorization: `Bearer ${accessToken}` } }).then((response) => response.status);
  });
  if (authStatus !== 200) throw new Error(`Campaign browser authentication preflight failed with HTTP ${authStatus}`);
}

async function verifyDrawer(button, endpointPattern, auditEvent, expectCampaignId = true, confirm = false, mutation = false, clickOptions = undefined) {
  await button.click(clickOptions);
  if (confirm) {
    await page.locator('.ant-popconfirm:visible').getByRole('button', { name: '确 定' }).click();
  }
  const drawer = page.locator('.taf-campaign-action-drawer:visible');
  await drawer.waitFor({ state: 'visible', timeout: 6_000 });
  const text = await drawer.textContent();
  const result = {
    endpoint: endpointPattern.test(text ?? ''),
    audit: Boolean(text?.includes(auditEvent)),
    receipt: mutation
      ? Boolean(text?.includes('业务操作已持久化') && text?.includes('PostgreSQL') && text?.includes('audit_logs') && text?.includes('completed'))
      : Boolean(text?.includes('访问操作已审计') && text?.includes('campaign_action_jobs') && text?.includes('audit_logs') && text?.includes('completed')),
    requestBody: Boolean(text?.includes('simulation') && text?.includes('dry_run'))
      && (expectCampaignId ? Boolean(text?.includes('campaign_id')) : !text?.includes('campaign_id')),
  };
  await drawer.locator('.ant-drawer-close').click();
  return result;
}

async function verifyNavigation(button, pathname, campaignId = '', confirm = false) {
  await button.click();
  if (confirm) {
    await page.locator('.ant-popconfirm:visible').getByRole('button', { name: '确 定' }).click();
  }
  await page.waitForURL((url) => url.pathname === pathname, { timeout: 10_000 });
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  const destination = new URL(page.url());
  const passed = destination.pathname === pathname && (!campaignId || destination.searchParams.get('campaign') === campaignId);
  await openCampaign(false);
  return passed;
}

async function verifyDetailDrawerNavigation(button, campaignId) {
  await button.click();
  const drawer = page.locator('.taf-campaign-detail-drawer:visible');
  await drawer.waitFor({ state: 'visible', timeout: 10_000 });
  const drawerShowsCampaign = (await drawer.textContent())?.includes(campaignId) ?? false;
  await drawer.getByRole('button', { name: '打开完整详情' }).click();
  await page.waitForURL((url) => url.pathname === `/campaigns/${encodeURIComponent(campaignId)}`, { timeout: 10_000 });
  const passed = drawerShowsCampaign && new URL(page.url()).pathname === `/campaigns/${encodeURIComponent(campaignId)}`;
  await openCampaign(false);
  return passed;
}

await openCampaign(true);
fs.mkdirSync(evidenceDir, { recursive: true });
await page.screenshot({ path: implementationPath, fullPage: false });

const chartCanvasCount = await page.locator('.taf-campaign-workbench canvas').count();
const attackGraphCount = await page.locator('[data-chart-engine="echarts"][data-series-type="graph"]').count();
const chartBounds = await page.locator('.taf-campaign-workbench canvas').evaluateAll((canvases) => canvases.map((canvas) => {
  const rect = canvas.getBoundingClientRect();
  const context = canvas.getContext('2d');
  const pixels = context?.getImageData(0, 0, canvas.width, canvas.height).data ?? [];
  let paintedSamples = 0;
  for (let index = 3; index < pixels.length; index += 64) {
    if (pixels[index] > 0) paintedSamples += 1;
  }
  const nonBlank = paintedSamples >= 20;
  return { width: rect.width, height: rect.height, top: rect.top, bottom: rect.bottom, paintedSamples, nonBlank, visible: rect.width >= 80 && rect.height >= 80 && rect.top >= 80 && rect.bottom <= 997 && nonBlank };
}));

await page.evaluate(() => sessionStorage.removeItem('taf:campaign-action-audit'));

const actionChecks = [];
actionChecks.push(await verifyDrawer(page.locator('.taf-campaign-list-panel > .taf-panel__header button').nth(1), /\/v1\/campaigns\/actions/, 'CAMPAIGN_LIST_SETTINGS_UPDATED', false));
const downloadPromise = page.waitForEvent('download', { timeout: 8_000 });
await page.locator('.taf-campaign-list-panel > .taf-panel__header button').first().click();
const download = await downloadPromise;
const exportWorked = Boolean(download.suggestedFilename().match(/^campaigns-.*\.json$/));
await page.locator('.taf-campaign-action-drawer:visible .ant-drawer-close').click();

await openCampaign(false);

const tableRows = page.locator('.taf-campaign-list-panel .ant-table-tbody tr');
const paginationTotalText = (await page.locator('.taf-campaign-list-panel .ant-pagination-total-text').innerText()).trim();
const initialRowCount = await tableRows.count();
const paginationTotal = Number(paginationTotalText.match(/\d+/)?.[0] ?? 0);
const paginationPageSizeWorked = initialRowCount > 0 && initialRowCount <= 8 && paginationTotal >= initialRowCount;
const riskChartDataBeforePageSizeChange = await page.locator('.taf-campaign-risk-distribution').getAttribute('data-chart-values');
const pageSizeSelect = page.locator('.taf-campaign-list-panel .ant-pagination-options-size-changer');
await pageSizeSelect.click();
const pageSizeFiveResponsePromise = page.waitForResponse((response) => {
  if (response.request().method() !== 'GET' || !response.url().includes('/v1/campaigns?')) return false;
  const url = new URL(response.url());
  return url.searchParams.get('limit') === '5' && url.searchParams.get('offset') === '0';
});
await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: /^5\b/ }).first().click();
const pageSizeFiveResponse = await pageSizeFiveResponsePromise;
await page.waitForFunction(() => document.querySelectorAll('.taf-campaign-list-panel .ant-table-tbody tr').length === 5, undefined, { timeout: 10_000 });
const pageSizeFiveRowCount = await tableRows.count();
const riskChartDataAfterPageSizeChange = await page.locator('.taf-campaign-risk-distribution').getAttribute('data-chart-values');
const riskChartGlobalSummaryStable = Boolean(riskChartDataBeforePageSizeChange)
  && Boolean(riskChartDataAfterPageSizeChange)
  && riskChartDataBeforePageSizeChange === riskChartDataAfterPageSizeChange;
const pageSizeEightResponsePromise = page.waitForResponse((response) => {
  if (response.request().method() !== 'GET' || !response.url().includes('/v1/campaigns?')) return false;
  const url = new URL(response.url());
  return url.searchParams.get('limit') === '8' && url.searchParams.get('offset') === '0';
});
await openCampaign(false);
const pageSizeEightResponse = await pageSizeEightResponsePromise;
await page.waitForFunction(() => document.querySelectorAll('.taf-campaign-list-panel .ant-table-tbody tr').length === 8, undefined, { timeout: 10_000 });
const pageSizeChangerWorked = pageSizeFiveResponse.ok() && pageSizeEightResponse.ok() && pageSizeFiveRowCount === 5;
const firstPageFirstCampaign = await tableRows.first().textContent();
const secondPageResponsePromise = page.waitForResponse((response) => response.request().method() === 'GET' && response.url().includes('/v1/campaigns?') && response.url().includes('offset=8'));
await page.locator('.taf-campaign-list-panel .ant-pagination-item-2').click();
const secondPageResponse = await secondPageResponsePromise;
await page.waitForFunction((firstText) => document.querySelector('.taf-campaign-list-panel .ant-table-tbody tr')?.textContent !== firstText, firstPageFirstCampaign);
const secondPageFirstCampaign = await tableRows.first().textContent();
const paginationWorked = Boolean(firstPageFirstCampaign && secondPageFirstCampaign && firstPageFirstCampaign !== secondPageFirstCampaign)
  && secondPageResponse.ok()
  && new URL(secondPageResponse.url()).searchParams.get('offset') === '8'
  && await page.locator('.taf-campaign-list-panel .ant-pagination-item-2').getAttribute('class').then((value) => Boolean(value?.includes('active')));

const paginationItems = page.locator('.taf-campaign-list-panel .ant-pagination-item');
const lastPageNumber = Number((await paginationItems.last().innerText()).trim());
const lastPageOffset = (lastPageNumber - 1) * 8;
const lastPageResponsePromise = page.waitForResponse((response) => {
  if (response.request().method() !== 'GET' || !response.url().includes('/v1/campaigns?')) return false;
  return new URL(response.url()).searchParams.get('offset') === String(lastPageOffset);
});
await paginationItems.last().click();
const lastPageResponse = await lastPageResponsePromise;
const lastPagePayload = await lastPageResponse.json();
const lastPageResponseTotal = Number(lastPagePayload?.data?.total ?? lastPagePayload?.total ?? 0);
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
const lastPageRowCount = await tableRows.count();
const lastPageWorked = lastPageNumber >= 2 && lastPageResponse.ok() && lastPageRowCount > 0 && lastPageRowCount <= 8
  && lastPageResponseTotal >= paginationTotal && new URL(lastPageResponse.url()).searchParams.get('offset') === String(lastPageOffset);
const lastPageFirstCampaign = await tableRows.first().textContent();
await paginationItems.first().click();
await page.waitForFunction((lastText) => {
  const firstPage = document.querySelector('.taf-campaign-list-panel .ant-pagination-item-1');
  const firstRow = document.querySelector('.taf-campaign-list-panel .ant-table-tbody tr');
  return firstPage?.className.includes('active') && firstRow?.textContent !== lastText;
}, lastPageFirstCampaign, { timeout: 10_000 });

const initialRiskLabels = await tableRows.locator('td:nth-child(3)').allInnerTexts();
const riskCandidates = [
  { label: '高风险', value: 'high' },
  { label: '中风险', value: 'medium' },
  { label: '低风险', value: 'low' },
];
let availableRisk;
for (const candidate of riskCandidates) {
  if (initialRiskLabels.every((label) => label.trim() === candidate.label)) continue;
  const response = await fetch(`${baseUrl}/api/v1/campaigns?limit=1&risk=${candidate.value}`, { headers: { Authorization: `Bearer ${token}` } });
  const payload = await response.json();
  const candidates = payload?.data?.campaigns ?? payload?.campaigns ?? [];
  if (response.ok && candidates.length > 0) {
    availableRisk = candidate.label;
    break;
  }
}
availableRisk ??= initialRiskLabels[0]?.trim();
if (!['高风险', '中风险', '低风险'].includes(availableRisk)) throw new Error(`Unexpected Campaign risk label: ${availableRisk}`);
const riskSelect = page.locator('.taf-campaign-filter .ant-select').first();
await riskSelect.click();
await page.locator('.ant-select-dropdown:visible .ant-select-item-option', { hasText: availableRisk }).click();
const expectedRiskParam = { '高风险': 'high', '中风险': 'medium', '低风险': 'low' }[availableRisk];
const filterResponsePromise = page.waitForResponse((response) => {
  if (response.request().method() !== 'GET' || !response.url().includes('/v1/campaigns?')) return false;
  const url = new URL(response.url());
  return url.searchParams.get('risk') === expectedRiskParam && url.searchParams.get('limit') === '8';
});
await page.locator('.taf-campaign-filter > button').nth(1).click();
const filterResponse = await filterResponsePromise;
const filterDomSettled = await page.waitForFunction((risk) => {
  const rows = Array.from(document.querySelectorAll('.taf-campaign-list-panel .ant-table-tbody tr'));
  return rows.length > 0 && rows.every((row) => row.textContent?.includes(String(risk)));
}, availableRisk, { timeout: 10_000 }).then(() => true).catch(() => false);
const filteredRowTexts = await tableRows.allTextContents();
const filterUrl = new URL(filterResponse.url());
const filterWorked = filterResponse.ok()
  && filterUrl.searchParams.get('risk') === expectedRiskParam
  && filterDomSettled
  && filteredRowTexts.length > 0
  && filteredRowTexts.every((text) => text.includes(availableRisk));
await page.locator('.taf-campaign-filter > button').first().click();

async function verifySelectFilter(selectIndex, label, parameter, expectedValue) {
  await page.locator('.taf-campaign-filter .ant-select').nth(selectIndex).click();
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option', { hasText: label }).click();
  const responsePromise = page.waitForResponse((response) => {
    if (response.request().method() !== 'GET' || !response.url().includes('/v1/campaigns?')) return false;
    return new URL(response.url()).searchParams.get(parameter) === expectedValue;
  });
  await page.locator('.taf-campaign-filter > button').nth(1).click();
  const response = await responsePromise;
  const settled = await page.waitForFunction((expectedLabel) => {
    const rows = Array.from(document.querySelectorAll('.taf-campaign-list-panel .ant-table-tbody tr'));
    return rows.length > 0 && rows.every((row) => row.textContent?.includes(String(expectedLabel)));
  }, label, { timeout: 10_000 }).then(() => true).catch(() => false);
  const url = new URL(response.url());
  const passed = response.ok() && url.searchParams.get(parameter) === expectedValue && url.searchParams.get('limit') === '8' && settled;
  await page.locator('.taf-campaign-filter > button').first().click();
  await page.waitForTimeout(300);
  return passed;
}

const resetRowTexts = await tableRows.allTextContents();
const availableStatus = ['活跃中', '调查中', '已结束'].find((label) => resetRowTexts.some((text) => text.includes(label)));
if (!availableStatus) throw new Error('No supported Campaign status available for server-filter verification');
const statusParam = { 活跃中: 'active', 调查中: 'investigating', 已结束: 'closed' }[availableStatus];
const statusFilterWorked = await verifySelectFilter(1, availableStatus, 'status', statusParam);

const availablePhase = '数据外传';
const phaseParam = 'exfiltration';
await page.locator('.taf-campaign-filter .ant-select').nth(2).click();
await page.locator('.ant-select-dropdown:visible .ant-select-item-option', { hasText: availablePhase }).click();
const phaseResponsePromise = page.waitForResponse((response) => {
  if (response.request().method() !== 'GET' || !response.url().includes('/v1/campaigns?')) return false;
  return new URL(response.url()).searchParams.get('phase') === phaseParam;
});
await page.locator('.taf-campaign-filter > button').nth(1).click();
const phaseResponse = await phaseResponsePromise;
const phasePayload = await phaseResponse.json();
const phaseCampaigns = phasePayload?.data?.campaigns ?? phasePayload?.campaigns ?? [];
const phaseFilterWorked = phaseResponse.ok() && phaseCampaigns.length > 0
  && phaseCampaigns.every((campaign) => Array.isArray(campaign.attack_phases) && campaign.attack_phases.includes(phaseParam))
  && await tableRows.count() > 0;
await page.locator('.taf-campaign-filter > button').first().click();
await page.waitForTimeout(300);

const keywordCampaign = (await tableRows.first().locator('td').first().innerText()).trim();
const keyword = keywordCampaign.slice(0, Math.min(12, keywordCampaign.length));
await page.getByRole('textbox', { name: '战役名称 / 关键字' }).fill(keyword);
const keywordResponsePromise = page.waitForResponse((response) => {
  if (response.request().method() !== 'GET' || !response.url().includes('/v1/campaigns?')) return false;
  return new URL(response.url()).searchParams.get('keyword') === keyword;
});
await page.locator('.taf-campaign-filter > button').nth(1).click();
const keywordResponse = await keywordResponsePromise;
const keywordRows = await tableRows.allTextContents();
const keywordFilterWorked = keywordResponse.ok() && keywordRows.length > 0 && keywordRows.every((text) => text.toLowerCase().includes(keyword.toLowerCase()));
await page.locator('.taf-campaign-filter > button').first().click();

actionChecks.push(await verifyDrawer(
  page.locator('[data-chart-engine="echarts"][data-series-type="graph"] canvas'),
  /\/v1\/campaigns\/[^/]+\/actions/,
  'CAMPAIGN_PHASE_VIEWED',
  true,
  false,
  false,
  { position: { x: 60, y: 114 } },
));
actionChecks.push(await verifyDrawer(page.locator('.taf-campaign-impact button').first(), /\/v1\/campaigns\/[^/]+\/actions/, 'CAMPAIGN_IMPACT_VIEWED'));
actionChecks.push(await verifyDrawer(page.getByRole('button', { name: '查看证据中心', exact: true }), /\/v1\/campaigns\/[^/]+\/actions/, 'CAMPAIGN_EVIDENCE_VIEWED'));
actionChecks.push(await verifyDrawer(page.locator('.taf-campaign-actions button').nth(1), /\/v1\/campaigns\/[^/]+\/actions/, 'CAMPAIGN_STATUS_CHANGED', true, true, true));
actionChecks.push(await verifyDrawer(page.locator('.taf-campaign-actions button').nth(2), /\/v1\/campaigns\/[^/]+\/actions/, 'CAMPAIGN_REPORT_REQUESTED', true, false, true));
const firstRowActions = page.locator('.taf-campaign-list-panel .ant-table-tbody tr').first().locator('.taf-campaign-row-actions button');
actionChecks.push(await verifyDrawer(firstRowActions.nth(1), /\/v1\/campaigns\/[^/]+\/actions/, 'CAMPAIGN_STATUS_CHANGED', true, true, true));
actionChecks.push(await verifyDrawer(firstRowActions.nth(2), /\/v1\/campaigns\/[^/]+\/actions/, 'CAMPAIGN_CONTEXT_ACTION_REQUESTED'));

const selectedCampaignId = (await page.locator('.taf-campaign-summary > div strong').textContent())?.trim() ?? '';
const detailNavigation = await verifyDetailDrawerNavigation(
  page.locator('.taf-campaign-rail > .taf-panel').first().locator('.taf-panel__header button').first(),
  selectedCampaignId,
);
const attackChainCampaignId = (await page.locator('.taf-campaign-summary > div strong').textContent())?.trim() ?? '';
const attackChainNavigation = await verifyNavigation(page.locator('.taf-campaign-actions button').nth(3), '/attack-chains', attackChainCampaignId);
const graphCampaignId = (await page.locator('.taf-campaign-summary > div strong').textContent())?.trim() ?? '';
const graphNavigation = await verifyNavigation(page.locator('.taf-campaign-actions button').nth(4), '/graph', graphCampaignId);
const soarCampaignId = (await page.locator('.taf-campaign-summary > div strong').textContent())?.trim() ?? '';
const soarNavigation = await verifyNavigation(page.locator('.taf-campaign-actions button').nth(5), '/playbooks', soarCampaignId, true);
const assetNavigation = await verifyNavigation(page.locator('.taf-campaign-rail > .taf-panel').nth(1).locator('.taf-panel__header button').first(), '/assets');

await page.waitForTimeout(500);
const auditRecords = await page.evaluate(() => JSON.parse(sessionStorage.getItem('taf:campaign-action-audit') ?? '[]'));
const responseJobIds = [...new Set(campaignActionResponses.map((item) => item.job_id).filter(Boolean))];
const persistedJobs = await Promise.all(responseJobIds.map(async (jobId) => {
  const response = await fetch(`${baseUrl}/api/v1/campaigns/jobs/${encodeURIComponent(jobId)}`, { headers: { Authorization: `Bearer ${token}` } });
  const payload = await response.json();
  const job = payload.data ?? payload;
  return { http_status: response.status, job_id: job.job_id, status: job.status, tenant_id: job.tenant_id, action_id: job.action_id };
}));
const auditResponse = await fetch(`${baseUrl}/api/v1/audit/logs?object_type=campaign&limit=100`, { headers: { Authorization: `Bearer ${token}` } });
const auditPayload = await auditResponse.json();
const auditTrails = auditPayload.data?.trails ?? auditPayload.trails ?? [];
const serverAuditJobIds = [...new Set(auditTrails.map((trail) => trail.details?.job_id).filter((jobId) => responseJobIds.includes(jobId)))];
const serverAuditCount = serverAuditJobIds.length;
const viewerToken = smokeToken(['alert:read']);
const viewerResponse = await fetch(`${baseUrl}/api/v1/campaigns/${encodeURIComponent(selectedCampaignId)}/actions`, {
  method: 'POST',
  headers: { Authorization: `Bearer ${viewerToken}`, 'Content-Type': 'application/json' },
  body: JSON.stringify({ action_id: 'campaign-status-change', target: selectedCampaignId, metadata: { dry_run: true }, simulation: true, dry_run: true }),
});
const viewerWriteDenied = viewerResponse.status === 403;
await page.screenshot({ path: interactionPath, fullPage: false });
const screenshotValid = fs.statSync(implementationPath).size > 100_000 && fs.statSync(interactionPath).size > 100_000;
const externalRequestFailures = requestFailures.filter((item) => item.url.startsWith('https://api.yhchj.com/'));
const appRequestFailures = requestFailures.filter((item) => !item.url.startsWith('https://api.yhchj.com/'));
const externalConsoleErrors = externalRequestFailures.length
  ? consoleErrors.filter((item) => item === 'Failed to load resource: net::ERR_CONNECTION_CLOSED')
  : [];
const appConsoleErrors = consoleErrors.filter((item) => !externalConsoleErrors.includes(item));
const externalPageErrors = externalRequestFailures.length ? pageErrors.filter((item) => item === 'Object') : [];
const appPageErrors = pageErrors.filter((item) => !externalPageErrors.includes(item));

const result = {
  result: chartCanvasCount === 3
    && attackGraphCount === 1
    && chartBounds.every((item) => item.visible)
    && paginationWorked
    && paginationPageSizeWorked
    && pageSizeChangerWorked
    && lastPageWorked
    && filterWorked
    && riskChartGlobalSummaryStable
    && statusFilterWorked
    && phaseFilterWorked
    && keywordFilterWorked
    && actionChecks.every((item) => item.endpoint && item.audit && item.receipt && item.requestBody)
    && exportWorked
    && detailNavigation
    && attackChainNavigation
    && graphNavigation
    && soarNavigation
    && assetNavigation
    && auditRecords.length >= 12
    && campaignActionResponses.length >= 12
    && campaignActionResponses.every((item) => item.status === 200)
    && campaignPageResponses.some((item) => item.status === 200 && item.url.includes('offset=8'))
    && auditResponse.ok
    && responseJobIds.length >= 12
    && persistedJobs.length === responseJobIds.length
    && persistedJobs.every((job) => job.http_status === 200 && job.status === 'completed' && job.tenant_id === 'default' && responseJobIds.includes(job.job_id))
    && serverAuditCount === responseJobIds.length
    && viewerWriteDenied
    && screenshotValid
    && badResponses.length === 0
    && appConsoleErrors.length === 0
    && appPageErrors.length === 0
    && appRequestFailures.length === 0 ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP',
  browser: version.Browser,
  chart_canvas_count: chartCanvasCount,
  attack_graph_count: attackGraphCount,
  chart_bounds: chartBounds,
  pagination_worked: paginationWorked,
  pagination_total_text: paginationTotalText,
  pagination_total: paginationTotal,
  pagination_page_size_worked: paginationPageSizeWorked,
  page_size_five_row_count: pageSizeFiveRowCount,
  page_size_changer_worked: pageSizeChangerWorked,
  last_page_number: lastPageNumber,
  last_page_row_count: lastPageRowCount,
  last_page_response_total: lastPageResponseTotal,
  last_page_worked: lastPageWorked,
  first_page_first_campaign: firstPageFirstCampaign,
  second_page_first_campaign: secondPageFirstCampaign,
  filtered_risk: availableRisk,
  filter_worked: filterWorked,
  risk_chart_global_summary_stable: riskChartGlobalSummaryStable,
  risk_chart_data_before_page_size_change: riskChartDataBeforePageSizeChange,
  risk_chart_data_after_page_size_change: riskChartDataAfterPageSizeChange,
  status_filter: availableStatus,
  status_filter_worked: statusFilterWorked,
  phase_filter: availablePhase,
  phase_filter_worked: phaseFilterWorked,
  keyword_filter: keyword,
  keyword_filter_worked: keywordFilterWorked,
  action_checks: actionChecks,
  campaign_action_responses: campaignActionResponses,
  campaign_page_responses: campaignPageResponses,
  export_worked: exportWorked,
  navigation: { detail: detailNavigation, attack_chain: attackChainNavigation, graph: graphNavigation, soar: soarNavigation, assets: assetNavigation },
  audit_record_count: auditRecords.length,
  response_job_ids: responseJobIds,
  persisted_jobs: persistedJobs,
  server_audit_job_ids: serverAuditJobIds,
  server_audit_count: serverAuditCount,
  viewer_write_denied: viewerWriteDenied,
  screenshot_valid: screenshotValid,
  bad_responses: badResponses,
  console_errors: appConsoleErrors,
  page_errors: appPageErrors,
  request_failures: appRequestFailures,
  external_browser_noise: {
    console_errors: externalConsoleErrors,
    page_errors: externalPageErrors,
    request_failures: externalRequestFailures,
  },
  implementation_screenshot: path.relative(root, implementationPath),
  interaction_screenshot: path.relative(root, interactionPath),
  timestamp: new Date().toISOString(),
};

fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);

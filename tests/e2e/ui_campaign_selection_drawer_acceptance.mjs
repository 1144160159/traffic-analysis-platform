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
const revision = process.env.CAMPAIGN_EVIDENCE_REVISION?.trim() || 'r767-prod';
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/campaigns');
const outputPath = path.join(evidenceDir, `selection-drawer-${revision}.json`);
const screenshotPath = path.join(evidenceDir, `selection-drawer-${revision}.png`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const encoded = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
    { encoding: 'utf8', env: process.env, timeout: 15_000 },
  );
  const now = Math.floor(Date.now() / 1_000);
  const userId = crypto.randomUUID();
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: userId,
    jti: crypto.randomUUID(),
    user_id: userId,
    tenant_id: 'default',
    username: 'codex-windows-cdp-admin',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:*', 'graph:read', 'playbook:execute'],
    token_type: 'access',
    session_id: crypto.randomUUID(),
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const box = async (locator) => locator.evaluate((element) => {
  const rect = element.getBoundingClientRect();
  return {
    left: Math.round(rect.left),
    top: Math.round(rect.top),
    right: Math.round(rect.right),
    bottom: Math.round(rect.bottom),
    width: Math.round(rect.width),
    height: Math.round(rect.height),
  };
});

const overlaps = (a, b) => a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const cdp = await page.context().newCDPSession(page);
let exitCode = 1;

try {
  await page.setViewportSize({ width: 1920, height: 1080 });
  const badResponses = [];
  const consoleErrors = [];
  const pageErrors = [];
  const requestFailures = [];
  page.on('response', (response) => {
    if (response.status() >= 400) badResponses.push({ status: response.status(), url: response.url() });
  });
  page.on('console', (entry) => {
    if (entry.type() === 'error') consoleErrors.push(entry.text());
  });
  page.on('pageerror', (error) => pageErrors.push(error.message));
  page.on('requestfailed', (request) => requestFailures.push({
    url: request.url(),
    error: request.failure()?.errorText ?? 'unknown',
  }));

  await page.goto(`${baseUrl}/login`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.evaluate((accessToken) => {
    localStorage.removeItem('traffic-ui-refresh-token');
    localStorage.setItem('traffic-ui-token', accessToken);
  }, smokeToken());
  badResponses.length = 0;
  consoleErrors.length = 0;
  pageErrors.length = 0;
  requestFailures.length = 0;

  await page.goto(`${baseUrl}/campaigns?windowsCdpSelectionTs=${Date.now()}`, {
    waitUntil: 'domcontentloaded',
    timeout: 45_000,
  });
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  await page.locator('.taf-campaign-workbench').waitFor({ state: 'visible', timeout: 15_000 });

  const rows = page.locator('.taf-campaign-list-panel .ant-table-tbody tr.ant-table-row');
  if (await rows.count() < 2) throw new Error('Campaign selection acceptance requires at least two rows');
  const firstCampaignId = (await rows.nth(0).locator('td').first().innerText()).trim();
  const secondCampaignId = (await rows.nth(1).locator('td').first().innerText()).trim();
  const initialSummaryId = (await page.locator('.taf-campaign-summary > div strong').innerText()).trim();
  const secondDetailResponse = page.waitForResponse((response) => (
    response.request().method() === 'GET'
    && response.url().includes(`/v1/campaigns/${encodeURIComponent(secondCampaignId)}`)
  ), { timeout: 12_000 }).catch(() => null);

  await rows.nth(1).click();
  await page.waitForFunction((campaignId) => new URL(window.location.href).searchParams.get('campaign') === campaignId, secondCampaignId);
  await page.locator('.taf-campaign-summary > div strong').filter({ hasText: secondCampaignId }).waitFor({ state: 'visible' });
  const detailResponse = await secondDetailResponse;
  await page.waitForTimeout(250);

  const selectedRowsAfterSecondClick = await rows.evaluateAll((items) => items.map((item, index) => ({
    index,
    selectedClass: item.classList.contains('is-selected'),
    ariaSelected: item.getAttribute('aria-selected'),
  })));
  const secondSummaryId = (await page.locator('.taf-campaign-summary > div strong').innerText()).trim();
  const railPanelTexts = await page.locator('.taf-campaign-rail > .taf-panel').evaluateAll((panels) => (
    panels.map((panel) => panel.textContent?.replace(/\s+/g, ' ').trim() ?? '')
  ));

  const actionResponse = page.waitForResponse((response) => (
    response.request().method() === 'POST'
    && response.url().includes(`/v1/campaigns/${encodeURIComponent(secondCampaignId)}/actions`)
  ), { timeout: 12_000 });
  await page.locator('.taf-campaign-summary-panel .taf-panel__header').getByRole('button', { name: '查看详情' }).click();
  const openedActionResponse = await actionResponse;
  const drawer = page.locator('.taf-campaign-detail-drawer:visible');
  await drawer.waitFor({ state: 'visible', timeout: 12_000 });
  await drawer.getByText(secondCampaignId, { exact: false }).first().waitFor({ state: 'visible' });

  const drawerWrapper = drawer.locator('.ant-drawer-content-wrapper');
  const drawerBounds = await box(drawerWrapper);
  const listBounds = await box(page.locator('.taf-campaign-list-panel'));
  const summaryBounds = await box(drawer.locator('.taf-campaign-detail-drawer__summary'));
  const workspaceBounds = await box(drawer.locator('.taf-campaign-detail-drawer__workspace'));
  const footerBounds = await box(drawer.locator('.taf-campaign-detail-drawer__footer'));
  const detailColumns = drawer.locator('.taf-campaign-detail-drawer__workspace[data-active-tab="detail"] > aside, .taf-campaign-detail-drawer__workspace[data-active-tab="detail"] > main');
  const detailColumnBounds = await detailColumns.evaluateAll((elements) => elements.map((element) => {
    const rect = element.getBoundingClientRect();
    return {
      left: Math.round(rect.left),
      top: Math.round(rect.top),
      right: Math.round(rect.right),
      bottom: Math.round(rect.bottom),
      width: Math.round(rect.width),
      height: Math.round(rect.height),
    };
  }));
  const graphCanvas = drawer.locator('[data-chart-engine="echarts"][data-series-type="lines+scatter"] canvas').first();
  await graphCanvas.waitFor({ state: 'visible', timeout: 12_000 });
  const graphBounds = await box(graphCanvas);
  const graphPanelBounds = await box(drawer.locator('.taf-campaign-detail-drawer__graph'));
  const summaryCardCount = await drawer.locator('.taf-campaign-detail-drawer__summary > span').count();
  const tabLabels = await drawer.locator('.taf-detail-drawer-tabs .ant-tabs-tab').allTextContents();

  await drawer.getByRole('tab', { name: '证据' }).click();
  await drawer.locator('[data-tab-panel="evidence"]:visible').waitFor({ state: 'visible' });
  const evidencePanelBounds = await box(drawer.locator('[data-tab-panel="evidence"]:visible'));
  await drawer.getByRole('tab', { name: '审计' }).click();
  await drawer.locator('[data-tab-panel="audit"]:visible').waitFor({ state: 'visible' });
  const auditPanelBounds = await box(drawer.locator('[data-tab-panel="audit"]:visible'));
  await drawer.getByRole('tab', { name: '详情' }).click();
  await drawer.locator('.taf-campaign-detail-drawer__graph:visible').waitFor({ state: 'visible' });

  fs.mkdirSync(evidenceDir, { recursive: true });
  await page.screenshot({ path: screenshotPath, fullPage: false });

  const appRequestFailures = requestFailures.filter((item) => !item.url.startsWith('https://api.yhchj.com/'));
  const appConsoleErrors = consoleErrors.filter((item) => item !== 'Failed to load resource: net::ERR_CONNECTION_CLOSED');
  const result = {
    result: initialSummaryId === firstCampaignId
      && firstCampaignId !== secondCampaignId
      && secondSummaryId === secondCampaignId
      && new URL(page.url()).searchParams.get('campaign') === secondCampaignId
      && selectedRowsAfterSecondClick[1]?.selectedClass
      && selectedRowsAfterSecondClick[1]?.ariaSelected === 'true'
      && selectedRowsAfterSecondClick.filter((row) => row.selectedClass).length === 1
      && detailResponse?.ok()
      && railPanelTexts.length === 4
      && railPanelTexts[0]?.includes(secondCampaignId)
      && openedActionResponse.ok()
      && drawerBounds.width <= 1200
      && drawerBounds.width >= 1080
      && drawerBounds.left > 0
      && drawerBounds.right < 1920
      && drawerBounds.top > 0
      && drawerBounds.bottom < 1080
      && drawerBounds.left > listBounds.left
      && summaryCardCount === 6
      && tabLabels.join('|') === '详情|证据|审计'
      && detailColumnBounds.length === 3
      && detailColumnBounds.every((column) => column.width >= 240)
      && !overlaps(detailColumnBounds[0], detailColumnBounds[1])
      && !overlaps(detailColumnBounds[1], detailColumnBounds[2])
      && graphBounds.left >= graphPanelBounds.left
      && graphBounds.right <= graphPanelBounds.right
      && graphBounds.top >= graphPanelBounds.top
      && graphBounds.bottom <= graphPanelBounds.bottom
      && summaryBounds.bottom <= workspaceBounds.top
      && workspaceBounds.bottom <= footerBounds.top
      && evidencePanelBounds.width >= workspaceBounds.width - 24
      && auditPanelBounds.width >= workspaceBounds.width - 24
      && badResponses.length === 0
      && appConsoleErrors.length === 0
      && pageErrors.length === 0
      && appRequestFailures.length === 0 ? 'pass' : 'fail',
    browser_backend: 'Windows Chrome CDP over Xshell 9224 -> 9222',
    browser: version.Browser,
    campaigns: {
      first: firstCampaignId,
      second: secondCampaignId,
      initial_summary: initialSummaryId,
      selected_summary: secondSummaryId,
      url_campaign: new URL(page.url()).searchParams.get('campaign'),
      selected_rows: selectedRowsAfterSecondClick,
      detail_response_status: detailResponse?.status() ?? null,
      rail_panel_texts: railPanelTexts,
    },
    drawer: {
      bounds: drawerBounds,
      list_bounds: listBounds,
      summary_bounds: summaryBounds,
      workspace_bounds: workspaceBounds,
      footer_bounds: footerBounds,
      detail_columns: detailColumnBounds,
      graph_bounds: graphBounds,
      graph_panel_bounds: graphPanelBounds,
      summary_card_count: summaryCardCount,
      tabs: tabLabels,
      evidence_panel_bounds: evidencePanelBounds,
      audit_panel_bounds: auditPanelBounds,
    },
    bad_responses: badResponses,
    console_errors: appConsoleErrors,
    page_errors: pageErrors,
    request_failures: appRequestFailures,
    screenshot: path.relative(root, screenshotPath),
    timestamp: new Date().toISOString(),
  };

  fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
  console.log(JSON.stringify(result, null, 2));
  exitCode = result.result === 'pass' ? 0 : 1;
} finally {
  await cdp.send('Emulation.clearDeviceMetricsOverride').catch(() => {});
  await page.close().catch(() => {});
}

process.exit(exitCode);

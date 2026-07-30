#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const baseUrl = process.env.UI_BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.UI_CDP_URL || 'http://127.0.0.1:9224';
const revision = process.env.CAMPAIGN_EVIDENCE_REVISION || 'r763';
const campaignId = process.env.CAMPAIGN_ID || 'campaign-exfil-default-1782729598739-e1d2dc37';
const visualFixture = process.env.CAMPAIGN_VISUAL_FIXTURE === 'true';
const requireRealApi = process.env.CAMPAIGN_REQUIRE_REAL_API
  ? process.env.CAMPAIGN_REQUIRE_REAL_API === 'true'
  : new URL(baseUrl).port === '30180';
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/campaign-detail');
const reportPath = path.join(evidenceDir, `inline-acceptance-${revision}.json`);
const screenshotPath = path.join(evidenceDir, `implementation-${revision}.png`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const env = { ...process.env };
  for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete env[key];
  const encodedSecret = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
    { encoding: 'utf8', env, timeout: 15_000 },
  );
  const secret = Buffer.from(encodedSecret, 'base64').toString('utf8');
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-windows-cdp-admin',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:read', 'campaign:read'],
    token_type: 'access',
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP tunnel is unavailable');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const errors = { console: [], page: [], requests: [], responses: [] };
const campaignApiResponses = [];
const campaignApiResponseTasks = [];
let campaignApiSemantics = null;
let exitStatus = 1;

page.on('console', (entry) => {
  if (entry.type() === 'error' && !entry.text().includes('favicon')) errors.console.push(entry.text());
});
page.on('pageerror', (error) => errors.page.push(error.message));
page.on('requestfailed', (request) => errors.requests.push({
  url: request.url(),
  error: request.failure()?.errorText ?? 'unknown',
}));
page.on('response', (response) => {
  if (response.url().includes('/api/v1/campaigns/') || response.url().includes('/v1/campaigns/')) {
    campaignApiResponses.push({ status: response.status(), url: response.url() });
    campaignApiResponseTasks.push(
      response.json().then((body) => {
        const payload = body?.data?.data ?? body?.data ?? body;
        if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return;
        const impactDataBacked = payload.impact_data_backed;
        campaignApiSemantics = {
          campaignId: payload.campaign_id ?? null,
          phaseDataBacked: payload.phase_data_backed === true,
          impactDataBacked: impactDataBacked && typeof impactDataBacked === 'object'
            ? Object.fromEntries(Object.entries(impactDataBacked).map(([key, value]) => [key, value === true]))
            : null,
          impactRowCounts: {
            assets: Array.isArray(payload.impact_assets) ? payload.impact_assets.length : null,
            accounts: Array.isArray(payload.impact_accounts) ? payload.impact_accounts.length : null,
            services: Array.isArray(payload.impact_services) ? payload.impact_services.length : null,
            departments: Array.isArray(payload.impact_departments) ? payload.impact_departments.length : null,
            campuses: Array.isArray(payload.impact_campuses) ? payload.impact_campuses.length : null,
            businessSystems: Array.isArray(payload.impact_business_systems) ? payload.impact_business_systems.length : null,
          },
          evidenceSummary: Array.isArray(payload.evidence_summary)
            ? payload.evidence_summary.map((item) => ({
                key: item?.key ?? null,
                current: typeof item?.current === 'number' ? item.current : null,
                expected: typeof item?.expected === 'number' ? item.expected : null,
                available: item?.available === true,
              }))
            : null,
          statusTransitions: Array.isArray(payload.status_transitions)
            ? payload.status_transitions.map((item) => ({
                status: item?.status ?? null,
                changedAt: item?.changed_at ?? null,
                source: item?.source ?? null,
              }))
            : null,
        };
      }).catch(() => {}),
    );
  }
  if (response.status() >= 400 && response.url().startsWith(baseUrl)) {
    errors.responses.push({ status: response.status(), url: response.url() });
  }
});

try {
  await page.setViewportSize({ width: 1920, height: 1080 });
  const url = new URL(`/campaigns/${encodeURIComponent(campaignId)}`, baseUrl);
  if (visualFixture) url.searchParams.set('__codex_ui_breakdown_production', '1');
  if (requireRealApi) url.hash = new URLSearchParams({ codex_smoke_token: smokeToken() }).toString();
  url.searchParams.set('campaignDetailAcceptanceTs', String(Date.now()));
  await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  const detail = page.locator('[data-page-id="campaign-detail"]');
  await detail.waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  await page.waitForTimeout(600);

  const title = await detail.locator('.taf-campaign-detail-titlebar h1').innerText();
  const finalUrl = page.url().replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
  const ringChecks = {
    campaignScore: await detail.locator('.taf-campaign-detail-risk canvas').count(),
    impactScope: await detail.locator('.taf-campaign-detail-impact-panel canvas').count(),
    evidenceCompleteness: await detail.locator('.taf-campaign-detail-evidence-panel canvas').count(),
  };

  const alertFilters = [];
  for (const risk of ['高危', '中危', '低危', '全部']) {
    const button = detail.locator('.taf-campaign-detail-alert-filter button', { hasText: risk });
    await button.click();
    await page.waitForTimeout(80);
    const selected = await button.getAttribute('aria-pressed');
    const rows = await detail.locator('.taf-campaign-detail-alerts .ant-table-tbody > tr:not(.ant-table-placeholder)').count();
    const values = await detail.locator('.taf-campaign-detail-alerts .ant-table-tbody > tr:not(.ant-table-placeholder) td:nth-child(5)').allTextContents();
    const emptyVisible = await detail.locator('.taf-campaign-detail-alerts .ant-empty:visible').count() > 0;
    alertFilters.push({
      risk,
      selected: selected === 'true',
      routeAlertRisk: new URL(page.url()).searchParams.get('alertRisk'),
      rows,
      emptyVisible,
      rowsMatch: risk === '全部'
        ? rows > 0 || emptyVisible
        : rows > 0
          ? values.every((value) => value.includes(risk.replace('危', '')))
          : emptyVisible,
    });
  }

  const expectedImpactSignatures = {
    资产: '受影响资产',
    账号: '关键账号',
    服务: '关键服务',
    部门: '关键部门',
    校区: '关键校区',
    业务系统: '关键业务系统',
  };
  const impactTabs = [];
  for (const label of ['资产', '账号', '服务', '部门', '校区', '业务系统']) {
    const tab = detail.locator('.taf-campaign-detail-impact-tab', { hasText: label });
    await tab.click();
    await page.waitForTimeout(120);
    const panel = detail.locator('.taf-campaign-detail-impact-panel');
    const contentText = (await panel.innerText()).replace(/\s+/g, ' ').trim();
    const dataRows = await panel.locator('.ant-table-tbody > tr:not(.ant-table-placeholder), [role="row"].taf-campaign-impact-account-table__row').count();
    const emptyVisible = await panel.locator('.ant-empty:visible, .taf-campaign-impact-table-empty:visible, .taf-campaign-impact-empty:visible').count() > 0;
    const screenshot = path.join(evidenceDir, `impact-${label}-${revision}.png`);
    fs.mkdirSync(evidenceDir, { recursive: true });
    await panel.screenshot({ path: screenshot });
    impactTabs.push({
      label,
      selected: await tab.evaluate((element) => element.classList.contains('is-active')),
      routeImpact: new URL(page.url()).searchParams.get('impact'),
      signature: expectedImpactSignatures[label],
      signatureVisible: contentText.includes(expectedImpactSignatures[label]),
      dataRows,
      emptyVisible,
      inlineCanvas: await panel.locator('canvas').count(),
      visibleModal: await page.locator('.taf-campaign-impact-modal:visible').count(),
      screenshot: path.relative(root, screenshot),
    });
  }

  const measureLayout = async () => detail.evaluate((element) => {
    const rect = (selector) => {
      const node = element.querySelector(selector);
      if (!node) return null;
      const box = node.getBoundingClientRect();
      return {
        left: Math.round(box.left),
        top: Math.round(box.top),
        right: Math.round(box.right),
        bottom: Math.round(box.bottom),
        width: Math.round(box.width),
        height: Math.round(box.height),
      };
    };
    const overlap = (a, b) => Boolean(a && b && a.left < b.right - 1 && a.right > b.left + 1 && a.top < b.bottom - 1 && a.bottom > b.top + 1);
    const evidenceOverview = rect('.taf-campaign-detail-evidence-panel');
    const evidenceTable = rect('.taf-campaign-detail-evidence-summary-panel');
    const alertTable = element.querySelector('.taf-campaign-detail-alerts .ant-table-content');
    const scoreChart = rect('.taf-campaign-detail-risk > div');
    const scoreLabel = rect('.taf-campaign-detail-risk__label');
    const businessScrollOwner = document.querySelector('.taf-main');
    const businessScrollStyle = businessScrollOwner ? getComputedStyle(businessScrollOwner) : null;
    const businessGrid = element.querySelector('.taf-campaign-detail-grid');
    const businessGridBox = businessGrid?.getBoundingClientRect();
    const businessScrollBox = businessScrollOwner?.getBoundingClientRect();
    const businessScrollEnabled = Boolean(
      businessScrollOwner
      && businessScrollStyle
      && ['auto', 'scroll'].includes(businessScrollStyle.overflowY)
      && businessScrollOwner.scrollHeight > businessScrollOwner.clientHeight + 2,
    );
    const businessContentReachable = Boolean(
      businessGridBox
      && businessScrollBox
      && (
        businessGridBox.bottom <= businessScrollBox.bottom + 2
        || (
          businessScrollEnabled
          && businessGridBox.bottom - businessScrollBox.top + businessScrollOwner.scrollTop
            <= businessScrollOwner.scrollHeight + 2
        )
      ),
    );
    return {
      viewport: { width: window.innerWidth, height: window.innerHeight },
      horizontalOverflow: document.documentElement.scrollWidth > window.innerWidth + 2,
      titlebar: rect('.taf-campaign-detail-titlebar'),
      profile: rect('.taf-campaign-detail-profile'),
      businessGrid: rect('.taf-campaign-detail-grid'),
      businessScroll: businessScrollOwner ? {
        clientHeight: businessScrollOwner.clientHeight,
        scrollHeight: businessScrollOwner.scrollHeight,
        overflowY: businessScrollStyle?.overflowY ?? null,
        enabled: businessScrollEnabled,
      } : null,
      businessContentReachable,
      phasePanel: rect('.taf-campaign-detail-phase-panel'),
      bottomGrid: rect('.taf-campaign-detail-bottom-grid'),
      rail: rect('.taf-campaign-detail-rail'),
      scoreChart,
      scoreLabel,
      evidenceOverview,
      evidenceTable,
      evidenceSectionsOverlap: overlap(evidenceOverview, evidenceTable),
      phaseBottomOverlap: overlap(rect('.taf-campaign-detail-phase-panel'), rect('.taf-campaign-detail-bottom-grid')),
      mainRailOverlap: overlap(rect('.taf-campaign-detail-main'), rect('.taf-campaign-detail-rail')),
      profileGridOverlap: overlap(rect('.taf-campaign-detail-profile'), rect('.taf-campaign-detail-grid')),
      alertTableHorizontalOverflow: Boolean(
        alertTable
        && alertTable.scrollWidth > alertTable.clientWidth + 8
        && !['auto', 'scroll'].includes(getComputedStyle(alertTable).overflowX),
      ),
      businessGridClearsBottomBar: businessContentReachable,
      scoreLabelInsideChart: Boolean(scoreChart && scoreLabel
        && scoreLabel.left >= scoreChart.left
        && scoreLabel.right <= scoreChart.right
        && scoreLabel.top >= scoreChart.top
        && scoreLabel.bottom <= scoreChart.bottom),
    };
  });

  const responsiveLayouts = [];
  for (const viewport of [
    { width: 1920, height: 1080 },
    { width: 1500, height: 900 },
    { width: 1366, height: 768 },
    { width: 1024, height: 768 },
  ]) {
    await page.setViewportSize(viewport);
    await page.waitForTimeout(180);
    responsiveLayouts.push({ viewport, ...(await measureLayout()) });
  }
  await page.setViewportSize({ width: 1920, height: 1080 });
  await detail.locator('.taf-campaign-detail-impact-tab', { hasText: '资产' }).click();
  await page.waitForTimeout(180);
  await page.evaluate(() => window.scrollTo(0, 0));
  await Promise.all(campaignApiResponseTasks);
  const layout = await measureLayout();
  const visualContract = await detail.evaluate((element) => {
    const titleNode = element.querySelector('.taf-campaign-detail-titlebar h1');
    const titleStyle = titleNode ? getComputedStyle(titleNode) : null;
    const responseSteps = Array.from(element.querySelectorAll('.taf-campaign-detail-response-step'));
    const responseLabels = responseSteps.map((node) => node.querySelector('strong')?.textContent?.trim() ?? '');
    const responseFlow = element.querySelector('.taf-campaign-detail-response-flow');
    const responseFlowRect = responseFlow?.getBoundingClientRect();
    const responseTimelineNodes = Array.from(element.querySelectorAll(
      '.taf-campaign-detail-response-step, .taf-campaign-detail-response-connector',
    ));
    const responseTimelineInsideFlow = Boolean(responseFlowRect) && responseTimelineNodes.every((node) => {
      const rect = node.getBoundingClientRect();
      return rect.left >= responseFlowRect.left - 1
        && rect.right <= responseFlowRect.right + 1
        && rect.top >= responseFlowRect.top - 1
        && rect.bottom <= responseFlowRect.bottom + 1;
    });
    const responseTimesClipped = responseSteps.some((node) => {
      const time = node.querySelector('span');
      return Boolean(time && time.scrollWidth > time.clientWidth + 1);
    });
    const digestRows = Array.from(element.querySelectorAll('.taf-campaign-detail-evidence-digest__row'));
    const digestLabels = digestRows.map((node) => node.querySelector('span')?.textContent?.trim() ?? '');
    const alertRows = Array.from(element.querySelectorAll('.taf-campaign-detail-alerts .ant-table-tbody > tr:not(.ant-table-placeholder)'));
    const alertCellsOverlap = alertRows.some((row) => {
      const cells = Array.from(row.querySelectorAll('td')).map((cell) => cell.getBoundingClientRect());
      return cells.some((cell, index) => {
        const next = cells[index + 1];
        return Boolean(next && cell.right > next.left + 1);
      });
    });
    return {
      titleFontSize: titleStyle ? Number.parseFloat(titleStyle.fontSize) : 0,
      titleFontWeight: titleStyle ? Number.parseInt(titleStyle.fontWeight, 10) : 0,
      responseStepCount: responseSteps.length,
      responseLabels,
      responseIcons: element.querySelectorAll('.taf-campaign-detail-response-step > i .anticon').length,
      responseConnectorCount: element.querySelectorAll('.taf-campaign-detail-response-connector').length,
      responseTimelineInsideFlow,
      responseTimesClipped,
      responseActionCount: element.querySelectorAll('.taf-campaign-detail-action-row').length,
      reviewRowCount: element.querySelectorAll('.taf-campaign-detail-review-row').length,
      digestRowCount: digestRows.length,
      digestLabels,
      alertCellsOverlap,
    };
  });

  fs.mkdirSync(evidenceDir, { recursive: true });
  await page.screenshot({ path: screenshotPath, fullPage: false });

  const result = {
    revision,
    generated_at: new Date().toISOString(),
    browser_backend: 'Windows Chrome CDP over Xshell 9224 -> 9222',
    browser: version.Browser,
    base_url: baseUrl,
    campaign_id: campaignId,
    data_mode: visualFixture ? 'visual-fixture' : 'runtime',
    require_real_api: requireRealApi,
    campaign_api_responses: campaignApiResponses,
    campaign_api_semantics: campaignApiSemantics,
    final_url: finalUrl,
    smoke_hash_consumed: !page.url().includes('codex_smoke_token'),
    title,
    titlePass: title.trim() === '战役详情',
    visualContract,
    visualContractPass: visualContract.titleFontSize >= 30
      && visualContract.titleFontWeight >= 700
      && visualContract.responseStepCount === 6
      && visualContract.responseIcons === 6
      && visualContract.responseConnectorCount === 5
      && visualContract.responseTimelineInsideFlow
      && !visualContract.responseTimesClipped
      && visualContract.responseActionCount > 0
      && visualContract.reviewRowCount > 0
      && ['发现', '研判', '遏制', '根除', '恢复', '复盘'].every((label, index) => visualContract.responseLabels[index] === label)
      && visualContract.digestRowCount === 5
      && ['首个可疑文件', 'SHA256', '首次外联域名', '解析 IP', '首次外联时间'].every((label, index) => visualContract.digestLabels[index] === label)
      && !visualContract.alertCellsOverlap,
    ringChecks,
    ringsPass: Object.values(ringChecks).every((count) => count === 1),
    alertFilters,
    alertFiltersPass: alertFilters.every((item) => item.selected
      && item.rowsMatch
      && (item.risk === '全部' ? item.routeAlertRisk === null : item.routeAlertRisk === item.risk)),
    impactTabs,
    impactTabsPass: impactTabs.every((item) => item.selected
      && item.signatureVisible
      && (item.dataRows > 0 || item.emptyVisible)
      && item.inlineCanvas === 1
      && item.visibleModal === 0),
    layout,
    responsiveLayouts,
    responsiveLayoutsPass: responsiveLayouts.every((item) => !item.horizontalOverflow
      && !item.phaseBottomOverlap
      && !item.mainRailOverlap
      && !item.profileGridOverlap
      && !item.evidenceSectionsOverlap
      && item.businessContentReachable),
    realApiPass: !requireRealApi || (
      !page.url().includes('codex_smoke_token')
      && campaignApiResponses.some((item) => item.status >= 200 && item.status < 300)
      && campaignApiSemantics !== null
      && campaignApiSemantics.impactDataBacked !== null
      && campaignApiSemantics.evidenceSummary !== null
      && campaignApiSemantics.statusTransitions !== null
    ),
    layoutPass: !layout.horizontalOverflow
      && !layout.alertTableHorizontalOverflow
      && layout.businessGridClearsBottomBar
      && !layout.evidenceSectionsOverlap
      && !layout.phaseBottomOverlap
      && !layout.mainRailOverlap
      && !layout.profileGridOverlap
      && layout.businessContentReachable
      && layout.scoreLabelInsideChart,
    errors,
  };
  result.status = result.titlePass
    && result.ringsPass
    && result.visualContractPass
    && result.alertFiltersPass
    && result.impactTabsPass
    && result.layoutPass
    && result.responsiveLayoutsPass
    && result.realApiPass
    && Object.values(errors).every((items) => items.length === 0)
    ? 'pass'
    : 'fail';
  fs.writeFileSync(reportPath, `${JSON.stringify(result, null, 2)}\n`);
  console.log(JSON.stringify({
    status: result.status,
    report: path.relative(root, reportPath),
    screenshot: path.relative(root, screenshotPath),
    title: result.title,
    visualContract: result.visualContract,
    rings: result.ringChecks,
    alertFilters: result.alertFilters,
    impactTabs: result.impactTabs,
    layout: result.layout,
    responsiveLayouts: result.responsiveLayouts,
    campaignApiResponses: result.campaign_api_responses,
    campaignApiSemantics: result.campaign_api_semantics,
    errors: result.errors,
  }, null, 2));
  exitStatus = result.status === 'pass' ? 0 : 1;
} finally {
  await page.close().catch(() => {});
}
process.exit(exitStatus);

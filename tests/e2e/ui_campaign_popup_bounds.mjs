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
const revision = process.env.CAMPAIGN_EVIDENCE_REVISION?.trim() || 'r741';
const frontendRevision = process.env.CAMPAIGN_FRONTEND_REVISION?.trim() || 'r748';
const backendRevision = process.env.CAMPAIGN_BACKEND_REVISION?.trim() || revision;
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/campaigns');
const outputPath = path.join(evidenceDir, `popup-and-responsive-${revision}.json`);

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

async function measurePopup(page, selector, name) {
  const modal = page.locator(`${selector}:visible`);
  await modal.waitFor({ state: 'visible', timeout: 15_000 });
  const surface = modal.locator('.ant-modal-content:visible, .ant-drawer-content:visible').first();
  await surface.waitFor({ state: 'visible', timeout: 15_000 });
  await page.waitForTimeout(350);
  const measurement = await surface.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    const body = element.querySelector('.ant-modal-body, .ant-drawer-body');
    const scrollHost = element.querySelector('.taf-campaign-detail-drawer__workspace, .taf-attack-chain-drawer-grid');
    const footer = element.querySelector('.taf-campaign-detail-drawer__footer, .taf-attack-chain-drawer-content > footer');
    const footerRect = footer?.getBoundingClientRect();
    const summary = element.querySelector('.taf-campaign-detail-drawer__summary, .taf-attack-chain-drawer-summary');
    const tabs = element.querySelector('.taf-detail-drawer-tabs');
    const summaryRect = summary?.getBoundingClientRect();
    const tabsRect = tabs?.getBoundingClientRect();
    const scrollHostRect = scrollHost?.getBoundingClientRect();
    const intersects = (left, right) => {
      const width = Math.min(left.right, right.right) - Math.max(left.left, right.left);
      const height = Math.min(left.bottom, right.bottom) - Math.max(left.top, right.top);
      return width > 2 && height > 2;
    };
    const panels = Array.from(element.querySelectorAll(
      '.taf-campaign-detail-drawer__workspace .taf-campaign-detail-drawer__section, .taf-attack-chain-drawer-grid .taf-panel',
    )).filter((node) => getComputedStyle(node).display !== 'none').map((node) => node.getBoundingClientRect());
    let panelOverlapCount = 0;
    for (let left = 0; left < panels.length; left += 1) {
      for (let right = left + 1; right < panels.length; right += 1) {
        if (intersects(panels[left], panels[right])) panelOverlapCount += 1;
      }
    }
    const footerButtons = Array.from(footer?.querySelectorAll('button') ?? [])
      .filter((node) => getComputedStyle(node).display !== 'none')
      .map((node) => node.getBoundingClientRect());
    let footerButtonOverlapCount = 0;
    for (let left = 0; left < footerButtons.length; left += 1) {
      for (let right = left + 1; right < footerButtons.length; right += 1) {
        if (intersects(footerButtons[left], footerButtons[right])) footerButtonOverlapCount += 1;
      }
    }
    return {
      viewport: { width: window.innerWidth, height: window.innerHeight },
      surfaceType: element.classList.contains('ant-drawer-content') ? 'drawer' : 'modal',
      bounds: {
        left: Math.round(rect.left),
        top: Math.round(rect.top),
        right: Math.round(rect.right),
        bottom: Math.round(rect.bottom),
        width: Math.round(rect.width),
        height: Math.round(rect.height),
      },
      body: body ? {
        clientWidth: body.clientWidth,
        clientHeight: body.clientHeight,
        scrollWidth: body.scrollWidth,
        scrollHeight: body.scrollHeight,
        overflowY: getComputedStyle(body).overflowY,
      } : null,
      scrollHost: scrollHost ? {
        clientHeight: scrollHost.clientHeight,
        scrollHeight: scrollHost.scrollHeight,
        overflowY: getComputedStyle(scrollHost).overflowY,
      } : null,
      footerVisible: Boolean(footerRect
        && footerRect.top >= rect.top
        && footerRect.bottom <= rect.bottom
        && footerRect.height > 0),
      contentOrderValid: Boolean(summaryRect && tabsRect && scrollHostRect
        && summaryRect.bottom <= tabsRect.top + 2
        && tabsRect.bottom <= scrollHostRect.top + 2),
      panelOverlapCount,
      footerButtonOverlapCount,
    };
  });
  const { bounds, viewport } = measurement;
  const rightMargin = viewport.width - bounds.right;
  const bottomMargin = viewport.height - bounds.bottom;
  return {
    name,
    ...measurement,
    withinViewport: bounds.left >= 16
      && bounds.top >= 16
      && bounds.right <= viewport.width - 16
      && bounds.bottom <= viewport.height - 16,
    intrinsicWidth: bounds.width <= Math.round(viewport.width * 0.9),
    intrinsicHeight: bounds.height <= Math.round(viewport.height * 0.92),
    rightAligned: measurement.surfaceType === 'drawer'
      ? bounds.right >= viewport.width - 48 && bounds.right <= viewport.width - 16
      : null,
    desktopDrawerGeometry: measurement.surfaceType === 'drawer' && viewport.width >= 1600
      ? Math.abs(bounds.width - 900) <= 2
        && Math.abs(rightMargin - 40) <= 2
        && Math.abs(bounds.top - 48) <= 2
        && Math.abs(bottomMargin - 48) <= 2
      : null,
    reportGeometry: name === 'campaign-report' && viewport.width >= 1600
      ? Math.abs(bounds.width - 1200) <= 2 && Math.abs(bounds.height - 740) <= 2
      : null,
    intrinsicImpactGeometry: name.startsWith('impact-')
      ? bounds.width <= 1040
        && bounds.width <= Math.round(viewport.width * 0.8)
        && bounds.height <= Math.min(760, viewport.height - 96)
      : null,
  };
}

fs.mkdirSync(evidenceDir, { recursive: true });
const version = await fetch(`${cdpUrl}/json/version`).then((response) => response.json());
const targetList = await fetch(`${cdpUrl}/json/list`).then((response) => response.json());
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const existingPages = context.pages();
const carrierPage = existingPages.find((candidate) => candidate.url().startsWith(baseUrl))
  ?? existingPages[0]
  ?? await context.newPage();
for (const stalePage of existingPages) {
  if (stalePage !== carrierPage && stalePage.url().startsWith(baseUrl)) await stalePage.close().catch(() => {});
}
let page = await context.newPage();
const errors = { responses: [], console: [], page: [], requests: [] };
const report = {
  revision,
  timestamp: new Date().toISOString(),
  browser: version.Browser,
  protocol: version['Protocol-Version'],
  cdpTargetCount: Array.isArray(targetList) ? targetList.filter((target) => target.type === 'page').length : 0,
  release: `campaign-domain-ui-${frontendRevision}-backend-${backendRevision}`,
  frontendRelease: `campaign-domain-${frontendRevision}`,
  backendRelease: `campaign-domain-${backendRevision}`,
  popups: [],
  responsive: [],
  drawerResponsive: [],
  impactCloseChecks: [],
  errors,
  result: 'fail',
};

const persist = () => fs.writeFileSync(outputPath, `${JSON.stringify(report, null, 2)}\n`);
const bindErrors = (targetPage) => {
  targetPage.on('response', (response) => {
    if (response.status() >= 400) errors.responses.push({ status: response.status(), url: response.url() });
  });
  targetPage.on('console', (entry) => {
    if (entry.type() === 'error') errors.console.push(entry.text());
  });
  targetPage.on('pageerror', (error) => errors.page.push(error.message));
  targetPage.on('requestfailed', (request) => errors.requests.push({ url: request.url(), error: request.failure()?.errorText ?? 'unknown' }));
};
bindErrors(page);
persist();

try {
  await page.setViewportSize({ width: 1920, height: 1080 });
  const token = smokeToken();
  await page.goto(`${baseUrl}/login`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.evaluate((accessToken) => {
    localStorage.removeItem('traffic-ui-refresh-token');
    localStorage.setItem('traffic-ui-token', accessToken);
  }, token);

  await page.goto(`${baseUrl}/campaigns?popupBoundsTs=${Date.now()}`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-campaign-workbench').waitFor({ state: 'visible', timeout: 15_000 });
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  const firstRow = page.locator('.taf-campaign-list-panel .ant-table-tbody tr').first();
  await firstRow.waitFor({ state: 'visible', timeout: 15_000 });
  const campaignId = (await firstRow.locator('td').first().innerText()).trim();
  report.campaignId = campaignId;

  await page.locator('.taf-campaign-list-panel > .taf-panel__header button').nth(1).click();
  report.popups.push(await measurePopup(page, '.taf-campaign-action-drawer', 'operation-receipt'));
  persist();
  await page.screenshot({ path: path.join(evidenceDir, `popup-operation-${revision}.png`) });
  await page.locator('.taf-campaign-action-drawer:visible .ant-modal-close').click();

  await firstRow.getByRole('button', { name: `查看${campaignId}详情` }).click();
  report.popups.push(await measurePopup(page, '.taf-campaign-detail-drawer', 'campaign-detail'));
  persist();
  await page.screenshot({ path: path.join(evidenceDir, `popup-campaign-detail-${revision}.png`) });
  await page.getByRole('button', { name: '关闭战役详情' }).click();

  await page.goto(`${baseUrl}/campaigns/${encodeURIComponent(campaignId)}?popupBoundsTs=${Date.now()}`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('[data-page-id="campaign-detail"]').waitFor({ state: 'visible', timeout: 15_000 });
  await page.getByRole('button', { name: '生成战役报告' }).click();
  report.popups.push(await measurePopup(page, '.taf-campaign-report-modal', 'campaign-report'));
  persist();
  await page.screenshot({ path: path.join(evidenceDir, `popup-campaign-report-${revision}.png`) });
  await page.locator('.taf-campaign-report-modal:visible .ant-modal-close').click();

  await page.goto(`${baseUrl}/attack-chains?chain=${encodeURIComponent(campaignId)}&popupBoundsTs=${Date.now()}`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-attack-chain').waitFor({ state: 'visible', timeout: 15_000 });
  await page.locator('.taf-attack-canvas:not(.is-empty)').waitFor({ state: 'visible', timeout: 20_000 });
  await page.getByRole('button', { name: '链路详情' }).click();
  report.popups.push(await measurePopup(page, '.taf-attack-chain-detail-drawer', 'attack-chain-detail'));
  persist();
  await page.screenshot({ path: path.join(evidenceDir, `popup-attack-chain-${revision}.png`) });
  await page.getByRole('button', { name: '关闭攻击链详情' }).click();

  for (const viewport of [{ width: 960, height: 900 }, { width: 640, height: 860 }]) {
    const label = `${viewport.width}x${viewport.height}`;
    await page.setViewportSize(viewport);
    await page.goto(`${baseUrl}/campaigns?__codex_ui_breakdown_production=1&__codex_page_id=drawer-campaign-detail&drawerResponsiveTs=${Date.now()}`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
    const campaignDrawer = await measurePopup(page, '.taf-campaign-detail-drawer', `campaign-detail-${label}`);
    report.drawerResponsive.push({
      ...campaignDrawer,
      safeMargins: campaignDrawer.bounds.left >= 16
        && campaignDrawer.bounds.top >= 16
        && campaignDrawer.bounds.right <= viewport.width - 16
        && campaignDrawer.bounds.bottom <= viewport.height - 16,
      internalScrollReady: Boolean(campaignDrawer.scrollHost
        && ['auto', 'scroll'].includes(campaignDrawer.scrollHost.overflowY)
        && campaignDrawer.scrollHost.scrollHeight >= campaignDrawer.scrollHost.clientHeight),
    });
    await page.screenshot({ path: path.join(evidenceDir, `popup-campaign-detail-${label}-${revision}.png`) });
    await page.getByRole('button', { name: '关闭战役详情' }).click();

    await page.goto(`${baseUrl}/attack-chains?__codex_ui_breakdown_production=1&__codex_page_id=drawer-attack-chain-detail&drawerResponsiveTs=${Date.now()}`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
    const attackDrawer = await measurePopup(page, '.taf-attack-chain-detail-drawer', `attack-chain-detail-${label}`);
    report.drawerResponsive.push({
      ...attackDrawer,
      safeMargins: attackDrawer.bounds.left >= 16
        && attackDrawer.bounds.top >= 16
        && attackDrawer.bounds.right <= viewport.width - 16
        && attackDrawer.bounds.bottom <= viewport.height - 16,
      internalScrollReady: Boolean(attackDrawer.scrollHost
        && ['auto', 'scroll'].includes(attackDrawer.scrollHost.overflowY)
        && attackDrawer.scrollHost.scrollHeight >= attackDrawer.scrollHost.clientHeight),
    });
    await page.screenshot({ path: path.join(evidenceDir, `popup-attack-chain-detail-${label}-${revision}.png`) });
    await page.getByRole('button', { name: '关闭攻击链详情' }).click();
  }

  const impacts = ['account', 'business-system', 'service', 'campus', 'department'];
  for (const impact of impacts) {
    await page.setViewportSize({ width: 1366, height: 768 });
    const visualUrl = new URL(`/campaigns/${encodeURIComponent(campaignId)}`, baseUrl);
    visualUrl.searchParams.set('__codex_ui_breakdown_production', '1');
    visualUrl.searchParams.set('__codex_page_id', `campaign-detail-impact-${impact}`);
    visualUrl.searchParams.set('impact', impact);
    visualUrl.searchParams.set('responsiveTs', String(Date.now()));
    await page.goto(visualUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
    const impactModal = page.locator('.taf-campaign-impact-modal:visible');
    await impactModal.waitFor({ state: 'visible', timeout: 15_000 });
    const modalMeasurement = await measurePopup(page, '.taf-campaign-impact-modal', `impact-${impact}`);
    report.popups.push(modalMeasurement);
    const focus = impactModal.locator(`[data-page-id="campaign-detail-impact-${impact}"]`);
    await focus.waitFor({ state: 'visible', timeout: 15_000 });
    await page.waitForTimeout(350);
    const measurement = await focus.evaluate((element) => {
      const host = element.closest('.ant-modal-body');
      const rect = element.getBoundingClientRect();
      const hostRect = host?.getBoundingClientRect();
      const donut = element.querySelector('.taf-campaign-impact-account-donut');
      const donutRect = donut?.getBoundingClientRect();
      const table = element.querySelector('.taf-campaign-impact-account-table');
      const tableRect = table?.getBoundingClientRect();
      const lastCellRight = Array.from(element.querySelectorAll('.taf-campaign-impact-account-table__row'))
        .map((row) => row.lastElementChild)
        .filter(Boolean)
        .reduce((right, cell) => Math.max(right, cell.getBoundingClientRect().right), 0);
      const riskPercentages = Array.from(element.querySelectorAll('.taf-campaign-impact-account-risk-row em'))
        .map((node) => node.textContent?.trim() ?? '');
      const topbar = document.querySelector('.taf-topbar');
      const sidebar = document.querySelector('.taf-sidebar');
      const campaignPage = document.querySelector('[data-page-id="campaign-detail"]');
      return {
        viewport: { width: window.innerWidth, height: window.innerHeight },
        hostContext: {
          topbarVisible: Boolean(topbar && getComputedStyle(topbar).display !== 'none'),
          sidebarVisible: Boolean(sidebar && getComputedStyle(sidebar).display !== 'none'),
          campaignPageVisible: Boolean(campaignPage && getComputedStyle(campaignPage).display !== 'none'),
          bodyHorizontalOverflow: document.documentElement.scrollWidth > window.innerWidth + 2,
        },
        focus: { left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom, width: rect.width, height: rect.height },
        host: host && hostRect ? {
          left: hostRect.left,
          top: hostRect.top,
          right: hostRect.right,
          bottom: hostRect.bottom,
          width: hostRect.width,
          height: hostRect.height,
          clientWidth: host.clientWidth,
          scrollWidth: host.scrollWidth,
          clientHeight: host.clientHeight,
          scrollHeight: host.scrollHeight,
          overflowX: getComputedStyle(host).overflowX,
          overflowY: getComputedStyle(host).overflowY,
        } : null,
        echartsCanvasCount: element.querySelectorAll('.taf-campaign-impact-account-donut canvas').length,
        donut: donutRect ? { width: donutRect.width, height: donutRect.height } : null,
        riskPercentages,
        table: table && tableRect ? {
          clientWidth: table.clientWidth,
          scrollWidth: table.scrollWidth,
          left: tableRect.left,
          right: tableRect.right,
          lastCellRight,
        } : null,
        transform: getComputedStyle(element).transform,
      };
    });
    await page.screenshot({ path: path.join(evidenceDir, `responsive-impact-${impact}-${revision}.png`) });
    const allLink = focus.locator('.taf-campaign-impact-account-all-link');
    await allLink.scrollIntoViewIfNeeded();
    const linkVisible = await allLink.isVisible();
    report.responsive.push({
      impact,
      modal: modalMeasurement,
      ...measurement,
      noHorizontalOverflow: Boolean(measurement.host && measurement.host.scrollWidth <= measurement.host.clientWidth + 2),
      hostContextVisible: measurement.hostContext.topbarVisible
        && measurement.hostContext.sidebarVisible
        && measurement.hostContext.campaignPageVisible,
      noPageHorizontalOverflow: !measurement.hostContext.bodyHorizontalOverflow,
      nativeLayout: measurement.transform === 'none',
      echartsDonut: measurement.echartsCanvasCount > 0,
      donutCircular: Boolean(measurement.donut && Math.abs(measurement.donut.width - measurement.donut.height) <= 2),
      riskPercentagesComplete: measurement.riskPercentages.length === 3
        && measurement.riskPercentages.every((value) => /^\d+(?:\.\d+)?%$/.test(value)),
      tableFits: Boolean(measurement.table
        && measurement.table.scrollWidth <= measurement.table.clientWidth + 2
        && measurement.table.lastCellRight <= measurement.table.right + 2),
      bottomLinkReachable: linkVisible,
    });
    persist();
  }

  for (const impact of impacts) {
    await page.setViewportSize({ width: 1366, height: 768 });
    const realUrl = new URL(`/campaigns/${encodeURIComponent(campaignId)}`, baseUrl);
    realUrl.searchParams.set('impact', impact);
    realUrl.searchParams.set('closeTs', String(Date.now()));
    await page.goto(realUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
    await page.locator('[data-page-id="campaign-detail"]').waitFor({ state: 'visible', timeout: 15_000 });
    const impactModal = page.locator('.taf-campaign-impact-modal:visible');
    await impactModal.waitFor({ state: 'visible', timeout: 15_000 });
    await page.waitForFunction(() => {
      const title = document.querySelector('.taf-campaign-detail-profile-main p')?.textContent?.trim();
      const phases = document.querySelectorAll('.taf-campaign-detail-phase-card').length;
      return Boolean(title && title !== '战役详情加载中' && phases > 0);
    }, undefined, { timeout: 20_000 });
    const readCampaignContent = () => page.evaluate(() => ({
      campaignId: document.querySelector('.taf-campaign-detail-profile-main h2')?.childNodes[0]?.textContent?.trim() ?? '',
      title: document.querySelector('.taf-campaign-detail-profile-main p')?.textContent?.trim() ?? '',
      phaseCount: document.querySelectorAll('.taf-campaign-detail-phase-card').length,
      alertPanelTitle: document.querySelector('.taf-campaign-detail-alerts .taf-panel__header h2')?.textContent?.trim() ?? '',
      riskText: document.querySelector('.taf-campaign-detail-risk .ant-progress-text')?.textContent?.trim() ?? '',
    }));
    const contentBeforeClose = await readCampaignContent();
    await impactModal.locator('.ant-modal-close').click();
    await impactModal.waitFor({ state: 'hidden', timeout: 8_000 });
    await page.waitForFunction((expected) => {
      const title = document.querySelector('.taf-campaign-detail-profile-main p')?.textContent?.trim();
      const phases = document.querySelectorAll('.taf-campaign-detail-phase-card').length;
      return title === expected.title && phases === expected.phaseCount && phases > 0;
    }, contentBeforeClose, { timeout: 15_000 });
    const contentAfterClose = await readCampaignContent();
    const closedState = await page.evaluate((expectedCampaignId) => {
      const current = new URL(window.location.href);
      const topbar = document.querySelector('.taf-topbar');
      const sidebar = document.querySelector('.taf-sidebar');
      const campaignPage = document.querySelector('[data-page-id="campaign-detail"]');
      const visible = (element) => Boolean(
        element
        && getComputedStyle(element).display !== 'none'
        && element.getBoundingClientRect().width > 0
        && element.getBoundingClientRect().height > 0
      );
      return {
        modalHidden: !visible(document.querySelector('.taf-campaign-impact-modal .ant-modal-content')),
        campaignDetailVisible: visible(campaignPage),
        routeRetained: current.pathname === `/campaigns/${encodeURIComponent(expectedCampaignId)}`,
        impactParameterRemoved: !current.searchParams.has('impact'),
        hostContextVisible: visible(topbar) && visible(sidebar) && visible(campaignPage),
        pathname: current.pathname,
        search: current.search,
      };
    }, campaignId);
    const campaignDetailLoaded = contentAfterClose.title !== '战役详情加载中'
      && contentAfterClose.phaseCount > 0
      && contentAfterClose.alertPanelTitle.length > 0
      && contentAfterClose.riskText !== '0';
    const campaignContentRetained = JSON.stringify(contentAfterClose) === JSON.stringify(contentBeforeClose);
    const screenshot = path.join(evidenceDir, `responsive-impact-${impact}-after-close-${revision}.png`);
    await page.screenshot({ path: screenshot });
    report.impactCloseChecks.push({
      impact,
      ...closedState,
      campaignDetailLoaded,
      campaignContentRetained,
      contentBeforeClose,
      contentAfterClose,
      screenshot: path.relative(root, screenshot),
      result: closedState.modalHidden
        && closedState.campaignDetailVisible
        && closedState.routeRetained
        && closedState.impactParameterRemoved
        && closedState.hostContextVisible
        && campaignDetailLoaded
        && campaignContentRetained
        ? 'pass'
        : 'fail',
    });
    persist();
  }

  const externalRequestFailures = errors.requests.filter((item) => item.url.startsWith('https://api.yhchj.com/'));
  const appRequestFailures = errors.requests.filter((item) => !item.url.startsWith('https://api.yhchj.com/'));
  const externalConsoleErrors = externalRequestFailures.length
    ? errors.console.filter((entry) => entry === 'Failed to load resource: net::ERR_CONNECTION_CLOSED')
    : [];
  const appConsoleErrors = errors.console.filter((entry) => !externalConsoleErrors.includes(entry) && !entry.includes('favicon'));
  const externalPageErrors = externalRequestFailures.length ? errors.page.filter((entry) => entry === 'Object') : [];
  const appPageErrors = errors.page.filter((entry) => !externalPageErrors.includes(entry));
  report.externalBrowserNoise = {
    console: externalConsoleErrors,
    page: externalPageErrors,
    requests: externalRequestFailures,
  };
  report.appErrors = {
    responses: errors.responses,
    console: appConsoleErrors,
    page: appPageErrors,
    requests: appRequestFailures,
  };
  const popupPass = report.popups.length === 9
    && report.popups.every((item) => item.withinViewport && item.intrinsicWidth && item.intrinsicHeight)
    && report.popups.find((item) => item.name === 'campaign-report')?.reportGeometry === true
    && report.popups.filter((item) => item.name.startsWith('impact-')).length === 5
    && report.popups.filter((item) => item.name.startsWith('impact-')).every((item) => item.intrinsicImpactGeometry === true);
  const desktopDrawers = report.popups.filter((item) => item.surfaceType === 'drawer');
  const drawerPass = desktopDrawers.length === 2
    && ['campaign-detail', 'attack-chain-detail'].every((name) => desktopDrawers.some((item) => item.name === name))
    && desktopDrawers.every((item) => item.rightAligned === true && item.desktopDrawerGeometry === true);
  const drawerResponsivePass = report.drawerResponsive.length === 4
    && report.drawerResponsive.every((item) => item.safeMargins
      && item.internalScrollReady
      && item.footerVisible
      && item.contentOrderValid
      && item.panelOverlapCount === 0
      && item.footerButtonOverlapCount === 0);
  const responsivePass = report.responsive.length === 5
    && report.responsive.every((item) => item.noHorizontalOverflow
      && item.hostContextVisible
      && item.noPageHorizontalOverflow
      && item.nativeLayout
      && item.echartsDonut
      && item.donutCircular
      && item.riskPercentagesComplete
      && item.tableFits
      && item.bottomLinkReachable);
  const impactClosePass = report.impactCloseChecks.length === 5
    && report.impactCloseChecks.every((item) => item.result === 'pass');
  report.result = popupPass
    && drawerPass
    && drawerResponsivePass
    && responsivePass
    && impactClosePass
    && errors.responses.length === 0
    && appPageErrors.length === 0
    && appRequestFailures.length === 0
    && appConsoleErrors.length === 0
    ? 'pass'
    : 'fail';
} catch (error) {
  report.fatal = error instanceof Error ? error.stack ?? error.message : String(error);
} finally {
  persist();
  await page.close().catch(() => {});
}

console.log(JSON.stringify({ outputPath, result: report.result, popups: report.popups, drawerResponsive: report.drawerResponsive, responsive: report.responsive, errors: report.errors, fatal: report.fatal }, null, 2));
process.exit(report.result === 'pass' ? 0 : 1);

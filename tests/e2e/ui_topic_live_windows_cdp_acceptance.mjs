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
const revision = 'r758-r761';
const outputPath = path.join(root, `evidence/ui-image-breakdowns/pages/topics-live-windows-cdp-${revision}.json`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
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
    tenant_id: 'default',
    username: 'codex-topic-windows-cdp-admin',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'topic:read', 'topic:write', 'topic:export', 'audit:read', 'user:read'],
    token_type: 'access',
    session_id: `codex-topic-${revision}`,
    iat: now,
    exp: now + 3_600,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

function screenshotPath(topic) {
  const directory = topic === 'tunnel' ? 'topics-encrypted-tunnel' : topic === 'exfil' ? 'topics-data-exfiltration' : 'topics-apt-campaign';
  return path.join(root, `evidence/ui-image-breakdowns/pages/${directory}/implementation-${revision}-live.png`);
}

function boundsPass(bounds, maxWidth, maxHeight, viewport) {
  return bounds.width <= maxWidth
    && bounds.height <= maxHeight
    && bounds.left > 0
    && bounds.top > 0
    && bounds.right < viewport.width
    && bounds.bottom < viewport.height;
}

async function elementBounds(locator) {
  return locator.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    return {
      left: Math.round(rect.left),
      top: Math.round(rect.top),
      width: Math.round(rect.width),
      height: Math.round(rect.height),
      right: Math.round(rect.right),
      bottom: Math.round(rect.bottom),
    };
  });
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
for (const stalePage of context.pages()) {
  if (stalePage.url().includes('/topics?') && stalePage.url().includes('windowsTopicLiveTs=')) await stalePage.close().catch(() => {});
}
const page = await context.newPage();
const cdp = await page.context().newCDPSession(page);
const token = smokeToken();
const productBadResponses = [];
const productRequestFailures = [];
const consoleErrors = [];
const externalConsoleErrors = [];
const pageErrors = [];
const externalRuntimeEvents = [];
const apiPayloads = {};
let exitCode = 1;

page.on('response', async (response) => {
  const url = response.url();
  if (url.startsWith(`${baseUrl}/api/`) && response.status() >= 400) {
    productBadResponses.push({ status: response.status(), url });
  }
  const match = url.match(/\/api\/v1\/topics\/(tunnel|exfil|apt)(?:\?|$)/u);
  if (match && response.request().method() === 'GET' && response.ok()) {
    apiPayloads[match[1]] = await response.json().catch(() => ({}));
  }
});
page.on('requestfailed', (request) => {
  if (request.url().startsWith(baseUrl)) productRequestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? '' });
});
await page.route('https://api.yhchj.com/ip', async (route) => {
  externalRuntimeEvents.push({ type: 'fulfilled-external-ip-helper', url: route.request().url() });
  await route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ ip: '127.0.0.1', source: 'windows-cdp-acceptance' }),
  });
});
page.on('console', (entry) => {
  if (entry.type() === 'error') {
    const item = { text: entry.text(), location: entry.location() };
    if (item.location.url && !item.location.url.startsWith(baseUrl)) {
      externalConsoleErrors.push(item);
    } else {
      consoleErrors.push(item);
    }
  }
});
page.on('pageerror', (error) => pageErrors.push({ message: error.message, stack: error.stack ?? '' }));

async function openTopic(topic, viewport = { width: 1920, height: 1080 }) {
  await page.setViewportSize(viewport);
  const url = new URL(`/topics?topic=${topic}&tab=${topic}&windowsTopicLiveTs=${Date.now()}`, baseUrl);
  url.hash = `codex_smoke_token=${token}`;
  await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator(`.taf-topic-${topic === 'tunnel' ? 'tunnel' : topic === 'exfil' ? 'exfil' : 'apt'}-layout`).waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => {});
  await page.waitForFunction((key) => {
    const rootElement = document.querySelector('.taf-topic-page');
    return Boolean(rootElement && !rootElement.textContent?.includes('真实 API 数据加载失败') && rootElement.textContent?.includes(key));
  }, topic === 'apt' ? '关联战役数' : topic === 'exfil' ? '外传路径数' : '隧道协议数', { timeout: 20_000 });
  return url;
}

async function pageLayoutEvidence() {
  return page.evaluate(() => {
    const pageElement = document.querySelector('.taf-topic-page');
    const shellElement = document.querySelector('.taf-topic-shell');
    const pageRect = pageElement?.getBoundingClientRect();
    const shellRect = shellElement?.getBoundingClientRect();
    return {
      viewport: { width: window.innerWidth, height: window.innerHeight },
      document_width: document.documentElement.scrollWidth,
      body_width: document.body.scrollWidth,
      body_horizontal_overflow: Math.max(document.documentElement.scrollWidth, document.body.scrollWidth) > window.innerWidth + 2,
      page_bounds: pageRect ? { left: Math.round(pageRect.left), right: Math.round(pageRect.right), width: Math.round(pageRect.width), top: Math.round(pageRect.top), bottom: Math.round(pageRect.bottom) } : null,
      shell_bounds: shellRect ? { left: Math.round(shellRect.left), right: Math.round(shellRect.right), width: Math.round(shellRect.width), top: Math.round(shellRect.top), bottom: Math.round(shellRect.bottom) } : null,
      shell_overflow_y: shellElement ? getComputedStyle(shellElement).overflowY : '',
    };
  });
}

async function metricEvidence() {
  return page.locator('.taf-topic-kpis .taf-metric, .taf-topic-tunnel-kpis .taf-topic-tunnel-kpi').allTextContents();
}

async function verifyModal(button, endpointPattern, maxWidth, maxHeight, methods = ['POST'], evidenceName = '') {
  await button.click();
  const modal = page.locator('.taf-topic-governance-modal:visible .ant-modal-content');
  await modal.waitFor({ state: 'visible', timeout: 8_000 });
  const bounds = await elementBounds(modal);
  let screenshot = '';
  if (evidenceName) {
    const screenshotPath = path.join(root, `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/${evidenceName}-${revision}.png`);
    fs.mkdirSync(path.dirname(screenshotPath), { recursive: true });
    await page.screenshot({ path: screenshotPath, fullPage: false });
    screenshot = path.relative(root, screenshotPath);
  }
  const responsePromise = page.waitForResponse((response) => endpointPattern.test(response.url()) && methods.includes(response.request().method()), { timeout: 12_000 });
  await modal.getByRole('button', { name: '确认提交' }).click();
  const response = await responsePromise;
  await modal.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 8_000 });
  const passed = response.ok() && boundsPass(bounds, maxWidth, maxHeight, { width: 1920, height: 1080 });
  const modalRoot = page.locator('.taf-topic-governance-modal:visible');
  await modalRoot.locator('.ant-modal-close').click();
  await modalRoot.waitFor({ state: 'hidden', timeout: 8_000 });
  return { passed, status: response.status(), bounds, screenshot };
}

async function verifyDrawer(button, endpointPattern) {
  await button.click();
  const drawer = page.locator('.taf-topic-governance-drawer:visible');
  await drawer.waitFor({ state: 'visible', timeout: 8_000 });
  const bounds = await elementBounds(drawer);
  const responsePromise = page.waitForResponse((response) => endpointPattern.test(response.url()) && ['POST', 'PUT'].includes(response.request().method()), { timeout: 12_000 });
  await drawer.getByRole('button', { name: '确认保存' }).click();
  const response = await responsePromise;
  await drawer.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 8_000 });
  const passed = response.ok()
    && bounds.width <= 520
    && bounds.left > 0
    && bounds.right <= 1920
    && bounds.top === 0
    && bounds.bottom === 1080;
  await drawer.locator('.ant-drawer-close').click();
  return { passed, status: response.status(), bounds };
}

try {
  const topics = {};
  const responsive = [];
  for (const topic of ['tunnel', 'exfil', 'apt']) {
    console.error(`[topic-live] opening ${topic}`);
    const url = await openTopic(topic);
    const metrics = await metricEvidence();
    const layout = await pageLayoutEvidence();
    const canvasCount = await page.locator('.taf-topic-page canvas').count();
    const svgCount = await page.locator('.taf-topic-page svg').count();
    const screenshot = screenshotPath(topic);
    fs.mkdirSync(path.dirname(screenshot), { recursive: true });
    await page.screenshot({ path: screenshot, fullPage: false });
    for (const viewport of [{ width: 1600, height: 900 }, { width: 1366, height: 768 }]) {
      await page.setViewportSize(viewport);
      await page.waitForTimeout(250);
      const responsiveLayout = await pageLayoutEvidence();
      const responsiveScreenshot = path.join(
        root,
        `evidence/ui-image-breakdowns/pages/${topic === 'tunnel' ? 'topics-encrypted-tunnel' : topic === 'exfil' ? 'topics-data-exfiltration' : 'topics-apt-campaign'}/responsive-${viewport.width}x${viewport.height}-${revision}.png`,
      );
      fs.mkdirSync(path.dirname(responsiveScreenshot), { recursive: true });
      await page.screenshot({ path: responsiveScreenshot, fullPage: false });
      responsive.push({
        topic,
        viewport,
        layout: responsiveLayout,
        screenshot: path.relative(root, responsiveScreenshot),
        passed: !responsiveLayout.body_horizontal_overflow
          && Boolean(responsiveLayout.page_bounds)
          && responsiveLayout.page_bounds.left >= 0
          && responsiveLayout.page_bounds.right <= viewport.width + 2,
      });
    }
    await page.setViewportSize({ width: 1920, height: 1080 });
    topics[topic] = {
      route: url.toString().replace(/codex_smoke_token=[^&#]+/u, 'codex_smoke_token=<redacted>'),
      metrics,
      layout,
      canvas_count: canvasCount,
      svg_count: svgCount,
      screenshot: path.relative(root, screenshot),
    };
    console.error(`[topic-live] captured ${topic}`);
  }

  await openTopic('tunnel');
  console.error('[topic-live] verifying governance overlays');
  const governance = {
    scope: await verifyModal(page.getByTitle('编辑范围').first(), /\/api\/v1\/topics\/scopes\/tunnel$/u, 620, 760, ['PUT'], 'scope-modal'),
    saved_view: await verifyModal(page.getByTitle('保存视图').first(), /\/api\/v1\/topics\/views$/u, 620, 760),
    report_export: await verifyModal(page.getByTitle('导出报告').first(), /\/api\/v1\/topics\/reports\/export$/u, 620, 760),
    evidence_export: await verifyModal(page.getByTitle('导出证据包').first(), /\/api\/v1\/topics\/evidence-packages\/export$/u, 620, 760),
    subscription: await verifyDrawer(page.getByTitle('订阅').first(), /\/api\/v1\/topics\/subscriptions$/u),
  };

  const shareButton = page.getByTitle('分享').first();
  await shareButton.click();
  const shareMenu = page.locator('.ant-dropdown:visible');
  await shareMenu.waitFor({ state: 'visible', timeout: 8_000 });
  const shareBounds = await elementBounds(shareMenu);
  const shareResponsePromise = page.waitForResponse((response) => /\/api\/v1\/topics\/views\/[^/]+$/u.test(response.url()) && response.request().method() === 'PATCH', { timeout: 12_000 });
  await shareMenu.getByText('共享当前视图', { exact: true }).click();
  const shareResponse = await shareResponsePromise;
  governance.share = {
    passed: shareResponse.ok() && boundsPass(shareBounds, 360, 260, { width: 1920, height: 1080 }),
    status: shareResponse.status(),
    bounds: shareBounds,
  };
  console.error('[topic-live] governance overlays verified');

  const aptData = apiPayloads.apt?.data ?? apiPayloads.apt ?? {};
  const exfilData = apiPayloads.exfil?.data ?? apiPayloads.exfil ?? {};
  const tunnelData = apiPayloads.tunnel?.data ?? apiPayloads.tunnel ?? {};
  const dataChecks = {
    tunnel_live: Number(tunnelData?.summary?.session_count ?? 0) > 0,
    exfil_live: Number(exfilData?.summary?.path_count ?? 0) > 0,
    exfil_percent_sane: !topics.exfil.metrics.some((value) => /\\d{4,}(?:\\.\\d+)?%/u.test(value)),
    apt_30_day_window: Number(aptData?.time_range?.end ?? 0) - Number(aptData?.time_range?.start ?? 0) >= 29 * 24 * 60 * 60 * 1000,
    apt_live: Number(aptData?.summary?.campaign_count ?? 0) > 0 && Array.isArray(aptData?.campaigns) && aptData.campaigns.length > 0,
    apt_derived_metrics: ['cluster_density', 'lateral_move_links', 'persistence_signals', 'exfil_evidence_count', 'closure_rate', 'report_confidence']
      .every((key) => Object.hasOwn(aptData?.summary ?? {}, key)),
  };

  const result = {
    result: Object.values(dataChecks).every(Boolean)
      && Object.values(governance).every((item) => item.passed)
      && responsive.every((item) => item.passed)
      && productBadResponses.length === 0
      && productRequestFailures.length === 0
      && consoleErrors.length === 0
      && pageErrors.length === 0 ? 'pass' : 'fail',
    browser_backend: 'Windows Chrome CDP through Xshell 9224 -> 9222',
    browser: version.Browser,
    topics,
    governance,
    responsive,
    data_checks: dataChecks,
    api_summaries: {
      tunnel: tunnelData.summary,
      exfil: exfilData.summary,
      apt: aptData.summary,
    },
    product_bad_responses: productBadResponses,
    product_request_failures: productRequestFailures,
    console_errors: consoleErrors,
    external_console_errors: externalConsoleErrors,
    page_errors: pageErrors,
    external_runtime_events: externalRuntimeEvents,
    timestamp: new Date().toISOString(),
  };
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
  console.log(JSON.stringify(result, null, 2));
  exitCode = result.result === 'pass' ? 0 : 1;
} finally {
  await cdp.send('Emulation.clearDeviceMetricsOverride').catch(() => {});
  await page.close().catch(() => {});
}

process.exit(exitCode);

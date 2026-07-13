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
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/topics-apt-campaign/interaction-r259-topology-svg.json');
const screenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/topics-apt-campaign/interaction-r259-topology-svg.png');
const tunnelScreenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/topics-apt-campaign/interaction-r259-topology-svg-tunnel.png');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const encoded = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({ iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(), tenant_id: 'default', username: 'codex-windows-cdp-admin', roles: ['admin'], permissions: ['*', 'admin:*', 'topic:read', 'topic:write'], token_type: 'access', iat: now, exp: now + 1_800 })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
const consoleErrors = [];
const pageErrors = [];
const badResponses = [];
page.on('console', (entry) => { if (entry.type() === 'error') consoleErrors.push(entry.text()); });
page.on('pageerror', (error) => pageErrors.push(error.message));
page.on('response', (response) => { if (response.status() >= 400) badResponses.push({ status: response.status(), url: response.url() }); });

async function openTopic(topic) {
  const url = new URL(`/topics?topic=${topic}&tab=${topic}&__codex_ui_breakdown_production=1&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
  url.hash = `codex_smoke_token=${smokeToken()}`;
  await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  await page.locator('.taf-topic-page').waitFor({ state: 'visible', timeout: 15_000 });
  return url;
}

async function confirmVisibleTopicAction() {
  const drawer = page.locator('.taf-topic-action-drawer:visible');
  await drawer.waitFor({ state: 'visible', timeout: 5_000 });
  await drawer.getByRole('button', { name: '确认提交' }).click();
  await drawer.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 5_000 });
  const visible = await drawer.locator('.ant-alert-success').isVisible();
  await drawer.locator('.ant-drawer-close').click();
  return visible;
}

const tunnelUrl = await openTopic('tunnel');
await page.locator('.taf-topic-tunnel-layout').waitFor({ state: 'visible', timeout: 10_000 });
await page.locator('.taf-topic-tunnel-impact .taf-api-topology-svg').waitFor({ state: 'visible', timeout: 10_000 });
const tunnelTopologySvgCount = await page.locator('.taf-topic-tunnel-impact .taf-api-topology-svg').count();
await page.locator('.taf-topic-tunnel-panel-actions button').first().evaluate((button) => button.click());
const tunnelActionVisible = await confirmVisibleTopicAction();
await page.getByRole('button', { name: '隧道证据下一页' }).click();
const tunnelPageTwo = await page.locator('.taf-topic-tunnel-table-footer button[aria-current="page"]').textContent();
await page.getByRole('button', { name: '隧道源TOP5' }).click();
const activeTunnelTab = await page.locator('.taf-topic-tunnel-analysis-tabs button.is-active').textContent();
const tunnelScroll = await page.evaluate(() => window.getComputedStyle(document.querySelector('.taf-topic-tunnel-table-body')).overflowY);
await page.screenshot({ path: tunnelScreenshotPath, fullPage: false });

const exfilUrl = await openTopic('exfil');
await page.locator('.taf-topic-exfil-layout').waitFor({ state: 'visible', timeout: 10_000 });
await page.getByRole('button', { name: '导出总报告' }).first().click();
const exfilActionVisible = await confirmVisibleTopicAction();
const exfilCanvasCount = await page.locator('.taf-topic-exfil-canvas-panel canvas').count();

const aptUrl = await openTopic('apt');
await page.locator('.taf-topic-apt-layout').waitFor({ state: 'visible', timeout: 10_000 });
await page.locator('.taf-topic-apt-attack-map .taf-api-topology-svg').waitFor({ state: 'visible', timeout: 10_000 });
const aptTopologySvgCount = await page.locator('.taf-topic-apt-attack-map .taf-api-topology-svg').count();
await page.locator('.taf-topic-apt-trend-echart canvas').waitFor({ state: 'visible', timeout: 10_000 });
await page.locator('.taf-topic-apt-response-echart canvas').waitFor({ state: 'visible', timeout: 10_000 });
await page.getByRole('button', { name: 'APT 证据下一页' }).click();
const aptPageTwo = await page.locator('.taf-topic-apt-table-footer button[aria-current="page"]').textContent();
await page.locator('.taf-topic-apt-table-actions button').first().click();
const aptActionVisible = await confirmVisibleTopicAction();
await page.screenshot({ path: screenshotPath, fullPage: false });

const result = {
  result: tunnelTopologySvgCount === 1 && tunnelActionVisible && tunnelPageTwo === '2' && activeTunnelTab === '隧道源TOP5' && tunnelScroll === 'auto'
    && exfilActionVisible && exfilCanvasCount >= 1 && aptTopologySvgCount === 1 && aptPageTwo === '2' && aptActionVisible
    && badResponses.length === 0 && consoleErrors.length === 0 && pageErrors.length === 0 ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP', browser: version.Browser,
  routes: { tunnel: tunnelUrl.toString().replace(/codex_smoke_token=[^&#]+/, 'codex_smoke_token=<redacted>'), exfil: exfilUrl.toString().replace(/codex_smoke_token=[^&#]+/, 'codex_smoke_token=<redacted>'), apt: aptUrl.toString().replace(/codex_smoke_token=[^&#]+/, 'codex_smoke_token=<redacted>') },
  tunnel_topology_svg_count: tunnelTopologySvgCount, tunnel_action_visible: tunnelActionVisible, tunnel_page_two: tunnelPageTwo, active_tunnel_tab: activeTunnelTab, tunnel_table_overflow_y: tunnelScroll,
  exfil_action_visible: exfilActionVisible, exfil_canvas_count: exfilCanvasCount, apt_topology_svg_count: aptTopologySvgCount, apt_page_two: aptPageTwo, apt_action_visible: aptActionVisible,
  bad_responses: badResponses, console_errors: consoleErrors, page_errors: pageErrors, screenshot: path.relative(root, screenshotPath), tunnel_screenshot: path.relative(root, tunnelScreenshotPath), timestamp: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await browser.close();
if (result.result !== 'pass') process.exitCode = 1;

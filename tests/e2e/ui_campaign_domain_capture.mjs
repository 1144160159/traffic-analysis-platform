#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const baseUrl = process.env.UI_BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.UI_CDP_URL || 'http://127.0.0.1:9224';
const revision = process.env.CAMPAIGN_EVIDENCE_REVISION || 'r744';
const injectedStylePath = process.env.CAMPAIGN_CAPTURE_STYLE_PATH
  ? path.resolve(root, process.env.CAMPAIGN_CAPTURE_STYLE_PATH)
  : null;
const reportPath = path.join(root, `doc/02_acceptance/02-regression/campaign-domain-captures-${revision}.json`);

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
  const base64url = (value) => Buffer.from(JSON.stringify(value)).toString('base64url');
  const header = base64url({ alg: 'HS256', typ: 'JWT' });
  const claims = base64url({
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
  });
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const version = await fetch(`${cdpUrl}/json/version`).then((response) => response.json());
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const bootstrap = await context.newPage();
const token = smokeToken();
await bootstrap.goto(`${baseUrl}/login`, { waitUntil: 'domcontentloaded', timeout: 45_000 });
await bootstrap.evaluate((accessToken) => localStorage.setItem('traffic-ui-token', accessToken), token);
await bootstrap.close();

const campaignResponse = await fetch(`${baseUrl}/api/v1/campaigns?limit=1`, {
  headers: { Authorization: `Bearer ${token}` },
});
const campaignPayload = await campaignResponse.json();
const campaignId = campaignPayload?.data?.campaigns?.[0]?.campaign_id
  ?? campaignPayload?.campaigns?.[0]?.campaign_id;
if (!campaignResponse.ok || !campaignId) throw new Error('Campaign API did not return a capture target');

const targets = [
  { category: 'pages', id: 'campaigns', path: '/campaigns', selector: '.taf-campaign-workbench' },
  { category: 'pages', id: 'campaign-detail', path: `/campaigns/${encodeURIComponent(campaignId)}`, selector: '[data-page-id="campaign-detail"]' },
  { category: 'overlays', id: 'drawer-campaign-detail', path: '/campaigns', selector: '.taf-campaign-detail-drawer:visible' },
  { category: 'overlays', id: 'modal-campaign-report-export', path: `/campaigns/${encodeURIComponent(campaignId)}`, selector: '.taf-campaign-report-modal:visible' },
  { category: 'pages', id: 'campaign-detail-impact-account', path: `/campaigns/${encodeURIComponent(campaignId)}?impact=account`, selector: '[data-page-id="campaign-detail-impact-account"]', modalRoi: true },
  { category: 'pages', id: 'campaign-detail-impact-service', path: `/campaigns/${encodeURIComponent(campaignId)}?impact=service`, selector: '[data-page-id="campaign-detail-impact-service"]', modalRoi: true },
  { category: 'pages', id: 'campaign-detail-impact-department', path: `/campaigns/${encodeURIComponent(campaignId)}?impact=department`, selector: '[data-page-id="campaign-detail-impact-department"]', modalRoi: true },
  { category: 'pages', id: 'campaign-detail-impact-campus', path: `/campaigns/${encodeURIComponent(campaignId)}?impact=campus`, selector: '[data-page-id="campaign-detail-impact-campus"]', modalRoi: true },
  { category: 'pages', id: 'campaign-detail-impact-business-system', path: `/campaigns/${encodeURIComponent(campaignId)}?impact=business-system`, selector: '[data-page-id="campaign-detail-impact-business-system"]', modalRoi: true },
  { category: 'pages', id: 'attack-chains', path: `/attack-chains?chain=${encodeURIComponent(campaignId)}`, selector: '.taf-attack-chain' },
  { category: 'overlays', id: 'drawer-attack-chain-detail', path: `/attack-chains?chain=${encodeURIComponent(campaignId)}`, selector: '.taf-attack-chain-detail-drawer:visible' },
];

const results = [];
for (const target of targets) {
  const page = await context.newPage();
  await page.setViewportSize({ width: 1920, height: 1080 });
  const errors = { responses: [], console: [], page: [], requests: [] };
  page.on('response', (response) => {
    if (response.status() >= 400 && response.url().startsWith(baseUrl)) errors.responses.push({ status: response.status(), url: response.url() });
  });
  page.on('console', (entry) => {
    if (entry.type() === 'error' && !entry.text().includes('ERR_CONNECTION_CLOSED')) errors.console.push(entry.text());
  });
  page.on('pageerror', (error) => {
    if (error.message !== 'Object' && !error.message.includes("reading 'disconnect'")) errors.page.push(error.message);
  });
  page.on('requestfailed', (request) => {
    if (!request.url().startsWith('https://api.yhchj.com/')) errors.requests.push({ url: request.url(), error: request.failure()?.errorText });
  });

  const url = new URL(target.path, baseUrl);
  url.searchParams.set('__codex_ui_breakdown_production', '1');
  if (target.category === 'overlays') url.searchParams.set('__codex_page_id', target.id);
  if (target.id.startsWith('campaign-detail-impact-')) url.searchParams.set('__codex_page_id', target.id);
  url.searchParams.set('captureTs', String(Date.now()));
  await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator(target.selector).waitFor({ state: 'visible', timeout: 20_000 });
  if (injectedStylePath) await page.addStyleTag({ path: injectedStylePath });
  if (target.id === 'attack-chains' || target.id === 'drawer-attack-chain-detail') {
    await page.locator('.taf-attack-canvas:not(.is-empty)').waitFor({ state: 'visible', timeout: 20_000 });
  }
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  await page.waitForTimeout(500);
  const implementation = path.join(root, 'evidence/ui-image-breakdowns', target.category, target.id, 'implementation.png');
  fs.mkdirSync(path.dirname(implementation), { recursive: true });
  await page.screenshot({ path: implementation, fullPage: false });
  let implementationRoi = null;
  let roiDimensions = null;
  if (target.modalRoi) {
    const focus = page.locator(target.selector);
    await focus.evaluate((element) => {
      element.style.height = 'auto';
      element.style.maxHeight = 'none';
      let ancestor = element.parentElement;
      while (ancestor && ancestor !== document.body) {
        ancestor.style.maxHeight = 'none';
        ancestor.style.overflow = 'visible';
        ancestor = ancestor.parentElement;
      }
    });
    await page.waitForTimeout(100);
    const box = await focus.boundingBox();
    if (!box) throw new Error(`Modal ROI ${target.selector} has no bounding box`);
    const clip = {
      x: Math.max(0, box.x),
      y: Math.max(0, box.y),
      width: Math.min(box.width, 1920 - Math.max(0, box.x)),
      height: Math.min(box.height, 1080 - Math.max(0, box.y)),
    };
    implementationRoi = path.join(root, 'evidence/ui-image-breakdowns', target.category, target.id, 'implementation-roi.png');
    await page.screenshot({ path: implementationRoi, clip });
    roiDimensions = {
      x: Math.round(clip.x),
      y: Math.round(clip.y),
      width: Math.round(clip.width),
      height: Math.round(clip.height),
    };
  }
  const dimensions = await page.evaluate(() => ({
    width: window.innerWidth,
    height: window.innerHeight,
    horizontalOverflow: document.documentElement.scrollWidth > window.innerWidth + 2,
  }));
  const status = Object.values(errors).every((items) => items.length === 0)
    && dimensions.width === 1920
    && dimensions.height === 1080
    && !dimensions.horizontalOverflow ? 'pass' : 'fail';
  results.push({
    id: target.id,
    status,
    route: url.pathname + url.search,
    implementation: path.relative(root, implementation),
    implementation_roi: implementationRoi ? path.relative(root, implementationRoi) : null,
    roi_dimensions: roiDimensions,
    dimensions,
    errors,
  });
  await page.close();
}

const report = {
  revision,
  generated_at: new Date().toISOString(),
  browser_backend: 'Windows Chrome CDP over Xshell 9224',
  browser: version.Browser,
  campaign_id: campaignId,
  result: results.every((target) => target.status === 'pass') ? 'pass' : 'fail',
  targets: results,
};
fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify({ report: path.relative(root, reportPath), result: report.result, targets: results.length }, null, 2));
process.exit(report.result === 'pass' ? 0 : 1);

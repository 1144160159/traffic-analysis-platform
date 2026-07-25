#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const cdpUrl = process.env.UI_CDP_URL ?? 'http://127.0.0.1:9224';
const outputPath = path.join(root, 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-viewport-restore-latest.json');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const originalPages = context.pages();
const productPages = originalPages.filter((page) => {
  try {
    return new URL(page.url()).origin === 'http://10.0.5.8:30180';
  } catch {
    return false;
  }
});
const baselinePage = productPages.find((page) => {
  try {
    return new URL(page.url()).pathname === '/login';
  } catch {
    return false;
  }
});
const baseline = await baselinePage?.evaluate(() => ({
  inner_width: innerWidth,
  inner_height: innerHeight,
  device_pixel_ratio: devicePixelRatio,
})).catch(() => null);
if (!baseline) throw new Error('No non-emulated Windows Chrome product tab is available as the real-window baseline');

const restoredPage = await context.newPage();
await restoredPage.goto('http://10.0.5.8:30180/campaigns', { waitUntil: 'domcontentloaded', timeout: 45_000 });
await restoredPage.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
await restoredPage.locator('.taf-campaign-workbench').waitFor({ state: 'visible', timeout: 20_000 });
const restoredViewport = await restoredPage.evaluate(() => ({
  inner_width: innerWidth,
  inner_height: innerHeight,
  device_pixel_ratio: devicePixelRatio,
  document_client_width: document.documentElement.clientWidth,
  document_scroll_width: document.documentElement.scrollWidth,
  rail_right: document.querySelector('.taf-campaign-rail')?.getBoundingClientRect().right ?? 0,
}));

const closedPages = [];
for (const page of productPages) {
  let pathname = '';
  try {
    pathname = new URL(page.url()).pathname;
  } catch {
    continue;
  }
  if (!pathname.startsWith('/campaigns')) continue;
  closedPages.push(page.url());
  await page.close().catch(() => {});
}
await restoredPage.bringToFront();

const result = {
  result: restoredViewport.inner_width === baseline.inner_width
    && restoredViewport.inner_height === baseline.inner_height
    && restoredViewport.document_scroll_width === restoredViewport.document_client_width
    && restoredViewport.rail_right <= restoredViewport.inner_width
    ? 'pass'
    : 'fail',
  browser_backend: 'Windows Chrome CDP over Xshell 9224',
  baseline,
  restored_viewport: restoredViewport,
  closed_emulated_campaign_tabs: closedPages,
  restored_url: restoredPage.url(),
  timestamp: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
process.exit(result.result === 'pass' ? 0 : 1);

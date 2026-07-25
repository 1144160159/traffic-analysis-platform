#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const cdpUrl = process.env.UI_CDP_URL ?? 'http://127.0.0.1:9224';
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/campaigns/rail-current-diagnostic.json');
const screenshotPath = path.join(root, 'evidence/ui-image-breakdowns/pages/campaigns/rail-current-diagnostic.png');
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const pages = context.pages();
const page = [...pages].reverse().find((candidate) => {
  try {
    return new URL(candidate.url()).pathname === '/campaigns';
  } catch {
    return false;
  }
});

if (!page) throw new Error('No current Windows Chrome campaign page is available for rail diagnostics');

await page.bringToFront();
await page.locator('.taf-campaign-rail').waitFor({ state: 'visible', timeout: 15_000 });
const metrics = await page.evaluate(() => {
  const rect = (element) => {
    const box = element?.getBoundingClientRect();
    return box ? {
      x: box.x,
      y: box.y,
      width: box.width,
      height: box.height,
      right: box.right,
      bottom: box.bottom,
    } : null;
  };
  const rail = document.querySelector('.taf-campaign-rail');
  const panels = [...document.querySelectorAll('.taf-campaign-rail > .taf-panel')];
  const overflows = rail
    ? [...rail.querySelectorAll('*')]
      .filter((element) => element.scrollWidth > element.clientWidth + 1)
      .map((element) => ({
        tag: element.tagName,
        class_name: typeof element.className === 'string' ? element.className : '',
        text: element.textContent?.trim().replace(/\s+/g, ' ').slice(0, 120) ?? '',
        client_width: element.clientWidth,
        scroll_width: element.scrollWidth,
        overflow_x: getComputedStyle(element).overflowX,
        rect: rect(element),
      }))
    : [];
  return {
    url: location.href,
    viewport: {
      inner_width: innerWidth,
      inner_height: innerHeight,
      outer_width: outerWidth,
      outer_height: outerHeight,
      device_pixel_ratio: devicePixelRatio,
      visual_width: visualViewport?.width ?? 0,
      visual_height: visualViewport?.height ?? 0,
      visual_scale: visualViewport?.scale ?? 0,
    },
    document: {
      client_width: document.documentElement.clientWidth,
      scroll_width: document.documentElement.scrollWidth,
      body_client_width: document.body.clientWidth,
      body_scroll_width: document.body.scrollWidth,
    },
    shell: rect(document.querySelector('.taf-shell')),
    main: rect(document.querySelector('.taf-main')),
    campaign: rect(document.querySelector('.taf-campaign-workbench')),
    rail: {
      rect: rect(rail),
      client_width: rail?.clientWidth ?? 0,
      scroll_width: rail?.scrollWidth ?? 0,
      overflow_x: rail ? getComputedStyle(rail).overflowX : '',
      display: rail ? getComputedStyle(rail).display : '',
      grid_columns: rail ? getComputedStyle(rail).gridTemplateColumns : '',
    },
    panels: panels.map((panel) => ({
      class_name: panel.className,
      rect: rect(panel),
      client_width: panel.clientWidth,
      scroll_width: panel.scrollWidth,
      body_client_width: panel.querySelector('.taf-panel__body')?.clientWidth ?? 0,
      body_scroll_width: panel.querySelector('.taf-panel__body')?.scrollWidth ?? 0,
    })),
    overflow_count: overflows.length,
    overflows,
  };
});

await page.screenshot({ path: screenshotPath, fullPage: false });
const result = {
  browser_backend: 'Windows Chrome CDP over Xshell 9224',
  metrics,
  screenshot: path.relative(root, screenshotPath),
  timestamp: new Date().toISOString(),
};
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));

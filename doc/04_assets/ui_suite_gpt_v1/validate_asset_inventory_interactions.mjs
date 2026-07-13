#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '../../..');
const requireFromUi = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = requireFromUi('@playwright/test');

const args = Object.fromEntries(process.argv.slice(2).map((value, index, all) => {
  if (!value.startsWith('--')) return [];
  const key = value.slice(2).replace(/-([a-z])/g, (_, char) => char.toUpperCase());
  return [key, all[index + 1]?.startsWith('--') ? true : all[index + 1]];
}).filter((entry) => entry.length));
const baseUrl = String(args.baseUrl || 'http://10.0.5.8:4202');
const cdpUrl = String(args.cdpUrl || 'http://127.0.0.1:9224');
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/assets-interaction');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
process.env.NO_PROXY = process.env.NO_PROXY || '127.0.0.1,localhost,10.0.5.8';

const checks = [];
const check = (name, actual, expected) => {
  const pass = typeof expected === 'function' ? expected(actual) : actual === expected;
  checks.push({ name, pass, actual, expected: typeof expected === 'function' ? 'predicate' : expected });
  if (!pass) throw new Error(`${name}: expected ${String(expected)}, received ${String(actual)}`);
};

const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0];
const page = await context.newPage();

try {
  await page.goto(`${baseUrl}/assets?tab=endpoint&assetId=PC-0082`, { waitUntil: 'networkidle' });
  await page.locator('.taf-asset-inventory').waitFor();
  check('endpoint URL state', page.url(), (value) => value.includes('tab=endpoint') && value.includes('assetId=PC-0082'));
  check('endpoint selected row', await page.locator('.ant-table-row-selected').innerText(), (value) => value.includes('PC-0082') && value.includes('实验楼-PC-0082'));
  check('endpoint right-rail identity', await page.locator('.taf-asset-detail-head strong').innerText(), '实验楼-PC-0082');
  check('endpoint has no server detail tabs', await page.locator('.taf-asset-detail-tabs').count(), 0);
  check('endpoint has no server detail action', await page.locator('.taf-asset-action-rail button').filter({ hasText: '进入资产详情' }).count(), 0);

  await page.locator('.taf-asset-tabs').getByRole('tab', { name: '服务器', exact: true }).click();
  await page.waitForURL((url) => url.searchParams.get('tab') === 'server' && url.searchParams.get('assetId') === 'SRV-0007');
  check('server selected row', await page.locator('.taf-asset-dense__row.is-selected').innerText(), (value) => value.includes('SRV-0007') && value.includes('实验楼-SRV-12'));
  check('server right-rail identity', await page.locator('.taf-asset-detail-head strong').innerText(), '实验楼-SRV-12');
  check('server exposes detail tabs', await page.locator('.taf-asset-detail-tabs').count(), 1);

  await page.locator('.taf-asset-action-rail button').filter({ hasText: '进入资产详情' }).click();
  await page.waitForURL((url) => url.searchParams.get('detail') === 'basic');
  await page.locator('.taf-asset-detail-drawer').waitFor({ state: 'visible' });
  check('drawer preserves host workbench', await page.locator('.taf-asset-main').count(), 1);
  check('drawer identity', await page.locator('.taf-asset-detail-drawer .taf-asset-detail-workspace__identity strong').innerText(), '实验楼-SRV-12');

  await page.locator('.taf-asset-detail-drawer').getByRole('tab', { name: '开放服务', exact: true }).click();
  await page.waitForURL((url) => url.searchParams.get('tab') === 'server'
    && url.searchParams.get('assetId') === 'SRV-0007'
    && url.searchParams.get('detail') === 'open-services');
  check('detail transition preserves server identity', await page.locator('.taf-asset-detail-drawer .taf-asset-detail-workspace__identity').innerText(), (value) => value.includes('SRV-0007') && value.includes('实验楼-SRV-12'));

  fs.mkdirSync(evidenceDir, { recursive: true });
  await page.screenshot({ path: path.join(evidenceDir, 'open-services-drawer.png'), fullPage: false });
  await page.getByRole('button', { name: '关闭详情', exact: true }).click();
  await page.waitForURL((url) => url.searchParams.get('tab') === 'server'
    && url.searchParams.get('assetId') === 'SRV-0007'
    && !url.searchParams.has('detail'));
  await page.locator('.taf-asset-detail-drawer').waitFor({ state: 'hidden' });
  check('drawer closes without losing selection', await page.locator('.taf-asset-detail-drawer:visible').count(), 0);

  const classifications = [
    ['终端', 'endpoint', 'PC-0082'],
    ['网络设备', 'network-device', 'NET-0001'],
    ['业务系统', 'business-system', 'BIZ-0001'],
    ['未知资产', 'unknown', 'UNK-10.12.88.45'],
  ];
  for (const [label, tab, assetId] of classifications) {
    await page.locator('.taf-asset-tabs').getByRole('tab', { name: label, exact: true }).click();
    await page.waitForURL((url) => url.searchParams.get('tab') === tab && url.searchParams.get('assetId') === assetId);
    check(`${label} does not expose server detail tabs`, await page.locator('.taf-asset-detail-tabs').count(), 0);
    check(`${label} does not open a detail drawer`, await page.locator('.taf-asset-detail-drawer:visible').count(), 0);
  }

  const report = {
    status: 'pass',
    checked_at: new Date().toISOString(),
    base_url: baseUrl,
    cdp_url: cdpUrl,
    checks,
    screenshot: 'evidence/ui-image-breakdowns/pages/assets-interaction/open-services-drawer.png',
  };
  fs.writeFileSync(path.join(evidenceDir, 'interaction-report.json'), `${JSON.stringify(report, null, 2)}\n`);
  console.log(JSON.stringify(report, null, 2));
} catch (error) {
  fs.mkdirSync(evidenceDir, { recursive: true });
  const report = {
    status: 'fail',
    checked_at: new Date().toISOString(),
    base_url: baseUrl,
    cdp_url: cdpUrl,
    checks,
    error: error instanceof Error ? error.message : String(error),
  };
  fs.writeFileSync(path.join(evidenceDir, 'interaction-report.json'), `${JSON.stringify(report, null, 2)}\n`);
  console.error(JSON.stringify(report, null, 2));
  process.exitCode = 1;
} finally {
  await page.close();
  await browser.close();
}

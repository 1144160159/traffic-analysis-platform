#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

const root = process.cwd();
const require = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = require('@playwright/test');
const baseUrl = process.env.UI_BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.UI_CDP_URL || 'http://127.0.0.1:9224';
const outputPath = path.join(root, 'doc/02_acceptance/02-regression/threat-navigation-stability-r531.json');
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/navigation/threat-stability-r531');

const token = () => {
  const encodedSecret = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], {
    encoding: 'utf8', env: process.env, timeout: 15_000,
  });
  const secret = Buffer.from(encodedSecret, 'base64').toString('utf8');
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(),
    tenant_id: 'default', username: 'codex-windows-cdp-navigation', roles: ['admin'], permissions: ['*', 'admin:*'],
    token_type: 'access', session_id: `navigation-${crypto.randomUUID()}`, iat: now, exp: now + 1800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
};

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
page.setDefaultTimeout(12_000);
fs.mkdirSync(evidenceDir, { recursive: true });

const startUrl = new URL(`/alerts?navigationStabilityTs=${Date.now()}`, baseUrl);
startUrl.hash = `codex_smoke_token=${token()}`;
await page.goto(startUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-sidebar').waitFor({ state: 'visible' });

const states = [];
const capture = async (id) => {
  await page.waitForTimeout(400);
  const metrics = await page.evaluate(() => {
    const sidebar = document.querySelector('.taf-sidebar');
    const main = document.querySelector('.taf-main');
    const threat = [...document.querySelectorAll('.taf-sidebar__nav section')].find((section) => section.querySelector('.taf-sidebar__group')?.textContent?.includes('威胁分析'));
    const active = threat?.querySelector('.taf-sidebar__item.is-active');
    return {
      path: location.pathname,
      shell_classes: document.querySelector('.taf-shell')?.className ?? '',
      sidebar_width: sidebar?.getBoundingClientRect().width ?? 0,
      main_left: main?.getBoundingClientRect().left ?? 0,
      threat_group_expanded: threat?.querySelector('.taf-sidebar__group')?.getAttribute('aria-expanded') === 'true',
      threat_items: [...(threat?.querySelectorAll('.taf-sidebar__item') ?? [])].map((item) => item.textContent?.replace(/\s+/g, ' ').trim()),
      active_item: active?.textContent?.replace(/\s+/g, ' ').trim() ?? '',
      active_item_height: active?.getBoundingClientRect().height ?? 0,
      horizontal_overflow: document.documentElement.scrollWidth > document.documentElement.clientWidth,
    };
  });
  const screenshot = path.join(evidenceDir, `${id}-1920x1080.png`);
  await page.screenshot({ path: screenshot, fullPage: false });
  states.push({ id, ...metrics, screenshot: path.relative(root, screenshot) });
};

await capture('alerts');
await page.locator('.taf-sidebar__item', { hasText: '战役列表' }).click();
await page.waitForURL(/\/campaigns(?:\?|$)/);
await capture('campaigns');
await page.locator('.taf-sidebar__item', { hasText: '攻击链分析' }).click();
await page.waitForURL(/\/attack-chains(?:\?|$)/);
await capture('attack-chains');
await page.locator('.taf-sidebar__item', { hasText: '告警中心' }).click();
await page.waitForURL(/\/alerts(?:\?|$)/);
await capture('alerts-return');

const widths = new Set(states.map((state) => state.sidebar_width));
const mainLefts = new Set(states.map((state) => state.main_left));
const itemHeights = new Set(states.map((state) => state.active_item_height));
const menuShapes = new Set(states.map((state) => JSON.stringify(state.threat_items)));
const checks = {
  sidebar_width_stable: widths.size === 1 && [...widths][0] === 166,
  main_left_stable: mainLefts.size === 1 && [...mainLefts][0] === 166,
  active_item_height_stable: itemHeights.size === 1,
  threat_menu_shape_stable: menuShapes.size === 1,
  threat_group_always_expanded: states.every((state) => state.threat_group_expanded),
  route_specific_shell_removed: states.every((state) => !state.shell_classes.includes('taf-shell--ui-redevelopment')),
  no_horizontal_overflow: states.every((state) => !state.horizontal_overflow),
};
const result = {
  result: Object.values(checks).every(Boolean) ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP over Xshell 9224',
  browser: version.Browser,
  checks,
  states,
  generated_at: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`, 'utf8');
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);

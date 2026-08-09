#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');

function parseArgs(argv) {
  const result = {};
  for (let index = 0; index < argv.length; index += 1) {
    const key = argv[index];
    if (!key.startsWith('--') || index + 1 >= argv.length) throw new Error(`invalid argument: ${key}`);
    result[key.slice(2).replace(/-([a-z])/gu, (_, letter) => letter.toUpperCase())] = argv[index + 1];
    index += 1;
  }
  return result;
}

const args = {
  baseUrl: 'http://10.0.5.8:30186',
  cdpUrl: 'http://127.0.0.1:9224',
  runId: '',
  evidenceDir: '',
  outputJson: '',
  ...parseArgs(process.argv.slice(2)),
};
if (!args.runId || !args.evidenceDir || !args.outputJson) throw new Error('--run-id, --evidence-dir and --output-json are required');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = process.env.NO_PROXY || '127.0.0.1,localhost,10.0.5.8';

const evidenceDirectory = path.resolve(root, args.evidenceDir);
const outputPath = path.resolve(root, args.outputJson);
if (fs.existsSync(evidenceDirectory) || fs.existsSync(outputPath)) throw new Error('refusing to overwrite immutable Product Design evidence');
fs.mkdirSync(evidenceDirectory, { recursive: true });

function makeSmokeToken() {
  const encoded = execFileSync('kubectl', [
    '-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials',
    '-o', 'jsonpath={.data.JWT_SECRET}',
  ], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-product-design-dashboard-auditor',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'dashboard:read'],
    token_type: 'access',
    session_id: `dashboard-product-design-${args.runId}`,
    iat: now,
    exp: now + 3600,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

function sha256(file) {
  return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

const versionResponse = await fetch(`${args.cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(args.cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const badResponses = [];
const requestFailures = [];
const consoleErrors = [];
const pageErrors = [];
const mutationRequests = [];
page.on('request', (request) => {
  if (request.url().startsWith(`${args.baseUrl}/api/`) && !['GET', 'HEAD', 'OPTIONS'].includes(request.method())) {
    mutationRequests.push({ method: request.method(), url: request.url() });
  }
});
page.on('response', (response) => {
  if (response.url().startsWith(`${args.baseUrl}/api/`) && response.status() >= 400) {
    badResponses.push({ method: response.request().method(), status: response.status(), url: response.url() });
  }
});
page.on('requestfailed', (request) => {
  if (request.url().startsWith(args.baseUrl)) requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? '' });
});
page.on('console', (entry) => {
  if (entry.type() === 'error' && entry.location().url?.startsWith(args.baseUrl)) consoleErrors.push({ text: entry.text(), location: entry.location() });
});
page.on('pageerror', (error) => pageErrors.push({ message: error.message }));

const token = makeSmokeToken();
const widths = [1920, 1600, 1440];
const observations = [];

try {
  for (const width of widths) {
    await page.setViewportSize({ width, height: 1080 });
    const url = new URL(`/dashboard?productDesignAuditTs=${Date.now()}`, args.baseUrl);
    url.hash = `codex_smoke_token=${token}`;
    await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
    await page.locator('.taf-dashboard-workbench').waitFor({ state: 'visible', timeout: 20_000 });
    await page.locator('.taf-dashboard-kpi').first().waitFor({ state: 'visible', timeout: 20_000 });
    await page.locator('.taf-deficit-item').first().waitFor({ state: 'visible', timeout: 20_000 });
    await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => {});
    await page.waitForTimeout(400);

    const geometry = await page.locator('.taf-dashboard-workbench').evaluate((host) => {
      const visible = (element) => {
        const rect = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        return rect.width > 0 && rect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden';
      };
      const contained = (inner, outer) => inner.left >= outer.left - 1 && inner.top >= outer.top - 1
        && inner.right <= outer.right + 1 && inner.bottom <= outer.bottom + 1;
      const overlaps = (left, right) => left.left < right.right - 1 && left.right > right.left + 1
        && left.top < right.bottom - 1 && left.bottom > right.top + 1;
      const rootElement = document.scrollingElement ?? document.documentElement;
      const kpis = [...host.querySelectorAll('.taf-dashboard-kpi')].filter(visible);
      const kpiValues = kpis.map((card) => {
        const value = card.querySelector(':scope > strong');
        if (!(value instanceof HTMLElement)) return null;
        const cardRect = card.getBoundingClientRect();
        const valueRect = value.getBoundingClientRect();
        return {
          display: value.textContent?.trim() ?? '',
          full_value: value.getAttribute('title') ?? '',
          contained: contained(valueRect, cardRect),
          overflow: value.scrollWidth > value.clientWidth + 1,
        };
      }).filter(Boolean);
      const deficitList = host.querySelector('.taf-deficit-list');
      const deficitListRect = deficitList?.getBoundingClientRect();
      const deficits = [...host.querySelectorAll('.taf-deficit-item')].filter(visible).map((item) => {
        const itemRect = item.getBoundingClientRect();
        const label = item.querySelector(':scope > span:not(.taf-deficit-item__icon)');
        const value = item.querySelector(':scope > strong');
        const context = item.querySelector(':scope > em');
        const button = item.querySelector(':scope > button');
        const labelRect = label?.getBoundingClientRect();
        const valueRect = value?.getBoundingClientRect();
        const contextRect = context?.getBoundingClientRect();
        const buttonRect = button?.getBoundingClientRect();
        return {
          label: label?.textContent?.trim() ?? '',
          value: value?.textContent?.trim() ?? '',
          full_value: value?.getAttribute('title') ?? '',
          context: context?.textContent?.trim() ?? '',
          full_context: context?.getAttribute('title') ?? '',
          item_contained: Boolean(deficitListRect && contained(itemRect, deficitListRect)),
          value_overflow: value instanceof HTMLElement && value.scrollWidth > value.clientWidth + 1,
          context_overflow: context instanceof HTMLElement && context.scrollWidth > context.clientWidth + 1,
          action_disabled: button instanceof HTMLButtonElement ? button.disabled : null,
          action_size: buttonRect ? { width: Math.round(buttonRect.width), height: Math.round(buttonRect.height) } : null,
          action_overlap: Boolean(buttonRect && [labelRect, valueRect, contextRect].filter(Boolean).some((rect) => overlaps(buttonRect, rect))),
        };
      });
      const stageGrid = host.querySelector('.taf-stage-basket__cards');
      const stageColumns = stageGrid ? getComputedStyle(stageGrid).gridTemplateColumns.split(' ').filter(Boolean).length : 0;
      const stageCards = [...host.querySelectorAll('.taf-stage-card')].filter(visible).map((card) => {
        const value = card.querySelector(':scope > strong');
        const cardRect = card.getBoundingClientRect();
        const valueRect = value?.getBoundingClientRect();
        return {
          value: value?.textContent?.trim() ?? '',
          full_value: value?.getAttribute('title') ?? '',
          value_contained: Boolean(valueRect && contained(valueRect, cardRect)),
          value_overflow: value instanceof HTMLElement && value.scrollWidth > value.clientWidth + 1,
        };
      });
      return {
        page_horizontal_overflow: rootElement.scrollWidth > rootElement.clientWidth + 1,
        kpi_count: kpis.length,
        kpi_values: kpiValues,
        million_kpi_count: kpiValues.filter((item) => /\d{1,3}(?:,\d{3}){2,}/u.test(item.full_value)).length,
        compact_kpi_count: kpiValues.filter((item) => /[万亿]/u.test(item.display)).length,
        deficit_list_overflow: deficitList instanceof HTMLElement && deficitList.scrollHeight > deficitList.clientHeight + 1,
        deficits,
        disabled_action_count: deficits.filter((item) => item.action_disabled === true).length,
        enabled_action_count: deficits.filter((item) => item.action_disabled === false).length,
        stage_count: stageCards.length,
        stage_columns: stageColumns,
        stage_cards: stageCards,
      };
    });

    const partialText = (await page.locator('.ant-alert-warning').filter({ hasText: '统一快照部分可用' }).first().innerText()).trim();
    const screenshot = path.join(evidenceDirectory, `dashboard-${width}x1080.png`);
    await page.screenshot({ path: screenshot, fullPage: false, timeout: 60_000 });
    const deficitsPassed = geometry.deficits.length === 6
      && !geometry.deficit_list_overflow
      && geometry.deficits.every((item) => item.item_contained && !item.value_overflow && !item.context_overflow
        && !item.action_overlap && item.action_size?.width >= 24 && item.action_size?.height >= 24)
      && geometry.disabled_action_count > 0
      && geometry.enabled_action_count > 0;
    const passed = !geometry.page_horizontal_overflow
      && geometry.kpi_count >= 8
      && geometry.million_kpi_count >= 2
      && geometry.compact_kpi_count >= geometry.million_kpi_count
      && geometry.kpi_values.every((item) => item.contained && !item.overflow)
      && deficitsPassed
      && geometry.stage_count > 0
      && geometry.stage_columns === geometry.stage_count
      && geometry.stage_cards.every((item) => item.value_contained && !item.value_overflow)
      && partialText.includes('reconciliation.alerts_projection');
    observations.push({
      width,
      height: 1080,
      screenshot: path.relative(root, screenshot),
      screenshot_sha256: sha256(screenshot),
      reference_image: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/dashboard.png',
      reference_sha256: sha256(path.resolve(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/dashboard.png')),
      partial_text: partialText,
      geometry,
      passed,
    });
  }
} finally {
  await page.setViewportSize({ width: 1920, height: 1080 }).catch(() => {});
  await page.close().catch(() => {});
  await browser.close().catch(() => {});
}

const result = observations.length === widths.length
  && observations.every((observation) => observation.passed)
  && mutationRequests.length === 0
  && badResponses.length === 0
  && requestFailures.length === 0
  && consoleErrors.length === 0
  && pageErrors.length === 0 ? 'PASS' : 'FAIL';
const report = {
  schema_version: 1,
  run_id: args.runId,
  gate: 'PRODUCT_DESIGN_DASHBOARD_READONLY_AUDIT',
  result,
  generated_at: new Date().toISOString(),
  browser: version.Browser,
  browser_path: 'Windows Chrome via Xshell/CDP 127.0.0.1:9224',
  base_url: args.baseUrl,
  read_only: true,
  production_mutations: mutationRequests,
  bad_responses: badResponses,
  request_failures: requestFailures,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  observations,
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify({ result, output: path.relative(root, outputPath), observations: observations.map(({ width, passed }) => ({ width, passed })) }, null, 2));
process.exit(result === 'PASS' ? 0 : 1);

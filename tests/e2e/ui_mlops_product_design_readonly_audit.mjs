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
  baseUrl: 'http://10.0.5.8:30185',
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
    username: 'codex-product-design-mlops-auditor',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'model:read'],
    token_type: 'access',
    session_id: `mlops-product-design-${args.runId}`,
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
    const url = new URL(`/mlops?productDesignAuditTs=${Date.now()}`, args.baseUrl);
    url.hash = `codex_smoke_token=${token}`;
    await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
    await page.locator('.taf-mlops').waitFor({ state: 'visible', timeout: 20_000 });
    await page.locator('.taf-mlops-bottom .ant-table-tbody tr:not(.ant-table-measure-row)').first().waitFor({ state: 'visible', timeout: 20_000 });
    await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => {});
    await page.waitForTimeout(400);

    const table = page.locator('.taf-mlops-bottom .ant-table-wrapper').last();
    const scrollHost = table.locator('.ant-table-body, .ant-table-content').first();
    await scrollHost.evaluate((element) => { element.scrollLeft = element.scrollWidth; element.dispatchEvent(new Event('scroll')); });
    await page.waitForTimeout(250);

    const geometry = await table.evaluate((host) => {
      const visible = (element) => {
        const rect = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        return rect.width > 0 && rect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden';
      };
      const parseAlpha = (color) => {
        const rgba = color.match(/^rgba?\([^,]+,[^,]+,[^,]+(?:,\s*([0-9.]+))?\)$/u);
        return rgba ? Number(rgba[1] ?? 1) : 0;
      };
      const fixedCells = [...host.querySelectorAll('.ant-table-cell-fix-right')].filter(visible);
      const bodyFixedCells = fixedCells.filter((cell) => cell.tagName === 'TD');
      const fixedCellDetails = fixedCells.map((cell) => {
        const style = getComputedStyle(cell);
        const rect = cell.getBoundingClientRect();
        const top = document.elementFromPoint(Math.max(rect.left + 2, rect.right - 18), rect.top + rect.height / 2);
        return {
          tag: cell.tagName,
          background: style.backgroundColor,
          alpha: parseAlpha(style.backgroundColor),
          position: style.position,
          right: style.right,
          z_index: Number(style.zIndex),
          topmost_cell_is_fixed: Boolean(top?.closest('.ant-table-cell-fix-right') === cell),
        };
      });
      const actionGroups = [...host.querySelectorAll('td.ant-table-cell-fix-right .taf-mlops-row-actions')].filter(visible);
      const actions = actionGroups.map((group) => {
        const groupRect = group.getBoundingClientRect();
        const buttons = [...group.querySelectorAll('button')];
        const buttonDetails = buttons.map((button) => {
          const rect = button.getBoundingClientRect();
          return {
            title: button.getAttribute('title'),
            disabled: button.disabled,
            width: Math.round(rect.width),
            height: Math.round(rect.height),
            contained: rect.left >= groupRect.left - 1 && rect.right <= groupRect.right + 1,
          };
        });
        return {
          row_text: group.closest('tr')?.textContent?.replace(/\s+/gu, ' ').trim() ?? '',
          buttons: buttonDetails,
          passed: buttonDetails.length === 3
            && buttonDetails.map((button) => button.title).join('|') === '查看任务|停止任务|重试任务'
            && buttonDetails.every((button) => button.width >= 24 && button.height >= 24 && button.contained)
            && buttonDetails[0].disabled === false,
        };
      });
      const root = document.scrollingElement ?? document.documentElement;
      return {
        page_horizontal_overflow: root.scrollWidth > root.clientWidth + 1,
        scroll_left: host.querySelector('.ant-table-body, .ant-table-content')?.scrollLeft ?? 0,
        scroll_width: host.querySelector('.ant-table-body, .ant-table-content')?.scrollWidth ?? 0,
        client_width: host.querySelector('.ant-table-body, .ant-table-content')?.clientWidth ?? 0,
        fixed_cell_details: fixedCellDetails,
        fixed_cells_opaque: fixedCellDetails.length > 0 && fixedCellDetails.every((cell) => cell.alpha === 1),
        fixed_cells_sticky: fixedCellDetails.length > 0 && fixedCellDetails.every((cell) => cell.position === 'sticky' && cell.right === '0px' && cell.z_index > 0),
        fixed_cells_topmost: fixedCellDetails.length > 0 && fixedCellDetails.every((cell) => cell.topmost_cell_is_fixed),
        action_groups: actions,
        action_groups_passed: actions.length > 0 && actions.every((action) => action.passed),
        distinct_disabled_combinations: [...new Set(actions.map((action) => action.buttons.map((button) => button.disabled ? '1' : '0').join('')))],
        retry_enabled_rows: actions.filter((action) => action.buttons[2]?.disabled === false).length,
        stop_enabled_rows: actions.filter((action) => action.buttons[1]?.disabled === false).length,
        body_fixed_cell_count: bodyFixedCells.length,
      };
    });

    const firstCheckbox = table.locator('input[type="checkbox"]').first();
    await firstCheckbox.focus();
    let focusedAction = null;
    for (let index = 0; index < 120; index += 1) {
      await page.keyboard.press('Tab');
      focusedAction = await page.evaluate(() => {
        const active = document.activeElement;
        if (!(active instanceof HTMLButtonElement) || !active.closest('.taf-mlops-row-actions')) return null;
        const style = getComputedStyle(active);
        return {
          title: active.getAttribute('title'),
          outline_style: style.outlineStyle,
          outline_width: style.outlineWidth,
          outline_color: style.outlineColor,
        };
      });
      if (focusedAction) break;
    }
    const focusPassed = Boolean(focusedAction
      && focusedAction.outline_style !== 'none'
      && Number.parseFloat(focusedAction.outline_width) >= 0.5);
    const screenshot = path.join(evidenceDirectory, `mlops-${width}x1080-fixed-action-column.png`);
    await page.screenshot({ path: screenshot, fullPage: false, timeout: 60_000 });
    const horizontalScrollRequired = geometry.scroll_width > geometry.client_width + 1;
    const passed = !geometry.page_horizontal_overflow
      && (!horizontalScrollRequired || geometry.scroll_left > 0)
      && geometry.fixed_cells_opaque
      && geometry.fixed_cells_sticky
      && (!horizontalScrollRequired || geometry.fixed_cells_topmost)
      && geometry.action_groups_passed
      && geometry.distinct_disabled_combinations.length >= 2
      && geometry.retry_enabled_rows > 0
      && focusPassed;
    observations.push({
      width,
      height: 1080,
      screenshot: path.relative(root, screenshot),
      screenshot_sha256: sha256(screenshot),
      reference_image: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/mlops.png',
      reference_sha256: sha256(path.resolve(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/mlops.png')),
      horizontal_scroll_required: horizontalScrollRequired,
      geometry,
      keyboard_focus: focusedAction,
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
  && badResponses.length === 0
  && requestFailures.length === 0
  && consoleErrors.length === 0
  && pageErrors.length === 0 ? 'PASS' : 'FAIL';
const report = {
  schema_version: 1,
  run_id: args.runId,
  gate: 'PRODUCT_DESIGN_MLOPS_READONLY_AUDIT',
  result,
  generated_at: new Date().toISOString(),
  browser: version.Browser,
  browser_path: 'Windows Chrome via Xshell/CDP 127.0.0.1:9224',
  base_url: args.baseUrl,
  read_only: true,
  production_mutations: [],
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

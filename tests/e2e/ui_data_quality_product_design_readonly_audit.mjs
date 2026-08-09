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
  baseUrl: 'http://10.0.5.8:30190',
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
    username: 'codex-product-design-data-quality-auditor',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'data-quality:read'],
    token_type: 'access',
    session_id: `data-quality-product-design-${args.runId}`,
    iat: now,
    exp: now + 3600,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

function sha256(file) {
  return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

const checkTargets = {
  flow_rate: 'topic-health',
  data_completeness: 'field-quality',
  end_to_end_latency: 'flink-quality',
  schema_drift: 'field-quality',
  kafka_lag_proxy: 'topic-health',
  kafka_consumer_lag: 'topic-health',
  flink_event_time_watermark: 'flink-quality',
  sink_commit_watermark: 'storage-quality',
};
const sorted = (values) => [...values].sort((left, right) => left.localeCompare(right));

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
    const url = new URL(`/data-quality?productDesignAuditTs=${Date.now()}`, args.baseUrl);
    url.hash = `codex_smoke_token=${token}`;
    const snapshotPromise = page.waitForResponse(
      (response) => response.request().method() === 'GET' && response.url().includes('/api/v1/data-quality'),
      { timeout: 45_000 },
    );
    await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
    const snapshotResponse = await snapshotPromise;
    const snapshotPayload = await snapshotResponse.json();
    await page.locator('.taf-data-quality-shell.is-unified-tabs').waitFor({ state: 'visible', timeout: 20_000 });
    await page.locator('.taf-data-quality-rail').waitFor({ state: 'visible', timeout: 20_000 });
    await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => {});
    await page.waitForTimeout(350);

    const body = snapshotPayload?.data ?? snapshotPayload;
    const checks = Array.isArray(body?.checks) ? body.checks.map((check) => ({
      name: String(check?.name ?? ''),
      status: ['pass', 'warn', 'fail'].includes(String(check?.status)) ? String(check.status) : 'unknown',
      measured: typeof check?.measured === 'boolean' ? check.measured : true,
      source: String(check?.source ?? ''),
      message: String(check?.message ?? ''),
    })) : [];
    const expectedAnomalies = sorted(checks.filter((check) => check.status !== 'pass').map((check) => check.name));
    const expectedLocate = sorted(checks.filter((check) => checkTargets[check.name]).map((check) => check.name));
    const expectedRepair = sorted(checks.filter((check) => checkTargets[check.name] && check.measured && check.status !== 'pass').map((check) => check.name));

    const rail = page.locator('.taf-data-quality-rail');
    const anomalyPanel = rail.locator('.taf-panel').filter({ hasText: '质量异常告警（当前快照）' }).first();
    const locatePanel = rail.locator('.taf-panel').filter({ hasText: '快速定位' }).first();
    const repairPanel = rail.locator('.taf-panel').filter({ hasText: '质量修复建议' }).first();
    const anomalyRows = await anomalyPanel.locator('button[data-check-name]').evaluateAll((buttons) => buttons.map((button) => ({
      name: button.dataset.checkName ?? '',
      status: button.dataset.checkStatus ?? '',
      measured: button.dataset.checkMeasured ?? '',
      source: button.dataset.checkSource ?? '',
      title: button.getAttribute('title') ?? '',
    })));
    const locateRows = await locatePanel.locator('button[data-check-name]').evaluateAll((buttons) => buttons.map((button) => ({
      name: button.dataset.checkName ?? '',
      status: button.dataset.checkStatus ?? '',
      measured: button.dataset.checkMeasured ?? '',
      source: button.dataset.checkSource ?? '',
      title: button.getAttribute('title') ?? '',
    })));
    const repairRows = await repairPanel.locator('button[data-check-name]').evaluateAll((buttons) => buttons.map((button) => ({
      name: button.dataset.checkName ?? '',
      status: button.dataset.checkStatus ?? '',
      measured: button.dataset.checkMeasured ?? '',
      source: button.dataset.checkSource ?? '',
      title: button.getAttribute('title') ?? '',
    })));
    const geometry = await rail.evaluate((host) => {
      const rootElement = document.scrollingElement ?? document.documentElement;
      const panels = [...host.querySelectorAll(':scope > .taf-panel')];
      const repairButtons = [...panels[2]?.querySelectorAll('button[data-check-name]') ?? []];
      const repairBody = panels[2]?.querySelector('.taf-panel__body');
      const firstRepairRect = repairButtons[0]?.getBoundingClientRect();
      const repairBodyRect = repairBody?.getBoundingClientRect();
      return {
        page_horizontal_overflow: rootElement.scrollWidth > rootElement.clientWidth + 1,
        rail_horizontal_overflow: host.scrollWidth > host.clientWidth + 1,
        panel_count: panels.length,
        repair_button_count: repairButtons.length,
        first_repair_aligned_to_top: !firstRepairRect || !repairBodyRect || firstRepairRect.top <= repairBodyRect.top + 8,
        visible_buttons: [...host.querySelectorAll('button[data-check-name]')].map((button) => {
          const rect = button.getBoundingClientRect();
          return { name: button.dataset.checkName ?? '', width: Math.round(rect.width), height: Math.round(rect.height) };
        }),
      };
    });

    let drilldown = null;
    if (locateRows.length > 0) {
      const firstName = locateRows[0].name;
      const targetTab = checkTargets[firstName];
      await locatePanel.locator(`button[data-check-name="${firstName}"]`).click();
      await page.waitForURL((current) => current.searchParams.get('tab') === targetTab, { timeout: 10_000 });
      const selected = await page.locator(`button[data-tab-slug="${targetTab}"]`).getAttribute('aria-selected');
      drilldown = { check_name: firstName, expected_tab: targetTab, selected };
      await page.locator('button[data-tab-slug="overview"]').click();
      await page.waitForURL((current) => !current.searchParams.get('tab') || current.searchParams.get('tab') === 'overview', { timeout: 10_000 });
    }

    const screenshot = path.join(evidenceDirectory, `data-quality-${width}x1080-overview.png`);
    await page.screenshot({ path: screenshot, fullPage: false, timeout: 60_000 });
    const sourceContractPassed = checks.length > 0 && checks.every((check) => check.name && check.status && typeof check.measured === 'boolean' && check.source && check.message);
    const rowsPreserveContract = [...anomalyRows, ...locateRows, ...repairRows].every((row) => row.name && row.status && row.measured && row.source && row.title.includes(row.source));
    const passed = snapshotResponse.status() === 200
      && sourceContractPassed
      && JSON.stringify(sorted(anomalyRows.map((row) => row.name))) === JSON.stringify(expectedAnomalies)
      && JSON.stringify(sorted(locateRows.map((row) => row.name))) === JSON.stringify(expectedLocate)
      && JSON.stringify(sorted(repairRows.map((row) => row.name))) === JSON.stringify(expectedRepair)
      && repairRows.every((row) => row.measured === 'true' && row.status !== 'pass')
      && rowsPreserveContract
      && drilldown?.selected === 'true'
      && !geometry.page_horizontal_overflow
      && !geometry.rail_horizontal_overflow
      && geometry.panel_count === 4
      && geometry.first_repair_aligned_to_top
      && geometry.visible_buttons.every((button) => button.width >= 24 && button.height >= 24);
    observations.push({
      width,
      height: 1080,
      screenshot: path.relative(root, screenshot),
      screenshot_sha256: sha256(screenshot),
      reference_image: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality.png',
      reference_sha256: sha256(path.resolve(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality.png')),
      snapshot: {
        http_status: snapshotResponse.status(),
        contract_version: snapshotPayload?.meta?.contract_version ?? body?.meta?.contract_version ?? '',
        snapshot_id: snapshotPayload?.meta?.snapshot_id ?? body?.meta?.snapshot_id ?? '',
        partial: snapshotPayload?.meta?.partial ?? body?.meta?.partial ?? null,
        missing_sections: snapshotPayload?.meta?.missing_sections ?? body?.meta?.missing_sections ?? [],
        source_watermarks: snapshotPayload?.meta?.source_watermarks ?? body?.meta?.source_watermarks ?? {},
        checks,
      },
      expected: { anomalies: expectedAnomalies, locate: expectedLocate, repair: expectedRepair },
      rendered: { anomalies: anomalyRows, locate: locateRows, repair: repairRows },
      drilldown,
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
  gate: 'PRODUCT_DESIGN_DATA_QUALITY_READONLY_AUDIT',
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

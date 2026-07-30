#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = process.env.TRAFFIC_UI_BASE_URL || 'http://10.0.5.8:30180';
const cdpUrl = process.env.TRAFFIC_WINDOWS_CDP_URL || 'http://127.0.0.1:9224';
const alertId = process.env.TRAFFIC_ALERT_DETAIL_ID || 'alert-detail-accept-r802';
const outputPath = path.join(
  root,
  'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-alert-detail-r820-all-downloads.json',
);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const encodedSecret = execFileSync(
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
    username: 'codex-windows-cdp-admin',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:read', 'alert:write'],
    token_type: 'access',
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const signingInput = `${header}.${claims}`;
  const secret = Buffer.from(encodedSecret, 'base64').toString('utf8');
  const signature = crypto.createHmac('sha256', secret).update(signingInput).digest('base64url');
  return `${signingInput}.${signature}`;
}

const expectedFiles = {
  'c2-tunnel.pcap': { bytes: 94, contentType: 'application/vnd.tcpdump.pcap' },
  'session-primary.json': { bytes: 132, contentType: 'application/json' },
  'session-heartbeat.json': { bytes: 137, contentType: 'application/json' },
  'ids-alert-detail-r802.log': { bytes: 103, contentType: 'text/plain' },
  'path-alert-detail-r802.json': { bytes: 178, contentType: 'application/json' },
  'hash.txt': { bytes: 65, contentType: 'text/plain' },
};

const version = await (await fetch(`${cdpUrl}/json/version`)).json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
const requests = [];
const apiResponses = [];
const consoleErrors = [];
page.on('request', (request) => {
  if (request.url().includes('/api/v1/alerts/')) {
    requests.push({ method: request.method(), url: request.url(), post_data: request.postData() });
  }
});
page.on('response', (response) => {
  if (response.url().includes(`/api/v1/alerts/${encodeURIComponent(alertId)}`)) {
    apiResponses.push({ method: response.request().method(), url: response.url(), status: response.status() });
  }
});
page.on('console', (entry) => {
  if (entry.type() === 'error' && !entry.location().url.startsWith('chrome-extension://')) {
    consoleErrors.push(entry.text());
  }
});

const route = new URL(`/alerts/${encodeURIComponent(alertId)}?downloadAcceptance=${Date.now()}&evidenceView=all`, baseUrl);
route.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(route.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
const panel = page.locator('.taf-alert-detail-evidence-panel');
await panel.locator('.ant-table-tbody > tr:not(.ant-table-placeholder):not(.ant-table-measure-row)').first()
  .waitFor({ state: 'visible', timeout: 20_000 });

const runtimeConfig = await page.evaluate(() => window.__RUNTIME_CONFIG__);
const downloads = [];

async function downloadVisibleRows() {
  const buttons = panel.getByRole('button', { name: /^下载证据 / });
  const count = await buttons.count();
  for (let index = 0; index < count; index += 1) {
    const button = buttons.nth(index);
    const ariaLabel = await button.getAttribute('aria-label');
    const accessPromise = page.waitForResponse(
      (response) => response.request().method() === 'POST' && response.url().includes('/evidence/access'),
      { timeout: 30_000 },
    );
    const filePromise = page.waitForResponse(
      (response) => response.request().method() === 'GET' && response.url().includes('/download?'),
      { timeout: 60_000 },
    );
    const downloadPromise = page.waitForEvent('download', { timeout: 60_000 });
    await button.click();
    const [accessResponse, fileResponse, download] = await Promise.all([accessPromise, filePromise, downloadPromise]);
    const accessPayload = await accessResponse.json();
    const body = await fileResponse.body();
    downloads.push({
      aria_label: ariaLabel,
      evidence_id: accessPayload?.data?.evidence_id ?? '',
      access_status: accessResponse.status(),
      file_status: fileResponse.status(),
      file_name: download.suggestedFilename(),
      content_type: fileResponse.headers()['content-type'] ?? '',
      content_disposition: fileResponse.headers()['content-disposition'] ?? '',
      bytes: body.length,
      magic: body.subarray(0, 4).toString('hex'),
    });
    await page.waitForTimeout(100);
  }
}

await downloadVisibleRows();
const nextButton = panel.locator('.ant-pagination-next button');
if (await nextButton.count()) {
  const previousFirstLabel = await panel.getByRole('button', { name: /^下载证据 / }).first().getAttribute('aria-label');
  await nextButton.click();
  await page.waitForFunction(
    (label) => document.querySelector('.taf-alert-detail-evidence-panel button[aria-label^="下载证据 "]')?.getAttribute('aria-label') !== label,
    previousFirstLabel,
  );
  await downloadVisibleRows();
}

const auditRows = JSON.parse(execFileSync(
  'kubectl',
  [
    '-n', 'databases', 'exec', 'postgres-primary-0', '--',
    'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc',
    `SELECT COALESCE(json_agg(t), '[]'::json) FROM (
      SELECT action, object_id, detail->>'result' AS result, detail->>'bytes' AS bytes, created_at
      FROM audit_logs
      WHERE action = 'ALERT_EVIDENCE_DOWNLOADED'
        AND object_id LIKE 'alert-detail-r802-%'
      ORDER BY created_at DESC LIMIT 6
    ) t;`,
  ],
  { encoding: 'utf8', env: process.env, timeout: 15_000 },
).trim());

const uniqueFileNames = new Set(downloads.map((item) => item.file_name));
const uniqueEvidenceIds = new Set(downloads.map((item) => item.evidence_id));
const realAlertReads = apiResponses.filter((item) => item.method === 'GET' && (
  item.url.endsWith(`/api/v1/alerts/${alertId}`)
  || item.url.includes(`/api/v1/alerts/${alertId}/evidence`)
));
const fileContractsPass = downloads.every((item) => {
  const expected = expectedFiles[item.file_name];
  return Boolean(expected)
    && item.access_status === 201
    && item.file_status === 200
    && item.bytes === expected.bytes
    && item.content_type.startsWith(expected.contentType)
    && item.content_disposition.includes('attachment;');
});
const pcap = downloads.find((item) => item.file_name === 'c2-tunnel.pcap');
const auditContractsPass = auditRows.length === 6
  && new Set(auditRows.map((item) => item.object_id)).size === 6
  && auditRows.every((item) => item.result === 'success' && Number(item.bytes) > 0);
const genericAxiosToastCount = await page.locator('.ant-message-notice-content')
  .filter({ hasText: /Request failed with status code/i }).count();

const result = {
  result: (
    runtimeConfig?.USE_MOCK === 'false'
    && runtimeConfig?.ALERT_DETAIL_API_ENABLED === 'true'
    && realAlertReads.length >= 2
    && realAlertReads.every((item) => item.status === 200)
    && downloads.length === 6
    && uniqueFileNames.size === 6
    && uniqueEvidenceIds.size === 6
    && fileContractsPass
    && pcap?.magic === 'd4c3b2a1'
    && auditContractsPass
    && genericAxiosToastCount === 0
    && consoleErrors.length === 0
  ) ? 'pass' : 'fail',
  browser: version.Browser,
  cdp_mapping: '127.0.0.1:9224 -> Windows 127.0.0.1:9222',
  runtime_config: runtimeConfig,
  real_alert_reads: realAlertReads,
  downloads,
  audit_rows: auditRows,
  generic_axios_toast_count: genericAxiosToastCount,
  relevant_requests: requests.filter((item) => item.url.includes('/evidence/')),
  console_errors: consoleErrors,
  timestamp: new Date().toISOString(),
};

fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close();
await browser.close();
if (result.result !== 'pass') process.exitCode = 1;

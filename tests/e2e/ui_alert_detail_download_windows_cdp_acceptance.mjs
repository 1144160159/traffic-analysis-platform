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
const outputPath = path.join(root, 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-alert-detail-r818-download.json');

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

const version = await (await fetch(`${cdpUrl}/json/version`)).json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
const requests = [];
const consoleErrors = [];
page.on('request', (request) => {
  if (request.url().includes('/api/v1/alerts/')) requests.push({
    method: request.method(),
    url: request.url(),
    post_data: request.postData(),
  });
});
page.on('console', (entry) => {
  if (entry.type() === 'error' && !entry.location().url.startsWith('chrome-extension://')) consoleErrors.push(entry.text());
});

const route = new URL(`/alerts/${encodeURIComponent(alertId)}?downloadAcceptance=${Date.now()}&evidenceView=pcap`, baseUrl);
route.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(route.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
const panel = page.locator('.taf-alert-detail-evidence-panel');
await panel.locator('.ant-table-tbody > tr:not(.ant-table-placeholder):not(.ant-table-measure-row)').first().waitFor({ state: 'visible', timeout: 20_000 });
const button = panel.getByRole('button', { name: /^下载证据 / }).first();
await button.waitFor({ state: 'visible' });
const buttonState = await button.evaluate((element) => ({
  disabled: element.disabled,
  ariaLabel: element.getAttribute('aria-label'),
}));
const assetFieldAudit = await page.locator('.taf-alert-detail-asset-card dd').evaluateAll((elements) => {
  const truncated = elements.filter((element) => element.scrollWidth > element.clientWidth);
  return {
    field_count: elements.length,
    truncated_count: truncated.length,
    all_truncated_fields_have_full_title: truncated.every((element) => (
      Boolean(element.getAttribute('title')?.trim())
    )),
  };
});

const accessPromise = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().includes('/evidence/access'), { timeout: 30_000 }).catch(() => null);
const filePromise = page.waitForResponse((response) => response.request().method() === 'GET' && response.url().includes('/download?'), { timeout: 60_000 }).catch(() => null);
const downloadPromise = page.waitForEvent('download', { timeout: 60_000 }).catch(() => null);
await button.click();
const accessResponse = await accessPromise;
if (!accessResponse) {
  const diagnostics = {
    button: buttonState,
    current_url: page.url(),
    messages: await page.locator('.ant-message-notice-content').allTextContents(),
    relevant_requests: requests.filter((item) => item.url.includes('/evidence/')),
    console_errors: consoleErrors,
  };
  console.error(JSON.stringify(diagnostics, null, 2));
  await page.close();
  await browser.close();
  process.exit(1);
}
const accessResult = await accessResponse.json();
const fileResponse = await filePromise;
const download = await downloadPromise;
if (!fileResponse || !download) {
  const diagnostics = {
    access_result: accessResult,
    file_response_seen: Boolean(fileResponse),
    download_event_seen: Boolean(download),
    messages: await page.locator('.ant-message-notice-content').allTextContents(),
    relevant_requests: requests.filter((item) => item.url.includes('/evidence/')),
    console_errors: consoleErrors,
  };
  console.error(JSON.stringify(diagnostics, null, 2));
  await page.close();
  await browser.close();
  process.exit(1);
}
const fileBody = await fileResponse.body();
const contentType = fileResponse.headers()['content-type'] ?? '';
const pcapMagic = fileBody.subarray(0, 4).toString('hex');
const validPcapMagic = ['d4c3b2a1', 'a1b2c3d4', '4d3cb2a1', 'a1b23c4d'].includes(pcapMagic);
const auditRows = JSON.parse(execFileSync(
  'kubectl',
  [
    '-n', 'databases', 'exec', 'postgres-primary-0', '--',
    'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc',
    `SELECT COALESCE(json_agg(t), '[]'::json) FROM (
      SELECT action, object_id, detail->>'result' AS result, detail->>'bytes' AS bytes, created_at
      FROM audit_logs
      WHERE action IN ('ALERT_EVIDENCE_ACCESS_REQUESTED','ALERT_EVIDENCE_DOWNLOADED')
        AND object_id = 'alert-detail-r802-pcap'
      ORDER BY created_at DESC LIMIT 2
    ) t;`,
  ],
  { encoding: 'utf8', env: process.env, timeout: 15_000 },
).trim());
const completedAudit = auditRows.find((row) => row.action === 'ALERT_EVIDENCE_DOWNLOADED');

const result = {
  result: (
    !buttonState.disabled
    && accessResponse.status() === 201
    && accessResult?.data?.audit_event === 'ALERT_EVIDENCE_ACCESS_REQUESTED'
    && accessResult?.data?.download_url?.includes('/download?')
    && fileResponse.status() === 200
    && fileResponse.headers()['content-disposition']?.includes('attachment;')
    && contentType.includes('application/vnd.tcpdump.pcap')
    && fileBody.length >= 24
    && validPcapMagic
    && download.suggestedFilename().endsWith('.pcap')
    && completedAudit?.result === 'success'
    && Number(completedAudit?.bytes) === fileBody.length
    && assetFieldAudit.all_truncated_fields_have_full_title
    && consoleErrors.length === 0
  ) ? 'pass' : 'fail',
  browser: version.Browser,
  cdp_mapping: '127.0.0.1:9224 -> Windows 127.0.0.1:9222',
  button: buttonState,
  asset_fields: assetFieldAudit,
  access_status: accessResponse.status(),
  access_result: accessResult,
  file_status: fileResponse.status(),
  content_type: contentType,
  content_disposition: fileResponse.headers()['content-disposition'] ?? '',
  file_bytes: fileBody.length,
  pcap_magic: pcapMagic,
  valid_pcap_magic: validPcapMagic,
  browser_download_filename: download.suggestedFilename(),
  audit_rows: auditRows,
  relevant_requests: requests.filter((item) => item.url.includes('/evidence/')),
  console_errors: consoleErrors,
  timestamp: new Date().toISOString(),
};
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close();
await browser.close();
if (result.result !== 'pass') process.exitCode = 1;

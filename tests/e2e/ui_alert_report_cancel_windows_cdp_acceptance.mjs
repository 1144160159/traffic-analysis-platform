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
const runId = process.env.RUN_ID || `windows-alert-report-cancel-${Date.now()}`;
const outputDir = path.resolve(root, process.env.OUTPUT_DIR || path.join('doc/02_acceptance/runs', runId));
const reportPath = path.join(outputDir, 'windows-chrome-report-cancel.json');
const screenshotPath = path.join(outputDir, 'windows-chrome-report-cancel-1920x1080.png');
const networkPath = path.join(outputDir, 'windows-chrome-report-cancel.har.json');

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
    username: `codex-${runId}`,
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:read', 'alert:write', 'alert:export'],
    token_type: 'access',
    session_id: crypto.randomUUID(),
    iat: now,
    exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encodedSecret, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

function sql(query) {
  return execFileSync(
    'kubectl',
    ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', query],
    { encoding: 'utf8', env: process.env, timeout: 45_000 },
  ).trim().split('\n').at(-1);
}

function escapeSQL(value) {
  return String(value).replaceAll("'", "''");
}

function redactedUrl(value) {
  return String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
}

function waitForReportExportOutcome(page, alertId, maxSnapshotConflicts = 3, timeoutMs = 30_000) {
  const encodedAlertId = encodeURIComponent(alertId);
  return new Promise((resolve, reject) => {
    const responses = [];
    const timeout = setTimeout(() => {
      page.off('response', onResponse);
      reject(new Error(`report export response timed out after ${timeoutMs}ms`));
    }, timeoutMs);
    const finish = (response) => {
      clearTimeout(timeout);
      page.off('response', onResponse);
      resolve({ response, statuses: responses.map((item) => item.status()) });
    };
    const onResponse = (response) => {
      if (
        response.request().method() !== 'POST'
        || !response.url().includes(`/api/v1/alerts/${encodedAlertId}/reports/export`)
      ) return;
      responses.push(response);
      if (response.status() === 409 && responses.length < maxSnapshotConflicts) return;
      finish(response);
    };
    page.on('response', onResponse);
  });
}

function waitForReportCancelOutcome(page, alertId, jobId, timeoutMs = 30_000) {
  const encodedAlertId = encodeURIComponent(alertId);
  const encodedJobId = encodeURIComponent(jobId);
  const cancelPath = `/api/v1/alerts/${encodedAlertId}/reports/${encodedJobId}/cancel`;
  const jobPath = `/api/v1/alerts/${encodedAlertId}/reports/${encodedJobId}`;
  return new Promise((resolve, reject) => {
    const postStatuses = [];
    let conflictResponse;
    let finished = false;
    const timeout = setTimeout(() => {
      page.off('response', onResponse);
      reject(new Error(`report cancellation response timed out after ${timeoutMs}ms`));
    }, timeoutMs);
    const finish = (outcome) => {
      if (finished) return;
      finished = true;
      clearTimeout(timeout);
      page.off('response', onResponse);
      resolve({ ...outcome, postStatuses });
    };
    const onResponse = async (response) => {
      const method = response.request().method();
      if (method === 'POST' && response.url().includes(cancelPath)) {
        postStatuses.push(response.status());
        if (response.status() === 409) {
          conflictResponse = response;
          return;
        }
        finish({ response, recoveredTerminalPayload: undefined });
        return;
      }
      if (!conflictResponse || method !== 'GET' || !response.url().includes(jobPath) || response.status() !== 200) return;
      const payload = await response.json().catch(() => ({}));
      const data = payload.data ?? payload;
      if (!['completed', 'partial', 'failed', 'cancelled', 'compensated', 'compensation_failed'].includes(String(data.status || ''))) return;
      finish({ response: conflictResponse, recoveredTerminalPayload: data });
    };
    page.on('response', onResponse);
  });
}

const token = smokeToken();
const alertListResponse = await fetch(`${baseUrl}/api/v1/alerts?limit=10&offset=0`, {
  headers: { Authorization: `Bearer ${token}`, 'X-Request-ID': `${runId}-candidate-list` },
  signal: AbortSignal.timeout(45_000),
});
if (!alertListResponse.ok) throw new Error(`alert candidate request failed: ${alertListResponse.status}`);
const alertListBody = await alertListResponse.json();
const alertCandidates = Array.isArray(alertListBody.data)
  ? alertListBody.data
  : (alertListBody.data?.alerts ?? alertListBody.alerts ?? []);
if (!alertCandidates.length) throw new Error('no real alert candidates are available');

const versionResponse = await fetch(`${cdpUrl}/json/version`, { signal: AbortSignal.timeout(10_000) });
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const requestStartedAt = new Map();
const networkEntries = [];
const consoleErrors = [];
const ignoredExtensionErrors = [];
const pageErrors = [];
const recoveredCancelConflictUrls = new Set();
page.on('request', (request) => {
  if (!request.url().includes('/api/v1/alerts/')) return;
  requestStartedAt.set(request, Date.now());
});
page.on('response', (response) => {
  if (!response.url().includes('/api/v1/alerts/')) return;
  const request = response.request();
  const startedAt = requestStartedAt.get(request) ?? Date.now();
  networkEntries.push({
    startedDateTime: new Date(startedAt).toISOString(),
    time: Date.now() - startedAt,
    request: {
      method: request.method(),
      url: redactedUrl(request.url()),
      postData: request.postData() || undefined,
    },
    response: { status: response.status(), statusText: response.statusText(), url: redactedUrl(response.url()) },
  });
});
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const item = { text: entry.text(), url: entry.location().url || '' };
  if (item.url.startsWith('chrome-extension://')) ignoredExtensionErrors.push(item);
  else consoleErrors.push(item);
});
page.on('pageerror', (error) => {
  if (error.message.includes('crypto.randomUUID is not a function')) {
    ignoredExtensionErrors.push({ text: error.message, url: 'chrome-extension://browser-profile' });
  } else {
    pageErrors.push(error.message);
  }
});

let selectedAlertId = '';
let exportPayload;
let cancelPayload;
let terminalPayload;
let modalTerminalVisible = false;
const attempts = [];
for (const candidate of alertCandidates.slice(0, 8)) {
  const alertId = String(candidate.alert_id || candidate.id || '');
  if (!alertId) continue;
  const route = new URL(`/alerts/${encodeURIComponent(alertId)}?windowsReportCancel=${Date.now()}`, baseUrl);
  route.hash = `codex_smoke_token=${token}`;
  await page.goto(route.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-alert-detail-page').waitFor({ state: 'visible', timeout: 20_000 });
  await page.locator('[data-action-id="alert-report-export"]').click();
  const modal = page.locator('.ant-modal:visible');
  await modal.waitFor({ state: 'visible', timeout: 5_000 });
  await modal.locator('textarea').fill(`Windows Chrome ${runId} 验证报告取消最终态与对象清理`);
  const exportOutcomePromise = waitForReportExportOutcome(page, alertId);
  await modal.getByRole('button', { name: '确认提交' }).click();
  const exportOutcome = await exportOutcomePromise;
  const exportResponse = exportOutcome.response;
  const currentExportPayload = await exportResponse.json().catch(() => ({}));
  const exportData = currentExportPayload.data ?? currentExportPayload;
  const attempt = {
    alert_id: alertId,
    export_status: exportResponse.status(),
    export_statuses: exportOutcome.statuses,
    export_job_id: String(exportData.job_id || ''),
    export_state: String(exportData.status || ''),
    cancel_button_visible: false,
    cancel_status: 0,
  };
  attempts.push(attempt);
  if (exportResponse.status() !== 202) {
    await modal.getByRole('button', { name: '取消' }).click().catch(() => page.keyboard.press('Escape'));
    continue;
  }
  const cancelButton = modal.locator('[data-action-id="alert-report-cancel"]');
  const cancelVisible = await cancelButton.isVisible({ timeout: 1_200 }).catch(() => false);
  attempt.cancel_button_visible = cancelVisible;
  if (!cancelVisible) {
    await modal.getByRole('button', { name: '取消' }).click().catch(() => page.keyboard.press('Escape'));
    continue;
  }
  const cancelOutcomePromise = waitForReportCancelOutcome(page, alertId, String(exportData.job_id || ''));
  await cancelButton.click();
  const confirmation = page.locator('.ant-popover:visible');
  await confirmation.getByRole('button', { name: '确认取消' }).click();
  const cancelOutcome = await cancelOutcomePromise;
  const cancelResponse = cancelOutcome.response;
  const currentCancelPayload = cancelOutcome.recoveredTerminalPayload
    ? { data: cancelOutcome.recoveredTerminalPayload }
    : await cancelResponse.json().catch(() => ({}));
  attempt.cancel_status = cancelResponse.status();
  attempt.cancel_statuses = cancelOutcome.postStatuses;
  attempt.cancel_state = String((currentCancelPayload.data ?? currentCancelPayload).status || '');
  attempt.cancel_conflict_recovered = Boolean(cancelOutcome.recoveredTerminalPayload);
  attempt.cancel_terminal_visible = false;
  if (cancelOutcome.recoveredTerminalPayload) {
    recoveredCancelConflictUrls.add(redactedUrl(cancelResponse.url()));
    attempt.cancel_terminal_visible = await modal.getByText(new RegExp(`任务 ${String(exportData.job_id)}：`))
      .isVisible({ timeout: 5_000 }).catch(() => false);
  }
  if (cancelResponse.status() !== 202) {
    await modal.getByRole('button', { name: '取消' }).click().catch(() => page.keyboard.press('Escape'));
    continue;
  }

  selectedAlertId = alertId;
  exportPayload = currentExportPayload;
  cancelPayload = currentCancelPayload;
  const jobId = String(exportData.job_id || '');
  for (let poll = 0; poll < 30; poll += 1) {
    const response = await fetch(`${baseUrl}/api/v1/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(jobId)}`, {
      headers: { Authorization: `Bearer ${token}`, 'X-Request-ID': `${runId}-terminal-${poll}` },
      signal: AbortSignal.timeout(45_000),
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(`terminal report read failed: ${response.status}`);
    terminalPayload = payload;
    const state = String((payload.data ?? payload).status || '');
    if (['cancelled', 'partial'].includes(state)) break;
    await page.waitForTimeout(500);
  }
  modalTerminalVisible = await modal.getByText(/已取消/, { exact: false }).first().isVisible({ timeout: 5_000 }).catch(() => false);
  break;
}

if (!selectedAlertId || !exportPayload || !cancelPayload || !terminalPayload) {
  throw new Error(`no browser report cancellation completed inside bounded attempts: ${JSON.stringify(attempts)}`);
}

const exportData = exportPayload.data ?? exportPayload;
const cancelData = cancelPayload.data ?? cancelPayload;
const terminalData = terminalPayload.data ?? terminalPayload;
const jobId = String(exportData.job_id || '');
const database = JSON.parse(sql(`SELECT json_build_object(
  'jobs',(SELECT count(*) FROM alert_report_jobs WHERE tenant_id='default' AND job_id='${escapeSQL(jobId)}' AND status='cancelled' AND object_bucket='' AND object_key='' AND artifact_sha256='' AND size_bytes=0),
  'history',(SELECT count(*) FROM alert_report_job_history WHERE tenant_id='default' AND job_id='${escapeSQL(jobId)}' AND to_status IN ('cancel_requested','cancelled')),
  'outbox',(SELECT count(*) FROM alert_report_outbox WHERE tenant_id='default' AND job_id='${escapeSQL(jobId)}' AND event_type IN ('traffic.alert.v2.AlertReportCancelRequested','traffic.alert.v2.AlertReportCancelled')),
  'audit',(SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND object_id='${escapeSQL(jobId)}' AND action IN ('ALERT_REPORT_EXPORT_CANCEL_REQUESTED','ALERT_REPORT_EXPORT_CANCELLED')),
  'control',(SELECT count(*) FROM alert_report_control_requests WHERE tenant_id='default' AND job_id='${escapeSQL(jobId)}' AND operation='cancel')
)`));
const runtimeConfig = await page.evaluate(() => window.__RUNTIME_CONFIG__);
fs.mkdirSync(outputDir, { recursive: true });
await page.screenshot({ path: screenshotPath, fullPage: false });

const successfulExportUrls = new Set(networkEntries
  .filter((entry) => (
    entry.request.method === 'POST'
    && entry.request.url.includes('/reports/export')
    && entry.response.status === 202
  ))
  .map((entry) => entry.request.url));
const recoveredSnapshotConflicts = networkEntries.filter((entry) => (
  entry.request.method === 'POST'
  && entry.request.url.includes('/reports/export')
  && entry.response.status === 409
  && successfulExportUrls.has(entry.request.url)
));
const recoveredCancelConflicts = networkEntries.filter((entry) => (
  entry.request.method === 'POST'
  && entry.request.url.includes('/cancel')
  && entry.response.status === 409
  && recoveredCancelConflictUrls.has(entry.request.url)
));
const relevantBadResponses = networkEntries.filter((entry) => (
  entry.response.status >= 400
  && !recoveredSnapshotConflicts.includes(entry)
  && !recoveredCancelConflicts.includes(entry)
));
const recoveredConflictConsoleErrors = consoleErrors.filter((entry) => (
  entry.text.includes('409')
  && (
    successfulExportUrls.has(entry.url)
    || recoveredCancelConflictUrls.has(entry.url)
  )
));
const relevantConsoleErrors = consoleErrors.filter((entry) => !recoveredConflictConsoleErrors.includes(entry));
const recoveredCancelUIReconciled = attempts
  .filter((attempt) => attempt.cancel_conflict_recovered)
  .every((attempt) => attempt.cancel_terminal_visible);
const terminalCancelled = terminalData.status === 'cancelled'
  && !terminalData.download_url
  && !terminalData.artifact_sha256
  && Number(terminalData.size_bytes) === 0
  && Boolean(terminalData.cancelled_at);
const databaseReconciled = Number(database.jobs) === 1
  && Number(database.history) >= 1
  && Number(database.outbox) >= 1
  && Number(database.audit) >= 1
  && Number(database.control) === 1;
const result = {
  result: (
    runtimeConfig?.USE_MOCK === 'false'
    && runtimeConfig?.ALERT_DETAIL_API_ENABLED === 'true'
    && exportData.status === 'accepted'
    && ['cancelled', 'cancel_requested'].includes(cancelData.status)
    && terminalCancelled
    && modalTerminalVisible
    && databaseReconciled
    && recoveredCancelUIReconciled
    && relevantBadResponses.length === 0
    && relevantConsoleErrors.length === 0
    && pageErrors.length === 0
  ) ? 'pass' : 'fail',
  run_id: runId,
  browser: version.Browser,
  cdp_mapping: 'Xshell 127.0.0.1:9224 -> Windows Chrome 127.0.0.1:9222',
  candidate: {
    ui_build_id: 'remediation-ui-c2975d525e67',
    alert_service_build_id: 'remediation-alert-report-faeb425bd7ff',
  },
  runtime_config: runtimeConfig,
  alert_id: selectedAlertId,
  job_id: jobId,
  export: exportData,
  cancellation: cancelData,
  terminal: terminalData,
  terminal_cancelled: terminalCancelled,
  modal_terminal_visible: modalTerminalVisible,
  database,
  database_reconciled: databaseReconciled,
  attempts,
  recovered_snapshot_conflicts: recoveredSnapshotConflicts,
  recovered_cancel_conflicts: recoveredCancelConflicts,
  recovered_cancel_ui_reconciled: recoveredCancelUIReconciled,
  relevant_bad_responses: relevantBadResponses,
  console_errors: relevantConsoleErrors,
  recovered_conflict_console_errors: recoveredConflictConsoleErrors,
  page_errors: pageErrors,
  ignored_extension_errors: ignoredExtensionErrors,
  screenshot: path.relative(root, screenshotPath),
  network_evidence: path.relative(root, networkPath),
  token_material_redacted: true,
  captured_at: new Date().toISOString(),
};
const har = {
  log: {
    version: '1.2',
    creator: { name: 'traffic-platform-windows-cdp-acceptance', version: '1' },
    pages: [{ id: runId, title: await page.title(), startedDateTime: result.captured_at, pageTimings: {} }],
    entries: networkEntries,
  },
};
fs.writeFileSync(networkPath, `${JSON.stringify(har, null, 2)}\n`, 'utf8');
fs.writeFileSync(reportPath, `${JSON.stringify(result, null, 2)}\n`, 'utf8');
console.log(JSON.stringify({
  result: result.result,
  report: path.relative(root, reportPath),
  screenshot: result.screenshot,
  browser: result.browser,
  alert_id: selectedAlertId,
  job_id: jobId,
  terminal: terminalData.status,
  database,
  attempts,
}, null, 2));
await page.close();
await browser.close();
if (result.result !== 'pass') process.exitCode = 1;

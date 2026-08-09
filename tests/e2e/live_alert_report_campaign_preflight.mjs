#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const root = process.cwd();
const baseUrl = process.env.BASE_URL || 'http://10.0.5.8:30180/api/v1';
const runId = process.env.RUN_ID || `alert-report-campaign-${Date.now()}`;
const outputPath = path.resolve(
  root,
  process.env.OUTPUT_PATH || path.join('doc/02_acceptance/runs', runId, 'report.json'),
);
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

const secret = Buffer.from(execFileSync(
  'kubectl',
  ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
  { encoding: 'utf8', env: process.env, timeout: 15_000 },
), 'base64').toString('utf8');

function token({ tenantId = 'default', permissions = ['*', 'admin:*', 'alert:read', 'alert:write', 'alert:export', 'campaign:read', 'campaign:write'] } = {}) {
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: tenantId,
    username: `codex-${runId}`,
    roles: permissions.includes('*') ? ['admin'] : ['viewer'],
    permissions,
    token_type: 'access',
    session_id: crypto.randomUUID(),
    iat: now,
    exp: now + 1800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const adminToken = token();
const viewerToken = token({ permissions: [] });
const otherTenantToken = token({ tenantId: 'tenant-b' });
const checks = [];
const traces = [];
const check = (name, passed, detail = {}) => checks.push({ name, passed: Boolean(passed), detail });

async function request(endpoint, { expected = 200, tokenValue = adminToken, raw = false, ...options } = {}) {
  const response = await fetch(`${baseUrl}${endpoint}`, {
    ...options,
    headers: {
      Authorization: `Bearer ${tokenValue}`,
      'Content-Type': 'application/json',
      'X-Request-ID': `${runId}-${crypto.randomUUID()}`,
      ...(options.headers || {}),
    },
    signal: AbortSignal.timeout(45_000),
  });
  traces.push({ method: options.method || 'GET', endpoint, status: response.status, trace_id: response.headers.get('x-trace-id') || '' });
  const body = raw ? Buffer.from(await response.arrayBuffer()) : await response.json().catch(() => ({}));
  if (response.status !== expected) {
    throw new Error(`${options.method || 'GET'} ${endpoint}: expected ${expected}, got ${response.status}: ${raw ? '<binary>' : JSON.stringify(body).slice(0, 500)}`);
  }
  return { response, body, data: raw ? body : (body.data ?? body) };
}

function sql(query) {
  const output = execFileSync(
    'kubectl',
    ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', query],
    { encoding: 'utf8', env: process.env, timeout: 45_000 },
  ).trim().split('\n').at(-1);
  return output;
}

const escapeSQL = (value) => String(value).replaceAll("'", "''");
const list = await request('/alerts?limit=10&offset=0');
const alerts = Array.isArray(list.data) ? list.data : list.data.alerts ?? [];
check('real alert candidates available', alerts.length >= 2 && alerts.every((item) => item.alert_id), { count: alerts.length });

const campaignsResponse = await request('/campaigns?limit=10&offset=0');
const campaigns = campaignsResponse.data.campaigns ?? [];
check('real campaign candidates available', campaigns.length >= 2 && campaigns.every((item) => item.campaign_id), { count: campaigns.length });

let selectedAlert;
let selectedCampaign;
for (const alert of alerts.slice(0, 5)) {
  const links = await request(`/alerts/${encodeURIComponent(alert.alert_id)}/campaign-links`);
  const rows = Array.isArray(links.data) ? links.data : links.data.links ?? [];
  const linked = new Set(rows.map((item) => item.campaign_id));
  const candidate = campaigns.find((item) => !linked.has(item.campaign_id));
  if (candidate) {
    selectedAlert = alert;
    selectedCampaign = candidate;
    break;
  }
}
if (!selectedAlert || !selectedCampaign) {
  throw new Error('no unlinked alert/campaign pair available in bounded candidate scan');
}
const alertId = selectedAlert.alert_id;
const campaignId = selectedCampaign.campaign_id;

let reportCreate;
let snapshotId;
let reportKey;
for (let attempt = 0; attempt < 3; attempt += 1) {
  const detail = await request(`/alerts/${encodeURIComponent(alertId)}`);
  const stateVersion = Number(detail.data.state_version);
  if (!Number.isInteger(stateVersion) || stateVersion < 0) throw new Error('alert detail is missing state_version');
  snapshotId = `alert:${alertId}:revision:${stateVersion}`;
  reportKey = `alert-report:${runId}:${attempt}:${crypto.randomUUID()}`;
  const response = await fetch(`${baseUrl}/alerts/${encodeURIComponent(alertId)}/reports/export`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${adminToken}`,
      'Content-Type': 'application/json',
      'Idempotency-Key': reportKey,
      'X-Request-ID': `${runId}-report-${attempt}`,
    },
    body: JSON.stringify({
      action_id: 'alert-report-export',
      format: 'json',
      snapshot_id: snapshotId,
      reason: `confirmed ${runId} deterministic report`,
    }),
    signal: AbortSignal.timeout(45_000),
  });
  const body = await response.json().catch(() => ({}));
  traces.push({ method: 'POST', endpoint: `/alerts/${alertId}/reports/export`, status: response.status, trace_id: response.headers.get('x-trace-id') || '' });
  if (response.status === 202) {
    reportCreate = { response, body, data: body.data ?? body };
    break;
  }
  if (response.status !== 409 || body?.error?.code !== 'SNAPSHOT_CONFLICT') {
    throw new Error(`report create failed: HTTP ${response.status} ${JSON.stringify(body).slice(0, 500)}`);
  }
}
if (!reportCreate) throw new Error('alert revision changed on every bounded report attempt');
const reportJobId = reportCreate.data.job_id;
check('report accepted with stable snapshot', reportCreate.data.status === 'accepted' && reportCreate.body.meta?.snapshot_id === snapshotId && Boolean(reportCreate.data.snapshot_sha256), {
  job_id: reportJobId,
  snapshot_id: snapshotId,
  snapshot_sha256: reportCreate.data.snapshot_sha256,
});

const reportReplay = await request(`/alerts/${encodeURIComponent(alertId)}/reports/export`, {
  expected: 202,
  method: 'POST',
  headers: { 'Idempotency-Key': reportKey },
  body: JSON.stringify({
    action_id: 'alert-report-export',
    format: 'json',
    snapshot_id: snapshotId,
    reason: `confirmed ${runId} deterministic report`,
  }),
});
check('report idempotent replay reuses job', reportReplay.data.job_id === reportJobId, { job_id: reportReplay.data.job_id });
await request(`/alerts/${encodeURIComponent(alertId)}/reports/export`, {
  expected: 409,
  method: 'POST',
  headers: { 'Idempotency-Key': reportKey },
  body: JSON.stringify({
    action_id: 'alert-report-export',
    format: 'pdf',
    snapshot_id: snapshotId,
    reason: `confirmed ${runId} conflict`,
  }),
});
check('report idempotency conflict rejected', true);

let reportJob;
for (let attempt = 0; attempt < 30; attempt += 1) {
  const current = await request(`/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(reportJobId)}`);
  reportJob = current.data;
  if (reportJob.status === 'completed' || reportJob.status === 'failed') break;
  await new Promise((resolve) => setTimeout(resolve, 1_000));
}
check('report worker completed object manifest', reportJob?.status === 'completed'
  && String(reportJob?.artifact_sha256).startsWith('sha256:')
  && Number(reportJob?.size_bytes) > 0
  && Boolean(reportJob?.download_url), reportJob);
if (reportJob?.status !== 'completed' || !reportJob?.download_url) {
  throw new Error(`report worker did not complete: status=${reportJob?.status || 'missing'} error=${reportJob?.error_message || ''}`);
}

const download = await request(reportJob.download_url.replace(/^\/v1/u, ''), { raw: true });
const artifactSHA256 = `sha256:${crypto.createHash('sha256').update(download.data).digest('hex')}`;
check('download body matches manifest sha and size', artifactSHA256 === reportJob.artifact_sha256
  && download.data.length === Number(reportJob.size_bytes)
  && download.response.headers.get('x-content-sha256') === reportJob.artifact_sha256, {
  artifact_sha256: artifactSHA256,
  size_bytes: download.data.length,
});
await request(`/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(reportJobId)}`, {
  expected: 403,
  tokenValue: viewerToken,
});
await request(`/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(reportJobId)}`, {
  expected: 404,
  tokenValue: otherTenantToken,
});
check('report permission and cross-tenant negatives enforced', true);

let cancelledReportCreate;
let cancelledReportControl;
let cancelledReportControlKey;
let cancelledReportControlBody;
for (let createAttempt = 0; createAttempt < 6 && !cancelledReportControl; createAttempt += 1) {
  const createKey = `alert-report-cancellable:${runId}:${createAttempt}:${crypto.randomUUID()}`;
  const create = await request(`/alerts/${encodeURIComponent(alertId)}/reports/export`, {
    expected: 202,
    method: 'POST',
    headers: { 'Idempotency-Key': createKey },
    body: JSON.stringify({
      action_id: 'alert-report-export',
      format: 'json',
      snapshot_id: snapshotId,
      reason: `confirmed ${runId} cancellable report`,
    }),
  });
  let current = create.data;
  for (let controlAttempt = 0; controlAttempt < 3; controlAttempt += 1) {
    const controlKey = `alert-report-cancel:${runId}:${createAttempt}:${controlAttempt}:${crypto.randomUUID()}`;
    const controlBody = {
      action_id: 'alert-report-cancel',
      expected_revision: Number(current.revision),
      reason: `confirmed ${runId} cooperative cancellation`,
    };
    const endpoint = `/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(create.data.job_id)}/cancel`;
    const response = await fetch(`${baseUrl}${endpoint}`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${adminToken}`,
        'Content-Type': 'application/json',
        'Idempotency-Key': controlKey,
        'X-Request-ID': `${runId}-cancel-${createAttempt}-${controlAttempt}`,
      },
      body: JSON.stringify(controlBody),
      signal: AbortSignal.timeout(45_000),
    });
    const body = await response.json().catch(() => ({}));
    traces.push({ method: 'POST', endpoint, status: response.status, trace_id: response.headers.get('x-trace-id') || '' });
    if (response.status === 202) {
      cancelledReportCreate = create;
      cancelledReportControl = { response, body, data: body.data ?? body };
      cancelledReportControlKey = controlKey;
      cancelledReportControlBody = controlBody;
      break;
    }
    if (response.status !== 409) {
      throw new Error(`report cancellation failed: HTTP ${response.status} ${JSON.stringify(body).slice(0, 500)}`);
    }
    const refreshed = await request(`/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(create.data.job_id)}`);
    current = refreshed.data;
    if (!['accepted', 'running'].includes(current.status)) break;
  }
}
if (!cancelledReportControl) {
  throw new Error('could not acquire an accepted/running report inside the bounded cancellation window');
}
const cancelledReportJobId = cancelledReportCreate.data.job_id;
check('report cancellation accepted with optimistic revision',
  ['cancelled', 'cancel_requested'].includes(cancelledReportControl.data.status)
    && Number(cancelledReportControl.data.revision) === Number(cancelledReportControlBody.expected_revision) + 1,
  cancelledReportControl.data);

let cancelledReportJob = cancelledReportControl.data;
for (let attempt = 0; attempt < 30; attempt += 1) {
  const current = await request(`/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(cancelledReportJobId)}`);
  cancelledReportJob = current.data;
  if (['cancelled', 'partial'].includes(cancelledReportJob.status)) break;
  await new Promise((resolve) => setTimeout(resolve, 500));
}
check('report cancellation reaches terminal state without residual manifest',
  cancelledReportJob.status === 'cancelled'
    && !cancelledReportJob.download_url
    && !cancelledReportJob.artifact_sha256
    && Number(cancelledReportJob.size_bytes) === 0
    && Boolean(cancelledReportJob.cancelled_at),
  cancelledReportJob);
if (cancelledReportJob.status !== 'cancelled') {
  throw new Error(`report cancellation did not reach cancelled: ${JSON.stringify(cancelledReportJob).slice(0, 500)}`);
}
const cancelReplay = await request(`/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(cancelledReportJobId)}/cancel`, {
  expected: 202,
  method: 'POST',
  headers: { 'Idempotency-Key': cancelledReportControlKey },
  body: JSON.stringify(cancelledReportControlBody),
});
check('report cancellation idempotent replay preserves terminal revision',
  cancelReplay.data.job_id === cancelledReportJobId
    && cancelReplay.data.status === 'cancelled'
    && Number(cancelReplay.data.revision) === Number(cancelledReportJob.revision),
  cancelReplay.data);
await request(`/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(cancelledReportJobId)}/cancel`, {
  expected: 403,
  method: 'POST',
  tokenValue: viewerToken,
  headers: { 'Idempotency-Key': `viewer-cancel-${runId}-${crypto.randomUUID()}` },
  body: JSON.stringify(cancelledReportControlBody),
});
await request(`/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(cancelledReportJobId)}/cancel`, {
  expected: 404,
  method: 'POST',
  tokenValue: otherTenantToken,
  headers: { 'Idempotency-Key': `other-tenant-cancel-${runId}-${crypto.randomUUID()}` },
  body: JSON.stringify(cancelledReportControlBody),
});
check('report cancellation permission and tenant negatives enforced', true);

const relationKey = `alert-campaign-link:${runId}:${crypto.randomUUID()}`;
const linkBody = {
  campaign_id: campaignId,
  expected_revision: 0,
  reason: `confirmed ${runId} alert campaign relation`,
};
const linkCreate = await request(`/alerts/${encodeURIComponent(alertId)}/campaign-links`, {
  method: 'POST',
  headers: { 'Idempotency-Key': relationKey },
  body: JSON.stringify(linkBody),
});
const relationId = linkCreate.data.relation_id;
check('campaign relation created with revision', Boolean(relationId)
  && linkCreate.data.status === 'linked'
  && linkCreate.data.revision === 1
  && linkCreate.data.idempotent_reuse === false, linkCreate.data);
const linkReplay = await request(`/alerts/${encodeURIComponent(alertId)}/campaign-links`, {
  method: 'POST',
  headers: { 'Idempotency-Key': relationKey },
  body: JSON.stringify(linkBody),
});
check('campaign relation idempotent replay is explicit', linkReplay.data.relation_id === relationId
  && linkReplay.data.idempotent_reuse === true, linkReplay.data);
const conflictingCampaign = campaigns.find((item) => item.campaign_id !== campaignId);
await request(`/alerts/${encodeURIComponent(alertId)}/campaign-links`, {
  expected: 409,
  method: 'POST',
  headers: { 'Idempotency-Key': relationKey },
  body: JSON.stringify({
    campaign_id: conflictingCampaign.campaign_id,
    expected_revision: 0,
    reason: `confirmed ${runId} conflict`,
  }),
});
check('campaign relation idempotency conflict rejected', true);
const linkList = await request(`/alerts/${encodeURIComponent(alertId)}/campaign-links`);
const linkRows = Array.isArray(linkList.data) ? linkList.data : linkList.data.links ?? [];
check('campaign relation list readback', linkRows.some((item) => item.relation_id === relationId && item.campaign_id === campaignId), {
  relation_id: relationId,
});
const otherTenantLinks = await request(`/alerts/${encodeURIComponent(alertId)}/campaign-links`, { tokenValue: otherTenantToken });
const otherRows = Array.isArray(otherTenantLinks.data) ? otherTenantLinks.data : otherTenantLinks.data.links ?? [];
check('campaign relation cross-tenant read is empty', otherRows.length === 0, { rows: otherRows.length });

const reportSQL = JSON.parse(sql(`SELECT json_build_object(
  'jobs',(SELECT count(*) FROM alert_report_jobs WHERE tenant_id='default' AND job_id='${escapeSQL(reportJobId)}' AND status='completed'),
  'outbox',(SELECT count(*) FROM alert_report_outbox WHERE tenant_id='default' AND job_id='${escapeSQL(reportJobId)}'),
  'audit',(SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND object_id='${escapeSQL(reportJobId)}' AND action IN ('ALERT_REPORT_EXPORT_REQUESTED','ALERT_REPORT_EXPORT_COMPLETED','ALERT_REPORT_DOWNLOADED'))
)`));
check('report PG job outbox and audit reconcile', Number(reportSQL.jobs) === 1
  && Number(reportSQL.outbox) >= 2
  && Number(reportSQL.audit) >= 3, reportSQL);

const cancellationSQL = JSON.parse(sql(`SELECT json_build_object(
  'jobs',(SELECT count(*) FROM alert_report_jobs WHERE tenant_id='default' AND job_id='${escapeSQL(cancelledReportJobId)}' AND status='cancelled' AND object_bucket='' AND object_key='' AND artifact_sha256='' AND size_bytes=0),
  'history',(SELECT count(*) FROM alert_report_job_history WHERE tenant_id='default' AND job_id='${escapeSQL(cancelledReportJobId)}' AND to_status IN ('cancel_requested','cancelled')),
  'outbox',(SELECT count(*) FROM alert_report_outbox WHERE tenant_id='default' AND job_id='${escapeSQL(cancelledReportJobId)}' AND event_type IN ('traffic.alert.v2.AlertReportCancelRequested','traffic.alert.v2.AlertReportCancelled')),
  'audit',(SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND object_id='${escapeSQL(cancelledReportJobId)}' AND action IN ('ALERT_REPORT_EXPORT_CANCEL_REQUESTED','ALERT_REPORT_EXPORT_CANCELLED')),
  'control',(SELECT count(*) FROM alert_report_control_requests WHERE tenant_id='default' AND job_id='${escapeSQL(cancelledReportJobId)}' AND operation='cancel')
)`));
check('report cancellation PG history outbox audit control and manifest reconcile',
  Number(cancellationSQL.jobs) === 1
    && Number(cancellationSQL.history) >= 1
    && Number(cancellationSQL.outbox) >= 1
    && Number(cancellationSQL.audit) >= 1
    && Number(cancellationSQL.control) === 1,
  cancellationSQL);

const relationSQL = JSON.parse(sql(`SELECT json_build_object(
  'links',(SELECT count(*) FROM campaign_alert_links WHERE tenant_id='default' AND relation_id='${escapeSQL(relationId)}'::uuid AND status='linked' AND revision=1),
  'history',(SELECT count(*) FROM campaign_alert_link_history WHERE tenant_id='default' AND relation_id='${escapeSQL(relationId)}'::uuid),
  'outbox',(SELECT count(*) FROM campaign_alert_link_outbox WHERE tenant_id='default' AND aggregate_id='${escapeSQL(relationId)}'::uuid),
  'audit',(SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND object_id='${escapeSQL(relationId)}' AND action IN ('ALERT_CAMPAIGN_LINKED','ALERT_CAMPAIGN_LINK_REUSED'))
)`));
check('campaign relation PG history outbox and audit reconcile', Number(relationSQL.links) === 1
  && Number(relationSQL.history) === 1
  && Number(relationSQL.outbox) === 1
  && Number(relationSQL.audit) >= 2, relationSQL);

const result = {
  schema_version: 1,
  run_id: runId,
  result: checks.every((item) => item.passed) ? 'pass' : 'fail',
  candidate: { alert_id: alertId, campaign_id: campaignId },
  report: {
    job_id: reportJobId,
    snapshot_id: snapshotId,
    snapshot_sha256: reportJob.snapshot_sha256,
    artifact_sha256: reportJob.artifact_sha256,
    size_bytes: reportJob.size_bytes,
    mime_type: reportJob.mime_type,
    partial: Boolean(reportCreate.body.meta?.partial),
    missing_sections: reportCreate.body.meta?.missing_sections ?? [],
    source_watermarks: reportCreate.body.meta?.source_watermarks ?? {},
  },
  cancelled_report: {
    job_id: cancelledReportJobId,
    status: cancelledReportJob.status,
    revision: cancelledReportJob.revision,
    cancel_requested_at: cancelledReportJob.cancel_requested_at,
    cancelled_at: cancelledReportJob.cancelled_at,
    residual_manifest: Boolean(cancelledReportJob.artifact_sha256) || Number(cancelledReportJob.size_bytes) !== 0,
  },
  campaign_relation: {
    relation_id: relationId,
    revision: linkCreate.data.revision,
    campaign_revision: linkCreate.body.meta?.source_watermarks?.['postgresql.campaign_workbench_state.revision'] ?? '',
  },
  checks,
  traces,
  token_material_redacted: true,
  captured_at: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`, 'utf8');
console.log(JSON.stringify({
  result: result.result,
  output: path.relative(root, outputPath),
  checks: `${checks.filter((item) => item.passed).length}/${checks.length}`,
  report_job_id: reportJobId,
  cancelled_report_job_id: cancelledReportJobId,
  relation_id: relationId,
}, null, 2));
if (result.result !== 'pass') process.exitCode = 1;

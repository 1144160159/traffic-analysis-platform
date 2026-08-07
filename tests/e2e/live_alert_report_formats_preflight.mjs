#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const root = process.cwd();
const baseUrl = process.env.BASE_URL || 'http://10.0.5.8:30180/api/v1';
const runId = process.env.RUN_ID || `alert-report-formats-${Date.now()}`;
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

function token() {
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: `codex-${runId}`,
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:read', 'alert:export'],
    token_type: 'access',
    session_id: crypto.randomUUID(),
    iat: now,
    exp: now + 1800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const authorization = `Bearer ${token()}`;
const checks = [];
const traces = [];
const check = (name, passed, detail = {}) => checks.push({ name, passed: Boolean(passed), detail });

async function request(endpoint, { expected = 200, raw = false, ...options } = {}) {
  const response = await fetch(`${baseUrl}${endpoint}`, {
    ...options,
    headers: {
      Authorization: authorization,
      'Content-Type': 'application/json',
      'X-Request-ID': `${runId}-${crypto.randomUUID()}`,
      ...(options.headers || {}),
    },
    signal: AbortSignal.timeout(45_000),
  });
  const body = raw ? Buffer.from(await response.arrayBuffer()) : await response.json().catch(() => ({}));
  traces.push({
    method: options.method || 'GET',
    endpoint,
    status: response.status,
    trace_id: response.headers.get('x-trace-id') || '',
  });
  if (response.status !== expected) {
    throw new Error(`${options.method || 'GET'} ${endpoint}: expected ${expected}, got ${response.status}: ${raw ? '<binary>' : JSON.stringify(body).slice(0, 600)}`);
  }
  return { response, body, data: raw ? body : (body.data ?? body) };
}

function sql(query) {
  return execFileSync(
    'kubectl',
    ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', query],
    { encoding: 'utf8', env: process.env, timeout: 45_000 },
  ).trim().split('\n').at(-1);
}

const escapeSQL = (value) => String(value).replaceAll("'", "''");

async function createReport(alertId, format) {
  let accepted;
  for (let attempt = 0; attempt < 5; attempt += 1) {
    const detail = await request(`/alerts/${encodeURIComponent(alertId)}`);
    const revision = Number(detail.data.state_version);
    const snapshotId = `alert:${alertId}:revision:${revision}`;
    const idempotencyKey = `alert-report:${format}:${runId}:${attempt}:${crypto.randomUUID()}`;
    const response = await fetch(`${baseUrl}/alerts/${encodeURIComponent(alertId)}/reports/export`, {
      method: 'POST',
      headers: {
        Authorization: authorization,
        'Content-Type': 'application/json',
        'Idempotency-Key': idempotencyKey,
        'X-Request-ID': `${runId}-${format}-${attempt}`,
      },
      body: JSON.stringify({
        action_id: 'alert-report-export',
        format,
        snapshot_id: snapshotId,
        reason: `confirmed ${runId} ${format} artifact`,
      }),
      signal: AbortSignal.timeout(45_000),
    });
    const body = await response.json().catch(() => ({}));
    traces.push({
      method: 'POST',
      endpoint: `/alerts/${alertId}/reports/export`,
      status: response.status,
      trace_id: response.headers.get('x-trace-id') || '',
    });
    if (response.status === 202) {
      accepted = { body, data: body.data ?? body, snapshotId };
      break;
    }
    if (response.status !== 409 || body?.error?.code !== 'SNAPSHOT_CONFLICT') {
      throw new Error(`${format} report create failed: HTTP ${response.status} ${JSON.stringify(body).slice(0, 500)}`);
    }
  }
  if (!accepted) throw new Error(`${format} report revision changed on every bounded attempt`);

  let job;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const current = await request(`/alerts/${encodeURIComponent(alertId)}/reports/${encodeURIComponent(accepted.data.job_id)}`);
    job = current.data;
    if (job.status === 'completed' || job.status === 'failed') break;
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  if (job?.status !== 'completed' || !job?.download_url) {
    throw new Error(`${format} worker status=${job?.status || 'missing'} error=${job?.error_message || ''}`);
  }
  const download = await request(job.download_url.replace(/^\/v1/u, ''), { raw: true });
  const digest = `sha256:${crypto.createHash('sha256').update(download.data).digest('hex')}`;
  return {
    format,
    snapshot_id: accepted.snapshotId,
    snapshot_sha256: job.snapshot_sha256,
    job_id: job.job_id,
    artifact_sha256: job.artifact_sha256,
    size_bytes: job.size_bytes,
    mime_type: job.mime_type,
    content_type: download.response.headers.get('content-type') || '',
    content_disposition: download.response.headers.get('content-disposition') || '',
    header_sha256: download.response.headers.get('x-content-sha256') || '',
    body_sha256: digest,
    body: download.data,
    partial: Boolean(accepted.body.meta?.partial),
    missing_sections: accepted.body.meta?.missing_sections ?? [],
  };
}

let result;
try {
  const list = await request('/alerts?limit=10&offset=0');
  const alerts = Array.isArray(list.data) ? list.data : list.data.alerts ?? [];
  const alertId = alerts[0]?.alert_id;
  if (!alertId) throw new Error('no live alert candidate available');
  const artifacts = [];
  for (const format of ['pdf', 'docx']) {
    const artifact = await createReport(alertId, format);
    const baseValid = artifact.body_sha256 === artifact.artifact_sha256
      && artifact.header_sha256 === artifact.artifact_sha256
      && artifact.body.length === Number(artifact.size_bytes)
      && artifact.content_type.toLowerCase().startsWith(artifact.mime_type.toLowerCase())
      && artifact.content_disposition.toLowerCase().includes(`.${format}`);
    check(`${format} object manifest and download body reconcile`, baseValid, {
      job_id: artifact.job_id,
      snapshot_sha256: artifact.snapshot_sha256,
      artifact_sha256: artifact.artifact_sha256,
      size_bytes: artifact.size_bytes,
      mime_type: artifact.mime_type,
      content_type: artifact.content_type,
      content_disposition: artifact.content_disposition,
    });
    if (format === 'pdf') {
      const text = artifact.body.toString('latin1');
      check('PDF artifact has a complete PDF envelope',
        artifact.body.subarray(0, 5).toString('ascii') === '%PDF-'
          && text.includes('/Type /Catalog')
          && text.trimEnd().endsWith('%%EOF'),
        { prefix: artifact.body.subarray(0, 8).toString('ascii'), bytes: artifact.body.length });
    } else {
      const binary = artifact.body.toString('latin1');
      check('DOCX artifact has required OPC package members',
        artifact.body.subarray(0, 4).toString('hex') === '504b0304'
          && binary.includes('[Content_Types].xml')
          && binary.includes('_rels/.rels')
          && binary.includes('word/document.xml'),
        { prefix_hex: artifact.body.subarray(0, 4).toString('hex'), bytes: artifact.body.length });
    }
    delete artifact.body;
    artifacts.push(artifact);
  }

  const jobIds = artifacts.map((item) => `'${escapeSQL(item.job_id)}'`).join(',');
  const pg = JSON.parse(sql(`SELECT json_build_object(
    'jobs',(SELECT count(*) FROM alert_report_jobs WHERE tenant_id='default' AND job_id IN (${jobIds}) AND status='completed' AND format IN ('pdf','docx')),
    'outbox',(SELECT count(*) FROM alert_report_outbox WHERE tenant_id='default' AND job_id IN (${jobIds})),
    'audit',(SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND object_id IN (${jobIds}) AND action IN ('ALERT_REPORT_EXPORT_REQUESTED','ALERT_REPORT_EXPORT_COMPLETED','ALERT_REPORT_DOWNLOADED'))
  )`));
  check('PDF and DOCX jobs reconcile with PG outbox and audit',
    Number(pg.jobs) === 2 && Number(pg.outbox) >= 4 && Number(pg.audit) >= 6, pg);

  result = {
    schema_version: 1,
    run_id: runId,
    result: checks.every((item) => item.passed) ? 'pass' : 'fail',
    candidate: {
      alert_id: alertId,
      image: 'docker.io/traffic/alert-service:remediation-7c3110d2d17f',
    },
    artifacts,
    checks,
    traces,
    token_material_redacted: true,
    captured_at: new Date().toISOString(),
  };
} catch (error) {
  check('preflight execution', false, {
    error: error instanceof Error ? error.message : String(error),
  });
  result = {
    schema_version: 1,
    run_id: runId,
    result: 'fail',
    checks,
    traces,
    token_material_redacted: true,
    captured_at: new Date().toISOString(),
  };
}

fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`, 'utf8');
console.log(JSON.stringify({
  result: result.result,
  output: path.relative(root, outputPath),
  checks: `${checks.filter((item) => item.passed).length}/${checks.length}`,
  artifacts: result.artifacts?.map((item) => ({
    format: item.format,
    job_id: item.job_id,
    size_bytes: item.size_bytes,
  })) ?? [],
}, null, 2));
if (result.result !== 'pass') process.exitCode = 1;

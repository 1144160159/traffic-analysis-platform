#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const root = process.cwd();
const baseUrl = process.env.APISIX || 'http://10.0.5.8:30180';
const runId = process.env.RUN_ID || `forensics-source-refs-${new Date().toISOString().replace(/[-:.TZ]/gu, '').slice(0, 14)}`;
const outputDir = path.join(root, 'doc/02_acceptance/runs', runId);
const tenant = process.env.TENANT || 'default';
const otherTenant = process.env.OTHER_TENANT || 'campus-a';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

fs.mkdirSync(outputDir, { recursive: false });

function kubectl(args, options = {}) {
  return execFileSync('kubectl', args, {
    encoding: 'utf8',
    timeout: 20_000,
    env: process.env,
    ...options,
  }).trim();
}

function secretValue(key) {
  return Buffer.from(kubectl([
    '-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials',
    '-o', `jsonpath={.data.${key}}`,
  ]), 'base64').toString('utf8');
}

function tokenFor(targetTenant, username) {
  const now = Math.floor(Date.now() / 1000);
  const encode = (value) => Buffer.from(JSON.stringify(value)).toString('base64url');
  const header = encode({ alg: 'HS256', typ: 'JWT' });
  const claims = encode({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: targetTenant,
    username,
    email: `${username}@local`,
    roles: ['analyst'],
    permissions: ['pcap:read', 'pcap:write', 'pcap:download'],
    token_type: 'access',
    session_id: `${runId}-${crypto.randomUUID()}`,
    iat: now,
    exp: now + 1800,
  });
  const input = `${header}.${claims}`;
  const signature = crypto.createHmac('sha256', secretValue('JWT_SECRET')).update(input).digest('base64url');
  return `${input}.${signature}`;
}

function sqlLiteral(value) {
  return `'${String(value).replaceAll("'", "''")}'`;
}

function postgres(sql) {
  const password = secretValue('PG_PASSWORD');
  try {
    return kubectl([
      '-n', 'databases', 'exec', 'postgres-primary-0', '--',
      'env', `PGPASSWORD=${password}`,
      'psql', '-U', 'postgres', '-d', 'traffic_platform', '-v', 'ON_ERROR_STOP=1', '-Atc', sql,
    ], { maxBuffer: 1024 * 1024, stdio: ['ignore', 'pipe', 'pipe'] });
  } catch {
    throw new Error('PostgreSQL reconciliation command failed; inspect the redacted database-side logs');
  }
}

const checks = [];
const artifacts = {};
const record = (name, pass, detail = {}) => checks.push({ name, status: pass ? 'pass' : 'fail', detail });

async function request(name, method, pathname, token, { body, headers = {} } = {}) {
  const response = await fetch(new URL(pathname, baseUrl), {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      'X-Tenant-ID': headers['X-Tenant-ID'] || tenant,
      Accept: 'application/json',
      ...(body ? { 'Content-Type': 'application/json' } : {}),
      ...headers,
    },
    body: body ? JSON.stringify(body) : undefined,
    signal: AbortSignal.timeout(20_000),
  });
  const payload = await response.json().catch(() => null);
  const artifact = {
    name,
    method,
    path: pathname,
    request_headers: Object.fromEntries(Object.entries(headers).filter(([key]) => key.toLowerCase() !== 'authorization')),
    request_body: body || null,
    response_status: response.status,
    response_headers: {
      'x-request-id': response.headers.get('x-request-id'),
      'x-trace-id': response.headers.get('x-trace-id'),
      'x-event-id': response.headers.get('x-event-id'),
      'idempotency-key': response.headers.get('idempotency-key'),
      etag: response.headers.get('etag'),
    },
    response: payload,
  };
  artifacts[name] = artifact;
  fs.writeFileSync(path.join(outputDir, `${name}.json`), `${JSON.stringify(artifact, null, 2)}\n`, 'utf8');
  return { response, payload, artifact };
}

const sourceRefs = {
  alert_id: `AL-LIVE-${crypto.randomUUID()}`,
  campaign_id: `CP-LIVE-${crypto.randomUUID()}`,
  baseline_id: `BL-LIVE-${crypto.randomUUID()}`,
  evidence_id: `EV-LIVE-${crypto.randomUUID()}`,
  evidence_type: 'pcap',
};
const now = Date.now();
const createBody = {
  ...sourceRefs,
  src_ip: '192.0.2.84',
  dst_ip: '198.51.100.84',
  protocol: 6,
  start_time: now - 120_000,
  end_time: now - 60_000,
  max_packets: 10,
};
const idempotencyKey = `forensics-source-refs-${crypto.randomUUID()}`;
const requestId = `forensics-source-refs-${crypto.randomUUID()}`;
const traceId = crypto.randomBytes(16).toString('hex');
const commandHeaders = {
  'Idempotency-Key': idempotencyKey,
  'X-Action-Reason': `live source-reference acceptance ${runId}`,
  'X-Request-ID': requestId,
  'X-Trace-ID': traceId,
};

let jobId = '';
let eventId = '';
let unexpectedError = '';

try {
  const primaryToken = tokenFor(tenant, 'codex-forensics-source-refs');
  const otherToken = tokenFor(otherTenant, 'codex-forensics-source-refs-other');

  const created = await request('01-create', 'POST', '/api/v1/pcap/jobs', primaryToken, {
    body: createBody,
    headers: commandHeaders,
  });
  jobId = String(created.payload?.data?.job_id || '');
  eventId = String(created.payload?.data?.event_id || '');
  record('accepted command returns stable receipt',
    created.response.status === 202
      && created.payload?.success === true
      && /^[0-9a-f-]{36}$/u.test(jobId)
      && created.payload?.data?.revision === 1
      && created.payload?.data?.action_id === 'forensics.pcap-cut.create'
      && created.payload?.data?.idempotency_key === idempotencyKey
      && created.payload?.data?.outbox_status === 'pending'
      && created.payload?.data?.compatibility_mode !== true,
    { status: created.response.status, job_id: jobId, event_id: eventId, trace_id: created.artifact.response_headers['x-trace-id'] });
  record('request trace propagated to response', created.artifact.response_headers['x-trace-id'] === traceId, {
    requested: traceId,
    returned: created.artifact.response_headers['x-trace-id'],
  });

  const replay = await request('02-idempotent-replay', 'POST', '/api/v1/pcap/jobs', primaryToken, {
    body: createBody,
    headers: commandHeaders,
  });
  record('same idempotency key replays the original receipt',
    replay.response.status === 202
      && replay.payload?.data?.job_id === jobId
      && replay.payload?.data?.event_id === eventId
      && replay.payload?.data?.replayed === true,
    { status: replay.response.status, job_id: replay.payload?.data?.job_id, replayed: replay.payload?.data?.replayed });

  const conflict = await request('03-idempotency-conflict', 'POST', '/api/v1/pcap/jobs', primaryToken, {
    body: { ...createBody, max_packets: 11 },
    headers: commandHeaders,
  });
  record('idempotency key rejects a different command', conflict.response.status === 409
    && ['BIZ_3011', 'DEDUP_CONFLICT', 'IDEMPOTENCY_CONFLICT', 'CONFLICT'].includes(String(conflict.payload?.error?.code || conflict.payload?.code || '')),
  { status: conflict.response.status, error_code: conflict.payload?.error?.code || conflict.payload?.code || '' });

  const query = new URLSearchParams({ ...sourceRefs, limit: '20' }).toString();
  const matching = await request('04-filter-all-source-refs', 'GET', `/api/v1/pcap/jobs?${query}`, primaryToken);
  const matchingRows = Array.isArray(matching.payload?.data) ? matching.payload.data : [];
  record('combined source-reference filter reads the operational task', matching.response.status === 200
    && matchingRows.length === 1
    && matchingRows[0]?.job_id === jobId
    && Object.entries(sourceRefs).every(([key, value]) => matchingRows[0]?.params?.[key] === value), {
    status: matching.response.status,
    total: matching.payload?.meta?.page?.total,
    job_ids: matchingRows.map((row) => row.job_id),
  });

  const noMatch = await request('05-filter-nonmatch', 'GET', `/api/v1/pcap/jobs?alert_id=${encodeURIComponent(`${sourceRefs.alert_id}-missing`)}&limit=20`, primaryToken);
  const noMatchRows = Array.isArray(noMatch.payload?.data) ? noMatch.payload.data : [];
  record('nonmatching source reference returns an authoritative empty result', noMatch.response.status === 200
    && noMatchRows.length === 0
    && noMatch.payload?.meta?.page?.total === 0,
  { status: noMatch.response.status, total: noMatch.payload?.meta?.page?.total });

  const crossTenant = await request('06-cross-tenant-isolation', 'GET', `/api/v1/pcap/jobs?alert_id=${encodeURIComponent(sourceRefs.alert_id)}&limit=20`, otherToken, {
    headers: { 'X-Tenant-ID': otherTenant },
  });
  const crossRows = Array.isArray(crossTenant.payload?.data) ? crossTenant.payload.data : [];
  record('another tenant cannot observe the source-linked task', crossTenant.response.status === 200
    && crossRows.length === 0
    && crossTenant.payload?.meta?.page?.total === 0,
  { status: crossTenant.response.status, tenant: otherTenant, total: crossTenant.payload?.meta?.page?.total });

  if (!jobId || !eventId) {
    throw new Error('PostgreSQL reconciliation skipped because the accepted command receipt is incomplete');
  }
  const dbRaw = postgres(`
    SELECT json_build_object(
      'tenant_id', t.tenant_id,
      'task_id', t.task_id,
      'alert_id', t.params->>'alert_id',
      'campaign_id', t.params->>'campaign_id',
      'baseline_id', t.params->>'baseline_id',
      'evidence_id', t.params->>'evidence_id',
      'evidence_type', t.params->>'evidence_type',
      'current_trace_id', t.last_trace_id,
      'create_trace_id', (SELECT h.trace_id FROM forensics_task_history h WHERE h.tenant_id=t.tenant_id AND h.task_id=t.task_id AND h.revision=1),
      'initial_history', (SELECT count(*) FROM forensics_task_history h WHERE h.tenant_id=t.tenant_id AND h.task_id=t.task_id AND h.revision=1 AND h.operation='create' AND h.action_id='forensics.pcap-cut.create' AND h.trace_id=${sqlLiteral(traceId)}),
      'initial_outbox', (SELECT count(*) FROM forensics_task_outbox o WHERE o.tenant_id=t.tenant_id AND o.task_id=t.task_id AND o.aggregate_version=1 AND o.event_id::text=${sqlLiteral(eventId)} AND o.trace_id=${sqlLiteral(traceId)}),
      'command_request', (SELECT count(*) FROM forensics_task_requests r WHERE r.tenant_id=t.tenant_id AND r.task_id=t.task_id AND r.idempotency_key=${sqlLiteral(idempotencyKey)} AND r.resulting_revision=1 AND r.trace_id=${sqlLiteral(traceId)}),
      'audit_record', (SELECT count(*) FROM audit_logs a WHERE a.tenant_id=t.tenant_id AND a.object_type='pcap_task' AND a.object_id=t.task_id::text AND a.action='forensics.pcap-cut.create' AND a.trace_id=${sqlLiteral(traceId)})
    )
    FROM tasks t
    WHERE t.tenant_id=${sqlLiteral(tenant)} AND t.task_id=${sqlLiteral(jobId)}::uuid;
  `);
  const db = JSON.parse(dbRaw);
  fs.writeFileSync(path.join(outputDir, '07-postgresql-reconciliation.json'), `${JSON.stringify(db, null, 2)}\n`, 'utf8');
  record('PostgreSQL preserves every source reference', db.tenant_id === tenant
    && db.task_id === jobId
    && Object.entries(sourceRefs).every(([key, value]) => db[key] === value), db);
  record('history, outbox, request and audit share the immutable create trace', db.create_trace_id === traceId
    && Number(db.initial_history) === 1
    && Number(db.initial_outbox) === 1
    && Number(db.command_request) === 1
    && Number(db.audit_record) === 1,
  { create_trace_id: db.create_trace_id, current_trace_id: db.current_trace_id, initial_history: db.initial_history, initial_outbox: db.initial_outbox, command_request: db.command_request, audit_record: db.audit_record });
} catch (error) {
  unexpectedError = error instanceof Error ? error.stack || error.message : String(error);
  record('acceptance execution completed', false, { error: unexpectedError });
}

const deployment = JSON.parse(kubectl([
  '-n', 'traffic-analysis', 'get', 'deploy', 'forensics-service', '-o', 'json',
]));
const pod = JSON.parse(kubectl([
  '-n', 'traffic-analysis', 'get', 'pod', '-l', 'app=forensics-service',
  '-o', 'json',
]));
const livePod = pod.items.find((item) => item.status?.containerStatuses?.[0]?.ready) || pod.items[0];
const result = checks.length > 0 && checks.every((check) => check.status === 'pass') ? 'pass' : 'fail';
const report = {
  schema_version: 1,
  run_id: runId,
  result,
  gate: 'G2_G3_FORENSICS_SOURCE_REFERENCE_WRITE_FILTER_RECONCILE',
  candidate: {
    image: deployment.spec?.template?.spec?.containers?.[0]?.image || '',
    image_id: livePod?.status?.containerStatuses?.[0]?.imageID || '',
    source_sha256: deployment.spec?.template?.metadata?.annotations?.['traffic.analysis/source-sha256'] || '',
    pod: livePod?.metadata?.name || '',
    ready: livePod?.status?.containerStatuses?.[0]?.ready === true,
    restarts: livePod?.status?.containerStatuses?.[0]?.restartCount ?? null,
  },
  tenant,
  other_tenant: otherTenant,
  job_id: jobId,
  event_id: eventId,
  source_refs: sourceRefs,
  trace_id: traceId,
  idempotency_key_sha256: crypto.createHash('sha256').update(idempotencyKey).digest('hex'),
  checks_total: checks.length,
  checks_passed: checks.filter((check) => check.status === 'pass').length,
  checks_failed: checks.filter((check) => check.status === 'fail').length,
  checks,
  artifacts: Object.keys(artifacts).map((name) => `${name}.json`),
  secret_material_redacted: true,
  unexpected_error: unexpectedError,
  captured_at: new Date().toISOString(),
};
fs.writeFileSync(path.join(outputDir, 'report.json'), `${JSON.stringify(report, null, 2)}\n`, 'utf8');
console.log(JSON.stringify({ result, output: path.relative(root, path.join(outputDir, 'report.json')), checks_total: report.checks_total, checks_passed: report.checks_passed, checks_failed: report.checks_failed, job_id: jobId }, null, 2));
if (result !== 'pass') process.exitCode = 1;

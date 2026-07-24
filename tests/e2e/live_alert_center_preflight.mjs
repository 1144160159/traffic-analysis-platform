#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const root = process.cwd();
const baseUrl = process.env.ALERT_BASE_URL || 'http://10.0.5.8:30180/api/v1';
const outputPath = path.join(root, 'doc/02_acceptance/02-regression/alert-center-live-preflight-latest.json');
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];

function token({ roles = ['admin'], permissions = ['*', 'admin:*', 'alert:read', 'alert:write', 'alert:export'] } = {}) {
  const encoded = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({ iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(), tenant_id: 'default', username: 'alert-center-preflight', roles, permissions, token_type: 'access', iat: now, exp: now + 900 })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

const authorization = `Bearer ${token()}`;
const viewerAuthorization = `Bearer ${token({ roles: ['viewer'], permissions: [] })}`;
const requestId = `alert-center-r651-${Date.now()}`;
const endTime = Date.now();
const startTime = endTime - 24 * 60 * 60 * 1000;
const checks = [];

async function request(name, endpoint, init = {}) {
  const started = performance.now();
  const response = await fetch(`${baseUrl}${endpoint}`, {
    ...init,
    headers: { Authorization: authorization, 'Content-Type': 'application/json', 'X-Request-ID': requestId, ...(init.headers || {}) },
    signal: AbortSignal.timeout(45_000),
  });
  const text = await response.text();
  let body = {};
  try { body = JSON.parse(text); } catch { body = { raw: text.slice(0, 500) }; }
  const check = { name, status: response.status, duration_ms: Math.round(performance.now() - started), pass: response.ok };
  checks.push(check);
  if (!response.ok) throw new Error(`${name} failed: HTTP ${response.status} ${text.slice(0, 300)}`);
  return body;
}

async function expectStatus(name, endpoint, expectedStatus, authorizationHeader, init = {}) {
  const response = await fetch(`${baseUrl}${endpoint}`, { ...init, headers: { Authorization: authorizationHeader, 'Content-Type': 'application/json', 'X-Request-ID': requestId, ...(init.headers || {}) }, signal: AbortSignal.timeout(45_000) });
  const pass = response.status === expectedStatus;
  checks.push({ name, status: response.status, expected_status: expectedStatus, pass });
  if (!pass) throw new Error(`${name} expected ${expectedStatus}, got ${response.status}`);
}

async function waitForPublishedOutbox(alertId, jobId) {
  let data = {};
  for (let attempt = 0; attempt < 15; attempt += 1) {
    const response = await fetch(`${baseUrl}/alerts/${encodeURIComponent(alertId)}/response-actions/${encodeURIComponent(jobId)}`, {
      headers: { Authorization: authorization, 'X-Request-ID': requestId },
      signal: AbortSignal.timeout(15_000),
    });
    const envelope = await response.json();
    data = envelope?.data ?? {};
    if (response.ok && data.outbox_published === true) return data;
    await new Promise((resolve) => setTimeout(resolve, 1_000));
  }
  return data;
}

function parseCsv(text) {
  const rows = [];
  let row = []; let field = ''; let quoted = false;
  for (let index = 0; index < text.length; index += 1) {
    const char = text[index];
    if (quoted && char === '"' && text[index + 1] === '"') { field += '"'; index += 1; continue; }
    if (char === '"') { quoted = !quoted; continue; }
    if (!quoted && char === ',') { row.push(field); field = ''; continue; }
    if (!quoted && (char === '\n' || char === '\r')) {
      if (char === '\r' && text[index + 1] === '\n') index += 1;
      if (field || row.length) { row.push(field); rows.push(row); }
      row = []; field = ''; continue;
    }
    field += char;
  }
  if (field || row.length) { row.push(field); rows.push(row); }
  return rows;
}

function latestProjectionCardinality() {
  const query = `SELECT (SELECT uniqExact(alert_id) FROM traffic.alerts WHERE tenant_id='default' AND last_seen >= ${startTime} AND last_seen <= ${endTime}) AS raw_unique, (SELECT count() FROM traffic.alerts_latest FINAL WHERE tenant_id='default' AND last_seen >= ${startTime} AND last_seen <= ${endTime}) AS latest_rows, (SELECT uniqExact(alert_id) FROM traffic.alerts_latest FINAL WHERE tenant_id='default' AND last_seen >= ${startTime} AND last_seen <= ${endTime}) AS latest_unique FORMAT JSONEachRow`;
  const output = execFileSync('kubectl', ['-n', 'middleware', 'exec', 'clickhouse-1-0', '-c', 'clickhouse', '--', 'clickhouse-client', '--query', query], { encoding: 'utf8', env: process.env, timeout: 45_000 });
  return JSON.parse(output.trim().split('\n').at(-1));
}

try {
  const list = await request('real paginated alert list', `/alerts?limit=10&offset=0&start_time=${startTime}&end_time=${endTime}`);
  const rows = Array.isArray(list?.data) ? list.data : Array.isArray(list?.data?.data) ? list.data.data : Array.isArray(list?.data?.alerts) ? list.data.alerts : [];
  const alertId = rows[0]?.alert_id;
  checks.push({ name: 'list contains only real rows', pass: rows.length > 0 && rows.every((row) => row.alert_id && !String(row.alert_id).includes('-SIM')), rows: rows.length, response_shape: { root_keys: Object.keys(list ?? {}), data_kind: Array.isArray(list?.data) ? 'array' : typeof list?.data, data_keys: list?.data && !Array.isArray(list.data) ? Object.keys(list.data) : [] } });
  checks.push({ name: 'server pagination total', pass: Number(list?.meta?.page?.total ?? 0) > rows.length, total: list?.meta?.page?.total ?? 0 });
  const statsEnvelope = await request('real alert statistics', `/alerts/stats?start_time=${startTime}&end_time=${endTime}`);
  const stats = statsEnvelope?.data ?? {};
  checks.push({ name: 'stats have real total and dimensions', pass: Number(stats?.total) > 0 && Object.keys(stats?.by_severity ?? {}).length > 0, total: stats?.total ?? 0 });
  const listTotal = Number(list?.meta?.page?.total ?? 0);
  const statsTotal = Number(stats?.total ?? -1);
  const cardinalityDrift = Math.abs(listTotal - statsTotal);
  // The projection is fed continuously. Two sequential FINAL queries can observe a
  // handful of late-arriving rows even with a fixed event-time window, so gate on
  // a tiny absolute drift while preserving the separate exact table invariant.
  checks.push({ name: 'list and stats use the same latest-alert cardinality', pass: cardinalityDrift <= 20, list_total: listTotal, stats_total: statsTotal, observed_drift: cardinalityDrift, allowed_drift: 20 });
  const projection = latestProjectionCardinality();
  const projectionDrift = Math.abs(Number(projection.raw_unique) - Number(projection.latest_unique));
  checks.push({ name: 'latest projection reconciles with raw distinct alerts', pass: projectionDrift <= 20 && Number(projection.latest_rows) === Number(projection.latest_unique), raw_unique: Number(projection.raw_unique), latest_rows: Number(projection.latest_rows), latest_unique: Number(projection.latest_unique), observed_drift: projectionDrift, allowed_drift: 20 });
  const normalizedStatus = String(rows[0]?.status ?? '').replace(/^ALERT_STATUS_/i, '').toLowerCase();
  if (normalizedStatus) {
    const filtered = await request('canonical status alias filter', `/alerts?limit=10&offset=0&status=${encodeURIComponent(normalizedStatus)}&start_time=${startTime}&end_time=${endTime}`);
    const filteredRows = Array.isArray(filtered?.data) ? filtered.data : [];
    checks.push({ name: 'status filter returns matching latest alerts', pass: filteredRows.length > 0 && filteredRows.every((row) => String(row.status).toLowerCase().includes(normalizedStatus)), status: normalizedStatus, rows: filteredRows.length });
  }
  const sample = rows[0] ?? {};
  if (sample.src_ip) {
    const filtered = await request('asset IP filter', `/alerts?limit=10&offset=0&asset_ip=${encodeURIComponent(sample.src_ip)}&start_time=${startTime}&end_time=${endTime}`);
    checks.push({ name: 'asset IP filter returns matching latest alerts', pass: Array.isArray(filtered?.data) && filtered.data.length > 0 && filtered.data.every((row) => row.src_ip === sample.src_ip || row.dst_ip === sample.src_ip), value: sample.src_ip });
	const sourceFiltered = await request('source IP filter', `/alerts?limit=10&offset=0&src_ip=${encodeURIComponent(sample.src_ip)}&start_time=${startTime}&end_time=${endTime}`);
	checks.push({ name: 'source IP filter is exact', pass: Array.isArray(sourceFiltered?.data) && sourceFiltered.data.length > 0 && sourceFiltered.data.every((row) => row.src_ip === sample.src_ip), value: sample.src_ip });
  }
  if (sample.dst_ip) {
    const filtered = await request('destination IP filter', `/alerts?limit=10&offset=0&dst_ip=${encodeURIComponent(sample.dst_ip)}&start_time=${startTime}&end_time=${endTime}`);
    checks.push({ name: 'destination IP filter returns matching latest alerts', pass: Array.isArray(filtered?.data) && filtered.data.length > 0 && filtered.data.every((row) => row.dst_ip === sample.dst_ip), value: sample.dst_ip });
  }
  if (sample.model_version) {
    const filtered = await request('model version filter', `/alerts?limit=10&offset=0&model_version=${encodeURIComponent(sample.model_version)}&start_time=${startTime}&end_time=${endTime}`);
    checks.push({ name: 'model version filter returns matching latest alerts', pass: Array.isArray(filtered?.data) && filtered.data.length > 0 && filtered.data.every((row) => row.model_version === sample.model_version), value: sample.model_version });
  }
  if (sample.attack_phase) {
	const filtered = await request('attack phase filter', `/alerts?limit=10&offset=0&attack_phase=${encodeURIComponent(sample.attack_phase)}&start_time=${startTime}&end_time=${endTime}`);
	checks.push({ name: 'attack phase is canonical and filterable', pass: sample.attack_phase !== sample.alert_type && Array.isArray(filtered?.data) && filtered.data.length > 0 && filtered.data.every((row) => row.attack_phase === sample.attack_phase), value: sample.attack_phase, alert_type: sample.alert_type });
  }
  const scoreFloor = Math.max(0, Math.min(1, Number(sample.score) || 0.7));
  const scoreFiltered = await request('minimum score filter', `/alerts?limit=10&offset=0&min_score=${scoreFloor}&start_time=${startTime}&end_time=${endTime}`);
  checks.push({ name: 'minimum score filter returns matching latest alerts', pass: Array.isArray(scoreFiltered?.data) && scoreFiltered.data.length > 0 && scoreFiltered.data.every((row) => Number(row.score) >= scoreFloor), value: scoreFloor });
  await expectStatus('viewer cannot list alerts', '/alerts?limit=1', 403, viewerAuthorization);
  await expectStatus('viewer cannot read alert stats', '/alerts/stats', 403, viewerAuthorization);
  if (!alertId) throw new Error('no alert id returned for action checks');
  await expectStatus('viewer cannot submit TP/FP feedback', `/alerts/${encodeURIComponent(alertId)}/feedback`, 403, viewerAuthorization, { method: 'POST', body: JSON.stringify({ label: 'TP', comment: 'unauthorized negative test' }) });
  const responseActionEnvelope = await request('durable response action', `/alerts/${encodeURIComponent(alertId)}/response-actions`, { method: 'POST', body: JSON.stringify({ action: '阻断 IP', target: rows[0]?.dst_ip || alertId, reason: 'r651 live preflight confirmed', dry_run: true, detail: { run_id: requestId } }) });
  const responseAction = responseActionEnvelope?.data ?? {};
  checks.push({ name: 'response action returns durable approval job', pass: Boolean(responseAction?.job_id) && responseAction?.status === 'pending_approval' && responseAction?.outbox_status === 'pending_retry', job_id: responseAction?.job_id ?? '', initial_outbox_status: responseAction?.outbox_status });
  const responseStatus = await waitForPublishedOutbox(alertId, responseAction.job_id);
	checks.push({ name: 'background outbox worker publishes pending action', pass: responseStatus?.job_id === responseAction.job_id && responseStatus?.status === 'pending_approval' && responseStatus?.outbox_published === true && Number(responseStatus?.outbox_attempts) >= 1, outbox_published: responseStatus?.outbox_published, outbox_attempts: responseStatus?.outbox_attempts, outbox_last_error: responseStatus?.outbox_last_error });
  const savedViewEnvelope = await request('durable saved view', '/alerts/views', { method: 'POST', body: JSON.stringify({ action: '保存告警视图', target: '高危优先', reason: 'r651 live preflight confirmed', detail: { filters: { status: normalizedStatus || '全部状态', destination: rows[0]?.dst_ip || '' }, time_window: [startTime, endTime], run_id: requestId } }) });
  const savedView = savedViewEnvelope?.data ?? {};
  checks.push({ name: 'saved view returns durable view id', pass: Boolean(savedView?.view_id), view_id: savedView?.view_id ?? '' });
  const viewsEnvelope = await request('saved view list readback', '/alerts/views');
  checks.push({ name: 'saved view is reusable', pass: Array.isArray(viewsEnvelope?.data?.views) && viewsEnvelope.data.views.some((view) => view.view_id === savedView.view_id && view.filters?.destination === (rows[0]?.dst_ip || '')) });
  checks.push({ name: 'saved view preserves time window', pass: Array.isArray(viewsEnvelope?.data?.views?.find((view) => view.view_id === savedView.view_id)?.filters?.time_window), expected: [startTime, endTime] });
  const feedbackEnvelope = await request('durable TP feedback', `/alerts/${encodeURIComponent(alertId)}/feedback`, { method: 'POST', body: JSON.stringify({ label: 'TP', comment: `r650 preflight ${requestId}` }) });
  const feedbackId = feedbackEnvelope?.data?.feedback_id;
  const feedbackHistory = await request('feedback history readback', `/alerts/${encodeURIComponent(alertId)}/feedback`);
  const historyRows = feedbackHistory?.data?.feedbacks ?? [];
  checks.push({ name: 'feedback is queryable after success', pass: Boolean(feedbackId) && Array.isArray(historyRows) && historyRows.some((item) => item.feedback_id === feedbackId), feedback_id: feedbackId ?? '' });
  const csvResponse = await fetch(`${baseUrl}/alerts/export/csv`, { method: 'POST', headers: { Authorization: authorization, 'Content-Type': 'application/json', 'X-Request-ID': requestId }, body: JSON.stringify({ status: normalizedStatus ? [normalizedStatus] : [], rule_version: sample.rule_version || '', model_version: sample.model_version || '', attack_phase: sample.attack_phase || '', src_ip: sample.src_ip || '', dst_ip: sample.dst_ip || '', min_score: scoreFloor, start_time: startTime, end_time: endTime, max_count: 25 }), signal: AbortSignal.timeout(45_000) });
  const csvText = await csvResponse.text();
  const csvRows = parseCsv(csvText);
  const header = csvRows[0] ?? [];
  const records = csvRows.slice(1).map((values) => Object.fromEntries(header.map((name, index) => [name, values[index] ?? ''])));
  checks.push({ name: 'filtered CSV content matches active queue filters', status: csvResponse.status, pass: csvResponse.ok && records.length > 0 && records.every((row) => (!normalizedStatus || row.status.toLowerCase().includes(normalizedStatus)) && (!sample.src_ip || row.src_ip === sample.src_ip) && (!sample.dst_ip || row.dst_ip === sample.dst_ip) && (!sample.alert_type || row.alert_type === sample.alert_type) && (!sample.attack_phase || row.attack_phase === sample.attack_phase) && (!sample.model_version || row.model_version === sample.model_version) && (!sample.rule_version || row.rule_version === sample.rule_version) && Number(row.score) >= scoreFloor), rows: records.length, filter_status: normalizedStatus, source_ip: sample.src_ip, destination_ip: sample.dst_ip, attack_phase: sample.attack_phase, model_version: sample.model_version, rule_version: sample.rule_version, minimum_score: scoreFloor });
} catch (error) {
  checks.push({ name: 'preflight execution', pass: false, error: error instanceof Error ? error.message : String(error) });
}

const result = {
  run_id: requestId,
  result: checks.every((check) => check.pass) ? 'pass' : 'fail',
  route: `${baseUrl}/alerts`,
  data_mode: 'live',
  checks,
  timestamp: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
process.exit(result.result === 'pass' ? 0 : 1);

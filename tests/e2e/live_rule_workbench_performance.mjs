#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { performance } from 'node:perf_hooks';

const root = process.cwd();
const baseUrl = 'http://10.0.5.8:30180';
const outputPath = path.join(root, 'evidence/ui-image-breakdowns/pages/rules/performance-r254-normal-api.json');
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

const encodedSecret = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
const secret = Buffer.from(encodedSecret, 'base64').toString('utf8');
const now = Math.floor(Date.now() / 1_000);
const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
const claims = Buffer.from(JSON.stringify({
  iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(), tenant_id: 'default',
  username: 'codex-rules-performance', roles: ['admin'], permissions: ['*', 'admin:*', 'rule:read', 'rule:write'], token_type: 'access', iat: now, exp: now + 1_800,
})).toString('base64url');
const input = `${header}.${claims}`;
const token = `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;

async function request(pathname, init = {}) {
  const started = performance.now();
  const response = await fetch(`${baseUrl}${pathname}`, {
    ...init,
    headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json', ...(init.headers ?? {}) },
  });
  const body = await response.json().catch(() => null);
  return { status: response.status, duration_ms: performance.now() - started, body };
}

function percentile(values, value) {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.max(0, Math.ceil(sorted.length * value) - 1)] ?? 0;
}

const seed = await request('/api/v1/rules?limit=1&offset=0');
if (seed.status !== 200 || !seed.body?.data?.[0]?.rule_id) throw new Error('cannot resolve rule for performance test');
const ruleId = seed.body.data[0].rule_id;
const reads = [];
const actions = [];
let serverErrors = 0;

for (let index = 0; index < 20; index += 1) {
  const list = await request(`/api/v1/rules?limit=7&offset=${index % 2 ? 7 : 0}`);
  const workbench = await request(`/api/v1/rules/${encodeURIComponent(ruleId)}/workbench`);
  reads.push(list.duration_ms, workbench.duration_ms);
  if (list.status >= 500 || workbench.status >= 500) serverErrors += Number(list.status >= 500) + Number(workbench.status >= 500);
}

for (let index = 0; index < 5; index += 1) {
  const action = await request(`/api/v1/rules/${encodeURIComponent(ruleId)}/actions`, {
    method: 'POST',
    body: JSON.stringify({ action_id: crypto.randomUUID(), action: 'rule-validate', target: `performance-proof-${index}`, payload: { source: 'live_rule_workbench_performance' } }),
  });
  actions.push(action.duration_ms);
  if (action.status !== 202) serverErrors += 1;
}

const requestCount = reads.length + actions.length;
const metrics = {
  read_samples: reads.length,
  action_samples: actions.length,
  read_p95_ms: Number(percentile(reads, 0.95).toFixed(3)),
  action_accept_p95_ms: Number(percentile(actions, 0.95).toFixed(3)),
  server_error_rate: serverErrors / requestCount,
};
const assertions = {
  read_p95_under_500ms: metrics.read_p95_ms < 500,
  action_accept_p95_under_1000ms: metrics.action_accept_p95_ms < 1_000,
  server_error_rate_under_0_001: metrics.server_error_rate <= 0.001,
};
const result = {
  result: Object.values(assertions).every(Boolean) ? 'pass' : 'fail',
  route: '/api/v1/rules',
  rule_id: ruleId,
  metrics,
  assertions,
  thresholds: { read_p95_ms: 500, action_accept_p95_ms: 1_000, server_error_rate: 0.001 },
  timestamp: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
process.exit(result.result === 'pass' ? 0 : 1);

#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const root = process.cwd();
const baseUrl = (process.env.UI_BASE_URL ?? 'http://10.0.5.8:30180').replace(/\/+$/, '');
const rounds = Number(process.env.ROUNDS ?? '10');
const limits = String(process.env.ATTACK_CHAIN_LIMITS ?? '8,100')
  .split(',')
  .map((value) => Number(value.trim()))
  .filter((value) => Number.isInteger(value) && value > 0 && value <= 100);
const outputPath = path.resolve(root, process.env.OUTPUT_PATH ?? '/tmp/attack-chain-list-performance.json');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
process.env.NO_PROXY = process.env.NO_PROXY || '127.0.0.1,localhost,10.0.5.8';

if (!Number.isInteger(rounds) || rounds < 1 || rounds > 100) throw new Error('ROUNDS must be between 1 and 100');
if (!limits.length) throw new Error('ATTACK_CHAIN_LIMITS must contain at least one integer from 1 to 100');

function createSmokeToken() {
  const encoded = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
    { encoding: 'utf8', env: process.env, timeout: 15_000 },
  );
  const now = Math.floor(Date.now() / 1_000);
  const b64url = (value) => Buffer.from(JSON.stringify(value)).toString('base64url');
  const header = b64url({ alg: 'HS256', typ: 'JWT' });
  const claims = b64url({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-attack-chain-performance',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:*', 'graph:read'],
    token_type: 'access',
    session_id: crypto.randomUUID(),
    iat: now,
    exp: now + 3_600,
  });
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

function percentile(values, fraction) {
  const sorted = [...values].sort((left, right) => left - right);
  return sorted[Math.max(0, Math.ceil(sorted.length * fraction) - 1)];
}

const token = createSmokeToken();
const cases = [];
for (const limit of limits) {
  const samples = [];
  const failures = [];
  let lastTotal = null;
  for (let round = 1; round <= rounds; round += 1) {
    const started = performance.now();
    const response = await fetch(`${baseUrl}/api/v1/attack-chains?limit=${limit}`, {
      headers: { Authorization: `Bearer ${token}`, 'X-Tenant-ID': 'default' },
    });
    const elapsedMs = performance.now() - started;
    const text = await response.text();
    let payload = null;
    try {
      payload = JSON.parse(text);
    } catch {
      payload = null;
    }
    const data = payload?.data ?? payload;
    const chains = Array.isArray(data?.chains) ? data.chains : [];
    lastTotal = Number(data?.total ?? lastTotal ?? 0);
    samples.push(elapsedMs);
    if (!response.ok || !payload || chains.length > limit || (limit <= lastTotal && chains.length !== limit)) {
      failures.push({
        round,
        status: response.status,
        elapsed_ms: Number(elapsedMs.toFixed(3)),
        json: Boolean(payload),
        returned: chains.length,
        body_prefix: payload ? null : text.slice(0, 120),
      });
    }
  }
  cases.push({
    limit,
    rounds,
    total: lastTotal,
    p50_ms: Number(percentile(samples, 0.50).toFixed(3)),
    p95_ms: Number(percentile(samples, 0.95).toFixed(3)),
    p99_ms: Number(percentile(samples, 0.99).toFixed(3)),
    max_ms: Number(Math.max(...samples).toFixed(3)),
    error_count: failures.length,
    failures,
  });
}

const report = {
  schema_version: 1,
  generated_at: new Date().toISOString(),
  base_url: baseUrl,
  endpoint: '/api/v1/attack-chains',
  tenant: 'default',
  auth_material_captured: false,
  cases,
  status: cases.every((item) => item.error_count === 0) ? 'PASS' : 'FAIL',
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify(report, null, 2));
process.exit(report.status === 'PASS' ? 0 : 1);

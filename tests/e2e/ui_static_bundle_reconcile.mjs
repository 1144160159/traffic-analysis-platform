#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const root = process.cwd();
const runId = process.env.RUN_ID || `web-ui-static-bundle-${Date.now()}`;
const outputDir = path.join(root, 'doc/02_acceptance/runs', runId);
const distRoot = path.join(root, 'web/ui/dist');
const runtimeOnly = new Set(['config.js', 'config.js.template']);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

function kubectl(args) {
  return execFileSync('kubectl', args, {
    cwd: root,
    env: process.env,
    encoding: 'utf8',
    timeout: 30_000,
    stdio: ['ignore', 'pipe', 'pipe'],
  }).trim();
}

function walk(directory, prefix = '') {
  const result = [];
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const relative = prefix ? `${prefix}/${entry.name}` : entry.name;
    const absolute = path.join(directory, entry.name);
    if (entry.isDirectory()) result.push(...walk(absolute, relative));
    else if (entry.isFile()) result.push({ path: relative, sha256: crypto.createHash('sha256').update(fs.readFileSync(absolute)).digest('hex') });
  }
  return result.sort((left, right) => left.path.localeCompare(right.path));
}

function parseShaList(value) {
  return value.split(/\r?\n/u).filter(Boolean).map((line) => {
    const match = line.match(/^([0-9a-f]{64})\s+\.\/(.+)$/u);
    if (!match) throw new Error(`unexpected sha256sum line: ${line}`);
    return { path: match[2], sha256: match[1] };
  }).sort((left, right) => left.path.localeCompare(right.path));
}

function listDigest(items) {
  const hash = crypto.createHash('sha256');
  for (const item of items) {
    hash.update(item.path);
    hash.update('\0');
    hash.update(item.sha256);
    hash.update('\0');
  }
  return hash.digest('hex');
}

fs.mkdirSync(outputDir, { recursive: false });
const deployment = JSON.parse(kubectl(['-n', 'traffic-analysis', 'get', 'deploy', 'web-ui', '-o', 'json']));
const podList = JSON.parse(kubectl(['-n', 'traffic-analysis', 'get', 'pod', '-l', 'app=web-ui', '-o', 'json']));
const pod = podList.items.find((item) => item.status?.containerStatuses?.[0]?.ready) || podList.items[0];
if (!pod) throw new Error('no web-ui pod found');

const localAll = walk(distRoot);
const local = localAll.filter((item) => !runtimeOnly.has(item.path));
const liveAll = parseShaList(kubectl([
  '-n', 'traffic-analysis', 'exec', pod.metadata.name, '--', 'sh', '-c',
  "cd /usr/share/nginx/html && find . -type f -exec sha256sum {} + | sort -k2",
]));
const live = liveAll.filter((item) => !runtimeOnly.has(item.path));
const localMap = new Map(local.map((item) => [item.path, item.sha256]));
const liveMap = new Map(live.map((item) => [item.path, item.sha256]));
const missingInLive = [...localMap].filter(([name]) => !liveMap.has(name)).map(([name]) => name);
const extraInLive = [...liveMap].filter(([name]) => !localMap.has(name)).map(([name]) => name);
const hashMismatch = [...localMap].filter(([name, sha]) => liveMap.get(name) && liveMap.get(name) !== sha).map(([name]) => name);
const runtimePaths = liveAll.filter((item) => runtimeOnly.has(item.path)).map((item) => item.path).sort();
const runtimeConfig = kubectl(['-n', 'traffic-analysis', 'exec', pod.metadata.name, '--', 'cat', '/usr/share/nginx/html/config.js']);
const runtimeConfigValid = runtimeConfig.includes('API_BASE_URL: "/api"')
  && runtimeConfig.includes('AUTH_ENABLED: "true"')
  && runtimeConfig.includes('USE_MOCK: "false"');
const checks = [
  { name: 'same relative file set', pass: missingInLive.length === 0 && extraInLive.length === 0, missing_in_live: missingInLive, extra_in_live: extraInLive },
  { name: 'same per-file SHA-256', pass: hashMismatch.length === 0, mismatched: hashMismatch },
  { name: 'only declared runtime files differ', pass: JSON.stringify(runtimePaths) === JSON.stringify(['config.js', 'config.js.template']), runtime_paths: runtimePaths },
  { name: 'runtime config is production-safe', pass: runtimeConfigValid },
  { name: 'live pod is clean and ready', pass: pod.status?.containerStatuses?.[0]?.ready === true && pod.status?.containerStatuses?.[0]?.restartCount === 0 },
];
const result = checks.every((check) => check.pass) ? 'pass' : 'fail';
const report = {
  schema_version: 1,
  run_id: runId,
  gate: 'G6_WEB_UI_STATIC_BUNDLE_RECONCILIATION',
  result,
  candidate: {
    image: deployment.spec?.template?.spec?.containers?.[0]?.image || '',
    source_sha256: deployment.spec?.template?.metadata?.annotations?.['traffic.analysis/source-sha256'] || '',
    image_id: pod.status?.containerStatuses?.[0]?.imageID || '',
    image_manifest_digest: deployment.spec?.template?.metadata?.annotations?.['traffic.analysis/image-manifest-digest'] || '',
    pod: pod.metadata.name,
  },
  local_dist_file_count: localAll.length,
  compared_file_count: local.length,
  live_static_file_count: liveAll.length,
  runtime_only_files: runtimePaths,
  local_list_sha256: listDigest(local),
  live_list_sha256: listDigest(live),
  checks,
  secret_material_redacted: true,
  captured_at: new Date().toISOString(),
};
fs.writeFileSync(path.join(outputDir, 'report.json'), `${JSON.stringify(report, null, 2)}\n`, 'utf8');
console.log(JSON.stringify({ result, output: path.relative(root, path.join(outputDir, 'report.json')), compared_file_count: local.length, local_list_sha256: report.local_list_sha256, live_list_sha256: report.live_list_sha256 }, null, 2));
if (result !== 'pass') process.exitCode = 1;

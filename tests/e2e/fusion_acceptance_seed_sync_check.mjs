#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const canonicalPath = path.join(root, 'common/sql/pg/07-fusion-acceptance.sql');
const manifestPath = path.join(root, 'deployments/kubernetes/init-jobs/07-fusion-acceptance-fixture.yaml');
const write = process.argv.includes('--write');

const read = (file) => fs.readFileSync(file, 'utf8').replace(/\r\n/g, '\n');
const indentYamlLiteral = (value) => value.split('\n').map((line) => `    ${line}`).join('\n');
const canonical = read(canonicalPath);
const manifest = read(manifestPath);
const startMarker = '  07-fusion-acceptance.sql: |\n';
const endMarker = '---\napiVersion: batch/v1\n';
const start = manifest.indexOf(startMarker);
const end = manifest.indexOf(endMarker, start + startMarker.length);
if (start < 0 || end < 0 || end <= start) {
  throw new Error('fusion acceptance ConfigMap markers are missing or out of order');
}
const expected = `${manifest.slice(0, start)}${startMarker}${indentYamlLiteral(canonical)}\n${manifest.slice(end)}`;
const requiredTokens = [
  "current_setting('traffic.enable_fusion_acceptance_fixture', true)",
  "current_setting('traffic.fusion_acceptance_tenant_id', true)",
  "current_setting('traffic.fusion_acceptance_fixture_action', true)",
  "'fusion-workbench-v1'",
  "'acceptance_fixture'",
  'generate_series(7, 26)',
  'LIMIT 18',
  'suspend: true',
  'name: cleanup-fusion-acceptance',
  'traffic.fusion_acceptance_fixture_action=cleanup',
];

if (write && manifest !== expected) fs.writeFileSync(manifestPath, expected, 'utf8');

const synchronized = write ? true : manifest === expected;
const tokenStatus = Object.fromEntries(requiredTokens.map((token) => [token, expected.includes(token)]));
const ok = synchronized && Object.values(tokenStatus).every(Boolean);
console.log(JSON.stringify({
  status: ok ? (write ? 'updated' : 'pass') : 'fail',
  mode: write ? 'write' : 'check',
  canonical: path.relative(root, canonicalPath),
  manifest: path.relative(root, manifestPath),
  synchronized,
  required_tokens: tokenStatus,
}, null, 2));
if (!ok) process.exitCode = 1;

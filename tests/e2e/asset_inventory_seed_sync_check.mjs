#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const canonicalPath = path.join(root, 'common/sql/pg/05-default-data.sql');
const kubernetesPath = path.join(root, 'deployments/kubernetes/init-jobs/02-postgres-schema.yaml');
const dockerPath = path.join(root, 'go/control-plane/deployments/docker/init/postgres_merged.sql');
const write = process.argv.includes('--write');

const read = (file) => fs.readFileSync(file, 'utf8');
const canonical = read(canonicalPath).replace(/\r\n/g, '\n');

const assetStart = '-- 资产台账验收数据：';
const canonicalEnd = '-- 默认 feature_set';
const assetStartIndex = canonical.indexOf(assetStart);
const canonicalEndIndex = canonical.indexOf(canonicalEnd);
if (assetStartIndex < 0 || canonicalEndIndex < 0 || canonicalEndIndex <= assetStartIndex) {
  throw new Error('canonical asset fixture markers are missing or out of order');
}
const canonicalAssetBlock = canonical.slice(assetStartIndex, canonicalEndIndex).trimEnd();

const replaceBetween = (source, startMarker, endMarker, replacement) => {
  const start = source.indexOf(startMarker);
  const end = source.indexOf(endMarker, start + startMarker.length);
  if (start < 0 || end < 0 || end <= start) {
    throw new Error(`markers are missing or out of order: ${startMarker} -> ${endMarker}`);
  }
  return `${source.slice(0, start)}${replacement}${source.slice(end)}`;
};

const indentYamlLiteral = (value) => value
  .split('\n')
  .map((line) => `    ${line}`)
  .join('\n');

const kubernetes = read(kubernetesPath).replace(/\r\n/g, '\n');
const expectedKubernetes = replaceBetween(
  kubernetes,
  '  05-default-data.sql: |\n',
  '  06-graph.sql: |\n',
  `  05-default-data.sql: |\n${indentYamlLiteral(canonical)}\n`,
);

const docker = read(dockerPath).replace(/\r\n/g, '\n');
const expectedDocker = replaceBetween(
  docker,
  assetStart,
  '-- 初始化 Graph Service 配置',
  `${canonicalAssetBlock}\n\n`,
);

const requiredTokens = [
  "current_setting('traffic.enable_asset_acceptance_fixture', true)",
  'generate_series(1, 10)',
  "'traffic_profile'",
  "'topology_graph'",
  "'network_interfaces'",
  "'business_domain'",
  "'discovery_timeline'",
  "'ticket_steps'",
  "'pcap_cut'",
  "'asset-inventory-acceptance'",
];

const targets = [
  { name: 'kubernetes-configmap', file: kubernetesPath, current: kubernetes, expected: expectedKubernetes },
  { name: 'docker-merged-init', file: dockerPath, current: docker, expected: expectedDocker },
];

const report = targets.map((target) => ({
  name: target.name,
  file: path.relative(root, target.file),
  synchronized: target.current === target.expected,
  required_tokens: Object.fromEntries(requiredTokens.map((token) => [token, target.expected.includes(token)])),
}));

if (write) {
  for (const target of targets) {
    if (target.current !== target.expected) fs.writeFileSync(target.file, target.expected, 'utf8');
  }
}

const ok = report.every((item) => (write || item.synchronized) && Object.values(item.required_tokens).every(Boolean));
console.log(JSON.stringify({
  status: ok ? 'pass' : write ? 'updated' : 'fail',
  canonical: path.relative(root, canonicalPath),
  mode: write ? 'write' : 'check',
  targets: report,
}, null, 2));

if (!write && !ok) process.exitCode = 1;

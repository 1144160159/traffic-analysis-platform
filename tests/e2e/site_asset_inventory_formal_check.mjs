#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const defaults = {
  input: process.env.SITE_ASSET_INVENTORY_JSON || '',
  outputJson: 'doc/02_acceptance/02-regression/asset-inventory-review/site-asset-inventory-formal-check-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/asset-inventory-review/site-asset-inventory-formal-check-latest.md',
  minAssets: '1',
};

const config = { ...defaults, ...parseArgs(process.argv.slice(2)) };
const root = process.cwd();
const forbiddenMarkerPattern = /\b(tbd|review-template|needs_site_owner_review|bootstrap)\b/i;

function parseArgs(argv) {
  const parsed = {};
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index];
    if (!item.startsWith('--')) throw new Error(`unexpected argument: ${item}`);
    const key = item.slice(2).replace(/-([a-z])/g, (_, char) => char.toUpperCase());
    const next = argv[index + 1];
    if (next === undefined || next.startsWith('--')) {
      parsed[key] = true;
    } else {
      parsed[key] = next;
      index += 1;
    }
  }
  return parsed;
}

function resolveRepo(file) {
  return path.isAbsolute(file) ? file : path.join(root, file);
}

function repoRel(file) {
  return path.relative(root, resolveRepo(file)).split(path.sep).join('/');
}

function ensureDirFor(file) {
  fs.mkdirSync(path.dirname(resolveRepo(file)), { recursive: true });
}

function norm(value) {
  return String(value ?? '').trim();
}

function normLower(value) {
  return norm(value).toLowerCase();
}

function normMac(value) {
  return normLower(value).replace(/[^0-9a-f]/g, '');
}

function hasForbiddenMarker(value) {
  return forbiddenMarkerPattern.test(String(value ?? ''));
}

function check(checks, phase, name, severity, passed, status, detail = '', artifact = '') {
  checks.push({ phase, name, severity, passed, status, detail, artifact });
}

function duplicateValues(rows, valueOf) {
  const byKey = new Map();
  for (const row of rows) {
    const value = valueOf(row);
    if (!value) continue;
    if (!byKey.has(value)) byKey.set(value, []);
    byKey.get(value).push(row.index);
  }
  return Array.from(byKey.entries())
    .filter(([, indexes]) => indexes.length > 1)
    .map(([value, indexes]) => ({ value, indexes }));
}

function renderMarkdown(summary) {
  const lines = [
    '# Site Asset Inventory Formal Check',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Input: \`${summary.input || 'missing'}\``,
    `- Assets: \`${summary.asset_count}\``,
    `- Passed: \`${summary.passed}/${summary.total}\``,
    `- Blockers: \`${summary.blockers}\``,
    `- Warnings: \`${summary.warnings}\``,
    '',
    'This check validates that a site asset inventory is formal evidence, not a bootstrap or review template. It does not approve an inventory; it only rejects files that cannot be used as formal coverage input.',
    '',
    '## Checks',
    '',
  ];
  for (const item of summary.checks) {
    lines.push(`- ${item.passed ? 'pass' : item.severity}: ${item.name} (${item.status}) ${item.detail || ''}`.trim());
  }
  lines.push('');
  return `${lines.join('\n')}\n`;
}

const checks = [];
const inputPath = norm(config.input);
const minAssets = Number(config.minAssets);
let payload = null;
let rawText = '';
let assets = [];
let normalizedAssets = [];
let objectPayload = false;

if (!inputPath) {
  check(checks, 'input', 'SITE_ASSET_INVENTORY_JSON is provided', 'blocker', false, 'missing', 'set --input or SITE_ASSET_INVENTORY_JSON');
} else if (!fs.existsSync(resolveRepo(inputPath))) {
  check(checks, 'input', 'SITE_ASSET_INVENTORY_JSON exists', 'blocker', false, 'missing', inputPath, inputPath);
} else {
  rawText = fs.readFileSync(resolveRepo(inputPath), 'utf8');
  try {
    payload = JSON.parse(rawText);
    check(checks, 'input', 'site inventory JSON parses', 'info', true, 'ok', inputPath, inputPath);
  } catch (error) {
    check(checks, 'input', 'site inventory JSON parses', 'blocker', false, 'invalid_json', error.message, inputPath);
  }
}

if (payload !== null) {
  objectPayload = Boolean(payload && typeof payload === 'object' && !Array.isArray(payload));
  assets = objectPayload ? payload.assets : payload;
  check(checks, 'schema', 'inventory is an object with an assets array', 'blocker', objectPayload && Array.isArray(assets), objectPayload ? (Array.isArray(assets) ? 'ok' : 'assets_missing') : 'not_object', 'formal inventory must include approval metadata next to assets');

  const reviewRequired = payload.review_required;
  check(checks, 'approval', 'review_required is false', 'blocker', reviewRequired === false, `review_required=${String(reviewRequired)}`, 'formal coverage cannot use a review-required packet');

  const approvalFields = ['approved_by', 'approved_at', 'approval_evidence'];
  for (const field of approvalFields) {
    const value = norm(payload[field]);
    check(checks, 'approval', `${field} is filled`, 'blocker', value.length > 0 && !hasForbiddenMarker(value), value ? 'filled' : 'missing', value ? '' : `${field} is required`);
  }
  const approvedAt = Date.parse(norm(payload.approved_at));
  check(checks, 'approval', 'approved_at is parseable', 'blocker', Number.isFinite(approvedAt), Number.isFinite(approvedAt) ? 'ok' : 'invalid_date', norm(payload.approved_at));

  const markersFound = hasForbiddenMarker(rawText);
  check(checks, 'approval', 'no draft markers remain in file', 'blocker', !markersFound, markersFound ? 'draft_marker_detected' : 'ok', 'blocked markers: TBD, review-template, needs_site_owner_review, bootstrap');

  if (Array.isArray(assets)) {
    normalizedAssets = assets.map((item, index) => ({
      index: index + 1,
      raw: item,
      asset_id: norm(item?.asset_id ?? item?.id),
      mac_address: norm(item?.mac_address ?? item?.mac),
      mac_key: normMac(item?.mac_address ?? item?.mac),
      ip_address: norm(item?.ip_address ?? item?.ip),
      ip_key: normLower(item?.ip_address ?? item?.ip),
      hostname: norm(item?.hostname ?? item?.name),
      hostname_key: normLower(item?.hostname ?? item?.name),
      expected_type: norm(item?.expected_type ?? item?.asset_type ?? item?.type),
      location: norm(item?.location),
    }));
  }

  check(checks, 'assets', 'asset count meets minimum', 'blocker', normalizedAssets.length >= minAssets, `asset_count=${normalizedAssets.length}`, `min_assets=${minAssets}`);

  const invalidRows = normalizedAssets
    .filter((item) => !item.asset_id || (!item.mac_key && !item.ip_key && !item.hostname_key) || !item.expected_type)
    .map((item) => item.index);
  check(checks, 'assets', 'every asset has id, identity key, and expected_type', 'blocker', invalidRows.length === 0, invalidRows.length ? 'invalid_rows' : 'ok', invalidRows.slice(0, 40).join(','));

  const markerRows = normalizedAssets
    .filter((item) => ['asset_id', 'mac_address', 'ip_address', 'hostname', 'expected_type', 'location'].some((field) => hasForbiddenMarker(item[field])))
    .map((item) => item.index);
  check(checks, 'assets', 'asset rows do not contain draft markers', 'blocker', markerRows.length === 0, markerRows.length ? 'draft_marker_rows' : 'ok', markerRows.slice(0, 40).join(','));

  const missingLocationRows = normalizedAssets.filter((item) => !item.location).map((item) => item.index);
  check(checks, 'assets', 'asset rows include location context', 'warn', missingLocationRows.length === 0, missingLocationRows.length ? 'missing_location' : 'ok', missingLocationRows.slice(0, 40).join(','));

  const duplicateAssetIds = duplicateValues(normalizedAssets, (item) => item.asset_id);
  const duplicateMacs = duplicateValues(normalizedAssets, (item) => item.mac_key);
  const duplicateIps = duplicateValues(normalizedAssets, (item) => item.ip_key);
  const duplicateHosts = duplicateValues(normalizedAssets, (item) => item.hostname_key);
  check(checks, 'assets', 'asset_id values are unique', 'blocker', duplicateAssetIds.length === 0, duplicateAssetIds.length ? 'duplicates' : 'ok', JSON.stringify(duplicateAssetIds.slice(0, 20)));
  check(checks, 'assets', 'mac_address values are unique when present', 'blocker', duplicateMacs.length === 0, duplicateMacs.length ? 'duplicates' : 'ok', JSON.stringify(duplicateMacs.slice(0, 20)));
  check(checks, 'assets', 'ip_address values are unique when present', 'warn', duplicateIps.length === 0, duplicateIps.length ? 'duplicates' : 'ok', JSON.stringify(duplicateIps.slice(0, 20)));
  check(checks, 'assets', 'hostname values are unique when present', 'warn', duplicateHosts.length === 0, duplicateHosts.length ? 'duplicates' : 'ok', JSON.stringify(duplicateHosts.slice(0, 20)));
}

const passed = checks.filter((item) => item.passed).length;
const blockers = checks.filter((item) => !item.passed && item.severity === 'blocker').length;
const warnings = checks.filter((item) => !item.passed && item.severity === 'warn').length;
const result = blockers > 0 ? 'blocked' : (warnings > 0 ? 'warn' : 'pass');
const summary = {
  package_id: 'site_asset_inventory_formal_check',
  result,
  generated_at: new Date().toISOString(),
  input: inputPath,
  asset_count: normalizedAssets.length,
  min_assets: minAssets,
  passed,
  total: checks.length,
  blockers,
  warnings,
  checks,
};

ensureDirFor(config.outputJson);
fs.writeFileSync(resolveRepo(config.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
ensureDirFor(config.outputMd);
fs.writeFileSync(resolveRepo(config.outputMd), renderMarkdown(summary), 'utf8');

console.log(`site-asset-inventory-formal-check result=${summary.result} checks=${summary.passed}/${summary.total} json=${repoRel(config.outputJson)} md=${repoRel(config.outputMd)}`);
if (result === 'blocked') process.exitCode = 1;

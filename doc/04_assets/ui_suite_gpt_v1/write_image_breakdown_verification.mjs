#!/usr/bin/env node

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.resolve(__dirname, '../../..');

function parseArgs() {
  const args = process.argv.slice(2);
  const out = {
    mainThreadJudgment: 'not-pixel-accepted',
    auxiliaryStatus: 'requested',
    auxiliaryAgent: 'Kierkegaard',
    auxiliaryNotes: [],
  };
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '--record') out.record = args[++i];
    else if (arg === '--main-thread-judgment') out.mainThreadJudgment = args[++i];
    else if (arg === '--auxiliary-status') out.auxiliaryStatus = args[++i];
    else if (arg === '--auxiliary-agent') out.auxiliaryAgent = args[++i];
    else if (arg === '--auxiliary-note') out.auxiliaryNotes.push(args[++i]);
    else throw new Error(`unknown argument: ${arg}`);
  }
  if (!out.record) throw new Error('usage: node write_image_breakdown_verification.mjs --record <breakdown.json>');
  return out;
}

function repoPath(file) {
  return path.isAbsolute(file) ? file : path.join(ROOT, file);
}

function repoRel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, '/');
}

function readJson(file, fallback = null) {
  const abs = repoPath(file);
  if (!fs.existsSync(abs)) return fallback;
  return JSON.parse(fs.readFileSync(abs, 'utf8'));
}

function writeJson(file, value) {
  const abs = repoPath(file);
  fs.mkdirSync(path.dirname(abs), { recursive: true });
  fs.writeFileSync(abs, `${JSON.stringify(value, null, 2)}\n`);
}

function exists(file) {
  return Boolean(file) && fs.existsSync(repoPath(file));
}

function defaultEvidencePath(record, filename) {
  return `evidence/ui-image-breakdowns/${record.category}/${record.id}/${filename}`;
}

function evidence(record, key, filename) {
  return record.evidence?.[key] || defaultEvidencePath(record, filename);
}

function metricRatio(metrics) {
  const value = metrics?.visual_diff?.pixel_mismatch_ratio ?? metrics?.pixel_mismatch_ratio ?? null;
  return typeof value === 'number' ? value : Number(value);
}

function metricMax(metrics) {
  const value = metrics?.visual_diff?.max_pixel_ratio ?? metrics?.max_pixel_ratio ?? 0.015;
  return typeof value === 'number' ? value : Number(value);
}

function openDifferences(record) {
  const out = new Set();
  for (const item of record.differences || []) {
    if (/unresolved|open|blocked|fail|pending|未解决|待解决|未关闭/i.test(String(item.status || ''))) {
      out.add(`${item.type || 'difference'}:${item.location || item.region || 'unknown'}`);
    }
  }
  for (const item of record.unresolved || []) out.add(String(item));
  return [...out];
}

function main() {
  const args = parseArgs();
  const record = readJson(args.record);
  if (!record) throw new Error(`record not found: ${args.record}`);

  const verificationPath = defaultEvidencePath(record, 'verification.json');
  const metricsPath = evidence(record, 'metrics', 'metrics.json');
  const metrics = readJson(metricsPath, {});
  const cdpVersion = readJson(evidence(record, 'cdp_version', 'cdp-version.json'), {});
  const cdpList = readJson(evidence(record, 'cdp_list', 'cdp-list.json'), []);
  const captureMeta = readJson(evidence(record, 'capture_meta', 'capture-meta.json'), {});
  const ratio = metricRatio(metrics);
  const maxRatio = metricMax(metrics);
  const evidencePaths = {
    target: evidence(record, 'target', 'target.png'),
    implementation: evidence(record, 'implementation', 'implementation.png'),
    diff: evidence(record, 'diff', 'diff.png'),
    regions_overlay: evidence(record, 'regions_overlay', 'regions-overlay.png'),
    metrics: metricsPath,
    capture_meta: evidence(record, 'capture_meta', 'capture-meta.json'),
    measurement: evidence(record, 'measurement', 'measurement.json'),
    text_ocr: evidence(record, 'text_ocr', 'text-ocr.txt'),
    verification: verificationPath,
  };
  const missingEvidence = Object.entries(evidencePaths)
    .filter(([key, value]) => key !== 'verification' && !exists(value))
    .map(([key]) => key);
  const unresolved = openDifferences(record);
  const diffPass = metrics?.status === 'pass' && Number.isFinite(ratio) && ratio <= maxRatio;
  const productionRouteOk =
    record.category !== 'pages' ||
    (record.evidence?.evidence_mode === 'production-route' && captureMeta?.evidence_mode === 'production-route');
  const accepted =
    args.mainThreadJudgment === 'pixel-accepted' &&
    diffPass &&
    productionRouteOk &&
    missingEvidence.length === 0 &&
    unresolved.length === 0 &&
    args.auxiliaryStatus === 'reviewed';
  const status = accepted ? 'pixel-accepted' : record.status || 'diff-pending';

  const verification = {
    generated_by: 'write_image_breakdown_verification.mjs',
    generated_at: new Date().toISOString(),
    id: record.id,
    category: record.category,
    source_image: record.source_image,
    status,
    accepted,
    main_thread_judgment: args.mainThreadJudgment,
    main_thread_decision_basis: accepted
      ? 'All evidence exists, Windows Chrome screenshot and visual diff pass, no open differences, auxiliary review completed, and page evidence uses production-route mode where required.'
      : 'Main thread rejects pixel acceptance until evidence is complete, pages use production-route screenshots, diff passes, auxiliary review is completed, and all open differences are closed.',
    browser: {
      backend: 'Windows Chrome CDP',
      cdp_url: 'http://127.0.0.1:9224',
      browser: cdpVersion.Browser || '',
      user_agent: cdpVersion['User-Agent'] || '',
      tabs_seen: Array.isArray(cdpList) ? cdpList.length : null,
    },
    viewport: metrics?.viewport || captureMeta?.viewport || record.evidence?.viewport || record.canvas || null,
    url: record.evidence?.url || captureMeta?.url || '',
    final_url: record.evidence?.final_url || captureMeta?.final_url || '',
    evidence_mode: record.evidence?.evidence_mode || captureMeta?.evidence_mode || '',
    production_route: {
      required: record.category === 'pages',
      ok: productionRouteOk,
      route: record.implementation?.route || record.route || '',
      resolved_path: record.implementation?.resolved_path || captureMeta?.resolved_path || '',
      route_state: record.implementation?.route_state || captureMeta?.route_state || {},
      runtime_status: captureMeta?.runtime_status || captureMeta?.status || '',
      runtime_reasons: captureMeta?.runtime_reasons || [],
    },
    evidence: evidencePaths,
    missing_evidence: missingEvidence,
    visual_diff: {
      status: metrics?.status || 'missing',
      pixel_mismatch_ratio: Number.isFinite(ratio) ? ratio : null,
      max_pixel_ratio: Number.isFinite(maxRatio) ? maxRatio : null,
      metrics: metricsPath,
      diff: evidencePaths.diff,
    },
    auxiliary_agent_review: {
      agent: args.auxiliaryAgent,
      status: args.auxiliaryStatus,
      notes: args.auxiliaryNotes,
    },
    differences: record.differences || [],
    ...(unresolved.length ? { unresolved } : {}),
    reproduction_steps: [
      'Check Windows Chrome CDP with curl http://127.0.0.1:9224/json/version and /json/list.',
      record.category === 'pages'
        ? 'Open the real APISIX/Web UI route in Windows Chrome through connectOverCDP(http://127.0.0.1:9224); do not load target.png as the implementation.'
        : 'Open the deterministic implementation URL in Windows Chrome through connectOverCDP(http://127.0.0.1:9224).',
      'Capture implementation.png at 1920x1080 with deviceScaleFactor 1.',
      'Generate diff.png and metrics.json against target.png.',
      'Review target.png, implementation.png, diff.png, regions-overlay.png, and this verification.json before main-thread judgment.',
    ],
  };

  writeJson(verificationPath, verification);
  record.evidence = {
    ...(record.evidence || {}),
    verification: repoRel(repoPath(verificationPath)),
  };
  if (accepted) {
    record.status = 'pixel-accepted';
    record.accepted = true;
    delete record.unresolved;
  }
  writeJson(args.record, record);

  console.log(
    JSON.stringify(
      {
        id: record.id,
        verification: repoRel(repoPath(verificationPath)),
        status,
        accepted,
        missing_evidence: missingEvidence,
        pixel_mismatch_ratio: Number.isFinite(ratio) ? ratio : null,
        unresolved: unresolved.length,
      },
      null,
      2,
    ),
  );
}

main();

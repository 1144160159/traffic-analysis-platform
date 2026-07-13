#!/usr/bin/env node

import fs from 'fs';
import path from 'path';
import { spawnSync } from 'child_process';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.resolve(__dirname, '../../..');
const BREAKDOWN_DIR = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns');

function parseArgs() {
  const args = process.argv.slice(2);
  const out = {};
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '--review') out.review = args[++i];
    else if (arg === '--category') out.category = args[++i];
    else throw new Error(`unknown argument: ${arg}`);
  }
  if (!out.review) throw new Error('usage: node apply_image_breakdown_agent_review.mjs --review <review-batch.json>');
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

function metricRatio(metrics) {
  const value = metrics?.visual_diff?.pixel_mismatch_ratio ?? metrics?.pixel_mismatch_ratio;
  return typeof value === 'number' ? value : Number(value);
}

function metricMax(metrics) {
  const value = metrics?.visual_diff?.max_pixel_ratio ?? metrics?.max_pixel_ratio ?? 0.015;
  return typeof value === 'number' ? value : Number(value);
}

function recordPathFor(item) {
  return path.join(BREAKDOWN_DIR, item.category, `${item.id}.json`);
}

function reviewPathFor(item) {
  return path.join(BREAKDOWN_DIR, item.category, `${item.id}.review.md`);
}

function run(command, args) {
  const result = spawnSync(command, args, {
    cwd: ROOT,
    encoding: 'utf8',
    maxBuffer: 1024 * 1024 * 64,
  });
  if (result.status !== 0) {
    const detail = [result.stdout, result.stderr].filter(Boolean).join('\n').trim();
    throw new Error(`${command} ${args.join(' ')} failed${detail ? `\n${detail}` : ''}`);
  }
  return result.stdout.trim();
}

function evidenceOk(record) {
  const required = ['target', 'implementation', 'diff', 'regions_overlay', 'metrics', 'measurement', 'text_ocr', 'verification', 'capture_meta'];
  const missing = required.filter((key) => !exists(record.evidence?.[key]));
  const metrics = readJson(record.evidence?.metrics, {});
  const capture = readJson(record.evidence?.capture_meta, {});
  const ratio = metricRatio(metrics);
  const maxRatio = metricMax(metrics);
  const problems = [];
  if (missing.length) problems.push(`missing evidence: ${missing.join(', ')}`);
  if (metrics?.status !== 'pass') problems.push(`metrics status is ${metrics?.status || 'missing'}`);
  if (!Number.isFinite(ratio) || ratio > maxRatio) problems.push(`pixel ratio ${ratio} exceeds ${maxRatio}`);
  if (record.category === 'pages') {
    if (record.evidence?.evidence_mode !== 'production-route') problems.push('page evidence mode is not production-route');
    if (capture?.evidence_mode !== 'production-route') problems.push('page capture evidence_mode is not production-route');
  }
  if (capture?.browser_backend !== 'Windows Chrome CDP') problems.push('capture backend is not Windows Chrome CDP');
  if (capture?.viewport?.width !== 1920 || capture?.viewport?.height !== 1080) problems.push('capture viewport is not 1920x1080');
  if (Math.abs(Number(capture?.device_pixel_ratio ?? 1) - 1) > 0.001) problems.push('capture DPR is not 1');
  if (capture?.has_vertical_scroll || capture?.has_horizontal_scroll) problems.push('capture reports scroll');
  if ((capture?.console_errors || []).length) problems.push('capture console errors exist');
  if ((capture?.page_errors || []).length) problems.push('capture page errors exist');
  if ((capture?.request_failures || []).length) problems.push('capture request failures exist');
  return { ok: problems.length === 0, problems, metrics, capture };
}

function appendFinalReview(reviewPath, batchRel, agentItem) {
  const abs = repoPath(reviewPath);
  const current = fs.existsSync(abs) ? fs.readFileSync(abs, 'utf8') : '';
  const marker = `Independent review batch: ${batchRel}`;
  if (current.includes(marker)) return;
  const findings = (agentItem.findings || []).length ? agentItem.findings.join('; ') : 'No rejecting findings in independent review.';
  const evidenceChecked = Array.isArray(agentItem.evidence_checked)
    ? agentItem.evidence_checked
    : Object.entries(agentItem.evidence_checked || {})
        .filter(([, value]) => Boolean(value))
        .map(([key]) => key);
  const section = `

## Independent Auxiliary Agent Review

- ${marker}
- Subagent status: ${agentItem.status}
- Evidence checked: ${evidenceChecked.join(', ')}
- Metric ratio: ${agentItem.metric_ratio}
- Findings: ${findings}
- Main-thread application: accepted after rechecking metrics, Windows Chrome capture metadata, evidence paths, and record completeness.
`;
  fs.writeFileSync(abs, `${current.trimEnd()}\n${section.trimEnd()}\n`);
}

function main() {
  const args = parseArgs();
  const reviewPath = repoPath(args.review);
  const batch = readJson(reviewPath, null);
  if (!batch) throw new Error(`review batch not found: ${args.review}`);
  if (batch.generated_by !== 'subagent-ui-aux-review') {
    throw new Error(`review batch generated_by must be subagent-ui-aux-review, got ${batch.generated_by}`);
  }
  const batchRel = repoRel(reviewPath);
  const applied = [];
  const rejected = [];
  for (const item of batch.items || []) {
    if (args.category && item.category !== args.category) continue;
    if (item.status !== 'reviewed') {
      rejected.push({ id: item.id, reason: `subagent status ${item.status}` });
      continue;
    }
    const recordPath = recordPathFor(item);
    const record = readJson(recordPath, null);
    if (!record) {
      rejected.push({ id: item.id, reason: 'record missing' });
      continue;
    }
    const evidence = evidenceOk(record);
    if (!evidence.ok) {
      rejected.push({ id: item.id, reason: evidence.problems.join('; ') });
      continue;
    }
    run('node', [
      'doc/04_assets/ui_suite_gpt_v1/write_image_breakdown_verification.mjs',
      '--record',
      repoRel(recordPath),
      '--main-thread-judgment',
      'pixel-accepted',
      '--auxiliary-status',
      'reviewed',
      '--auxiliary-agent',
      `independent-subagent:${batch.agent_id || batch.generated_by}`,
      '--auxiliary-note',
      `Independent review batch applied by main thread: ${batchRel}`,
    ]);
    const updatedRecord = readJson(recordPath);
    const verificationPath = updatedRecord.evidence?.verification;
    const verification = readJson(verificationPath, {});
    verification.independent_subagent_review = {
      status: 'applied',
      review_batch: batchRel,
      agent_id: batch.agent_id || '',
      item_status: item.status,
      findings: item.findings || [],
      screenshot_paths: item.screenshot_paths || {},
    };
    writeJson(verificationPath, verification);
    appendFinalReview(reviewPathFor(item), batchRel, item);
    applied.push({
      id: item.id,
      category: item.category,
      record: repoRel(recordPath),
      verification: verificationPath,
      metric_ratio: metricRatio(evidence.metrics),
    });
  }
  console.log(JSON.stringify({ applied_count: applied.length, rejected_count: rejected.length, applied, rejected }, null, 2));
}

main();

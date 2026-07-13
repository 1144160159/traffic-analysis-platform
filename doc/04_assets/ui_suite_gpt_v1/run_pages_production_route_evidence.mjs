#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.resolve(__dirname, '../../..');
const INDEX_PATH = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1/specs/pixel-perfect-breakdown-index.json');
const PAGES_MENU_QUEUE_PATH = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1/specs/pages-menu-order-queue.json');

function parseArgs() {
  const argv = process.argv.slice(2);
  const out = {
    category: 'pages',
    limit: 1,
    all: false,
    waitMs: 1800,
    baseUrl: 'http://10.0.5.8:30180',
    cdpUrl: 'http://127.0.0.1:9224',
    force: false,
    captureTimeoutMs: 180_000,
  };
  let limitExplicit = false;
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === '--image') out.image = argv[++index];
    else if (arg === '--category') out.category = argv[++index];
    else if (arg === '--limit') {
      out.limit = Number(argv[++index]);
      limitExplicit = true;
    }
    else if (arg === '--all') out.all = true;
    else if (arg === '--wait-ms') out.waitMs = Number(argv[++index]);
    else if (arg === '--base-url') out.baseUrl = argv[++index];
    else if (arg === '--cdp-url') out.cdpUrl = argv[++index];
    else if (arg === '--capture-timeout-ms') out.captureTimeoutMs = Number(argv[++index]);
    else if (arg === '--force') out.force = true;
    else throw new Error(`unknown argument: ${arg}`);
  }
  if (out.all && !limitExplicit) out.limit = Number.POSITIVE_INFINITY;
  return out;
}

function repoRel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, '/');
}

function repoPath(file) {
  return path.isAbsolute(file) ? file : path.join(ROOT, file);
}

function readJson(file, fallback = null) {
  const abs = repoPath(file);
  if (!fs.existsSync(abs)) return fallback;
  return JSON.parse(fs.readFileSync(abs, 'utf8'));
}

function run(command, args, { allowFailure = false, timeoutMs } = {}) {
  const result = spawnSync(command, args, {
    cwd: ROOT,
    encoding: 'utf8',
    maxBuffer: 1024 * 1024 * 128,
    timeout: timeoutMs,
  });
  if (result.status !== 0 && !allowFailure) {
    const detail = [result.stdout, result.stderr].filter(Boolean).join('\n').trim();
    throw new Error(`${command} ${args.join(' ')} failed${detail ? `\n${detail}` : ''}`);
  }
  return result;
}

function evidenceMode(record) {
  return record?.evidence?.evidence_mode || '';
}

function orderedItems(index, category) {
  const items = index.items.filter((item) => item.category === category);
  if (category !== 'pages') return items;
  const queue = readJson(PAGES_MENU_QUEUE_PATH, null);
  if (!queue?.items?.length) return items;
  const byId = new Map(items.map((item) => [item.id, item]));
  const ordered = [];
  const seen = new Set();
  for (const queueItem of queue.items) {
    const item = byId.get(queueItem.id);
    if (!item || seen.has(item.id)) continue;
    ordered.push(item);
    seen.add(item.id);
  }
  for (const item of items) {
    if (!seen.has(item.id)) ordered.push(item);
  }
  return ordered;
}

function selectItems(index, args) {
  let items = orderedItems(index, args.category);
  if (args.image) {
    items = items.filter((item) => item.id === args.image || item.source_image.endsWith(`/${args.image}`));
  }
  items = items.filter((item) => {
    if (args.force) return true;
    const record = readJson(item.json, null);
    return evidenceMode(record) !== 'production-route' || record?.status !== 'evidence-ready';
  });
  return items.slice(0, args.limit);
}

function main() {
  const args = parseArgs();
  const index = readJson(INDEX_PATH);
  if (!index?.items?.length) throw new Error(`missing index: ${repoRel(INDEX_PATH)}`);
  const selected = selectItems(index, args);
  const results = [];
  for (const [offset, item] of selected.entries()) {
    const marker = { index: offset + 1, total: selected.length, id: item.id, category: item.category };
    console.log(JSON.stringify({ event: 'start-production-route-capture', ...marker }));
    if (!fs.existsSync(repoPath(item.json))) {
      throw new Error(`breakdown json missing for ${item.id}: ${item.json}`);
    }
    const capture = run('node', [
      'doc/04_assets/ui_suite_gpt_v1/capture_image_breakdown_production_route.mjs',
      '--record',
      item.json,
      '--base-url',
      args.baseUrl,
      '--cdp-url',
      args.cdpUrl,
      '--wait-ms',
      String(args.waitMs),
    ], { allowFailure: true, timeoutMs: args.captureTimeoutMs });
    if (capture.status !== 0) {
      const failure = {
        ...marker,
        status: capture.error?.code === 'ETIMEDOUT' ? 'capture-timeout' : 'capture-failed',
        accepted: false,
        error: [capture.error?.message, capture.stderr, capture.stdout].filter(Boolean).join('\n').trim().slice(0, 4000),
      };
      results.push(failure);
      console.log(JSON.stringify({ event: 'finish-production-route-capture', ...failure }));
      continue;
    }
    const captureSummary = JSON.parse(String(capture.stdout || '{}'));
    const verification = run('node', [
      'doc/04_assets/ui_suite_gpt_v1/write_image_breakdown_verification.mjs',
      '--record',
      item.json,
      '--main-thread-judgment',
      'not-pixel-accepted',
      '--auxiliary-status',
      'requested',
      '--auxiliary-agent',
      'independent-subagent-required',
      '--auxiliary-note',
      'Production-route Windows Chrome evidence captured; awaiting independent review and main-thread closure.',
    ]);
    const verificationSummary = JSON.parse(String(verification.stdout || '{}'));
    const record = readJson(item.json, {});
    const result = {
      ...marker,
      status: record.status,
      accepted: record.accepted === true,
      route: record.implementation?.route || record.route || '',
      resolved_path: record.implementation?.resolved_path || '',
      implementation: record.evidence?.implementation || '',
      diff: record.evidence?.diff || '',
      metrics: record.evidence?.metrics || '',
      capture_meta: record.evidence?.capture_meta || '',
      verification: record.evidence?.verification || '',
      runtime_status: captureSummary.runtime_status,
      visual_diff_status: captureSummary.visual_diff_status,
      pixel_mismatch_ratio: captureSummary.pixel_mismatch_ratio,
      verification_status: verificationSummary.status,
    };
    results.push(result);
    console.log(JSON.stringify({ event: 'finish-production-route-capture', ...result }));
  }
  console.log(JSON.stringify({ processed: results.length, results }, null, 2));
}

main();

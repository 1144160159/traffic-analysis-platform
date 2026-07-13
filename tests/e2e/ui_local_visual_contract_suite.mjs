#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';

const defaults = {
  baseUrl: 'http://127.0.0.1:5173',
  visualAcceptance: 'doc/04_assets/ui_suite_gpt_v1/specs/visual-acceptance.json',
  outputDir: 'doc/02_acceptance/02-regression/ui-visual-interaction/local-dev',
  runId: '',
  waitMs: '2500',
  width: '1920',
  height: '1080',
  cdpUrl: '',
  smokeTokenEnv: 'DESKTOP_SMOKE_TOKEN',
  requireSmokeToken: false,
  targetIds: '',
};

const args = { ...defaults, ...parseArgs(process.argv.slice(2)) };
const root = process.cwd();
const iterationScript = path.join(root, 'tests/e2e/ui_local_visual_iteration.mjs');
const runId = args.runId || timestampRunId();
const runDir = resolveRepo(path.join(args.outputDir, runId));

function parseArgs(argv) {
  const parsed = {};
  const repeatedTargetIds = [];
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index];
    if (!item.startsWith('--')) throw new Error(`unexpected argument: ${item}`);
    const key = item.slice(2).replace(/-([a-z])/g, (_, char) => char.toUpperCase());
    const next = argv[index + 1];
    const value = next === undefined || next.startsWith('--') ? true : next;
    if (key === 'targetId') {
      repeatedTargetIds.push(String(value));
    } else {
      parsed[key] = value;
    }
    if (value !== true) index += 1;
  }
  if (repeatedTargetIds.length > 0) parsed.targetIds = repeatedTargetIds.join(',');
  return parsed;
}

function resolveRepo(file) {
  return path.isAbsolute(file) ? file : path.join(root, file);
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(resolveRepo(file), 'utf8'));
}

function timestampRunId() {
  const stamp = new Date().toISOString()
    .replace(/[-:]/g, '')
    .replace(/\.\d{3}Z$/, 'Z')
    .replace('T', '-');
  return `${stamp}-local-visual-contract-suite`;
}

function visualStatesOf(route) {
  if (Array.isArray(route.visualStates) && route.visualStates.length > 0) {
    return route.visualStates.map((state) => ({
      id: state.id || `${route.id}-${state.title || 'state'}`,
      routeId: route.id,
      route: route.route,
      query: state.query || '',
      sourceImage: state.sourceImage || '',
    }));
  }
  return [{
    id: route.id,
    routeId: route.id,
    route: route.route,
    query: '',
    sourceImage: route.sourceImage || '',
  }];
}

function uniqueList(values) {
  return [...new Set(values.filter(Boolean))];
}

function selectedTargets(states) {
  const requested = uniqueList(String(args.targetIds || '')
    .split(',')
    .map((value) => value.trim()));
  if (requested.length === 0) return states;
  const known = new Set(states.map((state) => state.id));
  const unknown = requested.filter((id) => !known.has(id));
  if (unknown.length > 0) {
    throw new Error(`unknown visual target id(s): ${unknown.join(', ')}`);
  }
  const allow = new Set(requested);
  return states.filter((state) => allow.has(state.id));
}

function truthy(value) {
  return ['1', 'true', 'yes', 'on'].includes(String(value).toLowerCase());
}

function runIteration(targetId) {
  const childArgs = [
    iterationScript,
    '--base-url', String(args.baseUrl),
    '--visual-acceptance', String(args.visualAcceptance),
    '--output-dir', String(args.outputDir),
    '--target-id', targetId,
    '--run-id', runId,
    '--wait-ms', String(args.waitMs),
    '--width', String(args.width),
    '--height', String(args.height),
    '--smoke-token-env', String(args.smokeTokenEnv),
  ];
  if (args.cdpUrl) childArgs.push('--cdp-url', String(args.cdpUrl));
  if (truthy(args.requireSmokeToken)) childArgs.push('--require-smoke-token');

  return new Promise((resolve) => {
    const startedAt = Date.now();
    const child = spawn(process.execPath, childArgs, {
      cwd: root,
      env: process.env,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (chunk) => {
      stdout += chunk.toString();
      process.stdout.write(chunk);
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk.toString();
      process.stderr.write(chunk);
    });
    child.on('close', (code, signal) => resolve({
      target_id: targetId,
      exit_code: code,
      signal,
      duration_ms: Date.now() - startedAt,
      stdout,
      stderr,
    }));
  });
}

function loadCaptureMeta(targetId) {
  const metaPath = path.join(runDir, targetId, 'capture-meta.json');
  if (!fs.existsSync(metaPath)) {
    return {
      target_id: targetId,
      meta_path: metaPath,
      missing_meta: true,
      failures: ['missing capture-meta.json'],
    };
  }
  const meta = JSON.parse(fs.readFileSync(metaPath, 'utf8'));
  const failures = [];
  if (meta.acceptance_eligible !== false) failures.push('local meta must be acceptance_eligible:false');
  if (meta.status !== 'local-dev-only') failures.push(`unexpected status ${meta.status}`);
  if (meta.viewport?.width !== Number(args.width) || meta.viewport?.height !== Number(args.height)) {
    failures.push(`viewport mismatch ${JSON.stringify(meta.viewport)}`);
  }
  if (meta.document_width !== Number(args.width)) failures.push(`document_width=${meta.document_width}`);
  if (meta.document_height !== Number(args.height)) failures.push(`document_height=${meta.document_height}`);
  if (meta.has_vertical_scroll) failures.push('has_vertical_scroll=true');
  if (meta.has_horizontal_scroll) failures.push('has_horizontal_scroll=true');
  if ((meta.console_errors || []).length > 0) failures.push(`console_errors=${meta.console_errors.length}`);
  if ((meta.page_errors || []).length > 0) failures.push(`page_errors=${meta.page_errors.length}`);
  if ((meta.request_failures || []).length > 0) failures.push(`request_failures=${meta.request_failures.length}`);
  if ((meta.server_errors || []).length > 0) failures.push(`server_errors=${meta.server_errors.length}`);
  if (!meta.screenshot || !fs.existsSync(meta.screenshot)) failures.push('missing screenshot');
  return {
    target_id: targetId,
    route_id: meta.route_id,
    meta_path: metaPath,
    screenshot: meta.screenshot,
    url: meta.url,
    final_url: meta.final_url,
    title: meta.title,
    browser_backend: meta.browser_backend,
    acceptance_eligible: meta.acceptance_eligible,
    document_width: meta.document_width,
    document_height: meta.document_height,
    has_vertical_scroll: meta.has_vertical_scroll,
    has_horizontal_scroll: meta.has_horizontal_scroll,
    console_errors: (meta.console_errors || []).length,
    page_errors: (meta.page_errors || []).length,
    request_failures: (meta.request_failures || []).length,
    server_errors: (meta.server_errors || []).length,
    failures,
  };
}

function writeSummary(summary) {
  fs.mkdirSync(runDir, { recursive: true });
  fs.writeFileSync(path.join(runDir, 'summary.json'), JSON.stringify(summary, null, 2) + '\n', 'utf8');
  const lines = [
    '# Local Visual Contract Suite',
    '',
    `- run_id: ${summary.run_id}`,
    `- status: ${summary.ok ? 'passed' : 'failed'}`,
    `- acceptance_eligible: ${summary.acceptance_eligible}`,
    `- base_url: ${summary.base_url}`,
    `- target_count: ${summary.target_count}`,
    `- failure_count: ${summary.failure_count}`,
    `- output_dir: ${summary.output_dir}`,
    '',
    '## Targets',
    '',
    '| target | route | backend | scroll | errors | result |',
    '|---|---|---|---|---|---|',
    ...summary.targets.map((target) => {
      const scroll = target.has_vertical_scroll || target.has_horizontal_scroll ? 'yes' : 'no';
      const errors = target.console_errors + target.page_errors + target.request_failures + target.server_errors;
      const result = target.failures.length === 0 ? 'pass' : `fail: ${target.failures.join('; ')}`;
      return `| ${target.target_id} | ${target.route_id || ''} | ${target.browser_backend || ''} | ${scroll} | ${errors} | ${result} |`;
    }),
    '',
  ];
  fs.writeFileSync(path.join(runDir, 'summary.md'), `${lines.join('\n')}\n`, 'utf8');
}

const visual = readJson(args.visualAcceptance);
const states = selectedTargets((Array.isArray(visual.routes) ? visual.routes : []).flatMap(visualStatesOf));
if (states.length === 0) throw new Error('no visual targets selected');

fs.mkdirSync(runDir, { recursive: true });
const iterationResults = [];
for (const state of states) {
  console.log(`[ui-local-visual-suite] target=${state.id}`);
  const result = await runIteration(state.id);
  iterationResults.push(result);
}

const targets = states.map((state) => {
  const meta = loadCaptureMeta(state.id);
  const iteration = iterationResults.find((result) => result.target_id === state.id);
  const failures = [...(meta.failures || [])];
  if (iteration?.exit_code !== 0) failures.push(`iteration_exit=${iteration?.exit_code ?? 'unknown'}`);
  if (iteration?.signal) failures.push(`iteration_signal=${iteration.signal}`);
  return {
    ...meta,
    target_id: state.id,
    route_id: meta.route_id || state.routeId,
    iteration_duration_ms: iteration?.duration_ms ?? null,
    failures: uniqueList(failures),
  };
});

const failedTargets = targets.filter((target) => target.failures.length > 0);
const summary = {
  ok: failedTargets.length === 0,
  status: failedTargets.length === 0 ? 'passed' : 'failed',
  acceptance_eligible: false,
  reason: 'Local Playwright capture is for frontend visual iteration only. Formal acceptance requires Windows Codex Desktop Chrome extension evidence.',
  run_id: runId,
  base_url: args.baseUrl,
  output_dir: runDir,
  visual_acceptance: resolveRepo(args.visualAcceptance),
  target_count: states.length,
  failure_count: failedTargets.length,
  targets,
};

writeSummary(summary);
console.log(JSON.stringify({
  ok: summary.ok,
  acceptance_eligible: false,
  run_id: runId,
  target_count: summary.target_count,
  failure_count: summary.failure_count,
  summary_json: path.join(runDir, 'summary.json'),
  summary_md: path.join(runDir, 'summary.md'),
}, null, 2));

if (!summary.ok) process.exitCode = 1;

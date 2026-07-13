#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

const defaults = {
  payloadJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.json',
  payloadJs: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.js',
  payloadSelftest: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-selftest-latest.json',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.md',
  toolName: 'mcp__codex_desktop_node_repl__js',
  timeoutMs: '900000',
  title: 'Traffic UI Desktop Chrome full capture: 30 visual + 28 interaction',
};

const args = { ...defaults, ...parseArgs(process.argv.slice(2)) };
const root = process.cwd();

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

function readJson(file) {
  return JSON.parse(fs.readFileSync(resolveRepo(file), 'utf8'));
}

function readText(file) {
  return fs.readFileSync(resolveRepo(file), 'utf8');
}

function sha256(value) {
  return crypto.createHash('sha256').update(value).digest('hex');
}

function parsesAsAsyncFunction(source) {
  try {
    new Function(`return (async () => {\n${source}\n});`);
    return { ok: true, error: '' };
  } catch (error) {
    return { ok: false, error: error.message };
  }
}

function secretScan(source) {
  const patterns = [
    /ANTHROPIC_AUTH_TOKEN/,
    /OPENAI_API_KEY/,
    /sk-proj-[A-Za-z0-9_-]+/,
    /sk-[A-Za-z0-9_-]{32,}/,
    /Bearer [A-Za-z0-9._-]{32,}/,
    /eyJ[A-Za-z0-9_-]{20,}/,
  ];
  return patterns.filter((pattern) => pattern.test(source)).map((pattern) => pattern.source);
}

function addCheck(checks, name, passed, detail, artifact = '') {
  checks.push({ name, passed, status: passed ? 'ok' : 'fail', detail, artifact });
}

function renderMarkdown(summary) {
  const lines = [
    '# Desktop Chrome Bridge Tool Call',
    '',
    `- Result: \`${summary.result}\``,
    `- Tool: \`${summary.tool_name}\``,
    `- Timeout: \`${summary.arguments.timeout_ms}\` ms`,
    `- Payload SHA256: \`${summary.payload.sha256}\``,
    `- Payload self-test: \`${summary.payload_selftest.result}\` (${summary.payload_selftest.passed}/${summary.payload_selftest.total})`,
    `- Visual targets: \`${summary.payload.visual_target_count}\``,
    `- Interaction targets: \`${summary.payload.interaction_target_count}\``,
    `- Receiver uploads: \`${summary.payload.receiver_upload_count}\``,
    '',
    '## Usage Boundary',
    '',
    'This file is the outer MCP call shape. Its `arguments.code` field is the inner JavaScript payload for the trusted Desktop Node REPL. Replace only the placeholder values inside the code before execution; do not convert the whole file into JavaScript and do not run it through shell Node.',
    '',
    '## Checks',
    '',
  ];
  for (const check of summary.checks) {
    lines.push(`- ${check.passed ? 'pass' : 'fail'}: ${check.name} (${check.detail})`);
  }
  lines.push('');
  return `${lines.join('\n')}\n`;
}

const payload = readJson(args.payloadJson);
const payloadSelftest = readJson(args.payloadSelftest);
const payloadJs = readText(args.payloadJs);
const syntax = parsesAsAsyncFunction(payloadJs);
const findings = secretScan(payloadJs);
const timeoutMs = Number(args.timeoutMs);
const toolCall = {
  tool: args.toolName,
  arguments: {
    code: payloadJs,
    timeout_ms: timeoutMs,
    title: args.title,
  },
};

const checks = [];
addCheck(checks, 'tool name targets Desktop Node REPL JS bridge', args.toolName === 'mcp__codex_desktop_node_repl__js', args.toolName);
addCheck(checks, 'payload summary requires Chrome extension backend', payload.backend_required === 'codex-desktop-chrome-extension', payload.backend_required || 'missing', repoRel(args.payloadJson));
addCheck(checks, 'payload forbids iab backend', payload.forbidden_backend === 'iab' && !payloadJs.includes("agent.browsers.get('iab')"), payload.forbidden_backend || 'missing');
addCheck(checks, 'payload self-test is pass', payloadSelftest.result === 'pass', `${payloadSelftest.result || 'missing'} ${payloadSelftest.passed || 0}/${payloadSelftest.total || 0}`, repoRel(args.payloadSelftest));
addCheck(checks, 'payload JS parses as async Desktop Node REPL code', syntax.ok, syntax.error || 'parse ok', repoRel(args.payloadJs));
addCheck(checks, 'tool-call code matches payload JS exactly', toolCall.arguments.code === payloadJs, `sha256=${sha256(payloadJs)}`);
addCheck(checks, 'tool-call keeps placeholders and does not embed runtime secrets', payloadJs.includes('<CODEX_CAPTURE_KEY>') && payloadJs.includes('<CODEX_SMOKE_NONCE>') && findings.length === 0, findings.length ? findings.join(', ') : 'placeholders only');
addCheck(checks, 'payload target URLs use Windows localhost tunnel endpoints', payloadJs.includes('http://127.0.0.1:25173') && payloadJs.includes('http://127.0.0.1:25174') && payloadJs.includes('http://127.0.0.1:25175') && !/"url_template":\s*"http:\/\/10\.0\.5\.8:30180/.test(payloadJs), 'requires 25173/25174/25175 and no direct APISIX url_template');
addCheck(checks, 'tool-call timeout is numeric and long enough for full capture', Number.isFinite(timeoutMs) && timeoutMs >= 600000, String(timeoutMs));

const passed = checks.filter((check) => check.passed).length;
const summary = {
  package_id: 'ui_desktop_chrome_bridge_tool_call',
  result: passed === checks.length ? 'pass' : 'fail',
  generated_at: new Date().toISOString(),
  tool_name: args.toolName,
  payload: {
    json: repoRel(args.payloadJson),
    js: repoRel(args.payloadJs),
    sha256: sha256(payloadJs),
    visual_target_count: payload.visual_target_count,
    interaction_target_count: payload.interaction_target_count,
    receiver_upload_count: payload.receiver_upload_count,
  },
  payload_selftest: {
    path: repoRel(args.payloadSelftest),
    result: payloadSelftest.result,
    passed: payloadSelftest.passed,
    total: payloadSelftest.total,
  },
  arguments: {
    timeout_ms: timeoutMs,
    title: args.title,
    code_placeholder_policy: 'contains <CODEX_CAPTURE_KEY> and <CODEX_SMOKE_NONCE> only; do not write concrete values to this artifact',
  },
  output_json: repoRel(args.outputJson),
  output_md: repoRel(args.outputMd),
  tool_call: toolCall,
  checks,
  passed,
  total: checks.length,
};

ensureDirFor(args.outputJson);
fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
ensureDirFor(args.outputMd);
fs.writeFileSync(resolveRepo(args.outputMd), renderMarkdown(summary), 'utf8');

console.log(`ui-desktop-chrome-bridge-tool-call result=${summary.result} passed=${passed}/${checks.length} json=${summary.output_json} md=${summary.output_md}`);
if (summary.result !== 'pass') process.exit(1);

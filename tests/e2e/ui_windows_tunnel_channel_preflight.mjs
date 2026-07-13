#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';

const defaults = {
  windowsHost: '10.3.6.59',
  windowsUser: 'LongShine',
  localAppUrl: 'http://127.0.0.1:5173/screen',
  localRuntimeConfigUrl: 'http://127.0.0.1:5173/src/config/runtime.ts',
  windowsAppUrl: 'http://127.0.0.1:25173/screen',
  windowsRuntimeConfigUrl: 'http://127.0.0.1:25173/src/config/runtime.ts',
  localReceiverHealthUrl: 'http://127.0.0.1:15174/health',
  localViewportProbeUrl: 'http://127.0.0.1:15174/viewport-probe',
  localRedirectHealthUrl: 'http://127.0.0.1:15175/health',
  windowsReceiverHealthUrl: 'http://127.0.0.1:25174/health',
  windowsViewportProbeUrl: 'http://127.0.0.1:25174/viewport-probe',
  windowsRedirectHealthUrl: 'http://127.0.0.1:25175/health',
  capturePlan: 'doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json',
  captureSession: 'doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-tunnel-channel-preflight-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-tunnel-channel-preflight-latest.md',
  timeoutMs: '8000',
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

function readJsonState(file) {
  const full = resolveRepo(file);
  if (!fs.existsSync(full)) return { exists: false, valid: false, data: null, error: 'missing' };
  try {
    return { exists: true, valid: true, data: JSON.parse(fs.readFileSync(full, 'utf8')), error: null };
  } catch (error) {
    return { exists: true, valid: false, data: null, error: error.message };
  }
}

function ensureDirFor(file) {
  fs.mkdirSync(path.dirname(resolveRepo(file)), { recursive: true });
}

function commandLine(parts) {
  return parts.map((part) => shellQuote(String(part))).join(' ');
}

function shellQuote(value) {
  if (/^[A-Za-z0-9_/:=.,@%+~-]+$/.test(value)) return value;
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

function runCommand(command, commandArgs, options = {}) {
  return new Promise((resolve) => {
    const startedAt = Date.now();
    const child = spawn(command, commandArgs, {
      cwd: root,
      env: process.env,
      stdio: ['ignore', 'pipe', 'pipe'],
      ...options,
    });
    let stdout = '';
    let stderr = '';
    let settled = false;
    const timeout = setTimeout(() => {
      if (!settled) child.kill('SIGTERM');
    }, Number(args.timeoutMs));
    child.stdout.on('data', (chunk) => {
      stdout += chunk.toString();
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk.toString();
    });
    child.on('close', (code, signal) => {
      settled = true;
      clearTimeout(timeout);
      resolve({
        code,
        signal,
        stdout: stdout.trim(),
        stderr: stderr.trim(),
        duration_ms: Date.now() - startedAt,
      });
    });
  });
}

async function curlStatus(url) {
  const result = await runCommand('curl', ['--noproxy', '*', '-s', '-o', '/dev/null', '-w', '%{http_code}', url]);
  return {
    url,
    command: commandLine(['curl', '--noproxy', '*', '-s', '-o', '/dev/null', '-w', '%{http_code}', url]),
    status: result.stdout,
    passed: result.code === 0 && result.stdout === '200',
    exit_code: result.code,
    stderr: result.stderr,
    duration_ms: result.duration_ms,
  };
}

async function curlText(url) {
  const result = await runCommand('curl', ['--noproxy', '*', '-fsSL', url]);
  return {
    url,
    command: commandLine(['curl', '--noproxy', '*', '-fsSL', url]),
    status: result.code === 0 ? '200' : '000',
    text: result.stdout,
    exit_code: result.code,
    stderr: result.stderr,
    duration_ms: result.duration_ms,
  };
}

async function windowsCurlStatus(url) {
  const remoteCommand = `curl.exe --noproxy * -s -o nul -w %{http_code} ${url}`;
  const sshArgs = [
    '-o', 'StrictHostKeyChecking=no',
    '-o', 'UserKnownHostsFile=/dev/null',
    '-o', 'PreferredAuthentications=password',
    '-o', 'PubkeyAuthentication=no',
    '-o', 'NumberOfPasswordPrompts=1',
    `${args.windowsUser}@${args.windowsHost}`,
    remoteCommand,
  ];
  const command = process.env.SSHPASS ? 'sshpass' : 'ssh';
  const commandArgs = process.env.SSHPASS ? ['-e', 'ssh', ...sshArgs] : sshArgs;
  const result = await runCommand(command, commandArgs);
  return {
    url,
    command: process.env.SSHPASS
      ? commandLine(['sshpass', '-e', 'ssh', '<ssh-options>', `${args.windowsUser}@${args.windowsHost}`, remoteCommand])
      : commandLine(['ssh', '<ssh-options>', `${args.windowsUser}@${args.windowsHost}`, remoteCommand]),
    status: result.stdout,
    passed: result.code === 0 && result.stdout === '200',
    exit_code: result.code,
    stderr: result.stderr,
    duration_ms: result.duration_ms,
  };
}

async function windowsCurlText(url) {
  const remoteCommand = `curl.exe --noproxy * -fsSL ${url}`;
  const sshArgs = [
    '-o', 'StrictHostKeyChecking=no',
    '-o', 'UserKnownHostsFile=/dev/null',
    '-o', 'PreferredAuthentications=password',
    '-o', 'PubkeyAuthentication=no',
    '-o', 'NumberOfPasswordPrompts=1',
    `${args.windowsUser}@${args.windowsHost}`,
    remoteCommand,
  ];
  const command = process.env.SSHPASS ? 'sshpass' : 'ssh';
  const commandArgs = process.env.SSHPASS ? ['-e', 'ssh', ...sshArgs] : sshArgs;
  const result = await runCommand(command, commandArgs);
  return {
    url,
    command: process.env.SSHPASS
      ? commandLine(['sshpass', '-e', 'ssh', '<ssh-options>', `${args.windowsUser}@${args.windowsHost}`, remoteCommand])
      : commandLine(['ssh', '<ssh-options>', `${args.windowsUser}@${args.windowsHost}`, remoteCommand]),
    status: result.code === 0 ? '200' : '000',
    text: result.stdout,
    exit_code: result.code,
    stderr: result.stderr,
    duration_ms: result.duration_ms,
  };
}

function runtimeValue(text, key) {
  const match = String(text || '').match(new RegExp(`${key}"?\\s*:\\s*"([^"]*)"`));
  return match ? match[1] : null;
}

function runtimeConfigStatus(result) {
  const runtime = {
    VITE_AUTH_ENABLED: runtimeValue(result.text, 'VITE_AUTH_ENABLED'),
    VITE_USE_MOCK: runtimeValue(result.text, 'VITE_USE_MOCK'),
    VITE_DESKTOP_SMOKE_TOKEN_ENABLED: runtimeValue(result.text, 'VITE_DESKTOP_SMOKE_TOKEN_ENABLED'),
  };
  const reasons = [];
  if (result.exit_code !== 0) reasons.push(`curl exit=${result.exit_code}`);
  if (runtime.VITE_AUTH_ENABLED !== 'true') reasons.push(`VITE_AUTH_ENABLED=${runtime.VITE_AUTH_ENABLED || 'missing'}`);
  if (runtime.VITE_USE_MOCK !== 'false') reasons.push(`VITE_USE_MOCK=${runtime.VITE_USE_MOCK || 'missing'}`);
  if (runtime.VITE_DESKTOP_SMOKE_TOKEN_ENABLED !== 'true') {
    reasons.push(`VITE_DESKTOP_SMOKE_TOKEN_ENABLED=${runtime.VITE_DESKTOP_SMOKE_TOKEN_ENABLED || 'missing'}`);
  }
  return {
    url: result.url,
    command: result.command,
    status: result.status,
    passed: reasons.length === 0,
    exit_code: result.exit_code,
    stderr: result.stderr,
    duration_ms: result.duration_ms,
    runtime,
    detail: reasons.length === 0 ? 'runtime config ready for protected-route smoke capture' : reasons.join('; '),
  };
}

function planStatus() {
  const state = readJsonState(args.capturePlan);
  if (!state.valid) {
    return {
      path: repoRel(args.capturePlan),
      exists: state.exists,
      valid: state.valid,
      passed: false,
      reason: state.error,
    };
  }
  const summary = state.data.summary || {};
  return {
    path: repoRel(args.capturePlan),
    exists: true,
    valid: true,
    passed: state.data.base_url === 'http://127.0.0.1:25173' && state.data.receiver_url === 'http://127.0.0.1:25174',
    base_url: state.data.base_url,
    receiver_url: state.data.receiver_url,
    visual_target_count: summary.visual_target_count ?? null,
    visual_passed_count: summary.visual_passed_count ?? null,
    visual_missing_or_failing_count: summary.visual_missing_or_failing_count ?? null,
    interaction_count: summary.interaction_count ?? null,
    interaction_passed_count: summary.interaction_passed_count ?? null,
    interaction_missing_or_failing_count: summary.interaction_missing_or_failing_count ?? null,
  };
}

function sessionStatus() {
  const state = readJsonState(args.captureSession);
  if (!state.valid) {
    return {
      path: repoRel(args.captureSession),
      exists: state.exists,
      valid: state.valid,
      passed: false,
      reason: state.error,
    };
  }
  const summary = state.data.summary || {};
  const commands = state.data.commands || {};
  return {
    path: repoRel(args.captureSession),
    exists: true,
    valid: true,
    passed: Boolean(commands.app_reverse_tunnel && commands.evidence_reverse_tunnel && commands.viewport_probe_open),
    status: state.data.status || null,
    visual_pending_count: summary.visual_pending_count ?? null,
    interaction_pending_count: summary.interaction_pending_count ?? null,
    receiver_selftest: state.data.sources?.receiver_selftest?.result || null,
    viewport_probe: state.data.sources?.viewport_probe?.result || null,
    has_app_reverse_tunnel_command: Boolean(commands.app_reverse_tunnel),
    has_evidence_reverse_tunnel_command: Boolean(commands.evidence_reverse_tunnel),
    has_viewport_probe_command: Boolean(commands.viewport_probe_open),
  };
}

function renderMarkdown(summary) {
  const lines = [
    '# Windows Tunnel Channel Preflight',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Windows host: \`${summary.windows.host}\``,
    `- Windows user: \`${summary.windows.user}\``,
    '',
    '## Endpoint Checks',
    '',
    '| scope | name | status | result | detail | url |',
    '|---|---|---:|---|---|---|',
  ];
  for (const check of summary.endpoint_checks) {
    lines.push(`| ${check.scope} | ${check.name} | ${check.status || 'n/a'} | ${check.passed ? 'pass' : 'fail'} | ${mdCell(check.detail || '')} | ${check.url} |`);
  }
  lines.push('');
  lines.push('## Formal Capture Package');
  lines.push('');
  lines.push(`- Capture plan: \`${summary.capture_plan.path}\` result=\`${summary.capture_plan.passed ? 'pass' : 'fail'}\``);
  lines.push(`- Capture session: \`${summary.capture_session.path}\` status=\`${summary.capture_session.status || 'n/a'}\``);
  lines.push(`- Visual pending: \`${summary.capture_session.visual_pending_count}\``);
  lines.push(`- Interaction pending: \`${summary.capture_session.interaction_pending_count}\``);
  lines.push('');
  lines.push('This preflight proves that the Windows-local URLs are reachable and the formal capture package is aligned to the tunnel. It does not replace Desktop Chrome extension screenshots or interaction evidence.');
  lines.push('');
  return `${lines.join('\n')}\n`;
}

function mdCell(value) {
  return String(value ?? '').replace(/\|/g, '\\|').replace(/\n/g, '<br>');
}

const endpointChecks = [
  { scope: 'linux', name: 'app', promise: curlStatus(args.localAppUrl) },
  { scope: 'linux', name: 'runtime-config', promise: curlText(args.localRuntimeConfigUrl).then(runtimeConfigStatus) },
  { scope: 'linux', name: 'receiver-health', promise: curlStatus(args.localReceiverHealthUrl) },
  { scope: 'linux', name: 'viewport-probe', promise: curlStatus(args.localViewportProbeUrl) },
  { scope: 'linux', name: 'redirect-health', promise: curlStatus(args.localRedirectHealthUrl) },
  { scope: 'windows', name: 'app', promise: windowsCurlStatus(args.windowsAppUrl) },
  { scope: 'windows', name: 'runtime-config', promise: windowsCurlText(args.windowsRuntimeConfigUrl).then(runtimeConfigStatus) },
  { scope: 'windows', name: 'receiver-health', promise: windowsCurlStatus(args.windowsReceiverHealthUrl) },
  { scope: 'windows', name: 'viewport-probe', promise: windowsCurlStatus(args.windowsViewportProbeUrl) },
  { scope: 'windows', name: 'redirect-health', promise: windowsCurlStatus(args.windowsRedirectHealthUrl) },
];

const checks = [];
for (const item of endpointChecks) {
  const result = await item.promise;
  checks.push({ scope: item.scope, name: item.name, ...result });
}

const capturePlan = planStatus();
const captureSession = sessionStatus();
const allPassed = checks.every((check) => check.passed) && capturePlan.passed && captureSession.passed;
const summary = {
  package_id: 'ui_windows_tunnel_channel_preflight',
  result: allPassed ? 'pass' : 'fail',
  generated_at: new Date().toISOString(),
  windows: {
    host: args.windowsHost,
    user: args.windowsUser,
  },
  endpoint_checks: checks,
  capture_plan: capturePlan,
  capture_session: captureSession,
  acceptance_effect: 'Readiness only. Formal acceptance still requires Codex Desktop Chrome extension capture metadata and interaction evidence.',
};

ensureDirFor(args.outputJson);
fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
ensureDirFor(args.outputMd);
fs.writeFileSync(resolveRepo(args.outputMd), renderMarkdown(summary), 'utf8');

console.log(JSON.stringify({
  ok: allPassed,
  result: summary.result,
  output_json: repoRel(args.outputJson),
  output_md: repoRel(args.outputMd),
}, null, 2));

if (!allPassed) process.exitCode = 1;

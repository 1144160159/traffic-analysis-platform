#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';

const defaults = {
  windowsHost: '10.3.6.59',
  windowsUser: 'LongShine',
  payloadSummary: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.json',
  tunnelPreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-tunnel-channel-preflight-latest.json',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-desktop-bridge-host-preflight-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-desktop-bridge-host-preflight-latest.md',
  timeoutMs: '10000',
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

function readJsonState(file) {
  const full = resolveRepo(file);
  if (!fs.existsSync(full)) return { exists: false, valid: false, data: null, error: 'missing' };
  try {
    return { exists: true, valid: true, data: JSON.parse(fs.readFileSync(full, 'utf8')), error: null };
  } catch (error) {
    return { exists: true, valid: false, data: null, error: error.message };
  }
}

function runCommand(command, commandArgs) {
  return new Promise((resolve) => {
    const startedAt = Date.now();
    const child = spawn(command, commandArgs, {
      cwd: root,
      env: process.env,
      stdio: ['ignore', 'pipe', 'pipe'],
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

function sshArgs(remoteCommand) {
  return [
    '-o', 'StrictHostKeyChecking=no',
    '-o', 'UserKnownHostsFile=/dev/null',
    '-o', 'PreferredAuthentications=password',
    '-o', 'PubkeyAuthentication=no',
    '-o', 'NumberOfPasswordPrompts=1',
    '-o', 'ConnectTimeout=8',
    `${args.windowsUser}@${args.windowsHost}`,
    remoteCommand,
  ];
}

async function runWindows(remoteCommand) {
  const command = process.env.SSHPASS ? 'sshpass' : 'ssh';
  const commandArgs = process.env.SSHPASS ? ['-e', 'ssh', ...sshArgs(remoteCommand)] : sshArgs(remoteCommand);
  const result = await runCommand(command, commandArgs);
  return {
    command: process.env.SSHPASS
      ? `sshpass -e ssh <ssh-options> ${args.windowsUser}@${args.windowsHost} <redacted-command>`
      : `ssh <ssh-options> ${args.windowsUser}@${args.windowsHost} <redacted-command>`,
    exit_code: result.code,
    stdout: result.stdout,
    stderr: sanitizeStderr(result.stderr),
    duration_ms: result.duration_ms,
    passed: result.code === 0,
  };
}

function sanitizeStderr(value) {
  return String(value || '').replace(/Warning: Permanently added .*known hosts\.\s*/g, '').trim();
}

function fileUrlToWindowsPath(value) {
  const text = String(value || '');
  if (!text.startsWith('file:///')) return '';
  return decodeURIComponent(text.slice('file:///'.length)).replace(/\//g, '\\');
}

function windowsPathToFileUrl(value) {
  const normalized = String(value || '').replace(/\\/g, '/');
  return normalized ? `file:///${encodeURI(normalized)}` : null;
}

function csvName(line) {
  const match = String(line || '').match(/^"([^"]+)"/);
  return match ? match[1].toLowerCase() : null;
}

function parseInventory(stdout) {
  const lines = String(stdout || '').split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
  const result = {
    user: null,
    user_profile: null,
    windows_version: null,
    codex_dirs: [],
    chrome_clients: [],
    processes: {
      chrome: 0,
      codex: 0,
      code: 0,
    },
  };
  for (const line of lines) {
    if (line.startsWith('user=')) result.user = line.slice('user='.length).trim();
    else if (line.startsWith('profile=')) result.user_profile = line.slice('profile='.length).trim();
    else if (line.startsWith('windows_version=')) result.windows_version = line.slice('windows_version='.length).trim();
    else if (line.startsWith('codex_dir=')) result.codex_dirs.push(line.slice('codex_dir='.length).trim());
    else if (line.startsWith('chrome_client=')) result.chrome_clients.push(line.slice('chrome_client='.length).trim());
    else {
      const name = csvName(line);
      if (name === 'chrome.exe') result.processes.chrome += 1;
      if (name === 'codex.exe') result.processes.codex += 1;
      if (name === 'code.exe') result.processes.code += 1;
    }
  }
  result.codex_dirs = [...new Set(result.codex_dirs)];
  result.chrome_clients = [...new Set(result.chrome_clients)];
  return result;
}

function renderMarkdown(summary) {
  const lines = [
    '# Windows Desktop Bridge Host Preflight',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Windows host: \`${summary.windows.host}\``,
    `- Windows user: \`${summary.windows.user}\``,
    `- SSH command: \`${summary.ssh.passed ? 'pass' : 'fail'}\``,
    `- Tunnel preflight: \`${summary.tunnel_preflight.result || 'missing'}\``,
    `- Chrome processes: \`${summary.inventory.processes.chrome}\``,
    `- Codex processes: \`${summary.inventory.processes.codex}\``,
    `- VSCode processes: \`${summary.inventory.processes.code}\``,
    `- Chrome client files discovered: \`${summary.inventory.chrome_clients.length}\``,
    `- Payload Chrome client exists on Windows: \`${summary.payload_chrome_client.exists}\``,
    `- Selected Chrome client URL: \`${summary.selected_chrome_client_url || 'none'}\``,
    '',
    'This preflight is intentionally read-only. It proves Windows host readiness boundaries, but it cannot prove that this Codex session exposes the Desktop Chrome MCP bridge.',
    '',
    '## Blockers',
    '',
  ];
  for (const blocker of summary.blockers) {
    lines.push(`- ${blocker}`);
  }
  if (summary.blockers.length === 0) lines.push('- none');
  lines.push('');
  lines.push('## Chrome Client Candidates');
  lines.push('');
  if (summary.inventory.chrome_clients.length === 0) {
    lines.push('- none discovered under `C:\\Users\\*\\.codex\\plugins\\cache\\openai-bundled\\chrome\\*\\scripts\\browser-client.mjs`');
  } else {
    for (const item of summary.inventory.chrome_clients) lines.push(`- \`${item}\``);
  }
  lines.push('');
  return `${lines.join('\n')}\n`;
}

const payloadState = readJsonState(args.payloadSummary);
const tunnelState = readJsonState(args.tunnelPreflight);
const payloadChromeClientUrl = payloadState.data?.chrome_client_url || '';
const payloadChromeClientPath = fileUrlToWindowsPath(payloadChromeClientUrl);

const inventoryCommand = [
  'cmd /v:on /c "',
  'echo user=%USERNAME%',
  '& echo profile=%USERPROFILE%',
  '& for /f "tokens=*" %V in (\'ver\') do @echo windows_version=%V',
  '& for /d %U in (C:\\Users\\*) do @if exist "%U\\.codex" echo codex_dir=%U\\.codex',
  '& for /d %U in (C:\\Users\\*) do @for /d %C in ("%U\\.codex\\plugins\\cache\\openai-bundled\\chrome\\*") do @if exist "%C\\scripts\\browser-client.mjs" echo chrome_client=%C\\scripts\\browser-client.mjs',
  '& tasklist /FI "IMAGENAME eq chrome.exe" /FO CSV /NH',
  '& tasklist /FI "IMAGENAME eq Codex.exe" /FO CSV /NH',
  '& tasklist /FI "IMAGENAME eq codex.exe" /FO CSV /NH',
  '& tasklist /FI "IMAGENAME eq Code.exe" /FO CSV /NH',
  '"',
].join(' ');

const payloadPathCommand = payloadChromeClientPath
  ? `cmd /c "if exist \\"${payloadChromeClientPath}\\" (echo exists) else (echo missing)"`
  : 'cmd /c "echo missing"';

const sshResult = await runWindows(inventoryCommand);
const payloadPathResult = await runWindows(payloadPathCommand);
const inventory = parseInventory(sshResult.stdout);
const normalizeWindowsPath = (value) => String(value || '').toLowerCase().replace(/\//g, '\\');
const payloadExists = payloadPathResult.stdout.includes('exists')
  || inventory.chrome_clients.some((candidate) => normalizeWindowsPath(candidate) === normalizeWindowsPath(payloadChromeClientPath));
const selectedChromeClientPath = inventory.chrome_clients[0] || (payloadExists ? payloadChromeClientPath : '');
const selectedChromeClientUrl = windowsPathToFileUrl(selectedChromeClientPath);

const blockers = [];
if (!sshResult.passed) blockers.push(`ssh inventory command failed: ${sshResult.stderr || `exit=${sshResult.exit_code}`}`);
if (inventory.processes.chrome <= 0) blockers.push('Windows Chrome process is not running');
if (inventory.processes.codex <= 0) blockers.push('Windows Codex process is not running');
if (inventory.processes.code <= 0) blockers.push('Windows VSCode process is not running');
if (!payloadExists && inventory.chrome_clients.length === 0) {
  blockers.push('No Windows Chrome browser-client.mjs was found for the Desktop bridge payload');
}
if (!tunnelState.valid || tunnelState.data?.result !== 'pass') {
  blockers.push(`Windows tunnel channel preflight is not passing: ${tunnelState.error || tunnelState.data?.result || 'missing'}`);
}

const summary = {
  package_id: 'ui_windows_desktop_bridge_host_preflight',
  result: blockers.length === 0 ? 'pass' : 'blocked',
  generated_at: new Date().toISOString(),
  windows: {
    host: args.windowsHost,
    user: args.windowsUser,
  },
  ssh: {
    command: sshResult.command,
    passed: sshResult.passed,
    exit_code: sshResult.exit_code,
    stderr: sshResult.stderr,
    duration_ms: sshResult.duration_ms,
  },
  inventory,
  payload_summary: {
    path: repoRel(args.payloadSummary),
    exists: payloadState.exists,
    valid: payloadState.valid,
  },
  payload_chrome_client: {
    url: payloadChromeClientUrl,
    windows_path: payloadChromeClientPath,
    exists: payloadExists,
  },
  selected_chrome_client_url: selectedChromeClientUrl,
  tunnel_preflight: {
    path: repoRel(args.tunnelPreflight),
    exists: tunnelState.exists,
    valid: tunnelState.valid,
    result: tunnelState.data?.result || null,
  },
  codex_session_bridge_tool_exposure: {
    status: 'not_verifiable_from_shell',
    required_tools: [
      'desktop_chrome_open_url',
      'desktop_chrome_list_tabs',
      'desktop_chrome_claim_url',
      'mcp__codex_desktop_node_repl__js',
    ],
  },
  blockers,
};

ensureDirFor(args.outputJson);
fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
ensureDirFor(args.outputMd);
fs.writeFileSync(resolveRepo(args.outputMd), renderMarkdown(summary), 'utf8');

console.log(`ui-windows-desktop-bridge-host-preflight result=${summary.result} chrome_clients=${inventory.chrome_clients.length} chrome_processes=${inventory.processes.chrome} codex_processes=${inventory.processes.codex} json=${repoRel(args.outputJson)} md=${repoRel(args.outputMd)}`);

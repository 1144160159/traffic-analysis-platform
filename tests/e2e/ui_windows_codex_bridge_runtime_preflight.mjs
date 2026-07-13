#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';

const defaults = {
  windowsHost: '10.3.6.59',
  windowsUser: 'LongShine',
  codexExe: 'C:\\Users\\LongShine\\AppData\\Local\\OpenAI\\Codex\\bin\\ea1c60319a1dcb19\\codex.exe',
  hostPreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-desktop-bridge-host-preflight-latest.json',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-codex-bridge-runtime-preflight-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-codex-bridge-runtime-preflight-latest.md',
  timeoutMs: '15000',
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

async function runWindows(label, remoteCommand) {
  const command = process.env.SSHPASS ? 'sshpass' : 'ssh';
  const commandArgs = process.env.SSHPASS ? ['-e', 'ssh', ...sshArgs(remoteCommand)] : sshArgs(remoteCommand);
  const result = await runCommand(command, commandArgs);
  return {
    label,
    command: process.env.SSHPASS
      ? `sshpass -e ssh <ssh-options> ${args.windowsUser}@${args.windowsHost} <${label}>`
      : `ssh <ssh-options> ${args.windowsUser}@${args.windowsHost} <${label}>`,
    exit_code: result.code,
    signal: result.signal,
    stdout: result.stdout,
    stderr: sanitizeStderr(result.stderr),
    duration_ms: result.duration_ms,
    passed: result.code === 0,
  };
}

function sanitizeStderr(value) {
  return sanitizeText(value);
}

function sanitizeText(value) {
  return String(value || '')
    .replace(/\b(pass(word)?|pwd)\s*[:=]\s*\S+/gi, '$1=<redacted>')
    .replace(/sk-proj-[A-Za-z0-9_-]+/g, '<redacted-openai-key>')
    .replace(/Bearer\s+[A-Za-z0-9._-]+/gi, 'Bearer <redacted>')
    .replace(/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g, '<redacted-jwt>')
    .replace(/\\\\\.\\pipe\\codex-computer-use-[0-9A-Fa-f-]+/g, '<redacted-native-pipe>')
    .replace(/codex-computer-use-[0-9A-Fa-f-]+/g, 'codex-computer-use-<redacted>')
    .replace(/Warning: Permanently added .*known hosts\.\s*/g, '')
    .trim();
}

function pushUnique(list, value) {
  const text = String(value || '').trim();
  if (text && !list.includes(text)) list.push(text);
}

function outputLines(result) {
  return String(result?.stdout || '').split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
}

function parseProcessCounts(tasklistText) {
  const counts = { chrome: 0, codex: 0, code: 0 };
  for (const line of outputLines({ stdout: tasklistText })) {
    const lower = line.toLowerCase();
    if (lower.startsWith('chrome.exe')) counts.chrome += 1;
    if (lower.startsWith('codex.exe')) counts.codex += 1;
    if (lower.startsWith('code.exe')) counts.code += 1;
  }
  return counts;
}

function parseNodeReplMcpConfig(stdout) {
  const text = String(stdout || '').trim();
  if (!text) {
    return {
      result: 'missing',
      enabled: null,
      transport_type: null,
      command_present: false,
      command_length: 0,
      env_keys: [],
      env_inventory: {},
      parse_error: null,
    };
  }
  try {
    const config = JSON.parse(text);
    const transport = config?.transport || {};
    const env = transport.env || {};
    return {
      result: 'pass',
      enabled: config?.enabled ?? null,
      transport_type: transport.type || null,
      command_present: typeof transport.command === 'string' && transport.command.length > 0,
      command_length: typeof transport.command === 'string' ? transport.command.length : 0,
      env_keys: Object.keys(env),
      env_inventory: Object.fromEntries(Object.entries(env).map(([key, value]) => [key, {
        present: typeof value === 'string' && value.length > 0,
        length: typeof value === 'string' ? value.length : 0,
      }])),
      parse_error: null,
    };
  } catch (error) {
    return {
      result: 'invalid_json',
      enabled: null,
      transport_type: null,
      command_present: false,
      command_length: 0,
      env_keys: [],
      env_inventory: {},
      parse_error: error.message,
      raw_prefix: sanitizeText(text).slice(0, 300),
    };
  }
}

function arr(value) {
  return Array.isArray(value) ? value : [];
}

function renderMarkdown(summary) {
  const runtime = summary.runtime || {};
  const lines = [
    '# Windows Codex Bridge Runtime Preflight',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Windows host: \`${summary.windows.host}\``,
    `- Windows user: \`${summary.windows.user}\``,
    `- Host preflight: \`${summary.host_preflight.result || 'missing'}\``,
    `- Codex commands: \`${arr(runtime.codex_commands).length}\``,
    `- Explicit Codex exists: \`${runtime.explicit_codex_exists}\``,
    `- Codex version lines: \`${arr(runtime.codex_version_lines).length}\``,
    `- Explicit Codex version lines: \`${arr(runtime.explicit_codex_version_lines).length}\``,
    `- MCP list lines: \`${arr(runtime.mcp_list_lines).length}\``,
    `- node_repl MCP config: \`${runtime.node_repl_mcp_config?.result || 'missing'}\``,
    `- node_repl MCP env keys: \`${arr(runtime.node_repl_mcp_config?.env_keys).length}\``,
    `- Chrome client files: \`${arr(runtime.chrome_clients).length}\``,
    `- Desktop bridge candidate dirs: \`${arr(runtime.desktop_bridge_candidate_dirs).length}\``,
    `- Node REPL candidate dirs: \`${arr(runtime.node_repl_candidate_dirs).length}\``,
    `- MCP/config marker files: \`${arr(runtime.mcp_config_files).length}\``,
    `- Chrome processes: \`${runtime.process_counts?.chrome ?? 0}\``,
    `- Codex processes: \`${runtime.process_counts?.codex ?? 0}\``,
    `- VSCode processes: \`${runtime.process_counts?.code ?? 0}\``,
    '',
    'This preflight is read-only. It lists paths, process counts, MCP server names, and node_repl MCP config shape only. It does not store config values, tokens, cookies, or browser state.',
    '',
    'It proves Windows-side runtime prerequisites. It still cannot prove that the current Linux Codex session exposes `desktop_chrome_*` or `mcp__codex_desktop_node_repl__js`.',
    '',
    '## Blockers',
    '',
  ];
  if (summary.blockers.length === 0) lines.push('- none');
  for (const blocker of summary.blockers) lines.push(`- ${blocker}`);
  lines.push('');
  lines.push('## Chrome Clients');
  lines.push('');
  if (arr(runtime.chrome_clients).length === 0) lines.push('- none');
  for (const item of arr(runtime.chrome_clients)) lines.push(`- \`${item}\``);
  lines.push('');
  lines.push('## Bridge Candidates');
  lines.push('');
  const candidates = [...arr(runtime.desktop_bridge_candidate_dirs), ...arr(runtime.node_repl_candidate_dirs)];
  if (candidates.length === 0) lines.push('- none');
  for (const item of [...new Set(candidates)].slice(0, 80)) lines.push(`- \`${item}\``);
  lines.push('');
  return `${lines.join('\n')}\n`;
}

const userRoot = `C:\\Users\\${args.windowsUser}`;
const commands = {
  identity: 'cmd /c "echo user=%USERNAME%&echo profile=%USERPROFILE%"',
  roots: `cmd /c "if exist ${userRoot}\\.codex echo codex_root_exists=true&if exist ${userRoot}\\.codex\\plugins\\cache echo plugin_cache_exists=true"`,
  codexCommand: 'cmd /c "where codex 2>nul"',
  codexVersion: 'cmd /c "codex --version 2>nul"',
  explicitCodexExists: `cmd /c "if exist ${args.codexExe} echo explicit_codex_exists=true"`,
  explicitCodexVersion: `cmd /c "${args.codexExe} --version 2>nul"`,
  mcpList: `cmd /c "${args.codexExe} mcp list 2>nul"`,
  nodeReplConfig: `cmd /c "${args.codexExe} mcp get node_repl --json 2>nul"`,
  chromeClients: 'cmd /v:on /c "for /d %U in (C:\\Users\\*) do @for /d %C in ("%U\\.codex\\plugins\\cache\\openai-bundled\\chrome\\*") do @if exist "%C\\scripts\\browser-client.mjs" echo chrome_client=%C\\scripts\\browser-client.mjs"',
  desktopCandidates: 'cmd /v:on /c "for /d %U in (C:\\Users\\*) do @for /f "tokens=*" %D in (\'dir /s /b /a:d "%U\\.codex\\plugins\\cache\\*desktop*" 2^>nul\') do @echo desktop_bridge_candidate=%D"',
  bridgeCandidates: 'cmd /v:on /c "for /d %U in (C:\\Users\\*) do @for /f "tokens=*" %B in (\'dir /s /b /a:d "%U\\.codex\\plugins\\cache\\*bridge*" 2^>nul\') do @echo desktop_bridge_candidate=%B"',
  nodeReplCandidates: 'cmd /v:on /c "for /d %U in (C:\\Users\\*) do @for /f "tokens=*" %N in (\'dir /s /b /a:d "%U\\.codex\\plugins\\cache\\*node*repl*" 2^>nul\') do @echo node_repl_candidate=%N"',
  mcpFiles: 'cmd /v:on /c "for /d %U in (C:\\Users\\*) do @for /f "tokens=*" %M in (\'dir /s /b /a:-d "%U\\.codex\\*mcp*" 2^>nul\') do @echo mcp_config_file=%M"',
  tasklist: 'cmd /c "tasklist"',
};

const hostState = readJsonState(args.hostPreflight);
const commandResults = {};
for (const [label, command] of Object.entries(commands)) {
  commandResults[label] = await runWindows(label, command);
}

const runtime = {
  user: null,
  user_profile: null,
  codex_root_exists: false,
  plugin_cache_exists: false,
  explicit_codex_path: args.codexExe,
  explicit_codex_exists: false,
  codex_commands: [],
  codex_version_lines: [],
  explicit_codex_version_lines: [],
  mcp_list_lines: [],
  node_repl_mcp_config: null,
  chrome_clients: [],
  desktop_bridge_candidate_dirs: [],
  node_repl_candidate_dirs: [],
  mcp_config_files: [],
  process_counts: parseProcessCounts(commandResults.tasklist.stdout),
};

for (const line of outputLines(commandResults.identity)) {
  if (line.startsWith('user=')) runtime.user = line.slice('user='.length).trim();
  if (line.startsWith('profile=')) runtime.user_profile = line.slice('profile='.length).trim();
}
for (const line of outputLines(commandResults.roots)) {
  if (line === 'codex_root_exists=true') runtime.codex_root_exists = true;
  if (line === 'plugin_cache_exists=true') runtime.plugin_cache_exists = true;
}
for (const line of outputLines(commandResults.explicitCodexExists)) {
  if (line === 'explicit_codex_exists=true') runtime.explicit_codex_exists = true;
}
for (const line of outputLines(commandResults.codexCommand)) pushUnique(runtime.codex_commands, line);
for (const line of outputLines(commandResults.codexVersion)) pushUnique(runtime.codex_version_lines, line);
for (const line of outputLines(commandResults.explicitCodexVersion)) pushUnique(runtime.explicit_codex_version_lines, sanitizeText(line));
for (const line of outputLines(commandResults.mcpList)) pushUnique(runtime.mcp_list_lines, sanitizeText(line));
runtime.node_repl_mcp_config = parseNodeReplMcpConfig(commandResults.nodeReplConfig.stdout);
for (const line of outputLines(commandResults.chromeClients)) {
  if (line.startsWith('chrome_client=')) pushUnique(runtime.chrome_clients, line.slice('chrome_client='.length));
}
for (const line of outputLines(commandResults.desktopCandidates)) {
  if (line.startsWith('desktop_bridge_candidate=')) pushUnique(runtime.desktop_bridge_candidate_dirs, line.slice('desktop_bridge_candidate='.length));
}
for (const line of outputLines(commandResults.bridgeCandidates)) {
  if (line.startsWith('desktop_bridge_candidate=')) pushUnique(runtime.desktop_bridge_candidate_dirs, line.slice('desktop_bridge_candidate='.length));
}
for (const line of outputLines(commandResults.nodeReplCandidates)) {
  if (line.startsWith('node_repl_candidate=')) pushUnique(runtime.node_repl_candidate_dirs, line.slice('node_repl_candidate='.length));
}
for (const line of outputLines(commandResults.mcpFiles)) {
  if (line.startsWith('mcp_config_file=')) pushUnique(runtime.mcp_config_files, line.slice('mcp_config_file='.length));
}

const blockers = [];
for (const [label, result] of Object.entries(commandResults)) {
  if (!result.passed && !['codexCommand', 'codexVersion', 'desktopCandidates', 'bridgeCandidates', 'nodeReplCandidates', 'mcpFiles'].includes(label)) {
    blockers.push(`Windows ${label} command failed: ${result.stderr || `exit=${result.exit_code}`}`);
  }
}
if (!hostState.valid || hostState.data?.result !== 'pass') {
  blockers.push(`Windows host preflight is not passing: ${hostState.error || hostState.data?.result || 'missing'}`);
}
if (!runtime.codex_root_exists) blockers.push('Windows .codex root is missing');
if (!runtime.plugin_cache_exists) blockers.push('Windows .codex plugin cache is missing');
if (!runtime.explicit_codex_exists) blockers.push(`Explicit Windows Codex executable is missing: ${args.codexExe}`);
if (runtime.node_repl_mcp_config?.result !== 'pass') blockers.push(`Windows node_repl MCP config is not readable through explicit codex.exe: ${runtime.node_repl_mcp_config?.result || 'missing'}`);
if (runtime.node_repl_mcp_config?.transport_type !== 'stdio') blockers.push(`Windows node_repl MCP transport is not stdio: ${runtime.node_repl_mcp_config?.transport_type || 'missing'}`);
if (arr(runtime.chrome_clients).length === 0) blockers.push('No Windows Chrome browser-client.mjs files were found');
if ((runtime.process_counts?.chrome ?? 0) <= 0) blockers.push('Windows Chrome process is not running');
if ((runtime.process_counts?.codex ?? 0) <= 0) blockers.push('Windows Codex process is not running');
if ((runtime.process_counts?.code ?? 0) <= 0) blockers.push('Windows VSCode process is not running');

const summary = {
  package_id: 'ui_windows_codex_bridge_runtime_preflight',
  result: blockers.length === 0 ? 'pass' : 'blocked',
  generated_at: new Date().toISOString(),
  windows: {
    host: args.windowsHost,
    user: args.windowsUser,
  },
  host_preflight: {
    path: repoRel(args.hostPreflight),
    exists: hostState.exists,
    valid: hostState.valid,
    result: hostState.data?.result || null,
  },
  command_results: Object.fromEntries(Object.entries(commandResults).map(([label, result]) => [label, {
    command: result.command,
    passed: result.passed,
    exit_code: result.exit_code,
    signal: result.signal,
    stderr: result.stderr,
    duration_ms: result.duration_ms,
  }])),
  runtime,
  codex_session_bridge_tool_exposure: {
    status: 'not_proven_by_windows_runtime_preflight',
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

console.log(`ui-windows-codex-bridge-runtime-preflight result=${summary.result} chrome_clients=${arr(runtime.chrome_clients).length} desktop_candidates=${arr(runtime.desktop_bridge_candidate_dirs).length} node_repl_candidates=${arr(runtime.node_repl_candidate_dirs).length} json=${repoRel(args.outputJson)} md=${repoRel(args.outputMd)}`);

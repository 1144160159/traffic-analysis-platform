#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

const defaults = {
  windowsHost: '10.3.6.59',
  windowsUser: 'LongShine',
  codexExe: 'C:\\Users\\LongShine\\AppData\\Local\\OpenAI\\Codex\\bin\\ea1c60319a1dcb19\\codex.exe',
  chromeClientUrl: 'file:///C:/Users/LongShine/.codex/plugins/cache/openai-bundled/chrome/latest/scripts/browser-client.mjs',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-node-repl-env-matrix-smoke-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-node-repl-env-matrix-smoke-latest.md',
  timeoutMs: '120000',
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

function sanitizeText(value) {
  return String(value || '')
    .replace(/\x1B(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1B\\))/g, '')
    .replace(/\x07/g, '')
    .replace(/\b(pass(word)?|pwd)\s*[:=]\s*\S+/gi, '$1=<redacted>')
    .replace(/sk-proj-[A-Za-z0-9_-]+/g, '<redacted-openai-key>')
    .replace(/Bearer\s+[A-Za-z0-9._-]+/gi, 'Bearer <redacted>')
    .replace(/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g, '<redacted-jwt>')
    .replace(/\\\\\.\\pipe\\codex-computer-use-[0-9A-Fa-f-]+/g, '<redacted-native-pipe>')
    .replace(/codex-computer-use-[0-9A-Fa-f-]+/g, 'codex-computer-use-<redacted>')
    .replace(/Warning: Permanently added .*known hosts\.\s*/g, '')
    .trim();
}

function ssh(remoteCommand, input = '', timeoutMs = Number(args.timeoutMs)) {
  if (!process.env.SSHPASS) {
    return {
      ok: false,
      status: 2,
      signal: null,
      stdout: '',
      stderr: 'SSHPASS is required via environment; it must not be written to files.',
      error: '',
    };
  }
  const result = spawnSync(
    'sshpass',
    [
      '-e',
      'ssh',
      '-o', 'StrictHostKeyChecking=no',
      '-o', 'UserKnownHostsFile=/dev/null',
      '-o', 'PreferredAuthentications=password',
      '-o', 'PubkeyAuthentication=no',
      '-o', 'NumberOfPasswordPrompts=1',
      '-o', 'ConnectTimeout=8',
      `${args.windowsUser}@${args.windowsHost}`,
      remoteCommand,
    ],
    {
      cwd: root,
      env: process.env,
      input,
      encoding: 'utf8',
      timeout: timeoutMs,
      maxBuffer: 20 * 1024 * 1024,
    },
  );
  return {
    ok: result.status === 0,
    status: result.status ?? null,
    signal: result.signal,
    stdout: String(result.stdout || ''),
    stderr: sanitizeText(String(result.stderr || '')),
    error: result.error ? result.error.message : '',
  };
}

function parseJsonLines(stdout) {
  return String(stdout || '')
    .split(/\r?\n/)
    .filter(Boolean)
    .map((line) => {
      try {
        return JSON.parse(line);
      } catch (error) {
        return { parse_error: error.message, raw_prefix: sanitizeText(line).slice(0, 200) };
      }
    });
}

function mcpMessages(code, title, timeoutMs) {
  return [
    {
      jsonrpc: '2.0',
      id: 1,
      method: 'initialize',
      params: {
        protocolVersion: '2025-06-18',
        capabilities: {},
        clientInfo: { name: 'ui-windows-node-repl-env-matrix-smoke', version: '1' },
      },
    },
    { jsonrpc: '2.0', method: 'notifications/initialized', params: {} },
    { jsonrpc: '2.0', id: 2, method: 'tools/list', params: {} },
    {
      jsonrpc: '2.0',
      id: 3,
      method: 'tools/call',
      params: {
        name: 'js',
        arguments: { code, timeout_ms: timeoutMs, title },
      },
    },
  ];
}

function inputFor(messages) {
  return `${messages.map((message) => JSON.stringify(message)).join('\n')}\n`;
}

function responseById(messages, id) {
  return messages.find((message) => message.id === id) || null;
}

function toolNames(toolsResponse) {
  const tools = toolsResponse?.result?.tools;
  return Array.isArray(tools) ? tools.map((tool) => tool.name).filter(Boolean) : [];
}

function callText(callResponse) {
  const content = callResponse?.result?.content;
  if (!Array.isArray(content)) return '';
  return sanitizeText(content.map((item) => (typeof item?.text === 'string' ? item.text : '')).join('\n'));
}

function parseCallJson(callResponse) {
  const text = callText(callResponse);
  try {
    return JSON.parse(text);
  } catch {
    return text ? { text_prefix: text.slice(0, 500) } : null;
  }
}

function classify(callData) {
  const error = String(callData?.error || callData?.text_prefix || '');
  if (callData?.ok === true && callData.backend === 'extension') return 'extension_ready';
  if (callData?.ok === true) return 'pass';
  if (/browser-client is not trusted|privileged native pipe bridge is not available/i.test(error)) {
    return 'native_pipe_or_trust_unavailable';
  }
  if (/helper_firewall_rule_create_or_add_failed|SetRemotePorts|HRESULT\(0x80070005\)|拒绝访问/i.test(error)) {
    return 'sandbox_firewall_denied';
  }
  if (/timeout|timed out|ETIMEDOUT/i.test(error)) return 'timeout';
  if (error) return 'js_error';
  return 'missing_result';
}

function remoteNodeReplCommand(nodeReplCommand, env, envKeys) {
  if (envKeys.length === 0) return `"${nodeReplCommand}"`;
  const setCommands = envKeys
    .filter((key) => typeof env[key] === 'string' && env[key].length > 0)
    .map((key) => `set ${key}=${env[key]}`);
  return `cmd /d /c "${setCommands.join('&&')}&&${nodeReplCommand}"`;
}

function runMcpCase(nodeReplCommand, env, envKeys, code, title, timeoutMs = 60000) {
  const command = remoteNodeReplCommand(nodeReplCommand, env, envKeys);
  const result = ssh(command, inputFor(mcpMessages(code, title, timeoutMs)), timeoutMs + 45000);
  const messages = parseJsonLines(result.stdout);
  const tools = toolNames(responseById(messages, 2));
  const call = responseById(messages, 3);
  const data = parseCallJson(call);
  return {
    ssh_exit_code: result.status,
    signal: result.signal,
    stderr: sanitizeText(result.stderr),
    tools,
    is_error: Boolean(call?.result?.isError),
    data,
    classification: classify(data),
  };
}

function renderMarkdown(summary) {
  const lines = [
    '# Windows Node REPL Env Matrix Smoke',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Windows host: \`${summary.windows.host}\``,
    `- MCP config: \`${summary.mcp_config.result}\``,
    `- Matrix cases: \`${summary.matrix.length}\``,
    `- JS pass cases: \`${summary.counts.js_pass_cases}\``,
    `- Chrome extension-ready cases: \`${summary.counts.chrome_extension_ready_cases}\``,
    '',
    'This matrix tries selected node_repl environment subsets over SSH stdio. It records environment key names only, never values. It does not upload screenshots or close visual acceptance.',
    '',
    '## Matrix',
    '',
  ];
  for (const item of summary.matrix) {
    lines.push(`- \`${item.name}\`: js=\`${item.direct.classification}\` chrome=\`${item.chrome?.classification || 'skipped'}\` env_keys=\`${item.env_keys.join(',') || 'none'}\``);
  }
  lines.push('', '## Blockers', '');
  if (summary.blockers.length === 0) lines.push('- none');
  for (const blocker of summary.blockers) lines.push(`- ${blocker}`);
  lines.push('');
  return `${lines.join('\n')}\n`;
}

const configProbe = ssh(`"${args.codexExe}" mcp get node_repl --json`);
let config = null;
try {
  config = JSON.parse(configProbe.stdout);
} catch {
  config = null;
}
const transport = config?.transport || {};
const env = transport.env || {};
const nodeReplCommand = transport.command || 'C:\\Users\\LongShine\\AppData\\Local\\OpenAI\\Codex\\runtimes\\cua_node\\1b23c930bdf84ed6\\bin\\node_repl.exe';
const availableEnvKeys = new Set(Object.keys(env));
const pick = (keys) => keys.filter((key) => availableEnvKeys.has(key));

const envCases = [
  { name: 'none', keys: [] },
  { name: 'trusted-browser-client', keys: pick(['NODE_REPL_TRUSTED_BROWSER_CLIENT_SHA256S']) },
  { name: 'trusted-code', keys: pick(['NODE_REPL_TRUSTED_CODE_PATHS']) },
  { name: 'native-pipe', keys: pick(['SKY_CUA_NATIVE_PIPE', 'SKY_CUA_NATIVE_PIPE_DIRECTORY']) },
  { name: 'browser-backend-metadata', keys: pick(['BROWSER_USE_AVAILABLE_BACKENDS', 'BROWSER_USE_CODEX_APP_VERSION', 'BROWSER_USE_CODEX_APP_BUILD_FLAVOR']) },
  { name: 'codex-context', keys: pick(['CODEX_HOME', 'CODEX_CLI_PATH']) },
  { name: 'node-paths', keys: pick(['NODE_REPL_NODE_PATH', 'NODE_REPL_NODE_MODULE_DIRS']) },
  { name: 'trust-and-pipe', keys: pick(['NODE_REPL_TRUSTED_BROWSER_CLIENT_SHA256S', 'NODE_REPL_TRUSTED_CODE_PATHS', 'SKY_CUA_NATIVE_PIPE', 'SKY_CUA_NATIVE_PIPE_DIRECTORY']) },
  { name: 'trust-pipe-backend-codex', keys: pick(['NODE_REPL_TRUSTED_BROWSER_CLIENT_SHA256S', 'NODE_REPL_TRUSTED_CODE_PATHS', 'SKY_CUA_NATIVE_PIPE', 'SKY_CUA_NATIVE_PIPE_DIRECTORY', 'BROWSER_USE_AVAILABLE_BACKENDS', 'BROWSER_USE_CODEX_APP_VERSION', 'BROWSER_USE_CODEX_APP_BUILD_FLAVOR', 'CODEX_HOME', 'CODEX_CLI_PATH']) },
  { name: 'all-except-node-paths', keys: Object.keys(env).filter((key) => !['NODE_REPL_NODE_PATH', 'NODE_REPL_NODE_MODULE_DIRS'].includes(key)) },
  { name: 'all-except-native-pipe', keys: Object.keys(env).filter((key) => !['SKY_CUA_NATIVE_PIPE', 'SKY_CUA_NATIVE_PIPE_DIRECTORY'].includes(key)) },
  { name: 'full', keys: Object.keys(env) },
];

const directJsCode = 'nodeRepl.write(JSON.stringify({ok:true,cwd:nodeRepl.cwd,homeDir:nodeRepl.homeDir}))';
const chromeJsCode = `
var result = { ok: false, step: "chrome-extension-target-smoke" };
try {
  var desktopChromeClient = await import(${JSON.stringify(args.chromeClientUrl)});
  await desktopChromeClient.setupBrowserRuntime({ globals: globalThis });
  var targets = await agent.browsers.list();
  var chrome = await agent.browsers.get('extension');
  var userTabs = await chrome.user.openTabs();
  result = {
    ok: true,
    backend: 'extension',
    targets: targets.map((target) => ({
      name: target.name,
      type: target.type,
      id: target.id,
      profileName: target.metadata?.profileName ?? null,
    })),
    openUserTabCount: userTabs.length,
  };
} catch (error) {
  result.error = String(error && (error.stack || error.message) || error);
}
nodeRepl.write(JSON.stringify(result));
`;

const matrix = [];
for (const item of envCases) {
  const direct = runMcpCase(nodeReplCommand, env, item.keys, directJsCode, `env-matrix-direct-${item.name}`, 30000);
  let chrome = null;
  if (direct.classification === 'pass') {
    chrome = runMcpCase(nodeReplCommand, env, item.keys, chromeJsCode, `env-matrix-chrome-${item.name}`, 70000);
  }
  matrix.push({
    name: item.name,
    env_keys: item.keys,
    direct,
    chrome,
  });
}

const chromeReadyCases = matrix.filter((item) => item.chrome?.classification === 'extension_ready');
const blockers = [];
if (!config) blockers.push('Windows node_repl MCP config was not parseable from explicit codex.exe');
if (chromeReadyCases.length === 0) {
  blockers.push('No tested node_repl environment subset reached the Chrome extension backend from SSH stdio');
}

const summary = {
  package_id: 'ui_windows_node_repl_env_matrix_smoke',
  result: chromeReadyCases.length > 0 ? 'pass' : 'blocked_chrome_bridge',
  generated_at: new Date().toISOString(),
  windows: {
    host: args.windowsHost,
    user: args.windowsUser,
  },
  mcp_config: {
    result: config ? 'pass' : 'blocked',
    enabled: config?.enabled ?? null,
    transport_type: transport.type || null,
    env_keys: Object.keys(env),
    env_inventory: Object.fromEntries(Object.entries(env).map(([key, value]) => [key, {
      present: typeof value === 'string' && value.length > 0,
      length: typeof value === 'string' ? value.length : 0,
    }])),
  },
  counts: {
    js_pass_cases: matrix.filter((item) => item.direct.classification === 'pass').length,
    chrome_extension_ready_cases: chromeReadyCases.length,
    chrome_native_pipe_or_trust_cases: matrix.filter((item) => item.chrome?.classification === 'native_pipe_or_trust_unavailable').length,
    chrome_sandbox_firewall_cases: matrix.filter((item) => item.chrome?.classification === 'sandbox_firewall_denied').length,
  },
  matrix,
  acceptance_effect: 'boundary_evidence_only_not_visual_acceptance',
  blockers,
};

ensureDirFor(args.outputJson);
fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
ensureDirFor(args.outputMd);
fs.writeFileSync(resolveRepo(args.outputMd), renderMarkdown(summary), 'utf8');

console.log(`ui-windows-node-repl-env-matrix-smoke result=${summary.result} js_pass=${summary.counts.js_pass_cases}/${summary.matrix.length} chrome_ready=${summary.counts.chrome_extension_ready_cases} json=${repoRel(args.outputJson)} md=${repoRel(args.outputMd)}`);

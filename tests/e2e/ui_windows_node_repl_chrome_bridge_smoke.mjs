#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

const defaults = {
  windowsHost: '10.3.6.59',
  windowsUser: 'LongShine',
  codexExe: 'C:\\Users\\LongShine\\AppData\\Local\\OpenAI\\Codex\\bin\\ea1c60319a1dcb19\\codex.exe',
  chromeClientUrl: 'file:///C:/Users/LongShine/.codex/plugins/cache/openai-bundled/chrome/latest/scripts/browser-client.mjs',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-node-repl-chrome-bridge-smoke-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-node-repl-chrome-bridge-smoke-latest.md',
  timeoutMs: '150000',
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

function ssh(remoteCommand, input = '') {
  if (!process.env.SSHPASS) {
    return {
      ok: false,
      status: 2,
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
      timeout: Number(args.timeoutMs),
      maxBuffer: 20 * 1024 * 1024,
    },
  );
  return {
    ok: result.status === 0,
    status: result.status ?? null,
    signal: result.signal,
    stdout: String(result.stdout || ''),
    stderr: sanitize(String(result.stderr || '')),
    error: result.error ? result.error.message : '',
  };
}

function sanitize(value) {
  return sanitizeText(String(value || '').replace(/Warning: Permanently added .*known hosts\.\s*/g, '')).trim();
}

function parseJsonLines(stdout) {
  return String(stdout || '')
    .split(/\r?\n/)
    .filter(Boolean)
    .map((line) => {
      try {
        return JSON.parse(line);
      } catch (error) {
        return { parse_error: error.message, raw_prefix: line.slice(0, 200) };
      }
    });
}

function mcpMessages(code, title, timeoutMs = 120000) {
  return [
    {
      jsonrpc: '2.0',
      id: 1,
      method: 'initialize',
      params: {
        protocolVersion: '2025-06-18',
        capabilities: {},
        clientInfo: { name: 'ui-windows-node-repl-chrome-bridge-smoke', version: '1' },
      },
    },
    { jsonrpc: '2.0', method: 'notifications/initialized', params: {} },
    {
      jsonrpc: '2.0',
      id: 2,
      method: 'tools/list',
      params: {},
    },
    {
      jsonrpc: '2.0',
      id: 3,
      method: 'tools/call',
      params: {
        name: 'js',
        arguments: {
          code,
          timeout_ms: timeoutMs,
          title,
        },
      },
    },
  ];
}

function inputFor(messages) {
  return `${messages.map((message) => JSON.stringify(message)).join('\n')}\n`;
}

function remoteNodeReplCommand(nodeReplCommand, env = {}, includeEnv = false) {
  if (!includeEnv) return `"${nodeReplCommand}"`;
  const envKeys = [
    'NODE_REPL_TRUSTED_BROWSER_CLIENT_SHA256S',
    'BROWSER_USE_AVAILABLE_BACKENDS',
    'BROWSER_USE_CODEX_APP_VERSION',
    'CODEX_CLI_PATH',
    'NODE_REPL_NODE_MODULE_DIRS',
    'NODE_REPL_TRUSTED_CODE_PATHS',
    'CODEX_HOME',
    'NODE_REPL_NATIVE_PIPE_CONNECT_TIMEOUT_MS',
    'BROWSER_USE_CODEX_APP_BUILD_FLAVOR',
    'NODE_REPL_NODE_PATH',
    'SKY_CUA_NATIVE_PIPE',
    'SKY_CUA_NATIVE_PIPE_DIRECTORY',
  ];
  const setCommands = envKeys
    .filter((key) => typeof env[key] === 'string' && env[key].length > 0)
    .map((key) => `set ${key}=${env[key]}`);
  return `cmd /d /c "${setCommands.join('&&')}&&${nodeReplCommand}"`;
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

function sanitizeText(value) {
  return String(value || '')
    .replace(/\b(pass(word)?|pwd)\s*[:=]\s*\S+/gi, '$1=<redacted>')
    .replace(/sk-proj-[A-Za-z0-9_-]+/g, '<redacted-openai-key>')
    .replace(/Bearer\s+[A-Za-z0-9._-]+/gi, 'Bearer <redacted>')
    .replace(/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g, '<redacted-jwt>')
    .replace(/\\\\\.\\pipe\\codex-computer-use-[0-9A-Fa-f-]+/g, '<redacted-native-pipe>')
    .replace(/codex-computer-use-[0-9A-Fa-f-]+/g, 'codex-computer-use-<redacted>');
}

function parseCallJson(callResponse) {
  const text = callText(callResponse);
  try {
    return JSON.parse(text);
  } catch {
    return text ? { text_prefix: text.slice(0, 500) } : null;
  }
}

function classifyChromeResult(callData) {
  const error = String(callData?.error || callData?.text_prefix || '');
  if (callData?.ok === true && callData.backend === 'extension') return 'extension_ready';
  if (/browser-client is not trusted|privileged native pipe bridge is not available/i.test(error)) {
    return 'native_pipe_or_trust_unavailable';
  }
  if (/helper_firewall_rule_create_or_add_failed|SetRemotePorts|HRESULT\\(0x80070005\\)|拒绝访问/i.test(error)) {
    return 'sandbox_firewall_denied';
  }
  if (error) return 'chrome_smoke_failed';
  return 'missing_result';
}

function renderMarkdown(summary) {
  const lines = [
    '# Windows Node REPL Chrome Bridge Smoke',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Windows host: \`${summary.windows.host}\``,
	    `- MCP config: \`${summary.mcp_config.result}\``,
	    `- Direct JS smoke: \`${summary.direct_js_smoke.result}\``,
	    `- Full-env JS smoke: \`${summary.full_env_js_smoke.result}\``,
	    `- Chrome extension smoke without env: \`${summary.chrome_extension_no_env_smoke.result}\``,
	    `- Chrome no-env failure class: \`${summary.chrome_extension_no_env_smoke.failure_class}\``,
	    `- Chrome extension smoke: \`${summary.chrome_extension_smoke.result}\``,
	    `- Chrome failure class: \`${summary.chrome_extension_smoke.failure_class}\``,
    '',
    'This smoke is intentionally narrow. It executes JavaScript through Windows node_repl over SSH stdio and attempts read-only Chrome extension target discovery. It does not capture UI screenshots and does not close visual acceptance.',
    '',
    '## Blockers',
    '',
  ];
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
const envInventory = Object.fromEntries(Object.entries(env).map(([key, value]) => [key, {
  present: typeof value === 'string' && value.length > 0,
  length: typeof value === 'string' ? value.length : 0,
}]));

const directJsCode = 'nodeRepl.write(JSON.stringify({ok:true,cwd:nodeRepl.cwd,homeDir:nodeRepl.homeDir}))';
const chromeJsCode = `
var result = { ok: false, step: "chrome-extension-target-smoke" };
try {
  var client = await import(${JSON.stringify(args.chromeClientUrl)});
  await client.setupBrowserRuntime({ globals: globalThis });
  var targets = await agent.browsers.list();
  result.targets = targets.map((target) => ({ name: target.name, type: target.type, id: target.id, profileName: target.metadata?.profileName ?? null })).slice(0, 20);
  var chromeTarget = targets.find((target) => target.type === "extension" || target.name === "Chrome");
  result.chromeTarget = chromeTarget ? { name: chromeTarget.name, type: chromeTarget.type, id: chromeTarget.id } : null;
  if (!chromeTarget) throw new Error("Chrome extension target not found");
  var desktopChrome = await agent.browsers.get("extension");
  var userTabs = await desktopChrome.user.openTabs();
  result.backend = "extension";
  result.userTabCount = userTabs.length;
  result.userTabs = userTabs.map((tab) => ({ id: tab.id, title: tab.title, url: tab.url })).slice(0, 10);
  result.ok = true;
} catch (error) {
  result.error = String(error?.stack || error?.message || error);
}
nodeRepl.write(JSON.stringify(result));
`;

const directProbe = ssh(
  remoteNodeReplCommand(nodeReplCommand, env, false),
  inputFor(mcpMessages(directJsCode, 'Direct minimal JS smoke', 30000)),
);
const directMessages = parseJsonLines(directProbe.stdout);
const directCall = responseById(directMessages, 3);
const directData = parseCallJson(directCall);
const directOk = directProbe.ok && directCall?.result?.isError === false && directData?.ok === true;

const fullEnvProbe = ssh(
  remoteNodeReplCommand(nodeReplCommand, env, true),
  inputFor(mcpMessages(directJsCode, 'Full env minimal JS smoke', 30000)),
);
const fullEnvMessages = parseJsonLines(fullEnvProbe.stdout);
const fullEnvCall = responseById(fullEnvMessages, 3);
const fullEnvData = parseCallJson(fullEnvCall);
const fullEnvOk = fullEnvProbe.ok && fullEnvCall?.result?.isError === false && fullEnvData?.ok === true;

const chromeNoEnvProbe = ssh(
  remoteNodeReplCommand(nodeReplCommand, env, false),
  inputFor(mcpMessages(chromeJsCode, 'Chrome extension target smoke without env', 120000)),
);
const chromeNoEnvMessages = parseJsonLines(chromeNoEnvProbe.stdout);
const chromeNoEnvTools = toolNames(responseById(chromeNoEnvMessages, 2));
const chromeNoEnvCall = responseById(chromeNoEnvMessages, 3);
const chromeNoEnvData = parseCallJson(chromeNoEnvCall);
const chromeNoEnvFailureClass = classifyChromeResult(chromeNoEnvData);
const chromeNoEnvOk = chromeNoEnvFailureClass === 'extension_ready';

const chromeProbe = ssh(
  remoteNodeReplCommand(nodeReplCommand, env, true),
  inputFor(mcpMessages(chromeJsCode, 'Chrome extension target smoke', 120000)),
);
const chromeMessages = parseJsonLines(chromeProbe.stdout);
const chromeTools = toolNames(responseById(chromeMessages, 2));
const chromeCall = responseById(chromeMessages, 3);
const chromeData = parseCallJson(chromeCall);
const chromeFailureClass = classifyChromeResult(chromeData);
const chromeOk = chromeFailureClass === 'extension_ready';

const blockers = [];
if (!configProbe.ok || !config) blockers.push('Windows codex mcp get node_repl --json did not return parseable config');
if (!directOk) blockers.push('Direct SSH-spawned node_repl minimal JS smoke failed');
if (!fullEnvOk) blockers.push('Full-env SSH-spawned node_repl minimal JS smoke failed');
if (!chromeNoEnvOk) blockers.push(`No-env Chrome extension target smoke did not reach extension backend: ${chromeNoEnvFailureClass}`);
if (!chromeOk) blockers.push(`Full-env Chrome extension target smoke did not reach extension backend: ${chromeFailureClass}`);
const allSmokesOk = directOk && fullEnvOk && chromeNoEnvOk && chromeOk;

const summary = {
  package_id: 'ui_windows_node_repl_chrome_bridge_smoke',
  result: allSmokesOk ? 'pass' : directOk ? 'blocked_chrome_bridge' : 'blocked_node_repl_js',
  generated_at: new Date().toISOString(),
  windows: {
    host: args.windowsHost,
    user: args.windowsUser,
    node_repl_command: nodeReplCommand,
    chrome_client_url: args.chromeClientUrl,
  },
  mcp_config: {
    result: config ? 'pass' : 'blocked',
    enabled: config?.enabled ?? null,
    transport_type: transport.type || null,
    env_keys: Object.keys(env),
    env_inventory: envInventory,
  },
  direct_js_smoke: {
    result: directOk ? 'pass' : 'blocked',
    ssh_exit_code: directProbe.status,
    tools: toolNames(responseById(directMessages, 2)),
    data: directData,
    is_error: directCall?.result?.isError ?? null,
    stderr: directProbe.stderr.slice(0, 500),
  },
  full_env_js_smoke: {
    result: fullEnvOk ? 'pass' : 'blocked',
    ssh_exit_code: fullEnvProbe.status,
    data: fullEnvData,
    is_error: fullEnvCall?.result?.isError ?? null,
    stderr: fullEnvProbe.stderr.slice(0, 500),
  },
  chrome_extension_no_env_smoke: {
    result: chromeNoEnvOk ? 'pass' : 'blocked',
    failure_class: chromeNoEnvFailureClass,
    ssh_exit_code: chromeNoEnvProbe.status,
    tools: chromeNoEnvTools,
    data: chromeNoEnvData,
    is_error: chromeNoEnvCall?.result?.isError ?? null,
    stderr: chromeNoEnvProbe.stderr.slice(0, 500),
  },
  chrome_extension_smoke: {
    result: chromeOk ? 'pass' : 'blocked',
    failure_class: chromeFailureClass,
    ssh_exit_code: chromeProbe.status,
    tools: chromeTools,
    data: chromeData,
    is_error: chromeCall?.result?.isError ?? null,
    stderr: chromeProbe.stderr.slice(0, 500),
  },
  acceptance_effect: 'boundary_evidence_only_not_visual_acceptance',
  blockers,
};

ensureDirFor(args.outputJson);
fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
ensureDirFor(args.outputMd);
fs.writeFileSync(resolveRepo(args.outputMd), renderMarkdown(summary), 'utf8');

console.log(`ui-windows-node-repl-chrome-bridge-smoke result=${summary.result} direct_js=${summary.direct_js_smoke.result} full_env_js=${summary.full_env_js_smoke.result} chrome_no_env=${summary.chrome_extension_no_env_smoke.result}:${summary.chrome_extension_no_env_smoke.failure_class} chrome=${summary.chrome_extension_smoke.result}:${summary.chrome_extension_smoke.failure_class} json=${repoRel(args.outputJson)} md=${repoRel(args.outputMd)}`);

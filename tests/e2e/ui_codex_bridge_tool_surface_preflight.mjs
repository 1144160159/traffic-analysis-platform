#!/usr/bin/env node
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

const defaults = {
  pluginDir: '/root/.codex/plugins/cache/personal/codex-desktop-iab-bridge/0.1.0',
  proxyPath: '/root/.codex/bin/node-repl-mcp-proxy.py',
  backendHost: '127.0.0.1',
  backendPort: '19998',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/codex-bridge-tool-surface-preflight-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/codex-bridge-tool-surface-preflight-latest.md',
  timeoutMs: '5000',
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
  if (!fs.existsSync(file)) return { exists: false, valid: false, data: null, error: 'missing' };
  try {
    return { exists: true, valid: true, data: JSON.parse(fs.readFileSync(file, 'utf8')), error: null };
  } catch (error) {
    return { exists: true, valid: false, data: null, error: error.message };
  }
}

function run(command, commandArgs, options = {}) {
  const result = spawnSync(command, commandArgs, {
    cwd: root,
    encoding: 'utf8',
    timeout: Number(args.timeoutMs),
    ...options,
  });
  return {
    command: [command, ...commandArgs].join(' '),
    exit_code: result.status,
    signal: result.signal,
    stdout: String(result.stdout || '').trim(),
    stderr: String(result.stderr || '').trim(),
    error: result.error ? result.error.message : null,
    passed: result.status === 0,
  };
}

function checkTcp(host, port, timeoutMs = 1200) {
  return new Promise((resolve) => {
    const socket = new net.Socket();
    let settled = false;
    const finish = (open, detail) => {
      if (settled) return;
      settled = true;
      socket.destroy();
      resolve({ open, detail });
    };
    socket.setTimeout(timeoutMs);
    socket.once('connect', () => finish(true, 'connect'));
    socket.once('timeout', () => finish(false, 'timeout'));
    socket.once('error', (error) => finish(false, `${error.name}: ${error.message}`));
    socket.connect(Number(port), host);
  });
}

function proxyJsonRpc(messages) {
  const input = messages.map((message) => JSON.stringify(message)).join('\n') + '\n';
  const result = run('python3', [args.proxyPath], { input });
  const responses = result.stdout
    .split(/\r?\n/)
    .filter(Boolean)
    .map((line) => {
      try {
        return JSON.parse(line);
      } catch (error) {
        return { parse_error: error.message, raw: line.slice(0, 200) };
      }
    });
  return { ...result, responses };
}

function hasBridgePlugin(pluginListText) {
  return /codex-desktop-iab-bridge@personal\s+installed,\s*enabled/.test(pluginListText);
}

function renderMarkdown(summary) {
  const lines = [
    '# Codex Bridge Tool Surface Preflight',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Plugin installed/enabled: \`${summary.plugin.installed_enabled}\``,
    `- MCP config valid: \`${summary.plugin.mcp_config_valid}\``,
    `- Proxy path exists: \`${summary.proxy.path_exists}\``,
    `- Proxy initialize fallback: \`${summary.proxy.initialize_fallback_ok}\``,
    `- Proxy tools/list without backend: \`${summary.proxy.tools_list_status}\``,
    `- Backend ${summary.backend.host}:${summary.backend.port}: \`${summary.backend.open ? 'open' : 'closed'}\``,
    `- Session Desktop tools status: \`${summary.codex_session.desktop_tool_status}\``,
    '',
    'This preflight separates local plugin installation from current-chat callable tool exposure. It does not execute browser JavaScript, inspect Chrome state, or close Desktop Chrome acceptance.',
    '',
    '## Blockers',
    '',
  ];
  if (summary.blockers.length === 0) lines.push('- none');
  for (const blocker of summary.blockers) lines.push(`- ${blocker}`);
  lines.push('');
  return `${lines.join('\n')}\n`;
}

const pluginJsonPath = path.join(args.pluginDir, '.codex-plugin/plugin.json');
const mcpJsonPath = path.join(args.pluginDir, '.mcp.json');
const pluginJson = readJsonState(pluginJsonPath);
const mcpJson = readJsonState(mcpJsonPath);
const pluginList = run('codex', ['plugin', 'list']);
const backend = await checkTcp(args.backendHost, Number(args.backendPort));
const initialize = proxyJsonRpc([
  {
    jsonrpc: '2.0',
    id: 1,
    method: 'initialize',
    params: {
      protocolVersion: '2025-06-18',
      capabilities: {},
      clientInfo: { name: 'codex-bridge-tool-surface-preflight', version: '1' },
    },
  },
]);
const toolsList = proxyJsonRpc([
  {
    jsonrpc: '2.0',
    id: 1,
    method: 'initialize',
    params: {
      protocolVersion: '2025-06-18',
      capabilities: {},
      clientInfo: { name: 'codex-bridge-tool-surface-preflight', version: '1' },
    },
  },
  { jsonrpc: '2.0', method: 'notifications/initialized', params: {} },
  { jsonrpc: '2.0', id: 2, method: 'tools/list', params: {} },
]);
const toolListResponse = toolsList.responses.find((item) => item.id === 2) || null;
const toolsListStatus = toolListResponse?.result?.tools
  ? 'listed'
  : toolListResponse?.error?.message
    ? `error: ${toolListResponse.error.message}`
    : 'missing-response';
const sessionToolStatus = process.env.CODEX_SESSION_DESKTOP_TOOL_STATUS || 'not_exposed_in_current_chat_tool_surface';
const sessionToolDetail = process.env.CODEX_SESSION_DESKTOP_TOOL_DETAIL || 'tool discovery returned unrelated app connector tools only';

const blockers = [];
if (!hasBridgePlugin(pluginList.stdout)) blockers.push('codex-desktop-iab-bridge plugin is not installed and enabled in codex plugin list');
if (!pluginJson.valid) blockers.push(`plugin.json is not valid: ${pluginJson.error}`);
if (!mcpJson.valid) blockers.push(`.mcp.json is not valid: ${mcpJson.error}`);
if (!fs.existsSync(args.proxyPath)) blockers.push(`proxy path missing: ${args.proxyPath}`);

const summary = {
  package_id: 'ui_codex_bridge_tool_surface_preflight',
  result: blockers.length === 0 ? 'pass' : 'blocked',
  generated_at: new Date().toISOString(),
  plugin: {
    plugin_dir: args.pluginDir,
    plugin_json_path: pluginJsonPath,
    plugin_json_valid: pluginJson.valid,
    mcp_json_path: mcpJsonPath,
    mcp_config_valid: mcpJson.valid,
    installed_enabled: hasBridgePlugin(pluginList.stdout),
    mcp_server_names: Object.keys(mcpJson.data?.mcpServers || {}),
    codex_plugin_list_exit_code: pluginList.exit_code,
  },
  proxy: {
    path: args.proxyPath,
    path_exists: fs.existsSync(args.proxyPath),
    initialize_exit_code: initialize.exit_code,
    initialize_fallback_ok: Boolean(initialize.responses.find((item) => item.id === 1)?.result?.serverInfo?.name === 'node_repl_proxy'),
    tools_list_exit_code: toolsList.exit_code,
    tools_list_status: toolsListStatus,
    tools_list_error: toolListResponse?.error?.message || null,
  },
  backend: {
    host: args.backendHost,
    port: Number(args.backendPort),
    open: backend.open,
    detail: backend.detail,
  },
  codex_session: {
    desktop_tool_status: sessionToolStatus,
    desktop_tool_detail: sessionToolDetail,
    required_tools: [
      'mcp__codex_desktop_node_repl__js',
      'desktop_chrome_open_url',
      'desktop_chrome_list_tabs',
      'desktop_chrome_claim_url',
    ],
  },
  acceptance_effect: 'boundary_evidence_only_not_desktop_chrome_acceptance',
  inference: backend.open
    ? 'A local backend listener exists, but formal Chrome acceptance still requires callable current-session Desktop Chrome tools and a bridge run artifact.'
    : 'The local plugin and proxy are installed, but there is no local node_repl backend listener and the current chat tool surface still lacks the Desktop Chrome tools.',
  blockers,
};

ensureDirFor(args.outputJson);
fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
ensureDirFor(args.outputMd);
fs.writeFileSync(resolveRepo(args.outputMd), renderMarkdown(summary), 'utf8');

console.log(`ui-codex-bridge-tool-surface-preflight result=${summary.result} plugin_installed=${summary.plugin.installed_enabled} backend_open=${summary.backend.open} tools_list_status=${summary.proxy.tools_list_status} session_tool_status=${summary.codex_session.desktop_tool_status} json=${repoRel(args.outputJson)} md=${repoRel(args.outputMd)}`);

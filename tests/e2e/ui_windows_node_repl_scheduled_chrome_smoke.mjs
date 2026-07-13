#!/usr/bin/env node
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

const defaults = {
  windowsHost: '10.3.6.59',
  windowsUser: 'LongShine',
  codexExe: 'C:\\Users\\LongShine\\AppData\\Local\\OpenAI\\Codex\\bin\\ea1c60319a1dcb19\\codex.exe',
  nodeExe: 'D:\\soft\\nvm\\nodejs\\node.exe',
  chromeClientUrl: 'file:///C:/Users/LongShine/.codex/plugins/cache/openai-bundled/chrome/latest/scripts/browser-client.mjs',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-node-repl-scheduled-chrome-smoke-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-node-repl-scheduled-chrome-smoke-latest.md',
  timeoutMs: '180000',
  pollSeconds: '90',
};

const args = { ...defaults, ...parseArgs(process.argv.slice(2)) };
const root = process.cwd();
const runStamp = String(Date.now()).slice(-10);
const runId = `CodexCB${runStamp}`;
const remoteDir = `C:\\Users\\${args.windowsUser}\\AppData\\Local\\Temp\\${runId}`;
const remoteRunner = `${remoteDir}\\scheduled-runner.mjs`;
const remoteCmd = `${remoteDir}\\run.cmd`;
const remoteResult = `${remoteDir}\\scheduled-result.json`;
const remoteStdout = `${remoteDir}\\task-stdout.txt`;
const remoteStderr = `${remoteDir}\\task-stderr.txt`;
const remoteDirShell = remoteDir.replace(/\\/g, '/');
const remoteCmdShell = remoteCmd.replace(/\\/g, '/');
const taskName = `CodexCB${runStamp}`;

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
    .replace(/0;C:\\WINDOWS\\system32\\conhost\.exe/gi, '')
    .replace(/\b(pass(word)?|pwd)\s*[:=]\s*\S+/gi, '$1=<redacted>')
    .replace(/sk-proj-[A-Za-z0-9_-]+/g, '<redacted-openai-key>')
    .replace(/Bearer\s+[A-Za-z0-9._-]+/gi, 'Bearer <redacted>')
    .replace(/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g, '<redacted-jwt>')
    .replace(/\\\\\.\\pipe\\codex-computer-use-[0-9A-Fa-f-]+/g, '<redacted-native-pipe>')
    .replace(/codex-computer-use-[0-9A-Fa-f-]+/g, 'codex-computer-use-<redacted>')
    .replace(/Warning: Permanently added .*known hosts\.\s*/g, '')
    .trim();
}

function hasRemoteCommandError(value) {
  return /(^|\s)(error|错误)\s*:|占位程序接收到错误数据|系统找不到指定的文件/i.test(String(value || ''));
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
    ok: result.status === 0 && !hasRemoteCommandError(`${result.stdout || ''}\n${result.stderr || ''}`),
    status: result.status ?? null,
    signal: result.signal,
    stdout: sanitizeText(String(result.stdout || '')),
    stderr: sanitizeText(String(result.stderr || '')),
    error: result.error ? result.error.message : '',
  };
}

function shellQuote(value) {
  return `'${String(value).replace(/'/g, `'\\''`)}'`;
}

function sshShell(remoteCommand) {
  if (!process.env.SSHPASS) {
    return {
      ok: false,
      status: 2,
      stdout: '',
      stderr: 'SSHPASS is required via environment; it must not be written to files.',
      error: '',
    };
  }
  const command = [
    'sshpass',
    '-e',
    'ssh',
    '-tt',
    '-o', 'StrictHostKeyChecking=no',
    '-o', 'UserKnownHostsFile=/dev/null',
    '-o', 'PreferredAuthentications=password',
    '-o', 'PubkeyAuthentication=no',
    '-o', 'NumberOfPasswordPrompts=1',
    '-o', 'ConnectTimeout=8',
    `${args.windowsUser}@${args.windowsHost}`,
    remoteCommand,
  ].map(shellQuote).join(' ');
  const result = spawnSync('bash', ['-lc', command], {
    cwd: root,
    env: process.env,
    encoding: 'utf8',
    timeout: Number(args.timeoutMs),
    maxBuffer: 20 * 1024 * 1024,
  });
  return {
    ok: result.status === 0 && !hasRemoteCommandError(`${result.stdout || ''}\n${result.stderr || ''}`),
    status: result.status ?? null,
    signal: result.signal,
    stdout: sanitizeText(String(result.stdout || '')),
    stderr: sanitizeText(String(result.stderr || '')),
    error: result.error ? result.error.message : '',
  };
}

function scp(localFile, remoteWindowsPath) {
  if (!process.env.SSHPASS) {
    return {
      ok: false,
      status: 2,
      stdout: '',
      stderr: 'SSHPASS is required via environment; it must not be written to files.',
      error: '',
    };
  }
  const remoteScpPath = remoteWindowsPath.replace(/\\/g, '/');
  const result = spawnSync(
    'sshpass',
    [
      '-e',
      'scp',
      '-o', 'StrictHostKeyChecking=no',
      '-o', 'UserKnownHostsFile=/dev/null',
      '-o', 'PreferredAuthentications=password',
      '-o', 'PubkeyAuthentication=no',
      '-o', 'NumberOfPasswordPrompts=1',
      '-o', 'ConnectTimeout=8',
      localFile,
      `${args.windowsUser}@${args.windowsHost}:${remoteScpPath}`,
    ],
    {
      cwd: root,
      env: process.env,
      encoding: 'utf8',
      timeout: Number(args.timeoutMs),
      maxBuffer: 20 * 1024 * 1024,
    },
  );
  return {
    ok: result.status === 0,
    status: result.status ?? null,
    signal: result.signal,
    stdout: sanitizeText(String(result.stdout || '')),
    stderr: sanitizeText(String(result.stderr || '')),
    error: result.error ? result.error.message : '',
  };
}

function cmd(command) {
  return `cmd /d /c ${command}`;
}

function quote(value) {
  return `"${String(value).replace(/"/g, '\\"')}"`;
}

function parseRemoteJson(value) {
  try {
    return JSON.parse(sanitizeText(value));
  } catch (error) {
    return { parse_error: error.message, raw_prefix: sanitizeText(value).slice(0, 800) };
  }
}

function runnerSource() {
  return `import fs from 'node:fs';
import { spawnSync } from 'node:child_process';

const codexExe = ${JSON.stringify(args.codexExe)};
const chromeClientUrl = ${JSON.stringify(args.chromeClientUrl)};
const resultPath = ${JSON.stringify(remoteResult)};

function sanitizeText(value) {
  return String(value || '')
    .replace(/\\\\b(pass(word)?|pwd)\\\\s*[:=]\\\\s*\\\\S+/gi, '$1=<redacted>')
    .replace(/sk-proj-[A-Za-z0-9_-]+/g, '<redacted-openai-key>')
    .replace(/Bearer\\\\s+[A-Za-z0-9._-]+/gi, 'Bearer <redacted>')
    .replace(/eyJ[A-Za-z0-9_-]+\\\\.[A-Za-z0-9_-]+\\\\.[A-Za-z0-9_-]+/g, '<redacted-jwt>')
    .replace(/\\\\\\\\\\\\\\\\.\\\\\\\\pipe\\\\\\\\codex-computer-use-[0-9A-Fa-f-]+/g, '<redacted-native-pipe>')
    .replace(/codex-computer-use-[0-9A-Fa-f-]+/g, 'codex-computer-use-<redacted>');
}

function parseJsonLines(stdout) {
  return String(stdout || '')
    .split(/\\r?\\n/)
    .filter(Boolean)
    .map((line) => {
      try {
        return JSON.parse(line);
      } catch (error) {
        return { parse_error: error.message, raw_prefix: sanitizeText(line).slice(0, 200) };
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
        clientInfo: { name: 'ui-windows-node-repl-scheduled-chrome-smoke', version: '1' },
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
  return \`\${messages.map((message) => JSON.stringify(message)).join('\\n')}\\n\`;
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
  return sanitizeText(content.map((item) => (typeof item?.text === 'string' ? item.text : '')).join('\\n'));
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
  if (/helper_firewall_rule_create_or_add_failed|SetRemotePorts|HRESULT\\\\(0x80070005\\\\)|拒绝访问/i.test(error)) {
    return 'sandbox_firewall_denied';
  }
  if (/ETIMEDOUT|timeout|timed out/i.test(error)) return 'timeout';
  if (error) return 'chrome_smoke_failed';
  return 'missing_result';
}

function runMcp(nodeReplCommand, env, code, title, timeoutMs) {
  const startedAt = Date.now();
  const result = spawnSync(nodeReplCommand, [], {
    env: { ...process.env, ...env },
    input: inputFor(mcpMessages(code, title, timeoutMs)),
    encoding: 'utf8',
    timeout: timeoutMs + 30000,
    maxBuffer: 20 * 1024 * 1024,
  });
  const messages = parseJsonLines(result.stdout);
  const call = responseById(messages, 3);
  const data = parseCallJson(call);
  return {
    exit_code: result.status ?? null,
    signal: result.signal || null,
    duration_ms: Date.now() - startedAt,
    tools: toolNames(responseById(messages, 2)),
    is_error: call?.result?.isError ?? null,
    data,
    stderr: sanitizeText(result.stderr).slice(0, 800),
    error: result.error ? sanitizeText(result.error.message) : '',
  };
}

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

const directJsCode = 'nodeRepl.write(JSON.stringify({ok:true,cwd:nodeRepl.cwd,homeDir:nodeRepl.homeDir}))';
const chromeJsCode = \`
var result = { ok: false, step: "scheduled-chrome-extension-target-smoke" };
try {
  var client = await import(\${JSON.stringify(chromeClientUrl)});
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
  result.ok = true;
} catch (error) {
  result.error = String(error?.stack || error?.message || error);
}
nodeRepl.write(JSON.stringify(result));
\`;

const configProbe = spawnSync(codexExe, ['mcp', 'get', 'node_repl', '--json'], {
  encoding: 'utf8',
  timeout: 30000,
  maxBuffer: 5 * 1024 * 1024,
});

let config = null;
try {
  config = JSON.parse(configProbe.stdout);
} catch {
  config = null;
}

const transport = config?.transport || {};
const fullEnv = {};
for (const key of envKeys) {
  if (typeof transport.env?.[key] === 'string' && transport.env[key].length > 0) {
    fullEnv[key] = transport.env[key];
  }
}
const envInventory = Object.fromEntries(Object.entries(fullEnv).map(([key, value]) => [key, {
  present: true,
  length: value.length,
}]));
const nodeReplCommand = transport.command || '';

let directProbe = null;
let fullEnvProbe = null;
let chromeProbe = null;
if (nodeReplCommand) {
  directProbe = runMcp(nodeReplCommand, {}, directJsCode, 'Scheduled direct minimal JS smoke', 30000);
  fullEnvProbe = runMcp(nodeReplCommand, fullEnv, directJsCode, 'Scheduled full-env minimal JS smoke', 30000);
  chromeProbe = runMcp(nodeReplCommand, fullEnv, chromeJsCode, 'Scheduled Chrome extension target smoke', 120000);
}

const directOk = directProbe?.exit_code === 0 && directProbe?.is_error === false && directProbe?.data?.ok === true;
const fullEnvOk = fullEnvProbe?.exit_code === 0 && fullEnvProbe?.is_error === false && fullEnvProbe?.data?.ok === true;
const chromeFailureClass = classifyChromeResult(chromeProbe?.data);
const chromeOk = chromeFailureClass === 'extension_ready';
const blockers = [];
if (!config) blockers.push('Windows scheduled task runner could not read parseable codex node_repl MCP config');
if (!nodeReplCommand) blockers.push('Windows node_repl transport command is missing');
if (!directOk) blockers.push('Scheduled direct node_repl minimal JS smoke failed');
if (!fullEnvOk) blockers.push('Scheduled full-env node_repl minimal JS smoke failed');
if (!chromeOk) blockers.push(\`Scheduled full-env Chrome extension target smoke did not reach extension backend: \${chromeFailureClass}\`);

const summary = {
  package_id: 'ui_windows_node_repl_scheduled_chrome_smoke_remote_runner',
  result: chromeOk ? 'pass' : directOk ? 'blocked_chrome_bridge' : 'blocked_node_repl_js',
  generated_at: new Date().toISOString(),
  context: {
    launcher: 'windows_scheduled_task',
    cwd: process.cwd(),
    node: process.execPath,
  },
  mcp_config: {
    result: config ? 'pass' : 'blocked',
    enabled: config?.enabled ?? null,
    transport_type: transport.type || null,
    env_keys: Object.keys(fullEnv),
    env_inventory: envInventory,
  },
  direct_js_smoke: {
    result: directOk ? 'pass' : 'blocked',
    ...directProbe,
  },
  full_env_js_smoke: {
    result: fullEnvOk ? 'pass' : 'blocked',
    ...fullEnvProbe,
  },
  chrome_extension_smoke: {
    result: chromeOk ? 'pass' : 'blocked',
    failure_class: chromeFailureClass,
    ...chromeProbe,
  },
  acceptance_effect: 'boundary_evidence_only_not_visual_acceptance',
  blockers,
};

fs.writeFileSync(resultPath, \`\${JSON.stringify(summary, null, 2)}\\n\`, 'utf8');
`;
}

function cmdSource() {
  return `@echo off\r\n${quote(args.nodeExe)} ${quote(remoteRunner)} > ${quote(remoteStdout)} 2> ${quote(remoteStderr)}\r\n`;
}

function renderMarkdown(summary) {
  const runner = summary.runner || {};
  const lines = [
    '# Windows Scheduled Node REPL Chrome Bridge Smoke',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Windows host: \`${summary.windows.host}\``,
    `- Scheduled task create: \`${summary.scheduled_task.create.ok ? 'pass' : 'blocked'}\``,
    `- Scheduled task run: \`${summary.scheduled_task.run.ok ? 'pass' : 'blocked'}\``,
    `- Runner result: \`${runner.result || 'missing'}\``,
    `- Direct JS smoke: \`${runner.direct_js_smoke?.result || 'missing'}\``,
    `- Full-env JS smoke: \`${runner.full_env_js_smoke?.result || 'missing'}\``,
    `- Chrome extension smoke: \`${runner.chrome_extension_smoke?.result || 'missing'}\``,
    `- Chrome failure class: \`${runner.chrome_extension_smoke?.failure_class || 'missing'}\``,
    '',
    'This smoke runs a temporary Windows scheduled task under the target user context and asks that task to start node_repl with env loaded at runtime from Windows Codex MCP config. It is boundary evidence only; it does not upload UI screenshots or close visual acceptance.',
    '',
    '## Blockers',
    '',
  ];
  if ((summary.blockers || []).length === 0) lines.push('- none');
  for (const blocker of summary.blockers || []) lines.push(`- ${blocker}`);
  lines.push('');
  return `${lines.join('\n')}\n`;
}

const localTmp = fs.mkdtempSync(path.join(os.tmpdir(), `${runId}-`));
const localRunner = path.join(localTmp, 'scheduled-runner.mjs');
const localCmd = path.join(localTmp, 'run.cmd');
fs.writeFileSync(localRunner, runnerSource(), 'utf8');
fs.writeFileSync(localCmd, cmdSource(), 'utf8');

const remoteCommands = {
  mkdir: cmd(`mkdir "${remoteDir}" 2>nul`),
  create_task: cmd(`schtasks /Create /TN ${taskName} /TR "${remoteCmdShell}" /SC ONCE /ST 23:59 /RL HIGHEST /F /IT /RU ${args.windowsUser}`),
  run_task: cmd(`schtasks /Run /TN ${taskName}`),
  read_result: cmd(`if exist "${remoteResult}" type "${remoteResult}"`),
  query_task: cmd(`schtasks /Query /TN ${taskName} /V /FO LIST`),
  delete_task: cmd(`schtasks /Delete /TN ${taskName} /F`),
  cleanup_files: cmd(`rmdir /s /q "${remoteDir}"`),
};

const mkdirResult = sshShell(remoteCommands.mkdir);
const copyRunner = scp(localRunner, remoteRunner);
const copyCmd = scp(localCmd, remoteCmd);
const createTask = sshShell(remoteCommands.create_task);
const runTask = createTask.ok
  ? sshShell(remoteCommands.run_task)
  : { ok: false, status: null, stdout: '', stderr: 'task was not created', error: '' };

let runner = null;
let outputRead = null;
let queryTask = null;
const pollCount = Math.max(1, Math.ceil(Number(args.pollSeconds) / 2));
for (let index = 0; index < pollCount; index += 1) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 2000);
  outputRead = sshShell(remoteCommands.read_result);
  if (outputRead.ok && outputRead.stdout.trim().startsWith('{')) {
    runner = parseRemoteJson(outputRead.stdout);
    break;
  }
}
if (!runner) {
  queryTask = sshShell(remoteCommands.query_task);
}
const cleanupTask = sshShell(remoteCommands.delete_task);
const cleanupFiles = sshShell(remoteCommands.cleanup_files);

const blockers = [];
if (!mkdirResult.ok && !/already exists/i.test(mkdirResult.stderr)) blockers.push(`remote temp directory could not be created: ${mkdirResult.stderr || mkdirResult.status}`);
if (!copyRunner.ok) blockers.push(`remote runner copy failed: ${copyRunner.stderr || copyRunner.status}`);
if (!copyCmd.ok) blockers.push(`remote command copy failed: ${copyCmd.stderr || copyCmd.status}`);
if (!createTask.ok) blockers.push(`scheduled task create failed: ${createTask.stdout || createTask.stderr || createTask.status}`);
if (!runTask.ok) blockers.push(`scheduled task run failed: ${runTask.stderr || runTask.stdout || runTask.status}`);
if (!runner || runner.parse_error) blockers.push(`scheduled task runner did not produce parseable result: ${runner?.parse_error || outputRead?.stderr || outputRead?.status || 'missing-output'}`);
for (const blocker of runner?.blockers || []) blockers.push(blocker);

const summary = {
  package_id: 'ui_windows_node_repl_scheduled_chrome_smoke',
  result: runner?.result === 'pass' ? 'pass' : runner?.direct_js_smoke?.result === 'pass' ? 'blocked_chrome_bridge' : 'blocked_node_repl_js',
  generated_at: new Date().toISOString(),
  windows: {
    host: args.windowsHost,
    user: args.windowsUser,
    node_exe: args.nodeExe,
    codex_exe: args.codexExe,
    chrome_client_url: args.chromeClientUrl,
  },
  scheduled_task: {
    task_name: taskName,
    remote_dir: remoteDir,
    command_shapes: remoteCommands,
    create: createTask,
    run: runTask,
    query_after_timeout: queryTask,
    delete: cleanupTask,
    cleanup_files: cleanupFiles,
  },
  transfer: {
    mkdir: mkdirResult,
    copy_runner: copyRunner,
    copy_cmd: copyCmd,
  },
  runner,
  acceptance_effect: 'boundary_evidence_only_not_visual_acceptance',
  blockers,
};

ensureDirFor(args.outputJson);
fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
ensureDirFor(args.outputMd);
fs.writeFileSync(resolveRepo(args.outputMd), renderMarkdown(summary), 'utf8');

console.log(`ui-windows-node-repl-scheduled-chrome-smoke result=${summary.result} runner=${runner?.result || 'missing'} direct_js=${runner?.direct_js_smoke?.result || 'missing'} full_env_js=${runner?.full_env_js_smoke?.result || 'missing'} chrome=${runner?.chrome_extension_smoke?.result || 'missing'}:${runner?.chrome_extension_smoke?.failure_class || 'missing'} json=${repoRel(args.outputJson)} md=${repoRel(args.outputMd)}`);

#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';

const defaults = {
  windowsHost: '10.3.6.59',
  windowsUser: 'LongShine',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-node-repl-active-execs-preflight-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-node-repl-active-execs-preflight-latest.md',
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
        stdout,
        stderr,
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
      ? `sshpass -e ssh <ssh-options> ${args.windowsUser}@${args.windowsHost} <active-execs-preflight>`
      : `ssh <ssh-options> ${args.windowsUser}@${args.windowsHost} <active-execs-preflight>`,
    exit_code: result.code,
    signal: result.signal,
    stdout: result.stdout,
    stderr: sanitizeStderr(result.stderr),
    duration_ms: result.duration_ms,
    passed: result.code === 0,
  };
}

function sanitizeStderr(value) {
  return String(value || '').replace(/Warning: Permanently added .*known hosts\.\s*/g, '').trim();
}

function csvFields(line) {
  const fields = [];
  const re = /"([^"]*)"/g;
  let match;
  while ((match = re.exec(line)) !== null) fields.push(match[1]);
  return fields;
}

function parseTasklist(lines) {
  const processes = [];
  for (const line of lines) {
    const fields = csvFields(line);
    if (fields.length < 2) continue;
    processes.push({
      image_name: fields[0],
      pid: fields[1],
      session_name: fields[2] || null,
      session_number: fields[3] || null,
      memory_usage: fields[4] || null,
    });
  }
  return processes;
}

function parseNetstat(lines) {
  const entries = [];
  for (const line of lines) {
    const text = line.trim();
    if (!text.includes(':19998')) continue;
    const fields = text.split(/\s+/);
    if (fields.length < 4) continue;
    const protocol = fields[0];
    const pid = fields[fields.length - 1];
    const state = fields.length >= 5 ? fields[3] : null;
    entries.push({
      raw: text,
      protocol,
      local_address: fields[1] || null,
      foreign_address: fields[2] || null,
      state,
      pid,
    });
  }
  return entries;
}

function parseActiveExecRecords(lines) {
  const files = [];
  const records = [];
  let activeExecsDirExists = null;
  let currentFile = null;
  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (line === 'active_execs_dir_exists=true') activeExecsDirExists = true;
    if (line === 'active_execs_dir_exists=false') activeExecsDirExists = false;
    if (line.startsWith('active_exec_file=')) {
      currentFile = line.slice('active_exec_file='.length);
      files.push(currentFile);
      continue;
    }
    if (!line.startsWith('{') || !line.endsWith('}')) continue;
    try {
      const parsed = JSON.parse(line);
      records.push({
        file: currentFile,
        version: parsed.version ?? null,
        execId: parsed.execId || null,
        sessionId: parsed.sessionId || null,
        turnId: parsed.turnId || null,
        sandbox: parsed.sandbox || null,
        nodeReplPid: parsed.nodeReplPid ?? null,
        kernelPid: parsed.kernelPid ?? null,
        startedAtMs: parsed.startedAtMs ?? null,
      });
    } catch {
      records.push({
        file: currentFile,
        parse_error: 'invalid json line',
        raw_prefix: line.slice(0, 120),
      });
    }
  }
  return { activeExecsDirExists, files: [...new Set(files)], records };
}

function linesOf(stdout) {
  return String(stdout || '').split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
}

function renderMarkdown(summary) {
  const runtime = summary.runtime || {};
  const lines = [
    '# Windows Node REPL Active Execs Preflight',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Windows host: \`${summary.windows.host}\``,
    `- Windows user: \`${summary.windows.user}\``,
    `- SSH command: \`${summary.ssh.passed ? 'pass' : 'fail'}\``,
    `- Active execs dir exists: \`${runtime.active_execs_dir_exists}\``,
    `- Active exec files: \`${runtime.active_exec_files.length}\``,
    `- Active exec records: \`${runtime.active_exec_records.length}\``,
    `- Running node_repl.exe PIDs: \`${runtime.running_node_repl_pids.join(', ') || 'none'}\``,
    `- Live active exec records: \`${runtime.live_active_exec_records.length}\``,
    `- Stale active exec records: \`${runtime.stale_active_exec_records.length}\``,
    `- 127.0.0.1:19998 entries: \`${runtime.port_19998_entries.length}\``,
    `- Channel status: \`${summary.channel_status}\``,
    '',
    'This preflight is read-only. It proves whether SSH-visible active_execs metadata maps to a currently running Windows node_repl.exe process or a local 19998 proxy listener. It does not execute JavaScript, read browser state, or close Desktop Chrome acceptance.',
    '',
    '## Blockers',
    '',
  ];
  if (summary.blockers.length === 0) lines.push('- none');
  for (const blocker of summary.blockers) lines.push(`- ${blocker}`);
  lines.push('');
  lines.push('## Active Exec Records');
  lines.push('');
  if (runtime.active_exec_records.length === 0) {
    lines.push('- none');
  } else {
    for (const record of runtime.active_exec_records) {
      lines.push(`- \`${record.execId || 'unknown'}\`: nodeReplPid=\`${record.nodeReplPid ?? 'missing'}\` kernelPid=\`${record.kernelPid ?? 'missing'}\` file=\`${record.file || 'unknown'}\``);
    }
  }
  lines.push('');
  return `${lines.join('\n')}\n`;
}

const userRoot = `C:\\Users\\${args.windowsUser}`;
const activeExecDir = `${userRoot}\\.codex\\node_repl\\active_execs`;
const activeExecCommand = [
  'cmd /v:on /c "',
  `if exist ${activeExecDir} (echo active_execs_dir_exists=true)`,
  `& if not exist ${activeExecDir} (echo active_execs_dir_exists=false)`,
  `& if exist ${activeExecDir} (for %F in (${activeExecDir}\\*.json) do @echo active_exec_file=%F)`,
  `& if exist ${activeExecDir} (for %F in (${activeExecDir}\\*.json) do @type %F)`,
  '"',
].join(' ');
const tasklistCommand = 'cmd /c "tasklist /fo csv /nh"';
const netstatCommand = 'cmd /c "netstat -ano"';

const activeResult = await runWindows(activeExecCommand);
const tasklistResult = await runWindows(tasklistCommand);
const netstatResult = await runWindows(netstatCommand);
const activeExecs = parseActiveExecRecords(linesOf(activeResult.stdout));
const processes = parseTasklist(linesOf(tasklistResult.stdout));
const netstat = parseNetstat(linesOf(netstatResult.stdout));
const runningNodeReplPids = processes
  .filter((item) => String(item.image_name || '').toLowerCase() === 'node_repl.exe')
  .map((item) => String(item.pid));
const runningPidSet = new Set(processes.map((item) => String(item.pid)));
const liveActiveExecRecords = activeExecs.records.filter((record) => (
  record.nodeReplPid !== null && runningPidSet.has(String(record.nodeReplPid))
));
const staleActiveExecRecords = activeExecs.records.filter((record) => (
  record.nodeReplPid !== null && !runningPidSet.has(String(record.nodeReplPid))
));
const portListeners = netstat.filter((entry) => String(entry.state || '').toUpperCase() === 'LISTENING');
let channelStatus = 'no_active_exec_metadata_or_proxy_listener';
if (liveActiveExecRecords.length > 0) {
  channelStatus = 'active_exec_metadata_matches_running_node_repl';
} else if (portListeners.length > 0) {
  channelStatus = 'proxy_port_19998_listener_present';
} else if (activeExecs.records.length > 0) {
  channelStatus = 'active_exec_metadata_stale_no_proxy_listener';
}

const blockers = [];
if (!activeResult.passed) blockers.push(`ssh active_execs command failed: ${activeResult.stderr || `exit=${activeResult.exit_code}`}`);
if (!tasklistResult.passed) blockers.push(`ssh tasklist command failed: ${tasklistResult.stderr || `exit=${tasklistResult.exit_code}`}`);
if (!netstatResult.passed) blockers.push(`ssh netstat command failed: ${netstatResult.stderr || `exit=${netstatResult.exit_code}`}`);

const summary = {
  package_id: 'ui_windows_node_repl_active_execs_preflight',
  result: blockers.length === 0 ? 'pass' : 'blocked',
  generated_at: new Date().toISOString(),
  windows: {
    host: args.windowsHost,
    user: args.windowsUser,
    active_execs_dir: activeExecDir,
  },
  ssh: {
    passed: activeResult.passed && tasklistResult.passed && netstatResult.passed,
    commands: {
      active_execs: activeResult.command,
      tasklist: tasklistResult.command,
      netstat: netstatResult.command,
    },
    exit_codes: {
      active_execs: activeResult.exit_code,
      tasklist: tasklistResult.exit_code,
      netstat: netstatResult.exit_code,
    },
    signals: {
      active_execs: activeResult.signal,
      tasklist: tasklistResult.signal,
      netstat: netstatResult.signal,
    },
    stderr: {
      active_execs: activeResult.stderr,
      tasklist: tasklistResult.stderr,
      netstat: netstatResult.stderr,
    },
    duration_ms: {
      active_execs: activeResult.duration_ms,
      tasklist: tasklistResult.duration_ms,
      netstat: netstatResult.duration_ms,
    },
  },
  runtime: {
    active_execs_dir_exists: activeExecs.activeExecsDirExists,
    active_exec_files: activeExecs.files,
    active_exec_records: activeExecs.records,
    running_node_repl_pids: runningNodeReplPids,
    live_active_exec_records: liveActiveExecRecords,
    stale_active_exec_records: staleActiveExecRecords,
    port_19998_entries: netstat,
    port_19998_listeners: portListeners,
    observed_process_count: processes.length,
  },
  channel_status: channelStatus,
  acceptance_effect: 'boundary_evidence_only_not_desktop_chrome_acceptance',
  inference: liveActiveExecRecords.length > 0 || portListeners.length > 0
    ? 'SSH-visible runtime has a candidate Node REPL/proxy channel, but trusted current-session MCP exposure is still required for formal Chrome capture.'
    : 'SSH-visible active_execs metadata is not a reusable trusted channel for this session; use the trusted Windows Codex Desktop / VSCode MCP tool surface.',
  blockers,
};

ensureDirFor(args.outputJson);
fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
ensureDirFor(args.outputMd);
fs.writeFileSync(resolveRepo(args.outputMd), renderMarkdown(summary), 'utf8');

console.log(`ui-windows-node-repl-active-execs-preflight result=${summary.result} channel_status=${summary.channel_status} active_records=${activeExecs.records.length} live_active_records=${liveActiveExecRecords.length} running_node_repl_pids=${runningNodeReplPids.join(',') || 'none'} port_19998_listeners=${portListeners.length} json=${repoRel(args.outputJson)} md=${repoRel(args.outputMd)}`);

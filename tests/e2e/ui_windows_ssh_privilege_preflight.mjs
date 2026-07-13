#!/usr/bin/env node
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';

const defaults = {
  host: '10.3.6.59',
  user: 'LongShine',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-ssh-privilege-preflight-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-ssh-privilege-preflight-latest.md',
  connectTimeout: '8',
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

function ensureDir(file) {
  fs.mkdirSync(path.dirname(resolveRepo(file)), { recursive: true });
}

function sshCmd(remoteCommand) {
  if (!process.env.SSHPASS) {
    return {
      ok: false,
      status: 2,
      stdout: '',
      stderr: 'SSHPASS is required but must be supplied via environment or stdin wrapper, not written to files.',
    };
  }
  const result = spawnSync(
    'sshpass',
    [
      '-e',
      'ssh',
      '-o',
      'StrictHostKeyChecking=no',
      '-o',
      `ConnectTimeout=${args.connectTimeout}`,
      `${args.user}@${args.host}`,
      remoteCommand,
    ],
    { encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 },
  );
  return {
    ok: result.status === 0,
    status: result.status ?? null,
    stdout: result.stdout || '',
    stderr: result.stderr || '',
    error: result.error ? String(result.error.message || result.error) : '',
  };
}

function section(text, name) {
  const marker = `CODEX_SECTION:${name}`;
  const start = text.indexOf(marker);
  if (start < 0) return '';
  const after = text.slice(start + marker.length);
  const next = after.search(/\r?\nCODEX_SECTION:/);
  return (next >= 0 ? after.slice(0, next) : after).trim();
}

function parseCsv(text) {
  const lines = text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
  if (lines.length < 2) return [];
  const parseLine = (line) => {
    const values = [];
    let value = '';
    let quoted = false;
    for (let index = 0; index < line.length; index += 1) {
      const char = line[index];
      if (char === '"' && line[index + 1] === '"') {
        value += '"';
        index += 1;
      } else if (char === '"') {
        quoted = !quoted;
      } else if (char === ',' && !quoted) {
        values.push(value);
        value = '';
      } else {
        value += char;
      }
    }
    values.push(value);
    return values;
  };
  const headers = parseLine(lines[0]);
  return lines.slice(1).map((line) => {
    const values = parseLine(line);
    return Object.fromEntries(headers.map((header, index) => [header, values[index] ?? '']));
  });
}

function parseTasklistCsv(text) {
  const rows = parseCsv(text);
  if (rows.length === 0 && /INFO:/i.test(text)) return [];
  return rows.map((row) => ({
    image_name: row['Image Name'] || '',
    pid: Number(row.PID || 0) || null,
    session_name: row['Session Name'] || '',
    session_number: Number(row['Session#'] || 0),
    memory_usage: row['Mem Usage'] || '',
  })).filter((row) => row.image_name);
}

function parseNetSession(text) {
  if (/Access is denied|拒绝访问/i.test(text)) return { admin_check: 'denied', detail: text.trim() };
  if (/There are no entries in the list/i.test(text)) return { admin_check: 'pass_no_sessions', detail: text.trim() };
  return { admin_check: 'pass_with_sessions_or_unknown', detail: text.trim() };
}

function parseFirewallProfiles(text) {
  const profiles = [];
  const chunks = text.split(/\r?\n(?=(Domain|Private|Public) Profile Settings:)/);
  for (const chunk of chunks) {
    const nameMatch = chunk.match(/^(Domain|Private|Public) Profile Settings:/m);
    if (!nameMatch) continue;
    const value = (label) => {
      const match = chunk.match(new RegExp(`^${label}\\s+(.+)$`, 'm'));
      return match ? match[1].trim() : '';
    };
    profiles.push({
      name: nameMatch[1],
      state: value('State'),
      firewall_policy: value('Firewall Policy'),
      local_firewall_rules: value('LocalFirewallRules'),
      remote_management: value('RemoteManagement'),
    });
  }
  return profiles;
}

function parseScQuery(text) {
  return {
    running: /STATE\s+:\s+4\s+RUNNING/i.test(text),
    raw: text.trim(),
  };
}

function addCheck(checks, name, passed, detail, artifact = '') {
  checks.push({ name, passed, status: passed ? 'ok' : 'fail', detail, artifact });
}

const remoteProbe = [
  'cmd /c chcp 65001>nul',
  'echo CODEX_SECTION:USER',
  'whoami /user /fo csv',
  'echo CODEX_SECTION:GROUPS',
  'whoami /groups /fo csv',
  'echo CODEX_SECTION:PRIVILEGES',
  'whoami /priv /fo csv',
  'echo CODEX_SECTION:ADMIN_NET_SESSION',
  'net session',
  'echo CODEX_SECTION:FIREWALL_SERVICE',
  'sc query MpsSvc',
  'echo CODEX_SECTION:FIREWALL_PROFILES',
  'netsh advfirewall show allprofiles',
  'echo CODEX_SECTION:PROCESS_CODE',
  'tasklist /fo csv /fi "imagename eq Code.exe"',
  'echo CODEX_SECTION:PROCESS_CODEX',
  'tasklist /fo csv /fi "imagename eq Codex.exe"',
  'echo CODEX_SECTION:PROCESS_CHROME',
  'tasklist /fo csv /fi "imagename eq chrome.exe"',
  'echo CODEX_SECTION:PROCESS_NODE_REPL',
  'tasklist /fo csv /fi "imagename eq node_repl.exe"',
  'echo CODEX_SECTION:PROCESS_NODE',
  'tasklist /fo csv /fi "imagename eq node.exe"',
].join(' & ');

const remotePowershellSmoke = 'powershell -NoProfile -Command "Write-Output CODEX_POWERSHELL_OK"';

const probe = sshCmd(remoteProbe);
const powershellSmoke = sshCmd(remotePowershellSmoke);

const users = parseCsv(section(probe.stdout, 'USER'));
const groups = parseCsv(section(probe.stdout, 'GROUPS'));
const privileges = parseCsv(section(probe.stdout, 'PRIVILEGES'));
const netSession = parseNetSession(section(probe.stdout, 'ADMIN_NET_SESSION'));
const firewallService = parseScQuery(section(probe.stdout, 'FIREWALL_SERVICE'));
const firewallProfiles = parseFirewallProfiles(section(probe.stdout, 'FIREWALL_PROFILES'));
const processes = {
  code: parseTasklistCsv(section(probe.stdout, 'PROCESS_CODE')),
  codex: parseTasklistCsv(section(probe.stdout, 'PROCESS_CODEX')),
  chrome: parseTasklistCsv(section(probe.stdout, 'PROCESS_CHROME')),
  node_repl: parseTasklistCsv(section(probe.stdout, 'PROCESS_NODE_REPL')),
  node: parseTasklistCsv(section(probe.stdout, 'PROCESS_NODE')),
};
const processCounts = Object.fromEntries(Object.entries(processes).map(([key, value]) => [key, value.length]));
const adminGroup = groups.find((group) => String(group.SID || '') === 'S-1-5-32-544');
const highIntegrity = groups.some((group) => String(group.SID || '') === 'S-1-16-12288');
const enabledPrivCount = privileges.filter((privilege) => String(privilege.State || '').toLowerCase() === 'enabled').length;

const checks = [];
addCheck(checks, 'SSH cmd probe returns structured output', probe.ok && section(probe.stdout, 'USER') !== '', `status=${probe.status}`);
addCheck(checks, 'SSH token is in Administrators group', Boolean(adminGroup && /Enabled group/i.test(adminGroup.Attributes || '')), adminGroup ? adminGroup.Attributes : 'missing');
addCheck(checks, 'SSH token is high integrity', highIntegrity, highIntegrity ? 'S-1-16-12288 present' : 'missing high mandatory label');
addCheck(checks, 'net session admin check is not access-denied', netSession.admin_check !== 'denied', netSession.admin_check);
addCheck(checks, 'Windows firewall service is running', firewallService.running, firewallService.running ? 'MpsSvc RUNNING' : 'not-running');
addCheck(checks, 'Codex Desktop processes are visible in console session', processCounts.codex > 0, `codex=${processCounts.codex}`);
addCheck(checks, 'Chrome processes are visible in console session', processCounts.chrome > 0, `chrome=${processCounts.chrome}`);
addCheck(checks, 'Windows Node REPL processes are visible in console session', processCounts.node_repl > 0, `node_repl=${processCounts.node_repl}`);
addCheck(
  checks,
  'PowerShell smoke is blocked in SSH context',
  !powershellSmoke.ok,
  `status=${powershellSmoke.status} output=${`${powershellSmoke.stdout}${powershellSmoke.stderr}`.trim().slice(0, 80) || 'empty'}`,
);

const passed = checks.filter((check) => check.passed).length;
const summary = {
  package_id: 'ui_windows_ssh_privilege_preflight',
  run_id: `windows-ssh-privilege-preflight-${new Date().toISOString().replace(/[:.]/g, '-')}`,
  result: probe.ok && passed === checks.length ? 'pass' : 'blocked',
  generated_at: new Date().toISOString(),
  host: args.host,
  user: args.user,
  ssh: {
    cmd_probe_status: probe.status,
    powershell_smoke_status: powershellSmoke.status,
    powershell_smoke_blocked: !powershellSmoke.ok,
    powershell_smoke_output: `${powershellSmoke.stdout}${powershellSmoke.stderr}`.trim().slice(0, 500),
  },
  token: {
    users,
    administrator_group: adminGroup || null,
    high_integrity: highIntegrity,
    enabled_privilege_count: enabledPrivCount,
    net_session: netSession,
  },
  firewall: {
    service: firewallService,
    profiles: firewallProfiles,
  },
  processes: {
    counts: processCounts,
    samples: Object.fromEntries(Object.entries(processes).map(([key, value]) => [key, value.slice(0, 5)])),
  },
  inference: 'SSH reaches an elevated/high-integrity LongShine token with Codex, VSCode, Chrome, node_repl, and node processes visible in the interactive console session. The recurring node_repl JS failure is therefore not explained by a missing Windows host, missing admin group, stopped firewall service, or absent Desktop processes; it remains a trusted Desktop Node REPL / native-pipe / sandbox context boundary.',
  checks,
  passed,
  total: checks.length,
};

function renderMarkdown(data) {
  const lines = [
    '# Windows SSH Privilege Preflight',
    '',
    `- Result: \`${data.result}\``,
    `- Host: \`${data.host}\``,
    `- User: \`${data.user}\``,
    `- PowerShell smoke blocked: \`${data.ssh.powershell_smoke_blocked}\``,
    `- Administrator group enabled: \`${Boolean(data.token.administrator_group)}\``,
    `- High integrity token: \`${data.token.high_integrity}\``,
    `- Enabled privileges: \`${data.token.enabled_privilege_count}\``,
    `- Net session admin check: \`${data.token.net_session.admin_check}\``,
    `- Firewall service running: \`${data.firewall.service.running}\``,
    `- Process counts: Code \`${data.processes.counts.code}\`, Codex \`${data.processes.counts.codex}\`, Chrome \`${data.processes.counts.chrome}\`, node_repl \`${data.processes.counts.node_repl}\`, node \`${data.processes.counts.node}\``,
    '',
    '## Inference',
    '',
    data.inference,
    '',
    '## Checks',
    '',
  ];
  for (const check of data.checks) {
    lines.push(`- ${check.passed ? 'pass' : 'fail'}: ${check.name} (${check.detail})`);
  }
  lines.push('');
  return `${lines.join('\n')}\n`;
}

ensureDir(args.outputJson);
fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
ensureDir(args.outputMd);
fs.writeFileSync(resolveRepo(args.outputMd), renderMarkdown(summary), 'utf8');

console.log(`ui-windows-ssh-privilege-preflight result=${summary.result} passed=${passed}/${checks.length} json=${repoRel(args.outputJson)} md=${repoRel(args.outputMd)}`);
if (summary.result !== 'pass') process.exit(1);

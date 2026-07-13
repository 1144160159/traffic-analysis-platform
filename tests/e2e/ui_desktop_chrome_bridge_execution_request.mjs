#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const defaults = {
  toolCall: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json',
  payloadSelftest: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-selftest-latest.json',
  tunnelPreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-tunnel-channel-preflight-latest.json',
  hostPreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-desktop-bridge-host-preflight-latest.json',
  runtimePreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-codex-bridge-runtime-preflight-latest.json',
  toolSurfacePreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/codex-bridge-tool-surface-preflight-latest.json',
  activeExecsPreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-node-repl-active-execs-preflight-latest.json',
  nodeReplChromeSmoke: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-node-repl-chrome-bridge-smoke-latest.json',
  scheduledChromeSmoke: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-node-repl-scheduled-chrome-smoke-latest.json',
  sshPrivilegePreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-ssh-privilege-preflight-latest.json',
  uiPreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction-preflight-latest.json',
  bridgeRunResult: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-run-latest.json',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-execution-request-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-execution-request-latest.md',
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

function readJson(file) {
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

function statusOk(value) {
  return ['pass', 'passed', 'ok', 'ready', 'ready_for_trusted_context'].includes(String(value || '').toLowerCase());
}

function passResult(state) {
  return state.exists && state.valid && statusOk(state.data?.result || state.data?.status);
}

function checked(id, label, state, passed, detail = '') {
  return {
    id,
    label,
    path: state.path,
    exists: state.exists,
    valid: state.valid,
    passed,
    result: state.data?.result || state.data?.status || null,
    detail: detail || state.error || null,
  };
}

function markdown(summary) {
  const lines = [
    '# Desktop Chrome Bridge Execution Request',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Tool: \`${summary.execution.tool_name}\``,
    `- Timeout: \`${summary.execution.timeout_ms}\` ms`,
    `- Payload SHA256: \`${summary.execution.payload_sha256}\``,
    `- Visual targets: \`${summary.execution.visual_target_count}\``,
    `- Interaction targets: \`${summary.execution.interaction_target_count}\``,
    `- Receiver uploads: \`${summary.execution.receiver_upload_count}\``,
    `- Bridge run result: \`${summary.formal_acceptance.bridge_run_result_exists ? 'present' : 'missing'}\``,
    '',
    'This is an execution request for the trusted Windows Codex Desktop / VSCode Chrome bridge context. It is not visual acceptance evidence by itself.',
    '',
    '## Required Trusted Tool Call',
    '',
    `- Call \`${summary.execution.tool_name}\` with the JSON arguments from \`${summary.execution.tool_call_path}\`.`,
    '- Replace only `<CODEX_CAPTURE_KEY>` and `<CODEX_SMOKE_NONCE>` inside `arguments.code` at execution time.',
    '- Do not write concrete capture keys, smoke nonces, JWTs, or bearer tokens into repo files.',
    '- The payload must use the Chrome extension backend and must not fall back to iab.',
    '',
    '## Readiness Checks',
    '',
    '| check | result | detail |',
    '|---|---|---|',
  ];
  for (const item of summary.readiness_checks) {
    lines.push(`| ${item.label} | ${item.passed ? 'pass' : 'blocked'} | ${item.detail || item.result || ''} |`);
  }
  lines.push('');
  lines.push('## Boundary Evidence');
  lines.push('');
  lines.push('| check | result | detail |');
  lines.push('|---|---|---|');
  for (const item of summary.boundary_checks) {
    lines.push(`| ${item.label} | ${item.passed ? 'pass' : 'missing'} | ${item.detail || item.result || ''} |`);
  }
  lines.push('');
  lines.push('## Formal Closure After Trusted Run');
  lines.push('');
  for (const command of summary.after_trusted_run.commands) lines.push(`- \`${command}\``);
  lines.push('');
  return `${lines.join('\n')}\n`;
}

const states = Object.fromEntries(Object.entries({
  toolCall: args.toolCall,
  payloadSelftest: args.payloadSelftest,
  tunnelPreflight: args.tunnelPreflight,
  hostPreflight: args.hostPreflight,
  runtimePreflight: args.runtimePreflight,
  toolSurfacePreflight: args.toolSurfacePreflight,
  activeExecsPreflight: args.activeExecsPreflight,
  nodeReplChromeSmoke: args.nodeReplChromeSmoke,
  scheduledChromeSmoke: args.scheduledChromeSmoke,
  sshPrivilegePreflight: args.sshPrivilegePreflight,
  uiPreflight: args.uiPreflight,
  bridgeRunResult: args.bridgeRunResult,
}).map(([key, file]) => {
  const state = readJson(file);
  state.path = repoRel(file);
  return [key, state];
}));

const toolCall = states.toolCall.data || {};
const payload = toolCall.payload || {};
const toolArgs = toolCall.arguments || {};
const payloadSelftest = states.payloadSelftest.data || {};
const uiPreflight = states.uiPreflight.data || {};
const runtime = (states.runtimePreflight.data || {}).runtime || {};
const runtimeProcessCounts = runtime.process_counts || {};
const toolSurface = states.toolSurfacePreflight.data || {};
const activeExecs = states.activeExecsPreflight.data || {};
const activeExecsRuntime = activeExecs.runtime || {};
const nodeReplChromeSmoke = states.nodeReplChromeSmoke.data || {};
const scheduledChromeSmoke = states.scheduledChromeSmoke.data || {};

const checks = [
  checked(
    'windows_tunnel_preflight',
    'Windows localhost tunnel endpoints are reachable',
    states.tunnelPreflight,
    passResult(states.tunnelPreflight),
    'requires 127.0.0.1:25173/25174/25175 with Windows proxy bypass',
  ),
  checked(
    'windows_host_preflight',
    'Windows host has Chrome/Codex/VSCode and bridge client files',
    states.hostPreflight,
    passResult(states.hostPreflight),
    `chrome_clients=${states.hostPreflight.data?.inventory?.chrome_clients?.length ?? 'missing'} chrome_processes=${states.hostPreflight.data?.inventory?.processes?.chrome ?? 'missing'}`,
  ),
  checked(
    'windows_runtime_preflight',
    'Windows Codex bridge runtime prerequisites are present',
    states.runtimePreflight,
    passResult(states.runtimePreflight) &&
      Array.isArray(runtime.chrome_clients) &&
      runtime.chrome_clients.length > 0 &&
      Number(runtimeProcessCounts.chrome) > 0 &&
      Number(runtimeProcessCounts.codex) > 0 &&
      Number(runtimeProcessCounts.code) > 0,
    `chrome_clients=${runtime.chrome_clients?.length ?? 'missing'} processes=${JSON.stringify(runtimeProcessCounts)}`,
  ),
  checked(
    'ssh_privilege_preflight',
    'Windows SSH privilege probe narrows ordinary-SSH boundary',
    states.sshPrivilegePreflight,
    passResult(states.sshPrivilegePreflight),
    `high_integrity=${states.sshPrivilegePreflight.data?.token?.high_integrity ?? 'missing'} privileges=${states.sshPrivilegePreflight.data?.token?.enabled_privilege_count ?? 'missing'}`,
  ),
  checked(
    'payload_selftest',
    'Desktop Chrome bridge payload self-test passes',
    states.payloadSelftest,
    passResult(states.payloadSelftest) &&
      Number(payloadSelftest.passed) === Number(payloadSelftest.total) &&
      Number(payloadSelftest.total) > 0,
    `checks=${payloadSelftest.passed ?? 'missing'}/${payloadSelftest.total ?? 'missing'}`,
  ),
  checked(
    'tool_call_template',
    'Outer MCP tool-call template is ready',
    states.toolCall,
    passResult(states.toolCall) &&
      toolCall.tool_name === 'mcp__codex_desktop_node_repl__js' &&
      Number(toolCall.passed) === Number(toolCall.total) &&
      Number(toolArgs.timeout_ms) >= 600000 &&
      typeof payload.sha256 === 'string' &&
      payload.sha256.length === 64,
    `tool=${toolCall.tool_name || 'missing'} checks=${toolCall.passed ?? 'missing'}/${toolCall.total ?? 'missing'}`,
  ),
  checked(
    'ui_preflight_batch',
    'UI preflight is aligned to the Windows tunnel capture batch',
    states.uiPreflight,
    states.uiPreflight.exists &&
      states.uiPreflight.valid &&
      uiPreflight.capture_session_covers_current_gaps === true &&
      uiPreflight.payload_selftest_ready === true &&
      uiPreflight.bridge_tool_call_ready === true,
    `ui_result=${uiPreflight.result || 'missing'} visual=${uiPreflight.visual_diff_passed_count ?? 'missing'}/${uiPreflight.visual_diff_required_count ?? 'missing'} interaction=${uiPreflight.interaction_passed_count ?? 'missing'}/${uiPreflight.interaction_required_count ?? 'missing'}`,
  ),
];

const allReady = checks.every((item) => item.passed);
const bridgeRunExists = states.bridgeRunResult.exists && states.bridgeRunResult.valid;
const boundaryChecks = [
  checked(
    'codex_bridge_tool_surface',
    'Local Codex bridge plugin/proxy tool surface is diagnosed',
    states.toolSurfacePreflight,
    states.toolSurfacePreflight.exists &&
      states.toolSurfacePreflight.valid &&
      statusOk(toolSurface.result || toolSurface.status),
    `plugin=${toolSurface.plugin?.installed_enabled ?? 'missing'} backend_open=${toolSurface.backend?.open ?? 'missing'} tools_list=${toolSurface.proxy?.tools_list_status || 'missing'} session_tools=${toolSurface.codex_session?.desktop_tool_status || 'missing'}`,
  ),
  checked(
    'windows_node_repl_active_execs',
    'Windows active_execs/proxy channel probe is current',
    states.activeExecsPreflight,
    states.activeExecsPreflight.exists &&
      states.activeExecsPreflight.valid &&
      statusOk(activeExecs.result || activeExecs.status),
    `channel=${activeExecs.channel_status || 'missing'} active_records=${activeExecsRuntime.active_exec_records?.length ?? 'missing'} live_records=${activeExecsRuntime.live_active_exec_records?.length ?? 'missing'} node_repl_pids=${activeExecsRuntime.running_node_repl_pids?.join(',') || 'missing'} port19998=${activeExecsRuntime.port_19998_listeners?.length ?? 'missing'}`,
  ),
  checked(
    'windows_node_repl_chrome_smoke',
    'Windows node_repl stdio JS and Chrome trust boundary smoke is current',
    states.nodeReplChromeSmoke,
    states.nodeReplChromeSmoke.exists &&
      states.nodeReplChromeSmoke.valid &&
      nodeReplChromeSmoke.direct_js_smoke?.result === 'pass' &&
      typeof nodeReplChromeSmoke.chrome_extension_no_env_smoke?.failure_class === 'string' &&
      typeof nodeReplChromeSmoke.chrome_extension_smoke?.failure_class === 'string',
    `result=${nodeReplChromeSmoke.result || 'missing'} direct_js=${nodeReplChromeSmoke.direct_js_smoke?.result || 'missing'} full_env_js=${nodeReplChromeSmoke.full_env_js_smoke?.result || 'missing'} chrome_no_env=${nodeReplChromeSmoke.chrome_extension_no_env_smoke?.result || 'missing'}:${nodeReplChromeSmoke.chrome_extension_no_env_smoke?.failure_class || 'missing'} chrome=${nodeReplChromeSmoke.chrome_extension_smoke?.result || 'missing'}:${nodeReplChromeSmoke.chrome_extension_smoke?.failure_class || 'missing'}`,
  ),
  checked(
    'windows_node_repl_scheduled_chrome_smoke',
    'Windows scheduled-task node_repl Chrome bridge smoke is diagnosed',
    states.scheduledChromeSmoke,
    states.scheduledChromeSmoke.exists &&
      states.scheduledChromeSmoke.valid &&
      typeof scheduledChromeSmoke.result === 'string' &&
      Array.isArray(scheduledChromeSmoke.blockers),
    `result=${scheduledChromeSmoke.result || 'missing'} create=${scheduledChromeSmoke.scheduled_task?.create?.ok ?? 'missing'} run=${scheduledChromeSmoke.scheduled_task?.run?.ok ?? 'missing'} runner=${scheduledChromeSmoke.runner?.result || 'missing'} blocker=${(scheduledChromeSmoke.blockers || [])[0] || 'none'}`,
  ),
];
const summary = {
  package_id: 'desktop_chrome_bridge_execution_request',
  result: allReady && !bridgeRunExists ? 'ready_for_trusted_context' : allReady ? 'bridge_run_present_review_required' : 'blocked',
  generated_at: new Date().toISOString(),
  readiness_checks: checks,
  boundary_checks: boundaryChecks,
  execution: {
    trusted_context_required: true,
    tool_name: toolCall.tool_name || 'mcp__codex_desktop_node_repl__js',
    timeout_ms: toolArgs.timeout_ms ?? null,
    tool_call_path: repoRel(args.toolCall),
    payload_js_path: payload.js || null,
    payload_json_path: payload.json || null,
    payload_sha256: payload.sha256 || null,
    visual_target_count: payload.visual_target_count ?? null,
    interaction_target_count: payload.interaction_target_count ?? null,
    receiver_upload_count: payload.receiver_upload_count ?? null,
    placeholder_policy: toolArgs.code_placeholder_policy || null,
  },
  formal_acceptance: {
    bridge_run_result_path: repoRel(args.bridgeRunResult),
    bridge_run_result_exists: bridgeRunExists,
    active_execs_channel_status: activeExecs.channel_status || null,
    node_repl_chrome_smoke_result: nodeReplChromeSmoke.result || null,
    node_repl_direct_js_result: nodeReplChromeSmoke.direct_js_smoke?.result || null,
    node_repl_chrome_no_env_failure_class: nodeReplChromeSmoke.chrome_extension_no_env_smoke?.failure_class || null,
    node_repl_chrome_full_env_failure_class: nodeReplChromeSmoke.chrome_extension_smoke?.failure_class || null,
    scheduled_chrome_smoke_result: scheduledChromeSmoke.result || null,
    scheduled_chrome_task_create_ok: scheduledChromeSmoke.scheduled_task?.create?.ok ?? null,
    scheduled_chrome_first_blocker: (scheduledChromeSmoke.blockers || [])[0] || null,
    codex_bridge_tool_surface_status: toolSurface.codex_session?.desktop_tool_status || null,
    codex_bridge_backend_open: toolSurface.backend?.open ?? null,
    current_visual_diff: `${uiPreflight.visual_diff_passed_count ?? 0}/${uiPreflight.visual_diff_required_count ?? 30}`,
    current_business_interaction: `${uiPreflight.interaction_passed_count ?? 0}/${uiPreflight.interaction_required_count ?? 28}`,
    completion_status: bridgeRunExists ? 'review bridge run and rerun formal gates' : 'not accepted until trusted Chrome bridge run uploads evidence',
  },
  after_trusted_run: {
    commands: [
      'RUN_ID=<run-id> ALLOW_BLOCKERS=false python3 tests/e2e/ui_visual_interaction_evidence_finalize.py --capture-plan doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json',
      'RUN_ID=<run-id> CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json DESKTOP_CHROME_STATUS=pass ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh',
      'RUN_ID=<run-id> ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh',
    ],
  },
};

ensureDirFor(args.outputJson);
fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`);
fs.writeFileSync(resolveRepo(args.outputMd), markdown(summary));

console.log(JSON.stringify({
  ok: summary.result !== 'blocked',
  result: summary.result,
  output_json: repoRel(args.outputJson),
  output_md: repoRel(args.outputMd),
}, null, 2));

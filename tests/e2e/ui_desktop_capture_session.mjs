#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const defaults = {
  sessionId: `ui-desktop-capture-session-${new Date().toISOString().replace(/[:.]/g, '-')}`,
  capturePlan: 'doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-latest.json',
  gapReport: 'doc/02_acceptance/02-regression/ui-visual-interaction-gap-report-latest.json',
  desktopBridge: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-latest.json',
  bridgeHostPreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-desktop-bridge-host-preflight-latest.json',
  bridgeRuntimePreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-codex-bridge-runtime-preflight-latest.json',
  viewportProbe: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-viewport-probe-latest.json',
  receiverSelftest: 'doc/02_acceptance/02-regression/ui-visual-interaction/receiver-selftest-latest.json',
  smokeTokenPreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-smoke-token-preflight-latest.json',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-latest.md',
  receiverUrl: 'http://10.0.5.8:15174',
  smokeRedirectBaseUrl: 'http://10.0.5.8:15175',
  windowsHost: '10.3.6.59',
  windowsUser: 'LongShine',
  appWindowsPort: '30180',
  appLocalPort: '5173',
  receiverWindowsPort: '15174',
  redirectWindowsPort: '15175',
  receiverPort: '15174',
  redirectPort: '15175',
  maxVisualTargets: 0,
  maxInteractionRoutes: 0,
};

const args = parseArgs(process.argv.slice(2));
const config = { ...defaults, ...args };
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

function limitItems(items, max) {
  const count = Number(max);
  if (!Number.isFinite(count) || count <= 0) return items;
  return items.slice(0, count);
}

function shellQuote(value) {
  const text = String(value);
  if (/^[A-Za-z0-9_/:=.,@%+~-]+$/.test(text)) return text;
  return `'${text.replace(/'/g, `'\\''`)}'`;
}

function commandLine(parts) {
  return parts.filter(Boolean).map((part) => {
    const text = String(part);
    const match = text.match(/^([A-Za-z_][A-Za-z0-9_]*)=(.*)$/);
    if (match) return `${match[1]}=${shellQuote(match[2])}`;
    return shellQuote(text);
  }).join(' ');
}

function normalizeReceiverUpload(value, kind, targetId) {
  if (value) return value;
  return `${String(config.receiverUrl).replace(/\/+$/, '')}/${kind}/${targetId}`;
}

function safeWrapperUrl(call) {
  return call?.args?.url || '';
}

function reasonText(reasons) {
  if (!Array.isArray(reasons) || reasons.length === 0) return 'none';
  return reasons.join('; ');
}

function statusFromBridge(bridge) {
  if (!bridge.exists) return 'desktop_bridge_evidence_missing';
  if (!bridge.valid) return 'desktop_bridge_evidence_invalid';
  const result = String(bridge.data?.result || bridge.data?.status || '').toLowerCase();
  if (['pass', 'passed', 'ok'].includes(result)) return 'ready_for_desktop_capture';
  const detail = JSON.stringify(bridge.data || {});
  if (detail.includes('Transport closed')) return 'blocked_desktop_transport_closed';
  return result ? `desktop_bridge_${result}` : 'desktop_bridge_not_pass';
}

function finiteNumber(value) {
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
}

function sizeFrom(width, height) {
  const normalizedWidth = finiteNumber(width);
  const normalizedHeight = finiteNumber(height);
  if (normalizedWidth === null || normalizedHeight === null) return null;
  return { width: normalizedWidth, height: normalizedHeight };
}

function normalizeViewportProbe(state) {
  const data = state.valid && state.data && typeof state.data === 'object' ? state.data : {};
  const windowMetrics = data.window_metrics || data.windowMetrics || {};
  const screenshot = data.screenshot || {};
  const viewportSize = sizeFrom(data.viewport?.width ?? data.width, data.viewport?.height ?? data.height);
  const windowMetricSize = sizeFrom(
    windowMetrics.inner_width ?? windowMetrics.innerWidth ?? windowMetrics.visual_viewport_width ?? windowMetrics.visualViewportWidth,
    windowMetrics.inner_height ?? windowMetrics.innerHeight ?? windowMetrics.visual_viewport_height ?? windowMetrics.visualViewportHeight,
  );
  const screenshotSize = sizeFrom(screenshot.width, screenshot.height);
  const observedSize = viewportSize || windowMetricSize || screenshotSize;
  const expectedSize = sizeFrom(
    data.expected_size?.width ?? data.expectedSize?.width,
    data.expected_size?.height ?? data.expectedSize?.height,
  ) || { width: 1920, height: 1080 };
  const result = data.result || data.status || null;
  const viewportSource = viewportSize ? 'viewport' : (windowMetricSize ? 'window_metrics' : (screenshotSize ? 'screenshot' : null));
  const mismatchReason = observedSize && (observedSize.width !== expectedSize.width || observedSize.height !== expectedSize.height)
    ? `viewport ${observedSize.width}x${observedSize.height} != ${expectedSize.width}x${expectedSize.height}`
    : null;
  return {
    exists: state.exists,
    valid: state.valid,
    result,
    viewport: observedSize,
    viewport_source: viewportSource,
    window_metrics: windowMetricSize,
    screenshot_size: screenshotSize,
    expected_size: expectedSize,
    mismatch_reason: mismatchReason,
    acceptance_effect: data.acceptance_effect || null,
  };
}

const planState = readJsonState(config.capturePlan);
const gapState = readJsonState(config.gapReport);
const bridgeState = readJsonState(config.desktopBridge);
const bridgeHostPreflightState = readJsonState(config.bridgeHostPreflight);
const bridgeRuntimePreflightState = readJsonState(config.bridgeRuntimePreflight);
const viewportProbeState = readJsonState(config.viewportProbe);
const viewportProbe = normalizeViewportProbe(viewportProbeState);
const receiverSelftestState = readJsonState(config.receiverSelftest);
const smokeTokenPreflightState = readJsonState(config.smokeTokenPreflight);
if (!planState.valid) {
  throw new Error(`capture plan is required and must be valid JSON: ${config.capturePlan} ${planState.error || ''}`);
}

const plan = planState.data;
const planVisualTargets = Array.isArray(plan.visual_targets) ? plan.visual_targets : [];
const planInteractions = Array.isArray(plan.interactions) ? plan.interactions : [];
const gapVisualItems = Array.isArray(gapState.data?.visual_gaps) ? gapState.data.visual_gaps : [];
const gapInteractionItems = Array.isArray(gapState.data?.interaction_gaps) ? gapState.data.interaction_gaps : [];
const planVisualById = new Map(planVisualTargets.map((target) => [target.target_id, target]));
const planInteractionById = new Map(planInteractions.map((route) => [route.route_id, route]));

function gapNeedsSmoke(value) {
  return String(value || '').includes('SMOKE_REDIRECT_BASE_URL') || String(value || '').includes('CODEX_SMOKE_NONCE');
}

function visualFromGap(gap) {
  const targetId = gap.target_id || gap.id;
  const planned = planVisualById.get(targetId) || {};
  return {
    ...planned,
    target_id: targetId,
    route_id: gap.route_id || planned.route_id,
    title: planned.title || gap.title || targetId,
    url: planned.url || gap.url,
    requires_smoke_token: planned.requires_smoke_token ?? gapNeedsSmoke(gap.safe_redirect_url_pattern),
    safe_redirect_url_pattern: planned.safe_redirect_url_pattern || gap.safe_redirect_url_pattern,
    safe_wrapper_call: planned.safe_wrapper_call || gap.safe_wrapper_call,
    receiver_upload: planned.receiver_upload,
    source_image: gap.source_image || planned.source_image,
    evidence: {
      ...(planned.evidence || {}),
      passed: false,
      reasons: gap.reasons || planned.evidence?.reasons || [],
    },
    metrics_command: gap.metrics_command || planned.metrics_command,
  };
}

function interactionFromGap(gap) {
  const routeId = gap.route_id || gap.id;
  const planned = planInteractionById.get(routeId) || {};
  return {
    ...planned,
    route_id: routeId,
    title: planned.title || gap.title || routeId,
    route: planned.route || gap.route,
    resolved_path: planned.resolved_path || gap.expected_final_path,
    requires_smoke_token: planned.requires_smoke_token ?? gapNeedsSmoke(gap.safe_redirect_url_pattern),
    safe_redirect_url_pattern: planned.safe_redirect_url_pattern || gap.safe_redirect_url_pattern,
    safe_wrapper_call: planned.safe_wrapper_call || gap.safe_wrapper_call,
    receiver_upload: planned.receiver_upload,
    interaction_screenshot_upload: planned.interaction_screenshot_upload,
    interaction_template: planned.interaction_template || gap.interaction_template,
    output_path: planned.evidence?.interaction || gap.output_path,
    api_endpoints: planned.api_endpoints || gap.api_endpoints || [],
    business_action_hint: planned.business_action_hint || gap.business_action_hint,
    interaction_requirements: planned.interaction_requirements,
    evidence: {
      ...(planned.evidence || {}),
      interaction: planned.evidence?.interaction || gap.output_path,
      passed: false,
      reasons: gap.reasons || planned.evidence?.reasons || [],
    },
  };
}

const visualPending = gapVisualItems.length > 0
  ? gapVisualItems.map(visualFromGap)
  : planVisualTargets.filter((target) => target?.evidence?.passed !== true);
const interactionPending = gapInteractionItems.length > 0
  ? gapInteractionItems.map(interactionFromGap)
  : planInteractions.filter((route) => route?.evidence?.passed !== true);
const visualBatch = limitItems(visualPending, config.maxVisualTargets).map((target, index) => ({
  order: index + 1,
  target_id: target.target_id,
  route_id: target.route_id,
  title: target.title,
  url: target.url,
  requires_smoke_token: target.requires_smoke_token,
  safe_redirect_url_pattern: target.safe_redirect_url_pattern || safeWrapperUrl(target.safe_wrapper_call),
  safe_wrapper_call: target.safe_wrapper_call,
  receiver_upload: normalizeReceiverUpload(target.receiver_upload, 'upload', target.target_id),
  source_image: target.source_image,
  evidence: target.evidence,
  metrics_command: Array.isArray(target.metrics_command) ? commandLine(target.metrics_command) : target.metrics_command,
  reasons: target.evidence?.reasons || [],
}));

const interactionBatch = limitItems(interactionPending, config.maxInteractionRoutes).map((route, index) => ({
  order: index + 1,
  route_id: route.route_id,
  title: route.title,
  route: route.route,
  requires_smoke_token: route.requires_smoke_token,
  expected_final_path: route.interaction_requirements?.expected_final_path || route.resolved_path,
  safe_redirect_url_pattern: route.safe_redirect_url_pattern,
  safe_wrapper_call: route.safe_wrapper_call,
  receiver_upload: normalizeReceiverUpload(route.receiver_upload, 'interaction', route.route_id),
  interaction_screenshot_upload: normalizeReceiverUpload(route.interaction_screenshot_upload, 'interaction-screenshot', route.route_id),
  interaction_template: route.interaction_template,
  output_path: route.evidence?.interaction,
  api_endpoints: route.api_endpoints || [],
  business_action_hint: route.business_action_hint,
  interaction_requirements: route.interaction_requirements,
  reasons: route.evidence?.reasons || [],
}));

const screenshotUploadCount = visualBatch.length + interactionBatch.length;
const bridgeResultUploadCount = screenshotUploadCount > 0 ? 1 : 0;
const receiverUploadCount = screenshotUploadCount + bridgeResultUploadCount;
const bridgeResultUpload = config.receiverUrl && config.receiverUrl !== '<receiver-url>'
  ? `${String(config.receiverUrl).replace(/\/+$/, '')}/bridge-result`
  : '<receiver-url>/bridge-result';

const receiverStart = commandLine([
  'DESKTOP_SMOKE_TOKEN=<redacted>',
  'CODEX_CAPTURE_KEY=<redacted>',
  'tests/e2e/ui_desktop_capture_receiver.py',
  '--host', '0.0.0.0',
  '--port', config.receiverPort,
  '--evidence-dir', plan.evidence_dir || 'doc/02_acceptance/02-regression/ui-visual-interaction/latest',
  '--max-uploads', String(Math.max(receiverUploadCount, 1)),
  '--expected-width', '1920',
  '--expected-height', '1080',
]);

const redirectOpenCount = visualBatch.filter((item) => item.requires_smoke_token).length
  + interactionBatch.filter((item) => item.requires_smoke_token).length;

const redirectStart = commandLine([
  'DESKTOP_SMOKE_TOKEN=<redacted>',
  'CODEX_SMOKE_NONCE=<redacted>',
  'tests/e2e/ui_desktop_smoke_redirect.py',
  '--host', '0.0.0.0',
  '--port', config.redirectPort,
  '--app-base-url', plan.base_url || 'http://10.0.5.8:30180',
  '--default-route', '/dashboard',
  '--max-redirects', String(Math.max(redirectOpenCount, 1)),
]);

const capturePlanCommand = commandLine([
  'tests/e2e/ui_desktop_capture_plan.mjs',
  '--base-url', plan.base_url || 'http://10.0.5.8:30180',
  '--receiver-url', config.receiverUrl,
]);

const appReverseTunnel = commandLine([
  'ssh',
  '-N',
  '-R',
  `127.0.0.1:${config.appWindowsPort}:127.0.0.1:${config.appLocalPort}`,
  `${config.windowsUser}@${config.windowsHost}`,
]);

const evidenceReverseTunnel = commandLine([
  'ssh',
  '-N',
  '-R',
  `127.0.0.1:${config.receiverWindowsPort}:127.0.0.1:${config.receiverPort}`,
  '-R',
  `127.0.0.1:${config.redirectWindowsPort}:127.0.0.1:${config.redirectPort}`,
  `${config.windowsUser}@${config.windowsHost}`,
]);

const viewportProbeUrl = `${String(config.receiverUrl).replace(/\/+$/, '')}/viewport-probe`;
const viewportProbeCommand = config.receiverUrl && config.receiverUrl !== '<receiver-url>'
  ? `mcp__codex_desktop_node_repl.desktop_chrome_open_url url=${viewportProbeUrl} keep=true wait_ms=1500`
  : 'start receiver with a concrete --receiver-url, then open <receiver-url>/viewport-probe using mcp__codex_desktop_node_repl.desktop_chrome_open_url';

const session = {
  package_id: 'ui_desktop_capture_session',
  run_id: config.sessionId,
  session_id: config.sessionId,
  generated_at: new Date().toISOString(),
  status: statusFromBridge(bridgeState),
  note: 'Execution package only. It does not prove visual or interaction acceptance until real Codex Desktop Chrome evidence is captured and the dual gate passes.',
  sources: {
    capture_plan: {
      path: repoRel(config.capturePlan),
      exists: planState.exists,
      valid: planState.valid,
      generated_at: plan.generated_at || null,
      summary: plan.summary || null,
    },
    gap_report: {
      path: repoRel(config.gapReport),
      exists: gapState.exists,
      valid: gapState.valid,
      run_id: gapState.data?.run_id || null,
      summary: gapState.data?.summary || null,
      reason_groups: gapState.data?.reason_groups || null,
    },
    desktop_bridge: {
      path: repoRel(config.desktopBridge),
      exists: bridgeState.exists,
      valid: bridgeState.valid,
      result: bridgeState.data?.result || null,
      detail: bridgeState.data?.detail || bridgeState.data?.error || null,
    },
    bridge_host_preflight: {
      path: repoRel(config.bridgeHostPreflight),
      exists: bridgeHostPreflightState.exists,
      valid: bridgeHostPreflightState.valid,
      result: bridgeHostPreflightState.data?.result || null,
      selected_chrome_client_url: bridgeHostPreflightState.data?.selected_chrome_client_url || null,
      chrome_client_count: Array.isArray(bridgeHostPreflightState.data?.inventory?.chrome_clients)
        ? bridgeHostPreflightState.data.inventory.chrome_clients.length
        : null,
      chrome_process_count: bridgeHostPreflightState.data?.inventory?.processes?.chrome ?? null,
      codex_process_count: bridgeHostPreflightState.data?.inventory?.processes?.codex ?? null,
    },
    bridge_runtime_preflight: {
      path: repoRel(config.bridgeRuntimePreflight),
      exists: bridgeRuntimePreflightState.exists,
      valid: bridgeRuntimePreflightState.valid,
      result: bridgeRuntimePreflightState.data?.result || null,
      chrome_client_count: Array.isArray(bridgeRuntimePreflightState.data?.runtime?.chrome_clients)
        ? bridgeRuntimePreflightState.data.runtime.chrome_clients.length
        : null,
      desktop_bridge_candidate_count: Array.isArray(bridgeRuntimePreflightState.data?.runtime?.desktop_bridge_candidate_dirs)
        ? bridgeRuntimePreflightState.data.runtime.desktop_bridge_candidate_dirs.length
        : null,
      node_repl_candidate_count: Array.isArray(bridgeRuntimePreflightState.data?.runtime?.node_repl_candidate_dirs)
        ? bridgeRuntimePreflightState.data.runtime.node_repl_candidate_dirs.length
        : null,
      chrome_process_count: bridgeRuntimePreflightState.data?.runtime?.process_counts?.chrome ?? null,
      codex_process_count: bridgeRuntimePreflightState.data?.runtime?.process_counts?.codex ?? null,
      code_process_count: bridgeRuntimePreflightState.data?.runtime?.process_counts?.code ?? null,
    },
    viewport_probe: {
      path: repoRel(config.viewportProbe),
      ...viewportProbe,
    },
    receiver_selftest: {
      path: repoRel(config.receiverSelftest),
      exists: receiverSelftestState.exists,
      valid: receiverSelftestState.valid,
      result: receiverSelftestState.data?.result || null,
      passed: receiverSelftestState.data?.passed ?? null,
      total: receiverSelftestState.data?.total ?? null,
      acceptance_effect: receiverSelftestState.data?.acceptance_effect || null,
    },
    smoke_token_preflight: {
      path: repoRel(config.smokeTokenPreflight),
      exists: smokeTokenPreflightState.exists,
      valid: smokeTokenPreflightState.valid,
      result: smokeTokenPreflightState.data?.result || null,
      runtime: smokeTokenPreflightState.data?.runtime || null,
      final_path: smokeTokenPreflightState.data?.final_path || null,
      acceptance_effect: smokeTokenPreflightState.data?.reason || null,
    },
  },
  summary: {
    visual_target_count: gapState.data?.summary?.visual_required_count ?? planVisualTargets.length,
    visual_passed_count: gapState.data?.summary?.visual_passed_count ?? plan.summary?.visual_passed_count ?? null,
    visual_pending_count: visualPending.length,
    visual_batch_count: visualBatch.length,
    interaction_route_count: gapState.data?.summary?.interaction_required_count ?? planInteractions.length,
    interaction_passed_count: gapState.data?.summary?.interaction_passed_count ?? plan.summary?.interaction_passed_count ?? null,
    interaction_pending_count: interactionPending.length,
    interaction_batch_count: interactionBatch.length,
    screenshot_upload_count: screenshotUploadCount,
    bridge_result_upload_count: bridgeResultUploadCount,
    receiver_upload_count: receiverUploadCount,
    smoke_redirect_open_count: redirectOpenCount,
  },
  acceptance_contract: {
    desktop_backend: 'codex-desktop-chrome-extension',
    forbidden_backends: ['iab'],
    expected_viewport: plan.expected_viewport || { width: 1920, height: 1080 },
    required_visual_files: ['actual-1920.png', 'diff-1920.png', 'metrics.json', 'capture-meta.json'],
    required_interaction_files: ['interaction.json', 'interaction.png', 'interaction-capture-meta.json'],
    capture_meta_must_prove: [
      'backend is codex-desktop-chrome-extension',
      'uploaded screenshot is 1920x1080',
      'stored screenshot is 1920x1080',
      'Desktop Chrome viewport is 1920x1080',
      'post_capture_resize is false',
    ],
    pre_capture_viewport_calibration: [
      'run ui_desktop_capture_receiver_selftest.py',
      'start ui_desktop_capture_receiver.py',
      'open /viewport-probe using mcp__codex_desktop_node_repl.desktop_chrome_open_url',
      'confirm desktop-chrome-viewport-probe-latest.json result is pass',
      'do not upload visual screenshots while the probe reports any viewport other than 1920x1080',
    ],
    interaction_must_prove: [
      'Desktop Chrome backend status is pass',
      'no 4xx or 5xx',
      'no requestfailed',
      'no pageerror',
      'no console error',
      'route-specific business action was performed',
      'protected route hash was consumed',
      'final URL does not contain smoke token material',
      'protected route did not resolve to /login',
      'interaction screenshot is 1920x1080',
      'interaction-capture-meta proves Desktop Chrome backend and no post-capture resize',
      'Desktop Chrome bridge run summary is uploaded to desktop-chrome-bridge-run-latest.json',
    ],
  },
  commands: {
    direct_app_url: plan.base_url || 'http://10.0.5.8:30180',
    direct_receiver_url: config.receiverUrl,
    direct_smoke_redirect_url: config.smokeRedirectBaseUrl,
    receiver_selftest: 'python3 tests/e2e/ui_desktop_capture_receiver_selftest.py',
    smoke_token_preflight: 'node tests/e2e/ui_desktop_smoke_token_preflight.mjs --base-url http://10.0.5.8:30180 --apisix-url http://10.0.5.8:30180 --route /dashboard --expected-path /dashboard',
    receiver_start: receiverStart,
    viewport_probe_open: viewportProbeCommand,
    smoke_redirect_start: redirectStart,
    bridge_result_upload: bridgeResultUpload,
    capture_plan_refresh: capturePlanCommand,
    evidence_finalize: 'ALLOW_BLOCKERS=false tests/e2e/ui_visual_interaction_evidence_finalize.py',
    ui_visual_interaction_preflight: 'DESKTOP_CHROME_STATUS=pass ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh',
    project_completion_audit: 'ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh',
  },
  visual_batch: visualBatch,
  interaction_batch: interactionBatch,
  bridge_result_upload: bridgeResultUpload,
  viewport_calibration: {
    required: true,
    url: config.receiverUrl && config.receiverUrl !== '<receiver-url>' ? viewportProbeUrl : '<receiver-url>/viewport-probe',
    command: viewportProbeCommand,
    latest_probe: {
      path: repoRel(config.viewportProbe),
      ...viewportProbe,
    },
    failure_effect: 'If this probe is missing or blocked, captured actual screenshots are expected to fail capture-meta size checks.',
  },
};

ensureDirFor(config.outputJson);
fs.writeFileSync(resolveRepo(config.outputJson), `${JSON.stringify(session, null, 2)}\n`, 'utf8');
ensureDirFor(config.outputMd);
fs.writeFileSync(resolveRepo(config.outputMd), renderMarkdown(session), 'utf8');

console.log(`ui-desktop-capture-session status=${session.status} visual_batch=${session.summary.visual_batch_count}/${session.summary.visual_pending_count} interaction_batch=${session.summary.interaction_batch_count}/${session.summary.interaction_pending_count} json=${repoRel(config.outputJson)} md=${repoRel(config.outputMd)}`);

function mdCell(value) {
  return String(value ?? '').replace(/\|/g, '\\|').replace(/\n/g, '<br>');
}

function renderMarkdown(data) {
  const lines = [];
  lines.push('# UI Desktop Capture Session');
  lines.push('');
  lines.push(`- Session ID: \`${data.session_id}\``);
  lines.push(`- Status: \`${data.status}\``);
  lines.push(`- Generated: \`${data.generated_at}\``);
  lines.push(`- Capture plan: \`${data.sources.capture_plan.path}\``);
  lines.push(`- Gap report: \`${data.sources.gap_report.path}\``);
  lines.push(`- Windows bridge host preflight: \`${data.sources.bridge_host_preflight.result || (data.sources.bridge_host_preflight.exists ? 'present' : 'missing')}\``);
  lines.push(`- Windows bridge runtime preflight: \`${data.sources.bridge_runtime_preflight.result || (data.sources.bridge_runtime_preflight.exists ? 'present' : 'missing')}\``);
  lines.push(`- Receiver self-test: \`${data.sources.receiver_selftest.result || (data.sources.receiver_selftest.exists ? 'present' : 'missing')}\``);
  lines.push(`- Smoke token preflight: \`${data.sources.smoke_token_preflight.result || (data.sources.smoke_token_preflight.exists ? 'present' : 'missing')}\``);
  lines.push(`- Visual pending: \`${data.summary.visual_pending_count}\``);
  lines.push(`- Interaction pending: \`${data.summary.interaction_pending_count}\``);
  lines.push(`- Viewport calibration: \`${data.viewport_calibration.latest_probe.result || (data.viewport_calibration.latest_probe.exists ? 'present' : 'missing')}\``);
  lines.push('');
  lines.push('This package is a Desktop Chrome execution queue. It is not acceptance evidence and cannot close the dual gate by itself.');
  lines.push('');
  lines.push('## Commands');
  lines.push('');
  for (const [name, command] of Object.entries(data.commands)) {
    lines.push(`### ${name}`);
    lines.push('');
    lines.push('```bash');
    lines.push(command);
    lines.push('```');
    lines.push('');
  }
  lines.push('## Visual Batch');
  lines.push('');
  lines.push('| Order | Target | Route | Receiver Upload | Reasons |');
  lines.push('|---:|---|---|---|---|');
  for (const item of data.visual_batch) {
    lines.push(`| ${item.order} | ${mdCell(item.target_id)} | ${mdCell(item.route_id)} | ${mdCell(item.receiver_upload)} | ${mdCell(reasonText(item.reasons))} |`);
  }
  lines.push('');
  lines.push('## Interaction Batch');
  lines.push('');
  lines.push('| Order | Route ID | Expected Path | Interaction JSON Upload | Screenshot Upload | Reasons |');
  lines.push('|---:|---|---|---|---|---|');
  for (const item of data.interaction_batch) {
    lines.push(`| ${item.order} | ${mdCell(item.route_id)} | ${mdCell(item.expected_final_path)} | ${mdCell(item.receiver_upload)} | ${mdCell(item.interaction_screenshot_upload)} | ${mdCell(reasonText(item.reasons))} |`);
  }
  lines.push('');
  lines.push('## Acceptance Contract');
  lines.push('');
  lines.push('- Backend must be `codex-desktop-chrome-extension`; `iab` is forbidden for this evidence.');
  lines.push('- Before screenshots, open `/viewport-probe` through `mcp__codex_desktop_node_repl.desktop_chrome_open_url` and confirm it reports `1920x1080`.');
  lines.push('- Visual evidence requires `actual-1920.png`, `diff-1920.png`, `metrics.json`, and `capture-meta.json` for every visual target.');
  lines.push('- Interaction evidence requires `interaction.json` for every route and must prove no API/runtime failures plus a route-specific business action.');
  lines.push('- Protected routes must consume the smoke hash, land on the requested route, avoid `/login`, and leave no token material in the final URL.');
  lines.push('');
  return `${lines.join('\n')}\n`;
}

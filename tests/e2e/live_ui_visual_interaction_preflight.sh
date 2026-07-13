#!/usr/bin/env bash
set -euo pipefail

RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-ui-visual-interaction-preflight}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
VISUAL_ACCEPTANCE="${VISUAL_ACCEPTANCE:-doc/04_assets/ui_suite_gpt_v1/specs/visual-acceptance.json}"
ROUTE_PAGE_MAP="${ROUTE_PAGE_MAP:-doc/04_assets/ui_suite_gpt_v1/specs/route-page-map.json}"
EVIDENCE_DIR="${EVIDENCE_DIR:-$REGRESSION_DIR/ui-visual-interaction/latest}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
DESKTOP_CHROME_STATUS="${DESKTOP_CHROME_STATUS:-missing}"
DESKTOP_CHROME_DETAIL="${DESKTOP_CHROME_DETAIL:-not provided by Codex Desktop Chrome wrapper}"
DESKTOP_CHROME_ARTIFACT="${DESKTOP_CHROME_ARTIFACT:-}"
VISUAL_DIFF_MAX_PIXEL_RATIO="${VISUAL_DIFF_MAX_PIXEL_RATIO:-0.015}"
DESKTOP_SMOKE_TOKEN_REQUIRED="${DESKTOP_SMOKE_TOKEN_REQUIRED:-true}"
CHECK_LIVE_DESKTOP_SMOKE="${CHECK_LIVE_DESKTOP_SMOKE:-true}"
LIVE_CONFIG_URL="${LIVE_CONFIG_URL:-http://10.0.5.8:30180/config.js}"
APP_BASE_URL="${APP_BASE_URL:-http://10.0.5.8:30180}"
SMOKE_TOKEN_PARAM="${SMOKE_TOKEN_PARAM:-codex_smoke_token}"
SMOKE_TOKEN_PLACEHOLDER="${SMOKE_TOKEN_PLACEHOLDER:-<DESKTOP_SMOKE_TOKEN>}"
SMOKE_REDIRECT_BASE_URL="${SMOKE_REDIRECT_BASE_URL:-<SMOKE_REDIRECT_BASE_URL>}"
SMOKE_NONCE_PLACEHOLDER="${SMOKE_NONCE_PLACEHOLDER:-<CODEX_SMOKE_NONCE>}"
DESKTOP_CHROME_WAIT_MS="${DESKTOP_CHROME_WAIT_MS:-3500}"
CAPTURE_SESSION="${CAPTURE_SESSION:-$REGRESSION_DIR/ui-visual-interaction/capture-session-latest.json}"
VIEWPORT_PROBE="${VIEWPORT_PROBE:-$REGRESSION_DIR/ui-visual-interaction/desktop-chrome-viewport-probe-latest.json}"
RECEIVER_SELFTEST="${RECEIVER_SELFTEST:-$REGRESSION_DIR/ui-visual-interaction/receiver-selftest-latest.json}"
BRIDGE_RUNTIME_PREFLIGHT="${BRIDGE_RUNTIME_PREFLIGHT:-$REGRESSION_DIR/ui-visual-interaction/windows-codex-bridge-runtime-preflight-latest.json}"
TOOL_SURFACE_PREFLIGHT="${TOOL_SURFACE_PREFLIGHT:-$REGRESSION_DIR/ui-visual-interaction/codex-bridge-tool-surface-preflight-latest.json}"
ACTIVE_EXECS_PREFLIGHT="${ACTIVE_EXECS_PREFLIGHT:-$REGRESSION_DIR/ui-visual-interaction/windows-node-repl-active-execs-preflight-latest.json}"
NODE_REPL_CHROME_SMOKE="${NODE_REPL_CHROME_SMOKE:-$REGRESSION_DIR/ui-visual-interaction/windows-node-repl-chrome-bridge-smoke-latest.json}"
NODE_REPL_ENV_MATRIX_SMOKE="${NODE_REPL_ENV_MATRIX_SMOKE:-$REGRESSION_DIR/ui-visual-interaction/windows-node-repl-env-matrix-smoke-latest.json}"
SCHEDULED_CHROME_SMOKE="${SCHEDULED_CHROME_SMOKE:-$REGRESSION_DIR/ui-visual-interaction/windows-node-repl-scheduled-chrome-smoke-latest.json}"
SSH_PRIVILEGE_PREFLIGHT="${SSH_PRIVILEGE_PREFLIGHT:-$REGRESSION_DIR/ui-visual-interaction/windows-ssh-privilege-preflight-latest.json}"
PAYLOAD_SELFTEST="${PAYLOAD_SELFTEST:-$REGRESSION_DIR/ui-visual-interaction/desktop-chrome-bridge-payload-selftest-latest.json}"
BRIDGE_TOOL_CALL="${BRIDGE_TOOL_CALL:-$REGRESSION_DIR/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json}"
BRIDGE_RUN_RESULT="${BRIDGE_RUN_RESULT:-$REGRESSION_DIR/ui-visual-interaction/desktop-chrome-bridge-run-latest.json}"

REPORT="$LOG_DIR/ui-visual-interaction-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/ui-visual-interaction-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
MATRIX="$LOG_DIR/ui-visual-interaction-matrix.json"
COUNTS="$LOG_DIR/ui-visual-interaction-counts.json"
GAP_REPORT_JSON="$LOG_DIR/ui-visual-interaction-gap-report.json"
GAP_REPORT_MD="$LOG_DIR/ui-visual-interaction-gap-report.md"

mkdir -p "$LOG_DIR" "$REGRESSION_DIR" "$EVIDENCE_DIR"
: >"$REPORT"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

json_log() {
  local phase="$1" name="$2" severity="$3" passed="$4" status="$5" detail="${6:-}" artifact="${7:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg phase "$phase" \
    --arg name "$name" \
    --arg severity "$severity" \
    --argjson passed "$passed" \
    --arg status "$status" \
    --arg detail "$detail" \
    --arg artifact "$artifact" \
    '{ts:$ts, phase:$phase, name:$name, severity:$severity, passed:$passed, status:$status, detail:$detail, artifact:$artifact}' >>"$REPORT"
}

finalize() {
  local passed total blockers warnings result
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  total="$(jq -s 'length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed != true and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.passed != true and ((.severity == "warn") or (.severity == "warning")))] | length' "$REPORT")"
  result="pass"
  if [[ "$blockers" -gt 0 ]]; then
    result="blocked"
  elif [[ "$warnings" -gt 0 ]]; then
    result="warn"
  fi

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg generated_at "$(date -Iseconds)" \
    --arg visual_acceptance "$VISUAL_ACCEPTANCE" \
    --arg route_page_map "$ROUTE_PAGE_MAP" \
    --arg evidence_dir "$EVIDENCE_DIR" \
    --arg matrix "$MATRIX" \
    --arg gap_report "$GAP_REPORT_JSON" \
    --argjson passed "$passed" \
    --argjson total "$total" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --slurpfile counts "$COUNTS" \
    --slurpfile checks "$REPORT" \
    '($counts[0] // {}) + {
      run_id:$run_id,
      result:$result,
      generated_at:$generated_at,
      visual_acceptance:$visual_acceptance,
      route_page_map:$route_page_map,
      evidence_dir:$evidence_dir,
      matrix:$matrix,
      gap_report:$gap_report,
      acceptance_note:"UI contract is not treated as 1:1 visual acceptance. This gate requires real React pages, per-route screenshot diff evidence, and route business-interaction evidence.",
      required_visual_evidence:"For every visual target id: actual-1920.png, diff-1920.png, metrics.json, capture-meta.json under ui-visual-interaction/latest/<visual-target-id>/, with metrics status pass, pixel mismatch ratio <= threshold, and receiver capture metadata proving the uploaded Desktop Chrome screenshot was 1920x1080 before storage.",
      required_interaction_evidence:"For every route id: interaction.json, interaction.png, and interaction-capture-meta.json under ui-visual-interaction/latest/<route-id>/, with status pass, Desktop Chrome backend evidence, 1920x1080 screenshot metadata, no 4xx/5xx, no requestfailed, no pageerror, no console error, and a route-specific business action.",
      passed:$passed,
      total:$total,
      blockers:$blockers,
      warnings:$warnings,
      checks:$checks
    }' >"$SUMMARY"

  {
    echo "# UI Visual Interaction Dual Gate"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    jq -r '"- Target routes: " + (.target_route_count | tostring)' "$SUMMARY"
    jq -r '"- Visual targets: " + (.visual_target_count | tostring)' "$SUMMARY"
    jq -r '"- Target source images present: " + (.target_source_image_present_count | tostring) + "/" + (.visual_target_count | tostring)' "$SUMMARY"
    jq -r '"- React page components present: " + (.react_page_present_count | tostring) + "/" + (.target_route_count | tostring)' "$SUMMARY"
    jq -r '"- Visual diff evidence passed: " + (.visual_diff_passed_count | tostring) + "/" + (.visual_diff_required_count | tostring)' "$SUMMARY"
    jq -r '"- Business interaction evidence passed: " + (.interaction_passed_count | tostring) + "/" + (.interaction_required_count | tostring)' "$SUMMARY"
    jq -r '"- Full-page design-image reference blockers: " + (.full_page_design_image_reference_count | tostring)' "$SUMMARY"
    jq -r '"- Desktop smoke token config: repo=" + (.desktop_smoke_repo_config_ok | tostring) + " live=" + (.desktop_smoke_live_config_ok | tostring)' "$SUMMARY"
    jq -r '"- Desktop Chrome status: `" + .desktop_chrome_status + "`"' "$SUMMARY"
    jq -r '"- Capture session: `" + (.capture_session_status | tostring) + "` covers_current_gaps=" + (.capture_session_covers_current_gaps | tostring)' "$SUMMARY"
    echo "- Matrix: \`$MATRIX\`"
    echo
    echo "This gate is intentionally stricter than the UI contract gate. Passing the contract proves route/API/page structure; it does not prove that the real frontend visually matches the generated UI references 1:1."
    echo
    echo "## Blockers"
    jq -r '.checks[] | select(.passed != true and .severity == "blocker") | "- `" + .phase + "` " + .name + ": " + .detail' "$SUMMARY"
    echo
    echo "## Required Evidence Layout"
    echo
    echo "\`\`\`text"
    echo "doc/02_acceptance/02-regression/ui-visual-interaction/latest/"
    echo "  <visual-target-id>/"
    echo "    actual-1920.png"
    echo "    diff-1920.png"
    echo "    metrics.json"
    echo "    capture-meta.json"
	    echo "  <route-id>/"
	    echo "    interaction.json"
	    echo "    interaction.png"
	    echo "    interaction-capture-meta.json"
	    echo "  desktop-chrome-bridge-tool-call-latest.json"
	    echo "  desktop-chrome-bridge-run-latest.json"
	    echo "\`\`\`"
    echo
    echo "## Next Interaction Capture Queue"
    echo
    echo "| route | expected final path | safe redirect URL | template | missing or failing |"
    echo "|---|---|---|---|---|"
    jq -r '.routes[] | select(.interactionEvidence.passed != true) | "| `" + .id + "` | `" + .interactionEvidence.expectedFinalPath + "` | `" + .safeRedirectUrlPattern + "` | `" + .interactionTemplate + "` | " + (.interactionEvidence.reasons | join("; ")) + " |"' "$MATRIX"
    echo
    echo "Open each safe redirect URL with \`mcp__codex_desktop_node_repl.desktop_chrome_open_url\` after starting the nonce-only smoke redirect helper."
  } >"$LOCAL_REPORT"

  cp "$SUMMARY" "$REGRESSION_DIR/ui-visual-interaction-preflight-latest.json"
  cp "$LOCAL_REPORT" "$REGRESSION_DIR/ui-visual-interaction-preflight-latest.md"
  cp "$MATRIX" "$REGRESSION_DIR/ui-visual-interaction-matrix-latest.json"
  cp "$GAP_REPORT_JSON" "$REGRESSION_DIR/ui-visual-interaction-gap-report-latest.json"
  cp "$GAP_REPORT_MD" "$REGRESSION_DIR/ui-visual-interaction-gap-report-latest.md"

  echo "ui-visual-interaction-preflight result=$result summary=$SUMMARY"
  if [[ "$result" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
    exit 1
  fi
}

need_cmd git
need_cmd jq
need_cmd node
if [[ "$CHECK_LIVE_DESKTOP_SMOKE" == "true" ]]; then
  need_cmd curl
fi

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"
git diff --stat >"$LOG_DIR/git-diff-stat.txt" || true

node - "$VISUAL_ACCEPTANCE" "$ROUTE_PAGE_MAP" "$EVIDENCE_DIR" "$MATRIX" "$COUNTS" "$REPORT" "$RUN_ID" "$DESKTOP_CHROME_STATUS" "$DESKTOP_CHROME_DETAIL" "$DESKTOP_CHROME_ARTIFACT" "$VISUAL_DIFF_MAX_PIXEL_RATIO" "$DESKTOP_SMOKE_TOKEN_REQUIRED" "$CHECK_LIVE_DESKTOP_SMOKE" "$LIVE_CONFIG_URL" "$APP_BASE_URL" "$SMOKE_TOKEN_PARAM" "$SMOKE_TOKEN_PLACEHOLDER" "$SMOKE_REDIRECT_BASE_URL" "$SMOKE_NONCE_PLACEHOLDER" "$DESKTOP_CHROME_WAIT_MS" "$GAP_REPORT_JSON" "$GAP_REPORT_MD" "$CAPTURE_SESSION" "$VIEWPORT_PROBE" "$RECEIVER_SELFTEST" "$BRIDGE_RUNTIME_PREFLIGHT" "$TOOL_SURFACE_PREFLIGHT" "$ACTIVE_EXECS_PREFLIGHT" "$NODE_REPL_CHROME_SMOKE" "$NODE_REPL_ENV_MATRIX_SMOKE" "$SCHEDULED_CHROME_SMOKE" "$SSH_PRIVILEGE_PREFLIGHT" "$PAYLOAD_SELFTEST" "$BRIDGE_TOOL_CALL" "$BRIDGE_RUN_RESULT" <<'JS'
const fs = require('node:fs');
const path = require('node:path');

const [
  visualAcceptancePath,
  routePageMapPath,
  evidenceDir,
  matrixPath,
  countsPath,
  reportPath,
  runId,
  desktopChromeStatus,
  desktopChromeDetail,
  desktopChromeArtifact,
  maxPixelRatioRaw,
  desktopSmokeTokenRequiredRaw,
  checkLiveDesktopSmokeRaw,
  liveConfigUrl,
  appBaseUrlRaw,
  smokeTokenParam,
  smokeTokenPlaceholder,
  smokeRedirectBaseUrlRaw,
  smokeNoncePlaceholder,
  desktopChromeWaitMsRaw,
  gapReportJsonPath,
  gapReportMdPath,
  captureSessionPath,
  viewportProbePath,
  receiverSelftestPath,
  bridgeRuntimePreflightPath,
  toolSurfacePreflightPath,
  activeExecsPreflightPath,
  nodeReplChromeSmokePath,
  nodeReplEnvMatrixSmokePath,
  scheduledChromeSmokePath,
  sshPrivilegePreflightPath,
  payloadSelftestPath,
  bridgeToolCallPath,
  bridgeRunResultPath,
] = process.argv.slice(2);

const root = process.cwd();
const maxPixelRatio = Number(maxPixelRatioRaw);
const desktopSmokeTokenRequired = parseBool(desktopSmokeTokenRequiredRaw);
const checkLiveDesktopSmoke = parseBool(checkLiveDesktopSmokeRaw);
const appBaseUrl = String(appBaseUrlRaw || 'http://10.0.5.8:30180').replace(/\/+$/, '');
const smokeRedirectBaseUrl = String(smokeRedirectBaseUrlRaw || '').replace(/\/+$/, '');
const desktopChromeWaitMs = Number(desktopChromeWaitMsRaw || 3500);
const generatedAt = new Date().toISOString();
const checks = [];
const desktopSmokeManifestPaths = [
  'deployments/kubernetes/applications/web-ui.yaml',
  'web/ui/deployments/kubernetes/deployment.yaml',
];

function parseBool(value) {
  return ['1', 'true', 'yes', 'on'].includes(String(value || '').toLowerCase());
}

function resolveRepo(file) {
  return path.isAbsolute(file) ? file : path.join(root, file);
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(resolveRepo(file), 'utf8'));
}

function readText(file) {
  return fs.readFileSync(resolveRepo(file), 'utf8');
}

function exists(file) {
  return fs.existsSync(resolveRepo(file));
}

function jsonLog(phase, name, severity, passed, status, detail = '', artifact = '') {
  checks.push({ phase, name, severity, passed, status, detail, artifact });
  const row = {
    ts: new Date().toISOString(),
    phase,
    name,
    severity,
    passed,
    status,
    detail,
    artifact,
  };
  fs.appendFileSync(resolveRepo(reportPath), JSON.stringify(row) + '\n', 'utf8');
}

function isObject(value) {
  return value && typeof value === 'object' && !Array.isArray(value);
}

function pngSize(file) {
  const full = resolveRepo(file);
  const buf = fs.readFileSync(full);
  const signatureOk =
    buf.length >= 24 &&
    buf[0] === 0x89 &&
    buf.toString('ascii', 1, 4) === 'PNG' &&
    buf.toString('ascii', 12, 16) === 'IHDR';
  if (!signatureOk) {
    return { valid: false, width: 0, height: 0 };
  }
  return {
    valid: true,
    width: buf.readUInt32BE(16),
    height: buf.readUInt32BE(20),
  };
}

function walk(dir, files = []) {
  const full = resolveRepo(dir);
  if (!fs.existsSync(full)) return files;
  for (const entry of fs.readdirSync(full, { withFileTypes: true })) {
    const child = path.join(full, entry.name);
    if (entry.isDirectory()) {
      if (['node_modules', 'dist', 'dist-codex-preview', '.vite'].includes(entry.name)) continue;
      walk(child, files);
    } else {
      files.push(child);
    }
  }
  return files;
}

function rel(full) {
  return path.relative(root, full).split(path.sep).join('/');
}

function readEvidenceJson(file) {
  const full = resolveRepo(file);
  if (!fs.existsSync(full)) return { ok: false, reason: 'missing' };
  const text = fs.readFileSync(full, 'utf8');
  const draftMarker = /review-template|review_required|bootstrap|TBD|待补|占位/i.test(text);
  try {
    return { ok: true, data: JSON.parse(text), text, draftMarker };
  } catch (err) {
    return { ok: false, reason: `invalid-json: ${err.message}` };
  }
}

function readOptionalJson(file) {
  const full = resolveRepo(file);
  if (!fs.existsSync(full)) return { exists: false, valid: false, data: null, reason: 'missing' };
  try {
    return { exists: true, valid: true, data: JSON.parse(fs.readFileSync(full, 'utf8')), reason: '' };
  } catch (err) {
    return { exists: true, valid: false, data: null, reason: `invalid-json: ${err.message}` };
  }
}

function manifestEnablesDesktopSmokeToken(text) {
  return /DESKTOP_SMOKE_TOKEN_ENABLED[\s\S]{0,160}value:\s*["']?true["']?/i.test(text);
}

function liveConfigEnablesDesktopSmokeToken(text) {
  return /DESKTOP_SMOKE_TOKEN_ENABLED\s*:\s*["']?true["']?/i.test(text);
}

function readLiveConfig(url) {
  if (!checkLiveDesktopSmoke) {
    return { checked: false, ok: true, enabled: null, detail: 'live config check disabled' };
  }
  try {
    const { execFileSync } = require('node:child_process');
    const text = execFileSync('curl', ['--noproxy', '*', '-fsSL', url], { encoding: 'utf8', timeout: 10000 });
    return {
      checked: true,
      ok: true,
      enabled: liveConfigEnablesDesktopSmokeToken(text),
      detail: `fetched ${url}`,
    };
  } catch (error) {
    return {
      checked: true,
      ok: false,
      enabled: false,
      detail: error instanceof Error ? error.message : String(error),
    };
  }
}

function statusOk(value) {
  return ['pass', 'passed', 'ok'].includes(String(value || '').toLowerCase());
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

function normalizeViewportProbe(input, fallbackPath = '') {
  const data = isObject(input?.data) ? input.data : (isObject(input) ? input : {});
  const exists = input?.exists ?? Object.keys(data).length > 0;
  const valid = input?.valid ?? exists;
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
    path: data.path || fallbackPath || '',
    exists,
    valid,
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

function viewportProbeReady(probe, expectedWidth, expectedHeight) {
  return (
    probe.exists &&
    probe.valid &&
    statusOk(probe.result) &&
    Number(probe.viewport?.width) === expectedWidth &&
    Number(probe.viewport?.height) === expectedHeight
  );
}

function boolOk(data, keys) {
  return keys.every((key) => data[key] === true);
}

function resolveRoutePath(route) {
  if (route === '*') return '/__codex_visual_not_found__';
  return route
    .replace(':alertId', 'AL-20260620-000123')
    .replace(':campaignId', 'APT-20260619-001');
}

function pathWithQuery(routePath, query = '') {
  if (!query) return routePath;
  const separator = routePath.includes('?') ? '&' : '?';
  return `${routePath}${separator}${String(query).replace(/^\?/, '')}`;
}

function absoluteUrl(routePath, query = '') {
  return new URL(pathWithQuery(routePath, query), `${appBaseUrl}/`).toString();
}

function authenticatedUrlPattern(url, routeId) {
  if (!routeNeedsSmokeToken(routeId)) return url;
  const separator = url.includes('#') ? '&' : '#';
  return `${url}${separator}${smokeTokenParam}=${smokeTokenPlaceholder}`;
}

function smokeRedirectUrl(routePath, routeId, query = '') {
  if (!routeNeedsSmokeToken(routeId)) return absoluteUrl(routePath, query);
  const queryPart = query ? `&query=${encodeURIComponent(String(query).replace(/^\?/, ''))}` : '';
  return `${smokeRedirectBaseUrl}/start?nonce=${encodeURIComponent(smokeNoncePlaceholder)}&route=${encodeURIComponent(routePath)}${queryPart}`;
}

function pathFromUrl(value) {
  try {
    return new URL(value).pathname;
  } catch {
    return '';
  }
}

function assertionIsTrue(data, key) {
  return data[key] === true || data.assertions?.[key] === true;
}

function routeNeedsSmokeToken(routeId) {
  return routeId !== 'login';
}

function shellQuote(value) {
  const text = String(value);
  if (/^[A-Za-z0-9_./:=?&@%+-]+$/.test(text)) return text;
  return `'${text.replace(/'/g, "'\\''")}'`;
}

function markdownCell(value) {
  return String(value ?? '').replace(/\|/g, '\\|').replace(/\n/g, '<br>');
}

function reasonGroup(reason) {
  if (/viewport probe|desktop-chrome-viewport-probe|latest Desktop Chrome viewport probe/i.test(reason)) return 'viewport_probe_blocked';
  if (/missing actual/i.test(reason)) return 'missing_actual_screenshot';
  if (/missing diff/i.test(reason)) return 'missing_diff_image';
  if (/metrics (missing|invalid)|missing numeric pixel|metrics status/i.test(reason)) return 'metrics_missing_or_failing';
  if (/pixel mismatch ratio/i.test(reason)) return 'visual_mismatch_threshold';
  if (/viewport|uploaded screenshot|stored screenshot|Desktop Chrome viewport/i.test(reason)) return 'viewport_or_capture_size';
  if (/capture-meta/i.test(reason)) return 'capture_meta_missing_or_failing';
  if (/interaction (missing|invalid)|interaction status/i.test(reason)) return 'interaction_missing_or_failing';
  if (/network\/runtime|no_4xx_5xx|no_requestfailed|no_pageerror|no_console_error/i.test(reason)) return 'runtime_or_network_evidence';
  if (/final path|final_url|\/login|smoke token|smoke_hash|not_login_shell/i.test(reason)) return 'protected_route_auth_evidence';
  if (/business_action/i.test(reason)) return 'business_action_missing';
  if (/Desktop Chrome backend/i.test(reason)) return 'desktop_chrome_backend_missing';
  if (/draft|bootstrap|TBD|占位/i.test(reason)) return 'draft_marker';
  return 'other';
}

function countReasonGroups(items) {
  const counts = {};
  for (const item of items) {
    for (const reason of item.reasons || []) {
      const group = reasonGroup(reason);
      counts[group] = (counts[group] || 0) + 1;
    }
  }
  return counts;
}

function metricsCommand(target) {
  return [
    'tests/e2e/ui_visual_diff_metrics.py',
    '--target-id',
    target.id,
    '--route',
    pathWithQuery(target.resolvedPath, target.query || ''),
    '--source',
    target.sourceImage,
    '--actual',
    target.actual,
    '--diff',
    target.diff,
    '--metrics',
    target.metrics,
  ];
}

function renderGapReportMd(report) {
  const lines = [];
  lines.push('# UI Visual Interaction Gap Report');
  lines.push('');
  lines.push(`- Run ID: \`${report.run_id}\``);
  lines.push(`- Result: \`${report.result_hint}\``);
  lines.push(`- Visual gaps: \`${report.summary.visual_gap_count}/${report.summary.visual_required_count}\``);
  lines.push(`- Interaction gaps: \`${report.summary.interaction_gap_count}/${report.summary.interaction_required_count}\``);
  lines.push(`- Desktop Chrome status: \`${report.desktop_chrome.status}\``);
  lines.push('');
  lines.push('This report is an execution queue. It is not acceptance evidence and cannot close the gate by itself.');
  lines.push('');
  lines.push('## Desktop Bridge');
  lines.push('');
  lines.push(`- Detail: ${report.desktop_chrome.detail || 'not recorded'}`);
  lines.push(`- Next action: ${report.desktop_chrome.next_action}`);
  lines.push('');
  lines.push('## Capture Session');
  lines.push('');
  lines.push(`- Path: \`${report.capture_session.path}\``);
  lines.push(`- Status: \`${report.capture_session.status}\``);
  lines.push(`- Run ID: \`${report.capture_session.run_id || 'missing'}\``);
  lines.push(`- Covers current gaps: \`${report.capture_session.covers_current_gaps}\``);
  lines.push(`- Visual batch: \`${report.capture_session.visual_batch_count}/${report.summary.visual_gap_count}\``);
  lines.push(`- Interaction batch: \`${report.capture_session.interaction_batch_count}/${report.summary.interaction_gap_count}\``);
  lines.push('');
  lines.push('## Reason Groups');
  lines.push('');
  lines.push('| group | count |');
  lines.push('|---|---:|');
  for (const [group, count] of Object.entries(report.reason_groups).sort(([a], [b]) => a.localeCompare(b))) {
    lines.push(`| \`${group}\` | ${count} |`);
  }
  lines.push('');
  lines.push('## Formal Commands');
  lines.push('');
  lines.push('```bash');
  for (const command of report.formal_commands) lines.push(command);
  lines.push('```');
  lines.push('');
  lines.push('## Visual Gaps');
  lines.push('');
  lines.push('| target | route | safe URL | missing files | reasons | metrics command |');
  lines.push('|---|---|---|---|---|---|');
  for (const item of report.visual_gaps) {
    lines.push(`| \`${item.target_id}\` | \`${item.route_id}\` | \`${item.safe_redirect_url_pattern}\` | ${markdownCell(item.missing_files.join(', ') || 'none')} | ${markdownCell(item.reasons.join('; '))} | \`${markdownCell(item.metrics_command)}\` |`);
  }
  lines.push('');
  lines.push('## Interaction Gaps');
  lines.push('');
  lines.push('| route | safe URL | template | expected final path | reasons |');
  lines.push('|---|---|---|---|---|');
  for (const item of report.interaction_gaps) {
    lines.push(`| \`${item.route_id}\` | \`${item.safe_redirect_url_pattern}\` | \`${item.interaction_template}\` | \`${item.expected_final_path}\` | ${markdownCell(item.reasons.join('; '))} |`);
  }
  lines.push('');
  return `${lines.join('\n')}\n`;
}

function authMode(routeId) {
  if (routeId === 'login') return 'public-login';
  if (routeId === 'screen') return 'authenticated-or-screen-masked-demo';
  return 'authenticated-or-controlled-smoke-token';
}

function routeBusinessHint(route) {
  if (route.id === 'login') return 'render login form and captcha challenge without submitting credentials';
  if (route.id === 'not-found') return 'render not-found recovery action and return navigation affordance';
  const endpointText = Array.isArray(route.apiEndpoints) && route.apiEndpoints.length > 0
    ? `verify live data from ${route.apiEndpoints.join(', ')}`
    : 'verify visible page-specific content';
  return `${route.title}: ${endpointText}; perform one route-specific read or safe UI action`;
}

function routeInteractionRequirements(route, expectedFinalPath) {
  if (route.id === 'login') {
    return {
      expected_final_path: expectedFinalPath,
      required_assertions: ['product_name_visible', 'account_password_tab_visible', 'captcha_visible', 'submit_visible', 'smoke_hash_absent'],
      forbidden_final_paths: [],
    };
  }
  return {
    expected_final_path: expectedFinalPath,
    required_assertions: ['smoke_hash_consumed', 'not_login_shell', 'access_denied_absent'],
    forbidden_final_paths: ['/login'],
    expected_text_markers: [route.title],
    api_endpoints: route.apiEndpoints || [],
  };
}

function evidenceFile(routeId, name) {
  return path.join(evidenceDir, routeId, name).split(path.sep).join('/');
}

function interactionTemplateFile(routeId) {
  return path.join(path.dirname(evidenceDir), 'templates', routeId, 'interaction.template.json').split(path.sep).join('/');
}

function writeInteractionTemplate(route, expectedFinalPath, routeUrl, routeAuthenticatedUrl, routeSafeRedirectUrl) {
  const file = interactionTemplateFile(route.id);
  const full = resolveRepo(file);
  fs.mkdirSync(path.dirname(full), { recursive: true });
  const isProtected = routeNeedsSmokeToken(route.id);
  const template = {
    template_type: 'ui_visual_interaction_route_evidence',
    template_version: 1,
    route_id: route.id,
    title: route.title,
    route: route.route,
    expected_final_path: expectedFinalPath,
    url: routeUrl,
    authenticated_url_pattern: routeAuthenticatedUrl,
    safe_redirect_url_pattern: routeSafeRedirectUrl,
    safe_wrapper_call: {
      tool: 'mcp__codex_desktop_node_repl.desktop_chrome_open_url',
      args: { url: routeSafeRedirectUrl, keep: true, wait_ms: desktopChromeWaitMs },
    },
    output_path: evidenceFile(route.id, 'interaction.json'),
    required_screenshot_path: evidenceFile(route.id, 'interaction.png'),
    required_screenshot_meta_path: evidenceFile(route.id, 'interaction-capture-meta.json'),
    business_action_hint: routeBusinessHint(route),
    api_endpoints: route.apiEndpoints || [],
    acceptance_requirements: routeInteractionRequirements(route, expectedFinalPath),
    interaction_json_skeleton: {
      status: 'pass',
      route_id: route.id,
      route: route.route,
      final_url: routeUrl,
      business_action: routeBusinessHint(route),
      desktop_backend: 'codex-desktop-chrome-extension',
      desktop_chrome_backend_status: 'pass',
      no_4xx_5xx: true,
      no_requestfailed: true,
      no_pageerror: true,
      no_console_error: true,
      target_screenshot: evidenceFile(route.id, 'interaction.png'),
      assertions: isProtected
        ? {
            smoke_hash_consumed: true,
            not_login_shell: true,
            access_denied_absent: true,
          }
        : {
            product_name_visible: true,
            account_password_tab_visible: true,
            captcha_visible: true,
            submit_visible: true,
            smoke_hash_absent: true,
          },
    },
    note: 'Template only. Do not copy this file into latest/<route-id>/interaction.json without replacing placeholders with real Desktop Chrome evidence.',
  };
  fs.writeFileSync(full, JSON.stringify(template, null, 2) + '\n', 'utf8');
  return file;
}

const visual = readJson(visualAcceptancePath);
const routeMap = readJson(routePageMapPath);
const routes = Array.isArray(visual.routes) ? visual.routes : [];
const mapById = new Map(routeMap.map((item) => [item.id, item]));
const expectedWidth = visual.global?.imageSize?.width ?? 1920;
const expectedHeight = visual.global?.imageSize?.height ?? 1080;

function stableJson(value) {
  return JSON.stringify(value ?? null);
}

function visualStatesOf(route) {
  if (Array.isArray(route.visualStates) && route.visualStates.length > 0) {
    return route.visualStates.map((state) => ({
      id: state.id || `${route.id}-${state.title || 'state'}`,
      title: state.title || route.title,
      query: state.query || '',
      routeId: route.id,
      route: route.route,
      pageComponent: route.pageComponent,
      sourceImage: state.sourceImage || '',
    }));
  }
  if (route.sourceImage) {
    return [{
      id: route.id,
      title: route.title,
      query: '',
      routeId: route.id,
      route: route.route,
      pageComponent: route.pageComponent,
      sourceImage: route.sourceImage,
    }];
  }
  return [];
}

jsonLog(
  'config',
  'visual acceptance route list is present',
  'blocker',
  routes.length > 0,
  routes.length > 0 ? 'ok' : 'missing',
  `routes=${routes.length}`,
  visualAcceptancePath,
);

const routeMapAligned = routes.every((route) => {
  const mapped = mapById.get(route.id);
  return (
    mapped &&
    mapped.route === route.route &&
    mapped.pageComponent === route.pageComponent &&
    String(mapped.sourceImage || '') === String(route.sourceImage || '') &&
    stableJson(mapped.visualStates) === stableJson(route.visualStates)
  );
});
jsonLog(
  'config',
  'visual acceptance routes align with route-page-map',
  'blocker',
  routeMapAligned && routeMap.length === routes.length,
  routeMapAligned ? 'ok' : 'mismatch',
  `visual_routes=${routes.length} route_page_map=${routeMap.length}`,
  routePageMapPath,
);

const desktopSmokeManifestResults = desktopSmokeManifestPaths.map((file) => {
  if (!exists(file)) return { file, exists: false, enabled: false };
  return {
    file,
    exists: true,
    enabled: manifestEnablesDesktopSmokeToken(readText(file)),
  };
});
const desktopSmokeRepoOk = desktopSmokeManifestResults.every((item) => item.exists && item.enabled);
jsonLog(
  'config',
  'Desktop smoke token is enabled in repo Web UI manifests',
  desktopSmokeTokenRequired ? 'blocker' : 'warn',
  !desktopSmokeTokenRequired || desktopSmokeRepoOk,
  desktopSmokeRepoOk ? 'ok' : 'missing-or-disabled',
  JSON.stringify(desktopSmokeManifestResults),
  desktopSmokeManifestPaths.join(', '),
);

const liveDesktopSmoke = readLiveConfig(liveConfigUrl);
const desktopSmokeLiveOk = !checkLiveDesktopSmoke || (liveDesktopSmoke.ok && liveDesktopSmoke.enabled === true);
jsonLog(
  'config',
  'Desktop smoke token is enabled in live runtime config',
  desktopSmokeTokenRequired ? 'blocker' : 'warn',
  !desktopSmokeTokenRequired || desktopSmokeLiveOk,
  desktopSmokeLiveOk ? 'ok' : 'missing-or-disabled',
  liveDesktopSmoke.detail,
  liveConfigUrl,
);

const srcFiles = walk('web/ui/src').filter((file) => /\.(tsx?|jsx?|css|scss)$/.test(file));
const implementationAssetFiles = [
  ...srcFiles,
  ...walk('web/ui/public/ui-assets').filter((file) => /\.(png|jpe?g|webp|svg)$/i.test(file)),
  ...walk('web/ui/scripts').filter((file) => /\.(mjs|cjs|js|ts|py|sh)$/i.test(file)),
];
const forbiddenRefs = [];
for (const file of implementationAssetFiles) {
  const isBinaryAsset = /\.(png|jpe?g|webp)$/i.test(file);
  const text = isBinaryAsset ? '' : fs.readFileSync(file, 'utf8');
  const matches = [
    ...text.matchAll(/doc\/04_assets\/ui_suite_gpt_v1\/screens\/pages\/[^'")\s]+/g),
    ...text.matchAll(/screens\/pages\/[^'")\s]+(?:raw-imagegen|\.png)/g),
    ...text.matchAll(/raw-imagegen\.png/g),
    ...text.matchAll(/login-underlay\.(?:png|jpe?g|webp|svg)/g),
  ];
  if (isBinaryAsset && /\/(?:derived\/)?[^/]*underlay[^/]*\.(?:png|jpe?g|webp)$/i.test(file)) {
    matches.push([rel(file)]);
  }
  if (matches.length > 0) {
    forbiddenRefs.push({
      file: rel(file),
      matches: [...new Set(matches.map((match) => match[0]))],
    });
  }
}
jsonLog(
  'implementation',
  'frontend does not embed design page PNGs or target-derived underlays as implementation',
  'blocker',
  forbiddenRefs.length === 0,
  forbiddenRefs.length === 0 ? 'ok' : 'forbidden-reference',
  forbiddenRefs.length === 0 ? 'no source design PNG or target-derived underlay references under frontend implementation assets' : JSON.stringify(forbiddenRefs.slice(0, 10)),
  'web/ui',
);

const routeDetails = [];
let visualTargetCount = 0;
let sourcePresent = 0;
let sourceDimensionOk = 0;
let pagePresent = 0;
let visualDiffPassed = 0;
let interactionPassed = 0;

for (const route of routes) {
  const componentPath = `web/ui/src/pages/${route.pageComponent}.tsx`;
  const componentExists = exists(componentPath);
  if (componentExists) pagePresent += 1;

  const visualTargets = visualStatesOf(route);
  visualTargetCount += visualTargets.length;
  const visualTargetDetails = [];
  for (const target of visualTargets) {
    const targetResolvedPath = resolveRoutePath(target.route);
    const targetUrl = absoluteUrl(targetResolvedPath, target.query);
    const targetSafeRedirectUrl = smokeRedirectUrl(targetResolvedPath, target.routeId, target.query);
    const targetAuthenticatedUrl = authenticatedUrlPattern(targetUrl, target.routeId);
    const sourceImage = target.sourceImage || '';
    const sourceExists = sourceImage ? exists(sourceImage) : false;
    let sourceSize = { valid: false, width: 0, height: 0 };
    if (sourceExists) {
      sourceSize = pngSize(sourceImage);
    }
    const sourceOk = sourceExists && sourceSize.valid && sourceSize.width === expectedWidth && sourceSize.height === expectedHeight;
    if (sourceExists) sourcePresent += 1;
    if (sourceOk) sourceDimensionOk += 1;

    const actual = evidenceFile(target.id, 'actual-1920.png');
    const diff = evidenceFile(target.id, 'diff-1920.png');
    const metricsPath = evidenceFile(target.id, 'metrics.json');
    const captureMetaPath = evidenceFile(target.id, 'capture-meta.json');
    const actualExists = exists(actual);
    const diffExists = exists(diff);
    const metrics = readEvidenceJson(metricsPath);
    const captureMeta = readEvidenceJson(captureMetaPath);

    let metricsRatio = null;
    let metricsStatus = '';
    let captureMetaStatus = '';
    let captureUploadedSize = null;
    let captureStoredSize = null;
    let captureDesktopViewportSize = null;
    let visualReasons = [];
    if (!sourceOk) visualReasons.push(`source image missing or not ${expectedWidth}x${expectedHeight}`);
    if (!componentExists) visualReasons.push('missing React page component');
    if (!actualExists) visualReasons.push('missing actual-1920.png');
    if (!diffExists) visualReasons.push('missing diff-1920.png');
    if (!metrics.ok) {
      visualReasons.push(`metrics ${metrics.reason || 'missing'}`);
    } else {
      const data = metrics.data;
      metricsStatus = String(data.status || data.result || '');
      metricsRatio = Number(
        data.visual_diff?.pixel_mismatch_ratio ??
          data.visualDiff?.pixelMismatchRatio ??
          data.pixel_mismatch_ratio ??
          data.mismatch_ratio,
      );
      if (!statusOk(metricsStatus)) visualReasons.push(`metrics status=${metricsStatus || 'missing'}`);
      if (!Number.isFinite(metricsRatio)) visualReasons.push('missing numeric pixel mismatch ratio');
      if (Number.isFinite(metricsRatio) && metricsRatio > maxPixelRatio) {
        visualReasons.push(`pixel mismatch ratio ${metricsRatio} > ${maxPixelRatio}`);
      }
      const viewport = data.viewport || {};
      const width = Number(viewport.width ?? data.width);
      const height = Number(viewport.height ?? data.height);
      if (width !== expectedWidth || height !== expectedHeight) {
        visualReasons.push(`viewport ${width || 'missing'}x${height || 'missing'} != ${expectedWidth}x${expectedHeight}`);
      }
      if (metrics.draftMarker) visualReasons.push('metrics contains draft/bootstrap markers');
    }
    if (!captureMeta.ok) {
      visualReasons.push(`capture-meta ${captureMeta.reason || 'missing'}`);
    } else {
      const data = captureMeta.data;
      captureMetaStatus = String(data.status || data.result || '');
      captureUploadedSize = data.uploaded_size || data.uploadedSize || null;
      captureStoredSize = data.stored_size || data.storedSize || null;
      captureDesktopViewportSize = data.desktop_viewport || data.desktopViewport || null;
      if (!statusOk(captureMetaStatus)) visualReasons.push(`capture-meta status=${captureMetaStatus || 'missing'}`);
      if (data.backend !== 'codex-desktop-chrome-extension') {
        visualReasons.push(`capture-meta backend=${data.backend || 'missing'}`);
      }
      if (data.post_capture_resize !== false) {
        visualReasons.push('capture-meta post_capture_resize is not false');
      }
      const uploadedWidth = Number(captureUploadedSize?.width);
      const uploadedHeight = Number(captureUploadedSize?.height);
      const storedWidth = Number(captureStoredSize?.width);
      const storedHeight = Number(captureStoredSize?.height);
      const desktopViewportWidth = Number(captureDesktopViewportSize?.width);
      const desktopViewportHeight = Number(captureDesktopViewportSize?.height);
      if (uploadedWidth !== expectedWidth || uploadedHeight !== expectedHeight) {
        visualReasons.push(`uploaded screenshot ${uploadedWidth || 'missing'}x${uploadedHeight || 'missing'} != ${expectedWidth}x${expectedHeight}`);
      }
      if (storedWidth !== expectedWidth || storedHeight !== expectedHeight) {
        visualReasons.push(`stored screenshot ${storedWidth || 'missing'}x${storedHeight || 'missing'} != ${expectedWidth}x${expectedHeight}`);
      }
      if (desktopViewportWidth !== expectedWidth || desktopViewportHeight !== expectedHeight) {
        visualReasons.push(`Desktop Chrome viewport ${desktopViewportWidth || 'missing'}x${desktopViewportHeight || 'missing'} != ${expectedWidth}x${expectedHeight}`);
      }
      if (captureMeta.draftMarker) visualReasons.push('capture-meta contains draft/bootstrap markers');
    }
    const visualPassed = sourceOk && componentExists && actualExists && diffExists && metrics.ok && captureMeta.ok && visualReasons.length === 0;
    if (visualPassed) visualDiffPassed += 1;

    visualTargetDetails.push({
      id: target.id,
      routeId: target.routeId,
      title: target.title,
      query: target.query,
      route: target.route,
      resolvedPath: targetResolvedPath,
      url: targetUrl,
      authMode: authMode(target.routeId),
      requiresSmokeToken: routeNeedsSmokeToken(target.routeId),
      authenticatedUrlPattern: targetAuthenticatedUrl,
      safeRedirectUrlPattern: targetSafeRedirectUrl,
      safeWrapperCall: {
        tool: 'mcp__codex_desktop_node_repl.desktop_chrome_open_url',
        args: { url: targetSafeRedirectUrl, keep: true, wait_ms: desktopChromeWaitMs },
      },
      sourceImage,
      sourceExists,
      sourceSize,
      sourceOk,
      actual,
      diff,
      metrics: metricsPath,
      captureMeta: captureMetaPath,
      actualExists,
      diffExists,
      metricsExists: metrics.ok,
      captureMetaExists: captureMeta.ok,
      metricsStatus,
      captureMetaStatus,
      captureUploadedSize,
      captureStoredSize,
      captureDesktopViewportSize,
      pixelMismatchRatio: Number.isFinite(metricsRatio) ? metricsRatio : null,
      passed: visualPassed,
      reasons: visualReasons,
      metricsCommand: metricsCommand({
        id: target.id,
        resolvedPath: targetResolvedPath,
        query: target.query,
        sourceImage,
        actual,
        diff,
        metrics: metricsPath,
      }),
    });
  }

  const interactionPath = evidenceFile(route.id, 'interaction.json');
  const interaction = readEvidenceJson(interactionPath);
  const expectedFinalPath = resolveRoutePath(route.route);
  const routeUrl = absoluteUrl(expectedFinalPath);
  const routeAuthenticatedUrl = authenticatedUrlPattern(routeUrl, route.id);
  const routeSafeRedirectUrl = smokeRedirectUrl(expectedFinalPath, route.id);
  const interactionTemplatePath = writeInteractionTemplate(route, expectedFinalPath, routeUrl, routeAuthenticatedUrl, routeSafeRedirectUrl);

  let interactionReasons = [];
  let interactionStatus = '';
  let interactionFinalUrl = '';
  let interactionFinalPath = '';
  let interactionScreenshot = evidenceFile(route.id, 'interaction.png');
  let interactionScreenshotSize = null;
  let interactionCaptureMetaPath = evidenceFile(route.id, 'interaction-capture-meta.json');
  let interactionCaptureMetaStatus = '';
  let interactionCaptureUploadedSize = null;
  let interactionCaptureStoredSize = null;
  let interactionCaptureDesktopViewportSize = null;
  if (!interaction.ok) {
    interactionReasons.push(`interaction ${interaction.reason || 'missing'}`);
  } else {
    const data = interaction.data;
    interactionStatus = String(data.status || data.result || '');
    if (!statusOk(interactionStatus)) interactionReasons.push(`interaction status=${interactionStatus || 'missing'}`);
    if (!boolOk(data, ['no_4xx_5xx', 'no_requestfailed', 'no_pageerror', 'no_console_error'])) {
      interactionReasons.push('network/runtime booleans are not all true');
    }
    if (data.route_id && data.route_id !== route.id) {
      interactionReasons.push(`route_id ${data.route_id} != ${route.id}`);
    }
    if (data.route && data.route !== route.route) {
      interactionReasons.push(`route ${data.route} != ${route.route}`);
    }
    if (!data.business_action || String(data.business_action).trim().length < 3) {
      interactionReasons.push('missing route-specific business_action');
    }
    if (!statusOk(data.desktop_chrome_backend_status || data.desktop_chrome_status || 'missing')) {
      interactionReasons.push('missing passing Desktop Chrome backend status');
    }
    const desktopBackend = data.desktop_backend || data.desktopBackend || '';
    if (desktopBackend !== 'codex-desktop-chrome-extension') {
      interactionReasons.push(`desktop_backend ${desktopBackend || 'missing'} != codex-desktop-chrome-extension`);
    }
    const screenshot = data.target_screenshot || data.screenshot || data.actual_screenshot;
    if (screenshot) {
      interactionScreenshot = screenshot;
      interactionCaptureMetaPath = path.join(path.dirname(screenshot), 'interaction-capture-meta.json').split(path.sep).join('/');
    }
    if (!screenshot || !exists(screenshot)) {
      interactionReasons.push('missing interaction target screenshot');
    } else {
      const size = pngSize(screenshot);
      interactionScreenshotSize = size.valid ? { width: size.width, height: size.height } : null;
      if (!size.valid || size.width !== expectedWidth || size.height !== expectedHeight) {
        interactionReasons.push(`interaction screenshot ${size.width || 'missing'}x${size.height || 'missing'} != ${expectedWidth}x${expectedHeight}`);
      }
    }
    const interactionCaptureMeta = readEvidenceJson(interactionCaptureMetaPath);
    if (!interactionCaptureMeta.ok) {
      interactionReasons.push(`interaction-capture-meta ${interactionCaptureMeta.reason || 'missing'}`);
    } else {
      const meta = interactionCaptureMeta.data;
      interactionCaptureMetaStatus = String(meta.status || meta.result || '');
      interactionCaptureUploadedSize = meta.uploaded_size || meta.uploadedSize || null;
      interactionCaptureStoredSize = meta.stored_size || meta.storedSize || null;
      interactionCaptureDesktopViewportSize = meta.desktop_viewport || meta.desktopViewport || null;
      if (!statusOk(interactionCaptureMetaStatus)) {
        interactionReasons.push(`interaction-capture-meta status=${interactionCaptureMetaStatus || 'missing'}`);
      }
      if (meta.backend !== 'codex-desktop-chrome-extension') {
        interactionReasons.push(`interaction-capture-meta backend=${meta.backend || 'missing'}`);
      }
      if (meta.post_capture_resize !== false) {
        interactionReasons.push('interaction-capture-meta post_capture_resize is not false');
      }
      const uploadedWidth = Number(interactionCaptureUploadedSize?.width);
      const uploadedHeight = Number(interactionCaptureUploadedSize?.height);
      const storedWidth = Number(interactionCaptureStoredSize?.width);
      const storedHeight = Number(interactionCaptureStoredSize?.height);
      const desktopViewportWidth = Number(interactionCaptureDesktopViewportSize?.width);
      const desktopViewportHeight = Number(interactionCaptureDesktopViewportSize?.height);
      if (uploadedWidth !== expectedWidth || uploadedHeight !== expectedHeight) {
        interactionReasons.push(`interaction uploaded screenshot ${uploadedWidth || 'missing'}x${uploadedHeight || 'missing'} != ${expectedWidth}x${expectedHeight}`);
      }
      if (storedWidth !== expectedWidth || storedHeight !== expectedHeight) {
        interactionReasons.push(`interaction stored screenshot ${storedWidth || 'missing'}x${storedHeight || 'missing'} != ${expectedWidth}x${expectedHeight}`);
      }
      if (desktopViewportWidth !== expectedWidth || desktopViewportHeight !== expectedHeight) {
        interactionReasons.push(`interaction Desktop Chrome viewport ${desktopViewportWidth || 'missing'}x${desktopViewportHeight || 'missing'} != ${expectedWidth}x${expectedHeight}`);
      }
      if (interactionCaptureMeta.draftMarker) interactionReasons.push('interaction-capture-meta contains draft/bootstrap markers');
    }
    interactionFinalUrl = String(data.final_url || data.url || '');
    interactionFinalPath = pathFromUrl(interactionFinalUrl);
    if (!interactionFinalUrl) {
      interactionReasons.push('missing final_url');
    }
    if (interactionFinalPath && interactionFinalPath !== expectedFinalPath) {
      interactionReasons.push(`final path ${interactionFinalPath} != ${expectedFinalPath}`);
    }
    if (routeNeedsSmokeToken(route.id)) {
      if (interactionFinalPath === '/login') interactionReasons.push('protected route resolved to /login');
      if (interactionFinalUrl.includes('codex_smoke_token') || interactionFinalUrl.includes('<DESKTOP_SMOKE_TOKEN>')) {
        interactionReasons.push('smoke token remains in final_url');
      }
      if (!assertionIsTrue(data, 'smoke_hash_consumed')) interactionReasons.push('missing smoke_hash_consumed assertion');
      if (!assertionIsTrue(data, 'not_login_shell')) interactionReasons.push('missing not_login_shell assertion');
    } else if (route.id === 'login' && data.assertions && data.assertions.smoke_hash_absent !== true) {
      interactionReasons.push('login evidence does not assert smoke_hash_absent');
    }
    if (interaction.draftMarker) interactionReasons.push('interaction contains draft/bootstrap markers');
  }
  const interactionOk = componentExists && interaction.ok && interactionReasons.length === 0;
  if (interactionOk) interactionPassed += 1;

  routeDetails.push({
    id: route.id,
    title: route.title,
    route: route.route,
    resolvedPath: expectedFinalPath,
    url: routeUrl,
    pageComponent: route.pageComponent,
    componentPath,
    componentExists,
    authMode: authMode(route.id),
    requiresSmokeToken: routeNeedsSmokeToken(route.id),
    authenticatedUrlPattern: routeAuthenticatedUrl,
    safeRedirectUrlPattern: routeSafeRedirectUrl,
    safeWrapperCall: {
      tool: 'mcp__codex_desktop_node_repl.desktop_chrome_open_url',
      args: { url: routeSafeRedirectUrl, keep: true, wait_ms: desktopChromeWaitMs },
    },
    interactionTemplate: interactionTemplatePath,
    apiEndpoints: route.apiEndpoints || [],
    businessActionHint: routeBusinessHint(route),
    interactionRequirements: routeInteractionRequirements(route, expectedFinalPath),
    visualTargets: visualTargetDetails,
    interactionEvidence: {
      required: true,
      interaction: interactionPath,
      screenshot: interactionScreenshot,
      screenshotSize: interactionScreenshotSize,
      captureMeta: interactionCaptureMetaPath,
      captureMetaStatus: interactionCaptureMetaStatus,
      captureUploadedSize: interactionCaptureUploadedSize,
      captureStoredSize: interactionCaptureStoredSize,
      captureDesktopViewportSize: interactionCaptureDesktopViewportSize,
      exists: interaction.ok,
      status: interactionStatus,
      finalUrl: interactionFinalUrl,
      finalPath: interactionFinalPath,
      expectedFinalPath,
      passed: interactionOk,
      reasons: interactionReasons,
    },
  });
}

const allVisualTargets = routeDetails.flatMap((route) => route.visualTargets);
const missingSources = allVisualTargets.filter((target) => !target.sourceOk);
const missingComponents = routeDetails.filter((route) => !route.componentExists);
const missingVisualEvidence = allVisualTargets.filter((target) => !target.passed);
const missingInteractionEvidence = routeDetails.filter((route) => !route.interactionEvidence.passed);
const captureSession = readOptionalJson(captureSessionPath);
const captureSessionSummary = captureSession.data?.summary || {};
const captureSessionVisualBatch = Array.isArray(captureSession.data?.visual_batch) ? captureSession.data.visual_batch : [];
const captureSessionInteractionBatch = Array.isArray(captureSession.data?.interaction_batch) ? captureSession.data.interaction_batch : [];
const captureSessionVisualIds = new Set(captureSessionVisualBatch.map((item) => item.target_id).filter(Boolean));
const captureSessionInteractionIds = new Set(captureSessionInteractionBatch.map((item) => item.route_id).filter(Boolean));
const pendingVisualIds = missingVisualEvidence.map((target) => target.id);
const pendingInteractionIds = missingInteractionEvidence.map((route) => route.id);
const captureSessionMissingVisualIds = pendingVisualIds.filter((id) => !captureSessionVisualIds.has(id));
const captureSessionMissingInteractionIds = pendingInteractionIds.filter((id) => !captureSessionInteractionIds.has(id));
const captureSessionCountsMatch =
  Number(captureSessionSummary.visual_pending_count) === pendingVisualIds.length &&
  Number(captureSessionSummary.interaction_pending_count) === pendingInteractionIds.length &&
  Number(captureSessionSummary.visual_batch_count) === pendingVisualIds.length &&
  Number(captureSessionSummary.interaction_batch_count) === pendingInteractionIds.length;
const hasPendingCaptureWork = pendingVisualIds.length > 0 || pendingInteractionIds.length > 0;
const captureSessionCoversCurrentGaps =
  captureSession.exists &&
  captureSession.valid &&
  captureSessionCountsMatch &&
  captureSessionMissingVisualIds.length === 0 &&
  captureSessionMissingInteractionIds.length === 0;
const captureSessionStatus = captureSession.data?.status || (captureSession.exists ? (captureSession.valid ? 'present' : 'invalid') : 'missing');
const captureSessionViewportCalibration = isObject(captureSession.data?.viewport_calibration)
  ? captureSession.data.viewport_calibration
  : {};
const captureSessionViewportCommand = String(captureSessionViewportCalibration.command || captureSession.data?.commands?.viewport_probe_open || '');
const captureSessionViewportRequired = captureSessionViewportCalibration.required === true;
const captureSessionViewportUrl = String(captureSessionViewportCalibration.url || '');
const captureSessionHasViewportCalibration =
  captureSession.exists &&
  captureSession.valid &&
  captureSessionViewportRequired &&
  captureSessionViewportCommand.includes('desktop_chrome_open_url') &&
  captureSessionViewportUrl.includes('/viewport-probe');
const embeddedViewportProbe = isObject(captureSessionViewportCalibration.latest_probe)
  ? captureSessionViewportCalibration.latest_probe
  : {};
const directViewportProbeState = readOptionalJson(viewportProbePath);
const directViewportProbe = normalizeViewportProbe(directViewportProbeState, viewportProbePath);
const embeddedViewportProbeNormalized = normalizeViewportProbe(
  embeddedViewportProbe,
  embeddedViewportProbe.path || viewportProbePath,
);
const captureSessionViewportProbe = directViewportProbe.exists || directViewportProbe.valid
  ? directViewportProbe
  : embeddedViewportProbeNormalized;
const captureSessionViewportProbeReady = viewportProbeReady(captureSessionViewportProbe, expectedWidth, expectedHeight);
const receiverSelftest = readOptionalJson(receiverSelftestPath);
const receiverSelftestPassed =
  receiverSelftest.exists &&
  receiverSelftest.valid &&
  statusOk(receiverSelftest.data?.result || receiverSelftest.data?.status) &&
  Number(receiverSelftest.data?.passed) === Number(receiverSelftest.data?.total) &&
  Number(receiverSelftest.data?.total) > 0;
const bridgeRuntimePreflight = readOptionalJson(bridgeRuntimePreflightPath);
const bridgeRuntimeData = bridgeRuntimePreflight.data || {};
const bridgeRuntime = bridgeRuntimeData.runtime || {};
const bridgeRuntimeProcessCounts = bridgeRuntime.process_counts || {};
const bridgeRuntimeNodeReplMcpConfig = bridgeRuntime.node_repl_mcp_config || {};
const bridgeRuntimeMcpListLines = Array.isArray(bridgeRuntime.mcp_list_lines) ? bridgeRuntime.mcp_list_lines : [];
const bridgeRuntimeExplicitCodexVersionLines = Array.isArray(bridgeRuntime.explicit_codex_version_lines) ? bridgeRuntime.explicit_codex_version_lines : [];
const bridgeRuntimePassed =
  bridgeRuntimePreflight.exists &&
  bridgeRuntimePreflight.valid &&
  statusOk(bridgeRuntimeData.result || bridgeRuntimeData.status) &&
  bridgeRuntime.explicit_codex_exists === true &&
  statusOk(bridgeRuntimeNodeReplMcpConfig.result) &&
  bridgeRuntimeNodeReplMcpConfig.transport_type === 'stdio' &&
  Array.isArray(bridgeRuntime.chrome_clients) &&
  bridgeRuntime.chrome_clients.length > 0 &&
  Number(bridgeRuntimeProcessCounts.chrome) > 0 &&
  Number(bridgeRuntimeProcessCounts.codex) > 0 &&
  Number(bridgeRuntimeProcessCounts.code) > 0;
const toolSurfacePreflight = readOptionalJson(toolSurfacePreflightPath);
const toolSurfaceData = toolSurfacePreflight.data || {};
const toolSurfacePassed =
  toolSurfacePreflight.exists &&
  toolSurfacePreflight.valid &&
  statusOk(toolSurfaceData.result || toolSurfaceData.status);
const activeExecsPreflight = readOptionalJson(activeExecsPreflightPath);
const activeExecsData = activeExecsPreflight.data || {};
const activeExecsRuntime = activeExecsData.runtime || {};
const activeExecsPassed =
  activeExecsPreflight.exists &&
  activeExecsPreflight.valid &&
  statusOk(activeExecsData.result || activeExecsData.status);
const nodeReplChromeSmoke = readOptionalJson(nodeReplChromeSmokePath);
const nodeReplChromeSmokeData = nodeReplChromeSmoke.data || {};
const nodeReplChromeNoEnv = nodeReplChromeSmokeData.chrome_extension_no_env_smoke || {};
const nodeReplChromeFullEnv = nodeReplChromeSmokeData.chrome_extension_smoke || {};
const nodeReplChromeSmokePassed =
  nodeReplChromeSmoke.exists &&
  nodeReplChromeSmoke.valid &&
  nodeReplChromeSmokeData.direct_js_smoke?.result === 'pass' &&
  typeof nodeReplChromeNoEnv.failure_class === 'string' &&
  nodeReplChromeNoEnv.failure_class.length > 0 &&
  typeof nodeReplChromeFullEnv.failure_class === 'string' &&
  nodeReplChromeFullEnv.failure_class.length > 0;
const nodeReplEnvMatrixSmoke = readOptionalJson(nodeReplEnvMatrixSmokePath);
const nodeReplEnvMatrixSmokeData = nodeReplEnvMatrixSmoke.data || {};
const nodeReplEnvMatrixCounts = nodeReplEnvMatrixSmokeData.counts || {};
const nodeReplEnvMatrixSmokePassed =
  nodeReplEnvMatrixSmoke.exists &&
  nodeReplEnvMatrixSmoke.valid &&
  typeof nodeReplEnvMatrixSmokeData.result === 'string' &&
  Array.isArray(nodeReplEnvMatrixSmokeData.matrix) &&
  nodeReplEnvMatrixSmokeData.counts &&
  typeof nodeReplEnvMatrixSmokeData.counts === 'object';
const scheduledChromeSmoke = readOptionalJson(scheduledChromeSmokePath);
const scheduledChromeSmokeData = scheduledChromeSmoke.data || {};
const scheduledChromeTask = scheduledChromeSmokeData.scheduled_task || {};
const scheduledChromeRunner = scheduledChromeSmokeData.runner || {};
const scheduledChromeSmokePassed =
  scheduledChromeSmoke.exists &&
  scheduledChromeSmoke.valid &&
  typeof scheduledChromeSmokeData.result === 'string' &&
  Array.isArray(scheduledChromeSmokeData.blockers);
const sshPrivilegePreflight = readOptionalJson(sshPrivilegePreflightPath);
const sshPrivilegeData = sshPrivilegePreflight.data || {};
const sshPrivilegeToken = sshPrivilegeData.token || {};
const sshPrivilegeFirewall = sshPrivilegeData.firewall || {};
const sshPrivilegeProcesses = (sshPrivilegeData.processes || {}).counts || {};
const sshPrivilegePassed =
  sshPrivilegePreflight.exists &&
  sshPrivilegePreflight.valid &&
  statusOk(sshPrivilegeData.result || sshPrivilegeData.status) &&
  sshPrivilegeToken.high_integrity === true &&
  sshPrivilegeToken.administrator_group &&
  sshPrivilegeToken.net_session &&
  sshPrivilegeToken.net_session.admin_check !== 'denied' &&
  sshPrivilegeFirewall.service &&
  sshPrivilegeFirewall.service.running === true &&
  Number(sshPrivilegeProcesses.codex) > 0 &&
  Number(sshPrivilegeProcesses.chrome) > 0 &&
  Number(sshPrivilegeProcesses.node_repl) > 0;
const payloadSelftest = readOptionalJson(payloadSelftestPath);
const payloadSelftestData = payloadSelftest.data || {};
const payloadSelftestCounts = payloadSelftestData.counts || {};
const payloadSelftestPassed =
  payloadSelftest.exists &&
  payloadSelftest.valid &&
  statusOk(payloadSelftestData.result || payloadSelftestData.status) &&
  Number(payloadSelftestData.passed) === Number(payloadSelftestData.total) &&
  Number(payloadSelftestData.total) > 0 &&
  Number(payloadSelftestCounts.visual_targets) === Number(captureSessionSummary.visual_batch_count || 0) &&
  Number(payloadSelftestCounts.interaction_targets) === Number(captureSessionSummary.interaction_batch_count || 0);
const bridgeToolCall = readOptionalJson(bridgeToolCallPath);
const bridgeToolCallData = bridgeToolCall.data || {};
const bridgeToolCallPayload = bridgeToolCallData.payload || {};
const bridgeToolCallSelftest = bridgeToolCallData.payload_selftest || {};
const bridgeToolCallArgs = bridgeToolCallData.arguments || {};
const bridgeToolCallPassed =
  bridgeToolCall.exists &&
  bridgeToolCall.valid &&
  statusOk(bridgeToolCallData.result || bridgeToolCallData.status) &&
  Number(bridgeToolCallData.passed) === Number(bridgeToolCallData.total) &&
  Number(bridgeToolCallData.total) > 0 &&
  bridgeToolCallData.tool_name === 'mcp__codex_desktop_node_repl__js' &&
  Number(bridgeToolCallArgs.timeout_ms) >= 600000 &&
  typeof bridgeToolCallPayload.sha256 === 'string' &&
  bridgeToolCallPayload.sha256.length === 64 &&
  Number(bridgeToolCallPayload.visual_target_count) === Number(captureSessionSummary.visual_batch_count || 0) &&
  Number(bridgeToolCallPayload.interaction_target_count) === Number(captureSessionSummary.interaction_batch_count || 0) &&
  statusOk(bridgeToolCallSelftest.result) &&
  Number(bridgeToolCallSelftest.passed) === Number(bridgeToolCallSelftest.total) &&
  Number(bridgeToolCallSelftest.total) > 0;
const bridgeRunResult = readOptionalJson(bridgeRunResultPath);
const bridgeRunData = bridgeRunResult.data || {};
const bridgeRunPassed =
  bridgeRunResult.exists &&
  bridgeRunResult.valid &&
  statusOk(bridgeRunData.result || bridgeRunData.status) &&
  bridgeRunData.backend === 'codex-desktop-chrome-extension' &&
  Number(bridgeRunData.visual_count) === Number(captureSessionSummary.visual_batch_count || 0) &&
  Number(bridgeRunData.interaction_count) === Number(captureSessionSummary.interaction_batch_count || 0);

jsonLog(
  'execution-package',
  'capture session covers current pending visual and interaction batches',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? captureSessionCoversCurrentGaps : true,
  captureSessionCoversCurrentGaps ? 'ok' : captureSessionStatus,
  captureSessionCoversCurrentGaps
    ? `capture session covers ${pendingVisualIds.length} visual gaps and ${pendingInteractionIds.length} interaction gaps`
    : `path=${captureSessionPath} exists=${captureSession.exists} valid=${captureSession.valid} counts_match=${captureSessionCountsMatch} missing_visual=${captureSessionMissingVisualIds.join(',') || 'none'} missing_interaction=${captureSessionMissingInteractionIds.join(',') || 'none'} reason=${captureSession.reason || captureSessionStatus}`,
  captureSessionPath,
);

jsonLog(
  'execution-package',
  'capture session includes Desktop Chrome 1920x1080 viewport probe step',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? captureSessionHasViewportCalibration : true,
  captureSessionHasViewportCalibration ? 'ok' : 'missing-viewport-calibration',
  captureSessionHasViewportCalibration
    ? `viewport_probe=${captureSessionViewportUrl} latest_result=${captureSessionViewportProbe.result || 'missing'}`
    : `path=${captureSessionPath} exists=${captureSession.exists} valid=${captureSession.valid} command_present=${captureSessionViewportCommand ? 'true' : 'false'} url=${captureSessionViewportUrl || 'missing'}`,
  captureSessionPath,
);

jsonLog(
  'execution-package',
  'latest Desktop Chrome viewport probe is pass at 1920x1080',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? captureSessionViewportProbeReady : true,
  captureSessionViewportProbeReady ? 'ok' : (captureSessionViewportProbe.result || (captureSessionViewportProbe.exists ? 'not-pass' : 'missing')),
  captureSessionViewportProbeReady
    ? `viewport=${captureSessionViewportProbe.viewport.width}x${captureSessionViewportProbe.viewport.height}`
    : `path=${captureSessionViewportProbe.path || viewportProbePath} exists=${captureSessionViewportProbe.exists} valid=${captureSessionViewportProbe.valid} result=${captureSessionViewportProbe.result || 'missing'} viewport=${captureSessionViewportProbe.viewport ? `${captureSessionViewportProbe.viewport.width}x${captureSessionViewportProbe.viewport.height}` : 'missing'} source=${captureSessionViewportProbe.viewport_source || 'missing'} reason=${captureSessionViewportProbe.mismatch_reason || 'not-pass'}`,
  captureSessionViewportProbe.path || viewportProbePath,
);

jsonLog(
  'execution-package',
  'Desktop capture receiver self-test passes',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? receiverSelftestPassed : true,
  receiverSelftestPassed ? 'ok' : (receiverSelftest.exists ? (receiverSelftest.valid ? 'not-pass' : 'invalid') : 'missing'),
  receiverSelftestPassed
    ? `run_id=${receiverSelftest.data?.run_id || 'unknown'} checks=${receiverSelftest.data?.passed}/${receiverSelftest.data?.total}`
    : `path=${receiverSelftestPath} exists=${receiverSelftest.exists} valid=${receiverSelftest.valid} result=${receiverSelftest.data?.result || 'missing'} checks=${receiverSelftest.data?.passed ?? 'missing'}/${receiverSelftest.data?.total ?? 'missing'} reason=${receiverSelftest.reason || 'not-pass'}`,
  receiverSelftestPath,
);

jsonLog(
  'execution-package',
  'Windows Codex bridge runtime preflight passes',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? bridgeRuntimePassed : true,
  bridgeRuntimePassed ? 'ok' : (bridgeRuntimePreflight.exists ? (bridgeRuntimePreflight.valid ? 'not-pass' : 'invalid') : 'missing'),
  bridgeRuntimePassed
    ? `chrome_clients=${bridgeRuntime.chrome_clients.length} processes chrome=${bridgeRuntimeProcessCounts.chrome} codex=${bridgeRuntimeProcessCounts.codex} code=${bridgeRuntimeProcessCounts.code}`
    : `path=${bridgeRuntimePreflightPath} exists=${bridgeRuntimePreflight.exists} valid=${bridgeRuntimePreflight.valid} result=${bridgeRuntimeData.result || bridgeRuntimeData.status || 'missing'} chrome_clients=${Array.isArray(bridgeRuntime.chrome_clients) ? bridgeRuntime.chrome_clients.length : 'missing'} processes chrome=${bridgeRuntimeProcessCounts.chrome ?? 'missing'} codex=${bridgeRuntimeProcessCounts.codex ?? 'missing'} code=${bridgeRuntimeProcessCounts.code ?? 'missing'} reason=${bridgeRuntimePreflight.reason || 'not-pass'}`,
  bridgeRuntimePreflightPath,
);

jsonLog(
  'execution-package',
  'Local Codex bridge plugin/proxy tool surface is diagnosed',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? toolSurfacePassed : true,
  toolSurfacePassed ? 'ok' : (toolSurfacePreflight.exists ? (toolSurfacePreflight.valid ? 'not-pass' : 'invalid') : 'missing'),
  toolSurfacePassed
    ? `plugin=${toolSurfaceData.plugin?.installed_enabled} backend_open=${toolSurfaceData.backend?.open} tools_list=${toolSurfaceData.proxy?.tools_list_status} session_tools=${toolSurfaceData.codex_session?.desktop_tool_status}`
    : `path=${toolSurfacePreflightPath} exists=${toolSurfacePreflight.exists} valid=${toolSurfacePreflight.valid} result=${toolSurfaceData.result || toolSurfaceData.status || 'missing'} reason=${toolSurfacePreflight.reason || 'not-pass'}`,
  toolSurfacePreflightPath,
);

jsonLog(
  'execution-package',
  'Windows Node REPL active_execs/proxy probe is current',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? activeExecsPassed : true,
  activeExecsPassed ? 'ok' : (activeExecsPreflight.exists ? (activeExecsPreflight.valid ? 'not-pass' : 'invalid') : 'missing'),
  activeExecsPassed
    ? `channel=${activeExecsData.channel_status} active_records=${activeExecsRuntime.active_exec_records?.length ?? 'missing'} live_records=${activeExecsRuntime.live_active_exec_records?.length ?? 'missing'} node_repl_pids=${activeExecsRuntime.running_node_repl_pids?.join(',') || 'none'} port19998=${activeExecsRuntime.port_19998_listeners?.length ?? 'missing'}`
    : `path=${activeExecsPreflightPath} exists=${activeExecsPreflight.exists} valid=${activeExecsPreflight.valid} result=${activeExecsData.result || activeExecsData.status || 'missing'} reason=${activeExecsPreflight.reason || 'not-pass'}`,
  activeExecsPreflightPath,
);

jsonLog(
  'execution-package',
  'Windows node_repl stdio JS and Chrome trust boundary smoke is current',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? nodeReplChromeSmokePassed : true,
  nodeReplChromeSmokePassed ? 'ok' : (nodeReplChromeSmoke.exists ? (nodeReplChromeSmoke.valid ? 'not-current' : 'invalid') : 'missing'),
  nodeReplChromeSmokePassed
    ? `result=${nodeReplChromeSmokeData.result} direct_js=${nodeReplChromeSmokeData.direct_js_smoke?.result} full_env_js=${nodeReplChromeSmokeData.full_env_js_smoke?.result} chrome_no_env=${nodeReplChromeNoEnv.result}:${nodeReplChromeNoEnv.failure_class} chrome=${nodeReplChromeFullEnv.result}:${nodeReplChromeFullEnv.failure_class}`
    : `path=${nodeReplChromeSmokePath} exists=${nodeReplChromeSmoke.exists} valid=${nodeReplChromeSmoke.valid} result=${nodeReplChromeSmokeData.result || nodeReplChromeSmokeData.status || 'missing'} direct_js=${nodeReplChromeSmokeData.direct_js_smoke?.result || 'missing'} chrome_no_env=${nodeReplChromeNoEnv.result || 'missing'}:${nodeReplChromeNoEnv.failure_class || 'missing'} chrome=${nodeReplChromeFullEnv.result || 'missing'}:${nodeReplChromeFullEnv.failure_class || 'missing'} reason=${nodeReplChromeSmoke.reason || 'not-current'}`,
  nodeReplChromeSmokePath,
);

jsonLog(
  'execution-package',
  'Windows node_repl environment matrix Chrome bridge smoke is diagnosed',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? nodeReplEnvMatrixSmokePassed : true,
  nodeReplEnvMatrixSmokePassed ? 'ok' : (nodeReplEnvMatrixSmoke.exists ? (nodeReplEnvMatrixSmoke.valid ? 'not-current' : 'invalid') : 'missing'),
  nodeReplEnvMatrixSmokePassed
    ? `result=${nodeReplEnvMatrixSmokeData.result} js_pass=${nodeReplEnvMatrixCounts.js_pass_cases ?? 'missing'}/${nodeReplEnvMatrixSmokeData.matrix.length} chrome_ready=${nodeReplEnvMatrixCounts.chrome_extension_ready_cases ?? 'missing'} native_pipe_or_trust=${nodeReplEnvMatrixCounts.chrome_native_pipe_or_trust_cases ?? 'missing'} sandbox_firewall=${nodeReplEnvMatrixCounts.chrome_sandbox_firewall_cases ?? 'missing'} blocker=${(nodeReplEnvMatrixSmokeData.blockers || [])[0] || 'none'}`
    : `path=${nodeReplEnvMatrixSmokePath} exists=${nodeReplEnvMatrixSmoke.exists} valid=${nodeReplEnvMatrixSmoke.valid} result=${nodeReplEnvMatrixSmokeData.result || nodeReplEnvMatrixSmokeData.status || 'missing'} reason=${nodeReplEnvMatrixSmoke.reason || 'not-current'}`,
  nodeReplEnvMatrixSmokePath,
);

jsonLog(
  'execution-package',
  'Windows scheduled-task node_repl Chrome bridge smoke is diagnosed',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? scheduledChromeSmokePassed : true,
  scheduledChromeSmokePassed ? 'ok' : (scheduledChromeSmoke.exists ? (scheduledChromeSmoke.valid ? 'not-current' : 'invalid') : 'missing'),
  scheduledChromeSmokePassed
    ? `result=${scheduledChromeSmokeData.result} create=${scheduledChromeTask.create?.ok ?? 'missing'} run=${scheduledChromeTask.run?.ok ?? 'missing'} runner=${scheduledChromeRunner.result || 'missing'} blocker=${(scheduledChromeSmokeData.blockers || [])[0] || 'none'}`
    : `path=${scheduledChromeSmokePath} exists=${scheduledChromeSmoke.exists} valid=${scheduledChromeSmoke.valid} result=${scheduledChromeSmokeData.result || scheduledChromeSmokeData.status || 'missing'} reason=${scheduledChromeSmoke.reason || 'not-current'}`,
  scheduledChromeSmokePath,
);

jsonLog(
  'execution-package',
  'Windows SSH privilege and process preflight narrows bridge blocker',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? sshPrivilegePassed : true,
  sshPrivilegePassed ? 'ok' : (sshPrivilegePreflight.exists ? (sshPrivilegePreflight.valid ? 'not-pass' : 'invalid') : 'missing'),
  sshPrivilegePassed
    ? `admin=${Boolean(sshPrivilegeToken.administrator_group)} high_integrity=${sshPrivilegeToken.high_integrity} privileges=${sshPrivilegeToken.enabled_privilege_count ?? 'unknown'} net_session=${sshPrivilegeToken.net_session.admin_check} firewall_service=${sshPrivilegeFirewall.service.running} processes codex=${sshPrivilegeProcesses.codex} chrome=${sshPrivilegeProcesses.chrome} node_repl=${sshPrivilegeProcesses.node_repl} powershell_blocked=${sshPrivilegeData.ssh?.powershell_smoke_blocked}`
    : `path=${sshPrivilegePreflightPath} exists=${sshPrivilegePreflight.exists} valid=${sshPrivilegePreflight.valid} result=${sshPrivilegeData.result || sshPrivilegeData.status || 'missing'} high_integrity=${sshPrivilegeToken.high_integrity ?? 'missing'} admin=${Boolean(sshPrivilegeToken.administrator_group)} net_session=${sshPrivilegeToken.net_session?.admin_check ?? 'missing'} firewall_service=${sshPrivilegeFirewall.service?.running ?? 'missing'} processes codex=${sshPrivilegeProcesses.codex ?? 'missing'} chrome=${sshPrivilegeProcesses.chrome ?? 'missing'} node_repl=${sshPrivilegeProcesses.node_repl ?? 'missing'} reason=${sshPrivilegePreflight.reason || 'not-pass'}`,
  sshPrivilegePreflightPath,
);

jsonLog(
  'execution-package',
  'Desktop Chrome bridge payload self-test passes and matches capture batch',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? payloadSelftestPassed : true,
  payloadSelftestPassed ? 'ok' : (payloadSelftest.exists ? (payloadSelftest.valid ? 'not-pass' : 'invalid') : 'missing'),
  payloadSelftestPassed
    ? `checks=${payloadSelftestData.passed}/${payloadSelftestData.total} visual=${payloadSelftestCounts.visual_targets} interaction=${payloadSelftestCounts.interaction_targets} receiver_uploads=${payloadSelftestCounts.receiver_uploads}`
    : `path=${payloadSelftestPath} exists=${payloadSelftest.exists} valid=${payloadSelftest.valid} result=${payloadSelftestData.result || payloadSelftestData.status || 'missing'} checks=${payloadSelftestData.passed ?? 'missing'}/${payloadSelftestData.total ?? 'missing'} visual=${payloadSelftestCounts.visual_targets ?? 'missing'}/${captureSessionSummary.visual_batch_count ?? 'missing'} interaction=${payloadSelftestCounts.interaction_targets ?? 'missing'}/${captureSessionSummary.interaction_batch_count ?? 'missing'} reason=${payloadSelftest.reason || 'not-pass'}`,
  payloadSelftestPath,
);

jsonLog(
  'execution-package',
  'Desktop Chrome bridge MCP tool-call template passes and matches capture batch',
  hasPendingCaptureWork ? 'warn' : 'info',
  hasPendingCaptureWork ? bridgeToolCallPassed : true,
  bridgeToolCallPassed ? 'ok' : (bridgeToolCall.exists ? (bridgeToolCall.valid ? 'not-pass' : 'invalid') : 'missing'),
  bridgeToolCallPassed
    ? `tool=${bridgeToolCallData.tool_name} checks=${bridgeToolCallData.passed}/${bridgeToolCallData.total} timeout_ms=${bridgeToolCallArgs.timeout_ms} sha256=${bridgeToolCallPayload.sha256}`
    : `path=${bridgeToolCallPath} exists=${bridgeToolCall.exists} valid=${bridgeToolCall.valid} result=${bridgeToolCallData.result || bridgeToolCallData.status || 'missing'} tool=${bridgeToolCallData.tool_name || 'missing'} checks=${bridgeToolCallData.passed ?? 'missing'}/${bridgeToolCallData.total ?? 'missing'} visual=${bridgeToolCallPayload.visual_target_count ?? 'missing'}/${captureSessionSummary.visual_batch_count ?? 'missing'} interaction=${bridgeToolCallPayload.interaction_target_count ?? 'missing'}/${captureSessionSummary.interaction_batch_count ?? 'missing'} timeout_ms=${bridgeToolCallArgs.timeout_ms ?? 'missing'} reason=${bridgeToolCall.reason || 'not-pass'}`,
  bridgeToolCallPath,
);

jsonLog(
  'execution-package',
  'Desktop Chrome bridge run summary is present and matches capture batch',
  'blocker',
  bridgeRunPassed,
  bridgeRunPassed ? 'ok' : (bridgeRunResult.exists ? (bridgeRunResult.valid ? 'not-pass' : 'invalid') : 'missing'),
  bridgeRunPassed
    ? `visual=${bridgeRunData.visual_count} interaction=${bridgeRunData.interaction_count} backend=${bridgeRunData.backend}`
    : `path=${bridgeRunResultPath} exists=${bridgeRunResult.exists} valid=${bridgeRunResult.valid} result=${bridgeRunData.result || bridgeRunData.status || 'missing'} backend=${bridgeRunData.backend || 'missing'} visual=${bridgeRunData.visual_count ?? 'missing'}/${captureSessionSummary.visual_batch_count ?? 'missing'} interaction=${bridgeRunData.interaction_count ?? 'missing'}/${captureSessionSummary.interaction_batch_count ?? 'missing'} reason=${bridgeRunResult.reason || 'not-pass'}`,
  bridgeRunResultPath,
);

jsonLog(
  'targets',
  'all visual target images exist at required size',
  'blocker',
  missingSources.length === 0,
  missingSources.length === 0 ? 'ok' : 'missing-or-bad-dimension',
  missingSources.length === 0
    ? `${sourceDimensionOk}/${visualTargetCount} source images are ${expectedWidth}x${expectedHeight}`
    : missingSources.map((target) => `${target.id}:${target.sourceImage || 'missing-sourceImage'}`).join(', '),
  visualAcceptancePath,
);

jsonLog(
  'implementation',
  'all visual routes resolve to React page components',
  'blocker',
  missingComponents.length === 0,
  missingComponents.length === 0 ? 'ok' : 'missing-component',
  missingComponents.length === 0 ? `${pagePresent}/${routes.length} page components present` : missingComponents.map((route) => `${route.id}:${route.componentPath}`).join(', '),
  'web/ui/src/pages',
);

jsonLog(
  'visual-diff',
  'every visual target has passing 1920x1080 screenshot diff evidence',
  'blocker',
  missingVisualEvidence.length === 0,
  missingVisualEvidence.length === 0 ? 'ok' : 'missing-or-failing-evidence',
  missingVisualEvidence.length === 0
    ? `${visualDiffPassed}/${visualTargetCount} visual diffs passed`
    : `${missingVisualEvidence.length}/${visualTargetCount} visual targets missing or failing visual diff evidence: ${missingVisualEvidence.map((target) => target.id).join(', ')}`,
  evidenceDir,
);

jsonLog(
  'business-interaction',
  'every route has passing business interaction evidence',
  'blocker',
  missingInteractionEvidence.length === 0,
  missingInteractionEvidence.length === 0 ? 'ok' : 'missing-or-failing-evidence',
  missingInteractionEvidence.length === 0
    ? `${interactionPassed}/${routes.length} business interactions passed`
    : `${missingInteractionEvidence.length}/${routes.length} routes missing or failing business interaction evidence: ${missingInteractionEvidence.map((route) => route.id).join(', ')}`,
  evidenceDir,
);

const desktopStatusNorm = String(desktopChromeStatus || '').toLowerCase();
const desktopOk = ['pass', 'passed', 'ok'].includes(desktopStatusNorm);
jsonLog(
  'desktop-chrome',
  'Codex Desktop Chrome wrapper is available for screenshot and interaction capture',
  'blocker',
  desktopOk,
  desktopStatusNorm || 'missing',
  desktopOk ? 'Desktop Chrome backend reported pass' : desktopChromeDetail,
  desktopChromeArtifact,
);

const matrix = {
  package_id: 'ui_visual_interaction_dual_gate',
  run_id: runId,
  generated_at: generatedAt,
  visual_acceptance: visualAcceptancePath,
  route_page_map: routePageMapPath,
  evidence_dir: evidenceDir,
  app_base_url: appBaseUrl,
  auth_capture_strategy: 'Open protected routes through the nonce-only smoke redirect helper, then verify hash consumption, final path, not-login shell, and token removal before accepting interaction evidence.',
  desktop_chrome_wrapper_tool: 'mcp__codex_desktop_node_repl.desktop_chrome_open_url',
  capture_session: {
    path: captureSessionPath,
    exists: captureSession.exists,
    valid: captureSession.valid,
    run_id: captureSession.data?.run_id || captureSession.data?.session_id || null,
    status: captureSessionStatus,
    covers_current_gaps: captureSessionCoversCurrentGaps,
    counts_match: captureSessionCountsMatch,
    visual_batch_count: captureSessionVisualBatch.length,
    interaction_batch_count: captureSessionInteractionBatch.length,
    missing_visual_target_ids: captureSessionMissingVisualIds,
    missing_interaction_route_ids: captureSessionMissingInteractionIds,
    viewport_calibration_required: captureSessionViewportRequired,
    viewport_probe_url: captureSessionViewportUrl || null,
    viewport_probe_command: captureSessionViewportCommand || null,
    viewport_probe_latest: captureSessionViewportProbe || null,
  },
  viewport_probe: {
    path: captureSessionViewportProbe.path || viewportProbePath,
    exists: captureSessionViewportProbe.exists,
    valid: captureSessionViewportProbe.valid,
    result: captureSessionViewportProbe.result || null,
    ready: captureSessionViewportProbeReady,
    viewport: captureSessionViewportProbe.viewport || null,
    viewport_source: captureSessionViewportProbe.viewport_source || null,
    window_metrics: captureSessionViewportProbe.window_metrics || null,
    screenshot_size: captureSessionViewportProbe.screenshot_size || null,
    expected_size: captureSessionViewportProbe.expected_size || null,
    mismatch_reason: captureSessionViewportProbe.mismatch_reason || null,
  },
  receiver_selftest: {
    path: receiverSelftestPath,
    exists: receiverSelftest.exists,
    valid: receiverSelftest.valid,
    result: receiverSelftest.data?.result || null,
    passed: receiverSelftest.data?.passed ?? null,
    total: receiverSelftest.data?.total ?? null,
    ready: receiverSelftestPassed,
    acceptance_effect: receiverSelftest.data?.acceptance_effect || null,
  },
  bridge_runtime_preflight: {
    path: bridgeRuntimePreflightPath,
    exists: bridgeRuntimePreflight.exists,
    valid: bridgeRuntimePreflight.valid,
    result: bridgeRuntimeData.result || bridgeRuntimeData.status || null,
    ready: bridgeRuntimePassed,
    chrome_client_count: Array.isArray(bridgeRuntime.chrome_clients) ? bridgeRuntime.chrome_clients.length : null,
    explicit_codex_exists: bridgeRuntime.explicit_codex_exists ?? null,
    explicit_codex_version_count: bridgeRuntimeExplicitCodexVersionLines.length,
    mcp_list_line_count: bridgeRuntimeMcpListLines.length,
    node_repl_mcp_result: bridgeRuntimeNodeReplMcpConfig.result || null,
    node_repl_mcp_transport_type: bridgeRuntimeNodeReplMcpConfig.transport_type || null,
    node_repl_mcp_env_key_count: Array.isArray(bridgeRuntimeNodeReplMcpConfig.env_keys) ? bridgeRuntimeNodeReplMcpConfig.env_keys.length : null,
    desktop_bridge_candidate_count: Array.isArray(bridgeRuntime.desktop_bridge_candidate_dirs) ? bridgeRuntime.desktop_bridge_candidate_dirs.length : null,
    node_repl_candidate_count: Array.isArray(bridgeRuntime.node_repl_candidate_dirs) ? bridgeRuntime.node_repl_candidate_dirs.length : null,
    process_counts: bridgeRuntimeProcessCounts,
  },
  tool_surface_preflight: {
    path: toolSurfacePreflightPath,
    exists: toolSurfacePreflight.exists,
    valid: toolSurfacePreflight.valid,
    result: toolSurfaceData.result || toolSurfaceData.status || null,
    ready: toolSurfacePassed,
    plugin_installed_enabled: toolSurfaceData.plugin?.installed_enabled ?? null,
    backend_open: toolSurfaceData.backend?.open ?? null,
    backend_detail: toolSurfaceData.backend?.detail || null,
    tools_list_status: toolSurfaceData.proxy?.tools_list_status || null,
    session_tool_status: toolSurfaceData.codex_session?.desktop_tool_status || null,
    inference: toolSurfaceData.inference || null,
  },
  active_execs_preflight: {
    path: activeExecsPreflightPath,
    exists: activeExecsPreflight.exists,
    valid: activeExecsPreflight.valid,
    result: activeExecsData.result || activeExecsData.status || null,
    ready: activeExecsPassed,
    channel_status: activeExecsData.channel_status || null,
    active_exec_record_count: Array.isArray(activeExecsRuntime.active_exec_records) ? activeExecsRuntime.active_exec_records.length : null,
    live_active_exec_record_count: Array.isArray(activeExecsRuntime.live_active_exec_records) ? activeExecsRuntime.live_active_exec_records.length : null,
    stale_active_exec_record_count: Array.isArray(activeExecsRuntime.stale_active_exec_records) ? activeExecsRuntime.stale_active_exec_records.length : null,
    running_node_repl_pids: Array.isArray(activeExecsRuntime.running_node_repl_pids) ? activeExecsRuntime.running_node_repl_pids : [],
    port_19998_listener_count: Array.isArray(activeExecsRuntime.port_19998_listeners) ? activeExecsRuntime.port_19998_listeners.length : null,
    inference: activeExecsData.inference || null,
  },
  ssh_privilege_preflight: {
    path: sshPrivilegePreflightPath,
    exists: sshPrivilegePreflight.exists,
    valid: sshPrivilegePreflight.valid,
    result: sshPrivilegeData.result || sshPrivilegeData.status || null,
    ready: sshPrivilegePassed,
    high_integrity: sshPrivilegeToken.high_integrity ?? null,
    administrator_group: sshPrivilegeToken.administrator_group || null,
    enabled_privilege_count: sshPrivilegeToken.enabled_privilege_count ?? null,
    net_session_admin_check: sshPrivilegeToken.net_session?.admin_check || null,
    firewall_service_running: sshPrivilegeFirewall.service?.running ?? null,
    process_counts: sshPrivilegeProcesses,
    powershell_smoke_blocked: sshPrivilegeData.ssh?.powershell_smoke_blocked ?? null,
    inference: sshPrivilegeData.inference || null,
  },
  payload_selftest: {
    path: payloadSelftestPath,
    exists: payloadSelftest.exists,
    valid: payloadSelftest.valid,
    result: payloadSelftestData.result || payloadSelftestData.status || null,
    passed: payloadSelftestData.passed ?? null,
    total: payloadSelftestData.total ?? null,
    ready: payloadSelftestPassed,
    counts: payloadSelftestCounts,
  },
  bridge_tool_call: {
    path: bridgeToolCallPath,
    exists: bridgeToolCall.exists,
    valid: bridgeToolCall.valid,
    result: bridgeToolCallData.result || bridgeToolCallData.status || null,
    ready: bridgeToolCallPassed,
    tool_name: bridgeToolCallData.tool_name || null,
    passed: bridgeToolCallData.passed ?? null,
    total: bridgeToolCallData.total ?? null,
    timeout_ms: bridgeToolCallArgs.timeout_ms ?? null,
    payload_sha256: bridgeToolCallPayload.sha256 || null,
    payload: {
      visual_target_count: bridgeToolCallPayload.visual_target_count ?? null,
      interaction_target_count: bridgeToolCallPayload.interaction_target_count ?? null,
      receiver_upload_count: bridgeToolCallPayload.receiver_upload_count ?? null,
    },
  },
  smoke_redirect: {
    base_url: smokeRedirectBaseUrl,
    nonce_placeholder: smokeNoncePlaceholder,
    token_param: smokeTokenParam,
    token_placeholder: smokeTokenPlaceholder,
    wait_ms: desktopChromeWaitMs,
  },
  expected_image_size: { width: expectedWidth, height: expectedHeight },
  visual_diff_max_pixel_ratio: maxPixelRatio,
  desktop_smoke_token_required: desktopSmokeTokenRequired,
  desktop_smoke_repo_config_ok: desktopSmokeRepoOk,
  desktop_smoke_live_config_ok: desktopSmokeLiveOk,
  live_config_url: liveConfigUrl,
  required_evidence_layout: {
    per_route: ['actual-1920.png', 'diff-1920.png', 'metrics.json', 'capture-meta.json', 'interaction.json'],
    metrics_required_fields: ['status/result pass', 'viewport 1920x1080', 'pixel_mismatch_ratio <= threshold'],
    capture_meta_required_fields: ['status/result pass', 'backend codex-desktop-chrome-extension', 'uploaded_size 1920x1080', 'stored_size 1920x1080', 'desktop_viewport 1920x1080', 'post_capture_resize false'],
    interaction_required_fields: ['status/result pass', 'Desktop Chrome backend pass', 'no_4xx_5xx', 'no_requestfailed', 'no_pageerror', 'no_console_error', 'business_action', 'target_screenshot'],
    protected_route_required_fields: ['final_url path equals requested route', 'smoke_hash_consumed true', 'not_login_shell true', 'final_url does not contain codex_smoke_token'],
  },
  forbidden_design_image_references: forbiddenRefs,
  visual_target_count: visualTargetCount,
  routes: routeDetails,
};
fs.writeFileSync(resolveRepo(matrixPath), JSON.stringify(matrix, null, 2) + '\n', 'utf8');

const visualGapItems = missingVisualEvidence.map((target) => {
  const missingFiles = [];
  if (!target.actualExists) missingFiles.push('actual-1920.png');
  if (!target.diffExists) missingFiles.push('diff-1920.png');
  if (!target.metricsExists) missingFiles.push('metrics.json');
  if (!target.captureMetaExists) missingFiles.push('capture-meta.json');
  return {
    target_id: target.id,
    route_id: target.routeId,
    route: target.route,
    resolved_path: target.resolvedPath,
    query: target.query || '',
    url: target.url,
    safe_redirect_url_pattern: target.safeRedirectUrlPattern,
    safe_wrapper_call: target.safeWrapperCall,
    source_image: target.sourceImage,
    evidence_dir: path.dirname(target.actual),
    required_files: ['actual-1920.png', 'diff-1920.png', 'metrics.json', 'capture-meta.json'],
    missing_files: missingFiles,
    reasons: target.reasons,
    metrics_command: target.metricsCommand.map(shellQuote).join(' '),
  };
});

const interactionGapItems = missingInteractionEvidence.map((route) => ({
  route_id: route.id,
  title: route.title,
  route: route.route,
  expected_final_path: route.resolvedPath,
  url: route.url,
  safe_redirect_url_pattern: route.safeRedirectUrlPattern,
  safe_wrapper_call: route.safeWrapperCall,
  interaction_template: route.interactionTemplate,
  output_path: route.interactionEvidence.interaction,
  business_action_hint: route.businessActionHint,
  api_endpoints: route.apiEndpoints || [],
  reasons: route.interactionEvidence.reasons,
}));
const combinedReasonGroups = countReasonGroups(visualGapItems);
for (const [key, value] of Object.entries(countReasonGroups(interactionGapItems))) {
  combinedReasonGroups[key] = (combinedReasonGroups[key] || 0) + value;
}
if (hasPendingCaptureWork && !captureSessionViewportProbeReady) {
  combinedReasonGroups.viewport_probe_blocked = (combinedReasonGroups.viewport_probe_blocked || 0) + 1;
}
if (hasPendingCaptureWork && !receiverSelftestPassed) {
  combinedReasonGroups.receiver_selftest_missing_or_failed = (combinedReasonGroups.receiver_selftest_missing_or_failed || 0) + 1;
}
if (hasPendingCaptureWork && !bridgeRuntimePassed) {
  combinedReasonGroups.bridge_runtime_preflight_missing_or_failed = (combinedReasonGroups.bridge_runtime_preflight_missing_or_failed || 0) + 1;
}
if (hasPendingCaptureWork && !toolSurfacePassed) {
  combinedReasonGroups.tool_surface_preflight_missing_or_failed = (combinedReasonGroups.tool_surface_preflight_missing_or_failed || 0) + 1;
}
if (hasPendingCaptureWork && !activeExecsPassed) {
  combinedReasonGroups.active_execs_preflight_missing_or_failed = (combinedReasonGroups.active_execs_preflight_missing_or_failed || 0) + 1;
}
if (hasPendingCaptureWork && !sshPrivilegePassed) {
  combinedReasonGroups.ssh_privilege_preflight_missing_or_failed = (combinedReasonGroups.ssh_privilege_preflight_missing_or_failed || 0) + 1;
}
if (hasPendingCaptureWork && !payloadSelftestPassed) {
  combinedReasonGroups.payload_selftest_missing_or_failed = (combinedReasonGroups.payload_selftest_missing_or_failed || 0) + 1;
}
if (hasPendingCaptureWork && !bridgeToolCallPassed) {
  combinedReasonGroups.bridge_tool_call_missing_or_failed = (combinedReasonGroups.bridge_tool_call_missing_or_failed || 0) + 1;
}

const gapReport = {
  package_id: 'ui_visual_interaction_gap_report',
  run_id: runId,
  generated_at: generatedAt,
  result_hint: missingVisualEvidence.length === 0 && missingInteractionEvidence.length === 0 && desktopOk ? 'ready-to-pass' : 'blocked',
  summary: {
    visual_required_count: visualTargetCount,
    visual_passed_count: visualDiffPassed,
    visual_gap_count: missingVisualEvidence.length,
    interaction_required_count: routes.length,
    interaction_passed_count: interactionPassed,
    interaction_gap_count: missingInteractionEvidence.length,
  },
  desktop_chrome: {
    status: desktopStatusNorm || 'missing',
    detail: desktopChromeDetail,
    artifact: desktopChromeArtifact,
    next_action: desktopOk
      ? 'Use the Chrome extension backend to capture the remaining visual and interaction evidence.'
      : 'Restore codex-desktop-node-repl / Chrome extension transport, then rerun capture plan and this preflight with DESKTOP_CHROME_STATUS=pass.',
  },
  capture_session: {
    path: captureSessionPath,
    exists: captureSession.exists,
    valid: captureSession.valid,
    run_id: captureSession.data?.run_id || captureSession.data?.session_id || null,
    status: captureSessionStatus,
    covers_current_gaps: captureSessionCoversCurrentGaps,
    counts_match: captureSessionCountsMatch,
    visual_batch_count: captureSessionVisualBatch.length,
    interaction_batch_count: captureSessionInteractionBatch.length,
    missing_visual_target_ids: captureSessionMissingVisualIds,
    missing_interaction_route_ids: captureSessionMissingInteractionIds,
    viewport_calibration_required: captureSessionViewportRequired,
    viewport_probe_url: captureSessionViewportUrl || null,
    viewport_probe_command: captureSessionViewportCommand || null,
    viewport_probe_latest: captureSessionViewportProbe || null,
  },
  viewport_probe: {
    path: captureSessionViewportProbe.path || viewportProbePath,
    exists: captureSessionViewportProbe.exists,
    valid: captureSessionViewportProbe.valid,
    result: captureSessionViewportProbe.result || null,
    ready: captureSessionViewportProbeReady,
    viewport: captureSessionViewportProbe.viewport || null,
    viewport_source: captureSessionViewportProbe.viewport_source || null,
    window_metrics: captureSessionViewportProbe.window_metrics || null,
    screenshot_size: captureSessionViewportProbe.screenshot_size || null,
    expected_size: captureSessionViewportProbe.expected_size || null,
    mismatch_reason: captureSessionViewportProbe.mismatch_reason || null,
    next_action: captureSessionViewportProbeReady
      ? 'Proceed to route screenshot and interaction capture.'
      : 'Open <receiver-url>/viewport-probe through Desktop Chrome and resolve browser viewport to 1920x1080 before uploading screenshots.',
  },
  receiver_selftest: {
    path: receiverSelftestPath,
    exists: receiverSelftest.exists,
    valid: receiverSelftest.valid,
    result: receiverSelftest.data?.result || null,
    passed: receiverSelftest.data?.passed ?? null,
    total: receiverSelftest.data?.total ?? null,
    ready: receiverSelftestPassed,
    next_action: receiverSelftestPassed
      ? 'Receiver endpoints are ready for real Desktop Chrome capture.'
      : 'Run python3 tests/e2e/ui_desktop_capture_receiver_selftest.py before starting Desktop Chrome capture.',
  },
  bridge_runtime_preflight: {
    path: bridgeRuntimePreflightPath,
    exists: bridgeRuntimePreflight.exists,
    valid: bridgeRuntimePreflight.valid,
    result: bridgeRuntimeData.result || bridgeRuntimeData.status || null,
    ready: bridgeRuntimePassed,
    chrome_client_count: Array.isArray(bridgeRuntime.chrome_clients) ? bridgeRuntime.chrome_clients.length : null,
    explicit_codex_exists: bridgeRuntime.explicit_codex_exists ?? null,
    explicit_codex_version_count: bridgeRuntimeExplicitCodexVersionLines.length,
    mcp_list_line_count: bridgeRuntimeMcpListLines.length,
    node_repl_mcp_result: bridgeRuntimeNodeReplMcpConfig.result || null,
    node_repl_mcp_transport_type: bridgeRuntimeNodeReplMcpConfig.transport_type || null,
    node_repl_mcp_env_key_count: Array.isArray(bridgeRuntimeNodeReplMcpConfig.env_keys) ? bridgeRuntimeNodeReplMcpConfig.env_keys.length : null,
    desktop_bridge_candidate_count: Array.isArray(bridgeRuntime.desktop_bridge_candidate_dirs) ? bridgeRuntime.desktop_bridge_candidate_dirs.length : null,
    node_repl_candidate_count: Array.isArray(bridgeRuntime.node_repl_candidate_dirs) ? bridgeRuntime.node_repl_candidate_dirs.length : null,
    process_counts: bridgeRuntimeProcessCounts,
    next_action: bridgeRuntimePassed
      ? 'Windows Codex/Chrome runtime prerequisites are ready; expose the current-session Desktop Chrome MCP bridge and run the generated payload.'
      : 'Run SSHPASS=<redacted> node tests/e2e/ui_windows_codex_bridge_runtime_preflight.mjs and fix Windows runtime prerequisites before Chrome capture.',
  },
  tool_surface_preflight: {
    path: toolSurfacePreflightPath,
    exists: toolSurfacePreflight.exists,
    valid: toolSurfacePreflight.valid,
    result: toolSurfaceData.result || toolSurfaceData.status || null,
    ready: toolSurfacePassed,
    plugin_installed_enabled: toolSurfaceData.plugin?.installed_enabled ?? null,
    backend_open: toolSurfaceData.backend?.open ?? null,
    backend_detail: toolSurfaceData.backend?.detail || null,
    tools_list_status: toolSurfaceData.proxy?.tools_list_status || null,
    session_tool_status: toolSurfaceData.codex_session?.desktop_tool_status || null,
    inference: toolSurfaceData.inference || null,
    next_action: toolSurfacePassed
      ? 'Use this as local bridge tooling boundary evidence; formal capture still requires callable current-session Desktop Chrome tools.'
      : 'Run node tests/e2e/ui_codex_bridge_tool_surface_preflight.mjs before Desktop Chrome capture.',
  },
  active_execs_preflight: {
    path: activeExecsPreflightPath,
    exists: activeExecsPreflight.exists,
    valid: activeExecsPreflight.valid,
    result: activeExecsData.result || activeExecsData.status || null,
    ready: activeExecsPassed,
    channel_status: activeExecsData.channel_status || null,
    active_exec_record_count: Array.isArray(activeExecsRuntime.active_exec_records) ? activeExecsRuntime.active_exec_records.length : null,
    live_active_exec_record_count: Array.isArray(activeExecsRuntime.live_active_exec_records) ? activeExecsRuntime.live_active_exec_records.length : null,
    stale_active_exec_record_count: Array.isArray(activeExecsRuntime.stale_active_exec_records) ? activeExecsRuntime.stale_active_exec_records.length : null,
    running_node_repl_pids: Array.isArray(activeExecsRuntime.running_node_repl_pids) ? activeExecsRuntime.running_node_repl_pids : [],
    port_19998_listener_count: Array.isArray(activeExecsRuntime.port_19998_listeners) ? activeExecsRuntime.port_19998_listeners.length : null,
    inference: activeExecsData.inference || null,
    next_action: activeExecsPassed
      ? 'Use this as boundary evidence only; the formal capture still requires the trusted Desktop Chrome MCP tool to upload bridge-run evidence.'
      : 'Run SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_active_execs_preflight.mjs to refresh active_execs/proxy channel evidence.',
  },
  node_repl_chrome_smoke: {
    path: nodeReplChromeSmokePath,
    exists: nodeReplChromeSmoke.exists,
    valid: nodeReplChromeSmoke.valid,
    result: nodeReplChromeSmokeData.result || nodeReplChromeSmokeData.status || null,
    ready: nodeReplChromeSmokePassed,
    direct_js_result: nodeReplChromeSmokeData.direct_js_smoke?.result || null,
    full_env_js_result: nodeReplChromeSmokeData.full_env_js_smoke?.result || null,
    chrome_no_env_result: nodeReplChromeNoEnv.result || null,
    chrome_no_env_failure_class: nodeReplChromeNoEnv.failure_class || null,
    chrome_full_env_result: nodeReplChromeFullEnv.result || null,
    chrome_full_env_failure_class: nodeReplChromeFullEnv.failure_class || null,
    acceptance_effect: nodeReplChromeSmokeData.acceptance_effect || null,
    next_action: nodeReplChromeSmokePassed
      ? 'Use this as boundary evidence: SSH stdio can execute minimal JS, but formal Chrome capture still requires the trusted Desktop native-pipe context.'
      : 'Run SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_chrome_bridge_smoke.mjs to refresh Windows node_repl stdio and Chrome trust-boundary evidence.',
  },
  node_repl_env_matrix_smoke: {
    path: nodeReplEnvMatrixSmokePath,
    exists: nodeReplEnvMatrixSmoke.exists,
    valid: nodeReplEnvMatrixSmoke.valid,
    result: nodeReplEnvMatrixSmokeData.result || nodeReplEnvMatrixSmokeData.status || null,
    ready: nodeReplEnvMatrixSmokePassed,
    matrix_case_count: Array.isArray(nodeReplEnvMatrixSmokeData.matrix) ? nodeReplEnvMatrixSmokeData.matrix.length : null,
    js_pass_cases: nodeReplEnvMatrixCounts.js_pass_cases ?? null,
    chrome_extension_ready_cases: nodeReplEnvMatrixCounts.chrome_extension_ready_cases ?? null,
    chrome_native_pipe_or_trust_cases: nodeReplEnvMatrixCounts.chrome_native_pipe_or_trust_cases ?? null,
    chrome_sandbox_firewall_cases: nodeReplEnvMatrixCounts.chrome_sandbox_firewall_cases ?? null,
    first_blocker: (nodeReplEnvMatrixSmokeData.blockers || [])[0] || null,
    acceptance_effect: nodeReplEnvMatrixSmokeData.acceptance_effect || null,
    next_action: nodeReplEnvMatrixSmokePassed
      ? 'Use this as boundary evidence: selected SSH-spawned node_repl env subsets were tested and none reached the Chrome extension backend.'
      : 'Run SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_env_matrix_smoke.mjs to refresh node_repl env matrix evidence.',
  },
  scheduled_chrome_smoke: {
    path: scheduledChromeSmokePath,
    exists: scheduledChromeSmoke.exists,
    valid: scheduledChromeSmoke.valid,
    result: scheduledChromeSmokeData.result || scheduledChromeSmokeData.status || null,
    ready: scheduledChromeSmokePassed,
    scheduled_task_create_ok: scheduledChromeTask.create?.ok ?? null,
    scheduled_task_run_ok: scheduledChromeTask.run?.ok ?? null,
    runner_result: scheduledChromeRunner.result || null,
    first_blocker: (scheduledChromeSmokeData.blockers || [])[0] || null,
    acceptance_effect: scheduledChromeSmokeData.acceptance_effect || null,
    next_action: scheduledChromeSmokePassed
      ? 'Use this as boundary evidence: scheduled-task automation was tested and does not replace the trusted current-session Desktop Chrome bridge.'
      : 'Run SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_scheduled_chrome_smoke.mjs to refresh scheduled-task Chrome bridge boundary evidence.',
  },
  ssh_privilege_preflight: {
    path: sshPrivilegePreflightPath,
    exists: sshPrivilegePreflight.exists,
    valid: sshPrivilegePreflight.valid,
    result: sshPrivilegeData.result || sshPrivilegeData.status || null,
    ready: sshPrivilegePassed,
    high_integrity: sshPrivilegeToken.high_integrity ?? null,
    administrator_group: sshPrivilegeToken.administrator_group || null,
    enabled_privilege_count: sshPrivilegeToken.enabled_privilege_count ?? null,
    net_session_admin_check: sshPrivilegeToken.net_session?.admin_check || null,
    firewall_service_running: sshPrivilegeFirewall.service?.running ?? null,
    process_counts: sshPrivilegeProcesses,
    powershell_smoke_blocked: sshPrivilegeData.ssh?.powershell_smoke_blocked ?? null,
    inference: sshPrivilegeData.inference || null,
    next_action: sshPrivilegePassed
      ? 'SSH privilege/process evidence is ready; continue to the trusted Desktop Chrome MCP bridge execution path rather than treating SSH permissions or process absence as the blocker.'
      : 'Run SSHPASS=<redacted> node tests/e2e/ui_windows_ssh_privilege_preflight.mjs to verify SSH token, firewall service, and Desktop process visibility.',
  },
  payload_selftest: {
    path: payloadSelftestPath,
    exists: payloadSelftest.exists,
    valid: payloadSelftest.valid,
    result: payloadSelftestData.result || payloadSelftestData.status || null,
    passed: payloadSelftestData.passed ?? null,
    total: payloadSelftestData.total ?? null,
    ready: payloadSelftestPassed,
    counts: payloadSelftestCounts,
    next_action: payloadSelftestPassed
      ? 'Generated Desktop Chrome payload covers the current gap report and is ready to execute once the bridge tool is exposed.'
      : 'Run node tests/e2e/ui_desktop_chrome_bridge_payload_selftest.mjs after regenerating capture session and payload.',
  },
  bridge_tool_call: {
    path: bridgeToolCallPath,
    exists: bridgeToolCall.exists,
    valid: bridgeToolCall.valid,
    result: bridgeToolCallData.result || bridgeToolCallData.status || null,
    ready: bridgeToolCallPassed,
    tool_name: bridgeToolCallData.tool_name || null,
    passed: bridgeToolCallData.passed ?? null,
    total: bridgeToolCallData.total ?? null,
    timeout_ms: bridgeToolCallArgs.timeout_ms ?? null,
    payload_sha256: bridgeToolCallPayload.sha256 || null,
    payload: {
      visual_target_count: bridgeToolCallPayload.visual_target_count ?? null,
      interaction_target_count: bridgeToolCallPayload.interaction_target_count ?? null,
      receiver_upload_count: bridgeToolCallPayload.receiver_upload_count ?? null,
    },
    next_action: bridgeToolCallPassed
      ? 'Call mcp__codex_desktop_node_repl__js from the trusted Windows Codex Desktop / VSCode session and pass the JSON arguments from this template after replacing placeholders.'
      : 'Run node tests/e2e/ui_desktop_chrome_bridge_tool_call.mjs after regenerating the Desktop Chrome payload.',
  },
  receiver_requirements: {
    viewport_probe_url: '<receiver-url>/viewport-probe',
    viewport_probe_latest_artifact: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-viewport-probe-latest.json',
    receiver_selftest_latest_artifact: 'doc/02_acceptance/02-regression/ui-visual-interaction/receiver-selftest-latest.json',
    visual_upload_endpoint_pattern: '<receiver-url>/upload/<visual-target-id>',
    interaction_upload_endpoint_pattern: '<receiver-url>/interaction/<route-id>',
    auth_header: 'X-Codex-Capture-Key',
    required_visual_headers: [
      'X-Codex-Desktop-Viewport-Width=1920',
      'X-Codex-Desktop-Viewport-Height=1080',
      'X-Codex-Desktop-Device-Pixel-Ratio=<observed>',
    ],
    accepted_image_formats: ['image/png', 'image/jpeg'],
    sensitive_material_rejected: ['Bearer tokens', 'codex_smoke_token=', 'refresh_token'],
  },
  reason_groups: combinedReasonGroups,
  formal_commands: [
    'python3 tests/e2e/ui_desktop_capture_receiver_selftest.py',
    'tests/e2e/ui_desktop_capture_receiver.py --host 0.0.0.0 --port 15174 --evidence-dir doc/02_acceptance/02-regression/ui-visual-interaction/latest --expected-width 1920 --expected-height 1080',
    'DESKTOP_SMOKE_TOKEN=<redacted> CODEX_SMOKE_NONCE=<redacted> tests/e2e/ui_desktop_smoke_redirect.py --host 0.0.0.0 --port 15175 --app-base-url http://10.0.5.8:30180 --default-route /dashboard --max-redirects 1',
    'tests/e2e/ui_desktop_capture_plan.mjs --base-url http://10.0.5.8:30180 --receiver-url http://10.0.5.8:15174 --smoke-redirect-base-url http://10.0.5.8:15175',
    'tests/e2e/ui_desktop_capture_session.mjs --session-id <session-id> --receiver-url http://10.0.5.8:15174 --smoke-redirect-base-url http://10.0.5.8:15175 --receiver-port 15174 --redirect-port 15175',
    'mcp__codex_desktop_node_repl.desktop_chrome_open_url url=http://10.0.5.8:15174/viewport-probe keep=true wait_ms=1500',
    "DESKTOP_CHROME_STATUS=pass DESKTOP_CHROME_DETAIL='Chrome extension backend captured all listed evidence' ALLOW_BLOCKERS=false RUN_ID=<next-run-id> tests/e2e/live_ui_visual_interaction_preflight.sh",
    'ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh',
  ],
  visual_gaps: visualGapItems,
  interaction_gaps: interactionGapItems,
};
fs.writeFileSync(resolveRepo(gapReportJsonPath), JSON.stringify(gapReport, null, 2) + '\n', 'utf8');
fs.writeFileSync(resolveRepo(gapReportMdPath), renderGapReportMd(gapReport), 'utf8');

const counts = {
  target_route_count: routes.length,
  visual_target_count: visualTargetCount,
  target_source_image_present_count: sourcePresent,
  target_source_image_dimension_ok_count: sourceDimensionOk,
  react_page_present_count: pagePresent,
  visual_diff_required_count: visualTargetCount,
  visual_diff_passed_count: visualDiffPassed,
  interaction_required_count: routes.length,
  interaction_passed_count: interactionPassed,
  full_page_design_image_reference_count: forbiddenRefs.length,
  desktop_smoke_required: desktopSmokeTokenRequired,
  desktop_smoke_repo_config_ok: desktopSmokeRepoOk,
  desktop_smoke_live_config_ok: desktopSmokeLiveOk,
  live_config_url: liveConfigUrl,
  missing_source_target_ids: missingSources.map((target) => target.id),
  missing_component_route_ids: missingComponents.map((route) => route.id),
  missing_visual_diff_target_ids: missingVisualEvidence.map((target) => target.id),
  missing_interaction_route_ids: missingInteractionEvidence.map((route) => route.id),
  desktop_chrome_status: desktopStatusNorm || 'missing',
  desktop_chrome_detail: desktopChromeDetail,
  desktop_chrome_artifact: desktopChromeArtifact,
  capture_session_path: captureSessionPath,
  capture_session_exists: captureSession.exists,
  capture_session_valid: captureSession.valid,
  capture_session_run_id: captureSession.data?.run_id || captureSession.data?.session_id || null,
  capture_session_status: captureSessionStatus,
  capture_session_covers_current_gaps: captureSessionCoversCurrentGaps,
  capture_session_visual_batch_count: captureSessionVisualBatch.length,
  capture_session_interaction_batch_count: captureSessionInteractionBatch.length,
  capture_session_missing_visual_target_ids: captureSessionMissingVisualIds,
  capture_session_missing_interaction_route_ids: captureSessionMissingInteractionIds,
  capture_session_viewport_calibration_present: captureSessionHasViewportCalibration,
  capture_session_viewport_probe_url: captureSessionViewportUrl || null,
  capture_session_viewport_probe_latest: captureSessionViewportProbe || null,
  viewport_probe_ready: captureSessionViewportProbeReady,
  viewport_probe_path: captureSessionViewportProbe.path || viewportProbePath,
  viewport_probe_result: captureSessionViewportProbe.result || null,
  viewport_probe_size: captureSessionViewportProbe.viewport || null,
  viewport_probe_source: captureSessionViewportProbe.viewport_source || null,
  viewport_probe_mismatch_reason: captureSessionViewportProbe.mismatch_reason || null,
  receiver_selftest_path: receiverSelftestPath,
  receiver_selftest_exists: receiverSelftest.exists,
  receiver_selftest_valid: receiverSelftest.valid,
  receiver_selftest_result: receiverSelftest.data?.result || null,
  receiver_selftest_passed: receiverSelftest.data?.passed ?? null,
  receiver_selftest_total: receiverSelftest.data?.total ?? null,
  receiver_selftest_ready: receiverSelftestPassed,
  bridge_runtime_preflight_path: bridgeRuntimePreflightPath,
  bridge_runtime_preflight_exists: bridgeRuntimePreflight.exists,
  bridge_runtime_preflight_valid: bridgeRuntimePreflight.valid,
  bridge_runtime_preflight_result: bridgeRuntimeData.result || bridgeRuntimeData.status || null,
  bridge_runtime_preflight_ready: bridgeRuntimePassed,
  bridge_runtime_chrome_client_count: Array.isArray(bridgeRuntime.chrome_clients) ? bridgeRuntime.chrome_clients.length : null,
  bridge_runtime_chrome_clients: Array.isArray(bridgeRuntime.chrome_clients) ? bridgeRuntime.chrome_clients.length : null,
  bridge_runtime_explicit_codex_exists: bridgeRuntime.explicit_codex_exists ?? null,
  bridge_runtime_explicit_codex_version_count: bridgeRuntimeExplicitCodexVersionLines.length,
  bridge_runtime_mcp_list_line_count: bridgeRuntimeMcpListLines.length,
  bridge_runtime_node_repl_mcp_result: bridgeRuntimeNodeReplMcpConfig.result || null,
  bridge_runtime_node_repl_mcp_transport_type: bridgeRuntimeNodeReplMcpConfig.transport_type || null,
  bridge_runtime_node_repl_mcp_env_key_count: Array.isArray(bridgeRuntimeNodeReplMcpConfig.env_keys) ? bridgeRuntimeNodeReplMcpConfig.env_keys.length : null,
  bridge_runtime_desktop_bridge_candidate_count: Array.isArray(bridgeRuntime.desktop_bridge_candidate_dirs) ? bridgeRuntime.desktop_bridge_candidate_dirs.length : null,
  bridge_runtime_node_repl_candidate_count: Array.isArray(bridgeRuntime.node_repl_candidate_dirs) ? bridgeRuntime.node_repl_candidate_dirs.length : null,
  bridge_runtime_process_counts: bridgeRuntimeProcessCounts,
  tool_surface_preflight_path: toolSurfacePreflightPath,
  tool_surface_preflight_exists: toolSurfacePreflight.exists,
  tool_surface_preflight_valid: toolSurfacePreflight.valid,
  tool_surface_preflight_result: toolSurfaceData.result || toolSurfaceData.status || null,
  tool_surface_preflight_ready: toolSurfacePassed,
  tool_surface_plugin_installed_enabled: toolSurfaceData.plugin?.installed_enabled ?? null,
  tool_surface_backend_open: toolSurfaceData.backend?.open ?? null,
  tool_surface_tools_list_status: toolSurfaceData.proxy?.tools_list_status || null,
  tool_surface_session_tool_status: toolSurfaceData.codex_session?.desktop_tool_status || null,
  active_execs_preflight_path: activeExecsPreflightPath,
  active_execs_preflight_exists: activeExecsPreflight.exists,
  active_execs_preflight_valid: activeExecsPreflight.valid,
  active_execs_preflight_result: activeExecsData.result || activeExecsData.status || null,
  active_execs_preflight_ready: activeExecsPassed,
  active_execs_channel_status: activeExecsData.channel_status || null,
  active_execs_record_count: Array.isArray(activeExecsRuntime.active_exec_records) ? activeExecsRuntime.active_exec_records.length : null,
  active_execs_live_record_count: Array.isArray(activeExecsRuntime.live_active_exec_records) ? activeExecsRuntime.live_active_exec_records.length : null,
  active_execs_stale_record_count: Array.isArray(activeExecsRuntime.stale_active_exec_records) ? activeExecsRuntime.stale_active_exec_records.length : null,
  active_execs_running_node_repl_pids: Array.isArray(activeExecsRuntime.running_node_repl_pids) ? activeExecsRuntime.running_node_repl_pids : [],
  active_execs_port_19998_listener_count: Array.isArray(activeExecsRuntime.port_19998_listeners) ? activeExecsRuntime.port_19998_listeners.length : null,
  node_repl_chrome_smoke_path: nodeReplChromeSmokePath,
  node_repl_chrome_smoke_exists: nodeReplChromeSmoke.exists,
  node_repl_chrome_smoke_valid: nodeReplChromeSmoke.valid,
  node_repl_chrome_smoke_result: nodeReplChromeSmokeData.result || nodeReplChromeSmokeData.status || null,
  node_repl_chrome_smoke_ready: nodeReplChromeSmokePassed,
  node_repl_direct_js_result: nodeReplChromeSmokeData.direct_js_smoke?.result || null,
  node_repl_full_env_js_result: nodeReplChromeSmokeData.full_env_js_smoke?.result || null,
  node_repl_chrome_no_env_result: nodeReplChromeNoEnv.result || null,
  node_repl_chrome_no_env_failure_class: nodeReplChromeNoEnv.failure_class || null,
  node_repl_chrome_full_env_result: nodeReplChromeFullEnv.result || null,
  node_repl_chrome_full_env_failure_class: nodeReplChromeFullEnv.failure_class || null,
  node_repl_env_matrix_smoke_path: nodeReplEnvMatrixSmokePath,
  node_repl_env_matrix_smoke_exists: nodeReplEnvMatrixSmoke.exists,
  node_repl_env_matrix_smoke_valid: nodeReplEnvMatrixSmoke.valid,
  node_repl_env_matrix_smoke_result: nodeReplEnvMatrixSmokeData.result || nodeReplEnvMatrixSmokeData.status || null,
  node_repl_env_matrix_smoke_ready: nodeReplEnvMatrixSmokePassed,
  node_repl_env_matrix_case_count: Array.isArray(nodeReplEnvMatrixSmokeData.matrix) ? nodeReplEnvMatrixSmokeData.matrix.length : null,
  node_repl_env_matrix_js_pass_cases: nodeReplEnvMatrixCounts.js_pass_cases ?? null,
  node_repl_env_matrix_chrome_extension_ready_cases: nodeReplEnvMatrixCounts.chrome_extension_ready_cases ?? null,
  node_repl_env_matrix_chrome_native_pipe_or_trust_cases: nodeReplEnvMatrixCounts.chrome_native_pipe_or_trust_cases ?? null,
  node_repl_env_matrix_chrome_sandbox_firewall_cases: nodeReplEnvMatrixCounts.chrome_sandbox_firewall_cases ?? null,
  node_repl_env_matrix_first_blocker: (nodeReplEnvMatrixSmokeData.blockers || [])[0] || null,
  scheduled_chrome_smoke_path: scheduledChromeSmokePath,
  scheduled_chrome_smoke_exists: scheduledChromeSmoke.exists,
  scheduled_chrome_smoke_valid: scheduledChromeSmoke.valid,
  scheduled_chrome_smoke_result: scheduledChromeSmokeData.result || scheduledChromeSmokeData.status || null,
  scheduled_chrome_smoke_ready: scheduledChromeSmokePassed,
  scheduled_chrome_task_create_ok: scheduledChromeTask.create?.ok ?? null,
  scheduled_chrome_task_run_ok: scheduledChromeTask.run?.ok ?? null,
  scheduled_chrome_runner_result: scheduledChromeRunner.result || null,
  scheduled_chrome_first_blocker: (scheduledChromeSmokeData.blockers || [])[0] || null,
  ssh_privilege_preflight_path: sshPrivilegePreflightPath,
  ssh_privilege_preflight_exists: sshPrivilegePreflight.exists,
  ssh_privilege_preflight_valid: sshPrivilegePreflight.valid,
  ssh_privilege_preflight_result: sshPrivilegeData.result || sshPrivilegeData.status || null,
  ssh_privilege_preflight_ready: sshPrivilegePassed,
  ssh_privilege_high_integrity: sshPrivilegeToken.high_integrity ?? null,
  ssh_privilege_admin_group: sshPrivilegeToken.administrator_group || null,
  ssh_privilege_enabled_privilege_count: sshPrivilegeToken.enabled_privilege_count ?? null,
  ssh_privilege_net_session_admin_check: sshPrivilegeToken.net_session?.admin_check || null,
  ssh_privilege_firewall_service_running: sshPrivilegeFirewall.service?.running ?? null,
  ssh_privilege_process_counts: sshPrivilegeProcesses,
  ssh_privilege_powershell_smoke_blocked: sshPrivilegeData.ssh?.powershell_smoke_blocked ?? null,
  payload_selftest_path: payloadSelftestPath,
  payload_selftest_exists: payloadSelftest.exists,
  payload_selftest_valid: payloadSelftest.valid,
  payload_selftest_result: payloadSelftestData.result || payloadSelftestData.status || null,
  payload_selftest_passed: payloadSelftestData.passed ?? null,
  payload_selftest_total: payloadSelftestData.total ?? null,
  payload_selftest_ready: payloadSelftestPassed,
  payload_selftest_counts: payloadSelftestCounts,
  bridge_tool_call_path: bridgeToolCallPath,
  bridge_tool_call_exists: bridgeToolCall.exists,
  bridge_tool_call_valid: bridgeToolCall.valid,
  bridge_tool_call_result: bridgeToolCallData.result || bridgeToolCallData.status || null,
  bridge_tool_call_ready: bridgeToolCallPassed,
  bridge_tool_call_tool_name: bridgeToolCallData.tool_name || null,
  bridge_tool_call_passed: bridgeToolCallData.passed ?? null,
  bridge_tool_call_total: bridgeToolCallData.total ?? null,
  bridge_tool_call_timeout_ms: bridgeToolCallArgs.timeout_ms ?? null,
  bridge_tool_call_payload_sha256: bridgeToolCallPayload.sha256 || null,
  bridge_tool_call_payload: {
    visual_target_count: bridgeToolCallPayload.visual_target_count ?? null,
    interaction_target_count: bridgeToolCallPayload.interaction_target_count ?? null,
    receiver_upload_count: bridgeToolCallPayload.receiver_upload_count ?? null,
  },
};
fs.writeFileSync(resolveRepo(countsPath), JSON.stringify(counts, null, 2) + '\n', 'utf8');
JS

finalize

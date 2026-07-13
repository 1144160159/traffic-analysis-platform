#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const defaults = {
  payloadJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.json',
  payloadJs: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.js',
  captureSession: 'doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json',
  gapReport: 'doc/02_acceptance/02-regression/ui-visual-interaction-gap-report-latest.json',
  bridgeHostPreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-desktop-bridge-host-preflight-latest.json',
  bridgeRuntimePreflight: 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-codex-bridge-runtime-preflight-latest.json',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-selftest-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-selftest-latest.md',
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

function readTextState(file) {
  const full = resolveRepo(file);
  if (!fs.existsSync(full)) return { exists: false, text: '', error: 'missing' };
  try {
    return { exists: true, text: fs.readFileSync(full, 'utf8'), error: null };
  } catch (error) {
    return { exists: true, text: '', error: error.message };
  }
}

function countLiteralArrayItems(source, name, idKey) {
  const marker = `const ${name} = `;
  const markerIndex = source.indexOf(marker);
  if (markerIndex < 0) return null;
  const arrayStart = source.indexOf('[', markerIndex + marker.length);
  if (arrayStart < 0) return null;
  let depth = 0;
  let inString = false;
  let escaped = false;
  let arrayEnd = -1;
  for (let index = arrayStart; index < source.length; index += 1) {
    const char = source[index];
    if (inString) {
      if (escaped) {
        escaped = false;
      } else if (char === '\\') {
        escaped = true;
      } else if (char === '"') {
        inString = false;
      }
      continue;
    }
    if (char === '"') inString = true;
    else if (char === '[') depth += 1;
    else if (char === ']') {
      depth -= 1;
      if (depth === 0) {
        arrayEnd = index + 1;
        break;
      }
    }
  }
  if (arrayEnd < 0) return null;
  try {
    const parsed = JSON.parse(source.slice(arrayStart, arrayEnd));
    return {
      count: Array.isArray(parsed) ? parsed.length : null,
      ids: Array.isArray(parsed) ? parsed.map((item) => item[idKey]).filter(Boolean) : [],
    };
  } catch {
    return null;
  }
}

function addCheck(checks, name, passed, detail, artifact = '') {
  checks.push({ name, passed, status: passed ? 'ok' : 'fail', detail, artifact });
}

function parsesAsAsyncFunction(source) {
  try {
    // Parse only. The payload runs as top-level JS in the Desktop Node REPL;
    // an async wrapper lets this self-test accept top-level await syntax.
    new Function(`return (async () => {\n${source}\n});`);
    return { ok: true, error: '' };
  } catch (error) {
    return { ok: false, error: error.message };
  }
}

function renderMarkdown(summary) {
  const lines = [
    '# Desktop Chrome Bridge Payload Self-Test',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Payload: \`${summary.payload_json.path}\``,
    `- Payload JS: \`${summary.payload_js.path}\``,
    `- Visual targets: \`${summary.counts.visual_targets}\``,
    `- Interaction targets: \`${summary.counts.interaction_targets}\``,
    `- Screenshot uploads: \`${summary.counts.screenshot_uploads}\``,
    `- Bridge result uploads: \`${summary.counts.bridge_result_uploads}\``,
    `- Receiver uploads: \`${summary.counts.receiver_uploads}\``,
    '',
    '## Checks',
    '',
  ];
  for (const check of summary.checks) {
    lines.push(`- ${check.passed ? 'pass' : 'fail'}: ${check.name} (${check.detail})`);
  }
  lines.push('');
  return `${lines.join('\n')}\n`;
}

const payloadState = readJsonState(args.payloadJson);
const payloadJsState = readTextState(args.payloadJs);
const sessionState = readJsonState(args.captureSession);
const gapState = readJsonState(args.gapReport);
const hostState = readJsonState(args.bridgeHostPreflight);
const runtimeState = readJsonState(args.bridgeRuntimePreflight);
const checks = [];

const payload = payloadState.data || {};
const session = sessionState.data || {};
const gap = gapState.data || {};
const host = hostState.data || {};
const runtime = runtimeState.data || {};
const sessionSummary = session.summary || {};
const gapSummary = gap.summary || {};
const visualTargets = countLiteralArrayItems(payloadJsState.text, 'VISUAL_TARGETS', 'target_id');
const interactionTargets = countLiteralArrayItems(payloadJsState.text, 'INTERACTION_TARGETS', 'route_id');
const payloadSyntax = parsesAsAsyncFunction(payloadJsState.text);

addCheck(checks, 'payload summary JSON is valid', payloadState.exists && payloadState.valid, payloadState.error || 'valid', repoRel(args.payloadJson));
addCheck(checks, 'payload JS exists', payloadJsState.exists && !payloadJsState.error, payloadJsState.error || 'present', repoRel(args.payloadJs));
addCheck(checks, 'capture session is valid', sessionState.exists && sessionState.valid, sessionState.error || 'valid', repoRel(args.captureSession));
addCheck(checks, 'gap report is valid', gapState.exists && gapState.valid, gapState.error || 'valid', repoRel(args.gapReport));
addCheck(checks, 'Windows host preflight is pass', hostState.valid && host.result === 'pass', host.result || hostState.error || 'missing', repoRel(args.bridgeHostPreflight));
addCheck(checks, 'Windows runtime preflight is pass', runtimeState.valid && runtime.result === 'pass', runtime.result || runtimeState.error || 'missing', repoRel(args.bridgeRuntimePreflight));
addCheck(checks, 'payload uses Windows LongShine Chrome client', String(payload.chrome_client_url || '').includes('C:/Users/LongShine/.codex/plugins/cache/openai-bundled/chrome/'), payload.chrome_client_url || 'missing');
addCheck(checks, 'payload JS parses as async Desktop Node REPL code', payloadSyntax.ok, payloadSyntax.error || 'parse ok');
addCheck(checks, 'payload target URLs have no unresolved smoke redirect base URL marker', !/"url_template":\s*"[^"]*<SMOKE_REDIRECT_BASE_URL>/.test(payloadJsState.text), '<SMOKE_REDIRECT_BASE_URL> absent from url_template values');
addCheck(checks, 'payload uses Windows localhost tunnel URLs instead of direct APISIX for capture', payloadJsState.text.includes('http://127.0.0.1:25173') && payloadJsState.text.includes('http://127.0.0.1:25174') && payloadJsState.text.includes('http://127.0.0.1:25175') && !payloadJsState.text.includes('http://10.0.5.8:30180'), 'requires 25173/25174/25175 and rejects 10.0.5.8:30180 in payload JS');
addCheck(checks, 'payload requires Chrome extension backend', payload.backend_required === 'codex-desktop-chrome-extension' && payloadJsState.text.includes("agent.browsers.get('extension')"), `backend=${payload.backend_required || 'missing'}`);
addCheck(checks, 'payload forbids iab', payload.forbidden_backend === 'iab' && !payloadJsState.text.includes("agent.browsers.get('iab')"), `forbidden=${payload.forbidden_backend || 'missing'}`);
addCheck(checks, 'payload contains placeholders only', payload.placeholders?.CODEX_CAPTURE_KEY === '<CODEX_CAPTURE_KEY>' && payload.placeholders?.CODEX_SMOKE_NONCE === '<CODEX_SMOKE_NONCE>', JSON.stringify(payload.placeholders || {}));
const jwtPrefixPattern = ['e', 'y', 'J'].join('');
addCheck(checks, 'payload JS has no JWT or Bearer material', !(new RegExp(`${jwtPrefixPattern}|Bearer\\\\s+`)).test(payloadJsState.text), 'secret scan pattern absent');
addCheck(checks, 'payload uploads bridge result summary', payload.bridge_result_upload_count === 1 && payloadJsState.text.includes('BRIDGE_RESULT_UPLOAD') && payloadJsState.text.includes('postJson(BRIDGE_RESULT_UPLOAD, results)'), `bridge_result_upload_count=${payload.bridge_result_upload_count}`);
addCheck(checks, 'payload visual count matches current gap report', Number(payload.visual_target_count) === Number(gapSummary.visual_gap_count), `${payload.visual_target_count}/${gapSummary.visual_gap_count}`);
addCheck(checks, 'payload interaction count matches current gap report', Number(payload.interaction_target_count) === Number(gapSummary.interaction_gap_count), `${payload.interaction_target_count}/${gapSummary.interaction_gap_count}`);
addCheck(checks, 'payload counts match capture session', Number(payload.visual_target_count) === Number(sessionSummary.visual_batch_count) && Number(payload.interaction_target_count) === Number(sessionSummary.interaction_batch_count), `payload=${payload.visual_target_count}/${payload.interaction_target_count} session=${sessionSummary.visual_batch_count}/${sessionSummary.interaction_batch_count}`);
addCheck(checks, 'payload receiver upload count is complete', Number(payload.receiver_upload_count) === Number(payload.screenshot_upload_count) + Number(payload.bridge_result_upload_count), `${payload.receiver_upload_count}=${payload.screenshot_upload_count}+${payload.bridge_result_upload_count}`);
addCheck(checks, 'payload JS target literal counts match summary', visualTargets?.count === payload.visual_target_count && interactionTargets?.count === payload.interaction_target_count, `js=${visualTargets?.count ?? 'missing'}/${interactionTargets?.count ?? 'missing'} summary=${payload.visual_target_count}/${payload.interaction_target_count}`);
addCheck(checks, 'payload covers all interaction gap route ids', Array.isArray(gap.interaction_gaps) && interactionTargets?.ids && gap.interaction_gaps.every((item) => interactionTargets.ids.includes(item.route_id)), `covered=${interactionTargets?.ids?.length ?? 'missing'} gaps=${Array.isArray(gap.interaction_gaps) ? gap.interaction_gaps.length : 'missing'}`);

const passed = checks.filter((check) => check.passed).length;
const summary = {
  package_id: 'ui_desktop_chrome_bridge_payload_selftest',
  result: passed === checks.length ? 'pass' : 'fail',
  generated_at: new Date().toISOString(),
  payload_json: {
    path: repoRel(args.payloadJson),
    exists: payloadState.exists,
    valid: payloadState.valid,
  },
  payload_js: {
    path: repoRel(args.payloadJs),
    exists: payloadJsState.exists,
  },
  counts: {
    visual_targets: payload.visual_target_count ?? null,
    interaction_targets: payload.interaction_target_count ?? null,
    screenshot_uploads: payload.screenshot_upload_count ?? null,
    bridge_result_uploads: payload.bridge_result_upload_count ?? null,
    receiver_uploads: payload.receiver_upload_count ?? null,
  },
  checks,
  passed,
  total: checks.length,
};

ensureDirFor(args.outputJson);
fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
ensureDirFor(args.outputMd);
fs.writeFileSync(resolveRepo(args.outputMd), renderMarkdown(summary), 'utf8');

console.log(`ui-desktop-chrome-bridge-payload-selftest result=${summary.result} passed=${passed}/${checks.length} json=${repoRel(args.outputJson)} md=${repoRel(args.outputMd)}`);
if (summary.result !== 'pass') process.exit(1);

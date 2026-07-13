#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const defaults = {
  baseUrl: 'http://10.0.5.8:30180',
  receiverUrl: 'http://10.0.5.8:15174',
  visualAcceptance: 'doc/04_assets/ui_suite_gpt_v1/specs/visual-acceptance.json',
  routePageMap: 'doc/04_assets/ui_suite_gpt_v1/specs/route-page-map.json',
  evidenceDir: 'doc/02_acceptance/02-regression/ui-visual-interaction/latest',
  templateDir: 'doc/02_acceptance/02-regression/ui-visual-interaction/templates',
  gapReport: 'doc/02_acceptance/02-regression/ui-visual-interaction-gap-report-latest.json',
  viewportProbe: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-viewport-probe-latest.json',
  receiverSelftest: 'doc/02_acceptance/02-regression/ui-visual-interaction/receiver-selftest-latest.json',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-latest.md',
  alertId: process.env.UI_VISUAL_ALERT_ID || 'alert-default-1782752318016-1dd589c4',
  campaignId: process.env.UI_VISUAL_CAMPAIGN_ID || 'campaign-exfil-default-1782729598739-e1d2dc37',
  notFoundPath: '/__codex_visual_not_found__',
  smokeTokenParam: 'codex_smoke_token',
  smokeTokenPlaceholder: '<DESKTOP_SMOKE_TOKEN>',
  smokeRedirectBaseUrl: 'http://10.0.5.8:15175',
  receiverPort: '15174',
  redirectPort: '15175',
  smokeNoncePlaceholder: '<CODEX_SMOKE_NONCE>',
  waitMs: 3500,
  maxPixelRatio: 0.015,
};

const args = parseArgs(process.argv.slice(2));
const config = { ...defaults, ...args };
const root = process.cwd();

function parseArgs(argv) {
  const parsed = {};
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index];
    if (!item.startsWith('--')) {
      throw new Error(`unexpected argument: ${item}`);
    }
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
  return JSON.parse(fs.readFileSync(resolveRepo(file), 'utf8'));
}

function maybeReadJson(file) {
  const full = resolveRepo(file);
  if (!fs.existsSync(full)) return { exists: false, valid: false, data: null };
  try {
    return { exists: true, valid: true, data: JSON.parse(fs.readFileSync(full, 'utf8')) };
  } catch (error) {
    return { exists: true, valid: false, error: error.message, data: null };
  }
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

function exists(file) {
  return fs.existsSync(resolveRepo(file));
}

function ensureDirFor(file) {
  fs.mkdirSync(path.dirname(resolveRepo(file)), { recursive: true });
}

function normalizeBaseUrl(value) {
  return String(value || '').replace(/\/+$/, '');
}

function normalizeReceiverUrl(value) {
  return String(value || '').replace(/\/+$/, '');
}

function visualStatesOf(route) {
  if (Array.isArray(route.visualStates) && route.visualStates.length > 0) {
    return route.visualStates.map((state) => ({
      id: state.id || `${route.id}-${state.title || 'state'}`,
      title: state.title || route.title,
      routeId: route.id,
      route: route.route,
      query: state.query || '',
      pageComponent: route.pageComponent,
      sourceImage: state.sourceImage || '',
    }));
  }
  if (route.sourceImage) {
    return [{
      id: route.id,
      title: route.title,
      routeId: route.id,
      route: route.route,
      query: '',
      pageComponent: route.pageComponent,
      sourceImage: route.sourceImage,
    }];
  }
  return [];
}

function resolveRoutePath(route) {
  if (route === '*') return config.notFoundPath;
  return route
    .replace(':alertId', config.alertId)
    .replace(':campaignId', config.campaignId);
}

function pathWithQuery(routePath, query) {
  if (!query) return routePath;
  const separator = routePath.includes('?') ? '&' : '';
  return `${routePath}${separator}${query.startsWith('?') ? query : `?${query}`}`;
}

function absoluteUrl(routePath, query = '') {
  const baseUrl = normalizeBaseUrl(config.baseUrl);
  const resolved = pathWithQuery(routePath, query);
  return new URL(resolved, `${baseUrl}/`).toString();
}

function requiresSmokeToken(routeId) {
  return authMode(routeId) !== 'public-login';
}

function authenticatedUrlPattern(url, routeId) {
  if (!requiresSmokeToken(routeId)) return url;
  const separator = url.includes('#') ? '&' : '#';
  return `${url}${separator}${config.smokeTokenParam}=${config.smokeTokenPlaceholder}`;
}

function smokeRedirectUrl(routePath, routeId, query = '') {
  if (!requiresSmokeToken(routeId)) return absoluteUrl(routePath, query);
  const base = String(config.smokeRedirectBaseUrl || '').replace(/\/+$/, '');
  const routeParam = encodeURIComponent(routePath);
  const queryParam = query ? `&query=${encodeURIComponent(query.replace(/^\?/, ''))}` : '';
  return `${base}/start?nonce=${encodeURIComponent(config.smokeNoncePlaceholder)}&route=${routeParam}${queryParam}`;
}

function pathFromUrl(value) {
  try {
    return new URL(value).pathname;
  } catch {
    return '';
  }
}

function evidenceFile(targetId, name) {
  return path.join(config.evidenceDir, targetId, name).split(path.sep).join('/');
}

function interactionTemplateFile(routeId) {
  return path.join(config.templateDir, routeId, 'interaction.template.json').split(path.sep).join('/');
}

function receiverEndpoint(kind, targetId) {
  const receiverUrl = normalizeReceiverUrl(config.receiverUrl);
  if (!receiverUrl) return '';
  return `${receiverUrl}/${kind}/${targetId}`;
}

function metricStatus(file) {
  const metrics = maybeReadJson(file);
  if (!metrics.exists) return { exists: false, valid: false, passed: false, reasons: ['missing metrics.json'] };
  if (!metrics.valid) return { exists: true, valid: false, passed: false, reasons: [`invalid metrics.json: ${metrics.error}`] };

  const data = metrics.data;
  const status = String(data.status || data.result || '');
  const ratio = Number(
    data.visual_diff?.pixel_mismatch_ratio ??
      data.visualDiff?.pixelMismatchRatio ??
      data.pixel_mismatch_ratio ??
      data.mismatch_ratio,
  );
  const viewport = data.viewport || {};
  const width = Number(viewport.width ?? data.width);
  const height = Number(viewport.height ?? data.height);
  const reasons = [];
  if (!['pass', 'passed', 'ok'].includes(status.toLowerCase())) reasons.push(`metrics status=${status || 'missing'}`);
  if (!Number.isFinite(ratio)) reasons.push('missing pixel mismatch ratio');
  if (Number.isFinite(ratio) && ratio > Number(config.maxPixelRatio)) reasons.push(`pixel mismatch ratio ${ratio} > ${config.maxPixelRatio}`);
  if (width !== 1920 || height !== 1080) reasons.push(`viewport ${width || 'missing'}x${height || 'missing'} != 1920x1080`);
  return {
    exists: true,
    valid: true,
    passed: reasons.length === 0,
    status,
    pixelMismatchRatio: Number.isFinite(ratio) ? ratio : null,
    viewport: { width: Number.isFinite(width) ? width : null, height: Number.isFinite(height) ? height : null },
    reasons,
  };
}

function captureMetaStatus(file) {
  const meta = maybeReadJson(file);
  if (!meta.exists) return { exists: false, valid: false, passed: false, reasons: ['missing capture-meta.json'] };
  if (!meta.valid) return { exists: true, valid: false, passed: false, reasons: [`invalid capture-meta.json: ${meta.error}`] };

  const data = meta.data;
  const status = String(data.status || data.result || '');
  const uploaded = data.uploaded_size || data.uploadedSize || {};
  const stored = data.stored_size || data.storedSize || {};
  const desktopViewport = data.desktop_viewport || data.desktopViewport || {};
  const uploadedWidth = Number(uploaded.width);
  const uploadedHeight = Number(uploaded.height);
  const storedWidth = Number(stored.width);
  const storedHeight = Number(stored.height);
  const desktopViewportWidth = Number(desktopViewport.width);
  const desktopViewportHeight = Number(desktopViewport.height);
  const reasons = [];
  if (!['pass', 'passed', 'ok'].includes(status.toLowerCase())) reasons.push(`capture-meta status=${status || 'missing'}`);
  if (data.backend !== 'codex-desktop-chrome-extension') reasons.push(`capture-meta backend=${data.backend || 'missing'}`);
  if (data.post_capture_resize !== false) reasons.push('capture-meta post_capture_resize is not false');
  if (uploadedWidth !== 1920 || uploadedHeight !== 1080) {
    reasons.push(`uploaded screenshot ${uploadedWidth || 'missing'}x${uploadedHeight || 'missing'} != 1920x1080`);
  }
  if (storedWidth !== 1920 || storedHeight !== 1080) {
    reasons.push(`stored screenshot ${storedWidth || 'missing'}x${storedHeight || 'missing'} != 1920x1080`);
  }
  if (desktopViewportWidth !== 1920 || desktopViewportHeight !== 1080) {
    reasons.push(`Desktop Chrome viewport ${desktopViewportWidth || 'missing'}x${desktopViewportHeight || 'missing'} != 1920x1080`);
  }
  return {
    exists: true,
    valid: true,
    passed: reasons.length === 0,
    status,
    uploadedSize: Number.isFinite(uploadedWidth) && Number.isFinite(uploadedHeight)
      ? { width: uploadedWidth, height: uploadedHeight }
      : null,
    storedSize: Number.isFinite(storedWidth) && Number.isFinite(storedHeight)
      ? { width: storedWidth, height: storedHeight }
      : null,
    desktopViewportSize: Number.isFinite(desktopViewportWidth) && Number.isFinite(desktopViewportHeight)
      ? { width: desktopViewportWidth, height: desktopViewportHeight }
      : null,
    reasons,
  };
}

function assertionIsTrue(data, key) {
  return data[key] === true || data.assertions?.[key] === true;
}

function interactionStatus(file, route) {
  const interaction = maybeReadJson(file);
  if (!interaction.exists) return { exists: false, valid: false, passed: false, reasons: ['missing interaction.json'] };
  if (!interaction.valid) return { exists: true, valid: false, passed: false, reasons: [`invalid interaction.json: ${interaction.error}`] };

  const data = interaction.data;
  const status = String(data.status || data.result || '');
  const reasons = [];
  if (!['pass', 'passed', 'ok'].includes(status.toLowerCase())) reasons.push(`interaction status=${status || 'missing'}`);
  if (data.route_id && data.route_id !== route.id) reasons.push(`route_id ${data.route_id} != ${route.id}`);
  if (data.route && data.route !== route.route) reasons.push(`route ${data.route} != ${route.route}`);
  for (const key of ['no_4xx_5xx', 'no_requestfailed', 'no_pageerror', 'no_console_error']) {
    if (data[key] !== true) reasons.push(`${key} is not true`);
  }
  if (!data.business_action || String(data.business_action).trim().length < 3) reasons.push('missing business_action');
  const backend = data.desktop_chrome_backend_status || data.desktop_chrome_status || '';
  if (!['pass', 'passed', 'ok'].includes(String(backend).toLowerCase())) reasons.push('missing passing Desktop Chrome backend status');
  const screenshot = data.target_screenshot || data.screenshot || data.actual_screenshot || '';
  if (!screenshot || !exists(screenshot)) reasons.push('missing target screenshot');
  const finalUrl = String(data.final_url || data.url || '');
  const finalPath = pathFromUrl(finalUrl);
  if (!finalUrl) reasons.push('missing final_url');
  if (finalPath && finalPath !== route.resolvedPath) reasons.push(`final path ${finalPath} != ${route.resolvedPath}`);
  if (requiresSmokeToken(route.id)) {
    if (finalPath === '/login') reasons.push('protected route resolved to /login');
    if (finalUrl.includes(config.smokeTokenParam) || finalUrl.includes(config.smokeTokenPlaceholder)) {
      reasons.push('smoke token remains in final_url');
    }
    if (!assertionIsTrue(data, 'smoke_hash_consumed')) reasons.push('missing smoke_hash_consumed assertion');
    if (!assertionIsTrue(data, 'not_login_shell')) reasons.push('missing not_login_shell assertion');
  } else if (route.id === 'login' && data.assertions && data.assertions.smoke_hash_absent !== true) {
    reasons.push('login evidence does not assert smoke_hash_absent');
  }
  return {
    exists: true,
    valid: true,
    passed: reasons.length === 0,
    status,
    backendStatus: backend,
    finalUrl,
    finalPath,
    targetScreenshot: screenshot,
    reasons,
  };
}

function authMode(routeId) {
  if (routeId === 'login') return 'public-login';
  if (routeId === 'screen') return 'authenticated-or-screen-masked-demo';
  return 'authenticated-or-controlled-smoke-token';
}

function routeBusinessHint(route) {
  const endpointText = Array.isArray(route.apiEndpoints) && route.apiEndpoints.length > 0
    ? `verify live data from ${route.apiEndpoints.join(', ')}`
    : 'verify visible page-specific content';
  if (route.id === 'login') return 'render login form and captcha challenge without submitting credentials';
  if (route.id === 'not-found') return 'render not-found recovery action and return navigation affordance';
  return `${route.title}: ${endpointText}; perform one route-specific read or safe UI action`;
}

function routeInteractionRequirements(route) {
  if (route.id === 'login') {
    return {
      expected_final_path: route.resolvedPath,
      required_assertions: ['product_name_visible', 'account_password_tab_visible', 'captcha_visible', 'submit_visible', 'smoke_hash_absent'],
      forbidden_final_paths: [],
    };
  }
  const requiredAssertions = ['smoke_hash_consumed', 'not_login_shell', 'access_denied_absent'];
  return {
    expected_final_path: route.resolvedPath,
    required_assertions: requiredAssertions,
    forbidden_final_paths: ['/login'],
    expected_text_markers: [route.title],
    api_endpoints: route.apiEndpoints || [],
  };
}

function writeInteractionTemplate(route, routeUrl, safeRedirectUrl) {
  const file = interactionTemplateFile(route.id);
  ensureDirFor(file);
  const protectedRoute = requiresSmokeToken(route.id);
  const template = {
    template_type: 'ui_visual_interaction_route_evidence',
    template_version: 1,
    route_id: route.id,
    title: route.title,
    route: route.route,
    expected_final_path: route.resolvedPath,
    url: routeUrl,
    authenticated_url_pattern: authenticatedUrlPattern(routeUrl, route.id),
    safe_redirect_url_pattern: safeRedirectUrl,
    safe_wrapper_call: {
      tool: 'mcp__codex_desktop_node_repl.desktop_chrome_open_url',
      args: { url: safeRedirectUrl, keep: true, wait_ms: Number(config.waitMs) },
    },
    output_path: evidenceFile(route.id, 'interaction.json'),
    required_screenshot_path: evidenceFile(route.id, 'interaction.png'),
    interaction_screenshot_upload: receiverEndpoint('interaction-screenshot', route.id),
    business_action_hint: routeBusinessHint(route),
    api_endpoints: route.apiEndpoints || [],
    acceptance_requirements: routeInteractionRequirements(route),
    interaction_json_skeleton: {
      status: 'pass',
      route_id: route.id,
      route: route.route,
      final_url: routeUrl,
      business_action: routeBusinessHint(route),
      desktop_chrome_backend_status: 'pass',
      no_4xx_5xx: true,
      no_requestfailed: true,
      no_pageerror: true,
      no_console_error: true,
      target_screenshot: evidenceFile(route.id, 'interaction.png'),
      assertions: protectedRoute
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
  fs.writeFileSync(resolveRepo(file), `${JSON.stringify(template, null, 2)}\n`, 'utf8');
  return file;
}

const visualAcceptance = readJson(config.visualAcceptance);
const routePageMap = readJson(config.routePageMap);
const routes = Array.isArray(visualAcceptance.routes) ? visualAcceptance.routes : [];
const routeMapIds = new Set(Array.isArray(routePageMap) ? routePageMap.map((item) => item.id) : []);
const baseUrl = normalizeBaseUrl(config.baseUrl);

const visualTargets = [];
const interactions = [];

for (const route of routes) {
  const resolvedPath = resolveRoutePath(route.route);
  const routeWithResolvedPath = { ...route, resolvedPath };
  const interactionPath = evidenceFile(route.id, 'interaction.json');
  const interaction = interactionStatus(interactionPath, routeWithResolvedPath);
  const routeUrl = absoluteUrl(resolvedPath);
  const safeRedirectUrl = smokeRedirectUrl(resolvedPath, route.id);
  const interactionTemplate = writeInteractionTemplate(routeWithResolvedPath, routeUrl, safeRedirectUrl);

  interactions.push({
    route_id: route.id,
    title: route.title,
    route: route.route,
    resolved_path: resolvedPath,
    url: routeUrl,
    page_component: route.pageComponent,
    auth_mode: authMode(route.id),
    requires_smoke_token: requiresSmokeToken(route.id),
    authenticated_url_pattern: authenticatedUrlPattern(routeUrl, route.id),
    safe_redirect_url_pattern: safeRedirectUrl,
    api_endpoints: route.apiEndpoints || [],
    business_action_hint: routeBusinessHint(route),
    interaction_requirements: routeInteractionRequirements(routeWithResolvedPath),
    wrapper_call: {
      tool: 'mcp__codex_desktop_node_repl.desktop_chrome_open_url',
      args: { url: authenticatedUrlPattern(routeUrl, route.id), keep: true, wait_ms: Number(config.waitMs) },
    },
    safe_wrapper_call: {
      tool: 'mcp__codex_desktop_node_repl.desktop_chrome_open_url',
      args: { url: safeRedirectUrl, keep: true, wait_ms: Number(config.waitMs) },
    },
    receiver_upload: receiverEndpoint('interaction', route.id),
    interaction_screenshot_upload: receiverEndpoint('interaction-screenshot', route.id),
    interaction_template: interactionTemplate,
    evidence: {
      interaction: interactionPath,
      exists: interaction.exists,
      valid: interaction.valid,
      passed: interaction.passed,
      reasons: interaction.reasons,
    },
  });

  for (const target of visualStatesOf(route)) {
    const targetPath = resolveRoutePath(target.route);
    const url = absoluteUrl(targetPath, target.query);
    const actual = evidenceFile(target.id, 'actual-1920.png');
    const diff = evidenceFile(target.id, 'diff-1920.png');
    const metrics = evidenceFile(target.id, 'metrics.json');
    const captureMeta = evidenceFile(target.id, 'capture-meta.json');
    const metric = metricStatus(metrics);
    const capture = captureMetaStatus(captureMeta);
    const targetReasons = [];
    if (!exists(actual)) targetReasons.push('missing actual-1920.png');
    if (!exists(diff)) targetReasons.push('missing diff-1920.png');
    if (!metric.passed) targetReasons.push(...metric.reasons);
    if (!capture.passed) targetReasons.push(...capture.reasons);
    visualTargets.push({
      target_id: target.id,
      route_id: target.routeId,
      title: target.title,
      route: target.route,
      resolved_path: targetPath,
      query: target.query,
      url,
      page_component: target.pageComponent,
      auth_mode: authMode(target.routeId),
      requires_smoke_token: requiresSmokeToken(target.routeId),
      authenticated_url_pattern: authenticatedUrlPattern(url, target.routeId),
      source_image: target.sourceImage,
      wrapper_call: {
        tool: 'mcp__codex_desktop_node_repl.desktop_chrome_open_url',
        args: { url: authenticatedUrlPattern(url, target.routeId), keep: true, wait_ms: Number(config.waitMs) },
      },
      safe_wrapper_call: {
        tool: 'mcp__codex_desktop_node_repl.desktop_chrome_open_url',
        args: { url: smokeRedirectUrl(targetPath, target.routeId, target.query), keep: true, wait_ms: Number(config.waitMs) },
      },
      receiver_upload: receiverEndpoint('upload', target.id),
      metrics_command: [
        'tests/e2e/ui_visual_diff_metrics.py',
        '--target-id', target.id,
        '--route', pathWithQuery(targetPath, target.query),
        '--source', target.sourceImage,
        '--actual', actual,
        '--diff', diff,
        '--metrics', metrics,
      ],
      evidence: {
        actual,
        diff,
        metrics,
        capture_meta: captureMeta,
        actual_exists: exists(actual),
        diff_exists: exists(diff),
        metrics_exists: metric.exists,
        metrics_valid: metric.valid,
        metrics_passed: metric.passed,
        capture_meta_exists: capture.exists,
        capture_meta_valid: capture.valid,
        capture_meta_passed: capture.passed,
        capture_uploaded_size: capture.uploadedSize ?? null,
        capture_stored_size: capture.storedSize ?? null,
        capture_desktop_viewport_size: capture.desktopViewportSize ?? null,
        pixel_mismatch_ratio: metric.pixelMismatchRatio ?? null,
        viewport: metric.viewport ?? null,
        passed: exists(actual) && exists(diff) && metric.passed && capture.passed,
        reasons: targetReasons,
      },
    });
  }
}

const missingVisualTargets = visualTargets.filter((target) => !target.evidence.passed);
const missingInteractions = interactions.filter((item) => !item.evidence.passed);
const captureScreenshotCount = visualTargets.length + interactions.length;
const redirectOpenCount = visualTargets.filter((target) => target.requires_smoke_token).length
  + interactions.filter((item) => item.requires_smoke_token).length;
const viewportProbeState = maybeReadJson(config.viewportProbe);
const viewportProbe = normalizeViewportProbe(viewportProbeState);
const receiverSelftest = maybeReadJson(config.receiverSelftest);
const gapReport = maybeReadJson(config.gapReport);
const plan = {
  package_id: 'ui_desktop_capture_plan',
  generated_at: new Date().toISOString(),
  base_url: baseUrl,
  receiver_url: normalizeReceiverUrl(config.receiverUrl) || null,
  smoke_redirect_base_url: normalizeBaseUrl(config.smokeRedirectBaseUrl),
  capture_key_header: 'X-Codex-Capture-Key',
  token_endpoint: normalizeReceiverUrl(config.receiverUrl) ? `${normalizeReceiverUrl(config.receiverUrl)}/token` : null,
  viewport_probe_url: normalizeReceiverUrl(config.receiverUrl) ? `${normalizeReceiverUrl(config.receiverUrl)}/viewport-probe` : null,
  visual_acceptance: repoRel(config.visualAcceptance),
  route_page_map: repoRel(config.routePageMap),
  evidence_dir: repoRel(config.evidenceDir),
  template_dir: repoRel(config.templateDir),
  gap_report: {
    path: repoRel(config.gapReport),
    exists: gapReport.exists,
    valid: gapReport.valid,
    run_id: gapReport.data?.run_id || null,
    summary: gapReport.data?.summary || null,
    reason_groups: gapReport.data?.reason_groups || null,
  },
  viewport_probe: {
    path: repoRel(config.viewportProbe),
    ...viewportProbe,
  },
  receiver_selftest: {
    path: repoRel(config.receiverSelftest),
    exists: receiverSelftest.exists,
    valid: receiverSelftest.valid,
    result: receiverSelftest.data?.result || null,
    passed: receiverSelftest.data?.passed ?? null,
    total: receiverSelftest.data?.total ?? null,
    acceptance_effect: receiverSelftest.data?.acceptance_effect || null,
  },
  expected_viewport: visualAcceptance.global?.imageSize || { width: 1920, height: 1080 },
  desktop_smoke_prerequisites: {
    DESKTOP_SMOKE_TOKEN_ENABLED: true,
    DESKTOP_SMOKE_TOKEN: '<redacted>',
    CODEX_CAPTURE_KEY: '<redacted>',
    protected_route_url_pattern: `${baseUrl}/<route>#${config.smokeTokenParam}=${config.smokeTokenPlaceholder}`,
    safe_wrapper_url_pattern: `${String(config.smokeRedirectBaseUrl).replace(/\/+$/, '')}/start?nonce=${config.smokeNoncePlaceholder}&route=<encoded-route>`,
    verify_hash_consumed: true,
  },
  auth_capture_strategy: 'Open protected routes through the nonce-only smoke redirect helper when possible, then verify the hash is consumed, the final page path remains the requested route, and no smoke token remains in the final URL. Directly opening protected route URLs without auth is expected to redirect to /login and must not be accepted as route evidence.',
  route_map_aligned: routes.every((route) => routeMapIds.has(route.id)),
  dynamic_route_params: {
    alertId: config.alertId,
    campaignId: config.campaignId,
    notFoundPath: config.notFoundPath,
  },
  summary: {
    route_count: routes.length,
    visual_target_count: visualTargets.length,
    visual_passed_count: visualTargets.length - missingVisualTargets.length,
    visual_missing_or_failing_count: missingVisualTargets.length,
    interaction_count: interactions.length,
    interaction_passed_count: interactions.length - missingInteractions.length,
    interaction_missing_or_failing_count: missingInteractions.length,
  },
  usage: {
    receiver_selftest: 'python3 tests/e2e/ui_desktop_capture_receiver_selftest.py',
    receiver_start: [
      'DESKTOP_SMOKE_TOKEN=<redacted>',
      'CODEX_CAPTURE_KEY=<redacted>',
      'tests/e2e/ui_desktop_capture_receiver.py',
      '--host 0.0.0.0',
      `--port ${config.receiverPort}`,
      `--evidence-dir ${repoRel(config.evidenceDir)}`,
      `--max-uploads ${Math.max(captureScreenshotCount, 1)}`,
      '--expected-width 1920',
      '--expected-height 1080',
    ].join(' '),
    smoke_redirect_start: [
      'DESKTOP_SMOKE_TOKEN=<redacted>',
      'CODEX_SMOKE_NONCE=<redacted>',
      'tests/e2e/ui_desktop_smoke_redirect.py',
      '--host 0.0.0.0',
      `--port ${config.redirectPort}`,
      `--app-base-url ${baseUrl}`,
      '--default-route /dashboard',
      `--max-redirects ${Math.max(redirectOpenCount, 1)}`,
    ].join(' '),
    viewport_probe_open: normalizeReceiverUrl(config.receiverUrl)
      ? `mcp__codex_desktop_node_repl.desktop_chrome_open_url url=${normalizeReceiverUrl(config.receiverUrl)}/viewport-probe keep=true wait_ms=1500`
      : 'start receiver with --receiver-url, then open <receiver-url>/viewport-probe using mcp__codex_desktop_node_repl.desktop_chrome_open_url',
    preflight: 'DESKTOP_CHROME_STATUS=pass ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh',
    note: 'The plan lists wrapper calls and upload endpoints only. It does not prove visual or interaction acceptance until real Desktop Chrome screenshots, capture metadata, diff metrics, and interaction.json files pass the dual gate.',
  },
  visual_targets: visualTargets,
  interactions,
  next_visual_targets: missingVisualTargets.map((target) => target.target_id),
  next_interaction_routes: missingInteractions.map((item) => item.route_id),
};

ensureDirFor(config.outputJson);
fs.writeFileSync(resolveRepo(config.outputJson), `${JSON.stringify(plan, null, 2)}\n`, 'utf8');

ensureDirFor(config.outputMd);
fs.writeFileSync(resolveRepo(config.outputMd), renderMarkdown(plan), 'utf8');

console.log(`ui-desktop-capture-plan visual=${plan.summary.visual_passed_count}/${plan.summary.visual_target_count} interactions=${plan.summary.interaction_passed_count}/${plan.summary.interaction_count} json=${repoRel(config.outputJson)} md=${repoRel(config.outputMd)}`);

function renderMarkdown(planData) {
  const lines = [];
  lines.push('# UI Desktop Capture Plan');
  lines.push('');
  lines.push(`- Generated: \`${planData.generated_at}\``);
  lines.push(`- Base URL: \`${planData.base_url}\``);
  lines.push(`- Receiver URL: \`${planData.receiver_url || '<not provided>'}\``);
  lines.push(`- Viewport probe URL: \`${planData.viewport_probe_url || '<not provided>'}\``);
  lines.push(`- Visual evidence: \`${planData.summary.visual_passed_count}/${planData.summary.visual_target_count}\``);
  lines.push(`- Interaction evidence: \`${planData.summary.interaction_passed_count}/${planData.summary.interaction_count}\``);
  lines.push('');
  lines.push('This is a capture work queue, not acceptance evidence. The dual gate only passes after real Desktop Chrome screenshots, receiver capture metadata, metrics, and interaction JSON files pass.');
  lines.push('');
  lines.push('## Auth Capture Strategy');
  lines.push('');
  lines.push('Protected routes must be opened with a short-lived hash smoke token, for example:');
  lines.push('');
  lines.push('```text');
  lines.push(`${planData.base_url}/dashboard#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>`);
  lines.push('```');
  lines.push('');
  lines.push('The hash must be consumed by the app before evidence is accepted, and the final path must still be the intended route. A protected route that redirects to `/login` is a failed capture, not valid route evidence.');
  lines.push('');
  lines.push('To avoid putting the token into the Desktop Chrome wrapper input, prefer the nonce-only redirect helper:');
  lines.push('');
  lines.push('```bash');
  lines.push(planData.usage.smoke_redirect_start);
  lines.push('```');
  lines.push('');
  lines.push('Then open the route-specific `safe_redirect_url_pattern` with `desktop_chrome_open_url`; the helper redirects Chrome to the token hash URL exactly once.');
  lines.push('');
  lines.push('Before capturing screenshots, open the receiver viewport probe with the Desktop Chrome wrapper and confirm it reports `1920x1080`:');
  lines.push('');
  lines.push('```text');
  lines.push(planData.usage.viewport_probe_open);
  lines.push('```');
  lines.push('');
  lines.push('Before starting a capture session, run the receiver endpoint self-test:');
  lines.push('');
  lines.push('```bash');
  lines.push(planData.usage.receiver_selftest);
  lines.push('```');
  if (planData.receiver_selftest.exists) {
    const selftest = planData.receiver_selftest;
    lines.push('');
    lines.push('## Receiver Self-test');
    lines.push('');
    lines.push(`- Self-test: \`${selftest.path}\``);
    lines.push(`- Result: \`${selftest.result || 'unknown'}\``);
    lines.push(`- Checks: \`${selftest.passed ?? 'unknown'}/${selftest.total ?? 'unknown'}\``);
    lines.push(`- Acceptance effect: ${selftest.acceptance_effect || 'not recorded'}`);
  }
  if (planData.viewport_probe.exists) {
    const probe = planData.viewport_probe;
    const size = probe.viewport?.width && probe.viewport?.height
      ? `${probe.viewport.width}x${probe.viewport.height}`
      : 'unknown';
    lines.push('');
    lines.push('## Desktop Viewport Probe');
    lines.push('');
    lines.push(`- Probe: \`${probe.path}\``);
    lines.push(`- Result: \`${probe.result || 'unknown'}\``);
    lines.push(`- Current screenshot size: \`${size}\``);
    lines.push(`- Acceptance effect: ${probe.acceptance_effect || 'not recorded'}`);
  }
  if (planData.gap_report.exists) {
    lines.push('');
    lines.push('## Latest Gap Report');
    lines.push('');
    lines.push(`- Gap report: \`${planData.gap_report.path}\``);
    lines.push(`- Run ID: \`${planData.gap_report.run_id || 'unknown'}\``);
    const gapSummary = planData.gap_report.summary || {};
    lines.push(`- Visual gaps: \`${gapSummary.visual_gap_count ?? 'unknown'}/${gapSummary.visual_required_count ?? 'unknown'}\``);
    lines.push(`- Interaction gaps: \`${gapSummary.interaction_gap_count ?? 'unknown'}/${gapSummary.interaction_required_count ?? 'unknown'}\``);
  }
  lines.push('');
  lines.push('## Receiver');
  lines.push('');
  lines.push('```bash');
  lines.push(planData.usage.receiver_start);
  lines.push('```');
  lines.push('');
  lines.push('## Next Visual Targets');
  lines.push('');
  lines.push('| target | route | URL | missing or failing |');
  lines.push('|---|---|---|---|');
  for (const target of planData.visual_targets.filter((item) => !item.evidence.passed)) {
    lines.push(`| \`${target.target_id}\` | \`${target.route_id}\` | \`${target.authenticated_url_pattern}\` | ${target.evidence.reasons.join('; ')} |`);
  }
  lines.push('');
  lines.push('## Next Interaction Routes');
  lines.push('');
  lines.push('| route | URL | template | auth mode | required business action | missing or failing |');
  lines.push('|---|---|---|---|---|---|');
  for (const item of planData.interactions.filter((route) => !route.evidence.passed)) {
    lines.push(`| \`${item.route_id}\` | \`${item.safe_redirect_url_pattern}\` | \`${item.interaction_template}\` | \`${item.auth_mode}\` | ${item.business_action_hint} | ${item.evidence.reasons.join('; ')} |`);
  }
  lines.push('');
  lines.push('## Metrics Commands');
  lines.push('');
  lines.push('Run the matching command after each real screenshot upload. The upload must also create `capture-meta.json`; a cropped or resized file without receiver metadata remains blocked.');
  lines.push('');
  lines.push('```bash');
  for (const target of planData.visual_targets.filter((item) => !item.evidence.passed)) {
    lines.push(`${target.metrics_command.map(shellQuote).join(' ')} || true`);
  }
  lines.push('```');
  lines.push('');
  return `${lines.join('\n')}\n`;
}

function shellQuote(value) {
  const text = String(value);
  if (/^[A-Za-z0-9_./:=?&@%+-]+$/.test(text)) return text;
  return `'${text.replace(/'/g, "'\\''")}'`;
}

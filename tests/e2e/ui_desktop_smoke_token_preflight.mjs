#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';

const defaults = {
  baseUrl: 'http://127.0.0.1:5173',
  apisixUrl: 'http://10.0.5.8:30180',
  route: '/dashboard',
  expectedPath: '/dashboard',
  outputJson: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-smoke-token-preflight-latest.json',
  outputMd: 'doc/02_acceptance/02-regression/ui-visual-interaction/desktop-smoke-token-preflight-latest.md',
  tenant: 'default',
  kubectl: 'kubectl',
  jwtSecretNamespace: 'traffic-analysis',
  jwtSecretName: 'traffic-credentials',
  jwtSecretKey: 'JWT_SECRET',
  username: 'codex-ui-desktop-admin',
  waitMs: '2500',
  width: '1920',
  height: '1080',
};

const args = { ...defaults, ...parseArgs(process.argv.slice(2)) };
const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');

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

function noProxyEnv() {
  const next = { ...process.env };
  for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
    delete next[key];
  }
  return next;
}

function b64url(buffer) {
  return Buffer.from(buffer).toString('base64url');
}

function makeJwt(secret) {
  const now = Math.floor(Date.now() / 1000);
  const header = { alg: 'HS256', typ: 'JWT' };
  const claims = {
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: String(args.tenant),
    username: String(args.username),
    email: `${args.username}@local`,
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:read', 'graph:read', 'rule:read', 'model:read', 'token:read', 'screen:view'],
    token_type: 'access',
    session_id: `codex-desktop-smoke-${crypto.randomUUID()}`,
    iat: now,
    exp: now + 1800,
  };
  const signingInput = [
    b64url(JSON.stringify(header)),
    b64url(JSON.stringify(claims)),
  ].join('.');
  const signature = crypto.createHmac('sha256', secret).update(signingInput).digest();
  return `${signingInput}.${b64url(signature)}`;
}

function redactUrl(value) {
  return String(value || '').replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
}

function pass(name, passed, detail = '', artifact = '') {
  return { name, status: passed ? 'pass' : 'fail', passed, detail, artifact };
}

function runtimeValue(text, key) {
  const match = text.match(new RegExp(`${key}"?\\s*:\\s*"([^"]*)"`));
  return match ? match[1] : null;
}

async function readText(url) {
  const response = await fetch(url, { headers: { 'Cache-Control': 'no-cache' } });
  return { status: response.status, text: await response.text() };
}

async function main() {
  const checks = [];
  const baseUrl = String(args.baseUrl).replace(/\/+$/, '');
  const apisixUrl = String(args.apisixUrl).replace(/\/+$/, '');
  const runtimeUrl = `${baseUrl}/src/config/runtime.ts?codex_smoke_preflight=${Date.now()}`;

  const runtime = await readText(runtimeUrl);
  const authEnabled = runtimeValue(runtime.text, 'VITE_AUTH_ENABLED');
  const useMock = runtimeValue(runtime.text, 'VITE_USE_MOCK');
  const smokeEnabled = runtimeValue(runtime.text, 'VITE_DESKTOP_SMOKE_TOKEN_ENABLED');
  checks.push(pass('Vite auth enabled', runtime.status === 200 && authEnabled === 'true', `status=${runtime.status} VITE_AUTH_ENABLED=${authEnabled}`));
  checks.push(pass('Vite mock disabled', runtime.status === 200 && useMock === 'false', `status=${runtime.status} VITE_USE_MOCK=${useMock}`));
  checks.push(pass('Vite desktop smoke token enabled', runtime.status === 200 && smokeEnabled === 'true', `status=${runtime.status} VITE_DESKTOP_SMOKE_TOKEN_ENABLED=${smokeEnabled}`));

  const encodedSecret = execFileSync(
    String(args.kubectl),
    ['-n', String(args.jwtSecretNamespace), 'get', 'secret', String(args.jwtSecretName), '-o', `jsonpath={.data.${args.jwtSecretKey}}`],
    { encoding: 'utf8', env: noProxyEnv(), timeout: 15000 },
  );
  const jwtSecret = Buffer.from(encodedSecret, 'base64').toString('utf8');
  const token = makeJwt(jwtSecret);
  checks.push(pass('JWT generated from K8s secret', Boolean(token), `${args.jwtSecretNamespace}/${args.jwtSecretName}#${args.jwtSecretKey}`));

  const authMe = await fetch(`${apisixUrl}/api/v1/auth/me`, {
    headers: {
      Authorization: `Bearer ${token}`,
      'X-Tenant-ID': String(args.tenant),
    },
  });
  const authMeText = await authMe.text();
  let authMeJson = null;
  try {
    authMeJson = JSON.parse(authMeText);
  } catch {
    authMeJson = null;
  }
  checks.push(pass('short-lived JWT accepted by auth/me', authMe.status === 200, `status=${authMe.status} username=${authMeJson?.username || authMeJson?.user?.username || 'unknown'}`));

  const targetUrl = new URL(String(args.route), `${baseUrl}/`);
  targetUrl.hash = new URLSearchParams({ codex_smoke_token: token }).toString();

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: Number(args.width), height: Number(args.height) },
    deviceScaleFactor: 1,
  });
  const page = await context.newPage();
  const runtimeEvents = {
    console_errors: [],
    page_errors: [],
    request_failures: [],
    server_errors: [],
  };
  page.on('console', (message) => {
    if (message.type() === 'error') runtimeEvents.console_errors.push(message.text().slice(0, 500));
  });
  page.on('pageerror', (error) => runtimeEvents.page_errors.push(String(error.message || error).slice(0, 500)));
  page.on('requestfailed', (request) => runtimeEvents.request_failures.push(`${request.method()} ${redactUrl(request.url())} ${request.failure()?.errorText ?? ''}`.slice(0, 500)));
  page.on('response', (response) => {
    if (response.status() >= 400) runtimeEvents.server_errors.push({ status: response.status(), url: redactUrl(response.url()) });
  });

  await page.goto(targetUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForLoadState('networkidle', { timeout: Number(args.waitMs) }).catch(() => undefined);
  await page.waitForTimeout(Number(args.waitMs));
  const finalUrl = page.url();
  const finalPath = new URL(finalUrl).pathname;
  const bodyText = (await page.locator('body').innerText({ timeout: 10000 }).catch(() => '')).slice(0, 1000);
  const title = await page.title();
  await context.close();
  await browser.close();

  checks.push(pass('smoke hash consumed by app', !finalUrl.includes('codex_smoke_token'), `final_url=${redactUrl(finalUrl)}`));
  checks.push(pass('protected route stayed on expected path', finalPath === String(args.expectedPath), `final_path=${finalPath}`));
  checks.push(pass('protected route did not fall back to login', finalPath !== '/login' && !bodyText.includes('验证码'), `title=${title}`));
  checks.push(pass('no browser runtime errors', runtimeEvents.console_errors.length === 0 && runtimeEvents.page_errors.length === 0 && runtimeEvents.request_failures.length === 0 && runtimeEvents.server_errors.length === 0, JSON.stringify({
    console_errors: runtimeEvents.console_errors.length,
    page_errors: runtimeEvents.page_errors.length,
    request_failures: runtimeEvents.request_failures.length,
    server_errors: runtimeEvents.server_errors.length,
  })));

  const failed = checks.filter((check) => !check.passed);
  const summary = {
    package_id: 'ui_desktop_smoke_token_preflight',
    generated_at: new Date().toISOString(),
    result: failed.length === 0 ? 'pass' : 'fail',
    acceptance_eligible: false,
    reason: 'Local readiness check only. Formal acceptance still requires Windows Codex Desktop Chrome extension evidence.',
    base_url: baseUrl,
    apisix_url: apisixUrl,
    route: String(args.route),
    expected_path: String(args.expectedPath),
    final_url: redactUrl(finalUrl),
    final_path: finalPath,
    title,
    runtime: {
      VITE_AUTH_ENABLED: authEnabled,
      VITE_USE_MOCK: useMock,
      VITE_DESKTOP_SMOKE_TOKEN_ENABLED: smokeEnabled,
    },
    auth_me: {
      status: authMe.status,
      username: authMeJson?.username || authMeJson?.user?.username || null,
      tenant_id: authMeJson?.tenant_id || authMeJson?.tenantId || authMeJson?.user?.tenant_id || null,
    },
    runtime_events: runtimeEvents,
    checks,
  };

  ensureDirFor(args.outputJson);
  fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
  ensureDirFor(args.outputMd);
  fs.writeFileSync(resolveRepo(args.outputMd), renderMarkdown(summary), 'utf8');
  console.log(`ui-desktop-smoke-token-preflight result=${summary.result} checks=${checks.length - failed.length}/${checks.length} json=${repoRel(args.outputJson)} md=${repoRel(args.outputMd)}`);
  return failed.length === 0 ? 0 : 1;
}

function renderMarkdown(summary) {
  const lines = [
    '# Desktop Smoke Token Preflight',
    '',
    `- Result: \`${summary.result}\``,
    `- Generated: \`${summary.generated_at}\``,
    `- Base URL: \`${summary.base_url}\``,
    `- APISIX URL: \`${summary.apisix_url}\``,
    `- Route: \`${summary.route}\``,
    `- Final path: \`${summary.final_path}\``,
    `- Final URL: \`${summary.final_url}\``,
    '',
    'This is a local readiness check only. It proves that the current Vite target can consume a valid short-lived smoke JWT and keep a protected route authenticated. It does not replace Windows Codex Desktop Chrome extension visual or interaction evidence.',
    '',
    '## Checks',
    '',
  ];
  for (const check of summary.checks) {
    lines.push(`- \`${check.status}\` ${check.name}: ${check.detail}`);
  }
  return `${lines.join('\n')}\n`;
}

main().then((code) => {
  process.exitCode = code;
}).catch((error) => {
  const summary = {
    package_id: 'ui_desktop_smoke_token_preflight',
    generated_at: new Date().toISOString(),
    result: 'fail',
    acceptance_eligible: false,
    error: error.message,
  };
  ensureDirFor(args.outputJson);
  fs.writeFileSync(resolveRepo(args.outputJson), `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
  ensureDirFor(args.outputMd);
  fs.writeFileSync(resolveRepo(args.outputMd), `# Desktop Smoke Token Preflight\n\n- Result: \`fail\`\n- Error: ${error.message}\n`, 'utf8');
  console.error(`ui-desktop-smoke-token-preflight result=fail error=${error.message}`);
  process.exitCode = 1;
});

#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = 'http://10.0.5.8:30180';
const cdpUrl = 'http://127.0.0.1:9224';
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/notifications');
const outputPath = path.join(evidenceDir, 'interaction-r467.json');
const screenshotPaths = {
  main: path.join(evidenceDir, 'actual-r467-main-1920.png'),
  channel: path.join(evidenceDir, 'actual-r467-channel-drawer-1920.png'),
  template: path.join(evidenceDir, 'actual-r467-template-preview-1920.png'),
  silence: path.join(evidenceDir, 'actual-r467-silence-edit-1920.png'),
};

const cleanEnv = { ...process.env };
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete cleanEnv[key];
cleanEnv.NO_PROXY = '127.0.0.1,localhost,10.0.5.8,10.0.5.9';
cleanEnv.no_proxy = cleanEnv.NO_PROXY;
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = cleanEnv.NO_PROXY;
process.env.no_proxy = cleanEnv.NO_PROXY;
fs.mkdirSync(evidenceDir, { recursive: true });

const kubectl = (args, options = {}) => execFileSync('kubectl', args, { encoding: 'utf8', env: cleanEnv, timeout: 20_000, ...options }).trim();
const secret = (key) => Buffer.from(kubectl(['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', `jsonpath={.data.${key}}`]), 'base64').toString();
const jwtSecret = secret('JWT_SECRET');
const pgPassword = secret('PG_PASSWORD');
const now = Math.floor(Date.now() / 1_000);
const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
const claims = Buffer.from(JSON.stringify({
  iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(),
  tenant_id: 'default', username: 'codex-notification-windows-cdp-admin', roles: ['admin'],
  permissions: ['*', 'admin:*', 'audit:read'], token_type: 'access', iat: now, exp: now + 1_800,
})).toString('base64url');
const input = `${header}.${claims}`;
const adminToken = `${input}.${crypto.createHmac('sha256', jwtSecret).update(input).digest('base64url')}`;

const api = async (method, endpoint, body) => {
  const response = await fetch(`${baseUrl}/api${endpoint}`, {
    method,
    headers: { Authorization: `Bearer ${adminToken}`, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await response.text();
  let payload;
  try { payload = JSON.parse(text); } catch { payload = { raw: text }; }
  if (!response.ok) throw new Error(`${method} ${endpoint}: HTTP ${response.status} ${text.slice(0, 300)}`);
  return payload?.data;
};
const psql = (sql) => kubectl(['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'env', `PGPASSWORD=${pgPassword}`, 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-v', 'ON_ERROR_STOP=1', '-Atc', sql]);
const sqlText = (value) => `'${String(value ?? '').replaceAll("'", "''")}'`;
const redact = (value) => String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
const isExternalNoise = (url) => url.includes('api.yhchj.com/ip') || url.startsWith('chrome-extension://');

const original = await api('GET', '/v1/notifications/workbench?limit=100');
const originalById = {
  rules: new Map(original.rules.map((item) => [item.rule_id, item])),
  templates: new Map(original.templates.map((item) => [item.template_id, item])),
  policies: new Map(original.escalation_policies.map((item) => [item.policy_id, item])),
  silences: new Map(original.silence_rules.map((item) => [item.rule_id, item])),
};
const created = { templates: [], policies: [], silences: [], deliveries: [] };
const restored = [];
const dirty = { rules: new Set(), templates: new Set(), policies: new Set(), silences: new Set() };
let retriedOriginal;
let cleanupVerification;

const versionResponse = await fetch(`${cdpUrl}/json/version`);
const listResponse = await fetch(`${cdpUrl}/json/list`);
if (!versionResponse.ok || !listResponse.ok) throw new Error('Windows Chrome CDP 9224 preflight failed');
const version = await versionResponse.json();
const cdpTargets = await listResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const cdp = await context.newCDPSession(page);
await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
await cdp.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
page.setDefaultTimeout(15_000);

const badResponses = [];
const expectedBusinessResponses = [];
const consoleErrors = [];
const expectedBusinessConsoleErrors = [];
const pageErrors = [];
const requestFailures = [];
const mutationResponses = [];
const ignoredExternalFailures = [];
page.on('response', async (response) => {
  const url = response.url();
  if (response.status() >= 400) {
    const item = { status: response.status(), url: redact(url) };
    const expectedProviderFailure = response.status() === 503 && (
      /\/api\/v1\/notifications\/test(?:\?|$)/.test(url)
      || /\/api\/v1\/notifications\/templates\/[^/]+\/test(?:\?|$)/.test(url)
      || /\/api\/v1\/notifications\/deliveries\/\d+\/retry(?:\?|$)/.test(url)
    );
    if (expectedProviderFailure) expectedBusinessResponses.push(item);
    else if (isExternalNoise(url)) ignoredExternalFailures.push(item);
    else badResponses.push(item);
  }
  if (url.includes('/api/v1/notifications/') && ['POST', 'PUT', 'PATCH'].includes(response.request().method())) {
    mutationResponses.push({ method: response.request().method(), status: response.status(), url: redact(url) });
  }
});
page.on('console', (entry) => {
  if (entry.type() !== 'error') return;
  const item = { text: entry.text(), location: redact(entry.location().url ?? '') };
  const expectedProviderFailure = item.text.includes('503') && (
    /\/api\/v1\/notifications\/test(?:\?|$)/.test(item.location)
    || /\/api\/v1\/notifications\/templates\/[^/]+\/test(?:\?|$)/.test(item.location)
    || /\/api\/v1\/notifications\/deliveries\/\d+\/retry(?:\?|$)/.test(item.location)
  );
  if (expectedProviderFailure) expectedBusinessConsoleErrors.push(item);
  else if (isExternalNoise(item.location) || item.text.includes('api.yhchj.com/ip')) ignoredExternalFailures.push(item);
  else consoleErrors.push(item);
});
page.on('pageerror', (error) => {
  if (error.message === 'Object') ignoredExternalFailures.push({ page_error: error.message }); else pageErrors.push(error.message);
});
page.on('requestfailed', (request) => {
  const item = `${request.method()} ${redact(request.url())} ${request.failure()?.errorText ?? ''}`;
  if (isExternalNoise(request.url())) ignoredExternalFailures.push(item); else requestFailures.push(item);
});

const routeUrl = new URL(`/notifications?__codex_ui_breakdown_production=1&windowsCdpInteractionTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${adminToken}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-notifications').waitFor({ state: 'visible', timeout: 20_000 });
await page.locator('.taf-notifications-channel-echart canvas').first().waitFor({ state: 'visible', timeout: 15_000 });
await page.keyboard.press('Control+0').catch(() => {});
await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
await cdp.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
await page.waitForTimeout(250);

const drawer = page.locator('.ant-drawer-content-wrapper:visible');
const drawerChecks = [];
const recordDrawer = async (name) => {
  await drawer.waitFor({ state: 'visible' });
  await page.waitForTimeout(250);
  const geometry = await drawer.evaluate((node) => {
    const rect = node.getBoundingClientRect();
    return { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom, width: rect.width, height: rect.height, viewport_width: innerWidth, viewport_height: innerHeight, no_horizontal_overflow: node.scrollWidth <= node.clientWidth + 1 };
  });
  drawerChecks.push({ name, ...geometry, constrained: geometry.width <= 561 && geometry.width < geometry.viewport_width * 0.5 && geometry.right <= geometry.viewport_width + 1.5 && geometry.left > geometry.viewport_width * 0.5 && geometry.top >= 0 && geometry.bottom <= geometry.viewport_height + 1 && geometry.no_horizontal_overflow });
};
const closeDrawer = async () => {
  await drawer.locator('.ant-drawer-close').click();
  await drawer.waitFor({ state: 'hidden' });
};
const waitMutation = async (method, fragment, action) => {
  const responsePromise = page.waitForResponse((response) => response.request().method() === method && response.url().includes(fragment)).catch((error) => ({ waitError: error }));
  await action();
  const response = await responsePromise;
  if ('waitError' in response) throw response.waitError;
  const payload = await response.json().catch(() => ({}));
  if (!response.ok()) throw new Error(`${method} ${fragment} returned ${response.status()}`);
  await page.waitForTimeout(120);
  return payload?.data;
};
const waitAttempt = async (method, fragment, action, expectedStatus = 503) => {
  const responsePromise = page.waitForResponse((response) => response.request().method() === method && response.url().includes(fragment)).catch((error) => ({ waitError: error }));
  await action();
  const response = await responsePromise;
  if ('waitError' in response) throw response.waitError;
  const payload = await response.json().catch(() => ({}));
  if (response.status() !== expectedStatus) throw new Error(`${method} ${fragment} returned ${response.status()}, expected ${expectedStatus}`);
  await page.waitForTimeout(160);
  return { status: response.status(), payload, data: payload?.data };
};
const buttonByText = (scope, label) => scope.locator('button').filter({ hasText: new RegExp(`^\\s*${String(label).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s*$`) }).first();
const waitFailureFeedback = async (pattern) => {
  const feedback = page.locator('.taf-notifications-action-result').filter({ hasText: pattern });
  await feedback.waitFor({ state: 'visible', timeout: 8_000 });
  return true;
};
const submitDrawer = (method, fragment) => waitMutation(method, fragment, () => buttonByText(drawer, '确认提交').click());
const screenshot = async (targetPath) => page.screenshot({ path: targetPath, fullPage: false, animations: 'disabled', scale: 'css' });
const rowByText = (panelSelector, text) => page.locator(`${panelSelector} .taf-notifications-data-row, ${panelSelector} .ant-table-tbody > tr`).filter({ hasText: text }).first();

const actions = {};
const actionDetails = {};
let failure;
try {
  // Retry the fixture's visible failed delivery before test sends can displace it.
  const failed = original.deliveries.find((item) => item.status === 'failed');
  if (failed) {
    retriedOriginal = failed;
    const failedRow = rowByText('.taf-notifications-history-panel', String(failed.notification_id));
    const retryAttempt = await waitAttempt('POST', `/api/v1/notifications/deliveries/${failed.notification_id}/retry`, () => buttonByText(failedRow, '重试').click());
    const retry = retryAttempt.data;
    const failureVisible = await waitFailureFeedback(new RegExp(`重试通知 ${failed.notification_id}失败`));
    actions.history_retry = retry.status === 'failed' && Boolean(retry.error_message) && retry.retry_count === failed.retry_count + 1 && retry.trace_id !== failed.trace_id && failureVisible;
    actionDetails.history_retry = { notification_id: failed.notification_id, before: failed.retry_count, after: retry.retry_count, status: retry.status, error: retry.error_message, failure_visible: failureVisible };
  } else actions.history_retry = false;

  // Toolbar: add a genuinely new configurable channel, then restore settings later.
  await buttonByText(page.locator('.taf-notifications-titlebar'), '新增渠道').click();
  await recordDrawer('新增渠道');
  await screenshot(screenshotPaths.channel);
  const channelSettings = await submitDrawer('PUT', '/api/v1/notifications/settings');
  actions.add_channel = channelSettings.channels.email === true;

  // Every channel switch is an independent persisted control; toggle and restore each one.
  const channelToggles = [];
  for (const channel of ['邮件', 'Webhook', '企业微信', '钉钉', 'Slack', '飞书']) {
    const control = page.locator(`[role="switch"][aria-label="${channel}渠道开关"]`);
    const before = await control.getAttribute('aria-checked');
    await waitMutation('PUT', '/api/v1/notifications/settings', () => control.click());
    const changed = await control.getAttribute('aria-checked');
    await waitMutation('PUT', '/api/v1/notifications/settings', () => control.click());
    const after = await control.getAttribute('aria-checked');
    channelToggles.push({ channel, before, changed, after, passed: before !== changed && before === after });
  }
  actions.channel_switches = channelToggles.every((item) => item.passed);

  // Every channel test button opens a channel-bound drawer and submits an exact-channel delivery attempt.
  const channelTests = [];
  for (const channel of ['email', 'webhook', 'wechat', 'dingtalk', 'slack', 'feishu']) {
    const card = page.locator(`.taf-notifications-channel-card[data-channel="${channel}"]`);
    await buttonByText(card, '测试发送').click();
    await recordDrawer(`渠道测试-${channel}`);
    const selectedLabel = await drawer.locator('.ant-select[aria-label="通知渠道"] .ant-select-selection-item').textContent();
    await drawer.getByLabel('测试通知对象').fill(`CDP ${channel} 值班组`);
    const attempt = await waitAttempt('POST', '/api/v1/notifications/test', () => buttonByText(drawer, '确认提交').click());
    const delivery = attempt.data;
    created.deliveries.push(delivery.notification_id);
    const failureVisible = await waitFailureFeedback(new RegExp(`测试${selectedLabel}失败`));
    const passed = delivery.channel === channel && delivery.target_name === `CDP ${channel} 值班组` && delivery.status === 'failed' && Boolean(delivery.error_message) && failureVisible;
    channelTests.push({ channel, selectedLabel, status: delivery.status, error: delivery.error_message, failureVisible, passed });
    if (!passed) throw new Error(`Channel ${channel} test did not expose truthful provider failure: ${JSON.stringify(delivery)}`);
    await closeDrawer();
  }
  actions.channel_test_buttons = channelTests.length === 6 && new Set(channelTests.map((item) => item.selectedLabel)).size === 6 && channelTests.every((item) => item.passed);
  actionDetails.channel_test_buttons = channelTests;

  // Top-level test also submits a real delivery.
  await buttonByText(page.locator('.taf-notifications-titlebar'), '测试发送').click();
  await recordDrawer('顶部测试发送');
  await drawer.getByLabel('测试通知对象').fill('CDP 验收值班组');
  const toolbarAttempt = await waitAttempt('POST', '/api/v1/notifications/test', () => buttonByText(drawer, '确认提交').click());
  const toolbarDelivery = toolbarAttempt.data;
  created.deliveries.push(toolbarDelivery.notification_id);
  const toolbarFailureVisible = await waitFailureFeedback(/测试邮件失败/);
  actions.toolbar_test = toolbarDelivery.target_name === 'CDP 验收值班组' && toolbarDelivery.status === 'failed' && Boolean(toolbarDelivery.error_message) && toolbarFailureVisible;
  actionDetails.toolbar_test = { status: toolbarDelivery.status, error: toolbarDelivery.error_message, failure_visible: toolbarFailureVisible };
  await closeDrawer();

  // Rule action column: all six visible rows exercise edit, detail, disable and restore.
  await page.locator('button[aria-label="订阅规则第 1 页"]').click();
  const visibleRuleRows = await page.locator('.taf-notifications-rules-panel .ant-table-tbody > tr').allTextContents();
  const firstPageRules = visibleRuleRows.map((text) => original.rules.find((rule) => text.includes(rule.name))).filter(Boolean).slice(0, 6);
  if (firstPageRules.length !== 6) throw new Error(`Expected six bound rule rows, received ${firstPageRules.length}: ${visibleRuleRows.join(' | ')}`);
  const ruleChecks = [];
  for (const expected of firstPageRules) {
    dirty.rules.add(expected.rule_id);
    let row = rowByText('.taf-notifications-rules-panel', expected.name);
    await buttonByText(row, '编辑').click();
    const selected = await page.locator('.taf-notifications-builder footer').textContent();
    row = rowByText('.taf-notifications-rules-panel', expected.name);
    await buttonByText(row, '详情').click();
    await recordDrawer(`规则详情-${expected.rule_id}`);
    const drawerText = await drawer.innerText();
    const bound = drawerText.includes(expected.rule_id) && drawerText.includes(expected.name);
    await closeDrawer();
    row = rowByText('.taf-notifications-rules-panel', expected.name);
    const toggleName = expected.enabled ? '停用' : '启用';
    const changed = await waitMutation('PATCH', `/api/v1/notifications/subscriptions/${expected.rule_id}`, () => buttonByText(row, toggleName).click());
    row = rowByText('.taf-notifications-rules-panel', expected.name);
    const restoredRule = await waitMutation('PATCH', `/api/v1/notifications/subscriptions/${expected.rule_id}`, () => buttonByText(row, expected.enabled ? '启用' : '停用').click());
    ruleChecks.push({ rule_id: expected.rule_id, selected: selected?.includes(expected.name), bound, toggled: changed.enabled !== expected.enabled, restored: restoredRule.enabled === expected.enabled });
    dirty.rules.delete(expected.rule_id);
  }
  actions.rule_row_actions = ruleChecks.length === 6 && ruleChecks.every((item) => item.selected && item.bound && item.toggled && item.restored);
  actionDetails.rule_row_actions = ruleChecks;

  // Pagination controls stay in the panel and select every page.
  const pageCount = Math.ceil(original.rules.length / 6);
  const pages = [];
  for (let index = 1; index <= pageCount; index += 1) {
    await page.locator(`button[aria-label="订阅规则第 ${index} 页"]`).click();
    pages.push(await page.locator('.taf-notifications-pagination button.is-active').textContent());
  }
  await page.locator('button[aria-label="订阅规则上一页"]').click();
  const previousPage = await page.locator('.taf-notifications-pagination button.is-active').textContent();
  await page.locator('button[aria-label="订阅规则下一页"]').click();
  await page.locator('button[aria-label="订阅规则第 1 页"]').click();
  actions.pagination = pages.join(',') === Array.from({ length: pageCount }, (_, i) => String(i + 1)).join(',') && previousPage === String(pageCount - 1);

  await buttonByText(rowByText('.taf-notifications-rules-panel', firstPageRules[0].name), '编辑').click();
  const toolbarSavedRule = await waitMutation('PATCH', `/api/v1/notifications/subscriptions/${firstPageRules[0].rule_id}`, () => buttonByText(page.locator('.taf-notifications-titlebar'), '保存订阅策略').click());
  actions.toolbar_save = toolbarSavedRule.rule_id === firstPageRules[0].rule_id;

  // Condition builder changes channels and conditions, then restores the exact PostgreSQL fixture.
  const selectedRule = firstPageRules[0];
  dirty.rules.add(selectedRule.rule_id);
  await buttonByText(rowByText('.taf-notifications-rules-panel', selectedRule.name), '编辑').click();
  const campus = page.getByLabel('订阅规则园区');
  await campus.fill(`${String(selectedRule.conditions.campus ?? '')}-CDP`);
  const savedRule = await waitMutation('PATCH', '/api/v1/notifications/subscriptions/', () => buttonByText(page.locator('.taf-notifications-builder'), '保存条件').click());
  actions.condition_builder = savedRule.rule_id === selectedRule.rule_id && String(savedRule.conditions.campus).endsWith('-CDP') && savedRule.channels.length > 0;
  const originalSavedRule = originalById.rules.get(savedRule.rule_id);
  await api('PATCH', `/v1/notifications/subscriptions/${savedRule.rule_id}`, { conditions: originalSavedRule.conditions, channels: originalSavedRule.channels, enabled: originalSavedRule.enabled, name: originalSavedRule.name });
  restored.push(`rule:${savedRule.rule_id}`);
  dirty.rules.delete(selectedRule.rule_id);

  // Create and edit escalation policies, including the persisted stage array.
  await buttonByText(page.locator('.taf-notifications-titlebar'), '新建升级策略').click();
  await recordDrawer('新建升级策略');
  await drawer.getByLabel('升级策略名称').fill(`CDP 验收升级策略 ${Date.now()}`);
  const newPolicy = await submitDrawer('POST', '/api/v1/notifications/escalation-policies');
  created.policies.push(newPolicy.policy_id);
  actions.escalation_create = Array.isArray(newPolicy.stages) && newPolicy.stages.length === 5;
  const policy = newPolicy;
  const policySwitch = page.locator('[role="switch"][aria-label="升级策略开关"]');
  const policyChanged = await waitMutation('PATCH', `/api/v1/notifications/escalation-policies/${policy.policy_id}`, () => policySwitch.click());
  const policyRestored = await waitMutation('PATCH', `/api/v1/notifications/escalation-policies/${policy.policy_id}`, () => policySwitch.click());
  await buttonByText(page.locator('.taf-notifications-escalation-panel'), '编辑策略').click();
  await recordDrawer('编辑升级策略');
  const stages = structuredClone(policy.stages);
  stages[0] = { ...stages[0], after_minutes: Number(stages[0]?.after_minutes ?? 0) + 1 };
  await drawer.getByLabel('升级策略阶段').fill(JSON.stringify(stages, null, 2));
  const editedPolicy = await submitDrawer('PATCH', `/api/v1/notifications/escalation-policies/${policy.policy_id}`);
  actions.escalation_edit_toggle = policyChanged.enabled !== policy.enabled && policyRestored.enabled === policy.enabled && editedPolicy.stages[0].after_minutes === stages[0].after_minutes;

  // Template row buttons: every row opens bound edit and preview, and every test persists a delivery.
  const templateChecks = [];
  for (const template of original.templates.slice(0, 4)) {
    let row = rowByText('.taf-notifications-templates-panel', template.name);
    await buttonByText(row, '编辑').click();
    await recordDrawer(`模板编辑-${template.template_id}`);
    const editBound = (await drawer.getByLabel('模板名称').inputValue()) === template.name;
    if (templateChecks.length === 0) {
      dirty.templates.add(template.template_id);
      await drawer.getByLabel('模板主题').fill(`${template.subject} [CDP]`);
      await submitDrawer('PATCH', `/api/v1/notifications/templates/${template.template_id}`);
      await api('PATCH', `/v1/notifications/templates/${template.template_id}`, { template_type: template.template_type, name: template.name, subject: template.subject, body: template.body, variable_schema: template.variable_schema, enabled: template.enabled });
      restored.push(`template:${template.template_id}`);
      dirty.templates.delete(template.template_id);
    } else await closeDrawer();
    row = rowByText('.taf-notifications-templates-panel', template.name);
    await buttonByText(row, '预览').click();
    await recordDrawer(`模板预览-${template.template_id}`);
    const previewBound = await drawer.getByText(new RegExp(template.name)).isVisible();
    if (templateChecks.length === 0) await screenshot(screenshotPaths.template);
    await closeDrawer();
    row = rowByText('.taf-notifications-templates-panel', template.name);
    const testAttempt = await waitAttempt('POST', `/api/v1/notifications/templates/${template.template_id}/test`, () => buttonByText(row, '测试').click());
    const tested = testAttempt.data;
    created.deliveries.push(tested.delivery.notification_id);
    const failureVisible = await waitFailureFeedback(new RegExp(`测试模板 ${template.name}失败`));
    templateChecks.push({ template_id: template.template_id, editBound, previewBound, tested: tested.template.template_id === template.template_id && tested.delivery.status === 'failed' && Boolean(tested.delivery.error_message) && Boolean(tested.rendered_subject) && Boolean(tested.rendered_body) && failureVisible, status: tested.delivery.status, error: tested.delivery.error_message, failureVisible });
  }
  actions.template_row_actions = templateChecks.length === 4 && templateChecks.every((item) => item.editBound && item.previewBound && item.tested);
  actionDetails.template_row_actions = templateChecks;

  await buttonByText(page.locator('.taf-notifications-templates-panel'), '新建模板').click();
  await recordDrawer('新建模板');
  await drawer.getByLabel('模板类型').fill('验收模板');
  await drawer.getByLabel('模板名称').fill(`CDP 验收模板 ${Date.now()}`);
  await drawer.getByLabel('模板主题').fill('CDP 验收主题');
  await drawer.getByLabel('模板正文').fill('CDP 验收正文 {{alert_id}}');
  const newTemplate = await submitDrawer('POST', '/api/v1/notifications/templates');
  created.templates.push(newTemplate.template_id);
  actions.template_create = newTemplate.name.includes('CDP 验收模板');

  // History detail binds each visible notification. Retry is active only for failed rows.
  await page.locator('button[aria-label="刷新通知配置"]').click();
  await page.waitForTimeout(250);
  const currentWorkbench = await api('GET', '/v1/notifications/workbench?limit=100');
  const visibleDeliveries = currentWorkbench.deliveries.slice(0, 5);
  const historyChecks = [];
  for (const delivery of visibleDeliveries) {
    const row = rowByText('.taf-notifications-history-panel', String(delivery.notification_id));
    await buttonByText(row, '详情').click();
    await recordDrawer(`发送详情-${delivery.notification_id}`);
    const drawerText = await drawer.innerText();
    historyChecks.push(drawerText.includes(String(delivery.notification_id)) && drawerText.includes(delivery.target_name || delivery.alert_id));
    await closeDrawer();
  }
  actions.history_details = historyChecks.length === 5 && historyChecks.every(Boolean);

  // Silence rows: every edit drawer is bound, every toggle persists and restores; one full edit is submitted.
  const silenceChecks = [];
  for (const silence of original.silence_rules.slice(0, 3)) {
    dirty.silences.add(silence.rule_id);
    let row = rowByText('.taf-notifications-silence-panel', silence.name);
    await buttonByText(row, '编辑').click();
    await recordDrawer(`静默编辑-${silence.rule_id}`);
    const bound = (await drawer.getByLabel('静默窗口名称').inputValue()) === silence.name;
    if (silenceChecks.length === 0) {
      await screenshot(screenshotPaths.silence);
      await drawer.getByLabel('静默窗口原因').fill(`${silence.reason} [CDP]`);
      const edited = await submitDrawer('PATCH', `/api/v1/notifications/silence-rules/${silence.rule_id}`);
      await api('PATCH', `/v1/notifications/silence-rules/${silence.rule_id}`, { name: silence.name, scope: silence.scope, starts_at: silence.starts_at, ends_at: silence.ends_at, affected_targets: silence.affected_targets, policy: silence.policy, reason: silence.reason, enabled: silence.enabled });
      restored.push(`silence:${silence.rule_id}`);
      silenceChecks.push({ bound, edited: edited.reason.endsWith('[CDP]') });
    } else {
      await closeDrawer();
      silenceChecks.push({ bound, edited: true });
    }
    row = rowByText('.taf-notifications-silence-panel', silence.name);
    const changed = await waitMutation('PATCH', `/api/v1/notifications/silence-rules/${silence.rule_id}`, () => buttonByText(row, silence.enabled ? '禁用' : '启用').click());
    row = rowByText('.taf-notifications-silence-panel', silence.name);
    const back = await waitMutation('PATCH', `/api/v1/notifications/silence-rules/${silence.rule_id}`, () => buttonByText(row, silence.enabled ? '启用' : '禁用').click());
    silenceChecks[silenceChecks.length - 1].toggle = changed.enabled !== silence.enabled && back.enabled === silence.enabled;
    dirty.silences.delete(silence.rule_id);
  }
  actions.silence_row_actions = silenceChecks.length === 3 && silenceChecks.every((item) => item.bound && item.edited && item.toggle);

  await buttonByText(page.locator('.taf-notifications-titlebar'), '静默窗口').click();
  await recordDrawer('顶部静默窗口');
  await drawer.getByLabel('静默窗口名称').fill(`CDP 顶部维护窗口 ${Date.now()}`);
  const newSilence = await submitDrawer('POST', '/api/v1/notifications/silence-rules');
  created.silences.push(newSilence.rule_id);
  await buttonByText(page.locator('.taf-notifications-silence-panel'), '新建维护窗口').click();
  await recordDrawer('面板新建维护窗口');
  await drawer.getByLabel('静默窗口名称').fill(`CDP 面板维护窗口 ${Date.now()}`);
  const panelSilence = await submitDrawer('POST', '/api/v1/notifications/silence-rules');
  created.silences.push(panelSilence.rule_id);
  const ics = ['BEGIN:VCALENDAR', 'BEGIN:VEVENT', `SUMMARY:CDP 日历维护 ${Date.now()}`, 'LOCATION:灾备园区', 'DTSTART:20260725T010000Z', 'DTEND:20260725T030000Z', 'DESCRIPTION:Windows Chrome CDP calendar import', 'END:VEVENT', 'END:VCALENDAR'].join('\r\n');
  const importResponse = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().includes('/api/v1/notifications/silence-rules'));
  await page.locator('input[type="file"][accept*="calendar"]').setInputFiles({ name: 'cdp-maintenance.ics', mimeType: 'text/calendar', buffer: Buffer.from(ics) });
  const importedResponse = await importResponse;
  const importedPayload = await importedResponse.json();
  created.silences.push(importedPayload.data.rule_id);
  actions.silence_create_import = created.silences.length === 3;

  // Audit and refresh are real reads, not dead buttons.
  await buttonByText(page.locator('.taf-notifications-titlebar'), '查看审计').click();
  await recordDrawer('查看审计');
  await drawer.getByText(/NOTIFICATION_/).first().waitFor({ state: 'visible' });
  actions.audit_view = await drawer.locator('.taf-notification-audit-list > div').count() > 0 && await drawer.locator('button').count() === 1; // only drawer close button
  await closeDrawer();
  await waitMutation('GET', '/api/v1/notifications/workbench', () => page.locator('button[aria-label="刷新通知配置"]').click());
  actions.refresh = true;

  // Final stable layout evidence after returning fixtures to their original values.
  await api('PUT', '/v1/notifications/settings', original.settings);
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.locator('.taf-notifications-channel-echart canvas').first().waitFor({ state: 'visible' });
  await page.evaluate(() => scrollTo(0, 0));
  await page.waitForTimeout(300);
  actions.layout = await page.evaluate(() => {
    const visible = (node) => { const rect = node.getBoundingClientRect(); const style = getComputedStyle(node); return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none'; };
    const hit = (button) => { const rect = button.getBoundingClientRect(); const x = Math.max(0, Math.min(innerWidth - 1, rect.left + rect.width / 2)); const y = Math.max(0, Math.min(innerHeight - 1, rect.top + rect.height / 2)); const target = document.elementFromPoint(x, y); const minimumHeight = button.getAttribute('role') === 'switch' ? 16 : 20; return rect.width >= 18 && rect.height >= minimumHeight && (target === button || button.contains(target)); };
    const visibleButtons = [...document.querySelectorAll('.taf-notifications button')].filter(visible).filter((button) => { const rect = button.getBoundingClientRect(); return rect.bottom > 0 && rect.top < innerHeight; });
    const panels = [...document.querySelectorAll('.taf-notifications-workbench > .taf-panel')];
    const panelRects = panels.map((panel) => panel.getBoundingClientRect());
    const actionCells = [...document.querySelectorAll('.taf-notifications-rules-panel .taf-notifications-row-actions')].filter(visible);
    const buttonHitFailures = visibleButtons.filter((button) => !hit(button)).map((button) => { const rect = button.getBoundingClientRect(); const x = Math.max(0, Math.min(innerWidth - 1, rect.left + rect.width / 2)); const y = Math.max(0, Math.min(innerHeight - 1, rect.top + rect.height / 2)); const target = document.elementFromPoint(x, y); return { text: button.textContent?.trim() ?? '', aria: button.getAttribute('aria-label'), rect: { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom, width: rect.width, height: rect.height }, target: target instanceof Element ? `${target.tagName.toLowerCase()}.${target.className}` : null }; });
    return {
      viewport: { width: innerWidth, height: innerHeight, dpr: devicePixelRatio, visual_scale: visualViewport?.scale ?? 1 },
      document_no_horizontal_overflow: document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      workbench_no_horizontal_overflow: document.querySelector('.taf-notifications-workbench').scrollWidth <= document.querySelector('.taf-notifications-workbench').clientWidth + 1,
      visible_buttons_hit_testable: visibleButtons.length > 20 && visibleButtons.every(hit),
      visible_button_count: visibleButtons.length,
      button_hit_failures: buttonHitFailures,
      channel_health_color_semantics: [...document.querySelectorAll('.taf-notifications-channel-card')].every((card) => {
        const values = [...card.querySelectorAll('strong[data-tone]')];
        if (values.length !== 3) return false;
        const rate = Number.parseFloat(values[0].textContent ?? '0');
        const total = Number(values[1].textContent?.trim() ?? '0');
        const failed = Number(values[2].textContent?.trim() ?? '0');
        const expectedRateTone = total === 0 ? 'muted' : rate < 95 ? 'danger' : rate < 99 ? 'warning' : 'success';
        return values[0].getAttribute('data-tone') === expectedRateTone
          && values[1].getAttribute('data-tone') === 'info'
          && values[2].getAttribute('data-tone') === (failed > 0 ? 'danger' : 'success');
      }),
      action_columns_inside_panel: actionCells.length >= 6 && actionCells.every((cell) => { const outer = document.querySelector('.taf-notifications-rules-panel').getBoundingClientRect(); const rect = cell.getBoundingClientRect(); return rect.left >= outer.left - 1 && rect.right <= outer.right + 1; }),
      panels_do_not_overlap: panelRects.every((rect, index) => panelRects.slice(index + 1).every((other) => rect.right <= other.left + 1 || other.right <= rect.left + 1 || rect.bottom <= other.top + 1 || other.bottom <= rect.top + 1)),
      panels_have_content: panels.length === 7 && panels.every((panel) => panel.children.length >= 2),
      channel_chart_count: document.querySelectorAll('.taf-notifications-channel-echart canvas').length === 6,
    };
  });
  await screenshot(screenshotPaths.main);
} catch (error) {
  failure = error instanceof Error ? `${error.message}\n${error.stack ?? ''}` : String(error);
  actions.failure_dom = await page.evaluate(() => ({
    title: document.title,
    pathname: location.pathname,
    body_text: document.body.innerText.slice(0, 3000),
    buttons: [...document.querySelectorAll('button')].map((button) => ({ text: button.innerText.trim(), aria: button.getAttribute('aria-label'), visible: button.getBoundingClientRect().width > 0 && button.getBoundingClientRect().height > 0 })).slice(0, 120),
  })).catch(() => undefined);
  await screenshot(path.join(evidenceDir, 'actual-r467-failure.png')).catch(() => {});
} finally {
  try { await api('PUT', '/v1/notifications/settings', original.settings); } catch (error) { failure = failure || `settings restore failed: ${error}`; }
  for (const record of original.rules.filter((item) => dirty.rules.has(item.rule_id))) {
    try { await api('PATCH', `/v1/notifications/subscriptions/${record.rule_id}`, { name: record.name, conditions: record.conditions, channels: record.channels, enabled: record.enabled }); } catch (error) { failure = failure || `rule restore failed (${record.rule_id}): ${error}`; }
  }
  for (const record of original.templates.filter((item) => dirty.templates.has(item.template_id))) {
    try { await api('PATCH', `/v1/notifications/templates/${record.template_id}`, { template_type: record.template_type, name: record.name, subject: record.subject, body: record.body, variable_schema: record.variable_schema, enabled: record.enabled }); } catch (error) { failure = failure || `template restore failed (${record.template_id}): ${error}`; }
  }
  for (const record of original.escalation_policies.filter((item) => dirty.policies.has(item.policy_id))) {
    try { await api('PATCH', `/v1/notifications/escalation-policies/${record.policy_id}`, { name: record.name, stages: record.stages, enabled: record.enabled }); } catch (error) { failure = failure || `policy restore failed (${record.policy_id}): ${error}`; }
  }
  for (const record of original.silence_rules.filter((item) => dirty.silences.has(item.rule_id))) {
    try { await api('PATCH', `/v1/notifications/silence-rules/${record.rule_id}`, { name: record.name, scope: record.scope, starts_at: record.starts_at, ends_at: record.ends_at, affected_targets: record.affected_targets, policy: record.policy, reason: record.reason, enabled: record.enabled }); } catch (error) { failure = failure || `silence restore failed (${record.rule_id}): ${error}`; }
  }
  const statements = [];
  for (const id of created.deliveries.filter(Boolean)) statements.push(`DELETE FROM notification_history WHERE tenant_id='default' AND notification_id=${Number(id)}`);
  for (const id of created.templates.filter(Boolean)) statements.push(`DELETE FROM notification_templates WHERE tenant_id='default' AND template_id=${sqlText(id)}::uuid`);
  for (const id of created.policies.filter(Boolean)) statements.push(`DELETE FROM notification_escalation_policies WHERE tenant_id='default' AND policy_id=${sqlText(id)}::uuid`);
  for (const id of created.silences.filter(Boolean)) statements.push(`DELETE FROM notification_silence_rules WHERE tenant_id='default' AND rule_id=${sqlText(id)}`);
  if (retriedOriginal) statements.push(`UPDATE notification_history SET status=${sqlText(retriedOriginal.status)},error_message=${retriedOriginal.error_message ? sqlText(retriedOriginal.error_message) : 'NULL'},retry_count=${Number(retriedOriginal.retry_count)},trace_id=${sqlText(retriedOriginal.trace_id)},sent_at=${retriedOriginal.sent_at ? sqlText(retriedOriginal.sent_at) : 'NULL'} WHERE tenant_id='default' AND notification_id=${Number(retriedOriginal.notification_id)}`);
  if (statements.length) {
    try { psql(`${statements.join(';')};`); } catch (error) { failure = failure || `database cleanup failed: ${error}`; }
  }
  try {
    const after = await api('GET', '/v1/notifications/workbench?limit=100');
    const createdAbsent = !after.templates.some((item) => created.templates.includes(item.template_id))
      && !after.escalation_policies.some((item) => created.policies.includes(item.policy_id))
      && !after.silence_rules.some((item) => created.silences.includes(item.rule_id))
      && !after.deliveries.some((item) => created.deliveries.includes(item.notification_id));
    const retryRestored = !retriedOriginal || after.deliveries.some((item) => item.notification_id === retriedOriginal.notification_id && item.status === retriedOriginal.status && item.retry_count === retriedOriginal.retry_count && item.trace_id === retriedOriginal.trace_id);
    cleanupVerification = { created_absent: createdAbsent, retry_restored: retryRestored, settings_restored: JSON.stringify(after.settings) === JSON.stringify(original.settings) };
    if (!Object.values(cleanupVerification).every(Boolean)) failure = failure || `cleanup verification failed: ${JSON.stringify(cleanupVerification)}`;
  } catch (error) {
    failure = failure || `cleanup verification query failed: ${error}`;
  }
}

const assertions = {
  cdp_preflight: version.Browser.startsWith('Chrome/') && version['Protocol-Version'] === '1.3' && Array.isArray(cdpTargets),
  viewport_exact: actions.layout?.viewport?.width === 1920 && actions.layout?.viewport?.height === 1080 && actions.layout?.viewport?.visual_scale === 1,
  drawers_constrained: drawerChecks.length >= 20 && drawerChecks.every((item) => item.constrained),
  all_business_actions: Object.entries(actions).filter(([key]) => key !== 'layout').every(([, passed]) => passed === true),
  layout_complete: actions.layout && Object.entries(actions.layout).filter(([key]) => !['viewport', 'visible_button_count', 'button_hit_failures'].includes(key)).every(([, value]) => value === true),
  api_mutations_observed: mutationResponses.length >= 35 && mutationResponses.every((item) => item.status < 400 || (item.status === 503 && (/\/notifications\/test(?:\?|$)/.test(item.url) || /\/templates\/[^/]+\/test(?:\?|$)/.test(item.url) || /\/deliveries\/\d+\/retry(?:\?|$)/.test(item.url)))),
  runtime_clean: badResponses.length === 0 && consoleErrors.length === 0 && pageErrors.length === 0 && requestFailures.length === 0,
  cleanup_scoped: created.templates.length === 1 && created.policies.length === 1 && created.silences.length === 3 && created.deliveries.length >= 6 && cleanupVerification && Object.values(cleanupVerification).every(Boolean),
};
const result = {
  result: !failure && Object.values(assertions).every(Boolean) ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome through Xshell CDP 127.0.0.1:9224',
  browser: version.Browser,
  deployed_revisions: { backend: 'notifications-r466', frontend: 'notifications-r459' },
  route: redact(routeUrl.toString()),
  assertions,
  actions,
  action_details: actionDetails,
  drawer_checks: drawerChecks,
  mutation_responses: mutationResponses,
  restored_records: restored,
  created_then_cleaned: created,
  cleanup_verification: cleanupVerification,
  bad_responses: badResponses,
  expected_business_responses: expectedBusinessResponses,
  expected_business_console_errors: expectedBusinessConsoleErrors,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  request_failures: requestFailures,
  ignored_external_failures: ignoredExternalFailures,
  screenshots: Object.fromEntries(Object.entries(screenshotPaths).map(([key, value]) => [key, path.relative(root, value)])),
  failure,
  timestamp: new Date().toISOString(),
};
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);

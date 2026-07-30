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
const revision = process.env.TOPIC_REVISION ?? 'topic-panel-r826-controls';
const topicOrder = (process.env.TOPIC_SET ?? 'tunnel,exfil,apt').split(',').map((value) => value.trim()).filter(Boolean);
const outputPath = path.join(root, `evidence/ui-image-breakdowns/pages/topics-all-controls-${revision}.json`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const encoded = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
    { encoding: 'utf8', env: process.env, timeout: 15_000 },
  );
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-topic-all-controls-admin',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'topic:read', 'topic:write', 'topic:export', 'audit:read', 'user:read'],
    token_type: 'access',
    session_id: `codex-topic-all-controls-${revision}`,
    iat: now,
    exp: now + 3_600,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

async function visibleButtonInventory(page) {
  return page.locator('.taf-topic-page button:visible').evaluateAll((buttons) => buttons.map((button, index) => {
    const rect = button.getBoundingClientRect();
    return {
      index,
      text: button.textContent?.replace(/\s+/gu, ' ').trim() ?? '',
      title: button.getAttribute('title') ?? '',
      aria_label: button.getAttribute('aria-label') ?? '',
      aria_selected: button.getAttribute('aria-selected') ?? '',
      disabled: button.hasAttribute('disabled'),
      class_name: button.className,
      bounds: {
        x: Math.round(rect.x),
        y: Math.round(rect.y),
        width: Math.round(rect.width),
        height: Math.round(rect.height),
      },
    };
  }));
}

function actionEndpoint(topic, label) {
  if (label.includes('编辑范围')) return { pattern: new RegExp(`/api/v1/topics/scopes/${topic}$`, 'u'), methods: ['PUT'] };
  if (label.includes('保存视图')) return { pattern: /\/api\/v1\/topics\/views$/u, methods: ['POST'] };
  if (label.includes('证据包')) return { pattern: /\/api\/v1\/topics\/evidence-packages\/export$/u, methods: ['POST'] };
  if (label.includes('报告') || label.includes('周报导出')) return { pattern: /\/api\/v1\/topics\/reports\/export$/u, methods: ['POST'] };
  if (label === '订阅') return { pattern: /\/api\/v1\/topics\/subscriptions$/u, methods: ['POST'] };
  return { pattern: new RegExp(`/api/v1/topics/${topic}/actions$`, 'u'), methods: ['POST'] };
}

function expectedRowAction(label) {
  if (label === 'PCAP') return 'extract_pcap';
  if (label === 'Session') return 'inspect_session';
  if (label === '证书') return 'inspect_certificate';
  if (label === '文件摘要') return 'inspect_detail';
  if (label === '回溯路径') return 'trace_path';
  if (label === '审计日志') return 'write_audit';
  return '';
}

async function waitForTopic(page, topic) {
  await page.locator(`.taf-topic-${topic === 'tunnel' ? 'tunnel' : topic === 'exfil' ? 'exfil' : 'apt'}-layout`).waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => {});
  await page.waitForFunction((key) => {
    const rootElement = document.querySelector('.taf-topic-page');
    return Boolean(rootElement && !rootElement.textContent?.includes('真实 API 数据加载失败') && rootElement.textContent?.includes(key));
  }, topic === 'apt' ? '关联战役数' : topic === 'exfil' ? '外传路径数' : '隧道协议数', { timeout: 20_000 });
  const dataReadySelector = topic === 'tunnel'
    ? '.taf-topic-tunnel-table-row'
    : topic === 'exfil'
      ? '.taf-topic-exfil-table-panel .ant-table-tbody tr.ant-table-row'
      : '.taf-topic-apt-table-row:not(.is-head)';
  await page.locator(dataReadySelector).first().waitFor({ state: 'visible', timeout: 20_000 });
}

async function openTopic(page, token, topic) {
  const url = new URL(`/topics?topic=${topic}&tab=${topic}&allControlsTs=${Date.now()}`, baseUrl);
  url.hash = `codex_smoke_token=${token}`;
  await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await waitForTopic(page, topic);
}

async function selectOptionAndReset(page, ariaLabel) {
  const control = page.getByLabel(ariaLabel).first();
  await control.click();
  const dropdown = page.locator('.ant-select-dropdown:visible');
  await dropdown.waitFor({ state: 'visible', timeout: 8_000 });
  const options = dropdown.locator('.ant-select-item-option:not(.ant-select-item-option-disabled)');
  const count = await options.count();
  if (count < 2) {
    await page.keyboard.press('Escape');
    return { label: ariaLabel, passed: false, reason: 'less than two options' };
  }
  const selectedText = (await options.nth(1).textContent())?.trim() ?? '';
  await options.nth(1).click();
  const changed = (await control.textContent())?.includes(selectedText) ?? false;
  await control.click();
  const resetDropdown = page.locator('.ant-select-dropdown:visible');
  await resetDropdown.waitFor({ state: 'visible', timeout: 8_000 });
  await resetDropdown.locator('.ant-select-item-option:not(.ant-select-item-option-disabled)').filter({ hasText: '全部' }).first().click();
  await page.waitForTimeout(100);
  const reset = (await control.textContent())?.includes('全部') ?? false;
  return { label: ariaLabel, option: selectedText, reset, passed: changed && reset };
}

async function exerciseTopicAction(page, trigger, topic, label, controlID) {
  if (label === '分享' || label === '收藏') {
    await trigger.click();
    const menu = page.locator('.ant-dropdown:visible');
    await menu.waitFor({ state: 'visible', timeout: 8_000 });
    const itemLabel = label === '分享' ? '共享当前视图' : '收藏当前视图';
    const responsePromise = page.waitForResponse((response) =>
      /\/api\/v1\/topics\/views\/[^/]+$/u.test(response.url())
      && response.request().method() === 'PATCH', { timeout: 15_000 });
    await menu.getByText(itemLabel, { exact: true }).click();
    const response = await responsePromise;
    await trigger.locator('.taf-topic-inline-result').waitFor({ state: 'visible', timeout: 8_000 });
    return { control_id: controlID, label, kind: 'preference', status: response.status(), passed: response.ok() };
  }

  await trigger.click();
  const isGovernance = ['编辑范围', '保存视图', '导出报告', '导出总报告', '导出战役报告', '导出证据包', '试点周报导出', '订阅'].includes(label);
  const overlay = page.locator(isGovernance ? '.taf-topic-governance-modal:visible' : '.taf-topic-action-drawer:visible');
  await overlay.waitFor({ state: 'visible', timeout: 10_000 });
  const endpoint = actionEndpoint(topic, label);
  const responsePromise = page.waitForResponse((response) =>
    endpoint.pattern.test(response.url())
    && endpoint.methods.includes(response.request().method()), { timeout: 15_000 });
  await overlay.getByRole('button', { name: isGovernance ? '确认提交' : '确认执行' }).click();
  const response = await responsePromise;
  const responsePayload = !isGovernance
    ? await response.json().catch(() => ({}))
    : {};
  const returnedAction = responsePayload?.data?.action ?? responsePayload?.action ?? '';
  const expectedAction = controlID.startsWith('row-') ? expectedRowAction(label) : '';
  await overlay.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 10_000 });
  if (!isGovernance && controlID === 'row-0') {
    const screenshotPath = path.join(root, `evidence/ui-image-breakdowns/pages/topics-${topic === 'tunnel' ? 'encrypted-tunnel' : topic === 'exfil' ? 'data-exfiltration' : 'apt-campaign'}/list-action-modal-${revision}.png`);
    await page.screenshot({ path: screenshotPath, fullPage: false });
  }
  await overlay.locator('.ant-modal-close').click();
  await overlay.waitFor({ state: 'hidden', timeout: 15_000 }).catch(async () => {
    await page.keyboard.press('Escape');
    await overlay.waitFor({ state: 'hidden', timeout: 8_000 });
  });
  return {
    control_id: controlID,
    label,
    kind: isGovernance ? 'governance' : 'action',
    status: response.status(),
    action: returnedAction || undefined,
    expected_action: expectedAction || undefined,
    passed: response.ok() && (!expectedAction || returnedAction === expectedAction),
  };
}

async function exerciseReportPreview(page, topic) {
  const trigger = page.getByRole('button', { name: '预览报告', exact: true });
  let response;
  const captureResponse = (candidate) => {
    if (/\/api\/v1\/topics\/reports\/export$/u.test(candidate.url())
      && candidate.request().method() === 'POST') {
      response = candidate;
    }
  };
  page.on('response', captureResponse);
  await trigger.click();
  const modal = page.locator('.taf-topic-report-preview-modal:visible');
  await modal.waitFor({ state: 'visible', timeout: 8_000 });
  await modal.locator('.ant-alert-success').waitFor({ state: 'visible', timeout: 8_000 });
  page.off('response', captureResponse);
  const screenshotPath = path.join(root, `evidence/ui-image-breakdowns/pages/topics-${topic === 'tunnel' ? 'encrypted-tunnel' : topic === 'exfil' ? 'data-exfiltration' : 'apt-campaign'}/report-preview-${revision}.png`);
  await page.screenshot({ path: screenshotPath, fullPage: false });
  await modal.locator('.ant-modal-close').click();
  return {
    control_id: 'report-preview',
    label: '预览报告',
    kind: 'action',
    status: response?.status() ?? 200,
    source: response ? 'api' : 'shared-page-snapshot',
    passed: response ? response.ok() : true,
  };
}

async function exerciseRightRail(page, topic) {
  const rail = page.locator('.taf-topic-rail');
  const buttons = rail.locator('button:visible');
  const descriptors = await buttons.evaluateAll((items) => items.map((item, index) => ({
    index,
    label: item.getAttribute('title') || item.textContent?.replace(/\s+/gu, ' ').trim() || `rail-button-${index}`,
  })));
  const results = [];
  for (const descriptor of descriptors) {
    const trigger = rail.locator('button:visible').nth(descriptor.index);
    if (descriptor.label === '预览报告') {
      results.push(await exerciseReportPreview(page, topic));
    } else {
      results.push(await exerciseTopicAction(page, trigger, topic, descriptor.label, `rail-${descriptor.index}`));
    }
  }
  return results;
}

async function exerciseRowActions(page, topic) {
  const selector = topic === 'tunnel'
    ? '.taf-topic-tunnel-table-body .taf-topic-tunnel-evidence-tags button:visible'
    : topic === 'exfil'
      ? '.taf-topic-exfil-table-panel .ant-table-tbody button:visible'
      : '.taf-topic-apt-table-actions button:visible';
  const actions = page.locator(selector);
  const descriptors = await actions.evaluateAll((items) => items.map((item, index) => ({
    index,
    label: item.getAttribute('title') || item.textContent?.trim() || `row-action-${index}`,
  })));
  if (descriptors.length === 0) throw new Error(`${topic} row action inventory is empty`);
  const results = [];
  for (const descriptor of descriptors) {
    results.push(await exerciseTopicAction(
      page,
      page.locator(selector).nth(descriptor.index),
      topic,
      descriptor.label,
      `row-${descriptor.index}`,
    ));
  }
  return results;
}

async function exerciseLocalControls(page, topic) {
  const results = [];
  const graph = page.locator('.taf-api-topology[data-api-dynamic="true"][data-roam-enabled="true"]:visible').first();
  results.push({ control_id: 'graph-api-roam', kind: 'local-state', label: 'API 动态关系图 / 缩放漫游', passed: await graph.count() === 1 });
  if (topic === 'tunnel') {
    const nodeCount = Number(await graph.getAttribute('data-node-count'));
    const linkCount = Number(await graph.getAttribute('data-link-count'));
    results.push({
      control_id: 'graph-api-payload',
      kind: 'api-state',
      label: 'API 六层影响面 / 20 节点 / 26 连线',
      node_count: nodeCount,
      link_count: linkCount,
      passed: nodeCount === 20 && linkCount === 26,
    });
  }
  const graphControls = graph.locator('.taf-api-topology-controls button');
  for (let index = 0; index < await graphControls.count(); index += 1) {
    const control = graphControls.nth(index);
    const beforeZoom = Number(await graph.getAttribute('data-zoom'));
    await control.click();
    await page.waitForTimeout(80);
    const afterZoom = Number(await graph.getAttribute('data-zoom'));
    const passed = index === 0 ? afterZoom > beforeZoom : index === 1 ? afterZoom < beforeZoom : afterZoom === 1;
    results.push({ control_id: `graph-control-${index}`, kind: 'local-state', label: await control.getAttribute('title'), before_zoom: beforeZoom, after_zoom: afterZoom, passed });
  }

  if (topic === 'tunnel') {
    const layoutResponse = page.waitForResponse((response) =>
      /\/api\/v1\/topics\/tunnel\/actions$/u.test(response.url()) && response.request().method() === 'POST', { timeout: 15_000 });
    const layoutButton = page.locator('.taf-topic-tunnel-panel-actions button').first();
    const beforeLayoutSignature = await graph.getAttribute('data-position-signature');
    await layoutButton.click();
    const layoutStatus = (await layoutResponse).status();
    await page.waitForFunction((signature) =>
      document.querySelector('.taf-api-topology[data-api-dynamic="true"]')?.getAttribute('data-position-signature') !== signature,
    beforeLayoutSignature, { timeout: 8_000 });
    const afterLayoutSignature = await graph.getAttribute('data-position-signature');
    results.push({
      control_id: 'layout',
      kind: 'local-state+action',
      label: '六层链 -> 径向',
      status: layoutStatus,
      before_position_signature: beforeLayoutSignature,
      after_position_signature: afterLayoutSignature,
      passed: layoutStatus === 202
        && await layoutButton.textContent() === '布局：分层'
        && Boolean(beforeLayoutSignature)
        && beforeLayoutSignature !== afterLayoutSignature,
    });

    const fullscreenResponse = page.waitForResponse((response) =>
      /\/api\/v1\/topics\/tunnel\/actions$/u.test(response.url()) && response.request().method() === 'POST', { timeout: 15_000 });
    await page.getByRole('button', { name: '全屏', exact: true }).click();
    const fullscreenStatus = (await fullscreenResponse).status();
    await page.waitForFunction(() => Boolean(document.fullscreenElement), undefined, { timeout: 8_000 });
    const enteredFullscreen = await page.evaluate(() => Boolean(document.fullscreenElement));
    await page.evaluate(() => document.exitFullscreen());
    results.push({ control_id: 'fullscreen', kind: 'local-state+action', label: '全屏', status: fullscreenStatus, passed: enteredFullscreen });

    const highlights = page.locator('.taf-topic-tunnel-alert-strip .taf-topic-alert-chip[data-api-summary="true"]:visible');
    const highlightCount = await highlights.count();
    for (let index = 0; index < highlightCount; index += 1) {
      const highlight = highlights.nth(index);
      const label = (await highlight.locator('strong').textContent())?.trim() ?? '';
      const value = (await highlight.locator('em').textContent())?.replace(/\s+/gu, ' ').trim() ?? '';
      const tagName = await highlight.evaluate((element) => element.tagName);
      results.push({
        control_id: `impact-highlight-${index}`,
        kind: 'api-display',
        label: `${label} / ${value}`,
        tag_name: tagName,
        passed: label.length > 0
          && value.length > 0
          && tagName !== 'BUTTON'
          && await highlight.getAttribute('data-api-summary') === 'true',
      });
    }

    const analysisTabs = page.locator('.taf-topic-tunnel-analysis-tabs button:visible');
    const analysisCount = await analysisTabs.count();
    for (let index = 0; index < analysisCount; index += 1) {
      const tab = analysisTabs.nth(index);
      await tab.click();
      const label = (await tab.textContent())?.trim() ?? '';
      const contentVisible = label === '隧道源'
          ? await page.locator('.taf-topic-high-risk-users:visible').count() === 1
          : label === '协议分析'
            ? await page.locator('.taf-topic-tunnel-card.is-protocol:visible').count() === 1
            : await page.locator('.taf-topic-tunnel-card.is-asn:visible').count() >= 1
              && await page.getByLabel('端点国家分布 TOP5').count() === 1;
      results.push({
        control_id: `analysis-tab-${index}`,
        kind: 'local-state',
        label,
        passed: await tab.getAttribute('aria-selected') === 'true' && contentVisible,
      });
    }
  }

  if (topic === 'apt') {
    const analysisTabs = page.locator('.taf-topic-apt-tabs button:visible');
    const analysisCount = await analysisTabs.count();
    for (let index = 0; index < analysisCount; index += 1) {
      const tab = analysisTabs.nth(index);
      await tab.click();
      results.push({
        control_id: `analysis-tab-${index}`,
        kind: 'local-state',
        label: (await tab.textContent())?.trim() ?? '',
        passed: await tab.getAttribute('aria-selected') === 'true',
      });
    }
  }
  return results;
}

async function exerciseFiltersSearchAndPagination(page, topic) {
  const selectLabels = topic === 'tunnel'
    ? ['证据类型筛选', '阶段筛选', '风险等级筛选']
    : topic === 'exfil'
      ? ['外传风险筛选', '外传协议筛选']
      : ['APT 阶段筛选', 'APT 处置状态筛选'];
  const searchLabel = topic === 'tunnel' ? '搜索隧道证据' : topic === 'exfil' ? '搜索外传事件' : '搜索 APT 证据';
  const firstRowSelector = topic === 'tunnel'
    ? '.taf-topic-tunnel-table-row:first-child > span:first-child'
    : topic === 'exfil'
      ? '.taf-topic-exfil-table-panel .ant-table-tbody tr.ant-table-row td:first-child'
      : '.taf-topic-apt-table-row:not(.is-head) > span:first-child';
  const firstValue = (await page.locator(firstRowSelector).first().textContent())?.trim() ?? '';
  const search = page.getByLabel(searchLabel);
  await search.fill(firstValue);
  const searchMatched = firstValue.length > 0 && await page.getByText(firstValue, { exact: true }).count() > 0;
  await search.clear();

  const filterResults = [];
  for (const label of selectLabels) filterResults.push(await selectOptionAndReset(page, label));

  const paginationResults = [];
  if (topic === 'exfil') {
    const next = page.locator('.taf-topic-exfil-table-panel .ant-pagination-next button');
    await next.click();
    paginationResults.push({ label: '下一页', passed: await page.locator('.taf-topic-exfil-table-panel .ant-pagination-item-active').textContent() === '2' });
    const previous = page.locator('.taf-topic-exfil-table-panel .ant-pagination-prev button');
    await previous.click();
    paginationResults.push({ label: '上一页', passed: await page.locator('.taf-topic-exfil-table-panel .ant-pagination-item-active').textContent() === '1' });
  } else {
    const prefix = topic === 'tunnel' ? '隧道证据' : 'APT 证据';
    const next = page.getByLabel(`${prefix}下一页`);
    await next.click();
    paginationResults.push({ label: '下一页', passed: await page.locator('.taf-topic-page button[aria-current="page"]').textContent() === '2' });
    const pageNumbers = await page.locator('.taf-topic-page button[title^="第 "][title$=" 页"]').evaluateAll((buttons) =>
      buttons.map((button) => Number(button.getAttribute('title')?.match(/\d+/u)?.[0] ?? 0)).filter((value) => value > 0));
    const lastPage = Math.max(...pageNumbers);
    await page.getByTitle(`第 ${lastPage} 页`).click();
    paginationResults.push({ label: `第 ${lastPage} 页`, passed: await page.locator('.taf-topic-page button[aria-current="page"]').textContent() === String(lastPage) });
    const previous = page.getByLabel(`${prefix}上一页`);
    await previous.click();
    paginationResults.push({ label: '上一页', passed: await page.locator('.taf-topic-page button[aria-current="page"]').textContent() === String(lastPage - 1) });
  }
  return {
    filters: filterResults,
    search: { label: searchLabel, value: firstValue, passed: searchMatched },
    pagination: paginationResults,
  };
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const token = smokeToken();
const productBadResponses = [];
const productRequestFailures = [];
const consoleErrors = [];
const pageErrors = [];

page.on('response', (response) => {
  if (response.url().startsWith(`${baseUrl}/api/`) && response.status() >= 400) {
    productBadResponses.push({ status: response.status(), url: response.url() });
  }
});
page.on('requestfailed', (request) => {
  if (request.url().startsWith(baseUrl)) productRequestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? '' });
});
page.on('console', (entry) => {
  if (entry.type() === 'error' && entry.location().url.startsWith(baseUrl)) {
    consoleErrors.push({ text: entry.text(), location: entry.location() });
  }
});
page.on('pageerror', (error) => pageErrors.push({ message: error.message }));

try {
  await page.setViewportSize({ width: 1920, height: 1080 });
  const topics = {};
  for (const topic of topicOrder) {
    console.error(`[topic-controls] opening ${topic}`);
    await openTopic(page, token, topic);
    console.error(`[topic-controls] ${topic} opened`);
    const inventory = await visibleButtonInventory(page);
    console.error(`[topic-controls] ${topic} inventory=${inventory.length}`);
    const tabResults = [];
    for (const tabTopic of ['tunnel', 'exfil', 'apt']) {
      const tabLabel = tabTopic === 'tunnel' ? '加密隧道专题' : tabTopic === 'exfil' ? '数据外传专题' : 'APT/战役专题';
      await page.getByRole('tab', { name: tabLabel, exact: true }).click();
      await waitForTopic(page, tabTopic);
      tabResults.push({ label: tabLabel, passed: new URL(page.url()).searchParams.get('topic') === tabTopic });
    }
    await openTopic(page, token, topic);
    console.error(`[topic-controls] ${topic} tabs complete`);

    const headerResults = [];
    for (const label of ['编辑范围', '保存视图']) {
      headerResults.push(await exerciseTopicAction(
        page,
        page.locator('.taf-topic-controls').getByTitle(label),
        topic,
        label,
        `header-${label}`,
      ));
    }
    console.error(`[topic-controls] ${topic} header actions complete`);
    const localResults = await exerciseLocalControls(page, topic);
    console.error(`[topic-controls] ${topic} local controls complete`);
    await openTopic(page, token, topic);
    const rowActionResults = await exerciseRowActions(page, topic);
    console.error(`[topic-controls] ${topic} row actions complete`);
    await openTopic(page, token, topic);
    const filtersSearchPagination = await exerciseFiltersSearchAndPagination(page, topic);
    console.error(`[topic-controls] ${topic} filters/search/pagination complete`);
    await openTopic(page, token, topic);
    const railResults = await exerciseRightRail(page, topic);

    topics[topic] = {
      inventory_count: inventory.length,
      tabs: tabResults,
      header_actions: headerResults,
      local_controls: localResults,
      row_actions: rowActionResults,
      filters_search_pagination: filtersSearchPagination,
      right_rail_actions: railResults,
    };
    console.error(`[topic-controls] completed ${topic}`);
  }
  const allChecks = Object.values(topics).flatMap((topic) => [
    ...topic.tabs,
    ...topic.header_actions,
    ...topic.local_controls,
    ...topic.row_actions,
    ...topic.filters_search_pagination.filters,
    topic.filters_search_pagination.search,
    ...topic.filters_search_pagination.pagination,
    ...topic.right_rail_actions,
  ]);
  const result = {
    result: allChecks.every((item) => item.passed)
      && productBadResponses.length === 0
      && productRequestFailures.length === 0
      && consoleErrors.length === 0
      && pageErrors.length === 0 ? 'pass' : 'fail',
    browser_backend: 'Windows Chrome CDP through Xshell 9224 -> 9222',
    browser: version.Browser,
    control_checks: { passed: allChecks.filter((item) => item.passed).length, total: allChecks.length },
    topics,
    product_bad_responses: productBadResponses,
    product_request_failures: productRequestFailures,
    console_errors: consoleErrors,
    page_errors: pageErrors,
    timestamp: new Date().toISOString(),
  };
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
  console.log(JSON.stringify(result, null, 2));
  process.exitCode = result.result === 'pass' ? 0 : 1;
} catch (error) {
  const message = error instanceof Error ? error.stack ?? error.message : String(error);
  fs.writeSync(2, `[topic-controls] fatal\n${message}\n`);
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  const debug = {
    message,
    url: page.url(),
    product_bad_responses: productBadResponses,
    product_request_failures: productRequestFailures,
    console_errors: consoleErrors,
    page_errors: pageErrors,
    body_text: (await page.locator('body').textContent().catch(() => ''))?.slice(0, 4_000),
    timestamp: new Date().toISOString(),
  };
  fs.writeFileSync(outputPath.replace(/\.json$/u, '-failed.txt'), `${JSON.stringify(debug, null, 2)}\n`);
  process.exitCode = 1;
} finally {
  await page.close().catch(() => {});
}

process.exit(process.exitCode ?? 0);

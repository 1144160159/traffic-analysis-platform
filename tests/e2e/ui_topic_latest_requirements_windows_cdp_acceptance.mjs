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
const revision = process.env.TOPIC_REVISION ?? 'topic-panel-latest-requirements';
const evidenceRoot = path.join(root, 'evidence/ui-image-breakdowns/pages');
const outputPath = path.join(evidenceRoot, `topics-latest-requirements-${revision}.json`);

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
    username: 'codex-topic-latest-requirements-admin',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'topic:read', 'topic:write', 'topic:export', 'audit:read'],
    token_type: 'access',
    session_id: `codex-topic-latest-${revision}`,
    iat: now,
    exp: now + 3_600,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

async function openTopic(page, token, topic, viewport) {
  await page.setViewportSize(viewport);
  const url = new URL(`/topics?topic=${topic}&tab=${topic}&latestRequirementsTs=${Date.now()}`, baseUrl);
  url.hash = `codex_smoke_token=${token}`;
  await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator(`.taf-topic-${topic}-layout`).waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForFunction((key) => {
    const topicPage = document.querySelector('.taf-topic-page');
    return Boolean(topicPage
      && !topicPage.textContent?.includes('真实 API 数据加载失败')
      && topicPage.textContent?.includes(key));
  }, topic === 'apt' ? '关联战役数' : topic === 'exfil' ? '外传路径数' : '隧道协议数', { timeout: 20_000 });
  await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => {});
  await page.waitForTimeout(500);
}

async function capture(page, directory, filename, fullPage = false) {
  const file = path.join(evidenceRoot, directory, `${filename}-${revision}.png`);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  await page.screenshot({ path: file, fullPage, timeout: 60_000 });
  return path.relative(root, file);
}

async function inspectTopology(page, rootSelector, screenshotDirectory, name) {
  const host = page.locator(`${rootSelector} .taf-api-topology`).first();
  await host.waitFor({ state: 'visible', timeout: 10_000 });
  const before = await host.evaluate((element) => ({
    frameContract: element.getAttribute('data-node-frame-contract'),
    nodeGutterContract: element.getAttribute('data-node-gutter-contract'),
    nodeLayoutCapacityViolationCount: Number(element.getAttribute('data-node-layout-capacity-violation-count')),
    edgeAnchorContract: element.getAttribute('data-edge-anchor-contract'),
    edgeSourceContract: element.getAttribute('data-edge-source-contract'),
    edgeLineContract: element.getAttribute('data-edge-line-contract'),
    edgeSymbolSize: element.getAttribute('data-edge-symbol-size'),
    edgeFrameIntrusionCount: Number(element.getAttribute('data-edge-frame-intrusion-count')),
    edgeAnchorSignature: element.getAttribute('data-edge-anchor-signature'),
    visualProfile: element.getAttribute('data-visual-profile'),
    nodeCount: Number(element.getAttribute('data-node-count')),
    linkCount: Number(element.getAttribute('data-link-count')),
    dashedLinkCount: Number(element.getAttribute('data-dashed-link-count')),
    duplicateNodeCount: Number(element.getAttribute('data-duplicate-node-count')),
    danglingLinkCount: Number(element.getAttribute('data-dangling-link-count')),
    selfLinkCount: Number(element.getAttribute('data-self-link-count')),
    nodeOverlapCount: Number(element.getAttribute('data-node-overlap-count')),
    nodeMinimumGap: Number(element.getAttribute('data-node-minimum-gap')),
    nodeProximityCount: Number(element.getAttribute('data-node-proximity-count')),
    nodeContentOverflowCount: Number(element.getAttribute('data-node-content-overflow-count')),
    nodeInsetViolationCount: Number(element.getAttribute('data-node-inset-violation-count')),
    chartSize: element.getAttribute('data-chart-size'),
    safePositionSignature: element.getAttribute('data-safe-position-signature'),
    frameSizeSignature: element.getAttribute('data-frame-size-signature'),
    nodeLabelSignature: element.getAttribute('data-node-label-signature'),
    nodeIconSignature: element.getAttribute('data-node-icon-signature'),
    linkToneSignature: element.getAttribute('data-link-tone-signature'),
    curvedLinkCount: Number(element.getAttribute('data-curved-link-count')),
    linkCurvenessSignature: element.getAttribute('data-link-curveness-signature'),
    rightAssetColumnContract: element.getAttribute('data-right-asset-column-contract'),
    zoom: Number(element.getAttribute('data-zoom')),
    overflow: getComputedStyle(element).overflow,
  }));
  const frameHeights = (before.frameSizeSignature ?? '')
    .split('|')
    .map((item) => Number(item.split('x').at(-1)))
    .filter(Number.isFinite);
  const positions = new Map((before.safePositionSignature ?? '').split('|').flatMap((item) => {
    const [id, coordinates] = item.split(':');
    const [x, y] = (coordinates ?? '').split(',').map(Number);
    return id && Number.isFinite(x) && Number.isFinite(y) ? [[id, { x, y }]] : [];
  }));
  const frameSizes = new Map((before.frameSizeSignature ?? '').split('|').flatMap((item) => {
    const [id, dimensions] = item.split(':');
    const [width, height] = (dimensions ?? '').split('x').map(Number);
    return id && Number.isFinite(width) && Number.isFinite(height) ? [[id, { width, height }]] : [];
  }));
  const chartWidth = Number((before.chartSize ?? '').split('x')[0]);
  const campaignPosition = positions.get('campaign-0');
  const initialPosition = positions.get('phase-initial');
  const campaignSize = frameSizes.get('campaign-0');
  const initialSize = frameSizes.get('phase-initial');
  const aptCampaignPhaseGap = campaignPosition && initialPosition && campaignSize && initialSize
    ? ((initialPosition.x - campaignPosition.x) / 100 * Math.max(1, chartWidth - 36))
      - campaignSize.width / 2
      - initialSize.width / 2
    : null;
  for (let index = 0; index < 4; index += 1) await host.getByTitle('放大关系图').click();
  await page.waitForTimeout(300);
  const zoomed = Number(await host.getAttribute('data-zoom'));
  for (let index = 0; index < 8; index += 1) await host.getByTitle('缩小关系图').click();
  await page.waitForTimeout(300);
  const reduced = Number(await host.getAttribute('data-zoom'));
  await host.getByTitle('自动适配关系图').click();
  await page.waitForTimeout(300);
  const reset = Number(await host.getAttribute('data-zoom'));
  const screenshot = await capture(page, screenshotDirectory, `${name}-topology-frame-contract`);
  return {
    ...before,
    minNodeFrameHeight: frameHeights.length ? Math.min(...frameHeights) : 0,
    aptCampaignPhaseGap,
    zoomed,
    reduced,
    reset,
    screenshot,
    passed: before.frameContract === 'roundRect-inside-responsive'
      && before.edgeAnchorContract === 'rect-boundary-anchor-per-link'
      && before.edgeSourceContract === 'source-marker-outside-frame'
      && before.edgeFrameIntrusionCount === 0
      && Boolean(before.edgeAnchorSignature)
      && (before.visualProfile === 'apt-reference'
        ? before.edgeLineContract === 'reference-colored-dashed-source-dot-target-arrow'
          && before.edgeSymbolSize === '3x9'
          && before.nodeCount === 22
          && before.linkCount === 19
          && before.dashedLinkCount === 19
          && before.curvedLinkCount >= 4
          && (aptCampaignPhaseGap ?? -1) >= 14
          && before.rightAssetColumnContract === 'assets-after-evidence'
          && [
            'evidence-audit>asset-group', 'evidence-pcap>asset-account',
            'evidence-domain>asset-service', 'evidence-site>asset-evidence',
          ].every((value) => before.linkCurvenessSignature?.includes(value))
          && [
            'campaign-0:campaign', 'phase-initial:initial', 'phase-execute:execute',
            'phase-persist:persist', 'phase-evasion:evasion', 'phase-credential:credential',
            'phase-discovery:discovery', 'phase-lateral:lateral', 'phase-c2:c2',
            'phase-exfil:exfil', 'evidence-audit:audit', 'asset-evidence:evidence',
          ].every((value) => before.nodeIconSignature?.includes(value))
          && before.nodeLabelSignature?.includes('evidence-audit:日志 / 审计:134 条')
          && before.linkToneSignature?.includes('phase-exfil>evidence-audit:warn:dashed')
        : before.edgeLineContract === 'continuous-solid-boundary-target-arrow'
          && before.edgeSymbolSize === '1x10')
      && before.nodeCount > 0
      && before.linkCount > 0
      && before.duplicateNodeCount === 0
      && before.danglingLinkCount === 0
      && before.selfLinkCount === 0
      && before.nodeOverlapCount === 0
      && before.nodeProximityCount === 0
      && before.nodeGutterContract === 'minimum-13px-responsive-two-axis'
      && before.nodeLayoutCapacityViolationCount === 0
      && before.nodeMinimumGap >= 12.5
      && before.nodeContentOverflowCount === 0
      && before.nodeInsetViolationCount === 0
      && (name !== 'latest-exfil' || (frameHeights.length === 25 && Math.min(...frameHeights) >= 56))
      && /^\d+x\d+$/u.test(before.chartSize ?? '')
      && Boolean(before.safePositionSignature)
      && Boolean(before.frameSizeSignature)
      && before.overflow === 'hidden'
      && zoomed > before.zoom
      && reduced < zoomed
      && reset === 1,
  };
}

async function inspectExfilCanvasStructure(page) {
  return page.locator('.taf-topic-exfil-canvas').evaluate((canvas) => {
    const rect = (element) => {
      const value = element.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const expectedHeadings = [
      '内部源资产',
      '文件服务 / 共享',
      '代理 / 中转节点',
      '外部目的地（按国家）',
      '风险路径（TOP）',
    ];
    const headings = [...canvas.querySelectorAll('.taf-topic-exfil-stage-headings > span')]
      .map((element) => element.textContent?.trim() ?? '');
    const summary = canvas.querySelector('.taf-topic-sankey-summary');
    const graph = canvas.querySelector('.taf-api-topology');
    const canvasRect = rect(canvas);
    const summaryRect = summary ? rect(summary) : null;
    const graphRect = graph ? rect(graph) : null;
    const summaryText = summary?.textContent?.replace(/\s+/gu, ' ').trim() ?? '';
    const summaryInsideCanvas = Boolean(summaryRect
      && summaryRect.left >= canvasRect.left - 1
      && summaryRect.right <= canvasRect.right + 1
      && summaryRect.top >= canvasRect.top - 1
      && summaryRect.bottom <= canvasRect.bottom + 1);
    const summaryVisible = Boolean(summary && getComputedStyle(summary).visibility !== 'hidden'
      && getComputedStyle(summary).display !== 'none' && (summaryRect?.height ?? 0) >= 24);
    const passed = JSON.stringify(headings) === JSON.stringify(expectedHeadings)
      && summaryVisible
      && summaryInsideCanvas
      && summaryText.includes('总外传流量')
      && summaryText.includes('涉及路径')
      && summaryText.includes('闭环可信度')
      && (graphRect?.height ?? 0) >= 300;
    return {
      headings,
      canvasRect,
      summaryRect,
      graphRect,
      summaryText,
      summaryVisible,
      summaryInsideCanvas,
      passed,
    };
  });
}

async function inspectDeliveryDonut(page, selector) {
  return page.locator(selector).evaluate((host) => {
    const toRect = (element) => {
      const value = element.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const isIntersecting = (left, right, tolerance = 0.5) =>
      Math.max(0, Math.min(left.right, right.right) - Math.max(left.left, right.left)) > tolerance
      && Math.max(0, Math.min(left.bottom, right.bottom) - Math.max(left.top, right.top)) > tolerance;
    const ring = host.querySelector('.taf-topic-progress-donut__ring');
    const value = host.querySelector('.taf-topic-progress-donut__value');
    const caption = host.querySelector('.taf-topic-progress-donut__caption');
    const ringBounds = toRect(ring);
    const valueBounds = toRect(value);
    const captionBounds = toRect(caption);
    return {
      centerValueCount: ring.querySelectorAll('.taf-topic-progress-donut__value').length,
      centerText: value.textContent?.trim() ?? '',
      captionText: caption.textContent?.trim() ?? '',
      ring: ringBounds,
      value: valueBounds,
      caption: captionBounds,
      captionRingOverlap: isIntersecting(captionBounds, ringBounds),
      captionBelowRing: captionBounds.top >= ringBounds.bottom - 0.5,
    };
  });
}

async function inspectDeliverySummaryScale(page, panelSelector) {
  return page.locator(panelSelector).evaluate((panel) => {
    const toRect = (element) => {
      const value = element.getBoundingClientRect();
      return {
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const grid = panel.querySelector('[data-responsive-summary-contract]');
    const ring = panel.querySelector('.taf-topic-progress-donut__ring');
    const stats = panel.querySelector('.taf-topic-exfil-delivery-stats, .taf-topic-tunnel-delivery-stats');
    const row = stats.querySelector('span');
    const label = row.querySelector('b');
    const value = row.querySelector('strong');
    const centerValue = panel.querySelector('.taf-topic-progress-donut__value');
    return {
      viewport: { width: window.innerWidth, height: window.innerHeight },
      contract: grid.getAttribute('data-responsive-summary-contract'),
      panel: toRect(panel),
      grid: toRect(grid),
      ring: toRect(ring),
      stats: toRect(stats),
      labelFont: Number.parseFloat(getComputedStyle(label).fontSize),
      valueFont: Number.parseFloat(getComputedStyle(value).fontSize),
      centerFont: Number.parseFloat(getComputedStyle(centerValue).fontSize),
      labelText: label.textContent?.trim() ?? '',
      valueText: value.textContent?.trim() ?? '',
    };
  });
}

function deliverySummaryResponsive(base, wide) {
  return {
    at1920x1080: base,
    atWideViewport: wide,
    passed: base.contract === 'ring-legend-values-container-proportional'
      && wide.contract === 'ring-legend-values-container-proportional'
      && wide.panel.width > base.panel.width
      && wide.grid.width > base.grid.width
      && wide.ring.width > base.ring.width
      && wide.ring.height > base.ring.height
      && wide.stats.width > base.stats.width
      && wide.labelFont > base.labelFont
      && wide.valueFont > base.valueFont
      && wide.centerFont > base.centerFont
      && Boolean(base.labelText)
      && Boolean(base.valueText),
  };
}

async function inspectExfilTable(page, viewport) {
  await openTopic(page, token, 'exfil', viewport);
  const host = page.locator('.taf-topic-exfil-table-host');
  await host.waitFor({ state: 'visible', timeout: 10_000 });
  const result = await host.evaluate((element, currentViewport) => {
    const toRect = (target) => {
      const value = target.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const rows = element.querySelectorAll('.ant-table-tbody tr.ant-table-row');
    const body = element.querySelector('.ant-table-body');
    const pager = element.querySelector('.ant-table-pagination');
    const panel = element.closest('.taf-topic-exfil-table-panel');
    const bodyBounds = toRect(body);
    const pagerBounds = toRect(pager);
    const panelBounds = toRect(panel);
    const beforeTop = pagerBounds.top;
    body.scrollTop = Math.min(56, Math.max(0, body.scrollHeight - body.clientHeight));
    const afterTop = toRect(pager).top;
    return {
      viewport: currentViewport,
      pageSizeContract: element.getAttribute('data-page-size'),
      scrollOwnerContract: element.getAttribute('data-scroll-owner'),
      renderedRows: rows.length,
      bodyClientHeight: body.clientHeight,
      bodyScrollHeight: body.scrollHeight,
      bodyOverflowY: getComputedStyle(body).overflowY,
      panelOverflowY: getComputedStyle(panel.querySelector('.taf-panel__body')).overflowY,
      bodyBounds,
      pagerBounds,
      panelBounds,
      pagerInsidePanel: pagerBounds.left >= panelBounds.left - 1
        && pagerBounds.right <= panelBounds.right + 1
        && pagerBounds.bottom <= panelBounds.bottom + 1,
      pagerBelowBody: pagerBounds.top >= bodyBounds.bottom - 1,
      pagerStableOnBodyScroll: Math.abs(afterTop - beforeTop) <= 0.5,
    };
  }, viewport);
  result.passed = result.pageSizeContract === '10'
    && result.scrollOwnerContract === 'tbody'
    && result.renderedRows === 10
    && ['auto', 'scroll'].includes(result.bodyOverflowY)
    && result.panelOverflowY === 'hidden'
    && result.pagerInsidePanel
    && result.pagerBelowBody
    && result.pagerStableOnBodyScroll;
  return result;
}

async function inspectExfilAnalysisLayout(page) {
  return page.locator('.taf-topic-exfil-dashboard').evaluate((dashboard) => {
    const toRect = (element) => {
      const value = element.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const expectedSlots = ['destination', 'trend', 'confidence', 'sensitive', 'protocol', 'account-service'];
    const slots = Object.fromEntries(expectedSlots.map((slot) => {
      const card = dashboard.querySelector(`[data-layout-slot="${slot}"]`);
      return [slot, card ? toRect(card) : null];
    }));
    const titles = Object.fromEntries(expectedSlots.map((slot) => {
      const card = dashboard.querySelector(`[data-layout-slot="${slot}"]`);
      return [slot, card?.querySelector('header strong')?.textContent?.replace(/\s+/gu, ' ').trim() ?? ''];
    }));
    const sensitiveChart = dashboard.querySelector('[data-layout-slot="sensitive"] .taf-topic-exfil-donut-layout > div:first-child');
    const protocolChart = dashboard.querySelector('[data-layout-slot="protocol"] .taf-topic-exfil-donut-layout > div:first-child');
    const trendChart = dashboard.querySelector('[data-layout-slot="trend"] canvas');
    const accountChart = dashboard.querySelector('[data-layout-slot="account-service"] canvas');
    const confidenceRing = dashboard.querySelector('[data-layout-slot="confidence"] .taf-topic-progress-donut__ring');
    const modulePanel = dashboard.closest('.taf-topic-exfil-analysis-panel');
    const moduleTitle = modulePanel?.querySelector(':scope > .taf-panel__header h2')?.textContent?.replace(/\s+/gu, ' ').trim() ?? '';
    const chartBounds = {
      sensitive: sensitiveChart ? toRect(sensitiveChart) : null,
      protocol: protocolChart ? toRect(protocolChart) : null,
      trend: trendChart ? toRect(trendChart) : null,
      accountService: accountChart ? toRect(accountChart) : null,
      confidence: confidenceRing ? toRect(confidenceRing) : null,
    };
    const estimatedDonutDiameters = {
      sensitive: chartBounds.sensitive ? Math.min(chartBounds.sensitive.width, chartBounds.sensitive.height) * 0.76 : 0,
      protocol: chartBounds.protocol ? Math.min(chartBounds.protocol.width, chartBounds.protocol.height) * 0.76 : 0,
    };
    const sameColumn = (items) => Math.max(...items.map((item) => item.left)) - Math.min(...items.map((item) => item.left)) <= 1;
    const heights = expectedSlots.map((slot) => slots[slot]?.height ?? 0);
    const passed = expectedSlots.every((slot) => Boolean(slots[slot]))
      && sameColumn([slots.destination, slots.trend, slots.confidence])
      && sameColumn([slots.sensitive, slots.protocol, slots['account-service']])
      && slots.destination.left < slots.sensitive.left
      && slots.destination.top < slots.trend.top
      && slots.trend.top < slots.confidence.top
      && slots.sensitive.top < slots.protocol.top
      && slots.protocol.top < slots['account-service'].top
      && Math.min(...heights) >= 80
      && titles.destination.includes('目的地国家')
      && titles.sensitive === '敏感数据类型分布'
      && titles.protocol === '外传协议占比'
      && titles.trend === '异常上传峰值趋势'
      && titles.confidence === '路径置信度评分'
      && titles['account-service'].includes('可疑账号')
      && dashboard.getAttribute('data-analysis-module-contract') === 'destination-sensitive-trend-protocol-confidence'
      && moduleTitle === '数据外传分析'
      && Object.values(chartBounds).every((bounds) => bounds && bounds.width >= 68 && bounds.height >= 60)
      && Object.values(estimatedDonutDiameters).every((diameter) => diameter >= 64);
    return {
      moduleTitle,
      moduleContract: dashboard.getAttribute('data-analysis-module-contract'),
      slots,
      titles,
      chartBounds,
      estimatedDonutDiameters,
      passed,
    };
  });
}

async function inspectExfilAnalysisScale(page) {
  return page.locator('.taf-topic-exfil-dashboard').evaluate((dashboard) => {
    const toRect = (element) => {
      const value = element.getBoundingClientRect();
      return {
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const sensitive = dashboard.querySelector('[data-layout-slot="sensitive"]');
    const protocol = dashboard.querySelector('[data-layout-slot="protocol"]');
    const sensitiveChart = sensitive.querySelector('.taf-topic-exfil-donut-layout > div:first-child');
    const protocolChart = protocol.querySelector('.taf-topic-exfil-donut-layout > div:first-child');
    const sensitiveLegend = sensitive.querySelector('.taf-topic-exfil-legend span');
    const protocolLegend = protocol.querySelector('.taf-topic-exfil-legend span');
    return {
      viewport: { width: window.innerWidth, height: window.innerHeight },
      sensitiveChart: toRect(sensitiveChart),
      protocolChart: toRect(protocolChart),
      sensitiveLegendFont: Number.parseFloat(getComputedStyle(sensitiveLegend).fontSize),
      protocolLegendFont: Number.parseFloat(getComputedStyle(protocolLegend).fontSize),
      sensitiveLegendWidth: toRect(sensitive.querySelector('.taf-topic-exfil-legend')).width,
      protocolLegendWidth: toRect(protocol.querySelector('.taf-topic-exfil-legend')).width,
    };
  });
}

async function inspectAptResponseScale(page) {
  return page.locator('.taf-topic-apt-response-panel').evaluate((panel) => {
    const toRect = (element) => {
      const value = element.getBoundingClientRect();
      return {
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const chart = panel.querySelector('.taf-topic-apt-response-chart');
    const legend = panel.querySelector('.taf-topic-apt-response > div:last-child');
    const legendRow = legend.querySelector('span');
    const value = legendRow.querySelector('em');
    return {
      viewport: { width: window.innerWidth, height: window.innerHeight },
      chart: toRect(chart),
      legend: toRect(legend),
      legendFont: Number.parseFloat(getComputedStyle(legendRow).fontSize),
      valueFont: Number.parseFloat(getComputedStyle(value).fontSize),
      centerText: panel.querySelector('.taf-topic-apt-response-center')?.textContent?.replace(/\s+/gu, ' ').trim() ?? '',
      contract: chart.getAttribute('data-responsive-chart-contract'),
    };
  });
}

async function inspectPreviewButton(page) {
  return page.locator('.taf-topic-report-preview-trigger').first().evaluate((button) => {
    const style = getComputedStyle(button);
    return {
      text: button.textContent?.replace(/\s+/gu, ' ').trim() ?? '',
      backgroundColor: style.backgroundColor,
      backgroundImage: style.backgroundImage,
      borderColor: style.borderColor,
      color: style.color,
      className: button.className,
      passed: button.textContent?.includes('预览报告')
        && button.classList.contains('ant-btn-primary')
        && button.classList.contains('taf-topic-report-preview-trigger')
        && style.backgroundImage !== 'none'
        && style.borderColor !== 'rgba(0, 0, 0, 0)'
        && style.color !== 'rgba(0, 0, 0, 0)',
    };
  });
}

async function inspectTunnelWorkspace(page) {
  return page.locator('.taf-topic-tunnel-boardline').evaluate((boardline) => {
    const toRect = (element) => {
      const value = element.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const impact = boardline.querySelector('.taf-topic-tunnel-impact-panel');
    const analysis = boardline.querySelector('.taf-topic-tunnel-analysis');
    const topology = boardline.querySelector('.taf-api-topology');
    const boardlineBounds = toRect(boardline);
    const impactBounds = toRect(impact);
    const analysisBounds = toRect(analysis);
    const impactWidthRatio = impactBounds.width / Math.max(1, impactBounds.width + analysisBounds.width);
    return {
      viewport: { width: window.innerWidth, height: window.innerHeight },
      boardline: boardlineBounds,
      impact: impactBounds,
      analysis: analysisBounds,
      impactWidthRatio,
      topologyChartSize: topology?.getAttribute('data-chart-size') ?? '',
      nodeContentOverflowCount: Number(topology?.getAttribute('data-node-content-overflow-count')),
      nodeInsetViolationCount: Number(topology?.getAttribute('data-node-inset-violation-count')),
      passed: boardlineBounds.height >= 420
        && impactBounds.height >= 420
        && analysisBounds.height >= 420
        && impactBounds.width >= 600
        && analysisBounds.width >= 480
        && impactWidthRatio >= 0.56
        && impactWidthRatio <= 0.61
        && Number(topology?.getAttribute('data-node-content-overflow-count')) === 0
        && Number(topology?.getAttribute('data-node-inset-violation-count')) === 0,
    };
  });
}

async function inspectAptTabLayout(page, label) {
  return page.locator('.taf-topic-apt-analysis-panel').evaluate((panel, currentLabel) => {
    const toRect = (element) => {
      const value = element.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const boardline = panel.closest('.taf-topic-apt-boardline');
    const body = panel.querySelector('.taf-panel__body');
    const grid = panel.querySelector('.taf-topic-apt-analysis-grid');
    const boardlineBounds = toRect(boardline);
    const panelBounds = toRect(panel);
    const bodyBounds = toRect(body);
    const gridBounds = toRect(grid);
    const cards = [...grid.querySelectorAll('[data-business-view]')];
    const cardBounds = cards.map(toRect);
    const usedArea = cardBounds.reduce((sum, item) => sum + item.width * item.height, 0);
    const usedAreaRatio = usedArea / Math.max(1, gridBounds.width * gridBounds.height);
    const lastCard = cardBounds.at(-1);
    const lastCardSpans = currentLabel === 'ATT&CK阶段覆盖'
      || (lastCard
        && Math.abs(lastCard.left - gridBounds.left) <= 1
        && Math.abs(lastCard.right - gridBounds.right) <= 1);
    const evidenceList = grid.querySelector('[data-business-view="evidence-completeness"] > .taf-topic-exfil-evidence-list');
    const evidenceRows = evidenceList ? [...evidenceList.children] : [];
    const evidenceReadable = !evidenceList || (
      evidenceList.scrollWidth <= evidenceList.clientWidth + 1
      && evidenceRows.length >= 6
      && evidenceRows.every((row) => row.scrollWidth <= row.clientWidth + 1)
    );
    return {
      label: currentLabel,
      activeTab: grid.getAttribute('data-active-tab'),
      geometryContract: grid.getAttribute('data-tab-geometry-contract'),
      boardline: boardlineBounds,
      panel: panelBounds,
      body: bodyBounds,
      grid: gridBounds,
      relativeGeometry: {
        panel: {
          left: panelBounds.left - boardlineBounds.left,
          top: panelBounds.top - boardlineBounds.top,
        },
        body: {
          left: bodyBounds.left - panelBounds.left,
          top: bodyBounds.top - panelBounds.top,
        },
        grid: {
          left: gridBounds.left - panelBounds.left,
          top: gridBounds.top - panelBounds.top,
        },
      },
      cards: cardBounds,
      usedAreaRatio,
      lastCardSpans,
      evidenceReadable,
      passed: cards.length >= 3
        && grid.getAttribute('data-active-tab') === currentLabel
        && grid.getAttribute('data-tab-geometry-contract') === 'fixed-within-viewport'
        && usedAreaRatio >= 0.9
        && lastCardSpans
        && evidenceReadable,
    };
  }, label);
}

function aptTabGeometryStable(tabs, viewport) {
  const dimensionFields = ['width', 'height'];
  const offsetFields = ['left', 'top'];
  const baseline = tabs[0]?.layout;
  const deltas = tabs.map((item) => ({
    label: item.label,
    panel: Object.fromEntries(dimensionFields.map((field) => [
      field,
      Math.round(Math.abs(item.layout.panel[field] - baseline.panel[field]) * 100) / 100,
    ])),
    body: Object.fromEntries(dimensionFields.map((field) => [
      field,
      Math.round(Math.abs(item.layout.body[field] - baseline.body[field]) * 100) / 100,
    ])),
    grid: Object.fromEntries(dimensionFields.map((field) => [
      field,
      Math.round(Math.abs(item.layout.grid[field] - baseline.grid[field]) * 100) / 100,
    ])),
    relativeGeometry: Object.fromEntries(['panel', 'body', 'grid'].map((section) => [
      section,
      Object.fromEntries(offsetFields.map((field) => [
        field,
        Math.round(Math.abs(
          item.layout.relativeGeometry[section][field]
          - baseline.relativeGeometry[section][field],
        ) * 100) / 100,
      ])),
    ])),
  }));
  const passed = Boolean(baseline)
    && tabs.length >= 6
    && tabs.every((item) => item.layout.geometryContract === 'fixed-within-viewport')
    && deltas.every((item) => [
      item.panel,
      item.body,
      item.grid,
      ...Object.values(item.relativeGeometry),
    ].every((section) => Object.values(section).every((delta) => delta <= 1)));
  return { viewport, baseline, deltas, passed };
}

async function inspectAptTabGeometryAtViewport(page, labels, viewport) {
  await page.setViewportSize(viewport);
  await page.waitForTimeout(400);
  const tabs = [];
  for (const label of labels) {
    await page.getByRole('tab', { name: label, exact: true }).click();
    await page.waitForTimeout(180);
    tabs.push({ label, layout: await inspectAptTabLayout(page, label) });
  }
  return aptTabGeometryStable(tabs, viewport);
}

async function inspectAptEvidencePagination(page) {
  const host = page.locator('.taf-topic-apt-evidence-table');
  await host.waitFor({ state: 'attached', timeout: 10_000 });
  const firstPage = await host.evaluate((element) => ({
    pageSize: element.getAttribute('data-page-size'),
    currentPage: element.getAttribute('data-current-page'),
    renderedRowCount: Number(element.getAttribute('data-rendered-row-count')),
    rowCount: element.querySelectorAll('.taf-topic-apt-table-row').length,
    firstRow: element.querySelector('.taf-topic-apt-table-row > span')?.textContent?.trim() ?? '',
    footerText: element.querySelector('.taf-topic-apt-table-footer')?.textContent?.replace(/\s+/gu, ' ').trim() ?? '',
  }));
  await page.getByLabel('APT 证据下一页').click();
  await page.waitForFunction((selector) => document.querySelector(selector)?.getAttribute('data-current-page') === '2', '.taf-topic-apt-evidence-table');
  const secondPage = await host.evaluate((element) => ({
    currentPage: element.getAttribute('data-current-page'),
    renderedRowCount: Number(element.getAttribute('data-rendered-row-count')),
    rowCount: element.querySelectorAll('.taf-topic-apt-table-row').length,
    firstRow: element.querySelector('.taf-topic-apt-table-row > span')?.textContent?.trim() ?? '',
  }));
  const passed = firstPage.pageSize === '10'
    && firstPage.currentPage === '1'
    && firstPage.renderedRowCount === 10
    && firstPage.rowCount === 10
    && firstPage.footerText.includes('10 条/页')
    && secondPage.currentPage === '2'
    && secondPage.renderedRowCount === 10
    && secondPage.rowCount === 10
    && secondPage.firstRow
    && secondPage.firstRow !== firstPage.firstRow;
  return { firstPage, secondPage, passed };
}

async function inspectTopicOuterGeometry(page, topic) {
  await openTopic(page, token, topic, { width: 1920, height: 1080 });
  return page.locator(`.taf-topic-${topic}-layout`).evaluate((layout, currentTopic) => {
    const rect = (element) => {
      const value = element.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const left = layout.querySelector(`.taf-topic-${currentTopic}-left`);
    const title = left.querySelector('.taf-topic-titlebar');
    const tabs = left.querySelector('.taf-topic-tabs');
    const facts = left.querySelector('.taf-topic-facts');
    const main = left.querySelector(`.taf-topic-${currentTopic}-main`);
    const rail = layout.querySelector(`.taf-topic-${currentTopic}-rail`);
    const shell = layout.closest('.taf-topic-shell');
    return {
      topic: currentTopic,
      shell: rect(shell),
      layout: rect(layout),
      left: rect(left),
      title: rect(title),
      tabs: rect(tabs),
      facts: rect(facts),
      main: rect(main),
      rail: rect(rail),
    };
  }, topic);
}

function topicOuterGeometryStable(geometries) {
  const baseline = geometries[0];
  const stableKeys = ['shell', 'layout', 'left', 'title', 'tabs', 'facts', 'main', 'rail'];
  const comparableFields = {
    shell: ['left', 'top', 'width'],
    layout: ['left', 'top', 'width'],
    left: ['left', 'top', 'width'],
    title: ['left', 'top', 'width', 'height'],
    tabs: ['left', 'top', 'height'],
    facts: ['left', 'top', 'height'],
    main: ['left', 'top'],
    rail: ['top', 'right', 'width', 'height'],
  };
  const deltas = Object.fromEntries(geometries.slice(1).map((geometry) => [
    `${baseline.topic}->${geometry.topic}`,
    Object.fromEntries(stableKeys.map((key) => [
      key,
      Object.fromEntries(comparableFields[key].map((field) => [
        field,
        Math.round(Math.abs(geometry[key][field] - baseline[key][field]) * 100) / 100,
      ])),
    ])),
  ]));
  const passed = Object.values(deltas).every((comparison) =>
    Object.values(comparison).every((section) =>
      Object.values(section).every((delta) => delta <= 1)));
  return { geometries, deltas, passed };
}

async function inspectDeliveryStack(page, selector) {
  return page.locator(selector).evaluate((panel) => {
    const rect = (element) => {
      const value = element.getBoundingClientRect();
      return {
        top: Math.round(value.top * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        left: Math.round(value.left * 100) / 100,
        right: Math.round(value.right * 100) / 100,
      };
    };
    const body = panel.querySelector('.taf-panel__body');
    const grid = panel.querySelector('[data-responsive-summary-contract]');
    const caption = panel.querySelector('.taf-topic-progress-donut__caption');
    const actions = panel.querySelector('.taf-topic-tunnel-delivery-actions, .taf-topic-exfil-delivery-actions');
    const panelBounds = rect(panel);
    const bodyBounds = rect(body);
    const gridBounds = rect(grid);
    const captionBounds = rect(caption);
    const actionBounds = rect(actions);
    return {
      panel: panelBounds,
      body: bodyBounds,
      grid: gridBounds,
      caption: captionBounds,
      actions: actionBounds,
      captionAboveActions: captionBounds.bottom <= actionBounds.top - 1,
      gridAboveActions: gridBounds.bottom <= actionBounds.top - 1,
      buttonsAtBottom: bodyBounds.bottom - actionBounds.bottom <= 8,
      actionsInsidePanel: actionBounds.left >= panelBounds.left
        && actionBounds.right <= panelBounds.right
        && actionBounds.bottom <= panelBounds.bottom,
    };
  }).then((value) => ({
    ...value,
    passed: value.captionAboveActions
      && value.gridAboveActions
      && value.buttonsAtBottom
      && value.actionsInsidePanel,
  }));
}

async function inspectTableSeparators(page, selector, cellSelector) {
  return page.locator(selector).evaluate((host, currentCellSelector) => {
    const cells = [...host.querySelectorAll(currentCellSelector)];
    const widths = cells.slice(0, Math.min(cells.length, 8)).map((cell) =>
      Number.parseFloat(getComputedStyle(cell).borderRightWidth));
    return {
      contract: host.getAttribute('data-column-separators'),
      cellCount: cells.length,
      borderRightWidths: widths,
      passed: host.getAttribute('data-column-separators') === 'visible'
        && cells.length >= 7
        // Windows Chrome is rendered at 150% DPI, so a one-CSS-pixel border is
        // reported as 0.6667 layout pixels while remaining one physical pixel.
        && widths.slice(0, -1).every((width) => width >= 0.5),
    };
  }, cellSelector);
}

async function inspectAptToolbar(page) {
  return page.locator('.taf-topic-apt-table-toolbar').evaluate((toolbar) => {
    const controls = [...toolbar.children];
    const values = controls.slice(0, 2).map((control) => {
      const item = control.querySelector('.ant-select-selection-item');
      return {
        width: Math.round(control.getBoundingClientRect().width * 100) / 100,
        text: item?.textContent?.replace(/\s+/gu, ' ').trim() ?? '',
        readable: Boolean(item) && item.scrollWidth <= item.clientWidth + 1,
      };
    });
    const searchWidth = Math.round(controls[2].getBoundingClientRect().width * 100) / 100;
    return {
      values,
      searchWidth,
      passed: values.every((value) => value.width >= 150 && value.readable && value.text.length > 0)
        && searchWidth >= 185,
    };
  });
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP /json/version preflight failed');
const listResponse = await fetch(`${cdpUrl}/json/list`);
if (!listResponse.ok) throw new Error('Windows Chrome CDP /json/list preflight failed');
const version = await versionResponse.json();
const tabList = await listResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
for (const stalePage of context.pages()) {
  if (stalePage.url().includes('latestRequirementsTs=')) await stalePage.close().catch(() => {});
}
const page = await context.newPage();
const token = smokeToken();
const productBadResponses = [];
const requestFailures = [];
const consoleErrors = [];
const pageErrors = [];

page.on('response', async (response) => {
  if (response.url().startsWith(`${baseUrl}/api/`) && response.status() >= 400) {
    productBadResponses.push({
      status: response.status(),
      url: response.url(),
      body: (await response.text().catch(() => '')).slice(0, 800),
    });
  }
});
page.on('requestfailed', (request) => {
  if (request.url().startsWith(baseUrl)) requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? '' });
});
page.on('console', (entry) => {
  if (entry.type() === 'error' && (!entry.location().url || entry.location().url.startsWith(baseUrl))) {
    consoleErrors.push({ text: entry.text(), location: entry.location() });
  }
});
page.on('pageerror', (error) => pageErrors.push({ message: error.message, stack: error.stack ?? '' }));
await page.route('https://api.yhchj.com/ip', (route) => route.fulfill({
  status: 200,
  contentType: 'application/json',
  body: JSON.stringify({ ip: '127.0.0.1', source: 'windows-cdp-latest-requirements' }),
}));

let exitCode = 1;
try {
  const exfilLarge = await inspectExfilTable(page, { width: 1920, height: 1300 });
  const exfilCompact = await inspectExfilTable(page, { width: 1920, height: 768 });
  await openTopic(page, token, 'exfil', { width: 1920, height: 1080 });
  const exfilAnalysisLayout = await inspectExfilAnalysisLayout(page);
  const exfilAnalysisScaleBase = await inspectExfilAnalysisScale(page);
  const exfilDeliveryScaleBase = await inspectDeliverySummaryScale(page, '.taf-topic-exfil-delivery');
  const exfilPreviewButton = await inspectPreviewButton(page);
  await page.setViewportSize({ width: 2048, height: 1152 });
  await page.waitForTimeout(500);
  const exfilAnalysisScaleWide = await inspectExfilAnalysisScale(page);
  const exfilDeliveryScaleWide = await inspectDeliverySummaryScale(page, '.taf-topic-exfil-delivery');
  const exfilDeliveryResponsive = deliverySummaryResponsive(exfilDeliveryScaleBase, exfilDeliveryScaleWide);
  const exfilAnalysisResponsive = {
    at1920x1080: exfilAnalysisScaleBase,
    at2048x1152: exfilAnalysisScaleWide,
    passed: exfilAnalysisScaleWide.sensitiveChart.width > exfilAnalysisScaleBase.sensitiveChart.width
      && exfilAnalysisScaleWide.sensitiveChart.height > exfilAnalysisScaleBase.sensitiveChart.height
      && exfilAnalysisScaleWide.protocolChart.width > exfilAnalysisScaleBase.protocolChart.width
      && exfilAnalysisScaleWide.protocolChart.height > exfilAnalysisScaleBase.protocolChart.height
      && exfilAnalysisScaleWide.sensitiveLegendWidth > exfilAnalysisScaleBase.sensitiveLegendWidth
      && exfilAnalysisScaleWide.protocolLegendWidth > exfilAnalysisScaleBase.protocolLegendWidth
      && exfilAnalysisScaleWide.sensitiveLegendFont > exfilAnalysisScaleBase.sensitiveLegendFont
      && exfilAnalysisScaleWide.protocolLegendFont > exfilAnalysisScaleBase.protocolLegendFont,
  };
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.waitForTimeout(400);
  const exfilTopology = await inspectTopology(page, '.taf-topic-exfil-canvas', 'topics-data-exfiltration', 'latest-exfil');
  const exfilCanvasStructure = await inspectExfilCanvasStructure(page);
  const exfilDonut = await inspectDeliveryDonut(page, '.taf-topic-exfil-delivery .taf-topic-progress-donut');
  const exfilDeliveryStack = await inspectDeliveryStack(page, '.taf-topic-exfil-delivery');
  const exfilTableSeparators = await inspectTableSeparators(
    page,
    '.taf-topic-exfil-table-host',
    '.ant-table-thead > tr:first-child > th',
  );
  exfilDonut.passed = exfilDonut.centerValueCount === 1
    && /^\d+%$/u.test(exfilDonut.centerText)
    && exfilDonut.captionText.length > 0
    && !exfilDonut.captionRingOverlap
    && exfilDonut.captionBelowRing;
  const exfilScreenshot = await capture(page, 'topics-data-exfiltration', 'latest-fixed-ten-table-and-delivery', true);

  await openTopic(page, token, 'tunnel', { width: 1920, height: 1080 });
  const tunnelWorkspace = await inspectTunnelWorkspace(page);
  const tunnelDeliveryScaleBase = await inspectDeliverySummaryScale(page, '.taf-topic-tunnel-delivery');
  const tunnelTopology = await inspectTopology(page, '.taf-topic-tunnel-impact-panel', 'topics-encrypted-tunnel', 'latest-tunnel');
  const tunnelDonut = await inspectDeliveryDonut(page, '.taf-topic-tunnel-delivery .taf-topic-progress-donut');
  const tunnelDeliveryStack = await inspectDeliveryStack(page, '.taf-topic-tunnel-delivery');
  const tunnelTableSeparators = await inspectTableSeparators(
    page,
    '.taf-topic-tunnel-table',
    '.taf-topic-tunnel-table-head > b',
  );
  tunnelDonut.passed = tunnelDonut.centerValueCount === 1
    && /^\d+%$/u.test(tunnelDonut.centerText)
    && tunnelDonut.captionText.length > 0
    && !tunnelDonut.captionRingOverlap
    && tunnelDonut.captionBelowRing;
  await page.setViewportSize({ width: 2048, height: 1024 });
  await page.waitForTimeout(500);
  const tunnelWorkspaceWide = await inspectTunnelWorkspace(page);
  const tunnelDeliveryScaleWide = await inspectDeliverySummaryScale(page, '.taf-topic-tunnel-delivery');
  const tunnelDeliveryResponsive = deliverySummaryResponsive(tunnelDeliveryScaleBase, tunnelDeliveryScaleWide);
  const widthScale = tunnelWorkspaceWide.impact.width / Math.max(1, tunnelWorkspace.impact.width);
  const heightScale = tunnelWorkspaceWide.impact.height / Math.max(1, tunnelWorkspace.impact.height);
  const tunnelResponsiveWorkspace = {
    at1920x1080: tunnelWorkspace,
    at2048x1024: tunnelWorkspaceWide,
    widthScale,
    heightScale,
    proportionalDelta: Math.abs(widthScale - heightScale),
    screenshot: await capture(page, 'topics-encrypted-tunnel', 'latest-tunnel-responsive-proportional'),
    passed: tunnelWorkspace.passed
      && tunnelWorkspaceWide.passed
      && tunnelWorkspaceWide.impact.width > tunnelWorkspace.impact.width
      && tunnelWorkspaceWide.impact.height > tunnelWorkspace.impact.height
      && Math.abs(widthScale - heightScale) <= 0.04,
  };
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.waitForTimeout(400);
  const tunnelTabLabels = await page.locator('.taf-topic-tunnel-analysis-tabs button').allTextContents();
  await page.getByRole('tab', { name: '隧道源', exact: true }).click();
  await page.waitForTimeout(200);
  const sourceViews = await page.locator('.taf-topic-tunnel-analysis-grid[data-active-tab="source"] [data-business-view]').count();
  const sourceApiSources = await page.locator('.taf-topic-tunnel-analysis-grid[data-active-tab="source"] [data-business-view]').evaluateAll(
    (elements) => elements.map((element) => element.getAttribute('data-api-source') ?? ''),
  );
  const sourceScreenshot = await capture(page, 'topics-encrypted-tunnel', 'latest-tunnel-source-business');
  await page.getByRole('tab', { name: '端点国家分布', exact: true }).click();
  await page.waitForTimeout(200);
  const destinationViews = await page.locator('.taf-topic-tunnel-analysis-grid[data-active-tab="destination"] [data-business-view]').count();
  const destinationApiSources = await page.locator('.taf-topic-tunnel-analysis-grid[data-active-tab="destination"] [data-business-view]').evaluateAll(
    (elements) => elements.map((element) => element.getAttribute('data-api-source') ?? ''),
  );
  const destinationScreenshot = await capture(page, 'topics-encrypted-tunnel', 'latest-tunnel-destination-business');
  const tunnelTabs = {
    labels: tunnelTabLabels.map((item) => item.replace(/\s+/gu, ' ').trim()),
    sourceViews,
    sourceApiSources,
    destinationViews,
    destinationApiSources,
    sourceScreenshot,
    destinationScreenshot,
  };
  tunnelTabs.passed = tunnelTabs.labels.includes('隧道源')
    && !tunnelTabs.labels.some((item) => item.includes('TOP5'))
    && sourceViews >= 4
    && destinationViews >= 4
    && sourceApiSources.every(Boolean)
    && destinationApiSources.every(Boolean);

  await openTopic(page, token, 'apt', { width: 1920, height: 1080 });
  const aptEvidencePagination = await inspectAptEvidencePagination(page);
  const aptTopology = await inspectTopology(page, '.taf-topic-apt-canvas', 'topics-apt-campaign', 'latest-apt');
  const aptToolbar = await inspectAptToolbar(page);
  const aptResponseScaleBase = await inspectAptResponseScale(page);
  const aptDeliveryScaleBase = await inspectDeliverySummaryScale(page, '.taf-topic-apt-delivery');
  const aptDeliveryStack = await inspectDeliveryStack(page, '.taf-topic-apt-delivery');
  const aptPreviewButton = await inspectPreviewButton(page);
  await page.setViewportSize({ width: 2048, height: 1152 });
  await page.waitForTimeout(500);
  const aptResponseScaleWide = await inspectAptResponseScale(page);
  const aptDeliveryScaleWide = await inspectDeliverySummaryScale(page, '.taf-topic-apt-delivery');
  const aptDeliveryResponsive = deliverySummaryResponsive(aptDeliveryScaleBase, aptDeliveryScaleWide);
  const aptResponseResponsive = {
    at1920x1080: aptResponseScaleBase,
    at2048x1152: aptResponseScaleWide,
    passed: aptResponseScaleBase.contract === 'container-proportional'
      && aptResponseScaleWide.contract === 'container-proportional'
      && aptResponseScaleWide.chart.width > aptResponseScaleBase.chart.width
      && aptResponseScaleWide.chart.height > aptResponseScaleBase.chart.height
      && aptResponseScaleWide.legend.width > aptResponseScaleBase.legend.width
      && aptResponseScaleWide.legendFont > aptResponseScaleBase.legendFont
      && aptResponseScaleWide.valueFont > aptResponseScaleBase.valueFont
      && aptResponseScaleBase.centerText.includes('总计')
      && aptResponseScaleWide.centerText.includes('总计'),
  };
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.waitForTimeout(400);
  const aptDonut = await inspectDeliveryDonut(page, '.taf-topic-apt-delivery .taf-topic-progress-donut');
  aptDonut.passed = aptDonut.centerValueCount === 1
    && /^\d+%$/u.test(aptDonut.centerText)
    && aptDonut.captionText.length > 0
    && !aptDonut.captionRingOverlap
    && aptDonut.captionBelowRing;
  const aptTabLabels = await page.locator('.taf-topic-apt-tabs button').allTextContents();
  const aptTabButtonsReadable = await page.locator('.taf-topic-apt-tabs button').evaluateAll(
    (buttons) => buttons.every((button) => button.scrollWidth <= button.clientWidth + 1),
  );
  const aptTabs = [];
  for (const rawLabel of aptTabLabels) {
    const label = rawLabel.replace(/\s+/gu, ' ').trim();
    await page.getByRole('tab', { name: label, exact: true }).click();
    await page.waitForTimeout(180);
    const activePanel = page.locator('.taf-topic-apt-analysis-grid');
    const views = await activePanel.locator('[data-business-view]').count();
    const apiSources = await activePanel.locator('[data-business-view]').evaluateAll(
      (elements) => elements.map((element) => element.getAttribute('data-api-source') ?? ''),
    );
    const layout = await inspectAptTabLayout(page, label);
    let semantics = null;
    if (label === 'ATT&CK阶段覆盖') {
      semantics = await activePanel.evaluate((element) => {
        const tacticIds = [...element.querySelectorAll('.taf-topic-apt-matrix > b')]
          .slice(1)
          .map((cell) => cell.childNodes[0]?.textContent?.trim() ?? '');
        const trend = element.querySelector('[data-business-view="campaign-event-trend"]');
        const firstSeenValues = [...element.querySelectorAll('[data-business-view="ioc-top5"] [data-column="first-seen"]')]
          .map((cell) => cell.textContent?.trim() ?? '');
        return {
          tacticIds,
          timelinePointCount: Number(trend?.getAttribute('data-timeline-point-count')),
          visibleEventCount: Number(trend?.getAttribute('data-visible-event-count')),
          totalEventCount: Number(trend?.getAttribute('data-total-event-count')),
          firstSeenValues,
          passed: JSON.stringify(tacticIds) === JSON.stringify([
            'TA0001', 'TA0002', 'TA0003', 'TA0005', 'TA0006', 'TA0007', 'TA0008', 'TA0011', 'TA0010',
          ])
            && Number(trend?.getAttribute('data-timeline-point-count')) >= 7
            && Number(trend?.getAttribute('data-visible-event-count')) > 0
            && Number(trend?.getAttribute('data-total-event-count')) >= Number(trend?.getAttribute('data-visible-event-count'))
            && firstSeenValues.length >= 5
            && firstSeenValues.every((value) => /^\d{2}-\d{2} \d{2}:\d{2}$/u.test(value)),
        };
      });
    }
    if (label === '关键 IoC 命中') {
      semantics = await activePanel.evaluate((element) => {
        const campaigns = [...element.querySelectorAll('[data-business-view="ioc-campaign"] [data-column="campaign"]')]
          .map((cell) => cell.textContent?.trim() ?? '');
        return {
          campaigns,
          passed: campaigns.length >= 5 && campaigns.every((value) => value && !/^\d{2}-\d{2}/u.test(value)),
        };
      });
    }
    aptTabs.push({
      label,
      views,
      apiSources,
      layout,
      semantics,
      passed: views >= 3 && apiSources.every(Boolean) && layout.passed && (semantics?.passed ?? true),
      screenshot: await capture(page, 'topics-apt-campaign', `latest-apt-tab-${aptTabs.length + 1}`),
    });
  }
  const aptTabsPass = aptTabs.length >= 6 && aptTabButtonsReadable && aptTabs.every((item) => item.passed);
  const aptTabGeometry1920 = aptTabGeometryStable(aptTabs, { width: 1920, height: 1080 });
  const aptTabGeometry1366 = await inspectAptTabGeometryAtViewport(
    page,
    aptTabLabels.map((item) => item.replace(/\s+/gu, ' ').trim()),
    { width: 1366, height: 768 },
  );
  const aptTabGeometryScreenshot = await capture(
    page,
    'topics-apt-campaign',
    'latest-apt-tab-fixed-geometry-1366',
    true,
  );
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.waitForTimeout(400);
  const aptScreenshot = await capture(page, 'topics-apt-campaign', 'latest-apt-delivery-and-analysis', true);
  const outerGeometry = topicOuterGeometryStable([
    await inspectTopicOuterGeometry(page, 'tunnel'),
    await inspectTopicOuterGeometry(page, 'exfil'),
    await inspectTopicOuterGeometry(page, 'apt'),
  ]);
  const rightRailLayout = {
    deliveryStacks: {
      tunnel: tunnelDeliveryStack,
      exfil: exfilDeliveryStack,
      apt: aptDeliveryStack,
    },
    railHeights: outerGeometry.geometries.map((item) => ({ topic: item.topic, height: item.rail.height })),
    passed: tunnelDeliveryStack.passed
      && exfilDeliveryStack.passed
      && aptDeliveryStack.passed
      && Math.max(...outerGeometry.geometries.map((item) => item.rail.height))
        - Math.min(...outerGeometry.geometries.map((item) => item.rail.height)) <= 1,
  };

  const runtime = {
    productBadResponses,
    requestFailures,
    consoleErrors,
    pageErrors,
    passed: productBadResponses.length === 0
      && requestFailures.length === 0
      && consoleErrors.length === 0
      && pageErrors.length === 0,
  };
  const assertions = {
    exfilFixedTenLarge: exfilLarge.passed,
    exfilFixedTenCompact: exfilCompact.passed,
    exfilAnalysisLayout: exfilAnalysisLayout.passed,
    exfilAnalysisResponsive: exfilAnalysisResponsive.passed,
    exfilPreviewButton: exfilPreviewButton.passed,
    exfilDeliveryResponsive: exfilDeliveryResponsive.passed,
    topicOuterGeometryStable: outerGeometry.passed,
    rightRailLayoutUnified: rightRailLayout.passed,
    tunnelTableSeparators: tunnelTableSeparators.passed,
    exfilTableSeparators: exfilTableSeparators.passed,
    exfilCanvasStructure: exfilCanvasStructure.passed,
    aptToolbarReadable: aptToolbar.passed,
    aptEvidenceFixedTen: aptEvidencePagination.passed,
    exfilTopology: exfilTopology.passed,
    tunnelTopology: tunnelTopology.passed,
    tunnelWorkspace: tunnelWorkspace.passed,
    tunnelResponsiveWorkspace: tunnelResponsiveWorkspace.passed,
    tunnelDeliveryResponsive: tunnelDeliveryResponsive.passed,
    aptTopology: aptTopology.passed,
    exfilDeliveryDonut: exfilDonut.passed,
    tunnelDeliveryDonut: tunnelDonut.passed,
    aptDeliveryDonut: aptDonut.passed,
    aptResponseResponsive: aptResponseResponsive.passed,
    aptPreviewButton: aptPreviewButton.passed,
    aptDeliveryResponsive: aptDeliveryResponsive.passed,
    tunnelBusinessTabs: tunnelTabs.passed,
    aptBusinessTabs: aptTabsPass,
    aptAnalysisTabGeometryStable: aptTabGeometry1920.passed && aptTabGeometry1366.passed,
    runtime: runtime.passed,
  };
  const passed = Object.values(assertions).every(Boolean);
  const payload = {
    generated_at: new Date().toISOString(),
    source: 'Windows Chrome via Xshell/CDP 127.0.0.1:9224 -> Windows 9222',
    cdp: { browser: version.Browser, protocol_version: version['Protocol-Version'], tab_count: tabList.length },
    revision,
    target: baseUrl,
    passed,
    assertions,
    exfil: {
      large: exfilLarge,
      compact: exfilCompact,
      analysisLayout: exfilAnalysisLayout,
      analysisResponsive: exfilAnalysisResponsive,
      previewButton: exfilPreviewButton,
      deliveryResponsive: exfilDeliveryResponsive,
      deliveryStack: exfilDeliveryStack,
      tableSeparators: exfilTableSeparators,
      topology: exfilTopology,
      canvasStructure: exfilCanvasStructure,
      deliveryDonut: exfilDonut,
      screenshot: exfilScreenshot,
    },
    tunnel: {
      workspace: tunnelWorkspace,
      responsiveWorkspace: tunnelResponsiveWorkspace,
      topology: tunnelTopology,
      deliveryDonut: tunnelDonut,
      deliveryResponsive: tunnelDeliveryResponsive,
      deliveryStack: tunnelDeliveryStack,
      tableSeparators: tunnelTableSeparators,
      tabs: tunnelTabs,
    },
    apt: {
      evidencePagination: aptEvidencePagination,
      toolbar: aptToolbar,
      topology: aptTopology,
      deliveryDonut: aptDonut,
      responseResponsive: aptResponseResponsive,
      previewButton: aptPreviewButton,
      deliveryResponsive: aptDeliveryResponsive,
      deliveryStack: aptDeliveryStack,
      tabButtonsReadable: aptTabButtonsReadable,
      tabs: aptTabs,
      tabGeometry: {
        at1920x1080: aptTabGeometry1920,
        at1366x768: aptTabGeometry1366,
        compactScreenshot: aptTabGeometryScreenshot,
      },
      screenshot: aptScreenshot,
    },
    outerGeometry,
    rightRailLayout,
    runtime,
  };
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  fs.writeFileSync(outputPath, `${JSON.stringify(payload, null, 2)}\n`);
  console.log(JSON.stringify(payload, null, 2));
  exitCode = passed ? 0 : 1;
} finally {
  await page.close().catch(() => {});
  await browser.close().catch(() => {});
}

process.exitCode = exitCode;

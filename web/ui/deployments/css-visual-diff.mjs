import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { chromium } from 'playwright-core';

const baselineOrigin = process.env.BASELINE_ORIGIN;
const candidateOrigin = process.env.CANDIDATE_ORIGIN;
const outputDir = process.env.OUTPUT_DIR || '/tmp/visual-diff';

if (!baselineOrigin || !candidateOrigin) {
  throw new Error('BASELINE_ORIGIN and CANDIDATE_ORIGIN are required');
}

const fixture = `
  <main class="taf-page taf-alert-detail-page is-visual-target">
    <section class="taf-panel">
      <header class="taf-panel__header"><h2>处置与响应</h2></header>
      <div class="taf-panel__body">
        <div class="taf-alert-detail-response">
          <button class="is-risk"><span>阻断源地址</span><em>高风险</em></button>
          <button class="is-warn"><span>隔离资产</span><em>需审批</em></button>
          <button class="is-info"><span>下发规则</span><em>灰度验证</em></button>
          <button><span>创建工单</span><em>人工交接</em></button>
          <button><span>导出证据</span><em>可追溯</em></button>
          <p>推荐动作必须经过权限、审批和外部 provider receipt 闭环。</p>
        </div>
      </div>
    </section>
    <section class="taf-panel taf-alert-detail-feedback-panel">
      <header class="taf-panel__header"><h2>反馈与学习</h2></header>
      <div class="taf-panel__body">
        <div class="taf-alert-detail-feedback">
          <label><span>标签</span><span class="ant-radio-group"><label class="ant-radio-wrapper">TP</label><label class="ant-radio-wrapper">FP</label></span></label>
          <div class="taf-alert-detail-feedback-inline"><label class="ant-checkbox-wrapper">同步进入规则复审</label><button class="ant-btn">提交反馈</button></div>
          <label class="taf-alert-detail-feedback-comment"><span>原因</span><textarea class="ant-input">长文本验证：可疑资产与告警证据需要完整保留上下文。</textarea></label>
          <div class="taf-alert-detail-feedback-actions"><span class="taf-alert-detail-feedback-pending-hint">待确认是未决状态</span><button class="ant-btn">确认提交</button></div>
        </div>
      </div>
    </section>
  </main>`;

const fixtureStyle = `
  html, body { margin: 0; width: 100%; min-height: 100%; background: #03111c; }
  body { padding: 24px; box-sizing: border-box; }
  .taf-alert-detail-page { width: min(1120px, calc(100vw - 48px)); margin: 0 auto; }
`;

const sha256 = (buffer) => createHash('sha256').update(buffer).digest('hex');

async function stylesheetUrl(origin) {
  const response = await fetch(`${origin}/`);
  if (!response.ok) throw new Error(`cannot load ${origin}: HTTP ${response.status}`);
  const html = await response.text();
  const match = html.match(/href="([^\"]*\/assets\/index-[^\"]+\.css)"/);
  if (!match) throw new Error(`cannot resolve compiled stylesheet from ${origin}`);
  return new URL(match[1], origin).toString();
}

async function capture(browser, origin, viewport, name) {
  const cssUrl = await stylesheetUrl(origin);
  const page = await browser.newPage({ viewport, deviceScaleFactor: 1 });
  await page.setContent(`<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="${cssUrl}"><style>${fixtureStyle}</style></head><body>${fixture}</body></html>`, { waitUntil: 'networkidle' });
  await page.evaluate(async () => {
    await document.fonts.ready;
    const link = document.querySelector('link[rel="stylesheet"]');
    if (!(link instanceof HTMLLinkElement) || !link.sheet) throw new Error('compiled stylesheet was not applied');
  });
  const styles = await page.evaluate(() => {
    const pick = (selector, properties) => {
      const element = document.querySelector(selector);
      if (!(element instanceof HTMLElement)) throw new Error(`missing fixture element ${selector}`);
      const computed = getComputedStyle(element);
      return Object.fromEntries(properties.map((property) => [property, computed.getPropertyValue(property)]));
    };
    return {
      response: pick('.taf-alert-detail-response', ['display', 'grid-template-columns', 'gap']),
      risk: pick('.taf-alert-detail-response .is-risk', ['min-height', 'display', 'color', 'background-color', 'border-color', 'border-radius']),
      feedback: pick('.taf-alert-detail-feedback', ['display', 'gap']),
      feedbackLabel: pick('.taf-alert-detail-feedback > label', ['display', 'grid-template-columns', 'gap', 'align-items']),
      feedbackText: pick('.taf-alert-detail-feedback textarea', ['height', 'min-height', 'resize']),
    };
  });
  const image = await page.screenshot({ fullPage: true, animations: 'disabled' });
  await writeFile(`${outputDir}/${name}-${viewport.width}x${viewport.height}.png`, image);
  await page.close();
  return { css_url: cssUrl, screenshot_sha256: sha256(image), screenshot_bytes: image.length, computed_styles: styles, image };
}

await mkdir(outputDir, { recursive: true });
const browser = await chromium.launch({
  executablePath: '/usr/bin/chromium',
  headless: true,
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-crash-reporter', '--disable-crashpad', '--noerrdialogs'],
});
const results = [];
try {
  for (const viewport of [{ width: 1366, height: 900 }, { width: 1600, height: 900 }]) {
    const baseline = await capture(browser, baselineOrigin, viewport, 'baseline');
    const candidate = await capture(browser, candidateOrigin, viewport, 'candidate');
    const screenshotEqual = baseline.image.equals(candidate.image);
    const computedStylesEqual = JSON.stringify(baseline.computed_styles) === JSON.stringify(candidate.computed_styles);
    results.push({
      viewport,
      screenshot_equal: screenshotEqual,
      computed_styles_equal: computedStylesEqual,
      baseline: { css_url: baseline.css_url, screenshot_sha256: baseline.screenshot_sha256, screenshot_bytes: baseline.screenshot_bytes, computed_styles: baseline.computed_styles },
      candidate: { css_url: candidate.css_url, screenshot_sha256: candidate.screenshot_sha256, screenshot_bytes: candidate.screenshot_bytes, computed_styles: candidate.computed_styles },
    });
  }
} finally {
  await browser.close();
}

const passed = results.every((result) => result.screenshot_equal && result.computed_styles_equal);
console.log(JSON.stringify({ status: passed ? 'PASS' : 'FAIL', exact_pixel_diff: true, results }));
if (!passed) process.exitCode = 1;

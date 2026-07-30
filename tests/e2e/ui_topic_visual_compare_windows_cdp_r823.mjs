#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const requireFromUi = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = requireFromUi('@playwright/test');
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
const cdpUrl = process.env.CDP_URL ?? 'http://127.0.0.1:9224';
const revision = process.env.TOPIC_REVISION ?? 'topic-panel-r824';
const tolerance = Number(process.env.CHANNEL_TOLERANCE ?? 90);
const maxMismatch = Number(process.env.MAX_MISMATCH_RATIO ?? 0.125);
const requestedTopics = new Set((process.env.TOPIC_SET ?? 'tunnel,exfil,apt').split(',').map((value) => value.trim()).filter(Boolean));
const topics = [
  {
    key: 'tunnel',
    directory: 'topics-encrypted-tunnel',
    source: 'topics-encrypted-tunnel.png',
    sourceBusinessCrop: { x: 198, y: 80, width: 1722, height: 917 },
    implementationBusinessCrop: { x: 190, y: 72, width: 1716, height: 888 },
  },
  {
    key: 'exfil',
    directory: 'topics-data-exfiltration',
    source: 'topics-data-exfiltration.png',
    sourceBusinessCrop: { x: 198, y: 80, width: 1722, height: 917 },
    implementationBusinessCrop: { x: 198, y: 80, width: 1712, height: 917 },
  },
  {
    key: 'apt',
    directory: 'topics-apt-campaign',
    source: 'topics-apt-campaign.png',
    sourceBusinessCrop: { x: 198, y: 80, width: 1722, height: 917 },
    implementationBusinessCrop: { x: 198, y: 80, width: 1712, height: 917 },
  },
].filter((topic) => requestedTopics.has(topic.key));

const asDataUrl = (file) => `data:image/png;base64,${fs.readFileSync(file).toString('base64')}`;
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 620 });
const results = [];

for (const topic of topics) {
  const sourceBusinessCrop = topic.sourceBusinessCrop;
  const implementationBusinessCrop = topic.implementationBusinessCrop;
  const sourcePath = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages', topic.source);
  const implementationPath = path.join(root, 'evidence/ui-image-breakdowns/pages', topic.directory, `implementation-${revision}-live.png`);
  const comparisonPath = path.join(root, 'evidence/ui-image-breakdowns/pages', topic.directory, `comparison-${revision}.png`);
  const businessComparisonPath = path.join(root, 'evidence/ui-image-breakdowns/pages', topic.directory, `comparison-business-${revision}.png`);
  const sourceUrl = asDataUrl(sourcePath);
  const implementationUrl = asDataUrl(implementationPath);
  await page.setContent(`<!doctype html><html><head><style>
    *{box-sizing:border-box}html,body{margin:0;width:1920px;height:620px;overflow:hidden;background:#020b13;color:#dff6ff;font-family:Inter,"Microsoft YaHei",sans-serif}
    header{height:72px;display:grid;grid-template-columns:1fr 1fr;align-items:center;text-align:center;font-size:22px;font-weight:700;background:#071725;border-bottom:1px solid #19445c}
    main{display:grid;grid-template-columns:1fr 1fr;width:1920px;height:540px}
    figure{margin:0;width:960px;height:540px;overflow:hidden;border-right:1px solid #19445c}
    img{display:block;width:960px;height:540px;object-fit:fill}
  </style></head><body><header><span>UI 基准图 / ${topic.key}</span><span>Windows Chrome 实现 / ${revision}</span></header><main><figure><img id="source" src="${sourceUrl}"></figure><figure><img id="implementation" src="${implementationUrl}"></figure></main></body></html>`);
  await page.locator('img').first().waitFor({ state: 'visible' });
  const comparison = await page.evaluate(async ({
    tolerance: pixelTolerance,
    sourceCrop,
    implementationCrop,
  }) => {
    const source = document.querySelector('#source');
    const implementation = document.querySelector('#implementation');
    await Promise.all([source.decode(), implementation.decode()]);
    const measure = (leftPixels, rightPixels, width, height) => {
      let mismatched = 0;
      let absoluteDifference = 0;
      for (let index = 0; index < leftPixels.length; index += 4) {
        const red = Math.abs(leftPixels[index] - rightPixels[index]);
        const green = Math.abs(leftPixels[index + 1] - rightPixels[index + 1]);
        const blue = Math.abs(leftPixels[index + 2] - rightPixels[index + 2]);
        const maximum = Math.max(red, green, blue);
        if (maximum > pixelTolerance) mismatched += 1;
        absoluteDifference += red + green + blue;
      }
      const pixels = width * height;
      return {
        width,
        height,
        channel_tolerance: pixelTolerance,
        mismatched_pixels: mismatched,
        mismatch_ratio: mismatched / pixels,
        normalized_mean_absolute_difference: absoluteDifference / (pixels * 3 * 255),
      };
    };
    const pixelsFor = (image, crop, width, height) => {
      const canvas = document.createElement('canvas');
      canvas.width = width;
      canvas.height = height;
      const context2d = canvas.getContext('2d', { willReadFrequently: true });
      context2d.drawImage(image, crop.x, crop.y, crop.width, crop.height, 0, 0, width, height);
      return { canvas, pixels: context2d.getImageData(0, 0, width, height).data };
    };
    const width = Math.min(source.naturalWidth, implementation.naturalWidth);
    const height = Math.min(source.naturalHeight, implementation.naturalHeight);
    const fullSource = pixelsFor(source, { x: 0, y: 0, width: source.naturalWidth, height: source.naturalHeight }, width, height);
    const fullImplementation = pixelsFor(implementation, { x: 0, y: 0, width: implementation.naturalWidth, height: implementation.naturalHeight }, width, height);
    const businessWidth = sourceCrop.width;
    const businessHeight = sourceCrop.height;
    const businessSource = pixelsFor(source, sourceCrop, businessWidth, businessHeight);
    const businessImplementation = pixelsFor(implementation, implementationCrop, businessWidth, businessHeight);
    return {
      full: measure(fullSource.pixels, fullImplementation.pixels, width, height),
      business: measure(businessSource.pixels, businessImplementation.pixels, businessWidth, businessHeight),
      source_business_url: businessSource.canvas.toDataURL('image/png'),
      implementation_business_url: businessImplementation.canvas.toDataURL('image/png'),
    };
  }, { tolerance, sourceCrop: sourceBusinessCrop, implementationCrop: implementationBusinessCrop });
  await page.screenshot({ path: comparisonPath });
  await page.setContent(`<!doctype html><html><head><style>
    *{box-sizing:border-box}html,body{margin:0;width:1920px;height:620px;overflow:hidden;background:#020b13;color:#dff6ff;font-family:Inter,"Microsoft YaHei",sans-serif}
    header{height:72px;display:grid;grid-template-columns:1fr 1fr;align-items:center;text-align:center;font-size:22px;font-weight:700;background:#071725;border-bottom:1px solid #19445c}
    main{display:grid;grid-template-columns:1fr 1fr;width:1920px;height:540px}
    figure{margin:0;width:960px;height:540px;overflow:hidden;border-right:1px solid #19445c}
    img{display:block;width:960px;height:540px;object-fit:fill}
  </style></head><body><header><span>UI 业务区基准 / ${topic.key}</span><span>归一化业务区实现 / ${revision}</span></header><main><figure><img src="${comparison.source_business_url}"></figure><figure><img src="${comparison.implementation_business_url}"></figure></main></body></html>`);
  await page.locator('img').first().waitFor({ state: 'visible' });
  await page.screenshot({ path: businessComparisonPath });
  results.push({
    ...topic,
    source: path.relative(root, sourcePath),
    implementation: path.relative(root, implementationPath),
    comparison: path.relative(root, comparisonPath),
    business_comparison: path.relative(root, businessComparisonPath),
    source_business_crop: sourceBusinessCrop,
    implementation_business_crop: implementationBusinessCrop,
    ...comparison.full,
    business_metrics: comparison.business,
    passed: comparison.full.mismatch_ratio <= maxMismatch && comparison.business.mismatch_ratio <= maxMismatch,
  });
}

await page.close();
await browser.close();
const report = {
  result: results.every((item) => item.passed) ? 'pass' : 'blocked',
  browser: 'Windows Chrome CDP through Xshell 9224 -> 9222',
  tolerance,
  max_mismatch_ratio: maxMismatch,
  results,
  timestamp: new Date().toISOString(),
};
const reportPath = path.join(root, `evidence/ui-image-breakdowns/pages/topics-visual-compare-${revision}.json`);
fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify(report, null, 2));
process.exitCode = report.result === 'pass' ? 0 : 1;

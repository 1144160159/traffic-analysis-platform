#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const cdpUrl = 'http://127.0.0.1:9224';
const revision = process.env.MODEL_EVIDENCE_REVISION?.trim() || 'r300';
const outputDir = path.join(root, `evidence/ui-image-breakdowns/pages/models/visual-${revision}`);
const region = { id: 'models-full-state-v2', x: 0, y: 0, width: 1920, height: 1080 };
const tolerance = 64;
const cases = [
  { id: 'main', source: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/models.png', actual: `evidence/ui-image-breakdowns/pages/models/interaction-${revision}.png`, maxRatio: 0.125 },
  { id: 'activation-audit-gate', source: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/models-activation-audit-gate.png', actual: `evidence/ui-image-breakdowns/pages/models/state-${revision}-activation-audit-gate.png`, maxRatio: 0.15 },
  { id: 'rule-contribution', source: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/models-feature-rule-contribution.png', actual: `evidence/ui-image-breakdowns/pages/models/state-${revision}-rule-contribution.png`, maxRatio: 0.15 },
  { id: 'anomaly-explanation', source: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/models-feature-anomaly-explanation.png', actual: `evidence/ui-image-breakdowns/pages/models/state-${revision}-anomaly-explanation.png`, maxRatio: 0.15 },
  { id: 'sample-examples', source: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/models-feature-sample-examples.png', actual: `evidence/ui-image-breakdowns/pages/models/state-${revision}-sample-examples.png`, maxRatio: 0.15 },
];

const dataUrl = (filePath) => `data:image/png;base64,${fs.readFileSync(path.join(root, filePath)).toString('base64')}`;
const version = await (await fetch(`${cdpUrl}/json/version`)).json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();

fs.mkdirSync(outputDir, { recursive: true });
const metrics = [];
for (const item of cases) {
  const compared = await page.evaluate(async ({ sourceUrl, actualUrl, scoringRegion, channelTolerance }) => {
    const load = (url) => new Promise((resolve, reject) => {
      const image = new Image();
      image.onload = () => resolve(image);
      image.onerror = reject;
      image.src = url;
    });
    const [source, actual] = await Promise.all([load(sourceUrl), load(actualUrl)]);
    if (source.naturalWidth !== actual.naturalWidth || source.naturalHeight !== actual.naturalHeight) throw new Error('visual inputs have different dimensions');
    const width = source.naturalWidth;
    const height = source.naturalHeight;
    const sourceCanvas = document.createElement('canvas');
    const actualCanvas = document.createElement('canvas');
    const diffCanvas = document.createElement('canvas');
    for (const canvas of [sourceCanvas, actualCanvas, diffCanvas]) { canvas.width = width; canvas.height = height; }
    const sourceContext = sourceCanvas.getContext('2d', { willReadFrequently: true });
    const actualContext = actualCanvas.getContext('2d', { willReadFrequently: true });
    const diffContext = diffCanvas.getContext('2d');
    sourceContext.drawImage(source, 0, 0);
    actualContext.drawImage(actual, 0, 0);
    const sourcePixels = sourceContext.getImageData(0, 0, width, height);
    const actualPixels = actualContext.getImageData(0, 0, width, height);
    const diffPixels = diffContext.createImageData(width, height);
    let roiMismatch = 0;
    let fullMismatch = 0;
    for (let y = 0; y < height; y += 1) {
      for (let x = 0; x < width; x += 1) {
        const index = (y * width + x) * 4;
        const delta = Math.max(Math.abs(sourcePixels.data[index] - actualPixels.data[index]), Math.abs(sourcePixels.data[index + 1] - actualPixels.data[index + 1]), Math.abs(sourcePixels.data[index + 2] - actualPixels.data[index + 2]));
        const mismatch = delta > channelTolerance;
        if (mismatch) fullMismatch += 1;
        if (mismatch && x >= scoringRegion.x && x < scoringRegion.x + scoringRegion.width && y >= scoringRegion.y && y < scoringRegion.y + scoringRegion.height) roiMismatch += 1;
        const gray = Math.round((actualPixels.data[index] + actualPixels.data[index + 1] + actualPixels.data[index + 2]) / 3);
        diffPixels.data[index] = mismatch ? 255 : gray;
        diffPixels.data[index + 1] = mismatch ? 42 : gray;
        diffPixels.data[index + 2] = mismatch ? 164 : gray;
        diffPixels.data[index + 3] = mismatch ? 255 : 90;
      }
    }
    diffContext.putImageData(diffPixels, 0, 0);
    const sideCanvas = document.createElement('canvas');
    sideCanvas.width = width * 2;
    sideCanvas.height = height;
    const sideContext = sideCanvas.getContext('2d');
    sideContext.drawImage(source, 0, 0);
    sideContext.drawImage(actual, width, 0);
    return { width, height, roiMismatch, fullMismatch, diffUrl: diffCanvas.toDataURL('image/png'), sideUrl: sideCanvas.toDataURL('image/png') };
  }, { sourceUrl: dataUrl(item.source), actualUrl: dataUrl(item.actual), scoringRegion: region, channelTolerance: tolerance });
  const decode = (url) => Buffer.from(url.slice(url.indexOf(',') + 1), 'base64');
  const diffPath = path.join(outputDir, `${item.id}-diff.png`);
  const sidePath = path.join(outputDir, `${item.id}-side-by-side.png`);
  fs.writeFileSync(diffPath, decode(compared.diffUrl));
  fs.writeFileSync(sidePath, decode(compared.sideUrl));
  const ratio = compared.roiMismatch / (region.width * region.height);
  metrics.push({
    id: item.id,
    status: ratio <= item.maxRatio ? 'pass' : 'fail',
    source_image: item.source,
    actual_image: item.actual,
    side_by_side_image: path.relative(root, sidePath),
    diff_image: path.relative(root, diffPath),
    viewport: { width: compared.width, height: compared.height },
    scoring_region: region,
    channel_tolerance: tolerance,
    mismatch_pixels: compared.roiMismatch,
    compared_pixels: region.width * region.height,
    mismatch_ratio: ratio,
    max_ratio: item.maxRatio,
    full_image_mismatch_ratio: compared.fullMismatch / (compared.width * compared.height),
  });
}

const report = {
  route: '/models',
  browser_path: 'Xshell tunnel -> 127.0.0.1:9224 -> Windows Chrome',
  browser: version.Browser,
  revision,
  status: metrics.every((item) => item.status === 'pass') ? 'pass' : 'fail',
  cases: metrics,
};
fs.writeFileSync(path.join(outputDir, 'metrics.json'), `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify(report));
await page.close();
await browser.close();
if (report.status !== 'pass') process.exitCode = 1;

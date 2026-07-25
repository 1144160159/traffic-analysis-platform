#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const cdpUrl = process.env.UI_CDP_URL || 'http://127.0.0.1:9224';
const revision = process.env.CAMPAIGN_EVIDENCE_REVISION || 'r730';
const channelTolerance = Number(process.env.CAMPAIGN_VISUAL_CHANNEL_TOLERANCE || '64');
const maxPixelMismatchRatio = Number(process.env.CAMPAIGN_VISUAL_MAX_MISMATCH_RATIO || '0.125');
const targets = [
  ['pages', 'campaigns'],
  ['pages', 'campaign-detail'],
  ['overlays', 'drawer-campaign-detail'],
  ['overlays', 'modal-campaign-report-export'],
  ['pages', 'campaign-detail-impact-account'],
  ['pages', 'campaign-detail-impact-service'],
  ['pages', 'campaign-detail-impact-department'],
  ['pages', 'campaign-detail-impact-campus'],
  ['pages', 'campaign-detail-impact-business-system'],
  ['pages', 'attack-chains'],
  ['overlays', 'drawer-attack-chain-detail'],
];

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];

const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const output = [];
for (const [category, id] of targets) {
  const page = await context.newPage();
  try {
    await page.setViewportSize({ width: 1920, height: 1080 });
    const directory = path.join(root, 'evidence/ui-image-breakdowns', category, id);
    const targetPath = path.join(directory, 'target.png');
    const implementationPath = path.join(directory, 'implementation.png');
    const comparisonPath = path.join(directory, `comparison-${revision}.png`);
    const modalRoi = id.startsWith('campaign-detail-impact-');
    const implementationRoiPath = path.join(directory, 'implementation-roi.png');
    const comparisonRoiPath = path.join(directory, `comparison-roi-${revision}.png`);
    if (!fs.existsSync(targetPath) || !fs.existsSync(implementationPath)) {
      output.push({ id, status: 'missing-input' });
      continue;
    }
    if (modalRoi && !fs.existsSync(implementationRoiPath)) {
      output.push({ id, status: 'missing-modal-roi' });
      continue;
    }
    const compare = async (actualPath, normalizeToActual) => page.evaluate(async ({ target, implementation, channelTolerance, normalizeToActual }) => {
      const load = (source) => new Promise((resolve, reject) => {
        const image = new Image();
        image.onload = () => resolve(image);
        image.onerror = reject;
        image.src = source;
      });
      const [reference, actual] = await Promise.all([load(target), load(implementation)]);
      const width = normalizeToActual ? actual.naturalWidth : reference.naturalWidth;
      const height = normalizeToActual ? actual.naturalHeight : reference.naturalHeight;
      if (!normalizeToActual && (actual.naturalWidth !== width || actual.naturalHeight !== height)) {
        throw new Error(`dimension mismatch: ${width}x${height} vs ${actual.naturalWidth}x${actual.naturalHeight}`);
      }
      const canvas = document.createElement('canvas');
      canvas.width = width * 2;
      canvas.height = height;
      const context2d = canvas.getContext('2d');
      context2d.fillStyle = '#020b13';
      context2d.fillRect(0, 0, canvas.width, canvas.height);
      context2d.drawImage(reference, 0, 0, width, height);
      context2d.drawImage(actual, width, 0, width, height);
      const referenceCanvas = document.createElement('canvas');
      const actualCanvas = document.createElement('canvas');
      referenceCanvas.width = actualCanvas.width = width;
      referenceCanvas.height = actualCanvas.height = height;
      referenceCanvas.getContext('2d').drawImage(reference, 0, 0, width, height);
      actualCanvas.getContext('2d').drawImage(actual, 0, 0, width, height);
      const referencePixels = referenceCanvas.getContext('2d').getImageData(0, 0, width, height).data;
      const actualPixels = actualCanvas.getContext('2d').getImageData(0, 0, width, height).data;
      let mismatched = 0;
      let deltaSum = 0;
      for (let index = 0; index < referencePixels.length; index += 4) {
        const red = Math.abs(referencePixels[index] - actualPixels[index]);
        const green = Math.abs(referencePixels[index + 1] - actualPixels[index + 1]);
        const blue = Math.abs(referencePixels[index + 2] - actualPixels[index + 2]);
        deltaSum += red + green + blue;
        if (Math.max(red, green, blue) > channelTolerance) mismatched += 1;
      }
      return {
        width,
        height,
        mismatchRatio: mismatched / (width * height),
        meanAbsoluteChannelDelta: deltaSum / (width * height * 3),
        dataUrl: canvas.toDataURL('image/png'),
      };
    }, {
      target: `data:image/png;base64,${fs.readFileSync(targetPath).toString('base64')}`,
      implementation: `data:image/png;base64,${fs.readFileSync(actualPath).toString('base64')}`,
      channelTolerance,
      normalizeToActual,
    });
    const fullFrameComparison = await compare(implementationPath, false);
    fs.writeFileSync(comparisonPath, Buffer.from(fullFrameComparison.dataUrl.split(',')[1], 'base64'));
    const gatingComparison = modalRoi
      ? await compare(implementationRoiPath, true)
      : fullFrameComparison;
    if (modalRoi) {
      fs.writeFileSync(comparisonRoiPath, Buffer.from(gatingComparison.dataUrl.split(',')[1], 'base64'));
    }
    output.push({
      id,
      status: gatingComparison.mismatchRatio <= maxPixelMismatchRatio ? 'pass' : 'fail',
      comparison_mode: modalRoi ? 'normalized-modal-roi' : 'full-frame',
      viewport: `${gatingComparison.width}x${gatingComparison.height}`,
      pixel_mismatch_ratio: gatingComparison.mismatchRatio,
      mean_absolute_channel_delta: gatingComparison.meanAbsoluteChannelDelta,
      reference: path.relative(root, targetPath),
      implementation: path.relative(root, modalRoi ? implementationRoiPath : implementationPath),
      comparison: path.relative(root, modalRoi ? comparisonRoiPath : comparisonPath),
      full_frame_diagnostic: modalRoi ? {
        pixel_mismatch_ratio: fullFrameComparison.mismatchRatio,
        mean_absolute_channel_delta: fullFrameComparison.meanAbsoluteChannelDelta,
        implementation: path.relative(root, implementationPath),
        comparison: path.relative(root, comparisonPath),
      } : null,
    });
  } finally {
    await page.close().catch(() => {});
  }
}

const result = output.length === targets.length && output.every((target) => target.status === 'pass') ? 'pass' : 'fail';
const reportPath = path.join(root, `doc/02_acceptance/02-regression/campaign-domain-comparisons-${revision}.json`);
fs.writeFileSync(reportPath, `${JSON.stringify({
  generated_at: new Date().toISOString(),
  browser: 'Windows Chrome over Xshell CDP 9224',
  revision,
  result,
  acceptance: { channel_tolerance: channelTolerance, max_pixel_mismatch_ratio: maxPixelMismatchRatio },
  convention: 'reference on left, implementation on right',
  targets: output,
}, null, 2)}\n`);
console.log(JSON.stringify({ report: path.relative(root, reportPath), result, targets: output.length }, null, 2));
process.exit(result === 'pass' ? 0 : 1);

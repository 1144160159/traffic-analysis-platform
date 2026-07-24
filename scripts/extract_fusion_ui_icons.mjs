#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import crypto from 'node:crypto';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost';

const root = process.cwd();
const require = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = require('@playwright/test');
const cdpUrl = process.env.UI_CDP_URL || 'http://127.0.0.1:9224';
const sourcePath = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/fusion.png');
const candidateId = process.env.FUSION_ICON_CANDIDATE_ID || 'fusion-r599-source-candidate';
if (!/^[a-z0-9-]+$/.test(candidateId)) throw new Error('FUSION_ICON_CANDIDATE_ID must contain only lowercase letters, digits, and hyphens');
const candidateDir = path.join(root, 'evidence/ui-image-breakdowns/pages/fusion/icon-candidates', candidateId);
const assetDir = path.join(candidateDir, 'assets');

// Coordinates are measured against the canonical 1920x1080 Fusion UI image.
// Source icons keep their original circular plate; rule/output glyphs are keyed
// against the sampled panel background so the exported PNGs have real alpha.
const iconSpecs = [
  { name: 'source-flow', bbox: [201, 159, 40, 40], mode: 'circle' },
  { name: 'source-asset', bbox: [425, 159, 40, 40], mode: 'circle' },
  { name: 'source-device-log', bbox: [641, 159, 40, 40], mode: 'circle' },
  { name: 'source-user-event', bbox: [861, 159, 40, 40], mode: 'circle' },
  { name: 'source-threat-intel', bbox: [1047, 159, 40, 40], mode: 'circle' },
  { name: 'source-vulnerability', bbox: [1229, 159, 40, 40], mode: 'circle' },
];

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed with ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const sourceDataUrl = `data:image/png;base64,${fs.readFileSync(sourcePath).toString('base64')}`;

const result = await page.evaluate(async ({ sourceDataUrl, iconSpecs }) => {
  const image = await new Promise((resolve, reject) => {
    const candidate = new Image();
    candidate.onload = () => resolve(candidate);
    candidate.onerror = () => reject(new Error('Fusion source PNG decode failed'));
    candidate.src = sourceDataUrl;
  });
  if (image.naturalWidth !== 1920 || image.naturalHeight !== 1080) throw new Error('Unexpected Fusion source dimensions');
  const exports = [];
  for (const spec of iconSpecs) {
    const [x, y, width, height] = spec.bbox;
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const context = canvas.getContext('2d', { willReadFrequently: true });
    context.drawImage(image, x, y, width, height, 0, 0, width, height);
    const pixels = context.getImageData(0, 0, width, height);
    if (spec.mode === 'circle') {
      const cx = (width - 1) / 2;
      const cy = (height - 1) / 2;
      const radius = Math.min(width, height) / 2 - 0.75;
      for (let row = 0; row < height; row += 1) {
        for (let column = 0; column < width; column += 1) {
          const index = (row * width + column) * 4;
          const distance = Math.hypot(column - cx, row - cy);
          const mask = Math.max(0, Math.min(1, radius + 0.75 - distance));
          pixels.data[index + 3] = Math.round(pixels.data[index + 3] * mask);
        }
      }
    } else {
      for (let index = 0; index < pixels.data.length; index += 4) {
        const maximum = Math.max(pixels.data[index], pixels.data[index + 1], pixels.data[index + 2]);
        const minimum = Math.min(pixels.data[index], pixels.data[index + 1], pixels.data[index + 2]);
        const alpha = Math.max(0, Math.min(255, Math.max((maximum - 46) * 5, (maximum - minimum - 16) * 6)));
        pixels.data[index + 3] = Math.min(pixels.data[index + 3], alpha);
      }
    }
    context.putImageData(pixels, 0, 0);
    exports.push({ ...spec, png: canvas.toDataURL('image/png').split(',')[1] });
  }

  const sheet = document.createElement('canvas');
  sheet.width = iconSpecs.length * 200;
  sheet.height = 116;
  const sheetContext = sheet.getContext('2d');
  sheetContext.fillStyle = '#061c2b';
  sheetContext.fillRect(0, 0, sheet.width, sheet.height);
  sheetContext.fillStyle = '#d8ecf7';
  sheetContext.font = '12px sans-serif';
  for (let index = 0; index < exports.length; index += 1) {
    const left = index * 200;
    const top = 8;
    const icon = await new Promise((resolve, reject) => {
      const candidate = new Image();
      candidate.onload = () => resolve(candidate);
      candidate.onerror = reject;
      candidate.src = `data:image/png;base64,${exports[index].png}`;
    });
    const [cropX, cropY, cropWidth, cropHeight] = exports[index].bbox;
    sheetContext.drawImage(image, cropX - 10, cropY - 10, cropWidth + 20, cropHeight + 20, left + 4, top, 60, 60);
    sheetContext.drawImage(icon, left + 72, top + 14, 32, 32);
    sheetContext.drawImage(icon, left + 112, top, 80, 80);
    sheetContext.fillText(exports[index].name.replace(/^source-/, ''), left + 4, top + 96, 188);
  }
  return { exports, sheet: sheet.toDataURL('image/png').split(',')[1] };
}, { sourceDataUrl, iconSpecs });

fs.mkdirSync(assetDir, { recursive: true });
for (const icon of result.exports) {
  const target = path.join(assetDir, `fusion-${icon.name}.png`);
  fs.writeFileSync(target, Buffer.from(icon.png, 'base64'));
}
fs.writeFileSync(path.join(candidateDir, 'review-sheet.png'), Buffer.from(result.sheet, 'base64'));
const sha256 = (buffer) => crypto.createHash('sha256').update(buffer).digest('hex');
const provenance = {
  status: 'candidate',
  candidate_id: candidateId,
  source: path.relative(root, sourcePath),
  source_sha256: sha256(fs.readFileSync(sourcePath)),
  source_dimensions: { width: 1920, height: 1080 },
  extraction_browser: version.Browser,
  extraction_path: 'Xshell tunnel -> 127.0.0.1:9224 -> Windows Chrome canvas',
  icons: result.exports.map(({ name, bbox, mode }) => {
    const output = path.join(assetDir, `fusion-${name}.png`);
    return ({
    name,
    bbox: { x: bbox[0], y: bbox[1], width: bbox[2], height: bbox[3] },
    alpha_mode: mode,
    output: path.relative(root, output),
    sha256: sha256(fs.readFileSync(output)),
  }); }),
  evidence_sheet: path.relative(root, path.join(candidateDir, 'review-sheet.png')),
  generated_at: new Date().toISOString(),
};
fs.writeFileSync(path.join(assetDir, 'fusion-icons.source.json'), `${JSON.stringify(provenance, null, 2)}\n`);
console.log(JSON.stringify(provenance, null, 2));
await page.close();
process.exit(0);

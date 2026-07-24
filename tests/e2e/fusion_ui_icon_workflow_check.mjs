#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const candidateRoot = path.join(root, 'evidence/ui-image-breakdowns/pages/fusion/icon-candidates');
const formalDir = path.join(root, 'web/ui/src/assets/screenshot-icons/fusion');
const pagePath = path.join(root, 'web/ui/src/pages/FusionWorkbenchPage.tsx');
const pageSource = fs.readFileSync(pagePath, 'utf8');
const failures = [];
const candidates = [];

for (const entry of fs.readdirSync(candidateRoot, { withFileTypes: true })) {
  if (!entry.isDirectory()) continue;
  const directory = path.join(candidateRoot, entry.name);
  const reviewPath = path.join(directory, 'review.json');
  if (!fs.existsSync(reviewPath)) {
    failures.push(`${entry.name} has no review.json`);
    continue;
  }
  const review = JSON.parse(fs.readFileSync(reviewPath, 'utf8'));
  if (!['approved', 'rejected'].includes(review.decision)) failures.push(`${entry.name} review is not final`);
  if (review.decision !== 'approved' && review.formal_asset_copy !== false) failures.push(`${entry.name} rejected candidate is marked copied`);
  if (!Array.isArray(review.reviewed_images) || review.reviewed_images.length === 0) failures.push(`${entry.name} has no reviewed images`);
  candidates.push({ id: entry.name, decision: review.decision, formal_asset_copy: review.formal_asset_copy, issues: review.blocking_issues?.length ?? 0 });
}

let formalAssets = { present: fs.existsSync(formalDir), approved: false, count: 0 };
if (formalAssets.present) {
  const provenancePath = path.join(formalDir, 'fusion-icons.source.json');
  if (!fs.existsSync(provenancePath)) failures.push('formal asset directory has no approved provenance');
  else {
    const provenance = JSON.parse(fs.readFileSync(provenancePath, 'utf8'));
    formalAssets = { present: true, approved: provenance.status === 'approved', count: provenance.icons?.length ?? 0 };
    if (!formalAssets.approved) failures.push('formal assets do not have approved status');
  }
}
if (!formalAssets.approved && pageSource.includes('@/assets/screenshot-icons/fusion/')) failures.push('Fusion page imports unapproved raster assets');
for (const marker of ['LineChartOutlined', 'AppstoreOutlined', 'FileTextOutlined', 'UserOutlined', 'AimOutlined', 'SafetyCertificateOutlined']) {
  if (!pageSource.includes(marker)) failures.push(`reviewed code-native fallback is missing ${marker}`);
}

const report = {
  result: failures.length === 0 ? 'pass' : 'fail',
  policy: 'candidate -> single/sheet review -> approval -> formal copy',
  candidates,
  formal_assets: formalAssets,
  production_page: path.relative(root, pagePath),
  production_strategy: formalAssets.approved ? 'approved-raster-assets' : 'reviewed-code-native-icons',
  failures,
};
console.log(JSON.stringify(report, null, 2));
process.exit(failures.length === 0 ? 0 : 1);

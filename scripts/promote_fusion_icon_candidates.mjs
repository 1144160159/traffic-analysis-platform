#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const candidateId = process.argv[2];
if (!candidateId || !/^[a-z0-9-]+$/.test(candidateId)) throw new Error('usage: node scripts/promote_fusion_icon_candidates.mjs <candidate-id>');
const candidateDir = path.join(root, 'evidence/ui-image-breakdowns/pages/fusion/icon-candidates', candidateId);
const candidateAssetDir = path.join(candidateDir, 'assets');
const provenancePath = path.join(candidateAssetDir, 'fusion-icons.source.json');
const reviewPath = path.join(candidateDir, 'review.json');
const targetDir = path.join(root, 'web/ui/src/assets/screenshot-icons/fusion');
const provenance = JSON.parse(fs.readFileSync(provenancePath, 'utf8'));
const review = JSON.parse(fs.readFileSync(reviewPath, 'utf8'));
const digest = (buffer) => crypto.createHash('sha256').update(buffer).digest('hex');

if (provenance.status !== 'candidate' || provenance.candidate_id !== candidateId) throw new Error('candidate provenance mismatch');
if (review.candidate_id !== candidateId || review.decision !== 'approved') throw new Error('candidate review is not approved');
if (!review.reviewer || !review.reviewed_at || !Array.isArray(review.reviewed_images) || review.reviewed_images.length < provenance.icons.length + 1) throw new Error('review evidence is incomplete');
if (!Array.isArray(review.blocking_issues) || review.blocking_issues.length !== 0) throw new Error('review still contains blocking issues');
for (const icon of provenance.icons) {
  const sourcePath = path.join(root, icon.output);
  const bytes = fs.readFileSync(sourcePath);
  if (digest(bytes) !== icon.sha256) throw new Error(`${icon.name} checksum mismatch`);
  if (!review.reviewed_images.includes(path.basename(icon.output))) throw new Error(`${icon.name} lacks review evidence`);
}

fs.mkdirSync(targetDir, { recursive: true });
for (const icon of provenance.icons) fs.copyFileSync(path.join(root, icon.output), path.join(targetDir, path.basename(icon.output)));
const promoted = {
  ...provenance,
  status: 'approved',
  promoted_from: candidateId,
  promoted_at: new Date().toISOString(),
  review: path.relative(root, reviewPath),
  icons: provenance.icons.map((icon) => ({ ...icon, output: path.relative(root, path.join(targetDir, path.basename(icon.output))) })),
};
fs.writeFileSync(path.join(targetDir, 'fusion-icons.source.json'), `${JSON.stringify(promoted, null, 2)}\n`);
console.log(JSON.stringify({ result: 'promoted', candidate_id: candidateId, target: path.relative(root, targetDir), count: promoted.icons.length }, null, 2));

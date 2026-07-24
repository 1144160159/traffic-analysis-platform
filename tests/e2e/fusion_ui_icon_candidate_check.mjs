#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const candidateId = process.env.FUSION_ICON_CANDIDATE_ID || 'fusion-r599-source-candidate';
if (!/^[a-z0-9-]+$/.test(candidateId)) throw new Error('invalid FUSION_ICON_CANDIDATE_ID');
const candidateDir = path.join(root, 'evidence/ui-image-breakdowns/pages/fusion/icon-candidates', candidateId);
const provenancePath = path.join(candidateDir, 'assets/fusion-icons.source.json');
const pagePath = path.join(root, 'web/ui/src/pages/FusionWorkbenchPage.tsx');
const expected = [
  'source-flow', 'source-asset', 'source-device-log', 'source-user-event', 'source-threat-intel', 'source-vulnerability',
];

const digest = (buffer) => crypto.createHash('sha256').update(buffer).digest('hex');
const provenance = JSON.parse(fs.readFileSync(provenancePath, 'utf8'));
const sourcePath = path.join(root, provenance.source);
const pageSource = fs.readFileSync(pagePath, 'utf8');
const failures = [];

if (provenance.status !== 'candidate' || provenance.candidate_id !== candidateId) failures.push('provenance is not a matching candidate record');
if (provenance.source_dimensions?.width !== 1920 || provenance.source_dimensions?.height !== 1080) failures.push('canonical source dimensions are not 1920x1080');
if (!fs.existsSync(sourcePath) || digest(fs.readFileSync(sourcePath)) !== provenance.source_sha256) failures.push('canonical source digest mismatch');
if (!Array.isArray(provenance.icons) || provenance.icons.length !== expected.length) failures.push(`expected ${expected.length} icon records`);
const names = new Set(provenance.icons?.map((icon) => icon.name));
for (const name of expected) if (!names.has(name)) failures.push(`missing icon record ${name}`);

for (const icon of provenance.icons ?? []) {
  const outputPath = path.resolve(root, icon.output);
  const allowedRoot = path.resolve(candidateDir, 'assets') + path.sep;
  if (!outputPath.startsWith(allowedRoot)) {
    failures.push(`${icon.name} output escapes asset directory`);
    continue;
  }
  if (!fs.existsSync(outputPath)) {
    failures.push(`${icon.name} output is missing`);
    continue;
  }
  const buffer = fs.readFileSync(outputPath);
  if (buffer.subarray(0, 8).toString('hex') !== '89504e470d0a1a0a') failures.push(`${icon.name} is not PNG`);
  const width = buffer.readUInt32BE(16);
  const height = buffer.readUInt32BE(20);
  if (width !== icon.bbox.width || height !== icon.bbox.height) failures.push(`${icon.name} dimensions differ from bbox`);
  if (digest(buffer) !== icon.sha256) failures.push(`${icon.name} checksum mismatch`);
}
if (pageSource.includes('@/assets/screenshot-icons/fusion/')) failures.push('unreviewed candidates are imported by FusionWorkbenchPage');
if (!fs.existsSync(path.join(candidateDir, 'review-sheet.png'))) failures.push('candidate review sheet is missing');

const report = {
  result: failures.length === 0 ? 'pass' : 'fail',
  scope: 'candidate-integrity-only',
  quality_gate: fs.existsSync(path.join(candidateDir, 'review.json')) ? JSON.parse(fs.readFileSync(path.join(candidateDir, 'review.json'), 'utf8')).decision : 'pending',
  candidate_id: candidateId,
  source: provenance.source,
  source_sha256: provenance.source_sha256,
  icon_count: provenance.icons?.length ?? 0,
  expected_icon_count: expected.length,
  page: path.relative(root, pagePath),
  failures,
};
console.log(JSON.stringify(report, null, 2));
process.exit(failures.length === 0 ? 0 : 1);

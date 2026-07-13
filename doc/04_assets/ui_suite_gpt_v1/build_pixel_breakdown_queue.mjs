#!/usr/bin/env node

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.resolve(__dirname, '../../..');
const SUITE_DIR = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1');
const SCREENS_DIR = path.join(SUITE_DIR, 'screens');
const SPEC_DIR = path.join(SUITE_DIR, 'specs');
const OUT_JSON = path.join(SPEC_DIR, 'pixel-perfect-breakdown-index.json');
const OUT_MD = path.join(SPEC_DIR, 'PIXEL_PERFECT_BREAKDOWN_INDEX.md');

const CATEGORY_ORDER = ['foundations', 'components', 'pages', 'overlays', 'states', 'responsive'];

function rel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, '/');
}

function walk(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const file = path.join(dir, entry.name);
    return entry.isDirectory() ? walk(file) : [file];
  });
}

function pngDimensions(file) {
  const buf = fs.readFileSync(file);
  if (buf.length < 24 || buf.toString('hex', 0, 8) !== '89504e470d0a1a0a') {
    throw new Error(`${file} is not a PNG`);
  }
  return { width: buf.readUInt32BE(16), height: buf.readUInt32BE(20) };
}

function categoryRank(category) {
  const index = CATEGORY_ORDER.indexOf(category);
  return index < 0 ? CATEGORY_ORDER.length : index;
}

function canonicalImages() {
  return walk(SCREENS_DIR)
    .filter((file) => file.endsWith('.png'))
    .filter((file) => !path.basename(file).includes('.raw-') && !path.basename(file).includes('.before-'))
    .map((file) => {
      const relative = path.relative(SCREENS_DIR, file).split(path.sep);
      const category = relative[0];
      const id = path.basename(file, '.png');
      const dims = pngDimensions(file);
      return {
        id,
        category,
        source_image: rel(file),
        canvas: dims,
        status: 'not-started',
        breakdown: `doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/${category}/${id}.md`,
        review: `doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/${category}/${id}.review.md`,
        json: `doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/${category}/${id}.json`,
        evidence_dir: `evidence/ui-image-breakdowns/${category}/${id}`,
      };
    })
    .sort((a, b) => categoryRank(a.category) - categoryRank(b.category) || a.id.localeCompare(b.id));
}

function counts(items) {
  return items.reduce((acc, item) => {
    acc[item.category] = (acc[item.category] ?? 0) + 1;
    return acc;
  }, {});
}

function markdown(items) {
  const byCategory = counts(items);
  const lines = [];
  lines.push('# Pixel Perfect 逐图精拆索引');
  lines.push('');
  lines.push('本文件只记录执行队列和状态，不替代任何单图精拆记录。每张图必须按 `PIXEL_PERFECT_IMAGE_BREAKDOWN_PLAN.md` 单独处理。');
  lines.push('');
  lines.push('## 总量');
  lines.push('');
  lines.push(`- canonical PNG：${items.length}`);
  for (const category of CATEGORY_ORDER) lines.push(`- ${category}：${byCategory[category] ?? 0}`);
  lines.push('');
  lines.push('## 队列');
  lines.push('');
  lines.push('| 顺序 | 状态 | 分类 | 图片 ID | 源图 | 精拆记录 |');
  lines.push('|---:|---|---|---|---|---|');
  items.forEach((item, index) => {
    lines.push(`| ${index + 1} | \`${item.status}\` | \`${item.category}\` | \`${item.id}\` | \`${item.source_image}\` | \`${item.breakdown}\` |`);
  });
  return lines.join('\n');
}

function main() {
  const items = canonicalImages();
  const payload = {
    generated_by: 'build_pixel_breakdown_queue.mjs',
    purpose: 'queue-only; not a visual breakdown and not an acceptance result',
    canonical_png_count: items.length,
    category_counts: counts(items),
    status_counts: { 'not-started': items.length },
    items,
  };
  fs.writeFileSync(OUT_JSON, `${JSON.stringify(payload, null, 2)}\n`);
  fs.writeFileSync(OUT_MD, `${markdown(items)}\n`);
  console.log(`${items.length} canonical PNG queued`);
}

main();

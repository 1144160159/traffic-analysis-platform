#!/usr/bin/env node

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.resolve(__dirname, '../../..');
const SUITE_DIR = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1');
const SPEC_DIR = path.join(SUITE_DIR, 'specs');
const INDEX_PATH = path.join(SPEC_DIR, 'pixel-perfect-breakdown-index.json');
const OUT_JSON = path.join(SPEC_DIR, 'image-breakdown-record-status.json');
const OUT_MD = path.join(SPEC_DIR, 'IMAGE_BREAKDOWN_RECORD_STATUS.md');

const REQUIRED_ARRAYS = ['regions', 'texts', 'components', 'icons', 'tokens', 'interactions'];
const REQUIRED_EVIDENCE = ['target'];
const REQUIRED_MD_SECTIONS = [
  '## 基本信息',
  '## 目标图观察',
  '## 区域与坐标',
  '## 文本清单',
  '## 组件清单',
  '## 图标清单',
  '## Token 与样式',
  '## 状态与交互',
  '## 实现映射',
  '## 验收证据',
  '## 差异清单',
  '## 结论',
];
const DEEP_MINIMUMS = {
  markdownLines: 220,
  reviewLines: 35,
  regions: 12,
  texts: 30,
  components: 6,
  icons: 5,
  tokens: 10,
  interactions: 5,
};

function repoPath(file) {
  return path.join(ROOT, file);
}

function existsRepo(file) {
  return Boolean(file) && fs.existsSync(repoPath(file));
}

function readJson(file, fallback = null) {
  if (!fs.existsSync(file)) return fallback;
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function readRepoText(file) {
  if (!existsRepo(file)) return '';
  return fs.readFileSync(repoPath(file), 'utf8');
}

function lineCount(text) {
  if (!text) return 0;
  return text.split(/\r?\n/).length;
}

function recordStatus(item) {
  const missing = [];
  const warnings = [];
  if (!existsRepo(item.breakdown)) missing.push('markdown');
  if (!existsRepo(item.json)) missing.push('json');
  if (!existsRepo(item.review)) missing.push('review');

  let record = null;
  let jsonError = null;
  const markdownText = readRepoText(item.breakdown);
  const reviewText = readRepoText(item.review);

  if (markdownText) {
    if (lineCount(markdownText) < DEEP_MINIMUMS.markdownLines) missing.push(`markdown-lines>=${DEEP_MINIMUMS.markdownLines}`);
    for (const section of REQUIRED_MD_SECTIONS) {
      if (!markdownText.includes(section)) missing.push(`markdown-section:${section.replace(/^## /, '')}`);
    }
  }
  if (reviewText && lineCount(reviewText) < DEEP_MINIMUMS.reviewLines) missing.push(`review-lines>=${DEEP_MINIMUMS.reviewLines}`);

  if (existsRepo(item.json)) {
    try {
      record = JSON.parse(fs.readFileSync(repoPath(item.json), 'utf8'));
    } catch (error) {
      jsonError = String(error.message || error);
      missing.push('json-parseable');
    }
  }

  if (record) {
    if (record.id !== item.id) missing.push('id-match');
    if (record.category !== item.category) missing.push('category-match');
    if (record.source_image !== item.source_image) missing.push('source-image-match');
    if (record.canvas?.width !== item.canvas.width || record.canvas?.height !== item.canvas.height) missing.push('canvas-match');
    for (const key of REQUIRED_ARRAYS) {
      if (!Array.isArray(record[key]) || record[key].length === 0) missing.push(`${key}-nonempty`);
    }
    for (const [key, min] of Object.entries(DEEP_MINIMUMS)) {
      if (!REQUIRED_ARRAYS.includes(key)) continue;
      if (!Array.isArray(record[key]) || record[key].length < min) missing.push(`${key}>=${min}`);
    }
    if (Array.isArray(record.regions) && record.regions.some((entry) => !entry.bbox || typeof entry.bbox.x !== 'number' || typeof entry.bbox.y !== 'number' || typeof entry.bbox.w !== 'number' || typeof entry.bbox.h !== 'number')) {
      missing.push('regions-bbox');
    }
    if (Array.isArray(record.texts) && record.texts.some((entry) => !entry.value || !entry.type)) missing.push('texts-value-type');
    for (const key of REQUIRED_EVIDENCE) {
      if (!existsRepo(record.evidence?.[key])) missing.push(`evidence-${key}`);
    }
    if (!record.observation && !record.focus && !record.purpose) missing.push('observation-or-purpose');
    if (Array.isArray(record.unresolved) && record.unresolved.some((entry) => /visual diff|screenshot|implementation/i.test(String(entry)))) {
      warnings.push('record still contains implementation/diff unresolved items; acceptable for breakdown gate but not pixel gate');
    }
  }

  const passed = missing.length === 0;
  return {
    id: item.id,
    category: item.category,
    source_image: item.source_image,
    passed,
    stage: passed ? 'breakdown-accepted' : 'breakdown-incomplete',
    missing,
    warnings,
    json_error: jsonError,
    files: {
      markdown: existsRepo(item.breakdown) ? item.breakdown : '',
      json: existsRepo(item.json) ? item.json : '',
      review: existsRepo(item.review) ? item.review : '',
    },
    deep_minimums: {
      markdown_lines: markdownText ? lineCount(markdownText) : 0,
      review_lines: reviewText ? lineCount(reviewText) : 0,
      regions: Array.isArray(record?.regions) ? record.regions.length : 0,
      texts: Array.isArray(record?.texts) ? record.texts.length : 0,
      components: Array.isArray(record?.components) ? record.components.length : 0,
      icons: Array.isArray(record?.icons) ? record.icons.length : 0,
      tokens: Array.isArray(record?.tokens) ? record.tokens.length : 0,
      interactions: Array.isArray(record?.interactions) ? record.interactions.length : 0,
    },
  };
}

function counts(items) {
  return items.reduce((acc, item) => {
    const key = item.passed ? 'breakdown-accepted' : 'breakdown-incomplete';
    acc[key] = (acc[key] ?? 0) + 1;
    return acc;
  }, {});
}

function markdown(report) {
  const lines = [];
  lines.push('# UI 图拆解记录门禁状态');
  lines.push('');
  lines.push('本文件只判断“逐图拆解记录”是否完整，不判断前端实现截图和 pixel diff。像素级复刻仍以 `PIXEL_PERFECT_PIPELINE_STATUS.md` 为准。');
  lines.push('');
  lines.push('## 汇总');
  lines.push('');
  lines.push(`- 总图数：${report.total}`);
  lines.push(`- 拆解通过：${report.accepted}`);
  lines.push(`- 拆解未通过：${report.total - report.accepted}`);
  for (const [key, value] of Object.entries(report.stage_counts)) lines.push(`- ${key}：${value}`);
  lines.push('');
  lines.push('## 深拆最低门槛');
  lines.push('');
  lines.push(`- 主拆解记录行数：>= ${DEEP_MINIMUMS.markdownLines}`);
  lines.push(`- review 行数：>= ${DEEP_MINIMUMS.reviewLines}`);
  lines.push(`- regions/texts/components/icons/tokens/interactions：>= ${DEEP_MINIMUMS.regions}/${DEEP_MINIMUMS.texts}/${DEEP_MINIMUMS.components}/${DEEP_MINIMUMS.icons}/${DEEP_MINIMUMS.tokens}/${DEEP_MINIMUMS.interactions}`);
  lines.push('- 基准：`foundation-color-status`。本门禁不代表像素 diff 通过。');
  lines.push('');
  lines.push('## 未通过队列');
  lines.push('');
  lines.push('| 分类 | 图片 ID | 缺失项 |');
  lines.push('|---|---|---|');
  for (const item of report.items.filter((entry) => !entry.passed)) {
    lines.push(`| \`${item.category}\` | \`${item.id}\` | ${item.missing.map((entry) => `\`${entry}\``).join('<br>')} |`);
  }
  return lines.join('\n');
}

function main() {
  const index = readJson(INDEX_PATH);
  if (!index?.items?.length) {
    throw new Error('missing pixel-perfect-breakdown-index.json; run build_pixel_breakdown_queue.mjs first');
  }
  const items = index.items.map(recordStatus);
  const accepted = items.filter((item) => item.passed).length;
  const report = {
    generated_by: 'validate_image_breakdown_records.mjs',
    total: items.length,
    accepted,
    stage_counts: counts(items),
    items,
  };
  fs.writeFileSync(OUT_JSON, `${JSON.stringify(report, null, 2)}\n`);
  fs.writeFileSync(OUT_MD, `${markdown(report)}\n`);
  console.log(JSON.stringify({ total: report.total, accepted: report.accepted, stage_counts: report.stage_counts }, null, 2));
}

main();

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
const OUT_JSON = path.join(SPEC_DIR, 'pixel-perfect-pipeline-status.json');
const OUT_MD = path.join(SPEC_DIR, 'PIXEL_PERFECT_PIPELINE_STATUS.md');

const REQUIRED_EVIDENCE = [
  ['target', 'target.png'],
  ['implementation', 'implementation.png'],
  ['diff', 'diff.png'],
  ['regions_overlay', 'regions-overlay.png'],
  ['verification', 'verification.json'],
  ['metrics', 'metrics.json'],
];

function repoPath(file) {
  return path.join(ROOT, file);
}

function repoRel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, '/');
}

function existsRepo(file) {
  return Boolean(file) && fs.existsSync(repoPath(file));
}

function readJsonAbs(file, fallback = null) {
  if (!file || !fs.existsSync(file)) return fallback;
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function readJsonRepo(file, fallback = null) {
  return readJsonAbs(repoPath(file), fallback);
}

function readTextRepo(file) {
  if (!existsRepo(file)) return '';
  return fs.readFileSync(repoPath(file), 'utf8');
}

function listFilesRecursive(root) {
  if (!fs.existsSync(root)) return [];
  const entries = fs.readdirSync(root, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const child = path.join(root, entry.name);
    if (entry.isDirectory()) files.push(...listFilesRecursive(child));
    else if (entry.isFile()) files.push(child);
  }
  return files;
}

function forbiddenPublicTargetResources() {
  const publicRoot = path.join(ROOT, 'web/ui/public');
  const canonicalRoot = path.join(publicRoot, 'ui-assets/canonical');
  const candidates = [
    ...listFilesRecursive(canonicalRoot),
    ...listFilesRecursive(path.join(publicRoot, 'screens')),
  ];
  return Array.from(new Set(candidates))
    .filter((file) => {
      const rel = repoRel(file).toLowerCase();
      return (
        rel.includes('/ui-assets/canonical/') ||
        rel.includes('/screens/pages/') ||
        rel.includes('/screens/components/') ||
        rel.includes('/screens/overlays/') ||
        rel.endsWith('/target.png') ||
        rel.endsWith('/regions-overlay.png') ||
        rel.endsWith('/implementation.png')
      );
    })
    .map(repoRel);
}

function defaultEvidencePath(item, filename) {
  return `${item.evidence_dir}/${filename}`;
}

function evidencePath(item, record, key, filename) {
  return record?.evidence?.[key] || defaultEvidencePath(item, filename);
}

function metricRatio(metrics) {
  const value =
    metrics?.visual_diff?.pixel_mismatch_ratio ??
    metrics?.visualDiff?.pixelMismatchRatio ??
    metrics?.pixel_mismatch_ratio ??
    null;
  return typeof value === 'number' ? value : Number(value);
}

function metricMax(metrics) {
  const value = metrics?.visual_diff?.max_pixel_ratio ?? metrics?.max_pixel_ratio ?? 0.015;
  return typeof value === 'number' ? value : Number(value);
}

function hasUnresolvedText(text) {
  if (!text) return false;
  return /\bunresolved\b|未解决|待解决|未关闭|blocked|diff-pending/i.test(text);
}

function unresolvedItems(record, verification, markdownText, reviewText) {
  const items = [];
  if (Array.isArray(record?.unresolved)) {
    for (const item of record.unresolved) items.push(`record:${String(item)}`);
  }
  if (Array.isArray(verification?.unresolved)) {
    for (const item of verification.unresolved) items.push(`verification:${String(item)}`);
  }
  if (Array.isArray(record?.differences)) {
    for (const item of record.differences) {
      const status = String(item?.status ?? '');
      if (/unresolved|open|blocked|fail|pending|未解决|待解决|未关闭/i.test(status)) {
        items.push(`difference:${item?.type ?? 'unknown'}:${item?.location ?? item?.region ?? 'unknown'}`);
      }
    }
  }
  if (Array.isArray(verification?.differences)) {
    for (const item of verification.differences) {
      const status = String(item?.status ?? '');
      if (/unresolved|open|blocked|fail|pending|未解决|待解决|未关闭/i.test(status)) {
        items.push(`verification-difference:${item?.type ?? 'unknown'}:${item?.location ?? item?.region ?? 'unknown'}`);
      }
    }
  }
  if (hasUnresolvedText(markdownText)) items.push('markdown:contains unresolved/open diff wording');
  if (hasUnresolvedText(reviewText)) items.push('review:contains unresolved/open diff wording');
  return items;
}

function browserEvidenceOk(verification, metrics) {
  const backend =
    verification?.browser?.backend ??
    verification?.windows_chrome?.backend ??
    verification?.desktop_chrome_backend_status ??
    metrics?.desktop_chrome_backend_status ??
    '';
  const backendText = String(backend).toLowerCase();
  return backendText.includes('windows') || backendText.includes('chrome') || backendText === 'pass';
}

function filesFor(item, record) {
  const files = {
    markdown: existsRepo(item.breakdown) ? item.breakdown : '',
    json: existsRepo(item.json) ? item.json : '',
    review: existsRepo(item.review) ? item.review : '',
  };
  for (const [key, filename] of REQUIRED_EVIDENCE) {
    const candidate = evidencePath(item, record, key, filename);
    files[key] = existsRepo(candidate) ? candidate : '';
  }
  return files;
}

function firstMissing(files, keys) {
  return keys.find((key) => !files[key]) ?? null;
}

function stageFor({ record, jsonError, verification, verificationError, metrics, captureMeta, files, unresolved, forbiddenPublicResources }) {
  const missingBreakdown = firstMissing(files, ['markdown', 'json', 'review']);
  if (missingBreakdown) return `breakdown-${missingBreakdown}-missing`;
  if (jsonError || !record) return 'breakdown-json-invalid';
  if (record?.category === 'pages' && forbiddenPublicResources.length) return 'forbidden-public-ui-target-resource';
  if (!record.regions?.length || !record.texts?.length || !record.components?.length) return 'breakdown-incomplete';
  if (firstMissing(files, ['target'])) return 'target-missing';
  if (!record.implementation?.source && !(record.implementation?.pages?.length || record.implementation?.components?.length)) return 'implementation-missing';
  if (firstMissing(files, ['implementation'])) return 'windows-cdp-screenshot-missing';
  if (firstMissing(files, ['diff'])) return 'visual-diff-missing';
  if (firstMissing(files, ['regions_overlay'])) return 'regions-overlay-missing';
  if (firstMissing(files, ['metrics'])) return 'diff-metrics-missing';
  if (firstMissing(files, ['verification'])) return 'verification-missing';
  if (verificationError || !verification) return 'verification-json-invalid';
  if (record?.category === 'pages') {
    if (record?.evidence?.evidence_mode !== 'production-route' || captureMeta?.evidence_mode !== 'production-route') {
      return 'production-route-evidence-missing';
    }
  }

  const ratio = metricRatio(metrics);
  const maxRatio = metricMax(metrics);
  if (metrics?.status !== 'pass') return 'visual-diff-failed';
  if (!Number.isFinite(ratio)) return 'diff-metrics-invalid';
  if (ratio > maxRatio) return 'visual-diff-failed';

  if (unresolved.length) return 'unresolved-open';
  if (!browserEvidenceOk(verification, metrics)) return 'windows-chrome-proof-missing';
  if (!verification?.auxiliary_agent_review || verification.auxiliary_agent_review.status !== 'reviewed') return 'auxiliary-agent-review-missing';
  if (verification?.main_thread_judgment !== 'pixel-accepted') return 'main-thread-judgment-missing';
  if (record.status !== 'pixel-accepted' || verification.status !== 'pixel-accepted' || !record.accepted || !verification.accepted) return 'not-accepted';
  return 'pixel-accepted';
}

function statusFor(item, forbiddenPublicResources) {
  let record = null;
  let jsonError = null;
  if (existsRepo(item.json)) {
    try {
      record = readJsonRepo(item.json);
    } catch (error) {
      jsonError = String(error.message || error);
    }
  }

  const files = filesFor(item, record);
  const markdownText = readTextRepo(item.breakdown);
  const reviewText = readTextRepo(item.review);

  let verification = null;
  let verificationError = null;
  if (files.verification) {
    try {
      verification = readJsonRepo(files.verification);
    } catch (error) {
      verificationError = String(error.message || error);
    }
  }

  let metrics = null;
  let metricsError = null;
  if (files.metrics) {
    try {
      metrics = readJsonRepo(files.metrics);
    } catch (error) {
      metricsError = String(error.message || error);
    }
  }

  let captureMeta = null;
  if (record?.evidence?.capture_meta && existsRepo(record.evidence.capture_meta)) {
    try {
      captureMeta = readJsonRepo(record.evidence.capture_meta);
    } catch {
      captureMeta = null;
    }
  }

  const unresolved = unresolvedItems(record, verification, markdownText, reviewText);
  const stage = stageFor({ record, jsonError, verification, verificationError, metrics, captureMeta, files, unresolved, forbiddenPublicResources });
  return {
    id: item.id,
    category: item.category,
    source_image: item.source_image,
    stage,
    accepted: stage === 'pixel-accepted',
    status: record?.status ?? verification?.status ?? 'not-started',
    files,
    viewport: verification?.viewport ?? metrics?.viewport ?? null,
    url: verification?.url ?? record?.evidence?.url ?? '',
    visual_diff: metrics?.visual_diff ?? null,
    evidence_mode: record?.evidence?.evidence_mode ?? captureMeta?.evidence_mode ?? '',
    pixel_mismatch_ratio: Number.isFinite(metricRatio(metrics)) ? metricRatio(metrics) : null,
    max_pixel_ratio: Number.isFinite(metricMax(metrics)) ? metricMax(metrics) : null,
    main_thread_judgment: verification?.main_thread_judgment ?? '',
    auxiliary_agent_review: verification?.auxiliary_agent_review ?? null,
    json_error: jsonError,
    verification_error: verificationError,
    metrics_error: metricsError,
    unresolved,
    forbidden_public_target_resources: record?.category === 'pages' ? forbiddenPublicResources : [],
  };
}

function counts(statuses) {
  const out = {};
  for (const item of statuses) out[item.stage] = (out[item.stage] ?? 0) + 1;
  return out;
}

function markdown(report) {
  const lines = [];
  lines.push('# Pixel Perfect 全流程状态');
  lines.push('');
  lines.push('本文件是机器门禁状态，不替代逐图精拆记录。只有 `pixel-accepted` 才表示该图完整走完拆解、实现、Windows Chrome 截图、overlay、diff、智能体辅助审查和主线程验收。');
  lines.push('');
  lines.push('## 汇总');
  lines.push('');
  lines.push(`- 总图数：${report.total}`);
  lines.push(`- 已 pixel accepted：${report.accepted}`);
  lines.push(`- 未完成：${report.total - report.accepted}`);
  for (const [stage, count] of Object.entries(report.stage_counts)) lines.push(`- ${stage}：${count}`);
  if (report.forbidden_public_target_resources?.length) {
    lines.push('- 禁止的 web public UI 图资源：');
    for (const file of report.forbidden_public_target_resources) lines.push(`  - \`${file}\``);
  }
  lines.push('');
  lines.push('## 未完成队列');
  lines.push('');
  lines.push('| 分类 | 图片 ID | 当前阶段 | 状态 | mismatch | 主线程判定 | 未解决项 |');
  lines.push('|---|---|---|---|---:|---|---|');
  for (const item of report.items.filter((entry) => entry.stage !== 'pixel-accepted')) {
    const unresolved = item.unresolved.length ? item.unresolved.map((entry) => `\`${entry.replaceAll('|', '/')}\``).join('<br>') : '-';
    const ratio = typeof item.pixel_mismatch_ratio === 'number' ? item.pixel_mismatch_ratio.toFixed(6) : '-';
    lines.push(`| \`${item.category}\` | \`${item.id}\` | \`${item.stage}\` | \`${item.status}\` | ${ratio} | \`${item.main_thread_judgment || '-'}\` | ${unresolved} |`);
  }
  return lines.join('\n');
}

function main() {
  const index = readJsonAbs(INDEX_PATH);
  if (!index?.items?.length) {
    throw new Error(`missing queue index: ${repoRel(INDEX_PATH)}; run build_pixel_breakdown_queue.mjs first`);
  }
  const forbiddenPublicResources = forbiddenPublicTargetResources();
  const items = index.items.map((item) => statusFor(item, forbiddenPublicResources));
  const report = {
    generated_by: 'validate_pixel_breakdown_pipeline.mjs',
    total: items.length,
    accepted: items.filter((item) => item.stage === 'pixel-accepted').length,
    forbidden_public_target_resources: forbiddenPublicResources,
    stage_counts: counts(items),
    items,
  };
  fs.writeFileSync(OUT_JSON, `${JSON.stringify(report, null, 2)}\n`);
  fs.writeFileSync(OUT_MD, `${markdown(report)}\n`);
  console.log(JSON.stringify({ total: report.total, accepted: report.accepted, stage_counts: report.stage_counts }, null, 2));
}

main();

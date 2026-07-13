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
const BREAKDOWN_DIR = path.join(SPEC_DIR, 'image-breakdowns');
const EVIDENCE_ROOT = path.join(ROOT, 'evidence/ui-image-breakdowns');
const MANIFEST_PATH = path.join(SUITE_DIR, 'manifest.json');
const ROUTE_MAP_PATH = path.join(SPEC_DIR, 'route-page-map.json');

function parseArgs() {
  const args = process.argv.slice(2);
  const out = {};
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '--image') out.image = args[++i];
    else if (arg === '--force') out.force = true;
    else throw new Error(`unknown argument: ${arg}`);
  }
  if (!out.image) throw new Error('usage: node start_pixel_image_breakdown.mjs --image <one PNG>');
  return out;
}

function rel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, '/');
}

function readJson(file, fallback) {
  if (!fs.existsSync(file)) return fallback;
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function pngDimensions(file) {
  const buf = fs.readFileSync(file);
  if (buf.length < 24 || buf.toString('hex', 0, 8) !== '89504e470d0a1a0a') {
    throw new Error(`${file} is not a PNG`);
  }
  return { width: buf.readUInt32BE(16), height: buf.readUInt32BE(20) };
}

function canonicalImagePath(input) {
  const abs = path.isAbsolute(input) ? input : path.join(ROOT, input);
  if (!abs.startsWith(SCREENS_DIR + path.sep)) throw new Error(`image must be under ${rel(SCREENS_DIR)}`);
  const base = path.basename(abs);
  if (!base.endsWith('.png')) throw new Error('only PNG images are supported');
  if (base.includes('.raw-') || base.includes('.before-')) throw new Error('raw/before images are not canonical targets');
  return abs;
}

function categoryFromImage(imageAbs) {
  const relative = path.relative(SCREENS_DIR, imageAbs).split(path.sep);
  return relative[0];
}

function findRoute(id, category) {
  if (category !== 'pages') return null;
  const routes = readJson(ROUTE_MAP_PATH, []);
  return routes.find((item) => item.id === id) ?? null;
}

function findManifestItem(id, imageRel) {
  const manifest = readJson(MANIFEST_PATH, { items: [] });
  return manifest.items.find((item) => item.id === id || item.targetFile === imageRel || item.file === imageRel) ?? null;
}

function promptPathFor(id) {
  const prompt = path.join(SUITE_DIR, 'prompts', `${id}.prompt.txt`);
  return fs.existsSync(prompt) ? rel(prompt) : null;
}

function mdTemplate(record) {
  return `# ${record.id}.png 逐图精拆记录

## 基本信息

- 分类：\`${record.category}\`
- 源图：\`${record.source_image}\`
- 源图尺寸：\`${record.canvas.width}x${record.canvas.height}\`
- 对应 prompt：${record.prompt ? `\`${record.prompt}\`` : '无直接 prompt'}
- 对应路由/宿主路由：${record.route ? `\`${record.route}\`` : '待确认'}
- 当前状态：\`${record.status}\`
- 复刻等级：\`draft\`

## 目标图观察

- 整体布局：待逐图视觉读取后填写。
- 业务重点：待逐图视觉读取后填写。
- 当前页面/浮层状态：待逐图视觉读取后填写。

## 区域与坐标

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 待拆解 | 待测量 | 待确认 | 待逐图填写 | 不得用程序推断替代 |

## 文本清单

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|

## 图标清单

| 位置 | 图标 | 图标库/实现 | 语义 | 是否需自绘 |
|---|---|---|---|---|

## Token 与样式

| 项 | 值 | 来源 | 备注 |
|---|---|---|---|

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|

## 实现映射

- 页面：待确认
- 组件：待确认
- API/数据：待确认
- 样式：待确认

## 验收证据

- URL：未实现
- 视口：未截图
- 目标图：\`${record.evidence.target}\`
- 实现截图：未生成
- diff 图：未生成
- 控制台/网络：未验证

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|

## 结论

- 是否 pixel-accepted：否
- 未解决问题：尚未完成逐图人工精拆、实现截图和视觉 diff。
- 下一步：按目标图进行区域测量、OCR 校正、组件/图标确认和实现映射。
`;
}

function reviewTemplate(record) {
  return `# ${record.id}.png 精拆 Review

- 状态：\`${record.status}\`
- 源图：\`${record.source_image}\`
- 证据目录：\`${record.evidence_dir}\`

## 人工视觉校正

- [ ] 已查看目标 PNG
- [ ] 已校正区域 bbox
- [ ] 已校正文案/OCR
- [ ] 已确认组件与元素
- [ ] 已确认图标
- [ ] 已确认 token 与样式
- [ ] 已确认交互状态

## Windows Chrome 视觉验收

- [ ] 已用 Windows Chrome 截图
- [ ] 已生成 implementation.png
- [ ] 已生成 diff.png
- [ ] 已检查布局、文本、菜单、交互状态、错误提示、视觉重叠和响应式问题

## 未解决项

- 尚未开始。
`;
}

function main() {
  const args = parseArgs();
  const imageAbs = canonicalImagePath(args.image);
  const imageRel = rel(imageAbs);
  const category = categoryFromImage(imageAbs);
  const id = path.basename(imageAbs, '.png');
  const dims = pngDimensions(imageAbs);
  const outDir = path.join(BREAKDOWN_DIR, category);
  const evidenceDir = path.join(EVIDENCE_ROOT, category, id);
  const mdFile = path.join(outDir, `${id}.md`);
  const jsonFile = path.join(outDir, `${id}.json`);
  const reviewFile = path.join(outDir, `${id}.review.md`);
  if (!args.force && (fs.existsSync(mdFile) || fs.existsSync(jsonFile) || fs.existsSync(reviewFile))) {
    throw new Error(`single-image record already exists for ${category}/${id}; use --force only after reviewing existing work`);
  }

  fs.mkdirSync(outDir, { recursive: true });
  fs.mkdirSync(evidenceDir, { recursive: true });
  fs.copyFileSync(imageAbs, path.join(evidenceDir, 'target.png'));

  const route = findRoute(id, category);
  const manifestItem = findManifestItem(id, imageRel);
  const record = {
    id,
    category,
    source_image: imageRel,
    canvas: dims,
    status: 'draft',
    route: route?.route ?? null,
    host_route: null,
    prompt: promptPathFor(id),
    manifest_item: manifestItem ? { id: manifestItem.id, type: manifestItem.type, title: manifestItem.title } : null,
    regions: [],
    texts: [],
    components: [],
    icons: [],
    tokens: [],
    interactions: [],
    implementation: { pages: [], components: [], services: [], styles: [] },
    evidence: {
      target: rel(path.join(evidenceDir, 'target.png')),
      implementation: '',
      diff: '',
      regions_overlay: '',
      viewport: '',
      url: '',
    },
    evidence_dir: rel(evidenceDir),
    differences: [],
    unresolved: ['manual_visual_breakdown_pending', 'windows_chrome_screenshot_pending', 'visual_diff_pending'],
    accepted: false,
  };

  fs.writeFileSync(jsonFile, `${JSON.stringify(record, null, 2)}\n`);
  fs.writeFileSync(mdFile, mdTemplate(record));
  fs.writeFileSync(reviewFile, reviewTemplate(record));
  console.log(`${rel(mdFile)} created`);
}

main();

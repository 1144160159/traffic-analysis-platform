// 多模态视觉开发回路:Playwright 无头 Chromium 截图调度中心各页面,
// 产出 PNG 供视觉模型审阅(read_image),可选 --ocr 用 tesseract(chi_sim+eng)
// 提取页面文字,为文本型模型(如 deepseek-v4-pro)提供"页面可见内容"视觉桥。
// 用法:node scripts/visual-inspect.cjs [baseURL] [outDir] [--ocr]
//   默认 baseURL=http://127.0.0.1:5173(dev server,MSW mock 数据),outDir=/tmp/ui-shots
const { chromium } = require('playwright');
const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const baseURL = process.argv[2] || 'http://127.0.0.1:5173';
const outDir = process.argv[3] || '/tmp/ui-shots';
const withOCR = process.argv.includes('--ocr');

const pages = [
  { path: '/login', name: 'login', width: 1440, height: 900 },
  { path: '/analysis/tasks', name: 'analysis-tasks', width: 1440, height: 900 },
  { path: '/analysis/schedules', name: 'analysis-schedules', width: 1440, height: 900 },
  { path: '/analysis/orchestration', name: 'analysis-orchestration', width: 1440, height: 900 },
  { path: '/analysis/runs', name: 'analysis-runs', width: 1440, height: 900 },
  { path: '/analysis/reports', name: 'analysis-reports', width: 1440, height: 900 },
  { path: '/analysis/resources', name: 'analysis-resources', width: 1440, height: 900 },
  { path: '/analysis/task-definitions/d2d479ee-2e20-42c6-b4eb-1d4c1921d496', name: 'analysis-task-def-detail', width: 1440, height: 900 },
  { path: '/analysis/runs', name: 'analysis-runs-mobile', width: 390, height: 844 },
];

(async () => {
  fs.mkdirSync(outDir, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  for (const p of pages) {
    const page = await browser.newPage({ viewport: { width: p.width, height: p.height } });
    const errors = [];
    page.on('pageerror', (e) => errors.push(`pageerror: ${e.message}`));
    page.on('console', (m) => {
      if (m.type() === 'error') errors.push(`console: ${m.text().slice(0, 160)}`);
    });
    try {
      await page.goto(baseURL + p.path, { waitUntil: 'networkidle', timeout: 30000 });
      await page.waitForTimeout(1200); // 等 mock 数据/动画稳定
      const file = path.join(outDir, `${p.name}.png`);
      await page.screenshot({ path: file, fullPage: false });
      const body = await page.evaluate(() => document.body.innerText.slice(0, 120).replace(/\n/g, ' '));
      // DOM 文本快照(干净通道):文本型模型读此文件获得页面完整可见内容
      const domText = await page.evaluate(() => document.body.innerText);
      const domFile = file.replace(/\.png$/, '.dom.txt');
      fs.writeFileSync(domFile, domText);
      console.log(`[OK] ${p.name} (${p.width}x${p.height}) -> ${file} | text: ${body}`);
      console.log(`  [DOM] ${domFile} (${domText.trim().split(/\s+/).length} tokens)`);
      if (errors.length) console.log(`  [WARN] ${errors.slice(0, 3).join(' | ')}`);
      if (withOCR) {
        try {
          const ocrFile = file.replace(/\.png$/, '.ocr.txt');
          const ocr = execFileSync('tesseract', [file, 'stdout', '-l', 'chi_sim+eng', '--psm', '6'], { encoding: 'utf8', timeout: 30000 });
          fs.writeFileSync(ocrFile, ocr);
          console.log(`  [OCR] ${ocrFile} (${ocr.trim().split(/\s+/).length} tokens)`);
        } catch (oe) {
          console.log(`  [OCR-FAIL] ${oe.message.split('\n')[0]}`);
        }
      }
    } catch (e) {
      console.log(`[FAIL] ${p.name}: ${e.message.split('\n')[0]}`);
    } finally {
      await page.close();
    }
  }
  await browser.close();
  console.log('DONE');
})();

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const pagePath = path.join(sourceRoot, 'pages', 'GraphEntityPage.tsx');
const stylesPath = path.join(sourceRoot, 'styles', 'pages.css');

const read = (filePath: string) => fs.readFileSync(filePath, 'utf8');

describe('entity graph authority and empty-state semantics', () => {
  it('labels browser-local query history and JSON exports without claiming server evidence', () => {
    const page = read(pagePath);

    expect(page).toContain('查询治理（当前浏览器）');
    expect(page).toContain('耗时历史仅来自当前浏览器已返回的查询，不代表服务端全局指标。');
    expect(page).toContain('本浏览器查询历史');
    expect(page).toContain('最近查询（本地）');
    expect(page).toContain('本地慢查询');
    expect(page).toContain('本地平均耗时');
    expect(page).toContain('导出视图 JSON');
    expect(page).toContain('该文件不是服务端证据对象，也不替代审计导出。');
    expect(page).not.toContain('>导出证据</Button>');
    expect(page).not.toContain('实体、关系与证据索引已导出。');
  });

  it('centers the tunnel empty state below its legend instead of overlapping it', () => {
    const css = read(stylesPath);
    const selector = '.taf-topic-tunnel-impact > .taf-topic-empty';
    const ruleStart = css.lastIndexOf(`${selector} {`);
    const rule = ruleStart >= 0 ? css.slice(ruleStart, css.indexOf('}', ruleStart) + 1) : '';

    expect(rule).toContain('position: absolute;');
    expect(rule).toContain('inset: 34px 12px 12px;');
    expect(rule).toContain('place-items: center;');
    expect(rule).toContain('text-align: center;');
  });
});

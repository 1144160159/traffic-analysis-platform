import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const pagePath = path.join(sourceRoot, 'pages', 'DataQualityPage.tsx');
const stylesPath = path.join(sourceRoot, 'styles', 'pages.css');

const read = (filePath: string) => fs.readFileSync(filePath, 'utf8');

function lastRuleBlock(css: string, selector: string) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replace(/\\ /g, '\\s+');
  const rule = new RegExp(`${escaped}\\s*\\{([^}]*)\\}`, 'g');
  const matches = [...css.matchAll(rule)];
  return matches.length > 0 ? matches[matches.length - 1][1] : '';
}

describe('data quality tab geometry', () => {
  it('keeps the eight data-quality tabs in fixed equal slots across states', () => {
    const page = read(pagePath);
    const css = read(stylesPath);
    const titlebar = lastRuleBlock(css, '.taf-data-quality-shell.is-unified-tabs > .taf-data-quality-titlebar');
    const shellTabs = lastRuleBlock(css, '.taf-data-quality-shell.is-unified-tabs > .taf-data-quality-titlebar .taf-data-quality-tabs');
    const tabButton = lastRuleBlock(css, '.taf-data-quality-shell.is-unified-tabs .taf-data-quality-tabs button');

    expect(page.match(/slug:\s*["']/g)).toHaveLength(8);
    expect(page).toContain('data-tab-slot={index + 1}');
    expect(page).toContain('title={tab.label}');
    expect(titlebar).toContain('position: sticky;');
    expect(titlebar).toContain('grid-column: 1 / -1;');
    expect(shellTabs).toContain('grid-template-columns: repeat(8, minmax(0, 1fr));');
    expect(shellTabs).toContain('width: 100%;');
    expect(shellTabs).toContain('overflow: hidden;');
    expect(shellTabs).not.toContain('overflow-x: auto');
    expect(css).toContain('grid-template-columns: minmax(0, 1fr) 320px;');
    expect(css).toContain('grid-template-rows: 52px auto;');
    expect(tabButton).toContain('width: 100%;');
    expect(tabButton).toContain('white-space: nowrap;');
    expect(tabButton).toContain('text-overflow: ellipsis;');
  });

  it('uses API-backed ECharts, refresh controls, actionable field details, and table pagination', () => {
    const page = read(pagePath);
    const normalizedPage = page.split('"').join("'");
    const css = read(stylesPath);

    expect(page).toContain('DataQualityDonutChart');
    expect(page).toContain("scoreValue === null ? '暂不可用'");
    expect(page).toContain("'质量总分暂不可用'");
    expect(page).toContain('DataQualityFieldTrendChart');
    expect(page).toContain('DataQualityHeatmapChart');
    expect(page).toContain('DataQualityKpiSparklineChart');
    expect(page).toContain('DataQualityTrendChart');
    expect(page).toContain('<DataQualityFieldTrendChart');
    expect(page).toContain('<DataQualityKpiSparklineChart');
    expect(page).toContain('<DataQualityTrendChart');
    expect(page).toContain('<DataQualityDonutChart');
    expect(page).toContain('<DataQualityHeatmapChart');
    expect(page).toContain('fieldKpiTrends');
    expect(page).toContain('values: chart[key]');
    expect(page).toContain('refetchInterval: autoRefresh && !isVisualBreakdown ? 30_000 : false');
    expect(page).toContain('aria-pressed={autoRefresh}');
    expect(page).toContain('onClick={() => onAutoRefreshChange(!autoRefresh)}');
    expect(normalizedPage).toContain("queryKey: ['page-snapshot', route.id, timeRange]");
    expect(normalizedPage).toContain("fetchPageSnapshot(route.id, { dataQualityTimeRange: timeRange })");
    expect(page).toContain('onChange={onTimeRangeChange}');
    expect(page).toContain('className="taf-data-quality-field-detail-drawer"');
    expect(page).toContain('function FieldQualitySideRail');
    expect(page).toContain('<DataUnavailable section="字段质量侧栏统计" />');
    expect(page).toContain('function DataQualityPagination');
    expect(page).toContain('function useDataQualityPagination');
    expect(page).toContain('fetchDataQualityTablePage');
    expect(page).toContain('data-pagination-source={dataset ?');
    expect(page).toContain('aria-label={`${label}分页`}');
    expect(page).toContain('className="taf-data-quality-field-table-rows"');
    expect(page).toContain('return <DataUnavailable section="质量设置" />;');
    expect(page).toContain('<QualityCheckLinks checks={snapshot?.dataQualityChecks} mode="locate"');
    expect(page).toContain('<QualityCheckLinks checks={snapshot?.dataQualityChecks} mode="repair"');
    expect(page).toContain('data-check-measured={String(check.measured)}');
    expect(page).toContain('data-check-source={check.source}');
    expect(page.match(/data-dq-action-managed="true"/gu)?.length ?? 0).toBeGreaterThanOrEqual(3);
    expect(page).not.toContain('<DataUnavailable section="快速定位" />');
    expect(page).not.toContain('<DataUnavailable section="质量修复建议" />');
    expect(css).toContain('grid-auto-rows: minmax(30px, auto);');
    expect(css).toContain('align-content: start;');
    expect(css).toContain('scrollbar-gutter: stable;');
    expect(css).toContain('.taf-data-quality-field-pagination');
    expect(css).toContain('.taf-data-quality-paged-table');
    expect(css).toContain('flex: 0 0 34px;');
  });

  it('never substitutes static business facts for missing production snapshot sections', () => {
    const page = read(pagePath);

    expect(page).not.toContain('?? topicHealthFallbackMetrics');
    expect(page).not.toContain('?? flinkQualityFallbackMetrics');
    expect(page).not.toContain('?? fieldQualityFallbackMetrics');
    expect(page).not.toContain('?? storageQualityFallbackMetrics');
    expect(page).not.toContain('?? replayReconcileFallbackMetrics');
    expect(page).not.toMatch(/rows\?\.length\s*\?\s*rows\s*:\s*\w*Fallback/);
    expect(page).not.toContain('|| 92');
    expect(page).not.toContain('rows={replayRows}');
    expect(page).not.toContain("const rangeLabel = '2025-");
    expect(page).not.toContain('buildFallbackHeatmap(');
    expect(page).not.toContain('buildFallbackFlinkMetrics(');
    expect(page).toContain('页面不会以静态业务数据替代');
  });
});

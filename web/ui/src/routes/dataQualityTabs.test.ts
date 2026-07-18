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
    expect(page).toContain('function FieldQualityRailLinks');
    expect(page).toContain('function DataQualityPagination');
    expect(page).toContain('function useDataQualityPagination');
    expect(page).toContain('fetchDataQualityTablePage');
    expect(page).toContain('data-pagination-source={dataset ?');
    expect(page).toContain('aria-label={`${label}分页`}');
    expect(page).toContain('className="taf-data-quality-field-table-rows"');
    expect(page).toContain('onOpenReplayReconcile={openReplayReconcile}');
    expect(css).toContain('scrollbar-gutter: stable;');
    expect(css).toContain('.taf-data-quality-field-pagination');
    expect(css).toContain('.taf-data-quality-paged-table');
    expect(css).toContain('flex: 0 0 34px;');
  });
});

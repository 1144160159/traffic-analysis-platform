import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const page = fs.readFileSync(path.join(sourceRoot, 'pages', 'CampaignWorkbenchPage.tsx'), 'utf8');
const styles = fs.readFileSync(path.join(sourceRoot, 'styles', 'pages.css'), 'utf8');

describe('campaign workbench business contract', () => {
  it('uses dynamic ECharts instead of decorative progress rings', () => {
    expect(page).toContain("import { DataQualityDonutChart } from '@/components/charts'");
    expect(page.match(/<DataQualityDonutChart/g)).toHaveLength(2);
    expect(page).toContain('战役风险分布动态图');
    expect(page).toContain('战役证据完整度动态图');
    expect(page).not.toContain('<Progress');
  });

  it('keeps filters, pagination, and all visible actions interactive', () => {
    expect(page).toContain('filterDraft');
    expect(page).toContain('setAppliedFilters(filterDraft)');
    expect(page).toContain('onPressEnter={onSubmit}');
    expect(page).toContain('current: page');
    expect(page).toContain('setPageSize(visualBreakdownMode ? 10 : 8)');
    expect(page).toContain('setPageSize(nextPageSize)');
    expect(page).toContain("queryKey: ['page-snapshot', route.id, requestPage, requestPageSize, appliedFilters]");
    expect(page).toContain('fetchPageSnapshot(route.id, { page: requestPage, pageSize: requestPageSize, campaignFilters: appliedFilters })');
    expect(page).toContain("message.success('已提交服务端查询')");
    expect(page).toContain('visualBreakdownMode ? buildCampaignSimulationRows(apiRows, campaignTotal) : apiRows');
    expect(page).toContain('showTotal: (total) => `共 ${total} 条`');
    expect(page).toContain("submitCampaignAction");
    expect(page).toContain("'campaign-export'");
    expect(page).toContain("'campaign-phase-inspect'");
    expect(page).toContain("'campaign-impact-inspect'");
    expect(page).toContain("'campaign-evidence-view'");
    expect(page).toContain("'campaign-soar-response'");
    expect(page).toContain('导出当前页');
    expect(page).toContain('模拟变更状态');
    expect(page).toContain('模拟 SOAR 处置');
    expect(page).toContain("percent === undefined ? []");
    expect(page).not.toContain('const ratios = items');
    expect(page).toContain('className="taf-campaign-action-drawer"');
    expect(styles).toContain('.taf-campaign-list-panel .ant-table-content');
    expect(styles).toContain('scrollbar-gutter: stable;');
  });
});

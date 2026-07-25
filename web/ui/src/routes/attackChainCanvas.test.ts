import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const pagePath = path.join(sourceRoot, 'pages', 'AttackChainAnalysisPage.tsx');
const adapterPath = path.join(sourceRoot, 'services', 'pageSnapshotAdapters.ts');

const read = (filePath: string) => fs.readFileSync(filePath, 'utf8');

describe('attack chain visual implementation', () => {
  it('keeps attack chain analysis data-driven and free of target bitmap replay', () => {
    const page = read(pagePath);
    const adapter = read(adapterPath);

    expect(page).toContain("queryFn: () => fetchPageSnapshot(route.id)");
    expect(page).toContain('queryFn: () => fetchAttackChainDetail(selectedChainId)');
    expect(page).toContain('<AttackCanvas phases=');
    expect(page).toContain('<EvidenceAnchorList rows=');
    expect(page).toContain('<ResponseRecommendations rows=');
    expect(page).toContain("runChainAction('campaign-evidence-view'");
    expect(page).toContain("runChainAction('campaign-soar-response'");
    expect(page).toContain('<PathDetail rows={rows} columns={columns} isLoading={activeLoading} />');
    expect(page).toContain('<AttackCanvas phases={scopedPhaseRows} viewMode={viewMode} />');
    expect(page).toContain('当前资产范围没有匹配的攻击链阶段');
    expect(page).toContain("if (viewMode === '泳道视图')");
    expect(page).toContain("if (viewMode === '矩阵视图')");
    expect(page).toContain("runChainAction('campaign-export'");
    expect(page).toContain('rootClassName="taf-attack-chain-detail-drawer"');
    expect(page).toContain('placement="right"');
    expect(page).toContain('width="min(900px, calc(100dvw - 32px))"');
    expect(page).toContain('aria-label="关闭攻击链详情"');
    expect(page).toContain('className="taf-detail-drawer-tabs"');
    expect(page).toContain("{ key: 'evidence', label: '证据' }");
    expect(page).toContain("{ key: 'audit', label: '审计' }");
    expect(page).toContain("next.set('detailTab', tab)");
    expect(page).toContain("searchParams.get('chain')");
    expect(page).toContain("next.delete('campaign')");
    expect(page).toContain('phase?.key_events.some');
    expect(page).toContain("? chains.find((chain) => chain.chain_id === sourceCampaign)?.chain_id");
    expect(page).toContain("setActiveTab(requestedTab && ['detail', 'evidence', 'audit'].includes(requestedTab) ? requestedTab : 'detail')");
    expect(page).toContain("next.set('drawer', 'attack-chain-detail')");
    expect(page).toContain("next.delete('drawer')");
    expect(page).toContain('okButtonProps={{ loading: pending }}');
    expect(page).toContain('disabled={pending}');
    expect(page).toContain('确认提交调查结论？');
    expect(page).not.toMatch(/<Modal[\s\S]*className="taf-attack-chain-detail-drawer"/);
    expect(page).not.toMatch(/attack-chains\.(?:png|jpe?g|webp)/i);
    expect(page).not.toMatch(/implementation\.html/i);
    expect(page).not.toMatch(/ui-image-breakdowns/i);
    expect(page).not.toMatch(/screens\/pages/i);

    expect(adapter).toContain("if (page.id === 'attack-chains') return adaptAttackChains(page, primaryPayload);");
    expect(adapter).toContain('const adaptAttackChains');
    expect(adapter).toContain("evidence('Attack Chains API', '/v1/attack-chains', 'ok')");
  });
});

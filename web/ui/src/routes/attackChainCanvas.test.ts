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
    expect(page).toContain('<AttackCanvas />');
    expect(page).toContain('<EvidenceAnchorList />');
    expect(page).toContain('<ResponseRecommendations />');
    expect(page).toContain('<PathDetail rows={rows} columns={columns} isLoading={isLoading} />');
    expect(page).not.toMatch(/attack-chains\.(?:png|jpe?g|webp)/i);
    expect(page).not.toMatch(/implementation\.html/i);
    expect(page).not.toMatch(/ui-image-breakdowns/i);
    expect(page).not.toMatch(/screens\/pages/i);

    expect(adapter).toContain("if (page.id === 'attack-chains') return adaptAttackChains(page, primaryPayload);");
    expect(adapter).toContain('const adaptAttackChains');
    expect(adapter).toContain("evidence('Attack Chains API', '/v1/attack-chains', 'ok')");
  });
});

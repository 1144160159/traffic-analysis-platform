import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const pagePath = path.join(sourceRoot, 'pages', 'CampaignDetailPage.tsx');
const stylesPath = path.join(sourceRoot, 'styles', 'pages.css');
const shellStylesPath = path.join(sourceRoot, 'styles', 'app-shell.css');

const read = (filePath: string) => fs.readFileSync(filePath, 'utf8');

function lastRuleBlock(css: string, selector: string) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replace(/\\ /g, '\\s+');
  const rule = new RegExp(`${escaped}\\s*\\{([^}]*)\\}`, 'g');
  const matches = [...css.matchAll(rule)];
  return matches.length > 0 ? matches[matches.length - 1][1] : '';
}

describe('campaign impact focus canvas', () => {
  it('reserves enough of the normal campaign detail column for the impact table', () => {
    const css = read(stylesPath);
    const detailMain = lastRuleBlock(css, '.taf-campaign-detail-page.is-redesigned .taf-campaign-detail-main');
    const normalContent = lastRuleBlock(css, '.taf-campaign-detail-page.is-redesigned .taf-campaign-detail-impact-panel .taf-campaign-impact-account-content');
    const normalTable = lastRuleBlock(css, '.taf-campaign-detail-page.is-redesigned .taf-campaign-detail-impact-panel .taf-campaign-impact-account-table-block');

    expect(detailMain).toContain('grid-template-rows: auto auto;');
    expect(css).toContain('Campaign detail r763 final override');
    expect(css).toContain('grid-template-columns: repeat(2, minmax(0, 1fr));');
    expect(css).toContain('.taf-campaign-detail-evidence-stack');
    expect(normalContent).toContain('min-height: 0;');
    expect(normalContent).toContain('grid-template-columns: minmax(0, 1fr);');
    expect(normalTable).toContain('overflow: hidden;');
  });

  it('keeps impact states data-driven inside the campaign detail card', () => {
    const page = read(pagePath);
    const css = read(stylesPath);
    const shellCss = read(shellStylesPath);
    const impactBody = lastRuleBlock(css, '.taf-campaign-detail-page.is-redesigned .taf-campaign-detail-impact-panel .taf-panel__body');

    expect(page).toContain("activeImpact === 'campus'");
    expect(page).toContain("activeImpact === 'department'");
    expect(page).toContain("activeImpact === 'service'");
    expect(page).toContain('snapshot.impactCampus');
    expect(page).toContain('snapshot.impactDepartment');
    expect(page).toContain('snapshot.impactService');
    expect(page).toContain('data-page-id="campaign-detail-impact-campus"');
    expect(page).toContain('data-page-id="campaign-detail-impact-department"');
    expect(page).toContain('data-page-id="campaign-detail-impact-service"');
    expect(page).not.toMatch(/campaign-detail-impact-campus\.(?:png|jpe?g|webp)/i);
    expect(page).not.toMatch(/campaign-detail-impact-department\.(?:png|jpe?g|webp)/i);
    expect(page).not.toMatch(/campaign-detail-impact-service\.(?:png|jpe?g|webp)/i);

    expect(page).toContain('<DataQualityDonutChart');
    expect(page).toContain('<CampaignImpactInlineContent snapshot={snapshot} activeImpact={activeImpact} />');
    expect(page).toContain('impactDestination(activeImpact)');
    expect(page).toContain('className="taf-campaign-detail-evidence-stack"');
    expect(page).toContain('title="证据摘要"');
    expect(page).toContain('className="taf-campaign-detail-evidence-digest"');
    expect(page).not.toContain('className="taf-campaign-impact-modal"');
    expect(page).not.toContain('setImpactOpen');
    expect(page).toContain("next.set('impact', nextImpact)");
    expect(page).not.toContain('if (visualBreakdownMode && activeImpact');
    expect(shellCss).not.toContain(':has(.taf-campaign-impact-account-focus)');
    expect(impactBody).toContain('display: flex;');
    expect(impactBody).toContain('overflow: hidden;');
    expect(css).toContain('@media (max-width: 900px)');
  });
});

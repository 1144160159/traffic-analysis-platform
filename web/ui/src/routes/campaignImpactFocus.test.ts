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
    const detailMain = lastRuleBlock(css, '.taf-campaign-detail-main');
    const normalContent = lastRuleBlock(css, '.taf-campaign-detail-impact-panel .taf-campaign-impact-account-content');
    const normalTable = lastRuleBlock(css, '.taf-campaign-detail-impact-panel .taf-campaign-impact-account-table-block');

    expect(detailMain).toContain('grid-template-rows: 265px minmax(430px, 1fr);');
    expect(detailMain).toContain('grid-template-columns: minmax(0, 1fr);');
    expect(normalContent).toContain('min-height: 0;');
    expect(normalTable).toContain('gap: 4px;');
    expect(normalTable).toContain('padding: 6px 8px;');
  });

  it('keeps impact states data-driven inside an intrinsic responsive modal', () => {
    const page = read(pagePath);
    const css = read(stylesPath);
    const shellCss = read(shellStylesPath);
    const focus = lastRuleBlock(css, '.taf-campaign-impact-account-focus');
    const modal = lastRuleBlock(css, '.taf-campaign-impact-modal');
    const modalContent = lastRuleBlock(css, '.taf-campaign-impact-modal .ant-modal-content');
    const modalBody = lastRuleBlock(css, '.taf-campaign-impact-modal .ant-modal-body');

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
    expect(page).toContain('className="taf-campaign-impact-modal"');
    expect(page).toContain('width="min(1040px, calc(100dvw - 96px))"');
    expect(page).toContain('centered');
    expect(page).toContain("visualPageId.startsWith('campaign-detail-impact-') || searchParams.has('impact')");
    expect(page).toContain("next.delete('impact')");
    expect(page).not.toContain('if (visualBreakdownMode && activeImpact');
    expect(shellCss).not.toContain(':has(.taf-campaign-impact-account-focus)');
    expect(css).toContain('max-width: calc(100dvw - 96px);');
    expect(modal).toContain('max-width: calc(100dvw - 32px);');
    expect(modalContent).toContain('calc(100dvh - 96px)');
    expect(modalBody).toContain('overflow: auto;');
    expect(css).toContain('.taf-campaign-impact-modal .taf-campaign-impact-account-focus');
    expect(css).not.toContain('min-height: 664px;');
    expect(css).not.toContain('min-height: 680px;');
    expect(focus).toContain('width: 100%;');
    expect(focus).toContain('height: 100%;');
    expect(focus).toContain('transform: none;');
    expect(css).not.toContain('transform: scale(var(--taf-campaign-focus-scale));');
    expect(css).toContain('width: clamp(180px, 20vw, 280px);');
    expect(css).toContain('@media (max-width: 980px)');
  });
});

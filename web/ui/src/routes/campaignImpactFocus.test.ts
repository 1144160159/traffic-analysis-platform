import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const pagePath = path.join(sourceRoot, 'pages', 'CampaignDetailPage.tsx');
const stylesPath = path.join(sourceRoot, 'styles', 'pages.css');

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

    expect(detailMain).toContain('grid-template-rows: minmax(0, 2.1fr) minmax(0, 1fr);');
    expect(detailMain).not.toContain('grid-template-rows: 278px');
    expect(normalContent).toContain('min-height: 0;');
    expect(normalTable).toContain('gap: 4px;');
    expect(normalTable).toContain('padding: 6px 8px;');
  });

  it('keeps the service, campus, and department states data-driven and scales their design canvas to the real viewport', () => {
    const page = read(pagePath);
    const css = read(stylesPath);
    const host = lastRuleBlock(css, '.taf-campaign-impact-account-visual-page');
    const focus = lastRuleBlock(css, '.taf-campaign-impact-account-focus');
    const entityFocus = lastRuleBlock(css, '.taf-campaign-impact-entity-focus');

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

    expect(host).toContain('place-items: center;');
    expect(host).toContain('overflow: hidden;');
    expect(focus).toContain('width: 2133px;');
    expect(focus).toContain('height: 1200px;');
    expect(focus).toContain('calc(100dvw / 2133px)');
    expect(focus).toContain('calc(100dvh / 1200px)');
    expect(focus).toContain('transform: scale(var(--taf-campaign-focus-scale));');
    expect(entityFocus).toContain('width: 2097px;');
    expect(entityFocus).toContain('height: 1180px;');
    expect(entityFocus).not.toContain('--taf-window-inner-width');
    expect(entityFocus).not.toContain('--taf-window-inner-height');
  });
});

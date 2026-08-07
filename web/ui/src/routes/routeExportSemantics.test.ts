import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const page = (name: string) => fs.readFileSync(path.join(sourceRoot, 'pages', name), 'utf8');

describe('cross-page export semantics', () => {
  it('does not turn accepted campaign actions into browser-generated server artifacts', () => {
    const list = page('CampaignWorkbenchPage.tsx');
    const detail = page('CampaignDetailPage.tsx');
    const attackChain = page('AttackChainAnalysisPage.tsx');

    expect(list).toContain('服务端制品与下载终态尚未返回，本次不会生成浏览器伪制品');
    expect(list).not.toContain('new Blob(');
    expect(detail).toContain("reportMutation.mutate({\n      format: 'json'");
    expect(detail).not.toContain('new Blob(');
    expect(attackChain).toContain("runChainAction('campaign-report-generate', '导出攻击链报告'");
    expect(attackChain).toContain('请等待服务端终态后下载');
    expect(attackChain).not.toContain('new Blob(');
  });

  it('labels the model browser snapshot as local and non-audit evidence', () => {
    const models = page('ModelManagementPage.tsx');
    expect(models).toContain("scope: 'current-browser-snapshot', audit_evidence: false");
    expect(models).toContain('下载本地快照 JSON');
    expect(models).toContain('不是服务端审计证据');
    expect(models).not.toContain('生成审计报告');
  });
});

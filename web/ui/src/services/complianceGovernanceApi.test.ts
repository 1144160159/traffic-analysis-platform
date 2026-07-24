import { beforeEach, describe, expect, it, vi } from 'vitest';

const get = vi.fn();
const post = vi.fn();

vi.mock('@/services/api', () => ({ api: { get, post } }));

describe('compliance governance API', () => {
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
  });

  it('loads tenant reports and audit trail through dedicated endpoints', async () => {
    get.mockResolvedValueOnce({ data: { data: { reports: [{ report_id: 'r1' }], total: 1 } } });
    get.mockResolvedValueOnce({ data: { data: { trails: [{ log_id: 'a1' }], total: 1 } } });
    const api = await import('./complianceGovernanceApi');
    expect((await api.fetchComplianceReports()).total).toBe(1);
    expect((await api.fetchComplianceAuditTrail()).total).toBe(1);
    expect(get).toHaveBeenNthCalledWith(1, '/v1/compliance/reports', { params: { limit: 50 } });
    expect(get).toHaveBeenNthCalledWith(2, '/v1/compliance/audit-trail', { params: { limit: 50 } });
  });

  it('generates reports with an explicit report type', async () => {
    post.mockResolvedValue({ data: { data: { report_id: 'r2', status: 'completed' } } });
    const api = await import('./complianceGovernanceApi');
    const report = await api.generateComplianceReport({ reportType: 'weekly' });
    expect(report.report_id).toBe('r2');
    expect(post).toHaveBeenCalledWith('/v1/compliance/reports/generate', { report_type: 'weekly' });
  });

  it('requests a server-generated evidence package', async () => {
    post.mockResolvedValue({ data: { data: { export_id: 'e1', report_id: 'r1', sha256: 'sha256:abc' } } });
    const api = await import('./complianceGovernanceApi');
    const bundle = await api.exportComplianceEvidencePackage('r1');
    expect(bundle.sha256).toBe('sha256:abc');
    expect(post).toHaveBeenCalledWith('/v1/compliance/reports/r1/evidence-package');
  });

  it('uses dedicated report export, remediation, and finalization endpoints', async () => {
    post
      .mockResolvedValueOnce({ data: { data: { export_id: 'x1', artifact_type: 'report_pdf' } } })
      .mockResolvedValueOnce({ data: { data: { report_id: 'r1', tasks: [], total: 0 } } })
      .mockResolvedValueOnce({ data: { data: { finalization_id: 'f1', status: 'finalized' } } });
    const api = await import('./complianceGovernanceApi');
    expect((await api.exportComplianceReport('r1', 'pdf')).artifact_type).toBe('report_pdf');
    expect((await api.createComplianceRemediations('r1')).total).toBe(0);
    expect((await api.finalizeComplianceReport('r1')).status).toBe('finalized');
    expect(post).toHaveBeenNthCalledWith(1, '/v1/compliance/reports/r1/export', { format: 'pdf' });
    expect(post).toHaveBeenNthCalledWith(2, '/v1/compliance/reports/r1/remediations', {});
    expect(post).toHaveBeenNthCalledWith(3, '/v1/compliance/reports/r1/finalize', {});
  });
});

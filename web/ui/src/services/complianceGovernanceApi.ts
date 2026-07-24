import { api } from "@/services/api";

export type ComplianceSummary = {
  total_alerts: number;
  critical_alerts: number;
  resolved_alerts: number;
  false_positives: number;
  avg_response_time_min: number;
  sla_violations: number;
};

export type ComplianceSection = {
  section_name: string;
  title: string;
  content: Record<string, unknown>;
  status: "pass" | "warn" | "blocked" | "insufficient_evidence" | string;
};

export type ComplianceReport = {
  report_id: string;
  tenant_id: string;
  report_type: "weekly" | "monthly" | "custom" | string;
  time_range: { start: number; end: number };
  generated_at: number;
  generated_by: string;
  status: "completed" | "insufficient_evidence" | string;
  summary: ComplianceSummary;
  sections: ComplianceSection[];
};

export type ComplianceAuditTrail = {
  log_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  details: Record<string, unknown>;
  timestamp: number;
  result: string;
};

type Envelope<T> = { success: boolean; data: T };

export async function fetchComplianceReports(): Promise<{
  reports: ComplianceReport[];
  total: number;
}> {
  const response = await api.get<
    Envelope<{ reports: ComplianceReport[]; total: number }>
  >("/v1/compliance/reports", { params: { limit: 50 } });
  return response.data.data;
}

export async function fetchComplianceAuditTrail(): Promise<{
  trails: ComplianceAuditTrail[];
  total: number;
}> {
  const response = await api.get<
    Envelope<{ trails: ComplianceAuditTrail[]; total: number }>
  >("/v1/compliance/audit-trail", { params: { limit: 50 } });
  return response.data.data;
}

export async function generateComplianceReport(input: {
  reportType: "weekly" | "monthly" | "custom";
  start?: number;
  end?: number;
}): Promise<ComplianceReport> {
  const body: Record<string, unknown> = { report_type: input.reportType };
  if (input.reportType === "custom")
    body.time_range = { start: input.start, end: input.end };
  const response = await api.post<Envelope<ComplianceReport>>(
    "/v1/compliance/reports/generate",
    body,
  );
  return response.data.data;
}

export type ComplianceArtifact = {
  export_id: string;
  report_id: string;
  artifact_type: "evidence_package" | "report_pdf" | "report_docx";
  filename: string;
  mime_type: string;
  sha256: string;
  content_base64: string;
  generated_at: number;
};

export async function exportComplianceEvidencePackage(
  reportId: string,
): Promise<ComplianceArtifact> {
  const response = await api.post<Envelope<ComplianceArtifact>>(
    `/v1/compliance/reports/${encodeURIComponent(reportId)}/evidence-package`,
  );
  return response.data.data;
}

export async function exportComplianceReport(
  reportId: string,
  format: "pdf" | "docx",
): Promise<ComplianceArtifact> {
  const response = await api.post<Envelope<ComplianceArtifact>>(
    `/v1/compliance/reports/${encodeURIComponent(reportId)}/export`,
    { format },
  );
  return response.data.data;
}

export async function createComplianceRemediations(reportId: string): Promise<{
  report_id: string;
  tasks: Array<{
    task_id: string;
    section_name: string;
    title: string;
    status: string;
  }>;
  total: number;
  created: number;
  reused: number;
}> {
  const response = await api.post<
    Envelope<{
      report_id: string;
      tasks: Array<{
        task_id: string;
        section_name: string;
        title: string;
        status: string;
      }>;
      total: number;
      created: number;
      reused: number;
    }>
  >(`/v1/compliance/reports/${encodeURIComponent(reportId)}/remediations`, {});
  return response.data.data;
}

export async function finalizeComplianceReport(reportId: string): Promise<{
  finalization_id: string;
  report_id: string;
  report_sha256: string;
  status: string;
  finalized_at: number;
}> {
  const response = await api.post<
    Envelope<{
      finalization_id: string;
      report_id: string;
      report_sha256: string;
      status: string;
      finalized_at: number;
    }>
  >(`/v1/compliance/reports/${encodeURIComponent(reportId)}/finalize`, {});
  return response.data.data;
}

export function downloadComplianceArtifact(bundle: ComplianceArtifact): void {
  const binary = window.atob(bundle.content_base64);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1)
    bytes[index] = binary.charCodeAt(index);
  const url = URL.createObjectURL(
    new Blob([bytes], { type: bundle.mime_type || "application/zip" }),
  );
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download =
    bundle.filename || `compliance-evidence-${bundle.report_id}.zip`;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

export const downloadComplianceEvidencePackage = downloadComplianceArtifact;

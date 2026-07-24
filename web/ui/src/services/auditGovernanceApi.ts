import { api } from '@/services/api';

export type AuditRisk = 'low' | 'medium' | 'high' | 'critical' | string;

export type AuditLogRecord = {
  log_id: string;
  tenant_id: string;
  user_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  details: Record<string, unknown>;
  ip_address: string;
  user_agent?: string;
  request_id?: string;
  trace_id?: string;
  timestamp: number;
  result: string;
  risk?: AuditRisk;
};

export type AuditLogFilters = {
  limit?: number;
  offset?: number;
  log_id?: string;
  user_id?: string;
  action?: string;
  object_type?: string;
  object_id?: string;
  result?: string;
  risk?: string;
  request_id?: string;
  trace_id?: string;
  start?: number;
  end?: number;
};

export type AuditLogList = {
  trails: AuditLogRecord[];
  total: number;
  summary?: {
    today?: number;
    failed: number;
    high_risk: number;
    exports: number;
    pcap_access: number;
    integrity_rate: number;
  };
  retention?: {
    retention_days: number;
    archived_until?: number;
    archive_location: string;
    integrity_rate: number;
    masked_rate: number;
    last_checked_at?: number;
  };
};

export type AuditArtifact = {
  export_id: string;
  filename: string;
  mime_type: string;
  sha256: string;
  content_base64: string;
  row_count: number;
  total_matching: number;
  truncated: boolean;
  mask_sensitive: boolean;
  generated_at: number;
};

export type AuditReview = {
  review_id: string;
  log_id: string;
  status: string;
  reason: string;
  created_at: number;
};

export type AuditIntegrityCheck = {
  check_id: string;
  status: 'passed' | 'failed' | 'baseline_created' | 'no_records' | string;
  records_checked: number;
  valid: boolean;
  baseline_created?: boolean;
  matched?: number;
  baselined?: number;
  mismatched?: number;
  added?: number;
  missing?: number;
  root_sha256: string;
  checked_at: number;
};

type Envelope<T> = { success: boolean; data: T };

export async function fetchAuditLogs(filters: AuditLogFilters = {}): Promise<AuditLogList> {
  const response = await api.get<Envelope<AuditLogList>>('/v1/audit/logs', { params: filters });
  return response.data.data;
}

export async function fetchAuditLogDetail(logId: string): Promise<AuditLogRecord> {
  const response = await api.get<Envelope<AuditLogRecord>>(`/v1/audit/logs/${encodeURIComponent(logId)}`);
  return response.data.data;
}

export async function saveAuditQuery(input: { name: string; filters: AuditLogFilters }): Promise<{
  query_id: string;
  name: string;
  created_at: number;
}> {
  const response = await api.post<Envelope<{ query_id: string; name: string; created_at: number }>>('/v1/audit/saved-queries', input);
  return response.data.data;
}

export async function exportAuditLogs(input: { format: 'pdf' | 'csv' | 'json'; filters: AuditLogFilters; mask_sensitive?: boolean }): Promise<AuditArtifact> {
  const response = await api.post<Envelope<AuditArtifact>>('/v1/audit/exports', input);
  return response.data.data;
}

export async function createAuditReview(input: { log_id: string; reason: string }): Promise<AuditReview> {
  const response = await api.post<Envelope<AuditReview>>('/v1/audit/reviews', input);
  return response.data.data;
}

export async function verifyAuditIntegrity(filters: AuditLogFilters): Promise<AuditIntegrityCheck> {
  const response = await api.post<Envelope<AuditIntegrityCheck>>('/v1/audit/integrity-checks', { filters });
  return response.data.data;
}

export function downloadAuditArtifact(artifact: AuditArtifact): void {
  const binary = window.atob(artifact.content_base64);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
  const url = URL.createObjectURL(new Blob([bytes], { type: artifact.mime_type || 'application/octet-stream' }));
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = artifact.filename || `audit-export-${artifact.export_id}`;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

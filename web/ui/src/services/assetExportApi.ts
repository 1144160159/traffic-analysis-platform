import { api } from '@/services/api';

export type AssetExportFormat = 'csv' | 'jsonl';
export type AssetExportStatus = 'accepted' | 'running' | 'completed' | 'failed' | 'cancelled';

export type AssetExportFilter = {
  asset_type?: string;
  status?: string;
  search?: string;
  department?: string;
  campus?: string;
  ip_prefix?: string;
  vendor?: string;
};

export type AssetExportJob = {
  job_id: string;
  action_id: 'asset-inventory-export';
  format: AssetExportFormat;
  status: AssetExportStatus;
  revision: number;
  columns: string[];
  filter: AssetExportFilter;
  query_sha256: string;
  reason: string;
  snapshot_id?: string;
  source_watermarks: Record<string, string>;
  row_count: number;
  mime_type?: string;
  artifact_sha256?: string;
  size_bytes: number;
  retention_until?: string;
  error_message?: string;
  attempts: number;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  idempotent_replay?: boolean;
};

export type AssetColumnPreference = {
  view_id: 'asset-inventory';
  columns: string[];
  revision: number;
  updated_at?: string;
};

type Envelope<T> = { data: T };

export async function createAssetExportJob(input: {
  format: AssetExportFormat;
  columns: string[];
  filter: AssetExportFilter;
  reason: string;
  idempotencyKey: string;
}): Promise<AssetExportJob> {
  const response = await api.post<Envelope<AssetExportJob>>('/v1/assets/exports', {
    action_id: 'asset-inventory-export',
    format: input.format,
    columns: input.columns,
    filter: input.filter,
    reason: input.reason,
  }, { headers: { 'Idempotency-Key': input.idempotencyKey } });
  return response.data.data;
}

export async function fetchAssetExportJob(jobID: string): Promise<AssetExportJob> {
  const response = await api.get<Envelope<AssetExportJob>>(`/v1/assets/exports/${encodeURIComponent(jobID)}`);
  return response.data.data;
}

export async function downloadAssetExport(job: AssetExportJob): Promise<void> {
  const response = await api.get<Blob>(`/v1/assets/exports/${encodeURIComponent(job.job_id)}/download`, {
    responseType: 'blob',
  });
  const objectURL = URL.createObjectURL(response.data);
  const anchor = document.createElement('a');
  const disposition = String(response.headers['content-disposition'] ?? '');
  const headerName = disposition.match(/filename="?([^";]+)"?/i)?.[1];
  anchor.href = objectURL;
  anchor.download = headerName || `asset-inventory-${job.job_id}.${job.format}`;
  anchor.click();
  URL.revokeObjectURL(objectURL);
}

export async function fetchAssetColumnPreference(): Promise<AssetColumnPreference> {
  const response = await api.get<Envelope<AssetColumnPreference>>('/v1/assets/preferences/columns', {
    params: { view_id: 'asset-inventory' },
  });
  return response.data.data;
}

export async function updateAssetColumnPreference(input: {
  columns: string[];
  expectedRevision: number;
  reason: string;
}): Promise<AssetColumnPreference> {
  const response = await api.put<Envelope<AssetColumnPreference>>('/v1/assets/preferences/columns', {
    view_id: 'asset-inventory',
    columns: input.columns,
    expected_revision: input.expectedRevision,
    reason: input.reason,
  });
  return response.data.data;
}

export function newAssetExportIdempotencyKey(): string {
  const randomPart = globalThis.crypto?.randomUUID?.()
    ?? `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  return `asset-export:${randomPart}`;
}

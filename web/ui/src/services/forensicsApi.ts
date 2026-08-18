import { api } from '@/services/httpClient';

export type ForensicsJobStatus = 'queued' | 'processing' | 'partial' | 'completed' | 'failed' | 'cancelled';

export type ForensicsObjectAuthority = {
  bucket: string;
  object_key: string;
  object_version: string;
  etag: string;
  size_bytes: number;
  sha256: string;
  receipt_observed_at: string;
  retention_until: string;
  legal_hold: boolean;
};

export type ForensicsRestorationReceipt = {
  restoration_id: string;
  revision: number;
  status: string;
  object_sha256: string;
  event_id: string;
  outbox_status: string;
  created_at: string;
  replayed: boolean;
};

export type ForensicsJobManifest = {
  manifest_version: number;
  tenant_id: string;
  task_id: string;
  restoration_contract_version: number;
  pcap_index_ids: string[];
  source_object_receipts: ForensicsObjectAuthority[];
  result_object: ForensicsObjectAuthority;
  restoration_receipts: ForensicsRestorationReceipt[];
  status: 'completed' | 'partial';
  created_at: string;
  completed_at: string;
  executable: false;
  automatic_open: false;
};

export type ForensicsJob = {
  job_id: string;
  status: ForensicsJobStatus;
  progress: number;
  total_packets: number;
  total_bytes: number;
  files_scanned: number;
  result_file_key?: string;
  sha256?: string;
  error_message?: string;
  params?: Record<string, unknown>;
  created_at: number;
  updated_at: number;
  completed_at?: number;
  revision: number;
  event_id?: string;
  action_id?: string;
  idempotency_key?: string;
  outbox_status?: string;
  replayed?: boolean;
  compatibility_mode?: boolean;
  manifest_sha256?: string;
  manifest?: ForensicsJobManifest;
  result_object_version?: string;
  retention_until?: number;
};

export type ForensicsJobListResult = {
  jobs: ForensicsJob[];
  total: number;
  limit: number;
  offset: number;
};

export type ForensicsJobFilters = {
  limit?: number;
  offset?: number;
  status?: ForensicsJobStatus;
  taskId?: string;
  assetId?: string;
  alertId?: string;
  campaignId?: string;
  evidenceId?: string;
  evidenceType?: string;
  srcIp?: string;
  dstIp?: string;
  protocol?: string;
  port?: string;
  tuple?: string;
};

export type ForensicsFiveTuple = {
  source_ip: string;
  destination_ip: string;
  source_port: number;
  destination_port: number;
  protocol: 6;
};

export type ForensicsRestorationTaskInput = {
  request_id: string;
  session_id: string;
  community_id: string;
  flow_ids: string[];
  flow_id: string;
  five_tuple: ForensicsFiveTuple;
  direction: 'client_to_server' | 'server_to_client';
  protocol_profile_id: 'http1-response-body-v1' | 'ftp-passive-retr-v1' | 'smtp-data-mime-v1';
  ftp_data?: { community_id: string; flow_id: string; five_tuple: ForensicsFiveTuple };
  ftp_tls_enabled?: boolean;
};

export type ForensicsJobInput = {
  assetId?: string;
  alertIds?: string[];
  caseIds?: string[];
  campaignId?: string;
  baselineId?: string;
  evidenceId?: string;
  evidenceType?: string;
  probeIds: string[];
  srcIp?: string;
  dstIp?: string;
  srcPort?: number;
  dstPort?: number;
  protocol?: number;
  communityId?: string;
  startTime: number;
  endTime: number;
  maxPackets?: number;
  purpose: string;
  retentionPolicy?: string;
  restorations?: ForensicsRestorationTaskInput[];
};

export type ForensicsCommandReceipt = {
  job_id: string;
  status: ForensicsJobStatus;
  revision: number;
  event_id?: string;
  action_id?: string;
  idempotency_key?: string;
  outbox_status?: string;
  created_at?: number | string;
  replayed?: boolean;
  compatibility_mode?: boolean;
};

export type ForensicsCommandOptions = {
  idempotencyKey: string;
  reason: string;
};

export type ForensicsVerifyResult = {
  key: string;
  tenant_id: string;
  sha256: string;
  expected_sha256?: string;
  registered_sha256?: string;
  verified: boolean;
  size_bytes: number;
};

export type ForensicsPresignResult = {
  key: string;
  url: string;
  expires_at: number;
  sha256?: string;
  object_version?: string;
  manifest_sha256?: string;
  purpose?: string;
};

type Envelope<T> = { data?: T } & Partial<T>;

const payload = <T>(value: Envelope<T>): T => (value.data ?? value) as T;

const commandHeaders = (options: ForensicsCommandOptions, revision?: number) => ({
  'Idempotency-Key': options.idempotencyKey,
  'X-Action-Reason': options.reason,
  ...(revision === undefined ? {} : { 'If-Match': `W/"${revision}"` }),
});

const canonicalValues = (values: Array<string | undefined>) =>
  Array.from(new Set(values.map((value) => value?.trim()).filter((value): value is string => Boolean(value)))).sort();

const normalizeCommandReceipt = (value: Envelope<ForensicsCommandReceipt> & { task_id?: string }) => {
  const receipt = payload(value);
  const jobId = receipt.job_id || value.task_id || (receipt as ForensicsCommandReceipt & { task_id?: string }).task_id;
  if (!jobId) throw new Error('取证命令响应缺少任务 ID');
  return { ...receipt, job_id: jobId };
};

export const makeForensicsCommandOptions = (
  action: 'create' | 'cancel' | 'retry',
  target: string,
): ForensicsCommandOptions => ({
  idempotencyKey: `forensics-${action}-${crypto.randomUUID()}`,
  reason: `${action} forensics evidence ${target}`,
});

export const listForensicsJobs = async (filters: ForensicsJobFilters = {}): Promise<ForensicsJobListResult> => {
  const limit = filters.limit ?? 20;
  const offset = filters.offset ?? 0;
  const response = await api.get<{ data?: ForensicsJob[]; pagination?: { total?: number; limit?: number; offset?: number } }>(
    '/v1/pcap/jobs',
    {
      params: {
        limit,
        offset,
        status: filters.status || undefined,
        task_id: filters.taskId || undefined,
        asset_id: filters.assetId || undefined,
        alert_id: filters.alertId || undefined,
        campaign_id: filters.campaignId || undefined,
        evidence_id: filters.evidenceId || undefined,
        evidence_type: filters.evidenceType || undefined,
        src_ip: filters.srcIp || undefined,
        dst_ip: filters.dstIp || undefined,
        protocol: filters.protocol || undefined,
        port: filters.port || undefined,
        tuple: filters.tuple || undefined,
      },
    },
  );
  const jobs = Array.isArray(response.data.data) ? response.data.data : [];
  return {
    jobs,
    total: response.data.pagination?.total ?? jobs.length,
    limit: response.data.pagination?.limit ?? limit,
    offset: response.data.pagination?.offset ?? offset,
  };
};

export const getForensicsJob = async (jobId: string): Promise<ForensicsJob> => {
  const response = await api.get<Envelope<ForensicsJob>>(`/v1/pcap/jobs/${encodeURIComponent(jobId)}`);
  return payload(response.data);
};

export const createForensicsJob = async (
  input: ForensicsJobInput,
  options: ForensicsCommandOptions,
): Promise<ForensicsCommandReceipt> => {
  const probeIds = canonicalValues(input.probeIds);
  if (!probeIds.length || !input.purpose.trim()) throw new Error('新建取证任务需要 probe 和用途');
  const response = await api.post<Envelope<ForensicsCommandReceipt> & { task_id?: string }>(
    '/v1/pcap/jobs',
    {
      asset_id: input.assetId || undefined,
      alert_ids: canonicalValues(input.alertIds ?? []),
      case_ids: canonicalValues(input.caseIds ?? []),
      campaign_id: input.campaignId || undefined,
      baseline_id: input.baselineId || undefined,
      evidence_id: input.evidenceId || undefined,
      evidence_type: input.evidenceType || undefined,
      probe_ids: probeIds,
      src_ip: input.srcIp || undefined,
      dst_ip: input.dstIp || undefined,
      src_port: input.srcPort || undefined,
      dst_port: input.dstPort || undefined,
      protocol: input.protocol || undefined,
      community_id: input.communityId || undefined,
      start_time: input.startTime,
      end_time: input.endTime,
      max_packets: input.maxPackets ?? 100_000,
      purpose: input.purpose.trim(),
      retention_policy: input.retentionPolicy ?? 'forensics-standard',
      restoration_contract_version: 1,
      restorations: input.restorations ?? [],
    },
    { headers: commandHeaders(options) },
  );
  return normalizeCommandReceipt(response.data);
};

export const cancelForensicsJob = async (
  jobId: string,
  revision: number,
  options: ForensicsCommandOptions,
): Promise<ForensicsCommandReceipt> => {
  const response = await api.post<Envelope<ForensicsCommandReceipt> & { task_id?: string }>(
    `/v1/pcap/jobs/${encodeURIComponent(jobId)}/cancel`,
    undefined,
    { headers: commandHeaders(options, revision) },
  );
  return normalizeCommandReceipt(response.data);
};

export const retryForensicsJob = async (
  jobId: string,
  revision: number,
  options: ForensicsCommandOptions,
): Promise<ForensicsCommandReceipt> => {
  const response = await api.post<Envelope<ForensicsCommandReceipt> & { task_id?: string }>(
    `/v1/pcap/jobs/${encodeURIComponent(jobId)}/retry`,
    undefined,
    { headers: commandHeaders(options, revision) },
  );
  return normalizeCommandReceipt(response.data);
};

export const verifyForensicsPcap = async (key: string, expectedSha256?: string): Promise<ForensicsVerifyResult> => {
  const response = await api.post<Envelope<ForensicsVerifyResult>>('/v1/pcap/verify', {
    key,
    expected_sha256: expectedSha256 || undefined,
  });
  return payload(response.data);
};

export const presignForensicsPcap = async (
  key: string,
  purpose: string,
  expirySeconds = 900,
): Promise<ForensicsPresignResult> => {
  if (!purpose.trim()) throw new Error('下载用途不能为空');
  const response = await api.post<Envelope<ForensicsPresignResult>>('/v1/pcap/presign', {
    key,
    purpose: purpose.trim(),
    expiry_seconds: expirySeconds,
  });
  return payload(response.data);
};

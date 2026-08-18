import { api } from '@/services/api';

export type AttackChainEvent = {
  event_id: string;
  timestamp: number;
  description: string;
  entity?: string;
  src_ip: string;
  dst_ip: string;
  technique?: string;
  severity: string;
  evidence_ids?: string[];
  edge_id?: string;
  relation_type?: string;
  provenance?: AttackChainProvenance;
  uncertainty?: string;
  confidence?: number;
  source?: AttackChainIdentity;
  target?: AttackChainIdentity;
  evidence?: AttackChainEvidenceAnchor[];
};

export type AttackChainProvenance = 'observed' | 'derived' | 'analyst';

export type AttackChainIdentity = {
  tenant_id: string;
  entity_type: string;
  canonical_id: string;
  vertex_id: string;
};

export type AttackChainEvidenceAnchor = {
  tenant_id: string;
  evidence_id: string;
  kind: 'event' | 'rule' | 'model' | 'analyst_conclusion';
  immutable_uri: string;
  sha256: string;
  source_event_id: string;
  occurred_at: number;
  available: boolean;
};

export type AttackChainSnapshotEdge = {
  edge_id: string;
  relation_type: string;
  stage: string;
  source: AttackChainIdentity;
  target: AttackChainIdentity;
  event_time: number;
  provenance: AttackChainProvenance;
  confidence: number;
  uncertainty: string;
  evidence: AttackChainEvidenceAnchor[];
};

export type AttackChainSnapshotPath = {
  path_id: string;
  kind: 'candidate' | 'alternative';
  edges: AttackChainSnapshotEdge[];
  confidence: number;
  uncertainty: string;
  contradicts_path_ids: string[];
  partial: boolean;
  partial_reasons: string[];
  path_sha256: string;
};

export type AttackChainGraphSnapshot = {
  snapshot_id: string;
  schema_version: string;
  nodes: AttackChainIdentity[];
  edge_ids: string[];
  label_refs: Record<string, string>;
  evidence_refs: string[];
  source_watermarks: Record<string, string>;
  node_count: number;
  edge_count: number;
  node_sha256: string;
  edge_sha256: string;
  snapshot_sha256: string;
};

export type AttackChainContractMeta = {
  contract_version: number;
  schema_version: number;
  snapshot_id: string;
  as_of: string;
  trace_id: string;
  result_code: string;
  partial: boolean;
  missing_sections: string[];
  source_watermarks: Record<string, string>;
};

type AttackChainSnapshot = {
  snapshot_id: string;
  tenant_id: string;
  chain_id: string;
  version: number;
  as_of: string;
  source: AttackChainIdentity;
  target: AttackChainIdentity;
  stages: string[];
  candidate_path: AttackChainSnapshotPath;
  alternative_paths: AttackChainSnapshotPath[];
  graph_snapshot: AttackChainGraphSnapshot;
  partial: boolean;
  partial_reasons: string[];
  truncated: boolean;
  truncation_reason?: string;
  continuation_boundary?: string;
  snapshot_sha256: string;
};

export type AttackChainPhase = {
  phase: string;
  alert_ids: string[];
  start_time: number;
  end_time: number;
  key_events: AttackChainEvent[];
  confidence: number;
};

export type AttackChainDetail = {
  chain_id: string;
  tenant_id: string;
  title: string;
  description: string;
  phases: AttackChainPhase[];
  risk_score: number;
  root_alert_id: string;
  source_ip: string;
  entity_count: number;
  alert_count: number;
  start_time: number;
  end_time: number;
  status: string;
  mitre_techniques: string[];
  snapshot_id?: string;
  snapshot_version?: number;
  as_of?: string;
  source?: AttackChainIdentity;
  target?: AttackChainIdentity;
  candidate_path?: AttackChainSnapshotPath;
  alternative_paths?: AttackChainSnapshotPath[];
  graph_snapshot?: AttackChainGraphSnapshot;
  partial?: boolean;
  partial_reasons?: string[];
  truncated?: boolean;
  truncation_reason?: string;
  continuation_boundary?: string;
  snapshot_sha256?: string;
  contract_meta?: AttackChainContractMeta;
};

export type AttackChainEvidence = {
  evidence_id: string;
  alert_id: string;
  phase: string;
  type: string;
  summary: string;
  timestamp: number;
  integrity: number;
  visualization_url?: string;
  kind?: AttackChainEvidenceAnchor['kind'];
  immutable_uri?: string;
  sha256?: string;
  source_event_id?: string;
  available?: boolean;
  path_ids?: string[];
  stages?: string[];
};

export type AttackChainPath = {
  path_id: string;
  phase: string;
  technique: string;
  entity: string;
  alert: string;
  evidence_id: string;
  action: string;
  status: string;
  source_ip: string;
  destination_ip: string;
  timestamp: number;
  kind?: AttackChainSnapshotPath['kind'];
  confidence?: number;
  uncertainty?: string;
  provenance?: AttackChainProvenance[];
  partial?: boolean;
  partial_reasons?: string[];
  contradicts_path_ids?: string[];
  path_sha256?: string;
  edges?: AttackChainSnapshotEdge[];
};

export type AttackChainRecommendation = {
  recommendation_id: string;
  category: AttackChainRecommendationCategory;
  priority: string;
  target: string;
  action: string;
  impact: string;
  phase: string;
};

export type AttackChainEvidenceType = '' | 'alert' | 'pcap' | 'session' | 'log' | 'graph' | 'rule_model';
export type AttackChainRecommendationCategory = 'block' | 'isolate' | 'allowlist' | 'playbook';

type AttackChainPageEnvelope = {
  data?: {
    items?: unknown[];
    total?: number;
    limit?: number;
    offset?: number;
  };
  items?: unknown[];
  total?: number;
  limit?: number;
  offset?: number;
  meta?: AttackChainContractMeta;
};

export type AttackChainPage<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
  meta?: AttackChainContractMeta;
};

type AttackChainListEnvelope = {
  data?: {
    chains?: Array<AttackChainDetail | AttackChainSnapshot>;
    total?: number;
  };
  chains?: Array<AttackChainDetail | AttackChainSnapshot>;
  total?: number;
  meta?: AttackChainContractMeta;
};

type AttackChainDetailEnvelope = {
  data?: AttackChainDetail | AttackChainSnapshot;
  meta?: AttackChainContractMeta;
} & Partial<AttackChainDetail>;

export async function fetchAttackChains(limit = 8): Promise<{ chains: AttackChainDetail[]; total: number; meta?: AttackChainContractMeta }> {
  const response = await api.get<AttackChainListEnvelope>('/v1/attack-chains', { params: { limit, offset: 0 } });
  const envelope = response.data.data ?? response.data;
  const chains = Array.isArray(envelope.chains)
    ? envelope.chains.map((item) => normalizeAttackChainDetail(item, response.data.meta))
    : [];
  return { chains, total: Number(envelope.total ?? chains.length), meta: response.data.meta };
}

export async function fetchAttackChainDetail(chainId: string): Promise<AttackChainDetail> {
  if (!chainId.trim()) throw new Error('attack chain id required');
  const response = await api.get<AttackChainDetailEnvelope>(`/v1/attack-chains/${encodeURIComponent(chainId)}`);
  const payload = response.data.data ?? response.data;
  if (!payload.chain_id) throw new Error('攻击链详情接口未返回 chain_id');
  return normalizeAttackChainDetail(payload as AttackChainDetail | AttackChainSnapshot, response.data.meta);
}

export async function fetchAttackChainEvidence(
  chainId: string,
  request: { limit: number; offset: number; type?: AttackChainEvidenceType; phase?: string },
): Promise<AttackChainPage<AttackChainEvidence>> {
  return fetchAttackChainPage<AttackChainEvidence>(chainId, 'evidence', request, normalizeAttackChainEvidenceItems);
}

export async function fetchAttackChainPaths(
  chainId: string,
  request: { limit: number; offset: number; phase?: string },
): Promise<AttackChainPage<AttackChainPath>> {
  return fetchAttackChainPage<AttackChainPath>(chainId, 'paths', request, normalizeAttackChainPathItems);
}

export async function fetchAttackChainRecommendations(
  chainId: string,
  request: { limit: number; offset: number; category: AttackChainRecommendationCategory; phase?: string },
): Promise<AttackChainPage<AttackChainRecommendation>> {
  return fetchAttackChainPage<AttackChainRecommendation>(
    chainId,
    'recommendations',
    request,
    (items) => items as AttackChainRecommendation[],
  );
}

async function fetchAttackChainPage<T>(
  chainId: string,
  resource: 'evidence' | 'paths' | 'recommendations',
  request: { limit: number; offset: number; type?: AttackChainEvidenceType; category?: AttackChainRecommendationCategory; phase?: string },
  normalizeItems: (items: unknown[]) => T[],
): Promise<AttackChainPage<T>> {
  if (!chainId.trim()) throw new Error('attack chain id required');
  const response = await api.get<AttackChainPageEnvelope>(
    `/v1/attack-chains/${encodeURIComponent(chainId)}/${resource}`,
    { params: request },
  );
  const payload = response.data.data ?? response.data;
  const items = normalizeItems(Array.isArray(payload.items) ? payload.items : []);
  return {
    items,
    total: Number(payload.total ?? items.length),
    limit: Number(payload.limit ?? request.limit),
    offset: Number(payload.offset ?? request.offset),
    meta: response.data.meta,
  };
}

const isAttackChainSnapshot = (
  value: AttackChainDetail | AttackChainSnapshot,
): value is AttackChainSnapshot => 'candidate_path' in value && 'graph_snapshot' in value;

export const normalizeAttackChainDetail = (
  value: AttackChainDetail | AttackChainSnapshot,
  meta?: AttackChainContractMeta,
): AttackChainDetail => {
  if (!isAttackChainSnapshot(value)) return { ...value, contract_meta: meta };
  const edges = value.candidate_path.edges ?? [];
  const phaseOrder = value.stages.length
    ? value.stages
    : Array.from(new Set(edges.map((edge) => edge.stage)));
  const phases = phaseOrder.map((phase) => {
    const phaseEdges = edges.filter((edge) => edge.stage === phase);
    const timestamps = phaseEdges.map((edge) => edge.event_time).filter(Number.isFinite);
    const confidence = phaseEdges.length
      ? phaseEdges.reduce((sum, edge) => sum + Number(edge.confidence || 0), 0) / phaseEdges.length
      : 0;
    return {
      phase,
      alert_ids: Array.from(new Set(phaseEdges.flatMap((edge) => edge.evidence.map((item) => item.source_event_id)))).filter(Boolean),
      start_time: timestamps.length ? Math.min(...timestamps) : 0,
      end_time: timestamps.length ? Math.max(...timestamps) : 0,
      confidence,
      key_events: phaseEdges.map((edge) => ({
        event_id: edge.edge_id,
        timestamp: edge.event_time,
        description: edge.relation_type,
        entity: `${edge.source.canonical_id} → ${edge.target.canonical_id}`,
        src_ip: edge.source.canonical_id,
        dst_ip: edge.target.canonical_id,
        technique: edge.relation_type,
        severity: edge.confidence >= 0.85 ? 'high' : edge.confidence >= 0.6 ? 'medium' : 'low',
        evidence_ids: edge.evidence.map((item) => item.evidence_id),
        edge_id: edge.edge_id,
        relation_type: edge.relation_type,
        provenance: edge.provenance,
        uncertainty: edge.uncertainty,
        confidence: edge.confidence,
        source: edge.source,
        target: edge.target,
        evidence: edge.evidence,
      })),
    } satisfies AttackChainPhase;
  });
  const timestamps = edges.map((edge) => edge.event_time).filter(Number.isFinite);
  const evidenceEventIDs = Array.from(new Set(edges.flatMap((edge) => edge.evidence.map((item) => item.source_event_id)))).filter(Boolean);
  return {
    chain_id: value.chain_id,
    tenant_id: value.tenant_id,
    title: `${value.source.canonical_id} → ${value.target.canonical_id}`,
    description: 'M07 不可变攻击链快照；边仅来自 observed、derived 或 analyst 证据。',
    phases,
    risk_score: Math.round(Number(value.candidate_path.confidence || 0) * 100),
    root_alert_id: evidenceEventIDs[0] ?? '',
    source_ip: value.source.canonical_id,
    entity_count: value.graph_snapshot.node_count,
    alert_count: evidenceEventIDs.length,
    start_time: timestamps.length ? Math.min(...timestamps) : Date.parse(value.as_of),
    end_time: timestamps.length ? Math.max(...timestamps) : Date.parse(value.as_of),
    status: value.truncated ? 'truncated' : value.partial ? 'partial' : 'active',
    mitre_techniques: phaseOrder,
    snapshot_id: value.snapshot_id,
    snapshot_version: value.version,
    as_of: value.as_of,
    source: value.source,
    target: value.target,
    candidate_path: value.candidate_path,
    alternative_paths: value.alternative_paths,
    graph_snapshot: value.graph_snapshot,
    partial: value.partial,
    partial_reasons: value.partial_reasons,
    truncated: value.truncated,
    truncation_reason: value.truncation_reason,
    continuation_boundary: value.continuation_boundary,
    snapshot_sha256: value.snapshot_sha256,
    contract_meta: meta,
  };
};

const normalizeAttackChainEvidenceItems = (items: unknown[]): AttackChainEvidence[] => items.map((item) => {
  const value = item as AttackChainEvidence & AttackChainEvidenceAnchor & { path_ids?: string[]; stages?: string[] };
  if (!value.immutable_uri) return value;
  return {
    evidence_id: value.evidence_id,
    alert_id: value.source_event_id,
    phase: value.stages?.[0] ?? '',
    type: value.kind,
    summary: value.immutable_uri,
    timestamp: value.occurred_at,
    integrity: value.available && /^[0-9a-f]{64}$/.test(value.sha256) ? 100 : 0,
    kind: value.kind,
    immutable_uri: value.immutable_uri,
    sha256: value.sha256,
    source_event_id: value.source_event_id,
    available: value.available,
    path_ids: value.path_ids ?? [],
    stages: value.stages ?? [],
  };
});

const normalizeAttackChainPathItems = (items: unknown[]): AttackChainPath[] => items.map((item) => {
  const value = item as AttackChainPath | AttackChainSnapshotPath;
  if (!('edges' in value) || !Array.isArray(value.edges)) return value as AttackChainPath;
  const first = value.edges[0];
  const last = value.edges[value.edges.length - 1];
  const provenance = Array.from(new Set(value.edges.map((edge) => edge.provenance)));
  return {
    path_id: value.path_id,
    phase: Array.from(new Set(value.edges.map((edge) => edge.stage))).join(' → '),
    technique: Array.from(new Set(value.edges.map((edge) => edge.relation_type))).join(' → '),
    entity: first && last ? `${first.source.canonical_id} → ${last.target.canonical_id}` : '未提供',
    alert: `${value.kind} / ${provenance.join('+') || '未提供'}`,
    evidence_id: first?.evidence[0]?.evidence_id ?? '',
    action: '下钻证据，不补线',
    status: value.partial ? 'partial' : 'confirmed',
    source_ip: first?.source.canonical_id ?? '',
    destination_ip: last?.target.canonical_id ?? '',
    timestamp: first?.event_time ?? 0,
    kind: value.kind,
    confidence: value.confidence,
    uncertainty: value.uncertainty,
    provenance,
    partial: value.partial,
    partial_reasons: value.partial_reasons,
    contradicts_path_ids: value.contradicts_path_ids,
    path_sha256: value.path_sha256,
    edges: value.edges,
  };
});

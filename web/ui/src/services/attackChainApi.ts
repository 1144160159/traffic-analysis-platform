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

type AttackChainPageEnvelope<T> = {
  data?: {
    items?: T[];
    total?: number;
    limit?: number;
    offset?: number;
  };
  items?: T[];
  total?: number;
  limit?: number;
  offset?: number;
};

export type AttackChainPage<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

type AttackChainListEnvelope = {
  data?: {
    chains?: AttackChainDetail[];
    total?: number;
  };
  chains?: AttackChainDetail[];
  total?: number;
};

type AttackChainDetailEnvelope = {
  data?: AttackChainDetail;
} & Partial<AttackChainDetail>;

export async function fetchAttackChains(limit = 8): Promise<{ chains: AttackChainDetail[]; total: number }> {
  const response = await api.get<AttackChainListEnvelope>('/v1/attack-chains', { params: { limit, offset: 0 } });
  const envelope = response.data.data ?? response.data;
  const chains = Array.isArray(envelope.chains) ? envelope.chains : [];
  return { chains, total: Number(envelope.total ?? chains.length) };
}

export async function fetchAttackChainDetail(chainId: string): Promise<AttackChainDetail> {
  if (!chainId.trim()) throw new Error('attack chain id required');
  const response = await api.get<AttackChainDetailEnvelope>(`/v1/attack-chains/${encodeURIComponent(chainId)}`);
  const payload = response.data.data ?? response.data;
  if (!payload.chain_id) throw new Error('攻击链详情接口未返回 chain_id');
  return payload as AttackChainDetail;
}

export async function fetchAttackChainEvidence(
  chainId: string,
  request: { limit: number; offset: number; type?: AttackChainEvidenceType; phase?: string },
): Promise<AttackChainPage<AttackChainEvidence>> {
  return fetchAttackChainPage<AttackChainEvidence>(chainId, 'evidence', request);
}

export async function fetchAttackChainPaths(
  chainId: string,
  request: { limit: number; offset: number; phase?: string },
): Promise<AttackChainPage<AttackChainPath>> {
  return fetchAttackChainPage<AttackChainPath>(chainId, 'paths', request);
}

export async function fetchAttackChainRecommendations(
  chainId: string,
  request: { limit: number; offset: number; category: AttackChainRecommendationCategory; phase?: string },
): Promise<AttackChainPage<AttackChainRecommendation>> {
  return fetchAttackChainPage<AttackChainRecommendation>(chainId, 'recommendations', request);
}

async function fetchAttackChainPage<T>(
  chainId: string,
  resource: 'evidence' | 'paths' | 'recommendations',
  request: { limit: number; offset: number; type?: AttackChainEvidenceType; category?: AttackChainRecommendationCategory; phase?: string },
): Promise<AttackChainPage<T>> {
  if (!chainId.trim()) throw new Error('attack chain id required');
  const response = await api.get<AttackChainPageEnvelope<T>>(
    `/v1/attack-chains/${encodeURIComponent(chainId)}/${resource}`,
    { params: request },
  );
  const payload = response.data.data ?? response.data;
  const items = Array.isArray(payload.items) ? payload.items : [];
  return {
    items,
    total: Number(payload.total ?? items.length),
    limit: Number(payload.limit ?? request.limit),
    offset: Number(payload.offset ?? request.offset),
  };
}

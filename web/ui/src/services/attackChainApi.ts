import { api } from '@/services/api';

export type AttackChainEvent = {
  event_id: string;
  timestamp: number;
  description: string;
  src_ip: string;
  dst_ip: string;
  technique?: string;
  severity: string;
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

export async function fetchAttackChains(limit = 50): Promise<{ chains: AttackChainDetail[]; total: number }> {
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

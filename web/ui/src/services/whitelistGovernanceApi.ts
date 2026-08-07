import { api } from '@/services/api';

export type WhitelistType = 'ip' | 'domain' | 'asset' | 'account' | 'rule' | 'model' | 'subnet' | 'fingerprint';
export type WhitelistStatus = 'draft' | 'pending' | 'active' | 'disabled';
export type WhitelistApprovalStatus = 'draft' | 'pending' | 'approved' | 'rejected';
export type WhitelistRisk = 'low' | 'medium' | 'high' | 'critical';

export type WhitelistEntry = {
  id: string;
  tenant_id: string;
  type: WhitelistType;
  value: string;
  reason: string;
  description: string;
  status: WhitelistStatus;
  approval_status: WhitelistApprovalStatus;
  source_alert_id?: string;
  feedback_id?: string;
  owner_role?: string;
  scope?: string;
  risk_level?: WhitelistRisk;
  covered_alerts?: number;
  covered_assets?: number;
  version: number;
  created_by: string;
  approved_by?: string;
  approved_at?: string;
  disabled_at?: string;
  expires_at?: string;
  created_at: string;
  updated_at: string;
  archived_at?: string;
  last_action_id?: string;
  last_trace_id?: string;
  rule_effect_status?: 'pending' | 'applied' | 'failed';
  rule_desired_state?: 'effective' | 'revoked';
  rule_revision?: string;
};

type Envelope<T> = { success: boolean; data: T };

export type CreateWhitelistDraft = {
  type: WhitelistType;
  value: string;
  reason: string;
  description: string;
  source_alert_id?: string;
  feedback_id?: string;
  owner_role: string;
  scope: string;
  risk_level: WhitelistRisk;
  covered_alerts: number;
  covered_assets: number;
  expires_at: string;
};

export async function fetchWhitelistEntries(): Promise<{ entries: WhitelistEntry[]; total: number }> {
  const response = await api.get<Envelope<{ entries: WhitelistEntry[]; total: number }>>('/v1/whitelist', { params: { limit: 200 } });
  return { entries: response.data.data.entries ?? [], total: response.data.data.total ?? 0 };
}

export async function createWhitelistDraft(input: CreateWhitelistDraft): Promise<WhitelistEntry> {
  const response = await api.post<Envelope<WhitelistEntry>>('/v1/whitelist', {
    ...input,
    status: 'draft',
    approval_status: 'draft',
  }, whitelistCommandConfig('whitelist-create'));
  return response.data.data;
}

export type WhitelistTransition = 'submit' | 'approve' | 'reject' | 'extend' | 'disable' | 'assign';

export async function transitionWhitelistEntry({
  entry,
  action,
  reason,
  expiresAt,
  ownerRole,
}: {
  entry: WhitelistEntry;
  action: WhitelistTransition;
  reason?: string;
  expiresAt?: string;
  ownerRole?: string;
}): Promise<WhitelistEntry> {
  const common = { expected_revision: entry.version };
  const body = action === 'submit'
    ? { ...common, status: 'pending', approval_status: 'pending', reason: reason || '提交独立审批' }
    : action === 'approve'
      ? { ...common, status: 'active', approval_status: 'approved', reason: reason || entry.reason }
      : action === 'reject'
        ? { ...common, status: 'disabled', approval_status: 'rejected', reason: reason || '审批驳回' }
        : action === 'extend'
          ? { ...common, expires_at: expiresAt, reason: reason || entry.reason }
          : action === 'assign'
            ? { ...common, owner_role: ownerRole }
            : { ...common, status: 'disabled', reason: reason || entry.reason };
  const response = await api.patch<Envelope<WhitelistEntry>>(
    `/v1/whitelist/${encodeURIComponent(entry.id)}`,
    body,
    whitelistCommandConfig(whitelistActionID(action)),
  );
  return response.data.data;
}

export const whitelistTypeLabels: Record<WhitelistType, string> = {
  ip: 'IP',
  subnet: 'IP',
  domain: '域名',
  asset: '资产',
  account: '账号',
  rule: '规则',
  model: '模型',
  fingerprint: '指纹',
};

const whitelistActionID = (action: WhitelistTransition) => ({
  submit: 'whitelist-submit-approval',
  approve: 'whitelist-approve',
  reject: 'whitelist-reject',
  extend: 'whitelist-extend',
  disable: 'whitelist-disable',
  assign: 'whitelist-assign',
} as const)[action];

const whitelistCommandConfig = (actionID: string) => {
  const nonce = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return {
    headers: {
      'X-Action-ID': actionID,
      'Idempotency-Key': `whitelist:${actionID}:${nonce}`,
    },
  };
};

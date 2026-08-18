import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '@/services/httpClient';
import {
  buildEncryptedTrafficRangeParams,
  fetchEncryptedTrafficSnapshot,
  submitEncryptedTrafficEgressAction,
  submitEncryptedTrafficEvidenceAction,
} from '@/services/encryptedTrafficApi';

describe('encrypted traffic typed client', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('builds deterministic request bounds for every supported range', () => {
    const endTime = 1_700_000_000_000;
    expect(buildEncryptedTrafficRangeParams('近 1 小时', endTime)).toEqual({
      start_time: endTime - 60 * 60 * 1_000,
      end_time: endTime,
    });
    expect(buildEncryptedTrafficRangeParams('近 24 小时', endTime)).toEqual({
      start_time: endTime - 24 * 60 * 60 * 1_000,
      end_time: endTime,
    });
    expect(buildEncryptedTrafficRangeParams('近 7 天', endTime)).toEqual({
      start_time: endTime - 7 * 24 * 60 * 60 * 1_000,
      end_time: endTime,
    });
  });

  it('loads the versioned snapshot from the real typed endpoint without a tenant override', async () => {
    const snapshot = {
      snapshot_id: 'encrypted-1',
      tenant_id: 'tenant-from-auth',
      as_of: '2026-08-15T12:00:00Z',
      window_start: '2026-08-15T11:00:00Z',
      window_end: '2026-08-15T12:00:00Z',
      flow_metadata: { availability: 'no_sample', sample_count: 0, source: 'sessions', source_watermark: 'w1', rule_versions: [], model_versions: [], partial: false, missing_reasons: [], facts: [] },
      plaintext_visible: { availability: 'no_sample', sample_count: 0, source: 'feature_fp', source_watermark: 'w2', rule_versions: [], model_versions: [], partial: false, missing_reasons: [], facts: [] },
      side_channel: { availability: 'no_sample', sample_count: 0, source: 'sessions', source_watermark: 'w1', rule_versions: [], model_versions: [], partial: false, missing_reasons: [], facts: [] },
      raw_reference: { availability: 'forbidden', sample_count: 0, source: 'permission.pcap:read', source_watermark: 'forbidden', rule_versions: [], model_versions: [], partial: true, missing_reasons: ['pcap:read_required'], facts: [] },
      randomness_statistics: { availability: 'not_computable', sample_count: 1, source: 'feature_fp', source_watermark: 'w2', rule_versions: [], model_versions: [], partial: true, missing_reasons: ['sample_bytes_not_persisted'], facts: [] },
    } as const;
    const meta = { contract_version: 1, schema_version: 1, snapshot_id: 'encrypted-1', as_of: snapshot.as_of, trace_id: 'trace-1', result_code: 'SUCCESS', partial: true, missing_sections: ['raw_reference.permission'], source_watermarks: { sessions: 'w1' } };
    const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { data: snapshot, meta } });

    await expect(fetchEncryptedTrafficSnapshot('近 1 小时', 1_700_000_000_000)).resolves.toEqual({ snapshot, meta });
    expect(get).toHaveBeenCalledWith('/v1/encrypted-traffic/snapshot', {
      params: { start_time: 1_700_000_000_000 - 60 * 60 * 1_000, end_time: 1_700_000_000_000 },
    });
    expect(get.mock.calls[0]?.[1]?.params).not.toHaveProperty('tenant_id');
  });

  it('preserves the egress action endpoint, default action and body shape', async () => {
    const result = {
      action_id: 'action-1',
      action: 'create_alert',
      audit_event: 'ENCRYPTED_EGRESS_ALERT_REQUESTED',
      status: 'recorded' as const,
      target: '203.0.113.8',
    };
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { data: result } });

    await expect(submitEncryptedTrafficEgressAction({
      actionId: 'egress-create-alert',
      target: '203.0.113.8',
      dataMode: 'live',
    })).resolves.toEqual(result);
    expect(post).toHaveBeenCalledWith('/v1/encrypted-traffic/egress-actions', {
      action: 'create_alert',
      target: '203.0.113.8',
      data_mode: 'live',
    });
  });

  it('preserves the evidence action endpoint, default action and flat response fallback', async () => {
    const result = {
      action_id: 'action-2',
      action: 'verify_hash',
      audit_event: 'ENCRYPTED_EVIDENCE_HASH_VERIFICATION_REQUESTED',
      status: 'recorded' as const,
      target: 'session-1',
    };
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: result });

    await expect(submitEncryptedTrafficEvidenceAction({
      actionId: 'evidence-verify-hash',
      target: 'session-1',
      dataMode: 'partial',
    })).resolves.toEqual(result);
    expect(post).toHaveBeenCalledWith('/v1/encrypted-traffic/evidence-actions', {
      action: 'verify_hash',
      target: 'session-1',
      data_mode: 'partial',
    });
  });
});

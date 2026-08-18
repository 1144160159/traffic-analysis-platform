import { describe, expect, it } from 'vitest';
import { normalizeAttackChainDetail } from './attackChainApi';


describe('M07 attack-chain snapshot adapter', () => {
  it('preserves explicit endpoints, provenance, alternatives and partial boundaries', () => {
    const identity = (canonicalId: string) => ({
      tenant_id: 'tenant-a',
      entity_type: 'asset',
      canonical_id: canonicalId,
      vertex_id: `vertex-${canonicalId}`,
    });
    const evidence = (evidenceId: string, kind: 'event' | 'rule' | 'analyst_conclusion') => ({
      tenant_id: 'tenant-a',
      evidence_id: evidenceId,
      kind,
      immutable_uri: `minio://evidence/${evidenceId}`,
      sha256: 'a'.repeat(64),
      source_event_id: `event-${evidenceId}`,
      occurred_at: 1_800_000_000_000,
      available: true,
    });
    const source = identity('source');
    const middle = identity('middle');
    const target = identity('target');
    const observed = {
      edge_id: 'edge-observed', relation_type: 'connected_to', stage: 'initial_access',
      source, target: middle, event_time: 1_800_000_000_001,
      provenance: 'observed' as const, confidence: 0.91, uncertainty: '',
      evidence: [evidence('packet-1', 'event')],
    };
    const derived = {
      edge_id: 'edge-derived', relation_type: 'executed_on', stage: 'execution',
      source: middle, target, event_time: 1_800_000_000_002,
      provenance: 'derived' as const, confidence: 0.72, uncertainty: 'rule-only',
      evidence: [evidence('rule-1', 'rule')],
    };
    const analyst = {
      edge_id: 'edge-analyst', relation_type: 'suspected_path', stage: 'initial_access',
      source, target, event_time: 1_800_000_000_002,
      provenance: 'analyst' as const, confidence: 0.55, uncertainty: 'pending peer review',
      evidence: [evidence('conclusion-1', 'analyst_conclusion')],
    };
    const snapshot = {
      snapshot_id: 'snapshot-1', tenant_id: 'tenant-a', chain_id: 'chain-1', version: 4,
      as_of: '2027-01-15T08:00:00Z', source, target,
      stages: ['initial_access', 'execution'],
      candidate_path: {
        path_id: 'candidate-1', kind: 'candidate' as const, edges: [observed, derived],
        confidence: 0.8, uncertainty: 'bounded source window', contradicts_path_ids: ['alternative-1'],
        partial: false, partial_reasons: [], path_sha256: 'b'.repeat(64),
      },
      alternative_paths: [{
        path_id: 'alternative-1', kind: 'alternative' as const, edges: [analyst],
        confidence: 0.55, uncertainty: 'analyst hypothesis', contradicts_path_ids: ['candidate-1'],
        partial: true, partial_reasons: ['evidence_unavailable:x'], path_sha256: 'c'.repeat(64),
      }],
      graph_snapshot: {
        snapshot_id: 'graph-1', schema_version: 'gnn-graph/v1', nodes: [source, middle, target],
        edge_ids: ['edge-observed', 'edge-derived', 'edge-analyst'], label_refs: {}, evidence_refs: [],
        source_watermarks: { clickhouse: 'window:42', nebulagraph: 'revision:9' },
        node_count: 3, edge_count: 3, node_sha256: 'd'.repeat(64), edge_sha256: 'e'.repeat(64),
        snapshot_sha256: 'f'.repeat(64),
      },
      partial: true, partial_reasons: ['path:alternative-1'], truncated: true,
      truncation_reason: 'path_budget', continuation_boundary: 'alternative-1',
      snapshot_sha256: '1'.repeat(64),
    };

    const detail = normalizeAttackChainDetail(snapshot, {
      contract_version: 1, schema_version: 1, snapshot_id: 'snapshot-1',
      as_of: snapshot.as_of, trace_id: 'trace-1', result_code: 'PARTIAL', partial: true,
      missing_sections: ['chain-1:path_budget'], source_watermarks: { 'chain-1:clickhouse': 'window:42' },
    });

    expect(detail.source?.canonical_id).toBe('source');
    expect(detail.target?.canonical_id).toBe('target');
    expect(detail.phases.map((phase) => phase.phase)).toEqual(['initial_access', 'execution']);
    expect(detail.phases[0].key_events[0].provenance).toBe('observed');
    expect(detail.phases[1].key_events[0].uncertainty).toBe('rule-only');
    expect(detail.alternative_paths?.[0].edges[0].provenance).toBe('analyst');
    expect(detail.truncated).toBe(true);
    expect(detail.continuation_boundary).toBe('alternative-1');
    expect(detail.contract_meta?.trace_id).toBe('trace-1');
  });
});

import { describe, expect, it } from 'vitest';
import {
  extractNamedPageSnapshotList,
  extractPageSnapshotList,
  readPageSnapshotEnvelope,
} from '@/services/pageSnapshotEnvelope';

describe('page snapshot envelope tools', () => {
  it('preserves error, partial and source watermark facts beside recursively unwrapped data', () => {
    expect(readPageSnapshotEnvelope({
      data: { data: { sessions: [{ session_id: 'session-1' }] } },
      error: { code: 'PARTIAL_SOURCE' },
      meta: {
        contract_version: 1,
        snapshot_id: 'snapshot-1',
        as_of: '2026-08-15T12:00:00Z',
        trace_id: 'trace-1',
        partial: true,
        missing_sections: ['raw_reference'],
        source_watermarks: { clickhouse: '2026-08-15T11:59:58Z', invalid: 42 },
      },
    })).toEqual({
      data: { sessions: [{ session_id: 'session-1' }] },
      error: { code: 'PARTIAL_SOURCE' },
      contractVersion: 1,
      snapshotId: 'snapshot-1',
      asOf: '2026-08-15T12:00:00Z',
      traceId: 'trace-1',
      partial: true,
      missingSections: ['raw_reference'],
      sourceWatermarks: { clickhouse: '2026-08-15T11:59:58Z' },
    });
  });

  it('keeps broad and named-list extraction behavior distinct', () => {
    const payload = { data: { sessions: [{ id: 1 }], unrelated: [{ id: 2 }] } };
    expect(extractPageSnapshotList(payload, ['sessions'])).toEqual([{ id: 1 }]);
    expect(extractNamedPageSnapshotList(payload, ['sessions'])).toEqual([{ id: 1 }]);
    expect(extractNamedPageSnapshotList(payload, ['missing'])).toEqual([]);
  });
});


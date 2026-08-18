import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import {
  encryptedSnapshotAvailabilityCopy,
  encryptedSnapshotDrilldownPath,
  encryptedSnapshotSectionPresentation,
} from '@/services/encryptedTrafficSnapshotPresentation';

describe('EncryptedTrafficPage versioned snapshot presentation', () => {
  it('keeps zero, no sample, not computable, unavailable and forbidden distinct', () => {
    expect(encryptedSnapshotAvailabilityCopy('zero').label).toBe('实测为零');
    expect(encryptedSnapshotAvailabilityCopy('no_sample').label).toBe('无样本');
    expect(encryptedSnapshotAvailabilityCopy('not_computable').label).toBe('不可计算');
    expect(encryptedSnapshotAvailabilityCopy('unavailable').label).toBe('来源不可用');
    expect(encryptedSnapshotAvailabilityCopy('forbidden').label).toBe('字段受限');
  });

  it('derives bounded evidence drilldowns without inventing references', () => {
    expect(encryptedSnapshotDrilldownPath({ pcap_index_ids: ['pcap/a b'] })).toBe('/forensics?pcap_index=pcap%2Fa%20b');
    expect(encryptedSnapshotDrilldownPath({ evidence_refs: ['evidence-1'] })).toBe('/forensics?query=evidence-1');
    expect(encryptedSnapshotDrilldownPath({ session_id: 'session-1' })).toBe('/forensics?session=session-1');
    expect(encryptedSnapshotDrilldownPath({ source_event_ids: ['event-1'] })).toBe('/alerts?event_id=event-1');
    expect(encryptedSnapshotDrilldownPath({ unrelated: 'no-reference' })).toBeUndefined();
  });

  it('keeps model changes, partial limitations, permission and empty states visible', () => {
    const base = {
      sample_count: 0,
      source: 'clickhouse.traffic.feature_fp',
      source_watermark: 'closed-window-1',
      rule_versions: [] as string[],
      model_versions: [] as string[],
      partial: false,
      missing_reasons: [] as string[],
      facts: [],
    };
    const model = encryptedSnapshotSectionPresentation({ ...base, availability: 'available', sample_count: 1, model_versions: ['model-v2'], facts: [{ session_id: 'session-1' }] });
    expect(model.modelVersions).toBe('model-v2');
    expect(model.drilldown).toBe('/forensics?session=session-1');

    const partial = encryptedSnapshotSectionPresentation({ ...base, availability: 'not_computable', partial: true, missing_reasons: ['sample_bytes_not_persisted'] });
    expect(partial.limitations).toContain('sample_bytes_not_persisted');
    expect(partial.availability.label).toBe('不可计算');

    const forbidden = encryptedSnapshotSectionPresentation({ ...base, availability: 'forbidden', partial: true, missing_reasons: ['pcap:read_required'] });
    expect(forbidden.drilldown).toBeUndefined();
    expect(forbidden.drilldownLabel).toBe('字段权限不足');

    const empty = encryptedSnapshotSectionPresentation({ ...base, availability: 'no_sample' });
    expect(empty.availability.label).toBe('无样本');
    expect(empty.drilldownLabel).toBe('暂无可下钻事实');
  });

  it('renders the five source/version/limitation sections and the interpretation boundary', () => {
    const source = [
      'src/pages/EncryptedTrafficPage.tsx',
      'src/services/encryptedTrafficSnapshotPresentation.ts',
    ].map((file) => fs.readFileSync(path.join(process.cwd(), file), 'utf8')).join('\n');
    for (const marker of [
      "'flow_metadata'",
      "'plaintext_visible'",
      "'side_channel'",
      "'raw_reference'",
      "'randomness_statistics'",
      'section.source_watermark',
      'section.rule_versions',
      'section.model_versions',
      'section.missing_reasons',
      '加密传输本身不构成恶意判定',
      '缺失事实不会用模拟值填充',
    ]) {
      expect(source).toContain(marker);
    }
  });
});

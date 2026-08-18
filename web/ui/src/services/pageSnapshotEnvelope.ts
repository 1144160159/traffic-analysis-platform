export type PageSnapshotEnvelopeView = {
  data: unknown;
  error?: Record<string, unknown>;
  contractVersion?: number;
  snapshotId?: string;
  asOf?: string;
  traceId?: string;
  partial: boolean;
  missingSections: string[];
  sourceWatermarks: Record<string, string>;
};

export const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

export const unwrapPageSnapshotPayload = (payload: unknown): unknown => {
  if (!isRecord(payload)) return payload;
  return 'data' in payload ? unwrapPageSnapshotPayload(payload.data) : payload;
};

export const extractPageSnapshotList = (payload: unknown, keys: string[]): Record<string, unknown>[] => {
  const data = unwrapPageSnapshotPayload(payload);
  if (Array.isArray(data)) return data.filter(isRecord);
  if (isRecord(data)) {
    for (const key of keys) {
      const value = data[key];
      if (Array.isArray(value)) return value.filter(isRecord);
      if (isRecord(value)) {
        const nested = extractPageSnapshotList(value, keys);
        if (nested.length) return nested;
      }
    }
    for (const value of Object.values(data)) {
      if (Array.isArray(value)) return value.filter(isRecord);
    }
  }
  return [];
};

export const extractNamedPageSnapshotList = (payload: unknown, keys: string[]): Record<string, unknown>[] => {
  const data = unwrapPageSnapshotPayload(payload);
  if (!isRecord(data)) return [];
  for (const key of keys) {
    const value = data[key];
    if (Array.isArray(value)) return value.filter(isRecord);
  }
  return [];
};

export const readPageSnapshotEnvelope = (payload: unknown): PageSnapshotEnvelopeView => {
  const root = isRecord(payload) ? payload : {};
  const meta = isRecord(root.meta) ? root.meta : {};
  const error = isRecord(root.error) ? root.error : undefined;
  const rawWatermarks = isRecord(meta.source_watermarks) ? meta.source_watermarks : {};
  const sourceWatermarks = Object.fromEntries(
    Object.entries(rawWatermarks)
      .filter(([, value]) => typeof value === 'string')
      .map(([key, value]) => [key, value as string]),
  );
  return {
    data: unwrapPageSnapshotPayload(payload),
    error,
    contractVersion: finiteNumber(meta.contract_version),
    snapshotId: stringValue(meta.snapshot_id),
    asOf: stringValue(meta.as_of),
    traceId: stringValue(meta.trace_id),
    partial: meta.partial === true,
    missingSections: Array.isArray(meta.missing_sections)
      ? meta.missing_sections.map((value) => String(value)).filter(Boolean)
      : [],
    sourceWatermarks,
  };
};

const finiteNumber = (value: unknown) => (
  typeof value === 'number' && Number.isFinite(value) ? value : undefined
);

const stringValue = (value: unknown) => (
  typeof value === 'string' && value ? value : undefined
);


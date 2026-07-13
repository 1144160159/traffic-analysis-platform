import { appConfig } from '@/config/runtime';
import { api } from '@/services/api';

export type AlertBatchStatusItem = {
  alertId: string;
  stateVersion?: number;
};

export type BatchUpdateAlertStatusResult = {
  totalCount: number;
  successCount: number;
  failedCount: number;
  successIds: string[];
  failedIds: string[];
  errors: Record<string, string>;
  errorCodes: Record<string, string>;
  stateVersions: Record<string, number>;
};

export const buildBatchUpdateAlertStatusRequest = (
  items: AlertBatchStatusItem[],
  status: string,
  reason: string,
) => ({
  status,
  reason: reason.trim(),
  items: items
    .map((item) => ({
      alert_id: item.alertId.trim(),
      ...(isPositiveStateVersion(item.stateVersion) ? { state_version: Math.trunc(item.stateVersion) } : {}),
    }))
    .filter((item) => item.alert_id),
});

export async function batchUpdateAlertStatus(
  items: AlertBatchStatusItem[],
  status: string,
  reason: string,
): Promise<BatchUpdateAlertStatusResult> {
  const request = buildBatchUpdateAlertStatusRequest(items, status, reason);
  if (appConfig.useMock) {
    return {
      totalCount: request.items.length,
      successCount: request.items.length,
      failedCount: 0,
      successIds: request.items.map((item) => item.alert_id),
      failedIds: [],
      errors: {},
      errorCodes: {},
      stateVersions: Object.fromEntries(request.items.map((item) => [item.alert_id, item.state_version ?? 0]).filter(([, version]) => Number(version) > 0)),
    };
  }

  const response = await api.put('/v1/alerts/batch/status', request);
  const payload = unwrapPayload(response.data);
  return {
    totalCount: numberFrom(payload, ['total_count', 'totalCount']),
    successCount: numberFrom(payload, ['success_count', 'successCount']),
    failedCount: numberFrom(payload, ['failed_count', 'failedCount']),
    successIds: stringListFrom(valueAt(payload, ['success_ids', 'successIds'])),
    failedIds: stringListFrom(valueAt(payload, ['failed_ids', 'failedIds'])),
    errors: recordOfStrings(valueAt(payload, ['errors'])),
    errorCodes: recordOfStrings(valueAt(payload, ['error_codes', 'errorCodes'])),
    stateVersions: recordOfNumbers(valueAt(payload, ['state_versions', 'stateVersions'])),
  };
}

function isPositiveStateVersion(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0;
}

function unwrapPayload(payload: unknown): unknown {
  if (!isRecord(payload)) return payload;
  if ('data' in payload) return unwrapPayload(payload.data);
  return payload;
}

function valueAt(source: unknown, keys: string[]) {
  if (!isRecord(source)) return undefined;
  for (const key of keys) {
    if (key in source) return source[key];
  }
  return undefined;
}

function numberFrom(source: unknown, keys: string[]) {
  const value = valueAt(source, keys);
  const numeric = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
}

function stringListFrom(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((item) => String(item)).filter(Boolean);
  if (typeof value === 'string' && value) return [value];
  return [];
}

function recordOfStrings(value: unknown): Record<string, string> {
  if (!isRecord(value)) return {};
  return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, String(item)]));
}

function recordOfNumbers(value: unknown): Record<string, number> {
  if (!isRecord(value)) return {};
  return Object.fromEntries(
    Object.entries(value)
      .map(([key, item]) => [key, typeof item === 'number' ? item : Number(item)] as const)
      .filter(([, item]) => Number.isFinite(item)),
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

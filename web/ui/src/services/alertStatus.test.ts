import { describe, expect, it } from 'vitest';
import {
  alertAllowedNextStatuses,
  alertStatusLabel,
  canTransitionAlertStatus,
  normalizeAlertStatus,
} from '@/services/alertStatus';

describe('alertStatus', () => {
  it('normalizes backend, legacy and display status values', () => {
    expect(normalizeAlertStatus('new')).toBe('new');
    expect(normalizeAlertStatus('ALERT_STATUS_NEW')).toBe('new');
    expect(normalizeAlertStatus('ALERT_STATUS_REVIEWING')).toBe('triage');
    expect(normalizeAlertStatus('processing')).toBe('triage');
    expect(normalizeAlertStatus('false_positive')).toBe('closed');
    expect(normalizeAlertStatus('研判中')).toBe('triage');
    expect(alertStatusLabel('resolved')).toBe('已关闭');
  });

  it('mirrors the backend alert state machine transitions', () => {
    expect(alertAllowedNextStatuses('new')).toEqual(['triage', 'assigned', 'closed']);
    expect(alertAllowedNextStatuses('研判中')).toEqual(['assigned', 'closed']);
    expect(canTransitionAlertStatus('new', 'assigned')).toBe(true);
    expect(canTransitionAlertStatus('triage', 'assigned')).toBe(true);
    expect(canTransitionAlertStatus('triage', 'new')).toBe(false);
    expect(canTransitionAlertStatus('assigned', 'triage')).toBe(true);
    expect(canTransitionAlertStatus('closed', 'new')).toBe(true);
    expect(canTransitionAlertStatus('closed', 'triage')).toBe(false);
  });
});

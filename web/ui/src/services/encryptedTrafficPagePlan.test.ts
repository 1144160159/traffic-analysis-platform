import { describe, expect, it } from 'vitest';
import { encryptedTrafficPagePlan } from '@/services/encryptedTrafficPagePlan';
import { getPageActionPlan, getPageApiPlan, pageApiPlans } from '@/services/pageApiPlans';

describe('encrypted traffic page plan seam', () => {
  it('switches the compatibility registry and lookup to the extracted plan', () => {
    expect(pageApiPlans['encrypted-traffic']).toBe(encryptedTrafficPagePlan);
    expect(getPageApiPlan('encrypted-traffic')).toBe(encryptedTrafficPagePlan);
    expect(encryptedTrafficPagePlan.primary).toBe('/v1/encrypted-traffic/stats');
    expect(encryptedTrafficPagePlan.secondary).toEqual([
      '/v1/encrypted-traffic/sessions',
      '/v1/encrypted-traffic/ja3',
      '/v1/encrypted-traffic/tunnels',
      '/v1/encrypted-traffic/exfiltration',
      '/v1/encrypted-traffic/evidence',
    ]);
  });

  it('preserves the complete action identity and request contract', () => {
    expect(encryptedTrafficPagePlan.actions).toHaveLength(17);
    expect(new Set(encryptedTrafficPagePlan.actions?.map((action) => action.id)).size).toBe(17);
    expect(getPageActionPlan('encrypted-traffic', 'egress-create-alert')).toMatchObject({
      method: 'POST',
      endpoint: '/v1/encrypted-traffic/egress-actions',
      auditEvent: 'ENCRYPTED_EGRESS_ALERT_REQUESTED',
      defaultBody: { action: 'create_alert' },
    });
    expect(getPageActionPlan('encrypted-traffic', 'evidence-verify-hash')).toMatchObject({
      method: 'POST',
      endpoint: '/v1/encrypted-traffic/evidence-actions',
      auditEvent: 'ENCRYPTED_EVIDENCE_HASH_VERIFICATION_REQUESTED',
      defaultBody: { action: 'verify_hash' },
    });
    for (const action of encryptedTrafficPagePlan.actions ?? []) {
      expect(action.requiredScopes).toEqual(['alert:write']);
      expect(action.acceptedScopes).toEqual(['alert:write', 'alert:*', 'admin:*', '*']);
      expect(action.guardrails.length).toBeGreaterThanOrEqual(2);
    }
  });
});

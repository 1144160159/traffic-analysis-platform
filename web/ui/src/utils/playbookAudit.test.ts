import { describe, expect, it } from 'vitest';
import { isPlaybookRollbackEvidence } from './playbookAudit';

describe('isPlaybookRollbackEvidence', () => {
  it('recognizes the durable drill rollback audit action', () => {
    expect(isPlaybookRollbackEvidence('PLAYBOOK_DRILL_ROLLED_BACK')).toBe(true);
    expect(isPlaybookRollbackEvidence('PLAYBOOK_DRILL_COMPLETED')).toBe(false);
    expect(isPlaybookRollbackEvidence('ROLLBACK')).toBe(false);
  });
});

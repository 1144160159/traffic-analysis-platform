import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api, exportTopicArtifact, submitTopicAction } from './api';

describe('topic API contracts', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('reuses the preview export id when downloading a report artifact', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        data: {
          export_id: 'derived-export',
          topic: 'exfil',
          export_type: 'report',
          format: 'pdf',
          status: 'completed',
          file_name: 'exfil-report.pdf',
          content_type: 'application/pdf',
          content_base64: 'JVBERi0xLjQ=',
          result: { snapshot_sha256: 'sha256-value' },
          created_at: 1,
        },
      },
    } as never);

    await exportTopicArtifact(
      'exfiltration',
      'report',
      'pdf',
      {
        data_mode: 'simulated',
        simulation_id: 'topic-exfil-ui-v2',
        simulation_version: 'ui-suite-gpt-v2',
      },
      'preview-export',
    );

    expect(post).toHaveBeenCalledWith('/v1/topics/reports/export', expect.objectContaining({
      topic: 'exfil',
      format: 'pdf',
      source_export_id: 'preview-export',
      parameters: expect.objectContaining({
        data_mode: 'simulated',
        simulation_id: 'topic-exfil-ui-v2',
        simulation_version: 'ui-suite-gpt-v2',
      }),
    }));
  });

  it('maps a drill-down label to the backend trace action allowlist', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        data: {
          action_id: 'action-1',
          tenant_id: 'default',
          topic: 'apt',
          action: 'trace',
          label: '下钻',
          target: 'APT-EVT-001',
          data_mode: 'live',
          status: 'completed',
          requested_by: 'tester',
          created_at: 1,
        },
      },
    } as never);

    await submitTopicAction('apt', '下钻', 'APT-EVT-001', { data_mode: 'live' });

    expect(post).toHaveBeenCalledWith('/v1/topics/apt/actions', expect.objectContaining({
      action: 'trace',
      label: '下钻',
      target: 'APT-EVT-001',
    }));
  });
});

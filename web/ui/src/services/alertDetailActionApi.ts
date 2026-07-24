import { getPageActionPlan } from '@/services/pageApiPlans';
import { submitAlertTriageAction } from '@/services/alertTriageApi';

export type AlertDetailActionId =
  | 'alert-report-export'
  | 'alert-campaign-link'
  | 'alert-evidence-access'
  | 'alert-response-request'
  | 'alert-investigation-note';

export type AlertDetailActionInput = {
  alertId: string;
  actionId: AlertDetailActionId;
  target: string;
};

export type AlertDetailActionResult = {
  actionId: AlertDetailActionId;
  action: string;
  apiContract: string;
  auditEvent: string;
  jobId: string;
  status: 'recorded' | 'pending_approval';
  target: string;
  mode: 'live';
};

export async function submitAlertDetailAction({ alertId, actionId, target }: AlertDetailActionInput): Promise<AlertDetailActionResult> {
  const plan = getPageActionPlan('alert-detail', actionId);
  if (!plan) throw new Error(`未找到告警详情动作契约：${actionId}`);

  const isResponse = actionId === 'alert-response-request';
  const submission = await submitAlertTriageAction({
    kind: isResponse ? 'response-action' : 'investigation-note',
    alertId,
    action: plan.label,
    target,
    reason: `告警详情提交：${plan.label}`,
    dryRun: isResponse,
    detail: { action_id: actionId, source: 'alert-detail', api_contract: plan.endpoint },
  });
  return {
    actionId,
    action: plan.label,
    apiContract: plan.endpoint.replace('{id}', encodeURIComponent(alertId)),
    auditEvent: plan.auditEvent,
    jobId: submission.job_id ?? submission.view_id ?? '',
    status: submission.status ?? 'recorded',
    target,
    mode: 'live',
  };
}

import { getPageActionPlan } from '@/services/pageApiPlans';

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
  status: 'queued';
  target: string;
  mode: 'simulated';
};

// The backend endpoint is registered in pageApiPlans. Until the service is deployed,
// the UI returns a typed asynchronous simulation instead of issuing a known 404.
export async function submitAlertDetailAction({ alertId, actionId, target }: AlertDetailActionInput): Promise<AlertDetailActionResult> {
  const plan = getPageActionPlan('alert-detail', actionId);
  if (!plan) throw new Error(`未找到告警详情动作契约：${actionId}`);

  await new Promise<void>((resolve) => window.setTimeout(resolve, 180));
  const timestamp = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14);
  return {
    actionId,
    action: plan.label,
    apiContract: plan.endpoint.replace('{id}', encodeURIComponent(alertId)),
    auditEvent: plan.auditEvent,
    jobId: `SIM-ALERT-${timestamp}`,
    status: 'queued',
    target,
    mode: 'simulated',
  };
}

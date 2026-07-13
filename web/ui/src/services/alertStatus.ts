export type AlertStatusCode = 'new' | 'triage' | 'assigned' | 'closed';

const labels: Record<AlertStatusCode, string> = {
  new: '未处理',
  triage: '研判中',
  assigned: '已指派',
  closed: '已关闭',
};

const aliases: Record<string, AlertStatusCode> = {
  new: 'new',
  open: 'new',
  unhandled: 'new',
  alert_status_new: 'new',
  未处理: 'new',
  triage: 'triage',
  investigating: 'triage',
  investigation: 'triage',
  review: 'triage',
  reviewing: 'triage',
  in_progress: 'triage',
  processing: 'triage',
  alert_status_triage: 'triage',
  alert_status_reviewing: 'triage',
  研判中: 'triage',
  assigned: 'assigned',
  delegated: 'assigned',
  alert_status_assigned: 'assigned',
  已指派: 'assigned',
  closed: 'closed',
  resolved: 'closed',
  confirmed: 'closed',
  ignored: 'closed',
  false_positive: 'closed',
  alert_status_closed: 'closed',
  alert_status_resolved: 'closed',
  已关闭: 'closed',
};

const transitions: Record<AlertStatusCode, AlertStatusCode[]> = {
  new: ['triage', 'assigned', 'closed'],
  triage: ['assigned', 'closed'],
  assigned: ['triage', 'closed'],
  closed: ['new'],
};

export const normalizeAlertStatus = (value: string): AlertStatusCode | undefined => {
  const normalized = value.trim().toLowerCase();
  if (!normalized) return undefined;
  return aliases[normalized];
};

export const alertStatusLabel = (value: string) => {
  const status = normalizeAlertStatus(value);
  return status ? labels[status] : value || '未知';
};

export const alertStatusOptions: Array<{ value: AlertStatusCode; label: string }> = [
  { value: 'new', label: labels.new },
  { value: 'triage', label: labels.triage },
  { value: 'assigned', label: labels.assigned },
  { value: 'closed', label: labels.closed },
];

export const alertStatusFlow: AlertStatusCode[] = ['new', 'triage', 'assigned', 'closed'];

export const alertAllowedNextStatuses = (from: string) => {
  const status = normalizeAlertStatus(from);
  return status ? transitions[status] : [];
};

export const canTransitionAlertStatus = (from: string, to: string) => {
  const target = normalizeAlertStatus(to);
  return Boolean(target && alertAllowedNextStatuses(from).includes(target));
};

import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

export type AlertActionControl = {
  actionId: string;
  title: string;
  kind: 'saved-view' | 'response-action' | 'investigation-note';
  target: 'provided' | 'alert' | 'source-ip' | 'asset';
};

export type AlertAction = AlertActionControl & {
  alertId: string;
  targetValue: string;
  endpoint: string;
  auditEvent: string;
};

export const ALERT_SAVE_VIEW_CONTROL: AlertActionControl = {
  actionId: 'alert-view-save',
  title: '保存告警视图',
  kind: 'saved-view',
  target: 'provided',
};

export const ALERT_ROW_CONTROLS = {
  reanalyze: { actionId: 'alert-response-reanalyze', title: '重新研判告警', kind: 'response-action', target: 'alert' },
  note: { actionId: 'alert-investigation-note', title: '补充研判记录', kind: 'investigation-note', target: 'alert' },
} as const satisfies Record<string, AlertActionControl>;

export const ALERT_RESPONSE_CONTROLS: AlertActionControl[] = [
  { actionId: 'alert-response-isolate-host', title: '隔离主机', kind: 'response-action', target: 'asset' },
  { actionId: 'alert-response-block-connection', title: '阻断连接', kind: 'response-action', target: 'source-ip' },
  { actionId: 'alert-response-block-ip', title: '封禁 IP', kind: 'response-action', target: 'source-ip' },
  { actionId: 'alert-response-run-script', title: '下发脚本', kind: 'response-action', target: 'asset' },
  { actionId: 'alert-response-add-whitelist', title: '加入白名单', kind: 'response-action', target: 'source-ip' },
];

const ALERT_TABLE_ROW_HEIGHT = 39;
const ALERT_TABLE_HEADER_HEIGHT = 39;
const ALERT_TABLE_PAGINATION_RESERVE = 50;

export const alertTableVerticalScrollHeight = (
  viewportHeight: number,
  rowCount: number,
  pageSize: number,
  internalScrollAllowed = true,
): number | undefined => {
  if (!internalScrollAllowed) return undefined;
  const naturalBodyHeight = Math.min(rowCount, pageSize) * ALERT_TABLE_ROW_HEIGHT;
  const availableBodyHeight = Math.max(
    1,
    Math.floor(viewportHeight - ALERT_TABLE_HEADER_HEIGHT - ALERT_TABLE_PAGINATION_RESERVE),
  );
  return availableBodyHeight + 1 < naturalBodyHeight ? availableBodyHeight : undefined;
};

export function alertDetailRoute(alertId: string, currentSearch: URLSearchParams): string {
  const query = currentSearch.toString();
  const returnTo = `/alerts${query ? `?${query}` : ''}`;
  return `/alerts/${encodeURIComponent(alertId)}?returnTo=${encodeURIComponent(returnTo)}`;
}

export type AlertTriageTimelineItem = PageSnapshot['timeline'][number] & {
  time: string;
  timestamp: number;
};

const text = (row: SnapshotRow | undefined, key: string, fallback: string) => {
  const value = row?.[key];
  return value === undefined || value === null || value === '' ? fallback : String(value);
};

const alertTimelineEpoch = (value: string) => {
  if (!value) return 0;
  const numeric = Number(value);
  const parsed = Number.isFinite(numeric) && numeric > 0
    ? new Date(numeric < 10_000_000_000 ? numeric * 1_000 : numeric)
    : new Date(value);
  return Number.isFinite(parsed.getTime()) ? parsed.getTime() : 0;
};

const alertTimelineClock = (value: string) => {
  if (!value) return '--:--:--';
  const timestamp = alertTimelineEpoch(value);
  if (timestamp > 0) return new Date(timestamp).toLocaleTimeString('zh-CN', { hour12: false });
  return value.match(/(\d{2}:\d{2}:\d{2})/)?.[1] ?? '--:--:--';
};

export const alertTimelineItems = (row?: SnapshotRow): AlertTriageTimelineItem[] => {
  if (!row) {
    return [{
      time: '--:--:--',
      timestamp: 0,
      title: '暂无研判事件',
      description: '请选择一条告警以查看该记录的真实时间字段。',
      status: 'info',
    }];
  }
  const name = text(row, '告警名称', '-');
  const source = text(row, '源 IP', '-');
  const destination = text(row, '目的 IP', '-');
  const status = text(row, '状态', '-');
  const stateVersion = text(row, '__stateVersion', '');
  const timestamps = {
    firstSeen: text(row, '__firstSeen', text(row, '首次发生', '')),
    createdAt: text(row, '__createdAt', ''),
    lastSeen: text(row, '__lastSeen', ''),
    updatedAt: text(row, '__updatedAt', ''),
  };
  const timedItems = [
    {
      rawTime: timestamps.firstSeen,
      title: '首次发生',
      description: `${name} 首次观测，通信 ${source} → ${destination}`,
      status: 'info' as const,
    },
    {
      rawTime: timestamps.createdAt,
      title: '告警创建',
      description: '告警记录已写入研判队列。',
      status: 'warn' as const,
    },
    {
      rawTime: timestamps.lastSeen,
      title: '最近观测',
      description: '这是告警记录返回的最近观测时间。',
      status: 'warn' as const,
    },
    {
      rawTime: timestamps.updatedAt,
      title: '最近更新',
      description: stateVersion ? `状态记录已更新，版本 ${stateVersion}。` : '状态记录已更新。',
      status: /确认|关闭|忽略/.test(status) ? 'ok' as const : 'info' as const,
    },
  ]
    .filter((item) => Boolean(item.rawTime))
    .map((item) => ({
      time: alertTimelineClock(item.rawTime),
      timestamp: alertTimelineEpoch(item.rawTime),
      title: item.title,
      description: item.description,
      status: item.status,
    }))
    .sort((left, right) => left.timestamp - right.timestamp);
  const currentTime = timestamps.updatedAt || timestamps.lastSeen || timestamps.createdAt || timestamps.firstSeen;
  const currentItem: AlertTriageTimelineItem = {
    time: alertTimelineClock(currentTime),
    timestamp: alertTimelineEpoch(currentTime),
    title: '当前状态',
    description: `当前状态为 ${status}；未返回的研判动作不在此处推断。`,
    status: /确认|关闭|忽略/.test(status) ? 'ok' : 'info',
  };
  return [...timedItems, currentItem].slice(-5);
};

export const createAlertAction = (control: AlertActionControl, targetValue: string, alertId = targetValue): AlertAction => ({
  ...control,
  alertId,
  targetValue,
  endpoint: control.kind === 'saved-view'
    ? '/v1/alerts/views'
    : control.kind === 'response-action'
      ? `/v1/alerts/${alertId}/response-actions`
      : `/v1/alerts/${alertId}/investigation-notes`,
  auditEvent: control.kind === 'saved-view'
    ? 'ALERT_VIEW_SAVED'
    : control.kind === 'response-action'
      ? 'ALERT_RESPONSE_ACTION_REQUESTED'
      : 'ALERT_INVESTIGATION_NOTE_RECORDED',
});

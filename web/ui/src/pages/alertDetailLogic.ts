import type { AlertDetailEvidenceRow } from '@/services/alertDetailApi';

export const ALERT_DETAIL_EVIDENCE_PAGE_SIZE = 5;

export function evidenceViewRoute(row: AlertDetailEvidenceRow, alertId: string) {
  const configuredRoute = row.viewUrl?.trim();
  if (configuredRoute?.startsWith('/') && !configuredRoute.startsWith('//')) return configuredRoute;
  const params = new URLSearchParams({
    alert_id: alertId,
    evidence: row.文件记录,
    type: row.evidenceKind || row.证据类型,
  });
  return `/forensics?${params.toString()}`;
}

export function evidenceFocusView(title: string) {
  if (title.startsWith('全部 ') || title.startsWith('查看全部证据')) return 'all';
  if (title.startsWith('PCAP ') || title.startsWith('查看全部 PCAP')) return 'pcap';
  if (title.startsWith('Session ') || title.startsWith('查看全部 Session')) return 'session';
  if (title.startsWith('日志 ') || title.startsWith('查看全部 日志')) return 'logs';
  if (title.startsWith('图谱路径 ') || title.startsWith('查看全部 图谱路径')) return 'graph-path';
  if (title.startsWith('文件 ') || title.startsWith('查看全部 文件')) return 'files';
  return '';
}

export function evidenceFocusRoute(alertId: string, currentSearch: string, focusedEvidenceView: string) {
  const nextSearch = new URLSearchParams(currentSearch);
  nextSearch.delete('evidence');
  nextSearch.set('evidenceView', focusedEvidenceView);
  const query = nextSearch.toString();
  return `/alerts/${encodeURIComponent(alertId)}${query ? `?${query}` : ''}`;
}

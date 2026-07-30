import { describe, expect, it } from 'vitest';
import {
  ALERT_DETAIL_EVIDENCE_PAGE_SIZE,
  evidenceFocusRoute,
  evidenceFocusView,
  evidenceViewRoute,
} from '@/pages/AlertDetailPage';

describe('alert detail inline evidence tab routing', () => {
  it.each([
    ['全部 6', 'all'],
    ['PCAP 1', 'pcap'],
    ['Session 2', 'session'],
    ['日志 1', 'logs'],
    ['图谱路径 1', 'graph-path'],
    ['文件 1', 'files'],
    ['查看全部 文件 1 项', 'files'],
  ])('maps %s to the inline evidence tab %s', (label, view) => {
    expect(evidenceFocusView(label)).toBe(view);
  });

  it('uses a five-row page and keeps view navigation separate from download handling', () => {
    expect(ALERT_DETAIL_EVIDENCE_PAGE_SIZE).toBe(5);
    expect(evidenceViewRoute({
      证据类型: '日志',
      文件记录: 'ids-alert.log',
      内容摘要: 'IDS hit',
      大小: '183 KB',
      生成时间: '2026-06-20 03:43:03',
      状态: '已生成',
      操作: '下载 / 查看',
      viewUrl: '/audit?evidence=ids-alert',
    }, 'AL-1')).toBe('/audit?evidence=ids-alert');
    expect(evidenceViewRoute({
      证据类型: '文件',
      文件记录: 'hash.txt',
      内容摘要: 'SHA256',
      大小: '64 B',
      生成时间: '2026-06-20 03:43:04',
      状态: '已生成',
      操作: '下载 / 查看',
      viewUrl: 'https://untrusted.example/evidence',
    }, 'AL-1')).toBe('/forensics?alert_id=AL-1&evidence=hash.txt&type=%E6%96%87%E4%BB%B6');
  });

  it('keeps returnTo and unrelated query state when switching evidence tabs', () => {
    expect(evidenceFocusRoute(
      'alert-detail-accept-r802',
      '?returnTo=%2Falerts%3Fseverity%3Dhigh&evidence=pcap&trace=acceptance',
      'logs',
    )).toBe('/alerts/alert-detail-accept-r802?returnTo=%2Falerts%3Fseverity%3Dhigh&trace=acceptance&evidenceView=logs');
  });
});

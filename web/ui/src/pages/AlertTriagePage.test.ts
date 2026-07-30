import { describe, expect, it } from 'vitest';
import { alertTableVerticalScrollHeight, alertTimelineItems } from '@/pages/AlertTriagePage';

describe('alertTableVerticalScrollHeight', () => {
  it('keeps a ten-row page free of vertical scrolling when the panel has enough room', () => {
    expect(alertTableVerticalScrollHeight(500, 10, 10)).toBeUndefined();
  });

  it('enables vertical scrolling only when ten rows cannot fit', () => {
    expect(alertTableVerticalScrollHeight(450, 10, 10)).toBe(361);
  });

  it('does not reserve scroll space for a partially filled page', () => {
    expect(alertTableVerticalScrollHeight(450, 9, 10)).toBeUndefined();
  });

  it('uses the selected page size when more than ten rows are requested', () => {
    expect(alertTableVerticalScrollHeight(500, 20, 20)).toBe(411);
  });

  it('keeps the table inside an extremely short viewport without forcing a two-row minimum', () => {
    expect(alertTableVerticalScrollHeight(80, 10, 10)).toBe(1);
  });

  it('supports a fifty-row page and disables internal scrolling in the compact layout', () => {
    expect(alertTableVerticalScrollHeight(500, 50, 50)).toBe(411);
    expect(alertTableVerticalScrollHeight(180, 10, 10, false)).toBeUndefined();
  });

  it('does not add a scroll region for an empty page', () => {
    expect(alertTableVerticalScrollHeight(80, 0, 10)).toBeUndefined();
  });
});

describe('alertTimelineItems', () => {
  it('orders only returned alert timestamps and labels the current state without inventing evidence or response events', () => {
    const items = alertTimelineItems({
      '告警名称': 'statistical_anomaly',
      '源 IP': '10.0.0.1',
      '目的 IP': '10.0.0.2',
      状态: '处理中',
      __firstSeen: '2026-07-27T07:41:33Z',
      __createdAt: '2026-07-27T07:41:34Z',
      __lastSeen: '2026-07-27T07:42:00Z',
      __updatedAt: '2026-07-27T07:43:00Z',
      __stateVersion: 3,
    });

    expect(items.map((item) => item.title)).toEqual(['首次发生', '告警创建', '最近观测', '最近更新', '当前状态']);
    expect(items.slice(0, 4).map((item) => item.timestamp)).toEqual([...items.slice(0, 4).map((item) => item.timestamp)].sort((a, b) => a - b));
    expect(items.map((item) => item.description).join(' ')).not.toMatch(/证据生成|处置动作|规则命中/);
    expect(items[items.length - 1]?.description).toContain('未返回的研判动作不在此处推断');
  });

  it('shows an explicit empty state when no alert is selected', () => {
    expect(alertTimelineItems()).toEqual([
      expect.objectContaining({
        title: '暂无研判事件',
        time: '--:--:--',
      }),
    ]);
  });
});

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PageStateBoundary } from './PageStateBoundary';
import { resolvePageState, type PageStateKind } from './pageState';

describe('PageStateBoundary', () => {
  afterEach(() => cleanup());

  it.each<[PageStateKind, string]>([
    ['loading', '正在加载权威数据'],
    ['empty', '当前没有可显示的数据'],
    ['partial', '页面数据部分可用'],
    ['unavailable', '权威数据暂不可用'],
    ['conflict', '页面状态已发生冲突'],
    ['final', '权威业务内容'],
  ])('renders the %s state with stable semantics', (state, expectedText) => {
    const { container } = render(
      <PageStateBoundary state={state} missingSections={['evidence.provider']}>
        <div>权威业务内容</div>
      </PageStateBoundary>,
    );
    expect(container.querySelector(`[data-page-state="${state}"]`)).toBeInTheDocument();
    expect(screen.getByText(expectedText)).toBeInTheDocument();
    if (state === 'partial' || state === 'final') expect(screen.getByText('权威业务内容')).toBeInTheDocument();
  });

  it('retries unavailable state without exposing children as fallback content', () => {
    const onRetry = vi.fn();
    render(
      <PageStateBoundary state="unavailable" onRetry={onRetry}>
        <div>不应展示的旧内容</div>
      </PageStateBoundary>,
    );
    expect(screen.queryByText('不应展示的旧内容')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '刷新权威状态' }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('derives conflict, partial and final without collapsing them together', () => {
    expect(resolvePageState({ isLoading: true })).toBe('loading');
    expect(resolvePageState({ isLoading: false })).toBe('empty');
    expect(resolvePageState({ isLoading: false, error: { response: { status: 409 } } })).toBe('conflict');
    expect(resolvePageState({ isLoading: false, error: new Error('offline') })).toBe('unavailable');
    expect(resolvePageState({ isLoading: false, data: { id: 1 }, partial: true })).toBe('partial');
    expect(resolvePageState({ isLoading: false, data: { id: 1 } })).toBe('final');
  });
});

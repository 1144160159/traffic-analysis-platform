import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { CloseableConfirmButton } from './CloseableConfirmButton';

describe('CloseableConfirmButton', () => {
  beforeAll(() => {
    const getComputedStyle = window.getComputedStyle.bind(window);
    vi.spyOn(window, 'getComputedStyle').mockImplementation((element, pseudoElement) =>
      getComputedStyle(element, pseudoElement ? undefined : pseudoElement),
    );
  });

  afterEach(() => cleanup());
  afterAll(() => {
    vi.restoreAllMocks();
  });

  const expectModalClosed = async (modal: HTMLElement) => {
    await waitFor(() => {
      const wrap = modal.closest('.ant-modal-wrap') as HTMLElement | null;
      expect(modal.style.display === 'none' || wrap?.style.display === 'none').toBe(true);
    });
  };

  it('shows a top-right close button and dismisses the small popup', async () => {
    const onConfirm = vi.fn();
    const { baseElement } = render(
      <CloseableConfirmButton
        title="停止当前任务"
        description="确认后写入 audit trace。"
        confirmText="确认停止"
        danger
        buttonProps={{ size: 'small', danger: true }}
        onConfirm={onConfirm}
      >
        停止任务
      </CloseableConfirmButton>,
    );

    fireEvent.click(screen.getByRole('button', { name: '停止任务' }));

    expect(screen.getByText('停止当前任务')).toBeInTheDocument();
    const modal = baseElement.querySelector('.taf-small-confirm-modal') as HTMLElement;
    expect(modal).toBeInstanceOf(HTMLElement);
    const closeButton = baseElement.querySelector('.taf-small-confirm-modal .ant-modal-close');
    expect(closeButton).toBeInstanceOf(HTMLButtonElement);

    fireEvent.click(closeButton as HTMLButtonElement);

    await expectModalClosed(modal);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('closes after confirm callback completes', async () => {
    const onConfirm = vi.fn();
    const { baseElement } = render(
      <CloseableConfirmButton title="删除规则确认" description="删除前确认影响范围。" confirmText="确认删除" onConfirm={onConfirm}>
        删除规则
      </CloseableConfirmButton>,
    );

    fireEvent.click(screen.getByRole('button', { name: '删除规则' }));
    const modal = baseElement.querySelector('.taf-small-confirm-modal') as HTMLElement;
    expect(modal).toBeInstanceOf(HTMLElement);
    fireEvent.click(screen.getByRole('button', { name: '确认删除' }));

    await expectModalClosed(modal);
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});

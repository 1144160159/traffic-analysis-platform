import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useEffect, useState } from 'react';
import { useDrawerFocusReturn } from './useDrawerFocusReturn';

function DrawerHarness() {
  const [open, setOpen] = useState(false);
  const { captureReturnFocus, afterOpenChange } = useDrawerFocusReturn({ initialFocusSelector: '[data-test-drawer-heading]' });
  useEffect(() => afterOpenChange(open), [afterOpenChange, open]);
  return (
    <div>
      <button type="button" onClick={(event) => { captureReturnFocus(event.currentTarget); setOpen(true); }}>打开详情</button>
      {open && (
        <section role="dialog" aria-labelledby="test-drawer-title">
          <h2 id="test-drawer-title" tabIndex={-1} data-test-drawer-heading>详情标题</h2>
          <button type="button" onClick={() => setOpen(false)}>关闭详情</button>
        </section>
      )}
    </div>
  );
}

describe('useDrawerFocusReturn', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('moves focus into the drawer and restores the exact trigger after close', async () => {
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      callback(0);
      return 1;
    });
    render(<DrawerHarness />);
    const trigger = screen.getByRole('button', { name: '打开详情' });
    trigger.focus();
    fireEvent.click(trigger);
    await waitFor(() => expect(screen.getByRole('heading', { name: '详情标题' })).toHaveFocus());
    fireEvent.click(screen.getByRole('button', { name: '关闭详情' }));
    await waitFor(() => expect(trigger).toHaveFocus());
  });
});

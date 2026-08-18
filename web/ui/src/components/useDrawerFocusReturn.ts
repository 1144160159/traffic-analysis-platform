import { useCallback, useRef } from 'react';

type DrawerFocusOptions = {
  initialFocusSelector: string;
};

export function useDrawerFocusReturn({ initialFocusSelector }: DrawerFocusOptions) {
  const returnFocusRef = useRef<HTMLElement | null>(null);

  const captureReturnFocus = useCallback((element?: HTMLElement | null) => {
    const candidate = element ?? document.activeElement;
    returnFocusRef.current = candidate instanceof HTMLElement ? candidate : null;
  }, []);

  const afterOpenChange = useCallback((open: boolean) => {
    window.requestAnimationFrame(() => {
      if (open) {
        document.querySelector<HTMLElement>(initialFocusSelector)?.focus({ preventScroll: true });
        return;
      }
      const target = returnFocusRef.current;
      returnFocusRef.current = null;
      if (target?.isConnected) target.focus({ preventScroll: true });
    });
  }, [initialFocusSelector]);

  return { captureReturnFocus, afterOpenChange };
}

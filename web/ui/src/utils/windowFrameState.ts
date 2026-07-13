import { useEffect, useState, type CSSProperties } from 'react';

export type WindowFrameState = {
  innerWidth: number;
  innerHeight: number;
  outerWidth: number;
  outerHeight: number;
  screenAvailWidth: number;
  screenAvailHeight: number;
  frameWidth: number;
  frameHeight: number;
  windowed: boolean;
  constrainedWidth: boolean;
  compactWidth: boolean;
  compactHeight: boolean;
  shortHeight: boolean;
  narrowWidth: boolean;
};

export function getWindowFrameState(): WindowFrameState {
  if (typeof window === 'undefined') {
    return {
      innerWidth: 0,
      innerHeight: 0,
      outerWidth: 0,
      outerHeight: 0,
      screenAvailWidth: 0,
      screenAvailHeight: 0,
      frameWidth: 0,
      frameHeight: 0,
      windowed: false,
      constrainedWidth: false,
      compactWidth: false,
      compactHeight: false,
      shortHeight: false,
      narrowWidth: false,
    };
  }

  const innerWidth = Math.round(window.innerWidth || 0);
  const innerHeight = Math.round(window.innerHeight || 0);
  const rawOuterWidth = Math.round(window.outerWidth || 0);
  const rawOuterHeight = Math.round(window.outerHeight || 0);
  const outerWidth = rawOuterWidth >= innerWidth ? rawOuterWidth : innerWidth;
  const outerHeight = rawOuterHeight >= innerHeight ? rawOuterHeight : innerHeight;
  const screenAvailWidth = Math.round(window.screen?.availWidth || 0);
  const screenAvailHeight = Math.round(window.screen?.availHeight || 0);
  const frameWidth = Math.max(0, outerWidth - innerWidth);
  const frameHeight = Math.max(0, outerHeight - innerHeight);
  const effectiveWidth = Math.min(innerWidth || outerWidth, outerWidth || innerWidth);
  const effectiveHeight = Math.min(innerHeight || outerHeight, outerHeight || innerHeight);
  const hasPhysicalWindowFrame = frameHeight >= 48;
  const windowed = Boolean(
    (screenAvailWidth && outerWidth && outerWidth < screenAvailWidth - 24) ||
      (screenAvailHeight && outerHeight && outerHeight < screenAvailHeight - 24),
  );

  return {
    innerWidth,
    innerHeight,
    outerWidth,
    outerHeight,
    screenAvailWidth,
    screenAvailHeight,
    frameWidth,
    frameHeight,
    windowed,
    constrainedWidth: outerWidth <= 1440 || effectiveWidth <= 1366,
    compactWidth: outerWidth <= 1180 || effectiveWidth <= 1100,
    compactHeight: (hasPhysicalWindowFrame && (outerHeight <= 1080 || effectiveHeight <= 960)) || effectiveHeight <= 820,
    shortHeight: (hasPhysicalWindowFrame && (outerHeight <= 820 || effectiveHeight <= 760)) || effectiveHeight <= 700,
    narrowWidth: outerWidth <= 900 || effectiveWidth <= 900,
  };
}

export function getWindowFrameCssVars(frame: WindowFrameState) {
  return {
    '--taf-window-inner-width': `${frame.innerWidth || 0}px`,
    '--taf-window-inner-height': `${frame.innerHeight || 0}px`,
    '--taf-window-outer-width': `${frame.outerWidth || frame.innerWidth || 0}px`,
    '--taf-window-outer-height': `${frame.outerHeight || frame.innerHeight || 0}px`,
    '--taf-window-frame-width': `${frame.frameWidth || 0}px`,
    '--taf-window-frame-height': `${frame.frameHeight || 0}px`,
  } as CSSProperties;
}

export function applyWindowFrameCssVars(target: HTMLElement, frame: WindowFrameState) {
  const vars = getWindowFrameCssVars(frame);
  Object.entries(vars).forEach(([key, value]) => {
    target.style.setProperty(key, String(value));
  });
  target.classList.toggle('taf-window--os-windowed', frame.windowed);
  target.classList.toggle('taf-window--os-constrained-width', frame.constrainedWidth);
  target.classList.toggle('taf-window--os-compact-width', frame.compactWidth);
  target.classList.toggle('taf-window--os-compact-height', frame.compactHeight);
  target.classList.toggle('taf-window--os-short-height', frame.shortHeight);
  target.classList.toggle('taf-window--os-narrow-width', frame.narrowWidth);
  target.dataset.tafWindowInner = `${frame.innerWidth}x${frame.innerHeight}`;
  target.dataset.tafWindowOuter = `${frame.outerWidth}x${frame.outerHeight}`;
  target.dataset.tafWindowFrame = `${frame.frameWidth}x${frame.frameHeight}`;
  target.dataset.tafScreenAvail = `${frame.screenAvailWidth}x${frame.screenAvailHeight}`;
}

export function useWindowFrameState() {
  const [frameState, setFrameState] = useState<WindowFrameState>(() => getWindowFrameState());

  useEffect(() => {
    let frame = 0;
    const sync = () => {
      frame = 0;
      setFrameState((current) => {
        const next = getWindowFrameState();
        return current.innerWidth === next.innerWidth &&
          current.innerHeight === next.innerHeight &&
          current.outerWidth === next.outerWidth &&
          current.outerHeight === next.outerHeight &&
          current.screenAvailWidth === next.screenAvailWidth &&
          current.screenAvailHeight === next.screenAvailHeight
          ? current
          : next;
      });
    };
    const schedule = () => {
      if (frame) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(sync);
    };

    schedule();
    const interval = window.setInterval(schedule, 250);
    window.addEventListener('resize', schedule);
    window.visualViewport?.addEventListener('resize', schedule);
    window.screen?.orientation?.addEventListener?.('change', schedule);
    document.addEventListener('visibilitychange', schedule);
    return () => {
      if (frame) cancelAnimationFrame(frame);
      window.clearInterval(interval);
      window.removeEventListener('resize', schedule);
      window.visualViewport?.removeEventListener('resize', schedule);
      window.screen?.orientation?.removeEventListener?.('change', schedule);
      document.removeEventListener('visibilitychange', schedule);
    };
  }, []);

  return frameState;
}

export function useDocumentWindowFrameCssVars() {
  const frame = useWindowFrameState();

  useEffect(() => {
    applyWindowFrameCssVars(document.documentElement, frame);
  }, [frame]);
}

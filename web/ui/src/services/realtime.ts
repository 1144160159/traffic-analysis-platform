import { useEffect, useMemo, useState } from 'react';
import { appConfig } from '@/config/runtime';
import type { SessionPrincipal } from '@/routes/access';
import { getAuthToken } from '@/services/authStorage';

export type RealtimeStatus = 'disabled' | 'idle' | 'connecting' | 'connected' | 'error' | 'closed';

export type RealtimeConnectionOptions = {
  enabled: boolean;
  token?: string | null;
  authEnabled: boolean;
};

export type RealtimeUrlOptions = {
  wsUrl: string;
  token?: string | null;
  tenantId?: string;
  baseHref?: string;
};

export const realtimeStatusLabel = (status: RealtimeStatus) => {
  const labels: Record<RealtimeStatus, string> = {
    disabled: '未启用',
    idle: '待授权',
    connecting: '连接中',
    connected: '已连接',
    error: '异常',
    closed: '已断开',
  };
  return labels[status];
};

export const shouldConnectRealtime = ({ enabled, token, authEnabled }: RealtimeConnectionOptions) => {
  if (!enabled) return false;
  if (!authEnabled) return true;
  return Boolean(token);
};

export const normalizeWsUrl = (wsUrl: string, baseHref = window.location.href) => {
  const rawUrl = wsUrl.trim() || '/ws';
  const base = new URL(baseHref);
  const parsed = rawUrl.startsWith('ws://') || rawUrl.startsWith('wss://') ? new URL(rawUrl) : new URL(rawUrl, base);
  if (parsed.protocol === 'http:') parsed.protocol = 'ws:';
  if (parsed.protocol === 'https:') parsed.protocol = 'wss:';
  return parsed;
};

export const buildRealtimeUrl = ({ wsUrl, token, tenantId, baseHref }: RealtimeUrlOptions) => {
  const url = normalizeWsUrl(wsUrl, baseHref);
  if (token) url.searchParams.set('token', token);
  if (tenantId) url.searchParams.set('tenant_id', tenantId);
  return url.toString();
};

export const useAuthorizedRealtime = (currentUser?: SessionPrincipal) => {
  const [status, setStatus] = useState<RealtimeStatus>(appConfig.enableRealtime ? 'idle' : 'disabled');

  useEffect(() => {
    if (!appConfig.enableRealtime) {
      setStatus('disabled');
      return undefined;
    }

    const token = getAuthToken();
    if (!currentUser || !shouldConnectRealtime({ enabled: appConfig.enableRealtime, token, authEnabled: appConfig.authEnabled })) {
      setStatus('idle');
      return undefined;
    }

    let active = true;
    let socket: WebSocket;
    setStatus('connecting');

    const handleOpen = () => active && setStatus('connected');
    const handleError = () => active && setStatus('error');
    const handleClose = () => active && setStatus('closed');

    try {
      socket = new WebSocket(buildRealtimeUrl({ wsUrl: appConfig.wsUrl, token, tenantId: currentUser.tenantId }));
    } catch {
      setStatus('error');
      return undefined;
    }

    socket.addEventListener('open', handleOpen);
    socket.addEventListener('error', handleError);
    socket.addEventListener('close', handleClose);

    return () => {
      active = false;
      socket.removeEventListener('open', handleOpen);
      socket.removeEventListener('error', handleError);
      socket.removeEventListener('close', handleClose);
      if (socket.readyState === WebSocket.CONNECTING || socket.readyState === WebSocket.OPEN) {
        socket.close(1000, 'auth-context-disposed');
      }
    };
  }, [currentUser]);

  return useMemo(
    () => ({
      status,
      label: realtimeStatusLabel(status),
    }),
    [status],
  );
};

import { describe, expect, it } from 'vitest';
import { buildRealtimeUrl, normalizeWsUrl, shouldConnectRealtime } from '@/services/realtime';

describe('realtime authorization', () => {
  it('converts relative websocket paths to the current secure websocket origin', () => {
    expect(normalizeWsUrl('/ws/events', 'https://console.example.com/dashboard').toString()).toBe(
      'wss://console.example.com/ws/events',
    );
    expect(normalizeWsUrl('/ws/events', 'http://127.0.0.1:4177/dashboard').toString()).toBe(
      'ws://127.0.0.1:4177/ws/events',
    );
  });

  it('keeps absolute websocket URLs and appends authorized query context', () => {
    expect(
      buildRealtimeUrl({
        wsUrl: 'wss://traffic.example.com/ws/stream?channel=alerts',
        token: 'token-1',
        tenantId: 'tenant-a',
      }),
    ).toBe('wss://traffic.example.com/ws/stream?channel=alerts&token=token-1&tenant_id=tenant-a');
  });

  it('requires an auth token when auth is enabled', () => {
    expect(shouldConnectRealtime({ enabled: true, authEnabled: true, token: null })).toBe(false);
    expect(shouldConnectRealtime({ enabled: true, authEnabled: true, token: 'token-1' })).toBe(true);
    expect(shouldConnectRealtime({ enabled: true, authEnabled: false, token: null })).toBe(true);
    expect(shouldConnectRealtime({ enabled: false, authEnabled: false, token: null })).toBe(false);
  });
});

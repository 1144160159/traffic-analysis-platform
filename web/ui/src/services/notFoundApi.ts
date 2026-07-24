import { api } from '@/services/api';

export type NavigationMissStatus = {
  id: string;
  label: string;
  state: 'healthy' | 'degraded' | 'unavailable' | string;
  value: string;
};

export type NavigationMissContext = {
  event_id: string;
  trace_id: string;
  occurred_at: string;
  tenant_id: string;
  tenant_name: string;
  site_name: string;
  access_source: string;
  audit_action: string;
  persisted: boolean;
  statuses: NavigationMissStatus[];
};

export type NavigationSupportContext = {
  support_request_id: string;
  navigation_event_id: string;
  trace_id: string;
  occurred_at: string;
  queue: string;
  status: 'queued' | string;
  audit_action: string;
  persisted: boolean;
};

export async function recordNavigationMiss(eventId: string): Promise<NavigationMissContext> {
  const response = await api.post<NavigationMissContext>('/v1/auth/navigation-miss', {
    event_id: eventId,
    source: 'web-ui',
  });
  return response.data;
}

export async function requestNavigationSupport(eventId: string): Promise<NavigationSupportContext> {
  const response = await api.post<NavigationSupportContext>('/v1/auth/navigation-miss/support', {
    event_id: eventId,
  });
  return response.data;
}

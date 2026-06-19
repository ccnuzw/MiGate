import { get } from './client';
import type { DashboardSummary, Resources, TrafficClient, TrafficInbound, TrafficSeriesPoint, TrafficSummary } from './types';

export const trafficAPI = {
  dashboardSummary: () => get<DashboardSummary>('/api/dashboard/summary'),
  trafficSummary: () => get<TrafficSummary>('/api/traffic/summary'),
  trafficInbounds: () => get<{ inbounds: TrafficInbound[] }>('/api/traffic/inbounds'),
  trafficClients: () => get<{ clients: TrafficClient[] }>('/api/traffic/clients'),
  trafficSeries: (params: { scope_type?: 'client' | 'inbound' | 'outbound' | 'core'; since?: string; limit?: number } = {}) => {
    const query = new URLSearchParams();
    if (params.scope_type) query.set('scope_type', params.scope_type);
    if (params.since) query.set('since', params.since);
    if (params.limit) query.set('limit', String(params.limit));
    const suffix = query.toString();
    return get<{ series: TrafficSeriesPoint[] }>(`/api/traffic/series${suffix ? `?${suffix}` : ''}`);
  },
  resources: () => get<Resources>('/api/system/resources'),
};

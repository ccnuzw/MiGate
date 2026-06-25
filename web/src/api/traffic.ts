import { get } from './client';
import type { DashboardSummary, Resources, TrafficV2AnalyticsResponse, TrafficV2Snapshot } from './types';

export const trafficV2StreamPath = '/api/traffic/v2/stream';

export const trafficAPI = {
  dashboardSummary: () => get<DashboardSummary>('/api/dashboard/summary'),
  trafficV2Snapshot: () => get<TrafficV2Snapshot>('/api/traffic/v2/snapshot'),
  trafficV2Analytics: (params: { range?: string; metric?: string; scope_type?: string; top?: number } = {}) => {
    const query = new URLSearchParams();
    if (params.range) query.set('range', params.range);
    if (params.metric) query.set('metric', params.metric);
    if (params.scope_type) query.set('scope_type', params.scope_type);
    if (params.top) query.set('top', String(params.top));
    const suffix = query.toString();
    return get<TrafficV2AnalyticsResponse>(`/api/traffic/v2/analytics${suffix ? `?${suffix}` : ''}`);
  },
  resources: () => get<Resources>('/api/system/resources'),
};

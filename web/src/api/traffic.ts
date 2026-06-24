import { get } from './client';
import type { DashboardSummary, Resources, TrafficV2SeriesPoint, TrafficV2Snapshot } from './types';

export const trafficV2StreamPath = '/api/traffic/v2/stream';

export const trafficAPI = {
  dashboardSummary: () => get<DashboardSummary>('/api/dashboard/summary'),
  trafficV2Snapshot: () => get<TrafficV2Snapshot>('/api/traffic/v2/snapshot'),
  trafficV2Series: (params: { since?: string; limit?: number } = {}) => {
    const query = new URLSearchParams();
    if (params.since) query.set('since', params.since);
    if (params.limit) query.set('limit', String(params.limit));
    const suffix = query.toString();
    return get<{ series: TrafficV2SeriesPoint[] }>(`/api/traffic/v2/series${suffix ? `?${suffix}` : ''}`);
  },
  resources: () => get<Resources>('/api/system/resources'),
};

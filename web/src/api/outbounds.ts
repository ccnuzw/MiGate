import { del, get, post, put } from './client';
import type { Outbound, OutboundSubscription, OutboundSubscriptionPreview, PingResult, ProxyPoolProxy, ProxyPoolResponse, SingboxWriteResponse } from './types';

export const outboundsAPI = {
  outbounds: () => get<Outbound[]>('/api/outbounds'),
  createOutbound: (body: Record<string, unknown>) => post<({ outbound: Outbound } | Outbound) & SingboxWriteResponse>('/api/outbounds', body),
  updateOutbound: (id: number, body: Record<string, unknown>) => put<({ outbound: Outbound } | Outbound) & SingboxWriteResponse>(`/api/outbounds/${id}`, body),
  deleteOutbound: (id: number) => del<{ status: string } & SingboxWriteResponse>(`/api/outbounds/${id}`),
  toggleOutbound: (item: Outbound, enabled: boolean) => put<({ outbound: Outbound } | Outbound) & SingboxWriteResponse>(`/api/outbounds/${item.id}`, { ...item, enabled }),
  pingOutbound: (id: number) => get<PingResult>(`/api/outbounds/${id}/ping`),
  speedtestAll: () => post<Record<string, PingResult>>('/api/outbounds/speedtest-all'),
  reorderOutbounds: (ids: number[]) => post<{ status: string } & SingboxWriteResponse>('/api/outbounds/reorder', { ids }),
  proxyPool: (type: 'socks5' | 'http' | 'https', country = '') => get<ProxyPoolResponse>(`/api/outbounds/${type}-pool${country ? `?country=${encodeURIComponent(country)}` : ''}`),
  pingProxyPool: (type: 'socks5' | 'http' | 'https', proxy: Pick<ProxyPoolProxy, 'address' | 'port'>) => post<PingResult>(`/api/outbounds/${type}-pool/ping`, proxy),
  importProxyPool: (type: 'socks5' | 'http' | 'https', proxy: ProxyPoolProxy) => post<({ outbound: Outbound } | Outbound) & SingboxWriteResponse>(`/api/outbounds/${type}-pool/import`, proxy),
  socks5Pool: (country = '') => get<ProxyPoolResponse>(`/api/outbounds/socks5-pool${country ? `?country=${encodeURIComponent(country)}` : ''}`),
  pingSocks5Pool: (proxy: Pick<ProxyPoolProxy, 'address' | 'port'>) => post<PingResult>('/api/outbounds/socks5-pool/ping', proxy),
  importSocks5Pool: (proxy: ProxyPoolProxy) => post<({ outbound: Outbound } | Outbound) & SingboxWriteResponse>('/api/outbounds/socks5-pool/import', proxy),
  outboundSubscriptions: () => get<OutboundSubscription[]>('/api/outbound-subscriptions'),
  createOutboundSubscription: (body: Record<string, unknown>) => post<{ subscription: OutboundSubscription }>('/api/outbound-subscriptions', body),
  updateOutboundSubscription: (id: number, body: Record<string, unknown>) => put<{ subscription: OutboundSubscription; needs_refresh?: boolean } & SingboxWriteResponse>(`/api/outbound-subscriptions/${id}`, body),
  deleteOutboundSubscription: (id: number) => del<{ status: string } & SingboxWriteResponse>(`/api/outbound-subscriptions/${id}`),
  refreshOutboundSubscription: (id: number) => post<{ result: { subscription_id: number; count: number; skipped_count: number } } & SingboxWriteResponse>(`/api/outbound-subscriptions/${id}/refresh`),
  refreshOutboundSubscriptions: () => post<{ results: Array<Record<string, unknown>> } & SingboxWriteResponse>('/api/outbound-subscriptions/refresh'),
  previewOutboundSubscription: (body: Record<string, unknown>) => post<OutboundSubscriptionPreview>('/api/outbound-subscriptions/preview', body),
  reorderOutboundSubscriptions: (ids: number[]) => post<{ status: string } & SingboxWriteResponse>('/api/outbound-subscriptions/reorder', { ids }),
};

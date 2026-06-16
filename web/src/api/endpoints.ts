import { del, get, patch, post, put } from './client';
import type {
  CoreStatus,
  ConfigValidation,
  CoreActionResponse,
  DashboardSummary,
  Inbound,
  Outbound,
  PingResult,
  Resources,
  RoutingRule,
  Session,
  SessionInfo,
  Settings,
  CertStatus,
  ProxyPoolProxy,
  ProxyPoolResponse,
  TrafficClient,
  TrafficInbound,
  TrafficSeriesPoint,
  TrafficSummary,
  UpdateCheck,
  UpdateStatus,
  VersionResponse,
} from './types';

type RoutingRuleResponse = RoutingRule | { rule: RoutingRule };

function unwrapRoutingRule(response: RoutingRuleResponse): RoutingRule {
  if ('rule' in response) return (response as { rule: RoutingRule }).rule;
  return response as RoutingRule;
}

export const api = {
  login: (username: string, password: string) => post<{ status: string }>('/api/login', { username, password }),
  logout: () => post<{ status: string }>('/api/logout'),
  session: () => get<Session>('/api/session'),
  sessions: () => get<SessionInfo[]>('/api/sessions'),
  revokeSession: (id: number) => del<{ status: string }>(`/api/sessions/${id}`),
  revokeOtherSessions: () => del<{ status: string; revoked: number }>('/api/sessions/others'),
  health: () => get<{ status: string; mode: string }>('/api/health'),
  version: () => get<VersionResponse>('/api/version'),
  inbounds: async () => {
    const response = await get<Inbound[] | { inbounds?: Inbound[] }>('/api/inbounds');
    return Array.isArray(response) ? response : response.inbounds || [];
  },
  inboundTraffic: async () => {
    const response = await get<Inbound[] | { inbounds?: Inbound[] }>('/api/inbounds?refresh=traffic');
    return Array.isArray(response) ? response : response.inbounds || [];
  },
  createInbound: (body: Record<string, unknown>) => post<Inbound | { inbound: Inbound }>('/api/inbounds', body),
  updateInbound: (id: number, body: Record<string, unknown>) => put<Inbound>(`/api/inbounds/${id}`, body),
  deleteInbound: (id: number) => del<{ status: string }>(`/api/inbounds/${id}`),
  toggleInbound: (id: number, enabled: boolean) => patch<Inbound>(`/api/inbounds/${id}/enabled`, { enabled }),
  createClient: (inboundId: number, body: Record<string, unknown>) => post(`/api/inbounds/${inboundId}/clients`, body),
  updateClient: (inboundId: number, id: number, body: Record<string, unknown>) => put(`/api/inbounds/${inboundId}/clients/${id}`, body),
  deleteClient: (inboundId: number, id: number) => del(`/api/inbounds/${inboundId}/clients/${id}`),
  toggleClient: (inboundId: number, id: number, enabled: boolean) => patch(`/api/inbounds/${inboundId}/clients/${id}/enabled`, { enabled }),
  resetClientTraffic: (inboundId: number, id: number) => post(`/api/inbounds/${inboundId}/clients/${id}/reset-traffic`),
  outbounds: () => get<Outbound[]>('/api/outbounds'),
  createOutbound: (body: Record<string, unknown>) => post('/api/outbounds', body),
  updateOutbound: (id: number, body: Record<string, unknown>) => put(`/api/outbounds/${id}`, body),
  deleteOutbound: (id: number) => del(`/api/outbounds/${id}`),
  toggleOutbound: (item: Outbound, enabled: boolean) => put(`/api/outbounds/${item.id}`, { ...item, enabled }),
  pingOutbound: (id: number) => get<PingResult>(`/api/outbounds/${id}/ping`),
  speedtestAll: () => post<Record<string, PingResult>>('/api/outbounds/speedtest-all'),
  reorderOutbounds: (ids: number[]) => post<{ status: string }>('/api/outbounds/reorder', { ids }),
  proxyPool: (type: 'socks5' | 'http' | 'https', country = '') => get<ProxyPoolResponse>(`/api/outbounds/${type}-pool${country ? `?country=${encodeURIComponent(country)}` : ''}`),
  pingProxyPool: (type: 'socks5' | 'http' | 'https', proxy: Pick<ProxyPoolProxy, 'address' | 'port'>) => post<PingResult>(`/api/outbounds/${type}-pool/ping`, proxy),
  importProxyPool: (type: 'socks5' | 'http' | 'https', proxy: ProxyPoolProxy) => post(`/api/outbounds/${type}-pool/import`, proxy),
  socks5Pool: (country = '') => get<ProxyPoolResponse>(`/api/outbounds/socks5-pool${country ? `?country=${encodeURIComponent(country)}` : ''}`),
  pingSocks5Pool: (proxy: Pick<ProxyPoolProxy, 'address' | 'port'>) => post<PingResult>('/api/outbounds/socks5-pool/ping', proxy),
  importSocks5Pool: (proxy: ProxyPoolProxy) => post('/api/outbounds/socks5-pool/import', proxy),
  routingRules: () => get<RoutingRule[]>('/api/routing-rules'),
  createRoutingRule: async (body: Record<string, unknown>) => unwrapRoutingRule(await post<RoutingRuleResponse>('/api/routing-rules', body)),
  updateRoutingRule: async (id: number, body: Record<string, unknown>) => unwrapRoutingRule(await put<RoutingRuleResponse>(`/api/routing-rules/${id}`, body)),
  deleteRoutingRule: (id: number) => del(`/api/routing-rules/${id}`),
  reorderRoutingRules: (ids: number[]) => post<{ status: string }>('/api/routing-rules/reorder', { ids }),
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
  stats: () => get<unknown>('/api/stats'),
  resources: () => get<Resources>('/api/system/resources'),
  xrayStatus: () => get<CoreStatus>('/api/xray/status'),
  xrayVersion: () => get<VersionResponse>('/api/xray/version'),
  xrayConfig: () => get<unknown>('/api/xray/config'),
  xrayValidate: () => get<ConfigValidation>('/api/xray/validate'),
  xrayApply: () => post<CoreActionResponse>('/api/xray/apply', { confirm: true, allow_system_changes: true }),
  xrayInstall: () => post<CoreActionResponse>('/api/xray/install', { confirm: true, allow_system_changes: true }),
  xrayUninstall: () => post<CoreActionResponse>('/api/xray/uninstall', { confirm: true, allow_system_changes: true }),
  xrayLogs: () => get<{ logs?: string; lines?: string[] }>('/api/xray/logs'),
  singboxStatus: () => get<CoreStatus>('/api/singbox/status'),
  singboxVersion: () => get<VersionResponse>('/api/singbox/version'),
  singboxConfig: () => get<unknown>('/api/singbox/config'),
  singboxValidate: () => get<ConfigValidation>('/api/singbox/validate'),
  singboxApply: () => post<CoreActionResponse>('/api/singbox/apply', { confirm: true, allow_system_changes: true }),
  singboxInstall: () => post<CoreActionResponse>('/api/singbox/install', { confirm: true, allow_system_changes: true }),
  singboxUninstall: () => post<CoreActionResponse>('/api/singbox/uninstall', { confirm: true, allow_system_changes: true }),
  singboxLogs: () => get<{ logs?: string; lines?: string[] }>('/api/singbox/logs'),
  settings: () => get<Settings>('/api/settings'),
  saveSettings: (body: Settings) => put<{ status: string }>('/api/settings', body),
  certStatus: () => get<CertStatus>('/api/cert/status'),
  issueCert: (domain: string, email: string) => post<{ status: string; domain: string; cert_path: string; key_path: string }>('/api/cert/issue', { domain, email, confirm: true, allow_system_changes: true }),
  restart: () => post<{ status: string }>('/api/restart', { confirm: true, allow_system_changes: true }),
  serviceStatus: () => get<{ service: string; status: string; detail?: string }>('/api/service/status'),
  updateCheck: () => get<UpdateCheck>('/api/update/check'),
  updateStatus: () => get<UpdateStatus>('/api/update/status'),
  update: () => post<{ status: string; command: string; message?: string }>('/api/update', { confirm: true, allow_system_changes: true }),
};

import { del, get, patch, post, put } from './client';
import type {
  CoreStatus,
  ConfigValidation,
  CoreActionResponse,
  Client,
  DashboardSummary,
  CreateClientResponse,
  CreateInboundResponse,
  Inbound,
  InboundCapability,
  Outbound,
  PingResult,
  Resources,
  RoutingRule,
  Session,
  SessionInfo,
  Settings,
  CertStatus,
  SingboxConfigPreview,
  SingboxDiagnostics,
  SingboxWriteResponse,
  XrayConfigPreview,
  XrayDiagnostics,
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

export type RoutingRuleResponse = (RoutingRule | { rule: RoutingRule }) & SingboxWriteResponse;

function unwrapRoutingRule(response: RoutingRuleResponse): RoutingRule & SingboxWriteResponse {
  if ('rule' in response && response.rule) {
    return {
      ...(response.rule as RoutingRule),
      applied: response.applied,
      error: response.error,
      detail: response.detail,
      warnings: response.warnings,
      post_apply_warnings: response.post_apply_warnings,
      non_fatal_warnings: response.non_fatal_warnings,
      singbox: response.singbox,
      xray: response.xray,
    };
  }
  return response as RoutingRule & SingboxWriteResponse;
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
  inboundCapabilities: async () => {
    const response = await get<{ capabilities?: InboundCapability[] }>('/api/inbound-capabilities');
    return response.capabilities || [];
  },
  generateRealityKeypair: () => post<{ private_key: string; public_key: string }>('/api/reality/keypair', {}),
  inboundTraffic: async () => {
    const response = await get<Inbound[] | { inbounds?: Inbound[] }>('/api/inbounds?refresh=traffic');
    return Array.isArray(response) ? response : response.inbounds || [];
  },
  createInbound: (body: Record<string, unknown>) => post<CreateInboundResponse>('/api/inbounds', body),
  updateInbound: (id: number, body: Record<string, unknown>) => put<(Inbound | { inbound: Inbound }) & SingboxWriteResponse>(`/api/inbounds/${id}`, body),
  deleteInbound: (id: number) => del<{ status: string } & SingboxWriteResponse>(`/api/inbounds/${id}`),
  toggleInbound: (id: number, enabled: boolean) => patch<(Inbound | { inbound: Inbound }) & SingboxWriteResponse>(`/api/inbounds/${id}/enabled`, { enabled }),
  createClient: (inboundId: number, body: Record<string, unknown>) => post<CreateClientResponse>(`/api/inbounds/${inboundId}/clients`, body),
  updateClient: (inboundId: number, id: number, body: Record<string, unknown>) => put<({ client?: Client } | Client) & SingboxWriteResponse>(`/api/inbounds/${inboundId}/clients/${id}`, body),
  deleteClient: (inboundId: number, id: number) => del<{ status: string } & SingboxWriteResponse>(`/api/inbounds/${inboundId}/clients/${id}`),
  toggleClient: (inboundId: number, id: number, enabled: boolean) => patch<({ client?: Client } | Client) & SingboxWriteResponse>(`/api/inbounds/${inboundId}/clients/${id}/enabled`, { enabled }),
  resetClientTraffic: (inboundId: number, id: number) => post(`/api/inbounds/${inboundId}/clients/${id}/reset-traffic`),
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
  routingRules: () => get<RoutingRule[]>('/api/routing-rules'),
  createRoutingRule: async (body: Record<string, unknown>) => unwrapRoutingRule(await post<RoutingRuleResponse>('/api/routing-rules', body)),
  updateRoutingRule: async (id: number, body: Record<string, unknown>) => unwrapRoutingRule(await put<RoutingRuleResponse>(`/api/routing-rules/${id}`, body)),
  deleteRoutingRule: (id: number) => del<{ status: string } & SingboxWriteResponse>(`/api/routing-rules/${id}`),
  reorderRoutingRules: (ids: number[]) => post<{ status: string } & SingboxWriteResponse>('/api/routing-rules/reorder', { ids }),
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
  xrayDiagnostics: () => get<XrayDiagnostics>('/api/xray/diagnostics'),
  xrayVersion: () => get<VersionResponse>('/api/xray/version'),
  xrayConfig: () => get<unknown>('/api/xray/config'),
  xrayConfigPreview: () => get<XrayConfigPreview>('/api/xray/config/preview'),
  xrayValidate: () => get<ConfigValidation>('/api/xray/validate'),
  xrayApply: () => post<CoreActionResponse>('/api/xray/apply', { confirm: true, allow_system_changes: true }),
  xrayInstall: () => post<CoreActionResponse>('/api/xray/install', { confirm: true, allow_system_changes: true }),
  xrayUninstall: () => post<CoreActionResponse>('/api/xray/uninstall', { confirm: true, allow_system_changes: true }),
  xrayLogs: () => get<{ logs?: string; lines?: string[] }>('/api/xray/logs'),
  singboxStatus: () => get<CoreStatus>('/api/singbox/status'),
  singboxDiagnostics: () => get<SingboxDiagnostics>('/api/singbox/diagnostics'),
  singboxVersion: () => get<VersionResponse>('/api/singbox/version'),
  singboxConfig: () => get<unknown>('/api/singbox/config'),
  singboxConfigPreview: () => get<SingboxConfigPreview>('/api/singbox/config/preview'),
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
  updateLogs: () => get<{ logs?: string; lines?: string[]; path?: string }>('/api/update/logs?lines=160'),
  update: () => post<{ status: string; command: string; message?: string }>('/api/update', { confirm: true, allow_system_changes: true }),
};

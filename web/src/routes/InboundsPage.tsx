import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Copy, Edit2, Plus, Power, RotateCcw, Trash2 } from 'lucide-react';
import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { ApiError, appPath } from '../api/client';
import { api } from '../api/endpoints';
import type { CertStatus, Client, Inbound } from '../api/types';
import { EmptyState, LoadingBlock, SpinnerButton, StatusBadge, useConfirm, useToast } from '../components/ui';
import { formatBytes, randomUUID } from '../lib/format';
import { useI18n } from '../lib/i18n';
import { usePageVisible } from '../lib/visibility';
import { PageTitle } from './OverviewPage';
import type { ClientValues, InboundValues } from './InboundsPageForms';

const InboundModal = lazy(() => import('./InboundsPageForms').then((module) => ({ default: module.InboundModal })));
const ClientModal = lazy(() => import('./InboundsPageForms').then((module) => ({ default: module.ClientModal })));

export const advancedFields = [
  'ws_path',
  'ws_host',
  'grpc_service_name',
  'reality_dest',
  'reality_server_names',
  'reality_short_id',
  'reality_private_key',
  'reality_public_key',
  'ss_method',
  'tls_cert_file',
  'tls_key_file',
  'tls_sni',
  'tls_fingerprint',
  'tls_alpn',
  'xhttp_path',
  'xhttp_mode',
  'hy2_up_mbps',
  'hy2_down_mbps',
  'hy2_obfs',
  'hy2_obfs_password',
  'hy2_mport',
  'tuic_congestion_control',
  'tuic_zero_rtt',
  'shadowtls_version',
  'shadowtls_password',
] as const;

export const numericAdvancedFields = new Set<(typeof advancedFields)[number]>([
  'hy2_up_mbps',
  'hy2_down_mbps',
  'shadowtls_version',
]);

type SortKey = 'id' | 'port' | 'protocol' | 'clients';

export default function InboundsPage() {
  const queryClient = useQueryClient();
  const confirm = useConfirm();
  const { showToast } = useToast();
  const { text } = useI18n();
  const visible = usePageVisible();
  const [editingInbound, setEditingInbound] = useState<Inbound | null>(null);
  const [clientInbound, setClientInbound] = useState<Inbound | null>(null);
  const [editingClient, setEditingClient] = useState<{ inbound: Inbound; client: Client } | null>(null);
  const [search, setSearch] = useState('');
  const [sort, setSort] = useState<SortKey>('id');
  const inbounds = useQuery({ queryKey: ['inbounds'], queryFn: api.inbounds, staleTime: 30_000 });
  const inboundTraffic = useQuery({
    queryKey: ['inbounds', 'traffic'],
    queryFn: api.inboundTraffic,
    enabled: visible && Boolean(inbounds.data),
    refetchInterval: visible ? 10_000 : false,
    staleTime: 5_000,
  });
  useEffect(() => {
    if (!inboundTraffic.data) return;
    queryClient.setQueryData<Inbound[]>(['inbounds'], (current) => mergeInboundTraffic(current || [], inboundTraffic.data || []));
  }, [inboundTraffic.data, queryClient]);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['inbounds'] });
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = (inbounds.data || []).filter((item) => !q || [item.remark, item.protocol, String(item.port), item.network, item.security].join(' ').toLowerCase().includes(q));
    return [...list].sort((a, b) => {
      if (sort === 'port') return a.port - b.port;
      if (sort === 'protocol') return a.protocol.localeCompare(b.protocol);
      if (sort === 'clients') return (b.clients || []).length - (a.clients || []).length;
      return a.id - b.id;
    });
  }, [inbounds.data, search, sort]);

  const toggleInbound = useMutation({
    mutationFn: (item: Inbound) => api.toggleInbound(item.id, !item.enabled),
    onSuccess: () => {
      showToast('入站状态已更新', 'success');
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, '入站状态更新失败'), 'error'),
  });
  const deleteInbound = useMutation({
    mutationFn: api.deleteInbound,
    onSuccess: () => {
      showToast('入站已删除', 'success');
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, '删除入站失败'), 'error'),
  });
  const deleteClient = useMutation({
    mutationFn: ({ inboundId, id }: { inboundId: number; id: number }) => api.deleteClient(inboundId, id),
    onSuccess: () => {
      showToast('客户端已删除', 'success');
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, '删除客户端失败'), 'error'),
  });
  const toggleClient = useMutation({
    mutationFn: ({ inboundId, client }: { inboundId: number; client: Client }) => api.toggleClient(inboundId, client.id, !client.enabled),
    onSuccess: () => {
      showToast('客户端状态已更新', 'success');
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, '客户端状态更新失败'), 'error'),
  });
  const resetTraffic = useMutation({
    mutationFn: ({ inboundId, id }: { inboundId: number; id: number }) => api.resetClientTraffic(inboundId, id),
    onSuccess: () => {
      showToast('流量已重置', 'success');
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, '重置流量失败'), 'error'),
  });

  if (inbounds.isLoading) return <LoadingBlock />;

  return (
    <div className="page-stack">
      <PageTitle
        title="入站与客户端"
        description="管理协议入站、客户端凭据、订阅链接和流量状态。"
        action={
          <button className="btn primary" onClick={() => setEditingInbound(createDefaultInbound())}>
            <Plus className="h-4 w-4" /> 新增入站
          </button>
        }
      />
      <div className="toolbar">
        <input className="max-w-md" placeholder="搜索入站、协议、端口..." value={search} onChange={(e) => setSearch(e.target.value)} />
        <select className="w-44" value={sort} onChange={(e) => setSort(e.target.value as SortKey)}>
          <option value="id">按创建顺序</option>
          <option value="port">按端口</option>
          <option value="protocol">按协议</option>
          <option value="clients">按客户端数</option>
        </select>
      </div>
      {filtered.length === 0 ? (
        <EmptyState title="暂无入站" description="创建第一个入站后，可继续为它添加客户端并复制订阅链接。" />
      ) : (
        <div className="grid gap-4">
          {filtered.map((inbound) => (
            <div key={inbound.id} className="resource-card">
              <div className="resource-header">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h2 className="truncate text-base font-semibold">{inbound.remark || `${inbound.protocol}:${inbound.port}`}</h2>
                    <StatusBadge enabled={inbound.enabled} />
                  </div>
                  <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-panel-muted">
                    <span>{inbound.protocol}</span>
                    <span>:{inbound.port}</span>
                    <span>{inbound.network || 'tcp'} / {inbound.security || 'none'}</span>
                    <span>{(inbound.clients || []).length} 客户端</span>
                  </div>
                </div>
                <div className="action-row">
                  <SpinnerButton className="icon-button" loading={toggleInbound.isPending} onClick={() => toggleInbound.mutate(inbound)} title="启停">
                    <Power className="h-4 w-4" />
                  </SpinnerButton>
                  <button className="icon-button" onClick={() => setEditingInbound(inbound)} title="编辑">
                    <Edit2 className="h-4 w-4" />
                  </button>
                  <button className="icon-button danger-text" onClick={async () => (await confirm({ title: '删除入站？', description: '该入站下的客户端也会被删除。', tone: 'danger' })) && deleteInbound.mutate(inbound.id)} title="删除">
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              </div>
              <div className="mt-3 grid gap-2 text-xs text-panel-muted sm:grid-cols-4">
                <MetaItem label={text('上行')} value={formatBytes(inbound.traffic_up)} />
                <MetaItem label={text('下行')} value={formatBytes(inbound.traffic_down)} />
                <MetaItem label={text('合计')} value={formatBytes(inbound.traffic_total)} />
                <MetaItem label={text('统计源')} value={sourceLabel(inbound.traffic_stats_source, inbound.realtime_stats_source, text)} />
              </div>
              <div className="mt-4 border-t border-panel-line pt-3">
                <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
                  <div className="text-sm font-medium">客户端</div>
                  <button className="btn secondary h-8" onClick={() => setClientInbound(inbound)}>
                    <Plus className="h-4 w-4" /> 新增客户端
                  </button>
                </div>
                <div className="grid gap-2">
                  {(inbound.clients || []).map((client) => (
                    <ClientRow
                      key={client.id}
                      inbound={inbound}
                      client={mergeClientTraffic(inbound, client)}
                      onCopySub={() => copyText(subscriptionURL(client), '订阅链接已复制', showToast)}
                      onCopyShare={() => copyShareLink(client, showToast)}
                      onToggle={() => toggleClient.mutate({ inboundId: inbound.id, client })}
                      onEdit={() => setEditingClient({ inbound, client })}
                      onReset={async () => (await confirm({ title: '重置客户端流量？' })) && resetTraffic.mutate({ inboundId: inbound.id, id: client.id })}
                      onDelete={async () => (await confirm({ title: '删除客户端？', tone: 'danger' })) && deleteClient.mutate({ inboundId: inbound.id, id: client.id })}
                    />
                  ))}
                  {(inbound.clients || []).length === 0 ? <EmptyState title="暂无客户端" /> : null}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
      <Suspense fallback={null}>
        {editingInbound ? <InboundModal inbound={editingInbound} onClose={() => setEditingInbound(null)} onSaved={refresh} /> : null}
        {clientInbound ? <ClientModal inbound={clientInbound} onClose={() => setClientInbound(null)} onSaved={refresh} /> : null}
        {editingClient ? <ClientModal inbound={editingClient.inbound} client={editingClient.client} onClose={() => setEditingClient(null)} onSaved={refresh} /> : null}
      </Suspense>
    </div>
  );
}

function ClientRow({
  client,
  onCopySub,
  onCopyShare,
  onToggle,
  onEdit,
  onReset,
  onDelete,
}: {
  inbound: Inbound;
  client: Client;
  onCopySub: () => void;
  onCopyShare: () => void;
  onToggle: () => void;
  onEdit: () => void;
  onReset: () => void;
  onDelete: () => void;
}) {
  const { text } = useI18n();
  const used = Number(client.up || 0) + Number(client.down || 0);
  const limit = Number(client.traffic_limit || 0);
  return (
    <div className="client-row">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate font-medium">{client.email}</span>
          <StatusBadge enabled={client.enabled} />
        </div>
        <div className="mt-1 break-all text-xs text-panel-muted">{client.uuid}</div>
        <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-panel-muted">
          <MetaItem label={text('上行')} value={formatBytes(client.up)} />
          <MetaItem label={text('下行')} value={formatBytes(client.down)} />
          <MetaItem label={text('限额')} value={limit > 0 ? `${formatBytes(used)} / ${formatBytes(limit)}` : text('不限制')} />
          <MetaItem label={text('过期')} value={client.expiry_at ? new Date(client.expiry_at * 1000).toLocaleString() : text('不限制')} />
          <MetaItem label={text('实时')} value={sourceLabel(client.traffic_stats_source, client.realtime_stats_source, text)} />
        </div>
      </div>
      <div className="action-row">
        <button className="icon-button" onClick={onCopySub} title="复制订阅链接"><Copy className="h-4 w-4" /></button>
        <button className="icon-button" onClick={onCopyShare} title="复制客户端分享链接"><Copy className="h-4 w-4" /></button>
        <button className="icon-button" onClick={onToggle} title="启停"><Power className="h-4 w-4" /></button>
        <button className="icon-button" onClick={onEdit} title="编辑"><Edit2 className="h-4 w-4" /></button>
        <button className="icon-button" onClick={onReset} title="重置流量"><RotateCcw className="h-4 w-4" /></button>
        <button className="icon-button danger-text" onClick={onDelete} title="删除"><Trash2 className="h-4 w-4" /></button>
      </div>
    </div>
  );
}

function MetaItem({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex gap-1">
      <span>{label}</span>
      <span>{value}</span>
    </span>
  );
}

export function createDefaultInbound(): Inbound {
  return {
    id: 0,
    remark: '',
    protocol: 'vless',
    port: 443,
    network: 'tcp',
    security: 'reality',
    enabled: true,
    uuid: randomUUID(),
    clients: [],
    reality_dest: 'www.cloudflare.com:443',
    reality_server_names: 'www.cloudflare.com',
    ss_method: '2022-blake3-aes-128-gcm',
    xhttp_mode: 'stream-one',
    hy2_up_mbps: 100,
    hy2_down_mbps: 100,
    tuic_congestion_control: 'bbr',
    shadowtls_version: 3,
  };
}

export function inboundFormValues(inbound: Inbound): InboundValues {
  const base = {
    remark: inbound.remark || '',
    protocol: inbound.protocol as InboundValues['protocol'],
    port: inbound.port || 443,
    network: inbound.network || 'tcp',
    security: inbound.security || 'none',
    uuid: String(inbound.uuid || ''),
    enabled: inbound.enabled ?? true,
  } as InboundValues;
  for (const key of advancedFields) {
    const value = inbound[key];
    (base as Record<string, unknown>)[key] = value ?? defaultAdvancedValue(key);
  }
  return base;
}

export function buildFullInboundPayload(inbound: Inbound | null, values: InboundValues): Record<string, unknown> {
  const payload: Record<string, unknown> = inbound ? { ...inbound } : {};
  delete payload.id;
  delete payload.clients;
  delete payload.traffic_up;
  delete payload.traffic_down;
  delete payload.traffic_total;
  delete payload.traffic_stats_source;
  delete payload.realtime_stats_source;
  delete payload.client_traffic;
  Object.assign(payload, values);
  for (const key of advancedFields) {
    payload[key] = normalizeAdvancedValue(key, payload[key]);
  }
  return payload;
}

function defaultAdvancedValue(key: (typeof advancedFields)[number]): string | number | boolean {
  if (numericAdvancedFields.has(key)) return 0;
  if (key === 'tuic_zero_rtt') return false;
  return '';
}

function normalizeAdvancedValue(key: (typeof advancedFields)[number], value: unknown): string | number | boolean {
  if (numericAdvancedFields.has(key)) {
    const n = Number(value ?? 0);
    return Number.isFinite(n) ? n : 0;
  }
  if (key === 'tuic_zero_rtt') return Boolean(value);
  return value == null ? '' : String(value);
}

export function hasAttachableSettingCert(cert?: CertStatus | null) {
  return Boolean(cert?.issued && cert.cert_path.trim() && cert.key_path.trim());
}

function mergeClientTraffic(inbound: Inbound, client: Client): Client {
  const live = inbound.client_traffic?.[String(client.id)] || inbound.client_traffic?.[client.id as unknown as string];
  return {
    ...client,
    up: Number(live?.up ?? client.up ?? 0),
    down: Number(live?.down ?? client.down ?? 0),
    xray_up: Number(live?.xray_up ?? client.xray_up ?? 0),
    xray_down: Number(live?.xray_down ?? client.xray_down ?? 0),
    traffic_stats_source: live?.source || client.traffic_stats_source || inbound.traffic_stats_source,
    realtime_stats_source: live?.realtime_source || client.realtime_stats_source || inbound.realtime_stats_source,
  };
}

export function mergeInboundTraffic(current: Inbound[], traffic: Inbound[]): Inbound[] {
  const byID = new Map(traffic.map((inbound) => [inbound.id, inbound]));
  return current.map((inbound) => {
    const update = byID.get(inbound.id);
    if (!update) return inbound;
    return {
      ...inbound,
      enabled: update.enabled,
      clients: mergeClients(inbound.clients || [], update.clients || []),
      traffic_up: update.traffic_up,
      traffic_down: update.traffic_down,
      traffic_total: update.traffic_total,
      traffic_stats_source: update.traffic_stats_source,
      realtime_stats_source: update.realtime_stats_source,
      client_traffic: update.client_traffic,
    };
  });
}

function mergeClients(current: Client[], traffic: Client[]): Client[] {
  const byID = new Map(traffic.map((client) => [client.id, client]));
  return current.map((client) => {
    const update = byID.get(client.id);
    if (!update) return client;
    return {
      ...client,
      enabled: update.enabled,
      up: update.up,
      down: update.down,
      traffic_limit: update.traffic_limit,
      expiry_at: update.expiry_at,
    };
  });
}

export function clientFormValues(inbound: Inbound, client?: Client): ClientValues {
  return {
    email: client?.email || '',
    uuid: client?.uuid || generatedProtocolCredential(inbound.protocol),
    enabled: client?.enabled ?? true,
    traffic_limit: client?.traffic_limit || 0,
    expiry_at: client?.expiry_at || 0,
  };
}

function sourceLabel(source: string | undefined, realtime: string | undefined, text: (value: string) => string) {
  if (realtime === 'xray') return `Xray ${text('实时')}`;
  if (source === 'unavailable') return text('不可用');
  return source || 'db';
}

export function generatedProtocolCredential(protocol?: string) {
  if (protocol === 'hysteria2' || protocol === 'tuic' || protocol === 'shadowtls') return randomUUID().replace(/-/g, '');
  return randomUUID();
}

function subscriptionURL(client: Client) {
  return `${window.location.origin}${appPath(`/sub/${subscriptionToken(client)}`)}`;
}

async function copyText(value: string, title: string, showToast: (title: string, tone?: 'success' | 'error' | 'info') => void) {
  await navigator.clipboard?.writeText(value);
  showToast(title, 'success');
}

async function copyShareLink(client: Client, showToast: (title: string, tone?: 'success' | 'error' | 'info') => void) {
  try {
    const response = await fetch(appPath(`/sub/${subscriptionToken(client)}`), { credentials: 'same-origin' });
    if (!response.ok) throw new Error('share_link_unavailable');
    const text = await response.text();
    await copyText(text.trim(), '客户端分享链接已复制', showToast);
  } catch {
    showToast('复制分享链接失败', 'error');
  }
}

function subscriptionToken(client: Client) {
  return client.subscription_token || client.uuid;
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof ApiError ? error.message : fallback;
}

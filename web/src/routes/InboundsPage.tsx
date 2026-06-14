import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Copy, Edit2, Plus, Power, RotateCcw, Trash2 } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { ApiError, appPath } from '../api/client';
import { api } from '../api/endpoints';
import type { Client, Inbound } from '../api/types';
import { EmptyState, Field, FieldError, LoadingBlock, Modal, SpinnerButton, StatusBadge, useConfirm, useToast } from '../components/ui';
import { formatBytes, randomUUID } from '../lib/format';
import { PageTitle } from './OverviewPage';

const advancedFields = [
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

const numericAdvancedFields = new Set<(typeof advancedFields)[number]>([
  'hy2_up_mbps',
  'hy2_down_mbps',
  'shadowtls_version',
]);

const inboundSchema = z.object({
  remark: z.string().min(1, '请输入名称'),
  protocol: z.enum(['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'shadowtls']),
  port: z.coerce.number().int().min(1).max(65535),
  network: z.string().min(1),
  security: z.string().min(1),
  uuid: z.string().optional(),
  enabled: z.boolean().default(true),
  ws_path: z.string().optional(),
  ws_host: z.string().optional(),
  grpc_service_name: z.string().optional(),
  reality_dest: z.string().optional(),
  reality_server_names: z.string().optional(),
  reality_short_id: z.string().optional(),
  reality_private_key: z.string().optional(),
  reality_public_key: z.string().optional(),
  ss_method: z.string().optional(),
  tls_cert_file: z.string().optional(),
  tls_key_file: z.string().optional(),
  tls_sni: z.string().optional(),
  tls_fingerprint: z.string().optional(),
  tls_alpn: z.string().optional(),
  xhttp_path: z.string().optional(),
  xhttp_mode: z.string().optional(),
  hy2_up_mbps: z.coerce.number().int().min(0).optional(),
  hy2_down_mbps: z.coerce.number().int().min(0).optional(),
  hy2_obfs: z.string().optional(),
  hy2_obfs_password: z.string().optional(),
  hy2_mport: z.string().optional(),
  tuic_congestion_control: z.string().optional(),
  tuic_zero_rtt: z.boolean().default(false),
  shadowtls_version: z.coerce.number().int().min(0).optional(),
  shadowtls_password: z.string().optional(),
});

const clientSchema = z.object({
  email: z.string().min(1, '请输入客户端标识'),
  uuid: z.string().min(1, '请输入凭据'),
  enabled: z.boolean().default(true),
  traffic_limit: z.coerce.number().min(0).optional(),
  expiry_at: z.coerce.number().min(0).optional(),
});

type InboundValues = z.infer<typeof inboundSchema>;
type ClientValues = z.infer<typeof clientSchema>;
type SortKey = 'id' | 'port' | 'protocol' | 'clients';

export default function InboundsPage() {
  const queryClient = useQueryClient();
  const confirm = useConfirm();
  const { showToast } = useToast();
  const [editingInbound, setEditingInbound] = useState<Inbound | null>(null);
  const [clientInbound, setClientInbound] = useState<Inbound | null>(null);
  const [editingClient, setEditingClient] = useState<{ inbound: Inbound; client: Client } | null>(null);
  const [search, setSearch] = useState('');
  const [sort, setSort] = useState<SortKey>('id');
  const inbounds = useQuery({ queryKey: ['inbounds'], queryFn: api.inbounds });
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
                <span>上行 {formatBytes(inbound.traffic_up)}</span>
                <span>下行 {formatBytes(inbound.traffic_down)}</span>
                <span>合计 {formatBytes(inbound.traffic_total)}</span>
                <span>统计源 {sourceLabel(inbound.traffic_stats_source, inbound.realtime_stats_source)}</span>
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
      <InboundModal inbound={editingInbound} onClose={() => setEditingInbound(null)} onSaved={refresh} />
      <ClientModal inbound={clientInbound} onClose={() => setClientInbound(null)} onSaved={refresh} />
      {editingClient ? <ClientModal inbound={editingClient.inbound} client={editingClient.client} onClose={() => setEditingClient(null)} onSaved={refresh} /> : null}
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
          <span>上行 {formatBytes(client.up)}</span>
          <span>下行 {formatBytes(client.down)}</span>
          <span>限额 {limit > 0 ? `${formatBytes(used)} / ${formatBytes(limit)}` : '不限制'}</span>
          <span>过期 {client.expiry_at ? new Date(client.expiry_at * 1000).toLocaleString() : '不限制'}</span>
          <span>实时 {sourceLabel(client.traffic_stats_source, client.realtime_stats_source)}</span>
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

function InboundModal({ inbound, onClose, onSaved }: { inbound: Inbound | null; onClose: () => void; onSaved: () => void }) {
  const { showToast } = useToast();
  const form = useForm<InboundValues>({
    resolver: zodResolver(inboundSchema),
    values: inbound ? inboundFormValues(inbound) : undefined,
  });
  const protocol = form.watch('protocol');
  const network = form.watch('network');
  const security = form.watch('security');
  const save = useMutation({
    mutationFn: (values: InboundValues) => (inbound?.id ? api.updateInbound(inbound.id, buildFullInboundPayload(inbound, values)) : api.createInbound(buildFullInboundPayload(inbound, values))),
    onSuccess: () => {
      showToast('入站已保存', 'success');
      onSaved();
      onClose();
    },
    onError: (error) => showToast(errorMessage(error, '保存入站失败'), 'error'),
  });
  return (
    <Modal
      open={!!inbound}
      title={inbound?.id ? '编辑入站' : '新增入站'}
      onClose={onClose}
      footer={
        <>
          <button className="btn secondary" onClick={onClose}>取消</button>
          <SpinnerButton className="btn primary" loading={save.isPending} onClick={form.handleSubmit((values) => save.mutate(values))}>保存</SpinnerButton>
        </>
      }
    >
      <div className="form-grid">
        <Field label="名称"><input {...form.register('remark')} /><FieldError message={form.formState.errors.remark?.message} /></Field>
        <Field label="端口"><input type="number" {...form.register('port')} /><FieldError message={form.formState.errors.port?.message} /></Field>
        <Field label="协议">
          <select {...form.register('protocol')}>
            {['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'shadowtls'].map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </Field>
        <Field label="传输">
          <select {...form.register('network')}>
            {['tcp', 'ws', 'grpc', 'h2', 'xhttp', 'quic', 'kcp'].map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </Field>
        <Field label="安全">
          <select {...form.register('security')}>
            {['none', 'tls', 'reality'].map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </Field>
        <Field label={protocol === 'shadowsocks' || protocol === 'hysteria2' || protocol === 'tuic' || protocol === 'shadowtls' ? '密码 / 密钥' : 'UUID'}>
          <input {...form.register('uuid')} />
        </Field>
        {(network === 'ws' || network === 'h2') ? (
          <>
            <Field label="WS/H2 Path"><input {...form.register('ws_path')} /></Field>
            <Field label="WS/H2 Host"><input {...form.register('ws_host')} /></Field>
          </>
        ) : null}
        {network === 'grpc' ? <Field label="gRPC Service"><input {...form.register('grpc_service_name')} /></Field> : null}
        {network === 'xhttp' ? (
          <>
            <Field label="XHTTP Path"><input {...form.register('xhttp_path')} /></Field>
            <Field label="XHTTP Mode"><input {...form.register('xhttp_mode')} placeholder="stream-one" /></Field>
          </>
        ) : null}
        {security === 'reality' ? (
          <>
            <Field label="REALITY Dest"><input {...form.register('reality_dest')} placeholder="example.com:443" /></Field>
            <Field label="REALITY Server Names"><input {...form.register('reality_server_names')} /></Field>
            <Field label="REALITY Short ID"><input {...form.register('reality_short_id')} /></Field>
            <Field label="REALITY Private Key"><input {...form.register('reality_private_key')} /></Field>
            <Field label="REALITY Public Key"><input {...form.register('reality_public_key')} readOnly /></Field>
          </>
        ) : null}
        {security === 'tls' ? (
          <>
            <Field label="TLS Cert File"><input {...form.register('tls_cert_file')} /></Field>
            <Field label="TLS Key File"><input {...form.register('tls_key_file')} /></Field>
            <Field label="TLS SNI"><input {...form.register('tls_sni')} /></Field>
            <Field label="TLS Fingerprint"><input {...form.register('tls_fingerprint')} /></Field>
            <Field label="TLS ALPN"><input {...form.register('tls_alpn')} /></Field>
          </>
        ) : null}
        {protocol === 'shadowsocks' ? <Field label="Shadowsocks Method"><input {...form.register('ss_method')} placeholder="2022-blake3-aes-128-gcm" /></Field> : null}
        {protocol === 'hysteria2' ? (
          <>
            <Field label="HY2 上行 Mbps"><input type="number" {...form.register('hy2_up_mbps')} /></Field>
            <Field label="HY2 下行 Mbps"><input type="number" {...form.register('hy2_down_mbps')} /></Field>
            <Field label="HY2 Obfs"><input {...form.register('hy2_obfs')} /></Field>
            <Field label="HY2 Obfs Password"><input {...form.register('hy2_obfs_password')} /></Field>
            <Field label="HY2 多端口"><input {...form.register('hy2_mport')} placeholder="40000-50000" /></Field>
          </>
        ) : null}
        {protocol === 'tuic' ? (
          <>
            <Field label="TUIC 拥塞控制"><input {...form.register('tuic_congestion_control')} placeholder="bbr" /></Field>
            <label className="checkbox-field"><input type="checkbox" {...form.register('tuic_zero_rtt')} /> 启用 0-RTT</label>
          </>
        ) : null}
        {protocol === 'shadowtls' ? (
          <>
            <Field label="ShadowTLS Version"><input type="number" {...form.register('shadowtls_version')} /></Field>
            <Field label="ShadowTLS Password"><input {...form.register('shadowtls_password')} /></Field>
          </>
        ) : null}
      </div>
    </Modal>
  );
}

function ClientModal({ inbound, client, onClose, onSaved }: { inbound: Inbound | null; client?: Client; onClose: () => void; onSaved: () => void }) {
  const { showToast } = useToast();
  const form = useForm<ClientValues>({
    resolver: zodResolver(clientSchema),
    values: inbound
      ? { email: client?.email || '', uuid: client?.uuid || defaultClientCredential(inbound.protocol), enabled: client?.enabled ?? true, traffic_limit: client?.traffic_limit || 0, expiry_at: client?.expiry_at || 0 }
      : undefined,
  });
  const save = useMutation({
    mutationFn: (values: ClientValues) => (client ? api.updateClient(inbound!.id, client.id, { ...client, ...values }) : api.createClient(inbound!.id, values)),
    onSuccess: () => {
      showToast('客户端已保存', 'success');
      onSaved();
      onClose();
    },
    onError: (error) => showToast(errorMessage(error, '保存客户端失败'), 'error'),
  });
  return (
    <Modal
      open={!!inbound}
      title={client ? '编辑客户端' : '新增客户端'}
      onClose={onClose}
      footer={
        <>
          <button className="btn secondary" onClick={onClose}>取消</button>
          <SpinnerButton className="btn primary" loading={save.isPending} onClick={form.handleSubmit((values) => save.mutate(values))}>保存</SpinnerButton>
        </>
      }
    >
      <div className="form-grid">
        <Field label="客户端标识"><input {...form.register('email')} /><FieldError message={form.formState.errors.email?.message} /></Field>
        <Field label="UUID / 密码 / 密钥"><input {...form.register('uuid')} /><FieldError message={form.formState.errors.uuid?.message} /></Field>
        <Field label="流量限额（字节，0 不限制）"><input type="number" {...form.register('traffic_limit')} /></Field>
        <Field label="过期时间戳（0 不限制）"><input type="number" {...form.register('expiry_at')} /></Field>
        <label className="checkbox-field"><input type="checkbox" {...form.register('enabled')} /> 已启用</label>
      </div>
    </Modal>
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

function sourceLabel(source?: string, realtime?: string) {
  if (realtime === 'xray') return 'Xray 实时';
  if (source === 'unavailable') return '不可用';
  return source || 'db';
}

function defaultClientCredential(protocol?: string) {
  if (protocol === 'hysteria2' || protocol === 'tuic' || protocol === 'shadowtls') return randomUUID().replace(/-/g, '');
  return randomUUID();
}

function subscriptionURL(client: Client) {
  return `${window.location.origin}${appPath(`/sub/${client.uuid}`)}`;
}

async function copyText(value: string, title: string, showToast: (title: string, tone?: 'success' | 'error' | 'info') => void) {
  await navigator.clipboard?.writeText(value);
  showToast(title, 'success');
}

async function copyShareLink(client: Client, showToast: (title: string, tone?: 'success' | 'error' | 'info') => void) {
  try {
    const response = await fetch(appPath(`/sub/${client.uuid}`), { credentials: 'same-origin' });
    if (!response.ok) throw new Error('share_link_unavailable');
    const text = await response.text();
    await copyText(text.trim(), '客户端分享链接已复制', showToast);
  } catch {
    showToast('复制分享链接失败', 'error');
  }
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof ApiError ? error.message : fallback;
}

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowDown, ArrowUp, Edit2, Gauge, Plus, Power, Trash2 } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { ApiError } from '../api/client';
import { api } from '../api/endpoints';
import type { Outbound, PingResult, ProxyPoolProxy } from '../api/types';
import { EmptyState, Field, FieldError, LoadingBlock, Modal, SpinnerButton, StatusBadge, toggleButtonClass, useConfirm, useToast } from '../components/ui';
import { useI18n } from '../lib/i18n';
import { PageTitle } from './OverviewPage';

const schema = z.object({
  tag: z.string().min(1, '请输入 tag'),
  remark: z.string().optional(),
  protocol: z.enum(['socks', 'http', 'freedom', 'blackhole']),
  address: z.string().optional(),
  port: z.coerce.number().min(0).max(65535).optional(),
  username: z.string().optional(),
  password: z.string().optional(),
  enabled: z.boolean().default(true),
});
type InputValues = z.input<typeof schema>;
type Values = z.output<typeof schema>;
type ProxyPoolType = 'socks5' | 'http' | 'https';

export default function OutboundsPage() {
  const queryClient = useQueryClient();
  const confirm = useConfirm();
  const { showToast } = useToast();
  const { text } = useI18n();
  const [editing, setEditing] = useState<Outbound | null>(null);
  const [poolOpen, setPoolOpen] = useState(false);
  const [latency, setLatency] = useState<Record<number, PingResult>>({});
  const [pingingIds, setPingingIds] = useState<Set<number>>(() => new Set());
  const outbounds = useQuery({ queryKey: ['outbounds'], queryFn: api.outbounds });
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['outbounds'] });
  const items = outbounds.data || [];
  const defaultItems = items.filter(isFixedDefaultOutbound);
  const customItems = items.filter((item) => !isFixedDefaultOutbound(item));
  const reorderableItems = customItems.filter(isReorderableOutbound);

  const toggle = useMutation({
    mutationFn: (item: Outbound) => api.toggleOutbound(item, !item.enabled),
    onSuccess: () => {
      showToast(text('出站状态已更新'), 'success');
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, text('出站状态更新失败')), 'error'),
  });
  const remove = useMutation({
    mutationFn: api.deleteOutbound,
    onSuccess: () => {
      showToast(text('出站已删除'), 'success');
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, text('删除出站失败')), 'error'),
  });
  const ping = useMutation({
    mutationFn: api.pingOutbound,
    onSuccess: (result, id) => setLatency((prev) => ({ ...prev, [id]: result })),
    onError: (error) => showToast(errorMessage(error, text('测速失败')), 'error'),
  });
  const speedtest = useMutation({
    mutationFn: api.speedtestAll,
    onSuccess: (result) => {
      const mapped: Record<number, PingResult> = {};
      Object.entries(result).forEach(([id, value]) => {
        mapped[Number(id)] = value;
      });
      setLatency((prev) => ({ ...prev, ...mapped }));
      showToast(text('批量测速完成'), 'success');
    },
    onError: (error) => showToast(errorMessage(error, text('批量测速失败')), 'error'),
  });
  const reorder = useMutation({
    mutationFn: (ids: number[]) => api.reorderOutbounds(ids),
    onSuccess: () => {
      showToast(text('出站顺序已保存'), 'success');
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, text('保存顺序失败')), 'error'),
  });
  const pingOutbound = async (id: number) => {
    if (pingingIds.has(id)) return;
    setPingingIds((prev) => new Set(prev).add(id));
    try {
      await ping.mutateAsync(id);
    } catch {
      // The mutation's onError handler already shows the user-facing message.
    } finally {
      setPingingIds((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  };

  if (outbounds.isLoading) return <LoadingBlock />;
  return (
    <div className="page-stack">
      <PageTitle
        title={text('出站管理')}
        description={text('配置默认直连、阻断以及 SOCKS / HTTP 代理链路。')}
        action={
          <div className="flex flex-wrap gap-2">
            <button className="btn secondary" onClick={() => setPoolOpen(true)}>{text('导入代理池')}</button>
            <SpinnerButton className="btn secondary" loading={speedtest.isPending} onClick={() => speedtest.mutate()}><Gauge className="h-4 w-4" /> {text('批量测速')}</SpinnerButton>
            <button className="btn primary" onClick={() => setEditing({ id: 0, tag: '', protocol: 'socks', enabled: true })}><Plus className="h-4 w-4" /> {text('新增出站')}</button>
          </div>
        }
      />
      {items.length === 0 ? <EmptyState title={text('暂无出站')} /> : null}
      {defaultItems.length ? <h2 className="section-title">{text('默认出站')}</h2> : null}
      <div className="grid gap-3">
        {defaultItems.map((item, index) => (
          <div key={item.id} className="resource-card">
            <div className="resource-header">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="rounded bg-panel-soft px-2 py-1 text-xs">#{index + 1}</span>
                  <h2 className="truncate text-base font-semibold">{item.tag}</h2>
                  <StatusBadge enabled={item.enabled} />
                  <span className="rounded bg-panel-soft px-2 py-1 text-xs text-panel-muted">{text('默认')}</span>
                </div>
                <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-panel-muted">
                  <span>{item.protocol}</span>
                  {item.address ? <span>{item.address}:{item.port || ''}</span> : null}
                  {item.remark ? <span>{outboundRemarkLabel(item.remark, text)}</span> : null}
                  <span>{formatLatency(latency[item.id], text)}</span>
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>
      {customItems.length ? <h2 className="section-title">{text('自定义出站')}</h2> : null}
      <div className="grid gap-3">
        {customItems.map((item, index) => (
          <div key={item.id} className="resource-card">
            <div className="resource-header">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="rounded bg-panel-soft px-2 py-1 text-xs">#{index + 1}</span>
                  <h2 className="truncate text-base font-semibold">{item.tag}</h2>
                  <StatusBadge enabled={item.enabled} />
                </div>
                <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-panel-muted">
                  <span>{item.protocol}</span>
                  {item.address ? <span>{item.address}:{item.port || ''}</span> : null}
                  {item.remark ? <span>{outboundRemarkLabel(item.remark, text)}</span> : null}
                  <span>{formatLatency(latency[item.id], text)}</span>
                </div>
              </div>
              <div className="action-row">
                <button className="icon-button" disabled={!isReorderableOutbound(item) || reorderableItems.findIndex((o) => o.id === item.id) === 0} onClick={() => moveCustomOutbound(reorderableItems, reorderableItems.findIndex((o) => o.id === item.id), -1, reorder.mutate)} title={text('上移')}><ArrowUp className="h-4 w-4" /></button>
                <button className="icon-button" disabled={!isReorderableOutbound(item) || reorderableItems.findIndex((o) => o.id === item.id) === reorderableItems.length - 1} onClick={() => moveCustomOutbound(reorderableItems, reorderableItems.findIndex((o) => o.id === item.id), 1, reorder.mutate)} title={text('下移')}><ArrowDown className="h-4 w-4" /></button>
                <SpinnerButton className="icon-button" loading={pingingIds.has(item.id)} onClick={() => pingOutbound(item.id)} title="Ping"><Gauge className="h-4 w-4" /></SpinnerButton>
                <button className={toggleButtonClass(item.enabled)} onClick={() => toggle.mutate(item)} title={text('启停')}><Power className="h-4 w-4" /></button>
                <button className="icon-button" onClick={() => setEditing(item)} title={text('编辑')}><Edit2 className="h-4 w-4" /></button>
                <button className="icon-button danger-text" onClick={async () => (await confirm({ title: text('删除出站？'), tone: 'danger' })) && remove.mutate(item.id)} title={text('删除')}><Trash2 className="h-4 w-4" /></button>
              </div>
            </div>
          </div>
        ))}
      </div>
      <OutboundModal outbound={editing} onClose={() => setEditing(null)} onSaved={refresh} />
      <ProxyPoolModal open={poolOpen} onClose={() => setPoolOpen(false)} onImported={() => { refresh(); setPoolOpen(false); }} />
    </div>
  );
}

function OutboundModal({ outbound, onClose, onSaved }: { outbound: Outbound | null; onClose: () => void; onSaved: () => void }) {
  const { showToast } = useToast();
  const { text } = useI18n();
  const form = useForm<InputValues, unknown, Values>({
    resolver: zodResolver(schema),
    values: outbound ? { tag: outbound.tag || '', remark: outbound.remark || '', protocol: (outbound.protocol || 'socks') as Values['protocol'], address: outbound.address || '', port: Number(outbound.port || 0), username: outbound.username || '', password: outbound.password || '', enabled: outbound.enabled ?? true } : undefined,
  });
  const protocol = form.watch('protocol');
  const save = useMutation({
    mutationFn: (values: Values) => {
      const payload = outbound ? { ...outbound, ...values } : values;
      return outbound?.id ? api.updateOutbound(outbound.id, payload) : api.createOutbound(payload);
    },
    onSuccess: () => {
      showToast(text('出站已保存'), 'success');
      onSaved();
      onClose();
    },
    onError: (error) => showToast(errorMessage(error, text('保存出站失败')), 'error'),
  });
  return (
    <Modal open={!!outbound} title={text(outbound?.id ? '编辑出站' : '新增出站')} onClose={onClose} footer={<><button className="btn secondary" onClick={onClose}>{text('取消')}</button><SpinnerButton className="btn primary" loading={save.isPending} onClick={form.handleSubmit((v) => save.mutate(v))}>{text('保存')}</SpinnerButton></>}>
      <div className="form-grid">
        <Field label={text('标签')}><input {...form.register('tag')} /><FieldError message={form.formState.errors.tag?.message ? text(form.formState.errors.tag.message) : undefined} /></Field>
        <Field label={text('备注')}><input {...form.register('remark')} /></Field>
        <Field label={text('协议')}><select {...form.register('protocol')}><option value="socks">SOCKS5</option><option value="http">HTTP</option><option value="freedom">freedom</option><option value="blackhole">blackhole</option></select></Field>
        {protocol === 'socks' || protocol === 'http' ? (
          <>
            <Field label={text('地址')}><input {...form.register('address')} /></Field>
            <Field label={text('端口')}><input type="number" {...form.register('port')} /></Field>
            <Field label={text('用户名')}><input {...form.register('username')} /></Field>
            <Field label={text('密码')}><input type="password" {...form.register('password')} /></Field>
          </>
        ) : null}
        <label className="checkbox-field"><input type="checkbox" {...form.register('enabled')} /> {text('已启用')}</label>
      </div>
    </Modal>
  );
}

function ProxyPoolModal({ open, onClose, onImported }: { open: boolean; onClose: () => void; onImported: () => void }) {
  const { showToast } = useToast();
  const { text } = useI18n();
  const [poolType, setPoolType] = useState<ProxyPoolType>('socks5');
  const [country, setCountry] = useState('');
  const [selected, setSelected] = useState<ProxyPoolProxy | null>(null);
  const [latency, setLatency] = useState<Record<string, PingResult>>({});
  const pool = useQuery({ queryKey: ['proxy-pool', poolType, country], queryFn: () => api.proxyPool(poolType, country), enabled: open, staleTime: 60_000 });
  const regions = useMemo(() => [...(pool.data?.regions || [])].sort((a, b) => b.count - a.count), [pool.data]);
  const ping = useMutation({
    mutationFn: (proxy: Pick<ProxyPoolProxy, 'address' | 'port'>) => api.pingProxyPool(poolType, proxy),
    onSuccess: (result, proxy) => setLatency((prev) => ({ ...prev, [proxyKey(proxy)]: result })),
  });
  const importProxy = useMutation({
    mutationFn: (proxy: ProxyPoolProxy) => api.importProxyPool(poolType, proxy),
    onSuccess: () => {
      showToast(text('代理出站已导入'), 'success');
      onImported();
    },
    onError: (error) => showToast(errorMessage(error, text('导入失败')), 'error'),
  });
  return (
    <Modal
      open={open}
      title={text('导入代理池')}
      onClose={onClose}
      panelClassName="socks5-pool-panel"
      footer={
        <>
          <button className="btn secondary" onClick={onClose}>{text('取消')}</button>
          <SpinnerButton className="btn primary" loading={importProxy.isPending} disabled={!selected} onClick={() => selected && importProxy.mutate(selected)}>{text('导入选中代理')}</SpinnerButton>
        </>
      }
    >
      <div className="socks5-pool-layout">
        <div className="grid content-start gap-3">
          <Field label={text('代理类型')}>
            <select
              value={poolType}
              onChange={(event) => {
                setPoolType(event.target.value as ProxyPoolType);
                setCountry('');
                setSelected(null);
                setLatency({});
              }}
            >
              <option value="socks5">SOCKS5</option>
              <option value="http">HTTP</option>
              <option value="https">HTTPS</option>
            </select>
          </Field>
          <Field label={text('国家/地区')}>
            <select value={country} onChange={(event) => { setCountry(event.target.value); setSelected(null); }}>
              <option value="">{text('全部地区')}</option>
              {regions.map((region) => <option key={region.code} value={region.code}>{region.name || region.code} ({region.count})</option>)}
            </select>
          </Field>
          <div className="rounded-lg bg-panel-soft p-3 text-xs leading-6 text-panel-muted">
            <div>{text('缓存')}：{pool.data?.cache_status || '-'}</div>
            <div>{text('更新')}：{pool.data?.cache_updated_at || '-'}</div>
            <div>{text('下次刷新')}：{pool.data?.next_refresh_at || '-'}</div>
          </div>
          {selected ? <div className="rounded-lg bg-panel-soft p-3 text-xs leading-6"><b>{selected.address}:{selected.port}</b><br />{selected.city || selected.country} {selected.asn} {selected.organization}</div> : null}
        </div>
        <div className="socks5-pool-list">
          {pool.isLoading ? <LoadingBlock /> : null}
          {(pool.data?.proxies || []).map((proxy) => {
            const key = proxyKey(proxy);
            return (
              <button key={key} className={`pool-row ${selected === proxy ? 'pool-row-active' : ''}`} onClick={() => setSelected(proxy)} type="button">
                <span className="pool-row-main">
                  <b className="pool-row-address">{proxy.address}:{proxy.port}</b>
                  <span className="pool-row-meta">{proxy.country || proxy.country_code} · {proxy.city || '-'} · {proxy.asn || '-'} · {proxy.organization || '-'}</span>
                </span>
                <span className="pool-row-actions">
                  <span className="pool-row-latency">{formatLatency(latency[key], text)}</span>
                  <span className="btn secondary h-8" onClick={(event) => { event.stopPropagation(); ping.mutate(proxy); }}>Ping</span>
                </span>
              </button>
            );
          })}
          {!pool.isLoading && (pool.data?.proxies || []).length === 0 ? <EmptyState title={text('暂无代理')} /> : null}
        </div>
      </div>
    </Modal>
  );
}

export function isFixedDefaultOutbound(item: Outbound) {
  return (item.tag === 'direct' && item.protocol === 'freedom') || (item.tag === 'blocked' && item.protocol === 'blackhole');
}

export function isReorderableOutbound(item: Outbound) {
  return item.protocol !== 'freedom' && item.protocol !== 'blackhole';
}

function formatLatency(result: PingResult | undefined, text: (value: string) => string) {
  if (!result) return text('未测速');
  if (result.latency < 0) return result.error || text('不可达');
  return `${Number(result.latency).toFixed(0)} ms`;
}

export function customOutboundIds(items: Outbound[]): number[] {
  return items.filter(isReorderableOutbound).map((item) => item.id);
}

export function movedCustomOutboundIds(items: Outbound[], index: number, delta: number): number[] {
  const next = [...items];
  const target = index + delta;
  if (target < 0 || target >= next.length) return customOutboundIds(next);
  [next[index], next[target]] = [next[target], next[index]];
  return customOutboundIds(next);
}

function moveCustomOutbound(items: Outbound[], index: number, delta: number, save: (ids: number[]) => void) {
  save(movedCustomOutboundIds(items, index, delta));
}

function proxyKey(proxy: Pick<ProxyPoolProxy, 'address' | 'port'>) {
  return `${proxy.address}:${proxy.port}`;
}

export function outboundRemarkLabel(remark: string, text: (value: string) => string) {
  if (remark === '直接连接' || remark === '阻断') return text(remark);
  return remark;
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof ApiError ? error.message : fallback;
}

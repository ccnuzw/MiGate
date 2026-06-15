import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowDown, ArrowUp, Check, Edit2, Plus, Power, Search, Trash2 } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { ApiError } from '../api/client';
import { api } from '../api/endpoints';
import type { Inbound, Outbound, ProxyPoolProxy, ProxyPoolResponse, RoutingRule } from '../api/types';
import { EmptyState, Field, LoadingBlock, Modal, SpinnerButton, StatusBadge, toggleButtonClass, useConfirm, useToast } from '../components/ui';
import { useI18n } from '../lib/i18n';
import { PageTitle } from './OverviewPage';

const schema = z.object({
  inbound_tag: z.string().optional(),
  domain: z.string().optional(),
  ip: z.string().optional(),
  rule_set: z.string().optional(),
  protocol: z.string().optional(),
  outbound_tag: z.string().min(1, '请选择出站'),
  enabled: z.boolean().default(true),
});
type InputValues = z.input<typeof schema>;
type Values = z.output<typeof schema>;
type ProxyPoolType = 'socks5' | 'http' | 'https';
const proxyPoolTypes: ProxyPoolType[] = ['socks5', 'http', 'https'];

export default function RoutingPage() {
  const queryClient = useQueryClient();
  const confirm = useConfirm();
  const { showToast } = useToast();
  const { text } = useI18n();
  const [editing, setEditing] = useState<RoutingRule | null>(null);
  const rules = useQuery({ queryKey: ['routing-rules'], queryFn: api.routingRules });
  const outbounds = useQuery({ queryKey: ['outbounds'], queryFn: api.outbounds });
  const inbounds = useQuery({ queryKey: ['inbounds'], queryFn: api.inbounds });
  const hasPoolOutbounds = (outbounds.data || []).some((item) => Boolean(outboundPoolType(item)));
  const poolLookups = useQueries({
    queries: proxyPoolTypes.map((type) => ({
      queryKey: ['proxy-pool', type, ''],
      queryFn: () => api.proxyPool(type),
      enabled: hasPoolOutbounds,
      staleTime: 60_000,
    })),
  });
  const proxyLookup = useMemo(() => buildProxyLookup(poolLookups.map((result) => result.data)), [poolLookups]);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['routing-rules'] });
  const remove = useMutation({
    mutationFn: api.deleteRoutingRule,
    onSuccess: () => {
      showToast(text('路由规则已删除'), 'success');
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, text('删除路由规则失败')), 'error'),
  });
  const toggle = useMutation({
    mutationFn: (item: RoutingRule) => api.updateRoutingRule(item.id, routingPayload({ ...item, enabled: !item.enabled })),
    onSuccess: () => {
      showToast(text('路由规则状态已更新'), 'success');
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, text('更新路由规则失败')), 'error'),
  });
  const reorder = useMutation({
    mutationFn: api.reorderRoutingRules,
    onSuccess: () => {
      showToast(text('路由顺序已保存'), 'success');
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, text('保存顺序失败')), 'error'),
  });
  const items = rules.data || [];
  if (rules.isLoading) return <LoadingBlock />;
  return (
    <div className="page-stack">
      <PageTitle title={text('路由规则')} description={text('按来源入站、域名、IP、规则集或协议选择出站链路。')} action={<button className="btn primary" onClick={() => setEditing({ id: 0, outbound_tag: outbounds.data?.[0]?.tag || 'direct', enabled: true })}><Plus className="h-4 w-4" /> {text('新增路由')}</button>} />
      {items.length === 0 ? <EmptyState title={text('暂无路由规则')} /> : null}
      <div className="grid gap-3">
        {items.map((item, index) => (
          <div key={item.id} className="resource-card">
            <div className="resource-header">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="rounded bg-panel-soft px-2 py-1 text-xs">#{index + 1}</span>
                  <h2 className="truncate text-base font-semibold">{ruleTitle(item, text)}</h2>
                  <StatusBadge enabled={item.enabled} />
                </div>
                <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-panel-muted">
                  <span>inbound: {item.inbound_tag || text('全部')}</span>
                  <span>domain: {item.domain || '-'}</span>
                  <span>ip: {item.ip || '-'}</span>
                  <span>rule_set: {item.rule_set || '-'}</span>
                  <span>protocol: {item.protocol || '-'}</span>
                  <span>outbound: {item.outbound_tag}</span>
                </div>
              </div>
              <div className="action-row">
                <button className="icon-button" disabled={index === 0} onClick={() => moveRule(items, index, -1, reorder.mutate)} title={text('上移')}><ArrowUp className="h-4 w-4" /></button>
                <button className="icon-button" disabled={index === items.length - 1} onClick={() => moveRule(items, index, 1, reorder.mutate)} title={text('下移')}><ArrowDown className="h-4 w-4" /></button>
                <SpinnerButton className={toggleButtonClass(item.enabled)} loading={toggle.isPending} onClick={() => toggle.mutate(item)} title={text('启停')}><Power className="h-4 w-4" /></SpinnerButton>
                <button className="icon-button" onClick={() => setEditing(item)} title={text('编辑')}><Edit2 className="h-4 w-4" /></button>
                <button className="icon-button danger-text" onClick={async () => (await confirm({ title: text('删除路由规则？'), tone: 'danger' })) && remove.mutate(item.id)} title={text('删除')}><Trash2 className="h-4 w-4" /></button>
              </div>
            </div>
          </div>
        ))}
      </div>
      <RoutingModal
        rule={editing}
        outbounds={outbounds.data || []}
        inbounds={inbounds.data || []}
        proxyLookup={proxyLookup}
        onClose={() => setEditing(null)}
        onSaved={refresh}
      />
    </div>
  );
}

function RoutingModal({ rule, outbounds, inbounds, proxyLookup, onClose, onSaved }: { rule: RoutingRule | null; outbounds: Outbound[]; inbounds: Inbound[]; proxyLookup: Map<string, ProxyPoolProxy>; onClose: () => void; onSaved: () => void }) {
  const { showToast } = useToast();
  const { text } = useI18n();
  const inboundOptions = useMemo(() => inboundSelectionOptions(inbounds), [inbounds]);
  const outboundOptions = useMemo(() => outboundSelectionOptions(outbounds, proxyLookup), [outbounds, proxyLookup]);
  const form = useForm<InputValues, unknown, Values>({
    resolver: zodResolver(schema),
    values: rule
      ? {
          inbound_tag: rule.inbound_tag || '',
          domain: rule.domain || '',
          ip: rule.ip || '',
          rule_set: rule.rule_set || '',
          protocol: rule.protocol || '',
          outbound_tag: rule.outbound_tag || outboundOptions[0]?.tag || 'direct',
          enabled: rule.enabled ?? true,
        }
      : undefined,
  });
  const save = useMutation({
    mutationFn: (values: Values) => {
      const payload = routingPayload(values);
      return rule?.id ? api.updateRoutingRule(rule.id, payload) : api.createRoutingRule(payload);
    },
    onSuccess: () => {
      showToast(text('路由规则已保存'), 'success');
      onSaved();
      onClose();
    },
    onError: (error) => showToast(errorMessage(error, text('保存路由规则失败')), 'error'),
  });
  return (
    <Modal open={!!rule} title={text(rule?.id ? '编辑路由规则' : '新增路由规则')} onClose={onClose} panelClassName="routing-modal-panel" footer={<><button className="btn secondary" onClick={onClose}>{text('取消')}</button><SpinnerButton className="btn primary" loading={save.isPending} onClick={form.handleSubmit((v) => save.mutate(v))}>{text('保存')}</SpinnerButton></>}>
      <div className="form-grid routing-form-grid">
        <InboundPicker
          options={inboundOptions}
          value={form.watch('inbound_tag') || ''}
          onChange={(value) => form.setValue('inbound_tag', value, { shouldDirty: true })}
        />
        <OutboundPicker
          options={outboundOptions}
          value={form.watch('outbound_tag') || outboundOptions[0]?.tag || 'direct'}
          onChange={(value) => form.setValue('outbound_tag', value, { shouldDirty: true })}
        />
        <Field label={text('域名匹配')} help={text('支持逗号或换行分隔多个值。')}>
          <textarea rows={3} placeholder={text('geosite:netflix 或 example.com')} {...form.register('domain')} />
        </Field>
        <Field label={text('IP 匹配')} help={text('支持 geoip:cn、CIDR、单 IP，逗号或换行分隔。')}>
          <textarea rows={3} placeholder="geoip:private, 8.8.8.8/32" {...form.register('ip')} />
        </Field>
        <Field label={text('规则集')} help={text('预留字段，当前会保存但不会写入 Xray 配置。')}>
          <textarea rows={2} placeholder="geosite-category-ads-all" {...form.register('rule_set')} />
        </Field>
        <Field label={text('协议匹配')}><input placeholder="dns, bittorrent" {...form.register('protocol')} /></Field>
        <label className="checkbox-field"><input type="checkbox" {...form.register('enabled')} /> {text('已启用')}</label>
      </div>
    </Modal>
  );
}

function InboundPicker({ options, value, onChange }: { options: InboundOption[]; value: string; onChange: (value: string) => void }) {
  const { text } = useI18n();
  const [query, setQuery] = useState('');
  const filtered = filterInboundOptions(options, query);
  return (
    <div className="choice-field routing-picker">
      <div className="choice-field-header">
        <div>
          <span className="choice-label">{text('来源入站 Tag')}</span>
          <span className="choice-help">{text('留空表示所有入站。')}</span>
        </div>
        <span className="choice-count">{text('入站')} {options.filter((item) => item.value).length}</span>
      </div>
      <div className="choice-search">
        <Search className="h-4 w-4" />
        <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={text('搜索入站、Tag、客户端')} />
      </div>
      <div className="choice-list" role="radiogroup" aria-label={text('来源入站 Tag')}>
        {filtered.map((option) => (
          <button key={option.value || 'all'} type="button" className={option.value === value ? 'choice-row choice-row-active' : 'choice-row'} onClick={() => onChange(option.value)} role="radio" aria-checked={option.value === value}>
            <span className="choice-row-main">
              <span className="choice-row-title-line">
                <span className="choice-row-title">{option.title}</span>
                <span className="choice-type-badge">{option.typeLabel}</span>
              </span>
              <span className="choice-row-meta-grid">
                {option.meta.map((item) => <span key={`${option.value}-${item.label}`}><b>{item.label}</b>{item.value}</span>)}
              </span>
              {option.clients ? <span className="choice-row-sub">{option.clients}</span> : null}
            </span>
            {option.value === value ? <Check className="h-4 w-4" /> : null}
          </button>
        ))}
      </div>
    </div>
  );
}

function OutboundPicker({ options, value, onChange }: { options: OutboundOption[]; value: string; onChange: (value: string) => void }) {
  const { text } = useI18n();
  const [query, setQuery] = useState('');
  const filtered = filterOutboundOptions(options, query);
  return (
    <div className="choice-field routing-picker">
      <div className="choice-field-header">
        <div>
          <span className="choice-label">{text('目标出站')}</span>
          <span className="choice-help">{text('按 tag、备注或协议筛选。')}</span>
        </div>
        <span className="choice-count">{text('出站')} {options.length}</span>
      </div>
      <div className="choice-search">
        <Search className="h-4 w-4" />
        <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={text('搜索出站、Tag、协议')} />
      </div>
      <div className="choice-list" role="radiogroup" aria-label={text('目标出站')}>
        {filtered.map((option) => (
          <button key={option.tag} type="button" className={option.tag === value ? 'choice-row choice-row-active' : 'choice-row'} onClick={() => onChange(option.tag)} role="radio" aria-checked={option.tag === value}>
            <span className="choice-row-main">
              <span className="choice-row-title-line">
                <span className="choice-row-title">{option.tag}</span>
              </span>
              <span className="choice-row-meta-grid">
                {option.meta.map((item) => <span key={`${option.tag}-${item.label}`}><b>{item.label}</b>{item.value}</span>)}
              </span>
            </span>
            {option.tag === value ? <Check className="h-4 w-4" /> : null}
          </button>
        ))}
      </div>
    </div>
  );
}

function moveRule(items: RoutingRule[], index: number, delta: number, save: (ids: number[]) => void) {
  save(movedRoutingRuleIds(items, index, delta));
}

export function routingPayload(values: Pick<RoutingRule, 'inbound_tag' | 'domain' | 'ip' | 'rule_set' | 'protocol' | 'outbound_tag' | 'enabled'>): Record<string, unknown> {
  return {
    inbound_tag: values.inbound_tag || '',
    domain: values.domain || '',
    ip: values.ip || '',
    rule_set: values.rule_set || '',
    protocol: values.protocol || '',
    outbound_tag: values.outbound_tag,
    enabled: values.enabled,
  };
}

export function movedRoutingRuleIds(items: RoutingRule[], index: number, delta: number): number[] {
  const next = [...items];
  const target = index + delta;
  if (target < 0 || target >= next.length) return next.map((item) => item.id);
  [next[index], next[target]] = [next[target], next[index]];
  return next.map((item) => item.id);
}

export function generatedInboundTag(inbound: Pick<Inbound, 'id' | 'protocol'>): string {
  return `inbound-${inbound.id}-${String(inbound.protocol || '').trim().toLowerCase()}`;
}

export function inboundTagOptions(inbounds: Inbound[]): string[] {
  const values = inboundSelectionOptions(inbounds).map((item) => item.value);
  return Array.from(new Set(values.filter(Boolean)));
}

type InboundOption = {
  value: string;
  title: string;
  typeLabel: string;
  meta: Array<{ label: string; value: string }>;
  clients?: string;
  search: string;
};

type OutboundOption = {
  tag: string;
  meta: Array<{ label: string; value: string }>;
  search: string;
};

export function inboundSelectionOptions(inbounds: Inbound[]): InboundOption[] {
  const options: InboundOption[] = [
    {
      value: '',
      title: '全部入站',
      typeLabel: '全部',
      meta: [{ label: '范围：', value: '不限制来源入站' }],
      search: '全部入站 all inbound',
    },
  ];
  inbounds.forEach((item) => {
    const generated = generatedInboundTag(item);
    const remark = String(item.remark || '').trim();
    const clients = (item.clients || []).map((client) => client.email).filter(Boolean);
    const title = remark || generated;
    options.push({
      value: generated,
      title: generated,
      typeLabel: remark ? '实际 Tag' : '入站 Tag',
      meta: [
        remark ? { label: '备注：', value: remark } : null,
        { label: '协议：', value: `${item.protocol || '-'} ${item.port ? `:${item.port}` : ''}`.trim() },
        { label: '传输：', value: `${item.network || 'tcp'} / ${item.security || 'none'}` },
      ].filter(Boolean) as Array<{ label: string; value: string }>,
      clients: clients.length ? `客户端：${clients.slice(0, 3).join('、')}${clients.length > 3 ? ` 等 ${clients.length} 个` : ''}` : '客户端：0',
      search: [generated, remark, item.protocol, item.port, item.network, item.security, clients.join(' ')].filter(Boolean).join(' ').toLowerCase(),
    });
    if (remark && remark !== generated) {
      options.push({
        value: remark,
        title: remark,
        typeLabel: '备注别名',
        meta: [
          { label: '实际 Tag：', value: generated },
          { label: '协议：', value: `${item.protocol || '-'} ${item.port ? `:${item.port}` : ''}`.trim() },
          { label: '传输：', value: `${item.network || 'tcp'} / ${item.security || 'none'}` },
        ],
        clients: clients.length ? `客户端：${clients.slice(0, 3).join('、')}${clients.length > 3 ? ` 等 ${clients.length} 个` : ''}` : '客户端：0',
        search: [remark, generated, item.protocol, clients.join(' ')].filter(Boolean).join(' ').toLowerCase(),
      });
    }
  });
  const seen = new Set<string>();
  return options.filter((item) => {
    if (seen.has(item.value)) return false;
    seen.add(item.value);
    return true;
  });
}

export function outboundSelectionOptions(outbounds: Outbound[], proxyLookup = new Map<string, ProxyPoolProxy>()): OutboundOption[] {
  const values = outbounds.length
    ? outbounds
    : [
        { id: 0, tag: 'direct', protocol: 'freedom', remark: '直接连接', enabled: true },
        { id: 0, tag: 'blocked', protocol: 'blackhole', remark: '阻断', enabled: true },
      ];
  const seen = new Set<string>();
  return values
    .filter((item) => {
      if (!item.tag || seen.has(item.tag)) return false;
      seen.add(item.tag);
      return true;
    })
    .map((item) => {
      const proxy = proxyLookup.get(outboundLookupKey(item));
      const country = proxyCountryLabel(proxy);
      const remark = item.remark || '';
      const meta = [
        { label: '协议：', value: item.protocol || '-' },
        item.address ? { label: '地址：', value: `${item.address}:${item.port || ''}` } : null,
        country && (!remark || !remark.includes(country)) ? { label: '国家/地区：', value: country } : null,
        remark ? { label: '备注：', value: remark } : null,
        { label: '状态：', value: item.enabled === false ? '禁用' : '启用' },
      ].filter(Boolean) as Array<{ label: string; value: string }>;
      return {
        tag: item.tag,
        meta,
        search: [item.tag, item.remark, item.protocol, item.address, item.port, country].filter(Boolean).join(' ').toLowerCase(),
      };
    });
}

function filterInboundOptions(options: InboundOption[], query: string) {
  const needle = query.trim().toLowerCase();
  if (!needle) return options;
  return options.filter((item) => item.search.includes(needle));
}

function filterOutboundOptions(options: OutboundOption[], query: string) {
  const needle = query.trim().toLowerCase();
  if (!needle) return options;
  return options.filter((item) => item.search.includes(needle));
}

function outboundPoolType(item: Pick<Outbound, 'tag'>): ProxyPoolType | '' {
  if (item.tag.startsWith('pool-https-')) return 'https';
  if (item.tag.startsWith('pool-http-')) return 'http';
  if (item.tag.startsWith('pool-socks-')) return 'socks5';
  return '';
}

function outboundLookupKey(item: Pick<Outbound, 'tag' | 'address' | 'port'>) {
  const type = outboundPoolType(item);
  if (!type || !item.address || !item.port) return '';
  return `${type}:${item.address}:${item.port}`;
}

function buildProxyLookup(responses: Array<ProxyPoolResponse | undefined>) {
  const lookup = new Map<string, ProxyPoolProxy>();
  responses.forEach((response, index) => {
    const type = proxyPoolTypes[index];
    (response?.proxies || []).forEach((proxy) => {
      lookup.set(`${type}:${proxy.address}:${proxy.port}`, proxy);
    });
  });
  return lookup;
}

function proxyCountryLabel(proxy?: Pick<ProxyPoolProxy, 'country' | 'country_code'>) {
  return String(proxy?.country || proxy?.country_code || '').trim();
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof ApiError ? error.message : fallback;
}

export function ruleTitle(rule: RoutingRule, text: (value: string) => string) {
  const match = firstRoutingMatch(rule, text);
  return `${match} -> ${rule.outbound_tag}`;
}

function firstRoutingMatch(rule: RoutingRule, text: (value: string) => string) {
  if (rule.domain) return compactRuleValue(rule.domain);
  if (rule.ip) return compactRuleValue(rule.ip);
  if (rule.rule_set) return compactRuleValue(rule.rule_set);
  if (rule.protocol) return `${text('协议')}: ${compactRuleValue(rule.protocol)}`;
  return rule.inbound_tag ? `${text('入站')}: ${rule.inbound_tag}` : text('全部入站');
}

function compactRuleValue(value: string) {
  const first = value.split(/[\n,]/).map((item) => item.trim()).find(Boolean) || value.trim();
  return first.length > 64 ? `${first.slice(0, 61)}...` : first;
}

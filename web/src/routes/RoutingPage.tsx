import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowDown, ArrowUp, Edit2, Plus, Power, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { ApiError } from '../api/client';
import { api } from '../api/endpoints';
import type { Inbound, RoutingRule } from '../api/types';
import { EmptyState, Field, LoadingBlock, Modal, SpinnerButton, StatusBadge, useConfirm, useToast } from '../components/ui';
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

export default function RoutingPage() {
  const queryClient = useQueryClient();
  const confirm = useConfirm();
  const { showToast } = useToast();
  const { text } = useI18n();
  const [editing, setEditing] = useState<RoutingRule | null>(null);
  const rules = useQuery({ queryKey: ['routing-rules'], queryFn: api.routingRules });
  const outbounds = useQuery({ queryKey: ['outbounds'], queryFn: api.outbounds });
  const inbounds = useQuery({ queryKey: ['inbounds'], queryFn: api.inbounds });
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
                <SpinnerButton className="icon-button" loading={toggle.isPending} onClick={() => toggle.mutate(item)} title={text('启停')}><Power className="h-4 w-4" /></SpinnerButton>
                <button className="icon-button" onClick={() => setEditing(item)} title={text('编辑')}><Edit2 className="h-4 w-4" /></button>
                <button className="icon-button danger-text" onClick={async () => (await confirm({ title: text('删除路由规则？'), tone: 'danger' })) && remove.mutate(item.id)} title={text('删除')}><Trash2 className="h-4 w-4" /></button>
              </div>
            </div>
          </div>
        ))}
      </div>
      <RoutingModal
        rule={editing}
        outbounds={(outbounds.data || []).map((o) => o.tag)}
        inbounds={inboundTagOptions(inbounds.data || [])}
        onClose={() => setEditing(null)}
        onSaved={refresh}
      />
    </div>
  );
}

function RoutingModal({ rule, outbounds, inbounds, onClose, onSaved }: { rule: RoutingRule | null; outbounds: string[]; inbounds: string[]; onClose: () => void; onSaved: () => void }) {
  const { showToast } = useToast();
  const { text } = useI18n();
  const form = useForm<InputValues, unknown, Values>({
    resolver: zodResolver(schema),
    values: rule
      ? {
          inbound_tag: rule.inbound_tag || '',
          domain: rule.domain || '',
          ip: rule.ip || '',
          rule_set: rule.rule_set || '',
          protocol: rule.protocol || '',
          outbound_tag: rule.outbound_tag || outbounds[0] || 'direct',
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
    <Modal open={!!rule} title={text(rule?.id ? '编辑路由规则' : '新增路由规则')} onClose={onClose} footer={<><button className="btn secondary" onClick={onClose}>{text('取消')}</button><SpinnerButton className="btn primary" loading={save.isPending} onClick={form.handleSubmit((v) => save.mutate(v))}>{text('保存')}</SpinnerButton></>}>
      <div className="form-grid">
        <Field label={text('来源入站 Tag')} help={text('留空表示所有入站。')}>
          <input list="inbound-tags" {...form.register('inbound_tag')} />
          <datalist id="inbound-tags">{inbounds.map((tag) => <option key={tag} value={tag} />)}</datalist>
        </Field>
        <Field label={text('目标出站')}>
          <select {...form.register('outbound_tag')}>{Array.from(new Set(outbounds.length ? outbounds : ['direct', 'blocked'])).map((tag) => <option key={tag} value={tag}>{tag}</option>)}</select>
        </Field>
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
  const values = inbounds.flatMap((item) => {
    const generated = generatedInboundTag(item);
    const remark = String(item.remark || '').trim();
    return remark && remark !== generated ? [generated, remark] : [generated];
  });
  return Array.from(new Set(values.filter(Boolean)));
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof ApiError ? error.message : fallback;
}

function ruleTitle(rule: RoutingRule, text: (value: string) => string) {
  return rule.domain || rule.ip || rule.rule_set || rule.protocol || text('默认匹配');
}

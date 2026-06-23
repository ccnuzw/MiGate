import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, ArrowDown, ArrowRight, ArrowUp, Check, ChevronDown, Edit2, Plus, Power, Search, Trash2, UserRound } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { getAPIErrorMessage } from '../api/client';
import { api } from '../api/endpoints';
import type { Inbound, Outbound, RoutingRule } from '../api/types';
import { EmptyState, Field, LoadingBlock, Modal, SpinnerButton, StatusBadge, toggleButtonClass, useConfirm, useToast } from '../components/ui';
import { coreLabel, inboundCore, outboundSupportedCores, type CoreName } from '../lib/cores';
import { useI18n } from '../lib/i18n';
import { refreshTopologyDependencies } from '../lib/queryInvalidation';
import { generatedInboundTag } from '../lib/routing';
import { showCoreApplyWarning } from '../lib/coreApply';
import { z } from '../lib/zod';
import { PageTitle } from './OverviewPage';

const schema = z.object({
  inbound_id: z.coerce.number().optional(),
  inbound_tag: z.string().optional(),
  client_id: z.coerce.number().optional(),
  client_email: z.string().optional(),
  domain: z.string().optional(),
  ip: z.string().optional(),
  rule_set: z.string().optional(),
  protocol: z.string().optional(),
  outbound_tag: z.string().min(1, '请选择出站'),
  outbound_id: z.coerce.number().int().positive('请选择出站'),
  enabled: z.boolean().default(true),
});
type InputValues = z.input<typeof schema>;
type Values = z.output<typeof schema>;
type ProxyPoolType = 'socks5' | 'http' | 'https';
type ProxyCountry = { country?: string; country_code?: string };
const ROUTING_PAGE_SIZES = [10, 20, 50];

export default function RoutingPage() {
  const queryClient = useQueryClient();
  const confirm = useConfirm();
  const { showToast } = useToast();
  const { text } = useI18n();
  const [editing, setEditing] = useState<RoutingRule | null>(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(ROUTING_PAGE_SIZES[0]);
  const rules = useQuery({ queryKey: ['routing-rules'], queryFn: api.routingRules });
  const outbounds = useQuery({ queryKey: ['outbounds'], queryFn: api.outbounds });
  const inbounds = useQuery({ queryKey: ['inbounds'], queryFn: api.inbounds });
  const refresh = () => refreshTopologyDependencies(queryClient);
  const remove = useMutation({
    mutationFn: api.deleteRoutingRule,
    onSuccess: (response) => {
      if (!showCoreApplyWarning(response, '规则已删除，但核心配置未生效', showToast, text)) {
        showToast(text('路由规则已删除'), 'success');
      }
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, text('删除路由规则失败')), 'error'),
  });
  const toggle = useMutation({
    mutationFn: (item: RoutingRule) => api.updateRoutingRule(item.id, routingPayload({ ...item, enabled: !item.enabled })),
    onSuccess: (response) => {
      if (!showCoreApplyWarning(response, '规则已保存，但核心配置未生效', showToast, text)) {
        showToast(text('路由规则状态已更新'), 'success');
      }
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, text('更新路由规则失败')), 'error'),
  });
  const reorder = useMutation({
    mutationFn: api.reorderRoutingRules,
    onSuccess: (response) => {
      if (!showCoreApplyWarning(response, '规则已保存，但核心配置未生效', showToast, text)) {
        showToast(text('路由顺序已保存'), 'success');
      }
      refresh();
    },
    onError: (error) => showToast(errorMessage(error, text('保存顺序失败')), 'error'),
  });
  const items = rules.data || [];
  const pageState = routingPageWindow(items, page, pageSize);
  const createDraft = newRoutingRuleDraft(outbounds.data || []);
  if (rules.isLoading) return <LoadingBlock />;
  return (
    <div className="page-stack">
      <PageTitle title={text('路由规则')} description={text('按入站、客户端、域名、IP、规则集或协议选择出站链路。')} action={<button className="btn primary" disabled={!createDraft} onClick={() => createDraft && setEditing(createDraft)} title={createDraft ? text('新增路由') : text('请先创建出站')}><Plus className="h-4 w-4" /> {text('新增路由')}</button>} />
      {items.length === 0 ? <EmptyState title={text('暂无路由规则')} /> : null}
      {items.length > 0 ? (
        <RoutingListPager
          page={pageState.page}
          pageSize={pageState.pageSize}
          total={pageState.total}
          totalPages={pageState.totalPages}
          start={pageState.start}
          end={pageState.end}
          text={text}
          onPageChange={setPage}
          onPageSizeChange={(next) => {
            setPageSize(next);
            setPage(1);
          }}
        />
      ) : null}
      <div className="routing-rule-list">
        {pageState.items.map((item, pageIndex) => {
          const index = pageState.startIndex + pageIndex;
          return (
          <RoutingRuleCard
            key={item.id}
            item={item}
            index={index}
            total={items.length}
            inbounds={inbounds.data || []}
            outbounds={outbounds.data || []}
            text={text}
            toggleLoading={toggle.isPending}
            onMove={(delta) => moveRule(items, index, delta, reorder.mutate)}
            onToggle={() => toggle.mutate(item)}
            onEdit={() => setEditing(item)}
            onDelete={async () => (await confirm({ title: text('删除路由规则？'), tone: 'danger' })) && remove.mutate(item.id)}
          />
          );
        })}
      </div>
      {items.length > pageState.pageSize ? (
        <RoutingListPager
          page={pageState.page}
          pageSize={pageState.pageSize}
          total={pageState.total}
          totalPages={pageState.totalPages}
          start={pageState.start}
          end={pageState.end}
          text={text}
          onPageChange={setPage}
          onPageSizeChange={(next) => {
            setPageSize(next);
            setPage(1);
          }}
          compact
        />
      ) : null}
      <RoutingModal
        rule={editing}
        outbounds={outbounds.data || []}
        inbounds={inbounds.data || []}
        onClose={() => setEditing(null)}
        onSaved={refresh}
      />
    </div>
  );
}

function RoutingModal({ rule, outbounds, inbounds, onClose, onSaved }: { rule: RoutingRule | null; outbounds: Outbound[]; inbounds: Inbound[]; onClose: () => void; onSaved: () => void }) {
  const { showToast } = useToast();
  const { text } = useI18n();
  const inboundOptions = useMemo(() => inboundSelectionOptions(inbounds), [inbounds]);
  const initialOutboundOptions = useMemo(() => outboundSelectionOptions(outbounds), [outbounds]);
  const form = useForm<InputValues, unknown, Values>({
    resolver: zodResolver(schema),
    values: rule
      ? {
          inbound_tag: rule.inbound_tag || '',
          inbound_id: rule.inbound_id || 0,
          client_id: rule.client_id || 0,
          client_email: rule.client_email || '',
          domain: rule.domain || '',
          ip: rule.ip || '',
          rule_set: rule.rule_set || '',
          protocol: rule.protocol || '',
          outbound_tag: rule.outbound_tag || initialOutboundOptions[0]?.tag || 'direct',
          outbound_id: rule.outbound_id || initialOutboundOptions[0]?.id || 0,
          enabled: rule.enabled ?? true,
        }
      : undefined,
  });
  const watchedInboundTag = form.watch('inbound_tag') || '';
  const watchedInboundID = Number(form.watch('inbound_id') || 0);
  const watchedClientID = Number(form.watch('client_id') || 0);
  const outboundOptions = useMemo(() => outboundSelectionOptions(outbounds, new Map(), inbounds, watchedInboundID, watchedInboundTag, watchedClientID), [outbounds, inbounds, watchedInboundID, watchedInboundTag, watchedClientID]);
  const watchedOutboundTag = form.watch('outbound_tag') || outboundOptions[0]?.tag || 'direct';
  const watchedOutboundID = Number(form.watch('outbound_id') || 0);
  const clientOptions = useMemo(() => clientSelectionOptions(inbounds, watchedInboundID, watchedInboundTag, rule || undefined), [inbounds, watchedInboundID, watchedInboundTag, rule]);
  const selectedClientOption = clientOptions.find((option) => option.id === watchedClientID);
  const draftValues = {
    inbound_id: watchedInboundID,
    inbound_tag: watchedInboundTag,
    client_id: watchedClientID,
    client_email: form.watch('client_email') || '',
    domain: form.watch('domain') || '',
    ip: form.watch('ip') || '',
    rule_set: form.watch('rule_set') || '',
    protocol: form.watch('protocol') || '',
    outbound_tag: watchedOutboundTag,
    outbound_id: watchedOutboundID,
    enabled: form.watch('enabled') ?? true,
  };
  const diagnostics = routingDiagnostics(draftValues, inbounds, outbounds);
  const summaryStatus = routeSummaryStatus(draftValues, inbounds, outbounds);
  const visibleDiagnostics = diagnostics.filter((item) => item.tone === 'error' || item.tone === 'warning');
  const save = useMutation({
    mutationFn: (values: Values) => {
      const payload = routingPayload(values);
      return rule?.id ? api.updateRoutingRule(rule.id, payload) : api.createRoutingRule(payload);
    },
    onSuccess: (response) => {
      if (!showCoreApplyWarning(response, '规则已保存，但核心配置未生效', showToast, text)) {
        showToast(text('路由规则已保存'), 'success');
      }
      onSaved();
      onClose();
    },
    onError: (error) => showToast(errorMessage(error, text('保存路由规则失败')), 'error'),
  });
  return (
    <Modal open={!!rule} title={text(rule?.id ? '编辑路由规则' : '新增路由规则')} onClose={onClose} panelClassName="routing-modal-panel" footer={<div className="routing-modal-footer"><div className="routing-modal-actions"><button className="btn secondary" onClick={onClose}>{text('取消')}</button><SpinnerButton className="btn primary" loading={save.isPending} onClick={form.handleSubmit((v) => save.mutate(v))}>{text('保存')}</SpinnerButton></div></div>}>
      <div className="routing-rule-builder">
        <RouteSummary
          rule={draftValues}
          inbounds={inbounds}
          outbounds={outbounds}
          text={text}
          status={summaryStatus}
          core={inferRuleTargetCore(draftValues, inbounds, outbounds)}
          enabled={draftValues.enabled}
        />
        <div className="routing-picker-grid">
          <InboundPicker
            options={inboundOptions}
            value={watchedInboundID}
            fallbackTag={watchedInboundTag}
            onChange={(option) => {
              form.setValue('inbound_id', option.id, { shouldDirty: true });
              form.setValue('inbound_tag', option.value, { shouldDirty: true });
              form.setValue('client_id', 0, { shouldDirty: true });
              form.setValue('client_email', '', { shouldDirty: true });
            }}
          />
          <ClientPicker
            options={clientOptions}
            value={watchedClientID}
            missingLabel={selectedClientOption?.missing ? text('客户端已缺失') : ''}
            onChange={(option) => {
              if (option.id > 0) {
                form.setValue('inbound_id', option.inboundID || 0, { shouldDirty: true });
                form.setValue('inbound_tag', option.inboundTag || '', { shouldDirty: true });
              }
              form.setValue('client_id', option.id, { shouldDirty: true });
              form.setValue('client_email', option.email, { shouldDirty: true });
            }}
          />
          <OutboundPicker
            options={outboundOptions}
            value={watchedOutboundID}
            onChange={(option) => {
              if (option.disabled) return;
              form.setValue('outbound_tag', option.tag, { shouldDirty: true });
              form.setValue('outbound_id', option.id, { shouldDirty: true });
            }}
          />
        </div>
        <section className="routing-match-panel">
          <div className="routing-match-header">
            <div>
              <div className="routing-match-title">{text('匹配条件')}</div>
              <div className="routing-match-help">{text('基础条件默认参与匹配；全部留空时匹配所选来源的全部流量。')}</div>
            </div>
            <label className="checkbox-field routing-enabled-toggle"><input type="checkbox" {...form.register('enabled')} /> {text('已启用')}</label>
          </div>
          <details className="routing-match-details">
            <summary>
              <span className="routing-match-summary-copy">
                <span className="routing-match-summary-title">{text('条件编辑')}</span>
                <span className="routing-condition-tags" aria-label={text('当前条件标签')}>
                  {conditionTags(draftValues).map((tag) => <span key={`${tag.kind}-${tag.value}`} className="routing-condition-tag"><b>{text(tag.label)}</b>{tag.translateValue ? text(tag.value) : tag.value}</span>)}
                </span>
              </span>
              <ChevronDown className="h-4 w-4" />
            </summary>
            <div className="routing-match-grid">
              <Field label={text('域名匹配')} help={text('支持逗号或换行分隔多个值。')}>
                <textarea rows={3} placeholder={text('geosite:netflix 或 example.com')} {...form.register('domain')} />
              </Field>
              <Field label={text('IP 匹配')} help={text('支持 geoip:cn、CIDR、单 IP，逗号或换行分隔。')}>
                <textarea rows={3} placeholder="geoip:private, 8.8.8.8/32" {...form.register('ip')} />
              </Field>
              <Field label={text('协议匹配')} help={text('支持按连接协议匹配。')}>
                <input placeholder="dns, bittorrent" {...form.register('protocol')} />
              </Field>
              <Field label={text('规则集')} help={text('会保存到规则；部分核心配置生成暂不完整写入 rule_set，请应用前核对生成配置。')}>
                <input placeholder="geosite-category-ads-all" {...form.register('rule_set')} />
              </Field>
            </div>
          </details>
          {visibleDiagnostics.length ? <RoutingDiagnosticsPanel items={visibleDiagnostics} text={text} /> : null}
        </section>
      </div>
    </Modal>
  );
}

function RoutingRuleCard({ item, index, total, inbounds, outbounds, text, toggleLoading, onMove, onToggle, onEdit, onDelete }: { item: RoutingRule; index: number; total: number; inbounds: Inbound[]; outbounds: Outbound[]; text: (value: string) => string; toggleLoading: boolean; onMove: (delta: number) => void; onToggle: () => void; onEdit: () => void; onDelete: () => void }) {
  const tags = conditionTags(item);
  const technical = ruleTechnicalMeta(item, inbounds, outbounds, text);
  const missingClient = Number(item.client_id || 0) > 0 && !findClientById(inbounds, Number(item.client_id || 0));
  return (
    <article className="resource-card routing-policy-card">
      <div className="routing-policy-main">
        <div className="routing-policy-index">#{index + 1}</div>
        <div className="routing-policy-content">
          <div className="routing-policy-kicker">
            <span>{text(ruleTypeLabel(item))}</span>
            <StatusBadge enabled={item.enabled} />
            {missingClient ? <StatusBadge enabled={false}>{text('客户端已缺失')}</StatusBadge> : null}
          </div>
          <h2 className="routing-policy-title">{ruleTitle(item, text, inbounds, outbounds)}</h2>
          <div className="routing-condition-tags">
            {tags.map((tag) => <span key={`${tag.kind}-${tag.value}`} className="routing-condition-tag"><b>{text(tag.label)}</b>{tag.translateValue ? text(tag.value) : tag.value}</span>)}
          </div>
          <details className="routing-policy-details">
            <summary><span>{text('更多信息')}</span><ChevronDown className="h-4 w-4" /></summary>
            <div className="routing-policy-meta">
              {technical.map((meta) => <span key={meta.label}><b>{text(meta.label)}</b>{meta.value || '-'}</span>)}
            </div>
          </details>
        </div>
        <div className="action-row routing-policy-actions">
          <button className="icon-button" disabled={index === 0} onClick={() => onMove(-1)} title={text('上移')}><ArrowUp className="h-4 w-4" /></button>
          <button className="icon-button" disabled={index === total - 1} onClick={() => onMove(1)} title={text('下移')}><ArrowDown className="h-4 w-4" /></button>
          <SpinnerButton className={toggleButtonClass(item.enabled)} loading={toggleLoading} onClick={onToggle} title={text('启停')}><Power className="h-4 w-4" /></SpinnerButton>
          <button className="icon-button" onClick={onEdit} title={text('编辑')}><Edit2 className="h-4 w-4" /></button>
          <button className="icon-button danger-text" onClick={onDelete} title={text('删除')}><Trash2 className="h-4 w-4" /></button>
        </div>
      </div>
    </article>
  );
}

function RoutingListPager({ page, pageSize, total, totalPages, start, end, text, onPageChange, onPageSizeChange, compact = false }: { page: number; pageSize: number; total: number; totalPages: number; start: number; end: number; text: (value: string) => string; onPageChange: (page: number) => void; onPageSizeChange: (pageSize: number) => void; compact?: boolean }) {
  return (
    <div className={compact ? 'routing-list-pager routing-list-pager-compact' : 'routing-list-pager'}>
      <div className="routing-list-pager-copy">
        <strong>{text('规则列表')}</strong>
        <span>{text('共')} {total} {text('条')}，{text('当前')} {start}-{end}</span>
      </div>
      <div className="routing-list-pager-controls">
        {!compact ? (
          <label>
            <span>{text('每页')}</span>
            <select value={pageSize} onChange={(event) => onPageSizeChange(Number(event.target.value))}>
              {ROUTING_PAGE_SIZES.map((size) => <option key={size} value={size}>{size}</option>)}
            </select>
          </label>
        ) : null}
        <button className="btn secondary" type="button" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>{text('上一页')}</button>
        <span className="routing-list-page-index">{page} / {totalPages}</span>
        <button className="btn secondary" type="button" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>{text('下一页')}</button>
      </div>
    </div>
  );
}

function RouteSummary({ rule, inbounds, outbounds, text, status, core, enabled }: { rule: RoutingDraftValues; inbounds: Inbound[]; outbounds: Outbound[]; text: (value: string) => string; status: RouteSummaryStatus; core: string; enabled: boolean }) {
  const inbound = readableInboundName(rule, inbounds, text) || text('全部入站');
  const client = readableClientName(rule, inbounds, text);
  const clientHint = clientRouteHint(rule, inbounds);
  const outbound = readableOutboundName(rule, outbounds) || rule.outbound_tag || text('未选择出站');
  const condition = summaryCondition(rule, text);
  return (
    <div className="routing-summary-bar">
      <div className="routing-path-step">
        <ArrowRight className="h-4 w-4" />
        <span className="routing-path-label">{text('来源')}</span>
        <strong>{inbound}</strong>
      </div>
      <div className="routing-path-arrow">→</div>
      <div className={Number(rule.client_id || 0) > 0 ? 'routing-path-step' : 'routing-path-step routing-path-muted'}>
        <UserRound className="h-4 w-4" />
        <span className="routing-path-label">{text('客户端')}</span>
        <strong>{Number(rule.client_id || 0) > 0 ? client : text('不指定客户端')}</strong>
        {clientHint ? <span className="routing-path-hint">{text('匹配值')}：{clientHint}</span> : null}
      </div>
      <div className="routing-path-arrow">→</div>
      <div className="routing-path-step">
        <ArrowRight className="h-4 w-4" />
        <span className="routing-path-label">{text('出站')}</span>
        <strong>{outbound}</strong>
      </div>
      <div className="routing-summary-side">
        <span className="routing-summary-condition">{condition}</span>
        <div className="routing-summary-badges">
          <span className="choice-type-badge">{text(status.ruleType)}</span>
          <span className={`routing-save-badge routing-save-${status.tone}`}>{text(status.label)}</span>
          <span className="choice-type-badge">{text(core)}</span>
          <StatusBadge enabled={enabled}>{text(enabled ? '启用' : '禁用')}</StatusBadge>
        </div>
      </div>
    </div>
  );
}

function RoutingDiagnosticsPanel({ items, text }: { items: RouteDiagnostic[]; text: (value: string) => string }) {
  return (
    <div className="routing-diagnostics" aria-label={text('即时诊断')}>
      {items.map((item) => (
        <div key={`${item.tone}-${item.message}`} className={`routing-diagnostic routing-diagnostic-${item.tone}`}>
          {item.tone === 'ok' ? <Check className="h-4 w-4" /> : <AlertTriangle className="h-4 w-4" />}
          <span>{text(item.message)}</span>
        </div>
      ))}
    </div>
  );
}

function InboundPicker({ options, value, fallbackTag, onChange }: { options: InboundOption[]; value: number; fallbackTag: string; onChange: (option: InboundOption) => void }) {
  const { text } = useI18n();
  const [query, setQuery] = useState('');
  const filtered = filterInboundOptions(options, query);
  return (
    <div className="choice-field routing-picker">
      <div className="choice-field-header">
        <div>
          <span className="choice-label">{text('来源入站')}</span>
          <span className="choice-help">{text('留空表示所有入站。')}</span>
        </div>
        <span className="choice-count">{text('入站')} {options.filter((item) => item.value).length}</span>
      </div>
      <div className="choice-search">
        <Search className="h-4 w-4" />
        <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={text('搜索入站、名称、备注')} />
      </div>
      <div className="choice-list" role="radiogroup" aria-label={text('来源入站')}>
        {filtered.map((option) => (
          <button key={option.value || 'all'} type="button" className={inboundOptionMatches(option, value, fallbackTag) ? 'choice-row choice-row-active inbound-choice-row' : 'choice-row inbound-choice-row'} onClick={() => onChange(option)} role="radio" aria-checked={inboundOptionMatches(option, value, fallbackTag)}>
            <span className="choice-row-main">
              {option.subtitle ? <span className="choice-row-kicker">{option.subtitle}</span> : null}
              <span className="choice-row-title-line">
                <span className="choice-row-title">{option.title}</span>
                <span className="choice-type-badge">{text(option.typeLabel)}</span>
              </span>
              <span className="choice-row-meta-grid">
                {option.meta.map((item) => <ChoiceMetaItem key={`${option.value}-${item.label}`} item={item} text={text} />)}
              </span>
            </span>
            {inboundOptionMatches(option, value, fallbackTag) ? <Check className="h-4 w-4" /> : null}
          </button>
        ))}
      </div>
    </div>
  );
}

function ClientPicker({ options, value, missingLabel, onChange }: { options: ClientOption[]; value: number; missingLabel: string; onChange: (option: ClientOption) => void }) {
  const { text } = useI18n();
  const [query, setQuery] = useState('');
  const filtered = filterClientOptions(options, query);
  return (
    <div className="choice-field routing-picker">
      <div className="choice-field-header">
        <div>
          <span className="choice-label">{text('客户端')}</span>
          <span className="choice-help">{text('选择后生成客户端级规则：入站 / 客户端 -> 出站。')}</span>
        </div>
        <span className="choice-count">{missingLabel || `${text('客户端')} ${Math.max(options.length - 1, 0)}`}</span>
      </div>
      <div className="choice-search">
        <Search className="h-4 w-4" />
        <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={text('搜索客户端、邮箱、用户名、stats_key 或 UUID')} />
      </div>
      <div className="choice-list" role="radiogroup" aria-label={text('客户端')}>
        {filtered.map((option) => (
          <button key={option.id || 'inbound-level'} type="button" className={option.id === value ? 'choice-row choice-row-active' : 'choice-row'} onClick={() => onChange(option)} role="radio" aria-checked={option.id === value}>
            <span className="choice-row-main">
              <span className="choice-row-title-line">
                <span className="choice-row-title">{option.title}</span>
                <span className="choice-type-badge">{text(option.typeLabel)}</span>
              </span>
              {option.subtitle ? <span className="choice-row-sub">{option.subtitle}</span> : null}
              <span className="choice-row-meta-grid">
                {option.meta.map((item) => <ChoiceMetaItem key={`${option.id}-${item.label}`} item={item} text={text} />)}
              </span>
            </span>
            {option.id === value ? <Check className="h-4 w-4" /> : null}
          </button>
        ))}
      </div>
    </div>
  );
}

function OutboundPicker({ options, value, onChange }: { options: OutboundOption[]; value: number; onChange: (option: OutboundOption) => void }) {
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
          <button key={`${option.id}-${option.tag}`} type="button" disabled={option.disabled} className={`${option.id === value ? 'choice-row choice-row-active outbound-choice-row' : 'choice-row outbound-choice-row'}${option.disabled ? ' opacity-50' : ''}`} onClick={() => onChange(option)} role="radio" aria-checked={option.id === value}>
            <span className="choice-row-main">
              <span className="choice-row-title-line">
                <span className="choice-row-title">{option.title}</span>
                <span className={`protocol-badge choice-protocol-badge outbound-protocol-${option.protocolType || 'default'}`}>{option.protocolLabel}</span>
                {option.cores.map((core) => <span key={core} className="choice-type-badge">{coreLabel(core)}</span>)}
              </span>
              {option.subtitle ? <span className="choice-row-sub">{option.subtitle}</span> : null}
              <span className="choice-row-meta-grid">
                {option.meta.map((item) => <ChoiceMetaItem key={`${option.tag}-${item.label}`} item={item} text={text} />)}
              </span>
              {option.disabledReason ? <span className="choice-disabled-reason"><AlertTriangle className="h-4 w-4" /> {text(option.disabledReason)}</span> : null}
            </span>
            {option.id === value ? <Check className="h-4 w-4" /> : null}
          </button>
        ))}
      </div>
    </div>
  );
}

function moveRule(items: RoutingRule[], index: number, delta: number, save: (ids: number[]) => void) {
  save(movedRoutingRuleIds(items, index, delta));
}

export function newRoutingRuleDraft(outbounds: Outbound[]): RoutingRule | null {
  const outbound = outbounds.find((item) => Number(item.id || 0) > 0);
  if (!outbound) return null;
  return { id: 0, outbound_id: outbound.id, outbound_tag: outbound.tag, enabled: true };
}

export function routingPayload(values: Pick<RoutingRule, 'inbound_id' | 'inbound_tag' | 'client_id' | 'client_email' | 'domain' | 'ip' | 'rule_set' | 'protocol' | 'outbound_tag' | 'outbound_id' | 'enabled'>): Record<string, unknown> {
  return {
    inbound_id: Number(values.inbound_id || 0),
    inbound_tag: values.inbound_tag || '',
    client_id: Number(values.client_id || 0),
    client_email: values.client_email || '',
    domain: values.domain || '',
    ip: values.ip || '',
    rule_set: values.rule_set || '',
    protocol: values.protocol || '',
    outbound_id: Number(values.outbound_id || 0),
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

export function routingPageWindow<T>(items: T[], page: number, pageSize: number) {
  const safePageSize = ROUTING_PAGE_SIZES.includes(pageSize) ? pageSize : ROUTING_PAGE_SIZES[0];
  const total = items.length;
  const totalPages = Math.max(1, Math.ceil(total / safePageSize));
  const safePage = Math.min(Math.max(1, Math.floor(Number(page) || 1)), totalPages);
  const startIndex = total ? (safePage - 1) * safePageSize : 0;
  const endIndex = Math.min(startIndex + safePageSize, total);
  return {
    items: items.slice(startIndex, endIndex),
    page: safePage,
    pageSize: safePageSize,
    total,
    totalPages,
    startIndex,
    start: total ? startIndex + 1 : 0,
    end: endIndex,
  };
}

export { generatedInboundTag };

type RoutingDraftValues = Pick<RoutingRule, 'inbound_id' | 'inbound_tag' | 'client_id' | 'client_email' | 'domain' | 'ip' | 'rule_set' | 'protocol' | 'outbound_tag' | 'outbound_id' | 'enabled'>;
type ConditionTag = { kind: 'domain' | 'ip' | 'protocol' | 'rule_set' | 'catch_all'; label: string; value: string; translateValue?: boolean };
type RouteDiagnostic = { tone: 'ok' | 'info' | 'warning' | 'error'; message: string };
type RouteSummaryStatus = { ruleType: string; label: string; tone: 'ok' | 'warning' | 'error' };

export function conditionTags(rule: Pick<RoutingRule, 'domain' | 'ip' | 'protocol' | 'rule_set'>): ConditionTag[] {
  const tags: ConditionTag[] = [];
  if (hasRuleValue(rule.domain)) tags.push({ kind: 'domain', label: 'domain', value: compactRuleValue(String(rule.domain)) });
  if (hasRuleValue(rule.ip)) tags.push({ kind: 'ip', label: 'ip', value: compactRuleValue(String(rule.ip)) });
  if (hasRuleValue(rule.protocol)) tags.push({ kind: 'protocol', label: 'protocol', value: compactRuleValue(String(rule.protocol)) });
  if (hasRuleValue(rule.rule_set)) tags.push({ kind: 'rule_set', label: 'rule_set', value: compactRuleValue(String(rule.rule_set)) });
  return tags.length ? tags : [{ kind: 'catch_all', label: 'match', value: '全部流量', translateValue: true }];
}

export function routeSummaryText(rule: RoutingDraftValues, inbounds: Inbound[], outbounds: Outbound[], text: (value: string) => string) {
  const inbound = readableInboundName(rule, inbounds, text) || text('全部入站');
  const outbound = readableOutboundName(rule, outbounds) || rule.outbound_tag || text('未选择出站');
  const condition = summaryCondition(rule, text);
  if (Number(rule.client_id || 0) > 0) {
    return `${text('来自')} ${inbound} ${text('的客户端')} ${readableClientName(rule, inbounds, text)}，${condition}，${text('走')} ${outbound}`;
  }
  return `${text('来自')} ${inbound} ${condition}，${text('走')} ${outbound}`;
}

export function routeSummaryStatus(rule: RoutingDraftValues, inbounds: Inbound[], outbounds: Outbound[]): RouteSummaryStatus {
  const diagnostics = routingDiagnostics(rule, inbounds, outbounds);
  const hasError = diagnostics.some((item) => item.tone === 'error');
  const hasWarning = diagnostics.some((item) => item.tone === 'warning');
  return {
    ruleType: ruleTypeLabel(rule),
    label: hasError ? '不完整' : hasWarning ? '有风险' : '可保存',
    tone: hasError ? 'error' : hasWarning ? 'warning' : 'ok',
  };
}

export function inferRuleTargetCore(rule: Pick<RoutingRule, 'inbound_id' | 'inbound_tag' | 'client_id' | 'outbound_id' | 'outbound_tag'>, inbounds: Inbound[], outbounds: Outbound[]): string {
  const required = requiredRouteCores(inbounds, Number(rule.inbound_id || 0), rule.inbound_tag || '', Number(rule.client_id || 0));
  if (required.length > 1) return 'mixed';
  if (required.length === 1) return coreLabel(required[0]);
  const outbound = findOutboundForValues(outbounds, rule);
  const supported = outbound ? outboundSupportedCores(outbound) : [];
  if (supported.length > 1) return 'mixed';
  if (supported.length === 1) return coreLabel(supported[0]);
  return 'unknown';
}

export function outboundDisabledReason(outbound: Pick<Outbound, 'protocol' | 'enabled'> | undefined, requiredCores: CoreName[]): string {
  if (!outbound) return '';
  if (!requiredCores.length) return '';
  const supported = outboundSupportedCores(outbound);
  const missing = requiredCores.filter((core) => !supported.includes(core));
  if (!missing.length) return '';
  const supportsOnlySingbox = supported.length === 1 && supported[0] === 'sing-box';
  const supportsOnlyXray = supported.length === 1 && supported[0] === 'xray';
  if (requiredCores.length > 1) return '当前来源包含多个核心，目标出站不支持全部核心';
  if (supportsOnlySingbox) return '仅支持 sing-box，当前来源属于 Xray';
  if (supportsOnlyXray) return '仅支持 Xray，当前来源属于 sing-box';
  return '当前来源内核不支持';
}

function outboundSelectionWarning(outbound: Pick<Outbound, 'enabled'> | undefined): string {
  return outbound?.enabled === false ? '出站已禁用' : '';
}

export function routingDiagnostics(rule: RoutingDraftValues, inbounds: Inbound[], outbounds: Outbound[]): RouteDiagnostic[] {
  const diagnostics: RouteDiagnostic[] = [];
  const outboundID = Number(rule.outbound_id || 0);
  const outbound = findOutboundForValues(outbounds, rule);
  if (outboundID <= 0) diagnostics.push({ tone: 'error', message: '未选择目标出站，规则不完整。' });
  const required = requiredRouteCores(inbounds, Number(rule.inbound_id || 0), rule.inbound_tag || '', Number(rule.client_id || 0));
  const reason = outboundDisabledReason(outbound, required);
  if (reason) diagnostics.push({ tone: 'error', message: reason });
  if (outboundSelectionWarning(outbound)) diagnostics.push({ tone: 'warning', message: '出站已禁用，保存后不会生成可用链路。' });
  if (Number(rule.client_id || 0) > 0 && !findClientById(inbounds, Number(rule.client_id || 0))) {
    diagnostics.push({ tone: 'warning', message: '客户端已缺失，核心配置生成时会跳过。' });
  }
  if (!hasMatchConditions(rule)) diagnostics.push({ tone: 'info', message: '未设置任何匹配条件，将匹配所选来源的全部流量。' });
  if (!diagnostics.some((item) => item.tone === 'error' || item.tone === 'warning')) {
    diagnostics.unshift({ tone: 'ok', message: '当前规则可保存。' });
  }
  return diagnostics;
}

export function inboundTagOptions(inbounds: Inbound[]): string[] {
  const values = inboundSelectionOptions(inbounds).map((item) => item.value);
  return Array.from(new Set(values.filter(Boolean)));
}

type InboundOption = {
  id: number;
  value: string;
  aliases?: string[];
  title: string;
  subtitle?: string;
  typeLabel: string;
  meta: ChoiceMeta[];
  search: string;
};

type ClientOption = {
  id: number;
  email: string;
  title: string;
  subtitle?: string;
  typeLabel: string;
  missing?: boolean;
  inboundID?: number;
  inboundTag?: string;
  meta: ChoiceMeta[];
  search: string;
};

type OutboundOption = {
  id: number;
  tag: string;
  title: string;
  subtitle?: string;
  protocolType: string;
  protocolLabel: string;
  remark: string;
  meta: ChoiceMeta[];
  cores: ReturnType<typeof outboundSupportedCores>;
  disabled?: boolean;
  disabledReason?: string;
  search: string;
};

type ChoiceMeta = {
  label: string;
  value: string;
  translateValue?: boolean;
};

function isChoiceMeta(value: ChoiceMeta | null): value is ChoiceMeta {
  return value !== null;
}

function formatChoiceMetaValue(item: ChoiceMeta, text: (value: string) => string) {
  return item.translateValue ? text(item.value) : item.value;
}

function ChoiceMetaItem({ item, text }: { item: ChoiceMeta; text: (value: string) => string }) {
  return (
    <span>
      <b>{text(item.label)}</b>
      <span className="choice-meta-value">{formatChoiceMetaValue(item, text)}</span>
    </span>
  );
}

export function inboundSelectionOptions(inbounds: Inbound[]): InboundOption[] {
  const options: InboundOption[] = [
    {
      id: 0,
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
    const clientCount = (item.clients || []).length;
    options.push({
      id: item.id,
      value: generated,
      aliases: remark && remark !== generated ? [remark] : undefined,
      title: remark || generated,
      subtitle: remark ? generated : undefined,
      typeLabel: '入站',
      meta: [
        { label: '协议：', value: `${item.protocol || '-'} ${item.port ? `:${item.port}` : ''}`.trim() },
        { label: '内核：', value: coreLabel(inboundCore(item)) },
        { label: '传输：', value: `${item.network || 'tcp'} / ${item.security || 'none'}` },
        { label: '客户端：', value: String(clientCount) },
      ].filter(isChoiceMeta),
      search: [generated, remark, item.protocol, inboundCore(item), item.port, item.network, item.security].filter(Boolean).join(' ').toLowerCase(),
    });
  });
  const seen = new Set<string>();
  return options.filter((item) => {
    if (seen.has(item.value)) return false;
    seen.add(item.value);
    return true;
  });
}

function inboundOptionMatches(option: InboundOption, inboundID: number, fallbackTag = '') {
  if (inboundID > 0) return option.id === inboundID;
  return option.value === fallbackTag || Boolean(fallbackTag && option.aliases?.includes(fallbackTag));
}

function clientCredentialIDValue(client: { credential_id?: string; uuid?: string }) {
  const credentialID = String(client.credential_id || '').trim();
  if (credentialID) return credentialID;
  return String(client.uuid || '').trim();
}

function clientStatsNameValue(client: { stats_key?: string; email?: string }) {
  const statsKey = String(client.stats_key || '').trim();
  if (statsKey) return statsKey;
  return String(client.email || '').trim();
}

export function clientRouteMatchIdentity(protocol: string, client: Pick<NonNullable<Inbound['clients']>[number], 'credential_id' | 'uuid' | 'stats_key' | 'email'>) {
  switch (String(protocol || '').trim().toLowerCase()) {
    case 'socks':
    case 'http':
      return clientCredentialIDValue(client);
    default:
      return clientStatsNameValue(client);
  }
}

export function clientSelectionOptions(inbounds: Inbound[], inboundID: number, inboundTag = '', rule?: Pick<RoutingRule, 'client_id' | 'client_email' | 'inbound_id' | 'inbound_tag'>): ClientOption[] {
  const lookup = buildInboundLookup(inbounds);
  const selectedInbound = inboundID > 0 ? inbounds.find((inbound) => inbound.id === inboundID) : inboundTag ? lookup.get(inboundTag) : undefined;
  const sourceInbounds = selectedInbound ? [selectedInbound] : inbounds;
  const options: ClientOption[] = [
    {
      id: 0,
      email: '',
      title: '不指定客户端',
      typeLabel: '入站级',
      meta: [{ label: '规则：', value: '入站 -> 出站', translateValue: true }],
      search: '不指定客户端 inbound level all',
    },
  ];
  sourceInbounds.forEach((inbound) => {
    const inboundTagValue = generatedInboundTag(inbound);
    const inboundName = inbound.remark || inboundTagValue;
    (inbound.clients || []).forEach((client) => {
      const email = String(client.email || '').trim();
      const credentialID = clientCredentialIDValue(client);
      const statsName = clientStatsNameValue(client);
      const routeIdentity = clientRouteMatchIdentity(inbound.protocol, client);
      const title = clientDisplayName(client, `client-${client.id}`);
      const subtitle = routeIdentity && routeIdentity !== title ? `匹配值：${routeIdentity}` : undefined;
      options.push({
        id: client.id,
        email,
        title,
        subtitle,
        typeLabel: '客户端级',
        inboundID: inbound.id,
        inboundTag: inboundTagValue,
        meta: [
          { label: '入站：', value: inboundName },
          { label: '状态：', value: client.enabled === false ? '禁用' : '启用', translateValue: true },
        ].filter(isChoiceMeta),
        search: [routeIdentity, email, credentialID, statsName, client.uuid, inbound.remark, inboundTagValue].filter(Boolean).join(' ').toLowerCase(),
      });
    });
  });
  const clientID = Number(rule?.client_id || 0);
  if (clientID > 0 && !options.some((option) => option.id === clientID)) {
    const email = String(rule?.client_email || '').trim();
    options.push({
      id: clientID,
      email,
      title: email || `client-${clientID}`,
      typeLabel: '客户端已缺失',
      missing: true,
      inboundID: Number(rule?.inbound_id || inboundID || 0),
      inboundTag,
      meta: [
        { label: '状态：', value: '客户端已缺失', translateValue: true },
        { label: '规则：', value: '核心配置生成时跳过', translateValue: true },
      ],
      search: [email, clientID, 'missing deleted'].join(' ').toLowerCase(),
    });
  }
  return options;
}

export function outboundSelectionOptions(outbounds: Outbound[], proxyLookup = new Map<string, ProxyCountry>(), inbounds: Inbound[] = [], inboundID = 0, inboundTag = '', clientID = 0): OutboundOption[] {
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
      const rawProtocolType = outboundPoolType(item) || item.protocol || 'default';
      const protocolType = protocolSlug(rawProtocolType);
      const cores = outboundSupportedCores(item);
      const requiredCores = requiredRouteCores(inbounds, inboundID, inboundTag, clientID);
      const disabledReason = outboundDisabledReason(item, requiredCores);
      const warningReason = outboundSelectionWarning(item);
      const disabled = Boolean(disabledReason);
      const meta = [
        item.address ? { label: '地址：', value: `${item.address}:${item.port || ''}` } : null,
        { label: '内核：', value: cores.map(coreLabel).join(' / ') || '-' },
        country && (!remark || !remark.includes(country)) ? { label: '国家/地区：', value: country } : null,
        item.enabled === false ? { label: '状态：', value: '禁用', translateValue: true } : null,
      ].filter(isChoiceMeta);
      return {
        id: item.id,
        tag: item.tag,
        title: String(item.remark || item.tag).trim() || item.tag,
        subtitle: item.remark && item.remark !== item.tag ? item.tag : undefined,
        protocolType,
        protocolLabel: protocolLabel(rawProtocolType),
        remark,
        meta,
        cores,
        disabled,
        disabledReason: disabledReason || warningReason,
        search: [item.tag, item.remark, item.protocol, rawProtocolType, protocolType, protocolLabel(rawProtocolType), cores.join(' '), item.address, item.port, country].filter(Boolean).join(' ').toLowerCase(),
      };
    });
}

function requiredRouteCores(inbounds: Inbound[], inboundID: number, inboundTag: string, clientID: number) {
  if (clientID > 0) {
    const found = findClientById(inbounds, clientID);
    return found ? [inboundCore(found.inbound)] : [];
  }
  const lookup = buildInboundLookup(inbounds);
  const selected = inboundID > 0 ? inbounds.find((inbound) => inbound.id === inboundID) : inboundTag ? lookup.get(inboundTag) : undefined;
  const sourceInbounds = selected ? [selected] : inboundTag ? [] : inbounds;
  return Array.from(new Set(sourceInbounds.map(inboundCore)));
}

function filterInboundOptions(options: InboundOption[], query: string) {
  const needle = query.trim().toLowerCase();
  if (!needle) return options;
  return options.filter((item) => item.search.includes(needle));
}

function filterClientOptions(options: ClientOption[], query: string) {
  const needle = query.trim().toLowerCase();
  if (!needle) return options;
  return options.filter((item) => item.search.includes(needle));
}

function filterOutboundOptions(options: OutboundOption[], query: string) {
  const needle = query.trim().toLowerCase();
  if (!needle) return options;
  return options.filter((item) => item.search.includes(needle));
}

function buildInboundLookup(inbounds: Inbound[]) {
  const lookup = new Map<string, Inbound>();
  inbounds.forEach((inbound) => {
    const generated = generatedInboundTag(inbound);
    const remark = String(inbound.remark || '').trim();
    lookup.set(generated, inbound);
    if (remark) lookup.set(remark, inbound);
  });
  return lookup;
}

function findClientById(inbounds: Inbound[], clientID: number) {
  for (const inbound of inbounds) {
    const found = (inbound.clients || []).find((client) => client.id === clientID);
    if (found) return { inbound, client: found };
  }
  return undefined;
}

function readableClientRouteIdentity(found: { inbound: Inbound; client: NonNullable<Inbound['clients']>[number] }) {
  return clientRouteMatchIdentity(found.inbound.protocol, found.client);
}

function clientDisplayName(client: Pick<NonNullable<Inbound['clients']>[number], 'email' | 'credential_id' | 'uuid'>, fallback: string) {
  const email = String(client.email || '').trim();
  if (email) return email;
  const credentialID = String(client.credential_id || '').trim();
  if (credentialID) return credentialID;
  return String(client.uuid || fallback).trim() || fallback;
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

function proxyCountryLabel(proxy?: ProxyCountry) {
  return String(proxy?.country || proxy?.country_code || '').trim();
}

function protocolLabel(protocol: string) {
  const value = protocol || 'default';
  const normalized = protocolSlug(value);
  if (normalized === 'socks5') return 'SOCKS5';
  if (normalized === 'default') return 'OUT';
  return value.toUpperCase();
}

function protocolSlug(protocol: string) {
  return String(protocol || 'default').trim().toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '') || 'default';
}

function errorMessage(error: unknown, fallback: string) {
  return getAPIErrorMessage(error, fallback);
}

export function ruleTitle(rule: RoutingRule, text: (value: string) => string, inbounds: Inbound[] = [], outbounds: Outbound[] = []) {
  const inbound = readableInboundName(rule, inbounds, text);
  const outbound = readableOutboundName(rule, outbounds);
  if (Number(rule.client_id || 0) > 0) {
    return `${readableClientInboundName(rule, inbounds, text, inbound) || inbound} / ${readableClientName(rule, inbounds, text)} -> ${outbound}`;
  }
  return `${inbound || firstRoutingMatch(rule, text)} -> ${outbound}`;
}

function firstRoutingMatch(rule: Pick<RoutingRule, 'domain' | 'ip' | 'rule_set' | 'protocol' | 'inbound_tag'>, text: (value: string) => string) {
  if (rule.domain) return compactRuleValue(rule.domain);
  if (rule.ip) return compactRuleValue(rule.ip);
  if (rule.rule_set) return compactRuleValue(rule.rule_set);
  if (rule.protocol) return `${text('协议')}: ${compactRuleValue(rule.protocol)}`;
  return rule.inbound_tag ? `${text('入站')}: ${rule.inbound_tag}` : text('全部入站');
}

function readableInboundName(rule: Pick<RoutingRule, 'inbound_id' | 'inbound_tag'>, inbounds: Inbound[], text: (value: string) => string) {
  const inbound = findInboundForRule(inbounds, rule);
  if (inbound) return String(inbound.remark || generatedInboundTag(inbound)).trim() || text('未命名入站');
  if (!rule.inbound_tag) return '';
  return String(rule.inbound_tag).trim();
}

function readableClientName(rule: Pick<RoutingRule, 'client_id' | 'client_email' | 'client_label'>, inbounds: Inbound[], text: (value: string) => string) {
  const clientID = Number(rule.client_id || 0);
  if (!clientID) return '-';
  const found = findClientById(inbounds, clientID);
  if (found) {
    return clientDisplayName(found.client, `${text('客户端')} #${clientID}`);
  }
  return String(rule.client_email || rule.client_label || `${text('客户端')} #${clientID}`).trim();
}

function clientRouteHint(rule: Pick<RoutingRule, 'client_id'>, inbounds: Inbound[]) {
  const clientID = Number(rule.client_id || 0);
  const found = clientID ? findClientById(inbounds, clientID) : undefined;
  if (!found) return '';
  const displayName = clientDisplayName(found.client, `client-${clientID}`);
  const routeIdentity = readableClientRouteIdentity(found);
  return routeIdentity && routeIdentity !== displayName ? routeIdentity : '';
}

function readableClientInboundName(rule: Pick<RoutingRule, 'client_id'>, inbounds: Inbound[], text: (value: string) => string, fallback = '') {
  const clientID = Number(rule.client_id || 0);
  const found = clientID ? findClientById(inbounds, clientID) : undefined;
  if (!found) return fallback || text('全部入站');
  return String(found.inbound.remark || generatedInboundTag(found.inbound)).trim() || text('未命名入站');
}

function findInboundForRule(inbounds: Inbound[], rule: Pick<RoutingRule, 'inbound_id' | 'inbound_tag'>) {
  const inboundID = Number(rule.inbound_id || 0);
  if (inboundID > 0) return inbounds.find((item) => item.id === inboundID);
  return findInboundByTag(inbounds, rule.inbound_tag || '');
}

function findInboundByTag(inbounds: Inbound[], tag: string) {
  const normalized = String(tag || '').trim();
  return inbounds.find((item) => generatedInboundTag(item) === normalized || String(item.remark || '').trim() === normalized);
}

function readableOutboundName(rule: Pick<RoutingRule, 'outbound_id' | 'outbound_tag'>, outbounds: Outbound[]) {
  const outboundID = Number(rule.outbound_id || 0);
  if (outboundID > 0) {
    const outbound = outbounds.find((item) => item.id === outboundID);
    if (outbound) return String(outbound.remark || outbound.tag).trim();
  }
  return rule.outbound_tag;
}

function outboundTagName(rule: Pick<RoutingRule, 'outbound_id' | 'outbound_tag'>, outbounds: Outbound[]) {
  const outboundID = Number(rule.outbound_id || 0);
  if (outboundID > 0) {
    const outbound = outbounds.find((item) => item.id === outboundID);
    if (outbound) return outbound.tag;
  }
  return rule.outbound_tag;
}

function findOutboundForValues(outbounds: Outbound[], rule: Pick<RoutingRule, 'outbound_id' | 'outbound_tag'>) {
  const outboundID = Number(rule.outbound_id || 0);
  if (outboundID > 0) {
    const found = outbounds.find((item) => item.id === outboundID);
    if (found) return found;
  }
  const tag = String(rule.outbound_tag || '').trim();
  return tag ? outbounds.find((item) => item.tag === tag) : undefined;
}

function hasRuleValue(value: unknown) {
  return String(value || '').trim().length > 0;
}

function hasMatchConditions(rule: Pick<RoutingRule, 'domain' | 'ip' | 'protocol' | 'rule_set'>) {
  return hasRuleValue(rule.domain) || hasRuleValue(rule.ip) || hasRuleValue(rule.protocol) || hasRuleValue(rule.rule_set);
}

function summaryCondition(rule: Pick<RoutingRule, 'domain' | 'ip' | 'protocol' | 'rule_set'>, text: (value: string) => string) {
  if (!hasMatchConditions(rule)) return text('所有流量');
  const tags = conditionTags(rule).filter((tag) => tag.kind !== 'catch_all');
  const first = tags[0];
  if (!first) return text('所有流量');
  return `${text('命中')} ${first.value} ${text('时')}`;
}

function ruleTypeLabel(rule: Pick<RoutingRule, 'client_id'>) {
  return Number(rule.client_id || 0) > 0 ? '客户端级' : '入站级';
}

function ruleTechnicalMeta(rule: RoutingRule, inbounds: Inbound[], outbounds: Outbound[], text: (value: string) => string): Array<{ label: string; value: string }> {
  return [
    { label: 'inbound_id：', value: String(Number(rule.inbound_id || 0) || '-') },
    { label: 'client_id：', value: String(Number(rule.client_id || 0) || '-') },
    { label: 'outbound_id：', value: String(Number(rule.outbound_id || 0) || '-') },
    { label: '内核：', value: inferRuleTargetCore(rule, inbounds, outbounds) },
    { label: 'inbound_tag：', value: rule.inbound_tag || readableInboundName(rule, inbounds, text) || '-' },
    { label: 'outbound_tag：', value: outboundTagName(rule, outbounds) || '-' },
  ];
}

function compactRuleValue(value: string) {
  const first = value.split(/[\n,]/).map((item) => item.trim()).find(Boolean) || value.trim();
  return first.length > 64 ? `${first.slice(0, 61)}...` : first;
}

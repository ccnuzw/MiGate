import { useMutation, useQuery } from '@tanstack/react-query';
import { ChevronDown, Copy, Link2, RotateCcw } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { appPath } from '../api/client';
import { api } from '../api/endpoints';
import type { Client, Inbound } from '../api/types';
import { Field, FieldError, Modal, SpinnerButton, useToast } from '../components/ui';
import { useI18n } from '../lib/i18n';
import {
  applyInboundTemplate,
  buildClientPayload,
  buildFullInboundPayload,
  clientFormValues,
  generatedProtocolCredential,
  hasAttachableSettingCert,
  inboundFormValues,
  inboundTemplateOptions,
} from './InboundsPage';

const inboundSchema = z.object({
  remark: z.string().min(1, '请输入名称'),
  protocol: z.enum(['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'shadowtls']),
  port: z.preprocess((value) => (value === '' || value == null ? 0 : value), z.coerce.number().int().min(0).max(65535)),
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
  email: z.string().min(1, '请输入客户端名称'),
  uuid: z.string().min(1, '请输入凭据'),
  enabled: z.boolean().default(true),
  traffic_limit_gb: z.coerce.number().min(0).optional(),
  expiry_mode: z.enum(['unlimited', '30d', '90d', 'custom']).default('unlimited'),
  expiry_date: z.string().optional(),
});

export type InboundInput = z.input<typeof inboundSchema>;
export type InboundValues = z.output<typeof inboundSchema>;
export type ClientInput = z.input<typeof clientSchema>;
export type ClientValues = z.output<typeof clientSchema>;
type TemplateSelectValue = 'recommended' | 'compatible' | 'performance' | 'simple' | 'keep';

export function InboundModal({ inbound, onClose, onSaved }: { inbound: Inbound | null; onClose: () => void; onSaved: () => void }) {
  const { showToast } = useToast();
  const { text } = useI18n();
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [templateId, setTemplateId] = useState<TemplateSelectValue>('recommended');
  const form = useForm<InboundInput, unknown, InboundValues>({
    resolver: zodResolver(inboundSchema),
    values: inbound ? inboundFormValues(inbound) : undefined,
  });
  useEffect(() => {
    if (!inbound) return;
    setAdvancedOpen(false);
    setTemplateId(inbound.id ? 'keep' : 'recommended');
  }, [inbound]);
  const protocol = form.watch('protocol');
  const network = form.watch('network');
  const security = form.watch('security');
  const portValue = form.watch('port');
  const portRegistration = form.register('port');
  const cert = useQuery({ queryKey: ['cert-status'], queryFn: api.certStatus, enabled: !!inbound && security === 'tls', retry: false, staleTime: 60_000 });
  const settingCert = cert.data;
  const canAttachSettingCert = hasAttachableSettingCert(settingCert);
  const regenerateCredential = () => {
    form.setValue('uuid', generatedProtocolCredential(protocol), { shouldDirty: true, shouldValidate: true });
  };
  const applyTemplate = (id: Exclude<TemplateSelectValue, 'keep'>) => {
    setTemplateId(id);
    const next = applyInboundTemplate(form.getValues() as InboundValues, id);
    (Object.keys(next) as Array<keyof InboundValues>).forEach((key) => {
      form.setValue(key, next[key], { shouldDirty: true, shouldValidate: true });
    });
  };
  const attachSettingCert = () => {
    if (!canAttachSettingCert || !settingCert) return;
    form.setValue('tls_cert_file', settingCert.cert_path.trim(), { shouldDirty: true, shouldValidate: true });
    form.setValue('tls_key_file', settingCert.key_path.trim(), { shouldDirty: true, shouldValidate: true });
    if (!form.getValues('tls_sni') && settingCert.domain) {
      form.setValue('tls_sni', settingCert.domain, { shouldDirty: true, shouldValidate: true });
    }
    showToast(text('已关联设置中的 TLS 证书'), 'success');
  };
  const save = useMutation({
    mutationFn: (values: InboundValues) => (inbound?.id ? api.updateInbound(inbound.id, buildFullInboundPayload(inbound, values)) : api.createInbound(buildFullInboundPayload(inbound, values))),
    onSuccess: () => {
      showToast(text('入站已保存'), 'success');
      onSaved();
      onClose();
    },
    onError: (error) => showToast(errorMessage(error, text('保存入站失败')), 'error'),
  });
  return (
    <Modal
      open={!!inbound}
      title={text(inbound?.id ? '编辑入站' : '新增入站')}
      onClose={onClose}
      panelClassName="inbound-modal-panel"
      footer={
        <>
          <button className="btn secondary" onClick={onClose}>{text('取消')}</button>
          <SpinnerButton className="btn primary" loading={save.isPending} onClick={form.handleSubmit((values) => save.mutate(values))}>{text('保存')}</SpinnerButton>
        </>
      }
    >
      <div className="form-grid inbound-form-grid">
        <div className="template-panel span-2">
          <Field label={text('模板 / 推荐配置')} help={text('选择后会自动填充协议、传输、安全和常用默认值，仍可手动修改。')}>
            <div className="split-control">
              <select value={templateId} onChange={(event) => setTemplateId(event.target.value as TemplateSelectValue)}>
                <option value="keep">{text('保留当前配置')}</option>
                {inboundTemplateOptions().map((template) => <option key={template.id} value={template.id}>{text(template.label)}</option>)}
              </select>
              <button className="btn secondary whitespace-nowrap" type="button" disabled={templateId === 'keep'} onClick={() => applyTemplate(templateId as Exclude<TemplateSelectValue, 'keep'>)}>{text('应用模板')}</button>
            </div>
          </Field>
        </div>
        <Field label={text('名称')}><input {...form.register('remark')} /><FieldError message={form.formState.errors.remark?.message ? text(form.formState.errors.remark.message) : undefined} /></Field>
        <Field label={text('端口')} help={text('留空保存时自动分配可用端口')}>
          <input
            type="number"
            placeholder={text('自动分配')}
            name={portRegistration.name}
            ref={portRegistration.ref}
            onBlur={portRegistration.onBlur}
            value={Number(portValue || 0) === 0 ? '' : String(portValue)}
            onChange={(event) => {
              const raw = event.target.value;
              if (raw === '') {
                form.setValue('port', 0, { shouldDirty: true, shouldValidate: true });
                return;
              }
              const next = Number(raw);
              if (!Number.isFinite(next)) return;
              form.setValue('port', next, { shouldDirty: true, shouldValidate: true });
            }}
          />
          <FieldError message={form.formState.errors.port?.message} />
        </Field>
        <Field label={text('协议')}>
          <select {...form.register('protocol')}>
            {['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'shadowtls'].map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </Field>
        <Field label={text('传输')}>
          <select {...form.register('network')}>
            {['tcp', 'udp', 'ws', 'grpc', 'h2', 'xhttp', 'quic', 'kcp'].map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </Field>
        <Field label={text('安全')}>
          <select {...form.register('security')}>
            {['none', 'tls', 'reality'].map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </Field>
        {security === 'reality' ? (
          <>
            <Field label={text('伪装目标')}><input {...form.register('reality_dest')} placeholder="www.cloudflare.com:443" /></Field>
            <Field label="Server Name"><input {...form.register('reality_server_names')} placeholder="www.cloudflare.com" /></Field>
          </>
        ) : null}
        {security === 'tls' ? <Field label={text('域名 / SNI')}><input {...form.register('tls_sni')} placeholder="example.com" /></Field> : null}
        <div className="span-2">
          <button className="advanced-toggle" type="button" onClick={() => setAdvancedOpen((open) => !open)} aria-expanded={advancedOpen}>
            <ChevronDown className={advancedOpen ? 'h-4 w-4 rotate-180' : 'h-4 w-4'} />
            {text('高级设置')}
          </button>
        </div>
        {advancedOpen ? (
          <div className="advanced-panel span-2">
            <div className="form-grid advanced-form-grid">
              <Field label={text('入站内部 ID')}>
                <div className="input-action-row">
                  <input {...form.register('uuid')} />
                  <button className="icon-button" type="button" onClick={regenerateCredential} title={text('重新生成')}>
                    <RotateCcw className="h-4 w-4" />
                  </button>
                </div>
              </Field>
              {security === 'reality' ? <Field label={text('REALITY Short ID')}><input {...form.register('reality_short_id')} /></Field> : null}
              {(network === 'ws' || network === 'h2') ? <Field label={text('WS/H2 路径')}><input {...form.register('ws_path')} /></Field> : null}
              {network === 'grpc' ? <Field label={text('gRPC 服务名')}><input {...form.register('grpc_service_name')} /></Field> : null}
              {network === 'xhttp' ? <Field label={text('XHTTP 路径')}><input {...form.register('xhttp_path')} /></Field> : null}
              <div className="advanced-note span-2">{text('仅用于识别这个入站，不是客户端连接凭据；一般无需修改。')}</div>
              {(network === 'ws' || network === 'h2') ? (
                <>
                  <Field label={text('WS/H2 主机')}><input {...form.register('ws_host')} /></Field>
                </>
              ) : null}
              {network === 'xhttp' ? (
                <>
                  <Field label={text('XHTTP 模式')}><input {...form.register('xhttp_mode')} placeholder="stream-one" /></Field>
                </>
              ) : null}
              {security === 'reality' ? (
                <>
                  <Field label={text('REALITY 私钥')}><input {...form.register('reality_private_key')} /></Field>
                  <Field label={text('REALITY 公钥')}><input {...form.register('reality_public_key')} readOnly /></Field>
                  <Field label="TLS Fingerprint"><input {...form.register('tls_fingerprint')} /></Field>
                  <Field label="TLS ALPN"><input {...form.register('tls_alpn')} placeholder="h2,http/1.1" /></Field>
                </>
              ) : null}
              {security === 'tls' ? (
                <>
            <div className="span-2 rounded-lg border border-panel-line bg-panel-soft p-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-sm font-medium text-panel-text">{text('关联设置证书')}</div>
                  <div className="mt-1 break-all text-xs leading-5 text-panel-muted">
                    {settingCert?.domain ? `${text('域名')}：${settingCert.domain}` : text('设置中暂无证书域名')}
                    {settingCert?.issued ? ` · ${text('已获取')}` : ''}
                  </div>
                </div>
                <button type="button" className="btn secondary h-8" disabled={!canAttachSettingCert} onClick={attachSettingCert}>
                  <Link2 className="h-4 w-4" /> {text('关联证书')}
                </button>
              </div>
              <div className="mt-2 text-xs leading-5 text-panel-muted">
                {canAttachSettingCert ? text('将设置页已获取的证书路径填入下方 TLS 证书和私钥字段。') : text('请先在设置页配置并获取 TLS 证书。')}
              </div>
            </div>
            <Field label={text('TLS 证书文件')}><input {...form.register('tls_cert_file')} /></Field>
            <Field label={text('TLS 私钥文件')}><input {...form.register('tls_key_file')} /></Field>
            <Field label="TLS Fingerprint"><input {...form.register('tls_fingerprint')} /></Field>
            <Field label="TLS ALPN"><input {...form.register('tls_alpn')} /></Field>
                </>
              ) : null}
              {protocol === 'shadowsocks' ? <Field label={text('Shadowsocks 加密方法')}><input {...form.register('ss_method')} placeholder="2022-blake3-aes-128-gcm" /></Field> : null}
              {protocol === 'hysteria2' ? (
                <>
                  <Field label={text('HY2 上行 Mbps')}><input type="number" {...form.register('hy2_up_mbps')} /></Field>
                  <Field label={text('HY2 下行 Mbps')}><input type="number" {...form.register('hy2_down_mbps')} /></Field>
                  <Field label={text('HY2 混淆')}><input {...form.register('hy2_obfs')} /></Field>
                  <Field label={text('HY2 混淆密码')}><input {...form.register('hy2_obfs_password')} /></Field>
                  <Field label={text('HY2 多端口')}><input {...form.register('hy2_mport')} placeholder="40000-50000" /></Field>
                </>
              ) : null}
              {protocol === 'tuic' ? (
                <>
                  <Field label={text('TUIC 拥塞控制')}><input {...form.register('tuic_congestion_control')} placeholder="bbr" /></Field>
                  <label className="checkbox-field"><input type="checkbox" {...form.register('tuic_zero_rtt')} /> {text('启用 0-RTT')}</label>
                </>
              ) : null}
              {protocol === 'shadowtls' ? (
                <>
                  <Field label={text('ShadowTLS 版本')}><input type="number" {...form.register('shadowtls_version')} /></Field>
                  <Field label={text('ShadowTLS 密码')}><input {...form.register('shadowtls_password')} /></Field>
                </>
              ) : null}
            </div>
          </div>
        ) : null}
      </div>
    </Modal>
  );
}

export function ClientModal({ inbound, client, onClose, onSaved }: { inbound: Inbound | null; client?: Client; onClose: () => void; onSaved: () => void }) {
  const { showToast } = useToast();
  const { text } = useI18n();
  const [credentialOpen, setCredentialOpen] = useState(false);
  const [savedClient, setSavedClient] = useState<Client | null>(null);
  const values = useMemo(
    () => (inbound ? clientFormValues(inbound, client) : undefined),
    [client, inbound],
  );
  const form = useForm<ClientInput, unknown, ClientValues>({
    resolver: zodResolver(clientSchema),
    values,
  });
  const regenerateCredential = () => {
    form.setValue('uuid', generatedProtocolCredential(inbound?.protocol), { shouldDirty: true, shouldValidate: true });
  };
  const save = useMutation({
    mutationFn: async (values: ClientValues) => {
      const payload = buildClientPayload(values);
      const response = client ? await api.updateClient(inbound!.id, client.id, { ...client, ...payload }) : await api.createClient(inbound!.id, payload);
      return { payload, response };
    },
    onSuccess: ({ payload, response }) => {
      showToast(text('客户端已保存'), 'success');
      onSaved();
      setSavedClient(extractClientResponse(response, client, payload, inbound?.id || client?.inbound_id || 0));
    },
    onError: (error) => showToast(errorMessage(error, text('保存客户端失败')), 'error'),
  });
  const expiryMode = form.watch('expiry_mode');
  useEffect(() => {
    if (!inbound) return;
    setCredentialOpen(false);
    setSavedClient(null);
  }, [inbound, client]);
  const close = () => {
    setSavedClient(null);
    onClose();
  };
  return (
    <Modal
      open={!!inbound}
      title={text(client ? '编辑客户端' : '新增客户端')}
      onClose={close}
      footer={
        <>
          <button className="btn secondary" onClick={close}>{text(savedClient ? '完成' : '取消')}</button>
          <SpinnerButton className="btn primary" loading={save.isPending} disabled={Boolean(savedClient)} onClick={form.handleSubmit((values) => save.mutate(values))}>{text('保存')}</SpinnerButton>
        </>
      }
    >
      <div className="form-grid">
        <Field label={text('客户端名称')}><input {...form.register('email')} /><FieldError message={form.formState.errors.email?.message ? text(form.formState.errors.email.message) : undefined} /></Field>
        <Field label={text('流量限额（GB，0 不限制）')}><input type="number" step="0.1" {...form.register('traffic_limit_gb')} /></Field>
        <Field label={text('到期时间')}>
          <select {...form.register('expiry_mode')}>
            <option value="unlimited">{text('不限制')}</option>
            <option value="30d">{text('30 天')}</option>
            <option value="90d">{text('90 天')}</option>
            <option value="custom">{text('自定义日期')}</option>
          </select>
        </Field>
        {expiryMode === 'custom' ? <Field label={text('自定义到期日期')}><input type="date" {...form.register('expiry_date')} /></Field> : null}
        <label className="checkbox-field"><input type="checkbox" {...form.register('enabled')} /> {text('已启用')}</label>
        <div className="span-2">
          <button className="advanced-toggle" type="button" onClick={() => setCredentialOpen((open) => !open)} aria-expanded={credentialOpen}>
            <ChevronDown className={credentialOpen ? 'h-4 w-4 rotate-180' : 'h-4 w-4'} />
            {text('凭据')}
          </button>
        </div>
        {credentialOpen ? (
          <div className="advanced-panel span-2">
            <Field label={text('UUID / 密码 / 密钥')} help={text(client ? '现有凭据会反显，可手动修改。' : '默认自动生成，可在保存前手动修改。')}>
              <div className="input-action-row">
                <input {...form.register('uuid')} />
                <button className="icon-button" type="button" onClick={regenerateCredential} title={text('重新生成')}>
                  <RotateCcw className="h-4 w-4" />
                </button>
              </div>
              <FieldError message={form.formState.errors.uuid?.message ? text(form.formState.errors.uuid.message) : undefined} />
            </Field>
          </div>
        ) : null}
        {savedClient ? (
          <div className="advanced-panel span-2">
            <div className="mb-3 text-sm font-medium text-panel-text">{text('客户端已保存，可直接复制链接')}</div>
            <div className="action-row justify-start">
              <button className="btn secondary" type="button" onClick={() => copyText(subscriptionURL(savedClient), text('订阅链接已复制'), text('复制失败'), showToast)}>
                <Copy className="h-4 w-4" /> {text('复制订阅链接')}
              </button>
              <button className="btn secondary" type="button" onClick={() => copyShareLink(savedClient, showToast, text)}>
                <Copy className="h-4 w-4" /> {text('复制分享链接')}
              </button>
            </div>
          </div>
        ) : null}
      </div>
    </Modal>
  );
}

function extractClientResponse(response: unknown, fallback?: Client, payload?: ReturnType<typeof buildClientPayload>, inboundId = 0): Client | null {
  if (isClient(response)) return response;
  if (response && typeof response === 'object' && isClient((response as { client?: unknown }).client)) return (response as { client: Client }).client;
  if (!payload) return fallback || null;
  return {
    id: fallback?.id || 0,
    inbound_id: fallback?.inbound_id || inboundId,
    email: payload.email,
    uuid: payload.uuid,
    subscription_token: fallback?.subscription_token,
    enabled: payload.enabled,
    traffic_limit: payload.traffic_limit,
    expiry_at: payload.expiry_at,
  };
}

function isClient(value: unknown): value is Client {
  return Boolean(value && typeof value === 'object' && 'uuid' in value && 'email' in value);
}

function subscriptionURL(client: Client) {
  return `${window.location.origin}${appPath(`/sub/${subscriptionToken(client)}`)}`;
}

async function copyText(value: string, title: string, errorTitle: string, showToast: (title: string, tone?: 'success' | 'error' | 'info') => void) {
  try {
    if (!navigator.clipboard) throw new Error('clipboard_unavailable');
    await navigator.clipboard.writeText(value);
    showToast(title, 'success');
  } catch {
    showToast(errorTitle, 'error');
  }
}

async function copyShareLink(client: Client, showToast: (title: string, tone?: 'success' | 'error' | 'info') => void, text: (value: string) => string) {
  try {
    const response = await fetch(appPath(`/sub/${subscriptionToken(client)}`), { credentials: 'same-origin' });
    if (!response.ok) throw new Error('share_link_unavailable');
    const value = await response.text();
    await copyText(value.trim(), text('客户端分享链接已复制'), text('复制失败'), showToast);
  } catch {
    showToast(text('复制分享链接失败'), 'error');
  }
}

function subscriptionToken(client: Client) {
  return client.subscription_token || client.uuid;
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

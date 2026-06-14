import { useMutation, useQuery } from '@tanstack/react-query';
import { Link2, RotateCcw } from 'lucide-react';
import { useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { api } from '../api/endpoints';
import type { Client, Inbound } from '../api/types';
import { Field, FieldError, Modal, SpinnerButton, useToast } from '../components/ui';
import { useI18n } from '../lib/i18n';
import {
  buildFullInboundPayload,
  clientFormValues,
  generatedProtocolCredential,
  hasAttachableSettingCert,
  inboundFormValues,
} from './InboundsPage';

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

export type InboundInput = z.input<typeof inboundSchema>;
export type InboundValues = z.output<typeof inboundSchema>;
export type ClientInput = z.input<typeof clientSchema>;
export type ClientValues = z.output<typeof clientSchema>;

export function InboundModal({ inbound, onClose, onSaved }: { inbound: Inbound | null; onClose: () => void; onSaved: () => void }) {
  const { showToast } = useToast();
  const { text } = useI18n();
  const form = useForm<InboundInput, unknown, InboundValues>({
    resolver: zodResolver(inboundSchema),
    values: inbound ? inboundFormValues(inbound) : undefined,
  });
  const protocol = form.watch('protocol');
  const network = form.watch('network');
  const security = form.watch('security');
  const cert = useQuery({ queryKey: ['cert-status'], queryFn: api.certStatus, enabled: !!inbound && security === 'tls', retry: false, staleTime: 60_000 });
  const settingCert = cert.data;
  const canAttachSettingCert = hasAttachableSettingCert(settingCert);
  const regenerateCredential = () => {
    form.setValue('uuid', generatedProtocolCredential(protocol), { shouldDirty: true, shouldValidate: true });
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
      footer={
        <>
          <button className="btn secondary" onClick={onClose}>{text('取消')}</button>
          <SpinnerButton className="btn primary" loading={save.isPending} onClick={form.handleSubmit((values) => save.mutate(values))}>{text('保存')}</SpinnerButton>
        </>
      }
    >
      <div className="form-grid">
        <Field label={text('名称')}><input {...form.register('remark')} /><FieldError message={form.formState.errors.remark?.message ? text(form.formState.errors.remark.message) : undefined} /></Field>
        <Field label={text('端口')}><input type="number" {...form.register('port')} /><FieldError message={form.formState.errors.port?.message} /></Field>
        <Field label={text('协议')}>
          <select {...form.register('protocol')}>
            {['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'shadowtls'].map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </Field>
        <Field label={text('传输')}>
          <select {...form.register('network')}>
            {['tcp', 'ws', 'grpc', 'h2', 'xhttp', 'quic', 'kcp'].map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </Field>
        <Field label={text('安全')}>
          <select {...form.register('security')}>
            {['none', 'tls', 'reality'].map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </Field>
        <Field label={text(protocol === 'shadowsocks' || protocol === 'hysteria2' || protocol === 'tuic' || protocol === 'shadowtls' ? '密码 / 密钥' : 'UUID')}>
          <div className="input-action-row">
            <input {...form.register('uuid')} />
            <button className="icon-button" type="button" onClick={regenerateCredential} title={text('重新生成')}>
              <RotateCcw className="h-4 w-4" />
            </button>
          </div>
        </Field>
        {(network === 'ws' || network === 'h2') ? (
          <>
            <Field label={text('WS/H2 路径')}><input {...form.register('ws_path')} /></Field>
            <Field label={text('WS/H2 主机')}><input {...form.register('ws_host')} /></Field>
          </>
        ) : null}
        {network === 'grpc' ? <Field label={text('gRPC 服务名')}><input {...form.register('grpc_service_name')} /></Field> : null}
        {network === 'xhttp' ? (
          <>
            <Field label={text('XHTTP 路径')}><input {...form.register('xhttp_path')} /></Field>
            <Field label={text('XHTTP 模式')}><input {...form.register('xhttp_mode')} placeholder="stream-one" /></Field>
          </>
        ) : null}
        {security === 'reality' ? (
          <>
            <Field label={text('REALITY 目标地址')}><input {...form.register('reality_dest')} placeholder="example.com:443" /></Field>
            <Field label={text('REALITY 服务名')}><input {...form.register('reality_server_names')} /></Field>
            <Field label={text('REALITY Short ID')}><input {...form.register('reality_short_id')} /></Field>
            <Field label={text('REALITY 私钥')}><input {...form.register('reality_private_key')} /></Field>
            <Field label={text('REALITY 公钥')}><input {...form.register('reality_public_key')} readOnly /></Field>
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
            <Field label="TLS SNI"><input {...form.register('tls_sni')} /></Field>
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
    </Modal>
  );
}

export function ClientModal({ inbound, client, onClose, onSaved }: { inbound: Inbound | null; client?: Client; onClose: () => void; onSaved: () => void }) {
  const { showToast } = useToast();
  const { text } = useI18n();
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
    mutationFn: (values: ClientValues) => (client ? api.updateClient(inbound!.id, client.id, { ...client, ...values }) : api.createClient(inbound!.id, values)),
    onSuccess: () => {
      showToast(text('客户端已保存'), 'success');
      onSaved();
      onClose();
    },
    onError: (error) => showToast(errorMessage(error, text('保存客户端失败')), 'error'),
  });
  return (
    <Modal
      open={!!inbound}
      title={text(client ? '编辑客户端' : '新增客户端')}
      onClose={onClose}
      footer={
        <>
          <button className="btn secondary" onClick={onClose}>{text('取消')}</button>
          <SpinnerButton className="btn primary" loading={save.isPending} onClick={form.handleSubmit((values) => save.mutate(values))}>{text('保存')}</SpinnerButton>
        </>
      }
    >
      <div className="form-grid">
        <Field label={text('客户端标识')}><input {...form.register('email')} /><FieldError message={form.formState.errors.email?.message ? text(form.formState.errors.email.message) : undefined} /></Field>
        <Field label={text('UUID / 密码 / 密钥')}>
          <div className="input-action-row">
            <input {...form.register('uuid')} />
            <button className="icon-button" type="button" onClick={regenerateCredential} title={text('重新生成')}>
              <RotateCcw className="h-4 w-4" />
            </button>
          </div>
          <FieldError message={form.formState.errors.uuid?.message ? text(form.formState.errors.uuid.message) : undefined} />
        </Field>
        <Field label={text('流量限额（字节，0 不限制）')}><input type="number" {...form.register('traffic_limit')} /></Field>
        <Field label={text('过期时间戳（0 不限制）')}><input type="number" {...form.register('expiry_at')} /></Field>
        <label className="checkbox-field"><input type="checkbox" {...form.register('enabled')} /> {text('已启用')}</label>
      </div>
    </Modal>
  );
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

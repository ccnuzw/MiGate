import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ChevronDown, ChevronUp, ExternalLink, FileKey2, Link2, RefreshCw, RotateCcw, Save, ShieldCheck, ShieldX, Trash2, UploadCloud } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { ApiError, getAPIErrorMessage } from '../api/client';
import { api } from '../api/endpoints';
import type { CertificatePreflight, Inbound, ManagedCertificate, Settings, UpdateStatus } from '../api/types';
import { Card, Field, LoadingBlock, SpinnerButton, useConfirm, useToast } from '../components/ui';
import { serviceLabel } from '../lib/format';
import { useI18n } from '../lib/i18n';
import { refreshCertificateApplyDependencies, refreshQueries, refreshQuery, refreshSessionDependencies, refreshSettingsDependencies, refreshUpdateDependencies } from '../lib/queryInvalidation';
import { PageTitle } from './OverviewPage';

export default function SettingsPage() {
  const queryClient = useQueryClient();
  const confirm = useConfirm();
  const { showToast } = useToast();
  const { text } = useI18n();
  const [watchUpdateStatus, setWatchUpdateStatus] = useState(false);
  const [showAllSessions, setShowAllSessions] = useState(false);
  const [certDomainsInput, setCertDomainsInput] = useState('');
  const [certEmailInput, setCertEmailInput] = useState('');
  const [importName, setImportName] = useState('');
  const [importFullchain, setImportFullchain] = useState('');
  const [importKey, setImportKey] = useState('');
  const [selectedCertId, setSelectedCertId] = useState<number | null>(null);
  const [selectedInboundIds, setSelectedInboundIds] = useState<number[]>([]);
  const [preflightResult, setPreflightResult] = useState<CertificatePreflight | null>(null);
  const session = useQuery({ queryKey: ['session'], queryFn: api.session, staleTime: 5 * 60_000 });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings, retry: false, staleTime: 60_000 });
  const cert = useQuery({ queryKey: ['cert-status'], queryFn: api.certStatus, retry: false, staleTime: 60_000 });
  const certificates = useQuery({ queryKey: ['certificates'], queryFn: api.certificates, retry: false, staleTime: 60_000 });
  const certificateInbounds = useQuery({ queryKey: ['certificate-inbounds'], queryFn: api.certificateInboundTargets, retry: false, staleTime: 60_000 });
  const updateCheck = useQuery({
    queryKey: ['update-check'],
    queryFn: api.updateCheck,
    enabled: watchUpdateStatus,
    retry: false,
    refetchInterval: () => updateDependencyRefetchInterval(watchUpdateStatus),
  });
  const version = useQuery({
    queryKey: ['version'],
    queryFn: api.version,
    enabled: watchUpdateStatus,
    retry: false,
    refetchInterval: () => updateDependencyRefetchInterval(watchUpdateStatus),
  });
  const updateStatus = useQuery({
    queryKey: ['update-status'],
    queryFn: api.updateStatus,
    refetchInterval: (query) => updateStatusRefetchInterval(query.state.data?.status, watchUpdateStatus),
    staleTime: 30_000,
  });
  const updateLogs = useQuery({
    queryKey: ['update-logs'],
    queryFn: api.updateLogs,
    enabled: watchUpdateStatus,
    retry: false,
    refetchInterval: () => updateDependencyRefetchInterval(watchUpdateStatus),
  });
  const sessions = useQuery({ queryKey: ['sessions'], queryFn: api.sessions, retry: false, staleTime: 60_000 });
  const service = useQuery({
    queryKey: ['service-status'],
    queryFn: api.serviceStatus,
    retry: false,
    staleTime: 30_000,
    refetchInterval: () => updateDependencyRefetchInterval(watchUpdateStatus),
  });
  const form = useForm<Settings>({ values: settings.data || {} });
  const certDomain = form.watch('cert_domain') || cert.data?.domain || '';
  const certEmail = form.watch('cert_email') || cert.data?.email || '';
  const managedCertificates = certificates.data?.certificates || [];
  const tlsInbounds = certificateInbounds.data?.inbounds || [];
  const selectedCertificate = useMemo(() => managedCertificates.find((item) => item.id === selectedCertId) || managedCertificates[0], [managedCertificates, selectedCertId]);
  useEffect(() => {
    if (watchUpdateStatus && isUpdateTerminal(updateStatus.data?.status)) {
      refreshQueries([updateStatus, updateLogs, service, version, updateCheck]);
      if (updateStatus.data?.status !== 'completed' || version.data?.version) {
        setWatchUpdateStatus(false);
      }
    }
  }, [updateStatus.data?.status, version.data?.version, watchUpdateStatus]);
  useEffect(() => {
    if (watchUpdateStatus && version.data?.version && isUpdateTerminal(updateStatus.data?.status)) {
      refreshQuery(updateCheck);
    }
  }, [updateStatus.data?.status, version.data?.version, watchUpdateStatus]);
  const save = useMutation({
    mutationFn: (values: Settings) => api.saveSettings(settingsPayload(settings.data, values)),
    onSuccess: () => {
      showToast(text('设置已保存，端口、数据库或基础路径变更需要重启服务后生效'), 'success');
      form.setValue('panel_password', '');
      refreshSettingsDependencies(queryClient);
    },
    onError: (error) => showToast(errorMessage(error, text('保存设置失败')), 'error'),
  });
  const saveCert = useMutation({
    mutationFn: (values: Settings) => api.saveSettings(certSettingsPayload(settings.data, values)),
    onSuccess: () => {
      showToast(text('证书配置已保存'), 'success');
      refreshSettingsDependencies(queryClient);
    },
    onError: (error) => showToast(errorMessage(error, text('保存证书配置失败')), 'error'),
  });
  const restart = useMutation({
    mutationFn: api.restart,
    onSuccess: () => showToast(text('重启命令已发送'), 'success'),
    onError: (error) => showToast(errorMessage(error, text('重启失败')), 'error'),
  });
  const issueCert = useMutation({
    mutationFn: () => {
      const payload = certIssuePayload(form.getValues(), cert.data);
      return api.issueCert(payload.domain, payload.email);
    },
    onSuccess: () => {
      showToast(text('证书已获取'), 'success');
      refreshSettingsDependencies(queryClient);
    },
    onError: (error) => showToast(errorMessage(error, text('获取证书失败')), 'error'),
  });
  const preflightCert = useMutation({
    mutationFn: () => api.certificatePreflight(parseDomains(certDomainsInput || certDomain), certEmailInput || certEmail),
    onSuccess: (result) => setPreflightResult(result.preflight),
    onError: (error) => showToast(errorMessage(error, text('预检查失败')), 'error'),
  });
  const createManagedCert = useMutation({
    mutationFn: () => api.createCertificate(parseDomains(certDomainsInput || certDomain), certEmailInput || certEmail),
    onSuccess: (result) => {
      setPreflightResult(result.preflight);
      showToast(text('证书申请已完成'), 'success');
      refreshSettingsDependencies(queryClient);
    },
    onError: (error) => {
      const preflight = preflightFromAPIError(error);
      if (preflight) setPreflightResult(preflight);
      showToast(errorMessage(error, text('证书申请失败')), 'error');
    },
  });
  const importCert = useMutation({
    mutationFn: () => api.importCertificate({ name: importName, fullchain: importFullchain, private_key: importKey }),
    onSuccess: () => {
      setImportFullchain('');
      setImportKey('');
      showToast(text('证书已导入'), 'success');
      refreshSettingsDependencies(queryClient);
    },
    onError: (error) => showToast(errorMessage(error, text('导入证书失败')), 'error'),
  });
  const renewCerts = useMutation({
    mutationFn: () => api.renewDueCertificates(30),
    onSuccess: (result) => {
      showToast(`${text('续期检查完成')}：${result.renewal?.renewed?.length || 0} ${text('个已续期')}`, 'success');
      refreshSettingsDependencies(queryClient);
    },
    onError: (error) => showToast(errorMessage(error, text('续期检查失败')), 'error'),
  });
  const applyCert = useMutation({
    mutationFn: () => api.applyCertificate(selectedCertificate?.id || 0, selectedInboundIds),
    onSuccess: () => {
      showToast(text('证书已应用到入站'), 'success');
      refreshCertificateApplyDependencies(queryClient);
    },
    onError: (error) => showToast(errorMessage(error, text('应用证书失败')), 'error'),
  });
  const deleteCert = useMutation({
    mutationFn: (id: number) => api.deleteCertificate(id),
    onSuccess: () => {
      showToast(text('证书已删除'), 'success');
      refreshSettingsDependencies(queryClient);
    },
    onError: (error) => showToast(errorMessage(error, text('删除证书失败')), 'error'),
  });
  const update = useMutation({
    mutationFn: api.update,
    onSuccess: () => {
      setWatchUpdateStatus(true);
      showToast(text('更新命令已发送'), 'success');
      refreshUpdateDependencies(queryClient);
      refreshQuery(updateLogs);
    },
    onError: (error) => showToast(errorMessage(error, text('启动更新失败')), 'error'),
  });
  const revoke = useMutation({
    mutationFn: api.revokeSession,
    onSuccess: () => {
      showToast(text('会话已撤销'), 'success');
      refreshSessionDependencies(queryClient);
    },
    onError: (error) => showToast(errorMessage(error, text('撤销会话失败')), 'error'),
  });
  const revokeOthers = useMutation({
    mutationFn: api.revokeOtherSessions,
    onSuccess: (result) => {
      showToast(`${text('已撤销')} ${result.revoked} ${text('个其他会话')}`, 'success');
      refreshSessionDependencies(queryClient);
    },
    onError: (error) => showToast(errorMessage(error, text('撤销其他会话失败')), 'error'),
  });
  const sessionItems = sessions.data || [];
  const visibleSessions = showAllSessions ? sessionItems : sessionItems.slice(0, defaultVisibleSessions);
  const hiddenSessionCount = Math.max(0, sessionItems.length - visibleSessions.length);
  const waitingForService = watchUpdateStatus && [updateStatus, updateLogs, service, version, updateCheck].some((query) => query.isError);
  const updateSummary = updateStatusSummaryKey(updateStatus.data);

  if (settings.isLoading) return <LoadingBlock />;

  return (
    <div className="page-stack">
      <PageTitle title={text('面板设置')} description={text('管理面板端口、路径、凭据、证书、服务状态、更新与活动会话。')} />
      {session.data?.default_password ? (
        <Card className="border-red-200 bg-red-50 p-4 text-sm text-red-700">
          {text('当前仍在使用默认密码，请尽快修改面板密码。')}
        </Card>
      ) : null}
      <Card className="p-5">
        <form className="form-grid" onSubmit={form.handleSubmit((values) => save.mutate(values))}>
          <Field label={text('面板端口')}><input type="number" {...form.register('panel_port', { valueAsNumber: true })} /></Field>
          <Field label={text('用户名')}><input {...form.register('panel_username')} /></Field>
          <Field label={text('新密码')} help={settings.data?.has_password ? text('留空表示保留现有密码。') : undefined}><input type="password" autoComplete="new-password" {...form.register('panel_password')} /></Field>
          <Field label={text('Web 基础路径')}><input placeholder="/panel" {...form.register('web_base_path')} /></Field>
          <Field label={text('数据库路径')}><input {...form.register('database_path')} /></Field>
          <div className="span-2 flex flex-wrap justify-end gap-2">
            <button type="button" className="btn secondary" onClick={() => refreshQueries([settings, cert, service])}><RefreshCw className="h-4 w-4" /> {text('刷新')}</button>
            <SpinnerButton type="submit" className="btn primary" loading={save.isPending}><Save className="h-4 w-4" /> {text('保存设置')}</SpinnerButton>
            <SpinnerButton type="button" className="btn danger" loading={restart.isPending} onClick={async () => (await confirm({ title: text('重启 MiGate 服务？'), description: text('服务重启后当前连接可能短暂中断。'), tone: 'danger' })) && restart.mutate()}><RotateCcw className="h-4 w-4" /> {text('重启服务')}</SpinnerButton>
          </div>
        </form>
      </Card>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="p-5">
          <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
            <h2 className="section-title">{text('TLS 证书配置')}</h2>
            <div className="flex flex-wrap gap-2">
              <SpinnerButton type="button" className="btn secondary" loading={saveCert.isPending} onClick={form.handleSubmit((values) => saveCert.mutate(values))}><Save className="h-4 w-4" /> {text('保存证书配置')}</SpinnerButton>
              <SpinnerButton className="btn primary" loading={issueCert.isPending} disabled={!certDomain || !certEmail} onClick={async () => (await confirm({ title: text('获取 TLS 证书？'), description: text('兼容接口会使用 MiGate 原生 ACME HTTP-01 申请证书，并可能占用 80 端口。') })) && issueCert.mutate()}>{text('兼容申请')}</SpinnerButton>
            </div>
          </div>
          <div className="grid gap-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label={text('证书域名')}><input placeholder="example.com" {...form.register('cert_domain')} /></Field>
              <Field label={text('证书邮箱')}><input placeholder="admin@example.com" {...form.register('cert_email')} /></Field>
            </div>
            <CertificateManager
              certificates={managedCertificates}
              tlsInbounds={tlsInbounds}
              selectedCertificate={selectedCertificate}
              selectedInboundIds={selectedInboundIds}
              preflight={preflightResult}
              certDomainsInput={certDomainsInput}
              certEmailInput={certEmailInput}
              importName={importName}
              importFullchain={importFullchain}
              importKey={importKey}
              loading={certificates.isLoading || certificateInbounds.isLoading}
              busy={preflightCert.isPending || createManagedCert.isPending || importCert.isPending || renewCerts.isPending || applyCert.isPending || deleteCert.isPending}
              onDomainsChange={setCertDomainsInput}
              onEmailChange={setCertEmailInput}
              onImportNameChange={setImportName}
              onImportFullchainChange={setImportFullchain}
              onImportKeyChange={setImportKey}
              onSelectCertificate={(id) => setSelectedCertId(id)}
              onToggleInbound={(id) => setSelectedInboundIds(toggleID(selectedInboundIds, id))}
              onPreflight={() => preflightCert.mutate()}
              onCreate={async () => (await confirm({ title: text('申请 TLS 证书？'), description: text('MiGate 将执行 HTTP-01 预检查和 ACME 签发，可能短暂占用 80 端口。') })) && createManagedCert.mutate()}
              onImport={async () => (await confirm({ title: text('导入 TLS 证书？'), description: text('导入后证书和私钥会写入 /etc/migate/certs。') })) && importCert.mutate()}
              onRenew={async () => (await confirm({ title: text('检查并续期证书？'), description: text('30 天内到期的 ACME 证书会尝试续期。') })) && renewCerts.mutate()}
              onApply={async () => (await confirm({ title: text('应用证书到入站？'), description: text('该操作会更新入站 TLS 路径并重新应用对应核心配置。') })) && applyCert.mutate()}
              onDelete={async (id) => (await confirm({ title: text('删除证书？'), description: text('仍被入站使用的证书不会被删除。'), tone: 'danger' })) && deleteCert.mutate(id)}
              text={text}
            />
          </div>
        </Card>
        <Card className="p-5">
          <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
            <h2 className="section-title">{text('服务维护')}</h2>
            <div className="flex flex-wrap gap-2">
              <button className="btn secondary" onClick={() => refreshQuery(service)}><RefreshCw className="h-4 w-4" /> {text('刷新状态')}</button>
              <button className="btn secondary" onClick={() => refreshQuery(updateCheck)}><RefreshCw className="h-4 w-4" /> {text('检查更新')}</button>
              <button className="btn secondary" onClick={() => refreshQuery(updateLogs)}><RefreshCw className="h-4 w-4" /> {text('加载更新日志')}</button>
              <SpinnerButton className="btn primary" loading={update.isPending} disabled={isUpdateInProgress(updateStatus.data?.status)} onClick={async () => (await confirm({ title: text('立即更新 MiGate？'), description: text('更新器将通过 systemd-run 在服务外执行。') })) && update.mutate()}><UploadCloud className="h-4 w-4" /> {text('立即更新')}</SpinnerButton>
            </div>
          </div>
          <div className="grid gap-4 text-sm text-panel-muted xl:grid-cols-2">
            <div className="grid gap-2">
              <div className="text-xs font-semibold uppercase tracking-wide text-panel-muted">{text('运行状态')}</div>
              <div>{service.data?.service || 'migate'} · {text(serviceLabel(service.data?.status))}</div>
              {service.data?.detail ? <div>{service.data.detail}</div> : null}
            </div>
            <div className="grid gap-2">
              <div className="text-xs font-semibold uppercase tracking-wide text-panel-muted">{text('版本更新')}</div>
              <div>{text('当前')}：{version.data?.version || updateCheck.data?.current_version || updateStatus.data?.current_version || '-'}</div>
              <div>{text('最新')}：{updateCheck.data?.latest_version || '-'}</div>
              <div>{text('可更新')}：{text(updateCheck.data?.update_available ? '是' : '否')}</div>
              <div>{text('更新状态')}：{updateStatus.data?.status || '-'}</div>
              {waitingForService ? <div>{text('正在等待服务恢复')}</div> : null}
              {updateSummary ? <div>{text(updateSummary)}</div> : null}
              {updateStatus.data?.message ? <div>{text('消息')}：{updateStatus.data.message}</div> : null}
              {updateStatus.data?.health_check ? <div>{text('健康检查')}：{updateStatus.data.health_check}</div> : null}
              <div>{text('日志路径')}：{updateLogs.data?.path || '/var/log/migate-update.log'}</div>
              {updateCheck.data?.release_url ? <a className="inline-flex w-fit items-center gap-1 text-teal-700" href={updateCheck.data.release_url} target="_blank" rel="noreferrer">{text('发布说明')} <ExternalLink className="h-3 w-3" /></a> : null}
            </div>
          </div>
          <div className="mt-4">
            <pre className="code-block core-code-block">{formatUpdateLogs(updateLogs.data, text('点击“加载更新日志”查看最近更新日志。'))}</pre>
          </div>
        </Card>
      </div>
      <Card className="p-5">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="section-title">{text('活动会话')}</h2>
            <div className="mt-1 text-xs text-panel-muted">{text('最多保留最近')} {maxActiveSessionsLabel} {text('个活动会话')}</div>
          </div>
          <SpinnerButton className="btn danger h-8" loading={revokeOthers.isPending} disabled={sessionItems.length <= 1} onClick={async () => (await confirm({ title: text('撤销其他会话？'), description: text('当前会话会保留，其他设备和浏览器需要重新登录。'), tone: 'danger' })) && revokeOthers.mutate()}>
            <ShieldX className="h-4 w-4" /> {text('撤销其他会话')}
          </SpinnerButton>
        </div>
        <div className="grid gap-2">
          {visibleSessions.map((item) => (
            <div key={item.id} className="client-row">
              <div className="min-w-0">
                <div className="font-medium">{text('会话')} {item.id_prefix}</div>
                <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-panel-muted">
                  <span>{text('最后使用')}：{item.last_used || '-'}</span>
                  <span>{text('创建')}：{item.created_at || '-'}</span>
                  <span>{text('过期')}：{item.expires_at || '-'}</span>
                </div>
              </div>
              <SpinnerButton className="btn danger h-8" loading={revoke.isPending} onClick={async () => (await confirm({ title: text('撤销该会话？'), tone: 'danger' })) && revoke.mutate(item.id)}>
                <ShieldX className="h-4 w-4" /> {text('撤销')}
              </SpinnerButton>
            </div>
          ))}
          {hiddenSessionCount > 0 || showAllSessions ? (
            <button type="button" className="btn secondary h-8 w-fit" onClick={() => setShowAllSessions((value) => !value)}>
              {showAllSessions ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
              {showAllSessions ? text('收起会话') : `${text('展开全部')} ${sessionItems.length} ${text('个会话')}`}
            </button>
          ) : null}
          {sessionItems.length === 0 ? <div className="text-sm text-panel-muted">{text('暂无会话数据')}</div> : null}
        </div>
      </Card>
    </div>
  );
}

export const defaultVisibleSessions = 5;
export const maxActiveSessionsLabel = 10;

function errorMessage(error: unknown, fallback: string) {
  return getAPIErrorMessage(error, fallback);
}

export function settingsPayload(current: Settings | undefined, values: Settings): Settings {
  return { ...current, ...values, panel_password: values.panel_password || '' };
}

export function certSettingsPayload(current: Settings | undefined, values: Settings): Settings {
  return {
    ...current,
    cert_domain: values.cert_domain || '',
    cert_email: values.cert_email || '',
    panel_password: '',
  };
}

export function certIssuePayload(values: Settings, current?: { domain?: string; email?: string }): { domain: string; email: string } {
  return {
    domain: values.cert_domain || current?.domain || '',
    email: values.cert_email || current?.email || '',
  };
}

export function parseDomains(value: string | string[]): string[] {
  const items = Array.isArray(value) ? value : value.split(/[\s,，]+/);
  return Array.from(new Set(items.map((item) => item.trim().toLowerCase()).filter(Boolean)));
}

export function certificateStatusLabel(status?: string) {
  switch (status) {
    case 'issued':
      return '有效';
    case 'expiring_soon':
      return '即将到期';
    case 'expired':
      return '已过期';
    case 'pending':
      return '处理中';
    case 'failed':
      return '失败';
    default:
      return status || '未知';
  }
}

export function certificateStatusTone(status?: string) {
  if (status === 'issued') return 'text-emerald-700 bg-emerald-50 border-emerald-200';
  if (status === 'expiring_soon') return 'text-amber-800 bg-amber-50 border-amber-200';
  if (status === 'expired' || status === 'failed') return 'text-red-700 bg-red-50 border-red-200';
  return 'text-panel-muted bg-panel-soft border-panel-line';
}

export function preflightFromAPIError(error: unknown): CertificatePreflight | null {
  if (!(error instanceof ApiError)) return null;
  const preflight = error.fields?.preflight;
  if (!preflight || typeof preflight !== 'object') return null;
  const candidate = preflight as Partial<CertificatePreflight>;
  if (typeof candidate.ok !== 'boolean' || !Array.isArray(candidate.checks)) return null;
  return {
    ok: candidate.ok,
    checks: candidate.checks.filter((check): check is CertificatePreflight['checks'][number] => !!check && typeof check === 'object' && typeof (check as { code?: unknown }).code === 'string' && typeof (check as { status?: unknown }).status === 'string'),
  };
}

export function toggleID(ids: number[], id: number) {
  return ids.includes(id) ? ids.filter((item) => item !== id) : [...ids, id];
}

function CertificateManager({
  certificates,
  tlsInbounds,
  selectedCertificate,
  selectedInboundIds,
  preflight,
  certDomainsInput,
  certEmailInput,
  importName,
  importFullchain,
  importKey,
  loading,
  busy,
  onDomainsChange,
  onEmailChange,
  onImportNameChange,
  onImportFullchainChange,
  onImportKeyChange,
  onSelectCertificate,
  onToggleInbound,
  onPreflight,
  onCreate,
  onImport,
  onRenew,
  onApply,
  onDelete,
  text,
}: {
  certificates: ManagedCertificate[];
  tlsInbounds: Inbound[];
  selectedCertificate?: ManagedCertificate;
  selectedInboundIds: number[];
  preflight: CertificatePreflight | null;
  certDomainsInput: string;
  certEmailInput: string;
  importName: string;
  importFullchain: string;
  importKey: string;
  loading: boolean;
  busy: boolean;
  onDomainsChange: (value: string) => void;
  onEmailChange: (value: string) => void;
  onImportNameChange: (value: string) => void;
  onImportFullchainChange: (value: string) => void;
  onImportKeyChange: (value: string) => void;
  onSelectCertificate: (id: number) => void;
  onToggleInbound: (id: number) => void;
  onPreflight: () => void;
  onCreate: () => void;
  onImport: () => void;
  onRenew: () => void;
  onApply: () => void;
  onDelete: (id: number) => void;
  text: (value: string) => string;
}) {
  return (
    <div className="grid gap-4">
      <div className="rounded-md border border-panel-line bg-panel-soft p-4">
        <div className="grid gap-3 md:grid-cols-2">
          <Field label={text('申请域名 / SAN')} help={text('多个域名可用逗号或空格分隔。')}>
            <input value={certDomainsInput} onChange={(event) => onDomainsChange(event.target.value)} placeholder="example.com www.example.com" />
          </Field>
          <Field label={text('ACME 邮箱')}>
            <input value={certEmailInput} onChange={(event) => onEmailChange(event.target.value)} placeholder="admin@example.com" />
          </Field>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          <SpinnerButton className="btn secondary h-8" loading={busy} onClick={onPreflight}><ShieldCheck className="h-4 w-4" /> {text('运行预检查')}</SpinnerButton>
          <SpinnerButton className="btn primary h-8" loading={busy} onClick={onCreate}><FileKey2 className="h-4 w-4" /> {text('申请证书')}</SpinnerButton>
          <SpinnerButton className="btn secondary h-8" loading={busy} onClick={onRenew}><RefreshCw className="h-4 w-4" /> {text('检查续期')}</SpinnerButton>
        </div>
        {preflight ? (
          <div className="mt-3 grid gap-2">
            {preflight.checks.map((check) => (
              <div key={`${check.code}-${check.detail}`} className="flex flex-wrap items-center gap-2 text-xs">
                <span className={`rounded border px-2 py-0.5 ${certificateStatusTone(check.status === 'failed' ? 'failed' : check.status === 'warning' ? 'expiring_soon' : 'issued')}`}>{text(check.status)}</span>
                <span className="font-medium text-panel-text">{check.code}</span>
                {check.detail ? <span className="break-all text-panel-muted">{check.detail}</span> : null}
              </div>
            ))}
          </div>
        ) : null}
      </div>

      <div className="rounded-md border border-panel-line">
        <div className="grid grid-cols-[1.2fr_0.8fr_0.7fr_auto] gap-3 border-b border-panel-line px-3 py-2 text-xs font-semibold text-panel-muted">
          <span>{text('证书')}</span>
          <span>{text('到期时间')}</span>
          <span>{text('使用')}</span>
          <span>{text('操作')}</span>
        </div>
        {loading ? <div className="p-3 text-sm text-panel-muted">{text('加载中...')}</div> : null}
        {!loading && certificates.length === 0 ? <div className="p-3 text-sm text-panel-muted">{text('暂无托管证书')}</div> : null}
        {certificates.map((cert) => (
          <div key={cert.id} className="grid grid-cols-[1.2fr_0.8fr_0.7fr_auto] gap-3 border-b border-panel-line px-3 py-3 text-sm last:border-b-0">
            <button className="min-w-0 text-left" onClick={() => onSelectCertificate(cert.id)}>
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium text-panel-text">{cert.name || cert.domains?.[0] || `#${cert.id}`}</span>
                <span className={`rounded border px-2 py-0.5 text-xs ${certificateStatusTone(cert.status)}`}>{text(certificateStatusLabel(cert.status))}</span>
              </div>
              <div className="mt-1 break-all text-xs text-panel-muted">{cert.domains?.join(', ') || '-'}</div>
              <div className="mt-1 break-all text-xs text-panel-muted">{cert.cert_path}</div>
              {cert.last_error ? <div className="mt-1 break-all text-xs text-red-600">{cert.last_error}</div> : null}
            </button>
            <div className="text-panel-muted">{formatDate(cert.not_after)}</div>
            <div className="text-panel-muted">{cert.usage_count || 0}</div>
            <button className="icon-button h-8 w-8" title={text('删除证书')} onClick={() => onDelete(cert.id)}><Trash2 className="h-4 w-4" /></button>
          </div>
        ))}
      </div>

      <div className="rounded-md border border-panel-line bg-panel-soft p-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div className="text-sm font-semibold text-panel-text">{text('应用到 TLS 入站')}</div>
          <SpinnerButton className="btn primary h-8" loading={busy} disabled={!selectedCertificate || selectedInboundIds.length === 0} onClick={onApply}><Link2 className="h-4 w-4" /> {text('应用证书')}</SpinnerButton>
        </div>
        <div className="mb-3 text-xs text-panel-muted">{text('当前证书')}：{selectedCertificate?.name || selectedCertificate?.domains?.[0] || '-'}</div>
        <div className="grid gap-2 sm:grid-cols-2">
          {tlsInbounds.map((inbound) => (
            <label key={inbound.id} className="flex items-center gap-2 rounded border border-panel-line bg-panel-surface px-3 py-2 text-sm">
              <input type="checkbox" checked={selectedInboundIds.includes(inbound.id)} onChange={() => onToggleInbound(inbound.id)} />
              <span className="min-w-0 flex-1 truncate">{inbound.remark || inbound.protocol} · {inbound.protocol} · {inbound.port}</span>
            </label>
          ))}
          {tlsInbounds.length === 0 ? <div className="text-sm text-panel-muted">{text('暂无可绑定的 TLS 入站')}</div> : null}
        </div>
      </div>

      <div className="rounded-md border border-panel-line bg-panel-soft p-4">
        <div className="grid gap-3 md:grid-cols-2">
          <Field label={text('导入名称')}><input value={importName} onChange={(event) => onImportNameChange(event.target.value)} placeholder="example.com" /></Field>
          <div />
          <Field label={text('Fullchain PEM')}><textarea className="min-h-28" value={importFullchain} onChange={(event) => onImportFullchainChange(event.target.value)} /></Field>
          <Field label={text('Private Key PEM')}><textarea className="min-h-28" value={importKey} onChange={(event) => onImportKeyChange(event.target.value)} /></Field>
        </div>
        <div className="mt-3 flex justify-end">
          <SpinnerButton className="btn secondary h-8" loading={busy} disabled={!importFullchain || !importKey} onClick={onImport}><UploadCloud className="h-4 w-4" /> {text('导入证书')}</SpinnerButton>
        </div>
      </div>
    </div>
  );
}

function formatDate(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString();
}

export function updateStatusRefetchInterval(status?: string, watching = false) {
  return watching || isUpdateInProgress(status) ? 5000 : false;
}

export function updateDependencyRefetchInterval(watching = false) {
  return watching ? 5000 : false;
}

export function isUpdateInProgress(status?: string) {
  return ['pending', 'running', 'updating', 'downloading', 'installing', 'restarting'].includes(String(status || '').toLowerCase());
}

export function isUpdateTerminal(status?: string) {
  return ['started', 'failed', 'completed', 'idle'].includes(String(status || '').toLowerCase());
}

export function updateStatusSummaryKey(status?: Pick<UpdateStatus, 'status' | 'rolled_back' | 'rollback_status'>) {
  if (!status) return '';
  if (String(status.status).toLowerCase() === 'failed' && status.rolled_back && status.rollback_status === 'restored') {
    return '升级失败，已回滚，服务已恢复';
  }
  if (String(status.status).toLowerCase() === 'completed') {
    return '升级成功，服务已可用';
  }
  return '';
}

export function formatUpdateLogs(data: { logs?: string; lines?: string[] } | undefined, emptyMessage: string): string {
  if (!data) return emptyMessage;
  if (Array.isArray(data.lines)) return data.lines.join('\n');
  return data.logs || emptyMessage;
}

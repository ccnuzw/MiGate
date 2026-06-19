import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ChevronDown, ChevronUp, ExternalLink, RefreshCw, RotateCcw, Save, ShieldX, UploadCloud } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { getAPIErrorMessage } from '../api/client';
import { api } from '../api/endpoints';
import type { Settings } from '../api/types';
import { Card, Field, LoadingBlock, SpinnerButton, useConfirm, useToast } from '../components/ui';
import { serviceLabel } from '../lib/format';
import { useI18n } from '../lib/i18n';
import { refreshQueries, refreshQuery, refreshSessionDependencies, refreshSettingsDependencies, refreshUpdateDependencies } from '../lib/queryInvalidation';
import { PageTitle } from './OverviewPage';

export default function SettingsPage() {
  const queryClient = useQueryClient();
  const confirm = useConfirm();
  const { showToast } = useToast();
  const { text } = useI18n();
  const [watchUpdateStatus, setWatchUpdateStatus] = useState(false);
  const [showAllSessions, setShowAllSessions] = useState(false);
  const session = useQuery({ queryKey: ['session'], queryFn: api.session, staleTime: 5 * 60_000 });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings, retry: false, staleTime: 60_000 });
  const cert = useQuery({ queryKey: ['cert-status'], queryFn: api.certStatus, retry: false, staleTime: 60_000 });
  const updateCheck = useQuery({ queryKey: ['update-check'], queryFn: api.updateCheck, enabled: false });
  const updateStatus = useQuery({
    queryKey: ['update-status'],
    queryFn: api.updateStatus,
    refetchInterval: (query) => updateStatusRefetchInterval(query.state.data?.status, watchUpdateStatus),
    staleTime: 30_000,
  });
  const updateLogs = useQuery({ queryKey: ['update-logs'], queryFn: api.updateLogs, enabled: false });
  const sessions = useQuery({ queryKey: ['sessions'], queryFn: api.sessions, retry: false, staleTime: 60_000 });
  const service = useQuery({ queryKey: ['service-status'], queryFn: api.serviceStatus, retry: false, staleTime: 30_000 });
  const form = useForm<Settings>({ values: settings.data || {} });
  const certDomain = form.watch('cert_domain') || cert.data?.domain || '';
  const certEmail = form.watch('cert_email') || cert.data?.email || '';
  useEffect(() => {
    if (watchUpdateStatus && isUpdateTerminal(updateStatus.data?.status)) {
      setWatchUpdateStatus(false);
    }
  }, [updateStatus.data?.status, watchUpdateStatus]);
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
              <SpinnerButton className="btn primary" loading={issueCert.isPending} disabled={!certDomain || !certEmail} onClick={async () => (await confirm({ title: text('获取 TLS 证书？'), description: text('该操作会调用 acme.sh 并可能占用 80 端口。') })) && issueCert.mutate()}>{text('获取证书')}</SpinnerButton>
            </div>
          </div>
          <div className="grid gap-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label={text('证书域名')}><input placeholder="example.com" {...form.register('cert_domain')} /></Field>
              <Field label={text('证书邮箱')}><input placeholder="admin@example.com" {...form.register('cert_email')} /></Field>
            </div>
            <div className="grid gap-2 text-sm text-panel-muted">
              <div>{text('状态')}：{text(cert.data?.issued ? '已获取' : cert.data?.domain ? '未获取' : '未配置')}</div>
              <div>{text('域名')}：{cert.data?.domain || certDomain || '-'}</div>
              <div className="break-all">{text('证书')}：{cert.data?.cert_path || '-'}</div>
              <div className="break-all">{text('私钥')}：{cert.data?.key_path || '-'}</div>
            </div>
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
              <div>{text('当前')}：{updateCheck.data?.current_version || '-'}</div>
              <div>{text('最新')}：{updateCheck.data?.latest_version || '-'}</div>
              <div>{text('可更新')}：{text(updateCheck.data?.update_available ? '是' : '否')}</div>
              <div>{text('更新状态')}：{updateStatus.data?.status || '-'}</div>
              {updateStatus.data?.message ? <div>{text('消息')}：{updateStatus.data.message}</div> : null}
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

export function updateStatusRefetchInterval(status?: string, watching = false) {
  return watching || isUpdateInProgress(status) ? 5000 : false;
}

export function isUpdateInProgress(status?: string) {
  return ['pending', 'running', 'updating', 'downloading', 'installing'].includes(String(status || '').toLowerCase());
}

export function isUpdateTerminal(status?: string) {
  return ['started', 'restarting', 'failed', 'completed', 'idle'].includes(String(status || '').toLowerCase());
}

export function formatUpdateLogs(data: { logs?: string; lines?: string[] } | undefined, emptyMessage: string): string {
  if (!data) return emptyMessage;
  if (Array.isArray(data.lines)) return data.lines.join('\n');
  return data.logs || emptyMessage;
}

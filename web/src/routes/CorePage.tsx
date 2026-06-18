import { useMutation, useQuery } from '@tanstack/react-query';
import { AlertTriangle, CheckCircle2, Download, Play, RefreshCw, ShieldCheck, Trash2, XCircle } from 'lucide-react';
import { useState } from 'react';
import { ApiError } from '../api/client';
import { api } from '../api/endpoints';
import type { CoreActionResponse, CoreConfigPreview, CoreDiagnosticAction, CoreDiagnostics, CoreStatus } from '../api/types';
import { Card, LoadingBlock, SpinnerButton, useConfirm, useToast } from '../components/ui';
import { formatBytes, serviceLabel, versionLabel } from '../lib/format';
import { useI18n } from '../lib/i18n';
import { usePageVisible } from '../lib/visibility';
import { PageTitle } from './OverviewPage';

export default function CorePage({ core }: { core: 'xray' | 'singbox' }) {
  const visible = usePageVisible();
  const { showToast } = useToast();
  const { text } = useI18n();
  const confirm = useConfirm();
  const label = core === 'xray' ? 'Xray' : 'sing-box';
  const endpoints = core === 'xray'
    ? { status: api.xrayStatus, version: api.xrayVersion, config: api.xrayConfig, configPreview: api.xrayConfigPreview, diagnostics: api.xrayDiagnostics, logs: api.xrayLogs, validate: api.xrayValidate, apply: api.xrayApply, install: api.xrayInstall, uninstall: api.xrayUninstall }
    : { status: api.singboxStatus, version: api.singboxVersion, config: api.singboxConfig, configPreview: api.singboxConfigPreview, diagnostics: api.singboxDiagnostics, logs: api.singboxLogs, validate: api.singboxValidate, apply: api.singboxApply, install: api.singboxInstall, uninstall: api.singboxUninstall };
  const [lastResult, setLastResult] = useState<{ ok: boolean; message: string; detail?: string } | null>(null);
  const statusQuery = useQuery({ queryKey: [core, 'status'], queryFn: endpoints.status, refetchInterval: coreStatusRefetchInterval(visible), staleTime: 10_000 });
  const versionQuery = useQuery({ queryKey: [core, 'version'], queryFn: endpoints.version, retry: false, staleTime: 10 * 60_000 });
  const configQuery = useQuery({ queryKey: [core, 'config'], queryFn: endpoints.config, staleTime: 60_000 });
  const configPreviewQuery = useQuery({ queryKey: [core, 'config-preview'], queryFn: endpoints.configPreview, staleTime: 60_000 });
  const diagnosticsQuery = useQuery({ queryKey: [core, 'diagnostics'], queryFn: endpoints.diagnostics, staleTime: 20_000 });
  const logsQuery = useQuery({ queryKey: [core, 'logs'], queryFn: endpoints.logs, enabled: false });
  const validate = useMutation({
    mutationFn: endpoints.validate,
    onSuccess: (data) => {
      const message = data.valid ? `${label} 生成校验通过` : (data.error || `${label} 生成校验失败`);
      setLastResult({ ok: data.valid, message, detail: validationDetail(data) });
      showToast(text(message), data.valid ? 'success' : 'error');
    },
    onError: (error) => {
      const message = errorMessage(error, `${label} 生成校验失败`);
      setLastResult({ ok: false, message });
      showToast(message, 'error');
    },
  });
  const apply = useMutation({
    mutationFn: endpoints.apply,
    onSuccess: (data) => {
      const result = coreActionResult(data, `${label} 配置已应用`);
      setLastResult({ ok: result.ok, message: result.message, detail: result.detail });
      showToast(text(result.message), result.tone || (result.ok ? 'success' : 'error'));
      statusQuery.refetch();
      diagnosticsQuery.refetch();
      if (result.ok) {
        configQuery.refetch();
        configPreviewQuery.refetch();
      }
    },
    onError: (error) => setActionError(error, `${label} 应用失败`, setLastResult, showToast),
  });
  const install = useMutation({
    mutationFn: endpoints.install,
    onSuccess: (data) => {
      const result = coreActionResult(data, `${label} 安装命令已执行`);
      setLastResult({ ok: result.ok, message: result.message, detail: result.detail });
      showToast(text(result.message), result.ok ? 'success' : 'error');
      statusQuery.refetch();
      diagnosticsQuery.refetch();
    },
    onError: (error) => setActionError(error, `${label} 安装失败`, setLastResult, showToast),
  });
  const uninstall = useMutation({
    mutationFn: endpoints.uninstall,
    onSuccess: (data) => {
      const result = coreActionResult(data, `${label} 卸载命令已执行`);
      setLastResult({ ok: result.ok, message: result.message, detail: result.detail });
      showToast(text(result.message), result.ok ? 'success' : 'error');
      statusQuery.refetch();
      diagnosticsQuery.refetch();
    },
    onError: (error) => setActionError(error, `${label} 卸载失败`, setLastResult, showToast),
  });
  if (statusQuery.isLoading) return <LoadingBlock />;
  const status = statusQuery.data;
  const installed = isCoreInstalled(status);
  const installActionLabel = installed ? '升级/重装核心' : '安装核心';
  return (
    <div className={`page-stack core-page core-page-${core}`}>
      <PageTitle
        title={text(`${label} 配置`)}
        description={text('查看核心运行状态、配置预览、日志和系统级操作。')}
        action={
          <div className="core-action-row">
            <button className="btn secondary" onClick={() => { statusQuery.refetch(); versionQuery.refetch(); diagnosticsQuery.refetch(); configPreviewQuery.refetch(); }}><RefreshCw className="h-4 w-4" /> {text('刷新')}</button>
            <SpinnerButton className="btn secondary" loading={validate.isPending} onClick={() => validate.mutate()}><ShieldCheck className="h-4 w-4" /> {text('生成校验')}</SpinnerButton>
            <SpinnerButton className="btn secondary" loading={install.isPending} onClick={async () => (await confirm({ title: text(`${installActionLabel} ${label}？`), description: text(installed ? '该操作会重新执行安装脚本，通常用于升级或修复当前核心。' : '该操作会执行系统安装命令。') })) && install.mutate()}><Download className="h-4 w-4" /> {text(installActionLabel)}</SpinnerButton>
            <SpinnerButton className="btn danger" loading={uninstall.isPending} disabled={!installed} title={text(installed ? '卸载核心' : '核心未安装')} onClick={async () => installed && (await confirm({ title: text(`卸载 ${label} 核心？`), description: text('该操作会删除或停用系统服务。'), tone: 'danger' })) && uninstall.mutate()}><Trash2 className="h-4 w-4" /> {text('卸载核心')}</SpinnerButton>
            <SpinnerButton className="btn primary" loading={apply.isPending} onClick={async () => (await confirm({ title: text(`应用 ${label} 配置？`), description: text('该操作会重新生成并应用核心配置。') })) && apply.mutate()}><Play className="h-4 w-4" /> {text('应用')}</SpinnerButton>
          </div>
        }
      />
      <div className="metric-grid core-metric-grid">
        {coreStatusMetrics(status, versionQuery.data?.version).map((metric) => (
          <CoreMetric key={metric.label} label={text(metric.label)} value={text(metric.value)} />
        ))}
      </div>
      {lastResult ? (
        <Card className={`core-card p-4 ${lastResult.ok ? 'border-emerald-200 bg-emerald-50' : 'border-red-200 bg-red-50'}`}>
          <div className={`flex items-start gap-3 text-sm ${lastResult.ok ? 'text-emerald-800' : 'text-red-700'}`}>
            {lastResult.ok ? <CheckCircle2 className="mt-0.5 h-4 w-4" /> : <XCircle className="mt-0.5 h-4 w-4" />}
            <div className="min-w-0">
              <div className="font-medium">{text(lastResult.message)}</div>
              {lastResult.detail ? <pre className="mt-2 whitespace-pre-wrap break-words text-xs leading-5">{lastResult.detail}</pre> : null}
            </div>
          </div>
        </Card>
      ) : null}
      {status?.commands_executed?.length ? (
        <Card className="core-card p-5">
          <h2 className="section-title mb-3">{text('最近命令')}</h2>
          <pre className="code-block core-code-block">{status.commands_executed.join('\n')}</pre>
        </Card>
      ) : null}
      <CoreDiagnosticsPanel diagnostics={diagnosticsQuery.data} loading={diagnosticsQuery.isLoading} error={diagnosticsQuery.error} onRefresh={() => diagnosticsQuery.refetch()} text={text} />
      <CorePortDiagnostics status={status} diagnostics={diagnosticsQuery.data} text={text} />
      <CoreConfigSync preview={configPreviewQuery.data} loading={configPreviewQuery.isLoading} error={configPreviewQuery.error} onRefresh={() => { configQuery.refetch(); configPreviewQuery.refetch(); }} text={text} />
      <Card className="core-card p-5">
        <div className="core-card-header mb-3 flex items-center justify-between gap-3">
          <h2 className="section-title">{text('配置预览')}</h2>
          <button className="btn secondary h-8" onClick={() => { configQuery.refetch(); configPreviewQuery.refetch(); }}>{text('刷新配置')}</button>
        </div>
        <pre className="code-block core-code-block">{JSON.stringify(configQuery.data || {}, null, 2)}</pre>
      </Card>
      <Card className="core-card p-5">
        <div className="core-card-header mb-3 flex items-center justify-between gap-3">
          <h2 className="section-title">{text('日志')}</h2>
          <button className="btn secondary h-8" onClick={() => logsQuery.refetch()}>{text('加载日志')}</button>
        </div>
        <pre className="code-block core-code-block">{formatLogs(logsQuery.data, text('点击“加载日志”查看最近日志。'))}</pre>
      </Card>
    </div>
  );
}

function CorePortDiagnostics({ status, diagnostics, text }: { status?: CoreStatus; diagnostics?: CoreDiagnostics; text: (value: string) => string }) {
  const ports = coreListeningDiagnostics(status, diagnostics);
  if (!ports.length) return null;
  const missing = ports.some((port) => !port.listening);
  return (
    <Card className={`core-card p-5 ${missing ? 'border-amber-200 bg-amber-50' : ''}`}>
      <div className="core-card-header mb-3 flex items-center justify-between gap-3">
        <h2 className="section-title">{text('端口监听诊断')}</h2>
        {missing ? <span className="text-sm font-medium text-amber-700">{text('存在未监听端口')}</span> : null}
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead className="text-xs text-panel-muted">
            <tr>
              <th className="py-2 pr-3">{text('入站 ID')}</th>
              <th className="py-2 pr-3">{text('协议')}</th>
              <th className="py-2 pr-3">{text('端口')}</th>
              <th className="py-2 pr-3">UDP/TCP</th>
              <th className="py-2 pr-3">{text('传输')}</th>
              <th className="py-2 pr-3">{text('安全')}</th>
              <th className="py-2 pr-3">{text('详情')}</th>
              <th className="py-2 pr-3">{text('监听')}</th>
            </tr>
          </thead>
          <tbody>
            {ports.map((port) => (
              <tr key={`${port.inboundId}-${port.port}-${port.transport}`} className={!port.listening ? 'font-medium text-amber-800' : ''}>
                <td className="py-2 pr-3">{port.inboundId}</td>
                <td className="py-2 pr-3">{port.protocol}</td>
                <td className="py-2 pr-3">{port.port}</td>
                <td className="py-2 pr-3">{port.transport.toUpperCase()}</td>
                <td className="py-2 pr-3">{port.network || '-'}</td>
                <td className="py-2 pr-3">{port.security || '-'}</td>
                <td className="py-2 pr-3">{port.detail || '-'}</td>
                <td className="py-2 pr-3">{text(port.listening ? '正在监听' : '未监听')}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  );
}

function CoreDiagnosticsPanel({ diagnostics, loading, error, onRefresh, text }: { diagnostics?: CoreDiagnostics; loading: boolean; error?: unknown; onRefresh: () => void; text: (value: string) => string }) {
  const summary = coreDiagnosticsSummary(diagnostics, loading, error);
  const toneClass = summary.tone === 'error' ? 'border-red-200 bg-red-50' : summary.tone === 'warning' ? 'border-amber-200 bg-amber-50' : 'border-emerald-200 bg-emerald-50';
  const iconClass = summary.tone === 'error' ? 'text-red-700' : summary.tone === 'warning' ? 'text-amber-700' : 'text-emerald-700';
  const checks = coreDiagnosticChecks(diagnostics);
  const actions = coreDiagnosticActions(diagnostics);
  return (
    <Card className={`core-card p-5 ${toneClass}`}>
      <div className="core-card-header mb-4 flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          {summary.tone === 'ok' ? <CheckCircle2 className={`h-5 w-5 ${iconClass}`} /> : <AlertTriangle className={`h-5 w-5 ${iconClass}`} />}
          <div className="min-w-0">
            <h2 className="section-title">{text('诊断')}</h2>
            <div className={`mt-1 text-sm font-medium ${iconClass}`}>{text(summary.label)}</div>
          </div>
        </div>
        <button className="btn secondary h-8" onClick={onRefresh}><RefreshCw className="h-4 w-4" /> {text('刷新诊断')}</button>
      </div>
      {summary.detail ? <div className="mb-3 text-sm text-panel-muted">{text(summary.detail)}</div> : null}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        {checks.map((check) => (
          <div key={check.label} className="rounded border border-panel-border bg-white/70 p-3">
            <div className="text-xs text-panel-muted">{text(check.label)}</div>
            <div className={`mt-1 text-sm font-medium ${check.ok ? 'text-emerald-700' : 'text-amber-800'}`}>{text(check.value)}</div>
          </div>
        ))}
      </div>
      {diagnostics?.missing_listeners?.length ? (
        <div className="mt-4">
          <div className="mb-2 text-xs font-medium text-panel-muted">{text('未监听端口')}</div>
          <div className="flex flex-wrap gap-2">
            {diagnostics.missing_listeners.map((listener) => <span key={`${listener.inbound_id}-${listener.port}`} className="rounded border border-amber-300 bg-amber-100 px-2 py-1 text-xs font-medium text-amber-900">{listener.port}/{String(listener.transport || listener.network || 'tcp').toLowerCase()}</span>)}
          </div>
        </div>
      ) : null}
      {diagnostics?.config_error ? (
        <div className="mt-4">
          <div className="mb-2 text-xs font-medium text-panel-muted">{text('配置校验错误')}</div>
          <pre className="code-block core-code-block">{diagnostics.config_error}</pre>
        </div>
      ) : null}
      {diagnostics?.warnings?.length ? <DiagnosticList title={text('警告')} items={diagnostics.warnings.map((item) => text(diagnosticWarningLabel(item)))} /> : null}
      {actions.length ? <DiagnosticActionList title={text('建议操作')} actions={actions} text={text} /> : diagnostics?.suggestions?.length ? <DiagnosticList title={text('建议操作')} items={diagnostics.suggestions.map((item) => text(item))} /> : null}
      {diagnostics?.recent_logs?.length ? (
        <div className="mt-4">
          <div className="mb-2 text-xs font-medium text-panel-muted">{text('最近日志摘要')}</div>
          <pre className="code-block core-code-block">{diagnostics.recent_logs.slice(-8).join('\n')}</pre>
        </div>
      ) : null}
    </Card>
  );
}

function DiagnosticList({ title, items }: { title: string; items: string[] }) {
  return (
    <div className="mt-4">
      <div className="mb-2 text-xs font-medium text-panel-muted">{title}</div>
      <ul className="space-y-1 text-sm text-panel-text">
        {items.map((item) => <li key={item} className="break-words">- {item}</li>)}
      </ul>
    </div>
  );
}

function DiagnosticActionList({ title, actions, text }: { title: string; actions: CoreDiagnosticAction[]; text: (value: string) => string }) {
  return (
    <div className="mt-4">
      <div className="mb-2 text-xs font-medium text-panel-muted">{title}</div>
      <div className="grid gap-2">
        {actions.map((action) => {
          const formatted = formatDiagnosticAction(action);
          return (
            <div key={`${action.code}-${action.inbound_id || 0}-${action.port || 0}-${action.command || action.message}`} className="rounded border border-panel-border bg-white/70 p-3 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className={`rounded px-2 py-0.5 text-xs font-medium ${diagnosticActionToneClass(action.severity)}`}>{text(formatted.severity)}</span>
                <span className="text-xs text-panel-muted">{text(formatted.category)}</span>
                {formatted.target ? <span className="text-xs text-panel-muted">{formatted.target}</span> : null}
              </div>
              <div className="mt-2 break-words text-panel-text">{text(formatted.message)}</div>
              {formatted.command ? <code className="mt-2 block overflow-x-auto rounded border border-panel-border bg-panel-soft px-2 py-1 text-xs text-panel-text">{formatted.command}</code> : null}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function CoreConfigSync({ preview, loading, error, onRefresh, text }: { preview?: CoreConfigPreview; loading: boolean; error?: unknown; onRefresh: () => void; text: (value: string) => string }) {
  const state = configSyncState(preview, loading, error);
  return (
    <Card className={`core-card p-5 ${state.ok === false ? 'border-amber-200 bg-amber-50' : ''}`}>
      <div className="core-card-header mb-3 flex items-center justify-between gap-3">
        <h2 className="section-title">{text('配置同步状态')}</h2>
        <button className="btn secondary h-8" onClick={onRefresh}>{text('刷新配置')}</button>
      </div>
      <div className={`text-sm font-medium ${state.ok === false ? 'text-amber-800' : 'text-emerald-700'}`}>{text(state.label)}</div>
      {state.detail ? <div className="mt-2 text-xs text-panel-muted">{state.detail}</div> : null}
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <div>
          <div className="mb-2 text-xs font-medium text-panel-muted">{text('当前磁盘配置')}</div>
          <pre className="code-block core-code-block">{JSON.stringify(preview?.disk?.config || preview?.disk || {}, null, 2)}</pre>
        </div>
        <div>
          <div className="mb-2 text-xs font-medium text-panel-muted">{text('数据库生成配置')}</div>
          <pre className="code-block core-code-block">{JSON.stringify(preview?.generated?.config || preview?.generated || {}, null, 2)}</pre>
        </div>
      </div>
    </Card>
  );
}

function CoreMetric({ label, value }: { label: string; value: string }) {
  return (
    <Card className="core-metric-card p-4">
      <div className="text-sm text-panel-muted">{label}</div>
      <div className="mt-2 truncate text-xl font-semibold" title={value}>{value}</div>
    </Card>
  );
}

export function coreStatusMetrics(status?: CoreStatus, fallbackVersion?: string): Array<{ label: string; value: string }> {
  const installed = isCoreInstalled(status);
  return [
    { label: '安装', value: installed ? '已安装' : '未安装' },
    { label: '托管', value: status?.managed ? '已托管' : '未托管' },
    { label: '状态', value: serviceLabel(status?.status) },
    { label: '版本', value: versionLabel(status?.version || fallbackVersion) },
    { label: '内存', value: formatBytes(status?.memory_rss_bytes) },
    { label: '运行时长', value: status?.uptime || '-' },
    { label: '连接', value: String(status?.active_connections || 0) },
    { label: '配置路径', value: status?.config_path || '-' },
  ];
}

export function singboxListeningDiagnostics(status?: CoreStatus) {
  return coreListeningDiagnostics(status);
}

export function coreListeningDiagnostics(status?: CoreStatus, diagnostics?: CoreDiagnostics) {
  const source = status?.listening_ports?.length ? status.listening_ports : diagnostics?.expected_listeners || [];
  return source.map((item) => {
    const transport = String(item.transport || item.network || '-');
    const network = String(item.network || '');
    const detail = listenerDetail(item);
    return {
      inboundId: Number(item.inbound_id || 0),
      protocol: item.protocol || '-',
      port: Number(item.port || 0),
      transport,
      ...(network && network !== transport ? { network } : {}),
      ...(item.security ? { security: String(item.security) } : {}),
      ...(detail !== '-' ? { detail } : {}),
      listening: Boolean(item.listening),
    };
  });
}

function listenerDetail(item: { path?: string; grpc_service_name?: string }) {
  if (item.grpc_service_name) return item.grpc_service_name;
  if (item.path) return item.path;
  return '-';
}

export function configSyncState(preview?: CoreConfigPreview, loading = false, error?: unknown): { ok?: boolean; label: string; detail?: string } {
  if (loading) return { label: '正在检查配置同步状态' };
  if (error) return { ok: false, label: '生成配置预览失败', detail: error instanceof Error ? error.message : String(error) };
  if (!preview) return { label: '配置同步状态未知' };
  if (preview.in_sync) return { ok: true, label: '磁盘配置与数据库生成配置一致', detail: preview.generated?.hash || preview.disk?.hash };
  const reason = preview.reason ? `原因：${configSyncReasonLabel(preview.reason)}` : '';
  const hashDetail = [preview.disk?.hash ? `disk: ${preview.disk.hash}` : '', preview.generated?.hash ? `generated: ${preview.generated.hash}` : ''].filter(Boolean).join(' · ');
  const detail = [reason, preview.disk?.detail || preview.generated?.detail || hashDetail].filter(Boolean).join(' · ');
  return { ok: false, label: '磁盘配置与数据库生成配置不一致', detail };
}

export function configSyncReasonLabel(reason?: string): string {
  switch (reason) {
    case 'disk_missing':
      return '磁盘配置不存在';
    case 'generated_build_failed':
      return '数据库生成配置失败';
    case 'hash_mismatch':
      return '配置 hash 不一致';
    case 'disk_parse_failed':
      return '磁盘配置解析失败';
    default:
      return reason || '未知原因';
  }
}

export function singboxDiagnosticsSummary(diagnostics?: CoreDiagnostics, loading = false, error?: unknown): { tone: 'ok' | 'warning' | 'error'; label: string; detail?: string } {
  return coreDiagnosticsSummary(diagnostics, loading, error);
}

export function coreDiagnosticsSummary(diagnostics?: CoreDiagnostics, loading = false, error?: unknown): { tone: 'ok' | 'warning' | 'error'; label: string; detail?: string } {
  if (loading) return { tone: 'warning', label: '正在加载诊断' };
  if (error) return { tone: 'error', label: '诊断加载失败', detail: error instanceof Error ? error.message : String(error) };
  if (!diagnostics) return { tone: 'warning', label: '诊断状态未知' };
  const hardError = !diagnostics.installed || diagnostics.service_status === 'not_installed' || !diagnostics.config_exists || !diagnostics.config_valid || diagnostics.service_status === 'stopped';
  if (hardError) return { tone: 'error', label: '错误', detail: diagnostics.config_error || diagnostics.warnings?.[0] };
  if (diagnostics.warnings?.length || diagnostics.missing_listeners?.length || !diagnostics.disk_generated_in_sync || diagnostics.service_status === 'not_managed') return { tone: 'warning', label: '警告', detail: diagnostics.warnings?.[0] };
  return { tone: 'ok', label: '正常' };
}

export function singboxDiagnosticChecks(diagnostics?: CoreDiagnostics): Array<{ label: string; value: string; ok: boolean }> {
  return coreDiagnosticChecks(diagnostics);
}

export function coreDiagnosticChecks(diagnostics?: CoreDiagnostics): Array<{ label: string; value: string; ok: boolean }> {
  return [
    { label: '安装', value: diagnostics?.installed ? '已安装' : '未安装', ok: Boolean(diagnostics?.installed) },
    { label: 'systemd 托管', value: diagnostics?.managed ? diagnostics.service || '已托管' : '未托管', ok: Boolean(diagnostics?.managed) },
    { label: '服务状态', value: serviceLabel(diagnostics?.service_status), ok: diagnostics?.service_status === 'running' },
    { label: '配置校验', value: diagnostics?.config_valid ? '通过' : '失败', ok: Boolean(diagnostics?.config_valid) },
    { label: '配置同步', value: diagnostics?.disk_generated_in_sync ? '一致' : configSyncReasonLabel(diagnostics?.sync_reason), ok: Boolean(diagnostics?.disk_generated_in_sync) },
    { label: '端口监听', value: diagnostics?.missing_listeners?.length ? `缺失 ${diagnostics.missing_listeners.length} 个` : '完整', ok: !diagnostics?.missing_listeners?.length },
  ];
}

export function coreDiagnosticActions(diagnostics?: CoreDiagnostics): CoreDiagnosticAction[] {
  const actions = diagnostics?.actions?.length ? diagnostics.actions : diagnostics?.suggestion_details || [];
  return Array.isArray(actions) ? actions.filter((action) => Boolean(action?.code && action?.message)) : [];
}

export function diagnosticSuggestionItems(diagnostics?: CoreDiagnostics): string[] {
  const actions = coreDiagnosticActions(diagnostics);
  if (actions.length) return actions.map((action) => formatDiagnosticAction(action).summary);
  return diagnostics?.suggestions || [];
}

export function formatDiagnosticAction(action: CoreDiagnosticAction): { severity: string; category: string; message: string; command?: string; target?: string; summary: string } {
  const severity = diagnosticSeverityLabel(action.severity);
  const category = diagnosticCategoryLabel(action.category);
  const target = [action.inbound_id ? `入站 ${action.inbound_id}` : '', action.port ? `端口 ${action.port}` : ''].filter(Boolean).join(' · ');
  const command = String(action.command || '').trim() || undefined;
  const message = action.message || diagnosticWarningLabel(action.code);
  const summary = [severity, category, target, message, command ? `命令：${command}` : ''].filter(Boolean).join(' · ');
  return { severity, category, message, command, target: target || undefined, summary };
}

function diagnosticSeverityLabel(severity?: string): string {
  switch (severity) {
    case 'error':
      return '错误';
    case 'warning':
      return '警告';
    case 'info':
      return '提示';
    default:
      return severity || '提示';
  }
}

function diagnosticCategoryLabel(category?: string): string {
  switch (category) {
    case 'service':
      return '服务';
    case 'config':
      return '配置';
    case 'listener':
      return '监听';
    case 'log':
      return '日志';
    case 'security':
      return '安全';
    case 'routing':
      return '路由';
    default:
      return category || '诊断';
  }
}

function diagnosticActionToneClass(severity?: string): string {
  switch (severity) {
    case 'error':
      return 'bg-red-100 text-red-700';
    case 'warning':
      return 'bg-amber-100 text-amber-800';
    default:
      return 'bg-sky-100 text-sky-700';
  }
}

export function diagnosticWarningLabel(warning: string): string {
  const labels: Record<string, string> = {
    singbox_not_installed: 'sing-box 未安装',
    singbox_not_systemd_managed: 'sing-box 未被 systemd 托管',
    legacy_migate_singbox_service: '正在使用 legacy migate-singbox.service',
    singbox_service_not_running: 'sing-box 服务未运行',
    singbox_config_missing: '配置文件不存在',
    singbox_config_invalid: 'sing-box check 失败',
    singbox_config_out_of_sync: '磁盘配置与数据库生成配置不一致',
    singbox_missing_listeners: '服务运行但入站端口未监听',
    singbox_inbound_without_enabled_clients: '存在启用入站但没有启用客户端',
    singbox_client_credentials_missing: 'Hysteria2/TUIC 缺少可用客户端凭据',
    shadowtls_handshake_missing: 'ShadowTLS 缺少 handshake/SNI',
    singbox_route_outbound_unavailable: '路由规则引用不可用于 sing-box 的出站',
    singbox_stats_unsupported: 'sing-box 二进制不支持当前统计特性',
    singbox_stats_capability_check_failed: 'sing-box 二进制特性检测失败',
    singbox_generated_config_build_failed: '数据库生成 sing-box 配置失败',
    xray_not_installed: 'Xray 未安装',
    xray_not_systemd_managed: 'Xray 未被 systemd 托管',
    xray_service_not_running: 'Xray 服务未运行',
    xray_config_missing: '配置文件不存在',
    xray_config_invalid: 'xray run -test 失败',
    xray_config_out_of_sync: '磁盘配置与数据库生成配置不一致',
    xray_missing_listeners: '服务运行但入站端口未监听',
    xray_inbound_without_enabled_clients: '存在启用入站但没有启用客户端',
    xray_ws_path_invalid: 'WS/H2 path 配置无效',
    xray_grpc_service_name_default: 'gRPC serviceName 将使用默认值',
    xray_grpc_service_name_invalid: 'gRPC serviceName 配置无效',
    xray_xhttp_path_invalid: 'XHTTP path 配置无效',
    xray_reality_settings_incomplete: 'REALITY 配置不完整',
    xray_tls_certificate_missing: 'TLS 证书配置缺失',
    xray_shadowsocks_credentials_missing: 'Shadowsocks 2022 缺少可用凭据',
    xray_route_outbound_unavailable: '路由规则引用不可用于 Xray 的出站',
    xray_generated_config_build_failed: '数据库生成 Xray 配置失败',
  };
  return labels[warning] || warning;
}

function formatLogs(data: { logs?: string; lines?: string[] } | undefined, emptyMessage: string): string {
  if (!data) return emptyMessage;
  if (Array.isArray(data.lines)) return data.lines.join('\n');
  return data.logs || JSON.stringify(data, null, 2);
}

export function coreActionResult(data: CoreActionResponse, fallback: string): { ok: boolean; message: string; detail?: string; tone?: 'success' | 'error' | 'info' } {
  const status = normalizeStatus(data.status);
  const xrayStatus = normalizeStatus(data.xray?.status);
  const singboxReason = data.singbox?.reason === 'not_needed' ? '' : data.singbox?.reason;
  const hasNestedCoreResult = Boolean(data.xray || data.singbox);
  const error = data.error || data.xray?.error || data.xray?.detail || data.xray?.error_output || data.singbox?.error || singboxReason || data.reason;
  const failed =
    isFailureStatus(status) ||
    isFailureStatus(xrayStatus) ||
    data.xray?.applied === false ||
    (!hasNestedCoreResult && data.applied === false) ||
    (data.singbox?.applied === false && data.singbox.reason !== 'not_needed') ||
    Boolean(data.error || data.xray?.error || data.xray?.error_output || data.singbox?.error);
  const listenerWarning = firstListenerWarning(allWarnings(data.post_apply_warnings, data.xray?.post_apply_warnings, data.singbox?.post_apply_warnings, data.warnings, data.xray?.warnings, data.singbox?.warnings));
  const message = failed
    ? error || data.xray?.status || data.status || fallback
    : listenerWarning || (
      data.xray?.status
        ? `${fallback}：${data.xray.status}`
        : data.singbox?.applied === true || data.applied === true
          ? fallback
          : data.status || fallback
    );
  const detail = commandDetail(data, message);
  const tone = listenerWarning && !failed ? 'info' : undefined;
  return withDetail({ ok: !failed, message, tone }, detail);
}

function normalizeStatus(status?: string): string {
  return String(status || '').toLowerCase();
}

export function coreStatusRefetchInterval(visible: boolean) {
  return visible ? 12000 : false;
}

export function isCoreInstalled(status?: { installed?: boolean; version?: string; status?: string }): boolean {
  if (typeof status?.installed === 'boolean') return status.installed;
  const version = String(status?.version || '').trim().toLowerCase();
  if (version && version !== 'unknown' && version !== 'not_installed') return true;
  const serviceStatus = String(status?.status || '').trim().toLowerCase();
  return Boolean(serviceStatus && serviceStatus !== 'unknown' && serviceStatus !== 'not_installed');
}

function isFailureStatus(status: string): boolean {
  return status === 'failed' || status === 'error' || status === 'not_managed' || status.startsWith('failed:') || status.startsWith('error:');
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof ApiError ? error.message : fallback;
}

function setActionError(error: unknown, fallback: string, setLastResult: (value: { ok: boolean; message: string }) => void, showToast: (title: string, tone?: 'success' | 'error' | 'info') => void) {
  const message = errorMessage(error, fallback);
  setLastResult({ ok: false, message });
  showToast(message, 'error');
}

function commandDetail(data: CoreActionResponse, message: string) {
  const commands = firstNonEmptyCommands(data.commands_executed, data.xray?.commands_executed, data.singbox?.commands_executed);
  const singboxReason = data.singbox?.reason === 'not_needed' ? '' : data.singbox?.reason;
  const output = data.output || data.xray?.detail || data.xray?.error_output || data.xray?.error || data.singbox?.output || data.singbox?.error || singboxReason || '';
  const supplementalOutput = output && output !== message ? output : '';
  return [commands.length ? `commands:\n${commands.join('\n')}` : '', supplementalOutput ? `detail:\n${supplementalOutput}` : ''].filter(Boolean).join('\n\n') || undefined;
}

function firstNonEmptyCommands(...groups: Array<string[] | undefined>): string[] {
  return groups.find((group) => Array.isArray(group) && group.length > 0) || [];
}

function allWarnings(...groups: Array<string[] | undefined>): string[] {
  return groups.flatMap((group) => Array.isArray(group) ? group : []);
}

function firstListenerWarning(warnings: string[]): string {
  return warnings.find((warning) => warning.startsWith('配置已应用，但端口未监听')) || '';
}

function withDetail<T extends { ok: boolean; message: string }>(result: T, detail?: string): T & { detail?: string } {
  return detail ? { ...result, detail } : result;
}

function validationDetail(data: { warnings?: string[]; inbounds?: number; outbounds?: number; rules?: number }) {
  const summary = [data.inbounds != null ? `入站: ${data.inbounds}` : '', data.outbounds != null ? `出站: ${data.outbounds}` : '', data.rules != null ? `规则: ${data.rules}` : ''].filter(Boolean).join(' · ');
  const warnings = data.warnings?.length ? `警告:\n${data.warnings.join('\n')}` : '';
  return [summary, warnings].filter(Boolean).join('\n\n') || undefined;
}

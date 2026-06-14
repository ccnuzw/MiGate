import { useMutation, useQuery } from '@tanstack/react-query';
import { CheckCircle2, Download, Play, RefreshCw, ShieldCheck, Trash2, XCircle } from 'lucide-react';
import { useState } from 'react';
import { ApiError } from '../api/client';
import { api } from '../api/endpoints';
import type { CoreActionResponse } from '../api/types';
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
    ? { status: api.xrayStatus, version: api.xrayVersion, config: api.xrayConfig, logs: api.xrayLogs, validate: api.xrayValidate, apply: api.xrayApply, install: api.xrayInstall, uninstall: api.xrayUninstall }
    : { status: api.singboxStatus, version: api.singboxVersion, config: api.singboxConfig, logs: api.singboxLogs, validate: api.singboxValidate, apply: api.singboxApply, install: api.singboxInstall, uninstall: api.singboxUninstall };
  const [lastResult, setLastResult] = useState<{ ok: boolean; message: string; detail?: string } | null>(null);
  const statusQuery = useQuery({ queryKey: [core, 'status'], queryFn: endpoints.status, refetchInterval: coreStatusRefetchInterval(visible), staleTime: 10_000 });
  const versionQuery = useQuery({ queryKey: [core, 'version'], queryFn: endpoints.version, retry: false, staleTime: 10 * 60_000 });
  const configQuery = useQuery({ queryKey: [core, 'config'], queryFn: endpoints.config, staleTime: 60_000 });
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
      showToast(text(result.message), result.ok ? 'success' : 'error');
      statusQuery.refetch();
      if (result.ok) configQuery.refetch();
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
    },
    onError: (error) => setActionError(error, `${label} 卸载失败`, setLastResult, showToast),
  });
  if (statusQuery.isLoading) return <LoadingBlock />;
  const status = statusQuery.data;
  return (
    <div className="page-stack">
      <PageTitle
        title={text(`${label} 配置`)}
        description={text('查看核心运行状态、配置预览、日志和系统级操作。')}
        action={
          <div className="flex flex-wrap gap-2">
            <button className="btn secondary" onClick={() => { statusQuery.refetch(); versionQuery.refetch(); }}><RefreshCw className="h-4 w-4" /> {text('刷新')}</button>
            <SpinnerButton className="btn secondary" loading={validate.isPending} onClick={() => validate.mutate()}><ShieldCheck className="h-4 w-4" /> {text('生成校验')}</SpinnerButton>
            <SpinnerButton className="btn secondary" loading={install.isPending} onClick={async () => (await confirm({ title: text(`安装 ${label} 核心？`), description: text('该操作会执行系统安装命令。') })) && install.mutate()}><Download className="h-4 w-4" /> {text('安装核心')}</SpinnerButton>
            <SpinnerButton className="btn danger" loading={uninstall.isPending} onClick={async () => (await confirm({ title: text(`卸载 ${label} 核心？`), description: text('该操作会删除或停用系统服务。'), tone: 'danger' })) && uninstall.mutate()}><Trash2 className="h-4 w-4" /> {text('卸载核心')}</SpinnerButton>
            <SpinnerButton className="btn primary" loading={apply.isPending} onClick={async () => (await confirm({ title: text(`应用 ${label} 配置？`), description: text('该操作会重新生成并应用核心配置。') })) && apply.mutate()}><Play className="h-4 w-4" /> {text('应用')}</SpinnerButton>
          </div>
        }
      />
      <div className="metric-grid">
        <CoreMetric label={text('安装')} value={text(status?.installed === false ? '未安装' : '已安装')} />
        <CoreMetric label={text('托管')} value={text(status?.managed ? '已托管' : '未托管')} />
        <CoreMetric label={text('状态')} value={text(serviceLabel(status?.status))} />
        <CoreMetric label={text('版本')} value={text(versionLabel(status?.version || versionQuery.data?.version))} />
        <CoreMetric label={text('内存')} value={formatBytes(status?.memory_rss_bytes)} />
        <CoreMetric label={text('运行时长')} value={status?.uptime || '-'} />
        <CoreMetric label={text('连接')} value={String(status?.active_connections || 0)} />
        <CoreMetric label={text('配置路径')} value={status?.config_path || '-'} />
      </div>
      {lastResult ? (
        <Card className={`p-4 ${lastResult.ok ? 'border-emerald-200 bg-emerald-50' : 'border-red-200 bg-red-50'}`}>
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
        <Card className="p-5">
          <h2 className="section-title mb-3">{text('最近命令')}</h2>
          <pre className="code-block">{status.commands_executed.join('\n')}</pre>
        </Card>
      ) : null}
      <Card className="p-5">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h2 className="section-title">{text('配置预览')}</h2>
          <button className="btn secondary h-8" onClick={() => configQuery.refetch()}>{text('刷新配置')}</button>
        </div>
        <pre className="code-block">{JSON.stringify(configQuery.data || {}, null, 2)}</pre>
      </Card>
      <Card className="p-5">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h2 className="section-title">{text('日志')}</h2>
          <button className="btn secondary h-8" onClick={() => logsQuery.refetch()}>{text('加载日志')}</button>
        </div>
        <pre className="code-block">{formatLogs(logsQuery.data, text('点击“加载日志”查看最近日志。'))}</pre>
      </Card>
    </div>
  );
}

function CoreMetric({ label, value }: { label: string; value: string }) {
  return (
    <Card className="p-4">
      <div className="text-sm text-panel-muted">{label}</div>
      <div className="mt-2 truncate text-xl font-semibold" title={value}>{value}</div>
    </Card>
  );
}

function formatLogs(data: { logs?: string; lines?: string[] } | undefined, emptyMessage: string): string {
  if (!data) return emptyMessage;
  if (Array.isArray(data.lines)) return data.lines.join('\n');
  return data.logs || JSON.stringify(data, null, 2);
}

export function coreActionResult(data: CoreActionResponse, fallback: string): { ok: boolean; message: string; detail?: string } {
  const status = normalizeStatus(data.status);
  const xrayStatus = normalizeStatus(data.xray?.status);
  const error = data.error || data.xray?.error_output || data.singbox?.error || data.singbox?.reason || data.reason;
  const failed =
    isFailureStatus(status) ||
    isFailureStatus(xrayStatus) ||
    data.applied === false ||
    data.singbox?.applied === false ||
    Boolean(data.error || data.xray?.error_output || data.singbox?.error);
  const message = failed
    ? error || data.xray?.status || data.status || fallback
    : data.xray?.status
      ? `${fallback}：${data.xray.status}`
      : data.singbox?.applied === true || data.applied === true
        ? fallback
        : data.status || fallback;
  const detail = commandDetail(data, message);
  return withDetail({ ok: !failed, message }, detail);
}

function normalizeStatus(status?: string): string {
  return String(status || '').toLowerCase();
}

export function coreStatusRefetchInterval(visible: boolean) {
  return visible ? 12000 : false;
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
  const output = data.output || data.xray?.error_output || data.singbox?.output || data.singbox?.error || data.singbox?.reason || '';
  const supplementalOutput = output && output !== message ? output : '';
  return [commands.length ? `commands:\n${commands.join('\n')}` : '', supplementalOutput ? `detail:\n${supplementalOutput}` : ''].filter(Boolean).join('\n\n') || undefined;
}

function firstNonEmptyCommands(...groups: Array<string[] | undefined>): string[] {
  return groups.find((group) => Array.isArray(group) && group.length > 0) || [];
}

function withDetail<T extends { ok: boolean; message: string }>(result: T, detail?: string): T & { detail?: string } {
  return detail ? { ...result, detail } : result;
}

function validationDetail(data: { warnings?: string[]; inbounds?: number; outbounds?: number; rules?: number }) {
  const summary = [data.inbounds != null ? `入站: ${data.inbounds}` : '', data.outbounds != null ? `出站: ${data.outbounds}` : '', data.rules != null ? `规则: ${data.rules}` : ''].filter(Boolean).join(' · ');
  const warnings = data.warnings?.length ? `警告:\n${data.warnings.join('\n')}` : '';
  return [summary, warnings].filter(Boolean).join('\n\n') || undefined;
}

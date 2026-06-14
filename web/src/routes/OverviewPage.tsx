import { useQueries, useQuery } from '@tanstack/react-query';
import { Activity, AlertTriangle, ArrowDown, ArrowUp, Cpu, Database, HardDrive, Network, RefreshCw, Shield, Users } from 'lucide-react';
import { useMemo } from 'react';
import { api } from '../api/endpoints';
import type { DashboardSummary } from '../api/types';
import { Card, LoadingBlock } from '../components/ui';
import { formatBytes, formatDuration, formatPercent, serviceLabel } from '../lib/format';
import { usePageVisible } from '../lib/visibility';

export default function OverviewPage() {
  const visible = usePageVisible();
  const summary = useQuery({ queryKey: ['dashboard-summary'], queryFn: api.dashboardSummary, refetchInterval: visible ? 15000 : false, retry: false, staleTime: 10_000 });
  const resources = useQuery({ queryKey: ['resources'], queryFn: api.resources, refetchInterval: visible ? 10000 : false, staleTime: 5_000 });
  const [xray, singbox] = useQueries({
    queries: [
      { queryKey: ['xray-status'], queryFn: api.xrayStatus, refetchInterval: visible ? 15000 : false, staleTime: 10_000 },
      { queryKey: ['singbox-status'], queryFn: api.singboxStatus, refetchInterval: visible ? 15000 : false, staleTime: 10_000 },
    ],
  });

  const data = summary.data;
  const counts = data?.counts || emptyCounts;
  const traffic = data?.traffic || emptyTraffic;
  const protocols = Object.entries(data?.protocols || {}).map(([name, value]) => ({ name, value }));
  const trafficSeries = data?.traffic_series || [];

  if (summary.isLoading) return <LoadingBlock />;

  return (
    <div className="page-stack">
      <PageTitle
        title="运行概览"
        description="VPS 面板、核心服务和流量资源的实时摘要。"
        action={<button className="btn secondary" onClick={() => refreshOverview([summary, resources, xray, singbox])}><RefreshCw className="h-4 w-4" /> 刷新</button>}
      />
      <OverviewAlerts
          errors={[
            summary.error ? `概览摘要加载失败：${errorText(summary.error)}` : '',
            resources.error ? `资源加载失败：${errorText(resources.error)}` : '',
          xray.error ? `Xray 状态加载失败：${errorText(xray.error)}` : '',
          singbox.error ? `sing-box 状态加载失败：${errorText(singbox.error)}` : '',
          data?.validation.xray && !data.validation.xray.valid ? `Xray 生成校验失败：${data.validation.xray.error || '未知错误'}` : '',
          data?.validation.singbox && !data.validation.singbox.valid ? `sing-box 生成校验失败：${data.validation.singbox.error || '未知错误'}` : '',
        ].filter(Boolean)}
      />
      <div className="metric-grid">
        <Metric icon={Network} label="总流量" value={formatBytes(traffic.total)} sub={`${formatBytes(traffic.up)} ↑ / ${formatBytes(traffic.down)} ↓`} />
        <Metric icon={Users} label="客户端" value={String(counts.clients)} sub={`${counts.clients_active} active · ${counts.clients_expired} expired · ${counts.clients_limited} limited`} />
        <Metric icon={Shield} label="入站" value={String(counts.inbounds)} sub={`${counts.inbounds_enabled} enabled`} />
        <Metric icon={Activity} label="实时流量" value={formatBytes(traffic.xray_realtime)} sub={`${formatBytes(traffic.xray_up)} ↑ / ${formatBytes(traffic.xray_down)} ↓`} />
        <Metric icon={Network} label="出站" value={String(counts.outbounds)} sub={`${counts.outbounds_enabled} enabled`} />
        <Metric icon={Activity} label="路由规则" value={String(counts.routing_rules)} sub={`${counts.routing_enabled} enabled`} />
        <Metric icon={Activity} label="Xray" value={serviceLabel(xray.data?.status)} sub={xray.data?.version || '-'} />
        <Metric icon={Activity} label="sing-box" value={serviceLabel(singbox.data?.status)} sub={singbox.data?.version || '-'} />
      </div>
      <Card className="p-5">
        <h2 className="section-title mb-4">最近生成状态</h2>
        <div className="grid gap-3 md:grid-cols-2">
          <ValidationSummary label="Xray" loading={summary.isLoading} valid={data?.validation.xray.valid} error={summary.error} detail={validationSummary(data?.validation.xray, summary.error)} />
          <ValidationSummary label="sing-box" loading={summary.isLoading} valid={data?.validation.singbox.valid} error={summary.error} detail={validationSummary(data?.validation.singbox, summary.error)} />
        </div>
      </Card>
      <div className="grid gap-4 xl:grid-cols-[1.4fr_.9fr]">
        <Card className="p-5">
          <div className="mb-4 flex items-center justify-between gap-4">
            <h2 className="section-title">流量走势</h2>
            <div className="flex gap-2 text-xs text-panel-muted">
              <span className="inline-flex items-center gap-1">
                <ArrowUp className="h-3 w-3" /> {formatBytes(traffic.up)}
              </span>
              <span className="inline-flex items-center gap-1">
                <ArrowDown className="h-3 w-3" /> {formatBytes(traffic.down)}
              </span>
            </div>
          </div>
          <div className="h-64">
            <TrafficChart data={trafficSeries} />
          </div>
        </Card>
        <Card className="p-5">
          <h2 className="section-title mb-4">服务器资源</h2>
          <div className="grid gap-3">
            <Resource icon={Cpu} label="CPU" value={formatPercent(resources.data?.cpu_percent)} />
            <Resource icon={Database} label="内存" value={`${formatBytes(resources.data?.memory_used)} / ${formatBytes(resources.data?.memory_total)}`} />
            <Resource icon={HardDrive} label="磁盘" value={`${formatBytes(resources.data?.disk_used)} / ${formatBytes(resources.data?.disk_total)}`} />
            <Resource icon={Activity} label="运行时间" value={formatDuration(resources.data?.uptime_seconds)} />
          </div>
        </Card>
      </div>
      <Card className="p-5">
        <h2 className="section-title mb-4">协议分布</h2>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          {protocols.length ? protocols.map((item) => <div key={item.name} className="rounded-lg bg-panel-soft p-3 text-sm"><b>{item.name}</b><span className="ml-2 text-panel-muted">{item.value}</span></div>) : <span className="text-sm text-panel-muted">暂无入站</span>}
        </div>
      </Card>
    </div>
  );
}

const emptyCounts: DashboardSummary['counts'] = {
  inbounds: 0,
  inbounds_enabled: 0,
  clients: 0,
  clients_active: 0,
  clients_expired: 0,
  clients_limited: 0,
  outbounds: 0,
  outbounds_enabled: 0,
  routing_rules: 0,
  routing_enabled: 0,
};

const emptyTraffic: DashboardSummary['traffic'] = {
  up: 0,
  down: 0,
  total: 0,
  xray_up: 0,
  xray_down: 0,
  xray_realtime: 0,
};

function OverviewAlerts({ errors }: { errors: string[] }) {
  if (errors.length === 0) return null;
  return (
    <Card className="border-red-200 bg-red-50 p-4 text-sm text-red-700">
      <div className="flex items-start gap-3">
        <AlertTriangle className="mt-0.5 h-4 w-4" />
        <div className="grid gap-1">
          {errors.map((error) => <div key={error}>{error}</div>)}
        </div>
      </div>
    </Card>
  );
}

function TrafficChart({ data }: { data: DashboardSummary['traffic_series'] }) {
  const chart = useMemo(() => buildChart(data), [data]);
  if (data.length === 0) {
    return <div className="flex h-full items-center justify-center text-sm text-panel-muted">暂无流量数据</div>;
  }
  return (
    <div className="relative h-full w-full">
      <svg className="h-full w-full overflow-visible" viewBox="0 0 640 220" role="img" aria-label="流量走势">
        <defs>
          <linearGradient id="traffic-up-fill" x1="0" x2="0" y1="0" y2="1">
            <stop offset="0%" stopColor="#0f766e" stopOpacity="0.32" />
            <stop offset="100%" stopColor="#0f766e" stopOpacity="0.04" />
          </linearGradient>
          <linearGradient id="traffic-down-fill" x1="0" x2="0" y1="0" y2="1">
            <stop offset="0%" stopColor="#b45309" stopOpacity="0.26" />
            <stop offset="100%" stopColor="#b45309" stopOpacity="0.04" />
          </linearGradient>
        </defs>
        <g stroke="#e5e7eb" strokeWidth="1">
          {[0, 1, 2, 3].map((line) => <line key={line} x1="0" x2="640" y1={line * 55} y2={line * 55} />)}
        </g>
        <path d={chart.upArea} fill="url(#traffic-up-fill)" />
        <path d={chart.downArea} fill="url(#traffic-down-fill)" />
        <path d={chart.upLine} fill="none" stroke="#0f766e" strokeLinecap="round" strokeLinejoin="round" strokeWidth="3" />
        <path d={chart.downLine} fill="none" stroke="#b45309" strokeLinecap="round" strokeLinejoin="round" strokeWidth="3" />
        {chart.points.map((point) => (
          <g key={point.name + point.x}>
            <circle cx={point.x} cy={point.upY} fill="#0f766e" r="3.5" />
            <circle cx={point.x} cy={point.downY} fill="#b45309" r="3.5" />
            <title>{`${point.name}: ↑ ${formatBytes(point.up)} / ↓ ${formatBytes(point.down)}`}</title>
          </g>
        ))}
      </svg>
    </div>
  );
}

function buildChart(data: DashboardSummary['traffic_series']) {
  const width = 640;
  const bottom = 205;
  if (data.length === 0) {
    return { points: [], upLine: '', downLine: '', upArea: '', downArea: '' };
  }
  const max = Math.max(1, ...data.flatMap((item) => [Number(item.up || 0), Number(item.down || 0)]));
  const xFor = (index: number) => data.length === 1 ? width / 2 : (index / (data.length - 1)) * width;
  const yFor = (value: number) => bottom - (Number(value || 0) / max) * 180;
  const points = data.map((item, index) => ({
    name: item.name,
    up: Number(item.up || 0),
    down: Number(item.down || 0),
    x: xFor(index),
    upY: yFor(item.up),
    downY: yFor(item.down),
  }));
  const line = (key: 'upY' | 'downY') => points.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x.toFixed(1)} ${point[key].toFixed(1)}`).join(' ');
  const area = (path: string) => `${path} L ${points[points.length - 1].x.toFixed(1)} ${bottom} L ${points[0].x.toFixed(1)} ${bottom} Z`;
  const upLine = line('upY');
  const downLine = line('downY');
  return { points, upLine, downLine, upArea: area(upLine), downArea: area(downLine) };
}

function ValidationSummary({ label, loading, valid, error, detail }: { label: string; loading: boolean; valid?: boolean; error: unknown; detail: string }) {
  const failed = Boolean(error) || valid === false;
  const tone = loading ? 'text-panel-muted' : failed ? 'text-red-700' : valid === true ? 'text-emerald-700' : 'text-panel-muted';
  return (
    <div className="rounded-lg bg-panel-soft p-3 text-sm">
      <div className="flex items-center justify-between gap-3">
        <span className="font-medium">{label}</span>
        <span className={tone}>{loading ? '生成中' : error ? '不可用' : valid === false ? '失败' : valid === true ? '通过' : '未知'}</span>
      </div>
      <div className="mt-2 text-xs leading-5 text-panel-muted">{detail}</div>
    </div>
  );
}

function validationSummary(data: { valid: boolean; inbounds?: number; outbounds?: number; rules?: number; warnings?: string[]; error?: string } | undefined, error?: unknown) {
  if (error) return errorText(error);
  if (!data) return '等待校验结果';
  const parts = [];
  if (data.inbounds != null) parts.push(`inbounds ${data.inbounds}`);
  if (data.outbounds != null) parts.push(`outbounds ${data.outbounds}`);
  if (data.rules != null) parts.push(`rules ${data.rules}`);
  if (data.warnings?.length) parts.push(`warnings ${data.warnings.length}`);
  if (data.error) parts.push(data.error);
  return parts.join(' · ');
}

function errorText(error: unknown) {
  return error instanceof Error ? error.message : String(error || 'unknown');
}

function refreshOverview(queries: Array<{ refetch: () => unknown }>) {
  queries.forEach((query) => query.refetch());
}

export function PageTitle({ title, description, action }: { title: string; description?: string; action?: React.ReactNode }) {
  return (
    <div className="page-title">
      <div>
        <h1>{title}</h1>
        {description ? <p>{description}</p> : null}
      </div>
      {action}
    </div>
  );
}

function Metric({ icon: Icon, label, value, sub }: { icon: React.ElementType; label: string; value: string; sub?: string }) {
  return (
    <Card className="p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm text-panel-muted">{label}</div>
          <div className="mt-2 text-2xl font-semibold">{value}</div>
          {sub ? <div className="mt-1 truncate text-xs text-panel-muted">{sub}</div> : null}
        </div>
        <Icon className="h-5 w-5 text-panel-muted" />
      </div>
    </Card>
  );
}

function Resource({ icon: Icon, label, value }: { icon: React.ElementType; label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg bg-panel-soft px-3 py-2 text-sm">
      <span className="inline-flex items-center gap-2 text-panel-muted">
        <Icon className="h-4 w-4" /> {label}
      </span>
      <span className="min-w-0 break-words text-right font-medium">{value}</span>
    </div>
  );
}

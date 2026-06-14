import { useQueries, useQuery } from '@tanstack/react-query';
import { Activity, AlertTriangle, ArrowDown, ArrowUp, Cpu, Database, HardDrive, Network, RefreshCw, Shield, Users } from 'lucide-react';
import { Area, AreaChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { api } from '../api/endpoints';
import { Card, LoadingBlock } from '../components/ui';
import { formatBytes, formatDuration, formatPercent, serviceLabel } from '../lib/format';

export default function OverviewPage() {
  const visible = typeof document === 'undefined' || !document.hidden;
  const inbounds = useQuery({ queryKey: ['inbounds'], queryFn: api.inbounds, refetchInterval: visible ? 15000 : false });
  const outbounds = useQuery({ queryKey: ['outbounds'], queryFn: api.outbounds, refetchInterval: visible ? 30000 : false });
  const rules = useQuery({ queryKey: ['routing-rules'], queryFn: api.routingRules, refetchInterval: visible ? 30000 : false });
  const resources = useQuery({ queryKey: ['resources'], queryFn: api.resources, refetchInterval: visible ? 10000 : false });
  const xrayValidate = useQuery({ queryKey: ['xray-validate'], queryFn: api.xrayValidate, refetchInterval: visible ? 60000 : false, retry: false });
  const singboxValidate = useQuery({ queryKey: ['singbox-validate'], queryFn: api.singboxValidate, refetchInterval: visible ? 60000 : false, retry: false });
  const [xray, singbox] = useQueries({
    queries: [
      { queryKey: ['xray-status'], queryFn: api.xrayStatus, refetchInterval: visible ? 15000 : false },
      { queryKey: ['singbox-status'], queryFn: api.singboxStatus, refetchInterval: visible ? 15000 : false },
    ],
  });

  const list = inbounds.data || [];
  const clients = list.flatMap((item) => item.clients || []);
  const now = Math.floor(Date.now() / 1000);
  const activeClients = clients.filter((client) => {
    const used = Number(client.up || 0) + Number(client.down || 0);
    const limit = Number(client.traffic_limit || 0);
    if (!client.enabled) return false;
    if (client.expiry_at && client.expiry_at > 0 && client.expiry_at <= now) return false;
    if (limit > 0 && used >= limit) return false;
    return true;
  }).length;
  const expiredClients = clients.filter((client) => client.expiry_at && client.expiry_at > 0 && client.expiry_at <= now).length;
  const limitedClients = clients.filter((client) => Number(client.traffic_limit || 0) > 0 && Number(client.up || 0) + Number(client.down || 0) >= Number(client.traffic_limit || 0)).length;
  const up = list.reduce((sum, item) => sum + Number(item.traffic_up || 0), 0);
  const down = list.reduce((sum, item) => sum + Number(item.traffic_down || 0), 0);
  const realtimeUp = clients.reduce((sum, item) => sum + Number(item.xray_up || 0), 0);
  const realtimeDown = clients.reduce((sum, item) => sum + Number(item.xray_down || 0), 0);
  const protocols = Object.entries(
    list.reduce<Record<string, number>>((acc, item) => {
      acc[item.protocol] = (acc[item.protocol] || 0) + 1;
      return acc;
    }, {}),
  ).map(([name, value]) => ({ name, value }));

  if (inbounds.isLoading) return <LoadingBlock />;

  return (
    <div className="page-stack">
      <PageTitle
        title="运行概览"
        description="VPS 面板、核心服务和流量资源的实时摘要。"
        action={<button className="btn secondary" onClick={() => refreshOverview([inbounds, outbounds, rules, resources, xray, singbox, xrayValidate, singboxValidate])}><RefreshCw className="h-4 w-4" /> 刷新</button>}
      />
      <OverviewAlerts
        errors={[
          inbounds.error ? `入站加载失败：${errorText(inbounds.error)}` : '',
          outbounds.error ? `出站加载失败：${errorText(outbounds.error)}` : '',
          rules.error ? `路由加载失败：${errorText(rules.error)}` : '',
          resources.error ? `资源加载失败：${errorText(resources.error)}` : '',
          xray.error ? `Xray 状态加载失败：${errorText(xray.error)}` : '',
          singbox.error ? `sing-box 状态加载失败：${errorText(singbox.error)}` : '',
          xrayValidate.error ? `Xray 生成校验不可用：${errorText(xrayValidate.error)}` : '',
          singboxValidate.error ? `sing-box 生成校验不可用：${errorText(singboxValidate.error)}` : '',
          xrayValidate.data && !xrayValidate.data.valid ? `Xray 生成校验失败：${xrayValidate.data.error || '未知错误'}` : '',
          singboxValidate.data && !singboxValidate.data.valid ? `sing-box 生成校验失败：${singboxValidate.data.error || '未知错误'}` : '',
        ].filter(Boolean)}
      />
      <div className="metric-grid">
        <Metric icon={Network} label="总流量" value={formatBytes(up + down)} sub={`${formatBytes(up)} ↑ / ${formatBytes(down)} ↓`} />
        <Metric icon={Users} label="客户端" value={String(clients.length)} sub={`${activeClients} active · ${expiredClients} expired · ${limitedClients} limited`} />
        <Metric icon={Shield} label="入站" value={String(list.length)} sub={`${list.filter((i) => i.enabled).length} enabled`} />
        <Metric icon={Activity} label="实时流量" value={formatBytes(realtimeUp + realtimeDown)} sub={`${formatBytes(realtimeUp)} ↑ / ${formatBytes(realtimeDown)} ↓`} />
        <Metric icon={Network} label="出站" value={String((outbounds.data || []).length)} sub={`${(outbounds.data || []).filter((item) => item.enabled).length} enabled`} />
        <Metric icon={Activity} label="路由规则" value={String((rules.data || []).length)} sub={`${(rules.data || []).filter((item) => item.enabled).length} enabled`} />
        <Metric icon={Activity} label="Xray" value={serviceLabel(xray.data?.status)} sub={xray.data?.version || '-'} />
        <Metric icon={Activity} label="sing-box" value={serviceLabel(singbox.data?.status)} sub={singbox.data?.version || '-'} />
      </div>
      <Card className="p-5">
        <h2 className="section-title mb-4">最近生成状态</h2>
        <div className="grid gap-3 md:grid-cols-2">
          <ValidationSummary label="Xray" loading={xrayValidate.isLoading} valid={xrayValidate.data?.valid} error={xrayValidate.error} detail={validationSummary(xrayValidate.data, xrayValidate.error)} />
          <ValidationSummary label="sing-box" loading={singboxValidate.isLoading} valid={singboxValidate.data?.valid} error={singboxValidate.error} detail={validationSummary(singboxValidate.data, singboxValidate.error)} />
        </div>
      </Card>
      <div className="grid gap-4 xl:grid-cols-[1.4fr_.9fr]">
        <Card className="p-5">
          <div className="mb-4 flex items-center justify-between gap-4">
            <h2 className="section-title">流量走势</h2>
            <div className="flex gap-2 text-xs text-panel-muted">
              <span className="inline-flex items-center gap-1">
                <ArrowUp className="h-3 w-3" /> {formatBytes(up)}
              </span>
              <span className="inline-flex items-center gap-1">
                <ArrowDown className="h-3 w-3" /> {formatBytes(down)}
              </span>
            </div>
          </div>
          <div className="h-64">
            <ResponsiveContainer>
              <AreaChart data={list.map((item) => ({ name: item.remark || `${item.protocol}:${item.port}`, up: item.traffic_up || 0, down: item.traffic_down || 0 }))}>
                <XAxis dataKey="name" hide />
                <YAxis hide />
                <Tooltip formatter={(value) => formatBytes(Number(value))} />
                <Area dataKey="up" stroke="#0f766e" fill="#0f766e33" />
                <Area dataKey="down" stroke="#b45309" fill="#b4530933" />
              </AreaChart>
            </ResponsiveContainer>
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
      <span className="font-medium">{value}</span>
    </div>
  );
}

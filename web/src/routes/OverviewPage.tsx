import { useQueries, useQuery } from '@tanstack/react-query';
import { Activity, ArrowDown, ArrowUp, Cpu, Database, HardDrive, Network, Shield, Users } from 'lucide-react';
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
  const [xray, singbox] = useQueries({
    queries: [
      { queryKey: ['xray-status'], queryFn: api.xrayStatus, refetchInterval: visible ? 15000 : false },
      { queryKey: ['singbox-status'], queryFn: api.singboxStatus, refetchInterval: visible ? 15000 : false },
    ],
  });

  const list = inbounds.data || [];
  const clients = list.flatMap((item) => item.clients || []);
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
      <PageTitle title="运行概览" description="VPS 面板、核心服务和流量资源的实时摘要。" />
      <div className="metric-grid">
        <Metric icon={Network} label="总流量" value={formatBytes(up + down)} sub={`${formatBytes(up)} ↑ / ${formatBytes(down)} ↓`} />
        <Metric icon={Users} label="客户端" value={String(clients.length)} sub={`${clients.filter((c) => c.enabled).length} active`} />
        <Metric icon={Shield} label="入站" value={String(list.length)} sub={`${list.filter((i) => i.enabled).length} enabled`} />
        <Metric icon={Activity} label="实时流量" value={formatBytes(realtimeUp + realtimeDown)} sub={`${formatBytes(realtimeUp)} ↑ / ${formatBytes(realtimeDown)} ↓`} />
        <Metric icon={Network} label="出站" value={String((outbounds.data || []).length)} sub={`${(outbounds.data || []).filter((item) => item.enabled).length} enabled`} />
        <Metric icon={Activity} label="路由规则" value={String((rules.data || []).length)} sub={`${(rules.data || []).filter((item) => item.enabled).length} enabled`} />
        <Metric icon={Activity} label="Xray" value={serviceLabel(xray.data?.status)} sub={xray.data?.version || '-'} />
        <Metric icon={Activity} label="sing-box" value={serviceLabel(singbox.data?.status)} sub={singbox.data?.version || '-'} />
      </div>
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

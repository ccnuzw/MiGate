import '@xyflow/react/dist/style.css';

import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Background,
  BackgroundVariant,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  type ReactFlowInstance,
  type Edge,
  type Node,
  type NodeProps,
  useEdgesState,
  useNodesState,
} from '@xyflow/react';
import { AlertTriangle, Boxes, Network, Route, Shield, Users } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import clsx from 'clsx';
import { api } from '../api/endpoints';
import { Card, EmptyState, LoadingBlock, StatusBadge } from '../components/ui';
import { useI18n } from '../lib/i18n';
import { refetchTopologyDependencies } from '../lib/queryInvalidation';
import { PageTitle } from './OverviewPage';
import { buildTopologyGraph, type TopologyEdgeData, type TopologyNodeData } from './topologyGraph';

const nodeWidth = 270;
const nodeHeight = 128;
const clientNodeHeight = 116;
const lightweightLayoutNodeLimit = 28;
const lightweightLayoutEdgeLimit = 42;
const nodeTypes = { topologyNode: TopologyNode };

export default function TopologyPage() {
  const { text } = useI18n();
  const queryClient = useQueryClient();
  const inbounds = useQuery({ queryKey: ['inbounds'], queryFn: api.inbounds, staleTime: 30_000 });
  const outbounds = useQuery({ queryKey: ['outbounds'], queryFn: api.outbounds, staleTime: 30_000 });
  const routingRules = useQuery({ queryKey: ['routing-rules'], queryFn: api.routingRules, staleTime: 30_000 });
  const graph = useMemo(
    () => buildTopologyGraph(inbounds.data || [], outbounds.data || [], routingRules.data || []),
    [inbounds.data, outbounds.data, routingRules.data],
  );
  const [reactFlow, setReactFlow] = useState<ReactFlowInstance<Node<TopologyNodeData>, Edge<TopologyEdgeData>> | null>(null);
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<TopologyNodeData>>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge<TopologyEdgeData>>([]);
  const loading = inbounds.isLoading || outbounds.isLoading || routingRules.isLoading;
  const hasData = (inbounds.data?.length || 0) > 0 || (outbounds.data?.length || 0) > 0 || (routingRules.data?.length || 0) > 0;
  const stats = useMemo(() => topologyStats(graph.nodes, graph.edges), [graph]);
  const styledEdges = useMemo(() => edges.map(withEdgeDefaults), [edges]);

  useEffect(() => {
    void refetchTopologyDependencies(queryClient);
  }, [queryClient]);

  useEffect(() => {
    let cancelled = false;
    layoutGraph(graph.nodes, graph.edges).then((layouted) => {
      if (cancelled) return;
      setNodes(layouted.nodes);
      setEdges(layouted.edges);
    });
    return () => {
      cancelled = true;
    };
  }, [graph, setEdges, setNodes]);

  useEffect(() => {
    if (!reactFlow || nodes.length === 0) return;
    const frame = requestAnimationFrame(() => {
      void reactFlow.fitView({ padding: 0.2, duration: 0 });
    });
    return () => cancelAnimationFrame(frame);
  }, [reactFlow, nodes.length]);

  if (loading) return <LoadingBlock />;

  return (
    <div className="page-stack topology-page">
      <PageTitle title={text('链路画布')} description={text('只读展示入站、客户端、出站与路由规则之间的拓扑关系。')} />
      <div className="metric-grid topology-metrics">
        <TopologyMetric icon={Shield} tone="teal" label={text('入站')} value={String(stats.inbounds)} sub={`${stats.disabledInbounds} ${text('禁用')}`} />
        <TopologyMetric icon={Users} tone="blue" label={text('客户端')} value={String(stats.clients)} sub={text('作为入站附属节点')} />
        <TopologyMetric icon={Boxes} tone="amber" label={text('出站')} value={String(stats.outbounds)} sub={`${stats.missingOutbounds} ${text('缺失引用')}`} />
        <TopologyMetric icon={Route} tone="emerald" label={text('真实路由线')} value={String(stats.routingEdges)} sub={`${stats.disabledRoutes} ${text('禁用')}`} />
      </div>
      <Card className="topology-legend">
        <div className="topology-legend-item"><span className="legend-line legend-line-real" /> {text('实线路由：RoutingRule 入站/客户端 -> 出站')}</div>
        <div className="topology-legend-item"><span className="legend-line legend-line-all" /> {text('全部入站：规则未指定 inbound_id，展开到所有入站')}</div>
        <div className="topology-legend-item"><span className="legend-line legend-line-default" /> {text('默认出站：未命中启用路由时兜底到 direct，不是真实 RoutingRule')}</div>
        <div className="topology-legend-item"><span className="legend-line legend-line-client" /> {text('虚线客户端：仅表示附属/继承入站，不是真实客户端路由规则')}</div>
      </Card>
      {!hasData ? (
        <EmptyState title={text('暂无链路数据')} description={text('创建入站、出站或路由规则后，将在这里看到只读拓扑关系。')} />
      ) : (
        <Card className="topology-canvas-card">
          <ReactFlow
            nodes={nodes}
            edges={styledEdges}
            nodeTypes={nodeTypes}
            onInit={setReactFlow}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            nodesDraggable
            nodesConnectable={false}
            elementsSelectable={true}
            fitView
            fitViewOptions={{ padding: 0.2 }}
            minZoom={0.25}
            maxZoom={1.4}
          >
            <Background variant={BackgroundVariant.Dots} gap={18} size={1} />
            <MiniMap pannable zoomable nodeStrokeWidth={3} nodeColor={miniMapNodeColor} />
            <Controls showInteractive={false} />
          </ReactFlow>
        </Card>
      )}
    </div>
  );
}

function TopologyNode({ data }: NodeProps<Node<TopologyNodeData>>) {
  const { text } = useI18n();
  const Icon = data.kind === 'inbound' ? Shield : data.kind === 'client' ? Users : data.kind === 'missing-inbound' || data.kind === 'missing-client' || data.kind === 'missing-outbound' ? AlertTriangle : Boxes;
  return (
    <div className={clsx('topology-node', `topology-node-${data.kind}`, !data.enabled && 'topology-node-disabled', data.missing && 'topology-node-missing')}>
      <Handle type="target" position={Position.Left} className="topology-handle" />
      <Handle type="source" position={Position.Right} className="topology-handle" />
      <div className="topology-node-header">
        <div className="topology-node-icon"><Icon className="h-4 w-4" /></div>
        <div className="min-w-0 flex-1">
          <div className="topology-node-title">{data.title}</div>
          {data.subtitle ? <div className="topology-node-subtitle">{data.subtitle}</div> : null}
        </div>
        <StatusBadge enabled={data.enabled}>{data.missing ? text('缺失') : undefined}</StatusBadge>
      </div>
      <div className="topology-node-meta">
        {data.meta.map((item) => (
          <div key={item.label} className="topology-node-meta-row">
            <span>{text(item.label)}</span>
            <b>{text(item.value)}</b>
          </div>
        ))}
      </div>
    </div>
  );
}

function TopologyMetric({ icon: Icon, tone, label, value, sub }: { icon: typeof Network; tone: string; label: string; value: string; sub: string }) {
  return (
    <div className={`metric-card resource-card metric-${tone}`}>
      <div className="metric-icon"><Icon className="h-5 w-5" /></div>
      <div className="min-w-0">
        <div className="metric-label">{label}</div>
        <div className="metric-value">{value}</div>
        <div className="metric-sub">{sub}</div>
      </div>
    </div>
  );
}

async function layoutGraph(nodes: Array<Node<TopologyNodeData>>, edges: Array<Edge<TopologyEdgeData>>) {
  if (nodes.length === 0) return { nodes, edges };
  return { nodes: applyLightweightLayout(nodes, edges), edges };
}

export function shouldUseLightweightLayout(nodes: Array<Node<TopologyNodeData>>, edges: Array<Edge<TopologyEdgeData>>) {
  return nodes.length <= lightweightLayoutNodeLimit && edges.length <= lightweightLayoutEdgeLimit;
}

export function applyLightweightLayout(nodes: Array<Node<TopologyNodeData>>, edges: Array<Edge<TopologyEdgeData>>) {
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const inboundIds = nodes.filter((node) => node.data.kind === 'inbound' || node.data.kind === 'missing-inbound').map((node) => node.id);
  const clientIds = nodes.filter((node) => node.data.kind === 'client' || node.data.kind === 'missing-client').map((node) => node.id);
  const outboundIds = nodes.filter((node) => node.data.kind === 'outbound' || node.data.kind === 'missing-outbound').map((node) => node.id);
  const inboundOrder = orderByConnectivity(inboundIds, edges);
  const clientOrder = orderByConnectivity(clientIds, edges);
  const outboundOrder = orderByConnectivity(outboundIds, edges);
  const positioned = new Map<string, { x: number; y: number }>();
  const xByColumn = [0, 380, 760];
  placeColumn(inboundOrder, xByColumn[0], positioned, byId);
  placeColumn(clientOrder, xByColumn[1], positioned, byId);
  placeColumn(outboundOrder, xByColumn[2], positioned, byId);
  const remaining = nodes.filter((node) => !positioned.has(node.id)).map((node) => node.id);
  placeColumn(remaining, clientOrder.length ? xByColumn[1] : xByColumn[0], positioned, byId);
  return nodes.map((node) => ({ ...node, position: positioned.get(node.id) || node.position }));
}

function orderByConnectivity(ids: string[], edges: Array<Edge<TopologyEdgeData>>) {
  const score = new Map(ids.map((id) => [id, 0]));
  for (const edge of edges) {
    if (score.has(edge.source)) score.set(edge.source, (score.get(edge.source) || 0) + 1);
    if (score.has(edge.target)) score.set(edge.target, (score.get(edge.target) || 0) + 1);
  }
  return [...ids].sort((a, b) => (score.get(b) || 0) - (score.get(a) || 0) || a.localeCompare(b));
}

function placeColumn(ids: string[], x: number, positioned: Map<string, { x: number; y: number }>, byId: Map<string, Node<TopologyNodeData>>) {
  const gap = 34;
  let y = 0;
  for (const id of ids) {
    const node = byId.get(id);
    if (!node) continue;
    positioned.set(id, { x, y });
    y += (node.data.kind === 'client' ? clientNodeHeight : nodeHeight) + gap;
  }
}

function withEdgeDefaults(edge: Edge<TopologyEdgeData>): Edge<TopologyEdgeData> {
  const clientEdge = edge.data?.kind === 'client-inherits';
  const defaultEdge = edge.data?.kind === 'default-direct';
  const missingEdge = edge.data?.missingTarget;
  const enabledEdge = edge.data?.enabled !== false;
  const stroke = missingEdge ? '#dc2626' : defaultEdge ? '#64748b' : enabledEdge ? '#0f766e' : '#64748b';
  return {
    ...edge,
    style: {
      ...edge.style,
      stroke,
      strokeWidth: missingEdge ? 3.6 : clientEdge || defaultEdge ? 2.6 : 3.4,
      opacity: defaultEdge ? (enabledEdge ? 0.62 : 0.36) : enabledEdge ? 0.94 : 0.58,
    },
    markerEnd: clientEdge || defaultEdge ? undefined : {
      type: MarkerType.ArrowClosed,
      width: 24,
      height: 24,
      color: stroke,
    },
    className: clsx(
      edge.className,
      clientEdge && 'topology-edge-client',
      defaultEdge && 'topology-edge-default',
      edge.data?.kind === 'all-inbounds-routing' && 'topology-edge-all',
      edge.data?.missingTarget && 'topology-edge-missing',
      edge.data?.enabled === false && 'topology-edge-disabled',
    ),
  };
}

function topologyStats(nodes: Array<Node<TopologyNodeData>>, edges: Array<Edge<TopologyEdgeData>>) {
  return {
    inbounds: nodes.filter((node) => node.data.kind === 'inbound').length,
    disabledInbounds: nodes.filter((node) => node.data.kind === 'inbound' && !node.data.enabled).length,
    clients: nodes.filter((node) => node.data.kind === 'client').length,
    outbounds: nodes.filter((node) => node.data.kind === 'outbound').length,
    missingOutbounds: nodes.filter((node) => node.data.kind === 'missing-outbound').length,
    routingEdges: edges.filter((edge) => edge.data?.kind === 'routing' || edge.data?.kind === 'all-inbounds-routing' || edge.data?.kind === 'client-routing').length,
    disabledRoutes: edges.filter((edge) => (edge.data?.kind === 'routing' || edge.data?.kind === 'all-inbounds-routing' || edge.data?.kind === 'client-routing') && !edge.data.enabled).length,
  };
}

function miniMapNodeColor(node: Node<TopologyNodeData>) {
  if (node.data.kind === 'missing-inbound') return '#dc2626';
  if (node.data.kind === 'missing-outbound') return '#dc2626';
  if (!node.data.enabled) return '#94a3b8';
  if (node.data.kind === 'inbound') return '#0f766e';
  if (node.data.kind === 'client') return '#2563eb';
  return '#b45309';
}

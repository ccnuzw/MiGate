import '@xyflow/react/dist/style.css';

import { useQuery } from '@tanstack/react-query';
import {
  Background,
  BackgroundVariant,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
  useEdgesState,
  useNodesState,
} from '@xyflow/react';
import { AlertTriangle, Boxes, Network, Route, Shield, Users } from 'lucide-react';
import { useEffect, useMemo } from 'react';
import clsx from 'clsx';
import { api } from '../api/endpoints';
import { Card, EmptyState, LoadingBlock, StatusBadge } from '../components/ui';
import { useI18n } from '../lib/i18n';
import { PageTitle } from './OverviewPage';
import { buildTopologyGraph, type TopologyEdgeData, type TopologyNodeData } from './topologyGraph';

const nodeWidth = 270;
const nodeHeight = 128;
const clientNodeHeight = 116;
const nodeTypes = { topologyNode: TopologyNode };
let layoutRequestId = 0;
let layoutWorker: Worker | undefined;

export default function TopologyPage() {
  const { text } = useI18n();
  const inbounds = useQuery({ queryKey: ['inbounds'], queryFn: api.inbounds, staleTime: 30_000 });
  const outbounds = useQuery({ queryKey: ['outbounds'], queryFn: api.outbounds, staleTime: 30_000 });
  const routingRules = useQuery({ queryKey: ['routing-rules'], queryFn: api.routingRules, staleTime: 30_000 });
  const graph = useMemo(
    () => buildTopologyGraph(inbounds.data || [], outbounds.data || [], routingRules.data || []),
    [inbounds.data, outbounds.data, routingRules.data],
  );
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<TopologyNodeData>>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge<TopologyEdgeData>>([]);
  const loading = inbounds.isLoading || outbounds.isLoading || routingRules.isLoading;
  const hasData = (inbounds.data?.length || 0) > 0 || (outbounds.data?.length || 0) > 0 || (routingRules.data?.length || 0) > 0;
  const stats = useMemo(() => topologyStats(graph.nodes, graph.edges), [graph]);
  const styledEdges = useMemo(() => edges.map(withEdgeDefaults), [edges]);

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
        <div className="topology-legend-item"><span className="legend-line legend-line-all" /> {text('全部入站：规则未指定 inbound_tag，展开到所有入站')}</div>
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
  const layoutNodes = nodes.map((node) => ({
    id: node.id,
    width: nodeWidth,
    height: node.data.kind === 'client' ? clientNodeHeight : nodeHeight,
  }));
  const layoutEdges = edges.map((edge) => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
  }));
  const positions = await layoutInWorker(layoutNodes, layoutEdges).catch((error) => {
    console.warn('Topology layout worker failed; using initial graph positions.', error);
    return new Map<string, { x: number; y: number }>();
  });
  return {
    nodes: nodes.map((node) => ({ ...node, position: positions.get(node.id) || node.position })),
    edges,
  };
}

type LayoutNode = { id: string; width: number; height: number };
type LayoutEdge = { id: string; source: string; target: string };

function layoutInWorker(nodes: LayoutNode[], edges: LayoutEdge[]) {
  if (typeof Worker === 'undefined') return Promise.reject(new Error('worker_unavailable'));
  if (!layoutWorker) {
    layoutWorker = new Worker(new URL('./topologyLayout.worker.ts', import.meta.url), { type: 'module' });
  }
  const requestId = ++layoutRequestId;
  return new Promise<Map<string, { x: number; y: number }>>((resolve, reject) => {
    const cleanup = () => {
      layoutWorker?.removeEventListener('message', onMessage);
      layoutWorker?.removeEventListener('error', onError);
    };
    const onMessage = (event: MessageEvent<{ id: number; positions?: Record<string, { x: number; y: number }>; error?: string }>) => {
      if (event.data.id !== requestId) return;
      cleanup();
      if (event.data.error) {
        reject(new Error(event.data.error));
        return;
      }
      resolve(new Map(Object.entries(event.data.positions || {})));
    };
    const onError = (event: ErrorEvent) => {
      cleanup();
      layoutWorker?.terminate();
      layoutWorker = undefined;
      reject(event.error || new Error(event.message));
    };
    layoutWorker?.addEventListener('message', onMessage);
    layoutWorker?.addEventListener('error', onError);
    layoutWorker?.postMessage({ id: requestId, nodes, edges });
  });
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

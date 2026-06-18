type LayoutNode = {
  id: string;
  width: number;
  height: number;
};

type LayoutEdge = {
  id: string;
  source: string;
  target: string;
};

type LayoutRequest = {
  id: number;
  nodes: LayoutNode[];
  edges: LayoutEdge[];
};

type LayoutResponse = {
  id: number;
  positions?: Record<string, { x: number; y: number }>;
  error?: string;
};

self.onmessage = async (event: MessageEvent<LayoutRequest>) => {
  const request = event.data;
  try {
    const { default: ELK } = await import('elkjs/lib/elk.bundled.js');
    const elk = new ELK();
    const layout = await elk.layout({
      id: 'root',
      layoutOptions: {
        'elk.algorithm': 'layered',
        'elk.direction': 'RIGHT',
        'elk.spacing.nodeNode': '42',
        'elk.layered.spacing.nodeNodeBetweenLayers': '86',
        'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX',
      },
      children: request.nodes,
      edges: request.edges.map((edge) => ({
        id: edge.id,
        sources: [edge.source],
        targets: [edge.target],
      })),
    });
    const positions: LayoutResponse['positions'] = {};
    (layout.children || []).forEach((node) => {
      positions[node.id] = { x: node.x || 0, y: node.y || 0 };
    });
    self.postMessage({ id: request.id, positions } satisfies LayoutResponse);
  } catch (error) {
    self.postMessage({ id: request.id, error: error instanceof Error ? error.message : String(error) } satisfies LayoutResponse);
  }
};

import { describe, expect, it } from 'vitest';
import type { Edge, Node } from '@xyflow/react';
import { applyLightweightLayout, shouldUseLightweightLayout } from './TopologyPage';
import type { TopologyEdgeData, TopologyNodeData } from './topologyGraph';

function node(id: string, kind: TopologyNodeData['kind']): Node<TopologyNodeData> {
  return {
    id,
    type: 'topologyNode',
    position: { x: -1, y: -1 },
    data: { kind, title: id, enabled: true, meta: [] },
  };
}

function edge(id: string, source: string, target: string): Edge<TopologyEdgeData> {
  return { id, source, target, data: { kind: 'routing', label: id, ruleId: 1, enabled: true } };
}

describe('topology layout strategy', () => {
  it('uses lightweight layout for small graphs', () => {
    const nodes = [node('inbound:1', 'inbound'), node('client:1', 'client'), node('outbound:direct', 'outbound')];
    const edges = [edge('client-1', 'inbound:1', 'client:1'), edge('route-1', 'inbound:1', 'outbound:direct')];

    expect(shouldUseLightweightLayout(nodes, edges)).toBe(true);
    expect(applyLightweightLayout(nodes, edges).map((item) => item.position)).toEqual([
      { x: 0, y: 0 },
      { x: 380, y: 0 },
      { x: 760, y: 0 },
    ]);
  });

  it('marks large graphs as above the lightweight threshold', () => {
    const nodes = Array.from({ length: 29 }, (_, index) => node(`inbound:${index}`, 'inbound'));
    expect(shouldUseLightweightLayout(nodes, [])).toBe(false);
  });
});

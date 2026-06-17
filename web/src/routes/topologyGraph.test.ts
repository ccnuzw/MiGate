import { describe, expect, it } from 'vitest';
import type { Inbound, Outbound } from '../api/types';
import { buildInboundTagLookup, buildTopologyGraph } from './topologyGraph';

const inbounds: Inbound[] = [
  {
    id: 7,
    remark: 'edge-hk',
    protocol: 'VLESS',
    port: 443,
    network: 'tcp',
    security: 'reality',
    enabled: true,
    clients: [
      { id: 11, inbound_id: 7, email: 'alice@example.com', uuid: 'u-1', enabled: true, up: 1024, down: 2048, traffic_limit: 4096 },
      { id: 12, inbound_id: 7, email: 'bob@example.com', uuid: 'u-2', enabled: false },
    ],
  },
  {
    id: 8,
    remark: '',
    protocol: 'vmess',
    port: 8443,
    network: 'ws',
    security: 'tls',
    enabled: false,
    clients: [],
  },
];

const outbounds: Outbound[] = [
  { id: 1, tag: 'direct', protocol: 'freedom', enabled: true },
  { id: 2, tag: 'proxy-a', remark: 'Tokyo relay', protocol: 'socks', address: '127.0.0.1', port: 1080, enabled: false },
];

describe('topology graph helpers', () => {
  it('generates inbound, client and outbound nodes with display data', () => {
    const graph = buildTopologyGraph(inbounds, outbounds, []);

    expect(graph.nodes.find((node) => node.id === 'inbound:7')?.data).toMatchObject({
      kind: 'inbound',
      title: 'edge-hk',
      subtitle: 'inbound-7-vless',
      enabled: true,
    });
    expect(graph.nodes.find((node) => node.id === 'client:11')?.data).toMatchObject({
      kind: 'client',
      title: 'alice@example.com',
      enabled: true,
    });
    expect(graph.nodes.find((node) => node.id === 'outbound:proxy-a')?.data).toMatchObject({
      kind: 'outbound',
      title: 'proxy-a',
      subtitle: 'Tokyo relay',
      enabled: false,
    });
    expect(graph.edges.find((edge) => edge.id === 'client-7-11')?.data).toMatchObject({
      kind: 'client-inherits',
      label: '附属客户端',
      enabled: true,
    });
  });

  it('matches inbound_tag by generated tag and remark alias', () => {
    const lookup = buildInboundTagLookup(inbounds);
    expect(lookup.get('inbound-7-vless')?.id).toBe(7);
    expect(lookup.get('edge-hk')?.id).toBe(7);

    const graph = buildTopologyGraph(inbounds, outbounds, [
      { id: 21, inbound_tag: 'inbound-7-vless', outbound_tag: 'direct', enabled: true },
      { id: 22, inbound_tag: 'edge-hk', outbound_tag: 'proxy-a', enabled: true },
    ]);

    expect(graph.edges.find((edge) => edge.id.startsWith('rule-21-7'))).toMatchObject({
      source: 'inbound:7',
      target: 'outbound:direct',
    });
    expect(graph.edges.find((edge) => edge.id.startsWith('rule-22-7'))).toMatchObject({
      source: 'inbound:7',
      target: 'outbound:proxy-a',
    });
  });

  it('expands empty inbound_tag rules to all inbounds', () => {
    const graph = buildTopologyGraph(inbounds, outbounds, [
      { id: 31, inbound_tag: '', outbound_tag: 'direct', enabled: true },
      { id: 32, inbound_tag: '   ', outbound_tag: 'direct', enabled: true },
    ]);

    const routingEdges = graph.edges.filter((edge) => edge.id.startsWith('rule-31-'));
    expect(routingEdges).toHaveLength(2);
    expect(routingEdges.map((edge) => edge.source).sort()).toEqual(['inbound:7', 'inbound:8']);
    expect(routingEdges.every((edge) => edge.data?.kind === 'all-inbounds-routing')).toBe(true);

    const blankTagEdges = graph.edges.filter((edge) => edge.id.startsWith('rule-32-'));
    expect(blankTagEdges).toHaveLength(2);
    expect(blankTagEdges.every((edge) => edge.data?.kind === 'all-inbounds-routing')).toBe(true);
    expect(blankTagEdges.every((edge) => edge.label === '#32 全部入站')).toBe(true);
  });

  it('creates a missing outbound node when outbound_tag is not found', () => {
    const graph = buildTopologyGraph(inbounds, outbounds, [
      { id: 41, inbound_tag: 'inbound-7-vless', outbound_tag: 'deleted-proxy', enabled: true },
    ]);

    expect(graph.nodes.find((node) => node.id === 'outbound:deleted-proxy')?.data).toMatchObject({
      kind: 'missing-outbound',
      missing: true,
      enabled: false,
    });
    expect(graph.edges.find((edge) => edge.id.startsWith('rule-41-7'))?.data).toMatchObject({
      kind: 'routing',
      missingTarget: true,
    });
  });

  it('creates a missing inbound node when inbound_tag is not found', () => {
    const graph = buildTopologyGraph(inbounds, outbounds, [
      { id: 42, inbound_tag: 'deleted-inbound', outbound_tag: 'direct', enabled: true },
    ]);

    expect(graph.nodes.find((node) => node.id === 'inbound-missing:deleted-inbound')?.data).toMatchObject({
      kind: 'missing-inbound',
      title: 'deleted-inbound',
      missing: true,
      enabled: false,
    });
    expect(graph.nodes.find((node) => node.id === 'inbound-missing:deleted-inbound')?.position.y).toBeGreaterThan(
      graph.nodes.find((node) => node.id === 'client:12')?.position.y || 0,
    );
    expect(graph.edges.find((edge) => edge.id.startsWith('rule-42-missing-deleted-inbound'))).toMatchObject({
      source: 'inbound-missing:deleted-inbound',
      target: 'outbound:direct',
    });
    expect(graph.edges.find((edge) => edge.id.startsWith('rule-42-missing-deleted-inbound'))?.data).toMatchObject({
      kind: 'routing',
      enabled: false,
    });
  });

  it('marks disabled rules and nodes in graph data', () => {
    const graph = buildTopologyGraph(inbounds, outbounds, [
      { id: 51, inbound_tag: 'inbound-7-vless', outbound_tag: 'direct', enabled: false },
      { id: 52, inbound_tag: 'inbound-8-vmess', outbound_tag: 'proxy-a', enabled: true },
    ]);

    expect(graph.nodes.find((node) => node.id === 'inbound:8')?.data.enabled).toBe(false);
    expect(graph.nodes.find((node) => node.id === 'client:12')?.data.enabled).toBe(false);
    expect(graph.nodes.find((node) => node.id === 'outbound:proxy-a')?.data.enabled).toBe(false);
    expect(graph.edges.find((edge) => edge.id.startsWith('rule-51-7'))?.data?.enabled).toBe(false);
    expect(graph.edges.find((edge) => edge.id.startsWith('rule-52-8'))?.data?.enabled).toBe(false);

    const disabledOutboundGraph = buildTopologyGraph(inbounds, outbounds, [
      { id: 53, inbound_tag: 'inbound-7-vless', outbound_tag: 'proxy-a', enabled: true },
    ]);
    expect(disabledOutboundGraph.edges.find((edge) => edge.id.startsWith('rule-53-7'))?.data?.enabled).toBe(false);
  });

  it('does not create fake client-to-outbound routing edges', () => {
    const graph = buildTopologyGraph(inbounds, outbounds, [
      { id: 61, inbound_tag: 'inbound-7-vless', outbound_tag: 'direct', enabled: true },
    ]);

    expect(graph.edges.some((edge) => edge.source.startsWith('client:') && edge.target.startsWith('outbound:'))).toBe(false);
  });

  it('creates real client-to-outbound routing edges for client rules', () => {
    const graph = buildTopologyGraph(inbounds, outbounds, [
      { id: 62, inbound_tag: 'inbound-7-vless', client_id: 11, client_email: 'alice@example.com', outbound_tag: 'direct', enabled: true },
    ]);

    expect(graph.edges.find((edge) => edge.id.startsWith('rule-62-client-11'))).toMatchObject({
      source: 'client:11',
      target: 'outbound:direct',
      data: {
        kind: 'client-routing',
        ruleId: 62,
      },
    });
    expect(graph.edges.some((edge) => edge.id.startsWith('rule-62-7') && edge.source === 'inbound:7' && edge.target === 'outbound:direct')).toBe(false);
    expect(graph.edges.find((edge) => edge.id === 'default-direct-client-11')).toBeUndefined();
  });

  it('keeps client default direct inheritance when client routing target is unavailable', () => {
    const disabledTargetGraph = buildTopologyGraph(inbounds, outbounds, [
      { id: 64, inbound_tag: 'inbound-7-vless', client_id: 11, client_email: 'alice@example.com', outbound_tag: 'proxy-a', enabled: true },
    ]);
    expect(disabledTargetGraph.edges.find((edge) => edge.id.startsWith('rule-64-client-11'))?.data?.enabled).toBe(false);
    expect(disabledTargetGraph.edges.find((edge) => edge.id === 'default-direct-client-11')).toMatchObject({
      source: 'client:11',
      target: 'outbound:direct',
    });

    const missingTargetGraph = buildTopologyGraph(inbounds, outbounds, [
      { id: 65, inbound_tag: 'inbound-7-vless', client_id: 11, client_email: 'alice@example.com', outbound_tag: 'deleted-proxy', enabled: true },
    ]);
    expect(missingTargetGraph.edges.find((edge) => edge.id.startsWith('rule-65-client-11'))?.data?.enabled).toBe(false);
    expect(missingTargetGraph.edges.find((edge) => edge.id === 'default-direct-client-11')).toMatchObject({
      source: 'client:11',
      target: 'outbound:direct',
    });
  });

  it('creates a missing client node for deleted client routing rules', () => {
    const graph = buildTopologyGraph(inbounds, outbounds, [
      { id: 63, inbound_tag: 'inbound-7-vless', client_id: 99, client_email: 'deleted@example.com', outbound_tag: 'direct', enabled: true },
    ]);

    expect(graph.nodes.find((node) => node.id === 'client-missing:63')?.data).toMatchObject({
      kind: 'missing-client',
      title: 'deleted@example.com',
      missing: true,
      enabled: false,
    });
    expect(graph.edges.find((edge) => edge.id.startsWith('rule-63-missing-client-99'))).toMatchObject({
      source: 'client-missing:63',
      target: 'outbound:direct',
      data: {
        kind: 'client-routing',
        enabled: false,
      },
    });
  });

  it('uses unique missing client nodes for multiple rules referencing the same deleted client', () => {
    const graph = buildTopologyGraph(inbounds, outbounds, [
      { id: 66, inbound_tag: 'inbound-7-vless', client_id: 99, client_email: 'deleted@example.com', outbound_tag: 'direct', enabled: true },
      { id: 67, inbound_tag: 'deleted-inbound', client_id: 99, client_email: 'deleted@example.com', outbound_tag: 'direct', enabled: true },
    ]);

    expect(graph.nodes.filter((node) => node.id.startsWith('client-missing:')).map((node) => node.id).sort()).toEqual(['client-missing:66', 'client-missing:67']);
    expect(graph.nodes.find((node) => node.id === 'inbound-missing:deleted-inbound')?.data).toMatchObject({
      kind: 'missing-inbound',
      missing: true,
    });
  });

  it('adds a default direct visual edge only for inbounds without enabled routes', () => {
    const graph = buildTopologyGraph(inbounds, outbounds, [
      { id: 71, inbound_tag: 'inbound-7-vless', outbound_tag: 'direct', enabled: true },
      { id: 72, inbound_tag: 'inbound-8-vmess', outbound_tag: 'proxy-a', enabled: false },
    ]);

    expect(graph.edges.find((edge) => edge.id === 'default-direct-7')).toBeUndefined();
    expect(graph.edges.find((edge) => edge.id === 'default-direct-8')).toMatchObject({
      source: 'inbound:8',
      target: 'outbound:direct',
      data: {
        kind: 'default-direct',
        label: '默认出站',
        enabled: false,
      },
    });
    expect(graph.edges.find((edge) => edge.id === 'default-direct-client-11')).toBeUndefined();
  });

  it('keeps default direct visual edges when enabled routes point to unavailable outbounds', () => {
    const disabledTargetGraph = buildTopologyGraph(inbounds, outbounds, [
      { id: 73, inbound_tag: 'inbound-7-vless', outbound_tag: 'proxy-a', enabled: true },
    ]);
    expect(disabledTargetGraph.edges.find((edge) => edge.id.startsWith('rule-73-7'))?.data?.enabled).toBe(false);
    expect(disabledTargetGraph.edges.find((edge) => edge.id === 'default-direct-7')).toMatchObject({
      source: 'inbound:7',
      target: 'outbound:direct',
      data: { kind: 'default-direct', enabled: true },
    });

    const missingTargetGraph = buildTopologyGraph(inbounds, outbounds, [
      { id: 74, inbound_tag: 'inbound-7-vless', outbound_tag: 'missing-proxy', enabled: true },
    ]);
    expect(missingTargetGraph.edges.find((edge) => edge.id.startsWith('rule-74-7'))?.data?.enabled).toBe(false);
    expect(missingTargetGraph.edges.find((edge) => edge.id === 'default-direct-7')).toMatchObject({
      source: 'inbound:7',
      target: 'outbound:direct',
      data: { kind: 'default-direct', enabled: true },
    });
  });

  it('does not create default direct edges when direct outbound is missing', () => {
    const graph = buildTopologyGraph(inbounds, outbounds.filter((outbound) => outbound.tag !== 'direct'), []);

    expect(graph.edges.some((edge) => edge.data?.kind === 'default-direct')).toBe(false);
  });

  it('marks routing edges invalid when outbound profile does not support the source core', () => {
    const mixedInbounds: Inbound[] = [
      { id: 1, remark: 'xray-edge', protocol: 'vless', core: 'xray', port: 443, network: 'tcp', security: 'none', enabled: true, clients: [] },
      { id: 2, remark: 'sb-edge', protocol: 'hysteria2', core: 'sing-box', port: 8443, network: 'udp', security: 'tls', enabled: true, clients: [] },
    ];
    const mixedOutbounds: Outbound[] = [
      { id: 1, tag: 'direct', protocol: 'freedom', enabled: true },
      { id: 2, tag: 'hy2-out', protocol: 'hysteria2', supported_cores: ['sing-box'], enabled: true },
      { id: 3, tag: 'shared-socks', protocol: 'socks', supported_cores: ['xray', 'sing-box'], enabled: true },
    ];
    const graph = buildTopologyGraph(mixedInbounds, mixedOutbounds, [
      { id: 81, inbound_tag: 'inbound-1-vless', outbound_tag: 'hy2-out', enabled: true },
      { id: 82, inbound_tag: 'inbound-2-hysteria2', outbound_tag: 'shared-socks', enabled: true },
    ]);

    expect(graph.nodes.find((node) => node.id === 'inbound:1')?.data.meta).toContainEqual({ label: '内核', value: 'xray' });
    expect(graph.nodes.find((node) => node.id === 'outbound:hy2-out')?.data.meta).toContainEqual({ label: '内核', value: 'sing-box' });
    expect(graph.edges.find((edge) => edge.id.startsWith('rule-81-1'))?.data).toMatchObject({
      enabled: false,
      invalidTarget: true,
    });
    expect(graph.edges.find((edge) => edge.id.startsWith('rule-82-2'))?.data).toMatchObject({
      enabled: true,
      invalidTarget: false,
    });
  });

  it('resolves routing target by outbound profile id before falling back to tag', () => {
    const graph = buildTopologyGraph(inbounds, [
      { id: 1, tag: 'direct', protocol: 'freedom', enabled: true },
      { id: 42, tag: 'renamed-proxy', protocol: 'socks', supported_cores: ['xray', 'sing-box'], enabled: true },
    ], [
      { id: 91, inbound_tag: 'inbound-7-vless', outbound_id: 42, outbound_tag: 'old-proxy-tag', enabled: true },
    ]);

    expect(graph.nodes.find((node) => node.id === 'outbound:old-proxy-tag')).toBeUndefined();
    expect(graph.edges.find((edge) => edge.id.startsWith('rule-91-7'))).toMatchObject({
      source: 'inbound:7',
      target: 'outbound:renamed-proxy',
      data: {
        missingTarget: false,
        invalidTarget: false,
      },
    });
  });
});

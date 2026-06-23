import { describe, expect, it } from 'vitest';
import type { RoutingRule } from '../api/types';
import { clientRouteMatchIdentity, clientSelectionOptions, generatedInboundTag, inboundSelectionOptions, inboundTagOptions, movedRoutingRuleIds, newRoutingRuleDraft, outboundSelectionOptions, routingPayload, ruleTitle } from './RoutingPage';

describe('routing helpers', () => {
  it('builds create and edit payloads with backend field names only', () => {
    expect(routingPayload({ inbound_tag: 'edge', domain: 'geosite:netflix', ip: 'geoip:private', rule_set: 'geosite-category-ads-all', protocol: '', outbound_id: 9, outbound_tag: 'proxy-a', enabled: true })).toEqual({
      inbound_id: 0,
      inbound_tag: 'edge',
      client_id: 0,
      client_email: '',
      domain: 'geosite:netflix',
      ip: 'geoip:private',
      rule_set: 'geosite-category-ads-all',
      protocol: '',
      outbound_id: 9,
      outbound_tag: 'proxy-a',
      enabled: true,
    });

    expect(routingPayload({ inbound_tag: undefined, domain: undefined, protocol: 'bittorrent', outbound_id: 2, outbound_tag: 'blocked', enabled: false })).toEqual({
      inbound_id: 0,
      inbound_tag: '',
      client_id: 0,
      client_email: '',
      domain: '',
      ip: '',
      rule_set: '',
      protocol: 'bittorrent',
      outbound_id: 2,
      outbound_tag: 'blocked',
      enabled: false,
    });
  });

  it('preserves complete fields when toggling enabled state', () => {
    const rule: RoutingRule = { id: 8, inbound_tag: 'edge', domain: 'example.com', ip: '8.8.8.8', rule_set: 'geoip-cn', protocol: 'dns', outbound_id: 1, outbound_tag: 'direct', enabled: true };
    expect(routingPayload({ ...rule, enabled: !rule.enabled })).toEqual({
      inbound_id: 0,
      inbound_tag: 'edge',
      client_id: 0,
      client_email: '',
      domain: 'example.com',
      ip: '8.8.8.8',
      rule_set: 'geoip-cn',
      protocol: 'dns',
      outbound_id: 1,
      outbound_tag: 'direct',
      enabled: false,
    });
  });

  it('returns reordered rule ids', () => {
    const rules: RoutingRule[] = [
      { id: 1, outbound_tag: 'direct', enabled: true },
      { id: 2, outbound_tag: 'proxy-a', enabled: true },
      { id: 3, outbound_tag: 'blocked', enabled: false },
    ];
    expect(movedRoutingRuleIds(rules, 2, -1)).toEqual([1, 3, 2]);
  });

  it('creates new rule drafts only from real outbound ids', () => {
    expect(newRoutingRuleDraft([])).toBeNull();
    expect(newRoutingRuleDraft([{ id: 0, tag: 'direct', protocol: 'freedom', enabled: true }])).toBeNull();
    expect(newRoutingRuleDraft([{ id: 42, tag: 'real-direct', protocol: 'freedom', enabled: true }])).toMatchObject({
      id: 0,
      outbound_id: 42,
      outbound_tag: 'real-direct',
      enabled: true,
    });
  });

  it('offers generated inbound tags before remark aliases', () => {
    expect(generatedInboundTag({ id: 7, protocol: 'VLESS' })).toBe('inbound-7-vless');
    expect(inboundTagOptions([
      { id: 7, remark: 'edge', protocol: 'VLESS', port: 443, network: 'tcp', security: 'none', enabled: true, clients: [] },
      { id: 8, remark: '', protocol: 'vmess', port: 8443, network: 'ws', security: 'tls', enabled: true, clients: [] },
    ])).toEqual(['inbound-7-vless', 'inbound-8-vmess']);
  });

  it('shows each inbound once while keeping remark aliases searchable', () => {
    const options = inboundSelectionOptions([
      { id: 7, remark: 'edge', protocol: 'VLESS', port: 443, network: 'tcp', security: 'reality', enabled: true, clients: [{ id: 1, inbound_id: 7, email: 'alice', uuid: 'u', enabled: true }] },
    ]);

    expect(options).toHaveLength(2);
    expect(options[1]).toMatchObject({
      value: 'inbound-7-vless',
      aliases: ['edge'],
      title: 'edge',
      subtitle: 'inbound-7-vless',
      typeLabel: '入站',
    });
    expect(options[1].search).toContain('edge');
  });

  it('keeps user-provided names as raw display values', () => {
    const inboundOptions = inboundSelectionOptions([
      { id: 7, remark: '启用客户入口', protocol: 'VLESS', port: 443, network: 'tcp', security: 'reality', enabled: true, clients: [] },
    ]);
    expect(inboundOptions[1]).toMatchObject({
      title: '启用客户入口',
      subtitle: 'inbound-7-vless',
    });

    const clientOptions = clientSelectionOptions([
      { id: 7, remark: '启用客户入口', protocol: 'VLESS', port: 443, network: 'tcp', security: 'reality', enabled: true, clients: [{ id: 11, inbound_id: 7, email: '客户启用', uuid: 'u-1', enabled: true }] },
    ], 7, 'inbound-7-vless');
    expect(clientOptions.find((option) => option.id === 11)).toMatchObject({
      title: '客户启用',
      meta: expect.arrayContaining([{ label: '入站：', value: '启用客户入口' }]),
    });
  });

  it('offers client options scoped by selected inbound and includes missing clients', () => {
    const options = clientSelectionOptions([
      { id: 7, remark: 'edge', protocol: 'VLESS', port: 443, network: 'tcp', security: 'reality', enabled: true, clients: [{ id: 11, inbound_id: 7, email: 'alice@example.com', uuid: 'u-1', enabled: true }] },
      { id: 8, remark: 'other', protocol: 'vmess', port: 8443, network: 'ws', security: 'tls', enabled: true, clients: [{ id: 12, inbound_id: 8, email: 'bob@example.com', uuid: 'u-2', enabled: true }] },
    ], 7, 'inbound-7-vless', { inbound_id: 7, inbound_tag: 'inbound-7-vless', client_id: 99, client_email: 'deleted@example.com' });

    expect(options.map((option) => option.id)).toEqual([0, 11, 99]);
    expect(options.find((option) => option.id === 11)).toMatchObject({
      email: 'alice@example.com',
      title: 'alice@example.com',
      inboundID: 7,
      inboundTag: 'inbound-7-vless',
      typeLabel: '客户端级',
    });
    expect(options.find((option) => option.id === 99)).toMatchObject({
      missing: true,
      inboundID: 7,
      title: 'deleted@example.com',
      typeLabel: '客户端已缺失',
    });
  });

  it('matches socks and http clients by authenticated username in route identity', () => {
    const options = clientSelectionOptions([
      {
        id: 7,
        remark: 'local-socks',
        protocol: 'socks',
        port: 1080,
        network: 'tcp',
        security: 'none',
        enabled: true,
        clients: [{ id: 11, inbound_id: 7, email: 'sam@example.com', uuid: 'sam-uuid', credential_id: 'sam', stats_key: 'socks-stat', enabled: true }],
      },
      {
        id: 8,
        remark: 'local-http',
        protocol: 'http',
        port: 8080,
        network: 'tcp',
        security: 'none',
        enabled: true,
        clients: [{ id: 12, inbound_id: 8, email: 'ann@example.com', uuid: 'ann-uuid', credential_id: 'ann', stats_key: 'http-stat', enabled: true }],
      },
    ], 0, '');

    expect(options.find((option) => option.id === 11)).toMatchObject({
      title: 'sam',
      subtitle: 'sam@example.com',
      meta: expect.arrayContaining([{ label: '路由匹配：', value: 'sam' }]),
    });
    expect(options.find((option) => option.id === 12)).toMatchObject({
      title: 'ann',
      subtitle: 'ann@example.com',
      meta: expect.arrayContaining([{ label: '路由匹配：', value: 'ann' }]),
    });
  });

  it('falls back to uuid in picker and rule title when socks http credential_id is empty', () => {
    const text = (value: string) => value;
    const inbounds = [
      {
        id: 7,
        remark: 'local-socks',
        protocol: 'socks',
        port: 1080,
        network: 'tcp',
        security: 'none',
        enabled: true,
        clients: [{ id: 11, inbound_id: 7, email: 'sam@example.com', uuid: 'sam-uuid', credential_id: '', stats_key: 'socks-stat', enabled: true }],
      },
      {
        id: 8,
        remark: 'local-http',
        protocol: 'http',
        port: 8080,
        network: 'tcp',
        security: 'none',
        enabled: true,
        clients: [{ id: 12, inbound_id: 8, email: 'ann@example.com', uuid: 'ann-uuid', credential_id: '', stats_key: 'http-stat', enabled: true }],
      },
    ];

    const options = clientSelectionOptions(inbounds, 0, '');
    expect(options.find((option) => option.id === 11)).toMatchObject({
      title: 'sam-uuid',
      subtitle: 'sam@example.com',
      meta: expect.arrayContaining([{ label: '路由匹配：', value: 'sam-uuid' }]),
    });
    expect(options.find((option) => option.id === 12)).toMatchObject({
      title: 'ann-uuid',
      subtitle: 'ann@example.com',
      meta: expect.arrayContaining([{ label: '路由匹配：', value: 'ann-uuid' }]),
    });

    expect(ruleTitle({ id: 1, inbound_tag: 'inbound-7-socks', client_id: 11, outbound_tag: 'proxy-a', enabled: true }, text, inbounds)).toBe('local-socks / sam-uuid -> proxy-a');
    expect(ruleTitle({ id: 2, inbound_tag: 'inbound-8-http', client_id: 12, outbound_tag: 'proxy-a', enabled: true }, text, inbounds)).toBe('local-http / ann-uuid -> proxy-a');
  });

  it('matches vless vmess and trojan clients by stats_key before email', () => {
    const options = clientSelectionOptions([
      {
        id: 7,
        remark: 'vless-in',
        protocol: 'vless',
        port: 443,
        network: 'tcp',
        security: 'reality',
        enabled: true,
        clients: [{ id: 11, inbound_id: 7, email: 'alice@example.com', uuid: 'vless-uuid', stats_key: 'vless-stat', enabled: true }],
      },
      {
        id: 8,
        remark: 'vmess-in',
        protocol: 'vmess',
        port: 8443,
        network: 'ws',
        security: 'tls',
        enabled: true,
        clients: [{ id: 12, inbound_id: 8, email: 'bob@example.com', uuid: 'vmess-uuid', stats_key: 'vmess-stat', enabled: true }],
      },
      {
        id: 9,
        remark: 'trojan-in',
        protocol: 'trojan',
        port: 9443,
        network: 'tcp',
        security: 'tls',
        enabled: true,
        clients: [{ id: 13, inbound_id: 9, email: 'carol@example.com', uuid: 'trojan-uuid', stats_key: 'trojan-stat', enabled: true }],
      },
    ], 0, '');

    expect(options.find((option) => option.id === 11)).toMatchObject({
      title: 'vless-stat',
      subtitle: 'alice@example.com',
      meta: expect.arrayContaining([{ label: '路由匹配：', value: 'vless-stat' }]),
    });
    expect(options.find((option) => option.id === 12)).toMatchObject({
      title: 'vmess-stat',
      subtitle: 'bob@example.com',
      meta: expect.arrayContaining([{ label: '路由匹配：', value: 'vmess-stat' }]),
    });
    expect(options.find((option) => option.id === 13)).toMatchObject({
      title: 'trojan-stat',
      subtitle: 'carol@example.com',
      meta: expect.arrayContaining([{ label: '路由匹配：', value: 'trojan-stat' }]),
    });
  });

  it('falls back to email when non-socks http clients have no stats_key', () => {
    const options = clientSelectionOptions([
      {
        id: 7,
        remark: 'edge',
        protocol: 'vless',
        port: 443,
        network: 'tcp',
        security: 'reality',
        enabled: true,
        clients: [{ id: 11, inbound_id: 7, email: 'alice@example.com', uuid: 'u-1', enabled: true }],
      },
      {
        id: 8,
        remark: 'edge-vmess',
        protocol: 'vmess',
        port: 8443,
        network: 'ws',
        security: 'tls',
        enabled: true,
        clients: [{ id: 12, inbound_id: 8, email: 'bob@example.com', uuid: 'u-2', enabled: true }],
      },
      {
        id: 9,
        remark: 'edge-trojan',
        protocol: 'trojan',
        port: 9443,
        network: 'tcp',
        security: 'tls',
        enabled: true,
        clients: [{ id: 13, inbound_id: 9, email: 'carol@example.com', uuid: 'u-3', enabled: true }],
      },
    ], 0, '');

    expect(options.find((option) => option.id === 11)).toMatchObject({
      title: 'alice@example.com',
      subtitle: undefined,
      meta: expect.arrayContaining([{ label: '路由匹配：', value: 'alice@example.com' }]),
    });
    expect(options.find((option) => option.id === 12)).toMatchObject({
      title: 'bob@example.com',
      subtitle: undefined,
      meta: expect.arrayContaining([{ label: '路由匹配：', value: 'bob@example.com' }]),
    });
    expect(options.find((option) => option.id === 13)).toMatchObject({
      title: 'carol@example.com',
      subtitle: undefined,
      meta: expect.arrayContaining([{ label: '路由匹配：', value: 'carol@example.com' }]),
    });
  });

  it('keeps username email uuid and stats_key searchable for client options', () => {
    const options = clientSelectionOptions([
      {
        id: 7,
        remark: 'local-socks',
        protocol: 'socks',
        port: 1080,
        network: 'tcp',
        security: 'none',
        enabled: true,
        clients: [{ id: 11, inbound_id: 7, email: 'sam@example.com', uuid: 'sam-uuid', credential_id: 'sam', stats_key: 'socks-stat', enabled: true }],
      },
      {
        id: 8,
        remark: 'edge-vmess',
        protocol: 'vmess',
        port: 8443,
        network: 'ws',
        security: 'tls',
        enabled: true,
        clients: [{ id: 12, inbound_id: 8, email: 'bob@example.com', uuid: 'bob-uuid', stats_key: 'vmess-stat', enabled: true }],
      },
    ], 0, '');

    expect(options.find((option) => option.id === 11)?.search).toContain('sam');
    expect(options.find((option) => option.id === 11)?.search).toContain('sam@example.com');
    expect(options.find((option) => option.id === 11)?.search).toContain('sam-uuid');
    expect(options.find((option) => option.id === 12)?.search).toContain('bob@example.com');
    expect(options.find((option) => option.id === 12)?.search).toContain('bob-uuid');
    expect(options.find((option) => option.id === 12)?.search).toContain('vmess-stat');
  });

  it('computes route match identity with the same protocol rules as xray config generation', () => {
    expect(clientRouteMatchIdentity('socks', { credential_id: 'sam', uuid: 'sam-uuid', stats_key: 'socks-stat', email: 'sam@example.com' })).toBe('sam');
    expect(clientRouteMatchIdentity('http', { credential_id: 'ann', uuid: 'ann-uuid', stats_key: 'http-stat', email: 'ann@example.com' })).toBe('ann');
    expect(clientRouteMatchIdentity('socks', { credential_id: '', uuid: 'sam-uuid', stats_key: 'socks-stat', email: 'sam@example.com' })).toBe('sam-uuid');
    expect(clientRouteMatchIdentity('http', { credential_id: '', uuid: 'ann-uuid', stats_key: 'http-stat', email: 'ann@example.com' })).toBe('ann-uuid');
    expect(clientRouteMatchIdentity('vless', { credential_id: 'ignored', uuid: 'vless-uuid', stats_key: 'vless-stat', email: 'alice@example.com' })).toBe('vless-stat');
    expect(clientRouteMatchIdentity('vmess', { credential_id: 'ignored', uuid: 'vmess-uuid', stats_key: 'vmess-stat', email: 'bob@example.com' })).toBe('vmess-stat');
    expect(clientRouteMatchIdentity('trojan', { credential_id: 'ignored', uuid: 'trojan-uuid', stats_key: 'trojan-stat', email: 'carol@example.com' })).toBe('trojan-stat');
    expect(clientRouteMatchIdentity('vless', { credential_id: 'ignored', uuid: 'fallback-uuid', stats_key: '', email: 'fallback@example.com' })).toBe('fallback@example.com');
  });

  it('uses route match identity in rule titles for existing client rules', () => {
    const text = (value: string) => value;
    const inbounds = [
      {
        id: 7,
        remark: 'local-socks',
        protocol: 'socks',
        port: 1080,
        network: 'tcp',
        security: 'none',
        enabled: true,
        clients: [{ id: 11, inbound_id: 7, email: 'sam@example.com', uuid: 'sam-uuid', credential_id: 'sam', stats_key: 'socks-stat', enabled: true }],
      },
      {
        id: 8,
        remark: 'edge-vmess',
        protocol: 'vmess',
        port: 8443,
        network: 'ws',
        security: 'tls',
        enabled: true,
        clients: [{ id: 12, inbound_id: 8, email: 'bob@example.com', uuid: 'bob-uuid', stats_key: 'vmess-stat', enabled: true }],
      },
    ];

    expect(ruleTitle({ id: 1, inbound_tag: 'inbound-7-socks', client_id: 11, outbound_tag: 'proxy-a', enabled: true }, text, inbounds)).toBe('local-socks / sam -> proxy-a');
    expect(ruleTitle({ id: 2, inbound_tag: 'inbound-8-vmess', client_id: 12, outbound_tag: 'proxy-a', enabled: true }, text, inbounds)).toBe('edge-vmess / vmess-stat -> proxy-a');
  });

  it('shows outbound address and pool country details in route picker options', () => {
    const options = outboundSelectionOptions([
      { id: 9, tag: 'pool-https-205-178-136-32-8447', remark: 'Jacksonville AS19871 Web.com Group, Inc.', protocol: 'http', address: '205.178.136.32', port: 8447, enabled: true },
    ], new Map([
      ['https:205.178.136.32:8447', { address: '205.178.136.32', port: 8447, country: '美国', country_code: 'US', city: 'Jacksonville', asn: 'AS19871', organization: 'Web.com Group, Inc.' }],
    ]));

    expect(options[0].meta).toContainEqual({ label: '地址：', value: '205.178.136.32:8447' });
    expect(options[0].meta).toContainEqual({ label: '国家/地区：', value: '美国' });
  });

  it('uses outbound_id instead of stale outbound_tag in summaries', () => {
    const text = (value: string) => value;
    const outbounds = [
      { id: 1, tag: 'direct', protocol: 'freedom', enabled: true },
      { id: 2, tag: 'real-proxy', protocol: 'socks', enabled: true },
    ];
    expect(ruleTitle({ id: 10, outbound_id: 2, outbound_tag: 'old-proxy-tag', enabled: true }, text, [], outbounds)).toBe('全部入站 -> real-proxy');
    expect(routingPayload({ outbound_id: 2, outbound_tag: 'old-proxy-tag', enabled: true })).toMatchObject({
      outbound_id: 2,
      outbound_tag: 'old-proxy-tag',
    });
  });

  it('uses source and outbound summary instead of generic default titles', () => {
    const text = (value: string) => value;

    const inbounds = [
      { id: 7, remark: '深圳入口', protocol: 'vless', port: 443, network: 'tcp', security: 'reality', enabled: true, clients: [{ id: 11, inbound_id: 7, email: 'alice@example.com', uuid: 'u-1', enabled: true }] },
      { id: 8, remark: '', protocol: 'vmess', port: 8443, network: 'ws', security: 'tls', enabled: true, clients: [] },
    ];

    expect(ruleTitle({ id: 1, inbound_tag: 'haha', outbound_tag: 'direct', enabled: true }, text)).toBe('haha -> direct');
    expect(ruleTitle({ id: 2, outbound_tag: 'pool-socks', enabled: true }, text)).toBe('全部入站 -> pool-socks');
    expect(ruleTitle({ id: 3, domain: 'geosite:netflix, example.com', outbound_tag: 'proxy-a', enabled: true }, text)).toBe('geosite:netflix -> proxy-a');
    expect(ruleTitle({ id: 4, inbound_tag: 'inbound-7-vless', client_id: 11, client_email: 'alice@example.com', outbound_tag: 'proxy-a', enabled: true }, text)).toBe('inbound-7-vless / alice@example.com -> proxy-a');
    expect(ruleTitle({ id: 5, inbound_tag: 'inbound-7-vless', outbound_tag: 'proxy-a', enabled: true }, text, inbounds)).toBe('深圳入口 -> proxy-a');
    expect(ruleTitle({ id: 6, inbound_tag: 'inbound-7-vless', client_id: 11, outbound_tag: 'proxy-a', enabled: true }, text, inbounds)).toBe('深圳入口 / alice@example.com -> proxy-a');
    expect(ruleTitle({ id: 7, inbound_tag: 'inbound-8-vmess', outbound_tag: 'direct', enabled: true }, text, inbounds)).toBe('inbound-8-vmess -> direct');
    expect(ruleTitle({ id: 8, client_id: 11, outbound_tag: 'proxy-a', enabled: true }, text, inbounds)).toBe('深圳入口 / alice@example.com -> proxy-a');
    expect(ruleTitle({ id: 9, inbound_tag: 'inbound-7-vless', domain: 'geosite:netflix', outbound_tag: 'proxy-a', enabled: true }, text, inbounds)).toBe('深圳入口 -> proxy-a');
  });
});

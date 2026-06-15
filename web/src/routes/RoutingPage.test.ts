import { describe, expect, it } from 'vitest';
import type { RoutingRule } from '../api/types';
import { clientSelectionOptions, generatedInboundTag, inboundSelectionOptions, inboundTagOptions, movedRoutingRuleIds, outboundSelectionOptions, routingPayload, ruleTitle } from './RoutingPage';

describe('routing helpers', () => {
  it('builds create and edit payloads with backend field names only', () => {
    expect(routingPayload({ inbound_tag: 'edge', domain: 'geosite:netflix', ip: 'geoip:private', rule_set: 'geosite-category-ads-all', protocol: '', outbound_tag: 'proxy-a', enabled: true })).toEqual({
      inbound_tag: 'edge',
      client_id: 0,
      client_email: '',
      domain: 'geosite:netflix',
      ip: 'geoip:private',
      rule_set: 'geosite-category-ads-all',
      protocol: '',
      outbound_tag: 'proxy-a',
      enabled: true,
    });

    expect(routingPayload({ inbound_tag: undefined, domain: undefined, protocol: 'bittorrent', outbound_tag: 'blocked', enabled: false })).toEqual({
      inbound_tag: '',
      client_id: 0,
      client_email: '',
      domain: '',
      ip: '',
      rule_set: '',
      protocol: 'bittorrent',
      outbound_tag: 'blocked',
      enabled: false,
    });
  });

  it('preserves complete fields when toggling enabled state', () => {
    const rule: RoutingRule = { id: 8, inbound_tag: 'edge', domain: 'example.com', ip: '8.8.8.8', rule_set: 'geoip-cn', protocol: 'dns', outbound_tag: 'direct', enabled: true };
    expect(routingPayload({ ...rule, enabled: !rule.enabled })).toEqual({
      inbound_tag: 'edge',
      client_id: 0,
      client_email: '',
      domain: 'example.com',
      ip: '8.8.8.8',
      rule_set: 'geoip-cn',
      protocol: 'dns',
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
    ], 'inbound-7-vless');
    expect(clientOptions.find((option) => option.id === 11)).toMatchObject({
      title: '客户启用',
      meta: expect.arrayContaining([{ label: '入站：', value: '启用客户入口' }]),
    });
  });

  it('offers client options scoped by selected inbound and includes missing clients', () => {
    const options = clientSelectionOptions([
      { id: 7, remark: 'edge', protocol: 'VLESS', port: 443, network: 'tcp', security: 'reality', enabled: true, clients: [{ id: 11, inbound_id: 7, email: 'alice@example.com', uuid: 'u-1', enabled: true }] },
      { id: 8, remark: 'other', protocol: 'vmess', port: 8443, network: 'ws', security: 'tls', enabled: true, clients: [{ id: 12, inbound_id: 8, email: 'bob@example.com', uuid: 'u-2', enabled: true }] },
    ], 'inbound-7-vless', { client_id: 99, client_email: 'deleted@example.com' });

    expect(options.map((option) => option.id)).toEqual([0, 11, 99]);
    expect(options.find((option) => option.id === 11)).toMatchObject({
      email: 'alice@example.com',
      title: 'alice@example.com',
      inboundTag: 'inbound-7-vless',
      typeLabel: '客户端级',
    });
    expect(options.find((option) => option.id === 99)).toMatchObject({
      missing: true,
      title: 'deleted@example.com',
      typeLabel: '客户端已缺失',
    });
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

import { describe, expect, it } from 'vitest';
import type { RoutingRule } from '../api/types';
import { generatedInboundTag, inboundSelectionOptions, inboundTagOptions, movedRoutingRuleIds, outboundSelectionOptions, routingPayload, ruleTitle } from './RoutingPage';

describe('routing helpers', () => {
  it('builds create and edit payloads with backend field names only', () => {
    expect(routingPayload({ inbound_tag: 'edge', domain: 'geosite:netflix', ip: 'geoip:private', rule_set: 'geosite-category-ads-all', protocol: '', outbound_tag: 'proxy-a', enabled: true })).toEqual({
      inbound_tag: 'edge',
      domain: 'geosite:netflix',
      ip: 'geoip:private',
      rule_set: 'geosite-category-ads-all',
      protocol: '',
      outbound_tag: 'proxy-a',
      enabled: true,
    });

    expect(routingPayload({ inbound_tag: undefined, domain: undefined, protocol: 'bittorrent', outbound_tag: 'blocked', enabled: false })).toEqual({
      inbound_tag: '',
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
    ])).toEqual(['inbound-7-vless', 'edge', 'inbound-8-vmess']);
  });

  it('labels inbound actual tags and remark aliases clearly', () => {
    const options = inboundSelectionOptions([
      { id: 7, remark: 'edge', protocol: 'VLESS', port: 443, network: 'tcp', security: 'reality', enabled: true, clients: [{ id: 1, inbound_id: 7, email: 'alice', uuid: 'u', enabled: true }] },
    ]);

    expect(options[1]).toMatchObject({
      value: 'inbound-7-vless',
      title: 'inbound-7-vless',
      typeLabel: '实际 Tag',
    });
    expect(options[2]).toMatchObject({
      value: 'edge',
      title: 'edge',
      typeLabel: '备注别名',
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

    expect(ruleTitle({ id: 1, inbound_tag: 'haha', outbound_tag: 'direct', enabled: true }, text)).toBe('入站: haha -> direct');
    expect(ruleTitle({ id: 2, outbound_tag: 'pool-socks', enabled: true }, text)).toBe('全部入站 -> pool-socks');
    expect(ruleTitle({ id: 3, domain: 'geosite:netflix, example.com', outbound_tag: 'proxy-a', enabled: true }, text)).toBe('geosite:netflix -> proxy-a');
  });
});

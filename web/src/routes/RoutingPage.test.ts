import { describe, expect, it } from 'vitest';
import type { RoutingRule } from '../api/types';
import { generatedInboundTag, inboundTagOptions, movedRoutingRuleIds, routingPayload } from './RoutingPage';

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
});

import { describe, expect, it } from 'vitest';
import type { Outbound } from '../api/types';
import { customOutboundIds, isFixedDefaultOutbound, isReorderableOutbound, movedCustomOutboundIds, outboundFormValues, outboundMetaParts, outboundRemarkLabel } from './OutboundsPage';

describe('outbound helpers', () => {
  const outbounds: Outbound[] = [
    { id: 1, tag: 'direct', protocol: 'freedom', enabled: true },
    { id: 2, tag: 'blocked', protocol: 'blackhole', enabled: true },
    { id: 9, tag: 'proxy-a', protocol: 'socks', address: '127.0.0.1', port: 1080, enabled: true },
    { id: 10, tag: 'proxy-b', protocol: 'http', address: '127.0.0.2', port: 8080, enabled: true },
    { id: 11, tag: 'custom-direct', protocol: 'freedom', enabled: true },
  ];

  it('keeps non-reorderable protocols out of reorder payloads', () => {
    expect(customOutboundIds(outbounds)).toEqual([9, 10]);
    expect(movedCustomOutboundIds(outbounds.filter(isReorderableOutbound), 1, -1)).toEqual([10, 9]);
  });

  it('only marks direct and blocked tags as fixed defaults', () => {
    expect(isFixedDefaultOutbound(outbounds[0])).toBe(true);
    expect(isFixedDefaultOutbound(outbounds[1])).toBe(true);
    expect(isFixedDefaultOutbound(outbounds[2])).toBe(false);
    expect(isFixedDefaultOutbound(outbounds[4])).toBe(false);
    expect(isFixedDefaultOutbound({ id: 12, tag: 'direct', protocol: 'socks', enabled: true })).toBe(false);
    expect(isFixedDefaultOutbound({ id: 13, tag: 'blocked', protocol: 'http', enabled: true })).toBe(false);
    expect(isReorderableOutbound(outbounds[4])).toBe(false);
  });

  it('localizes built-in outbound remarks only', () => {
    const text = (value: string) => ({ 直接连接: 'Direct connection', 阻断: 'Blocked' })[value] || value;

    expect(outboundRemarkLabel('直接连接', text)).toBe('Direct connection');
    expect(outboundRemarkLabel('阻断', text)).toBe('Blocked');
    expect(outboundRemarkLabel('HK proxy', text)).toBe('HK proxy');
  });

  it('shows imported pool country details in outbound card metadata', () => {
    const text = (value: string) => value;

    expect(outboundMetaParts({
      protocol: 'http',
      address: '205.178.136.32',
      port: 8447,
      remark: 'Jacksonville AS19871 Web.com Group, Inc.',
    }, text, { country: 'United States', country_code: 'US' })).toEqual([
      'http',
      '205.178.136.32:8447',
      '国家/地区：United States',
      '备注：Jacksonville AS19871 Web.com Group, Inc.',
    ]);
  });

  it('derives outbound edit form values from the persisted outbound', () => {
    expect(outboundFormValues({
      id: 9,
      tag: 'proxy-a',
      remark: 'Proxy A',
      protocol: 'socks',
      address: '127.0.0.1',
      port: 1080,
      username: 'sam',
      password: 'secret',
      enabled: true,
    })).toMatchObject({
      tag: 'proxy-a',
      remark: 'Proxy A',
      protocol: 'socks',
      address: '127.0.0.1',
      port: 1080,
      username: 'sam',
      password: 'secret',
      enabled: true,
    });
  });
});

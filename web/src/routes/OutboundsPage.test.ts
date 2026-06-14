import { describe, expect, it } from 'vitest';
import type { Outbound } from '../api/types';
import { customOutboundIds, isFixedDefaultOutbound, isReorderableOutbound, movedCustomOutboundIds, outboundRemarkLabel } from './OutboundsPage';

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
    expect(isReorderableOutbound(outbounds[4])).toBe(false);
  });

  it('localizes built-in outbound remarks only', () => {
    const text = (value: string) => ({ 直接连接: 'Direct connection', 阻断: 'Blocked' })[value] || value;

    expect(outboundRemarkLabel('直接连接', text)).toBe('Direct connection');
    expect(outboundRemarkLabel('阻断', text)).toBe('Blocked');
    expect(outboundRemarkLabel('HK proxy', text)).toBe('HK proxy');
  });
});

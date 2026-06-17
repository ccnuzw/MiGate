import { describe, expect, it } from 'vitest';
import type { Outbound } from '../api/types';
import { outboundSupportedCores, outboundSupportLevel, outboundSupportLevelLabel } from '../lib/cores';
import { customOutboundIds, isFixedDefaultOutbound, isReorderableOutbound, movedCustomOutboundIds, outboundCredentialFields, outboundFormValues, outboundMetaParts, outboundPasswordLabel, outboundRemarkLabel, outboundUsernameLabel, sanitizeOutboundValues } from './OutboundsPage';

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

  it('labels constrained outbound credential fields by protocol', () => {
    expect(outboundUsernameLabel('shadowsocks')).toBe('Shadowsocks 加密方法');
    expect(outboundPasswordLabel('shadowtls')).toBe('ShadowTLS 密码');
    expect(outboundUsernameLabel('tuic')).toBe('UUID');
    expect(outboundUsernameLabel('vless')).toBe('UUID');
    expect(outboundPasswordLabel('vless')).toBe('密码');
  });

  it('derives credential field visibility by protocol', () => {
    expect(outboundCredentialFields('vless')).toEqual({ username: true, password: false });
    expect(outboundCredentialFields('tuic')).toEqual({ username: true, password: true });
    expect(outboundCredentialFields('shadowsocks')).toEqual({ username: true, password: true });
    expect(outboundCredentialFields('trojan')).toEqual({ username: false, password: true });
    expect(outboundCredentialFields('hysteria2')).toEqual({ username: false, password: true });
    expect(outboundCredentialFields('shadowtls')).toEqual({ username: false, password: true });
    expect(outboundCredentialFields('socks')).toEqual({ username: true, password: true });
  });

  it('derives outbound support level labels', () => {
    expect(outboundSupportLevelLabel('builtin')).toBe('内置');
    expect(outboundSupportLevelLabel('full')).toBe('完整');
    expect(outboundSupportLevelLabel('basic')).toBe('基础');
  });

  it('keeps outbound core support and support level aligned with backend protocol matrix', () => {
    const cases = [
      ['freedom', 'builtin', ['xray', 'sing-box']],
      ['blackhole', 'builtin', ['xray', 'sing-box']],
      ['dns', 'builtin', ['xray', 'sing-box']],
      ['direct', 'builtin', ['xray', 'sing-box']],
      ['block', 'builtin', ['xray', 'sing-box']],
      ['socks', 'full', ['xray', 'sing-box']],
      ['socks5', 'full', ['xray', 'sing-box']],
      ['http', 'full', ['xray', 'sing-box']],
      ['https', 'full', ['xray', 'sing-box']],
      ['vless', 'basic', ['xray', 'sing-box']],
      ['trojan', 'basic', ['xray', 'sing-box']],
      ['shadowsocks', 'basic', ['xray', 'sing-box']],
      ['hysteria2', 'basic', ['sing-box']],
      ['tuic', 'basic', ['sing-box']],
      ['shadowtls', 'basic', ['sing-box']],
      ['unknown', 'none', []],
    ] as const;
    cases.forEach(([protocol, level, cores]) => {
      expect(outboundSupportLevel({ protocol })).toBe(level);
      expect(outboundSupportedCores({ protocol })).toEqual(cores);
    });
  });

  it('clears hidden credential fields before saving', () => {
    expect(sanitizeOutboundValues({ protocol: 'vless', username: '11111111-1111-4111-8111-111111111111', password: 'unused' })).toMatchObject({
      username: '11111111-1111-4111-8111-111111111111',
      password: '',
    });
    expect(sanitizeOutboundValues({ protocol: 'trojan', username: 'unused', password: 'secret' })).toMatchObject({
      username: '',
      password: 'secret',
    });
  });

  it('clears hidden connection fields for builtin outbound protocols', () => {
    expect(sanitizeOutboundValues({ protocol: 'freedom', address: '127.0.0.1', port: 1080, username: 'unused', password: 'unused' })).toMatchObject({
      address: '',
      port: 0,
      username: '',
      password: '',
    });
    expect(sanitizeOutboundValues({ protocol: 'socks', address: '127.0.0.1', port: 1080, username: 'sam', password: 'secret' })).toMatchObject({
      address: '127.0.0.1',
      port: 1080,
      username: 'sam',
      password: 'secret',
    });
  });
});

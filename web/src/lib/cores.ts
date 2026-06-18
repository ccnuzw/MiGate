import type { Inbound, Outbound } from '../api/types';

export type CoreName = 'xray' | 'sing-box';
export type OutboundSupportLevel = 'builtin' | 'full' | 'basic' | 'none';

const singboxInboundProtocols = new Set(['hysteria2', 'tuic', 'shadowtls']);
const sharedOutboundProtocols = new Set(['socks', 'socks5', 'http', 'https', 'vless', 'trojan', 'shadowsocks', 'freedom', 'blackhole', 'direct', 'block', 'dns']);
const singboxOnlyOutboundProtocols = new Set(['hysteria2', 'tuic', 'shadowtls']);
const builtinOutboundProtocols = new Set(['freedom', 'blackhole', 'direct', 'block', 'dns']);
const fullOutboundProtocols = new Set(['socks', 'socks5', 'http', 'https']);
const basicOutboundProtocols = new Set(['vless', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'shadowtls']);

export function normalizeCore(value: unknown): CoreName {
  const raw = String(value || '').trim().toLowerCase();
  return raw === 'singbox' || raw === 'sing-box' ? 'sing-box' : 'xray';
}

export function inboundCore(inbound: Pick<Inbound, 'protocol' | 'core'>): CoreName {
  if (inbound.core) return normalizeCore(inbound.core);
  return singboxInboundProtocols.has(String(inbound.protocol || '').trim().toLowerCase()) ? 'sing-box' : 'xray';
}

export function outboundSupportedCores(outbound: Pick<Outbound, 'protocol'> | { protocol?: string }): CoreName[] {
  const protocol = String(outbound.protocol || '').trim().toLowerCase();
  if (singboxOnlyOutboundProtocols.has(protocol)) return ['sing-box'];
  if (sharedOutboundProtocols.has(protocol)) return ['xray', 'sing-box'];
  return [];
}

export function outboundSupportsCore(outbound: Pick<Outbound, 'protocol'> | { protocol?: string } | undefined, core: CoreName) {
  if (!outbound) return false;
  return outboundSupportedCores(outbound).includes(core);
}

export function outboundSupportLevel(outbound: Pick<Outbound, 'protocol'> | { protocol?: string } | undefined): OutboundSupportLevel {
  const protocol = String(outbound?.protocol || '').trim().toLowerCase();
  if (builtinOutboundProtocols.has(protocol)) return 'builtin';
  if (fullOutboundProtocols.has(protocol)) return 'full';
  if (basicOutboundProtocols.has(protocol)) return 'basic';
  return 'none';
}

export function outboundSupportLevelLabel(level: OutboundSupportLevel) {
  switch (level) {
    case 'builtin':
      return '内置';
    case 'full':
      return '完整';
    case 'basic':
      return '基础';
    default:
      return '';
  }
}

export function coreLabel(core: CoreName) {
  return core === 'sing-box' ? 'sing-box' : 'xray';
}
